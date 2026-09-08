// 속성 그리드 배치·디자인 설정 해석 검증.

import { describe, expect, it } from 'vitest';

import {
  applyPropertyOverride,
  DEFAULT_GRID_COLS,
  isStaleValue,
  moveArea,
  MAX_CARD_DIV,
  MIN_CARD_DIV,
  MAX_GRID_COLS,
  clampGridCols,
  moveToPosition,
  readPropertiesGridStyle,
  readPropertyOverride,
  reorderVisible,
  resizeArea,
  resolveCardLayout,
  resolveValueColor,
  selectEntries,
} from './propertiesGridStyle';

describe('clampGridCols', () => {
  it('정수로 맞춘다', () => {
    expect(clampGridCols(3.4)).toBe(3);
    expect(clampGridCols('4')).toBe(4);
  });

  it('한계 안으로 가둔다', () => {
    expect(clampGridCols(0)).toBe(DEFAULT_GRID_COLS);
    expect(clampGridCols(-2)).toBe(DEFAULT_GRID_COLS);
    expect(clampGridCols(999)).toBe(MAX_GRID_COLS);
  });

  it('손상/미설정 값은 기본값으로 떨어진다', () => {
    expect(clampGridCols(undefined)).toBe(DEFAULT_GRID_COLS);
    expect(clampGridCols('abc')).toBe(DEFAULT_GRID_COLS);
    expect(clampGridCols(null)).toBe(DEFAULT_GRID_COLS);
  });

  it('종전 버튼으로 고를 수 있던 1~6 을 넘어서도 받는다', () => {
    expect(clampGridCols(8)).toBe(8);
  });
});

describe('카드 분할', () => {
  it('기본은 3×3', () => {
    const { cardGrid } = readPropertiesGridStyle({});
    expect(cardGrid).toEqual({ rows: 3, cols: 3 });
  });

  it('행·열 수를 정한다', () => {
    expect(readPropertiesGridStyle({ cardRows: 2, cardCols: 4 }).cardGrid).toEqual({
      rows: 2,
      cols: 4,
    });
  });

  it('한계를 벗어나면 가둔다 — 너무 잘게 나누면 칸이 글자보다 좁아진다', () => {
    expect(readPropertiesGridStyle({ cardRows: 0, cardCols: 99 }).cardGrid).toEqual({
      rows: MIN_CARD_DIV,
      cols: MAX_CARD_DIV,
    });
  });
});

describe('카드 배치 — 시작 위치 + 칸 수', () => {
  it('기본은 좌열 세로 1칸씩 — 항목명 / 값 / 시각', () => {
    const { areas } = readPropertiesGridStyle({});
    expect(areas.label).toEqual({ row: 1, col: 1, rowSpan: 1, colSpan: 1 });
    expect(areas.value).toEqual({ row: 2, col: 1, rowSpan: 1, colSpan: 1 });
    expect(areas.time).toEqual({ row: 3, col: 1, rowSpan: 1, colSpan: 1 });
  });

  it('여러 칸을 쓰는 배치를 만든다 — 값을 위쪽 한 줄 전체로', () => {
    const { areas } = readPropertiesGridStyle({
      cardAreas: {
        value: { row: 1, col: 1, rowSpan: 1, colSpan: 3 },
        label: { row: 2, col: 1, rowSpan: 1, colSpan: 2 },
        time: { row: 2, col: 3, rowSpan: 1, colSpan: 1 },
      },
    });
    expect(areas.value).toEqual({ row: 1, col: 1, rowSpan: 1, colSpan: 3 });
    expect(areas.label.colSpan).toBe(2);
  });

  it('격자를 벗어나는 칸 수는 끝까지로 줄인다', () => {
    const { areas } = readPropertiesGridStyle({
      cardAreas: { label: { row: 3, col: 2, rowSpan: 5, colSpan: 5 } },
    });
    // 3×3 에서 (3,2) 시작이면 남은 칸은 1행 2열.
    expect(areas.label).toEqual({ row: 3, col: 2, rowSpan: 1, colSpan: 2 });
  });

  it('영역이 겹치면 뒤 조각을 빈 칸으로 민다 — 겹치면 글자가 포개진다', () => {
    const { areas } = readPropertiesGridStyle({
      cardAreas: {
        label: { row: 1, col: 1, rowSpan: 2, colSpan: 2 },
        value: { row: 1, col: 1, rowSpan: 1, colSpan: 1 },
      },
    });

    expect(areas.label).toEqual({ row: 1, col: 1, rowSpan: 2, colSpan: 2 });
    // 값은 겹치지 않는 첫 칸으로 밀린다.
    expect(areas.value.row === 1 && areas.value.col === 1).toBe(false);
    expect(areas.value.colSpan).toBe(1);
  });

  it('종전 칸 번호(1~9)를 그대로 옮긴다', () => {
    const { areas } = readPropertiesGridStyle({
      cardSlots: { label: 3, value: 5, time: 9 },
    });
    expect(areas.label).toEqual({ row: 1, col: 3, rowSpan: 1, colSpan: 1 });
    expect(areas.value).toEqual({ row: 2, col: 2, rowSpan: 1, colSpan: 1 });
    expect(areas.time).toEqual({ row: 3, col: 3, rowSpan: 1, colSpan: 1 });
  });

  it.each([
    ['stack', { label: [1, 1], value: [2, 1], time: [3, 1] }],
    ['inline', { label: [1, 1], value: [1, 3], time: [3, 1] }],
    ['value', { value: [1, 1], label: [2, 1], time: [3, 1] }],
  ])('더 오래된 %s 배치도 그대로 옮긴다', (layout, expected) => {
    const { areas } = readPropertiesGridStyle({ cardLayout: layout });
    for (const [element, [row, col]] of Object.entries(expected)) {
      const area = areas[element as 'label' | 'value' | 'time'];
      expect([area.row, area.col]).toEqual([row, col]);
    }
  });

  it('cardAreas 가 있으면 옛 형식은 무시한다', () => {
    const { areas } = readPropertiesGridStyle({
      cardLayout: 'value',
      cardSlots: { label: 9 },
      cardAreas: { label: { row: 2, col: 2, rowSpan: 1, colSpan: 1 } },
    });
    expect(areas.label).toEqual({ row: 2, col: 2, rowSpan: 1, colSpan: 1 });
  });
});

