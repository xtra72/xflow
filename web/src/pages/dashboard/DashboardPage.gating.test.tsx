// DashboardPage 대시보드 단위 게이팅 테스트 (SPEC-DASHBOARD-004 M6, AC-03 / AC-16).
//
// 구속 규칙(spec.md §2.11 S2, SPEC-AUTH-006 §4.2): 액션 컨트롤은 **숨기지 않고
// 비활성**한다. 그래서 이 테스트는 전부 "존재 + disabled" 를 단언하며 부재
// (queryBy...toBeNull)를 단언하지 않는다 — 부재 단언은 숨김 구현도 통과시켜
// 규칙을 지키지 못한다.
//
// 컨트롤 ↔ 플래그 대응은 서버의 인가 분기와 1:1 이며 각각 독립 검증한다:
//   이름 변경 → can_edit / 기본 지정 → can_grant / 삭제 → can_delete
//   생성      → 전역 dashboard.create (대시보드 단위 아님, AC-03)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Dashboard } from '@/types/dashboard';

// 데이터 훅 스텁 — 결정적 반환값.
vi.mock('@/hooks', () => ({
  useFlows: () => ({ data: { data: [] }, isLoading: false, error: null }),
  useWebSocket: () => ({ state: 'connected', client: null }),
}));
vi.mock('@/services/api/monitorService', () => ({
  getMetrics: vi.fn().mockResolvedValue(null),
}));
vi.mock('react-grid-layout', () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="grid">{children}</div>
  ),
}));
vi.mock('./renderDashboardPanel', () => ({
  renderDashboardPanel: () => null,
}));

// i18n: 실제 ko 번역을 해석한다 — 사유 툴팁·완화책 문구를 키가 아니라 실제
// 사용자 문자열로 단언해야 AC-16 의 "취지" 를 검증할 수 있다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return { useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }) };
});

// 대시보드 API — 요청 발생 여부와 인자를 관찰한다.
const createDashboardMock = vi.hoisted(() => vi.fn());
const patchDashboardMock = vi.hoisted(() => vi.fn());
const deleteDashboardMock = vi.hoisted(() => vi.fn());
const listDashboardsMock = vi.hoisted(() => vi.fn());
const ForbiddenError = vi.hoisted(() => class DashboardForbiddenError extends Error {});
vi.mock('@/services/api/dashboardService', () => ({
  createDashboard: createDashboardMock,
  patchDashboard: patchDashboardMock,
  deleteDashboard: deleteDashboardMock,
  listDashboards: listDashboardsMock,
  DashboardForbiddenError: ForbiddenError,
}));

// authStore — usePermission 이 읽는 최소 스냅샷. 테스트별로 permissions 를 바꾼다.
const authState = {
  authEnabled: true,
  permissions: new Set<string>(),
  permissionStatus: 'loaded' as const,
};
vi.mock('@/stores/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (state: typeof authState) => unknown) =>
      selector ? selector(authState) : authState,
    { getState: () => authState },
  ),
}));

import { useUIStore } from '@/stores/uiStore';

import DashboardPage from './DashboardPage';

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '운영 대시보드',
    owner: 'vie',
    visibility: 'private',
    is_default: true,
    sort_order: 0,
    version: 3,
    created_at: 0,
    updated_at: 0,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

/** 목록 축을 주입하고 활성 대시보드를 지정한다. */
function seedDashboards(list: Dashboard[], activeUid = list[0]?.uid ?? '') {
  useUIStore.getState().setDashboards(list);
  useUIStore.getState().setActiveDashboard(activeUid);
}

function setPermissions(keys: string[]) {
  authState.permissions = new Set(keys);
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={client}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  }
  return render(<DashboardPage />, { wrapper: Wrapper });
}

/** 대시보드 셀렉터 드롭다운을 연다 — 행별 컨트롤은 그 안에 있다. */
function openDashboardDropdown() {
  const buttons = screen.getAllByRole('button');
  const selector = buttons.find((b) => b.textContent?.includes('운영 대시보드'));
  fireEvent.click(selector!);
}

