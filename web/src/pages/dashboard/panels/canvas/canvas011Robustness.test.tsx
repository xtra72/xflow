// 011 의 견고성과 회귀 (SPEC-CANVAS-011 M12 · REQ-08 · REQ-09 · AC-78~AC-87).
//
// ## 이 파일이 지키는 셋
//
// **① 끊긴 연결은 살아남고, 화면이 그 사실을 말한다.** 그리지 않되 **버리지 않는다** —
// 조용히 사라지는 저술이 008 이 이름 적은 가장 나쁜 실패이고, 그리지 않기만 하고 말하지
// 않으면 사용자에게는 사라진 것과 구별되지 않는다. 그래서 여기서 재는 것은 "예외가 없다"
// 만이 아니라 **목록의 그 행이 끊겼다고 말하는가** 다.
//
// **② 지우면 함께 간다.** 요소를 지울 때 그것을 가리키던 연결선이 남으면 끊긴 연결이
// **저술하지 않은 채** 생긴다. 두 입구(캔버스의 Delete·목록의 휴지통)가 같은 함수를
// 지나는지도 함께 잰다 — 한쪽만 연동하면 "캔버스에서 지우면 선이 따라가는데 목록에서
// 지우면 끊긴 선이 남는다" 가 되고, 그 차이는 저장된 뒤에야 드러난다.
//
// **③ 뒤집지 않은 것은 그대로다.** 011 은 최상위 노드에 갈래 하나를 더했을 뿐이고,
// 008 의 경로 어휘 · 009 의 선택 모델 · 004 의 부품 타입 · 연결선을 쓰지 않은 config 의
// 결과는 한 글자도 달라지지 않았다. 그 "달라지지 않음" 은 저절로 참이 아니라 **재야**
// 참이다.
//
// ## 진짜 문구로 렌더한다 (그리고 양쪽 로케일에서)
//
// 키 통과 스텁을 쓰지 않는다. 이 파일이 재는 것 가운데 하나가 "행이 두 끝을 **말한다**"
// 이고, 스텁은 치환자를 채우지 않으므로 그 단언이 키 문자열만 보게 된다. 기본 로케일이
// `ko` 라 en 쪽 누락이 특히 조용히 지나가므로 양쪽에서 같은 단언을 돌린다.
//
// @spec SPEC-CANVAS-011 REQ-08 · REQ-09 · AC-78~AC-87

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasEditOverlay from './CanvasEditOverlay';
import CanvasElementsEditor from './CanvasElementsEditor';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { parseCanvasConfig, type CanvasElement, type CanvasSize } from './canvasConfig';
import { removeNodes, removeNodesWithConnectors } from './canvasEditArrange';
import type { CanvasProjection } from './canvasGeometry';
import { hitTest } from './canvasHitTest';
import { drawElements, type DrawContext2D } from './drawElement';
import { connectorTargets, resolveConnector } from './connector/resolveConnector';
import type { ConnectorElement } from './connector/connectorTypes';
import { frameKey, parseFrameKey, walkDrawables } from './group/frameKey';
import { GROUP_LOCAL_EXTENT, type CanvasNode, type GroupElement } from './group/groupTypes';
import type { PathCommand } from './shapes/pathTypes';

// --- 고정 입력 -----------------------------------------------------------

const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: { width: 250, height: 200 }, canvas: CANVAS };

/**
 * 장면 하나 — **살아 있는 선 셋과 끊긴 선 셋**을 한 배열에 담는다.
 *
 * 끊기는 경우를 셋 다 담는 것이 요점이다(`resolveConnector` 가 이름 적은 넷 가운데 셋 —
 * 넷째인 "연결선을 가리킨다" 는 저술로 만들 수 있으나 A6 이 금지한 형상이라 여기 두지
 * 않는다). 하나만 담으면 "요소가 없을 때만" 끊김으로 읽는 구현이 초록으로 지나간다:
 *
 *   - `c-gone`   — 가리킨 id 의 노드가 없다(지워졌다).
 *   - `c-anchor` — 요소는 있는데 그 **이름의 앵커 자리**가 없다(임의 앵커를 뺐다).
 *   - `c-part`   — 그룹 **부품 복합 키**를 가리킨다(A3 — 최상위에 그 id 가 없다).
 *
 * 날것 JSON 인 것에도 뜻이 있다. 편집기는 제 손으로 `parseCanvasConfig` 를 부르므로,
 * 이 배열이 파서를 지나는 것 자체가 "끊겨도 버려지지 않는다"(AC-79)의 절반이다.
 */
