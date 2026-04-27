# Plan: `agent exec` CLI 커맨드 및 API 엔드포인트 추가

## Context

MODBUS/TCP 서버 에이전트에 4개의 레지스터 읽기 Process 커맨드(`get_coils`, `get_discrete_inputs`, `get_holding_registers`, `get_input_registers`)를 구현했으나, CLI에 `agent exec` 서브커맨드가 없어 실행할 수 없다.

현재 CLI `agent` 서브커맨드: create, delete, export, get, import, list, restart, start, stop
필요: `exec` — 에이전트에 Process 커맨드를 전송하는 서브커맨드

또한 REST API에도 Process 엔드포인트가 없다. 전체 경로(CLI → API → Service Adapter)를 구현해야 한다.

## 수정 파일 (4개)

### 1. `internal/api/handler/agent.go` — AgentManager 인터페이스 + 핸들러 + 라우트

**인터페이스 추가** (line ~25):
```go
ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error)
```

**핸들러 메서드** (`Exec`):
- `POST /agents/{id}/exec`
- Request body: `{"command": "get_input_registers", "params": {"address": 0, "quantity": 10}}`
- `ctx.Bind()` → `json.Marshal()` → `h.agents.ExecAgent()` → `ctx.JSON(200, result)`
- 응답: `dto.NewSuccessResponse(resultMap)`

**라우트 등록** (line ~122):
```go
g.POST("/agents/{id}/exec", h.Exec)
```

### 2. `internal/api/service/agent_adapter.go` — ExecAgent 구현

```go
func (a *AgentServiceAdapter) ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error) {
    ag, err := a.manager.Get(id)
    // ag.Process(data) 호출
    // json.RawMessage로 반환
}
```

기존 패턴: `manager.Get(id)` → `Agent` 인터페이스 메서드 직접 호출 (Start, Stop과 동일)

### 3. `internal/cli/agent.go` — exec 서브커맨드

**사용법**:
```bash
# key=value 간편 형식
xflow agent exec <id|name> <command> [key=value ...]

# JSON 형식
xflow agent exec <id|name> --json '{"command":"...","params":{...}}'
```

**구현**:
- `newAgentExecCmd(client **Client) *cobra.Command`
- `Use: "exec [id|name] [command] [key=value ...]"`
- `Args: cobra.MinimumArgs(1)`
- 첫 번째 arg → agent ID/name (resolveAgentID 사용)
- 두 번째 arg → command name
- 나머지 args → `key=value` 파싱하여 `params` 맵 구성
- `--json` 플래그가 있으면 JSON 문자열을 직접 사용
- `--name` 플래그로 에이전트 이름 지정 가능
- `POST /api/v1/agents/{id}/exec` 호출
- `PrintResult()` 로 결과 출력

**key=value 파싱 로직**:
```
address=0    → params["address"] = 0 (숫자 자동 변환)
quantity=10  → params["quantity"] = 10
state=on     → params["state"] = "on"
flag=true    → params["flag"] = true
```
숫자/bool 자동 감지: `strconv.ParseFloat` → `strconv.ParseBool` → 문자열 폴백

### 4. `internal/api/dto/request.go` — AgentExecRequest DTO

```go
type AgentExecRequest struct {
    Command string         `json:"command" validate:"required"`
    Params  map[string]any `json:"params,omitempty"`
}
```

## 수정하지 않는 파일

- `internal/agent/manager.go` — Manager 인터페이스에 Process 추가 불필요 (adapter에서 `Get()` → `agent.Process()` 직접 호출)
- `internal/cli/client.go` — 기존 `Post()` 메서드로 충분

## Verification

1. `go vet ./internal/...`
2. `go build ./cmd/xflowd/`
3. `go build ./cmd/xflow/`
4. `go test ./internal/api/handler/ -run Agent -count=1`
5. `go test ./internal/api/service/ -run Agent -count=1`
6. `go test ./internal/cli/ -run Agent -count=1`
7. 통합 테스트: 서버 시작 → agent create → agent start → agent exec
