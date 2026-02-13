---
id: SPEC-API-001
type: plan
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
---

# SPEC-API-001: REST API Service System - 구현 계획

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 준수):
- **신규 코드 (TDD)**: `internal/api/` 전체가 신규 작성이므로 RED-GREEN-REFACTOR 사이클 적용
- 테스트 커버리지 목표: 85% 이상
- 모든 핸들러, 미들웨어, DTO에 대해 테스트를 먼저 작성한 후 구현

### 1.2 핵심 설계 결정

1. **HTTP 프레임워크 추상화**: `Router`, `Context`, `HandlerFunc` 인터페이스를 먼저 정의하고, Fiber v3 또는 Echo v4 구현을 어댑터로 작성
2. **의존성 주입**: 모든 핸들러는 인터페이스(FlowManager, AgentManager 등)를 통해 의존성을 주입받아 테스트 용이성 확보
3. **표준 응답 엔벨로프**: 제네릭 `APIResponse[T]`를 모든 응답에 적용하여 일관성 보장
4. **에러 매핑 계층**: `pkg/xferr/` 도메인 에러 -> `APIError` -> HTTP 응답으로 변환하는 중앙 매핑 레이어

### 1.3 기술 스택

| 구성 요소 | 선택 | 비고 |
|-----------|------|------|
| HTTP Framework | Fiber v3 또는 Echo v4 | 벤치마크 후 최종 결정, 인터페이스 추상화로 교체 가능 |
| JWT | golang-jwt/jwt/v5 v5.2+ | 토큰 검증 |
| WebSocket | gorilla/websocket v1.5+ | 실시간 스트리밍 |
| Validation | go-playground/validator v10+ | 구조체 태그 기반 유효성 검증 |
| Metrics | prometheus/client_golang v1.18+ | Prometheus 메트릭 |
| Logging | log/slog | Go 표준 구조화 로깅 |
| Testing | stretchr/testify v1.9+ | 테스트 어설션 |

---

## 2. 파일별 구현 상세

### 2.1 P0 핵심 파일 (1차 마일스톤)

#### `internal/api/errors.go` (~120 라인)
- `APIError` 구조체 정의
- 사전 정의된 에러 인스턴스 (ErrBadRequest, ErrUnauthorized 등 9개)
- `MapDomainError()`: `pkg/xferr/` -> `APIError` 변환
- `WithMessage()`, `WithDetails()` 에러 래핑 헬퍼
- 의존성: `pkg/xferr/` (SPEC-ERR-001)

#### `internal/api/dto/response.go` (~150 라인)
- `APIResponse[T]` 제네릭 응답 엔벨로프
- `ErrorDetail` 에러 상세 구조체
- `Meta`, `PaginationMeta` 메타 정보
- `Success()`, `Error()`, `Paginated()` 응답 생성 헬퍼
- 의존성: 없음 (순수 데이터 구조체)

#### `internal/api/dto/request.go` (~200 라인)
- `PaginationReq`: 페이지네이션 파라미터 (page, size, sort, order)
- `FlowCreateReq`, `FlowUpdateReq`: 플로우 CRUD 요청
- `AgentCreateReq`, `AgentUpdateReq`: Agent CRUD 요청
- `ConfigUpdateReq`: 런타임 설정 변경 요청
- `ListOptions`: 공통 목록 조회 옵션 (필터, 정렬, 페이지네이션)
- 의존성: `pkg/flow/` (SPEC-FLOW-001)

#### `internal/api/dto/validation.go` (~80 라인)
- 커스텀 유효성 검증 태그 등록
- `ValidateStruct()` 검증 실행 및 에러 포맷팅
- 검증 에러 -> `ErrorDetail` 변환
- 의존성: `go-playground/validator`

#### `internal/api/server.go` (~200 라인)
- `Server` 구조체: HTTP 서버 생명주기 관리
- `NewServer()`: 설정 기반 서버 초기화
- `Start()`: HTTP 리스너 시작, TLS 옵션
- `Stop()`: graceful shutdown (타임아웃 내 진행 중 요청 완료)
- `ListenAddr()`: 현재 리스닝 주소 반환
- `healthCheck()`, `readyCheck()`: 생존/준비 상태 확인
- `lifecycle.Lifecycle` 인터페이스 구현
- 의존성: `internal/config/`, `internal/observe/`, `pkg/lifecycle/`

#### `internal/api/router.go` (~180 라인)
- `Router` 인터페이스 정의 (HTTP 프레임워크 추상화)
- `Context` 인터페이스 정의
- `SetupRoutes()`: 라우트 등록 (버전 프리픽스, 그룹별 핸들러, 미들웨어 적용)
- `RegisterFlowRoutes()`, `RegisterAgentRoutes()` 등 도메인별 등록 함수
- 정적 파일 서빙 설정
- 의존성: 모든 핸들러, 미들웨어

