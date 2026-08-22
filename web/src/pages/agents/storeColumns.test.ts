// storeColumns — 컬럼 레지스트리 관련성/가시성 계산 + localStorage 헬퍼 단위 테스트.
//
// UI 컴포넌트(ColumnHeader 등)는 렌더 테스트 범위이며, 여기서는 순수 로직만 검증한다.

import { beforeEach, describe, expect, it } from 'vitest';

import {
  defaultVisibleColumns,
  loadVisibleColumns,
  relevantColumns,
  renderedColumns,
  saveVisibleColumns,
  storeColumnsStorageKey,
  STORE_COLUMNS,
  type StoreColumnId,
} from './storeColumnsModel';

describe('STORE_COLUMNS 레지스트리', () => {
  it('논리적 기본 순서를 정의한다', () => {
    expect(STORE_COLUMNS.map((c) => c.id)).toEqual([
      'key',
      'field',
      'value',
      'namespace',
      'tags',
      'binding',
      'ttl',
      'history',
      'updated',
      'actions',
    ]);
  });

  it('actions 만 숨길 수 없고 나머지는 숨길 수 있다', () => {
    const actions = STORE_COLUMNS.find((c) => c.id === 'actions');
    expect(actions?.hideable).toBe(false);
    for (const c of STORE_COLUMNS) {
      if (c.id !== 'actions') expect(c.hideable).toBe(true);
    }
  });
});

describe('relevantColumns', () => {
  it('태그 없음 + 히스토리 없음이면 tags/history 컬럼을 제외한다', () => {
    const ids = relevantColumns({ showTagsColumn: false, hasHistory: false }).map(
      (c) => c.id,
    );
    expect(ids).not.toContain('tags');
    expect(ids).not.toContain('history');
    expect(ids).toContain('actions');
  });

  it('태그/히스토리가 있으면 해당 컬럼을 포함한다', () => {
    const ids = relevantColumns({ showTagsColumn: true, hasHistory: true }).map(
      (c) => c.id,
    );
    expect(ids).toContain('tags');
    expect(ids).toContain('history');
  });
});

describe('renderedColumns', () => {
  it('actions 는 visible 집합과 무관하게 항상 렌더된다', () => {
    const ids = renderedColumns(
      { showTagsColumn: true, hasHistory: true },
      new Set<StoreColumnId>(),
    ).map((c) => c.id);
    expect(ids).toEqual(['actions']);
  });

  it('가시 집합에 포함된 컬럼만 렌더하고 순서를 보존한다', () => {
    const ids = renderedColumns(
      { showTagsColumn: true, hasHistory: false },
      new Set<StoreColumnId>(['key', 'value']),
    ).map((c) => c.id);
    // 기본 순서상 key → value → actions.
    expect(ids).toEqual(['key', 'value', 'actions']);
  });

  it('관련 없는(비표시) 조건부 컬럼은 가시 집합에 있어도 렌더되지 않는다', () => {
    const ids = renderedColumns(
      { showTagsColumn: false, hasHistory: false },
      new Set<StoreColumnId>(['tags', 'history', 'key']),
    ).map((c) => c.id);
    expect(ids).toEqual(['key', 'actions']);
  });
});

describe('defaultVisibleColumns', () => {
  it('숨김 가능한 모든 컬럼을 포함하고 actions 는 제외한다', () => {
    const set = defaultVisibleColumns();
    expect(set.has('actions')).toBe(false);
    expect(set.has('field')).toBe(true);
    expect(set.has('key')).toBe(true);
    expect(set.size).toBe(STORE_COLUMNS.filter((c) => c.hideable).length);
  });
});

describe('localStorage 헬퍼', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('저장값이 없으면 기본(전체 표시)을 반환한다', () => {
    const set = loadVisibleColumns('agent-1');
    expect(set).toEqual(defaultVisibleColumns());
  });

  it('키 포맷은 xflow.store.columns.<agentId> 이다', () => {
    expect(storeColumnsStorageKey('agent-7')).toBe(
      'xflow.store.columns.agent-7',
    );
  });

  it('저장 후 로드하면 동일한 가시 집합을 반환한다', () => {
    const visible = new Set<StoreColumnId>(['key', 'value']);
    saveVisibleColumns('agent-2', visible);
    expect(loadVisibleColumns('agent-2')).toEqual(visible);
  });

  it('per-agent 로 분리 저장된다', () => {
    saveVisibleColumns('agent-a', new Set<StoreColumnId>(['key']));
    saveVisibleColumns('agent-b', new Set<StoreColumnId>(['value']));
    expect(loadVisibleColumns('agent-a')).toEqual(
      new Set<StoreColumnId>(['key']),
    );
    expect(loadVisibleColumns('agent-b')).toEqual(
      new Set<StoreColumnId>(['value']),
    );
  });

  it('무효/미지정 id 와 actions 는 로드 시 무시된다', () => {
    window.localStorage.setItem(
      storeColumnsStorageKey('agent-3'),
      JSON.stringify(['key', 'bogus', 'actions', 42]),
    );
    expect(loadVisibleColumns('agent-3')).toEqual(
      new Set<StoreColumnId>(['key']),
    );
  });

  it('손상된 JSON 은 기본값으로 폴백한다', () => {
    window.localStorage.setItem(storeColumnsStorageKey('agent-4'), '{not json');
    expect(loadVisibleColumns('agent-4')).toEqual(defaultVisibleColumns());
  });

  it('배열이 아닌 저장값은 기본값으로 폴백한다', () => {
    window.localStorage.setItem(
      storeColumnsStorageKey('agent-5'),
      JSON.stringify({ name: true }),
    );
    expect(loadVisibleColumns('agent-5')).toEqual(defaultVisibleColumns());
  });

  it('빈 배열 저장은 모든 컬럼 숨김(빈 집합)으로 로드된다', () => {
    saveVisibleColumns('agent-6', new Set<StoreColumnId>());
    expect(loadVisibleColumns('agent-6')).toEqual(new Set<StoreColumnId>());
  });
});
