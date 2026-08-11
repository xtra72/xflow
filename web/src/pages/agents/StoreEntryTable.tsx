// 공용 Store 엔트리 테이블 — 에이전트 상세와 패널 설정이 공유하는 단일 소스.
//
// @spec SPEC-PANEL-SETTINGS-001 (T5 / 행위 보존 추출)
//
// 배경: 에이전트 상세(`AgentDetailPanel.tsx`)에 강결합돼 있던 Store 리스트의
// 헤더/본문 단일 목록 렌더(thead·tbody), 행 렌더(`StoreEntryRow`)와 셀 렌더
// (`renderCell`)를 이 파일로 추출한다. `storeColumns`(레지스트리/헤더/필터)와
// `storeEntrySort`(정렬)는 이동하지 않고 그대로 참조한다(중복 방지).
//
// 컨텍스트 분기는 props 주입으로 해결한다:
//   - 에이전트 상세: `rowActions`(정적/동적 액션 핸들러) 주입, `actions` 컬럼 포함,
//     `selection`/`renderCellExtra` 미주입 → 기존 동작 그대로(회귀 0, AC-16).
//   - 패널 설정(후속 마일스톤 T6/T7): `columns` 에서 `actions`→`alias` 치환,
//     `selection`(행 선행 체크박스) + `renderCellExtra`(Alias Name 셀) 주입.

import { useCallback, useEffect, useState } from 'react';
import { ArrowUpCircle, ChevronRight, Lock, Pencil, Tag, Trash2 } from 'lucide-react';

import { useExecAgent } from '@/hooks/useAgent';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { ColumnHeader, type StoreColumn, type StoreColumnId } from './storeColumns';
import type {
  ColumnFilter,
  ColumnFilterMap,
  FilterColumnId,
  SortState,
} from './storeEntrySort';
import {
  extractEntryMetricType,
  extractEntryTags,
  formatTimeAgo,
} from './storeEntryHelpers';

// ---- 컨텍스트 주입 계약 ----

/**
 * 에이전트 상세 컨텍스트의 행 액션 핸들러 묶음.
 * 패널 설정 컨텍스트(후속)에서는 readOnly 로 액션을 숨기고 selection 을 주입한다.
 */
export interface StoreEntryRowActions {
  agentId: string;
  onPromote: (key: string) => void;
  onRename: (key: string) => void;
  onReset: (key: string) => void;
  onEditMeta: (key: string) => void;
  /** READ-ONLY(원격 타깃/패널 설정): 변환/초기화/히스토리 어포던스를 숨긴다. */
  readOnly?: boolean;
}

/**
 * 행 선택(체크박스) 컨텍스트. 패널 설정 전용(후속 마일스톤 T6/T7).
 * 미주입 시 선행 체크박스 컬럼은 렌더되지 않아 에이전트 상세 동작이 불변이다.
 */
export interface StoreEntrySelectionContext {
  isSelected: (entry: Record<string, unknown>) => boolean;
  onToggle: (entry: Record<string, unknown>) => void;
}

// ---- 단일 행 렌더 ----

