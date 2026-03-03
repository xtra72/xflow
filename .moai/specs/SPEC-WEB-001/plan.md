---
id: SPEC-WEB-001
type: plan
version: "2.0.0"
status: in-progress
created: "2026-02-13"
updated: "2026-03-03"
author: xtra
---

# SPEC-WEB-001: Web Dashboard - 구현 계획

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 준수):
- **신규 코드 (TDD)**: `web/src/` 전체가 신규 작성이므로 RED-GREEN-REFACTOR 사이클 적용
- 테스트 커버리지 목표: 85% 이상
- 모든 컴포넌트, 훅, 서비스에 대해 테스트를 먼저 작성한 후 구현

### 1.2 핵심 설계 결정

1. **서버 상태 vs 클라이언트 상태 분리**: React Query로 서버 캐시 관리, Zustand로 순수 UI 상태만 관리하여 상태 동기화 복잡성 최소화
2. **컴포넌트 합성 패턴**: shadcn/ui + Radix UI 기반 Headless 컴포넌트를 합성하여 일관된 UI 구축
3. **API 클라이언트 추상화**: 도메인별 서비스 모듈(flowService, agentService 등)로 API 호출을 래핑하여 타입 안전성 보장
4. **코드 스플리팅**: react-router의 lazy 로딩으로 페이지별 코드 분할, 초기 번들 최소화
5. **React Flow 커스터마이징**: 커스텀 노드/엣지/핸들 컴포넌트로 XFlow 도메인에 맞는 시각적 표현 구현

### 1.3 기술 스택

| 구성 요소 | 선택 | 비고 |
|-----------|------|------|
| UI Framework | React 19.x | 서버 컴포넌트 미사용 (SPA) |
| Language | TypeScript 5.x | strict 모드 |
| Build Tool | Vite 6.x | HMR, 코드 스플리팅 |
| Node Editor | @xyflow/react 12.x+ | 커스텀 노드/엣지 |
| CSS | Tailwind CSS 4.x | dark 모드, 반응형 |
| Components | shadcn/ui + Radix UI | Headless 컴포넌트 합성 |
| Client State | Zustand 5.x | 최소 API, 미들웨어 지원 |
| Server State | @tanstack/react-query 5.x | 캐싱, 낙관적 업데이트 |
| HTTP Client | axios 또는 ky | 인터셉터, 재시도 |
| Routing | react-router 7.x | 선언적 라우팅, lazy 로딩 |
| Icons | lucide-react | 트리 셰이킹 |
| Charts | recharts 또는 @tremor/react | 메트릭 시각화 |
| Code Editor | @monaco-editor/react | Lua 구문 강조 |
| Unit Test | Vitest 3.x | Vite 네이티브 |
| Component Test | @testing-library/react | 사용자 관점 테스트 |
| E2E Test | Playwright | 크로스 브라우저 |
| Lint | ESLint 9.x + Prettier 3.x | Flat config |

---

## 2. 마일스톤 개요

### Milestone 1 (완료): Foundation

프로젝트 초기 설정, 타입 정의, API 클라이언트, WebSocket 클라이언트, Zustand 스토어, React Query 훅 구현.

**상태: COMPLETED**

### Milestone 2 (완료): App Shell + Flow Editor

App Shell 레이아웃 (Sidebar, Header, ErrorBoundary), 라우팅 설정, 로그인 페이지, 플로우 에디터 페이지, 커스텀 노드/엣지/핸들, Node Palette, Property Panel, 에디터 도구 모음 구현.

**상태: COMPLETED**

### Milestone 3 (완료): Dashboard + Monitoring + Settings

대시보드 페이지 및 4개 위젯 (SystemStatus, RecentFlows, Resource, AgentStatus), 모니터링 페이지 (MetricsChart, LogViewer, EventTimeline), 설정 페이지, 플로우 생성 모달 구현.

**상태: COMPLETED**

### Milestone 4 (완료): Theme Provider + i18n

다크/라이트 테마 전환, 시스템 테마 자동 감지, 다국어 지원 (ko/en) 기반 구조 구현.

**상태: COMPLETED**

---

## 3. Milestone 5 후속: 버그 수정 및 백엔드-프론트엔드 통합 강화

### 3.0 개요

Milestone 5 커밋(e55223a) 이후 발견된 플로우 에디터 제어 버튼 동작 버그("시작 실패: not found")를 수정하고, 백엔드-프론트엔드 데이터 통합을 강화하는 작업이다.

