// 플로우 상세 패널.
// 행 확장 시 표시되며, 플로우 내 노드 목록과 통계, 로그 레벨 설정을 제공한다.

import { useEffect, useMemo, useState } from 'react';
import { ArrowDownToLine, ArrowUpFromLine } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';

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
    stopped: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
    error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  };
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        colors[state] ?? 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
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
        'rounded-md border border-gray-300 px-2 py-1 text-xs',
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

  // 내부 생성 노드 존재 여부 (옵션 토글 노출 판단용)
  const hasInternal = useMemo(() => (nodes ?? []).some(isInternalNode), [nodes]);

  // 필터(내부 노드) + 정렬된 노드 목록
  const sortedNodes = useMemo(() => {
    if (!nodes) return [];
    const visible = showInternal ? nodes : nodes.filter((n) => !isInternalNode(n));
    return [...visible].sort((a, b) => {
      let aVal = '';
      let bVal = '';
      switch (sort.field) {
        case 'name': aVal = a.name; bVal = b.name; break;
        case 'type': aVal = a.type; bVal = b.type; break;
        case 'state': aVal = a.state; bVal = b.state; break;
        default: aVal = a.name; bVal = b.name;
      }
      const cmp = aVal.localeCompare(bVal);
      return sort.direction === 'asc' ? cmp : -cmp;
    });
  }, [nodes, sort, showInternal]);

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
      <div className="mb-3 flex items-center justify-between gap-3">
        <h4 className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
          {t('flows.detail.nodeInstances').replace('{count}', String(sortedNodes.length))}
        </h4>
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
      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="min-w-full divide-y divide-(--color-border-default) text-sm">
          <thead className="bg-(--color-bg-sunken)">
            <tr>
              <SortableHeader label={t('common.name')} field="name" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
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
      </div>
    </div>
  );
}
