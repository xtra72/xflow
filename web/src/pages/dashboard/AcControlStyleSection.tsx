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
  Square,
  SquareDashed,
  Tag,
  Thermometer,
  Type,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';

import ColorSwatchButton from './colorSwatchPalette';
import {
  AC_MODE_KEYS,
  AC_MODE_LABELS,
  FAN_SPEED_KEYS,
  FAN_SPEED_LABELS,
  type AcMode,
  type FanSpeed,
} from './panels/acControlTypes';
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
      title="제어 버튼"
      rightSummary={summary}
    >
      <div className="space-y-2">
        {/* 미선택 */}
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">미선택</span>
          <ColorSwatchButton
            color={cfg.unselected}
            onChange={setUnselected}
            ariaLabel="미선택 컬러"
          />
        </div>

        {/* 선택 모드 토글 */}
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">선택</span>
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
              통일
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
              개별
            </button>
          </div>
        </div>

        {/* 통일 모드: 단일 컬러 */}
        {mode === 'unified' && (
          <div className="flex items-center justify-between pl-2">
            <span className="text-[10px] text-(--color-text-muted)">선택 컬러</span>
            <ColorSwatchButton
              color={cfg.selectedColor}
              onChange={setSelectedColor}
              ariaLabel="선택 컬러"
            />
          </div>
        )}

        {/* 개별 모드: 모드별 컬러 */}
        {mode === 'individual' && (
          <div className="space-y-1 border-t border-(--color-border-subtle) pt-1.5">
            <span className="text-[10px] text-(--color-text-muted)">버튼별 컬러</span>
            {AC_MODE_KEYS.map((key) => (
              <div key={key} className="flex items-center justify-between">
                <span className="text-[11px] text-(--color-text-secondary)">
                  {AC_MODE_LABELS[key]}
                </span>
                <ColorSwatchButton
                  color={cfg.perButton?.[key]}
                  onChange={(c) => setPerButton(key, c)}
                  ariaLabel={`${AC_MODE_LABELS[key]} 컬러`}
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
  const cfg: FanLevelColorConfig = config ?? {};
  const setUnselected = (c: string | undefined) => onChange({ ...cfg, unselected: c });
  const setLevel = (key: FanSpeed, c: string | undefined) => {
    const perLevel = { ...(cfg.perLevel ?? {}) };
    if (c === undefined) delete perLevel[key];
    else perLevel[key] = c;
    onChange({ ...cfg, perLevel });
  };

  return (
    <ExpandableCard icon={<Fan className="h-3 w-3" />} title="풍량">
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-(--color-text-secondary)">미선택</span>
          <ColorSwatchButton
            color={cfg.unselected}
            onChange={setUnselected}
            ariaLabel="풍량 미선택 컬러"
          />
        </div>
        <div className="border-t border-(--color-border-subtle) pt-1" />
        {FAN_SPEED_KEYS.map((key) => (
          <div key={key} className="flex items-center justify-between">
            <span className="text-[11px] text-(--color-text-secondary)">
              {FAN_SPEED_LABELS[key]}
            </span>
            <ColorSwatchButton
              color={cfg.perLevel?.[key]}
              onChange={(c) => setLevel(key, c)}
              ariaLabel={`풍량 ${FAN_SPEED_LABELS[key]} 컬러`}
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
  panelColor: string | undefined;
  accentElements: Record<string, string | boolean>;
  config: Record<string, unknown>;
  onPanelColorChange: (next: string | undefined) => void;
  onAccentChange: (next: Record<string, string | boolean>) => void;
  onConfigChange: (patch: Record<string, unknown>) => void;
}

/**
 * ac-control 패널 전용 스타일 섹션.
 * - 단순 7항목: 전체 색상, 타이틀, 상태 배지, 현재 값, 전원 버튼, 라벨, 보더
 * - 펼침 2항목: 제어 버튼, 풍량
 *
 * 임계값은 별도 섹션 AcControlThresholdsSection 으로 분리되어 있다.
 */
export default function AcControlStyleSection({
  panelColor,
  accentElements,
  config,
  onPanelColorChange,
  onAccentChange,
  onConfigChange,
}: AcControlStyleSectionProps) {
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
      {/* 단순: 전체 색상 (panelColor 직접 제어) */}
      <SimpleStyleRow
        icon={<Square className="h-3 w-3" />}
        label="전체 색상"
        color={panelColor}
        onChange={onPanelColorChange}
      />
      {/* 단순: 타이틀 */}
      <SimpleStyleRow
        icon={<Type className="h-3 w-3" />}
        label="타이틀"
        color={readAccent(AC_STYLE_KEYS.title)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.title, c)}
      />
      {/* 단순: 상태 배지 */}
      <SimpleStyleRow
        icon={<Activity className="h-3 w-3" />}
        label="상태 배지"
        color={readAccent(AC_STYLE_KEYS.statusBadge)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.statusBadge, c)}
      />
      {/* 단순: 현재 값 (강조 라벨 톤) */}
      <SimpleStyleRow
        icon={<Thermometer className="h-3 w-3" />}
        label="현재 값"
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
        label="전원 버튼"
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
        label="라벨"
        color={readAccent(AC_STYLE_KEYS.labels)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.labels, c)}
      />
      {/* 단순: 보더 */}
      <SimpleStyleRow
        icon={<AlignLeft className="h-3 w-3 -rotate-90" />}
        label="보더"
        color={readAccent(AC_STYLE_KEYS.borders)}
        onChange={(c) => writeAccent(AC_STYLE_KEYS.borders, c)}
      />
    </div>
  );
}
