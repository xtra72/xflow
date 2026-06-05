// remote_commands_test.go 는 원격 명령 적용 어댑터의 도메인 라우팅·디코드·오류
// 경로를 검증한다(@SPEC:SPEC-REMOTE-001 M3, REQ-D04/D09).
//
// flow/agent 어댑터는 engine/agent 매니저 의존이 커서 본 단위 테스트는 device 도메인
// (의존 최소)과 공통 디코드 헬퍼·미지원 액션 경로에 집중한다. flow/agent 라우팅
// 자체는 internal/remote/apply_test.go(도메인 라우팅) + 어댑터 단위 테스트로 커버된다.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// fakeFlowAdapter 는 flowAdapter 의 테스트 구현이다. 호출된 메서드/ID 를 기록한다.
type fakeFlowAdapter struct {
	lastMethod string
	lastID     string
	err        error
}

func (f *fakeFlowAdapter) CreateFlow(_ context.Context, req *dto.FlowCreateRequest) (*handler.FlowInfo, error) {
	f.lastMethod = "create"
	if f.err != nil {
		return nil, f.err
	}
	return &handler.FlowInfo{ID: "new", Name: req.Name}, nil
}
func (f *fakeFlowAdapter) UpdateFlow(_ context.Context, id string, _ *dto.FlowUpdateRequest) (*handler.FlowInfo, error) {
	f.lastMethod, f.lastID = "update", id
	if f.err != nil {
		return nil, f.err
	}
	return &handler.FlowInfo{ID: id}, nil
}
func (f *fakeFlowAdapter) GetFlow(_ context.Context, id string) (*handler.FlowInfo, error) {
	f.lastMethod, f.lastID = "get", id
	if f.err != nil {
		return nil, f.err
	}
	return &handler.FlowInfo{ID: id}, nil
}
func (f *fakeFlowAdapter) ListFlows(_ context.Context, _ dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	f.lastMethod = "list"
	if f.err != nil {
		return nil, 0, f.err
	}
	return []handler.FlowInfo{{ID: "a"}}, 1, nil
}
func (f *fakeFlowAdapter) DeleteFlow(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "delete", id
	return f.err
}
func (f *fakeFlowAdapter) DeployFlow(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "deploy", id
	return f.err
}
func (f *fakeFlowAdapter) StartFlow(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "start", id
	return f.err
}
func (f *fakeFlowAdapter) StopFlow(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "stop", id
	return f.err
}

// fakeAgentAdapter 는 agentAdapter 의 테스트 구현이다.
type fakeAgentAdapter struct {
	lastMethod string
	lastID     string
	err        error
}

func (f *fakeAgentAdapter) CreateAgent(_ context.Context, req *dto.AgentCreateRequest) (*handler.AgentInfo, error) {
	f.lastMethod = "create"
	if f.err != nil {
		return nil, f.err
	}
	return &handler.AgentInfo{ID: "new", Name: req.Name}, nil
}
func (f *fakeAgentAdapter) UpdateAgent(_ context.Context, id string, _ *dto.AgentUpdateRequest) (*handler.AgentInfo, error) {
	f.lastMethod, f.lastID = "update", id
	if f.err != nil {
		return nil, f.err
	}
	return &handler.AgentInfo{ID: id}, nil
}
func (f *fakeAgentAdapter) GetAgent(_ context.Context, id, _ string) (*handler.AgentInfo, error) {
	f.lastMethod, f.lastID = "get", id
	if f.err != nil {
		return nil, f.err
	}
	return &handler.AgentInfo{ID: id}, nil
}
func (f *fakeAgentAdapter) ListAgents(_ context.Context, _ dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	f.lastMethod = "list"
	if f.err != nil {
		return nil, 0, f.err
	}
	return []handler.AgentInfo{{ID: "a"}}, 1, nil
}
func (f *fakeAgentAdapter) DeleteAgent(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "delete", id
	return f.err
}
func (f *fakeAgentAdapter) StartAgent(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "start", id
	return f.err
}
func (f *fakeAgentAdapter) StopAgent(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "stop", id
	return f.err
}
func (f *fakeAgentAdapter) RestartAgent(_ context.Context, id string) error {
	f.lastMethod, f.lastID = "restart", id
	return f.err
}

// TestFlowCommander_Routing 는 flow action → 어댑터 메서드 라우팅을 검증한다(REQ-D02).
func TestFlowCommander_Routing(t *testing.T) {
	tests := []struct {
		action string
		args   string
		method string
	}{
		{"create", `{"name":"f","definition":{}}`, "create"},
		{"update", `{"id":"f1","name":"x"}`, "update"},
		{"get", `{"id":"f1"}`, "get"},
		{"list", `{}`, "list"},
		{"delete", `{"id":"f1"}`, "delete"},
		{"deploy", `{"id":"f1"}`, "deploy"},
		{"start", `{"id":"f1"}`, "start"},
		{"stop", `{"id":"f1"}`, "stop"},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			fa := &fakeFlowAdapter{}
			c := &flowCommander{adapter: fa}
			_, err := c.Do(context.Background(), tc.action, json.RawMessage(tc.args))
			require.NoError(t, err)
			assert.Equal(t, tc.method, fa.lastMethod)
		})
	}
}

