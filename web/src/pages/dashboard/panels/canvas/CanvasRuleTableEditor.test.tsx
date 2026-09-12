// 조건 규칙 표 편집기 테스트 (SPEC-CANVAS-001 T8 · REQ-04).
//
// 이 편집기가 사용자에게 가르치는 것은 딱 하나 — "위에서부터 처음 일치한 행 하나가
// 이긴다" 이므로, 테스트의 무게중심도 거기에 둔다: 순서가 보이는가, 순서를 바꿀 수
// 있는가, 그리고 **뒤에서 일치한 행이 가려진 것으로 표시되는가**.
//
// i18n 은 `colorSwatchPalette.test.tsx` 선례대로 키 통과 스텁으로 갈아끼운다 — 문구
// 사본에 테스트를 묶지 않기 위해서다. 그래서 행 안의 개별 컨트롤은 aria-label 이
// 아니라 `data-testid` 로 집는다(스텁 t 는 `{index}` 를 만들어 주지 않는다).
//
// 남은 미도달 분기 세 갈래는 **jsdom 에서 도달할 수 없는 방어**이며 죽은 코드가
// 아니다. 기록해 두지 않으면 다음 사람이 커버리지 구멍을 쫓다 시간을 버린다.
//   - `parseThreshold` / `parseOptionalNumber` 의 비유한 갈래: jsdom 의 number 입력은
//     `1e999` 같은 문자열을 빈 값으로 소독해 버려 Infinity 가 함수까지 닿지 않는다.
//     실제 브라우저는 그 문자열을 그대로 넘기므로 가드는 유지한다.
//   - `moveRow` 의 범위 가드: 양 끝 행의 이동 버튼이 disabled 라 클릭이 발생하지 않는다
//     (`AcControlThresholdsSection` 과 같은 형태의 가드다).

import { describe, it, expect, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { RuleRow } from './canvasConfig';
import CanvasRuleTableEditor from './CanvasRuleTableEditor';

afterEach(cleanup);

/** 테스트에서 반복되는 행 생성기. 패치는 명시적으로만 채운다. */
function row(over: Partial<RuleRow> = {}): RuleRow {
  return { op: 'gt', value: 0, patch: {}, ...over };
}

/** 마지막 `onChange` 인자. 없으면 테스트가 실패하도록 단언한다. */
function lastPayload(spy: ReturnType<typeof vi.fn>): RuleRow[] | undefined {
  expect(spy).toHaveBeenCalled();
  return spy.mock.calls[spy.mock.calls.length - 1]![0] as RuleRow[] | undefined;
}

/** 표를 그리고 onChange 스파이를 돌려준다. */
function setup(
  rules: RuleRow[] | undefined,
  extra: { currentValue?: number | null; disabled?: boolean } = {},
) {
  const onChange = vi.fn();
  render(<CanvasRuleTableEditor rules={rules} onChange={onChange} {...extra} />);
  return onChange;
}

const testid = (id: string): HTMLElement => screen.getByTestId(id);

describe('CanvasRuleTableEditor — 표 렌더와 순서', () => {
  it('규칙이 없으면 빈 상태 안내를 보이고 행을 그리지 않는다', () => {
    setup(undefined);
    expect(screen.getByTestId('canvas-rule-empty')).toBeTruthy();
    expect(screen.queryByTestId('canvas-rule-row-0')).toBeNull();
  });

  it('행을 배열 순서대로 그리고 1-기반 순번을 보인다', () => {
    setup([row({ op: 'gt', value: 90 }), row({ op: 'lt', value: 10 })]);

    expect(testid('canvas-rule-order-0').textContent).toBe('1');
    expect(testid('canvas-rule-order-1').textContent).toBe('2');
    expect((testid('canvas-rule-op-0') as HTMLSelectElement).value).toBe('gt');
    expect((testid('canvas-rule-op-1') as HTMLSelectElement).value).toBe('lt');
    expect((testid('canvas-rule-value-0') as HTMLInputElement).value).toBe('90');
  });

  it('첫 행의 위로 이동과 마지막 행의 아래로 이동은 막혀 있다', () => {
    setup([row(), row()]);
    expect((testid('canvas-rule-move-up-0') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-rule-move-down-1') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-rule-move-down-0') as HTMLButtonElement).disabled).toBe(false);
  });

  // 설명은 줄로 깔지 않고 열 이름 뒤 `?` 에 담는다. 다만 **사라지지는 않는다** —
  // `FieldHelp` 가 sr-only 로 DOM 에 상주시키므로 스크린리더는 팝오버를 열지 않아도 읽는다.
  it('순서 설명을 열 이름 뒤 ? 에 담되 DOM 에서는 항상 읽을 수 있다', () => {
    setup([row()]);
    expect(screen.getByTestId('canvas-rule-order-help').textContent).toBe(
      'dashboard.canvas.rules.firstMatchWins',
    );
    expect(screen.getByText('dashboard.canvas.rules.headerCondition')).toBeTruthy();
    expect(screen.getByText('dashboard.canvas.rules.headerPatch')).toBeTruthy();
  });

  it('? 단추는 키보드로 닿는 button 이며 눌러야 설명이 눈에 보인다', () => {
    setup([row()]);
    // 도움말 단추는 `property.fieldHelp.viewDescription` 를 aria-label 로 쓴다(키 통과 스텁).
    const button = screen.getByRole('button', { name: 'property.fieldHelp.viewDescription' });
    expect(button.getAttribute('aria-expanded')).toBe('false');

    fireEvent.click(button);

    expect(button.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('tooltip').textContent).toBe(
      'dashboard.canvas.rules.firstMatchWins',
    );
  });
});

