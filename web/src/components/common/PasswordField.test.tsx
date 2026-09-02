// 비밀번호 표시/숨김 토글 컴포넌트 테스트.
//
// 이 컴포넌트가 존재하는 이유가 곧 검증 대상이다 — 인라인 복사본마다 따로 생기던
// 세 결함(폼 제출, 상태를 반영하지 않는 aria-label, 다이얼로그를 다시 열었을 때
// 남아 있는 표시 상태)을 한 곳에서 막는다.

import { fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import PasswordField from './PasswordField';

/** 제어 컴포넌트라 값 상태를 감싸 준다. */
function Harness(props: { fieldLabel?: string }) {
  const [value, setValue] = useState('');
  return (
    <I18nProvider>
      <PasswordField
        aria-label="비밀번호 입력"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        {...props}
      />
    </I18nProvider>
  );
}

const input = () => screen.getByLabelText('비밀번호 입력');
const toggle = () => screen.getByRole('button');

describe('PasswordField 표시 토글', () => {
  it('기본은 가려진 상태다', () => {
    render(<Harness />);

    expect(input()).toHaveAttribute('type', 'password');
    expect(toggle()).toHaveAttribute('aria-pressed', 'false');
    expect(toggle()).toHaveAttribute('aria-label', '비밀번호 표시');
  });

  it('토글이 양방향으로 type 을 뒤집고 자신의 aria 상태를 갱신한다', () => {
    render(<Harness />);

    // 숨김 -> 표시
    fireEvent.click(toggle());
    expect(input()).toHaveAttribute('type', 'text');
    expect(toggle()).toHaveAttribute('aria-pressed', 'true');
    // 이름은 "지금 누르면 일어날 일"이어야 한다 — 드러난 상태에서는 '숨기기'.
    expect(toggle()).toHaveAttribute('aria-label', '비밀번호 숨기기');

    // 표시 -> 숨김
    fireEvent.click(toggle());
    expect(input()).toHaveAttribute('type', 'password');
    expect(toggle()).toHaveAttribute('aria-pressed', 'false');
    expect(toggle()).toHaveAttribute('aria-label', '비밀번호 표시');
  });

  it('토글은 폼 안에서 눌러도 폼을 제출하지 않는다', () => {
    const onSubmit = vi.fn((e: React.FormEvent) => e.preventDefault());
    render(
      <I18nProvider>
        <form onSubmit={onSubmit}>
          <PasswordField aria-label="비밀번호 입력" defaultValue="" />
        </form>
      </I18nProvider>,
    );

    // button 의 기본 type 은 submit 이다. type="button" 을 빠뜨리면 여기서 걸린다.
    expect(toggle()).toHaveAttribute('type', 'button');
    fireEvent.click(toggle());
    expect(onSubmit).not.toHaveBeenCalled();
    expect(input()).toHaveAttribute('type', 'text'); // 토글 자체는 동작했다
  });

  it('토글은 키보드 탭 순서에 남는다', () => {
    render(<Harness />);

    // tabIndex={-1} 로 빼면 키보드만 쓰는 사용자가 토글에 도달할 수 없다.
    expect(toggle()).not.toHaveAttribute('tabindex', '-1');
    toggle().focus();
    expect(toggle()).toHaveFocus();
  });

  it('필드 이름을 주면 토글 이름에 붙어 서로 구분된다', () => {
    render(<Harness fieldLabel="새 비밀번호" />);

    expect(toggle()).toHaveAttribute('aria-label', '비밀번호 표시 새 비밀번호');
    fireEvent.click(toggle());
    expect(toggle()).toHaveAttribute('aria-label', '비밀번호 숨기기 새 비밀번호');
  });

  it('언마운트되면 표시 상태가 남지 않는다', () => {
    function Toggleable() {
      const [mounted, setMounted] = useState(true);
      return (
        <I18nProvider>
          <button type="button" onClick={() => setMounted((m) => !m)}>
            전환
          </button>
          {mounted && <PasswordField aria-label="비밀번호 입력" defaultValue="" />}
        </I18nProvider>
      );
    }
    render(<Toggleable />);

    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시' }));
    expect(input()).toHaveAttribute('type', 'text');

    // 닫았다 다시 연다 — 이전 표시 상태가 따라오면 비밀번호가 그대로 노출된다.
    fireEvent.click(screen.getByRole('button', { name: '전환' }));
    fireEvent.click(screen.getByRole('button', { name: '전환' }));

    expect(input()).toHaveAttribute('type', 'password');
    expect(screen.getByRole('button', { name: '비밀번호 표시' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });
});

describe('PasswordField 속성 전달', () => {
  it('autoComplete 를 그대로 넘긴다', () => {
    render(
      <I18nProvider>
        <PasswordField aria-label="비밀번호 입력" autoComplete="new-password" defaultValue="" />
      </I18nProvider>,
    );

    expect(input()).toHaveAttribute('autocomplete', 'new-password');
  });

  it('호출부의 입력 클래스를 유지하면서 토글 자리만 확보한다', () => {
    render(
      <I18nProvider>
        <PasswordField aria-label="비밀번호 입력" className="w-full px-3" defaultValue="" />
      </I18nProvider>,
    );

    // 주변 입력과 같은 모양을 유지해야 하므로 호출부 클래스는 살아 있어야 한다.
    expect(input().className).toContain('w-full');
    // 토글 버튼이 텍스트를 덮지 않도록 우측 여백만 덧붙인다.
    expect(input().className).toContain('pr-10');
  });
});
