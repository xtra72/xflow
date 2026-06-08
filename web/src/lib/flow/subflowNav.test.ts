// 서브플로우 네비게이션 백 스택 헬퍼 테스트.
// 들어가기(push) / 돌아가기(pop) 와 location.state 읽기의 순수 로직을 검증한다.

import { describe, expect, it } from 'vitest';

import {
  popBackStack,
  pushBackStack,
  readBackStack,
  SUBFLOW_BACK_STATE_KEY,
} from './subflowNav';

describe('readBackStack — location.state 에서 백 스택 읽기', () => {
  it('state 가 null/undefined 면 빈 배열을 반환한다', () => {
    expect(readBackStack(null)).toEqual([]);
    expect(readBackStack(undefined)).toEqual([]);
  });

  it('subflowBack 이 없으면 빈 배열을 반환한다', () => {
    expect(readBackStack({})).toEqual([]);
    expect(readBackStack({ other: 1 })).toEqual([]);
  });

  it('subflowBack 배열을 그대로 읽고, 비문자열/빈 문자열은 걸러낸다', () => {
    const state = { [SUBFLOW_BACK_STATE_KEY]: ['A', '', 'B', 42, null, 'C'] };
    expect(readBackStack(state)).toEqual(['A', 'B', 'C']);
  });
});

describe('pushBackStack — 들어가기', () => {
  it('현재 플로우 id 를 스택 끝에 추가한다', () => {
    expect(pushBackStack([], 'A')).toEqual(['A']);
    expect(pushBackStack(['A'], 'B')).toEqual(['A', 'B']);
    expect(pushBackStack(['A', 'B'], 'C')).toEqual(['A', 'B', 'C']);
  });

  it('빈 id 는 추가하지 않고 기존 스택 복사본을 반환한다', () => {
    const stack = ['A'];
    const result = pushBackStack(stack, '');
    expect(result).toEqual(['A']);
    expect(result).not.toBe(stack);
  });

  it('원본 스택을 변경하지 않는다(불변)', () => {
    const stack = ['A'];
    pushBackStack(stack, 'B');
    expect(stack).toEqual(['A']);
  });
});

describe('popBackStack — 돌아가기', () => {
  it('직전 플로우 id 와 나머지 스택을 반환한다', () => {
    expect(popBackStack(['A', 'B', 'C'])).toEqual({
      prev: 'C',
      rest: ['A', 'B'],
    });
    expect(popBackStack(['A'])).toEqual({ prev: 'A', rest: [] });
  });

  it('빈 스택이면 prev 는 null 이다', () => {
    expect(popBackStack([])).toEqual({ prev: null, rest: [] });
  });

  it('원본 스택을 변경하지 않는다(불변)', () => {
    const stack = ['A', 'B'];
    popBackStack(stack);
    expect(stack).toEqual(['A', 'B']);
  });
});

describe('중첩 시나리오 A→B→C 통합', () => {
  it('들어가기로 쌓고 돌아가기로 역순 복원한다', () => {
    // A 에서 B 로 들어가기
    const atB = pushBackStack([], 'A'); // ['A']
    // B 에서 C 로 들어가기
    const atC = pushBackStack(atB, 'B'); // ['A', 'B']
    expect(atC).toEqual(['A', 'B']);

    // C 에서 돌아가기 → B (스택 ['A'])
    const backToB = popBackStack(atC);
    expect(backToB.prev).toBe('B');
    expect(backToB.rest).toEqual(['A']);

    // B 에서 돌아가기 → A (스택 [])
    const backToA = popBackStack(backToB.rest);
    expect(backToA.prev).toBe('A');
    expect(backToA.rest).toEqual([]);

    // A 에서는 스택이 비어 돌아가기 숨김
    expect(popBackStack(backToA.rest).prev).toBeNull();
  });
});
