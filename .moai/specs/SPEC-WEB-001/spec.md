---
id: SPEC-WEB-001
version: "1.7.0"
status: in_progress
created: "2026-03-07"
updated: "2026-03-10"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-07 | 1.0.0 | 초기 SPEC 작성 - 에이전트 통계 버그 수정 + 컴포넌트별 로그 레벨 제어 |
| 2026-03-07 | 1.1.0 | Module 4 추가: 플로우 노드 통계 + 로그 레벨 UI. M1 근본 원인 백엔드로 수정 |
| 2026-03-07 | 1.2.0 | Module 5 추가: 캔버스 런타임 통계. M4 In/Out 분리 표시 + 실시간 갱신. 백엔드 포트 카운터/브릿지 버그 수정 |
| 2026-03-08 | 1.3.0 | Module 7 추가: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링. SPEC-OBS-004 연동 |
| 2026-03-09 | 1.4.0 | Module 8 추가: 동적 포트 시스템. 노드 타입/설정 기반 포트 동적 계산, 브릿지 방향별 포트, 스위치 라우트별 출력 포트 |
| 2026-03-09 | 1.5.0 | Module 9 추가: 에러 포트 타입 지원. 프론트엔드 PortDef/NodeTypeDefinition에 'error' direction 추가, CustomNode 에러 포트 렌더링(빨간색, 하단 배치), NodeHandle 에러 포트 색상 |
| 2026-03-09 | 1.6.0 | Module 10 추가: Handle ID 접두사 제거. 포트 이름을 Handle ID로 직접 사용하여 프론트엔드/백엔드 Handle↔Edge 매핑 간소화. Module 9 실제 구현 반영(에러 포트 Right position + offset 배치). PropertyPanel Port 타입에 'error' 추가 |
| 2026-03-09 | 1.7.0 | Module 11 추가: 리스트 정렬 기능. FlowListPage/AgentListPage 컬럼 정렬 지원, 이름 기본 정렬, 백엔드 ListOptions.Sort 구현, 프론트엔드 정렬 UI 컴포넌트 |

---

# SPEC-WEB-001: 웹 UI 에이전트 통계 표시 버그 수정 및 디버깅 레벨 설정

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 Web Dashboard에서 에이전트 관련 두 가지 이슈를 해결한다:

1. **에이전트 목록 통계 버그 수정**: 에이전트 목록 페이지에서 uptime, messages_in, messages_out 통계가 항상 빈 값(`-`)으로 표시되는 버그 (근본 원인: 백엔드 ListAgents가 `detail` 파라미터 무시)
2. **컴포넌트별 디버깅 레벨 설정**: 현재 글로벌 로그 레벨 설정만 지원하는데, 에이전트/노드 단위의 개별 로그 레벨 제어 기능 추가
3. **플로우 노드 통계 및 로그 레벨 표시**: 플로우 목록에서 노드별 In/Out 메시지 통계와 개별 로그 레벨 제어 기능 추가
4. **캔버스 런타임 통계 표시**: React Flow 에디터 캔버스에서 노드별 실시간 상태(running/stopped/error)와 In/Out 메시지 수 표시
5. **백엔드 포트 카운터 및 브릿지 버그 수정**: engine.go 포트 카운터 초기화 누락, bridge.go msgCh/Process 반환값 버그 수정
6. **로그 뷰어 컴포넌트/소스 필드 표시 및 필터링**: WebSocket `log.entry` 페이로드의 `component`/`source` 필드를 로그 뷰어에 표시하고, 소스 종류(agent/node/flow 등) 및 컴포넌트 이름으로 필터링 기능 추가
7. **동적 포트 시스템**: 플로우 노드의 입출력 포트가 설정한 갯수/방향과 일치하지 않는 버그 수정. 노드 타입과 설정(브릿지 방향, 스위치 라우트 등)에 따라 포트를 동적으로 계산하고, 설정 변경 시 포트를 재계산하는 시스템 구현
8. **에러 포트 타입 지원**: 프론트엔드가 백엔드의 `error` 포트 direction을 인식하지 못해 에러 포트가 렌더링되지 않는 버그 수정. `PortDef`/`NodeTypeDefinition` 타입에 `'error'` 추가, CustomNode에서 에러 포트를 빨간색으로 오른쪽(출력 포트 아래)에 렌더링, NodeHandle에 에러 포트 색상과 offset 위치 지원 추가
9. **Handle ID 접두사 제거**: Handle ID에서 불필요한 접두사(`in-`, `out-`, `err-`)를 제거하고 포트 이름을 그대로 Handle ID로 사용. 포트 direction이 이미 별도 필드로 구분되므로 접두사 불필요. 프론트엔드(CustomNode, NodeHandle, PropertyPanel)와 백엔드(flow_adapter.go)의 Handle↔Edge 매핑 간소화
10. **리스트 정렬 기능**: 플로우 목록과 에이전트 목록 페이지에서 각 컬럼 헤더를 클릭하여 정렬 가능. 이름(name)을 기본 정렬로 사용하며, 백엔드 `ListOptions.Sort` 파라미터를 실제 구현하여 서버 사이드 정렬 지원

### 1.2 기술 환경

- **프론트엔드**: TypeScript 5.x, React 19.x, Vite 6.x, Tailwind CSS 4.x
- **백엔드**: Go, xflowd 데몬
- **패키지 경로**: `web/src/` (프론트엔드), `internal/` (백엔드)
- **상태 관리**: Zustand 5.x (클라이언트 상태), @tanstack/react-query 5.x (서버 상태)
- **HTTP 클라이언트**: axios
- **UI 컴포넌트**: shadcn/ui + Radix UI
- **의존 SPEC**:
  - SPEC-API-001: REST API 엔드포인트
  - SPEC-OBS-001: 관찰성 시스템 (LevelManager 인터페이스)
  - SPEC-OBS-004: 로그 WebSocket 페이로드 확장 (component/source 필드)
  - SPEC-AGENT-001: Agent 관리

### 1.3 설계 원칙

