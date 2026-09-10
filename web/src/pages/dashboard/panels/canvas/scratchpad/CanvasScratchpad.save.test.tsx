// 끌어 저장 · 단추 저장 (SPEC-CANVAS-008 M9 · REQ-03 · AC-05 · AC-06 · AC-E7 · AC-E9).
//
// **이 파일은 전제부터 잰다.** 놓임 판정이 오버레이의 `pointerup` 에 사는 근거는 "오버레이가
// 누름에서 루트에 포인터를 잡는다" 하나이고, 그 전제가 거짓이면 이 설계 전체가 틀린 자리에
// 서 있는 것이다. 그래서 배선을 재기 전에 **잡기 자체**를 잰다(아래 첫 describe).
//
// 축척은 스테이지 200 × 100 · 캔버스 500 × 400 이라 **가로 0.4 · 세로 0.25** 로 갈린다.
// 두 축이 같으면 축을 뒤바꾼 결함이 드러나지 않는다.
//
// 드롭 존은 클라이언트 x 500..600 에 세운다 — **스테이지 밖**이다. 스테이지 안에 두면
// "클라이언트 좌표로 판정한다" 와 "스테이지 좌표로 판정한다" 가 같은 답을 내어 J7 이
// 재어지지 않는다.
//
// @spec SPEC-CANVAS-008 REQ-03 · REQ-07 · AC-05 · AC-06 · AC-E7 · AC-E9

import fs from 'node:fs';
import path from 'node:path';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { CanvasEditDockRegion } from '../CanvasEditDock';
import CanvasEditOverlay from '../CanvasEditOverlay';
import type { CanvasElement, CanvasSize } from '../canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import type { StageSize } from '../canvasGeometry';
import { useScratchpadStore } from './scratchpadStore';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

/**
 * 요소 **셋**이다. 둘만 두면 "고른 것만 저장한다" 가 "전부 저장한다" 와 구별되지 않는다.
 * 좌표는 서로 다르고 원점이 아니다 — (0, 0) 짜리 묶음은 좌상단 계산의 결함을 감춘다.
 */
const SEED: readonly CanvasElement[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 100, y: 80, w: 200, h: 160 }, style: { fill: '#a' } },
  {
    id: 'el-2',
    kind: 'ellipse',
    geometry: { x: 300, y: 240, w: 100, h: 80 },
    style: { fill: '#b' },
  },
  { id: 'el-3', kind: 'rect', geometry: { x: 20, y: 30, w: 50, h: 40 }, style: { fill: '#c' } },
];

// --- 하네스 ---------------------------------------------------------------

function Harness({
  docked = true,
  onChange,
}: {
  docked?: boolean;
  onChange?: (next: CanvasElement[]) => void;
}) {
  const [elements, setElements] = useState<readonly CanvasElement[]>(SEED);
  const state = useCanvasEditSelectionState();
  const emit = (next: CanvasElement[]): void => {
    onChange?.(next);
    setElements(next);
  };
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <CanvasEditDockRegion enabled={docked}>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={{ stage: STAGE, canvas: CANVAS }}
          textWidths={{}}
          onElementsChange={emit}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

function stubRect(node: Element, left: number, top: number, width: number, height: number): void {
  vi.spyOn(node, 'getBoundingClientRect').mockReturnValue({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect);
}

function send(type: string, x: number, y: number, init: MouseEventInit = {}): void {
  fireEvent(
    screen.getByTestId('canvas-edit-overlay'),
    new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init }),
  );
}

async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

/** 오버레이와 드롭 존의 클라이언트 상자를 세운다. 드롭 존은 **스테이지 밖**이다. */
function stubSurfaces(): void {
  stubRect(screen.getByTestId('canvas-edit-overlay'), 0, 0, 200, 100);
  const zone = screen.queryByTestId('canvas-scratchpad-dropzone');
  if (zone !== null) stubRect(zone, 500, 0, 100, 80);
}

