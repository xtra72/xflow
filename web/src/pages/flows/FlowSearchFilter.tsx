// 플로우 검색 및 필터 바.
// 이름 검색과 상태 뱃지 필터를 제공하여 플로우 목록을 필터링한다.

import {
  Activity,
  AlertTriangle,
  CircleStop,
  FileText,
  Rocket,
  Search,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

interface FlowSearchFilterProps {
  search: string;
  onSearchChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
}

/** 상태 필터 옵션 (아이콘 기반) — label 은 i18n 키로 저장하고 렌더 시 t() 변환 */
const STATUS_OPTIONS = [
  { value: 'running', labelKey: 'flows.filter.statusRunning', icon: <Activity className="h-3.5 w-3.5" />, color: 'text-green-600 dark:text-green-400', activeColor: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  { value: 'stopped', labelKey: 'flows.filter.statusStopped', icon: <CircleStop className="h-3.5 w-3.5" />, color: 'text-(--color-text-muted)', activeColor: 'bg-(--color-bg-sunken) text-(--color-text-secondary)' },
  { value: 'error', labelKey: 'flows.filter.statusError', icon: <AlertTriangle className="h-3.5 w-3.5" />, color: 'text-red-500 dark:text-red-400', activeColor: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400' },
  { value: 'stored', labelKey: 'flows.filter.statusStored', icon: <FileText className="h-3.5 w-3.5" />, color: 'text-blue-500 dark:text-blue-400', activeColor: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' },
  { value: 'loaded', labelKey: 'flows.filter.statusLoaded', icon: <Rocket className="h-3.5 w-3.5" />, color: 'text-yellow-500 dark:text-yellow-400', activeColor: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' },
];

/**
 * 플로우 검색 및 상태 필터 바.
 * 부모 컴포넌트에서 상태를 관리하고 값만 전달받는다.
 */
export default function FlowSearchFilter({
  search,
  onSearchChange,
  statusFilter,
  onStatusFilterChange,
}: FlowSearchFilterProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-3">
      {/* 검색 입력 */}
      <div className="relative flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-(--color-text-muted)" />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={t('flows.filter.searchPlaceholder')}
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
    </div>
  );
}
