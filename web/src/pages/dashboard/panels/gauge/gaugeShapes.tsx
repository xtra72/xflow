// 게이지 렌더러 — 8종 모양 + 기하 헬퍼.
//
// GaugePanel 에서 그대로 옮겨 왔다. 렌더러들은 `parseConfig` 가 만든 평범한 데이터
// 레코드만 받으므로 패널에 묶여 있지 않다 — sysmetrics 패널도 같은 게이지를 쓴다.
// 한 곳에서만 그려야 "게이지 패널의 바늘"과 "시스템 지표의 바늘"이 갈라지지 않는다.
//
// 이 파일은 그리기만 한다. 값을 어디서 가져오는지(스토어·채널·에이전트)는 패널의 몫이다.

// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import {
  readDecimalPlaces,
} from '@/pages/dashboard/panels/charts/decimalPlaces';
import { formatTickValue, scaleValueUnit } from '../charts/unitOptions';
// 현재값 크기·위치는 통계 패널과 **같은 정본**을 쓴다(`charts/valueScale`).
import { readValueOffset, readValueScale } from '../charts/valueScale';
import type { ReactElement } from 'react';

export type GaugeType =
  | 'simple'
  | 'half'
  | 'needle'
  | 'needle-rainbow'
  | 'vertical-bar'
  | 'half-rainbow';

/** 고를 수 있는 게이지 모양 (설정 UI 표시 순서) */
export const GAUGE_TYPES: readonly GaugeType[] = [
  'simple',
  'half',
  'needle',
  'needle-rainbow',
  'half-rainbow',
  'vertical-bar',
];

export interface ThresholdEntry {
  name: string;
  color: string;
  from: number;
  to: number;
}

/**
 * 게이지 **공통 기본 값 색**.
 *
 * 종전에는 유형마다 다른 색이 박혀 있었다 — 도넛·반원은 청회색(`#5B8FB9`), 바늘은
 * 빨강(`#EF4444`), 세로 바는 주황(`#F97316`), 멀티링은 링 3색을 따로 갖고 임계값 색을
 * 아예 쓰지 않았다. 유형은 **모양**을 고르는 설정인데 색까지 따라 바뀌어, 같은 값·같은
 * 설정에서 유형만 바꿔도 색이 튀었다. 색은 한 곳에서 정한다.
 */
export const DEFAULT_GAUGE_COLOR = '#5B8FB9';

/**
 * 연속 컬러 테마 — 값의 위치(0~1)에 따라 색이 이어서 바뀐다.
 *
 * 설정 화면의 `연속` 모드가 고르는 프리셋과 **같은 정본**이다. 종전에는 이 목록이
 * 설정 화면 안에만 있고 게이지는 읽지 않아, 테마를 바꿔도 화면이 그대로였다.
 */
export const GAUGE_COLOR_THEMES: Record<string, readonly string[]> = {
  'green-red': ['#10b981', '#f59e0b', '#ef4444'],
  'blue-purple': ['#3b82f6', '#8b5cf6', '#a855f7'],
  'cyan-blue': ['#06b6d4', '#3b82f6', '#1e40af'],
};

/** 테마를 모르면 첫 테마로 접는다. */
const DEFAULT_COLOR_THEME = 'green-red';

/** 16진수 색을 RGB 성분으로. `#rgb` 축약형도 받는다. */
function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
  return [
    parseInt(full.slice(0, 2), 16),
    parseInt(full.slice(2, 4), 16),
    parseInt(full.slice(4, 6), 16),
  ];
}

/** RGB 성분을 16진수 색으로. */
function rgbToHex(r: number, g: number, b: number): string {
  const to = (n: number): string => Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, '0');
  return `#${to(r)}${to(g)}${to(b)}`;
}

/**
 * 테마 색 목록을 `ratio`(0~1) 지점에서 **선형 보간**한다.
 *
 * 구간을 뚝뚝 끊지 않는 이유가 곧 "연속" 이다 — 끊어 쓸 것이면 개별 모드(임계값 색)와
 * 다를 바가 없다. 색이 하나뿐이면 그 색을 그대로 돌려준다.
 */
export function sampleColorTheme(themeId: string | undefined, ratio: number): string {
  const colors = GAUGE_COLOR_THEMES[themeId ?? ''] ?? GAUGE_COLOR_THEMES[DEFAULT_COLOR_THEME]!;
  if (colors.length === 1) return colors[0]!;
  const t = Math.max(0, Math.min(1, Number.isFinite(ratio) ? ratio : 0));
  const span = 1 / (colors.length - 1);
  const idx = Math.min(colors.length - 2, Math.floor(t / span));
  const local = (t - idx * span) / span;
  const [r1, g1, b1] = hexToRgb(colors[idx]!);
  const [r2, g2, b2] = hexToRgb(colors[idx + 1]!);
  return rgbToHex(
    r1 + (r2 - r1) * local,
    g1 + (g2 - g1) * local,
    b1 + (b2 - b1) * local,
  );
}

/**
 * 임계값을 지정하지 않은 게이지의 **기본 3구간**.
 *
 * 종전에는 이 기본값이 설정 화면 안에만 있었고(하드코딩 `0/60/80/100`), 게이지는
 * `config.thresholds ?? []` 로 읽었다. 그래서 편집기에는 정상·주의·위험 세 줄이 뜨고
 * "임계값 영역 표시" 도 켜져 보이는데 게이지에는 아무것도 나오지 않았다. 두 곳이 같은
 * 함수를 부르게 해서 **보이는 것과 그려지는 것을 일치**시킨다.
 *
 * **값 범위에 비례한다.** 종전 하드코딩은 0~100 을 전제해서, 범위를 0~1000 으로 잡으면
 * 세 구간이 아크의 앞 10% 에만 몰리고 값 600 은 어느 구간에도 걸리지 않아 폴백 색으로
 * 떨어졌다. 경계는 범위의 60% · 80% 지점이다 — 비율은 종전과 같고 기준만 범위로 바뀐다.
 *
 * 이름은 비운다. 화면에 보일 이름은 로케일마다 달라 표시 계층의 몫이고, 여기서 정해야
 * 하는 것은 **구간과 색**이다.
 */
