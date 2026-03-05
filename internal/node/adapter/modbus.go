package adapter

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// RegisterDef 는 Modbus 레지스터 정의를 나타낸다.
type RegisterDef struct {
	Name      string  `json:"name"`       // 레지스터 이름 (Payload 키로 사용)
	Address   uint16  `json:"address"`    // 시작 주소
	Count     uint16  `json:"count"`      // 레지스터 개수
	DataType  string  `json:"data_type"`  // int16, uint16, float32, uint32, int32
	ByteOrder string  `json:"byte_order"` // big, little (기본: big)
	Scale     float64 `json:"scale"`      // 스케일 팩터 (기본: 1.0)
	Offset    float64 `json:"offset"`     // 오프셋 (기본: 0.0)
	ReadOnly  bool    `json:"read_only"`  // 읽기 전용
}

// validDataTypes 는 지원하는 Modbus 레지스터 데이터 타입과 필요한 레지스터 개수이다.
var validDataTypes = map[string]int{
	"int16":   1,
	"uint16":  1,
	"float32": 2,
	"uint32":  2,
	"int32":   2,
}

// ModbusAdapter 는 Modbus 프로토콜 전용 브릿지 어댑터이다.
// 레지스터 정의를 기반으로 바이트 데이터와 Go 네이티브 값 간의 변환을 수행한다.
type ModbusAdapter struct {
	unitID          uint8
	registers       []RegisterDef
	pollingInterval time.Duration
}

// ModbusAdapterOption 은 ModbusAdapter 생성 시 적용할 옵션 함수 타입이다.
type ModbusAdapterOption func(*ModbusAdapter)

// WithUnitID 는 Modbus Unit ID를 설정한다.
func WithUnitID(id uint8) ModbusAdapterOption {
	return func(a *ModbusAdapter) {
		a.unitID = id
	}
}

// WithRegisters 는 레지스터 정의 목록을 설정한다.
func WithRegisters(regs []RegisterDef) ModbusAdapterOption {
	return func(a *ModbusAdapter) {
		a.registers = regs
	}
}

// WithPollingInterval 은 폴링 간격을 설정한다.
func WithPollingInterval(d time.Duration) ModbusAdapterOption {
	return func(a *ModbusAdapter) {
		a.pollingInterval = d
	}
}

