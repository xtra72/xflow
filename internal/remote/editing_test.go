// editing_test.go 는 M7 서버 측 원격 자원 편집 보조 메서드(노출 범위 확인 + 명령
// 결과 수신 후 미러 캐시 갱신)를 검증한다(@SPEC:SPEC-REMOTE-001 M7, REQ-I05/I11/E08).
//
// 본 메서드들은 핸들러(remote_editing.go)가 Dispatch 성공 후에만 호출한다(A4/E08 —
// 서버 단독 영속 금지). 따라서 본 테스트는 명령 경유 없이 미러 mutator 의 단독 동작과
// 노출 범위(미러 존재) 판정을 검증한다.
package remote

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// fakeMirror 는 MirrorRepository 의 최소 테스트 구현이다(편집 mutator 단독 검증용).
type fakeMirror struct {
	flows  map[string]storage.MirroredResource
	agents map[string]storage.MirroredResource
}

func newFakeMirror() *fakeMirror {
	return &fakeMirror{
		flows:  make(map[string]storage.MirroredResource),
		agents: make(map[string]storage.MirroredResource),
	}
}

func (m *fakeMirror) ReplaceFlows(_ context.Context, _ string, items []storage.MirroredResource) error {
	for _, it := range items {
		m.flows[it.ID] = it
	}
	return nil
}
func (m *fakeMirror) ReplaceAgents(_ context.Context, _ string, items []storage.MirroredResource) error {
	for _, it := range items {
		m.agents[it.ID] = it
	}
	return nil
}
func (m *fakeMirror) ReplaceDevices(_ context.Context, _ string, _ []storage.MirroredResource) error {
	return nil
}
func (m *fakeMirror) UpsertResource(_ context.Context, kind string, item storage.MirroredResource) error {
	switch kind {
	case KindFlow:
		m.flows[item.ID] = item
	case KindAgent:
		m.agents[item.ID] = item
	}
	return nil
}
func (m *fakeMirror) DeleteResource(_ context.Context, _ string, kind, id string) error {
	switch kind {
	case KindFlow:
		delete(m.flows, id)
	case KindAgent:
		delete(m.agents, id)
	}
	return nil
}
func (m *fakeMirror) ListFlows(_ context.Context, instanceID string) ([]storage.MirroredResource, error) {
	return filterByNode(m.flows, instanceID), nil
}
func (m *fakeMirror) ListAgents(_ context.Context, instanceID string) ([]storage.MirroredResource, error) {
	return filterByNode(m.agents, instanceID), nil
}
func (m *fakeMirror) ListDevices(_ context.Context, _ string) ([]storage.MirroredResource, error) {
	return nil, nil
}
func (m *fakeMirror) ListAllFlows(_ context.Context) ([]storage.MirroredResource, error) {
	return mapValues(m.flows), nil
}
func (m *fakeMirror) ListAllAgents(_ context.Context) ([]storage.MirroredResource, error) {
	return mapValues(m.agents), nil
}
func (m *fakeMirror) ListAllDevices(_ context.Context) ([]storage.MirroredResource, error) {
	return nil, nil
}
func (m *fakeMirror) DeleteByNode(_ context.Context, instanceID string) error {
	for id, r := range m.flows {
		if r.SourceInstanceID == instanceID {
			delete(m.flows, id)
		}
	}
	for id, r := range m.agents {
		if r.SourceInstanceID == instanceID {
			delete(m.agents, id)
		}
	}
	return nil
}
func (m *fakeMirror) NodeSummary(_ context.Context, _ string) (storage.NodeOperationalSummary, error) {
	return storage.NodeOperationalSummary{}, nil
}
func (m *fakeMirror) Close() error { return nil }

func filterByNode(src map[string]storage.MirroredResource, instanceID string) []storage.MirroredResource {
	out := make([]storage.MirroredResource, 0)
	for _, r := range src {
		if r.SourceInstanceID == instanceID {
			out = append(out, r)
		}
	}
	return out
}

func mapValues(src map[string]storage.MirroredResource) []storage.MirroredResource {
	out := make([]storage.MirroredResource, 0, len(src))
	for _, r := range src {
		out = append(out, r)
	}
	return out
}

// TestIsResourceExposed_PresentInMirror 는 미러에 존재하는 자원이 노출 범위 내로
// 판정되는지 검증한다(REQ-I05 — 수정/삭제 대상은 노출 범위 내여야 함).
func TestIsResourceExposed_PresentInMirror(t *testing.T) {
	mirror := newFakeMirror()
	mirror.flows["f1"] = storage.MirroredResource{ID: "f1", SourceInstanceID: "n1", Kind: KindFlow}
	s := NewServer(ServerConfig{Mirror: mirror}, nil)

	exposed, err := s.IsResourceExposed(context.Background(), "n1", KindFlow, "f1")
	require.NoError(t, err)
	assert.True(t, exposed, "미러에 존재하는 자원은 노출 범위 내여야 함")
}

// TestIsResourceExposed_AbsentFromMirror 는 미러에 없는 자원이 노출 범위 밖으로
// 판정되는지 검증한다(REQ-I05 — 범위 밖 편집 거부).
func TestIsResourceExposed_AbsentFromMirror(t *testing.T) {
	mirror := newFakeMirror()
	s := NewServer(ServerConfig{Mirror: mirror}, nil)

	exposed, err := s.IsResourceExposed(context.Background(), "n1", KindFlow, "missing")
	require.NoError(t, err)
	assert.False(t, exposed, "미러에 없는 자원은 노출 범위 밖이어야 함")
}

// TestUpsertMirror 는 update 성공 후 미러 행이 갱신되는지 검증한다(REQ-E08 — 결과
// 수신 후에만 캐시 갱신).
func TestUpsertMirror(t *testing.T) {
	mirror := newFakeMirror()
	s := NewServer(ServerConfig{Mirror: mirror}, nil)

	row := storage.MirroredResource{ID: "f1", SourceInstanceID: "n1", Kind: KindFlow, Name: "Updated"}
	require.NoError(t, s.UpsertMirror(context.Background(), KindFlow, row))

	assert.Equal(t, "Updated", mirror.flows["f1"].Name)
}

// TestDeleteMirror 는 delete 성공 후 미러 행이 제거되는지 검증한다(REQ-E08/I03).
func TestDeleteMirror(t *testing.T) {
	mirror := newFakeMirror()
	mirror.flows["f1"] = storage.MirroredResource{ID: "f1", SourceInstanceID: "n1", Kind: KindFlow}
	s := NewServer(ServerConfig{Mirror: mirror}, nil)

	require.NoError(t, s.DeleteMirror(context.Background(), "n1", KindFlow, "f1"))

	_, ok := mirror.flows["f1"]
	assert.False(t, ok, "delete 후 미러 행이 제거되어야 함")
}

// TestMirrorMutators_NoMirror 는 mirror 미구성 시 mutator 가 안전하게 no-op 하는지
// 검증한다(하위 호환).
func TestMirrorMutators_NoMirror(t *testing.T) {
	s := NewServer(ServerConfig{}, nil)

	require.NoError(t, s.UpsertMirror(context.Background(), KindFlow, storage.MirroredResource{ID: "x"}))
	require.NoError(t, s.DeleteMirror(context.Background(), "n1", KindFlow, "x"))
	exposed, err := s.IsResourceExposed(context.Background(), "n1", KindFlow, "x")
	require.NoError(t, err)
	// mirror 미구성이면 노출 범위를 판정할 수 없으므로 보수적으로 false(범위 밖)이다.
	assert.False(t, exposed)
}
