---
id: SPEC-FILE-001
version: "1.0.0"
status: completed
created: "2026-04-09"
updated: "2026-04-09"
author: xtra
priority: high
dependencies:
  - SPEC-SYSAGENT-001
  - SPEC-AGENT-001
tags:
  - file-agent
  - bridge-node
  - text-mode
  - system-agent
---

# SPEC-FILE-001: File Agent 기능 확장 (Bridge 통합, Text/Binary 모드, 지정 파일 관리)

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-04-09 | xtra | 최초 작성 |

---

## 1. 환경 (Environment)

### 1.1 현재 상태

File Agent(`internal/agent/system/file.go`)는 SPEC-SYSAGENT-001에 의해 구현된 System Agent이다:

- `FileOperator` 인터페이스: ReadFile, WriteFile, AppendFile, FileExists, RemoveFile, ListDir, WatchDir, UnwatchDir
- 샌드박스 보안 (`resolveSandboxPath`)
- 바이너리 전용 (`[]byte` 기반 I/O)
- fsnotify 기반 디렉토리 감시
- 원자적 통계 추적 (`fileAgentStats`)
- `agent.SystemAgent` 인터페이스 구현 (`IsSystem()`, `RequiresTransport()`)

### 1.2 제약 사항

- File Agent는 현재 직접 API 호출(코드 레벨)로만 접근 가능
- Bridge Node를 통한 플로우 연결 미지원 (Process() 미구현)
- 텍스트 인코딩 지원 없음 (UTF-8 변환, 줄 단위 처리 불가)
- 에이전트 설정에서 대상 파일을 지정하는 기능 없음

### 1.3 관련 시스템

| 컴포넌트 | 파일 | 역할 |
|----------|------|------|
| Agent 인터페이스 | `internal/agent/agent.go` | `Process()`, `MessageReceiver`, `Agent` 인터페이스 정의 |
| Bridge Node | `internal/node/bridge.go` | 에이전트-플로우 연결 (BridgeIn/Out/InOut/RequestReply) |
| Message | `pkg/message/` | Message 인터페이스 (Payload, Metadata) |
| SystemAgent | `internal/agent/system.go` | SystemAgent 마커 인터페이스 |
| AgentRef | `pkg/flow/node.go:44-47` | `{AgentName, Direction}` |
| BridgeDirection | `pkg/flow/node.go:26-40` | BridgeIn, BridgeOut, BridgeInOut, BridgeRequestReply |

---

## 2. 가정 (Assumptions)

- A1: 기존 FileOperator 인터페이스의 하위 호환성을 유지한다
- A2: 기존 FileAgentImpl 구조체를 확장하여 구현한다 (새 파일 분리 가능)
- A3: 텍스트 모드의 기본 인코딩은 UTF-8이다
- A4: Bridge Node 통합 시 기존 Bridge Node 코드를 수정하지 않고, File Agent가 적절한 인터페이스를 구현하여 연동한다
- A5: 지정 파일 경로는 AgentConfig를 통해 설정하며, 샌드박스 검증이 적용된다
- A6: 파일 감시 이벤트는 Bridge Node의 BridgeIn 방향을 통해 플로우로 전달된다

---

## 3. 요구 사항 (Requirements)

### 모듈 1: Text/Binary 모드 설정 및 연산

#### REQ-FILE-001: 파일 모드 설정 (Ubiquitous)

시스템은 **항상** File Agent가 `text` 또는 `binary` 모드를 AgentConfig에서 설정할 수 있어야 한다.

- 모드 미지정 시 기본값은 `binary` (기존 동작 유지)
- AgentConfig.Transport.Options에 `"mode": "text"` 또는 `"mode": "binary"` 키로 설정

#### REQ-FILE-002: 텍스트 모드 인코딩 (Event-Driven)

**WHEN** File Agent의 모드가 `text`로 설정되었을 때, **THEN** 시스템은 파일 읽기/쓰기 시 지정된 인코딩(기본: UTF-8)으로 바이트-문자열 변환을 수행해야 한다.

- `ReadFileText(path) (string, error)` - 파일을 문자열로 읽기
- `WriteFileText(path, content string, perm os.FileMode) error` - 문자열을 파일에 쓰기
- `AppendFileText(path, content string) error` - 문자열을 파일에 추가

#### REQ-FILE-003: 줄 단위 읽기/쓰기 (Event-Driven)

**WHEN** File Agent가 텍스트 모드일 때, **THEN** 시스템은 줄 단위 파일 읽기/쓰기를 지원해야 한다.

