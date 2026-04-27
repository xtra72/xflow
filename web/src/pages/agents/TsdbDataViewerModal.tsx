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
// @spec SPEC-WEB-005

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import {
  AlertCircle,
  AlertTriangle,
  Check,
  Loader2,
  Play,
  Search,
  X,
} from 'lucide-react';

import {
  datetimeLocalToEpochMs,
  estimateBucketCount,
  isValidInterval,
  parseIntervalToMs,
  type TsdbAggregation,
} from '@/services/api/tsdb';
import type {
  SeriesDataSource,
  SeriesMatrix,
  SeriesMatrixQuery,
} from '@/services/api/seriesDataSource';
import {
  useStoreKeysWithTags,
  useStoreTagPairs,
  type StoreKeyTagsMap,
  type StoreTagPair,
} from '@/services/api/store';
import {
  TagFilterChips,
  matchesTagFilter,
} from '@/components/property/TagFilterChips';

import SeriesResultMatrix from './TsdbResultMatrix';

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

/** 집계 라디오 옵션. */
const AGGREGATION_OPTIONS: { value: TsdbAggregation; label: string }[] = [
  { value: 'min', label: '최소 (min)' },
  { value: 'max', label: '최대 (max)' },
  { value: 'average', label: '평균 (average)' },
];

/** 상대 범위 프리셋 (지속 시간 ms). */
const RELATIVE_RANGE_PRESETS: { label: string; durationMs: number }[] = [
  { label: '지난 1시간', durationMs: 60 * 60 * 1000 },
  { label: '지난 6시간', durationMs: 6 * 60 * 60 * 1000 },
  { label: '지난 1일', durationMs: ONE_DAY_MS },
  { label: '지난 7일', durationMs: 7 * ONE_DAY_MS },
  { label: '지난 30일', durationMs: 30 * ONE_DAY_MS },
];

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
}

/**
 * Store 에이전트에 한해 태그 필터 관련 상태와 서버 데이터를 구독하는 래퍼 컴포넌트.
 *
 * useQuery 는 React 규칙상 조건부로 호출할 수 없으므로, kind 에 따라
 * "진짜 훅을 호출하는 서브컴포넌트" vs "no-op state" 를 선택한다.
 *
 * @spec SPEC-STORE-003
 */
function useStoreTagFilterState(
  kind: SeriesDataSource['kind'],
  agentName: string | undefined,
): StoreTagFilterState {
  const isStore = kind === 'store';
  // TSDB 모드에서는 useQuery 를 아예 호출하지 않도록, 내부적으로
  // 두 경로를 분리한다. React Hooks 규칙은 "같은 렌더 트리에서 호출 순서가
  // 동일해야 한다" 이므로, kind 는 모달 세션 내내 고정됨을 전제로 한다
  // (`dataSource.kind` 는 agentType 에 바인딩되어 바뀌지 않음).
  if (isStore) {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    return useStoreTagFilterStateImpl(agentName);
  }
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return useNoopTagFilterState();
}

/**
 * Store 모드 전용 구현.
 * useQuery 를 통해 태그 쌍과 키-태그 맵을 구독한다.
 */
function useStoreTagFilterStateImpl(
  agentName: string | undefined,
): StoreTagFilterState {
  const tagPairsQuery = useStoreTagPairs(agentName);
  const keysWithTagsQuery = useStoreKeysWithTags(agentName);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());

  const pairs = tagPairsQuery.data ?? [];
  const tagsByKey = keysWithTagsQuery.data?.tags ?? {};

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

  return { pairs, tagsByKey, selected, toggle, clearAll };
}

/**
 * TSDB 모드용 no-op 상태.
 * useQuery 호출 없이 빈 데이터를 돌려준다.
 */
function useNoopTagFilterState(): StoreTagFilterState {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const toggle = useCallback((filterId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(filterId)) next.delete(filterId);
      else next.add(filterId);
      return next;
    });
  }, []);
  const clearAll = useCallback(() => setSelected(new Set()), []);
  return { pairs: [], tagsByKey: {}, selected, toggle, clearAll };
}

