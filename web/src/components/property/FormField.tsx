// 타입별 폼 필드 렌더러 컴포넌트.
// ConfigField.type에 따라 적절한 입력 위젯을 렌더링한다.

import { useId } from 'react';

import type { ConfigField } from '@/types/node';
import { cn } from '@/lib/utils/cn';

interface FormFieldProps {
  field: ConfigField;
  value: unknown;
  onChange: (value: unknown) => void;
  error?: string;
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

export function FormField({ field, value, onChange, error }: FormFieldProps) {
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
          className={cn(inputClass, error && errorInputClass)}
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
          className={cn(inputClass, error && errorInputClass)}
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
            className="h-4 w-4 rounded border-gray-300 text-blue-500
              focus:ring-2 focus:ring-blue-400
              dark:border-gray-600 dark:bg-gray-800"
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
          className={cn(inputClass, error && errorInputClass)}
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
          className={cn(
            inputClass,
            'font-mono text-xs',
            error && errorInputClass,
          )}
          {...ariaProps}
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
