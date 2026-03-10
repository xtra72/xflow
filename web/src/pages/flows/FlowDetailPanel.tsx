// 플로우 상세 패널.
// 행 확장 시 표시되며, 플로우 내 노드 목록과 통계, 로그 레벨 설정을 제공한다.

import { useEffect, useMemo, useState } from 'react';
import { ArrowDownToLine, ArrowUpFromLine } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';

import { useFlowNodes, useFlowStatus } from '@/hooks/useFlow';
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
        addNotification({ type: 'success', message: `${nodeName} 로그 레벨이 기본값으로 리셋되었습니다` });
      } else {
        await setComponentLogLevel(componentKey, value);
        setLevel(value);
        addNotification({ type: 'success', message: `${nodeName} 로그 레벨이 "${value.toUpperCase()}"로 변경되었습니다` });
      }
    } catch {
      addNotification({ type: 'error', message: '로그 레벨 변경에 실패했습니다' });
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
        'dark:border-gray-600 dark:bg-gray-700 dark:text-white',
        'disabled:cursor-not-allowed disabled:opacity-50',
      )}
    >
      <option value="">기본값</option>
      <option value="debug">DEBUG</option>
      <option value="info">INFO</option>
      <option value="warn">WARN</option>
      <option value="error">ERROR</option>
    </select>
  );
}

/** 포트 통계 요약: in / out 분리 표시 */
function PortStats({ node }: { node: FlowNodeInfo }) {
  if (!node.ports || node.ports.length === 0) return null;

  const inMessages = node.ports
    .filter((p) => p.direction === 'input')
    .reduce((sum, p) => sum + p.messages, 0);
  const outMessages = node.ports
    .filter((p) => p.direction === 'output')
    .reduce((sum, p) => sum + p.messages, 0);

  return (
    <span className="inline-flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
      <span className="inline-flex items-center gap-0.5" title="입력">
        <ArrowDownToLine className="h-3 w-3" />
        {inMessages.toLocaleString()}
      </span>
      <span className="inline-flex items-center gap-0.5" title="출력">
        <ArrowUpFromLine className="h-3 w-3" />
        {outMessages.toLocaleString()}
      </span>
    </span>
  );
}

export default function FlowDetailPanel({ flowId }: FlowDetailPanelProps) {
  const { data: flowStatus } = useFlowStatus(flowId);
  const isRunning = flowStatus?.status === 'running';
  const { data: nodes, isLoading } = useFlowNodes(flowId, isRunning ? 3000 : undefined);
  const [sort, setSort] = useState<SortState>({ field: 'name', direction: 'asc' });

  // 정렬된 노드 목록
  const sortedNodes = useMemo(() => {
    if (!nodes) return [];
    return [...nodes].sort((a, b) => {
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
  }, [nodes, sort]);

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
            className="h-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"
          />
        ))}
      </div>
    );
  }

  if (!nodes || nodes.length === 0) {
    return (
      <div className="p-4 text-sm text-gray-500 dark:text-gray-400">
        노드 정보가 없습니다. 플로우를 배포하면 노드가 표시됩니다.
      </div>
    );
  }

  return (
    <div className="p-4">
      <h4 className="mb-3 text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
        노드 인스턴스 ({nodes.length})
      </h4>
      <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
        <table className="min-w-full divide-y divide-gray-200 text-sm dark:divide-gray-700">
          <thead className="bg-gray-100 dark:bg-gray-800">
            <tr>
              <SortableHeader label="이름" field="name" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <SortableHeader label="타입" field="type" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <SortableHeader label="상태" field="state" currentSort={sort} onSort={handleSort} className="px-3 py-2" />
              <th className="px-3 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400">
                In / Out
              </th>
              <th className="px-3 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400">
                로그 레벨
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-900">
            {sortedNodes.map((node) => (
              <tr key={node.node_id}>
                <td className="whitespace-nowrap px-3 py-2 font-medium text-gray-900 dark:text-white">
                  {node.name}
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-gray-500 dark:text-gray-400">
                  {node.type}
                </td>
                <td className="whitespace-nowrap px-3 py-2">
                  <NodeStateBadge state={node.state} />
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-right">
                  <PortStats node={node} />
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-right">
                  <NodeLogLevelSelect nodeName={node.name} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
