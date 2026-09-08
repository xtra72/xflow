// 노드 속성 패널 컴포넌트.
// 선택된 노드의 라벨, 포트, 설정 데이터를 편집할 수 있다.
// 변경 사항은 로컬 드래프트에 저장되며, 적용/취소 버튼으로 확정한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, ArrowDownToLine, ArrowUpFromLine, Check, Plus, RotateCcw, Settings2, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import { computePortsForNode, getConfigSchema, getNodeDescription, getNodeIODesc, getRequiredFieldErrors, type PortDef } from '@/config/nodeSchemas';
import { useAgents } from '@/hooks/useAgent';
import { normalizeNodeType } from '@/lib/flow/nodeType';
import { useEditorStore } from '@/stores/editorStore';
import { NODE_TYPE_META } from '@/pages/nodes/nodeTypeMeta';
import type { ConfigSchema } from '@/types/node';

import { DynamicForm } from './DynamicForm';
import { FieldHelp } from './FieldHelp';

// --- trigger 페이로드 하위호환 헬퍼 (v1.3.0 단일 페이로드 에디터) ---

/** map 여부(플레인 오브젝트, 배열/ null 제외). */
function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v != null && typeof v === 'object' && !Array.isArray(v);
}

/**
 * 로드 시 하위호환 병합: payload(any)와 payload_template(map)를 단일 에디터 값으로 합친다.
 * - 둘 다 map이면 병합(payload가 우선).
 * - 하나만 있으면 그 값을 사용(payload는 스칼라/배열도 그대로 표시).
 * - 둘 다 없으면 undefined(미설정).
 */
function mergeTriggerPayload(payload: unknown, template: unknown): unknown {
  const hasPayload = payload !== undefined && payload !== null;
  const hasTemplate = isPlainObject(template) && Object.keys(template).length > 0;
  if (hasPayload && hasTemplate && isPlainObject(payload)) {
    return { ...(template as Record<string, unknown>), ...payload };
  }
  if (hasPayload) return payload;
  if (hasTemplate) return template;
  return undefined;
}

/** 저장 시 페이로드가 비어있는지 판정(미설정/빈 문자열/빈 오브젝트 → 기본 폴백). */
function isNonEmptyPayload(payload: unknown): boolean {
  if (payload === undefined || payload === null) return false;
  if (typeof payload === 'string') return payload.trim() !== '';
  if (isPlainObject(payload)) return Object.keys(payload).length > 0;
  // 숫자/불리언/배열 등은 값이 있는 것으로 간주.
  return true;
}

// --- 포트 관리 서브 컴포넌트 ---

type Port = { name: string; direction: 'input' | 'output' | 'error' };

interface PortSectionProps {
  ports: Port[];
  onChange: (ports: Port[]) => void;
  /** 포트 이름 → 설명. 라벨 옆 `?` 도움말로 표시한다(편집은 그대로 유지). */
  portDescriptions?: Record<string, string>;
}

