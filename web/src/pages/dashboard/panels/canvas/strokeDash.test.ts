// 파선 무늬 잎 모듈 — 이름 넷 · 판별 · 무늬 산술 (SPEC-CANVAS-012 M2 · AC-07 · AC-14 · AC-15).
//
// 이 파일이 재는 것은 **순수 함수뿐**이다. 캔버스도 DOM 도 나오지 않는다 — 그 최소성이
// 잎 모듈의 값이고, 값이면 시험도 그 모양이어야 한다.

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_STROKE_DASH,
  STROKE_DASH_NAMES,
  dashPattern,
  isStrokeDash,
  type StrokeDash,
} from './strokeDash';

describe('이름 넷 (AC-07)', () => {
  it('목록이 넷이고 그 넷이다', () => {
    expect(STROKE_DASH_NAMES).toEqual(['solid', 'dash', 'dot', 'dashDot']);
  });

  it('기본값이 목록 안에 있다', () => {
    expect(STROKE_DASH_NAMES).toContain(DEFAULT_STROKE_DASH);
  });

  it('타입과 목록이 같은 집합이다 — 다섯째가 생기면 여기가 운다', () => {
    // `Record<StrokeDash, true>` 는 **빠짐과 넘침을 함께** 잡는다. 목록에 더하면서 타입을
    // 넓히지 않으면 넘침으로, 타입을 넓히면서 목록에 더하지 않으면 빠짐으로 운다.
    const seen: Record<StrokeDash, true> = {
      solid: true,
      dash: true,
      dot: true,
      dashDot: true,
    };
    expect(Object.keys(seen).sort()).toEqual([...STROKE_DASH_NAMES].sort());
  });
});

describe('판별 (REQ-06)', () => {
  it.each(STROKE_DASH_NAMES)('`%s` 를 받아들인다', (name) => {
    expect(isStrokeDash(name)).toBe(true);
  });

  // 모르는 값 · 다른 타입 · 경계값. `'Solid'` 가 든 것은 대소문자를 관대하게 읽지
  // **않는다**는 사실을 못박기 위해서다 — 관대하면 저장이 두 표기를 갖는다.
  it.each([undefined, null, '', 'Solid', 'DASH', 'dashdot', 'zigzag', 0, 1, [], {}, [6, 3]])(
    '%o 를 거절한다',
    (value) => {
      expect(isStrokeDash(value)).toBe(false);
    },
  );
});

describe('무늬는 두께의 배수다 (AC-14 · §결정 3)', () => {
  it('`dash` 는 [3w, 2w]', () => {
    expect(dashPattern('dash', 4)).toEqual([12, 8]);
  });

  it('`dot` 은 [w, 2w]', () => {
    expect(dashPattern('dot', 3)).toEqual([3, 6]);
  });

  it('`dashDot` 은 [3w, 2w, w, 2w]', () => {
    expect(dashPattern('dashDot', 2)).toEqual([6, 4, 2, 4]);
  });

  it('두께가 바뀌면 무늬도 **비례해서** 바뀐다 — 절대 길이가 아니다', () => {
    // 이 단언이 §결정 3 의 전부다. 절대 길이였다면 두 배 두께에서 같은 배열이 나온다.
    const thin = dashPattern('dash', 1);
    const thick = dashPattern('dash', 2);
    expect(thick).toEqual(thin.map((n) => n * 2));
  });
});

describe('실선과 퇴화 (AC-15)', () => {
  it('`solid` 는 빈 배열이다 — canvas 에서 빈 배열이 곧 실선이다', () => {
    expect(dashPattern('solid', 4)).toEqual([]);
  });

  it('미지정도 빈 배열이다 — 같은 그림이다', () => {
    expect(dashPattern(undefined, 4)).toEqual([]);
  });

  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])(
    '두께가 %o 면 무늬가 없다 — 칠해지지 않을 선이다',
    (width) => {
      expect(dashPattern('dash', width)).toEqual([]);
    },
  );
});