#### `internal/api/middleware.go` (~350 라인)
- `RequestIDMiddleware()`: UUID v4 생성, X-Request-ID 헤더
- `LoggerMiddleware()`: slog 구조화 로깅 (메서드, 경로, 상태, 응답 시간)
- `AuthMiddleware()`: JWT/API 키 검증 (`internal/auth/` 위임)
- `RBACMiddleware()`: 역할 기반 접근 제어
- `CORSMiddleware()`: 교차 출처 요청 허용 (설정 기반)
- `RateLimitMiddleware()`: 클라이언트별 요청 제한 (토큰 버킷/슬라이딩 윈도우)
- `RecoveryMiddleware()`: panic 복구, 500 에러 반환
- `CompressMiddleware()`: gzip 압축
- `TimeoutMiddleware()`: 요청 타임아웃 (기본 30초)
- 의존성: `internal/auth/`, `internal/observe/`, `internal/config/`

#### `internal/api/handler/flow.go` (~300 라인)
- `FlowHandler` 구조체: `FlowManager` 인터페이스 의존
- `NewFlowHandler()`: 의존성 주입 생성자
- `List()`: GET /flows (페이지네이션, 필터링)
- `Get()`: GET /flows/:id
- `Create()`: POST /flows (JSON/YAML 파싱, 유효성 검증)
- `Update()`: PUT /flows/:id
- `Delete()`: DELETE /flows/:id (실행 중 검사)
- `Deploy()`: POST /flows/:id/deploy
- `Start()`: POST /flows/:id/start
- `Stop()`: POST /flows/:id/stop
- `Restart()`: POST /flows/:id/restart
- `Configure()`: PUT /flows/:id/config
- `Status()`: GET /flows/:id/status
- 의존성: `internal/engine/` (SPEC-ENGINE-001), `pkg/flow/` (SPEC-FLOW-001)

#### `internal/api/handler/agent.go` (~280 라인)
- `AgentHandler` 구조체: `AgentManager` 인터페이스 의존
- `NewAgentHandler()`: 의존성 주입 생성자
- `List()`, `Get()`, `Create()`, `Update()`, `Delete()` CRUD
- `Start()`, `Stop()`, `Restart()` 생명주기 제어
- `Configure()`: PUT /agents/:id/config (런타임 설정 변경)
- `Stats()`: GET /agents/:id/stats
- 의존성: `internal/agent/` (SPEC-AGENT-001)

### 2.2 P1 확장 파일 (2차 마일스톤)

#### `internal/api/handler/node.go` (~150 라인)
- `NodeHandler` 구조체: `NodeRegistry`, `NodeManager` 인터페이스 의존
- `ListTypes()`, `GetType()`: 노드 타입 카탈로그
- `ListFlowNodes()`, `GetFlowNode()`: 플로우 내 노드 조회
- `Configure()`: 노드 런타임 설정 변경
- 의존성: `internal/node/` (SPEC-NODE-001)

#### `internal/api/handler/monitor.go` (~200 라인)
- `MonitorHandler` 구조체
- `Metrics()`: Prometheus 형식 메트릭 노출
- `Health()`: 시스템 전체 건강 상태
- `Status()`: 시스템 상태 개요
- `SetLogLevel()`, `GetLogLevels()`: 로그 레벨 제어
- `SSEEvents()`: SSE 이벤트 스트리밍
- 의존성: `internal/observe/` (SPEC-OBS-001)

#### `internal/api/handler/plugin.go` (~120 라인)
- `PluginHandler` 구조체
- `List()`, `Get()`, `Install()`, `Remove()` 플러그인 관리
- 의존성: `internal/plugin/` (미래 SPEC-PLUGIN-001)

#### `internal/api/handler/ws.go` (~250 라인)
- `WebSocketHandler` 구조체
- `Upgrade()`: WebSocket 핸드셰이크 + 인증
- `handleConnection()`: 연결별 goroutine 관리
- `subscribe()`, `unsubscribe()`: 채널 구독 관리
- `broadcast()`: 구독 채널별 메시지 전송
- heartbeat ping/pong 처리
- 연결 풀 관리 및 정리
- 의존성: `gorilla/websocket`

#### `internal/api/info.go` (~100 라인)
- `ServerInfo` 구조체: 런타임 스냅샷
- `ServerStats` 구조체: 요청 통계
- `EndpointStat`: 엔드포인트별 통계
- 통계 수집 및 조회 메서드
- 의존성: 없음

#### `internal/api/handler/auth.go` (~120 라인)
- `AuthHandler` 구조체: 인증 관련 엔드포인트
- `Login()`, `Logout()`, `Refresh()`, `CreateAPIKey()`
- `internal/auth/` 패키지에 모든 로직 위임
- 의존성: `internal/auth/` (SPEC-AUTH-001)

