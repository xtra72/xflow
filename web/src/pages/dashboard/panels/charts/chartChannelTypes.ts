// 차트 패널 공통 타입 + 값 추출 유틸.
// SPEC-CHART-001 §4.1 / §4.2.2 참조.
//
// SPEC-WEB-005 (차트 Store 소스): 차트 패널이 chart-emitter 채널 대신
// Store 에이전트에서 데이터를 가져올 수 있도록 `data_source` / `store_source`
// config 를 추가했다. 두 소스는 공존하며, `data_source` 미지정(undefined)은
// 기존 채널 경로('channel')로 해석되어 하위 호환을 보존한다.
// @spec SPEC-WEB-005

// Store 키 데이터 타입(int/float/string/boolean/bytes/json). type-only import 이므로
// 컴파일 시 erase 되어 런타임 순환 의존을 만들지 않는다(store.ts 는 chartChannelTypes 를
// import 하지 않는다).
import type { DataType } from '@/services/api/store';
// 표시 라벨 포맷은 매트릭스 컬럼명과 같은 곳(seriesLabels)에서 온다 — 같은 시리즈가 화면마다
// 다른 표기를 갖지 않도록 두 번째 포맷터를 만들지 않는다. 순수 모듈이라 순환 의존이 없다.
import { seriesRefDisplayName } from '@/services/api/seriesLabels';

import { resolveSeriesAlias, type AliasContext } from './aliasTemplate';

/** 단일 차트 항목 (WS 로 전송되는 entry) */
import type { SeriesRange } from './seriesRange';

export interface ChartEntry {
  /** epoch milliseconds (int64) */
  timestamp: number;
  /** 값 — 숫자/문자열/객체 모두 허용 */
  value: unknown;
  /** 선택적 라벨 (bar/pie 의 카테고리) */
  labels?: Record<string, string>;
  /** 디버깅/추적용 메타데이터 */
  meta?: Record<string, unknown>;
}

/** 연결 상태 (REQ-M4-09) */
export type ChartConnectionStatus =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'closed'
  | 'error';

/**
 * 차트 패널의 데이터 소스 종류.
 *
 * - `channel`: 기존 chart-emitter WebSocket 채널 경로(기본값).
 * - `store`: Store 에이전트(인메모리 TSDB)의 시리즈 매트릭스 폴링 경로.
 * - `tsdb`: **에이전트를 통해 접근하는 외부 시계열 DB**(InfluxDB 가 첫 백엔드).
 *
 * memTSDB(`internal/tsdb/` · `/api/v1/tsdb/*`)는 플로우 노드 · WS 구독자용 내부
 * 설비이며 **패널 데이터소스가 아니다** — 이 유니온에 대응 값이 없다.
 * `SeriesDataSourceKind` 쪽 memTSDB 값은 `'memtsdb'` 다.
 *
 * @spec SPEC-WEB-005 · SPEC-TSDB-002 §2.1 (U1)
 */
export type ChartDataSourceKind = 'channel' | 'store' | 'tsdb';

/**
 * Store 소스에서 조회할 단일 시리즈 참조.
 *
 * `key` 는 Store 키 이름이고, `field`/`tags` 가 지정되면 해당 key 의
 * 특정 시리즈(저장소 기준 분류)로 좁혀 조회한다. 미지정이면 그 key 의 모든
 * 시리즈를 조회한다. `alias`/`color` 는 표시 전용이다.
 *
 * @spec SPEC-WEB-005
 */
export interface StoreSeriesRef {
  /** Store 키 이름. */
  key: string;
  /** 시리즈별 선택 시 field 필터(선택). */
  field?: string;
  /** 시리즈별 선택 시 tag 필터(선택). */
  tags?: Record<string, string>;
  /** 키 데이터 타입(표시/필터 메타데이터). */
  data_type?: DataType;
  /** 표시 별칭(미지정 시 key). */
  alias?: string;
  /**
   * 시리즈를 나눌 태그 키 목록(TSDB group by 축). @spec SPEC-TSDB-004 §2.1
   *
   * Store 소스는 이 축을 만들지 않지만, TSDB config 가 같은 어휘로 이 변환기를
   * 공유하므로(`asSeriesConfig`) 여기에도 둔다. 없으면 정확 일치 모드다.
   */
  group_by?: string[];
  /**
   * **그룹별** 개별 표시 이름. 키는 `group_by` 를 정렬한 순서의 조합 서명이다
   * (`groupComboSignature`). @spec SPEC-TSDB-004 §2.12
   *
   * `alias` 는 항목 하나에 이름 하나라 group by 로 펼쳐진 N개 그룹에 서로 다른
   * 이름을 줄 수 없다. 토큰 템플릿(`CPU {$.tags.host}`)은 규칙이 있는 이름만
   * 만들 수 있으므로, 그룹마다 임의의 이름을 붙이려면 별도 축이 필요하다.
   *
   * 우선순위: 그룹별 이름 > 항목 `alias` > 패널 형식 > 내장 서술 표기.
   */
  group_alias?: Record<string, string>;
  /**
   * **그룹별** 라인 색. 키는 `group_alias` 와 같은 조합 서명이다.
   * @spec SPEC-TSDB-004 §2.14
   *
   * 항목의 `color` 는 group by 파생 줄에 쓰이지 않는다(OQ1) — 색 하나를 N개
   * 그룹에 나눠 줄 수 없기 때문이다. 지정하지 않은 그룹은 자동 팔레트를 쓴다.
   */
  group_color?: Record<string, string>;
  /** 라인/카테고리 색상(미지정 시 자동 팔레트). */
  color?: string;
  /**
   * 라인 차트 전용 per-line 스타일 (SPEC-WEB-005, ChannelRefConfig 와 동일 형상).
   * 채널 시리즈와 스토어 시리즈의 라인 스타일을 하나의 편집기로 통합하기 위해
   * StoreSeriesRef 에도 동일 필드를 둔다. 라인 차트가 아닌 패널에서는 무시된다.
   */
  /** 라인 스타일(solid/dashed/dotted). 기본 'solid'. */
  stroke_style?: StrokeStyle;
  /** 라인 두께(px). 기본 2. */
  stroke_width?: number;
  /** 부드러운 곡선. 기본 false. */
  smooth?: boolean;
  /**
   * 값 추출 필드(dot-path). 채널 시리즈와 형상 통일을 위해 둔다.
   * 단, 스토어 소스는 매트릭스가 이미 시리즈별 단일 숫자 값을 제공하므로
   * 렌더에는 영향을 주지 않는다(메타데이터/전방 호환 목적).
   */
  display_field?: string;
}

