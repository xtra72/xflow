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
	fn, err := compileExpression("{ device_id: $.payload.device_id, temperature: $.payload.object.temperature, humidity: $.payload.object.humidity }", TransformModeSelect)
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
	fn, err := compileExpression("{ msg_id: $.id, source: $.metadata._source, temp: $.payload.temperature }", TransformModeSelect)
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
	fn, err := compileExpression("{ ts: $.timestamp }", TransformModeSelect)
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
	fn, err := compileExpression("{ value: $.payload.nonexistent.path }", TransformModeSelect)
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
	fn, err := compileExpression("{ location: $.payload.deviceInfo.tags.location }", TransformModeSelect)
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

func TestCompileExpression_MergeMode(t *testing.T) {
	fn, err := compileExpression("{ temperature_f: $.payload.temperature }", TransformModeMerge)
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id":   "sensor-001",
		"temperature": 72.5,
		"humidity":    45.2,
		"firmware":    "v2.1.0",
	})))

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()

	// merge 모드: expression 결과가 추가된다
	if got, ok := resultMap["temperature_f"]; !ok || got != 72.5 {
		t.Errorf("temperature_f = %v, want 72.5", got)
	}

	// merge 모드: 원본 필드가 보존된다
	if got, ok := resultMap["device_id"]; !ok || got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got, ok := resultMap["humidity"]; !ok || got != 45.2 {
		t.Errorf("humidity = %v, want 45.2", got)
	}
	if got, ok := resultMap["firmware"]; !ok || got != "v2.1.0" {
		t.Errorf("firmware = %v, want \"v2.1.0\"", got)
	}
}

func TestCompileExpression_MergeMode_Overwrite(t *testing.T) {
	// merge 모드에서 기존 필드를 덮어쓰는 경우
	fn, err := compileExpression("{ temperature: $.payload.humidity }", TransformModeMerge)
	if err != nil {
		t.Fatalf("compileExpression() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 72.5,
		"humidity":    45.2,
	})))

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()

	// temperature가 humidity 값으로 덮어쓰기된다
	if got := resultMap["temperature"]; got != 45.2 {
		t.Errorf("temperature = %v, want 45.2 (overwritten by humidity)", got)
	}
	// humidity는 원본 그대로 보존
	if got := resultMap["humidity"]; got != 45.2 {
		t.Errorf("humidity = %v, want 45.2", got)
	}
}

func TestTransformNode_Process_MergeMode(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	err = tn.Configure(map[string]any{
		"expression": "{ temp_c: $.payload.temperature }",
		"mode":       "merge",
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id":   "sensor-001",
		"temperature": 72.5,
		"firmware":    "v2.1.0",
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// expression 결과 추가
	if got := outputMap["temp_c"]; got != 72.5 {
		t.Errorf("temp_c = %v, want 72.5", got)
	}
	// 원본 필드 보존
	if got := outputMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got := outputMap["firmware"]; got != "v2.1.0" {
		t.Errorf("firmware = %v, want \"v2.1.0\"", got)
	}
}

func TestCompileExclude(t *testing.T) {
	fn, err := compileExclude("firmware, raw_adc")
	if err != nil {
		t.Fatalf("compileExclude() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id":   "sensor-001",
		"temperature": 72.5,
		"humidity":    45.2,
		"firmware":    "v2.1.0",
		"raw_adc":     []any{1024.0, 2048.0},
	})))

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()

	// 제거된 필드 확인
	if _, ok := resultMap["firmware"]; ok {
		t.Error("firmware 필드가 제거되어야 한다")
	}
	if _, ok := resultMap["raw_adc"]; ok {
		t.Error("raw_adc 필드가 제거되어야 한다")
	}

	// 보존된 필드 확인
	if got := resultMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got := resultMap["temperature"]; got != 72.5 {
		t.Errorf("temperature = %v, want 72.5", got)
	}
	if got := resultMap["humidity"]; got != 45.2 {
		t.Errorf("humidity = %v, want 45.2", got)
	}
}