**상태: COMPLETED**

### 3.0.1 핵심 변경 사항

#### Backend: FlowServiceAdapter 구현 (+556 라인)

Engine과 Repository 간의 상태 불일치를 해결하기 위해 `FlowServiceAdapter`를 구현했다:

- **자동 배포(Auto-Deploy)**: Repository에 저장된 플로우를 시작할 때, Engine에 배포되지 않은 경우 자동으로 배포 후 시작
- **재배포(Redeploy)**: FlowStopped 상태의 플로우를 재시작할 때, UndeployFlow → DeployFlow → StartFlow 시퀀스 자동 실행
- **Engine.GetFlow()**: 배포된 플로우 조회 메서드 추가
- **UndeployFlow 상태 수정**: FlowLoaded 상태에서도 배포 해제 허용 (재배포 시나리오 지원)

#### Backend: 에러 처리 개선

- 404 에러 응답에 원본 에러 메시지 포함 (`ErrNotFound.WithMessage(err.Error())`)
- Start/Deploy 핸들러에 디버그 로깅 추가

#### Frontend: 에러 메시지 개선

- EditorToolbar의 NOT_FOUND 에러에 사용자 친화적 한국어 메시지 표시
- PropertyPanel 기능 확장 (+255 라인)
- DynamicForm 개선

#### 테스트 안정화

- UndeployFlow 테스트: FlowLoaded 상태 undeploy 성공 검증 + FlowRunning 상태 실패 검증
- PortCounter 타이밍 테스트: `time.Sleep(time.Millisecond)` 추가로 안정화
- FlowServiceAdapter 통합 테스트 101라인 추가

### 3.0.2 파일 변경 목록

| 파일 | 변경 | 라인 |
|------|------|------|
| `internal/api/service/flow_adapter.go` | 수정 | +556 |
| `web/src/components/property/PropertyPanel.tsx` | 수정 | +255 |
| `internal/api/service/flow_adapter_test.go` | 수정 | +101 |
| `web/src/components/flow/EditorToolbar.tsx` | 수정 | +71 |
| `internal/engine/engine_test.go` | 수정 | +20 |
| `internal/engine/engine.go` | 수정 | +17 |
| `web/src/components/property/DynamicForm.tsx` | 수정 | +10 |
| `web/src/pages/editor/EditorPage.tsx` | 수정 | +7 |
| `internal/api/handler/flow.go` | 수정 | +6 |
| `web/src/components/layout/AppLayout.tsx` | 수정 | +4 |
| `internal/api/errors.go` | 수정 | +2 |
| `internal/engine/port_counter_test.go` | 수정 | +2 |
| `web/src/components/layout/NotificationToast.tsx` | 신규 | - |
| `web/src/config/` | 신규 | - |
| `internal/node/expression_integration_test.go` | 신규 | - |

---

## 4. Milestone 5: 기능 완성 및 데이터 통합

### 4.1 개요

Milestone 1-4에서 구축한 인프라와 UI 셸을 기반으로, 나머지 미구현 페이지와 백엔드-프론트엔드 데이터 통합을 완성하는 마일스톤이다. 주요 작업은 다음 5개 서브 마일스톤으로 구성된다:

| 서브 마일스톤 | 범위 | 영역 |
|-------------|------|------|
| 5A | Flow List Page 완성 | Frontend Only |
| 5B | Agent Management Page 신규 | Frontend Only |
| 5C | Node Type Browser 신규 | Frontend Only |
| 5D | Dashboard 데이터 통합 | Backend + Frontend |
| 5E | WebSocket 실시간 통합 | Backend Only |

### 4.2 의존성 관계

```
5E (WebSocket Backend)
  ↓
5D (Dashboard 데이터 통합) ← 5E에서 WS 핸들러 구현 필요
  ↓
5A (Flow List Page) ← 독립적, 기존 API 사용
5B (Agent List Page) ← 독립적, 기존 API 사용
5C (Node Type Browser) ← 독립적, 기존 API 사용
```

**권장 구현 순서:**
1. **5A, 5B, 5C** (병렬 가능) - 기존 백엔드 API만 사용하므로 즉시 착수 가능
2. **5E** - WebSocket 백엔드 구현
3. **5D** - 메트릭 엔드포인트 구현 + 프론트엔드 위젯 연결

