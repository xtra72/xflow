// 007 의 회귀 게이트 — I23 대조 · 형상 회귀 · DOM 경계 · 유휴 정지
// (SPEC-CANVAS-007 M11 · AC-E1 · AC-E3 · AC-E10 · AC-E11 · AC-E12 · 불변식 K3~K15).
//
// **이 파일은 007 이 무엇을 바꾸지 않았는지를 잰다.** 그 목록이 곧 이 SPEC 의 크기이므로,
// 하나라도 조용히 늘어나면 SPEC 이 스스로 적은 경계를 넘은 것이다.
//
// **008 이 세운 게이트는 여기서 다시 쓰지 않는다.** `DrawContext2D` 멤버 수(008 AC-E2) ·
// 유휴 정지(008 AC-E3) · 팔레트 회귀(008 AC-E10) · I23 대조(008 AC-E9)는 제 파일에 그대로
// 있고 007 은 **한 글자도 고치지 않았다**. 여기 있는 것은 007 이 새로 만든 표면에 대한
// 같은 종류의 질문이다.
//
// **`.tsx` 는 M5 의 DOM 경계 가드를 지나가지 않는다.** 그 가드의 `productionSources()` 는
// `.ts` 만 모으므로(실측 `svgDocument.test.ts`), M9 가 더한 `CanvasSvgImport.tsx` 는 그
// 그물에 걸리지 않았다 — **관측 공백이다.** 여기서 `.tsx` 까지 넓힌 같은 가드를 세운다.
// 기존 가드는 고치지 않는다(남의 시험을 넓히는 대신 제 시험을 세운다).
//
// @spec SPEC-CANVAS-007 REQ-04 · REQ-07 · REQ-08 · AC-E10 · AC-E11 · AC-E12

import fs from 'node:fs';
import path from 'node:path';

import { useState } from 'react';

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import type { VisibilitySource } from '../charts/visiblePolling';
import CanvasSurface, { type FrameScheduler } from './CanvasSurface';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import {
  DEFAULT_CANVAS_SIZE,
  parseCanvasConfig,
  type CanvasElement,
  type CanvasPrimitiveKind,
  type PathElement,
} from './canvasConfig';
import { appendImportedElements } from './canvasElementFactory';
import type { CanvasProjection } from './canvasGeometry';
import { derivedCanvasSize } from './canvasWorkspace';
import { stageLattice } from './canvasGeometry';
import { MAX_PATH_COMMANDS, PATH_LOCAL_EXTENT } from './shapes/pathTypes';
import { planSvgImport } from './svgimport/svgImportPlan';
import { MAX_IMPORT_COMMANDS, MAX_IMPORT_ELEMENTS } from './svgimport/svgImportTypes';

const CANVAS_DIR = __dirname;
const IMPORT_DIR = path.join(CANVAS_DIR, 'svgimport');

/** 주석을 걷은 제품 소스. 산문에 적힌 금지어가 가드를 헛되이 울리지 않게 한다. */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

/**
 * `svgimport/` 의 제품 파일 전량 — **`.tsx` 를 포함한다**(M5 의 가드가 놓친 자리).
 */
function importSources(): { name: string; text: string }[] {
  return fs
    .readdirSync(IMPORT_DIR)
    .filter((name) => /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name))
    .map((name) => ({ name, text: stripComments(fs.readFileSync(path.join(IMPORT_DIR, name), 'utf8')) }));
}

// --- DOM 경계 (AC-E10 · K3 · K4 · K7) — `.tsx` 까지 -------------------------

