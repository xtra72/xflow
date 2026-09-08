// 에어컨 제어 패널 스타일 섹션.
// 패널 설정 다이얼로그에서 ac-control 패널일 때 "스타일" 섹션으로 표시한다.
//
// 항목 (디자인 v2 기준, 임계값은 별도 섹션 AcControlThresholdsSection 으로 분리):
//   단순 행      : 전체 색상, 타이틀, 상태 배지, 현재 값, 전원 버튼, 라벨, 보더
//   펼침 카드    : 제어 버튼, 풍량
//
// 단순 행은 [icon] [label] [color swatch] 구조이며 swatch 클릭 시 팔레트 팝오버를 표시한다.
// 펼침 카드는 <details> 기반으로 인라인 에디터를 보여준다 (제어 버튼=통일/개별, 풍량=단계별).

import {
  Activity,
  AlignLeft,
  Fan,
  Power,
  SquareDashed,
  Tag,
  Thermometer,
  Type,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import ColorSwatchButton from './colorSwatchPalette';
import {
  AC_MODE_KEYS,
  FAN_SPEED_KEYS,
  type AcMode,
  type FanSpeed,
} from './panels/acControlTypes';

// AC 모드/풍량 라벨 i18n 키 매핑 (acControlTypes 의 한국어 상수 대신 사용).
// AcControlPanel 과 동일한 키를 재사용해 일관성을 유지한다.
const AC_MODE_LABEL_KEYS: Record<AcMode, string> = {
  cool: 'dashboard.acPanel.cooling',
  heat: 'dashboard.acPanel.heating',
  auto: 'dashboard.acPanel.auto',
  dry: 'dashboard.acPanel.dehumidify',
  fan: 'dashboard.acControl.fan',
};

const FAN_SPEED_LABEL_KEYS: Record<FanSpeed, string> = {
  auto: 'dashboard.acPanel.auto',
  quiet: 'dashboard.acControl.fanQuiet',
  low: 'dashboard.acPanel.low',
  medium: 'dashboard.acPanel.medium',
  high: 'dashboard.acPanel.high',
  turbo: 'dashboard.acControl.fanTurbo',
};
import type {
  ControlButtonColorConfig,
  FanLevelColorConfig,
} from './panels/acControlColors';

// ---- 공통 ----

/** 단순 스타일 항목 행 — [icon][label][swatch] */
function SimpleStyleRow({
  icon,
  label,
  color,
  onChange,
}: {
  icon: React.ReactNode;
  label: string;
  color: string | undefined;
  onChange: (next: string | undefined) => void;
}) {
  return (
    <div className="flex items-center justify-between rounded-md border border-(--color-border-default) px-2.5 py-1.5">
      <span className="flex items-center gap-1.5 text-[11px] font-medium text-(--color-text-secondary)">
        {icon}
        {label}
      </span>
      <ColorSwatchButton color={color} onChange={onChange} ariaLabel={label} />
    </div>
  );
}

/** 펼침 카드 wrapper (헤더 + 디바이더 + 본문) */
function ExpandableCard({
  icon,
  title,
  rightSummary,
  defaultOpen = false,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  rightSummary?: React.ReactNode;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  return (
    <details
      open={defaultOpen}
      className="rounded-md border border-(--color-border-default)"
    >
      <summary className="flex cursor-pointer list-none items-center justify-between px-2.5 py-1.5 hover:bg-(--color-bg-elevated)">
        <span className="flex items-center gap-1.5 text-[11px] font-medium text-(--color-text-secondary)">
          {icon}
          {title}
        </span>
        <span className="flex items-center gap-1.5">{rightSummary}</span>
      </summary>
      <div className="border-t border-(--color-border-default) px-2.5 py-2">{children}</div>
    </details>
  );
}

// ---- 제어 버튼 섹션 ----

function ControlButtonColorEditor({
  config,
  onChange,
}: {
  config: ControlButtonColorConfig | undefined;
  onChange: (next: ControlButtonColorConfig | undefined) => void;
}) {
  const { t } = useTranslation();
  const cfg: ControlButtonColorConfig = config ?? {};
  const mode = cfg.selectedMode ?? 'unified';

  const setMode = (m: 'unified' | 'individual') => {
    onChange({ ...cfg, selectedMode: m });
  };
  const setUnselected = (c: string | undefined) => {
    onChange({ ...cfg, unselected: c });
  };
  const setSelectedColor = (c: string | undefined) => {
    onChange({ ...cfg, selectedColor: c });
  };
  const setPerButton = (key: AcMode, c: string | undefined) => {
    const perButton = { ...(cfg.perButton ?? {}) };
    if (c === undefined) delete perButton[key];
    else perButton[key] = c;
    onChange({ ...cfg, perButton });
  };

  const summary = cfg.selectedColor || cfg.unselected ? (
    <span
      className="h-2.5 w-2.5 rounded-full border border-(--color-border-default)"
      style={{ backgroundColor: cfg.selectedColor ?? cfg.unselected }}
    />
  ) : null;

  return (
    <ExpandableCard
      icon={<SquareDashed className="h-3 w-3" />}
      title={t('dashboard.acStyle.controlButton')}
      rightSummary={summary}
    >
      <div className="space-y-2">
        {/* 미선택 */}
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">{t('dashboard.acStyle.unselected')}</span>
          <ColorSwatchButton
            color={cfg.unselected}
            onChange={setUnselected}
            ariaLabel={t('dashboard.acStyle.unselectedColorAria')}
          />
        </div>

        {/* 선택 모드 토글 */}
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">{t('dashboard.acStyle.selected')}</span>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setMode('unified')}
              className={cn(
                'rounded-full px-2 py-0.5 text-[10px] font-medium border',
                mode === 'unified'
                  ? 'border-blue-500 bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'border-(--color-border-default) text-(--color-text-muted)',
              )}
            >
              {t('dashboard.acStyle.unified')}
            </button>
            <button
              type="button"
              onClick={() => setMode('individual')}
              className={cn(
                'rounded-full px-2 py-0.5 text-[10px] font-medium border',
                mode === 'individual'
                  ? 'border-blue-500 bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'border-(--color-border-default) text-(--color-text-muted)',
              )}
            >
              {t('dashboard.acStyle.individual')}
            </button>
          </div>
        </div>

        {/* 통일 모드: 단일 컬러 */}
        {mode === 'unified' && (
          <div className="flex items-center justify-between pl-2">
            <span className="text-[10px] text-(--color-text-muted)">{t('dashboard.acStyle.selectedColor')}</span>
            <ColorSwatchButton
              color={cfg.selectedColor}
              onChange={setSelectedColor}
              ariaLabel={t('dashboard.acStyle.selectedColorAria')}
            />
          </div>
        )}

        {/* 개별 모드: 모드별 컬러 */}
        {mode === 'individual' && (
          <div className="space-y-1 border-t border-(--color-border-subtle) pt-1.5">
            <span className="text-[10px] text-(--color-text-muted)">{t('dashboard.acStyle.perButtonColor')}</span>
            {AC_MODE_KEYS.map((key) => (
              <div key={key} className="flex items-center justify-between">
                <span className="text-[11px] text-(--color-text-secondary)">
                  {t(AC_MODE_LABEL_KEYS[key])}
                </span>
                <ColorSwatchButton
                  color={cfg.perButton?.[key]}
                  onChange={(c) => setPerButton(key, c)}
                  ariaLabel={t('dashboard.acStyle.modeColorAria').replace('{label}', t(AC_MODE_LABEL_KEYS[key]))}
                />
              </div>
            ))}
          </div>
        )}
      </div>
    </ExpandableCard>
  );
}

// ---- 풍량 섹션 ----

function FanLevelColorEditor({
  config,
  onChange,
}: {
  config: FanLevelColorConfig | undefined;
  onChange: (next: FanLevelColorConfig | undefined) => void;
}) {
  const { t } = useTranslation();
  const cfg: FanLevelColorConfig = config ?? {};
  const setUnselected = (c: string | undefined) => onChange({ ...cfg, unselected: c });
  const setLevel = (key: FanSpeed, c: string | undefined) => {
    const perLevel = { ...(cfg.perLevel ?? {}) };
    if (c === undefined) delete perLevel[key];
    else perLevel[key] = c;
    onChange({ ...cfg, perLevel });
  };

  return (
    <ExpandableCard icon={<Fan className="h-3 w-3" />} title={t('dashboard.acStyle.airflow')}>
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">{t('dashboard.acStyle.unselected')}</span>
          <ColorSwatchButton
            color={cfg.unselected}
            onChange={setUnselected}
            ariaLabel={t('dashboard.acStyle.fanUnselectedAria')}
          />
        </div>
        <div className="border-t border-(--color-border-subtle) pt-1" />
        {FAN_SPEED_KEYS.map((key) => (
          <div key={key} className="flex items-center justify-between">
            <span className="text-[11px] text-(--color-text-secondary)">
              {t(FAN_SPEED_LABEL_KEYS[key])}
            </span>
            <ColorSwatchButton
              color={cfg.perLevel?.[key]}
              onChange={(c) => setLevel(key, c)}
              ariaLabel={t('dashboard.acStyle.fanLevelColorAria').replace('{label}', t(FAN_SPEED_LABEL_KEYS[key]))}
            />
          </div>
        ))}
      </div>
    </ExpandableCard>
  );
}

// ---- 메인 ----

/**
 * 단순 항목 키 매핑.
 * accentElements 내부 키를 일관되게 사용하기 위한 상수.
 * AC 패널 렌더링 측에서도 이 키들을 참조해 색상을 적용한다.
 */
export const AC_STYLE_KEYS = {
  title: 'titleColor',
  statusBadge: 'statusBadgeColor',
  currentValue: 'currentValueAccent',
  power: 'powerColor',
  labels: 'labelsColor',
  borders: 'bordersColor',
} as const;

export interface AcControlStyleSectionProps {
  accentElements: Record<string, string | boolean>;
  config: Record<string, unknown>;
  onAccentChange: (next: Record<string, string | boolean>) => void;
  onConfigChange: (patch: Record<string, unknown>) => void;
}

/**
 * ac-control 패널 전용 스타일 섹션.
 * - 단순 6항목: 타이틀, 상태 배지, 현재 값, 전원 버튼, 라벨, 보더
 * - 펼침 2항목: 제어 버튼, 풍량
 *
 * 전체 색상(panelColor)은 이 섹션에 있지 않다 — 타입과 무관한 패널 속성이라
 * 패널 옵션의 패널 색상이 소유한다. 한 값을 두 자리에서 편집하지 않는다.
 *
 * 임계값은 별도 섹션 AcControlThresholdsSection 으로 분리되어 있다.
 */
export default function AcControlStyleSection({
  accentElements,
  config,
  onAccentChange,
  onConfigChange,
}: AcControlStyleSectionProps) {
  const { t } = useTranslation();
  const controlButtonColor = config.controlButtonColor as
    | ControlButtonColorConfig
    | undefined;
  const fanLevelColor = config.fanLevelColor as FanLevelColorConfig | undefined;

  // accent 단순 항목 read/write 헬퍼
  const readAccent = (key: string): string | undefined => {
    const v = accentElements[key];
    return typeof v === 'string' ? v : undefined;
  };
  const writeAccent = (key: string, color: string | undefined) => {
    const next = { ...accentElements };
    if (color === undefined) delete next[key];
    else next[key] = color;
    onAccentChange(next);
  };

  return (
    <div className="space-y-1.5">
      {/* 단순: 타이틀 */}
      <SimpleStyleRow
        icon={<Type className="h-3 w-3" />}
        label={t('dashboard.acStyle.title')}
        color={readAccent(AC_STYLE_KEYS.title)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.title, c)}
      />
      {/* 단순: 상태 배지 */}
      <SimpleStyleRow
        icon={<Activity className="h-3 w-3" />}
        label={t('dashboard.acStyle.statusBadge')}
        color={readAccent(AC_STYLE_KEYS.statusBadge)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.statusBadge, c)}
      />
      {/* 단순: 현재 값 (강조 라벨 톤) */}
      <SimpleStyleRow
        icon={<Thermometer className="h-3 w-3" />}
        label={t('dashboard.acStyle.currentValue')}
        color={readAccent(AC_STYLE_KEYS.currentValue)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.currentValue, c)}
      />
      {/* 펼침: 제어 버튼 */}
      <ControlButtonColorEditor
        config={controlButtonColor}
        onChange={(next) => onConfigChange({ controlButtonColor: next })}
      />
      {/* 단순: 전원 버튼 */}
      <SimpleStyleRow
        icon={<Power className="h-3 w-3" />}
        label={t('dashboard.acStyle.powerButton')}
        color={readAccent(AC_STYLE_KEYS.power)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.power, c)}
      />
      {/* 펼침: 풍량 */}
      <FanLevelColorEditor
        config={fanLevelColor}
        onChange={(next) => onConfigChange({ fanLevelColor: next })}
      />
      {/* 단순: 라벨 */}
      <SimpleStyleRow
        icon={<Tag className="h-3 w-3" />}
        label={t('dashboard.acStyle.label')}
        color={readAccent(AC_STYLE_KEYS.labels)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.labels, c)}
      />
      {/* 단순: 보더 */}
      <SimpleStyleRow
        icon={<AlignLeft className="h-3 w-3 -rotate-90" />}
        label={t('dashboard.acStyle.border')}
        color={readAccent(AC_STYLE_KEYS.borders)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.borders, c)}
      />
    </div>
  );
}