/**
 * 차트 패널 Store 소스 설정 블록(모든 차트 config 가 공유).
 *
 * 시간 윈도우는 "지금(now) 기준 상대 윈도우" 로 해석된다:
 *   endMs = now, startMs = now - time_window_ms.
 * 매트릭스는 `interval_ms` 버킷으로 서버/클라이언트 집계(`aggregation`)되어
 * 시리즈별 타임라인으로 변환된다.
 *
 * @spec SPEC-WEB-005
 */
export interface StoreSourceConfig {
  /**
   * Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006
   *
   * Store API 는 이름 주소(`/store/{agent_name}/...`)이지만, 에이전트 이름은
   * 변경될 수 있어 config 에 이름만 저장하면 리네임 시 연결이 끊긴다. 따라서
   * 불변 ID 를 정본으로 저장하고, 렌더/쿼리 시 이 id 로 현재 이름을 해석해
   * API 를 호출한다. 구 config 하위호환을 위해 옵셔널이며, 부재 시 `agent_name`
   * 을 그대로 사용한다.
   */
  agent_id?: string;
  /**
   * Store 에이전트 이름.
   *
   * `agent_id` 가 있으면 이 값은 표시용 스냅샷 + 하위호환 폴백으로만 쓰인다
   * (실제 API 호출 이름은 agent_id 로 해석한 현재 이름). `agent_id` 가 없는 구
   * config 에서는 이 값이 그대로 API 호출에 사용된다. @spec SPEC-WEB-006
   */
  agent_name: string;
  /** Store 네임스페이스(미지정 시 'default'). */
  namespace?: string;
  /**
   * 이름을 지정하지 않은 시리즈의 표시 이름 형식(템플릿). @spec SPEC-WEB-005
   *
   * `{$.measurement}` / `{$.field}` / `{$.tags.NAME}` 토큰과 리터럴을 섞어 쓴다
   * (aliasTemplate 과 같은 문법). 시리즈에 이름(alias)을 직접 입력하면 그 이름이
   * 항상 이기고, 이 형식은 이름이 비어 있는 시리즈에만 적용된다.
   *
   * 미지정이면 내장 서술 표기(`measurement · field{k=v}`)로 폴백한다.
   */
  series_name_format?: string;
  /**
   * 시리즈 선택 방식. @spec SPEC-WEB-005
   *
   * - `'keys'`(기본): 사용자가 `series[]` 를 직접 멀티셀렉트한다(기존 동작).
   * - `'tag'`: `tag_filters` 로 매칭되는 모든 store 키를 폴링 시점마다 동적으로
   *   시리즈로 확장한다. 태그 하위 키가 추가/삭제되면 자동 반영된다. `series[]` 는 무시된다.
   *
   * 미지정/undefined 는 `'keys'` 로 해석되어 하위 호환을 보존한다(기존 패널 무영향).
   */
  selection_mode?: 'keys' | 'tag';
  /**
   * 태그 AND 필터(`selection_mode === 'tag'` 일 때만 사용). @spec SPEC-WEB-005
   *
   * 예) `{ room: '1', type: 'temperature' }` → room=1 AND type=temperature 를 가진
   * 모든 키가 시리즈가 된다. 폴링마다 재해석되므로 키 추가/삭제가 자동 반영된다.
   * 비어있으면(키 0개) tag 모드는 비활성(idle)으로 취급한다.
   */
  tag_filters?: Record<string, string>;
  /**
   * 조회할 시리즈 목록. `'keys'` 모드의 정본이다. 비어있으면 store 소스는 비활성으로
   * 취급한다. `'tag'` 모드에서는 무시되며 키가 동적으로 해석된다.
   */
  series: StoreSeriesRef[];
  /**
   * 상대 시간 윈도우 길이(ms). now - time_window_ms 가 시작 시각.
   *
   * `range` 가 없는 구 config 의 정본이며, `range` 가 있으면 상대 방식의 폴백 값으로만
   * 쓰인다(`readSeriesRange`). 지우지 않는 이유는 되돌리기 때문이다 — 절대/갯수로
   * 바꿨다가 상대로 되돌렸을 때 예전 창 길이가 살아나야 한다.
   */
  time_window_ms: number;
  /**
   * 가져올 데이터 범위 — 기간(상대·절대) 또는 갯수. 미지정이면 `time_window_ms` 를
   * 상대 기간으로 해석한다(구 config 하위 호환).
   */
  range?: SeriesRange;
  /** 버킷 크기(ms). */
  interval_ms: number;
  /** 집계 함수(UI 표기 그대로). */
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last';
  /** 폴링 주기(ms). 미지정 시 기본값(약 5000ms)을 사용한다. */
  refresh_interval_ms?: number;
}

/**
 * Store 소스의 **조회 창 기본값**(시간창 · 버킷 · 집계 · 폴링 주기) 단일 정본.
 * @spec SPEC-CHART-002 §2.8 [E2] 1항
 *
 * 두 곳이 같은 값을 써야 한다.
 *
 *   1. `ChartPanelSections.tsx` 의 `defaultStoreSource()` — 사용자가 데이터 소스를
 *      처음 Store 로 토글할 때.
 *   2. `gaugeLegacyBinding.ts` 의 `buildGaugeStoreMigrationPatch()` — 게이지 레거시
 *      바인딩을 이관할 때(spec 이 "기본값(`defaultStoreSource()`)" 이라고 못박았다).
 *
 * 값을 각자 복제하면 한쪽만 바뀔 때 이관 결과가 조용히 어긋난다. 그렇다고
 * `defaultStoreSource()` 를 직접 import 하면 순수 모듈인 `gaugeLegacyBinding.ts` 가
 * React 트리를 끌어와 단위 테스트가 무거워진다. 그래서 **양쪽이 이미 import 하는
 * 의존성 없는 타입 모듈**인 여기에 값만 올린다.
 */
export const DEFAULT_STORE_SOURCE_WINDOW = {
  time_window_ms: 60 * 60 * 1000, // 지난 1시간
  interval_ms: 60 * 1000, // 1분 버킷
  aggregation: 'average',
  refresh_interval_ms: 5000,
} as const satisfies Pick<
  StoreSourceConfig,
  'time_window_ms' | 'interval_ms' | 'aggregation' | 'refresh_interval_ms'
