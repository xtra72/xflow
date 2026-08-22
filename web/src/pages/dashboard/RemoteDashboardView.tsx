// 원격 대시보드 뷰 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L10/L11/L12).
//
// 관리자가 서버에서 승인·온라인 원격 노드의 대시보드를 로컬과 동일한 패널로
// 보는 READ-ONLY 뷰이다(A16 — 로컬 DashboardPage 패널 재사용). 로컬 뷰와 달리:
//   - config 는 노드에서 READ-ONLY 취득한다(useDashboardConfigTarget, REQ-L01).
//     useUIStore/useDashboardSync 를 사용하지 않는다(원격 config 편집은 v1.5
//     비목표 — REQ-L12). 편집/sync/레이아웃 변경/테마 편집 컨트롤이 없다.
//   - TargetProvider(target)로 패널 트리에 원격 target 을 전파한다(REQ-L09) →
//     각 패널이 자체적으로 데이터 소스를 원격으로 전환한다(REQ-L04~L08).
//   - 게이팅(승인∧온라인, useTargetGating)·원격 컨텍스트 표시·503/504/502/404
//     실패 의미(editError)를 제공한다(REQ-L11).
//
// 패널 렌더는 공유 renderDashboardPanel 을 재사용한다(패널 중복 금지). config/
// title 변경 콜백은 no-op(읽기 전용).

import { useState } from 'react';
import GridLayout from 'react-grid-layout';
import { LayoutDashboard, Maximize2, Monitor, Network, RefreshCw } from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useDashboardConfigTarget } from '@/hooks/useDashboardConfigTarget';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import {
  extractMessage,
  extractStatus,
  remoteEditErrorMessage,
} from '@/lib/remote/editError';
import type { ResourceTarget } from '@/lib/remote/target';
import { TargetProvider } from '@/lib/remote/TargetProvider';
import type { RemoteDashboardScope } from '@/services/api/remoteService';
import {
  useUIStore,
  type DashboardLayoutItem,
  type PanelConfig,
  type RemoteDashboardRenderMode,
} from '@/stores/uiStore';
import type { DashboardPayload } from '@/types/dashboard';
import type { FlowInfo } from '@/types/flow';

import { renderDashboardPanel, type PanelChangeHandlers } from './renderDashboardPanel';

/** 그리드 설정(로컬 뷰와 동일). */
const GRID_MARGIN: [number, number] = [16, 16];

/** 읽기 전용 패널 변경 콜백(no-op). */
const NOOP_HANDLERS = (): PanelChangeHandlers => ({
  onConfigChange: () => {},
  onTitleChange: () => {},
});

/**
 * 에러가 "대시보드 미설정"을 의미하는지 판별한다(방어적).
 *
 * 주 경로는 백엔드가 200 + data:null 로 응답해 payload 가 undefined 인 정상 빈
 * 상태이다. 다만 일부 경로에서 미설정이 여전히 에러(404, 또는 not-found 메시지)
 * 로 표면화될 수 있으므로, 이를 하드 에러가 아닌 빈 상태로 취급한다(REQ-L11).
 */
function isDashboardNotFound(err: unknown): boolean {
  if (extractStatus(err) === 404) return true;
  const msg = extractMessage(err)?.toLowerCase();
  if (!msg) return false;
  return msg.includes('not found') || msg.includes('no dashboard');
}

interface RemoteDashboardViewProps {
  /** 원격 노드 타깃. */
  target: Extract<ResourceTarget, { type: 'remote' }>;
}

