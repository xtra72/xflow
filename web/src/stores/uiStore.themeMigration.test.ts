// v5 -> v6 테마 마이그레이션 검증.
//
// v6 에서 'custom' 은 별도 모드가 아니게 되었다. 이 경로가 틀리면 커스텀 팔레트를
// 쓰던 사용자의 색이 통째로 사라지거나, 반대로 쓰지도 않던 옛 순백 팔레트가
// 되살아나 v0.7.0 에서 낮춘 라이트 밝기를 되돌려 버린다.

import { describe, expect, it } from 'vitest';

import { useUIStore } from './uiStore';
import { DAY_PRESET } from '@/lib/theme/tokens';

/** persist 옵션에 등록된 migrate 를 직접 호출한다. */
function migrate(state: Record<string, unknown>, version: number): Record<string, unknown> {
  const fn = useUIStore.persist.getOptions().migrate;
  if (!fn) throw new Error('migrate 가 등록되어 있지 않다');
  return fn(state, version) as unknown as Record<string, unknown>;
}

describe('uiStore v5 -> v6 테마 마이그레이션', () => {
  it("custom 모드 사용자는 day 모드 + day 오버라이드로 옮겨진다", () => {
    const result = migrate(
      { theme: 'custom', customThemeTokens: { '--color-bg-primary': '#123456' } },
      5,
    );

    expect(result.theme).toBe('day');
    expect(result.themeOverrides).toEqual({ day: { '--color-bg-primary': '#123456' }, night: {} });
    expect(result.customThemeTokens).toBeUndefined();
  });

  it('custom 모드가 아니었다면 쓰이지 않던 옛 팔레트는 버린다', () => {
    // 에디터를 열어 저장만 해두고 day 모드로 돌아간 사용자의 상태.
    // 이걸 되살리면 옛 순백 라이트 팔레트가 그대로 고정된다.
    const result = migrate(
      { theme: 'day', customThemeTokens: { '--color-bg-surface': '#ffffff' } },
      5,
    );

    expect(result.theme).toBe('day');
    expect(result.themeOverrides).toEqual({ day: {}, night: {} });
    expect(result.customThemeTokens).toBeUndefined();
  });

  it('custom 팔레트 중 현재 기본값과 같은 색은 오버라이드로 남기지 않는다', () => {
    const result = migrate(
      {
        theme: 'custom',
        customThemeTokens: {
          '--color-bg-primary': DAY_PRESET['--color-bg-primary']!,
          '--color-bg-surface': '#eeeeee',
        },
      },
      5,
    );

    expect(result.themeOverrides).toEqual({ day: { '--color-bg-surface': '#eeeeee' }, night: {} });
  });

  it('customThemeTokens 가 아예 없어도 안전하게 빈 오버라이드를 만든다', () => {
    expect(migrate({ theme: 'night' }, 5).themeOverrides).toEqual({ day: {}, night: {} });
  });

  it('v0 의 light/dark 표기도 최종적으로 day/night + 오버라이드 형태가 된다', () => {
    const result = migrate({ theme: 'light' }, 0);
    expect(result.theme).toBe('day');
    expect(result.themeOverrides).toEqual({ day: {}, night: {} });
  });

  it('이미 v6 인 상태는 손대지 않는다', () => {
    const existing = { day: { '--color-bg-primary': '#abcdef' }, night: {} };
    const result = migrate({ theme: 'day', themeOverrides: existing }, 6);
    expect(result.themeOverrides).toBe(existing);
  });
});
