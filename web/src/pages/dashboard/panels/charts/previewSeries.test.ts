// buildPreviewSeries 단위 테스트.
//
// 보고된 결함: store 모드 라인 차트 미리보기가 채널 설정만 보고 가짜 sample 한 줄을 그려,
// 시리즈를 골라도·시리즈 이름 형식을 바꿔도 미리보기가 그대로였다.

import { describe, it, expect } from 'vitest';

import { buildPreviewSeries, type PreviewSeriesInput } from './previewSeries';
import type { StoreSourceConfig } from './chartChannelTypes';

const DASH = { solid: '', dashed: '6 4', dotted: '2 3' } as const;
const PALETTE = ['#c1', '#c2', '#c3'] as const;

function input(over: Partial<PreviewSeriesInput> = {}): PreviewSeriesInput {
  return {
    dataSource: undefined,
    storeSource: undefined,
    channels: [],
    channelName: '',
    globalSmooth: false,
    strokeDasharray: DASH,
    palette: PALETTE,
    sampleName: '(sample)',
    channelFallbackName: (i) => `채널 ${i}`,
    ...over,
  };
}

function storeSource(over: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    series: [
      { key: 'LAI', field: 'value', tags: { room: '1' } },
      { key: 'LAeq', field: 'value', tags: { room: '1' }, alias: '소음' },
    ],
    ...over,
  } as StoreSourceConfig;
}

describe('buildPreviewSeries — store 모드: 선택된 시리즈 반영', () => {
  it('선택된 store 시리즈를 그대로 미리보기 시리즈로 만든다', () => {
    const out = buildPreviewSeries(input({ dataSource: 'store', storeSource: storeSource() }));
    expect(out).toHaveLength(2);
    expect(out.map((s) => s.key)).toEqual(['LAI · value{room=1}', '소음']);
  });

  it('이름을 직접 입력한 시리즈는 그 이름을 쓴다', () => {
    const out = buildPreviewSeries(input({ dataSource: 'store', storeSource: storeSource() }));
    expect(out[1]!.key).toBe('소음');
  });

  it('per-series 색/선스타일/곡선이 반영된다', () => {
    const src = storeSource({
      series: [
        { key: 'k', field: 'value', color: '#abc', stroke_style: 'dashed', stroke_width: 4, smooth: true },
      ],
    } as Partial<StoreSourceConfig>);
    const [s] = buildPreviewSeries(input({ dataSource: 'store', storeSource: src }));
    expect(s).toEqual({
      key: 'k · value',
      color: '#abc',
      smooth: true,
      strokeWidth: 4,
      strokeDasharray: '6 4',
    });
  });

  it('색 미지정 시 인덱스 팔레트를 배정한다', () => {
    const out = buildPreviewSeries(input({ dataSource: 'store', storeSource: storeSource() }));
    expect(out.map((s) => s.color)).toEqual(['#c1', '#c2']);
  });

  it('선택된 시리즈가 없으면 sample 한 줄로 폴백한다(빈 차트 방지)', () => {
    const out = buildPreviewSeries(
      input({ dataSource: 'store', storeSource: storeSource({ series: [] }) }),
    );
    expect(out).toEqual([
      { key: '(sample)', color: '#c1', smooth: false, strokeWidth: 2, strokeDasharray: '' },
    ]);
  });
});

describe('buildPreviewSeries — store 모드: 패널 옵션 반영', () => {
  it('시리즈 이름 형식이 이름 없는 시리즈에 적용된다', () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: storeSource({ series_name_format: '{$.measurement}/{$.field}' }),
      }),
    );
    expect(out[0]!.key).toBe('LAI/value');
  });

  it('직접 입력한 이름은 이름 형식을 이긴다', () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: storeSource({ series_name_format: '{$.measurement}/{$.field}' }),
      }),
    );
    expect(out[1]!.key).toBe('소음');
  });

  it('태그 토큰도 해석한다', () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: storeSource({ series_name_format: '[{$.tags.room}] {$.measurement}' }),
      }),
    );
    expect(out[0]!.key).toBe('[1] LAI');
  });

  it('legacy 기본 alias(=measurement)는 이름 형식에 자리를 내준다', () => {
    const src = storeSource({
      series: [{ key: 'LAI', field: 'value', alias: 'LAI' }],
      series_name_format: '{$.measurement}/{$.field}',
    } as Partial<StoreSourceConfig>);
    const out = buildPreviewSeries(input({ dataSource: 'store', storeSource: src }));
    expect(out[0]!.key).toBe('LAI/value');
  });
});