>;

/**
 * 아무 것도 바인딩되지 않은 **기본 Store 소스**. `DEFAULT_STORE_SOURCE_WINDOW` 를 감싼
 * 유일한 팩토리이며, 이 형상을 필요로 하는 모든 지점이 여기를 부른다.
 *
 *   1. `ChartPanelSections.tsx` 의 `defaultStoreSource()` — 데이터 소스를 처음 Store 로
 *      토글할 때.
 *   2. `uiStore.ts` 의 `createDefaultPanel()` — 통계/게이지/바/파이 신규 패널의 기본
 *      데이터 소스(생성 위저드가 채널 이름을 묻지 않고 곧바로 Store 로 시작한다).
 *   3. `AddPanelDialog.tsx` 의 Store 라인 차트 프리셋.
 *
 * 세 지점이 값을 각자 복제하고 있으면 한 곳만 바뀔 때 "같은 기본값" 이라는 전제가
 * 조용히 깨진다 — 특히 (2)와 (3)은 사용자가 나란히 만드는 패널이라 어긋남이 바로
 * 드러난다. 함수로 두는 이유는 `series` 배열이 패널마다 독립이어야 하기 때문이다
 * (상수를 공유하면 한 패널의 시리즈 추가가 다른 패널로 샌다).
 *
 * `selection_mode` 를 넣지 않는 것이 기존 동작이다 — 부재는 `'keys'` 와 동일하게
 * 해석되며(`panelDataSource.isStoreSourceActive`), 태그 모드는 설정 화면의 태그 피커로
 * 진입한다.
 */
export function buildDefaultStoreSource(): StoreSourceConfig {
  return {
    agent_name: '',
    namespace: 'default',
    series: [],
    ...DEFAULT_STORE_SOURCE_WINDOW,
  };
}

/**
 * TSDB 소스가 지원하는 백엔드. 확장 지점.
 *
 * 백엔드가 늘어도 `ChartDataSourceKind` 는 늘지 않는다 — 백엔드는 종류가 아니라
 * TSDB 종류의 하위 축이며, 질의 라우팅의 정본은 참조된 에이전트의 실제 타입이다.
 *
 * @spec SPEC-TSDB-002 §2.2 (U2) · §2.18 (U11)
 */
export type TsdbBackend = 'influxdb';

/**
 * TSDB 시리즈 참조. **어휘는 `StoreSeriesRef` 와 동일**하며, 백엔드별 개념 대응은
 * 쓰기 경로(`internal/node/storage_backend_*.go`)의 매핑 규약을 그대로 따른다.
 *
 *   key → measurement (influxdb)
 *   field → field
 *   tags → tags
 *
 * @spec SPEC-TSDB-002 §2.2 (U2)
 */
export interface TsdbSeriesRef {
  /** 시리즈 키. influxdb 백엔드에서는 measurement 이름이다. */
  key: string;
  /**
   * 값 필드. TSDB 소스에서는 **필수**다.
   *
   * `StoreSeriesRef.field` 와 달리 옵셔널이 아닌 이유는 "첫 번째 숫자 필드" 같은
   * 폴백이 조용한 오답이기 때문이다(§2.16 #4).
   */
  field: string;
  /** 시리즈 태그 필터(사전 필터 — 어느 데이터를 볼지). */
  tags?: Record<string, string>;
  /**
   * 시리즈를 나눌 태그 키 목록(분할 축 — 어떻게 나눌지). @spec SPEC-TSDB-004 §2.1
   *
   * 비어 있거나 없으면 **정확 일치 모드**이며 이 항목은 시리즈 1개다(현행 동작).
   * 키가 하나 이상이면 **group by 모드**이며 그 키들의 값 조합마다 시리즈가
   * 하나씩 생긴다 — 항목 1개가 런타임에 N개로 펼쳐진다.
   *
   * `tags` 와 직교한다. `tags: {region:'kr'}` + `group_by: ['host']` 는
   * "kr 리전 안에서 host 별로" 를 뜻한다. 같은 키를 양쪽에 두면 서버가 400 으로
   * 거부한다 — 값이 고정된 키로 나누면 그룹이 항상 1개이기 때문이다.
   *
   * group by 모드에서 `color` 는 무시되고 자동 팔레트가 그룹마다 배정된다
   * (색 하나를 N개 그룹에 나눠 줄 수 없다). `stroke_style`·`stroke_width`·
   * `smooth` 는 전 그룹이 공유한다.
   */
  group_by?: string[];
  /**
   * 표시할 그룹을 태그 값 **조합 목록**으로 고른 것. @spec SPEC-TSDB-004 §2.7.1
   *
   * 없거나 비면 `group_by` 가 만드는 **전 그룹**을 표시한다(하위호환 · 백엔드 규약과
   * 동일). 설정 UI 는 사용자가 표에서 고른 조합을 여기에 명시한다.
   *
   * 조합 목록인 이유는 다중 키 때문이다 — 키별 허용값 맵으로 두면 데카르트 곱이
   * 되어 실재하지 않는 조합까지 고르게 된다.
   */
  group_filter?: Array<Record<string, string>>;
  /** 표시 별칭(미지정 시 형식/서술 표기로 폴백). group by 항목에서는 템플릿이다. */
  alias?: string;
  /**
   * **그룹별** 개별 표시 이름. 키는 `group_by` 를 정렬한 순서의 조합 서명이다
   * (`groupComboSignature`). @spec SPEC-TSDB-004 §2.12
   *
   * `alias` 하나로는 펼쳐진 N개 그룹에 서로 다른 이름을 줄 수 없다. 토큰
   * 템플릿은 규칙 있는 이름만 만들므로, 그룹마다 임의의 이름(예: "실습실")을
   * 붙이려면 이 축이 필요하다. 우선순위는 그룹별 이름 > `alias` > 패널 형식.
   *
   * 조합을 해제해도 여기 남은 이름은 지우지 않는다 — 다시 켰을 때 이름이
   * 돌아오는 편이 놀랍지 않다. 항목을 지우면 함께 사라진다.
   */
  group_alias?: Record<string, string>;
  /**
   * **그룹별** 라인 색. 키는 `group_alias` 와 같은 조합 서명이다.
   * @spec SPEC-TSDB-004 §2.14
   *
   * 항목의 `color` 는 group by 파생 줄에 쓰이지 않는다(OQ1) — 색 하나를 N개
   * 그룹에 나눠 줄 수 없기 때문이다. 지정하지 않은 그룹은 자동 팔레트를 쓴다.
   */
  group_color?: Record<string, string>;
  /**
   * 라인/카테고리 색상(미지정 시 자동 팔레트).
   *
   * `group_by` 가 지정된 항목에서는 무시된다(위 참조).
   */
  color?: string;
  /** 라인 스타일(solid/dashed/dotted). 라인 차트 전용. */
  stroke_style?: StrokeStyle;
  /** 라인 두께(px). 라인 차트 전용. */
  stroke_width?: number;
  /** 부드러운 곡선. 라인 차트 전용. */
  smooth?: boolean;
}