---

### 4.3 Milestone 5A: Flow List Page (Frontend Only)

#### 목표

FlowListPage.tsx의 플레이스홀더("준비 중")를 완전한 플로우 관리 페이지로 교체한다.

#### 요구 기능

1. **플로우 테이블**
   - 컬럼: 이름, 상태(Status Badge), 노드 수, 생성일, 수정일
   - Status Badge 색상 규칙:
     - `Running` = green
     - `Stopped` = gray
     - `Error` = red
     - `Draft` = blue
     - `Deployed` = yellow
   - 행 클릭 시 `/editor/:flowId`로 이동

2. **검색 및 필터**
   - 이름 기반 텍스트 검색 (debounce 300ms)
   - 상태(Status) 드롭다운 필터 (All, Running, Stopped, Error, Draft, Deployed)

3. **페이지네이션**
   - 페이지당 10/20/50건 선택
   - 이전/다음 페이지 버튼, 현재 페이지 표시

4. **CRUD 액션**
   - "새 플로우" 버튼: DashboardPage의 CreateFlowModal 재사용
   - 삭제: 행별 삭제 버튼, 확인 다이얼로그 포함

5. **라이프사이클 액션**
   - 행별 드롭다운 메뉴: 시작(start), 중지(stop), 재시작(restart), 배포(deploy)
   - 액션 실행 후 목록 자동 갱신 (React Query invalidation)

#### 파일 목록

| 파일 경로 | 작업 | 예상 라인 |
|----------|------|----------|
| `web/src/pages/flows/FlowListPage.tsx` | **수정** (전면 재작성) | ~250 |
| `web/src/pages/flows/FlowStatusBadge.tsx` | **신규** | ~40 |
| `web/src/pages/flows/FlowActionMenu.tsx` | **신규** | ~80 |
| `web/src/pages/flows/FlowSearchFilter.tsx` | **신규** | ~60 |

#### 기존 의존성 (이미 구현됨)

- `web/src/services/api/flowService.ts` - Flow CRUD + lifecycle API 전체 구현
- `web/src/hooks/useFlow.ts` - React Query 훅 전체 구현
- `web/src/types/flow.ts` - FlowDefinition, FlowStatus 타입
- `web/src/pages/dashboard/CreateFlowModal.tsx` - 플로우 생성 모달 재사용
- `web/src/router.tsx` - `/flows` 라우트 이미 등록됨

---

### 4.4 Milestone 5B: Agent Management Page (Frontend Only)

#### 목표

Agent CRUD 및 라이프사이클 관리를 위한 전용 페이지를 신규 생성한다. API 서비스(`agentService.ts`)와 React Query 훅(`useAgent.ts`)은 이미 완전히 구현되어 있으므로 UI만 구축한다.

#### 요구 기능

1. **에이전트 테이블**
   - 컬럼: 이름, 타입, 상태(connected/disconnected), uptime, 메시지 통계(in/out)
   - 상태 표시: connected = green dot, disconnected = gray dot

2. **CRUD 기능**
   - "새 에이전트" 버튼: 생성 모달 (name, type 선택, config JSON 입력)
   - 행별: 편집, 삭제 (확인 다이얼로그)

3. **라이프사이클 관리**
   - 행별 버튼: 시작(start), 중지(stop), 재시작(restart)
   - 현재 상태에 따른 버튼 비활성화 (running이면 start 비활성)

4. **에이전트 상세 보기**
   - 행 확장(expandable row) 또는 사이드 패널로 상세 통계 표시
   - AgentStatsInfo: messages_in, messages_out, error_count, uptime

5. **라우팅 및 사이드바**
   - `/agents` 라우트를 router.tsx에 추가
   - Sidebar.tsx NAV_ITEMS에 "에이전트" 메뉴 추가 (Bot 아이콘)

#### 파일 목록

| 파일 경로 | 작업 | 예상 라인 |
|----------|------|----------|
| `web/src/pages/agents/AgentListPage.tsx` | **신규** | ~280 |
| `web/src/pages/agents/AgentStatusBadge.tsx` | **신규** | ~30 |
| `web/src/pages/agents/AgentActionButtons.tsx` | **신규** | ~60 |
| `web/src/pages/agents/CreateAgentModal.tsx` | **신규** | ~120 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | **신규** | ~80 |
| `web/src/router.tsx` | **수정** | +10 |
| `web/src/components/layout/Sidebar.tsx` | **수정** | +5 |
| `web/src/lib/i18n/ko.json` | **수정** | +10 |
| `web/src/lib/i18n/en.json` | **수정** | +10 |

