// 속성 그리드(디바이스 상태) 패널의 배치·디자인 설정 해석 — 순수 함수.
//
// 카드 하나는 항목명 · 값 · 마지막 갱신 시각 세 조각으로 이뤄진다. 종전에는 이 셋이
// 세로로 고정 배치였고 글자 모양도 고를 수 없었다. 좁은 칸에서는 세 줄이 넘치고,
// 값만 크게 보고 싶은 화면에서는 항목명이 자리를 잡아먹는다.
//
// 값의 단일 원천은 config 이고, 화면은 여기서 나온 결과만 쓴다.

import type { CSSProperties } from 'react';

import {
  isDerivedPropertyKey,
  PROPERTY_GROUPS,
  propertyGroupOf,
  type PropertyGroup,
} from '@/lib/utils/deviceLabels';

import { resolvePanelTitleStyle } from '../panelChromeContext';
import type { TileGrid } from './tileLayout';

/** 카드 안에서 자리를 갖는 조각. */
export type CardElement = 'label' | 'value' | 'time';

export const CARD_ELEMENTS: CardElement[] = ['label', 'value', 'time'];

/**
 * 카드 분할 한계.
 *
 * 카드 하나를 나누는 수라 대시보드 격자(MAX_GRID_COLS)와 다르다. 6 을 넘기면 칸이
 * 글자보다 좁아져 어느 조각이 어디 있는지 알 수 없다.
 */
export const MIN_CARD_DIV = 1;
export const MAX_CARD_DIV = 6;
export const DEFAULT_CARD_ROWS = 3;
export const DEFAULT_CARD_COLS = 3;

/**
 * 조각이 차지하는 영역 — 시작 행·열과 사용할 칸 수.
 *
 * 종전에는 칸 번호 하나(1~9)였다. 그러면 "값을 위쪽 한 줄 전체" 처럼 여러 칸을 쓰는
 * 배치를 만들 수 없어, 값이 길면 칸 밖으로 넘쳤다.
 */
export interface CardArea {
  /** 1-based 시작 행 */
  row: number;
  /** 1-based 시작 열 */
  col: number;
  /** 사용할 행 수(>=1) */
  rowSpan: number;
  /** 사용할 열 수(>=1) */
  colSpan: number;
}

/** 조각 → 영역. 두 조각의 영역은 겹치지 않는다. */
export type CardAreas = Record<CardElement, CardArea>;

/** 카드 분할 크기. */
export interface CardGrid {
  rows: number;
  cols: number;
}

/**
 * 기본 배치 — 좌열에 위에서부터 항목명 / 값 / 시각(각 1칸).
 * 종전 세로 쌓기와 같은 모양이라 기존 패널이 그대로 보인다.
 */
const DEFAULT_AREAS: CardAreas = {
  label: { row: 1, col: 1, rowSpan: 1, colSpan: 1 },
  value: { row: 2, col: 1, rowSpan: 1, colSpan: 1 },
  time: { row: 3, col: 1, rowSpan: 1, colSpan: 1 },
};

/** 종전 칸 번호(1~9, 3×3 기준) → 행·열. */
function slotToArea(slot: number): CardArea {
  return {
    row: Math.ceil(slot / 3),
    col: ((slot - 1) % 3) + 1,
    rowSpan: 1,
    colSpan: 1,
  };
}

/**
 * 종전 `cardLayout` 값 → 칸 번호.
 * 저장된 패널이 이 변경으로 배치를 잃지 않도록 그대로 옮긴다.
 */
const LEGACY_LAYOUT_SLOTS: Record<string, Record<CardElement, number>> = {
  stack: { label: 1, value: 4, time: 7 },
  inline: { label: 1, value: 3, time: 7 },
  value: { value: 1, label: 4, time: 7 },
};

/** 정수로 맞추고 [min, max] 안으로 가둔다. 손상 값은 기본값으로 떨어진다. */
function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, n));
}