/**
 * 외부 시계열 DB 소스 설정(`data_source: 'tsdb'` 일 때 사용).
 *
 * 백엔드 중립 키와 백엔드 전용 키가 한 블록에 **평탄하게 공존**한다. 이는 쓰기
 * 경로의 선례를 따른 것이다 — `storage_write.go` 가 "백엔드 전용 키(해당 없는
 * 백엔드는 무시)"를 같은 방식으로 다룬다. 백엔드마다 블록을 쪼개면 같은 개념이
 * 두 형태로 존재하게 된다(§2.16 #2).
 *
 * @spec SPEC-TSDB-002 §2.2 (U2)
 */
export interface TsdbSourceConfig {
  /**
   * 기록된 백엔드(스냅샷). **질의 라우팅의 정본이 아니다** — 정본은 항상 참조된
   * 에이전트의 실제 타입이다(§2.18). 이 값은 (a) 에이전트 목록이 로드되기 전
   * 설정 UI 를 그리기 위한 낙관적 표시값이고, (b) 불일치를 감지하기 위한 대조군이다.
   */
  backend: TsdbBackend;

  /** 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  agent_id?: string;
  /** 에이전트 이름(표시용 스냅샷 + 하위호환 폴백). @spec SPEC-WEB-006 */
  agent_name: string;

  /** [influxdb 전용] v2 = bucket, v3 = database. 미지정이면 에이전트 기본값. */
  bucket?: string;

  /** 조회할 시리즈. 비어 있으면 소스는 비활성이다(§2.3). */
  series: TsdbSeriesRef[];

  /** 상대 시간 윈도우 길이(ms). `range` 의 상대 방식 폴백 값이다(Store 와 같은 규칙). */
  time_window_ms: number;
  /** 가져올 데이터 범위 — 기간(상대·절대) 또는 갯수. Store 와 같은 어휘를 쓴다. */
  range?: SeriesRange;
  /** 버킷 크기(ms). */
  interval_ms: number;
  /**
   * 인터벌(버킷) 집계 함수. @spec SPEC-TSDB-004 §2.18
   *
   * `StoreSourceConfig.aggregation` 보다 **넓다** — Store 백엔드는 min/max/avg 세
   * 종만 처리하고, InfluxDB 는 일곱 종을 처리한다. 어휘를 억지로 맞추면 한쪽에
   * 없는 값이 조용히 400 이 된다.
   */
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last' | 'sum' | 'count';
  /** 빈 버킷 처리 전략. `'avg'` 는 InfluxDB 양쪽 모두 대응물이 없어 지원하지 않는다(§2.7). */
  fill?: '' | 'null' | 'zero' | 'previous';
  /**
   * `previous` 채우기로 직전값을 이어 쓸 수 있는 **최대 기간(ms)**.
   *
   * 없거나 0 이면 제한 없이 계속 이어 쓴다(종전 동작). 버킷 개수가 아니라
   * 시간이라, 인터벌을 바꿔도 "최대 5분까지 쓴다" 는 뜻이 그대로 유지된다.
   * `fill === 'previous'` 가 아니면 읽지 않는다.
   */
  fill_previous_max_ms?: number;
  /** 사용 기간을 넘긴 버킷의 처리. 미지정이면 비운다(null). */
  fill_previous_overflow?: '' | 'value';
  /** 위가 `'value'` 일 때 채울 값. */
  fill_previous_overflow_value?: number;
  /** 폴링 주기(ms). 미지정 시 기본값(약 5000ms)을 사용한다. */
  refresh_interval_ms?: number;
  /**
   * 한 페이지에 조회할 그룹 수(시리즈축 페이지네이션). @spec SPEC-TSDB-004 §2.7
   *
   * 0 이거나 없으면 페이지네이션이 비활성이고 그룹 전량을 조회한다 — 저장된
   * config 의 동작이 변하지 않는다. 페이지 **인덱스**는 여기 두지 않는다.
   * 그것은 보기 커서이며 페이지를 넘길 때마다 config 가 저장되면 안 된다.
   */
  group_page_size?: number;
  /** 이름을 지정하지 않은 시리즈의 표시 이름 형식(템플릿). @spec SPEC-WEB-005 */
  series_name_format?: string;
}

/**
 * TSDB 소스 블록의 초기값. 사용자가 데이터 소스를 처음 TSDB 로 토글할 때 쓴다.
 * @spec SPEC-TSDB-002 §2.2 (U2) · §2.12 (E2)
 *
 * 조회 창(시간창 · 인터벌 · 집계 · 폴링 주기)은 `DEFAULT_STORE_SOURCE_WINDOW` 를
 * **전개**한다. 값을 복제하면 한쪽만 바뀔 때, 소스를 갈아탄 사용자가 조용히 다른
 * 창을 보게 된다.
 *
 * `agent_name` 이 빈 문자열이고 `series` 가 빈 배열이므로 이 블록은 **비활성**이다
 * (§2.3) — 에이전트를 고르기 전에는 조회하지 않는다.
 */
export function defaultTsdbSource(): TsdbSourceConfig {
  return {
    backend: 'influxdb',
    agent_name: '',
    series: [],
    ...DEFAULT_STORE_SOURCE_WINDOW,
  };
}

/**
 * 윈도우 단위 **구간 대표값** 함수. @spec SPEC-CHART-002 §2.2
 *
 * 한 시리즈의 시간 윈도우 타임라인 전체를 숫자 1개로 접는다. 계산 규칙은
 * `seriesReduce.ts` 의 `reduceSeries` 가 단일 정본으로 소유한다.
 *
 * `'first'` 는 사용자 선택지로 노출하지 않는다 — `delta`(= last − first)의 내부
 * 입력으로만 쓴다. `StoreSourceConfig.aggregation` 의 `'first'` 는 **다른 축**의
 * 값이며 이것과 무관하다(아래 두 축 구분 참조).
 */