describe('갱신 시각 표시', () => {
  it('기본은 표시다', () => {
    expect(readPropertiesGridStyle({}).showUpdatedAt).toBe(true);
  });

  it('명시적으로 false 일 때만 숨긴다', () => {
    expect(readPropertiesGridStyle({ showUpdatedAt: false }).showUpdatedAt).toBe(false);
    expect(readPropertiesGridStyle({ showUpdatedAt: true }).showUpdatedAt).toBe(true);
  });
});

describe('글자 디자인', () => {
  it('항목명·값·시각을 따로 정한다', () => {
    const style = readPropertiesGridStyle({
      label_font: { size: 10 },
      value_font: { size: 24, weight: 'bold' },
      time_font: { size: 9 },
    });

    expect(style.labelStyle).toMatchObject({ fontSize: '10px' });
    expect(style.valueStyle).toMatchObject({ fontSize: '24px', fontWeight: 'bold' });
    expect(style.timeStyle).toMatchObject({ fontSize: '9px' });
  });

  it('항목명 색을 정하지 않았으면 종전 labels 악센트를 쓴다', () => {
    const style = readPropertiesGridStyle({ accentElements: { labels: '#ff0000' } });
    expect(style.labelStyle).toMatchObject({ color: '#ff0000' });
  });

  it('디자인 설정의 색이 악센트를 이긴다', () => {
    const style = readPropertiesGridStyle({
      accentElements: { labels: '#ff0000' },
      label_font: { color: '#00ff00' },
    });
    expect(style.labelStyle).toMatchObject({ color: '#00ff00' });
  });

  it('아무것도 정하지 않으면 스타일을 만들지 않는다', () => {
    const style = readPropertiesGridStyle({});
    expect(style.labelStyle).toBeUndefined();
    expect(style.valueStyle).toBeUndefined();
    expect(style.timeStyle).toBeUndefined();
  });
});

describe('selectEntries — 고른 항목을 고른 순서대로', () => {
  const entries = [
    { key: 'temperature' },
    { key: 'humidity' },
    { key: 'power' },
    { key: 'gw.rssi' },
    { key: 'meta.name' },
  ];

  it('선택 순서가 곧 배치 순서다', () => {
    const out = selectEntries(entries, ['power', 'temperature']);
    expect(out.map((e) => e.key)).toEqual(['power', 'temperature']);
  });

  it('고르지 않은 항목은 빠진다', () => {
    const out = selectEntries(entries, ['humidity']);
    expect(out.map((e) => e.key)).toEqual(['humidity']);
  });

  it('목록에 있지만 이 디바이스에 없는 키는 조용히 무시한다', () => {
    const out = selectEntries(entries, ['power', 'not_reported']);
    expect(out.map((e) => e.key)).toEqual(['power']);
  });

  it('전체(미선택)는 속성만 원래 순서로 낸다 — 파생 카드는 골랐을 때만', () => {
    const out = selectEntries(entries, []);
    expect(out.map((e) => e.key)).toEqual(['temperature', 'humidity', 'power']);
  });

  it('전체가 아니면 파생 카드도 고른 순서대로 낸다', () => {
    const out = selectEntries(entries, ['meta.name', 'gw.rssi']);
    expect(out.map((e) => e.key)).toEqual(['meta.name', 'gw.rssi']);
  });
});

