---
id: SPEC-OBS-004
version: "1.0.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
priority: high
type: acceptance
---

# SPEC-OBS-004 수락 기준: 로그 출력 요소 식별 강화

---

## Module 1: WebSocket 로그에 component 필드 추가

### AC-01-01: component 필드 추출 (REQ-OBS-004-01-01)

**Given** Observer 시스템이 `"component": "agent.modbus.reader"` 속성을 포함한 JSON 로그를 생성하고
**When** `wsLogWriter.Write()`가 해당 JSON 로그 바이트를 수신하면
**Then** WebSocket 페이로드의 `"component"` 필드에 `"agent.modbus.reader"` 값이 포함되어야 한다

**Given** JSON 로그 레코드에 `"component"` 키가 없고
**When** `wsLogWriter.Write()`가 해당 로그를 수신하면
**Then** WebSocket 페이로드의 `"component"` 필드에 `"unknown"` 기본값이 설정되어야 한다

### AC-01-02: source 분류 (REQ-OBS-004-01-02)

**Given** 다양한 컴포넌트 이름이 주어질 때
**When** source 분류가 수행되면
**Then** 다음과 같이 분류되어야 한다:

| component | source |
|-----------|--------|
| `agent.modbus.reader` | `agent` |
| `agent.mqtt.broker-1` | `agent` |
| `flow.mqtt-flow.node.filter-input` | `node` |
| `node.transform-1` | `node` |
| `flow.data-pipeline` | `flow` |
| `api.handler.flow` | `api` |
| `api.ws.hub` | `api` |
| `xflowd` | `engine` |
| `engine.scheduler` | `engine` |
| `plugin.custom` | `system` |
| `unknown` | `system` |
| `""` | `system` |

### AC-01-03: 확장된 WebSocket 페이로드 (REQ-OBS-004-01-03)

**Given** `"component": "api.handler.flow"` 속성을 포함한 INFO 레벨 JSON 로그가 있고
**When** `wsLogWriter.Write()`가 처리하여 WebSocket으로 브로드캐스트하면
**Then** `TypeLogEntry` 메시지의 페이로드에 다음 5개 필드가 모두 포함되어야 한다:
- `"level"`: `"INFO"`
- `"message"`: 로그 메시지 문자열
- `"timestamp"`: 타임스탬프 문자열
- `"component"`: `"api.handler.flow"`
- `"source"`: `"api"`

### AC-01-04: 기존 동작 호환성 (REQ-OBS-004-01-04)

**Given** 기존 `log_writer_test.go`의 모든 테스트가 존재하고
**When** 변경 후 테스트를 실행하면
**Then** `level`, `message`, `timestamp` 관련 기존 검증이 모두 통과해야 한다

### AC-01-T1: 단위 테스트 - classifySource

**Given** `classifySource` 함수가 구현되었을 때
**When** 위 AC-01-02의 모든 입력 케이스에 대해 테이블 기반 테스트를 실행하면
**Then** 모든 케이스가 기대값과 일치해야 한다

### AC-01-T2: 단위 테스트 - component 미존재 케이스

**Given** `{"level":"INFO","msg":"test","time":"2026-03-08T00:00:00Z"}` 형태의 component 없는 JSON 로그가 있고
**When** `wsLogWriter.Write()`가 처리하면
**Then** 페이로드의 `"component"`가 `"unknown"`, `"source"`가 `"system"` 이어야 한다

---

## Module 2: Flow 컨텍스트를 노드 컴포넌트 이름에 포함

### AC-02-01: 노드 컴포넌트 이름 형식 (REQ-OBS-004-02-01)

**Given** "mqtt-metrics" 이름의 플로우에 "filter-input" 이름의 노드가 있고
**When** `DeployFlow()`가 해당 플로우를 배포하며 노드 로거를 생성하면
**Then** 노드의 컴포넌트 이름이 `"flow.mqtt-metrics.node.filter-input"` 이어야 한다

### AC-02-02: 플로우 이름 정규화 (REQ-OBS-004-02-02)

**Given** 다양한 플로우 이름이 주어질 때
**When** `sanitizeFlowName()` 함수가 처리하면
**Then** 다음과 같이 정규화되어야 한다:

