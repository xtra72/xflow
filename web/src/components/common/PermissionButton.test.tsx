// PermissionButton 단위 테스트 (SPEC-AUTH-006 E1 / AC-07).
//
// AC-07 의 핵심은 두 가지다 — 권한이 없으면 숨김이 아니라 비활성 + aria-disabled +
// 사유 툴팁이고, 권한을 추가로 부여하면 해당 컨트롤만 활성화되고 나머지는 비활성
// 으로 남는다.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

const permissionMock = vi.hoisted(() => ({ granted: new Set<string>() }));
vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => permissionMock.granted.has(key),
    hasAnyPermission: (keys: readonly string[]) =>
      keys.some((k) => permissionMock.granted.has(k)),
    isPermissionUnavailable: false,
  }),
}));

import PermissionButton from './PermissionButton';

function renderButtons(onClick = vi.fn()) {
  render(
    <I18nProvider>
      <PermissionButton permission="agent.create" onClick={onClick}>
        등록
      </PermissionButton>
      <PermissionButton permission="agent.update" onClick={onClick}>
        수정
      </PermissionButton>
      <PermissionButton permission="agent.delete" onClick={onClick}>
        삭제
      </PermissionButton>
      <PermissionButton permission="agent.execute" onClick={onClick}>
        시작
      </PermissionButton>
    </I18nProvider>,
  );
  return onClick;
}

beforeEach(() => {
  permissionMock.granted = new Set<string>();
});

describe('PermissionButton — 권한 부족 시 비활성 (AC-07)', () => {
  it('agent.read 만 가진 사용자에게 등록·수정·삭제·시작이 모두 비활성이다', () => {
    permissionMock.granted = new Set(['agent.read']);
    renderButtons();

    for (const label of ['등록', '수정', '삭제', '시작']) {
      const btn = screen.getByRole('button', { name: label });
      expect(btn).toBeDisabled();
      expect(btn).toHaveAttribute('aria-disabled', 'true');
    }
  });

  it('숨기지 않는다 — 컨트롤은 DOM 에 남아 있다', () => {
    permissionMock.granted = new Set(['agent.read']);
    renderButtons();
    expect(screen.getByRole('button', { name: '삭제' })).toBeInTheDocument();
  });

  it('비활성 컨트롤은 사유 툴팁을 가진다', () => {
    permissionMock.granted = new Set(['agent.read']);
    renderButtons();
    expect(screen.getByRole('button', { name: '삭제' })).toHaveAttribute(
      'title',
      '권한이 필요합니다',
    );
  });

  it('권한이 없으면 클릭해도 onClick 이 호출되지 않는다', () => {
    permissionMock.granted = new Set(['agent.read']);
    const onClick = renderButtons();
    fireEvent.click(screen.getByRole('button', { name: '삭제' }));
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe('PermissionButton — 권한 추가 부여 (AC-07 후단)', () => {
  it('agent.execute 를 추가하면 시작만 활성화되고 등록·수정·삭제는 비활성으로 남는다', () => {
    permissionMock.granted = new Set(['agent.read', 'agent.execute']);
    renderButtons();

    expect(screen.getByRole('button', { name: '시작' })).toBeEnabled();
    for (const label of ['등록', '수정', '삭제']) {
      expect(screen.getByRole('button', { name: label })).toBeDisabled();
    }
  });

  it('권한이 있으면 클릭이 전달된다', () => {
    permissionMock.granted = new Set(['agent.execute']);
    const onClick = renderButtons();
    fireEvent.click(screen.getByRole('button', { name: '시작' }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});

describe('PermissionButton — 기존 비활성 사유와의 합성', () => {
  it('권한이 있어도 호출부가 disabled 를 주면 비활성이고 원래 툴팁을 유지한다', () => {
    permissionMock.granted = new Set(['agent.execute']);
    render(
      <I18nProvider>
        <PermissionButton permission="agent.execute" disabled title="실행 중">
          시작
        </PermissionButton>
      </I18nProvider>,
    );
    const btn = screen.getByRole('button', { name: '시작' });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute('title', '실행 중');
  });

  it('권한이 없으면 호출부 툴팁 대신 권한 사유를 보여준다', () => {
    permissionMock.granted = new Set<string>();
    render(
      <I18nProvider>
        <PermissionButton permission="agent.execute" title="시작">
          시작
        </PermissionButton>
      </I18nProvider>,
    );
    expect(screen.getByRole('button', { name: '시작' })).toHaveAttribute(
      'title',
      '권한이 필요합니다',
    );
  });
});
