package century

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// ---------------------------------------------------------------------------
// CenturyDeviceProvider — device.DeviceProvider 어댑터 (REQ-CENTURY-015)
//
// 본 어댑터는 *CenturyAgent.ListDevices() 의 CenturyDeviceSnapshot 슬라이스를
// 통합 device.Device 인터페이스로 노출한다. Properties 맵의 키 명명은 NASA/LGCNP
// 와 정렬된 통합 속성명 (power/mode/fan_speed/target_temp/current_temp) 과 Century
// 전용 속성 (temp_evap_a/temp_evap_b/op_val_1/op_val_2/status_bits) 를 모두 포함한다.
// ---------------------------------------------------------------------------

// CenturyDeviceProvider 는 CenturyAgent 를 device.DeviceProvider 인터페이스로 래핑한다.
type CenturyDeviceProvider struct {
	agent *CenturyAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*CenturyDeviceProvider)(nil)

// NewCenturyDeviceProvider 는 주어진 CenturyAgent 의 DeviceProvider 어댑터를 생성한다.
//
// agent 가 nil 이면 panic 한다 (호출자가 항상 살아있는 agent 를 전달하도록 강제).
func NewCenturyDeviceProvider(agent *CenturyAgent) *CenturyDeviceProvider {
	if agent == nil {
		panic("century: NewCenturyDeviceProvider: agent must not be nil")
	}
	return &CenturyDeviceProvider{agent: agent}
}

// Devices 는 모든 등록된 device 의 통합 Device 슬라이스를 반환한다.
func (p *CenturyDeviceProvider) Devices() []device.Device {
	snaps := p.agent.ListDevices()
	out := make([]device.Device, 0, len(snaps))
	agentName := p.agent.Name()
	for i := range snaps {
		out = append(out, newCenturyDeviceAdapter(agentName, snaps[i]))
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
func (p *CenturyDeviceProvider) Device(id string) (device.Device, error) {
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
			return newCenturyDeviceAdapter(agentName, snaps[i]), nil
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
// centuryDeviceAdapter — device.Device 구현 (in-package, 추가 의존성 없음)
// ---------------------------------------------------------------------------

// centuryDeviceAdapter 는 CenturyDeviceSnapshot 을 device.Device 로 노출한다.
type centuryDeviceAdapter struct {
	agentName string
	snap      CenturyDeviceSnapshot
}

// Compile-time interface check.
var _ device.Device = (*centuryDeviceAdapter)(nil)

// newCenturyDeviceAdapter 는 어댑터 인스턴스를 생성한다.
func newCenturyDeviceAdapter(agentName string, snap CenturyDeviceSnapshot) *centuryDeviceAdapter {
	return &centuryDeviceAdapter{agentName: agentName, snap: snap}
}

// ID 는 글로벌 device ID 를 반환한다 ("<agent>:<sub_dev_id_hex>").
func (a *centuryDeviceAdapter) ID() string {
	return fmt.Sprintf("%s:%02x", a.agentName, a.snap.SubDevID)
}

// Name 은 사용자에게 표시되는 이름을 반환한다 (snap.Label, 없으면 기본 이름).
func (a *centuryDeviceAdapter) Name() string {
	if a.snap.Label != "" {
		return a.snap.Label
	}
	return fmt.Sprintf("Century indoor 0x%02X", a.snap.SubDevID)
}

// Type 은 Century device 가 항상 indoor unit 인 점을 반영한다 (v0.1.0 범위).
func (a *centuryDeviceAdapter) Type() device.DeviceType {
	return device.DeviceTypeIndoor
}

// Protocol 은 "century-hvac" 를 반환한다.
func (a *centuryDeviceAdapter) Protocol() string {
	return "century-hvac"
}

// AgentName 은 owning 에이전트의 이름을 반환한다.
func (a *centuryDeviceAdapter) AgentName() string {
	return a.agentName
}

// Online 은 device 의 현재 online 여부를 반환한다.
func (a *centuryDeviceAdapter) Online() bool {
	return a.snap.Online
}

// LastSeen 은 device 의 마지막 통신 시각을 반환한다.
func (a *centuryDeviceAdapter) LastSeen() time.Time {
	return a.snap.LastSeen
}

// State 는 device 의 현재 상태 스냅샷을 반환한다.
func (a *centuryDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.snap.Online,
		LastSeen:   a.snap.LastSeen,
		ErrorCount: a.snap.ErrorCount,
		Properties: a.buildProperties(),
	}
}

// Metadata 는 사용자 정의 메타데이터를 반환한다. v0.1.0 은 빈 값.
func (a *centuryDeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

// Source 는 device 의 등록 출처를 반환한다 ("auto" 또는 "config").
func (a *centuryDeviceAdapter) Source() string {
	if a.snap.Source != "" {
		return a.snap.Source
	}
	return "auto"
}

// Capabilities 는 device 가 제공하는 capability 목록을 반환한다.
//
// Century 는 패시브 캡처 전용이므로 "passive-monitor" 만 노출한다 — LGCNP 와 정렬.
func (a *centuryDeviceAdapter) Capabilities() []string {
	return []string{"passive-monitor"}
}

// Execute 는 Century 가 패시브 전용이므로 항상 device.ErrNotControllable 을 반환한다.
//
// (REQ-CENTURY-017, AC-B9: transport.Write 절대 금지)
func (a *centuryDeviceAdapter) Execute(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, device.ErrNotControllable
}

// buildProperties 는 CenturyDeviceState 를 통합 + Century 전용 속성 맵으로 변환한다.
//
// 통합 속성 (NASA/LGCNP 정렬): power, mode, fan_speed, target_temp, current_temp
// Century 전용: temp_evap_a, temp_evap_b, op_val_1, op_val_2, status_bits
//
// power 는 가장 최근 mode (reg 0x02 의 Mode 또는 reg 0x04 WRITE 의 ModeCmd) 가
// "off" 가 아니면 true 로 추정한다. mode 정보가 없으면 power 는 노출하지 않는다.
func (a *centuryDeviceAdapter) buildProperties() map[string]any {
	props := make(map[string]any)
	if a.snap.State == nil {
		return props
	}
	st := a.snap.State

	// Reg 0x02 — 운전 모드/풍량/설정 온도.
	if st.Reg02 != nil {
		modeStr := st.Reg02.Mode.Value
		props["mode"] = modeStr
		props["power"] = modeStr != "off"
		// fan_speed: Century 는 정수형 step (CAP-3 17 관측). 통합 속성으로 노출.
		props["fan_speed"] = int(st.Reg02.Fan.Value)
		props["target_temperature"] = float64(st.Reg02.SetpointC.Value)
	}

	// Reg 0x03 — 증발기 냉매 배관 온도 (Century 전용).
	if st.Reg03 != nil {
		props["temp_evap_a"] = float64(st.Reg03.TempEvapAC.Value)
		props["temp_evap_b"] = float64(st.Reg03.TempEvapBC.Value)
	}

	// Reg 0x04 read — 운전 데이터.
	if st.Reg04Read != nil {
		props["op_val_1"] = uint16(st.Reg04Read.OpVal1.Value)
		props["op_val_2"] = uint16(st.Reg04Read.OpVal2.Value)
		props["status_bits"] = uint8(st.Reg04Read.StatusBits.Value)
		// current_temp: temp_A_c 가 실내/리턴에어 온도로 추정 (Inferred).
		props["current_temperature"] = float64(st.Reg04Read.TempAC.Value)
	}

	return props
}
