// 시리즈 데이터 뷰어 모달.
// 시리즈 멀티셀렉트, 시간 범위, 인터벌, 집계 함수 입력을 받아 매트릭스 쿼리를 실행한다.
// `ImportDialog` 의 포털/배경 클릭/Esc 닫기 패턴을 재사용한다.
//
// SPEC-WEB-005 v0.2.0 에서 `dataSource: SeriesDataSource` prop 을 받아
// TSDB/Store 양쪽 모두에 동작하도록 리팩터되었다.
//
// SPEC-WEB-005 v0.3.0 UI/UX 개선:
//   - 기본 시간 범위를 "지난 1일" 로 설정 (모달 오픈 시마다 리셋).
//   - 상대 범위 프리셋 버튼 ("지난 1시간" ~ "지난 30일") 추가.
//   - 체크박스 행의 영구 선택 하이라이트 제거 (체크 상태만 유지).
//   - 모달 크기를 95vw × 95vh 로 확대, 본문은 flex-1 스크롤 영역.
//
// SPEC-WEB-005 v0.3.0 Wave 2 UI/UX 개선:
//   - 시간 범위에 "절대/상대" 모드 탭 추가.
//     * 절대 (기본): 기존 datetime-local 입력 + 빠른 범위 버튼 유지.
//     * 상대: 드롭다운(프리셋 + 커스텀 duration) 만 노출하고, 실제 시간은
//       실행 버튼 클릭 시점의 `now` 를 기준으로 계산된다.
//   - 모달 오픈 시 모드는 항상 "절대" 로 리셋된다 (기본 동작 보존).
//   - 결과 매트릭스에 CSV 내보내기 버튼을 위한 agentName / 범위 전달.
//
// SPEC-WEB-005 v0.7.0 (M16, Task 11/13) 메타데이터 표시 + 신규 필터 UI:
//   - Store 모드 시리즈 행에 data_type / field / registration 칩을 표시.
//   - 시리즈 풀 위에 data_type / field / registration 필터 UI 추가
//     (Store 모드에 한정; TSDB 모드는 메타데이터 소스가 없어 필터를 노출하지 않음).
//   - 신규 필터는 기존 태그 필터 + 검색과 AND 결합되며, 모두 클라이언트 측에서 적용된다
//     (서버 라운드트립 없음 — Phase A 의 `keyObjects` 응답을 그대로 사용).
//
// SPEC-WEB-005 v0.7.0 (Option A) TSDB 메타데이터 필터 확장:
//   - TSDB 모드에서도 data_type / field 필터를 노출한다.
//     field 은 키 이름에서 자동 추출 (InfluxDB measurement / 첫 segment),
//     data_type 은 시계열 numeric 가정으로 'float' 고정.
//   - registration 은 TSDB 에 개념이 없으므로 Store 모드에서만 노출 (필터 자체가 숨김).
//   - 합성 메타데이터는 `useExtractedTagFilterState` 가 빌드하며,
//     기존 `filteredKeys` 의 메타 매칭 분기를 그대로 재사용한다.
//
// @spec SPEC-WEB-005
// @spec SPEC-WEB-005 v0.7.0 (M16)
// @spec SPEC-WEB-005 v0.7.0 (Option A)

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import {
  AlertCircle,
  AlertTriangle,
  Check,
  Loader2,
  Play,
  X,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import {
  datetimeLocalToEpochMs,
  estimateBucketCount,
  isValidInterval,
  parseIntervalToMs,
  type TsdbAggregation,
  type TsdbFill,
} from '@/services/api/tsdb';
import type {
  SeriesDataSource,
  SeriesMatrix,
  SeriesMatrixQuery,
  SeriesSelectorFilter,
} from '@/services/api/seriesDataSource';
import { makeSeriesId } from '@/services/api/seriesLabels';
import { SeriesSelectTable, type SeriesRow } from './SeriesSelectTable';
import {
  useStoreKeysWithTags,
  useStoreTagPairs,
  type StoreKeyObject,
  type StoreKeyTagsMap,
  type StoreTagPair,
} from '@/services/api/store';

import SeriesResultMatrix from './TsdbResultMatrix';
import {
  buildExtractedTagPairs,
  buildExtractedTagsByKey,
  DEFAULT_SEGMENT_SEPARATOR,
  extractMetricTypeFromKey,
} from './keyTagExtractor';

/** 5,000행 초과 시 경고 임계치. */
const MATRIX_ROW_WARNING_THRESHOLD = 5000;

/** 하루(ms). 모달 오픈 시 기본 범위 산정에 사용. */
const ONE_DAY_MS = 24 * 60 * 60 * 1000;

/** 인터벌 프리셋. */
const INTERVAL_PRESETS = [
  '10s',
  '30s',
  '1m',
  '5m',
  '15m',
  '30m',
  '1h',
  '6h',
  '1d',
] as const;

type IntervalValue = (typeof INTERVAL_PRESETS)[number] | 'custom';

/** 집계 라디오 옵션. label 은 i18n 키이며 렌더 시 t(labelKey) 로 해석한다. */
const AGGREGATION_OPTIONS: { value: TsdbAggregation; labelKey: string }[] = [
  { value: 'min', labelKey: 'tsdb.aggMin' },
  { value: 'max', labelKey: 'tsdb.aggMax' },
  { value: 'average', labelKey: 'tsdb.aggAverage' },
  { value: 'first', labelKey: 'tsdb.aggFirst' },
  { value: 'last', labelKey: 'tsdb.aggLast' },
  { value: 'sum', labelKey: 'tsdb.aggSum' },
  { value: 'count', labelKey: 'tsdb.aggCount' },
];

/** 빈 버킷 채우기(gap-fill) 전략 옵션. 인터벌 구간에 값이 없을 때 적용. */
const FILL_OPTIONS: { value: TsdbFill; labelKey: string }[] = [
  { value: '', labelKey: 'tsdb.fillNone' },
  { value: 'null', labelKey: 'tsdb.fillNull' },
  { value: 'previous', labelKey: 'tsdb.fillPrevious' },
  { value: 'avg', labelKey: 'tsdb.fillAvg' },
  { value: 'zero', labelKey: 'tsdb.fillZero' },
];

/**
 * 상대 범위 프리셋 (지속 시간 ms).
 *
 * `label` 은 `RelativeSelectValue` 타입 식별자 겸 select value 로 사용되므로
 * 한국어 리터럴을 그대로 유지한다(기능 식별자). 화면 표시는 `RELATIVE_LABEL_KEY`
 * 의 i18n 키로 t() 해석한다.
 */
const RELATIVE_RANGE_PRESETS: { label: string; durationMs: number }[] = [
  { label: '지난 1시간', durationMs: 60 * 60 * 1000 },
  { label: '지난 6시간', durationMs: 6 * 60 * 60 * 1000 },
  { label: '지난 1일', durationMs: ONE_DAY_MS },
  { label: '지난 7일', durationMs: 7 * ONE_DAY_MS },
  { label: '지난 30일', durationMs: 30 * ONE_DAY_MS },
];

/** 상대 범위 프리셋 label → 표시용 i18n 키 매핑. */
const RELATIVE_LABEL_KEY: Record<string, string> = {
  '지난 1시간': 'tsdb.relLast1h',
  '지난 6시간': 'tsdb.relLast6h',
  '지난 1일': 'tsdb.relLast1d',
  '지난 7일': 'tsdb.relLast7d',
  '지난 30일': 'tsdb.relLast30d',
};

/**
 * 상대 모드 드롭다운 선택 값. 프리셋 문자열은 `RELATIVE_RANGE_PRESETS` 의 label 과
 * 1:1 대응되며, 'custom' 은 사용자 지정 duration 입력을 의미한다.
 */
type RelativeSelectValue =
  | '지난 1시간'
  | '지난 6시간'
  | '지난 1일'
  | '지난 7일'
  | '지난 30일'
  | 'custom';

const RELATIVE_DEFAULT: RelativeSelectValue = '지난 1일';

/** 시간 범위 입력 모드. 기본은 'absolute' (절대). */
type RangeMode = 'absolute' | 'relative';

/**
 * 주어진 절대 범위가 상대 프리셋과 일치(±1초 허용)하는지 찾아 해당 label 을 반환.
 * 일치하는 프리셋이 없으면 null.
 * 절대→상대 모드 전환 시 UX 개선용 매칭 로직.
 */
function matchRelativePreset(
  startMs: number,
  endMs: number,
  nowMs: number,
): RelativeSelectValue | null {
  // end 가 "현재 시각" 과 충분히 가까워야 (± 2초) 상대 범위로 해석 가능.
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return null;
  if (Math.abs(endMs - nowMs) > 2_000) return null;
  const duration = endMs - startMs;
  for (const p of RELATIVE_RANGE_PRESETS) {
    // 근사 일치(±1초) — datetime-local 은 초 단위 해상도라 완벽히 맞지 않을 수 있다.
    if (Math.abs(duration - p.durationMs) <= 1_000) {
      return p.label as RelativeSelectValue;
    }
  }
  return null;
}

/**
 * 로컬 타임존 기준 epoch ms 를 `YYYY-MM-DDTHH:mm` 형태로 포맷한다.
 * `<input type="datetime-local">` 의 value 에 직접 바인딩 가능하다.
 *
 * JS Date 의 toISOString 은 UTC 기준이므로, 로컬 타임존 오프셋을 빼고
 * 사용자가 보는 시각과 일치시킨다.
 */
function epochMsToDatetimeLocal(ms: number): string {
  const d = new Date(ms);
  const tzOffsetMs = d.getTimezoneOffset() * 60 * 1000;
  const local = new Date(ms - tzOffsetMs);
  // `2026-04-23T12:34` — 밀리초/초 이하 생략.
  return local.toISOString().slice(0, 16);
}

interface SeriesDataViewerModalProps {
  isOpen: boolean;
  onClose: () => void;
  /**
   * 모달 오픈 시 기본 선택할 시리즈 키.
   * v0.3.0 현재 상위 `SeriesTab` 은 단일 트리거 방식으로 전환되어 전달되지 않는다.
   * 과거 콜사이트 호환을 위해 optional 로만 남겨두었다.
   */
  initialSeriesKey?: string;
  /** 멀티셀렉트 옵션 풀. */
  allSeriesKeys: string[];
  /** TSDB/Store 공용 데이터 소스. */
  dataSource: SeriesDataSource;
  /**
   * 결과 CSV 내보내기 파일명에 사용할 에이전트 이름.
   * 미지정 시 `series` 로 대체된다 (하위 호환).
   */
  agentName?: string;
}

/** 태그 필터 섹션 훅의 반환 형상. */
interface StoreTagFilterState {
  /** 서버가 제공한 태그 쌍. 태그가 없으면 빈 배열. */
  pairs: StoreTagPair[];
  /** 키 → 태그맵 사전 (정적 키 전용, 동적 키는 미포함). */
  tagsByKey: StoreKeyTagsMap;
  /** 현재 선택된 "tagKey=tagValue" 필터. */
  selected: Set<string>;
  /** 선택 토글. */
  toggle: (filterId: string) => void;
  /** 전체 해제. */
  clearAll: () => void;
  /**
   * 키 → 메타데이터 객체 맵.
   *
   * Store 모드에서만 채워지며 (Phase A 의 `keyObjects` 응답에서 derived),
   * TSDB/그 외 모드에서는 빈 객체를 반환한다. v0.7.0 (M16) 메타데이터 칩
   * 표시와 data_type/field/registration 필터 적용에 사용된다.
   *
   * 같은 key 에 다중 시리즈가 있으면 마지막 시리즈가 대표값으로 들어간다.
   * 시리즈별 구분 표시는 `seriesByKey` 를 사용한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M16)
   */
  keyMetaByKey: Record<string, StoreKeyObject>;
  /**
   * 키 → 해당 key 의 모든 시리즈(metric/tags 별) 배열.
   *
   * @spec SPEC-STORE-004 (M5)
   * 백엔드 GET /keys 는 같은 key 를 metric/tags 별 다중 행으로 반환한다. 시리즈
   * 풀 리스트에서 한 key 아래 여러 시리즈를 "구분된 행" 으로 표시하기 위해, key
   * 단위로 그 key 의 모든 StoreKeyObject 를 모아둔다. Store 모드는 백엔드 응답을
   * 그대로 그룹화하고, TSDB/그 외 모드는 합성 메타 1개를 단일 원소 배열로 채운다.
   */
  seriesByKey: Record<string, StoreKeyObject[]>;
}

/**
 * Store 에이전트에 한해 태그 필터 관련 상태와 서버 데이터를 구독하는 래퍼 컴포넌트.
 *
 * useQuery 는 React 규칙상 조건부로 호출할 수 없으므로, kind 에 따라
 * "진짜 훅을 호출하는 서브컴포넌트" vs "키 자동 추출 기반 state" 를 선택한다.
 *
 * SPEC-WEB-005: 정적 태그가 없는 시리즈에 대해서도 키 구조 기반 자동 추출로
 * 태그 필터를 노출한다. Store 모드는 정적 태그 우선, 미보유 키는 자동 추출.
 * TSDB/그 외 모드는 자동 추출만 사용한다.
 *
 * @spec SPEC-WEB-005 SPEC-STORE-003
 */
function useStoreTagFilterState(
  kind: SeriesDataSource['kind'],
  agentName: string | undefined,
  allSeriesKeys: string[],
  separator: string,
): StoreTagFilterState {
  const isStore = kind === 'store';
  // TSDB 모드에서는 useQuery 를 아예 호출하지 않도록, 내부적으로
  // 두 경로를 분리한다. React Hooks 규칙은 "같은 렌더 트리에서 호출 순서가
  // 동일해야 한다" 이므로, kind 는 모달 세션 내내 고정됨을 전제로 한다
  // (`dataSource.kind` 는 agentType 에 바인딩되어 바뀌지 않음).
  if (isStore) {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    return useStoreTagFilterStateImpl(agentName, allSeriesKeys, separator);
  }
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return useExtractedTagFilterState(allSeriesKeys, separator);
}

/**
 * Store 모드 전용 구현.
 * useQuery 를 통해 태그 쌍과 키-태그 맵을 구독한다.
 *
 * SPEC-WEB-005: 정적 태그가 없는 키에 대해서는 `keyTagExtractor` 로
 * 키 패턴 기반 자동 추출을 수행하여 페어/키맵을 통합한다.
 */
function useStoreTagFilterStateImpl(
  agentName: string | undefined,
  allSeriesKeys: string[],
  separator: string,
): StoreTagFilterState {
  // tagPairsQuery 의 결과는 자동 추출과 통합되므로 직접 사용하지 않고,
  // keysWithTags 의 정적 태그 맵만 사용한다. (서버가 제공하는 페어 목록과
  // 자동 추출 페어 목록은 동일한 키 입력에서 합쳐져 일관된 페어를 만들어낸다.)
  void useStoreTagPairs(agentName);
  const keysWithTagsQuery = useStoreKeysWithTags(agentName);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());

  // staticTags 를 useMemo 로 안정화 — 매 렌더 새 객체가 만들어지면
  // 의존하는 useMemo 들이 매번 재계산되는 문제를 막는다.
  const staticTags = useMemo<Record<string, Record<string, string>>>(
    () => keysWithTagsQuery.data?.tags ?? {},
    [keysWithTagsQuery.data],
  );
  // SPEC-WEB-005 v0.7.0 (M16): 키 → StoreKeyObject 맵 (메타데이터 칩 + 신규 필터에 사용).
  // Phase A 의 `keyObjects` 응답을 키 단위 lookup 으로 변환한다.
  const keyMetaByKey = useMemo<Record<string, StoreKeyObject>>(() => {
    const map: Record<string, StoreKeyObject> = {};
    for (const obj of keysWithTagsQuery.data?.keyObjects ?? []) {
      map[obj.key] = obj;
    }
    return map;
  }, [keysWithTagsQuery.data]);
  // SPEC-STORE-004 (M5): key → 다중 시리즈 그룹. 백엔드 keyObjects 는 같은 key 를
  // metric/tags 별 여러 행으로 반환하므로, key 단위로 모아 "구분된 시리즈 행" 표시에 쓴다.
  const seriesByKey = useMemo<Record<string, StoreKeyObject[]>>(() => {
    const map: Record<string, StoreKeyObject[]> = {};
    for (const obj of keysWithTagsQuery.data?.keyObjects ?? []) {
      (map[obj.key] ??= []).push(obj);
    }
    return map;
  }, [keysWithTagsQuery.data]);
  // 모달은 부모로부터 받은 `allSeriesKeys` 를 정렬 기준 풀로 사용한다.
  // 서버의 keys 와 부모 풀이 다를 수 있으므로 양쪽 합집합을 채택한다.
  const allKeys = useMemo(() => {
    const set = new Set<string>(allSeriesKeys);
    for (const k of Object.keys(staticTags)) set.add(k);
    return [...set];
  }, [allSeriesKeys, staticTags]);

  const pairs = useMemo(
    () => buildExtractedTagPairs(allKeys, staticTags, separator),
    [allKeys, staticTags, separator],
  );
  const tagsByKey = useMemo(
    () => buildExtractedTagsByKey(allKeys, staticTags, separator),
    [allKeys, staticTags, separator],
  );

  const toggle = useCallback((filterId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(filterId)) {
        next.delete(filterId);
      } else {
        next.add(filterId);
      }
      return next;
    });
  }, []);

  const clearAll = useCallback(() => setSelected(new Set()), []);

  return { pairs, tagsByKey, selected, toggle, clearAll, keyMetaByKey, seriesByKey };
}

