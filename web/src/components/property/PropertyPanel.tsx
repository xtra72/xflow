// 노드 속성 패널 컴포넌트.
// 선택된 노드의 라벨, 설정 데이터를 편집할 수 있다.

import { useCallback, useMemo } from 'react';
import { Settings2, X } from 'lucide-react';

import { useEditorStore } from '@/stores/editorStore';
import type { ConfigSchema } from '@/types/node';

import { DynamicForm } from './DynamicForm';

export function PropertyPanel() {
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const nodes = useEditorStore((s) => s.nodes);
  const updateNodeData = useEditorStore((s) => s.updateNodeData);
  const selectNode = useEditorStore((s) => s.selectNode);

  // 선택된 노드 찾기
  const selectedNode = useMemo(
    () => nodes.find((n) => n.id === selectedNodeId),
    [nodes, selectedNodeId],
  );

  /** 노드 데이터 변경 핸들러 */
  const handleDataChange = useCallback(
    (data: Record<string, unknown>) => {
      if (selectedNodeId) {
        updateNodeData(selectedNodeId, data);
      }
    },
    [selectedNodeId, updateNodeData],
  );

  /** 라벨 변경 핸들러 */
  const handleLabelChange = useCallback(
    (label: string) => {
      if (selectedNodeId) {
        updateNodeData(selectedNodeId, { label });
      }
    },
    [selectedNodeId, updateNodeData],
  );

  // 선택된 노드가 없을 때 빈 상태
  if (!selectedNode) {
    return (
      <aside
        className="flex w-[300px] shrink-0 flex-col items-center justify-center
          border-l border-gray-200 bg-white p-4
          dark:border-gray-700 dark:bg-gray-900"
      >
        <Settings2 className="mb-2 h-8 w-8 text-gray-300 dark:text-gray-600" />
        <p className="text-sm text-gray-400 dark:text-gray-500">
          노드를 선택하면 속성을 볼 수 있습니다
        </p>
      </aside>
    );
  }

  const nodeData = (selectedNode.data ?? {}) as Record<string, unknown>;
  const nodeType = (nodeData.type as string) ?? selectedNode.type ?? 'unknown';
  const nodeLabel = (nodeData.label as string) ?? '';
  const configSchema = nodeData.config_schema as ConfigSchema | undefined;

  return (
    <aside
      className="flex w-[300px] shrink-0 flex-col border-l border-gray-200
        bg-white dark:border-gray-700 dark:bg-gray-900"
    >
      {/* 헤더: 노드 타입 + 닫기 버튼 */}
      <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3 dark:border-gray-700">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
            {nodeType}
          </h3>
          <p className="truncate text-xs text-gray-400 dark:text-gray-500">
            {selectedNode.id}
          </p>
        </div>
        <button
          type="button"
          onClick={() => selectNode(null)}
          className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600
            dark:hover:bg-gray-800 dark:hover:text-gray-300 transition-colors"
          aria-label="속성 패널 닫기"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* 속성 편집 영역 */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* 라벨 입력 */}
        <div className="space-y-1">
          <label
            htmlFor="node-label"
            className="block text-xs font-medium text-gray-700 dark:text-gray-300"
          >
            라벨
          </label>
          <input
            id="node-label"
            type="text"
            value={nodeLabel}
            onChange={(e) => handleLabelChange(e.target.value)}
            placeholder="노드 이름 입력..."
            className="w-full rounded-md border border-gray-200 bg-gray-50 px-2.5 py-1.5
              text-sm placeholder:text-gray-400
              focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400
              dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100
              dark:placeholder:text-gray-500 dark:focus:border-blue-500"
          />
        </div>

        {/* 구분선 */}
        <hr className="border-gray-200 dark:border-gray-700" />

        {/* 동적 폼 */}
        <DynamicForm
          nodeId={selectedNode.id}
          data={nodeData}
          schema={configSchema}
          onChange={handleDataChange}
        />
      </div>
    </aside>
  );
}