describe('moveToPosition — 배치 순번 지정', () => {
  const list = ['a', 'b', 'c', 'd'];

  it('끼워 넣는다 — 자리를 맞바꾸지 않는다', () => {
    // 맞바꾸면 c 를 1번으로 옮겼을 때 a 가 3번으로 떨어진다.
    expect(moveToPosition(list, 'c', 1)).toEqual(['c', 'a', 'b', 'd']);
  });

  it('뒤로도 옮긴다', () => {
    expect(moveToPosition(list, 'a', 3)).toEqual(['b', 'c', 'a', 'd']);
  });

  it('범위를 벗어난 순번은 양 끝으로 가둔다', () => {
    expect(moveToPosition(list, 'c', 0)).toEqual(['c', 'a', 'b', 'd']);
    expect(moveToPosition(list, 'a', 99)).toEqual(['b', 'c', 'd', 'a']);
  });

  it('같은 자리면 그대로 둔다', () => {
    expect(moveToPosition(list, 'b', 2)).toBe(list);
  });

  it('목록에 없는 키는 그대로 둔다', () => {
    expect(moveToPosition(list, 'zzz', 1)).toBe(list);
  });

  it('숫자가 아니면 그대로 둔다', () => {
    expect(moveToPosition(list, 'c', Number.NaN)).toBe(list);
  });
});

describe('값에 따른 색', () => {
  it('먼저 맞는 규칙이 이긴다 — 순서가 곧 우선순위', () => {
    const rules = [
      { op: 'gte' as const, value: '30', color: '#ff0000' },
      { op: 'gte' as const, value: '20', color: '#ffaa00' },
    ];
    expect(resolveValueColor(rules, 35)).toBe('#ff0000');
    expect(resolveValueColor(rules, 25)).toBe('#ffaa00');
    expect(resolveValueColor(rules, 10)).toBeUndefined();
  });

  it('숫자로 읽히면 숫자로 비교한다 — 문자열 비교면 "9" > "10" 이 된다', () => {
    const rules = [{ op: 'gt' as const, value: '10', color: '#f00' }];
    expect(resolveValueColor(rules, '9')).toBeUndefined();
    expect(resolveValueColor(rules, '11')).toBe('#f00');
  });

  it('문자열 값은 같음/다름만 판정한다', () => {
    expect(resolveValueColor([{ op: 'eq', value: 'error', color: '#f00' }], 'error')).toBe('#f00');
    expect(resolveValueColor([{ op: 'ne', value: 'ok', color: '#f00' }], 'error')).toBe('#f00');
    // 사전순 대소 비교는 의도인 경우가 드물어 맞지 않음으로 둔다.
    expect(resolveValueColor([{ op: 'gt', value: 'a', color: '#f00' }], 'b')).toBeUndefined();
  });

  it('색이 비어 있는 규칙은 건너뛴다', () => {
    const rules = [
      { op: 'gte' as const, value: '0', color: '' },
      { op: 'gte' as const, value: '0', color: '#0f0' },
    ];
    expect(resolveValueColor(rules, 1)).toBe('#0f0');
  });

  it('규칙이 없으면 색을 정하지 않는다', () => {
    expect(resolveValueColor(undefined, 1)).toBeUndefined();
    expect(resolveValueColor([], 1)).toBeUndefined();
  });
});

