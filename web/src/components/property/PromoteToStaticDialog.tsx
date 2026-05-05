// 동적 키를 정적 키로 변환하기 위한 모달 컴포넌트.
//
// Store 에이전트의 storage 목록에서 "동적" 키(설정의 keys 배열에 등록되지 않은 키)
// 를 사용자가 선택하여 정적 키로 승격(promote)할 때 사용한다.
//
// v0.7.0 (M14) Phase D 진화:
//   - data_type 셀렉트 (필수, 6종 enum: int|float|string|boolean|bytes|json).
//     변환 시점에 사용자가 명시적으로 직렬화 형식을 선언하므로 manual 모드와
//     동일하게 필수로 강제한다 (미선택 시 변환 버튼 비활성).
//   - metric_type 입력 (선택, 정규식 `^[a-zA-Z0-9_-]+$`).
//     빈 값이면 백엔드가 default `"unknown"` 적용.
//   - onConfirm 시그니처 진화: `(tags) => void` → `({data_type, metric_type, tags}) => void`.
//   - 부모로부터 `defaultDataType` 을 전달받으면 셀렉트에 사전 채움한다 (백엔드가
//     이미 키에 대해 추론한 타입이 있을 경우 — 이벤트 기반 워크플로우).
//
// 백엔드 변경 없이 PUT /api/v1/agents/{id}/config 엔드포인트를 통해 config.keys
// 배열에 새 엔트리를 추가하는 방식으로 동작한다 (Phase A/B 와 동일).
//
// @spec SPEC-STORE-003 v0.3.0
// @spec SPEC-WEB-005 v0.7.0 (M14)

import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from 'react';
import { Loader2, Plus, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  DATA_TYPE_OPTIONS,
  validateDataType,
  validateMetricType,
} from './storeKeysValidation';
import type { DataType } from '@/services/api/store';

// ---- Props ----

/**
 * 변환 확인 시 부모에 전달되는 페이로드.
 *
 * - data_type: 필수 (사용자가 셀렉트에서 선택한 값).
 * - metric_type: 선택. 빈 문자열은 부모/백엔드에서 default `"unknown"` 적용으로 처리된다.
 * - tags: 사용자가 입력한 태그 맵. 비어있으면 빈 객체 `{}`.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M14)
 */
export interface PromoteToStaticPayload {
  data_type: DataType;
  metric_type?: string;
  tags: Record<string, string>;
}

interface PromoteToStaticDialogProps {
  /** 모달 표시 여부 */
  isOpen: boolean;
  /** 모달 닫기 핸들러 (취소 또는 외부 클릭 시) */
  onClose: () => void;
  /** 승격할 키 이름 (읽기 전용 표시). */
  keyName: string;
  /**
   * 변환 확인 핸들러.
   * data_type / metric_type / tags 페이로드를 받아 부모에서
   * PUT /agents/{id}/config 호출을 처리한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M14)
   */
  onConfirm: (payload: PromoteToStaticPayload) => void | Promise<void>;
  /** 부모의 mutation 진행 중 상태. true 이면 변환 버튼 비활성 + 스피너 표시. */
  isSubmitting?: boolean;
  /**
   * 사전 채움할 data_type. 백엔드가 auto 등록 시점에 이미 키의 직렬화 타입을
   * 추론한 경우, 부모가 그 값을 전달하여 사용자 경험을 개선한다 (이벤트 기반).
   *
   * 미전달 시 셀렉트는 unset 상태로 시작하며 사용자가 명시적으로 선택해야 한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M14)
   */
  defaultDataType?: DataType;
}

// ---- 내부 상태 타입 ----

/** 태그 입력 행. React 리스트 안정성을 위해 id 를 부여한다. */
interface TagRow {
  id: string;
  k: string;
  v: string;
}

// ---- 검증 ----

/** 태그 키 허용 문자: 영문/숫자/언더스코어/하이픈. (StoreKeysEditor 와 동일 규칙) */
const TAG_KEY_REGEX = /^[a-zA-Z0-9_-]+$/;

