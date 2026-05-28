package node

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// dummyNode 는 테스트용 Node 구현이다.
type dummyNode struct {
	*BaseNode
}

func (d *dummyNode) Init(_ context.Context) error { return nil }
func (d *dummyNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}
func (d *dummyNode) Shutdown(_ context.Context) error { return nil }

// dummyFactory 는 테스트용 NodeFactory이다.
func dummyFactory(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	return &dummyNode{BaseNode: NewBaseNode(def, opts...)}, nil
}

// --- NewRegistry 테스트 ---

// TestNewRegistry_기본생성_빌트인포함 은 기본 레지스트리가 빌트인 노드를 포함하는지 확인한다.
func TestNewRegistry_기본생성_빌트인포함(t *testing.T) {
	r := NewRegistry()

	builtins := []string{"filter", "transform", "switch", "bridge", "script", "catch", "aggregate", "mapping", "modbus", "output", "deadletter", "samsung_hvacr01_status", "samsung_hvacr01_control", "samsung_hvacr01", "mqtt-subscriber", "mqtt-publisher", "modbus-poller", "modbus-writer", "lgap-status", "lgap-control", "lgap", "lgcp-status", "lgcp-control", "lgcp", "tsdb-write", "tsdb-query", "store-write", "store-read", "serial-in", "serial-out", "tcp-in", "tcp-out", "framer"}
	for _, typ := range builtins {
		assert.True(t, r.Has(typ), "빌트인 타입 %q가 등록되어 있어야 한다", typ)
	}
}

// TestNewRegistry_WithoutBuiltins_빈레지스트리 는 WithoutBuiltins 옵션으로 빈 레지스트리를 생성하는지 확인한다.
func TestNewRegistry_WithoutBuiltins_빈레지스트리(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	assert.Empty(t, r.Types())
}

// --- Register 테스트 ---

// TestRegistry_Register_정상 은 팩토리를 등록하고 조회할 수 있는지 확인한다.
func TestRegistry_Register_정상(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	err := r.Register("custom", dummyFactory)
	require.NoError(t, err)
	assert.True(t, r.Has("custom"))
}

// TestRegistry_Register_중복에러 는 이미 등록된 타입을 재등록하면 에러를 반환하는지 확인한다.
func TestRegistry_Register_중복에러(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	err := r.Register("custom", dummyFactory)
	require.NoError(t, err)

	err = r.Register("custom", dummyFactory)
	assert.ErrorIs(t, err, ErrNodeTypeAlreadyRegistered)
}

// --- Create 테스트 ---

// TestRegistry_Create_정상 은 등록된 팩토리로 노드를 생성할 수 있는지 확인한다.
func TestRegistry_Create_정상(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())
	_ = r.Register("custom", dummyFactory)

	def := flow.NewNodeDef("test", "custom")
	node, err := r.Create(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "test", node.Name())
	assert.Equal(t, "custom", node.Type())
}

// TestRegistry_Create_미등록타입에러 는 미등록 타입으로 생성 시 에러를 반환하는지 확인한다.
func TestRegistry_Create_미등록타입에러(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	def := flow.NewNodeDef("test", "unknown")
	_, err := r.Create(def)
	assert.ErrorIs(t, err, ErrNodeTypeNotFound)
}

// TestRegistry_Create_옵션전달 은 Create 시 NodeOption이 팩토리에 전달되는지 확인한다.
func TestRegistry_Create_옵션전달(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	var capturedOpts []NodeOption
	factory := func(def flow.NodeDef, opts ...NodeOption) (Node, error) {
		capturedOpts = opts
		return &dummyNode{BaseNode: NewBaseNode(def, opts...)}, nil
	}
	_ = r.Register("opttest", factory)

	def := flow.NewNodeDef("test", "opttest")
	_, err := r.Create(def, WithLogger(nil))
	require.NoError(t, err)
	assert.Len(t, capturedOpts, 1)
}

