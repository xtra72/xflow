# SPEC-AGENT-STORE-001: 수용 기준

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-AGENT-STORE-001 |
| 관련 요구사항 | REQ-1 ~ REQ-7 |
| 검증 방법 | 자동 테스트 + 수동 검증 |

---

## 수용 기준 (Given-When-Then)

### AC-1: 에이전트 생성 시 영속화 (REQ-1)

**Given** xflowd 서버가 File/SQLite/Postgres 저장소 중 하나로 구성되어 실행 중일 때
**When** API `POST /api/v1/agents`를 통해 새 에이전트를 생성하면
**Then** 에이전트 설정이 저장소에 영속화되어야 한다:
- File 백엔드: `{directory}/agents/{agentID}.yaml` 파일이 생성된다
- SQLite 백엔드: `agents` 테이블에 레코드가 삽입된다
- Postgres 백엔드: `agents` 테이블에 레코드가 삽입된다
- 저장된 설정은 생성 시 전달된 설정과 동일해야 한다 (Name, Type, Transport, Metadata)

**Given** 에이전트 생성 API 호출 시
**When** 저장소 쓰기가 실패하면
**Then** 에이전트 생성이 롤백되어야 한다:
- Manager에서 해당 에이전트가 제거된다
- API는 에러 응답을 반환한다
- 인메모리와 저장소 간 불일치가 발생하지 않는다

---

### AC-2: 서버 재시작 시 에이전트 복원 (REQ-2)

**Given** 서버에 에이전트 3개(A, B, C)가 생성되어 저장소에 영속화된 상태에서
**When** 서버를 종료 후 재시작하면
**Then** 이전에 생성된 3개 에이전트가 모두 복원되어야 한다:
- `GET /api/v1/agents`에서 3개 에이전트가 조회된다
- 각 에이전트의 Name, Type, Transport 설정이 원본과 동일하다
- 복원된 에이전트의 상태는 `stopped` (자동 시작하지 않음)

**Given** 저장소에 유효하지 않은 에이전트 설정이 포함된 상태에서
**When** 서버가 시작되면
**Then** 유효하지 않은 에이전트는 건너뛰고 나머지는 정상 로드된다:
- 유효하지 않은 에이전트에 대해 경고 로그가 출력된다
- 서버 시작 자체는 차단되지 않는다
- 유효한 에이전트는 모두 정상 복원된다

---

### AC-3: 에이전트 삭제 시 저장소 제거 (REQ-3)

**Given** 저장소에 영속화된 에이전트가 존재할 때
**When** API `DELETE /api/v1/agents/{id}`를 호출하면
**Then** 에이전트가 저장소에서도 제거되어야 한다:
- File 백엔드: 해당 YAML 파일이 삭제된다
- SQLite/Postgres 백엔드: 해당 레코드가 삭제된다
- 서버 재시작 후 삭제된 에이전트가 복원되지 않는다

**Given** 저장소 삭제가 실패할 때
**When** 에이전트 삭제 API를 호출하면
**Then** Manager에서도 삭제되지 않아야 한다:
- 인메모리와 저장소의 일관성이 유지된다
- API는 에러 응답을 반환한다

---

### AC-4: 에이전트 업데이트 시 저장소 갱신 (REQ-4)

**Given** 저장소에 영속화된 에이전트가 존재할 때
**When** API `PUT /api/v1/agents/{id}`를 통해 Name을 변경하면
**Then** 저장소의 에이전트 설정도 갱신되어야 한다:
- 서버 재시작 후 변경된 Name으로 복원된다
- 변경되지 않은 필드(Type, Transport 등)는 유지된다

---

### AC-5: 3개 저장소 백엔드 동작 (REQ-5)

**Given** 저장소 타입이 `file`로 구성되었을 때
**When** 에이전트 CRUD 작업을 수행하면
**Then** `{directory}/agents/` 디렉토리에 YAML 파일로 저장/조회/삭제된다

**Given** 저장소 타입이 `sqlite`로 구성되었을 때
**When** 에이전트 CRUD 작업을 수행하면
**Then** 동일 SQLite DB의 `agents` 테이블에서 저장/조회/삭제된다

**Given** 저장소 타입이 `postgres`로 구성되었을 때
**When** 에이전트 CRUD 작업을 수행하면
**Then** 동일 Postgres DB의 `agents` 테이블에서 저장/조회/삭제된다

---

### AC-6: 저장소 격리 (REQ-6)

**Given** File 백엔드가 구성되었을 때
**When** 에이전트와 플로우를 각각 저장하면
**Then** 에이전트는 `{directory}/agents/` 하위에, 플로우는 `{directory}/` 하위에 저장되어 서로 간섭하지 않는다