/** 카드 분할 크기를 읽는다. 기본 3×3. */
export function readCardGrid(config: Record<string, unknown> | undefined): CardGrid {
  return {
    rows: clampInt(config?.cardRows, MIN_CARD_DIV, MAX_CARD_DIV, DEFAULT_CARD_ROWS),
    cols: clampInt(config?.cardCols, MIN_CARD_DIV, MAX_CARD_DIV, DEFAULT_CARD_COLS),
  };
}

/** 영역이 격자를 벗어나지 않도록 가둔다. */
function clampArea(raw: Partial<CardArea> | undefined, grid: CardGrid): CardArea {
  const row = clampInt(raw?.row, 1, grid.rows, 1);
  const col = clampInt(raw?.col, 1, grid.cols, 1);
  return {
    row,
    col,
    // 시작 위치에서 격자 끝까지가 쓸 수 있는 최대 칸 수다.
    rowSpan: clampInt(raw?.rowSpan, 1, grid.rows - row + 1, 1),
    colSpan: clampInt(raw?.colSpan, 1, grid.cols - col + 1, 1),
  };
}

/** 값을 [min, max] 안으로 가둔다. max < min 인 퇴화 격자에서는 min 을 준다. */
function clampTo(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), Math.max(min, max));
}

/**
 * 영역을 칸 단위로 옮긴다 — 크기는 그대로 두고 격자 안에 가둔다.
 *
 * 격자 끝에 부딪히면 그 자리에 멈춘다. 끌다가 크기까지 줄어들면 손을 떼기 전에는
 * 무엇이 놓일지 알 수 없다.
 */
export function moveArea(area: CardArea, grid: CardGrid, dRow: number, dCol: number): CardArea {
  return {
    ...area,
    row: clampTo(area.row + dRow, 1, grid.rows - area.rowSpan + 1),
    col: clampTo(area.col + dCol, 1, grid.cols - area.colSpan + 1),
  };
}

/**
 * 오른쪽 아래 모서리를 끌어 칸 수를 바꾼다 — 시작 위치는 그대로다.
 *
 * 시작 위치에서 격자 끝까지가 쓸 수 있는 최대 칸 수다(clampArea 와 같은 규칙).
 */
export function resizeArea(
  area: CardArea,
  grid: CardGrid,
  dRowSpan: number,
  dColSpan: number,
): CardArea {
  return {
    ...area,
    rowSpan: clampTo(area.rowSpan + dRowSpan, 1, grid.rows - area.row + 1),
    colSpan: clampTo(area.colSpan + dColSpan, 1, grid.cols - area.col + 1),
  };
}

/** 두 영역이 겹치는지(직사각형 교차). */
function overlaps(a: CardArea, b: CardArea): boolean {
  return (
    a.col < b.col + b.colSpan &&
    b.col < a.col + a.colSpan &&
    a.row < b.row + b.rowSpan &&
    b.row < a.row + a.rowSpan
  );
}

/** 이미 놓인 영역들과 겹치지 않는 첫 1×1 칸. 없으면 좌상단. */
function firstFreeCell(placed: CardArea[], grid: CardGrid): CardArea {
  for (let row = 1; row <= grid.rows; row += 1) {
    for (let col = 1; col <= grid.cols; col += 1) {
      const cell: CardArea = { row, col, rowSpan: 1, colSpan: 1 };
      if (!placed.some((p) => overlaps(p, cell))) return cell;
    }
  }
  return { row: 1, col: 1, rowSpan: 1, colSpan: 1 };
}

/**
 * 조각별 영역을 읽는다.
 *
 * 영역이 겹치면 **먼저 오는 조각이 갖고** 뒤 조각은 빈 칸 하나로 밀린다
 * (label → value → time 순). 겹친 채로 그리면 글자가 포개져 둘 다 못 읽는다.
 *
 * 설정이 없으면 종전 형식을 차례로 옮긴다: `cardAreas` → `cardSlots`(칸 번호) →
 * `cardLayout`(정해진 배치 이름) → 기본 배치.
 */
