// 에이전트 관리 페이지.
// 에이전트 목록을 테이블로 표시하며, 행 클릭으로 상세 패널을 토글한다.
// 생성 모달, 로딩/에러/빈 상태를 포함한다.

import { useMemo, useState } from 'react';
import { Bot, ChevronDown, ChevronRight, Plus } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useAgents } from '@/hooks/useAgent';
import type { AgentInfo } from '@/types/agent';

import AgentActionButtons from './AgentActionButtons';
import AgentDetailPanel from './AgentDetailPanel';
import AgentStatusBadge from './AgentStatusBadge';
import CreateAgentModal from './CreateAgentModal';

export default function AgentListPage() {
  const { data, isLoading, error, refetch } = useAgents();
  const [modalOpen, setModalOpen] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  const agents: AgentInfo[] = data?.data ?? [];

  // 클라이언트 측 정렬
  const sortedAgents = useMemo(() => {
    const sorted = [...agents];
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;

    sorted.sort((a, b) => {
      let va: string;
      let vb: string;

      switch (field) {
        case 'name':
          va = a.name.toLowerCase();
          vb = b.name.toLowerCase();
          break;
        case 'type':
          va = a.type.toLowerCase();
          vb = b.type.toLowerCase();
          break;
        case 'status':
          va = a.status.toLowerCase();
          vb = b.status.toLowerCase();
          break;
        default:
          return 0;
      }

      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });

    return sorted;
  }, [agents, sort]);

  /** 정렬 필드 변경 핸들러. 같은 필드 클릭 시 방향 토글, 다른 필드 시 asc. */
  const handleSort = (field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
  };

  /** 행 클릭 시 상세 패널 토글 */
  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">에이전트</h2>
        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-4 w-4" />
          새 에이전트
        </button>
      </div>

      {/* 로딩 스켈레톤 */}
      {isLoading && (
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
          <div className="divide-y divide-gray-200 dark:divide-gray-700">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-6 py-4">
                <div className="h-4 w-32 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="h-4 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
                <div className="ml-auto h-4 w-28 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 에러 상태 */}
      {error && !isLoading && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-600 dark:text-red-400">
            에이전트 목록을 불러오는데 실패했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700"
          >
            다시 시도
          </button>
        </div>
      )}

      {/* 빈 상태 */}
      {!isLoading && !error && agents.length === 0 && (
        <div className="rounded-lg border border-gray-200 bg-white p-12 text-center dark:border-gray-700 dark:bg-gray-800">
          <Bot className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <h3 className="mt-4 text-lg font-medium text-gray-900 dark:text-white">
            에이전트가 없습니다
          </h3>
          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
            새 에이전트를 만들어 데이터 수집을 시작하세요.
          </p>
          <button
            type="button"
            onClick={() => setModalOpen(true)}
            className="mt-4 inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            새 에이전트 만들기
          </button>
        </div>
      )}

      {/* 에이전트 테이블 */}
      {!isLoading && !error && agents.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="w-8 px-3 py-3" />
                <SortableHeader label="이름" field="name" currentSort={sort} onSort={handleSort} className="px-6 py-3" />
                <SortableHeader label="타입" field="type" currentSort={sort} onSort={handleSort} className="px-6 py-3" />
                <SortableHeader label="상태" field="status" currentSort={sort} onSort={handleSort} className="px-6 py-3" />
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  업타임
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  메시지 (IN/OUT)
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  액션
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-900">
              {sortedAgents.map((agent) => {
                const isExpanded = expandedId === agent.id;
                return (
                  <AgentRow
                    key={agent.id}
                    agent={agent}
                    isExpanded={isExpanded}
                    onToggle={() => toggleExpand(agent.id)}
                  />
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* 에이전트 생성 모달 */}
      <CreateAgentModal open={modalOpen} onClose={() => setModalOpen(false)} />
    </div>
  );
}

// ---- 에이전트 행 컴포넌트 ----

interface AgentRowProps {
  agent: AgentInfo;
  isExpanded: boolean;
  onToggle: () => void;
}

/** 에이전트 테이블 행 (확장 가능) */
function AgentRow({ agent, isExpanded, onToggle }: AgentRowProps) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-800"
      >
        {/* 확장 아이콘 */}
        <td className="px-3 py-4 text-gray-400">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>

        {/* 이름 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">
          {agent.name}
        </td>

        {/* 타입 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {agent.type}
        </td>

        {/* 상태 배지 */}
        <td className="whitespace-nowrap px-6 py-4">
          <AgentStatusBadge connected={agent.connected} status={agent.status} />
        </td>

        {/* 업타임 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {agent.uptime ?? '-'}
        </td>

        {/* 메시지 통계 */}
        <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
          {agent.stats
            ? `${agent.stats.messages_in.toLocaleString()} / ${agent.stats.messages_out.toLocaleString()}`
            : '-'}
        </td>

        {/* 액션 버튼 */}
        <td className="whitespace-nowrap px-6 py-4 text-right">
          <AgentActionButtons agent={agent} />
        </td>
      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={7} className="bg-gray-50 dark:bg-gray-800/50">
            <AgentDetailPanel agentId={agent.id} agentType={agent.type} />
          </td>
        </tr>
      )}
    </>
  );
}
