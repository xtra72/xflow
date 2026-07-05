// remote_secret_merge_test.go 는 M7 시크릿 redaction 라운드트립 보존(REQ-I07)을
// 검증한다(@SPEC:SPEC-REMOTE-001 M7, 그룹 I).
//
// 결정(spec §5.9 OPEN QUESTION 6 RESOLVED — "필드 부재 + 노드 backfill"): 갱신
// 페이로드는 마스킹/미변경 시크릿 필드를 완전히 생략한다(와이어에 sentinel 없음).
// 노드(client/apply 측)는 어댑터 Update 호출 전 기존 자원 정의를 로드하여 부재한
// 시크릿 필드를 기존값으로 backfill 한 뒤 적용한다.
//
// 본 테스트는 update 명령에서 생략된 시크릿이 기존값으로 복원되고, 명시적으로 제공된
// (변경된) 시크릿은 새 값으로 적용되며, 비시크릿 필드는 들어온 정의가 권위임을
// 검증한다.
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
)

// mergeFlowAdapter 는 GetFlow 가 기존 시크릿을 보유한 정의를 반환하고, UpdateFlow 가
// 수신한 Definition 을 캡처하는 테스트 어댑터이다(시크릿 병합 검증용).
type mergeFlowAdapter struct {
	existing      map[string]any // GetFlow 가 반환할 기존 정의(시크릿 포함)
	gotDefinition map[string]any // UpdateFlow 가 수신한 정의
}

func (f *mergeFlowAdapter) CreateFlow(_ context.Context, req *dto.FlowCreateRequest) (*handler.FlowInfo, error) {
	return &handler.FlowInfo{ID: "new", Name: req.Name}, nil
}
func (f *mergeFlowAdapter) UpdateFlow(_ context.Context, id string, req *dto.FlowUpdateRequest) (*handler.FlowInfo, error) {
	f.gotDefinition = req.Definition
	return &handler.FlowInfo{ID: id}, nil
}
func (f *mergeFlowAdapter) GetFlow(_ context.Context, id string) (*handler.FlowInfo, error) {
	return &handler.FlowInfo{ID: id, Config: f.existing}, nil
}
func (f *mergeFlowAdapter) ListFlows(_ context.Context, _ dto.ListOptions) ([]handler.FlowInfo, int64, error) {
	return nil, 0, nil
}
func (f *mergeFlowAdapter) DeleteFlow(_ context.Context, _ string) error { return nil }
func (f *mergeFlowAdapter) DeployFlow(_ context.Context, _ string) error { return nil }
func (f *mergeFlowAdapter) StartFlow(_ context.Context, _ string) error  { return nil }
func (f *mergeFlowAdapter) StopFlow(_ context.Context, _ string) error   { return nil }

// mergeAgentAdapter 는 GetAgent 가 기존 시크릿을 반환하고, UpdateAgent 가 수신한
// Config 를 캡처하는 테스트 어댑터이다.
type mergeAgentAdapter struct {
	existing  map[string]any
	gotConfig map[string]any
}

func (f *mergeAgentAdapter) CreateAgent(_ context.Context, req *dto.AgentCreateRequest) (*handler.AgentInfo, error) {
	return &handler.AgentInfo{ID: "new", Name: req.Name}, nil
}
func (f *mergeAgentAdapter) UpdateAgent(_ context.Context, id string, req *dto.AgentUpdateRequest) (*handler.AgentInfo, error) {
	f.gotConfig = req.Config
	return &handler.AgentInfo{ID: id}, nil
}
func (f *mergeAgentAdapter) GetAgent(_ context.Context, id, _ string) (*handler.AgentInfo, error) {
	return &handler.AgentInfo{ID: id, Config: f.existing}, nil
}
func (f *mergeAgentAdapter) ListAgents(_ context.Context, _ dto.ListOptions) ([]handler.AgentInfo, int64, error) {
	return nil, 0, nil
}
func (f *mergeAgentAdapter) DeleteAgent(_ context.Context, _ string) error  { return nil }
func (f *mergeAgentAdapter) StartAgent(_ context.Context, _ string) error   { return nil }
func (f *mergeAgentAdapter) StopAgent(_ context.Context, _ string) error    { return nil }
func (f *mergeAgentAdapter) RestartAgent(_ context.Context, _ string) error { return nil }

