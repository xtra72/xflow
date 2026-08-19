// useDashboardMutations 테스트 (SPEC-DASHBOARD-004 M6 후속).
//
// 배경: `name` 과 `is_default` 는 1급 엔티티의 **컬럼**이 되었고 본문 PUT 은
// `{ payload }` 만 보낸다. 따라서 PATCH 를 보내지 않으면 이름 변경·기본 지정이
// 영속되지 않는다 — 사용자는 바뀐 화면을 보고 새로고침에서 잃는다.
//
// 검증:
//   - 세 동작이 각각 올바른 요청을 올바른 인자로 보낸다.
//   - 거부된 요청은 로컬 상태를 **원상 복구**하고 토스트를 남긴다.
//   - 활성 대시보드를 지우면 M5 폴백 사슬(`is_default` → 첫 항목)로 착지한다.
//   - 403 은 목록을 재조회해 판정을 다시 읽는다(추측하지 않는다).

import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Dashboard } from '@/types/dashboard';

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

const patchDashboardMock = vi.hoisted(() => vi.fn());
const deleteDashboardMock = vi.hoisted(() => vi.fn());
const listDashboardsMock = vi.hoisted(() => vi.fn());
const ForbiddenError = vi.hoisted(() => class DashboardForbiddenError extends Error {});
vi.mock('@/services/api/dashboardService', () => ({
  patchDashboard: patchDashboardMock,
  deleteDashboard: deleteDashboardMock,
  listDashboards: listDashboardsMock,
  DashboardForbiddenError: ForbiddenError,
}));

import { useUIStore } from '@/stores/uiStore';

import { useDashboardMutations } from './useDashboardMutations';

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '운영',
    owner: 'edi',
    visibility: 'shared',
    is_default: false,
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

function seed(list: Dashboard[], activeUid = list[0]?.uid ?? '') {
  useUIStore.getState().setDashboards(list);
  useUIStore.getState().setActiveDashboard(activeUid);
  useUIStore.getState().clearNotifications();
}

const nameOf = (uid: string) => useUIStore.getState().dashboards.find((d) => d.uid === uid)?.name;
const pageNameOf = (uid: string) =>
  useUIStore.getState().dashboardPages.find((p) => p.id === uid)?.name;
const errorMessages = () =>
  useUIStore.getState().notifications.filter((n) => n.type === 'error').map((n) => n.message);

beforeEach(() => {
  patchDashboardMock.mockReset();
  deleteDashboardMock.mockReset();
  listDashboardsMock.mockReset();
  seed([makeDashboard()]);
});

describe('rename — PATCH {name}', () => {
  it('올바른 인자로 PATCH 를 보낸다', async () => {
    patchDashboardMock.mockResolvedValue({ ...makeDashboard({ name: '새 이름' }), payload: null });
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.rename('d1', '새 이름');
    });

    expect(patchDashboardMock).toHaveBeenCalledTimes(1);
    expect(patchDashboardMock).toHaveBeenCalledWith('d1', { name: '새 이름' });
    expect(nameOf('d1')).toBe('새 이름');
    // 본문 축도 함께 갱신되어야 화면(드롭다운)과 판정이 어긋나지 않는다.
    expect(pageNameOf('d1')).toBe('새 이름');
  });

  it('앞뒤 공백을 제거해 보낸다', async () => {
    patchDashboardMock.mockResolvedValue({ ...makeDashboard({ name: '새 이름' }), payload: null });
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.rename('d1', '  새 이름  ');
    });

    expect(patchDashboardMock).toHaveBeenCalledWith('d1', { name: '새 이름' });
  });

  it('거부되면 이름을 원래대로 되돌리고 토스트를 남긴다', async () => {
    patchDashboardMock.mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => useDashboardMutations());

    let ok: boolean | undefined;
    await act(async () => {
      ok = await result.current.rename('d1', '새 이름');
    });

    expect(ok).toBe(false);
    // 서버가 거부한 변경이 화면에 남아 있으면 안 된다.
    expect(nameOf('d1')).toBe('운영');
    expect(pageNameOf('d1')).toBe('운영');
    expect(errorMessages()).toContain('대시보드 이름을 변경하지 못했습니다');
  });

  it('403 이면 되돌린 뒤 목록을 재조회해 판정을 다시 읽는다', async () => {
    patchDashboardMock.mockRejectedValue(new ForbiddenError('forbidden'));
    // 재조회 결과: 편집 권한을 잃은 상태로 돌아온다.
    listDashboardsMock.mockResolvedValue([makeDashboard({ can_edit: false })]);
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.rename('d1', '새 이름');
    });

    expect(nameOf('d1')).toBe('운영');
    expect(listDashboardsMock).toHaveBeenCalledTimes(1);
    // 판정을 추측하지 않고 서버가 준 값을 반영한다.
    await waitFor(() => {
      expect(useUIStore.getState().dashboards[0]?.can_edit).toBe(false);
    });
    expect(errorMessages()).toContain('이 대시보드를 편집할 권한이 없습니다');
  });

  it('이름이 그대로면 요청하지 않는다', async () => {
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.rename('d1', '운영');
    });

    expect(patchDashboardMock).not.toHaveBeenCalled();
  });
});