export type SeriesReduceFunc = 'max' | 'avg' | 'min' | 'last' | 'sum' | 'count' | 'delta';

/**
 * 구간 대표값 선택기를 노출하는 패널 타입 집합. @spec SPEC-CHART-002 §2.3
 *
 * `line-chart` · `table` · `heatmap` 은 같은 `StoreSourceSection` 을 쓰지만
 * 선택기가 노출되지 않으며 `series_reduce` 를 읽지도 않는다(UB1-10).
 */
export const REDUCE_PANEL_TYPES: ReadonlySet<string> = new Set([
  'stat',
  'gauge',
  'bar-chart',
  'pie-chart',
]);

/** 모든 차트 패널이 공유하는 공통 config (REQ-M4-02) */
export interface ChartPanelConfigBase {
  channel_name: string;
  display_field?: string;
  label_field?: string;
  max_points?: number;
  refresh_on_reconnect?: boolean;
  /**
   * 데이터 소스 종류. 미지정/undefined 는 'channel'(기존 채널 경로)로
   * 해석되어 하위 호환을 보존한다. @spec SPEC-WEB-005
   */
  data_source?: ChartDataSourceKind;
  /** Store 소스 설정(data_source === 'store' 일 때 사용). @spec SPEC-WEB-005 */
  store_source?: StoreSourceConfig;
  /**
   * 외부 TSDB 소스 설정(data_source: 'tsdb' 일 때 사용). @spec SPEC-TSDB-002 §2.2
   *
   * `store_source` 와 **공존**한다 — 소스를 전환해도 다른 소스의 블록은 삭제하지
   * 않는다(§2.12 [E2]). 되돌리기가 가능해야 사용자가 전환을 시도한다.
   */
  tsdb_source?: TsdbSourceConfig;
  /**
   * 윈도우 단위 구간 대표값. @spec SPEC-CHART-002 §2.1 [U1]
   *
   * **`store_source.aggregation` 과는 서로 다른 축이며 절대 겸용하지 않는다.**
   *
   * | 축 | 필드 | 적용 시점 | 결과 형상 |
   * |----|------|-----------|-----------|
   * | 버킷 집계 | `store_source.aggregation` | 조회 시점(서버) | 시리즈당 `interval_ms` 버킷마다 값 1개 → **타임라인** |
   * | 윈도우 대표값 | `series_reduce` (이 필드) | 렌더 시점(클라이언트 순수 계산) | 시리즈당 **숫자 1개** |
   *
   * 두 축은 순차 합성된다:
   *   `원시 표본 → (aggregation) → 버킷 타임라인 → (series_reduce) → 대표값 1개`.
   * 예) `aggregation:'average'` + `series_reduce:'max'` = "1분 평균들의 구간 최댓값"
   * 이며, `aggregation:'max'` + `series_reduce:'max'`("구간 최댓값")와 결과가 다르다.
   *
   * `store_source` **블록 밖**에 두는 이유: `store_source` 는 `useStoreChartData`
   * 의 `pollKey` 소재지이고 pollKey 는 "재조회가 필요한가" 를 판정한다. 대표값은
   * 조회 파라미터가 아니라 표현 파라미터이므로, 같은 블록에 두면 대표값 변경이
   * 불필요한 재조회를 유발하거나(포함 시) 한 블록 안에서 필드별 취급이 갈린다
   * (제외 시). 블록을 나누면 "조회 축은 store_source, 표현 축은 패널 config" 가
   * 타입 수준에서 드러난다(§4.1).
   *
   * **미지정(부재) = 레거시 렌더 경로**다(§2.9 [S1]). 기본값을 정의하지 않는다 —
   * 부재를 `'last'` 로 해석하면 저장된 config 를 건드리지 않고도 기존 패널 외형이
   * 바뀐다. `data_source !== 'store'` 인 경우에도 읽지 않는다(§2.10 [S2]).
   */
  series_reduce?: SeriesReduceFunc;
  /**
   * 다중 출력(타일/게이지/막대/조각)의 표시 개수 상한. @spec SPEC-CHART-002 §2.4 [U4]
   *
   * 미지정이면 `DEFAULT_MULTI_OUTPUT_LIMIT`(= 12, `SeriesTileGrid.tsx` 소유)를 쓴다.
   * 상한을 넘는 출력은 순서상 뒤에서부터 잘리고 `+K` 표기로 잘린 개수를 알린다.
   *
   * 시리즈 **선택** 상한(`STORE_SERIES_LIMIT` = 48)과는 다른 축이다 — 선택 상한은
   * 조회 부하를, 이 상한은 가독성을 보호한다(§4.6). 48개를 조회하되 12개만 그리는
   * 상태는 정상이다.
   *
   * `series_reduce` 부재(레거시) 경로에서는 읽지 않는다.
   */
  multi_output_limit?: number;
  /**
   * 다중 출력 타일 배열의 **목표 행 수**. 미지정이면 `DEFAULT_TILE_ROWS`(= 1) — 한 줄.
   *
   * 열 수는 `ceil(N / tile_rows)` 로 파생된다(`tileColumnCount`). 상한이 아니라 목표라서,
   * 패널이 좁아 타일 최소 폭을 확보하지 못하면 열이 줄고 행이 목표보다 늘어난다.
   *
   * 타일 배열을 쓰는 **통계 · 게이지**만 읽는다. 바 · 파이는 시리즈를 한 차트 안의 막대 ·
   * 조각으로 그리므로 배열 개념이 없고, `series_reduce` 부재(레거시) 경로에서도 읽지 않는다.
   */
  tile_rows?: number;
}

// --- 차트 타입별 config (SPEC-CHART-001 §4.2.2) ---

export interface StatPanelConfig extends ChartPanelConfigBase {
  unit?: string;
  decimal_places?: number;
  threshold_color_rules?: Array<{ min: number; color: string }>;
}

/**
 * 툴팁 표시 설정 (line-chart).
 *
 * 두 값 모두 미지정이 종전 동작이다 — 툴팁을 켜고, 가리킨 시각의 **모든** 시리즈를
 * 한 상자에 모아 보여 준다. 저장된 대시보드의 동작이 변하지 않도록 기본값을 그렇게 둔다.
 */
