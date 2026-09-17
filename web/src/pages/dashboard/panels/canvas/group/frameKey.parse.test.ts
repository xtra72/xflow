// 복합 키의 분해와 판별 (SPEC-CANVAS-009 M2).
//
// 이 파일이 지키는 것은 **왕복**이다 — `frameKey` 가 만든 것을 `parseFrameKey` 가 되돌리고,
// 되돌린 것을 다시 만들면 같은 문자열이다. 그 성질이 깨지면 선택 상태에 든 키가 어느
// 부품도 가리키지 못하고, 그 증상은 예외가 아니라 "고른 것이 사라진다" 로만 보인다.
//
// @spec SPEC-CANVAS-009 REQ-01 · AC-01 ~ AC-04

import { describe, expect, it } from 'vitest';

import { frameKey, isPartKey, parseFrameKey } from './frameKey';

describe('parseFrameKey — 왕복이 성립한다 (AC-01 · AC-02)', () => {
  it('부품 키를 만들었다가 풀면 같은 두 조각이다', () => {
    for (const [nodeId, partId] of [
      ['grp-1', 'body'],
      ['el-12', 'stem'],
      ['g', 'p'],
    ] as const) {
      expect(parseFrameKey(frameKey(nodeId, partId))).toEqual({ nodeId, partId });
    }
  });

  it('최상위 키를 풀면 `partId` 자리가 빈다', () => {
    for (const nodeId of ['el-3', 'rect-1', 'grp-1', '']) {
      const parsed = parseFrameKey(frameKey(nodeId));
      expect(parsed).toEqual({ nodeId });
      expect(parsed.partId).toBeUndefined();
    }
  });

  it('푼 것을 다시 만들면 원래 문자열이다 (반대 방향 왕복)', () => {
    for (const key of ['el-3', 'grp-1/body', 'grp-1/a/b', 'grp-1/']) {
      const { nodeId, partId } = parseFrameKey(key);
      expect(frameKey(nodeId, partId)).toBe(key);
    }
  });

  it('빈 문자열 부품 id 도 **부재가 아니다** — 복합 키로 되돌아온다', () => {
    // `frameKey` 가 `partId === undefined` 로 가르므로 분해도 같은 자로 갈라야 한다.
    // 참 판정으로 적으면(`partId ? … : …`) `'grp-1/'` 이 최상위 키로 되읽혀
    // `grp-1` 그룹 자신과 구별되지 않는다.
    expect(parseFrameKey('grp-1/')).toEqual({ nodeId: 'grp-1', partId: '' });
  });
});

describe('parseFrameKey — 첫 구분자에서 한 번만 쪼갠다 (AC-03)', () => {
  it('부품 id 에 구분자가 있어도 `nodeId` 는 온전하다', () => {
    expect(parseFrameKey('grp-1/a/b')).toEqual({ nodeId: 'grp-1', partId: 'a/b' });
  });

  it('구분자가 여럿이어도 `nodeId` 는 첫 조각 하나다', () => {
    expect(parseFrameKey('g/a/b/c/d').nodeId).toBe('g');
    expect(parseFrameKey('g/a/b/c/d').partId).toBe('a/b/c/d');
  });

  it('뒤에서 쪼갰다면 나왔을 값과 **다르다** (뮤테이션 확인)', () => {
    // `lastIndexOf` 로 적었다면 `nodeId` 가 `'grp-1/a'` 였을 것이다. 그 값이 나오지
    // 않는다는 것이 이 가드의 전부다.
    expect(parseFrameKey('grp-1/a/b').nodeId).not.toBe('grp-1/a');
  });

  it('맨 앞이 구분자면 `nodeId` 는 빈 문자열이다 (예외를 내지 않는다)', () => {
    expect(parseFrameKey('/body')).toEqual({ nodeId: '', partId: 'body' });
  });
});

describe('isPartKey — 판별의 유일한 자리 (AC-04)', () => {
  it('복합 키는 참이다', () => {
    expect(isPartKey(frameKey('grp-1', 'body'))).toBe(true);
    expect(isPartKey('grp-1/a/b')).toBe(true);
    expect(isPartKey('grp-1/')).toBe(true);
  });

  it('최상위 키는 거짓이다', () => {
    for (const id of ['el-3', 'rect-1', 'grp-1', '']) {
      expect(isPartKey(frameKey(id))).toBe(false);
    }
  });

  it('판별과 분해가 같은 자를 쓴다', () => {
    // 둘이 갈라지면 "부품이라고 했는데 풀면 부품이 아니다" 가 표현 가능해진다.
    for (const key of ['el-3', 'grp-1/body', 'grp-1/a/b', 'grp-1/', '/body', '']) {
      expect(isPartKey(key)).toBe(parseFrameKey(key).partId !== undefined);
    }
  });
});