/**
 * TSDB(또는 비-Store) 모드용 자동 추출 기반 상태.
 *
 * 정적 태그 소스가 없으므로 `allSeriesKeys` 풀에서 키 구조만으로
 * 태그 페어/키맵을 빌드한다. 키에 추출 가능한 구조(InfluxDB / colon / slash)
 * 가 전혀 없으면 페어 배열은 빈 상태로 남으며, 모달은 태그 필터 섹션을
 * 자연스럽게 숨긴다.
 *
 * @spec SPEC-WEB-005
 */
function useExtractedTagFilterState(
  allSeriesKeys: string[],
  separator: string,
): StoreTagFilterState {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const pairs = useMemo(
    () => buildExtractedTagPairs(allSeriesKeys, {}, separator),
    [allSeriesKeys, separator],
  );
  const tagsByKey = useMemo(
    () => buildExtractedTagsByKey(allSeriesKeys, {}, separator),
    [allSeriesKeys, separator],
  );
  const toggle = useCallback((filterId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(filterId)) next.delete(filterId);
      else next.add(filterId);
      return next;
    });
  }, []);
  const clearAll = useCallback(() => setSelected(new Set()), []);
  // SPEC-WEB-005 v0.7.0 (Option A): TSDB 모드 합성 메타데이터.
  //   - field: 키 이름에서 자동 추출 (InfluxDB measurement / 첫 segment / 키 전체).
  //                  추출 실패한 빈 문자열은 'unknown' 으로 대체해 필터 매칭 가능하게 한다.
  //   - data_type: TSDB 시계열은 numeric 이므로 'float' 고정.
  //                필터에서 'float' 외 값을 선택하면 0개 매치 (의도적 동작).
  //   - registration: TSDB 에는 개념이 없어 임의로 'manual' 을 채우지만
  //                   UI 에서 registration 필터는 숨겨져 사용자에게 노출되지 않는다.
  //   - tags: extractedTagsByKey 결과를 그대로 매핑해 메타데이터 칩과 일관성 유지.
  // 이 합성 메타맵은 기존 `filteredKeys` 의 메타데이터 필터 분기를 그대로 재사용한다.
  const keyMetaByKey = useMemo<Record<string, StoreKeyObject>>(() => {
    const out: Record<string, StoreKeyObject> = {};
    for (const k of allSeriesKeys) {
      const metric = extractMetricTypeFromKey(k, separator);
      out[k] = {
        key: k,
        registration: 'manual',
        data_type: 'float',
        field: metric || 'unknown',
        tags: tagsByKey[k] ?? {},
      };
    }
    return out;
  }, [allSeriesKeys, separator, tagsByKey]);
  // SPEC-STORE-004 (M5): TSDB/그 외 모드는 키당 합성 시리즈 1개. keyMetaByKey 의
  // 각 항목을 단일 원소 배열로 감싸 Store 모드와 동일한 seriesByKey 형상을 제공한다.
  const seriesByKey = useMemo<Record<string, StoreKeyObject[]>>(() => {
    const out: Record<string, StoreKeyObject[]> = {};
    for (const [k, meta] of Object.entries(keyMetaByKey)) {
      out[k] = [meta];
    }
    return out;
  }, [keyMetaByKey]);
  return { pairs, tagsByKey, selected, toggle, clearAll, keyMetaByKey, seriesByKey };
}

