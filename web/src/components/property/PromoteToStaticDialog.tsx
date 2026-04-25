// 동적 키를 정적 키로 변환하기 위한 모달 컴포넌트.
//
// Store 에이전트의 storage 목록에서 "동적" 키(설정의 keys 배열에 등록되지 않은 키)
// 를 사용자가 선택하여 정적 키로 승격(promote)할 때 사용한다.
// 사용자는 변환 시점에 옵션 태그를 입력할 수 있다.
//
// 백엔드 변경 없이 PUT /api/v1/agents/{id}/config 엔드포인트를 통해 config.keys
// 배열에 새 엔트리를 추가하는 방식으로 동작한다.
//
// @spec SPEC-STORE-003

import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from 'react';
import { Loader2, Plus, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- Props ----

interface PromoteToStaticDialogProps {
  /** 모달 표시 여부 */
  isOpen: boolean;
  /** 모달 닫기 핸들러 (취소 또는 외부 클릭 시) */
  onClose: () => void;
  /** 승격할 키 이름 (읽기 전용 표시). */
  keyName: string;
  /**
   * 변환 확인 핸들러.
   * 입력된 태그 맵을 받아 부모에서 PUT /agents/{id}/config 호출을 처리한다.
   * 태그가 비어있으면 빈 객체 `{}` 를 전달한다.
   */
  onConfirm: (tags: Record<string, string>) => void | Promise<void>;
  /** 부모의 mutation 진행 중 상태. true 이면 변환 버튼 비활성 + 스피너 표시. */
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

// ---- 컴포넌트 ----

export function PromoteToStaticDialog({
  isOpen,
  onClose,
  keyName,
  onConfirm,
  isSubmitting = false,
}: PromoteToStaticDialogProps) {
  // 태그 행 상태. 모달이 닫힐 때 초기화한다.
  const [rows, setRows] = useState<TagRow[]>([]);

  // 모달 열림/닫힘에 따른 상태 리셋.
  useEffect(() => {
    if (isOpen) {
      setRows([]);
    }
  }, [isOpen]);

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

  // 변환 버튼 활성 조건: 모든 행 유효 + 제출 중이 아님.
  const canConfirm = allRowsValid && !isSubmitting;

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
    await onConfirm(tags);
  }, [canConfirm, rows, onConfirm]);

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
                            (keyInvalid || orphanValue) &&
                              'border-amber-400 focus:border-amber-500 focus:ring-amber-400',
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
                            valueInvalid &&
                              'border-amber-400 focus:border-amber-500 focus:ring-amber-400',
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
