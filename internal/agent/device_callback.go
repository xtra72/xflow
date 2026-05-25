// device_callback.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T2)
//
// 본 파일은 모든 에이전트가 공유하는 디바이스 상태 변경 콜백 시그니처를
// 정의한다.
//
// 시그니처 진화 (Soft Deprecation):
//
//   - v0.x (Phase A 까지): `func(agentName, deviceID string)` 에서 deviceID 는
//     composite key ("agent_name:local_id"). 외부 클라이언트가 이 형식에
//     결합되어 있으므로 호환 유지.
//
//   - v0.y (Phase B 부터): `DeviceStateChangeCallbackV2 = func(agentName,
//     deviceUID, deviceCompositeID string)` 에서 deviceUID 는 글로벌 UUID v4,
//     deviceCompositeID 는 v0.x 호환을 위한 composite key 병기. UUID 가 빈
//     문자열이면 (저장소 미설정) graceful degradation 으로 deviceCompositeID
//     만 의미를 가진다.
//
//   - v1.0 (Phase D): v1 시그니처 폐기, V2 시그니처 1급 — 다만 본 세션의 범위
//     밖이며 별도 메이저 버전 (xflowd v1.0) 으로 분리.
//
// 호환 어댑터 헬퍼:
//
//   각 에이전트는 V2 콜백을 1급으로 호출하면서, 등록된 v1 콜백 (기존 외부
//   클라이언트의 등록 경로) 도 동시에 호출해야 한다. AdaptLegacyCallback
//   헬퍼가 v1 → V2 어댑터를 생성하여 동시 등록을 단순화한다.

package agent

// DeviceStateChangeCallback 은 디바이스 상태 변경 시 호출되는 v0.x 시그니처이다.
//
// deviceID 는 composite key ("agent_name:local_id" — 예: "lgcnp:81") 이다.
//
// Deprecated: SPEC-DEVICE-IDENTITY-001 Phase B 의 1급 시그니처는
// DeviceStateChangeCallbackV2 이다. 본 시그니처는 호환 alias 로 v1.0 (Phase D)
// 까지 유지되며 그 이후 제거된다. 신규 호출자는 V2 를 사용하라.
type DeviceStateChangeCallback = func(agentName, deviceID string)

// DeviceStateChangeCallbackV2 는 디바이스 상태 변경 시 호출되는 Phase B 시그니처이다.
//
// 인자:
//   - agentName: 디바이스를 소유하는 에이전트의 Name() ("lgcnp", "samsung" 등).
//   - deviceUID: 디바이스의 글로벌 UUID v4 (Device.UID() 반환값).
//     DeviceIDRepository 미설정 / 매핑 부재 시 빈 문자열 (graceful degradation).
//   - deviceCompositeID: v0.x 호환을 위한 composite key ("agent_name:local_id").
//     Phase D 에서 제거 예정. 호출 측 (WebSocket / 로그 / 메트릭) 은 deviceUID
//     를 1급으로 사용하고 deviceCompositeID 는 Deprecation 헤더 / 메트릭 라벨
//     용으로만 활용해야 한다.
//
// 호출 사이트는 deviceUID 가 빈 문자열인 경우에도 callback 을 호출한다. 호출
// 측 책임으로 deviceUID 검증 후 fallback (deviceCompositeID 사용 또는 skip)
// 처리한다. observe.IncDeviceUIDMissing 카운터가 이미 호출 빈도를 추적하므로
// 별도 panic 이나 에러는 발생시키지 않는다.
//
// SPEC-DEVICE-IDENTITY-001 § M3.
type DeviceStateChangeCallbackV2 = func(agentName, deviceUID, deviceCompositeID string)

// AdaptLegacyCallback 은 v0.x 시그니처 콜백을 V2 시그니처 어댑터로 변환한다.
//
// 변환 규칙: V2 의 deviceCompositeID 를 v1 의 deviceID 인자로 그대로 전달한다.
// deviceUID 는 v1 시그니처에 존재하지 않으므로 drop. 결과적으로 v1 콜백은
// composite key 만 받게 되어 v0.x 동작과 정확히 동일하다.
//
// 호출자 패턴 (각 에이전트의 콜백 emit 사이트):
//
//	if v2 := a.onDeviceStateChangeV2; v2 != nil {
//	    go v2(agentName, deviceUID, compositeID)
//	}
//	if v1 := a.onDeviceStateChange; v1 != nil {
//	    go v1(agentName, compositeID)
//	}
//
// fn 이 nil 이면 nil 반환 (no-op wrapper 회피).
func AdaptLegacyCallback(fn DeviceStateChangeCallback) DeviceStateChangeCallbackV2 {
	if fn == nil {
		return nil
	}
	return func(agentName, _, deviceCompositeID string) {
		fn(agentName, deviceCompositeID)
	}
}
