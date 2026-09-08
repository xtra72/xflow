// 목록형 패널(플로우 현황 · 에이전트 현황)의 디자인 설정 해석 — 순수 함수.
//
// 글자 모양(글꼴·크기·굵기)을 자리마다 고르던 설정 넷은 **걷어냈다**(타이틀 · 표 머리글 ·
// 표 본문 · 요약 배지). 지운 키 이름과 그 회귀 방지는 `removedFontConfigKeys.test.ts` 가
// 갖는다 — 여기 이름을 적으면 그 원문 검사가 이 파일을 위반으로 잡는다.
//
// 남은 것은 **색** 하나이며, 그 색의 출처는 악센트 설정 한 곳뿐이다:
//
//   테이블 헤더 → config.accentElements.table  (없으면 config.panelColor)
//   요약 배지   → config.accentElements.badges (없으면 config.panelColor)
//               + config.showSummaryBadges (표시 여부)
//
// 본문 셀은 색을 정할 자리가 아예 없으므로 패널 기본 클래스가 그대로 산다.

import type { CSSProperties } from 'react';

import { readTileFont, readTileItems, resolveTileStyle, type TileStyle } from './tileSelection';

/** 배지 배경 틴트의 알파(16진 2자리) — 글자색과 같은 색을 옅게 깐다. */
const BADGE_TINT_ALPHA = '20';

export interface ListPanelStyle {
  /** 상태 요약 배지를 그릴지. 기본 true — 명시적으로 끌 때만 숨긴다. */
  showSummaryBadges: boolean;
  /** 배지 스타일(악센트 색 + 그 색의 옅은 틴트). 색이 없으면 undefined. */
  badgeStyle: CSSProperties | undefined;
  /** 테이블 헤더 스타일(악센트 색). 색이 없으면 undefined. */
  headerStyle: CSSProperties | undefined;
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

/** 패널 config 에서 디자인 설정을 읽는다. 손상/미설정 값은 종전 모양으로 폴백한다. */
export function readListPanelStyle(config: Record<string, unknown> | undefined): ListPanelStyle {
  const accentElements =
    (config?.accentElements as Record<string, string | boolean> | undefined) ?? {};
  const panelColor = config?.panelColor as string | undefined;

  const tableAccent = accentColor(accentElements, panelColor, 'table');
  const badgeAccent = accentColor(accentElements, panelColor, 'badges');

  return {
    showSummaryBadges: config?.showSummaryBadges !== false,
    // 배지는 글자색과 같은 색을 옅게 깔아 알약 모양을 만든다. 색이 정해졌을 때만이다 —
    // 색이 없으면 상태별 기본 클래스(초록/빨강 등)가 그대로 살아야 한다.
    badgeStyle: badgeAccent
      ? { color: badgeAccent, backgroundColor: `${badgeAccent}${BADGE_TINT_ALPHA}` }
      : undefined,
    headerStyle: tableAccent ? { color: tableAccent } : undefined,
    headerAccent: tableAccent,
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
