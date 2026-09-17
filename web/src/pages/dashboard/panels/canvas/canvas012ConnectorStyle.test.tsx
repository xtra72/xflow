// 연결선 겉모습 칸 (SPEC-CANVAS-012 M4 · AC-19~AC-23b).
//
// ## 이 파일이 지키는 셋
//
// **① 칸이 서고, 고친 값이 저장에 닿는다.** 렌더 경로는 011 이래 이미 서 있었으므로
// (`drawConnector` 가 `globalAlpha` 를 걸고 `paintStroke` 를 부른다) 012 가 세운 것은
// 칸뿐이다. 그 칸이 **끝까지 닿는지**가 여기서 재는 것이다.
//
// **② 안내문이 죽었다.** 그 문장은 고칠 칸이 없다는 사실의 대역이었고, 011 은 그것을 보는
// 시험을 **하나도 두지 않았다** — 그래서 지워도 아무도 울지 않았다. 012 는 그 자리를 채운다:
// 이제 안내문이 되살아나면 여기가 운다.
//
// **③ 층을 건넌다.** 칸에서 고른 값이 **저장을 지나 렌더 호출까지** 가는지를 한 단언이
// 꿴다. 편집기와 렌더러가 각각 100% 덮여도 그 사이 이음매는 덮이지 않는다 — 이 저장소가
// 이미 값을 치른 함정이다.
//
// 진짜 문구로 렌더한다(키 통과 스텁을 쓰지 않는다). 기본 로케일이 `ko` 라 `en` 쪽 누락이
// 특히 조용히 지나가므로 i18n 단언은 양쪽에서 돈다.
//
// @spec SPEC-CANVAS-012 REQ-02 · REQ-03 · REQ-05

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';
import { pickColorByTestId } from '@/test/pickColor';

import CanvasElementsEditor from './CanvasElementsEditor';
import { type CanvasNode } from './group/groupTypes';
import { type CanvasSize } from './canvasConfig';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import { drawElements, type DrawContext2D } from './drawElement';
import type { CanvasProjection } from './canvasGeometry';
import { STROKE_DASH_NAMES, dashPattern } from './strokeDash';

// --- 고정 입력 -------------------------------------------------------------

const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: { width: 250, height: 200 }, canvas: CANVAS };

/** 연결선 하나가 **둘째 자리**에 선다 — 첫 자리면 index 를 틀린 구현이 초록으로 지나간다. */
const ROW = 1;

const RAW_NODES: readonly Record<string, unknown>[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 80 }, style: {} },
  {
    id: 'c-1',
    kind: 'connector',
    from: { x: 10, y: 20 },
    to: { x: 300, y: 250 },
    route: 'straight',
  },
];

function config(): Record<string, unknown> {
  return { canvas: { ...CANVAS }, elements: RAW_NODES.map((n) => ({ ...n })) };
}

/**
 * 방출본을 **되먹이는** 껍데기 — 실제 앱과 같은 모양이다.
 *
 * 되먹이지 않으면 편집이 쌓이지 않는다: 매 칸이 처음 받은 `node` 위에 패치를 얹으므로
 * 마지막 편집 하나만 남고, 그 하네스로는 "두 칸을 잇달아 고쳤다" 를 잴 수 없다. 그것은
 * 제품의 성질이 아니라 **하네스의 거짓**이므로, 여기서 고친다.
 */
function Harness({ onEmit }: { onEmit: (patch: Record<string, unknown>) => void }) {
  const [cfg, setCfg] = useState<Record<string, unknown>>(() => config());
  return (
    <CanvasElementsEditor
      config={cfg}
      onConfigChange={(patch) => {
        onEmit(patch as Record<string, unknown>);
        setCfg((prev) => ({ ...prev, ...(patch as Record<string, unknown>) }));
      }}
    />
  );
}

