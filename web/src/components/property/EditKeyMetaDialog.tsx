// 임의 엔트리(정적 + 동적)의 필드(field) 과 태그(tags) 를 편집하는 모달.
//
// Store 에이전트 저장소 탭의 각 행에서 호출되며, `PUT /store/{name}/keys/{key}/meta`
// (useSetStoreKeyMeta) 로 적용한다.
//
// 동작 규칙:
//   - tags 는 **전체 교체** 이므로 기존 태그를 행으로 미리 채워 편집 후 전체를 전송한다.
//   - field 은 선택 (정규식 `^[a-zA-Z0-9_-]+$`). 비워두면 백엔드가 "unknown" 적용.
//   - data_type 은 이 엔드포인트로 변경할 수 없으므로 노출하지 않는다 (정적 키는 보존,
//     동적 키는 string/auto 보존).
//
// 검증/태그 입력 패턴은 PromoteToStaticDialog 와 일관되게 유지한다.
//
// @spec SPEC-STORE-003 v0.4.0

import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from 'react';
import { Loader2, Plus, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { validateMetricType } from './storeKeysValidation';

// ---- Props ----

/**
 * 편집 확인 시 부모에 전달되는 페이로드.
 *
 * - field: 선택. 빈 문자열은 부모/백엔드에서 default `"unknown"` 적용으로 처리된다.
 * - tags: 전체 교체할 태그 맵. 비어있으면 빈 객체 `{}`.
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export interface EditKeyMetaPayload {
  field?: string;
  tags: Record<string, string>;
}

interface EditKeyMetaDialogProps {
  /** 모달 표시 여부 */
  isOpen: boolean;
  /** 모달 닫기 핸들러 (취소 또는 외부 클릭 시) */
  onClose: () => void;
  /** 편집 대상 키 이름 (읽기 전용 표시). */
  keyName: string;
  /** 현재 field (사전 채움). 빈 문자열이면 unset 상태. */
  initialField?: string;
  /** 현재 태그 맵 (사전 채움). tags 전체 교체를 위해 기존 값을 모두 불러온다. */
  initialTags?: Record<string, string>;
  /**
   * 편집 확인 핸들러. field / tags 페이로드를 받아 부모에서
   * PUT .../keys/{key}/meta 호출을 처리한다.
   */
  onConfirm: (payload: EditKeyMetaPayload) => void | Promise<void>;
  /** 부모의 mutation 진행 중 상태. true 이면 저장 버튼 비활성 + 스피너 표시. */
  isSubmitting?: boolean;
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
 * - 키와 값이 모두 비어있는 행은 "빈 행"으로 무시 (저장 시 제외).
 * - 키만 입력되었거나 키가 잘못된 형식이면 "유효하지 않음".
 * - 키는 유효한데 값이 비어있으면 "유효하지 않음".
 * - 키는 비었는데 값만 있으면 "유효하지 않음".
 */
function isRowValid(row: TagRow): boolean {
  const k = row.k.trim();
  const v = row.v.trim();
  if (k === '' && v === '') return true;
  if (k !== '' && !isValidTagKey(k)) return false;
  if (k !== '' && v === '') return false;
  if (k === '' && v !== '') return false;
  return true;
}

let tagRowCounter = 0;
function nextTagRowId(): string {
  return `edit-tag-row-${++tagRowCounter}`;
}

/** 초기 태그 맵을 편집 행으로 변환한다. */
function tagsToRows(tags: Record<string, string> | undefined): TagRow[] {
  if (!tags) return [];
  return Object.entries(tags).map(([k, v]) => ({
    id: nextTagRowId(),
    k,
    v: typeof v === 'string' ? v : String(v ?? ''),
  }));
}

// ---- 스타일 ----

const inputCls = cn(
  'rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

const errorInputCls =
  'border-amber-400 focus:border-amber-500 focus:ring-amber-400';

// ---- 컴포넌트 ----

export function EditKeyMetaDialog({
  isOpen,
  onClose,
  keyName,
  initialField,
  initialTags,
  onConfirm,
  isSubmitting = false,
}: EditKeyMetaDialogProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<TagRow[]>([]);
  const [fieldName, setField] = useState<string>('');

  // 모달이 열릴 때 현재 field / tags 로 상태를 초기화 (사전 채움).
  useEffect(() => {
    if (isOpen) {
      setField(initialField ?? '');
      setRows(tagsToRows(initialTags));
    }
  }, [isOpen, initialField, initialTags]);

  // Esc 키로 닫기 (제출 중에는 무시).
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape' && !isSubmitting) onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, isSubmitting]);

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !isSubmitting) onClose();
    },
    [onClose, isSubmitting],
  );

  const handleAddRow = useCallback(() => {
    setRows((prev) => [...prev, { id: nextTagRowId(), k: '', v: '' }]);
  }, []);

  const handleRemoveRow = useCallback((id: string) => {
    setRows((prev) => prev.filter((r) => r.id !== id));
  }, []);

  const handleRowChange = useCallback(
    (id: string, field: 'k' | 'v', value: string) => {
      setRows((prev) =>
        prev.map((r) => (r.id === id ? { ...r, [field]: value } : r)),
      );
    },
    [],
  );

  // 모든 행이 유효한지 (빈 행 허용, 부분 입력은 disable).
  const allRowsValid = useMemo(() => rows.every(isRowValid), [rows]);

  // field 검증 — 빈 값 허용, 정규식 위반 시 에러.
  const fieldNameValidation = useMemo(
    () => validateMetricType(fieldName),
    [fieldName],
  );

  // 중복 태그 키 검출 (마지막 값이 이긴다 — 안내만, 저장은 허용).
  const duplicateKeys = useMemo(() => {
    const seen = new Map<string, number>();
    for (const row of rows) {
      const k = row.k.trim();
      if (k === '') continue;
      seen.set(k, (seen.get(k) ?? 0) + 1);
    }
    const dupes = new Set<string>();
    for (const [k, count] of seen) {
      if (count > 1) dupes.add(k);
    }
    return dupes;
  }, [rows]);

  const canConfirm = allRowsValid && fieldNameValidation.valid && !isSubmitting;

  const disabledReason = useMemo(() => {
    if (!fieldNameValidation.valid)
      return fieldNameValidation.error ?? t('property.meta.fieldNameFormatCheck');
    if (!allRowsValid) return t('property.meta.tagInputCheck');
    return undefined;
  }, [fieldNameValidation, allRowsValid, t]);

  // 저장 실행 — 비어있지 않은 행만 모아 태그 맵을 구성하고 부모에 전체 전달.
  const handleConfirm = useCallback(async () => {
    if (!canConfirm) return;
    const tags: Record<string, string> = {};
    for (const row of rows) {
      const k = row.k.trim();
      const v = row.v.trim();
      if (k === '' && v === '') continue;
      tags[k] = v;
    }
    const trimmedMetric = fieldName.trim();
    const payload: EditKeyMetaPayload = { tags };
    if (trimmedMetric !== '') {
      payload.field = trimmedMetric;
    }
    await onConfirm(payload);
  }, [canConfirm, rows, fieldName, onConfirm]);

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
      aria-labelledby="edit-key-meta-title"
    >
      <div
        className="mx-4 flex w-full max-w-[480px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="edit-key-meta-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {t('property.editMeta.title')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
            aria-label={t('property.meta.closeAria')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 px-5 py-4">
          {/* 키 표시 (읽기 전용) */}
          <div>
            <p className="mb-1 text-xs font-medium text-(--color-text-muted)">{t('property.meta.key')}</p>
            <p className="break-all rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 font-mono text-sm text-(--color-text-primary)">
              {keyName}
            </p>
          </div>

          {/* field 입력 (선택) */}
          <div>
            <label
              htmlFor="edit-metric-type"
              className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
            >
              {t('property.meta.fieldNameOptional')}
            </label>
            <p className="mb-1.5 text-[11px] text-(--color-text-muted)">
              {t('property.editMeta.fieldNameHelp')}
            </p>
            <input
              id="edit-metric-type"
              type="text"
              value={fieldName}
              onChange={(e) => setField(e.target.value)}
              onKeyDown={handleInputKeyDown}
              disabled={isSubmitting}
              placeholder="unknown"
              aria-invalid={!fieldNameValidation.valid || undefined}
              aria-describedby={
                !fieldNameValidation.valid ? 'edit-metric-type-error' : undefined
              }
              className={cn(
                inputCls,
                'w-full',
                !fieldNameValidation.valid && errorInputCls,
                isSubmitting && 'cursor-not-allowed opacity-60',
              )}
            />
            {!fieldNameValidation.valid && (
              <p
                id="edit-metric-type-error"
                className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400"
              >
                {fieldNameValidation.error}
              </p>
            )}
          </div>

          {/* 태그 편집 (전체 교체) */}
          <div>
            <p className="mb-1 text-xs font-medium text-(--color-text-secondary)">
              {t('property.tag.optional')}
            </p>
            <p className="mb-2 text-[11px] text-(--color-text-muted)">
              {t('property.editMeta.tagsLoadedHelp')}
            </p>

            {rows.length === 0 ? (
              <p className="rounded-md border border-dashed border-(--color-border-default) px-3 py-3 text-center text-xs text-(--color-text-muted)">
                {t('property.tag.empty')}
              </p>
            ) : (
              <div className="space-y-1.5">
                {rows.map((row) => {
                  const k = row.k.trim();
                  const v = row.v.trim();
                  const keyInvalid = k !== '' && !isValidTagKey(k);
                  const valueInvalid = k !== '' && v === '';
                  const orphanValue = k === '' && v !== '';
                  const isDuplicate = k !== '' && duplicateKeys.has(k);

                  return (
                    <div key={row.id}>
                      <div className="flex items-center gap-1.5">
                        <input
                          type="text"
                          value={row.k}
                          onChange={(e) => handleRowChange(row.id, 'k', e.target.value)}
                          onKeyDown={handleInputKeyDown}
                          placeholder={t('property.tag.keyPlaceholder')}
                          disabled={isSubmitting}
                          aria-label={t('property.tag.keyAria')}
                          aria-invalid={keyInvalid || orphanValue || undefined}
                          className={cn(
                            inputCls,
                            'flex-1',
                            (keyInvalid || orphanValue || isDuplicate) && errorInputCls,
                            isSubmitting && 'cursor-not-allowed opacity-60',
                          )}
                        />
                        <span className="text-(--color-text-muted)">=</span>
                        <input
                          type="text"
                          value={row.v}
                          onChange={(e) => handleRowChange(row.id, 'v', e.target.value)}
                          onKeyDown={handleInputKeyDown}
                          placeholder={t('property.tag.valuePlaceholder')}
                          disabled={isSubmitting}
                          aria-label={t('property.tag.valueAria')}
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
                          className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                          aria-label={t('property.tag.deleteAria')}
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                      {keyInvalid && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          {t('property.tag.keyInvalid')}
                        </p>
                      )}
                      {valueInvalid && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          {t('property.tag.valueRequired')}
                        </p>
                      )}
                      {orphanValue && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          {t('property.tag.keyRequired')}
                        </p>
                      )}
                      {isDuplicate && !keyInvalid && !valueInvalid && (
                        <p className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                          {t('property.tag.duplicate')}
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
              {t('property.tag.add')}
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
            {t('property.editMeta.cancel')}
          </button>
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={!canConfirm}
            title={disabledReason}
            data-testid="edit-key-meta-confirm"
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {isSubmitting ? t('property.editMeta.saving') : t('property.editMeta.save')}
          </button>
        </div>
      </div>
    </div>
  );
}
