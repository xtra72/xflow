# api - REST API 서비스 시스템

`internal/api` 패키지는 XFlow 플랫폼의 REST API 서비스 레이어를 제공한다. Go 1.22+ net/http 표준 라이브러리 기반 HTTP 서버, 라우터, 미들웨어 스택, 핸들러, DTO, 에러 매핑을 통합 지원한다.

**SPEC**: SPEC-API-001

## 아키텍처 개요

```
    REST API 서비스 아키텍처 (net/http 기반)

    +---------------------------------------------------------+
    |                      Server                              |
    |  lifecycle.State 관리 (Created/Running/Stopping/Stopped) |
    |  Graceful Shutdown, Health Check, Stats Collection       |
    +---------------------------------------------------------+
            |                             |
    +-------v--------+           +--------v---------+
    |     Router      |           |  statsCollector   |
    |  Go 1.22+       |           |  atomic 기반      |
    |  ServeMux       |           |  lock-free 통계   |
    +----------------+           +------------------+
            |
    +-------+------------------+
    |                          |
    v                          v
Middleware Stack          RouteGroup
(8개 미들웨어)           (/api/v1/ 프리픽스)
                               |
                    +----------+----------+
                    |                     |
                    v                     v
              FlowHandler           AgentHandler
              (11 엔드포인트)        (10 엔드포인트)
                    |                     |
                    v                     v
              FlowManager           AgentManager
              (인터페이스)           (인터페이스)
```

**핵심 구성 요소**:

1. **Server**: HTTP 서버 생명주기 관리 (Start/Stop/Graceful Shutdown)
2. **Router**: Go 1.22+ ServeMux 기반 라우팅, RouteGroup으로 경로 그룹화
3. **Context 인터페이스**: 15개 메서드를 가진 요청 컨텍스트 추상화
4. **Middleware Stack**: 8개 미들웨어 (Recovery, RequestID, Logger, Compress, CORS, RateLimit, Timeout, Auth)
5. **FlowHandler**: 플로우 CRUD + 실행 제어 (deploy/start/stop/restart/config/status)
6. **AgentHandler**: Agent CRUD + 생명주기 제어 (start/stop/restart/config/stats)
7. **DTO 패키지**: Generic APIResponse[T] 엔벨로프, 유효성 검증, 페이지네이션
8. **에러 시스템**: 10개 센티널 에러, 도메인 에러 -> HTTP 에러 매핑

## 빠른 시작

### Server 생성 및 시작

`NewServer()` 함수는 Options 패턴으로 설정을 받아 Server를 생성한다. `SetupRoutes()`로 미들웨어와 기본 라우트를 구성한 후, `Start()`로 HTTP 리스닝을 시작한다.

```go
package main

import (
    "context"
    "log/slog"

    "github.com/xtra/xflow/internal/api"
    "github.com/xtra/xflow/internal/api/handler"
    "github.com/xtra/xflow/internal/config"
)

func main() {
    ctx := context.Background()

    cfg := &config.ServerConfig{
        Host: "0.0.0.0",
        Port: 8080,
        Mode: "development",
        CORS: config.CORSConfig{Enabled: true, AllowedOrigins: []string{"*"}},
        RateLimit: config.RateLimitConfig{Enabled: true, RequestsPerSecond: 100},
    }

    srv := api.NewServer(cfg,
        api.WithLogger(slog.Default()),
    )

    // 미들웨어 및 기본 라우트 구성
    srv.SetupRoutes()

    // 핸들러 라우트 등록
    srv.RegisterRoutes(func(g *api.RouteGroup) {
        flowHandler := handler.NewFlowHandler(myFlowManager, slog.Default())
        flowHandler.RegisterRoutes(g)

        agentHandler := handler.NewAgentHandler(myAgentManager, slog.Default())
        agentHandler.RegisterRoutes(g)
    })

    // 서버 시작 (블로킹)
    if err := srv.Start(ctx); err != nil {
        panic(err)
    }
}
```

### Health Check / Ready Check

서버는 `/health`(라이브니스)와 `/ready`(레디니스) 엔드포인트를 기본 제공한다.

```go
// 라이브니스 체크
// GET /health -> {"success": true, "data": {"status": "ok"}}

// 레디니스 체크 (의존성 상태 포함)
// GET /ready -> {"success": true, "data": {"status": "ok", "checks": {...}}}

// 의존성 등록
srv := api.NewServer(cfg,
    api.WithHealthDependency("database", dbChecker),
    api.WithHealthDependency("engine", engineChecker),
)
```

