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

interface FlowSearchFilterProps {
  search: string;
  onSearchChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
}

/** 상태 필터 옵션 (아이콘 기반) */
const STATUS_OPTIONS = [
  { value: 'running', label: '실행 중', icon: <Activity className="h-3.5 w-3.5" />, color: 'text-green-600 dark:text-green-400', activeColor: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' },
  { value: 'stopped', label: '중지됨', icon: <CircleStop className="h-3.5 w-3.5" />, color: 'text-gray-500 dark:text-gray-400', activeColor: 'bg-gray-100 text-gray-700 dark:bg-gray-700/30 dark:text-gray-300' },
  { value: 'error', label: '오류', icon: <AlertTriangle className="h-3.5 w-3.5" />, color: 'text-red-500 dark:text-red-400', activeColor: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400' },
  { value: 'stored', label: '저장됨', icon: <FileText className="h-3.5 w-3.5" />, color: 'text-blue-500 dark:text-blue-400', activeColor: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' },
  { value: 'loaded', label: '탑재됨', icon: <Rocket className="h-3.5 w-3.5" />, color: 'text-yellow-500 dark:text-yellow-400', activeColor: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' },
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
  return (
    <div className="flex items-center gap-3">
      {/* 검색 입력 */}
      <div className="relative flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="플로우 검색..."
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
    </div>
  );
}