export function readCardAreas(
  config: Record<string, unknown> | undefined,
  grid: CardGrid,
): CardAreas {
  const rawAreas = config?.cardAreas as Partial<Record<CardElement, Partial<CardArea>>> | undefined;
  const rawSlots = config?.cardSlots as Partial<Record<CardElement, unknown>> | undefined;
  const legacySlots = LEGACY_LAYOUT_SLOTS[String(config?.cardLayout ?? '')];

  const placed: CardArea[] = [];
  const out = {} as CardAreas;
  for (const element of CARD_ELEMENTS) {
    let wanted: Partial<CardArea> | undefined;
    if (rawAreas?.[element]) {
      wanted = rawAreas[element];
    } else if (rawSlots?.[element] !== undefined) {
      wanted = slotToArea(clampInt(rawSlots[element], 1, 9, 1));
    } else if (legacySlots) {
      wanted = slotToArea(legacySlots[element]);
    } else if (!rawAreas && !rawSlots) {
      wanted = DEFAULT_AREAS[element];
    }

    let area = clampArea(wanted, grid);
    if (placed.some((p) => overlaps(p, area))) {
      area = firstFreeCell(placed, grid);
    }
    placed.push(area);
    out[element] = area;
  }
  return out;
}

/** 열 수 한계. 1 미만은 격자가 성립하지 않고, 너무 크면 칸이 글자보다 좁아진다. */
export const MIN_GRID_COLS = 1;
export const MAX_GRID_COLS = 12;
export const DEFAULT_GRID_COLS = 3;

export interface PropertiesGridStyle {
  /** 격자 열 수(정수, 한계 안) */
  gridCols: number;
  /** 카드 분할(행×열). 기본 3×3. */
  cardGrid: CardGrid;
  /** 조각별 영역(시작 행·열 + 칸 수). 서로 겹치지 않는다. */
  areas: CardAreas;
  /** 마지막 갱신 시각을 그릴지. 기본 true — 명시적으로 끌 때만 숨긴다. */
  showUpdatedAt: boolean;
  labelStyle: CSSProperties | undefined;
  valueStyle: CSSProperties | undefined;
  timeStyle: CSSProperties | undefined;
}

/** 열 수를 정수로 맞추고 한계 안으로 가둔다. 손상 값은 기본값으로 떨어진다. */
export function clampGridCols(value: unknown): number {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n) || n <= 0) return DEFAULT_GRID_COLS;
  return Math.min(MAX_GRID_COLS, Math.max(MIN_GRID_COLS, n));
}

/**
 * 글자 스타일을 만들되 색이 비어 있으면 악센트 색으로 채운다.
 * 디자인 설정이 색을 정하면 그것이 이기고, 아니면 종전 악센트 색이 그대로 산다.
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

/** 패널 config 에서 배치·디자인 설정을 읽는다. */
export function readPropertiesGridStyle(
  config: Record<string, unknown> | undefined,
): PropertiesGridStyle {
  const cardGrid = readCardGrid(config);
  const accentElements =
    (config?.accentElements as Record<string, string | boolean> | undefined) ?? {};
  const panelColor = config?.panelColor as string | undefined;
  // 항목명은 종전에 악센트 `labels` 색을 따랐다 — 디자인이 색을 정하기 전까지 유지한다.
  const labelAccent = accentColor(accentElements, panelColor, 'labels');

  return {
    gridCols: clampGridCols(config?.gridCols),
    cardGrid,
    areas: readCardAreas(config, cardGrid),
    showUpdatedAt: config?.showUpdatedAt !== false,
    labelStyle: styleWithFallbackColor(config?.label_font, labelAccent),
    valueStyle: resolvePanelTitleStyle(config?.value_font),
    timeStyle: resolvePanelTitleStyle(config?.time_font),
  };
}

