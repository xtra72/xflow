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
			expr:       "{ temperature: $.object.temperature }",
			wantFields: 1,
			wantKeys:   []string{"temperature"},
			wantPaths:  []string{"$.object.temperature"},
		},
		{
			name:       "여러 필드",
			expr:       "{ device_id: $.deviceInfo.devEui, location: $.deviceInfo.tags.location, temperature: $.object.temperature, humidity: $.object.humidity }",
			wantFields: 4,
			wantKeys:   []string{"device_id", "location", "temperature", "humidity"},
			wantPaths:  []string{"$.deviceInfo.devEui", "$.deviceInfo.tags.location", "$.object.temperature", "$.object.humidity"},
		},
		{
			name:       "공백 포함",
			expr:       "{  temp : $.data.temp ,  hum : $.data.hum  }",
			wantFields: 2,
			wantKeys:   []string{"temp", "hum"},
			wantPaths:  []string{"$.data.temp", "$.data.hum"},
		},
		{
			name:       "최상위 필드",
			expr:       "{ name: $.name }",
			wantFields: 1,
			wantKeys:   []string{"name"},
			wantPaths:  []string{"$.name"},
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
		{"중괄호 없음", "temperature: $.temp"},
		{"여는 중괄호만", "{ temperature: $.temp"},
		{"닫는 중괄호만", "temperature: $.temp }"},
		{"빈 중괄호", "{ }"},
		{"콜론 없음", "{ temperature $.temp }"},
		{"빈 키", "{ : $.temp }"},
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
	fn, err := compileExpression("{ device_id: $.device_id, temperature: $.object.temperature, humidity: $.object.humidity }")
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

func TestCompileExpression_MissingPath(t *testing.T) {
	fn, err := compileExpression("{ value: $.nonexistent.path }")
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
	fn, err := compileExpression("{ location: $.deviceInfo.tags.location }")
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
		"expression": "{ temp: $.data.temperature }",
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
		"expression": "{ temp: $.data.temperature }",
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
		"expression": "{ device_id: $.device_id, temperature: $.object.temperature }",
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
