---
id: SPEC-WEB-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-WEB-001: Web Dashboard - React SPA, 플로우 에디터, 실시간 모니터링, 관리 UI

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 Web Dashboard는 순수 프론트엔드 SPA(Single Page Application)로, Go 백엔드 데몬(`xflowd`)과 REST API 및 WebSocket을 통해 통신한다. 사용자는 브라우저를 통해 IoT Flow-Based Programming 플로우를 시각적으로 설계, 배포, 모니터링할 수 있다.

본 SPEC은 다음을 포함한다:

- **App Shell & Routing** (`App.tsx`, `router.tsx`): SPA 루트 레이아웃, 사이드바 네비게이션, 라우팅 구성, 인증 가드
- **Authentication Pages** (`pages/auth/`): 로그인, 로그아웃, 토큰 갱신, 세션 관리 UI
- **Flow Editor** (`pages/editor/`): React Flow 기반 드래그 앤 드롭 플로우 에디터, 커스텀 노드/엣지 렌더링, 줌/팬
- **API Client Service** (`services/api/`): axios/ky 기반 HTTP 클라이언트, 인터셉터, 에러 핸들링, WebSocket 클라이언트
- **State Management** (`stores/`): Zustand 기반 글로벌 상태, React Query 서버 상태 캐싱
- **Dashboard Page** (`pages/dashboard/`): 시스템 개요, 플로우 목록, 상태 요약 위젯
- **Monitoring Panel** (`pages/monitoring/`): 실시간 메트릭 차트, 로그 스트리밍, 이벤트 타임라인
- **Node Palette** (`components/palette/`): 노드 타입 카탈로그, 드래그 가능한 노드 목록, 검색/필터
- **Property Panel** (`components/property/`): 선택된 노드/엣지의 설정 편집 폼, 유효성 검증
- **Settings Page** (`pages/settings/`): 사용자 설정, 시스템 설정, 로그 레벨 변경
- **Script Editor** (`components/script/`): Monaco 기반 Lua 스크립트 에디터, 구문 강조, 자동 완성
- **Theme & i18n** (`lib/theme/`, `lib/i18n/`): 다크/라이트 테마 전환, 다국어 지원 기반 구조

### 1.2 기술 환경

- **언어**: TypeScript 5.x
- **패키지 경로**: `web/src/`
- **빌드 도구**: Vite 6.x
- **UI 프레임워크**: React 19.x + React DOM 19.x
- **노드 에디터**: @xyflow/react 12.x+ (React Flow)
- **CSS**: Tailwind CSS 4.x
- **컴포넌트 라이브러리**: shadcn/ui + Radix UI
- **상태 관리**: Zustand 5.x (클라이언트 상태), @tanstack/react-query 5.x (서버 상태)
- **HTTP 클라이언트**: axios 또는 ky (최신)
- **라우팅**: react-router 7.x
- **아이콘**: lucide-react (최신)
- **차트/시각화**: recharts 또는 @tremor/react (최신)
- **코드 에디터**: @monaco-editor/react (최신) - Lua 스크립트 편집
- **테스트 프레임워크**:
  - 단위/컴포넌트: Vitest 3.x + @testing-library/react (최신)
  - E2E: Playwright (최신)
- **린팅/포맷팅**: ESLint 9.x + Prettier 3.x
- **의존 SPEC**:
  - SPEC-API-001: REST API 엔드포인트 (모든 CRUD 및 실행 제어)
  - SPEC-AUTH-001: JWT 인증/인가, RBAC (Admin, Editor, Viewer)
  - SPEC-FLOW-001: 플로우 정의 구조체 (Flow, Node, Wire 타입)
  - SPEC-MSG-001: 메시지 타입 (Payload, Metadata, History)
  - SPEC-NODE-001: 노드 타입 카탈로그 (Input/Output/Process/Bridge/Special)
  - SPEC-AGENT-001: Agent 관리 (MQTT, Modbus, HTTP, File 등)
  - SPEC-OBS-001: 관찰성 시스템 (메트릭, 로그, 트레이스)

### 1.3 설계 원칙