/**
 * 표시할 항목을 고르고 **고른 순서대로** 배열한다.
 *
 * `visibleProperties` 는 선택 목록이자 배치 순서다. 배열 순서를 무시하고 원본 순서로
 * 그리면 사용자가 정한 순번이 화면에 반영되지 않는다.
 *
 * 아무것도 고르지 않은 "전체" 는 디바이스가 보고하는 속성만 원래 순서로 낸다 —
 * 게이트웨이 수신 정보·메타데이터는 부가 정보라 골랐을 때만 그린다(파생 카드).
 */
export function selectEntries<T extends { key: string }>(
  entries: T[],
  visible: string[],
  /**
   * 값이 아직 없는 항목의 빈 카드를 만든다. 넘기지 않으면 그런 항목은 빠진다.
   *
   * 고정 설치 디바이스는 첫 통신 전까지 보고하는 값이 하나도 없다. 그때 카드까지
   * 사라지면 화면에는 아무것도 남지 않아, 고정해 둔 것이 무엇이었는지조차 알 수 없다.
   * 자리를 지키고 값만 '-' 로 두면 "아직 안 왔다"가 화면에서 읽힌다.
   *
   * `undefined` 를 돌려주면 그 항목은 만들지 않는다 — 지금은 없는 파생 항목처럼 되살릴
   * 수 없는 키를 위해서다.
   */
  make?: (key: string) => T | undefined,
): T[] {
  if (visible.length === 0) {
    return entries.filter((e) => !isDerivedPropertyKey(e.key));
  }
  const byKey = new Map(entries.map((e) => [e.key, e]));
  const out: T[] = [];
  for (const key of visible) {
    const found = byKey.get(key);
    if (found !== undefined) {
      out.push(found);
      continue;
    }
    const made = make?.(key);
    if (made !== undefined) out.push(made);
  }
  return out;
}

/**
 * 항목을 원하는 순번(1-based)으로 옮긴다.
 *
 * 끼워 넣기다 — 자리를 맞바꾸지 않는다. 맞바꾸면 3번을 1번으로 옮겼을 때 1번이
 * 3번으로 밀려나, "위로 올렸는데 다른 게 아래로 떨어지는" 예상 밖 결과가 된다.
 * 범위를 벗어난 순번은 양 끝으로 가둔다.
 */
export function moveToPosition<T extends string>(list: T[], key: T, position: number): T[] {
  const from = list.indexOf(key);
  if (from === -1) return list;
  const target = Math.min(list.length - 1, Math.max(0, Math.round(position) - 1));
  if (!Number.isFinite(target) || target === from) return list;

  const next = list.slice();
  next.splice(from, 1);
  next.splice(target, 0, key);
  return next;
}

// ---------------------------------------------------------------------------
// 항목별 세부 설정 — 디자인 덮어쓰기 + 값에 따른 색
// ---------------------------------------------------------------------------

/** 값 비교 연산. */
export type ValueRuleOp = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne';

export const VALUE_RULE_OPS: ValueRuleOp[] = ['gt', 'gte', 'lt', 'lte', 'eq', 'ne'];

/** 값이 조건을 만족하면 그 색을 쓴다. */
export interface ValueColorRule {
  op: ValueRuleOp;
  /** 비교 대상. 숫자로 읽히면 숫자로, 아니면 문자열로 비교한다. */
  value: string;
  color: string;
}

/** 항목 하나에만 걸리는 설정. 지정하지 않은 것은 카드 전체 설정을 따른다. */
export interface PropertyOverride {
  /** 이 항목만 쓰는 이름. 비우면 기본 이름을 쓴다. */
  label?: string;
  /** 값 뒤에 붙일 단위. 비우면 붙이지 않는다. */
  unit?: string;
  /** 이 항목만 쓰는 배경색(hex). 비우면 공통 배경을 따른다. */
  bg?: string;
  /** 이 항목만 쓰는 카드 분할(행). 없으면 카드 전체 설정을 따른다. */
  cardRows?: number;
  /** 이 항목만 쓰는 카드 분할(열). 없으면 카드 전체 설정을 따른다. */
  cardCols?: number;
  /** 이 항목만 쓰는 조각 배치. 없으면 카드 전체 설정을 따른다. */
  cardAreas?: unknown;
  label_font?: unknown;
  value_font?: unknown;
  time_font?: unknown;
  /** 값에 따른 색. 위에서부터 **먼저 맞는 규칙**이 이긴다. */
  valueColors?: ValueColorRule[];
}

