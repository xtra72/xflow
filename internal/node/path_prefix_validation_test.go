package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
)

// 이 파일은 SINGLE message-field-path 선택자(deduplicate.key / deduplicate.compare_fields /
// storage-write.value_key)에 대해 `$.` prefix 를 강제하는 Configure-time 검증을 검증한다.
// key_template / store-read 배치 변수({item}) 는 단일 경로 선택자가 아니므로 영향받지 않는다.

// ---------------------------------------------------------------------------
// deduplicate.key — `$.` prefix 강제
// ---------------------------------------------------------------------------

// 빈 key 는 허용 (전체 메시지 기준 = "_all").
func TestDeduplicate_Configure_EmptyKey_OK(t *testing.T) {
	def := flow.NewNodeDef("dedup-empty-key", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"window": "30s",
		// key 미지정 → _all 허용
	})
	require.NoError(t, err)
}

// $.-경로 key 는 허용.
func TestDeduplicate_Configure_DollarPathKey_OK(t *testing.T) {
	def := flow.NewNodeDef("dedup-dollar-key", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":    "$.payload.idu_num",
		"window": "30s",
	})
	require.NoError(t, err)
}

// bare name key 는 Configure 에서 명시적 에러.
func TestDeduplicate_Configure_BareKey_Error(t *testing.T) {
	def := flow.NewNodeDef("dedup-bare-key", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key")
	assert.Contains(t, err.Error(), "idu_num")
	assert.Contains(t, err.Error(), "$.")
}

// ---------------------------------------------------------------------------
// deduplicate.compare_fields — `$.` prefix 강제 (string + array 형식)
// ---------------------------------------------------------------------------

// 빈 compare_fields (allFields 모드) 는 허용.
func TestDeduplicate_Configure_EmptyCompareFields_OK(t *testing.T) {
	def := flow.NewNodeDef("dedup-empty-cmp", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":    "$.payload.idu_num",
		"window": "30s",
		// compare_fields 미지정 → 전체 payload 비교 (allFields)
	})
	require.NoError(t, err)
}

// $.-경로 compare_fields (string 형식) 는 허용.
func TestDeduplicate_Configure_DollarCompareFieldsString_OK(t *testing.T) {
	def := flow.NewNodeDef("dedup-dollar-cmp-str", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":            "$.payload.idu_num",
		"window":         "30s",
		"compare_fields": "$.payload.current_temperature:0.5, $.payload.target_temperature",
	})
	require.NoError(t, err)
}

// $.-경로 compare_fields (array 형식) 는 허용.
func TestDeduplicate_Configure_DollarCompareFieldsArray_OK(t *testing.T) {
	def := flow.NewNodeDef("dedup-dollar-cmp-arr", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":    "$.payload.idu_num",
		"window": "30s",
		"compare_fields": []any{
			map[string]any{"name": "$.payload.current_temperature", "tolerance": 0.5},
			map[string]any{"name": "$.metadata.device.type"},
		},
	})
	require.NoError(t, err)
}

// bare compare_fields 엔트리 (string 형식) 는 Configure 에서 명시적 에러.
func TestDeduplicate_Configure_BareCompareFieldsString_Error(t *testing.T) {
	def := flow.NewNodeDef("dedup-bare-cmp-str", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":            "$.payload.idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature, target_temperature",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compare_fields")
	assert.Contains(t, err.Error(), "current_temperature")
	assert.Contains(t, err.Error(), "$.")
}

// bare compare_fields 엔트리 (array 형식) 는 Configure 에서 명시적 에러.
func TestDeduplicate_Configure_BareCompareFieldsArray_Error(t *testing.T) {
	def := flow.NewNodeDef("dedup-bare-cmp-arr", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))

	err = n.Configure(map[string]any{
		"key":    "$.payload.idu_num",
		"window": "30s",
		"compare_fields": []any{
			map[string]any{"name": "current_temperature", "tolerance": 0.5},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compare_fields")
	assert.Contains(t, err.Error(), "current_temperature")
	assert.Contains(t, err.Error(), "$.")
}
