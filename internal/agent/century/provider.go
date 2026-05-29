package century

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// ---------------------------------------------------------------------------
// Hvacr01DeviceProvider — device.DeviceProvider 어댑터 (REQ-CENTURY-015)
//
// 본 어댑터는 *Hvacr01Agent.ListDevices() 의 Icp01DeviceSnapshot 슬라이스를
// 통합 device.Device 인터페이스로 노출한다. Properties 맵의 키 명명은 NASA/LG ICP-01
// 와 정렬된 통합 속성명 (power/mode/fan_speed/target_temp/current_temp) 과 Century
// 전용 속성 (evaporator_temperature_a/evaporator_temperature_b/op_val_1/op_val_2/status_bits) 를 모두 포함한다.
// ---------------------------------------------------------------------------

// Hvacr01DeviceProvider 는 Hvacr01Agent 를 device.DeviceProvider 인터페이스로 래핑한다.
type Hvacr01DeviceProvider struct {
	agent *Hvacr01Agent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*Hvacr01DeviceProvider)(nil)

// NewHvacr01DeviceProvider 는 주어진 Hvacr01Agent 의 DeviceProvider 어댑터를 생성한다.
//
// agent 가 nil 이면 panic 한다 (호출자가 항상 살아있는 agent 를 전달하도록 강제).
func NewHvacr01DeviceProvider(agent *Hvacr01Agent) *Hvacr01DeviceProvider {
	if agent == nil {
		panic("century_hvacr01: NewHvacr01DeviceProvider: agent must not be nil")
	}
	return &Hvacr01DeviceProvider{agent: agent}
}

// Devices 는 모든 등록된 device 의 통합 Device 슬라이스를 반환한다.
func (p *Hvacr01DeviceProvider) Devices() []device.Device {
	snaps := p.agent.ListDevices()
	out := make([]device.Device, 0, len(snaps))
	agentName := p.agent.Name()
	for i := range snaps {
		out = append(out, newHvacr01DeviceAdapter(agentName, snaps[i]))
	}
	return out
}