export function defaultGaugeThresholds(min: number, max: number): ThresholdEntry[] {
  const at = (ratio: number): number => min + (max - min) * ratio;
  return [
    { name: '', color: '#10b981', from: min, to: at(0.6) },
    { name: '', color: '#f59e0b', from: at(0.6), to: at(0.8) },
    { name: '', color: '#ef4444', from: at(0.8), to: max },
  ];
}

// ---- 헬퍼 함수 ----

/** 각도(deg)를 라디안으로 변환 */
const toRad = (deg: number) => (deg * Math.PI) / 180;

/** 극좌표 → 직교좌표 (SVG 좌표계, 12시 방향 = 0도) */
function polarToCartesian(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = toRad(angleDeg - 90);
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) };
}

/** 호(arc) SVG path — startAngle→endAngle 시계방향(화면 기준) */
function describeArc(
  cx: number,
  cy: number,
  r: number,
  startAngle: number,
  endAngle: number,
): string {
  const s = polarToCartesian(cx, cy, r, startAngle);
  const e = polarToCartesian(cx, cy, r, endAngle);
  const span = ((endAngle - startAngle) % 360 + 360) % 360;
  const largeArc = span > 180 ? 1 : 0;
  return `M ${s.x},${s.y} A ${r},${r} 0 ${largeArc},1 ${e.x},${e.y}`;
}

/**
 * 임계값 영역용 **도넛 띠** path — 트랙을 따라 도는 얇은 고리 조각.
 *
 * 종전에는 중심에서 퍼지는 부채꼴(파이)이었다. 파이는 게이지 한가운데를 덮어 값 숫자와
 * 눈금 라벨 위로 색이 깔렸고, 아크로 값을 읽는 게이지 안에 축이 다른 도형이 하나 더
 * 생기는 꼴이었다. 띠로 바꾸면 임계값 구간이 값 아크와 **같은 길** 위에 놓여, 니들 RB ·
 * 반원 RB 의 세그먼트 아크와도 같은 언어가 된다.
 *
 * 360°(전 구간을 덮는 단일 임계값)는 시작점과 끝점이 겹쳐 호가 성립하지 않으므로
 * 바깥 원과 안쪽 원을 **반대 방향**으로 그려 고리를 만든다(nonzero 규칙으로 가운데가 뚫린다).
 */
function describeDonutBand(
  cx: number,
  cy: number,
  outerR: number,
  innerR: number,
  startAngle: number,
  endAngle: number,
): string {
  const span = endAngle - startAngle;
  // 뒤집힌 구간(to < from)이나 눈에 보이지 않는 구간은 그리지 않는다.
  if (!Number.isFinite(span) || span < 0.1) return '';
  if (span >= 359.9) {
    return [
      `M ${cx - outerR},${cy}`,
      `a ${outerR},${outerR} 0 1,0 ${outerR * 2},0`,
      `a ${outerR},${outerR} 0 1,0 ${-outerR * 2},0`,
      'Z',
      `M ${cx - innerR},${cy}`,
      `a ${innerR},${innerR} 0 1,1 ${innerR * 2},0`,
      `a ${innerR},${innerR} 0 1,1 ${-innerR * 2},0`,
      'Z',
    ].join(' ');
  }
  return describeDonutArc(cx, cy, outerR, innerR, startAngle, endAngle);
}

/** 도넛형 아크 path (외원 → 내원) */
function describeDonutArc(
  cx: number,
  cy: number,
  outerR: number,
  innerR: number,
  startAngle: number,
  endAngle: number,
): string {
  const outerStart = polarToCartesian(cx, cy, outerR, startAngle);
  const outerEnd = polarToCartesian(cx, cy, outerR, endAngle);
  const innerStart = polarToCartesian(cx, cy, innerR, startAngle);
  const innerEnd = polarToCartesian(cx, cy, innerR, endAngle);
  const largeArc = endAngle - startAngle > 180 ? 1 : 0;
  return [
    `M ${outerStart.x},${outerStart.y}`,
    `A ${outerR},${outerR} 0 ${largeArc},1 ${outerEnd.x},${outerEnd.y}`,
    `L ${innerEnd.x},${innerEnd.y}`,
    `A ${innerR},${innerR} 0 ${largeArc},0 ${innerStart.x},${innerStart.y}`,
    'Z',
  ].join(' ');
}

/** 값을 비율로 변환 */
export function normalize(value: number, min: number, max: number) {
  if (max === min) return 0;
  return Math.max(0, Math.min(1, (value - min) / (max - min)));
}

/** 임계값에 따른 색상 결정 */
export function getThresholdColor(value: number, thresholds: ThresholdEntry[], fallback: string): string {
  for (const t of thresholds) {
    if (value >= t.from && value <= t.to) return t.color;
  }
  return fallback;
}

/**
 * 게이지 값에 쓸 색 — 유형이 아니라 **설정**이 정한다.
 *
 * 우선순위:
 *   1. 연속 모드 → 테마를 값 위치(0~1)에서 보간한 색
 *   2. 개별 모드 → 값이 든 임계값 구간의 색
 *   3. 어느 구간에도 들지 않음 → 공통 기본색
 */
export function resolveGaugeValueColor(args: {
  value: number;
  min: number;
  max: number;
  thresholds: ThresholdEntry[];
  colorMode: 'individual' | 'continuous';
  colorTheme?: string;
}): string {
  const { value, min, max, thresholds, colorMode, colorTheme } = args;
  if (colorMode === 'continuous') {
    return sampleColorTheme(colorTheme, normalize(value, min, max));
  }
  return getThresholdColor(value, thresholds, DEFAULT_GAUGE_COLOR);
}

// ---- config 파싱 ----

