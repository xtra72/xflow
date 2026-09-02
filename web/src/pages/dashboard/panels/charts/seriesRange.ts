// 가져올 데이터 범위 — 기간(상대·절대) 또는 갯수.
//
// Store · TSDB 가 공유한다(차트·테이블 모든 패널이 같은 어휘를 쓴다). 의존성 없는
// 순수 모듈이라 훅 없이 전수 테스트할 수 있다.

/** 범위 지정 방식. */
export type SeriesRangeMode = 'relative' | 'absolute' | 'count';

/**
 * 가져올 데이터 범위.
 *
 * 세 방식이 한 오브젝트에 모여 있고 `mode` 가 어느 항을 읽을지 정한다. 방식을 바꿔도
 * 다른 방식의 값이 남아 있어, 되돌렸을 때 이전 설정이 그대로 살아난다.
 */
export interface SeriesRange {
  mode: SeriesRangeMode;
  /** relative: 현재부터 얼마 전까지(ms). */
  window_ms?: number;
  /** absolute: 시작 epoch ms(포함). */
  start_ms?: number;
  /** absolute: 끝 epoch ms(포함). */
  end_ms?: number;
  /** count: 가장 최근 버킷 N개. */
  count?: number;
}

/** 갯수 방식의 기본값·상한. 상한은 한 번의 조회가 만드는 버킷 수를 제한한다. */
export const DEFAULT_RANGE_COUNT = 100;
export const MAX_RANGE_COUNT = 10_000;

/** 해석된 조회 구간. */
export interface ResolvedWindow {
  startMs: number;
  endMs: number;
  /**
   * 조회 후 남길 **가장 최근 버킷 수**. 갯수 방식에서만 값이 있다.
   *
   * 갯수 방식도 조회 자체는 구간으로 나간다(조회 API 가 `time_range` 뿐이다).
   * `interval × count` 로 구간을 역산해 보내고, 경계 정렬 때문에 버킷이 하나 더
   * 딸려 올 수 있어 받은 뒤 마지막 N개로 자른다.
   */
  limitBuckets?: number;
}

/**
 * 저장된 config 에서 범위를 읽는다.
 *
 * `range` 가 없는 구 config 는 기존 `time_window_ms` 를 상대 기간으로 해석한다 —
 * 이 폴백이 있어 기존 패널은 마이그레이션 없이 종전과 똑같이 동작한다.
 */
export function readSeriesRange(
  range: SeriesRange | undefined,
  timeWindowMs: number,
): SeriesRange {
  if (!range || !range.mode) return { mode: 'relative', window_ms: timeWindowMs };
  return range;
}

/**
 * 범위를 실제 조회 구간으로 해석한다.
 *
 * 해석 불가(값이 비었거나 앞뒤가 뒤집힌 절대 구간, 0 이하 갯수)면 `undefined` 를
 * 반환한다 — 호출부는 조회를 시작하지 않는다. 여기서 임의 기본값으로 메우면
 * 사용자가 잘못 넣은 값이 "왜 다른 구간이 보이지" 로 이어진다.
 */
export function resolveSeriesWindow(
  range: SeriesRange,
  intervalMs: number,
  now: number,
): ResolvedWindow | undefined {
  switch (range.mode) {
    case 'relative': {
      const w = range.window_ms ?? 0;
      if (!(w > 0)) return undefined;
      return { startMs: now - w, endMs: now };
    }
    case 'absolute': {
      const s = range.start_ms ?? 0;
      const e = range.end_ms ?? 0;
      if (!(s > 0) || !(e > 0) || e < s) return undefined;
      return { startMs: s, endMs: e };
    }
    case 'count': {
      const n = range.count ?? 0;
      if (!(n > 0) || !(intervalMs > 0)) return undefined;
      const capped = Math.min(n, MAX_RANGE_COUNT);
      return { startMs: now - capped * intervalMs, endMs: now, limitBuckets: capped };
    }
    default:
      return undefined;
  }
}

/**
 * 범위를 폴링 재시작 키의 일부로 직렬화한다.
 *
 * **절대 구간이 아니면 시각을 넣지 않는다.** 상대·갯수는 매 폴링마다 `now` 가
 * 달라지므로 해석된 구간을 키에 넣으면 키가 계속 바뀌어 폴링이 재시작된다.
 */
