# SPEC-AGENT-STORE-001: Agent 영속 저장소 구현

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-AGENT-STORE-001 |
| 제목 | Agent Persistent Storage Implementation |
| 생성일 | 2026-02-25 |
| 상태 | Completed |
| 우선순위 | High |
| 담당 | expert-backend |

## 컨텍스트 및 문제 정의

### 현재 상태

xflow 프로젝트의 플로우 처리 엔진은 Agent와 Flow 두 가지 핵심 엔티티를 관리한다. Flow는 SPEC-STORE-001을 통해 `FlowRepository` 인터페이스(File/SQLite/Postgres 백엔드)로 이미 영속화되어 있다. 그러나 Agent는 여전히 메모리 내 `map[string]Agent`에만 저장되어 서버 재시작 시 모든 에이전트 설정이 유실된다.

### 문제 요약

- **데이터 유실**: 서버 재시작 시 등록된 모든 에이전트 설정이 사라진다.
- **운영 부담**: 재시작 후 에이전트를 수동으로 다시 생성해야 한다.
- **일관성 부재**: Flow는 영속 저장되지만 Agent는 그렇지 않아 시스템 상태가 불완전하다.

### 현재 아키텍처 분석

```
cmd/xflowd/main.go (서버 와이어링)
  ├── agentMgr := agent.NewManager()         // 저장소 없음
  ├── agentSvc := service.NewAgentServiceAdapter(agentMgr, logger)  // 저장소 없음
  └── flowSvc := service.NewFlowServiceAdapter(eng, repo, logger)   // repo 있음
```

**핵심 데이터 모델** (`internal/agent/config.go`):

- `AgentConfig`: ID, Name, Type, Transport(Type/Options), ProtocolFile, HealthCheckInterval, MaxRestarts, StopOnZeroRef, BufferSize, LogLevel, Metadata
- `TransportConfig`: Type, Options (map[string]any)

**Agent Manager** (`internal/agent/manager.go`):

- `DefaultManager`: `agents map[string]Agent` (인메모리 전용)
- 인터페이스: Create, Start, Stop, Restart, Delete, Get, List, Shutdown, Summary

**기존 저장소 인프라** (`internal/storage/`):

- `FlowRepository` 인터페이스: Save, Get, List, Delete, Close
- 3개 백엔드: FileRepository, SQLiteRepository, PostgresRepository
- Factory: `NewRepository(ctx, cfg)` (config.Type으로 분기)
- `StorageConfig`: Type, FileDirectory, SQLitePath, PostgresDSN, PoolSize

---

## 요구사항 (EARS 형식)

### REQ-1: 에이전트 생성 시 영속화

**WHEN** API를 통해 에이전트가 생성될 때
**THEN** 시스템은 에이전트 설정을 구성된 저장소 백엔드에 영속화해야 한다.

- 트리거: `AgentServiceAdapter.CreateAgent()` 호출
- 저장 대상: `AgentConfig` 전체 (Transport 포함)
- 실패 시: 에이전트 생성도 롤백 (저장 실패 = 생성 실패)

### REQ-2: 서버 시작 시 에이전트 복원

**WHEN** 서버가 시작될 때
**THEN** 시스템은 저장소에서 모든 에이전트 설정을 로드하고 AgentManager에 재생성해야 한다.

- 타입 레지스트리 등록이 로드보다 먼저 수행되어야 한다
- 로드 실패한 개별 에이전트는 로깅 후 건너뛴다 (전체 시작을 차단하지 않음)
- 복원된 에이전트는 생성만 되고, 자동 시작은 하지 않는다

### REQ-3: 에이전트 삭제 시 저장소 제거

**WHEN** API를 통해 에이전트가 삭제될 때
**THEN** 시스템은 영속 저장소에서 해당 에이전트 설정을 제거해야 한다.

- Manager.Delete() 와 Repository.Delete() 모두 수행
- 저장소 삭제 실패 시 에러 반환 (불일치 방지)

