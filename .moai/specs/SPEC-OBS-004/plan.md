---
id: SPEC-OBS-004
version: "1.0.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
priority: high
type: plan
---

# SPEC-OBS-004 구현 계획: 로그 출력 요소 식별 강화

## 1. 개요

### 1.1 목표

XFlow 시스템의 로그 출력에서 컴포넌트 식별 정보를 Web UI까지 전달하고, 노드 로그에 플로우 컨텍스트를 추가하며, API 레이어를 Observer 시스템에 통합한다.

### 1.2 개발 방법론

- **모드**: Hybrid (TDD for new, DDD for existing)
- **새 코드**: `classifySource`, `sanitizeFlowName` 등 유틸 함수 -> TDD (RED-GREEN-REFACTOR)
- **기존 코드 수정**: `wsLogWriter.Write()`, `DeployFlow()`, API 핸들러 -> DDD (ANALYZE-PRESERVE-IMPROVE)

### 1.3 모듈 독립성

3개 모듈은 서로 독립적으로 구현 가능하다. Module 1을 먼저 완료하면 Web UI에서 즉시 효과를 확인할 수 있으며, Module 2와 Module 3은 병렬 또는 순차적으로 진행 가능하다.

---

## 2. 마일스톤

### Primary Goal: Module 1 - WebSocket 로그 component 필드 추가

가장 높은 즉각적 효과를 제공한다. 기존 Observer 시스템이 이미 component를 로그에 포함하고 있으므로, wsLogWriter에서 이를 추출하여 전달하기만 하면 된다.

**구현 단계:**

1. **ANALYZE**: `wsLogWriter.Write()` 현재 동작 분석, JSON 로그 레코드의 component 필드 위치 확인
2. **PRESERVE**: 기존 `log_writer_test.go` 테스트 통과 확인
3. **TDD (새 함수)**: `classifySource()` 함수에 대한 테스트 먼저 작성 -> 구현
4. **IMPROVE**: `wsLogWriter.Write()`에 component/source 추출 및 페이로드 확장 적용
5. **검증**: 확장된 페이로드로 기존 테스트 업데이트 및 새 테스트 추가
6. **(Optional)** Web UI 로그 컴포넌트 업데이트

**변경 파일 요약:**

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/api/ws/log_writer.go` | 수정 (DDD) | component, source 추출 및 페이로드 확장 |
| `internal/api/ws/log_writer_test.go` | 수정 (DDD+TDD) | 기존 테스트 업데이트 + 새 테스트 |

### Secondary Goal: Module 2 - Flow 컨텍스트 노드 컴포넌트 이름 포함

노드 로그에 플로우 컨텍스트를 추가하여 다중 플로우 환경에서 노드를 구분 가능하게 한다.

**구현 단계:**

1. **ANALYZE**: `engine.go:92` DeployFlow의 노드 컴포넌트 이름 생성 로직 분석
2. **TDD (새 함수)**: `sanitizeFlowName()` 함수 테스트 먼저 작성 -> 구현
3. **PRESERVE**: 기존 엔진 테스트 통과 확인, 와일드카드 패턴 동작 확인
4. **IMPROVE**: `DeployFlow()`의 컴포넌트 이름을 `flow.{flowName}.node.{nodeName}` 으로 변경
5. **검증**: 와일드카드 패턴 매칭 테스트, 기존 엔진 테스트 회귀 검증

**변경 파일 요약:**

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/engine/engine.go` | 수정 (DDD) | 노드 컴포넌트 이름 패턴 변경 |
| `internal/engine/engine_test.go` | 수정 (DDD+TDD) | 컴포넌트 이름 검증 + 새 테스트 |

### Final Goal: Module 3 - API 레이어 Observer 통합

API 핸들러를 Observer 시스템에 통합하여 모든 API 로그가 컴포넌트 식별 정보를 포함하도록 한다.

**구현 단계:**

