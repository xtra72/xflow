// 통합 색 고르개 — 저장소의 모든 색 자리가 쓰는 한 부품.
//
// 조립만 한다. **색 형식은 모른다** — 자릿수를 세고 약식을 펴고 알파를 접는 일은 전부
// `colorFormat.ts` 가 한다. 오늘 `PanelColorFreeInput` 이 "값 형식 판정은 한 곳에만
// 있다. 화면은 형식을 모른다" 고 적어 둔 규율을 그대로 잇는다.
//
// 모달이 아니라 **고정 위치 팝오버**다. 색 자리 40 가운데 17이 스크롤 컨테이너 안의 표
// 행이고 2가 이미 포털 팝오버 안이라, 모달을 겹치면 포커스 덫이 둘이 된다.
// `document.body` 로 포털하는 것은 `transform` 을 쓴 조상(react-grid-layout)이 `fixed`
// 를 가두기 때문이며, 이는 `StatElementStylePopover` 가 이미 같은 이유로 택한 길이다.
//
// @spec SPEC-COLOR-001 §결정 1 · §결정 6 · §결정 7 · §결정 9 (M5)

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Pipette, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import SaturationField from './SaturationField';
import SwatchGrid from './SwatchGrid';
import { hexToHsv, hsvToHex, normalizeColor, type Hsva } from './colorFormat';
import { PALETTE_ROWS } from './palette';
import { placePopover, POPOVER_WIDTH } from './popoverPlacement';
import { pushRecentColor, visibleRecentColors } from './recentColors';

export interface ColorPickerProps {
  /** 현재 값. `undefined` = 정해진 것 없음. */
  value: string | undefined;
  /** 정규화를 통과한 값만 나간다. `undefined` = 비움(`clearable` 일 때만 가능). */
  onChange: (next: string | undefined) => void;
  /** 접근성 이름. 필수 — 자리마다 다르므로 기본값을 두지 않는다. */
  ariaLabel: string;
  /** 알파(8자리)를 허용하는가. **기본 `false`** — 켜는 데 근거가 필요하고 끄는 데는 없다. */
  alpha?: boolean;
  /** 값이 없을 때 실제로 적용되는 색. **표시 전용 — 저장되지 않는다.** */
  inheritedColor?: string;
  /** 비우기(✕) 단추를 그리는가. 기본 `false`. */
  clearable?: boolean;
  /** 프리셋 목록 대체. 기본은 통합 팔레트. 통합 이후 이 속성을 쓰는 자리는 0 이어야 한다. */
  presets?: readonly string[];
  /** 목록 안에서 개별 고르개를 집을 때. */
  testId?: string;
}

/** 한 행에 놓는 칸 수 — 통합 팔레트의 색상환 행과 같은 수를 쓴다. */
const COLUMNS = 10;

/**
 * 값이 hex 가 아닐 때 판·슬라이더가 시작하는 자리.
 *
 * `#9ca3af` 를 HSV 로 읽은 값이며(되돌리면 정확히 `#9ca3af` 가 나온다), 오늘
 * `StatElementStylePopover.toColorInputValue` 가 `var(--color-text-muted)` 를 만났을 때
 * 쓰는 중립 회색과 같다. 여기서 `hexToHsv('#9ca3af')` 를 풀지 않고 상수로 적는 것은,
 * 그 함수가 `null` 을 낼 수 있는 서명이라 결코 지나지 않는 분기가 하나 생기기 때문이다.
 */
const NEUTRAL_HSV: Hsva = { h: 218, s: 0.109, v: 0.686, a: 1 };

/** 크로미엄 계열에만 있다. 결과는 `{ sRGBHex }` 이며 **언제나 불투명**하다. */
interface EyeDropperLike {
  open(): Promise<{ sRGBHex: string }>;
}
type EyeDropperCtor = new () => EyeDropperLike;