- **순수 프론트엔드 SPA**: 모든 비즈니스 로직은 백엔드(xflowd)에 존재하며, Web Dashboard는 API를 통해서만 데이터를 조회/조작한다
- **컴포넌트 기반 아키텍처**: 재사용 가능한 UI 컴포넌트를 합성하여 페이지를 구성한다
- **타입 안전성**: TypeScript strict 모드를 사용하여 컴파일 타임에 타입 오류를 방지한다
- **서버 상태 분리**: React Query로 서버 상태를 관리하고, Zustand로 순수 클라이언트 상태만 관리한다
- **낙관적 업데이트**: 사용자 경험 향상을 위해 가능한 경우 낙관적 업데이트를 적용한다
- **반응형 디자인**: 최소 1024px 해상도부터 지원하며, Tailwind CSS 반응형 유틸리티를 활용한다
- **접근성**: WCAG 2.1 AA 수준을 준수하며, 키보드 네비게이션과 스크린 리더를 지원한다
- **관찰성 내장**: 프론트엔드 에러 추적, 성능 메트릭, 사용자 액션 로깅을 포함한다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- App Shell: SPA 레이아웃, 사이드바 네비게이션, 라우팅, 인증 가드, 에러 바운더리
- Authentication: 로그인/로그아웃 UI, 토큰 관리, 세션 갱신 로직
- Flow Editor: React Flow 기반 시각적 플로우 에디터, 커스텀 노드/엣지, 줌/팬/미니맵
- API Client: REST API 통신 모듈, WebSocket 클라이언트, 인터셉터, 에러 핸들링
- State Management: Zustand 스토어, React Query 쿼리/뮤테이션 설정
- Dashboard: 시스템 개요 페이지, 플로우 목록, 상태 요약
- Monitoring: 실시간 메트릭 차트, 로그 스트리밍 뷰어, 이벤트 타임라인
- Node Palette: 드래그 가능한 노드 목록, 카테고리 필터, 검색
- Property Panel: 노드/엣지 설정 편집 폼, 유효성 검증
- Settings: 사용자/시스템 설정 페이지
- Script Editor: Monaco 기반 Lua 에디터
- Theme & i18n: 테마 전환, 다국어 기반 구조

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- 백엔드 비즈니스 로직 (xflowd 서버, SPEC-API-001 이하 모든 백엔드 SPEC)
- 모바일 네이티브 앱
- 오프라인 모드 / PWA
- 복잡한 대시보드 커스터마이징 (사용자 위젯 드래그 배치)
- 멀티 테넌시
- 실시간 협업 편집 (Conflict-free 동시 편집)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| SPA | Single Page Application, 단일 HTML에서 JavaScript로 페이지 전환을 처리하는 웹 앱 |
| Flow Editor | React Flow 기반의 시각적 플로우 편집 영역 |
| Node | 플로우 내의 처리 단위 (입력/출력/프로세스/브릿지/특수 노드) |
| Wire | 노드 간 메시지를 전달하는 연결선 (React Flow의 Edge에 해당) |
| Port | 노드의 입력/출력 연결 지점 (React Flow의 Handle에 해당) |
| Agent | 외부 시스템과 브릿지 역할을 하는 연결 관리자 (MQTT, Modbus 등) |
| Node Palette | 사용 가능한 노드 타입을 카테고리별로 나열한 드래그 소스 패널 |
| Property Panel | 선택한 노드/엣지의 설정을 편집하는 사이드 패널 |
| Dashboard | 시스템 전체 상태를 요약하여 보여주는 메인 페이지 |
| RBAC | Role-Based Access Control, 역할 기반 접근 제어 (Admin/Editor/Viewer) |
| JWT | JSON Web Token, 인증에 사용되는 토큰 형식 (Access + Refresh) |
| WebSocket | 양방향 실시간 통신 프로토콜 (메트릭/로그/상태 스트리밍) |
| Optimistic Update | 서버 응답 전에 UI를 먼저 업데이트하고, 실패 시 롤백하는 패턴 |
| Characterization Test | 기존 동작을 보존하기 위한 테스트 (DDD PRESERVE 단계) |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-WEB-001: 백엔드 REST API(SPEC-API-001)가 표준 응답 엔벨로프 `{success, data, error, meta}` 형식을 따른다
- A-WEB-002: JWT 인증(SPEC-AUTH-001)은 Access Token + Refresh Token 쌍으로 동작하며, Access Token 만료 시 자동 갱신된다
- A-WEB-003: WebSocket 연결은 `ws://` 또는 `wss://` 프로토콜로 xflowd 서버에 직접 연결되며, 인증 토큰을 쿼리 파라미터 또는 첫 메시지로 전달한다
- A-WEB-004: 노드 타입 카탈로그(SPEC-NODE-001)는 REST API를 통해 조회 가능하며, 각 노드 타입의 아이콘, 포트 정의, 설정 스키마를 포함한다
- A-WEB-005: xflowd 서버가 `web/dist/` 빌드 결과물을 정적 파일로 서빙한다
- A-WEB-006: React Flow(@xyflow/react 12.x+)는 커스텀 노드 컴포넌트, 커스텀 엣지 컴포넌트, Handle(Port) 커스터마이징을 지원한다

### 3.2 운영 가정

- A-WEB-007: 최소 지원 해상도는 1024x768이며, 권장 해상도는 1920x1080이다
- A-WEB-008: 지원 브라우저는 Chrome(최신-2), Firefox(최신-2), Safari(최신-2), Edge(최신-2)이다
- A-WEB-009: 플로우 에디터에서 동시에 표시 가능한 최대 노드 수는 500개이다
- A-WEB-010: WebSocket을 통한 실시간 메트릭 업데이트 주기는 1초 이하이다

### 3.3 보안 가정

- A-WEB-011: 모든 API 요청은 Authorization 헤더에 Bearer 토큰을 포함하며, 401 응답 시 자동으로 토큰 갱신을 시도한다
- A-WEB-012: XSS 방지를 위해 사용자 입력은 항상 이스케이프 처리되며, `dangerouslySetInnerHTML`은 사용하지 않는다
- A-WEB-013: RBAC에 따라 Admin은 모든 기능, Editor는 플로우 편집/배포, Viewer는 조회만 가능하다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: App Shell & Routing (P0) - App.tsx, router.tsx

#### REQ-WEB-001-01-01 (Ubiquitous)
시스템은 **항상** SPA 루트 레이아웃으로 사이드바 네비게이션, 헤더 바, 메인 콘텐츠 영역으로 구성된 App Shell을 렌더링해야 한다.

#### REQ-WEB-001-01-02 (Ubiquitous)
시스템은 **항상** react-router 기반의 선언적 라우팅을 제공하며, 다음 경로를 포함해야 한다:
- `/` - Dashboard (메인 페이지)
- `/flows` - 플로우 목록
- `/editor/:flowId` - 플로우 에디터
- `/monitoring` - 모니터링 패널
- `/settings` - 설정 페이지
- `/login` - 로그인 페이지

