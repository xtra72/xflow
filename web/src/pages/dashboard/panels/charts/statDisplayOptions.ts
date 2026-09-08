// 통계 패널 보조 표기(변화량 · 구간 통계 · 보조 줄 크기)의 config 해석.
//
// 이 모듈은 **해석만** 한다 — 값을 계산하지도(그것은 `seriesReduce`), 그리지도
// 않는다(그것은 `StatSubLines`). 셋을 나눈 이유는 경로 의존 기본값(§5 D2)이
// 패널 본문에 삼항식으로 흩어지면 두 렌더 경로가 조용히 어긋나기 때문이다.
//
// @spec SPEC-CHART-003 §2.2 [U2] / §2.3 [U3] / §2.4 [U4]

import { readValueScale } from './valueScale';

/** 보조 표기가 놓이는 렌더 경로. 불리언(`isTile`)으로 두면 호출부에서 뜻이 사라진다. */
export type StatRenderPath = 'legacy' | 'tile';

/** 구간 통계 항목. 표시 순서가 곧 이 배열의 순서다(§2.3 U3-3). */
export type WindowStatKind = 'avg' | 'max' | 'min';

/** 켠 항목을 담을 때 쓰는 고정 순서. 설정 기재 순서를 따르지 않는다. */
const WINDOW_STAT_ORDER: readonly WindowStatKind[] = ['avg', 'max', 'min'];

/**
 * 색 미지정 시의 기본값 — 현행 하드코딩(`text-emerald-500` / `text-rose-500` /
 * `text-(--color-text-muted)`)과 같은 색이다. 설정을 비워 둔 패널의 외형이
 * 변하지 않아야 한다(§3.2).
 */
export const DEFAULT_DELTA_COLORS = {
  up: '#10b981',
  down: '#f43f5e',
  flat: 'var(--color-text-muted)',
} as const;

export interface DeltaColors {
  up: string;
  down: string;
  flat: string;
}

/** config 의 한 키를 객체로 읽는다. 배열·null·원시값은 "미지정" 으로 접는다. */
function readObject(config: Record<string, unknown>, key: string): Record<string, unknown> {
  const raw = config[key];
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return {};
  return raw as Record<string, unknown>;
}

/**
 * 변화량을 그릴지. **미지정의 뜻이 경로마다 다르다**(§5 D2).
 *
 * 레거시 경로는 변화량을 항상 그리고 있었고, 다중 타일 경로는 SPEC-CHART-002 §7 OQ5
 * 결정에 따라 한 번도 그리지 않았다. 미지정을 한쪽으로 통일하면 반드시 한쪽 저장
 * 대시보드의 외형이 바뀌므로, 미지정을 "현행 유지" 로 정의한다.
 *
 * 명시된 `true` / `false` 는 두 경로에서 똑같이 작동한다.
 */
export function readDeltaEnabled(
  config: Record<string, unknown>,
  path: StatRenderPath,
): boolean {
  const enabled = readObject(config, 'delta_display').enabled;
  if (typeof enabled === 'boolean') return enabled;
  return path === 'legacy';
}

/** 방향별 변화량 글자색. 지정하지 않은 색만 기본값으로 채운다. */
export function readDeltaColors(config: Record<string, unknown>): DeltaColors {
  const d = readObject(config, 'delta_display');
  const pick = (key: string, fallback: string): string =>
    typeof d[key] === 'string' && d[key] !== '' ? (d[key] as string) : fallback;
  return {
    up: pick('up_color', DEFAULT_DELTA_COLORS.up),
    down: pick('down_color', DEFAULT_DELTA_COLORS.down),
    flat: pick('flat_color', DEFAULT_DELTA_COLORS.flat),
  };
}

/**
 * 켠 구간 통계 항목을 **고정 순서**로 반환한다.
 *
 * 배열로 돌려주는 이유: 순서 규칙이 렌더 쪽에도 있으면 두 곳이 갈린다. 순서는
 * 여기 한 곳에만 있다.
 */
export function readWindowStats(config: Record<string, unknown>): WindowStatKind[] {
  const w = readObject(config, 'window_stats');
  return WINDOW_STAT_ORDER.filter((kind) => w[kind] === true);
}

/**
 * 보조 줄(변화량 + 구간 통계) 공용 크기 배율.
 *
 * 본값의 `value_scale` 과 **별개 축**이다 — 본값만 키우고 보조 줄은 그대로 두거나
 * 그 반대도 가능해야 한다. 클램프 규칙은 본값과 공유한다(§2.4 U4-3).
 */
export function readSubValueScale(config: Record<string, unknown>): number {
  return readValueScale(config.sub_value_scale);
}
