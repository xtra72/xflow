// 조건 규칙 표 평가기 단위 테스트 (SPEC-CANVAS-001 T5).
//
// 고정하는 계약: 첫 일치 승리 순서, 미일치 폴백, `between` 경계 포함과 저술 순서
// 정규화, `nodata` 의 표 내 위치, 부동소수 `eq/ne` 허용 오차, 그리고 순수성.
// DOM 을 쓰지 않으므로 jsdom 없이도 돈다.

import { describe, it, expect } from 'vitest';

import { evaluateRules, matchesRule, RULE_EPSILON, type ResolvedStyle } from './canvasRules';
import type { ElementStyle, RuleOp, RuleRow, RuleValue, StylePatch } from './canvasConfig';

/** 규칙 행을 짧게 만든다(표의 순서가 눈에 들어오도록). */
function row(op: RuleOp, value: RuleValue, patch: StylePatch = {}): RuleRow {
  return { op, value, patch };
}

/** 깊은 비교용 스냅샷. 규칙·스타일은 전부 JSON 안전한 값만 담는다. */
function snapshot<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

/** `nodata` 를 뺀 비교 연산자 7종. "모든 연산자" 를 도는 테스트가 참조한다. */
const SCALAR_OPS: readonly Exclude<RuleOp, 'nodata'>[] = [
  'gt',
  'gte',
  'lt',
  'lte',
  'eq',
  'ne',
  'between',
];

// AC-02 의 표를 그대로 옮긴다.
const AC02_BASE: ElementStyle = { fill: '#9ca3af', stroke: '#000000', strokeWidth: 2 };
const AC02_RULES: RuleRow[] = [
  row('gt', 80, { fill: '#ef4444' }),
  row('gt', 50, { fill: '#eab308', strokeWidth: 4 }),
];

describe('matchesRule — 비교 연산자 집합', () => {
  it('gt 는 임계값 초과에서만 일치한다(경계는 불일치)', () => {
    expect(matchesRule(90, row('gt', 80))).toBe(true);
    expect(matchesRule(80, row('gt', 80))).toBe(false);
  });

  it('gte 는 경계를 포함한다', () => {
    expect(matchesRule(80, row('gte', 80))).toBe(true);
    expect(matchesRule(79.9, row('gte', 80))).toBe(false);
  });

  it('lt 는 임계값 미만에서만 일치한다(경계는 불일치)', () => {
    expect(matchesRule(79, row('lt', 80))).toBe(true);
    expect(matchesRule(80, row('lt', 80))).toBe(false);
  });

  it('lte 는 경계를 포함한다', () => {
    expect(matchesRule(80, row('lte', 80))).toBe(true);
    expect(matchesRule(80.1, row('lte', 80))).toBe(false);
  });

  it('eq 는 같은 값에서만 일치한다', () => {
    expect(matchesRule(80, row('eq', 80))).toBe(true);
    expect(matchesRule(81, row('eq', 80))).toBe(false);
  });

  it('ne 는 eq 의 정확한 부정이다', () => {
    expect(matchesRule(81, row('ne', 80))).toBe(true);
    expect(matchesRule(80, row('ne', 80))).toBe(false);
  });

  it('between 은 양쪽 경계를 포함한다', () => {
    expect(matchesRule(50, row('between', [50, 80]))).toBe(true);
    expect(matchesRule(80, row('between', [50, 80]))).toBe(true);
    expect(matchesRule(65, row('between', [50, 80]))).toBe(true);
  });

  it('between 은 경계 바로 바깥에서 불일치한다', () => {
    expect(matchesRule(49.999, row('between', [50, 80]))).toBe(false);
    expect(matchesRule(80.001, row('between', [50, 80]))).toBe(false);
  });

  it('between 은 저술 순서가 뒤집혀도 같은 구간으로 본다', () => {
    // 파서가 사용자가 적은 순서를 그대로 보존하므로, 정규화는 평가기의 몫이다.
    const reversed = row('between', [80, 50]);
    expect(matchesRule(65, reversed)).toBe(true);
    expect(matchesRule(50, reversed)).toBe(true);
    expect(matchesRule(80, reversed)).toBe(true);
    expect(matchesRule(90, reversed)).toBe(false);
    expect(matchesRule(49, reversed)).toBe(false);
  });

  it('nodata 는 null · undefined · NaN 에 일치한다', () => {
    expect(matchesRule(null, row('nodata', 0))).toBe(true);
    expect(matchesRule(undefined, row('nodata', 0))).toBe(true);
    expect(matchesRule(Number.NaN, row('nodata', 0))).toBe(true);
  });

  it('nodata 는 값이 있으면 불일치한다(0 도 값이다)', () => {
    expect(matchesRule(0, row('nodata', 0))).toBe(false);
    expect(matchesRule(90, row('nodata', 0))).toBe(false);
  });

  it('nodata 를 뺀 모든 연산자는 결측에 일치하지 않는다', () => {
    for (const op of SCALAR_OPS) {
      const r = row(op, op === 'between' ? [0, 1e9] : 0);
      expect(matchesRule(null, r), `${op} / null`).toBe(false);
      expect(matchesRule(undefined, r), `${op} / undefined`).toBe(false);
      expect(matchesRule(Number.NaN, r), `${op} / NaN`).toBe(false);
    }
  });
});

