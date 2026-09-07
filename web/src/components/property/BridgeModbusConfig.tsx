// Modbus 브릿지 어댑터 설정 섹션 컴포넌트.
// Unit ID와 레지스터 맵 테이블을 설정한다.
// 폴링 간격은 에이전트 설정(poll_interval)에서 관리한다.
// 레지스터 맵 편집은 기존 RegisterMapEditor 컴포넌트를 재사용한다.

import { useId } from 'react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import { RegisterMapEditor } from './RegisterMapEditor';

// --- 스타일 ---

const inputClass = cn(
  'w-full rounded-md border px-2.5 py-1.5 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'placeholder:text-(--color-text-muted)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  ' dark:focus:border-blue-500',
);

// readOnly 스타일.
//
// 주의: number 등 readOnly attr 를 지원하는 input 에는 `disabled` 가 아닌
// `readOnly` 를 사용한다. `disabled` 는 다크모드에서 텍스트를 흐리게 렌더링해
// 값이 거의 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 / 309966e 와 동일 패턴).
const readOnlyClass = 'cursor-not-allowed bg-(--color-bg-elevated)';

// --- Props ---

interface BridgeModbusConfigProps {
  data: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
}

// --- 컴포넌트 ---

export function BridgeModbusConfig({ data, onChange, readOnly }: BridgeModbusConfigProps) {
  const { t } = useTranslation();
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
          {t('bridge.modbusSettings')}
        </span>
        <div className="h-px flex-1 bg-(--color-border-default)" />
      </div>

      {/* Unit ID */}
      <div className="space-y-1">
        <label
          htmlFor={unitIdField}
          className="block text-xs font-medium text-(--color-text-secondary)"
        >
          {t('bridge.unitId')}
        </label>
        <input
          id={unitIdField}
          type="number"
          min={1}
          max={247}
          value={unitId}
          // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
          readOnly={readOnly}
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
          {t('bridge.unitIdDescription')}
        </p>
      </div>

      {/* 레지스터 맵 */}
      <div className="space-y-1">
        <span className="block text-xs font-medium text-(--color-text-secondary)">
          {t('bridge.registerMap')}
        </span>
        <RegisterMapEditor
          value={data.register_map}
          onChange={(val) => handleChange('register_map', val)}
          readOnly={readOnly}
        />
        <p className="text-xs text-(--color-text-muted)">
          {t('bridge.registerMapDescription')}
        </p>
      </div>
    </div>
  );
}