// TestFlowUpdate_BackfillsOmittedSecret 는 갱신 정의에서 생략된 시크릿(password)이
// 기존값으로 backfill 되는지 검증한다(REQ-I07 핵심).
func TestFlowUpdate_BackfillsOmittedSecret(t *testing.T) {
	fa := &mergeFlowAdapter{
		existing: map[string]any{
			"name":     "old",
			"password": "real-secret",
		},
	}
	c := &flowCommander{adapter: fa}

	// 갱신 정의: password 생략(마스킹되어 와이어에서 제거됨), name 변경.
	args := json.RawMessage(`{"id":"f1","definition":{"name":"new","host":"h"}}`)
	_, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	// 생략된 시크릿이 기존값으로 복원되어야 한다.
	assert.Equal(t, "real-secret", fa.gotDefinition["password"],
		"생략된 시크릿은 기존값으로 backfill 되어야 함")
	// 비시크릿 필드는 들어온 정의가 권위.
	assert.Equal(t, "new", fa.gotDefinition["name"])
	assert.Equal(t, "h", fa.gotDefinition["host"])
}

// TestFlowUpdate_KeepsProvidedSecret 는 명시적으로 제공된(변경된) 시크릿이 새 값으로
// 적용되는지 검증한다(REQ-I07 — backfill 은 생략 시에만).
func TestFlowUpdate_KeepsProvidedSecret(t *testing.T) {
	fa := &mergeFlowAdapter{
		existing: map[string]any{"password": "old-secret"},
	}
	c := &flowCommander{adapter: fa}

	args := json.RawMessage(`{"id":"f1","definition":{"password":"changed"}}`)
	_, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	assert.Equal(t, "changed", fa.gotDefinition["password"],
		"제공된 시크릿은 새 값으로 적용되어야 함(backfill 하지 않음)")
}

// TestFlowUpdate_BackfillsNestedSecret 는 중첩 맵의 생략 시크릿이 backfill 되는지
// 검증한다(RedactSensitiveConfig 가 재귀적이므로 병합도 재귀적이어야 함 — REQ-I07).
func TestFlowUpdate_BackfillsNestedSecret(t *testing.T) {
	fa := &mergeFlowAdapter{
		existing: map[string]any{
			"broker": map[string]any{
				"url":   "mqtt://h",
				"token": "real-token",
			},
		},
	}
	c := &flowCommander{adapter: fa}

	// 중첩 broker 의 token 생략, url 변경.
	args := json.RawMessage(`{"id":"f1","definition":{"broker":{"url":"mqtt://h2"}}}`)
	_, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	broker, ok := fa.gotDefinition["broker"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "real-token", broker["token"], "중첩 생략 시크릿은 backfill 되어야 함")
	assert.Equal(t, "mqtt://h2", broker["url"])
}

// TestAgentUpdate_BackfillsOmittedSecret 는 agent update 의 생략 시크릿 backfill 을
// 검증한다(REQ-I07/I04).
func TestAgentUpdate_BackfillsOmittedSecret(t *testing.T) {
	fa := &mergeAgentAdapter{
		existing: map[string]any{
			"username": "u",
			"password": "agent-secret",
		},
	}
	c := &agentCommander{adapter: fa}

	args := json.RawMessage(`{"id":"a1","config":{"qos":1}}`)
	_, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	assert.Equal(t, "agent-secret", fa.gotConfig["password"],
		"agent 생략 시크릿은 기존값으로 backfill 되어야 함")
	assert.Equal(t, "u", fa.gotConfig["username"], "agent 생략 시크릿(username)도 backfill")
	assert.EqualValues(t, 1, fa.gotConfig["qos"])
}

// TestFlowUpdate_NoDefinition_NoBackfill 는 Definition 이 없는 부분 갱신(name 만)에서
// GetFlow/backfill 을 건너뛰는지 검증한다(정의 미변경 시 시크릿 병합 불필요).
func TestFlowUpdate_NoDefinition_NoBackfill(t *testing.T) {
	fa := &mergeFlowAdapter{existing: map[string]any{"password": "x"}}
	c := &flowCommander{adapter: fa}

	args := json.RawMessage(`{"id":"f1","name":"renamed"}`)
	_, err := c.Do(context.Background(), "update", args)
	require.NoError(t, err)

	assert.Nil(t, fa.gotDefinition, "Definition 미포함 갱신은 backfill 하지 않아야 함")
}
