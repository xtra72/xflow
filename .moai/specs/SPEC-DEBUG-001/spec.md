# SPEC-DEBUG-001: Output 노드를 Debug 노드로 통합

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-DEBUG-001 |
| 제목 | Output 노드를 Debug 노드로 통합 |
| 생성일 | 2026-04-06 |
| 상태 | Completed (v1.2.0) |
| 완료일 | 2026-05-21 |
| 우선순위 | High |
| 담당 | expert-backend |
| 관련 SPEC | SPEC-LOG-001, SPEC-NODE-001 |

---

## Environment (환경)

- **프로젝트**: xflow - IoT/빌딩 자동화를 위한 플로우 기반 데이터 처리 엔진
- **언어/런타임**: Go 1.22+
- **대상 모듈**: `internal/node/debug.go`, `internal/node/output.go`
- **노드 라이프사이클**: BaseNode 임베딩, Configure, Init, Process, Shutdown
- **레지스트리**: `internal/node/registry.go`에서 "debug", "output" 모두 builtin 노드로 등록
- **기존 debug 노드**: 118줄, logLevel/filePath/file/mu 필드, pass-through 동작
- **기존 output 노드**: 145줄, prefix/tmpl/filePath/file/mu 필드, Go text/template 지원, pass-through 동작
- **개발 방법론**: hybrid (새 코드: TDD, 기존 코드: DDD)

---

## Assumptions (가정)

- A1: output 노드와 debug 노드는 기능적 차이가 없으며, 통합이 사용자 경험을 개선한다
- A2: 기존 output 노드 설정(prefix, template, file)을 사용하는 플로우 YAML 파일이 존재할 수 있으며, 하위 호환성이 필요하다
- A3: "output" 타입은 레지스트리에서 NewDebugNode의 별칭(alias)으로 유지되어 기존 설정이 깨지지 않는다
- A4: DebugSink 인터페이스는 플로우 에디터 디버그 패널로 메시지를 전송하기 위한 새로운 인터페이스이다
- A5: 선택적 필드 출력(fields)은 payload의 최상위 키만 필터링한다 (중첩 필드 필터링은 범위 밖)

---

## Requirements (요구사항)

### 기능 요구사항

#### REQ-1: 메시지 템플릿 기능 (Event-Driven)

**WHEN** debug 노드의 config에 `template` 필드가 설정되어 있을 때,
**THEN** Go text/template을 사용하여 payload 데이터로 템플릿을 실행하고, 결과를 출력해야 한다.

**WHEN** `template` 필드가 비어있거나 설정되지 않았을 때,
**THEN** 기존 debug 노드의 기본 포맷("message id=<ID> payload=<map> metadata=<map>")으로 출력해야 한다.

#### REQ-2: 출력 대상 선택 (State-Driven)

**IF** `output` 설정이 `slog`이면 **THEN** 노드의 ComponentLogger(slog)를 통해 설정된 log level로 출력해야 한다 (기본값).
**IF** `output` 설정이 `logger`이면 **THEN** `agent_ref`로 지정된 에이전트(예: ConsoleLogger)의 AgentTransport를 통해 메시지를 전송해야 한다.
**IF** `output` 설정이 `terminal`이면 **THEN** os.Stdout에 직접 출력해야 한다.
**IF** `output` 설정이 `file`이면 **THEN** `file` 경로로 지정된 파일에 출력해야 한다.
**IF** `output` 설정이 `editor`이면 **THEN** DebugSink 인터페이스를 통해 플로우 에디터 디버그 패널로 전송해야 한다.

#### REQ-3: 선택적 필드 출력 (State-Driven)

**IF** `fields` 설정이 비어있거나 nil이면 **THEN** payload의 모든 필드를 출력해야 한다.
**IF** `fields` 설정에 특정 키 목록이 지정되어 있으면 **THEN** payload에서 해당 키에 대응하는 값만 포함하여 출력해야 한다.

#### REQ-4: prefix 지원 (Event-Driven)

