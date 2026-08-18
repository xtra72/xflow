// 사용자 관리 페이지 — 자리표시자 (SPEC-AUTH-006).
//
// 본 파일은 M3(메뉴·라우트 게이팅)이 `/admin/users` 라우트를 등록하기 위한
// 최소 자리표시자다. 목록·등록·역할 변경·비밀번호 재설정·삭제와 409 잠금 방지
// 안내는 M2(U2, AC-03)가 이 파일을 대체하며 구현한다.
//
// 라우트를 M2 까지 미등록으로 두면 메뉴만 존재하고 클릭 시 빈 화면이 되므로,
// 게이팅 전환(M3)이 자체적으로 검증 가능하도록 자리표시자를 먼저 둔다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M3.4 — M2 가 대체)

import { useTranslation } from '@/lib/i18n';

export default function UsersPage(): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-6" data-testid="admin-users-page">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('nav.users')}
      </h1>
      <p className="text-sm text-(--color-text-muted)">{t('admin.comingSoon')}</p>
    </div>
  );
}
