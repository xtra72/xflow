package modbusserver

import (
	"fmt"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// 설정 구조체
// ---------------------------------------------------------------------------

// 트랜스포트 디스크리미네이터 상수.
const (
	// TransportTCP 는 MODBUS/TCP(MBAP) 서버 트랜스포트이다(기본값).
	TransportTCP = "tcp"
	// TransportRTU 는 MODBUS RTU(시리얼 슬레이브) 서버 트랜스포트이다.
	TransportRTU = "rtu"
)

// 역할(role) 상수 — 공유 레지스터 맵 기능(M3).
const (
	// RoleMain 은 자체 RegisterMap 을 소유하는 주 서버이다(기본값).
	RoleMain = "main"
	// RoleSub 는 SharedFrom 이 가리키는 주 서버의 RegisterMap 을 라이브 공유하는 서브 서버이다.
	RoleSub = "sub"
)

// ModbusServerConfig 는 MODBUS 서버 에이전트의 설정을 나타낸다.
type ModbusServerConfig struct {
	Transport      string            // "tcp" | "rtu" (기본값 "tcp")
	Serial         SerialConfig      // Transport == "rtu" 일 때만 유효한 시리얼 파라미터
	ListenAddress  string            // 리슨 주소 (기본값 "0.0.0.0", TCP 전용)
	ListenPort     int               // 리슨 포트 (기본값 502, 범위 1-65535, TCP 전용)
	UnitID         byte              // 유닛 ID (기본값 1, 범위 0-247) — 하위 호환용, Devices 가 없을 때 사용
	MaxConnections int               // 최대 연결 수 (기본값 10, > 0, TCP 전용)
	IdleTimeout    time.Duration     // 유휴 타임아웃 (기본값 60s, TCP 전용)
	MsgChannelSize int               // 메시지 채널 버퍼 크기 (기본값 256)
	RegisterMap    RegisterMapConfig // 하위 호환용, Devices 가 없을 때 사용
	Devices        []DeviceConfig    // 다중 디바이스 설정 (멀티-디바이스 지원)
	Role           string            // "main" | "sub" (기본값 "main") — 공유 레지스터 맵(M3)
	SharedFrom     string            // Role == "sub" 일 때 필수: 공유할 주 서버 에이전트 ID
	NotifyOnWrite  bool              // 외부 통신(원격 마스터 와이어 쓰기)로 레지스터가 변경될 때만 register_change 알림 발행 (기본값 false, opt-in)
	LogFrames      bool              // TX/RX 프레임 요약 로그 활성 여부 (기본값 false, Configure 로 라이브 갱신)
	LogRawFrames   bool              // 프레임 로그에 전체 ADU hex 포함 여부 (LogFrames 가 켜져 있을 때만 의미, 기본값 false, 라이브 갱신)
}

// SerialConfig 는 RTU 트랜스포트의 시리얼 포트 파라미터이다.
// 클라이언트 에이전트(internal/agent/modbus)의 SerialConfig 와 동일한 옵션 키를 사용한다.
// transport == "rtu" 일 때 Transport.Options 에서 파싱·검증된다.
type SerialConfig struct {
	Port     string // 시리얼 포트 경로 (필수, 예: /dev/ttyUSB0)
	BaudRate int    // 기본값 9600
	DataBits int    // 기본값 8
	StopBits int    // 기본값 1 (1 또는 2)
	Parity   string // "none" | "even" | "odd" (기본값 "none")
}

// DeviceConfig 는 단일 가상 디바이스의 설정을 나타낸다.
type DeviceConfig struct {
	UnitID       byte              // 유닛 ID (범위 1-247)
	Name         string            // 디바이스 이름 (선택, 로깅/식별용)
	RegisterMap  RegisterMapConfig // 디바이스별 레지스터 맵
	RegisterDefs []any             // 디바이스별 레지스터 정의 (Bridge Adapter 매핑용)
}

// RegisterMapConfig 는 레지스터 맵의 설정을 나타낸다.
// 각 영역은 하나 이상의 비연속 세그먼트를 가질 수 있다.
type RegisterMapConfig struct {
	Coils            []*RegisterAreaConfig // 코일 영역 (FC01/FC05/FC15)
	DiscreteInputs   []*RegisterAreaConfig // 이산 입력 영역 (FC02)
	HoldingRegisters []*RegisterAreaConfig // 보유 레지스터 영역 (FC03/FC06/FC16)
	InputRegisters   []*RegisterAreaConfig // 입력 레지스터 영역 (FC04)
}

// RegisterAreaConfig 는 단일 레지스터 영역의 설정을 나타낸다.
type RegisterAreaConfig struct {
	StartAddress  uint16                // 디바이스 주소 (config 키 "address", 하위 호환 "start_address")
	Count         uint16                // 레지스터 수 (필수, > 0)
	InitialValues []any                 // 초기값 (선택); 코일/DI 는 bool, 레지스터는 숫자 (로컬 세그먼트 전용)
	DataType      string                // 영역 기본 데이터 타입 (기본: "uint16", 로컬 세그먼트 전용)
	TypeMap       []modbus.TypeMapEntry // 주소별 타입 오버라이드 (선택, 로컬 세그먼트 전용)

	// 공유 세그먼트(intra-server): shared_address 가 있으면 IsShared=true 이며,
	// 디바이스 주소 [StartAddress, StartAddress+Count) 는 unit_id 0(공유 컨테이너)의
	// 같은 영역 [SharedAddress, SharedAddress+Count) 로 앨리어싱된다. data_type/type
	// 오버레이는 컨테이너 맵에서 상속하므로 공유 세그먼트에는 요구하지 않는다.
	IsShared      bool   // shared_address 존재 여부 (로컬 vs 공유 판별자)
	SharedAddress uint16 // 공유 컨테이너(unit_id 0)에서의 시작 주소 (IsShared 일 때만 유효)

	Description string // 세그먼트 설명 (선택, 메타데이터 전용 — 와이어 서빙에 영향 없음)
}

// ---------------------------------------------------------------------------
// 설정 파싱
// ---------------------------------------------------------------------------

// parseModbusServerConfig 는 Transport.Options 맵에서 ModbusServerConfig 를 파싱한다.
func parseModbusServerConfig(opts map[string]any) (ModbusServerConfig, error) {
	cfg := ModbusServerConfig{
		Transport:      TransportTCP,
		ListenAddress:  "0.0.0.0",
		ListenPort:     502,
		UnitID:         1,
		MaxConnections: 10,
		IdleTimeout:    60 * time.Second,
		MsgChannelSize: 256,
		Role:           RoleMain,
	}

	// transport (선택, 기본 "tcp" — 생략 시 기존 TCP 동작 보존)
	if v, ok := opts["transport"]; ok {
		s, _ := v.(string)
		switch s {
		case TransportTCP, "":
			cfg.Transport = TransportTCP
		case TransportRTU:
			cfg.Transport = TransportRTU
		default:
			return ModbusServerConfig{}, fmt.Errorf(
				"modbus-server: transport %q must be %q or %q", s, TransportTCP, TransportRTU)
		}
	}

	// RTU 시리얼 파라미터 (transport == "rtu" 일 때 파싱·검증)
	if cfg.Transport == TransportRTU {
		sc, err := parseServerSerialConfig(opts)
		if err != nil {
			return ModbusServerConfig{}, err
		}
		cfg.Serial = sc
	}

	// role (선택, 기본 "main") — 공유 레지스터 맵(M3)
	if v, ok := opts["role"]; ok {
		s, _ := v.(string)
		switch s {
		case RoleMain, "":
			cfg.Role = RoleMain
		case RoleSub:
			cfg.Role = RoleSub
		default:
			return ModbusServerConfig{}, fmt.Errorf(
				"modbus-server: role %q must be %q or %q", s, RoleMain, RoleSub)
		}
	}

	// shared_from (role == "sub" 일 때 필수)
	if v, ok := opts["shared_from"]; ok {
		cfg.SharedFrom, _ = v.(string)
	}
	if cfg.Role == RoleSub && cfg.SharedFrom == "" {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: role=sub requires shared_from (main agent id): %w", ErrInvalidSharedConfig)
	}
	if cfg.Role == RoleMain {
		// main 은 shared_from 을 무시한다.
		cfg.SharedFrom = ""
	}

	// listen_address
	if v, ok := opts["listen_address"]; ok {
		if s, ok := v.(string); ok {
			cfg.ListenAddress = s
		}
	}

	// listen_port
	if v, ok := opts["listen_port"]; ok {
		cfg.ListenPort = toInt(v)
	}
	if cfg.ListenPort < 0 || cfg.ListenPort > 65535 {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: listen_port must be 0-65535 (got %d)", cfg.ListenPort)
	}

	// unit_id (하위 호환용)
	if v, ok := opts["unit_id"]; ok {
		id := toInt(v)
		if id < 0 || id > 247 {
			return ModbusServerConfig{}, fmt.Errorf(
				"modbus-server: unit_id must be 0-247 (got %d)", id)
		}
		cfg.UnitID = byte(id)
	}

	// max_connections
	if v, ok := opts["max_connections"]; ok {
		cfg.MaxConnections = toInt(v)
	}
	if cfg.MaxConnections < 1 {
		return ModbusServerConfig{}, fmt.Errorf(
			"modbus-server: max_connections must be > 0 (got %d)", cfg.MaxConnections)
	}

	// idle_timeout
	if v, ok := opts["idle_timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return ModbusServerConfig{}, fmt.Errorf("modbus-server: invalid idle_timeout: %w", err)
			}
			cfg.IdleTimeout = d
		}
	}

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// notify_on_write (선택, 기본 false — opt-in). true 이면 외부 통신(원격 마스터의
	// 와이어 쓰기)으로 레지스터가 변경될 때만 register_change 알림을 발행한다. 플로우 입력
	// 포트(set_*/bulk_write)로 인한 변경 알림(sendChangeEvent)에는 영향을 주지 않는다.
	if v, ok := opts["notify_on_write"]; ok {
		if b, ok := v.(bool); ok {
			cfg.NotifyOnWrite = b
		}
	}

	// log_frames (선택, 기본 false — 라이브 갱신). true 이면 TX/RX 프레임 요약을 INFO 로 남긴다.
	if v, ok := opts["log_frames"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogFrames = b
		}
	}

	// log_raw_frames (선택, 기본 false — 라이브 갱신). true 이고 log_frames 도 true 일 때만
	// 프레임 로그에 전체 ADU hex 를 포함한다(log_frames 가 꺼져 있으면 무의미).
	if v, ok := opts["log_raw_frames"]; ok {
		if b, ok := v.(bool); ok {
			cfg.LogRawFrames = b
		}
	}

	// ---------------------------------------------------------------
	// devices (멀티-디바이스) 또는 register_map (하위 호환)
	// ---------------------------------------------------------------
	if devicesRaw, ok := opts["devices"]; ok {
		// 멀티-디바이스 설정
		devices, err := parseDevicesConfig(devicesRaw)
		if err != nil {
			return ModbusServerConfig{}, err
		}
		cfg.Devices = devices
	} else if rmRaw, ok := opts["register_map"]; ok {
		// 하위 호환: unit_id + register_map → 단일 DeviceConfig 로 변환
		rmMap, ok := rmRaw.(map[string]any)
		if !ok {
			return ModbusServerConfig{}, fmt.Errorf(
				"modbus-server: register_map must be a map: %w", ErrInvalidRegisterMap)
		}
		rmCfg, err := parseRegisterMapConfig(rmMap)
		if err != nil {
			return ModbusServerConfig{}, err
		}
		cfg.RegisterMap = rmCfg
		// 최상위 register_defs 를 단일 디바이스에 상속 (하위 호환)
		var topDefs []any
		if rawDefs, ok := opts["register_defs"]; ok {
			if defs, ok := rawDefs.([]any); ok {
				topDefs = defs
			}
		}
		cfg.Devices = []DeviceConfig{
			{
				UnitID:       cfg.UnitID,
				Name:         "",
				RegisterMap:  rmCfg,
				RegisterDefs: topDefs,
			},
		}
	} else {
		// devices/register_map 둘 다 없으면 zero-device 서버로 구성한다.
		// role=sub(공유 상속) 뿐 아니라 role=main 도 허용한다: 디바이스는 생성 이후
		// config 업데이트(device 탭 → PUT /agents/{id}/config)로 추가되므로 생성 폼은
		// 더 이상 devices 를 공급하지 않는다. cfg.Devices 는 빈 채로 두고,
		// NewModbusServerAgent 가 NewEmptyDeviceManager 로 빈 서빙 집합을 구성한다
		// (어떤 unit_id 요청도 디바이스를 못 찾을 뿐 panic 없음).
	}

	// Devices 유효성 검증: 자체 디바이스를 가진 경우에만 검증한다.
	// (main-상속 서브는 이 시점에 디바이스가 없으며 Start 에서 채워진다.)
	if len(cfg.Devices) > 0 {
		if err := validateDevices(cfg.Devices); err != nil {
			return ModbusServerConfig{}, err
		}
	}

	return cfg, nil
}