function renderList(locale: 'ko' | 'en' = 'ko'): ReturnType<typeof vi.fn> {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  const onConfigChange = vi.fn();
  render(
    <I18nProvider>
      <Harness onEmit={onConfigChange} />
    </I18nProvider>,
  );
  fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW}`));
  return onConfigChange;
}

/** 마지막으로 방출된 연결선. 방출이 없으면 **여기서** 죽는다. */
function emittedConnector(spy: ReturnType<typeof vi.fn>): ConnectorElement {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls.at(-1)![0] as Record<string, unknown>;
  const nodes = patch.elements as CanvasNode[];
  const found = nodes.find((n) => n.id === 'c-1');
  expect(found).toBeDefined();
  if (found === undefined || !isConnector(found)) throw new Error('연결선이 사라졌다');
  return found;
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
  vi.restoreAllMocks();
});

// --- ① 칸이 선다 (AC-19 · AC-21 · AC-22) -----------------------------------

describe('겉모습 칸 넷이 선다 (AC-19)', () => {
  it.each([
    ['선색', 'stroke'],
    ['선 두께', 'stroke-width'],
    ['선 스타일', 'stroke-dash'],
    ['불투명도', 'opacity'],
  ])('%s 칸이 있다', (_label, tail) => {
    renderList();
    expect(screen.getByTestId(`canvas-connector-row-${tail}-${ROW}`)).toBeTruthy();
  });

  it('칸 넷이 **한 묶음** 안에 선다', () => {
    renderList();
    const group = screen.getByTestId(`canvas-connector-row-style-${ROW}`);
    for (const tail of ['stroke', 'stroke-width', 'stroke-dash', 'opacity']) {
      expect(group.contains(screen.getByTestId(`canvas-connector-row-${tail}-${ROW}`)), tail).toBe(
        true,
      );
    }
  });

  it('접힌 행에는 칸이 없다 — 몸통 안에 산다', () => {
    cleanup();
    globalThis.localStorage.clear();
    render(
      <I18nProvider>
        <CanvasElementsEditor config={config()} onConfigChange={vi.fn()} />
      </I18nProvider>,
    );
    expect(screen.queryByTestId(`canvas-connector-row-style-${ROW}`)).toBeNull();
  });
});

describe('읽기 전용 안내문이 없다 (AC-21 · REQ-02)', () => {
  it('그 문단이 렌더되지 않는다', () => {
    // **011 은 이 단언을 두지 않았다.** 그래서 그 문장은 지워도 아무도 울지 않는 자리에
    // 있었다. 012 가 그 자리를 채운다 — 이제 되살아나면 여기가 운다.
    renderList();
    expect(screen.queryByTestId(`canvas-connector-row-hint-${ROW}`)).toBeNull();
  });

  it.each(['ko', 'en'] as const)('%s 사전에서도 그 키가 죽었다', (locale) => {
    const tree = locale === 'ko' ? ko : en;
    expect(lookup(tree, 'dashboard.canvas.elements.connectorReadOnlyHint')).toBeUndefined();
  });

  it('끊김 안내는 이 절과 무관하다 — 좌표가 아니라 상태를 말한다', () => {
    // 살아 있는 선이라 끊김 안내는 애초에 서지 않는다. 그 키가 **사전에는 살아 있다**는
    // 것이 요점이다 — 012 가 걷은 것은 읽기 전용 변명 하나뿐이다.
    renderList();
    expect(screen.queryByTestId(`canvas-connector-row-broken-hint-${ROW}`)).toBeNull();
    expect(lookup(ko, 'dashboard.canvas.elements.connectorBrokenHint')).toBeTypeOf('string');
  });
});

// --- ② 고친 값이 저장에 닿는다 (AC-20) --------------------------------------

describe('고친 값이 저장에 닿는다 (AC-20 · REQ-03)', () => {
  it('선색', () => {
    const emit = renderList();
    pickColorByTestId(`canvas-connector-row-stroke-${ROW}`, '#ff8800');
    expect(emittedConnector(emit).style?.stroke).toBe('#ff8800');
  });

  it('선 두께', () => {
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-width-${ROW}`), {
      target: { value: '4' },
    });
    expect(emittedConnector(emit).style?.strokeWidth).toBe(4);
  });

  it.each(STROKE_DASH_NAMES)('선 스타일 `%s`', (name) => {
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: name },
    });
    expect(emittedConnector(emit).style?.strokeDash).toBe(name);
  });

  it('선 스타일 **미지정**은 키를 지운다 — `solid` 와 다른 값이다', () => {
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: 'dot' },
    });
    expect(emittedConnector(emit).style?.strokeDash).toBe('dot');
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: '' },
    });
    // 키 **자체가** 없어야 한다 — `undefined` 를 담아 두면 직렬화가 미지정과 `solid` 를
    // 가르지 못한다. 되먹이는 하네스라 이 단언은 "한 번 담겼던 키를 실제로 지웠는가" 다.
    expect('strokeDash' in (emittedConnector(emit).style ?? {})).toBe(false);
  });

  it('형제 스타일을 지우지 않는다 — 한 칸이 다른 칸을 먹지 않는다', () => {
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-width-${ROW}`), {
      target: { value: '4' },
    });
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: 'dash' },
    });
    // **두 번째 편집이 첫 번째를 들고 간다.** 하네스가 방출본을 되먹이므로 이 단언은
    // 진짜로 누적을 잰다 — 한 칸이 형제 칸을 먹으면 여기가 운다.
    const out = emittedConnector(emit);
    expect(out.style?.strokeWidth).toBe(4);
    expect(out.style?.strokeDash).toBe('dash');
    expect(out.route).toBe('straight');
    expect(out.id).toBe('c-1');
  });

  it('연결선의 다른 축을 건드리지 않는다 — 끝점과 그리는 법은 그대로다', () => {
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: 'dot' },
    });
    const out = emittedConnector(emit);
    expect(out.from).toEqual({ x: 10, y: 20 });
    expect(out.to).toEqual({ x: 300, y: 250 });
  });
});

// --- ③ 층을 건넌다 (AC-23) --------------------------------------------------

describe('칸 · 저장 · 렌더가 한 단언으로 꿰인다 (AC-23)', () => {
  it('칸에서 고른 `dot` 이 `setLineDash` 까지 간다', () => {
    // **이 단언이 이 파일의 이유다.** 편집기와 렌더러가 각각 100% 덮여도 그 사이 이음매는
    // 덮이지 않는다 — 칸이 `strokeDash` 를 쓰는데 렌더가 그 이름을 모르거나, 렌더가 아는
    // 이름을 칸이 다르게 적으면 양쪽 시험은 모두 초록인 채 화면만 틀린다.
    const emit = renderList();
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-width-${ROW}`), {
      target: { value: '3' },
    });
    fireEvent.change(screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`), {
      target: { value: 'dot' },
    });
    pickColorByTestId(`canvas-connector-row-stroke-${ROW}`, '#00ffff');

    const saved = emittedConnector(emit);
    expect(saved.style?.strokeDash).toBe('dot');

    // 저장된 그 노드를 **그대로** 그린다. 시험이 스타일을 다시 짓지 않는 것이 요점이다.
    const calls: [string, ...unknown[]][] = [];
    const rec =
      (op: string) =>
      (...args: unknown[]): void => {
        calls.push([op, ...args]);
      };
    const ctx = {
      save: rec('save'),
      restore: rec('restore'),
      setTransform: rec('setTransform'),
      beginPath: rec('beginPath'),
      rect: rec('rect'),
      ellipse: rec('ellipse'),
      moveTo: rec('moveTo'),
      lineTo: rec('lineTo'),
      closePath: rec('closePath'),
      bezierCurveTo: rec('bezierCurveTo'),
      setLineDash: rec('setLineDash'),
      stroke: rec('stroke'),
      fill: rec('fill'),
      fillText: rec('fillText'),
      clearRect: rec('clearRect'),
      fillRect: rec('fillRect'),
      measureText: (text: string) => ({ width: text.length * 10 }),
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 1,
      globalAlpha: 1,
      font: '',
      textAlign: 'left' as CanvasTextAlign,
      textBaseline: 'alphabetic' as CanvasTextBaseline,
    } satisfies DrawContext2D;

    drawElements(ctx, [saved], {}, {}, PROJ);

    const dashCalls = calls.filter((c) => c[0] === 'setLineDash').map((c) => c.slice(1));
    // 두께 3 의 `dot` 은 `[3, 6]` 이다 — 잎 모듈의 표에서 가져온다(시험이 산술을 두지 않는다).
    expect(dashCalls).toEqual([[dashPattern('dot', 3)]]);
  });
});

// --- ④ 012 가 더한 문구 (AC-23b) -------------------------------------------

describe('012 가 더한 키를 012 가 센다 (AC-23b)', () => {
  /**
   * **손으로 적은 리터럴이다.** 코드에서 유도하면 "코드가 부르지 않는 키" 를 셀 수 없고,
   * 문구를 지우면서 호출부를 함께 지운 변경이 가드를 **줄이면서** 초록으로 남는다.
   *
   * 011 의 `ADDED` 에 얹지 않는 것에 뜻이 있다 — 그 목록의 뜻은 "011 이 더한 키" 이고,
   * 섞으면 한 목록이 두 SPEC 을 책임진다.
   */
  const ADDED: readonly string[] = [
    'dashboard.canvas.elements.connectorStyleLabel',
    'dashboard.canvas.elements.connectorStrokeAria',
    'dashboard.canvas.elements.connectorStrokeWidthAria',
    'dashboard.canvas.elements.connectorStrokeDashAria',
    'dashboard.canvas.elements.connectorOpacityAria',
    'dashboard.canvas.elements.strokeDashSolid',
    'dashboard.canvas.elements.strokeDashDash',
    'dashboard.canvas.elements.strokeDashDot',
    'dashboard.canvas.elements.strokeDashDashDot',
  ].map((k) => k);

  it('세어서 적는다 — 아홉이다', () => {
    expect(ADDED).toHaveLength(9);
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

  it.each(ADDED)('%s 의 두 로케일이 서로 다르다 — 한쪽을 베껴 넣지 않았다', (key) => {
    expect(lookup(ko, key), key).not.toBe(lookup(en, key));
  });

  it('접근성 이름 넷이 **양쪽 로케일에** 번호 치환자를 들고 있다', () => {
    // 기본 로케일이 `ko` 라 en 쪽 누락이 특히 조용히 지나간다. 빠지면 연결선이 둘 이상일
    // 때 네 칸의 이름이 **구별 불가능**해진다.
    const ARIA = [
      'dashboard.canvas.elements.connectorStrokeAria',
      'dashboard.canvas.elements.connectorStrokeWidthAria',
      'dashboard.canvas.elements.connectorStrokeDashAria',
      'dashboard.canvas.elements.connectorOpacityAria',
    ];
    for (const key of ARIA) {
      for (const [name, tree] of [
        ['ko', ko],
        ['en', en],
      ] as const) {
        expect(lookup(tree, key) as string, `${name}:${key}`).toContain('{index}');
      }
    }
  });

  it.each(['ko', 'en'] as const)('%s — 선 스타일 선택지가 **번역된 글자**를 낸다', (locale) => {
    renderList(locale);
    const select = screen.getByTestId(`canvas-connector-row-stroke-dash-${ROW}`);
    const text = select.textContent ?? '';
    // 키가 그대로 새는 형상은 점이 든 문자열이다 — 그것을 직접 막는다.
    expect(text).not.toContain('dashboard.');
    for (const name of STROKE_DASH_NAMES) {
      const label = lookup(
        locale === 'ko' ? ko : en,
        `dashboard.canvas.elements.strokeDash${name.charAt(0).toUpperCase()}${name.slice(1)}`,
      );
      expect(label, name).toBeTypeOf('string');
      expect(text, `${locale}:${name}`).toContain(label as string);
    }
  });
});
