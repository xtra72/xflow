// 정렬 가능한 테이블 헤더 컴포넌트.
// 클릭 시 정렬 방향을 토글하며, 현재 정렬 필드에 방향 표시자를 보여준다.

/** 정렬 상태 */
export interface SortState {
  field: string;
  direction: 'asc' | 'desc';
}

interface SortableHeaderProps {
  /** 표시할 라벨 */
  label: string;
  /** 정렬 기준 필드명 */
  field: string;
  /** 현재 정렬 상태 */
  currentSort: SortState;
  /** 정렬 변경 콜백 */
  onSort: (field: string) => void;
  /** 추가 CSS 클래스 (페이지별 패딩 차이 등) */
  className?: string;
}

/**
 * 정렬 가능한 테이블 헤더.
 * 같은 필드 클릭 시 방향 토글, 다른 필드 클릭 시 asc 로 설정.
 */
export default function SortableHeader({
  label,
  field,
  currentSort,
  onSort,
  className = '',
}: SortableHeaderProps) {
  const isActive = currentSort.field === field;

  return (
    <th
      onClick={() => onSort(field)}
      className={`cursor-pointer select-none text-left text-xs font-medium uppercase tracking-wider transition-colors ${
        isActive
          ? 'text-gray-700 dark:text-gray-200'
          : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
      } ${className}`}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        <span
          className={`text-[10px] ${
            isActive ? 'text-blue-500 dark:text-blue-400' : 'text-gray-300 dark:text-gray-600'
          }`}
          aria-hidden="true"
        >
          {isActive ? (currentSort.direction === 'asc' ? '\u25B2' : '\u25BC') : '\u25B2'}
        </span>
      </span>
    </th>
  );
}
