// 비밀번호 입력 필드 — 표시/숨김 토글을 포함한 공용 컴포넌트.
//
// 비밀번호를 입력받는 화면이 여러 곳(사용자 등록·비밀번호 재설정·비밀번호 변경)
// 이라 토글을 화면마다 인라인으로 복사하면 결함도 각자 따로 생긴다. 실제로 잘
// 빠뜨리는 것이 셋이다.
//   1. type="button" — 지정하지 않으면 button 의 기본 type 은 "submit" 이라
//      토글을 누를 때마다 폼이 제출된다.
//   2. aria-label 이 현재 상태가 아니라 "동작"을 가리켜야 한다는 점 — 가려진
//      상태의 버튼은 "표시", 드러난 상태의 버튼은 "숨기기"로 읽혀야 한다.
//   3. aria-pressed — 토글 버튼임을 보조기술에 알리는 유일한 신호다.
// 이 셋을 한 곳에서만 지키면 되도록 컴포넌트로 묶었다.
//
// 표시 상태(revealed)는 이 컴포넌트 내부에만 둔다. 다이얼로그가 닫힐 때 트리에서
// 언마운트되므로 다시 열면 자동으로 '숨김'으로 돌아간다 — 이전에 열었던 다이얼로그
// 에서 드러낸 비밀번호가 그대로 남아 보이는 유출을 막기 위한 것이다. 다이얼로그를
// 언마운트하지 않고 CSS 로만 감추도록 바꾸면 이 보장이 깨진다. PasswordField.test.tsx
// 와 UsersPanel.test.tsx 가 그 회귀를 잠근다.

import { useState } from 'react';
import type { InputHTMLAttributes } from 'react';
import { Eye, EyeOff } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface PasswordFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  /**
   * 토글 버튼 이름에 덧붙일 필드 이름 (예: '새 비밀번호').
   *
   * 한 폼에 비밀번호 입력이 둘 이상이면 토글 버튼의 접근 가능한 이름이 전부
   * "비밀번호 표시"로 같아져 어느 필드의 토글인지 구분할 수 없다. 이 프로젝트가
   * 이미 쓰는 `${동작} ${대상}` 형태(예: "삭제 alice")를 그대로 따른다.
   */
  fieldLabel?: string;
}

/**
 * `type="password"` 와 `type="text"` 를 오가는 토글이 달린 입력 필드.
 *
 * className 은 input 에 적용된다 — 호출부가 화면의 공용 입력 클래스
 * (예: UsersPanel 의 INPUT_CLASS)를 그대로 넘기면 주변 입력과 같은 모양이 된다.
 * 토글 버튼 자리를 확보하는 우측 여백(pr-10)만 이 컴포넌트가 덧붙인다.
 */
export default function PasswordField({
  fieldLabel,
  className,
  ...rest
}: PasswordFieldProps): React.JSX.Element {
  const { t } = useTranslation();
  const [revealed, setRevealed] = useState(false);

  // 버튼 이름은 "지금 누르면 일어날 일"이다. 드러난 상태에서는 '숨기기'가 된다.
  const action = revealed ? t('auth.hidePassword') : t('auth.showPassword');
  const toggleLabel = fieldLabel ? `${action} ${fieldLabel}` : action;

  return (
    <div className="relative">
      <input
        {...rest}
        type={revealed ? 'text' : 'password'}
        className={cn(className, 'pr-10')}
      />
      {/* type="button" 은 필수다. 생략하면 기본값이 submit 이라 토글이 폼을
          제출한다. tabIndex 는 지정하지 않는다 — 키보드만 쓰는 사용자도 토글에
          도달할 수 있어야 하므로 기본 탭 순서에 남긴다. */}
      <button
        type="button"
        onClick={() => setRevealed((prev) => !prev)}
        className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-(--color-text-muted) hover:text-(--color-text-secondary)"
        aria-label={toggleLabel}
        aria-pressed={revealed}
      >
        {revealed ? (
          <EyeOff className="h-4 w-4" aria-hidden="true" />
        ) : (
          <Eye className="h-4 w-4" aria-hidden="true" />
        )}
      </button>
    </div>
  );
}
