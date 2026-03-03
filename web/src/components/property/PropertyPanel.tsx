// 노드 속성 패널 컴포넌트.
// 선택된 노드의 라벨, 포트, 설정 데이터를 편집할 수 있다.
// 변경 사항은 로컬 드래프트에 저장되며, 적용/취소 버튼으로 확정한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { ArrowDownToLine, ArrowUpFromLine, Check, Plus, RotateCcw, Settings2, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

import { getConfigSchema } from '@/config/nodeSchemas';
import { useEditorStore } from '@/stores/editorStore';
import type { ConfigSchema } from '@/types/node';

import { DynamicForm } from './DynamicForm';

// --- 포트 관리 서브 컴포넌트 ---

type Port = { name: string; direction: 'input' | 'output' };

interface PortSectionProps {
  ports: Port[];
  onChange: (ports: Port[]) => void;
}

function PortSection({ ports, onChange }: PortSectionProps) {
  const [adding, setAdding] = useState(false);
  const [newDirection, setNewDirection] = useState<'input' | 'output'>('input');
  const [newName, setNewName] = useState('');

  const handleAdd = () => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    onChange([...ports, { name: trimmed, direction: newDirection }]);
    setNewName('');
    setAdding(false);
  };

  const handleDelete = (index: number) => {
    onChange(ports.filter((_, i) => i !== index));
  };

  const handleNameChange = (index: number, name: string) => {
    const updated = ports.map((p, i) => (i === index ? { ...p, name } : p));
    onChange(updated);
  };

  return (
    <div className="space-y-2">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-gray-700 dark:text-gray-300">
          포트
        </span>
        <button
          type="button"
          onClick={() => setAdding(!adding)}
          className={cn(
            'rounded p-0.5 transition-colors',
            'text-gray-400 hover:bg-gray-100 hover:text-gray-600',
            'dark:hover:bg-gray-800 dark:hover:text-gray-300',
          )}
          aria-label="포트 추가"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* 포트 목록 */}
      {ports.length === 0 && !adding && (
        <p className="text-xs text-gray-400 dark:text-gray-500">포트 없음</p>
      )}
      {ports.map((port, idx) => (
        <div key={idx} className="flex items-center gap-1.5">
          {/* 방향 아이콘 */}
          {port.direction === 'input' ? (
            <ArrowDownToLine className="h-3.5 w-3.5 shrink-0 text-blue-500 dark:text-blue-400" />
          ) : (
            <ArrowUpFromLine className="h-3.5 w-3.5 shrink-0 text-emerald-500 dark:text-emerald-400" />
          )}
          {/* 포트 이름 */}
          <input
            type="text"
            value={port.name}
            onChange={(e) => handleNameChange(idx, e.target.value)}
            className={cn(
              'min-w-0 flex-1 rounded border px-1.5 py-0.5 text-xs',
              'border-gray-200 bg-white text-gray-900',
              'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
              'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
            )}
          />
          {/* 삭제 버튼 */}
          <button
            type="button"
            onClick={() => handleDelete(idx)}
            className={cn(
              'rounded p-0.5 transition-colors',
              'text-gray-400 hover:bg-red-50 hover:text-red-500',
              'dark:hover:bg-red-900/20 dark:hover:text-red-400',
            )}
            aria-label={`포트 ${port.name} 삭제`}
          >
            <Trash2 className="h-3 w-3" />
          </button>
        </div>
      ))}

      {/* 추가 폼 */}
      {adding && (
        <div className="flex items-center gap-1.5 rounded border border-dashed border-gray-300 p-1.5 dark:border-gray-600">
          <select
            value={newDirection}
            onChange={(e) => setNewDirection(e.target.value as 'input' | 'output')}
            className={cn(
              'rounded border px-1 py-0.5 text-xs',
              'border-gray-200 bg-white text-gray-900',
              'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
            )}
          >
            <option value="input">입력</option>
            <option value="output">출력</option>
          </select>
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleAdd();
              if (e.key === 'Escape') setAdding(false);
            }}
            placeholder="포트 이름"
            autoFocus
            className={cn(
              'min-w-0 flex-1 rounded border px-1.5 py-0.5 text-xs',
              'border-gray-200 bg-white text-gray-900 placeholder:text-gray-400',
              'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
              'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
              'dark:placeholder:text-gray-500',
            )}
          />
          <button
            type="button"
            onClick={handleAdd}
            className={cn(
              'rounded px-1.5 py-0.5 text-xs font-medium transition-colors',
              'bg-blue-500 text-white hover:bg-blue-600',
              'dark:bg-blue-600 dark:hover:bg-blue-700',
            )}
          >
            추가
          </button>
        </div>
      )}
    </div>
  );
}

