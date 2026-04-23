// 시리즈 데이터 뷰어 모달.
// 시리즈 멀티셀렉트, 시간 범위, 인터벌, 집계 함수 입력을 받아 매트릭스 쿼리를 실행한다.
// `ImportDialog` 의 포털/배경 클릭/Esc 닫기 패턴을 재사용한다.
//
// SPEC-WEB-005 v0.2.0 에서 `dataSource: SeriesDataSource` prop 을 받아
// TSDB/Store 양쪽 모두에 동작하도록 리팩터되었다.
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

import { cn } from '@/lib/utils/cn';
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

import SeriesResultMatrix from './TsdbResultMatrix';

/** 5,000행 초과 시 경고 임계치. */
const MATRIX_ROW_WARNING_THRESHOLD = 5000;

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

interface SeriesDataViewerModalProps {
  isOpen: boolean;
  onClose: () => void;
  /** 모달 오픈 시 기본 선택할 시리즈 키. */
  initialSeriesKey?: string;
  /** 멀티셀렉트 옵션 풀. */
  allSeriesKeys: string[];
  /** TSDB/Store 공용 데이터 소스. */
  dataSource: SeriesDataSource;
}

function SeriesDataViewerModalImpl({
  isOpen,
  onClose,
  initialSeriesKey,
  allSeriesKeys,
  dataSource,
}: SeriesDataViewerModalProps) {
  // --- 폼 상태 ---
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [keySearch, setKeySearch] = useState('');
  const [startLocal, setStartLocal] = useState('');
  const [endLocal, setEndLocal] = useState('');
  const [intervalSelect, setIntervalSelect] = useState<IntervalValue>('1m');
  const [customInterval, setCustomInterval] = useState('');
  const [aggregation, setAggregation] = useState<TsdbAggregation>('average');

  // 5,000행 경고 확인 상태: pending 은 "경고 표시됨, 사용자 확정 대기 중".
  const [warningPending, setWarningPending] = useState(false);

  // 매트릭스 쿼리 mutation — dataSource.queryMatrix 를 호출한다.
  const mutation = useMutation<SeriesMatrix, Error, SeriesMatrixQuery>({
    mutationFn: (params) => dataSource.queryMatrix(params),
  });

  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusRef = useRef<HTMLInputElement>(null);

  // --- 모달 오픈 시 상태 초기화 ---
  useEffect(() => {
    if (!isOpen) return;
    setSelectedKeys(initialSeriesKey ? [initialSeriesKey] : []);
    setKeySearch('');
    setStartLocal('');
    setEndLocal('');
    setIntervalSelect('1m');
    setCustomInterval('');
    setAggregation('average');
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

  const startMs = useMemo(() => datetimeLocalToEpochMs(startLocal), [startLocal]);
  const endMs = useMemo(() => datetimeLocalToEpochMs(endLocal), [endLocal]);

  const intervalValid = useMemo(() => {
    if (intervalSelect !== 'custom') return true;
    return isValidInterval(customInterval.trim());
  }, [intervalSelect, customInterval]);

  const timeRangeValid = useMemo(() => {
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return false;
    return endMs > startMs;
  }, [startMs, endMs]);

  const expectedBuckets = useMemo(() => {
    if (!timeRangeValid || !intervalValid) return 0;
    return estimateBucketCount(startMs, endMs, effectiveInterval);
  }, [startMs, endMs, effectiveInterval, timeRangeValid, intervalValid]);

  const canExecute =
    selectedKeys.length > 0 &&
    timeRangeValid &&
    intervalValid &&
    !mutation.isPending;

  /** 검색어로 필터링된 시리즈 키 옵션. */
  const filteredKeys = useMemo(() => {
    const q = keySearch.trim().toLowerCase();
    if (!q) return allSeriesKeys;
    return allSeriesKeys.filter((k) => k.toLowerCase().includes(q));
  }, [allSeriesKeys, keySearch]);

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

  const performQuery = useCallback(() => {
    if (!canExecute) return;
    const orderedKeys = [...selectedKeys];
    // Go duration 을 milliseconds 로 역환산. 유효성은 위에서 이미 확인됨.
    const intervalMs = parseIntervalToMs(effectiveInterval);
    mutation.mutate({
      keys: orderedKeys,
      startMs,
      endMs,
      intervalMs,
      aggregation,
    });
  }, [
    canExecute,
    selectedKeys,
    startMs,
    endMs,
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
      <div
        ref={modalRef}
        className="mx-4 flex w-full max-w-4xl max-h-[90vh] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-6 py-4">
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

        {/* 본문 */}
        <div className="flex-1 space-y-4 overflow-y-auto px-6 py-4">
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
            {/* 체크박스 옵션 리스트 */}
            <div className="mt-2 max-h-40 overflow-y-auto rounded-md border border-(--color-border-default) bg-(--color-bg-primary)">
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
                        <label
                          className={cn(
                            'flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs hover:bg-(--color-bg-elevated)',
                            checked && 'bg-blue-50 dark:bg-blue-950/30',
                          )}
                        >
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

          {/* 결과 매트릭스 */}
          {mutation.isSuccess && mutation.data && mutation.data.columns.length > 0 && (
            <section aria-label="쿼리 결과">
              <h3 className="mb-2 text-sm font-semibold text-(--color-text-primary)">
                결과 매트릭스
              </h3>
              <SeriesResultMatrix matrix={mutation.data} />
            </section>
          )}
        </div>

        {/* 푸터 */}
        <div className="flex items-center justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
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
