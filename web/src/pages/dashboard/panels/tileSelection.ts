// 타일 목록 패널의 공통 해석 — 어느 타일을 어떤 차례로 낼지, 타일마다 어떤 모양으로.
//
// 에이전트 현황(요약 배지)과 에이전트 상태(통계 타일)가 같은 규칙을 쓴다. 각자 구현하면
// "미설정이면 전부" 나 "배경을 정했을 때만 기본 색을 걷어낸다" 같은 규칙이 조용히 갈라진다.
// 타일 이름과 config 키만 패널마다 다르고 규칙은 하나다.

import type { CSSProperties } from 'react';

import { resolvePanelTitleStyle } from '../panelChromeContext';
import { resolveValueColor, type ValueColorRule } from './propertiesGridStyle';

/** 배경 틴트의 알파(16진 2자리) — 글자색과 같은 색을 옅게 깐다. */
export const TILE_TINT_ALPHA = '20';

/**
 * 그릴 타일과 그 순서.
 *
 * 미설정이면 `all` 을 기본 순서로 — 지금까지의 화면이 그대로 산다. 배열이 곧 순서이므로
 * 사용자가 정한 차례가 화면에 그대로 반영된다. 알 수 없는 값과 중복은 걸러 낸다.
 * 빈 배열은 "하나도 안 냄"이라 기본으로 되돌리지 않는다 — 그러면 끌 수가 없다.
 */
export function readTileItems<T extends string>(
  raw: unknown,
  all: readonly T[],
  /** 미설정일 때 쓸 목록. 고를 수 있는 것과 처음부터 켜 두는 것이 다를 때 넘긴다. */
  fallback: readonly T[] = all,
): T[] {
  if (!Array.isArray(raw)) return [...fallback];
  const known = new Set<string>(all);
  const seen = new Set<string>();
  const out: T[] = [];
  for (const v of raw) {
    if (typeof v === 'string' && known.has(v) && !seen.has(v)) {
      seen.add(v);
      out.push(v as T);
    }
  }
  return out;
}

/** 타일 하나의 글자 설정(`<styles>.<타일>`). 없으면 빈 설정 — 공통 설정을 따른다. */
export function readTileFont(styles: unknown, item: string): Record<string, unknown> {
  if (!styles || typeof styles !== 'object') return {};
  const own = (styles as Record<string, unknown>)[item];
  return own && typeof own === 'object' ? (own as Record<string, unknown>) : {};
}

/** 타일 하나에 얹을 최종 모양. */
export interface TileStyle {
  /** 글자 + 배경 스타일. 정한 것이 없으면 공통 설정 그대로. */
  style: CSSProperties | undefined;
  /**
   * 배경색이 정해졌는지. 정해지지 않았으면 타일별 기본 색 클래스가 그대로 살아야 한다 —
   * 색으로 상태를 읽던 단서를 함부로 뺏지 않는다.
   */
  hasOwnBackground: boolean;
}

const HEX_COLOR = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i;

/**
 * 공통 설정 위에 타일별 설정을 덮는다.
 *
 * 타일 색을 정하면 배경 틴트도 그 색으로 다시 만든다 — 남겨 두면 글자만 바뀌고 배경은
 * 이전 색이라 서로 어긋난다. 배경을 직접 정했으면 그것이 이긴다.
 */
export function resolveTileStyle(
  base: CSSProperties | undefined,
  font: Record<string, unknown>,
): TileStyle {
  const own = resolvePanelTitleStyle(font);
  const bg =
    typeof font.bg === 'string' && HEX_COLOR.test(font.bg.trim()) ? font.bg.trim() : undefined;

  if (own === undefined && bg === undefined) {
    return { style: base, hasOwnBackground: base?.backgroundColor !== undefined };
  }

  const merged: CSSProperties = { ...(base ?? {}), ...own };
  if (own?.color) merged.backgroundColor = `${own.color}${TILE_TINT_ALPHA}`;
  if (bg) merged.backgroundColor = bg;

  return { style: merged, hasOwnBackground: merged.backgroundColor !== undefined };
}

// ---------------------------------------------------------------------------
// 타일 디자인 — 타이틀·값 글자를 따로, 값에 따른 색
// ---------------------------------------------------------------------------

/** 타일 하나의 디자인 설정(저장 형태). */
export interface TileDesign {
  /** 타이틀(항목명) 글자. */
  label_font?: unknown;
  /** 값 글자. */
  value_font?: unknown;
  /** 값에 따른 색 규칙 — 위에서 먼저 맞는 것이 이긴다. */
  valueColors?: ValueColorRule[];
  /** 배경색(hex). 정하지 않으면 타일별 기본 색이 그대로 산다. */
  bg?: string;
}

/**
 * 타일 하나의 디자인을 읽는다.
 *
 * 글꼴 항목이 최상위에 있는 옛 형태(`{ size, color, ... }`)는 **값 글자**로 읽는다. 이
 * 기능이 처음 나왔을 때는 타일 하나에 글자 설정이 하나뿐이었고, 그 하나가 걸리던 자리가
 * 값이었다 — 그대로 두면 저장된 설정이 조용히 사라진다.
 */
export function readTileDesign(styles: unknown, item: string): TileDesign {
  const own = readTileFont(styles, item);
  if (own.label_font !== undefined || own.value_font !== undefined || own.valueColors !== undefined) {
    return own as TileDesign;
  }
  const { bg, ...font } = own;
  const legacy = Object.keys(font).length > 0 ? font : undefined;
  return { value_font: legacy, bg: typeof bg === 'string' ? bg : undefined };
}

/**
 * 공통 설정 위에 타일별 설정을 덮는다.
 *
 * 글꼴은 **항목 단위로** 덮는다 — 공통에서 크기만 정하고 타일에서 색만 정하면 둘 다
 * 살아야 한다. 통째로 갈아치우면 타일에서 색 하나 바꾸는 순간 공통 크기가 사라진다.
 */
export function mergeTileDesign(common: TileDesign, own: TileDesign): TileDesign {
  const font = (a: unknown, b: unknown): unknown => {
    if (a === undefined) return b;
    if (b === undefined) return a;
    return { ...(a as object), ...(b as object) };
  };
  return {
    label_font: font(common.label_font, own.label_font),
    value_font: font(common.value_font, own.value_font),
    valueColors: own.valueColors ?? common.valueColors,
    bg: own.bg ?? common.bg,
  };
}

/** 타일에 얹을 최종 모양 — 상자·타이틀·값. */
export interface ResolvedTileDesign {
  /** 상자(배경) 스타일. */
  box: CSSProperties | undefined;
  hasOwnBackground: boolean;
  labelStyle: CSSProperties | undefined;
  valueStyle: CSSProperties | undefined;
}

/**
 * 디자인을 화면에 얹을 스타일로 편다.
 *
 * 값 색 규칙이 맞으면 값 글자색을 덮는다 — "값에 따라 변한다"가 이 기능의 요지다.
 */
export function resolveTileDesign(design: TileDesign, value: unknown): ResolvedTileDesign {
  const bg =
    typeof design.bg === 'string' && HEX_COLOR.test(design.bg.trim()) ? design.bg.trim() : undefined;
  const labelStyle = resolvePanelTitleStyle(design.label_font);
  const base = resolvePanelTitleStyle(design.value_font);
  const ruled = resolveValueColor(design.valueColors, value);

  const valueStyle = ruled ? { ...(base ?? {}), color: ruled } : base;
  return {
    box: bg ? { backgroundColor: bg } : undefined,
    hasOwnBackground: bg !== undefined,
    labelStyle,
    valueStyle,
  };
}
