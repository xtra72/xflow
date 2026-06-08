// 관리자 뷰 상단 바 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M04~M06).
//
// M9 좌측 디렉토리(20rem) + 우측 대시보드 master/detail 레이아웃을 상단 수평 바 +
// 풀폭 노드 화면으로 진화시킨 관리 크롬이다. 구성(좌→우):
//   - 나가기(exit): 노드 선택 해제(?node=/?tab= 제거) → 노드 피커/빈 상태로 복귀.
//   - 노드 피커(NodePicker): 그룹 묶음 드롭다운(M9 그룹 보존, REQ-M05/OQ-M3).
//   - 서브탭 네비(개요/플로우/에이전트/디바이스/대시보드): M9 NodeDashboard 서브탭을
//     상단 바로 호이스팅(REQ-M06). 노드 선택 시에만 노출. `?tab=` 을 갱신한다.
//   - 관리 액션: 선택 노드 그룹 배정/해제(NodeGroupMenu 재배치, REQ-K02).
//
// 서브탭 상태의 단일 출처는 URL(`?tab=`)이다 — 상단 바와 NodeDashboard 콘텐츠가
// 각자 `?tab=` 을 읽어 동기화된다(프롭 드릴링 없이 일관).

import { Bot, Gauge, HardDrive, LayoutDashboard, LogOut, Workflow } from 'lucide-react';

import { NodeGroupMenu } from '@/components/remote/NodeGroupMenu';
import { NodePicker } from '@/components/remote/NodePicker';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { ManagedNode, NodeGroup } from '@/types/remote';

/** 관리자 뷰 서브탭 식별자(NodeDashboard 와 동일 집합). */
export type ManagerViewTab = 'overview' | 'flows' | 'agents' | 'devices' | 'dashboard';

/** 유효한 서브탭 식별자 집합(URL 파라미터 검증용). */
export const MANAGER_VIEW_TABS: readonly ManagerViewTab[] = [
  'overview',
  'flows',
  'agents',
  'devices',
  'dashboard',
];

/** URL `?tab=` 원시 값을 ManagerViewTab 으로 파싱한다(미지정/무효 → overview). */
export function parseManagerViewTab(raw: string | null): ManagerViewTab {
  return MANAGER_VIEW_TABS.includes(raw as ManagerViewTab)
    ? (raw as ManagerViewTab)
    : 'overview';
}

const TABS: { id: ManagerViewTab; labelKey: string; Icon: typeof Workflow }[] = [
  { id: 'overview', labelKey: 'remote.dashboard.tab.overview', Icon: LayoutDashboard },
  { id: 'dashboard', labelKey: 'remote.dashboard.tab.dashboard', Icon: Gauge },
  { id: 'flows', labelKey: 'remote.dashboard.tab.flows', Icon: Workflow },
  { id: 'agents', labelKey: 'remote.dashboard.tab.agents', Icon: Bot },
  { id: 'devices', labelKey: 'remote.dashboard.tab.devices', Icon: HardDrive },
];

interface ManagerViewTopBarProps {
  /** 정렬된 그룹 목록("전체" 먼저). */
  groups: NodeGroup[];
  /** group_name → 노드 배열 매핑. */
  nodesByGroup: Map<string, ManagedNode[]>;
  /** 현재 선택된 노드(없으면 null). */
  selectedNode: ManagedNode | null;
  /** 활성 서브탭(URL `?tab=` 파생). */
  activeTab: ManagerViewTab;
  /** 그룹 배정 메뉴 후보 라벨("전체" 제외). */
  groupNames: string[];
  /** 노드 선택 핸들러. */
  onSelect: (instanceId: string) => void;
  /** 서브탭 전환 핸들러(`?tab=` 갱신). */
  onTabChange: (tab: ManagerViewTab) => void;
  /** 나가기(선택 해제) 핸들러. */
  onExit: () => void;
  /** 그룹 배정 핸들러(선택 노드 대상). */
  onAssignGroup: (groupName: string) => void;
  /** 그룹 해제 핸들러("전체" 환원). */
  onClearGroup: () => void;
  /** 그룹 변경 진행 중 여부. */
  groupPending: boolean;
}

/** 관리자 뷰 상단 수평 바. */
export function ManagerViewTopBar({
  groups,
  nodesByGroup,
  selectedNode,
  activeTab,
  groupNames,
  onSelect,
  onTabChange,
  onExit,
  onAssignGroup,
  onClearGroup,
  groupPending,
}: ManagerViewTopBarProps): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <header
      data-testid="manager-view-top-bar"
      className="flex shrink-0 flex-col gap-2 border-b border-(--color-border-default) bg-(--color-bg-surface) px-4 py-2"
    >
      {/* 1행: 나가기 + 노드 피커 + 관리 액션 */}
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onExit}
          disabled={!selectedNode}
          aria-label={t('remote.managerView.exit')}
          title={t('remote.managerView.exit')}
          data-testid="manager-view-exit"
          className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:cursor-not-allowed disabled:opacity-40"
        >
          <LogOut className="h-4 w-4" aria-hidden="true" />
          <span className="hidden sm:inline">{t('remote.managerView.exit')}</span>
        </button>

        <NodePicker
          groups={groups}
          nodesByGroup={nodesByGroup}
          selectedNode={selectedNode}
          onSelect={onSelect}
        />

        {/* 관리 액션: 선택 노드 그룹 배정/해제 */}
        {selectedNode && (
          <div className="ml-auto flex items-center gap-1">
            <NodeGroupMenu
              currentGroup={selectedNode.group_name ?? ''}
              groupNames={groupNames}
              pending={groupPending}
              onAssign={onAssignGroup}
              onClear={onClearGroup}
            />
          </div>
        )}
      </div>

      {/* 2행: 서브탭 네비(노드 선택 시에만) */}
      {selectedNode && (
        <nav
          role="tablist"
          aria-label={t('remote.dashboard.tabsLabel')}
          data-testid="manager-view-tabs"
          className="flex items-center gap-1"
        >
          {TABS.map(({ id, labelKey, Icon }) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={activeTab === id}
              data-testid={`manager-view-tab-${id}`}
              onClick={() => onTabChange(id)}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                activeTab === id
                  ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                  : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)',
              )}
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              {t(labelKey)}
            </button>
          ))}
        </nav>
      )}
    </header>
  );
}
