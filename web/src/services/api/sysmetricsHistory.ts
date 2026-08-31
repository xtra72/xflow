// sysmetrics 이력 조회 — 에이전트 버퍼를 시리즈 매트릭스로 옮긴다.
//
// 에이전트가 설정된 기간만큼 표본을 들고 있고(`sysmetrics_history.go`), 패널은 그
// 이력을 **질의**한다. 브라우저가 점을 누적하던 종전 방식과 달리 패널을 열자마자 과거
// 구간이 그려지고, 닫았다 열어도 선이 초기화되지 않는다.
//
// 누적 카운터(네트워크·디스크 I/O)는 **에이전트가 이미 초당 증가량으로 환산**해 둔다.
// 그래서 여기서는 숫자 시계열을 버킷으로 접기만 하면 되고, 증가량 단위 같은 이 소스
// 전용 축이 화면에 남지 않는다 — 패널 UI 가 Store · TSDB 와 같아지는 근거다.
//
// 같은 카운터의 **누적 원값**은 `mode: 'total'` 시리즈로 함께 온다. 증가량과는 다른
// 시리즈이며 `mode` 태그로 갈린다 — 기본 표현(증가량)에는 이 태그가 없다.
//
// 피벗은 소스 중립 기계(`buildSeriesMatrix`)를 그대로 쓴다. 소스마다 다른 것은 "키별로
// 시리즈를 어떻게 가져오는가" 하나뿐이다.

import { queryAgent } from './agentService';
import { buildSeriesMatrix, type PivotKeySeries } from './seriesMatrixPivot';
import { makeSeriesId } from './seriesLabels';
import {
  aggregateValues,
  type SeriesMatrix,
  type SeriesMatrixQuery,
} from './seriesDataSource';

/** 에이전트가 돌려주는 시리즈 동일성. Go `SysMetricsSeriesKey` 와 1:1 이다. */
export interface SysmetricsHistorySeries {
  /** 필드 이름(`bytes_recv`). Store 어휘의 measurement 다. */
  measurement: string;
  /** 지표 분류(`network`). */
  category: string;
  /** 인스턴스 대상(`en0`). 없으면 종합이다. */
  target?: string;
  /**
   * 표현 방식. **없으면 그 분류의 기본 표현**이다(카운터는 증가량, 나머지는 상태값).
   *
   * `'total'` 만 실려 온다 — 기본 표현에 이름을 싣지 않는 것은 호환 때문이다. 이미
   * 저장된 패널의 시리즈 태그에는 이 축이 없다(Go `SysMetricsSeriesKey.Mode`).
   */
  mode?: string;
}

/** 한 시점의 값 배열. `values[i]` 가 `series[i]` 에 대응하고, 없으면 null 이다. */
export interface SysmetricsHistoryRow {
  time_ms: number;
  values: Array<number | null>;
}

/** `get_history` 응답. 열 지향이다 — 점마다 키를 반복하면 응답이 키로 가득 찬다. */
export interface SysmetricsHistoryResult {
  series: SysmetricsHistorySeries[];
  points: SysmetricsHistoryRow[];
  /** 실제 보관 기간(초). 표본 수 상한에 걸리면 설정값보다 짧다. */
  retention_seconds: number;
  /** 표본 수 상한 때문에 오래된 표본을 버리고 있는가. */
  truncated: boolean;
}

/**
 * 에이전트 이력을 조회한다.
 *
 * `POST /agents/{id}/query` 를 쓴다 — 전용 라우트를 내지 않는 이유는 그 엔드포인트가
 * 이미 화이트리스트 + `agent.read` 권한 모델을 갖고 있기 때문이다.
 *
 * **응답은 에이전트 결과 그 자체다.** 서버는 `{success, data}` 로 감싸 보내지만
 * 클라이언트 인터셉터(`client.ts`)가 그 봉투를 벗기므로, 여기 도달하는 값은 이미
 * `{series, points, ...}` 이다. `AgentExecResponse`(`{result}`) 타입은 실제 형상과
 * 어긋나 있어 다른 호출부들도 모두 캐스팅으로 우회한다 — 그 계층을 한 겹 더 벗기려
 * 들면 조용히 `undefined` 를 파싱해 **빈 차트**가 된다.
 */
export async function fetchSysmetricsHistory(
  agentId: string,
  startMs: number,
  endMs: number,
): Promise<SysmetricsHistoryResult> {
  const res = await queryAgent(agentId, {
    command: 'get_history',
    params: { start_ms: startMs, end_ms: endMs },
  });
  return parseHistoryResult(res);
}

/**
 * 응답 본문을 결과로 정규화한다. 형상이 어긋나면 **빈 결과**로 떨어진다.
 *
 * 던지지 않는 이유: 에이전트가 다른 타입이거나 구버전이면 형상이 다를 수 있고, 그때
 * 패널이 오류 오버레이를 띄우는 것보다 빈 차트가 옳다 — 소스를 잘못 고른 것은 설정
 * 화면에서 이미 드러난다.
 */
