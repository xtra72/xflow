// HTTP 브릿지 어댑터 설정 섹션 컴포넌트.
// Content-Type, URL 템플릿, 타임아웃을 설정한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';

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
  'border-gray-200 bg-white text-gray-900',
  'placeholder:text-gray-400',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
  'dark:placeholder:text-gray-500 dark:focus:border-blue-500',
);

const readOnlyClass = 'opacity-60 cursor-not-allowed bg-gray-50 dark:bg-gray-900';

// --- Props ---

interface BridgeHttpConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeHttpConfig({ data, onChange, readOnly }: BridgeHttpConfigProps) {
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
        <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
        <span className="text-xs font-medium text-gray-500 dark:text-gray-400">
          HTTP 설정
        </span>
        <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
      </div>

      {/* Content-Type */}
      <div className="space-y-1">
        <label
          htmlFor={contentTypeId}
          className="block text-xs font-medium text-gray-700 dark:text-gray-300"
        >
          Content-Type
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
        <p className="text-xs text-gray-400 dark:text-gray-500">
          요청/응답 본문 형식
        </p>
      </div>

      {/* URL 템플릿 */}
      <div className="space-y-1">
        <label
          htmlFor={urlTemplateId}
          className="block text-xs font-medium text-gray-700 dark:text-gray-300"
        >
          URL 템플릿
        </label>
        <input
          id={urlTemplateId}
          type="text"
          value={urlTemplate}
          disabled={readOnly}
          onChange={(e) => handleChange('url_template', e.target.value)}
          placeholder="/api/{resource}/{id}"
          className={cn(inputClass, readOnly && readOnlyClass)}
        />
        <p className="text-xs text-gray-400 dark:text-gray-500">
          {'URL 경로 템플릿 ({필드명} 형식으로 동적 치환)'}
        </p>
      </div>

      {/* 타임아웃 */}
      <div className="space-y-1">
        <label
          htmlFor={timeoutId}
          className="block text-xs font-medium text-gray-700 dark:text-gray-300"
        >
          타임아웃 (ms)
        </label>
        <input
          id={timeoutId}
          type="number"
          min={100}
          step={100}
          value={timeoutMs}
          disabled={readOnly}
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
        <p className="text-xs text-gray-400 dark:text-gray-500">
          HTTP 요청 타임아웃 (밀리초)
        </p>
      </div>
    </div>
  );
}
