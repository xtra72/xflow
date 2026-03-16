package node

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xtra/xflow/pkg/message"
)

// ReadSpec 은 브릿지가 에이전트에게 요청할 단일 읽기 단위를 정의한다.
// Modbus: FunctionCode(3=holding, 4=input), 시작 주소, 레지스터 개수, Unit ID.
type ReadSpec struct {
	FunctionCode uint8  // Modbus Function Code (3=holding, 4=input)
	StartAddr    uint16 // 시작 레지스터 주소
	Quantity     uint16 // 읽을 레지스터 개수
	UnitID       uint8  // Modbus Unit ID
}

// ReadResult 는 에이전트로부터 받은 단일 읽기 결과이다.
type ReadResult struct {
	Spec ReadSpec
	Data []byte // 원시 바이트 (레지스터당 2바이트)
}

// PollableAdapter 는 브릿지 주도 폴링을 지원하는 어댑터 인터페이스이다.
// 이 인터페이스를 구현하는 어댑터가 있으면, 브릿지가 직접 폴링 루프를 구동한다.
type PollableAdapter interface {
	// ReadSpecs 는 이 어댑터의 레지스터 맵을 읽기 단위 목록으로 변환한다.
	ReadSpecs() []ReadSpec

	// AssembleMessage 는 여러 ReadResult를 조합하여 플로우 Message를 생성한다.
	AssembleMessage(results []ReadResult) (message.Message, error)
}

// CommandPollAdapter 는 JSON 명령 기반 폴링을 지원하는 어댑터 인터페이스이다.
// PollableAdapter가 레지스터 단위 읽기를 사용하는 반면,
// CommandPollAdapter는 에이전트의 Process() 명령을 통해 상태를 조회한다.
type CommandPollAdapter interface {
	// PollCommand 는 ag.Process()에 전달할 JSON 명령 바이트를 반환한다.
	PollCommand() []byte

	// AssemblePollMessage 는 Process() 응답을 플로우 Message로 변환한다.
	AssemblePollMessage(response []byte) (message.Message, error)
}

// BridgeAdapter 는 에이전트 타입별 전용 브릿지 어댑터 인터페이스이다.
// 각 에이전트 타입(MQTT, HTTP, Modbus 등)은 이 인터페이스를 구현하여
// 프로토콜별 메타데이터 변환 및 설정 검증 로직을 제공한다.
type BridgeAdapter interface {
	// Validate 는 주어진 BridgeConfig가 이 어댑터에서 유효한지 검증한다.
	Validate(config BridgeConfig) error

	// DefaultConfig 는 이 어댑터의 기본 BridgeConfig를 반환한다.
	DefaultConfig() BridgeConfig

	// TransformToFlow 는 에이전트로부터 수신한 바이트 데이터를 플로우 Message로 변환한다.
	// AgentMeta를 통해 프로토콜별 메타데이터를 전달받는다.
	TransformToFlow(data []byte, meta AgentMeta) (message.Message, error)

	// TransformToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
	// 프로토콜별 메타데이터를 AgentMeta로 반환한다.
	TransformToAgent(msg message.Message) ([]byte, AgentMeta, error)

	// HandleControl 은 제어 메시지를 처리한다.
	HandleControl(msg message.Message) error
}

// AgentConfigurable 은 에이전트 설정으로부터 per-agent 어댑터 인스턴스를 생성하는 선택적 인터페이스이다.
// 전역 레지스트리의 싱글턴 어댑터가 이 인터페이스를 구현하면, Bridge Init() 시
// 에이전트의 Transport.Options를 전달받아 해당 에이전트에 특화된 어댑터를 반환한다.
// Modbus 어댑터처럼 에이전트별 RegisterDef, UnitID 등이 다른 경우 반드시 구현해야 한다.
type AgentConfigurable interface {
	// ConfigureFromAgent 는 에이전트 설정(Transport.Options)을 기반으로
	// 새로운 BridgeAdapter 인스턴스를 생성하여 반환한다.
	// 원본 어댑터는 변경하지 않는다 (팩토리 패턴).
	ConfigureFromAgent(agentConfig map[string]any) (BridgeAdapter, error)
}

// BridgeConfigurable 은 Bridge 노드 설정으로부터 per-bridge 어댑터 인스턴스를 생성하는 선택적 인터페이스이다.
// AgentConfigurable이 에이전트 설정(Transport.Options)을 기반으로 하는 반면,
// BridgeConfigurable은 Bridge 노드 설정(PublishTopic 등)을 기반으로 한다.
// MQTT 어댑터처럼 Bridge별 발행 토픽이 다른 경우 구현한다.
type BridgeConfigurable interface {
	// ConfigureFromBridge 는 Bridge 설정을 기반으로
	// 새로운 BridgeAdapter 인스턴스를 생성하여 반환한다.
	// 원본 어댑터는 변경하지 않는다 (팩토리 패턴).
	ConfigureFromBridge(config BridgeConfig) (BridgeAdapter, error)
}

