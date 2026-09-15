// 윤곽 상자를 **값으로** 못박는다 (SPEC-CANVAS-011 M2 · AC-06 · AC-08).
//
// `CanvasEditOverlay.outline.test.tsx` 는 같은 여섯 갈래를 **화면을 지나** 잰다. 두 파일을
// 함께 두는 이유는 시험 규율 D6 그대로다 — 순수 산술은 jsdom 없이도 참이고, 그 값이 화면에
// 닿는가는 산술이 참이어도 거짓일 수 있다. 여기가 함수를, 저기가 이음매를 맡는다.
//
// 이 파일만이 잴 수 있는 것이 하나 있다: **렌더 자리의 `MIN_OUTLINE_PX` clamp 에 가려지는
// 값들**이다. 실측 폭이 없는 문구의 폭 0 과 크기 0 인 퇴화 도형은 화면에서 언제나 2px 로
// 올라오므로, 0 이라는 사실은 함수를 직접 불러야만 보인다.
//
// 고정 입력은 이웃 파일과 **같은 투영**이다 — 스테이지 200×100 · 캔버스 500×400 → 축척
// 가로 0.4 · 세로 0.25. 두 파일이 같은 수를 말하는지 눈으로 견줄 수 있어야 한다.

import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import type { GroupElement } from './group/groupTypes';
import { outlineBox, resolveFontSize, resolveMeasuredWidth } from './canvasOutline';

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { width: 500, height: 400 },
};

/** 글자 폭 장부가 비었다는 뜻 — 아직 한 프레임도 그리지 않은 상태다. */
const NO_WIDTHS: Readonly<Record<string, number>> = {};

function text(style: CanvasElement['style'], id = 't'): CanvasElement {
  return { id, kind: 'text', geometry: { x: 250, y: 200 }, style, text: 'abc' };
}

// --- 여섯 갈래 -------------------------------------------------------------

describe('outlineBox — 여섯 갈래 (AC-06)', () => {
  it('rect 는 투영한 상자 그대로다', () => {
    const el: CanvasElement = { id: 'r', kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 160 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 20, y: 10, w: 40, h: 40 });
  });

  it('ellipse 는 rect 와 같은 상자 갈래다', () => {
    const el: CanvasElement = { id: 'e', kind: 'ellipse', geometry: { x: 100, y: 80, w: 200, h: 120 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 40, y: 20, w: 80, h: 30 });
  });

  it('path 는 명령이 아니라 **요소 상자**를 두른다', () => {
    // 명령을 로컬 격자 한구석에만 몰아 두어도 상자는 요소 기하 그대로다 — 잉크의
    // 경계 상자를 재는 것이 아니다.
    const el: CanvasElement = {
      id: 'p',
      kind: 'path',
      geometry: { x: 150, y: 120, w: 250, h: 200 },
      path: [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 1, y: 1 },
      ],
      style: {},
    };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 60, y: 30, w: 100, h: 50 });
  });

  it('line 은 두 끝점을 감싼다', () => {
    const el: CanvasElement = { id: 'l', kind: 'line', geometry: { x1: 50, y1: 80, x2: 250, y2: 240 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 20, y: 20, w: 80, h: 40 });
  });

  it('text 의 세로 기준은 **중심**이다 (TEXT_BASELINE === middle)', () => {
    // 기준점 (100,50). 상단은 50 - 20/2 = 40 이며, 상단으로 착각하면 50 이 된다.
    expect(outlineBox(text({ fontSize: 20 }), PROJ, { t: 30 })).toEqual({ x: 100, y: 40, w: 30, h: 20 });
  });

  it('group 은 부품의 합집합이 아니라 **제 상자**다', () => {
    const el: GroupElement = {
      id: 'g',
      kind: 'group',
      geometry: { x: 200, y: 160, w: 150, h: 80 },
      // 그룹 상자 밖으로 크게 비어져 나간 부품. 합집합으로 쟀다면 네 수가 전부 달라진다.
      parts: [{ id: 'gp', kind: 'rect', geometry: { x: -500, y: -500, w: 3000, h: 3000 }, style: {} }],
    };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 80, y: 40, w: 60, h: 20 });
  });
});

// --- 정규화 ----------------------------------------------------------------

describe('outlineBox — 음수·퇴화 (AC-06)', () => {
  it('음수 크기 상자를 양수 범위로 편다', () => {
    const el: CanvasElement = { id: 'n', kind: 'rect', geometry: { x: 250, y: 200, w: -100, h: -80 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 60, y: 30, w: 40, h: 20 });
  });

  it('끝점을 거꾸로 적은 선도 같은 상자를 낸다', () => {
    const el: CanvasElement = { id: 'l', kind: 'line', geometry: { x1: 250, y1: 240, x2: 50, y2: 80 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 20, y: 20, w: 80, h: 40 });
  });

  it('크기 0 인 도형은 **0 을 그대로** 낸다 — 2px 는 렌더 자리의 clamp 다', () => {
    const el: CanvasElement = { id: 'z', kind: 'rect', geometry: { x: 250, y: 200, w: 0, h: 0 }, style: {} };
    expect(outlineBox(el, PROJ, NO_WIDTHS)).toEqual({ x: 100, y: 50, w: 0, h: 0 });
  });
});

// --- 문구의 폭과 크기 ------------------------------------------------------

