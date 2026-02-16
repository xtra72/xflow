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

func (d *dummyNode) Init(_ context.Context) error                                          { return nil }
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

	builtins := []string{"filter", "transform", "switch", "bridge", "script", "catch"}
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