/** 목록을 행으로 끊는다. */
function chunk(list: readonly string[], size: number): readonly (readonly string[])[] {
  const out: string[][] = [];
  for (const item of list) {
    const last = out[out.length - 1];
    if (last === undefined || last.length === size) out.push([item]);
    else last.push(item);
  }
  return out;
}

/**
 * 판·슬라이더가 시작할 HSV.
 *
 * 값이 hex 가 아니면(`var(--color-text-muted)` 가 실제로 저장돼 있다) 상속색으로
 * 미끄러지지 않고 중립 회색으로 간다 — 그 값은 "정해진 것 없음" 이 아니라 **정해진 다른
 * 표기**이고, 상속색을 보여 주면 저장된 값과 화면이 어긋난다.
 */
function seedHsv(value: string | undefined, inherited: string | undefined): Hsva {
  if (value !== undefined) return hexToHsv(value) ?? NEUTRAL_HSV;
  if (inherited !== undefined) return hexToHsv(inherited) ?? NEUTRAL_HSV;
  return NEUTRAL_HSV;
}

export default function ColorPicker({
  value,
  onChange,
  ariaLabel,
  alpha = false,
  inheritedColor,
  clearable = false,
  presets,
  testId,
}: ColorPickerProps) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popRef = useRef<HTMLDivElement>(null);
  const hexRef = useRef<HTMLInputElement>(null);

  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number }>({ top: 0, left: 0 });
  const [hsv, setHsv] = useState<Hsva>(() => seedHsv(value, inheritedColor));
  const [draft, setDraft] = useState(value ?? '');
  const [synced, setSynced] = useState(value);
  const [recent, setRecent] = useState<readonly string[]>([]);

  /**
   * 연 뒤 실제로 나간 마지막 색. 최근 목록에는 **닫을 때 이 값 하나만** 올린다.
   *
   * 나가는 값마다 올리면 판을 한 번 끄는 동안 중간 색 수십 개가 밀려 들어와 12칸이
   * 전부 그 끌기의 잔해가 된다 — "방금 쓴 색" 이라는 기능의 목적이 사라진다.
   */
  const lastEmitted = useRef<string | undefined>(undefined);

  // 바깥에서 값이 바뀌면 따라간다. 다만 **우리가 방금 내보낸 값이면 HSV 를 다시 읽지
  // 않는다** — 채도 0·명도 0 에서 색상(hue)이 소실되어 슬라이더가 튀기 때문이다.
  if (value !== synced) {
    setSynced(value);
    setDraft(value ?? '');
    if (value !== hsvToHex(hsv, { alpha })) setHsv(seedHsv(value, inheritedColor));
  }

  const close = useCallback((): void => {
    setOpen(false);
    pushRecentColor(lastEmitted.current);
    // 포커스가 팝오버 안에 있을 때만 되돌린다 — 바깥을 눌러 닫았다면 사용자가 방금 누른
    // 자리에서 포커스를 빼앗지 않는다. 되돌리지 않으면 다음 Tab 이 문서 맨 앞으로 튄다.
    if (popRef.current?.contains(document.activeElement)) triggerRef.current?.focus();
  }, []);

  const emitHex = useCallback(
    (next: string): void => {
      lastEmitted.current = next;
      onChange(next);
    },
    [onChange],
  );

  /** 판·슬라이더에서 나가는 길. HSV 를 상태로 들고 hex 는 거기서 파생한다. */
  const emitHsv = (next: Hsva): void => {
    setHsv(next);
    emitHex(hsvToHex(next, { alpha }));
  };

  /** 프리셋·최근색을 고르는 길. 고르면 닫는다(오늘 `ColorSwatchButton` 의 거동). */
  const pick = (color: string): void => {
    emitHex(color);
    close();
  };

  // 팝오버를 띄울 자리. 배치 산술은 순수 함수로 빼 두었다 — jsdom 에 레이아웃이 없어
  // 컴포넌트 안에 두면 잴 수가 없다.
  useLayoutEffect(() => {
    if (!open) return;
    const trigger = triggerRef.current;
    if (trigger === null) return;
    const r = trigger.getBoundingClientRect();
    setPos(
      placePopover({
        trigger: { top: r.top, bottom: r.bottom, right: r.right },
        // `open` 일 때 이 효과가 도는 시점에는 팝오버가 이미 DOM 에 있다.
        popoverHeight: popRef.current?.offsetHeight ?? 0,
        viewportHeight: window.innerHeight,
      }),
    );
  }, [open]);

  // 열면 첫 조작 요소로 포커스를 옮긴다 — 키보드로 연 사람에게 갈 곳이 있어야 한다.
  useEffect(() => {
    if (open) hexRef.current?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      const el = e.target as Node | null;
      if (el !== null && (popRef.current?.contains(el) || triggerRef.current?.contains(el))) return;
      close();
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') close();
    };
    // `fixed` 는 스크롤을 따라가지 않는다. 어긋난 자리에 떠 있느니 닫는다.
    const onShift = (): void => close();
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    document.addEventListener('scroll', onShift, true);
    window.addEventListener('resize', onShift);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('scroll', onShift, true);
      window.removeEventListener('resize', onShift);
    };
  }, [open, close]);

  /** 스포이트는 **렌더 시각**에 판정한다 — 모듈 상수로 두면 두 갈래를 시험할 수 없다. */
  const EyeDropperCtor = (window as unknown as { EyeDropper?: EyeDropperCtor }).EyeDropper;

  const runEyeDropper = async (ctor: EyeDropperCtor): Promise<void> => {
    try {
      const { sRGBHex } = await new ctor().open();
      const picked = hexToHsv(sRGBHex);
      // 스포이트가 hex 가 아닌 것을 주면 무시한다 — 우리가 만드는 값이 아니다.
      if (picked === null) return;
      // 결과는 언제나 불투명하지만 **현재 알파를 유지한다**. 반투명을 맞춰 둔 뒤 색만
      // 다시 집는 흐름을 끊지 않기 위해서다.
      emitHsv({ ...picked, a: hsv.a });
    } catch {
      // 취소는 오류가 아니다 — 조용히 삼킨다.
    }
  };

  /** 스와치가 보여 줄 색. 상속색은 **표시 전용**이라 저장되지 않는다. */
  const preview = value ?? inheritedColor;
  const presetRows = presets === undefined ? PALETTE_ROWS : chunk(presets, COLUMNS);
  const recentRows = chunk(recent, COLUMNS);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-label={ariaLabel}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => {
          if (open) {
            close();
            return;
          }
          lastEmitted.current = undefined;
          setHsv(seedHsv(value, inheritedColor));
          setDraft(value ?? '');
          setRecent(visibleRecentColors({ alpha }));
          setOpen(true);
        }}
        className={cn(
          'h-6 w-6 shrink-0 rounded-md border border-(--color-border-default) transition-transform hover:scale-110',
          preview === undefined && 'bg-(--color-bg-surface)',
        )}
        style={preview === undefined ? undefined : { backgroundColor: preview }}
        {...(testId === undefined ? {} : { 'data-testid': testId })}
      />
      {open &&
        createPortal(
          <div
            ref={popRef}
            role="dialog"
            aria-label={t('colorPicker.ariaDialog').replace('{label}', ariaLabel)}
            data-testid="colorpicker-popover"
            style={{ position: 'fixed', top: pos.top, left: pos.left, width: POPOVER_WIDTH }}
            className="z-50 flex flex-col gap-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2 shadow-lg"
          >
            <SaturationField hsv={hsv} onPick={(next) => emitHsv({ ...hsv, ...next })} />

            <div className="flex items-center gap-1.5">
              <span
                role="img"
                aria-label={t('colorPicker.custom')}
                data-testid="colorpicker-preview"
                className="h-6 w-6 shrink-0 rounded-md border border-(--color-border-default)"
                style={preview === undefined ? undefined : { backgroundColor: preview }}
              />
              <input
                ref={hexRef}
                type="text"
                data-testid="colorpicker-hex"
                aria-label={t('colorPicker.hex')}
                value={draft}
                spellCheck={false}
                placeholder={alpha ? '#rrggbbaa' : '#rrggbb'}
                maxLength={alpha ? 9 : 7}
                onChange={(e) => {
                  // 치는 도중(`#ab`)은 막지 않되, 형식이 맞는 순간에만 바깥으로 낸다.
                  setDraft(e.target.value);
                  const normalized = normalizeColor(e.target.value, { alpha });
                  if (normalized !== null) emitHex(normalized);
                }}
                // 형식이 안 맞는 채로 떠나면 실제 저장값으로 되돌린다 — 화면의 글자와
                // 적용된 색이 어긋난 채 남지 않도록.
                onBlur={() => setDraft(value ?? '')}
                className="h-6 min-w-0 flex-1 rounded-md bg-(--color-bg-elevated) px-1.5 text-center font-mono text-[11px] text-(--color-text-secondary) outline-none focus:ring-1 focus:ring-(--color-interactive-primary)"
              />
            </div>

            <input
              type="range"
              min={0}
              max={359}
              step={1}
              data-testid="colorpicker-hue"
              aria-label={t('colorPicker.hue')}
              value={Math.round(hsv.h)}
              onChange={(e) => emitHsv({ ...hsv, h: Number(e.target.value) })}
              className="h-2 w-full cursor-pointer appearance-none rounded-full"
              style={{
                background:
                  'linear-gradient(to right, #ff0000, #ffff00, #00ff00, #00ffff, #0000ff, #ff00ff, #ff0000)',
              }}
            />

            {alpha && (
              <input
                type="range"
                min={0}
                max={100}
                step={1}
                data-testid="colorpicker-alpha"
                aria-label={t('colorPicker.opacity')}
                value={Math.round(hsv.a * 100)}
                onChange={(e) => emitHsv({ ...hsv, a: Number(e.target.value) / 100 })}
                className="h-2 w-full cursor-pointer appearance-none rounded-full"
                style={{
                  background: `linear-gradient(to right, ${hsvToHex({ ...hsv, a: 0 }, { alpha: false })}00, ${hsvToHex({ ...hsv, a: 1 }, { alpha: false })})`,
                }}
              />
            )}

            {/* 없는 브라우저에서는 그리지 않는다 — 비활성 단추가 좁은 표 행에서 차지하는
                폭이 아깝고, 눌러도 아무 일이 없는 칸은 설명이 필요해진다. */}
            {EyeDropperCtor !== undefined && (
              <button
                type="button"
                data-testid="colorpicker-eyedropper"
                aria-label={t('colorPicker.eyedropper')}
                onClick={() => void runEyeDropper(EyeDropperCtor)}
                className="flex h-6 items-center justify-center gap-1 rounded-md bg-(--color-bg-elevated) text-(--color-text-muted) hover:bg-(--color-border-default)"
              >
                <Pipette className="h-3.5 w-3.5" />
              </button>
            )}

            <SwatchGrid
              rows={presetRows}
              value={value}
              onPick={pick}
              ariaLabel={t('colorPicker.presets')}
              testId="colorpicker-presets"
            />

            {recent.length > 0 && (
              <SwatchGrid
                rows={recentRows}
                value={value}
                onPick={pick}
                ariaLabel={t('colorPicker.recent')}
                testId="colorpicker-recent"
              />
            )}

            {clearable && (
              <button
                type="button"
                data-testid="colorpicker-clear"
                aria-label={t('colorPicker.clear')}
                onClick={() => {
                  lastEmitted.current = undefined;
                  onChange(undefined);
                  close();
                }}
                className="flex h-6 items-center justify-center gap-1 rounded-md bg-(--color-bg-elevated) text-(--color-text-muted) hover:bg-(--color-border-default)"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>,
          document.body,
        )}
    </>
  );
}