/** 원격 노드 대시보드 READ-ONLY 뷰. */
export default function RemoteDashboardView({
  target,
}: RemoteDashboardViewProps): React.JSX.Element {
  const { t } = useTranslation();

  // 게이팅(승인∧온라인). 노드 ready 가 아니면 config 쿼리를 막아 404/503 노이즈 방지.
  const gating = useTargetGating(target);
  const nodeReady = gating.nodeReady;

  // 스코프(공유/내 대시보드). 로컬 탭과 동일 의미이나 원격은 READ-ONLY.
  const [scope, setScope] = useState<RemoteDashboardScope>('shared');

  // 렌더 모드(반응형/고정, SPEC-REMOTE-001 M11.4). uiStore 에 영속되는 뷰 전역
  // 환경설정이다 — DashboardCanvas(NodeDashboard)가 같은 값을 읽어 고정 캔버스
  // 래핑 여부를 결정한다. 기본값 'responsive'.
  const renderMode = useUIStore((s) => s.remoteDashboardRenderMode);
  const setRenderMode = useUIStore((s) => s.setRemoteDashboardRenderMode);

  const { payload, isLoading, error, refetch } = useDashboardConfigTarget(
    target,
    scope,
    nodeReady,
  );

  return (
    <div className="-m-6 flex flex-1 flex-col" data-testid="remote-dashboard">
      {/* 스코프 토글(READ-ONLY) */}
      <div
        role="tablist"
        aria-label={t('remote.remoteDashboard.scopeLabel')}
        className="flex h-9 shrink-0 items-center gap-1 border-b border-(--color-border-default) bg-(--color-bg-surface) px-6"
      >
        <ScopeTab active={scope === 'shared'} label={t('remote.remoteDashboard.shared')} onClick={() => setScope('shared')} />
        <ScopeTab active={scope === 'mine'} label={t('remote.remoteDashboard.mine')} onClick={() => setScope('mine')} />
        <div className="ml-auto flex items-center gap-3">
          {/* 렌더 모드 토글(반응형/고정) — 원격 대시보드 탭 전용 (M11.4) */}
          <RenderModeToggle mode={renderMode} onChange={setRenderMode} />
          <span className="inline-flex items-center gap-1 text-[11px] text-(--color-text-muted)">
            {t('remote.remoteDashboard.readOnly')}
          </span>
        </div>
      </div>

      {/* 원격 컨텍스트 헤더(노드 이름 + 새로고침) */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
        <div className="flex min-w-0 items-center gap-2">
          <Network className="h-4 w-4 shrink-0 text-blue-500" aria-hidden="true" />
          <span className="truncate text-base font-semibold text-(--color-text-primary)">
            {t('remote.remoteDashboard.title')}
          </span>
          <span className="truncate text-sm text-(--color-text-muted)">
            {gating.nodeLabel ?? target.instanceId}
          </span>
        </div>
        <button
          type="button"
          onClick={() => refetch()}
          disabled={!nodeReady || isLoading}
          className="text-(--color-text-muted) transition-colors hover:text-(--color-text-primary) disabled:opacity-50"
          aria-label={t('common.refresh')}
        >
          <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
        </button>
      </header>

      {/* 콘텐츠 */}
      <div className="flex-1 overflow-auto">
        <RemoteDashboardBody
          target={target}
          nodeReady={nodeReady}
          payload={payload}
          isLoading={isLoading}
          error={error}
          onRetry={() => refetch()}
        />
      </div>
    </div>
  );
}

/** 스코프 토글 탭. */
function ScopeTab({
  active,
  label,
  onClick,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
}): React.JSX.Element {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`inline-flex h-7 items-center rounded-md px-3 text-[12px] font-medium transition-colors ${
        active
          ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
          : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
      }`}
    >
      {label}
    </button>
  );
}

/**
 * 렌더 모드 세그먼티드 토글(반응형 ↔ 고정, SPEC-REMOTE-001 M11.4).
 *
 * "화면 맞춤(반응형)" 은 그리드가 관리자 영역을 채우는 해상도 독립 모드,
 * "노드 해상도(고정)" 는 노드 해상도 고정 캔버스(픽셀 충실 재현) 모드이다.
 * 변경 시 uiStore 에 영속되어 세션/노드 간 유지된다.
 */
