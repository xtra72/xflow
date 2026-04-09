---
id: SPEC-AGENT-005
version: "1.0.0"
status: completed
created: "2026-04-09"
updated: "2026-04-09"
author: xtra
priority: high
tags: [agent, lifecycle, configuration]
---

# SPEC-AGENT-005: Agent Enable/Disable - 에이전트 활성화/비활성화 기능

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-AGENT-005 |
| 제목 | Agent Enable/Disable (에이전트 활성화/비활성화) |
| 버전 | 1.0.0 |
| 상태 | draft |
| 작성일 | 2026-04-09 |
| 작성자 | xtra |
| 우선순위 | high |
| 관련 SPEC | SPEC-AGENT-001 (Agent Framework), SPEC-AGENT-002, SPEC-AGENT-003, SPEC-AGENT-004 |
| 도메인 | Agent Lifecycle / Configuration |

---

## 1. Overview (개요)

### 1.1 배경

현재 xflow 의 에이전트는 데몬 시작 시 모든 등록된 에이전트가 자동으로 시작된다. 이는 다음과 같은 운영상의 어려움을 야기한다:

- **임시 비활성화 불가**: 특정 에이전트를 일시적으로 자동 시작에서 제외하려면 에이전트 자체를 삭제해야 하며, 재등록 시 모든 설정을 재입력해야 한다.
- **장애 격리 어려움**: 문제가 발생한 에이전트를 데몬 재시작 시 함께 시작하지 않도록 하려면 수동으로 매번 정지해야 한다.
- **유지보수 비효율**: 점검 중인 에이전트를 데몬 재시작 후에도 정지 상태로 유지하려면 별도 운영 절차가 필요하다.
- **단계적 배포 제약**: 신규 에이전트를 등록하되 즉시 자동 시작되지 않도록 제어할 수 없다.

### 1.2 제안 기능

`AgentConfig` 에 영속적인 `Enabled` 필드를 추가하여, 에이전트의 **자동 시작 여부**를 설정으로 제어할 수 있도록 한다. 이는 런타임 lifecycle (start/stop) 과는 완전히 독립적인 **영속 설정**이다.

### 1.3 핵심 원칙

1. **영속성**: `Enabled` 값은 데몬 재시작 후에도 유지된다 (저장소에 영속화).
2. **런타임 독립성**: `Enabled` 와 런타임 상태(running/stopped)는 완전히 독립적이다.
   - Disabled 상태에서도 수동 Start 는 허용 (일시 시작)
   - Running 상태에서 Disable 해도 즉시 정지하지 않음 (다음 데몬 재시작 시 적용)
3. **하위 호환성**: 기존 에이전트(필드 없음)는 기본값 `Enabled = true` 로 동작한다.
4. **명시적 검증**: Disabled 에이전트를 참조하는 플로우의 배포는 거부한다.
5. **일관성**: 기존 `NodeDef.Enabled *bool` + `IsEnabled()` 패턴을 따른다.

### 1.4 Start/Stop 과의 차이점

| 구분 | Start/Stop (런타임) | Enable/Disable (설정) |
|------|---------------------|------------------------|
| 영속성 | 휘발성 (메모리) | 영속 (저장소) |
| 데몬 재시작 영향 | 모두 자동 시작됨 | Disabled 는 시작 안 됨 |
| 적용 시점 | 즉시 | 다음 데몬 재시작 |
| 반복 호출 | 가능 | 가능 |
| 플로우 검증 | 영향 없음 | DeployFlow 거부 |

---

## 2. Environment (환경)

### 2.1 현재 아키텍처

- **AgentConfig** (`internal/agent/config.go`): 에이전트의 정적 설정 구조체. JSON/YAML 직렬화 지원 (`internal/agent/serialize.go`).
- **데몬 자동 시작** (`cmd/xflowd/main.go`): 데몬 부팅 시 저장된 모든 에이전트를 순회하며 `agentMgr.Start()` 호출.
- **저장소** (`internal/store`): 에이전트 설정을 JSON BLOB 으로 저장. 스키마 변경 없이 신규 필드 마샬링 가능.
- **참조 패턴**: `pkg/flow/node.go` 에 이미 `NodeDef.Enabled *bool` + `IsEnabled()` 메서드가 존재한다. 본 SPEC 은 동일 패턴을 따른다.

### 2.2 영향 범위

- Core 계층: `internal/agent/config.go`, `internal/agent/serialize.go`, `internal/agent/manager.go`
- Daemon 계층: `cmd/xflowd/main.go`
- API 계층: `internal/api/handler/agent.go`, `internal/api/service/agent_adapter.go`, `internal/api/dto/response.go`
- Engine 계층: `internal/engine/agent_validation.go`, `internal/engine/errors.go`
- Web UI: `web/src/types/agent.ts`, `web/src/hooks/useAgent.ts`, `web/src/pages/agents/*`