describe('항목별 세부 설정', () => {
  const base = readPropertiesGridStyle({
    label_font: { size: 10 },
    value_font: { size: 14, color: '#111111' },
    time_font: { size: 9 },
  });

  it('지정하지 않으면 카드 전체 설정을 그대로 쓴다', () => {
    const out = applyPropertyOverride(base, {}, 1);
    expect(out.labelStyle).toEqual(base.labelStyle);
    expect(out.valueStyle).toEqual(base.valueStyle);
    expect(out.timeStyle).toEqual(base.timeStyle);
  });

  it('항목별 글자 설정이 카드 전체 설정을 이긴다', () => {
    const out = applyPropertyOverride(base, { value_font: { size: 28 } }, 1);
    expect(out.valueStyle).toMatchObject({ fontSize: '28px' });
    // 덮지 않은 조각은 그대로다.
    expect(out.labelStyle).toEqual(base.labelStyle);
  });

  it('값 색 규칙이 고정 색을 덮는다 — "값에 따라 변한다"가 이 기능의 요지다', () => {
    const out = applyPropertyOverride(
      base,
      { valueColors: [{ op: 'gte', value: '30', color: '#ff0000' }] },
      35,
    );
    expect(out.valueStyle).toMatchObject({ color: '#ff0000' });
  });

  it('규칙에 맞지 않으면 원래 색이 남는다', () => {
    const out = applyPropertyOverride(
      base,
      { valueColors: [{ op: 'gte', value: '30', color: '#ff0000' }] },
      10,
    );
    expect(out.valueStyle).toMatchObject({ color: '#111111' });
  });
});

describe('readPropertyOverride', () => {
  it('없는 항목은 빈 설정', () => {
    expect(readPropertyOverride(undefined, 'temperature')).toEqual({});
    expect(readPropertyOverride({}, 'temperature')).toEqual({});
  });

  it('항목 키로 찾는다', () => {
    const config = { propertyOverrides: { temperature: { value_font: { size: 20 } } } };
    expect(readPropertyOverride(config, 'temperature')).toEqual({ value_font: { size: 20 } });
    expect(readPropertyOverride(config, 'humidity')).toEqual({});
  });
});

describe('카드 배치 끌기', () => {
  const grid = { rows: 3, cols: 3 };

  it('칸 단위로 옮기고 크기는 그대로 둔다', () => {
    const area = { row: 1, col: 1, rowSpan: 1, colSpan: 2 };
    expect(moveArea(area, grid, 1, 1)).toEqual({ row: 2, col: 2, rowSpan: 1, colSpan: 2 });
  });

  it('격자 끝에 부딪히면 멈춘다 — 밖으로 나가거나 줄어들지 않는다', () => {
    const area = { row: 1, col: 2, rowSpan: 1, colSpan: 2 };
    // 오른쪽으로 아무리 끌어도 2칸짜리는 3열 격자에서 2열이 끝이다.
    expect(moveArea(area, grid, 0, 5)).toEqual({ row: 1, col: 2, rowSpan: 1, colSpan: 2 });
    expect(moveArea(area, grid, -5, -5)).toEqual({ row: 1, col: 1, rowSpan: 1, colSpan: 2 });
  });

  it('모서리를 끌면 시작 위치는 그대로 두고 칸 수만 바뀐다', () => {
    const area = { row: 1, col: 1, rowSpan: 1, colSpan: 1 };
    expect(resizeArea(area, grid, 1, 2)).toEqual({ row: 1, col: 1, rowSpan: 2, colSpan: 3 });
  });

  it('칸 수는 시작 위치에서 격자 끝까지로 묶인다', () => {
    const area = { row: 2, col: 2, rowSpan: 1, colSpan: 1 };
    expect(resizeArea(area, grid, 9, 9)).toEqual({ row: 2, col: 2, rowSpan: 2, colSpan: 2 });
    expect(resizeArea(area, grid, -9, -9)).toEqual({ row: 2, col: 2, rowSpan: 1, colSpan: 1 });
  });

  it('1x1 격자에서도 무너지지 않는다', () => {
    const one = { rows: 1, cols: 1 };
    const area = { row: 1, col: 1, rowSpan: 1, colSpan: 1 };
    expect(moveArea(area, one, 3, 3)).toEqual(area);
    expect(resizeArea(area, one, 3, 3)).toEqual(area);
  });
});

describe('항목별 카드 배치', () => {
  const base = readPropertiesGridStyle({ cardRows: 3, cardCols: 3 });

  it('따로 잡지 않으면 카드 전체 배치를 그대로 쓴다', () => {
    const out = resolveCardLayout(base, {});
    expect(out.grid).toEqual(base.cardGrid);
    expect(out.areas).toBe(base.areas);
  });

  it('항목별 배치가 있으면 그것을 쓴다', () => {
    const own = { label: { row: 1, col: 1, rowSpan: 1, colSpan: 2 } };
    const out = resolveCardLayout(base, { cardAreas: own });
    expect(out.areas.label).toMatchObject({ row: 1, col: 1, colSpan: 2 });
  });

  it('분할만 바꾸면 배치는 전체 설정에서 물려받아 그 격자에 가둔다', () => {
    const wide = readPropertiesGridStyle({
      cardRows: 3,
      cardCols: 3,
      cardAreas: { value: { row: 3, col: 3, rowSpan: 1, colSpan: 1 } },
    });
    const out = resolveCardLayout(wide, { cardRows: 2, cardCols: 2 });
    expect(out.grid).toEqual({ rows: 2, cols: 2 });
    // 3행 3열은 2x2 격자에 없다 — 밖으로 나가지 않고 안으로 들어와야 한다.
    expect(out.areas.value.row).toBeLessThanOrEqual(2);
    expect(out.areas.value.col).toBeLessThanOrEqual(2);
  });

  it('지정하지 않은 축은 전체 설정을 따른다', () => {
    const out = resolveCardLayout(base, { cardRows: 1 });
    expect(out.grid).toEqual({ rows: 1, cols: base.cardGrid.cols });
  });
});

