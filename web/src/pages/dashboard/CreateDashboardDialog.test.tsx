// CreateDashboardDialog 테스트 (SPEC-DASHBOARD-004 M6 6.4, AC-03).
//
// 검증:
//   - 제출 시 `POST /api/v1/dashboards` 가 발생하고, **서버가 발급한 uid** 가
//     스토어에 반영된다(클라이언트가 uid 를 지어내지 않는다).
//   - `dashboard.create` 미보유 시 제출 컨트롤은 렌더되되 비활성 + 사유
//     (숨기지 않는다 — SPEC-AUTH-006 §4.2).
//   - 서버가 거부하면 모달을 닫지 않는다("만들어졌다" 오해 방지).

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// i18n: 실제 ko 번역 해석.
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

// 대시보드 서비스 — POST 관찰용. Forbidden 에러 클래스는 실제와 같은 이름으로 stub.
const createDashboardMock = vi.hoisted(() => vi.fn());
const StubForbiddenError = vi.hoisted(() => class DashboardForbiddenError extends Error {});
vi.mock('@/services/api/dashboardService', () => ({
  createDashboard: createDashboardMock,
  DashboardForbiddenError: StubForbiddenError,
}));

// authStore — usePermission 스냅샷.
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

import CreateDashboardDialog from './CreateDashboardDialog';

function setPermissions(keys: string[]) {
  authState.permissions = new Set(keys);
}

function serverDetail(uid: string, name: string) {
  return {
    uid,
    name,
    owner: 'edi',
    visibility: 'private' as const,
    is_default: false,
    sort_order: 0,
    version: 1,
    created_at: 0,
    updated_at: 0,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    payload: { panels: [], layout: [] },
  };
}

function renderDialog(onClose = vi.fn()) {
  render(<CreateDashboardDialog open onClose={onClose} />);
  return { onClose };
}

function submitButton() {
  return screen.getByRole('button', { name: '생성' });
}

beforeEach(() => {
  createDashboardMock.mockReset();
  setPermissions(['dashboard.read', 'dashboard.create']);
  useUIStore.getState().setDashboards([]);
  useUIStore.getState().setActiveDashboard('');
});

describe('CreateDashboardDialog — 서버 생성', () => {
  it('제출하면 POST 가 발생하고 서버가 발급한 uid 가 반영된다', async () => {
    createDashboardMock.mockResolvedValue(serverDetail('srv-generated-uid', '분석'));
    const { onClose } = renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '분석' } });
    fireEvent.click(submitButton());

    await waitFor(() => expect(onClose).toHaveBeenCalled());

    expect(createDashboardMock).toHaveBeenCalledWith('분석');
    // 서버가 준 uid 를 그대로 쓴다 — 클라이언트가 UUID 를 짓지 않는다.
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['srv-generated-uid']);
    expect(useUIStore.getState().activeDashboardId).toBe('srv-generated-uid');
    expect(useUIStore.getState().dashboardPages.map((p) => p.id)).toEqual(['srv-generated-uid']);
  });

  it('이름 앞뒤 공백을 제거해 보낸다', async () => {
    createDashboardMock.mockResolvedValue(serverDetail('u1', '분석'));
    renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '  분석  ' } });
    fireEvent.click(submitButton());

    await waitFor(() => expect(createDashboardMock).toHaveBeenCalledWith('분석'));
  });

  it('서버가 거부하면 모달을 닫지 않고 스토어도 바뀌지 않는다', async () => {
    createDashboardMock.mockRejectedValue(new StubForbiddenError('forbidden'));
    const { onClose } = renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '분석' } });
    fireEvent.click(submitButton());

    await waitFor(() => expect(createDashboardMock).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
    expect(useUIStore.getState().dashboards).toHaveLength(0);
  });
});

describe('CreateDashboardDialog — 생성 권한 게이팅 (AC-03)', () => {
  it('dashboard.create 미보유 시 제출 버튼이 렌더되되 비활성이다(숨기지 않는다)', () => {
    setPermissions(['dashboard.read']);
    renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '분석' } });

    const submit = submitButton();
    expect(submit).toBeInTheDocument();
    expect(submit).toBeDisabled();
    expect(submit).toHaveAttribute('aria-disabled', 'true');
    expect(submit).toHaveAttribute('title', '대시보드를 생성할 권한이 없습니다');
  });

  it('dashboard.create 미보유 시 사유와 완화책을 화면에 표시한다', () => {
    setPermissions(['dashboard.read']);
    renderDialog();

    const notice = screen.getByTestId('create-dashboard-denied');
    expect(notice).toHaveTextContent('대시보드를 생성할 권한이 없습니다');
    expect(notice).toHaveTextContent('관리자에게 대시보드 편집 권한을 요청하세요');
  });

  it('dashboard.create 미보유 시 제출해도 POST 가 발생하지 않는다', () => {
    setPermissions(['dashboard.read']);
    renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '분석' } });
    fireEvent.submit(submitButton().closest('form')!);

    expect(createDashboardMock).not.toHaveBeenCalled();
  });

  it('dashboard.create 보유 시 이름을 입력하면 제출 버튼이 활성이다', () => {
    setPermissions(['dashboard.read', 'dashboard.create']);
    renderDialog();

    fireEvent.change(screen.getByLabelText('대시보드 이름'), { target: { value: '분석' } });

    expect(submitButton()).toBeEnabled();
    expect(screen.queryByTestId('create-dashboard-denied')).toBeNull();
  });
});
