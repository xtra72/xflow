// 엣지(Wire) 속성 패널 컴포넌트.
// 선택된 엣지의 모드와 버퍼 크기를 편집할 수 있다.

import { useCallback, useMemo } from 'react';
import { X } from 'lucide-react';

import { useEditorStore } from '@/stores/editorStore';

const WIRE_MODE_OPTIONS = [
  { value: 'bypass', label: 'Bypass (동기)' },
  { value: 'buffer', label: 'Buffer (비동기)' },
  { value: 'drop_oldest', label: 'Drop Oldest (오래된 메시지 드롭)' },
] as const;

interface EdgePropertyPanelProps {
  width?: number;
}

export function EdgePropertyPanel({ width }: EdgePropertyPanelProps): React.ReactElement | null {
  const selectedEdgeId = useEditorStore((s) => s.selectedEdgeId);
  const edges = useEditorStore((s) => s.edges);
  const nodes = useEditorStore((s) => s.nodes);
  const updateEdgeData = useEditorStore((s) => s.updateEdgeData);
  const selectEdge = useEditorStore((s) => s.selectEdge);

  const selectedEdge = useMemo(
    () => edges.find((e) => e.id === selectedEdgeId),
    [edges, selectedEdgeId],
  );

  const sourceNodeLabel = useMemo(() => {
    if (!selectedEdge) return '';
    const node = nodes.find((n) => n.id === selectedEdge.source);
    return (node?.data?.label as string) || node?.id || selectedEdge.source;
  }, [nodes, selectedEdge]);

  const targetNodeLabel = useMemo(() => {
    if (!selectedEdge) return '';
    const node = nodes.find((n) => n.id === selectedEdge.target);
    return (node?.data?.label as string) || node?.id || selectedEdge.target;
  }, [nodes, selectedEdge]);

  const mode = (selectedEdge as Record<string, unknown> | undefined)?.mode as string ?? 'bypass';
  const bufferSize = (selectedEdge as Record<string, unknown> | undefined)?.buffer_size as number ?? 0;
  const wireName = (selectedEdge as Record<string, unknown> | undefined)?.name as string ?? '';

  const handleModeChange = useCallback(
    (newMode: string) => {
      if (!selectedEdgeId) return;
      const updates: Record<string, unknown> = { mode: newMode };
      // buffer/drop_oldest 로 전환 시 기본 buffer_size 설정
      if (newMode !== 'bypass' && bufferSize === 0) {
        updates.buffer_size = 256;
      }
      // bypass 로 전환 시 buffer_size 초기화
      if (newMode === 'bypass') {
        updates.buffer_size = 0;
      }
      updateEdgeData(selectedEdgeId, updates);
    },
    [selectedEdgeId, bufferSize, updateEdgeData],
  );

  const handleBufferSizeChange = useCallback(
    (value: string) => {
      if (!selectedEdgeId) return;
      const parsed = parseInt(value, 10);
      if (!isNaN(parsed) && parsed >= 0) {
        updateEdgeData(selectedEdgeId, { buffer_size: parsed });
      }
    },
    [selectedEdgeId, updateEdgeData],
  );

  if (!selectedEdge) return null;

  return (
    <aside
      style={width ? { width: `${width}px` } : undefined}
      className={`flex shrink-0 flex-col border-l border-(--color-border-default)
        bg-(--color-bg-surface)
        ${width ? '' : 'w-[300px]'}`}
    >
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-(--color-text-primary)">
            Wire
          </h3>
          <p className="truncate text-xs text-(--color-text-muted)">
            {selectedEdge.id}
          </p>
        </div>
        <button
          type="button"
          onClick={() => selectEdge(null)}
          className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600
            dark:hover:bg-gray-800 dark:hover:text-gray-300 transition-colors"
          aria-label="속성 패널 닫기"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Wire 이름 (읽기 전용) */}
        {wireName && (
          <div>
            <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">
              이름
            </label>
            <p className="text-xs text-(--color-text-primary) break-all">{wireName}</p>
          </div>
        )}

        {/* Source -> Target (읽기 전용) */}
        <div>
          <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">
            연결
          </label>
          <p className="text-xs text-(--color-text-primary)">
            {sourceNodeLabel}
            <span className="mx-1 text-(--color-text-muted)">&rarr;</span>
            {targetNodeLabel}
          </p>
        </div>

        {/* Mode 선택 */}
        <div>
          <label
            htmlFor="wire-mode"
            className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
          >
            모드
          </label>
          <select
            id="wire-mode"
            value={mode}
            onChange={(e) => handleModeChange(e.target.value)}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5
              text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {WIRE_MODE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        {/* Buffer Size (bypass 가 아닐 때만) */}
        {mode !== 'bypass' && (
          <div>
            <label
              htmlFor="wire-buffer-size"
              className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
            >
              버퍼 크기
            </label>
            <input
              id="wire-buffer-size"
              type="number"
              min={1}
              value={bufferSize}
              onChange={(e) => handleBufferSizeChange(e.target.value)}
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5
                text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            <p className="mt-1 text-xs text-(--color-text-muted)">
              {mode === 'buffer'
                ? '버퍼가 가득 차면 송신 측이 대기합니다.'
                : '버퍼가 가득 차면 가장 오래된 메시지가 삭제됩니다.'}
            </p>
          </div>
        )}
      </div>
    </aside>
  );
}
