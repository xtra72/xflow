// heatmapConfig 파서 단위 테스트 (SPEC-HEATMAP-PANEL-001 T1).
// 하위호환 기본값 분기(결측 좌표/bounds/color_table/기본 power/손상 입력)를 커버한다.

import { describe, it, expect } from 'vitest';

import {
  parseHeatmapConfig,
  buildDefaultHeatmapConfig,
  buildDefaultHeatmapStoreSource,
  DEFAULT_IDW_POWER,
  DEFAULT_GRID_RESOLUTION,
  DEFAULT_HEATMAP_OPACITY,
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

// SPEC-HEATMAP-PANEL-002 T1: floor_plan / heatmap_opacity / editor 신규 필드 파싱.
describe('parseHeatmapConfig — SPEC-002 additive fields', () => {
  it('AC-E5: MVP 시절 config(신규 필드 없음)는 기본값으로 채우고 기존 필드는 불변이다', () => {
    // floor_plan/heatmap_opacity/editor 가 전혀 없는 MVP config.
    const mvp = {
      data_source: 'store',
      store_source: buildDefaultHeatmapStoreSource(),
      sensor_positions: { s1: { x: 0.3, y: 0.7 } },
      idw: { power: 2, grid_resolution: 32 },
    };
    const cfg = parseHeatmapConfig(mvp);
    // 신규 필드 기본값(배경 없음 / opacity 0.6 / editor 없음).
    expect(cfg.floor_plan).toBeUndefined();
    expect(cfg.heatmap_opacity).toBe(DEFAULT_HEATMAP_OPACITY);
    expect(cfg.editor).toBeUndefined();
    // 기존 MVP 필드 의미 불변.
    expect(cfg.sensor_positions).toEqual({ s1: { x: 0.3, y: 0.7 } });
    expect(cfg.idw).toEqual({ power: 2, grid_resolution: 32 });
  });

  it('floor_plan.image 가 있으면 image + 기본 fit(contain)으로 파싱한다', () => {
    const cfg = parseHeatmapConfig({
      floor_plan: { image: 'data:image/png;base64,AAAA' },
    });
    expect(cfg.floor_plan).toEqual({ image: 'data:image/png;base64,AAAA', fit: 'contain' });
  });

  it('floor_plan.fit=cover 와 natural_width/height 를 통과시킨다', () => {
    const cfg = parseHeatmapConfig({
      floor_plan: {
        image: 'data:image/png;base64,BBBB',
        fit: 'cover',
        natural_width: 800,
        natural_height: 600,
      },
    });
    expect(cfg.floor_plan).toEqual({
      image: 'data:image/png;base64,BBBB',
      fit: 'cover',
      natural_width: 800,
      natural_height: 600,
    });
  });

  it('image 가 없거나 빈 문자열이면 floor_plan 은 undefined(배경 없음)', () => {
    expect(parseHeatmapConfig({ floor_plan: { fit: 'cover' } }).floor_plan).toBeUndefined();
    expect(parseHeatmapConfig({ floor_plan: { image: '   ' } }).floor_plan).toBeUndefined();
    expect(parseHeatmapConfig({ floor_plan: 'not-an-object' }).floor_plan).toBeUndefined();
  });

  it('무효 fit / natural 크기는 무시한다(기본 contain, 크기 미포함)', () => {
    const cfg = parseHeatmapConfig({
      floor_plan: {
        image: 'data:image/png;base64,CCCC',
        fit: 'stretch',
        natural_width: -1,
        natural_height: 0,
      },
    });
    expect(cfg.floor_plan).toEqual({ image: 'data:image/png;base64,CCCC', fit: 'contain' });
  });

  it('heatmap_opacity 는 0..1 로 clamp 하고 비숫자는 기본값(0.6)으로 폴백한다', () => {
    expect(parseHeatmapConfig({ heatmap_opacity: 0.3 }).heatmap_opacity).toBe(0.3);
    expect(parseHeatmapConfig({ heatmap_opacity: 1.5 }).heatmap_opacity).toBe(1);
    expect(parseHeatmapConfig({ heatmap_opacity: -0.4 }).heatmap_opacity).toBe(0);
    expect(parseHeatmapConfig({ heatmap_opacity: 'x' }).heatmap_opacity).toBe(DEFAULT_HEATMAP_OPACITY);
    expect(parseHeatmapConfig({}).heatmap_opacity).toBe(DEFAULT_HEATMAP_OPACITY);
  });

  it('editor 는 양의 snap/marker_size 만 통과, 그 외/없음은 undefined', () => {
    expect(parseHeatmapConfig({ editor: { snap: 0.05, marker_size: 12 } }).editor).toEqual({
      snap: 0.05,
      marker_size: 12,
    });
    expect(parseHeatmapConfig({ editor: { snap: 0.1 } }).editor).toEqual({ snap: 0.1 });
    expect(parseHeatmapConfig({ editor: { snap: -1, marker_size: 0 } }).editor).toBeUndefined();
    expect(parseHeatmapConfig({ editor: {} }).editor).toBeUndefined();
    expect(parseHeatmapConfig({}).editor).toBeUndefined();
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
