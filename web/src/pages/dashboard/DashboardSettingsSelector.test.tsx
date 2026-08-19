// 설정(편집) 모드 대시보드 셀렉터 + 관리 컨트롤 테스트 (SPEC-DASHBOARD-004, M7 재작업).
//
// 삭제된 DashboardAdminPage.test.tsx / DashboardAdminPage.route.test.tsx 가 검증하던
// 축을 새 표면 위로 옮겨온 것이다. 커버리지를 버리지 않고 대상만 바꾼다.
//
// 검증 축:
//   1) 목록 — 볼 수 있는 대시보드를 **전부**(비활성·편집 불가 포함) 싣는가,
//      행 클릭이 활성 대시보드를 바꾸는가.
//   2) 메뉴 축(`nav.dashboard`) — 관리 어포던스를 아예 렌더하지 않는가.
//      메뉴 클래스 판정이므로 여기서는 **부재**를 단언한다(구 AC-10 의 후속 형태).
//   3) 대시보드 단위 인가 — 렌더된 컨트롤을 숨기지 않고 **비활성**하는가.
//      플래그 하나만 내렸을 때 나머지가 살아 있음을 함께 단언해 축의 독립을 고정한다.
//   4) 각 경로가 어떤 요청을 보내는가, 거부되면 되돌리고 사유를 알리는가.
//
// 서비스 계층만 스텁하고 훅(useCreateDashboard / useDashboardMutations)·스토어·i18n 은
// 실제 코드를 돌린다 — i18n 을 실제 Provider 로 두면 누락된 키가 렌더 문자열에서
// 즉시 드러난다.
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.5, §2.7, §2.9, §2.11, §2.12)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { Dashboard } from '@/types/dashboard';

// ---- 서비스 스텁 ----
// 에러 클래스는 실제 구현을 그대로 쓴다 — 훅이 instanceof 로 분기한다.

const createDashboard = vi.hoisted(() => vi.fn());
const patchDashboard = vi.hoisted(() => vi.fn());
const deleteDashboard = vi.hoisted(() => vi.fn());
const listDashboards = vi.hoisted(() => vi.fn());
const getAcl = vi.hoisted(() => vi.fn());
const putAcl = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/dashboardService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/dashboardService')>(
    '@/services/api/dashboardService',
  );
  return {
    ...actual,
    createDashboard,
    patchDashboard,
    deleteDashboard,
    listDashboards,
    getAcl,
    putAcl,
  };
});

// ---- 권한 스텁 ----
// 데이터 권한과 메뉴 권한을 같은 집합에서 판정한다 — 테스트가 키 단위로 조립한다.

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => granted.keys.has(k)),
    canSeeMenu: (key: string) => granted.keys.has(key),
    isPermissionUnavailable: false,
  }),
}));

import { DashboardForbiddenError } from '@/services/api/dashboardService';
import { useUIStore } from '@/stores/uiStore';

import DashboardSettingsSelector from './DashboardSettingsSelector';

function grant(...keys: string[]): void {
  granted.keys = new Set(keys);
}

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '운영 대시보드',
    owner: 'root',
    visibility: 'private',
    is_default: false,
    sort_order: 0,
    version: 3,
    created_at: 1_700_000_000_000,
    updated_at: 1_700_000_000_000,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

/** 목록 축을 주입하고 활성 대시보드를 지정한다. */
function seed(list: Dashboard[], activeUid = list[0]?.uid ?? ''): void {
  useUIStore.getState().setDashboards(list);
  useUIStore.getState().setActiveDashboard(activeUid);
}

function renderSelector(activeName = '운영 대시보드') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <DashboardSettingsSelector activeName={activeName} />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

/** 셀렉터를 열어 목록을 드러낸다. 관리 컨트롤은 전부 이 안에 있다. */
function openList(): void {
  fireEvent.click(screen.getByTestId('dashboard-settings-selector-toggle'));
}

function notificationMessages(): string[] {
  return useUIStore.getState().notifications.map((n) => n.message);
}

beforeEach(() => {
  vi.clearAllMocks();
  grant('nav.dashboard', 'dashboard.create', 'dashboard.read', 'dashboard.update', 'dashboard.delete');
  seed([makeDashboard()]);
  useUIStore.getState().clearNotifications();
  listDashboards.mockResolvedValue([]);
  getAcl.mockResolvedValue([]);
});

