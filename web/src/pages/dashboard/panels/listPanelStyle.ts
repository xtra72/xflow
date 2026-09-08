// 목록형 패널(플로우 현황 · 에이전트 현황)의 디자인 설정 해석 — 순수 함수.
//
// 이 패널들은 글자 모양을 정할 자리가 셋이다(타이틀·테이블·요약 배지). 종전에는 타이틀
// 색이 "타이틀 디자인"과 "스타일(악센트 header)" 두 곳에 있어 어느 쪽이 이기는지 알 수
// 없었다. 이제 각 자리는 **디자인 설정 한 곳**만 갖는다:
//
//   타이틀      → config.title_font        (패널 공통 크롬)
//   테이블 헤더 → config.table_header_font
//   테이블 요소 → config.table_cell_font
//   요약 배지   → config.badge_font + config.showSummaryBadges
//
// 악센트 색(accentElements.table / .badges)은 **폴백**으로만 남긴다. 종전에 색을 지정해
// 둔 패널이 이 변경으로 조용히 원래 색을 잃지 않도록 하기 위함이다.

import type { CSSProperties } from 'react';

import { resolvePanelTitleStyle } from '../panelChromeContext';
import { readTileFont, readTileItems, resolveTileStyle, type TileStyle } from './tileSelection';

/** 배지 배경 틴트의 알파(16진 2자리) — 글자색과 같은 색을 옅게 깐다. */
const BADGE_TINT_ALPHA = '20';

export interface ListPanelStyle {
  /** 상태 요약 배지를 그릴지. 기본 true — 명시적으로 끌 때만 숨긴다. */
  showSummaryBadges: boolean;
  /** 배지 글자 스타일. 지정이 없으면 undefined(패널 기본 클래스가 그대로 산다). */
  badgeStyle: CSSProperties | undefined;
  /** 테이블 헤더 글자 스타일 */
  headerStyle: CSSProperties | undefined;
  /** 테이블 요소(본문 셀) 글자 스타일 */
  cellStyle: CSSProperties | undefined;
  /** 헤더 강조색 — 정렬 화살표처럼 색만 필요한 자리에 쓴다. */
  headerAccent: string | undefined;
}

/** accentElements 에서 그룹의 유효 색을 읽는다(false = 끔). */
function accentColor(
  accentElements: Record<string, string | boolean>,
  panelColor: string | undefined,
  group: string,
): string | undefined {
  if (accentElements[group] === false) return undefined;
  const value = accentElements[group];
  if (typeof value === 'string') return value;
  return panelColor;
}

/**
 * 글자 스타일을 만들되, 색이 비어 있으면 악센트 색으로 채운다.
 *
 * 디자인 설정이 색을 정하면 그것이 이기고, 아직 정하지 않았다면 종전 악센트 색을
 * 그대로 쓴다 — 설정 화면이 바뀌었다고 화면 색이 달라지면 안 된다.
 */
function styleWithFallbackColor(
  font: unknown,
  fallback: string | undefined,
): CSSProperties | undefined {
  const style = resolvePanelTitleStyle(font);
  if (style?.color) return style;
  if (!fallback) return style;
  return { ...(style ?? {}), color: fallback };
}

/** 패널 config 에서 디자인 설정을 읽는다. 손상/미설정 값은 종전 모양으로 폴백한다. */
export function readListPanelStyle(config: Record<string, unknown> | undefined): ListPanelStyle {
  const accentElements =
    (config?.accentElements as Record<string, string | boolean> | undefined) ?? {};
  const panelColor = config?.panelColor as string | undefined;

  const tableAccent = accentColor(accentElements, panelColor, 'table');
  const badgeAccent = accentColor(accentElements, panelColor, 'badges');

  const badgeStyle = styleWithFallbackColor(config?.badge_font, badgeAccent);

  return {
    showSummaryBadges: config?.showSummaryBadges !== false,
    // 배지는 글자색과 같은 색을 옅게 깔아 알약 모양을 만든다. 색이 정해졌을 때만이다 —
    // 색이 없으면 상태별 기본 클래스(초록/빨강 등)가 그대로 살아야 한다.
    badgeStyle: badgeStyle?.color
      ? { ...badgeStyle, backgroundColor: `${badgeStyle.color}${BADGE_TINT_ALPHA}` }
      : badgeStyle,
    headerStyle: styleWithFallbackColor(config?.table_header_font, tableAccent),
    cellStyle: resolvePanelTitleStyle(config?.table_cell_font),
    headerAccent: (resolvePanelTitleStyle(config?.table_header_font)?.color as string | undefined) ?? tableAccent,
  };
}

// ---------------------------------------------------------------------------
// 요약 타일 — 표시 항목 선택 + 항목별 디자인
// ---------------------------------------------------------------------------

/** 요약 타일 종류. 순서는 기본 표시 순서다. */
export type SummaryItem = 'total' | 'active' | 'inactive';

export const SUMMARY_ITEMS: SummaryItem[] = ['total', 'active', 'inactive'];

/** 그릴 요약 타일과 그 순서. 규칙은 타일 공통 모듈이 갖는다. */
export function readSummaryItems(config: Record<string, unknown> | undefined): SummaryItem[] {
  return readTileItems(config?.summaryItems, SUMMARY_ITEMS);
}

/** 타일 하나의 글자 설정(`summaryStyles.<타일>`). */
export function readSummaryTileFont(
  config: Record<string, unknown> | undefined,
  item: SummaryItem,
): Record<string, unknown> {
  return readTileFont(config?.summaryStyles, item);
}

/** 공통 배지 설정 위에 타일별 설정을 덮는다. */
export const resolveSummaryTileStyle = resolveTileStyle;

/** @deprecated 이름만 남긴 별칭 — 타일 모양 타입은 공통 모듈이 갖는다. */
export type SummaryTileStyle = TileStyle;