export function parseHistoryResult(raw: unknown): SysmetricsHistoryResult {
  const empty: SysmetricsHistoryResult = {
    series: [],
    points: [],
    retention_seconds: 0,
    truncated: false,
  };
  if (!raw || typeof raw !== 'object') return empty;
  const obj = raw as Record<string, unknown>;
  if (!Array.isArray(obj.series) || !Array.isArray(obj.points)) return empty;

  const series = obj.series.filter(
    (s): s is SysmetricsHistorySeries =>
      !!s && typeof s === 'object' && typeof (s as { measurement?: unknown }).measurement === 'string',
  );
  const points = obj.points.filter(
    (p): p is SysmetricsHistoryRow =>
      !!p &&
      typeof p === 'object' &&
      typeof (p as { time_ms?: unknown }).time_ms === 'number' &&
      Array.isArray((p as { values?: unknown }).values),
  );

  return {
    series,
    points,
    retention_seconds:
      typeof obj.retention_seconds === 'number' ? obj.retention_seconds : 0,
    truncated: obj.truncated === true,
  };
}

/** 이력 시리즈의 라벨 맵 — Store 어휘의 tags 와 같은 자리다. */
export function historySeriesTags(s: SysmetricsHistorySeries): Record<string, string> {
  const tags: Record<string, string> = { category: s.category };
  if (s.target !== undefined && s.target !== '') {
    // 태그 키는 분류에서 파생한다 — 에이전트가 대상 축 이름을 따로 싣지 않는다.
    tags[TAG_KEY_BY_CATEGORY[s.category] ?? 'target'] = s.target;
  }
  // 기본 표현에는 축을 싣지 않는다 — 요청 쪽(`sysmetricSeriesTags`)과 같은 규칙이라야
  // 두 식별자가 맞물린다.
  if (s.mode !== undefined && s.mode !== '') tags.mode = s.mode;
  return tags;
}

/**
 * 분류 → 인스턴스 태그 키.
 *
 * `sysmetricsSource.TAG_KEY_BY_GROUP` 과 같은 어휘다. 그쪽은 스냅샷 형상의 camelCase
 * 그룹으로 키잉하고, 여기는 에이전트가 싣는 분류 이름(`disk_io`)으로 키잉한다.
 */
const TAG_KEY_BY_CATEGORY: Record<string, string> = {
  network: 'interface',
  disk_io: 'device',
  storage: 'mountpoint',
};

/** 이력 시리즈의 결정적 식별자 — 요청 시리즈와 짝짓는 데 쓴다. */
export function historySeriesId(s: SysmetricsHistorySeries): string {
  return makeSeriesId(s.measurement, undefined, historySeriesTags(s));
}

/**
 * 이력을 시리즈 매트릭스로 옮긴다.
 *
 * 요청의 `keys[i]` + `seriesFilters[i].tags` 로 이력 시리즈를 찾아 버킷으로 접는다.
 * 버킷 집계는 Store 와 **같은 함수**(`aggregateValues`)를 쓴다 — 소스마다 다른 집계를
 * 두면 같은 설정이 소스에 따라 다른 수를 낸다.
 */
export async function querySysmetricsMatrix(
  agentId: string,
  params: SeriesMatrixQuery,
  signal?: AbortSignal,
): Promise<SeriesMatrix> {
  return buildSeriesMatrix(params, async (p) => {
    if (signal?.aborted) throw new DOMException('aborted', 'AbortError');
    const history = await fetchSysmetricsHistory(agentId, p.startMs, p.endMs);

    // 이력 시리즈를 식별자로 색인해 요청과 짝짓는다.
    const byId = new Map<string, number>();
    history.series.forEach((s, i) => byId.set(historySeriesId(s), i));

    return p.keys.map((key, idx) => {
      const tags = p.seriesFilters?.[idx]?.tags ?? {};
      const col = byId.get(makeSeriesId(key, undefined, tags));
      if (col === undefined) return [];
      return [pivotSeries(history, col, p)];
    });
  });
}

/**
 * 이력의 한 열을 버킷 맵으로 접는다.
 *
 * 버킷 경계는 epoch 0 격자(`floor(t / interval) * interval`)이며, 집계는 Store 와
 * 같은 함수(`aggregateValues`)를 쓴다 — 소스마다 경계나 집계가 다르면 같은 설정이
 * 소스에 따라 다른 그래프를 낸다.
 */
function pivotSeries(
  history: SysmetricsHistoryResult,
  col: number,
  params: SeriesMatrixQuery,
): PivotKeySeries {
  // 버킷 시작 → 그 버킷에 든 값들. 집계는 값이 다 모인 뒤에 한 번 한다.
  const collected = new Map<number, number[]>();
  for (const point of history.points) {
    const value = point.values[col];
    if (typeof value !== 'number' || !Number.isFinite(value)) continue;
    // epoch-zero 정렬 — Store 의 `bucketAndAggregate` 와 **같은 규칙**이다.
    // 조회 시작 시각을 기준으로 접으면, 상대 창은 폴링마다 startMs 가 달라지므로
    // 같은 표본이 폴링마다 다른 버킷에 들어가 점이 흔들린다.
    const bucket = Math.floor(point.time_ms / params.intervalMs) * params.intervalMs;
    const list = collected.get(bucket);
    if (list) list.push(value);
    else collected.set(bucket, [value]);
  }

  const buckets = new Map<number, number>();
  for (const [bucket, values] of collected) {
    const agg = aggregateValues(values, params.aggregation);
    if (agg !== null) buckets.set(bucket, agg);
  }
  // 라벨은 붙이지 않는다 — 요청 1건이 시리즈 1개이므로 컬럼명은 요청 key 가 된다.
  return { labels: undefined, buckets };
}
