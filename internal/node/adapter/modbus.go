package adapter

import (
	"encoding/binary"
	"encoding/json"
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
	Precision *int    `json:"precision,omitempty"` // 소수점 자릿수 (nil=타입 기본값: float→1, 정수→0)
	ReadOnly  bool    `json:"read_only"`  // 읽기 전용
	Area      string  `json:"area"`       // 레지스터 영역 (holding_registers, input_registers, coils, discrete_inputs)
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

// ConfigureFromAgent 는 에이전트 설정(Transport.Options)을 기반으로
// 새로운 ModbusAdapter 인스턴스를 생성하여 반환한다.
// config에서 "unit_id"와 "register_defs" 키를 추출한다.
// register_defs가 없으면 원본 어댑터를 그대로 반환한다.
func (a *ModbusAdapter) ConfigureFromAgent(agentConfig map[string]any) (node.BridgeAdapter, error) {
	unitID := a.unitID
	if v, ok := agentConfig["unit_id"]; ok {
		unitID = toUint8(v)
	}

	regsRaw, ok := agentConfig["register_defs"]
	if !ok {
		// register_defs가 없으면 원본 어댑터 반환 (기존 동작 유지)
		return a, nil
	}

	regs, err := parseRegisterDefs(regsRaw)
	if err != nil {
		return nil, fmt.Errorf("modbus adapter: %w", err)
	}

	return NewModbusAdapter(
		WithUnitID(unitID),
		WithRegisters(regs),
		WithPollingInterval(a.pollingInterval),
	), nil
}

// parseRegisterDefs 는 에이전트 config의 register_defs 값을 []RegisterDef로 파싱한다.
func parseRegisterDefs(raw any) ([]RegisterDef, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("register_defs must be an array")
	}

	regs := make([]RegisterDef, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("register_defs[%d] must be an object", i)
		}

		reg := RegisterDef{
			Scale: 1.0, // 기본값
		}

		if v, ok := m["name"].(string); ok {
			reg.Name = v
		}
		if v, ok := m["address"]; ok {
			reg.Address = toUint16(v)
		}
		if v, ok := m["count"]; ok {
			reg.Count = toUint16(v)
		} else {
			// count 미지정 시 data_type으로 추론
			if dt, ok := m["data_type"].(string); ok {
				if cnt, ok := validDataTypes[dt]; ok {
					reg.Count = uint16(cnt)
				}
			}
		}
		if v, ok := m["data_type"].(string); ok {
			reg.DataType = v
		}
		if v, ok := m["byte_order"].(string); ok {
			reg.ByteOrder = v
		}
		if v, ok := m["scale"]; ok {
			reg.Scale = toFloat64(v)
		}
		if v, ok := m["offset"]; ok {
			reg.Offset = toFloat64(v)
		}
		if v, ok := m["read_only"].(bool); ok {
			reg.ReadOnly = v
		}
		if v, ok := m["area"].(string); ok {
			reg.Area = v
		}

		regs = append(regs, reg)
	}

	return regs, nil
}

// toUint8 은 다양한 숫자 타입을 uint8로 변환한다.
func toUint8(v any) uint8 {
	switch n := v.(type) {
	case int:
		return uint8(n)
	case float64:
		return uint8(n)
	case int64:
		return uint8(n)
	case uint8:
		return n
	default:
		return 0
	}
}

// toUint16 은 다양한 숫자 타입을 uint16로 변환한다.
func toUint16(v any) uint16 {
	switch n := v.(type) {
	case int:
		return uint16(n)
	case float64:
		return uint16(n)
	case int64:
		return uint16(n)
	case uint16:
		return n
	default:
		return 0
	}
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
		rounded := roundToPrecision(scaled, resolvePrecision(reg))
		msg.Payload().Set(reg.Name, rounded)
		offset += byteCount
	}

	// Modbus 메타데이터 설정
	node.MetaToMetadata(meta, msg.Metadata())

	return msg, nil
}

// bulkWriteEntry 는 bulk_write 커맨드의 개별 쓰기 항목이다.
type bulkWriteEntry struct {
	Area      string  `json:"area"`
	Address   uint16  `json:"address"`
	Value     float64 `json:"value"`
	DataType  string  `json:"data_type"`
	ByteOrder string  `json:"byte_order"`
}

// bulkWriteRequest 는 ModbusServerAgent에 전달할 bulk_write 커맨드 구조체이다.
type bulkWriteRequest struct {
	Command string         `json:"command"`
	Params  map[string]any `json:"params"`
}

