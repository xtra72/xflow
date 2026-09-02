// 시스템 관측 대상 선택기 (마운트 / 디스크 장치 / 네트워크 인터페이스).
//
// 호스트가 실제로 가진 목록을 받아 체크박스로 고르게 한다. 손으로 적던 것을
// 목록에서 고르게 바꾸는 것이 목적이므로, 조회 실패나 목록이 빈 경우에도 편집이
// 막히지 않도록 직접 입력 경로를 함께 둔다.

import { useCallback, useMemo, useState } from 'react';
import { AlertTriangle, Plus, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { useSysResources, type SysResources } from '@/services/api/monitorService';

/** 어떤 축의 목록을 고를지 */
export type SysResourceKind = keyof SysResources;

interface SysResourceSelectorProps {
  kind: SysResourceKind;
  /** 현재 선택값. 빈 배열이면 "전체" 를 뜻한다(백엔드 규약과 동일). */
  value: unknown;
  onChange: (value: string[]) => void;
  readOnly?: boolean;
}

/** 저장된 값을 문자열 배열로 정규화한다. */
function toSelected(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is string => typeof v === 'string' && v !== '');
}

export function SysResourceSelector({
  kind,
  value,
  onChange,
  readOnly,
}: SysResourceSelectorProps) {
  const { t } = useTranslation();
  const { data, isLoading, isError } = useSysResources();
  const [draft, setDraft] = useState('');

  const selected = useMemo(() => toSelected(value), [value]);
  const available = useMemo(() => data?.[kind] ?? [], [data, kind]);

  /**
   * 화면에 그릴 항목 = 호스트 목록 ∪ 이미 고른 값.
   *
   * 이미 고른 값이 지금 호스트에 없을 수 있다(마운트 해제, NIC 제거, 다른 장비의
   * 설정을 복사). 목록에만 의존해 그리면 그 항목이 화면에서 사라지고, 사용자가
   * 저장하는 순간 조용히 설정에서 빠진다.
   */
  const entries = useMemo(() => {
    const known = new Set(available);
    const missing = selected.filter((name) => !known.has(name));
    return [
      ...available.map((name) => ({ name, present: true })),
      ...missing.map((name) => ({ name, present: false })),
    ];
  }, [available, selected]);

  const toggle = useCallback(
    (name: string) => {
      onChange(
        selected.includes(name)
          ? selected.filter((n) => n !== name)
          : // 목록 순서를 유지해야 저장값이 클릭 순서에 따라 흔들리지 않는다.
            [...selected, name].sort(),
      );
    },
    [selected, onChange],
  );

  const addDraft = useCallback(() => {
    const name = draft.trim();
    if (!name || selected.includes(name)) {
      setDraft('');
      return;
    }
    onChange([...selected, name].sort());
    setDraft('');
  }, [draft, selected, onChange]);

  return (
    <div className="space-y-1.5" data-testid={`sysresource-${kind}`}>
      {isLoading && (
        <p className="text-xs text-(--color-text-muted)">{t('common.loading')}</p>
      )}

      {isError && (
        <p
          data-testid={`sysresource-${kind}-error`}
          className="flex items-center gap-1 text-xs text-yellow-700 dark:text-yellow-300"
        >
          <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
          {t('agents.sysResource.loadFailed')}
        </p>
      )}

      {!isLoading && entries.length === 0 && !isError && (
        <p className="text-xs text-(--color-text-muted)">
          {t('agents.sysResource.empty')}
        </p>
      )}

      {entries.length > 0 && (
        <div className="max-h-48 space-y-0.5 overflow-y-auto rounded-md border border-(--color-border-default) p-1">
          {entries.map(({ name, present }) => (
            <label
              key={name}
              className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)"
            >
              <input
                type="checkbox"
                disabled={readOnly}
                checked={selected.includes(name)}
                onChange={() => toggle(name)}
                data-testid={`sysresource-${kind}-option-${name}`}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="truncate text-sm text-(--color-text-primary)">{name}</span>
              {!present && (
                <span
                  data-testid={`sysresource-${kind}-missing-${name}`}
                  className="shrink-0 rounded bg-yellow-100 px-1 py-px text-[10px] text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300"
                >
                  {t('agents.sysResource.missing')}
                </span>
              )}
            </label>
          ))}
        </div>
      )}

      {/* 목록에 없는 대상을 미리 지정하는 경로. 아직 붙지 않은 디스크나 조회가
          실패한 상황에서도 설정을 이어갈 수 있어야 한다. */}
      {!readOnly && (
        <div className="flex items-center gap-1">
          <input
            type="text"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                addDraft();
              }
            }}
            placeholder={t('agents.sysResource.addPlaceholder')}
            data-testid={`sysresource-${kind}-draft`}
            className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-400 focus:outline-none"
          />
          <button
            type="button"
            onClick={addDraft}
            data-testid={`sysresource-${kind}-add`}
            className="flex shrink-0 items-center gap-0.5 rounded-md border border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Plus className="h-3 w-3" />
            {t('agents.sysResource.add')}
          </button>
        </div>
      )}

      <div className="flex items-center justify-between">
        <p className="text-[10px] text-(--color-text-muted)">
          {selected.length === 0
            ? t('agents.sysResource.allHint')
            : `${t('agents.sysResource.selected')}: ${selected.length}`}
        </p>
        {selected.length > 0 && !readOnly && (
          <button
            type="button"
            onClick={() => onChange([])}
            data-testid={`sysresource-${kind}-clear`}
            className="flex items-center gap-0.5 rounded px-1 py-0.5 text-[10px] text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
          >
            <X className="h-3 w-3" />
            {t('agents.sysResource.clear')}
          </button>
        )}
      </div>
    </div>
  );
}
