// 그룹 나누기와 그룹 안 차례 바꾸기.

import { describe, it, expect } from 'vitest';

import { groupEntries, moveWithinGroup, setGroupSelection } from './propertiesGridStyle';

const E = (key: string) => ({ key });

describe('groupEntries', () => {
  it('셋으로 나누고 그룹 순서를 지킨다', () => {
    const got = groupEntries([E('temperature'), E('meta.name'), E('gw.rssi')]);
    expect(got.map((g) => g.group)).toEqual(['basic', 'status', 'gateway']);
  });

  it('그룹 안에서는 고른 차례를 지킨다', () => {
    const got = groupEntries([E('humidity'), E('temperature')]);
    expect(got[0]!.entries.map((e) => e.key)).toEqual(['humidity', 'temperature']);
  });

  it('빈 그룹은 내지 않는다', () => {
    expect(groupEntries([E('temperature')]).map((g) => g.group)).toEqual(['status']);
    expect(groupEntries([])).toEqual([]);
  });
});

describe('moveWithinGroup', () => {
  // 표시 목록은 그룹과 무관한 한 줄이라, 그룹 안 이웃을 건너뛰어 찾아야 한다.
  const list = ['meta.name', 'temperature', 'meta.location', 'humidity'];

  it('같은 그룹의 이웃과 자리를 바꾼다 — 사이에 낀 다른 그룹은 건너뛴다', () => {
    expect(moveWithinGroup(list, 'meta.location', -1)).toEqual([
      'meta.location',
      'temperature',
      'meta.name',
      'humidity',
    ]);
  });

  it('아래로도 같은 규칙', () => {
    expect(moveWithinGroup(list, 'temperature', 1)).toEqual([
      'meta.name',
      'humidity',
      'meta.location',
      'temperature',
    ]);
  });

  it('그룹의 끝이면 그대로 둔다', () => {
    expect(moveWithinGroup(list, 'meta.name', -1)).toBe(list);
    expect(moveWithinGroup(list, 'humidity', 1)).toBe(list);
  });

  it('목록에 없는 항목은 건드리지 않는다', () => {
    expect(moveWithinGroup(list, 'pressure', -1)).toBe(list);
  });
});

describe('setGroupSelection', () => {
  const shown = ['temperature', 'meta.name'];

  it('그룹을 켜면 그 그룹 나머지를 뒤에 붙인다 — 고른 차례를 흔들지 않는다', () => {
    expect(setGroupSelection(shown, ['temperature', 'humidity'], true)).toEqual([
      'temperature',
      'meta.name',
      'humidity',
    ]);
  });

  it('그룹을 끄면 그 그룹만 걷어낸다 — 다른 그룹은 그대로', () => {
    expect(setGroupSelection(shown, ['temperature', 'humidity'], false)).toEqual(['meta.name']);
  });

  it('이미 다 켜져 있으면 그대로', () => {
    expect(setGroupSelection(shown, ['temperature'], true)).toEqual(shown);
  });

  it('없는 그룹을 꺼도 아무 일이 없다', () => {
    expect(setGroupSelection(shown, ['pressure'], false)).toEqual(shown);
  });
});
