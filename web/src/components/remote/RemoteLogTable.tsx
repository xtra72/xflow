// 원격 관리 로그 표 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 전체 로그 페이지와 노드 화면의 로그 탭이 이 표를 공유한다. 노드 칸은 전체 화면에서만
// 보이면 되므로 `showNode` 로 가른다 — 한 노드만 보는 자리에서 같은 값이 모든 줄에
// 반복되면 읽을 것이 줄어든다.
//
// 정렬은 이 표가 하지 않는다. 헤더를 누르면 부모에게 알리고, 부모가 서버에 다시
// 묻는다 — 받아 온 쪽 안에서만 정렬하면 그 결과가 전체를 대표하지 않기 때문이다.

import { AlertTriangle, Loader2 } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { logActionKey } from '@/components/remote/remoteLogAction';
import type { RemoteLogEntry } from '@/types/remote';
import { useTranslation } from '@/lib/i18n';
import { formatDate } from '@/lib/utils/format';
import { cn } from '@/lib/utils/cn';

/** 명령 줄의 보조 설명(도메인/액션). 명령이 아니면 사유를 쓴다. */
function detailOf(entry: RemoteLogEntry): string {
  if (entry.action === 'command' && entry.domain) {
    return entry.command_action ? `${entry.domain} / ${entry.command_action}` : entry.domain;
  }
  return entry.reason ?? '';
}

interface RemoteLogTableProps {
  entries: RemoteLogEntry[];
  isLoading?: boolean;
  isError?: boolean;
  /** 노드 칸 표시 여부. 한 노드만 보는 자리에서는 false. */
  showNode?: boolean;
  /** instance_id → 호스트명. 없는 노드(삭제됨)는 id 를 그대로 쓴다. */
  nodeNames?: Map<string, string>;
  /** true 면 이름 대신 id 를 적는다(전환 단추). */
  showNodeIds?: boolean;
  /** 현재 정렬 상태. 주면 헤더가 정렬 가능해진다. */
  sort?: SortState;
  /** 헤더 클릭 콜백. sort 와 함께 준다. */
  onSort?: (field: string) => void;
}

/** 한 줄의 노드 표기 — 이름 우선, 모르면 id. */
export function nodeLabel(
  instanceId: string,
  names?: Map<string, string>,
  showIds = false,
): string {
  if (showIds) return instanceId;
  return names?.get(instanceId) ?? instanceId;
}

export default function RemoteLogTable({
  entries,
  isLoading = false,
  isError = false,
  showNode = true,
  nodeNames,
  showNodeIds = false,
  sort,
  onSort,
}: RemoteLogTableProps) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-12 text-sm text-(--color-text-muted)">
        <Loader2 className="h-4 w-4 animate-spin" /> {t('common.loading')}
      </div>
    );
  }

  if (isError) {
    return (
      <div
        data-testid="remote-log-error"
        className="flex flex-col items-center gap-2 py-12 text-center text-(--color-text-muted)"
      >
        <AlertTriangle className="h-8 w-8 opacity-40" />
        <p className="text-sm">{t('remote.loadError')}</p>
      </div>
    );
  }

  if (entries.length === 0) {
    return (
      <p data-testid="remote-log-empty" className="py-12 text-center text-sm text-(--color-text-muted)">
        {t('remote.log.empty')}
      </p>
    );
  }

  /** 정렬 가능 헤더 — sort/onSort 가 없으면 평범한 th 로 떨어진다. */
  const header = (field: string, labelKey: string) =>
    sort && onSort ? (
      <SortableHeader
        key={field}
        label={t(labelKey)}
        field={field}
        currentSort={sort}
        onSort={onSort}
        className="px-4 py-3"
      />
    ) : (
      <th key={field} className="whitespace-nowrap px-4 py-3">
        {t(labelKey)}
      </th>
    );

  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-(--color-border-default)" data-testid="remote-log-table">
        <thead>
          <tr className="text-left text-xs font-medium uppercase text-(--color-text-muted)">
            {header('ts', 'remote.log.col.time')}
            {showNode && header('instance_id', 'remote.log.col.node')}
            {header('action', 'remote.log.col.action')}
            {header('actor', 'remote.log.col.actor')}
            <th className="whitespace-nowrap px-4 py-3">{t('remote.log.col.detail')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--color-border-default)">
          {entries.map((entry) => (
            <tr key={entry.id} className="text-sm text-(--color-text-primary)">
              <td className="whitespace-nowrap px-4 py-3 text-(--color-text-muted)">
                {formatDate(new Date(entry.timestamp), 'long')}
              </td>
              {showNode && (
                <td
                  className={cn(
                    'whitespace-nowrap px-4 py-3',
                    showNodeIds && 'font-mono text-xs',
                  )}
                  // 이름으로 보는 동안에도 id 를 확인할 자리가 필요하다.
                  title={entry.instance_id}
                >
                  {nodeLabel(entry.instance_id, nodeNames, showNodeIds)}
                </td>
              )}
              <td className="whitespace-nowrap px-4 py-3">
                <span
                  className={cn(
                    'rounded px-2 py-0.5 text-xs font-medium',
                    entry.result === 'error'
                      ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                      : 'bg-(--color-bg-elevated) text-(--color-text-primary)',
                  )}
                >
                  {t(logActionKey(entry))}
                </span>
              </td>
              <td className="whitespace-nowrap px-4 py-3 text-(--color-text-muted)">{entry.actor}</td>
              <td className="px-4 py-3 text-(--color-text-muted)">{detailOf(entry)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
