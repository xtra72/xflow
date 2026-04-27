// 에이전트 관리 페이지.
// 에이전트 목록을 테이블로 표시하며, 행 클릭으로 상세 패널을 토글한다.
// 검색, 상태 필터, 페이지네이션을 지원한다.

import { useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  Bot,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleStop,
  Download,
  Pencil,
  Plus,
  Upload,
  X,
} from 'lucide-react';

import ImportDialog from '@/components/common/ImportDialog';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useAgents, useUpdateAgent } from '@/hooks/useAgent';
import { downloadJSON } from '@/lib/utils/download';
import { exportAllAgents } from '@/services/api/agentService';
import { useUIStore } from '@/stores/uiStore';
import type { AgentInfo } from '@/types/agent';

import AgentActionButtons from './AgentActionButtons';
import AgentDetailPanel from './AgentDetailPanel';
import AgentEnabledBadge from './AgentEnabledBadge';
import AgentSearchFilter from './AgentSearchFilter';
import CreateAgentModal from './CreateAgentModal';

/** 페이지 크기 옵션 */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

export default function AgentListPage() {
  const queryClient = useQueryClient();
  const refreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;
  const { data, isLoading, error, refetch } = useAgents(undefined, refreshMs);
  const [modalOpen, setModalOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  /** 전체 내보내기 핸들러 */
  const handleExportAll = async () => {
    try {
      const data = await exportAllAgents();
      downloadJSON(data, 'agents.json');
    } catch {
      // 내보내기 실패 시 무시
    }
  };

  /** 가져오기 성공 핸들러 */
  const handleImportSuccess = () => {
    queryClient.invalidateQueries({ queryKey: ['agents'] });
  };

  // 검색 및 필터 상태
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 페이지네이션 상태
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const allAgents: AgentInfo[] = data?.data ?? [];

  /** 에이전트의 표시 상태를 결정한다 (connected/disconnected/error). */
  const getDisplayStatus = (agent: AgentInfo): string => {
    if (agent.status === 'error') return 'error';
    return agent.connected === true ? 'connected' : 'disconnected';
  };

  // 클라이언트 측 필터링
  const filteredAgents = useMemo(() => {
    let result = allAgents;

    // 이름 검색
    if (search.trim()) {
      const query = search.trim().toLowerCase();
      result = result.filter(
        (a) =>
          a.name.toLowerCase().includes(query) ||
          a.type.toLowerCase().includes(query),
      );
    }

    // 상태 필터
    if (statusFilter) {
      result = result.filter((a) => getDisplayStatus(a) === statusFilter);
    }

    return result;
  }, [allAgents, search, statusFilter]);

  // 클라이언트 측 정렬
  const sortedAgents = useMemo(() => {
    const sorted = [...filteredAgents];
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
          va = getDisplayStatus(a);
          vb = getDisplayStatus(b);
          break;
        default:
          return 0;
      }

      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });

    return sorted;
  }, [filteredAgents, sort]);

  /** 정렬 필드 변경 핸들러. 같은 필드 클릭 시 방향 토글, 다른 필드 시 asc. */
  const handleSort = (field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
    setPage(1);
  };

  // 페이지네이션 계산
  const totalItems = sortedAgents.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedAgents = sortedAgents.slice(startIndex, startIndex + pageSize);

  // 필터 변경 시 페이지 초기화
  const handleSearchChange = (v: string) => {
    setSearch(v);
    setPage(1);
  };
  const handleStatusFilterChange = (v: string) => {
    setStatusFilter(v);
    setPage(1);
  };
  const handlePageSizeChange = (size: number) => {
    setPageSize(size);
    setPage(1);
  };

  /** 행 클릭 시 상세 패널 토글 */
  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  // --- 로딩 상태 ---
  if (isLoading && !data) {
    return (
      <div className="space-y-6">
        {/* 액션 버튼 스켈레톤 */}
        <div className="flex items-center justify-end">
          <div className="h-10 w-28 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
        {/* 필터 스켈레톤 */}
        <div className="flex items-center gap-3">
          <div className="h-10 flex-1 animate-pulse rounded bg-(--color-bg-elevated)" />
          <div className="h-10 w-36 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
        {/* 테이블 스켈레톤 */}
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div
              key={i}
              className="h-14 animate-pulse rounded bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      </div>
    );
  }

  // --- 에러 상태 ---
  if (error) {
    return (
      <div className="space-y-6">
        <div className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-700 dark:text-red-400">
            에이전트 목록을 불러오는 중 오류가 발생했습니다.
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            다시 시도
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 액션 버튼 */}
      <div className="flex items-center justify-end">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setImportDialogOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) px-3 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Upload className="h-4 w-4" />
            가져오기
          </button>
          <button
            type="button"
            onClick={handleExportAll}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) px-3 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Download className="h-4 w-4" />
            전체 내보내기
          </button>
          <button
            type="button"
            onClick={() => setModalOpen(true)}
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            새 에이전트
          </button>
        </div>
      </div>

      {/* 검색 및 필터 */}
      <AgentSearchFilter
        search={search}
        onSearchChange={handleSearchChange}
        statusFilter={statusFilter}
        onStatusFilterChange={handleStatusFilterChange}
      />

      {/* 테이블 또는 빈 상태 */}
      {filteredAgents.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center">
          <Bot className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {allAgents.length === 0
              ? '등록된 에이전트가 없습니다. 새 에이전트를 만들어 보세요.'
              : '검색 결과가 없습니다.'}
          </p>
          {allAgents.length === 0 && (
            <button
              type="button"
              onClick={() => setModalOpen(true)}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-4 w-4" />
              새 에이전트
            </button>
          )}
        </div>
      ) : (
        <>
          {/* 페이지네이션 */}
          <div className="flex items-center justify-between">
            {/* 페이지 크기 선택 */}
            <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
              <span>페이지당</span>
              <select
                value={pageSize}
                onChange={(e) => handlePageSizeChange(Number(e.target.value))}
                className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                {PAGE_SIZE_OPTIONS.map((size) => (
                  <option key={size} value={size}>
                    {size}
                  </option>
                ))}
              </select>
              <span>건</span>
              <span className="ml-2 text-gray-400">|</span>
              <span className="ml-2">
                총 {totalItems}건 중 {startIndex + 1}-
                {Math.min(startIndex + pageSize, totalItems)}건
              </span>
            </div>

            {/* 페이지 이동 버튼 */}
            <div className="flex items-center gap-1">
              <button
                type="button"
                disabled={safePage <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="이전 페이지"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <span className="px-3 text-sm text-(--color-text-muted)">
                {safePage} / {totalPages}
              </span>
              <button
                type="button"
                disabled={safePage >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="다음 페이지"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          </div>

          {/* 에이전트 테이블 */}
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-primary)">
                <tr>
                  <th className="w-8 px-3 py-3" />
                  <SortableHeader label="이름" field="name" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="타입" field="type" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="상태" field="status" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    업타임
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    메시지 (IN/OUT)
                  </th>
                  <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    액션
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
                {pagedAgents.map((agent) => {
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
        </>
      )}

      {/* 에이전트 생성 모달 */}
      <CreateAgentModal open={modalOpen} onClose={() => setModalOpen(false)} />

      {/* 가져오기 대화 상자 */}
      <ImportDialog
        open={importDialogOpen}
        onClose={() => setImportDialogOpen(false)}
        type="agent"
        onImportSuccess={handleImportSuccess}
      />
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
  const [editingName, setEditingName] = useState(false);
  const [nameValue, setNameValue] = useState(agent.name);
  const updateAgent = useUpdateAgent();

  // 이름 저장 핸들러
  const handleSaveName = (e: React.MouseEvent | React.KeyboardEvent) => {
    e.stopPropagation();
    const trimmed = nameValue.trim();
    if (trimmed && trimmed !== agent.name) {
      updateAgent.mutate(
        { id: agent.id, req: { name: trimmed } },
        { onSuccess: () => setEditingName(false) },
      );
    } else {
      setEditingName(false);
      setNameValue(agent.name);
    }
  };

  // 이름 편집 취소 핸들러
  const handleCancelName = (e: React.MouseEvent | React.KeyboardEvent) => {
    e.stopPropagation();
    setEditingName(false);
    setNameValue(agent.name);
  };

  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer transition-colors hover:bg-(--color-bg-elevated)"
      >
        {/* 확장 아이콘 */}
        <td className="px-3 py-3 text-gray-400">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </td>

        {/* 이름 (인라인 편집 가능) */}
        <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-(--color-text-primary)">
          {editingName ? (
            <span className="inline-flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
              <input
                type="text"
                value={nameValue}
                onChange={(e) => setNameValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleSaveName(e);
                  if (e.key === 'Escape') handleCancelName(e);
                }}
                className="rounded border border-(--color-border) bg-(--color-bg-base) px-2 py-0.5 text-sm focus:outline-none focus:ring-1 focus:ring-(--color-primary)"
                autoFocus
              />
              <button
                onClick={handleSaveName}
                className="rounded p-0.5 text-green-600 hover:bg-green-100 dark:hover:bg-green-900/30"
                title="저장"
              >
                <Check className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={handleCancelName}
                className="rounded p-0.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700"
                title="취소"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </span>
          ) : (
            <span className="group inline-flex items-center gap-1.5">
              {agent.name}
              <AgentEnabledBadge enabled={agent.enabled} />
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setNameValue(agent.name);
                  setEditingName(true);
                }}
                className="rounded p-0.5 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-(--color-bg-elevated)"
                title="이름 편집"
              >
                <Pencil className="h-3 w-3 text-(--color-text-muted)" />
              </button>
            </span>
          )}
        </td>

        {/* 타입 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {agent.type}
        </td>

        {/* 상태 아이콘 */}
        <td className="whitespace-nowrap px-4 py-3">
          {(() => {
            if (agent.status === 'error') {
              return (
                <span className="inline-flex items-center text-red-600 dark:text-red-400" title="오류">
                  <AlertTriangle className="h-4 w-4" />
                </span>
              );
            }
            return agent.connected === true ? (
              <span className="inline-flex items-center text-green-600 dark:text-green-400" title="연결됨">
                <Activity className="h-4 w-4" />
              </span>
            ) : (
              <span className="inline-flex items-center text-gray-400 dark:text-gray-500" title="연결 해제">
                <CircleStop className="h-4 w-4" />
              </span>
            );
          })()}
        </td>

        {/* 업타임 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {agent.uptime ?? '-'}
        </td>

        {/* 메시지 통계 */}
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {agent.stats
            ? `${agent.stats.messages_in.toLocaleString()} / ${agent.stats.messages_out.toLocaleString()}`
            : '-'}
        </td>

        {/* 액션 버튼 */}
        <td className="whitespace-nowrap px-4 py-3 text-right">
          <AgentActionButtons agent={agent} />
        </td>
      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={7} className="bg-(--color-bg-sunken)">
            <AgentDetailPanel
              agentId={agent.id}
              agentType={agent.type}
              agentName={agent.name}
            />
          </td>
        </tr>
      )}
    </>
  );
}
