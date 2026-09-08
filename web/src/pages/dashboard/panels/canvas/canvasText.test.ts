// 문구 템플릿 토큰 치환 단위 테스트 (SPEC-CANVAS-001 T6).
//
// 고정하는 계약은 넷이다: (1) 정의된 3종 토큰만 치환하고 미지 토큰은 원문 유지,
// (2) 값 결측은 결측 표기로 정상 폴백(AC-E2), (3) 치환 문자열의 `$` 특수 시퀀스가
// 발동하지 않음, (4) 단일 패스(재귀 확장 없음). DOM 을 쓰지 않으므로 jsdom 없이 돈다.

import { describe, it, expect } from 'vitest';

import { DEFAULT_DECIMALS } from './canvasConfig';
import {
  renderTextTemplate,
  DEFAULT_MISSING_MARKER,
  MAX_DECIMALS,
  type TextTemplateContext,
} from './canvasText';

/** 타입 검사를 우회해 손상 입력을 밀어 넣는다(config 는 사용자 데이터다). */
function corrupt<T>(v: unknown): T {
  return v as T;
}

describe('renderTextTemplate — AC-04 인수 기준', () => {
  it('"{name}: {value}{unit}" 를 "실외기: 23.5℃" 로 치환한다', () => {
    expect(
      renderTextTemplate('{name}: {value}{unit}', {
        name: '실외기',
        value: 23.456,
        decimals: 1,
        unit: '℃',
      }),
    ).toBe('실외기: 23.5℃');
  });

  it('정의되지 않은 토큰은 치환하지 않고 원문 그대로 남긴다', () => {
    expect(renderTextTemplate('{foo} {value}', { value: 1, decimals: 0 })).toBe('{foo} 1');
  });

  it('미지 토큰만 있는 템플릿은 아무것도 바뀌지 않는다', () => {
    expect(renderTextTemplate('{Value} {NAME} {units} {}', { value: 1, name: 'a' })).toBe(
      '{Value} {NAME} {units} {}',
    );
  });
});

describe('renderTextTemplate — {value} 반올림', () => {
  it.each([
    [23.456, 0, '23'],
    [23.456, 1, '23.5'],
    [23.456, 2, '23.46'],
    [23.456, 3, '23.456'],
  ])('값 %s 를 소수 %s 자리로 반올림한다', (value, decimals, expected) => {
    expect(renderTextTemplate('{value}', { value, decimals })).toBe(expected);
  });

  it('음수도 부호를 유지한 채 반올림한다', () => {
    expect(renderTextTemplate('{value}', { value: -23.456, decimals: 1 })).toBe('-23.5');
    expect(renderTextTemplate('{value}', { value: -0.5, decimals: 0 })).toBe('-1');
  });

  it('정확한 반값은 0 에서 먼 쪽으로 간다(toFixed 규약)', () => {
    expect(renderTextTemplate('{value}', { value: 0.5, decimals: 0 })).toBe('1');
    expect(renderTextTemplate('{value}', { value: 1.5, decimals: 0 })).toBe('2');
    expect(renderTextTemplate('{value}', { value: 2.5, decimals: 0 })).toBe('3');
  });

  it('0 은 결측이 아니다 — 지정 자리로 서식된다', () => {
    expect(renderTextTemplate('{value}', { value: 0, decimals: 2 })).toBe('0.00');
  });

  it('decimals 미지정이면 기본 소수 자리를 쓴다', () => {
    expect(renderTextTemplate('{value}', { value: 23.456 })).toBe(
      (23.456).toFixed(DEFAULT_DECIMALS),
    );
  });
});

describe('renderTextTemplate — AC-E2 결측 값', () => {
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['NaN', Number.NaN],
    ['Infinity', Number.POSITIVE_INFINITY],
    ['-Infinity', Number.NEGATIVE_INFINITY],
  ])('%s 은 기본 결측 표기로 치환된다', (_label, value) => {
    expect(renderTextTemplate('{value}', { value })).toBe(DEFAULT_MISSING_MARKER);
    expect(DEFAULT_MISSING_MARKER).toBe('-');
  });

  it('ctx 자체가 없어도 결측 표기로 떨어진다', () => {
    expect(renderTextTemplate('{value}')).toBe('-');
  });

  it('숫자가 아닌 값(손상 입력)도 결측으로 본다', () => {
    expect(renderTextTemplate('{value}', corrupt<TextTemplateContext>({ value: '23.4' }))).toBe(
      '-',
    );
  });

  it('사용자 지정 결측 표기가 기본값을 덮어쓴다', () => {
    expect(renderTextTemplate('{value}', { value: null, missing: 'N/A' })).toBe('N/A');
  });

  it('빈 문자열 결측 표기도 뜻이 있어 그대로 쓴다', () => {
    expect(renderTextTemplate('[{value}]', { value: null, missing: '' })).toBe('[]');
  });

  it('결측 표기가 문자열이 아니면 기본 표기로 되돌린다', () => {
    expect(
      renderTextTemplate('{value}', corrupt<TextTemplateContext>({ value: null, missing: 7 })),
    ).toBe(DEFAULT_MISSING_MARKER);
  });
});

