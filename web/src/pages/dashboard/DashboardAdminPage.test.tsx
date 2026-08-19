// 대시보드 관리 화면 테스트 (SPEC-DASHBOARD-004 M7 7.1).
//
// 서비스 계층만 스텁하고 훅(useCreateDashboard / useDashboardMutations)·스토어·
// i18n 은 실제 코드를 돌린다. i18n 을 실제 Provider 로 두면 누락된 키가 렌더
// 문자열에서 즉시 드러난다(RolesPage.test.tsx 와 동일한 방침).
//
// 검증 축:
//   - CRUD 각 경로가 **어떤 요청을 보내는가** (계약은 spec.md §2.3 라우트 표).
//   - 컨트롤이 항목별 인가 판정(can_edit / can_grant / can_delete)을 따르며
//     숨기지 않고 비활성 + 사유 툴팁이다(spec.md §2.11 S2).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.3, §2.7, §2.11)

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

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => granted.keys.has(k)),
    canSeeMenu: (key: string) => granted.keys.has(key),
    isPermissionUnavailable: false,
  }),
}));

import { useUIStore } from '@/stores/uiStore';
import DashboardAdminPage from './DashboardAdminPage';

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

function seed(list: Dashboard[]): void {
  useUIStore.getState().setDashboards(list);
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <DashboardAdminPage />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  grant('dashboard.create', 'dashboard.update', 'dashboard.delete', 'dashboard.read');
  seed([makeDashboard()]);
  listDashboards.mockResolvedValue([]);
  getAcl.mockResolvedValue([]);
});

describe('DashboardAdminPage — 목록', () => {
  it('목록 축(dashboards)의 이름·소유자·공개범위를 렌더한다', () => {
    seed([
      makeDashboard({ uid: 'd1', name: '운영', owner: 'root', visibility: 'shared' }),
      makeDashboard({ uid: 'd2', name: '개인', owner: 'edi', visibility: 'private' }),
    ]);
    renderPage();

    expect(screen.getByText('운영')).toBeInTheDocument();
    expect(screen.getByText('개인')).toBeInTheDocument();
    expect(screen.getByText('root')).toBeInTheDocument();
    expect(screen.getByText('edi')).toBeInTheDocument();
    expect(screen.getByLabelText('공개범위 변경 운영')).toHaveValue('shared');
    expect(screen.getByLabelText('공개범위 변경 개인')).toHaveValue('private');
  });

  it('목록이 비면 빈 상태 문구를 표시한다', () => {
    seed([]);
    renderPage();
    expect(screen.getByText('접근 가능한 대시보드가 없습니다')).toBeInTheDocument();
  });

  it('기본 대시보드에는 기본 배지를 표시한다', () => {
    seed([makeDashboard({ uid: 'd1', is_default: true })]);
    renderPage();
    expect(screen.getByTestId('dashboard-admin-default-d1')).toBeInTheDocument();
  });
});

describe('DashboardAdminPage — 생성 (spec.md §2.7 E1)', () => {
  it('이름을 제출하면 POST /dashboards 로 생성한다', async () => {
    createDashboard.mockResolvedValue({
      ...makeDashboard({ uid: 'd9', name: '신규' }),
      payload: { panels: [], layout: [] },
    });
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: '대시보드 생성' }));
    fireEvent.change(screen.getByLabelText('이름'), { target: { value: '  신규  ' } });
    fireEvent.submit(screen.getByTestId('dashboard-admin-create-form'));

    // 이름은 trim 되어 전송된다. owner/visibility/uid 는 보내지 않는다.
    await waitFor(() => expect(createDashboard).toHaveBeenCalledWith('신규'));
  });

  it('dashboard.create 가 없으면 생성 버튼을 비활성하고 사유를 툴팁으로 준다', () => {
    grant('dashboard.read');
    renderPage();

    const button = screen.getByRole('button', { name: '대시보드 생성' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', '대시보드를 생성할 권한이 없습니다');
  });
});