describe('matchesRule — 부동소수 허용 오차', () => {
  it('0.1 + 0.2 는 0.3 과 같다(eq)', () => {
    expect(matchesRule(0.1 + 0.2, row('eq', 0.3))).toBe(true);
  });

  it('0.1 + 0.2 는 0.3 과 다르지 않다(ne)', () => {
    expect(matchesRule(0.1 + 0.2, row('ne', 0.3))).toBe(false);
  });

  it('사용자가 구분하는 자리는 그대로 구분한다', () => {
    expect(matchesRule(0.3001, row('eq', 0.3))).toBe(false);
    expect(matchesRule(0.3001, row('ne', 0.3))).toBe(true);
  });

  it('큰 크기에서는 상대 오차로 동작한다', () => {
    // 1e12 에서 허용 오차는 1e-9 * 1e12 = 1e3 — 배정밀도가 못 세는 자리를 같다고 본다.
    expect(matchesRule(1e12 + 1, row('eq', 1e12))).toBe(true);
    expect(matchesRule(1.1e12, row('eq', 1e12))).toBe(false);
  });

  it('허용 오차 계수는 공개되어 있고 양수다', () => {
    expect(RULE_EPSILON).toBeGreaterThan(0);
    expect(RULE_EPSILON).toBeLessThan(1);
  });
});

describe('matchesRule — Infinity 방어', () => {
  it('Infinity 는 결측이 아니라 값이다', () => {
    expect(matchesRule(Number.POSITIVE_INFINITY, row('nodata', 0))).toBe(false);
    expect(matchesRule(Number.POSITIVE_INFINITY, row('gt', 80))).toBe(true);
    expect(matchesRule(Number.NEGATIVE_INFINITY, row('lt', 0))).toBe(true);
  });

  it('Infinity 는 유한 임계값과 같지 않다(오차 척도가 무한대로 새지 않는다)', () => {
    expect(matchesRule(Number.POSITIVE_INFINITY, row('eq', 80))).toBe(false);
    expect(matchesRule(Number.POSITIVE_INFINITY, row('ne', 80))).toBe(true);
    expect(matchesRule(Number.POSITIVE_INFINITY, row('between', [0, 100]))).toBe(false);
  });

  it('같은 부호의 무한대끼리는 같다', () => {
    expect(matchesRule(Number.POSITIVE_INFINITY, row('eq', Number.POSITIVE_INFINITY))).toBe(true);
    expect(matchesRule(Number.POSITIVE_INFINITY, row('eq', Number.NEGATIVE_INFINITY))).toBe(false);
  });

  it('Infinity 로 평가해도 결과 스타일에 NaN 이 섞이지 않는다', () => {
    const resolved = evaluateRules(Number.POSITIVE_INFINITY, [row('gt', 80, { strokeWidth: 4 })], {
      strokeWidth: 2,
      opacity: 0.5,
    });
    expect(resolved.strokeWidth).toBe(4);
    expect(resolved.opacity).toBe(0.5);
    expect(Number.isNaN(resolved.strokeWidth)).toBe(false);
  });
});

