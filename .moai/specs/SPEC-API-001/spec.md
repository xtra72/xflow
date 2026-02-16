---
id: SPEC-API-001
version: "1.0.0"
status: implemented
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-API-001: REST API Service System - HTTP 서버, 라우팅, 미들웨어, 핸들러, DTO, WebSocket

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 REST API 서비스 레이어를 정의한다. API Layer는 Client Layer(Web Dashboard, CLI, External API)와 내부 핵심 시스템(Engine, Agent, Node, Storage) 사이의 게이트웨이 역할을 수행한다. CLI(`xflow`)와 Web Dashboard 모두 원격으로 이 REST API를 통해 `xflowd` 데몬 서버에 접속하여 모든 관리 작업을 수행한다.

본 SPEC은 다음을 포함한다:

- **API Server Core** (`server.go`): HTTP 서버 생명주기 관리, graceful shutdown, health check
- **Router & Versioning** (`router.go`): API 버전 프리픽스(/api/v1/), 도메인별 라우트 그룹, 정적 파일 서빙
- **Middleware Stack** (`middleware.go`): 인증, CORS, 로깅, 레이트 리밋, 에러 핸들링, 요청 ID, 압축, 타임아웃
- **Flow Handler** (`handler/flow.go`): 플로우 CRUD + 실행 제어(deploy/start/stop/restart) + 런타임 설정 변경
- **Agent Handler** (`handler/agent.go`): Agent CRUD + 생명주기 제어 + 런타임 설정 변경
- **Node Handler** (`handler/node.go`): 노드 타입 카탈로그 + 플로우 내 노드 조회/설정 변경
- **Monitoring Handler** (`handler/monitor.go`): Prometheus 메트릭, 시스템 상태, 로그 레벨 제어, SSE 이벤트 스트림
- **Plugin Handler** (`handler/plugin.go`): 플러그인 목록/상세/설치/제거
- **WebSocket Handler** (`handler/ws.go`): 실시간 플로우 상태/로그/메트릭 스트리밍
- **DTO & Validation** (`dto/`): 요청/응답 DTO, 표준 API 응답 엔벨로프, 페이지네이션
- **API Info & Stats** (`info.go`): 서버 런타임 정보, 요청 통계
- **Error Types** (`errors.go`): 표준 HTTP 에러 매핑, 에러 응답 형식

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/api/`
- **Tier**: internal (비공개 패키지)
- **HTTP 프레임워크**: Fiber v3 또는 Echo v4 (벤치마크 결과에 따라 최종 선택)
  - Fiber: fasthttp 기반 최고 HTTP 성능, Express.js 유사 API
  - Echo: net/http 호환, 풍부한 미들웨어 생태계, 검증된 안정성
  - 프레임워크 추상화 인터페이스를 통해 교체 가능하도록 설계
- **JWT**: `github.com/golang-jwt/jwt/v5` v5.2+
- **Metrics**: `github.com/prometheus/client_golang` v1.18+
- **Logging**: `log/slog` (Go 표준 라이브러리)
- **의존 패키지**:
  - `internal/engine/` (SPEC-ENGINE-001): 플로우 엔진 제어 (시작/중지/재시작/상태 조회)
  - `internal/node/` (SPEC-NODE-001): 노드 타입 레지스트리 조회
  - `internal/agent/` (SPEC-AGENT-001): Agent 관리 (CRUD, 생명주기 제어)
  - `internal/config/` (SPEC-CFG-001): 서버 설정 (포트, TLS, CORS, 레이트 리밋)
  - `internal/observe/` (SPEC-OBS-001): 관찰성 통합 (로깅, 메트릭, 트레이싱)
  - `internal/auth/` (SPEC-AUTH-001, 별도): 인증/인가 (JWT 검증, API 키, RBAC)
  - `internal/storage/` (SPEC-STORE-001): 데이터 영속화
  - `internal/plugin/` (미래 SPEC-PLUGIN-001): 플러그인 관리
  - `pkg/flow/` (SPEC-FLOW-001): 플로우 정의 구조체
  - `pkg/message/` (SPEC-MSG-001): 메시지 타입
  - `pkg/lifecycle/` (SPEC-LIFE-001): 생명주기 상태
  - `pkg/xferr/` (SPEC-ERR-001): 에러/상태 타입
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **게이트웨이 패턴**: API Layer는 모든 외부 접근의 단일 진입점으로, CLI와 Web Dashboard가 동일한 API를 공유
- **인터페이스 추상화**: 핸들러 의존성(Engine, Agent, Node 매니저)은 인터페이스로 주입하여 테스트 용이성 확보
- **HTTP 프레임워크 추상화**: Fiber/Echo 선택을 인터페이스 뒤에 숨겨 교체 가능하도록 설계
- **표준 응답 엔벨로프**: 모든 API 응답은 `{success, data, error, meta}` 형식을 따름
- **Graceful Shutdown**: 진행 중인 요청을 완료한 후 서버 종료, 데이터 유실 방지
- **관찰성 내장**: 모든 요청에 대해 구조화된 로깅, 메트릭 수집, 요청 추적
- **보안 우선**: 인증/인가 미들웨어를 통한 모든 보호 엔드포인트 접근 제어

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- API Server Core: HTTP 서버 초기화, 시작, 종료, health check
- Router: API 버전 프리픽스, 도메인별 라우트 그룹 구성, 정적 파일 서빙
- Middleware: 인증, CORS, 로깅, 레이트 리밋, 에러 핸들링, 요청 ID, 압축, 타임아웃
- Handlers: Flow, Agent, Node, Monitor, Plugin, WebSocket 핸들러
- DTO: 요청/응답 DTO, 표준 엔벨로프, 페이지네이션, 유효성 검증
- Error Types: HTTP 상태 코드 매핑, 표준 에러 응답 형식
- API Info/Stats: 서버 런타임 정보, 요청 통계

**OUT OF SCOPE (별도 SPEC)**:
- 인증/인가 로직 상세 구현 (별도 SPEC-AUTH-001: `internal/auth/`)
- Flow Engine 실행 로직 (SPEC-ENGINE-001: `internal/engine/`)
- 노드 타입 레지스트리 (SPEC-NODE-001: `internal/node/`)
- Agent 시스템 구현 (SPEC-AGENT-001: `internal/agent/`)
- 플러그인 시스템 (미래 SPEC-PLUGIN-001: `internal/plugin/`)
- 데이터 저장소 (SPEC-STORE-001: `internal/storage/`)
- 관찰성 시스템 구현 (SPEC-OBS-001: `internal/observe/`)
- 설정 시스템 구현 (SPEC-CFG-001: `internal/config/`)
- Web Dashboard 프론트엔드 (별도 프론트엔드 SPEC)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| API Server | xflowd 데몬 내 HTTP/WebSocket 서버 인스턴스 |
| Handler | 특정 엔드포인트의 요청/응답 처리 함수 |
| Middleware | 요청 파이프라인에서 핸들러 전후에 실행되는 처리 계층 |
| DTO | Data Transfer Object, API 요청/응답 데이터 구조 |
| Envelope | 표준 API 응답 래퍼 `{success, data, error, meta}` |
| Rate Limit | 클라이언트별 단위 시간당 최대 요청 수 제한 |
| Health Check | 서버 상태 확인 엔드포인트 (/health, /ready) |
| SSE | Server-Sent Events, 서버에서 클라이언트로 단방향 이벤트 스트리밍 |
| Graceful Shutdown | 진행 중인 요청 완료 후 서버 안전 종료 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-API-001: HTTP 프레임워크(Fiber v3 또는 Echo v4)는 프로젝트 초기 벤치마크 이후 최종 선택되며, 인터페이스 추상화로 교체 가능하다
- A-API-002: JWT 토큰 검증 및 RBAC 권한 확인은 `internal/auth/` 패키지에 위임하며, API Layer는 미들웨어 통합만 담당한다
- A-API-003: 의존하는 내부 패키지(engine, agent, node, storage)는 Go 인터페이스를 통해 접근하며, 모킹이 가능하다
- A-API-004: WebSocket 연결은 gorilla/websocket 라이브러리를 사용하며, 동일한 HTTP 서버 인스턴스에서 서빙된다
- A-API-005: 정적 파일 서빙(Web Dashboard)은 API 서버가 `web/dist/` 빌드 결과물을 내장 서빙한다

### 3.2 운영 가정

- A-API-006: 프로덕션 환경에서는 TLS가 활성화되며, 개발 환경에서는 HTTP로 동작한다
- A-API-007: 단일 인스턴스 기준 동시 연결 수는 1,000개 이하이다
- A-API-008: API 응답 시간(p95)은 일반 CRUD 엔드포인트 기준 50ms 미만이다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: API Server Core (P0) - server.go

#### REQ-API-001-01-01 (Ubiquitous)
시스템은 **항상** `Server` 구조체를 통해 HTTP 서버의 전체 생명주기(초기화, 시작, 종료)를 관리해야 한다.

#### REQ-API-001-01-02 (Event-Driven)
**WHEN** `Server.Start()` 호출 시, **THEN** 설정된 포트와 TLS 옵션에 따라 HTTP 리스너를 시작하고, 서버 상태를 `Running`으로 전이해야 한다.

#### REQ-API-001-01-03 (Event-Driven)
**WHEN** `Server.Stop()` 호출 시, **THEN** 설정된 타임아웃(기본 30초) 내에 진행 중인 요청을 완료한 후 서버를 graceful shutdown 해야 한다.

#### REQ-API-001-01-04 (Ubiquitous)
시스템은 **항상** `/health` 엔드포인트를 통해 서버 생존 상태(liveness)를 반환해야 한다.

#### REQ-API-001-01-05 (Ubiquitous)
시스템은 **항상** `/ready` 엔드포인트를 통해 서버 준비 상태(readiness)를 반환해야 하며, 모든 의존 서비스(DB, Engine)의 상태를 포함해야 한다.

#### REQ-API-001-01-06 (Ubiquitous)
시스템은 **항상** `Server` 구조체가 `pkg/lifecycle/` 패키지의 `Lifecycle` 인터페이스를 구현하여 통합 생명주기 관리가 가능해야 한다.

#### REQ-API-001-01-07 (Event-Driven)
**WHEN** OS 시그널(SIGTERM, SIGINT)을 수신하면, **THEN** graceful shutdown 프로세스를 시작해야 한다.

### 4.2 Module 2: Router & Versioning (P0) - router.go

#### REQ-API-001-02-01 (Ubiquitous)
시스템은 **항상** 모든 API 엔드포인트를 `/api/v1/` 버전 프리픽스 하위에 등록해야 한다.

#### REQ-API-001-02-02 (Ubiquitous)
시스템은 **항상** 도메인별 라우트 그룹(flows, agents, nodes, monitor, plugins, events)을 분리하여 등록해야 한다.

#### REQ-API-001-02-03 (Optional)
**가능하면** `/api/v1/docs` 엔드포인트를 통해 OpenAPI 문서를 자동 생성하여 서빙해야 한다.

#### REQ-API-001-02-04 (Ubiquitous)
시스템은 **항상** Web Dashboard 정적 파일(web/dist/)을 루트 경로(`/`)에서 서빙해야 한다.

#### REQ-API-001-02-05 (State-Driven)
**WHILE** API 서버가 `Running` 상태인 동안, 시스템은 등록된 모든 라우트에 대해 요청을 수신하고 처리해야 한다.

### 4.3 Module 3: Middleware Stack (P0) - middleware.go

#### REQ-API-001-03-01 (Ubiquitous)
시스템은 **항상** 모든 요청에 고유 요청 ID(UUID v4)를 생성하여 `X-Request-ID` 헤더에 포함해야 한다.

#### REQ-API-001-03-02 (Ubiquitous)
시스템은 **항상** 모든 요청/응답을 slog 기반 구조화된 로그로 기록해야 하며, 요청 메서드, 경로, 상태 코드, 응답 시간, 요청 ID를 포함해야 한다.

#### REQ-API-001-03-03 (Event-Driven)
**WHEN** 보호된 엔드포인트에 요청이 도달하면, **THEN** 인증 미들웨어가 JWT 토큰 또는 API 키를 검증하고, 유효하지 않으면 401 Unauthorized를 반환해야 한다.

#### REQ-API-001-03-04 (Event-Driven)
**WHEN** 인증된 사용자의 역할이 엔드포인트에 필요한 권한을 충족하지 못하면, **THEN** 403 Forbidden을 반환해야 한다.

#### REQ-API-001-03-05 (Ubiquitous)
시스템은 **항상** CORS 미들웨어를 적용하여 설정된 출처(origins), 메서드, 헤더에 대한 교차 출처 요청을 허용해야 한다.

#### REQ-API-001-03-06 (Event-Driven)
**WHEN** 클라이언트가 설정된 레이트 리밋(기본: 100 req/min per client)을 초과하면, **THEN** 429 Too Many Requests를 반환하고 `Retry-After` 헤더를 포함해야 한다.

#### REQ-API-001-03-07 (Event-Driven)
**WHEN** 핸들러에서 panic이 발생하면, **THEN** 에러 핸들링 미들웨어가 panic을 복구(recover)하고 500 Internal Server Error를 표준 에러 응답 형식으로 반환해야 한다.

#### REQ-API-001-03-08 (Ubiquitous)
시스템은 **항상** 응답 본문에 gzip 압축을 적용해야 하며, `Accept-Encoding: gzip` 헤더를 보낸 클라이언트에만 적용해야 한다.

#### REQ-API-001-03-09 (Event-Driven)
**WHEN** 요청 처리 시간이 설정된 타임아웃(기본: 30초)을 초과하면, **THEN** 408 Request Timeout을 반환하고 처리를 중단해야 한다.

#### REQ-API-001-03-10 (Unwanted)
시스템은 인증 토큰이나 API 키를 응답 본문 또는 로그에 평문으로 노출**하지 않아야** 한다.

### 4.4 Module 4: Flow Handler (P0) - handler/flow.go

#### REQ-API-001-04-01 (Event-Driven)
**WHEN** `GET /api/v1/flows` 요청을 수신하면, **THEN** 플로우 목록을 페이지네이션 및 필터링(상태, 이름) 파라미터에 따라 반환해야 한다.

#### REQ-API-001-04-02 (Event-Driven)
**WHEN** `GET /api/v1/flows/:id` 요청을 수신하면, **THEN** 해당 플로우의 상세 정보(정의, 상태, 노드 목록, 통계)를 반환해야 한다.

#### REQ-API-001-04-03 (Event-Driven)
**WHEN** `POST /api/v1/flows` 요청을 수신하면, **THEN** JSON/YAML 형식의 플로우 정의를 파싱하고 유효성을 검증한 후 새 플로우를 생성해야 한다.

#### REQ-API-001-04-04 (Event-Driven)
**WHEN** `PUT /api/v1/flows/:id` 요청을 수신하면, **THEN** 기존 플로우 정의를 업데이트하고 유효성을 재검증해야 한다.

#### REQ-API-001-04-05 (Event-Driven)
**WHEN** `DELETE /api/v1/flows/:id` 요청을 수신하면, **THEN** 해당 플로우가 실행 중이지 않은 경우 삭제하고, 실행 중이면 409 Conflict를 반환해야 한다.

#### REQ-API-001-04-06 (Event-Driven)
**WHEN** `POST /api/v1/flows/:id/deploy` 요청을 수신하면, **THEN** 플로우를 배포(엔진에 로드)하고 `Deployed` 상태로 전이해야 한다.

#### REQ-API-001-04-07 (Event-Driven)
**WHEN** `POST /api/v1/flows/:id/start` 요청을 수신하면, **THEN** 플로우 엔진을 통해 해당 플로우를 시작하고 `Running` 상태로 전이해야 한다.

#### REQ-API-001-04-08 (Event-Driven)
**WHEN** `POST /api/v1/flows/:id/stop` 요청을 수신하면, **THEN** 해당 플로우를 graceful stop 하고 `Stopped` 상태로 전이해야 한다.

#### REQ-API-001-04-09 (Event-Driven)
**WHEN** `POST /api/v1/flows/:id/restart` 요청을 수신하면, **THEN** 해당 플로우를 stop 후 start 하여 재시작해야 한다.

#### REQ-API-001-04-10 (Event-Driven)
**WHEN** `PUT /api/v1/flows/:id/config` 요청을 수신하면, **THEN** 플로우의 런타임 설정(백프레셔 임계값, 실행 정책 등)을 재시작 없이 변경해야 한다.

#### REQ-API-001-04-11 (Event-Driven)
**WHEN** `GET /api/v1/flows/:id/status` 요청을 수신하면, **THEN** 플로우의 현재 상태, 실행 시간, 노드별 처리 통계를 반환해야 한다.

#### REQ-API-001-04-12 (Unwanted)
시스템은 유효성 검증에 실패한 플로우 정의를 생성하거나 배포**하지 않아야** 한다.

### 4.5 Module 5: Agent Handler (P0) - handler/agent.go

#### REQ-API-001-05-01 (Event-Driven)
**WHEN** `GET /api/v1/agents` 요청을 수신하면, **THEN** Agent 목록을 페이지네이션 파라미터에 따라 반환해야 한다.

#### REQ-API-001-05-02 (Event-Driven)
**WHEN** `GET /api/v1/agents/:id` 요청을 수신하면, **THEN** 해당 Agent의 상세 정보(Info + Stats, SPEC-AGENT-001 참조)를 반환해야 한다.

#### REQ-API-001-05-03 (Event-Driven)
**WHEN** `POST /api/v1/agents` 요청을 수신하면, **THEN** Agent 설정을 검증하고 새 Agent를 등록해야 한다.

#### REQ-API-001-05-04 (Event-Driven)
**WHEN** `PUT /api/v1/agents/:id` 요청을 수신하면, **THEN** 기존 Agent 설정을 업데이트해야 한다.

#### REQ-API-001-05-05 (Event-Driven)
**WHEN** `DELETE /api/v1/agents/:id` 요청을 수신하면, **THEN** 해당 Agent가 다른 플로우에서 참조 중이지 않은 경우 삭제하고, 참조 중이면 409 Conflict를 반환해야 한다.

#### REQ-API-001-05-06 (Event-Driven)
**WHEN** `POST /api/v1/agents/:id/start` 요청을 수신하면, **THEN** Agent를 시작하고 `Running` 상태로 전이해야 한다.

#### REQ-API-001-05-07 (Event-Driven)
**WHEN** `POST /api/v1/agents/:id/stop` 요청을 수신하면, **THEN** Agent를 graceful stop 하고 `Stopped` 상태로 전이해야 한다.

#### REQ-API-001-05-08 (Event-Driven)
**WHEN** `POST /api/v1/agents/:id/restart` 요청을 수신하면, **THEN** Agent를 stop 후 start 하여 재시작해야 한다.

#### REQ-API-001-05-09 (Event-Driven)
**WHEN** `PUT /api/v1/agents/:id/config` 요청을 수신하면, **THEN** Agent의 런타임 설정(인증 정보, 폴링 주기, 프로토콜 파라미터 등)을 재시작 없이 변경해야 한다.

#### REQ-API-001-05-10 (Event-Driven)
**WHEN** `GET /api/v1/agents/:id/stats` 요청을 수신하면, **THEN** Agent의 실행 통계(수신/송신 메시지 수, 연결 상태, 업타임 등)를 반환해야 한다.

### 4.6 Module 6: Node Handler (P1) - handler/node.go

#### REQ-API-001-06-01 (Event-Driven)
**WHEN** `GET /api/v1/nodes` 요청을 수신하면, **THEN** 등록된 모든 노드 타입(내장 + 플러그인)의 목록을 반환해야 한다.

#### REQ-API-001-06-02 (Event-Driven)
**WHEN** `GET /api/v1/nodes/:type` 요청을 수신하면, **THEN** 해당 노드 타입의 상세 정보(입/출력 포트, 설정 스키마, 설명)를 반환해야 한다.

#### REQ-API-001-06-03 (Event-Driven)
**WHEN** `GET /api/v1/flows/:flowId/nodes` 요청을 수신하면, **THEN** 해당 플로우에 속한 노드 인스턴스 목록을 반환해야 한다.

#### REQ-API-001-06-04 (Event-Driven)
**WHEN** `GET /api/v1/flows/:flowId/nodes/:nodeId` 요청을 수신하면, **THEN** 해당 노드 인스턴스의 상세 정보와 현재 상태를 반환해야 한다.

#### REQ-API-001-06-05 (Event-Driven)
**WHEN** `PUT /api/v1/flows/:flowId/nodes/:nodeId/config` 요청을 수신하면, **THEN** 해당 노드의 런타임 설정(필터 조건, 변환 규칙, 집계 윈도우 등)을 재시작 없이 변경해야 한다.

### 4.7 Module 7: Monitoring Handler (P1) - handler/monitor.go

#### REQ-API-001-07-01 (Ubiquitous)
시스템은 **항상** `GET /api/v1/monitor/metrics` 엔드포인트를 통해 Prometheus 형식의 메트릭을 노출해야 한다.

#### REQ-API-001-07-02 (Event-Driven)
**WHEN** `GET /api/v1/monitor/health` 요청을 수신하면, **THEN** 시스템 전체 건강 상태(DB, Engine, Agent 상태 포함)를 요약하여 반환해야 한다.

#### REQ-API-001-07-03 (Event-Driven)
**WHEN** `GET /api/v1/monitor/status` 요청을 수신하면, **THEN** 시스템 상태 개요(실행 중 플로우 수, 활성 Agent 수, 서버 업타임 등)를 반환해야 한다.

#### REQ-API-001-07-04 (Event-Driven)
**WHEN** `PUT /api/v1/observe/level` 요청을 수신하면, **THEN** 지정된 컴포넌트의 로그 레벨을 런타임에 변경해야 한다 (예: `?component=agent.mqtt&level=debug`).

#### REQ-API-001-07-05 (Event-Driven)
**WHEN** `GET /api/v1/observe/level` 요청을 수신하면, **THEN** 현재 모든 컴포넌트의 로그 레벨 설정을 반환해야 한다.

#### REQ-API-001-07-06 (Event-Driven)
**WHEN** `GET /api/v1/events` 요청을 수신하면, **THEN** SSE(Server-Sent Events) 연결을 수립하고 시스템 이벤트(플로우 상태 변경, Agent 연결/해제, 에러 발생 등)를 실시간으로 스트리밍해야 한다.

### 4.8 Module 8: Plugin Handler (P1) - handler/plugin.go

#### REQ-API-001-08-01 (Event-Driven)
**WHEN** `GET /api/v1/plugins` 요청을 수신하면, **THEN** 설치된 플러그인 목록을 반환해야 한다.

#### REQ-API-001-08-02 (Event-Driven)
**WHEN** `GET /api/v1/plugins/:id` 요청을 수신하면, **THEN** 해당 플러그인의 상세 정보(제공 노드 타입, 버전, 상태)를 반환해야 한다.

#### REQ-API-001-08-03 (Event-Driven)
**WHEN** `POST /api/v1/plugins` 요청을 수신하면, **THEN** 플러그인 바이너리 또는 참조를 검증하고 설치해야 한다.

#### REQ-API-001-08-04 (Event-Driven)
**WHEN** `DELETE /api/v1/plugins/:id` 요청을 수신하면, **THEN** 해당 플러그인을 언로드하고 제거해야 하며, 플러그인이 제공하는 노드 타입이 사용 중이면 409 Conflict를 반환해야 한다.

### 4.9 Module 9: DTO & Validation (P0) - dto/

#### REQ-API-001-09-01 (Ubiquitous)
시스템은 **항상** 모든 API 응답을 표준 엔벨로프 형식 `{success: bool, data: T, error: ErrorDetail?, meta: Meta?}`로 래핑해야 한다.

#### REQ-API-001-09-02 (Ubiquitous)
시스템은 **항상** 요청 DTO에 대해 구조체 태그 기반 유효성 검증을 수행하고, 실패 시 400 Bad Request와 필드별 에러 상세를 반환해야 한다.

#### REQ-API-001-09-03 (Ubiquitous)
시스템은 **항상** 목록 조회 API에서 페이지네이션 응답을 포함해야 하며, `{page, size, total, total_pages}` 메타 정보를 반환해야 한다.

#### REQ-API-001-09-04 (Event-Driven)
**WHEN** 요청 본문이 잘못된 JSON/YAML 형식이면, **THEN** 400 Bad Request와 파싱 에러 상세를 반환해야 한다.

### 4.10 Module 10: WebSocket Handler (P1) - handler/ws.go

#### REQ-API-001-10-01 (Event-Driven)
**WHEN** 클라이언트가 WebSocket 업그레이드 요청(`/api/v1/ws`)을 보내면, **THEN** 인증을 검증한 후 WebSocket 연결을 수립해야 한다.

#### REQ-API-001-10-02 (State-Driven)
**WHILE** WebSocket 연결이 활성 상태인 동안, 시스템은 구독된 채널(플로우 상태, 로그, 메트릭)의 실시간 업데이트를 스트리밍해야 한다.

#### REQ-API-001-10-03 (Ubiquitous)
시스템은 **항상** WebSocket 연결에 대해 주기적 heartbeat(ping/pong)를 수행하고, 응답이 없는 연결은 정리해야 한다.

#### REQ-API-001-10-04 (Event-Driven)
**WHEN** WebSocket 연결이 끊어지면, **THEN** 관련 리소스(구독, 버퍼)를 즉시 정리하고 연결 풀에서 제거해야 한다.

#### REQ-API-001-10-05 (Optional)
**가능하면** WebSocket 클라이언트의 자동 재연결을 지원하여 이전 구독 상태를 복원해야 한다.

### 4.11 Module 11: API Info & Stats (P1) - info.go

#### REQ-API-001-11-01 (Event-Driven)
**WHEN** `ServerInfo()` 호출 시, **THEN** 서버 런타임 스냅샷(업타임, 활성 연결 수, 등록된 라우트 수, Go 버전)을 반환해야 한다.

#### REQ-API-001-11-02 (Event-Driven)
**WHEN** `ServerStats()` 호출 시, **THEN** 요청 통계(총 요청 수, 에러율, 평균 응답 시간, 엔드포인트별 통계)를 반환해야 한다.

### 4.12 Module 12: Error Types (P0) - errors.go

#### REQ-API-001-12-01 (Ubiquitous)
시스템은 **항상** 다음 표준 HTTP 에러 타입을 정의하고 사용해야 한다:
- `ErrBadRequest` (400): 잘못된 요청
- `ErrUnauthorized` (401): 인증 실패
- `ErrForbidden` (403): 권한 부족
- `ErrNotFound` (404): 리소스 미존재
- `ErrConflict` (409): 리소스 충돌
- `ErrValidationFailed` (422): 유효성 검증 실패
- `ErrRateLimitExceeded` (429): 레이트 리밋 초과
- `ErrInternalServer` (500): 내부 서버 오류
- `ErrServiceUnavailable` (503): 서비스 불가

#### REQ-API-001-12-02 (Ubiquitous)
시스템은 **항상** 에러 응답을 표준 형식 `{success: false, error: {code: string, message: string, details: any?}}` 으로 반환해야 한다.

#### REQ-API-001-12-03 (Event-Driven)
**WHEN** `pkg/xferr/` 패키지의 도메인 에러가 핸들러에서 반환되면, **THEN** 에러 매핑 레이어가 적절한 HTTP 상태 코드와 에러 응답으로 변환해야 한다.

#### REQ-API-001-12-04 (Unwanted)
시스템은 500 Internal Server Error 응답에서 내부 스택 트레이스나 시스템 경로 정보를 클라이언트에 노출**하지 않아야** 한다.

---

## 5. Specifications (설계)

### 5.1 파일 구조

```
internal/api/
├── server.go           # Server 구조체, Start/Stop/ListenAddr, health check
├── router.go           # SetupRoutes, API 버전 프리픽스, 라우트 그룹
├── middleware.go        # Auth, CORS, Logger, RateLimit, Recovery, RequestID, Compress, Timeout
├── info.go             # ServerInfo, ServerStats 구조체 및 메서드
├── errors.go           # APIError 타입, HTTP 상태 코드 매핑, 에러 응답 헬퍼
├── handler/
│   ├── flow.go         # FlowHandler (CRUD + deploy/start/stop/restart + config)
│   ├── agent.go        # AgentHandler (CRUD + start/stop/restart + config + stats)
│   ├── node.go         # NodeHandler (타입 카탈로그 + 플로우 내 노드 조회/설정)
│   ├── monitor.go      # MonitorHandler (metrics, health, status, log level, SSE events)
│   ├── plugin.go       # PluginHandler (list/detail/install/remove)
│   ├── ws.go           # WebSocketHandler (연결 관리, 실시간 스트리밍)
│   └── auth.go         # AuthHandler (login, logout, refresh, apikey - internal/auth 위임)
├── dto/
│   ├── request.go      # 요청 DTO (FlowCreateReq, AgentCreateReq, PaginationReq 등)
│   ├── response.go     # 응답 DTO (APIResponse, FlowResp, AgentResp, PaginationMeta 등)
│   └── validation.go   # 커스텀 유효성 검증 헬퍼
└── api_test.go         # API 통합 테스트
```

### 5.2 인터페이스 정의

```go
// === server.go ===

