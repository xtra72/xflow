// remote_admin_m4_test.go 는 M4 인벤토리 미러 목록 엔드포인트와 E08(서버 편집→명령
// 전파) 불변식을 검증한다(@SPEC:SPEC-REMOTE-001 M4, REQ-E05/E06/E08).
//
// fakeNodeAdmin(remote_admin_test.go)을 미러 목록 메서드로 확장한다.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// --- fakeNodeAdmin 미러 목록 확장 ---

// mirrorByNode 는 instanceID → kind → views 를 보관하는 테스트 캐시이다. 이 캐시는
// 오직 recordDelta(노드 push 모사)로만 변경된다(E08 — 서버 편집은 직접 쓰지 않음).
type mirrorStore struct {
	flows   map[string][]remote.MirroredResourceView
	agents  map[string][]remote.MirroredResourceView
	devices map[string][]remote.MirroredResourceView
}

func newMirrorStore() *mirrorStore {
	return &mirrorStore{
		flows:   map[string][]remote.MirroredResourceView{},
		agents:  map[string][]remote.MirroredResourceView{},
		devices: map[string][]remote.MirroredResourceView{},
	}
}

func mkView(id, node, name, kind string, online bool) remote.MirroredResourceView {
	return remote.MirroredResourceView{
		MirroredResource: storage.MirroredResource{ID: id, SourceInstanceID: node, Name: name, Kind: kind},
		Online:           online,
	}
}

func (f *fakeNodeAdmin) ListMirroredFlows(_ context.Context, id string) ([]remote.MirroredResourceView, error) {
	if _, ok := f.nodes[id]; !ok {
		return nil, storage.ErrManagedNodeNotFound
	}
	return f.mirror.flows[id], nil
}

func (f *fakeNodeAdmin) ListMirroredAgents(_ context.Context, id string) ([]remote.MirroredResourceView, error) {
	if _, ok := f.nodes[id]; !ok {
		return nil, storage.ErrManagedNodeNotFound
	}
	return f.mirror.agents[id], nil
}

func (f *fakeNodeAdmin) ListMirroredDevices(_ context.Context, id string) ([]remote.MirroredResourceView, error) {
	if _, ok := f.nodes[id]; !ok {
		return nil, storage.ErrManagedNodeNotFound
	}
	return f.mirror.devices[id], nil
}

func (f *fakeNodeAdmin) ListAllMirroredFlows(_ context.Context) ([]remote.MirroredResourceView, error) {
	var out []remote.MirroredResourceView
	for _, v := range f.mirror.flows {
		out = append(out, v...)
	}
	return out, nil
}

func (f *fakeNodeAdmin) ListAllMirroredAgents(_ context.Context) ([]remote.MirroredResourceView, error) {
	var out []remote.MirroredResourceView
	for _, v := range f.mirror.agents {
		out = append(out, v...)
	}
	return out, nil
}

func (f *fakeNodeAdmin) ListAllMirroredDevices(_ context.Context) ([]remote.MirroredResourceView, error) {
	var out []remote.MirroredResourceView
	for _, v := range f.mirror.devices {
		out = append(out, v...)
	}
	return out, nil
}

// --- 테스트 ---

// TestRemoteAdmin_NodeFlows 는 노드별 미러 목록이 출처/online 태그를 포함하는지
// 검증한다(REQ-E04/E05/E06).
func TestRemoteAdmin_NodeFlows(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved", Online: true}
	svc.mirror.flows["n1"] = []remote.MirroredResourceView{mkView("f1", "n1", "flowA", "flow", true)}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/nodes/n1/flows")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "source_instance_id")
	assert.Contains(t, body, "n1")
	assert.Contains(t, body, "flowA")
}

// TestRemoteAdmin_NodeAgentsAndDevices 는 agent/device 노드별 엔드포인트를 검증한다.
func TestRemoteAdmin_NodeAgentsAndDevices(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Online: true}
	svc.mirror.agents["n1"] = []remote.MirroredResourceView{mkView("a1", "n1", "agentA", "agent", true)}
	svc.mirror.devices["n1"] = []remote.MirroredResourceView{mkView("d1", "n1", "devA", "device", true)}

	do := registerAdminRoutes(t, svc, "admin")

	recA := do(http.MethodGet, "/api/v1/remote/nodes/n1/agents")
	require.Equal(t, http.StatusOK, recA.Code)
	assert.Contains(t, recA.Body.String(), "agentA")

	recD := do(http.MethodGet, "/api/v1/remote/nodes/n1/devices")
	require.Equal(t, http.StatusOK, recD.Code)
	assert.Contains(t, recD.Body.String(), "devA")
}