describe('matchesRule — 편집 도중의 어긋난 행 형상', () => {
  // 설정 UI 는 사용자가 연산자를 바꾸는 도중의 행을 그대로 넘길 수 있다.
  // 살아 있는 일치 표시가 예외로 죽지 않아야 한다.
  it('스칼라 연산자에 튜플이 오면 첫 원소를 임계값으로 본다', () => {
    expect(matchesRule(80, row('gt', [70, 90]))).toBe(true);
    expect(matchesRule(60, row('gt', [70, 90]))).toBe(false);
  });

  it('between 에 스칼라가 오면 그 값 하나의 축퇴 구간으로 본다', () => {
    expect(matchesRule(50, row('between', 50))).toBe(true);
    expect(matchesRule(51, row('between', 50))).toBe(false);
  });
});

describe('evaluateRules — AC-02 첫 일치 우선과 기본 스타일 병합', () => {
  it('첫 일치 행만 적용되고 뒤 행은 결과에 섞이지 않는다', () => {
    const resolved = evaluateRules(90, AC02_RULES, AC02_BASE);
    expect(resolved.fill).toBe('#ef4444'); // 첫 일치 행 [gt 80] 의 빨강
    expect(resolved.strokeWidth).toBe(2); // 두 번째 행의 4 가 새어 들어오지 않는다
    expect(resolved.stroke).toBe('#000000'); // 패치에 없는 속성은 기본 스타일 유지
  });

  it('첫 행이 일치하지 않으면 두 번째 행이 이긴다', () => {
    const resolved = evaluateRules(60, AC02_RULES, AC02_BASE);
    expect(resolved.fill).toBe('#eab308');
    expect(resolved.strokeWidth).toBe(4);
    expect(resolved.stroke).toBe('#000000');
  });
});

describe('evaluateRules — 미일치 폴백 (AC-E3)', () => {
  it('어떤 행도 일치하지 않으면 기본 스타일 그대로다', () => {
    const resolved = evaluateRules(10, AC02_RULES, AC02_BASE);
    expect(resolved).toEqual(AC02_BASE);
  });

  it('규칙이 undefined 이면 기본 스타일 그대로다', () => {
    expect(evaluateRules(90, undefined, AC02_BASE)).toEqual(AC02_BASE);
  });

  it('규칙이 빈 배열이어도 기본 스타일 그대로다', () => {
    expect(evaluateRules(90, [], AC02_BASE)).toEqual(AC02_BASE);
  });

  it('폴백도 새 객체를 준다(기본 스타일을 그대로 넘겨주지 않는다)', () => {
    expect(evaluateRules(10, AC02_RULES, AC02_BASE)).not.toBe(AC02_BASE);
  });
});

describe('evaluateRules — nodata 의 표 내 위치 (AC-E2)', () => {
  const gray: StylePatch = { fill: '#6b7280' };
  const black: StylePatch = { fill: '#000000' };

  it('맨 위의 nodata 가 결측을 먼저 잡는다', () => {
    const rules = [row('nodata', 0, gray), row('nodata', 0, black)];
    expect(evaluateRules(null, rules, {}).fill).toBe('#6b7280');
  });

  it('순서를 뒤집으면 다른 행이 이긴다(순서를 사용자가 통제한다)', () => {
    const rules = [row('nodata', 0, black), row('nodata', 0, gray)];
    expect(evaluateRules(null, rules, {}).fill).toBe('#000000');
  });

  it('맨 아래의 nodata 도 위 행이 모두 불일치하면 정상적으로 일치한다', () => {
    const rules = [row('gt', 80, { fill: '#ef4444' }), row('nodata', 0, gray)];
    expect(evaluateRules(null, rules, {}).fill).toBe('#6b7280');
    expect(evaluateRules(undefined, rules, {}).fill).toBe('#6b7280');
    expect(evaluateRules(Number.NaN, rules, {}).fill).toBe('#6b7280');
  });

  it('값이 있으면 nodata 행은 건너뛰고 비교 행이 이긴다', () => {
    const rules = [row('nodata', 0, gray), row('gt', 80, { fill: '#ef4444' })];
    expect(evaluateRules(90, rules, {}).fill).toBe('#ef4444');
  });
});