describe('DashboardSettingsSelector — 목록 (비활성 대시보드 포함)', () => {
  it('볼 수 있는 대시보드를 전부 싣는다 — 활성이 아닌 것도 뺴지 않는다', () => {
    seed(
      [
        makeDashboard({ uid: 'd1', name: '운영' }),
        makeDashboard({ uid: 'd2', name: '개인' }),
        makeDashboard({ uid: 'd3', name: '창고' }),
      ],
      'd1',
    );
    renderSelector();
    openList();

    expect(screen.getByTestId('dashboard-settings-row-d1')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-settings-row-d2')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-settings-row-d3')).toBeInTheDocument();
  });

  it('편집할 수 없는 대시보드도 행에서 빼지 않는다 (볼 수 있으면 나온다)', () => {
    seed(
      [
        makeDashboard({ uid: 'd1', name: '운영' }),
        makeDashboard({
          uid: 'd2',
          name: '남의 것',
          can_edit: false,
          can_delete: false,
          can_grant: false,
        }),
      ],
      'd1',
    );
    renderSelector();
    openList();

    // 행은 존재하고, 그 행의 컨트롤만 비활성이다.
    expect(screen.getByTestId('dashboard-settings-row-d2')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-settings-rename-d2')).toBeDisabled();
  });

  it('행을 선택하면 활성 대시보드가 그 대시보드로 바뀐다', () => {
    seed(
      [makeDashboard({ uid: 'd1', name: '운영' }), makeDashboard({ uid: 'd2', name: '개인' })],
      'd1',
    );
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-select-d2'));

    expect(useUIStore.getState().activeDashboardId).toBe('d2');
    // 선택하면 목록은 닫힌다.
    expect(screen.queryByTestId('dashboard-settings-list')).not.toBeInTheDocument();
  });

  it('목록이 비면 빈 상태 문구를 표시한다', () => {
    seed([]);
    renderSelector('대시보드');
    openList();
    expect(screen.getByText('접근 가능한 대시보드가 없습니다')).toBeInTheDocument();
  });
});

describe('DashboardSettingsSelector — nav.dashboard 게이팅 (메뉴 축)', () => {
  // 구 AC-10 은 "관리 메뉴 미노출" 이었다. 메뉴가 사라졌으므로 그 후속 형태는
  // "설정 모드 안의 관리 어포던스 미노출" 이다. 메뉴 클래스 판정이므로 부재를
  // 단언한다 — 액션 컨트롤의 "숨기지 말고 비활성" 규칙과는 다른 축이다.
  const MANAGE_TESTIDS = [
    'dashboard-settings-default-d1',
    'dashboard-settings-rename-d1',
    'dashboard-settings-visibility-d1',
    'dashboard-settings-acl-d1',
    'dashboard-settings-delete-d1',
    'dashboard-settings-create',
  ];

  it('nav.dashboard 가 없으면 관리 컨트롤을 아예 렌더하지 않는다', () => {
    grant('dashboard.create', 'dashboard.read', 'dashboard.update', 'dashboard.delete');
    renderSelector();
    openList();

    for (const id of MANAGE_TESTIDS) {
      expect(screen.queryByTestId(id)).not.toBeInTheDocument();
    }
  });

  it('nav.dashboard 가 없어도 대시보드 선택은 계속 가능하다 (보기는 다른 축)', () => {
    grant('dashboard.read');
    seed(
      [makeDashboard({ uid: 'd1', name: '운영' }), makeDashboard({ uid: 'd2', name: '개인' })],
      'd1',
    );
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-select-d2'));
    expect(useUIStore.getState().activeDashboardId).toBe('d2');
  });

  it('nav.dashboard 가 있으면 관리 컨트롤이 전부 렌더된다', () => {
    renderSelector();
    openList();

    for (const id of MANAGE_TESTIDS) {
      expect(screen.getByTestId(id)).toBeInTheDocument();
    }
  });
});