## API 엔드포인트 레퍼런스

### 서버 기본 엔드포인트 (2개)

| 메서드 | 경로 | 핸들러 | 설명 |
|--------|------|--------|------|
| GET | `/health` | Server.healthCheck | 서버 라이브니스 체크 |
| GET | `/ready` | Server.readyCheck | 서버 레디니스 체크 (의존성 포함) |

### Flow 엔드포인트 (11개)

| 메서드 | 경로 | 핸들러 | 설명 |
|--------|------|--------|------|
| GET | `/api/v1/flows` | FlowHandler.List | 플로우 목록 조회 (페이지네이션, 필터링) |
| GET | `/api/v1/flows/{id}` | FlowHandler.Get | 플로우 상세 조회 |
| POST | `/api/v1/flows` | FlowHandler.Create | 플로우 생성 |
| PUT | `/api/v1/flows/{id}` | FlowHandler.Update | 플로우 업데이트 |
| DELETE | `/api/v1/flows/{id}` | FlowHandler.Delete | 플로우 삭제 |
| POST | `/api/v1/flows/{id}/deploy` | FlowHandler.Deploy | 플로우 배포 |
| POST | `/api/v1/flows/{id}/start` | FlowHandler.Start | 플로우 시작 |
| POST | `/api/v1/flows/{id}/stop` | FlowHandler.Stop | 플로우 정지 |
| POST | `/api/v1/flows/{id}/restart` | FlowHandler.Restart | 플로우 재시작 |
| PUT | `/api/v1/flows/{id}/config` | FlowHandler.Configure | 플로우 런타임 설정 변경 |
| GET | `/api/v1/flows/{id}/status` | FlowHandler.Status | 플로우 상세 상태 조회 |

### Agent 엔드포인트 (10개)

| 메서드 | 경로 | 핸들러 | 설명 |
|--------|------|--------|------|
| GET | `/api/v1/agents` | AgentHandler.List | Agent 목록 조회 (페이지네이션) |
| GET | `/api/v1/agents/{id}` | AgentHandler.Get | Agent 상세 조회 |
| POST | `/api/v1/agents` | AgentHandler.Create | Agent 생성 |
| PUT | `/api/v1/agents/{id}` | AgentHandler.Update | Agent 업데이트 |
| DELETE | `/api/v1/agents/{id}` | AgentHandler.Delete | Agent 삭제 |
| POST | `/api/v1/agents/{id}/start` | AgentHandler.Start | Agent 시작 |
| POST | `/api/v1/agents/{id}/stop` | AgentHandler.Stop | Agent 정지 |
| POST | `/api/v1/agents/{id}/restart` | AgentHandler.Restart | Agent 재시작 |
| PUT | `/api/v1/agents/{id}/config` | AgentHandler.Configure | Agent 런타임 설정 변경 |
| GET | `/api/v1/agents/{id}/stats` | AgentHandler.Stats | Agent 실행 통계 조회 |

## 미들웨어 스택

요청은 다음 순서로 미들웨어를 통과한다:

```
Request
  -> Recovery (패닉 복구, 500 에러 반환)
  -> RequestID (UUID v4 생성, X-Request-ID 헤더)
  -> Logger (slog 기반 구조화된 요청/응답 로깅)
  -> Compress (Accept-Encoding: gzip 시 gzip 압축)
  -> CORS (교차 출처 요청 허용, 설정 기반)
  -> RateLimit (토큰 버킷 기반 클라이언트별 속도 제한)
  -> Timeout (요청 처리 타임아웃, 기본 30초)
  -> Auth (JWT/API 키 검증 - P1에서는 패스스루)
  -> Handler (실제 요청 처리)
Response
```

### 미들웨어 상세

