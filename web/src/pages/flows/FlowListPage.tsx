// 플로우 목록 페이지.
// 등록된 플로우의 CRUD 및 lifecycle 관리 기능을 제공한다.
// 검색, 상태 필터, 페이지네이션을 지원한다.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleStop,
  Download,
  FileText,
  Plus,
  Rocket,
  Upload,
  Workflow,
} from 'lucide-react';

import ImportDialog from '@/components/common/ImportDialog';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { RemoteTargetBanner } from '@/components/remote/RemoteTargetBanner';
import { useFlowsTarget } from '@/hooks/useResourceTargets';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTargetParam } from '@/hooks/useTargetParam';
import PermissionButton from '@/components/common/PermissionButton';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { TargetProvider } from '@/lib/remote/TargetProvider';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import { downloadJSON } from '@/lib/utils/download';
import { exportAllFlows, updateFlow } from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';
import CreateFlowModal from '@/pages/dashboard/CreateFlowModal';

import FlowActionMenu from './FlowActionMenu';
import FlowDetailPanel from './FlowDetailPanel';
import FlowSearchFilter from './FlowSearchFilter';

/**
 * 상태별 색상 및 아이콘 매핑 (대시보드 FlowPanel과 동일).
 * `labelKey`는 i18n 키(`status.*`)이며 렌더 시 t()로 변환한다(컴포넌트 밖 t() 호출 금지).
 */
const STATUS_CONFIG: Record<string, { labelKey: string; color: string; icon: React.ReactNode }> = {
  running: {
    labelKey: 'status.running',
    color: 'text-green-600 dark:text-green-400',
    icon: <Activity className="h-4 w-4" />,
  },
  stopped: {
    labelKey: 'status.stopped',
    color: 'text-gray-600 dark:text-gray-400',
    icon: <CircleStop className="h-4 w-4" />,
  },
  error: {
    labelKey: 'status.error',
    color: 'text-red-600 dark:text-red-400',
    icon: <AlertTriangle className="h-4 w-4" />,
  },
  stored: {
    labelKey: 'status.stored',
    color: 'text-blue-600 dark:text-blue-400',
    icon: <FileText className="h-4 w-4" />,
  },
  loaded: {
    labelKey: 'status.loaded',
    color: 'text-yellow-600 dark:text-yellow-400',
    icon: <Rocket className="h-4 w-4" />,
  },
};

/** 페이지 크기 옵션 */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

/** 플로우 목록 페이지 props. */
interface FlowListPageProps {
  /**
   * 자원 타깃 오버라이드 (SPEC-REMOTE-001 M9, 그룹 K). 주어지면 URL `?target=`
   * 대신 이 값을 사용한다. 노드 대시보드가 페이지를 서브탭에 임베드하며 원격
   * 타깃을 주입하기 위함이다. 미지정 시(로컬 라우트 `/flows`) 기존처럼 URL 의
   * `?target=` 를 읽으므로 로컬 사용은 회귀 없이 동일하게 동작한다.
   */
  target?: ResourceTarget;
  /**
   * 원격 타깃 배너 숨김 여부 (SPEC-REMOTE-001 M9, 그룹 K). 노드 대시보드가
   * 페이지를 서브탭에 임베드할 때 true 로 주입한다. 디렉토리+대시보드 헤더가
   * 이미 선택 노드를 표시하므로 임베드 컨텍스트에서 배너는 중복이며,
   * "로컬로 돌아가기" 도 무의미하다. 미지정/false 면 기존처럼 배너를 렌더한다
   * (단독 `?target=` 딥링크는 회귀 없음, 로컬은 null).
   */
  hideRemoteBanner?: boolean;
}

/**
 * 플로우 목록 페이지.
 * 테이블 형태로 플로우를 표시하며 검색, 필터, 페이지네이션을 지원한다.
 */