**WHEN** `prefix` 설정이 지정되어 있을 때, **THEN** 출력 앞에 해당 prefix를 붙여야 한다.
**WHEN** `prefix` 설정이 없을 때, **THEN** 노드 이름을 기본 prefix로 사용해야 한다.

#### REQ-5: 하위 호환성 (Ubiquitous)

시스템은 **항상** 레지스트리에서 "output" 타입을 NewDebugNode 팩토리의 별칭으로 유지하여, 기존 "output" 타입을 사용하는 플로우 정의가 정상 동작해야 한다.

시스템은 **항상** 기존 debug 노드의 config 키(level, file)를 변경 없이 지원해야 한다.

시스템은 **항상** 기존 output 노드의 config 키(prefix, template, file)를 debug 노드에서 동일하게 지원해야 한다.

### 비기능 요구사항

#### REQ-6: Pass-through 동작 유지 (Ubiquitous)

통합된 debug 노드는 **항상** 입력 메시지를 변경 없이 그대로 반환(pass-through)해야 한다.

#### REQ-7: 스레드 안전성 (Ubiquitous)

파일 출력은 **항상** sync.RWMutex를 사용하여 동시 접근으로부터 보호되어야 한다.

### 금지 요구사항

#### REQ-8: output.go 제거 (Unwanted)

통합 완료 후, `internal/node/output.go` 파일은 **존재하지 않아야 한다** (삭제).
`internal/node/output_test.go` 파일도 **존재하지 않아야 한다** (테스트를 debug_test.go로 이관 후 삭제).

---

## Specifications (상세 명세)

### 향상된 DebugNode 구조체

```yaml
config:
  level: "debug"                      # 로그 레벨: debug/info/warn (기본값: debug)
  output: "slog"                      # 출력 대상: slog/logger/terminal/file/editor (기본값: slog)
  agent_ref: "console-logger"        # 에이전트 참조 (output=logger 일 때 필요)
  file: "/path/to/file"              # 파일 경로 (output=file 일 때만 사용)
  template: "{{.temperature}}C"       # Go text/template (선택사항)
  prefix: "[debug]"                   # 출력 prefix (선택사항, 기본값: 노드 이름)
  fields: ["temperature", "humidity"] # 선택적 payload 필드 (선택사항, 비어있으면 전체)
```

### DebugSink 인터페이스

```go
// DebugSink는 플로우 에디터 디버그 패널로 메시지를 전송하는 인터페이스
type DebugSink interface {
    SendDebug(nodeID string, message string) error
}
```

### 출력 포맷 규칙

1. **fields가 설정된 경우**: payload에서 지정된 키만 추출하여 새 map 생성
2. **template가 설정된 경우**: 필터링된(또는 전체) payload로 템플릿 실행
3. **template가 없는 경우**: 기본 포맷 사용 (`message id=<ID> payload=<map> metadata=<map>`)
4. **prefix 적용**: 최종 출력 문자열 앞에 prefix 추가

### 처리 순서

1. payload에서 fields 필터링 적용
2. template 또는 기본 포맷으로 출력 문자열 생성
3. prefix 추가
4. 설정된 output 대상으로 전송

### 레지스트리 변경

```go
// registry.go 변경
Register("debug", "debug", NewDebugNode)   // 기존 유지
Register("output", "debug", NewDebugNode)  // output을 debug의 별칭으로 변경
```

### 수정 대상 파일

| 파일 | 작업 |
|------|------|
| `internal/node/debug.go` | template, output, prefix, fields 기능 추가 |
| `internal/node/output.go` | 삭제 |
| `internal/node/output_test.go` | 삭제 (테스트를 debug_test.go로 이관) |
| `internal/node/debug_test.go` | 새 기능 테스트 추가, output 테스트 이관 |
| `internal/node/registry.go` | "output" 팩토리를 NewDebugNode로 변경 |
| `internal/node/registry_test.go` | 필요시 업데이트 |

---

## Traceability (추적성)

