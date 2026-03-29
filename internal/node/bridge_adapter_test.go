package node

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// --- DefaultAdapter 테스트 ---

// TestDefaultAdapter_TransformToFlow 는 DefaultAdapter가 DefaultTransformer와 동일하게 AgentToFlow를 수행하는지 확인한다.
func TestDefaultAdapter_TransformToFlow(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "일반 바이트 데이터",
			data: []byte(`{"key":"value"}`),
		},
		{
			name: "빈 바이트 데이터",
			data: []byte{},
		},
		{
			name: "nil 데이터",
			data: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewDefaultAdapter()
			transformer := NewDefaultTransformer()

			// 어댑터 변환
			adapterMsg, err := adapter.TransformToFlow(tt.data, AgentMeta{})
			require.NoError(t, err)
			require.NotNil(t, adapterMsg)

			// 트랜스포머 변환
			transformerMsg, err := transformer.AgentToFlow(tt.data)
			require.NoError(t, err)
			require.NotNil(t, transformerMsg)

			// _raw 페이로드 비교
			adapterRaw, adapterOk := adapterMsg.Payload().Get("_raw")
			transformerRaw, transformerOk := transformerMsg.Payload().Get("_raw")
			assert.Equal(t, transformerOk, adapterOk)
			assert.Equal(t, transformerRaw, adapterRaw)
		})
	}
}

// TestDefaultAdapter_TransformToAgent 는 DefaultAdapter가 DefaultTransformer와 동일하게 FlowToAgent를 수행하는지 확인한다.
func TestDefaultAdapter_TransformToAgent(t *testing.T) {
	tests := []struct {
		name       string
		setupMsg   func() message.Message
		wantData   []byte
		wantNoMeta bool
	}{
		{
			name: "_raw에 []byte가 있는 경우",
			setupMsg: func() message.Message {
				msg := message.New()
				msg.Payload().Set("_raw", []byte("hello"))
				return msg
			},
			wantData:   []byte("hello"),
			wantNoMeta: true,
		},
		{
			name: "_raw에 string이 있는 경우",
			setupMsg: func() message.Message {
				msg := message.New()
				msg.Payload().Set("_raw", "world")
				return msg
			},
			wantData:   []byte("world"),
			wantNoMeta: true,
		},
		{
			name: "_raw가 없는 경우 JSON 폴백",
			setupMsg: func() message.Message {
				msg := message.New()
				msg.Payload().Set("key", "value")
				return msg
			},
			wantNoMeta: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewDefaultAdapter()
			transformer := NewDefaultTransformer()
			msg := tt.setupMsg()

			// 어댑터 변환
			adapterData, adapterMeta, err := adapter.TransformToAgent(msg)
			require.NoError(t, err)

			// 트랜스포머 변환
			transformerData, transformerErr := transformer.FlowToAgent(msg)
			require.NoError(t, transformerErr)

			// 데이터 비교
			assert.Equal(t, transformerData, adapterData)

			// 빈 메타 확인
			if tt.wantNoMeta {
				assert.Equal(t, AgentMeta{}, adapterMeta)
			}
		})
	}
}

// TestDefaultAdapter_Validate 는 DefaultAdapter.Validate가 항상 nil을 반환하는지 확인한다.
func TestDefaultAdapter_Validate(t *testing.T) {
	adapter := NewDefaultAdapter()

	err := adapter.Validate(BridgeConfig{})
	assert.NoError(t, err)

	err = adapter.Validate(BridgeConfig{BufferSize: 1024})
	assert.NoError(t, err)
}

// TestDefaultAdapter_HandleControl 은 DefaultAdapter.HandleControl이 항상 nil을 반환하는지 확인한다.
func TestDefaultAdapter_HandleControl(t *testing.T) {
	adapter := NewDefaultAdapter()

	msg := message.New()
	msg.Payload().Set("action", "subscribe")

	err := adapter.HandleControl(msg)
	assert.NoError(t, err)
}

// TestDefaultAdapter_DefaultConfig 는 DefaultAdapter.DefaultConfig가 빈 BridgeConfig를 반환하는지 확인한다.
func TestDefaultAdapter_DefaultConfig(t *testing.T) {
	adapter := NewDefaultAdapter()
	cfg := adapter.DefaultConfig()
	assert.Equal(t, BridgeConfig{}, cfg)
}

// --- AdapterRegistry 테스트 ---

// TestAdapterRegistry_RegisterAndGet 은 어댑터를 등록하고 조회할 수 있는지 확인한다.
func TestAdapterRegistry_RegisterAndGet(t *testing.T) {
	registry := NewAdapterRegistry()
	adapter := NewDefaultAdapter()

	registry.RegisterAdapter("mqtt-client", adapter)

	got, ok := registry.GetAdapter("mqtt-client")
	assert.True(t, ok)
	assert.Equal(t, adapter, got)
}