// TestFlowCommander_ErrorAndUnknown 는 어댑터 오류 전파(REQ-D09)와 미지원/미지원
// 액션·디코드 실패 경로를 검증한다.
func TestFlowCommander_ErrorAndUnknown(t *testing.T) {
	fa := &fakeFlowAdapter{err: errors.New("boom")}
	c := &flowCommander{adapter: fa}

	_, err := c.Do(context.Background(), "start", json.RawMessage(`{"id":"f1"}`))
	assert.ErrorContains(t, err, "boom")

	_, err = c.Do(context.Background(), "frob", json.RawMessage(`{"id":"f1"}`))
	assert.Error(t, err, "알 수 없는 액션은 오류")

	_, err = c.Do(context.Background(), "start", json.RawMessage(`{bad`))
	assert.Error(t, err, "디코드 실패는 오류")

	_, err = c.Do(context.Background(), "pause", json.RawMessage(`{"id":"f1"}`))
	assert.Error(t, err, "pause 는 미지원(M4 seam)")

	_, err = c.Do(context.Background(), "create", json.RawMessage(`{bad`))
	assert.Error(t, err, "create args 디코드 실패")
}

// TestFlowCommander_ErrorPerAction 는 모든 flow 액션에서 어댑터 오류가 전파되는지
// 검증한다(REQ-D09 — 부분 적용 없음, 결과 반환 경로 포함).
func TestFlowCommander_ErrorPerAction(t *testing.T) {
	actions := map[string]string{
		"create": `{"name":"f","definition":{}}`,
		"update": `{"id":"f1"}`,
		"get":    `{"id":"f1"}`,
		"list":   `{}`,
		"delete": `{"id":"f1"}`,
		"deploy": `{"id":"f1"}`,
		"start":  `{"id":"f1"}`,
		"stop":   `{"id":"f1"}`,
	}
	for action, args := range actions {
		t.Run(action, func(t *testing.T) {
			c := &flowCommander{adapter: &fakeFlowAdapter{err: errors.New("x")}}
			_, err := c.Do(context.Background(), action, json.RawMessage(args))
			assert.Error(t, err)
		})
	}
}

// TestAgentCommander_ErrorPerAction 는 모든 agent 액션에서 어댑터 오류가 전파되는지
// 검증한다.
func TestAgentCommander_ErrorPerAction(t *testing.T) {
	actions := map[string]string{
		"create":  `{"name":"a","type":"socket"}`,
		"update":  `{"id":"a1"}`,
		"get":     `{"id":"a1"}`,
		"list":    `{}`,
		"delete":  `{"id":"a1"}`,
		"start":   `{"id":"a1"}`,
		"stop":    `{"id":"a1"}`,
		"restart": `{"id":"a1"}`,
	}
	for action, args := range actions {
		t.Run(action, func(t *testing.T) {
			c := &agentCommander{adapter: &fakeAgentAdapter{err: errors.New("x")}}
			_, err := c.Do(context.Background(), action, json.RawMessage(args))
			assert.Error(t, err)
		})
	}
}

// TestFlowCommander_ResultsMarshaled 는 결과 반환 액션이 JSON 결과를 직렬화하는지
// 검증한다(create/get/list/update).
func TestFlowCommander_ResultsMarshaled(t *testing.T) {
	c := &flowCommander{adapter: &fakeFlowAdapter{}}
	for _, tc := range []struct{ action, args string }{
		{"create", `{"name":"f","definition":{}}`},
		{"update", `{"id":"f1"}`},
		{"get", `{"id":"f1"}`},
		{"list", `{}`},
	} {
		res, err := c.Do(context.Background(), tc.action, json.RawMessage(tc.args))
		require.NoError(t, err, tc.action)
		assert.NotEmpty(t, res, "결과 반환 액션은 JSON 을 반환해야 함: %s", tc.action)
	}
}

// TestAgentCommander_ResultsMarshaled 는 agent 결과 반환 액션을 검증한다.
func TestAgentCommander_ResultsMarshaled(t *testing.T) {
	c := &agentCommander{adapter: &fakeAgentAdapter{}}
	for _, tc := range []struct{ action, args string }{
		{"create", `{"name":"a","type":"socket"}`},
		{"update", `{"id":"a1"}`},
		{"get", `{"id":"a1"}`},
		{"list", `{}`},
	} {
		res, err := c.Do(context.Background(), tc.action, json.RawMessage(tc.args))
		require.NoError(t, err, tc.action)
		assert.NotEmpty(t, res, "결과 반환 액션은 JSON 을 반환해야 함: %s", tc.action)
	}
}

