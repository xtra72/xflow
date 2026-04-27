# SPEC-AGENT-STORE-001: 구현 계획

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-AGENT-STORE-001 |
| 개발 방법론 | Hybrid (TDD for new + DDD for legacy) |
| 영향 파일 수 | 13개 (신규 7, 수정 6) |
| 관련 요구사항 | REQ-1 ~ REQ-7 |

---

## 마일스톤

### Primary Goal: AgentRepository 인터페이스 및 직렬화 기반

**목표**: AgentConfig 직렬화와 저장소 인터페이스를 확립한다.

**단계**:

1. **AgentConfig 직렬화 함수 구현** (`internal/agent/serialize.go`)
   - `AgentConfigToJSON` / `AgentConfigFromJSON`
   - `AgentConfigToYAML` / `AgentConfigFromYAML`
   - `time.Duration`을 문자열로 변환하는 보조 구조체 (marshalable config)
   - `map[string]any` (Transport.Options) 직렬화 처리

2. **직렬화 테스트** (`internal/agent/serialize_test.go`)
   - 왕복(roundtrip) 테스트: Config -> JSON -> Config 동일성 검증
   - 왕복 테스트: Config -> YAML -> Config 동일성 검증
   - 빈 필드, 제로값, 특수 문자 처리 테스트
   - `time.Duration` 문자열 변환 테스트

3. **AgentRepository 인터페이스 정의** (`internal/storage/agent_repository.go`)
   - `AgentRepository` 인터페이스 (Save, Get, List, Delete, Close)
   - `ErrAgentNotFound` 에러 변수

**관련 요구사항**: REQ-5 (동일 백엔드 지원), REQ-6 (저장소 격리)

---

### Secondary Goal: 3개 백엔드 구현

**목표**: File, SQLite, PostgreSQL 백엔드를 FlowRepository 패턴 그대로 구현한다.

**단계**:

4. **File 백엔드 구현** (`internal/storage/agent_file.go`)
   - `AgentFileRepository` 구조체
   - `{directory}/agents/` 하위 디렉토리 자동 생성
   - YAML 형식 저장: `{agentID}.yaml`
   - 원자적 쓰기 (임시 파일 + os.Rename)
   - `.yaml.tmp` 파일 제외 로직
   - `FlowRepository`의 `FileRepository` 패턴 미러링

5. **File 백엔드 테스트** (`internal/storage/agent_file_test.go`)
   - CRUD 전체 테스트
   - 존재하지 않는 에이전트 조회/삭제 시 ErrAgentNotFound
   - 빈 저장소에서 List 시 빈 슬라이스 반환
   - 동일 ID로 Save 시 덮어쓰기

6. **SQLite 백엔드 구현** (`internal/storage/agent_sqlite.go`)
   - `AgentSQLiteRepository` 구조체
   - 기존 SQLiteRepository와 동일한 DB 경로 사용 (동일 `*sql.DB` 재사용 가능)
   - `agents` 테이블 자동 생성 (CREATE TABLE IF NOT EXISTS)
   - 스키마: `id TEXT PK, name TEXT, type TEXT, data BLOB, created_at, updated_at`
   - UPSERT 패턴으로 Save 구현

7. **SQLite 백엔드 테스트** (`internal/storage/agent_sqlite_test.go`)
   - CRUD 전체 테스트
   - 테이블 자동 생성 검증
   - 동시 읽기/쓰기 안전성 (WAL 모드)

8. **PostgreSQL 백엔드 구현** (`internal/storage/agent_postgres.go`)
   - `AgentPostgresRepository` 구조체
   - 기존 PostgresRepository와 동일한 커넥션 풀 재사용 가능
   - `agents` 테이블 자동 생성
   - 스키마: `id TEXT PK, name TEXT, type TEXT, data JSONB, created_at, updated_at`
   - UPSERT 패턴으로 Save 구현

9. **Factory 확장** (`internal/storage/factory.go`)
   - `NewAgentRepository(ctx, cfg)` 함수 추가
   - File/SQLite/Postgres 분기 (기존 `NewRepository`와 동일 패턴)

10. **Factory 테스트 확장** (`internal/storage/factory_test.go`)
    - `NewAgentRepository` 호출 검증
    - 유효하지 않은 저장소 타입 에러 검증

**관련 요구사항**: REQ-5, REQ-6

---