beforeEach(() => {
  createDashboardMock.mockReset();
  patchDashboardMock.mockReset();
  deleteDashboardMock.mockReset();
  listDashboardsMock.mockReset();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  setPermissions(['dashboard.read', 'dashboard.create', 'dashboard.update', 'dashboard.delete']);
  useUIStore.getState().setDashboardEditMode(false);
  seedDashboards([makeDashboard()]);
});

describe('DashboardPage — 편집 게이팅 (can_edit)', () => {
  it('can_edit=false 면 레이아웃 편집 버튼이 렌더되되 비활성이다(숨기지 않는다)', () => {
    seedDashboards([makeDashboard({ can_edit: false })]);
    renderPage();

    const editButton = screen.getByTestId('dashboard-edit-mode');
    // 존재한다 — 숨김이 아니라 비활성이어야 한다.
    expect(editButton).toBeInTheDocument();
    expect(editButton).toBeDisabled();
    expect(editButton).toHaveAttribute('aria-disabled', 'true');
  });

  it('can_edit=false 면 편집 버튼에 사유 툴팁이 붙는다', () => {
    seedDashboards([makeDashboard({ can_edit: false })]);
    renderPage();

    expect(screen.getByTestId('dashboard-edit-mode')).toHaveAttribute(
      'title',
      '이 대시보드를 편집할 권한이 없습니다',
    );
  });

  it('can_edit=true 면 편집 버튼이 활성이다', () => {
    seedDashboards([makeDashboard({ can_edit: true })]);
    renderPage();

    expect(screen.getByTestId('dashboard-edit-mode')).toBeEnabled();
  });

  it('편집 불가여도 상시 안내 배너를 표시하지 않는다', () => {
    seedDashboards([makeDashboard({ can_edit: false })]);
    renderPage();

    expect(screen.queryByTestId('dashboard-readonly-notice')).toBeNull();
  });

  it('배너가 없어도 편집 버튼 툴팁이 사유를 전달한다', () => {
    // 배너를 걷어낸 뒤 사유 전달 수단이 통째로 사라지지 않았는지 지킨다.
    seedDashboards([makeDashboard({ can_edit: false })]);
    renderPage();

    expect(screen.getByTestId('dashboard-edit-mode')).toHaveAttribute(
      'title',
      '이 대시보드를 편집할 권한이 없습니다',
    );
  });
});

describe('DashboardPage — 컨트롤별 플래그 독립 검증', () => {
  it('이름 변경은 can_edit 만 따른다(can_grant / can_delete 는 무관)', () => {
    // can_edit 만 false, 나머지는 true.
    seedDashboards([makeDashboard({ can_edit: false, can_grant: true, can_delete: true })]);
    renderPage();
    openDashboardDropdown();

    const rename = screen.getByTestId('dashboard-rename-d1');
    expect(rename).toBeInTheDocument();
    expect(rename).toBeDisabled();
    expect(rename).toHaveAttribute('title', '이 대시보드를 편집할 권한이 없습니다');

    // 같은 상태에서 기본 지정은 can_grant=true 이므로 활성이어야 한다.
    expect(screen.getByTestId('dashboard-set-default-d1')).toBeEnabled();
  });

  it('기본 지정은 can_grant 만 따른다(can_edit 이 true 여도 막힌다)', () => {
    seedDashboards([makeDashboard({ can_edit: true, can_grant: false, can_delete: true })]);
    renderPage();
    openDashboardDropdown();

    const setDefault = screen.getByTestId('dashboard-set-default-d1');
    expect(setDefault).toBeInTheDocument();
    expect(setDefault).toBeDisabled();
    expect(setDefault).toHaveAttribute(
      'title',
      '이 대시보드의 공개 설정을 변경할 권한이 없습니다',
    );

    // 이름 변경은 can_edit=true 이므로 활성이다.
    expect(screen.getByTestId('dashboard-rename-d1')).toBeEnabled();
  });

  it('can_delete 만 false 여도 이름 변경·기본 지정은 활성이다', () => {
    seedDashboards([makeDashboard({ can_edit: true, can_grant: true, can_delete: false })]);
    renderPage();
    openDashboardDropdown();

    expect(screen.getByTestId('dashboard-rename-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-set-default-d1')).toBeEnabled();
  });

  it('행마다 그 대시보드의 판정을 쓴다(활성 대시보드로 목록 전체를 잠그지 않는다)', () => {
    seedDashboards(
      [
        makeDashboard({ uid: 'd1', name: '운영 대시보드', can_edit: false }),
        makeDashboard({ uid: 'd2', name: '내 대시보드', is_default: false, can_edit: true }),
      ],
      'd1',
    );
    renderPage();
    openDashboardDropdown();

    expect(screen.getByTestId('dashboard-rename-d1')).toBeDisabled();
    expect(screen.getByTestId('dashboard-rename-d2')).toBeEnabled();
  });
});