export function parseConfig(config: Record<string, unknown>) {
  const value = (config.value as number) ?? 0;
  // 값 표기 자릿수. 종전에는 `{value}` 를 그대로 찍어 21.533333333333335 가 나왔다.
  // 눈금 라벨(min/max 사이 등분)은 눈금자이므로 종전대로 정수 반올림을 유지한다.
  const decimals = readDecimalPlaces(config);
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const configuredUnit = (config.unit as string) ?? '%';
  const gaugeType = (config.gaugeType as GaugeType) ?? 'simple';
  // 미지정이면 범위에 비례한 기본 3구간이다 — 설정 화면이 보여주는 것과 같은 값이다.
  const thresholds = (config.thresholds as ThresholdEntry[]) ?? defaultGaugeThresholds(min, max);
  // 옵션: 임계값 영역을 트랙을 따라 도는 도넛 띠로 시각화.
  // 명시적으로 false 가 아닌 한 thresholds 가 1개 이상이면 기본 ON.
  // 미설정(undefined) 이면 thresholds 존재 여부로 결정한다.
  const baseColor = readBaseColor(config.base_color);
  // 니들 색. 비어 있으면 본문 글자색을 따른다(종전 동작) — 색을 박지 않고 테마 변수를
  // 쓰면 다크·라이트 양쪽에서 보이던 성질을 잃지 않는다.
  const needleColor = typeof config.needle_color === 'string' ? config.needle_color : '';
  const valueScale = readValueScale(config.value_scale);
  const valueOffsetX = readValueOffset(config.value_offset_x);
  const valueOffsetY = readValueOffset(config.value_offset_y);
  const colorMode: 'individual' | 'continuous' =
    config.colorMode === 'continuous' ? 'continuous' : 'individual';
  const colorTheme = config.colorTheme as string | undefined;
  // 반원 RB 전용. 다른 타입은 읽지 않으므로 저장돼 있어도 무해하다.
  const halfRainbowDirection = readHalfRainbowDirection(config.half_rainbow_direction);
  const showThresholdZones = config.showThresholdZones === false
    ? false
    : config.showThresholdZones === true || thresholds.length > 0;
  // 표기 문자열과 접미사는 **함께** 정해진다 — 자동 데이터 량은 값의 크기가 접미사를
  // 정하므로 둘을 따로 계산하면 `1.21` 옆에 `auto:bytes` 가 붙는다.
  const shown = scaleValueUnit(value, decimals, configuredUnit);
  return {
    value,
    // 렌더러가 그리는 **표기 문자열**. 비율 계산에는 여전히 `value`(수)를 쓴다.
    valueText: shown.text,
    decimals,
    min,
    max,
    // 저장된 단위가 아니라 **이 값에 붙는** 단위다. 고정 단위면 저장값 그대로다.
    unit: shown.suffix,
    /** 저장된 단위 설정. 값이 바뀔 때 접미사를 다시 정하려면 이 값이 필요하다. */
    configuredUnit,
    gaugeType,
    thresholds,
    showThresholdZones,
    halfRainbowDirection,
    colorMode,
    colorTheme,
    /** 테두리 안쪽을 채우는 색. 빈 문자열이면 채우지 않는다. */
    baseColor,
    /** 니들 색. 빈 문자열이면 본문 글자색을 따른다. */
    needleColor,
    /** 현재값 글자 크기 배율(기본 1). */
    valueScale,
    /** 현재값 가로 변위(viewBox 좌표, 기본 0). */
    valueOffsetX,
    /** 현재값 세로 변위(viewBox 좌표, 기본 0). */
    valueOffsetY,
    /**
     * 이 값에 쓸 색. **유형과 무관하게 여기 하나에서 정한다.**
     *
     * 종전에는 유형마다 폴백 색이 달라 유형만 바꿔도 색이 튀었다. 규칙은 하나다:
     * 연속 모드면 테마를 값 위치에서 보간하고, 개별 모드면 값이 든 임계값 구간 색을
     * 쓰고, 어느 구간에도 없으면 공통 기본색이다.
     */
    valueColor: resolveGaugeValueColor({
      value,
      min,
      max,
      thresholds,
      colorMode,
      colorTheme,
    }),
  };
}

/**
 * 파싱된 config 의 값만 **라이브 값으로 갈아 끼운다.**
 *
 * `{ ...base, value }` 로 덮으면 안 된다 — 표기 문자열(`valueText`)이 config 의 정적
 * 값에서 만들어진 채로 남아, 그림은 라이브 값으로 움직이는데 가운데 숫자만 옛 값에
 * 멈춰 있는 상태가 된다. 두 필드가 항상 함께 바뀌도록 여기 하나로 모은다.
 */
export function withGaugeValue(
  base: ReturnType<typeof parseConfig>,
  value: number,
): ReturnType<typeof parseConfig> {
  const shown = scaleValueUnit(value, base.decimals, base.configuredUnit);
  return { ...base, value, valueText: shown.text, unit: shown.suffix };
}

/**
 * 임계값 구간을 게이지의 **트랙 자체**에 칠할 조각들.
 *
 * 종전에는 트랙이 회색이고 임계 색은 안쪽 얇은 띠에만 있었다. 게이지의 본체인 원·반원이
 * 정작 무채색이라, 값이 어느 구간에 있는지 읽으려면 눈을 안쪽 띠로 옮겨야 했다.
 * 구간과 값을 **한 고리에서** 읽도록 트랙을 구간 색으로 나눈다.
 *
 * 값까지는 진하게, 값을 넘은 구간은 연하게 그린다 — 같은 고리 위에서 "여기까지 왔다" 와
 * "이 다음은 무슨 구간이다" 가 함께 읽힌다. 조각은 값 지점에서 한 번 잘리므로, 값이
 * 구간 한가운데 있으면 그 구간이 진한 쪽과 연한 쪽 둘로 나뉜다.
 *
 * @param valueRatio 현재 값의 위치(0~1). 값이 없으면 0 을 넘겨 전부 연하게 만든다.
 */
export function trackThresholdSegments(
  thresholds: ThresholdEntry[],
  min: number,
  max: number,
  valueRatio: number,
): { startRatio: number; endRatio: number; color: string; filled: boolean }[] {
  const out: { startRatio: number; endRatio: number; color: string; filled: boolean }[] = [];
  for (const t of thresholds) {
    const from = normalize(t.from, min, max);
    const to = normalize(t.to, min, max);
    if (!(to > from)) continue; // 뒤집힌 구간·폭 0 구간은 그리지 않는다.
    // 값 지점에서 자른다. 값이 구간 밖이면 한쪽 조각만 남는다.
    const cut = Math.max(from, Math.min(to, valueRatio));
    if (cut > from) out.push({ startRatio: from, endRatio: cut, color: t.color, filled: true });
    if (to > cut) out.push({ startRatio: cut, endRatio: to, color: t.color, filled: false });
  }
  return out;
}

