// 역할 관리 페이지 — 자리표시자 (SPEC-AUTH-006).
//
// 본 파일은 M3(메뉴·라우트 게이팅)이 `/admin/roles` 라우트를 등록하기 위한
// 최소 자리표시자다. 목록·생성·권한 행렬 체크박스·빌트인 역할 비활성화는
// M2(U2, AC-04)가 이 파일을 대체하며 구현한다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M3.4 — M2 가 대체)

import { useTranslation } from '@/lib/i18n';

export default function RolesPage(): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-6" data-testid="admin-roles-page">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('nav.roles')}
      </h1>
      <p className="text-sm text-(--color-text-muted)">{t('admin.comingSoon')}</p>
    </div>
  );
}