export interface TooltipConfig {
  /** 툴팁을 띄울지. 미지정이면 켬. */
  enabled?: boolean;
  /**
   * 가리킨 **한 시리즈**의 값만 보여줄지. 미지정이면 전체 시리즈를 함께 보여 준다.
   * 시리즈가 많아 상자가 화면을 덮을 때 쓴다.
   */
  single?: boolean;
}

/** Y축 도메인 결정 방식 (line-chart) */
export type YAxisMode = 'auto' | 'manual' | 'auto_padded';

/**
 * Y축 데이터 타입 (line-chart).
 * - `numeric`(기본): 숫자 축. y_axis_mode(auto/manual/auto_padded) + y_min/y_max 로 범위 결정.
 * - `enum`: 열거형 축. y_enum_labels 의 값→라벨 매핑으로 눈금/툴팁을 문자열로 표시한다.
 *   (기존 boolean 자동 표시 0→false / 1→true 를 사용자 정의로 일반화한 것)
 */
export type YAxisDataType = 'numeric' | 'enum';

/** 열거형 Y축의 값→라벨 매핑 항목 (line-chart). 예: { value: 0, label: '정지' } */
export interface YEnumLabel {
  /** 매핑할 숫자 값. */
  value: number;
  /** 해당 값에 표시할 문자열. */
  label: string;
}

/** X축 시간 윈도우 결정 방식 (line-chart) */
export type TimeWindowMode = 'points' | 'recent' | 'fixed';

/** Y축 임계선 심각도 (line-chart) — 하위 호환용 유지 */
export type ThresholdSeverity = 'info' | 'warning' | 'critical';

/** 경계 라인 정의.
 *  ReferenceLine 으로 그려지고, fill_to 지정 시 ReferenceArea 로 사이를 색칠. */
/** 경계 채우기 방향 */
export type FillDirection = 'below' | 'above';

export interface YThreshold {
  /** Y축 값 */
  value: number;
  /** 솔리드 색상 (필수). 라인과 fill 에 모두 사용 */
  color: string;
  /** 채우기 방향. 'below' = 값 이하, 'above' = 값 이상. 미지정 시 채우기 없음 */
  fill_direction?: FillDirection;
  /** @deprecated fill_to 대신 fill_direction 사용 */
  fill_to?: number;
  /** 하위 호환: severity */
  severity?: ThresholdSeverity;
  /** 하위 호환: label */
  label?: string;
}

/**
 * 시리즈 자동 색상 팔레트. 데이터 소스 선택 시 시리즈 인덱스별로 서로 다른 색을
 * 자동 배정하는 데 사용한다(사용자가 개별 색을 지정하면 그 값이 우선).
 * LineChartPanel 렌더와 설정 편집기(스와치 기본값)가 동일 팔레트를 공유한다.
 */
export const SERIES_PALETTE: readonly string[] = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
];

/** 시리즈 인덱스에 대응하는 팔레트 색을 반환한다(팔레트 길이로 순환). */
export function pickSeriesColor(index: number): string {
  const n = SERIES_PALETTE.length;
  const i = ((Math.trunc(index) % n) + n) % n;
  return SERIES_PALETTE[i]!;
}

/**
 * StoreSeriesRef 의 동일성 식별자(key + field + 정렬된 tags). keys 모드에서 선택된
 * 시리즈를 판정/추가/제거할 때 쓴다. 반환 형식은 `"<key> <metric> <k=v,...>"` 로 고정한다.
 * @spec SPEC-PANEL-SETTINGS-001 (시리즈 선택 단일화 — 체크박스 ↔ series)
 */
export function storeSeriesId(
  key: string,
  metric: string,
  tags: Record<string, string>,
): string {
  const tagPart = Object.keys(tags)
    .sort()
    .map((k) => `${k}=${tags[k]}`)
    .join(',');
  return `${key} ${metric} ${tagPart}`;
}

/**
 * StoreSeriesRef 의 **표시 라벨**(사람이 읽는 이름). `storeSeriesId` 의 표시 짝이다.
 *
 * 결함 배경: 시리즈의 동일성은 (key, field, tags) 인데 표시에는 key 만 쓰여서, 한 key 를
 * metric/tags 로 나눠 갖는 형제 시리즈들이 목록·마커에서 **같은 글자**로 보였다. 좌표/매칭은
 * 이미 동일성 키로 분리되어 있었으므로(SPEC-HEATMAP-PANEL-001 재키잉) 남은 것은 표기뿐이며,
 * 이 함수가 그 표기를 한 곳으로 모은다.
 *
 * 규칙:
 *   - 사용자가 붙인 이름(alias)이 있으면 그 이름이 항상 이긴다.
 *   - 그 외에는 매트릭스 컬럼과 동일한 서술 표기(`key · metric{k=v}`)를 쓴다. metric/tags 가
 *     없으면 자연히 `key` 하나로 줄어든다(구분자 잔여물 없음).
 *
 * 과거에는 `alias === key` 를 "이름 없음"으로 취급했다. 시리즈 생성 시 `alias: key` 를
 * 기본값으로 기록했기 때문에 alias 존재만으로는 기본값과 사용자 입력을 구분할 수 없었다.
 * 그 대가로 사용자가 measurement 와 똑같은 이름을 **직접 입력해도** 무시되어, 설정의 이름
 * 입력·미리보기(이름 그대로)와 목록의 표시 이름(서술 표기)이 갈리는 결함이 있었다.
 * 이제 생성 시 alias 를 비워 두고(기본값 제거), 읽는 시점에 legacy 기본값을 걷어내므로
 * (normalizeStoreSeriesAlias) alias 존재 = 사용자 입력이 되어 이 예외가 필요 없다.
 *
 * 매칭·동일성에는 절대 쓰지 않는다 — 이름을 바꿔도 좌표/선택이 끊기면 안 된다.
 */
export function storeSeriesLabel(
  ref: Pick<StoreSeriesRef, 'key' | 'field' | 'tags' | 'alias'>,
  nameFormat?: string,
): string {
  const ctx = aliasContextOf(ref);
  // 1) 시리즈에 직접 붙인 이름이 항상 이긴다. 토큰을 쓴 이름도 해석한다.
  const alias = ref.alias?.trim() ?? '';
  if (alias !== '') return resolveSeriesAlias(alias, ctx);
  // 2) 패널이 지정한 이름 형식. 해석 결과가 비면(참조 토큰이 전부 빈 값) 폴백한다.
  const fmt = nameFormat?.trim() ?? '';
  if (fmt !== '') {
    const resolved = resolveSeriesAlias(fmt, ctx).trim();
    if (resolved !== '') return resolved;
  }
  // 3) 내장 서술 표기.
  return seriesRefDisplayName(ref.key, ref.field, ref.tags);
}

