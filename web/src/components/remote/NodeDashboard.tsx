// 노드 대시보드 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K13/K14).
//
// 디렉토리 뷰에서 노드를 선택하면 표시되는 대시보드이다. 구성:
//   (a) 개요(overview) — BASIC 시스템 정보(hostname/OS/arch/version/uptime/
//       online·last-seen/status) + 운영 요약(플로우/에이전트/디바이스 카운트+상태).
//   (b) Flow/Agent/Device 서브탭 — 기존 M8 통합 제어 페이지(FlowListPage/
//       AgentListPage/DeviceListPage)를 `target=remote:{instanceId}` 로 재사용한다.
//       별도 원격 전용 화면을 신설하지 않으며(REQ-K14), 변경은 여전히 그룹 D/I
//       경로로만 수행된다(통합 페이지 내부가 라우팅 — REQ-J03/J12 일관).
//
// 서브탭 통합 방식: 통합 페이지는 선택적 `target` prop 을 받으며, 주어지면 URL
// `?target=` 대신 이 값을 사용한다. 본 대시보드는 remote 타깃을 prop 으로 주입해
// 서브탭에 임베드한다. 로컬 라우트(`/flows` 등)는 prop 없이 렌더되므로 URL 의
// `?target=` 를 읽어 기존과 동일하게 동작한다(회귀 없음).

import { lazy, Suspense, useMemo } from 'react';
import { useSearchParams } from 'react-router';
import { Bot, Cpu, Gauge, HardDrive, LayoutDashboard, Workflow } from 'lucide-react';

import { FixedCanvasScaler } from '@/components/remote/FixedCanvasScaler';
import { NodeDisplayResolutionSection } from '@/components/remote/NodeDisplayResolutionSection';
import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import { useRemoteNodeDetail } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import type { ResourceTarget } from '@/lib/remote/target';
import { cn } from '@/lib/utils/cn';
import { formatDate, formatDuration } from '@/lib/utils/format';

// 노드 해상도 미보고(0/absent) 시 고정 캔버스 폴백 크기(REQ-M03, OQ-M1).
const FALLBACK_DISPLAY_WIDTH = 1920;
const FALLBACK_DISPLAY_HEIGHT = 1080;

// 통합 제어 페이지는 지연 로딩한다(서브탭 활성 시에만 로드 → 초기 비용 절감).
const FlowListPage = lazy(() => import('@/pages/flows/FlowListPage'));
const AgentListPage = lazy(() => import('@/pages/agents/AgentListPage'));
const DeviceListPage = lazy(() => import('@/pages/devices/DeviceListPage'));
// 대시보드 서브탭(SPEC-REMOTE-001 M10, 그룹 L, REQ-L10): 로컬 DashboardPage 를
// target=remote:{id} 로 재사용한다(별도 원격 대시보드 화면 미신설 — A16).
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'));

/** 대시보드 서브탭 식별자. */
type DashboardTab = 'overview' | 'flows' | 'agents' | 'devices' | 'dashboard';

/** 유효한 서브탭 식별자 집합(URL 파라미터 검증용). */
const DASHBOARD_TABS: readonly DashboardTab[] = [
  'overview',
  'flows',
  'agents',
  'devices',
  'dashboard',
];

/** URL `?tab=` 원시 값을 DashboardTab 으로 파싱한다(미지정/무효 → overview). */
function parseDashboardTab(raw: string | null): DashboardTab {
  return DASHBOARD_TABS.includes(raw as DashboardTab)
    ? (raw as DashboardTab)
    : 'overview';
}

interface NodeDashboardProps {
  /** 대시보드 대상 노드 식별자. */
  instanceId: string;
  /** 쿼리 활성 여부(server 모드에서만 true). */
  enabled: boolean;
  /**
   * 내부 서브탭 네비를 숨길지 여부(SPEC-REMOTE-001 M11, 그룹 M, REQ-M06).
   *
   * 관리자 뷰(상단 바)에서는 서브탭 네비가 상단 바(ManagerViewTopBar)로
   * 호이스팅되므로 NodeDashboard 자체 네비를 숨긴다. 미지정(기본 false)이면
   * 기존 standalone 동작대로 자체 네비를 렌더한다(회귀 없음). 어느 경우든
   * 활성 탭의 단일 출처는 URL(`?tab=`)이다.
   */
  hideTabNav?: boolean;
}