describe('DashboardPage — 생성 게이팅 (AC-03)', () => {
  it('dashboard.create 미보유 시 생성 컨트롤이 렌더되되 비활성 + 사유를 붙인다', () => {
    // viewer: read 만 보유.
    setPermissions(['dashboard.read']);
    seedDashboards([makeDashboard()]);
    renderPage();
    openDashboardDropdown();

    const addButton = screen.getByTestId('dashboard-add');
    expect(addButton).toBeInTheDocument();
    expect(addButton).toBeDisabled();
    expect(addButton).toHaveAttribute('title', '대시보드를 생성할 권한이 없습니다');
  });

  it('dashboard.create 보유 시 생성 컨트롤이 활성이다', () => {
    setPermissions(['dashboard.read', 'dashboard.create']);
    seedDashboards([makeDashboard()]);
    renderPage();
    openDashboardDropdown();

    expect(screen.getByTestId('dashboard-add')).toBeEnabled();
  });

  it('생성은 로컬 스토어가 아니라 서버 POST 를 발생시킨다', async () => {
    createDashboardMock.mockResolvedValue({
      ...makeDashboard({ uid: 'srv-uid', name: '새 대시보드', is_default: false }),
      payload: { panels: [], layout: [] },
    });
    setPermissions(['dashboard.read', 'dashboard.create']);
    seedDashboards([makeDashboard()]);
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-add'));

    expect(createDashboardMock).toHaveBeenCalledTimes(1);
    expect(createDashboardMock).toHaveBeenCalledWith('새 대시보드');

    // 서버가 발급한 uid 가 그대로 활성 대시보드가 된다(클라이언트가 짓지 않는다).
    await waitFor(() => {
      expect(useUIStore.getState().activeDashboardId).toBe('srv-uid');
    });
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toContain('srv-uid');
  });

  it('can_edit=false 라도 생성 판정은 전역 권한을 따른다(대시보드 단위가 아니다)', () => {
    setPermissions(['dashboard.read', 'dashboard.create']);
    seedDashboards([makeDashboard({ can_edit: false })]);
    renderPage();
    openDashboardDropdown();

    // 편집은 막히지만 생성은 열려 있다.
    expect(screen.getByTestId('dashboard-edit-mode')).toBeDisabled();
    expect(screen.getByTestId('dashboard-add')).toBeEnabled();
  });
});