| 미들웨어 | 설명 | 비고 |
|----------|------|------|
| `Recovery` | 핸들러 패닉 복구, 500 표준 에러 응답 반환 | 항상 활성 |
| `RequestID` | UUID v4 요청 ID 생성, X-Request-ID 헤더 설정 | 기존 ID 우선 사용 |
| `Logger` | 메서드, 경로, 상태 코드, 응답 시간, 요청 ID, 클라이언트 IP 로깅 | slog 기반 |
| `Compress` | gzip 압축 적용 (Accept-Encoding: gzip 시) | Content-Length 제거 |
| `CORS` | Access-Control-Allow-* 헤더 설정, OPTIONS 프리플라이트 처리 | 설정 기반 활성화 |
| `RateLimit` | 토큰 버킷 알고리즘, 클라이언트 IP별 속도 제한, 429 반환 | Retry-After 헤더 포함 |
| `Timeout` | context.WithTimeout 기반, 초과 시 408 반환 | 기본 30초 |
| `Auth` | JWT/API 키 검증 플레이스홀더 | SPEC-AUTH-001에서 구현 예정 |

## 표준 응답 형식

### 성공 응답

모든 API 응답은 `APIResponse[T]` 제네릭 엔벨로프로 래핑된다.

```json
{
    "success": true,
    "data": { ... },
    "meta": {
        "request_id": "uuid-v4",
        "pagination": {
            "page": 1,
            "size": 20,
            "total": 150,
            "total_pages": 8
        }
    }
}
```

### 에러 응답

```json
{
    "success": false,
    "error": {
        "code": "VALIDATION_FAILED",
        "message": "validation failed",
        "details": [
            {"field": "name", "message": "is required"}
        ]
    }
}
```

## 에러 코드 레퍼런스

| HTTP 코드 | 에러 코드 | 센티널 변수 | 설명 |
|-----------|----------|------------|------|
| 400 | `BAD_REQUEST` | `ErrBadRequest` | 잘못된 요청 |
| 401 | `UNAUTHORIZED` | `ErrUnauthorized` | 인증 실패 |
| 403 | `FORBIDDEN` | `ErrForbidden` | 권한 부족 |
| 404 | `NOT_FOUND` | `ErrNotFound` | 리소스 미존재 |
| 408 | `REQUEST_TIMEOUT` | `ErrRequestTimeout` | 요청 타임아웃 |
| 409 | `CONFLICT` | `ErrConflict` | 리소스 충돌 |
| 422 | `VALIDATION_FAILED` | `ErrValidationFailed` | 유효성 검증 실패 |
| 429 | `RATE_LIMIT_EXCEEDED` | `ErrRateLimitExceeded` | 속도 제한 초과 |
| 500 | `INTERNAL_ERROR` | `ErrInternalServer` | 내부 서버 오류 |
| 503 | `SERVICE_UNAVAILABLE` | `ErrServiceUnavailable` | 서비스 불가 |

### 도메인 에러 매핑

`MapDomainError()` 함수는 `pkg/xferr` 도메인 에러를 적절한 HTTP 에러로 변환한다:

- `xferr.ErrNilMessage`, `ErrInvalidSeverity` 등 -> 400 Bad Request
- `xferr.ErrNoReceiver` -> 503 Service Unavailable
- `xferr.ErrSameStateTransition` -> 409 Conflict
- `xferr.ErrAlertThresholdExceeded` -> 429 Rate Limit Exceeded
- 기타 -> 500 Internal Server Error

## DTO 레퍼런스

### 요청 DTO

| DTO | 필드 | 유효성 검증 |
|-----|------|------------|
| `FlowCreateRequest` | name (필수), description, definition (필수) | name: 1-255자, definition: 필수 |
| `FlowUpdateRequest` | name, description, definition | name: 1-255자 (선택적) |
| `AgentCreateRequest` | name (필수), type (필수), config | name: 1-255자, type: 필수 |
| `AgentUpdateRequest` | name, config | name: 1-255자 (선택적) |
| `ConfigUpdateRequest` | config (필수) | config: 필수 |
| `PaginationParams` | page (기본 1), size (기본 20, 최대 100) | 자동 정규화 |
| `ListOptions` | PaginationParams + sort, filter, status | 목록 조회 공통 옵션 |

### 응답 타입

| 타입 | 설명 |
|------|------|
| `APIResponse[T]` | 표준 응답 엔벨로프 (success, data, error, meta) |
| `ErrorDetail` | 에러 상세 (code, message, details) |
| `Meta` | 메타데이터 (request_id, pagination) |
| `PaginationMeta` | 페이지네이션 (page, size, total, total_pages) |
| `ServerInfo` | 서버 런타임 스냅샷 (version, uptime, go_version 등) |
| `ServerStats` | 요청 통계 (total_requests, error_rate, avg_latency 등) |
| `EndpointStat` | 엔드포인트별 통계 (method, path, requests, errors) |

