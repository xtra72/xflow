// SPEC-WEB-006 v0.1.0 (M8) — RestartGuide 단위 테스트.
//
// `ready_to_restart` 상태에서 운영자가 직접 xflowd 를 재시작해야 할 때 안내하는
// 컴포넌트. CLI 명령어 표시 + 클립보드 복사 동작을 검증한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M8)

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { RestartGuide } from './RestartGuide';

// ─────────────────────────────────────────────────────────────────────
// navigator.clipboard mock
// ─────────────────────────────────────────────────────────────────────

const writeTextMock = vi.fn(async (_text: string) => {});

beforeEach(() => {
  writeTextMock.mockReset();
  // jsdom 은 기본 clipboard 미지원 — 매 테스트마다 명시적으로 주입.
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: writeTextMock },
  });
});

afterEach(() => {
  vi.useRealTimers();
});

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

describe('RestartGuide', () => {
  it('재시작 안내 제목과 설명을 표시한다', () => {
    render(<RestartGuide />);
    expect(
      screen.getByTestId('restart-guide-title'),
    ).toHaveTextContent(/재시작/);
  });

  it('CLI 명령어 두 종류(systemd, manual)를 표시한다', () => {
    render(<RestartGuide />);
    const cmds = screen.getAllByTestId(/^restart-guide-cmd-/);
    expect(cmds.length).toBeGreaterThanOrEqual(2);
    expect(screen.getByTestId('restart-guide-cmd-systemd')).toHaveTextContent(
      /systemctl restart/,
    );
    expect(screen.getByTestId('restart-guide-cmd-manual')).toHaveTextContent(
      /xflowd/,
    );
  });

  it('복사 버튼 클릭 시 navigator.clipboard.writeText 가 호출된다', async () => {
    render(<RestartGuide />);
    const copyBtn = screen.getByTestId('restart-guide-copy-systemd');
    fireEvent.click(copyBtn);
    await waitFor(() => {
      expect(writeTextMock).toHaveBeenCalledTimes(1);
    });
    expect(writeTextMock.mock.calls[0]![0]).toMatch(/systemctl restart/);
  });

  it('복사 후 "복사됨" 인라인 표시가 나타난다', async () => {
    render(<RestartGuide />);
    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));
    await waitFor(() => {
      expect(
        screen.getByTestId('restart-guide-copied-systemd'),
      ).toBeInTheDocument();
    });
  });

  it('복사 표시는 3초 후 자동으로 사라진다', async () => {
    // setTimeout 을 spy 해서 ms 인자로 호출되었는지 검증한다.
    // (실제 타이머 진행은 환경 의존성이 크므로, 등록 자체를 검증하는 방식으로
    //  계약을 보장한다.)
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');

    render(<RestartGuide />);
    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));

    await waitFor(() => {
      expect(
        screen.getByTestId('restart-guide-copied-systemd'),
      ).toBeInTheDocument();
    });

    // setTimeout 이 3000 (or larger) 으로 호출되어 3초 reset 을 예약했는지 확인.
    const calls = setTimeoutSpy.mock.calls.filter(
      ([, delay]) => typeof delay === 'number' && delay >= 2900 && delay <= 3500,
    );
    expect(calls.length).toBeGreaterThan(0);

    setTimeoutSpy.mockRestore();
  });

  it('두 복사 버튼은 독립적으로 상태를 가진다', async () => {
    render(<RestartGuide />);
    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));
    await waitFor(() => {
      expect(
        screen.getByTestId('restart-guide-copied-systemd'),
      ).toBeInTheDocument();
    });
    // manual 쪽은 아직 복사되지 않았으므로 표시가 없어야 한다.
    expect(
      screen.queryByTestId('restart-guide-copied-manual'),
    ).not.toBeInTheDocument();
  });

  it('operationId 가 주어지면 카드 영역에 노출된다', () => {
    render(<RestartGuide operationId="op-abc-123" />);
    expect(
      screen.getByTestId('restart-guide-operation-id'),
    ).toHaveTextContent(/op-abc-123/);
  });

  it('clipboard.writeText 실패 시 silent fail (예외 없이 badge 미노출)', async () => {
    writeTextMock.mockRejectedValueOnce(new Error('NotAllowedError'));
    render(<RestartGuide />);
    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));

    // 다음 microtask 까지 진행해도 "복사됨" 표시는 등장하지 않아야 한다.
    await Promise.resolve();
    await Promise.resolve();
    expect(
      screen.queryByTestId('restart-guide-copied-systemd'),
    ).not.toBeInTheDocument();
  });

  it('타이머 만료 시 "복사됨" 배지가 제거된다 (실제 setTimeout 콜백 실행)', async () => {
    // 짧은 TTL 을 흉내내기 위해 setTimeout 의 delay 를 단축한다.
    // (실제 3초를 기다리지 않고 동작을 검증하기 위함.)
    const originalSetTimeout = globalThis.setTimeout.bind(globalThis);
    const setTimeoutSpy = vi
      .spyOn(globalThis, 'setTimeout')
      .mockImplementation(((cb: () => void, delay?: number) => {
        // 3000ms 의 reset 만 50ms 로 단축. 0ms 같은 microtask 단위는 그대로 둔다.
        const useDelay = typeof delay === 'number' && delay > 100 ? 50 : delay;
        return originalSetTimeout(cb, useDelay);
      }) as unknown as typeof setTimeout);

    try {
      render(<RestartGuide />);
      fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));

      // 첫 번째 단계: "복사됨" 배지가 나타나야 한다.
      await waitFor(() => {
        expect(
          screen.getByTestId('restart-guide-copied-systemd'),
        ).toBeInTheDocument();
      });

      // 두 번째 단계: 50ms 단축 타이머 만료 후 배지가 사라져야 한다.
      await waitFor(
        () => {
          expect(
            screen.queryByTestId('restart-guide-copied-systemd'),
          ).not.toBeInTheDocument();
        },
        { timeout: 2000 },
      );
    } finally {
      setTimeoutSpy.mockRestore();
    }
  });

  it('연속 클릭 시 이전 타이머가 취소되고 새 타이머가 등록된다', async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');
    const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout');

    render(<RestartGuide />);
    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));
    await waitFor(() => {
      expect(
        screen.getByTestId('restart-guide-copied-systemd'),
      ).toBeInTheDocument();
    });
    const setTimeoutCallsBefore = setTimeoutSpy.mock.calls.filter(
      ([, delay]) =>
        typeof delay === 'number' && delay >= 2900 && delay <= 3500,
    ).length;

    fireEvent.click(screen.getByTestId('restart-guide-copy-systemd'));
    await waitFor(() => {
      const setTimeoutCallsAfter = setTimeoutSpy.mock.calls.filter(
        ([, delay]) =>
          typeof delay === 'number' && delay >= 2900 && delay <= 3500,
      ).length;
      expect(setTimeoutCallsAfter).toBeGreaterThan(setTimeoutCallsBefore);
    });
    expect(clearTimeoutSpy).toHaveBeenCalled();

    setTimeoutSpy.mockRestore();
    clearTimeoutSpy.mockRestore();
  });
});
