// 디바이스 관리 페이지.
// 에이전트 페이지와 동일한 테이블 리스트 형식으로 디바이스를 표시한다.
// 행 클릭으로 상세 패널을 토글하고, 검색/필터/페이지네이션을 지원한다.

import { useMemo, useRef, useState, useEffect } from 'react';
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Columns3,
  HardDrive,
  Plus,
  Trash2,
} from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import { RemoteTargetBanner } from '@/components/remote/RemoteTargetBanner';
import {
  ALL_DEVICE_COLUMNS,
  DEVICE_COLUMN_LABELS,
  useDeviceColumns,
  type DeviceListColumnKey,
} from '@/hooks/useDeviceColumns';
import { useAgents } from '@/hooks/useAgent';
import { useDeleteDevice, useSetDeviceReport } from '@/hooks/useDevice';
import { useDevicesTarget } from '@/hooks/useResourceTargets';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTargetParam } from '@/hooks/useTargetParam';
import PermissionButton from '@/components/common/PermissionButton';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { TargetProvider } from '@/lib/remote/TargetProvider';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import { getDeviceDisplayName } from '@/lib/utils/deviceLabels';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { DeviceInfo, DeviceListParams } from '@/types/device';

import DeviceDetailPanel from './DeviceDetailPanel';
import { DeviceCell } from './DeviceCell';
import { ReportToggleSwitch } from './ReportToggleSwitch';
import DeviceSearchFilter from './DeviceSearchFilter';
import AddDeviceDialog from './AddDeviceDialog';

/** 페이지 크기 옵션 */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

/**
 * `remove_device` exec 를 지원하는 에이전트 타입 집합.
 * Samsung HVACR / LGAP / LG ICP-01 / LG ICP-02 가 디바이스 삭제를 지원한다
 * (Century/system/modbus 미지원).
 */
const REMOVABLE_AGENT_TYPES = new Set(['samsung_hvacr01', 'lgap', 'lg_hvacr01', 'lg_hvacr02']);

/**
 * `set_device` exec 를 지원하는 에이전트 타입 집합 (디바이스별 상태 전송 on/off).
 * Samsung HVACR / LGAP / LG ICP-01 / LG ICP-02 가 디바이스별 report_enabled 게이트를 지원한다.
 */
const REPORT_TOGGLE_AGENT_TYPES = new Set(['samsung_hvacr01', 'lgap', 'lg_hvacr01', 'lg_hvacr02']);

/** 디바이스 목록 페이지 props. */
interface DeviceListPageProps {
  /**
   * 자원 타깃 오버라이드 (SPEC-REMOTE-001 M9, 그룹 K). 주어지면 URL `?target=`
   * 대신 이 값을 사용한다(노드 대시보드 서브탭 임베드용). 미지정 시 기존처럼
   * URL `?target=` 를 읽으므로 로컬 사용은 회귀 없이 동일하게 동작한다.
   */
  target?: ResourceTarget;
  /**
   * 원격 타깃 배너 숨김 여부 (SPEC-REMOTE-001 M9, 그룹 K). 노드 대시보드가
   * 서브탭에 임베드할 때 true 로 주입한다(디렉토리+대시보드 헤더가 이미 선택
   * 노드를 표시 → 배너 중복, "로컬로 돌아가기" 무의미). 미지정/false 면 기존처럼
   * 배너를 렌더한다(단독 `?target=` 딥링크 회귀 없음, 로컬은 null).
   */
  hideRemoteBanner?: boolean;
}

