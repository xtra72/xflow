// CSV 변환 유틸 테스트.

import { describe, it, expect } from 'vitest';

import { chartDataToCsv } from './csvExport';

describe('chartDataToCsv', () => {
  it('단일 시리즈: 헤더 + 행', () => {
    const rows = [
      { timestamp: 1000, value: 10 },
      { timestamp: 2000, value: 20 },
    ];
    const csv = chartDataToCsv(rows, ['value']);
    const lines = csv.trim().split('\n');
    expect(lines[0]).toBe('timestamp,iso,value');
    expect(lines[1]).toMatch(/^1000,1970-01-01T00:00:01\.000Z,10$/);
    expect(lines[2]).toMatch(/^2000,1970-01-01T00:00:02\.000Z,20$/);
  });

  it('다중 시리즈: 시리즈 키 순서대로 컬럼 생성', () => {
    const rows = [
      { timestamp: 1000, A: 10, B: 100 },
      { timestamp: 2000, A: 20, B: 200 },
    ];
    const csv = chartDataToCsv(rows, ['A', 'B']);
    const lines = csv.trim().split('\n');
    expect(lines[0]).toBe('timestamp,iso,A,B');
    expect(lines[1]).toContain(',10,100');
    expect(lines[2]).toContain(',20,200');
  });

  it('누락 값은 빈 문자열', () => {
    const rows = [
      { timestamp: 1000, A: 10 },
      { timestamp: 2000, B: 20 },
    ];
    const csv = chartDataToCsv(rows, ['A', 'B']);
    const lines = csv.trim().split('\n');
    expect(lines[1]).toMatch(/,10,$/);
    expect(lines[2]).toMatch(/,,20$/);
  });

  it('컴마 또는 따옴표 포함된 키는 따옴표로 감싸고 이스케이프', () => {
    const rows = [{ timestamp: 1000, 'a,b': 1, 'q"x': 2 }];
    const csv = chartDataToCsv(rows, ['a,b', 'q"x']);
    const lines = csv.trim().split('\n');
    expect(lines[0]).toBe('timestamp,iso,"a,b","q""x"');
  });

  it('NaN/Infinity 는 빈 문자열', () => {
    const rows = [{ timestamp: 1000, value: NaN }, { timestamp: 2000, value: Infinity }];
    const csv = chartDataToCsv(rows, ['value']);
    const lines = csv.trim().split('\n');
    expect(lines[1]).toMatch(/,$/);
    expect(lines[2]).toMatch(/,$/);
  });

  it('빈 rows: 헤더만', () => {
    const csv = chartDataToCsv([], ['value']);
    expect(csv.trim()).toBe('timestamp,iso,value');
  });
});
