// remote_grouping_test.go 는 v1.4(M9, 그룹 K) 노드 그룹핑 + 노드 상세 REST API 의
// 권한·동작을 검증한다(@SPEC:SPEC-REMOTE-001 M9, REQ-K02~K06/K08/K10/F04).
//
// admin 게이팅(비-admin 403), 그룹 배정/해제, distinct 그룹 목록(전체 포함), 노드 상세
// (시스템 정보+uptime+운영 요약), 미존재 노드 404 를 검증한다.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// fakeGrouping 은 NodeGroupingService 의 테스트 구현이다.
type fakeGrouping struct {
	groups       map[string]string // instance_id -> group_name
	overrides    map[string][2]int // instance_id -> [width, height] 해상도 오버라이드(M11 확장)
	detail       remote.NodeDetail
	detailErr    error
	setErr       error
	renamed      [2]string                    // [oldName, newName] 마지막 rename 기록
	deletedGroup string                       // 마지막 delete 그룹 기록
	dispatched   []string                     // "group/domain/action" 기록(CommandGroup 경로)
	lastArgs     json.RawMessage              // 마지막 DispatchGroup args(CommandGroup 경로)
	updatedGroup string                       // 마지막 DispatchGroupUpdate 대상 그룹
	lastPlan     remote.GroupUpdatePlan       // 마지막 DispatchGroupUpdate plan(업데이트 경로)
	dispatchRes  []remote.GroupDispatchResult // DispatchGroup/Update 반환값
	groupErr     error                        // rename/delete/dispatch 공통 에러 주입
}

func (f *fakeGrouping) RenameGroup(_ context.Context, oldName, newName string) (int, error) {
	if f.groupErr != nil {
		return 0, f.groupErr
	}
	f.renamed = [2]string{oldName, newName}
	return 1, nil
}

func (f *fakeGrouping) DeleteGroup(_ context.Context, groupName string) (int, error) {
	if f.groupErr != nil {
		return 0, f.groupErr
	}
	f.deletedGroup = groupName
	return 1, nil
}

func (f *fakeGrouping) DispatchGroup(
	_ context.Context,
	groupName, domain, action string,
	args json.RawMessage,
) ([]remote.GroupDispatchResult, error) {
	if f.groupErr != nil {
		return nil, f.groupErr
	}
	f.dispatched = append(f.dispatched, groupName+"/"+domain+"/"+action)
	f.lastArgs = args
	return f.dispatchRes, nil
}

// DispatchGroupUpdate 은 아키텍처-aware 그룹 업데이트 경로를 캡처한다. 단일 버전(pin)
// 경로의 하위 호환 검증을 위해 plan 의 Channel/UpdateURL/DefaultVersion 을 lastArgs
// (SystemUpdateArgs JSON)로도 합성해 기존 어서션을 유지한다.
func (f *fakeGrouping) DispatchGroupUpdate(
	_ context.Context,
	groupName string,
	plan remote.GroupUpdatePlan,
) ([]remote.GroupDispatchResult, error) {
	if f.groupErr != nil {
		return nil, f.groupErr
	}
	f.dispatched = append(f.dispatched, groupName+"/"+remote.DomainSystem+"/"+remote.ActionSystemUpdate)
	f.updatedGroup = groupName
	f.lastPlan = plan
	// pin 경로 하위 호환: DefaultVersion 을 TargetVersion 으로 노출(소스 주입 검증용).
	f.lastArgs, _ = json.Marshal(remote.SystemUpdateArgs{
		TargetVersion: plan.DefaultVersion,
		Channel:       plan.Channel,
		UpdateURL:     plan.UpdateURL,
		Restart:       plan.Restart,
	})
	return f.dispatchRes, nil
}

func newFakeGrouping() *fakeGrouping {
	return &fakeGrouping{groups: make(map[string]string), overrides: make(map[string][2]int)}
}