- **최소 변경 원칙**: 기존 백엔드 LevelManager 인터페이스를 최대한 활용하여 새로운 추상화를 최소화한다
- **프론트엔드+백엔드 동시 수정**: Issue 1은 프론트엔드(`detail=summary` 추가)와 백엔드(`ListOptions.Detail` 파라미터 전달) 양쪽 수정이 필요하다
- **점진적 기능 추가**: 글로벌 로그 레벨 기능을 유지하면서 컴포넌트별 로그 레벨 기능을 추가한다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Module 1: 에이전트 목록 API `detail` 파라미터 전달 버그 수정 (프론트엔드 + 백엔드)
- Module 2: 컴포넌트별 로그 레벨 API 엔드포인트 추가 (백엔드)
- Module 3: 에이전트 컴포넌트별 로그 레벨 UI 추가 (프론트엔드)
- Module 4: 플로우 노드 통계 + 로그 레벨 UI 추가 (프론트엔드) — In/Out 분리 표시, 실시간 갱신
- Module 5: 캔버스 런타임 통계 표시 (프론트엔드) — RuntimeStatsContext, CustomNode In/Out 표시
- Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 + 소스 종류 필터 + 컴포넌트 이름 필터 (프론트엔드)
- Module 8: 동적 포트 시스템 — 노드 타입/설정 기반 포트 동적 계산 + 브릿지 방향별 포트 + 스위치 라우트별 출력 포트 + 설정 변경 시 포트 재계산 (프론트엔드 + 백엔드)
- Module 9: 에러 포트 타입 지원 — PortDef/NodeTypeDefinition에 `'error'` direction 추가 + CustomNode 에러 포트 빨간색 오른쪽 렌더링 + NodeHandle 에러 포트 색상/offset (프론트엔드)
- Module 10: Handle ID 접두사 제거 — 포트 이름을 Handle ID로 직접 사용 + flow_adapter.go TrimPrefix 제거 + PropertyPanel Port 타입 'error' 추가 (프론트엔드 + 백엔드)
- Module 11: 리스트 정렬 기능 — FlowListPage/AgentListPage 정렬 가능 컬럼 헤더 UI + 백엔드 sort 파라미터 처리 구현 (프론트엔드 + 백엔드)
- 백엔드 버그 수정: engine.go 포트 카운터 초기화, bridge.go msgCh/Process 반환값

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- 로그 레벨 영속화 (서버 재시작 시 초기화됨)
- 로그 레벨 변경 이력 감사(audit) 로그
- 실시간 WebSocket 기반 로그 레벨 동기화

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| LevelManager | Go 백엔드의 `observe.LevelManager` 인터페이스. 컴포넌트별 slog.Level을 관리한다 |
| Component | 로그 레벨을 개별 설정할 수 있는 단위. `agent.{name}`, `node.{name}` 형식 |
| Pattern | 와일드카드(`*`)를 포함하는 컴포넌트 매칭 문자열. 예: `agent.*` |
| detail parameter | 에이전트 목록 API의 쿼리 파라미터. `summary`일 때 stats, uptime, health 포함 |
| Global Log Level | 시스템 전체에 적용되는 기본 로그 레벨 |
| Component Log Level | 특정 에이전트 또는 노드에만 적용되는 개별 로그 레벨 |
| Source | `classifySource()`가 분류한 로그 출처 카테고리. agent, node, flow, api, engine, system 중 하나 |
| Source Filter | 로그 뷰어에서 source 카테고리별 토글 필터 (다중 선택 가능) |
| Component Filter | 로그 뷰어에서 component 이름 텍스트 검색 필터 (포함 매칭) |
| PortDef | 프론트엔드 포트 정의 인터페이스. `{ name: string, direction: 'input' \| 'output' }` |
| computePortsForNode | 노드 타입과 설정(config)을 기반으로 포트를 동적 계산하는 함수 |
| BridgeDirection | 브릿지 노드의 데이터 방향. `BridgeIn`(agent->flow), `BridgeOut`(flow->agent), `BridgeInOut`, `BridgeRequestReply` |
| Switch Routes | 스위치 노드의 라우팅 경로 목록. 각 라우트가 별도 출력 포트로 매핑됨 |
| getDefaultPorts | 노드 타입의 정적 기본 포트를 반환하는 기존 함수. Module 8에서 `computePortsForNode`로 대체/보완 |
| Error Port | 백엔드 `PortError` direction을 가진 포트. 노드의 에러 출력 경로를 나타낸다. 프론트엔드에서 빨간색으로 노드 하단에 렌더링됨 |
| PortDirection (확장) | 프론트엔드 포트 방향. Module 9에서 기존 `'input' \| 'output'`에 `'error'`가 추가되어 `'input' \| 'output' \| 'error'`로 확장됨 |
| Handle ID | React Flow Handle 컴포넌트의 `id` prop. Module 10에서 포트 이름(`name`)을 그대로 사용하도록 변경. 이전에는 방향 접두사(`in-`, `out-`, `err-`)를 포함했음 |
| Handle↔Edge 매핑 | React Flow Edge의 `sourceHandle`/`targetHandle`과 Handle `id`의 대응 관계. 접두사 제거로 포트 이름 = Handle ID = Edge Handle 값으로 단순화됨 |
| Sort Parameter | 목록 API의 `sort` 쿼리 파라미터. `field:direction` 형식 (예: `name:asc`, `created_at:desc`) |
| Sortable Header | 클릭하여 정렬 방향을 변경할 수 있는 테이블 컬럼 헤더. 정렬 방향 인디케이터(▲/▼)를 포함 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: 백엔드 `GET /agents` API에서 `detail` 쿼리 파라미터를 `ListOptions`에 전달하고, `agentToHandlerInfo`에서 detail 값에 따라 stats, uptime, health 필드를 포함한다
- A-002: `observe.LevelManager` 인터페이스의 `SetLevel`, `GetLevel`, `SetLevelByPattern`, `ListLevels` 메서드가 정상 동작한다
- A-003: `internal/api/handler/monitor.go`의 기존 `PUT /monitor/loglevel` 핸들러가 글로벌 로그 레벨 변경 기능을 올바르게 수행한다
- A-004: 프론트엔드 `AgentInfo` 타입에 이미 `stats`, `uptime`, `health` optional 필드가 정의되어 있다
- A-007: SPEC-OBS-004 구현이 완료되어 백엔드 WebSocket `log.entry` 페이로드에 `component`와 `source` 필드가 포함되어 전송된다
- A-008: `classifySource()` 함수가 반환하는 source 카테고리는 `agent`, `node`, `flow`, `api`, `engine`, `system` 6가지이다

- A-009: `web/src/config/nodeSchemas.ts`의 `NODE_SCHEMAS` 레지스트리에 각 노드 타입의 기본 포트 정의(`defaultPorts`)가 존재하며, `getDefaultPorts(nodeType)` 함수가 이를 반환한다
- A-010: 브릿지 노드의 direction 설정은 노드 config 객체의 `direction` 필드에 저장되며, 값은 `in`, `out`, `inout`, `request_reply` 중 하나이다
- A-011: 스위치 노드의 라우트 설정은 노드 config 객체의 `routes` 배열에 저장되며, 각 라우트에 `name` 필드가 존재한다
- A-012: 백엔드 `pkg/flow/node.go`의 `NewNodeDef()` 팩토리가 항상 기본 `[in, out]` 포트를 생성하며, 노드 타입별 포트 커스터마이징 로직이 없다

### 3.2 운영 가정

- A-005: 컴포넌트별 로그 레벨은 서버 메모리에만 저장되며, 서버 재시작 시 기본 레벨로 초기화된다
- A-006: 동시에 활성화되는 컴포넌트별 로그 레벨 오버라이드 수는 최대 100개 이내이다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: 에이전트 목록 통계 버그 수정 (P0 - 버그 수정)

#### REQ-WEB-001-01-01 (Event-Driven)
**WHEN** 에이전트 목록 페이지가 로드될 때, **THEN** `GET /agents` API 호출 시 `detail=summary` 쿼리 파라미터를 포함하여 stats, uptime, health 데이터를 함께 요청해야 한다.

#### REQ-WEB-001-01-02 (State-Driven)
**IF** 에이전트 목록 API 응답에 `stats` 필드가 포함된 상태 **THEN** 에이전트 테이블의 "업타임" 컬럼에 `uptime` 값을, "메시지 (IN/OUT)" 컬럼에 `stats.messages_in / stats.messages_out` 값을 표시해야 한다.

#### REQ-WEB-001-01-03 (State-Driven)
**IF** 에이전트의 `stats` 필드가 없거나 null인 상태 **THEN** 해당 통계 컬럼에 `-` 기본값을 표시해야 한다.

### 4.2 Module 2: 백엔드 컴포넌트별 로그 레벨 API (P1 - 신규 기능)

#### REQ-WEB-001-02-01 (Event-Driven)
**WHEN** `PUT /monitor/loglevel/{component}` 요청을 수신하면, **THEN** `LevelManager.SetLevel(component, level)`을 호출하여 해당 컴포넌트의 로그 레벨을 설정하고, 성공 응답을 반환해야 한다.