// TestAgentCommander_UpdateBadBody 는 update 의 본문 디코드 실패를 검증한다.
func TestAgentCommander_UpdateBadBody(t *testing.T) {
	c := &agentCommander{adapter: &fakeAgentAdapter{}}
	// id 는 유효하지만 본문 전체가 update 구조로 디코드 불가한 경우는 동일 args 라
	// 정상 디코드되므로, 여기서는 id 디코드 실패를 통해 update 경로의 오류를 확인한다.
	_, err := c.Do(context.Background(), "update", json.RawMessage(`{bad`))
	assert.Error(t, err)
}

// TestAgentCommander_Routing 는 agent action → 어댑터 메서드 라우팅을 검증한다(REQ-D03).
func TestAgentCommander_Routing(t *testing.T) {
	tests := []struct {
		action string
		args   string
		method string
	}{
		{"create", `{"name":"a","type":"socket"}`, "create"},
		{"update", `{"id":"a1","name":"x"}`, "update"},
		{"get", `{"id":"a1"}`, "get"},
		{"list", `{}`, "list"},
		{"delete", `{"id":"a1"}`, "delete"},
		{"start", `{"id":"a1"}`, "start"},
		{"stop", `{"id":"a1"}`, "stop"},
		{"restart", `{"id":"a1"}`, "restart"},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			fa := &fakeAgentAdapter{}
			c := &agentCommander{adapter: fa}
			_, err := c.Do(context.Background(), tc.action, json.RawMessage(tc.args))
			require.NoError(t, err)
			assert.Equal(t, tc.method, fa.lastMethod)
		})
	}
}

// TestAgentCommander_ErrorAndUnknown 는 agent 어댑터 오류/미지원/디코드 실패를
// 검증한다.
func TestAgentCommander_ErrorAndUnknown(t *testing.T) {
	fa := &fakeAgentAdapter{err: errors.New("boom")}
	c := &agentCommander{adapter: fa}

	_, err := c.Do(context.Background(), "start", json.RawMessage(`{"id":"a1"}`))
	assert.ErrorContains(t, err, "boom")

	_, err = c.Do(context.Background(), "frob", json.RawMessage(`{"id":"a1"}`))
	assert.Error(t, err)

	_, err = c.Do(context.Background(), "start", json.RawMessage(`{bad`))
	assert.Error(t, err)

	_, err = c.Do(context.Background(), "create", json.RawMessage(`{bad`))
	assert.Error(t, err)
}

// fakeDeviceRegistry 는 deviceRegistrySetter 의 테스트 구현이다.
type fakeDeviceRegistry struct {
	lastID   string
	lastMeta device.DeviceMetadata
	err      error
	calls    int
}

func (f *fakeDeviceRegistry) SetMetadata(id string, meta device.DeviceMetadata) error {
	f.calls++
	f.lastID = id
	f.lastMeta = meta
	return f.err
}

// fakeMetaRepo 는 deviceMetadataRepo 의 테스트 구현이다.
type fakeMetaRepo struct {
	saved   map[string]device.DeviceMetadata
	deleted []string
	saveErr error
	delErr  error
}

func newFakeMetaRepo() *fakeMetaRepo {
	return &fakeMetaRepo{saved: make(map[string]device.DeviceMetadata)}
}

func (f *fakeMetaRepo) Save(_ context.Context, id string, meta device.DeviceMetadata) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved[id] = meta
	return nil
}

func (f *fakeMetaRepo) Delete(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

// TestDeviceCommander_Update 는 update 가 레지스트리 SetMetadata + 영속 Save 를
// 순서대로 수행하는지 검증한다(REQ-D04 — 핸들러 동일 경로).
func TestDeviceCommander_Update(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	repo := newFakeMetaRepo()
	c := &deviceCommander{registry: reg, repo: repo}

	args := json.RawMessage(`{"id":"dev-1","metadata":{"name":"보일러","location":"1F"}}`)
	res, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	assert.Equal(t, "dev-1", reg.lastID)
	assert.Equal(t, "보일러", reg.lastMeta.Name)
	assert.Equal(t, "보일러", repo.saved["dev-1"].Name, "영속 저장에도 반영되어야 함")
	assert.Contains(t, string(res), "보일러")
}

// TestDeviceCommander_DeleteMetadata 는 delete_metadata 가 레지스트리 메타 초기화 +
// 영속 Delete 를 수행하는지 검증한다.
func TestDeviceCommander_DeleteMetadata(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	repo := newFakeMetaRepo()
	c := &deviceCommander{registry: reg, repo: repo}

	res, err := c.Do(context.Background(), "delete_metadata", json.RawMessage(`{"id":"dev-2"}`))
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, device.DeviceMetadata{}, reg.lastMeta, "레지스트리 메타가 초기화되어야 함")
	assert.Equal(t, []string{"dev-2"}, repo.deleted)
}