describe('DashboardSettingsSelector — 컨트롤 ↔ 플래그 (숨기지 않고 비활성)', () => {
  it('can_edit=false 는 이름 변경만 잠근다', () => {
    seed([makeDashboard({ can_edit: false })]);
    renderSelector();
    openList();

    const rename = screen.getByTestId('dashboard-settings-rename-d1');
    expect(rename).toBeInTheDocument();
    expect(rename).toBeDisabled();
    expect(rename).toHaveAttribute('title', '이 대시보드를 편집할 권한이 없습니다');

    // 독립 검증 — 다른 축은 살아 있다.
    expect(screen.getByTestId('dashboard-settings-default-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-visibility-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-acl-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-delete-d1')).toBeEnabled();
  });

  it('can_grant=false 는 기본 지정·공개범위·권한 부여만 잠근다', () => {
    seed([makeDashboard({ can_grant: false })]);
    renderSelector();
    openList();

    for (const id of [
      'dashboard-settings-default-d1',
      'dashboard-settings-visibility-d1',
      'dashboard-settings-acl-d1',
    ]) {
      const control = screen.getByTestId(id);
      expect(control).toBeInTheDocument();
      expect(control).toBeDisabled();
      expect(control).toHaveAttribute(
        'title',
        '이 대시보드의 공개 설정을 변경할 권한이 없습니다',
      );
    }

    expect(screen.getByTestId('dashboard-settings-rename-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-delete-d1')).toBeEnabled();
  });

  it('can_delete=false 는 삭제만 잠근다', () => {
    seed([makeDashboard({ can_delete: false })]);
    renderSelector();
    openList();

    const del = screen.getByTestId('dashboard-settings-delete-d1');
    expect(del).toBeInTheDocument();
    expect(del).toBeDisabled();
    expect(del).toHaveAttribute('title', '이 대시보드를 삭제할 권한이 없습니다');

    expect(screen.getByTestId('dashboard-settings-rename-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-default-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-visibility-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-acl-d1')).toBeEnabled();
  });

  it('dashboard.create 가 없으면 생성만 잠근다 (전역 권한 축)', () => {
    grant('nav.dashboard', 'dashboard.read', 'dashboard.update', 'dashboard.delete');
    renderSelector();
    openList();

    const create = screen.getByTestId('dashboard-settings-create');
    expect(create).toBeDisabled();
    expect(create).toHaveAttribute('title', '대시보드를 생성할 권한이 없습니다');
    expect(screen.getByTestId('dashboard-settings-rename-d1')).toBeEnabled();
    expect(screen.getByTestId('dashboard-settings-delete-d1')).toBeEnabled();
  });
});

describe('DashboardSettingsSelector — 요청 계약 (spec.md §2.3)', () => {
  it('이름을 바꾸면 PATCH {name} 을 보낸다', async () => {
    patchDashboard.mockResolvedValue({
      ...makeDashboard({ name: '바뀐 이름' }),
      payload: { panels: [], layout: [] },
    });
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-rename-d1'));
    fireEvent.change(screen.getByLabelText('대시보드 이름 편집'), {
      target: { value: '바뀐 이름' },
    });
    fireEvent.click(screen.getByTestId('dashboard-settings-rename-save-d1'));

    await waitFor(() =>
      expect(patchDashboard).toHaveBeenCalledWith('d1', { name: '바뀐 이름' }),
    );
  });

  it('기본 지정은 PATCH {is_default} 를 보낸다', async () => {
    patchDashboard.mockResolvedValue({
      ...makeDashboard({ is_default: true }),
      payload: { panels: [], layout: [] },
    });
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-default-d1'));

    await waitFor(() =>
      expect(patchDashboard).toHaveBeenCalledWith('d1', { is_default: true }),
    );
  });

  it('공개범위를 바꾸면 PATCH {visibility} 를 보낸다', async () => {
    patchDashboard.mockResolvedValue({
      ...makeDashboard({ visibility: 'acl' }),
      payload: { panels: [], layout: [] },
    });
    renderSelector();
    openList();

    fireEvent.change(screen.getByTestId('dashboard-settings-visibility-d1'), {
      target: { value: 'acl' },
    });

    await waitFor(() =>
      expect(patchDashboard).toHaveBeenCalledWith('d1', { visibility: 'acl' }),
    );
  });

  it('삭제는 확인을 거친 뒤에만 DELETE 를 보낸다', async () => {
    deleteDashboard.mockResolvedValue(undefined);
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-delete-d1'));
    expect(deleteDashboard).not.toHaveBeenCalled();

    const dialog = screen.getByRole('dialog', { name: '대시보드 삭제' });
    fireEvent.click(within(dialog).getByRole('button', { name: '삭제' }));

    await waitFor(() => expect(deleteDashboard).toHaveBeenCalledWith('d1'));
  });

  it('마지막 1장이어도 삭제를 막지 않는다 (spec.md 엣지 케이스)', () => {
    seed([makeDashboard()]);
    renderSelector();
    openList();
    expect(screen.getByTestId('dashboard-settings-delete-d1')).toBeEnabled();
  });
});

