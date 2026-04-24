// 태그 기반 필터 칩 그룹.
//
// 서버가 제공하는 `StoreTagPair[]` (각 pair = { key, values })를 받아
// 태그 키별로 칩 그룹을 렌더링하고, 선택된 필터를 "key=value" 문자열 집합으로
// 상위에 통보한다.
//
// 사용처:
//   - AgentDetailPanel.StoreTab: 저장소 엔트리 테이블 필터
//   - TsdbDataViewerModal (Store): 멀티셀렉트 가능 시리즈 키 풀 필터
//
// @spec SPEC-WEB-005 SPEC-STORE-003

import { useCallback } from 'react';

import { cn } from '@/lib/utils/cn';
import type { StoreTagPair } from '@/services/api/store';

// ---- Props ----

interface TagFilterChipsProps {
  /** 서버가 제공한 태그 쌍 목록 (키별로 사용 중인 값들). */
  pairs: StoreTagPair[];
  /**
   * 현재 선택된 필터. "tagKey=tagValue" 형식 문자열의 집합.
   * 예: `["room=1", "type=temperature"]`
   */
  selected: ReadonlySet<string>;
  /** 선택 토글 콜백. filterId 는 "tagKey=tagValue" 형식. */
  onToggle: (filterId: string) => void;
  /** 전체 해제 콜백. 선택이 0 이면 호출되지 않는다. */
  onClearAll: () => void;
  /** 선택이 전혀 없는 경우 표시할 프롬프트 텍스트. 기본: "태그로 필터링" */
  label?: string;
}

/** "tagKey=tagValue" 형식의 ID 를 만든다. 값에 '=' 가 포함되어 있어도 안전하게 파싱 가능하다. */
export function makeFilterId(tagKey: string, tagValue: string): string {
  return `${tagKey}=${tagValue}`;
}

// ---- 스타일 ----

const chipBase = cn(
  'inline-flex items-center gap-1 rounded-full px-2.5 py-0.5',
  'text-xs font-medium transition-colors',
  'border',
);

const chipIdle = cn(
  chipBase,
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-secondary)',
  'hover:bg-(--color-bg-elevated)',
);

const chipSelected = cn(
  chipBase,
  'border-blue-500 bg-blue-600 text-white',
  'hover:bg-blue-700',
  'dark:bg-blue-500 dark:hover:bg-blue-600',
);

// ---- 컴포넌트 ----

export function TagFilterChips({
  pairs,
  selected,
  onToggle,
  onClearAll,
  label = '태그로 필터링',
}: TagFilterChipsProps) {
  // 활성 필터 요약 (key=value pairs).
  const activeFilters = Array.from(selected);

  const handleClick = useCallback(
    (tagKey: string, tagValue: string) => {
      onToggle(makeFilterId(tagKey, tagValue));
    },
    [onToggle],
  );

  return (
    <div className="space-y-2">
      {/* 상태 헤더: 활성 필터 요약 + 전체 해제 */}
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-(--color-text-muted)">
          {activeFilters.length > 0 ? (
            <>
              <span className="font-medium text-(--color-text-secondary)">
                필터:
              </span>{' '}
              {activeFilters.join(', ')}
            </>
          ) : (
            label
          )}
        </p>
        {activeFilters.length > 0 && (
          <button
            type="button"
            onClick={onClearAll}
            className="text-[11px] font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
          >
            전체 해제
          </button>
        )}
      </div>

      {/* 태그 키별 칩 그룹 */}
      <div className="space-y-1.5">
        {pairs.map((pair) => (
          <div key={pair.key} className="flex flex-wrap items-center gap-1.5">
            <span className="min-w-[5rem] text-xs font-medium text-(--color-text-muted)">
              {pair.key}:
            </span>
            {pair.values.map((v) => {
              const id = makeFilterId(pair.key, v);
              const isSelected = selected.has(id);
              return (
                <button
                  key={v}
                  type="button"
                  onClick={() => handleClick(pair.key, v)}
                  className={isSelected ? chipSelected : chipIdle}
                  aria-pressed={isSelected}
                  data-testid={`tag-filter-${pair.key}-${v}`}
                >
                  {v}
                </button>
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- 필터 매칭 유틸 ----

/**
 * 엔트리의 태그 맵이 선택된 필터 집합을 AND 로 모두 만족하는지 확인한다.
 *
 * - selected 가 비어 있으면 항상 true.
 * - selected 의 각 "key=value" 에 대해 entryTags[key] === value 여야 한다.
 * - 같은 태그 키에 여러 값이 동시에 선택되면(예: room=1 + room=2) 둘 다 만족하는
 *   엔트리는 없으므로 false (AND 의 정상적 결과).
 *
 * @spec SPEC-STORE-003
 */
export function matchesTagFilter(
  entryTags: Record<string, string> | null | undefined,
  selected: ReadonlySet<string>,
): boolean {
  if (selected.size === 0) return true;
  if (!entryTags) return false;
  for (const id of selected) {
    const eqIdx = id.indexOf('=');
    if (eqIdx < 0) continue; // 잘못된 형식 무시
    const tagKey = id.slice(0, eqIdx);
    const tagValue = id.slice(eqIdx + 1);
    if (entryTags[tagKey] !== tagValue) {
      return false;
    }
  }
  return true;
}