### REQ-4: 에이전트 업데이트 시 저장소 갱신

**WHEN** API를 통해 에이전트 설정이 업데이트될 때
**THEN** 시스템은 영속 저장소의 설정을 갱신해야 한다.

- `AgentServiceAdapter.UpdateAgent()` 에서 Configure 호출 후 Save 수행
- 메모리와 저장소의 설정이 항상 일치해야 한다

### REQ-5: 동일 저장소 백엔드 지원

시스템은 **항상** FlowRepository와 동일한 저장소 백엔드(file, sqlite, postgres)를 지원해야 한다.

- 동일한 `StorageConfig` 사용
- 동일한 Factory 패턴 적용

### REQ-6: 저장소 격리

시스템은 **항상** 에이전트 데이터를 적절히 격리하여 저장해야 한다.

- File 백엔드: `{directory}/agents/` 하위 디렉토리에 `{agentID}.yaml` 파일로 저장
- SQLite 백엔드: 동일 DB 내 `agents` 테이블
- PostgreSQL 백엔드: 동일 DB 내 `agents` 테이블

### REQ-7: 타입 레지스트리 선행 등록

**WHEN** 서버 시작 시 영속된 에이전트를 로드할 때
**THEN** 시스템은 에이전트 타입 등록을 로드보다 먼저 수행해야 한다.

- 이미 `cmd/xflowd/main.go`에서 타입 등록이 Manager 생성 직후 수행됨
- 저장소 로드는 타입 등록 완료 후에 수행

---

## 기술 설계

### AgentRepository 인터페이스

`internal/storage/agent_repository.go`에 FlowRepository와 대칭 구조로 정의:

```go
type AgentRepository interface {
    Save(ctx context.Context, config agent.AgentConfig) error
    Get(ctx context.Context, id string) (agent.AgentConfig, error)
    List(ctx context.Context) ([]agent.AgentConfig, error)
    Delete(ctx context.Context, id string) error
    Close() error
}
```

### 직렬화 전략

| 백엔드 | 저장 형식 | 비고 |
|---------|----------|------|
| File | YAML | 사람이 읽기 쉬움, 기존 agent YAML 형식과 호환 |
| SQLite | JSON (BLOB) | FlowRepository와 동일한 패턴 |
| PostgreSQL | JSONB | FlowRepository와 동일한 패턴, 필드 쿼리 가능 |

### AgentConfig 직렬화

`AgentConfig`의 JSON/YAML 직렬화를 위해 `pkg/agent/serialize.go` 또는 `internal/agent/serialize.go`에 직렬화 함수를 추가한다:

- `AgentConfigToJSON(config AgentConfig) ([]byte, error)`
- `AgentConfigFromJSON(data []byte) (AgentConfig, error)`
- `AgentConfigToYAML(config AgentConfig) ([]byte, error)`
- `AgentConfigFromYAML(data []byte) (AgentConfig, error)`

`time.Duration` 필드는 문자열로 직렬화한다 (예: `"30s"`, `"5m"`).

### 저장소 백엔드 구현

#### File 백엔드 (`internal/storage/agent_file.go`)

- 저장 경로: `{FileDirectory}/agents/{agentID}.yaml`
- 원자적 쓰기: 임시 파일 → `os.Rename`
- FlowRepository의 FileRepository 패턴 그대로 적용

#### SQLite 백엔드 (`internal/storage/agent_sqlite.go`)

- 기존 SQLiteRepository의 `*sql.DB` 공유 또는 동일 경로 재사용
- `agents` 테이블 자동 생성 (CREATE TABLE IF NOT EXISTS)
- 스키마: `id TEXT PK, name TEXT, type TEXT, data BLOB, created_at, updated_at`

#### PostgreSQL 백엔드 (`internal/storage/agent_postgres.go`)

- 기존 PostgresRepository의 `*pgxpool.Pool` 공유 또는 동일 DSN 재사용
- `agents` 테이블 자동 생성
- 스키마: `id TEXT PK, name TEXT, type TEXT, data JSONB, created_at, updated_at`

