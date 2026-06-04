package node

import (
	"sort"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
)

// NodeFactory 는 NodeDef로부터 Node를 생성하는 팩토리 함수 타입이다.
type NodeFactory func(def flow.NodeDef, opts ...NodeOption) (Node, error)

// NodeTypeMeta 는 노드 타입의 메타데이터를 나타낸다.
type NodeTypeMeta struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// RegistryOption 은 Registry 생성 시 적용할 수 있는 옵션 함수 타입이다.
type RegistryOption func(*Registry)

// WithoutBuiltins 는 빌트인 노드 타입을 등록하지 않는 옵션을 반환한다.
// 테스트나 커스텀 레지스트리를 만들 때 유용하다.
func WithoutBuiltins() RegistryOption {
	return func(r *Registry) {
		r.skipBuiltins = true
	}
}

// Registry 는 노드 타입별 팩토리를 관리하는 레지스트리이다.
// 빌트인 노드 타입(filter, transform, switch, bridge, script, catch,
// aggregate, mapping, modbus, output, deadletter, samsung_hvacr01_status, samsung_hvacr01_control, samsung_hvacr01,
// mqtt-subscriber, mqtt-publisher, modbus-poller, modbus-writer, lgap-status, lgap-control, lgap,
// lgcp-status, lgcp-control, lgcp, lg_hvacr01_status, lg_hvacr01_control, lg_hvacr01,
// tsdb-write, tsdb-query, influxdb-write, influxdb-read, influxdb-query,
// store-write, store-read, serial-in, serial-out, tcp-in, tcp-out,
// framer, deduplicate, trigger, chart-emitter, inventory, select-field, flow-node)을 자동 등록한다.
type Registry struct {
	mu           sync.RWMutex
	factories    map[string]NodeFactory
	metadata     map[string]NodeTypeMeta
	skipBuiltins bool
}

// NewRegistry 는 새로운 Registry를 생성한다.
// WithoutBuiltins 옵션이 없으면 48개의 빌트인 노드 타입이 자동 등록된다.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		factories: make(map[string]NodeFactory),
		metadata:  make(map[string]NodeTypeMeta),
	}

	for _, opt := range opts {
		opt(r)
	}

	if !r.skipBuiltins {
		r.registerBuiltins()
	}

	return r
}

