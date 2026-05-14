---
id: SPEC-LGCP-003
version: "1.1.0"
status: completed
created: "2026-04-06"
updated: "2026-05-14"
author: xtra
priority: high
tags: lgcp, flow-node, status, control, lg, indoor-unit, hvac
prerequisite: SPEC-LGCP-001, SPEC-LGCP-002
---

# SPEC-LGCP-003: LGCP 플로우 노드 구현 (lgcp-status, lgcp-control, lgcp)

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-04-06 | xtra | 최초 작성 |
| 1.1.0 | 2026-05-14 | xtra | **노드 Init-tolerance 패턴 적용**. `initAgent()` 가 Init 시점에 에이전트를 resolve 하지 못하면(disabled 또는 미등록) hard-fail 하지 않고 경고 로그 + Running 전이(deferred connection) 후, 에이전트 활성화 시 SPEC-ENGINE-001 `ReinitNodesForAgent` 로 자동 재연결한다. `ErrLGCPNoResolver`(resolver 미설정, 구성 오류)와 `ErrLGCPAgentNotLGCP`(타입 불일치)는 회복 불가능하므로 hard-fail 유지. REQ-LGCP-NODE-003 amend. 관련: SPEC-AGENT-005 v1.1.0, SPEC-ENGINE-001 v1.3.0 Module 8, SPEC-SERIAL-001 v2.2.0. |

---

## 1. 개요

LGCP 에이전트(SPEC-LGCP-001, SPEC-LGCP-002)에 직접 연결되는 3종의 플로우 노드를 구현한다. 기존 LGAP 노드(`internal/node/lgap.go`)와 동일한 아키텍처 패턴을 따르되, LGCP 프로토콜의 고유한 특성(address 기반 실내기 지정, get_stats/get_recent 상태 조회, control_enabled 필수 등)을 반영한다.

### 1.1 목적

- LGCP 에이전트의 상태 조회 및 제어 기능을 플로우 노드로 노출
- 기존 BridgeNode를 거치지 않고 LGCP 에이전트에 직접 접근하는 전용 노드 제공
- LGAP 노드와 일관된 사용 패턴으로 학습 비용 최소화

### 1.2 범위

- `lgcp-status`: 폴링 기반 상태 조회 노드 (SourceNode)
- `lgcp-control`: 제어 명령 전송 노드
- `lgcp` (통합): 상태 조회 + 제어 자동 감지 통합 노드 (SourceNode)

### 1.3 LGAP vs LGCP 주요 차이점

| 항목 | LGAP | LGCP |
|------|------|------|
| 디바이스 식별 | `device_id` (문자열) | `address` (hex 문자열, 실내기 주소) |
| 상태 조회 명령 | `get_state` / `get_all_states` | `get_stats` / `get_recent` |
| 제어 활성화 | 항상 가능 | `control_enabled: true` 필요 |
| 제어 명령 파라미터 | `device_id` + settings | `address` + params |
| 에이전트 타입 체크 | `*lg.LGAPAgent` | `*lg.LGCPAgent` |

---

## 2. 환경 (Environment)

### 기술 스택

- 언어: Go 1.23+
- 에이전트 프레임워크: `internal/agent/lg/` (LGCPAgent)
- 노드 프레임워크: `internal/node/` (BaseNode, Node, SourceNode 인터페이스)
- 생명주기: `pkg/lifecycle/` (BaseLifecycle)
- 메시지: `pkg/message/` (Message 인터페이스)

### 의존성

- `internal/node` - BaseNode, AgentResolver, AgentAccessor 인터페이스
- `internal/agent/lg` - LGCPAgent 타입 (타입 체크용)
- `internal/agent` - Agent 인터페이스 (Process 호출)
- `pkg/flow` - NodeDef, AgentRef
- `pkg/message` - Message 인터페이스
- `pkg/lifecycle` - 상태 전이

### 제약 조건

- 기존 LGAP 노드 구조체/함수를 변경하지 않는다
- LGCP 에이전트의 Process() JSON 프로토콜을 그대로 사용한다
- 노드 레지스트리에 빌트인으로 등록한다

---

## 3. 가정 (Assumptions)

- A1: LGCP 에이전트의 Process() 메서드는 SPEC-LGCP-001에 정의된 JSON 커맨드 형식을 수용한다
- A2: LGCP 에이전트의 제어 기능은 SPEC-LGCP-002에 따라 `control_enabled: true`일 때만 동작한다
- A3: 기존 LGAP 노드의 아키텍처 패턴(lgapNodeBase, pollLoop, SourceNode)을 LGCP에도 동일하게 적용한다
- A4: address 파라미터는 hex 문자열로 실내기 주소를 나타내며, LGCP 에이전트가 내부적으로 파싱한다
- A5: AgentResolver 및 AgentAccessor 인터페이스가 LGCP 에이전트에서도 동일하게 동작한다