// --- Types 테스트 ---

// TestRegistry_Types_정렬 은 Types가 정렬된 목록을 반환하는지 확인한다.
func TestRegistry_Types_정렬(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())
	_ = r.Register("zebra", dummyFactory)
	_ = r.Register("alpha", dummyFactory)
	_ = r.Register("middle", dummyFactory)

	types := r.Types()
	assert.True(t, sort.StringsAreSorted(types))
	assert.Equal(t, []string{"alpha", "middle", "zebra"}, types)
}

// --- Has 테스트 ---

// TestRegistry_Has_존재 은 등록된 타입에 대해 true를 반환하는지 확인한다.
func TestRegistry_Has_존재(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())
	_ = r.Register("test", dummyFactory)

	assert.True(t, r.Has("test"))
	assert.False(t, r.Has("nonexistent"))
}

// --- TypeMeta 테스트 ---

// TestRegistry_TypeMeta_빌트인 은 빌트인 노드의 메타데이터가 올바른지 확인한다.
func TestRegistry_TypeMeta_빌트인(t *testing.T) {
	r := NewRegistry()

	expected := map[string]struct {
		category    string
		description string
		source      string
	}{
		"filter":                  {"processing", "조건에 따라 메시지를 필터링", "builtin"},
		"transform":               {"processing", "메시지 데이터를 변환", "builtin"},
		"switch":                  {"routing", "조건에 따라 메시지를 라우팅", "builtin"},
		"bridge":                  {"io", "외부 에이전트와 메시지 송수신", "builtin"},
		"script":                  {"processing", "스크립트로 메시지를 처리", "builtin"},
		"catch":                   {"error", "에러 메시지를 캐치하여 처리", "builtin"},
		"aggregate":               {"processing", "여러 메시지를 집계", "builtin"},
		"mapping":                 {"processing", "키 기반 값 매핑", "builtin"},
		"output":                  {"io", "메시지를 포맷팅하여 출력", "builtin"},
		"deadletter":              {"error", "처리 실패 메시지를 보관", "builtin"},
		"modbus":                  {"processing", "MODBUS 레지스터 읽기/쓰기", "builtin"},
		"samsung_hvacr01_status":  {"io", "Samsung HVACR-01 (NASA) 디바이스 상태 조회", "builtin"},
		"samsung_hvacr01_control": {"io", "Samsung HVACR-01 (NASA) 디바이스 제어", "builtin"},
		"samsung_hvacr01":         {"io", "Samsung HVACR-01 (NASA) 상태 조회 + 제어 통합", "builtin"},
		"mqtt-subscriber":         {"io", "MQTT 토픽 구독 및 메시지 수신", "builtin"},
		"mqtt-publisher":          {"io", "MQTT 토픽으로 메시지 발행", "builtin"},
		"modbus-poller":           {"io", "MODBUS 레지스터를 주기적으로 폴링 읽기", "builtin"},
		"modbus-writer":           {"io", "MODBUS 레지스터 쓰기 전용", "builtin"},
		"lgap-status":             {"io", "LG LGAP 디바이스 상태 조회", "builtin"},
		"lgap-control":            {"io", "LG LGAP 디바이스 제어", "builtin"},
		"lgap":                    {"io", "LG LGAP 상태 조회 + 제어 통합", "builtin"},
		"lgcp-status":             {"io", "LG LGCP 디바이스 상태 조회", "builtin"},
		"lgcp-control":            {"io", "LG LGCP 디바이스 제어", "builtin"},
		"lgcp":                    {"io", "LG LGCP 상태 조회 + 제어 통합", "builtin"},
		"tsdb-write":              {"storage", "메시지를 시계열 DB에 기록", "builtin"},
		"tsdb-query":              {"storage", "시계열 DB에서 데이터를 조회", "builtin"},
		"store-write":             {"storage", "메시지 데이터를 키-값 저장소에 기록", "builtin"},
		"store-read":              {"storage", "키-값 저장소에서 데이터를 조회", "builtin"},
		"serial-in":               {"io", "시리얼 포트에서 데이터 수신", "builtin"},
		"serial-out":              {"io", "시리얼 포트로 데이터 전송", "builtin"},
		"tcp-in":                  {"io", "TCP 에이전트로부터 메시지 수신 (연결 정보 포함)", "builtin"},
		"tcp-out":                 {"io", "TCP 에이전트를 통해 메시지 전송 (연결별 라우팅)", "builtin"},
		"framer":                  {"processing", "바이트 스트림에서 프로토콜 프레임을 분리하여 완성된 프레임을 출력", "builtin"},
	}

	for typeName, exp := range expected {
		meta, ok := r.TypeMeta(typeName)
		assert.True(t, ok, "빌트인 타입 %q의 메타데이터가 있어야 한다", typeName)
		assert.Equal(t, typeName, meta.Type)
		assert.Equal(t, exp.category, meta.Category)
		assert.Equal(t, exp.description, meta.Description)
		assert.Equal(t, exp.source, meta.Source)
	}
}

