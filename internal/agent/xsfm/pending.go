package xsfm

import (
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// B3 — 제어 응답 대기 / pending 명령 레지스트리 (REQ-XSFM-001-03-08/09/10)
// ---------------------------------------------------------------------------
//
// 제어 명령을 방출한 뒤 control_response_timeout 내에 기대한 상태 에코가 유입되면 명령을
// "해소(ok)"로, 아니면 "타임아웃"으로 종결한다. 상관(correlation)은 device_id + command kind
// + 기대 값 일치로 한다.
//
// 동시성 규약:
//   - 레지스트리는 로스터 락(XSFMAgent.mu)과 절대 공유·중첩하지 않는 자체 mu 로만
//     보호된다 (프로젝트의 RWMutex 재진입 deadlock 트랩 회피). resolve 는 상태 유입 경로에서
//     로스터 락 해제 후 호출된다.
//   - (device_id, command kind) 별 pending 은 독립이다: 한 pending 의 해소/타임아웃이 다른
//     pending 에 영향을 주지 않는다 (REQ-03-10).
//   - 대기는 호출자 고루틴에서 채널 수신으로 이뤄지며 별도 고루틴을 만들지 않는다. 유일한
//     비동기 요소는 time.AfterFunc 타이머로, 모든 종결 경로에서 Stop 되어 타이머/고루틴 누수가
//     없다.

// pendingKey 는 pending 을 device_id + command kind 로 식별한다. 같은 device 라도 command
// kind 가 다르면(set_power vs set_fan_speed) 독립 엔트리이다 (REQ-03-10).
type pendingKey struct {
	deviceID string
	command  string
}

// pendingExpect 는 제어 명령이 유발할 것으로 기대하는 상태 변화이다. 설정된 축(non-nil)만
// 에코 매칭에 사용된다 — commandPayload 를 그대로 반영한다.
type pendingExpect struct {
	power    *bool
	fanSpeed *int
}

// expectFromPayload 는 commandPayload 로부터 기대 변화를 도출한다 (값 복사로 aliasing 회피).
func expectFromPayload(cmd commandPayload) pendingExpect {
	var e pendingExpect
	if cmd.Power != nil {
		v := *cmd.Power
		e.power = &v
	}
	if cmd.FanSpeed != nil {
		v := *cmd.FanSpeed
		e.fanSpeed = &v
	}
	return e
}

// isEmpty 는 어떤 축도 기대하지 않는지 반환한다 (등록 방어용).
func (e pendingExpect) isEmpty() bool { return e.power == nil && e.fanSpeed == nil }

// matches 는 유입 디코딩 상태 st 가 기대 변화를 모두 반영하는지 판정한다. 설정된 각 축이 st
// 에서 관측(*Set)되고 값이 일치해야 한다. 빈 기대는 어떤 상태로도 해소하지 않는다(방어).
func (e pendingExpect) matches(st decodedState) bool {
	if e.isEmpty() {
		return false
	}
	if e.power != nil && (!st.PowerSet || st.Power != *e.power) {
		return false
	}
	if e.fanSpeed != nil && (!st.FanSpeedSet || st.FanSpeed != *e.fanSpeed) {
		return false
	}
	return true
}

// pending 은 단일 제어 명령의 응답 대기 엔트리이다.
type pending struct {
	key    pendingKey
	expect pendingExpect

	done  chan struct{} // finalize 시 정확히 한 번 닫힌다.
	timer *time.Timer   // control_response_timeout AfterFunc 타이머.

	// final/outcome 은 registry.mu 하에서만 접근한다. done 이 닫히기 전에 outcome 이 설정되고,
	// done 닫힘이 wait 측에 happens-before 로 전파되므로 wait 는 락 없이 outcome 을 읽는다.
	final   bool
	outcome error // nil(에코 해소) 또는 ErrControlTimeout(타임아웃/취소/정지/supersede).
}

// wait 는 에코 해소 또는 타임아웃까지 블록하고 결과를 반환한다. 호출자 고루틴에서 실행되며
// 별도 고루틴을 만들지 않는다 (nil=ok, ErrControlTimeout=timeout).
func (p *pending) wait() error {
	<-p.done
	return p.outcome
}

// pendingRegistry 는 (device_id, command kind) → *pending 레지스트리이다. 자체 mu 로만
// 보호되며 로스터 락과 공유하지 않는다.
type pendingRegistry struct {
	mu     sync.Mutex
	m      map[pendingKey]*pending
	closed bool
}

// newPendingRegistry 는 빈 레지스트리를 생성한다.
func newPendingRegistry() *pendingRegistry {
	return &pendingRegistry{m: make(map[pendingKey]*pending)}
}

// register 는 방출 직전에 pending 을 등록한다. register→emit→wait 순서를 지키면 방출 직후
// 되돌아오는 에코를 놓치지 않는다 (REQ-03-08). timeout>0 전제로 호출한다. 같은 key 의 선행
// pending 은 supersede(타임아웃 종결) 후 교체한다. 레지스트리가 이미 닫혔으면(에이전트 정지)
// 즉시 타임아웃 종결된 pending 을 반환한다.
func (r *pendingRegistry) register(deviceID, command string, expect pendingExpect, timeout time.Duration) *pending {
	key := pendingKey{deviceID: deviceID, command: command}
	p := &pending{key: key, expect: expect, done: make(chan struct{})}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		p.final = true
		p.outcome = ErrControlTimeout
		close(p.done)
		return p
	}
	if old, ok := r.m[key]; ok {
		r.finalizeLocked(old, ErrControlTimeout) // 선행 명령 supersede — 대기자 즉시 해제.
	}
	r.m[key] = p
	// AfterFunc 콜백은 r.mu 를 획득하려 하므로 register 가 unlock 하기 전엔 실행되지 못한다.
	p.timer = time.AfterFunc(timeout, func() {
		r.mu.Lock()
		if cur, ok := r.m[key]; ok && cur == p {
			r.finalizeLocked(p, ErrControlTimeout)
		}
		r.mu.Unlock()
	})
	return p
}

