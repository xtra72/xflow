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
	FanByte     *int     `json:"fan_byte,omitempty"`
	OpMode      *int     `json:"op_mode,omitempty"`
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
// 속성명은 NASA/LGCP 에이전트와 통일: power, current_temp, target_temp, mode, fan_speed.
func (s *LGCNPDeviceState) toProperties() map[string]any {
	props := make(map[string]any)

	// power: OP_MODE bit5로 판별. bit5=0 → ON, bit5=1 → OFF.
	// 실측: 0x14(bit5=0)=ON, 0x24(bit5=1)=OFF.
	powerOn := true
	if s.OpMode != nil {
		powerOn = *s.OpMode&0x20 == 0
	}
	props["power"] = powerOn

	// OFF 시 운전 모드/풍량 표시하지 않음
	if powerOn {
		if s.OpMode != nil {
			props["mode"] = lgcnpDecodeOpMode(*s.OpMode)
		}
		if s.FanByte != nil {
			props["fan_speed"] = lgcnpDecodeFanSpeed(*s.FanByte)
		}
	}

	if s.SetTemp != nil {
		props["target_temp"] = *s.SetTemp
	}
	if s.RoomTemp != nil {
		props["current_temp"] = *s.RoomTemp
	}
	if s.InletTemp != nil {
		props["inlet_temp"] = *s.InletTemp
	}
	if s.OutletTemp != nil {
		props["outlet_temp"] = *s.OutletTemp
	}
	return props
}

// lgcnpDecodeOpMode 는 LGCNP-01 OP_MODE 바이트를 운전 모드 문자열로 변환한다.
// 하위 니블이 LGAP 모드 코드와 일치: 0=냉방, 1=제습, 2=송풍, 3=자동, 4=난방.
// 실측 확인: 0x14 → 하위 니블 4 → 난방.
func lgcnpDecodeOpMode(raw int) string {
	switch raw & 0x0F {
	case 0:
		return "cooling"
	case 1:
		return "dehumidify"
	case 2:
		return "fan"
	case 3:
		return "auto"
	case 4:
		return "heating"
	default:
		return fmt.Sprintf("unknown(0x%02X)", raw)
	}
}

// lgcnpDecodeFanSpeed 는 LGCNP-01 b[30] 바이트를 풍량 문자열로 변환한다.
// 실측 확인: 0x54(bit6=1)=미풍, 0x14(bit6=0)=약풍.
// bit6 기반 판별. 중/강/자동은 추가 관측 필요.
func lgcnpDecodeFanSpeed(raw int) string {
	switch raw {
	case 0x54:
		return "quiet"
	case 0x14:
		return "low"
	default:
		return "auto"
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