---

## 3. Assumptions (가정)

1. 저장소는 JSON BLOB 으로 에이전트 설정을 저장하므로, 신규 필드 추가 시 스키마 마이그레이션이 불필요하다.
2. 기존 운영 중인 에이전트는 모두 활성 상태로 간주되어야 한다 (하위 호환).
3. `Enabled *bool` 포인터 타입을 사용하여 "값 없음"(nil) 과 "명시적 false" 를 구분한다.
4. 데몬 재시작은 정상적인 운영 절차이며, 즉시 적용이 필요한 경우 사용자가 수동으로 Stop 호출 가능하다.
5. 플로우는 배포 시점(DeployFlow)에 참조하는 에이전트의 상태를 검증한다.
6. Web UI 는 에이전트 목록에서 enabled/disabled 상태를 시각적으로 구분 표시한다.

---

## 4. Requirements (요구사항 - EARS Format)

### M1: AgentConfig 확장 (Enabled 필드)

**R1.1 (Ubiquitous)**: 시스템은 항상 `AgentConfig` 에 `Enabled *bool` 필드를 제공해야 한다.

**R1.2 (Ubiquitous)**: `AgentConfig` 는 항상 `IsEnabled() bool` 헬퍼 메서드를 제공해야 한다. `Enabled` 가 nil 이면 `true` 를 반환한다.

**R1.3 (Ubiquitous)**: `AgentConfig` 의 JSON/YAML 직렬화는 항상 `enabled` 필드를 `omitempty` 정책으로 처리해야 한다 (기본값일 경우 출력 생략).

**R1.4 (State-Driven)**: IF `AgentConfig.Enabled` 가 nil 이면 THEN `IsEnabled()` 는 `true` 를 반환해야 한다.

**R1.5 (State-Driven)**: IF `AgentConfig.Enabled` 가 명시적으로 `false` 이면 THEN `IsEnabled()` 는 `false` 를 반환해야 한다.

---

### M2: 데몬 자동 시작 제어

**R2.1 (Event-Driven)**: WHEN 데몬이 시작되어 저장된 에이전트를 자동 시작하는 루프를 실행할 때, THEN 시스템은 각 에이전트의 `IsEnabled()` 를 확인해야 한다.

**R2.2 (State-Driven)**: WHILE 자동 시작 루프가 진행되는 동안, IF 에이전트가 disabled 상태이면 THEN 시스템은 해당 에이전트의 `Start()` 호출을 건너뛰어야 한다.

**R2.3 (Event-Driven)**: WHEN 자동 시작 루프가 disabled 에이전트를 건너뛸 때, THEN 시스템은 INFO 레벨로 "skipped auto-start (disabled)" 로그를 남겨야 한다.

**R2.4 (Ubiquitous)**: 시스템은 항상 disabled 에이전트도 매니저에 등록(Create)해야 한다. 즉, 목록 조회/조회 API 에서는 disabled 에이전트도 노출되어야 한다.

**R2.5 (Unwanted)**: 시스템은 disabled 에이전트를 데몬 부팅 시 자동 시작해서는 안 된다.

---

### M3: Enable/Disable API 엔드포인트

**R3.1 (Ubiquitous)**: 시스템은 항상 `POST /agents/{id}/enable` 엔드포인트를 제공해야 한다.

**R3.2 (Ubiquitous)**: 시스템은 항상 `POST /agents/{id}/disable` 엔드포인트를 제공해야 한다.

**R3.3 (Event-Driven)**: WHEN `POST /agents/{id}/enable` 이 호출되면, THEN 시스템은 해당 에이전트의 `Enabled` 값을 `true` 로 설정하고 저장소에 영속화해야 한다.

**R3.4 (Event-Driven)**: WHEN `POST /agents/{id}/disable` 이 호출되면, THEN 시스템은 해당 에이전트의 `Enabled` 값을 `false` 로 설정하고 저장소에 영속화해야 한다.

**R3.5 (Event-Driven)**: WHEN Enable/Disable API 호출이 성공하면, THEN 시스템은 변경된 에이전트 정보(AgentInfo, `enabled` 필드 포함)를 200 OK 와 함께 반환해야 한다.

**R3.6 (Event-Driven)**: WHEN 존재하지 않는 에이전트 ID 로 Enable/Disable 이 호출되면, THEN 시스템은 404 Not Found 를 반환해야 한다.

**R3.7 (Unwanted)**: Disable 작업은 현재 실행 중인 에이전트의 런타임 상태(running)에 영향을 주어서는 안 된다. 즉, Stop 을 호출해서는 안 된다.

**R3.8 (Unwanted)**: Enable 작업은 현재 정지된 에이전트를 자동으로 Start 해서는 안 된다.

