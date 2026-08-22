// 디바이스 패널 컴포넌트.
// 상단에 상태별 요약 뱃지(전체/온라인/오프라인), 하단에 디바이스 리스트 테이블을 표시한다.

import { useEffect, useMemo, useState } from 'react';
import {
  ArrowRight,
  HardDrive,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { Link } from 'react-router';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useDevicesTarget } from '@/hooks/useResourceTargets';
import { useTranslation } from '@/lib/i18n';
import { usePanelTitleVisible } from '../panelChromeContext';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { getDeviceDisplayName, getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import { type DeviceColumnKey, ALL_DEVICE_COLUMNS } from '@/stores/uiStore';

/** 상대 시간 포맷 (예: "3분 전") */
function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const then = date.getTime();
  if (isNaN(then)) return '-';

  // Go zero time ("0001-01-01T00:00:00Z") 등 유효하지 않은 과거 날짜 처리
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

interface DevicePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  refreshMs: number;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 디바이스 상태 요약 + 디바이스 리스트 테이블 패널 */
export default function DevicePanel({
  panelId: _panelId,
  title,
  config: _config,
  refreshMs: _refreshMs,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: DevicePanelProps) {
  const showTitle = usePanelTitleVisible();
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

  // 컬럼 가시성 상태 (스토어 config에서 읽기)
  const visibleColumns = useMemo(
    () => (_config.visibleColumns as DeviceColumnKey[]) ?? [...ALL_DEVICE_COLUMNS],
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
    if (!visibleColumns.includes(sort.field as DeviceColumnKey)) {
      setSort({ field: 'name', direction: 'asc' });
    }
  }, [visibleColumns, sort.field]);

  const show = (key: DeviceColumnKey) => visibleColumns.includes(key);

  // 상태 요약 집계
  const summary = useMemo(() => {
    const total = devices.length;
    const online = devices.filter((d) => d.online).length;
    const offline = total - online;
    return { total, online, offline };
  }, [devices]);

  // 정렬된 디바이스 목록 (최대 10개)
  const sortedDevices = useMemo(() => {
    const sorted = [...devices].sort((a, b) => {
      let valA: string;
      let valB: string;

      switch (sort.field) {
        case 'name':
          valA = (a.name || a.id).toLowerCase();
          valB = (b.name || b.id).toLowerCase();
          break;
        case 'type':
          valA = a.type.toLowerCase();
          valB = b.type.toLowerCase();
          break;
        case 'status':
          valA = a.online ? '1' : '0';
          valB = b.online ? '1' : '0';
          break;
        case 'agent':
          valA = a.agent_name.toLowerCase();
          valB = b.agent_name.toLowerCase();
          break;
        case 'last_seen':
          valA = a.last_seen ?? '';
          valB = b.last_seen ?? '';
          break;
        default:
          valA = (a.name || a.id).toLowerCase();
          valB = (b.name || b.id).toLowerCase();
      }

      if (valA < valB) return sort.direction === 'asc' ? -1 : 1;
      if (valA > valB) return sort.direction === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted.slice(0, 10);
  }, [devices, sort]);

  /** 정렬 변경 핸들러 */
  const handleSort = (field: string) => {
    setSort((prev) => ({
      field,
      direction: prev.field === field && prev.direction === 'asc' ? 'desc' : 'asc',
    }));
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
              style={acColor('header') ? { color: acColor('header')! } : undefined}
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
          <div className="mb-6 flex shrink-0 gap-3">
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

          {/* 디바이스 리스트 테이블 */}
          {sortedDevices.length === 0 ? (
            <p className="text-sm text-(--color-text-muted)">
              {t('dashboard.devicePanel.empty')}
            </p>
          ) : (
            <div className="min-h-0 flex-1 overflow-y-auto">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-(--color-border-default)">
                      {show('name') && (
                        <SortableHeader
                          label={t('dashboard.col.name')}
                          field="name"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                          accentColor={acColor('table') ?? panelColor}
                        />
                      )}
                      {show('type') && (
                        <SortableHeader
                          label={t('dashboard.col.type')}
                          field="type"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                          accentColor={acColor('table') ?? panelColor}
                        />
                      )}
                      {show('status') && (
                        <th
                          className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                          style={acColor('table') ? { color: acColor('table')! } : undefined}
                        >
                          {t('dashboard.col.status')}
                        </th>
                      )}
                      {show('agent') && (
                        <SortableHeader
                          label={t('dashboard.col.agent')}
                          field="agent"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                          accentColor={acColor('table') ?? panelColor}
                        />
                      )}
                      {show('last_seen') && (
                        <SortableHeader
                          label={t('dashboard.col.lastSeen')}
                          field="last_seen"
                          currentSort={sort}
                          onSort={handleSort}
                          className="px-4 py-3"
                          accentColor={acColor('table') ?? panelColor}
                        />
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {sortedDevices.map((device) => (
                      <tr
                        key={device.uid ?? device.id}
                        className="transition-colors hover:bg-(--color-bg-elevated)"
                      >
                        {show('name') && (
                          <td className="px-4 py-3">
                            <span className="text-sm font-medium text-(--color-text-primary)">
                              {getDeviceDisplayName(device)}
                            </span>
                          </td>
                        )}
                        {show('type') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-secondary)">
                            {getDeviceTypeLabel(device.type)}
                          </td>
                        )}
                        {show('status') && (
                          <td className="px-4 py-3">
                            {device.online ? (
                              <span className="inline-flex items-center text-green-600 dark:text-green-400" title={t('dashboard.panel.online')}>
                                <Wifi className="h-4 w-4" />
                              </span>
                            ) : (
                              <span className="inline-flex items-center text-gray-400 dark:text-gray-500" title={t('dashboard.panel.offline')}>
                                <WifiOff className="h-4 w-4" />
                              </span>
                            )}
                          </td>
                        )}
                        {show('agent') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-secondary)">
                            {device.agent_name}
                          </td>
                        )}
                        {show('last_seen') && (
                          <td className="px-4 py-3 text-sm text-(--color-text-muted)">
                            {formatRelativeTime(device.last_seen)}
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {/* 더 보기 링크 — 원격은 로컬 `/devices` 로 이탈하므로 숨긴다. */}
              {!remote && devices.length > 10 && (
                <div className="mt-4 text-right">
                  <Link
                    to="/devices"
                    className="inline-flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                  >
                    {t('dashboard.panel.more')}
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
