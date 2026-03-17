// 노드 카테고리 탭 필터 컴포넌트.
// 카테고리별 노드 타입 필터링을 위한 탭 UI를 제공한다.

import { cn } from '@/lib/utils/cn';

// --- 카테고리 색상 매핑 ---

/** 카테고리별 활성 탭 스타일 (배경 + 텍스트) */
const CATEGORY_ACTIVE_STYLES: Record<string, string> = {
  input: 'bg-blue-500 text-white',
  output: 'bg-green-500 text-white',
  process: 'bg-purple-500 text-white',
  bridge: 'bg-orange-500 text-white',
  special: 'bg-gray-500 text-white',
};

// --- Props ---

interface NodeCategoryTabsProps {
  /** 현재 선택된 카테고리 ("전체" 또는 카테고리 이름) */
  selected: string;
  /** 카테고리 변경 콜백 */
  onChange: (category: string) => void;
  /** 필터링 가능한 카테고리 목록 */
  categories: string[];
}

/**
 * 노드 카테고리 탭 필터.
 * "전체" 탭과 카테고리별 탭을 렌더링하며
 * 선택된 탭은 카테고리 고유 색상으로 강조된다.
 */
export default function NodeCategoryTabs({
  selected,
  onChange,
  categories,
}: NodeCategoryTabsProps) {
  const tabs = ['전체', ...categories];

  return (
    <div className="flex flex-wrap gap-2">
      {tabs.map((tab) => {
        const isActive = selected === tab;
        // "전체" 탭은 기본 blue 스타일 사용
        const activeStyle =
          tab === '전체' ? 'bg-blue-500 text-white' : CATEGORY_ACTIVE_STYLES[tab] ?? 'bg-gray-500 text-white';

        return (
          <button
            key={tab}
            type="button"
            onClick={() => onChange(tab)}
            className={cn(
              'rounded-full px-4 py-1.5 text-sm font-medium cursor-pointer transition-colors',
              isActive
                ? activeStyle
                : 'bg-(--color-bg-elevated) text-(--color-text-muted) hover:bg-gray-200 dark:hover:bg-gray-600',
            )}
          >
            {tab === '전체' ? tab : tab}
          </button>
        );
      })}
    </div>
  );
}