// registerBuiltins 는 빌트인 노드 팩토리와 메타데이터를 등록한다.
func (r *Registry) registerBuiltins() {
	builtins := []struct {
		typeName    string
		factory     NodeFactory
		category    string
		description string
	}{
		{"deduplicate", NewDeduplicateNode, "processing", "시간 창 내 중복 메시지 제거"},
		{"filter", NewFilterNode, "processing", "조건에 따라 메시지를 필터링"},
		{"transform", NewTransformNode, "processing", "메시지 데이터를 변환"},
		{"switch", NewSwitchNode, "routing", "조건에 따라 메시지를 라우팅"},
		{"bridge", NewBridgeNode, "io", "외부 에이전트와 메시지 송수신"},
		{"script", NewScriptNode, "processing", "스크립트로 메시지를 처리"},
		{"catch", NewCatchNode, "error", "에러 메시지를 캐치하여 처리"},
		{"aggregate", NewAggregateNode, "processing", "여러 메시지를 집계"},
		{"mapping", NewMappingNode, "processing", "키 기반 값 매핑"},
		{"modbus", NewModbusNode, "processing", "MODBUS 레지스터 읽기/쓰기"},
		{"output", NewDebugNode, "io", "메시지를 포맷팅하여 출력"},
		{"deadletter", NewDeadLetterNode, "error", "처리 실패 메시지를 보관"},
		{"samsung_hvacr01_status", NewSamsungHvacr01StatusNode, "io", "Samsung HVACR-01 (NASA) 디바이스 상태 조회"},
		{"samsung_hvacr01_control", NewSamsungHvacr01ControlNode, "io", "Samsung HVACR-01 (NASA) 디바이스 제어"},
		{"samsung_hvacr01", NewSamsungHvacr01Node, "io", "Samsung HVACR-01 (NASA) 상태 조회 + 제어 통합"},
		{"mqtt-subscriber", NewMQTTSubNode, "io", "MQTT 토픽 구독 및 메시지 수신"},
		{"mqtt-publisher", NewMQTTPublisherNode, "io", "MQTT 토픽으로 메시지 발행"},
		{"modbus-poller", NewModbusPollerNode, "io", "MODBUS 레지스터를 주기적으로 폴링 읽기"},
		{"modbus-writer", NewModbusWriterNode, "io", "MODBUS 레지스터 쓰기 전용"},
		{"lgap-status", NewLGAPStatusNode, "io", "LG LGAP 디바이스 상태 조회"},
		{"lgap-control", NewLGAPControlNode, "io", "LG LGAP 디바이스 제어"},
		{"lgap", NewLGAPNode, "io", "LG LGAP 상태 조회 + 제어 통합"},
		{"lg_hvacr02_status", NewLGHvacr02StatusNode, "io", "LG HVACR-02 (LG ICP-02) 디바이스 상태 조회"},
		{"lg_hvacr02_control", NewLGHvacr02ControlNode, "io", "LG HVACR-02 (LG ICP-02) 디바이스 제어"},
		{"lg_hvacr02", NewLGHvacr02Node, "io", "LG HVACR-02 (LG ICP-02) 상태 조회 + 제어 통합"},
		{"lg_hvacr01_status", NewLGHvacr01StatusNode, "io", "LG HVACR-01 (LG ICP-01) 디바이스 상태 조회"},
		{"lg_hvacr01_control", NewLGHvacr01ControlNode, "io", "LG HVACR-01 (LG ICP-01) 디바이스 제어 (미지원)"},
		{"lg_hvacr01", NewLGHvacr01Node, "io", "LG HVACR-01 (LG ICP-01) 상태 조회 + 제어 통합"},
		{"century_hvacr01_status", NewCenturyHvacr01StatusNode, "io", "Century HVACR-01 디바이스 상태 조회 (패시브 캡처)"},
		{"century_hvacr01_control", NewCenturyHvacr01ControlNode, "io", "Century HVACR-01 디바이스 제어 (미지원, 패시브 전용)"},
		{"century_hvacr01", NewCenturyHvacr01Node, "io", "Century HVACR-01 상태 조회 + 제어 통합 (emit_raw_frames 옵션 지원)"},
		{"tsdb-write", NewTSDBWriteNode, "storage", "메시지를 시계열 DB에 기록"},
		{"tsdb-query", NewTSDBQueryNode, "storage", "시계열 DB에서 데이터를 조회"},
		{"influxdb-write", NewInfluxDBWriteNode, "storage", "메시지를 InfluxDB에 기록"},
		{"influxdb-read", NewInfluxDBReadNode, "storage", "InfluxDB에서 주기적으로 데이터 조회"},
		{"influxdb-query", NewInfluxDBQueryNode, "storage", "입력 메시지 기반 InfluxDB 쿼리 실행"},
		{"store-write", NewStoreWriteNode, "storage", "메시지 데이터를 키-값 저장소에 기록"},
		{"store-read", NewStoreReadNode, "storage", "키-값 저장소에서 데이터를 조회"},
		{"serial-in", NewSerialInNode, "io", "시리얼 포트에서 데이터 수신"},
		{"serial-out", NewSerialOutNode, "io", "시리얼 포트로 데이터 전송"},
		{"tcp-in", NewTCPInNode, "io", "TCP 에이전트로부터 메시지 수신 (연결 정보 포함)"},
		{"tcp-out", NewTCPOutNode, "io", "TCP 에이전트를 통해 메시지 전송 (연결별 라우팅)"},
		{"framer", framerFactory, "processing", "바이트 스트림에서 프로토콜 프레임을 분리하여 완성된 프레임을 출력"},
		{"trigger", NewTriggerNode, "input", "스케줄 기반 데이터 자동 생성"},
		{"chart-emitter", NewChartEmitterNode, "output", "차트 패널용 WebSocket 채널로 메시지 발행"},
		{"inventory", NewInventoryNode, "processing", "in-process 디바이스/에이전트/노드/플로우 인벤토리 스냅샷을 emit"},
		{"select-field", NewSelectFieldNode, "processing", "메시지에서 지정한 필드만 남깁니다 (payload/metadata/message 그룹별 화이트리스트, 누락 시 무시/드랍/채움)"},
		{"flow-node", NewFlowNodePlaceholder, "composition", "다른 플로우를 참조하는 서브플로우 노드 (배포 시 확장됨)"},
	}
	for _, b := range builtins {
		r.factories[b.typeName] = b.factory
		r.metadata[b.typeName] = NodeTypeMeta{
			Type:        b.typeName,
			Category:    b.category,
			Description: b.description,
			Source:      "builtin",
		}
	}
}

// Register 는 새로운 노드 타입과 팩토리를 등록한다.
// 이미 등록된 타입이면 ErrNodeTypeAlreadyRegistered를 반환한다.
func (r *Registry) Register(typeName string, factory NodeFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[typeName]; exists {
		return ErrNodeTypeAlreadyRegistered
	}
	r.factories[typeName] = factory
	return nil
}

// RegisterWithMeta 는 새로운 노드 타입, 팩토리, 메타데이터를 함께 등록한다.
// 이미 등록된 타입이면 ErrNodeTypeAlreadyRegistered를 반환한다.
func (r *Registry) RegisterWithMeta(typeName string, factory NodeFactory, meta NodeTypeMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[typeName]; exists {
		return ErrNodeTypeAlreadyRegistered
	}
	r.factories[typeName] = factory
	meta.Type = typeName
	r.metadata[typeName] = meta
	return nil
}

// TypeMeta 는 지정된 타입의 메타데이터를 반환한다.
// 등록되지 않은 타입이면 false를 반환한다.
func (r *Registry) TypeMeta(typeName string) (NodeTypeMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.metadata[typeName]
	return meta, ok
}

// AllTypeMeta 는 등록된 모든 노드 타입의 메타데이터를 타입명 기준으로 정렬하여 반환한다.
func (r *Registry) AllTypeMeta() []NodeTypeMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.metadata))
	for t := range r.metadata {
		types = append(types, t)
	}
	sort.Strings(types)

	result := make([]NodeTypeMeta, 0, len(types))
	for _, t := range types {
		result = append(result, r.metadata[t])
	}
	return result
}

// Create 는 NodeDef의 Type에 해당하는 팩토리를 찾아 노드를 생성한다.
// 등록되지 않은 타입이면 ErrNodeTypeNotFound를 반환한다.
func (r *Registry) Create(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	r.mu.RLock()
	factory, exists := r.factories[def.Type]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrNodeTypeNotFound
	}
	return factory(def, opts...)
}

// Types 는 등록된 모든 노드 타입의 정렬된 목록을 반환한다.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// Has 는 지정된 타입이 등록되어 있는지 확인한다.
func (r *Registry) Has(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[typeName]
	return exists
}
