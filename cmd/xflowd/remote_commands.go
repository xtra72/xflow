// remote_commands.go 는 원격 관리 클라이언트(mode=client)의 명령 적용 어댑터를
// 정의한다(@SPEC:SPEC-REMOTE-001 M3, spec §5.7, REQ-D02/D03/D04, A5).
//
// internal/remote 는 import cycle 회피를 위해 internal/api/service /
// internal/api/handler 를 import 하지 않고 좁은 DomainCommander 인터페이스만
// 정의한다. 본 파일이 그 인터페이스를 구체 어댑터(FlowServiceAdapter /
// AgentServiceAdapter / device 서비스)에 바인딩한다.
//
// 적용 경로는 로컬 API 와 동일한 어댑터/검증을 통과한다(원격 우회 없음 — A5). 각
// 명령은 단일 어댑터 호출이므로 실패 시 부분 적용 없이 오류를 반환한다(REQ-D09).
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// flowAdapter 는 flowCommander 가 사용하는 FlowServiceAdapter 의 좁은 인터페이스이다.
// *service.FlowServiceAdapter 가 이를 만족하며, 테스트는 fake 로 대체한다(원격 우회
// 없음 — 실제 wiring 은 동일 어댑터 인스턴스를 주입; A5).
type flowAdapter interface {
	CreateFlow(ctx context.Context, req *dto.FlowCreateRequest) (*handler.FlowInfo, error)
	UpdateFlow(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*handler.FlowInfo, error)
	GetFlow(ctx context.Context, id string) (*handler.FlowInfo, error)
	ListFlows(ctx context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error)
	DeleteFlow(ctx context.Context, id string) error
	DeployFlow(ctx context.Context, id string) error
	StartFlow(ctx context.Context, id string) error
	StopFlow(ctx context.Context, id string) error
}

// agentAdapter 는 agentCommander 가 사용하는 AgentServiceAdapter 의 좁은 인터페이스이다.
type agentAdapter interface {
	CreateAgent(ctx context.Context, req *dto.AgentCreateRequest) (*handler.AgentInfo, error)
	UpdateAgent(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*handler.AgentInfo, error)
	GetAgent(ctx context.Context, id string, detail string) (*handler.AgentInfo, error)
	ListAgents(ctx context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error)
	DeleteAgent(ctx context.Context, id string) error
	StartAgent(ctx context.Context, id string) error
	StopAgent(ctx context.Context, id string) error
	RestartAgent(ctx context.Context, id string) error
}

// marshalResult 는 어댑터 결과를 json.RawMessage 로 직렬화한다. nil 은 빈 결과로
// 처리한다.
func marshalResult(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal command result: %w", err)
	}
	return data, nil
}

// flowCommander 는 flow 도메인 명령을 FlowServiceAdapter 로 적용한다(REQ-D02).
type flowCommander struct {
	adapter flowAdapter
}

// Do 는 action 별로 FlowServiceAdapter 메서드에 라우팅한다.
//
// args 디코드 실패는 적용 전 거부되며(부분 적용 없음 — REQ-D09), 어댑터 오류는 그대로
// 전파된다. ID 기반 액션은 {"id": "..."} 형태의 args 를 기대한다.
func (c *flowCommander) Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	switch action {
	case "create":
		var req dto.FlowCreateRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("flow create args: %w", err)
		}
		info, err := c.adapter.CreateFlow(ctx, &req)
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "update":
		id, rest, err := decodeIDWithBody(args)
		if err != nil {
			return nil, err
		}
		var req dto.FlowUpdateRequest
		if uerr := json.Unmarshal(rest, &req); uerr != nil {
			return nil, fmt.Errorf("flow update args: %w", uerr)
		}
		info, err := c.adapter.UpdateFlow(ctx, id, &req)
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "get":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		info, err := c.adapter.GetFlow(ctx, id)
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "list":
		var opts dto.ListOptions
		_ = json.Unmarshal(args, &opts) // args 선택적.
		opts.Normalize()
		flows, total, err := c.adapter.ListFlows(ctx, opts)
		if err != nil {
			return nil, err
		}
		return marshalResult(map[string]any{"flows": flows, "total": total})
	case "delete":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.DeleteFlow(ctx, id)
	case "deploy":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.DeployFlow(ctx, id)
	case "start":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.StartFlow(ctx, id)
	case "stop":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.StopFlow(ctx, id)
	case "pause":
		// 엔진 일시정지는 ConfigureFlow 경로가 아닌 별도 의미이나, 어댑터에 직접 Pause
		// 메서드가 없으므로 현재는 Stop 의미로 매핑하지 않고 미지원으로 명시한다.
		// M4+ 에서 어댑터 Pause 노출 시 연결한다.
		return nil, fmt.Errorf("flow pause: 미지원 액션(어댑터 Pause 노출 후 연결 — M4 seam)")
	default:
		return nil, fmt.Errorf("flow: 알 수 없는 액션 %q", action)
	}
}

// agentCommander 는 agent 도메인 명령을 AgentServiceAdapter 로 적용한다(REQ-D03).
type agentCommander struct {
	adapter agentAdapter
}

