// 통계 패널 요소의 **글자 스타일 팝오버** — 요소를 더블클릭하면 열린다.
//
// `document.body` 로 포털한다. 패널 안에 그리면 `overflow: hidden` 과 낮은 z-index 에
// 잘린다(`DesignPopover` 와 같은 판단 — spec.md §7 OQ4).
//
// 변화량만 색 칸이 다르다. 변화량의 색은 **방향**(증가/감소/변화없음)이 정하므로
// (SPEC-CHART-003 §2.2), 정적 글자색을 따로 두면 두 축이 싸운다. 그래서 색 자리에
// `delta_display` 의 3색을 낸다(spec.md §5 D4).
//
// @spec SPEC-CHART-004 §2.3 [U3]

import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { useTranslation } from '@/lib/i18n';

import { TextStyleFields } from './textStyleFields';
import { DEFAULT_DELTA_COLORS, type DeltaColors } from './panels/charts/statDisplayOptions';
import type { StatElementKind } from './panels/charts/statLayout';
import type { ChartFontFamily } from './panels/charts/textStyle';

/** 팝오버가 바라는 폭(px). 좁으면 글꼴 칸이 짜부라진다. */
const POPOVER_WIDTH_PX = 288;
/** 화면 가장자리와의 최소 여백(px). */
const VIEWPORT_MARGIN_PX = 8;

/** 팝오버가 편집하는 값 — 이미 해석된 형태로 받는다. */
export interface StatStyleValue {
  fontFamily: ChartFontFamily | undefined;
  fontSize: number | undefined;
  fontColor: string | undefined;
  fontWeight: 'normal' | 'bold' | undefined;
}

export interface StatStylePatch {
  font_family?: ChartFontFamily | undefined;
  font_size?: number | undefined;
  font_color?: string | undefined;
  font_weight?: 'normal' | 'bold' | undefined;
}

/**
 * `<input type="color">` 이 받을 수 있는 형태로 바꾼다.
 *
 * 변화량의 "변화 없음" 기본색은 `var(--color-text-muted)` 라 색 입력에 넣을 수 없다 —
 * 그대로 주면 브라우저가 조용히 `#000000` 으로 떨어뜨려, 열기만 해도 검정으로 보인다.
 * 스와치에는 실제 값을 그대로 쓰고, 입력의 초기값만 중립 회색으로 대체한다.
 */
function toColorInputValue(v: string): string {
  return /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(v.trim()) ? v.trim() : '#9ca3af';
}

/** 요소 종류별 라벨 i18n 키. */
const LABEL_KEYS: Record<StatElementKind, string> = {
  value: 'dashboard.chart.statElementValue',
  delta: 'dashboard.chart.statElementDelta',
  stats: 'dashboard.chart.statElementStats',
};