/** el-1 과 el-2 를 고르고, el-1 을 잡아 무리 이동을 시작한다. */
function grabTwo(): void {
  send('pointerdown', 60, 30); // el-1 (px 40..120 × 20..60)
  send('pointerdown', 130, 65, { shiftKey: true }); // el-2 (px 120..160 × 60..80)
  send('pointerdown', 60, 30); // 이미 고른 것을 다시 잡으면 무리가 유지된다
}

beforeEach(() => {
  localStorage.clear();
  useScratchpadStore.setState({ entries: [], notice: null });
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

// --- 전제 ---------------------------------------------------------------

describe('전제: 오버레이가 포인터를 잡는다 (놓임 판정이 여기 사는 근거)', () => {
  it('누름은 **오버레이 루트**에 포인터를 잡는다 — 도크가 아니다', () => {
    render(<Harness />);
    const root = screen.getByTestId('canvas-edit-overlay') as HTMLElement & {
      setPointerCapture?: unknown;
    };
    const dock = screen.getByTestId('canvas-dock-panel') as HTMLElement & {
      setPointerCapture?: unknown;
    };
    // jsdom 에는 이 API 가 없다. 오버레이가 `?.` 로 부르므로 여기서 심어 관측한다.
    const rootCapture = vi.fn();
    const dockCapture = vi.fn();
    root.setPointerCapture = rootCapture;
    dock.setPointerCapture = dockCapture;
    stubSurfaces();

    send('pointerdown', 60, 30);

    expect(rootCapture).toHaveBeenCalledTimes(1);
    expect(dockCapture).not.toHaveBeenCalled();
  });

  it('잡힌 뒤의 뗌은 오버레이로 온다 — 도크 안의 노드는 그 사건을 받지 못한다', () => {
    render(<Harness />);
    stubSurfaces();
    // 드롭 존이 제 손으로 놓임을 받으려 했다면 이런 처리자를 달았을 것이다.
    const zone = screen.getByTestId('canvas-scratchpad-dropzone');
    const own = vi.fn();
    zone.addEventListener('pointerup', own);
    zone.addEventListener('pointerenter', own);
    zone.addEventListener('drop', own);

    // 잡혔을 때 브라우저가 하는 일: 손이 도크 위에 있어도 사건의 target 은 오버레이다.
    send('pointerdown', 60, 30);
    send('pointerup', 540, 40);

    expect(own).not.toHaveBeenCalled();
    // 그런데 저장은 되었다 — 판정이 오버레이 쪽에 있기 때문이다.
    expect(useScratchpadStore.getState().entries).toHaveLength(1);
  });
});

// --- AC-05 끌어 저장 -----------------------------------------------------

describe('끌어서 스크래치패드에 저장한다 (AC-05 · REQ-03)', () => {
  it('드롭 존 안에서 손을 떼면 고른 둘의 사본이 항목 하나로 들어간다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);

    const entries = useScratchpadStore.getState().entries;
    expect(entries).toHaveLength(1);
    // 고른 둘만이다 — el-3 은 들어가지 않는다.
    expect(entries[0]?.elements.map((el) => el.id)).toEqual(['el-1', 'el-2']);
    // 묶음의 좌상단이 함께 저장된다(REQ-03). el-1 의 (100, 80) 이다.
    expect(entries[0]?.origin).toEqual({ x: 100, y: 80 });
  });

  it('**복사이지 이동이 아니다** — 캔버스의 셋이 그대로 셋이고 기하가 시작값과 같다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    // 실제로 멀리 끈다. 옮겨진 뒤 되돌리는지를 재려면 도중에 한 번 흘려야 한다.
    send('pointermove', 540, 40);
    await nextFrame();
    expect(liveElements()[0]?.geometry, '끄는 도중에는 옮겨져 있어야 한다').not.toEqual(
      SEED[0]?.geometry,
    );

    send('pointerup', 540, 40);

    expect(liveElements()).toHaveLength(3);
    expect(liveElements().map((el) => el.geometry)).toEqual(SEED.map((el) => el.geometry));
  });

  it('되돌림은 서랍에 든 것과 같다 — 두 값이 갈라지지 않는다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);

    const saved = useScratchpadStore.getState().entries[0]?.elements ?? [];
    const live = liveElements().filter((el) => el.id !== 'el-3');
    expect(saved.map((el) => el.geometry)).toEqual(live.map((el) => el.geometry));
  });

  it('드롭 존 **밖**에서 떼면 항목은 생기지 않고 이동이 확정된다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    send('pointermove', 100, 50);
    await nextFrame();
    send('pointerup', 100, 50);

    expect(useScratchpadStore.getState().entries).toHaveLength(0);
    // px (60,30) → (100,50) = 캔버스 (100, 80) 만큼. 확정된 값이 남는다.
    expect(liveElements()[0]?.geometry).toEqual({ x: 200, y: 160, w: 200, h: 160 });
  });

  it('취소(`pointercancel`)는 드롭 존 위에서도 항목을 만들지 않는다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointercancel', 540, 40);

    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });

  it('손잡이 드래그는 서랍으로 가지 않는다 — 크기 조절은 저장이 아니다', async () => {
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30); // el-1 하나만 고르면 손잡이가 뜬다
    const handle = screen.getByTestId('canvas-handle-se');
    fireEvent(
      handle,
      new MouseEvent('pointerdown', { clientX: 120, clientY: 60, bubbles: true, cancelable: true }),
    );
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);

    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });
});

