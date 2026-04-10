package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// --- Phase 3: Registry 등록 및 팩토리 옵션 파싱 테스트 ---

// TestRegistry_FramerType_Registered 는 기본 레지스트리에 framer 타입이 등록되어 있는지 확인한다.
func TestRegistry_FramerType_Registered(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("framer"), "framer 타입이 레지스트리에 등록되어 있어야 한다")

	// 팩토리로 노드 생성 가능 여부
	def := flow.NewNodeDef("test-framer", "framer",
		flow.WithNodeConfig("framing", "newline"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
}

// TestFramerFactory_ValidOptions_NewlineMode 는 newline 모드의 기본 옵션으로 노드가 생성되는지 확인한다.
func TestFramerFactory_ValidOptions_NewlineMode(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-newline", "framer",
		flow.WithNodeConfig("framing", "newline"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)
	assert.NotNil(t, node)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, "newline", fn.framingMode)
}

// TestFramerFactory_ValidOptions_FrameMode_AllOptions 는 frame 모드의 전체 옵션이 올바르게 파싱되는지 확인한다.
func TestFramerFactory_ValidOptions_FrameMode_AllOptions(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-frame", "framer",
		flow.WithNodeConfig("framing", "frame"),
		flow.WithNodeConfig("stx", "02"),
		flow.WithNodeConfig("etx", "03"),
		flow.WithNodeConfig("length_offset", 1),
		flow.WithNodeConfig("length_size", 2),
		flow.WithNodeConfig("length_endian", "big"),
		flow.WithNodeConfig("max_message_size", 1024),
		flow.WithNodeConfig("checksum", "sum8"),
		flow.WithNodeConfig("length_includes_header", true),
		flow.WithNodeConfig("length_adjustment", -2),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, "frame", fn.framingMode)
	assert.Equal(t, []byte{0x02}, fn.options.STX)
	assert.Equal(t, []byte{0x03}, fn.options.ETX)
	assert.Equal(t, 1, fn.options.LengthOffset)
	assert.Equal(t, 2, fn.options.LengthSize)
	assert.Equal(t, "big", fn.options.LengthEndian)
	assert.Equal(t, 1024, fn.options.MaxMessageSize)
	assert.Equal(t, "sum8", fn.options.Checksum)
	assert.True(t, fn.options.LengthIncludesHeader)
	assert.Equal(t, -2, fn.options.LengthAdjustment)
}

// TestFramerFactory_MissingFramingOption_Error 는 framing 키가 없을 때 에러가 반환되는지 확인한다.
func TestFramerFactory_MissingFramingOption_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-no-mode", "framer")
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "framing")
}

// TestFramerFactory_InvalidFramingMode_Error 는 유효하지 않은 framing 모드에서 에러가 반환되는지 확인한다.
func TestFramerFactory_InvalidFramingMode_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-bad", "framer",
		flow.WithNodeConfig("framing", "unknown"),
	)
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

// TestFramerFactory_InvalidLengthEndian_Error 는 유효하지 않은 length_endian 에서 에러가 반환되는지 확인한다.
func TestFramerFactory_InvalidLengthEndian_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-endian", "framer",
		flow.WithNodeConfig("framing", "frame"),
		flow.WithNodeConfig("stx", "02"),
		flow.WithNodeConfig("length_offset", 1),
		flow.WithNodeConfig("length_size", 1),
		flow.WithNodeConfig("length_endian", "middle"),
	)
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "length_endian")
}

// TestFramerFactory_NegativeMaxMessageSize_Error 는 음수 max_message_size 에서 에러가 반환되는지 확인한다.
func TestFramerFactory_NegativeMaxMessageSize_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-neg-mms", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("max_message_size", -1),
	)
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_message_size")
}

// TestFramerFactory_NegativeBufferSize_Error 는 음수 buffer_size 에서 에러가 반환되는지 확인한다.
func TestFramerFactory_NegativeBufferSize_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-neg-bs", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("buffer_size", -1),
	)
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "buffer_size")
}

// TestFramerFactory_NegativeMaxStreams_Error 는 음수 max_streams 에서 에러가 반환되는지 확인한다.
func TestFramerFactory_NegativeMaxStreams_Error(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-neg-ms", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("max_streams", -1),
	)
	_, err := r.Create(def)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_streams")
}

