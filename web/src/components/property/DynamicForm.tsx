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
}

export function DynamicForm({ nodeId, data, schema, onChange }: DynamicFormProps) {
  // 로컬 폼 상태 관리 (nodeId 변경 시 리셋)
  const [localData, setLocalData] = useState<Record<string, unknown>>(data);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // 선택된 노드가 바뀌면 로컬 상태 동기화
  useEffect(() => {
    setLocalData(data);
    setErrors({});
  }, [nodeId, data]);

  /** 필드 값 변경 핸들러 */
  const handleFieldChange = useCallback(
    (fieldName: string, value: unknown) => {
      const updated = { ...localData, [fieldName]: value };
      setLocalData(updated);

      // 필수 필드 검증
      if (schema) {
        const field = schema.fields.find((f) => f.name === fieldName);
        if (field?.required && (value === '' || value == null)) {
          setErrors((prev) => ({ ...prev, [fieldName]: '필수 항목입니다' }));
        } else {
          setErrors((prev) => {
            const next = { ...prev };
            delete next[fieldName];
            return next;
          });
        }
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
            onChange={(v) => handleFieldChange(field.name, v)}
            error={errors[field.name]}
          />
        ))}
      </div>
    );
  }

  // 스키마가 없는 경우: key-value 쌍으로 렌더링
  const entries = Object.entries(localData).filter(
    // label 등 내부 속성은 PropertyPanel에서 별도 처리
    ([key]) => key !== 'label',
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
            type: typeof val === 'boolean' ? 'boolean' : typeof val === 'number' ? 'number' : 'string',
            label: key,
          }}
          value={val}
          onChange={(v) => handleFieldChange(key, v)}
        />
      ))}
    </div>
  );
}
