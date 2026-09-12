// 설정된 요소 상한이 가져오기에 닿는 길을 검증한다 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// **두 가지를 가른다.** 하나는 `resolveImportLimits` 라는 순수 함수(서버가 말한 수 하나를
// 상한 한 쌍으로 푼다), 다른 하나는 `planSvgImport` 가 그 쌍을 **실제로 쓰는가**이다.
// 앞쪽만 재면 계획 층이 컴파일 상수를 그대로 읽는 회귀가 초록으로 통과한다 — 이 저장소가
// 이미 물린 부류다.

import { describe, expect, it } from 'vitest';

import { planSvgImport } from './svgImportPlan';
import {
  COMMANDS_PER_ELEMENT,
  DEFAULT_IMPORT_LIMITS,
  MAX_IMPORT_COMMANDS,
  MAX_IMPORT_ELEMENTS,
  resolveImportLimits,
} from './svgImportTypes';

const CANVAS = { width: 800, height: 600 } as const;

/** 요소 n 개짜리 최소 문서 — 사각형은 명령을 한 칸도 쓰지 않는다. */
function rects(n: number): string {
  const body = Array.from(
    { length: n },
    (_v, i) => `<rect x="${i % 20}" y="1" width="3" height="3" fill="#c0392b"/>`,
  ).join('');
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">${body}</svg>`;
}

describe('resolveImportLimits', () => {
  it('양의 정수는 그대로 쓰고 명령 상한을 비만큼 유도한다', () => {
    expect(resolveImportLimits(2048)).toEqual({ maxElements: 2048, maxCommands: 2048 * 10 });
  });

  // **비는 10 이지 16 이 아니다.** 0.7.0 이 두 상한을 각각 ×16 으로 **올린** 것과, 두
  // 상한 **사이의 비**(요소당 10 명령)는 다른 수다. 이 시험은 그 둘을 혼동한 회귀를
  // 잡으라고 있다 — 실제로 한 번 혼동했다.
  it('비는 오늘의 두 상수에서 온 것이지 새로 고른 수가 아니다', () => {
    expect(COMMANDS_PER_ELEMENT).toBe(10);
    expect(MAX_IMPORT_COMMANDS).toBe(MAX_IMPORT_ELEMENTS * COMMANDS_PER_ELEMENT);
  });

  it('기본값을 그대로 넘기면 오늘의 두 상수가 나온다 — 설정 없는 설치는 달라지지 않는다', () => {
    expect(resolveImportLimits(MAX_IMPORT_ELEMENTS)).toEqual(DEFAULT_IMPORT_LIMITS);
  });

  // **값을 못 얻은 길 전부가 기본값으로 떨어진다.** 서버가 이상한 수를 말했다고 가져오기가
  // 서지 않으면 사용자는 스스로 고칠 수 없는 자리에서 막힌다.
  it.each([
    ['undefined(못 물어봤다)', undefined],
    ['0', 0],
    ['음수', -1],
    ['소수', 100.5],
    ['NaN', Number.NaN],
  ])('%s 는 컴파일 기본값으로 떨어진다', (_label, value) => {
    expect(resolveImportLimits(value)).toEqual(DEFAULT_IMPORT_LIMITS);
  });
});

describe('planSvgImport 가 주입된 상한을 쓴다', () => {
  it('상한을 낮추면 그 수에서 거절하고, 거절이 **그 수**를 말한다', () => {
    const limits = resolveImportLimits(10);
    const result = planSvgImport(rects(11), CANVAS, { limits });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.refusal.reason).toBe('tooManyElements');
      expect(result.refusal.actual).toBe(11);
      // 상한을 화면이 함께 말하므로(`{limit}`), 여기서 상수가 새면 사용자가 틀린 수를 읽는다.
      expect(result.refusal.limit).toBe(10);
    }
  });

  it('상한과 정확히 같으면 받는다', () => {
    const result = planSvgImport(rects(10), CANVAS, { limits: resolveImportLimits(10) });

    expect(result.ok).toBe(true);
    if (result.ok) expect(result.shapes).toHaveLength(10);
  });

  it('상한을 올리면 컴파일 기본값을 **넘는** 문서도 받는다', () => {
    const over = MAX_IMPORT_ELEMENTS + 40;

    // 기본값으로는 거절된다 — 이 줄이 아래 줄의 대조군이다.
    const refused = planSvgImport(rects(over), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.limit).toBe(MAX_IMPORT_ELEMENTS);

    // 설정을 올리면 같은 문서가 통과한다.
    const accepted = planSvgImport(rects(over), CANVAS, { limits: resolveImportLimits(over) });
    expect(accepted.ok).toBe(true);
    if (accepted.ok) expect(accepted.shapes).toHaveLength(over);
  });

  it('상한을 주지 않으면 컴파일 기본값이 선다(폴백)', () => {
    const result = planSvgImport(rects(MAX_IMPORT_ELEMENTS + 1), CANVAS);

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.refusal.limit).toBe(MAX_IMPORT_ELEMENTS);
  });

  it('명령 상한도 함께 따라간다 — 요소만 올리고 명령이 남으면 올린 뜻이 사라진다', () => {
    // 요소 4개, 명령은 요소당 12개(= 48). 요소 상한 4 는 넉넉하지만 명령 상한이
    // ×16 을 따라오지 않으면 여기서 잘못 거절된다.
    const path = '<path d="M0 0 C1 1 2 2 3 3 C4 4 5 5 6 6 C7 7 8 8 9 9 C10 10 11 11 12 12"/>';
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">${path.repeat(4)}</svg>`;

    const limits = resolveImportLimits(4);
    expect(limits.maxCommands).toBe(40);

    const result = planSvgImport(svg, CANVAS, { limits });
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.report.commands).toBeLessThanOrEqual(limits.maxCommands);
  });
});
