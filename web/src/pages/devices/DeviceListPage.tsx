// 디바이스 관리 페이지.
// 디바이스 목록을 테이블로 표시하며, 행 클릭으로 상세 패널을 토글한다.
// 필터 바, 로딩/에러/빈 상태를 포함한다.

import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, HardDrive, Search } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useDevices } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { DeviceInfo, DeviceListParams } from '@/types/device';

import DeviceDetailPanel from './DeviceDetailPanel';
import DeviceStatusBadge from './DeviceStatusBadge';

/** 상대 시간 포맷 (예: "3분 전") */
function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '-';
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  if (isNaN(then)) return '-';

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

/** 디바이스 타입별 한글 표시명 */
function deviceTypeLabel(type: string): string {
  switch (type) {
    case 'indoor':
      return '실내기';
    case 'outdoor':
      return '실외기';
    case 'sensor':
      return '센서';
    case 'controller':
      return '컨트롤러';
    case 'gateway':
      return '게이트웨이';
    default:
      return type;
  }
}

export default function DeviceListPage() {
  const refreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;

  // 필터 상태
  const [filters, setFilters] = useState<DeviceListParams>({});
  const [searchQuery, setSearchQuery] = useState('');

  const { data, isLoading, error, refetch } = useDevices(filters, refreshMs);

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 확장 행 상태
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const devices: DeviceInfo[] = data?.data ?? [];

  // 클라이언트 측 검색 필터
  const filteredDevices = useMemo(() => {
    if (!searchQuery.trim()) return devices;
    const q = searchQuery.toLowerCase();
    return devices.filter(
      (d) =>
        d.name.toLowerCase().includes(q) ||
        d.id.toLowerCase().includes(q) ||
        d.type.toLowerCase().includes(q) ||
        d.agent_name.toLowerCase().includes(q),
    );
  }, [devices, searchQuery]);

  // 클라이언트 측 정렬
  const sortedDevices = useMemo(() => {
    const sorted = [...filteredDevices];
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;

    sorted.sort((a, b) => {
      switch (field) {
        case 'name': {
          const va = (a.name || a.id).toLowerCase();
          const vb = (b.name || b.id).toLowerCase();
          return va < vb ? -1 * mul : va > vb ? 1 * mul : 0;
        }
        case 'type': {
          const va = a.type.toLowerCase();
          const vb = b.type.toLowerCase();
          return va < vb ? -1 * mul : va > vb ? 1 * mul : 0;
        }
        case 'protocol': {
          const va = a.protocol.toLowerCase();
          const vb = b.protocol.toLowerCase();
          return va < vb ? -1 * mul : va > vb ? 1 * mul : 0;
        }
        case 'online': {
          const va = a.online ? 1 : 0;
          const vb = b.online ? 1 : 0;
          return (va - vb) * mul;
        }
        default:
          return 0;
      }
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
  };

  /** 행 클릭 시 상세 패널 토글 */
  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  /** 필터 변경 핸들러 */
  const handleFilterChange = (key: keyof DeviceListParams, value: string) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (!value) {
        delete next[key];
      } else if (key === 'online') {
        (next as Record<string, unknown>)[key] = value === 'true';
      } else {
        (next as Record<string, unknown>)[key] = value;
      }
      return next;
    });
  };

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">디바이스</h2>
      </div>

      {/* 필터 바 */}
      <div className="flex flex-wrap items-center gap-3">
        {/* 검색 */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="이름, ID, 타입 검색..."
            className="rounded-md border border-gray-300 py-2 pl-9 pr-3 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-blue-400"
          />
        </div>

        {/* 프로토콜 필터 */}
        <select
          value={filters.protocol ?? ''}
          onChange={(e) => handleFilterChange('protocol', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300"
        >
          <option value="">전체 프로토콜</option>
          <option value="nasa">NASA</option>
          <option value="modbus">Modbus</option>
        </select>

        {/* 타입 필터 */}
        <select
          value={filters.type ?? ''}
          onChange={(e) => handleFilterChange('type', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300"
        >
          <option value="">전체 타입</option>
          <option value="indoor">실내기</option>
          <option value="outdoor">실외기</option>
          <option value="sensor">센서</option>
          <option value="controller">컨트롤러</option>
          <option value="gateway">게이트웨이</option>
        </select>

        {/* 온라인 필터 */}
        <select
          value={filters.online != null ? String(filters.online) : ''}
          onChange={(e) => handleFilterChange('online', e.target.value)}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300"
        >
          <option value="">전체 상태</option>
          <option value="true">온라인</option>
          <option value="false">오프라인</option>
        </select>

        {/* 에이전트 필터 */}
        <input
          type="text"
          value={filters.agent ?? ''}
          onChange={(e) => handleFilterChange('agent', e.target.value)}
          placeholder="에이전트 필터"
          className="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 placeholder-gray-400 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300 dark:placeholder-gray-500"
        />
      </div>

      {/* 로딩 스켈레톤 */}
      {isLoading && (
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
          <div className="divide-y divide-gray-200 dark:divide-gray-700">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-6 py-4">
                <div className="h-4 w-32 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="ml-auto h-4 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 에러 상태 */}
      {error && !isLoading && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-600 dark:text-red-400">
            디바이스 목록을 불러오는데 실패했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700"
          >
            다시 시도
          </button>
        </div>
      )}

      {/* 빈 상태 */}
      {!isLoading && !error && devices.length === 0 && (
        <div className="rounded-lg border border-gray-200 bg-white p-12 text-center dark:border-gray-700 dark:bg-gray-800">
          <HardDrive className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <h3 className="mt-4 text-lg font-medium text-gray-900 dark:text-white">
            등록된 디바이스가 없습니다
          </h3>
          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
            에이전트를 시작하면 디바이스가 자동으로 검색됩니다.
          </p>
        </div>
      )}

      {/* 검색 결과 없음 */}
      {!isLoading && !error && devices.length > 0 && sortedDevices.length === 0 && (
        <div className="rounded-lg border border-gray-200 bg-white p-8 text-center dark:border-gray-700 dark:bg-gray-800">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            검색 조건에 맞는 디바이스가 없습니다.
          </p>
        </div>
      )}

      {/* 디바이스 테이블 */}
      {!isLoading && !error && sortedDevices.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="w-8 px-3 py-3" />
                <SortableHeader
                  label="이름"
                  field="name"
                  currentSort={sort}
                  onSort={handleSort}
                  className="px-6 py-3"
                />
                <SortableHeader
                  label="타입"
                  field="type"
                  currentSort={sort}
                  onSort={handleSort}
                  className="px-6 py-3"
                />
                <SortableHeader
                  label="프로토콜"
                  field="protocol"
                  currentSort={sort}
                  onSort={handleSort}
                  className="px-6 py-3"
                />
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  에이전트
                </th>
                <SortableHeader
                  label="상태"
                  field="online"
                  currentSort={sort}
                  onSort={handleSort}
                  className="px-6 py-3"
                />
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  마지막 통신
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  기능
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-900">
              {sortedDevices.map((device) => {
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
      )}
    </div>
  );
}

// ---- 디바이스 행 컴포넌트 ----

interface DeviceRowProps {
  device: DeviceInfo;
  isExpanded: boolean;
  onToggle: () => void;
}

/** 디바이스 테이블 행 (확장 가능) */
function DeviceRow({ device, isExpanded, onToggle }: DeviceRowProps) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-800"
      >
        {/* 확장 아이콘 */}
        <td className="px-3 py-4 text-gray-400">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>

        {/* 이름 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">
          {device.name || device.id}
        </td>

        {/* 타입 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {deviceTypeLabel(device.type)}
        </td>

        {/* 프로토콜 */}
        <td className="whitespace-nowrap px-6 py-4">
          <span
            className={cn(
              'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
              device.protocol === 'nasa'
                ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                : 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
            )}
          >
            {device.protocol.toUpperCase()}
          </span>
        </td>

        {/* 에이전트 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {device.agent_name}
        </td>

        {/* 온라인 상태 */}
        <td className="whitespace-nowrap px-6 py-4">
          <DeviceStatusBadge online={device.online} />
        </td>

        {/* 마지막 통신 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {formatRelativeTime(device.last_seen)}
        </td>

        {/* 기능 수 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {device.capabilities?.length ?? 0}
        </td>
      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={8} className="bg-gray-50 dark:bg-gray-800/50">
            <DeviceDetailPanel deviceId={device.id} />
          </td>
        </tr>
      )}
    </>
  );
}