1. **ANALYZE**: `internal/api/server.go` Server 구조체 및 초기화 패턴 분석
2. **ANALYZE**: 모든 `slog.Default()` 사용 위치 파악 (약 20개 파일)
3. **PRESERVE**: 기존 API 테스트 통과 확인
4. **TDD**: `WithObserver()` 옵션 함수 테스트 작성 -> 구현
5. **IMPROVE**: Server에 Observer 주입, 핸들러별 컴포넌트 로거 전달
6. **IMPROVE**: 각 핸들러/서비스에서 `slog.Default()` -> 컴포넌트 로거 교체
7. **IMPROVE**: `cmd/xflowd/main.go`에서 Observer를 API 서버에 전달
8. **검증**: 전체 API 테스트 회귀 검증, 로그 출력에 컴포넌트 포함 확인

**변경 파일 요약:**

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/api/server.go` | 수정 (DDD) | Observer 주입, 컴포넌트 로거 생성 |
| `internal/api/router.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/handler/flow.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/handler/agent.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/handler/node.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/handler/monitor.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/handler/websocket.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/ws/hub.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/ws/client.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/ws/event_publisher.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/ws/broadcaster.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/service/flow_adapter.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/service/agent_adapter.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `internal/api/service/node_adapter.go` | 수정 (DDD) | 컴포넌트 로거 사용 |
| `cmd/xflowd/main.go` | 수정 (DDD) | Observer를 API 서버에 전달 |

---

## 3. 기술 접근 방식

### 3.1 Module 1: WebSocket 페이로드 확장

**접근**: `wsLogWriter.Write()` 메서드 내에서 JSON 파싱 후 `component` 필드를 추가 추출한다. 이미 JSON unmarshal이 수행되므로 추가 성능 비용이 최소이다.

**핵심 변경:**
```go
// 기존 추출
msg, _ := logLine["msg"].(string)
ts, _ := logLine["time"].(string)

// 추가 추출
component, _ := logLine["component"].(string)
if component == "" {
    component = "unknown"
}
source := classifySource(component)

// 확장된 페이로드
payload := map[string]string{
    "level":     levelStr,
    "message":   msg,
    "timestamp": ts,
    "component": component,
    "source":    source,
}
```

### 3.2 Module 2: 노드 컴포넌트 이름 확장

**접근**: `engine.go`의 DeployFlow 루프에서 컴포넌트 이름 생성 라인만 변경한다.

**핵심 변경:**
```go
// 기존
component := fmt.Sprintf("node.%s", nd.Name)

// 변경
flowName := sanitizeFlowName(f.Name())
if flowName == "" {
    flowName = sanitizeFlowName(f.ID())
}
if flowName == "" {
    flowName = "unnamed"
}
component := fmt.Sprintf("flow.%s.node.%s", flowName, nd.Name)
```

### 3.3 Module 3: API Observer 통합

**접근**: Server 구조체에 Observer 필드를 추가하고, 핸들러 생성 시 컴포넌트 로거를 주입한다. Observer가 없는 경우 기존 `slog.Default()` 동작을 유지한다.

**핵심 패턴:**
```go
// Server 구조체 확장
type Server struct {
    // ... 기존 필드
    observer *observe.Observer  // NEW
}

// Observer 주입 옵션
func WithObserver(obs *observe.Observer) ServerOption {
    return func(s *Server) {
        s.observer = obs
        if obs != nil {
            s.logger = obs.Loggers.NewLogger("api.server").Logger()
        }
    }
}

// 핸들러 생성 시 로거 주입
func (s *Server) createHandlers() {
    var flowLogger *slog.Logger
    if s.observer != nil {
        flowLogger = s.observer.Loggers.NewLogger("api.handler.flow").Logger()
    } else {
        flowLogger = slog.Default()
    }
    // ...
}
```

---

## 4. 리스크 분석

### 4.1 리스크 식별