const RAW_NODES: readonly Record<string, unknown>[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 80 }, style: {} },
  { id: 'el-2', kind: 'ellipse', geometry: { x: 300, y: 200, w: 60, h: 40 }, style: {} },
  {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 20, y: 300, w: 80, h: 60 },
    parts: [
      {
        id: 'body',
        kind: 'rect',
        geometry: { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT },
        style: {},
      },
    ],
  },
  { id: 'c-live', kind: 'connector', from: { el: 'el-1', a: 'e' }, to: { el: 'el-2', a: 'w' } },
  {
    id: 'c-gone',
    kind: 'connector',
    route: 'elbow',
    from: { el: 'ghost', a: 'c' },
    to: { el: 'el-2', a: 'w' },
    points: [{ x: 200, y: 120 }],
  },
  {
    id: 'c-anchor',
    kind: 'connector',
    route: 'curve',
    from: { el: 'el-1', a: 'no-such-anchor' },
    to: { el: 'el-2', a: 'w' },
  },
  {
    id: 'c-part',
    kind: 'connector',
    route: 'free',
    from: { el: 'grp-1/body', a: 'c' },
    to: { el: 'el-2', a: 'w' },
  },
  { id: 'c-free', kind: 'connector', from: { x: 10, y: 20 }, to: { x: 30, y: 40 } },
  { id: 'c-grp', kind: 'connector', from: { el: 'grp-1', a: 'c' }, to: { el: 'el-2', a: 'w' } },
];

/** 위 배열의 **행 자리**. 목록의 `idx` 는 노드 배열의 자리 그대로다. */
const ROW = {
  el1: 0,
  el2: 1,
  grp: 2,
  live: 3,
  gone: 4,
  anchor: 5,
  part: 6,
  free: 7,
  grpLine: 8,
} as const;

/** 끊긴 셋과 서는 셋. 아래 시험들이 **양쪽을** 함께 재는 근거 목록이다. */
const BROKEN_ROWS = [ROW.gone, ROW.anchor, ROW.part] as const;
const INTACT_ROWS = [ROW.live, ROW.free, ROW.grpLine] as const;

function config(nodes: readonly Record<string, unknown>[] = RAW_NODES): Record<string, unknown> {
  return { canvas: { ...CANVAS }, elements: nodes.map((n) => ({ ...n })) };
}

/** 파싱을 지난 장면. 순수 층 시험이 쓰는 것과 편집기가 만드는 것이 **같은 배열**이다. */
function scene(): CanvasNode[] {
  return parseCanvasConfig(config()).elements;
}

function nodeById(nodes: readonly CanvasNode[], id: string): CanvasNode {
  const found = nodes.find((n) => n.id === id);
  expect(found, id).toBeDefined();
  return found!;
}

function connectorById(nodes: readonly CanvasNode[], id: string): ConnectorElement {
  return nodeById(nodes, id) as ConnectorElement;
}

// --- 그리기 스텁 ----------------------------------------------------------

/**
 * 부르는 이름만 적는 캔버스 — **던지는지**와 **잉크를 내렸는지**만 본다.
 *
 * 좌표까지 재는 두꺼운 기록기는 `drawElement.connector.test.ts` 가 이미 들고 있다.
 * 여기서 겨누는 것은 M6 의 그림이 아니라 REQ-08 의 "예외를 내서는 안 된다" 이다.
 */
function makeStubCtx(): DrawContext2D & { readonly ops: string[] } {
  const ops: string[] = [];
  const rec =
    (op: string) =>
    (...args: unknown[]): void => {
      void args;
      ops.push(op);
    };
  return {
    ops,
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
    textAlign: 'left',
    textBaseline: 'alphabetic',
  };
}

// --- 목록 렌더 하네스 ------------------------------------------------------

const LOCALES = { ko, en } as const;
type LocaleName = keyof typeof LOCALES;
const LOCALE_NAMES: readonly LocaleName[] = ['ko', 'en'];

/** 점 표기 키를 트리에서 조회한다 — `t()` 와 같은 규칙이다(문자열이 아니면 부재). */
function lookup(tree: unknown, key: string): string | undefined {
  const value = key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node !== null && typeof node === 'object'
          ? (node as Record<string, unknown>)[seg]
          : undefined,
      tree,
    );
  return typeof value === 'string' ? value : undefined;
}

function renderList(
  locale: LocaleName = 'ko',
  nodes: readonly Record<string, unknown>[] = RAW_NODES,
): ReturnType<typeof vi.fn> {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  const onConfigChange = vi.fn();
  render(
    <I18nProvider>
      <CanvasElementsEditor config={config(nodes)} onConfigChange={onConfigChange} />
    </I18nProvider>,
  );
  return onConfigChange;
}

/** 마지막으로 방출된 최상위 배열. */
function lastElements(spy: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls.at(-1)![0] as Record<string, unknown>;
  return patch.elements as CanvasNode[];
}

beforeEach(() => {
  globalThis.localStorage.clear();
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
  vi.restoreAllMocks();
});

// --- ① 끊긴 연결 — 예외가 없다 (REQ-08) ------------------------------------

