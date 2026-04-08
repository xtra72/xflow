---
id: SPEC-OBS-005
version: "1.0.0"
status: planned
created: "2026-04-06"
updated: "2026-04-06"
author: xtra
priority: high
tags: debug, output, node, merge, template, observability
prerequisite: SPEC-OBS-004
---

# SPEC-OBS-005: Debug 노드 통합 (output 노드 흡수)

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-04-06 | xtra | 최초 작성 |

---

## 1. 개요

기능이 거의 동일한 `debug` 노드와 `output` 노드를 하나의 통합 `debug` 노드로 병합한다. `output` 노드의 Go text/template 기능을 `debug` 노드에 흡수하고, 출력 대상(output destination)을 선택할 수 있도록 확장하며, 선택적 필드 출력 기능을 추가한다.

### 1.1 목적

- 중복 코드 제거: debug/output 노드의 파일 I/O, shutdown, lifecycle 코드 중복 해소
- 기능 통합: 템플릿 렌더링, 다중 출력 대상, 필드 선택을 하나의 노드로 제공
- 하위 호환성: 기존 debug/output 노드 설정이 그대로 동작

### 1.2 범위

- `debug` 노드: 확장된 설정 구조체로 리라이트
- `output` 노드: `debug` 노드로 흡수 후 파일 삭제, registry에서 alias 매핑
- 출력 대상: `logger`, `terminal`, `file`, `editor` 4종
- 필드 선택: payload/metadata 키 기반 선택적 출력

### 1.3 현재 구현 분석

#### DebugNode (internal/node/debug.go)

- 구조: `*BaseNode`, `logLevel`, `filePath`, `file`, `mu`
- 설정: `level` (string), `file` (string)
- 출력 형식: `message id=<ID> payload=<map> metadata=<map>`
- 동작: slog 로거로 로그 기록 후 메시지 pass-through

#### OutputNode (internal/node/output.go)

- 구조: `*BaseNode`, `prefix`, `tmpl`, `filePath`, `file`, `mu`
- 설정: `prefix` (string), `template` (string), `file` (string)
- 출력 형식: `[prefix] <template-rendered-or-json>`
- 동작: 템플릿 렌더링 또는 JSON 출력 후 메시지 pass-through

#### 공통 패턴 (중복)

- Init: 파일 열기 (`os.OpenFile`)
- Shutdown: 파일 닫기 (`file.Close()`)
- Process: 메시지 포맷팅 -> 로거 출력 -> 파일 출력 -> pass-through
- `sync.RWMutex` 기반 동시성 보호

---

## 2. 환경 (Environment)

### 기술 스택

- 언어: Go 1.23+
- 패키지 경로: `internal/node/`
- 의존성: `text/template` (표준 라이브러리), `log/slog` (표준 라이브러리)
- 테스트 프레임워크: Go 표준 `testing` 패키지
- 관련 SPEC: SPEC-OBS-004 (completed - 로그 출력 요소 식별 강화)

### 제약 조건

- 기존 `debug` 노드 설정(`level`, `file`)으로 작성된 플로우가 변경 없이 동작해야 한다
- 기존 `output` 노드 설정(`prefix`, `template`, `file`)으로 작성된 플로우가 변경 없이 동작해야 한다
- `output` 타입을 사용하는 기존 플로우 YAML이 호환되어야 한다

---

## 3. 가정 (Assumptions)

- A1: `output` 노드를 사용하는 기존 플로우는 registry의 타입 alias로 하위 호환성을 보장할 수 있다
- A2: `editor` 출력 대상은 현재 WebSocket 기반 로그 스트리밍(wsLogWriter)과 별도의 전용 채널이 필요하며, 초기 구현에서는 slog 로거를 사용하되 `"debug_output"` 속성을 추가하여 향후 WebSocket 필터링 포인트로 활용한다
- A3: Go `text/template`의 컴파일 결과는 thread-safe하게 재사용 가능하다
- A4: `fields` 설정은 빈 배열 또는 미설정 시 전체 메시지를 출력하는 것이 자연스러운 기본 동작이다
- A5: BaseNode.Logger()가 nil을 반환할 수 있으므로 nil 체크가 필요하다

---

## 4. 기능 요구사항 (Requirements)

### 4.1 통합 DebugNode 설정 구조체

**REQ-OBS-005-001** [유비쿼터스]
시스템은 **항상** 다음 설정 필드를 가진 `DebugNodeConfig`를 지원해야 한다:

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `level` | string | `"debug"` | 로그 레벨 (`"debug"`, `"info"`, `"warn"`) |
| `output` | string | `"logger"` | 출력 대상 (`"logger"`, `"terminal"`, `"file"`, `"editor"`) |
| `file` | string | `""` | 파일 경로 (`output="file"` 시 필수) |
| `template` | string | `""` | Go text/template 문자열 (선택) |
| `prefix` | string | `""` | 출력 접두사 (선택) |
| `fields` | []string | `[]` | 선택적 출력 필드 (빈 배열=전체 출력) |