### 2.3 테스트 파일

#### `internal/api/api_test.go` (~500 라인)
- 서버 시작/종료 통합 테스트
- health/ready 엔드포인트 테스트
- 미들웨어 체인 테스트 (인증, CORS, 레이트 리밋)
- 핸들러 단위 테스트 (목 인터페이스 사용)
- 에러 매핑 테스트
- DTO 유효성 검증 테스트
- 엔드투엔드 요청/응답 흐름 테스트

---

## 3. 마일스톤 단계

### Milestone 1: 핵심 인프라 (Primary Goal)

**목표**: API 서버의 기본 골격을 구축하여 요청을 수신하고 응답할 수 있는 상태

**작업 순서**:
1. `errors.go` + 테스트: 에러 타입 정의 (의존성 없음)
2. `dto/response.go` + 테스트: 응답 엔벨로프 (의존성 없음)
3. `dto/request.go` + `dto/validation.go` + 테스트: 요청 DTO 및 유효성 검증
4. `router.go`: Router/Context 인터페이스 정의 + Fiber/Echo 어댑터
5. `middleware.go` + 테스트: 핵심 미들웨어 (RequestID, Logger, Recovery, CORS)
6. `server.go` + 테스트: 서버 생명주기, health/ready

**완료 기준**: HTTP 서버가 시작/종료되고, health check가 동작하며, 미들웨어 체인이 올바르게 작동

### Milestone 2: 핵심 핸들러 (Secondary Goal)

**목표**: Flow와 Agent의 전체 CRUD 및 실행 제어 API 완성

**작업 순서**:
1. `handler/flow.go` + 테스트: 플로우 CRUD + 실행 제어 (12개 엔드포인트)
2. `handler/agent.go` + 테스트: Agent CRUD + 생명주기 제어 (10개 엔드포인트)
3. `middleware.go` 확장: Auth + RBAC + RateLimit + Timeout + Compress 미들웨어 추가
4. 통합 테스트: 전체 요청 흐름 (인증 -> 핸들러 -> 응답) 검증

**완료 기준**: Flow 및 Agent에 대한 전체 CRUD + 실행 제어가 인증/인가와 함께 동작

### Milestone 3: 확장 핸들러 (Tertiary Goal)

**목표**: Node, Monitor, Plugin 핸들러와 실시간 기능 완성

**작업 순서**:
1. `handler/node.go` + 테스트: 노드 카탈로그 + 플로우 내 노드 관리
2. `handler/monitor.go` + 테스트: 메트릭, 상태, 로그 레벨 제어, SSE 스트리밍
3. `handler/plugin.go` + 테스트: 플러그인 관리
4. `handler/auth.go` + 테스트: 인증 엔드포인트 (internal/auth 위임)

**완료 기준**: 모든 P1 엔드포인트가 동작하며, SSE 이벤트 스트리밍이 가능

### Milestone 4: 실시간 & 통계 (Optional Goal)

**목표**: WebSocket 실시간 스트리밍과 서버 통계 기능 완성

**작업 순서**:
1. `handler/ws.go` + 테스트: WebSocket 업그레이드, 연결 관리, 채널 구독, heartbeat
2. `info.go` + 테스트: ServerInfo, ServerStats, 엔드포인트별 통계
3. 정적 파일 서빙: Web Dashboard 빌드 결과물 내장 서빙
4. 전체 통합 테스트 및 성능 검증

**완료 기준**: WebSocket 실시간 스트리밍, 서버 통계, 정적 파일 서빙이 모두 동작

---

## 4. 의존성 그래프

```
errors.go (독립)  <----+
dto/response.go (독립) |
dto/request.go --------+
dto/validation.go -----+
                       |
server.go <------------+-- router.go <-- middleware.go
    |                                        |
    v                                        v
  handler/flow.go  ------> FlowManager (internal/engine)
  handler/agent.go ------> AgentManager (internal/agent)
  handler/node.go  ------> NodeRegistry (internal/node)
  handler/monitor.go ----> observe (internal/observe)
  handler/plugin.go -----> plugin (internal/plugin)
  handler/auth.go  ------> auth (internal/auth)
  handler/ws.go    ------> gorilla/websocket
  info.go (독립)
```

**구현 순서 결정 원칙**:
- 의존성이 없는 파일(errors.go, dto/)을 먼저 구현
- 인터페이스 정의(router.go)를 핸들러보다 먼저 구현
- 각 핸들러는 목(mock) 인터페이스로 독립적 테스트 가능

---

## 5. 리스크 분석