**R3.9 (Ubiquitous)**: `GET /agents` 와 `GET /agents/{id}` 응답은 항상 `enabled: bool` 필드를 포함해야 한다.

---

### M4: Disabled 에이전트 수동 Start 허용

**R4.1 (Event-Driven)**: WHEN `POST /agents/{id}/start` 가 disabled 에이전트에 대해 호출되면, THEN 시스템은 일반 에이전트와 동일하게 정상적으로 시작해야 한다 (일시 시작).

**R4.2 (Ubiquitous)**: 수동으로 시작된 disabled 에이전트는 다음 데몬 재시작 시 자동 시작되지 않아야 한다 (`Enabled` 값은 변경되지 않는다).

**R4.3 (Event-Driven)**: WHEN disabled 에이전트가 수동으로 시작되면, THEN 시스템은 INFO 레벨로 "manual start of disabled agent" 로그를 남겨야 한다.

**R4.4 (Event-Driven)**: WHEN `POST /agents/{id}/stop` 이 disabled 에이전트에 대해 호출되면, THEN 시스템은 일반 에이전트와 동일하게 정상적으로 정지해야 한다.

---

### M5: 플로우 배포 시 Disabled 에이전트 검증

**R5.1 (Event-Driven)**: WHEN `DeployFlow` 가 호출되어 플로우의 노드들을 검증할 때, THEN 시스템은 `validateAgentRefs()` 를 통해 각 노드의 `AgentRef.AgentID` 가 disabled 상태인지 확인해야 한다.

**R5.2 (State-Driven)**: IF 플로우의 어떤 노드가 disabled 에이전트를 참조하면 THEN 시스템은 `ErrAgentDisabled` 에러와 함께 DeployFlow 를 거부해야 한다.

**R5.3 (Ubiquitous)**: `ErrAgentDisabled` 에러 메시지는 항상 다음 정보를 포함해야 한다:
- 비활성화된 에이전트의 ID
- 해당 에이전트를 참조하는 노드의 ID 또는 이름
- 권장 조치 (Enable API 호출 안내)

**R5.4 (Ubiquitous)**: `internal/engine/errors.go` 는 항상 sentinel error `ErrAgentDisabled` 를 export 해야 한다.

**R5.5 (Unwanted)**: 시스템은 disabled 에이전트를 참조하는 플로우를 배포해서는 안 된다 (런타임 시점에 nil agent 참조 오류 방지).

**R5.6 (Optional)**: WHERE 가능한 경우, 검증 에러는 disabled 에이전트가 여러 개일 때 모두 한 번에 보고해야 한다 (조기 반환 대신 모든 검증 누적).

---

### M6: Web UI Enable/Disable 컨트롤

**R6.1 (Ubiquitous)**: Web UI 의 `AgentInfo` TypeScript 인터페이스는 항상 `enabled?: boolean` 필드를 포함해야 한다.

**R6.2 (Ubiquitous)**: Web UI 는 항상 에이전트 목록의 각 항목에 enabled/disabled 상태를 시각적으로 구분 표시해야 한다 (예: 회색 "Disabled" 배지).

**R6.3 (Event-Driven)**: WHEN 사용자가 에이전트 행의 Enable/Disable 토글 버튼을 클릭하면, THEN Web UI 는 해당하는 API 엔드포인트(`/enable` 또는 `/disable`)를 호출해야 한다.

**R6.4 (Event-Driven)**: WHEN Enable/Disable API 호출이 성공하면, THEN Web UI 는 React Query 캐시를 무효화하여 목록을 즉시 갱신해야 한다.

**R6.5 (Optional)**: WHERE 가능하면, 에이전트 목록 페이지는 enabled/disabled 상태로 필터링할 수 있는 컨트롤을 제공해야 한다.

**R6.6 (Optional)**: WHERE 가능하면, disabled 상태로 표시된 에이전트의 Start 버튼을 클릭할 때 "비활성화된 에이전트의 일시 시작입니다" 와 같은 안내 메시지를 표시해야 한다.

---

### M7: 하위 호환성 (기존 에이전트 기본값 Enabled)

**R7.1 (State-Driven)**: IF 저장소에서 로드한 `AgentConfig` 의 `Enabled` 필드가 nil (JSON 에 `enabled` 키 없음) 이면 THEN 시스템은 해당 에이전트를 enabled 로 취급해야 한다.

**R7.2 (Ubiquitous)**: 시스템은 항상 기존에 등록된 모든 에이전트를 본 SPEC 적용 후에도 동일한 동작 (자동 시작) 으로 유지해야 한다.

**R7.3 (Unwanted)**: 본 SPEC 의 도입은 기존 저장소 데이터의 마이그레이션 작업을 요구해서는 안 된다.

