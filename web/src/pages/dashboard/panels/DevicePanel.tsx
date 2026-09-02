// 디바이스 패널 컴포넌트.
// 상단에 상태별 요약 뱃지(전체/온라인/오프라인), 검색·필터 바, 하단에 디바이스
// 리스트 테이블을 표시한다.
//
// 표시 컬럼·셀 렌더·검색/필터 규칙은 디바이스 탭(DeviceListPage)과 동일한 모듈을
// 공유한다 (useDeviceColumns / DeviceCell / DeviceSearchFilter). 패널이 자체 사본을
// 갖고 있으면 탭에 컬럼이 늘어나도 패널은 따라가지 못한다.

import { useEffect, useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight, HardDrive, Wifi, WifiOff } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import {
  ALL_DEVICE_COLUMNS,
  DEVICE_COLUMN_LABELS,
  type DeviceListColumnKey,
} from '@/hooks/useDeviceColumns';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useDevicesTarget } from '@/hooks/useResourceTargets';
import { useTranslation } from '@/lib/i18n';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { DeviceCell } from '@/pages/devices/DeviceCell';
import DeviceSearchFilter from '@/pages/devices/DeviceSearchFilter';
import type { DeviceInfo } from '@/types/device';

/** 페이지 크기 옵션. 디바이스 탭과 동일하다. */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

/** 등록('source') 컬럼은 디바이스 탭과 동일하게 정렬 비대상이다. */
const UNSORTABLE_COLUMNS = new Set<DeviceListColumnKey>(['source']);

/**
 * 정렬 비교 키를 뽑는다. 디바이스 탭(DeviceListPage)의 정렬 규칙과 동일하다
 * — id 는 uid 우선, status 는 온라인이 먼저(asc), 나머지는 소문자 문자열 비교.
 */
function sortKey(device: DeviceInfo, field: string): string {
  switch (field) {
    case 'name':
      return (device.name || device.id).toLowerCase();
    case 'id':
      return (device.uid || device.id).toLowerCase();
    case 'type':
      return device.type.toLowerCase();
    case 'protocol':
      return device.protocol.toLowerCase();
    case 'status':
      return device.online ? '0' : '1';
    case 'agent':
      return device.agent_name.toLowerCase();
    case 'last_seen':
      return device.last_seen ?? '';
    default:
      return (device.name || device.id).toLowerCase();
  }
}