describe('reorderVisible', () => {
  const shown = ['a', 'b', 'c'];

  it('고른 목록 안에서 대상 앞으로 옮긴다', () => {
    expect(reorderVisible(['a', 'b', 'c'], shown, 'c', 'a')).toEqual(['c', 'a', 'b']);
  });

  it('목록이 비어 있으면 지금 보이는 차례를 굳힌 뒤 옮긴다', () => {
    expect(reorderVisible([], shown, 'c', 'a')).toEqual(['c', 'a', 'b']);
  });

  it('제자리에 놓으면 아무것도 바뀌지 않는다', () => {
    const visible = ['a', 'b', 'c'];
    expect(reorderVisible(visible, shown, 'b', 'b')).toBe(visible);
  });

  it('목록에 없는 항목은 건드리지 않는다', () => {
    const visible = ['a', 'b'];
    expect(reorderVisible(visible, shown, 'c', 'a')).toBe(visible);
  });
});

describe('갱신 시간 제한', () => {
  const now = 1_000_000;

  it('제한을 넘기면 오래된 값이다', () => {
    expect(isStaleValue(now - 301_000, 300, now)).toBe(true);
  });

  it('제한 안이면 아니다 — 경계에서는 아직 아니다', () => {
    expect(isStaleValue(now - 299_000, 300, now)).toBe(false);
    expect(isStaleValue(now - 300_000, 300, now)).toBe(false);
  });

  it('제한을 정하지 않았으면 표시하지 않는다 — 지금까지의 화면이 그대로 산다', () => {
    expect(isStaleValue(now - 999_999_000, undefined, now)).toBe(false);
  });

  it('0 이나 음수 제한은 미설정으로 본다 — 그대로 쓰면 방금 온 값도 오래된 것이 된다', () => {
    expect(isStaleValue(now - 1000, 0, now)).toBe(false);
    expect(isStaleValue(now - 1000, -5, now)).toBe(false);
  });

  it('갱신 시각을 모르면 단정하지 않는다 — 자리표시자까지 흐려진다', () => {
    expect(isStaleValue(undefined, 300, now)).toBe(false);
    expect(isStaleValue(Number.NaN, 300, now)).toBe(false);
  });
})

describe('값이 없는 항목의 자리', () => {
  type Entry = { id: string; key: string; value: number | undefined };
  const make = (key: string): Entry => ({ id: key, key, value: undefined });

  it('고른 항목에 값이 없으면 빈 카드를 만든다 — 첫 통신 전 고정 디바이스가 이 경우다', () => {
    const got = selectEntries<Entry>([], ['temperature', 'humidity'], make);
    expect(got.map((e) => e.key)).toEqual(['temperature', 'humidity']);
    expect(got[0]!.value).toBeUndefined();
  });

  it('있는 값은 그대로 쓰고 없는 것만 채운다', () => {
    const entries: Entry[] = [{ id: 'humidity', key: 'humidity', value: 55 }];
    const got = selectEntries(entries, ['temperature', 'humidity'], make);
    expect(got.map((e) => [e.key, e.value])).toEqual([
      ['temperature', undefined],
      ['humidity', 55],
    ]);
  });

  it('고른 순서를 지킨다', () => {
    const entries: Entry[] = [
      { id: 'a', key: 'a', value: 1 },
      { id: 'b', key: 'b', value: 2 },
    ];
    expect(selectEntries(entries, ['b', 'c', 'a'], make).map((e) => e.key)).toEqual(['b', 'c', 'a']);
  });

  it('만드는 함수를 주지 않으면 종전처럼 빠진다', () => {
    expect(selectEntries([], ['temperature'])).toEqual([]);
  });

  it('아무것도 고르지 않았으면 없는 항목을 지어내지 않는다', () => {
    expect(selectEntries<Entry>([], [], make)).toEqual([]);
  });
})
