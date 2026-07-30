// XsfmLinesTab 테스트 (SPEC-XSFM-LINE-001 Module 6, M6 / AC-7.1, AC-1.x).
//
// 라인 목록(코드+이름+정렬) 표시, 라인 추가(add_line, 코드 포맷 힌트·검증), 라인 삭제
// (remove_line, ErrLineInUse 친화적 메시지)를 검증한다. XsfmGroupsTab.test.tsx 패턴 미러.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';

import type { Line } from '@/hooks/useLine';

// ---- mocks ----

const linesMock = vi.hoisted(() => ({ current: [] as Line[] }));
const mutations = vi.hoisted(() => ({
  add: vi.fn(),
  remove: vi.fn(),
}));

vi.mock('@/hooks/useLine', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useLine')>();
  return {
    ...actual, // isValidLineCode / isLineInUseError / LINE_CODE_PATTERN 는 실제 구현 사용.
    useLines: () => ({ data: linesMock.current, isLoading: false }),
    useAddLine: () => ({ mutate: mutations.add, isPending: false }),
    useRemoveLine: () => ({ mutate: mutations.remove, isPending: false }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) => selector({ addNotification }),
}));

import XsfmLinesTab from './XsfmLinesTab';

function line(code: string, name = code, order = 0): Line {
  return { code, name, order };
}

beforeEach(() => {
  mutations.add.mockReset();
  mutations.remove.mockReset();
  addNotification.mockReset();
  linesMock.current = [];
});

describe('XsfmLinesTab', () => {
  it('라인이 없으면 빈 상태를 표시한다 (AC-1.5 빈 라인 레지스트리)', () => {
    render(<XsfmLinesTab agentId="a1" />);
    expect(screen.getByText('agents.detail.lines.noLines')).toBeInTheDocument();
  });

  it('라인 목록을 코드 + 이름 + 정렬과 함께 표시한다 (AC-7.1, AC-1.1)', () => {
    linesMock.current = [line('line_1', '1호선', 1), line('line_2', '2호선', 2)];
    render(<XsfmLinesTab agentId="a1" />);

    expect(screen.getByTestId('line-list')).toBeInTheDocument();
    const item = screen.getByTestId('line-item-line_2');
    expect(within(item).getByText('2호선')).toBeInTheDocument();
    expect(within(item).getByText('line_2')).toBeInTheDocument();
  });

  it('라인 추가: add_line 을 { code, name, order } 로 호출한다 (AC-7.1)', () => {
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByText('agents.detail.lines.addLine'));
    fireEvent.change(screen.getByTestId('line-code-input'), { target: { value: 'line_2' } });
    fireEvent.change(screen.getByTestId('line-name-input'), { target: { value: '2호선' } });
    fireEvent.change(screen.getByTestId('line-order-input'), { target: { value: '2' } });
    fireEvent.click(screen.getByTestId('line-form-submit'));

    expect(mutations.add).toHaveBeenCalledTimes(1);
    expect(mutations.add).toHaveBeenCalledWith({ code: 'line_2', name: '2호선', order: 2 }, expect.anything());
  });

  it('코드 포맷 위반 시 제출 버튼이 비활성화되고 힌트가 붉게 표시된다 (AC-1.10, RD-6)', () => {
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByText('agents.detail.lines.addLine'));
    // 대문자/공백 포함 → 포맷 위반.
    fireEvent.change(screen.getByTestId('line-code-input'), { target: { value: 'Line 2' } });
    fireEvent.change(screen.getByTestId('line-name-input'), { target: { value: '2호선' } });

    expect(screen.getByTestId('line-form-submit')).toBeDisabled();
    expect(screen.getByTestId('line-code-hint').className).toContain('text-red-500');
    expect(mutations.add).not.toHaveBeenCalled();
  });

  it('유효 코드면 힌트가 붉지 않고 제출이 활성화된다', () => {
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByText('agents.detail.lines.addLine'));
    fireEvent.change(screen.getByTestId('line-code-input'), { target: { value: 'line_2' } });
    fireEvent.change(screen.getByTestId('line-name-input'), { target: { value: '2호선' } });

    expect(screen.getByTestId('line-code-hint').className).not.toContain('text-red-500');
    expect(screen.getByTestId('line-form-submit')).not.toBeDisabled();
  });

  it('라인 삭제: 확인 후 remove_line 을 code 로 호출한다 (AC-1.7)', () => {
    linesMock.current = [line('line_9', '9호선', 9)];
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByTestId('line-remove-line_9'));
    fireEvent.click(screen.getByRole('button', { name: 'common.confirm' }));

    expect(mutations.remove).toHaveBeenCalledTimes(1);
    expect(mutations.remove).toHaveBeenCalledWith('line_9', expect.anything());
  });

  it('참조 역사 존재로 삭제가 거부되면(ErrLineInUse) 친화적 메시지를 노출한다 (AC-1.8, RD-5)', () => {
    // remove.mutate 가 onError 콜백을 ErrLineInUse 메시지로 호출하도록 스텁.
    mutations.remove.mockImplementation(
      (_code: string, opts: { onError?: (e: unknown) => void }) => {
        opts.onError?.(new Error('xsfm: line is in use (referenced by one or more stations)'));
      },
    );
    linesMock.current = [line('line_2', '2호선', 2)];
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByTestId('line-remove-line_2'));
    fireEvent.click(screen.getByRole('button', { name: 'common.confirm' }));

    expect(addNotification).toHaveBeenCalledWith({
      type: 'error',
      message: 'agents.detail.lines.inUseError',
    });
  });

  it('편집 모드에서는 코드 입력이 잠기고 힌트가 미노출된다 (upsert 갱신)', () => {
    linesMock.current = [line('line_2', '2호선', 2)];
    render(<XsfmLinesTab agentId="a1" />);

    fireEvent.click(screen.getByTestId('line-edit-line_2'));
    expect(screen.getByTestId('line-code-input')).toBeDisabled();
    expect(screen.queryByTestId('line-code-hint')).toBeNull();
  });
});
