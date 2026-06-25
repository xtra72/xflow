// 노드 타입 브라우저 페이지.
// 등록된 노드 타입을 카드 그리드로 표시하며 검색 및 카테고리 필터링을 지원한다.
// 카드 클릭 시 상세 패널(포트, 설정, 예제, 인스턴스)이 확장된다.

import { Fragment, useMemo, useState } from 'react';
import { Search } from 'lucide-react';

import { useNodeTypes } from '@/hooks/useNodeTypes';
import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import NodeCategoryTabs from '@/pages/nodes/NodeCategoryTabs';
import NodeTypeCard from '@/pages/nodes/NodeTypeCard';
import NodeTypeDetailPanel from '@/pages/nodes/NodeTypeDetailPanel';

/** 그리드 열 수 (lg 기준) */
const GRID_COLS = 3;

/**
 * 노드 타입 브라우저 페이지.
 * 검색, 카테고리 탭 필터, 카드 그리드로 구성된다.
 */
export default function NodeTypesPage() {
  const { t } = useTranslation();
  const { data: nodeTypes, isLoading, error, refetch } = useNodeTypes();
  const [search, setSearch] = useState('');
  // 'allCategories' 번역값을 초기 선택 상태 및 필터 비교에 사용한다.
  const ALL_CATEGORIES = t('nodes.allCategories');
  const [selectedCategory, setSelectedCategory] = useState(ALL_CATEGORIES);
  const [expandedType, setExpandedType] = useState<string | null>(null);

  // 고유 카테고리 목록 추출
  const categories = useMemo(() => {
    if (!nodeTypes) return [];
    const unique = [...new Set(nodeTypes.map((n) => n.category))];
    return unique.sort();
  }, [nodeTypes]);

  // 검색어 및 카테고리로 필터링
  const filteredNodes = useMemo(() => {
    if (!nodeTypes) return [];
    const query = search.toLowerCase().trim();

    return nodeTypes.filter((node) => {
      // 카테고리 필터
      if (selectedCategory !== ALL_CATEGORIES && node.category !== selectedCategory) {
        return false;
      }
      // 검색어 필터 (type, description 대상)
      if (query) {
        return (
          node.type.toLowerCase().includes(query) ||
          node.description.toLowerCase().includes(query)
        );
      }
      return true;
    });
  }, [nodeTypes, search, selectedCategory, ALL_CATEGORIES]);

  /** 카드 클릭 시 상세 패널 토글 */
  const toggleExpand = (type: string) => {
    setExpandedType((prev) => (prev === type ? null : type));
  };

  return (
    <div className="space-y-6">
      {/* 페이지 헤더 */}
      <div>
        <h2 className="text-2xl font-bold text-(--color-text-primary)">
          {t('nodes.title')}
        </h2>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('nodes.subtitle')}
        </p>
      </div>

      {/* 검색 입력 */}
      <div className="relative max-w-md">
        <Search
          className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400 dark:text-gray-500"
          aria-hidden="true"
        />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('nodes.search')}
          className={cn(
            'block w-full rounded-md border border-(--color-border-strong) py-2 pl-10 pr-3 text-sm',
            'placeholder:text-gray-400',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'bg-(--color-bg-surface) text-(--color-text-primary)',
          )}
        />
      </div>

      {/* 카테고리 탭 */}
      {!isLoading && !error && categories.length > 0 && (
        <NodeCategoryTabs
          selected={selectedCategory}
          onChange={setSelectedCategory}
          categories={categories}
        />
      )}

      {/* 로딩 상태 */}
      {isLoading && <LoadingSkeleton />}

      {/* 에러 상태 */}
      {error && (
        <div className="rounded-lg border border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/20 p-6 text-center">
          <p className="text-sm text-red-600 dark:text-red-400">
            {t('nodes.loadError')}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className={cn(
              'mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white',
              'hover:bg-red-700 transition-colors',
            )}
          >
            {t('common.retry')}
          </button>
        </div>
      )}

      {/* 결과 그리드 + 확장 상세 패널 */}
      {!isLoading && !error && filteredNodes.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredNodes.map((node, index) => {
            const isExpanded = expandedType === node.type;
            // 행의 마지막 카드 뒤에 상세 패널 삽입 (3열 그리드 기준)
            const isRowEnd =
              (index + 1) % GRID_COLS === 0 || index === filteredNodes.length - 1;
            const showPanel = isRowEnd && expandedType && isExpandedInRow(filteredNodes, expandedType, index);

            return (
              <Fragment key={node.type}>
                <NodeTypeCard
                  node={node}
                  isExpanded={isExpanded}
                  onToggle={() => toggleExpand(node.type)}
                />
                {showPanel && expandedType && (
                  <div className="col-span-1 md:col-span-2 lg:col-span-3">
                    <NodeTypeDetailPanel nodeType={expandedType} />
                  </div>
                )}
              </Fragment>
            );
          })}
        </div>
      )}

      {/* 빈 상태 */}
      {!isLoading && !error && nodeTypes && filteredNodes.length === 0 && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-12 text-center">
          <p className="text-sm text-(--color-text-muted)">
            {t('nodes.noMatch')}
          </p>
        </div>
      )}
    </div>
  );
}

/**
 * 확장된 노드가 현재 인덱스와 같은 행에 있는지 확인.
 * 행의 마지막 카드 뒤에만 패널을 삽입하기 위해 사용한다.
 */
function isExpandedInRow(
  nodes: { type: string }[],
  expandedType: string,
  currentIndex: number,
): boolean {
  const rowStart = currentIndex - (currentIndex % GRID_COLS);
  const rowEnd = Math.min(rowStart + GRID_COLS - 1, nodes.length - 1);
  for (let i = rowStart; i <= rowEnd; i++) {
    if (nodes[i]!.type === expandedType) return true;
  }
  return false;
}

// --- 로딩 스켈레톤 ---

/** 카드 그리드 로딩 스켈레톤. 6개의 애니메이션 카드를 표시한다. */
function LoadingSkeleton() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {Array.from({ length: 6 }).map((_, i) => (
        <div
          key={i}
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4 animate-pulse"
        >
          <div className="flex items-center justify-between gap-2">
            <div className="h-4 w-32 rounded bg-(--color-bg-elevated)" />
            <div className="h-5 w-16 rounded-full bg-(--color-bg-elevated)" />
          </div>
          <div className="mt-3 space-y-2">
            <div className="h-3 w-full rounded bg-(--color-bg-elevated)" />
            <div className="h-3 w-2/3 rounded bg-(--color-bg-elevated)" />
          </div>
          <div className="mt-2 h-3 w-20 rounded bg-(--color-bg-elevated)" />
        </div>
      ))}
    </div>
  );
}
