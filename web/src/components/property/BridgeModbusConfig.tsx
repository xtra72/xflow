// Modbus 브릿지 어댑터 설정 섹션 컴포넌트.
// Unit ID와 레지스터 맵 테이블을 설정한다.
// 폴링 간격은 에이전트 설정(poll_interval)에서 관리한다.
// 레지스터 맵 편집은 기존 RegisterMapEditor 컴포넌트를 재사용한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';

import { RegisterMapEditor } from './RegisterMapEditor';

// --- 스타일 ---

const inputClass = cn(
  'w-full rounded-md border px-2.5 py-1.5 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'placeholder:text-gray-400',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:placeholder:text-gray-500 dark:focus:border-blue-500',
);

const readOnlyClass = 'opacity-60 cursor-not-allowed bg-gray-50 dark:bg-gray-900';

// --- Props ---

interface BridgeModbusConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeModbusConfig({ data, onChange, readOnly }: BridgeModbusConfigProps) {
  const unitIdField = useId();

  const unitId = data.unit_id != null ? Number(data.unit_id) : 1;

  const handleChange = (field: string, value: unknown) => {
    onChange({ ...data, [field]: value });
  };

  return (
    <div className="space-y-3">
      {/* 섹션 헤더 */}
      <div className="flex items-center gap-2">
        <div className="h-px flex-1 bg-(--color-border-default)" />
        <span className="text-xs font-medium text-(--color-text-muted)">
          Modbus 설정
        </span>
        <div className="h-px flex-1 bg-(--color-border-default)" />
      </div>

      {/* Unit ID */}
      <div className="space-y-1">
        <label
          htmlFor={unitIdField}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          Unit ID
        </label>
        <input
          id={unitIdField}
          type="number"
          min={1}
          max={247}
          value={unitId}
          disabled={readOnly}
          onChange={(e) => {
            const val = Number(e.target.value);
            if (val >= 1 && val <= 247) {
              handleChange('unit_id', val);
            } else if (e.target.value === '') {
              handleChange('unit_id', undefined);
            }
          }}
          placeholder="1"
          className={cn(inputClass, 'w-28', readOnly && readOnlyClass)}
        />
        <p className="text-xs text-(--color-text-muted)">
          Modbus 슬레이브 주소 (1-247)
        </p>
      </div>

      {/* 레지스터 맵 */}
      <div className="space-y-1">
        <span className="block text-xs font-medium text-(--color-text-secondary)">
          레지스터 맵
        </span>
        <RegisterMapEditor
          value={data.register_map}
          onChange={(val) => handleChange('register_map', val)}
          readOnly={readOnly}
        />
        <p className="text-xs text-(--color-text-muted)">
          읽기/쓰기 대상 레지스터 영역 정의
        </p>
      </div>
    </div>
  );
}
