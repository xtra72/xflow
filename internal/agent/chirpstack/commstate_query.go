package chirpstack

import (
	"encoding/json"
	"fmt"
)

// CommSnapshot 은 devEui 의 마지막 캐시된 comm-state 를 값 복사로 반환한다
// (SPEC-CHIRPSTACK-002 REQ-M3-01/03).
//
// 엔트리가 없으면 zero-value(online=false, last_seen_ms=0) + ok=false 를 반환한다.
// 호출자는 이를 offline/unknown 으로 취급해야 하며 online=true 를 조기 보고해서는
// 안 된다 (REQ-M3-04, SPEC-001 REQ-M5-06 과 일관).
//
// HVAC 락 함정 회피(REQ-FROZEN-B, R4): commMu 보유 구간에서 a.Name() 등 a.mu 를 다시
// 잡는 에이전트 메서드를 호출하지 않는다 — commMu → a.mu 락 순서 엣지를 새로 만들지
// 않는다(v0.18.6 재귀 RLock deadlock 트랩 재현 금지). 내부 포인터(*commEntry)는
// 노출하지 않고 값만 복사해 반환한다(맵 엔트리 외부 변조 차단).
func (a *ChirpStackAgent) CommSnapshot(devEui string) (commEntry, bool) {
	if devEui == "" {
		return commEntry{}, false
	}

	a.commMu.Lock()
	e, ok := a.comm[devEui]
	if !ok || e == nil {
		a.commMu.Unlock()
		return commEntry{}, false
	}
	snap := *e
	a.commMu.Unlock()

	return snap, true
}

// CommStateRecordJSON 은 devEui 의 캐시된 comm-state 를 device_state 레코드 JSON 으로
// 직렬화한다 (REQ-M3-01). status 노드가 소비하는 패키지 경계 접근자이다.
//
// commEntry 는 패키지 비공개 타입이라 노드 패키지가 직접 다룰 수 없으므로, 값 복사
// 스냅샷을 수신 경로와 동일한 빌더(buildDeviceStateRecord)로 조립해 JSON 으로만
// 넘긴다 — 방출 shape 가 수신 경로와 갈라지는 2차 구현을 만들지 않기 위함이다.
//
// 엔트리 부재 시 zero-value 엔트리로 조립되어 online=false / last_seen_ms=0
// (offline/unknown) 이 방출된다 (REQ-M3-04).
func (a *ChirpStackAgent) CommStateRecordJSON(devEui, trigger string) ([]byte, error) {
	snap, _ := a.CommSnapshot(devEui)

	b, err := json.Marshal(buildDeviceStateRecord(devEui, trigger, snap))
	if err != nil {
		return nil, fmt.Errorf("chirpstack: device_state 레코드 직렬화 실패: %w", err)
	}
	return b, nil
}

// CommStateEnabled 는 emit_comm_state 노브의 활성화 여부를 반환한다.
//
// comm 맵은 이 노브가 켜져 있을 때만 채워지므로(agent.go handleUplink), 노브가 꺼진
// 에이전트에 붙은 status 노드는 항상 offline/unknown 을 방출한다. status 노드는 Init
// 에서 이 값을 확인해 1회 경고를 남긴다(전제조건 공개 — REQ-M3-01 의 "comm 맵에서
// 방출" 문구는 그대로 유지된다).
//
// csConfig 는 Configure 가 런타임에 교체할 수 있으므로 cs() 스냅샷으로 읽는다
// (handleUplink 의 기존 관용구와 동일).
func (a *ChirpStackAgent) CommStateEnabled() bool {
	return a.cs().EmitCommState
}