// --- 드롭 존 강조 ---------------------------------------------------------

describe('강조는 드롭 존 자신이 입는다 (REQ-07 · 불변식 I23)', () => {
  it('손이 드롭 존 위로 들어오고 나가는 것을 그 노드가 말한다', async () => {
    render(<Harness />);
    stubSurfaces();
    const zone = screen.getByTestId('canvas-scratchpad-dropzone');
    expect(zone.dataset.active).toBe('false');

    grabTwo();
    send('pointermove', 540, 40);
    await nextFrame();
    expect(zone.dataset.active, '서랍 위인데 꺼져 있다').toBe('true');

    send('pointermove', 100, 50);
    await nextFrame();
    expect(zone.dataset.active, '서랍 밖인데 켜져 있다').toBe('false');
  });

  it('손잡이로 크기를 조절하는 중에는 켜지지 않는다', async () => {
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    fireEvent(
      screen.getByTestId('canvas-handle-se'),
      new MouseEvent('pointerdown', { clientX: 120, clientY: 60, bubbles: true, cancelable: true }),
    );
    send('pointermove', 540, 40);
    await nextFrame();

    expect(screen.getByTestId('canvas-scratchpad-dropzone').dataset.active).toBe('false');
  });

  it('놓고 난 뒤에는 꺼진다', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);
    await nextFrame();

    expect(screen.getByTestId('canvas-scratchpad-dropzone').dataset.active).toBe('false');
  });
});

// --- AC-06 끌지 않는 길 ---------------------------------------------------

describe('끌지 않고도 같은 일이 된다 (AC-06 · WCAG 2.2 SC 2.5.7)', () => {
  it('단추가 만든 항목의 몸체가 **끌어 넣은 것과 같다** (불변식 J11)', async () => {
    render(<Harness />);
    stubSurfaces();
    grabTwo();
    await nextFrame();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);
    const dragged = useScratchpadStore.getState().entries[0];

    // 서랍을 비우고 같은 둘을 고른 뒤 단추로 넣는다.
    cleanup();
    useScratchpadStore.setState({ entries: [], notice: null });
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    send('pointerdown', 130, 65, { shiftKey: true });
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));
    const clicked = useScratchpadStore.getState().entries[0];

    expect(clicked?.elements).toEqual(dragged?.elements);
    expect(clicked?.origin).toEqual(dragged?.origin);
  });

  it('고른 것이 없으면 단추를 쓸 수 없다', () => {
    render(<Harness />);
    stubSurfaces();
    expect(screen.getByTestId<HTMLButtonElement>('canvas-scratchpad-save').disabled).toBe(true);

    send('pointerdown', 60, 30);
    expect(screen.getByTestId<HTMLButtonElement>('canvas-scratchpad-save').disabled).toBe(false);
  });

  it('단추로 저장해도 **요소 배열은 한 번도 쓰이지 않는다** (AC-E8)', () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    onChange.mockClear();

    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    expect(useScratchpadStore.getState().entries).toHaveLength(1);
    // 서랍은 config 를 넓히지 않는다 — 저장은 요소 배열을 건드릴 일이 없다.
    expect(onChange).not.toHaveBeenCalled();
  });
});

