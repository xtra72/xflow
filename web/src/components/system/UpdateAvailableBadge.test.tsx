// SPEC-WEB-006 v0.1.0 (M4) — UpdateAvailableBadge 컴포넌트 테스트.
//
// 헤더 우측에 상시 노출되는 작은 아이콘 버튼으로, 업데이트가 있으면 노란 점
// 인디케이터와 툴팁/aria-label 로 알린다. disabled 상태(비관리자)에서는 흐려지고
// 클릭 핸들러가 호출되지 않는다.
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { UpdateAvailableBadge } from './UpdateAvailableBadge';

describe('UpdateAvailableBadge', () => {
  it('아이콘 버튼은 항상 렌더된다 (available=false)', () => {
    render(
      <UpdateAvailableBadge available={false} onClick={() => {}} />,
    );
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('available=false 일 때 점 인디케이터는 보이지 않고 aria-label 은 "최신 버전" 형태', () => {
    render(
      <UpdateAvailableBadge available={false} onClick={() => {}} />,
    );
    const button = screen.getByRole('button');
    expect(button.getAttribute('aria-label')).toBe('시스템 상태 (최신 버전)');
    expect(screen.queryByTestId('update-badge-dot')).not.toBeInTheDocument();
  });

  it('available=true 일 때 노란 점 인디케이터가 보이고 aria-label 에 latestVersion 이 포함된다', () => {
    render(
      <UpdateAvailableBadge
        available={true}
        latestVersion="v1.2.3"
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole('button');
    expect(button.getAttribute('aria-label')).toBe('업데이트 가능: v1.2.3');
    expect(screen.getByTestId('update-badge-dot')).toBeInTheDocument();
  });

  it('available=true + latestVersion=null 일 때 fallback "확인 필요" 사용', () => {
    render(
      <UpdateAvailableBadge
        available={true}
        latestVersion={null}
        onClick={() => {}}
      />,
    );
    const button = screen.getByRole('button');
    expect(button.getAttribute('aria-label')).toBe('업데이트 가능: 확인 필요');
  });

  it('disabled=true 일 때 opacity-50 클래스가 적용되고 aria-label 은 권한 메시지', () => {
    render(
      <UpdateAvailableBadge
        available={false}
        onClick={() => {}}
        disabled={true}
      />,
    );
    const button = screen.getByRole('button');
    expect(button.getAttribute('aria-label')).toBe('관리자 권한 필요');
    expect(button.className).toContain('opacity-50');
    expect(button).toBeDisabled();
  });

  it('onClick 은 disabled=false 일 때 호출된다', () => {
    const handler = vi.fn();
    render(
      <UpdateAvailableBadge available={true} onClick={handler} />,
    );
    fireEvent.click(screen.getByRole('button'));
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('onClick 은 disabled=true 일 때 호출되지 않는다', () => {
    const handler = vi.fn();
    render(
      <UpdateAvailableBadge available={true} onClick={handler} disabled={true} />,
    );
    fireEvent.click(screen.getByRole('button'));
    expect(handler).not.toHaveBeenCalled();
  });

  it('available=false 일 때 title 속성은 "최신 버전입니다"', () => {
    render(
      <UpdateAvailableBadge available={false} onClick={() => {}} />,
    );
    expect(screen.getByRole('button').getAttribute('title')).toBe(
      '최신 버전입니다',
    );
  });

  it('available=true 일 때 title 속성에 새 버전 정보가 포함된다', () => {
    render(
      <UpdateAvailableBadge
        available={true}
        latestVersion="v2.0.0"
        onClick={() => {}}
      />,
    );
    expect(screen.getByRole('button').getAttribute('title')).toBe(
      '새 버전 v2.0.0 사용 가능',
    );
  });

  it('available 토글에 따라 aria-label 이 바뀐다', () => {
    const { rerender } = render(
      <UpdateAvailableBadge available={false} onClick={() => {}} />,
    );
    expect(screen.getByRole('button').getAttribute('aria-label')).toBe(
      '시스템 상태 (최신 버전)',
    );
    rerender(
      <UpdateAvailableBadge
        available={true}
        latestVersion="v3.0.0"
        onClick={() => {}}
      />,
    );
    expect(screen.getByRole('button').getAttribute('aria-label')).toBe(
      '업데이트 가능: v3.0.0',
    );
  });
});