#### REQ-WEB-001-02-02 (Event-Driven)
**WHEN** `GET /monitor/loglevel` 요청을 수신하면, **THEN** `LevelManager.ListLevels()`를 호출하여 모든 컴포넌트의 로그 레벨 맵(`component -> level`)과 현재 기본 레벨을 반환해야 한다.

#### REQ-WEB-001-02-03 (Event-Driven)
**WHEN** `DELETE /monitor/loglevel/{component}` 요청을 수신하면, **THEN** 해당 컴포넌트의 개별 로그 레벨 오버라이드를 제거하고 기본 레벨로 리셋해야 한다.

#### REQ-WEB-001-02-04 (Unwanted)
시스템은 유효하지 않은 로그 레벨 값(debug, info, warn, error 외)에 대해 설정을 **허용하지 않아야 한다**. 400 Bad Request 응답을 반환해야 한다.

#### REQ-WEB-001-02-05 (Ubiquitous)
시스템은 **항상** 기존 `PUT /monitor/loglevel` 글로벌 로그 레벨 변경 API의 하위 호환성을 유지해야 한다.

### 4.3 Module 3: 프론트엔드 컴포넌트별 로그 레벨 UI (P1 - 신규 기능)

#### REQ-WEB-001-03-01 (Event-Driven)
**WHEN** 에이전트 상세 패널이 열리면, **THEN** 해당 에이전트의 현재 로그 레벨을 표시하는 드롭다운 컨트롤을 통계 탭 또는 별도 영역에 제공해야 한다.

#### REQ-WEB-001-03-02 (Event-Driven)
**WHEN** 에이전트 로그 레벨 드롭다운 값을 변경하면, **THEN** `PUT /monitor/loglevel/agent.{agent-id}` API를 호출하여 해당 에이전트의 로그 레벨을 변경하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-03-03 (Event-Driven)
**WHEN** 에이전트 로그 레벨을 "기본값" 옵션으로 변경하면, **THEN** `DELETE /monitor/loglevel/agent.{agent-id}` API를 호출하여 개별 오버라이드를 제거해야 한다.

#### REQ-WEB-001-03-04 (Event-Driven)
**WHEN** 설정 페이지의 시스템 탭이 로드되면, **THEN** `GET /monitor/loglevel` API를 호출하여 글로벌 기본 레벨과 컴포넌트별 오버라이드 목록을 표시해야 한다.

#### REQ-WEB-001-03-05 (Event-Driven)
**WHEN** 설정 페이지에서 컴포넌트별 오버라이드의 "리셋" 버튼을 클릭하면, **THEN** `DELETE /monitor/loglevel/{component}` API를 호출하여 해당 오버라이드를 제거하고 목록을 갱신해야 한다.

#### REQ-WEB-001-03-06 (State-Driven)
**IF** 사용자 역할이 Viewer인 경우 **THEN** 로그 레벨 변경 드롭다운과 리셋 버튼을 비활성화(disabled)해야 한다.

### 4.4 Module 4: 플로우 노드 통계 및 로그 레벨 UI (P1 - 신규 기능)

#### REQ-WEB-001-04-01 (Event-Driven)
**WHEN** 플로우 목록 테이블의 행을 클릭하면, **THEN** 해당 플로우의 노드 인스턴스 목록을 `GET /flows/{id}/nodes` API로 조회하여 확장 패널에 표시해야 한다.

#### REQ-WEB-001-04-02 (State-Driven)
**IF** 플로우 확장 패널이 열린 상태 **THEN** 각 노드의 이름, 타입, 상태(running/stopped/error 배지), 포트별 In/Out 메시지 합계(입력/출력 아이콘 분리)를 테이블 형태로 표시해야 한다.

#### REQ-WEB-001-04-03 (Event-Driven)
**WHEN** 플로우 확장 패널의 노드별 로그 레벨 드롭다운을 변경하면, **THEN** `PUT /monitor/loglevel/node.{node-name}` API를 호출하여 해당 노드의 로그 레벨을 변경하고, 성공/실패 피드백을 표시해야 한다.

#### REQ-WEB-001-04-04 (Event-Driven)
**WHEN** 노드 로그 레벨을 "기본값" 옵션으로 변경하면, **THEN** `DELETE /monitor/loglevel/node.{node-name}` API를 호출하여 개별 오버라이드를 제거해야 한다.

#### REQ-WEB-001-04-05 (State-Driven)
**IF** 플로우가 배포되지 않아 노드 정보가 없는 상태 **THEN** "노드 정보가 없습니다. 플로우를 배포하면 노드가 표시됩니다." 안내 메시지를 표시해야 한다.

#### REQ-WEB-001-04-06 (State-Driven)
**IF** 플로우가 running 상태 **THEN** 노드 인스턴스 데이터를 3초 간격으로 자동 갱신하여 실시간 통계를 반영해야 한다. 플로우가 running이 아닌 상태에서는 자동 갱신을 비활성화해야 한다.

### 4.5 Module 5: 캔버스 런타임 통계 표시 (P1 - 신규 기능)

#### REQ-WEB-001-05-01 (State-Driven)
**IF** 에디터 페이지에서 플로우가 running 상태 **THEN** React Flow 캔버스의 각 CustomNode에 실시간 상태 표시등(running=초록, stopped=회색, error=빨강)과 In/Out 메시지 수를 표시해야 한다.

#### REQ-WEB-001-05-02 (State-Driven)
**IF** 플로우가 running 상태 **THEN** 에디터 페이지는 3초 간격으로 `GET /flows/{id}/nodes` API를 폴링하여 런타임 통계를 갱신해야 한다.

#### REQ-WEB-001-05-03 (Unwanted)
런타임 통계 데이터는 에디터 스토어(editorStore)의 노드 데이터에 직접 저장**하지 않아야 한다**. isDirty 플래그 및 undo/redo 이력에 영향을 주지 않도록 별도의 React Context(RuntimeStatsContext)를 통해 전달해야 한다.

#### REQ-WEB-001-05-04 (State-Driven)
**IF** 플로우가 running이 아닌 상태 **THEN** CustomNode의 런타임 통계 표시를 숨기고 폴링을 비활성화해야 한다.

### 4.7 Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링 (P1 - 신규 기능)

#### REQ-WEB-001-07-01 (Ubiquitous)
시스템은 **항상** WebSocket `log.entry` 페이로드의 `component`와 `source` 필드를 LogEntry 데이터에 포함하여 저장해야 한다. `component`와 `source` 필드가 없는 로그 항목은 각각 빈 문자열과 `system`을 기본값으로 사용한다.

#### REQ-WEB-001-07-02 (Event-Driven)
**WHEN** WebSocket `log.entry` 메시지를 수신하면, **THEN** `component`와 `source` 필드를 페이로드에서 추출하여 LogEntry 객체에 포함해야 한다.

#### REQ-WEB-001-07-03 (State-Driven)
**IF** 로그 항목에 `component`와 `source` 필드가 있는 상태 **THEN** 로그 뷰어의 각 행에서 레벨 배지 옆에 source 카테고리 배지(색상 구분)와 component 이름을 표시해야 한다.

#### REQ-WEB-001-07-04 (Event-Driven)
**WHEN** 로그 뷰어 툴바의 source 필터 버튼(agent/node/flow/api/engine/system)을 토글하면, **THEN** 해당 source 카테고리의 로그만 표시하거나, 모든 필터가 비활성화되면 전체 로그를 표시해야 한다. 다중 선택이 가능해야 한다.

