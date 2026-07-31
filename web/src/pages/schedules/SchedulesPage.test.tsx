// SPEC-SCHEDULE-VIEW-001 M5 — SchedulesPage 셸.
// 2탭(관리/실행 로그) 전환 + 로그 탭(ScheduleLogTab) 렌더. 각 탭 본문은 별도 테스트
// (ScheduleManagementTab.test / ScheduleLogTab.test)에서 검증하므로 여기서는 스텁으로 대체한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('./ScheduleManagementTab', () => ({
  default: () => <div data-testid="manage-tab-stub">manage</div>,
}));
vi.mock('./ScheduleLogTab', () => ({
  default: () => <div data-testid="log-tab-stub">logs</div>,
}));

import SchedulesPage from './SchedulesPage';

describe('SchedulesPage — 2탭 셸', () => {
  it('기본은 관리 탭이며, 로그 탭 클릭 시 로그 탭을 렌더한다', () => {
    render(<SchedulesPage />);

    // 기본: 관리 탭.
    expect(screen.getByTestId('manage-tab-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('log-tab-stub')).not.toBeInTheDocument();

    // 로그 탭 전환.
    fireEvent.click(screen.getByTestId('schedules-tab-logs'));
    expect(screen.getByTestId('log-tab-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('manage-tab-stub')).not.toBeInTheDocument();

    // 관리 탭 복귀.
    fireEvent.click(screen.getByTestId('schedules-tab-manage'));
    expect(screen.getByTestId('manage-tab-stub')).toBeInTheDocument();
  });
});
