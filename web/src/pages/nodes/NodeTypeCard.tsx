// 노드 타입 카드 컴포넌트.
// 개별 노드 타입의 정보를 카드 형태로 표시한다.

import { cn } from '@/lib/utils/cn';
import type { NodeTypeInfo } from '@/types/node';

// --- 카테고리 배지 색상 매핑 ---

/** 카테고리별 배지 스타일 (배경 + 텍스트) */
const CATEGORY_BADGE_STYLES: Record<string, string> = {
  input: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  output: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  process: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  bridge: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  special: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
};

// --- Props ---

interface NodeTypeCardProps {
  /** 노드 타입 정보 */
  node: NodeTypeInfo;
}

/**
 * 노드 타입 카드.
 * 타입 이름, 카테고리 배지, 설명, 소스를 카드 형태로 표시한다.
 */
export default function NodeTypeCard({ node }: NodeTypeCardProps) {
  const badgeStyle = CATEGORY_BADGE_STYLES[node.category] ?? CATEGORY_BADGE_STYLES.special;

  return (
    <div
      className={cn(
        'rounded-lg border border-gray-200 dark:border-gray-700',
        'bg-white dark:bg-gray-800 p-4',
        'hover:shadow-md transition-shadow',
      )}
    >
      {/* 타입 이름과 카테고리 배지 */}
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-bold text-gray-900 dark:text-white truncate">
          {node.type}
        </h3>
        <span
          className={cn(
            'inline-flex shrink-0 rounded-full px-2 py-0.5 text-xs font-medium',
            badgeStyle,
          )}
        >
          {node.category}
        </span>
      </div>

      {/* 설명 */}
      {node.description && (
        <p className="text-sm text-gray-600 dark:text-gray-400 mt-2 line-clamp-2">
          {node.description}
        </p>
      )}

      {/* 소스 */}
      {node.source && (
        <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
          {node.source}
        </p>
      )}
    </div>
  );
}