### Factory 확장

`internal/storage/factory.go`에 Agent용 Factory 함수 추가:

```go
func NewAgentRepository(ctx context.Context, cfg config.StorageConfig) (AgentRepository, error)
```

### 서비스 어댑터 통합

`AgentServiceAdapter`에 `repo AgentRepository` 필드 추가:

- `CreateAgent()`: manager.Create 후 repo.Save
- `UpdateAgent()`: agent.Configure 후 repo.Save
- `DeleteAgent()`: manager.Delete 후 repo.Delete (또는 역순)

### 서버 시작 시 로드 로직

`cmd/xflowd/main.go`에서 기존 흐름에 추가:

```
1. agentMgr := agent.NewManager()
2. 타입 등록 (HTTP, MQTT, InfluxDB, Samsung NASA 등)
3. agentRepo := storage.NewAgentRepository(ctx, storageCfg)   // NEW
4. 저장소에서 에이전트 로드 및 Manager에 재생성              // NEW
5. agentSvc := service.NewAgentServiceAdapter(agentMgr, agentRepo, logger)  // CHANGED
```

---

## 파일 변경 매트릭스

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/storage/agent_repository.go` | 신규 | AgentRepository 인터페이스 + 에러 변수 |
| `internal/storage/agent_file.go` | 신규 | File 백엔드 구현 |
| `internal/storage/agent_sqlite.go` | 신규 | SQLite 백엔드 구현 |
| `internal/storage/agent_postgres.go` | 신규 | PostgreSQL 백엔드 구현 |
| `internal/storage/agent_file_test.go` | 신규 | File 백엔드 테스트 |
| `internal/storage/agent_sqlite_test.go` | 신규 | SQLite 백엔드 테스트 |
| `internal/storage/factory.go` | 수정 | `NewAgentRepository` 팩토리 함수 추가 |
| `internal/storage/factory_test.go` | 수정 | Agent Factory 테스트 추가 |
| `internal/agent/serialize.go` | 신규 | AgentConfig JSON/YAML 직렬화 |
| `internal/agent/serialize_test.go` | 신규 | 직렬화 테스트 |
| `internal/api/service/agent_adapter.go` | 수정 | repo 필드 추가, CRUD에 저장소 연동 |
| `internal/api/service/agent_adapter_test.go` | 수정 | 저장소 연동 테스트 추가 |
| `cmd/xflowd/main.go` | 수정 | AgentRepository 초기화, 시작 시 로드 로직 |

---

## 의존성

### 내부 의존성

- `internal/agent`: AgentConfig, Manager 인터페이스
- `internal/storage`: 기존 FlowRepository 패턴, Factory
- `internal/config`: StorageConfig
- `cmd/xflowd`: 서버 와이어링

### 외부 의존성

- `modernc.org/sqlite`: Pure Go SQLite 드라이버 (기존 사용 중)
- `github.com/jackc/pgx/v5`: PostgreSQL 드라이버 (기존 사용 중)
- `gopkg.in/yaml.v3`: YAML 직렬화 (기존 사용 중)

### 선행 조건

- SPEC-STORE-001 (FlowRepository) 이미 완료됨 -- 패턴 재사용
- 에이전트 타입 레지스트리 패턴 이미 구현됨

---

## 제약 조건

- 시스템은 에이전트의 **설정(AgentConfig)**만 영속화하며, **런타임 상태**(통계, 헬스체크 결과 등)는 영속하지 않는다.
- 시스템은 복원된 에이전트를 자동으로 **시작하지 않는다** (생성만 수행).
- 기존 `agent import/export` CLI 기능은 이 SPEC의 범위에 포함되지 않는다 (CLI는 현재 API를 통해 동작하므로 자동으로 혜택을 받음).

---

## 추적성

- 관련 SPEC: SPEC-STORE-001 (FlowRepository 영속 저장소)
- 관련 코드: `internal/storage/`, `internal/agent/`, `internal/api/service/agent_adapter.go`