export function StatElementStylePopover({
  kind,
  anchor,
  value,
  deltaColors,
  onChange,
  onDeltaColorChange,
  onClose,
}: {
  kind: StatElementKind;
  /** 팝오버를 붙일 요소. 이 상자 아래에 뜬다. */
  anchor: HTMLElement;
  value: StatStyleValue;
  /** 변화량에서만 쓴다 — 방향별 3색. */
  deltaColors?: DeltaColors;
  onChange: (patch: StatStylePatch) => void;
  /** 변화량에서만 쓴다. `delta_display` 를 갱신한다. */
  onDeltaColorChange?: (patch: Record<string, string>) => void;
  onClose: () => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const boxRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number; width: number } | null>(null);

  // 화면 경계를 넘지 않는 자리로 옮긴다. 폭을 먼저 화면에 맞춰 줄이므로 항상 들어온다.
  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect();
    const width = Math.min(POPOVER_WIDTH_PX, window.innerWidth - VIEWPORT_MARGIN_PX * 2);
    const left = Math.min(
      Math.max(r.left, VIEWPORT_MARGIN_PX),
      Math.max(VIEWPORT_MARGIN_PX, window.innerWidth - width - VIEWPORT_MARGIN_PX),
    );
    setPos({ top: r.bottom + 4, left, width });
  }, [anchor]);

  // 열리면 첫 입력으로 포커스를 옮긴다 — 키보드로 연 경우 갈 곳이 있어야 한다.
  //
  // 닫을 때는 열기 전에 있던 자리로 되돌린다. 되돌리지 않으면 포커스가 사라진 포털
  // 안에 남아, 다음 Tab 이 문서 맨 앞으로 튄다 — 키보드로 연 사람은 방금 있던 자리를
  // 손으로 다시 찾아가야 한다(AC-23).
  useEffect(() => {
    const opener = document.activeElement;
    boxRef.current?.querySelector<HTMLElement>('select, input')?.focus();
    return () => {
      // 그 사이 화면에서 사라졌을 수 있다(패널 재구성·항목 삭제). 없는 자리를
      // 붙잡지 않고 그냥 둔다.
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    };
  }, []);

  // 바깥 클릭 / Esc 로 닫는다.
  useEffect(() => {
    const onDown = (e: MouseEvent): void => {
      const el = e.target as Node | null;
      if (el && (boxRef.current?.contains(el) || anchor.contains(el))) return;
      onClose();
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [anchor, onClose]);

  const colors = deltaColors ?? DEFAULT_DELTA_COLORS;

  return createPortal(
    <div
      ref={boxRef}
      data-testid="stat-style-popover"
      data-stat-style-kind={kind}
      role="dialog"
      aria-label={t(LABEL_KEYS[kind])}
      style={
        pos
          ? { top: pos.top, left: pos.left, width: pos.width }
          : { top: -9999, left: -9999, width: POPOVER_WIDTH_PX }
      }
      className="fixed z-50 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-3 shadow-lg"
    >
      <TextStyleFields
        label={t(LABEL_KEYS[kind])}
        family={value.fontFamily}
        size={value.fontSize}
        // 변화량은 글자색 축이 없다 — 칸 자체를 없애고 아래에서 방향별 3색을 대신 낸다.
        color={value.fontColor}
        showColor={kind !== 'delta'}
        weight={value.fontWeight ?? 'inherit'}
        sizePlaceholder={t('dashboard.chart.inherit')}
        testIdPrefix={`stat-style-${kind}`}
        onChange={(patch) =>
          onChange({
            ...('family' in patch ? { font_family: patch.family } : {}),
            ...('size' in patch ? { font_size: patch.size } : {}),
            ...('color' in patch ? { font_color: patch.color } : {}),
            ...('weight' in patch ? { font_weight: patch.weight } : {}),
          })
        }
      />
      {kind === 'delta' && (
        <div className="mt-2 flex flex-wrap gap-3" data-testid="stat-style-delta-colors">
          {(
            [
              ['up', 'up_color', 'dashboard.chart.deltaUpColor'],
              ['down', 'down_color', 'dashboard.chart.deltaDownColor'],
              ['flat', 'flat_color', 'dashboard.chart.deltaFlatColor'],
            ] as const
          ).map(([slot, key, labelKey]) => (
            <label
              key={slot}
              className="flex items-center gap-1.5 text-[11px] text-(--color-text-muted)"
            >
              <span
                className="h-4 w-4 shrink-0 rounded-sm border border-(--color-border-default)"
                style={{ backgroundColor: colors[slot] }}
              />
              {t(labelKey)}
              <input
                type="color"
                data-testid={`stat-style-delta-${slot}`}
                aria-label={t(labelKey)}
                value={toColorInputValue(colors[slot])}
                onChange={(e) => onDeltaColorChange?.({ [key]: e.target.value })}
                className="h-0 w-0 opacity-0"
              />
            </label>
          ))}
        </div>
      )}
    </div>,
    document.body,
  );
}
