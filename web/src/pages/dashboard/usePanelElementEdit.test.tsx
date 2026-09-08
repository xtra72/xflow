// 패널 요소 편집 훅의 **정렬·초기화 저장 규칙** 검증.
//
// 이 훅이 밖으로 내놓는 약속은 "무엇을 저장하는가" 하나다. 정렬은 여러 요소를 한꺼번에
// 움직이므로 저장이 한 번에 나가야 하고(나눠 나가면 서로를 덮어쓴다), 맞출 상대가 없는
// 경우에는 아예 나가지 않아야 한다 — 빈 저장은 설정을 바꾸지 않으면서 변경으로만 남는다.
//
// @spec SPEC-CHART-005 §2.1 [U1] / §2.4 [U4]

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import {
  usePanelElementEdit,
  type ElementOffset,
  type OffsetPatch,
} from './usePanelElementEdit';

type Kind = 'value' | 'delta' | 'stats';
const ALL_KINDS: readonly Kind[] = ['value', 'delta', 'stats'];

/** 기준 상자 200×100, 화면 원점에 놓인 상황(panelEditAlign 테스트와 같은 기준). */
const BOUNDS_W = 200;
const BOUNDS_H = 100;

interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** 요소들이 가로로 어긋나 있는 배치 — 왼쪽 맞춤이 눈에 보이는 일을 한다. */
const SCATTERED: Record<Kind, Rect> = {
  value: { left: 10, top: 0, width: 50, height: 20 },
  delta: { left: 30, top: 30, width: 40, height: 20 },
  stats: { left: 60, top: 60, width: 30, height: 20 },
};

const ZERO_OFFSETS: Record<Kind, ElementOffset> = {
  value: { x: 0, y: 0 },
  delta: { x: 0, y: 0 },
  stats: { x: 0, y: 0 },
};

/** jsdom 은 레이아웃을 하지 않으므로 상자를 직접 심는다. */
function stubRect(el: Element, r: Rect): void {
  vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
    ...r,
    right: r.left + r.width,
    bottom: r.top + r.height,
    x: r.left,
    y: r.top,
    toJSON: () => ({}),
  } as DOMRect);
}

/** 기준 상자와 렌더된 요소들의 자리를 심는다. */
function layout(rendered: readonly Kind[]): void {
  stubRect(screen.getByTestId('bounds'), {
    left: 0,
    top: 0,
    width: BOUNDS_W,
    height: BOUNDS_H,
  });
  for (const kind of rendered) stubRect(screen.getByTestId(kind), SCATTERED[kind]);
}

/**
 * 훅을 쓰는 패널의 최소 모양 — 정렬 도구모음 버튼과 기준 상자, 요소들.
 *
 * `kinds`(패널이 가진 요소)와 `rendered`(지금 화면에 그려진 요소)를 나눠 받는다.
 * 보조 줄을 끄면 이 둘이 갈리며, 그때의 동작이 이 파일이 잠그는 것 중 하나다.
 */
function Fixture({
  kinds = ALL_KINDS,
  rendered = ALL_KINDS,
  enabled = true,
  offsets = ZERO_OFFSETS,
  writeOffsets,
  attachBounds = true,
  selectKinds = [],
}: {
  kinds?: readonly Kind[];
  rendered?: readonly Kind[];
  enabled?: boolean;
  offsets?: Record<Kind, ElementOffset>;
  writeOffsets: (patches: ReadonlyArray<OffsetPatch<Kind>>) => void;
  attachBounds?: boolean;
  selectKinds?: readonly Kind[];
}) {
  const { boundsRef, align, reset, selection, setSelection } = usePanelElementEdit<Kind>({
    kinds,
    enabled,
    offsets,
    writeOffsets,
  });

  return (
    <div>
      <button type="button" data-testid="align-left" onClick={() => align('horizontal', 'start')}>
        왼쪽 맞춤
      </button>
      <button type="button" data-testid="reset" onClick={reset}>
        배치 초기화
      </button>
      <button
        type="button"
        data-testid="select"
        onClick={() => setSelection(new Set(selectKinds))}
      >
        고르기
      </button>
      <span data-testid="selected-count">{selection.size}</span>
      <div ref={attachBounds ? boundsRef : null} data-testid="bounds" data-panel-bounds="">
        {rendered.map((kind) => (
          <div key={kind} data-panel-drag={kind} data-testid={kind} />
        ))}
      </div>
    </div>
  );
}

/**
 * 저장된 오프셋을 적용했을 때 요소의 왼쪽 변이 놓이는 자리(px).
 *
 * 오프셋은 기준 상자 대비 백분율이므로, "몇 %를 저장했나" 가 아니라 "요소가 어디에
 * 서는가" 로 봐야 정렬이 실제로 한 일을 재는 것이 된다.
 */
