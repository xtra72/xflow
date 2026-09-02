// TablePagination — 공용 페이지네이션 컨트롤 단위 테스트.
//
// 범위:
//   - 페이지 크기 옵션(10/25/50/100) 렌더 + 변경 콜백.
//   - "N-M / 총 T" 범위 라벨이 page/pageSize/totalItems 로부터 정확히 계산.
//   - 이전/다음 버튼 disabled 경계 + clamp 된 페이지 이동 콜백.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import TablePagination from './TablePagination';

function renderPagination(props: Partial<React.ComponentProps<typeof TablePagination>> = {}) {
  const onPageChange = vi.fn();
  const onPageSizeChange = vi.fn();
  render(
    <I18nProvider>
      <TablePagination
        page={props.page ?? 1}
        pageSize={props.pageSize ?? 10}
        totalItems={props.totalItems ?? 0}
        onPageChange={props.onPageChange ?? onPageChange}
        onPageSizeChange={props.onPageSizeChange ?? onPageSizeChange}
        pageSizeOptions={props.pageSizeOptions}
      />
    </I18nProvider>,
  );
  return { onPageChange, onPageSizeChange };
}

describe('TablePagination', () => {
  it('기본 페이지 크기 옵션은 10/25/50/100 이다', () => {
    renderPagination({ totalItems: 12 });
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).toEqual(['10', '25', '50', '100']);
  });

  it('페이지 크기 변경 시 onPageSizeChange 를 숫자로 호출한다', () => {
    const { onPageSizeChange } = renderPagination({ totalItems: 30 });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '25' } });
    expect(onPageSizeChange).toHaveBeenCalledWith(25);
  });

  it('범위 라벨은 현재 페이지 슬라이스 경계를 반영한다(2페이지, pageSize=10, total=23 → 11-20 / 23)', () => {
    renderPagination({ page: 2, pageSize: 10, totalItems: 23 });
    expect(screen.getByText(/11-20/)).toBeTruthy();
    expect(screen.getByText(/23/)).toBeTruthy();
    // 페이지 표시: 2 / 3
    expect(screen.getByText('2 / 3')).toBeTruthy();
  });

  it('첫 페이지에서 이전 버튼은 비활성, 다음은 활성', () => {
    const { onPageChange } = renderPagination({ page: 1, pageSize: 10, totalItems: 23 });
    const prev = screen.getByLabelText('이전 페이지');
    const next = screen.getByLabelText('다음 페이지');
    expect(prev).toBeDisabled();
    expect(next).toBeEnabled();
    fireEvent.click(next);
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it('마지막 페이지에서 다음 버튼은 비활성, 이전은 clamp 된 페이지로 이동', () => {
    const { onPageChange } = renderPagination({ page: 3, pageSize: 10, totalItems: 23 });
    const prev = screen.getByLabelText('이전 페이지');
    const next = screen.getByLabelText('다음 페이지');
    expect(next).toBeDisabled();
    fireEvent.click(prev);
    expect(onPageChange).toHaveBeenCalledWith(2);
  });
});
