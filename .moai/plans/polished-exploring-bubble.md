# 플로우 노드 인스턴스 설정/상태 조회 구현 계획

## Context

배포된 플로우를 구성하는 런타임 노드 인스턴스의 설정(config)과 상태(lifecycle state, ports)를
API 및 CLI에서 조회할 수 있는 기능이 없다.

현재 `GET /api/v1/flows/{id}/status`는 플로우 수준의 집계 정보(메시지 수, 에러 수)만 반환하며,
`FlowStatusInfo.NodeStats` 필드가 정의되어 있지만 실제로 채워지지 않는다.

**목표**: 배포된 플로우 내 개별 노드 인스턴스의 설정, 라이프사이클 상태, 포트 정보를 조회하는 API와 CLI를 구현한다.

## 현재 아키텍처

- [engine.go](internal/engine/engine.go): `flowRuntime.nodes map[string]node.Node` — 런타임 노드 인스턴스 (private)
- [types.go](internal/engine/types.go): `FlowStatus` — 플로우 수준 집계만, 노드별 정보 없음
- [flow.go (handler)](internal/api/handler/flow.go): `FlowStatusInfo.NodeStats []NodeStatInfo` — 정의만 있고 미구현
- [flow_adapter.go](internal/api/service/flow_adapter.go#L227-L245): `FlowStatus()` — NodeStats 미포함
- [base.go](internal/node/base.go): `BaseNode` — `CurrentState()`, `GetConfig()`, `Ports()` 메서드 제공
- [state.go](pkg/lifecycle/state.go): 7가지 노드 상태 (created, initializing, running, paused, stopping, stopped, error)

## 변경 범위

| 파일 | 변경 유형 | 설명 |
|------|-----------|------|
| [types.go](internal/engine/types.go) | 수정 | `NodeInstanceInfo` 구조체 추가 |
| [engine.go](internal/engine/engine.go) | 수정 | `GetFlowNodes()`, `GetFlowNode()` 메서드 추가 |
| [flow.go (handler)](internal/api/handler/flow.go) | 수정 | `FlowManager` 인터페이스 확장, `NodeInstanceInfo` DTO, ListNodes/GetNode 핸들러, 라우트 추가 |
| [flow_adapter.go](internal/api/service/flow_adapter.go) | 수정 | `ListFlowNodes()`, `GetFlowNode()` 구현, `FlowStatus()` NodeStats 채우기 |
| [flow.go (CLI)](internal/cli/flow.go) | 수정 | `flow nodes`, `flow node` 서브커맨드 추가 |
| [engine_test.go](internal/engine/engine_test.go) | 수정 | `GetFlowNodes`, `GetFlowNode` 테스트 추가 |
| [flow_test.go (handler)](internal/api/handler/flow_test.go) | 수정 | ListNodes, GetNode 핸들러 테스트 추가 |
| [flow_adapter_test.go](internal/api/service/flow_adapter_test.go) | 수정 | ListFlowNodes, GetFlowNode 어댑터 테스트 추가 |
| [flow_test.go (CLI)](internal/cli/flow_test.go) | 수정 | flow nodes, flow node CLI 테스트 추가 |

## 구현 단계

### 1단계: Engine — 노드 인스턴스 정보 조회 메서드

**[types.go](internal/engine/types.go)** — `NodeInstanceInfo` 구조체 추가:

```go
type NodeInstanceInfo struct {
    NodeID string
    Name   string
    Type   string
    State  string            // lifecycle state (created, running, stopped, error, ...)
    Config map[string]any    // 노드 설정 복사본
    Ports  []NodePortInfo    // 포트 목록
}

type NodePortInfo struct {
    ID        string
    Name      string
    Direction string // input, output, error
    Connected bool
}
```

**[engine.go](internal/engine/engine.go)** — 2개 메서드 추가:

- `GetFlowNodes(flowID string) ([]NodeInstanceInfo, error)` — 배포된 플로우의 모든 노드 인스턴스 정보
- `GetFlowNode(flowID, nodeID string) (*NodeInstanceInfo, error)` — 특정 노드 인스턴스 정보

노드에서 정보 추출 방식:
- `n.ID()`, `n.Name()`, `n.Type()` — Node 인터페이스 메서드
- `n.Ports()` — Node 인터페이스 메서드
- `n.CurrentState()` — `BaseLifecycle` 임베딩으로 접근 (type assertion: `stateQuerier` 인터페이스)
- `n.GetConfig()` — `BaseNode` 메서드 (type assertion: `configQuerier` 인터페이스)

```go
// 선택적 인터페이스 (type assertion용)
type stateQuerier interface {
    CurrentState() lifecycle.State
}
type configQuerier interface {
    GetConfig() map[string]any
}
```

### 2단계: Handler — API 엔드포인트 확장

**[flow.go (handler)](internal/api/handler/flow.go)** 수정:

1. `NodeInstanceInfo` DTO 구조체 추가 (JSON 태그 포함):
```go
type NodeInstanceInfo struct {
    NodeID string           `json:"node_id"`
    Name   string           `json:"name"`
    Type   string           `json:"type"`
    State  string           `json:"state"`
    Config map[string]any   `json:"config,omitempty"`
    Ports  []NodePortInfo   `json:"ports,omitempty"`
}

type NodePortInfo struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Direction string `json:"direction"`
    Connected bool   `json:"connected"`
}
```

2. `FlowManager` 인터페이스에 2개 메서드 추가:
```go
ListFlowNodes(ctx context.Context, flowID string) ([]NodeInstanceInfo, error)
GetFlowNode(ctx context.Context, flowID, nodeID string) (*NodeInstanceInfo, error)
```

3. 핸들러 메서드 추가:
- `ListNodes(ctx api.Context) error` — `GET /flows/{id}/nodes`
- `GetNode(ctx api.Context) error` — `GET /flows/{id}/nodes/{nodeID}`

4. `RegisterRoutes`에 라우트 추가:
```go
g.GET("/flows/{id}/nodes", h.ListNodes)
g.GET("/flows/{id}/nodes/{nodeID}", h.GetNode)
```

### 3단계: Service Adapter — Engine 연결

**[flow_adapter.go](internal/api/service/flow_adapter.go)** 수정:

1. `ListFlowNodes()` 구현: `engine.GetFlowNodes()` → `[]handler.NodeInstanceInfo` 변환
2. `GetFlowNode()` 구현: `engine.GetFlowNode()` → `*handler.NodeInstanceInfo` 변환
3. 기존 `FlowStatus()` 메서드에서 `NodeStats` 필드 채우기: `engine.GetFlowNodes()` → `NodeStatInfo` 변환

### 4단계: CLI — flow nodes / flow node 서브커맨드

**[flow.go (CLI)](internal/cli/flow.go)** 수정:

1. `flow nodes <id|name>` 서브커맨드:
   - `GET /api/v1/flows/{id}/nodes` 호출
   - 테이블 형식: NODE_ID, NAME, TYPE, STATE
   - `--name` 필터링 지원

2. `flow node <id|name> <nodeID>` 서브커맨드:
   - `GET /api/v1/flows/{id}/nodes/{nodeID}` 호출
   - 상세 정보 출력 (text/json/yaml)

### 5단계: 테스트

**[engine_test.go](internal/engine/engine_test.go)** 추가:
- `TestGetFlowNodes_정상` — 배포된 플로우의 노드 목록
- `TestGetFlowNodes_미배포에러` — 존재하지 않는 플로우 에러
- `TestGetFlowNode_정상` — 특정 노드 조회
- `TestGetFlowNode_미존재에러` — 존재하지 않는 노드 에러

**[flow_test.go (handler)](internal/api/handler/flow_test.go)** 추가:
- `TestFlowHandler_ListNodes` — 노드 목록 200
- `TestFlowHandler_GetNode_Success` — 노드 상세 200
- `TestFlowHandler_GetNode_NotFound` — 미존재 404

**[flow_adapter_test.go](internal/api/service/flow_adapter_test.go)** 추가:
- `TestFlowServiceAdapter_ListFlowNodes` — 어댑터 변환
- `TestFlowServiceAdapter_GetFlowNode` — 어댑터 변환
- `TestFlowServiceAdapter_FlowStatus_NodeStats` — NodeStats 채워지는지 확인

**[flow_test.go (CLI)](internal/cli/flow_test.go)** 추가:
- `TestFlowNodesCmd` — flow nodes 테이블 출력
- `TestFlowNodeCmd` — flow node 상세 출력

## 검증 방법

```bash
go test -race -cover ./internal/engine/...
go test -race -cover ./internal/api/handler/...
go test -race -cover ./internal/api/service/...
go test -race -cover ./internal/cli/...
go vet ./...
go build ./...
```

수동 검증:
```bash
# 플로우 배포 후
xflow flow nodes <flowID>
xflow flow node <flowID> <nodeID>
xflow flow status <flowID>  # NodeStats 포함 확인
```
