// 원격 관리 로그 페이지 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 사이드바 `원격 관리` 하위 진입점이다. 모든 노드의 사건을 한 시간축에서 본다.
// 노드 하나만 보려면 여기서 고르거나, 노드 화면의 로그 탭을 쓴다(같은 표를 공유).
//
// 정렬·필터·쪽 나누기는 서버가 맡는다. 받아 온 쪽 안에서만 정렬하면 "수행자
// 오름차순" 같은 결과가 전체가 아니라 그 쪽에만 적용되어 읽는 사람을 속인다.
//
// 권한/모드: admin 전용 라우트 + server 모드에서만 조회한다(비-server 는 안내).

import { useEffect, useMemo, useState } from 'react';
import { Hash, RotateCw, Tag } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';

import RemoteLogTable from '@/components/remote/RemoteLogTable';
import { REMOTE_LOG_ACTIONS } from '@/components/remote/remoteLogAction';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import TablePagination from '@/pages/agents/TablePagination';
import { useManagedNodes, useRemoteMode } from '@/hooks/useRemote';
import {
  REMOTE_LOG_PAGE_SIZE,
  REMOTE_LOG_PAGE_SIZE_OPTIONS,
  useRemoteLogs,
} from '@/hooks/useRemoteLogs';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { RemoteLogSortField } from '@/types/remote';
import type { SortState } from '@/components/common/SortableHeader';

/** 이름/id 표시 선택을 기억하는 키. */
const NODE_ID_VIEW_KEY = 'xflow_remote_log_node_ids';

const controlClass = cn(
  'rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm',
  'text-(--color-text-primary) focus:outline-none focus:ring-2 focus:ring-blue-500',
);