- `ReadLines(path string) ([]string, error)` - 모든 줄을 슬라이스로 반환
- `ReadLine(path string, lineNum int) (string, error)` - 특정 줄 번호 읽기 (1-based)
- `WriteLines(path string, lines []string, perm os.FileMode) error` - 줄 슬라이스를 파일에 쓰기
- `AppendLine(path string, line string) error` - 한 줄 추가

#### REQ-FILE-004: 바이너리 모드 명시적 유지 (Ubiquitous)

시스템은 **항상** 바이너리 모드에서 기존 `[]byte` 기반 ReadFile/WriteFile/AppendFile 동작을 변경 없이 유지해야 한다.

### 모듈 2: Bridge Node 통합 (Process 메서드, 메시지 핸들링)

#### REQ-FILE-010: Process 메서드 구현 (Ubiquitous)

시스템은 **항상** File Agent의 `Process(data []byte) ([]byte, error)` 메서드가 JSON 명령 메시지를 수신하여 파일 연산을 실행하고 결과를 JSON으로 반환해야 한다.

명령 메시지 포맷:
```json
{
  "command": "read_file" | "write_file" | "append_file" | "file_exists" | "remove_file" | "list_dir" | "read_lines" | "read_line" | "write_lines" | "append_line",
  "params": {
    "path": "relative/path",
    "data": "base64 또는 text 내용",
    "perm": 0644,
    "line_num": 1,
    "lines": ["line1", "line2"]
  }
}
```

응답 포맷:
```json
{
  "success": true,
  "data": "base64 또는 text 결과",
  "error": ""
}
```

#### REQ-FILE-011: MessageReceiver 인터페이스 구현 (Event-Driven)

**WHEN** Bridge Node가 BridgeIn 방향으로 File Agent에 연결될 때, **THEN** File Agent는 `MessageReceiver` 인터페이스를 구현하여 파일 감시 이벤트를 비동기 메시지로 수신 루프에 전달해야 한다.

- `ReceiveMessage(ctx context.Context) ([]byte, error)` 구현
- WatchDir 이벤트를 내부 채널로 전달
- Bridge Node의 수신 루프가 이 채널에서 이벤트를 소비

#### REQ-FILE-012: BridgeAdapter 등록 (Event-Driven)

**WHEN** Bridge Node가 File Agent를 해석할 때, **THEN** File Agent 전용 BridgeAdapter가 등록되어 플로우 메시지와 파일 명령 간의 변환을 수행해야 한다.

- `TransformToAgent(msg message.Message) ([]byte, AgentMeta, error)` - 플로우 메시지를 JSON 명령으로 변환
- `TransformFromAgent(data []byte) (message.Message, error)` - JSON 응답을 플로우 메시지로 변환

#### REQ-FILE-013: Bridge 방향 지원 (State-Driven)

**IF** Bridge Node의 방향이 `BridgeIn`이면 **THEN** File Agent는 파일 감시 이벤트만 플로우로 전달한다.
**IF** Bridge Node의 방향이 `BridgeOut`이면 **THEN** File Agent는 플로우에서 받은 명령을 실행한다.
**IF** Bridge Node의 방향이 `BridgeInOut`이면 **THEN** File Agent는 명령 실행과 이벤트 전달을 모두 수행한다.
**IF** Bridge Node의 방향이 `BridgeRequestReply`이면 **THEN** File Agent는 명령 실행 후 결과를 동기적으로 응답한다.

### 모듈 3: 지정 파일 관리 (설정 기반 파일 타겟)

#### REQ-FILE-020: 설정 기반 대상 파일 지정 (Event-Driven)

**WHEN** AgentConfig에 `target_files` 옵션이 설정될 때, **THEN** 시스템은 해당 파일 목록을 Agent에 등록하고 명령 실행 시 경로 없이 별칭으로 접근할 수 있게 해야 한다.

설정 예:
```json
{
  "target_files": {
    "config": "etc/app.conf",
    "log": "var/log/app.log",
    "data": "data/sensor.csv"
  }
}
```

- 별칭 기반 접근: `{"command": "read_file", "params": {"alias": "config"}}`
- 경로 직접 지정도 여전히 가능 (하위 호환)

#### REQ-FILE-021: 대상 파일 자동 감시 (Event-Driven)

**WHEN** `target_files`가 설정되고 Agent가 초기화될 때, **THEN** 시스템은 대상 파일이 위치한 디렉토리들을 자동으로 WatchDir에 등록해야 한다.

- 중복 디렉토리 감시 방지
- Agent Stop 시 자동 해제

#### REQ-FILE-022: 대상 파일 유효성 검증 (Unwanted)

