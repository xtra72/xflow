// 윤곽 상자가 **여섯 갈래 전부에서 어떤 수를 내는가** (SPEC-CANVAS-011 M2 · AC-06).
//
// 011 M2 는 `outlineBox` 를 오버레이 밖 공유 모듈로 **옮긴다**. 옮기는 일이 값을 한 글자도
// 바꾸지 않았다는 것은 옮긴 **뒤에** 확인할 수 없다 — 그때는 비교할 이전 값이 이미 없다.
// 그래서 이 파일은 옮기기 **전에** 써서 초록으로 못박고, 옮긴 뒤 **한 글자도 고치지 않고**
// 다시 초록이어야 한다. 이 파일이 재는 것은 기능이 아니라 **이사의 무동작성**이다.
//
// ## 왜 DOM 을 지나 재는가
//
// 옮기기 전의 `outlineBox` 는 오버레이의 모듈 private 이라 직접 부를 수 없다. 그래서 그것이
// 세상에 남기는 **유일한 관측 가능한 흔적** — 선택 외곽선 `<div>` 의 네 인라인 스타일 —
// 으로 잰다. 옮긴 뒤에는 함수를 직접 부를 수 있게 되므로 `canvasOutline.test.ts` 가 같은
// 값을 값으로 다시 못박지만, 이 파일은 **그때도 지운 뒤 바꾸지 않는다**: 함수가 옳아도
// 오버레이가 그것을 부르지 않으면 화면은 여전히 틀리기 때문이다(이음매는 층 양쪽이
// 100% 여도 덮이지 않는다).
//
// ## 고정 입력을 고른 방식
//
// 스테이지 200×100 · 캔버스 500×400 → 축척 **가로 0.4 · 세로 0.25**. 축을 다르게 두어야
// 축을 뒤바꾼 계산이 드러난다(이웃 파일의 관용구 그대로).
//
// 여섯 상자의 변은 전부 `MIN_OUTLINE_PX`(2) 보다 **넉넉히 크다** — 렌더 자리의 clamp 가
// 대신 답해 버리면 이 파일은 `outlineBox` 가 아니라 `Math.max` 를 재게 된다. 유일한 예외는
// §실측 폭 결측인데, 그 경우의 폭 0 은 clamp 를 피할 수 없으므로 폭을 뺀 세 값만 잰다
// (폭 0 자체는 옮긴 뒤 `canvasOutline.test.ts` 가 값으로 잡는다).
//
// ## 선택을 몸짓이 아니라 provider 로 준다
//
// 클릭으로 고르면 이 파일은 히트 판정 · 그룹 진입 규칙 · z-order 까지 함께 재게 되고,
// 그중 하나가 바뀌면 **윤곽 상자와 무관한 이유로** 빨개진다. 여기서 재려는 것은 "고른 뒤
// 두르는 상자" 하나뿐이므로 선택은 provider 값으로 곧장 넣는다. 그래서 캔버스 클릭으로는
// 고를 수 없는 **그룹**(009 이후 클릭은 부품을 고른다)도 같은 표에 설 수 있다.

import { useMemo } from 'react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';

// i18n 은 키를 그대로 돌려준다(이웃 파일의 관용구).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { CanvasElement, CanvasSize } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import {
  CanvasEditSelectionContext,
  canvasAutoExpandedId,
  type CanvasSelection,
} from './canvasEditContext';
import type { StageSize } from './canvasGeometry';
import type { CanvasNode, GroupElement } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

const RECT: CanvasElement = {
  id: 'r',
  kind: 'rect',
  geometry: { x: 50, y: 40, w: 100, h: 160 },
  style: {},
};

const ELLIPSE: CanvasElement = {
  id: 'e',
  kind: 'ellipse',
  geometry: { x: 100, y: 80, w: 200, h: 120 },
  style: {},
};

/** 경로는 **rect 와 같은 상자 갈래다** — 명령 목록은 상자에 관여하지 않는다. */
const PATH: CanvasElement = {
  id: 'p',
  kind: 'path',
  geometry: { x: 150, y: 120, w: 250, h: 200 },
  path: [
    { c: 'M', x: 0, y: 0 },
    { c: 'L', x: 10000, y: 10000 },
  ],
  style: {},
};

