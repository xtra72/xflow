// 스키마 기반 동적 폼 렌더러 컴포넌트.
// ConfigSchema가 있으면 타입별 필드를, 없으면 key-value 쌍으로 렌더링한다.
// advanced=true 로 표시된 필드는 접을 수 있는 "고급 설정" 섹션에 분리되어 렌더링된다.

import { useCallback, useEffect, useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
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
    [localData, onChange, schema],
  );

  // 스키마가 있는 경우: 스키마 필드 기반 렌더링
  if (schema && schema.fields.length > 0) {
    // visibleWhen 조건에 따라 필드 필터링 (value가 배열이면 OR 조건)
    const visibleFields = schema.fields.filter((field) => {
      if (!field.visibleWhen) return true;
      const actual = localData[field.visibleWhen.field];
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
