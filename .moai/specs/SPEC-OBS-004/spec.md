---
id: SPEC-OBS-004
version: "1.0.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-08 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-OBS-004: 로그 출력 요소 식별 강화 (Log Source Identification Enhancement)

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 엔진의 로그 출력 시 어떤 에이전트, 플로우, 노드에서 생성된 로그인지 식별할 수 없는 문제를 해결한다. 현재 `Observer` 시스템은 `ComponentLogger`를 통해 `"component"` 속성을 모든 로그에 자동 포함하지만, WebSocket 로그 스트리밍(`wsLogWriter`)이 `level`, `message`, `timestamp`만 전송하여 `component` 정보가 완전히 유실된다. 또한 노드 컴포넌트 이름에 플로우 컨텍스트가 없어 다중 플로우 환경에서 동명 노드를 구분할 수 없으며, API 레이어는 `slog.Default()`를 직접 사용하여 Observer 통합이 이루어지지 않고 있다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/observe/`, `internal/api/ws/`, `internal/api/`, `internal/engine/`
- **의존성**: `log/slog` (표준 라이브러리)
- **테스트 프레임워크**: Go 표준 `testing` 패키지
- **관련 SPEC**: SPEC-OBS-001 (구현 완료 - 핵심 Observer 시스템), SPEC-OBS-002 (planned), SPEC-OBS-003 (planned)
- **관련 구현**: SPEC-WEB-003 (completed - 에이전트 Observer 통합)

### 1.3 현재 아키텍처

#### Observer 시스템
- `Observer` (`internal/observe/observe.go`): LoggerFactory, LevelManager, StreamRouter, MetricsCollector를 통합하는 중앙 관찰성 시스템
- `ComponentLogger` (`internal/observe/logger.go`): `"component"` 속성을 자동 포함하는 구조화된 로거 생성
- `StreamRouter` (`internal/observe/stream.go`): 컴포넌트 이름 기반으로 로그를 다른 `io.Writer`로 라우팅

#### 현재 컴포넌트 명명 패턴
- **Agent**: `agent.{type}.{name}` (예: `agent.modbus.device-reader`) - `internal/agent/manager.go:79`에서 설정
- **Node**: `node.{name}` (예: `node.filter-input`) - `internal/engine/engine.go:92`에서 설정
- **Engine**: `xflowd` - `cmd/xflowd/main.go:140`에서 설정
- **Flow**: 컴포넌트 패턴 없음 - 엔진 로그 호출 시 flowID, flowName 속성으로만 추가
- **API Layer**: `slog.Default()` 직접 사용 - Observer 통합 없음

#### WebSocket Log Writer (`internal/api/ws/log_writer.go`)
현재 추출 및 전송하는 필드:
```go
payload := map[string]string{
    "level":     levelStr,
    "message":   msg,
    "timestamp": ts,
}
```
JSON 로그 레코드의 `component` 필드가 추출되지도, 전송되지도 않는다.

### 1.4 식별된 격차

1. WebSocket 로그 스트리밍이 컴포넌트 식별 정보를 완전히 유실함
2. 노드 로그에 플로우 컨텍스트가 없어 다중 플로우 간 동명 노드 구분 불가
3. API 레이어 (8개 이상 파일)가 `slog.Default()`를 사용하여 Observer 미통합
4. 브릿지 노드가 로그에 에이전트 참조를 포함하지 않음

### 1.5 설계 원칙

- **하위 호환성**: 기존 로그 레벨 설정 및 라우팅 동작을 유지
- **점진적 개선**: 3개 모듈을 독립적으로 구현 가능한 구조
- **최소 변경**: 기존 인터페이스를 확장하되 변경하지 않음
- **Web UI 친화**: 프론트엔드에서 컴포넌트별 필터링이 가능하도록 구조화

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: slog JSON Handler는 `"component"` 키를 JSON 로그 출력의 최상위 필드로 포함한다
- A2: WebSocket 클라이언트(Web UI)는 `log.entry` 메시지 포맷 변경에 유연하게 대응할 수 있다
- A3: `slog.Default()`를 `Observer` 기반 로거로 교체해도 API 동작에 영향이 없다
- A4: 노드 컴포넌트 이름 변경은 로그 레벨 와일드카드 패턴(`node.*`)에 영향을 줄 수 있으며, `flow.*.node.*` 패턴을 추가 지원해야 한다
- A5: API 서버는 `Server` 구조체에 `*slog.Logger`를 이미 보유하고 있어 Observer 주입이 가능하다

### 2.2 도메인 가정

- A6: 이 SPEC의 변경은 기존 SPEC-OBS-001의 인터페이스를 수정하지 않는다
- A7: Web UI 프론트엔드의 로그 표시 컴포넌트 업데이트는 이 SPEC의 범위에 포함된다 (Module 1)
- A8: 노드 컴포넌트 이름 변경 시 기존 `node.*` 와일드카드 패턴도 계속 동작해야 한다
- A9: API 핸들러별 컴포넌트 로거 명명은 `api.handler.{type}` 패턴을 따른다
- A10: 각 모듈은 독립적으로 구현 및 테스트 가능하며, 모듈 간 직접적 의존성은 없다

---

## 3. Requirements (요구사항)

### Module 1: WebSocket 로그에 component 필드 추가 - P0

#### REQ-OBS-004-01-01 (Event-Driven) component 필드 추출

**WHEN** `wsLogWriter.Write()`가 JSON 로그 라인을 수신하면, **THEN** `"component"` 키의 값을 추출하여 WebSocket 페이로드에 포함해야 한다.

- JSON 로그 레코드에서 `logLine["component"]` 값을 문자열로 추출한다
- `component` 키가 없거나 빈 문자열인 경우 `"unknown"`을 기본값으로 사용한다

#### REQ-OBS-004-01-02 (Event-Driven) source 필드 분류

**WHEN** `wsLogWriter.Write()`가 `component` 값을 추출하면, **THEN** 컴포넌트 이름의 접두사를 기반으로 `source` 분류를 생성해야 한다.

- `agent.*` -> `source: "agent"`
- `node.*` 또는 `flow.*.node.*` -> `source: "node"`
- `flow.*` (node 제외) -> `source: "flow"`
- `api.*` -> `source: "api"`
- `xflowd` 또는 `engine.*` -> `source: "engine"`
- 그 외 -> `source: "system"`

#### REQ-OBS-004-01-03 (Ubiquitous) WebSocket 페이로드 확장

시스템은 **항상** `log.entry` WebSocket 메시지의 페이로드에 다음 필드를 포함해야 한다:

```go
payload := map[string]string{
    "level":     levelStr,
    "message":   msg,
    "timestamp": ts,
    "component": component,  // NEW: 컴포넌트 이름
    "source":    source,     // NEW: 출력 요소 분류
}
```

#### REQ-OBS-004-01-04 (Unwanted) 기존 WebSocket 동작 파괴 금지

시스템은 기존 `level`, `message`, `timestamp` 필드의 전송 동작을 **변경하지 않아야 한다**. 새로운 필드는 추가만 한다.

#### REQ-OBS-004-01-05 (Optional) Web UI 로그 컴포넌트 표시

**가능하면** Web UI의 로그 표시 컴포넌트가 `component`와 `source` 정보를 시각적으로 표현하는 기능을 제공한다.

- source별 색상 구분 또는 아이콘 표시
- component 기반 필터링 기능

---

### Module 2: Flow 컨텍스트를 노드 컴포넌트 이름에 포함 - P0

#### REQ-OBS-004-02-01 (Event-Driven) 노드 컴포넌트 이름 변경

**WHEN** `DeployFlow()`에서 노드 로거를 생성할 때, **THEN** 컴포넌트 이름을 `flow.{flowName}.node.{nodeName}` 형식으로 설정해야 한다.

- 기존: `node.{name}` (예: `node.filter-input`)
- 변경: `flow.{flowName}.node.{nodeName}` (예: `flow.mqtt-metrics.node.filter-input`)
- `flowName`은 `f.Name()` 또는 `f.ID()`에서 가져온다

#### REQ-OBS-004-02-02 (Ubiquitous) 플로우 이름 정규화

시스템은 **항상** 컴포넌트 이름에 사용되는 플로우 이름을 dot-notation 안전하게 정규화해야 한다.

- 공백, 특수문자를 하이픈(`-`)으로 교체
- 연속 하이픈을 단일 하이픈으로 축소
- 소문자 변환

#### REQ-OBS-004-02-03 (Ubiquitous) 로그 레벨 와일드카드 호환성

시스템은 **항상** 다음 와일드카드 패턴이 정상 동작해야 한다:

| 패턴 | 매칭 대상 |
|------|----------|
| `flow.*` | 모든 플로우 관련 컴포넌트 |
| `flow.mqtt-metrics.*` | 특정 플로우의 모든 노드 |
| `flow.*.node.*` | 모든 플로우의 모든 노드 |
| `flow.mqtt-metrics.node.filter-input` | 특정 플로우의 특정 노드 |

#### REQ-OBS-004-02-04 (State-Driven) 플로우 이름 미정 시 폴백

**IF** 플로우 이름이 빈 문자열이거나 사용할 수 없는 경우, **THEN** 플로우 ID를 대신 사용하며, ID도 없는 경우 `"unnamed"` 을 사용해야 한다.

#### REQ-OBS-004-02-05 (Unwanted) 기존 노드 동작 영향 금지

시스템은 노드 컴포넌트 이름 변경으로 인해 노드의 메시지 처리, 와이어 연결, 에러 핸들링 동작이 **영향받지 않아야 한다**. 변경은 로깅 레이어에만 국한된다.

---

### Module 3: API 레이어 Observer 통합 - P1

#### REQ-OBS-004-03-01 (Ubiquitous) API Server Observer 주입

시스템은 **항상** API 서버 초기화 시 `Observer`를 주입받아 컴포넌트 로거를 생성할 수 있어야 한다.

- `Server` 구조체에 `observer *observe.Observer` 필드를 추가하거나, 기존 `logger` 필드를 Observer 기반 로거로 교체한다
- `WithObserver(obs *observe.Observer) ServerOption` 옵션 함수를 제공한다
- `cmd/xflowd/main.go`에서 Observer를 API 서버에 전달한다

#### REQ-OBS-004-03-02 (Ubiquitous) 핸들러별 컴포넌트 로거 생성

시스템은 **항상** 각 핸들러 타입에 대해 개별 컴포넌트 로거를 생성해야 한다.

| 핸들러 | 컴포넌트 이름 |
|--------|--------------|
| FlowHandler | `api.handler.flow` |
| AgentHandler | `api.handler.agent` |
| NodeHandler | `api.handler.node` |
| MonitorHandler | `api.handler.monitor` |
| WebSocketHandler | `api.handler.websocket` |
| Router/Middleware | `api.router` |
| Hub | `api.ws.hub` |
| EventPublisher | `api.ws.event` |

#### REQ-OBS-004-03-03 (Event-Driven) slog.Default() 교체

**WHEN** Observer가 주입된 경우, **THEN** API 레이어의 모든 `slog.Default()`, `slog.Info()`, `slog.Warn()`, `slog.Error()`, `slog.Debug()` 호출을 해당 컴포넌트 로거로 교체해야 한다.

- `internal/api/server.go`
- `internal/api/router.go`
- `internal/api/handler/*.go` (flow.go, agent.go, node.go, monitor.go, websocket.go)
- `internal/api/ws/*.go` (hub.go, client.go, event_publisher.go, broadcaster.go)
- `internal/api/service/*.go` (flow_adapter.go, agent_adapter.go, node_adapter.go)

#### REQ-OBS-004-03-04 (State-Driven) Observer 미주입 시 폴백

**IF** Observer가 주입되지 않은 경우, **THEN** 기존 `slog.Default()` 기반 동작을 유지해야 한다. Observer 주입은 선택적이다.

#### REQ-OBS-004-03-05 (Unwanted) API 기능 퇴행 금지

시스템은 Observer 통합으로 인해 기존 API 엔드포인트의 요청/응답 동작, 에러 핸들링, WebSocket 연결 동작이 **변경되지 않아야 한다**.

---

## 4. Specifications (사양)

### 4.1 Module 1 변경 대상 파일

```
internal/api/ws/log_writer.go     # component, source 필드 추출 및 페이로드 확장
internal/api/ws/log_writer_test.go # 확장 페이로드 검증 테스트
web/src/                          # (Optional) 로그 표시 컴포넌트 업데이트
```

### 4.2 Module 2 변경 대상 파일

```
internal/engine/engine.go          # DeployFlow()의 노드 컴포넌트 이름 변경
internal/engine/engine_test.go     # 변경된 컴포넌트 이름 검증
```

### 4.3 Module 3 변경 대상 파일

```
internal/api/server.go             # Observer 주입, 컴포넌트 로거 생성
internal/api/router.go             # 라우터 컴포넌트 로거 사용
internal/api/handler/flow.go       # 컴포넌트 로거 사용
internal/api/handler/agent.go      # 컴포넌트 로거 사용
internal/api/handler/node.go       # 컴포넌트 로거 사용
internal/api/handler/monitor.go    # 컴포넌트 로거 사용
internal/api/handler/websocket.go  # 컴포넌트 로거 사용
internal/api/ws/hub.go             # 컴포넌트 로거 사용
internal/api/ws/client.go          # 컴포넌트 로거 사용
internal/api/ws/event_publisher.go # 컴포넌트 로거 사용
internal/api/ws/broadcaster.go     # 컴포넌트 로거 사용
internal/api/service/*.go          # 컴포넌트 로거 사용
cmd/xflowd/main.go                # Observer를 API 서버에 전달
```

### 4.4 source 분류 함수

```go
// classifySource 는 컴포넌트 이름에서 출력 요소 분류를 반환한다.
func classifySource(component string) string {
    switch {
    case strings.HasPrefix(component, "agent."):
        return "agent"
    case strings.HasPrefix(component, "flow.") && strings.Contains(component, ".node."):
        return "node"
    case strings.HasPrefix(component, "node."):
        return "node"
    case strings.HasPrefix(component, "flow."):
        return "flow"
    case strings.HasPrefix(component, "api."):
        return "api"
    case component == "xflowd" || strings.HasPrefix(component, "engine."):
        return "engine"
    default:
        return "system"
    }
}
```

### 4.5 플로우 이름 정규화 함수

```go
// sanitizeFlowName 은 플로우 이름을 컴포넌트 이름에 안전한 형식으로 변환한다.
func sanitizeFlowName(name string) string {
    // 영숫자와 하이픈만 허용, 나머지는 하이픈으로 치환
    // 연속 하이픈 축소, 소문자 변환
}
```

### 4.6 컴포넌트 명명 규칙 (확장)

| 구성 요소 | 기존 패턴 | 변경 후 패턴 |
|-----------|----------|-------------|
| Engine | `xflowd` | `xflowd` (변경 없음) |
| Agent | `agent.{type}.{name}` | `agent.{type}.{name}` (변경 없음) |
| Node (M2) | `node.{name}` | `flow.{flowName}.node.{nodeName}` |
| API Server (M3) | (없음) | `api.handler.{type}`, `api.router`, `api.ws.{type}` |
| WebSocket (M3) | (없음) | `api.ws.hub`, `api.ws.event` |

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-OBS-004-01-01 | M1: WS component | Event-Driven | 단위 테스트 (component 추출 검증) |
| REQ-OBS-004-01-02 | M1: WS source | Event-Driven | 단위 테스트 (source 분류 검증) |
| REQ-OBS-004-01-03 | M1: WS payload | Ubiquitous | 단위 테스트 (페이로드 필드 검증) |
| REQ-OBS-004-01-04 | M1: WS 호환성 | Unwanted | 기존 테스트 유지 + 회귀 테스트 |
| REQ-OBS-004-01-05 | M1: Web UI | Optional | 수동 검증 (UI 확인) |
| REQ-OBS-004-02-01 | M2: 노드 이름 | Event-Driven | 단위 테스트 (컴포넌트 이름 형식 검증) |
| REQ-OBS-004-02-02 | M2: 정규화 | Ubiquitous | 단위 테스트 (정규화 함수 검증) |
| REQ-OBS-004-02-03 | M2: 와일드카드 | Ubiquitous | 단위 테스트 (패턴 매칭 검증) |
| REQ-OBS-004-02-04 | M2: 폴백 | State-Driven | 단위 테스트 (빈 이름 처리 검증) |
| REQ-OBS-004-02-05 | M2: 무영향 | Unwanted | 기존 엔진 테스트 + `go test -race` |
| REQ-OBS-004-03-01 | M3: Observer 주입 | Ubiquitous | 단위 테스트 (Server 초기화 검증) |
| REQ-OBS-004-03-02 | M3: 핸들러 로거 | Ubiquitous | 단위 테스트 (컴포넌트 로거 생성 검증) |
| REQ-OBS-004-03-03 | M3: slog 교체 | Event-Driven | 통합 테스트 (로그 출력 컴포넌트 확인) |
| REQ-OBS-004-03-04 | M3: 폴백 | State-Driven | 단위 테스트 (Observer 없이 동작 검증) |
| REQ-OBS-004-03-05 | M3: 무퇴행 | Unwanted | 기존 API 테스트 유지 + 회귀 테스트 |

---

## Implementation Notes

### 구현 완료: 2026-03-08

**커밋**: `c651b59` (branch: claude)

### 변경 파일 요약

| 모듈 | 파일 | 변경 유형 |
|------|------|----------|
| M1 | `internal/api/ws/log_writer.go` | classifySource() 추가, Write() 페이로드 확장 |
| M1 | `internal/api/ws/log_writer_test.go` | 3개 테스트 함수 추가 |
| M2 | `internal/engine/engine.go` | sanitizeFlowName(), resolveFlowComponentPrefix() 추가 |
| M2 | `internal/engine/engine_test.go` | TestSanitizeFlowName 추가, 기존 테스트 업데이트 |
| M2 | `internal/observe/level.go` | matchGlob() 중간 와일드카드 지원 추가 |
| M2 | `internal/observe/level_test.go` | 와일드카드 패턴 테스트 추가 |
| M3 | `internal/api/server.go` | observer 필드, WithObserver() 옵션 추가 |
| M3 | `cmd/xflowd/main.go` | 핸들러/서비스별 컴포넌트 로거 12개 생성 |

### 계획 대비 변경 사항

- **M2 scope expansion**: 계획에 없던 `matchGlob()` 함수 추가. `SetLevelByPattern()`이 trailing `*`만 지원하여 `flow.*.node.*` 패턴 매칭 불가 문제를 해결하기 위해 중간 와일드카드 지원 추가.
- **M3 scope reduction**: 계획에서 15개 파일 수정 예상 → 실제 2개 파일(`server.go`, `main.go`)만 수정. 핸들러/서비스 파일들은 이미 생성자에서 `*slog.Logger`를 주입받는 구조여서 `main.go`에서 컴포넌트별 로거를 생성하여 전달하는 것만으로 충분.

### 품질 검증 결과

- 테스트: 221+ 건 전체 통과
- Race 검출: 0건
- go vet: 클린
- TRUST 5: 전 차원 PASS