describe('DashboardAdminPage — 이름 변경 (can_edit)', () => {
  it('이름을 바꾸면 PATCH {name} 을 보낸다', async () => {
    patchDashboard.mockResolvedValue({
      ...makeDashboard({ name: '바뀐 이름' }),
      payload: { panels: [], layout: [] },
    });
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: '이름 변경 운영 대시보드' }));
    fireEvent.change(screen.getByLabelText('대시보드 이름 편집'), {
      target: { value: '바뀐 이름' },
    });
    fireEvent.click(screen.getByRole('button', { name: '이름 저장' }));

    await waitFor(() =>
      expect(patchDashboard).toHaveBeenCalledWith('d1', { name: '바뀐 이름' }),
    );
  });

  it('can_edit=false 면 이름 변경 버튼을 비활성한다 (숨기지 않는다)', () => {
    seed([makeDashboard({ can_edit: false })]);
    renderPage();

    const button = screen.getByRole('button', { name: '이름 변경 운영 대시보드' });
    expect(button).toBeInTheDocument();
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', '이 대시보드를 편집할 권한이 없습니다');
  });
});

describe('DashboardAdminPage — 공개범위 변경 (can_grant)', () => {
  it('공개범위를 바꾸면 PATCH {visibility} 를 보낸다', async () => {
    patchDashboard.mockResolvedValue({
      ...makeDashboard({ visibility: 'acl' }),
      payload: { panels: [], layout: [] },
    });
    renderPage();

    fireEvent.change(screen.getByLabelText('공개범위 변경 운영 대시보드'), {
      target: { value: 'acl' },
    });

    await waitFor(() =>
      expect(patchDashboard).toHaveBeenCalledWith('d1', { visibility: 'acl' }),
    );
  });

  it('can_grant=false 면 공개범위 select 를 비활성한다', () => {
    seed([makeDashboard({ can_grant: false })]);
    renderPage();

    const select = screen.getByLabelText('공개범위 변경 운영 대시보드');
    expect(select).toBeDisabled();
    expect(select).toHaveAttribute(
      'title',
      '이 대시보드의 공개 설정을 변경할 권한이 없습니다',
    );
  });
});

describe('DashboardAdminPage — 삭제 (can_delete)', () => {
  it('확인 후 DELETE /dashboards/{uid} 를 보낸다', async () => {
    deleteDashboard.mockResolvedValue(undefined);
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: '삭제 운영 대시보드' }));
    // 확인 다이얼로그를 거치지 않고는 지우지 않는다.
    expect(deleteDashboard).not.toHaveBeenCalled();

    const dialog = screen.getByRole('dialog', { name: '대시보드 삭제' });
    fireEvent.click(within(dialog).getByRole('button', { name: '삭제' }));

    await waitFor(() => expect(deleteDashboard).toHaveBeenCalledWith('d1'));
  });

  it('마지막 1장이어도 삭제를 막지 않는다 (spec.md 엣지 케이스)', () => {
    seed([makeDashboard()]);
    renderPage();
    expect(screen.getByRole('button', { name: '삭제 운영 대시보드' })).toBeEnabled();
  });

  it('can_delete=false 면 삭제 버튼을 비활성한다', () => {
    seed([makeDashboard({ can_delete: false })]);
    renderPage();

    const button = screen.getByRole('button', { name: '삭제 운영 대시보드' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', '이 대시보드를 삭제할 권한이 없습니다');
  });
});

describe('DashboardAdminPage — 권한 부여 패널 진입 (grant 인가)', () => {
  it('can_grant=true 면 패널을 열고 ACL 을 조회한다', async () => {
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: '권한 부여 운영 대시보드' }));

    await waitFor(() => expect(getAcl).toHaveBeenCalledWith('d1'));
    expect(screen.getByTestId('dashboard-acl-panel')).toBeInTheDocument();
  });

  it('can_grant=false 면 권한 부여 버튼이 비활성이고 패널이 열리지 않는다', () => {
    seed([makeDashboard({ can_grant: false })]);
    renderPage();

    const button = screen.getByRole('button', { name: '권한 부여 운영 대시보드' });
    expect(button).toBeDisabled();
    fireEvent.click(button);

    expect(screen.queryByTestId('dashboard-acl-panel')).not.toBeInTheDocument();
    expect(getAcl).not.toHaveBeenCalled();
  });
});