// parseDevicesConfig 는 devices 배열을 파싱하여 []DeviceConfig 를 반환한다.
func parseDevicesConfig(raw any) ([]DeviceConfig, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: devices must be an array: %w", ErrInvalidDeviceConfig)
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("modbus-server: devices must have at least one device: %w", ErrInvalidDeviceConfig)
	}

	devices := make([]DeviceConfig, 0, len(arr))
	for i, item := range arr {
		devMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modbus-server: devices[%d] must be a map: %w", i, ErrInvalidDeviceConfig)
		}

		var dev DeviceConfig

		// unit_id (필수). 0 은 공유 컨테이너(와이어 미서빙), 1-247 은 서빙 디바이스.
		if v, ok := devMap["unit_id"]; ok {
			id := toInt(v)
			if id < 0 || id > 247 {
				return nil, fmt.Errorf(
					"modbus-server: devices[%d].unit_id must be 0-247 (got %d)", i, id)
			}
			dev.UnitID = byte(id)
		} else {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].unit_id is required", i)
		}

		// name (선택)
		if v, ok := devMap["name"]; ok {
			if s, ok := v.(string); ok {
				dev.Name = s
			}
		}

		// register_map (필수)
		rmRaw, ok := devMap["register_map"]
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].register_map is required: %w", i, ErrInvalidRegisterMap)
		}
		rmMap, ok := rmRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: devices[%d].register_map must be a map: %w", i, ErrInvalidRegisterMap)
		}
		rmCfg, err := parseRegisterMapConfig(rmMap)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: devices[%d]: %w", i, err)
		}
		dev.RegisterMap = rmCfg

		// register_defs (선택: 디바이스별 레지스터 정의)
		if rawDefs, ok := devMap["register_defs"]; ok {
			if defs, ok := rawDefs.([]any); ok {
				dev.RegisterDefs = defs
			}
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

// areaNames 는 register_map 의 4개 표준 영역 이름이다.
var areaNames = []string{"coils", "discrete_inputs", "holding_registers", "input_registers"}

// areaSegments 는 RegisterMapConfig 에서 영역 이름에 해당하는 세그먼트 슬라이스를 반환한다.
func areaSegments(cfg RegisterMapConfig, area string) []*RegisterAreaConfig {
	switch area {
	case "coils":
		return cfg.Coils
	case "discrete_inputs":
		return cfg.DiscreteInputs
	case "holding_registers":
		return cfg.HoldingRegisters
	case "input_registers":
		return cfg.InputRegisters
	default:
		return nil
	}
}

// hasSharedSegment 는 register_map 에 공유 세그먼트가 하나라도 있으면 true 를 반환한다.
func hasSharedSegment(cfg RegisterMapConfig) bool {
	for _, area := range areaNames {
		for _, seg := range areaSegments(cfg, area) {
			if seg.IsShared {
				return true
			}
		}
	}
	return false
}

// rangeWithinAnySegment 는 [start, start+count) 가 segs 중 하나의 범위에 완전히 포함되면 true.
func rangeWithinAnySegment(segs []*RegisterAreaConfig, start, count uint16) bool {
	end := uint32(start) + uint32(count)
	for _, s := range segs {
		if uint32(start) >= uint32(s.StartAddress) && end <= uint32(s.StartAddress)+uint32(s.Count) {
			return true
		}
	}
	return false
}

// validateDevices 는 디바이스 목록의 유효성을 검증한다.
//   - Unit ID 범위: 0(공유 컨테이너) 또는 1-247(서빙), 중복 불가
//   - 서빙 디바이스(1-247) 최소 1개 필요
//   - 공유 세그먼트(shared_address)는 컨테이너(unit_id 0)가 존재해야 하며, 컨테이너의
//     같은 영역 선언 범위 안에 있어야 한다
//   - unit_id 0 의 세그먼트는 모두 로컬이어야 한다(shared_address 금지)
func validateDevices(devices []DeviceConfig) error {
	if len(devices) == 0 {
		return fmt.Errorf("modbus-server: at least one device is required: %w", ErrInvalidDeviceConfig)
	}

	seen := make(map[byte]bool, len(devices))
	var container *DeviceConfig
	servedCount := 0
	for i := range devices {
		dev := &devices[i]
		if dev.UnitID > 247 {
			return fmt.Errorf(
				"modbus-server: devices[%d].unit_id must be 0-247 (got %d)", i, dev.UnitID)
		}
		if seen[dev.UnitID] {
			return fmt.Errorf(
				"modbus-server: duplicate unit_id %d in devices: %w", dev.UnitID, ErrDuplicateUnitID)
		}
		seen[dev.UnitID] = true

		if dev.UnitID == 0 {
			container = dev
			// 컨테이너 세그먼트는 모두 로컬이어야 한다.
			if hasSharedSegment(dev.RegisterMap) {
				return fmt.Errorf(
					"modbus-server: unit_id 0 (shared container) segments must be local (no shared_address): %w",
					ErrSharedUnderContainer)
			}
		} else {
			servedCount++
		}
	}

	if servedCount == 0 {
		return fmt.Errorf(
			"modbus-server: at least one served device (unit_id 1-247) is required: %w", ErrInvalidDeviceConfig)
	}

	// 공유 세그먼트 검증: 컨테이너 존재 + 범위 포함.
	for i := range devices {
		dev := &devices[i]
		if dev.UnitID == 0 {
			continue
		}
		for _, area := range areaNames {
			for _, seg := range areaSegments(dev.RegisterMap, area) {
				if !seg.IsShared {
					continue
				}
				if container == nil {
					return fmt.Errorf(
						"modbus-server: devices unit_id %d %s has a shared segment but no unit_id 0 container exists: %w",
						dev.UnitID, area, ErrSharedMapMissing)
				}
				if !rangeWithinAnySegment(areaSegments(container.RegisterMap, area), seg.SharedAddress, seg.Count) {
					return fmt.Errorf(
						"modbus-server: unit_id %d %s shared range [%d,%d) is out of container %s bounds: %w",
						dev.UnitID, area, seg.SharedAddress, uint32(seg.SharedAddress)+uint32(seg.Count),
						area, ErrSharedRangeOutOfBounds)
				}
			}
		}
	}

	return nil
}

// parseRegisterMapConfig 는 레지스터 맵 설정 맵을 RegisterMapConfig 로 파싱한다.
func parseRegisterMapConfig(m map[string]any) (RegisterMapConfig, error) {
	var cfg RegisterMapConfig
	hasArea := false

	// coils
	if v, ok := m["coils"]; ok {
		areas, err := parseAreaSegments(v, "coils")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.Coils = areas
		hasArea = true
	}

	// discrete_inputs
	if v, ok := m["discrete_inputs"]; ok {
		areas, err := parseAreaSegments(v, "discrete_inputs")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.DiscreteInputs = areas
		hasArea = true
	}

	// holding_registers
	if v, ok := m["holding_registers"]; ok {
		areas, err := parseAreaSegments(v, "holding_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.HoldingRegisters = areas
		hasArea = true
	}

	// input_registers
	if v, ok := m["input_registers"]; ok {
		areas, err := parseAreaSegments(v, "input_registers")
		if err != nil {
			return RegisterMapConfig{}, err
		}
		cfg.InputRegisters = areas
		hasArea = true
	}

	// 최소 하나의 영역이 있어야 한다
	if !hasArea {
		return RegisterMapConfig{}, fmt.Errorf(
			"modbus-server: register_map must have at least one area: %w", ErrInvalidRegisterMap)
	}

	return cfg, nil
}

// parseAreaSegments 는 단일 맵(하위 호환) 또는 배열(다중 세그먼트) 형식을 파싱한다.
// 단일 맵: {"start_address": 0, "count": 100}
// 다중 세그먼트: [{"start_address": 0, "count": 100}, {"start_address": 200, "count": 100}]
func parseAreaSegments(v any, areaName string) ([]*RegisterAreaConfig, error) {
	switch val := v.(type) {
	case map[string]any:
		// 단일 블록 (하위 호환)
		area, err := parseRegisterAreaConfig(val, areaName)
		if err != nil {
			return nil, err
		}
		return []*RegisterAreaConfig{&area}, nil

	case []any:
		// 다중 세그먼트
		if len(val) == 0 {
			return nil, fmt.Errorf("modbus-server: register_map.%s must have at least one segment", areaName)
		}
		areas := make([]*RegisterAreaConfig, 0, len(val))
		for i, item := range val {
			areaMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("modbus-server: register_map.%s[%d] must be a map", areaName, i)
			}
			segName := fmt.Sprintf("%s[%d]", areaName, i)
			area, err := parseRegisterAreaConfig(areaMap, segName)
			if err != nil {
				return nil, err
			}
			areas = append(areas, &area)
		}
		// 세그먼트 간 겹침 검증
		if err := validateSegmentOverlap(areas, areaName); err != nil {
			return nil, err
		}
		return areas, nil

	default:
		return nil, fmt.Errorf("modbus-server: register_map.%s must be a map or array", areaName)
	}
}

// validateSegmentOverlap 는 세그먼트 간 주소 범위 겹침을 검증한다.
func validateSegmentOverlap(segments []*RegisterAreaConfig, areaName string) error {
	for i := 0; i < len(segments); i++ {
		iStart := segments[i].StartAddress
		iEnd := iStart + segments[i].Count
		for j := i + 1; j < len(segments); j++ {
			jStart := segments[j].StartAddress
			jEnd := jStart + segments[j].Count
			if iStart < jEnd && jStart < iEnd {
				return fmt.Errorf(
					"modbus-server: register_map.%s segments overlap: [%d,%d) and [%d,%d)",
					areaName, iStart, iEnd, jStart, jEnd)
			}
		}
	}
	return nil
}

// parseRegisterAreaConfig 는 레지스터 영역 설정 맵을 RegisterAreaConfig 로 파싱한다.
func parseRegisterAreaConfig(m map[string]any, areaName string) (RegisterAreaConfig, error) {
	var area RegisterAreaConfig

	// address (신규 키) — 하위 호환으로 start_address 도 허용
	if v, ok := m["address"]; ok {
		area.StartAddress = toUint16(v)
	} else if v, ok := m["start_address"]; ok {
		area.StartAddress = toUint16(v)
	}

	// count (필수, > 0)
	if v, ok := m["count"]; ok {
		area.Count = toUint16(v)
	}
	if area.Count == 0 {
		return RegisterAreaConfig{}, fmt.Errorf(
			"modbus-server: register_map.%s.count must be > 0", areaName)
	}

	// description (선택, 메타데이터 전용) — 로컬/공유 세그먼트 모두 적용.
	// shared_address 조기 반환 이전에 읽어 두 경로 모두에서 보존한다.
	if v, ok := m["description"]; ok {
		if s, ok := v.(string); ok {
			area.Description = s
		}
	}

	// shared_address (선택) — 존재하면 공유 세그먼트로 판별된다.
	// 공유 세그먼트는 data_type/initial_values/type_map 를 컨테이너(unit_id 0)에서
	// 상속하므로 로컬 전용 필드를 파싱하지 않는다.
	if v, ok := m["shared_address"]; ok {
		area.IsShared = true
		area.SharedAddress = toUint16(v)
		return area, nil
	}

	// initial_values (선택, 로컬 세그먼트 전용)
	if v, ok := m["initial_values"]; ok {
		if vals, ok := v.([]any); ok {
			if len(vals) > int(area.Count) {
				return RegisterAreaConfig{}, fmt.Errorf(
					"modbus-server: register_map.%s.initial_values length (%d) exceeds count (%d)",
					areaName, len(vals), area.Count)
			}
			area.InitialValues = vals
		}
	}

	// data_type (선택, 기본값 "uint16")
	if v, ok := m["data_type"]; ok {
		if s, ok := v.(string); ok {
			if !modbus.IsValidDataType(s) {
				return RegisterAreaConfig{}, fmt.Errorf(
					"modbus-server: register_map.%s.data_type is not supported: %q", areaName, s)
			}
			area.DataType = s
		}
	}

	// type_map (선택)
	if v, ok := m["type_map"]; ok {
		if entries, ok := v.([]any); ok {
			typeMap, err := parseTypeMap(entries, areaName)
			if err != nil {
				return RegisterAreaConfig{}, err
			}
			area.TypeMap = typeMap
		}
	}

	// type_map 검증
	if len(area.TypeMap) > 0 {
		if err := validateTypeMap(area.TypeMap, area.StartAddress, area.Count, areaName); err != nil {
			return RegisterAreaConfig{}, err
		}
	}

	return area, nil
}

// parseTypeMap은 []any 로부터 TypeMapEntry 슬라이스를 파싱한다.
func parseTypeMap(entries []any, areaName string) ([]modbus.TypeMapEntry, error) {
	result := make([]modbus.TypeMapEntry, 0, len(entries))
	for i, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d] must be a map", areaName, i)
		}

		var tme modbus.TypeMapEntry

		// address (필수)
		if v, ok := m["address"]; ok {
			tme.Address = toUint16(v)
		} else {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].address is required", areaName, i)
		}

		// data_type (필수)
		if v, ok := m["data_type"]; ok {
			if s, ok := v.(string); ok {
				if !modbus.IsValidDataType(s) {
					return nil, fmt.Errorf(
						"modbus-server: register_map.%s.type_map[%d].data_type is not supported: %q", areaName, i, s)
				}
				tme.DataType = s
			}
		} else {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].data_type is required", areaName, i)
		}

		// byte_order (선택, 기본값 "big_endian")
		tme.ByteOrder = modbus.ByteOrderBigEndian
		if v, ok := m["byte_order"]; ok {
			if s, ok := v.(string); ok {
				tme.ByteOrder = s
			}
		}
		if tme.ByteOrder != modbus.ByteOrderBigEndian && tme.ByteOrder != modbus.ByteOrderLittleEndian {
			return nil, fmt.Errorf(
				"modbus-server: register_map.%s.type_map[%d].byte_order must be %q or %q (got %q)",
				areaName, i, modbus.ByteOrderBigEndian, modbus.ByteOrderLittleEndian, tme.ByteOrder)
		}

		result = append(result, tme)
	}
	return result, nil
}

