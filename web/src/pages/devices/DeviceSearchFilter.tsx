// 디바이스 검색 및 필터 바.
// 이름 검색, 상태 뱃지, 프로토콜/타입 필터를 제공하여 디바이스 목록을 필터링한다.

import { Search, Wifi, WifiOff } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

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

/** 상태 필터 옵션. label 은 i18n 키로 저장하고 렌더 시 t() 로 변환한다. */
const STATUS_OPTIONS = [
  { value: 'online', labelKey: 'devices.status.online', icon: <Wifi className="h-3.5 w-3.5" />, color: 'text-green-600 dark:text-green-400', activeColor: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  { value: 'offline', labelKey: 'devices.status.offline', icon: <WifiOff className="h-3.5 w-3.5" />, color: 'text-(--color-text-muted)', activeColor: 'bg-(--color-bg-sunken) text-(--color-text-secondary)' },
];

/** 프로토콜 필터 옵션 (라벨은 고유명사라 번역 대상 아님) */
const PROTOCOL_OPTIONS = [
  { value: 'samsung_nasa', label: 'Samsung NASA' },
  { value: 'lgap', label: 'LGAP' },
  { value: 'modbus', label: 'Modbus' },
];

/** 타입 필터 옵션 (v0.18.3: HVACR.IDU/HVACR.ODU). label 은 i18n 키. */
const TYPE_OPTIONS = [
  { value: 'HVACR.IDU', labelKey: 'devices.type.indoor' },
  { value: 'HVACR.ODU', labelKey: 'devices.type.outdoor' },
  { value: 'sensor', labelKey: 'devices.type.sensor' },
  { value: 'controller', labelKey: 'devices.type.controller' },
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
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* 검색 입력 */}
      <div className="relative min-w-[200px] flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-(--color-text-muted)" />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={t('devices.filter.searchPlaceholder')}
          className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) py-2 pl-9 pr-3 text-sm text-(--color-text-primary) placeholder-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
      </div>

      {/* 상태 필터 뱃지 */}
      <div className="flex items-center gap-1.5">
        {STATUS_OPTIONS.map((opt) => {
          const isActive = statusFilter === opt.value;
          const label = t(opt.labelKey);
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
              title={label}
            >
              {opt.icon}
              {label}
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
        <option value="">{t('devices.filter.allProtocols')}</option>
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
        <option value="">{t('devices.filter.allTypes')}</option>
        {TYPE_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {t(opt.labelKey)}
          </option>
        ))}
      </select>
    </div>
  );
}