const LINE: CanvasElement = {
  id: 'l',
  kind: 'line',
  geometry: { x1: 50, y1: 80, x2: 250, y2: 240 },
  style: {},
};

const TEXT: CanvasElement = {
  id: 't',
  kind: 'text',
  geometry: { x: 250, y: 200 },
  style: { fontSize: 20 },
  text: 'abc',
};

/** 가운데 정렬 — 기준점이 상자의 **가로 중심**이 되는 갈래(`resolveTextOrigin`). */
const TEXT_CENTER: CanvasElement = {
  id: 'tc',
  kind: 'text',
  geometry: { x: 250, y: 200 },
  style: { fontSize: 20, align: 'center' },
  text: 'abc',
};

/** 그룹의 윤곽은 **제 상자**다 — 부품의 합집합을 다시 재지 않는다. */
const GROUP: GroupElement = {
  id: 'g',
  kind: 'group',
  geometry: { x: 200, y: 160, w: 150, h: 80 },
  parts: [
    // 부품을 비워 두면 "합집합으로 재도 우연히 같다" 를 배제하지 못한다. 그룹 상자
    // **밖으로 비어져 나가는** 부품을 하나 둔다 — 합집합이었다면 윤곽이 더 커진다.
    { id: 'gp', kind: 'rect', geometry: { x: -500, y: -500, w: 3000, h: 3000 }, style: {} },
  ],
};

/** 001 은 음수 크기 박스를 그릴 수 있게 해 두었다 — 펴는 것은 `normalizeBox` 다. */
const NEGATIVE: CanvasElement = {
  id: 'n',
  kind: 'rect',
  geometry: { x: 250, y: 200, w: -100, h: -80 },
  style: {},
};

/** 아직 한 프레임도 그리지 않아 실측 폭이 없는 문구. */
const TEXT_UNMEASURED: CanvasElement = {
  id: 'tu',
  kind: 'text',
  geometry: { x: 250, y: 200 },
  style: { fontSize: 20 },
  text: 'abc',
};

const ELEMENTS: readonly CanvasNode[] = [
  RECT,
  ELLIPSE,
  PATH,
  LINE,
  TEXT,
  TEXT_CENTER,
  GROUP,
  NEGATIVE,
  TEXT_UNMEASURED,
];

/** 실측 글자 폭 장부 — `tu` 는 일부러 빠져 있다. */
const TEXT_WIDTHS: Readonly<Record<string, number>> = { t: 30, tc: 30 };

// --- 하네스 ---------------------------------------------------------------

/**
 * 선택을 provider 값으로 곧장 넣는 오버레이. `setSelection` 은 눈이 아니므로 no-op 이며,
 * 값 전체를 `useMemo` 로 고정해 오버레이의 효과 의존성이 렌더마다 흔들리지 않게 한다.
 */
function Harness({ selected }: { selected: string }) {
  const value = useMemo(() => {
    const selection: CanvasSelection = new Set([selected]);
    return {
      selection,
      setSelection: () => {},
      autoExpandedId: canvasAutoExpandedId(selection),
    };
  }, [selected]);
  return (
    <CanvasEditSelectionContext value={value}>
      {/* 도크 자리를 내지 않는다 — 도구는 도크에만 그려지므로 없으면 팔레트가 아예
          렌더되지 않고, 이 파일은 윤곽선만 든 화면을 본다. 팔레트를 띄우면 카탈로그
          썸네일이 jsdom 에 없는 `getContext` 를 두드려 재는 것과 무관한 소음이 쌓인다. */}
      <div className="relative">
        <CanvasEditOverlay
          enabled
          elements={ELEMENTS}
          projection={{ stage: STAGE, canvas: CANVAS }}
          textWidths={TEXT_WIDTHS}
          onElementsChange={() => {}}
        />
      </div>
    </CanvasEditSelectionContext>
  );
}