---

## 4. 기능 요구사항 (Requirements)

### 4.1 공통 기반 (lgcpNodeBase)

**REQ-LGCP-NODE-001** [유비쿼터스]
시스템은 **항상** LGCP 노드 공통 설정 구조체(`LGCPNodeConfig`)를 제공해야 한다. 설정 필드: `agent_ref`(필수), `default_address`(선택), `poll_interval`(선택, 기본 "30s"), `timeout`(선택, 기본 "5s"), `poll_command`(선택, 기본 "get_stats"), `recent_count`(선택, 기본 10).

**REQ-LGCP-NODE-002** [이벤트 기반]
**WHEN** `agent_ref` 설정이 누락되면 **THEN** `ErrLGCPMissingAgentRef` 에러를 반환해야 한다.

**REQ-LGCP-NODE-003** [이벤트 기반]
**WHEN** `initAgent()`가 호출되면 **THEN** AgentResolver를 통해 에이전트를 resolve하고, AgentAccessor로 원본 에이전트를 획득하여 `*lg.LGCPAgent` 타입인지 확인해야 한다.

> **v1.1.0 보강 — Init-tolerance**: `initAgent()` 가 에이전트를 resolve 하지 못하는
> 경우(disabled 또는 미등록), hard-fail 하지 **않는다**. 대신 경고(WARNING) 로그를
> 남기고 노드를 `Running` 으로 전이시키며 에이전트 연결을 보류(deferred connection)한다.
> 이후 해당 에이전트가 활성화되면 SPEC-ENGINE-001 `ReinitNodesForAgent` 경로로 자동
> 재연결된다. 단, `ErrLGCPNoResolver`(resolver 미설정, REQ-LGCP-NODE-???)와
> `ErrLGCPAgentNotLGCP`(타입 불일치, REQ-LGCP-NODE-004)는 deferred connection 으로
> 회복 불가능한 구성 오류이므로 기존대로 hard-fail 한다. 관련: SPEC-AGENT-005 v1.1.0,
> SPEC-ENGINE-001 v1.3.0 Module 8.

**REQ-LGCP-NODE-004** [이벤트 기반]
**WHEN** resolve된 에이전트가 `*lg.LGCPAgent` 타입이 아니면 **THEN** `ErrLGCPAgentNotLGCP` 에러를 반환해야 한다.

**REQ-LGCP-NODE-005** [유비쿼터스]
시스템은 **항상** `callAgentProcess()` 호출 시 설정된 timeout으로 context deadline을 적용해야 한다.

### 4.2 LGCP Status Node (lgcp-status)

**REQ-LGCP-NODE-006** [유비쿼터스]
시스템은 **항상** `lgcp-status` 노드를 노드 레지스트리에 빌트인으로 등록해야 한다.

**REQ-LGCP-NODE-007** [유비쿼터스]
`lgcp-status` 노드는 **항상** `Node` 및 `SourceNode` 인터페이스를 구현해야 한다.

**REQ-LGCP-NODE-008** [이벤트 기반]
**WHEN** `lgcp-status` 노드가 Init되면 **THEN** 에이전트를 resolve하고, 설정된 `poll_interval` 간격으로 상태 조회 폴링 고루틴을 시작해야 한다.

**REQ-LGCP-NODE-009** [상태 기반]
**IF** `poll_command`가 `"get_stats"`이면 **THEN** 폴링 시 `{"command": "get_stats"}` 커맨드를 전송해야 한다.

**REQ-LGCP-NODE-010** [상태 기반]
**IF** `poll_command`가 `"get_recent"`이면 **THEN** 폴링 시 `{"command": "get_recent", "count": <recent_count>}` 커맨드를 전송해야 한다.

**REQ-LGCP-NODE-011** [이벤트 기반]
**WHEN** Process()가 호출되면 **THEN** 입력 메시지의 payload에서 `poll_command`, `count`, `address`, `timeout`을 오버라이드하고 상태 조회를 수행하여 결과를 반환해야 한다.

**REQ-LGCP-NODE-012** [이벤트 기반]
**WHEN** Shutdown이 호출되면 **THEN** 폴링 고루틴을 정지하고 노드를 종료해야 한다.

### 4.3 LGCP Control Node (lgcp-control)

**REQ-LGCP-NODE-013** [유비쿼터스]
시스템은 **항상** `lgcp-control` 노드를 노드 레지스트리에 빌트인으로 등록해야 한다.

**REQ-LGCP-NODE-014** [유비쿼터스]
`lgcp-control` 노드는 **항상** `Node` 인터페이스를 구현해야 한다 (SourceNode 아님).