#### REQ-WEB-001-07-05 (Event-Driven)
**WHEN** 로그 뷰어 툴바의 컴포넌트 검색 입력에 텍스트를 입력하면, **THEN** component 이름에 해당 텍스트가 포함된(대소문자 구분 없음) 로그 항목만 표시해야 한다. 입력은 300ms 디바운스 처리한다.

#### REQ-WEB-001-07-06 (State-Driven)
**IF** 레벨 필터, source 필터, 컴포넌트 검색이 동시에 활성화된 상태 **THEN** 세 가지 필터 조건을 모두 AND 결합하여 적용해야 한다. 필터 결과 건수를 툴바에 실시간으로 표시해야 한다.

#### REQ-WEB-001-07-07 (Unwanted)
source 필터와 컴포넌트 검색 기능은 기존 가상화 렌더링 성능에 영향을 주지 **않아야 한다**. MAX_ENTRIES(10,000건) 상태에서도 필터링이 16ms(60fps) 이내에 완료되어야 한다.

### 4.8 Module 8: 동적 포트 시스템 (P0 - 버그 수정)

#### REQ-WEB-001-08-01 (Event-Driven)
**WHEN** 노드를 생성하거나 노드 설정이 변경될 때, **THEN** `computePortsForNode(nodeType, config)` 함수를 호출하여 노드 타입과 현재 설정(config)을 기반으로 포트 목록을 동적으로 계산해야 한다. 기존 `getDefaultPorts(nodeType)`의 정적 포트 대신 설정 기반 포트를 반환해야 한다.

#### REQ-WEB-001-08-02 (State-Driven)
**IF** 브릿지 노드의 direction이 `in`(agent->flow)인 상태 **THEN** 출력 포트(output)만 표시해야 한다. **IF** direction이 `out`(flow->agent)인 상태 **THEN** 입력 포트(input)만 표시해야 한다. **IF** direction이 `inout` 또는 `request_reply`인 상태 **THEN** 입력 포트와 출력 포트를 모두 표시해야 한다. direction이 미설정인 경우 기존 기본값(input+output)을 유지해야 한다.

#### REQ-WEB-001-08-03 (State-Driven)
**IF** 스위치 노드에 라우트가 설정된 상태 **THEN** 입력 포트 1개와 각 라우트별 출력 포트(라우트 이름으로 명명) + 기본(default) 출력 포트를 표시해야 한다. **IF** 라우트가 미설정인 상태 **THEN** 기본 입출력 포트(`in`, `out`)를 표시해야 한다.

#### REQ-WEB-001-08-04 (Event-Driven)
**WHEN** 에디터에서 노드의 설정(config)이 변경되면(예: 브릿지 direction 변경, 스위치 라우트 추가/삭제), **THEN** `computePortsForNode(nodeType, updatedConfig)`를 호출하여 포트를 재계산하고, 에디터 스토어의 해당 노드 `data.ports`를 업데이트해야 한다. 기존 엣지(연결선) 중 삭제된 포트에 연결된 엣지는 자동으로 제거해야 한다.

#### REQ-WEB-001-08-05 (Event-Driven)
**WHEN** 백엔드에서 브릿지 타입의 `NewNodeDef()`를 생성할 때, **THEN** direction 설정에 따라 적절한 Inputs/Outputs 포트를 생성해야 한다. `BridgeIn`은 Outputs만, `BridgeOut`은 Inputs만, `BridgeInOut`/`BridgeRequestReply`는 Inputs와 Outputs 모두를 기본 포트로 설정해야 한다.

### 4.9 Module 9: 에러 포트 타입 지원 (P0 - 버그 수정)

#### REQ-WEB-001-09-01 (State-Driven)
**IF** 프론트엔드가 백엔드로부터 포트 데이터를 수신한 상태 **THEN** `'input'`과 `'output'` 외에 `'error'`를 유효한 포트 direction으로 지원해야 한다. `PortDef` 타입과 `NodeTypeDefinition.ports[].direction` 타입에 `'error'`를 추가해야 한다.

#### REQ-WEB-001-09-02 (State-Driven)
**IF** 노드에 에러 포트가 존재하는 상태 **THEN** CustomNode 컴포넌트는 에러 포트를 출력 포트와 시각적으로 구분하여 빨간색으로 노드 오른쪽(출력 포트 아래)에 offset 배치하여 렌더링해야 한다. 출력 포트와 에러 포트가 모두 있는 경우 `((i+1)/(total+1))*100%` 공식으로 균등 분배한다.

#### REQ-WEB-001-09-03 (State-Driven)
**IF** NodeHandle이 에러 포트를 렌더링하는 상태 **THEN** 핸들은 빨간색(`bg-red-500`)으로 표시되어 출력 포트(초록색)와 입력 포트(파란색)와 구분되어야 한다.

#### REQ-WEB-001-09-04 (Event-Driven)
**WHEN** 에러 포트를 지원하는 노드 타입에 대해 `computePortsForNode()`가 호출될 때, **THEN** 반환되는 포트 배열에 에러 포트 정의(`direction: 'error'`)가 포함되어야 한다.

#### REQ-WEB-001-09-05 (State-Driven)
**IF** 에러 포트가 없는 기존 노드를 로드하는 상태 **THEN** 에러 포트 핸들 없이 기존과 동일하게 정상 렌더링되어야 한다.

### 4.10 Module 10: Handle ID 접두사 제거 (P0 - 리팩토링)

#### REQ-WEB-001-10-01 (Ubiquitous)
시스템은 **항상** 포트 이름(`name`)을 React Flow Handle ID로 직접 사용해야 한다. 포트 방향 접두사(`in-`, `out-`, `err-`)를 Handle ID에 추가**하지 않아야 한다**. 포트 direction은 `PortDef.direction` 필드로 이미 구분되므로 접두사는 불필요하다.

#### REQ-WEB-001-10-02 (State-Driven)
**IF** 백엔드가 React Flow Edge를 생성하는 상태 **THEN** `sourceHandle`에 Wire의 `SourcePort`를 그대로 설정하고, `targetHandle`에 Wire의 `TargetPort`를 그대로 설정해야 한다. 접두사를 추가하거나 제거하는 변환을 적용**하지 않아야 한다**.

#### REQ-WEB-001-10-03 (State-Driven)
**IF** 프론트엔드가 React Flow Edge를 백엔드 Wire로 변환하는 상태 **THEN** Edge의 `sourceHandle`을 Wire의 `source_port`로 직접 매핑하고, `targetHandle`을 `target_port`로 직접 매핑해야 한다. `TrimPrefix` 등의 변환을 적용**하지 않아야 한다**.

#### REQ-WEB-001-10-04 (State-Driven)
**IF** PropertyPanel에서 포트를 관리하는 상태 **THEN** Port 타입이 `'input' | 'output' | 'error'` 세 가지 direction을 모두 지원해야 한다. 삭제된 포트에 연결된 엣지 정리 시 포트 이름(`name`)으로 직접 비교해야 한다.

### 4.11 Module 11: 리스트 정렬 기능 (P1 - 신규 기능)

#### REQ-WEB-001-11-01 (Ubiquitous)
시스템은 **항상** 리스트 페이지(FlowListPage, AgentListPage)의 데이터를 정렬된 상태로 표시해야 한다. 명시적 정렬 요청이 없는 경우 기본 정렬은 `name:asc` (이름 오름차순)이다.