/** config 에서 항목별 설정을 읽는다. */
export function readPropertyOverride(
  config: Record<string, unknown> | undefined,
  key: string,
): PropertyOverride {
  const all = config?.propertyOverrides as Record<string, PropertyOverride> | undefined;
  return all?.[key] ?? {};
}

/**
 * 값이 규칙에 맞는지.
 *
 * 양쪽이 모두 숫자로 읽히면 숫자로 비교한다 — 문자열 비교로 두면 "9" > "10" 이 되어
 * 임계값이 뒤집힌다. 그 밖에는 문자열로 비교한다(상태 문자열 등).
 */
function ruleMatches(rule: ValueColorRule, value: unknown): boolean {
  const rawLeft = typeof value === 'boolean' ? String(value) : value;
  const left = Number(rawLeft);
  const right = Number(rule.value);
  const numeric =
    rawLeft !== null &&
    rawLeft !== undefined &&
    String(rawLeft).trim() !== '' &&
    rule.value.trim() !== '' &&
    Number.isFinite(left) &&
    Number.isFinite(right);

  if (numeric) {
    switch (rule.op) {
      case 'gt':
        return left > right;
      case 'gte':
        return left >= right;
      case 'lt':
        return left < right;
      case 'lte':
        return left <= right;
      case 'eq':
        return left === right;
      case 'ne':
        return left !== right;
    }
  }

  const ls = String(rawLeft ?? '');
  switch (rule.op) {
    case 'eq':
      return ls === rule.value;
    case 'ne':
      return ls !== rule.value;
    // 문자열에 대소 비교는 뜻이 모호하다 — 사전순 비교가 사용자의 의도인 경우가
    // 드물어 맞지 않음으로 둔다.
    default:
      return false;
  }
}

/** 값에 맞는 첫 규칙의 색. 없으면 undefined. */
export function resolveValueColor(
  rules: ValueColorRule[] | undefined,
  value: unknown,
): string | undefined {
  if (!rules) return undefined;
  for (const rule of rules) {
    if (rule.color && ruleMatches(rule, value)) return rule.color;
  }
  return undefined;
}

/**
 * 카드 전체 설정에 항목별 설정을 덮어 최종 스타일을 만든다.
 *
 * 항목별 글자 설정이 있으면 그것이 이기고, 없으면 카드 전체 설정이 그대로 산다.
 * 값 색 규칙은 마지막에 적용되어 글자색을 덮는다 — "값에 따라 색이 변한다" 는 것이
 * 이 기능의 요지이므로 고정 색보다 우선한다.
 */
export function applyPropertyOverride(
  base: PropertiesGridStyle,
  override: PropertyOverride,
  value: unknown,
): Pick<PropertiesGridStyle, 'labelStyle' | 'valueStyle' | 'timeStyle'> {
  const labelStyle = override.label_font
    ? resolvePanelTitleStyle(override.label_font)
    : base.labelStyle;
  const timeStyle = override.time_font ? resolvePanelTitleStyle(override.time_font) : base.timeStyle;
  let valueStyle = override.value_font
    ? resolvePanelTitleStyle(override.value_font)
    : base.valueStyle;

  const color = resolveValueColor(override.valueColors, value);
  if (color) valueStyle = { ...(valueStyle ?? {}), color };

  return { labelStyle, valueStyle, timeStyle };
}


