package airpurifier

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// B3 — 제어 응답 대기 / pending 레지스트리 (acceptance.md 3B 시나리오)
//
// 모든 대기 테스트는 짧은 timeout(50~200ms)을 사용하며 실제 5s 를 기다리지 않는다.
// ---------------------------------------------------------------------------

// respWaitOpts 는 지정 control_response_timeout 을 갖는 단일 디바이스 direct 옵션을 반환한다.
func respWaitOpts(timeout time.Duration) map[string]any {
	opts := directOpts()
	opts["control_response_timeout"] = timeout.String()
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	return opts
}

// publishedLen 은 mock 의 발행 수를 락 하에 반환한다 (고루틴 동시 관측 안전, -race).
func (m *mockMQTTClient) publishedLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// waitForPublish 는 발행 수가 n 이상이 될 때까지 폴링한다 (관측되면 pending 등록이 이미 끝났음).
func waitForPublish(m *mockMQTTClient, n int) bool {
	for i := 0; i < 2000; i++ {
		if m.publishedLen() >= n {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// 3B.1: control_response_timeout>0, set_power true, 매칭 power=true 에코를 timeout 전에 주입 →
// status "ok". pending 이 응답 반환 전에 등록되었음을 확인한다.
func TestB3_ResponseWaitSuccess(t *testing.T) {
	ap, mock := directAgentWithMock(t, respWaitOpts(2*time.Second))

	regCh := make(chan int, 1)
	go func() {
		// 발행 관측 → register→emit 순서상 pending 은 이미 등록됨.
		waitForPublish(mock, 1)
		regCh <- ap.pendings.len()
		ap.FeedState("ap-101", []byte(`{"power":true}`)) // 매칭 에코 주입.
	}()

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.GreaterOrEqual(t, <-regCh, 1, "pending 이 응답 반환 전에 등록되어야 한다")
	assert.Equal(t, 0, ap.pendings.len(), "에코 해소 후 pending 제거")
}

// 3B.2: 에코 없음 → timeout 내 미수신 → ErrControlTimeout, pending 제거.
func TestB3_ResponseWaitTimeout(t *testing.T) {
	ap, _ := directAgentWithMock(t, respWaitOpts(50*time.Millisecond))

	start := time.Now()
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrControlTimeout)
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond, "타임아웃까지 대기해야 한다")
	assert.Equal(t, 0, ap.pendings.len(), "타임아웃 후 pending 제거")
}

// controlDevice 는 타임아웃 시 memberResult.Status="timeout" 을 반환한다 (B4 fan-out 집계 계약).
func TestB3_ControlDeviceTimeoutResult(t *testing.T) {
	ap, _ := directAgentWithMock(t, respWaitOpts(50*time.Millisecond))
	pw := true
	res, err := ap.controlDevice("ap-101", "set_power", commandPayload{Power: &pw})
	assert.ErrorIs(t, err, ErrControlTimeout)
	assert.Equal(t, "timeout", res.Status)
	assert.Equal(t, "ap-101", res.DeviceID)
	assert.Equal(t, "set_power", res.Command)
}

// controlDevice 는 에코 해소 시 memberResult.Status="ok" 를 반환한다 (B4 계약).
func TestB3_ControlDeviceOKResult(t *testing.T) {
	ap, mock := directAgentWithMock(t, respWaitOpts(2*time.Second))
	go func() {
		waitForPublish(mock, 1)
		ap.FeedState("ap-101", []byte(`{"power":true}`))
	}()
	pw := true
	res, err := ap.controlDevice("ap-101", "set_power", commandPayload{Power: &pw})
	require.NoError(t, err)
	assert.Equal(t, "ok", res.Status)
}

// 3B.3: port 모드 — set_power 를 제어 포트로 방출, 포트 drain 후 power=true 에코를 FeedState
// 주입 → 동일 디코드/상관 경로로 pending 해소 (추가 코드 없이 FeedState→handleStatePayload).
func TestB3_PortModeResolve(t *testing.T) {
	opts := map[string]any{
		"transport_mode":           "port",
		"payload_mapping":          validPayloadMapping(),
		"control_response_timeout": "2s",
		"devices":                  []any{map[string]any{"device_id": "ap-101"}},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	go func() {
		select {
		case <-ap.ControlPort(): // 제어 포트 방출 관측 → pending 등록 완료.
			ap.FeedState("ap-101", []byte(`{"power":true}`))
		case <-time.After(2 * time.Second):
		}
	}()

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.Equal(t, 0, ap.pendings.len())
}

// 3B.4: pending 이 이미 타임아웃 제거된 뒤 도착한 late echo 는 로스터만 갱신하고 소급 해소하지
// 않는다 (REQ-03-10).
func TestB3_LateEchoNoRetroactiveResolve(t *testing.T) {
	ap, _ := directAgentWithMock(t, respWaitOpts(50*time.Millisecond))

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.ErrorIs(t, err, ErrControlTimeout)
	require.Equal(t, 0, ap.pendings.len())

	// 늦은 에코: 로스터는 갱신되지만 pending 은 없으므로 소급 해소 없음.
	ap.FeedState("ap-101", []byte(`{"power":true,"fan_speed":2}`))
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Power, "late echo 는 로스터를 정상 갱신한다")
	assert.Equal(t, 2, dev.FanSpeed)
	assert.Equal(t, 0, ap.pendings.len(), "late echo 는 pending 을 만들지도 해소하지도 않는다")
}

// 3B.5: 같은 device 의 set_power + set_fan_speed 가 동시 pending. power-only 에코만 도착 →
// set_power 는 "ok" 해소, set_fan_speed 는 독립적으로 타임아웃 (REQ-03-10 동시성).
func TestB3_ConcurrentIndependentPendings(t *testing.T) {
	ap, mock := directAgentWithMock(t, respWaitOpts(200*time.Millisecond))

	type result struct {
		res memberResult
		err error
	}
	powerCh := make(chan result, 1)
	fanCh := make(chan result, 1)

	pw := true
	go func() {
		res, err := ap.controlDevice("ap-101", "set_power", commandPayload{Power: &pw})
		powerCh <- result{res, err}
	}()
	fs := 2
	go func() {
		res, err := ap.controlDevice("ap-101", "set_fan_speed", commandPayload{FanSpeed: &fs})
		fanCh <- result{res, err}
	}()

	// 두 명령 모두 방출 + 독립 pending 2개 등록 확인.
	require.True(t, waitForPublish(mock, 2), "두 명령 모두 방출되어야 한다")
	require.Eventually(t, func() bool { return ap.pendings.len() == 2 }, time.Second, time.Millisecond,
		"set_power/set_fan_speed 는 독립 pending 2개여야 한다")

	// power-only 에코 → set_power 만 해소.
	ap.FeedState("ap-101", []byte(`{"power":true}`))

	pr := <-powerCh
	require.NoError(t, pr.err, "set_power 는 에코로 해소되어야 한다")
	assert.Equal(t, "ok", pr.res.Status)

	fr := <-fanCh
	assert.ErrorIs(t, fr.err, ErrControlTimeout, "set_fan_speed 는 독립적으로 타임아웃되어야 한다")
	assert.Equal(t, "timeout", fr.res.Status)

	assert.Equal(t, 0, ap.pendings.len(), "양쪽 종결 후 pending 없음")
}

// 3B.6: timeout=0 → fire-and-forget, 즉시 "ok", pending 미등록.
func TestB3_TimeoutZeroFireAndForget(t *testing.T) {
	ap, mock := directAgentWithMock(t, respWaitOpts(0))

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.Equal(t, 1, mock.publishedLen(), "명령은 방출되어야 한다")
	assert.Equal(t, 0, ap.pendings.len(), "timeout=0 은 pending 을 등록하지 않는다")
}

// 고루틴/타이머 누수 없음: N 회 제어+타임아웃 사이클 후 pending 이 모두 제거되고 고루틴 수가
// 안정적이어야 한다 (AfterFunc 타이머는 모든 종결 경로에서 Stop 된다).
func TestB3_NoGoroutineLeakAfterCycles(t *testing.T) {
	ap, _ := directAgentWithMock(t, respWaitOpts(20*time.Millisecond))

	const cycles = 30
	runCycle := func() {
		for i := 0; i < cycles; i++ {
			_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
			require.ErrorIs(t, err, ErrControlTimeout)
		}
	}

	runCycle() // 워밍업 (초기 런타임 고루틴 안정화).
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	runCycle()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	assert.Equal(t, 0, ap.pendings.len(), "모든 pending 이 제거되어야 한다")
	assert.LessOrEqual(t, after, before+2,
		"제어+타임아웃 사이클 후 고루틴 수가 안정적이어야 한다 (누수 없음): before=%d after=%d", before, after)
}

// 에이전트 Stop 은 대기 중인 pending 을 즉시 해제하고 정리한다 (누수 방지).
func TestB3_StopReleasesPendings(t *testing.T) {
	ap, mock := directAgentWithMock(t, respWaitOpts(5*time.Second))

	waitCh := make(chan error, 1)
	pw := true
	go func() {
		_, err := ap.controlDevice("ap-101", "set_power", commandPayload{Power: &pw})
		waitCh <- err
	}()

	require.True(t, waitForPublish(mock, 1))
	require.Eventually(t, func() bool { return ap.pendings.len() == 1 }, time.Second, time.Millisecond)

	require.NoError(t, ap.Stop(context.Background()))

	select {
	case err := <-waitCh:
		assert.ErrorIs(t, err, ErrControlTimeout, "Stop 은 대기 중 pending 을 해제한다")
	case <-time.After(time.Second):
		t.Fatal("Stop 후에도 controlDevice 가 해제되지 않았다 (누수)")
	}
	assert.Equal(t, 0, ap.pendings.len())
}

// expectFromPayload + matches 단위 검증: 기대 변화 매칭 규칙 (축별 관측+값 일치).
func TestB3_ExpectMatches(t *testing.T) {
	pTrue := true
	e := expectFromPayload(commandPayload{Power: &pTrue})
	assert.True(t, e.matches(decodedState{PowerSet: true, Power: true}))
	assert.False(t, e.matches(decodedState{PowerSet: true, Power: false}), "값 불일치")
	assert.False(t, e.matches(decodedState{}), "미관측(PowerSet=false) → 불일치")

	fs := 2
	ef := expectFromPayload(commandPayload{FanSpeed: &fs})
	assert.True(t, ef.matches(decodedState{FanSpeedSet: true, FanSpeed: 2}))
	assert.False(t, ef.matches(decodedState{FanSpeedSet: true, FanSpeed: 3}), "값 불일치")
	assert.False(t, ef.matches(decodedState{PowerSet: true, Power: true}), "fan 축 미관측")

	// 빈 기대는 어떤 상태로도 해소하지 않는다 (방어).
	assert.False(t, pendingExpect{}.matches(decodedState{PowerSet: true, Power: true}))

	// set_multiple: power=false + fan 은 두 축 모두 일치해야 매칭.
	pFalse := false
	em := expectFromPayload(commandPayload{Power: &pFalse, FanSpeed: &fs})
	assert.True(t, em.matches(decodedState{PowerSet: true, Power: false, FanSpeedSet: true, FanSpeed: 2}))
	assert.False(t, em.matches(decodedState{PowerSet: true, Power: false}), "fan 축 미관측 → 미해소")
}

// 방출 실패 시 등록된 pending 이 정리되어 대기자/타이머가 누수되지 않는다.
func TestB3_EmitFailureCancelsPending(t *testing.T) {
	a, err := NewAirPurifierAgent(baseAgentConfig(respWaitOpts(5 * time.Second)))
	require.NoError(t, err)
	ap := asAP(t, a)

	// 미연결 mock → brokerCommandSink.SendCommand 가 ErrNotConnected 를 반환한다.
	mock := newMockMQTTClient() // Connect() 호출하지 않음.
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}

	pw := true
	_, err = ap.controlDevice("ap-101", "set_power", commandPayload{Power: &pw})
	assert.ErrorIs(t, err, ErrNotConnected)
	assert.Equal(t, 0, ap.pendings.len(), "방출 실패 시 pending 이 정리되어야 한다 (누수 방지)")
}