export default function DeviceListPage({
  target: targetProp,
  hideRemoteBanner = false,
}: DeviceListPageProps = {}) {
  const { t } = useTranslation();
  // 필터 상태
  const [filters, setFilters] = useState<DeviceListParams>({});
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('');

  // SPEC-REMOTE-001 M8 (그룹 J): 타깃에 따라 데이터 소스를 전환한다(로컬은 기존
  // useDevicesRealtime(filters) 동작과 동일 — 회귀 없음). M9(그룹 K)에서 노드
  // 대시보드가 targetProp 로 원격 타깃을 주입할 수 있다(URL 대신 prop 우선).
  const paramTarget = useTargetParam();
  const target = targetProp ?? paramTarget;
  const remote = isRemoteTarget(target);
  const { data, isLoading, error, refetch } = useDevicesTarget(target, filters);
  const gating = useTargetGating(target);
  const showLocalWrites = !remote;

  // 삭제 가능 여부 판정을 위한 에이전트 타입 맵. device.agent_name → { id, type }.
  // remove_device 는 에이전트 exec 이므로 소유 에이전트 ID 와 지원 타입 확인이 필요하다.
  // 원격 타깃에서는 로컬 쓰기(exec)를 노출하지 않으므로 조회를 건너뛴다.
  const { data: agentsData } = useAgents(undefined, undefined);
  const agentMap = useMemo(() => {
    const map = new Map<string, { id: string; type: string }>();
    for (const a of agentsData?.data ?? []) {
      map.set(a.name, { id: a.id, type: a.type });
    }
    return map;
  }, [agentsData]);

  const deleteDevice = useDeleteDevice();
  const setDeviceReport = useSetDeviceReport();
  const addNotification = useUIStore((s) => s.addNotification);
  // 삭제 확인 다이얼로그 대상(1개). null 이면 닫힌 상태.
  const [deleteTarget, setDeleteTarget] = useState<DeviceInfo | null>(null);

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 확장 상태
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // 페이지네이션 상태
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // 디바이스 추가 다이얼로그
  const [showAddDialog, setShowAddDialog] = useState(false);

  // 컬럼 구성(서버 영속, 전역 1벌). 토글 변경 시 PUT 으로 저장된다.
  const { columns: visibleColumns, setColumns } = useDeviceColumns();
  const [showColumnsMenu, setShowColumnsMenu] = useState(false);

  const devices: DeviceInfo[] = useMemo(() => data?.data ?? [], [data?.data]);

  // 클라이언트 측 필터링
  const filteredDevices = useMemo(() => {
    let result = devices;

    // 이름/ID/UID/타입/에이전트 검색.
    // SPEC-DEVICE-IDENTITY-001 Phase D (M11 / D-T20): backend `id` 가 PR4
    // 이후 UUID 시맨틱이므로 `uid` 도 함께 검색하여 양쪽 호환.
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(
        (d) =>
          d.name.toLowerCase().includes(q) ||
          d.id.toLowerCase().includes(q) ||
          (d.uid?.toLowerCase().includes(q) ?? false) ||
          d.type.toLowerCase().includes(q) ||
          d.agent_name.toLowerCase().includes(q),
      );
    }

    // 온라인/오프라인 필터
    if (statusFilter === 'online') {
      result = result.filter((d) => d.online);
    } else if (statusFilter === 'offline') {
      result = result.filter((d) => !d.online);
    }

    return result;
  }, [devices, searchQuery, statusFilter]);

  // 클라이언트 측 정렬
  const sortedDevices = useMemo(() => {
    const sorted = [...filteredDevices];
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;

    sorted.sort((a, b) => {
      let va: string;
      let vb: string;

      switch (field) {
        case 'name':
          va = (a.name || a.id).toLowerCase();
          vb = (b.name || b.id).toLowerCase();
          break;
        case 'id':
          // uid(1급 UUID 식별자) 우선, 없으면 id 로 정렬.
          va = (a.uid || a.id).toLowerCase();
          vb = (b.uid || b.id).toLowerCase();
          break;
        case 'type':
          va = a.type.toLowerCase();
          vb = b.type.toLowerCase();
          break;
        case 'protocol':
          va = a.protocol.toLowerCase();
          vb = b.protocol.toLowerCase();
          break;
        case 'status':
          va = a.online ? '0' : '1';
          vb = b.online ? '0' : '1';
          break;
        case 'agent':
          va = a.agent_name.toLowerCase();
          vb = b.agent_name.toLowerCase();
          break;
        case 'last_seen':
          va = a.last_seen;
          vb = b.last_seen;
          break;
        default:
          return 0;
      }

      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });

    return sorted;
  }, [filteredDevices, sort]);

  /** 정렬 필드 변경 핸들러 */
  const handleSort = (field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
    setPage(1);
  };

  // 페이지네이션 계산
  const totalItems = sortedDevices.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedDevices = sortedDevices.slice(startIndex, startIndex + pageSize);

  // 필터 변경 시 페이지 초기화
  const handleSearchChange = (v: string) => {
    setSearchQuery(v);
    setPage(1);
  };
  const handleStatusFilterChange = (v: string) => {
    setStatusFilter(v);
    setPage(1);
  };
  const handleProtocolFilterChange = (v: string) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (!v) delete next.protocol;
      else next.protocol = v;
      return next;
    });
    setPage(1);
  };
  const handleTypeFilterChange = (v: string) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (!v) delete next.type;
      else next.type = v;
      return next;
    });
    setPage(1);
  };
  const handlePageSizeChange = (size: number) => {
    setPageSize(size);
    setPage(1);
  };

  /** 행 클릭 시 상세 패널 토글 (읽기 모드) */
  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  /**
   * 디바이스가 삭제 가능하면 소유 에이전트 ID 를 반환하고, 아니면 null.
   * 조건: 로컬 타깃 + 소유 에이전트 타입이 remove_device 지원.
   * config 소스도 삭제 가능하다(수동 추가 후 재시작으로 config 로 굳은 디바이스를
   * 사용자가 직접 삭제할 수 있도록 보호를 제거함 — 서버도 동일).
   */
  const resolveDeletableAgentId = (device: DeviceInfo): string | null => {
    if (!showLocalWrites) return null;
    const meta = agentMap.get(device.agent_name);
    if (!meta || !REMOVABLE_AGENT_TYPES.has(meta.type)) return null;
    return meta.id;
  };

  /** 삭제 확인 다이얼로그에서 확인 클릭 시 실행. */
  const handleConfirmDelete = () => {
    if (!deleteTarget) return;
    const agentId = resolveDeletableAgentId(deleteTarget);
    if (!agentId) {
      setDeleteTarget(null);
      return;
    }
    const target = deleteTarget;
    deleteDevice.mutate(
      { agentId, deviceId: target.uid ?? target.id },
      {
        onSuccess: () => {
          setDeleteTarget(null);
          addNotification({ type: 'success', message: t('devices.list.deleteSuccess') });
        },
        onError: (err) => {
          setDeleteTarget(null);
          addNotification({
            type: 'error',
            message: t('devices.list.deleteError').replace(
              '{message}',
              err instanceof Error ? err.message : t('devices.list.unknownError'),
            ),
          });
        },
      },
    );
  };

  /**
   * 디바이스가 상태 전송 토글 가능하면 소유 에이전트 ID 를 반환하고, 아니면 null.
   * 조건: 로컬 타깃 + 소유 에이전트 타입이 set_device 지원(samsung_hvacr01/lgap).
   */
  const resolveReportTogglableAgentId = (device: DeviceInfo): string | null => {
    if (!showLocalWrites) return null;
    const meta = agentMap.get(device.agent_name);
    if (!meta || !REPORT_TOGGLE_AGENT_TYPES.has(meta.type)) return null;
    return meta.id;
  };

  /** 상태 전송 on/off 토글. 낙관적 업데이트는 훅이 처리하며 실패 시 알림. */
  const handleToggleReport = (device: DeviceInfo, next: boolean) => {
    const agentId = resolveReportTogglableAgentId(device);
    if (!agentId) return;
    setDeviceReport.mutate(
      { agentId, deviceId: device.uid ?? device.id, reportEnabled: next },
      {
        onError: (err) => {
          addNotification({
            type: 'error',
            message: t('devices.list.reportError').replace(
              '{message}',
              err instanceof Error ? err.message : t('devices.list.unknownError'),
            ),
          });
        },
      },
    );
  };


  // --- 로딩 상태 ---
  if (isLoading && !data) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-end">
          <div className="h-10 w-28 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
        <div className="flex items-center gap-3">
          <div className="h-10 flex-1 animate-pulse rounded bg-(--color-bg-elevated)" />
          <div className="h-10 w-36 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
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
            {t('devices.loadError')}
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
          localHref="/devices"
        />
      )}

      {/* 툴바: 컬럼 설정(항상 표시) + 디바이스 추가(로컬 전용). 원격 타깃에서는
          로컬 추가 어포던스를 숨긴다(디바이스 추가는 그룹 D 명령 경로). */}
      <div className="flex items-center justify-end gap-2">
        <ColumnsSettingButton
          open={showColumnsMenu}
          onOpenChange={setShowColumnsMenu}
          visibleColumns={visibleColumns}
          onChange={setColumns}
        />
        {showLocalWrites && (
          <PermissionButton
            type="button"
            permission="device.create"
            onClick={() => setShowAddDialog(true)}
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            {t('devices.addDevice')}
          </PermissionButton>
        )}
      </div>

      {/* 검색 및 필터 */}
      <DeviceSearchFilter
        search={searchQuery}
        onSearchChange={handleSearchChange}
        statusFilter={statusFilter}
        onStatusFilterChange={handleStatusFilterChange}
        protocolFilter={filters.protocol ?? ''}
        onProtocolFilterChange={handleProtocolFilterChange}
        typeFilter={filters.type ?? ''}
        onTypeFilterChange={handleTypeFilterChange}
      />

      {/* 테이블 또는 빈 상태 */}
      {filteredDevices.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center">
          <HardDrive className="mx-auto h-12 w-12 text-(--color-border-strong)" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {devices.length === 0
              ? t('devices.emptyTitle')
              : t('devices.noSearchResults')}
          </p>
          {devices.length === 0 && showLocalWrites && (
            <PermissionButton
              type="button"
              permission="device.create"
              onClick={() => setShowAddDialog(true)}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-4 w-4" />
              {t('devices.addDevice')}
            </PermissionButton>
          )}
        </div>
      ) : (
        <>
          {/* 페이지네이션 */}
          <div className="flex items-center justify-between">
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
              <span className="ml-2 text-(--color-text-muted)">|</span>
              <span className="ml-2">
                {t('common.pagination.range')
                  .replace('{total}', String(totalItems))
                  .replace('{start}', String(startIndex + 1))
                  .replace('{end}', String(Math.min(startIndex + pageSize, totalItems)))}
              </span>
            </div>

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

          {/* 디바이스 테이블 (선택된 컬럼만 헤더/셀 렌더) */}
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-primary)">
                <tr>
                  <th className="w-8 px-3 py-3" />
                  {visibleColumns.map((col) =>
                    col === 'source' ? (
                      // '등록' 컬럼은 정렬 비대상 (기존 동작 보존)
                      <th
                        key={col}
                        className="px-4 py-3 text-left text-xs font-medium text-(--color-text-muted) uppercase tracking-wider"
                      >
                        {t(DEVICE_COLUMN_LABELS[col])}
                      </th>
                    ) : (
                      <SortableHeader
                        key={col}
                        label={t(DEVICE_COLUMN_LABELS[col])}
                        field={col}
                        currentSort={sort}
                        onSort={handleSort}
                        className="px-4 py-3"
                      />
                    ),
                  )}
                  {/* 액션(상태 전송 토글 + 삭제) 컬럼 — 정렬 비대상 빈 헤더. */}
                  <th className="w-24 px-3 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
                {pagedDevices.map((device) => {
                  const isExpanded = expandedId === device.id;
                  const canDelete = resolveDeletableAgentId(device) !== null;
                  const canToggleReport = resolveReportTogglableAgentId(device) !== null;
                  return (
                    <DeviceRow
                      key={device.id}
                      device={device}
                      columns={visibleColumns}
                      isExpanded={isExpanded}
                      onToggle={() => toggleExpand(device.id)}
                      onDelete={canDelete ? () => setDeleteTarget(device) : undefined}
                      onToggleReport={
                        canToggleReport
                          ? (next) => handleToggleReport(device, next)
                          : undefined
                      }
                      t={t}
                    />
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {/* 디바이스 추가 다이얼로그 (로컬 전용) */}
      {showLocalWrites && showAddDialog && (
        <AddDeviceDialog onClose={() => setShowAddDialog(false)} />
      )}

      {/* 디바이스 삭제 확인 다이얼로그 */}
      {deleteTarget && (
        <ConfirmDialog
          isOpen
          onClose={() => setDeleteTarget(null)}
          onConfirm={handleConfirmDelete}
          title={t('devices.list.deleteTitle')}
          message={t('devices.list.deleteMessage').replace(
            '{name}',
            getDeviceDisplayName(deleteTarget),
          )}
          confirmLabel={t('common.delete')}
          variant="danger"
          isSubmitting={deleteDevice.isPending}
        />
      )}
    </div>
    </TargetProvider>
  );
}

// ---- 컬럼 설정 버튼 (체크박스 토글 팝오버) ----

interface ColumnsSettingButtonProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  visibleColumns: DeviceListColumnKey[];
  onChange: (cols: DeviceListColumnKey[]) => void;
}

/** 목록 상단 "컬럼 설정" 버튼 + 체크박스 토글 팝오버 (최소 1개 강제). */
function ColumnsSettingButton({
  open,
  onOpenChange,
  visibleColumns,
  onChange,
}: ColumnsSettingButtonProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);

  // 바깥 클릭 시 팝오버 닫기
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onOpenChange(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open, onOpenChange]);

  const toggle = (key: DeviceListColumnKey) => {
    if (visibleColumns.includes(key)) {
      // 최소 1개 강제 — 마지막 1개는 해제 불가
      if (visibleColumns.length > 1) {
        onChange(visibleColumns.filter((c) => c !== key));
      }
    } else {
      onChange([...visibleColumns, key]);
    }
  };

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-haspopup="true"
        aria-expanded={open}
        className="inline-flex items-center gap-2 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
      >
        <Columns3 className="h-4 w-4" />
        {t('devices.columnsSetting')}
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 z-20 mt-1 w-44 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-1 shadow-lg"
        >
          {ALL_DEVICE_COLUMNS.map((key) => {
            const checked = visibleColumns.includes(key);
            const lastOne = checked && visibleColumns.length === 1;
            return (
              <label
                key={key}
                className={cn(
                  'flex items-center gap-2 rounded-md px-2 py-1.5 transition-colors',
                  lastOne
                    ? 'cursor-not-allowed opacity-60'
                    : 'cursor-pointer hover:bg-(--color-bg-elevated)',
                )}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={lastOne}
                  onChange={() => toggle(key)}
                  className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-(--color-text-primary)">
                  {t(DEVICE_COLUMN_LABELS[key])}
                </span>
              </label>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ---- 디바이스 행 컴포넌트 ----

interface DeviceRowProps {
  device: DeviceInfo;
  columns: DeviceListColumnKey[];
  isExpanded: boolean;
  onToggle: () => void;
  /**
   * 삭제 요청 핸들러. 삭제 불가 디바이스(설정 소스/미지원 에이전트/원격)면 undefined 이며,
   * 이 경우 휴지통 아이콘을 렌더하지 않는다.
   */
  onDelete?: () => void;
  /**
   * 상태 전송 on/off 토글 핸들러. 토글 미지원 디바이스(미지원 에이전트/원격)면 undefined 이며,
   * 이 경우 스위치를 렌더하지 않는다. `next` 는 전환할 목표 값이다.
   */
  onToggleReport?: (next: boolean) => void;
  /** 번역 함수(상위에서 주입). */
  t: TranslationFn;
}

/** 상태 전송 on/off 스위치. 디바이스 행 액션 셀에서 사용한다. */

function DeviceRow({
  device,
  columns,
  isExpanded,
  onToggle,
  onDelete,
  onToggleReport,
  t,
}: DeviceRowProps) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer transition-colors hover:bg-(--color-bg-elevated)"
      >
        {/* 확장 아이콘 */}
        <td className="px-3 py-3 text-(--color-text-muted)">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>

        {columns.map((col) => (
          <DeviceCell key={col} column={col} device={device} t={t} />
        ))}

        {/* 액션 셀. 상태 전송 토글(지원 디바이스) + 삭제(삭제 가능 디바이스)를 렌더한다.
            행 클릭(확장)과 겹치지 않도록 각 컨트롤에서 stopPropagation 한다. */}
        <td className="px-3 py-3">
          <div className="flex items-center justify-end gap-2">
            {onToggleReport && (
              <ReportToggleSwitch
                enabled={device.report_enabled ?? true}
                onToggle={onToggleReport}
                t={t}
              />
            )}
            {onDelete && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete();
                }}
                className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 disabled:opacity-50 dark:hover:bg-red-950"
                title={t('devices.list.deleteTooltip')}
                aria-label={t('devices.list.deleteTooltip')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </td>
      </tr>

      {/* 확장된 상세 패널 (colspan = 확장 아이콘 1 + 표시 컬럼 수 + 액션 1) */}
      {isExpanded && (
        <tr>
          <td colSpan={columns.length + 2} className="bg-(--color-bg-sunken)">
            <DeviceDetailPanel deviceId={device.id} />
          </td>
        </tr>
      )}
    </>
  );
}
