package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- MappingNode 인터페이스 준수 ---

var _ Node = (*MappingNode)(nil)

// --- NewMappingNode 테스트 ---

// TestNewMappingNode_정상생성 은 MappingNode가 올바르게 생성되는지 확인한다.
func TestNewMappingNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("mapping-1", "mapping")
	node, err := NewMappingNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "mapping-1", node.Name())
	assert.Equal(t, "mapping", node.Type())
}

// --- Init 테스트 ---

// TestMappingNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestMappingNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("mapping-init", "mapping")
	node, _ := NewMappingNode(def)
	mn := node.(*MappingNode)

	err := mn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, mn.CurrentState())
}

// --- Configure 테스트 ---

// TestMappingNode_Configure_정상 은 유효한 설정이 성공하는지 확인한다.
func TestMappingNode_Configure_정상(t *testing.T) {
	def := flow.NewNodeDef("mapping-cfg", "mapping")
	node, _ := NewMappingNode(def)

	err := node.Configure(map[string]any{
		"field": "$.payload.status_code",
		"mappings": map[string]any{
			"1": "활성",
			"2": "비활성",
		},
	})
	require.NoError(t, err)
}

// TestMappingNode_Configure_field없음_에러 는 field 미설정 시 ErrInvalidConfig를 반환하는지 확인한다.
func TestMappingNode_Configure_field없음_에러(t *testing.T) {
	def := flow.NewNodeDef("mapping-no-field", "mapping")
	node, _ := NewMappingNode(def)

	err := node.Configure(map[string]any{
		"mappings": map[string]any{"1": "one"},
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestMappingNode_Configure_mappings없음_에러 는 mappings 미설정 시 ErrInvalidConfig를 반환하는지 확인한다.
func TestMappingNode_Configure_mappings없음_에러(t *testing.T) {
	def := flow.NewNodeDef("mapping-no-mappings", "mapping")
	node, _ := NewMappingNode(def)

	err := node.Configure(map[string]any{
		"field": "$.payload.code",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestMappingNode_Configure_빈mappings_에러 는 빈 mappings 설정 시 ErrInvalidConfig를 반환하는지 확인한다.
func TestMappingNode_Configure_빈mappings_에러(t *testing.T) {
	def := flow.NewNodeDef("mapping-empty-mappings", "mapping")
	node, _ := NewMappingNode(def)

	err := node.Configure(map[string]any{
		"field":    "$.payload.code",
		"mappings": map[string]any{},
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// --- Process 테스트 ---

// TestMappingNode_Process 는 다양한 매핑 시나리오를 테이블 기반으로 테스트한다.
func TestMappingNode_Process(t *testing.T) {
	tests := []struct {
		name        string
		field       string
		mappings    map[string]any
		defaultVal  any
		hasDefault  bool
		target      string
		payload     map[string]any
		wantErr     error
		wantKey     string // 결과를 확인할 payload 키
		wantVal     any    // 기대하는 결과 값
	}{
		{
			name:     "매칭키_매핑값_반환",
			field:    "$.payload.status_code",
			mappings: map[string]any{"1": "활성", "2": "비활성"},
			payload:  map[string]any{"status_code": "1"},
			wantKey:  "status_code",
			wantVal:  "활성",
		},
		{
			name:       "미매칭키_기본값_반환",
			field:      "$.payload.status_code",
			mappings:   map[string]any{"1": "활성"},
			defaultVal: "알수없음",
			hasDefault: true,
			payload:    map[string]any{"status_code": "999"},
			wantKey:    "status_code",
			wantVal:    "알수없음",
		},
		{
			name:     "미매칭키_기본값없음_ErrMappingKeyNotFound",
			field:    "$.payload.status_code",
			mappings: map[string]any{"1": "활성"},
			payload:  map[string]any{"status_code": "999"},
			wantErr:  ErrMappingKeyNotFound,
		},
		{
			name:     "소스필드없음_ErrMappingFieldNotFound",
			field:    "$.payload.missing_field",
			mappings: map[string]any{"1": "활성"},
			payload:  map[string]any{"other": "value"},
			wantErr:  ErrMappingFieldNotFound,
		},
		{
			name:     "target지정_target필드에_기록",
			field:    "$.payload.code",
			mappings: map[string]any{"A": "Alpha", "B": "Beta"},
			target:   "code_name",
			payload:  map[string]any{"code": "A"},
			wantKey:  "code_name",
			wantVal:  "Alpha",
		},
		{
			name:     "target미지정_소스필드_덮어쓰기",
			field:    "$.payload.code",
			mappings: map[string]any{"A": "Alpha"},
			payload:  map[string]any{"code": "A"},
			wantKey:  "code",
			wantVal:  "Alpha",
		},
		{
			name:     "숫자키_int를_문자열로_변환",
			field:    "$.payload.level",
			mappings: map[string]any{"42": "정답"},
			payload:  map[string]any{"level": 42},
			wantKey:  "level",
			wantVal:  "정답",
		},
		{
			name:     "불리언키_bool을_문자열로_변환",
			field:    "$.payload.flag",
			mappings: map[string]any{"true": "참", "false": "거짓"},
			payload:  map[string]any{"flag": true},
			wantKey:  "flag",
			wantVal:  "참",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("mapping-process", "mapping")
			node, _ := NewMappingNode(def)
			mn := node.(*MappingNode)

			// 설정 직접 적용 (Configure를 우회하여 테스트 단순화)
			mn.field = tt.field
			mn.mappings = tt.mappings
			mn.defaultValue = tt.defaultVal
			mn.hasDefault = tt.hasDefault
			mn.target = tt.target

			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			results, err := mn.Process(context.Background(), msg)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, results)
			} else {
				require.NoError(t, err)
				require.Len(t, results, 1)
				val, ok := results[0].Payload().Get(tt.wantKey)
				assert.True(t, ok, "결과 페이로드에 키 %q가 있어야 한다", tt.wantKey)
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}

// --- Shutdown 테스트 ---

// TestMappingNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestMappingNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("mapping-shut", "mapping")
	node, _ := NewMappingNode(def)
	mn := node.(*MappingNode)

	// Init으로 Running 상태가 되어야 Stopping으로 전이 가능
	_ = mn.Init(context.Background())
	err := mn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, mn.CurrentState())
}

// --- lastPathKey 테스트 ---

// TestLastPathKey 는 lastPathKey가 JSONPath에서 마지막 키를 올바르게 추출하는지 확인한다.
func TestLastPathKey(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"JSONPath_마지막키", "$.payload.status_code", "status_code"},
		{"2단계경로", "$.payload.data", "data"},
		{"점없는경로", "field_name", "field_name"},
		{"달러점으로시작", "$.field", "field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastPathKey(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}
