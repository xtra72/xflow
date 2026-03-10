# Plan: Per-Panel Settings (Title + Field Selection)

## Context

대시보드 3개 패널(FlowPanel, AgentPanel, ResourceWidget)의 타이틀이 하드코딩되어 있고, 테이블 컬럼/메트릭 항목을 사용자가 선택할 수 없다. ResourceWidget의 메트릭 선택은 글로벌 편집 모드 바에 있어 패널과 분리되어 있다.

**목표**: 각 패널 내부에서 타이틀 변경 + 표시 필드 선택이 가능하도록 한다.

## Step 1: uiStore 타입 및 상태 확장

**File:** `web/src/stores/uiStore.ts`

타입 추가:
```typescript
export const ALL_FLOW_COLUMNS = ['name', 'status', 'node_count', 'updated_at', 'actions'] as const;
export type FlowColumnKey = (typeof ALL_FLOW_COLUMNS)[number];

export const ALL_AGENT_COLUMNS = ['name', 'type', 'status', 'uptime', 'messages', 'actions'] as const;
export type AgentColumnKey = (typeof ALL_AGENT_COLUMNS)[number];
```

상태 추가 (persist 대상):
- `flowPanelTitle: string` (기본: '플로우 현황')
- `flowVisibleColumns: FlowColumnKey[]` (기본: 전체)
- `agentPanelTitle: string` (기본: '에이전트 현황')
- `agentVisibleColumns: AgentColumnKey[]` (기본: 전체)
- `resourcePanelTitle: string` (기본: '프로세스 리소스')
- `dashboardVisibleMetrics`: 기존 유지 (ResourceWidget용)

액션 추가:
- `setFlowPanelTitle`, `setFlowVisibleColumns`
- `setAgentPanelTitle`, `setAgentVisibleColumns`
- `setResourcePanelTitle`

`resetDashboardLayout`에 새 필드 초기화 포함. `partialize`에 새 필드 추가.

## Step 2: PanelSettingsDropdown 공통 컴포넌트

**File:** `web/src/components/common/PanelSettingsDropdown.tsx` (신규)

Props:
```typescript
interface PanelSettingsDropdownProps<T extends string> {
  title: string;
  onTitleChange: (title: string) => void;
  columns: { key: T; label: string }[];
  visibleColumns: T[];
  onColumnsChange: (columns: T[]) => void;
}
```

구현:
- Settings(gear) 아이콘 버튼 → 드롭다운 토글
- 타이틀 입력 필드 (빈 값 방지: blur 시 이전값 복원)
- 구분선
- "표시 항목" 레이블 + 체크박스 목록
- 최소 1개 필드 보호 (마지막 체크박스 disabled)
- 외부 클릭 닫기 (useRef + mousedown listener)

## Step 3: FlowPanel 수정

**File:** `web/src/pages/dashboard/panels/FlowPanel.tsx`

- store에서 `flowPanelTitle`, `flowVisibleColumns` 읽기
- 헤더: 하드코딩 "플로우 현황" → store title + PanelSettingsDropdown
- 테이블 `<th>`/`<td>` 각각 `visibleColumns.includes(key)` 조건 렌더링
- 숨겨진 컬럼으로 정렬 중이면 기본(name)으로 fallback

## Step 4: AgentPanel 수정

**File:** `web/src/pages/dashboard/panels/AgentPanel.tsx`

FlowPanel과 동일 패턴 적용.

## Step 5: ResourceWidget 수정

**File:** `web/src/pages/dashboard/widgets/ResourceWidget.tsx`

- `visibleMetrics` prop 제거 → store에서 직접 읽기
- 헤더: 하드코딩 "프로세스 리소스" → store title + PanelSettingsDropdown
- 메트릭 선택을 패널 내부로 이동

## Step 6: DashboardPage 정리

**File:** `web/src/pages/dashboard/DashboardPage.tsx`

- 글로벌 편집 바에서 메트릭 체크박스/라벨 제거 (초기화 버튼만 유지)
- `visibleMetrics`, `setVisibleMetrics`, `toggleMetric`, `METRIC_LABELS`, `ALL_METRIC_KEYS` 관련 코드 제거
- `<ResourceWidget>` 에서 `visibleMetrics` prop 제거

## Files to Modify (5) + Create (1)

1. `web/src/stores/uiStore.ts` - 타입/상태/액션/persist 확장
2. `web/src/components/common/PanelSettingsDropdown.tsx` - 신규 공통 컴포넌트
3. `web/src/pages/dashboard/panels/FlowPanel.tsx` - 타이틀+컬럼 설정
4. `web/src/pages/dashboard/panels/AgentPanel.tsx` - 타이틀+컬럼 설정
5. `web/src/pages/dashboard/widgets/ResourceWidget.tsx` - 타이틀+메트릭 설정 (패널 내부)
6. `web/src/pages/dashboard/DashboardPage.tsx` - 글로벌 메트릭 UI 제거

## Verification

1. `npx tsc --noEmit` - TypeScript 검증
2. 각 패널 기어 아이콘 클릭 → 타이틀 변경 + 필드 체크박스 동작 확인
3. 최소 1개 필드 보호 확인 (마지막 체크박스 해제 불가)
4. 브라우저 새로고침 → 설정 유지 확인
5. 레이아웃 초기화 → 타이틀/필드도 기본값 복원 확인