#### REQ-WEB-001-01-03 (State-Driven)
**IF** 사용자가 인증되지 않은 상태 **THEN** 보호된 라우트 접근 시 `/login` 페이지로 리다이렉트해야 한다.

#### REQ-WEB-001-01-04 (Event-Driven)
**WHEN** 라우트 전환 시, **THEN** 사이드바의 활성 메뉴 항목이 현재 경로에 맞게 하이라이트되어야 한다.

#### REQ-WEB-001-01-05 (Ubiquitous)
시스템은 **항상** 렌더링 에러 발생 시 Error Boundary를 통해 폴백 UI를 표시하고, 에러 정보를 콘솔에 로깅해야 한다.

#### REQ-WEB-001-01-06 (Ubiquitous)
시스템은 **항상** Suspense를 활용한 코드 스플리팅으로 각 페이지를 lazy 로딩하며, 로딩 중 스켈레톤 UI를 표시해야 한다.

#### REQ-WEB-001-01-07 (Ubiquitous)
시스템은 **항상** 사이드바를 접기/펼치기(collapse/expand) 할 수 있어야 하며, 사용자 설정을 localStorage에 저장해야 한다.

#### REQ-WEB-001-01-08 (Ubiquitous)
시스템은 **항상** 키보드 네비게이션을 지원하며, Tab 키로 인터랙티브 요소 간 이동이 가능해야 한다.

### 4.2 Module 2: Authentication Pages (P0) - pages/auth/

#### REQ-WEB-001-02-01 (Ubiquitous)
시스템은 **항상** 이메일/비밀번호 기반 로그인 폼을 제공하며, 클라이언트 측 유효성 검증(필수 입력, 이메일 형식)을 수행해야 한다.

#### REQ-WEB-001-02-02 (Event-Driven)
**WHEN** 로그인 폼 제출 시, **THEN** POST /api/v1/auth/login 엔드포인트를 호출하고, 성공 시 Access Token과 Refresh Token을 저장해야 한다.

#### REQ-WEB-001-02-03 (Event-Driven)
**WHEN** 로그인 성공 시, **THEN** 이전에 접근하려던 페이지(returnUrl)로 리다이렉트하거나, 기본값으로 Dashboard(`/`)로 이동해야 한다.

#### REQ-WEB-001-02-04 (Event-Driven)
**WHEN** 로그인 실패 시, **THEN** 서버 에러 메시지를 사용자에게 표시하며, 비밀번호 필드를 초기화해야 한다.

#### REQ-WEB-001-02-05 (Event-Driven)
**WHEN** Access Token이 만료되었을 때, **THEN** Refresh Token을 사용하여 자동으로 새 Access Token을 발급받고, 실패한 요청을 재시도해야 한다.

#### REQ-WEB-001-02-06 (Event-Driven)
**WHEN** Refresh Token 갱신이 실패하면, **THEN** 사용자를 로그아웃 처리하고 `/login` 페이지로 리다이렉트하며, "세션이 만료되었습니다" 메시지를 표시해야 한다.

#### REQ-WEB-001-02-07 (Event-Driven)
**WHEN** 로그아웃 버튼 클릭 시, **THEN** 저장된 토큰을 삭제하고, POST /api/v1/auth/logout을 호출하며, `/login` 페이지로 이동해야 한다.

#### REQ-WEB-001-02-08 (Ubiquitous)
시스템은 **항상** 토큰을 httpOnly 쿠키 또는 메모리에 저장하며, localStorage에 직접 저장하지 않아야 한다.

### 4.3 Module 3: Flow Editor (P0) - pages/editor/

#### REQ-WEB-001-03-01 (Ubiquitous)
시스템은 **항상** React Flow(@xyflow/react) 기반의 시각적 플로우 에디터를 제공하며, 캔버스 줌(zoom), 팬(pan), 피트(fit view) 기능을 지원해야 한다.

#### REQ-WEB-001-03-02 (Event-Driven)
**WHEN** 플로우 에디터 페이지(`/editor/:flowId`)에 진입하면, **THEN** GET /api/v1/flows/:flowId 엔드포인트를 호출하여 플로우 정의를 로드하고, 노드와 와이어(엣지)를 캔버스에 렌더링해야 한다.

#### REQ-WEB-001-03-03 (Event-Driven)
**WHEN** Node Palette에서 노드를 캔버스로 드래그 앤 드롭하면, **THEN** 해당 노드 타입의 인스턴스를 플로우에 추가하고, 기본 설정값으로 초기화해야 한다.

#### REQ-WEB-001-03-04 (Event-Driven)
**WHEN** 노드의 출력 포트(Handle)에서 다른 노드의 입력 포트로 드래그하면, **THEN** 새로운 와이어(Edge)를 생성하여 두 노드를 연결해야 한다.

#### REQ-WEB-001-03-05 (Unwanted)
시스템은 호환되지 않는 포트 타입 간의 연결을 **허용하지 않아야 한다**. 연결 시도 시 시각적 피드백(빨간색 하이라이트)을 표시해야 한다.

#### REQ-WEB-001-03-06 (Event-Driven)
**WHEN** 캔버스에서 노드를 선택(클릭)하면, **THEN** Property Panel에 해당 노드의 설정 폼이 표시되어야 한다.