describe('AC-E10 — 파싱한 문서가 살아 있는 DOM 에 붙지 않는다 (`.tsx` 포함)', () => {
  it('제품 파일 목록에 `.tsx` 가 실제로 들어 있다 — 그물이 성긴지를 먼저 잰다', () => {
    const names = importSources().map((s) => s.name);
    // 켜져 있음을 먼저 단언한다. `.tsx` 가 없으면 아래 순회가 M5 의 가드와 같은 것을 두 번
    // 도는 것에 지나지 않는다.
    expect(names).toContain('CanvasSvgImport.tsx');
    expect(names.filter((n) => n.endsWith('.tsx')).length).toBeGreaterThanOrEqual(1);
    expect(names.length).toBeGreaterThanOrEqual(10);
  });

  it('붙이는 자리가 하나도 없다 (K3)', () => {
    for (const { name, text } of importSources()) {
      for (const forbidden of [
        'appendChild',
        'insertBefore',
        'replaceWith',
        'importNode',
        'adoptNode',
        'document.body',
        'document.createElement',
      ]) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });

  it('SVG 기하 DOM API 를 하나도 쓰지 않는다 (K4)', () => {
    for (const { name, text } of importSources()) {
      for (const forbidden of [
        'getBBox',
        'getTotalLength',
        'getPointAtLength',
        'pathSegList',
        'getCTM',
        'getComputedStyle',
        // 브라우저에게 크기를 묻지 않는다(REQ-02) — `svgAsset.ts` 가 기록한 그 함정이
        // 007 에는 존재할 자리가 없다.
        'naturalWidth',
        'naturalHeight',
      ]) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });

  it('`DOMParser`·`Document`·`Element` 를 참조하는 파일이 **정확히 하나**다 (K7)', () => {
    const referencing = importSources()
      .filter(({ text }) => /\bDOMParser\b|\bDocument\b|\bElement\b/.test(text))
      .map(({ name }) => name);
    expect(referencing).toEqual(['svgDocument.ts']);
  });

  it('사용자 파일이 서버로 가지 않는다 — 자산 API 도 `fetch` 도 부르지 않는다', () => {
    for (const { name, text } of importSources()) {
      for (const forbidden of ['fetch(', 'dashboardAssetService', 'XMLHttpRequest', 'axios']) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });

  it('`DrawContext2D` 에 007 이 바라는 멤버가 없다 — 004 의 둘째 근거를 되살리지 않는다 (K1)', () => {
    const draw = stripComments(fs.readFileSync(path.join(CANVAS_DIR, 'drawElement.ts'), 'utf8'));
    const iface = draw.slice(draw.indexOf('interface DrawContext2D'));
    const body = iface.slice(0, iface.indexOf('\n}'));
    for (const forbidden of ['arc', 'drawImage', 'Promise', 'createPattern', 'createLinearGradient']) {
      expect(`${forbidden}:${body.includes(forbidden)}`).toBe(`${forbidden}:false`);
    }
  });
});

// --- 형상 회귀 (AC-E12 · K6 · K12 · K13 · K14) ------------------------------

describe('AC-E12 — 007 이 넓히지 않은 것들', () => {
  it('`shapes/pathTypes.ts` 의 상수와 어휘가 그대로다 (K6)', () => {
    expect(MAX_PATH_COMMANDS).toBe(256);
    expect(PATH_LOCAL_EXTENT).toBe(10000);
    const source = fs.readFileSync(path.join(CANVAS_DIR, 'shapes', 'pathTypes.ts'), 'utf8');
    // `PathCommand` 의 갈래가 넷이다 — 다섯 번째가 생기면 여기서 드러난다.
    const kinds = [...source.matchAll(/c:\s*'([MLCZ])'/g)].map((m) => m[1]);
    expect(new Set(kinds)).toEqual(new Set(['M', 'L', 'C', 'Z']));
    expect(source).not.toContain("c: 'A'");
    expect(source).not.toContain("c: 'Q'");
  });

  it('`PanelSettingsDialog.tsx` 가 **한 줄도 자라지 않았다** (K13 · 006 I7)', () => {
    const dialog = fs.readFileSync(
      path.join(CANVAS_DIR, '..', '..', 'PanelSettingsDialog.tsx'),
      'utf8',
    );
    // `wc -l` 과 같은 셈이다 — 마지막 줄바꿈 뒤의 빈 조각을 세지 않는다. `split('\n').length`
    // 로 세면 8,372 가 나와 SPEC 이 적은 실측값과 하나 어긋난다.
    expect(dialog.split('\n').filter((_l, i, all) => i < all.length - 1 || _l !== '').length).toBe(
      8371,
    );
  });

  it('팔레트는 **넷**이다 — 다섯 번째 원시형 단추를 만들지 않았다 (K14)', () => {
    // 타입 수준의 사실을 먼저 못박는다: 원시형 넷을 전부 적은 표는 다섯 번째가 생기면
    // 컴파일이 서지 않는다.
    const table: Record<CanvasPrimitiveKind, true> = {
      rect: true,
      ellipse: true,
      line: true,
      text: true,
    };
    expect(Object.keys(table)).toHaveLength(4);

    const dock = stripComments(fs.readFileSync(path.join(CANVAS_DIR, 'CanvasEditDock.tsx'), 'utf8'));
    const palette = /const PALETTE_KINDS: readonly CanvasPrimitiveKind\[\] = \[([^\]]*)\]/.exec(dock);
    expect(palette).not.toBeNull();
    expect((palette?.[1] ?? '').split(',').filter((s) => s.trim() !== '')).toHaveLength(4);
  });

  it('`PATH_LOCAL_EXTENT` 로 **나누는** 자리가 `canvasGeometry.ts` 밖에 없다 (K8)', () => {
    const offenders: string[] = [];
    const walk = (dir: string): void => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(full);
        } else if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) {
          const text = stripComments(fs.readFileSync(full, 'utf8'));
          if (/\/\s*PATH_LOCAL_EXTENT/.test(text)) offenders.push(path.relative(CANVAS_DIR, full));
        }
      }
    };
    walk(CANVAS_DIR);
    // 007 은 **곱하기만** 한다.
    expect(offenders).toEqual(['canvasGeometry.ts']);
  });

  it('가져온 요소의 필드 이름 집합이 config 스키마를 넓히지 않는다 (K12)', () => {
    const doc =
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="-13 7 317 181">' +
      '<rect x="5" y="12" width="40" height="20" fill="#c0392b"/></svg>';
    const result = planSvgImport(doc, { ...DEFAULT_CANVAS_SIZE });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    const { created } = appendImportedElements([], result.shapes, result.box);
    expect(created).toHaveLength(1);
    // `catalog_id` 를 심지 않는다 — 그 필드는 "어느 카탈로그 도형에서 나왔는가" 를 뜻하고,
    // 가져온 요소에는 카탈로그가 없다. 예약값을 넣으면 한 필드가 두 뜻을 갖는다.
    expect(Object.keys(created[0]!).sort()).toEqual(['geometry', 'id', 'kind', 'path', 'style']);

    // 파서 왕복이 필드를 잃지도 더하지도 않는다.
    const reopened = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: { ...DEFAULT_CANVAS_SIZE }, elements: created })) as unknown,
    );
    expect(reopened.elements).toHaveLength(1);
    expect(reopened.elements[0]).toEqual(created[0]);
  });
});

