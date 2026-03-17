// 플로우 검색 및 필터 바.
// 이름 검색과 상태 필터를 제공하여 플로우 목록을 필터링한다.

import { Search } from 'lucide-react';

interface FlowSearchFilterProps {
  search: string;
  onSearchChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
}

/** 상태 필터 옵션 목록 */
const STATUS_OPTIONS = [
  { value: '', label: '전체 상태' },
  { value: 'running', label: '실행 중' },
  { value: 'stopped', label: '중지됨' },
  { value: 'error', label: '오류' },
  { value: 'stored', label: '저장됨' },
  { value: 'loaded', label: '탑재됨' },
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

      {/* 상태 필터 드롭다운 */}
      <select
        value={statusFilter}
        onChange={(e) => onStatusFilterChange(e.target.value)}
        className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        {STATUS_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  );
}
