// dashboardAccess 단위 테스트 (SPEC-DASHBOARD-004 M6 6.1).
//
// 검증:
//   - 서버가 실어 보낸 can_edit / can_grant / can_delete 를 그대로 읽는다.
//   - 세 플래그는 서로 독립이다(하나를 끄면 나머지는 영향받지 않는다).
//   - 목록에 없는 uid 는 허용으로 둔다 — 부팅 중 화면이 잠기지 않아야 하고,
//     실제 차단은 서버가 403 으로 수행한다.

import { describe, expect, it } from 'vitest';

import { dashboardAccessByUid, dashboardAccessOf } from './dashboardAccess';
import type { Dashboard } from '@/types/dashboard';

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'd1',
    name: '대시보드 1',
    owner: 'edi',
    visibility: 'private',
    is_default: false,
    sort_order: 0,
    version: 1,
    created_at: 0,
    updated_at: 0,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

describe('dashboardAccessOf', () => {
  it('서버 플래그를 그대로 읽는다', () => {
    expect(
      dashboardAccessOf(makeDashboard({ can_edit: false, can_grant: true, can_delete: false })),
    ).toEqual({ canEdit: false, canGrant: true, canDelete: false });
  });

  it('can_edit 만 false 여도 grant / delete 는 유지된다(플래그 독립)', () => {
    expect(dashboardAccessOf(makeDashboard({ can_edit: false }))).toEqual({
      canEdit: false,
      canGrant: true,
      canDelete: true,
    });
  });

  it('can_grant 만 false 여도 edit / delete 는 유지된다', () => {
    expect(dashboardAccessOf(makeDashboard({ can_grant: false }))).toEqual({
      canEdit: true,
      canGrant: false,
      canDelete: true,
    });
  });

  it('can_delete 만 false 여도 edit / grant 는 유지된다', () => {
    expect(dashboardAccessOf(makeDashboard({ can_delete: false }))).toEqual({
      canEdit: true,
      canGrant: true,
      canDelete: false,
    });
  });

  it('항목이 없으면 전부 허용한다(목록 미도착 시 선제 차단 금지)', () => {
    expect(dashboardAccessOf(undefined)).toEqual({
      canEdit: true,
      canGrant: true,
      canDelete: true,
    });
  });
});

describe('dashboardAccessByUid', () => {
  it('uid 로 해당 항목의 판정을 고른다', () => {
    const list = [
      makeDashboard({ uid: 'a', can_edit: true }),
      makeDashboard({ uid: 'b', can_edit: false }),
    ];
    expect(dashboardAccessByUid(list, 'a').canEdit).toBe(true);
    expect(dashboardAccessByUid(list, 'b').canEdit).toBe(false);
  });

  it('목록에 없는 uid 는 허용으로 둔다', () => {
    expect(dashboardAccessByUid([makeDashboard({ uid: 'a' })], 'zzz')).toEqual({
      canEdit: true,
      canGrant: true,
      canDelete: true,
    });
  });
});