#### REQ-WEB-001-03-07 (Event-Driven)
**WHEN** "저장" 버튼 클릭 또는 Ctrl+S 단축키 입력 시, **THEN** PUT /api/v1/flows/:flowId 엔드포인트를 호출하여 현재 플로우 상태를 서버에 저장해야 한다.

#### REQ-WEB-001-03-08 (Event-Driven)
**WHEN** "배포" 버튼 클릭 시, **THEN** POST /api/v1/flows/:flowId/deploy 엔드포인트를 호출하고, 배포 진행 상태를 표시하며, 완료 시 토스트 알림을 표시해야 한다.

#### REQ-WEB-001-03-09 (Event-Driven)
**WHEN** 배포된 플로우에서 "시작/중지/재시작" 버튼 클릭 시, **THEN** 해당 제어 API(start/stop/restart)를 호출하고, 플로우 상태 변화를 UI에 반영해야 한다.

#### REQ-WEB-001-03-10 (Ubiquitous)
시스템은 **항상** 플로우 에디터에 미니맵(MiniMap) 컴포넌트를 표시하여 전체 플로우의 축소 미리보기를 제공해야 한다.

#### REQ-WEB-001-03-11 (Ubiquitous)
시스템은 **항상** 플로우 에디터 상단에 도구 모음(Toolbar)을 제공하며, 저장/배포/실행 제어/실행 취소(Undo)/다시 실행(Redo) 버튼을 포함해야 한다.

#### REQ-WEB-001-03-12 (Event-Driven)
**WHEN** 노드 또는 와이어를 선택 후 Delete 키를 누르면, **THEN** 확인 다이얼로그 없이 해당 요소를 플로우에서 제거하고, Undo로 복원 가능해야 한다.

#### REQ-WEB-001-03-13 (State-Driven)
**IF** 플로우에 미저장 변경사항이 있는 상태 **THEN** 페이지를 떠나려 할 때 브라우저 beforeunload 경고를 표시해야 한다.

#### REQ-WEB-001-03-14 (Ubiquitous)
시스템은 **항상** 각 노드 타입에 대해 커스텀 노드 컴포넌트를 렌더링하며, 노드 아이콘, 이름, 포트(Handle), 상태 표시기를 포함해야 한다.

### 4.4 Module 4: API Client Service (P0) - services/api/

#### REQ-WEB-001-04-01 (Ubiquitous)
시스템은 **항상** 중앙화된 API 클라이언트 인스턴스를 제공하며, base URL, 타임아웃(30초), Content-Type 헤더를 기본 설정으로 포함해야 한다.

#### REQ-WEB-001-04-02 (Ubiquitous)
시스템은 **항상** 요청 인터셉터를 통해 Authorization 헤더에 `Bearer {accessToken}`을 자동으로 추가해야 한다.

#### REQ-WEB-001-04-03 (Event-Driven)
**WHEN** API 응답이 401 Unauthorized인 경우, **THEN** 응답 인터셉터에서 토큰 갱신을 시도하고, 성공 시 원래 요청을 재시도해야 한다.

#### REQ-WEB-001-04-04 (Event-Driven)
**WHEN** API 요청이 네트워크 오류로 실패하면, **THEN** 설정된 횟수(최대 3회)까지 자동 재시도하고, 재시도 간격을 지수적으로 증가(exponential backoff)시켜야 한다.

#### REQ-WEB-001-04-05 (Ubiquitous)
시스템은 **항상** API 응답의 표준 엔벨로프 `{success, data, error, meta}`를 파싱하고, 에러 시 일관된 에러 객체로 변환해야 한다.

#### REQ-WEB-001-04-06 (Ubiquitous)
시스템은 **항상** WebSocket 클라이언트를 제공하며, 연결 수립/해제, 자동 재연결(5초 간격, 최대 10회), 하트비트(30초 간격) 기능을 포함해야 한다.

#### REQ-WEB-001-04-07 (Event-Driven)
**WHEN** WebSocket 연결이 끊어지면, **THEN** 자동 재연결을 시도하고, 재연결 상태를 UI 상단에 표시해야 한다.

#### REQ-WEB-001-04-08 (Ubiquitous)
시스템은 **항상** 각 도메인(Flow, Agent, Node, Monitor)별 API 서비스 모듈을 제공하여, 엔드포인트 호출을 타입 안전하게 래핑해야 한다.

### 4.5 Module 5: State Management (P0) - stores/

#### REQ-WEB-001-05-01 (Ubiquitous)
시스템은 **항상** Zustand 스토어를 통해 다음 클라이언트 상태를 관리해야 한다:
- `authStore`: 인증 상태 (토큰, 사용자 정보, 로그인 상태)
- `uiStore`: UI 상태 (사이드바 접힘, 테마, 알림)
- `editorStore`: 에디터 상태 (선택된 노드/엣지, 편집 모드, Undo/Redo 스택)

#### REQ-WEB-001-05-02 (Ubiquitous)
시스템은 **항상** React Query를 통해 서버 상태를 관리하며, 플로우 목록, 플로우 상세, 노드 타입 카탈로그, 에이전트 목록, 메트릭 데이터에 대한 쿼리 키를 정의해야 한다.

#### REQ-WEB-001-05-03 (Event-Driven)
**WHEN** 서버 데이터가 변경(mutation 성공)되면, **THEN** 관련 쿼리를 자동으로 무효화(invalidate)하여 최신 데이터를 다시 가져와야 한다.

