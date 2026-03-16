// 스키마 기반 동적 폼 렌더러 컴포넌트.
// ConfigSchema가 있으면 타입별 필드를, 없으면 key-value 쌍으로 렌더링한다.

import { useCallback, useEffect, useState } from 'react';

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
    return (
      <div className="space-y-3">
        {schema.fields.map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={localData[field.name]}
            agentName={field.type === 'agent_select' ? (localData['agent_name'] as string) : undefined}
            onChange={(v) => handleFieldChange(field.name, v)}
            error={errors[field.name]}
            readOnly={readOnly}
          />
        ))}
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
      <p className="text-xs text-gray-400 dark:text-gray-500">
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
