// MQTT 브릿지 어댑터 설정 섹션 컴포넌트.
// QoS 레벨, Retained 플래그, 발행 토픽 템플릿을 설정한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';

// --- 상수 ---

const QOS_OPTIONS = [
  { value: '0', label: 'QoS 0 - 최대 1회 전송' },
  { value: '1', label: 'QoS 1 - 최소 1회 전송' },
  { value: '2', label: 'QoS 2 - 정확히 1회 전송' },
] as const;

// --- 스타일 ---

const inputClass = cn(
  'w-full rounded-md border px-2.5 py-1.5 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'placeholder:text-gray-400',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:placeholder:text-gray-500 dark:focus:border-blue-500',
);

// readOnly 스타일.
//
// 주의: text 등 readOnly attr 를 지원하는 input 에는 `disabled` 가 아닌
// `readOnly` 를 사용한다. `disabled` 는 다크모드에서 텍스트를 흐리게 렌더링해
// 값이 거의 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 / 309966e 와 동일 패턴).
// select / checkbox 는 readOnly attr 미지원이므로 `disabled={readOnly}` 를 그대로 사용한다.
const readOnlyClass = 'cursor-not-allowed bg-(--color-bg-elevated)';

// --- Props ---

interface BridgeMqttConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeMqttConfig({ data, onChange, readOnly }: BridgeMqttConfigProps) {
  const qosId = useId();
  const retainedId = useId();
  const topicId = useId();

  const qos = String(data.default_qos ?? '0');
  const retained = Boolean(data.default_retained ?? false);
  const publishTopic = String(data.publish_topic ?? '');

  const handleChange = (field: string, value: unknown) => {
    onChange({ ...data, [field]: value });
  };

  return (
    <div className="space-y-3">
      {/* 섹션 헤더 */}
      <div className="flex items-center gap-2">
        <div className="h-px flex-1 bg-(--color-border-default)" />
        <span className="text-xs font-medium text-(--color-text-muted)">
          MQTT 설정
        </span>
        <div className="h-px flex-1 bg-(--color-border-default)" />
      </div>

      {/* QoS 레벨 */}
      <div className="space-y-1">
        <label
          htmlFor={qosId}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          QoS 레벨
        </label>
        <select
          id={qosId}
          value={qos}
          disabled={readOnly}
          onChange={(e) => handleChange('default_qos', e.target.value)}
          className={cn(inputClass, readOnly && readOnlyClass)}
        >
          {QOS_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-(--color-text-muted)">
          메시지 전달 보장 수준
        </p>
      </div>

      {/* Retained 플래그 */}
      <div className="space-y-1">
        <label
          htmlFor={retainedId}
          className="flex items-center gap-2"
        >
          <input
            id={retainedId}
            type="checkbox"
            checked={retained}
            disabled={readOnly}
            onChange={(e) => handleChange('default_retained', e.target.checked)}
            className={cn(
              'h-4 w-4 rounded border-gray-300 text-blue-500',
              'focus:ring-2 focus:ring-blue-400',
              'dark:border-gray-600 dark:bg-gray-800',
              readOnly && 'opacity-60 cursor-not-allowed',
            )}
          />
          <span className="text-xs font-medium text-(--color-text-secondary)">
            Retained 메시지
          </span>
        </label>
        <p className="text-xs text-(--color-text-muted)">
          브로커에 마지막 메시지를 유지합니다
        </p>
      </div>

      {/* 발행 토픽 */}
      <div className="space-y-1">
        <label
          htmlFor={topicId}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          발행 토픽
        </label>
        <input
          id={topicId}
          type="text"
          value={publishTopic}
          // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
          readOnly={readOnly}
          onChange={(e) => handleChange('publish_topic', e.target.value)}
          placeholder="devices/{device_id}/data"
          className={cn(inputClass, readOnly && readOnlyClass)}
        />
        <p className="text-xs text-(--color-text-muted)">
          {'토픽 템플릿 ({필드명} 형식으로 동적 치환)'}
        </p>
      </div>
    </div>
  );
}
