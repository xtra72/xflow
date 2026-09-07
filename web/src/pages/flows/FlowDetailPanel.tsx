// 플로우 상세 패널.
// 행 확장 시 표시되며, 플로우 내 노드 목록과 통계, 로그 레벨 설정을 제공한다.

import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDownToLine, ArrowUpFromLine, Check, Copy, Search } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';

import { shortenNodeId } from '@/lib/flow/subflowNamespace';

import { useFlowNodesTarget, useFlowStatusTarget } from '@/hooks/useDetailTargets';
import { useTranslation } from '@/lib/i18n';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { isRemoteTarget } from '@/lib/remote/target';
import { cn } from '@/lib/utils/cn';
import {
  getLogLevels,
  setComponentLogLevel,
  resetComponentLogLevel,
} from '@/services/api/monitorService';
import { useUIStore } from '@/stores/uiStore';
import type { FlowNodeInfo } from '@/types/flow';

interface FlowDetailPanelProps {
  flowId: string;
}

/** 노드 상태 배지 */
function NodeStateBadge({ state }: { state: string }) {
  const colors: Record<string, string> = {
    running: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
    stopped: 'bg-(--color-bg-sunken) text-(--color-text-secondary)',
    error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  };
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        colors[state] ?? 'bg-(--color-bg-sunken) text-(--color-text-secondary)',
      )}
    >
      {state}
    </span>
  );
}

