// 정적 키 등록을 위한 "키 선택" 모달 컴포넌트.
//
// 자동 검색(runtime auto-registration)으로 생성된 동적 키 목록에서 하나를 선택하거나,
// 목록에 없는 키 이름을 직접 입력(수동)하여 정적 키 등록 대상을 고른다.
//
// 이 다이얼로그는 키를 "고르기만" 하며, 실제 정적 등록(data_type/metric_type/tags 확정 및
// PUT /agents/{id}/config)은 부모가 선택된 키로 기존 PromoteToStaticDialog 를 열어 처리한다.
// 즉 선택(1단계) → 설정/확정(2단계)의 2단계 플로우 중 1단계를 담당한다.
//
// @spec SPEC-STORE-003 v0.3.0
// @spec SPEC-WEB-005

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';
import { Plus, Search, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import type { StoreKeyObject } from '@/services/api/store';

interface SelectStaticKeyDialogProps {
  /** 모달 표시 여부. */
  isOpen: boolean;
  /** 모달 닫기(취소/외부 클릭/ESC). */
  onClose: () => void;
  /**
   * 등록 후보(자동 검색된 동적 키). 이미 정적인 키는 부모가 제외해 전달한다.
   * 각 항목의 data_type 은 후보 목록에 뱃지로 표시하고, 선택 시 부모가
   * PromoteToStaticDialog 의 default data_type 으로 재활용한다.
   */
  candidates: StoreKeyObject[];
  /**
   * 키 선택 시 호출된다(후보 선택 또는 수동 입력). 부모는 이 키로 정적 변환
   * 다이얼로그를 연다. 단일 선택이며, 반복 등록은 부모가 다시 이 다이얼로그를 열어 처리한다.
   */
  onSelect: (key: string) => void;
}

/**
 * 정적 키 등록 대상 선택 모달.
 *
 * - 검색 입력: 후보 목록을 부분일치(대소문자 무시)로 필터링하며, 동시에 수동 입력란으로도 쓴다.
 * - 후보 행 클릭: 해당 키를 선택.
 * - 수동 등록: 입력값과 정확히 일치하는 후보가 없으면 "직접 등록: '<입력값>'" 행을 노출한다.
 */
export default function SelectStaticKeyDialog({
  isOpen,
  onClose,
  candidates,
  onSelect,
}: SelectStaticKeyDialogProps): React.ReactElement | null {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // 열릴 때마다 입력 초기화 + 포커스.
  useEffect(() => {
    if (!isOpen) return;
    setQuery('');
    const id = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(id);
  }, [isOpen]);

  // ESC 로 닫기.
  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: globalThis.KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, onClose]);

  const trimmed = query.trim();
  const lower = trimmed.toLowerCase();

  const filtered = useMemo(() => {
    if (lower === '') return candidates;
    return candidates.filter((c) => {
      if (c.key.toLowerCase().includes(lower)) return true;
      if (c.metric_type.toLowerCase().includes(lower)) return true;
      return Object.entries(c.tags).some(
        ([k, v]) =>
          k.toLowerCase().includes(lower) || v.toLowerCase().includes(lower),
      );
    });
  }, [candidates, lower]);

  // 입력값과 정확히 일치하는 후보가 없으면 수동 등록 행을 노출한다.
  const showManual =
    trimmed !== '' && !candidates.some((c) => c.key === trimmed);

  const handleSelect = useCallback(
    (key: string) => {
      const k = key.trim();
      if (k === '') return;
      onSelect(k);
    },
    [onSelect],
  );

  const handleInputKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key !== 'Enter') return;
      e.preventDefault();
      // 정확히 일치하는 후보가 있으면 그 키를, 없고 입력값이 있으면 수동 등록.
      if (trimmed !== '') handleSelect(trimmed);
    },
    [trimmed, handleSelect],
  );

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="select-static-key-title"
    >
      <div
        className="mx-4 flex max-h-[80vh] w-full max-w-[480px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="select-static-key-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {t('agents.detail.store.selectStaticKeyTitle')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('property.meta.closeAria')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="flex min-h-0 flex-col gap-3 px-5 py-4">
          <p className="text-xs text-(--color-text-muted)">
            {t('agents.detail.store.selectStaticKeyDesc')}
          </p>

          {/* 검색 / 수동 입력 */}
          <div className="relative">
            <Search
              className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-(--color-text-muted)"
              aria-hidden="true"
            />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleInputKeyDown}
              placeholder={t('agents.detail.store.selectStaticKeySearchPlaceholder')}
              aria-label={t('agents.detail.store.selectStaticKeySearchPlaceholder')}
              data-testid="select-static-key-input"
              className="w-full rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) py-1.5 pl-8 pr-3 text-xs text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          {/* 후보 목록 + 수동 등록 행 */}
          <div
            className="min-h-0 flex-1 overflow-y-auto rounded-md border border-(--color-border-default)"
            data-testid="select-static-key-list"
          >
            {filtered.length === 0 && !showManual && (
              <p className="px-3 py-6 text-center text-xs text-(--color-text-muted)">
                {t('agents.detail.store.selectStaticKeyEmpty')}
              </p>
            )}

            <ul className="divide-y divide-(--color-border-default)">
              {filtered.map((c) => (
                <li key={c.key}>
                  <button
                    type="button"
                    onClick={() => handleSelect(c.key)}
                    data-testid={`select-static-key-candidate-${c.key}`}
                    className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left transition-colors hover:bg-(--color-bg-elevated)"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block break-all font-mono text-xs text-(--color-text-primary)">
                        {c.key}
                      </span>
                      {c.metric_type && c.metric_type !== 'unknown' && (
                        <span className="text-[10px] text-(--color-text-muted)">
                          {c.metric_type}
                        </span>
                      )}
                    </span>
                    <span className="shrink-0 rounded bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-[10px] text-(--color-text-secondary)">
                      {c.data_type}
                    </span>
                  </button>
                </li>
              ))}

              {showManual && (
                <li>
                  <button
                    type="button"
                    onClick={() => handleSelect(trimmed)}
                    data-testid="select-static-key-manual"
                    className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-blue-600 transition-colors hover:bg-(--color-bg-elevated) dark:text-blue-400"
                  >
                    <Plus className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    <span className="min-w-0 break-all">
                      {t('agents.detail.store.selectStaticKeyManual').replace(
                        '{key}',
                        trimmed,
                      )}
                    </span>
                  </button>
                </li>
              )}
            </ul>
          </div>
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)"
          >
            {t('property.promote.cancel')}
          </button>
        </div>
      </div>
    </div>
  );
}