#### REQ-WEB-001-05-04 (Ubiquitous)
시스템은 **항상** 에디터 스토어에서 Undo/Redo 기능을 지원하며, 최대 50개의 히스토리를 유지해야 한다.

#### REQ-WEB-001-05-05 (Event-Driven)
**WHEN** React Query 캐시가 stale 상태가 되면, **THEN** 윈도우 포커스 시 자동으로 refetch해야 한다.

### 4.6 Module 6: Dashboard Page (P1) - pages/dashboard/

#### REQ-WEB-001-06-01 (Ubiquitous)
시스템은 **항상** 대시보드 페이지에 다음 위젯을 표시해야 한다:
- 시스템 상태 요약 (Running/Stopped/Error 플로우 수)
- 최근 활동 플로우 목록 (최대 10개)
- 시스템 리소스 개요 (CPU, 메모리 사용률)
- 활성 에이전트 수

#### REQ-WEB-001-06-02 (Event-Driven)
**WHEN** 대시보드 페이지 진입 시, **THEN** GET /api/v1/flows, GET /api/v1/monitor/metrics 엔드포인트를 병렬로 호출하여 데이터를 로드해야 한다.

#### REQ-WEB-001-06-03 (Event-Driven)
**WHEN** 플로우 목록에서 플로우를 클릭하면, **THEN** `/editor/:flowId` 페이지로 이동해야 한다.

#### REQ-WEB-001-06-04 (Event-Driven)
**WHEN** "새 플로우" 버튼 클릭 시, **THEN** 플로우 생성 모달을 표시하고, 이름/설명 입력 후 POST /api/v1/flows를 호출하여 플로우를 생성해야 한다.

#### REQ-WEB-001-06-05 (State-Driven)
**IF** WebSocket이 연결된 상태 **THEN** 대시보드 위젯의 데이터를 실시간으로 업데이트해야 한다.

### 4.7 Module 7: Monitoring Panel (P1) - pages/monitoring/

#### REQ-WEB-001-07-01 (Ubiquitous)
시스템은 **항상** 모니터링 페이지에 다음 섹션을 제공해야 한다:
- 메트릭 차트 (CPU, 메모리, 처리량, 에러율)
- 실시간 로그 뷰어
- 플로우별 상태 테이블
- 이벤트 타임라인

#### REQ-WEB-001-07-02 (Event-Driven)
**WHEN** 모니터링 페이지 진입 시, **THEN** WebSocket 연결을 수립하고 실시간 메트릭 스트리밍을 시작해야 한다.

#### REQ-WEB-001-07-03 (Event-Driven)
**WHEN** WebSocket으로 새 메트릭 데이터가 수신되면, **THEN** 해당 차트를 실시간으로 업데이트하며, 최근 5분간의 데이터를 유지해야 한다.

#### REQ-WEB-001-07-04 (Event-Driven)
**WHEN** WebSocket으로 새 로그 메시지가 수신되면, **THEN** 로그 뷰어에 새 항목을 추가하고, 자동 스크롤(auto-scroll)을 적용해야 한다.

#### REQ-WEB-001-07-05 (Event-Driven)
**WHEN** 로그 뷰어에서 로그 레벨 필터(DEBUG/INFO/WARN/ERROR)를 변경하면, **THEN** 선택된 레벨 이상의 로그만 표시해야 한다.

#### REQ-WEB-001-07-06 (Event-Driven)
**WHEN** 모니터링 페이지를 떠나면, **THEN** WebSocket 연결을 정리하고 메트릭 스트리밍을 중지해야 한다.

#### REQ-WEB-001-07-07 (Unwanted)
시스템은 로그 뷰어가 과도한 로그(초당 1000건 이상)로 인해 브라우저 성능이 저하되는 것을 **방지해야 한다**. 가상 스크롤(virtualization)을 적용해야 한다.

### 4.8 Module 8: Node Palette (P0) - components/palette/

#### REQ-WEB-001-08-01 (Ubiquitous)
시스템은 **항상** 플로우 에디터 왼쪽에 Node Palette 패널을 표시하며, 사용 가능한 노드 타입을 카테고리별(Input, Output, Process, Bridge, Special)로 그룹화해야 한다.

#### REQ-WEB-001-08-02 (Event-Driven)
**WHEN** 에디터 페이지 로드 시, **THEN** GET /api/v1/nodes/types 엔드포인트를 호출하여 노드 타입 카탈로그를 로드해야 한다.

#### REQ-WEB-001-08-03 (Event-Driven)
**WHEN** 검색 필드에 텍스트를 입력하면, **THEN** 노드 이름/설명에 대해 실시간 필터링을 수행하여 일치하는 노드만 표시해야 한다.

#### REQ-WEB-001-08-04 (Ubiquitous)
시스템은 **항상** 각 노드 항목에 아이콘, 이름, 간략 설명을 표시하며, HTML5 Drag API를 통해 캔버스로 드래그가 가능해야 한다.

#### REQ-WEB-001-08-05 (Event-Driven)
**WHEN** Node Palette에서 카테고리 헤더를 클릭하면, **THEN** 해당 카테고리의 노드 목록을 접기/펼치기(accordion)해야 한다.

### 4.9 Module 9: Property Panel (P0) - components/property/

#### REQ-WEB-001-09-01 (Ubiquitous)
시스템은 **항상** 플로우 에디터 오른쪽에 Property Panel을 표시하며, 선택된 노드 또는 엣지의 설정을 편집할 수 있는 동적 폼을 렌더링해야 한다.

