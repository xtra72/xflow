# Plan: xflowd + xflow-agent 바이너리 구현

## Context

xflow 프로젝트는 3개의 바이너리를 정의하지만 현재 `cmd/xflow/`(CLI)만 구현되어 있다.
나머지 2개 - `cmd/xflowd/`(데몬 서버)와 `cmd/xflow-agent/`(경량 에지 에이전트)를 구현한다.

이미 구현된 내부 패키지들(api, engine, config, observe, node, agent)을 조합하여
각 바이너리의 main.go 엔트리포인트를 작성한다.

## 구현 대상

### 1. cmd/xflowd/main.go - 데몬 서버

xflow 플랫폼의 중앙 서버. API 서버 + Flow Engine + Agent Manager를 실행한다.

**초기화 순서:**
1. cobra 루트 커맨드 생성 (flags: --config, --foreground, --host, --port)
2. `config.Load()` → 설정 로딩 (`internal/config/config.go:165`)
3. `observe.New()` → 관찰성 초기화 (`internal/observe/observe.go:33`)
4. `node.NewRegistry()` → 노드 레지스트리 (내장 10종 자동 등록, `internal/node/registry.go`)
5. `engine.NewEngine(opts...)` → Flow 엔진 (`internal/engine/engine.go`)
6. `agent.NewManager()` → Agent 매니저 (`internal/agent/manager.go`)
7. `api.NewServer(cfg.Server(), opts...)` → API 서버 (`internal/api/server.go:55`)
8. `server.SetupRoutes()` → 기본 라우트 (/health, /ready)
9. `server.RegisterRoutes()` → Flow/Agent 핸들러 등록
   - `handler.NewFlowHandler().RegisterRoutes(g)` (`internal/api/handler/flow.go:91`)
   - `handler.NewAgentHandler().RegisterRoutes(g)` (`internal/api/handler/agent.go:77`)
10. `server.Start(ctx)` → HTTP 서버 시작 (블로킹, ctx 취소 시 자동 Stop)

**시그널 처리:**
- `os.Signal` (SIGINT, SIGTERM) 수신 → context cancel → graceful shutdown
- `server.Start(ctx)`는 `<-ctx.Done()` 시 `server.Stop()` 호출 (이미 구현됨)
- 추가 정리: `agentManager.Shutdown(ctx)` 호출

**cobra 커맨드 구조:**
```
xflowd           # 서버 시작 (기본)
xflowd version   # 버전 정보
```

**플래그:**
- `--config` - 설정 파일 경로 (기본: `~/.xflow/config.yaml`)
- `--host` - 바인드 호스트 (기본: 설정 파일 값 또는 localhost)
- `--port` - 바인드 포트 (기본: 설정 파일 값 또는 8080)
- `--foreground` - 포그라운드 모드 (초기 구현에서는 항상 foreground)
- `--log-level` - 로그 레벨 (debug, info, warn, error)

### 2. cmd/xflow-agent/main.go - 경량 에지 에이전트

에지 환경에서 데이터를 수집하여 중앙 xflowd 서버로 전송하는 경량 바이너리.

**초기화 순서:**
1. cobra 루트 커맨드 생성 (flags: --config, --server, --agent-id)
2. 설정 로딩 (경량: server URL, agent ID, heartbeat interval)
3. `observe.New()` → 경량 로거 (메트릭/트레이싱 비활성)
4. `node.NewRegistry(node.WithoutBuiltins())` → 최소 노드 (filter, transform만 수동 등록)
5. `engine.NewEngine(opts...)` → 경량 Flow 엔진
6. HTTP 클라이언트로 중앙 서버 연결 확인
7. 하트비트 루프 시작 (주기적으로 서버에 상태 보고)
8. 시그널 수신 시 graceful shutdown

**cobra 커맨드 구조:**
```
xflow-agent           # 에이전트 시작
xflow-agent version   # 버전 정보
xflow-agent status    # 로컬 상태 확인
```

**플래그:**
- `--config` - 설정 파일 경로 (기본: `~/.xflow/agent.yaml`)
- `--server` - 중앙 서버 URL (기본: `http://localhost:8080`)
- `--agent-id` - 에이전트 고유 ID (기본: 호스트명 기반 자동 생성)
- `--heartbeat` - 하트비트 간격 (기본: 30s)
- `--log-level` - 로그 레벨

## 파일 목록

| 파일 | 설명 | 예상 라인 수 |
|------|------|-------------|
| `cmd/xflowd/main.go` | 데몬 서버 엔트리포인트 | ~120 |
| `cmd/xflow-agent/main.go` | 에지 에이전트 엔트리포인트 | ~150 |

## 재사용할 기존 코드

| 패키지 | 함수/타입 | 용도 |
|--------|----------|------|
| `internal/config` | `Load()`, `WithConfigFile()`, `Config` 인터페이스 | 설정 로딩 |
| `internal/observe` | `New()`, `Observer` | 로깅/메트릭 |
| `internal/api` | `NewServer()`, `SetupRoutes()`, `RegisterRoutes()` | HTTP 서버 |
| `internal/api/handler` | `NewFlowHandler()`, `NewAgentHandler()` | API 핸들러 |
| `internal/engine` | `NewEngine()`, `WithNodeRegistry()`, `WithLogger()` | Flow 엔진 |
| `internal/node` | `NewRegistry()`, `WithoutBuiltins()` | 노드 레지스트리 |
| `internal/agent` | `NewManager()` | Agent 관리 |
| `internal/cli` | `Version`, `Commit`, `BuildDate` | 빌드 정보 공유 |

## 검증 방법

```bash
# 빌드 검증
go build ./cmd/xflowd/
go build ./cmd/xflow-agent/

# 버전 출력 확인
./xflowd version
./xflow-agent version

# 서버 시작 확인 (Ctrl+C로 종료)
./xflowd --foreground --port 9090

# go vet
go vet ./cmd/xflowd/ ./cmd/xflow-agent/
```

## 테스트 전략

엔트리포인트 바이너리는 내부 패키지 통합 테스트로 대체한다.
(각 내부 패키지는 이미 85%+ 커버리지를 달성함)
빌드 가능 여부와 기본 플래그 동작만 검증한다.
