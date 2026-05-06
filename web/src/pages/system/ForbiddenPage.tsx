// SPEC-WEB-006 v0.1.0 (M1, M11) — 권한 부족(403) 안내 페이지.
//
// `/admin/system` 같은 관리자 전용 라우트에 비-관리자(viewer/editor)가 접근하면
// AuthGuard 의 `requireRole` 분기가 본 페이지를 렌더한다.
//
// 페이지 책임:
//   - 한글 안내 메시지 (관리자 권한 필요)
//   - 홈("/")으로 돌아가는 링크 제공
//
// 구현 메모:
//   - 별도 i18n 키를 추가하지 않고 인라인 한글 사용 (다른 시스템 페이지와 일관).
//   - 단순한 정보성 페이지이므로 hooks/effects 없이 순수 마크업만 포함한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M1, M11)

import { Link } from 'react-router';
import { ShieldAlert } from 'lucide-react';

/**
 * 관리자 권한이 필요한 라우트에 접근하려는 비-관리자 사용자에게
 * 표시되는 403 안내 페이지.
 */
export function ForbiddenPage(): React.JSX.Element {
  return (
    <div
      data-testid="forbidden-page"
      className="mx-auto flex max-w-lg flex-col items-center gap-4 p-12 text-center"
      role="alert"
    >
      <ShieldAlert
        className="h-12 w-12 text-(--color-text-muted)"
        aria-hidden="true"
      />
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        접근 권한이 없습니다
      </h1>
      <p className="text-sm text-(--color-text-muted)">
        이 페이지는 관리자(admin) 권한을 가진 사용자만 이용할 수 있습니다.
      </p>
      <Link
        to="/"
        className="mt-2 inline-flex items-center gap-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-4 py-2 text-sm font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
        data-testid="forbidden-home-link"
      >
        홈으로 돌아가기
      </Link>
    </div>
  );
}

export default ForbiddenPage;
