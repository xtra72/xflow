// Modbus 브릿지 어댑터 설정 섹션 컴포넌트.
// Unit ID, 폴링 간격, 레지스터 맵 테이블을 설정한다.
// 레지스터 맵 편집은 기존 RegisterMapEditor 컴포넌트를 재사용한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';

import { RegisterMapEditor } from './RegisterMapEditor';

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

interface BridgeModbusConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeModbusConfig({ data, onChange, readOnly }: BridgeModbusConfigProps) {
  const unitIdField = useId();
  const pollingId = useId();

  const unitId = data.unit_id != null ? Number(data.unit_id) : 1;
  const pollingInterval = data.polling_interval_ms != null ? Number(data.polling_interval_ms) : 1000;

  const handleChange = (field: string, value: unknown) => {
    onChange({ ...data, [field]: value });
  };

  return (
    <div className="space-y-3">
      {/* 섹션 헤더 */}
      <div className="flex items-center gap-2">
        <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
        <span className="text-xs font-medium text-gray-500 dark:text-gray-400">
          Modbus 설정
        </span>
        <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
      </div>

      {/* Unit ID */}
      <div className="space-y-1">
        <label
          htmlFor={unitIdField}
          className="block text-xs font-medium text-gray-700 dark:text-gray-300"
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
        <p className="text-xs text-gray-400 dark:text-gray-500">
          Modbus 슬레이브 주소 (1-247)
        </p>
      </div>

      {/* 폴링 간격 */}
      <div className="space-y-1">
        <label
          htmlFor={pollingId}
          className="block text-xs font-medium text-gray-700 dark:text-gray-300"
        >
          폴링 간격 (ms)
        </label>
        <input
          id={pollingId}
          type="number"
          min={100}
          step={100}
          value={pollingInterval}
          disabled={readOnly}
          onChange={(e) => {
            const val = Number(e.target.value);
            if (val >= 100) {
              handleChange('polling_interval_ms', val);
            } else if (e.target.value === '') {
              handleChange('polling_interval_ms', undefined);
            }
          }}
          placeholder="1000"
          className={cn(inputClass, 'w-32', readOnly && readOnlyClass)}
        />
        <p className="text-xs text-gray-400 dark:text-gray-500">
          레지스터 읽기 주기 (최소 100ms)
        </p>
      </div>

      {/* 레지스터 맵 */}
      <div className="space-y-1">
        <span className="block text-xs font-medium text-gray-700 dark:text-gray-300">
          레지스터 맵
        </span>
        <RegisterMapEditor
          value={data.register_map}
          onChange={(val) => handleChange('register_map', val)}
          readOnly={readOnly}
        />
        <p className="text-xs text-gray-400 dark:text-gray-500">
          읽기/쓰기 대상 레지스터 영역 정의
        </p>
      </div>
    </div>
  );
}