/** 고른 요소의 윤곽 네 변을 px 문자열로 읽는다. */
function outlineStyle(id: string): Record<string, string> {
  render(<Harness selected={id} />);
  const { style } = screen.getByTestId(`canvas-selection-${id}`);
  return { left: style.left, top: style.top, width: style.width, height: style.height };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 여섯 갈래 -------------------------------------------------------------

describe('윤곽 상자 — 여섯 갈래의 값 (AC-06)', () => {
  it('rect 는 투영한 상자 그대로다', () => {
    // 50·0.4=20 · 40·0.25=10 · 100·0.4=40 · 160·0.25=40
    expect(outlineStyle('r')).toEqual({
      left: '20px',
      top: '10px',
      width: '40px',
      height: '40px',
    });
  });

  it('ellipse 는 rect 와 같은 상자 갈래다', () => {
    // 100·0.4=40 · 80·0.25=20 · 200·0.4=80 · 120·0.25=30
    expect(outlineStyle('e')).toEqual({
      left: '40px',
      top: '20px',
      width: '80px',
      height: '30px',
    });
  });

  it('path 는 명령이 아니라 **요소 상자**를 두른다', () => {
    // 150·0.4=60 · 120·0.25=30 · 250·0.4=100 · 200·0.25=50.
    // 종전의 `default:` 로 떨어지면 기본 글자 크기의 작은 상자가 나왔다.
    expect(outlineStyle('p')).toEqual({
      left: '60px',
      top: '30px',
      width: '100px',
      height: '50px',
    });
  });

  it('line 은 두 끝점을 감싼다', () => {
    // (50,80)-(250,240) → (20,20)-(100,60)
    expect(outlineStyle('l')).toEqual({
      left: '20px',
      top: '20px',
      width: '80px',
      height: '40px',
    });
  });

  it('text 의 세로 기준은 **중심**이다 (TEXT_BASELINE === middle)', () => {
    // 기준점 (100,50). 상단은 50 - 20/2 = 40 이며, 상단으로 착각하면 50 이 된다.
    expect(outlineStyle('t')).toEqual({
      left: '100px',
      top: '40px',
      width: '30px',
      height: '20px',
    });
  });

  it('가운데 정렬 text 는 기준점이 가로 중심이다', () => {
    // 기준점 x 100, 폭 30 → 좌변 100 - 30/2 = 85
    expect(outlineStyle('tc')).toEqual({
      left: '85px',
      top: '40px',
      width: '30px',
      height: '20px',
    });
  });

  it('group 은 부품의 합집합이 아니라 **제 상자**다', () => {
    // 200·0.4=80 · 160·0.25=40 · 150·0.4=60 · 80·0.25=20.
    // 부품 `gp` 는 그룹 상자 밖으로 크게 비어져 나가 있으므로, 합집합으로 쟀다면
    // 이 네 수가 전부 달라진다.
    expect(outlineStyle('g')).toEqual({
      left: '80px',
      top: '40px',
      width: '60px',
      height: '20px',
    });
  });
});

// --- 경계 ------------------------------------------------------------------

describe('윤곽 상자 — 경계 (AC-06)', () => {
  it('음수 크기 상자를 양수 범위로 편다', () => {
    // (250,200) 에서 (-100,-80) → px (100,50) 에서 (-40,-20) → 편 뒤 (60,30,40,20)
    expect(outlineStyle('n')).toEqual({
      left: '60px',
      top: '30px',
      width: '40px',
      height: '20px',
    });
  });

  it('실측 폭이 아직 없으면 폭을 0 으로 본다 (AC-E7)', () => {
    // 폭만은 렌더 자리의 `MIN_OUTLINE_PX` clamp 가 2px 로 올려 답하므로 여기서 잴 수
    // 있는 것은 **나머지 셋이 폭과 무관하게 그대로라는 것**이다. 폭 0 자체는 옮긴 뒤
    // `canvasOutline.test.ts` 가 값으로 잡는다.
    const style = outlineStyle('tu');
    expect(style.left).toBe('100px');
    expect(style.top).toBe('40px');
    expect(style.height).toBe('20px');
    // clamp 가 먹은 결과. 0 이 그대로 나왔다면 이 줄이 알려 준다.
    expect(style.width).toBe('2px');
  });
});