/**
 * 타일에 보일 이름 — 따로 정한 이름이 있으면 그것, 없으면 기본 이름.
 *
 * 공백만 있는 이름은 정하지 않은 것으로 본다. 빈 이름을 그대로 쓰면 그 타일이 무엇을
 * 보는 자리인지 화면에서 사라진다.
 */
export function resolveTileLabel(override: PropertyOverride | undefined, fallback: string): string {
  const own = typeof override?.label === 'string' ? override.label.trim() : '';
  return own !== '' ? own : fallback;
}

/**
 * 항목별 카드 배치 — 따로 잡은 것이 없으면 카드 전체 배치를 그대로 쓴다.
 *
 * 항목마다 판을 따로 두지 않는 것이 기본이다. 온도만 값을 크게 두고 나머지는 그대로
 * 두는 식으로 쓰라고 만든 것이지, 항목 수만큼 배치를 관리하라는 뜻이 아니다.
 *
 * 분할만 바꾸고 배치를 그대로 둘 수도 있어, 지정하지 않은 쪽은 전체 설정에서 물려받는다.
 */
export function resolveCardLayout(
  base: Pick<PropertiesGridStyle, 'cardGrid' | 'areas'>,
  override: PropertyOverride | undefined,
): { grid: CardGrid; areas: CardAreas } {
  const hasOwn =
    override !== undefined &&
    (override.cardAreas !== undefined ||
      override.cardRows !== undefined ||
      override.cardCols !== undefined);
  if (!hasOwn) return { grid: base.cardGrid, areas: base.areas };

  const grid: CardGrid = {
    rows: clampInt(override.cardRows, MIN_CARD_DIV, MAX_CARD_DIV, base.cardGrid.rows),
    cols: clampInt(override.cardCols, MIN_CARD_DIV, MAX_CARD_DIV, base.cardGrid.cols),
  };
  // 배치를 따로 잡지 않았어도 전체 배치를 이 격자에 맞춰 다시 가둔다 — 분할만 줄이면
  // 전체 배치가 격자를 넘칠 수 있다.
  return { grid, areas: readCardAreas({ cardAreas: override.cardAreas ?? base.areas }, grid) };
}

/**
 * 끌어 놓은 결과의 표시 순서.
 *
 * 표시 항목을 고르지 않은(= 전부 보이는) 상태에서는 옮길 순서 자체가 없다. 그래서 지금
 * 보이는 차례를 그대로 목록으로 굳힌 뒤 옮긴다 — 끌어 놓았는데 아무 일도 일어나지 않는
 * 것보다, 보이던 차례가 그대로 남는 편이 놀랍지 않다.
 */
export function reorderVisible(
  visible: string[],
  shown: string[],
  from: string,
  to: string,
): string[] {
  if (from === to) return visible;
  const list = visible.length > 0 ? visible : shown;
  const target = list.indexOf(to);
  if (target === -1 || list.indexOf(from) === -1) return visible;
  return moveToPosition(list, from, target + 1);
}

/**
 * 값이 갱신 시간 제한을 넘겨 오래되었는지.
 *
 * 제한을 정하지 않았으면 항상 false — 지금까지의 화면이 그대로 산다. 갱신 시각을 모르는
 * 값도 false 다: 언제 온 것인지 모른다는 것과 오래되었다는 것은 다른 말이고, 모른다는
 * 이유로 오래되었다고 단정하면 자리표시자까지 흐려진다.
 */
export function isStaleValue(
  timeMs: number | undefined,
  staleAfterSec: number | undefined,
  nowMs: number,
): boolean {
  if (staleAfterSec === undefined || !Number.isFinite(staleAfterSec) || staleAfterSec <= 0) {
    return false;
  }
  if (timeMs === undefined || !Number.isFinite(timeMs)) return false;
  return nowMs - timeMs > staleAfterSec * 1000;
}

// ---------------------------------------------------------------------------
// 그룹별 격자 배치
// ---------------------------------------------------------------------------