func (f *fakeGrouping) SetNodeGroup(_ context.Context, instanceID, groupName string) error {
	if f.setErr != nil {
		return f.setErr
	}
	if _, ok := f.groups[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	f.groups[instanceID] = groupName
	return nil
}

func (f *fakeGrouping) ClearNodeGroup(ctx context.Context, instanceID string) error {
	return f.SetNodeGroup(ctx, instanceID, "")
}

func (f *fakeGrouping) ListGroups(_ context.Context) ([]storage.NodeGroupCount, error) {
	counts := map[string]int{}
	for _, g := range f.groups {
		counts[g]++
	}
	out := []storage.NodeGroupCount{{GroupName: "", NodeCount: counts[""]}}
	for g, c := range counts {
		if g == "" {
			continue
		}
		out = append(out, storage.NodeGroupCount{GroupName: g, NodeCount: c})
	}
	return out, nil
}

func (f *fakeGrouping) NodeDetail(_ context.Context, _ string) (remote.NodeDetail, error) {
	if f.detailErr != nil {
		return remote.NodeDetail{}, f.detailErr
	}
	return f.detail, nil
}

// SetNodeDisplayOverride 는 해상도 오버라이드를 설정/해제한다(M11 확장). width<=0 또는
// height<=0 이면 0,0 으로 해제한다(저장소 계약 일관). 미존재 노드는 ErrManagedNodeNotFound.
func (f *fakeGrouping) SetNodeDisplayOverride(_ context.Context, instanceID string, width, height int) error {
	if f.setErr != nil {
		return f.setErr
	}
	if _, ok := f.groups[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	if width <= 0 || height <= 0 {
		f.overrides[instanceID] = [2]int{0, 0}
		return nil
	}
	f.overrides[instanceID] = [2]int{width, height}
	return nil
}

// ClearNodeDisplayOverride 는 해상도 오버라이드를 해제한다(0,0 환원).
func (f *fakeGrouping) ClearNodeDisplayOverride(ctx context.Context, instanceID string) error {
	return f.SetNodeDisplayOverride(ctx, instanceID, 0, 0)
}

// doGrouping 은 지정 역할로 그룹핑 라우트에 요청한다.
func doGrouping(t *testing.T, svc NodeGroupingService, role, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteGroupingHandler(svc)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), role))
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestRemoteGrouping_SetGroup 는 admin 이 그룹을 배정하는지 검증한다(REQ-K02).
func TestRemoteGrouping_SetGroup(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""

	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/group", `{"group_name":"prod"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "prod", svc.groups["n1"])
}