#### REQ-WEB-001-09-02 (Event-Driven)
**WHEN** 노드를 선택하면, **THEN** 해당 노드 타입의 설정 스키마에 따라 폼 필드(텍스트, 숫자, 선택, 토글 등)를 동적으로 생성해야 한다.

#### REQ-WEB-001-09-03 (Event-Driven)
**WHEN** Property Panel에서 설정값을 변경하면, **THEN** 에디터 스토어의 노드 데이터를 즉시 업데이트하고, 캔버스의 노드 렌더링에 반영해야 한다.

#### REQ-WEB-001-09-04 (Ubiquitous)
시스템은 **항상** 폼 입력에 대해 클라이언트 측 유효성 검증을 수행하며, 오류 시 해당 필드에 인라인 에러 메시지를 표시해야 한다.

#### REQ-WEB-001-09-05 (State-Driven)
**IF** 아무 노드도 선택되지 않은 상태 **THEN** Property Panel에 "노드를 선택하세요" 안내 메시지를 표시해야 한다.

#### REQ-WEB-001-09-06 (Event-Driven)
**WHEN** ScriptNode 타입을 선택하면, **THEN** Property Panel에 Monaco 기반 Lua 스크립트 에디터를 인라인으로 렌더링해야 한다.

### 4.10 Module 10: Settings Page (P1) - pages/settings/

#### REQ-WEB-001-10-01 (Ubiquitous)
시스템은 **항상** 설정 페이지에 다음 섹션을 제공해야 한다:
- 사용자 프로필 (이름, 이메일, 비밀번호 변경)
- 시스템 설정 (로그 레벨, API 서버 정보)
- 테마 설정 (라이트/다크)
- 언어 설정 (한국어/영어)

#### REQ-WEB-001-10-02 (Event-Driven)
**WHEN** 설정 변경 후 "저장" 버튼 클릭 시, **THEN** 해당 API를 호출하여 서버에 설정을 저장하고, 성공/실패 토스트를 표시해야 한다.

#### REQ-WEB-001-10-03 (State-Driven)
**IF** 사용자 역할이 Viewer인 경우 **THEN** 시스템 설정 섹션을 비활성화(disabled)하고, "관리자 권한이 필요합니다" 안내를 표시해야 한다.

#### REQ-WEB-001-10-04 (Event-Driven)
**WHEN** 로그 레벨 변경 요청 시, **THEN** PUT /api/v1/monitor/loglevel 엔드포인트를 호출하여 런타임 로그 레벨을 변경해야 한다.

### 4.11 Module 11: Script Editor (P1) - components/script/

#### REQ-WEB-001-11-01 (Ubiquitous)
시스템은 **항상** ScriptNode의 Lua 스크립트 편집을 위해 Monaco Editor를 제공하며, 구문 강조(syntax highlighting)를 지원해야 한다.

#### REQ-WEB-001-11-02 (Ubiquitous)
시스템은 **항상** Lua 기본 키워드 자동 완성(autocomplete)을 제공해야 한다.

#### REQ-WEB-001-11-03 (Event-Driven)
**WHEN** 스크립트 내용을 변경하면, **THEN** 에디터 스토어에 변경 사항을 반영하고, 플로우 저장 시 함께 저장되어야 한다.

#### REQ-WEB-001-11-04 (Ubiquitous)
시스템은 **항상** 스크립트 에디터 영역의 크기를 드래그로 조절 가능하게(resizable) 해야 한다.

### 4.12 Module 12: Theme & i18n (P2) - lib/theme/, lib/i18n/

#### REQ-WEB-001-12-01 (Ubiquitous)
시스템은 **항상** 라이트 테마와 다크 테마를 지원하며, Tailwind CSS의 dark 모드 클래스를 활용해야 한다.

#### REQ-WEB-001-12-02 (Event-Driven)
**WHEN** 테마 전환 토글 클릭 시, **THEN** 즉시 전체 UI의 테마를 변경하고, 사용자 설정을 localStorage에 저장해야 한다.

#### REQ-WEB-001-12-03 (Optional)
**가능하면** 시스템 테마(prefers-color-scheme) 설정을 감지하여 초기 테마를 자동으로 설정해야 한다.

#### REQ-WEB-001-12-04 (Ubiquitous)
시스템은 **항상** 다국어 지원을 위한 기반 구조(i18n)를 갖추며, 초기에는 한국어(ko)와 영어(en)를 지원해야 한다.

#### REQ-WEB-001-12-05 (Event-Driven)
**WHEN** 언어 설정 변경 시, **THEN** 전체 UI의 텍스트를 선택된 언어로 전환해야 한다.

---

## 5. Specifications (기술 사양)

### 5.1 프로젝트 구조

