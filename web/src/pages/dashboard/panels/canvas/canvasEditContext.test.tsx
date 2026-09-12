// 캔버스 편집 선택 컨텍스트 테스트 (SPEC-CANVAS-002 T4).
//
// 덮는 축 셋.
//   1) **provider 없이 동작한다** — 대시보드에 놓인 패널 곁에는 목록 편집기가 없다.
//      그때 기본값이 곧 로컬 선택이며, 소비자가 죽거나 조용히 무동작이 되지 않는다.
//   2) **provider 가 있으면 공유한다** — 미리보기와 목록 편집기가 같은 선택을 본다.
//   3) **자동 펼침 id 는 선택에서 파생된다** — 단일 선택일 때만 값이 있고, 둘 이상이면
//      `null` 이다(AC-06 "다중 선택 시 아무 행도 자동으로 펼치지 않는다").
//
// DOM 이 필요 없는 순수 판정(`canvasAutoExpandedId`)은 따로 잠근다.

import { describe, expect, it } from 'vitest';
import { act, render, renderHook, screen } from '@testing-library/react';

import { EMPTY_SELECTION } from '../charts/panelEditSelection';
import {
  CanvasEditSelectionContext,
  CanvasLiveSeriesContext,
  canvasAutoExpandedId,
  sameSeriesOptions,
  useCanvasEditSelection,
  useCanvasEditSelectionState,
  useCanvasLiveSeries,
  useCanvasLiveSeriesPublisher,
  useCanvasLiveSeriesState,
  type CanvasSelection,
  type CanvasSeriesOption,
} from './canvasEditContext';

// --- 순수 판정 -----------------------------------------------------------

describe('canvasAutoExpandedId — 자동 펼침은 선택에서 파생된다', () => {
  it('빈 선택이면 null 이다', () => {
    expect(canvasAutoExpandedId(EMPTY_SELECTION)).toBeNull();
  });

  it('하나만 골랐으면 그 id 다', () => {
    expect(canvasAutoExpandedId(new Set(['a']))).toBe('a');
  });

  it('둘 이상 골랐으면 null 이다 (펼침이 쌓이지 않게 한다)', () => {
    expect(canvasAutoExpandedId(new Set(['a', 'b']))).toBeNull();
  });
});

// --- provider 없는 기본값 ------------------------------------------------

describe('useCanvasEditSelection — provider 없이도 동작한다', () => {
  it('처음에는 아무것도 골라져 있지 않다', () => {
    const { result } = renderHook(() => useCanvasEditSelection());
    expect(result.current.selection.size).toBe(0);
    expect(result.current.autoExpandedId).toBeNull();
  });

  it('선택을 바꾸면 로컬 상태가 따라온다 (조용한 무동작이 아니다)', () => {
    const { result } = renderHook(() => useCanvasEditSelection());
    act(() => result.current.setSelection(new Set(['a'])));
    expect([...result.current.selection]).toEqual(['a']);
    expect(result.current.autoExpandedId).toBe('a');
  });

  it('둘을 고르면 자동 펼침 id 가 사라진다', () => {
    const { result } = renderHook(() => useCanvasEditSelection());
    act(() => result.current.setSelection(new Set(['a', 'b'])));
    expect(result.current.selection.size).toBe(2);
    expect(result.current.autoExpandedId).toBeNull();
  });

  it('setSelection 은 렌더가 바뀌어도 같은 참조다 (효과 의존성에 들어간다)', () => {
    const { result } = renderHook(() => useCanvasEditSelection());
    const first = result.current.setSelection;
    act(() => result.current.setSelection(new Set(['a'])));
    expect(result.current.setSelection).toBe(first);
  });

  it('EMPTY_SELECTION 으로 되돌리면 빈 선택이 된다', () => {
    const { result } = renderHook(() => useCanvasEditSelection());
    act(() => result.current.setSelection(new Set(['a'])));
    act(() => result.current.setSelection(EMPTY_SELECTION));
    expect(result.current.selection.size).toBe(0);
  });

  it('provider 가 없으면 소비자끼리 상태를 나누지 않는다 (각자 로컬이다)', () => {
    render(
      <>
        <Probe testId="one" />
        <Probe testId="two" />
      </>,
    );
    act(() => screen.getByTestId('one-set').click());
    expect(screen.getByTestId('one').textContent).toBe('a');
    expect(screen.getByTestId('two').textContent).toBe('');
  });
});