/**
 * 노드 대시보드 — 개요(시스템 정보 + 운영 요약) + Flow/Agent/Device/대시보드 서브탭.
 */
export function NodeDashboard({
  instanceId,
  enabled,
  hideTabNav = false,
}: NodeDashboardProps): React.JSX.Element {
  const { t } = useTranslation();

  // 활성 서브탭을 URL `?tab=` 에 동기화한다(SPEC-REMOTE-001: 에디터 "노드로
  // 돌아가기"가 직전 서브탭을 정확히 복원하도록 — 딥링크/뒤로가기 지원). 미지정/
  // 무효 값은 overview 로 폴백한다(기존 기본 동작 불변). `?node=` 등 다른
  // 파라미터는 보존한다.
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = parseDashboardTab(searchParams.get('tab'));

  const setTab = (next: DashboardTab): void => {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        // overview(기본)는 URL 을 깔끔히 유지하기 위해 파라미터를 제거한다.
        if (next === 'overview') params.delete('tab');
        else params.set('tab', next);
        return params;
      },
      { replace: true },
    );
  };

  // 원격 타깃(서브탭이 통합 페이지에 주입). instanceId 가 바뀔 때만 새 객체 생성.
  const target = useMemo<ResourceTarget>(
    () => ({ type: 'remote', instanceId }),
    [instanceId],
  );

  const tabs: { id: DashboardTab; labelKey: string; Icon: typeof Workflow }[] = [
    { id: 'overview', labelKey: 'remote.dashboard.tab.overview', Icon: LayoutDashboard },
    { id: 'dashboard', labelKey: 'remote.dashboard.tab.dashboard', Icon: Gauge },
    { id: 'flows', labelKey: 'remote.dashboard.tab.flows', Icon: Workflow },
    { id: 'agents', labelKey: 'remote.dashboard.tab.agents', Icon: Bot },
    { id: 'devices', labelKey: 'remote.dashboard.tab.devices', Icon: HardDrive },
  ];

  // 관리자 뷰(상단 바)에서는 콘텐츠가 풀폭/풀하이트 영역을 채워야 하므로
  // space-y 컨테이너 대신 flex 컬럼으로 렌더한다. standalone(자체 네비)에서는
  // 기존 space-y-4 배치를 유지한다(회귀 없음).
  return (
    <div
      className={hideTabNav ? 'flex h-full min-h-0 flex-col' : 'space-y-4'}
      data-testid="node-dashboard"
      data-instance-id={instanceId}
    >
      {/* 서브탭 헤더(standalone 전용 — 관리자 뷰에서는 상단 바로 호이스팅됨, REQ-M06) */}
      {!hideTabNav && (
        <div
          role="tablist"
          aria-label={t('remote.dashboard.tabsLabel')}
          className="flex items-center gap-1 border-b border-(--color-border-default)"
        >
          {tabs.map(({ id, labelKey, Icon }) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              data-testid={`node-dashboard-tab-${id}`}
              onClick={() => setTab(id)}
              className={cn(
                'inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                tab === id
                  ? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-400'
                  : 'border-transparent text-(--color-text-muted) hover:text-(--color-text-secondary)',
              )}
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              {t(labelKey)}
            </button>
          ))}
        </div>
      )}

      {/* 탭 본문 */}
      {tab === 'overview' ? (
        <NodeOverview instanceId={instanceId} enabled={enabled} />
      ) : (
        <div className={hideTabNav ? 'min-h-0 flex-1' : undefined}>
          <Suspense
            fallback={
              <div
                data-testid="node-dashboard-subtab-loading"
                className="h-40 animate-pulse rounded bg-(--color-bg-elevated)"
              />
            }
          >
            {/* hideRemoteBanner: 디렉토리+대시보드 헤더가 이미 선택 노드를 표시하므로
                임베드 컨텍스트에서 원격 배너는 중복이다(REQ-K14). 플로우/에이전트/
                디바이스 서브탭은 풀폭 반응형을 유지한다(고정 캔버스 미적용 — OQ-M2). */}
            {tab === 'flows' && <FlowListPage target={target} hideRemoteBanner />}
            {tab === 'agents' && <AgentListPage target={target} hideRemoteBanner />}
            {tab === 'devices' && <DeviceListPage target={target} hideRemoteBanner />}
            {/* 대시보드 서브탭: 로컬 DashboardPage 를 원격 target 으로 재사용한다(REQ-L10).
                노드 해상도 고정 캔버스(FixedCanvasScaler)로 감싸 충실 재현한다 —
                대시보드 탭에만 적용(REQ-M07/M08, OQ-M2). 패널 코드/데이터/게이팅은
                불변이며 스케일은 바깥 래퍼가 담당한다(A21/A16, REQ-M09). */}
            {tab === 'dashboard' && (
              <DashboardCanvas instanceId={instanceId} enabled={enabled} target={target} />
            )}
          </Suspense>
        </div>
      )}
    </div>
  );
}

