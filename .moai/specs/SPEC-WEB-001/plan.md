---
id: SPEC-WEB-001
type: plan
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
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

## 2. 파일별 구현 상세

### 2.1 P0 핵심 파일 (1차 목표)

#### `web/src/types/api.ts` (~60 라인)
- `APIResponse<T>` 제네릭 응답 타입
- `ErrorDetail` 에러 상세 타입
- `Meta`, `PaginationMeta` 타입
- `APIError` 클래스 (프론트엔드 에러 래핑)
- 의존성: 없음 (순수 타입 정의)

#### `web/src/types/flow.ts` (~80 라인)
- `FlowDefinition`, `FlowNode`, `FlowWire`, `Port` 타입
- `FlowStatus` 유니온 타입 ('Draft' | 'Deployed' | 'Running' | 'Stopped' | 'Error')
- `NodeData`, `FlowSettings` 타입
- React Flow 노드/엣지 타입 변환 유틸리티 타입
- 의존성: 없음 (순수 타입 정의)

#### `web/src/types/auth.ts` (~40 라인)
- `User`, `UserRole`, `AuthTokens` 타입
- `LoginRequest`, `LoginResponse` 타입
- 의존성: 없음 (순수 타입 정의)

#### `web/src/types/node.ts` (~50 라인)
- `NodeType`, `NodeCategory` 타입 (Input/Output/Process/Bridge/Special)
- `NodeTypeDefinition` 타입 (아이콘, 포트 스키마, 설정 스키마)
- `ConfigSchema`, `ConfigField` 타입 (동적 폼 생성용)
- 의존성: 없음 (순수 타입 정의)

#### `web/src/services/api/client.ts` (~80 라인)
- axios/ky 인스턴스 생성 (baseURL, timeout, headers)
- 요청/응답 인터셉터 등록
- 에러 변환 함수
- 의존성: axios 또는 ky

#### `web/src/services/api/interceptors.ts` (~100 라인)
- 요청 인터셉터: Authorization 헤더 자동 추가
- 응답 인터셉터: 401 시 토큰 갱신 + 원래 요청 재시도
- 토큰 갱신 대기 큐 (동시 401 요청 처리)
- 의존성: client.ts, authStore.ts

#### `web/src/services/api/authService.ts` (~60 라인)
- `login(email, password)`: POST /api/v1/auth/login
- `logout()`: POST /api/v1/auth/logout
- `refreshToken(refreshToken)`: POST /api/v1/auth/refresh
- 의존성: client.ts, types/auth.ts

#### `web/src/services/api/flowService.ts` (~120 라인)
- `getFlows(params)`: GET /api/v1/flows (페이지네이션)
- `getFlow(id)`: GET /api/v1/flows/:id
- `createFlow(data)`: POST /api/v1/flows
- `updateFlow(id, data)`: PUT /api/v1/flows/:id
- `deleteFlow(id)`: DELETE /api/v1/flows/:id
- `deployFlow(id)`: POST /api/v1/flows/:id/deploy
- `startFlow(id)`: POST /api/v1/flows/:id/start
- `stopFlow(id)`: POST /api/v1/flows/:id/stop
- `restartFlow(id)`: POST /api/v1/flows/:id/restart
- 의존성: client.ts, types/flow.ts, types/api.ts

#### `web/src/services/api/nodeService.ts` (~40 라인)
- `getNodeTypes()`: GET /api/v1/nodes/types
- 의존성: client.ts, types/node.ts

#### `web/src/services/api/agentService.ts` (~60 라인)
- `getAgents(params)`: GET /api/v1/agents
- `getAgent(id)`: GET /api/v1/agents/:id
- 의존성: client.ts, types/api.ts

#### `web/src/services/api/monitorService.ts` (~40 라인)
- `getMetrics()`: GET /api/v1/monitor/metrics
- `setLogLevel(level)`: PUT /api/v1/monitor/loglevel
- 의존성: client.ts, types/api.ts