describe('CanvasRuleTableEditor — 행 추가 · 삭제 · 이동', () => {
  it('행을 추가하면 기본 행이 표 끝에 붙는다', () => {
    const onChange = setup([row({ op: 'lt', value: 5 })]);
    fireEvent.click(testid('canvas-rule-add'));

    expect(lastPayload(onChange)).toEqual([
      { op: 'lt', value: 5, patch: {} },
      { op: 'gt', value: 0, patch: {} },
    ]);
  });

  it('빈 표에서 행을 추가하면 한 행짜리 배열을 올린다', () => {
    const onChange = setup(undefined);
    fireEvent.click(testid('canvas-rule-add'));
    expect(lastPayload(onChange)).toEqual([{ op: 'gt', value: 0, patch: {} }]);
  });

  it('중간 행을 지우면 나머지 순서가 그대로 유지된다', () => {
    const onChange = setup([
      row({ value: 1 }),
      row({ value: 2 }),
      row({ value: 3 }),
    ]);
    fireEvent.click(testid('canvas-rule-delete-1'));

    expect(lastPayload(onChange)).toEqual([
      { op: 'gt', value: 1, patch: {} },
      { op: 'gt', value: 3, patch: {} },
    ]);
  });

  it('마지막 한 행을 지우면 `[]` 가 아니라 undefined 를 올린다', () => {
    // parseCanvasConfig 가 빈 표를 미지정으로 접으므로, `[]` 를 저장하면 저장 왕복에
    // config 가 달라진다. 편집기가 미리 접어서 왕복을 안정시킨다.
    const onChange = setup([row()]);
    fireEvent.click(testid('canvas-rule-delete-0'));

    expect(onChange).toHaveBeenCalledWith(undefined);
    expect(lastPayload(onChange)).toBeUndefined();
  });

  it('행을 위로 올리면 앞 행과 자리를 바꾼다', () => {
    const onChange = setup([row({ value: 1 }), row({ value: 2 })]);
    fireEvent.click(testid('canvas-rule-move-up-1'));

    expect(lastPayload(onChange)).toEqual([
      { op: 'gt', value: 2, patch: {} },
      { op: 'gt', value: 1, patch: {} },
    ]);
  });

  it('행을 아래로 내리면 뒤 행과 자리를 바꾼다', () => {
    const onChange = setup([row({ value: 1 }), row({ value: 2 }), row({ value: 3 })]);
    fireEvent.click(testid('canvas-rule-move-down-0'));

    expect(lastPayload(onChange)).toEqual([
      { op: 'gt', value: 2, patch: {} },
      { op: 'gt', value: 1, patch: {} },
      { op: 'gt', value: 3, patch: {} },
    ]);
  });
});

