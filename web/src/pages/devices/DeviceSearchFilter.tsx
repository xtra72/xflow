// 디바이스 검색 및 필터 바.
// 이름 검색, 상태 뱃지, 프로토콜/타입 필터를 제공하여 디바이스 목록을 필터링한다.

import { Search, Wifi, WifiOff } from 'lucide-react';

interface DeviceSearchFilterProps {
  search: string;
  onSearchChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
  protocolFilter: string;
  onProtocolFilterChange: (v: string) => void;
  typeFilter: string;
  onTypeFilterChange: (v: string) => void;
}

/** 상태 필터 옵션 */
const STATUS_OPTIONS = [
  { value: 'online', label: '온라인', icon: <Wifi className="h-3.5 w-3.5" />, color: 'text-green-600 dark:text-green-400', activeColor: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  { value: 'offline', label: '오프라인', icon: <WifiOff className="h-3.5 w-3.5" />, color: 'text-gray-500 dark:text-gray-400', activeColor: 'bg-gray-100 text-gray-700 dark:bg-gray-700/30 dark:text-gray-300' },
];

/** 프로토콜 필터 옵션 */
const PROTOCOL_OPTIONS = [
  { value: 'samsung_nasa', label: 'Samsung NASA' },
  { value: 'lgap', label: 'LGAP' },
  { value: 'modbus', label: 'Modbus' },
];

/** 타입 필터 옵션 (v0.18.3: HVACR.IDU/HVACR.ODU) */
const TYPE_OPTIONS = [
  { value: 'HVACR.IDU', label: '실내기' },
  { value: 'HVACR.ODU', label: '실외기' },
  { value: 'sensor', label: '센서' },
  { value: 'controller', label: '제어기' },
];

export default function DeviceSearchFilter({
  search,
  onSearchChange,
  statusFilter,
  onStatusFilterChange,
  protocolFilter,
  onProtocolFilterChange,
  typeFilter,
  onTypeFilterChange,
}: DeviceSearchFilterProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* 검색 입력 */}
      <div className="relative min-w-[200px] flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="이름, ID, 타입, 에이전트로 검색..."
          className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) py-2 pl-9 pr-3 text-sm text-(--color-text-primary) placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
      </div>

      {/* 상태 필터 뱃지 */}
      <div className="flex items-center gap-1.5">
        {STATUS_OPTIONS.map((opt) => {
          const isActive = statusFilter === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => onStatusFilterChange(isActive ? '' : opt.value)}
              className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors ${
                isActive
                  ? opt.activeColor
                  : `${opt.color} hover:bg-(--color-bg-elevated)`
              }`}
              title={opt.label}
            >
              {opt.icon}
              {opt.label}
            </button>
          );
        })}
      </div>

      {/* 프로토콜 필터 */}
      <select
        value={protocolFilter}
        onChange={(e) => onProtocolFilterChange(e.target.value)}
        className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        <option value="">모든 프로토콜</option>
        {PROTOCOL_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>

      {/* 타입 필터 */}
      <select
        value={typeFilter}
        onChange={(e) => onTypeFilterChange(e.target.value)}
        className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        <option value="">모든 타입</option>
        {TYPE_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  );
}