## 설정 레퍼런스

### ServerConfig

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `Host` | string | `"0.0.0.0"` | 바인드 호스트 |
| `Port` | int | `8080` | 리스닝 포트 |
| `Mode` | string | `"development"` | 실행 모드 (development/production) |
| `CORS.Enabled` | bool | `false` | CORS 활성화 |
| `CORS.AllowedOrigins` | []string | `[]` | 허용 출처 (`*` 전체 허용) |
| `RateLimit.Enabled` | bool | `false` | 속도 제한 활성화 |
| `RateLimit.RequestsPerSecond` | int | `10` | 초당 최대 요청 수 |

### HTTP 서버 타임아웃

| 설정 | 값 | 설명 |
|------|-----|------|
| ReadTimeout | 15초 | 요청 읽기 타임아웃 |
| WriteTimeout | 15초 | 응답 쓰기 타임아웃 |
| IdleTimeout | 60초 | Keep-Alive 유휴 타임아웃 |
| ShutdownTimeout | 30초 | Graceful Shutdown 타임아웃 |
| RequestTimeout | 30초 | 미들웨어 요청 처리 타임아웃 |

### ServerOption 옵션

| 옵션 함수 | 설명 |
|-----------|------|
| `WithLogger(logger)` | 서버 로거 설정 |
| `WithHealthDependency(name, checker)` | 레디니스 체크 의존성 추가 |

## 사용 예시

### 플로우 CRUD

```go
// 플로우 생성
// POST /api/v1/flows
// Body: {"name": "data-pipeline", "description": "ETL 파이프라인", "definition": {...}}
// Response: {"success": true, "data": {"id": "flow-1", "name": "data-pipeline", "status": "created"}}

// 플로우 목록 조회 (페이지네이션 + 필터)
// GET /api/v1/flows?page=1&size=20&status=running
// Response: {"success": true, "data": [...], "meta": {"pagination": {"page": 1, "size": 20, "total": 50}}}

// 플로우 배포 -> 시작
// POST /api/v1/flows/flow-1/deploy -> {"success": true, "data": {"id": "flow-1", "status": "deployed"}}
// POST /api/v1/flows/flow-1/start  -> {"success": true, "data": {"id": "flow-1", "status": "started"}}

// 플로우 런타임 설정 변경 (재시작 없이)
// PUT /api/v1/flows/flow-1/config
// Body: {"config": {"backpressure_threshold": 1000}}
```

### Agent 생명주기 제어

```go
// Agent 생성
// POST /api/v1/agents
// Body: {"name": "mqtt-agent", "type": "mqtt", "config": {"broker": "tcp://localhost:1883"}}

// Agent 시작/정지/재시작
// POST /api/v1/agents/agent-1/start
// POST /api/v1/agents/agent-1/stop
// POST /api/v1/agents/agent-1/restart

// Agent 통계 조회
// GET /api/v1/agents/agent-1/stats
// Response: {"success": true, "data": {"id": "agent-1", "messages_in": 1500, "messages_out": 1480, "connected": true}}
```

## 설계 특징

- **Go 1.22+ net/http 표준 라이브러리**: 외부 HTTP 프레임워크 없이 표준 ServeMux 사용
- **Generic APIResponse[T]**: 타입 안전한 응답 래핑 (Go 1.18+ 제네릭)
- **의존성 역전**: FlowManager/AgentManager 인터페이스를 통한 핸들러-구현 분리
- **토큰 버킷 속도 제한**: sync.Map 기반 클라이언트별 lock-free 토큰 버킷
- **원자적 통계 수집**: atomic.Int64 + sync.Map 기반 lock-free 요청 통계
- **미들웨어 체인**: 글로벌 -> 그룹 -> 라우트별 3단계 미들웨어 적용
- **구조화된 로깅**: log/slog 기반 구조화된 요청/응답 로깅
- **Graceful Shutdown**: 진행 중인 요청 완료 후 안전 종료 (30초 타임아웃)
- **httpContext 구현**: 15개 Context 인터페이스 메서드의 net/http 구현
- **에러 매핑 레이어**: pkg/xferr 도메인 에러 -> HTTP 상태 코드 자동 변환

## 파일 구조