// TestAdapterRegistry_GetUnknown 은 등록되지 않은 타입 조회 시 (nil, false)를 반환하는지 확인한다.
func TestAdapterRegistry_GetUnknown(t *testing.T) {
	registry := NewAdapterRegistry()

	got, ok := registry.GetAdapter("unknown")
	assert.False(t, ok)
	assert.Nil(t, got)
}

// TestAdapterRegistry_Overwrite 는 같은 타입으로 다시 등록하면 덮어쓰는지 확인한다.
func TestAdapterRegistry_Overwrite(t *testing.T) {
	registry := NewAdapterRegistry()
	adapter1 := NewDefaultAdapter()
	adapter2 := NewDefaultAdapter()

	registry.RegisterAdapter("http", adapter1)
	registry.RegisterAdapter("http", adapter2)

	got, ok := registry.GetAdapter("http")
	assert.True(t, ok)
	assert.Equal(t, adapter2, got)
}

// TestAdapterRegistry_MultipleTypes 는 여러 타입의 어댑터를 동시에 관리할 수 있는지 확인한다.
func TestAdapterRegistry_MultipleTypes(t *testing.T) {
	registry := NewAdapterRegistry()
	mqttAdapter := NewDefaultAdapter()
	httpAdapter := NewDefaultAdapter()
	modbusAdapter := NewDefaultAdapter()

	registry.RegisterAdapter("mqtt-client", mqttAdapter)
	registry.RegisterAdapter("http", httpAdapter)
	registry.RegisterAdapter("modbus", modbusAdapter)

	got, ok := registry.GetAdapter("mqtt-client")
	assert.True(t, ok)
	assert.Equal(t, mqttAdapter, got)

	got, ok = registry.GetAdapter("http")
	assert.True(t, ok)
	assert.Equal(t, httpAdapter, got)

	got, ok = registry.GetAdapter("modbus")
	assert.True(t, ok)
	assert.Equal(t, modbusAdapter, got)
}

// TestAdapterRegistry_ConcurrentAccess 는 동시 접근이 안전한지 확인한다 (go test -race).
func TestAdapterRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewAdapterRegistry()
	adapter := NewDefaultAdapter()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // 등록 + 조회 고루틴

	// 동시 등록
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			agentType := "type-" + string(rune('a'+idx%26))
			registry.RegisterAdapter(agentType, adapter)
		}(i)
	}

	// 동시 조회
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			agentType := "type-" + string(rune('a'+idx%26))
			registry.GetAdapter(agentType)
		}(i)
	}

	wg.Wait()
}

// --- 전역 레지스트리 테스트 ---

// TestGlobalRegistry_RegisterAndGet 은 패키지 수준 함수로 전역 레지스트리를 사용할 수 있는지 확인한다.
func TestGlobalRegistry_RegisterAndGet(t *testing.T) {
	// 전역 레지스트리 초기화 (테스트 격리를 위해 기존 등록 무시)
	adapter := NewDefaultAdapter()
	agentType := "test-global-adapter"

	RegisterAdapter(agentType, adapter)

	got, ok := GetAdapter(agentType)
	assert.True(t, ok)
	assert.Equal(t, adapter, got)

	// 등록되지 않은 타입
	_, ok = GetAdapter("non-existent-global")
	assert.False(t, ok)
}

// --- AdapterTransformerBridge 테스트 ---

