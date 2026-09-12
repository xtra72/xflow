// 경로 명령 어휘와 상수 테스트 (SPEC-CANVAS-008 M1).
//
// 재는 것은 셋이다: 씨앗 경로가 **보이는 크기**인가, 명령 상한이 씨앗을 삼키지 않는가,
// 씨앗이 **공유되는 값**임을 형상으로 못박았는가.
//
// 첫째가 이 파일이 있는 이유다. 손상 폴백은 "요소를 버리지 않는다" 는 001 의 정책을
// 잇는데, 폴백이 **보이지 않는 크기**면 그 정책은 이름만 남는다 — 사용자는 화면에서
// 찾을 수 없는 요소를 고칠 수 없다. 그래서 씨앗의 범위를 0..PATH_LOCAL_EXTENT 로
// **수로** 잰다(0..1 이나 0..100 으로 잘못 적으면 상자의 1/10000 짜리 점이 된다).
//
// DOM 무의존이라 jsdom 없이 잰다.
//
// @spec SPEC-CANVAS-008 REQ-01

import { describe, expect, it } from 'vitest';

import { DEFAULT_PATH, MAX_PATH_COMMANDS, PATH_LOCAL_EXTENT, type PathCommand } from './pathTypes';

/** 명령의 좌표만 뽑는다(`Z` 는 좌표가 없다). */
function coordsOf(cmd: PathCommand): readonly number[] {
  switch (cmd.c) {
    case 'M':
    case 'L':
      return [cmd.x, cmd.y];
    case 'C':
      return [cmd.x1, cmd.y1, cmd.x2, cmd.y2, cmd.x, cmd.y];
    case 'Z':
      return [];
  }
}

describe('DEFAULT_PATH — 씨앗 경로', () => {
  it('단위 사각형이다 — M 하나로 시작해 L 셋을 지나 Z 로 닫는다', () => {
    expect(DEFAULT_PATH.map((c) => c.c)).toEqual(['M', 'L', 'L', 'L', 'Z']);
  });

  it('로컬 격자를 **가득** 채운다 — 최소가 0, 최대가 PATH_LOCAL_EXTENT 다', () => {
    const all = DEFAULT_PATH.flatMap(coordsOf);
    // 고정 입력이 0..1 이나 0..100 이면 상자의 1/10000 짜리 점이 되어 화면에서 사라진다.
    expect(Math.min(...all)).toBe(0);
    expect(Math.max(...all)).toBe(PATH_LOCAL_EXTENT);
  });

  it('네 꼭짓점이 모두 다르다 — 한 점으로 무너진 사각형은 그려지지 않는다', () => {
    const points = DEFAULT_PATH.filter((c) => c.c !== 'Z').map((c) => coordsOf(c).join(','));
    expect(new Set(points).size).toBe(points.length);
  });

  it('명령 상한이 씨앗을 삼키지 않는다', () => {
    expect(MAX_PATH_COMMANDS).toBeGreaterThan(DEFAULT_PATH.length);
  });

  it('공유되는 값이므로 얼려 있다 — 읽는 쪽이 제자리에서 고칠 수 없다', () => {
    // 얼지 않았다면 파서 한 번이 다음 폴백의 모양을 바꾼다(전역 오염).
    expect(Object.isFrozen(DEFAULT_PATH)).toBe(true);
    expect(DEFAULT_PATH.every((cmd) => Object.isFrozen(cmd))).toBe(true);
  });
});