// --- 목록 편집 -----------------------------------------------------------

describe('목록의 이름과 지우기', () => {
  it('이름이 없으면 자동 이름을 **보인다** — 저장하지는 않는다', () => {
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    const id = useScratchpadStore.getState().entries[0]?.id ?? '';
    const input = screen.getByTestId<HTMLInputElement>(`canvas-scratchpad-name-${id}`);
    expect(input.value).toBe('');
    expect(input.placeholder).not.toBe('');
    expect(useScratchpadStore.getState().entries[0]?.name).toBe('');
  });

  it('이름을 적으면 그 항목에 남는다', () => {
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));
    const id = useScratchpadStore.getState().entries[0]?.id ?? '';

    fireEvent.change(screen.getByTestId(`canvas-scratchpad-name-${id}`), {
      target: { value: '펌프' },
    });

    expect(useScratchpadStore.getState().entries[0]?.name).toBe('펌프');
  });

  it('지우기는 **두 걸음**이다 — 한 번 눌러서는 사라지지 않는다', () => {
    // 이 패널에는 되돌리기가 없다(위험 R8). 한 번의 잘못 누름이 저술을 지우면 회복할
    // 길이 없으므로 확인 줄을 지난다.
    render(<Harness />);
    stubSurfaces();
    send('pointerdown', 60, 30);
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));
    const id = useScratchpadStore.getState().entries[0]?.id ?? '';

    fireEvent.click(screen.getByTestId(`canvas-scratchpad-remove-${id}`));
    expect(useScratchpadStore.getState().entries, '한 번에 지워졌다').toHaveLength(1);

    fireEvent.click(screen.getByTestId(`canvas-scratchpad-remove-no-${id}`));
    expect(useScratchpadStore.getState().entries).toHaveLength(1);

    fireEvent.click(screen.getByTestId(`canvas-scratchpad-remove-${id}`));
    fireEvent.click(screen.getByTestId(`canvas-scratchpad-remove-yes-${id}`));
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });

  it('항목이 하나도 없으면 안내 문구 하나만 보인다 (REQ-05)', () => {
    render(<Harness />);
    expect(screen.getByTestId('canvas-scratchpad-empty')).toBeTruthy();
    expect(screen.queryByTestId('canvas-scratchpad-list')).toBeNull();
  });
});

// --- AC-E9 두 표면 --------------------------------------------------------