export default function FlowListPage({
  target: targetProp,
  hideRemoteBanner = false,
}: FlowListPageProps = {}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  // SPEC-REMOTE-001 M8 (그룹 J): 타깃(로컬 | 원격 노드)에 따라 데이터 소스를
  // 전환한다. 로컬이면 기존 useFlows 동작과 동일하다(회귀 없음). M9(그룹 K)에서
  // 노드 대시보드가 targetProp 로 원격 타깃을 주입할 수 있다(URL 대신 prop 우선).
  const paramTarget = useTargetParam();
  const target = targetProp ?? paramTarget;
  const remote = isRemoteTarget(target);
  const { data: flowsData, isLoading, error, refetch } = useFlowsTarget(target);
  const gating = useTargetGating(target);
  // 가져오기/전체 내보내기/자동시작 토글은 로컬 전용 어포던스이다(원격 미러는
  // redaction 정의만 보유하며 자동시작 토글은 전체 정의 갱신이 필요). 라이프사이클
  // 액션(시작/중지/배포/삭제)과 생성은 원격에서도 제공한다(REQ-J03/J12, M8 확장).
  const showLocalWrites = !remote;

  // 모달 상태
  const [modalOpen, setModalOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);

  /** 전체 내보내기 핸들러 */
  const handleExportAll = async () => {
    try {
      const data = await exportAllFlows();
      downloadJSON(data, 'flows.json');
    } catch {
      // 내보내기 실패 시 무시
    }
  };

  /** 자동시작 토글 핸들러 */
  const handleAutoStartToggle = async (flow: FlowInfo) => {
    try {
      await updateFlow(flow.id, { auto_start: !flow.auto_start });
      refetch();
    } catch (err) {
      console.error('Failed to toggle auto_start:', err);
    }
  };

  /** 가져오기 성공 핸들러 */
  const handleImportSuccess = () => {
    queryClient.invalidateQueries({ queryKey: ['flows'] });
  };

  // 검색 및 필터 상태
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 확장 상태
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // 페이지네이션 상태
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const allFlows: FlowInfo[] = useMemo(() => flowsData?.data ?? [], [flowsData?.data]);

  // 클라이언트 측 필터링
  const filteredFlows = useMemo(() => {
    let result = allFlows;

    // 이름 검색
    if (search.trim()) {
      const query = search.trim().toLowerCase();
      result = result.filter((f) => f.name.toLowerCase().includes(query));
    }

    // 상태 필터
    if (statusFilter) {
      result = result.filter((f) => f.status === statusFilter);
    }

    return result;
  }, [allFlows, search, statusFilter]);

  // 클라이언트 측 정렬
  const sortedFlows = useMemo(() => {
    const sorted = [...filteredFlows];
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;

    sorted.sort((a, b) => {
      let va: string | number;
      let vb: string | number;

      switch (field) {
        case 'name':
          va = a.name.toLowerCase();
          vb = b.name.toLowerCase();
          break;
        case 'status':
          va = a.status.toLowerCase();
          vb = b.status.toLowerCase();
          break;
        case 'created_at':
          va = a.created_at ?? '';
          vb = b.created_at ?? '';
          break;
        case 'updated_at':
          va = a.updated_at ?? '';
          vb = b.updated_at ?? '';
          break;
        default:
          return 0;
      }

      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });

    return sorted;
  }, [filteredFlows, sort]);

  /** 정렬 필드 변경 핸들러. 같은 필드 클릭 시 방향 토글, 다른 필드 시 asc. */
  const handleSort = (field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
    setPage(1);
  };

  // 페이지네이션 계산
  const totalItems = sortedFlows.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedFlows = sortedFlows.slice(startIndex, startIndex + pageSize);

  // 필터 변경 시 페이지 초기화
  const handleSearchChange = (v: string) => {
    setSearch(v);
    setPage(1);
  };
  const handleStatusFilterChange = (v: string) => {
    setStatusFilter(v);
    setPage(1);
  };
  const handlePageSizeChange = (size: number) => {
    setPageSize(size);
    setPage(1);
  };

  /** 날짜 포맷 (간략) */
  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '-';
    try {
      return new Date(dateStr).toLocaleDateString('ko-KR', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
      });
    } catch {
      return '-';
    }
  };

  // --- 로딩 상태 ---
  if (isLoading && !flowsData) {
    return (
      <div className="space-y-6">
        {/* 헤더 스켈레톤 */}
        <div className="flex items-center justify-between">
          <div className="h-8 w-24 animate-pulse rounded bg-(--color-bg-elevated)" />
          <div className="h-10 w-28 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
        {/* 필터 스켈레톤 */}
        <div className="flex items-center gap-3">
          <div className="h-10 flex-1 animate-pulse rounded bg-(--color-bg-elevated)" />
          <div className="h-10 w-36 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
        {/* 테이블 스켈레톤 */}
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="h-14 animate-pulse rounded bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      </div>
    );
  }

  // --- 에러 상태 ---
  if (error) {
    return (
      <div className="space-y-6">
        <div className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-700 dark:text-red-400">
            {t('flows.loadError')}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            {t('common.retry')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <TargetProvider target={target}>
    <div className="space-y-6">
      {/* 원격 타깃 배너(로컬이면 null). 대시보드 임베드 시 중복이므로 숨김. */}
      {!hideRemoteBanner && (
        <RemoteTargetBanner
          target={target}
          nodeLabel={gating.nodeLabel}
          nodeReady={gating.nodeReady}
          localHref="/flows"
        />
      )}

      {/* 액션 버튼. 가져오기/전체 내보내기는 로컬 전용, 생성은 타깃 인지. */}
      <div className="flex items-center justify-end">
        <div className="flex items-center gap-2">
          {showLocalWrites && (
            <>
              <PermissionButton
                type="button"
                permission="flow.create"
                onClick={() => setImportDialogOpen(true)}
                className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) px-3 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
              >
                <Upload className="h-4 w-4" />
                {t('common.import')}
              </PermissionButton>
              <PermissionButton
                type="button"
                permission="flow.read"
                onClick={handleExportAll}
                className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) px-3 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
              >
                <Download className="h-4 w-4" />
                {t('common.exportAll')}
              </PermissionButton>
            </>
          )}
          {/* 생성: 로컬은 모달, 원격은 시각 편집기 신규 라우트(노드 채번 — REQ-I08). */}
          <PermissionButton
            type="button"
            permission="flow.create"
            disabled={remote && !gating.nodeReady}
            title={remote && !gating.nodeReady ? t('remote.edit.createGateHint') : undefined}
            onClick={() => {
              if (remote && isRemoteTarget(target)) {
                navigate(`/admin/remote/nodes/${target.instanceId}/flows/new`);
              } else {
                setModalOpen(true);
              }
            }}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            {t('flows.newFlow')}
          </PermissionButton>
        </div>
      </div>

      {/* 검색 및 필터 */}
      <FlowSearchFilter
        search={search}
        onSearchChange={handleSearchChange}
        statusFilter={statusFilter}
        onStatusFilterChange={handleStatusFilterChange}
      />

      {/* 테이블 또는 빈 상태 */}
      {filteredFlows.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center">
          <Workflow className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {allFlows.length === 0
              ? t('flows.emptyTitle')
              : t('flows.noSearchResults')}
          </p>
          {allFlows.length === 0 && (showLocalWrites || gating.nodeReady) && (
            <PermissionButton
              type="button"
              permission="flow.create"
              onClick={() => {
                if (remote && isRemoteTarget(target)) {
                  navigate(`/admin/remote/nodes/${target.instanceId}/flows/new`);
                } else {
                  setModalOpen(true);
                }
              }}
              className="mt-4 inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-4 w-4" />
              {t('flows.newFlow')}
            </PermissionButton>
          )}
        </div>
      ) : (
        <>
          {/* 페이지네이션 */}
          <div className="flex items-center justify-between">
            {/* 페이지 크기 선택 */}
            <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
              <span>{t('common.pagination.perPage')}</span>
              <select
                value={pageSize}
                onChange={(e) => handlePageSizeChange(Number(e.target.value))}
                className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                {PAGE_SIZE_OPTIONS.map((size) => (
                  <option key={size} value={size}>
                    {size}
                  </option>
                ))}
              </select>
              <span>{t('common.pagination.unit')}</span>
              <span className="ml-2 text-gray-400">|</span>
              <span className="ml-2">
                {t('common.pagination.range')
                  .replace('{total}', String(totalItems))
                  .replace('{start}', String(startIndex + 1))
                  .replace('{end}', String(Math.min(startIndex + pageSize, totalItems)))}
              </span>
            </div>

            {/* 페이지 이동 버튼 */}
            <div className="flex items-center gap-1">
              <button
                type="button"
                disabled={safePage <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                aria-label={t('common.pagination.prev')}
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <span className="px-3 text-sm text-(--color-text-muted)">
                {safePage} / {totalPages}
              </span>
              <button
                type="button"
                disabled={safePage >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                aria-label={t('common.pagination.next')}
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          </div>

          {/* 플로우 테이블 */}
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-primary)">
                <tr>
                  <th className="w-8 px-3 py-3" />
                  <SortableHeader label={t('flows.colName')} field="name" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label={t('flows.colStatus')} field="status" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    {t('flows.colNode')}
                  </th>
                  <SortableHeader label={t('flows.colCreatedAt')} field="created_at" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label={t('flows.colUpdatedAt')} field="updated_at" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    {t('common.uptime')}
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    {t('flows.colAutoStart')}
                  </th>
                  <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    {t('common.actions')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
                {pagedFlows.map((flow) => {
                  const isExpanded = expandedId === flow.id;
                  return (
                    <FlowRow
                      key={flow.id}
                      flow={flow}
                      isExpanded={isExpanded}
                      onToggle={() => setExpandedId((prev) => (prev === flow.id ? null : flow.id))}
                      onNavigate={() =>
                        navigate(
                          remote && isRemoteTarget(target)
                            ? `/admin/remote/nodes/${target.instanceId}/flows/${flow.id}/edit`
                            : `/editor/${flow.id}`,
                        )
                      }
                      formatDate={formatDate}
                      onAutoStartToggle={handleAutoStartToggle}
                      showLocalWrites={showLocalWrites}
                      t={t}
                    />
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {/* 플로우 생성 모달 (로컬 전용) */}
      {showLocalWrites && (
        <CreateFlowModal open={modalOpen} onClose={() => setModalOpen(false)} />
      )}

      {/* 가져오기 대화 상자 (로컬 전용) */}
      {showLocalWrites && (
        <ImportDialog
          open={importDialogOpen}
          onClose={() => setImportDialogOpen(false)}
          type="flow"
          onImportSuccess={handleImportSuccess}
        />
      )}
    </div>
    </TargetProvider>
  );
}

// ---- 플로우 행 컴포넌트 ----

interface FlowRowProps {
  flow: FlowInfo;
  isExpanded: boolean;
  onToggle: () => void;
  onNavigate: () => void;
  formatDate: (dateStr?: string) => string;
  onAutoStartToggle: (flow: FlowInfo) => void;
  /** 로컬 쓰기 어포던스(자동시작 토글·액션 메뉴) 표시 여부(원격은 숨김). */
  showLocalWrites: boolean;
  /** 번역 함수(상위에서 주입). */
  t: TranslationFn;
}

/** 플로우 테이블 행 (확장 가능) */
function FlowRow({ flow, isExpanded, onToggle, onNavigate, formatDate, onAutoStartToggle, showLocalWrites, t }: FlowRowProps) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer transition-colors hover:bg-(--color-bg-elevated)"
      >
        {/* 확장 아이콘 */}
        <td className="px-3 py-3 text-gray-400">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>

        <td className="whitespace-nowrap px-4 py-3">
          <div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onNavigate();
              }}
              className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
            >
              {flow.name}
            </button>
            {flow.description && (
              <p className="mt-0.5 text-xs text-(--color-text-muted)">
                {flow.description}
              </p>
            )}
          </div>
        </td>
        <td className="whitespace-nowrap px-4 py-3">
          {(() => {
            const cfg = STATUS_CONFIG[flow.status];
            return cfg ? (
              <span className={`inline-flex items-center ${cfg.color}`} title={t(cfg.labelKey)}>
                {cfg.icon}
              </span>
            ) : (
              <span className="text-sm text-(--color-text-muted)">{flow.status}</span>
            );
          })()}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {flow.node_count}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatDate(flow.created_at)}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatDate(flow.updated_at)}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {flow.status === 'running' && flow.uptime ? flow.uptime : '-'}
        </td>
        <td className="whitespace-nowrap px-4 py-3">
          {showLocalWrites ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onAutoStartToggle(flow);
              }}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                flow.auto_start ? 'bg-blue-600' : 'bg-gray-300 dark:bg-gray-600'
              }`}
            >
              <span
                className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                  flow.auto_start ? 'translate-x-4.5' : 'translate-x-0.5'
                }`}
              />
            </button>
          ) : (
            <span className="text-xs text-(--color-text-muted)">
              {flow.auto_start ? 'ON' : 'OFF'}
            </span>
          )}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-right">
          {/* 액션 메뉴는 타깃 인지(원격은 그룹 D/M7 경로). 자동시작 토글만 로컬 전용. */}
          <FlowActionMenu flow={flow} />
        </td>
      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={9} className="bg-(--color-bg-sunken)">
            <FlowDetailPanel flowId={flow.id} />
          </td>
        </tr>
      )}
    </>
  );
}