/** 로그 레벨 드롭다운 */
function NodeLogLevelSelect({ nodeName }: { nodeName: string }) {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const [level, setLevel] = useState<string>('');
  const [updating, setUpdating] = useState(false);

  const componentKey = `node.${nodeName}`;

  useEffect(() => {
    let cancelled = false;
    getLogLevels()
      .then((info) => {
        if (cancelled) return;
        setLevel(info.components[componentKey] ?? '');
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [componentKey]);

  async function handleChange(value: string) {
    setUpdating(true);
    try {
      if (value === '') {
        await resetComponentLogLevel(componentKey);
        setLevel('');
        addNotification({ type: 'success', message: t('flows.detail.logLevelReset').replace('{node}', nodeName) });
      } else {
        await setComponentLogLevel(componentKey, value);
        setLevel(value);
        addNotification({
          type: 'success',
          message: t('flows.detail.logLevelChanged')
            .replace('{node}', nodeName)
            .replace('{level}', value.toUpperCase()),
        });
      }
    } catch {
      addNotification({ type: 'error', message: t('flows.detail.logLevelChangeFailed') });
    } finally {
      setUpdating(false);
    }
  }

  return (
    <select
      value={level}
      onChange={(e) => handleChange(e.target.value)}
      disabled={updating}
      className={cn(
        'rounded-md border border-(--color-border-strong) px-2 py-1 text-xs',
        'focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500',
        'border-(--color-border-strong) bg-(--color-bg-surface) text-(--color-text-primary)',
        'disabled:cursor-not-allowed disabled:opacity-50',
      )}
    >
      <option value="">{t('flows.detail.logLevelDefault')}</option>
      <option value="debug">DEBUG</option>
      <option value="info">INFO</option>
      <option value="warn">WARN</option>
      <option value="error">ERROR</option>
    </select>
  );
}

/** 포트 통계 요약: in / out 분리 표시 */
// 내부적으로 생성된 노드(공유 경계 tap 등 __xxx__ 규칙)는 사용자 정의 노드가
// 아니므로 인스턴스 리스트에서 기본 숨긴다. "내부 노드 표시" 옵션으로 노출 가능.
function isInternalNode(node: FlowNodeInfo): boolean {
  return node.type.startsWith('__');
}

/** 노드 ID 셀: 축약 표시 + 전체 값 툴팁 + 클릭 복사. */
function NodeIdCell({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(nodeId);
    } catch {
      // 복사 실패는 silent — 전체 값은 툴팁으로 노출되어 수동 선택이 가능하다.
      return;
    }
    if (timerRef.current) clearTimeout(timerRef.current);
    setCopied(true);
    timerRef.current = setTimeout(() => setCopied(false), 2000);
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      title={nodeId}
      aria-label={t('flows.detail.copyId')}
      className={cn(
        'inline-flex max-w-full items-center gap-1 rounded px-1 py-0.5 font-mono text-xs',
        'text-(--color-text-muted) hover:bg-(--color-bg-sunken) hover:text-(--color-text-primary)',
      )}
    >
      <span className="truncate">{shortenNodeId(nodeId)}</span>
      {copied ? (
        <Check className="h-3 w-3 shrink-0 text-green-600" aria-hidden="true" />
      ) : (
        <Copy className="h-3 w-3 shrink-0 opacity-50" aria-hidden="true" />
      )}
    </button>
  );
}

function PortStats({ node }: { node: FlowNodeInfo }) {
  const { t } = useTranslation();
  if (!node.ports || node.ports.length === 0) return null;

  const inMessages = node.ports
    .filter((p) => p.direction === 'input')
    .reduce((sum, p) => sum + p.messages, 0);
  const outMessages = node.ports
    .filter((p) => p.direction === 'output')
    .reduce((sum, p) => sum + p.messages, 0);

  return (
    <span className="inline-flex items-center gap-2 text-xs text-(--color-text-muted)">
      <span className="inline-flex items-center gap-0.5" title={t('flows.detail.portIn')}>
        <ArrowDownToLine className="h-3 w-3" />
        {inMessages.toLocaleString()}
      </span>
      <span className="inline-flex items-center gap-0.5" title={t('flows.detail.portOut')}>
        <ArrowUpFromLine className="h-3 w-3" />
        {outMessages.toLocaleString()}
      </span>
    </span>
  );
}

export default function FlowDetailPanel({ flowId }: FlowDetailPanelProps) {
  const { t } = useTranslation();
  // SPEC-REMOTE-001 M8 (그룹 J): 타깃에 따라 상태/노드 소스를 전환한다(로컬은
  // 기존 useFlowStatus/useFlowNodes 위임 — 회귀 없음). 원격은 READ 프록시.
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const { data: flowStatus } = useFlowStatusTarget(target, flowId);
  const isRunning = flowStatus?.status === 'running';
  const { data: nodes, isLoading } = useFlowNodesTarget(target, flowId, isRunning ? 3000 : undefined);
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });
  // 내부 생성 노드 표시 여부 (기본 숨김)
  const [showInternal, setShowInternal] = useState(false);
  // 이름/타입/노드 ID 부분 일치 검색.
  // 에러 로그의 노드 ID(subflow_<부모>_<원본>)를 그대로 붙여넣어 찾는 것이 주 용도다.
  const [query, setQuery] = useState('');

  // 내부 생성 노드 존재 여부 (옵션 토글 노출 판단용)
  const hasInternal = useMemo(() => (nodes ?? []).some(isInternalNode), [nodes]);

  // 필터(내부 노드 + 검색) + 정렬된 노드 목록
  const sortedNodes = useMemo(() => {
    if (!nodes) return [];
    let visible = showInternal ? nodes : nodes.filter((n) => !isInternalNode(n));
    const q = query.trim().toLowerCase();
    if (q) {
      visible = visible.filter(
        (n) =>
          n.node_id.toLowerCase().includes(q) ||
          n.name.toLowerCase().includes(q) ||
          n.type.toLowerCase().includes(q),
      );
    }
    return [...visible].sort((a, b) => {
      let aVal = '';
      let bVal = '';
      switch (sort.field) {
        case 'name': aVal = a.name; bVal = b.name; break;
        case 'type': aVal = a.type; bVal = b.type; break;
        case 'state': aVal = a.state; bVal = b.state; break;
        case 'node_id': aVal = a.node_id; bVal = b.node_id; break;
        default: aVal = a.name; bVal = b.name;
      }
      const cmp = aVal.localeCompare(bVal);
      return sort.direction === 'asc' ? cmp : -cmp;
    });
  }, [nodes, sort, showInternal, query]);

  function handleSort(field: string) {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-2 p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-10 animate-pulse rounded bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  if (!nodes || nodes.length === 0) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        {t('flows.detail.emptyNodes')}
      </div>
    );
  }

  return (
    <div className="p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h4 className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
          {t('flows.detail.nodeInstances').replace('{count}', String(sortedNodes.length))}
        </h4>
        <div className="flex flex-1 items-center justify-end gap-3">
          <div className="relative min-w-0 flex-1 sm:max-w-xs">
            <Search
              className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-(--color-text-muted)"
              aria-hidden="true"
            />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('flows.detail.searchPlaceholder')}
              aria-label={t('flows.detail.searchPlaceholder')}
              className={cn(
                'w-full rounded-md border py-1 pl-7 pr-2 text-xs',
                'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
                'placeholder:text-(--color-text-muted)',
                'focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500',
              )}
            />
          </div>
        {hasInternal && (
          <label className="flex cursor-pointer items-center gap-1.5 text-xs text-(--color-text-muted)">
            <input
              type="checkbox"
              className="h-3.5 w-3.5"
              checked={showInternal}
              onChange={(e) => setShowInternal(e.target.checked)}
            />
            {t('flows.detail.showInternalNodes')}
          </label>
        )}
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="min-w-full divide-y divide-(--color-border-default) text-sm">
          <thead className="bg-(--color-bg-sunken)">
            <tr>
              <SortableHeader label={t('common.name')} field="name" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <SortableHeader label={t('flows.detail.nodeId')} field="node_id" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <SortableHeader label={t('common.type')} field="type" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <SortableHeader label={t('common.status')} field="state" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <th className="px-3 py-2 text-right text-xs font-medium text-(--color-text-muted)">
                In / Out
              </th>
              {!remote && (
                <th className="px-3 py-2 text-right text-xs font-medium text-(--color-text-muted)">
                  {t('flows.detail.logLevel')}
                </th>
              )}
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
            {sortedNodes.map((node) => (
              <tr key={node.node_id}>
                <td className="whitespace-nowrap px-3 py-2 font-medium text-(--color-text-primary)">
                  {node.name}
                </td>
                <td className="whitespace-nowrap px-3 py-2">
                  <NodeIdCell nodeId={node.node_id} />
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-(--color-text-muted)">
                  {node.type}
                </td>
                <td className="whitespace-nowrap px-3 py-2">
                  <NodeStateBadge state={node.state} />
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-right">
                  <PortStats node={node} />
                </td>
                {!remote && (
                  <td className="whitespace-nowrap px-3 py-2 text-right">
                    <NodeLogLevelSelect nodeName={node.name} />
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
        {sortedNodes.length === 0 && (
          <div className="px-3 py-6 text-center text-sm text-(--color-text-muted)">
            {t('flows.detail.noMatch')}
          </div>
        )}
      </div>
    </div>
  );
}