// ---- 대시보드 고정 캔버스 래퍼 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M07~M09) ----

interface DashboardCanvasProps {
  instanceId: string;
  enabled: boolean;
  target: ResourceTarget;
}

/**
 * 대시보드 서브탭을 노드 해상도 고정 캔버스에 렌더한다.
 *
 * 노드 해상도(`display_width`/`display_height`)는 useRemoteNodeDetail 로 읽는다
 * (React Query 캐시로 NodeOverview 와 중복 요청 없이 공유). 미보고/0/잘못된 값은
 * 폴백 1920×1080 을 사용한다(REQ-M03/OQ-M1). 캔버스 내부 패널/데이터/게이팅은
 * 그룹 L(RemoteDashboardView via DashboardPage) 그대로다(REQ-M09).
 */
function DashboardCanvas({
  instanceId,
  enabled,
  target,
}: DashboardCanvasProps): React.JSX.Element {
  const { data: detail } = useRemoteNodeDetail(instanceId, enabled);

  // 미보고(0/absent/음수) → 폴백. 양수 보고값만 노드 해상도로 사용한다.
  const width =
    detail && detail.display_width > 0 ? detail.display_width : FALLBACK_DISPLAY_WIDTH;
  const height =
    detail && detail.display_height > 0 ? detail.display_height : FALLBACK_DISPLAY_HEIGHT;

  return (
    <FixedCanvasScaler width={width} height={height}>
      <DashboardPage target={target} />
    </FixedCanvasScaler>
  );
}

// ---- 개요(시스템 정보 + 운영 요약) ----

interface NodeOverviewProps {
  instanceId: string;
  enabled: boolean;
}

