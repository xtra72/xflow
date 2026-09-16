// 파선 축이 저장 · 규칙 · 트윈을 지나는 길 (SPEC-CANVAS-012 M2 · AC-08~AC-12).
//
// **이 파일이 막는 실패는 크래시가 아니다.** 008 이 예고한 대로 이 축은 `ElementStyle` 을
// 넓히고 규칙 패치와 004 의 캐스케이드를 함께 넓힌다. 그 넓어짐이 **끝까지 이어지는지**를
// 여기서 잰다 — 파서만 알고 규칙이 모르면, 저장에서는 통과한 점선이 규칙이 한 번 맞는
// 순간 조용히 실선으로 돌아간다.
//
// @spec SPEC-CANVAS-012 REQ-04 · REQ-06

import { describe, expect, it } from 'vitest';

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { parseCanvasConfig, type ElementStyle, type RuleRow } from './canvasConfig';
import { drawElements, type DrawContext2D } from './drawElement';
import { evaluateRules } from './canvasRules';
import { beginTween, sampleTween } from './canvasTween';
import { STROKE_DASH_NAMES } from './strokeDash';

/** 기본값 모양이 아닌 상자 — 원점 ≠ 0 · `x ≠ y` · 비정사각(시험 규율 D2·D3). */
const BOX = { x: 31, y: 57, w: 140, h: 90 } as const;

function rawConfig(style: Record<string, unknown>): Record<string, unknown> {
  return {
    elements: [{ id: 'el-1', kind: 'rect', geometry: { ...BOX }, style }],
  };
}

/** 파싱을 지난 첫 요소의 스타일. 사라졌으면 **여기서** 죽는다 — 조용한 폴백을 두지 않는다. */
function parsedStyle(style: Record<string, unknown>): ElementStyle {
  const cfg = parseCanvasConfig(rawConfig(style));
  const node = cfg.elements[0];
  if (node === undefined || !('style' in node) || node.style === undefined) {
    throw new Error('요소가 사라졌다');
  }
  return node.style;
}

describe('저장 왕복 (AC-08)', () => {
  it.each(STROKE_DASH_NAMES)('`%s` 가 왕복을 견딘다', (name) => {
    expect(parsedStyle({ strokeDash: name }).strokeDash).toBe(name);
  });
});

describe('모르는 값은 키만 버린다 (AC-09 · REQ-06)', () => {
  it('형제 스타일이 살아남는다 — 요소를 통째로 떨어뜨리지 않는다', () => {
    const style = parsedStyle({ stroke: '#ff0000', strokeWidth: 2, strokeDash: 'zigzag' });
    expect(style.strokeDash).toBeUndefined();
    expect(style.stroke).toBe('#ff0000');
    expect(style.strokeWidth).toBe(2);
  });

  it('요소 자체가 사라지지 않는다', () => {
    // 이것이 이 절의 진짜 요구다. 파서가 모르는 값에 엄격해지면 저장된 도형이 예외 하나
    // 없이 **읽을 때 사라지고**, 화면은 아무 말도 하지 않는다(008 위험 R2 와 같은 부류).
    const cfg = parseCanvasConfig(rawConfig({ strokeDash: 'zigzag' }));
    expect(cfg.elements).toHaveLength(1);
  });

  it.each([null, 42, [6, 3], { dash: true }, 'Solid'])('%o 도 같은 규율이다', (bad) => {
    expect(parsedStyle({ strokeDash: bad }).strokeDash).toBeUndefined();
  });
});

describe('미지정과 `solid` 는 다른 값이다 (AC-10)', () => {
  it('미지정이면 키가 없다', () => {
    expect('strokeDash' in parsedStyle({ stroke: '#000' })).toBe(false);
  });

  it('`solid` 면 키가 있다', () => {
    expect('strokeDash' in parsedStyle({ strokeDash: 'solid' })).toBe(true);
  });

  it('직렬화가 그 둘을 가른다', () => {
    // 그림은 같지만 뜻이 다르다 — 미지정은 캐스케이드 상위가 정할 수 있는 자리이고
    // `solid` 는 저술자가 정한 값이다. 직렬화에서 둘이 같아지면 "무슨 일이 있어도
    // 실선" 을 표현할 방법이 사라진다.
    const unset = JSON.stringify(parsedStyle({ stroke: '#000' }));
    const solid = JSON.stringify(parsedStyle({ stroke: '#000', strokeDash: 'solid' }));
    expect(unset).not.toBe(solid);
  });
});

