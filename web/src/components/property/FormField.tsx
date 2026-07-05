// 타입별 폼 필드 렌더러 컴포넌트.
// ConfigField.type에 따라 적절한 입력 위젯을 렌더링한다.

import { useEffect, useId, useMemo, useState } from 'react';
import { Eye, EyeOff, RefreshCw } from 'lucide-react';

import { useAgents } from '@/hooks/useAgent';
import { useFlowsTarget } from '@/hooks/useResourceTargets';
import { useManagedNodes, useNodeLiveList, useRemoteMode } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import {
  buildRemoteFlowRef,
  parseRemoteFlowRef,
} from '@/lib/flow/subflowPorts';
import { resolveRemoteNodeLabel } from '@/lib/remote/nodeLabel';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { useEditorStore } from '@/stores/editorStore';
import type { ManagedNode } from '@/types/remote';
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
import { MetricsEditor } from './MetricsEditor';

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
  const { t } = useTranslation();
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
          // 값 미지정(undefined/null/빈 문자열)이고 비어있지 않은 default 가 있으면
          // 그 default 를 표시값으로 사용한다(boolean 필드의 default fallback 과 일관).
          // default 가 없거나 ''(의도적 빈 선택)면 기존 "선택..." 동작을 유지한다.
          // SPEC-SUBFLOW-002 W01/AC-1: flow-node `mode` 토글이 미지정 시 shared 로 표시된다.
          value={
            (value == null || value === '') &&
            field.default != null &&
            field.default !== ''
              ? String(field.default)
              : ((value as string) ?? '')
          }
          onChange={(e) => onChange(e.target.value)}
          disabled={readOnly}
          className={cn(inputClass, error && errorInputClass, readOnly && readOnlyClass)}
          {...ariaProps}
        >
          <option value="">{t('property.field.selectPlaceholder')}</option>
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

      {/* object_fields: 중첩 객체를 네이티브 위젯 섹션으로 편집한다(JSON textarea 아님).
          - 값은 이 필드의 name 키 아래 중첩 객체(config[name] = {sub.name: value, ...}).
          - 각 하위 필드는 최상위와 동일한 FormField 로 재귀 렌더링된다(boolean→체크박스,
            string→텍스트, select→드롭다운 등). 하위 필드의 visibleWhen 은 중첩 객체
            기준으로 평가한다.
          - 하위 값 변경 시 전체 객체를 불변(immutable) 복제해 onChange 로 올린다.
            → DynamicForm 의 기존 flat 쓰기(config[name] = 객체)가 그대로 중첩을 만든다.
            dotted 키를 절대 만들지 않는다. */}
      {field.type === 'object_fields' && (
        <ObjectFieldsGroup
          value={value}
          fields={field.fields ?? []}
          onChange={onChange}
          readOnly={readOnly}
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

      {field.type === 'metrics_editor' && (
        <MetricsEditor
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
  const { t } = useTranslation();
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
          aria-label={revealed ? t('property.field.hideValue') : t('property.field.revealValue')}
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
  const { t } = useTranslation();
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
        {isLoading ? t('property.field.agentLoading') : t('property.field.agentSelectPlaceholder')}
      </option>
      {agents.map((agent) => (
        <option key={agent.id} value={agent.id}>
          {agent.name} ({agent.type})
        </option>
      ))}
    </select>
  );
}

// 로컬 편집 전용 노드 선택값: 'local' 또는 원격 노드 instance_id.
const LOCAL_NODE_OPTION = 'local';

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
//
// 그룹 RU (SPEC-SUBFLOW-001 v1.2): 로컬 편집(target=local)일 때, 로컬 플로우뿐 아니라
// 승인+온라인 원격 노드의 플로우도 고를 수 있도록 노드 선택 드롭다운을 추가한다.
// 원격 노드의 플로우를 고르면 flow_id 를 `remote://{instanceId}/{flowId}` 로 정규화
// 저장하며(배포 시 백엔드가 인라인 확장), 선택값/목록에 "원격" 배지와 노드 라벨을
// 표시한다. 원격 관리 비활성(non-server)/원격 노드 없음 환경에서는 노드 선택이
// "로컬"만 제공하거나 숨겨져 기존 동작과 동일하게 회귀 없이 동작한다.
// 동일 노드 원격 편집(target=remote)은 이전 동작(그 노드 플로우, bare id) 그대로다.
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
  const { t } = useTranslation();

  // 타깃 인지 플로우 목록: 로컬이면 매니저 로컬 플로우, 원격이면 그 노드의 플로우.
  // useTargetContext 는 Provider 미설정 시 로컬을 기본값으로 돌려주므로, 로컬 편집
  // 콜사이트(Provider 없음/로컬 타깃)는 useFlows 위임과 동일하게 회귀 없이 동작한다.
  const target = useTargetContext();
  const editorIsRemote = isRemoteTarget(target);
  const { data: flowsResult, isLoading } = useFlowsTarget(target);
  const currentFlowId = useEditorStore((s) => s.currentFlowId);

  // 현재 값이 `remote://{instanceId}/{flowId}` 정규화 참조면 파싱한다(로컬 편집에서
  // 원격 노드 플로우를 참조한 경우). 평문 id 면 null(로컬/동일노드 참조).
  const remoteRef = useMemo(() => parseRemoteFlowRef(value), [value]);

  // --- 노드 선택 후보 (로컬 편집 전용, 그룹 RU) ---
  // server 모드가 아니면 admin /remote/* 쿼리를 막아 404 노이즈를 방지한다.
  // 동일노드 원격 편집(editorIsRemote)에서는 노드 선택을 노출하지 않으므로 쿼리도 끈다.
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';
  const nodeSelectorEnabled = !editorIsRemote && isServer;
  const { data: managedNodes } = useManagedNodes(undefined, nodeSelectorEnabled);

  // 승인 + 온라인 노드만 후보로 제공한다(오프라인/미승인은 플로우 프록시 불가).
  const remoteNodes = useMemo<ManagedNode[]>(
    () =>
      (managedNodes ?? []).filter(
        (n) => n.status === 'approved' && n.online,
      ),
    [managedNodes],
  );

  // 선택된 노드: 현재 값이 원격 참조면 그 노드, 아니면 'local' 기본값.
  // 사용자가 노드 드롭다운을 바꾸면 selectedNode 가 갱신된다.
  const [selectedNode, setSelectedNode] = useState<string>(
    remoteRef ? remoteRef.instanceId : LOCAL_NODE_OPTION,
  );

  // 값(remote ref)이 외부에서 바뀌면 선택 노드를 동기화한다(노드 사전 선택 — 그룹 RU).
  useEffect(() => {
    setSelectedNode(remoteRef ? remoteRef.instanceId : LOCAL_NODE_OPTION);
  }, [remoteRef]);

  // 원격 노드가 선택되었는지(로컬 편집에서만 의미). 동일노드 원격 편집은 항상 false.
  const remoteNodeSelected =
    !editorIsRemote && selectedNode !== LOCAL_NODE_OPTION;

  // 원격 노드 플로우 목록: 원격 노드가 선택된 경우에만 라이브 조회한다(Rules of Hooks
  // 를 위해 훅은 항상 호출하되 enabled 로 무력화). getRemoteFlowsLive → FlowInfo[].
  const remoteListInstance = remoteNodeSelected ? selectedNode : '';
  const { data: remoteFlows, isLoading: remoteLoading } = useNodeLiveList(
    remoteListInstance,
    'flow',
    remoteNodeSelected,
  );

  // 노드 라벨(원격 배지 옆 표시). 호스트명 우선, 없으면 단축 instanceId.
  // 후보 목록(remoteNodes)에 없으면(오프라인/비-server) instanceId 단축형으로 폴백한다.
  const selectedNodeLabel = useMemo(() => {
    if (!remoteNodeSelected) return '';
    const node = remoteNodes.find((n) => n.instance_id === selectedNode);
    return resolveRemoteNodeLabel(node?.hostname, selectedNode);
  }, [remoteNodeSelected, remoteNodes, selectedNode]);

  // 노드 선택 드롭다운 노출 여부(로컬 편집 + server 모드 + 원격 노드 존재).
  const showNodeSelector = nodeSelectorEnabled && remoteNodes.length > 0;

  // 자기참조 방지: 현재 편집 중인 플로우를 로컬 후보에서 제외한다 (REQ-SUBFLOW-C04).
  // 원격 노드의 플로우는 다른 스코프이므로 currentFlowId 로 제외하지 않는다(그룹 RU).
  const flows = useMemo(
    () => (flowsResult?.data ?? []).filter((f) => f.id !== currentFlowId),
    [flowsResult?.data, currentFlowId],
  );

  // --- 활성 옵션(로컬 vs 원격) ---
  // 원격 노드 선택 시: 원격 노드 플로우 목록 + 현재 select 값은 원격 flowId(bare).
  // 로컬/동일노드 편집 시: 로컬(또는 그 노드) 플로우 목록 + 현재 값 그대로.
  const optionFlows = remoteNodeSelected ? (remoteFlows ?? []) : flows;
  const optionsLoading = remoteNodeSelected ? remoteLoading : isLoading;

  // select 의 현재 값: 원격 참조면 ref 의 flowId(bare), 아니면 value 그대로.
  const selectValue = remoteNodeSelected
    ? remoteRef && remoteRef.instanceId === selectedNode
      ? remoteRef.flowId
      : ''
    : value;

  // 참조 플로우가 목록에 없으면(삭제됨/접근 불가) 끊어진 참조로 안내한다.
  const isDangling =
    selectValue !== '' && !optionFlows.some((f) => f.id === selectValue);

  // 노드 선택 변경 핸들러: 노드를 바꾸면 현재 선택을 초기화한다(플로우 미선택 상태).
  const handleNodeChange = (node: string) => {
    setSelectedNode(node);
    onChange({ flow_id: '', flow_name: '' });
  };

  // 플로우 선택 변경 핸들러: 원격 노드면 remote:// 정규화 저장, 로컬이면 bare id.
  const handleFlowChange = (flowId: string) => {
    const selected = optionFlows.find((f) => f.id === flowId);
    if (!selected) {
      onChange({ flow_id: '', flow_name: '' });
      return;
    }
    if (remoteNodeSelected) {
      // 원격 분기: 노드 라벨 자동 산출에 쓸 호스트 라벨(`xagent04` 등)을
      // 임시 키 remote_node_label 로 함께 전달한다. DynamicForm 에서 라벨
      // 계산(`{호스트}.{플로우}`)에만 사용하고 저장 데이터에는 남기지 않는다.
      onChange({
        flow_id: buildRemoteFlowRef(selectedNode, selected.id),
        flow_name: selected.name,
        remote_node_label: selectedNodeLabel,
      });
    } else {
      // 로컬 분기: remote_node_label 미포함(로컬 라벨은 기존대로 플로우 이름).
      onChange({ flow_id: selected.id, flow_name: selected.name });
    }
  };

  return (
    <div className="space-y-1">
      {/* 노드 선택 드롭다운(로컬 편집 + server 모드 + 원격 노드 존재 시에만 노출).
          원격 노드가 없으면 표시하지 않아 기존 로컬 UX 와 동일하게 동작한다. */}
      {showNodeSelector && (
        <div className="flex items-center gap-1.5">
          <select
            aria-label={t('flowPicker.nodeSelectorLabel')}
            value={selectedNode}
            disabled={readOnly}
            onChange={(e) => handleNodeChange(e.target.value)}
            className={cn(className, 'flex-1')}
          >
            <option value={LOCAL_NODE_OPTION}>{t('flowPicker.local')}</option>
            {remoteNodes.map((n) => (
              <option key={n.instance_id} value={n.instance_id}>
                {resolveRemoteNodeLabel(n.hostname, n.instance_id)}
              </option>
            ))}
          </select>
          {/* 선택된 노드가 원격이면 배지로 명확히 표시한다(RU04). */}
          {remoteNodeSelected && <RemoteBadge label={selectedNodeLabel} />}
        </div>
      )}

      <div className="flex items-center gap-1.5">
        <select
          id={id}
          value={selectValue}
          disabled={readOnly}
          onChange={(e) => handleFlowChange(e.target.value)}
          className={cn(className, 'flex-1')}
          {...ariaProps}
        >
          <option value="">
            {optionsLoading
              ? t('property.field.flowLoading')
              : remoteNodeSelected
                ? t('flowPicker.remoteFlowSelect')
                : t('property.field.flowSelectPlaceholder')}
          </option>
          {/* 끊어진 참조도 현재 값을 유지해 사용자가 인지할 수 있게 표시한다. */}
          {isDangling && (
            <option value={selectValue}>
              {`${flowName || selectValue} ${t('property.field.danglingSuffix')}`}
            </option>
          )}
          {optionFlows.map((flow) => (
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
            title={t('property.field.refreshPortsTitle')}
            aria-label={t('property.field.refreshPortsAria')}
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
        {/* 노드 선택기가 숨겨졌더라도(비-server / 후보 없음) 기존 remote:// 값은
            배지로 명확히 표시한다(RU04 — 기존 값 렌더링 보장). */}
        {!showNodeSelector && remoteNodeSelected && (
          <RemoteBadge label={selectedNodeLabel} />
        )}
      </div>
      {isDangling && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          {remoteNodeSelected
            ? t('flowPicker.remoteFlowMissing')
            : t('property.field.flowMissing')}
        </p>
      )}
    </div>
  );
}

// 중첩 객체(object_fields)를 네이티브 위젯 섹션으로 편집하는 컴포넌트.
//
// 값은 하나의 객체({ <sub.name>: value, ... })이며, 이 컴포넌트는 각 하위 필드를
// 최상위와 동일한 FormField 로 재귀 렌더링한다. 하위 값 변경 시 전체 객체를 불변
// 복제해 상위 onChange 로 올리므로, DynamicForm 의 기존 flat 쓰기가 그대로
// config[name] = { ... } 중첩 객체를 만든다(dotted 키 없음).
//
// 하위 필드 하나를 지우면(빈 문자열 등) 해당 키만 갱신되고 객체는 유효하게 유지된다.
function ObjectFieldsGroup({
  value,
  fields,
  onChange,
  readOnly,
}: {
  value: unknown;
  fields: ConfigField[];
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}) {
  // 현재 중첩 객체(없거나 객체가 아니면 빈 객체로 취급 — backward compat: 기존 노드는
  // 이미 객체를 들고 있으므로 그대로 읽힌다).
  const obj: Record<string, unknown> =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};

  // 하위 필드 값 변경: 전체 객체를 불변 복제해 한 키만 갱신 후 상위로 올린다.
  const handleSubChange = (subName: string, subValue: unknown) => {
    onChange({ ...obj, [subName]: subValue });
  };

  // 하위 visibleWhen 은 중첩 객체 기준으로 평가한다(최상위 data 가 아니라 obj).
  const visibleSubFields = fields.filter((sub) => {
    if (!sub.visibleWhen) return true;
    const actual = obj[sub.visibleWhen.field];
    if (sub.visibleWhen.notEmpty) return actual != null && actual !== '';
    const expected = sub.visibleWhen.value;
    if (Array.isArray(expected)) return expected.includes(actual);
    return actual === expected;
  });

  return (
    <div
      className={cn(
        'space-y-3 rounded-md border border-(--color-border-default)',
        'bg-(--color-bg-surface) p-3',
      )}
    >
      {visibleSubFields.map((sub) => (
        <FormField
          key={sub.name}
          field={sub}
          value={obj[sub.name]}
          onChange={(v) => handleSubChange(sub.name, v)}
          readOnly={readOnly}
        />
      ))}
    </div>
  );
}

// 원격 노드의 플로우가 선택되었음을 알리는 컴팩트 배지(RU04). 노드 라벨을 함께
// 표시하여 어느 노드의 플로우인지 사용자가 인지할 수 있게 한다.
function RemoteBadge({ label }: { label: string }) {
  const { t } = useTranslation();
  return (
    <span
      title={t('flowPicker.remoteBadgeTitle')}
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5',
        'text-[10px] font-medium',
        'bg-indigo-100 text-indigo-700',
        'dark:bg-indigo-900/40 dark:text-indigo-300',
      )}
    >
      <span>{t('flowPicker.remoteBadge')}</span>
      {label && <span className="opacity-80">· {label}</span>}
    </span>
  );
}
