---
id: SPEC-WEB-001
version: "1.3.0"
status: completed
created: "2026-03-07"
updated: "2026-03-08"
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

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: 백엔드 `GET /agents` API에서 `detail` 쿼리 파라미터를 `ListOptions`에 전달하고, `agentToHandlerInfo`에서 detail 값에 따라 stats, uptime, health 필드를 포함한다
- A-002: `observe.LevelManager` 인터페이스의 `SetLevel`, `GetLevel`, `SetLevelByPattern`, `ListLevels` 메서드가 정상 동작한다
- A-003: `internal/api/handler/monitor.go`의 기존 `PUT /monitor/loglevel` 핸들러가 글로벌 로그 레벨 변경 기능을 올바르게 수행한다
- A-004: 프론트엔드 `AgentInfo` 타입에 이미 `stats`, `uptime`, `health` optional 필드가 정의되어 있다
- A-007: SPEC-OBS-004 구현이 완료되어 백엔드 WebSocket `log.entry` 페이로드에 `component`와 `source` 필드가 포함되어 전송된다
- A-008: `classifySource()` 함수가 반환하는 source 카테고리는 `agent`, `node`, `flow`, `api`, `engine`, `system` 6가지이다

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

### 5.8 UI 변경 요약

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

### 5.9 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC | 의존 내용 |
|-------------|-----------|----------|
| Module 1 | SPEC-API-001 | `GET /agents?detail=summary` 쿼리 파라미터 지원 |
| Module 2 | SPEC-OBS-001 | `observe.LevelManager` 인터페이스 (SetLevel, GetLevel, ListLevels) |
| Module 3 | SPEC-API-001 | 신규 `/monitor/loglevel/{component}` 엔드포인트 |
| Module 4 | SPEC-API-001 | `GET /flows/{id}/nodes`, `/monitor/loglevel/node.{name}` 엔드포인트 |
| Module 7 | SPEC-OBS-004 | WebSocket `log.entry` 페이로드의 `component`/`source` 필드 (백엔드 구현 완료) |

### 5.10 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: 에이전트 목록 통계 버그 수정 | 사용자에게 보이는 기존 기능 장애 |
| P1 (중요) | Module 2: 백엔드 로그 레벨 API | Module 3, 4의 전제 조건 |
| P1 (중요) | Module 3: 에이전트 로그 레벨 UI | 디버깅 효율성 향상 |
| P1 (중요) | Module 4: 노드 통계 + 로그 레벨 UI | 노드 단위 디버깅 지원 |
| P1 (중요) | Module 5: 캔버스 런타임 통계 | 에디터 캔버스 실시간 모니터링 |
| P0 (즉시) | 백엔드 버그 수정: 포트 카운터/브릿지 | 메시지 카운트 정확도 보장 |
| P1 (중요) | Module 7: 로그 뷰어 컴포넌트/소스 필터링 | 로그 디버깅 효율성 향상, SPEC-OBS-004 프론트엔드 연동 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.3.0*
*상태: planned*
*최종 수정: 2026-03-08*
