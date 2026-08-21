// StoreEntryTable — 공용 Store 테이블 단위 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T5)
//
// 에이전트 상세 경로(특성화 테스트)로 덮이지 않는 컨텍스트 주입 분기를 직접 검증한다:
//   - 히스토리 확장(maxHistorySize > 0 → 행 클릭 → get_history exec → 히스토리 렌더).
//   - selection 주입 시 선행 체크박스 컬럼 렌더 + 토글.
//   - renderCellExtra 오버라이드(노드 반환 시 대체, undefined 반환 시 기본 셀 유지).
//   - readOnly 시 액션 숨김 + 행 비클릭.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execMutate = vi.hoisted(() =>
  vi.fn(
    (
      _vars: unknown,
      opts?: { onSuccess?: (res: unknown) => void },
    ) => {
      opts?.onSuccess?.({
        history: [{ value: 42, timestamp: new Date().toISOString() }],
      });
    },
  ),
);

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('@/hooks/useAgent', () => ({
  useExecAgent: () => ({ isPending: false, mutate: execMutate }),
}));

import { StoreEntryTable } from './StoreEntryTable';
import { defaultVisibleColumns, renderedColumns } from './storeColumns';

const entries: Record<string, unknown>[] = [
  { key: 'k-static', value: 10, namespace: 'default', field: 'temperature', updated_at: new Date().toISOString() },
  { key: 'k-dynamic', value: 20, namespace: 'default', field: 'unknown', updated_at: new Date().toISOString() },
];

const columns = renderedColumns(
  { showTagsColumn: false, hasHistory: true },
  defaultVisibleColumns(),
);

const baseRowActions = {
  agentId: 'a1',
  onPromote: vi.fn(),
  onRename: vi.fn(),
  onReset: vi.fn(),
  onEditMeta: vi.fn(),
  readOnly: false,
};

function renderTable(overrides: Partial<React.ComponentProps<typeof StoreEntryTable>> = {}) {
  return render(
    <StoreEntryTable
      entries={entries}
      columns={columns}
      sort={{ column: null, direction: 'asc' }}
      onSort={() => {}}
      columnFilters={{}}
      onColumnFilterChange={() => {}}
      uniqueValuesByColumn={new Map()}
      maxHistorySize={5}
      staticKeyNames={new Set(['k-static'])}
      rowActions={baseRowActions}
      {...overrides}
    />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('StoreEntryTable — 히스토리 확장', () => {
  it('행 클릭 시 get_history exec 를 호출하고 히스토리를 렌더한다', () => {
    renderTable();
    const table = screen.getByRole('table');
    const bodyRows = within(table)
      .getAllByRole('row')
      .filter((r) => within(r).queryAllByRole('cell').length > 0);
    fireEvent.click(bodyRows[0]!);

    expect(execMutate).toHaveBeenCalledTimes(1);
    // onSuccess 로 주입된 히스토리 값(42)이 확장 행에 렌더된다.
    expect(screen.getByText('42')).toBeInTheDocument();
  });
});

describe('StoreEntryTable — selection 주입', () => {
  it('선행 체크박스가 렌더되고 토글 시 onToggle 이 호출된다', () => {
    const onToggle = vi.fn();
    renderTable({
      selection: { isSelected: (e) => e.key === 'k-static', onToggle },
    });
    const checkboxes = screen.getAllByLabelText('agents.detail.store.selectRowAriaLabel');
    expect(checkboxes).toHaveLength(2);
    // k-static 은 선택된 상태로 렌더된다.
    expect((checkboxes[0] as HTMLInputElement).checked).toBe(true);
    fireEvent.click(checkboxes[1]!);
    expect(onToggle).toHaveBeenCalledWith(entries[1]);
  });
});

describe('StoreEntryTable — renderCellExtra 오버라이드', () => {
  it('노드를 반환하면 셀을 대체하고 undefined 면 기본 셀을 유지한다', () => {
    renderTable({
      renderCellExtra: (col, entry) =>
        col === 'updated' ? <span>ALIAS-{entry.key as string}</span> : undefined,
    });
    // 오버라이드된 updated 셀.
    expect(screen.getByText('ALIAS-k-static')).toBeInTheDocument();
    // 오버라이드하지 않은 namespace 셀은 기본 렌더 유지.
    expect(screen.getAllByText('default').length).toBeGreaterThanOrEqual(1);
  });
});

describe('StoreEntryTable — 액션 핸들러', () => {
  it('동적 키 행의 액션 버튼이 rowActions 핸들러를 호출한다', () => {
    renderTable();
    // 동적 키(k-dynamic)는 편집/변환/이름변경/초기화 4버튼을 노출한다.
    fireEvent.click(screen.getAllByLabelText('agents.detail.store.promoteAriaLabel')[0]!);
    expect(baseRowActions.onPromote).toHaveBeenCalledWith('k-dynamic');
    fireEvent.click(screen.getAllByLabelText('agents.detail.store.renameAriaLabel')[0]!);
    expect(baseRowActions.onRename).toHaveBeenCalledWith('k-dynamic');
    // 초기화/편집은 정적/동적 모두 노출 → 첫 행(k-static) 기준.
    fireEvent.click(screen.getAllByLabelText('agents.detail.store.resetAriaLabel')[0]!);
    expect(baseRowActions.onReset).toHaveBeenCalledWith('k-static');
    fireEvent.click(screen.getAllByLabelText('agents.detail.store.editMetaAriaLabel')[0]!);
    expect(baseRowActions.onEditMeta).toHaveBeenCalledWith('k-static');
  });
});

describe('StoreEntryTable — 긴 값 셀 확장', () => {
  it('60자 초과 값은 축약되고 클릭 시 전체 값으로 확장된다', () => {
    const longVal = 'x'.repeat(80);
    render(
      <StoreEntryTable
        entries={[{ key: 'k1', value: longVal, namespace: 'default', field: 'm', updated_at: new Date().toISOString() }]}
        columns={columns}
        sort={{ column: null, direction: 'asc' }}
        onSort={() => {}}
        columnFilters={{}}
        onColumnFilterChange={() => {}}
        uniqueValuesByColumn={new Map()}
        maxHistorySize={0}
        staticKeyNames={new Set()}
        rowActions={baseRowActions}
      />,
    );
    const truncated = screen.getByText(`${longVal.slice(0, 60)}...`);
    fireEvent.click(truncated);
    expect(screen.getByText(longVal)).toBeInTheDocument();
  });
});

describe('StoreEntryTable — readOnly', () => {
  it('readOnly 면 액션 버튼을 숨기고 행을 비클릭 처리한다', () => {
    renderTable({ rowActions: { ...baseRowActions, readOnly: true } });
    // 편집/초기화 액션 버튼이 노출되지 않는다.
    expect(
      screen.queryByLabelText('agents.detail.store.editMetaAriaLabel'),
    ).not.toBeInTheDocument();
    // readOnly 면 히스토리 확장(exec)이 비활성 → 행 클릭해도 exec 호출 없음.
    const table = screen.getByRole('table');
    const bodyRows = within(table)
      .getAllByRole('row')
      .filter((r) => within(r).queryAllByRole('cell').length > 0);
    fireEvent.click(bodyRows[0]!);
    expect(execMutate).not.toHaveBeenCalled();
  });
});