// Server는 API 서버의 전체 생명주기를 관리한다.
type Server struct {
    config     *config.ServerConfig
    router     Router                    // HTTP 프레임워크 추상화
    logger     *slog.Logger
    lifecycle  lifecycle.BaseLifecycle   // SPEC-LIFE-001
    info       *ServerInfo
    stats      *ServerStats
}

// Start는 HTTP 리스너를 시작한다.
func (s *Server) Start(ctx context.Context) error

// Stop은 graceful shutdown을 수행한다.
func (s *Server) Stop(ctx context.Context) error

// ListenAddr는 현재 리스닝 주소를 반환한다.
func (s *Server) ListenAddr() string

// Router는 HTTP 프레임워크를 추상화하는 인터페이스이다.
type Router interface {
    GET(path string, handler HandlerFunc, middleware ...MiddlewareFunc)
    POST(path string, handler HandlerFunc, middleware ...MiddlewareFunc)
    PUT(path string, handler HandlerFunc, middleware ...MiddlewareFunc)
    DELETE(path string, handler HandlerFunc, middleware ...MiddlewareFunc)
    Group(prefix string, middleware ...MiddlewareFunc) Router
    Static(prefix, root string)
    Use(middleware ...MiddlewareFunc)
    ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// HandlerFunc는 핸들러 함수 시그니처이다.
type HandlerFunc func(ctx Context) error

// Context는 요청 컨텍스트를 추상화한다.
type Context interface {
    Param(name string) string
    Query(name string) string
    Bind(v any) error
    JSON(code int, v any) error
    NoContent(code int) error
    RequestID() string
    UserID() string
    UserRole() string
    Context() context.Context
}
```

```go
// === handler/flow.go ===

// FlowManager는 핸들러가 Engine에 접근하기 위한 인터페이스이다.
type FlowManager interface {
    ListFlows(ctx context.Context, opts ListOptions) ([]flow.Flow, int64, error)
    GetFlow(ctx context.Context, id string) (*flow.Flow, error)
    CreateFlow(ctx context.Context, def *flow.Flow) (*flow.Flow, error)
    UpdateFlow(ctx context.Context, id string, def *flow.Flow) (*flow.Flow, error)
    DeleteFlow(ctx context.Context, id string) error
    DeployFlow(ctx context.Context, id string) error
    StartFlow(ctx context.Context, id string) error
    StopFlow(ctx context.Context, id string) error
    RestartFlow(ctx context.Context, id string) error
    ConfigureFlow(ctx context.Context, id string, cfg map[string]any) error
    FlowStatus(ctx context.Context, id string) (*FlowStatusResp, error)
}

// FlowHandler는 플로우 관련 API 핸들러이다.
type FlowHandler struct {
    flows  FlowManager
    logger *slog.Logger
}
```

```go
// === handler/agent.go ===

// AgentManager는 핸들러가 Agent 시스템에 접근하기 위한 인터페이스이다.
type AgentManager interface {
    ListAgents(ctx context.Context, opts ListOptions) ([]agent.Info, int64, error)
    GetAgent(ctx context.Context, id string) (*agent.Info, error)
    CreateAgent(ctx context.Context, cfg *agent.Config) (*agent.Info, error)
    UpdateAgent(ctx context.Context, id string, cfg *agent.Config) (*agent.Info, error)
    DeleteAgent(ctx context.Context, id string) error
    StartAgent(ctx context.Context, id string) error
    StopAgent(ctx context.Context, id string) error
    RestartAgent(ctx context.Context, id string) error
    ConfigureAgent(ctx context.Context, id string, cfg map[string]any) error
    AgentStats(ctx context.Context, id string) (*agent.Stats, error)
}

// AgentHandler는 Agent 관련 API 핸들러이다.
type AgentHandler struct {
    agents AgentManager
    logger *slog.Logger
}
```

```go
// === handler/node.go ===

// NodeRegistry는 노드 타입 카탈로그 접근 인터페이스이다.
type NodeRegistry interface {
    ListNodeTypes(ctx context.Context) ([]node.TypeInfo, error)
    GetNodeType(ctx context.Context, typeName string) (*node.TypeInfo, error)
}

// NodeManager는 플로우 내 노드 인스턴스 관리 인터페이스이다.
type NodeManager interface {
    ListFlowNodes(ctx context.Context, flowID string) ([]node.InstanceInfo, error)
    GetFlowNode(ctx context.Context, flowID, nodeID string) (*node.InstanceInfo, error)
    ConfigureNode(ctx context.Context, flowID, nodeID string, cfg map[string]any) error
}
```

```go
// === dto/response.go ===

// APIResponse는 표준 API 응답 엔벨로프이다.
type APIResponse[T any] struct {
    Success bool         `json:"success"`
    Data    T            `json:"data,omitempty"`
    Error   *ErrorDetail `json:"error,omitempty"`
    Meta    *Meta        `json:"meta,omitempty"`
}

// ErrorDetail은 에러 상세 정보이다.
type ErrorDetail struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

// Meta는 응답 메타 정보이다.
type Meta struct {
    RequestID  string          `json:"request_id,omitempty"`
    Pagination *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta는 페이지네이션 메타 정보이다.
type PaginationMeta struct {
    Page       int   `json:"page"`
    Size       int   `json:"size"`
    Total      int64 `json:"total"`
    TotalPages int   `json:"total_pages"`
}
```

```go
// === errors.go ===

// APIError는 HTTP 에러를 표현하는 도메인 에러이다.
type APIError struct {
    HTTPCode int
    Code     string
    Message  string
    Details  any
}

func (e *APIError) Error() string

// 사전 정의된 에러 인스턴스
var (
    ErrBadRequest         = &APIError{HTTPCode: 400, Code: "BAD_REQUEST"}
    ErrUnauthorized       = &APIError{HTTPCode: 401, Code: "UNAUTHORIZED"}
    ErrForbidden          = &APIError{HTTPCode: 403, Code: "FORBIDDEN"}
    ErrNotFound           = &APIError{HTTPCode: 404, Code: "NOT_FOUND"}
    ErrConflict           = &APIError{HTTPCode: 409, Code: "CONFLICT"}
    ErrValidationFailed   = &APIError{HTTPCode: 422, Code: "VALIDATION_FAILED"}
    ErrRateLimitExceeded  = &APIError{HTTPCode: 429, Code: "RATE_LIMIT_EXCEEDED"}
    ErrInternalServer     = &APIError{HTTPCode: 500, Code: "INTERNAL_ERROR"}
    ErrServiceUnavailable = &APIError{HTTPCode: 503, Code: "SERVICE_UNAVAILABLE"}
)

// MapDomainError는 pkg/xferr 도메인 에러를 APIError로 변환한다.
func MapDomainError(err error) *APIError
```

```go
// === info.go ===

// ServerInfo는 서버 런타임 스냅샷이다.
type ServerInfo struct {
    Version          string    `json:"version"`
    Uptime           string    `json:"uptime"`
    StartedAt        time.Time `json:"started_at"`
    ActiveConnections int      `json:"active_connections"`
    RegisteredRoutes int       `json:"registered_routes"`
    GoVersion        string    `json:"go_version"`
}

// ServerStats는 요청 통계이다.
type ServerStats struct {
    TotalRequests    int64          `json:"total_requests"`
    ErrorCount       int64          `json:"error_count"`
    ErrorRate        float64        `json:"error_rate"`
    AvgLatencyMs     float64        `json:"avg_latency_ms"`
    P95LatencyMs     float64        `json:"p95_latency_ms"`
    EndpointStats    map[string]*EndpointStat `json:"endpoint_stats"`
}
```

### 5.3 API 엔드포인트 전체 목록

| 메서드 | 경로 | 핸들러 | 인증 | 권한 | 우선순위 |
|--------|------|--------|------|------|----------|
| GET | /health | Server.healthCheck | X | - | P0 |
| GET | /ready | Server.readyCheck | X | - | P0 |
| GET | /api/v1/flows | FlowHandler.List | O | Viewer+ | P0 |
| GET | /api/v1/flows/:id | FlowHandler.Get | O | Viewer+ | P0 |
| POST | /api/v1/flows | FlowHandler.Create | O | Editor+ | P0 |
| PUT | /api/v1/flows/:id | FlowHandler.Update | O | Editor+ | P0 |
| DELETE | /api/v1/flows/:id | FlowHandler.Delete | O | Admin | P0 |
| POST | /api/v1/flows/:id/deploy | FlowHandler.Deploy | O | Editor+ | P0 |
| POST | /api/v1/flows/:id/start | FlowHandler.Start | O | Editor+ | P0 |
| POST | /api/v1/flows/:id/stop | FlowHandler.Stop | O | Editor+ | P0 |
| POST | /api/v1/flows/:id/restart | FlowHandler.Restart | O | Editor+ | P0 |
| PUT | /api/v1/flows/:id/config | FlowHandler.Configure | O | Editor+ | P0 |
| GET | /api/v1/flows/:id/status | FlowHandler.Status | O | Viewer+ | P0 |
| GET | /api/v1/agents | AgentHandler.List | O | Viewer+ | P0 |
| GET | /api/v1/agents/:id | AgentHandler.Get | O | Viewer+ | P0 |
| POST | /api/v1/agents | AgentHandler.Create | O | Editor+ | P0 |
| PUT | /api/v1/agents/:id | AgentHandler.Update | O | Editor+ | P0 |
| DELETE | /api/v1/agents/:id | AgentHandler.Delete | O | Admin | P0 |
| POST | /api/v1/agents/:id/start | AgentHandler.Start | O | Editor+ | P0 |
| POST | /api/v1/agents/:id/stop | AgentHandler.Stop | O | Editor+ | P0 |
| POST | /api/v1/agents/:id/restart | AgentHandler.Restart | O | Editor+ | P0 |
| PUT | /api/v1/agents/:id/config | AgentHandler.Configure | O | Editor+ | P0 |
| GET | /api/v1/agents/:id/stats | AgentHandler.Stats | O | Viewer+ | P0 |
| GET | /api/v1/nodes | NodeHandler.ListTypes | O | Viewer+ | P1 |
| GET | /api/v1/nodes/:type | NodeHandler.GetType | O | Viewer+ | P1 |
| GET | /api/v1/flows/:flowId/nodes | NodeHandler.ListFlowNodes | O | Viewer+ | P1 |
| GET | /api/v1/flows/:flowId/nodes/:nodeId | NodeHandler.GetFlowNode | O | Viewer+ | P1 |
| PUT | /api/v1/flows/:flowId/nodes/:nodeId/config | NodeHandler.Configure | O | Editor+ | P1 |
| GET | /api/v1/monitor/metrics | MonitorHandler.Metrics | X | - | P1 |
| GET | /api/v1/monitor/health | MonitorHandler.Health | X | - | P1 |
| GET | /api/v1/monitor/status | MonitorHandler.Status | O | Viewer+ | P1 |
| PUT | /api/v1/observe/level | MonitorHandler.SetLogLevel | O | Admin | P1 |
| GET | /api/v1/observe/level | MonitorHandler.GetLogLevels | O | Viewer+ | P1 |
| GET | /api/v1/events | MonitorHandler.SSEEvents | O | Viewer+ | P1 |
| GET | /api/v1/plugins | PluginHandler.List | O | Viewer+ | P1 |
| GET | /api/v1/plugins/:id | PluginHandler.Get | O | Viewer+ | P1 |
| POST | /api/v1/plugins | PluginHandler.Install | O | Admin | P1 |
| DELETE | /api/v1/plugins/:id | PluginHandler.Remove | O | Admin | P1 |
| GET | /api/v1/ws | WebSocketHandler.Upgrade | O | Viewer+ | P1 |

### 5.4 미들웨어 실행 순서

```
Request
  -> Recovery (panic 복구)
  -> RequestID (UUID 생성)
  -> Logger (요청 로깅 시작)
  -> Compress (gzip)
  -> CORS (교차 출처)
  -> RateLimit (요청 제한)
  -> Timeout (요청 타임아웃)
  -> Auth (JWT/API Key 검증, 보호 엔드포인트만)
  -> RBAC (역할 기반 권한 검증)
  -> Handler (실제 처리)
  -> Logger (응답 로깅)
Response
```

### 5.5 구현 우선순위

| 우선순위 | 모듈 | 파일 | 설명 |
|----------|------|------|------|
| P0 (핵심) | Server Core | server.go | HTTP 서버 생명주기, health check |
| P0 (핵심) | Router | router.go | 라우팅, 버전 프리픽스, 라우트 그룹 |
| P0 (핵심) | Middleware | middleware.go | 인증, CORS, 로깅, 레이트 리밋, 에러 핸들링 |
| P0 (핵심) | Error Types | errors.go | 표준 에러 타입, HTTP 매핑 |
| P0 (핵심) | DTO | dto/ | 요청/응답 DTO, 유효성 검증, 엔벨로프 |
| P0 (핵심) | Flow Handler | handler/flow.go | 플로우 CRUD + 실행 제어 |
| P0 (핵심) | Agent Handler | handler/agent.go | Agent CRUD + 생명주기 제어 |
| P1 (확장) | Node Handler | handler/node.go | 노드 카탈로그 + 플로우 내 노드 관리 |
| P1 (확장) | Monitor Handler | handler/monitor.go | 메트릭, 상태, 로그 레벨, SSE |
| P1 (확장) | Plugin Handler | handler/plugin.go | 플러그인 관리 |
| P1 (확장) | WebSocket | handler/ws.go | 실시간 스트리밍 |
| P1 (확장) | API Info/Stats | info.go | 서버 정보, 요청 통계 |

### 5.6 의존성 다이어그램

```
internal/api/
  ├── server.go ──────────> internal/config/ (SPEC-CFG-001)
  │                 ├────-> internal/observe/ (SPEC-OBS-001)
  │                 └────-> pkg/lifecycle/   (SPEC-LIFE-001)
  │
  ├── handler/flow.go ───-> internal/engine/ (SPEC-ENGINE-001)
  │                 └────-> pkg/flow/        (SPEC-FLOW-001)
  │
  ├── handler/agent.go ──-> internal/agent/  (SPEC-AGENT-001)
  │
  ├── handler/node.go ───-> internal/node/   (SPEC-NODE-001)
  │
  ├── handler/monitor.go ─> internal/observe/ (SPEC-OBS-001)
  │
  ├── handler/plugin.go ──> internal/plugin/ (SPEC-PLUGIN-001)
  │
  ├── handler/auth.go ───-> internal/auth/   (SPEC-AUTH-001)
  │
  ├── middleware.go ──────> internal/auth/   (SPEC-AUTH-001)
  │                 └────-> internal/observe/ (SPEC-OBS-001)
  │
  ├── errors.go ─────────-> pkg/xferr/       (SPEC-ERR-001)
  │
  └── dto/ ──────────────-> pkg/flow/        (SPEC-FLOW-001)
                    └────-> pkg/message/     (SPEC-MSG-001)
```

---

*SPEC ID: SPEC-API-001*
*버전: 1.0.0*
*상태: implemented*
*최종 수정: 2026-02-13*
