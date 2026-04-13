package lg

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// ---------------------------------------------------------------------------
// LGCNP 디바이스 모델 — 패시브 캡처에서 자동 발견된 디바이스 상태 관리
// ---------------------------------------------------------------------------

// LGCNPDevice 는 LGCNP-01 버스에서 관측된 디바이스이다.
type LGCNPDevice struct {
	Address  string         // "odu" 또는 "81"~"85"
	Label    string         // "outdoor", "indoor-1"~"indoor-5"
	Type     string         // "outdoor" 또는 "indoor"
	Online   bool
	LastSeen time.Time
	Source   string         // "auto" 또는 "config"
	State    *LGCNPDeviceState
}

// LGCNPDeviceState 는 IDU 디바이스의 누적 상태이다.
type LGCNPDeviceState struct {
	SetTemp     *float64 `json:"set_temp,omitempty"`
	RoomTemp    *float64 `json:"room_temp,omitempty"`
	InletTemp   *float64 `json:"inlet_temp,omitempty"`
	OutletTemp  *float64 `json:"outlet_temp,omitempty"`
	OpMode      *int     `json:"op_mode,omitempty"`
	StatusFlags *int     `json:"status_flags,omitempty"`
	CMDCycle    *string  `json:"cmd_cycle,omitempty"`
	DevType     *int     `json:"dev_type,omitempty"`
	DeviceID    *int     `json:"device_id,omitempty"`
}

// LGCNPODUState 는 ODU(실외기)의 누적 상태이다.
type LGCNPODUState struct {
	OutdoorTempA   *float64 `json:"outdoor_temp_a,omitempty"`
	OutdoorTempB   *float64 `json:"outdoor_temp_b,omitempty"`
	CompressorFlag *int     `json:"compressor_flag,omitempty"`
}

// snapshot 은 현재 상태의 복사본을 반환한다.
func (s *LGCNPDeviceState) snapshot() LGCNPDeviceState {
	return *s
}

// snapshot 은 현재 상태의 복사본을 반환한다.
func (s *LGCNPODUState) snapshot() LGCNPODUState {
	return *s
}

// toProperties 는 디바이스 상태를 통합 속성 맵으로 변환한다.
// 속성명은 NASA/LGCP 에이전트와 통일: power, current_temp, target_temp, mode.
func (s *LGCNPDeviceState) toProperties() map[string]any {
	props := make(map[string]any)

	// power: STATUS_FLAGS 에서 추론 (bit0 = 운전 중 추정)
	if s.StatusFlags != nil {
		flags := *s.StatusFlags
		props["power"] = flags != 0
		props["status_flags"] = flags
	}

	// mode: OP_MODE 바이트를 사람이 읽을 수 있는 문자열로 변환
	if s.OpMode != nil {
		props["mode"] = lgcnpDecodeOpMode(*s.OpMode)
		props["op_mode"] = *s.OpMode
	}

	if s.RoomTemp != nil {
		props["current_temp"] = *s.RoomTemp
	}
	if s.SetTemp != nil {
		props["target_temp"] = *s.SetTemp
	}
	if s.InletTemp != nil {
		props["inlet_temp"] = *s.InletTemp
	}
	if s.OutletTemp != nil {
		props["outlet_temp"] = *s.OutletTemp
	}
	if s.CMDCycle != nil {
		props["cmd_cycle"] = *s.CMDCycle
	}
	return props
}

// lgcnpDecodeOpMode 는 LGCNP-01 OP_MODE 바이트를 운전 모드 문자열로 변환한다.
// 프로토콜 분석 보고서 기준: 0x14=냉방 관측, 나머지 모드는 미확정.
// 상위 니블 기반 추정 매핑을 적용하고, 미확정 값은 원시 코드를 표시한다.
func lgcnpDecodeOpMode(raw int) string {
	switch raw {
	case 0x14:
		return "cooling"
	case 0x18:
		return "heating"
	case 0x1C:
		return "auto"
	case 0x24:
		return "dehumidify"
	case 0x34:
		return "fan"
	default:
		return fmt.Sprintf("unknown(0x%02X)", raw)
	}
}

// toProperties 는 ODU 상태를 통합 속성 맵으로 변환한다.
func (s *LGCNPODUState) toProperties() map[string]any {
	props := make(map[string]any)
	if s.OutdoorTempA != nil {
		props["outdoor_temp"] = *s.OutdoorTempA
	}
	if s.OutdoorTempB != nil {
		props["outdoor_temp_b"] = *s.OutdoorTempB
	}
	if s.CompressorFlag != nil {
		props["compressor_flag"] = *s.CompressorFlag
	}
	return props
}

// lgcnpDeviceStateChanged 는 두 IDU 상태를 비교하여 주요 필드가 변경되었는지 판별한다.
func lgcnpDeviceStateChanged(prev, curr LGCNPDeviceState) bool {
	if !ptrF64Eq(prev.SetTemp, curr.SetTemp) {
		return true
	}
	if !ptrF64Eq(prev.RoomTemp, curr.RoomTemp) {
		return true
	}
	if !ptrF64Eq(prev.InletTemp, curr.InletTemp) {
		return true
	}
	if !ptrF64Eq(prev.OutletTemp, curr.OutletTemp) {
		return true
	}
	if !ptrIntEq(prev.OpMode, curr.OpMode) {
		return true
	}
	if !ptrIntEq(prev.StatusFlags, curr.StatusFlags) {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// LGCNPDeviceProvider — device.DeviceProvider 구현
// ---------------------------------------------------------------------------

// LGCNPDeviceProvider 는 LGCNP 에이전트의 device.DeviceProvider 구현이다.
type LGCNPDeviceProvider struct {
	agent *LGCNPAgent
}

// 컴파일 타임 인터페이스 체크
var _ device.DeviceProvider = (*LGCNPDeviceProvider)(nil)

// NewLGCNPDeviceProvider 는 LGCNP 에이전트를 래핑하는 DeviceProvider 를 생성한다.
func NewLGCNPDeviceProvider(a *LGCNPAgent) *LGCNPDeviceProvider {
	return &LGCNPDeviceProvider{agent: a}
}

// Devices 는 에이전트가 관리하는 모든 디바이스를 통합 Device 인터페이스로 반환한다.
func (p *LGCNPDeviceProvider) Devices() []device.Device {
	lgcnpDevices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(lgcnpDevices))
	agentName := p.agent.Name()

	for i := range lgcnpDevices {
		dev := &lgcnpDevices[i]
		info := lgcnpDeviceToInfo(dev)
		result = append(result, adapter.NewLGCNPDevice(agentName, info))
	}
	return result
}

// Device 는 글로벌 ID ("agentName:address") 로 특정 디바이스를 반환한다.
func (p *LGCNPDeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	lgcnpDevices := p.agent.ListDevices()
	for i := range lgcnpDevices {
		dev := &lgcnpDevices[i]
		if dev.Address == addrStr {
			info := lgcnpDeviceToInfo(dev)
			return adapter.NewLGCNPDevice(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// lgcnpDeviceToInfo 는 LGCNPDevice 를 adapter.LGCNPDeviceInfo 로 변환한다.
func lgcnpDeviceToInfo(dev *LGCNPDevice) adapter.LGCNPDeviceInfo {
	info := adapter.LGCNPDeviceInfo{
		Address:    dev.Address,
		Label:      dev.Label,
		DeviceType: dev.Type,
		Online:     dev.Online,
		LastSeen:   dev.LastSeen,
		Source:     dev.Source,
	}
	if dev.State != nil {
		info.Properties = dev.State.toProperties()
	}
	return info
}
