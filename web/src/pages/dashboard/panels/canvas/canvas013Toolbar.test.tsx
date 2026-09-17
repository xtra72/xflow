// 변환 절이 띠에 선다 (SPEC-CANVAS-013 M5 · AC-25~AC-29).
//
// **이 파일이 재는 것은 산술이 아니라 배선이다.** 좌표가 맞아도 단추가 그 함수를 부르지
// 않으면 화면은 그대로이고, 반대로 단추가 켜져 있는데 눌러도 아무 일이 없으면 사용자에게는
// 고장으로 보인다. 그래서 여기서는 **켜짐 조건**과 **눌렀을 때 실제로 바뀌는가**를 잰다.
//
// 진짜 문구로 렌더한다(키 통과 스텁을 쓰지 않는다). 기본 로케일이 `ko` 라 `en` 쪽 누락이
// 특히 조용히 지나가므로 i18n 단언은 양쪽에서 돈다.
//
// @spec SPEC-CANVAS-013 REQ-01 · REQ-02 · REQ-05

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditDockRegion } from './CanvasEditDock';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasElement } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';
import type { ConnectorElement } from './connector/connectorTypes';

const PROJ: CanvasProjection = { stage: { width: 250, height: 200 }, canvas: { width: 500, height: 400 } };

/** 단추 넷의 testId 꼬리. **여덟이 아니다** — 거울은 둘뿐이다. */
const BUTTONS = ['flip-x', 'flip-y', 'rotate-ccw', 'rotate-cw'] as const;

function rect(id: string, x: number, y: number): CanvasElement {
  return { id, kind: 'rect', style: {}, geometry: { x, y, w: 40, h: 20 } };
}

function link(): ConnectorElement {
  return {
    id: 'c1',
    kind: 'connector',
    from: { x: 10, y: 20 },
    to: { x: 300, y: 250 },
    route: 'straight',
  };
}

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      {/* 여럿 고르기는 Ctrl + 마키 몸짓인데, 그 몸짓을 흉내 내면 이 파일이 재려는 것
          (띠 배선)이 아니라 **마키**를 재게 된다. 선택 상태를 직접 세워 관심사를 가른다 —
          마키 자체는 006 의 시험이 이미 진다. */}
      <button
        type="button"
        data-testid="select-all"
        onClick={() => state.setSelection(new Set(elements.map((n) => n.id)))}
      >
        all
      </button>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <CanvasEditDockRegion enabled>
        <div data-testid="scaled-panel">
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={PROJ}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </div>
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function live(): CanvasNode[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasNode[];
}

function show(nodes: readonly CanvasNode[], locale: 'ko' | 'en' = 'ko'): void {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  render(
    <I18nProvider>
      <Harness initial={nodes} />
    </I18nProvider>,
  );
}

function button(tail: string): HTMLButtonElement {
  return screen.getByTestId(`canvas-transform-${tail}`) as HTMLButtonElement;
}

const lookup = (tree: unknown, key: string): unknown =>
  key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
      tree,
    );