// TestRegistry_TypeMeta_미등록 은 미등록 타입의 메타데이터 조회가 false를 반환하는지 확인한다.
func TestRegistry_TypeMeta_미등록(t *testing.T) {
	r := NewRegistry()

	_, ok := r.TypeMeta("nonexistent")
	assert.False(t, ok)
}

// TestRegistry_AllTypeMeta_정렬 은 AllTypeMeta가 정렬된 목록을 반환하는지 확인한다.
func TestRegistry_AllTypeMeta_정렬(t *testing.T) {
	r := NewRegistry()

	metas := r.AllTypeMeta()
	assert.Len(t, metas, 46) // +3 century 노드 (raw-frame 통합) + 1 inventory (SPEC-INVENTORY-001)

	// 타입명 기준 정렬 확인
	for i := 1; i < len(metas); i++ {
		assert.True(t, metas[i-1].Type < metas[i].Type,
			"정렬 위반: %s >= %s", metas[i-1].Type, metas[i].Type)
	}
}

// TestRegistry_RegisterWithMeta_정상 은 메타데이터 포함 등록이 올바른지 확인한다.
func TestRegistry_RegisterWithMeta_정상(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	err := r.RegisterWithMeta("custom", dummyFactory, NodeTypeMeta{
		Category:    "custom",
		Description: "커스텀 노드",
		Source:      "plugin",
	})
	require.NoError(t, err)
	assert.True(t, r.Has("custom"))

	meta, ok := r.TypeMeta("custom")
	assert.True(t, ok)
	assert.Equal(t, "custom", meta.Type) // Type은 typeName으로 설정됨
	assert.Equal(t, "custom", meta.Category)
	assert.Equal(t, "커스텀 노드", meta.Description)
	assert.Equal(t, "plugin", meta.Source)
}

// TestRegistry_RegisterWithMeta_중복에러 는 이미 등록된 타입을 재등록하면 에러를 반환하는지 확인한다.
func TestRegistry_RegisterWithMeta_중복에러(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	err := r.RegisterWithMeta("custom", dummyFactory, NodeTypeMeta{})
	require.NoError(t, err)

	err = r.RegisterWithMeta("custom", dummyFactory, NodeTypeMeta{})
	assert.ErrorIs(t, err, ErrNodeTypeAlreadyRegistered)
}

// --- 동시성 테스트 ---

// TestRegistry_동시성안전 은 레지스트리가 동시성 안전한지 확인한다.
func TestRegistry_동시성안전(t *testing.T) {
	r := NewRegistry(WithoutBuiltins())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			typeName := "type" + string(rune('A'+n))
			_ = r.Register(typeName, dummyFactory)
			_ = r.Has(typeName)
			_ = r.Types()
		}(i)
	}
	wg.Wait()
}