### Final Goal: 서비스 어댑터 통합 및 서버 와이어링

**목표**: AgentServiceAdapter에 저장소를 연결하고 서버 시작 시 에이전트를 복원한다.

**단계**:

11. **AgentServiceAdapter 수정** (`internal/api/service/agent_adapter.go`)
    - `repo AgentRepository` 필드 추가
    - `NewAgentServiceAdapter(mgr, repo, logger)` 시그니처 변경
    - `CreateAgent()`: `manager.Create()` 성공 후 `repo.Save()` 호출
      - `repo.Save()` 실패 시 `manager.Delete()`로 롤백
    - `UpdateAgent()`: `agent.Configure()` 성공 후 `repo.Save()` 호출
    - `DeleteAgent()`: `repo.Delete()` 호출 후 `manager.Delete()`
      - 저장소 삭제 실패 시 매니저 삭제 수행하지 않음
    - `repo`가 nil인 경우 저장소 연산 건너뛰기 (하위 호환성)

12. **AgentServiceAdapter 테스트 갱신** (`internal/api/service/agent_adapter_test.go`)
    - Mock AgentRepository 추가
    - CreateAgent: 저장소 Save 호출 검증
    - CreateAgent: Save 실패 시 롤백 검증
    - UpdateAgent: 저장소 갱신 검증
    - DeleteAgent: 저장소 삭제 검증
    - repo=nil 일 때 패닉 없이 동작 검증

13. **서버 시작 로직 수정** (`cmd/xflowd/main.go`)
    - AgentRepository 초기화 (FlowRepository 직후)
    - 저장소에서 에이전트 로드:
      ```
      configs, _ := agentRepo.List(ctx)
      for _, cfg := range configs {
          if _, err := agentMgr.Create(cfg); err != nil {
              logger.Warn("에이전트 복원 실패", "id", cfg.ID, "error", err)
          }
      }
      ```
    - `NewAgentServiceAdapter` 호출 시 repo 전달
    - defer agentRepo.Close()

**관련 요구사항**: REQ-1 (생성 시 영속화), REQ-2 (시작 시 복원), REQ-3 (삭제 시 제거), REQ-4 (업데이트 시 갱신), REQ-7 (타입 등록 선행)

---

## 파일별 변경 상세

### 신규 파일

| 파일 | 설명 | 의존성 |
|------|------|--------|
| `internal/agent/serialize.go` | AgentConfig JSON/YAML 직렬화 | `agent.AgentConfig`, `yaml.v3`, `encoding/json` |
| `internal/agent/serialize_test.go` | 직렬화 테스트 | `serialize.go` |
| `internal/storage/agent_repository.go` | AgentRepository 인터페이스 | `agent.AgentConfig` |
| `internal/storage/agent_file.go` | File 백엔드 | `agent_repository.go`, `serialize.go` |
| `internal/storage/agent_sqlite.go` | SQLite 백엔드 | `agent_repository.go`, `serialize.go` |
| `internal/storage/agent_postgres.go` | PostgreSQL 백엔드 | `agent_repository.go`, `serialize.go` |
| `internal/storage/agent_file_test.go` | File 백엔드 테스트 | `agent_file.go` |
| `internal/storage/agent_sqlite_test.go` | SQLite 백엔드 테스트 | `agent_sqlite.go` |

### 수정 파일

| 파일 | 변경 내용 | 영향 범위 |
|------|----------|----------|
| `internal/storage/factory.go` | `NewAgentRepository()` 추가 | 신규 함수, 기존 코드 변경 없음 |
| `internal/storage/factory_test.go` | Agent Factory 테스트 추가 | 테스트 추가, 기존 테스트 변경 없음 |
| `internal/api/service/agent_adapter.go` | repo 필드, 생성자 변경, CRUD 저장소 연동 | 시그니처 변경으로 호출부 영향 |
| `internal/api/service/agent_adapter_test.go` | Mock repo 추가, 저장소 테스트 | 테스트 확장 |
| `cmd/xflowd/main.go` | AgentRepo 초기화, 로드 로직, 어댑터 생성자 변경 | 서버 시작 흐름에 3줄 추가 |

---

## 기술 접근 방식

### 패턴: FlowRepository 미러링

기존 `FlowRepository` 패턴을 그대로 따른다:

