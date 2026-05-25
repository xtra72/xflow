// 타입별 폼 필드 렌더러 컴포넌트.
// ConfigField.type에 따라 적절한 입력 위젯을 렌더링한다.

import { useId, useMemo } from 'react';

import { useAgents } from '@/hooks/useAgent';
import type { ConfigField } from '@/types/node';
import { cn } from '@/lib/utils/cn';
import { RegisterMapEditor } from './RegisterMapEditor';
import { TransformPipelineEditor } from './TransformPipelineEditor';
import { KeyValueMapEditor } from './KeyValueMapEditor';
import { TypedKeyValueMapEditor } from './TypedKeyValueMapEditor';
import { StringListEditor } from './StringListEditor';
import { TriggerScheduleEditor } from './TriggerScheduleEditor';
import { CompareFieldsEditor } from './CompareFieldsEditor';

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
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'placeholder:text-gray-400',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:placeholder:text-gray-500 dark:focus:border-blue-500',
);

/** 에러 상태 스타일 */
const errorInputClass = cn(
  'border-red-400 focus:border-red-400 focus:ring-red-400',
  'dark:border-red-500 dark:focus:border-red-500 dark:focus:ring-red-500',
);

/** 읽기 전용 스타일.
 *
 * 주의: text/number/textarea 등 readOnly attr 를 지원하는 input 에는
 * `disabled` 가 아닌 `readOnly` 를 사용한다. `disabled` 를 적용하면
 * 브라우저가 텍스트를 흐리게(회색조로) 렌더링하여 다크모드에서
 * 값이 거의 보이지 않는 회귀가 발생한다. 따라서 가독성을 위해
 * 배경만 살짝 다르게 표시하고 opacity 는 낮추지 않는다.
 *
 * checkbox/select 는 readOnly attr 미지원이므로 별도 위치에서
 * `disabled={readOnly}` 를 그대로 사용한다. */
const readOnlyClass = 'cursor-not-allowed bg-(--color-bg-elevated)';

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
      {/* 레이블 (boolean은 체크박스 옆에 표시) */}
      {field.type !== 'boolean' && (
      <label
        htmlFor={id}
        className="block text-xs font-medium text-(--color-text-secondary)"
      >
        {field.label}
        {field.required && (
          <span className="ml-0.5 text-red-500" aria-hidden="true">
            *
          </span>
        )}
      </label>
      )}

      {/* 타입별 입력 위젯 */}
      {field.type === 'string' && (
        <input
          id={id}
          type="text"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.default != null ? String(field.default) : undefined}
          // readOnly attr 를 사용 — disabled 는 브라우저가 텍스트를
          // 흐리게 렌더링해 다크모드에서 값이 보이지 않게 만든다.
          readOnly={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        />
      )}

      {/* multiline: textarea + monospace 폰트. Lua 스크립트 등 다행 텍스트 입력 용도. */}
      {field.type === 'multiline' && (
        <textarea
          id={id}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.default != null ? String(field.default) : undefined}
          readOnly={readOnly}
          rows={12}
          spellCheck={false}
          className={cn(
            inputClass,
            'font-mono text-xs leading-snug whitespace-pre resize-y min-h-[12rem]',
            error && errorInputClass,
            readOnly && readOnlyClass,
          )}
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
          // readOnly attr 를 사용 — string field 와 동일한 이유.
          readOnly={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        />
      )}

      {field.type === 'boolean' && (
        <label className="flex items-center gap-2" htmlFor={id}>
          <input
            id={id}
            type="checkbox"
            // value 가 undefined 면 field.default 를 fallback 으로 사용.
            // string/number 는 placeholder 로 default 를 시각화하지만
            // checkbox 는 placeholder 가 없어 명시적 처리가 필요하다.
            checked={
              value === undefined && field.default !== undefined
                ? Boolean(field.default)
                : Boolean(value)
            }
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
          <span className="text-xs text-(--color-text-primary)">
            {field.label}
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
          agentTypes={field.options}
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
          // textarea 도 readOnly attr 지원 — 가독성 보존을 위해 사용.
          readOnly={readOnly}
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
          defaultMode={field.name === 'metadata_expression' ? 'merge' : 'select'}
        />
      )}

      {field.type === 'key_value_map' && (
        <KeyValueMapEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {field.type === 'typed_key_value_map' && (
        <TypedKeyValueMapEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {field.type === 'string_list' && (
        <StringListEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
          placeholder={field.description}
        />
      )}

      {field.type === 'trigger_schedules' && (
        <TriggerScheduleEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {field.type === 'compare_fields' && (
        <CompareFieldsEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {/* 설명 텍스트 */}
      {field.description && (
        <p
          id={descriptionId}
          className="text-xs text-(--color-text-muted)"
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
  agentTypes,
  onChange,
  className,
  ariaProps,
  readOnly,
}: {
  id: string;
  value: string;
  agentName?: string;
  /** 표시할 에이전트 타입 목록. 미지정 시 전체 표시 */
  agentTypes?: string[];
  onChange: (value: unknown) => void;
  className: string;
  ariaProps: Record<string, unknown>;
  readOnly?: boolean;
}) {
  const { data: agentsResult, isLoading } = useAgents();
  const allAgents = agentsResult?.data ?? [];
  const agents = agentTypes?.length
    ? allAgents.filter((a) => agentTypes.includes(a.type))
    : allAgents;

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