describe('DashboardSettingsSelector — 거부 시 되돌리기 + 토스트', () => {
  it('이름 변경이 거부되면 이름을 되돌리고 사유를 알린다', async () => {
    patchDashboard.mockRejectedValue(new Error('boom'));
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-rename-d1'));
    fireEvent.change(screen.getByLabelText('대시보드 이름 편집'), {
      target: { value: '바뀐 이름' },
    });
    fireEvent.click(screen.getByTestId('dashboard-settings-rename-save-d1'));

    await waitFor(() =>
      expect(notificationMessages()).toContain('대시보드 이름을 변경하지 못했습니다'),
    );
    // 서버가 거부한 변경을 화면에 남기지 않는다.
    expect(useUIStore.getState().dashboards[0]?.name).toBe('운영 대시보드');
  });

  it('공개범위 변경이 403 이면 되돌리고 권한 사유를 알린다', async () => {
    patchDashboard.mockRejectedValue(new DashboardForbiddenError());
    listDashboards.mockResolvedValue([makeDashboard()]);
    renderSelector();
    openList();

    fireEvent.change(screen.getByTestId('dashboard-settings-visibility-d1'), {
      target: { value: 'shared' },
    });

    await waitFor(() =>
      expect(notificationMessages()).toContain(
        '이 대시보드의 공개 설정을 변경할 권한이 없습니다',
      ),
    );
    expect(useUIStore.getState().dashboards[0]?.visibility).toBe('private');
    // 403 은 판정이 바뀌었을 수 있으므로 목록을 다시 읽는다.
    await waitFor(() => expect(listDashboards).toHaveBeenCalled());
  });

  it('삭제가 거부되면 목록에서 지우지 않고 사유를 알린다', async () => {
    deleteDashboard.mockRejectedValue(new Error('boom'));
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-delete-d1'));
    const dialog = screen.getByRole('dialog', { name: '대시보드 삭제' });
    fireEvent.click(within(dialog).getByRole('button', { name: '삭제' }));

    await waitFor(() =>
      expect(notificationMessages()).toContain('대시보드를 삭제하지 못했습니다'),
    );
    expect(useUIStore.getState().dashboards).toHaveLength(1);
  });
});

describe('DashboardSettingsSelector — 생성 (spec.md §2.7 E1)', () => {
  it('서버가 발급한 uid 로 목록에 넣고 그 대시보드로 전환한다', async () => {
    createDashboard.mockResolvedValue({
      ...makeDashboard({ uid: 'server-uid-9', name: '새 대시보드' }),
      payload: { panels: [], layout: [] },
    });
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-create'));

    await waitFor(() => expect(createDashboard).toHaveBeenCalledWith('새 대시보드'));
    await waitFor(() =>
      // uid 는 클라이언트가 지어내지 않는다 — 서버 응답 그대로 쓴다.
      expect(useUIStore.getState().activeDashboardId).toBe('server-uid-9'),
    );
    expect(
      useUIStore.getState().dashboards.some((d) => d.uid === 'server-uid-9'),
    ).toBe(true);
  });

  it('생성이 거부되면 사유를 알리고 목록을 늘리지 않는다', async () => {
    createDashboard.mockRejectedValue(new DashboardForbiddenError());
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-create'));

    await waitFor(() =>
      expect(notificationMessages()).toContain('대시보드를 생성할 권한이 없습니다'),
    );
    expect(useUIStore.getState().dashboards).toHaveLength(1);
  });
});

describe('DashboardSettingsSelector — 권한 부여 다이얼로그 (grant 인가)', () => {
  it('can_grant=true 면 다이얼로그를 열고 ACL 을 조회한다', async () => {
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-acl-d1'));

    await waitFor(() => expect(getAcl).toHaveBeenCalledWith('d1'));
    expect(screen.getByTestId('dashboard-acl-panel')).toBeInTheDocument();
  });

  it('can_grant=false 면 버튼이 비활성이고 다이얼로그가 열리지 않는다', () => {
    seed([makeDashboard({ can_grant: false })]);
    renderSelector();
    openList();

    const button = screen.getByTestId('dashboard-settings-acl-d1');
    expect(button).toBeDisabled();
    fireEvent.click(button);

    expect(screen.queryByTestId('dashboard-acl-panel')).not.toBeInTheDocument();
    expect(getAcl).not.toHaveBeenCalled();
  });

  it('다이얼로그를 닫으면 목록으로 돌아온다', async () => {
    renderSelector();
    openList();

    fireEvent.click(screen.getByTestId('dashboard-settings-acl-d1'));
    await waitFor(() => expect(screen.getByTestId('dashboard-acl-panel')).toBeInTheDocument());

    const panel = screen.getByTestId('dashboard-acl-panel');
    fireEvent.click(within(panel).getByRole('button', { name: '취소' }));

    expect(screen.queryByTestId('dashboard-acl-panel')).not.toBeInTheDocument();
    expect(screen.getByTestId('dashboard-settings-list')).toBeInTheDocument();
  });
});
