// 에어컨 제어 패널 임계값 섹션 (게이지 디자인 패턴 정합).
//
// 구성:
//   - 모드 토글 [개별 지정] [연속 컬러]
//   - 임계값 행 (개별 모드): [● color] [name] [min] ~ [max] [↑↓×]
//     - min undefined → "-∞" placeholder
//     - max undefined → "+∞" placeholder
//   - 매치 우선순위: 위에서부터 첫 매치
//   - "+ 임계값 추가" 버튼

import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import ColorSwatchButton from './colorSwatchPalette';
import type { ValueColorConfig, ValueColorRange } from './panels/acControlColors';

export interface AcControlThresholdsSectionProps {
  config: ValueColorConfig | undefined;
  onChange: (next: ValueColorConfig | undefined) => void;
}

export default function AcControlThresholdsSection({
  config,
  onChange,
}: AcControlThresholdsSectionProps) {
  const { t } = useTranslation();
  const cfg: ValueColorConfig = config ?? {};
  const ranges: ValueColorRange[] = Array.isArray(cfg.ranges) ? cfg.ranges : [];
  const mode = cfg.mode ?? 'individual';

  const updateRange = (idx: number, patch: Partial<ValueColorRange>) => {
    const next = ranges.map((r, i) => (i === idx ? { ...r, ...patch } : r));
    onChange({ ...cfg, ranges: next });
  };
  const addRange = () => {
    onChange({ ...cfg, ranges: [...ranges, { color: '#3b82f6' }] });
  };
  const removeRange = (idx: number) => {
    onChange({ ...cfg, ranges: ranges.filter((_, i) => i !== idx) });
  };
  // 구간 순서 이동 — 매치 우선순위가 위에서부터이므로 사용자가 직접 정렬할 수 있어야 한다.
  const moveRange = (idx: number, delta: -1 | 1) => {
    const target = idx + delta;
    if (target < 0 || target >= ranges.length) return;
    const next = ranges.slice();
    const [moved] = next.splice(idx, 1);
    if (moved !== undefined) next.splice(target, 0, moved);
    onChange({ ...cfg, ranges: next });
  };
  const setMode = (m: 'individual' | 'continuous') => {
    onChange({ ...cfg, mode: m });
  };

  // 빈 문자열은 undefined 로 처리 (-∞ / +∞ 의미)
  const parseBound = (raw: string): number | undefined => {
    const v = raw.trim();
    if (v === '') return undefined;
    const n = Number(v);
    return Number.isFinite(n) ? n : undefined;
  };

  return (
    <div className="space-y-2">
      {/* 모드 토글 */}
      <div className="flex w-full items-center rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) p-0.5">
        <button
          type="button"
          onClick={() => setMode('individual')}
          className={cn(
            'flex-1 rounded px-2 py-1 text-[11px] font-medium transition-colors',
            mode === 'individual'
              ? 'bg-blue-600 text-white'
              : 'text-(--color-text-muted) hover:text-(--color-text-secondary)',
          )}
        >
          {t('dashboard.thresholds.individual')}
        </button>
        <button
          type="button"
          onClick={() => setMode('continuous')}
          className={cn(
            'flex-1 rounded px-2 py-1 text-[11px] font-medium transition-colors',
            mode === 'continuous'
              ? 'bg-blue-600 text-white'
              : 'text-(--color-text-muted) hover:text-(--color-text-secondary)',
          )}
        >
          {t('dashboard.thresholds.continuous')}
        </button>
      </div>

      {/* 개별 모드 — 임계값 행 리스트 */}
      {mode === 'individual' && (
        <>
          {ranges.length === 0 ? (
            <p className="px-1 text-[10px] text-(--color-text-muted)">
              {t('dashboard.thresholds.empty')}
            </p>
          ) : (
            <div className="space-y-1.5">
              {ranges.map((r, idx) => (
                <div key={idx} className="flex w-full items-center gap-1.5">
                  <span className="shrink-0">
                    <ColorSwatchButton
                      color={r.color}
                      onChange={(c) => updateRange(idx, { color: c ?? '#3b82f6' })}
                      ariaLabel={t('dashboard.thresholds.colorAria').replace('{index}', String(idx + 1))}
                    />
                  </span>
                  <input
                    type="text"
                    value={r.name ?? ''}
                    onChange={(e) => updateRange(idx, { name: e.target.value })}
                    placeholder={t('dashboard.thresholds.namePlaceholder')}
                    className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                    aria-label={t('dashboard.thresholds.nameAria').replace('{index}', String(idx + 1))}
                  />
                  <input
                    type="number"
                    step="any"
                    value={r.min ?? ''}
                    onChange={(e) => updateRange(idx, { min: parseBound(e.target.value) })}
                    placeholder="-∞"
                    className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 text-center text-[11px] tabular-nums text-(--color-text-primary) outline-none focus:border-blue-500"
                    aria-label={t('dashboard.thresholds.minAria').replace('{index}', String(idx + 1))}
                  />
                  <span className="shrink-0 text-[10px] text-(--color-text-muted)">~</span>
                  <input
                    type="number"
                    step="any"
                    value={r.max ?? ''}
                    onChange={(e) => updateRange(idx, { max: parseBound(e.target.value) })}
                    placeholder="+∞"
                    className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 text-center text-[11px] tabular-nums text-(--color-text-primary) outline-none focus:border-blue-500"
                    aria-label={t('dashboard.thresholds.maxAria').replace('{index}', String(idx + 1))}
                  />
                  <button
                    type="button"
                    onClick={() => moveRange(idx, -1)}
                    disabled={idx === 0}
                    className="shrink-0 text-(--color-text-muted) hover:text-(--color-text-secondary) disabled:opacity-30"
                    aria-label={t('dashboard.thresholds.moveUpAria').replace('{index}', String(idx + 1))}
                    data-testid={`thresholds-move-up-${idx}`}
                  >
                    <ChevronUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => moveRange(idx, 1)}
                    disabled={idx === ranges.length - 1}
                    className="shrink-0 text-(--color-text-muted) hover:text-(--color-text-secondary) disabled:opacity-30"
                    aria-label={t('dashboard.thresholds.moveDownAria').replace('{index}', String(idx + 1))}
                    data-testid={`thresholds-move-down-${idx}`}
                  >
                    <ChevronDown className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => removeRange(idx)}
                    className="shrink-0 text-(--color-text-muted) hover:text-red-500"
                    aria-label={t('dashboard.thresholds.deleteAria').replace('{index}', String(idx + 1))}
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* + 임계값 추가 */}
          <button
            type="button"
            onClick={addRange}
            className="flex items-center gap-1 px-1 text-[11px] font-medium text-blue-500 hover:text-blue-600"
            data-testid="thresholds-add-range"
          >
            <Plus className="h-3 w-3" />
            {t('dashboard.thresholds.addThreshold')}
          </button>
        </>
      )}

      {/* 연속 컬러 모드 — 향후 테마 프리셋 추가 가능 */}
      {mode === 'continuous' && (
        <p className="px-1 text-[10px] text-(--color-text-muted)">
          {t('dashboard.thresholds.continuousHint')}
        </p>
      )}
    </div>
  );
}