/**
 * 그룹별 기본 격자와 타일 크기.
 *
 * 상태 정보는 항목이 가장 많아 네 줄을 준다. 기본 정보와 수신 정보는 대개 넷 안팎이라
 * 두 줄이면 넉넉하다. 모자라면 `placeTiles` 가 아래로 이어 붙이므로 잘리지 않는다.
 */
export const PROPERTY_GROUP_GRID: Record<PropertyGroup, TileGrid> = {
  basic: { rows: 2, cols: 8 },
  status: { rows: 4, cols: 8 },
  gateway: { rows: 2, cols: 8 },
};

/** 카드 하나가 쓰는 기본 칸 수 — 8칸 폭에 넷이 들어간다. */
export const PROPERTY_TILE_SIZE = { w: 2, h: 2 };

/** 그룹별 격자·자리가 config 에 쓰이는 키. 패널과 설정이 같은 자리를 봐야 한다. */
export const PROPERTY_GROUP_GRID_KEY: Record<PropertyGroup, string> = {
  basic: 'basicGrid',
  status: 'statusGrid',
  gateway: 'gatewayGrid',
};

export const PROPERTY_GROUP_AREA_KEY: Record<PropertyGroup, string> = {
  basic: 'basicAreas',
  status: 'statusAreas',
  gateway: 'gatewayAreas',
};

/** 그룹 이름 i18n 키. */
export const PROPERTY_GROUP_LABEL_KEYS: Record<PropertyGroup, string> = {
  basic: 'dashboard.settings.propertiesGridOpt.basic',
  status: 'dashboard.settings.propertiesGridOpt.status',
  gateway: 'dashboard.settings.propertiesGridOpt.gateway',
};

/**
 * 항목을 그룹별로 나눈다 — 고른 차례를 그룹 안에서도 지킨다.
 *
 * 값이 하나도 없는 그룹은 아예 내지 않는다. 빈 제목만 남으면 무엇이 없는 것인지
 * 화면에서 읽히지 않는다.
 */
export function groupEntries<T extends { key: string }>(
  entries: T[],
): { group: PropertyGroup; entries: T[] }[] {
  return PROPERTY_GROUPS.map((group) => ({
    group,
    entries: entries.filter((e) => propertyGroupOf(e.key) === group),
  })).filter((g) => g.entries.length > 0);
}

/**
 * 같은 그룹 안에서 한 칸 옮긴다.
 *
 * 표시 목록은 그룹과 무관한 한 줄이라, 그룹 안 이웃과 자리를 바꾸려면 그 이웃이 전체
 * 목록에서 어디 있는지 찾아야 한다. 이웃이 없으면(맨 위·맨 아래) 그대로 둔다.
 */
export function moveWithinGroup(
  visible: string[],
  key: string,
  direction: -1 | 1,
): string[] {
  const group = propertyGroupOf(key);
  const from = visible.indexOf(key);
  if (from === -1) return visible;

  // 같은 그룹의 이웃을 방향대로 찾는다.
  let to = -1;
  for (let i = from + direction; i >= 0 && i < visible.length; i += direction) {
    if (propertyGroupOf(visible[i]!) === group) {
      to = i;
      break;
    }
  }
  if (to === -1) return visible;

  const next = visible.slice();
  next[from] = visible[to]!;
  next[to] = key;
  return next;
}

/**
 * 한 그룹을 통째로 켜거나 끈다 — 다른 그룹의 선택은 건드리지 않는다.
 *
 * 켤 때는 이미 고른 것 뒤에 그룹의 나머지를 붙인다(고른 차례를 흔들지 않는다). 끌 때는
 * 그 그룹 항목만 걷어낸다.
 */
export function setGroupSelection(shown: string[], groupKeys: string[], on: boolean): string[] {
  if (!on) return shown.filter((k) => !groupKeys.includes(k));
  const missing = groupKeys.filter((k) => !shown.includes(k));
  return [...shown, ...missing];
}