function StoreEntryRow({
  entry,
  columns,
  keyExpanded,
  maxHistorySize,
  agentId,
  isStatic,
  onPromote,
  onRename,
  onReset,
  onEditMeta,
  readOnly = false,
  selection,
  rowDetail,
  renderCellExtra,
}: {
  entry: Record<string, unknown>;
  /**
   * 렌더할(보이는) 컬럼 목록. 헤더(thead)와 동일한 단일 출처를 공유하여 숨김 컬럼이
   * 헤더/본문에서 함께 사라지고, 히스토리 확장 행의 colSpan 이 항상 일치하도록 한다.
   */
  columns: readonly StoreColumn[];
  /**
   * 키 컬럼 전체 확장 상태. 개별 행이 아니라 키 컬럼 헤더에서 일괄 제어한다.
   * true 면 전체 키를, false 면 앞 8자만 표시한다.
   */
  keyExpanded: boolean;
  maxHistorySize: number;
  agentId: string;
  /** READ-ONLY(원격 타깃): 변환/초기화/히스토리(exec) 어포던스를 숨긴다. */
  readOnly?: boolean;
  /**
   * 이 엔트리의 키가 정적(설정의 keys 배열에 등록됨)인지 여부.
   * @spec SPEC-STORE-003
   */
  isStatic: boolean;
  /**
   * 동적 키를 정적으로 승격할 때 호출되는 핸들러.
   * 정적 키 행에서는 사용되지 않는다.
   * @spec SPEC-STORE-003
   */
  onPromote: (key: string) => void;
  /**
   * 동적 키(그 키의 모든 시리즈)를 새 키로 이동하는 핸들러. 동적 키에서만 노출된다.
   * @spec SPEC-STORE-004
   */
  onRename: (key: string) => void;
  /**
   * 행별 초기화 핸들러. 정적/동적 모두에서 노출되며 클릭 시 부모가
   * 확인 다이얼로그를 띄운다.
   * @spec SPEC-STORE-003
   */
  onReset: (key: string) => void;
  /**
   * 타입(metric_type)/태그 편집 핸들러. 정적/동적 모두에서 노출되며 클릭 시
   * 부모가 편집 다이얼로그를 띄운다.
   * @spec SPEC-STORE-003 v0.4.0
   */
  onEditMeta: (key: string) => void;
  /**
   * 행 선택 체크박스 컨텍스트. 미주입 시 선행 체크박스 셀을 렌더하지 않는다
   * (에이전트 상세 동작 불변). @spec SPEC-PANEL-SETTINGS-001 (후속 T6/T7)
   */
  selection?: { selected: boolean; onToggle: () => void };
  /**
   * 선택(체크)된 행의 인라인 펼침 상세(패널 설정 전용, REQ-18/19/20/21). 미주입 시 어떤
   * 행도 펼치지 않는다(에이전트 상세 동작 불변). `expandable` 인 행(=선택된 행)만 키 셀에
   * 펼침 셰브론을 노출하고, `expanded` 이면 그 행 아래 colSpan 상세 행에 `content` 를 렌더한다.
   * 상세에는 키 전체 상세 + 시리즈 이름/색상/선 스타일/(heatmap)위치 편집이 함께 들어간다.
   * @spec SPEC-PANEL-SETTINGS-001 (REQ-18/19/20/21)
   */
  rowDetail?: {
    expandable: boolean;
    expanded: boolean;
    onToggle: () => void;
    content: React.ReactNode;
  };
  /**
   * 컨텍스트별 셀 오버라이드(예: 패널 설정의 Alias Name 컬럼). 기본 컬럼 세트가
   * 처리하지 않는 컬럼 id 에 대해 호출된다. 미주입 시 미처리 컬럼은 null 을 렌더한다.
   * @spec SPEC-PANEL-SETTINGS-001 (후속 T6/T7)
   */
  renderCellExtra?: (
    columnId: StoreColumnId,
    entry: Record<string, unknown>,
  ) => React.ReactNode;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyData, setHistoryData] = useState<Array<{ value: unknown; timestamp: string }> | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const execAgent = useExecAgent();

  // 라이브 값이 없는 엔트리(예: 패널 설정의 메타데이터 파생 행)도 안전하게 처리한다.
  // JSON.stringify(undefined) 는 문자열이 아닌 undefined 를 반환하므로 '' 로 폴백한다.
  // 값이 항상 정의된 에이전트 상세 경로에서는 동작이 불변이다.
  const valueStr =
    typeof entry.value === 'string' ? entry.value : JSON.stringify(entry.value) ?? '';
  const truncated = valueStr.length > 60;
  const displayValue = truncated && !expanded ? valueStr.slice(0, 60) + '...' : valueStr;

  const updatedAt = entry.updated_at ? new Date(entry.updated_at as string) : null;
  const timeAgo = updatedAt ? formatTimeAgo(updatedAt, t) : '-';

  const stateHistoryCount = (entry.history_count as number) || 0;
  const historyCount = historyData !== null ? historyData.length : stateHistoryCount;
  // 히스토리 토글은 exec(get_history) 에 의존하므로 원격 READ-ONLY 에서는 비활성.
  const hasHistory = maxHistorySize > 0 && !readOnly;

  // fetchHistory 는 get_history(exec)로 현재 히스토리를 조회하여 로컬 state 에 반영한다.
  const fetchHistory = useCallback(() => {
    setHistoryLoading(true);
    execAgent.mutate(
      {
        id: agentId,
        req: {
          command: 'get_history',
          params: {
            // @spec SPEC-STORE-004: 히스토리는 인코딩 시리즈 키(storage_key)로 저장되므로
            // 디코드된 표시용 key 가 아니라 storage_key 로 조회해야 한다(없으면 key 폴백 — 레거시).
            key: (entry.storage_key as string) || (entry.key as string),
            // 네임스페이스 라운드트립 버그 수정 (v0.7.0 M14):
            // entry.namespace="" (빈 문자열)인 경우도 그대로 전송.
            // || 'default' 는 falsy 체크로 "" 를 "default" 로 강제했는데,
            // 백엔드의 ForNamespace("") 조회와 불일치 → 히스토리 0개 버그 발생.
            // ?? '' 를 사용하여 undefined/null 만 기본값으로, "" 는 유지.
            namespace: (entry.namespace as string) ?? '',
          },
        },
      },
      {
        onSuccess: (res) => {
          const data = res as unknown as Record<string, unknown>;
          const history = (data?.history as Array<{ value: unknown; timestamp: string }>) ?? [];
          setHistoryData(history);
          setHistoryLoading(false);
        },
        onError: () => {
          setHistoryData([]);
          setHistoryLoading(false);
        },
      },
    );
  }, [execAgent, agentId, entry.storage_key, entry.key, entry.namespace]);

  const handleRowClick = useCallback(() => {
    if (!hasHistory) return;
    setHistoryOpen((open) => !open);
  }, [hasHistory]);

  // 히스토리가 열려 있는 동안 엔트리가 갱신되면(새로고침으로 값/카운트/갱신시각 변경)
  // 히스토리를 다시 가져온다. 이전에는 히스토리가 펼칠 때 단 한 번만 로컬 state 에
  // 캐시되어, 새로고침해도 갱신되지 않고 행을 닫았다 다시 열어야만 반영됐다.
  useEffect(() => {
    if (!historyOpen) return;
    fetchHistory();
    // entry 의 변경 지표(history_count/updated_at/value)가 바뀔 때만 재조회한다.
    // 값이 동일하면 deps 가 그대로라 불필요한 재조회가 발생하지 않는다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [historyOpen, entry.history_count, entry.updated_at, entry.value]);

  // 히스토리 확장 행의 colSpan 은 실제 렌더된 컬럼 개수와 항상 일치한다(단일 출처).
  // 선행 체크박스(selection) 컬럼이 있으면 그만큼 더한다.
  const colSpan = columns.length + (selection ? 1 : 0);

  const entryTags = extractEntryTags(entry);

  // 키 컬럼 표시: 키 컬럼 헤더의 전체 확장 토글(keyExpanded, 에이전트 상세)로 제어한다.
  // 패널 설정은 keyExpanded 를 쓰지 않고 선택 행 인라인 펼침(rowDetail) 안에서 키 전체
  // 상세를 노출하므로, 셀은 항상 축약(앞 8자)으로 두고 전체 키는 title 로 유지한다.
  const fullKey = entry.key as string;
  const displayKey =
    !keyExpanded && fullKey.length > 8 ? fullKey.slice(0, 8) : fullKey;

  // 동적 키의 "정적으로 변환" 버튼 클릭 핸들러.
  // 행 클릭(히스토리 토글)과 분리하기 위해 이벤트 전파를 막는다.
  const handlePromoteClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      // @spec SPEC-STORE-004: 승격은 config 정적 키(=사용자 key)에 추가하므로 디코드된
      // 사용자 key 를 쓴다(히스토리/메타편집의 storage_key 와 다름).
      onPromote(entry.key as string);
    },
    [entry.key, onPromote],
  );

  // 동적 키 "이름 변경" 버튼 클릭 핸들러. 사용자 관점 key(그 키의 모든 시리즈)를 대상으로 한다.
  // @spec SPEC-STORE-004
  const handleRenameClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onRename(entry.key as string);
    },
    [entry.key, onRename],
  );

  // 행별 초기화 버튼 클릭 핸들러. 행 클릭(히스토리 토글)과 분리한다.
  // @spec SPEC-STORE-003
  const handleResetClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onReset(entry.key as string);
    },
    [entry.key, onReset],
  );

  // 타입/태그 편집 버튼 클릭 핸들러. 행 클릭(히스토리 토글)과 분리한다.
  // @spec SPEC-STORE-003 v0.4.0
  const handleEditMetaClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      // @spec SPEC-STORE-004: 메타 편집은 인코딩 시리즈 키(storage_key)로 동작해야 한다.
      onEditMeta((entry.storage_key as string) || (entry.key as string));
    },
    [entry.storage_key, entry.key, onEditMeta],
  );

  // 모든 엔트리가 metric_type 을 갖는다 (동적 키는 "unknown"). 빈 값은 "unknown" 표시.
  // @spec SPEC-STORE-003 v0.4.0
  const metricType = extractEntryMetricType(entry);
  const metricTypeLabel = metricType || 'unknown';
  const isUnknownMetric = metricTypeLabel === 'unknown';

  // 컬럼 id → 셀 내용 렌더 함수. 헤더/본문이 공유하는 컬럼 목록을 매핑하며,
  // 각 셀의 JSX(이름/키8자/바인딩 배지/메트릭 배지/값 확장/태그 칩/TTL ∞/히스토리/갱신/액션)를
  // 여기서 반환한다. td 래퍼(정렬/키)는 아래 map 에서 부여한다.
  function renderCell(columnId: StoreColumnId): React.ReactNode {
    // 컨텍스트별 셀 오버라이드(예: 패널 설정의 Alias). renderCellExtra 가 undefined 를
    // 반환하면 기본 셀을 사용하고, 노드를 반환하면 그 노드로 대체한다(오버라이드).
    if (renderCellExtra) {
      const override = renderCellExtra(columnId, entry);
      if (override !== undefined) return override;
    }
    switch (columnId) {
      case 'key':
        return (
          <span className="inline-flex items-center gap-1">
            {hasHistory && (
              <ChevronRight className={cn('h-3 w-3 text-(--color-text-muted) transition-transform', historyOpen && 'rotate-90')} />
            )}
            {/* 선택(체크)된 행의 인라인 펼침 셰브론(패널 설정, REQ-18/19). 선택된 행에만
                노출하며, 클릭 시 그 행 아래 상세(키 전체 상세 + 시리즈 편집)를 토글한다. */}
            {rowDetail?.expandable && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  rowDetail.onToggle();
                }}
                aria-expanded={rowDetail.expanded}
                aria-label={t(
                  rowDetail.expanded
                    ? 'agents.detail.store.keyRowCollapseAriaLabel'
                    : 'agents.detail.store.keyRowExpandAriaLabel',
                )}
                title={t(
                  rowDetail.expanded
                    ? 'agents.detail.store.keyRowCollapseAriaLabel'
                    : 'agents.detail.store.keyRowExpandAriaLabel',
                )}
                className="inline-flex items-center rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-(--color-text-primary)"
              >
                <ChevronRight
                  className={cn('h-3 w-3 transition-transform', rowDetail.expanded && 'rotate-90')}
                  aria-hidden="true"
                />
              </button>
            )}
            <span className={keyExpanded ? 'break-all' : 'truncate'} title={fullKey}>
              {displayKey}
            </span>
          </span>
        );
      case 'binding':
        return isStatic ? (
          <span
            className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
            title={t('agents.detail.store.staticTooltip')}
          >
            <Lock className="h-2.5 w-2.5" aria-hidden="true" />
            {t('agents.detail.store.static')}
          </span>
        ) : (
          <span
            className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
            title={t('agents.detail.store.dynamicTooltip')}
          >
            {t('agents.detail.store.dynamic')}
          </span>
        );
      case 'metric':
        return (
          <span
            className={cn(
              'inline-flex max-w-[120px] items-center rounded-full px-2 py-0.5 font-mono text-[10px] font-medium',
              isUnknownMetric
                ? 'bg-(--color-bg-elevated) text-(--color-text-muted)'
                : 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
            )}
            title={t('agents.detail.store.metricTypeTooltip').replace('{type}', metricTypeLabel)}
          >
            <span className="truncate">{metricTypeLabel}</span>
          </span>
        );
      case 'value':
        return (
          <span
            className={truncated ? 'cursor-pointer hover:text-(--color-text-primary)' : ''}
            onClick={(e) => {
              if (truncated) {
                e.stopPropagation();
                setExpanded(!expanded);
              }
            }}
          >
            {displayValue}
          </span>
        );
      case 'namespace':
        return (entry.namespace as string) || '-';
      case 'tags':
        return entryTags ? (
          <div className="flex flex-wrap gap-1">
            {Object.entries(entryTags).map(([k, v]) => (
              <span
                key={k}
                className="inline-flex items-center rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-[10px] font-medium text-(--color-text-secondary)"
              >
                {k}={v}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-(--color-text-muted)">-</span>
        );
      case 'ttl':
        return (entry.ttl as string) || '∞';
      case 'history':
        return historyCount;
      case 'updated':
        return timeAgo;
      case 'actions':
        // 액션 컬럼 (SPEC-STORE-003): 타입/태그 편집 + 정적으로 변환 + 이름변경 + 초기화.
        //   동적 키: [편집] [정적으로 변환] [이름변경] [초기화]
        //   정적 키: [편집] [초기화]
        return (
          <span className="inline-flex items-center gap-1">
            {!readOnly && (
              <button
                type="button"
                onClick={handleEditMetaClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-indigo-50 hover:text-indigo-600 dark:hover:bg-indigo-900/20 dark:hover:text-indigo-400"
                title={t('agents.detail.store.editMetaTooltip')}
                aria-label={t('agents.detail.store.editMetaAriaLabel').replace('{key}', entry.key as string)}
              >
                <Tag className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {!readOnly && !isStatic && (
              <button
                type="button"
                onClick={handlePromoteClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                title={t('agents.detail.store.promoteTooltip')}
                aria-label={t('agents.detail.store.promoteAriaLabel').replace('{key}', entry.key as string)}
              >
                <ArrowUpCircle className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {!readOnly && !isStatic && (
              <button
                type="button"
                onClick={handleRenameClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-amber-50 hover:text-amber-600 dark:hover:bg-amber-900/20 dark:hover:text-amber-400"
                title={t('agents.detail.store.renameTooltip')}
                aria-label={t('agents.detail.store.renameAriaLabel').replace('{key}', entry.key as string)}
              >
                <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {!readOnly && (
              <button
                type="button"
                onClick={handleResetClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                title={isStatic ? t('agents.detail.store.resetHistoryTooltip') : t('agents.detail.store.deleteEntryTooltip')}
                aria-label={t('agents.detail.store.resetAriaLabel').replace('{key}', entry.key as string)}
              >
                <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {readOnly && <span className="text-(--color-text-muted)">-</span>}
          </span>
        );
      default:
        return null;
    }
  }

  // 컬럼별 td 래퍼 클래스. 기존 셀 스타일을 컬럼 id 로 매핑하여 보존한다.
  function cellClassName(column: StoreColumn): string {
    switch (column.id) {
      case 'key':
        // 폭은 column.widthClass(min-w-[260px])가 관리한다.
        return 'px-3 py-2 font-mono text-xs text-(--color-text-primary)';
      case 'binding':
      case 'metric':
        return 'px-3 py-2 text-xs';
      case 'value':
        return 'px-3 py-2 font-mono text-xs text-(--color-text-secondary) max-w-[300px]';
      case 'actions':
        return 'px-3 py-2 text-right text-xs';
      default:
        return 'px-3 py-2 text-xs text-(--color-text-muted)';
    }
  }

  return (
    <>
      <tr
        className={cn('hover:bg-(--color-bg-secondary)/50', hasHistory && 'cursor-pointer')}
        onClick={handleRowClick}
      >
        {selection && (
          <td className="w-8 px-3 py-2 text-xs" onClick={(e) => e.stopPropagation()}>
            <input
              type="checkbox"
              checked={selection.selected}
              onChange={selection.onToggle}
              className="h-3.5 w-3.5"
              aria-label={t('agents.detail.store.selectRowAriaLabel').replace('{key}', fullKey)}
            />
          </td>
        )}
        {columns.map((column) => (
          <td
            key={column.id}
            className={cn(cellClassName(column), column.widthClass)}
            title={column.id === 'updated' ? (entry.updated_at as string) : undefined}
          >
            {renderCell(column.id)}
          </td>
        ))}
      </tr>
      {/* 선택 행 인라인 펼침 상세(REQ-18/19/20/21) — 키 전체 상세 + 시리즈 편집을 한 행에. */}
      {rowDetail?.expanded && (
        <tr>
          <td
            colSpan={colSpan}
            className="bg-(--color-bg-secondary)/30 px-6 py-3"
            data-testid={`store-row-detail-${fullKey}`}
          >
            {rowDetail.content}
          </td>
        </tr>
      )}
      {historyOpen && (
        <tr>
          <td colSpan={colSpan} className="bg-(--color-bg-secondary)/30 px-6 py-3">
            {historyLoading ? (
              <p className="text-xs text-(--color-text-muted)">{t('agents.detail.store.loading')}</p>
            ) : historyData && historyData.length > 0 ? (
              <div className="space-y-1">
                <p className="text-xs font-medium text-(--color-text-muted) mb-2">
                  {t('agents.detail.store.historyTitle').replace('{count}', String(historyData.length))}
                </p>
                <div className="space-y-1">
                  {historyData.map((h, i) => (
                    <div key={i} className="flex items-baseline gap-3 text-xs">
                      <span className="text-(--color-text-muted) whitespace-nowrap">
                        {new Date(h.timestamp).toLocaleString('ko-KR')}
                      </span>
                      <span className="font-mono text-(--color-text-secondary)">
                        {typeof h.value === 'string' ? h.value : JSON.stringify(h.value)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <p className="text-xs text-(--color-text-muted)">{t('agents.detail.store.historyEmpty')}</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

// ---- 공용 테이블(헤더/본문 단일 목록) ----

export interface StoreEntryTableProps {
  /** 렌더할(페이지 가시) 엔트리 목록. */
  entries: readonly Record<string, unknown>[];
  /**
   * 렌더할 컬럼 세트(단일 출처). 호출자가 컨텍스트에 맞춰 계산해 주입한다.
   * 에이전트 상세는 `actions` 포함, 패널 설정은 `alias` 로 치환한 세트를 넘긴다.
   */
  columns: readonly StoreColumn[];
  sort: SortState;
  onSort: (next: SortState) => void;
  columnFilters: ColumnFilterMap;
  onColumnFilterChange: (columnId: FilterColumnId, next: ColumnFilter) => void;
  /** 필터 가능한 컬럼별 고유 값 목록(필터 드롭다운 옵션). */
  uniqueValuesByColumn: ReadonlyMap<FilterColumnId, readonly string[]>;
  /**
   * 키 컬럼 전체 확장 상태 + 헤더 토글(에이전트 상세). 패널 설정은 대신 `keyRowExpansion`
   * (행별 접기/펼치기)을 주입하며 이 두 값을 생략한다(헤더 일괄 토글 미노출).
   */
  keyColumnExpanded?: boolean;
  onToggleKeyExpanded?: () => void;
  maxHistorySize: number;
  /** 정적 키 이름 집합(행별 정적/동적 판정 O(1)). */
  staticKeyNames: ReadonlySet<string>;
  /** 행 액션 핸들러(에이전트 상세). */
  rowActions: StoreEntryRowActions;
  /**
   * 행 선택 컨텍스트(패널 설정 전용, 후속 T6/T7). 미주입 시 선행 체크박스 컬럼 미표시.
   */
  selection?: StoreEntrySelectionContext;
  /**
   * 선택(체크)된 행의 인라인 펼침 상세(패널 설정 전용, REQ-18/19/20/21). 미주입 시 어떤 행도
   * 펼치지 않는다(에이전트 상세 불변). `isExpandable` 인 행(=선택된 행)만 펼침 셰브론을
   * 노출하고, 펼치면 `renderDetail(entry)` 가 그 행 아래 colSpan 상세로 렌더된다.
   * @spec SPEC-PANEL-SETTINGS-001 (REQ-18/19/20/21)
   */
  rowExpansion?: {
    isExpandable: (entry: Record<string, unknown>) => boolean;
    isExpanded: (entry: Record<string, unknown>) => boolean;
    onToggle: (entry: Record<string, unknown>) => void;
    renderDetail: (entry: Record<string, unknown>) => React.ReactNode;
  };
  /**
   * 컨텍스트별 셀 오버라이드(패널 설정의 Alias 등, 후속 T6/T7). 미주입 시 기본 셀만.
   */
  renderCellExtra?: (
    columnId: StoreColumnId,
    entry: Record<string, unknown>,
  ) => React.ReactNode;
  /**
   * 컬럼 헤더별 추가 슬롯(패널 설정 전용). 지정한 컬럼 헤더에 커스텀 필터 어포던스
   * (예: 태그 컬럼의 전용 AND 태그 피커 팝오버)를 주입한다. 미주입 시 헤더는 기존과
   * 동일하게 렌더된다(에이전트 상세 무영향, 회귀 0). @spec SPEC-PANEL-SETTINGS-001
   */
  columnHeaderSlots?: Partial<Record<StoreColumnId, React.ReactNode>>;
}

/**
 * 헤더(thead)와 본문(tbody)이 동일한 `columns` 목록을 공유하는 공용 Store 테이블.
 * 숨김 컬럼이 헤더/본문에서 함께 사라지고, 히스토리 확장 행의 colSpan 이 항상 일치한다.
 */
export function StoreEntryTable({
  entries,
  columns,
  sort,
  onSort,
  columnFilters,
  onColumnFilterChange,
  uniqueValuesByColumn,
  keyColumnExpanded,
  onToggleKeyExpanded,
  maxHistorySize,
  staticKeyNames,
  rowActions,
  selection,
  rowExpansion,
  renderCellExtra,
  columnHeaderSlots,
}: StoreEntryTableProps) {
  const { t } = useTranslation();

  return (
    <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-(--color-border-default) bg-(--color-bg-secondary)">
            {/* 선행 체크박스 헤더(패널 설정 전용) — 미주입 시 렌더하지 않는다. */}
            {selection && (
              <th
                className="w-8 px-3 py-2"
                aria-label={t('agents.detail.store.selectColumnAriaLabel')}
              />
            )}
            {/* 헤더/본문이 동일한 columns 를 매핑 — 숨김 컬럼이 함께 사라진다. */}
            {columns.map((column) => (
              <ColumnHeader
                key={column.id}
                column={column}
                sort={sort}
                onSort={onSort}
                filter={column.filterColumn ? columnFilters[column.filterColumn] : undefined}
                uniqueValues={
                  column.filterColumn
                    ? uniqueValuesByColumn.get(column.filterColumn) ?? []
                    : []
                }
                onFilterChange={onColumnFilterChange}
                keyExpanded={column.id === 'key' ? keyColumnExpanded ?? false : undefined}
                onToggleKeyExpanded={
                  column.id === 'key' ? onToggleKeyExpanded : undefined
                }
                headerSlot={columnHeaderSlots?.[column.id]}
                t={t}
              />
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-(--color-border-default)">
          {entries.map((entry) => (
            <StoreEntryRow
              key={entry.key as string}
              entry={entry}
              columns={columns}
              keyExpanded={keyColumnExpanded ?? false}
              maxHistorySize={maxHistorySize}
              agentId={rowActions.agentId}
              isStatic={staticKeyNames.has(entry.key as string)}
              onPromote={rowActions.onPromote}
              onRename={rowActions.onRename}
              onReset={rowActions.onReset}
              onEditMeta={rowActions.onEditMeta}
              readOnly={rowActions.readOnly}
              selection={
                selection
                  ? {
                      selected: selection.isSelected(entry),
                      onToggle: () => selection.onToggle(entry),
                    }
                  : undefined
              }
              rowDetail={
                rowExpansion
                  ? {
                      expandable: rowExpansion.isExpandable(entry),
                      expanded: rowExpansion.isExpanded(entry),
                      onToggle: () => rowExpansion.onToggle(entry),
                      content: rowExpansion.renderDetail(entry),
                    }
                  : undefined
              }
              renderCellExtra={renderCellExtra}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