describe('buildPreviewSeries — 채널 모드(기존 동작 보존)', () => {
  it('채널 목록을 그대로 시리즈로 만든다', () => {
    const out = buildPreviewSeries(
      input({ channels: [{ name: 'ch-a' }, { name: '', alias: '별칭' }] as never }),
    );
    expect(out.map((s) => s.key)).toEqual(['ch-a', '별칭']);
  });

  it('이름이 비면 인덱스 폴백 이름을 쓴다', () => {
    const out = buildPreviewSeries(input({ channels: [{ name: '' }] as never }));
    expect(out[0]!.key).toBe('채널 1');
  });

  it('채널이 없으면 channel_name, 그것도 없으면 sample 을 쓴다', () => {
    expect(buildPreviewSeries(input({ channelName: 'ch' }))[0]!.key).toBe('ch');
    expect(buildPreviewSeries(input())[0]!.key).toBe('(sample)');
  });

  it('store 모드가 아니면 store_source 를 무시한다', () => {
    const out = buildPreviewSeries(input({ dataSource: 'channel', storeSource: storeSource() }));
    expect(out[0]!.key).toBe('(sample)');
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `previewSeries.ts:65` 의 구조는 다음과 같다.
//
//   if (dataSource === 'store') { ... if (series.length > 0) return ...; }
//   if (channels.length > 0) { return ... }        // ← 흘러내리는 폴백
//   return [sample]
//
// 즉 **활성 판정과 채널 폴백이 결합**되어 있다. store 를 골랐지만 시리즈가 0개면
// `return` 하지 않고 아래로 흘러내려 채널 목록을 쓴다. M3 에서 이 블록을 조기
// 반환(early return)으로 바꾸면 그 폴백이 사라지고 미리보기 범례가 sample 한 줄로
// 퇴화한다 — CT-17 이 그 회귀를 잡는다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.5 CT-16 ~ CT-18 / AC-12
// ---------------------------------------------------------------------------
describe('buildPreviewSeries 활성 판정 + 채널 폴백 결합 특성화 (SPEC-TSDB-002 M2, CT-16~CT-18)', () => {
  it("CT-16: dataSource:'store' + 시리즈 N개면 store 시리즈 범례 N개다(채널이 있어도)", () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: storeSource(),
        channels: [{ name: 'ch-a' }, { name: 'ch-b' }, { name: 'ch-c' }] as never,
      }),
    );
    expect(out.map((s) => s.key)).toEqual(['LAI · value{room=1}', '소음']);
  });

  it("CT-17: dataSource:'store' + 시리즈 0개 + 채널 M개면 **채널 범례 M개**로 폴백한다", () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: storeSource({ series: [] }),
        channels: [{ name: 'ch-a' }, { name: 'ch-b' }, { name: 'ch-c' }] as never,
      }),
    );
    // sample 한 줄이 아니라 채널 3줄이다. 조기 반환으로 바꾸면 여기가 깨진다.
    expect(out).toHaveLength(3);
    expect(out.map((s) => s.key)).toEqual(['ch-a', 'ch-b', 'ch-c']);
  });

  it("CT-17: storeSource 자체가 없어도 채널 폴백은 동일하다", () => {
    const out = buildPreviewSeries(
      input({
        dataSource: 'store',
        storeSource: undefined,
        channels: [{ name: 'ch-a' }] as never,
      }),
    );
    expect(out.map((s) => s.key)).toEqual(['ch-a']);
  });

  it("CT-18: dataSource:'store' + 시리즈 0개 + 채널 0개면 sample 한 줄이다", () => {
    const out = buildPreviewSeries(
      input({ dataSource: 'store', storeSource: storeSource({ series: [] }), channels: [] }),
    );
    expect(out).toEqual([
      { key: '(sample)', color: '#c1', smooth: false, strokeWidth: 2, strokeDasharray: '' },
    ]);
  });
});
