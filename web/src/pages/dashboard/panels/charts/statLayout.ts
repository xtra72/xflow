// 통계 패널 요소의 배치·크기·글자 스타일 해석.
//
// 세 요소(본값 · 변화량 · 구간 통계)가 같은 규칙을 쓰므로 요소 종류(`kind`)를 받아
// 한 곳에서 푼다. **크기 폴백 규칙의 유일한 소유자**다 — `font_size` 가 배율을 이기고
// 곱해지지 않는다는 규칙(spec.md §3.2)이 세 자리에 흩어지면 갈린다.
//
// 요소마다 기본 px 이 다르다(본값 36 · 타일 24 · 보조 줄 14). 그 상수를 호출부가 들고
// 다니면 세 곳에 복제되므로 여기서만 안다.
//
// @spec SPEC-CHART-004 §2.1 [U1] / §2.2 [U2] / §3.2

import { clampPercentOffset } from './panelGeometry';
import {
  resolveFontColor,
  resolveFontFamily,
  type ChartFontFamily,
} from './textStyle';
import { readValueScale } from './valueScale';

/** 글자 크기(px) 허용 범위 — `textStyleFields` 의 입력 상한과 같은 값이어야 한다. */
export const FONT_SIZE_MIN = 6;
export const FONT_SIZE_MAX = 160;

/**
 * 오프셋 상한(백분율 포인트). 게이지·파이의 ±40(`PANEL_OFFSET_LIMIT`)과 **다르다.**
 *
 * 저 둘은 패널을 가득 채우는 그림을 옮기므로 40%면 이미 절반 이상이 잘려 나간다.
 * 통계는 작은 글자 덩어리라 사정이 반대다 — 흐름상 가운데에서 시작하므로, 패널의
 * 어느 모서리에든 놓으려면 각 축으로 50%가 필요하다. 40 에서 멈추면 "가장자리 근처까지만
 * 가고 더 안 간다" 가 된다(실제로 그렇게 보고됐다).
 */
export const STAT_OFFSET_LIMIT = 50;

/** 배치·스타일을 갖는 요소. 문자열로 두는 이유는 호출부에서 뜻이 보여야 하기 때문이다. */
export type StatElementKind = 'value' | 'delta' | 'stats';

/** 표시 순서 그대로의 요소 목록 — 정렬·초기화가 훑는 대상이다. */
export const STAT_ELEMENT_KINDS: readonly StatElementKind[] = ['value', 'delta', 'stats'];

/** 요소별 config 키와 기본 px. 상수의 유일한 자리다. */
const ELEMENT_SPEC: Record<
  StatElementKind,
  { key: string; basePx: number; scaleKey: string }
> = {
  value: { key: 'value_layout', basePx: 36, scaleKey: 'value_scale' },
  delta: { key: 'delta_layout', basePx: 14, scaleKey: 'sub_value_scale' },
  stats: { key: 'stats_layout', basePx: 14, scaleKey: 'sub_value_scale' },
};

/** 타일 경로에서 본값이 쓰는 기본 px — 단일 출력(36)보다 작다. */
const TILE_VALUE_BASE_PX = 24;

/** 저장 형태(config)와 달리 렌더가 바로 쓸 수 있게 푼 결과. */
export interface ResolvedStatLayout {
  /** 가로 오프셋(백분율 포인트). */
  offsetX: number;
  /** 세로 오프셋(백분율 포인트). */
  offsetY: number;
  /** 최종 글자 크기(px). 항상 값이 있다 — 폴백까지 여기서 끝낸다. */
  fontSize: number;
  /** CSS `font-family` 스택. 미지정이면 `undefined`(상속). */
  fontFamily: string | undefined;
  /** 글자색. 변화량은 항상 `undefined` — 방향별 색이 `delta_display` 소유다(§5 D4). */
  fontColor: string | undefined;
  fontWeight: 'normal' | 'bold' | undefined;
}

/**
 * 글자 크기를 허용 범위로 죈다. 수가 아니면 `undefined` — 폴백으로 떨어뜨린다.
 *
 * 0 이나 음수를 그대로 쓰면 글자가 사라져 되돌릴 수단이 화면에서 없어진다.
 */
