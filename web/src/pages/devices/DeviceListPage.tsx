// 디바이스 관리 페이지.
// 에이전트 페이지와 동일한 테이블 리스트 형식으로 디바이스를 표시한다.
// 행 클릭으로 상세 패널을 토글하고, 검색/필터/페이지네이션을 지원한다.

import { useMemo, useState } from 'react';
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  HardDrive,
  Lock,
  Plus,
} from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { RemoteTargetBanner } from '@/components/remote/RemoteTargetBanner';
import { useDevicesTarget } from '@/hooks/useResourceTargets';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTargetParam } from '@/hooks/useTargetParam';
import { TargetProvider } from '@/lib/remote/TargetContext';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import { getDeviceTypeLabel, getDeviceDisplayName } from '@/lib/utils/deviceLabels';
import { cn } from '@/lib/utils/cn';
import type { DeviceInfo, DeviceListParams } from '@/types/device';

import DeviceDetailPanel from './DeviceDetailPanel';
import DeviceStatusBadge from './DeviceStatusBadge';
import DeviceSearchFilter from './DeviceSearchFilter';
import AddDeviceDialog from './AddDeviceDialog';

/** 페이지 크기 옵션 */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

// 디바이스 source 값을 사용자 친화적 라벨로 매핑.
// 수동(manual)=config|pinned, 자동(auto)=auto|bridge.
function sourceVariant(source: string): { label: string; manual: boolean } | null {
  switch (source) {
    case 'config':
      return { label: '설정', manual: true };
    case 'pinned':
      return { label: '고정', manual: true };
    case 'auto':
      return { label: '자동', manual: false };
    case 'bridge':
      return { label: '브리지', manual: false };
    default:
      return source ? { label: source, manual: false } : null;
  }
}

/** 프로토콜 배지 색상 */
const PROTOCOL_COLORS: Record<string, string> = {
  samsung_nasa: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  lgap: 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400',
  modbus: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
};

/** 상대 시간 포맷 (예: "3분 전") */
function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const then = date.getTime();
  if (isNaN(then)) return '-';
  if (date.getUTCFullYear() < 2000) return '-';

  const now = Date.now();
  const diffMs = now - then;
  if (diffMs < 0) return '방금';

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}초 전`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}분 전`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}시간 전`;

  const days = Math.floor(hours / 24);
  return `${days}일 전`;
}

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

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 확장 상태
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // 페이지네이션 상태
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // 디바이스 추가 다이얼로그
  const [showAddDialog, setShowAddDialog] = useState(false);

  const devices: DeviceInfo[] = data?.data ?? [];

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
            디바이스 목록을 불러오는 중 오류가 발생했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            다시 시도
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

      {/* 액션 버튼 (원격 타깃에서는 로컬 추가 어포던스 숨김 — 디바이스 추가는
          그룹 D 명령 경로) */}
      {showLocalWrites && (
      <div className="flex items-center justify-end">
        <button
          type="button"
          onClick={() => setShowAddDialog(true)}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-4 w-4" />
          디바이스 추가
        </button>
      </div>
      )}

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
          <HardDrive className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {devices.length === 0
              ? '등록된 디바이스가 없습니다. 에이전트를 시작하면 디바이스가 자동으로 검색됩니다.'
              : '검색 결과가 없습니다.'}
          </p>
          {devices.length === 0 && showLocalWrites && (
            <button
              type="button"
              onClick={() => setShowAddDialog(true)}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-4 w-4" />
              디바이스 추가
            </button>
          )}
        </div>
      ) : (
        <>
          {/* 페이지네이션 */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
              <span>페이지당</span>
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
              <span>건</span>
              <span className="ml-2 text-gray-400">|</span>
              <span className="ml-2">
                총 {totalItems}건 중 {startIndex + 1}-
                {Math.min(startIndex + pageSize, totalItems)}건
              </span>
            </div>

            <div className="flex items-center gap-1">
              <button
                type="button"
                disabled={safePage <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="이전 페이지"
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
                aria-label="다음 페이지"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          </div>

          {/* 디바이스 테이블 */}
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-primary)">
                <tr>
                  <th className="w-8 px-3 py-3" />
                  <SortableHeader label="이름" field="name" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="타입" field="type" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="프로토콜" field="protocol" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="상태" field="status" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="에이전트" field="agent" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-left text-xs font-medium text-(--color-text-muted) uppercase tracking-wider">등록</th>
                  <SortableHeader label="최근 확인" field="last_seen" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
                {pagedDevices.map((device) => {
                  const isExpanded = expandedId === device.id;
                  return (
                    <DeviceRow
                      key={device.id}
                      device={device}
                      isExpanded={isExpanded}
                      onToggle={() => toggleExpand(device.id)}
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
    </div>
    </TargetProvider>
  );
}

// ---- 디바이스 행 컴포넌트 ----

interface DeviceRowProps {
  device: DeviceInfo;
  isExpanded: boolean;
  onToggle: () => void;
}

function DeviceRow({ device, isExpanded, onToggle }: DeviceRowProps) {
  const protocolColor = PROTOCOL_COLORS[device.protocol] ?? 'bg-(--color-bg-elevated) text-(--color-text-muted)';

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

        {/* 이름 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-(--color-text-primary)">
          {getDeviceDisplayName(device)}
        </td>

        {/* 타입 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {getDeviceTypeLabel(device.type)}
        </td>

        {/* 프로토콜 */}
        <td className="whitespace-nowrap px-4 py-3">
          <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', protocolColor)}>
            {device.protocol.toUpperCase()}
          </span>
        </td>

        {/* 상태 */}
        <td className="whitespace-nowrap px-4 py-3">
          <DeviceStatusBadge online={device.online} />
        </td>

        {/* 에이전트 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {device.agent_name}
        </td>

        {/* 등록 (자동/수동) */}
        <td className="whitespace-nowrap px-4 py-3">
          {(() => {
            const variant = sourceVariant(device.source);
            if (!variant) return <span className="text-xs text-(--color-text-muted)">-</span>;
            return (
              <span
                className={cn(
                  'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium',
                  variant.manual
                    ? 'bg-(--color-bg-elevated) text-(--color-text-muted)'
                    : 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400',
                )}
                title={variant.manual ? '수동 등록 (설정/고정)' : '자동 등록 (발견/브리지)'}
              >
                {variant.manual && <Lock className="h-2.5 w-2.5" />}
                {variant.label}
              </span>
            );
          })()}
        </td>

        {/* 최근 확인 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatRelativeTime(device.last_seen)}
        </td>

      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={8} className="bg-(--color-bg-sunken)">
            <DeviceDetailPanel deviceId={device.id} />
          </td>
        </tr>
      )}
    </>
  );
}