beforeEach(() => {
  globalThis.localStorage.clear();
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

// --- ① 단추가 넷이다 (AC-25 · AC-28) ---------------------------------------

describe('변환 절 (AC-25 · AC-28)', () => {
  it('단추가 **넷**이다 — 여덟이 아니다', () => {
    show([rect('a', 0, 0)]);
    for (const tail of BUTTONS) expect(button(tail)).toBeTruthy();
    // 위로 뒤집기와 아래로 뒤집기는 **같은 거울**이다. 갈라 두면 눌러도 결과가 같은
    // 단추가 둘 생긴다.
    expect(screen.queryByTestId('canvas-transform-flip-up')).toBeNull();
    expect(screen.queryByTestId('canvas-transform-flip-down')).toBeNull();
  });

  it('절이 **정렬 · 순서 옆**에 선다 (AC-28)', () => {
    show([rect('a', 0, 0)]);
    const band = screen.getByTestId('canvas-toolbar-panel');
    const kids = [...band.children];
    const at = (id: string): number =>
      kids.findIndex((k) => k.contains(screen.getByTestId(id)));
    // 정렬 → 순서 → 변환 → 그룹. 넷 다 **선택 위에서 도는 연산**이라 붙어 있어야
    // "고른 것에 무엇을 할 수 있는가" 가 한눈에 읽힌다.
    expect(at('canvas-align-left')).toBeLessThan(at('canvas-order-front'));
    expect(at('canvas-order-front')).toBeLessThan(at('canvas-transform-flip-x'));
    expect(at('canvas-transform-flip-x')).toBeLessThan(at('canvas-group-create'));
  });
});

// --- ② 켜짐 조건 (AC-26 · AC-27) -------------------------------------------

describe('켜짐 조건은 정렬과 **다른 자**다 (AC-26 · AC-27 · REQ-05)', () => {
  it('아무것도 고르지 않으면 꺼진다 (AC-27)', () => {
    show([rect('a', 0, 0)]);
    for (const tail of BUTTONS) expect(button(tail).disabled, tail).toBe(true);
  });

  it('**하나만** 골라도 켜진다 — 정렬은 그때 꺼져 있다 (AC-26)', () => {
    // 팔레트로 하나를 놓으면 그것이 곧 골라진다(006 이 세운 동작).
    show([]);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    for (const tail of BUTTONS) expect(button(tail).disabled, tail).toBe(false);
    // 같은 상태에서 정렬은 **꺼져 있다**. 이 짝단언이 "다른 자" 의 전부다.
    expect((screen.getByTestId('canvas-align-left') as HTMLButtonElement).disabled).toBe(true);
  });

  it('연결선만 골라 놓으면 꺼진다 — 뒤집을 축이 없다', () => {
    // 연결선에는 `geometry` 가 없어 윤곽 상자가 없다. 단추의 활성과 실제 동작이 **같은
    // 사실**(`selectionBox` 가 `undefined`)을 보아야 "켜졌는데 아무 일이 없다" 가 생기지
    // 않는다.
    show([link()]);
    for (const tail of BUTTONS) expect(button(tail).disabled, tail).toBe(true);
  });
});

// --- ③ 눌렀을 때 실제로 바뀐다 (REQ-01 · REQ-02) ----------------------------

describe('눌렀을 때 저장이 실제로 바뀐다', () => {
  function placedTwo(): void {
    show([]);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    fireEvent.click(screen.getByTestId('canvas-palette-add-ellipse'));
  }

  it('둘을 골라 가로로 뒤집으면 **자리를 맞바꾼다**', () => {
    placedTwo();
    fireEvent.click(screen.getByTestId('select-all'));
    const before = live();
    const xs = before.map((n) => ('geometry' in n && 'w' in n.geometry ? n.geometry.x : null));
    fireEvent.click(button('flip-x'));
    const after = live();
    expect(after).toHaveLength(before.length);
    // 왼쪽 것이 오른쪽으로, 오른쪽 것이 왼쪽으로 — §결정 1 의 전부다.
    const nx = after.map((n) => ('geometry' in n && 'w' in n.geometry ? n.geometry.x : null));
    expect(nx).not.toEqual(xs);
  });

  it('**대칭 도형 하나**는 뒤집어도 그대로다 — 그리고 그것이 옳다', () => {
    // 팔레트가 놓은 사각형 하나는 제 상자 안에서 좌우가 같다. 거울은 그것을 제자리에
    // 비추므로 저장이 한 글자도 바뀌지 않는다. "단추가 고장났다" 가 아니라 **거울의
    // 성질**이며, 이 단언이 없으면 훗날 누군가 이 무동작을 결함으로 읽고 고치려 든다.
    show([]);
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    const before = JSON.stringify(live());
    fireEvent.click(button('flip-x'));
    expect(JSON.stringify(live())).toBe(before);
  });

  it('네 번 돌리면 제자리다 (K1)', () => {
    placedTwo();
    fireEvent.click(screen.getByTestId('select-all'));
    const before = JSON.stringify(live());
    for (let i = 0; i < 4; i += 1) fireEvent.click(button('rotate-cw'));
    expect(JSON.stringify(live())).toBe(before);
  });

  it('오른쪽 한 번 → 왼쪽 한 번이 제자리다 (K1`)', () => {
    placedTwo();
    fireEvent.click(screen.getByTestId('select-all'));
    const before = JSON.stringify(live());
    fireEvent.click(button('rotate-cw'));
    expect(JSON.stringify(live())).not.toBe(before);
    fireEvent.click(button('rotate-ccw'));
    expect(JSON.stringify(live())).toBe(before);
  });
});

// --- ④ 문구 (AC-29) --------------------------------------------------------

describe('013 이 더한 키를 013 이 센다 (AC-29)', () => {
  /** 손으로 적은 리터럴이다 — 코드에서 유도하면 "코드가 부르지 않는 키" 를 셀 수 없다. */
  const ADDED: readonly string[] = [
    'dashboard.canvas.edit.dockTransform',
    'dashboard.canvas.edit.transformFlipX',
    'dashboard.canvas.edit.transformFlipY',
    'dashboard.canvas.edit.transformRotateLeft',
    'dashboard.canvas.edit.transformRotateRight',
  ];

  it('세어서 적는다 — 다섯이다', () => {
    expect(ADDED).toHaveLength(5);
  });

  it.each(ADDED)('%s 가 두 로케일에서 비어 있지 않은 문자열이다', (key) => {
    for (const [name, tree] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      const value = lookup(tree, key);
      expect(typeof value, `${name}:${key}`).toBe('string');
      expect((value as string).trim().length, `${name}:${key}`).toBeGreaterThan(0);
    }
  });

  it.each(ADDED)('%s 의 두 로케일이 서로 다르다', (key) => {
    expect(lookup(ko, key), key).not.toBe(lookup(en, key));
  });

  it.each(['ko', 'en'] as const)('%s — 단추가 **번역된 이름**을 낸다', (locale) => {
    show([rect('a', 0, 0)], locale);
    const tree = locale === 'ko' ? ko : en;
    const pairs: readonly (readonly [string, string])[] = [
      ['flip-x', 'dashboard.canvas.edit.transformFlipX'],
      ['flip-y', 'dashboard.canvas.edit.transformFlipY'],
      ['rotate-ccw', 'dashboard.canvas.edit.transformRotateLeft'],
      ['rotate-cw', 'dashboard.canvas.edit.transformRotateRight'],
    ];
    for (const [tail, key] of pairs) {
      const label = lookup(tree, key);
      expect(label, key).toBeTypeOf('string');
      expect(button(tail).getAttribute('aria-label'), `${locale}:${tail}`).toBe(label);
    }
  });
});