// validateTypeMap은 type_map의 겹침 검사 및 범위 검사를 수행한다.
func validateTypeMap(typeMap []modbus.TypeMapEntry, startAddr, count uint16, areaName string) error {
	endAddr := startAddr + count

	// 겹침 감지를 위한 점유 범위 목록
	type addrRange struct {
		start uint16
		end   uint16 // exclusive
	}
	occupied := make([]addrRange, 0, len(typeMap))

	for i, entry := range typeMap {
		regCount, err := modbus.RegisterCountForType(entry.DataType)
		if err != nil {
			return fmt.Errorf("modbus-server: register_map.%s.type_map[%d]: %w", areaName, i, err)
		}

		entryEnd := entry.Address + regCount

		// 범위 검사: [startAddr, startAddr+count) 안에 있어야 한다
		if entry.Address < startAddr || entryEnd > endAddr {
			return fmt.Errorf(
				"modbus-server: register_map.%s.type_map address %d (type %s, %d regs) exceeds range [%d, %d): %w",
				areaName, entry.Address, entry.DataType, regCount, startAddr, endAddr, ErrTypeMapOutOfRange)
		}

		// 이전 엔트리와의 겹침 검사
		for j, prev := range occupied {
			if entry.Address < prev.end && entryEnd > prev.start {
				return fmt.Errorf(
					"modbus-server: register_map.%s.type_map[%d] (addr %d) overlaps with type_map[%d] (addr %d-%d): %w",
					areaName, i, entry.Address, j, prev.start, prev.end-1, ErrTypeMapOverlap)
			}
		}

		occupied = append(occupied, addrRange{start: entry.Address, end: entryEnd})
	}

	return nil
}