function leftAfter(kind: Kind, patch: OffsetPatch<Kind>): number {
  return SCATTERED[kind].left + ((patch.x ?? 0) / 100) * BOUNDS_W;
}

describe('정렬 — 무엇을 맞추는가', () => {
  it('아무것도 고르지 않고 누르면 화면에 있는 요소를 모두 한 줄에 세운다', () => {
    const write = vi.fn();
    render(<Fixture writeOffsets={write} />);
    layout(ALL_KINDS);

    fireEvent.click(screen.getByTestId('align-left'));

    // 저장은 한 번에 나가야 한다 — 나눠 나가면 뒤 계산이 옛 config 를 읽어 앞을 덮는다.
    expect(write).toHaveBeenCalledTimes(1);
    const patches = write.mock.calls[0]![0] as OffsetPatch<Kind>[];
    expect(patches.map((p) => p.kind).sort()).toEqual(['delta', 'stats', 'value']);
    // 세 요소의 왼쪽 변이 가장 왼쪽 요소(=10px)에 나란히 선다.
    for (const p of patches) expect(leftAfter(p.kind, p)).toBeCloseTo(SCATTERED.value.left, 10);
  });

  it('둘 이상 고르면 고른 것끼리만 맞춘다 — 고르지 않은 요소는 그대로 둔다', () => {
    const write = vi.fn();
    render(<Fixture writeOffsets={write} selectKinds={['value', 'stats']} />);
    layout(ALL_KINDS);

    fireEvent.click(screen.getByTestId('select'));
    fireEvent.click(screen.getByTestId('align-left'));

    const patches = write.mock.calls[0]![0] as OffsetPatch<Kind>[];
    expect(patches.map((p) => p.kind).sort()).toEqual(['stats', 'value']);
    for (const p of patches) expect(leftAfter(p.kind, p)).toBeCloseTo(SCATTERED.value.left, 10);
  });

  it('화면에 없는 요소는 맞추지 않는다 — 보조 줄을 끄면 그 오프셋을 건드리지 않는다', () => {
    const write = vi.fn();
    // stats 는 패널이 가진 종류지만 지금은 꺼져 있어 그려지지 않았다.
    render(<Fixture writeOffsets={write} rendered={['value', 'delta']} />);
    layout(['value', 'delta']);

    fireEvent.click(screen.getByTestId('align-left'));

    const patches = write.mock.calls[0]![0] as OffsetPatch<Kind>[];
    expect(patches.map((p) => p.kind).sort()).toEqual(['delta', 'value']);
  });

  it('맞출 상대가 없으면 저장하지 않는다 — 빈 변경을 남기지 않는다', () => {
    const write = vi.fn();
    render(<Fixture writeOffsets={write} rendered={['value']} />);
    layout(['value']);

    fireEvent.click(screen.getByTestId('align-left'));

    expect(write).not.toHaveBeenCalled();
  });

  it('기준 상자가 아직 붙기 전에는 저장하지 않는다', () => {
    const write = vi.fn();
    // 편집이 켜지기 전이거나 패널 본문이 아직 마운트되지 않은 순간.
    render(<Fixture writeOffsets={write} attachBounds={false} />);

    fireEvent.click(screen.getByTestId('align-left'));

    expect(write).not.toHaveBeenCalled();
  });
});

describe('배치 초기화', () => {
  it('화면에 없는 요소까지 포함해 모든 종류의 오프셋을 지운다', () => {
    const write = vi.fn();
    // 꺼 둔 요소도 config 에는 오프셋이 남아 있다 — 다시 켰을 때 옛 자리로 튀면 안 된다.
    render(<Fixture writeOffsets={write} rendered={['value']} />);

    fireEvent.click(screen.getByTestId('reset'));

    expect(write).toHaveBeenCalledWith([
      { kind: 'value', x: undefined, y: undefined },
      { kind: 'delta', x: undefined, y: undefined },
      { kind: 'stats', x: undefined, y: undefined },
    ]);
  });
});

describe('선택 상태', () => {
  it('편집을 끄면 골라 둔 것이 풀린다 — 다음에 열 때 무엇인가 골라져 있지 않다', () => {
    const write = vi.fn();
    const { rerender } = render(<Fixture writeOffsets={write} selectKinds={['value', 'delta']} />);

    fireEvent.click(screen.getByTestId('select'));
    expect(screen.getByTestId('selected-count')).toHaveTextContent('2');

    rerender(<Fixture writeOffsets={write} selectKinds={['value', 'delta']} enabled={false} />);

    expect(screen.getByTestId('selected-count')).toHaveTextContent('0');
  });
});