#### REQ-WEB-001-11-02 (Event-Driven)
**WHEN** 사용자가 정렬 가능한 컬럼 헤더를 클릭하면 **THEN** 해당 필드 기준으로 정렬 방향이 토글되어야 한다 (asc → desc → asc). 현재 정렬 기준과 방향을 시각적으로 표시해야 한다 (▲ 오름차순, ▼ 내림차순).

#### REQ-WEB-001-11-03 (State-Driven)
**IF** FlowListPage가 표시되는 상태 **THEN** 이름(name), 상태(status), 생성일(created_at), 수정일(updated_at) 컬럼이 정렬 가능해야 한다. 노드(node_count) 컬럼과 액션 컬럼은 정렬 불가이다.

#### REQ-WEB-001-11-04 (State-Driven)
**IF** AgentListPage가 표시되는 상태 **THEN** 이름(name), 타입(type), 상태(status) 컬럼이 정렬 가능해야 한다. 업타임, 메시지, 액션 컬럼은 정렬 불가이다.

#### REQ-WEB-001-11-05 (Event-Driven)
**WHEN** 사용자가 정렬 기준을 변경하면 **THEN** 프론트엔드는 `sort` 쿼리 파라미터를 `field:direction` 형식으로 백엔드 API에 전달해야 한다 (예: `?sort=name:asc`).

#### REQ-WEB-001-11-06 (State-Driven)
**IF** 백엔드가 `sort` 쿼리 파라미터를 수신한 상태 **THEN** `FlowServiceAdapter.ListFlows`와 `AgentServiceAdapter.ListAgents`는 결과를 페이지네이션 적용 전에 해당 필드와 방향으로 정렬해야 한다.

### 4.6 백엔드 버그 수정 (P0 - 버그 수정)

#### REQ-WEB-001-06-01 (Unwanted)
엔진의 포트 카운터 초기화 시 모든 포트(입력/출력/에러)에 대해 `atomic.Int64` 카운터를 0으로 초기화**해야 한다**. 일부 포트만 초기화되어 나머지 포트의 메시지 카운트가 0으로 보고되는 버그를 수정한다.

#### REQ-WEB-001-06-02 (Unwanted)
Bridge 노드의 `Process()` 메서드는 정상 처리 시 nil을 반환**해야 한다**. 불필요한 에러 반환으로 메시지 처리가 중단되는 버그를 수정한다.

---

## 5. Specifications (기술 사양)

### 5.1 Module 1: 에이전트 목록 통계 버그 수정

**근본 원인**: 백엔드 `ListAgents`에서 `detail` 쿼리 파라미터를 `ListOptions`에 전달하지 않고, `agentToHandlerInfo(ag, "")`로 빈 문자열을 하드코딩하여 stats/uptime/health가 항상 생략됨.

**수정 파일 (프론트엔드)**:
- `web/src/services/api/agentService.ts`: `getAgents()` 호출에 `detail: 'summary'` 파라미터 추가

**수정 파일 (백엔드)**:
- `internal/api/dto/request.go`: `ListOptions` 구조체에 `Detail` 필드 추가
- `internal/api/handler/flow.go`: `parseListOptions()`에 `ctx.Query("detail")` 파싱 추가
- `internal/api/service/agent_adapter.go`: `agentToHandlerInfo(ag, "")` → `agentToHandlerInfo(ag, opts.Detail)` 변경

### 5.2 Module 2: 백엔드 API 엔드포인트

| Method | Endpoint | Request Body | Response | 설명 |
|--------|----------|-------------|----------|------|
| GET | `/monitor/loglevel` | - | `{ default_level: string, components: Record<string, string> }` | 전체 로그 레벨 조회 |
| PUT | `/monitor/loglevel` | `{ level: string }` | `{ level: string }` | 글로벌 기본 레벨 변경 (기존) |
| PUT | `/monitor/loglevel/{component}` | `{ level: string }` | `{ component: string, level: string }` | 컴포넌트별 레벨 설정 |
| DELETE | `/monitor/loglevel/{component}` | - | `{ component: string }` | 컴포넌트 레벨 리셋 |

**유효한 level 값**: `debug`, `info`, `warn`, `error`

**component 형식 예시**:
- `agent.modbus-001` - 특정 에이전트
- `node.transform-1` - 특정 노드
- `engine.scheduler` - 엔진 컴포넌트

### 5.3 Module 3: 프론트엔드 API 서비스 확장

**파일**: `web/src/services/api/monitorService.ts` - 확장

```typescript
// 기존 함수 유지
export async function setLogLevel(level: string): Promise<void>;

// 새로 추가
export async function getLogLevels(): Promise<LogLevelInfo>;
export async function setComponentLogLevel(component: string, level: string): Promise<void>;
export async function resetComponentLogLevel(component: string): Promise<void>;
```

**새 타입 정의**:
```typescript
interface LogLevelInfo {
  default_level: string;
  components: Record<string, string>;
}
```

### 5.4 Module 4: 플로우 노드 통계 및 로그 레벨 UI

**신규 파일**: `web/src/pages/flows/FlowDetailPanel.tsx`
- 플로우 확장 패널로 노드 인스턴스 테이블 표시
- 각 노드: 이름, 타입, 상태 배지(NodeStateBadge), **In/Out 메시지 분리 표시(PortStats)**, 로그 레벨 드롭다운(NodeLogLevelSelect)
- In 표시: `ArrowDownToLine` 아이콘 + input 방향 포트 메시지 합계
- Out 표시: `ArrowUpFromLine` 아이콘 + output 방향 포트 메시지 합계
- `useFlowNodes(flowId, refetchInterval)` 훅으로 `GET /flows/{id}/nodes` 호출
- `useFlowStatus(flowId)`로 running 상태 감지 → running 시 3초 자동 갱신
- `node.{name}` 키로 monitor API를 통한 로그 레벨 제어

**수정 파일**: `web/src/pages/flows/FlowListPage.tsx`
- 테이블에 확장 토글 컬럼(ChevronDown/ChevronRight) 추가
- FlowRow 컴포넌트로 확장 가능 행 분리
- 플로우명 클릭 시 에디터 이동, 행 클릭 시 패널 토글

**수정 파일**: `web/src/hooks/useFlow.ts`
- `useFlowNodes(flowId, refetchInterval?)`: 선택적 `refetchInterval` 파라미터 추가로 실시간 갱신 지원

**기존 API 활용**:
- `GET /flows/{id}/nodes`: 노드 인스턴스 목록 (FlowNodeInfo[])
- `GET /flows/{id}/status`: 플로우 상태 조회 (실시간 갱신 조건 판단)
- `GET /monitor/loglevel`: 현재 로그 레벨 조회 (`node.{name}` 키)
- `PUT /monitor/loglevel/node.{name}`: 노드 로그 레벨 설정
- `DELETE /monitor/loglevel/node.{name}`: 노드 로그 레벨 리셋

### 5.5 Module 5: 캔버스 런타임 통계

**신규 파일**: `web/src/contexts/RuntimeStatsContext.ts`
- `NodeRuntimeStats` 인터페이스: `{ inMessages: number, outMessages: number, state: string }`
- `RuntimeStatsContext`: `Record<string, NodeRuntimeStats>`를 제공하는 React Context
- `useNodeRuntimeStats(nodeId)`: 개별 노드의 런타임 통계를 구독하는 커스텀 훅
- editorStore와 완전 분리하여 isDirty/undo/redo에 영향 없음

**수정 파일**: `web/src/pages/editor/EditorPage.tsx`
- `useFlowStatus(flowId)`: 5초 간격 플로우 상태 폴링
- 플로우 running 시 `useFlowNodes(flowId, 3000)`: 3초 간격 노드 데이터 폴링
- `runtimeStatsMap` 구성: 노드별 포트 direction 필터링으로 inMessages/outMessages 계산
- `<RuntimeStatsContext.Provider>` 래핑으로 하위 CustomNode에 통계 전달

