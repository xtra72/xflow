// 타입별 폼 필드 렌더러 컴포넌트.
// ConfigField.type에 따라 적절한 입력 위젯을 렌더링한다.

import { useId, useMemo, useState } from 'react';
import { Eye, EyeOff, RefreshCw } from 'lucide-react';

import { useAgents } from '@/hooks/useAgent';
import { useFlowsTarget } from '@/hooks/useResourceTargets';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { useEditorStore } from '@/stores/editorStore';
import type { ConfigField } from '@/types/node';
import { cn } from '@/lib/utils/cn';
import { RegisterMapEditor } from './RegisterMapEditor';
import { TransformPipelineEditor } from './TransformPipelineEditor';
import { KeyValueMapEditor } from './KeyValueMapEditor';
import { TypedKeyValueMapEditor } from './TypedKeyValueMapEditor';
import { StringListEditor } from './StringListEditor';
import { TriggerScheduleEditor } from './TriggerScheduleEditor';
import { FieldHelp } from './FieldHelp';
import { CompareFieldsEditor } from './CompareFieldsEditor';
import { RoutesEditor } from './RoutesEditor';

interface FormFieldProps {
  field: ConfigField;
  value: unknown;
  onChange: (value: unknown) => void;
  error?: string;
  /** agent_select 타입 필드에서 ID 미설정 시 이름 기반 매칭에 사용 */
  agentName?: string;
  /** flow_picker 타입 필드: 노드 카드 표시용 참조 플로우 이름 (denormalize). */
  flowName?: string;
  /** flow_picker 타입 필드: "포트 갱신" 클릭 시 참조 플로우 포트를 재조회한다. */
  onFlowPortsRefresh?: () => void;
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

export function FormField({ field, value, onChange, error, agentName, flowName, onFlowPortsRefresh, readOnly }: FormFieldProps) {
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
      {/* 레이블 (boolean은 체크박스 옆에 표시) — 설명은 라벨 뒤 ? 아이콘 클릭 시 표시 */}
      {field.type !== 'boolean' && (
      <div className="flex items-center gap-1">
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
        {field.description && (
          <FieldHelp text={field.description} describedById={descriptionId} />
        )}
      </div>
      )}

      {/* 타입별 입력 위젯 */}
      {/* 비밀(sensitive) 문자열 필드: password 입력 + 표시/숨김 토글 */}
      {field.type === 'string' && field.sensitive && (
        <SensitiveStringInput
          id={id}
          value={(value as string) ?? ''}
          onChange={onChange}
          // field.placeholder 우선, 없으면 기존 동작(default 값 표시) 유지.
          placeholder={field.placeholder ?? (field.default != null ? String(field.default) : undefined)}
          error={error}
          readOnly={readOnly}
          ariaProps={ariaProps}
        />
      )}

      {/* 일반 문자열 필드 */}
      {field.type === 'string' && !field.sensitive && (
        <input
          id={id}
          type="text"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          // field.placeholder 우선, 없으면 기존 동작(default 값 표시) 유지.
          placeholder={field.placeholder ?? (field.default != null ? String(field.default) : undefined)}
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
        <div className="flex items-center gap-1">
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
        {field.description && (
          <FieldHelp text={field.description} describedById={descriptionId} />
        )}
        </div>
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

      {field.type === 'flow_picker' && (
        <FlowPickerInput
          id={id}
          value={(value as string) ?? ''}
          flowName={flowName}
          onChange={onChange}
          onRefresh={onFlowPortsRefresh}
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
          keyLabel={field.keyLabel}
          valueLabel={field.valueLabel}
          keyPlaceholder={field.keyPlaceholder}
          valuePlaceholder={field.valuePlaceholder}
          pathHelper={field.pathHelper}
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

      {field.type === 'routes_editor' && (
        <RoutesEditor
          value={value}
          onChange={onChange}
          readOnly={readOnly}
        />
      )}

      {/* 설명은 라벨 뒤 ? 아이콘(FieldHelp) 클릭 시 표시한다.
          (sr-only 설명은 FieldHelp 내부에 descriptionId 로 유지하여 접근성 보존) */}

      {/* 에러 메시지 */}
      {error && (
        <p id={errorId} className="text-xs text-red-500 dark:text-red-400">
          {error}
        </p>
      )}
    </div>
  );
}

// 비밀(sensitive) 문자열 입력 컴포넌트.
// 비밀번호/토큰 등의 값을 password 타입으로 마스킹하고, 우측의 눈 아이콘
// 버튼으로 표시/숨김을 토글한다. readOnly 모드에서는 토글 버튼을 숨긴다.
function SensitiveStringInput({
  id,
  value,
  onChange,
  placeholder,
  error,
  readOnly,
  ariaProps,
}: {
  id: string;
  value: string;
  onChange: (value: unknown) => void;
  placeholder?: string;
  error?: string;
  readOnly?: boolean;
  ariaProps: Record<string, unknown>;
}) {
  const [revealed, setRevealed] = useState(false);

  return (
    <div className="relative">
      <input
        id={id}
        type={revealed ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        readOnly={readOnly}
        autoComplete="off"
        // 우측 토글 버튼과 겹치지 않도록 padding-right 확보.
        className={cn(
          inputClass,
          'pr-9',
          error && errorInputClass,
          readOnly && readOnlyClass,
        )}
        {...ariaProps}
      />
      {!readOnly && (
        <button
          type="button"
          onClick={() => setRevealed((v) => !v)}
          tabIndex={-1}
          aria-label={revealed ? '값 숨기기' : '값 표시'}
          className={cn(
            'absolute inset-y-0 right-0 flex items-center px-2.5',
            'text-(--color-text-muted) hover:text-(--color-text-primary)',
            'transition-colors',
          )}
        >
          {revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
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

// 참조 플로우를 선택하는 드롭다운 컴포넌트 (SPEC-SUBFLOW-001 그룹 C, flow-node 전용).
// - 타깃 인지(target-aware) 플로우 목록으로 후보를 채운다(SPEC-REMOTE-001):
//   로컬 편집이면 매니저 로컬 플로우(useFlows 동형), 원격 노드 플로우 편집이면 그
//   노드의 플로우 목록(live + mirror 폴백)을 나열한다. 따라서 원격 편집 시 서브플로우
//   picker 는 매니저가 아닌 "그 노드"의 플로우를 보여주며, 선택된 flow_id 는 그 노드에
//   존재하는 플로우를 가리켜 배포 시 노드에서 네이티브로 해석된다.
// - 현재 편집 중인 플로우(currentFlowId)는 자기참조 방지를 위해 후보에서 제외한다
//   (REQ-SUBFLOW-C04). 백엔드도 순환을 거부하지만 명백한 자기 선택은 UI 에서 막는다.
// - 선택 시 { flow_id, flow_name } 복합 객체를 반환한다(DynamicForm 이 포트 비정규화 수행).
// - 우측 "포트 갱신" 버튼으로 참조 플로우 포트를 재조회한다(REQ-SUBFLOW-C03 항상 최신).
function FlowPickerInput({
  id,
  value,
  flowName,
  onChange,
  onRefresh,
  className,
  ariaProps,
  readOnly,
}: {
  id: string;
  value: string;
  flowName?: string;
  onChange: (value: unknown) => void;
  onRefresh?: () => void;
  className: string;
  ariaProps: Record<string, unknown>;
  readOnly?: boolean;
}) {
  // 타깃 인지 플로우 목록: 로컬이면 매니저 로컬 플로우, 원격이면 그 노드의 플로우.
  // useTargetContext 는 Provider 미설정 시 로컬을 기본값으로 돌려주므로, 로컬 편집
  // 콜사이트(Provider 없음/로컬 타깃)는 useFlows 위임과 동일하게 회귀 없이 동작한다.
  const target = useTargetContext();
  const { data: flowsResult, isLoading } = useFlowsTarget(target);
  const currentFlowId = useEditorStore((s) => s.currentFlowId);

  // 자기참조 방지: 현재 편집 중인 플로우를 후보에서 제외한다 (REQ-SUBFLOW-C04).
  // 원격 편집은 currentFlowId 가 null 이므로(라이브 제어 비대상) 제외가 무효지만,
  // 원격 picker 는 그 노드의 다른 플로우만 나열하면 충분하다.
  const flows = useMemo(
    () => (flowsResult?.data ?? []).filter((f) => f.id !== currentFlowId),
    [flowsResult?.data, currentFlowId],
  );

  // 참조 플로우가 목록에 없으면(삭제됨/접근 불가) 끊어진 참조로 안내한다.
  const isDangling = value !== '' && !flows.some((f) => f.id === value);

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1.5">
        <select
          id={id}
          value={value}
          disabled={readOnly}
          onChange={(e) => {
            const selected = flows.find((f) => f.id === e.target.value);
            if (selected) {
              onChange({ flow_id: selected.id, flow_name: selected.name });
            } else {
              onChange({ flow_id: '', flow_name: '' });
            }
          }}
          className={cn(className, 'flex-1')}
          {...ariaProps}
        >
          <option value="">
            {isLoading ? '로딩 중...' : '플로우 선택...'}
          </option>
          {/* 끊어진 참조도 현재 값을 유지해 사용자가 인지할 수 있게 표시한다. */}
          {isDangling && (
            <option value={value}>
              {flowName ? `${flowName} (참조 끊김)` : `${value} (참조 끊김)`}
            </option>
          )}
          {flows.map((flow) => (
            <option key={flow.id} value={flow.id}>
              {flow.name}
            </option>
          ))}
        </select>
        {/* 포트 갱신: 참조 플로우 정의를 재조회하여 핸들을 최신화한다.
            (항상-최신은 배포 시점에 강제되며, 에디터 뷰는 캐시 스냅샷이다.) */}
        {!readOnly && onRefresh && (
          <button
            type="button"
            onClick={onRefresh}
            disabled={value === ''}
            title="참조 플로우의 포트를 다시 불러옵니다"
            aria-label="포트 갱신"
            className={cn(
              'flex shrink-0 items-center justify-center rounded-md border px-2 py-1.5',
              'border-(--color-border-default) bg-(--color-bg-surface)',
              'text-(--color-text-secondary) transition-colors',
              'hover:bg-gray-50 dark:hover:bg-gray-700',
              value === '' && 'cursor-not-allowed opacity-50',
            )}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      {isDangling && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          참조 플로우를 찾을 수 없습니다. 삭제되었거나 접근할 수 없습니다.
        </p>
      )}
    </div>
  );
}