export function seriesRangeKey(range: SeriesRange): string {
  switch (range.mode) {
    case 'relative':
      return `rel:${range.window_ms ?? 0}`;
    case 'absolute':
      return `abs:${range.start_ms ?? 0}-${range.end_ms ?? 0}`;
    case 'count':
      return `cnt:${range.count ?? 0}`;
    default:
      return 'none';
  }
}

/** 갯수 방식에서 마지막 N개만 남긴다. 그 밖의 방식은 원본을 그대로 돌려준다. */
export function limitToRecent<T>(rows: readonly T[], limitBuckets: number | undefined): T[] {
  if (limitBuckets === undefined || rows.length <= limitBuckets) return rows.slice();
  return rows.slice(rows.length - limitBuckets);
}

/** 상대 기간 ms 를 사람이 읽는 눈금으로 표기한다(5m · 1h · 7d). */
export function formatWindowMs(ms: number): string {
  if (ms % 86_400_000 === 0) return `${ms / 86_400_000}d`;
  if (ms % 3_600_000 === 0) return `${ms / 3_600_000}h`;
  if (ms % 60_000 === 0) return `${ms / 60_000}m`;
  return `${Math.round(ms / 1000)}s`;
}

// ── 라인 차트 X축 범위 ────────────────────────────────────────────────────
//
// 채널(실시간) 모드의 X축은 오랫동안 별도 어휘를 썼다 — `time_window_mode`
// (points / recent / fixed) + max_points / recent_window_sec / fixed_start_ms /
// fixed_end_ms. 데이터 소스(Store · TSDB)가 쓰는 SeriesRange 와 뜻이 같은데
// 이름만 달라, 같은 개념이 화면마다 다르게 보였다.
//
// 두 어휘는 1:1 로 대응한다:
//
//     points ↔ count      recent ↔ relative      fixed ↔ absolute
//
// 아래 두 함수가 그 대응을 한 곳에 모은다. 기존 패널은 마이그레이션 없이
// 종전과 똑같이 동작한다 — `x_range` 가 없으면 구 필드를 읽어 해석한다.

/** 채널 모드 X축의 기본 포인트 수. 구 `max_points` 기본값과 같다. */
export const DEFAULT_CHART_POINTS = 100;

/** 구 X축 시간창 어휘. 새 config 는 쓰지 않지만 읽기 폴백에 필요하다. */
export type LegacyTimeWindowMode = 'points' | 'recent' | 'fixed';

/** `readChartXRange` 가 읽는 config 조각. 패널 config 의 부분집합이다. */
export interface ChartXRangeSource {
  x_range?: SeriesRange;
  time_window_mode?: LegacyTimeWindowMode;
  max_points?: number;
  recent_window_sec?: number;
  fixed_start_ms?: number;
  fixed_end_ms?: number;
}

/**
 * 라인 차트 X축 범위를 읽는다.
 *
 * `x_range` 가 있으면 그대로 쓴다. 없으면 구 `time_window_mode` 계열을 해석한다 —
 * 이 폴백이 있어 저장된 패널을 건드리지 않고도 새 어휘로 읽을 수 있다.
 *
 * 구 config 에서 `time_window_mode` 자체가 없으면 'points' 로 본다(구 기본값).
 */
export function readChartXRange(cfg: ChartXRangeSource): SeriesRange {
  if (cfg.x_range?.mode) return cfg.x_range;

  switch (cfg.time_window_mode ?? 'points') {
    case 'recent':
      return {
        mode: 'relative',
        // 구 필드는 초 단위였다. 값이 없으면 해석 불가로 두지 않고 기본 10분을 쓴다 —
        // 구 렌더러의 DEFAULT_RECENT_WINDOW_SEC 와 같은 값이라 화면이 바뀌지 않는다.
        window_ms: (cfg.recent_window_sec ?? 600) * 1000,
      };
    case 'fixed':
      return {
        mode: 'absolute',
        start_ms: cfg.fixed_start_ms,
        end_ms: cfg.fixed_end_ms,
      };
    default:
      return { mode: 'count', count: cfg.max_points ?? DEFAULT_CHART_POINTS };
  }
}

/** X축 범위가 잘라 낼 시간 구간. 갯수 방식은 시간으로 자르지 않는다. */
export interface ChartXWindow {
  startMs: number;
  endMs: number;
}