#### 기존 의존성 (이미 구현됨)

- `web/src/services/api/agentService.ts` - Agent CRUD + lifecycle + stats + exec API 전체 구현
- `web/src/hooks/useAgent.ts` - React Query 훅 전체 구현 (useAgents, useAgent, useAgentStats, useCreateAgent, useUpdateAgent, useDeleteAgent, useStartAgent, useStopAgent, useRestartAgent, useExecAgent)
- `web/src/types/agent.ts` - AgentInfo, AgentStatsInfo, AgentCreateRequest 등 타입 전체 정의

---

### 4.5 Milestone 5C: Node Type Browser (Frontend Only)

#### 목표

등록된 노드 타입 카탈로그를 탐색할 수 있는 전용 페이지를 신규 생성한다. 현재 노드 타입은 에디터의 Node Palette에서만 접근 가능한데, 독립 페이지로 제공하여 카탈로그 전체를 편리하게 브라우징할 수 있도록 한다.

#### 요구 기능

1. **카드 그리드 레이아웃**
   - 각 카드에 노드 타입 이름, 카테고리, 설명, source 표시
   - 카테고리별 색상 구분 (Input=blue, Output=green, Process=purple, Bridge=orange, Special=gray)

2. **카테고리 그룹핑**
   - 카테고리별 섹션 헤더 (Input, Output, Process, Bridge, Special)
   - "전체" 탭과 카테고리별 탭 제공

3. **검색/필터**
   - 노드 이름/설명 기반 텍스트 검색
   - 카테고리 탭 필터

4. **라우팅 및 사이드바**
   - `/nodes` 라우트를 router.tsx에 추가
   - Sidebar.tsx NAV_ITEMS에 "노드" 메뉴 추가 (Blocks 아이콘)

#### 파일 목록

| 파일 경로 | 작업 | 예상 라인 |
|----------|------|----------|
| `web/src/pages/nodes/NodeTypesPage.tsx` | **신규** | ~180 |
| `web/src/pages/nodes/NodeTypeCard.tsx` | **신규** | ~60 |
| `web/src/pages/nodes/NodeCategoryTabs.tsx` | **신규** | ~50 |
| `web/src/router.tsx` | **수정** (5B에서 이미 수정, 추가 라우트) | +10 |
| `web/src/components/layout/Sidebar.tsx` | **수정** (5B에서 이미 수정, 추가 메뉴) | +5 |
| `web/src/lib/i18n/ko.json` | **수정** | +8 |
| `web/src/lib/i18n/en.json` | **수정** | +8 |

#### 기존 의존성 (이미 구현됨)

- `web/src/services/api/nodeService.ts` - getNodeTypes() API 구현
- `web/src/hooks/useNodeTypes.ts` - React Query 훅 구현
- `web/src/types/node.ts` - NodeType, NodeCategory 등 타입 정의

---

### 4.6 Milestone 5D: Dashboard 데이터 통합 (Backend + Frontend)

#### 목표

대시보드 위젯이 실제 백엔드 데이터를 표시하도록 연결한다. 현재 `/api/v1/monitor/metrics` 엔드포인트가 404를 반환하므로 백엔드에 메트릭 API를 구현하고, 프론트엔드 ResourceWidget을 실제 데이터에 연결한다.

#### 요구 기능

**Backend:**
1. **GET /api/v1/monitor/metrics 엔드포인트 구현**
   - 응답 데이터: CPU 사용률(%), 메모리 사용량(bytes/percent), 활성 플로우 수, 에이전트 수, 총 메시지 처리량
   - Go runtime 패키지 활용: `runtime.MemStats`, `runtime.NumGoroutine()`
   - 플로우/에이전트 카운트는 기존 레지스트리에서 조회

2. **응답 포맷**
   ```
   {
     "success": true,
     "data": {
       "cpu_percent": 12.5,
       "memory_used_bytes": 134217728,
       "memory_total_bytes": 8589934592,
       "memory_percent": 1.56,
       "goroutines": 42,
       "flows_running": 3,
       "flows_total": 8,
       "agents_connected": 5,
       "agents_total": 7,
       "messages_processed": 15234,
       "uptime_seconds": 86400
     }
   }
   ```