func TestCompileExclude_EmptyFields(t *testing.T) {
	_, err := compileExclude("")
	if err == nil {
		t.Error("빈 exclude 필드에 대해 에러가 반환되어야 한다")
	}
}

func TestCompileExpressionPipeline(t *testing.T) {
	// 파이프라인: exclude → merge
	steps := []expressionStep{
		{mode: TransformModeExclude, value: "firmware, raw_adc"},
		{mode: TransformModeMerge, value: "{ source: $.metadata._source }"},
	}
	fn, err := compileExpressionPipeline(steps, nil)
	if err != nil {
		t.Fatalf("compileExpressionPipeline() error = %v", err)
	}

	inputMsg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"device_id":   "sensor-001",
			"temperature": 72.5,
			"firmware":    "v2.1.0",
			"raw_adc":     []any{1024.0},
		})),
		message.WithMetadata("_source", "mqtt-agent"),
	)

	result, err := fn(inputMsg)
	if err != nil {
		t.Fatalf("TransformFunc() error = %v", err)
	}

	resultMap := result.Payload().ToMap()

	// exclude 단계: firmware, raw_adc 제거
	if _, ok := resultMap["firmware"]; ok {
		t.Error("firmware 필드가 제거되어야 한다")
	}
	if _, ok := resultMap["raw_adc"]; ok {
		t.Error("raw_adc 필드가 제거되어야 한다")
	}

	// 원본 필드 보존
	if got := resultMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got := resultMap["temperature"]; got != 72.5 {
		t.Errorf("temperature = %v, want 72.5", got)
	}

	// merge 단계: metadata._source가 payload에 추가
	if got := resultMap["source"]; got != "mqtt-agent" {
		t.Errorf("source = %v, want \"mqtt-agent\"", got)
	}
}

func TestTransformNode_Configure_Pipeline(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// 배열 형식 expression 설정
	err = tn.Configure(map[string]any{
		"expression": []any{
			map[string]any{"exclude": "firmware, raw_adc"},
			map[string]any{"merge": "{ source: $.metadata._source }"},
		},
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	inputMsg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{
			"device_id":   "sensor-001",
			"temperature": 72.5,
			"firmware":    "v2.1.0",
			"raw_adc":     []any{1024.0},
		})),
		message.WithMetadata("_source", "mqtt-agent"),
	)

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// exclude: firmware, raw_adc 제거
	if _, ok := outputMap["firmware"]; ok {
		t.Error("firmware 필드가 제거되어야 한다")
	}
	if _, ok := outputMap["raw_adc"]; ok {
		t.Error("raw_adc 필드가 제거되어야 한다")
	}
	// 보존
	if got := outputMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
	if got := outputMap["temperature"]; got != 72.5 {
		t.Errorf("temperature = %v, want 72.5", got)
	}
	// merge: source 추가
	if got := outputMap["source"]; got != "mqtt-agent" {
		t.Errorf("source = %v, want \"mqtt-agent\"", got)
	}
}

