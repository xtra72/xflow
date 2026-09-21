// 유휴 경고 화면의 계약 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 로케일 둘을 모두 본다. 기본 로케일이 ko 라 한쪽만 보면 en 쪽 치환자 누락이
// 초록으로 통과한다 — 이 저장소에서 이미 한 번 겪은 함정이다.

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import IdleLogoutGuard from './IdleLogoutGuard';
import { formatRemaining, type IdlePhase } from '@/lib/idle/idlePolicy';

const extendMock = vi.hoisted(() => vi.fn());
const idleState = vi.hoisted(() => ({
  phase: 'warning' as IdlePhase,
  remainingMs: 45_000,
  setting: { enabled: true, timeoutMinutes: 10 },
}));

vi.mock('@/hooks/useIdleLogout', () => ({
  useIdleLogout: () => ({ ...idleState, extend: extendMock }),
  IDLE_LOGOUT_QUERY_KEY: ['settings', 'idle-logout'],
  IDLE_ACTIVITY_STORAGE_KEY: 'xflow_idle_last_activity',
}));

function renderGuard(locale: 'ko' | 'en') {
  localStorage.setItem('xflow-locale', locale);
  return render(
    <I18nProvider>
      <IdleLogoutGuard />
    </I18nProvider>,
  );
}

beforeEach(() => {
  extendMock.mockReset();
  idleState.phase = 'warning';
  idleState.remainingMs = 45_000;
  idleState.setting = { enabled: true, timeoutMinutes: 10 };
});

afterEach(() => {
  localStorage.clear();
});

describe('formatRemaining — 남은 시간 표기', () => {
  it('분:초로 적고 초는 두 자리로 채운다', () => {
    expect(formatRemaining(45_000)).toBe('0:45');
    expect(formatRemaining(60_000)).toBe('1:00');
    expect(formatRemaining(65_400)).toBe('1:06');
  });

  it('0 이하는 0:00 이다 — 음수를 화면에 내보내지 않는다', () => {
    expect(formatRemaining(0)).toBe('0:00');
    expect(formatRemaining(-5_000)).toBe('0:00');
  });
});

describe('IdleLogoutGuard — 경고 화면', () => {
  it('경고 국면이 아니면 아무것도 그리지 않는다', () => {
    idleState.phase = 'active';
    renderGuard('ko');
    expect(screen.queryByTestId('idle-logout-warning')).toBeNull();
  });

  it("'계속 사용'을 누르면 연장을 부른다", () => {
    renderGuard('ko');
    fireEvent.click(screen.getByRole('button'));
    expect(extendMock).toHaveBeenCalledTimes(1);
  });

  it.each(['ko', 'en'] as const)('%s 로케일에서 치환자가 남지 않는다', (locale) => {
    renderGuard(locale);
    const dialog = screen.getByTestId('idle-logout-warning');
    const text = dialog.textContent ?? '';

    // 원문 키도, 벌거벗은 치환자도 화면에 남으면 안 된다.
    expect(text).not.toContain('auth.idleLogout');
    expect(text).not.toContain('{minutes}');
    expect(text).not.toContain('{remaining}');
    // 실제 값이 박혔는지 확인한다.
    expect(text).toContain('10');
    expect(text).toContain('0:45');
  });
});
