// 엣지(Wire) 속성 패널 컴포넌트.
// 선택된 엣지의 모드와 버퍼 크기를 편집할 수 있다.

import { useCallback, useMemo } from 'react';
import { X } from 'lucide-react';

import { useEditorStore } from '@/stores/editorStore';

const WIRE_MODE_OPTIONS = [
  { value: 'buffer', label: '큐(가득 차면 대기)' },
  { value: 'drop_oldest', label: '큐(가득 차면 오래된 것 드랍)' },
  { value: 'bypass', label: '무버퍼(즉시 전달)' },
] as const;

/** 새 엣지/큐 모드 전환 시 사용하는 기본 큐 용량. */
const DEFAULT_BUFFER_SIZE = 100;

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

  const mode = (selectedEdge as Record<string, unknown> | undefined)?.mode as string ?? 'buffer';
  const bufferSize = (selectedEdge as Record<string, unknown> | undefined)?.buffer_size as number ?? DEFAULT_BUFFER_SIZE;
  const wireName = (selectedEdge as Record<string, unknown> | undefined)?.name as string ?? '';
  // SPEC-LINK-001: 가상 링크 여부(최상위 `virtual` 속성, 기본 false).
  const isVirtual = (selectedEdge as Record<string, unknown> | undefined)?.virtual === true;

  // 가상 링크 토글: 표시 전환일 뿐 라우팅은 불변. 실제 편집이므로 dirty 가 된다.
  const handleVirtualToggle = useCallback(
    (next: boolean) => {
      if (!selectedEdgeId) return;
      updateEdgeData(selectedEdgeId, { virtual: next });
    },
    [selectedEdgeId, updateEdgeData],
  );

  // 링크 이름 편집: 같은 이름의 가상 와이어들은 같은 링크 그룹으로 표시된다.
  const handleNameChange = useCallback(
    (value: string) => {
      if (!selectedEdgeId) return;
      updateEdgeData(selectedEdgeId, { name: value });
    },
    [selectedEdgeId, updateEdgeData],
  );

  const handleModeChange = useCallback(
    (newMode: string) => {
      if (!selectedEdgeId) return;
      const updates: Record<string, unknown> = { mode: newMode };
      // 큐(buffer/drop_oldest) 로 전환하는데 용량이 0(무버퍼) 이면 기본 용량으로 채운다.
      if (newMode !== 'bypass' && bufferSize === 0) {
        updates.buffer_size = DEFAULT_BUFFER_SIZE;
      }
      // 무버퍼(bypass) 로 전환 시 용량을 0 으로 초기화한다.
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
        {/* SPEC-LINK-001: 링크 이름 (편집 가능) — 같은 이름은 같은 링크 그룹 */}
        <div>
          <label
            htmlFor="wire-name"
            className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
          >
            링크 이름
          </label>
          <input
            id="wire-name"
            type="text"
            value={wireName}
            onChange={(e) => handleNameChange(e.target.value)}
            placeholder="링크 이름"
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5
              text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        {/* SPEC-LINK-001: 가상 링크 토글 — 켜면 연결선을 숨기고 양 끝 포트에
            "출력/입력 링크" 배지로 분해 표시한다. 라우팅은 변하지 않는다. */}
        <div>
          <label className="flex cursor-pointer items-center justify-between gap-2">
            <span className="text-xs font-medium text-(--color-text-secondary)">
              가상 링크
            </span>
            <input
              type="checkbox"
              role="switch"
              checked={isVirtual}
              onChange={(e) => handleVirtualToggle(e.target.checked)}
              aria-label="가상 링크"
              className="h-4 w-4 cursor-pointer accent-blue-500"
            />
          </label>
          <p className="mt-1 text-xs text-(--color-text-muted)">
            켜면 연결선을 숨기고 양 끝 포트에 링크 배지로 표시합니다(라우팅 불변).
          </p>
        </div>

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

        {/* 큐 용량 (0 = 무버퍼/bypass) */}
        <div>
          <label
            htmlFor="wire-buffer-size"
            className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
          >
            큐 용량
          </label>
          <input
            id="wire-buffer-size"
            type="number"
            min={0}
            value={bufferSize}
            onChange={(e) => handleBufferSizeChange(e.target.value)}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5
              text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <p className="mt-1 text-xs text-(--color-text-muted)">
            {mode === 'bypass'
              ? '무버퍼: 메시지를 큐에 쌓지 않고 즉시 전달합니다(0).'
              : mode === 'buffer'
                ? '큐가 가득 차면 송신 측이 대기합니다.'
                : '큐가 가득 차면 가장 오래된 메시지가 삭제됩니다.'}
          </p>
        </div>
      </div>
    </aside>
  );
}
