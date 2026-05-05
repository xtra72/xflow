import { describe, expect, it } from 'vitest';
import {
  resolveControlButtonColor,
  resolveFanLevelColor,
  resolveValueColor,
} from './acControlColors';

describe('resolveValueColor', () => {
  it('returns undefined when config is undefined', () => {
    expect(resolveValueColor(20, undefined)).toBeUndefined();
  });

  it('returns default when no ranges match', () => {
    expect(resolveValueColor(20, { default: '#000' })).toBe('#000');
  });

  it('returns default when value is undefined', () => {
    expect(
      resolveValueColor(undefined, {
        default: '#000',
        ranges: [{ min: 0, max: 10, color: '#fff' }],
      }),
    ).toBe('#000');
  });

  it('matches inclusive lower bound', () => {
    expect(
      resolveValueColor(20, { ranges: [{ min: 20, max: 30, color: '#aaa' }] }),
    ).toBe('#aaa');
  });

  it('matches exclusive upper bound', () => {
    expect(
      resolveValueColor(30, { ranges: [{ min: 20, max: 30, color: '#aaa' }] }),
    ).toBeUndefined();
  });

  it('handles open-ended lower bound (undefined min)', () => {
    expect(
      resolveValueColor(-100, { ranges: [{ max: 0, color: '#blue' }] }),
    ).toBe('#blue');
  });

  it('handles open-ended upper bound (undefined max)', () => {
    expect(
      resolveValueColor(100, { ranges: [{ min: 50, color: '#red' }] }),
    ).toBe('#red');
  });

  it('uses first matching range when multiple match', () => {
    expect(
      resolveValueColor(25, {
        ranges: [
          { min: 20, max: 30, color: '#first' },
          { min: 25, max: 35, color: '#second' },
        ],
      }),
    ).toBe('#first');
  });

  it('skips invalid range entries gracefully', () => {
    expect(
      resolveValueColor(20, {
        default: '#fallback',
        // @ts-expect-error invalid entry intentionally
        ranges: [null, { min: 0, max: 10, color: '#a' }],
      }),
    ).toBe('#fallback');
  });
});

describe('resolveControlButtonColor', () => {
  it('returns active=false when target differs from current mode', () => {
    expect(resolveControlButtonColor('cool', 'heat', undefined)).toEqual({
      active: false,
      color: undefined,
    });
  });

  it('returns active=true with no color when config is empty', () => {
    expect(resolveControlButtonColor('cool', 'cool', undefined)).toEqual({
      active: true,
      color: undefined,
    });
  });

  it('returns unselected color for non-active button', () => {
    expect(
      resolveControlButtonColor('cool', 'heat', { unselected: '#ccc' }),
    ).toEqual({ active: false, color: '#ccc' });
  });

  it('returns selectedColor for active button in unified mode', () => {
    expect(
      resolveControlButtonColor('cool', 'cool', {
        selectedMode: 'unified',
        selectedColor: '#3b82f6',
      }),
    ).toEqual({ active: true, color: '#3b82f6' });
  });

  it('returns perButton color in individual mode', () => {
    expect(
      resolveControlButtonColor('heat', 'heat', {
        selectedMode: 'individual',
        perButton: { heat: '#f97316' },
      }),
    ).toEqual({ active: true, color: '#f97316' });
  });

  it('falls back to selectedColor when individual mode lacks per-button entry', () => {
    expect(
      resolveControlButtonColor('dry', 'dry', {
        selectedMode: 'individual',
        perButton: { cool: '#abc' },
        selectedColor: '#fallback',
      }),
    ).toEqual({ active: true, color: '#fallback' });
  });
});

describe('resolveFanLevelColor', () => {
  it('returns active=true with perLevel color', () => {
    expect(
      resolveFanLevelColor('high', 'high', { perLevel: { high: '#1d4ed8' } }),
    ).toEqual({ active: true, color: '#1d4ed8' });
  });

  it('returns unselected color for inactive level', () => {
    expect(
      resolveFanLevelColor('high', 'low', { unselected: '#eee' }),
    ).toEqual({ active: false, color: '#eee' });
  });

  it('returns active=true with undefined color when level missing in perLevel', () => {
    expect(resolveFanLevelColor('auto', 'auto', { perLevel: {} })).toEqual({
      active: true,
      color: undefined,
    });
  });
});
