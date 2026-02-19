package node

import (
	"testing"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestParseExpression_Valid(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		wantFields int
		wantKeys   []string
		wantPaths  []string
	}{
		{
			name:       "단일 필드",
			expr:       "{ temperature: $.payload.object.temperature }",
			wantFields: 1,
			wantKeys:   []string{"temperature"},
			wantPaths:  []string{"$.payload.object.temperature"},
		},
		{
			name:       "여러 필드",
			expr:       "{ device_id: $.payload.deviceInfo.devEui, location: $.payload.deviceInfo.tags.location, temperature: $.payload.object.temperature, humidity: $.payload.object.humidity }",
			wantFields: 4,
			wantKeys:   []string{"device_id", "location", "temperature", "humidity"},
			wantPaths:  []string{"$.payload.deviceInfo.devEui", "$.payload.deviceInfo.tags.location", "$.payload.object.temperature", "$.payload.object.humidity"},
		},
		{
			name:       "공백 포함",
			expr:       "{  temp : $.payload.data.temp ,  hum : $.payload.data.hum  }",
			wantFields: 2,
			wantKeys:   []string{"temp", "hum"},
			wantPaths:  []string{"$.payload.data.temp", "$.payload.data.hum"},
		},
		{
			name:       "최상위 필드",
			expr:       "{ name: $.payload.name }",
			wantFields: 1,
			wantKeys:   []string{"name"},
			wantPaths:  []string{"$.payload.name"},
		},
		{
			name:       "메시지 ID 접근",
			expr:       "{ msg_id: $.id }",
			wantFields: 1,
			wantKeys:   []string{"msg_id"},
			wantPaths:  []string{"$.id"},
		},
		{
			name:       "메타데이터 접근",
			expr:       "{ source: $.metadata._source }",
			wantFields: 1,
			wantKeys:   []string{"source"},
			wantPaths:  []string{"$.metadata._source"},
		},
		{
			name:       "payload와 metadata 혼합",
			expr:       "{ temp: $.payload.temperature, source: $.metadata._source, msg_id: $.id }",
			wantFields: 3,
			wantKeys:   []string{"temp", "source", "msg_id"},
			wantPaths:  []string{"$.payload.temperature", "$.metadata._source", "$.id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, err := parseExpression(tt.expr)
			if err != nil {
				t.Fatalf("parseExpression() error = %v", err)
			}
			if len(fields) != tt.wantFields {
				t.Errorf("parseExpression() fields = %d, want %d", len(fields), tt.wantFields)
			}
			for i, f := range fields {
				if i < len(tt.wantKeys) && f.key != tt.wantKeys[i] {
					t.Errorf("field[%d].key = %q, want %q", i, f.key, tt.wantKeys[i])
				}
				if i < len(tt.wantPaths) && f.path != tt.wantPaths[i] {
					t.Errorf("field[%d].path = %q, want %q", i, f.path, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestParseExpression_Invalid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"빈 문자열", ""},
		{"중괄호 없음", "temperature: $.payload.temp"},
		{"여는 중괄호만", "{ temperature: $.payload.temp"},
		{"닫는 중괄호만", "temperature: $.payload.temp }"},
		{"빈 중괄호", "{ }"},
		{"콜론 없음", "{ temperature $.payload.temp }"},
		{"빈 키", "{ : $.payload.temp }"},
		{"잘못된 경로", "{ temp: invalid.path }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseExpression(tt.expr)
			if err == nil {
				t.Error("parseExpression() expected error, got nil")
			}
		})
	}
}

func TestCompileExpression_FieldExtraction(t *testing.T) {
	fn, err := compileExpression("{ device_id: $.payload.device_id, temperature: $.payload.object.temperature, humidity: $.payload.object.humidity }")
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	// 입력 메시지 생성
	inputPayload := message.NewPayload(map[string]any{
		"device_id": "sensor-001",
		"object": map[string]any{
			"temperature": 72.5,
			"humidity":    45.2,
		},
		"firmware": "v2.1.0",
		"raw_adc":  []any{1024.0, 2048.0, 512.0},
	})
	inputMsg := message.New(
		message.WithPayload(inputPayload),
		message.WithMetadata("_source", "mqtt-agent"),
	)

	// 변환 실행
	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	// 출력 페이로드 검증
	resultMap := result.Payload().ToMap()

	// 추출된 필드 확인
	if got, ok := resultMap["device_id"]; !ok || got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got, ok := resultMap["temperature"]; !ok || got != 72.5 {
		t.Errorf("temperature = %v, want 72.5", got)
	}
	if got, ok := resultMap["humidity"]; !ok || got != 45.2 {
		t.Errorf("humidity = %v, want 45.2", got)
	}

	// 불필요한 필드가 제거되었는지 확인
	if _, ok := resultMap["firmware"]; ok {
		t.Error("firmware 필드가 출력에 포함되면 안 된다")
	}
	if _, ok := resultMap["raw_adc"]; ok {
		t.Error("raw_adc 필드가 출력에 포함되면 안 된다")
	}

	// 메타데이터 보존 확인
	if got, ok := result.Metadata().Get("_source"); !ok || got != "mqtt-agent" {
		t.Errorf("metadata._source = %v, want \"mqtt-agent\"", got)
	}
}

func TestCompileExpression_MessageFields(t *testing.T) {
	fn, err := compileExpression("{ msg_id: $.id, source: $.metadata._source, temp: $.payload.temperature }")
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"temperature": 25.0,
		})),
		message.WithMetadata("_source", "mqtt-agent"),
	)

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()

	// $.id — 메시지 ID 접근
	if got, ok := resultMap["msg_id"]; !ok || got == nil || got == "" {
		t.Errorf("msg_id = %v, want non-empty string (message ID)", got)
	}
	// ID가 원본 메시지의 ID와 일치하는지 확인
	if got := resultMap["msg_id"]; got != inputMsg.ID() {
		t.Errorf("msg_id = %v, want %v", got, inputMsg.ID())
	}

	// $.metadata._source — 메타데이터 접근
	if got, ok := resultMap["source"]; !ok || got != "mqtt-agent" {
		t.Errorf("source = %v, want \"mqtt-agent\"", got)
	}

	// $.payload.temperature — 페이로드 접근
	if got, ok := resultMap["temp"]; !ok || got != 25.0 {
		t.Errorf("temp = %v, want 25.0", got)
	}
}