// AgentMeta 는 프로토콜별 메타데이터를 담는 구조체이다.
// 각 프로토콜(MQTT, HTTP, Modbus)의 고유 필드를 포함한다.
type AgentMeta struct {
	// AgentType 은 에이전트 타입 식별자이다 (예: "mqtt", "http", "modbus").
	AgentType string

	// MQTT 프로토콜 메타데이터
	Topic    string // MQTT 토픽
	QoS      int    // MQTT QoS 레벨 (0, 1, 2)
	Retained bool   // MQTT Retained 플래그

	// HTTP 프로토콜 메타데이터
	Headers     map[string]string // HTTP 헤더
	StatusCode  int               // HTTP 상태 코드
	ContentType string            // HTTP Content-Type
	URLPath     string            // HTTP URL 경로

	// Modbus 프로토콜 메타데이터
	UnitID        uint8  // Modbus Unit ID
	FunctionCode  uint8  // Modbus Function Code
	RegisterAddr  uint16 // Modbus 레지스터 주소
	RegisterCount uint16 // Modbus 레지스터 개수
}

// MetaToMetadata 는 AgentMeta 필드를 프로토콜별 접두사 키를 사용하여 message.Metadata에 설정한다.
// MQTT: mqtt.topic, mqtt.qos, mqtt.retained
// HTTP: http.status_code, http.content_type, http.url_path, http.header.{key}
// Modbus: modbus.unit_id, modbus.function_code, modbus.register_addr, modbus.register_count
func MetaToMetadata(meta AgentMeta, md message.Metadata) {
	// MQTT 메타데이터
	if meta.Topic != "" {
		md.Set("mqtt.topic", meta.Topic)
	}
	if meta.QoS != 0 {
		md.Set("mqtt.qos", strconv.Itoa(meta.QoS))
	}
	if meta.Retained {
		md.Set("mqtt.retained", "true")
	}

	// HTTP 메타데이터
	if meta.StatusCode != 0 {
		md.Set("http.status_code", strconv.Itoa(meta.StatusCode))
	}
	if meta.ContentType != "" {
		md.Set("http.content_type", meta.ContentType)
	}
	if meta.URLPath != "" {
		md.Set("http.url_path", meta.URLPath)
	}
	for key, value := range meta.Headers {
		md.Set(fmt.Sprintf("http.header.%s", key), value)
	}

	// Modbus 메타데이터
	if meta.UnitID != 0 {
		md.Set("modbus.unit_id", strconv.FormatUint(uint64(meta.UnitID), 10))
	}
	if meta.FunctionCode != 0 {
		md.Set("modbus.function_code", strconv.FormatUint(uint64(meta.FunctionCode), 10))
	}
	if meta.RegisterAddr != 0 {
		md.Set("modbus.register_addr", strconv.FormatUint(uint64(meta.RegisterAddr), 10))
	}
	if meta.RegisterCount != 0 {
		md.Set("modbus.register_count", strconv.FormatUint(uint64(meta.RegisterCount), 10))
	}
}

// MetadataToMeta 는 message.Metadata에서 프로토콜별 접두사 키를 추출하여 AgentMeta를 생성한다.
// MetaToMetadata의 역변환이다.
func MetadataToMeta(md message.Metadata) AgentMeta {
	meta := AgentMeta{}

	// MQTT 메타데이터 추출
	if v, ok := md.Get("mqtt.topic"); ok {
		meta.Topic = v
	}
	if v, ok := md.Get("mqtt.qos"); ok {
		if qos, err := strconv.Atoi(v); err == nil {
			meta.QoS = qos
		}
	}
	if v, ok := md.Get("mqtt.retained"); ok {
		meta.Retained = (v == "true")
	}

	// HTTP 메타데이터 추출
	if v, ok := md.Get("http.status_code"); ok {
		if code, err := strconv.Atoi(v); err == nil {
			meta.StatusCode = code
		}
	}
	if v, ok := md.Get("http.content_type"); ok {
		meta.ContentType = v
	}
	if v, ok := md.Get("http.url_path"); ok {
		meta.URLPath = v
	}

	// HTTP 헤더 추출 (http.header.{key} 형식)
	allMd := md.All()
	for key, value := range allMd {
		if strings.HasPrefix(key, "http.header.") {
			if meta.Headers == nil {
				meta.Headers = make(map[string]string)
			}
			headerKey := strings.TrimPrefix(key, "http.header.")
			meta.Headers[headerKey] = value
		}
	}

	// Modbus 메타데이터 추출
	if v, ok := md.Get("modbus.unit_id"); ok {
		if id, err := strconv.ParseUint(v, 10, 8); err == nil {
			meta.UnitID = uint8(id)
		}
	}
	if v, ok := md.Get("modbus.function_code"); ok {
		if fc, err := strconv.ParseUint(v, 10, 8); err == nil {
			meta.FunctionCode = uint8(fc)
		}
	}
	if v, ok := md.Get("modbus.register_addr"); ok {
		if addr, err := strconv.ParseUint(v, 10, 16); err == nil {
			meta.RegisterAddr = uint16(addr)
		}
	}
	if v, ok := md.Get("modbus.register_count"); ok {
		if count, err := strconv.ParseUint(v, 10, 16); err == nil {
			meta.RegisterCount = uint16(count)
		}
	}

	return meta
}