export function clampFontSize(v: unknown): number | undefined {
  if (typeof v !== 'number' || !Number.isFinite(v)) return undefined;
  return Math.min(Math.max(v, FONT_SIZE_MIN), FONT_SIZE_MAX);
}

/** config 의 한 키를 객체로 읽는다. 배열·null·원시값은 "미지정" 으로 접는다. */
function readObject(config: Record<string, unknown>, key: string): Record<string, unknown> {
  const raw = config[key];
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return {};
  return raw as Record<string, unknown>;
}

function readOffset(v: unknown): number {
  return clampPercentOffset(
    typeof v === 'number' && Number.isFinite(v) ? v : 0,
    STAT_OFFSET_LIMIT,
  );
}

function readWeight(v: unknown): 'normal' | 'bold' | undefined {
  return v === 'normal' || v === 'bold' ? v : undefined;
}

/**
 * 요소 하나의 배치·크기·글자 스타일을 푼다. @spec SPEC-CHART-004 §3.2
 *
 * 크기 결정 순서: `font_size` 가 있으면 그 px, 없으면 `기본 px × 배율`.
 * **두 값을 곱하지 않는다** — 곱하면 핸들로 맞춘 크기가 슬라이더를 건드릴 때마다
 * 달라져 어느 쪽이 크기를 정하는지 알 수 없다.
 */
export function readStatLayout(
  config: Record<string, unknown>,
  kind: StatElementKind,
  opts: { tile?: boolean; single?: boolean } = {},
): ResolvedStatLayout {
  const spec = ELEMENT_SPEC[kind];
  const layout = readObject(config, spec.key);

  // 타일 경로의 본값만 기준이 다르다. 타일이 1개뿐이면 단일 출력과 크기를 맞춘다.
  const basePx =
    kind === 'value' && opts.tile && !opts.single ? TILE_VALUE_BASE_PX : spec.basePx;

  const explicit = clampFontSize(layout.font_size);
  const scale = readValueScale(config[spec.scaleKey]);

  return {
    offsetX: readOffset(layout.offset_x),
    offsetY: readOffset(layout.offset_y),
    fontSize: explicit ?? basePx * scale,
    fontFamily: resolveFontFamily(layout.font_family),
    // 변화량은 글자색 축이 없다 — 방향별 3색이 `delta_display` 에 있다.
    fontColor: kind === 'delta' ? undefined : resolveFontColor(layout.font_color),
    fontWeight: readWeight(layout.font_weight),
  };
}

/**
 * 요소에 **직접 지정된** 글자 크기(px)만 읽는다. 폴백을 채우지 않는다.
 *
 * `readStatLayout` 은 항상 값을 돌려주므로 "직접 지정했는가" 를 물을 수 없다. 설정
 * 화면이 배율 무시 안내를 낼지 판단하려면 그 구분이 필요하다(§2.6 U6-2).
 */
export function readStatLayoutFontSize(
  config: Record<string, unknown>,
  kind: StatElementKind,
): number | undefined {
  return clampFontSize(readObject(config, ELEMENT_SPEC[kind].key).font_size);
}

/** `readStatLayout` 이 읽는 저장 형태. 부분 갱신에 쓴다. */
export interface StatLayoutPatch {
  offset_x?: number | undefined;
  offset_y?: number | undefined;
  font_size?: number | undefined;
  font_family?: ChartFontFamily | undefined;
  font_color?: string | undefined;
  font_weight?: 'normal' | 'bold' | undefined;
}

/**
 * 요소 하나의 layout 을 부분 갱신한 **config 패치**를 만든다.
 *
 * 다른 요소의 키를 담지 않는다 — 세 요소는 독립이므로, 하나를 옮길 때 나머지가
 * 인자에 실리면 동시 편집에서 서로를 덮어쓴다.
 *
 * 모든 하위 필드가 비면 키 자체를 지운다. 빈 객체가 남으면 "설정했다" 로 읽힌다.
 */
export function writeStatLayout(
  config: Record<string, unknown>,
  kind: StatElementKind,
  patch: StatLayoutPatch,
): Record<string, unknown> {
  const spec = ELEMENT_SPEC[kind];
  const next: Record<string, unknown> = { ...readObject(config, spec.key), ...patch };
  for (const [k, v] of Object.entries(next)) {
    if (v === undefined) delete next[k];
  }
  return { [spec.key]: Object.keys(next).length === 0 ? undefined : next };
}