func TestCompileExpression_Timestamp(t *testing.T) {
	fn, err := compileExpression("{ ts: $.timestamp }")
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New()

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()
	if got, ok := resultMap["ts"]; !ok || got == nil || got == "" {
		t.Errorf("ts = %v, want non-empty timestamp string", got)
	}
}

func TestCompileExpression_MissingPath(t *testing.T) {
	fn, err := compileExpression("{ value: $.payload.nonexistent.path }")
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"existing": "data",
	})))

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	// 존재하지 않는 경로는 nil로 설정된다
	resultMap := result.Payload().ToMap()
	if val, ok := resultMap["value"]; ok && val != nil {
		t.Errorf("value = %v, want nil", val)
	}
}

func TestCompileExpression_NestedPath(t *testing.T) {
	fn, err := compileExpression("{ location: $.payload.deviceInfo.tags.location }")
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"deviceInfo": map[string]any{
			"devEui": "0018B20000000001",
			"tags": map[string]any{
				"location": "factory-A/line-3",
			},
		},
	})))

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()
	if got, ok := resultMap["location"]; !ok || got != "factory-A/line-3" {
		t.Errorf("location = %v, want \"factory-A/line-3\"", got)
	}
}

func TestTransformNode_Configure_Expression(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// expression 설정
	err = tn.Configure(map[string]any{
		"expression": "{ temp: $.payload.data.temperature }",
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// transformFn이 설정되었는지 확인
	tn.mu.RLock()
	hasFn := tn.transformFn != nil
	tn.mu.RUnlock()
	if !hasFn {
		t.Fatal("Configure(expression) 후 transformFn이 nil이다")
	}
}

func TestTransformNode_Configure_TransformFuncPriority(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// TransformFunc와 expression을 동시에 설정
	called := false
	err = tn.Configure(map[string]any{
		"transform": TransformFunc(func(msg message.Message) (message.Message, error) {
			called = true
			return msg, nil
		}),
		"expression": "{ temp: $.payload.data.temperature }",
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// TransformFunc가 우선적으로 사용되어야 한다
	inputMsg := message.New()
	_, _ = tn.Process(nil, inputMsg)
	if !called {
		t.Error("TransformFunc가 expression보다 우선순위가 높아야 한다")
	}
}

func TestTransformNode_Configure_InvalidExpression(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	err = tn.Configure(map[string]any{
		"expression": "invalid expression",
	})
	if err == nil {
		t.Error("잘못된 expression에 대해 에러가 반환되어야 한다")
	}
}

func TestTransformNode_Process_WithExpression(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	err = tn.Configure(map[string]any{
		"expression": "{ device_id: $.payload.device_id, temperature: $.payload.object.temperature }",
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// 입력 메시지 (불필요한 필드 포함)
	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id": "sensor-001",
		"object": map[string]any{
			"temperature": 72.5,
			"humidity":    45.2,
		},
		"firmware": "v2.1.0",
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	// 출력 검증: device_id와 temperature만 포함
	outputMap := results[0].Payload().ToMap()
	if got := outputMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got := outputMap["temperature"]; got != 72.5 {
		t.Errorf("temperature = %v, want 72.5", got)
	}

	// 불필요한 필드 제거 확인
	if _, ok := outputMap["firmware"]; ok {
		t.Error("firmware 필드가 출력에 포함되면 안 된다")
	}
	if _, ok := outputMap["object"]; ok {
		t.Error("object 필드가 출력에 포함되면 안 된다")
	}
}

func TestMessageToMap(t *testing.T) {
	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"temperature": 25.0,
		})),
		message.WithMetadata("_source", "test"),
		message.WithMetadata("node_id", "node-1"),
	)

	m := messageToMap(msg)

	// id 확인
	if id, ok := m["id"].(string); !ok || id == "" {
		t.Error("messageToMap() id가 비어있다")
	}

	// timestamp 확인
	if ts, ok := m["timestamp"].(string); !ok || ts == "" {
		t.Error("messageToMap() timestamp가 비어있다")
	}

	// payload 확인
	payload, ok := m["payload"].(map[string]any)
	if !ok {
		t.Fatal("messageToMap() payload가 map[string]any 타입이 아니다")
	}
	if temp := payload["temperature"]; temp != 25.0 {
		t.Errorf("messageToMap() payload.temperature = %v, want 25.0", temp)
	}

	// metadata 확인
	metadata, ok := m["metadata"].(map[string]any)
	if !ok {
		t.Fatal("messageToMap() metadata가 map[string]any 타입이 아니다")
	}
	if src := metadata["_source"]; src != "test" {
		t.Errorf("messageToMap() metadata._source = %v, want \"test\"", src)
	}
	if nid := metadata["node_id"]; nid != "node-1" {
		t.Errorf("messageToMap() metadata.node_id = %v, want \"node-1\"", nid)
	}
}