#### `web/src/services/ws/wsClient.ts` (~120 라인)
- WebSocket 클래스: 연결/해제, 자동 재연결, 하트비트
- 이벤트 기반 메시지 핸들링 (EventEmitter 패턴)
- 재연결 로직 (5초 간격, 최대 10회, exponential backoff)
- 연결 상태 관리 (connecting, connected, disconnected, reconnecting)
- 의존성: 없음

#### `web/src/services/ws/wsHandlers.ts` (~60 라인)
- WebSocket 메시지 타입 라우팅
- 메트릭/로그/상태 업데이트 핸들러
- 의존성: wsClient.ts

#### `web/src/stores/authStore.ts` (~80 라인)
- Zustand 스토어: user, tokens, isAuthenticated, isLoading
- 액션: login, logout, setTokens, refreshTokens
- 토큰 메모리 저장 (localStorage 사용 금지)
- 의존성: zustand, types/auth.ts

#### `web/src/stores/uiStore.ts` (~50 라인)
- Zustand 스토어: sidebarCollapsed, theme, notifications
- 액션: toggleSidebar, setTheme, addNotification, dismissNotification
- localStorage 연동 (사이드바, 테마 설정 유지)
- 의존성: zustand

#### `web/src/stores/editorStore.ts` (~150 라인)
- Zustand 스토어: nodes, edges, selectedNodeId, selectedEdgeId, isDirty
- Undo/Redo 스택 (최대 50 히스토리)
- 액션: setNodes, setEdges, addNode, removeNode, updateNodeData, addEdge, removeEdge
- React Flow 상태와 동기화
- 의존성: zustand, types/flow.ts

#### `web/src/hooks/useAuth.ts` (~40 라인)
- 인증 상태 접근 훅
- 로그인/로그아웃 핸들러
- 인증 가드 로직
- 의존성: authStore.ts, authService.ts

#### `web/src/hooks/useFlow.ts` (~80 라인)
- React Query 기반 플로우 쿼리/뮤테이션 훅
- `useFlows()`, `useFlow(id)`, `useCreateFlow()`, `useUpdateFlow()`, `useDeleteFlow()`
- `useDeployFlow()`, `useStartFlow()`, `useStopFlow()`
- 쿼리 무효화 로직
- 의존성: @tanstack/react-query, flowService.ts

#### `web/src/hooks/useWebSocket.ts` (~60 라인)
- WebSocket 연결 관리 훅
- 자동 연결/해제 (페이지 마운트/언마운트)
- 메시지 구독 인터페이스
- 연결 상태 반환
- 의존성: wsClient.ts

#### `web/src/hooks/useTheme.ts` (~30 라인)
- 테마 상태 접근 및 전환 훅
- 시스템 테마 감지 (prefers-color-scheme)
- 의존성: uiStore.ts

#### `web/src/lib/utils/cn.ts` (~10 라인)
- Tailwind 클래스 병합 유틸리티 (clsx + tailwind-merge)
- 의존성: clsx, tailwind-merge

#### `web/src/lib/utils/format.ts` (~40 라인)
- 날짜 포맷팅 함수
- 숫자 포맷팅 함수 (바이트, 퍼센트 등)
- 의존성: 없음