func TestTransformNode_Configure_ExcludeString(t *testing.T) {
	def := flow.NewNodeDef("test-transform", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// 단일 문자열 + mode: "exclude" (하위 호환 형식)
	err = tn.Configure(map[string]any{
		"expression": "firmware, raw_adc",
		"mode":       "exclude",
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id":   "sensor-001",
		"temperature": 72.5,
		"firmware":    "v2.1.0",
		"raw_adc":     []any{1024.0},
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	outputMap := results[0].Payload().ToMap()
	if _, ok := outputMap["firmware"]; ok {
		t.Error("firmware 필드가 제거되어야 한다")
	}
	if got := outputMap["device_id"]; got != "sensor-001" {
		t.Errorf("device_id = %v, want \"sensor-001\"", got)
	}
}

func TestParseExpressionSteps_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		steps []any
	}{
		{"빈 배열", []any{}},
		{"맵이 아닌 요소", []any{"invalid"}},
		{"키가 여러 개인 맵", []any{map[string]any{"select": "{ a: $.payload.a }", "merge": "{ b: $.payload.b }"}}},
		{"값이 문자열이 아닌 맵", []any{map[string]any{"select": 123}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseExpressionSteps(tt.steps)
			if err == nil {
				t.Error("parseExpressionSteps() expected error, got nil")
			}
		})
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

	// v0.16.2: timestamp 는 epoch ms (int64).
	if ts, ok := m["timestamp"].(int64); !ok || ts <= 0 {
		t.Errorf("messageToMap() timestamp 는 양수 epoch ms (int64) 여야 함: got %T %v", m["timestamp"], m["timestamp"])
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

// =============================================================================
// TransformNode Configure 통합 테스트 (v2 연동)
// =============================================================================

// TestTransformNode_Configure_VariableBinding 은 config에서 변수 바인딩을
// 수집하여 expression에서 사용할 수 있는지 확인한다 (AC-14).
func TestTransformNode_Configure_VariableBinding(t *testing.T) {
	def := flow.NewNodeDef("test-var-bind", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// address_table 변수가 config에 포함된 expression 설정
	config := map[string]any{
		"address_table": map[string]any{
			"A:1F:L1": 0,
			"A:1F:L2": 8,
		},
		"expression": `{ base: $address_table["A:1F:L1"] }`,
	}

	err = tn.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// 메시지 생성 및 변환 실행
	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device_id": "sensor-001",
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// base == 0 확인 (address_table["A:1F:L1"]의 값)
	base, ok := outputMap["base"]
	if !ok {
		t.Fatal("base 필드가 출력에 포함되어야 한다")
	}
	// JSON에서 숫자는 float64로 파싱되지만, Go map에서 직접 설정한 int는 int로 유지될 수 있다.
	// toFloat64로 비교한다.
	baseF, baseOk := toFloat64(base)
	if !baseOk {
		t.Fatalf("base = %v (%T), 숫자 타입이어야 한다", base, base)
	}
	if baseF != 0 {
		t.Errorf("base = %v, want 0", baseF)
	}
}

// TestTransformNode_Configure_MqttToModbus_AddressResolver 는 mqtt-to-modbus
// 시나리오의 address-resolver 노드를 시뮬레이션한다 (AC-15).
func TestTransformNode_Configure_MqttToModbus_AddressResolver(t *testing.T) {
	def := flow.NewNodeDef("address-resolver", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	// mqtt-to-modbus.yaml의 address-resolver 노드 설정 시뮬레이션
	config := map[string]any{
		"address_table": map[string]any{
			"창고:서버 옆":   0,
			"실습실:전방 우측": 8,
		},
		"expression": `{
			payload: $.payload,
			_base: $address_table[
				$.payload.deviceInfo.tags.location & ":" & $.payload.deviceInfo.tags.point
			]
		}`,
	}

	err = tn.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// LoRaWAN 메시지 시뮬레이션
	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"deviceInfo": map[string]any{
			"devEui": "a1b2c3d4e5f60001",
			"tags": map[string]any{
				"location": "창고",
				"point":    "서버 옆",
			},
		},
		"object": map[string]any{
			"temperature": 23.5,
			"humidity":    65.0,
		},
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// payload가 보존되었는지 확인
	payload, ok := outputMap["payload"].(map[string]any)
	if !ok {
		t.Fatal("payload 필드가 map[string]any 타입이어야 한다")
	}
	deviceInfo, ok := payload["deviceInfo"].(map[string]any)
	if !ok {
		t.Fatal("payload.deviceInfo가 map[string]any 타입이어야 한다")
	}
	if devEui := deviceInfo["devEui"]; devEui != "a1b2c3d4e5f60001" {
		t.Errorf("payload.deviceInfo.devEui = %v, want \"a1b2c3d4e5f60001\"", devEui)
	}

	// _base == 0 확인 ("창고" & ":" & "서버 옆" = "창고:서버 옆" → 0)
	base, ok := outputMap["_base"]
	if !ok {
		t.Fatal("_base 필드가 출력에 포함되어야 한다")
	}
	baseF, baseOk := toFloat64(base)
	if !baseOk {
		t.Fatalf("_base = %v (%T), 숫자 타입이어야 한다", base, base)
	}
	if baseF != 0 {
		t.Errorf("_base = %v, want 0", baseF)
	}
}

// TestTransformNode_Configure_Arithmetic 은 expression에서 산술 연산이
// 올바르게 동작하는지 확인한다 (AC-15).
func TestTransformNode_Configure_Arithmetic(t *testing.T) {
	def := flow.NewNodeDef("test-arithmetic", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	config := map[string]any{
		"expression": `{
			command: "set_input",
			params: {
				area: "input_registers",
				address: $.payload._base + 0,
				value: $.payload.object.temperature,
				data_type: "float32"
			}
		}`,
	}

	err = tn.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// _base: 8 이 포함된 메시지로 처리
	// messageToMap은 payload를 $.payload 하위에 매핑하므로 $.payload._base로 접근
	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"_base": 8,
		"object": map[string]any{
			"temperature": 23.5,
		},
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// command 확인
	if got := outputMap["command"]; got != "set_input" {
		t.Errorf("command = %v, want \"set_input\"", got)
	}

	// params 확인
	params, ok := outputMap["params"].(map[string]any)
	if !ok {
		t.Fatal("params가 map[string]any 타입이어야 한다")
	}
	if got := params["area"]; got != "input_registers" {
		t.Errorf("params.area = %v, want \"input_registers\"", got)
	}

	// address == 8 (float64) 확인: $._base(=8) + 0 = 8
	addrF, addrOk := toFloat64(params["address"])
	if !addrOk {
		t.Fatalf("params.address = %v (%T), 숫자 타입이어야 한다", params["address"], params["address"])
	}
	if addrF != 8 {
		t.Errorf("params.address = %v, want 8", addrF)
	}

	// value == 23.5 확인
	valF, valOk := toFloat64(params["value"])
	if !valOk {
		t.Fatalf("params.value = %v (%T), 숫자 타입이어야 한다", params["value"], params["value"])
	}
	if valF != 23.5 {
		t.Errorf("params.value = %v, want 23.5", valF)
	}

	if got := params["data_type"]; got != "float32" {
		t.Errorf("params.data_type = %v, want \"float32\"", got)
	}
}

// TestTransformNode_Configure_FunctionCall 은 expression에서 함수 호출이
// 올바르게 동작하는지 확인한다.
func TestTransformNode_Configure_FunctionCall(t *testing.T) {
	def := flow.NewNodeDef("test-func-call", "transform")
	n, err := NewTransformNode(def)
	if err != nil {
		t.Fatalf("NewTransformNode() error = %v", err)
	}

	tn := n.(*TransformNode)

	config := map[string]any{
		"expression": `{
			device: $.payload.deviceInfo.devEui,
			timestamp: now()
		}`,
	}

	err = tn.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	inputMsg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"deviceInfo": map[string]any{
			"devEui": "a1b2c3d4e5f60001",
		},
	})))

	results, err := tn.Process(nil, inputMsg)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Process() results = %d, want 1", len(results))
	}

	outputMap := results[0].Payload().ToMap()

	// device 확인
	if got := outputMap["device"]; got != "a1b2c3d4e5f60001" {
		t.Errorf("device = %v, want \"a1b2c3d4e5f60001\"", got)
	}

	// timestamp가 비어있지 않은 문자열인지 확인
	ts, ok := outputMap["timestamp"].(string)
	if !ok || ts == "" {
		t.Errorf("timestamp = %v, want non-empty RFC3339Nano string", outputMap["timestamp"])
	}

	// RFC3339Nano 형식인지 간단히 확인 (T와 Z 또는 + 포함)
	if len(ts) < 20 {
		t.Errorf("timestamp = %q, RFC3339Nano 형식이 아닌 것 같다 (길이: %d)", ts, len(ts))
	}
}