**수정 파일**: `web/src/components/flow/CustomNode.tsx`
- `useNodeRuntimeStats(id)` 훅으로 런타임 통계 구독
- 상태 표시등: runtime state에 따라 초록(running)/빨강(error)/회색(기본) 점
- In/Out 표시: `ArrowDownToLine`/`ArrowUpFromLine` 아이콘 + 메시지 수 (노드 라벨 아래)

### 5.6 백엔드 버그 수정

**수정 파일**: `internal/engine/engine.go`
- 포트 카운터 초기화 시 모든 포트(입력/출력/에러)에 대해 `atomic.Int64` 카운터를 명시적으로 0으로 초기화
- 이전 코드: 일부 포트의 카운터가 초기화되지 않아 메시지 카운트가 항상 0으로 보고됨

**수정 파일**: `internal/node/bridge.go`
- `msgCh` 채널 초기화 누락 수정
- `Process()` 메서드의 불필요한 에러 반환 수정 → 정상 처리 시 nil 반환
- BridgeOut 노드의 Process 반환값 수정

**수정 파일**: `internal/node/bridge_test.go`
- 브릿지 노드 수정 사항에 맞는 테스트 케이스 업데이트

### 5.7 Module 7: 로그 뷰어 컴포넌트/소스 필드 표시 및 필터링

**배경**: SPEC-OBS-004에서 백엔드 `wsLogWriter.Write()`가 `log.entry` 페이로드에 `component`와 `source` 필드를 추가 전송하도록 구현 완료. 프론트엔드 LogViewer는 아직 이 필드를 사용하지 않음.

**수정 파일 1**: `web/src/pages/monitoring/LogViewer.tsx`

- `LogEntry` 인터페이스 확장:
  - `component?: string` 필드 추가 (예: `agent.modbus-001`, `node.transform-1`)
  - `source?: string` 필드 추가 (예: `agent`, `node`, `flow`, `api`, `engine`, `system`)

- `SourceType` 타입 및 `SOURCE_STYLES` 맵 추가:
  - `agent`: 보라색 배지 (`bg-purple-100 text-purple-700`)
  - `node`: 청록색 배지 (`bg-teal-100 text-teal-700`)
  - `flow`: 녹색 배지 (`bg-green-100 text-green-700`)
  - `api`: 주황색 배지 (`bg-orange-100 text-orange-700`)
  - `engine`: 남색 배지 (`bg-indigo-100 text-indigo-700`)
  - `system`: 회색 배지 (`bg-gray-200 text-gray-600`)

- 필터 상태 추가:
  - `sourceFilter: Set<SourceType>` — 활성화된 source 카테고리 (빈 Set = 전체 표시)
  - `componentSearch: string` — 컴포넌트 이름 검색어 (빈 문자열 = 전체 표시)
  - `debouncedSearch: string` — 300ms 디바운스된 검색어

- 필터 로직 (기존 level 필터와 AND 결합):
  1. `filter !== null` → level 필터 적용
  2. `sourceFilter.size > 0` → source 필터 적용
  3. `debouncedSearch !== ''` → component 이름 포함 매칭 (대소문자 무시)

- 툴바 UI 확장:
  - 기존 레벨 필터 버튼 행 유지
  - 레벨 필터 오른쪽에 구분선(|) + source 필터 토글 버튼 6개 추가
  - source 버튼은 다중 선택 가능 (클릭 시 Set에 add/delete 토글)
  - 레벨 필터 행 아래에 컴포넌트 검색 입력 (Search 아이콘 + input)

- 로그 행 표시 확장:
  - 기존: `[timestamp] [LEVEL] message`
  - 변경: `[timestamp] [LEVEL] [source배지] [component명] message`
  - source 배지: SOURCE_STYLES 색상, 레벨 배지보다 작은 폰트
  - component 이름: `text-gray-500` 색상, 고정 너비 (`w-[140px]` truncate)

**수정 파일 2**: `web/src/pages/monitoring/MonitoringPage.tsx`

- `handleLog` 콜백 수정:
  - 기존 타입 캐스팅에 `component?: string`, `source?: string` 추가
  - LogEntry 생성 시 `component: d.component ?? ''`, `source: d.source ?? 'system'` 포함

### 5.8 Module 8: 동적 포트 시스템

**근본 원인**: 세 가지 문제가 복합적으로 발생하여 노드 포트가 설정과 불일치함.

1. **브릿지 노드 방향 무시**: `BRIDGE_DEFAULT_PORTS`가 direction과 무관하게 항상 `[{name:'in', direction:'input'}, {name:'out', direction:'output'}]`를 반환. BridgeIn은 output만, BridgeOut은 input만 표시해야 함.
2. **스위치 노드 정적 포트**: 스위치 노드가 `[in, out]` 고정 포트를 사용하지만, 실제로는 라우트 수에 따라 다수의 출력 포트가 필요함.
3. **설정 변경 시 포트 미갱신**: `getDefaultPorts(nodeType)`가 노드 생성 시점에만 호출되고, 이후 설정(direction, routes) 변경 시 포트가 재계산되지 않음.

**신규 함수**: `web/src/config/nodeSchemas.ts` — `computePortsForNode(nodeType, config)`

```typescript
export function computePortsForNode(nodeType: string, config?: Record<string, unknown>): PortDef[] {
  if (nodeType === 'bridge') {
    const direction = config?.direction as string | undefined;
    switch (direction) {
      case 'in':     return [{ name: 'out', direction: 'output' }];
      case 'out':    return [{ name: 'in', direction: 'input' }];
      case 'inout':
      case 'request_reply':
        return [{ name: 'in', direction: 'input' }, { name: 'out', direction: 'output' }];
      default:
        return [{ name: 'in', direction: 'input' }, { name: 'out', direction: 'output' }];
    }
  }
  if (nodeType === 'switch') {
    const routes = config?.routes as Array<{ name: string }> | undefined;
    const ports: PortDef[] = [{ name: 'in', direction: 'input' }];
    if (routes && routes.length > 0) {
      routes.forEach(r => ports.push({ name: r.name, direction: 'output' }));
      ports.push({ name: 'default', direction: 'output' });
    } else {
      ports.push({ name: 'out', direction: 'output' });
    }
    return ports;
  }
  // 기타 노드: 기존 getDefaultPorts 로직 유지
  return getDefaultPorts(nodeType);
}
```

**수정 파일 1**: `web/src/config/nodeSchemas.ts`
- `computePortsForNode(nodeType, config)` 함수 추가
- 기존 `getDefaultPorts(nodeType)` 함수는 하위 호환성을 위해 유지 (config가 없는 경우 호출됨)
- `BRIDGE_DEFAULT_PORTS` 상수는 deprecated 처리 (computePortsForNode 내부로 로직 이동)

**수정 파일 2**: `web/src/pages/editor/EditorPage.tsx`
- 새 노드 생성 시: `getDefaultPorts(nodeType.type)` → `computePortsForNode(nodeType.type, initialConfig)` 변경
- 노드 설정 변경 콜백: config 변경 시 `computePortsForNode(nodeType, updatedConfig)` 호출하여 `data.ports` 업데이트
- 삭제된 포트에 연결된 엣지 자동 정리 로직 추가