// --- 메인 컴포넌트 ---

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

  // 원본 노드 데이터 (스토어 기준)
  const originalData = useMemo(
    () => (selectedNode?.data ?? {}) as Record<string, unknown>,
    [selectedNode?.data],
  );

  // 로컬 드래프트 상태: 변경 사항을 여기에 누적하고 적용/취소로 확정
  const [draft, setDraft] = useState<Record<string, unknown>>(originalData);

  // 선택 노드가 바뀌면 드래프트를 원본으로 리셋
  useEffect(() => {
    setDraft(originalData);
  }, [selectedNodeId, originalData]);

  // 변경 여부 감지
  const hasChanges = useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(originalData),
    [draft, originalData],
  );

  /** 드래프트 데이터 변경 (스토어에 반영하지 않음) */
  const handleDraftChange = useCallback(
    (data: Record<string, unknown>) => {
      setDraft((prev) => ({ ...prev, ...data }));
    },
    [],
  );

  /** 적용: 드래프트를 스토어에 반영 */
  const handleApply = useCallback(() => {
    if (!selectedNodeId || !hasChanges) return;
    updateNodeData(selectedNodeId, draft);
  }, [selectedNodeId, hasChanges, draft, updateNodeData]);

  /** 취소: 드래프트를 원본으로 되돌림 */
  const handleCancel = useCallback(() => {
    setDraft(originalData);
  }, [originalData]);

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

  const nodeType = (draft.nodeType as string) ?? (draft.type as string) ?? selectedNode.type ?? 'unknown';
  const nodeLabel = (draft.label as string) ?? '';
  const configSchema = (draft.config_schema as ConfigSchema | undefined) ?? getConfigSchema(nodeType);
  const ports = (draft.ports ?? []) as Port[];

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
            onChange={(e) => handleDraftChange({ label: e.target.value })}
            placeholder="노드 이름 입력..."
            className="w-full rounded-md border border-gray-200 bg-gray-50 px-2.5 py-1.5
              text-sm placeholder:text-gray-400
              focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400
              dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100
              dark:placeholder:text-gray-500 dark:focus:border-blue-500"
          />
        </div>

        {/* 포트 관리 */}
        <PortSection ports={ports} onChange={(newPorts) => handleDraftChange({ ports: newPorts })} />

        {/* 구분선 */}
        <hr className="border-gray-200 dark:border-gray-700" />

        {/* 동적 폼 */}
        <DynamicForm
          nodeId={selectedNode.id}
          data={draft}
          schema={configSchema}
          onChange={handleDraftChange}
        />
      </div>

      {/* 적용/취소 버튼 */}
      {hasChanges && (
        <div
          className="flex items-center gap-2 border-t border-gray-200 px-4 py-3
            dark:border-gray-700"
        >
          <button
            type="button"
            onClick={handleApply}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5',
              'text-sm font-medium transition-colors',
              'bg-blue-500 text-white hover:bg-blue-600',
              'dark:bg-blue-600 dark:hover:bg-blue-700',
            )}
          >
            <Check className="h-3.5 w-3.5" />
            적용
          </button>
          <button
            type="button"
            onClick={handleCancel}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5',
              'text-sm font-medium transition-colors',
              'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50',
              'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700',
            )}
          >
            <RotateCcw className="h-3.5 w-3.5" />
            취소
          </button>
        </div>
      )}
    </aside>
  );
}