// register 의 supersede(같은 key 재등록)와 closed(정지 후 등록) 경로를 직접 검증한다.
func TestB3_RegistrySupersedeAndClosed(t *testing.T) {
	r := newPendingRegistry()
	pw := true
	expect := expectFromPayload(commandPayload{Power: &pw})

	// 같은 key 재등록 → 선행 pending supersede (즉시 타임아웃 해제).
	p1 := r.register("ap-1", "set_power", expect, time.Second)
	p2 := r.register("ap-1", "set_power", expect, time.Second)
	assert.NotSame(t, p1, p2)
	assert.ErrorIs(t, p1.wait(), ErrControlTimeout, "supersede 된 선행 pending 은 즉시 해제된다")
	assert.Equal(t, 1, r.len(), "레지스트리에는 최신 pending 만 남는다")

	// 최신 pending 은 에코로 해소.
	r.resolve("ap-1", decodedState{PowerSet: true, Power: true})
	assert.NoError(t, p2.wait())
	assert.Equal(t, 0, r.len())

	// close 후 register 는 즉시 타임아웃 pending 을 반환한다.
	r.close()
	p3 := r.register("ap-1", "set_power", expect, time.Second)
	assert.ErrorIs(t, p3.wait(), ErrControlTimeout, "닫힌 레지스트리의 register 는 즉시 타임아웃")
	assert.Equal(t, 0, r.len())
}

// expectFromPayload 는 원본 포인터 aliasing 없이 값을 복사한다.
func TestB3_ExpectFromPayloadCopies(t *testing.T) {
	pw := true
	e := expectFromPayload(commandPayload{Power: &pw})
	pw = false // 원본 변경이 기대에 영향을 주지 않아야 한다.
	require.NotNil(t, e.power)
	assert.True(t, *e.power, "expectFromPayload 는 값을 복사해야 한다")
}