function SeriesDataViewerModalImpl({
  isOpen,
  onClose,
  initialSeriesKey,
  allSeriesKeys,
  dataSource,
  agentName,
}: SeriesDataViewerModalProps) {
  // --- 폼 상태 ---
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [keySearch, setKeySearch] = useState('');
  const [startLocal, setStartLocal] = useState('');
  const [endLocal, setEndLocal] = useState('');
  const [intervalSelect, setIntervalSelect] = useState<IntervalValue>('1m');
  const [customInterval, setCustomInterval] = useState('');
  const [aggregation, setAggregation] = useState<TsdbAggregation>('average');

  // v0.3.0 Wave 2: 절대/상대 모드 탭 + 상대 드롭다운 상태.
  //   - `rangeMode`: 'absolute' (기본) | 'relative'.
  //   - `relativeSelect`: 상대 모드 드롭다운 선택(프리셋 또는 'custom').
  //   - `relativeCustom`: 커스텀 duration 입력 문자열 (예: '2h', '45m').
  // 모드 전환은 모달 세션 내에서 유지되며, 모달 오픈 시마다 'absolute' 로 리셋된다.
  const [rangeMode, setRangeMode] = useState<RangeMode>('absolute');
  const [relativeSelect, setRelativeSelect] = useState<RelativeSelectValue>(RELATIVE_DEFAULT);
  const [relativeCustom, setRelativeCustom] = useState('');

  // 5,000행 경고 확인 상태: pending 은 "경고 표시됨, 사용자 확정 대기 중".
  const [warningPending, setWarningPending] = useState(false);

  // 매트릭스 쿼리 mutation — dataSource.queryMatrix 를 호출한다.
  const mutation = useMutation<SeriesMatrix, Error, SeriesMatrixQuery>({
    mutationFn: (params) => dataSource.queryMatrix(params),
  });

  // --- 태그 필터 (Store 전용, SPEC-STORE-003) ---
  // TSDB 데이터 소스에서는 훅이 no-op 로 동작한다.
  const tagFilter = useStoreTagFilterState(dataSource.kind, agentName);

  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusRef = useRef<HTMLInputElement>(null);

  // --- 모달 오픈 시 상태 초기화 ---
  // 시간 범위는 매번 "지난 1일" 로 리셋한다 (사용자 입력은 오픈 중에만 보존).
  // 모드는 매번 'absolute' 로 리셋되어 기존 기본 동작을 보존한다.
  useEffect(() => {
    if (!isOpen) return;
    setSelectedKeys(initialSeriesKey ? [initialSeriesKey] : []);
    setKeySearch('');
    const nowMs = Date.now();
    setEndLocal(epochMsToDatetimeLocal(nowMs));
    setStartLocal(epochMsToDatetimeLocal(nowMs - ONE_DAY_MS));
    setIntervalSelect('1m');
    setCustomInterval('');
    setAggregation('average');
    setRangeMode('absolute');
    setRelativeSelect(RELATIVE_DEFAULT);
    setRelativeCustom('');
    setWarningPending(false);
    mutation.reset();
    // mutation 은 ref-stable 해야 하지만 완벽히 안전하진 않으므로 exhaustive-deps 무시.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, initialSeriesKey]);

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
    selectedKeys.length > 0 &&
    timeRangeValid &&
    intervalValid &&
    !mutation.isPending;

  /**
   * 검색어 + (Store 전용) 태그 필터로 필터링된 시리즈 키 옵션.
   *
   * 태그 필터는 AND 로직이며, 정적 키가 아닌 키(tagsByKey 에 없음) 는
   * 태그 필터 활성 시 모두 제외된다.
   *
   * @spec SPEC-STORE-003
   */
  const filteredKeys = useMemo(() => {
    const q = keySearch.trim().toLowerCase();
    const tagActive = tagFilter.selected.size > 0;
    return allSeriesKeys.filter((k) => {
      if (q && !k.toLowerCase().includes(q)) return false;
      if (tagActive) {
        const tagsForKey = tagFilter.tagsByKey[k];
        if (!matchesTagFilter(tagsForKey, tagFilter.selected)) return false;
      }
      return true;
    });
  }, [allSeriesKeys, keySearch, tagFilter.selected, tagFilter.tagsByKey]);

  // --- 핸들러 ---

  const toggleKey = useCallback((key: string) => {
    setSelectedKeys((prev) => {
      if (prev.includes(key)) return prev.filter((k) => k !== key);
      return [...prev, key];
    });
  }, []);

  const removeKey = useCallback((key: string) => {
    setSelectedKeys((prev) => prev.filter((k) => k !== key));
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
    const orderedKeys = [...selectedKeys];
    // Go duration 을 milliseconds 로 역환산. 유효성은 위에서 이미 확인됨.
    const intervalMs = parseIntervalToMs(effectiveInterval);
    // 상대 모드는 실행 시점의 now 로 재계산된다.
    const { startMs: resolvedStart, endMs: resolvedEnd } = resolveQueryRange();
    if (!Number.isFinite(resolvedStart) || !Number.isFinite(resolvedEnd)) return;
    setLastQueryRange({ startMs: resolvedStart, endMs: resolvedEnd });
    mutation.mutate({
      keys: orderedKeys,
      startMs: resolvedStart,
      endMs: resolvedEnd,
      intervalMs,
      aggregation,
    });
  }, [
    canExecute,
    selectedKeys,
    resolveQueryRange,
    effectiveInterval,
    aggregation,
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
    return '쿼리 실행에 실패했습니다';
  }, [mutation.isError, mutation.error]);

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
              TSDB 데이터 뷰어
            </h2>
            {mutation.isPending && (
              <Loader2
                className="h-4 w-4 animate-spin text-blue-500"
                aria-label="쿼리 실행 중"
              />
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 폼 영역 — 자연 높이, 고정 */}
        <div className="flex-shrink-0 space-y-4 border-b border-(--color-border-default) px-6 py-4">
          {/* 시리즈 멀티셀렉트 */}
          <fieldset>
            <legend className="mb-2 block text-sm font-medium text-(--color-text-secondary)">
              시리즈 선택 ({selectedKeys.length}개 선택됨)
            </legend>
            {/* 선택된 키 pill */}
            {selectedKeys.length > 0 && (
              <div className="mb-2 flex flex-wrap gap-1.5">
                {selectedKeys.map((k) => (
                  <span
                    key={k}
                    className="inline-flex items-center gap-1 rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900 dark:text-blue-300"
                  >
                    <span className="font-mono">{k}</span>
                    <button
                      type="button"
                      onClick={() => removeKey(k)}
                      aria-label={`${k} 제거`}
                      className="rounded-full hover:bg-blue-200 dark:hover:bg-blue-800"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            {/*
              태그 필터 (SPEC-STORE-003): Store 에이전트가 태그 쌍을 제공할 때만 노출.
              태그 쌍이 비어 있으면 (TSDB 또는 정적 키 없는 Store) 섹션 전체를 숨긴다.
            */}
            {tagFilter.pairs.length > 0 && (
              <div className="mb-2 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2.5">
                <TagFilterChips
                  pairs={tagFilter.pairs}
                  selected={tagFilter.selected}
                  onToggle={tagFilter.toggle}
                  onClearAll={tagFilter.clearAll}
                />
              </div>
            )}

            {/* 검색 입력 */}
            <div className="relative">
              <Search
                className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-(--color-text-muted)"
                aria-hidden="true"
              />
              <input
                ref={firstFocusRef}
                type="text"
                placeholder="시리즈 키 검색"
                value={keySearch}
                onChange={(e) => setKeySearch(e.target.value)}
                className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) pl-7 pr-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>
            {/*
              체크박스 옵션 리스트.
              v0.3.0: 선택된 행에 배경 하이라이트를 적용하지 않는다 — 체크 표시만으로
              선택 상태를 표현하여 다중 선택 시 시각 노이즈를 줄인다.
              hover 배경은 그대로 유지.
            */}
            {/* 시리즈 multi-select 컨테이너.
                모달이 95vw × 95vh 로 확장되었으므로 (v0.3.0) max-h-40 에서
                max-h-[40vh] 로 확장하여 필터 결과 다수가 한눈에 보이도록 한다.
                여전히 폼 영역이 매트릭스 영역을 침범하지 않도록 vh 기반으로 제한. */}
            <div className="mt-2 max-h-[40vh] overflow-y-auto rounded-md border border-(--color-border-default) bg-(--color-bg-primary)">
              {filteredKeys.length === 0 ? (
                <p className="p-3 text-xs text-(--color-text-muted)">
                  일치하는 시리즈가 없습니다.
                </p>
              ) : (
                <ul>
                  {filteredKeys.map((k) => {
                    const checked = selectedKeys.includes(k);
                    return (
                      <li key={k}>
                        <label className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs hover:bg-(--color-bg-elevated)">
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleKey(k)}
                            className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
                          />
                          <span className="font-mono text-(--color-text-primary)">{k}</span>
                        </label>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
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
              aria-label="시간 범위 모드"
            >
              {(['absolute', 'relative'] as const).map((mode) => {
                const label = mode === 'absolute' ? '절대' : '상대';
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

            {rangeMode === 'absolute' ? (
              <>
                {/* 절대 모드: 빠른 범위 버튼 + datetime-local 입력 */}
                <div
                  className="flex flex-wrap items-center gap-1.5"
                  role="group"
                  aria-label="상대 범위 빠른 선택"
                >
                  <span className="mr-1 text-xs text-(--color-text-muted)">빠른 선택:</span>
                  {RELATIVE_RANGE_PRESETS.map((preset) => (
                    <button
                      key={preset.label}
                      type="button"
                      onClick={() => handleRelativeRange(preset.durationMs)}
                      className="rounded-full border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-0.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
                    >
                      {preset.label}
                    </button>
                  ))}
                </div>

                {/* 시간 범위 */}
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <div>
                    <label
                      htmlFor="tsdb-start"
                      className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                    >
                      시작 시각 (Local)
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
                      종료 시각 (Local)
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
                {/* 시간 범위 에러 안내 */}
                {startLocal && endLocal && !timeRangeValid && (
                  <p className="text-xs text-red-600 dark:text-red-400">
                    종료 시각은 시작 시각 이후여야 합니다.
                  </p>
                )}
              </>
            ) : (
              <>
                {/* 상대 모드: 프리셋 드롭다운 + 커스텀 duration 입력 */}
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <div>
                    <label
                      htmlFor="tsdb-relative-range"
                      className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                    >
                      범위
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
                          {p.label}
                        </option>
                      ))}
                      <option value="custom">커스텀</option>
                    </select>
                  </div>
                  {relativeSelect === 'custom' && (
                    <div>
                      <label
                        htmlFor="tsdb-relative-custom"
                        className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
                      >
                        커스텀 duration
                      </label>
                      <input
                        id="tsdb-relative-custom"
                        type="text"
                        placeholder="예: 2h, 45m, 30s"
                        value={relativeCustom}
                        onChange={(e) => setRelativeCustom(e.target.value)}
                        className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 font-mono text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      />
                      {!relativeCustomValid && relativeCustom.trim() !== '' && (
                        <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                          Go duration 문법 (ms/s/m/h) 을 사용하세요.
                        </p>
                      )}
                    </div>
                  )}
                </div>
                <p className="text-xs text-(--color-text-muted)">
                  실행 시각 기준 지난 기간을 조회합니다 (실행 시점에 현재 시각이 사용됩니다).
                </p>
              </>
            )}
          </div>

          {/* 인터벌 + 집계 */}
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div>
              <label
                htmlFor="tsdb-interval"
                className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
              >
                인터벌
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
                <option value="custom">사용자 지정</option>
              </select>
              {intervalSelect === 'custom' && (
                <div className="mt-1.5">
                  <input
                    type="text"
                    placeholder="예: 2m, 45s, 100ms"
                    value={customInterval}
                    onChange={(e) => setCustomInterval(e.target.value)}
                    className="block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 font-mono text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  />
                  {!intervalValid && customInterval.trim() !== '' && (
                    <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                      Go duration 문법 (ms/s/m/h) 을 사용하세요.
                    </p>
                  )}
                </div>
              )}
            </div>
            <fieldset>
              <legend className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                집계 함수
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
                    {opt.label}
                  </label>
                ))}
              </div>
            </fieldset>
          </div>

          {/* 경고 배너 (5,000행 초과) */}
          {warningPending && (
            <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-700 dark:bg-amber-950">
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
                <div className="flex-1">
                  <p className="font-medium text-amber-800 dark:text-amber-200">
                    결과 행 수가 많아 렌더링이 느릴 수 있습니다
                  </p>
                  <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                    예상 행 수: {expectedBuckets.toLocaleString()}행 (임계치 {MATRIX_ROW_WARNING_THRESHOLD.toLocaleString()} 초과)
                  </p>
                  <div className="mt-2 flex items-center gap-2">
                    <button
                      type="button"
                      onClick={handleConfirmWarning}
                      className="rounded-md bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-700"
                    >
                      계속 실행
                    </button>
                    <button
                      type="button"
                      onClick={handleCancelWarning}
                      className="rounded-md border border-amber-400 px-3 py-1 text-xs font-medium text-amber-700 hover:bg-amber-100 dark:border-amber-600 dark:text-amber-300 dark:hover:bg-amber-900"
                    >
                      취소
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
        */}
        <div
          className="flex-1 overflow-auto px-6 py-4"
          data-testid="tsdb-viewer-result-scroll"
        >
          {mutation.isSuccess && mutation.data && mutation.data.columns.length > 0 ? (
            <section aria-label="쿼리 결과">
              <h3 className="mb-2 text-sm font-semibold text-(--color-text-primary)">
                결과 매트릭스
              </h3>
              <SeriesResultMatrix
                matrix={mutation.data}
                agentName={agentName}
                exportStartMs={lastQueryRange?.startMs}
                exportEndMs={lastQueryRange?.endMs}
              />
            </section>
          ) : (
            <p className="text-center text-xs text-(--color-text-muted)">
              조건을 설정하고 실행하면 결과가 여기에 표시됩니다.
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
            닫기
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
                실행 중…
              </>
            ) : mutation.isSuccess ? (
              <>
                <Check className="h-4 w-4" />
                다시 실행
              </>
            ) : (
              <>
                <Play className="h-4 w-4" />
                실행
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