/**
 * X축 범위를 실제로 자를 구간으로 해석한다.
 *
 * 갯수 방식은 `undefined` 를 돌려준다 — 채널 버퍼는 시간이 아니라 개수로 잘린다
 * (버퍼 상한은 `chartXRangePoints`). 절대 구간에서 한쪽이 비어 있으면 그쪽은
 * 열어 둔다(구 `filterByTimeWindow` 의 ±Infinity 규약을 그대로 옮긴 것).
 */
export function resolveChartXWindow(
  range: SeriesRange,
  now: number,
): ChartXWindow | undefined {
  switch (range.mode) {
    case 'relative': {
      const w = range.window_ms ?? 0;
      if (!(w > 0)) return undefined;
      return { startMs: now - w, endMs: now };
    }
    case 'absolute':
      return {
        startMs: range.start_ms ?? Number.NEGATIVE_INFINITY,
        endMs: range.end_ms ?? Number.POSITIVE_INFINITY,
      };
    default:
      return undefined;
  }
}

/**
 * 채널 버퍼에 담아 둘 포인트 수를 정한다.
 *
 * 갯수 방식이면 그 값이 곧 버퍼 크기다. 상대 기간이면 1Hz 수신을 가정해 기간의
 * 2배를 잡는다 — 버퍼가 기간보다 짧으면 차트가 중간에서 끊겨 보인다. 상한 5,000
 * 은 메모리 폭주를 막는다. (구 `resolveMaxPoints` 규약을 그대로 옮긴 것.)
 */
export function chartXRangePoints(range: SeriesRange): number {
  if (range.mode === 'count' && range.count && range.count > 0) {
    return Math.min(MAX_RANGE_COUNT, range.count);
  }
  if (range.mode === 'relative' && range.window_ms && range.window_ms > 0) {
    const sec = range.window_ms / 1000;
    return Math.min(5000, Math.max(DEFAULT_CHART_POINTS, Math.round(sec * 2)));
  }
  return DEFAULT_CHART_POINTS;
}

/** 실시간 누적이 무엇을 남길지 — 시간 창과 점 개수 상한. */
export interface LiveRetention {
  /** 이보다 오래된 점을 버린다(ms). 시간으로 자르지 않으면 `Infinity`. */
  windowMs: number;
  /** 계열당 남길 점의 최대 개수. */
  maxPoints: number;
}

/**
 * 조회 범위를 **실시간 누적의 보관 기준**으로 해석한다.
 *
 * 실시간에는 버킷도 집계도 없다 — 스냅샷을 폴링해 오는 대로 쌓을 뿐이다. 그래서 범위는
 * "무엇을 질의할지" 가 아니라 "쌓아 둔 것 중 무엇을 남길지" 가 된다. 뜻이 이렇게 갈리므로
 * 조회 창 해석(`resolveSeriesWindow`)을 그대로 쓸 수 없다.
 *
 *   - `relative` — 그 기간보다 오래된 점을 버린다. 가장 곧은 대응이다.
 *   - `count`    — 시간으로 자르지 않고 최근 N개만 남긴다.
 *   - `absolute` — 시작·끝이 정해진 구간은 "지금부터 쌓는" 모델과 맞지 않는다. 구간의
 *     **길이**만 취해 상대 창처럼 다룬다 — 과거의 절대 구간을 실시간으로 채울 수는 없고,
 *     그렇다고 아무것도 남기지 않으면 빈 차트가 된다.
 *
 * `hardMaxPoints` 는 메모리 상한이다. 창이 아주 길거나 갯수가 크게 잡혀도 이 값을 넘지
 * 않는다 — 창은 사용자가 정하지만 브라우저가 버틸 수 있는 양은 그렇지 않다.
 */
export function resolveLiveRetention(range: SeriesRange, hardMaxPoints: number): LiveRetention {
  const positive = (v: number | undefined): number | undefined =>
    typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined;

  switch (range.mode) {
    case 'count': {
      const n = positive(range.count);
      return { windowMs: Infinity, maxPoints: Math.min(n ?? hardMaxPoints, hardMaxPoints) };
    }
    case 'absolute': {
      const span =
        positive(range.start_ms) !== undefined && positive(range.end_ms) !== undefined
          ? range.end_ms! - range.start_ms!
          : undefined;
      return { windowMs: positive(span) ?? Infinity, maxPoints: hardMaxPoints };
    }
    default:
      return { windowMs: positive(range.window_ms) ?? Infinity, maxPoints: hardMaxPoints };
  }
}
