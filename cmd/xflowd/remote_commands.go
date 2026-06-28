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
		// 시크릿 backfill(REQ-I07): 원격 갱신 정의는 마스킹/미변경 시크릿 필드를
		// 생략한다(필드 부재). 어댑터 적용 전 기존 플로우 정의를 로드하여 부재한 시크릿
		// 필드를 기존값으로 backfill 한다(마스킹 자리표시자 영속 방지 — "노드 backfill").
		// Definition 이 없는 부분 갱신은 병합 불필요.
		if req.Definition != nil {
			if existing, gerr := c.adapter.GetFlow(ctx, id); gerr == nil && existing != nil {
				req.Definition = mergeSecrets(existing.Config, req.Definition)
			}
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
		// 시크릿 backfill(REQ-I07): agent Config 는 자격증명(password/token 등)을 보유할
		// 수 있다. 갱신 Config 에서 생략된(마스킹) 시크릿을 기존 config 의 값으로
		// backfill 한다. Config 미포함 부분 갱신은 병합 불필요.
		if req.Config != nil {
			if existing, gerr := c.adapter.GetAgent(ctx, id, ""); gerr == nil && existing != nil {
				req.Config = mergeSecrets(existing.Config, req.Config)
			}
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

// deviceExecutor 는 device 런타임 명령(execute) 실행에 필요한 최소 인터페이스이다.
// device.DeviceRegistry.Execute 와 동일 시그니처로, 로컬 DeviceHandler.Execute
// (POST /devices/{id}/execute)가 호출하는 바로 그 registry 인스턴스를 주입한다 —
// 원격 실행이 로컬 제어와 완전히 동일한 경로/검증을 통과하도록 보장한다(REQ-D04,
// OQ-L4: 제어 쓰기는 그룹 D 를 재사용). device.ErrNotControllable /
// ErrDeviceNotFound 등 오류 의미는 그대로 전파된다.
type deviceExecutor interface {
	Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)
}

// deviceCommander 는 device 메타데이터 명령을 레지스트리 + 영속 저장소로 적용하고,
// device 런타임 명령(execute)을 로컬 Execute 서비스로 적용한다(REQ-D04). HTTP
// api.Context 를 위조하지 않고, device 핸들러와 동일한 clean 메서드(SetMetadata +
// Save/Delete, Execute)를 직접 호출하여 로컬 검증을 보존한다.
type deviceCommander struct {
	registry deviceRegistrySetter
	repo     deviceMetadataRepo
	executor deviceExecutor
}

// deviceMetadataArgs 는 device 메타데이터 명령 인자이다.
type deviceMetadataArgs struct {
	ID       string                `json:"id"`
	Metadata device.DeviceMetadata `json:"metadata"`
}

// deviceExecuteArgs 는 device 런타임 명령(execute) 인자이다. 프런트엔드 그룹 D
// 페이로드({domain:'device', action:'execute', args:{id, command, params}})와
// 동일한 모양을 소비한다(REQ-D04). handler.ExecuteRequest(command/params) +
// id 를 합친 형태이다.
type deviceExecuteArgs struct {
	ID      string         `json:"id"`
	Command string         `json:"command"`
	Params  map[string]any `json:"params,omitempty"`
}

// Do 는 update(메타데이터 설정+영속) / delete_metadata(메타데이터 제거+영속 삭제) /
// execute(런타임 명령 실행)를 적용한다. update/delete_metadata 는 device 핸들러
// UpdateMetadata/DeleteMetadata 와 동일한 순서/검증이고, execute 는 로컬
// DeviceHandler.Execute(POST /devices/{id}/execute)와 동일한 Execute 서비스를 호출한다.
func (c *deviceCommander) Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	// execute 는 메타데이터와 인자 모양이 다르므로 먼저 분기한다(런타임 제어 쓰기 —
	// OQ-L4: 제어 쓰기는 그룹 D 를 재사용). 의도된 WRITE 이므로 read-only 안전장치는
	// 적용하지 않으며, device.ErrNotControllable / ErrDeviceNotFound 등 오류는 그대로
	// 명령 오류로 전파된다(서버가 502 로 매핑 → UI 노출).
	if action == "execute" {
		return c.execute(ctx, args)
	}

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

// execute 는 device 런타임 명령을 로컬 Execute 서비스로 적용한다. 인자는 프런트엔드
// 그룹 D 페이로드(args:{id, command, params})와 동일하게 디코드하며, 로컬
// DeviceHandler.Execute 와 동일한 필수값 검증(id, command)을 적용한 뒤 동일한 registry
// Execute 메서드를 호출한다 — 원격 실행이 로컬 POST /devices/{id}/execute 와 동일한
// 경로/검증/오류 의미를 갖도록 보장한다. 실행 결과(map)는 그대로 직렬화하여 반환하고,
// device.ErrNotControllable / ErrDeviceNotFound 등 오류는 그대로 전파한다(부분 적용
// 없음 — REQ-D09).
func (c *deviceCommander) execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var p deviceExecuteArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("device execute args: %w", err)
	}
	if p.ID == "" {
		return nil, fmt.Errorf("device: id 는 필수입니다")
	}
	if p.Command == "" {
		return nil, fmt.Errorf("device: command 는 필수입니다")
	}
	if c.executor == nil {
		return nil, fmt.Errorf("device: execute 서비스가 구성되지 않았습니다")
	}
	result, err := c.executor.Execute(ctx, p.ID, p.Command, p.Params)
	if err != nil {
		return nil, err
	}
	return marshalResult(result)
}

// mergeSecrets 는 원격 갱신 정의(incoming)에서 생략된 시크릿 필드를 기존 정의
// (existing)의 값으로 backfill 한다(REQ-I07 — "필드 부재 + 노드 backfill").
//
// 규칙:
//   - 시크릿 키(handler.IsSensitiveConfigKey 의 SoT 기준)가 existing 에 있고 incoming
//     에 부재하면, existing 값을 incoming 에 채운다(backfill). 마스킹되어 생략된
//     시크릿이 기존값으로 복원된다.
//   - 시크릿 키가 incoming 에 존재하면(사용자가 변경) 그대로 둔다(새 값 적용).
//   - 비시크릿 필드는 incoming 이 권위이다(backfill 하지 않음 — 들어온 정의가 최신).
//   - 중첩 map[string]any 에 대해 재귀적으로 동작한다(redaction 이 재귀적이므로 — F06
//     일관). incoming 에 없던 중첩 키는 비시크릿이므로 채우지 않는다(시크릿 backfill
//     에만 한정).
//
// incoming 을 in-place 변형하여 반환한다. existing/incoming 중 하나가 nil 이면 incoming
// 을 그대로 반환한다(병합 대상 없음).
func mergeSecrets(existing, incoming map[string]any) map[string]any {
	if existing == nil || incoming == nil {
		return incoming
	}
	for k, ev := range existing {
		if handler.IsSensitiveConfigKey(k) {
			// 시크릿 키: incoming 에 부재하면 기존값으로 backfill.
			if _, present := incoming[k]; !present {
				incoming[k] = ev
			}
			continue
		}
		// 비시크릿 키: 양쪽 모두 중첩 맵이면 재귀 병합(중첩 시크릿 backfill).
		em, eok := ev.(map[string]any)
		iv, ipresent := incoming[k]
		im, iok := iv.(map[string]any)
		if eok && ipresent && iok {
			incoming[k] = mergeSecrets(em, im)
		}
	}
	return incoming
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