### Risk 1: HTTP 프레임워크 선택 지연
- **설명**: Fiber v3 vs Echo v4 벤치마크 결과 지연으로 구현 시작 차질
- **영향**: 높음
- **대응**: Router/Context 인터페이스를 먼저 정의하고, 어댑터 패턴으로 프레임워크를 추상화하여 나중에 교체 가능하도록 설계. 초기에는 Echo v4로 시작 (표준 라이브러리 호환성 우선)

### Risk 2: 의존 패키지 인터페이스 미확정
- **설명**: SPEC-ENGINE-001, SPEC-AGENT-001 등의 인터페이스가 아직 확정되지 않은 상태에서 핸들러 구현
- **영향**: 중간
- **대응**: 핸들러의 의존성을 인터페이스(FlowManager, AgentManager)로 정의하고, 목(mock) 구현으로 테스트. 실제 패키지 구현 시 어댑터만 추가

### Risk 3: WebSocket 연결 관리 복잡성
- **설명**: 다수 WebSocket 연결의 동시 관리, 메모리 누수, goroutine 누수 위험
- **영향**: 중간
- **대응**: 연결 풀 패턴 적용, heartbeat 기반 연결 상태 모니터링, goroutine 수 메트릭 추적, 타임아웃 기반 자동 정리

### Risk 4: 레이트 리밋 정확도
- **설명**: 분산 환경에서 클라이언트별 레이트 리밋의 정확도 보장 어려움
- **영향**: 낮음 (MVP는 단일 인스턴스)
- **대응**: 초기에는 인메모리 토큰 버킷으로 구현. 향후 분산 환경에서는 Redis 기반 슬라이딩 윈도우로 전환

### Risk 5: SSE 연결 유지 비용
- **설명**: 다수 SSE 연결이 장시간 유지되면 서버 리소스(goroutine, 메모리) 소모
- **영향**: 중간
- **대응**: SSE 연결 수 제한, 비활성 연결 자동 종료, 이벤트 버퍼 크기 제한, 연결 수 메트릭 모니터링

### Risk 6: 인증/인가 패키지 의존성
- **설명**: SPEC-AUTH-001이 별도 구현되므로 API 미들웨어와의 통합 시점 불확실
- **영향**: 중간
- **대응**: 인증 미들웨어는 `Authenticator` 인터페이스를 정의하고, 초기에는 개발용 목(mock) 인증기를 사용. AUTH 패키지 구현 완료 후 실제 구현체로 교체

### Risk 7: 성능 목표 미달성
- **설명**: API 응답 시간 p95 < 50ms 목표 달성 여부
- **영향**: 중간
- **대응**: 핵심 경로에 프로파일링 적용, 미들웨어 체인 최적화, 불필요한 직렬화/역직렬화 제거, 벤치마크 테스트 자동화

---

## 6. 파일 의존성 매트릭스

| 파일 | 의존하는 내부 파일 | 의존하는 외부 패키지 | 의존하는 SPEC |
|------|-------------------|---------------------|---------------|
| errors.go | - | pkg/xferr | SPEC-ERR-001 |
| dto/response.go | - | - | - |
| dto/request.go | - | pkg/flow | SPEC-FLOW-001 |
| dto/validation.go | dto/response.go | go-playground/validator | - |
| server.go | router.go, middleware.go, info.go | internal/config, internal/observe, pkg/lifecycle | SPEC-CFG-001, SPEC-OBS-001, SPEC-LIFE-001 |
| router.go | handler/*.go, middleware.go | HTTP framework (Fiber/Echo) | - |
| middleware.go | errors.go, dto/response.go | internal/auth, internal/observe, internal/config | SPEC-AUTH-001, SPEC-OBS-001, SPEC-CFG-001 |
| handler/flow.go | errors.go, dto/*.go | internal/engine, pkg/flow | SPEC-ENGINE-001, SPEC-FLOW-001 |
| handler/agent.go | errors.go, dto/*.go | internal/agent | SPEC-AGENT-001 |
| handler/node.go | errors.go, dto/*.go | internal/node | SPEC-NODE-001 |
| handler/monitor.go | errors.go, dto/*.go | internal/observe | SPEC-OBS-001 |
| handler/plugin.go | errors.go, dto/*.go | internal/plugin | SPEC-PLUGIN-001 |
| handler/ws.go | errors.go | gorilla/websocket | - |
| handler/auth.go | errors.go, dto/*.go | internal/auth | SPEC-AUTH-001 |
| info.go | - | - | - |

---

## 7. 총 예상 코드 규모

| 카테고리 | 파일 수 | 예상 라인 수 |
|----------|---------|-------------|
| P0 핵심 파일 | 9 | ~1,860 |
| P1 확장 파일 | 6 | ~940 |
| 테스트 파일 | 1+ | ~500+ |
| **합계** | **16+** | **~3,300+** |

---

*SPEC ID: SPEC-API-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-02-13*