describe('끊긴 참조가 어디서도 던지지 않는다 (REQ-08)', () => {
  it('해석은 **부재**를 낸다 — 반쪽 목록도 빈 배열도 아니다 (AC-43 · AC-44)', () => {
    const nodes = scene();
    for (const id of ['c-gone', 'c-anchor', 'c-part']) {
      expect(resolveConnector(connectorById(nodes, id), nodes, PROJ, {}), id).toBeUndefined();
    }
    for (const id of ['c-live', 'c-free', 'c-grp']) {
      expect(resolveConnector(connectorById(nodes, id), nodes, PROJ, {}), id).toBeDefined();
    }
  });

  it('그리기가 끊긴 선에 **잉크를 내리지 않고** 그대로 지나간다 (AC-52)', () => {
    const nodes = scene();
    const withBroken = makeStubCtx();
    expect(() => drawElements(withBroken, nodes, {}, {}, PROJ, {})).not.toThrow();

    // 끊긴 셋을 걷어 낸 장면과 **획 수가 같아야** 한다. "던지지 않는다" 만 재면 끊긴 선을
    // 어딘가에 그려 놓고도 초록이 된다.
    const intact = nodes.filter((n) => !(['c-gone', 'c-anchor', 'c-part'] as string[]).includes(n.id));
    const withoutBroken = makeStubCtx();
    drawElements(withoutBroken, intact, {}, {}, PROJ, {});
    expect(withBroken.ops).toEqual(withoutBroken.ops);
  });

  it('히트 판정이 끊긴 선을 잡지 않고 격자 전체에서 던지지 않는다 (AC-56)', () => {
    const nodes = scene();
    const hits: (string | undefined)[] = [];
    for (let x = 0; x <= PROJ.stage.width; x += 10) {
      for (let y = 0; y <= PROJ.stage.height; y += 10) {
        expect(() => hitTest(nodes, { x, y }, PROJ, {})).not.toThrow();
        hits.push(hitTest(nodes, { x, y }, PROJ, {})?.nodeId);
      }
    }
    for (const id of ['c-gone', 'c-anchor', 'c-part']) expect(hits).not.toContain(id);
  });

  it('목록이 끊긴 선을 **그린다** — 렌더가 던지지 않는다', () => {
    expect(() => renderList()).not.toThrow();
    for (const idx of BROKEN_ROWS) {
      expect(screen.getByTestId(`canvas-connector-row-${idx}`)).toBeTruthy();
    }
  });
});

// --- ① 끊긴 연결 — 목록이 말한다 (AC-78) -----------------------------------