/** 값까지 채운 조각의 불투명도. */
const TRACK_FILLED_OPACITY = 1;

/** 기본색 미지정 시의 트랙 채움 — 종전 트랙 회색이다. */
export const DEFAULT_TRACK_FILL = '#E2E8F0';

/**
 * 트랙 기본 채움색을 읽는다.
 *
 * 미지정(`undefined`)은 기본 회색이고, **빈 문자열은 "채우지 않음"** 이다. 둘을 나누는
 * 이유: 사용자가 채움을 끄는 것과 설정을 건드리지 않은 것은 다른 뜻이고, 끈 상태에서는
 * 테두리만 남아야 한다.
 */
export function readBaseColor(raw: unknown): string {
  if (raw === undefined || raw === null) return DEFAULT_TRACK_FILL;
  return typeof raw === 'string' ? raw : DEFAULT_TRACK_FILL;
}

/**
 * 아크형 게이지의 트랙 — **기본색 채움 + 값까지 구간 색 + 구간 테두리**.
 *
 * 다섯 유형(도넛 · 반원 · 바늘 · 니들 RB · 반원 RB)이 이 하나를 공유한다. 반지름과
 * 각도만 다를 뿐 그리는 순서와 규약이 같아야 하고, 유형마다 다시 짜면 테두리 굵기나
 * 채움 순서가 조용히 어긋난다.
 *
 * **두 가지 표현 방식**이 있다. 어느 쪽이든 기본색 채움이 맨 아래에 깔린다.
 *
 * - `progress`(도넛 · 반원 · 세로 바): 0~값 구간만 임계 색으로 채운다. 그 뒤는 기본색이
 *   그대로 보인다 — 채움이 "여기까지 왔고 지금 어느 구간이다" 를 함께 뜻한다.
 * - `zone`(바늘 · 니들 RB · 반원 RB): 구간 **전체**를 그 색으로 채운다. 값은 니들이
 *   가리키므로 채움이 값을 뜻할 필요가 없고, 눈금 전체가 색으로 읽힌다.
 *
 * 방식이 갈리는 이유는 값을 무엇으로 읽느냐다 — 니들이 있는 유형은 채움을 값 표시에
 * 쓰지 않으므로 구간을 통째로 칠하는 편이 눈금으로서 더 읽힌다.
 */
function ThresholdTrackArc({
  cx,
  cy,
  outerR,
  innerR,
  startAngle,
  totalAngle,
  thresholds,
  min,
  max,
  ratio,
  baseColor,
  mode,
}: {
  cx: number;
  cy: number;
  outerR: number;
  innerR: number;
  startAngle: number;
  totalAngle: number;
  thresholds: ThresholdEntry[];
  min: number;
  max: number;
  /** 현재 값의 위치(0~1). 값이 없으면 0. `zone` 에서는 쓰지 않는다. */
  ratio: number;
  baseColor: string;
  mode: 'progress' | 'zone';
}): ReactElement {
  const band = (from: number, to: number): string =>
    describeDonutBand(
      cx, cy, outerR, innerR,
      startAngle + from * totalAngle,
      startAngle + to * totalAngle,
    );
  const filled = trackThresholdSegments(thresholds, min, max, ratio).filter((s) => s.filled);

  if (mode === 'zone') {
    return (
      <>
        {baseColor !== '' && <path d={band(0, 1)} fill={baseColor} />}
        {thresholds.map((t, i) => {
          const d = band(normalize(t.from, min, max), normalize(t.to, min, max));
          return d ? <path key={`z${i}`} d={d} fill={t.color} /> : null;
        })}
      </>
    );
  }

  return (
    <>
      {baseColor !== '' && <path d={band(0, 1)} fill={baseColor} />}
      {filled.map((seg, i) => {
        const d = band(seg.startRatio, seg.endRatio);
        return d ? <path key={`f${i}`} d={d} fill={seg.color} opacity={TRACK_FILLED_OPACITY} /> : null;
      })}
    </>
  );
}

/**
 * 게이지 가운데의 **값 + 단위** — 두 행으로 쌓는다.
 *
 * 한 행에 이어 붙이면 단위가 길어질수록(`10.99KB/s`) 숫자가 밀려 작아지고, 좁은
 * 타일에서는 줄바꿈이 값과 단위를 임의의 자리에서 끊는다. 행을 나누면 숫자 크기가
 * 단위 길이에 영향받지 않는다.
 *
 * 단위가 없으면 둘째 행을 만들지 않는다 — 빈 행이 값을 위로 밀어 올린다.
 */
function GaugeValueText({
  x,
  y,
  valueText,
  unit,
  valueSize,
  unitSize,
  hasValue,
  valueFill,
  unitClassName,
  unitOpacity,
  scale = 1,
  offsetX = 0,
  offsetY = 0,
}: {
  x: number;
  y: number;
  valueText: string;
  unit: string;
  valueSize: number;
  unitSize: number;
  hasValue: boolean;
  /** 값 글자색 클래스 대신 직접 지정할 때(니들 배지처럼 어두운 바탕 위). */
  valueFill?: string;
  unitClassName?: string;
  unitOpacity?: number;
  /** 글자 크기 배율(기본 1). */
  scale?: number;
  /** 가로·세로 변위(viewBox 좌표, 기본 0). */
  offsetX?: number;
  offsetY?: number;
}): ReactElement {
  const twoLine = hasValue && unit !== '';
  const vSize = valueSize * scale;
  const uSize = unitSize * scale;
  // 두 행이면 블록 전체가 아래로 반 행 내려가므로 시작점을 그만큼 올린다.
  const startY = (twoLine ? y - uSize * 0.55 : y) + offsetY;
  const cx = x + offsetX;
  return (
    <text
      // 설정 미리보기의 드래그 레이어가 이 표시로 "값 글자를 잡았다" 를 판정한다.
      // 대시보드에 놓인 패널에는 드래그 레이어가 없으므로 표시만 남고 아무 일도 없다.
      data-gauge-value-text=""
      x={cx}
      y={startY}
      textAnchor="middle"
      dominantBaseline="central"
      className={valueFill ? undefined : 'fill-(--color-text-primary)'}
      fill={valueFill}
      fontWeight={700}
    >
      <tspan x={cx} fontSize={vSize}>
        {hasValue ? valueText : '--'}
      </tspan>
      {twoLine && (
        <tspan
          x={cx}
          dy={uSize * 1.15}
          fontSize={uSize}
          className={unitClassName}
          opacity={unitOpacity}
        >
          {unit}
        </tspan>
      )}
    </text>
  );
}