function isValidTagKey(k: string): boolean {
  return TAG_KEY_REGEX.test(k);
}

/**
 * 태그 행 검증.
 * - 키와 값이 모두 비어있는 행은 "빈 행"으로 무시 (변환 시 제외).
 * - 키만 입력되었거나 키가 잘못된 형식이면 "유효하지 않음".
 * - 키는 유효한데 값이 비어있으면 "유효하지 않음".
 */
function isRowValid(row: TagRow): boolean {
  const k = row.k.trim();
  const v = row.v.trim();
  // 둘 다 비어있으면 빈 행 (정상)
  if (k === '' && v === '') return true;
  // 키만 형식이 잘못된 경우
  if (k !== '' && !isValidTagKey(k)) return false;
  // 키 형식은 맞지만 값이 비어있는 경우
  if (k !== '' && v === '') return false;
  // 키는 비었는데 값만 있는 경우
  if (k === '' && v !== '') return false;
  return true;
}

let tagRowCounter = 0;
function nextTagRowId(): string {
  return `tag-row-${++tagRowCounter}`;
}

// ---- 스타일 ----

const inputCls = cn(
  'rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

// 에러 상태 input 스타일 — amber 톤으로 인라인 경고 표시.
const errorInputCls =
  'border-amber-400 focus:border-amber-500 focus:ring-amber-400';

// ---- 컴포넌트 ----

export function PromoteToStaticDialog({
  isOpen,
  onClose,
  keyName,
  onConfirm,
  isSubmitting = false,
  defaultDataType,
}: PromoteToStaticDialogProps) {
  // 태그 행 상태. 모달이 닫힐 때 초기화한다.
  const [rows, setRows] = useState<TagRow[]>([]);
  // data_type 상태 (빈 문자열 = unset). 변환 시 manual 모드 검증을 적용한다.
  const [dataType, setDataType] = useState<string>('');
  // metric_type 상태. 빈 문자열은 백엔드 default `"unknown"` 으로 매핑된다.
  const [metricType, setMetricType] = useState<string>('');

  // 모달 열림/닫힘에 따른 상태 리셋.
  // defaultDataType 이 전달되면 셀렉트에 사전 채움한다.
  useEffect(() => {
    if (isOpen) {
      setRows([]);
      setDataType(defaultDataType ?? '');
      setMetricType('');
    }
  }, [isOpen, defaultDataType]);

  // Esc 키로 닫기 (제출 중에는 무시).
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape' && !isSubmitting) onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, isSubmitting]);

  // 배경 클릭 시 닫기 (제출 중에는 무시).
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !isSubmitting) onClose();
    },
    [onClose, isSubmitting],
  );

  // 태그 행 추가.
  const handleAddRow = useCallback(() => {
    setRows((prev) => [...prev, { id: nextTagRowId(), k: '', v: '' }]);
  }, []);

  // 태그 행 삭제.
  const handleRemoveRow = useCallback((id: string) => {
    setRows((prev) => prev.filter((r) => r.id !== id));
  }, []);

  // 태그 행 키/값 갱신.
  const handleRowChange = useCallback(
    (id: string, field: 'k' | 'v', value: string) => {
      setRows((prev) =>
        prev.map((r) => (r.id === id ? { ...r, [field]: value } : r)),
      );
    },
    [],
  );

  // 모든 행이 유효한지 (빈 행은 허용, 부분 입력은 disable).
  const allRowsValid = useMemo(() => rows.every(isRowValid), [rows]);

  // data_type 검증 — 변환은 manual 모드 의미론(필수)을 따른다.
  // 사용자가 명시적으로 정적 키로 등록하는 행위이므로 data_type 미선택은 막는다.
  const dataTypeValidation = useMemo(
    () => validateDataType(dataType, 'manual'),
    [dataType],
  );

  // metric_type 검증 — 빈 값 허용, 정규식 위반 시 에러.
  const metricTypeValidation = useMemo(
    () => validateMetricType(metricType),
    [metricType],
  );

  // 변환 버튼 활성 조건: 모든 검증 통과 + 제출 중이 아님.
  const canConfirm =
    allRowsValid &&
    dataTypeValidation.valid &&
    metricTypeValidation.valid &&
    !isSubmitting;

  // 비활성 시 노출할 사유 (tooltip / aria-describedby 용도).
  const disabledReason = useMemo(() => {
    if (!dataTypeValidation.valid) return dataTypeValidation.error ?? 'data_type 을 선택해주세요';
    if (!metricTypeValidation.valid) return metricTypeValidation.error ?? 'metric_type 형식을 확인해주세요';
    if (!allRowsValid) return '태그 입력을 확인해주세요';
    return undefined;
  }, [dataTypeValidation, metricTypeValidation, allRowsValid]);

  // 변환 실행 — 비어있지 않은 행만 모아 태그 맵을 구성하고 부모에 전달.
  const handleConfirm = useCallback(async () => {
    if (!canConfirm) return;
    const tags: Record<string, string> = {};
    for (const row of rows) {
      const k = row.k.trim();
      const v = row.v.trim();
      if (k === '' && v === '') continue;
      tags[k] = v;
    }
    const trimmedMetric = metricType.trim();
    const payload: PromoteToStaticPayload = {
      // canConfirm 가드로 dataType 은 비어있지 않음이 보장된다.
      data_type: dataType as DataType,
      tags,
    };
    if (trimmedMetric !== '') {
      payload.metric_type = trimmedMetric;
    }
    await onConfirm(payload);
  }, [canConfirm, rows, dataType, metricType, onConfirm]);

  // Enter 키로 변환 실행 (입력 필드에서).
  const handleInputKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        void handleConfirm();
      }
    },
    [handleConfirm],
  );

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="promote-to-static-title"
    >
      <div
        className="mx-4 flex w-full max-w-[480px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="promote-to-static-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            정적 키로 변환
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 px-5 py-4">
          {/* 키 표시 (읽기 전용) */}
          <div>
            <p className="mb-1 text-xs font-medium text-(--color-text-muted)">키</p>
            <p className="break-all rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 font-mono text-sm text-(--color-text-primary)">
              {keyName}
            </p>
          </div>

          {/* data_type 셀렉트 (필수) */}
          <div>
            <label
              htmlFor="promote-data-type"
              className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
            >
              데이터 타입 <span className="text-red-500">*</span>
            </label>
            <p className="mb-1.5 text-[11px] text-(--color-text-muted)">
              값의 직렬화 형식을 선택하세요. 등록 후에는 다른 타입으로 쓰기를 시도하면 거절됩니다.
            </p>
            <select
              id="promote-data-type"
              value={dataType}
              onChange={(e) => setDataType(e.target.value)}
              disabled={isSubmitting}
              aria-invalid={!dataTypeValidation.valid || undefined}
              aria-describedby={
                !dataTypeValidation.valid ? 'promote-data-type-error' : undefined
              }
              className={cn(
                inputCls,
                'w-full',
                !dataTypeValidation.valid && errorInputCls,
                isSubmitting && 'cursor-not-allowed opacity-60',
              )}
            >
              <option value="" disabled>
                선택하세요
              </option>
              {DATA_TYPE_OPTIONS.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}
                </option>
              ))}
            </select>
            {!dataTypeValidation.valid && (
              <p
                id="promote-data-type-error"
                className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400"
              >
                {dataTypeValidation.error}
              </p>
            )}
          </div>

          {/* metric_type 입력 (선택) */}
          <div>
            <label
              htmlFor="promote-metric-type"
              className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
            >
              메트릭 타입 (선택)
            </label>
            <p className="mb-1.5 text-[11px] text-(--color-text-muted)">
              지표 분류용 라벨 (예: gauge, counter). 비워두면 백엔드가 "unknown" 으로 처리합니다.
            </p>
            <input
              id="promote-metric-type"
              type="text"
              value={metricType}
              onChange={(e) => setMetricType(e.target.value)}
              onKeyDown={handleInputKeyDown}
              disabled={isSubmitting}
              placeholder="unknown"
              aria-invalid={!metricTypeValidation.valid || undefined}
              aria-describedby={
                !metricTypeValidation.valid ? 'promote-metric-type-error' : undefined
              }
              className={cn(
                inputCls,
                'w-full',
                !metricTypeValidation.valid && errorInputCls,
                isSubmitting && 'cursor-not-allowed opacity-60',
              )}
            />
            {!metricTypeValidation.valid && (
              <p
                id="promote-metric-type-error"
                className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400"
              >
                {metricTypeValidation.error}
              </p>
            )}
          </div>

          {/* 태그 편집 */}
          <div>
            <p className="mb-1 text-xs font-medium text-(--color-text-secondary)">
              태그 (선택)
            </p>
            <p className="mb-2 text-[11px] text-(--color-text-muted)">
              태그를 추가하면 필터링과 그룹화에 사용할 수 있습니다. 태그 없이 변환할 수도 있습니다.
            </p>

            {rows.length === 0 ? (
              <p className="rounded-md border border-dashed border-(--color-border-default) px-3 py-3 text-center text-xs text-(--color-text-muted)">
                태그가 없습니다
              </p>
            ) : (
              <div className="space-y-1.5">
                {rows.map((row) => {
                  const k = row.k.trim();
                  const v = row.v.trim();
                  const keyInvalid = k !== '' && !isValidTagKey(k);
                  const valueInvalid = k !== '' && v === '';
                  const orphanValue = k === '' && v !== '';

                  return (
                    <div key={row.id}>
                      <div className="flex items-center gap-1.5">
                        <input
                          type="text"
                          value={row.k}
                          onChange={(e) => handleRowChange(row.id, 'k', e.target.value)}
                          onKeyDown={handleInputKeyDown}
                          placeholder="태그 키 (예: room)"
                          disabled={isSubmitting}
                          aria-label="태그 키"
                          aria-invalid={keyInvalid || orphanValue || undefined}
                          className={cn(
                            inputCls,
                            'flex-1',
                            (keyInvalid || orphanValue) && errorInputCls,
                            isSubmitting && 'cursor-not-allowed opacity-60',
                          )}
                        />
                        <span className="text-(--color-text-muted)">=</span>
                        <input
                          type="text"
                          value={row.v}
                          onChange={(e) => handleRowChange(row.id, 'v', e.target.value)}
                          onKeyDown={handleInputKeyDown}
                          placeholder="태그 값"
                          disabled={isSubmitting}
                          aria-label="태그 값"
                          aria-invalid={valueInvalid || undefined}
                          className={cn(
                            inputCls,
                            'flex-1',
                            valueInvalid && errorInputCls,
                            isSubmitting && 'cursor-not-allowed opacity-60',
                          )}
                        />
                        <button
                          type="button"
                          onClick={() => handleRemoveRow(row.id)}
                          disabled={isSubmitting}
                          className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                          aria-label="태그 삭제"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                      {keyInvalid && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          태그 키는 영문/숫자/언더스코어/하이픈만 허용됩니다
                        </p>
                      )}
                      {valueInvalid && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          태그 값을 입력하세요
                        </p>
                      )}
                      {orphanValue && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          태그 키를 입력하세요
                        </p>
                      )}
                    </div>
                  );
                })}
              </div>
            )}

            <button
              type="button"
              onClick={handleAddRow}
              disabled={isSubmitting}
              className="mt-2 inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:border-blue-500 dark:hover:text-blue-400"
            >
              <Plus className="h-3.5 w-3.5" />
              태그 추가
            </button>
          </div>
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            취소
          </button>
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={!canConfirm}
            title={disabledReason}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {isSubmitting ? '변환 중...' : '변환'}
          </button>
        </div>
      </div>
    </div>
  );
}