// --- provider 가 있을 때 --------------------------------------------------

describe('useCanvasEditSelection — provider 가 있으면 공유한다', () => {
  it('한 소비자가 고른 것을 다른 소비자가 본다 (미리보기 ↔ 목록 편집기)', () => {
    render(
      <Shared>
        <Probe testId="one" />
        <Probe testId="two" />
      </Shared>,
    );
    act(() => screen.getByTestId('one-set').click());
    expect(screen.getByTestId('one').textContent).toBe('a');
    expect(screen.getByTestId('two').textContent).toBe('a');
  });

  it('공유 상태에서도 자동 펼침 id 는 단일 선택일 때만 있다', () => {
    render(
      <Shared>
        <Probe testId="one" />
      </Shared>,
    );
    act(() => screen.getByTestId('one-set').click());
    expect(screen.getByTestId('one-expanded').textContent).toBe('a');
    act(() => screen.getByTestId('one-set-two').click());
    expect(screen.getByTestId('one-expanded').textContent).toBe('none');
  });
});

// --- 시험용 컴포넌트 ------------------------------------------------------

/** 선택을 읽고 쓰는 최소 소비자. */
function Probe({ testId }: { testId: string }) {
  const { selection, setSelection, autoExpandedId } = useCanvasEditSelection();
  return (
    <div>
      <span data-testid={testId}>{[...selection].join(',')}</span>
      <span data-testid={`${testId}-expanded`}>{autoExpandedId ?? 'none'}</span>
      <button
        type="button"
        data-testid={`${testId}-set`}
        onClick={() => setSelection(new Set<string>(['a']))}
      />
      <button
        type="button"
        data-testid={`${testId}-set-two`}
        onClick={() => setSelection(new Set<string>(['a', 'b']))}
      />
    </div>
  );
}

/** T11 이 다이얼로그에서 할 감싸기를 그대로 흉내 낸다(React 19 — 컨텍스트가 곧 provider). */
function Shared({ children }: { children: React.ReactNode }) {
  const state = useCanvasEditSelectionState();
  // 타입 축을 한 번 밟아 둔다 — 선택 키는 언제나 문자열 노드 id 다(REQ-06).
  const selection: CanvasSelection = state.selection;
  void selection;
  return (
    <CanvasEditSelectionContext value={state}>{children}</CanvasEditSelectionContext>
  );
}

// --- 라이브 시리즈 채널 ---------------------------------------------------
//
// 이 채널이 지는 약속은 셋이다.
//   1) **값 비교로 발행을 접는다** — 판독값 맵은 폴링마다 새로 만들어지므로 참조로 재면
//      매 주기 상태가 갈리고, 그 갈림이 렌더를 불러 다시 발행하는 고리가 된다.
//   2) **provider 없이도 죽지 않는다** — 발행은 무동작, 소비는 빈 목록이다. 빈 목록이
//      곧 "폴백하라" 는 신호이며, 그 폴백은 편집기가 소유한다.
//   3) **선택 채널과 서로를 깨우지 않는다** — 둘을 한 값으로 합치지 않은 이유 그 자체다.

const OPT_A: CanvasSeriesOption[] = [
  { id: 'a', label: 'A' },
  { id: 'b', label: 'B' },
];

describe('sameSeriesOptions — 값으로 잰다', () => {
  it('같은 참조는 같다', () => {
    expect(sameSeriesOptions(OPT_A, OPT_A)).toBe(true);
  });

  it('참조가 달라도 값이 같으면 같다 (폴링마다 새로 만들어진 목록)', () => {
    expect(sameSeriesOptions(OPT_A, [...OPT_A.map((o) => ({ ...o }))])).toBe(true);
  });

  it('길이가 다르면 다르다', () => {
    expect(sameSeriesOptions(OPT_A, [OPT_A[0]!])).toBe(false);
  });

  it('키가 하나라도 다르면 다르다', () => {
    expect(sameSeriesOptions(OPT_A, [OPT_A[0]!, { id: 'z', label: 'B' }])).toBe(false);
  });

  it('표시 이름만 달라도 다르다 (드롭다운 글자가 바뀐다)', () => {
    expect(sameSeriesOptions(OPT_A, [OPT_A[0]!, { id: 'b', label: 'B2' }])).toBe(false);
  });

  it('순서가 다르면 다르다 (드롭다운 순서가 곧 사용자가 보는 순서다)', () => {
    expect(sameSeriesOptions(OPT_A, [OPT_A[1]!, OPT_A[0]!])).toBe(false);
  });

  it('둘 다 비면 같다', () => {
    expect(sameSeriesOptions([], [])).toBe(true);
  });
});

