// 스키마 기반 동적 폼 렌더러 컴포넌트.
// ConfigSchema가 있으면 타입별 필드를, 없으면 key-value 쌍으로 렌더링한다.
// advanced=true 로 표시된 필드는 접을 수 있는 "고급 설정" 섹션에 분리되어 렌더링된다.

import { useCallback, useEffect, useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { resolveFlowNodePorts } from '@/lib/flow/subflowPorts';
import type { ConfigSchema } from '@/types/node';

import { FormField } from './FormField';

interface DynamicFormProps {
  nodeId: string;
  data: Record<string, unknown>;
  schema?: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

export function DynamicForm({ nodeId, data, schema, onChange, readOnly }: DynamicFormProps) {
  // 로컬 폼 상태 관리 (nodeId 변경 시 리셋)
  const [localData, setLocalData] = useState<Record<string, unknown>>(data);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // 노드 변경 또는 취소(원본 복원) 시 로컬 상태 동기화
  useEffect(() => {
    setLocalData(data);
    setErrors({});
  }, [nodeId, data]);

  /**
   * flow-node (flow_picker) 의 참조 플로우 포트를 비정규화하여 병합한다.
   * 참조 플로우 정의를 조회해 input_ports / output_ports / flow_name 을 채운다.
   * 이 값들은 핸들 렌더링(computePortsForNode) 에만 쓰이는 에디터 표시 전용 캐시이며,
   * 백엔드는 배포 시점에 참조 플로우 정의에서 포트를 재해석한다(SPEC-SUBFLOW-001).
   */
  const denormalizeFlowNodePorts = useCallback(
    async (flowId: string, base: Record<string, unknown>) => {
      if (!flowId) return;
      try {
        const resolved = await resolveFlowNodePorts(flowId);
        // 조회 도중 다른 플로우로 선택이 바뀌었으면 무시한다(stale 방지).
        setLocalData((prev) => {
          if ((prev.flow_id as string) !== flowId) return prev;
          const merged = { ...prev, ...resolved };
          onChange(merged);
          return merged;
        });
      } catch {
        // 조회 실패(삭제/네트워크 등) 시 포트 캐시를 비워 dangling 으로 둔다.
        setLocalData((prev) => {
          if ((prev.flow_id as string) !== flowId) return prev;
          const merged = { ...prev, input_ports: [], output_ports: [] };
          onChange(merged);
          return merged;
        });
      }
      void base;
    },
    [onChange],
  );

  /** 필드 값 변경 핸들러 */
  const handleFieldChange = useCallback(
    (fieldName: string, value: unknown) => {
      let updated: Record<string, unknown>;

      // agent_select 타입은 { agent_id, agent_name, agent_type } 복합 객체를 반환한다.
      // 필드 이름(예: agent_ref)에 agent_id 값을 저장하고, agent_name/agent_type도 병합한다.
      const field = schema?.fields.find((f) => f.name === fieldName);
      if (
        field?.type === 'agent_select' &&
        typeof value === 'object' &&
        value !== null
      ) {
        const compound = value as Record<string, unknown>;
        updated = {
          ...localData,
          [fieldName]: compound.agent_id ?? '',
          agent_id: compound.agent_id ?? '',
          agent_name: compound.agent_name ?? '',
          agent_type: compound.agent_type ?? '',
        };
      } else if (
        field?.type === 'flow_picker' &&
        typeof value === 'object' &&
        value !== null
      ) {
        // flow_picker 는 { flow_id, flow_name } 복합 객체를 반환한다.
        // flow_id 를 저장하고, 이전 포트 캐시는 즉시 비워(핸들 깜빡임 방지) 후
        // 비동기로 참조 플로우 포트를 비정규화한다.
        const compound = value as Record<string, unknown>;
        const flowId = (compound.flow_id as string) ?? '';
        updated = {
          ...localData,
          [fieldName]: flowId,
          flow_name: (compound.flow_name as string) ?? '',
          input_ports: [],
          output_ports: [],
        };
        setLocalData(updated);
        onChange(updated);
        if (field.required && flowId === '') {
          setErrors((prev) => ({ ...prev, [fieldName]: '필수 항목입니다' }));
        } else {
          setErrors((prev) => {
            const next = { ...prev };
            delete next[fieldName];
            return next;
          });
        }
        void denormalizeFlowNodePorts(flowId, updated);
        return;
      } else {
        updated = { ...localData, [fieldName]: value };
      }

      setLocalData(updated);

      // 필수 필드 검증
      if (field?.required) {
        const checkValue =
          field.type === 'agent_select' && typeof value === 'object' && value !== null
            ? (value as Record<string, unknown>).agent_id
            : value;
        if (checkValue === '' || checkValue == null) {
          setErrors((prev) => ({ ...prev, [fieldName]: '필수 항목입니다' }));
        } else {
          setErrors((prev) => {
            const next = { ...prev };
            delete next[fieldName];
            return next;
          });
        }
      } else {
        setErrors((prev) => {
          const next = { ...prev };
          delete next[fieldName];
          return next;
        });
      }

      onChange(updated);
    },
    [localData, onChange, schema, denormalizeFlowNodePorts],
  );

  /** flow_picker "포트 갱신": 현재 flow_id 로 참조 플로우 포트를 재조회한다. */
  const handleFlowPortsRefresh = useCallback(() => {
    const flowId = (localData.flow_id as string) ?? '';
    void denormalizeFlowNodePorts(flowId, localData);
  }, [localData, denormalizeFlowNodePorts]);

  // 스키마가 있는 경우: 스키마 필드 기반 렌더링
  if (schema && schema.fields.length > 0) {
    // visibleWhen 조건에 따라 필드 필터링 (value 비교 또는 notEmpty 검사)
    const visibleFields = schema.fields.filter((field) => {
      if (!field.visibleWhen) return true;
      const actual = localData[field.visibleWhen.field];
      if (field.visibleWhen.notEmpty) {
        return actual != null && actual !== '';
      }
      const expected = field.visibleWhen.value;
      if (Array.isArray(expected)) return expected.includes(actual);
      return actual === expected;
    });

    // advanced 플래그로 기본/고급 필드 분리
    const basicFields = visibleFields.filter((f) => !f.advanced);
    const advancedFields = visibleFields.filter((f) => f.advanced);

    const renderField = (field: (typeof visibleFields)[number]) => (
      <FormField
        key={field.name}
        field={field}
        value={localData[field.name]}
        agentName={field.type === 'agent_select' ? (localData['agent_name'] as string) : undefined}
        flowName={field.type === 'flow_picker' ? (localData['flow_name'] as string) : undefined}
        onFlowPortsRefresh={field.type === 'flow_picker' ? handleFlowPortsRefresh : undefined}
        onChange={(v) => handleFieldChange(field.name, v)}
        error={errors[field.name]}
        readOnly={readOnly}
      />
    );

    return (
      <div className="space-y-3">
        {basicFields.map(renderField)}

        {advancedFields.length > 0 && (
          <AdvancedSection>
            {advancedFields.map(renderField)}
          </AdvancedSection>
        )}
      </div>
    );
  }

  // 스키마가 없는 경우: key-value 쌍으로 렌더링
  // 내부 속성(React Flow 노드 메타데이터)은 PropertyPanel에서 별도 처리
  const INTERNAL_KEYS = new Set(['label', 'ports', 'nodeType', 'category', 'icon', 'status']);

  const entries = Object.entries(localData).filter(
    ([key]) => !INTERNAL_KEYS.has(key),
  );

  if (entries.length === 0) {
    return (
      <p className="text-xs text-(--color-text-muted)">
        설정 항목이 없습니다
      </p>
    );
  }

  return (
    <div className="space-y-3">
      {entries.map(([key, val]) => (
        <FormField
          key={key}
          field={{
            name: key,
            type: typeof val === 'boolean' ? 'boolean' : typeof val === 'number' ? 'number' : (typeof val === 'object' && val !== null) ? 'object' : 'string',
            label: key,
          }}
          value={val}
          onChange={(v) => handleFieldChange(key, v)}
          readOnly={readOnly}
        />
      ))}
    </div>
  );
}

// --- 고급 설정 섹션 ---

/** 고급 필드를 감싸는 접을 수 있는 섹션. 기본 접힘 상태로 표시된다. */
function AdvancedSection({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="rounded-md border border-dashed border-(--color-border-default)">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'flex w-full items-center gap-1 px-2 py-1.5 text-xs font-medium',
          'text-(--color-text-secondary) hover:text-(--color-text-primary)',
          'transition-colors',
        )}
        aria-expanded={open}
      >
        {open ? (
          <ChevronDown className="h-3.5 w-3.5" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5" />
        )}
        고급 설정
      </button>
      {open && (
        <div className="space-y-3 border-t border-(--color-border-default) px-2 py-3">
          {children}
        </div>
      )}
    </div>
  );
}
