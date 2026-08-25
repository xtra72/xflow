// 패널 설정 미리보기의 시리즈 산출 (순수 함수).
//
// 미리보기는 합성 데이터를 그리지만 **시리즈 목록·이름·스타일은 실제 설정에서** 와야 한다.
// 과거에는 LineChartMiniPreview 가 채널 설정만 읽어, store 모드 패널에서 시리즈를 골라도
// 이름 형식을 바꿔도 미리보기가 가짜 sample 한 줄로 고정돼 있었다(보고된 결함).
//
// Recharts 렌더 없이 검증할 수 있도록 산출 로직만 분리한다.

import { normalizeStoreSeriesAlias, storeSeriesLabel } from './chartChannelTypes';
import { groupComboSignature } from '@/services/api/tsdbSeriesEnum';
import type {
  ChannelRefConfig,
  StoreSourceConfig,
  StrokeStyle,
  TsdbSourceConfig,
} from './chartChannelTypes';
import { resolvePanelSourceBinding } from './panelDataSource';

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
  /** TSDB 소스 설정. 등록된 시리즈로 미리보기를 구성한다. @spec SPEC-TSDB-004 */
  tsdbSource?: TsdbSourceConfig | undefined;
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
    tsdbSource,
    channels,
    channelName,
    globalSmooth,
    strokeDasharray,
    palette,
    sampleName,
    channelFallbackName,
  } = input;

  const color = (i: number): string => palette[i % palette.length] ?? '';

  // SPEC-TSDB-002 §2.3 [U3]: 소스 종류 판정을 계약에 위임한다(부재·인식 불가 → channel).
  //
  // **활성 판정과 채널 폴백의 결합을 그대로 유지한다.** 아래는 조기 반환이 아니라
  // "store 인데 시리즈가 0개면 아래 채널 분기로 흘러내린다" 는 구조다. 이를 조기 반환으로
  // 바꾸면 폴백이 사라지고 미리보기 범례가 sample 한 줄로 퇴화한다(CT-17).
  // 그래서 안쪽 `series.length > 0` 검사도 `binding.active` 로 대체하지 않는다 —
  // 계약의 store 활성 조건에는 tag 대안이 있지만 여기 폴백 기준은 시리즈 길이뿐이다.
  const { kind } = resolvePanelSourceBinding({
    data_source: dataSource,
    store_source: storeSource,
    tsdb_source: tsdbSource,
  });
  if (kind === 'store') {
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

  // TSDB 도 같은 규칙으로 **등록된 시리즈**를 미리보기로 만든다.
  //
  // group by 항목은 config 상 1개이지만 런타임에는 고른 조합마다 한 줄이 되므로,
  // 미리보기도 그 수만큼 펼친다 — 펼치지 않으면 "설정에서는 한 줄인데 대시보드에는
  // 여러 줄" 이 되어 미리보기가 제 일을 못 한다.
  //
  // 색은 group by 파생 줄에서 항목 color 를 쓰지 않는다(SPEC-TSDB-004 OQ1) —
  // 실제 렌더가 자동 팔레트를 쓰므로 미리보기도 같아야 한다.
  if (kind === 'tsdb') {
    const lines: PreviewSeries[] = [];
    for (const ref of tsdbSource?.series ?? []) {
      const picks = ref.group_by?.length ? (ref.group_filter ?? []) : [];
      if (picks.length === 0) {
        lines.push({
          key: storeSeriesLabel(ref, tsdbSource?.series_name_format),
          color: ref.color ?? color(lines.length),
          smooth: ref.smooth ?? globalSmooth,
          strokeWidth: ref.stroke_width ?? 2,
          strokeDasharray: ref.stroke_style ? strokeDasharray[ref.stroke_style] : '',
        });
        continue;
      }
      const groupKeys = [...(ref.group_by ?? [])].sort();
      for (const combo of picks) {
        // 그룹별 개별 이름이 있으면 그것이 이긴다(§2.12) — 실제 렌더와 같은
        // 우선순위를 미리보기도 따라야 "미리보기와 대시보드가 다르다" 가 안 된다.
        const named = ref.group_alias?.[groupComboSignature(combo, groupKeys)]?.trim();
        const merged = {
          ...ref,
          tags: { ...(ref.tags ?? {}), ...combo },
          ...(named ? { alias: named } : {}),
        };
        lines.push({
          key: storeSeriesLabel(merged, tsdbSource?.series_name_format),
          color: color(lines.length),
          smooth: ref.smooth ?? globalSmooth,
          strokeWidth: ref.stroke_width ?? 2,
          strokeDasharray: ref.stroke_style ? strokeDasharray[ref.stroke_style] : '',
        });
      }
    }
    // 등록이 0개면 아래 폴백으로 흘러내린다 — 빈 차트는 고장처럼 보인다.
    if (lines.length > 0) return lines;
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