export default function RemoteLogPage(): React.JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  const [instanceId, setInstanceId] = useState('');
  const [action, setAction] = useState('');
  const [actor, setActor] = useState('');
  const [sort, setSort] = useState<SortState>({ field: 'ts', direction: 'desc' });
  const [pageSize, setPageSize] = useState(REMOTE_LOG_PAGE_SIZE);
  // 노드를 이름으로 볼지 id 로 볼지. 브라우저에 기억한다 — id 로 대조하는 일이 잦은
  // 사람이 화면을 열 때마다 다시 누르지 않아도 되게.
  const [showNodeIds, setShowNodeIds] = useState(() => {
    try {
      return localStorage.getItem(NODE_ID_VIEW_KEY) === '1';
    } catch {
      return false;
    }
  });
  const [page, setPage] = useState(1); // 1-base — TablePagination 규약.

  const { data: nodes } = useManagedNodes(undefined, isServer);
  const {
    data,
    isLoading,
    isError,
    isFetching,
  } = useRemoteLogs(
    {
      instanceId,
      action,
      actor,
      sort: sort.field as RemoteLogSortField,
      asc: sort.direction === 'asc',
      limit: pageSize,
      offset: (page - 1) * pageSize,
    },
    isServer,
  );

  // `?? []` 는 매 렌더 새 배열을 만들어, 이 값을 의존성으로 쓰는 useMemo 가 헛돈다.
  // 조회 결과가 실제로 바뀔 때만 새 참조가 되게 한다.
  const entries = useMemo(() => data?.entries ?? [], [data?.entries]);
  const total = data?.total ?? 0;

  // 조건이 바뀌면 첫 쪽으로 — 3쪽을 보다 필터를 걸면 빈 화면에 서게 된다.
  useEffect(() => {
    setPage(1);
  }, [instanceId, action, actor, pageSize, sort.field, sort.direction]);

  // instance_id → 이름. 삭제된 노드의 과거 기록은 이름을 모르므로 id 로 남는다.
  const nodeNames = useMemo(() => {
    const map = new Map<string, string>();
    for (const n of nodes ?? []) {
      if (n.hostname) map.set(n.instance_id, n.hostname);
    }
    return map;
  }, [nodes]);

  // 수행 주체 후보 — 현재 쪽에서 모은다. 서버가 목록을 주지 않으므로 보이는
  // 범위에서 만든 것이고, 고른 값은 서버 필터로 내려간다.
  const actorOptions = useMemo(() => {
    const set = new Set<string>(entries.map((e) => e.actor).filter(Boolean));
    if (actor) set.add(actor); // 필터 중인 값이 이번 쪽에 없어도 선택을 유지한다.
    return Array.from(set).sort();
  }, [entries, actor]);

  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6" data-testid="remote-log-page">
        <RemoteNotServerNotice />
      </div>
    );
  }

  const handleSort = (field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : // 다른 칸으로 옮기면 시각은 최신순, 그 밖은 오름차순이 자연스럽다.
          { field, direction: field === 'ts' ? 'desc' : 'asc' },
    );
  };

  return (
    <div className="space-y-4" data-testid="remote-log-page">
      <div className="flex flex-wrap items-center gap-2">
        <select
          className={controlClass}
          value={instanceId}
          onChange={(e) => setInstanceId(e.target.value)}
          aria-label={t('remote.log.col.node')}
          data-testid="remote-log-node-filter"
        >
          <option value="">{t('remote.log.allNodes')}</option>
          {(nodes ?? []).map((n) => (
            <option key={n.instance_id} value={n.instance_id}>
              {n.hostname || n.instance_id}
            </option>
          ))}
        </select>

        <select
          className={controlClass}
          value={action}
          onChange={(e) => setAction(e.target.value)}
          aria-label={t('remote.log.col.action')}
          data-testid="remote-log-action-filter"
        >
          <option value="">{t('remote.log.allActions')}</option>
          {REMOTE_LOG_ACTIONS.map((a) => (
            <option key={a} value={a}>
              {t(`remote.log.action.${a}`)}
            </option>
          ))}
        </select>

        <select
          className={controlClass}
          value={actor}
          onChange={(e) => setActor(e.target.value)}
          aria-label={t('remote.log.col.actor')}
          data-testid="remote-log-actor-filter"
        >
          <option value="">{t('remote.log.allActors')}</option>
          {actorOptions.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>

        <button
          type="button"
          onClick={() => {
            setShowNodeIds((prev) => {
              const next = !prev;
              try {
                localStorage.setItem(NODE_ID_VIEW_KEY, next ? '1' : '0');
              } catch {
                // 저장 실패는 이번 화면에만 적용되는 것으로 족하다.
              }
              return next;
            });
          }}
          className={cn(controlClass, 'inline-flex items-center gap-1.5')}
          data-testid="remote-log-node-view-toggle"
          aria-pressed={showNodeIds}
        >
          {showNodeIds ? (
            <Hash className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Tag className="h-4 w-4" aria-hidden="true" />
          )}
          {showNodeIds ? t('remote.log.showNames') : t('remote.log.showIds')}
        </button>

        <button
          type="button"
          onClick={() => void qc.invalidateQueries({ queryKey: ['remote', 'logs'] })}
          className={cn(controlClass, 'inline-flex items-center gap-1.5')}
          data-testid="remote-log-refresh"
        >
          <RotateCw className={cn('h-4 w-4', isFetching && 'animate-spin')} aria-hidden="true" />
          {t('common.refresh')}
        </button>
      </div>

      <div className="rounded-lg bg-(--color-bg-surface) shadow">
        <RemoteLogTable
          entries={entries}
          isLoading={isLoading}
          isError={isError}
          nodeNames={nodeNames}
          showNodeIds={showNodeIds}
          sort={sort}
          onSort={handleSort}
        />
      </div>

      <TablePagination
        page={page}
        pageSize={pageSize}
        totalItems={total}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        pageSizeOptions={REMOTE_LOG_PAGE_SIZE_OPTIONS}
      />
    </div>
  );
}