/** 시리즈 참조를 이름 템플릿 해석 컨텍스트로 변환한다(표시 경로 공통). */
export function aliasContextOf(
  ref: Pick<StoreSeriesRef, 'key' | 'field' | 'tags'>,
): AliasContext {
  return { measurement: ref.key, field: ref.field, tags: ref.tags ?? {} };
}

/**
 * 저장된 시리즈에서 legacy 기본 alias(`alias === key`)를 "이름 없음"으로 되돌린다.
 *
 * 과거 생성 경로가 `alias: key` 를 기본값으로 기록했기 때문에, 그 값을 그대로 두면
 * 사용자 입력과 구분할 수 없다. 읽는 시점에 한 번 걷어내면 이후로는 alias 존재 여부가
 * 곧 "사용자가 이름을 붙였는가" 가 된다(설정 화면의 이름 입력·미리보기와 목록 표시가 일치).
 *
 * config 를 저장하지는 않는다 — 표시·편집용 정규화이며, 사용자가 이름을 입력하면 그때
 * 정상 값으로 기록된다.
 */
export function normalizeStoreSeriesAlias(series: readonly StoreSeriesRef[]): StoreSeriesRef[] {
  return series.map((s) =>
    s.alias !== undefined && s.alias.trim() === s.key ? { ...s, alias: undefined } : s,
  );
}

/**
 * 선택 계열(series) 상한. 라이브 미리보기 성능 보호를 위한 합리적 상한(수십 개).
 * @spec SPEC-PANEL-SETTINGS-001 (AC-15)
 */
export const STORE_SERIES_LIMIT = 48;

/**
 * 축 텍스트(레이블/눈금) 폰트 스타일. 미지정 필드는 렌더 측 기본값으로 폴백한다.
 * 라인 차트의 X/Y 축 레이블(제목)과 값(눈금) 폰트를 축별로 독립 설정한다.
 */
export interface AxisFontStyle {
  /** 글자 크기(px). */
  size?: number;
  /** 글자 색상(hex). */
  color?: string;
  /** 굵기. */
  weight?: 'normal' | 'bold';
}

/** 축 폰트 기본값(기존 하드코딩 값과 동일). */
export const DEFAULT_AXIS_FONT: Required<AxisFontStyle> = {
  size: 10,
  color: '#9ca3af',
  weight: 'normal',
};

/**
 * AxisFontStyle 을 recharts 텍스트 props(fontSize/fill/fontWeight)로 변환한다.
 * 미지정 필드는 DEFAULT_AXIS_FONT 로 채운다.
 */
export function resolveAxisFont(
  font: AxisFontStyle | undefined,
): { fontSize: number; fill: string; fontWeight: 'normal' | 'bold' } {
  return {
    fontSize: font?.size ?? DEFAULT_AXIS_FONT.size,
    fill: font?.color ?? DEFAULT_AXIS_FONT.color,
    fontWeight: font?.weight ?? DEFAULT_AXIS_FONT.weight,
  };
}

/** 라인 스타일 — 채널별로 적용 */
export type StrokeStyle = 'solid' | 'dashed' | 'dotted';

/** 채널 ref. channels 배열의 각 항목. */
export interface ChannelRefConfig {
  /** chart-emitter 채널명 (필수) */
  name: string;
  /** 라인 표시 별칭. 미지정 시 name 사용 */
  alias?: string;
  /** 채널별 표시 필드. 미지정 시 'value' */
  display_field?: string;
  /** 라인 색상. 미지정 시 자동 팔레트 */
  color?: string;
  /** 라인 스타일. 기본 'solid' */
  stroke_style?: StrokeStyle;
  /** 라인 두께 (px). 기본 2 */
  stroke_width?: number;
  /** 부드러운 곡선. 기본 false */
  smooth?: boolean;
}

/** 범례 설정 */
export type LegendPosition = 'left' | 'right' | 'bottom';

export interface LegendConfig {
  /** 범례 위치. 기본 'bottom' */
  position?: LegendPosition;
  /** 시리즈 이름 표시. 기본 true */
  show_name?: boolean;
  /** 라인 미리보기 표시. 기본 true */
  show_line?: boolean;
  /** 마지막 값 표시. 기본 false */
  show_last_value?: boolean;
}

export interface LineChartPanelConfig extends ChartPanelConfigBase {
  /** 채널 목록 — 기본 입력 */
  channels?: ChannelRefConfig[];

  // X축
  /** X축 레이블 (예: "시간", "Time") */
  x_label?: string;
  /** X축 레이블(제목) 폰트. */
  x_label_font?: AxisFontStyle;
  /** X축 값(눈금) 폰트. */
  x_tick_font?: AxisFontStyle;
  /** Y축 레이블(제목) 폰트. */
  y_label_font?: AxisFontStyle;
  /** Y축 값(눈금) 폰트. */
  y_tick_font?: AxisFontStyle;

  // Y축
  /**
   * Y축 데이터 타입. 'numeric'(기본) 또는 'enum'.
   * 'enum' 이면 y_enum_labels 로 값→라벨 매핑을 표시하며, 숫자 범위(y_axis_mode/min/max)는
   * 무시된다(축 도메인은 enum 값 범위로 고정).
   */
  y_axis_type?: YAxisDataType;
  /** 열거형 값→라벨 매핑 (y_axis_type === 'enum' 일 때 사용). */
  y_enum_labels?: YEnumLabel[];
  y_min?: number;
  y_max?: number;
  y_axis_mode?: YAxisMode;
  y_axis_padding_pct?: number;
  /** Y축 레이블 (예: "온도") */
  y_label?: string;
  /** Y축 단위 (예: "°C", "kW") */
  y_unit?: string;
  /** 경계 라인 (threshold) */
  y_thresholds?: YThreshold[];

  /**
   * X축 범위 — 구간(absolute) · 최근(relative) · 포인트(count).
   *
   * 데이터 소스(Store · TSDB)가 쓰는 `SeriesRange` 와 **같은 어휘**다. 조회 범위와
   * 표시 범위는 같은 개념이므로 한 이름으로 쓴다.
   *
   * 없으면 아래 구 필드(`time_window_mode` 계열)를 읽어 해석한다 —
   * `readChartXRange` 가 그 폴백을 담당하므로 저장된 패널은 그대로 동작한다.
   */
  x_range?: SeriesRange;