| 입력 | 출력 |
|------|------|
| `"MQTT Metrics"` | `"mqtt-metrics"` |
| `"data_pipeline_v2"` | `"data-pipeline-v2"` |
| `"test--flow"` | `"test-flow"` |
| `"normal-flow"` | `"normal-flow"` |
| `"Flow #1 (test)"` | `"flow-1-test"` |
| `""` | `""` |

### AC-02-03: 와일드카드 패턴 호환성 (REQ-OBS-004-02-03)

**Given** 플로우 "mqtt-metrics"에 "filter-input"과 "transform-data" 노드가 배포되어 있고, 플로우 "http-api"에 "filter-input" 노드가 배포되어 있을 때
**When** `SetLevelByPattern()` 으로 레벨을 설정하면
**Then** 다음 패턴이 올바르게 매칭되어야 한다:

| 패턴 | 매칭 컴포넌트 |
|------|-------------|
| `flow.*` | `flow.mqtt-metrics.node.filter-input`, `flow.mqtt-metrics.node.transform-data`, `flow.http-api.node.filter-input` |
| `flow.mqtt-metrics.*` | `flow.mqtt-metrics.node.filter-input`, `flow.mqtt-metrics.node.transform-data` |
| `flow.*.node.filter-input` | `flow.mqtt-metrics.node.filter-input`, `flow.http-api.node.filter-input` |

### AC-02-04: 플로우 이름 폴백 (REQ-OBS-004-02-04)

**Given** 플로우 이름이 빈 문자열이고 ID가 "flow-abc123" 인 경우
**When** `DeployFlow()`가 노드 로거를 생성하면
**Then** 컴포넌트 이름이 `"flow.flow-abc123.node.{nodeName}"` 이어야 한다

**Given** 플로우 이름과 ID가 모두 빈 문자열인 경우
**When** `DeployFlow()`가 노드 로거를 생성하면
**Then** 컴포넌트 이름이 `"flow.unnamed.node.{nodeName}"` 이어야 한다

### AC-02-05: 노드 기능 무영향 (REQ-OBS-004-02-05)

**Given** 컴포넌트 이름이 변경된 노드가 배포되어 있고
**When** 플로우가 실행되어 메시지가 노드를 통과하면
**Then** 메시지 처리, 와이어 전달, 에러 핸들링이 기존과 동일하게 동작해야 한다

### AC-02-T1: 단위 테스트 - sanitizeFlowName

**Given** `sanitizeFlowName` 함수가 구현되었을 때
**When** AC-02-02의 모든 입력 케이스에 대해 테이블 기반 테스트를 실행하면
**Then** 모든 케이스가 기대값과 일치해야 한다

### AC-02-T2: 회귀 테스트 - 엔진 테스트 통과

**Given** 컴포넌트 이름 변경이 적용된 후
**When** `go test ./internal/engine/...` 를 실행하면
**Then** 기존 모든 테스트가 통과해야 한다

### AC-02-T3: 동시성 안전 테스트

**Given** 컴포넌트 이름 변경이 적용된 후
**When** `go test -race ./internal/engine/...` 를 실행하면
**Then** 데이터 레이스가 검출되지 않아야 한다

---

## Module 3: API 레이어 Observer 통합

### AC-03-01: Observer 주입 (REQ-OBS-004-03-01)

**Given** Observer 인스턴스가 생성되어 있고
**When** `api.NewServer(cfg, api.WithObserver(obs))` 로 서버를 생성하면
**Then** 서버 내부의 logger가 Observer 기반 컴포넌트 로거(`api.server`)로 설정되어야 한다

### AC-03-02: 핸들러별 컴포넌트 로거 (REQ-OBS-004-03-02)

**Given** Observer가 주입된 API 서버가 생성되었을 때
**When** 핸들러들이 초기화되면
**Then** Observer에 다음 컴포넌트가 등록되어야 한다:
- `api.handler.flow`
- `api.handler.agent`
- `api.handler.node`
- `api.handler.monitor`
- `api.handler.websocket`
- `api.router`
- `api.ws.hub`
- `api.ws.event`

### AC-03-03: slog.Default() 완전 교체 (REQ-OBS-004-03-03)

