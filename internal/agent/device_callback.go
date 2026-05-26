// device_callback.go (SPEC-DEVICE-IDENTITY-001 Phase D — D-T10)
//
// 본 파일은 모든 에이전트가 공유하는 디바이스 상태 변경 콜백 시그니처를
// 정의한다.
//
// Phase D (xflowd v1.0) 부터 V2 시그니처가 유일한 1급 콜백이며 v0.x 시그니처
// (`DeviceStateChangeCallback`) 와 호환 어댑터 (`AdaptLegacyCallback`) 는
// 완전히 제거되었다. greenfield 환경 가정 (외부 클라이언트 부재) 에 따라
// 호환 alias 유지 비용이 사라졌다.

package agent

// DeviceStateChangeCallbackV2 는 디바이스 상태 변경 시 호출되는 1급 시그니처이다.
//
// 인자:
//   - agentName: 디바이스를 소유하는 에이전트의 Name() ("lgcnp", "samsung" 등).
//   - deviceUID: 디바이스의 글로벌 UUID v4 (Device.UID() 반환값).
//     DeviceIDRepository 미설정 / 매핑 부재 시 빈 문자열 (graceful degradation).
//   - deviceCompositeID: v0.x 호환 정보용 composite key ("agent_name:local_id").
//     Phase D 에서 외부 노출이 사라졌으므로 내부 로깅 / 메트릭 용도로만 사용한다.
//
// 호출 사이트는 deviceUID 가 빈 문자열인 경우에도 callback 을 호출한다. 호출
// 측 책임으로 deviceUID 검증 후 fallback (deviceCompositeID 사용 또는 skip)
// 처리한다. observe.IncDeviceUIDMissing 카운터가 이미 호출 빈도를 추적하므로
// 별도 panic 이나 에러는 발생시키지 않는다.
//
// SPEC-DEVICE-IDENTITY-001 § M3.
type DeviceStateChangeCallbackV2 = func(agentName, deviceUID, deviceCompositeID string)