// TransformToAgent 는 플로우 Message를 ModbusServerAgent의 bulk_write 커맨드 JSON으로 변환한다.
// ReadOnly 레지스터는 건너뛰고, Payload에 없는 레지스터도 건너뛴다.
// 역 Scale/Offset을 적용하여 원시 값으로 변환한 후 bulk_write JSON으로 직렬화한다.
func (a *ModbusAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	var writes []bulkWriteEntry

	for _, reg := range a.registers {
		if reg.ReadOnly {
			continue
		}
		val, ok := msg.Payload().Get(reg.Name)
		if !ok || val == nil {
			continue
		}

		// 역 Scale/Offset: raw = (value - offset) / scale
		rawVal := reverseScaleOffset(val, reg.Scale, reg.Offset)

		// 레지스터 영역 결정 (기본값: holding_registers)
		area := reg.Area
		if area == "" {
			area = "holding_registers"
		}

		// byte_order 변환: "little" -> "little_endian", "big"/"" -> "big_endian"
		byteOrder := mapByteOrder(reg.ByteOrder)

		writes = append(writes, bulkWriteEntry{
			Area:      area,
			Address:   reg.Address,
			Value:     rawVal,
			DataType:  reg.DataType,
			ByteOrder: byteOrder,
		})
	}

	req := bulkWriteRequest{
		Command: "bulk_write",
		Params: map[string]any{
			"writes": writes,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("modbus adapter: marshal bulk_write: %w", err)
	}

	meta := node.AgentMeta{
		AgentType: "modbus",
		UnitID:    a.unitID,
	}

	return data, meta, nil
}

// mapByteOrder 는 RegisterDef의 byte_order ("big", "little", "")를
// ModbusServerAgent가 사용하는 형식 ("big_endian", "little_endian")으로 변환한다.
func mapByteOrder(order string) string {
	if order == "little" {
		return "little_endian"
	}
	return "big_endian"
}

// HandleControl 은 제어 메시지를 처리한다.
// Modbus 어댑터는 현재 별도의 제어 메시지 처리가 필요 없으므로 nil을 반환한다.
func (a *ModbusAdapter) HandleControl(_ message.Message) error {
	return nil
}

// areaToFunctionCode 는 레지스터 Area 문자열을 Modbus Function Code로 변환한다.
func areaToFunctionCode(area string) uint8 {
	switch area {
	case "input_registers":
		return 4 // FC04
	case "holding_registers", "":
		return 3 // FC03 (기본값)
	default:
		return 3
	}
}

// ReadSpecs 는 레지스터 정의 목록을 브릿지 폴링용 ReadSpec 목록으로 변환한다.
// 각 RegisterDef를 개별 ReadSpec으로 변환한다.
func (a *ModbusAdapter) ReadSpecs() []node.ReadSpec {
	specs := make([]node.ReadSpec, 0, len(a.registers))
	for _, reg := range a.registers {
		specs = append(specs, node.ReadSpec{
			FunctionCode: areaToFunctionCode(reg.Area),
			StartAddr:    reg.Address,
			Quantity:     reg.Count,
			UnitID:       a.unitID,
		})
	}
	return specs
}

// AssembleMessage 는 여러 ReadResult를 조합하여 플로우 Message를 생성한다.
// 각 ReadResult의 바이트를 해당 RegisterDef에 따라 typed value로 변환한다.
func (a *ModbusAdapter) AssembleMessage(results []node.ReadResult) (message.Message, error) {
	msg := message.New()

	// ReadResult를 (FunctionCode, StartAddr)로 인덱싱하여 빠른 매칭
	resultMap := make(map[uint32][]byte, len(results))
	for _, r := range results {
		key := uint32(r.Spec.FunctionCode)<<16 | uint32(r.Spec.StartAddr)
		resultMap[key] = r.Data
	}

	for _, reg := range a.registers {
		fc := areaToFunctionCode(reg.Area)
		key := uint32(fc)<<16 | uint32(reg.Address)
		data, ok := resultMap[key]
		if !ok {
			continue
		}

		byteCount := int(reg.Count) * 2
		if len(data) < byteCount {
			continue
		}

		value, err := registersToValue(data[:byteCount], reg)
		if err != nil {
			return msg, fmt.Errorf("modbus adapter: assemble register %q: %w", reg.Name, err)
		}

		scaled := applyScaleOffset(value, reg.Scale, reg.Offset)
		rounded := roundToPrecision(scaled, resolvePrecision(reg))
		msg.Payload().Set(reg.Name, rounded)
	}

	// Modbus 메타데이터
	meta := node.AgentMeta{
		AgentType: "modbus",
		UnitID:    a.unitID,
	}
	node.MetaToMetadata(meta, msg.Metadata())

	return msg, nil
}

// compile-time check: ModbusAdapter는 PollableAdapter를 구현한다.
var _ node.PollableAdapter = (*ModbusAdapter)(nil)

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

// resolvePrecision 은 레지스터의 유효 소수점 자릿수를 결정한다.
// Precision이 명시적으로 설정되면 해당 값을, 미설정이면 타입 기본값을 사용한다.
// 기본값: float32 → 1, 정수 타입(int16, uint16, int32, uint32) → 0.
func resolvePrecision(reg RegisterDef) int {
	if reg.Precision != nil {
		return *reg.Precision
	}
	switch reg.DataType {
	case "float32":
		return 1
	default:
		return 0
	}
}

// roundToPrecision 은 값을 지정된 소수점 자릿수로 반올림한다.
// precision < 0 이면 반올림하지 않는다.
func roundToPrecision(value float64, precision int) float64 {
	if precision < 0 {
		return value
	}
	pow := math.Pow(10, float64(precision))
	return math.Round(value*pow) / pow
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