describe('끊긴 연결이 목록에서 말해진다 (AC-78)', () => {
  it.each(LOCALE_NAMES)('%s — 끊긴 행에만 표시가 선다', (locale) => {
    renderList(locale);
    for (const idx of BROKEN_ROWS) {
      expect(screen.getByTestId(`canvas-connector-row-broken-${idx}`), String(idx)).toBeTruthy();
      expect(
        screen.getByTestId(`canvas-connector-row-${idx}`).getAttribute('data-broken'),
        String(idx),
      ).toBe('true');
    }
    for (const idx of INTACT_ROWS) {
      expect(screen.queryByTestId(`canvas-connector-row-broken-${idx}`), String(idx)).toBeNull();
      expect(
        screen.getByTestId(`canvas-connector-row-${idx}`).getAttribute('data-broken'),
        String(idx),
      ).toBeNull();
    }
  });

  it.each(LOCALE_NAMES)('%s — 표시가 **글자로도** 말한다 (칠만으로는 못 보는 사람에게 없다)', (locale) => {
    renderList(locale);
    const label = lookup(LOCALES[locale], 'dashboard.canvas.elements.connectorBroken');
    expect(label).toBeTruthy();
    expect(screen.getByTestId(`canvas-connector-row-broken-${ROW.gone}`).textContent).toBe(label);
  });

  it('펼치면 **고치는 길**까지 말한다 — 안 된다는 말만 남기지 않는다', () => {
    renderList();
    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.gone}`));
    const hint = screen.getByTestId(`canvas-connector-row-broken-hint-${ROW.gone}`);
    expect(hint.textContent?.length ?? 0).toBeGreaterThan(30);

    // 서는 선의 몸통에는 그 안내가 없다 — 있으면 멀쩡한 선이 고장난 것처럼 읽힌다.
    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.live}`));
    expect(
      screen.queryByTestId(`canvas-connector-row-broken-hint-${ROW.live}`),
    ).toBeNull();
  });

  it.each(LOCALE_NAMES)('%s — 행이 **두 끝과 그리는 법**을 말한다 (REQ-07)', (locale) => {
    renderList(locale);
    // 머리줄: 종류 이름 · id · 그리는 법.
    const head = screen.getByTestId(`canvas-connector-row-toggle-${ROW.anchor}`).textContent ?? '';
    const kindLabel = lookup(LOCALES[locale], 'dashboard.canvas.elements.connectorLabel');
    const routeLabel = lookup(LOCALES[locale], 'dashboard.canvas.elements.connectorRouteCurve');
    // 폴백을 두지 않는다 — 키가 빠진 로케일에서 `?? ''` 는 `toContain('')` 이 되어 아무것도
    // 재지 않으면서 초록이 된다. 없으면 **여기서** 빨개져야 한다.
    expect(kindLabel, locale).toBeTypeOf('string');
    expect(routeLabel, locale).toBeTypeOf('string');
    expect(head).toContain('c-anchor');
    expect(head).toContain(kindLabel);
    expect(head).toContain(routeLabel);

    // 몸통: 붙은 끝은 **요소와 자리 이름**을, 자유 끝은 **좌표**를 말한다.
    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.anchor}`));
    const from = screen.getByTestId(`canvas-connector-row-from-${ROW.anchor}`).textContent ?? '';
    expect(from).toContain('el-1');
    expect(from).toContain('no-such-anchor');

    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.free}`));
    const free = screen.getByTestId(`canvas-connector-row-from-${ROW.free}`).textContent ?? '';
    expect(free).toContain('10');
    expect(free).toContain('20');
  });

  it('꺾임점은 **수만** 말한다 — 좌표를 늘어놓지 않는다', () => {
    renderList();
    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.gone}`));
    const points = screen.getByTestId(`canvas-connector-row-points-${ROW.gone}`).textContent ?? '';
    expect(points).toContain('1');
    // 저술된 중간점은 (200, 120) 하나다. 그 수치가 새어 나오면 자유선 한 줄이 목록을 덮는다.
    expect(points).not.toContain('200');
    expect(points).not.toContain('120');
  });

  it('요소 행·그룹 행의 형상을 **가져가지 않는다** — 연결선에는 그 칸이 없다', () => {
    renderList();
    for (const idx of [...BROKEN_ROWS, ...INTACT_ROWS]) {
      fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${idx}`));
      expect(screen.queryByTestId(`canvas-element-${idx}`), String(idx)).toBeNull();
      expect(screen.queryByTestId(`canvas-element-tabpanel-${idx}`), String(idx)).toBeNull();
      expect(screen.queryByTestId(`canvas-group-row-${idx}`), String(idx)).toBeNull();
    }
  });

  it('머리줄의 일은 요소 행과 **같은 함수**를 지난다 — 순서 이동과 삭제', () => {
    const emit = renderList();
    fireEvent.click(screen.getByTestId(`canvas-connector-row-move-up-${ROW.live}`));
    expect(lastElements(emit).map((n) => n.id).slice(0, 5)).toEqual([
      'el-1',
      'el-2',
      'c-live',
      'grp-1',
      'c-gone',
    ]);

    emit.mockClear();
    fireEvent.click(screen.getByTestId(`canvas-connector-row-delete-${ROW.gone}`));
    expect(lastElements(emit).map((n) => n.id)).not.toContain('c-gone');
  });

  it('끝자리에서 바깥쪽 이동이 잠긴다 — 요소 행과 같은 규칙이다', () => {
    renderList();
    const last = screen.getByTestId(`canvas-connector-row-move-down-${ROW.grpLine}`);
    expect((last as HTMLButtonElement).disabled).toBe(true);
  });
});

// --- ① 끊겨도 버려지지 않는다 (AC-79) --------------------------------------

describe('끊겨도 config 에서 버려지지 않는다 (AC-79)', () => {
  it('파서가 끊긴 연결선을 **그대로 세운다**', () => {
    const ids = scene().map((n) => n.id);
    expect(ids).toEqual(RAW_NODES.map((n) => n.id));
  });

  it('저장 왕복에 값이 바뀌지 않는다 — 끊긴 선도 그대로 돌아온다', () => {
    const once = parseCanvasConfig(config());
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify(once)) as unknown);
    expect(twice).toEqual(once);
  });

  it('**다른 행을 고쳐도** 끊긴 선이 함께 쓸려 나가지 않는다', () => {
    // 목록을 한 번 편집하는 것만으로 저술이 사라지는 결함(004 가 그룹에서 겪은 것)의
    // 연결선 쪽 짝이다. 방출되는 것은 배열 통째이므로, 행을 그리지 않는 갈래가 하나라도
    // 걸러 낸 배열을 되돌려 쓰면 여기서 빨개진다.
    const emit = renderList();
    fireEvent.click(screen.getByTestId(`canvas-element-move-down-${ROW.el1}`));
    expect(lastElements(emit).map((n) => n.id).sort()).toEqual(
      RAW_NODES.map((n) => n.id as string).sort(),
    );
  });
});

