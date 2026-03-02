// 노드 타입 브라우저 페이지.
// 등록된 노드 타입을 카드 그리드로 표시하며 검색 및 카테고리 필터링을 지원한다.

import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';

import { useNodeTypes } from '@/hooks/useNodeTypes';
import { cn } from '@/lib/utils/cn';
import NodeCategoryTabs from '@/pages/nodes/NodeCategoryTabs';
import NodeTypeCard from '@/pages/nodes/NodeTypeCard';

/**
 * 노드 타입 브라우저 페이지.
 * 검색, 카테고리 탭 필터, 카드 그리드로 구성된다.
 */
export default function NodeTypesPage() {
  const { data: nodeTypes, isLoading, error, refetch } = useNodeTypes();
  const [search, setSearch] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('전체');

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
      if (selectedCategory !== '전체' && node.category !== selectedCategory) {
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
  }, [nodeTypes, search, selectedCategory]);

  return (
    <div className="space-y-6">
      {/* 페이지 헤더 */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">
          노드 타입
        </h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          등록된 노드 타입을 탐색합니다
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
          placeholder="노드 검색..."
          className={cn(
            'block w-full rounded-md border border-gray-300 py-2 pl-10 pr-3 text-sm',
            'placeholder:text-gray-400 dark:placeholder:text-gray-500',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'dark:border-gray-600 dark:bg-gray-700 dark:text-white',
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
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-6 text-center">
          <p className="text-sm text-red-600 dark:text-red-400">
            노드 타입을 불러오는 데 실패했습니다
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className={cn(
              'mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white',
              'hover:bg-red-700 transition-colors',
            )}
          >
            다시 시도
          </button>
        </div>
      )}

      {/* 결과 그리드 */}
      {!isLoading && !error && filteredNodes.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredNodes.map((node) => (
            <NodeTypeCard key={node.type} node={node} />
          ))}
        </div>
      )}

      {/* 빈 상태 */}
      {!isLoading && !error && nodeTypes && filteredNodes.length === 0 && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-12 text-center">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            일치하는 노드 타입이 없습니다
          </p>
        </div>
      )}
    </div>
  );
}

// --- 로딩 스켈레톤 ---

/** 카드 그리드 로딩 스켈레톤. 6개의 애니메이션 카드를 표시한다. */
function LoadingSkeleton() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {Array.from({ length: 6 }).map((_, i) => (
        <div
          key={i}
          className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 animate-pulse"
        >
          <div className="flex items-center justify-between gap-2">
            <div className="h-4 w-32 rounded bg-gray-200 dark:bg-gray-700" />
            <div className="h-5 w-16 rounded-full bg-gray-200 dark:bg-gray-700" />
          </div>
          <div className="mt-3 space-y-2">
            <div className="h-3 w-full rounded bg-gray-200 dark:bg-gray-700" />
            <div className="h-3 w-2/3 rounded bg-gray-200 dark:bg-gray-700" />
          </div>
          <div className="mt-2 h-3 w-20 rounded bg-gray-100 dark:bg-gray-700" />
        </div>
      ))}
    </div>
  );
}
