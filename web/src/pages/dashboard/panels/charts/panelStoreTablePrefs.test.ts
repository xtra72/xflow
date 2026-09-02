// panelStoreTablePrefs 순수 영속 헬퍼 테스트.
// @spec SPEC-PANEL-SETTINGS-001 (T7, AC-08)

import { beforeEach, describe, expect, it } from 'vitest';

import {
  defaultPanelStoreTablePrefs,
  loadPanelStoreTablePrefs,
  panelStoreTableStorageKey,
  savePanelStoreTablePrefs,
} from './panelStoreTablePrefs';

beforeEach(() => window.localStorage.clear());

describe('panelStoreTablePrefs — 키/기본값', () => {
  it('패널별 저장 키를 구성한다', () => {
    expect(panelStoreTableStorageKey('p1')).toBe('panel-settings.storeTable.p1');
  });
  it('기본값은 정렬 없음/필터 없음/숨김 없음', () => {
    expect(defaultPanelStoreTablePrefs()).toEqual({
      sort: { column: null, direction: 'asc' },
      filters: {},
      hidden: [],
    });
  });
});

describe('panelStoreTablePrefs — load 방어', () => {
  it('부재 시 기본값', () => {
    expect(loadPanelStoreTablePrefs('none')).toEqual(defaultPanelStoreTablePrefs());
  });
  it('손상(JSON 파싱 실패) 시 기본값 (throw 없음)', () => {
    window.localStorage.setItem(panelStoreTableStorageKey('p'), '{bad json');
    expect(loadPanelStoreTablePrefs('p')).toEqual(defaultPanelStoreTablePrefs());
  });
  it('무효 sort column 은 null 로, 방향 기본은 asc', () => {
    window.localStorage.setItem(
      panelStoreTableStorageKey('p'),
      JSON.stringify({ sort: { column: 'bogus', direction: 'sideways' } }),
    );
    expect(loadPanelStoreTablePrefs('p').sort).toEqual({ column: null, direction: 'asc' });
  });
});

describe('panelStoreTablePrefs — save/load 왕복', () => {
  it('sort/filters(Set)/hidden 을 왕복 보존한다', () => {
    savePanelStoreTablePrefs('p1', {
      sort: { column: 'field', direction: 'desc' },
      filters: { key: { text: 'abc', values: new Set(['x', 'y']) } },
      hidden: ['field'],
    });
    const loaded = loadPanelStoreTablePrefs('p1');
    expect(loaded.sort).toEqual({ column: 'field', direction: 'desc' });
    expect(loaded.hidden).toEqual(['field']);
    expect(loaded.filters.key?.text).toBe('abc');
    expect(loaded.filters.key?.values).toBeInstanceOf(Set);
    expect(Array.from(loaded.filters.key!.values).sort()).toEqual(['x', 'y']);
  });

  it('빈 필터(text 없음 + values 없음)는 저장/복원 시 제거된다', () => {
    savePanelStoreTablePrefs('p2', {
      sort: { column: null, direction: 'asc' },
      filters: { key: { text: '', values: new Set() } },
      hidden: [],
    });
    expect(loadPanelStoreTablePrefs('p2').filters).toEqual({});
  });
});