describe('규칙 패치가 이 축을 덮어쓴다 (AC-11)', () => {
  const base: ElementStyle = { stroke: '#000', strokeWidth: 2, strokeDash: 'solid' };
  const rows: RuleRow[] = [{ op: 'gt', value: 10, patch: { strokeDash: 'dot' } }];

  it('일치하면 패치가 이긴다', () => {
    expect(evaluateRules(20, rows, base).strokeDash).toBe('dot');
  });

  it('빗나가면 기본이 남는다', () => {
    expect(evaluateRules(5, rows, base).strokeDash).toBe('solid');
  });

  it('패치가 이 축을 말하지 않으면 기본이 남는다 — 지우지 않는다', () => {
    const quiet: RuleRow[] = [{ op: 'gt', value: 10, patch: { stroke: '#f00' } }];
    const out = evaluateRules(20, quiet, base);
    expect(out.strokeDash).toBe('solid');
    expect(out.stroke).toBe('#f00');
  });
});

describe('패치 열거가 새지 않는다 — 다음 축을 위한 가드', () => {
  it('`ElementStyle` 의 모든 축이 `mergePatch` 를 통과한다', () => {
    // **012 가 걸린 함정을 다음 축이 다시 밟지 않게 하는 가드다.**
    //
    // `mergePatch` 는 키를 손으로 열거한다. 그 방식 자체는 옳다(`StylePatch` 가
    // `Partial<ElementStyle>` 이라 스프레드로는 `undefined` 가 기본을 덮어쓴다). 위험한
    // 것은 열거가 **조용히 낡는다**는 것이다 — 축을 더하고 그 줄을 빠뜨리면 타입도 린트도
    // 울지 않고, 저장에서는 통과한 값이 규칙이 한 번 맞는 순간 사라진다. 이미
    // `canvasRules.ts` 머리말이 `fontSize`·`align` 에 대해 그 부류를 경고했고, 012 가
    // 셋째 사례가 될 뻔했다.
    //
    // `Record<keyof ElementStyle, …>` 라 **새 축이 생기면 이 객체가 컴파일되지 않는다.**
    // 그때 채워 넣으면 그 축이 실제로 통과하는지를 아래 루프가 바로 잰다.
    const PROBE: { [K in keyof ElementStyle]-?: NonNullable<ElementStyle[K]> } = {
      fill: '#123456',
      stroke: '#654321',
      strokeWidth: 7,
      strokeDash: 'dashDot',
      opacity: 0.25,
      fontSize: 19,
      fontWeight: 'bold',
      textColor: '#abcdef',
      align: 'right',
      visible: false,
    };
    // 기본은 **탐침과 모든 자리에서 다르다** — 같으면 "덮어썼다" 와 "원래 그랬다" 가
    // 구분되지 않는다.
    const base: ElementStyle = {
      fill: '#000000',
      stroke: '#000000',
      strokeWidth: 1,
      strokeDash: 'solid',
      opacity: 1,
      fontSize: 14,
      fontWeight: 'normal',
      textColor: '#000000',
      align: 'left',
      visible: true,
    };
    const rows: RuleRow[] = [{ op: 'gt', value: 0, patch: { ...PROBE } }];
    const out = evaluateRules(1, rows, base);
    for (const key of Object.keys(PROBE) as (keyof ElementStyle)[]) {
      expect(out[key], key).toBe(PROBE[key]);
    }
  });
});

