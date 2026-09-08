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
  canvasAutoExpandedId,
  useCanvasEditSelection,
  useCanvasEditSelectionState,
  type CanvasSelection,
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