// Do 는 action 별로 AgentServiceAdapter 메서드에 라우팅한다.
func (c *agentCommander) Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	switch action {
	case "create":
		var req dto.AgentCreateRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("agent create args: %w", err)
		}
		info, err := c.adapter.CreateAgent(ctx, &req)
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "update":
		id, rest, err := decodeIDWithBody(args)
		if err != nil {
			return nil, err
		}
		var req dto.AgentUpdateRequest
		if uerr := json.Unmarshal(rest, &req); uerr != nil {
			return nil, fmt.Errorf("agent update args: %w", uerr)
		}
		info, err := c.adapter.UpdateAgent(ctx, id, &req)
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "get":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		info, err := c.adapter.GetAgent(ctx, id, "")
		if err != nil {
			return nil, err
		}
		return marshalResult(info)
	case "list":
		var opts dto.ListOptions
		_ = json.Unmarshal(args, &opts)
		opts.Normalize()
		agents, total, err := c.adapter.ListAgents(ctx, opts)
		if err != nil {
			return nil, err
		}
		return marshalResult(map[string]any{"agents": agents, "total": total})
	case "delete":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.DeleteAgent(ctx, id)
	case "start":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.StartAgent(ctx, id)
	case "stop":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.StopAgent(ctx, id)
	case "restart":
		id, err := decodeID(args)
		if err != nil {
			return nil, err
		}
		return nil, c.adapter.RestartAgent(ctx, id)
	default:
		return nil, fmt.Errorf("agent: 알 수 없는 액션 %q", action)
	}
}

// deviceMetadataStore 는 device 메타데이터 적용에 필요한 최소 인터페이스이다.
// device.Registry(SetMetadata) + storage.DeviceMetadataFileRepository(Save/Delete)를
// 만족한다(로컬 API device 핸들러와 동일 경로 — REQ-D04).
type deviceRegistrySetter interface {
	SetMetadata(id string, metadata device.DeviceMetadata) error
}

type deviceMetadataRepo interface {
	Save(ctx context.Context, deviceID string, metadata device.DeviceMetadata) error
	Delete(ctx context.Context, deviceID string) error
}

// deviceCommander 는 device 메타데이터 명령을 레지스트리 + 영속 저장소로 적용한다
// (REQ-D04). HTTP api.Context 를 위조하지 않고, device 핸들러와 동일한 clean 메서드
// (SetMetadata + Save/Delete)를 직접 호출하여 로컬 검증을 보존한다.
type deviceCommander struct {
	registry deviceRegistrySetter
	repo     deviceMetadataRepo
}

// deviceMetadataArgs 는 device 메타데이터 명령 인자이다.
type deviceMetadataArgs struct {
	ID       string                `json:"id"`
	Metadata device.DeviceMetadata `json:"metadata"`
}

// Do 는 update(메타데이터 설정+영속) / delete_metadata(메타데이터 제거+영속 삭제)를
// 적용한다. device 핸들러 UpdateMetadata/DeleteMetadata 와 동일한 순서/검증이다.
func (c *deviceCommander) Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	var p deviceMetadataArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("device args: %w", err)
	}
	if p.ID == "" {
		return nil, fmt.Errorf("device: id 는 필수입니다")
	}
	switch action {
	case "update":
		// 레지스트리 인메모리 갱신 → 영속(핸들러 UpdateMetadata 순서 준용).
		if err := c.registry.SetMetadata(p.ID, p.Metadata); err != nil {
			return nil, err
		}
		if err := c.repo.Save(ctx, p.ID, p.Metadata); err != nil {
			return nil, fmt.Errorf("device metadata persist: %w", err)
		}
		return marshalResult(p.Metadata)
	case "delete_metadata":
		if err := c.registry.SetMetadata(p.ID, device.DeviceMetadata{}); err != nil {
			return nil, err
		}
		if err := c.repo.Delete(ctx, p.ID); err != nil {
			return nil, fmt.Errorf("device metadata delete: %w", err)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("device: 알 수 없는 액션 %q", action)
	}
}

// idArgs 는 ID 기반 액션의 공통 인자 형태이다.
type idArgs struct {
	ID string `json:"id"`
}

// decodeID 는 {"id": "..."} args 에서 ID 를 추출한다. 누락 시 오류(부분 적용 방지).
func decodeID(args json.RawMessage) (string, error) {
	var p idArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("decode id args: %w", err)
	}
	if p.ID == "" {
		return "", fmt.Errorf("id 는 필수입니다")
	}
	return p.ID, nil
}

// decodeIDWithBody 는 ID 를 추출하고 동일 args 본문을 그대로 반환한다(update 처럼
// id + 나머지 필드를 함께 디코드해야 하는 액션용).
func decodeIDWithBody(args json.RawMessage) (string, json.RawMessage, error) {
	id, err := decodeID(args)
	if err != nil {
		return "", nil, err
	}
	return id, args, nil
}

// 컴파일 타임: 각 commander 가 remote.DomainCommander 를 만족하는지 검증한다.
var (
	_ remote.DomainCommander = (*flowCommander)(nil)
	_ remote.DomainCommander = (*agentCommander)(nil)
	_ remote.DomainCommander = (*deviceCommander)(nil)
)
