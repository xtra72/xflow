// 타입별 폼 필드 렌더러 컴포넌트.
// ConfigField.type에 따라 적절한 입력 위젯을 렌더링한다.

import { useId, useMemo } from 'react';

import { useAgents } from '@/hooks/useAgent';
import type { ConfigField } from '@/types/node';
import { cn } from '@/lib/utils/cn';
import { RegisterMapEditor } from './RegisterMapEditor';
import { TransformPipelineEditor } from './TransformPipelineEditor';

interface FormFieldProps {
  field: ConfigField;
  value: unknown;
  onChange: (value: unknown) => void;
  error?: string;
  /** agent_select 타입 필드에서 ID 미설정 시 이름 기반 매칭에 사용 */
  agentName?: string;
  /** 읽기 전용 모드 */
  readOnly?: boolean;
}

/** 공통 입력 스타일 */
const inputClass = cn(
  'w-full rounded-md border px-2.5 py-1.5 text-sm',
  'border-gray-200 bg-white text-gray-900',
  'placeholder:text-gray-400',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
  'dark:placeholder:text-gray-500 dark:focus:border-blue-500',
);

/** 에러 상태 스타일 */
const errorInputClass = cn(
  'border-red-400 focus:border-red-400 focus:ring-red-400',
  'dark:border-red-500 dark:focus:border-red-500 dark:focus:ring-red-500',
);

/** 읽기 전용 스타일 */
const readOnlyClass = 'opacity-60 cursor-not-allowed bg-gray-50 dark:bg-gray-900';

export function FormField({ field, value, onChange, error, agentName, readOnly }: FormFieldProps) {
  const id = useId();
  const descriptionId = `${id}-desc`;
  const errorId = `${id}-error`;

  // aria 속성 구성
  const ariaProps = {
    'aria-describedby': cn(
      field.description ? descriptionId : undefined,
      error ? errorId : undefined,
    ) || undefined,
    'aria-invalid': error ? (true as const) : undefined,
  };

  return (
    <div className="space-y-1">
      {/* 레이블 */}
      <label
        htmlFor={id}
        className="block text-xs font-medium text-gray-700 dark:text-gray-300"
      >
        {field.label}
        {field.required && (
          <span className="ml-0.5 text-red-500" aria-hidden="true">
            *
          </span>
        )}
      </label>

      {/* 타입별 입력 위젯 */}
      {field.type === 'string' && (
        <input
          id={id}
          type="text"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.default != null ? String(field.default) : undefined}
          disabled={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        />
      )}

      {field.type === 'number' && (
        <input
          id={id}
          type="number"
          step="any"
          value={value != null ? Number(value) : ''}
          onChange={(e) =>
            onChange(e.target.value === '' ? undefined : Number(e.target.value))
          }
          placeholder={field.default != null ? String(field.default) : undefined}
          disabled={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        />
      )}

      {field.type === 'boolean' && (
        <label className="flex items-center gap-2" htmlFor={id}>
          <input
            id={id}
            type="checkbox"
            checked={Boolean(value)}
            onChange={(e) => onChange(e.target.checked)}
            disabled={readOnly}
            className={cn(
              'h-4 w-4 rounded border-gray-300 text-blue-500',
              'focus:ring-2 focus:ring-blue-400',
              'dark:border-gray-600 dark:bg-gray-800',
              readOnly && 'opacity-60 cursor-not-allowed',
            )}
            {...ariaProps}
          />
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {value ? '활성' : '비활성'}
          </span>
        </label>
      )}

      {field.type === 'select' && (
        <select
          id={id}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          disabled={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        >
          <option value="">선택...</option>
          {field.options?.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      )}

      {field.type === 'agent_select' && (
        <AgentSelectInput
          id={id}
          value={(value as string) ?? ''}
          agentName={agentName}
          onChange={onChange}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          ariaProps={ariaProps}
          readOnly={readOnly}
        />
      )}

      {field.type === 'object' && (
        <textarea
          id={id}
          rows={4}
          value={
            typeof value === 'string' ? value : JSON.stringify(value ?? {}, null, 2)
          }
          onChange={(e) => {
            try {
              onChange(JSON.parse(e.target.value) as unknown);
            } catch {
              // JSON 파싱 실패 시 문자열 그대로 저장
              onChange(e.target.value);
            }
          }}
          disabled={readOnly}
          className={cn(
            inputClass,
            'font-mono text-xs',
            error && errorInputClass,
            readOnly && readOnlyClass,
          )}
          {...ariaProps}
        />
      )}

      {field.type === 'register_map' && (
        <RegisterMapEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {field.type === 'transform_pipeline' && (
        <TransformPipelineEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {/* 설명 텍스트 */}
      {field.description && (
        <p
          id={descriptionId}
          className="text-xs text-gray-400 dark:text-gray-500"
        >
          {field.description}
        </p>
      )}

      {/* 에러 메시지 */}
      {error && (
        <p id={errorId} className="text-xs text-red-500 dark:text-red-400">
          {error}
        </p>
      )}
    </div>
  );
}

// 에이전트 목록에서 선택하는 드롭다운 컴포넌트.
// 선택 시 { agent_id, agent_name, agent_type } 복합 객체를 반환한다.
// agent_id 가 비어있고 agentName 이 있으면 이름 기반으로 매칭한다.
function AgentSelectInput({
  id,
  value,
  agentName,
  onChange,
  className,
  ariaProps,
  readOnly,
}: {
  id: string;
  value: string;
  agentName?: string;
  onChange: (value: unknown) => void;
  className: string;
  ariaProps: Record<string, unknown>;
  readOnly?: boolean;
}) {
  const { data: agentsResult, isLoading } = useAgents();
  const agents = agentsResult?.data ?? [];

  // agent_id 가 비어있으면 agent_name 으로 ID를 찾아 매칭한다
  const resolvedValue = useMemo(() => {
    if (value && agents.some((a) => a.id === value)) return value;
    if (agentName) {
      const matched = agents.find((a) => a.name === agentName);
      if (matched) return matched.id;
    }
    return value;
  }, [value, agentName, agents]);

  return (
    <select
      id={id}
      value={resolvedValue}
      disabled={readOnly}
      onChange={(e) => {
        const selected = agents.find((a) => a.id === e.target.value);
        if (selected) {
          onChange({ agent_id: selected.id, agent_name: selected.name, agent_type: selected.type });
        } else {
          onChange({ agent_id: '', agent_name: '', agent_type: '' });
        }
      }}
      className={className}
      {...ariaProps}
    >
      <option value="">
        {isLoading ? '로딩 중...' : '에이전트 선택...'}
      </option>
      {agents.map((agent) => (
        <option key={agent.id} value={agent.id}>
          {agent.name} ({agent.type})
        </option>
      ))}
    </select>
  );
}