describe('DashboardPage — 컨트롤이 서버에 반영된다 (영속성 회귀 방지)', () => {
  // `name` 과 `is_default` 는 1급 엔티티의 컬럼이 되었고 본문 PUT 은 `{ payload }`
  // 만 보낸다. 이 컨트롤들이 로컬 스토어만 바꾸면 사용자는 새로고침에서 변경을
  // 잃는다 — 게이팅만 하고 저장하지 않는 상태가 회귀다.

  it('이름 변경이 PATCH {name} 을 보낸다', async () => {
    patchDashboardMock.mockResolvedValue({
      ...makeDashboard({ name: '바뀐 이름' }),
      payload: null,
    });

    seedDashboards([makeDashboard({ uid: 'd1', name: '운영 대시보드' })]);
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-rename-d1'));
    const input = screen.getByDisplayValue('운영 대시보드');
    fireEvent.change(input, { target: { value: '바뀐 이름' } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(patchDashboardMock).toHaveBeenCalledWith('d1', { name: '바뀐 이름' });
    });
  });

  it('기본 지정이 PATCH {is_default} 를 보낸다', async () => {
    patchDashboardMock.mockResolvedValue({
      ...makeDashboard({ is_default: true }),
      payload: null,
    });
    seedDashboards([makeDashboard({ uid: 'd1', is_default: false })]);
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-set-default-d1'));

    await waitFor(() => {
      expect(patchDashboardMock).toHaveBeenCalledWith('d1', { is_default: true });
    });
  });

  it('삭제가 DELETE 를 보내고 활성 대시보드였으면 폴백으로 착지한다', async () => {
    deleteDashboardMock.mockResolvedValue(undefined);
    seedDashboards(
      [
        makeDashboard({ uid: 'd1', name: '운영 대시보드', is_default: false }),
        makeDashboard({ uid: 'd2', name: '기본 대시보드', is_default: true }),
      ],
      'd1',
    );
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-delete-d1'));

    await waitFor(() => {
      expect(deleteDashboardMock).toHaveBeenCalledWith('d1');
    });
    // M5 폴백 사슬: is_default 우선.
    await waitFor(() => {
      expect(useUIStore.getState().activeDashboardId).toBe('d2');
    });
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['d2']);
  });

  it('거부된 이름 변경은 화면을 원래대로 되돌린다', async () => {
    patchDashboardMock.mockRejectedValue(new Error('boom'));
    seedDashboards([makeDashboard({ uid: 'd1', name: '운영 대시보드' })]);
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-rename-d1'));
    const input = screen.getByDisplayValue('운영 대시보드');
    fireEvent.change(input, { target: { value: '바뀐 이름' } });
    fireEvent.blur(input);

    await waitFor(() => expect(patchDashboardMock).toHaveBeenCalled());
    // 서버가 거부한 이름이 스토어에 남아 있으면 안 된다.
    await waitFor(() => {
      expect(useUIStore.getState().dashboards[0]?.name).toBe('운영 대시보드');
    });
    expect(
      useUIStore.getState().notifications.some((n) => n.type === 'error'),
    ).toBe(true);
  });

  it('삭제 확인을 취소하면 DELETE 를 보내지 않는다', () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    seedDashboards([makeDashboard({ uid: 'd1' }), makeDashboard({ uid: 'd2' })], 'd1');
    renderPage();
    openDashboardDropdown();

    fireEvent.click(screen.getByTestId('dashboard-delete-d1'));

    expect(deleteDashboardMock).not.toHaveBeenCalled();
  });
});

// 설정(편집) 모드의 대시보드 관리 표면 (SPEC-DASHBOARD-004, M7 재작업).
//
// 별도 '대시보드 관리' 화면이 사라졌으므로, 관리 진입점이 실제로 **편집 모드 안에**
// 있는지는 DashboardPage 수준에서 확인해야 한다. 컨트롤 단위 계약은
// DashboardSettingsSelector.test.tsx 가 맡는다 — 여기서는 배선만 고정한다.
describe('DashboardPage — 설정 모드 대시보드 셀렉터', () => {
  /** 편집 모드로 진입한 뒤 셀렉터 목록을 연다. */
  function openSettingsList() {
    useUIStore.getState().setDashboardEditMode(true);
    renderPage();
    fireEvent.click(screen.getByTestId('dashboard-settings-selector-toggle'));
  }

  it('편집 모드에서 셀렉터가 렌더되고 편집 모드 뱃지가 남는다', () => {
    seedDashboards([makeDashboard({ uid: 'd1', name: '운영 대시보드' })]);
    openSettingsList();

    expect(screen.getByTestId('dashboard-settings-list')).toBeInTheDocument();
    expect(screen.getByText('편집 모드')).toBeInTheDocument();
  });

  it('활성이 아닌 대시보드도 목록에 나오고 선택하면 전환된다', () => {
    seedDashboards(
      [
        makeDashboard({ uid: 'd1', name: '운영 대시보드' }),
        makeDashboard({ uid: 'd2', name: '개인 대시보드' }),
      ],
      'd1',
    );
    openSettingsList();

    expect(screen.getByTestId('dashboard-settings-row-d2')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('dashboard-settings-select-d2'));
    expect(useUIStore.getState().activeDashboardId).toBe('d2');
  });

  it('일반 모드에서는 셀렉터가 렌더되지 않는다 (기존 드롭다운이 담당)', () => {
    seedDashboards([makeDashboard({ uid: 'd1' })]);
    useUIStore.getState().setDashboardEditMode(false);
    renderPage();

    expect(
      screen.queryByTestId('dashboard-settings-selector-toggle'),
    ).not.toBeInTheDocument();
  });
});
