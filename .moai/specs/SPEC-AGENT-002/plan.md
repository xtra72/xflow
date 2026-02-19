---
id: SPEC-AGENT-002
type: plan
version: "1.0.0"
spec_ref: SPEC-AGENT-002
---

# SPEC-AGENT-002 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반)
  - 기존 파일 수정(agent.go, agent handler): DDD (ANALYZE-PRESERVE-IMPROVE)
  - 신규 파일(예제 파일): TDD (RED-GREEN-REFACTOR)
- 기존 flow export/import 패턴을 참조하여 일관된 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행

### 1.2 기술 스택

- **언어**: Go 1.23+
- **CLI 프레임워크**: `github.com/spf13/cobra` + `github.com/spf13/viper`
- **직렬화**: `encoding/json`, `gopkg.in/yaml.v3`
- **파일 I/O**: `os`, `path/filepath`
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`

### 1.3 참조 패턴

- `internal/cli/flow.go`: `newFlowExportCmd()` (line 272-323), `newFlowImportCmd()` (line 327-363)
- `internal/cli/root.go`: `readFile()`, `detectFileFormat()` 공유 헬퍼
- `internal/cli/agent.go`: `newAgentCreateCmd()` 파일 기반 생성 패턴

---

## 2. 마일스톤

### Primary Goal: CLI Export/Import + 예제 파일 (P0)

**범위**: Module 1, 2, 5

**작업 항목**:

1. `internal/cli/agent.go` 확장 - Export 커맨드
   - `newAgentExportCmd(client **Client) *cobra.Command` 함수 추가
   - `stripRuntimeFields(agent map[string]any) map[string]any` 헬퍼 추가
   - `newAgentCmd()` 에 `cmd.AddCommand(newAgentExportCmd(client))` 등록
   - 단일 에이전트 Export 로직 구현 (flow export 패턴 참조)
   - `-o` (output) 플래그 처리 및 필수 검증
   - JSON/YAML 형식 자동 감지 (`detectFileFormat()` 재사용)
   - 런타임 필드 제거 (id, status, connected 등)

2. `internal/cli/agent.go` 확장 - Import 커맨드
   - `newAgentImportCmd(client **Client) *cobra.Command` 함수 추가
   - `newAgentCmd()` 에 `cmd.AddCommand(newAgentImportCmd(client))` 등록
   - 파일 읽기 및 파싱 (`readFile()`, `detectFileFormat()` 재사용)
   - `AgentCreateRequest` 형식 래핑 (name, type, config)
   - `POST /api/v1/agents` API 호출

3. `internal/cli/agent_test.go` 확장 - Export/Import 테스트
   - Export 커맨드 JSON 출력 테스트
   - Export 커맨드 YAML 출력 테스트
   - Export 런타임 필드 제거 테스트
   - Export 출력 경로 미지정 에러 테스트
   - Import 커맨드 JSON 파일 테스트
   - Import 커맨드 YAML 파일 테스트
   - Import 파일 경로 미지정 에러 테스트
   - Import 잘못된 파일 형식 에러 테스트

4. `examples/agents/` 디렉토리 및 예제 파일 생성
   - `serial-modbus.yaml`: Serial Transport + Modbus 프로토콜 예제
   - `tcp-custom.json`: TCP Transport + Custom 프로토콜 예제
   - `mqtt-sensor.yaml`: MQTT 기반 센서 에이전트 예제
   - 각 파일에 `name`, `type` 필수 필드 포함
   - YAML 파일에 설명 주석 포함

**완료 기준**:
- `agent export <id> -o <file>` 명령 동작
- `agent import -f <file>` 명령 동작
- 예제 파일 3개 존재 및 Import 호환
- Export/Import 테스트 통과
- `go test -race ./internal/cli/...` 통과

### Secondary Goal: Batch Operations (P1)

**범위**: Module 3

**작업 항목**:

1. `internal/cli/agent.go` 확장 - Batch Import
   - `importAgentFromDir(client **Client, dirPath string, format string, w io.Writer) error` 헬퍼 추가
   - `newAgentImportCmd` 내 디렉토리 감지 분기 추가 (`os.Stat` -> `IsDir()`)
   - `filepath.WalkDir()` 로 `.json`, `.yaml`, `.yml` 파일 탐색
   - 성공/실패 카운트 집계 및 요약 출력
   - 부분 실패 시 에러 출력 후 계속 처리

2. `internal/cli/agent.go` 확장 - Batch Export
   - `newAgentExportCmd` 내 `--all` 플래그 추가
   - `--export-format` 플래그 추가 (기본: yaml)
   - `--all` 시 전체 에이전트 조회 -> 디렉토리 생성 -> 개별 파일 저장
   - 파일명 생성 로직 (`sanitizeFileName()` 헬퍼)

3. 테스트 확장
   - Batch Import 디렉토리 테스트
   - Batch Import 부분 실패 테스트
   - Batch Export 전체 에이전트 테스트
   - Batch Export 형식 지정 테스트

**완료 기준**:
- `agent import -f <directory>` 명령 동작
- `agent export --all -o <directory>` 명령 동작
- Batch 성공/실패 요약 출력
- 부분 실패 시 계속 처리

### Tertiary Goal: API Export Endpoint (P1)

**범위**: Module 4

**작업 항목**:

1. `internal/api/handler/agent.go` 확장
   - `Export(ctx api.Context) error` 핸들러 추가
   - `ExportAll(ctx api.Context) error` 핸들러 추가
   - `RegisterRoutes` 에 2개 라우트 추가
   - `stripExportFields()` 헬퍼 (CLI 버전과 공유 가능)

2. 테스트 추가
   - `internal/api/handler/agent_test.go` 에 Export/ExportAll 테스트
   - 존재하지 않는 에이전트 Export 시 404 응답 테스트
   - ExportAll 빈 목록 테스트

**완료 기준**:
- `GET /api/v1/agents/{id}/export` 동작
- `GET /api/v1/agents/export` 동작
- 런타임 필드 제외된 응답
- 404 에러 처리

---

## 3. 기술 접근 방법

### 3.1 Export 구현 전략

flow export 패턴(`internal/cli/flow.go:272-323`)을 충실히 따른다:

1. API로 에이전트 정보 조회 (`GET /api/v1/agents/{id}`)
2. 런타임 필드 제거 (`stripRuntimeFields` 헬퍼)
3. 출력 파일 확장자 기반 형식 감지 (`detectFileFormat` 재사용)
4. JSON/YAML 직렬화
5. 파일 저장

**핵심 결정**: `stripRuntimeFields`는 `agent.go` 파일 내 패키지 레벨 함수로 구현한다. API handler에서도 동일한 런타임 필드 제거가 필요하므로, 공통 필드 목록은 상수 슬라이스로 정의한다.

### 3.2 Import 구현 전략

flow import 패턴(`internal/cli/flow.go:327-363`)을 따르되, agent 특화 래핑을 적용한다:

1. 파일 읽기 (`readFile` 재사용)
2. 형식 감지 및 파싱 (`detectFileFormat` 재사용)
3. `AgentCreateRequest` 형식 래핑
   - 파일의 `name` 필드 -> request.name
   - 파일의 `type` 필드 -> request.type
   - 파일의 나머지 필드 -> request.config (또는 `config` 키가 있으면 그대로 사용)
4. `POST /api/v1/agents` API 호출

**핵심 결정**: Import 시 `config` 키 유무에 따라 두 가지 케이스를 처리한다:
- `config` 키가 있으면: `{name, type, config}` 그대로 전송
- `config` 키가 없으면: `name`, `type`을 추출하고 나머지를 `config`로 래핑

### 3.3 Batch Operations 전략

디렉토리 감지는 `os.Stat` + `FileInfo.IsDir()`로 수행하며, Import/Export 단일 로직을 루프에서 호출하여 재사용한다.

### 3.4 API Export 전략

기존 `AgentHandler`의 `GetAgent`/`ListAgents` 호출을 재사용하되, 응답 형식만 다르게 처리한다. `dto.NewSuccessResponse()` 래핑 대신 원본 데이터를 직접 반환하여 파일 저장/Import 호환성을 보장한다.

---

## 4. 리스크 및 대응 방안

### 4.1 기존 코드 변경 리스크

| 리스크 | 영향 | 대응 |
|--------|------|------|
| `agent.go` 서브커맨드 추가 시 기존 커맨드 영향 | 낮음 | 독립적인 AddCommand 호출로 격리 |
| `agent handler` 라우트 추가 시 경로 충돌 | 낮음 | `/export` 경로는 `/{id}` 와 충돌 없음 |
| Export 런타임 필드 목록 누락 | 중간 | AgentInfo 구조체 필드와 Export 필드를 명시적 화이트리스트로 관리 |

### 4.2 기술적 리스크

| 리스크 | 영향 | 대응 |
|--------|------|------|
| YAML 직렬화 시 map 키 순서 불일치 | 낮음 | 순서 무관, 기능에 영향 없음 |
| Batch Import 시 대량 파일 성능 | 낮음 | 순차 처리로 단순화, 필요 시 병렬화 가능 |
| Export/Import 라운드트립 호환성 | 중간 | Export -> Import 라운드트립 테스트 필수 |

---

## 5. 의존성 순서

```
Module 5 (예제 파일) ─── 의존성 없음 (독립 실행 가능)
     │
