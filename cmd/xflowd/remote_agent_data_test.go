// remote_agent_data_test.go 는 M8(그룹 J) agent/store·agent/series 노드 read 어댑터를
// 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J04). 어댑터가 agent 매니저에서 이름으로
// 에이전트를 찾아 로컬과 동일 형상의 store keys / series 목록을 반환하는지 확인한다.
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/tsdb"
)

// fakeAgentMgrLister 는 agentLister 의 경량 페이크이다.
type fakeAgentMgrLister struct{ agents []agent.Agent }

func (f *fakeAgentMgrLister) List() []agent.Agent { return f.agents }

// fakeStoreAgent 는 storeKeysSnapshotter 를 만족하는 최소 agent.Agent 페이크이다.
type fakeStoreAgent struct {
	agent.Agent
	name     string
	snapshot map[string]system.StaticKeyMeta
}

func (a *fakeStoreAgent) Name() string { return a.name }
func (a *fakeStoreAgent) StaticKeysSnapshot() map[string]system.StaticKeyMeta {
	return a.snapshot
}

// fakeTSDBAgent 는 tsdbProvider 를 만족하는 최소 agent.Agent 페이크이다.
type fakeTSDBAgent struct {
	agent.Agent
	name string
	db   tsdb.TSDB
}

func (a *fakeTSDBAgent) Name() string    { return a.name }
func (a *fakeTSDBAgent) TSDB() tsdb.TSDB { return a.db }

// TestStoreReader_ReturnsKeysSnapshot 는 store reader 가 StaticKeysSnapshot 을 로컬
// ListKeys 와 동일 형상({count, keys:[…]})으로 반환하는지 검증한다(REQ-J04).
func TestStoreReader_ReturnsKeysSnapshot(t *testing.T) {
	storeAg := &fakeStoreAgent{
		name: "store1",
		snapshot: map[string]system.StaticKeyMeta{
			"b_key": {DataType: system.DataType("float"), Field: "temperature", Source: system.RegistrationSource("manual")},
			"a_key": {DataType: system.DataType("int"), Field: "count", Source: system.RegistrationSource("auto"), Tags: map[string]string{"room": "1"}},
		},
	}
	reader := newAgentManagerStoreReader(&fakeAgentMgrLister{agents: []agent.Agent{storeAg}})

	resp, err := reader.StoreKeys(context.Background(), "store1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 2, resp.Count)
	// 알파벳순 정렬(M9 안정성): a_key < b_key.
	require.Len(t, resp.Keys, 2)
	assert.Equal(t, "a_key", resp.Keys[0].Key)
	assert.Equal(t, "b_key", resp.Keys[1].Key)
	// Tags 는 항상 non-nil(빈 맵 보장).
	assert.NotNil(t, resp.Keys[1].Tags)
	assert.Equal(t, "manual", resp.Keys[1].Registration)
}

// TestStoreReader_NotFound 는 store 에이전트 미존재 시 오류를 반환하는지 검증한다.
func TestStoreReader_NotFound(t *testing.T) {
	reader := newAgentManagerStoreReader(&fakeAgentMgrLister{})
	_, err := reader.StoreKeys(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, remoteAgentDataErr)
}

// TestStoreReader_NotStoreAgent 는 store 가 아닌 에이전트에 대해 오류를 반환하는지
// 검증한다(type-assert 실패).
func TestStoreReader_NotStoreAgent(t *testing.T) {
	tsdbAg := &fakeTSDBAgent{name: "tsdb1"}
	reader := newAgentManagerStoreReader(&fakeAgentMgrLister{agents: []agent.Agent{tsdbAg}})
	_, err := reader.StoreKeys(context.Background(), "tsdb1")
	require.Error(t, err)
	assert.ErrorIs(t, err, remoteAgentDataErr)
}

// TestSeriesReader_ReturnsSeriesKeys 는 series reader 가 TSDB().SeriesKeys 를 반환하는지
// 검증한다(GET /tsdb/series 와 동일 소스 — REQ-J04).
func TestSeriesReader_ReturnsSeriesKeys(t *testing.T) {
	// DefaultConfig 사용(EvictionInterval 0 이면 ticker 패닉 — 양수 주기 필요).
	db := tsdb.New(tsdb.DefaultConfig())
	defer db.Close() // eviction 고루틴 정리(누수/레이스 방지).
	require.NoError(t, db.Write("cpu", map[string]string{"host": "h1"}, map[string]any{"v": 1.0}))
	require.NoError(t, db.Write("mem", map[string]string{"host": "h1"}, map[string]any{"v": 2.0}))

	tsdbAg := &fakeTSDBAgent{name: "tsdb1", db: db}
	reader := newAgentManagerSeriesReader(&fakeAgentMgrLister{agents: []agent.Agent{tsdbAg}})

	keys, err := reader.SeriesList(context.Background(), "tsdb1")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

// TestSeriesReader_NilDBReturnsEmpty 는 TSDB 미초기화(nil) 시 빈 목록을 반환하는지
// 검증한다(graceful).
func TestSeriesReader_NilDBReturnsEmpty(t *testing.T) {
	tsdbAg := &fakeTSDBAgent{name: "tsdb1", db: nil}
	reader := newAgentManagerSeriesReader(&fakeAgentMgrLister{agents: []agent.Agent{tsdbAg}})

	keys, err := reader.SeriesList(context.Background(), "tsdb1")
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// TestSeriesReader_NotFound 는 tsdb 에이전트 미존재 시 오류를 반환하는지 검증한다.
func TestSeriesReader_NotFound(t *testing.T) {
	reader := newAgentManagerSeriesReader(&fakeAgentMgrLister{})
	_, err := reader.SeriesList(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, remoteAgentDataErr)
}