// --- 006 · 008 이 배달한 성질이 그대로다 -------------------------------------

describe('앞선 SPEC 이 배달한 성질이 007 뒤에도 산다', () => {
  it('006 — `derivedCanvasSize` 가 바뀌지 않으면 **받은 객체를 그대로** 돌려준다', () => {
    // 참조 동일성이 쓰기를 삼키는 자리다(`CanvasPanel.tsx` 주석). 값만 같은 새 객체를
    // 돌려주면 편집 중 매 측정마다 config 쓰기가 돈다.
    const stored = { width: 500, height: 400 };
    const same = derivedCanvasSize(stored, { width: 500, height: 400 });
    expect(same).toBe(stored);
  });

  it('006 — `stageLattice` 의 한 칸이 정수 px 다', () => {
    const lattice = stageLattice({ width: 317, height: 181 }, { width: 500, height: 400 }, 25);
    // 켜져 있음: 격자가 실제로 켜졌다(칸이 1px 미만이면 정수화하지 않는다 — 그때는 이
    // 단언이 무엇도 재지 않으므로 먼저 1 이상임을 못박는다).
    expect(lattice.cell.x).toBeGreaterThanOrEqual(1);
    expect(Number.isInteger(lattice.cell.x)).toBe(true);
    // 칸은 **정사각**이다 — 두 축이 같은 축척 하나를 쓴다.
    expect(lattice.cell.y).toBe(lattice.cell.x);
  });

  it('008 — 원시형 넷과 요소 여섯이 **다른 타입**이다(팔레트가 요소 종류를 따라 자라지 않는다)', () => {
    const config = stripComments(fs.readFileSync(path.join(CANVAS_DIR, 'canvasConfig.ts'), 'utf8'));
    const primitive = /export type CanvasPrimitiveKind =([^;]*);/.exec(config);
    const element = /export type CanvasElementKind =([^;]*);/.exec(config);
    expect(primitive).not.toBeNull();
    expect(element).not.toBeNull();
    const parts = (s: string): string[] => s.split('|').map((p) => p.trim()).filter((p) => p !== '');
    // 원시형은 리터럴 **넷**이고 `path` 를 담지 않는다 — 팔레트가 그 표를 쓴다.
    expect(parts(primitive?.[1] ?? '')).toEqual(["'rect'", "'ellipse'", "'line'", "'text'"]);
    // 요소 종류는 그 넷을 **참조하고 `path` 를 더한** 별개의 타입이다. 008 이 만든 갈라짐이
    // 여기 있다: 요소가 늘어도 팔레트는 자라지 않는다.
    expect(parts(element?.[1] ?? '')).toEqual(['CanvasPrimitiveKind', "'path'"]);
  });
});