**REQ-LGCP-NODE-015** [이벤트 기반]
**WHEN** Process()가 호출되면 **THEN** 입력 메시지의 payload에서 제어 명령을 추출하여 LGCP 에이전트에 전달해야 한다.

**REQ-LGCP-NODE-016** [이벤트 기반]
**WHEN** payload에 `"command"` 키가 있으면 **THEN** 해당 값을 직접 커맨드로 사용하고, `address`와 `params`를 함께 전달해야 한다.

**REQ-LGCP-NODE-017** [이벤트 기반]
**WHEN** payload에 `"command"` 키가 없고 제어 키(`power`, `mode`, `temperature`, `fan_speed`)가 있으면 **THEN** `set_multiple` 커맨드를 구성하여 전달해야 한다.

**REQ-LGCP-NODE-018** [상태 기반]
**IF** payload에 `address` 키가 없고 노드 설정에 `default_address`가 있으면 **THEN** `default_address`를 제어 대상 주소로 사용해야 한다.

**REQ-LGCP-NODE-019** [비허용 동작]
시스템은 `address`가 비어있는 상태로 제어 명령을 전송**하지 않아야 한다**. address가 없으면 에러를 반환해야 한다.

### 4.4 LGCP 통합 노드 (lgcp)

**REQ-LGCP-NODE-020** [유비쿼터스]
시스템은 **항상** `lgcp` 노드를 노드 레지스트리에 빌트인으로 등록해야 한다.

**REQ-LGCP-NODE-021** [유비쿼터스]
`lgcp` 노드는 **항상** `Node` 및 `SourceNode` 인터페이스를 구현해야 한다.

**REQ-LGCP-NODE-022** [이벤트 기반]
**WHEN** Process()에서 payload에 제어 키(`power`, `mode`, `temperature`, `fan_speed`)가 하나라도 있으면 **THEN** 제어 명령으로 처리해야 한다.

**REQ-LGCP-NODE-023** [이벤트 기반]
**WHEN** Process()에서 payload에 제어 키가 없으면 **THEN** 상태 조회 명령으로 처리해야 한다.

**REQ-LGCP-NODE-024** [이벤트 기반]
**WHEN** `lgcp` 노드가 Init되면 **THEN** 에이전트를 resolve하고 폴링 고루틴을 시작해야 한다 (상태 조회 전용).

**REQ-LGCP-NODE-025** [이벤트 기반]
**WHEN** Shutdown이 호출되면 **THEN** 폴링 고루틴을 정지하고 노드를 종료해야 한다.

### 4.5 에러 정의

**REQ-LGCP-NODE-026** [유비쿼터스]
시스템은 **항상** 다음 LGCP 노드 전용 센티널 에러를 `internal/node/errors.go`에 정의해야 한다:
- `ErrLGCPAgentNotLGCP`: resolve된 Agent가 LGCPAgent 타입이 아닌 경우
- `ErrLGCPMissingAgentRef`: agent_ref 설정 누락
- `ErrLGCPNoResolver`: AgentResolver 미설정
- `ErrLGCPProcessFailed`: Agent Process() 호출 실패
- `ErrLGCPMissingAddress`: 제어 명령에 address 누락

### 4.6 노드 등록

**REQ-LGCP-NODE-027** [유비쿼터스]
시스템은 **항상** `internal/node/registry.go`의 `registerBuiltins()`에 다음 3종 노드를 등록해야 한다:
- `lgcp-status`: `NewLGCPStatusNode`, category `"io"`, description `"LG LGCP 디바이스 상태 조회"`
- `lgcp-control`: `NewLGCPControlNode`, category `"io"`, description `"LG LGCP 디바이스 제어"`
- `lgcp`: `NewLGCPNode`, category `"io"`, description `"LG LGCP 상태 조회 + 제어 통합"`

---

## 5. 비기능 요구사항

### 5.1 성능

- **NFR-LGCP-PERF-001**: 폴링 고루틴의 CPU 오버헤드는 유휴 상태에서 0.1% 이하여야 한다
- **NFR-LGCP-PERF-002**: Process() 호출의 오버헤드(에이전트 호출 제외)는 1ms 이내여야 한다

### 5.2 신뢰성

- **NFR-LGCP-REL-001**: 폴링 중 에이전트 에러 발생 시 고루틴이 중단되지 않고 다음 폴링 주기에서 재시도해야 한다
- **NFR-LGCP-REL-002**: Shutdown 호출 시 폴링 고루틴이 확실히 종료되어야 한다 (sync.Once 보호)

### 5.3 일관성

- **NFR-LGCP-CON-001**: LGAP 노드와 동일한 코드 구조 및 네이밍 패턴을 따라야 한다
- **NFR-LGCP-CON-002**: 메타데이터 키 접두사는 `lgcp_`를 사용해야 한다 (`lgcp_source`, `lgcp_node_id`, `lgcp_command`)