#### `web/src/App.tsx` (~60 라인)
- React Query Provider, Theme Provider 래핑
- App Shell 레이아웃 (Sidebar + Header + Main)
- 의존성: router.tsx, components/layout/*

#### `web/src/router.tsx` (~50 라인)
- react-router 라우트 설정
- lazy 로딩 + Suspense
- 인증 가드 라우트
- 의존성: react-router, pages/*

#### `web/src/main.tsx` (~20 라인)
- React DOM 렌더 엔트리포인트
- StrictMode 래핑
- 의존성: App.tsx

#### `web/src/components/layout/Sidebar.tsx` (~100 라인)
- 사이드바 네비게이션 메뉴
- 접기/펼치기 기능
- 활성 메뉴 하이라이트
- RBAC 기반 메뉴 필터링
- 의존성: react-router, uiStore.ts, lucide-react

#### `web/src/components/layout/Header.tsx` (~60 라인)
- 헤더 바: 페이지 제목, 사용자 정보, 로그아웃 버튼
- WebSocket 연결 상태 표시기
- 의존성: authStore.ts, useWebSocket.ts

#### `web/src/components/layout/ErrorBoundary.tsx` (~50 라인)
- React Error Boundary
- 폴백 UI (에러 메시지, 새로고침 버튼)
- 에러 로깅
- 의존성: React

#### `web/src/components/palette/NodePalette.tsx` (~80 라인)
- 노드 팔레트 컨테이너
- 검색 필드
- 카테고리별 노드 목록
- 의존성: NodeCategory.tsx, NodeItem.tsx, useFlow.ts

#### `web/src/components/palette/NodeCategory.tsx` (~40 라인)
- 아코디언 카테고리 헤더
- 접기/펼치기 상태
- 의존성: NodeItem.tsx

#### `web/src/components/palette/NodeItem.tsx` (~40 라인)
- 드래그 가능한 노드 항목
- HTML5 Drag API 핸들러
- 아이콘, 이름, 설명 표시
- 의존성: lucide-react

#### `web/src/components/property/PropertyPanel.tsx` (~80 라인)
- 속성 패널 컨테이너
- 선택된 노드/엣지에 따른 동적 렌더링
- "노드를 선택하세요" 빈 상태
- 의존성: DynamicForm.tsx, editorStore.ts

#### `web/src/components/property/DynamicForm.tsx` (~100 라인)
- 노드 설정 스키마 기반 동적 폼 렌더러
- 폼 상태 관리 및 유효성 검증
- onChange 콜백으로 에디터 스토어 업데이트
- 의존성: FormField.tsx, editorStore.ts

#### `web/src/components/property/FormField.tsx` (~80 라인)
- 타입별 폼 필드 렌더러 (text, number, select, toggle, textarea)
- 인라인 에러 메시지
- aria 속성 적용
- 의존성: shadcn/ui 컴포넌트

#### `web/src/components/script/LuaEditor.tsx` (~60 라인)
- Monaco Editor 래퍼
- Lua 구문 강조 설정
- 기본 자동 완성 (Lua 키워드)
- 크기 조절 가능 (resizable)
- 의존성: @monaco-editor/react

#### `web/src/components/flow/CustomNode.tsx` (~100 라인)
- React Flow 커스텀 노드 컴포넌트
- 노드 아이콘, 이름, 상태 표시기
- 입력/출력 Handle 렌더링
- 선택 상태 스타일링
- 의존성: @xyflow/react, NodeHandle.tsx

#### `web/src/components/flow/CustomEdge.tsx` (~50 라인)
- React Flow 커스텀 엣지 컴포넌트
- 와이어 스타일링 (활성/비활성)
- 삭제 버튼 (hover 시)
- 의존성: @xyflow/react

#### `web/src/components/flow/NodeHandle.tsx` (~40 라인)
- 커스텀 포트(Handle) 컴포넌트
- 포트 타입별 색상/모양
- 호환성 검증 시각적 피드백
- 의존성: @xyflow/react

#### `web/src/components/flow/EditorToolbar.tsx` (~60 라인)
- 에디터 상단 도구 모음
- 저장/배포/시작/중지/재시작/Undo/Redo 버튼
- 플로우 상태 표시
- 의존성: editorStore.ts, useFlow.ts, lucide-react

#### `web/src/pages/auth/LoginPage.tsx` (~100 라인)
- 로그인 폼 (이메일/비밀번호)
- 클라이언트 유효성 검증
- 로딩/에러 상태 처리
- 의존성: useAuth.ts, shadcn/ui

#### `web/src/pages/editor/EditorPage.tsx` (~150 라인)
- React Flow 캔버스
- NodePalette + PropertyPanel 레이아웃
- 플로우 로드/저장 로직
- Ctrl+S 단축키
- beforeunload 경고
- 의존성: @xyflow/react, editorStore.ts, useFlow.ts, NodePalette, PropertyPanel, EditorToolbar

### 2.2 P1 확장 파일 (2차 목표)

#### `web/src/pages/dashboard/DashboardPage.tsx` (~120 라인)
- 대시보드 레이아웃
- 위젯 그리드
- 데이터 로딩 (병렬 API 호출)
- 의존성: useFlow.ts, useWebSocket.ts, widgets/*

#### `web/src/pages/dashboard/widgets/` (~200 라인, 4-5 파일)
- `SystemStatusWidget.tsx`: Running/Stopped/Error 플로우 수
- `RecentFlowsWidget.tsx`: 최근 활동 플로우 목록
- `ResourceWidget.tsx`: CPU, 메모리 사용률 차트
- `AgentStatusWidget.tsx`: 활성 에이전트 수
- 의존성: recharts, useFlow.ts

#### `web/src/pages/monitoring/MonitoringPage.tsx` (~100 라인)
- 모니터링 레이아웃
- WebSocket 연결 관리
- 탭 구성 (메트릭/로그/이벤트)
- 의존성: useWebSocket.ts, MetricsChart, LogViewer, EventTimeline

#### `web/src/pages/monitoring/MetricsChart.tsx` (~80 라인)
- recharts 기반 실시간 메트릭 차트
- 최근 5분 데이터 유지
- CPU/메모리/처리량/에러율 차트
- 의존성: recharts, useWebSocket.ts

#### `web/src/pages/monitoring/LogViewer.tsx` (~100 라인)
- 실시간 로그 스트리밍 뷰어
- 가상 스크롤 (virtualization)
- 로그 레벨 필터
- 자동 스크롤 토글
- 의존성: useWebSocket.ts

#### `web/src/pages/monitoring/EventTimeline.tsx` (~60 라인)
- 이벤트 타임라인 (상태 변경, 배포, 에러)
- 시간순 정렬
- 의존성: useWebSocket.ts

#### `web/src/pages/settings/SettingsPage.tsx` (~120 라인)
- 설정 섹션 탭 (프로필/시스템/테마/언어)
- RBAC 기반 섹션 접근 제어
- 의존성: useAuth.ts, monitorService.ts

#### `web/src/components/script/LuaEditor.tsx` (P1 확장)
- Lua 고급 자동 완성 (XFlow 내장 함수)
- 에러 마커 표시
- 의존성: @monaco-editor/react

### 2.3 P2 선택 파일 (3차 목표)

#### `web/src/lib/theme/themeProvider.tsx` (~50 라인)
- 테마 프로바이더 컴포넌트
- 다크/라이트 전환 로직
- 시스템 테마 감지
- 의존성: React, uiStore.ts

#### `web/src/lib/i18n/index.ts` (~40 라인)
- i18n 설정 및 초기화
- 언어 전환 함수
- 의존성: i18next 또는 자체 구현

#### `web/src/lib/i18n/ko.json` (~200 라인)
- 한국어 번역 리소스
- 의존성: 없음

#### `web/src/lib/i18n/en.json` (~200 라인)
- 영어 번역 리소스
- 의존성: 없음

### 2.4 설정 파일

#### `web/vite.config.ts` (~40 라인)
- React 플러그인, alias 설정
- 프록시 설정 (개발 환경 API)
- 빌드 최적화 (코드 스플리팅)

#### `web/tsconfig.json` (~30 라인)
- TypeScript strict 모드
- path alias (@/ -> src/)
- JSX preserve 설정

#### `web/tailwind.config.ts` (~30 라인)
- 커스텀 색상, 다크 모드 설정
- shadcn/ui 플러그인 통합

#### `web/package.json` (~50 라인)
- 의존성 목록
- 스크립트 (dev, build, test, lint)

---

## 3. 마일스톤

### 1차 목표 (Primary Goal): 앱 기반 + API 통신

- 프로젝트 초기 설정 (Vite, TypeScript, Tailwind, ESLint)
- 타입 정의 (api.ts, flow.ts, auth.ts, node.ts)
- API 클라이언트 및 인터셉터 구현
- WebSocket 클라이언트 구현
- Zustand 스토어 3종 (auth, ui, editor)
- React Query 훅 구현

### 2차 목표 (Secondary Goal): App Shell + 인증 + 에디터

- App Shell 레이아웃 (Sidebar, Header, ErrorBoundary)
- 라우팅 설정 (인증 가드 포함)
- 로그인 페이지
- 플로우 에디터 페이지 (React Flow 캔버스)
- 커스텀 노드/엣지/핸들 컴포넌트
- Node Palette + Property Panel
- 에디터 도구 모음 (저장/배포/Undo/Redo)

### 3차 목표 (Tertiary Goal): 대시보드 + 모니터링 + 설정

- 대시보드 페이지 및 위젯
- 모니터링 페이지 (메트릭 차트, 로그 뷰어, 이벤트 타임라인)
- 설정 페이지
- Lua 스크립트 에디터 고급 기능

### 4차 목표 (Optional Goal): 테마 + i18n + 최적화

- 다크/라이트 테마 전환
- 다국어 지원 (ko/en)
- 성능 최적화 (번들 크기, 렌더링)
- E2E 테스트 (Playwright)

---

## 4. 의존성 그래프

```
타입 정의 (types/*)
    └── API 클라이언트 (services/api/client.ts, interceptors.ts)
        ├── 도메인 서비스 (flowService, authService, nodeService, ...)
        │   └── React Query 훅 (hooks/useFlow.ts, ...)
        │       └── 페이지 컴포넌트 (pages/*)
        └── 인증 스토어 (stores/authStore.ts)
            └── 인증 훅 (hooks/useAuth.ts)
                └── App Shell (App.tsx, router.tsx)
                    └── 모든 페이지

WebSocket 클라이언트 (services/ws/wsClient.ts)
    └── WebSocket 훅 (hooks/useWebSocket.ts)
        ├── 모니터링 페이지
        └── 대시보드 위젯

에디터 스토어 (stores/editorStore.ts)
    ├── 에디터 페이지 (pages/editor/EditorPage.tsx)
    ├── Property Panel (components/property/*)
    └── 에디터 도구 모음 (components/flow/EditorToolbar.tsx)

UI 스토어 (stores/uiStore.ts)
    ├── Sidebar (components/layout/Sidebar.tsx)
    └── 테마 훅 (hooks/useTheme.ts)
```

---

## 5. 리스크 분석

### Risk 1: React Flow 커스터마이징 복잡성
- **설명**: @xyflow/react의 커스텀 노드/엣지 렌더링, 포트 호환성 검증, Undo/Redo 통합이 예상보다 복잡할 수 있음
- **영향**: 높음
- **대응**: React Flow 공식 문서와 예제를 먼저 검증. 커스텀 노드 프로토타입을 1차 목표에서 먼저 구현하여 위험 조기 식별. Undo/Redo는 Zustand middleware(immer)로 구현

### Risk 2: SPEC-API-001 백엔드 미구현 상태
- **설명**: 프론트엔드 개발 시 백엔드 API가 아직 구현되지 않았을 수 있음
- **영향**: 중간
- **대응**: MSW(Mock Service Worker) 또는 json-server로 API 모킹. API 응답 타입은 SPEC-API-001의 DTO 정의를 기반으로 미리 작성. 백엔드 구현 완료 시 모킹 레이어만 제거

### Risk 3: WebSocket 실시간 성능
- **설명**: 대량의 실시간 데이터(메트릭, 로그)가 브라우저 성능에 영향을 미칠 수 있음
- **영향**: 중간
- **대응**: 로그 뷰어에 가상 스크롤(virtualization) 적용. 메트릭 데이터는 최근 5분만 유지하고 오래된 데이터는 자동 폐기. WebSocket 메시지 배치 처리(throttle)로 렌더 빈도 제한

### Risk 4: 500 노드 에디터 렌더링 성능
- **설명**: 대규모 플로우(500 노드)에서 React Flow 렌더링 성능 저하 우려
- **영향**: 중간
- **대응**: React.memo로 노드 컴포넌트 메모이제이션. React Flow의 viewport 밖 노드 비렌더링 기능 활용. 필요 시 노드 그룹화/축소 기능 추가

### Risk 5: 토큰 갱신 동시성
- **설명**: 여러 API 요청이 동시에 401을 받으면 토큰 갱신 요청이 중복 발생 가능
- **영향**: 낮음
- **대응**: 토큰 갱신 대기 큐 패턴 적용. 첫 번째 401에서만 갱신 요청을 보내고, 나머지 요청은 갱신 완료를 대기 후 재시도

### Risk 6: 노드 설정 스키마 동적 폼
- **설명**: 다양한 노드 타입별 설정 스키마를 동적 폼으로 렌더링하는 복잡성
- **영향**: 중간
- **대응**: JSON Schema 기반 동적 폼 렌더러 패턴 적용. 기본 필드 타입(text, number, select, toggle)을 먼저 구현하고, 복잡한 커스텀 필드는 점진적으로 추가

---

## 6. 파일 의존성 매트릭스

| 파일 | 의존하는 내부 파일 | 의존하는 외부 패키지 | 의존하는 SPEC |
|------|-------------------|---------------------|---------------|
| types/api.ts | - | - | SPEC-API-001 |
| types/flow.ts | - | @xyflow/react | SPEC-FLOW-001 |
| types/auth.ts | - | - | SPEC-AUTH-001 |
| types/node.ts | - | - | SPEC-NODE-001 |
| services/api/client.ts | - | axios/ky | - |
| services/api/interceptors.ts | client.ts, stores/authStore | axios/ky | - |
| services/api/authService.ts | client.ts, types/auth | - | SPEC-AUTH-001 |
| services/api/flowService.ts | client.ts, types/flow, types/api | - | SPEC-API-001 |
| services/api/nodeService.ts | client.ts, types/node | - | SPEC-NODE-001 |
| services/api/agentService.ts | client.ts, types/api | - | SPEC-AGENT-001 |
| services/api/monitorService.ts | client.ts, types/api | - | SPEC-OBS-001 |
| services/ws/wsClient.ts | - | - | SPEC-API-001 |
| services/ws/wsHandlers.ts | wsClient.ts | - | - |
| stores/authStore.ts | types/auth | zustand | SPEC-AUTH-001 |
| stores/uiStore.ts | - | zustand | - |
| stores/editorStore.ts | types/flow | zustand | - |
| hooks/useAuth.ts | stores/authStore, services/api/authService | - | - |
| hooks/useFlow.ts | services/api/flowService, types/flow | @tanstack/react-query | - |
| hooks/useWebSocket.ts | services/ws/wsClient | react | - |
| hooks/useTheme.ts | stores/uiStore | react | - |
| App.tsx | router.tsx, components/layout/* | react, @tanstack/react-query | - |
| router.tsx | pages/*, hooks/useAuth | react-router | - |
| pages/auth/LoginPage.tsx | hooks/useAuth | react, shadcn/ui | - |
| pages/editor/EditorPage.tsx | stores/editorStore, hooks/useFlow, components/flow/*, palette/*, property/* | @xyflow/react | - |
| pages/dashboard/DashboardPage.tsx | hooks/useFlow, hooks/useWebSocket, widgets/* | - | - |
| pages/monitoring/MonitoringPage.tsx | hooks/useWebSocket | - | - |
| pages/settings/SettingsPage.tsx | hooks/useAuth, services/api/monitorService | - | - |
| components/flow/CustomNode.tsx | types/flow, components/flow/NodeHandle | @xyflow/react, lucide-react | - |
| components/palette/NodePalette.tsx | components/palette/NodeCategory, hooks/useFlow | - | SPEC-NODE-001 |
| components/property/PropertyPanel.tsx | stores/editorStore, components/property/DynamicForm | - | - |
| components/script/LuaEditor.tsx | stores/editorStore | @monaco-editor/react | - |

---

## 7. 총 예상 코드 규모

| 카테고리 | 파일 수 | 예상 라인 수 |
|----------|---------|-------------|
| P0 핵심 파일 | ~35 | ~2,800 |
| P1 확장 파일 | ~10 | ~900 |
| P2 선택 파일 | ~4 | ~490 |
| 설정 파일 | ~4 | ~150 |
| 테스트 파일 | ~20+ | ~2,000+ |
| **합계** | **~73+** | **~6,340+** |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-02-13*