// Device 는 글로벌 ID ("agentName:<sub_dev_id_hex>") 로 특정 디바이스를 찾는다.
//
// 매칭 규칙:
//   - 접두사 "<agentName>:" 가 정확히 일치해야 한다.
//   - 접미사의 hex 문자열 ("3b" 등 1~2자리) 을 byte 로 파싱하여 SubDevID 와 비교.
//
// 매칭 실패 시 device.ErrDeviceNotFound 를 반환한다.
func (p *Hvacr01DeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if !strings.HasPrefix(id, prefix) {
		return nil, device.ErrDeviceNotFound
	}
	suffix := id[len(prefix):]
	if suffix == "" {
		return nil, device.ErrDeviceNotFound
	}
	subDev, err := parseSubDevHex(suffix)
	if err != nil {
		return nil, device.ErrDeviceNotFound
	}
	snaps := p.agent.ListDevices()
	for i := range snaps {
		if snaps[i].SubDevID == subDev {
			return newHvacr01DeviceAdapter(agentName, snaps[i]), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// parseSubDevHex 는 1~2자리 hex 문자열 ("3b", "3B", "0x3B") 을 byte 로 파싱한다.
func parseSubDevHex(s string) (byte, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if s == "" || len(s) > 2 {
		return 0, fmt.Errorf("invalid sub_dev_id hex: %q", s)
	}
	var v byte
	if _, err := fmt.Sscanf(s, "%x", &v); err != nil {
		return 0, fmt.Errorf("invalid sub_dev_id hex: %q", s)
	}
	return v, nil
}

// ---------------------------------------------------------------------------
// hvacr01DeviceAdapter — device.Device 구현 (in-package, 추가 의존성 없음)
// ---------------------------------------------------------------------------

// hvacr01DeviceAdapter 는 Icp01DeviceSnapshot 을 device.Device 로 노출한다.
type hvacr01DeviceAdapter struct {
	agentName string
	snap      Icp01DeviceSnapshot
}

// Compile-time interface check.
var _ device.Device = (*hvacr01DeviceAdapter)(nil)

// newHvacr01DeviceAdapter 는 어댑터 인스턴스를 생성한다.
func newHvacr01DeviceAdapter(agentName string, snap Icp01DeviceSnapshot) *hvacr01DeviceAdapter {
	return &hvacr01DeviceAdapter{agentName: agentName, snap: snap}
}

// ID returns the globally unique UUID v4 for this device.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1, Breaking):
// ID() now returns the UUID (same value as UID()). The legacy composite
// key ("agent:sub_dev_id_hex") format has been fully removed.
func (a *hvacr01DeviceAdapter) ID() string {
	localID := fmt.Sprintf("0x%02X", a.snap.SubDevID)
	return adapter.ResolveAdapterUID(a.agentName, localID)
}

// UID 는 (agentName, "0xXX") 의 글로벌 UUID v4 를 반환한다.
//
// localID 는 Hvacr01Agent 의 emit 경로에서 ResolveDeviceID 호출 시 사용하는
// unitID 형식 ("0x%02X" — uppercase hex with 0x prefix) 과 정확히 일치한다.
// 이를 통해 emit payload 의 device_id 와 UID() 의 결과가 동일한 UUID 로
// 보장된다.
//
// SPEC-DEVICE-IDENTITY-001 § M1. Phase D (v1.0): ID() == UID().
func (a *hvacr01DeviceAdapter) UID() string {
	localID := fmt.Sprintf("0x%02X", a.snap.SubDevID)
	return adapter.ResolveAdapterUID(a.agentName, localID)
}

// Name 은 사용자에게 표시되는 이름을 반환한다 (snap.Label, 없으면 기본 이름).
func (a *hvacr01DeviceAdapter) Name() string {
	if a.snap.Label != "" {
		return a.snap.Label
	}
	return fmt.Sprintf("Century indoor 0x%02X", a.snap.SubDevID)
}

// Type 은 Century device 가 항상 indoor unit 인 점을 반영한다 (v0.1.0 범위).
func (a *hvacr01DeviceAdapter) Type() device.DeviceType {
	return device.DeviceTypeIndoor
}

// Protocol 은 "century_icp01" 를 반환한다 (Century ICP-01 wire protocol).
func (a *hvacr01DeviceAdapter) Protocol() string {
	return "century_icp01"
}

// AgentName 은 owning 에이전트의 이름을 반환한다.
func (a *hvacr01DeviceAdapter) AgentName() string {
	return a.agentName
}

// Online 은 device 의 현재 online 여부를 반환한다.
func (a *hvacr01DeviceAdapter) Online() bool {
	return a.snap.Online
}

// LastSeen 은 device 의 마지막 통신 시각을 반환한다.
func (a *hvacr01DeviceAdapter) LastSeen() time.Time {
	return a.snap.LastSeen
}

// State 는 device 의 현재 상태 스냅샷을 반환한다.
func (a *hvacr01DeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.snap.Online,
		LastSeen:   a.snap.LastSeen,
		ErrorCount: a.snap.ErrorCount,
		Properties: a.buildProperties(),
	}
}

// Metadata 는 사용자 정의 메타데이터를 반환한다. v0.1.0 은 빈 값.
func (a *hvacr01DeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

// Source 는 device 의 등록 출처를 반환한다 ("auto" 또는 "config").
func (a *hvacr01DeviceAdapter) Source() string {
	if a.snap.Source != "" {
		return a.snap.Source
	}
	return "auto"
}

// Capabilities 는 device 가 제공하는 capability 목록을 반환한다.
//
// Century 는 패시브 캡처 전용이므로 "passive-monitor" 만 노출한다 — LG ICP-01 과 정렬.
func (a *hvacr01DeviceAdapter) Capabilities() []string {
	return []string{"passive-monitor"}
}

// Execute 는 Century 가 패시브 전용이므로 항상 device.ErrNotControllable 을 반환한다.
//
// (REQ-CENTURY-017, AC-B9: transport.Write 절대 금지)
func (a *hvacr01DeviceAdapter) Execute(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, device.ErrNotControllable
}

// buildProperties 는 Icp01DeviceState 를 통합 + Century 전용 속성 맵으로 변환한다.
//
// 통합 속성 (NASA/LG ICP-01 정렬): power, mode, fan_speed, target_temp, current_temp
// Century 전용: temp_evap_a, temp_evap_b, op_val_1, op_val_2, status_bits
//
// power 는 가장 최근 mode (reg 0x02 의 Mode 또는 reg 0x04 WRITE 의 ModeCmd) 가
// "off" 가 아니면 true 로 추정한다. mode 정보가 없으면 power 는 노출하지 않는다.
func (a *hvacr01DeviceAdapter) buildProperties() map[string]any {
	props := make(map[string]any)
	if a.snap.State == nil {
		return props
	}
	st := a.snap.State

	// Reg 0x02 — 운전 모드/풍량/설정 온도/현재 온도 (2026-05-29: current_temp 의 원천이
	// Reg04 TempAC → Reg02 CurrentTempC 로 정정됨).
	if st.Reg02 != nil {
		modeStr := st.Reg02.Mode.Value
		// SPEC-DEVICE-IDENTITY-001 후속: hvac 통일 ID (int) 로 emit
		// (LGCP / NASA / LGAP 와 일관). 사람이 읽는 형태는 web UI 가 `hvac.ModeName(id)` 로 변환.
		props["mode"] = hvac.ModeFromName(modeStr)
		props["power"] = modeStr != "off"
		// fan_speed: Century 의 Fan.Value 는 raw 프로토콜 step 값 (예: 17) 이며
		// hvac.FanSpeedFromName 의 canonical ID (0-6) 와 의미가 다르다. 별도
		// 매핑 테이블 부재로 raw 값 그대로 노출 (TODO: 향후 mapping 확정 시 통일).
		props["fan_speed"] = int(st.Reg02.Fan.Value)
		props["target_temperature"] = float64(st.Reg02.SetpointC.Value)
		// current_temp: Reg02 data[7..8] = 현재 실내 온도 (Confirmed, 2026-05-29 실측 검증).
		props["current_temperature"] = float64(st.Reg02.CurrentTempC.Value)
	}

	// Reg 0x03 — 증발기 냉매 배관 온도 (Century 전용).
	if st.Reg03 != nil {
		props["evaporator_temperature_a"] = float64(st.Reg03.EvaporatorTemperatureA.Value)
		props["evaporator_temperature_b"] = float64(st.Reg03.EvaporatorTemperatureB.Value)
	}

	// Reg 0x04 read — 운전 데이터. reg04_word_10 (이전 TempAC) 의 실제 의미 미확정.
	if st.Reg04Read != nil {
		props["op_val_1"] = uint16(st.Reg04Read.OpVal1.Value)
		props["op_val_2"] = uint16(st.Reg04Read.OpVal2.Value)
		props["status_bits"] = uint8(st.Reg04Read.StatusBits.Value)
	}

	return props
}
