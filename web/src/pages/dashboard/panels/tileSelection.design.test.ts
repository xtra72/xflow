// 타일 디자인 — 타이틀/값 분리와 값에 따른 색.

import { describe, it, expect } from 'vitest';

import { readTileDesign, resolveTileDesign } from './tileSelection';

describe('readTileDesign', () => {
  it('없으면 빈 설정', () => {
    expect(readTileDesign(undefined, 'errors')).toEqual({ value_font: undefined, bg: undefined });
  });

  it('타이틀·값을 나눠 읽는다', () => {
    const styles = { errors: { label_font: { size: 10 }, value_font: { size: 30 } } };
    expect(readTileDesign(styles, 'errors')).toMatchObject({
      label_font: { size: 10 },
      value_font: { size: 30 },
    });
  });

  // 이 기능이 처음 나왔을 때는 타일에 글자 설정이 하나뿐이었고 그것이 값에 걸렸다.
  it('옛 형태(최상위 글꼴)는 값 글자로 읽는다 — 저장된 설정이 사라지면 안 된다', () => {
    const got = readTileDesign({ errors: { size: 28, color: '#ff0000' } }, 'errors');
    expect(got.value_font).toMatchObject({ size: 28, color: '#ff0000' });
    expect(got.label_font).toBeUndefined();
  });

  it('옛 형태의 배경색도 이어받는다', () => {
    expect(readTileDesign({ errors: { bg: '#111111' } }, 'errors').bg).toBe('#111111');
  });
});

describe('resolveTileDesign', () => {
  it('타이틀과 값에 각자의 스타일을 얹는다', () => {
    const got = resolveTileDesign({ label_font: { size: 10 }, value_font: { size: 30 } }, 0);
    expect(got.labelStyle).toMatchObject({ fontSize: '10px' });
    expect(got.valueStyle).toMatchObject({ fontSize: '30px' });
  });

  it('값 색 규칙이 맞으면 값 글자색을 덮는다', () => {
    const got = resolveTileDesign(
      { value_font: { color: '#111111' }, valueColors: [{ op: 'gte', value: '10', color: '#ff0000' }] },
      15,
    );
    expect(got.valueStyle).toMatchObject({ color: '#ff0000' });
  });

  it('규칙에 맞지 않으면 원래 색이 남는다', () => {
    const got = resolveTileDesign(
      { value_font: { color: '#111111' }, valueColors: [{ op: 'gte', value: '10', color: '#ff0000' }] },
      1,
    );
    expect(got.valueStyle).toMatchObject({ color: '#111111' });
  });

  it('규칙은 타이틀에 걸리지 않는다 — 값에 따라 변하는 것은 값이다', () => {
    const got = resolveTileDesign({ valueColors: [{ op: 'gte', value: '0', color: '#ff0000' }] }, 1);
    expect(got.labelStyle).toBeUndefined();
  });

  it('배경색을 정하면 상자에 얹고, 기본 색을 걷어낼 신호를 준다', () => {
    const got = resolveTileDesign({ bg: '#331111' }, 0);
    expect(got.box).toMatchObject({ backgroundColor: '#331111' });
    expect(got.hasOwnBackground).toBe(true);
  });

  it('형식이 아닌 배경색은 무시한다', () => {
    expect(resolveTileDesign({ bg: 'red' }, 0).hasOwnBackground).toBe(false);
  });

  it('아무것도 정하지 않으면 어느 것도 얹지 않는다 — 기존 모양이 그대로 산다', () => {
    const got = resolveTileDesign({}, 0);
    expect(got.box).toBeUndefined();
    expect(got.labelStyle).toBeUndefined();
    expect(got.valueStyle).toBeUndefined();
  });
});
