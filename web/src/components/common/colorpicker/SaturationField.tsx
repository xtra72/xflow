// 2D 채도·명도 판 — 포인터 전용이고 탭 순서 밖이다.
//
// ARIA 에 두 축을 동시에 나르는 슬라이더 역할이 없다. 그래서 SPEC 은 판에
// `role="application"` 을 씌우는 대신(낭독기의 기본 탐색을 통째로 끄는 무거운 선언이다)
// **판을 `aria-hidden` 으로 두고 같은 일을 하는 키보드 길을 따로 보장하는** 쪽을 골랐다.
// 그 "같은 길" 은 `ColorPicker` 의 16진 칸 · 색상 슬라이더 · 프리셋 격자이며, 16진 칸은
// 이 판이 하는 일의 상위집합이다(임의의 색을 정확히 지정할 수 있다).
//
// **마우스 이벤트를 쓴다 — 포인터 이벤트가 아니다.** jsdom 26 에는 `PointerEvent` 가
// 없어서, 포인터로 끌면 끌기 경로를 시험으로 **잴 수가 없다**(실측: `fireEvent.pointerDown`
// 이 `clientX` 를 싣지 못하고 `undefined` 가 된다). 대신 터치 끌기를 이 판에서 받지
// 않는데, 터치 사용자는 색상·불투명도 슬라이더(네이티브 `range`)와 프리셋 격자로 같은
// 일을 할 수 있다.
//
// @spec SPEC-COLOR-001 §결정 7 (M5)

import { useCallback, useEffect, useRef } from 'react';

import { hsvToHex, type Hsva } from './colorFormat';

export interface SaturationFieldProps {
  /** 현재 색. 배경의 색상(hue)과 손잡이 자리를 정한다. */
  hsv: Hsva;
  /** 판 위 한 점이 정해졌을 때. 색상과 알파는 이 판이 건드리지 않는다. */
  onPick: (next: { s: number; v: number }) => void;
}

/** 0–1 로 가둔다 — 판 밖으로 끌고 나가도 색은 판 안에 머문다. */
function clamp01(n: number): number {
  return Math.min(1, Math.max(0, n));
}

export default function SaturationField({ hsv, onPick }: SaturationFieldProps) {
  const ref = useRef<HTMLDivElement>(null);
  /** 누른 채로 움직이는 중인가. 누르지 않고 지나간 마우스는 색을 바꾸지 않는다. */
  const dragging = useRef(false);

  const apply = useCallback(
    (clientX: number, clientY: number): void => {
      const r = ref.current?.getBoundingClientRect();
      // 폭·높이 0 은 "판이 납작하다" 가 아니라 **"재지 못했다"** 는 뜻이다(레이아웃이
      // 아직 없거나 숨겨진 자리). 0 으로 나누면 s·v 가 NaN 이 되고, NaN 은 예외를 내지
      // 않은 채 `#NaNNaNNaN` 같은 무효 문자열로 흘러나간다.
      if (!r || r.width === 0 || r.height === 0) return;
      onPick({
        s: clamp01((clientX - r.left) / r.width),
        v: 1 - clamp01((clientY - r.top) / r.height),
      });
    },
    [onPick],
  );

  // 끌기는 판 밖으로 나가도 이어져야 한다 — 그래서 창에 단다.
  useEffect(() => {
    const move = (e: MouseEvent): void => {
      if (dragging.current) apply(e.clientX, e.clientY);
    };
    const up = (): void => {
      dragging.current = false;
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
    return () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
  }, [apply]);

  return (
    <div
      ref={ref}
      // 탭 순서에 넣지 않는다. `tabIndex` 를 주지 않는 것이 이 판의 접근성 계약이다.
      aria-hidden="true"
      data-testid="colorpicker-field"
      className="relative h-28 w-full cursor-crosshair overflow-hidden rounded-md"
      style={{ backgroundColor: hsvToHex({ h: hsv.h, s: 1, v: 1, a: 1 }, { alpha: false }) }}
      onMouseDown={(e) => {
        dragging.current = true;
        apply(e.clientX, e.clientY);
      }}
    >
      {/* 가로는 흰색 → 투명(채도), 세로는 투명 → 검정(명도). 두 겹을 겹쳐 HSV 판이 된다. */}
      <div
        className="absolute inset-0"
        style={{ background: 'linear-gradient(to right, #ffffff, rgba(255,255,255,0))' }}
      />
      <div
        className="absolute inset-0"
        style={{ background: 'linear-gradient(to top, #000000, rgba(0,0,0,0))' }}
      />
      <span
        data-testid="colorpicker-field-handle"
        className="pointer-events-none absolute -ml-1.5 -mt-1.5 h-3 w-3 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(0,0,0,0.4)]"
        style={{ left: `${hsv.s * 100}%`, top: `${(1 - hsv.v) * 100}%` }}
      />
    </div>
  );
}