describe('CanvasRuleTableEditor — 연산자별 임계값 칸', () => {
  it('스칼라 연산자는 임계값 칸 하나를 보인다', () => {
    setup([row({ op: 'gte', value: 42 })]);
    expect(testid('canvas-rule-value-0')).toBeTruthy();
    expect(screen.queryByTestId('canvas-rule-value-low-0')).toBeNull();
    expect(screen.queryByTestId('canvas-rule-value-high-0')).toBeNull();
  });

  it('between 은 칸 두 개를 보인다', () => {
    setup([row({ op: 'between', value: [10, 20] })]);
    expect((testid('canvas-rule-value-low-0') as HTMLInputElement).value).toBe('10');
    expect((testid('canvas-rule-value-high-0') as HTMLInputElement).value).toBe('20');
    expect(screen.queryByTestId('canvas-rule-value-0')).toBeNull();
  });

  it('nodata 는 임계값 칸을 보이지 않는다', () => {
    setup([row({ op: 'nodata', value: 0 })]);
    expect(screen.queryByTestId('canvas-rule-value-0')).toBeNull();
    expect(screen.queryByTestId('canvas-rule-value-low-0')).toBeNull();
  });

  it('임계값을 고치면 그 행만 바뀐다', () => {
    const onChange = setup([row({ value: 1 }), row({ value: 2 })]);
    fireEvent.change(testid('canvas-rule-value-1'), { target: { value: '77' } });

    expect(lastPayload(onChange)).toEqual([
      { op: 'gt', value: 1, patch: {} },
      { op: 'gt', value: 77, patch: {} },
    ]);
  });

  it('임계값 칸을 비우면 0 으로 읽는다 — 임계값은 미지정이 될 수 없다', () => {
    const onChange = setup([row({ value: 5 })]);
    fireEvent.change(testid('canvas-rule-value-0'), { target: { value: '' } });
    expect(lastPayload(onChange)).toEqual([{ op: 'gt', value: 0, patch: {} }]);
  });

  it('between 인데 값이 스칼라인 편집 도중 행도 예외 없이 다룬다', () => {
    // 파서는 이런 행을 만들지 않지만, 연산자만 먼저 바뀐 중간 상태가 화면에 존재할 수
    // 있다. `canvasRules` 가 같은 상황을 판정에서 방어하듯 편집기도 방어한다.
    const onChange = setup([{ op: 'between', value: 7 as never, patch: {} }]);
    expect((testid('canvas-rule-value-low-0') as HTMLInputElement).value).toBe('7');
    expect((testid('canvas-rule-value-high-0') as HTMLInputElement).value).toBe('7');

    fireEvent.change(testid('canvas-rule-value-low-0'), { target: { value: '3' } });
    expect(lastPayload(onChange)).toEqual([{ op: 'between', value: [3, 7], patch: {} }]);
  });

  it('between 의 상한만 고치면 하한은 그대로 둔다(저술 순서 보존)', () => {
    const onChange = setup([row({ op: 'between', value: [30, 40] })]);
    fireEvent.change(testid('canvas-rule-value-high-0'), { target: { value: '99' } });
    expect(lastPayload(onChange)).toEqual([{ op: 'between', value: [30, 99], patch: {} }]);
  });
});