describe('백분율은 화면의 단위일 뿐이다 (AC-28 · AC-29 · AC-30)', () => {
  it('저장 형식이 한 바이트도 바뀌지 않는다 (AC-29)', () => {
    // **012 이전에 저장된 대시보드가 그대로 읽혀야 한다.** 저장을 100 눈금으로 바꿨다면
    // 이 왕복에서 `0.5` 가 `0.5%` 로 읽히거나 50 으로 부풀었을 것이다.
    const raw = rawConfig({ opacity: 0.5 });
    const once = parseCanvasConfig(raw);
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify(once)));
    expect(JSON.stringify(once)).toBe(JSON.stringify(twice));
    expect(parsedStyle({ opacity: 0.5 }).opacity).toBe(0.5);
  });

  it('렌더가 옛 눈금을 그대로 읽는다 (AC-30)', () => {
    // `resolveAlpha` 는 비공개라 직접 부를 수 없다. 대신 **관측 가능한 결과**로 잰다:
    // 저장 0.5 가 `globalAlpha` 0.5 로 간다. 저장이 백분율이었다면 여기가 50 이 되고
    // canvas 는 그 값을 1 로 죄어 **투명도가 통째로 사라진다.**
    const calls: [string, unknown][] = [];
    const rec = (op: string) => (): void => {
      calls.push([op, undefined]);
    };
    let globalAlpha = 1;
    const ctx = {
      save: rec('save'),
      restore: rec('restore'),
      setTransform: rec('setTransform'),
      beginPath: rec('beginPath'),
      rect: rec('rect'),
      ellipse: rec('ellipse'),
      moveTo: rec('moveTo'),
      lineTo: rec('lineTo'),
      closePath: rec('closePath'),
      bezierCurveTo: rec('bezierCurveTo'),
      stroke: rec('stroke'),
      fill: rec('fill'),
      fillText: rec('fillText'),
      clearRect: rec('clearRect'),
      fillRect: rec('fillRect'),
      measureText: (text: string) => ({ width: text.length * 10 }),
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 1,
      get globalAlpha() {
        return globalAlpha;
      },
      set globalAlpha(v: number) {
        globalAlpha = v;
        calls.push(['globalAlpha', v]);
      },
      font: '',
      textAlign: 'left' as CanvasTextAlign,
      textBaseline: 'alphabetic' as CanvasTextBaseline,
    } satisfies DrawContext2D;

    const el = parseCanvasConfig(rawConfig({ fill: '#123456', opacity: 0.5 })).elements;
    drawElements(ctx, el, {}, {}, { stage: { width: 250, height: 200 }, canvas: { width: 500, height: 400 } });
    expect(calls.filter((c) => c[0] === 'globalAlpha').map((c) => c[1])).toContain(0.5);
  });

  it('환산 산술이 칸마다 흩어져 있지 않다 (AC-28)', () => {
    // **소스 판정이다.** 네 칸(요소 · 그룹 · 규칙 표 · 연결선)이 제각기 `* 100` · `/ 100`
    // 을 적으면 그중 하나가 반올림을 다르게 하는 날이 오고, 그때 "같은 값인데 칸마다
    // 다르게 보인다" 가 시작된다. 환산은 잎 모듈 한 쌍만 한다.
    const sources = [
      'src/pages/dashboard/panels/canvas/CanvasElementsEditor.tsx',
      'src/pages/dashboard/panels/canvas/CanvasRuleTableEditor.tsx',
    ];
    for (const file of sources) {
      const text = readFileSync(resolve(process.cwd(), file), 'utf8');
      // 주석은 뺀다 — 설명에 적힌 `÷ 100` 은 산술이 아니다.
      const code = text
        .split('\n')
        .filter((line) => !line.trimStart().startsWith('//') && !line.trimStart().startsWith('*'))
        .join('\n');
      expect(code, `${file}: opacity 환산 산술`).not.toMatch(/opacity[^\n]*[*/]\s*100/);
      // 그리고 잎 모듈을 **실제로** 지난다.
      expect(text, `${file}: opacityPercent 사용`).toContain('percentInputToOpacity');
      expect(text, `${file}: opacityPercent 사용`).toContain('opacityToPercentInput');
    }
  });
});

describe('트윈이 이 축을 건너뛴다 (AC-12 · §결정 D6)', () => {
  it('중간 프레임에도 값이 **이름 둘 가운데 하나**다', () => {
    const from: ElementStyle = { strokeDash: 'solid', opacity: 0 };
    const to: ElementStyle = { strokeDash: 'dashDot', opacity: 1 };
    const state = beginTween(from, to, { duration_ms: 1000, easing: 'linear' }, 0);

    const mid = sampleTween(state, 500);
    // 즉시 전환이므로 첫 프레임부터 목표값이다. 보간할 중간값이 없는 축이다.
    expect(mid.style.strokeDash).toBe('dashDot');
    expect(STROKE_DASH_NAMES).toContain(mid.style.strokeDash);
    // 같은 트윈에서 **수치 축은 실제로 보간되고 있다** — 트윈이 멈춘 것이 아니라
    // 이 축만 건너뛴다는 뜻이다. 이 짝단언이 없으면 "트윈이 통째로 죽었다" 와
    // 구분되지 않는다.
    expect(mid.style.opacity).toBeGreaterThan(0);
    expect(mid.style.opacity).toBeLessThan(1);
  });
});