Module 1 (Export CLI) ── Module 5 (테스트에서 예제 파일 참조 가능)
     │
Module 2 (Import CLI) ── Module 5 (Import 테스트에 예제 파일 사용)
     │
Module 3 (Batch Ops) ── Module 1, 2 (단일 Export/Import 로직 재사용)
     │
Module 4 (API Export) ── Module 1 (stripRuntimeFields 로직 공유)
```

**권장 구현 순서**: Module 5 -> Module 1 -> Module 2 -> Module 3 -> Module 4

---

## 6. 테스트 전략

### 6.1 테스트 구조

| 테스트 파일 | 대상 모듈 | 테스트 유형 |
|------------|----------|-----------|
| `internal/cli/agent_test.go` | Module 1, 2, 3 | CLI 통합 테스트 |
| `internal/api/handler/agent_test.go` | Module 4 | API 핸들러 테스트 |

### 6.2 테스트 시나리오

**Export 테스트**:
- JSON 형식 Export 후 파일 내용 검증
- YAML 형식 Export 후 파일 내용 검증
- 런타임 필드 제거 검증
- 출력 경로 미지정 시 에러 검증
- 존재하지 않는 에이전트 Export 에러 검증

**Import 테스트**:
- JSON 파일 Import 성공 검증
- YAML 파일 Import 성공 검증
- 파일 경로 미지정 에러 검증
- 잘못된 파일 형식 에러 검증
- Export -> Import 라운드트립 검증

**Batch 테스트**:
- 디렉토리 일괄 Import 성공 검증
- 부분 실패 시 계속 처리 검증
- 전체 에이전트 Export 검증
- 빈 디렉토리 처리 검증

**API 테스트**:
- 단일 에이전트 Export 응답 검증
- 전체 에이전트 Export 응답 검증
- 존재하지 않는 에이전트 404 검증

### 6.3 커버리지 목표

- 전체: 85%+
- 신규 코드: 90%+
- Export/Import 핵심 로직: 95%+

---

## 7. 수정 대상 파일 목록

| 파일 | 변경 유형 | 모듈 |
|------|----------|------|
| `internal/cli/agent.go` | 수정 (확장) | 1, 2, 3 |
| `internal/cli/agent_test.go` | 수정 (확장) | 1, 2, 3 |
| `internal/api/handler/agent.go` | 수정 (확장) | 4 |
| `internal/api/handler/agent_test.go` | 수정 (확장) | 4 |
| `examples/agents/serial-modbus.yaml` | 신규 생성 | 5 |
| `examples/agents/tcp-custom.json` | 신규 생성 | 5 |
| `examples/agents/mqtt-sensor.yaml` | 신규 생성 | 5 |