**REQ-OBS-005-002** [이벤트 기반]
**WHEN** `output` 설정이 `"file"`이고 `file` 설정이 빈 문자열이면 **THEN** 설정 에러를 반환해야 한다.

**REQ-OBS-005-003** [유비쿼터스]
시스템은 **항상** `level`, `output`, `file` 설정에 대해 유효하지 않은 값이 주어지면 설정 에러를 반환해야 한다.

### 4.2 출력 대상 (Output Destination)

**REQ-OBS-005-004** [상태 기반]
**IF** `output`이 `"logger"`이면 **THEN** BaseNode의 slog 로거를 사용하여 설정된 `level`로 메시지를 출력해야 한다. 이것이 기존 debug 노드의 기본 동작이다.

**REQ-OBS-005-005** [상태 기반]
**IF** `output`이 `"terminal"`이면 **THEN** `os.Stdout`에 직접 메시지를 출력해야 한다.

**REQ-OBS-005-006** [상태 기반]
**IF** `output`이 `"file"`이면 **THEN** 설정된 `file` 경로의 파일에 메시지를 출력해야 한다. 파일은 Init 시 열고 Shutdown 시 닫는다.

**REQ-OBS-005-007** [상태 기반]
**IF** `output`이 `"editor"`이면 **THEN** slog 로거를 사용하되 `"debug_output": true` 속성을 추가하여 출력해야 한다. 이 속성은 향후 플로우 에디터의 디버그 패널이 WebSocket 스트림에서 필터링하는 데 사용된다.

### 4.3 템플릿 지원

**REQ-OBS-005-008** [이벤트 기반]
**WHEN** `template` 설정이 비어있지 않으면 **THEN** Configure 시점에 Go `text/template`으로 컴파일하고, Process 시 메시지 컨텍스트로 렌더링해야 한다.

**REQ-OBS-005-009** [유비쿼터스]
시스템은 **항상** 템플릿에 다음 컨텍스트를 제공해야 한다:

| 키 | 타입 | 설명 |
|----|------|------|
| `.ID` | string | 메시지 ID |
| `.Payload` | map[string]any | 메시지 페이로드 (fields 필터 적용 후) |
| `.Metadata` | map[string]string | 메시지 메타데이터 (fields 필터 적용 후) |
| `.Timestamp` | string | 현재 시각 (RFC3339) |

**REQ-OBS-005-010** [이벤트 기반]
**WHEN** 템플릿 실행이 실패하면 **THEN** 경고 로그를 출력하고 페이로드 JSON으로 폴백해야 한다.

**REQ-OBS-005-011** [상태 기반]
**IF** `template` 설정이 비어있으면 **THEN** 기본 포맷으로 출력해야 한다:
- `prefix`가 있으면: `[prefix] <payload-json>`
- `prefix`가 없으면: `message id=<ID> payload=<map> metadata=<map>` (기존 debug 노드 포맷)

### 4.4 필드 선택 (Field Selection)

**REQ-OBS-005-012** [상태 기반]
**IF** `fields` 배열이 비어있거나 미설정이면 **THEN** 전체 메시지(payload + metadata)를 출력해야 한다.

**REQ-OBS-005-013** [이벤트 기반]
**WHEN** `fields` 배열에 `"meta:"` 접두사 없는 키가 있으면 **THEN** 해당 키를 payload에서 선택하여 출력해야 한다.

예시: `fields: ["temperature", "power"]` -> payload에서 temperature, power 키만 출력

**REQ-OBS-005-014** [이벤트 기반]
**WHEN** `fields` 배열에 `"meta:"` 접두사가 있는 키가 있으면 **THEN** 접두사를 제거한 키를 metadata에서 선택하여 출력해야 한다.

예시: `fields: ["meta:lgcp_source"]` -> metadata에서 lgcp_source 키만 출력

**REQ-OBS-005-015** [유비쿼터스]
시스템은 **항상** fields 필터가 적용된 결과를 템플릿 컨텍스트에도 반영해야 한다. 즉 `.Payload`와 `.Metadata`는 필터링된 맵이어야 한다.

### 4.5 하위 호환성

**REQ-OBS-005-016** [유비쿼터스]
시스템은 **항상** `registry.go`에서 `"output"` 타입을 `NewDebugNode` 팩토리 함수로 매핑하여, 기존 `output` 타입의 플로우 YAML이 그대로 동작해야 한다.

**REQ-OBS-005-017** [유비쿼터스]
시스템은 **항상** 기존 `debug` 노드 설정(`level`, `file`만 포함)이 변경 없이 동작해야 한다. 새로운 설정 필드(`output`, `template`, `prefix`, `fields`)는 모두 선택적이며 기본값이 기존 동작을 재현한다.