```
internal/api/
  errors.go              # APIError 타입, 10개 센티널 에러, MapDomainError
  info.go                # ServerInfo, ServerStats, statsCollector (atomic 기반)
  middleware.go          # 8개 미들웨어 (Recovery, RequestID, Logger, Compress, CORS, RateLimit, Timeout, Auth)
  router.go              # Context 인터페이스 (15메서드), httpContext, Router, RouteGroup
  server.go              # Server 구조체, Start/Stop/Graceful Shutdown, Health/Ready
  dto/
    request.go           # PaginationParams, ListOptions, FlowCreate/Update, AgentCreate/Update, ConfigUpdate
    response.go          # APIResponse[T], ErrorDetail, Meta, PaginationMeta
    validation.go        # FieldError, ValidationErrors, ValidateFlowCreate/AgentCreate/ConfigUpdate
  handler/
    flow.go              # FlowManager 인터페이스, FlowHandler (11 엔드포인트)
    agent.go             # AgentManager 인터페이스, AgentHandler (10 엔드포인트)
  errors_test.go         # APIError 테스트, MapDomainError 테스트
  info_test.go           # ServerInfo, ServerStats, statsCollector 테스트
  middleware_test.go     # 미들웨어 테스트 (RequestID, Logger, Recovery, CORS, RateLimit, Timeout, Compress)
  router_test.go         # Router, RouteGroup, httpContext 테스트
  server_test.go         # Server 생명주기, Health/Ready 테스트
  dto/
    request_test.go      # PaginationParams, ListOptions 테스트
    response_test.go     # APIResponse, NewSuccessResponse, NewPaginatedResponse 테스트
    validation_test.go   # ValidateFlowCreate, ValidateAgentCreate, ValidateConfigUpdate 테스트
  handler/
    flow_test.go         # FlowHandler 전체 엔드포인트 테스트
    agent_test.go        # AgentHandler 전체 엔드포인트 테스트
```

## 의존성

- **표준 라이브러리**: `net/http`, `encoding/json`, `compress/gzip`, `context`, `log/slog`, `sync`, `sync/atomic`, `time`
- **외부 의존성**:
  - `github.com/google/uuid` - UUID v4 요청 ID 생성
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - 서버 상태 관리 (State 타입)
  - `pkg/xferr` (SPEC-ERR-001) - 도메인 에러 매핑 (MapDomainError)
  - `internal/config` (SPEC-CFG-001) - 서버 설정 (ServerConfig, CORSConfig, RateLimitConfig)

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/api/...

# Race Detector 포함 테스트
go test -race ./internal/api/...

# 커버리지 확인
go test -cover ./internal/api/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/api/...
go tool cover -html=cover.out
```

### 테스트 결과

- api 패키지 커버리지: 92.7%
- dto 패키지 커버리지: 100%
- handler 패키지 커버리지: 90.8%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 모범 사례

1. **인터페이스 주입**: FlowManager/AgentManager 인터페이스를 통해 테스트 시 모킹 용이
2. **에러 매핑**: 핸들러에서 도메인 에러를 직접 반환하고 MapDomainError()로 자동 변환
3. **유효성 검증 분리**: DTO 패키지에서 유효성 검증 로직을 분리하여 재사용
4. **미들웨어 조합**: 글로벌/그룹/라우트별 미들웨어를 조합하여 유연한 요청 처리 파이프라인 구성
5. **Graceful Shutdown**: 프로덕션 환경에서 반드시 Stop() 호출로 안전한 종료 보장
6. **표준 응답 형식**: 모든 응답을 APIResponse[T]로 래핑하여 일관된 클라이언트 경험 제공
7. **Health Check 의존성**: WithHealthDependency()로 DB, Engine 등 의존성 상태 자동 확인

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Server가 lifecycle.State 타입을 사용하여 상태 관리 |
| SPEC-ERR-001 | 의존 | MapDomainError가 pkg/xferr 에러를 HTTP 에러로 변환 |
| SPEC-CFG-001 | 의존 | ServerConfig, CORSConfig, RateLimitConfig 사용 |
| SPEC-AUTH-001 | 미래 | Auth/RequireRole 미들웨어가 인증/인가 시스템과 통합 예정 |
| SPEC-ENGINE-001 | 소비자 | FlowHandler가 FlowManager 인터페이스를 통해 엔진 제어 |
| SPEC-AGENT-001 | 소비자 | AgentHandler가 AgentManager 인터페이스를 통해 Agent 제어 |
| SPEC-OBS-001 | 소비자 | Logger 미들웨어가 관찰성 시스템과 통합 예정 |
