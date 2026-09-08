// 공통 타일 설정 위에 타일별 설정 덮기.

import { describe, it, expect } from 'vitest';

import { mergeTileDesign } from './tileSelection';

describe('mergeTileDesign', () => {
  it('글꼴은 항목 단위로 덮는다 — 통째로 갈아치우면 공통 설정이 사라진다', () => {
    const got = mergeTileDesign(
      { value_font: { size: 26, color: '#00ff00' } },
      { value_font: { size: 40 } },
    );
    expect(got.value_font).toEqual({ size: 40, color: '#00ff00' });
  });

  it('타일별에 없으면 공통이 그대로 산다', () => {
    const got = mergeTileDesign({ label_font: { size: 9 }, bg: '#112233' }, {});
    expect(got.label_font).toMatchObject({ size: 9 });
    expect(got.bg).toBe('#112233');
  });

  it('타일별 배경색이 공통을 이긴다', () => {
    expect(mergeTileDesign({ bg: '#112233' }, { bg: '#445566' }).bg).toBe('#445566');
  });

  it('값 색 규칙은 통째로 갈아친다 — 규칙 목록을 섞으면 우선순위를 알 수 없다', () => {
    const got = mergeTileDesign(
      { valueColors: [{ op: 'gte', value: '1', color: '#111111' }] },
      { valueColors: [{ op: 'gte', value: '2', color: '#222222' }] },
    );
    expect(got.valueColors).toEqual([{ op: 'gte', value: '2', color: '#222222' }]);
  });

  it('타일별에 규칙이 없으면 공통 규칙을 쓴다', () => {
    const rules = [{ op: 'gte' as const, value: '1', color: '#111111' }];
    expect(mergeTileDesign({ valueColors: rules }, {}).valueColors).toBe(rules);
  });

  it('둘 다 비면 아무것도 정하지 않은 것과 같다', () => {
    expect(mergeTileDesign({}, {})).toEqual({
      label_font: undefined,
      value_font: undefined,
      valueColors: undefined,
      bg: undefined,
    });
  });
});