**REQ-OBS-005-018** [유비쿼터스]
시스템은 **항상** 기존 `output` 노드 설정(`prefix`, `template`, `file` 포함)이 `debug` 노드에서 동작해야 한다. `template`이 설정되면 output 노드의 렌더링 동작을 재현한다.

### 4.6 파일 삭제 및 정리

**REQ-OBS-005-019** [유비쿼터스]
시스템은 **항상** `internal/node/output.go`와 `internal/node/output_test.go`를 삭제해야 한다.

### 4.7 메시지 pass-through

**REQ-OBS-005-020** [비허용 동작]
시스템은 입력 메시지를 변경**하지 않아야 한다**. debug 노드는 항상 입력 메시지를 그대로 출력으로 통과시킨다.

---

## 5. 비기능 요구사항

### 5.1 성능

- **NFR-OBS-PERF-001**: 템플릿은 Configure 시점에 한 번 컴파일하고 Process마다 재사용해야 한다. 메시지당 템플릿 재컴파일은 금지한다.

### 5.2 하위 호환성

- **NFR-OBS-CON-001**: 기존 `debug` 노드 설정(`level`, `file`만 포함)이 동일한 동작을 보장해야 한다.
- **NFR-OBS-CON-002**: 기존 `output` 노드 설정(`prefix`, `template`, `file` 포함)이 `debug` 노드에서 동일한 동작을 보장해야 한다.

### 5.3 동시성

- **NFR-OBS-CON-003**: 모든 공유 상태(file, config 필드)는 `sync.RWMutex`로 보호되어야 한다.

---

## 6. 기술 접근 / 아키텍처

### 6.1 파일 구조

```
internal/node/
├── debug.go        # 수정: 통합 DebugNode (DebugNodeConfig, 템플릿, 출력 대상, 필드 선택)
├── debug_test.go   # 수정: 통합 테스트 (모든 출력 모드, 템플릿, 필드 선택, 호환성)
├── output.go       # 삭제
├── output_test.go  # 삭제
├── registry.go     # 수정: output -> NewDebugNode alias
├── errors.go       # 변경 없음
```

### 6.2 확장 DebugNode 구조체

```go
type DebugNode struct {
    *BaseNode
    logLevel   string            // "debug", "info", "warn" (기본: "debug")
    outputDest string            // "logger", "terminal", "file", "editor" (기본: "logger")
    filePath   string            // 파일 경로 (output="file" 시 사용)
    file       *os.File          // 열린 파일 핸들
    tmpl       *template.Template // 컴파일된 템플릿 (nil이면 기본 포맷)
    prefix     string            // 출력 접두사
    fields     []string          // 선택적 출력 필드
    mu         sync.RWMutex
}
```

### 6.3 필드 선택 함수

```go
// filterFields 는 fields 설정에 따라 payload와 metadata를 필터링한다.
// fields가 비어있으면 원본 맵을 그대로 반환한다.
func filterFields(payload map[string]any, metadata map[string]string, fields []string) (map[string]any, map[string]string)
```

### 6.4 템플릿 컨텍스트 구조체

```go
type debugTemplateContext struct {
    ID        string
    Payload   map[string]any
    Metadata  map[string]string
    Timestamp string
}
```

### 6.5 출력 대상별 처리

```go
// writeOutput 은 설정된 출력 대상에 따라 포맷팅된 메시지를 출력한다.
func (n *DebugNode) writeOutput(formatted string)
```

### 6.6 Registry 변경

```go
// 기존
{"debug", NewDebugNode, "debug", "메시지를 디버그 출력"},
{"output", NewOutputNode, "debug", "메시지를 포맷팅하여 출력"},

// 변경 후
{"debug", NewDebugNode, "debug", "메시지를 디버그 출력 (템플릿, 다중 출력 대상 지원)"},
{"output", NewDebugNode, "debug", "메시지를 포맷팅하여 출력 (debug 노드 alias)"},
```

---

## 7. 추적성 (Traceability)

| 요구사항 | 파일 | 테스트 |
|---------|------|--------|
| REQ-OBS-005-001~003 | internal/node/debug.go (Configure) | debug_test.go |
| REQ-OBS-005-004~007 | internal/node/debug.go (writeOutput) | debug_test.go |
| REQ-OBS-005-008~011 | internal/node/debug.go (Process, template) | debug_test.go |
| REQ-OBS-005-012~015 | internal/node/debug.go (filterFields) | debug_test.go |
| REQ-OBS-005-016 | internal/node/registry.go | registry_test.go |
| REQ-OBS-005-017~018 | internal/node/debug.go (Configure) | debug_test.go |
| REQ-OBS-005-019 | 파일 삭제 | 빌드 검증 |
| REQ-OBS-005-020 | internal/node/debug.go (Process) | debug_test.go |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-06*
*작성: MoAI SPEC Builder (manager-spec)*