// NewModbusAdapter 는 주어진 옵션으로 새로운 ModbusAdapter 인스턴스를 반환한다.
func NewModbusAdapter(opts ...ModbusAdapterOption) *ModbusAdapter {
	a := &ModbusAdapter{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Validate 는 주어진 BridgeConfig와 어댑터 내부 설정이 유효한지 검증한다.
// Unit ID: 1-247, 최소 1개 이상의 레지스터 정의, 유효한 데이터 타입,
// In/InOut 방향에서 폴링 간격 >= 100ms를 검증한다.
func (a *ModbusAdapter) Validate(config node.BridgeConfig) error {
	// Unit ID: 1-247
	if a.unitID < 1 || a.unitID > 247 {
		return fmt.Errorf("modbus adapter: unit ID must be 1-247, got %d", a.unitID)
	}

	// 최소 1개 이상의 레지스터 정의 필요
	if len(a.registers) == 0 {
		return fmt.Errorf("modbus adapter: at least one register definition required")
	}

	// 각 레지스터 정의 검증
	for _, reg := range a.registers {
		if _, ok := validDataTypes[reg.DataType]; !ok {
			return fmt.Errorf("modbus adapter: invalid data type %q for register %q", reg.DataType, reg.Name)
		}
	}

	// In/InOut 방향에서 폴링 간격 검증
	dir := config.Direction
	if dir == flow.BridgeIn || dir == flow.BridgeInOut {
		if a.pollingInterval > 0 && a.pollingInterval < 100*time.Millisecond {
			return fmt.Errorf("modbus adapter: polling interval must be >= 100ms, got %v", a.pollingInterval)
		}
	}

	return nil
}

// DefaultConfig 는 이 어댑터의 기본 BridgeConfig를 반환한다.
func (a *ModbusAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{
		Direction:  flow.BridgeInOut,
		BufferSize: 64,
	}
}

// TransformToFlow 는 레지스터 바이트 데이터를 플로우 Message로 변환한다.
// data 포맷: 레지스터 정의 순서대로 연결된 원시 바이트 (레지스터당 2바이트).
// 각 레지스터 값에 Scale과 Offset을 적용하여 Payload에 이름으로 설정한다.
func (a *ModbusAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	if data == nil {
		return msg, nil
	}

	offset := 0
	for _, reg := range a.registers {
		byteCount := int(reg.Count) * 2
		if offset+byteCount > len(data) {
			break // 남은 데이터 부족
		}
		regBytes := data[offset : offset+byteCount]
		value, err := registersToValue(regBytes, reg)
		if err != nil {
			return msg, fmt.Errorf("modbus adapter: register %q conversion: %w", reg.Name, err)
		}

		// Scale/Offset 적용: result = raw * scale + offset
		scaled := applyScaleOffset(value, reg.Scale, reg.Offset)
		msg.Payload().Set(reg.Name, scaled)
		offset += byteCount
	}

	// Modbus 메타데이터 설정
	node.MetaToMetadata(meta, msg.Metadata())

	return msg, nil
}

// TransformToAgent 는 플로우 Message를 레지스터 바이트 데이터로 변환한다.
// ReadOnly 레지스터는 건너뛰고, Payload에 없는 레지스터도 건너뛴다.
// 역 Scale/Offset을 적용하여 원시 값으로 변환한 후 바이트 배열로 직렬화한다.
func (a *ModbusAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	var result []byte

	for _, reg := range a.registers {
		if reg.ReadOnly {
			continue
		}
		val, ok := msg.Payload().Get(reg.Name)
		if !ok {
			continue
		}

		// 역 Scale/Offset: raw = (value - offset) / scale
		rawVal := reverseScaleOffset(val, reg.Scale, reg.Offset)

		regBytes, err := valueToRegisters(rawVal, reg)
		if err != nil {
			return nil, node.AgentMeta{}, fmt.Errorf("modbus adapter: register %q conversion: %w", reg.Name, err)
		}
		result = append(result, regBytes...)
	}

	meta := node.AgentMeta{
		AgentType: "modbus",
		UnitID:    a.unitID,
	}

	// 레지스터 개수에 따른 Function Code 결정
	if len(result) <= 2 {
		meta.FunctionCode = 6 // FC06: Write Single Register
	} else {
		meta.FunctionCode = 16 // FC16: Write Multiple Registers
	}

	return result, meta, nil
}

// HandleControl 은 제어 메시지를 처리한다.
// Modbus 어댑터는 현재 별도의 제어 메시지 처리가 필요 없으므로 nil을 반환한다.
func (a *ModbusAdapter) HandleControl(_ message.Message) error {
	return nil
}

// registersToValue 는 레지스터 바이트를 RegisterDef에 따라 Go 네이티브 float64 값으로 변환한다.
func registersToValue(data []byte, reg RegisterDef) (float64, error) {
	order := getByteOrder(reg.ByteOrder)

	switch reg.DataType {
	case "uint16":
		if len(data) < 2 {
			return 0, fmt.Errorf("insufficient data for uint16: need 2 bytes, got %d", len(data))
		}
		return float64(order.Uint16(data)), nil
	case "int16":
		if len(data) < 2 {
			return 0, fmt.Errorf("insufficient data for int16: need 2 bytes, got %d", len(data))
		}
		return float64(int16(order.Uint16(data))), nil
	case "uint32":
		if len(data) < 4 {
			return 0, fmt.Errorf("insufficient data for uint32: need 4 bytes, got %d", len(data))
		}
		return float64(order.Uint32(data)), nil
	case "int32":
		if len(data) < 4 {
			return 0, fmt.Errorf("insufficient data for int32: need 4 bytes, got %d", len(data))
		}
		return float64(int32(order.Uint32(data))), nil
	case "float32":
		if len(data) < 4 {
			return 0, fmt.Errorf("insufficient data for float32: need 4 bytes, got %d", len(data))
		}
		bits := order.Uint32(data)
		return float64(math.Float32frombits(bits)), nil
	default:
		return 0, fmt.Errorf("unsupported data type %q", reg.DataType)
	}
}

// valueToRegisters 는 Go 네이티브 값을 RegisterDef에 따라 레지스터 바이트로 변환한다.
func valueToRegisters(value float64, reg RegisterDef) ([]byte, error) {
	order := getByteOrder(reg.ByteOrder)
	byteCount := int(reg.Count) * 2
	buf := make([]byte, byteCount)

	switch reg.DataType {
	case "uint16":
		order.PutUint16(buf, uint16(value))
	case "int16":
		order.PutUint16(buf, uint16(int16(value)))
	case "uint32":
		order.PutUint32(buf, uint32(value))
	case "int32":
		order.PutUint32(buf, uint32(int32(value)))
	case "float32":
		order.PutUint32(buf, math.Float32bits(float32(value)))
	default:
		return nil, fmt.Errorf("unsupported data type %q", reg.DataType)
	}
	return buf, nil
}

// getByteOrder 는 문자열 바이트 오더 지정자를 binary.ByteOrder로 변환한다.
// "little"이면 LittleEndian, 그 외에는 BigEndian(기본값)을 반환한다.
func getByteOrder(order string) binary.ByteOrder {
	if order == "little" {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// applyScaleOffset 은 원시 값에 Scale과 Offset을 적용한다.
// result = raw * scale + offset. Scale이 0이면 1.0으로 취급한다.
func applyScaleOffset(raw float64, scale, offset float64) float64 {
	if scale == 0 {
		scale = 1.0
	}
	return raw*scale + offset
}

// reverseScaleOffset 은 Scale과 Offset을 역으로 적용한다.
// raw = (value - offset) / scale. Scale이 0이면 1.0으로 취급한다.
func reverseScaleOffset(val any, scale, offset float64) float64 {
	if scale == 0 {
		scale = 1.0
	}
	v := toFloat64(val)
	return (v - offset) / scale
}

// toFloat64 는 다양한 숫자 타입을 float64로 변환한다.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		return 0
	}
}
