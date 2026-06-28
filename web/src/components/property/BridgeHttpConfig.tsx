// HTTP 브릿지 어댑터 설정 섹션 컴포넌트.
// Content-Type, URL 템플릿, 타임아웃을 설정한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// --- 상수 ---

const CONTENT_TYPES = [
  { value: 'application/json', label: 'application/json' },
  { value: 'text/plain', label: 'text/plain' },
  { value: 'application/x-www-form-urlencoded', label: 'application/x-www-form-urlencoded' },
  { value: 'multipart/form-data', label: 'multipart/form-data' },
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
// 주의: text/number 등 readOnly attr 를 지원하는 input 에는 `disabled` 가 아닌
// `readOnly` 를 사용한다. `disabled` 는 다크모드에서 텍스트를 흐리게 렌더링해
// 값이 거의 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 / 309966e 와 동일 패턴).
// select 는 readOnly attr 미지원이므로 `disabled={readOnly}` 를 그대로 사용한다.
const readOnlyClass = 'cursor-not-allowed bg-(--color-bg-elevated)';

// --- Props ---

interface BridgeHttpConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeHttpConfig({ data, onChange, readOnly }: BridgeHttpConfigProps) {
  const { t } = useTranslation();
  const contentTypeId = useId();
  const urlTemplateId = useId();
  const timeoutId = useId();

  const contentType = String(data.content_type ?? 'application/json');
  const urlTemplate = String(data.url_template ?? '');
  const timeoutMs = data.timeout_ms != null ? Number(data.timeout_ms) : 5000;

  const handleChange = (field: string, value: unknown) => {
    onChange({ ...data, [field]: value });
  };

  return (
    <div className="space-y-3">
      {/* 섹션 헤더 */}
      <div className="flex items-center gap-2">
        <div className="h-px flex-1 bg-(--color-border-default)" />
        <span className="text-xs font-medium text-(--color-text-muted)">
          {t('bridge.httpSettings')}
        </span>
        <div className="h-px flex-1 bg-(--color-border-default)" />
      </div>

      {/* Content-Type */}
      <div className="space-y-1">
        <label
          htmlFor={contentTypeId}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          {t('bridge.contentType')}
        </label>
        <select
          id={contentTypeId}
          value={contentType}
          disabled={readOnly}
          onChange={(e) => handleChange('content_type', e.target.value)}
          className={cn(inputClass, readOnly && readOnlyClass)}
        >
          {CONTENT_TYPES.map((ct) => (
            <option key={ct.value} value={ct.value}>
              {ct.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-(--color-text-muted)">
          {t('bridge.contentTypeDescription')}
        </p>
      </div>

      {/* URL 템플릿 */}
      <div className="space-y-1">
        <label
          htmlFor={urlTemplateId}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          {t('bridge.urlTemplate')}
        </label>
        <input
          id={urlTemplateId}
          type="text"
          value={urlTemplate}
          // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
          readOnly={readOnly}
          onChange={(e) => handleChange('url_template', e.target.value)}
          placeholder={t('bridge.urlTemplatePlaceholder')}
          className={cn(inputClass, readOnly && readOnlyClass)}
        />
        <p className="text-xs text-(--color-text-muted)">
          {t('bridge.urlTemplateDescription')}
        </p>
      </div>

      {/* 타임아웃 */}
      <div className="space-y-1">
        <label
          htmlFor={timeoutId}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          {t('bridge.timeout')}
        </label>
        <input
          id={timeoutId}
          type="number"
          min={100}
          step={100}
          value={timeoutMs}
          // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
          readOnly={readOnly}
          onChange={(e) => {
            const val = Number(e.target.value);
            if (val >= 100) {
              handleChange('timeout_ms', val);
            } else if (e.target.value === '') {
              handleChange('timeout_ms', undefined);
            }
          }}
          placeholder="5000"
          className={cn(inputClass, 'w-32', readOnly && readOnlyClass)}
        />
        <p className="text-xs text-(--color-text-muted)">
          {t('bridge.timeoutDescription')}
        </p>
      </div>
    </div>
  );
}
