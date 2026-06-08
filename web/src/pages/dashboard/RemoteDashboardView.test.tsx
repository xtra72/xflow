// RemoteDashboardView 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L10/L11/L12).
//
// 검증:
//   - 원격 config 를 READ-ONLY 로 로드한다(useDashboardConfigTarget). 로컬 동기화
//     (useDashboardSync/PUT)를 사용하지 않는다 — 모듈 임포트 부재로 보장.
//   - 노드 ready 아닐 때(게이팅) 안내를 표시하고 config 를 fetch 하지 않는다.
//   - 에러(503 등) 시 editError 메시지를 표시한다.
//   - config 의 패널들을 READ-ONLY 그리드로 렌더한다(편집 컨트롤 없음).

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const useDashboardConfigTargetMock = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDashboardConfigTarget', () => ({
  useDashboardConfigTarget: useDashboardConfigTargetMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: useTargetGatingMock,
}));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));
// GridLayout + 패널 렌더러는 스텁(그리드 셀 수만 검증).
vi.mock('react-grid-layout', () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="grid">{children}</div>
  ),
}));
vi.mock('./renderDashboardPanel', () => ({
  renderDashboardPanel: (panel: { id: string; type: string }) => (
    <div data-testid={`panel-${panel.id}`} data-type={panel.type} />
  ),
}));

import RemoteDashboardView from './RemoteDashboardView';

const TARGET = { type: 'remote' as const, instanceId: 'node-1' };

function gating(nodeReady: boolean) {
  return {
    isRemote: true,
    nodeReady,
    nodeLabel: 'host-a',
    canControl: () => nodeReady,
  };
}

beforeEach(() => {
  useDashboardConfigTargetMock.mockReset();
  useTargetGatingMock.mockReset();
});

describe('RemoteDashboardView', () => {
  it('노드 ready 아닐 때 게이팅 안내를 표시한다', () => {
    useTargetGatingMock.mockReturnValue(gating(false));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    expect(screen.getByTestId('remote-dashboard-gated')).toBeInTheDocument();
  });

  it('에러(503) 시 editError 메시지를 표시한다', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: undefined,
      isLoading: false,
      error: { status: 503 },
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    expect(screen.getByTestId('remote-dashboard-error')).toBeInTheDocument();
    expect(screen.getByText('remote.edit.errorUnavailable')).toBeInTheDocument();
  });

  it('payload 가 null(미설정, 에러 없음)이면 에러가 아닌 빈 상태를 표시한다', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    // 빨간 에러 화면이 아니라 친절한 빈 상태(제목 + 힌트)를 보여준다.
    expect(screen.queryByTestId('remote-dashboard-error')).not.toBeInTheDocument();
    expect(screen.getByTestId('remote-dashboard-empty')).toBeInTheDocument();
    expect(screen.getByText('remote.remoteDashboard.empty')).toBeInTheDocument();
    expect(screen.getByText('remote.remoteDashboard.emptyHint')).toBeInTheDocument();
  });

  it('미설정이 404 에러로 표면화되어도(방어적) 빈 상태로 취급한다', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: undefined,
      isLoading: false,
      error: { status: 404 },
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    expect(screen.queryByTestId('remote-dashboard-error')).not.toBeInTheDocument();
    expect(screen.getByTestId('remote-dashboard-empty')).toBeInTheDocument();
  });

  it('config 의 패널을 READ-ONLY 그리드로 렌더한다(편집 컨트롤 없음)', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: {
        dashboardPages: [
          {
            id: 'p1',
            name: 'P1',
            isDefault: true,
            panels: [{ id: 'flows-1', type: 'flows', title: 'F', config: {} }],
            layout: [{ i: 'flows-1', x: 0, y: 0, w: 4, h: 3 }],
          },
        ],
        activeDashboardId: 'p1',
        dashboardGridCols: 10,
        dashboardShowGridLines: false,
        dashboardRefreshInterval: 5,
        deviceGridLayout: {},
      },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    expect(screen.getByTestId('panel-flows-1')).toBeInTheDocument();
    // 읽기 전용 표식.
    expect(screen.getByText('remote.remoteDashboard.readOnly')).toBeInTheDocument();
    // 편집 진입/저장 등 로컬 편집 컨트롤이 없다(편집 모드 버튼 부재).
    expect(screen.queryByText('패널 추가')).not.toBeInTheDocument();
  });

  it('READ-ONLY config 취득에 useDashboardConfigTarget 을 scope 와 함께 사용한다', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    useDashboardConfigTargetMock.mockReturnValue({
      payload: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(<RemoteDashboardView target={TARGET} />);
    // 기본 스코프 'shared' + nodeReady=true 로 호출(게이팅 enabled).
    expect(useDashboardConfigTargetMock).toHaveBeenCalledWith(TARGET, 'shared', true);
  });

  it('스코프 전환(shared→mine) 시 useDashboardConfigTarget 을 새 스코프로 호출한다', () => {
    useTargetGatingMock.mockReturnValue(gating(true));
    // 공유 스코프엔 패널이 있고, 내 스코프엔 없음(빈 상태)을 스코프별로 반환한다.
    useDashboardConfigTargetMock.mockImplementation(
      (_target: unknown, scope: string) => {
        if (scope === 'shared') {
          return {
            payload: {
              dashboardPages: [
                {
                  id: 'p1',
                  name: 'P1',
                  isDefault: true,
                  panels: [{ id: 'flows-1', type: 'flows', title: 'F', config: {} }],
                  layout: [{ i: 'flows-1', x: 0, y: 0, w: 4, h: 3 }],
                },
              ],
              activeDashboardId: 'p1',
              dashboardGridCols: 10,
              dashboardShowGridLines: false,
              dashboardRefreshInterval: 5,
              deviceGridLayout: {},
            },
            isLoading: false,
            error: null,
            refetch: vi.fn(),
          };
        }
        return { payload: undefined, isLoading: false, error: null, refetch: vi.fn() };
      },
    );

    render(<RemoteDashboardView target={TARGET} />);
    // 초기(shared): 패널 렌더.
    expect(screen.getByTestId('panel-flows-1')).toBeInTheDocument();

    // 'mine' 탭 클릭 → 스코프 전환.
    fireEvent.click(screen.getByText('remote.remoteDashboard.mine'));

    // 훅이 'mine' 스코프로 호출되고, 빈 상태로 전환된다(공유 패널은 사라짐).
    expect(useDashboardConfigTargetMock).toHaveBeenCalledWith(TARGET, 'mine', true);
    expect(screen.queryByTestId('panel-flows-1')).not.toBeInTheDocument();
    expect(screen.getByTestId('remote-dashboard-empty')).toBeInTheDocument();
  });
});