```
web/
├── index.html
├── vite.config.ts
├── tsconfig.json
├── tailwind.config.ts
├── package.json
└── src/
    ├── main.tsx                    # 앱 엔트리포인트
    ├── App.tsx                     # 루트 App Shell
    ├── router.tsx                  # 라우트 설정
    ├── vite-env.d.ts               # Vite 타입 선언
    ├── components/
    │   ├── ui/                     # shadcn/ui 컴포넌트
    │   ├── layout/
    │   │   ├── Sidebar.tsx         # 사이드바 네비게이션
    │   │   ├── Header.tsx          # 헤더 바
    │   │   └── ErrorBoundary.tsx   # 에러 바운더리
    │   ├── palette/
    │   │   ├── NodePalette.tsx     # 노드 팔레트 컨테이너
    │   │   ├── NodeCategory.tsx    # 카테고리 아코디언
    │   │   └── NodeItem.tsx        # 드래그 가능한 노드 항목
    │   ├── property/
    │   │   ├── PropertyPanel.tsx   # 속성 패널 컨테이너
    │   │   ├── DynamicForm.tsx     # 동적 폼 렌더러
    │   │   └── FormField.tsx       # 폼 필드 컴포넌트
    │   ├── script/
    │   │   └── LuaEditor.tsx       # Monaco Lua 에디터
    │   ├── flow/
    │   │   ├── CustomNode.tsx      # 커스텀 노드 컴포넌트
    │   │   ├── CustomEdge.tsx      # 커스텀 엣지 컴포넌트
    │   │   ├── NodeHandle.tsx      # 커스텀 포트(Handle)
    │   │   └── EditorToolbar.tsx   # 에디터 도구 모음
    │   └── common/
    │       ├── LoadingSpinner.tsx  # 로딩 스피너
    │       ├── Toast.tsx           # 토스트 알림
    │       └── ConfirmDialog.tsx   # 확인 다이얼로그
    ├── pages/
    │   ├── auth/
    │   │   └── LoginPage.tsx       # 로그인 페이지
    │   ├── dashboard/
    │   │   ├── DashboardPage.tsx   # 대시보드 메인
    │   │   └── widgets/            # 대시보드 위젯
    │   ├── editor/
    │   │   └── EditorPage.tsx      # 플로우 에디터 페이지
    │   ├── monitoring/
    │   │   ├── MonitoringPage.tsx  # 모니터링 메인
    │   │   ├── MetricsChart.tsx    # 메트릭 차트
    │   │   ├── LogViewer.tsx       # 로그 뷰어
    │   │   └── EventTimeline.tsx   # 이벤트 타임라인
    │   └── settings/
    │       └── SettingsPage.tsx    # 설정 페이지
    ├── services/
    │   ├── api/
    │   │   ├── client.ts           # API 클라이언트 설정
    │   │   ├── interceptors.ts     # 요청/응답 인터셉터
    │   │   ├── flowService.ts      # 플로우 API
    │   │   ├── agentService.ts     # 에이전트 API
    │   │   ├── nodeService.ts      # 노드 API
    │   │   ├── monitorService.ts   # 모니터링 API
    │   │   └── authService.ts      # 인증 API
    │   └── ws/
    │       ├── wsClient.ts         # WebSocket 클라이언트
    │       └── wsHandlers.ts       # WebSocket 메시지 핸들러
    ├── stores/
    │   ├── authStore.ts            # 인증 상태
    │   ├── uiStore.ts              # UI 상태
    │   └── editorStore.ts          # 에디터 상태 (Undo/Redo 포함)
    ├── hooks/
    │   ├── useAuth.ts              # 인증 훅
    │   ├── useFlow.ts              # 플로우 React Query 훅
    │   ├── useWebSocket.ts         # WebSocket 훅
    │   └── useTheme.ts             # 테마 훅
    ├── types/
    │   ├── flow.ts                 # 플로우 관련 타입 (Node, Wire, Port)
    │   ├── api.ts                  # API 응답 타입 (APIResponse, PaginationMeta)
    │   ├── auth.ts                 # 인증 관련 타입 (User, Token, Role)
    │   └── node.ts                 # 노드 타입 카탈로그 타입
    ├── lib/
    │   ├── theme/
    │   │   └── themeProvider.tsx    # 테마 프로바이더
    │   ├── i18n/
    │   │   ├── index.ts            # i18n 설정
    │   │   ├── ko.json             # 한국어 리소스
    │   │   └── en.json             # 영어 리소스
    │   └── utils/
    │       ├── cn.ts               # Tailwind 클래스 병합 유틸리티
    │       └── format.ts           # 날짜/숫자 포맷 유틸리티
    └── __tests__/                  # 테스트 파일 (미러 구조)
```

### 5.2 핵심 타입 정의

```typescript
// types/api.ts
interface APIResponse<T> {
  success: boolean;
  data: T;
  error?: ErrorDetail;
  meta?: Meta;
}

interface ErrorDetail {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

interface Meta {
  pagination?: PaginationMeta;
  requestId?: string;
}

interface PaginationMeta {
  page: number;
  size: number;
  total: number;
  totalPages: number;
}

// types/flow.ts
interface FlowDefinition {
  id: string;
  name: string;
  description: string;
  status: FlowStatus;
  nodes: FlowNode[];
  wires: FlowWire[];
  settings: FlowSettings;
  createdAt: string;
  updatedAt: string;
}

type FlowStatus = 'Draft' | 'Deployed' | 'Running' | 'Stopped' | 'Error';

interface FlowNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: NodeData;
}

interface NodeData {
  label: string;
  config: Record<string, unknown>;
  ports: { inputs: Port[]; outputs: Port[] };
}

interface Port {
  id: string;
  name: string;
  type: string;
}

interface FlowWire {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
}

// types/auth.ts
type UserRole = 'Admin' | 'Editor' | 'Viewer';

interface User {
  id: string;
  email: string;
  name: string;
  role: UserRole;
}

interface AuthTokens {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}
```

### 5.3 API 엔드포인트 매핑 (SPEC-API-001 참조)

