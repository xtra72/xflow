// 권한 기반 비활성 버튼 (SPEC-AUTH-006 E1 / M4.1).
//
// spec.md §2.4·§4.2: 액션 컨트롤은 숨기지 않고 **비활성 + 사유 툴팁**으로 둔다.
// 버튼이 사라지면 사용자는 기능이 없는 것으로 오해하지만, 비활성 상태는
// 관리자에게 권한을 요청할 여지를 남긴다. 숨김은 화면 단위(메뉴·라우트)에만
// 적용하며 그쪽은 M3 가 처리했다.
//
// 폴백 규칙(인증 비활성·구버전 서버·조회 실패)은 usePermission 한 곳에만 있다.
// 호출부가 `!authEnabled || ...` 를 손으로 적으면 규칙이 갈라진다.

import type { ButtonHTMLAttributes, ReactNode } from 'react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';

interface PermissionButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** 이 컨트롤이 요구하는 권한 키 (예: 'agent.execute'). */
  permission: string;
  /** 권한 부족 시 표시할 사유. 미지정이면 공용 문구를 쓴다. */
  deniedTitle?: string;
  children?: ReactNode;
}

/**
 * 권한이 없으면 비활성 + `aria-disabled` + 사유 툴팁으로 렌더한다.
 *
 * 호출부가 이미 넘긴 `disabled` / `title` 과 합성된다 — 권한 판정은 기존
 * 비활성 사유(실행 중, 원격 미지원 등)를 덮어쓰지 않고 더한다. 권한 부족이
 * 사유일 때만 툴팁을 권한 문구로 바꾼다.
 */
export default function PermissionButton({
  permission,
  deniedTitle,
  disabled,
  title,
  onClick,
  children,
  ...rest
}: PermissionButtonProps) {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();

  const allowed = hasPermission(permission);
  const isDisabled = disabled === true || !allowed;

  return (
    <button
      {...rest}
      disabled={isDisabled}
      aria-disabled={isDisabled}
      title={allowed ? title : (deniedTitle ?? t('common.permissionRequired'))}
      onClick={allowed ? onClick : undefined}
    >
      {children}
    </button>
  );
}