// --- ② 지우기 연동 — 순수 층 (AC-80 · AC-81 · AC-82) -----------------------

describe('지우면 그것을 가리키던 연결선도 함께 간다 (AC-80 · AC-81)', () => {
  it('요소를 지우면 그 요소를 가리키던 선이 **전부** 빠진다 (AC-80)', () => {
    const nodes = scene();
    const next = removeNodesWithConnectors(nodes, new Set(['el-2']));
    // `el-2` 는 다섯 선의 **끝** 쪽이다 — 어느 끝이든 걸리면 간다.
    expect(next.map((n) => n.id)).toEqual(['el-1', 'grp-1', 'c-free']);
  });

  it('시작 쪽만 걸려도 간다 — 한쪽만 붙은 선을 남기지 않는다', () => {
    const nodes = scene();
    const next = removeNodesWithConnectors(nodes, new Set(['el-1']));
    expect(next.map((n) => n.id)).not.toContain('c-live');
    expect(next.map((n) => n.id)).not.toContain('c-anchor');
    // `el-1` 을 가리키지 않는 선들은 남는다.
    expect(next.map((n) => n.id)).toEqual(['el-2', 'grp-1', 'c-gone', 'c-part', 'c-free', 'c-grp']);
  });

  it('그룹을 지워도 같다 (AC-81)', () => {
    const nodes = scene();
    const next = removeNodesWithConnectors(nodes, new Set(['grp-1']));
    expect(next.map((n) => n.id)).not.toContain('c-grp');
    // 부품 복합 키를 가리키던 선은 **그룹 id 를 가리키지 않으므로** 그대로 남는다.
    // 그것이 이미 끊긴 선이라는 사실은 REQ-08 의 목록 표시가 말한다 — 지우기가 제 손으로
    // 복합 키를 쪼개면 구분자 판정이 둘이 된다(009 AC-04).
    expect(next.map((n) => n.id)).toContain('c-part');
  });

  it('연결선 자신을 지우는 길도 그대로다 — 이름으로 빠진다', () => {
    const nodes = scene();
    const next = removeNodesWithConnectors(nodes, new Set(['c-live']));
    expect(next.map((n) => n.id)).toEqual([
      'el-1',
      'el-2',
      'grp-1',
      'c-gone',
      'c-anchor',
      'c-part',
      'c-free',
      'c-grp',
    ]);
  });

  it('두 끝이 같은 요소를 가리켜도 한 번만 빠진다', () => {
    const nodes = parseCanvasConfig(
      config([
        RAW_NODES[0]!,
        { id: 'loop', kind: 'connector', from: { el: 'el-1', a: 'n' }, to: { el: 'el-1', a: 's' } },
      ]),
    ).elements;
    expect(connectorTargets(connectorById(nodes, 'loop'))).toEqual(['el-1', 'el-1']);
    expect(removeNodesWithConnectors(nodes, new Set(['el-1']))).toEqual([]);
  });

  it('자유 끝점뿐인 선은 무엇을 지워도 남는다 — 가리키는 것이 없다', () => {
    const nodes = scene();
    expect(connectorTargets(connectorById(nodes, 'c-free'))).toEqual([]);
    for (const id of ['el-1', 'el-2', 'grp-1']) {
      expect(removeNodesWithConnectors(nodes, new Set([id])).map((n) => n.id), id).toContain(
        'c-free',
      );
    }
  });
});

describe('지울 것이 없으면 **같은 참조**를 돌려준다 (AC-82)', () => {
  it('없는 id 를 지우면 받은 배열 그대로다', () => {
    const nodes = scene();
    expect(removeNodesWithConnectors(nodes, new Set(['nobody']))).toBe(nodes);
  });

  it('빈 집합도 그대로다 — 연동이 지름길을 만들지 않았다', () => {
    const nodes = scene();
    expect(removeNodesWithConnectors(nodes, new Set())).toBe(nodes);
  });

  it('연결선이 하나도 없는 장면에서 `removeNodes` 와 **같은 답**이다', () => {
    // 연동이 `filter` 를 한 벌 더 돌렸다면 여기서 참조가 갈린다 — 그 갈라짐은 "지울 것이
    // 없는데도 config 가 쓰인다" 는 헛된 프레임으로만 드러난다.
    const nodes = parseCanvasConfig(config([RAW_NODES[0]!, RAW_NODES[1]!])).elements;
    for (const ids of [new Set<string>(), new Set(['nobody']), new Set(['el-1'])]) {
      expect(removeNodesWithConnectors(nodes, ids)).toEqual(removeNodes(nodes, ids));
    }
    expect(removeNodesWithConnectors(nodes, new Set(['nobody']))).toBe(nodes);
  });

  it('아무 선도 가리키지 않는 요소를 지우면 남은 선들이 **같은 객체**다', () => {
    const nodes = parseCanvasConfig(
      config([
        RAW_NODES[0]!,
        RAW_NODES[1]!,
        { id: 'solo', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
        RAW_NODES[3]!,
      ]),
    ).elements;
    const before = nodeById(nodes, 'c-live');
    const next = removeNodesWithConnectors(nodes, new Set(['solo']));
    expect(next.map((n) => n.id)).toEqual(['el-1', 'el-2', 'c-live']);
    expect(nodeById(next, 'c-live')).toBe(before);
  });
});

// --- ② 지우기 연동 — **두 입구** 다 지난다 ---------------------------------

const CANVAS_DIR = __dirname;

/** 주석을 걷은 제품 소스 — 011 의 가드가 세운 그 규칙이다. */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function productSources(): { name: string; text: string }[] {
  const out: { name: string; text: string }[] = [];
  const walk = (dir: string): void => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(full);
        continue;
      }
      if (!/\.tsx?$/.test(entry.name) || /\.test\.tsx?$/.test(entry.name)) continue;
      out.push({
        name: path.relative(CANVAS_DIR, full),
        text: stripComments(fs.readFileSync(full, 'utf8')),
      });
    }
  };
  walk(CANVAS_DIR);
  return out;
}