describe('setDefault — PATCH {is_default}', () => {
  it('올바른 인자로 PATCH 를 보낸다', async () => {
    patchDashboardMock.mockResolvedValue({ ...makeDashboard({ is_default: true }), payload: null });
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.setDefault('d1');
    });

    expect(patchDashboardMock).toHaveBeenCalledTimes(1);
    expect(patchDashboardMock).toHaveBeenCalledWith('d1', { is_default: true });
    expect(useUIStore.getState().dashboards[0]?.is_default).toBe(true);
  });

  it('거부되면 기본 지정을 되돌리고 토스트를 남긴다', async () => {
    patchDashboardMock.mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.setDefault('d1');
    });

    expect(useUIStore.getState().dashboards[0]?.is_default).toBe(false);
    expect(errorMessages()).toContain('기본 대시보드를 변경하지 못했습니다');
  });

  it('403 이면 되돌린 뒤 목록을 재조회한다', async () => {
    patchDashboardMock.mockRejectedValue(new ForbiddenError('forbidden'));
    listDashboardsMock.mockResolvedValue([makeDashboard({ can_grant: false })]);
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.setDefault('d1');
    });

    expect(useUIStore.getState().dashboards[0]?.is_default).toBe(false);
    expect(listDashboardsMock).toHaveBeenCalledTimes(1);
    expect(errorMessages()).toContain('이 대시보드의 공개 설정을 변경할 권한이 없습니다');
  });
});

describe('remove — DELETE', () => {
  it('올바른 uid 로 DELETE 를 보내고 목록에서 제거한다', async () => {
    deleteDashboardMock.mockResolvedValue(undefined);
    seed([makeDashboard({ uid: 'd1' }), makeDashboard({ uid: 'd2', name: '분석' })], 'd2');
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.remove('d1');
    });

    expect(deleteDashboardMock).toHaveBeenCalledTimes(1);
    expect(deleteDashboardMock).toHaveBeenCalledWith('d1');
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['d2']);
    // 활성 대시보드가 아니었으므로 활성은 그대로다.
    expect(useUIStore.getState().activeDashboardId).toBe('d2');
  });

  it('활성 대시보드를 지우면 is_default 대시보드로 착지한다(M5 폴백 사슬)', async () => {
    deleteDashboardMock.mockResolvedValue(undefined);
    seed(
      [
        makeDashboard({ uid: 'active', name: '지울 것' }),
        makeDashboard({ uid: 'first', name: '첫번째' }),
        makeDashboard({ uid: 'starred', name: '기본', is_default: true }),
      ],
      'active',
    );
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.remove('active');
    });

    // 첫 항목이 아니라 is_default 가 우선한다.
    expect(useUIStore.getState().activeDashboardId).toBe('starred');
  });

  it('is_default 가 없으면 첫 항목으로 착지한다', async () => {
    deleteDashboardMock.mockResolvedValue(undefined);
    seed(
      [
        makeDashboard({ uid: 'active', name: '지울 것' }),
        makeDashboard({ uid: 'first', name: '첫번째' }),
        makeDashboard({ uid: 'second', name: '두번째' }),
      ],
      'active',
    );
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.remove('active');
    });

    expect(useUIStore.getState().activeDashboardId).toBe('first');
  });

  it('마지막 대시보드를 지우면 빈 상태가 된다(무한 재시도 없음)', async () => {
    deleteDashboardMock.mockResolvedValue(undefined);
    seed([makeDashboard({ uid: 'only' })], 'only');
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.remove('only');
    });

    expect(useUIStore.getState().dashboards).toHaveLength(0);
    expect(useUIStore.getState().activeDashboardId).toBe('');
  });

  it('거부되면 목록을 그대로 두고 토스트를 남긴다', async () => {
    deleteDashboardMock.mockRejectedValue(new Error('boom'));
    seed([makeDashboard({ uid: 'd1' }), makeDashboard({ uid: 'd2' })], 'd1');
    const { result } = renderHook(() => useDashboardMutations());

    let ok: boolean | undefined;
    await act(async () => {
      ok = await result.current.remove('d1');
    });

    expect(ok).toBe(false);
    // 서버가 거부했으므로 화면에서 사라지면 안 된다.
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['d1', 'd2']);
    expect(useUIStore.getState().activeDashboardId).toBe('d1');
    expect(errorMessages()).toContain('대시보드를 삭제하지 못했습니다');
  });

  it('403 이면 목록을 재조회해 판정을 다시 읽는다', async () => {
    deleteDashboardMock.mockRejectedValue(new ForbiddenError('forbidden'));
    listDashboardsMock.mockResolvedValue([makeDashboard({ uid: 'd1', can_delete: false })]);
    seed([makeDashboard({ uid: 'd1' })], 'd1');
    const { result } = renderHook(() => useDashboardMutations());

    await act(async () => {
      await result.current.remove('d1');
    });

    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['d1']);
    expect(listDashboardsMock).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(useUIStore.getState().dashboards[0]?.can_delete).toBe(false);
    });
    expect(errorMessages()).toContain('이 대시보드를 삭제할 권한이 없습니다');
  });
});