// resolve 는 유입 상태 st 로 deviceID 의 기대 일치 pending 을 모두 해소(제거)한다. 상태 유입
// 경로에서 로스터 락 해제 후 호출된다 (락 중첩 금지). 매칭 pending 이 없으면 — 이미 타임아웃
// 제거된 late echo 포함 — no-op 이다: 소급 해소하지 않는다 (REQ-03-10 late echo).
func (r *pendingRegistry) resolve(deviceID string, st decodedState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, p := range r.m {
		if key.deviceID == deviceID && p.expect.matches(st) {
			r.finalizeLocked(p, nil)
		}
	}
}

// cancel 은 방출 실패 등으로 대기가 불필요해진 pending 을 정리한다 (타이머/맵 정리).
func (r *pendingRegistry) cancel(p *pending) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeLocked(p, ErrControlTimeout)
}

// close 는 에이전트 정지 시 모든 미해소 pending 을 종결한다 (대기자 즉시 해제, 타이머 정지).
// 이후 register 는 즉시 타임아웃 pending 을 반환한다.
func (r *pendingRegistry) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for _, p := range r.m {
		r.finalizeLocked(p, ErrControlTimeout)
	}
}

// len 은 미해소 pending 수를 반환한다 (관측/테스트용).
func (r *pendingRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}

// finalizeLocked 는 pending 을 종결한다: 타이머 정지 · 맵 제거 · done 1회 close. r.mu 를 보유한
// 채 호출해야 한다. 중복 호출은 final 가드로 무해하다 (해소/타임아웃 race 안전). timer.Stop 은
// 콜백을 동기 실행하지 않으므로 재진입 락이 발생하지 않는다.
func (r *pendingRegistry) finalizeLocked(p *pending, outcome error) {
	if p.final {
		return
	}
	p.final = true
	p.outcome = outcome
	if p.timer != nil {
		p.timer.Stop()
	}
	delete(r.m, p.key)
	close(p.done)
}