describe('evaluateRules — 패치 병합 규율', () => {
  it('패치가 담은 모든 속성이 기본 스타일을 덮어쓴다', () => {
    const base: ElementStyle = {
      fill: '#111111',
      stroke: '#222222',
      strokeWidth: 1,
      opacity: 0.1,
      fontSize: 10,
      fontWeight: 'normal',
      textColor: '#333333',
      align: 'left',
      visible: false,
    };
    const patch: StylePatch = {
      fill: '#aaaaaa',
      stroke: '#bbbbbb',
      strokeWidth: 9,
      opacity: 0.9,
      fontSize: 20,
      fontWeight: 'bold',
      textColor: '#cccccc',
      align: 'right',
      visible: true,
      text: '경보',
    };
    expect(evaluateRules(90, [row('gt', 80, patch)], base)).toEqual({ ...base, ...patch });
  });

  it('빈 패치는 기본 스타일을 하나도 바꾸지 않는다', () => {
    const base: ElementStyle = {
      fill: '#111111',
      stroke: '#222222',
      strokeWidth: 1,
      opacity: 0.1,
      fontSize: 10,
      fontWeight: 'normal',
      textColor: '#333333',
      align: 'left',
      visible: false,
    };
    expect(evaluateRules(90, [row('gt', 80, {})], base)).toEqual(base);
  });

  it('명시적 undefined 값은 기본값을 지우지 않는다', () => {
    // JSON 왕복·UI 편집에서 흔한 형태다. 이것으로 색이 사라지면 사용자는 원인을 못 찾는다.
    const patch: StylePatch = { fill: undefined, strokeWidth: undefined, text: undefined };
    const resolved = evaluateRules(90, [row('gt', 80, patch)], AC02_BASE);
    expect(resolved.fill).toBe('#9ca3af');
    expect(resolved.strokeWidth).toBe(2);
    expect(resolved.text).toBeUndefined();
  });

  it('문구 패치가 없으면 결과에 text 가 생기지 않는다(요소 기본 문구를 쓰라는 뜻)', () => {
    const resolved = evaluateRules(90, [row('gt', 80, { fill: '#ef4444' })], AC02_BASE);
    expect('text' in resolved).toBe(false);
  });

  it('결과는 ElementStyle 로 그대로 쓸 수 있다(트윈 엔진의 입력 타입)', () => {
    const resolved: ResolvedStyle = evaluateRules(90, AC02_RULES, AC02_BASE);
    const asStyle: ElementStyle = resolved;
    expect(asStyle.fill).toBe('#ef4444');
  });
});

describe('evaluateRules — 순수성', () => {
  it('기본 스타일 · 규칙 행 · 패치를 변형하지 않는다', () => {
    const base: ElementStyle = { fill: '#9ca3af', stroke: '#000000', strokeWidth: 2 };
    const rules: RuleRow[] = [
      row('nodata', 0, { fill: '#6b7280' }),
      row('gt', 80, { fill: '#ef4444', text: '경보' }),
      row('between', [80, 50], { fill: '#eab308', strokeWidth: 4 }),
    ];
    const baseBefore = snapshot(base);
    const rulesBefore = snapshot(rules);

    evaluateRules(90, rules, base);
    evaluateRules(65, rules, base);
    evaluateRules(null, rules, base);

    expect(base).toEqual(baseBefore);
    expect(rules).toEqual(rulesBefore);
  });

  it('호출마다 새 객체를 준다(프레임 간 공유가 없다)', () => {
    const a = evaluateRules(90, AC02_RULES, AC02_BASE);
    const b = evaluateRules(90, AC02_RULES, AC02_BASE);
    expect(a).toEqual(b);
    expect(a).not.toBe(b);
  });

  it('결과를 고쳐도 기본 스타일에 되비치지 않는다', () => {
    const base: ElementStyle = { fill: '#9ca3af' };
    const resolved = evaluateRules(10, AC02_RULES, base);
    resolved.fill = '#ffffff';
    expect(base.fill).toBe('#9ca3af');
  });
});