// TestDeviceCommander_MissingID 는 id 누락 시 오류(부분 적용 방지)를 검증한다.
func TestDeviceCommander_MissingID(t *testing.T) {
	c := &deviceCommander{registry: &fakeDeviceRegistry{}, repo: newFakeMetaRepo()}
	_, err := c.Do(context.Background(), "update", json.RawMessage(`{"metadata":{}}`))
	assert.Error(t, err)
}

// TestDeviceCommander_BadArgs 는 잘못된 args 디코드 실패를 검증한다.
func TestDeviceCommander_BadArgs(t *testing.T) {
	c := &deviceCommander{registry: &fakeDeviceRegistry{}, repo: newFakeMetaRepo()}
	_, err := c.Do(context.Background(), "update", json.RawMessage(`{bad`))
	assert.Error(t, err)
}

// TestDeviceCommander_UnknownAction 는 미지원 액션 거부를 검증한다.
func TestDeviceCommander_UnknownAction(t *testing.T) {
	c := &deviceCommander{registry: &fakeDeviceRegistry{}, repo: newFakeMetaRepo()}
	_, err := c.Do(context.Background(), "frobnicate", json.RawMessage(`{"id":"x"}`))
	assert.Error(t, err)
}

// TestDeviceCommander_RegistryError 는 레지스트리 오류가 영속 전에 전파되어 부분
// 적용이 없는지 검증한다(REQ-D09).
func TestDeviceCommander_RegistryError(t *testing.T) {
	reg := &fakeDeviceRegistry{err: errors.New("device not found")}
	repo := newFakeMetaRepo()
	c := &deviceCommander{registry: reg, repo: repo}

	_, err := c.Do(context.Background(), "update", json.RawMessage(`{"id":"x","metadata":{}}`))
	assert.Error(t, err)
	assert.Empty(t, repo.saved, "레지스트리 실패 시 영속 저장이 일어나지 않아야 함")
}

// TestDeviceCommander_PersistError 는 영속 실패가 오류로 보고되는지 검증한다.
func TestDeviceCommander_PersistError(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	repo := newFakeMetaRepo()
	repo.saveErr = errors.New("disk full")
	c := &deviceCommander{registry: reg, repo: repo}

	_, err := c.Do(context.Background(), "update", json.RawMessage(`{"id":"x","metadata":{}}`))
	assert.Error(t, err)
}

// TestDecodeID 는 ID 추출 헬퍼의 정상/누락/오류 경로를 검증한다.
func TestDecodeID(t *testing.T) {
	id, err := decodeID(json.RawMessage(`{"id":"abc"}`))
	require.NoError(t, err)
	assert.Equal(t, "abc", id)

	_, err = decodeID(json.RawMessage(`{}`))
	assert.Error(t, err, "id 누락은 오류여야 함")

	_, err = decodeID(json.RawMessage(`not json`))
	assert.Error(t, err)
}

// TestDecodeIDWithBody 는 id + 본문 보존을 검증한다.
func TestDecodeIDWithBody(t *testing.T) {
	id, body, err := decodeIDWithBody(json.RawMessage(`{"id":"f1","name":"x"}`))
	require.NoError(t, err)
	assert.Equal(t, "f1", id)
	assert.JSONEq(t, `{"id":"f1","name":"x"}`, string(body))

	_, _, err = decodeIDWithBody(json.RawMessage(`{}`))
	assert.Error(t, err)
}

// TestMarshalResult 는 nil/값 직렬화를 검증한다.
func TestMarshalResult(t *testing.T) {
	res, err := marshalResult(nil)
	require.NoError(t, err)
	assert.Nil(t, res)

	res, err = marshalResult(map[string]string{"k": "v"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"k":"v"}`, string(res))
}

// TestCommanders_SatisfyInterface 는 세 commander 가 remote.DomainCommander 를
// 만족하는지(컴파일 타임 + 런타임) 확인하는 wiring 스모크 테스트이다.
func TestCommanders_SatisfyInterface(t *testing.T) {
	var (
		fc remote.DomainCommander = &flowCommander{}
		ac remote.DomainCommander = &agentCommander{}
		dc remote.DomainCommander = &deviceCommander{}
	)
	assert.NotNil(t, fc)
	assert.NotNil(t, ac)
	assert.NotNil(t, dc)

	// Applier 가 세 commander 를 묶어 domain 라우팅을 수행하는 wiring 확인.
	applier := remote.NewApplier(fc, ac, dc)
	var _ remote.CommandApplier = applier
	assert.NotNil(t, applier)
}