describe('useCanvasLiveSeries — provider 없이도 동작한다', () => {
  it('아무도 내놓지 않았으면 빈 목록이다 (편집기가 config 로 폴백할 신호)', () => {
    const { result } = renderHook(() => useCanvasLiveSeries());
    expect(result.current).toEqual([]);
  });

  it('빈 목록의 참조는 렌더가 바뀌어도 그대로다 (memo 를 매번 무효화하지 않는다)', () => {
    const { result, rerender } = renderHook(() => useCanvasLiveSeries());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });

  it('발행은 무동작이며 예외를 던지지 않는다 (대시보드에 놓인 패널)', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesPublisher());
    expect(() => act(() => result.current(OPT_A))).not.toThrow();
  });

  it('발행 통로의 참조는 렌더가 바뀌어도 그대로다 (효과 의존성에 들어간다)', () => {
    const { result, rerender } = renderHook(() => useCanvasLiveSeriesPublisher());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});

describe('useCanvasLiveSeriesState — 발행 고리를 끊는다', () => {
  it('처음에는 비어 있다', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesState());
    expect(result.current.options).toEqual([]);
  });

  it('새 값을 내놓으면 그것이 보인다', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesState());
    act(() => result.current.publish(OPT_A));
    expect(result.current.options).toEqual(OPT_A);
  });

  it('값이 같은 목록을 다시 내놓으면 **참조까지 그대로**다 (렌더가 다시 돌지 않는다)', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesState());
    act(() => result.current.publish(OPT_A));
    const first = result.current.options;
    act(() => result.current.publish(OPT_A.map((o) => ({ ...o }))));
    expect(result.current.options).toBe(first);
  });

  it('값이 달라지면 새 목록으로 갈린다', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesState());
    act(() => result.current.publish(OPT_A));
    act(() => result.current.publish([{ id: 'c', label: 'C' }]));
    expect(result.current.options.map((o) => o.id)).toEqual(['c']);
  });

  it('publish 는 렌더가 바뀌어도 같은 참조다', () => {
    const { result } = renderHook(() => useCanvasLiveSeriesState());
    const first = result.current.publish;
    act(() => result.current.publish(OPT_A));
    expect(result.current.publish).toBe(first);
  });
});

describe('useCanvasLiveSeries — provider 가 있으면 공유한다', () => {
  it('한 쪽이 내놓은 것을 다른 쪽이 본다 (미리보기 → 목록 편집기)', () => {
    render(
      <SharedSeries>
        <SeriesProbe testId="pub" />
        <SeriesProbe testId="sub" />
      </SharedSeries>,
    );
    act(() => screen.getByTestId('pub-publish').click());
    expect(screen.getByTestId('sub').textContent).toBe('a,b');
  });
});

/** 라이브 시리즈를 읽고 쓰는 최소 소비자. */
function SeriesProbe({ testId }: { testId: string }) {
  const options = useCanvasLiveSeries();
  const publish = useCanvasLiveSeriesPublisher();
  return (
    <div>
      <span data-testid={testId}>{options.map((o) => o.id).join(',')}</span>
      <button type="button" data-testid={`${testId}-publish`} onClick={() => publish(OPT_A)} />
    </div>
  );
}

/** 다이얼로그가 하는 감싸기를 그대로 흉내 낸다. */
function SharedSeries({ children }: { children: React.ReactNode }) {
  const state = useCanvasLiveSeriesState();
  return <CanvasLiveSeriesContext value={state}>{children}</CanvasLiveSeriesContext>;
}