describe('CanvasRuleTableEditor — 연산자 전환은 패치를 잃지 않는다', () => {
  const patch = { fill: '#ff0000', text: 'HOT', visible: true } as const;

  it('스칼라 → between: 임계값을 2원소로 넓히고 패치는 그대로 둔다', () => {
    const onChange = setup([row({ op: 'gt', value: 80, patch: { ...patch } })]);
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'between' } });

    expect(lastPayload(onChange)).toEqual([
      { op: 'between', value: [80, 80], patch: { ...patch } },
    ]);
  });

  it('between → 스칼라: 하한을 임계값으로 접고 패치는 그대로 둔다', () => {
    const onChange = setup([row({ op: 'between', value: [15, 25], patch: { ...patch } })]);
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'lte' } });

    expect(lastPayload(onChange)).toEqual([{ op: 'lte', value: 15, patch: { ...patch } }]);
  });

  it('nodata 로 바꾸면 임계값은 파서와 같이 0 으로 정규화되고 패치는 남는다', () => {
    const onChange = setup([row({ op: 'gt', value: 80, patch: { ...patch } })]);
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'nodata' } });

    expect(lastPayload(onChange)).toEqual([{ op: 'nodata', value: 0, patch: { ...patch } }]);
  });

  it('스칼라 → 스칼라: 임계값도 패치도 그대로 둔다', () => {
    const onChange = setup([row({ op: 'gt', value: 80, patch: { ...patch } })]);
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'lt' } });
    expect(lastPayload(onChange)).toEqual([{ op: 'lt', value: 80, patch: { ...patch } }]);
  });

  it('between → between 은 임계값 튜플을 그대로 유지한다', () => {
    const onChange = setup([row({ op: 'between', value: [1, 9], patch: {} })]);
    fireEvent.change(testid('canvas-rule-op-0'), { target: { value: 'between' } });
    expect(lastPayload(onChange)).toEqual([{ op: 'between', value: [1, 9], patch: {} }]);
  });
});