시스템은 `target_files`에 등록된 경로가 샌드박스 외부를 가리키는 것을 **허용하지 않아야 한다**.

- Init 시점에 모든 target_files 경로에 대해 `resolveSandboxPath` 검증
- 검증 실패 시 Init 에러 반환

### 모듈 4: 파일 이벤트 알림 via Bridge

#### REQ-FILE-030: 이벤트 메시지 포맷 (Ubiquitous)

시스템은 **항상** 파일 감시 이벤트를 다음 포맷의 JSON 메시지로 변환하여 Bridge에 전달해야 한다.

```json
{
  "event": "create" | "write" | "remove" | "rename" | "chmod",
  "path": "relative/path/to/file",
  "alias": "config",
  "timestamp": "2026-04-09T10:00:00Z"
}
```

- `alias` 필드는 target_files에 등록된 파일인 경우에만 포함
- 경로는 샌드박스 루트 기준 상대 경로

#### REQ-FILE-031: 이벤트 필터링 (Optional)

**가능하면** AgentConfig에서 감시할 이벤트 유형을 필터링할 수 있도록 제공한다.

- `"watch_events": ["create", "write", "remove"]`
- 미설정 시 모든 이벤트 전달 (기본 동작)

### 모듈 5: 에러 타입

#### REQ-FILE-040: 확장된 에러 타입 (Ubiquitous)

시스템은 **항상** 다음의 추가 센티넬 에러를 제공해야 한다.

| 에러 | 설명 |
|------|------|
| `ErrUnsupportedCommand` | 알 수 없는 Process 명령 |
| `ErrAliasNotFound` | target_files에 등록되지 않은 별칭 |
| `ErrInvalidCommandFormat` | JSON 명령 파싱 실패 |
| `ErrTextModeRequired` | 텍스트 전용 연산을 바이너리 모드에서 호출 |
| `ErrLineOutOfRange` | 줄 번호가 파일 범위를 초과 |

#### REQ-FILE-041: 에러 응답 포맷 (Event-Driven)

**WHEN** Process 명령 실행 중 에러가 발생할 때, **THEN** 시스템은 에러를 JSON 응답으로 래핑하여 반환해야 한다.

```json
{
  "success": false,
  "data": null,
  "error": "system/file: alias not found: config"
}
```

---

## 4. 사양 (Specifications)

### 4.1 인터페이스 확장

```go
// TextFileOperator 는 텍스트 모드 파일 연산 인터페이스이다.
type TextFileOperator interface {
    ReadFileText(path string) (string, error)
    WriteFileText(path string, content string, perm os.FileMode) error
    AppendFileText(path string, content string) error
    ReadLines(path string) ([]string, error)
    ReadLine(path string, lineNum int) (string, error)
    WriteLines(path string, lines []string, perm os.FileMode) error
    AppendLine(path string, line string) error
}
```

### 4.2 설정 구조

AgentConfig.Transport.Options 확장 키:

| 키 | 타입 | 기본값 | 설명 |
|----|------|--------|------|
| `mode` | string | `"binary"` | 파일 모드 (`text` / `binary`) |
| `encoding` | string | `"utf-8"` | 텍스트 인코딩 (text 모드 전용) |
| `target_files` | map[string]string | nil | 별칭-경로 매핑 |
| `watch_events` | []string | nil (모든 이벤트) | 감시할 이벤트 유형 |
| `event_buffer_size` | int | 256 | Bridge 이벤트 버퍼 크기 |

### 4.3 파일 구조

| 파일 | 용도 |
|------|------|
| `internal/agent/system/file.go` | 기존 FileAgentImpl 수정 (Process, MessageReceiver 구현) |
| `internal/agent/system/file_text.go` | TextFileOperator 구현 (신규) |
| `internal/agent/system/file_bridge.go` | BridgeAdapter, 명령 핸들링 (신규) |
| `internal/agent/system/file_config.go` | target_files 관리, 설정 파싱 (신규) |
| `internal/agent/system/file_errors.go` | 확장 에러 추가 |
| `internal/node/bridge_adapter_file.go` | File Agent 전용 BridgeAdapter 등록 (신규) |

### 4.4 추적성 (Traceability)

| 요구 사항 | 모듈 | 우선순위 |
|----------|------|---------|
| REQ-FILE-001~004 | Module 1: Text/Binary 모드 | High |
| REQ-FILE-010~013 | Module 2: Bridge 통합 | High |
| REQ-FILE-020~022 | Module 3: 지정 파일 관리 | Medium |
| REQ-FILE-030~031 | Module 4: 이벤트 알림 | Medium |
| REQ-FILE-040~041 | Module 5: 에러 타입 | High |