**Frontend:**
1. **ResourceWidget 데이터 연결**
   - monitorService.getMetrics() 호출 결과를 ResourceWidget에 전달
   - 5초 간격 자동 갱신 (React Query refetchInterval)

2. **모든 위젯 데이터 검증**
   - SystemStatusWidget: useFlows()에서 실제 플로우 상태 카운트 표시
   - RecentFlowsWidget: 실제 최근 생성/수정 플로우 목록 표시
   - AgentStatusWidget: useAgents()에서 실제 에이전트 연결 상태 표시
   - ResourceWidget: metrics API에서 실제 CPU/메모리 데이터 표시

#### 파일 목록

| 파일 경로 | 작업 | 예상 라인 |
|----------|------|----------|
| `internal/api/handler/monitor.go` | **신규** (또는 수정) | ~80 |
| `internal/api/server.go` | **수정** (라우트 등록) | +5 |
| `web/src/pages/dashboard/widgets/ResourceWidget.tsx` | **수정** | ~20 |
| `web/src/pages/dashboard/DashboardPage.tsx` | **수정** (메트릭 쿼리 연결) | ~15 |
| `web/src/services/api/monitorService.ts` | **검증** (이미 구현됨, 응답 타입 확인) | ~5 |

#### 기존 의존성 (이미 구현됨)

- `web/src/services/api/monitorService.ts` - getMetrics() 호출 함수 존재
- `web/src/pages/dashboard/widgets/ResourceWidget.tsx` - UI 셸 존재 (데이터 연결 필요)
- `internal/api/handler/flow.go` - 플로우 핸들러 (참고용)
- `internal/api/handler/agent.go` - 에이전트 핸들러 (참고용)

---

### 4.7 Milestone 5E: WebSocket 실시간 업데이트 (Backend)

#### 목표

백엔드에 WebSocket 핸들러를 구현하여 프론트엔드의 실시간 업데이트 인프라를 활성화한다. 프론트엔드의 `wsClient.ts`와 `wsHandlers.ts`는 이미 구현되어 있으므로, 백엔드에서 WebSocket 연결을 수락하고 메시지를 전송하는 핸들러만 추가한다.

#### 요구 기능

**Backend:**
1. **WebSocket 핸들러 구현**
   - 경로: `/api/v1/ws`
   - 프로토콜: gorilla/websocket 또는 nhooyr.io/websocket
   - 연결 관리: 클라이언트 등록/해제, 동시 접속 지원

2. **메시지 타입 정의**
   ```
   {
     "type": "flow_status" | "flow_metrics" | "agent_status" | "system_event" | "log",
     "payload": { ... },
     "timestamp": "2026-03-03T12:00:00Z"
   }
   ```

3. **지원 메시지 타입**
   - `flow_status`: 플로우 상태 변경 이벤트 (Running, Stopped, Error 등)
   - `flow_metrics`: 플로우별 처리량, 에러율 등 메트릭
   - `agent_status`: 에이전트 연결/해제 이벤트
   - `system_event`: 시스템 이벤트 (배포, 재시작 등)
   - `log`: 실시간 로그 메시지

4. **하트비트**
   - 서버 측 30초 간격 ping 전송
   - 클라이언트 pong 미응답 시 연결 종료

#### 파일 목록

| 파일 경로 | 작업 | 예상 라인 |
|----------|------|----------|
| `internal/api/handler/websocket.go` | **신규** | ~150 |
| `internal/api/ws/hub.go` | **신규** (WebSocket 허브, 클라이언트 관리) | ~120 |
| `internal/api/ws/client.go` | **신규** (WebSocket 클라이언트 래퍼) | ~100 |
| `internal/api/ws/message.go` | **신규** (메시지 타입 정의) | ~40 |
| `internal/api/server.go` | **수정** (WebSocket 라우트 등록) | +5 |
| `go.mod` | **수정** (WebSocket 라이브러리 의존성) | +1 |

#### 기존 의존성 (이미 구현됨)

- `web/src/services/ws/wsClient.ts` - WebSocket 클라이언트 (연결/재연결/하트비트)
- `web/src/services/ws/wsHandlers.ts` - 메시지 타입 라우팅 핸들러
- `web/src/hooks/useWebSocket.ts` - WebSocket 연결 관리 훅

---

