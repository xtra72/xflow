// 경로 요소를 만드는 입구 (SPEC-CANVAS-008 M7 · REQ-02 · 불변식 J9).
//
// 이 파일이 겨누는 함정 둘.
//
// **하나 — 얕은 복사.** 명령 목록을 `[...commands]` 로만 실으면 배열은 새것이되 **명령
// 객체는 카탈로그의 그것 그대로**다. 그때 "값이지 참조가 아니다" 는 절반만 참이고, 놓인
// 도형의 좌표를 고치는 어떤 경로든 카탈로그를 함께 고친다. 그래서 여기서는 **한 겹 안까지**
// 잰다 — 배열이 다른지가 아니라 **원소가 다른지**.
//
// **둘 — 이어 붙는 id 고정 입력.** 요소가 `el-1` 하나뿐인 배열에서는 `el-2` 가 나오는 것이
// 당연해 보이지만, 그 고정 입력은 "그냥 길이에 1 을 더한다" 는 구현도 통과시킨다. 그래서
// 배열에 **구멍을 낸다**(`el-1` · `el-3`) — 옳은 구현만 `el-2` 를 고른다.
//
// @spec SPEC-CANVAS-008 REQ-02 · AC-03

import { describe, expect, it } from 'vitest';

import type { CanvasElement, PathElement } from './canvasConfig';
import {
  SEED_COLOR,
  SEED_STROKE_WIDTH,
  appendPathElement,
  newPathElement,
  pathSeedStyle,
} from './canvasElementFactory';
import { findShape } from './shapes/shapeCatalog';
import type { PathCommand } from './shapes/pathTypes';

/** 카탈로그와 **같은 형상**으로 얼려 둔 원본 — 사본을 만들지 않으면 여기서 드러난다. */
const SOURCE: readonly PathCommand[] = Object.freeze([
  Object.freeze({ c: 'M', x: 0, y: 0 }),
  Object.freeze({ c: 'L', x: 9000, y: 1200 }),
  Object.freeze({ c: 'C', x1: 1, y1: 2, x2: 3, y2: 4, x: 5, y: 6 }),
  Object.freeze({ c: 'Z' }),
] as const);

/** 열린 목록(`Z` 가 없다). */
const OPEN_SOURCE: readonly PathCommand[] = Object.freeze([
  Object.freeze({ c: 'M', x: 10, y: 20 }),
  Object.freeze({ c: 'L', x: 30, y: 40 }),
] as const);

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 1, y: 2, w: 3, h: 4 }, style: {} };
}

describe('경로 요소를 만든다 (REQ-02)', () => {
  it('명령을 **한 겹 안까지** 복사한다 — 값이지 참조가 아니다', () => {
    const created = newPathElement('el-9', 'demo', SOURCE, 0);

    expect(created.path).toEqual([...SOURCE]);
    // 배열이 다른 것만으로는 부족하다.
    expect(created.path).not.toBe(SOURCE);
    for (let i = 0; i < SOURCE.length; i++) {
      expect(created.path[i], `${i} 번째 명령이 원본 객체 그대로다`).not.toBe(SOURCE[i]);
    }

    // 사본이므로 고칠 수 있고, 고쳐도 원본이 그대로다(원본은 얼려 있어 못 고친다).
    const first = created.path[0];
    if (first !== undefined && first.c === 'M') first.x = 777;
    expect(SOURCE[0]).toEqual({ c: 'M', x: 0, y: 0 });
  });

  it('출처를 적되 그리기는 그 값을 읽지 않는다', () => {
    // `catalog_id` 는 표시·감사용이다. 결측이거나 모르는 값이어도 그림이 완전해야 하므로
    // 여기서 재는 것은 "적혔는가" 하나다.
    expect(newPathElement('el-1', 'star5', SOURCE, 0).catalog_id).toBe('star5');
    expect(newPathElement('el-1', '알 수 없는 값', SOURCE, 0).catalog_id).toBe('알 수 없는 값');
  });

  it('씨앗 스타일은 닫혔는가로 갈린다 — 새 색을 지어내지 않는다', () => {
    // 닫힌 것은 `rect` 의 씨앗 그대로, 열린 것은 `line` 의 씨앗 그대로다.
    expect(pathSeedStyle(SOURCE)).toEqual({ fill: SEED_COLOR });
    expect(pathSeedStyle(OPEN_SOURCE)).toEqual({
      stroke: SEED_COLOR,
      strokeWidth: SEED_STROKE_WIDTH,
    });
    // 만든 요소도 같은 함수를 지난다 — 두 번째 규칙이 생기지 않는다.
    expect(newPathElement('a', 'x', OPEN_SOURCE, 0).style).toEqual(pathSeedStyle(OPEN_SOURCE));
  });

  it('상자는 `rect` 와 같은 씨앗 · 같은 계단이다', () => {
    // 계단 한 칸이 25 이고 여덟 칸마다 되감는다. 0 · 1 · 8 을 함께 재어 되감김까지 본다 —
    // `count` 를 그냥 곱하는 구현은 8 에서 갈린다.
    expect(newPathElement('a', 'x', SOURCE, 0).geometry).toEqual({ x: 50, y: 40, w: 100, h: 80 });
    expect(newPathElement('a', 'x', SOURCE, 1).geometry).toEqual({ x: 75, y: 65, w: 100, h: 80 });
    expect(newPathElement('a', 'x', SOURCE, 8).geometry).toEqual({ x: 50, y: 40, w: 100, h: 80 });
  });
});

describe('배열 끝에 붙인다 (REQ-02 · 불변식 J9)', () => {
  it('id 는 배열의 **구멍**을 메우고, 자리는 배열 길이를 따른다', () => {
    const elements = [rect('el-1'), rect('el-3')];
    const { next, created } = appendPathElement(elements, 'star5', SOURCE);

    // 길이에 1 을 더하는 구현은 `el-3` 을 골라 중복을 만든다.
    expect(created.id).toBe('el-2');
    // 끝에 붙는 것이 곧 맨 위다.
    expect(next).toHaveLength(3);
    expect(next[2]).toBe(created);
    // 계단은 **붙이기 전 길이**(2)를 본다.
    expect((created as PathElement).geometry.x).toBe(100);
    // 원본 배열은 그대로다 — 이 함수는 인자를 고치지 않는다.
    expect(elements).toHaveLength(2);
  });

  it('카탈로그의 얼린 명령을 그대로 받아도 던지지 않는다', () => {
    const entry = findShape('cloud');
    expect(entry).toBeDefined();
    if (entry === undefined) return;
    const { created } = appendPathElement([], entry.id, entry.path);
    expect((created as PathElement).path).toEqual([...entry.path]);
    expect((created as PathElement).path[0]).not.toBe(entry.path[0]);
  });
});