| 요구사항 | plan.md 마일스톤 | acceptance.md 시나리오 |
|----------|-----------------|----------------------|
| REQ-1 | M1: 템플릿 기능 | AC-1, AC-2 |
| REQ-2 | M2: 출력 대상 | AC-3, AC-4, AC-5, AC-6 |
| REQ-3 | M3: 필드 필터링 | AC-7, AC-8 |
| REQ-4 | M1: 템플릿 기능 | AC-9, AC-10 |
| REQ-5 | M4: 하위 호환성 | AC-11, AC-12, AC-13 |
| REQ-6 | 전체 | AC-14 |
| REQ-7 | 전체 | AC-15 |
| REQ-8 | M5: 정리 | AC-16 |

---

## 구현 노트 (Implementation Notes)

**구현일**: 2026-04-06
**커밋**: `be548e2` feat(node): Output 노드를 Debug 노드로 통합 (SPEC-DEBUG-001)
**개발 방법론**: Hybrid (DDD + TDD)

### 변경 파일

| 파일 | 작업 | 변경량 |
|------|------|--------|
| `internal/node/debug.go` | 수정 (기능 추가) | +164 lines |
| `internal/node/debug_test.go` | 수정 (테스트 추가) | +652 lines |
| `internal/node/registry.go` | 수정 (alias 변경) | 1 line |
| `internal/node/output.go` | 삭제 | -144 lines |
| `internal/node/output_test.go` | 삭제 | -149 lines |

### 검증 결과

- `go test -race ./internal/node/...` → PASS
- `go vet ./internal/node/...` → PASS (경고 없음)
- debug.go 커버리지: Process 100%, Configure 96.4%, Shutdown 100%
- TRUST 5 품질 검증: PASS (5/5)

### 구현 범위

모든 요구사항(REQ-1 ~ REQ-8)이 계획대로 구현됨. 범위 확장/축소 없음.

---

## v1.1.0 변경사항 (2026-04-08)

### slog/logger 출력 분리

- **기본 출력 대상 변경**: `logger` → `slog` (서버 내부 ComponentLogger/slog)
- **logger 출력**: `agent_ref` 설정을 통해 ConsoleLogger 등 외부 에이전트로 메시지 전송
- **AgentResolver 통합**: Engine에서 주입된 AgentResolver를 통해 에이전트 transport 해석
- **Web UI 스키마 업데이트**: output 옵션에 slog/logger/editor/terminal/file 5종 노출

### 추가된 설정 키

| 키 | 타입 | 설명 |
|----|------|------|
| `agent_ref` | string | output=logger 시 사용할 에이전트 이름 (예: "console-logger") |

### 변경 파일

| 파일 | 변경 |
|------|------|
| `internal/node/debug.go` | AgentResolver/Transport 필드 추가, slog/logger 분리 |
| `internal/node/debug_test.go` | 기본 출력 대상 테스트 업데이트 |
| `web/src/config/nodeSchemas.ts` | output 옵션 5종, 기본값 slog |

---

## v1.2.0 변경사항 (2026-05-21)

### plain default 출력에 메시지 전체 포함

**배경**: 출력 필드를 명시적으로 지정하지 않으면(`fields` 빈 배열) plain 포맷이
payload 만 출력하던 결함. 사용자는 "출력 필드를 지정하지 않으면, 메시지 전체가
출력되어야" 한다고 지적 — metadata 가 빠져서 디버깅 시 어떤 노드로부터 어떤
trigger 로 emit 되었는지 추적 불가.

### 변경

- **v0.7.12**: `fields` 미지정 시 `payload` + `metadata` 를 모두 포함하여 출력.
  포맷: `<time> <level> <name> <payload_json> <metadata_json>` (공백 구분 2 JSON).
- **v0.7.13** (이어서 hotfix): 두 JSON 을 공백으로 나란히 출력하면 가독성이 떨어지고
  "출력 형식 맞지 않음" 사용자 피드백 발생. 단일 JSON 객체로 통합:
  `<time> <level> <name> {"id": ..., "payload": {...}, "metadata": {...}}`.

### 변경 파일

| 파일 | 변경 |
|------|------|
| `internal/node/debug.go` | `buildLogLine` 의 `len(dispFields)==0` 분기에 메시지 전체 (id/payload/metadata) 를 단일 JSON 으로 직렬화 |