**R7.4 (Ubiquitous)**: API 응답에서 기존 에이전트(Enabled 필드 미설정)는 항상 `enabled: true` 로 응답해야 한다.

---

## 5. Out of Scope (범위 외)

본 SPEC 에서 다루지 않는 항목은 다음과 같다:

1. **스케줄 기반 자동 enable/disable**: 시간/요일 기반 자동 활성화는 별도 SPEC 으로 분리한다.
2. **그룹 단위 enable/disable**: 다수 에이전트를 한 번에 enable/disable 하는 기능은 별도 SPEC 에서 다룬다.
3. **Disable 시 즉시 정지 옵션**: 본 SPEC 에서는 Disable 시 런타임 정지하지 않는 정책으로 결정. 향후 옵션화는 별도 논의.
4. **에이전트 상태 변경 이벤트 알림**: enable/disable 변경 시 Webhook/MQTT 알림은 별도 SPEC.
5. **권한 기반 접근 제어**: enable/disable API 의 인증/인가는 SPEC-AUTH-* 에 위임한다.
6. **변경 이력 audit log**: 누가 언제 enable/disable 했는지에 대한 감사 기록은 별도 SPEC.

---

## 6. Non-Functional Requirements (비기능 요구사항)

### 6.1 하위 호환성 (Backward Compatibility)

- **NFR1**: 기존 저장소 데이터는 마이그레이션 없이 그대로 사용 가능해야 한다.
- **NFR2**: 기존 API 응답 구조는 `enabled` 필드 추가 외 변경되지 않아야 한다.
- **NFR3**: 기존 클라이언트(`enabled` 필드 인지 못하는 구버전)는 정상 동작해야 한다.

### 6.2 성능 (Performance)

- **NFR4**: 데몬 자동 시작 루프의 enabled 체크는 O(1) 이어야 하며, 전체 부팅 시간에 측정 가능한 영향을 주지 않아야 한다.
- **NFR5**: DeployFlow 의 disabled 검증은 노드 수에 비례하는 O(N) 이어야 한다.
- **NFR6**: API 호출 응답 시간은 기존 start/stop 엔드포인트 대비 ±10% 이내여야 한다.

### 6.3 신뢰성 (Reliability)

- **NFR7**: Enable/Disable 호출 후 저장소 영속화 실패 시 in-memory 상태를 롤백해야 한다.
- **NFR8**: 동시에 여러 클라이언트가 같은 에이전트의 enable/disable 을 호출해도 일관된 최종 상태를 보장해야 한다 (manager-level lock).

### 6.4 관측성 (Observability)

- **NFR9**: 자동 시작 시 disabled 로 인해 건너뛴 에이전트는 부팅 로그에 명시되어야 한다.
- **NFR10**: Enable/Disable 변경은 INFO 레벨로 기록되어야 한다 (에이전트 ID 포함).

### 6.5 일관성 (Consistency)

- **NFR11**: 본 SPEC 의 구현은 기존 `NodeDef.Enabled` + `IsEnabled()` 패턴과 코드 스타일/명명 규칙이 일치해야 한다.

---

## 7. Traceability

| 요구사항 | 구현 모듈 | 테스트 | 비고 |
|----------|-----------|--------|------|
| M1 (R1.1~R1.5) | `internal/agent/config.go`, `internal/agent/serialize.go` | `config_test.go`, `serialize_test.go` | NodeDef 패턴 참조 |
| M2 (R2.1~R2.5) | `cmd/xflowd/main.go` (auto-start loop) | `main_test.go` (integration) | |
| M3 (R3.1~R3.9) | `internal/api/handler/agent.go`, `internal/api/service/agent_adapter.go`, `internal/api/dto/response.go` | `agent_handler_test.go` | |
| M4 (R4.1~R4.4) | 기존 start/stop 핸들러 (변경 없음, 검증만) | 신규 통합 테스트 | |
| M5 (R5.1~R5.6) | `internal/engine/agent_validation.go`, `internal/engine/errors.go` | `agent_validation_test.go` | |
| M6 (R6.1~R6.6) | `web/src/types/agent.ts`, `web/src/hooks/useAgent.ts`, `web/src/pages/agents/*` | Vitest + RTL | |
| M7 (R7.1~R7.4) | `internal/agent/serialize.go` (omitempty + nil 처리) | `serialize_backward_test.go` | |

---

## 8. References (참조)

- **참조 패턴**: `pkg/flow/node.go` 의 `NodeDef.Enabled *bool` + `IsEnabled() bool` (동일 패턴 적용)
- **관련 SPEC**: SPEC-AGENT-001 (Agent Framework), SPEC-WIRE-001 (Engine Validation 패턴)
- **EARS Format**: Easy Approach to Requirements Syntax (Mavin, 2009)