describe('CanvasRuleTableEditor — 비운 패치 칸은 키 자체가 사라진다', () => {
  it('선 두께를 비우면 patch 에 strokeWidth 키가 남지 않는다', () => {
    // 빈 칸이 0 으로 직렬화되면 규칙이 일치하는 순간 기본 선 두께가 0 으로 덮인다.
    const onChange = setup([row({ patch: { strokeWidth: 3, fill: '#123456' } })]);
    fireEvent.change(testid('canvas-rule-stroke-width-0'), { target: { value: '' } });

    const next = lastPayload(onChange)!;
    expect(next[0]!.patch).not.toHaveProperty('strokeWidth');
    expect(next[0]!.patch).toEqual({ fill: '#123456' });
  });

  it('불투명도를 비우면 patch 에 opacity 키가 남지 않는다', () => {
    const onChange = setup([row({ patch: { opacity: 0.5 } })]);
    fireEvent.change(testid('canvas-rule-opacity-0'), { target: { value: '' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({});
  });

  it('문구를 비우면 patch 에 text 키가 남지 않는다', () => {
    const onChange = setup([row({ patch: { text: 'ALARM' } })]);
    fireEvent.change(testid('canvas-rule-text-0'), { target: { value: '' } });
    expect(lastPayload(onChange)![0]!.patch).not.toHaveProperty('text');
  });

  it('글자 굵기를 미지정으로 되돌리면 fontWeight 키가 사라진다', () => {
    const onChange = setup([row({ patch: { fontWeight: 'bold' } })]);
    fireEvent.change(testid('canvas-rule-font-weight-0'), { target: { value: '' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({});
  });

  it('표시 여부를 미지정으로 되돌리면 visible 키가 사라진다', () => {
    const onChange = setup([row({ patch: { visible: false } })]);
    fireEvent.change(testid('canvas-rule-visible-0'), { target: { value: '' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({});
  });

  it('패치 값을 채우면 그 키만 더해진다', () => {
    const onChange = setup([row({ patch: {} })]);

    fireEvent.change(testid('canvas-rule-stroke-width-0'), { target: { value: '4' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ strokeWidth: 4 });

    fireEvent.change(testid('canvas-rule-opacity-0'), { target: { value: '0.25' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ opacity: 0.25 });

    fireEvent.change(testid('canvas-rule-font-weight-0'), { target: { value: 'bold' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ fontWeight: 'bold' });

    fireEvent.change(testid('canvas-rule-text-0'), { target: { value: 'HOT' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ text: 'HOT' });
  });

  it('표시 여부는 보임/숨김을 미지정과 구분해 담는다', () => {
    const onChange = setup([row({ patch: {} })]);

    fireEvent.change(testid('canvas-rule-visible-0'), { target: { value: 'hide' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ visible: false });

    fireEvent.change(testid('canvas-rule-visible-0'), { target: { value: 'show' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({ visible: true });
  });

  it('숫자가 아닌 패치 입력은 미지정으로 떨어진다', () => {
    const onChange = setup([row({ patch: { strokeWidth: 2 } })]);
    fireEvent.change(testid('canvas-rule-stroke-width-0'), { target: { value: 'abc' } });
    expect(lastPayload(onChange)![0]!.patch).toEqual({});
  });


  it('컬러 스와치는 공용 팔레트를 그대로 쓰며 고른 색을 패치에 담는다', () => {
    // 새 컬러 픽커를 만들지 않고 `colorSwatchPalette` 를 재사용한다 — 팝오버의
    // 팔레트 버튼은 색 문자열 자체를 aria-label 로 쓴다.
    const onChange = setup([row({ patch: {} })]);

    fireEvent.click(testid('canvas-rule-fill-0'));
    const palette = screen.getByRole('dialog').querySelectorAll<HTMLElement>('button');
    expect(palette.length).toBeGreaterThan(1);
    const first = palette[0]!;
    fireEvent.click(first);

    expect(lastPayload(onChange)![0]!.patch).toEqual({
      fill: first.getAttribute('aria-label'),
    });
  });

  it('컬러 스와치를 미설정으로 되돌리면 그 키가 패치에서 사라진다', () => {
    const onChange = setup([row({ patch: { stroke: '#ef4444', fill: '#000000' } })]);

    fireEvent.click(testid('canvas-rule-stroke-0'));
    fireEvent.click(screen.getByLabelText('dashboard.colorSwatch.defaultAria'));

    const next = lastPayload(onChange)![0]!.patch;
    expect(next).not.toHaveProperty('stroke');
    expect(next).toEqual({ fill: '#000000' });
  });

  it('글자색 스와치도 같은 경로로 패치에 담긴다', () => {
    const onChange = setup([row({ patch: {} })]);

    fireEvent.click(testid('canvas-rule-text-color-0'));
    const first = screen.getByRole('dialog').querySelectorAll<HTMLElement>('button')[0]!;
    fireEvent.click(first);

    expect(lastPayload(onChange)![0]!.patch).toEqual({
      textColor: first.getAttribute('aria-label'),
    });
  });
});

describe('CanvasRuleTableEditor — 살아 있는 일치 표시', () => {
  it('두 행이 모두 일치하면 첫 행만 적용, 뒤 행은 가려짐으로 표시한다', () => {
    // 이 편집기가 가르쳐야 하는 단 하나의 개념이다 — 뒤 행이 "일치하지만 안 쓰인다".
    setup([row({ op: 'gt', value: 50 }), row({ op: 'gt', value: 10 })], { currentValue: 90 });

    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('winner');
    expect(testid('canvas-rule-row-1').getAttribute('data-match')).toBe('superseded');
    expect(testid('canvas-rule-badge-0').textContent).toBe('dashboard.canvas.rules.matchWinner');
    expect(testid('canvas-rule-badge-1').textContent).toBe(
      'dashboard.canvas.rules.matchSuperseded',
    );
  });

  it('일치하지 않는 행에는 표시가 붙지 않는다', () => {
    setup([row({ op: 'gt', value: 100 }), row({ op: 'gt', value: 50 })], { currentValue: 90 });

    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('none');
    expect(testid('canvas-rule-row-1').getAttribute('data-match')).toBe('winner');
    expect(screen.queryByTestId('canvas-rule-badge-0')).toBeNull();
  });

  it('순서를 바꾸면 이기는 행도 바뀐다 — 표시가 순서를 그대로 반영한다', () => {
    const rows = [row({ op: 'gt', value: 10 }), row({ op: 'gt', value: 50 })];
    const onChange = vi.fn();
    const { rerender } = render(
      <CanvasRuleTableEditor rules={rows} onChange={onChange} currentValue={90} />,
    );
    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('winner');

    rerender(
      <CanvasRuleTableEditor rules={[rows[1]!, rows[0]!]} onChange={onChange} currentValue={90} />,
    );
    expect((testid('canvas-rule-value-0') as HTMLInputElement).value).toBe('50');
    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('winner');
    expect(testid('canvas-rule-row-1').getAttribute('data-match')).toBe('superseded');
  });

  it('값이 결측이면 nodata 행이 이긴다', () => {
    setup([row({ op: 'gt', value: 0 }), row({ op: 'nodata', value: 0 })], { currentValue: null });

    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('none');
    expect(testid('canvas-rule-row-1').getAttribute('data-match')).toBe('winner');
  });
});

describe('CanvasRuleTableEditor — 바인딩 없음(disabled)', () => {
  it('설명을 보이고 모든 컨트롤을 잠근다', () => {
    setup([row({ op: 'between', value: [1, 2] }), row()], { disabled: true });

    expect(screen.getByTestId('canvas-rule-disabled-hint')).toBeTruthy();
    expect((testid('canvas-rule-op-0') as HTMLSelectElement).disabled).toBe(true);
    expect((testid('canvas-rule-value-low-0') as HTMLInputElement).disabled).toBe(true);
    expect((testid('canvas-rule-value-high-0') as HTMLInputElement).disabled).toBe(true);
    expect((testid('canvas-rule-stroke-width-0') as HTMLInputElement).disabled).toBe(true);
    expect((testid('canvas-rule-opacity-0') as HTMLInputElement).disabled).toBe(true);
    expect((testid('canvas-rule-font-weight-0') as HTMLSelectElement).disabled).toBe(true);
    expect((testid('canvas-rule-visible-0') as HTMLSelectElement).disabled).toBe(true);
    expect((testid('canvas-rule-text-0') as HTMLInputElement).disabled).toBe(true);
    // 공용 컬러 스와치에는 disabled 축이 없어 잠긴 표시 버튼으로 갈아 그린다.
    expect((testid('canvas-rule-fill-0') as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(testid('canvas-rule-fill-0'));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect((testid('canvas-rule-delete-0') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-rule-move-down-0') as HTMLButtonElement).disabled).toBe(true);
    expect((testid('canvas-rule-add') as HTMLButtonElement).disabled).toBe(true);
  });

  it('일치할 수 있는 값이 있어도 표시하지 않는다 — 캔버스가 규칙을 평가하지 않는다', () => {
    setup([row({ op: 'gt', value: 10 })], { currentValue: 90, disabled: true });
    expect(testid('canvas-rule-row-0').getAttribute('data-match')).toBe('none');
    expect(screen.queryByTestId('canvas-rule-badge-0')).toBeNull();
  });

  it('잠긴 컬러 스와치도 지정된 색은 그대로 보여 준다', () => {
    setup([row({ patch: { fill: '#ff0000' } })], { disabled: true });
    const swatch = testid('canvas-rule-fill-0');
    expect(swatch.style.backgroundColor).toBe('rgb(255, 0, 0)');
    // 미지정 칸은 색 대신 표면색 클래스를 쓴다.
    expect(testid('canvas-rule-stroke-0').style.backgroundColor).toBe('');
  });

  it('바인딩이 있으면 설명을 보이지 않는다', () => {
    setup([row()], { currentValue: 1 });
    expect(screen.queryByTestId('canvas-rule-disabled-hint')).toBeNull();
  });
});
