// 패널 설정 미리보기의 시리즈 산출 (순수 함수).
//
// 미리보기는 합성 데이터를 그리지만 **시리즈 목록·이름·스타일은 실제 설정에서** 와야 한다.
// 과거에는 LineChartMiniPreview 가 채널 설정만 읽어, store 모드 패널에서 시리즈를 골라도
// 이름 형식을 바꿔도 미리보기가 가짜 sample 한 줄로 고정돼 있었다(보고된 결함).
//
// Recharts 렌더 없이 검증할 수 있도록 산출 로직만 분리한다.

import { normalizeStoreSeriesAlias, storeSeriesLabel } from './chartChannelTypes';
import type { ChannelRefConfig, StoreSourceConfig, StrokeStyle } from './chartChannelTypes';

/** 미리보기 한 줄의 렌더 파라미터. */
export interface PreviewSeries {
  /** 범례/데이터 키로 쓰이는 표시 이름. */
  key: string;
  color: string;
  smooth: boolean;
  strokeWidth: number;
  strokeDasharray: string;
}

/** 미리보기 시리즈 산출 입력. */
export interface PreviewSeriesInput {
  dataSource: string | undefined;
  storeSource: StoreSourceConfig | undefined;
  channels: readonly ChannelRefConfig[];
  channelName: string;
  globalSmooth: boolean;
  /** stroke_style → SVG dasharray 매핑(호출부의 상수를 그대로 받는다). */
  strokeDasharray: Record<StrokeStyle, string>;
  /** 인덱스별 폴백 색 팔레트. */
  palette: readonly string[];
  /** 채널 미설정 시 쓰는 표시 이름. */
  sampleName: string;
  /** 다중 채널 모드에서 이름이 빈 채널의 표시 이름(인덱스 1-based). */
  channelFallbackName: (index1: number) => string;
}

/**
 * 설정에서 미리보기 시리즈 목록을 만든다.
 *
 * 우선순위:
 *   1. store 모드 + 선택된 시리즈 있음 → 선택 시리즈. 이름은 실제 렌더와 같은 규칙
 *      (직접 입력한 이름 → 패널의 시리즈 이름 형식 → 내장 서술 표기), 스타일은 per-series 값.
 *   2. 다중 채널 모드 → 채널 목록.
 *   3. 그 외 → sample 한 줄(미설정 상태 표시).
 *
 * store 모드인데 선택된 시리즈가 0개면 3번으로 폴백한다 — 빈 차트는 고장처럼 보인다.
 */
export function buildPreviewSeries(input: PreviewSeriesInput): PreviewSeries[] {
  const {
    dataSource,
    storeSource,
    channels,
    channelName,
    globalSmooth,
    strokeDasharray,
    palette,
    sampleName,
    channelFallbackName,
  } = input;

  const color = (i: number): string => palette[i % palette.length] ?? '';

  if (dataSource === 'store') {
    const series = normalizeStoreSeriesAlias(storeSource?.series ?? []);
    if (series.length > 0) {
      return series.map((ref, i) => ({
        key: storeSeriesLabel(ref, storeSource?.series_name_format),
        color: ref.color ?? color(i),
        smooth: ref.smooth ?? globalSmooth,
        strokeWidth: ref.stroke_width ?? 2,
        strokeDasharray: ref.stroke_style ? strokeDasharray[ref.stroke_style] : '',
      }));
    }
  }

  if (channels.length > 0) {
    return channels.map((c, i) => ({
      key: c.alias ?? (c.name || channelFallbackName(i + 1)),
      color: c.color ?? color(i),
      smooth: c.smooth ?? globalSmooth,
      strokeWidth: c.stroke_width ?? 2,
      strokeDasharray: c.stroke_style ? strokeDasharray[c.stroke_style] : '',
    }));
  }

  return [
    {
      key: channelName || sampleName,
      color: color(0),
      smooth: globalSmooth,
      strokeWidth: 2,
      strokeDasharray: '',
    },
  ];
}
