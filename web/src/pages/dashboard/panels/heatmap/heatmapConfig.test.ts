// heatmapConfig 파서 단위 테스트 (SPEC-HEATMAP-PANEL-001 T1).
// 하위호환 기본값 분기(결측 좌표/bounds/color_table/기본 power/손상 입력)를 커버한다.

import { describe, it, expect } from 'vitest';

import {
  parseHeatmapConfig,
  buildDefaultHeatmapConfig,
  buildDefaultHeatmapStoreSource,
  DEFAULT_IDW_POWER,
  DEFAULT_GRID_RESOLUTION,
} from './heatmapConfig';

describe('parseHeatmapConfig', () => {
  it('결측 sensor_positions 는 빈 객체로 보정한다', () => {
    const cfg = parseHeatmapConfig({ store_source: buildDefaultHeatmapStoreSource() });
    expect(cfg.sensor_positions).toEqual({});
  });

  it('결측 value_bounds 는 undefined(자동)로 둔다', () => {
    const cfg = parseHeatmapConfig({});
    expect(cfg.value_bounds).toBeUndefined();
  });

  it('결측 color_table 은 undefined(기본 gradient 폴백)로 둔다', () => {
    const cfg = parseHeatmapConfig({});
    expect(cfg.color_table).toBeUndefined();
  });

  it('결측 idw 는 기본 power/grid_resolution 으로 채운다', () => {
    const cfg = parseHeatmapConfig({});
    expect(cfg.idw.power).toBe(DEFAULT_IDW_POWER);
    expect(cfg.idw.grid_resolution).toBe(DEFAULT_GRID_RESOLUTION);
  });

  it('idw.power 만 지정하면 grid_resolution 은 기본값으로 채운다', () => {
    const cfg = parseHeatmapConfig({ idw: { power: 3 } });
    expect(cfg.idw.power).toBe(3);
    expect(cfg.idw.grid_resolution).toBe(DEFAULT_GRID_RESOLUTION);
  });

  it('power<=0 / grid_resolution<=0 은 기본값으로 폴백한다', () => {
    const cfg = parseHeatmapConfig({ idw: { power: 0, grid_resolution: -5 } });
    expect(cfg.idw.power).toBe(DEFAULT_IDW_POWER);
    expect(cfg.idw.grid_resolution).toBe(DEFAULT_GRID_RESOLUTION);
  });

  it.each([null, undefined, 'not-an-object', 42, []])(
    '손상/비객체 입력(%p)도 예외 없이 기본값으로 파싱한다',
    (raw) => {
      expect(() => parseHeatmapConfig(raw as unknown)).not.toThrow();
      const cfg = parseHeatmapConfig(raw as unknown);
      expect(cfg.sensor_positions).toEqual({});
      expect(cfg.idw.power).toBe(DEFAULT_IDW_POWER);
      expect(cfg.store_source.selection_mode).toBe('tag');
    },
  );

  it('x/y 가 유한 숫자가 아닌 sensor_positions 항목은 걸러낸다', () => {
    const cfg = parseHeatmapConfig({
      sensor_positions: {
        ok: { x: 0.2, y: 0.8 },
        nanX: { x: NaN, y: 0.5 },
        missingY: { x: 0.5 },
        stringVal: { x: '0.1', y: 0.1 },
        infX: { x: Infinity, y: 0.1 },
      },
    });
    expect(cfg.sensor_positions).toEqual({ ok: { x: 0.2, y: 0.8 } });
  });

  it('유효한 value_bounds / color_table 은 그대로 통과시킨다', () => {
    const cfg = parseHeatmapConfig({
      value_bounds: { min: 18, max: 26 },
      color_table: [
        { stop: 0, color: '#0000ff' },
        { stop: 1, color: '#ff0000' },
      ],
    });
    expect(cfg.value_bounds).toEqual({ min: 18, max: 26 });
    expect(cfg.color_table).toEqual([
      { stop: 0, color: '#0000ff' },
      { stop: 1, color: '#ff0000' },
    ]);
  });

  it('무효 color_table(빈 배열/비정지점)은 undefined 로 폴백한다', () => {
    expect(parseHeatmapConfig({ color_table: [] }).color_table).toBeUndefined();
    expect(
      parseHeatmapConfig({ color_table: [{ stop: 'x', color: 5 }] }).color_table,
    ).toBeUndefined();
  });

  it('무효 value_bounds(min/max 비숫자)는 undefined 로 폴백한다', () => {
    expect(parseHeatmapConfig({ value_bounds: { min: 1 } }).value_bounds).toBeUndefined();
    expect(
      parseHeatmapConfig({ value_bounds: { min: 'a', max: 'b' } }).value_bounds,
    ).toBeUndefined();
  });
});

describe('buildDefaultHeatmapStoreSource / buildDefaultHeatmapConfig', () => {
  it('기본 store 소스는 tag 모드 + last 집계다', () => {
    const src = buildDefaultHeatmapStoreSource();
    expect(src.selection_mode).toBe('tag');
    expect(src.aggregation).toBe('last');
    expect(src.tag_filters).toEqual({});
    expect(src.series).toEqual([]);
  });

  it('기본 config 는 data_source=store + 기본 idw 를 포함한다', () => {
    const config = buildDefaultHeatmapConfig();
    expect(config.data_source).toBe('store');
    expect(config.sensor_positions).toEqual({});
    expect(config.idw).toEqual({
      power: DEFAULT_IDW_POWER,
      grid_resolution: DEFAULT_GRID_RESOLUTION,
    });
    // 기본 config 는 parseHeatmapConfig 를 통과해도 안정적이어야 한다(round-trip).
    const parsed = parseHeatmapConfig(config);
    expect(parsed.store_source.selection_mode).toBe('tag');
    expect(parsed.idw.power).toBe(DEFAULT_IDW_POWER);
  });
});