describe('outlineBox — 문구의 실측 폭 (AC-E7)', () => {
  it('장부에 없으면 폭 0 이다 — 화면에서는 clamp 에 가려지는 값이다', () => {
    expect(outlineBox(text({ fontSize: 20 }), PROJ, NO_WIDTHS)).toEqual({ x: 100, y: 40, w: 0, h: 20 });
  });

  it('장부의 값이 0 · 음수 · 유한하지 않으면 모두 0 으로 본다', () => {
    const el = text({ fontSize: 20 });
    for (const bad of [0, -30, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(outlineBox(el, PROJ, { t: bad }).w).toBe(0);
    }
  });

  it('가운데 정렬은 기준점이 가로 중심이다', () => {
    expect(outlineBox(text({ fontSize: 20, align: 'center' }), PROJ, { t: 30 })).toEqual({
      x: 85,
      y: 40,
      w: 30,
      h: 20,
    });
  });

  it('오른쪽 정렬은 기준점이 우변이다', () => {
    expect(outlineBox(text({ fontSize: 20, align: 'right' }), PROJ, { t: 30 })).toEqual({
      x: 70,
      y: 40,
      w: 30,
      h: 20,
    });
  });

  it('글자 크기가 없거나 못 쓸 값이면 기본값 14 로 선다', () => {
    // 상단 50 - 14/2 = 43. 높이가 20 으로 나오면 폴백을 타지 않은 것이다.
    for (const bad of [undefined, 0, -20, Number.NaN]) {
      expect(outlineBox(text({ fontSize: bad }), PROJ, { t: 30 })).toEqual({
        x: 100,
        y: 43,
        w: 30,
        h: 14,
      });
    }
  });

  it('장부에 다른 id 의 폭만 있으면 제 폭은 0 이다', () => {
    // 장부를 id 로 키잉한다는 사실 자체를 잡는다 — 아무 값이나 집어 오면 여기가 빨개진다.
    expect(outlineBox(text({ fontSize: 20 }), PROJ, { other: 999 }).w).toBe(0);
  });
});

// --- 함께 옮겨 온 도우미 둘 ------------------------------------------------

describe('resolveFontSize · resolveMeasuredWidth', () => {
  it('글자 크기는 유한 양수만 그대로 쓴다', () => {
    expect(resolveFontSize(20)).toBe(20);
    expect(resolveFontSize(undefined)).toBe(14);
    expect(resolveFontSize(0)).toBe(14);
    expect(resolveFontSize(-1)).toBe(14);
    expect(resolveFontSize(Number.NaN)).toBe(14);
    expect(resolveFontSize(Number.POSITIVE_INFINITY)).toBe(14);
  });

  it('실측 폭은 유한 양수만 그대로 쓰고 나머지는 0 이다', () => {
    expect(resolveMeasuredWidth(30)).toBe(30);
    expect(resolveMeasuredWidth(undefined)).toBe(0);
    expect(resolveMeasuredWidth(0)).toBe(0);
    expect(resolveMeasuredWidth(-30)).toBe(0);
    expect(resolveMeasuredWidth(Number.NaN)).toBe(0);
    expect(resolveMeasuredWidth(Number.POSITIVE_INFINITY)).toBe(0);
  });
});

// --- 모듈 경계 -------------------------------------------------------------
//
// 아래 둘은 동작이 아니라 **형상**을 잰다 — 어겨도 화면은 한동안 멀쩡하고, 어긋남은
// 한참 뒤 다른 결함으로만 드러난다(009 가드가 세운 그 규율 그대로). 주석은 걷어내고
// 읽는다: 산문에 적힌 낱말이 가드를 헛되이 울리면 다음 사람은 주석을 고쳐 지나가고,
// 그때 가드는 이미 죽은 것이다.

const CANVAS_DIR = __dirname;

function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

describe('모듈의 형상 (AC-07 · AC-08)', () => {
  it('canvasOutline 은 document · window 를 모른다 (AC-08)', () => {
    // 이 모듈은 투영과 글자 폭 장부만 받는다. 스스로 재기 시작하면 측정원이 둘이 되고,
    // 그 순간 렌더 없이 값으로 시험할 수도 없게 된다.
    const src = stripComments(fs.readFileSync(path.join(CANVAS_DIR, 'canvasOutline.ts'), 'utf8'));
    expect(src).not.toMatch(/\bdocument\b/);
    expect(src).not.toMatch(/\bwindow\b/);
  });

  it('outlineBox 의 정의가 캔버스 전체에 **한 자리**뿐이다 (AC-07)', () => {
    // 두 벌이 되면 "보이는 상자와 선이 붙는 상자가 다르다" 가 표현 가능해진다 —
    // M2 가 이사를 감행한 유일한 이유가 그것이다(위험 R1).
    const found: string[] = [];
    const walk = (dir: string): void => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(full);
          continue;
        }
        if (!/\.tsx?$/.test(entry.name) || /\.test\.tsx?$/.test(entry.name)) continue;
        if (/function outlineBox\b/.test(stripComments(fs.readFileSync(full, 'utf8')))) {
          found.push(path.relative(CANVAS_DIR, full));
        }
      }
    };
    walk(CANVAS_DIR);
    expect(found).toEqual(['canvasOutline.ts']);
  });
});
