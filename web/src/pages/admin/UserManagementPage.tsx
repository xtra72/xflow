// 사용자 관리 화면 — 사용자 / 역할 두 탭의 컨테이너.
//
// 역할 관리를 별도 화면에서 이 화면의 탭으로 옮겼다. 두 화면 모두 목록+편집
// 폼이 있는 470~490줄 규모라 세로로 쌓으면 한 화면에 담기지 않는다.
//
// 제목은 그리지 않는다 — 앱 헤더(components/layout/Header.tsx)의 PAGE_TITLE_KEYS
// 가 '/admin/users' 를 nav.users 로 매핑해 표시한다. 본문에도 제목을 두면 같은
// 문구가 두 번 보인다.
//
// 탭 상태는 URL 쿼리(`?tab=roles`)에 둔다. 컴포넌트 state 로 두면 북마크·새로고침·
// 뒤로가기에서 탭이 초기화되고, 구 `/admin/roles` 북마크를 역할 탭으로 보내는
// 리다이렉트도 불가능하다. 기본 탭(사용자)은 파라미터를 제거해 URL 을 깨끗이
// 유지한다(NodeManagementPage 와 같은 규약).
//
// 게이팅 두 축을 섞지 않는다:
//   - 메뉴 축(nav.role): 없으면 역할 탭 자체를 렌더하지 않는다.
//   - 데이터 축(role.read / user.create 등): 있어도 권한이 없으면 컨트롤을
//     비활성 + 사유 툴팁으로 남긴다(PermissionButton). 각 패널이 처리한다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.2, M2.3)

import { useSearchParams } from 'react-router';
import { ShieldCheck, Users } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import RolesPanel from './RolesPanel';
import UsersPanel from './UsersPanel';

/** 탭 식별자. URL `?tab=` 값과 1:1 대응한다. */
type TabId = 'users' | 'roles';

/** 기본 탭(사용자)은 파라미터가 없거나 알 수 없는 값일 때 선택된다. */
function parseTab(raw: string | null): TabId {
  return raw === 'roles' ? 'roles' : 'users';
}

export default function UserManagementPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { canSeeMenu } = usePermission();
  const [searchParams, setSearchParams] = useSearchParams();

  // 메뉴 축 판정. nav.role 이 없으면 탭 자체가 없다.
  const canSeeRoles = canSeeMenu('nav.role');

  // nav.role 없이 `?tab=roles` 로 직접 들어오면(구 북마크 등) 사용자 탭으로
  // 낮춘다 — 렌더되지 않는 탭이 활성으로 남으면 본문이 비어 보인다.
  const requested = parseTab(searchParams.get('tab'));
  const activeTab: TabId = requested === 'roles' && !canSeeRoles ? 'users' : requested;

  function selectTab(tab: TabId): void {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        if (tab === 'users') params.delete('tab');
        else params.set('tab', tab);
        return params;
      },
      { replace: true },
    );
  }

  const tabs: { id: TabId; labelKey: string; icon: typeof Users }[] = [
    { id: 'users', labelKey: 'admin.tabUsers', icon: Users },
    ...(canSeeRoles
      ? [{ id: 'roles' as const, labelKey: 'admin.tabRoles', icon: ShieldCheck }]
      : []),
  ];

  // role="tablist" 은 좌우 화살표 이동을 전제한 계약이다. 클릭만 처리하면
  // 스크린리더 사용자가 탭 사이를 이동할 방법이 없으므로 함께 구현한다.
  function handleTabKeyDown(e: React.KeyboardEvent, index: number): void {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    e.preventDefault();
    const delta = e.key === 'ArrowRight' ? 1 : -1;
    // 양끝에서 순환한다(WAI-ARIA tabs 패턴).
    const next = tabs[(index + delta + tabs.length) % tabs.length];
    if (next) selectTab(next.id);
  }

  return (
    <div className="space-y-4" data-testid="admin-user-management-page">
      <div
        role="tablist"
        aria-label={t('admin.tabsLabel')}
        className="flex gap-1 border-b border-(--color-border-default)"
      >
        {tabs.map((tab, index) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`admin-tab-${tab.id}`}
              aria-selected={isActive}
              aria-controls={`admin-tabpanel-${tab.id}`}
              // 활성 탭만 탭 순서에 남긴다(roving tabindex) — tablist 는 하나의
              // 정지점으로 취급되고, 내부 이동은 화살표가 담당한다.
              tabIndex={isActive ? 0 : -1}
              onClick={() => selectTab(tab.id)}
              onKeyDown={(e) => handleTabKeyDown(e, index)}
              className={cn(
                'inline-flex items-center gap-2 whitespace-nowrap border-b-2 px-4 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
                  : 'border-transparent text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
              )}
            >
              <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>{t(tab.labelKey)}</span>
            </button>
          );
        })}
      </div>

      <div
        role="tabpanel"
        id={`admin-tabpanel-${activeTab}`}
        aria-labelledby={`admin-tab-${activeTab}`}
      >
        {activeTab === 'users' ? <UsersPanel /> : <RolesPanel />}
      </div>
    </div>
  );
}
