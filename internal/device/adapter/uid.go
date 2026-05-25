// uid.go (SPEC-DEVICE-IDENTITY-001 Phase A — M1)
//
// 본 파일은 모든 device.Device 어댑터가 공유하는 UID() 구현 헬퍼를 제공한다.
//
// 각 어댑터는 다음 두 가지를 가지고 있다:
//   - agentName (owning 에이전트의 Name())
//   - localID  (어댑터 내부의 디바이스 식별자 — NASA address / LGCNP address /
//               LGCP address / Century sub_dev_id hex / Modbus device_id)
//
// composite ID 는 항상 "agentName:localID" 형식이므로, 어댑터의 UID() 구현은
// ResolveAdapterUID(agentName, localID) 한 줄 호출로 동일하게 작성된다.
//
// 비용 모델:
//   - 첫 호출은 agent.ResolveDeviceID 를 통한 (in-process) repository 조회.
//     DeviceIDFileRepository / DeviceIDMemoryRepository 는 in-memory 캐시를
//     보유하므로 O(1) ~ O(log n).
//   - 어댑터는 매 요청마다 새로 인스턴스화될 수 있으므로 (provider.Devices 가
//     매번 새 슬라이스를 만든다), per-instance cache 는 의미가 없다.
//   - 빈 문자열 반환 시 (DeviceIDRepository 미설정 / 매핑 부재 / 에러)
//     observe.IncDeviceUIDMissing(agentName) 로 메트릭 카운터를 증가시킨다.
//
// 그래스풀 디그라데이션:
//   - DeviceIDRepository 미설정 → 빈 문자열 반환 (Phase A 허용).
//   - 호출자 (REST 응답 / emit payload) 는 빈 문자열이면 uid 키를 생략한다.

package adapter

import (
	"context"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/observe"
)

// ResolveAdapterUID 는 (agentName, localID) 의 글로벌 UUID 를 조회한다.
//
// Phase A 그래스풀 디그라데이션 동작:
//   - agentName 이 빈 문자열이면 즉시 "" 반환 (호출자 책임).
//   - localID 가 빈 문자열이면 즉시 "" 반환 (의미 없는 조회 방지).
//   - DeviceIDRepository 가 미설정이거나 에러를 반환하면 "" 반환 후
//     xflowd_device_uid_missing_total 카운터 증가.
//
// 호출 패턴 (모든 device.Device 어댑터의 UID() 메서드):
//
//	func (a *FooAdapter) UID() string {
//	    return ResolveAdapterUID(a.agentName, a.localID())
//	}
func ResolveAdapterUID(agentName, localID string) string {
	if agentName == "" || localID == "" {
		// 빈 입력은 메트릭에 기록하지 않는다 (정상 의도된 호출 — 예: 미초기화
		// 어댑터 — 가 아닌 한 발생하지 않음). 발생 시 호출 사이트의 버그.
		return ""
	}
	uid := agent.ResolveDeviceID(context.Background(), agentName, localID)
	if uid == "" {
		// graceful degradation 시그널: DeviceIDRepository 미설정 또는 에러.
		// Phase D 에서 부팅 실패로 전환할 신호로 사용된다.
		observe.IncDeviceUIDMissing(agentName)
	}
	return uid
}