| 기능 | Method | Endpoint | 사용 페이지 |
|------|--------|----------|------------|
| 로그인 | POST | /api/v1/auth/login | LoginPage |
| 로그아웃 | POST | /api/v1/auth/logout | Header |
| 토큰 갱신 | POST | /api/v1/auth/refresh | interceptors |
| 플로우 목록 | GET | /api/v1/flows | Dashboard, FlowList |
| 플로우 상세 | GET | /api/v1/flows/:id | EditorPage |
| 플로우 생성 | POST | /api/v1/flows | Dashboard |
| 플로우 수정 | PUT | /api/v1/flows/:id | EditorPage |
| 플로우 삭제 | DELETE | /api/v1/flows/:id | FlowList |
| 플로우 배포 | POST | /api/v1/flows/:id/deploy | EditorPage |
| 플로우 시작 | POST | /api/v1/flows/:id/start | EditorPage |
| 플로우 중지 | POST | /api/v1/flows/:id/stop | EditorPage |
| 플로우 재시작 | POST | /api/v1/flows/:id/restart | EditorPage |
| 노드 타입 목록 | GET | /api/v1/nodes/types | NodePalette |
| 에이전트 목록 | GET | /api/v1/agents | Dashboard |
| 메트릭 조회 | GET | /api/v1/monitor/metrics | Dashboard, Monitoring |
| 로그 레벨 변경 | PUT | /api/v1/monitor/loglevel | Settings |
| WebSocket 연결 | WS | /api/v1/ws | Monitoring, Dashboard |

### 5.4 성능 목표

| 항목 | 목표 |
|------|------|
| 초기 로딩 시간 (LCP) | < 2초 (Gzip + 코드 스플리팅) |
| 페이지 전환 시간 | < 300ms |
| 플로우 에디터 렌더링 (100 노드) | < 500ms |
| 플로우 에디터 렌더링 (500 노드) | < 2초 |
| WebSocket 메시지 처리 지연 | < 100ms |
| 번들 크기 (gzip) | < 500KB (초기 번들) |
| 메모리 사용량 (500 노드 에디터) | < 200MB |

### 5.5 접근성 요구사항

| 항목 | 기준 |
|------|------|
| WCAG 수준 | 2.1 AA |
| 키보드 네비게이션 | 모든 인터랙티브 요소 접근 가능 |
| 스크린 리더 | aria-label, role 속성 적용 |
| 색상 대비 | 최소 4.5:1 (텍스트), 3:1 (대형 텍스트) |
| 포커스 표시기 | 모든 포커스 가능한 요소에 시각적 표시 |
| 모션 감소 | prefers-reduced-motion 미디어 쿼리 지원 |

### 5.6 보안 요구사항

| 항목 | 구현 |
|------|------|
| XSS 방지 | React의 자동 이스케이프 + CSP 헤더 |
| CSRF 방지 | SameSite 쿠키 + CSRF 토큰 (필요 시) |
| 토큰 저장 | httpOnly 쿠키 또는 메모리 (localStorage 사용 금지) |
| 입력 검증 | Zod 스키마 기반 클라이언트 유효성 검증 |
| RBAC 적용 | 역할에 따른 UI 요소 조건부 렌더링 |
| Content Security Policy | script-src, style-src 화이트리스트 |

### 5.7 Cross-SPEC 의존성 매트릭스

| 본 SPEC 모듈 | 의존 SPEC | 의존 내용 |
|-------------|-----------|----------|
| Authentication Pages | SPEC-AUTH-001 | JWT 토큰 발급/갱신/검증 API |
| Flow Editor | SPEC-FLOW-001 | Flow, Node, Wire 데이터 구조 |
| Flow Editor | SPEC-API-001 | 플로우 CRUD + 실행 제어 API |
| Node Palette | SPEC-NODE-001 | 노드 타입 카탈로그 (Input/Output/Process/Bridge/Special) |
| Property Panel | SPEC-NODE-001 | 노드 설정 스키마 |
| Dashboard | SPEC-API-001 | 플로우 목록, 시스템 상태 API |
| Monitoring Panel | SPEC-OBS-001 | 메트릭, 로그, 이벤트 데이터 |
| Monitoring Panel | SPEC-API-001 | WebSocket 스트리밍 핸들러 |
| Script Editor | SPEC-NODE-001 | ScriptNode Lua 스크립트 구조 |
| Settings | SPEC-API-001 | 로그 레벨 변경, 시스템 정보 API |
| API Client | SPEC-API-001 | 표준 응답 엔벨로프, 에러 코드 |
| State Management | SPEC-AGENT-001 | 에이전트 목록/상태 데이터 |

### 5.8 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (핵심) | App Shell & Routing | SPA 기본 구조, 모든 페이지의 전제 조건 |
| P0 (핵심) | Authentication Pages | 보안 접근 제어의 기반 |
| P0 (핵심) | Flow Editor | 핵심 사용자 기능 (플로우 시각적 편집) |
| P0 (핵심) | API Client Service | 모든 서버 통신의 기반 |
| P0 (핵심) | State Management | 모든 페이지의 데이터 관리 기반 |
| P0 (핵심) | Node Palette | 플로우 에디터의 필수 구성 요소 |
| P0 (핵심) | Property Panel | 플로우 에디터의 필수 구성 요소 |
| P1 (확장) | Dashboard Page | 시스템 개요 및 진입점 |
| P1 (확장) | Monitoring Panel | 실시간 운영 모니터링 |
| P1 (확장) | Settings Page | 시스템/사용자 설정 관리 |
| P1 (확장) | Script Editor | ScriptNode Lua 편집 |
| P2 (선택) | Theme & i18n | 사용자 경험 향상 |

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-02-13*