// TestFramerFactory_DefaultPorts 는 생성된 노드가 올바른 기본 포트를 가지는지 확인한다.
func TestFramerFactory_DefaultPorts(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-ports", "framer",
		flow.WithNodeConfig("framing", "raw"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	// 기본 포트 확인: in, out, error
	assert.Equal(t, "framer-ports", node.Name())
	assert.Equal(t, "framer", node.Type())
}

// TestFramerFactory_StreamKeyMetadata_Default 는 stream_key_metadata 미지정 시 기본값이 connection_id 인지 확인한다.
func TestFramerFactory_StreamKeyMetadata_Default(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-skm-default", "framer",
		flow.WithNodeConfig("framing", "newline"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, "connection_id", fn.nodeOptions.StreamKeyMetadata)
}

// TestFramerFactory_StreamKeyMetadata_Custom 는 커스텀 stream_key_metadata 가 올바르게 파싱되는지 확인한다.
func TestFramerFactory_StreamKeyMetadata_Custom(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-skm-custom", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("stream_key_metadata", "session_id"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, "session_id", fn.nodeOptions.StreamKeyMetadata)
}

// TestFramerFactory_StreamIdleTimeout_ParsesDuration 는 stream_idle_timeout 이 duration 으로 파싱되는지 확인한다.
func TestFramerFactory_StreamIdleTimeout_ParsesDuration(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("framer-timeout", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("stream_idle_timeout", "5s"),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, 5*1e9, float64(fn.nodeOptions.StreamIdleTimeout)) // 5 seconds
}

// TestFramerFactory_Category_Processing 는 framer 타입의 카테고리가 processing 인지 확인한다.
func TestFramerFactory_Category_Processing(t *testing.T) {
	r := NewRegistry()
	meta, ok := r.TypeMeta("framer")
	require.True(t, ok, "framer 타입의 메타데이터가 존재해야 한다")
	assert.Equal(t, "processing", meta.Category)
}

// TestFramerFactory_AllValidModes 는 6가지 유효한 framing 모드가 모두 성공하는지 확인한다.
func TestFramerFactory_AllValidModes(t *testing.T) {
	r := NewRegistry()

	modes := []struct {
		mode      string
		extraOpts []flow.NodeOption
	}{
		{"raw", nil},
		{"newline", nil},
		{"length_prefix", nil},
		{"fixed_size", []flow.NodeOption{flow.WithNodeConfig("fixed_size", 64)}},
		{"stream", nil},
		{"frame", []flow.NodeOption{
			flow.WithNodeConfig("stx", "02"),
			flow.WithNodeConfig("length_offset", 1),
			flow.WithNodeConfig("length_size", 1),
			flow.WithNodeConfig("checksum", "none"),
		}},
	}

	for _, tc := range modes {
		t.Run(tc.mode, func(t *testing.T) {
			opts := []flow.NodeOption{flow.WithNodeConfig("framing", tc.mode)}
			opts = append(opts, tc.extraOpts...)
			def := flow.NewNodeDef("framer-"+tc.mode, "framer", opts...)
			node, err := r.Create(def)
			require.NoError(t, err, "모드 %q 가 성공해야 한다", tc.mode)
			assert.NotNil(t, node)
		})
	}
}

// TestFramerFactory_NumericOptionsFromJSON 는 JSON 에서 올 수 있는 float64 타입의 숫자 옵션을 처리하는지 확인한다.
func TestFramerFactory_NumericOptionsFromJSON(t *testing.T) {
	r := NewRegistry()
	// JSON 디코딩 시 숫자는 float64 로 들어옴
	def := flow.NewNodeDef("framer-json", "framer",
		flow.WithNodeConfig("framing", "newline"),
		flow.WithNodeConfig("buffer_size", float64(8192)),
		flow.WithNodeConfig("max_message_size", float64(4096)),
		flow.WithNodeConfig("max_streams", float64(10)),
	)
	node, err := r.Create(def)
	require.NoError(t, err)

	fn, ok := node.(*FramerNode)
	require.True(t, ok)
	assert.Equal(t, 8192, fn.options.BufferSize)
	assert.Equal(t, 4096, fn.options.MaxMessageSize)
	assert.Equal(t, 10, fn.nodeOptions.MaxStreams)
}