**Given** Observer가 주입된 API 서버가 동작 중이고
**When** API 요청을 처리하여 로그가 발생하면
**Then** 해당 로그에 `"component"` 속성이 포함되어야 하며, `slog.Default()`가 아닌 컴포넌트 로거를 통해 출력되어야 한다

**Given** 변경 적용 후
**When** `internal/api/` 디렉토리 전체를 `slog.Default()` 또는 `slog.Info()` 등으로 검색하면
**Then** 직접 호출이 존재하지 않아야 한다 (Observer 폴백 코드 제외)

### AC-03-04: Observer 미주입 폴백 (REQ-OBS-004-03-04)

**Given** Observer 없이 `api.NewServer(cfg)` 로 서버를 생성했을 때
**When** API 요청을 처리하면
**Then** 기존 `slog.Default()` 기반으로 정상 동작하며, panic이나 nil pointer 에러가 발생하지 않아야 한다

### AC-03-05: API 기능 무퇴행 (REQ-OBS-004-03-05)

**Given** Observer 통합이 적용된 API 서버가 동작 중이고
**When** 모든 API 엔드포인트에 대해 기존 테스트를 실행하면
**Then** 요청/응답 동작, 에러 핸들링, WebSocket 연결이 기존과 동일하게 동작해야 한다

### AC-03-T1: 단위 테스트 - WithObserver 옵션

**Given** `WithObserver()` 옵션 함수가 구현되었을 때
**When** Observer를 주입하여 서버를 생성하면
**Then** 서버의 logger가 Observer 기반이어야 하고, Observer 없이 생성 시 기본 logger를 사용해야 한다

### AC-03-T2: 회귀 테스트 - API 전체 테스트 통과

**Given** Module 3 변경이 모두 적용된 후
**When** `go test ./internal/api/...` 를 실행하면
**Then** 모든 기존 테스트가 통과해야 한다

### AC-03-T3: 회귀 테스트 - main 통합

**Given** `cmd/xflowd/main.go`에서 Observer를 API 서버에 전달하도록 변경한 후
**When** xflowd를 빌드하면
**Then** 컴파일 에러 없이 정상 빌드되어야 한다

---

## Quality Gate 체크리스트

### 코드 품질

- [ ] 모든 새 함수에 GoDoc 주석 작성 (Korean)
- [ ] `go vet ./...` 경고 없음
- [ ] `go test -race ./...` 데이터 레이스 없음
- [ ] 기존 테스트 전체 통과 (`go test ./...`)

### 테스트 커버리지

- [ ] Module 1: `classifySource` 100% 커버리지
- [ ] Module 1: `wsLogWriter.Write()` 확장 로직 커버리지 85% 이상
- [ ] Module 2: `sanitizeFlowName` 100% 커버리지
- [ ] Module 2: DeployFlow 컴포넌트 이름 변경 커버리지 85% 이상
- [ ] Module 3: `WithObserver` 옵션 커버리지 100%
- [ ] Module 3: Observer 폴백 케이스 커버리지 100%

### 하위 호환성

- [ ] 기존 `log.entry` WebSocket 메시지의 `level`, `message`, `timestamp` 필드 유지
- [ ] 기존 `node.*` 와일드카드 패턴이 새 `flow.*.node.*` 패턴에서도 동작 확인
- [ ] Observer 없이 API 서버 기동 시 정상 동작 확인

### 통합 검증

- [ ] 전체 파이프라인: 로그 생성 -> Observer -> StreamRouter -> wsLogWriter -> WebSocket -> Web UI에서 component 표시
- [ ] 다중 플로우 배포 시 노드 컴포넌트 이름 구분 가능
- [ ] API 로그가 Observer를 통해 WebSocket으로 전달됨

---

## Definition of Done

1. 3개 모듈 모두의 요구사항이 구현되었다
2. 모든 수락 기준 (AC-xx-xx)이 충족되었다
3. 모든 테스트 수락 기준 (AC-xx-Tx)이 충족되었다
4. `go test ./...` 전체 통과
5. `go test -race ./...` 데이터 레이스 없음
6. Quality Gate 체크리스트 전체 충족
7. 코드 리뷰 완료