**수정 파일 3**: `pkg/flow/node.go`
- `NewNodeDef()` 팩토리 또는 별도 함수에서 브릿지 direction 파라미터를 확인하여 Inputs/Outputs 포트를 적절히 생성
- BridgeIn: `Outputs = [Port{Name:"out"}]`, `Inputs = []`
- BridgeOut: `Inputs = [Port{Name:"in"}]`, `Outputs = []`
- BridgeInOut/BridgeRequestReply: `Inputs = [Port{Name:"in"}]`, `Outputs = [Port{Name:"out"}]`

**수정 파일 4** (선택): `web/src/components/flow/CustomNode.tsx`
- 포트 렌더링이 `data.ports`의 direction 필터로 동작하므로 기본적으로 변경 불필요
- error 포트 표시가 필요한 경우에만 수정

### 5.9 Module 9: 에러 포트 타입 지원

**근본 원인**: 프론트엔드 포트 타입 시스템이 `'input' | 'output'`만 허용하여 백엔드의 `PortError` direction(`"error"`)을 가진 포트가 무시됨.

1. **PortDef 타입 제한**: `web/src/config/nodeSchemas.ts:7`에서 `direction: 'input' | 'output'`만 정의
2. **NodeTypeDefinition 타입 제한**: `web/src/types/node.ts:47`에서 동일하게 `direction: 'input' | 'output'`만 정의
3. **CustomNode 포트 필터링**: `web/src/components/flow/CustomNode.tsx:61-62`에서 `direction === 'input'`과 `direction === 'output'`만 필터링하여 `direction === 'error'` 포트가 렌더링에서 누락됨
4. **NodeHandle 색상 미지원**: `web/src/components/flow/NodeHandle.tsx`에서 파란색(input)과 초록색(output) 2가지 색상만 지원

**백엔드 참고** (수정 불필요):
- `pkg/flow/node.go:16`: `PortError PortDirection = "error"` — 이미 에러 포트 타입 정의됨
- `pkg/flow/node.go:112`: `WithErrorPort()` — 에러 포트 생성 옵션 존재
- `internal/api/service/flow_adapter.go:737-741`: 에러 포트를 `direction: "error"`로 직렬화하여 프론트엔드로 전달

**수정 파일 1**: `web/src/config/nodeSchemas.ts`
- `PortDef` 인터페이스의 `direction` 타입을 `'input' | 'output'`에서 `'input' | 'output' | 'error'`로 확장
- `computePortsForNode()` 함수에서 에러 포트를 지원하는 노드 타입(예: `WithErrorPort` 옵션이 적용된 노드)의 포트 배열에 에러 포트 정의 포함

**수정 파일 2**: `web/src/types/node.ts`
- `NodeTypeDefinition.ports[].direction` 타입을 `'input' | 'output'`에서 `'input' | 'output' | 'error'`로 확장

**수정 파일 3**: `web/src/components/flow/CustomNode.tsx`
- 기존 포트 필터링에 에러 포트 그룹 추가: `const errorPorts = ports.filter(p => p.direction === 'error')`
- 출력 포트와 에러 포트를 합쳐 `rightPorts` 배열 구성: `const rightPorts = [...outputPorts, ...errorPorts]`
- 에러 포트를 노드 오른쪽(Right position)에 출력 포트 아래로 배치하여 빨간색 핸들로 렌더링
- 다중 핸들 offset 배치: `((i+1)/(total+1))*100%` 공식으로 균등 분배
- 에러 포트가 없는 노드는 기존과 동일하게 렌더링 (하위 호환)

**수정 파일 4**: `web/src/components/flow/NodeHandle.tsx`
- `isError` prop 추가: 에러 포트 여부를 외부에서 전달
- `offset` prop 추가: 다중 핸들 배치 시 `style={{ top: offset }}` 적용
- 에러 포트용 빨간색(`bg-red-500`) 핸들 색상 추가
- 포트 타입별 색상: input=파란색(`bg-blue-500`), output=초록색(`bg-green-500`), error=빨간색(`bg-red-500`)

### 5.10 Module 10: Handle ID 접두사 제거

**배경**: Module 8/9에서 포트 시스템을 개선하면서 Handle ID에 방향 접두사(`in-`, `out-`, `err-`)를 사용하고 있었으나, 포트 direction이 별도 필드로 이미 구분되므로 접두사가 불필요함. 접두사가 프론트엔드/백엔드 간 불일치를 유발하여 와이어 연결이 실패하는 원인이 됨.

**설계 원칙**: 포트 이름(`name`) = Handle ID = Wire Port. 중간 변환 없이 직접 매핑.

**수정 파일 1**: `web/src/components/flow/CustomNode.tsx`
- Input Handle: `id={port.name}` (이전: `id={\`in-${port.name}\`}`)
- Output Handle: `id={port.name}` (이전: `id={\`out-${port.name}\`}`)
- Error Handle: `id={port.name}` (이전: `id={\`err-${port.name}\`}`)

**수정 파일 2**: `internal/api/service/flow_adapter.go`
- `flowToReactFlowConfig()`: Edge 생성 시 `sourceHandle: w.SourcePort`, `targetHandle: w.TargetPort` (이전: `"out-" + w.SourcePort`, `"in-" + w.TargetPort`)
- `normalizeReactFlowDefinition()`: Edge 역변환 시 `source_port: sourceHandle`, `target_port: targetHandle` (이전: `strings.TrimPrefix(sh, "out-")`, `strings.TrimPrefix(th, "in-")`)
- `strings` 패키지 import 제거 (TrimPrefix 미사용)

**수정 파일 3**: `web/src/components/property/PropertyPanel.tsx`
- `Port` 타입: `direction: 'input' | 'output'` → `direction: 'input' | 'output' | 'error'`
- 엣지 정리 로직: `.map((p) => p.name)` (이전: 접두사 포함 Handle ID 비교)

### 5.11 Module 11: 리스트 정렬 기능

**배경**: FlowListPage와 AgentListPage에 정렬 기능이 없어 사용자가 원하는 항목을 찾기 어려움. 백엔드 `ListOptions.Sort` 필드와 프론트엔드 `ListOptions.sort` 타입이 이미 정의되어 있으나 실제 구현되지 않은 상태.

**정렬 파라미터 형식**: `field:direction`
- `field`: 정렬할 필드명 (예: `name`, `status`, `created_at`)
- `direction`: `asc` (오름차순) 또는 `desc` (내림차순)
- 기본값: `name:asc`

**수정 파일 1**: `internal/api/service/flow_adapter.go`
- `ListFlows()`: 상태 필터링 후, 페이지네이션 적용 전에 `opts.Sort` 파싱하여 `sort.Slice` 적용
- 정렬 가능 필드: `name`, `status`, `created_at`, `updated_at`
- `parseSortParam(sort string) (field string, ascending bool)` 유틸리티 함수 추가

**수정 파일 2**: `internal/api/service/agent_adapter.go`
- `ListAgents()`: 상태 필터링 후, 페이지네이션 적용 전에 `opts.Sort` 파싱하여 `sort.Slice` 적용
- 정렬 가능 필드: `name`, `type`, `status`

**수정 파일 3**: `web/src/pages/flows/FlowListPage.tsx`
- 정렬 상태: `useState<{field: string, direction: 'asc'|'desc'}>({field: 'name', direction: 'asc'})`
- 정렬 가능 헤더에 `SortableHeader` 컴포넌트 적용 (이름, 상태, 생성일, 수정일)
- API 호출 시 `sort` 파라미터 포함
- 클라이언트 사이드 정렬이 아닌 서버 사이드 정렬 사용