**Given** SQLite/Postgres 백엔드가 구성되었을 때
**When** 에이전트와 플로우를 각각 저장하면
**Then** 에이전트는 `agents` 테이블에, 플로우는 `flows` 테이블에 저장되어 서로 간섭하지 않는다

---

### AC-7: 타입 레지스트리 선행 등록 (REQ-7)

**Given** 저장소에 `type: "samsung-nasa"` 에이전트가 영속화된 상태에서
**When** 서버가 시작되면
**Then** Samsung NASA 타입이 먼저 등록된 후 에이전트가 로드되어 정상 복원된다:
- 타입이 등록되지 않은 상태에서 로드를 시도하면 해당 에이전트만 건너뛴다
- 타입 등록 순서: HTTP -> ConsoleLogger -> MQTT -> InfluxDB -> Samsung NASA -> 에이전트 로드

---

## 직렬화 관련 수용 기준

### AC-8: AgentConfig 직렬화 왕복 검증

**Given** 모든 필드가 채워진 AgentConfig (Transport.Options 포함)가 있을 때
**When** JSON으로 직렬화 후 역직렬화하면
**Then** 원본과 동일한 AgentConfig가 복원된다:
- `time.Duration` 필드가 문자열 형식으로 유지된다 (예: "30s")
- `map[string]any` 필드의 타입이 보존된다
- Metadata의 한글 문자열이 정상 처리된다

**Given** 모든 필드가 채워진 AgentConfig가 있을 때
**When** YAML로 직렬화 후 역직렬화하면
**Then** 원본과 동일한 AgentConfig가 복원된다

---

## 품질 게이트 기준

### Definition of Done

- [ ] 모든 수용 기준(AC-1 ~ AC-8)에 대한 자동 테스트가 존재한다
- [ ] 신규 코드 커버리지 85% 이상
- [ ] `go vet ./...` 경고 없음
- [ ] `go test -race ./...` 통과 (동시성 안전)
- [ ] 기존 테스트 전체 통과 (회귀 없음)
- [ ] AgentRepository 인터페이스에 대한 컴파일 타임 인터페이스 검증 존재
- [ ] `cmd/xflowd/main.go` 서버 시작 흐름이 정상 동작

---

## 수동 검증 절차

### 검증 1: File 백엔드 영속화 확인

1. `storage.type=file`, `storage.file.directory=./data` 로 서버 시작
2. `curl -X POST /api/v1/agents` 로 에이전트 생성
3. `ls ./data/agents/` 로 YAML 파일 존재 확인
4. YAML 파일 내용이 생성 요청과 일치하는지 확인
5. 서버 종료 후 재시작
6. `curl GET /api/v1/agents` 로 에이전트 복원 확인

### 검증 2: SQLite 백엔드 영속화 확인

1. `storage.type=sqlite`, `storage.sqlite.path=./data/xflow.db` 로 서버 시작
2. 에이전트 생성
3. `sqlite3 ./data/xflow.db "SELECT * FROM agents;"` 로 레코드 확인
4. 서버 재시작 후 에이전트 복원 확인

### 검증 3: 삭제 후 재시작

1. 에이전트 2개 생성 (A, B)
2. 에이전트 A 삭제
3. 서버 재시작
4. 에이전트 B만 복원되고 A는 복원되지 않음 확인

### 검증 4: 업데이트 후 재시작

1. 에이전트 생성 (Name: "original")
2. 에이전트 Name을 "updated"로 변경
3. 서버 재시작
4. 에이전트 Name이 "updated"인지 확인

### 검증 5: 타입 에이전트 복원

1. Samsung NASA 타입 에이전트 생성 (`examples/agents/samsung-nasa-serial.yaml` 참조)
2. 서버 재시작
3. 에이전트가 올바른 타입으로 복원되는지 확인
4. 에이전트 시작 가능 여부 확인

---

## 추적성 매트릭스

| 요구사항 | 수용 기준 | 자동 테스트 | 수동 검증 |
|----------|----------|-----------|----------|
| REQ-1 | AC-1 | agent_adapter_test.go | 검증 1, 2 |
| REQ-2 | AC-2 | agent_adapter_test.go, main 통합 | 검증 1, 2, 5 |
| REQ-3 | AC-3 | agent_adapter_test.go | 검증 3 |
| REQ-4 | AC-4 | agent_adapter_test.go | 검증 4 |
| REQ-5 | AC-5 | agent_file_test.go, agent_sqlite_test.go | 검증 1, 2 |
| REQ-6 | AC-6 | 백엔드별 격리 테스트 | 검증 1 (디렉토리 확인) |
| REQ-7 | AC-7 | 서버 시작 순서 테스트 | 검증 5 |
| - | AC-8 | serialize_test.go | - |