  // X축 시간 윈도우 — `x_range` 로 대체됨. 읽기 폴백으로만 남는다(신규 저장 없음).
  /** @deprecated `x_range` 사용. `readChartXRange` 가 count/relative/absolute 로 옮긴다. */
  time_window_mode?: TimeWindowMode;
  /** @deprecated `x_range.window_ms` 사용(초 → ms). */
  recent_window_sec?: number;
  /** @deprecated `x_range.start_ms` 사용. */
  fixed_start_ms?: number;
  /** @deprecated `x_range.end_ms` 사용. */
  fixed_end_ms?: number;
  /** 최근 범위에서 X축 끝(now)을 전진시키는 주기(ms). */
  time_window_refresh_ms?: number;

  /**
   * Y축 눈금의 소수점 이하 자릿수. 미지정이면 값을 그대로 쓴다(종전 동작).
   * 숫자형 축에서만 의미가 있다 — 열거형·불리언 축은 눈금이 라벨이다.
   */
  decimal_places?: number;

  /** 범례 */
  legend?: LegendConfig;

  /** 툴팁 표시 설정. 미지정이면 켬 + 전체 시리즈(종전 동작). */
  tooltip?: TooltipConfig;

  /**
   * 값이 없는 구간을 **점선으로 이어** 표기할 최소 연속 결측 개수.
   * @spec SPEC-TSDB-004 §2.19
   *
   * 없거나 0 이하이면 끈다 — 이때 라인은 종전대로 결측을 조용히 이어 그린다
   * (`connectNulls`), 저장된 대시보드의 그림이 변하지 않는다.
   *
   * 켜면 원래 라인은 결측에서 끊기고, 그 구간만 점선 덧그림으로 이어진다.
   * 이은 것과 잰 것을 눈으로 가를 수 있게 하는 것이 목적이다 — 3시간 정전이
   * "그렇게 측정된 직선" 과 똑같이 보이면 안 된다.
   *
   * 빈 구간 채우기(`fill: 'zero' | 'previous'`)를 쓰면 결측 자체가 생기지 않아
   * 이 설정은 아무 일도 하지 않는다.
   */
  gap_dash_threshold?: number;

  /** @deprecated 채널별로 이동됨 — 하위 호환 fallback */
  smooth?: boolean;
  multi_series_field?: string;
}

/** severity 별 기본 색상 — 하위 호환 및 기본 threshold 색상 */
export const THRESHOLD_DEFAULT_COLORS: Record<ThresholdSeverity, string> = {
  info: '#3b82f6',
  warning: '#f59e0b',
  critical: '#ef4444',
};

/** stroke_style → SVG strokeDasharray 매핑 */
export const STROKE_DASHARRAY: Record<StrokeStyle, string> = {
  solid: '',
  dashed: '8 4',
  dotted: '2 3',
};

export type BarChartMode = 'category' | 'time_bin';
export type AggFunc = 'count' | 'sum' | 'avg';

export interface BarChartPanelConfig extends ChartPanelConfigBase {
  mode?: BarChartMode;
  bin_sec?: number;
  agg_func?: AggFunc;
}

export interface PiePanelConfig extends ChartPanelConfigBase {
  agg_func?: AggFunc;
  show_legend?: boolean;
  show_percentage?: boolean;
}

export type TableColumnFormat = 'datetime' | 'number' | 'string';

export interface TableColumn {
  field: string;
  header: string;
  format?: TableColumnFormat;
  /**
   * 열 너비 비율(가중치). 지정한 열끼리의 상대 비율로 폭을 나눈다 — 절대 px 이 아니다.
   *
   * 패널은 그리드 안에서 임의 폭으로 늘어나므로 px 로 고정하면 좁은 패널에서 넘치고
   * 넓은 패널에서 남는다. 비율은 두 경우 모두 자연스럽게 늘어난다.
   *
   * 미지정(undefined)은 "자동" 이다 — 지정된 열이 비율만큼 가져가고 나머지 열이 남은
   * 폭을 균등하게 나눈다. 전 열이 미지정이면 브라우저 기본 테이블 레이아웃과 같다.
   */
  width?: number;
  /**
   * 헤더 클릭 정렬 허용 여부. 미지정은 `true`(허용) — 기존 동작이 전 열 정렬 가능이었다.
   */
  sortable?: boolean;
  /**
   * 열 필터 입력 노출 여부. 미지정은 `false` — 필터 행은 자리를 차지하므로 켠 열에만 준다.
   */
  filterable?: boolean;
}

export type SortOrder = 'asc' | 'desc';

export interface TablePanelConfig extends ChartPanelConfigBase {
  columns: TableColumn[];
  rows_per_page?: number;
  default_sort?: { field: string; order: SortOrder };
}

/**
 * 유효한 열거형 매핑만 추려 값→라벨 Map 을 만든다.
 * value 가 유한 숫자이고 label 이 비어있지 않은 항목만 포함한다.
 * 같은 value 가 중복되면 뒤 항목이 앞 항목을 덮어쓴다(마지막 정의 우선).
 */
export function buildEnumLabelMap(
  labels: YEnumLabel[] | undefined,
): Map<number, string> {
  const map = new Map<number, string>();
  if (!labels) return map;
  for (const item of labels) {
    if (typeof item.value !== 'number' || !Number.isFinite(item.value)) continue;
    const label = (item.label ?? '').trim();
    if (label === '') continue;
    map.set(item.value, label);
  }
  return map;
}

/**
 * 열거형 축에서 숫자 값을 라벨로 변환한다. 매핑에 없으면 숫자 문자열로 폴백한다.
 * 값이 숫자가 아니면 빈 문자열을 반환한다(눈금 사이 보간값 등).
 */
export function formatEnumValue(value: number, map: Map<number, string>): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '';
  return map.get(value) ?? String(value);
}

/**
 * dot 경로로 ChartEntry 에서 값을 추출한다.
 * 예: getByPath(entry, "labels.room") -> entry.labels.room
 *     getByPath(entry, "value.inner") -> (entry.value as any).inner
 *
 * 중간 경로가 null/undefined 이거나 객체가 아니면 undefined 반환.
 */
export function getByPath(entry: ChartEntry, path: string): unknown {
  if (!path) return undefined;
  const parts = path.split('.');
  let cur: unknown = entry;
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}