interface DevicePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  refreshMs: number;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 디바이스 상태 요약 + 검색/필터 + 디바이스 리스트 테이블 패널 */
export default function DevicePanel({
  panelId: _panelId,
  title,
  config: _config,
  refreshMs: _refreshMs,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: DevicePanelProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const { t } = useTranslation();
  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04): 원격이면 노드 미러 목록을
  // 소스로 쓴다(useDevicesTarget). 로컬은 기존 useDevicesRealtime 그대로(회귀 없음).
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localQuery = useDevicesRealtime();
  const remoteQuery = useDevicesTarget(target);
  const devices = useMemo(
    () => (remote ? (remoteQuery.data?.data ?? []) : (localQuery.data?.data ?? [])),
    [remote, remoteQuery.data, localQuery.data],
  );
  const isLoading = remote ? remoteQuery.isLoading : localQuery.isLoading;

  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 검색/필터 상태 — 디바이스 탭과 같은 축(검색어/상태/프로토콜/타입).
  // 탭은 프로토콜·타입을 서버 쿼리 파라미터로 넘기지만, 패널은 실시간 갱신 쿼리를
  // 재구독하지 않도록 클라이언트에서 거른다(백엔드 DeviceFilter 도 정확 일치라 결과 동일).
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [protocolFilter, setProtocolFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');

  // 페이지네이션 상태. 패널 안에서 전체 목록을 넘겨볼 수 있어야 하므로
  // 상한으로 잘라내고 디바이스 탭으로 넘기지 않는다.
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_OPTIONS[0]!);

  // 컬럼 가시성 상태 (패널 config). 미설정이면 전체 컬럼.
  const visibleColumns = useMemo(
    () => (_config.visibleColumns as DeviceListColumnKey[]) ?? [...ALL_DEVICE_COLUMNS],
    [_config.visibleColumns],
  );

  // 타이틀 상태 (prop 기반)
  const panelColor = _config.panelColor as string | undefined;
  const accentElements = (_config.accentElements as Record<string, string | boolean>) ?? {};
  const acColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };
  const [panelTitle, setPanelTitle] = useState(title);

  // 외부 title prop 변경 시 동기화
  useEffect(() => {
    setPanelTitle(title);
  }, [title]);

  // 숨겨진 컬럼으로 정렬 중이면 기본(name)으로 fallback
  useEffect(() => {
    if (!visibleColumns.includes(sort.field as DeviceListColumnKey)) {
      setSort({ field: 'name', direction: 'asc' });
    }
  }, [visibleColumns, sort.field]);

  // 필터링 — 디바이스 탭(DeviceListPage)의 검색/상태 규칙 + 프로토콜/타입 정확 일치.
  const filteredDevices = useMemo(() => {
    let result: DeviceInfo[] = devices;

    // 이름/ID/UID/타입/에이전트 검색 (탭과 동일 필드 집합).
    const q = search.trim().toLowerCase();
    if (q) {
      result = result.filter(
        (d) =>
          d.name.toLowerCase().includes(q) ||
          d.id.toLowerCase().includes(q) ||
          (d.uid?.toLowerCase().includes(q) ?? false) ||
          d.type.toLowerCase().includes(q) ||
          d.agent_name.toLowerCase().includes(q),
      );
    }
    if (statusFilter === 'online') result = result.filter((d) => d.online);
    else if (statusFilter === 'offline') result = result.filter((d) => !d.online);
    if (protocolFilter) result = result.filter((d) => d.protocol === protocolFilter);
    if (typeFilter) result = result.filter((d) => d.type === typeFilter);

    return result;
  }, [devices, search, statusFilter, protocolFilter, typeFilter]);

  // 상태 요약 집계 — 필터 결과 기준(요약과 표가 서로 다른 모집단을 말하지 않도록).
  const summary = useMemo(() => {
    const total = filteredDevices.length;
    const online = filteredDevices.filter((d) => d.online).length;
    return { total, online, offline: total - online };
  }, [filteredDevices]);

  // 정렬된 전체 목록 (페이지 슬라이스 전).
  const sortedDevices = useMemo(() => {
    const mul = sort.direction === 'asc' ? 1 : -1;
    return [...filteredDevices].sort((a, b) => {
      const va = sortKey(a, sort.field);
      const vb = sortKey(b, sort.field);
      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });
  }, [filteredDevices, sort]);

  // 페이지네이션 계산 (디바이스 탭과 동일). 필터로 총량이 줄어 현재 페이지가
  // 범위를 벗어나도 safePage 가 마지막 페이지로 당겨 빈 화면을 막는다.
  const totalItems = sortedDevices.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedDevices = sortedDevices.slice(startIndex, startIndex + pageSize);

  /** 정렬 변경 핸들러. 정렬이 바뀌면 첫 페이지로 되돌린다(탭과 동일). */
  const handleSort = (field: string) => {
    setSort((prev) => ({
      field,
      direction: prev.field === field && prev.direction === 'asc' ? 'desc' : 'asc',
    }));
    setPage(1);
  };

  /** 검색/필터 변경 핸들러 — 값 변경 시 첫 페이지로 되돌린다. */
  const withPageReset = <T,>(set: (v: T) => void) => (v: T) => {
    set(v);
    setPage(1);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-6 shadow">
      {/* 헤더: 타이틀 + 설정 */}
      {showTitle && (
        <div className="mb-4 flex shrink-0 items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <HardDrive className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
            <h3
              className="truncate text-lg font-semibold text-(--color-text-primary)"
              style={{ ...(acColor('header') ? { color: acColor('header')! } : undefined), ...titleStyle }}
            >
              {panelTitle}
            </h3>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <div
            className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600"
            style={acColor('header') ? { borderTopColor: acColor('header')! } : undefined}
          />
        </div>
      ) : (
        <>
          {/* 상태별 요약 뱃지 */}
          <div className="mb-3 flex shrink-0 flex-wrap gap-3">
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-blue-100 px-3 py-1 text-sm font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              <HardDrive className="h-4 w-4" />
              {t('dashboard.panel.total')} {summary.total}
            </span>
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-green-100 px-3 py-1 text-sm font-medium text-green-700 dark:bg-green-900/30 dark:text-green-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              <Wifi className="h-4 w-4" />
              {t('dashboard.panel.online')} {summary.online}
            </span>
            <span
              className="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-sm font-medium text-gray-600 dark:bg-gray-700/30 dark:text-gray-400"
              style={acColor('badges') ? { backgroundColor: `${acColor('badges')}20`, color: acColor('badges')! } : undefined}
            >
              <WifiOff className="h-4 w-4" />
              {t('dashboard.panel.offline')} {summary.offline}
            </span>
          </div>

          {/* 검색 + 필터 바 (디바이스 탭과 동일 컴포넌트) */}
          <div className="mb-4 shrink-0">
            <DeviceSearchFilter
              search={search}
              onSearchChange={withPageReset(setSearch)}
              statusFilter={statusFilter}
              onStatusFilterChange={withPageReset(setStatusFilter)}
              protocolFilter={protocolFilter}
              onProtocolFilterChange={withPageReset(setProtocolFilter)}
              typeFilter={typeFilter}
              onTypeFilterChange={withPageReset(setTypeFilter)}
            />
          </div>

          {/* 디바이스 리스트 테이블 */}
          {totalItems === 0 ? (
            <p className="text-sm text-(--color-text-muted)">
              {/* 디바이스가 아예 없는 것과 필터에 걸러진 것을 구분한다. */}
              {devices.length === 0
                ? t('dashboard.devicePanel.empty')
                : t('dashboard.devicePanel.noMatch')}
            </p>
          ) : (
            <div className="min-h-0 flex-1 overflow-y-auto">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-(--color-border-default)">
                      {visibleColumns.map((col) =>
                        UNSORTABLE_COLUMNS.has(col) ? (
                          <th
                            key={col}
                            className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                            style={acColor('table') ? { color: acColor('table')! } : undefined}
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
                            accentColor={acColor('table') ?? panelColor}
                          />
                        ),
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {pagedDevices.map((device) => (
                      <tr
                        key={device.uid ?? device.id}
                        className="transition-colors hover:bg-(--color-bg-elevated)"
                      >
                        {visibleColumns.map((col) => (
                          <DeviceCell key={col} column={col} device={device} t={t} />
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* 페이지네이션 — 패널 안에서 전체 목록을 넘겨본다(디바이스 탭 이탈 없음). */}
          {totalItems > 0 && (
            <div className="mt-3 flex shrink-0 flex-wrap items-center justify-between gap-2 text-xs text-(--color-text-muted)">
              <div className="flex items-center gap-1.5">
                <span>{t('common.pagination.perPage')}</span>
                <select
                  value={pageSize}
                  onChange={(e) => {
                    setPageSize(Number(e.target.value));
                    setPage(1);
                  }}
                  aria-label={t('common.pagination.perPage')}
                  className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-1.5 py-0.5 text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  {PAGE_SIZE_OPTIONS.map((size) => (
                    <option key={size} value={size}>
                      {size}
                    </option>
                  ))}
                </select>
                <span>{t('common.pagination.unit')}</span>
                <span className="ml-1.5 opacity-60">|</span>
                <span className="ml-1.5">
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
                  aria-label={t('common.pagination.prev')}
                  className="rounded-md border border-(--color-border-strong) p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                </button>
                <span className="px-2">
                  {safePage} / {totalPages}
                </span>
                <button
                  type="button"
                  disabled={safePage >= totalPages}
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  aria-label={t('common.pagination.next')}
                  className="rounded-md border border-(--color-border-strong) p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronRight className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