function PortSection({ ports, onChange, portDescriptions }: PortSectionProps) {
  const { t } = useTranslation();
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

  // 포트별 설명을 "포트" 타이틀 옆 ? 도움말 하나로 합친다. 행마다 ? 를 두면
  // 우측 끝에서 팝오버가 화면 밖으로 잘리므로(left-0 기준), 좌측 타이틀에 모은다.
  const portsHelpText = ports
    .map((p) => {
      const d = portDescriptions?.[p.name];
      return d ? `${p.name} — ${d}` : null;
    })
    .filter((v): v is string => !!v)
    .join('\n\n');

  return (
    <div className="space-y-2">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="flex items-center gap-1">
          <span className="text-xs font-medium text-(--color-text-secondary)">
            {t('property.panel.ports')}
          </span>
          {portsHelpText && (
            <FieldHelp text={portsHelpText} describedById="ports-desc" />
          )}
        </span>
        <button
          type="button"
          onClick={() => setAdding(!adding)}
          className={cn(
            'rounded p-0.5 transition-colors',
            'text-(--color-text-muted) hover:bg-(--color-bg-sunken) hover:text-(--color-text-secondary)',
          )}
          aria-label={t('property.panel.addPortAria')}
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* 포트 목록 */}
      {ports.length === 0 && !adding && (
        <p className="text-xs text-(--color-text-muted)">{t('property.panel.portsEmpty')}</p>
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
              'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
              'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
            )}
          />
          {/* 삭제 버튼 */}
          <button
            type="button"
            onClick={() => handleDelete(idx)}
            className={cn(
              'rounded p-0.5 transition-colors',
              'text-(--color-text-muted) hover:bg-red-50 hover:text-red-500',
              'dark:hover:bg-red-900/20 dark:hover:text-red-400',
            )}
            aria-label={t('property.panel.deletePortAria').replace('{name}', port.name)}
          >
            <Trash2 className="h-3 w-3" />
          </button>
        </div>
      ))}

      {/* 추가 폼 */}
      {adding && (
        <div className="flex items-center gap-1.5 rounded border border-dashed border-(--color-border-strong) p-1.5">
          <select
            value={newDirection}
            onChange={(e) => setNewDirection(e.target.value as 'input' | 'output')}
            className={cn(
              'rounded border px-1 py-0.5 text-xs',
              'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
            )}
          >
            <option value="input">{t('property.panel.portInput')}</option>
            <option value="output">{t('property.panel.portOutput')}</option>
          </select>
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleAdd();
              if (e.key === 'Escape') setAdding(false);
            }}
            placeholder={t('property.panel.portNamePlaceholder')}
            autoFocus
            className={cn(
              'min-w-0 flex-1 rounded border px-1.5 py-0.5 text-xs',
              'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary) placeholder:text-(--color-text-muted)',
              'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
              '',
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
            {t('property.panel.addPort')}
          </button>
        </div>
      )}
    </div>
  );
}

// --- 메인 컴포넌트 ---

interface PropertyPanelProps {
  /** 패널 너비 (px). 외부에서 리사이즈 핸들이 조절한다. */
  width?: number;
}

