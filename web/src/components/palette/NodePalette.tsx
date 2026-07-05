// 노드 팔레트 패널 컴포넌트.
// 검색, 카테고리별 그룹핑, 드래그 앤 드롭 지원.

import { useMemo, useState } from 'react';
import { Loader2, Search } from 'lucide-react';

import { useNodeTypes } from '@/hooks/useNodeTypes';
import type { NodeTypeInfo } from '@/types/node';

import { NodeCategory } from './NodeCategory';

/** 노드 목록을 카테고리별로 그룹화 */
function groupByCategory(
  nodes: NodeTypeInfo[],
): Record<string, NodeTypeInfo[]> {
  const groups: Record<string, NodeTypeInfo[]> = {};
  for (const node of nodes) {
    const key = node.category;
    if (!groups[key]) {
      groups[key] = [];
    }
    groups[key].push(node);
  }
  return groups;
}

export function NodePalette() {
  const [search, setSearch] = useState('');
  const { data: nodeTypes, isLoading } = useNodeTypes();

  // 노드 타입 레지스트리 API(/nodes)는 옛 `_` HVAC 별칭을 deprecated 로 함께
  // 반환한다(source === 'builtin-deprecated'). 팔레트에는 canonical `-` 타입만
  // 노출하여 중복을 제거한다.
  const canonicalTypes = useMemo(
    () => (nodeTypes ?? []).filter((n) => n.source !== 'builtin-deprecated'),
    [nodeTypes],
  );

  // 검색어로 노드 타입 필터링 (type, description 모두 매칭)
  const filtered = useMemo(() => {
    if (!search.trim()) return canonicalTypes;

    const q = search.toLowerCase();
    return canonicalTypes.filter(
      (n) =>
        n.type.toLowerCase().includes(q) ||
        n.description.toLowerCase().includes(q),
    );
  }, [canonicalTypes, search]);

  // 카테고리별 그룹화
  const grouped = useMemo(() => groupByCategory(filtered), [filtered]);
  const categoryKeys = Object.keys(grouped);

  return (
    <aside
      className="flex w-60 shrink-0 flex-col border-r border-(--color-border-default)
        bg-(--color-bg-surface)"
    >
      {/* 검색 입력 */}
      <div className="relative border-b border-(--color-border-default) p-2">
        <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="노드 검색..."
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) py-1.5 pl-8 pr-2
            text-sm text-(--color-text-primary) placeholder:text-gray-400
            focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400
            dark:placeholder:text-gray-500 dark:focus:border-blue-500"
        />
      </div>

      {/* 노드 목록 */}
      <div className="flex-1 overflow-y-auto p-1">
        {isLoading && (
          <div className="flex items-center justify-center py-8 text-gray-400">
            <Loader2 className="h-5 w-5 animate-spin" />
          </div>
        )}

        {!isLoading && categoryKeys.length === 0 && (
          <p className="px-2 py-4 text-center text-xs text-gray-400">
            검색 결과가 없습니다
          </p>
        )}

        {categoryKeys.map((category, index) => (
          <NodeCategory
            key={category}
            category={category}
            nodes={grouped[category]!}
            defaultOpen={index === 0}
          />
        ))}
      </div>
    </aside>
  );
}