// --- 유휴 정지 (AC-E3) ------------------------------------------------------

type RoCallback = (entries: Array<{ contentRect: { width: number; height: number } }>) => void;

class ImmediateResizeObserver {
  cb: RoCallback;
  constructor(cb: RoCallback) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { width: 96, height: 64 } }]);
  }
  unobserve() {}
  disconnect() {}
}

function makeScheduler() {
  let nextHandle = 1;
  let requested = 0;
  const pending = new Map<number, (nowMs: number) => void>();
  const scheduler: FrameScheduler = {
    request(cb) {
      requested += 1;
      const handle = nextHandle++;
      pending.set(handle, cb);
      return handle;
    },
    cancel(handle) {
      pending.delete(handle);
    },
  };
  return {
    scheduler,
    get requested() {
      return requested;
    },
    get pending() {
      return pending.size;
    },
    flush(nowMs: number) {
      const due = [...pending.values()];
      pending.clear();
      act(() => {
        for (const cb of due) cb(nowMs);
      });
    },
  };
}

const ALWAYS_VISIBLE: VisibilitySource = { isVisible: () => true, subscribe: () => () => {} };

function countingCtx() {
  const calls: string[] = [];
  return new Proxy({} as Record<string, unknown>, {
    get(_t, prop: string) {
      if (prop === '__calls') return calls;
      return () => {
        calls.push(prop);
        return prop === 'measureText' ? { width: 0 } : undefined;
      };
    },
    set() {
      return true;
    },
  }) as unknown as CanvasRenderingContext2D & { __calls: string[] };
}

/** 상한을 **정확히** 채운 가져오기 — 요소 64 · 명령 640. */
function maxImportElements(): PathElement[] {
  const pts = Array.from({ length: 9 }, (_v, i) => `${i % 30} ${(i * 7) % 40}`).join(' ');
  const doc =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="-13 7 317 181">' +
    `<polygon points="${pts}" fill="#c0392b"/>`.repeat(MAX_IMPORT_ELEMENTS) +
    '</svg>';
  const result = planSvgImport(doc, { ...DEFAULT_CANVAS_SIZE });
  if (!result.ok) throw new Error('고정 입력이 거절되었다 — 시험이 잴 것이 없다');
  return appendImportedElements([], result.shapes, result.box).created;
}