export function PropertyPanel({ width }: PropertyPanelProps) {
  const { t } = useTranslation();
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

  // 에이전트 목록 조회 (hooks 규칙상 조건부 반환 이전에 호출)
  const { data: agentsResult } = useAgents();
  const agents = useMemo(() => agentsResult?.data ?? [], [agentsResult?.data]);

  // 로컬 드래프트 상태: 변경 사항을 여기에 누적하고 적용/취소로 확정
  const [draft, setDraft] = useState<Record<string, unknown>>(originalData);

  // 선택 노드가 바뀌면 드래프트를 원본으로 리셋.
  // trigger 노드는 v1.3.0에서 단일 페이로드 에디터로 통합되었다. 하위호환을 위해
  // 로드 시 payload(map)와 payload_template(map)를 모두 수용하여 단일 `payload`
  // 에디터에 병합 표시하고, 폐기된 payload_mode 가상 필드는 제거한다.
  useEffect(() => {
    const nodeType = (originalData.nodeType as string) ?? '';
    if (nodeType === 'trigger') {
      // payload_mode(폐기)와 하위호환 payload_template를 드래프트에서 분리.
      const { payload_mode: _pm, payload, payload_template, ...rest } =
        originalData as {
          payload_mode?: unknown;
          payload?: unknown;
          payload_template?: unknown;
        } & Record<string, unknown>;
      const next: Record<string, unknown> = { ...rest };
      const merged = mergeTriggerPayload(payload, payload_template);
      if (merged !== undefined) next.payload = merged;
      setDraft(next);
      return;
    }
    setDraft(originalData);
  }, [selectedNodeId, originalData]);

  // Bridge 노드 agent_id 자동 해석: agent_name으로 스토어의 agent_id를 채운다.
  // YAML에서 로드한 플로우는 agent_name만 있고 agent_id가 비어있을 수 있다.
  useEffect(() => {
    if (!selectedNodeId) return;
    const nodeType = (originalData.nodeType as string) ?? '';
    if (nodeType !== 'bridge') return;

    const agentId = originalData.agent_id as string;
    const agentName = originalData.agent_name as string;
    if (agentId || !agentName || agents.length === 0) return;

    const matched = agents.find((a) => a.name === agentName);
    if (matched) {
      updateNodeData(selectedNodeId, {
        ...originalData,
        agent_id: matched.id,
        agent_type: matched.type,
      });
    }
  }, [selectedNodeId, originalData, agents, updateNodeData]);

  const hasChanges = useMemo(() => {
    return JSON.stringify(draft) !== JSON.stringify(originalData);
  }, [draft, originalData]);

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

    const nodeType = (draft.nodeType as string) ?? '';

    // bridge/switch/flow-node 노드: 설정 변경 시 포트 재계산.
    // flow-node 는 참조 플로우 포트(input_ports/output_ports)로부터 핸들을 파생한다.
    if (nodeType === 'bridge' || nodeType === 'switch' || nodeType === 'flow-node') {
      const newPorts = computePortsForNode(nodeType, draft);
      const updatedDraft = { ...draft, ports: newPorts };
      updateNodeData(selectedNodeId, updatedDraft);

      // 삭제된 포트에 연결된 엣지 자동 정리
      const oldPorts = (originalData.ports ?? []) as PortDef[];
      const removedHandleIds = oldPorts
        .filter(
          (op) =>
            !newPorts.some(
              (np) => np.name === op.name && np.direction === op.direction,
            ),
        )
        .map((p) => p.name);

      if (removedHandleIds.length > 0) {
        const { edges } = useEditorStore.getState();
        const filteredEdges = edges.filter((e) => {
          const isSource = e.source === selectedNodeId;
          const isTarget = e.target === selectedNodeId;
          if (isSource && removedHandleIds.includes(e.sourceHandle ?? '')) return false;
          if (isTarget && removedHandleIds.includes(e.targetHandle ?? '')) return false;
          return true;
        });
        if (filteredEdges.length !== edges.length) {
          useEditorStore.getState().setEdges(filteredEdges);
        }
      }
    } else if (nodeType === 'trigger') {
      // trigger 노드(v1.3.0): 단일 페이로드 에디터. 폐기된 payload_mode와 하위호환
      // payload_template 키를 제거하고, 페이로드는 `payload`(map)로 정규화 저장한다.
      // 에디터가 비어있으면(빈 오브젝트/미설정) payload 키를 제거해 기본 페이로드로 폴백.
      const { payload_mode: _pm, payload, payload_template: _pt, ...rest } =
        draft as {
          payload_mode?: unknown;
          payload?: unknown;
          payload_template?: unknown;
        } & Record<string, unknown>;
      const cleaned: Record<string, unknown> = { ...rest };
      if (isNonEmptyPayload(payload)) {
        cleaned.payload = payload;
      }
      updateNodeData(selectedNodeId, cleaned);
    } else {
      updateNodeData(selectedNodeId, draft);
    }
  }, [selectedNodeId, hasChanges, draft, originalData, updateNodeData]);

  /** 취소: 드래프트를 원본으로 되돌림 */
  const handleCancel = useCallback(() => {
    setDraft(originalData);
  }, [originalData]);

  // 에이전트 타입 해석: draft에 없으면 agent_name으로 조회
  // (hooks는 조건부 반환 이전에 호출해야 함)
  const agentType = useMemo(() => {
    const type = draft.agent_type as string;
    if (type) return type;
    const agentName = draft.agent_name as string;
    if (agentName && agents.length > 0) {
      const matched = agents.find((a) => a.name === agentName);
      if (matched) return matched.type;
    }
    return undefined;
  }, [draft.agent_type, draft.agent_name, agents]);

  // 선택된 노드가 없을 때 빈 상태
  if (!selectedNode) {
    return (
      <aside
        className="flex w-[300px] shrink-0 flex-col items-center justify-center
          border-l border-(--color-border-default) bg-(--color-bg-surface) p-4"
      >
        <Settings2 className="mb-2 h-8 w-8 text-(--color-border-strong)" />
        <p className="text-sm text-(--color-text-muted)">
          {t('editor.selectNode')}
        </p>
      </aside>
    );
  }

  const rawNodeType = (draft.nodeType as string) ?? (draft.type as string) ?? selectedNode.type ?? 'unknown';
  // 저장된 옛 `_` HVAC 타입도 canonical `-` 표기로 보여준다.
  const nodeType = normalizeNodeType(rawNodeType);
  const nodeLabel = (draft.label as string) ?? '';

  const configSchema = getConfigSchema(nodeType, agentType) ?? (draft.config_schema as ConfigSchema | undefined);
  const ports = (draft.ports ?? []) as Port[];

  // 타입 설명: NODE_TYPE_META 우선, 없으면 스키마 description.
  const typeDescription = NODE_TYPE_META[nodeType]?.description ?? getNodeDescription(nodeType);

  // 포트 이름 → 설명 맵.
  // NODE_TYPE_META 의 포트별 설명에 스키마의 방향별 입출력 메시지 설명
  // (inputDesc/outputDesc)을 합쳐 포트 ? 도움말 하나로 노출한다. 별도 인라인
  // IO 설명 블록 대신 포트 ? 로 일원화한다.
  // 이 블록은 위의 early return(노드 미선택) 이후이므로 hook 을 쓰지 않고 즉시 계산한다.
  const { inputDesc, outputDesc } = getNodeIODesc(nodeType, draft.direction as string | undefined);
  const metaPortDesc = new Map<string, string>();
  for (const p of NODE_TYPE_META[nodeType]?.ports ?? []) {
    if (p.description) metaPortDesc.set(p.name, p.description);
  }
  const portDescriptions: Record<string, string> = {};
  for (const port of ports) {
    const ioDesc =
      port.direction === 'input' ? inputDesc : port.direction === 'output' ? outputDesc : undefined;
    const parts = [metaPortDesc.get(port.name), ioDesc].filter((v): v is string => !!v);
    // 동일 문구 중복 노출 방지 후 단락 구분(\n\n)으로 합친다. FieldHelp 는
    // whitespace-pre-wrap 이라 줄바꿈이 그대로 렌더된다.
    const merged = [...new Set(parts)].join('\n\n');
    if (merged) portDescriptions[port.name] = merged;
  }

  // 필수 필드 누락 검사. draft 기준으로 계산하여 사용자가 값을 채우는 즉시 반영된다.
  const missingRequired = getRequiredFieldErrors(nodeType, draft, agentType);
  const hasMissingRequired = missingRequired.length > 0;

  return (
    <aside
      style={width ? { width: `${width}px` } : undefined}
      className={`flex shrink-0 flex-col border-l border-(--color-border-default)
        bg-(--color-bg-surface)
        ${width ? '' : 'w-[300px]'}`}
    >
      {/* 헤더: 타입/아이디 라벨 필드 + 닫기 버튼 */}
      <div className="flex items-start justify-between gap-2 border-b border-(--color-border-default) px-4 py-3">
        <div className="min-w-0 space-y-1.5">
          {/* 타입: 라벨 + canonical 타입 + 설명 ? 도움말 */}
          <div className="min-w-0">
            <div className="flex items-center gap-1">
              <span className="text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
                {t('property.panel.type')}
              </span>
              {typeDescription && (
                <FieldHelp text={typeDescription} describedById="node-type-desc" />
              )}
            </div>
            <p className="truncate text-sm font-semibold text-(--color-text-primary)">
              {nodeType}
            </p>
          </div>
          {/* 아이디: 라벨 + 노드 id */}
          <div className="min-w-0">
            <span className="text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
              {t('property.panel.id')}
            </span>
            <p className="truncate text-xs text-(--color-text-secondary)" title={selectedNode.id}>
              {selectedNode.id}
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={() => selectNode(null)}
          className="shrink-0 rounded p-1 text-(--color-text-muted) hover:bg-(--color-bg-sunken) hover:text-(--color-text-secondary)
            transition-colors"
          aria-label={t('property.panel.closeAria')}
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* 필수 필드 누락 경고 배너 */}
      {hasMissingRequired && (
        <div
          role="alert"
          className={cn(
            'flex items-start gap-2 border-b px-4 py-2 text-xs',
            'border-amber-300 bg-amber-50 text-amber-900',
            'dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-200',
          )}
        >
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <div className="space-y-0.5">
            <p className="font-medium">{t('property.panel.missingRequired')}</p>
            <ul className="list-disc pl-4">
              {missingRequired.map((e) => (
                <li key={e.name}>{e.label}</li>
              ))}
            </ul>
          </div>
        </div>
      )}

      {/* 속성 편집 영역 */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* 라벨 입력 */}
        <div className="space-y-1">
          <label
            htmlFor="node-label"
            className="block text-xs font-medium text-(--color-text-secondary)"
          >
            {t('property.panel.label')}
          </label>
          <input
            id="node-label"
            type="text"
            value={nodeLabel}
            onChange={(e) => handleDraftChange({ label: e.target.value })}
            placeholder={t('property.panel.labelPlaceholder')}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5
              text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted)
              focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400
              dark:focus:border-blue-500"
          />
        </div>

        {/* 활성화 토글 */}
        <div className="flex items-center justify-between">
          <label
            htmlFor="node-enabled"
            className="text-xs font-medium text-(--color-text-secondary)"
          >
            {t('property.panel.enabled')}
          </label>
          <button
            id="node-enabled"
            type="button"
            role="switch"
            aria-checked={draft.enabled !== false}
            onClick={() => handleDraftChange({ enabled: !(draft.enabled !== false) })}
            className={cn(
              'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors',
              draft.enabled !== false
                ? 'bg-blue-500 dark:bg-blue-600'
                : 'bg-(--color-border-strong)',
            )}
          >
            <span
              className={cn(
                'inline-block h-3.5 w-3.5 rounded-full bg-(--color-bg-surface) shadow transition-transform',
                draft.enabled !== false ? 'translate-x-4.5' : 'translate-x-0.5',
              )}
            />
          </button>
        </div>

        {/* 출력 미연결 경고 끄기 토글 (모든 노드 공통).
            ON 시 노드 config 에 suppress_unconnected_warning: true 를 기록한다.
            기본값(false)일 때는 명시적으로 false 를 기록한다(다른 공통 boolean 토글과 동일). */}
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1">
            <label
              htmlFor="node-suppress-unconnected-warning"
              className="text-xs font-medium text-(--color-text-secondary)"
            >
              {t('property.panel.suppressUnconnectedWarning')}
            </label>
            <FieldHelp
              text={t('property.panel.suppressUnconnectedWarningHelp')}
              describedById="node-suppress-unconnected-warning-desc"
            />
          </span>
          <button
            id="node-suppress-unconnected-warning"
            type="button"
            role="switch"
            aria-checked={draft.suppress_unconnected_warning === true}
            aria-describedby="node-suppress-unconnected-warning-desc"
            onClick={() =>
              handleDraftChange({
                suppress_unconnected_warning: draft.suppress_unconnected_warning !== true,
              })
            }
            className={cn(
              'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors',
              draft.suppress_unconnected_warning === true
                ? 'bg-blue-500 dark:bg-blue-600'
                : 'bg-(--color-border-strong)',
            )}
          >
            <span
              className={cn(
                'inline-block h-3.5 w-3.5 rounded-full bg-(--color-bg-surface) shadow transition-transform',
                draft.suppress_unconnected_warning === true ? 'translate-x-4.5' : 'translate-x-0.5',
              )}
            />
          </button>
        </div>

        {/* 포트 관리 */}
        <PortSection
          ports={ports}
          onChange={(newPorts) => handleDraftChange({ ports: newPorts })}
          portDescriptions={portDescriptions}
        />

        {/* 구분선 */}
        <hr className="border-(--color-border-default)" />

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
          className="flex items-center gap-2 border-t border-(--color-border-default) px-4 py-3"
        >
          <button
            type="button"
            onClick={handleApply}
            disabled={hasMissingRequired}
            title={hasMissingRequired ? t('property.panel.applyDisabledReason') : undefined}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5',
              'text-sm font-medium transition-colors',
              hasMissingRequired
                ? 'cursor-not-allowed bg-(--color-bg-sunken) text-(--color-text-muted)'
                : 'bg-blue-500 text-white hover:bg-blue-600 dark:bg-blue-600 dark:hover:bg-blue-700',
            )}
          >
            <Check className="h-3.5 w-3.5" />
            {t('property.panel.apply')}
          </button>
          <button
            type="button"
            onClick={handleCancel}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5',
              'text-sm font-medium transition-colors',
              'border border-(--color-border-strong) bg-(--color-bg-surface) text-(--color-text-secondary) hover:bg-(--color-bg-secondary)',
            )}
          >
            <RotateCcw className="h-3.5 w-3.5" />
            {t('property.panel.cancel')}
          </button>
        </div>
      )}
    </aside>
  );
}