function RenderModeToggle({
  mode,
  onChange,
}: {
  mode: RemoteDashboardRenderMode;
  onChange: (mode: RemoteDashboardRenderMode) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <div
      role="radiogroup"
      aria-label={t('remote.remoteDashboard.renderMode.label')}
      data-testid="render-mode-toggle"
      className="inline-flex items-center rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) p-0.5"
    >
      <RenderModeOption
        active={mode === 'responsive'}
        label={t('remote.remoteDashboard.renderMode.responsive')}
        title={t('remote.remoteDashboard.renderMode.responsiveHint')}
        testId="render-mode-responsive"
        onClick={() => onChange('responsive')}
        icon={<Maximize2 className="h-3 w-3" aria-hidden="true" />}
      />
      <RenderModeOption
        active={mode === 'fixed'}
        label={t('remote.remoteDashboard.renderMode.fixed')}
        title={t('remote.remoteDashboard.renderMode.fixedHint')}
        testId="render-mode-fixed"
        onClick={() => onChange('fixed')}
        icon={<Monitor className="h-3 w-3" aria-hidden="true" />}
      />
    </div>
  );
}

/** 렌더 모드 토글의 단일 옵션 버튼. */
function RenderModeOption({
  active,
  label,
  title,
  testId,
  onClick,
  icon,
}: {
  active: boolean;
  label: string;
  title: string;
  testId: string;
  onClick: () => void;
  icon: React.ReactNode;
}): React.JSX.Element {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      title={title}
      data-testid={testId}
      onClick={onClick}
      className={`inline-flex h-6 items-center gap-1 rounded px-2 text-[11px] font-medium transition-colors ${
        active
          ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
          : 'text-(--color-text-muted) hover:text-(--color-text-secondary)'
      }`}
    >
      {icon}
      {label}
    </button>
  );
}

interface RemoteDashboardBodyProps {
  target: Extract<ResourceTarget, { type: 'remote' }>;
  nodeReady: boolean;
  payload: DashboardPayload | undefined;
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}

/** 원격 대시보드 본문(게이팅·로딩·에러·그리드). */
function RemoteDashboardBody({
  target,
  nodeReady,
  payload,
  isLoading,
  error,
  onRetry,
}: RemoteDashboardBodyProps): React.JSX.Element {
  const { t } = useTranslation();

  // 게이팅: 노드 미승인/오프라인 → 제어 불가 안내(REQ-L11).
  if (!nodeReady) {
    return (
      <div
        data-testid="remote-dashboard-gated"
        className="m-6 rounded-md border border-amber-300 bg-amber-50 p-6 text-center text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200"
      >
        {t('remote.remoteDashboard.nodeNotReady')}
      </div>
    );
  }

  if (isLoading && !payload) {
    return (
      <div className="space-y-4 p-6" data-testid="remote-dashboard-loading">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
          <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
        </div>
      </div>
    );
  }

  // 에러: 503/504/502 등 실질 실패만 빨간 에러 UI 로 표시한다(REQ-L11).
  // "미설정"(404/not-found 메시지)은 하드 에러가 아니라 빈 상태로 떨어뜨린다.
  if (error && !isDashboardNotFound(error)) {
    return (
      <div
        data-testid="remote-dashboard-error"
        className="m-6 rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
      >
        <p className="text-sm text-red-700 dark:text-red-400">
          {remoteEditErrorMessage(error, t)}
        </p>
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          {t('common.retry')}
        </button>
      </div>
    );
  }

  // 빈 상태: payload 가 없거나(200 + data:null), 미설정이 에러로 표면화된 경우.
  // 에러 화면이 아니라 친절한 안내(아이콘 + 제목 + 힌트)를 보여준다. 상단의
  // 스코프 탭/새로고침은 항상 렌더되므로 사용자는 스코프를 전환할 수 있다.
  if (!payload) {
    return (
      <div
        data-testid="remote-dashboard-empty"
        className="m-6 flex flex-col items-center justify-center gap-3 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-10 text-center"
      >
        <LayoutDashboard
          className="h-10 w-10 text-(--color-text-muted)"
          aria-hidden="true"
        />
        <p className="text-sm font-medium text-(--color-text-primary)">
          {t('remote.remoteDashboard.empty')}
        </p>
        <p className="max-w-sm text-xs text-(--color-text-muted)">
          {t('remote.remoteDashboard.emptyHint')}
        </p>
      </div>
    );
  }

  return <RemoteDashboardGrid target={target} payload={payload} />;
}