---

## 6. 기술 접근 / 아키텍처

### 6.1 파일 구조

```
internal/node/
├── lgcp.go          # 신규: lgcpNodeBase, LGCPStatusNode, LGCPControlNode, LGCPNode
├── lgcp_test.go     # 신규: 3종 노드 테스트
├── errors.go        # 수정: LGCP 에러 정의 추가
├── registry.go      # 수정: LGCP 3종 노드 등록 추가
└── lgap.go          # 변경 없음 (참조 패턴)
```

### 6.2 클래스 구조

```
lgcpNodeBase
├── *BaseNode (임베딩)
├── lgcpCfg      LGCPNodeConfig
├── resolver     AgentResolver
├── transport    AgentTransport
├── agent        agent.Agent
├── timeout      time.Duration
├── mu           sync.RWMutex
├── configure()
├── initAgent()
├── callAgentProcess()
├── shutdown()
└── applyLGCPOverrides()

LGCPStatusNode
├── lgcpNodeBase (임베딩)
├── pollInterval time.Duration
├── sourceCh     chan message.Message
├── stopCh       chan struct{}
├── pollOnce     sync.Once
├── Configure(), Init(), Process(), Shutdown(), SourceCh()
└── pollLoop()

LGCPControlNode
├── lgcpNodeBase (임베딩)
├── Configure(), Init(), Process(), Shutdown()

LGCPNode
├── lgcpNodeBase (임베딩)
├── pollInterval time.Duration
├── sourceCh     chan message.Message
├── stopCh       chan struct{}
├── pollOnce     sync.Once
├── Configure(), Init(), Process(), Shutdown(), SourceCh()
└── pollLoop()
```

### 6.3 LGCPNodeConfig

```go
type LGCPNodeConfig struct {
    AgentRef       string `json:"agent_ref"`       // 필수: LGCP 에이전트 이름/ID
    DefaultAddress string `json:"default_address"` // 선택: 기본 실내기 주소 (hex)
    PollInterval   string `json:"poll_interval"`   // 선택: 폴링 간격 (기본 "30s")
    Timeout        string `json:"timeout"`         // 선택: Process 타임아웃 (기본 "5s")
    PollCommand    string `json:"poll_command"`    // 선택: 폴링 커맨드 (기본 "get_stats", "get_recent" 가능)
    RecentCount    int    `json:"recent_count"`    // 선택: get_recent 시 프레임 수 (기본 10)
}
```

### 6.4 커맨드 빌더

```go
// buildLGCPStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
// poll_command가 "get_recent"이면 count를 포함한다.
func buildLGCPStatusCommand(cfg LGCPNodeConfig) ([]byte, error)

// buildLGCPControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command"가 있으면 직접 전달, 없으면 제어 키에서 set_multiple 구성.
// address가 필수이며, 없으면 default_address를 사용, 둘 다 없으면 에러.
func buildLGCPControlCommand(msg message.Message, cfg LGCPNodeConfig) ([]byte, error)

// hasLGCPControlKeys 는 payload에 제어 키가 있는지 확인한다.
func hasLGCPControlKeys(msg message.Message) bool

// applyLGCPOverrides 는 메시지 payload에서 address, timeout, poll_command, count를 오버라이드한다.
func applyLGCPOverrides(msg message.Message, cfg LGCPNodeConfig) LGCPNodeConfig
```

### 6.5 LGCP 커맨드 JSON 형식

상태 조회:
```json
{"command": "get_stats"}
{"command": "get_recent", "count": 10}
```

제어 (직접 커맨드):
```json
{"command": "set_power", "address": "00000001", "params": {"power": "ON"}}
```

제어 (set_multiple):
```json
{"command": "set_multiple", "address": "00000001", "params": {"power": "ON", "temperature": 24}}
```

---

## 7. 추적성 (Traceability)

| 요구사항 | 파일 | 테스트 |
|---------|------|--------|
| REQ-LGCP-NODE-001~005 | internal/node/lgcp.go (lgcpNodeBase) | lgcp_test.go |
| REQ-LGCP-NODE-006~012 | internal/node/lgcp.go (LGCPStatusNode) | lgcp_test.go |
| REQ-LGCP-NODE-013~019 | internal/node/lgcp.go (LGCPControlNode) | lgcp_test.go |
| REQ-LGCP-NODE-020~025 | internal/node/lgcp.go (LGCPNode) | lgcp_test.go |
| REQ-LGCP-NODE-026 | internal/node/errors.go | lgcp_test.go |
| REQ-LGCP-NODE-027 | internal/node/registry.go | registry_test.go |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-06*
*작성: MoAI SPEC Builder (manager-spec)*
