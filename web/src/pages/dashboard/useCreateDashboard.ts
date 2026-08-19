// 대시보드 생성 (SPEC-DASHBOARD-004 M6 6.4, spec.md §2.7 E1).
//
// 구 모델의 `addDashboardPage(name)` 은 클라이언트 스토어만 바꾸는 로컬 생성이라
// 서버가 그 대시보드의 존재를 몰랐다. 여기서는 `POST /api/v1/dashboards` 로
// 생성하고 **서버가 발급한 uid** 를 그대로 축에 삽입한다 — uid 를 클라이언트가
// 지어내면 서버 행과 다른 식별자가 생겨 이후 PUT·DELETE 가 404 가 된다.
//
// 생성 즉시 목록을 재조회하지 않고 응답 본문으로 삽입한다(spec.md §2.7).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.7)

import { useCallback, useState } from 'react';

import { useTranslation } from '@/lib/i18n';
import {
  createDashboard,
  DashboardForbiddenError,
} from '@/services/api/dashboardService';
import { useUIStore } from '@/stores/uiStore';
import type { DashboardDetail } from '@/types/dashboard';

/** `useCreateDashboard()` 반환값. */
export interface CreateDashboardApi {
  /** 생성 요청. 성공하면 생성된 대시보드, 실패하면 null. */
  create: (name: string) => Promise<DashboardDetail | null>;
  /** 요청 진행 중 여부 (중복 제출 방지용). */
  isCreating: boolean;
}

/**
 * 서버에 대시보드를 생성하고 스토어에 반영한다.
 *
 * 실패는 throw 하지 않고 null 로 돌려준다 — 호출부(다이얼로그·드롭다운)는 모달을
 * 열어둔 채 사유만 보여주면 되고, 예외를 각자 분기하면 처리가 갈라진다.
 * 403 은 권한 사유로, 그 밖은 일반 실패로 알린다.
 */
export function useCreateDashboard(): CreateDashboardApi {
  const { t } = useTranslation();
  const applyDashboardDetail = useUIStore((s) => s.applyDashboardDetail);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const addNotification = useUIStore((s) => s.addNotification);
  const [isCreating, setIsCreating] = useState(false);

  const create = useCallback(
    async (name: string): Promise<DashboardDetail | null> => {
      const trimmed = name.trim();
      if (!trimmed) return null;

      setIsCreating(true);
      try {
        const detail = await createDashboard(trimmed);
        // 서버 응답을 그대로 반영한다 — uid·version·can_* 는 전부 서버 소유다.
        applyDashboardDetail(detail);
        setActiveDashboard(detail.uid);
        return detail;
      } catch (err) {
        addNotification({
          type: 'error',
          message: t(
            err instanceof DashboardForbiddenError
              ? 'dashboard.gate.createDenied'
              : 'dashboard.gate.createFailed',
          ),
        });
        return null;
      } finally {
        setIsCreating(false);
      }
    },
    [applyDashboardDetail, setActiveDashboard, addNotification, t],
  );

  return { create, isCreating };
}