describe('두 입구가 **같은 함수**를 지난다 (REQ-08 · 010 의 규율)', () => {
  it('`removeNodes` 를 직접 부르는 제품 파일이 **하나도 없다** — 전부 연동을 지난다', () => {
    // 010 은 "지우는 규칙은 한 곳" 을 세웠고 011 은 그 한 곳을 한 겹 감쌌다. 한쪽 입구만
    // 옛 함수를 계속 부르면 목록과 캔버스의 결과가 갈리며, 그 차이는 저장된 뒤에야
    // 드러난다. 정의가 사는 파일만 그 이름을 부른다.
    const callers = productSources()
      .filter(({ text }) => /\bremoveNodes\(/.test(text))
      .map(({ name }) => name);
    expect(callers).toEqual(['canvasEditArrange.ts']);
  });

  it('부르는 제품 파일이 **이름으로 적은 둘**이다 — 늘어난 자리를 세어서 적는다', () => {
    const callers = productSources()
      .filter(
        ({ name, text }) =>
          name !== 'canvasEditArrange.ts' && /\bremoveNodesWithConnectors\(/.test(text),
      )
      .map(({ name }) => name)
      .sort();
    expect(callers).toEqual(['CanvasEditOverlay.tsx', 'CanvasElementsEditor.tsx'].sort());
  });

  it('목록의 휴지통이 연결선을 함께 지운다', () => {
    const emit = renderList();
    fireEvent.click(screen.getByTestId(`canvas-element-delete-${ROW.el1}`));
    const ids = lastElements(emit).map((n) => n.id);
    expect(ids).not.toContain('el-1');
    expect(ids).not.toContain('c-live');
    expect(ids).not.toContain('c-anchor');
    expect(ids).toContain('c-free');
  });
});

/** 오버레이 하네스 — `CanvasEditOverlay.delete.test.tsx` 의 관용구 그대로다. */
function OverlayHarness({
  nodes,
  onElementsChange,
}: {
  nodes: readonly CanvasNode[];
  onElementsChange: (next: CanvasNode[]) => void;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <CanvasEditOverlay
        enabled
        elements={nodes}
        projection={PROJ}
        textWidths={{}}
        onElementsChange={onElementsChange}
      />
    </CanvasEditSelectionContext>
  );
}

describe('캔버스의 Delete 도 연결선을 함께 지운다 (AC-80 — 둘째 입구)', () => {
  it('고른 요소를 지우면 그것을 가리키던 선이 같은 방출에서 빠진다', () => {
    const emit = vi.fn();
    render(<I18nProvider><OverlayHarness nodes={scene()} onElementsChange={emit} /></I18nProvider>);
    const root = screen.getByTestId('canvas-edit-overlay');
    vi.spyOn(root, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      width: PROJ.stage.width,
      height: PROJ.stage.height,
      right: PROJ.stage.width,
      bottom: PROJ.stage.height,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);

    // `el-1` 의 중심: 캔버스 (100, 80) → 스테이지 축척 0.5 → px (50, 40).
    const init: MouseEventInit = { clientX: 50, clientY: 40, bubbles: true, cancelable: true };
    fireEvent(root, new MouseEvent('pointerdown', init));
    fireEvent(root, new MouseEvent('pointerup', init));
    expect(screen.getByTestId('selection').textContent).toBe('el-1');
    emit.mockClear();

    fireEvent.keyDown(root, { key: 'Delete' });

    // **한 번의 방출**이다 — 연동이 선을 따로 지우면 중간 배열이 화면에 선다.
    expect(emit).toHaveBeenCalledTimes(1);
    const ids = (emit.mock.calls[0]![0] as CanvasNode[]).map((n) => n.id);
    expect(ids).toEqual(['el-2', 'grp-1', 'c-gone', 'c-part', 'c-free', 'c-grp']);
  });
});

// --- ③ 회귀 — 뒤집지 않은 것 (AC-83 ~ AC-87) -------------------------------

describe('연결선을 쓰지 않은 config 가 이전과 같다 (AC-83)', () => {
  /** 011 이전의 다섯 종 + 그룹. `connectorParse.test.ts` 가 **파싱 쪽**을 이미 잰다. */
  const LEGACY: readonly Record<string, unknown>[] = [
    { id: 'r', kind: 'rect', geometry: { x: 10, y: 20, w: 30, h: 40 }, style: {} },
    { id: 'l', kind: 'line', geometry: { x1: 0, y1: 1, x2: 90, y2: 100 }, style: {} },
    // 색을 주는 것에 뜻이 있다 — 색 없는 글자는 그려지지 않으므로 재지도 않는다. 색 없이
    // 두면 아래 장부 단언이 **빈 장부**를 보고 아무것도 재지 않으면서 지나간다.
    {
      id: 't',
      kind: 'text',
      geometry: { x: 110, y: 120 },
      style: { fill: '#123456' },
      text: '{value}',
    },
    {
      id: 'g',
      kind: 'group',
      geometry: { x: 170, y: 180, w: 190, h: 200 },
      parts: [
        {
          id: 'body',
          kind: 'rect',
          geometry: { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT },
          style: {},
        },
      ],
    },
  ];

  it('프레임 키 집합이 한 글자도 달라지지 않는다 — 평평한 셋과 복합 하나', () => {
    const nodes = parseCanvasConfig(config(LEGACY)).elements;
    const items = [...walkDrawables(nodes)];
    expect(items.map((i) => i.key)).toEqual(['r', 'l', 't', 'g/body']);
    // 연결선 갈래가 **하나도** 나오지 않는다. 순회가 갈래를 달아 내므로 이 단언이
    // "요소만 나온다" 를 타입이 아니라 값으로 붙든다.
    expect(items.map((i) => i.kind)).toEqual(['element', 'element', 'element', 'element']);
  });

  it('그리기가 내는 글자 폭 장부의 **키 집합**이 프레임 키 그대로다 (G12)', () => {
    const nodes = parseCanvasConfig(config(LEGACY)).elements;
    const ctx = makeStubCtx();
    // 글자를 **실제로** 실어 준다 — 빈 장부로 부르면 재는 자리가 지나가지 않아 이 시험이
    // 아무것도 보지 않으면서 초록이 된다(그것이 첫 판에 걸린 자리다).
    const widths = drawElements(ctx, nodes, {}, { t: '42' }, PROJ, {});
    expect(ctx.ops.length).toBeGreaterThan(0);
    expect(Object.keys(widths)).toEqual(['t']);
  });

  it('임의 앵커를 쓰지 않은 요소에 `anchors` 키가 생기지 않는다', () => {
    const nodes = parseCanvasConfig(config(LEGACY)).elements;
    for (const node of nodes) expect(Object.keys(node), node.id).not.toContain('anchors');
  });
});

describe('009 의 선택 모델이 그대로다 (AC-84)', () => {
  it('최상위는 **평평한 키**, 부품만 복합 키다 — 연결선도 최상위라 평평하다', () => {
    const nodes = scene();
    const keys = [...walkDrawables(nodes)].map((i) => i.key);
    expect(keys).toEqual([
      'el-1',
      'el-2',
      'grp-1/body',
      'c-live',
      'c-gone',
      'c-anchor',
      'c-part',
      'c-free',
      'c-grp',
    ]);
  });

  it('키를 짓는 자리와 쪼개는 자리가 **서로의 역**이다', () => {
    expect(parseFrameKey(frameKey('el-1'))).toEqual({ nodeId: 'el-1', partId: undefined });
    expect(parseFrameKey(frameKey('grp-1', 'body'))).toEqual({
      nodeId: 'grp-1',
      partId: 'body',
    });
    expect(parseFrameKey(frameKey('c-live'))).toEqual({ nodeId: 'c-live', partId: undefined });
  });

  it('목록의 행이 **최상위 id** 로 골라진다 — 연결선 행도 같다', () => {
    renderList();
    fireEvent.click(screen.getByTestId(`canvas-connector-row-toggle-${ROW.live}`));
    expect(
      screen.getByTestId(`canvas-connector-row-${ROW.live}`).getAttribute('data-selected'),
    ).toBe('true');
    expect(
      screen.getByTestId(`canvas-connector-row-${ROW.live}`).getAttribute('data-element-id'),
    ).toBe('c-live');
  });
});

describe('008 의 경로 어휘가 그대로다 (AC-85)', () => {
  it('갈래가 **넷**(`M · L · C · Z`)이다 — 다섯째가 생기면 컴파일러가 이 자리를 가리킨다', () => {
    // 표를 `Record<PathCommand['c'], true>` 로 두는 것이 요점이다. 목록을 손으로 적으면
    // 유니온이 넓어져도 이 시험이 **초록으로** 지나간다.
    const SEEN: Readonly<Record<PathCommand['c'], true>> = { M: true, L: true, C: true, Z: true };
    expect(Object.keys(SEEN).sort()).toEqual(['C', 'L', 'M', 'Z']);
  });

  it('011 이 그 유니온에 한 줄도 더하지 않았다 — 소스에서 센다', () => {
    const text = stripComments(fs.readFileSync(path.join(CANVAS_DIR, 'shapes/pathTypes.ts'), 'utf8'));
    const union = /export type PathCommand =([\s\S]*?);\n/.exec(text)?.[1] ?? '';
    expect(union).not.toBe('');
    expect((union.match(/\bc: '/g) ?? []).length).toBe(4);
    // 2차 베지어를 들이지 않았다 — 011 의 곡선은 3차로 **환산**해 지나간다(AC-48).
    expect(union).not.toContain("'Q'");
  });
});

describe('그룹 중첩이 여전히 불가능하다 (AC-86)', () => {
  it('`parts` 가 `CanvasElement[]` 라 연결선도 그룹도 올 수 없다', () => {
    const connector: ConnectorElement = connectorById(scene(), 'c-live');
    const group: GroupElement = nodeById(scene(), 'grp-1') as GroupElement;

    // @ts-expect-error SPEC-CANVAS-011 — 연결선은 `CanvasElement` 가 아니다(A8 · A4).
    const withConnector: CanvasElement[] = [connector];
    // @ts-expect-error SPEC-CANVAS-011 — 그룹도 아니다(004 A6 이 세운 그대로다).
    const withGroup: CanvasElement[] = [group];

    // 런타임 값 자체는 멀쩡하다 — 막는 것은 **타입**이고, 위 두 줄이 그 사실이다.
    expect(withConnector).toHaveLength(1);
    expect(withGroup).toHaveLength(1);
    // 파서도 같은 답을 낸다: `parts` 에 섞여 들어온 연결선은 저절로 버려진다.
    const nested = parseCanvasConfig(
      config([
        {
          id: 'g2',
          kind: 'group',
          geometry: { x: 0, y: 0, w: 10, h: 10 },
          parts: [
            { id: 'ok', kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 }, style: {} },
            { id: 'nope', kind: 'connector', from: { x: 0, y: 0 }, to: { x: 1, y: 1 } },
          ],
        },
      ]),
    ).elements;
    expect((nested[0] as GroupElement).parts.map((p) => p.id)).toEqual(['ok']);
  });
});

describe('뒤집힌 시험에 근거가 남는다 (AC-87)', () => {
  /** 뒤집힌 단언이 사는 자리와, 그 자리에 **적혀 있어야 하는** 표시. */
  const FLIPPED: readonly { file: string; needle: string }[] = [
    // ① 팔레트 묶음 수 — 008 은 넷이었고 011 이 셋으로 접었다.
    { file: 'canvas008I18n.test.tsx', needle: '뒤집힌 수' },
    // ② 기본 접힘 — 008 은 `primitive` 를 접었고 011 이 `basic` 을 펼친다.
    { file: 'shapes/CanvasShapeCatalog.test.tsx', needle: '뒤집힌 단언' },
  ];

  it.each(FLIPPED)('$file 의 뒤집힌 단언이 SPEC-CANVAS-011 을 가리킨다', ({ file, needle }) => {
    // **주석을 걷지 않고** 읽는다 — 여기서 재는 것이 바로 그 주석이다.
    const text = fs.readFileSync(path.join(CANVAS_DIR, file), 'utf8');
    const lines = text.split('\n');
    const marked = lines.filter((line) => line.includes(needle));
    expect(marked.length, file).toBeGreaterThan(0);
    for (const line of marked) expect(line, file).toContain('SPEC-CANVAS-011');
  });

  it('셋째로 **넓힌** 가드도 제 근거를 들고 있다 — 009 의 좌표 변환 호출자', () => {
    // 이쪽은 뒤집은 것이 아니라 **넓힌** 것이라 허용 호출자를 이름으로 센다. 세 번째
    // 호출자가 생기면 다시 운다(011 이 가드에 대해 정한 규율).
    const text = fs.readFileSync(path.join(CANVAS_DIR, 'canvas009Guards.test.ts'), 'utf8');
    expect(text).toContain('AC-32');
    expect(text).toContain('009 AC-16 을 넓힌 것');
  });
});