// TestRemoteGrouping_SetGroupRequiresAdmin 는 비-admin 이 거부되는지 검증한다(REQ-K06/F04).
func TestRemoteGrouping_SetGroupRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""

	rec := doGrouping(t, svc, "node", http.MethodPut, "/api/v1/remote/nodes/n1/group", `{"group_name":"prod"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, svc.groups["n1"], "거부된 요청은 그룹을 변경하지 않아야 함")
}

// TestRemoteGrouping_SetGroupViewerRejected 는 viewer 거부를 검증한다.
func TestRemoteGrouping_SetGroupViewerRejected(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "viewer", http.MethodPut, "/api/v1/remote/nodes/n1/group", `{"group_name":"prod"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteGrouping_SetGroupUnknownNode 는 미존재 노드 404 를 검증한다.
func TestRemoteGrouping_SetGroupUnknownNode(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/missing/group", `{"group_name":"prod"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteGrouping_ClearGroup 는 그룹 해제(204)를 검증한다(REQ-K02/K05).
func TestRemoteGrouping_ClearGroup(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = "prod"

	rec := doGrouping(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1/group", "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, svc.groups["n1"], "그룹 해제 시 빈값으로 환원")
}

// TestRemoteGrouping_ClearGroupRequiresAdmin 는 비-admin 해제 거부를 검증한다.
func TestRemoteGrouping_ClearGroupRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = "prod"
	rec := doGrouping(t, svc, "node", http.MethodDelete, "/api/v1/remote/nodes/n1/group", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "prod", svc.groups["n1"])
}

// TestRemoteGrouping_ListGroups 는 distinct 그룹 + "전체" 포함 응답을 검증한다(REQ-K03).
func TestRemoteGrouping_ListGroups(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["a"] = "prod"
	svc.groups["b"] = "prod"
	svc.groups["c"] = "" // 전체

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/groups", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []NodeGroupDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	counts := map[string]int{}
	for _, g := range resp.Data {
		counts[g.GroupName] = g.NodeCount
	}
	assert.Equal(t, 2, counts["prod"])
	assert.Equal(t, 1, counts[""], "응답은 항상 '전체' 버킷(빈 라벨)을 포함해야 함")
}

// TestRemoteGrouping_ListGroupsRequiresAdmin 는 비-admin 목록 거부를 검증한다.
func TestRemoteGrouping_ListGroupsRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "viewer", http.MethodGet, "/api/v1/remote/groups", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteGrouping_NodeDetail 는 노드 상세(메타+시스템 정보+uptime+운영 요약)를
// 검증한다(REQ-K08/K10).
func TestRemoteGrouping_NodeDetail(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node: storage.ManagedNode{
			InstanceID: "n1", Hostname: "host", Version: "1.0.0", Status: "approved",
			GroupName: "prod", OS: "linux", Arch: "arm64", StartedAt: 1000, LastSeen: 9999,
		},
		Online:    true,
		UptimeMs:  60000,
		HasUptime: true,
		Summary: storage.NodeOperationalSummary{
			Flows:   storage.FlowSummary{Total: 3, Running: 2, Stopped: 1},
			Agents:  storage.AgentSummary{Total: 2, Connected: 1},
			Devices: storage.DeviceSummary{Total: 1, Online: 1},
		},
	}

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data NodeDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	d := resp.Data
	assert.Equal(t, "n1", d.InstanceID)
	assert.Equal(t, "linux", d.OS)
	assert.Equal(t, "arm64", d.Arch)
	assert.Equal(t, "prod", d.GroupName)
	assert.True(t, d.Online)
	require.NotNil(t, d.Uptime, "started_at>0 이면 uptime 이 채워져야 함")
	assert.Equal(t, int64(60000), *d.Uptime)
	assert.Equal(t, 3, d.Summary.Flows.Total)
	assert.Equal(t, 2, d.Summary.Flows.Running)
	assert.Equal(t, 1, d.Summary.Agents.Connected)
	assert.Equal(t, 1, d.Summary.Devices.Online)
}

// TestRemoteGrouping_NodeDetailNoUptime 는 started_at==0(미보고)일 때 uptime 이 null
// 인지 검증한다(REQ-K08/K09 하위 호환).
func TestRemoteGrouping_NodeDetailNoUptime(t *testing.T) {
	svc := newFakeGrouping()
	svc.detail = remote.NodeDetail{
		Node:      storage.ManagedNode{InstanceID: "n1", Status: "approved"},
		Online:    false,
		UptimeMs:  -1,
		HasUptime: false,
	}

	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data NodeDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Data.Uptime, "started_at==0 이면 uptime 은 null 이어야 함")
	assert.Equal(t, int64(0), resp.Data.StartedAt)
}

