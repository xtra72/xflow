// 플로우 목록 페이지.
// 등록된 플로우의 CRUD 및 lifecycle 관리 기능을 제공한다.
// 검색, 상태 필터, 페이지네이션을 지원한다.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ChevronDown, ChevronLeft, ChevronRight, Download, Plus, Upload, Workflow } from 'lucide-react';

import ImportDialog from '@/components/common/ImportDialog';
import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useFlows } from '@/hooks';
import { downloadJSON } from '@/lib/utils/download';
import { exportAllFlows } from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';
import CreateFlowModal from '@/pages/dashboard/CreateFlowModal';

import FlowActionMenu from './FlowActionMenu';
import FlowDetailPanel from './FlowDetailPanel';
import FlowSearchFilter from './FlowSearchFilter';
import FlowStatusBadge from './FlowStatusBadge';

/** 페이지 크기 옵션 */
const PAGE_SIZE_OPTIONS = [10, 20, 50];

/**
 * 플로우 목록 페이지.
 * 테이블 형태로 플로우를 표시하며 검색, 필터, 페이지네이션을 지원한다.
 */
export default function FlowListPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data: flowsData, isLoading, error, refetch } = useFlows();

  // 모달 상태
  const [modalOpen, setModalOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);

  /** 전체 내보내기 핸들러 */
  const handleExportAll = async () => {
    try {
      const data = await exportAllFlows();
      downloadJSON(data, 'flows.json');
    } catch {
      // 내보내기 실패 시 무시
    }
  };

  /** 가져오기 성공 핸들러 */
  const handleImportSuccess = () => {
    queryClient.invalidateQueries({ queryKey: ['flows'] });
  };

  // 검색 및 필터 상태
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');

  // 정렬 상태
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 확장 상태
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // 페이지네이션 상태
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const allFlows: FlowInfo[] = flowsData?.data ?? [];

  // 클라이언트 측 필터링
  const filteredFlows = useMemo(() => {
    let result = allFlows;

    // 이름 검색
    if (search.trim()) {
      const query = search.trim().toLowerCase();
      result = result.filter((f) => f.name.toLowerCase().includes(query));
    }

    // 상태 필터
    if (statusFilter) {
      result = result.filter((f) => f.status === statusFilter);
    }

    return result;
  }, [allFlows, search, statusFilter]);

  // 클라이언트 측 정렬
  const sortedFlows = useMemo(() => {
    const sorted = [...filteredFlows];
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;

    sorted.sort((a, b) => {
      let va: string | number;
      let vb: string | number;

      switch (field) {
        case 'name':
          va = a.name.toLowerCase();
          vb = b.name.toLowerCase();
          break;
        case 'status':
          va = a.status.toLowerCase();
          vb = b.status.toLowerCase();
          break;
        case 'created_at':
          va = a.created_at ?? '';
          vb = b.created_at ?? '';
          break;
        case 'updated_at':
          va = a.updated_at ?? '';
          vb = b.updated_at ?? '';
          break;
        default:
          return 0;
      }

      if (va < vb) return -1 * mul;
      if (va > vb) return 1 * mul;
      return 0;
    });

    return sorted;
  }, [filteredFlows, sort]);

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
  const totalItems = sortedFlows.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedFlows = sortedFlows.slice(startIndex, startIndex + pageSize);

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

  /** 날짜 포맷 (간략) */
  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '-';
    try {
      return new Date(dateStr).toLocaleDateString('ko-KR', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
      });
    } catch {
      return '-';
    }
  };

  // --- 로딩 상태 ---
  if (isLoading && !flowsData) {
    return (
      <div className="space-y-6">
        {/* 헤더 스켈레톤 */}
        <div className="flex items-center justify-between">
          <div className="h-8 w-24 animate-pulse rounded bg-(--color-bg-elevated)" />
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
        <h2 className="text-2xl font-bold text-(--color-text-primary)">플로우</h2>
        <div className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm text-red-700 dark:text-red-400">
            플로우 목록을 불러오는 중 오류가 발생했습니다.
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
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-(--color-text-primary)">플로우</h2>
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
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" />
            새 플로우
          </button>
        </div>
      </div>

      {/* 검색 및 필터 */}
      <FlowSearchFilter
        search={search}
        onSearchChange={handleSearchChange}
        statusFilter={statusFilter}
        onStatusFilterChange={handleStatusFilterChange}
      />

      {/* 테이블 또는 빈 상태 */}
      {filteredFlows.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center">
          <Workflow className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {allFlows.length === 0
              ? '등록된 플로우가 없습니다. 새 플로우를 만들어 보세요.'
              : '검색 결과가 없습니다.'}
          </p>
          {allFlows.length === 0 && (
            <button
              type="button"
              onClick={() => setModalOpen(true)}
              className="mt-4 inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              <Plus className="h-4 w-4" />
              새 플로우
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

          {/* 플로우 테이블 */}
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="min-w-full divide-y divide-(--color-border-default)">
              <thead className="bg-(--color-bg-primary)">
                <tr>
                  <th className="w-8 px-3 py-3" />
                  <SortableHeader label="이름" field="name" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="상태" field="status" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    노드
                  </th>
                  <SortableHeader label="생성일" field="created_at" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <SortableHeader label="수정일" field="updated_at" currentSort={sort} onSort={handleSort} className="px-4 py-3" />
                  <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
                    액션
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
                {pagedFlows.map((flow) => {
                  const isExpanded = expandedId === flow.id;
                  return (
                    <FlowRow
                      key={flow.id}
                      flow={flow}
                      isExpanded={isExpanded}
                      onToggle={() => setExpandedId((prev) => (prev === flow.id ? null : flow.id))}
                      onNavigate={() => navigate(`/editor/${flow.id}`)}
                      formatDate={formatDate}
                    />
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {/* 플로우 생성 모달 */}
      <CreateFlowModal open={modalOpen} onClose={() => setModalOpen(false)} />

      {/* 가져오기 대화 상자 */}
      <ImportDialog
        open={importDialogOpen}
        onClose={() => setImportDialogOpen(false)}
        type="flow"
        onImportSuccess={handleImportSuccess}
      />
    </div>
  );
}

// ---- 플로우 행 컴포넌트 ----

interface FlowRowProps {
  flow: FlowInfo;
  isExpanded: boolean;
  onToggle: () => void;
  onNavigate: () => void;
  formatDate: (dateStr?: string) => string;
}

/** 플로우 테이블 행 (확장 가능) */
function FlowRow({ flow, isExpanded, onToggle, onNavigate, formatDate }: FlowRowProps) {
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

        <td className="whitespace-nowrap px-4 py-3">
          <div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onNavigate();
              }}
              className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
            >
              {flow.name}
            </button>
            {flow.description && (
              <p className="mt-0.5 text-xs text-(--color-text-muted)">
                {flow.description}
              </p>
            )}
          </div>
        </td>
        <td className="whitespace-nowrap px-4 py-3">
          <FlowStatusBadge status={flow.status} />
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {flow.node_count}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatDate(flow.created_at)}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
          {formatDate(flow.updated_at)}
        </td>
        <td className="whitespace-nowrap px-4 py-3 text-right">
          <FlowActionMenu flow={flow} />
        </td>
      </tr>

      {/* 확장된 상세 패널 */}
      {isExpanded && (
        <tr>
          <td colSpan={7} className="bg-gray-50 dark:bg-gray-800/50">
            <FlowDetailPanel flowId={flow.id} />
          </td>
        </tr>
      )}
    </>
  );
}
