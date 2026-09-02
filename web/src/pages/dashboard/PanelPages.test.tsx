// 패널 생성/설정 딥링크 페이지 라우팅 스모크 테스트.
//
// AddPanelDialog / PanelSettingsDialog 본문(위저드 step, 타입별 섹션)은 각 전용
// 테스트가 검증하므로 여기서는 stub 으로 대체하고, 페이지 래퍼의 라우팅 배선만 검증한다:
//   - /panels/new           → AddPanelDialog(open) 렌더, 닫기 시 대시보드로 이동
//   - /panels/:panelId/settings → panelId 주입 + 존재 검사(부재 시 리다이렉트), 닫기 시 이동

import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// 활성 대시보드 패널 목록을 테스트에서 제어하기 위한 hoisted 상태.
const h = vi.hoisted(() => ({ panels: [] as { id: string }[] }));

// uiStore mock — 셀렉터에 최소 상태(activeDashboardId + dashboardPages)를 주입한다.
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({
      activeDashboardId: 'd1',
      dashboardPages: [{ id: 'd1', panels: h.panels }],
    }),
}));

// 무거운 다이얼로그 본문 stub — 주입된 props 만 노출한다.
vi.mock('./AddPanelDialog', () => ({
  default: ({ open, onClose }: { open: boolean; onClose: () => void }) => (
    <div>
      <span data-testid="create-stub">create:{String(open)}</span>
      <button type="button" onClick={onClose}>
        close-create
      </button>
    </div>
  ),
}));
vi.mock('./PanelSettingsDialog', () => ({
  default: ({ panelId, onClose }: { panelId: string; onClose: () => void }) => (
    <div>
      <span data-testid="settings-stub">settings:{panelId}</span>
      <button type="button" onClick={onClose}>
        close-settings
      </button>
    </div>
  ),
}));

import PanelCreatePage from './PanelCreatePage';
import PanelSettingsPage from './PanelSettingsPage';

/** 대시보드 라우트 마커(리다이렉트/네비게이트 도착지 확인용). */
function DashboardMarker() {
  return <div data-testid="dashboard">dashboard</div>;
}

function renderAt(initial: string) {
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <Routes>
        <Route path="/" element={<DashboardMarker />} />
        <Route path="/panels/new" element={<PanelCreatePage />} />
        <Route path="/panels/:panelId/settings" element={<PanelSettingsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('PanelCreatePage (/panels/new)', () => {
  beforeEach(() => {
    h.panels = [];
  });

  it('AddPanelDialog 를 open 상태로 렌더한다', () => {
    renderAt('/panels/new');
    expect(screen.getByTestId('create-stub')).toHaveTextContent('create:true');
  });

  it('닫기(onClose) 시 대시보드로 이동한다', () => {
    renderAt('/panels/new');
    fireEvent.click(screen.getByText('close-create'));
    expect(screen.getByTestId('dashboard')).toBeInTheDocument();
    expect(screen.queryByTestId('create-stub')).toBeNull();
  });
});

describe('PanelSettingsPage (/panels/:panelId/settings)', () => {
  beforeEach(() => {
    h.panels = [{ id: 'p1' }];
  });

  it('패널 존재 시 설정 본문을 렌더하고 panelId 를 주입한다', () => {
    renderAt('/panels/p1/settings');
    expect(screen.getByTestId('settings-stub')).toHaveTextContent('settings:p1');
  });

  it('패널 부재 시 대시보드로 리다이렉트한다', () => {
    h.panels = [];
    renderAt('/panels/missing/settings');
    expect(screen.getByTestId('dashboard')).toBeInTheDocument();
    expect(screen.queryByTestId('settings-stub')).toBeNull();
  });

  it('닫기(onClose) 시 대시보드로 이동한다', () => {
    renderAt('/panels/p1/settings');
    fireEvent.click(screen.getByText('close-settings'));
    expect(screen.getByTestId('dashboard')).toBeInTheDocument();
    expect(screen.queryByTestId('settings-stub')).toBeNull();
  });
});