| 리스크 | 심각도 | 발생 확률 | 영향 | 대응 방안 |
|--------|--------|----------|------|----------|
| 노드 컴포넌트 이름 변경으로 기존 와일드카드 레벨 설정 파괴 | 높음 | 중간 | 로그 레벨 설정이 적용되지 않음 | `flow.*.node.*` 패턴 지원 확인, 기존 `node.*` 패턴 호환성 테스트 |
| API 핸들러 slog 교체 시 누락 | 중간 | 중간 | 일부 API 로그에 컴포넌트 미포함 | Grep으로 모든 slog 호출 위치 사전 파악, 교체 후 검증 |
| WebSocket 페이로드 변경으로 프론트엔드 오류 | 중간 | 낮음 | Web UI 로그 표시 깨짐 | 필드 추가만 수행 (기존 필드 유지), 프론트엔드 호환성 확인 |
| JSON 로그에 component 키가 없는 경우 | 낮음 | 낮음 | source 분류 실패 | 기본값 "unknown"/"system" 적용 |

### 4.2 의존성

- SPEC-OBS-001 (구현 완료): Observer 시스템 기반 인프라 제공 -> **의존성 충족**
- SPEC-OBS-002 (planned): 서버 레벨 파일 기반 로그 출력 -> **독립적** (이 SPEC과 충돌 없음)
- SPEC-OBS-003 (planned): 노드별 로그 출력 라우팅 -> **독립적** (컴포넌트 이름 변경은 라우팅에 영향 없음)

### 4.3 영향 범위

- **Module 1**: 2개 파일 변경 (낮은 영향)
- **Module 2**: 2개 파일 변경 (중간 영향 - 컴포넌트 이름 체계 변경)
- **Module 3**: 15개 이상 파일 변경 (높은 영향 - 광범위한 로거 교체)

---

## 5. 전체 변경 파일 요약

| 모듈 | 파일 | 변경 유형 | 우선순위 |
|------|------|----------|---------|
| M1 | `internal/api/ws/log_writer.go` | 수정 | P0 |
| M1 | `internal/api/ws/log_writer_test.go` | 수정 | P0 |
| M2 | `internal/engine/engine.go` | 수정 | P0 |
| M2 | `internal/engine/engine_test.go` | 수정 | P0 |
| M3 | `internal/api/server.go` | 수정 | P1 |
| M3 | `internal/api/router.go` | 수정 | P1 |
| M3 | `internal/api/handler/flow.go` | 수정 | P1 |
| M3 | `internal/api/handler/agent.go` | 수정 | P1 |
| M3 | `internal/api/handler/node.go` | 수정 | P1 |
| M3 | `internal/api/handler/monitor.go` | 수정 | P1 |
| M3 | `internal/api/handler/websocket.go` | 수정 | P1 |
| M3 | `internal/api/ws/hub.go` | 수정 | P1 |
| M3 | `internal/api/ws/client.go` | 수정 | P1 |
| M3 | `internal/api/ws/event_publisher.go` | 수정 | P1 |
| M3 | `internal/api/ws/broadcaster.go` | 수정 | P1 |
| M3 | `internal/api/service/flow_adapter.go` | 수정 | P1 |
| M3 | `internal/api/service/agent_adapter.go` | 수정 | P1 |
| M3 | `internal/api/service/node_adapter.go` | 수정 | P1 |
| M3 | `cmd/xflowd/main.go` | 수정 | P1 |

---

## 6. 검증 전략

### 6.1 단위 테스트

- `classifySource()` 테이블 기반 테스트 (모든 source 분류 케이스)
- `sanitizeFlowName()` 테이블 기반 테스트 (특수문자, 빈 문자열, 정상 케이스)
- `wsLogWriter.Write()` 확장 페이로드 검증
- Observer 주입 시 컴포넌트 로거 생성 검증

### 6.2 통합 테스트

- 전체 파이프라인 검증: 로그 생성 -> StreamRouter -> wsLogWriter -> WebSocket payload
- API 서버 기동 시 Observer 통합 검증
- 다중 플로우 배포 시 노드 컴포넌트 이름 구분 검증

### 6.3 회귀 테스트

- 기존 `go test ./internal/api/...` 전체 통과
- 기존 `go test ./internal/engine/...` 전체 통과
- `go test -race ./...` 동시성 안전 검증