## 5. 파일 변경 요약

### 신규 파일 (Frontend)

| 파일 | 마일스톤 | 설명 |
|------|---------|------|
| `web/src/pages/flows/FlowStatusBadge.tsx` | 5A | 플로우 상태 뱃지 컴포넌트 |
| `web/src/pages/flows/FlowActionMenu.tsx` | 5A | 플로우 액션 드롭다운 메뉴 |
| `web/src/pages/flows/FlowSearchFilter.tsx` | 5A | 검색 및 필터 바 |
| `web/src/pages/agents/AgentListPage.tsx` | 5B | 에이전트 목록 페이지 |
| `web/src/pages/agents/AgentStatusBadge.tsx` | 5B | 에이전트 상태 뱃지 |
| `web/src/pages/agents/AgentActionButtons.tsx` | 5B | 에이전트 액션 버튼 그룹 |
| `web/src/pages/agents/CreateAgentModal.tsx` | 5B | 에이전트 생성 모달 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | 5B | 에이전트 상세 통계 패널 |
| `web/src/pages/nodes/NodeTypesPage.tsx` | 5C | 노드 타입 브라우저 페이지 |
| `web/src/pages/nodes/NodeTypeCard.tsx` | 5C | 노드 타입 카드 컴포넌트 |
| `web/src/pages/nodes/NodeCategoryTabs.tsx` | 5C | 카테고리 탭 필터 |

### 신규 파일 (Backend)

| 파일 | 마일스톤 | 설명 |
|------|---------|------|
| `internal/api/handler/monitor.go` | 5D | 메트릭 API 핸들러 |
| `internal/api/handler/websocket.go` | 5E | WebSocket 핸들러 |
| `internal/api/ws/hub.go` | 5E | WebSocket 허브 (연결 관리) |
| `internal/api/ws/client.go` | 5E | WebSocket 클라이언트 래퍼 |
| `internal/api/ws/message.go` | 5E | WebSocket 메시지 타입 |

### 수정 파일

| 파일 | 마일스톤 | 변경 내용 |
|------|---------|----------|
| `web/src/pages/flows/FlowListPage.tsx` | 5A | 플레이스홀더 -> 전체 구현 |
| `web/src/router.tsx` | 5B, 5C | `/agents`, `/nodes` 라우트 추가 |
| `web/src/components/layout/Sidebar.tsx` | 5B, 5C | "에이전트", "노드" 메뉴 추가 |
| `web/src/lib/i18n/ko.json` | 5B, 5C | 에이전트/노드 번역 키 추가 |
| `web/src/lib/i18n/en.json` | 5B, 5C | 에이전트/노드 번역 키 추가 |
| `web/src/pages/dashboard/widgets/ResourceWidget.tsx` | 5D | 실제 메트릭 데이터 연결 |
| `web/src/pages/dashboard/DashboardPage.tsx` | 5D | 메트릭 쿼리 연결 |
| `internal/api/server.go` | 5D, 5E | metrics, ws 라우트 등록 |
| `go.mod` | 5E | WebSocket 라이브러리 추가 |

---

## 6. 의존성 그래프

```
기존 구현 (Milestone 1-4)
├── 타입 정의 (types/*)
├── API 클라이언트 (services/api/client.ts, interceptors.ts)
│   ├── flowService.ts (완전 구현)
│   ├── agentService.ts (완전 구현)
│   ├── nodeService.ts (완전 구현)
│   └── monitorService.ts (완전 구현, 백엔드 미구현)
├── React Query 훅
│   ├── useFlow.ts (완전 구현)
│   ├── useAgent.ts (완전 구현)
│   └── useNodeTypes.ts (완전 구현)
├── WebSocket 클라이언트 (ws/wsClient.ts, wsHandlers.ts)
└── 모든 스토어, 레이아웃, 기존 페이지

Milestone 5 신규 작업
├── 5A FlowListPage ← flowService, useFlow, CreateFlowModal (기존)
├── 5B AgentListPage ← agentService, useAgent (기존)
├── 5C NodeTypesPage ← nodeService, useNodeTypes (기존)
├── 5D Metrics API ← Go runtime, flow/agent registry (기존)
│   └── ResourceWidget ← monitorService.getMetrics() (기존)
└── 5E WebSocket Handler ← gorilla/websocket (신규 의존성)
    └── wsClient.ts, wsHandlers.ts (기존, 연결 대기 중)
```