// ---------------------------------------------------------------------------
// RTU 시리얼 설정 파싱
// ---------------------------------------------------------------------------

// parseServerSerialConfig 는 Transport.Options 에서 RTU 시리얼 파라미터를 파싱·검증한다.
// 클라이언트 에이전트(internal/agent/modbus)의 parseSerialConfig 와 동일한 옵션 키를
// 사용한다: serial_port(또는 port)는 필수이며, 나머지는 관례적 기본값을 가진다.
func parseServerSerialConfig(opts map[string]any) (SerialConfig, error) {
	sc := SerialConfig{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}

	// serial_port / port (필수)
	if v, ok := opts["serial_port"]; ok {
		sc.Port, _ = v.(string)
	} else if v, ok := opts["port"]; ok {
		sc.Port, _ = v.(string)
	}
	if sc.Port == "" {
		return SerialConfig{}, ErrMissingSerialPort
	}

	// baud_rate (기본 9600, > 0)
	if v, ok := opts["baud_rate"]; ok {
		sc.BaudRate = toInt(v)
	}
	if sc.BaudRate <= 0 {
		return SerialConfig{}, fmt.Errorf("modbus-server: baud_rate must be > 0 (got %d): %w", sc.BaudRate, ErrInvalidSerialParam)
	}

	// data_bits (기본 8, 5-8)
	if v, ok := opts["data_bits"]; ok {
		sc.DataBits = toInt(v)
	}
	if sc.DataBits < 5 || sc.DataBits > 8 {
		return SerialConfig{}, fmt.Errorf("modbus-server: data_bits must be 5-8 (got %d): %w", sc.DataBits, ErrInvalidSerialParam)
	}

	// stop_bits (기본 1, 1 또는 2)
	if v, ok := opts["stop_bits"]; ok {
		sc.StopBits = toInt(v)
	}
	if sc.StopBits != 1 && sc.StopBits != 2 {
		return SerialConfig{}, fmt.Errorf("modbus-server: stop_bits must be 1 or 2 (got %d): %w", sc.StopBits, ErrInvalidSerialParam)
	}

	// parity (기본 "none", none|even|odd)
	if v, ok := opts["parity"]; ok {
		if s, ok := v.(string); ok && s != "" {
			sc.Parity = s
		}
	}
	switch sc.Parity {
	case "none", "even", "odd":
	default:
		return SerialConfig{}, fmt.Errorf("modbus-server: parity %q must be none|even|odd: %w", sc.Parity, ErrInvalidSerialParam)
	}

	return sc, nil
}

// ---------------------------------------------------------------------------
// 타입 변환 헬퍼
// ---------------------------------------------------------------------------

// toInt 는 int 또는 float64 값을 int 로 변환한다.
// YAML/JSON 파싱에서 숫자가 float64 로 전달될 수 있으므로 두 타입 모두 처리한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// toByte 는 int 또는 float64 값을 byte 로 변환한다.
func toByte(v any) byte {
	switch n := v.(type) {
	case int:
		return byte(n)
	case float64:
		return byte(n)
	default:
		return 0
	}
}

// toUint16 는 int 또는 float64 값을 uint16 으로 변환한다.
func toUint16(v any) uint16 {
	switch n := v.(type) {
	case int:
		return uint16(n)
	case float64:
		return uint16(n)
	default:
		return 0
	}
}