describe('그려지는 자리에 손잡이가 닿는다 (AC-E9 · 불변식 I23 · 시험 규율 D10)', () => {
  it('009 가 더한 노드는 **전부** 도크 안에 있고 캔버스 표면에는 하나도 없다', () => {
    // 한 표면만 재면 006 을 통과시킨 그 형상이 그대로 돌아온다. `docked` 를 갈아 끼운다.
    for (const docked of [true, false] as const) {
      cleanup();
      useScratchpadStore.setState({ entries: [], notice: null });
      render(<Harness docked={docked} />);
      if (docked) {
        stubSurfaces();
        // 목록과 안내까지 세워 두고 잰다 — 비어 있으면 "없다" 가 두 표면에서 똑같이 참이라
        // 이 시험이 아무것도 재지 못한다.
        send('pointerdown', 60, 30);
        fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));
      }

      const added = [
        ...document.querySelectorAll<HTMLElement>('[data-testid^="canvas-scratchpad-"]'),
      ];
      const overlayRoot = screen.getByTestId('canvas-edit-overlay');
      const dock = screen.queryByTestId('canvas-dock-panel');

      for (const node of added) {
        // ① 덮임 — 하나도 빠짐없이 도크 안이다.
        expect(dock?.contains(node), `${node.dataset.testid} 가 도크 밖이다`).toBe(true);
        // ② 캔버스 표면 위에는 아무것도 더하지 않는다 — 다스릴 수 없는 층을 만들지 않는다.
        expect(overlayRoot.contains(node), `${node.dataset.testid} 가 표면 위에 있다`).toBe(false);
      }

      // ③ 관계 — 도크의 유무와 스크래치패드의 유무가 **같은 한 값**이다.
      expect(added.length > 0, `@docked=${docked} 의 스크래치패드 유무`).toBe(dock !== null);
    }
  });

  it('도크가 없는 표면에서는 끌어도 항목이 생기지 않는다', async () => {
    render(<Harness docked={false} />);
    stubRect(screen.getByTestId('canvas-edit-overlay'), 0, 0, 200, 100);
    grabTwo();
    send('pointermove', 540, 40);
    await nextFrame();
    send('pointerup', 540, 40);

    expect(useScratchpadStore.getState().entries).toHaveLength(0);
    // ③ 의 근거: 그래도 놓인 요소를 다스리는 컨트롤은 이 표면에도 있다.
    expect(screen.getByTestId('canvas-edit-overlay')).toBeTruthy();
    expect(screen.getByTestId('canvas-selection-el-1')).toBeTruthy();
  });
});

// --- AC-E7 형상 가드 ------------------------------------------------------

describe('놓임 판정에 스테이지 산술이 없다 (AC-E7 · 불변식 J7)', () => {
  const read = (name: string): string =>
    fs.readFileSync(path.resolve(__dirname, name), 'utf8');

  /**
   * 선언부터 그 함수를 닫는 줄까지. 닫는 줄을 인자로 받는 것은 두 대상의 들여쓰기가
   * 다르기 때문이다 — 하나는 모듈 최상위 `function`(열 0 의 `}`), 하나는 컴포넌트 안의
   * 화살표 상수(두 칸 들여쓴 `};`)다.
   */
  const bodyOf = (source: string, decl: string, close: string): string => {
    const start = source.indexOf(decl);
    expect(start, `${decl} 를 찾지 못했다`).toBeGreaterThanOrEqual(0);
    const end = source.indexOf(close, start + decl.length);
    expect(end, `${decl} 의 끝을 찾지 못했다`).toBeGreaterThan(start);
    return source.slice(start, end);
  };

  it('`pointInRect` 의 본문에 투영도 스테이지도 나눗셈도 없다', () => {
    const body = bodyOf(read('canvasScratchpadDrop.ts'), 'export function pointInRect(', '\n}');
    // 고정 입력이 스스로 무엇을 재는지 먼저 단언한다 — 본문을 못 잡았으면 아래가 공허하다.
    expect(body).toContain('rect.left');
    for (const banned of ['projection', 'proj.', 'stage', 'stagePoint', 'unproject', '/']) {
      expect(body, `놓임 판정이 \`${banned}\` 를 본다`).not.toContain(banned);
    }
  });

  it('되돌림·저장 갈래에도 좌표 공간을 넘는 산술이 없다', () => {
    const body = bodyOf(
      read('../CanvasEditOverlay.tsx'),
      '  const dropToScratchpad = (drag: MoveDrag): void => {',
      '\n  };',
    );
    expect(body).toContain('patchNodeGeometry');
    for (const banned of ['stagePoint', 'unprojectPoint', 'projection', 'stage.width']) {
      expect(body, `되돌림 갈래가 \`${banned}\` 를 본다`).not.toContain(banned);
    }
  });
});