interface RemoteDashboardGridProps {
  target: Extract<ResourceTarget, { type: 'remote' }>;
  payload: DashboardPayload;
}

/**
 * 원격 config 의 활성 페이지를 READ-ONLY 그리드로 렌더한다. TargetProvider 로
 * 원격 target 을 패널 트리에 전파한다(REQ-L09) → 패널이 자체 원격 소스 사용.
 */
function RemoteDashboardGrid({
  target,
  payload,
}: RemoteDashboardGridProps): React.JSX.Element {
  // 활성 페이지(없으면 첫 페이지). 원격은 편집 불가이므로 정적 선택.
  const pages = payload.dashboardPages ?? [];
  const activePage =
    pages.find((p) => p.id === payload.activeDashboardId) ?? pages[0];
  const layout: DashboardLayoutItem[] = activePage?.layout ?? [];
  const panels: PanelConfig[] = activePage?.panels ?? [];
  const gridCols = payload.dashboardGridCols || 10;
  const refreshMs = (payload.dashboardRefreshInterval || 5) * 1000;

  // 빈 메트릭/flows: 원격 패널은 자체 소스로 데이터를 취득하므로 빈 로컬값을 넘긴다.
  const emptyFlows: FlowInfo[] = [];

  return (
    <TargetProvider target={target}>
      <RemoteGridInner
        layout={layout}
        panels={panels}
        gridCols={gridCols}
        refreshMs={refreshMs}
        emptyFlows={emptyFlows}
      />
    </TargetProvider>
  );
}

interface RemoteGridInnerProps {
  layout: DashboardLayoutItem[];
  panels: PanelConfig[];
  gridCols: number;
  refreshMs: number;
  emptyFlows: FlowInfo[];
}

function RemoteGridInner({
  layout,
  panels,
  gridCols,
  refreshMs,
  emptyFlows,
}: RemoteGridInnerProps): React.JSX.Element {
  const [containerWidth, setContainerWidth] = useState(0);

  const gridRefCallback = (node: HTMLDivElement | null): void => {
    if (node) setContainerWidth(node.clientWidth);
  };

  // 측정 전(또는 jsdom 처럼 clientWidth=0 환경)에는 합리적 기본 폭으로 폴백해
  // 그리드를 렌더한다(빈 화면 방지). 실제 측정값이 오면 즉시 교체된다.
  const effectiveWidth = containerWidth > 0 ? containerWidth : 1200;

  const gridRowHeight = Math.round(
    (effectiveWidth - GRID_MARGIN[0] * (gridCols - 1)) / gridCols,
  );

  return (
    <div className="relative p-6" ref={gridRefCallback}>
      {(
        <GridLayout
          layout={layout}
          width={effectiveWidth}
          gridConfig={{
            cols: gridCols,
            rowHeight: gridRowHeight,
            margin: GRID_MARGIN,
            containerPadding: [0, 0],
          }}
          // READ-ONLY: 드래그/리사이즈 비활성(원격 config 편집 비목표 — REQ-L12).
          dragConfig={{ enabled: false, handle: '.dashboard-drag-handle' }}
          resizeConfig={{ enabled: false, handles: ['se'] }}
          onLayoutChange={() => {}}
        >
          {panels.map((panel) => {
            const panelColor = panel.config?.panelColor as string | undefined;
            return (
              <div
                key={panel.id}
                className="relative flex flex-col overflow-hidden"
                style={
                  panelColor
                    ? ({
                        '--panel-accent': panelColor,
                        borderLeft: `4px solid ${panelColor}`,
                        borderTop: `2px solid ${panelColor}`,
                      } as React.CSSProperties)
                    : undefined
                }
              >
                {renderDashboardPanel(panel, emptyFlows, undefined, refreshMs, NOOP_HANDLERS)}
              </div>
            );
          })}
        </GridLayout>
      )}
    </div>
  );
}