- 동일한 인터페이스 구조 (Save, Get, List, Delete, Close)
- 동일한 Factory 패턴
- 동일한 에러 처리 패턴 (sentinel error)
- 동일한 백엔드별 구현 패턴 (atomic write, UPSERT, auto-migration)

이 접근 방식의 장점:
- 검증된 패턴 재사용으로 구현 위험 최소화
- 코드베이스 일관성 유지
- 기존 테스트 패턴 재활용 가능

### DB 연결 재사용 전략

SQLite와 PostgreSQL 백엔드에서 기존 FlowRepository와 동일한 DB를 사용하되, 별도의 `*sql.DB` / `*pgxpool.Pool` 인스턴스를 생성한다. 이는:

- FlowRepository와 AgentRepository의 생명주기 독립성 보장
- Close() 호출 시 상호 영향 없음
- 동일 DB 파일/서버에 테이블만 추가

### 롤백 전략 (CreateAgent)

```
1. manager.Create(config) → 성공 → 에이전트 인메모리 등록
2. repo.Save(config)       → 실패 → manager.Delete(config.ID)로 롤백
3. 저장소 저장 실패 에러 반환
```

### 하위 호환성

`NewAgentServiceAdapter`의 시그니처가 변경되므로, `repo`가 nil인 경우 저장소 연산을 건너뛰는 가드를 추가한다. 이를 통해 테스트 코드나 저장소 없이 사용하는 시나리오에서도 기존 동작이 유지된다.

---

## 테스트 전략

### 단위 테스트

| 대상 | 테스트 내용 | 방법 |
|------|-----------|------|
| `serialize.go` | JSON/YAML roundtrip | TDD (RED-GREEN-REFACTOR) |
| `agent_file.go` | File CRUD | TDD, t.TempDir() 사용 |
| `agent_sqlite.go` | SQLite CRUD | TDD, 인메모리 SQLite |
| `agent_postgres.go` | Postgres CRUD | 통합 테스트 (CI 전용) |
| `agent_adapter.go` | 저장소 연동 | DDD + Mock Repository |

### 테스트 격리

- File 테스트: `t.TempDir()` 사용으로 임시 디렉토리 자동 정리
- SQLite 테스트: `:memory:` DSN 사용 또는 t.TempDir() 내 파일
- Postgres 테스트: `//go:build integration` 빌드 태그로 분리

### 커버리지 목표

- 신규 코드: 85% 이상
- 수정 코드 (`agent_adapter.go`): 기존 커버리지 유지 또는 개선

---

## 위험 및 대응

| 위험 | 심각도 | 대응 계획 |
|------|--------|----------|
| `AgentConfig` 직렬화 시 `time.Duration` 호환성 | 중간 | 보조 구조체로 문자열 변환 처리, roundtrip 테스트로 검증 |
| `map[string]any` (Transport.Options) 타입 유실 | 중간 | JSON/YAML 직렬화 후 역직렬화 시 타입 변환 처리 (float64 -> int 등) |
| 서버 시작 시 에이전트 로드 실패로 전체 시작 차단 | 높음 | 개별 에이전트 로드 실패는 로깅 후 건너뛰기 |
| `NewAgentServiceAdapter` 시그니처 변경으로 기존 코드 깨짐 | 낮음 | 호출부가 `cmd/xflowd/main.go` 1곳이므로 영향 최소 |
| SQLite/Postgres DB 공유 시 마이그레이션 충돌 | 낮음 | 각 백엔드가 독립적으로 CREATE TABLE IF NOT EXISTS 수행 |

---

## 추적성

| 요구사항 | 구현 단계 | 테스트 |
|----------|----------|--------|
| REQ-1 | 단계 11 (CreateAgent 수정) | 단계 12 (CreateAgent 테스트) |
| REQ-2 | 단계 13 (서버 시작 로직) | acceptance.md AC-2 |
| REQ-3 | 단계 11 (DeleteAgent 수정) | 단계 12 (DeleteAgent 테스트) |
| REQ-4 | 단계 11 (UpdateAgent 수정) | 단계 12 (UpdateAgent 테스트) |
| REQ-5 | 단계 4, 6, 8 (3개 백엔드) | 단계 5, 7, 테스트 |
| REQ-6 | 단계 4, 6, 8 (격리 구조) | 단계 5, 7, 테스트 |
| REQ-7 | 단계 13 (로드 순서) | acceptance.md AC-7 |