// TestRemoteAdmin_AllAgentsAndDevices 는 agent/device 통합 엔드포인트를 검증한다.
func TestRemoteAdmin_AllAgentsAndDevices(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Online: true}
	svc.mirror.agents["n1"] = []remote.MirroredResourceView{mkView("a1", "n1", "agentA", "agent", true)}
	svc.mirror.devices["n1"] = []remote.MirroredResourceView{mkView("d1", "n1", "devA", "device", true)}

	do := registerAdminRoutes(t, svc, "admin")

	recA := do(http.MethodGet, "/api/v1/remote/agents")
	require.Equal(t, http.StatusOK, recA.Code)
	assert.Contains(t, recA.Body.String(), "agentA")

	recD := do(http.MethodGet, "/api/v1/remote/devices")
	require.Equal(t, http.StatusOK, recD.Code)
	assert.Contains(t, recD.Body.String(), "devA")
}

// TestRemoteAdmin_AllFlows_RequiresAdmin 는 통합 목록이 admin 권한을 강제하는지
// 검증한다(REQ-F04).
func TestRemoteAdmin_AllFlows_RequiresAdmin(t *testing.T) {
	svc := newFakeNodeAdmin()
	do := registerAdminRoutes(t, svc, "node")
	rec := do(http.MethodGet, "/api/v1/remote/flows")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteAdmin_AllFlows_Error 는 통합 조회 에러가 500 으로 매핑되는지 검증한다.
func TestRemoteAdmin_AllFlows_Error(t *testing.T) {
	h := NewRemoteAdminHandler(&erroringAdmin{}, nil)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))
	_, rec, req := newAdminRequest(http.MethodGet, "/api/v1/remote/flows", "admin")
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestRemoteAdmin_NodeFlows_UnknownNode 는 알 수 없는 노드가 404 를 반환하는지
// 검증한다(REQ-E05).
func TestRemoteAdmin_NodeFlows_UnknownNode(t *testing.T) {
	svc := newFakeNodeAdmin()
	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/nodes/ghost/flows")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteAdmin_NodeFlows_RequiresAdmin 는 비-admin 이 403 인지 검증한다(REQ-F04).
func TestRemoteAdmin_NodeFlows_RequiresAdmin(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1"}
	do := registerAdminRoutes(t, svc, "node")
	rec := do(http.MethodGet, "/api/v1/remote/nodes/n1/flows")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteAdmin_AllFlows 는 통합 목록이 여러 노드의 자원을 출처 태그와 함께
// 반환하는지 검증한다(REQ-E05).
func TestRemoteAdmin_AllFlows(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Online: true}
	svc.nodes["n2"] = storage.ManagedNode{InstanceID: "n2", Online: false}
	svc.mirror.flows["n1"] = []remote.MirroredResourceView{mkView("f1", "n1", "a", "flow", true)}
	svc.mirror.flows["n2"] = []remote.MirroredResourceView{mkView("f2", "n2", "b", "flow", false)}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/flows")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "n1")
	assert.Contains(t, body, "n2")
}

// TestRemoteAdmin_OfflineNodeReturnsLastKnownWithFlag 는 오프라인 노드의 미러가
// online=false 표식과 함께 반환되는지 검증한다(REQ-E06 last-known).
func TestRemoteAdmin_OfflineNodeReturnsLastKnownWithFlag(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Online: false}
	svc.mirror.flows["n1"] = []remote.MirroredResourceView{mkView("f1", "n1", "lastKnown", "flow", false)}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/nodes/n1/flows")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []MirroredResourceDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	assert.False(t, resp.Data[0].Online, "오프라인 노드 미러는 online=false 여야 함(last-known)")
	assert.Equal(t, "lastKnown", resp.Data[0].Name)
}

// TestRemoteAdmin_E08_EditDispatchesCommandNoDirectCacheWrite 는 E08 불변식을
// 검증한다: admin 편집(command)은 Dispatch 명령 경로로 전파되며, 서버 캐시(미러)를
// 직접 쓰지 않는다. 캐시는 이후 노드가 push 한 delta 로만 갱신된다(REQ-E08, A4).
func TestRemoteAdmin_E08_EditDispatchesCommandNoDirectCacheWrite(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved", Online: true}
	svc.dispatchResult = json.RawMessage(`{"ok":true}`)

	// 편집 전 미러 캐시는 비어 있다.
	require.Empty(t, svc.mirror.flows["n1"])

	do := registerAdminRoutes(t, svc, "admin")
	rec := doCommand(t, svc, "admin", "n1", `{"domain":"flow","action":"update","args":{"id":"f1","name":"renamed"}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	// 편집은 Dispatch 로 전파되었다(명령 경로 — D 그룹).
	require.Equal(t, []string{"flow/update"}, svc.dispatched)
	// 그리고 서버 캐시는 직접 쓰이지 않았다(E08 — 노드 push 전까지 미반영).
	flows, _ := svc.ListMirroredFlows(context.Background(), "n1")
	assert.Empty(t, flows, "admin 편집은 미러 캐시를 직접 쓰면 안 됨(E08)")

	// 이제 노드가 delta 를 push 했다고 가정(테스트가 직접 캐시 갱신 모사) → 캐시 반영.
	svc.mirror.flows["n1"] = []remote.MirroredResourceView{mkView("f1", "n1", "renamed", "flow", true)}
	rec = do(http.MethodGet, "/api/v1/remote/nodes/n1/flows")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "renamed", "노드 delta 수신 후에만 캐시가 갱신됨(E08)")
}