**수정 파일 4**: `web/src/pages/agents/AgentListPage.tsx`
- 정렬 상태: `useState<{field: string, direction: 'asc'|'desc'}>({field: 'name', direction: 'asc'})`
- 정렬 가능 헤더에 `SortableHeader` 컴포넌트 적용 (이름, 타입, 상태)
- API 호출 시 `sort` 파라미터 포함

**신규 파일**: `web/src/components/common/SortableHeader.tsx`
- Props: `label: string, field: string, currentSort: {field, direction}, onSort: (field) => void`
- 현재 정렬 필드이면 방향 인디케이터 표시 (▲/▼)
- 클릭 시 onSort 콜백 호출
- Tailwind CSS 스타일: 호버 시 커서 변경, 정렬 활성 시 색상 강조

**신규 파일(선택)**: `internal/api/service/sort_util.go`
- `parseSortParam(sort string) (field string, ascending bool)` 공통 정렬 파라미터 파싱 함수
- 유효하지 않은 정렬 파라미터는 기본값(`name:asc`)으로 폴백

### 5.12 UI 변경 요약

| 위치 | 변경 내용 |
|------|----------|
| `AgentDetailPanel.tsx` 통계 탭 | 로그 레벨 드롭다운 추가 (debug/info/warn/error/기본값) |
| `SettingsPage.tsx` 시스템 탭 | 컴포넌트별 오버라이드 목록 테이블 추가 (컴포넌트명, 레벨, 리셋 버튼) |
| `monitorService.ts` | `getLogLevels`, `setComponentLogLevel`, `resetComponentLogLevel` 함수 추가 |
| `agentService.ts` | `getAgents` 호출에 `detail=summary` 파라미터 추가 |
| `FlowDetailPanel.tsx` (신규) | 노드 인스턴스 테이블 + In/Out 분리 표시 + 로그 레벨 드롭다운 + 실시간 갱신 |
| `FlowListPage.tsx` | 확장 가능 행 추가 (FlowDetailPanel 토글) |
| `RuntimeStatsContext.ts` (신규) | 캔버스 런타임 통계 Context + useNodeRuntimeStats 훅 |
| `EditorPage.tsx` | RuntimeStatsContext.Provider 래핑 + 런타임 통계 폴링 |
| `CustomNode.tsx` | 런타임 상태 표시등 + In/Out 메시지 수 표시 |
| `useFlow.ts` | useFlowNodes에 refetchInterval 파라미터 추가 |
| `LogViewer.tsx` | LogEntry에 component/source 필드 추가, source 필터 버튼, 컴포넌트 검색 입력, 행별 source/component 배지 표시 |
| `MonitoringPage.tsx` | handleLog에서 component/source 필드 추출 |
| `nodeSchemas.ts` | `computePortsForNode(nodeType, config)` 함수 추가, 브릿지/스위치 동적 포트 로직 |
| `EditorPage.tsx` | 노드 생성 시 `computePortsForNode` 사용, 설정 변경 시 포트 재계산 + 엣지 정리 |
| `node.go` | `NewNodeDef()` 브릿지 direction 기반 포트 생성 |
| `nodeSchemas.ts` | `PortDef.direction` 타입에 `'error'` 추가 |
| `node.ts` | `NodeTypeDefinition.ports[].direction` 타입에 `'error'` 추가 |
| `CustomNode.tsx` | 에러 포트 필터링 + rightPorts(출력+에러) 합산 + offset 배치 |
| `NodeHandle.tsx` | `isError`/`offset` prop 추가 + 에러 포트 핸들 색상 `bg-red-500` |
| `flow_adapter.go` | Handle↔Edge 매핑에서 접두사(`in-`/`out-`/`err-`) 추가/제거 로직 삭제 |
| `PropertyPanel.tsx` | Port 타입에 `'error'` direction 추가, 엣지 정리 시 포트 이름 직접 비교 |
| `FlowListPage.tsx` | 정렬 가능 컬럼 헤더 추가 (이름, 상태, 생성일, 수정일), sort 파라미터 API 전달 |
| `AgentListPage.tsx` | 정렬 가능 컬럼 헤더 추가 (이름, 타입, 상태), sort 파라미터 API 전달 |
| `SortableHeader.tsx` (신규) | 정렬 방향 인디케이터(▲/▼) + 클릭 토글 공용 컴포넌트 |
| `flow_adapter.go` | ListFlows sort 파라미터 처리 구현, 정렬 후 페이지네이션 |
| `agent_adapter.go` | ListAgents sort 파라미터 처리 구현, 정렬 후 페이지네이션 |

### 5.13 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC | 의존 내용 |
|-------------|-----------|----------|
| Module 1 | SPEC-API-001 | `GET /agents?detail=summary` 쿼리 파라미터 지원 |
| Module 2 | SPEC-OBS-001 | `observe.LevelManager` 인터페이스 (SetLevel, GetLevel, ListLevels) |
| Module 3 | SPEC-API-001 | 신규 `/monitor/loglevel/{component}` 엔드포인트 |
| Module 4 | SPEC-API-001 | `GET /flows/{id}/nodes`, `/monitor/loglevel/node.{name}` 엔드포인트 |
| Module 7 | SPEC-OBS-004 | WebSocket `log.entry` 페이로드의 `component`/`source` 필드 (백엔드 구현 완료) |
| Module 8 | - | 독립 모듈. `nodeSchemas.ts`, `EditorPage.tsx`, `node.go` 자체 수정으로 완결 |
| Module 9 | - | 독립 모듈. `nodeSchemas.ts`, `node.ts`, `CustomNode.tsx`, `NodeHandle.tsx` 자체 수정으로 완결. 백엔드 수정 불필요 |
| Module 10 | Module 8, 9 | Handle ID 매핑 간소화. Module 8/9에서 도입한 포트 시스템의 Handle ID 규칙 통합 |
| Module 11 | SPEC-API-001 | `GET /flows?sort=field:dir`, `GET /agents?sort=field:dir` 쿼리 파라미터 |

### 5.14 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: 에이전트 목록 통계 버그 수정 | 사용자에게 보이는 기존 기능 장애 |
| P1 (중요) | Module 2: 백엔드 로그 레벨 API | Module 3, 4의 전제 조건 |
| P1 (중요) | Module 3: 에이전트 로그 레벨 UI | 디버깅 효율성 향상 |
| P1 (중요) | Module 4: 노드 통계 + 로그 레벨 UI | 노드 단위 디버깅 지원 |
| P1 (중요) | Module 5: 캔버스 런타임 통계 | 에디터 캔버스 실시간 모니터링 |
| P0 (즉시) | 백엔드 버그 수정: 포트 카운터/브릿지 | 메시지 카운트 정확도 보장 |
| P1 (중요) | Module 7: 로그 뷰어 컴포넌트/소스 필터링 | 로그 디버깅 효율성 향상, SPEC-OBS-004 프론트엔드 연동 |
| P0 (즉시) | Module 8: 동적 포트 시스템 | 노드 포트가 설정과 불일치하는 사용자 가시 버그 수정 |
| P0 (즉시) | Module 9: 에러 포트 타입 지원 | 백엔드 에러 포트가 프론트엔드에서 무시되는 사용자 가시 버그 수정 |
| P0 (즉시) | Module 10: Handle ID 접두사 제거 | Handle ID 불일치로 와이어 연결 실패 방지, 코드 간소화 |
| P1 (중요) | Module 11: 리스트 정렬 기능 | 사용자 편의성 향상, 대량 리스트 탐색 효율화 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.7.0*
*상태: in_progress*
*최종 수정: 2026-03-09*