---

## 7. 리스크 분석

### Risk 1: WebSocket 라이브러리 선택

- **설명**: Go 생태계에서 WebSocket 라이브러리 선택 (`gorilla/websocket` vs `nhooyr.io/websocket`). gorilla/websocket은 메인테이너 부재로 아카이브되었다가 커뮤니티에서 재활성화됨
- **영향**: 낮음
- **대응**: gorilla/websocket이 여전히 Go 생태계에서 가장 널리 사용되며 안정적. 대안으로 nhooyr.io/websocket도 호환 가능. 프론트엔드 wsClient.ts는 표준 WebSocket API를 사용하므로 백엔드 라이브러리와 무관

### Risk 2: 메트릭 데이터 정확성

- **설명**: Go runtime.MemStats 기반 메트릭이 실제 시스템 리소스 사용량과 차이가 있을 수 있음
- **영향**: 중간
- **대응**: 초기 구현은 Go runtime 메트릭으로 시작하고, 추후 `/proc/stat` (Linux) 등 OS 레벨 메트릭으로 확장 가능. CPU 사용률은 goroutine 수와 GOMAXPROCS 기반으로 추정치 제공

### Risk 3: 에이전트 생성 폼 유효성 검증

- **설명**: 에이전트 타입별 config 스키마가 동적이어서 프론트엔드에서 JSON 입력의 유효성을 완벽히 검증하기 어려움
- **영향**: 낮음
- **대응**: 초기 구현에서는 JSON 포맷 유효성만 검증하고, 상세 스키마 검증은 백엔드에 위임. 추후 에이전트 타입별 config schema API를 추가하여 동적 폼 생성 가능

### Risk 4: 대규모 플로우 목록 페이지네이션

- **설명**: 백엔드 GET /api/v1/flows의 페이지네이션 파라미터 지원 여부에 따라 프론트엔드 구현이 달라짐
- **영향**: 낮음
- **대응**: 백엔드가 `page`, `size` 쿼리 파라미터를 지원하는지 확인. 미지원 시 프론트엔드에서 전체 목록을 가져와 클라이언트 사이드 페이지네이션 적용

### Risk 5: WebSocket과 기존 프론트엔드 통합

- **설명**: wsClient.ts와 wsHandlers.ts가 특정 메시지 포맷을 기대하고 있을 수 있어, 백엔드 구현과 불일치할 수 있음
- **영향**: 중간
- **대응**: 백엔드 WebSocket 메시지 포맷을 프론트엔드 wsHandlers.ts의 기대 포맷에 맞추어 구현. 구현 전 wsHandlers.ts의 메시지 파싱 로직을 먼저 검토하여 스키마 결정

### Risk 6: 사이드바 메뉴 증가에 따른 UI 복잡성

- **설명**: Agents, Nodes 메뉴 추가로 사이드바 메뉴가 6개로 증가하여 접힌 상태에서 가독성 저하
- **영향**: 낮음
- **대응**: 아이콘만 표시하는 접힌 모드에서 각 메뉴의 아이콘이 직관적으로 구분되도록 적절한 lucide-react 아이콘 선택. 필요시 메뉴 그룹핑(섹션 구분선) 적용

---

## 8. 코드 규모 예상

| 마일스톤 | 신규 파일 | 수정 파일 | 예상 라인 (신규) |
|---------|----------|----------|----------------|
| 5A Flow List | 3 | 1 | ~430 |
| 5B Agent Management | 5 | 4 | ~570 |
| 5C Node Browser | 3 | 4 | ~290 |
| 5D Dashboard Data | 1 (backend) | 4 | ~125 |
| 5E WebSocket | 4 (backend) | 2 | ~410 |
| **합계** | **16** | **15** | **~1,825** |

---

## 9. 인증(Auth) 관련 참고사항

현재 AuthGuard는 passthrough (모든 요청 허용) 상태이며, 백엔드에 인증 API가 존재하지 않는다. 인증 기능은 본 Milestone 5의 범위에 포함되지 않으며, 별도 SPEC에서 다룰 예정이다. Milestone 5의 모든 프론트엔드 페이지는 AuthGuard가 passthrough인 상태에서 정상 동작하도록 구현한다.

---

*SPEC ID: SPEC-WEB-001*
*버전: 2.0.0*
*상태: in-progress*
*최종 수정: 2026-03-03*