describe('AC-E3 — 가져온 요소 64개가 있어도 유휴 정지가 그대로다 (001 REQ-05)', () => {
  let ctx: ReturnType<typeof countingCtx>;

  beforeEach(() => {
    ctx = countingCtx();
    vi.stubGlobal('ResizeObserver', ImmediateResizeObserver as unknown as typeof ResizeObserver);
    vi.stubGlobal('devicePixelRatio', 2);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('상한을 꽉 채운 가져오기 · 트윈 없음 — 마운트에 한 장, 그 뒤로 예약이 없다', () => {
    const elements = maxImportElements();
    // 켜져 있음을 먼저 못박는다 — 상한을 실제로 채웠는가.
    expect(elements).toHaveLength(MAX_IMPORT_ELEMENTS);
    expect(elements.reduce((n, el) => n + el.path.length, 0)).toBe(MAX_IMPORT_COMMANDS);

    const clock = makeScheduler();
    render(
      <CanvasSurface
        canvas={{ ...DEFAULT_CANVAS_SIZE }}
        elements={elements}
        targetStyles={{}}
        texts={{}}
        scheduler={clock.scheduler}
        visibilitySource={ALWAYS_VISIBLE}
      />,
    );
    expect(clock.requested).toBe(1);
    clock.flush(0);
    // 그렸음을 **먼저** 단언한다 — 아무것도 그리지 않았다면 "예약이 없다" 는 공허하다.
    expect(ctx.__calls.filter((c) => c === 'moveTo').length).toBeGreaterThanOrEqual(
      MAX_IMPORT_ELEMENTS,
    );
    expect(clock.requested).toBe(1);
    expect(clock.pending).toBe(0);
  });
});

// --- I23 대조 (AC-E11 · 006 불변식 I23) --------------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

function pathEl(id: string): PathElement {
  return {
    id,
    kind: 'path',
    geometry: { x: 40, y: 40, w: 160, h: 90 },
    path: [
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: PATH_LOCAL_EXTENT, y: 0 },
      { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
      { c: 'Z' },
    ],
    style: { fill: '#c0392b' },
  };
}

function EditHarness({ docked }: { docked: boolean }) {
  const [elements, setElements] = useState<readonly CanvasElement[]>([pathEl('p1')]);
  const state = useSelectionState();
  const overlay = (
    <CanvasEditOverlay
      enabled
      elements={elements}
      projection={PROJ}
      textWidths={{}}
      onElementsChange={setElements}
    />
  );
  return (
    <I18nProvider>
      <SelectionProvider value={state}>
        {docked ? <CanvasEditDockRegion enabled>{overlay}</CanvasEditDockRegion> : overlay}
      </SelectionProvider>
    </I18nProvider>
  );
}

describe('AC-E11 — 그려지는 자리에 손잡이가 닿는다 (I23 · 기계적 대조)', () => {
  /**
   * **007 이 캔버스 표면에 그리는 층은 0 개다.** 그래서 이 표는 비어 있고, 그 "비어 있음"
   * 자체를 아래 시험이 **기계로** 확인한다 — 유추가 아니라 노드를 모아 견준다(008 M7 의 규율).
   */
  const DOCK_ONLY: readonly string[] = [
    'canvas-svg-import-group',
    'canvas-scratchpad-dropzone',
    'canvas-palette-group-basic',
  ];

  it('① 007 의 노드 가운데 오버레이 루트의 자손인 것이 **하나도 없다**', () => {
    render(<EditHarness docked />);
    const overlay = screen.getByTestId('canvas-edit-overlay');
    // 도크는 포털이라 DOM 상 오버레이 **밖**에 있다 — 그 사실이 이 단언의 내용이다.
    expect(overlay.querySelectorAll('[data-testid^="canvas-svg-import"]')).toHaveLength(0);
    // 켜져 있음: 그 노드들은 화면 어딘가에 실제로 있다.
    expect(screen.queryByTestId('canvas-svg-import-group')).not.toBeNull();
  });

  it('② `dockHost === null` 인 표면에서 007 이 더한 노드가 **하나도** 없다', () => {
    render(<EditHarness docked={false} />);
    expect(document.querySelectorAll('[data-testid^="canvas-svg-import"]')).toHaveLength(0);
    for (const id of DOCK_ONLY) expect(screen.queryByTestId(id), id).toBeNull();
  });

  it('③ 그 "없음" 이 옳은 근거 — 놓인 경로를 다스리는 컨트롤은 **두 표면 모두**에 있다', () => {
    for (const docked of [true, false]) {
      render(<EditHarness docked={docked} />);
      vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
        left: 0, top: 0, width: PROJ.stage.width, height: PROJ.stage.height,
        right: PROJ.stage.width, bottom: PROJ.stage.height, x: 0, y: 0, toJSON: () => ({}),
      } as DOMRect);
      // 상자 px (20,20)-(100,65) 안, 삼각형 내부(로컬 lx > ly).
      fireEvent(
        screen.getByTestId('canvas-edit-overlay'),
        new MouseEvent('pointerdown', { clientX: 90, clientY: 40, bubbles: true, cancelable: true }),
      );
      // 층(선택 외곽선)과 손잡이가 **같은 조건**으로 선다 — 한쪽만 있으면 I23 이 깨진 것이다.
      const layer = screen.queryByTestId('canvas-selection-p1');
      const control = screen.queryByTestId('canvas-handle-se');
      expect(layer !== null, `docked=${docked}: 관계`).toBe(control !== null);
      expect(layer, `docked=${docked}: layer`).not.toBeNull();
      cleanup();
    }
  });

  it('④ 캔버스 표면에도 오버레이 루트에도 **파일 드롭 처리자가 없다** (§결정 6 · K10)', () => {
    render(<EditHarness docked />);
    const overlay = screen.getByTestId('canvas-edit-overlay');
    // React 는 처리자를 속성으로 남기지 않으므로 **소스 형상**으로 잰다 — 그것이 이 성질의
    // 유일한 관측 가능한 자리다.
    for (const file of ['CanvasEditOverlay.tsx', 'CanvasSurface.tsx']) {
      const text = stripComments(fs.readFileSync(path.join(CANVAS_DIR, file), 'utf8'));
      for (const forbidden of ['onDrop', 'onDragOver', 'onDragEnter', 'dataTransfer']) {
        expect(`${file}:${forbidden}:${text.includes(forbidden)}`).toBe(`${file}:${forbidden}:false`);
      }
    }
    // 그리고 007 의 화면 파일도 마찬가지다 — 드롭을 도크 안으로 옮겨 세우지도 않았다.
    for (const { name, text } of importSources()) {
      for (const forbidden of ['onDrop', 'onDragOver', 'dataTransfer']) {
        expect(`${name}:${forbidden}:${text.includes(forbidden)}`).toBe(`${name}:${forbidden}:false`);
      }
    }
    expect(overlay).toBeTruthy();
  });

  it('AC-E1 — 파일을 고르기 전에는 007 이 더한 노드가 묶음 머리 하나뿐이다', async () => {
    render(<EditHarness docked />);
    const nodes = [...document.querySelectorAll('[data-testid^="canvas-svg-import"]')].map((n) =>
      n.getAttribute('data-testid'),
    );
    expect(nodes).toEqual(['canvas-svg-import-group']);
    // 그리고 그 하나는 **접혀** 있다.
    expect(screen.getByTestId('canvas-svg-import-group').getAttribute('aria-expanded')).toBe('false');
    await waitFor(() => expect(screen.queryByTestId('canvas-svg-import-file')).toBeNull());
  });
});

// --- 선택 컨텍스트 ----------------------------------------------------------
// 시험 하네스가 쓰는 얇은 껍데기. 본체는 `canvasEditContext` 에 있다.

import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';

function useSelectionState(): ReturnType<typeof useCanvasEditSelectionState> {
  return useCanvasEditSelectionState();
}

function SelectionProvider({
  value,
  children,
}: {
  value: ReturnType<typeof useCanvasEditSelectionState>;
  children: React.ReactNode;
}): React.ReactElement {
  return <CanvasEditSelectionContext value={value}>{children}</CanvasEditSelectionContext>;
}