// ---- 게이지 렌더러 ----

/** 1. Simple Gauge (도넛형) — 360° 도넛 */
function SimpleGauge({ value, valueText, min, max, unit, thresholds, hasValue, showThresholdZones, valueColor: color, baseColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, outerR = 90, innerR = 72;
  const valueAngle = ratio * 360;

  const useZones = showThresholdZones && thresholds.length > 0;

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
      {useZones ? (
        <ThresholdTrackArc
          cx={cx} cy={cy} outerR={outerR} innerR={innerR}
          startAngle={0} totalAngle={360}
          thresholds={thresholds} min={min} max={max}
          ratio={hasValue ? ratio : 0} baseColor={baseColor}
          mode={'progress'}
        />
      ) : (
        <>
          {/* 임계 구간이 없으면 종전대로 트랙 + 값 아크 하나로 표시한다. */}
          <circle cx={cx} cy={cy} r={(outerR + innerR) / 2} fill="none"
            stroke={baseColor === '' ? 'none' : baseColor} strokeWidth={outerR - innerR} />
          {hasValue && valueAngle > 0.5 && (
            <path d={describeDonutArc(cx, cy, outerR, innerR, 0, valueAngle)} fill={color} />
          )}
        </>
      )}
      <GaugeValueText
        x={cx} y={cy - 2} valueText={valueText} unit={unit}
        valueSize={28} unitSize={14} hasValue={hasValue}
        unitClassName="fill-(--color-text-muted)"
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

/** 2. Half-Circular Gauge (반원형) — 상단 180° */
function HalfGauge({ value, valueText, min, max, unit, configuredUnit, thresholds, hasValue, showThresholdZones, valueColor: color, baseColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 120, cy = 100, outerR = 80, innerR = 62;
  const startAngle = 270; // 9시(왼쪽) 시작
  const totalAngle = 180; // → 3시(오른쪽) 끝
  const valueAngle = ratio * totalAngle;

  const useZones = showThresholdZones && thresholds.length > 0;

  return (
    <svg viewBox="0 0 240 140" className="h-full w-full">
      {useZones ? (
        <ThresholdTrackArc
          cx={cx} cy={cy} outerR={outerR} innerR={innerR}
          startAngle={startAngle} totalAngle={totalAngle}
          thresholds={thresholds} min={min} max={max}
          ratio={hasValue ? ratio : 0} baseColor={baseColor}
          mode={'progress'}
        />
      ) : (
        <>
          {/* 트랙 — 도넛형으로 통일 (round cap 아티팩트 제거) */}
          {baseColor !== '' && (
            <path d={describeDonutArc(cx, cy, outerR, innerR, startAngle, startAngle + totalAngle)}
              fill={baseColor} />
          )}
          {hasValue && valueAngle > 0.5 && (
            <path d={describeDonutArc(cx, cy, outerR, innerR, startAngle, startAngle + valueAngle)}
              fill={color} />
          )}
        </>
      )}
      {/* 양 끝 눈금 — 값과 같은 단위로 읽혀야 한다. 자릿수는 눈금 크기가 정한다. */}
      <text x={cx - outerR - 4} y={cy + 12} textAnchor="end"
        className="fill-(--color-text-muted)" fontSize={9} fontWeight={500}>
        {formatTickValue(min, configuredUnit)}
      </text>
      <text x={cx + outerR + 4} y={cy + 12} textAnchor="start"
        className="fill-(--color-text-muted)" fontSize={9} fontWeight={500}>
        {formatTickValue(max, configuredUnit)}
      </text>
      <GaugeValueText
        x={cx} y={cy + 10} valueText={valueText} unit={unit}
        valueSize={24} unitSize={12} hasValue={hasValue}
        unitClassName="fill-(--color-text-muted)"
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

/** 4. Circular Needle (원형 니들) — 360° + 니들 */
function NeedleGauge({ value, valueText, min, max, unit, configuredUnit, thresholds, hasValue, showThresholdZones, valueColor: color, baseColor, needleColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 100, cy = 100, r = 80;
  const needleAngle = ratio * 360;
  const needleEnd = polarToCartesian(cx, cy, r - 14, needleAngle);

  const ticks = Array.from({ length: 11 }, (_, i) => i);
  // 외곽 링을 임계 구간 색으로 나눈다. 링은 r 을 중심으로 두께 10 이다.
  const ringOuterR = r + 5;
  const ringInnerR = r - 5;
  const useZones = showThresholdZones && thresholds.length > 0;

  return (
    <svg viewBox="0 0 200 200" className="h-full w-full">
      {/*
        외곽 링 — 임계 구간이 있으면 링 **자체**를 구간 색으로 나눈다.
        종전에는 링이 값 색 한 가지로 통칠해져, 값이 어느 구간에 있는지는 안쪽 얇은
        띠를 따로 봐야 알 수 있었다. 링 하나에서 구간과 값을 함께 읽게 한다.
      */}
      {useZones ? (
        <ThresholdTrackArc
          cx={cx} cy={cy} outerR={ringOuterR} innerR={ringInnerR}
          startAngle={0} totalAngle={360}
          thresholds={thresholds} min={min} max={max}
          ratio={hasValue ? ratio : 0} baseColor={baseColor}
          mode={'zone'}
        />
      ) : (
        <circle cx={cx} cy={cy} r={r} fill="none" stroke={color} strokeWidth={10} />
      )}
      {/* 내부 원 */}
      <circle cx={cx} cy={cy} r={r - 10} fill="none" stroke="#E2E8F0" strokeWidth={1} />
      {/* 눈금 + 라벨 */}
      {ticks.map((i) => {
        const angle = (i / 10) * 360;
        const tickStart = polarToCartesian(cx, cy, r - 2, angle);
        const tickEnd = polarToCartesian(cx, cy, r - 10, angle);
        const labelPos = polarToCartesian(cx, cy, r - 22, angle);
        const tickValue = min + ((max - min) * i) / 10;
        return (
          <g key={i}>
            <line x1={tickStart.x} y1={tickStart.y} x2={tickEnd.x} y2={tickEnd.y}
              stroke={color} strokeWidth={2} />
            <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
              className="fill-(--color-text-muted)" fontSize={7} fontWeight={500}>
              {formatTickValue(tickValue, configuredUnit)}
            </text>
          </g>
        );
      })}
      {/* 니들 — 값이 있을 때만. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue && (
        <>
          <line x1={cx} y1={cy} x2={needleEnd.x} y2={needleEnd.y}
            className={needleColor === '' ? 'stroke-(--color-text-primary)' : undefined}
            stroke={needleColor === '' ? undefined : needleColor}
            strokeWidth={2} strokeLinecap="round" />
          <circle cx={cx} cy={cy} r={5}
            className={needleColor === '' ? 'fill-(--color-text-primary)' : undefined}
            fill={needleColor === '' ? undefined : needleColor} />
        </>
      )}
      {!hasValue && (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      {/* 값 배지 — 흰 텍스트와의 대비를 위해 항상 어두운 배경 유지 */}
      <rect x={cx - 26} y={cy + 28} width={52} height={20} rx={4} fill="#1E293B" />
      <GaugeValueText
        x={cx} y={cy + 38} valueText={valueText} unit={unit}
        valueSize={10} unitSize={7} hasValue={hasValue}
        valueFill="#FFFFFF" unitOpacity={0.7}
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

/** 5. Needle Rainbow (레인보우) — 270° 속도계 스타일 */
function NeedleRainbowGauge({ value, valueText, min, max, unit, configuredUnit, thresholds, hasValue, showThresholdZones, baseColor, needleColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const cx = 110, cy = 105, outerR = 85, innerR = 75;
  const startAngle = 225; // 7시 방향 시작 (하단 열림)
  const totalAngle = 270;
  const needleAngle = startAngle + ratio * totalAngle;

  // 이 게이지에서 임계값 영역은 **색 아크 그 자체**다 — 안쪽에 부채꼴을 겹쳐 그리면
  // 같은 구간이 두 번 그려진다. 그래서 토글은 "아크를 임계값으로 그릴 것인가" 를 켠다.
  // 끄면 기본 3구간 팔레트로 돌아간다(임계값 자체는 값 색에 그대로 쓰인다).
  const useThresholds = showThresholdZones && thresholds.length > 0;
  const segments = useThresholds
    ? thresholds.map((t) => ({
        color: t.color,
        startRatio: normalize(t.from, min, max),
        endRatio: normalize(t.to, min, max),
      }))
    : [
        { color: '#2BBDB1', startRatio: 0, endRatio: 0.5 },
        { color: '#E8943A', startRatio: 0.5, endRatio: 0.8 },
        { color: '#8B8055', startRatio: 0.8, endRatio: 1 },
      ];

  const needleTip = polarToCartesian(cx, cy, outerR - 4, needleAngle);
  const needleBase1 = polarToCartesian(cx, cy, 6, needleAngle + 90);
  const needleBase2 = polarToCartesian(cx, cy, 6, needleAngle - 90);

  // 눈금 수
  const tickCount = 10;
  const labelCount = Math.min(11, Math.ceil((max - min) / ((max - min) / 10)) + 1);

  return (
    <svg viewBox="0 0 220 210" className="h-full w-full">
      {/* 임계값을 쓰면 다른 유형과 **같은 트랙 규약**을 쓴다 — 기본색 채움 + 값까지
          구간 색 + 링 전체 테두리. 쓰지 않으면 종전 무지개 팔레트 아크다. */}
      {useThresholds ? (
        <ThresholdTrackArc
          cx={cx} cy={cy} outerR={outerR} innerR={innerR}
          startAngle={startAngle} totalAngle={totalAngle}
          thresholds={thresholds} min={min} max={max}
          ratio={hasValue ? ratio : 0} baseColor={baseColor}
          mode={'zone'}
        />
      ) : (
        segments.map((seg, i) => {
          const sAngle = startAngle + seg.startRatio * totalAngle;
          const eAngle = startAngle + seg.endRatio * totalAngle;
          if (eAngle - sAngle < 0.5) return null;
          const midR = (outerR + innerR) / 2;
          return (
            <path key={i}
              d={describeArc(cx, cy, midR, sAngle, eAngle)}
              fill="none" stroke={seg.color} strokeWidth={outerR - innerR} />
          );
        })
      )}
      {/* 눈금 */}
      {Array.from({ length: tickCount * 2 + 1 }, (_, i) => {
        const angle = startAngle + (i / (tickCount * 2)) * totalAngle;
        const isMajor = i % 2 === 0;
        const s = polarToCartesian(cx, cy, outerR, angle);
        const e = polarToCartesian(cx, cy, isMajor ? innerR : innerR + 4, angle);
        return (
          <line key={i} x1={s.x} y1={s.y} x2={e.x} y2={e.y}
            stroke="#FFFFFF" strokeWidth={isMajor ? 2 : 1} />
        );
      })}
      {/* 숫자 라벨 */}
      {Array.from({ length: labelCount }, (_, i) => {
        const angle = startAngle + (i / (labelCount - 1)) * totalAngle;
        const pos = polarToCartesian(cx, cy, outerR + 14, angle);
        const labelVal = min + ((max - min) * i) / (labelCount - 1);
        return (
          <text key={i} x={pos.x} y={pos.y} textAnchor="middle" dominantBaseline="central"
            className="fill-(--color-text-muted)" fontSize={7} fontWeight={600}>
            {formatTickValue(labelVal, configuredUnit)}
          </text>
        );
      })}
      {/* 니들 — 값이 있을 때만 표시. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue && (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            className={needleColor === '' ? 'fill-(--color-text-primary)' : undefined}
            fill={needleColor === '' ? undefined : needleColor}
          />
          <circle cx={cx} cy={cy} r={6}
            className={needleColor === '' ? 'fill-(--color-text-primary)' : undefined}
            fill={needleColor === '' ? undefined : needleColor} />
        </>
      )}
      {!hasValue && (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      {/* 값 텍스트 */}
      <GaugeValueText
        x={cx} y={cy + 24} valueText={valueText} unit={unit}
        valueSize={12} unitSize={8} hasValue={hasValue}
        unitClassName="fill-(--color-text-muted)"
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

/** 9. Vertical Bar Gauge (세로 바) */
function VerticalBarGauge({ value, valueText, min, max, unit, configuredUnit, thresholds, hasValue, showThresholdZones, valueColor: color, baseColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const barW = 48, barH = 180, x = 60, y = 10;
  const fillH = Math.max(0, ratio * barH);

  // 트랙(바)을 임계 구간으로 나눈다. 도넛·반원·바늘과 **같은 계산**을 쓴다 — 축이
  // 각도가 아니라 선형일 뿐, "값까지 진하게 / 그 뒤 연하게" 규약은 같아야 한다.
  const useZones = showThresholdZones && thresholds.length > 0;
  const segments = useZones
    ? trackThresholdSegments(thresholds, min, max, hasValue ? ratio : 0)
    : [];

  // min~max 기반 5단계 눈금
  const tickCount = 5;
  const ticks = Array.from({ length: tickCount + 1 }, (_, i) =>
    min + ((max - min) * i) / tickCount,
  );

  return (
    // 값이 두 행이 되면서 둘째 행(단위)이 종전 높이 210 을 넘어 잘리고 타일 캡션과
    // 겹쳤다. 바 크기는 그대로 두고 캔버스 아래를 넓힌다.
    <svg viewBox="0 0 140 224" className="h-full w-full">
      {/* 기본색 채움 — 아크형의 `ThresholdTrackArc` 와 같은 규약이다. 축이 선형이라
          도형만 사각형이고, 그리는 순서(채움 → 값까지 구간 색 → 전체 테두리)는 같다. */}
      {baseColor !== '' && (
        <rect x={x} y={y} width={barW} height={barH} rx={3} fill={baseColor} />
      )}
      {/* 값까지 구간 색으로 채운다. 아래(min)에서 위(max)로 쌓이므로 y 는 위쪽 끝에서 잰다. */}
      {segments.filter((seg) => seg.filled).map((seg, i) => {
        const segY = y + barH - seg.endRatio * barH;
        const segH = (seg.endRatio - seg.startRatio) * barH;
        if (!(segH > 0)) return null;
        return (
          <rect key={`f${i}`} x={x} y={segY} width={barW} height={segH} fill={seg.color}
            opacity={TRACK_FILLED_OPACITY} />
        );
      })}
      {/* 임계 구간이 없으면 종전대로 값 바 하나로 표시한다. */}
      {!useZones && hasValue && fillH > 0 && (
        <rect x={x} y={y + barH - fillH} width={barW} height={fillH} fill={color} />
      )}
      {/* Y축 눈금 */}
      {ticks.map((t) => {
        const tickRatio = normalize(t, min, max);
        const ty = y + barH - tickRatio * barH;
        return (
          <g key={t}>
            <line x1={x} y1={ty} x2={x + barW} y2={ty} stroke="#FFFFFF" strokeWidth={0.5} opacity={0.3} />
            <text x={x - 6} y={ty + 3} textAnchor="end"
              className="fill-(--color-text-muted)" fontSize={8} fontWeight={500}>
              {formatTickValue(t, configuredUnit)}
            </text>
          </g>
        );
      })}
      <GaugeValueText
        x={x + barW / 2} y={y + barH + 16} valueText={valueText} unit={unit}
        valueSize={13} unitSize={8} hasValue={hasValue}
        unitClassName="fill-(--color-text-muted)"
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

/** 반원 RB 게이지에서 반원이 향하는 쪽. */
export type HalfRainbowDirection = 'up' | 'down' | 'left' | 'right';

/** 방향 목록(설정 UI 와 공유하는 단일 정본). */
export const HALF_RAINBOW_DIRECTIONS: readonly HalfRainbowDirection[] = [
  'up',
  'down',
  'left',
  'right',
];

/**
 * 방향별 캔버스 배치.
 *
 * **방향마다 캔버스가 다른 이유**: 반원은 원의 절반이라 방향에 따라 차지하는 상자가
 * 가로로 넓거나(위·아래) 세로로 길다(왼쪽·오른쪽). 한 캔버스를 돌려 쓰면 한쪽 방향이
 * 반드시 잘린다 — 종전 기본값(`startAngle: 180`, 왼쪽 반원)이 240×150 캔버스에서
 * 아래쪽 40px 을 넘겨 잘려 있던 것이 그 사고다.
 *
 * `valueDx`/`valueDy` 는 중심에서 값 텍스트까지의 변위다. 값은 항상 **열린 쪽**에
 * 둔다 — 반원이 감싸는 반대편이 비는 자리이고, 거기 두어야 아크와 겹치지 않는다.
 *
 * 각도는 12시가 0° 이고 시계방향으로 증가한다(`polarToCartesian`).
 */
const HALF_RAINBOW_LAYOUT: Record<
  HalfRainbowDirection,
  { viewBox: string; cx: number; cy: number; startAngle: number; valueDx: number; valueDy: number }
> = {
  // 9시 → 3시. 위로 볼록한 속도계 형태.
  up: { viewBox: '0 0 240 150', cx: 120, cy: 110, startAngle: 270, valueDx: 0, valueDy: 18 },
  // 3시 → 9시. 아래로 볼록한 그릇 형태 — 값은 위쪽 빈자리에 둔다.
  down: { viewBox: '0 0 240 150', cx: 120, cy: 45, startAngle: 90, valueDx: 0, valueDy: -22 },
  // 6시 → 12시. 왼쪽 반원 — 열린 쪽이 오른쪽이다.
  left: { viewBox: '0 0 200 200', cx: 90, cy: 100, startAngle: 180, valueDx: 40, valueDy: 0 },
  // 12시 → 6시. 오른쪽 반원 — 열린 쪽이 왼쪽이다.
  right: { viewBox: '0 0 200 200', cx: 110, cy: 100, startAngle: 0, valueDx: -40, valueDy: 0 },
};

/** 저장된 값을 방향으로 읽는다. 모르는 값·미지정은 기본(위쪽)이다. */
export function readHalfRainbowDirection(raw: unknown): HalfRainbowDirection {
  return HALF_RAINBOW_DIRECTIONS.includes(raw as HalfRainbowDirection)
    ? (raw as HalfRainbowDirection)
    : 'up';
}

/** 10. Half Rainbow 2 (5단계 등급) */
function HalfRainbowGauge({ value, valueText, min, max, unit, thresholds, hasValue, showThresholdZones, halfRainbowDirection, baseColor, needleColor, valueScale, valueOffsetX, valueOffsetY }: ReturnType<typeof parseConfig> & { hasValue: boolean }) {
  const ratio = normalize(value, min, max);
  const layout = HALF_RAINBOW_LAYOUT[halfRainbowDirection];
  const { cx, cy, startAngle } = layout;
  const outerR = 80, innerR = 56;
  const totalAngle = 180;

  // 니들 RB 와 같은 규약 — 여기서도 임계값 영역은 세그먼트 아크 그 자체다.
  // 끄면 기본 5단계 등급 팔레트로 돌아간다.
  const useThresholds = showThresholdZones && thresholds.length > 0;
  const segments = useThresholds
    ? thresholds.map((t) => ({
        color: t.color,
        label: t.name,
        startRatio: normalize(t.from, min, max),
        endRatio: normalize(t.to, min, max),
      }))
    : [
        { color: '#EF4444', label: 'VERY POOR', startRatio: 0, endRatio: 0.2 },
        { color: '#F97316', label: 'POOR', startRatio: 0.2, endRatio: 0.4 },
        { color: '#EAB308', label: 'FAIR', startRatio: 0.4, endRatio: 0.6 },
        { color: '#22C55E', label: 'GOOD', startRatio: 0.6, endRatio: 0.8 },
        { color: '#3B82F6', label: 'EXCELLENT', startRatio: 0.8, endRatio: 1 },
      ];

  const needleAngle = startAngle + ratio * totalAngle;
  const needleTip = polarToCartesian(cx, cy, outerR - 4, needleAngle);
  const needleBase1 = polarToCartesian(cx, cy, 5, needleAngle + 90);
  const needleBase2 = polarToCartesian(cx, cy, 5, needleAngle - 90);

  return (
    <svg viewBox={layout.viewBox} className="h-full w-full">
      {/* 임계값을 쓰면 다른 유형과 같은 트랙 규약을 쓴다(기본색 채움 + 값까지 구간 색
          + 반원 전체 테두리). 쓰지 않으면 종전 5단계 등급 팔레트다. */}
      {useThresholds && (
        <ThresholdTrackArc
          cx={cx} cy={cy} outerR={outerR} innerR={innerR}
          startAngle={startAngle} totalAngle={totalAngle}
          thresholds={thresholds} min={min} max={max}
          ratio={hasValue ? ratio : 0} baseColor={baseColor}
          mode={'zone'}
        />
      )}
      {/* 세그먼트 아크 — 임계값을 쓰지 않을 때의 기본 등급 표시 */}
      {!useThresholds && segments.map((seg, i) => {
        const sAngle = startAngle + seg.startRatio * totalAngle;
        const eAngle = startAngle + seg.endRatio * totalAngle;
        if (eAngle - sAngle < 0.5) return null;
        return (
          <g key={i}>
            <path d={describeDonutArc(cx, cy, outerR, innerR, sAngle, eAngle)} fill={seg.color} />
            {/* 구간 라벨 */}
            {seg.label && (() => {
              const midAngle = (sAngle + eAngle) / 2;
              const labelPos = polarToCartesian(cx, cy, (outerR + innerR) / 2, midAngle);
              return (
                <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
                  fill="#FFFFFF" fontSize={6} fontWeight={600}>
                  {seg.label}
                </text>
              );
            })()}
          </g>
        );
      })}
      {/* 니들 — 값이 있을 때만. 다크모드에서도 보이도록 text-primary 사용. */}
      {hasValue ? (
        <>
          <polygon
            points={`${needleTip.x},${needleTip.y} ${needleBase1.x},${needleBase1.y} ${needleBase2.x},${needleBase2.y}`}
            className={needleColor === '' ? 'fill-(--color-text-primary)' : undefined}
            fill={needleColor === '' ? undefined : needleColor}
          />
          <circle cx={cx} cy={cy} r={6}
            className={needleColor === '' ? 'fill-(--color-text-primary)' : undefined}
            fill={needleColor === '' ? undefined : needleColor} />
        </>
      ) : (
        <circle cx={cx} cy={cy} r={4} fill="#9CA3AF" />
      )}
      <circle cx={cx} cy={cy} r={3.5} className="fill-(--color-bg-surface)" />
      {/* 값은 열린 쪽에 둔다 — 방향마다 그 자리가 다르다(HALF_RAINBOW_LAYOUT). */}
      <GaugeValueText
        x={cx + layout.valueDx} y={cy + layout.valueDy}
        valueText={valueText} unit={unit}
        valueSize={18} unitSize={11} hasValue={hasValue}
        unitClassName="fill-(--color-text-muted)"
        scale={valueScale} offsetX={valueOffsetX} offsetY={valueOffsetY}
      />
    </svg>
  );
}

// ---- 게이지 타입 디스패치 ----

/**
 * 게이지 타입별 렌더러 선택. 단일 출력(레거시)과 다중 출력(M4)이 **같은** 디스패치를
 * 공유하도록 컴포넌트 밖으로 끌어냈다 — 두 벌로 나뉘면 게이지 타입이 하나 늘 때마다
 * 한쪽만 고치는 사고가 난다.
 */
export function renderGaugeByType(
  parsed: ReturnType<typeof parseConfig>,
  hasValue: boolean,
): ReactElement {
  switch (parsed.gaugeType) {
    case 'simple':
      return <SimpleGauge {...parsed} hasValue={hasValue} />;
    case 'half':
      return <HalfGauge {...parsed} hasValue={hasValue} />;
    case 'needle':
      return <NeedleGauge {...parsed} hasValue={hasValue} />;
    case 'needle-rainbow':
      return <NeedleRainbowGauge {...parsed} hasValue={hasValue} />;
    case 'vertical-bar':
      return <VerticalBarGauge {...parsed} hasValue={hasValue} />;
    case 'half-rainbow':
      return <HalfRainbowGauge {...parsed} hasValue={hasValue} />;
    default:
      return <SimpleGauge {...parsed} hasValue={hasValue} />;
  }
}