// TestRemoteGrouping_NodeDetailRequiresAdmin 는 비-admin 상세 거부를 검증한다(REQ-K06).
func TestRemoteGrouping_NodeDetailRequiresAdmin(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "node", http.MethodGet, "/api/v1/remote/nodes/n1", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteGrouping_NodeDetailUnknownNode 는 미존재 노드 상세 404 를 검증한다.
func TestRemoteGrouping_NodeDetailUnknownNode(t *testing.T) {
	svc := newFakeGrouping()
	svc.detailErr = storage.ErrManagedNodeNotFound
	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/nodes/missing", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteGrouping_SetGroupBadBody 는 잘못된 JSON 본문이 400 으로 매핑되는지 검증한다.
func TestRemoteGrouping_SetGroupBadBody(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/group", `{not json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteGrouping_SetGroupInternalError 는 일반 에러가 500 으로 매핑되는지 검증한다.
func TestRemoteGrouping_SetGroupInternalError(t *testing.T) {
	svc := newFakeGrouping()
	svc.groups["n1"] = ""
	svc.setErr = errors.New("boom")
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/nodes/n1/group", `{"group_name":"x"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestRemoteGrouping_ListGroupsInternalError 는 목록 일반 에러가 500 인지 검증한다.
func TestRemoteGrouping_ListGroupsInternalError(t *testing.T) {
	svc := &erroringGrouping{}
	rec := doGrouping(t, svc, "admin", http.MethodGet, "/api/v1/remote/groups", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// erroringGrouping 은 항상 일반 에러를 반환하는 NodeGroupingService 이다(에러 경로).
type erroringGrouping struct{}

func (erroringGrouping) SetNodeGroup(context.Context, string, string) error {
	return errors.New("boom")
}
func (erroringGrouping) ClearNodeGroup(context.Context, string) error { return errors.New("boom") }
func (erroringGrouping) ListGroups(context.Context) ([]storage.NodeGroupCount, error) {
	return nil, errors.New("boom")
}
func (erroringGrouping) NodeDetail(context.Context, string) (remote.NodeDetail, error) {
	return remote.NodeDetail{}, errors.New("boom")
}
func (erroringGrouping) SetNodeDisplayOverride(context.Context, string, int, int) error {
	return errors.New("boom")
}
func (erroringGrouping) ClearNodeDisplayOverride(context.Context, string) error {
	return errors.New("boom")
}
func (erroringGrouping) RenameGroup(context.Context, string, string) (int, error) {
	return 0, errors.New("boom")
}
func (erroringGrouping) DeleteGroup(context.Context, string) (int, error) {
	return 0, errors.New("boom")
}
func (erroringGrouping) DispatchGroup(context.Context, string, string, string, json.RawMessage) ([]remote.GroupDispatchResult, error) {
	return nil, errors.New("boom")
}
func (erroringGrouping) DispatchGroupUpdate(context.Context, string, remote.GroupUpdatePlan) ([]remote.GroupDispatchResult, error) {
	return nil, errors.New("boom")
}

// TestRemoteGrouping_RenameGroup 는 admin 그룹 일괄 이름변경을 검증한다.
func TestRemoteGrouping_RenameGroup(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/groups/prod", `{"new_name":"production"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, [2]string{"prod", "production"}, svc.renamed)
}

// 빈 new_name 은 거부된다.
func TestRemoteGrouping_RenameGroup_EmptyName(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPut, "/api/v1/remote/groups/prod", `{"new_name":"  "}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteGrouping_DeleteGroup 는 admin 그룹 삭제(→전체)를 검증한다.
func TestRemoteGrouping_DeleteGroup(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodDelete, "/api/v1/remote/groups/prod", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "prod", svc.deletedGroup)
}

// TestRemoteGrouping_UpdateGroup 는 그룹 일괄 업데이트(strategy 미지정 = pin)가
// DispatchGroupUpdate 를 통해 system/update 로 디스패치되고, plan 이 단일 버전(pin)을
// DefaultVersion 으로 담는지(VersionByArch 없음, RequireMapping=false) 검증한다.
func TestRemoteGrouping_UpdateGroup(t *testing.T) {
	svc := newFakeGrouping()
	svc.dispatchRes = []remote.GroupDispatchResult{{InstanceID: "p1", OK: true}}
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/update", `{"version":"v1.3.0","restart":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, svc.dispatched, "prod/system/update")
	assert.Equal(t, "prod", svc.updatedGroup)
	// pin 경로: 단일 버전은 DefaultVersion 으로, 아키텍처 매핑은 없어야 한다.
	assert.Equal(t, "v1.3.0", svc.lastPlan.DefaultVersion)
	assert.True(t, svc.lastPlan.Restart)
	assert.Empty(t, svc.lastPlan.VersionByArch)
	assert.False(t, svc.lastPlan.RequireMapping)
}

// TestRemoteGrouping_UpdateGroup_InjectsSource 는 그룹 일괄 업데이트가 서버 저장
// 업데이트 소스(update_url/채널)를 주입하는지 검증한다.
func TestRemoteGrouping_UpdateGroup_InjectsSource(t *testing.T) {
	svc := newFakeGrouping()
	svc.dispatchRes = []remote.GroupDispatchResult{{InstanceID: "p1", OK: true}}
	settings := newMemSettings()
	require.NoError(t, settings.SetSetting(context.Background(), updateSourceSettingKey,
		`{"update_url":"https://dl.example.com/xflow","channel":"beta"}`))

	h := NewRemoteGroupingHandler(svc).WithSettings(settings)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remote/groups/prod/update", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), "admin"))
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var args struct {
		UpdateURL string `json:"update_url"`
		Channel   string `json:"channel"`
	}
	require.NoError(t, json.Unmarshal(svc.lastArgs, &args))
	assert.Equal(t, "https://dl.example.com/xflow", args.UpdateURL)
	assert.Equal(t, "beta", args.Channel)
}

// 잘못된 버전은 거부된다(디스패치 안 함).
func TestRemoteGrouping_UpdateGroup_InvalidVersion(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/update", `{"version":"garbage"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// newHandlerReleaseRepo 는 핸들러 테스트용 실제 릴리즈 저장소를 임시 DB 로 만든다.
func newHandlerReleaseRepo(t *testing.T) *storage.ReleaseRepository {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.OpenSQLiteDB(ctx, dir+"/xflow.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := storage.NewReleaseRepository(db, dir+"/releases")
	require.NoError(t, err)
	return repo
}

// putHandlerAsset 은 (version,os,arch) asset 을 올린다(서명 없이).
func putHandlerAsset(t *testing.T, repo *storage.ReleaseRepository, version, goos, arch string) {
	t.Helper()
	_, err := repo.PutAsset(context.Background(), version, goos, arch,
		strings.NewReader("bin"), nil, 1000)
	require.NoError(t, err)
}

// doGroupingWithReleases 는 릴리즈 저장소를 연결한 핸들러로 요청한다(strategy=latest/per_arch).
func doGroupingWithReleases(t *testing.T, svc NodeGroupingService, releases *storage.ReleaseRepository, role, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteGroupingHandler(svc).WithReleases(releases)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), role))
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestRemoteGrouping_UpdateGroup_StrategyLatest 는 strategy=latest 가 릴리즈 저장소에서
// 채널별 (os,arch) 슬롯 최신 버전으로 VersionByArch 를 구성하고 RequireMapping 을
// 활성화하는지 검증한다.
func TestRemoteGrouping_UpdateGroup_StrategyLatest(t *testing.T) {
	svc := newFakeGrouping()
	svc.dispatchRes = []remote.GroupDispatchResult{{InstanceID: "p1", OK: true}}
	releases := newHandlerReleaseRepo(t)
	// v1.0.0: amd64+arm64, v2.0.0: amd64 만(arm64 누락 → v1.0.0 폴백).
	putHandlerAsset(t, releases, "v1.0.0", "linux", "amd64")
	putHandlerAsset(t, releases, "v1.0.0", "linux", "arm64")
	putHandlerAsset(t, releases, "v2.0.0", "linux", "amd64")

	rec := doGroupingWithReleases(t, svc, releases, "admin", http.MethodPost,
		"/api/v1/remote/groups/prod/update", `{"strategy":"latest","channel":"stable"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, svc.dispatched, "prod/system/update")
	assert.True(t, svc.lastPlan.RequireMapping)
	assert.Equal(t, "v2.0.0", svc.lastPlan.VersionByArch["linux/amd64"])
	assert.Equal(t, "v1.0.0", svc.lastPlan.VersionByArch["linux/arm64"])
	assert.Empty(t, svc.lastPlan.DefaultVersion)
}

// TestRemoteGrouping_UpdateGroup_StrategyLatestNoReleases 는 릴리즈 저장소 미구성 시
// strategy=latest 가 400 으로 거부되는지 검증한다.
func TestRemoteGrouping_UpdateGroup_StrategyLatestNoReleases(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/update", `{"strategy":"latest"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// TestRemoteGrouping_UpdateGroup_StrategyPerArch 는 strategy=per_arch 가 명시 맵을 검증 후
// 그대로 plan.VersionByArch 로 전달하는지 검증한다(asset 존재 확인 포함).
func TestRemoteGrouping_UpdateGroup_StrategyPerArch(t *testing.T) {
	svc := newFakeGrouping()
	svc.dispatchRes = []remote.GroupDispatchResult{{InstanceID: "p1", OK: true}}
	releases := newHandlerReleaseRepo(t)
	putHandlerAsset(t, releases, "v1.0.0", "linux", "amd64")
	putHandlerAsset(t, releases, "v1.5.0", "linux", "arm64")

	body := `{"strategy":"per_arch","version_by_arch":{"linux/amd64":"v1.0.0","linux/arm64":"v1.5.0"}}`
	rec := doGroupingWithReleases(t, svc, releases, "admin", http.MethodPost,
		"/api/v1/remote/groups/prod/update", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, svc.lastPlan.RequireMapping)
	assert.Equal(t, "v1.0.0", svc.lastPlan.VersionByArch["linux/amd64"])
	assert.Equal(t, "v1.5.0", svc.lastPlan.VersionByArch["linux/arm64"])
}

// TestRemoteGrouping_UpdateGroup_PerArchMissingAsset 는 per_arch 맵의 버전이 해당
// os/arch asset 을 보유하지 않으면 400 으로 거부되는지(디스패치 안 함) 검증한다.
func TestRemoteGrouping_UpdateGroup_PerArchMissingAsset(t *testing.T) {
	svc := newFakeGrouping()
	releases := newHandlerReleaseRepo(t)
	// v1.0.0 은 amd64 만 보유 — arm64 매핑은 asset 부재로 400.
	putHandlerAsset(t, releases, "v1.0.0", "linux", "amd64")

	body := `{"strategy":"per_arch","version_by_arch":{"linux/arm64":"v1.0.0"}}`
	rec := doGroupingWithReleases(t, svc, releases, "admin", http.MethodPost,
		"/api/v1/remote/groups/prod/update", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// TestRemoteGrouping_UpdateGroup_PerArchInvalidVersion 는 per_arch 맵에 비-semver 가
// 있으면 400 으로 거부되는지 검증한다.
func TestRemoteGrouping_UpdateGroup_PerArchInvalidVersion(t *testing.T) {
	svc := newFakeGrouping()
	releases := newHandlerReleaseRepo(t)
	body := `{"strategy":"per_arch","version_by_arch":{"linux/amd64":"garbage"}}`
	rec := doGroupingWithReleases(t, svc, releases, "admin", http.MethodPost,
		"/api/v1/remote/groups/prod/update", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// TestRemoteGrouping_UpdateGroup_UnknownStrategy 는 알 수 없는 strategy 가 400 인지 검증한다.
func TestRemoteGrouping_UpdateGroup_UnknownStrategy(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/update", `{"strategy":"rolling"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// TestRemoteGrouping_CommandGroup 는 그룹 일괄 명령 디스패치를 검증한다.
func TestRemoteGrouping_CommandGroup(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/command", `{"domain":"agent","action":"stop"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, svc.dispatched, "prod/agent/stop")
}

// domain/action 누락은 거부된다.
func TestRemoteGrouping_CommandGroup_MissingFields(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "admin", http.MethodPost, "/api/v1/remote/groups/prod/command", `{"domain":"agent"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// 비-admin 은 그룹 관리 엔드포인트에서 403.
func TestRemoteGrouping_GroupOps_RequireAdmin(t *testing.T) {
	svc := newFakeGrouping()
	rec := doGrouping(t, svc, "", http.MethodPut, "/api/v1/remote/groups/prod", `{"new_name":"x"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