describe('renderTextTemplate — {name} / {unit} 부재', () => {
  it('이름이 없으면 빈 문자열로 치환한다(결측 표기가 아니다)', () => {
    expect(renderTextTemplate('[{name}]', { value: 1 })).toBe('[]');
  });

  it('단위가 없으면 빈 문자열로 치환한다', () => {
    expect(renderTextTemplate('{value}{unit}', { value: 1, decimals: 0 })).toBe('1');
  });

  it('이름·단위가 문자열이 아니면 빈 문자열로 본다', () => {
    expect(
      renderTextTemplate(
        '[{name}][{unit}]',
        corrupt<TextTemplateContext>({ name: 42, unit: null }),
      ),
    ).toBe('[][]');
  });

  it('같은 토큰이 여러 번 나오면 모두 치환한다', () => {
    expect(renderTextTemplate('{name}/{name}', { name: 'a' })).toBe('a/a');
  });
});

describe('renderTextTemplate — 치환 문자열의 $ 특수 시퀀스 함정', () => {
  it("이름에 $& · $' 가 있어도 그대로 출력한다", () => {
    expect(renderTextTemplate('<{name}>', { name: "A$&B$'C" })).toBe("<A$&B$'C>");
  });

  it('단위에 $` · $1 · $$ 가 있어도 그대로 출력한다', () => {
    expect(renderTextTemplate('{unit}', { unit: '$`$1$$' })).toBe('$`$1$$');
  });

  it('결측 표기에 $& 가 있어도 그대로 출력한다', () => {
    expect(renderTextTemplate('{value}', { value: null, missing: '$&' })).toBe('$&');
  });
});

describe('renderTextTemplate — 단일 패스(재귀 확장 없음)', () => {
  it('{name} 이 "{value}" 로 풀려도 다시 치환하지 않는다', () => {
    expect(renderTextTemplate('{name}', { name: '{value}', value: 5, decimals: 1 })).toBe(
      '{value}',
    );
  });

  it('단위가 토큰 모양이어도 다시 치환하지 않는다', () => {
    expect(renderTextTemplate('{value}{unit}', { value: 1, decimals: 0, unit: '{name}' })).toBe(
      '1{name}',
    );
  });
});

describe('renderTextTemplate — 템플릿 방어', () => {
  it('빈 템플릿은 빈 문자열이다', () => {
    expect(renderTextTemplate('', { value: 1 })).toBe('');
  });

  it.each([
    ['undefined', undefined],
    ['null', null],
    ['숫자', 42],
    ['객체', { a: 1 }],
    ['배열', ['{value}']],
  ])('%s 템플릿은 예외 없이 빈 문자열이다', (_label, template) => {
    expect(renderTextTemplate(corrupt<string>(template), { value: 1 })).toBe('');
  });

  it('토큰이 없는 템플릿은 원문 그대로다', () => {
    expect(renderTextTemplate('정지', { value: 1, name: 'a' })).toBe('정지');
  });
});

describe('renderTextTemplate — decimals 방어', () => {
  it.each([
    ['음수', -1],
    ['NaN', Number.NaN],
    ['Infinity', Number.POSITIVE_INFINITY],
    ['-Infinity', Number.NEGATIVE_INFINITY],
  ])('%s decimals 는 기본 소수 자리로 되돌린다', (_label, decimals) => {
    expect(renderTextTemplate('{value}', { value: 23.456, decimals })).toBe(
      (23.456).toFixed(DEFAULT_DECIMALS),
    );
  });

  it('숫자가 아닌 decimals 도 기본 소수 자리로 되돌린다', () => {
    expect(
      renderTextTemplate('{value}', corrupt<TextTemplateContext>({ value: 23.456, decimals: '2' })),
    ).toBe((23.456).toFixed(DEFAULT_DECIMALS));
  });

  it('소수 decimals 는 잘라낸다', () => {
    expect(renderTextTemplate('{value}', { value: 23.456, decimals: 1.9 })).toBe('23.5');
  });

  it('터무니없이 큰 decimals 는 상한으로 clamp 되어 예외를 던지지 않는다', () => {
    const out = renderTextTemplate('{value}', { value: 1, decimals: 1e9 });
    expect(out).toBe((1).toFixed(MAX_DECIMALS));
    expect(MAX_DECIMALS).toBe(20);
  });

  it('상한 경계(20)는 그대로 통과한다', () => {
    expect(renderTextTemplate('{value}', { value: 1, decimals: MAX_DECIMALS })).toBe(
      (1).toFixed(20),
    );
  });

  it('decimals 0 은 기본값으로 대체되지 않는다', () => {
    expect(renderTextTemplate('{value}', { value: 23.456, decimals: 0 })).toBe('23');
  });
});

describe('renderTextTemplate — 호출 간 상태 누수', () => {
  it('같은 템플릿을 연속 호출해도 결과가 같다(정규식 lastIndex 누수 없음)', () => {
    const ctx: TextTemplateContext = { name: 'a', value: 1, decimals: 0, unit: 'x' };
    const first = renderTextTemplate('{name}:{value}{unit}', ctx);
    expect(renderTextTemplate('{name}:{value}{unit}', ctx)).toBe(first);
    expect(first).toBe('a:1x');
  });
});