// TestAdapterTransformerBridge_AgentToFlow 는 AdapterTransformerBridge가 BridgeTransformer로서 정상 동작하는지 확인한다.
func TestAdapterTransformerBridge_AgentToFlow(t *testing.T) {
	adapter := NewDefaultAdapter()
	bridge := NewAdapterTransformerBridge(adapter)

	data := []byte(`{"test":"data"}`)
	msg, err := bridge.AgentToFlow(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	raw, ok := msg.Payload().Get("_raw")
	assert.True(t, ok)
	assert.Equal(t, data, raw)
}

// TestAdapterTransformerBridge_FlowToAgent 는 AdapterTransformerBridge.FlowToAgent가 정상 동작하는지 확인한다.
func TestAdapterTransformerBridge_FlowToAgent(t *testing.T) {
	adapter := NewDefaultAdapter()
	bridge := NewAdapterTransformerBridge(adapter)

	msg := message.New()
	msg.Payload().Set("_raw", []byte("hello"))

	data, err := bridge.FlowToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

// TestAdapterTransformerBridge_ImplementsTransformer 는 AdapterTransformerBridge가 BridgeTransformer 인터페이스를 구현하는지 확인한다.
func TestAdapterTransformerBridge_ImplementsTransformer(t *testing.T) {
	var _ BridgeTransformer = NewAdapterTransformerBridge(NewDefaultAdapter())
}

// --- MetaToMetadata / MetadataToMeta 테스트 ---

// TestMetaToMetadata_MQTT 는 MQTT 메타데이터가 올바르게 변환되는지 확인한다.
func TestMetaToMetadata_MQTT(t *testing.T) {
	meta := AgentMeta{
		Topic:    "sensor/temperature",
		QoS:      1,
		Retained: true,
	}

	md := message.NewMetadata()
	MetaToMetadata(meta, md)

	v, ok := md.Get("mqtt.topic")
	assert.True(t, ok)
	assert.Equal(t, "sensor/temperature", v)

	v, ok = md.Get("mqtt.qos")
	assert.True(t, ok)
	assert.Equal(t, "1", v)

	v, ok = md.Get("mqtt.retained")
	assert.True(t, ok)
	assert.Equal(t, "true", v)
}

// TestMetaToMetadata_HTTP 는 HTTP 메타데이터가 올바르게 변환되는지 확인한다.
func TestMetaToMetadata_HTTP(t *testing.T) {
	meta := AgentMeta{
		StatusCode:  200,
		ContentType: "application/json",
		URLPath:     "/api/v1/users",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
		},
	}

	md := message.NewMetadata()
	MetaToMetadata(meta, md)

	v, ok := md.Get("http.status_code")
	assert.True(t, ok)
	assert.Equal(t, "200", v)

	v, ok = md.Get("http.content_type")
	assert.True(t, ok)
	assert.Equal(t, "application/json", v)

	v, ok = md.Get("http.url_path")
	assert.True(t, ok)
	assert.Equal(t, "/api/v1/users", v)

	v, ok = md.Get("http.header.Authorization")
	assert.True(t, ok)
	assert.Equal(t, "Bearer token123", v)

	v, ok = md.Get("http.header.X-Custom")
	assert.True(t, ok)
	assert.Equal(t, "value", v)
}

// TestMetaToMetadata_Modbus 는 Modbus 메타데이터가 올바르게 변환되는지 확인한다.
func TestMetaToMetadata_Modbus(t *testing.T) {
	meta := AgentMeta{
		UnitID:        1,
		FunctionCode:  3,
		RegisterAddr:  100,
		RegisterCount: 10,
	}

	md := message.NewMetadata()
	MetaToMetadata(meta, md)

	v, ok := md.Get("modbus.unit_id")
	assert.True(t, ok)
	assert.Equal(t, "1", v)

	v, ok = md.Get("modbus.function_code")
	assert.True(t, ok)
	assert.Equal(t, "3", v)

	v, ok = md.Get("modbus.register_addr")
	assert.True(t, ok)
	assert.Equal(t, "100", v)

	v, ok = md.Get("modbus.register_count")
	assert.True(t, ok)
	assert.Equal(t, "10", v)
}

// TestMetaToMetadata_제로값_생략 은 제로값 필드가 메타데이터에 설정되지 않는지 확인한다.
func TestMetaToMetadata_제로값_생략(t *testing.T) {
	meta := AgentMeta{} // 모든 필드가 제로값

	md := message.NewMetadata()
	MetaToMetadata(meta, md)

	allMd := md.All()
	assert.Empty(t, allMd, "제로값 AgentMeta에서는 메타데이터가 설정되지 않아야 한다")
}

// TestMetadataToMeta_MQTT 는 MQTT 메타데이터 추출이 올바르게 동작하는지 확인한다.
func TestMetadataToMeta_MQTT(t *testing.T) {
	md := message.NewMetadata()
	md.Set("mqtt.topic", "device/status")
	md.Set("mqtt.qos", "2")
	md.Set("mqtt.retained", "true")

	meta := MetadataToMeta(md)

	assert.Equal(t, "device/status", meta.Topic)
	assert.Equal(t, 2, meta.QoS)
	assert.True(t, meta.Retained)
}

// TestMetadataToMeta_HTTP 는 HTTP 메타데이터 추출이 올바르게 동작하는지 확인한다.
func TestMetadataToMeta_HTTP(t *testing.T) {
	md := message.NewMetadata()
	md.Set("http.status_code", "404")
	md.Set("http.content_type", "text/plain")
	md.Set("http.url_path", "/not-found")
	md.Set("http.header.Content-Length", "42")
	md.Set("http.header.Accept", "application/json")

	meta := MetadataToMeta(md)

	assert.Equal(t, 404, meta.StatusCode)
	assert.Equal(t, "text/plain", meta.ContentType)
	assert.Equal(t, "/not-found", meta.URLPath)
	require.NotNil(t, meta.Headers)
	assert.Equal(t, "42", meta.Headers["Content-Length"])
	assert.Equal(t, "application/json", meta.Headers["Accept"])
}

// TestMetadataToMeta_Modbus 는 Modbus 메타데이터 추출이 올바르게 동작하는지 확인한다.
func TestMetadataToMeta_Modbus(t *testing.T) {
	md := message.NewMetadata()
	md.Set("modbus.unit_id", "5")
	md.Set("modbus.function_code", "4")
	md.Set("modbus.register_addr", "200")
	md.Set("modbus.register_count", "20")

	meta := MetadataToMeta(md)

	assert.Equal(t, uint8(5), meta.UnitID)
	assert.Equal(t, uint8(4), meta.FunctionCode)
	assert.Equal(t, uint16(200), meta.RegisterAddr)
	assert.Equal(t, uint16(20), meta.RegisterCount)
}

// TestMetadataToMeta_빈메타데이터 는 빈 메타데이터에서 제로값 AgentMeta를 반환하는지 확인한다.
func TestMetadataToMeta_빈메타데이터(t *testing.T) {
	md := message.NewMetadata()
	meta := MetadataToMeta(md)

	assert.Equal(t, AgentMeta{}, meta)
}

// TestMetaToMetadata_MetadataToMeta_라운드트립 은 변환 후 역변환이 원래 값을 복원하는지 확인한다.
func TestMetaToMetadata_MetadataToMeta_라운드트립(t *testing.T) {
	original := AgentMeta{
		Topic:         "test/topic",
		QoS:           1,
		Retained:      true,
		StatusCode:    200,
		ContentType:   "application/json",
		URLPath:       "/api/test",
		Headers:       map[string]string{"X-Key": "val"},
		UnitID:        10,
		FunctionCode:  3,
		RegisterAddr:  500,
		RegisterCount: 50,
	}

	md := message.NewMetadata()
	MetaToMetadata(original, md)
	restored := MetadataToMeta(md)

	assert.Equal(t, original.Topic, restored.Topic)
	assert.Equal(t, original.QoS, restored.QoS)
	assert.Equal(t, original.Retained, restored.Retained)
	assert.Equal(t, original.StatusCode, restored.StatusCode)
	assert.Equal(t, original.ContentType, restored.ContentType)
	assert.Equal(t, original.URLPath, restored.URLPath)
	assert.Equal(t, original.Headers, restored.Headers)
	assert.Equal(t, original.UnitID, restored.UnitID)
	assert.Equal(t, original.FunctionCode, restored.FunctionCode)
	assert.Equal(t, original.RegisterAddr, restored.RegisterAddr)
	assert.Equal(t, original.RegisterCount, restored.RegisterCount)
}

// TestMetadataToMeta_잘못된값_무시 는 파싱 불가능한 값이 무시되는지 확인한다.
func TestMetadataToMeta_잘못된값_무시(t *testing.T) {
	md := message.NewMetadata()
	md.Set("mqtt.qos", "invalid")
	md.Set("http.status_code", "not-a-number")
	md.Set("modbus.unit_id", "abc")
	md.Set("modbus.function_code", "xyz")
	md.Set("modbus.register_addr", "!!!")
	md.Set("modbus.register_count", "---")

	meta := MetadataToMeta(md)

	// 파싱 실패 시 제로값으로 유지
	assert.Equal(t, 0, meta.QoS)
	assert.Equal(t, 0, meta.StatusCode)
	assert.Equal(t, uint8(0), meta.UnitID)
	assert.Equal(t, uint8(0), meta.FunctionCode)
	assert.Equal(t, uint16(0), meta.RegisterAddr)
	assert.Equal(t, uint16(0), meta.RegisterCount)
}

// TestMetaToMetadata_MQTT_Retained_false 는 Retained가 false일 때 메타데이터에 설정되지 않는지 확인한다.
func TestMetaToMetadata_MQTT_Retained_false(t *testing.T) {
	meta := AgentMeta{
		Topic:    "test/topic",
		Retained: false,
	}

	md := message.NewMetadata()
	MetaToMetadata(meta, md)

	_, ok := md.Get("mqtt.retained")
	assert.False(t, ok, "Retained가 false이면 메타데이터에 설정되지 않아야 한다")
}

// TestMetadataToMeta_MQTT_Retained_false_문자열 은 "false" 문자열이 false로 변환되는지 확인한다.
func TestMetadataToMeta_MQTT_Retained_false_문자열(t *testing.T) {
	md := message.NewMetadata()
	md.Set("mqtt.retained", "false")

	meta := MetadataToMeta(md)
	assert.False(t, meta.Retained)
}