function SeriesDataViewerModalImpl({
  isOpen,
  onClose,
  initialSeriesKey,
  allSeriesKeys,
  dataSource,
  agentName,
}: SeriesDataViewerModalProps) {
  const { t } = useTranslation();
  // --- 폼 상태 ---
  // 시리즈별 선택(저장소 기준 분류 #2): 선택 단위는 key 가 아니라
  // SeriesID(key + metric + tags). 단일 시리즈 key 는 SeriesID 가 곧 그 시리즈를
  // 가리키고, 다중 시리즈 key 는 각 시리즈가 독립적으로 선택된다.
  const [selectedSeriesIds, setSelectedSeriesIds] = useState<string[]>([]);
  const [startLocal, setStartLocal] = useState('');
  const [endLocal, setEndLocal] = useState('');
  const [intervalSelect, setIntervalSelect] = useState<IntervalValue>('1m');
  const [customInterval, setCustomInterval] = useState('');
  const [aggregation, setAggregation] = useState<TsdbAggregation>('average');
  // 빈 버킷 채우기(gap-fill) 전략. ''=빈 버킷 생략(기본). 인터벌 적용 시에만 의미.
  const [fill, setFill] = useState<TsdbFill>('');
  // SPEC-WEB-005: 평균 집계 시 표시할 소수점 자릿수 (0-6, 기본 1).
  // min/max 집계에서는 무시되며 원본 값이 그대로 표시된다.
  const [decimalPrecision, setDecimalPrecision] = useState<number>(1);

  // v0.3.0 Wave 2: 절대/상대 모드 탭 + 상대 드롭다운 상태.
  //   - `rangeMode`: 'relative' (기본) | 'absolute'.
  //   - `relativeSelect`: 상대 모드 드롭다운 선택(프리셋 또는 'custom').
  //   - `relativeCustom`: 커스텀 duration 입력 문자열 (예: '2h', '45m').
  // 모드 전환은 모달 세션 내에서 유지되며, 모달 오픈 시마다 'relative' 로 리셋된다.
  // 기본을 상대로 한 이유: 대부분의 모니터링 사용 사례에서 "지난 N시간/일" 형태의
  // 조회가 자연스럽고, 절대 시각 입력보다 학습 비용이 낮다.
  const [rangeMode, setRangeMode] = useState<RangeMode>('relative');
  const [relativeSelect, setRelativeSelect] = useState<RelativeSelectValue>(RELATIVE_DEFAULT);
  const [relativeCustom, setRelativeCustom] = useState('');

  // 세그먼트 구분자 — 키 패턴에서 태그를 자동 추출할 때 사용한다.
  // 구분자 필터링 UI 는 제거되었고(#3), 표준 기본 구분자로 고정한다.
  const separator = DEFAULT_SEGMENT_SEPARATOR;

  // 결과 표시 모드 (테이블/차트). 모달에서 보유하여 "다시 실행" 시
  // 결과 컴포넌트가 unmount/remount 되어도 사용자 선택이 보존되도록 한다.
  // 모달 오픈 시 'table' 로 리셋된다.
  const [resultViewMode, setResultViewMode] = useState<'table' | 'chart'>('table');

  // 5,000행 경고 확인 상태: pending 은 "경고 표시됨, 사용자 확정 대기 중".
  const [warningPending, setWarningPending] = useState(false);

  // 메타데이터/태그/검색 필터는 SeriesSelectTable 내부 상태로 이동했다(컬럼별 정렬+필터).

  // 매트릭스 쿼리 mutation — dataSource.queryMatrix 를 호출한다.
  const mutation = useMutation<SeriesMatrix, Error, SeriesMatrixQuery>({
    mutationFn: (params) => dataSource.queryMatrix(params),
  });

  // --- 태그 필터 ---
  // Store 모드: 정적 태그 + 자동 추출 통합 (SPEC-STORE-003 + SPEC-WEB-005).
  // TSDB(or other) 모드: 키 패턴 기반 자동 추출만 사용 (SPEC-WEB-005).
  const tagFilter = useStoreTagFilterState(
    dataSource.kind,
    agentName,
    allSeriesKeys,
    separator,
  );

  /**
   * SeriesID 인덱스 (#2 저장소 기준 분류).
   *
   * - `byId`: SeriesID → { key, obj? }. obj 는 StoreKeyObject(메타데이터가 있을 때).
   * - `idsByKey`: key → 그 key 에 속한 SeriesID 목록(렌더/일괄선택 순서 보존).
   *
   * 메타데이터가 전혀 없는 key 는 "암묵적 단일 시리즈"로 취급하여 SeriesID = key
   * 로 둔다(필터 없이 key 단위 조회 → 기존 동작 보존).
   */
  const seriesIndex = useMemo(() => {
    const byId = new Map<string, { key: string; obj?: StoreKeyObject }>();
    const idsByKey = new Map<string, string[]>();
    const keysUnion = new Set<string>([
      ...allSeriesKeys,
      ...Object.keys(tagFilter.seriesByKey),
    ]);
    for (const k of keysUnion) {
      const arr = tagFilter.seriesByKey[k] ?? [];
      const ids: string[] = [];
      if (arr.length === 0) {
        // 메타데이터 없는 key → 암묵적 단일 시리즈(SeriesID = key).
        byId.set(k, { key: k });
        ids.push(k);
      } else {
        for (const o of arr) {
          const id = makeSeriesId(o.key, o.field, o.tags);
          byId.set(id, { key: o.key, obj: o });
          ids.push(id);
        }
      }
      idsByKey.set(k, ids);
    }
    return { byId, idsByKey };
  }, [allSeriesKeys, tagFilter.seriesByKey]);

  const seriesIdsForKey = useCallback(
    (key: string): string[] => seriesIndex.idsByKey.get(key) ?? [key],
    [seriesIndex],
  );

  // 시리즈 선택 테이블 행 — seriesIndex 와 동일한 SeriesID 를 사용해 선택 정합성을 보장한다.
  // 메타데이터(metric/data_type/registration/tags)는 StoreKeyObject 또는 합성 keyMeta 에서 채운다.
  const allRows = useMemo<SeriesRow[]>(() => {
    const rows: SeriesRow[] = [];
    for (const [id, { key, obj }] of seriesIndex.byId) {
      const meta = obj ?? tagFilter.keyMetaByKey[key];
      rows.push({
        id,
        key,
        field: meta?.field ?? '',
        dataType: meta?.data_type ?? '',
        registration: meta?.registration ?? '',
        tags: meta?.tags ?? {},
      });
    }
    return rows;
  }, [seriesIndex, tagFilter.keyMetaByKey]);

  // 일괄 선택/해제 — 테이블이 필터링한 id 목록을 받아 선택 집합에 가감한다.
  const selectManyIds = useCallback((ids: string[]) => {
    setSelectedSeriesIds((prev) => Array.from(new Set([...prev, ...ids])));
  }, []);
  const clearManyIds = useCallback((ids: string[]) => {
    setSelectedSeriesIds((prev) => prev.filter((id) => !ids.includes(id)));
  }, []);

  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusRef = useRef<HTMLInputElement>(null);

  // --- 모달 오픈 시 상태 초기화 ---
  // 시간 범위는 매번 "지난 1일" 로 리셋한다 (사용자 입력은 오픈 중에만 보존).
  // 모드는 매번 'absolute' 로 리셋되어 기존 기본 동작을 보존한다.
  // initialSeriesKey(키 문자열)를 SeriesID 로 해석하기 위한 대기 표식.
  // 모달 오픈 시점에는 Store 메타데이터(seriesByKey)가 아직 로드되지 않았을 수
  // 있으므로, 즉시 해석을 시도하되 실패하면 아래 resolver 가 뒤늦게 채운다.
  const pendingInitialKeyRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!isOpen) return;
    pendingInitialKeyRef.current = initialSeriesKey;
    setSelectedSeriesIds(initialSeriesKey ? seriesIdsForKey(initialSeriesKey) : []);
    const nowMs = Date.now();
    setEndLocal(epochMsToDatetimeLocal(nowMs));
    setStartLocal(epochMsToDatetimeLocal(nowMs - ONE_DAY_MS));
    setIntervalSelect('1m');
    setCustomInterval('');
    setAggregation('average');
    setDecimalPrecision(1);
    setRangeMode('relative');
    setRelativeSelect(RELATIVE_DEFAULT);
    setRelativeCustom('');
    setFill('');
    setResultViewMode('table');
    setWarningPending(false);
    mutation.reset();
    // mutation 은 ref-stable 해야 하지만 완벽히 안전하진 않으므로 exhaustive-deps 무시.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, initialSeriesKey]);

  // initialSeriesKey resolver: 오픈 직후 Store 메타데이터가 비어 SeriesID 해석에
  // 실패했다면, seriesByKey 가 로드되는 시점에 한 번 더 시도해 초기 선택을 채운다.
  useEffect(() => {
    if (!isOpen) return;
    const k = pendingInitialKeyRef.current;
    if (!k) return;
    const ids = seriesIdsForKey(k);
    // 암묵적 단일 시리즈(ids === [k]) 는 이미 오픈 시 반영되었으므로,
    // 메타데이터 기반 SeriesID 가 새로 확보되었을 때만 갱신한다.
    if (ids.length > 0 && !(ids.length === 1 && ids[0] === k)) {
      setSelectedSeriesIds(ids);
      pendingInitialKeyRef.current = undefined;
    }
  }, [isOpen, seriesIdsForKey]);

  // --- Esc 키 / 배경 클릭 닫기 ---
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !mutation.isPending) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, mutation.isPending]);

  // --- 포커스 트랩 초기화 ---
  useEffect(() => {
    if (!isOpen) return;
    // 다음 tick 에서 첫 입력 필드에 포커스.
    const id = window.setTimeout(() => {
      firstFocusRef.current?.focus();
    }, 0);
    return () => window.clearTimeout(id);
  }, [isOpen]);

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !mutation.isPending) {
        onClose();
      }
    },
    [onClose, mutation.isPending],
  );

  // --- 파생 값들 ---

  /** 현재 유효한 인터벌 문자열 (custom 이면 customInterval, 아니면 프리셋 값). */
  const effectiveInterval = useMemo(() => {
    return intervalSelect === 'custom' ? customInterval.trim() : intervalSelect;
  }, [intervalSelect, customInterval]);

  /**
   * 상대 모드에서 현재 선택된 duration(ms) 을 반환한다.
   * 프리셋이면 상수, 'custom' 이면 사용자 입력을 Go duration 으로 파싱.
   * 유효하지 않으면 NaN.
   */
  const relativeDurationMs = useMemo(() => {
    if (relativeSelect === 'custom') {
      return parseIntervalToMs(relativeCustom.trim());
    }
    const preset = RELATIVE_RANGE_PRESETS.find((p) => p.label === relativeSelect);
    return preset ? preset.durationMs : Number.NaN;
  }, [relativeSelect, relativeCustom]);

  /** 상대 커스텀 duration 유효성 (상대 모드 + 'custom' 선택일 때만 검증). */
  const relativeCustomValid = useMemo(() => {
    if (rangeMode !== 'relative') return true;
    if (relativeSelect !== 'custom') return true;
    return isValidInterval(relativeCustom.trim());
  }, [rangeMode, relativeSelect, relativeCustom]);

  // 절대 모드에서는 datetime-local 입력값을 사용, 상대 모드에서는 프리뷰용으로만
  // 현재 duration 을 기반으로 "대략의 start/end" 를 계산한다(실행 시점엔 재계산).
  const absoluteStartMs = useMemo(
    () => datetimeLocalToEpochMs(startLocal),
    [startLocal],
  );
  const absoluteEndMs = useMemo(
    () => datetimeLocalToEpochMs(endLocal),
    [endLocal],
  );

  /**
   * 실행 시점에 사용할 `{startMs, endMs}` 를 계산한다.
   * - 절대: datetime-local 입력값 그대로.
   * - 상대: `Date.now()` 기준으로 `start = now - duration, end = now`.
   *   `relativeDurationMs` 가 NaN 이면 `{NaN, NaN}` 반환.
   */
  const resolveQueryRange = useCallback((): { startMs: number; endMs: number } => {
    if (rangeMode === 'relative') {
      if (!Number.isFinite(relativeDurationMs) || relativeDurationMs <= 0) {
        return { startMs: Number.NaN, endMs: Number.NaN };
      }
      const nowMs = Date.now();
      return { startMs: nowMs - relativeDurationMs, endMs: nowMs };
    }
    return { startMs: absoluteStartMs, endMs: absoluteEndMs };
  }, [rangeMode, relativeDurationMs, absoluteStartMs, absoluteEndMs]);

  const intervalValid = useMemo(() => {
    if (intervalSelect !== 'custom') return true;
    return isValidInterval(customInterval.trim());
  }, [intervalSelect, customInterval]);

  /**
   * 현재 입력값이 쿼리 가능한 시간 범위인지 검증한다.
   * - 절대: datetime-local 값이 유효하고 end > start.
   * - 상대: duration 이 양수이고 파싱 가능.
   *
   * 상대 모드는 실행 시점의 `now` 에 의존하므로 초 단위 유효성은 항상 성립한다
   * (음수가 아닌 duration 이라면 now > now - duration 자명).
   */
  const timeRangeValid = useMemo(() => {
    if (rangeMode === 'relative') {
      return (
        Number.isFinite(relativeDurationMs) && relativeDurationMs > 0 && relativeCustomValid
      );
    }
    if (!Number.isFinite(absoluteStartMs) || !Number.isFinite(absoluteEndMs)) return false;
    return absoluteEndMs > absoluteStartMs;
  }, [rangeMode, relativeDurationMs, relativeCustomValid, absoluteStartMs, absoluteEndMs]);

  /**
   * 예상 버킷 수. 경고 임계치(5,000행) 판정에 사용된다.
   * 상대 모드는 duration 기반으로 직접 계산 (now 의존성 제거 → 안정적).
   */
  const expectedBuckets = useMemo(() => {
    if (!timeRangeValid || !intervalValid) return 0;
    if (rangeMode === 'relative') {
      const intervalMs = parseIntervalToMs(effectiveInterval);
      if (!Number.isFinite(intervalMs) || intervalMs <= 0) return 0;
      return Math.ceil(relativeDurationMs / intervalMs);
    }
    return estimateBucketCount(absoluteStartMs, absoluteEndMs, effectiveInterval);
  }, [
    timeRangeValid,
    intervalValid,
    rangeMode,
    relativeDurationMs,
    absoluteStartMs,
    absoluteEndMs,
    effectiveInterval,
  ]);

  const canExecute =
    selectedSeriesIds.length > 0 &&
    timeRangeValid &&
    intervalValid &&
    !mutation.isPending;

  // --- 핸들러 ---
  // 검색/태그/메타 필터링은 SeriesSelectTable 내부로 이동했다. 모달은 선택 토글과
  // 일괄 선택/해제(selectManyIds/clearManyIds, 위에서 정의)만 보유한다.

  const toggleSeries = useCallback((id: string) => {
    setSelectedSeriesIds((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id);
      return [...prev, id];
    });
  }, []);

  /**
   * 절대 모드의 상대 범위 프리셋 버튼 핸들러.
   * 클릭 시점의 현재 시각을 endMs 로, endMs - duration 을 startMs 로 채운다.
   * 쿼리를 자동 실행하지는 않는다 — 사용자가 "실행" 을 명시적으로 눌러야 한다.
   */
  const handleRelativeRange = useCallback((durationMs: number) => {
    const nowMs = Date.now();
    setEndLocal(epochMsToDatetimeLocal(nowMs));
    setStartLocal(epochMsToDatetimeLocal(nowMs - durationMs));
  }, []);

  /**
   * 절대 ↔ 상대 모드 탭 전환 핸들러.
   *
   * - 절대→상대: 현재 절대 범위가 상대 프리셋과 일치하면 해당 프리셋 선택,
   *   일치하지 않으면 '지난 1일' 기본값으로 초기화.
   * - 상대→절대: 현재 상대 duration 을 기준으로 now - duration 을 start,
   *   now 를 end 로 채워 사용자가 즉시 datetime-local 에서 이어가도록 한다.
   *
   * 동일 모드 재선택은 no-op.
   */
  const handleRangeModeChange = useCallback(
    (next: RangeMode) => {
      if (next === rangeMode) return;
      if (next === 'relative') {
        const match = matchRelativePreset(absoluteStartMs, absoluteEndMs, Date.now());
        setRelativeSelect(match ?? RELATIVE_DEFAULT);
        setRelativeCustom('');
        setRangeMode('relative');
        return;
      }
      // 상대 → 절대: 실제 now 시각으로 start/end 채우기.
      if (Number.isFinite(relativeDurationMs) && relativeDurationMs > 0) {
        const nowMs = Date.now();
        setEndLocal(epochMsToDatetimeLocal(nowMs));
        setStartLocal(epochMsToDatetimeLocal(nowMs - relativeDurationMs));
      }
      setRangeMode('absolute');
    },
    [rangeMode, absoluteStartMs, absoluteEndMs, relativeDurationMs],
  );

  // 쿼리 실행에 사용된 마지막 {startMs, endMs} — 결과 CSV 파일명 구성에 필요.
  // mutate 호출 직후에 세팅되어, 이후 mutation.data 와 함께 사용된다.
  const [lastQueryRange, setLastQueryRange] = useState<{
    startMs: number;
    endMs: number;
  } | null>(null);

  const performQuery = useCallback(() => {
    if (!canExecute) return;
    // 선택된 SeriesID 를 (key, seriesFilter) 쌍으로 환원한다(#2 저장소 기준 분류).
    //   - 단일 시리즈 key: filter 없이 key 단위 조회(기존 동작 보존).
    //   - 다중 시리즈 key: 같은 key 가 여러 번 등장하며 각자 metric/tags 필터로 구분.
    const orderedKeys: string[] = [];
    const seriesFilters: Array<SeriesSelectorFilter | undefined> = [];
    for (const id of selectedSeriesIds) {
      const entry = seriesIndex.byId.get(id);
      if (!entry) continue;
      const arr = tagFilter.seriesByKey[entry.key] ?? [];
      const isMulti = arr.length > 1;
      orderedKeys.push(entry.key);
      seriesFilters.push(
        isMulti && entry.obj
          ? { fieldName: entry.obj.field, tags: entry.obj.tags }
          : undefined,
      );
    }
    if (orderedKeys.length === 0) return;
    const anyFilter = seriesFilters.some((f) => f !== undefined);
    // Go duration 을 milliseconds 로 역환산. 유효성은 위에서 이미 확인됨.
    const intervalMs = parseIntervalToMs(effectiveInterval);
    // 상대 모드는 실행 시점의 now 로 재계산된다.
    const { startMs: resolvedStart, endMs: resolvedEnd } = resolveQueryRange();
    if (!Number.isFinite(resolvedStart) || !Number.isFinite(resolvedEnd)) return;
    setLastQueryRange({ startMs: resolvedStart, endMs: resolvedEnd });
    mutation.mutate({
      keys: orderedKeys,
      ...(anyFilter ? { seriesFilters } : {}),
      startMs: resolvedStart,
      endMs: resolvedEnd,
      intervalMs,
      aggregation,
      fill,
    });
  }, [
    canExecute,
    selectedSeriesIds,
    seriesIndex,
    tagFilter.seriesByKey,
    resolveQueryRange,
    effectiveInterval,
    aggregation,
    fill,
    mutation,
  ]);

  const handleExecuteClick = useCallback(() => {
    if (!canExecute) return;
    // 5,000 행 초과 예상 시 경고 확정 후에만 실행.
    if (expectedBuckets > MATRIX_ROW_WARNING_THRESHOLD && !warningPending) {
      setWarningPending(true);
      return;
    }
    setWarningPending(false);
    performQuery();
  }, [canExecute, expectedBuckets, warningPending, performQuery]);

  const handleConfirmWarning = useCallback(() => {
    setWarningPending(false);
    performQuery();
  }, [performQuery]);

  const handleCancelWarning = useCallback(() => {
    setWarningPending(false);
  }, []);

  // --- 서버 에러 메시지 ---
  const errorMessage = useMemo(() => {
    if (!mutation.isError) return null;
    const err = mutation.error;
    if (err instanceof Error) return err.message;
    return t('tsdb.queryFailed');
  }, [mutation.isError, mutation.error, t]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="tsdb-viewer-title"
    >
      {/*
        모달 컨테이너 — 뷰포트의 95% 를 차지한다.
        flex-col + overflow-hidden 으로 헤더/본문/푸터를 내부에서 분할한다.
        본문만 flex-1 + overflow-auto 로 스크롤되고, 폼 영역은 자연스러운 높이를 유지한다.
      */}
      <div
        ref={modalRef}
        data-testid="tsdb-viewer-modal-container"
        className="mx-4 flex h-[95vh] w-[95vw] max-w-[1600px] flex-col overflow-hidden rounded-lg bg-(--color-bg-surface) shadow-xl"
      >
        {/* 헤더 */}
        <div className="flex flex-shrink-0 items-center justify-between border-b border-(--color-border-default) px-6 py-4">
          <div className="flex items-center gap-2">
            <h2
              id="tsdb-viewer-title"
              className="text-lg font-semibold text-(--color-text-primary)"
            >
              {t('tsdb.title')}
            </h2>
            {mutation.isPending && (
              <Loader2
                className="h-4 w-4 animate-spin text-blue-500"
                aria-label={t('tsdb.queryRunningAria')}
              />
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label={t('common.close')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/*
          폼 영역 — 자연 높이를 갖되, 화면이 작을 때 매트릭스/푸터가 가려지지 않도록
          `min-h-0 overflow-y-auto` 로 자체 스크롤을 허용한다.
          `flex-shrink-0` 을 제거해 부모 flex-col 에서 공간 부족 시 줄어들 수 있게 한다.
        */}
        <div className="min-h-0 shrink overflow-y-auto space-y-4 border-b border-(--color-border-default) px-6 py-4">
          {/*
            시리즈 선택 — 저장소(StoreTab) 스타일의 정렬/필터 테이블.
            각 컬럼에 정렬 + 다중선택 필터(facet)가 있고, 행 체크박스로 조회 대상을
            선택한다. 필터/정렬 상태는 SeriesSelectTable 내부에서 관리한다.
          */}
          <fieldset data-testid="series-select-fieldset">
            <legend className="mb-2 block text-sm font-medium text-(--color-text-secondary)">
              {t('tsdb.seriesSelect').replace(
                '{count}',
                String(selectedSeriesIds.length),
              )}
            </legend>
            <SeriesSelectTable
              rows={allRows}
              selectedIds={selectedSeriesIds}
              onToggle={toggleSeries}
              onSelectMany={selectManyIds}
              onClearMany={clearManyIds}
              showRegistration={dataSource.kind === 'store'}
            />
          </fieldset>

          {/*
            시간 범위 모드 탭 (v0.3.0 Wave 2):
              - 절대 (기본): datetime-local 입력 + 빠른 범위 버튼.
              - 상대: 드롭다운으로 duration 선택. 실제 시간은 실행 시점의 now 기준.
            role=tablist 로 탭 시맨틱을 명시해 스크린리더 접근성을 보장한다.
          */}
          <div className="space-y-2">
            <div
              className="inline-flex rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) p-0.5"
              role="tablist"
              aria-label={t('tsdb.rangeModeAria')}
            >
              {(['absolute', 'relative'] as const).map((mode) => {
                const label =
                  mode === 'absolute'
                    ? t('tsdb.rangeAbsolute')
                    : t('tsdb.rangeRelative');
                const selected = rangeMode === mode;
                return (
                  <button
                    key={mode}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    data-testid={`tsdb-range-mode-${mode}`}
                    onClick={() => handleRangeModeChange(mode)}
                    className={`rounded px-3 py-1 text-xs font-medium transition-colors ${
                      selected
                        ? 'bg-blue-600 text-white'
                        : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
                    }`}
                  >
                    {label}
                  </button>
                );
              })}
            </div>

            {/*
              2열 레이아웃: 좌측 = 범위(절대/상대) + 인터벌, 우측 = 집계 함수.
              모드 탭은 위에 풀폭으로 유지되며, 선택된 모드의 범위 컨트롤이
              좌측 컬럼 상단에 노출되고 그 아래에 인터벌이 따라온다.
            */}
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              {/* 좌측: 범위 + 인터벌 */}
              <div className="flex flex-col gap-4">
                {/*
                  범위 컨트롤 영역 — 절대 모드(빠른선택 + datetime 2개) 의
                  자연 높이를 `min-h-[7.5rem]` 로 reserve 하여 절대↔상대 전환 시
                  하단의 인터벌이 위/아래로 움직이지 않도록 한다.
                  상대 모드는 본 영역 안에서 짧게 차지하고 남는 공간은 비워둔다.
                */}
                <div className="min-h-[7.5rem]">
                {rangeMode === 'absolute' ? (
                  <div className="flex flex-col gap-2">
                    {/* 절대 모드: 빠른 범위 버튼 + datetime-local 입력 */}
                    <div
                      className="flex flex-wrap items-center gap-1.5"
                      role="group"
                      aria-label={t('tsdb.quickRangeAria')}
                    >
                      <span className="mr-1 text-xs text-(--color-text-muted)">
                        {t('tsdb.quickSelect')}
                      </span>
                      {RELATIVE_RANGE_PRESETS.map((preset) => (
                        <button
                          key={preset.label}
                          type="button"
                          onClick={() => handleRelativeRange(preset.durationMs)}
                          className="rounded-full border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-0.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
                        >
                          {t(RELATIVE_LABEL_KEY[preset.label] ?? preset.label)}
                        </button>
                      ))}
                    </div>
                    <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                      <div>
                        <label
                          htmlFor="tsdb-start"
                          className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                        >
                          {t('tsdb.startTime')}
                        </label>
                        <input
                          id="tsdb-start"
                          type="datetime-local"
                          value={startLocal}
                          onChange={(e) => setStartLocal(e.target.value)}
                          className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        />
                      </div>
                      <div>
                        <label
                          htmlFor="tsdb-end"
                          className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                        >
                          {t('tsdb.endTime')}
                        </label>
                        <input
                          id="tsdb-end"
                          type="datetime-local"
                          value={endLocal}
                          onChange={(e) => setEndLocal(e.target.value)}
                          className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        />
                      </div>
                    </div>
                    {startLocal && endLocal && !timeRangeValid && (
                      <p className="text-xs text-red-600 dark:text-red-400">
                        {t('tsdb.endAfterStart')}
                      </p>
                    )}
                  </div>
                ) : (
                  <div className="flex flex-col gap-2">
                    {/* 상대 모드: 프리셋 드롭다운 + 커스텀 duration 입력 */}
                    <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                      <div>
                        <label
                          htmlFor="tsdb-relative-range"
                          className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                        >
                          {t('tsdb.range')}
                        </label>
                        <select
                          id="tsdb-relative-range"
                          value={relativeSelect}
                          onChange={(e) =>
                            setRelativeSelect(e.target.value as RelativeSelectValue)
                          }
                          className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        >
                          {RELATIVE_RANGE_PRESETS.map((p) => (
                            <option key={p.label} value={p.label}>
                              {t(RELATIVE_LABEL_KEY[p.label] ?? p.label)}
                            </option>
                          ))}
                          <option value="custom">{t('tsdb.custom')}</option>
                        </select>
                      </div>
                      {relativeSelect === 'custom' && (
                        <div>
                          <label
                            htmlFor="tsdb-relative-custom"
                            className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                          >
                            {t('tsdb.customDuration')}
                          </label>
                          <input
                            id="tsdb-relative-custom"
                            type="text"
                            placeholder={t('tsdb.durationPlaceholder')}
                            value={relativeCustom}
                            onChange={(e) => setRelativeCustom(e.target.value)}
                            className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 font-mono text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          />
                          {!relativeCustomValid && relativeCustom.trim() !== '' && (
                            <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                              {t('tsdb.goDurationSyntax')}
                            </p>
                          )}
                        </div>
                      )}
                    </div>
                    <p className="text-xs text-(--color-text-muted)">
                      {t('tsdb.relativeHint')}
                    </p>
                  </div>
                )}
                </div>

                {/* 인터벌 — 좌측 컬럼 하단에 위치 (디자인: 인터벌은 범위 아래) */}
                <div>
                  <label
                    htmlFor="tsdb-interval"
                    className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                  >
                    {t('tsdb.interval')}
                  </label>
                  <select
                    id="tsdb-interval"
                    value={intervalSelect}
                    onChange={(e) => setIntervalSelect(e.target.value as IntervalValue)}
                    className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  >
                    {INTERVAL_PRESETS.map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                    <option value="custom">{t('tsdb.intervalCustom')}</option>
                  </select>
                  {intervalSelect === 'custom' && (
                    <div className="mt-1.5">
                      <input
                        type="text"
                        placeholder={t('tsdb.intervalPlaceholder')}
                        value={customInterval}
                        onChange={(e) => setCustomInterval(e.target.value)}
                        className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 font-mono text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      />
                      {!intervalValid && customInterval.trim() !== '' && (
                        <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                          {t('tsdb.goDurationSyntax')}
                        </p>
                      )}
                    </div>
                  )}
                </div>
              </div>

              {/* 우측: 집계 함수 + 소수점 자릿수 */}
              <fieldset>
                <legend className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  {t('tsdb.aggregation')}
                </legend>
                <div className="flex items-center gap-3">
                  {AGGREGATION_OPTIONS.map((opt) => (
                    <label
                      key={opt.value}
                      className="inline-flex items-center gap-1.5 text-sm text-(--color-text-primary)"
                    >
                      <input
                        type="radio"
                        name="tsdb-aggregation"
                        value={opt.value}
                        checked={aggregation === opt.value}
                        onChange={() => setAggregation(opt.value)}
                        className="h-3.5 w-3.5 border-gray-300 text-blue-600"
                      />
                      {t(opt.labelKey)}
                    </label>
                  ))}
                </div>
                {/*
                  SPEC-WEB-005: 평균 집계일 때만 노출되는 소수점 자릿수 입력.
                  min/max 집계는 원본 값을 그대로 보여주므로 자릿수 설정이 무의미하다.
                */}
                {aggregation === 'average' && (
                  <div className="mt-2">
                    <label
                      htmlFor="tsdb-decimal-precision"
                      className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
                    >
                      {t('tsdb.decimalPrecision')}
                    </label>
                    <input
                      id="tsdb-decimal-precision"
                      data-testid="tsdb-decimal-precision"
                      type="number"
                      min={0}
                      max={6}
                      step={1}
                      value={decimalPrecision}
                      onChange={(e) => {
                        const raw = Number(e.target.value);
                        if (!Number.isFinite(raw)) return;
                        // 0-6 범위로 클램프 — 음수/큰 값은 사용성 저하만 야기.
                        const clamped = Math.max(0, Math.min(6, Math.floor(raw)));
                        setDecimalPrecision(clamped);
                      }}
                      className="block w-24 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                    />
                    <p className="mt-1 text-xs text-(--color-text-muted)">
                      {t('tsdb.decimalPrecisionHint')}
                    </p>
                  </div>
                )}

                {/* 빈 버킷 채우기(gap-fill): 인터벌 구간에 값이 없을 때 처리 방법 */}
                <div className="mt-3">
                  <label
                    htmlFor="tsdb-fill"
                    className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
                  >
                    {t('tsdb.fill')}
                  </label>
                  <select
                    id="tsdb-fill"
                    data-testid="tsdb-fill"
                    value={fill}
                    onChange={(e) => setFill(e.target.value as TsdbFill)}
                    className="block w-44 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  >
                    {FILL_OPTIONS.map((opt) => (
                      <option key={opt.value || 'none'} value={opt.value}>
                        {t(opt.labelKey)}
                      </option>
                    ))}
                  </select>
                  <p className="mt-1 text-xs text-(--color-text-muted)">
                    {t('tsdb.fillHint')}
                  </p>
                </div>
              </fieldset>
            </div>
          </div>

          {/* 경고 배너 (5,000행 초과) */}
          {warningPending && (
            <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-700 dark:bg-amber-950">
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
                <div className="flex-1">
                  <p className="font-medium text-amber-800 dark:text-amber-200">
                    {t('tsdb.warningManyRows')}
                  </p>
                  <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                    {t('tsdb.warningRowCount')
                      .replace('{count}', expectedBuckets.toLocaleString())
                      .replace(
                        '{threshold}',
                        MATRIX_ROW_WARNING_THRESHOLD.toLocaleString(),
                      )}
                  </p>
                  <div className="mt-2 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={handleConfirmWarning}
                      className="rounded-md bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-700"
                    >
                      {t('tsdb.warningContinue')}
                    </button>
                    <button
                      type="button"
                      onClick={handleCancelWarning}
                      className="rounded-md border border-amber-400 px-3 py-1 text-xs font-medium text-amber-700 hover:bg-amber-100 dark:border-amber-600 dark:text-amber-300 dark:hover:bg-amber-900"
                    >
                      {t('tsdb.warningCancel')}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* 에러 배너 */}
          {errorMessage && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
                <p className="text-sm text-red-700 dark:text-red-300">{errorMessage}</p>
              </div>
            </div>
          )}
        </div>

        {/*
          결과 매트릭스 영역 — 남은 세로 공간을 모두 차지하며 독립 스크롤.
          가로가 긴 매트릭스도 스크롤로 접근 가능.
          작은 화면에서도 결과가 너무 압축되지 않도록 최소 높이를 보장한다 (min-h-[180px]).
        */}
        <div
          className="flex-1 min-h-[180px] overflow-auto px-6 py-4"
          data-testid="tsdb-viewer-result-scroll"
        >
          {mutation.isSuccess && mutation.data && mutation.data.columns.length > 0 ? (
            <section aria-label={t('tsdb.resultAria')}>
              <h3 className="mb-2 text-sm font-semibold text-(--color-text-primary)">
                {t('tsdb.resultMatrix')}
              </h3>
              <SeriesResultMatrix
                matrix={mutation.data}
                agentName={agentName}
                exportStartMs={lastQueryRange?.startMs}
                exportEndMs={lastQueryRange?.endMs}
                aggregation={aggregation}
                decimalPrecision={decimalPrecision}
                viewMode={resultViewMode}
                onViewModeChange={setResultViewMode}
              />
            </section>
          ) : (
            <p className="text-center text-xs text-(--color-text-muted)">
              {t('tsdb.emptyState')}
            </p>
          )}
        </div>

        {/* 푸터 */}
        <div className="flex flex-shrink-0 items-center justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            {t('tsdb.close')}
          </button>
          <button
            type="button"
            onClick={handleExecuteClick}
            disabled={!canExecute}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {mutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('tsdb.running')}
              </>
            ) : mutation.isSuccess ? (
              <>
                <Check className="h-4 w-4" />
                {t('tsdb.rerun')}
              </>
            ) : (
              <>
                <Play className="h-4 w-4" />
                {t('tsdb.run')}
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---- Public exports ----

/** TSDB/Store 공용 데이터 뷰어 모달 (명시적 명칭). */
export const SeriesDataViewerModal = SeriesDataViewerModalImpl;

/** 기존 콜사이트 호환을 위한 default export. */
export default SeriesDataViewerModalImpl;