/** 개요 탭 — BASIC 시스템 정보 + 운영 요약(미러 파생). */
function NodeOverview({ instanceId, enabled }: NodeOverviewProps): React.JSX.Element {
  const { t } = useTranslation();
  const { data: detail, isLoading, error, refetch } = useRemoteNodeDetail(
    instanceId,
    enabled,
  );

  if (isLoading && !detail) {
    return (
      <div className="space-y-3" data-testid="node-overview-loading">
        <div className="h-32 animate-pulse rounded bg-(--color-bg-elevated)" />
        <div className="h-24 animate-pulse rounded bg-(--color-bg-elevated)" />
      </div>
    );
  }

  if (error || !detail) {
    return (
      <div
        data-testid="node-overview-error"
        className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
      >
        <p className="text-sm text-red-700 dark:text-red-400">{t('remote.loadError')}</p>
        <button
          type="button"
          onClick={() => refetch()}
          className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          {t('common.retry')}
        </button>
      </div>
    );
  }

  // uptime: started_at 미보고(null) → "미보고", 아니면 사람이 읽는 기간으로 변환.
  const uptimeLabel =
    detail.uptime === null
      ? t('remote.dashboard.uptimeUnreported')
      : formatDuration(Math.floor(detail.uptime / 1000));
  const lastSeenLabel =
    detail.last_seen > 0 ? formatDate(new Date(detail.last_seen), 'long') : '-';
  const groupLabel = detail.group_name || t('remote.group.all');

  return (
    <div className="space-y-4" data-testid="node-overview">
      {/* 시스템 정보(BASIC) */}
      <section
        aria-labelledby="node-systeminfo-heading"
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      >
        <div className="mb-3 flex items-center gap-2">
          <Cpu className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <h2
            id="node-systeminfo-heading"
            className="text-sm font-semibold text-(--color-text-primary)"
          >
            {t('remote.dashboard.systemInfo')}
          </h2>
        </div>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3">
          <InfoItem label={t('remote.dashboard.hostname')} value={detail.hostname || '-'} />
          <InfoItem label={t('remote.dashboard.os')} value={detail.os || '-'} />
          <InfoItem label={t('remote.dashboard.arch')} value={detail.arch || '-'} />
          <InfoItem label={t('remote.dashboard.version')} value={detail.version || '-'} />
          <InfoItem label={t('remote.dashboard.uptime')} value={uptimeLabel} testId="node-uptime" />
          <InfoItem label={t('remote.group.label')} value={groupLabel} />
          <div>
            <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.online')}
            </dt>
            <dd className="mt-1">
              <NodeOnlineIndicator online={detail.online} />
            </dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
              {t('remote.col.status')}
            </dt>
            <dd className="mt-1">
              <NodeStatusBadge status={detail.status} />
            </dd>
          </div>
          <InfoItem label={t('remote.col.lastSeen')} value={lastSeenLabel} />
        </dl>
      </section>

      {/* 디스플레이 해상도(EFFECTIVE + 출처 + admin 오버라이드 SET/CLEAR, M12) */}
      <NodeDisplayResolutionSection instanceId={instanceId} detail={detail} />

      {/* 운영 요약(미러 파생) */}
      <section
        aria-labelledby="node-summary-heading"
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      >
        <h2
          id="node-summary-heading"
          className="mb-3 text-sm font-semibold text-(--color-text-primary)"
        >
          {t('remote.dashboard.summary')}
        </h2>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <SummaryCard
            testId="summary-flows"
            Icon={Workflow}
            title={t('nav.flows')}
            total={detail.summary.flows.total}
            breakdown={[
              { label: t('remote.dashboard.running'), value: detail.summary.flows.running },
              { label: t('remote.dashboard.stopped'), value: detail.summary.flows.stopped },
            ]}
          />
          <SummaryCard
            testId="summary-agents"
            Icon={Bot}
            title={t('nav.agents')}
            total={detail.summary.agents.total}
            breakdown={[
              { label: t('remote.dashboard.connected'), value: detail.summary.agents.connected },
            ]}
          />
          <SummaryCard
            testId="summary-devices"
            Icon={HardDrive}
            title={t('nav.devices')}
            total={detail.summary.devices.total}
            breakdown={[
              { label: t('remote.online'), value: detail.summary.devices.online },
            ]}
          />
        </div>
      </section>
    </div>
  );
}

/** 시스템 정보 단일 항목(label/value). */
function InfoItem({
  label,
  value,
  testId,
}: {
  label: string;
  value: string;
  testId?: string;
}): React.JSX.Element {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
        {label}
      </dt>
      <dd
        data-testid={testId}
        className="mt-1 break-all text-sm text-(--color-text-primary)"
      >
        {value}
      </dd>
    </div>
  );
}

/** 운영 요약 카드(카운트 + 상태 분해). */
function SummaryCard({
  Icon,
  title,
  total,
  breakdown,
  testId,
}: {
  Icon: typeof Workflow;
  title: string;
  total: number;
  breakdown: { label: string; value: number }[];
  testId: string;
}): React.JSX.Element {
  return (
    <div
      data-testid={testId}
      className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-3"
    >
      <div className="flex items-center justify-between">
        <span className="inline-flex items-center gap-1.5 text-sm font-medium text-(--color-text-secondary)">
          <Icon className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          {title}
        </span>
        <span className="text-lg font-semibold text-(--color-text-primary)">{total}</span>
      </div>
      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-text-muted)">
        {breakdown.map((b) => (
          <span key={b.label}>
            {b.label}: <span className="font-medium text-(--color-text-secondary)">{b.value}</span>
          </span>
        ))}
      </div>
    </div>
  );
}
