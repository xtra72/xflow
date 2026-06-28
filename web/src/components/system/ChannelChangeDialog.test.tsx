// SPEC-UPDATE-002 v0.1.0 (M8) — ChannelChangeDialog 컴포넌트 테스트.
//
// 채널 변경 다이얼로그의 렌더링 + 사용자 상호작용 + 결과 알림 사양 테스트.
//
// 테스트 범위:
//   - open=false 시 렌더되지 않음
//   - open=true 시 현재 채널이 미리 선택된 상태로 노출
//   - useChannelInfo 의 available 채널이 dropdown 에 채워짐
//   - 동일 채널 선택 후 확인 → 뮤테이션 미트리거 + onClose 호출
//   - 다른 채널 선택 후 확인 → 뮤테이션 트리거
//   - 성공 시 success 토스트 + (message 가 있으면) info 토스트
//   - 에러 시 mapUpdateError 적용 (unauthorized / channel_invalid)
//   - 취소 버튼 → onClose, 진행 중에는 disabled
//
// systemUpdate 와 uiStore 는 SystemStatusPage.test 와 동일한 mock 패턴을 사용.
//
// @spec SPEC-UPDATE-002 v0.1.0 (M8)

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';

// ─────────────────────────────────────────────────────────────────────
// systemUpdate 훅 mock
// ─────────────────────────────────────────────────────────────────────

const useChannelInfoMock = vi.hoisted(() => vi.fn());
const useChangeChannelMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/systemUpdate', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/api/systemUpdate')
  >('@/services/api/systemUpdate');
  return {
    ...actual,
    useChannelInfo: useChannelInfoMock,
    useChangeChannel: useChangeChannelMock,
  };
});

// uiStore 알림 캡처용 mock.
const addNotificationMock = vi.hoisted(() => vi.fn());

vi.mock('@/stores/uiStore', async () => {
  const actual = await vi.importActual<typeof import('@/stores/uiStore')>(
    '@/stores/uiStore',
  );
  return {
    ...actual,
    useUIStore: Object.assign(
      (
        selector?: (state: {
          addNotification: typeof addNotificationMock;
        }) => unknown,
      ) => {
        const state = { addNotification: addNotificationMock };
        return selector ? selector(state) : state;
      },
      {
        getState: () => ({ addNotification: addNotificationMock }),
      },
    ),
  };
});

import type {
  ChangeChannelResponse,
  Channel,
  ChannelInfo,
} from '@/services/api/systemUpdate';

// i18n: t() 를 ko.json 키 해석으로 모킹해 한국어 단언을 유지한다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<
    string,
    unknown
  >;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) =>
        o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined,
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({
      t: resolve,
      locale: 'ko' as const,
      setLocale: () => {},
    }),
  };
});

import { ChannelChangeDialog } from './ChannelChangeDialog';

// ─────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────

interface ChangeMutationState {
  mutateAsync?: ReturnType<typeof vi.fn>;
  isPending?: boolean;
}

function setChannelInfo(info: ChannelInfo | undefined) {
  useChannelInfoMock.mockReturnValue({
    data: info,
    isLoading: false,
    isError: false,
  });
}

function setChangeMutation(state: ChangeMutationState = {}) {
  useChangeChannelMock.mockReturnValue({
    mutateAsync:
      state.mutateAsync ??
      vi.fn(async () => {
        const noop: ChangeChannelResponse = {
          previous: 'stable',
          current: 'stable',
          check_result: null,
        };
        return noop;
      }),
    isPending: state.isPending ?? false,
  });
}

beforeEach(() => {
  useChannelInfoMock.mockReset();
  useChangeChannelMock.mockReset();
  addNotificationMock.mockReset();

  setChannelInfo({
    current: 'stable',
    available: ['stable', 'beta', 'nightly'],
  });
  setChangeMutation();
});

afterEach(() => {
  vi.useRealTimers();
});

// ─────────────────────────────────────────────────────────────────────
// 1. open=false → 렌더되지 않음
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — open=false', () => {
  it('open=false 면 다이얼로그가 렌더되지 않는다', () => {
    render(
      <ChannelChangeDialog
        open={false}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    expect(
      screen.queryByTestId('channel-change-dialog'),
    ).not.toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 2. 현재 채널 pre-select
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 현재 채널 pre-select', () => {
  it('open=true 시 currentChannel 이 dropdown 의 기본 선택값이다', () => {
    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="beta"
      />,
    );

    const select = screen.getByTestId('channel-select') as HTMLSelectElement;
    expect(select.value).toBe('beta');
  });
});

// ─────────────────────────────────────────────────────────────────────
// 3. dropdown options
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — dropdown options', () => {
  it('useChannelInfo 의 available 채널 enum 이 모두 옵션으로 노출된다', () => {
    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    const select = screen.getByTestId('channel-select');
    const options = Array.from(select.querySelectorAll('option')).map(
      (o) => o.value,
    );
    expect(options).toEqual(['stable', 'beta', 'nightly']);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 4. 동일 채널 → mutation 미트리거
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 동일 채널 선택', () => {
  // 동일 채널 = 변경 불필요. UI 는 두 단계로 보호한다:
  //   (a) confirm 버튼이 disabled → 사용자 클릭 무효화.
  //   (b) handleConfirm 내부 가드 → 만약 disabled 우회되어도 mutation 미호출.
  // 본 테스트는 (a) UI guard 를 검증한다. (b) 로직 가드는 다른 테스트에서
  // 확인 가능 (이벤트 핸들러 직접 호출은 RTL 가이드라인에 어긋나므로 제외).
  it('동일 채널 상태에서는 confirm 버튼이 disabled 되어 mutation 이 호출되지 않는다', () => {
    const mutateAsync = vi.fn();
    setChangeMutation({ mutateAsync });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    const btn = screen.getByTestId('channel-dialog-confirm');
    expect(btn).toBeDisabled();
    // RTL fireEvent.click 은 disabled 시 React onClick 을 호출하지 않는다.
    fireEvent.click(btn);
    expect(mutateAsync).not.toHaveBeenCalled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 5. 다른 채널 → mutation 트리거
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 다른 채널 선택', () => {
  it('다른 채널을 선택 후 확인하면 mutateAsync 가 새 채널과 함께 호출된다', async () => {
    const response: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: null,
    };
    const mutateAsync = vi.fn(async () => response);
    setChangeMutation({ mutateAsync });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' as Channel },
    });
    fireEvent.click(screen.getByTestId('channel-dialog-confirm'));

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({ channel: 'beta' });
    });
  });
});

// ─────────────────────────────────────────────────────────────────────
// 6. 성공 → success 알림 + close
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 성공', () => {
  it('mutation 성공 시 success 알림 발송 + onClose 호출', async () => {
    const response: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: null,
    };
    const mutateAsync = vi.fn(async () => response);
    setChangeMutation({ mutateAsync });
    const onClose = vi.fn();

    render(
      <ChannelChangeDialog
        open={true}
        onClose={onClose}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('channel-dialog-confirm'));

    await waitFor(() => {
      expect(addNotificationMock).toHaveBeenCalled();
      expect(onClose).toHaveBeenCalled();
    });

    const successCall = addNotificationMock.mock.calls.find(
      (c) => (c[0] as { type: string }).type === 'success',
    );
    expect(successCall).toBeDefined();
    expect(
      (successCall![0] as { message: string }).message,
    ).toMatch(/stable.*beta/);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 7. 성공 + message → 추가 info 알림
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — message 안내', () => {
  it('응답 message 가 있으면 추가로 info 알림이 발송된다', async () => {
    const response: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: null,
      message: 'yaml 영구 저장은 CLI 사용',
    };
    const mutateAsync = vi.fn(async () => response);
    setChangeMutation({ mutateAsync });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('channel-dialog-confirm'));

    await waitFor(() => {
      const infoCall = addNotificationMock.mock.calls.find(
        (c) => (c[0] as { type: string }).type === 'info',
      );
      expect(infoCall).toBeDefined();
      expect((infoCall![0] as { message: string }).message).toMatch(
        /yaml 영구 저장/,
      );
    });
  });
});

// ─────────────────────────────────────────────────────────────────────
// 8. 에러 → mapUpdateError 알림
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 에러 매핑', () => {
  it('mutation 에러 시 mapUpdateError 결과로 toast 알림이 발송된다', async () => {
    const err = new APIError('UNAUTHORIZED', 'unauthorized', 401);
    const mutateAsync = vi.fn(async () => {
      throw err;
    });
    setChangeMutation({ mutateAsync });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('channel-dialog-confirm'));

    await waitFor(() => {
      const errorCall = addNotificationMock.mock.calls.find(
        (c) => (c[0] as { type: string }).type === 'error',
      );
      expect(errorCall).toBeDefined();
      // mapUpdateError 의 unauthorized → "권한이 없습니다"
      expect((errorCall![0] as { message: string }).message).toMatch(
        /권한이 없습니다/,
      );
    });
  });

  it('400 channel_invalid 에러는 한글로 매핑된다', async () => {
    const err = new APIError(
      'CHANNEL_INVALID',
      'invalid channel',
      400,
    );
    const mutateAsync = vi.fn(async () => {
      throw err;
    });
    setChangeMutation({ mutateAsync });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('channel-dialog-confirm'));

    await waitFor(() => {
      const errorCall = addNotificationMock.mock.calls.find(
        (c) => (c[0] as { type: string }).type === 'error',
      );
      expect(errorCall).toBeDefined();
      expect((errorCall![0] as { message: string }).message).toMatch(
        /채널/,
      );
    });
  });
});

// ─────────────────────────────────────────────────────────────────────
// 9. 취소 버튼
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 취소', () => {
  it('취소 버튼 클릭 시 mutation 호출 없이 onClose 만 호출된다', () => {
    const mutateAsync = vi.fn();
    setChangeMutation({ mutateAsync });
    const onClose = vi.fn();

    render(
      <ChannelChangeDialog
        open={true}
        onClose={onClose}
        currentChannel="stable"
      />,
    );

    fireEvent.click(screen.getByTestId('channel-dialog-cancel'));

    expect(mutateAsync).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 10. isPending 상태
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — isPending', () => {
  it('mutation 진행 중에는 confirm + cancel + select 가 disabled 된다', () => {
    setChangeMutation({ isPending: true });

    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    expect(screen.getByTestId('channel-dialog-confirm')).toBeDisabled();
    expect(screen.getByTestId('channel-dialog-cancel')).toBeDisabled();
    expect(screen.getByTestId('channel-select')).toBeDisabled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 11. 동일 채널이면 confirm 버튼이 disabled
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — 동일 채널 disabled', () => {
  it('새 채널이 현재 채널과 동일하면 confirm 버튼이 disabled 이다', () => {
    render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    // 현재 = stable, select 도 stable → 버튼 disabled.
    expect(screen.getByTestId('channel-dialog-confirm')).toBeDisabled();

    // beta 로 바꾸면 enabled.
    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    expect(screen.getByTestId('channel-dialog-confirm')).not.toBeDisabled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// 12. open 토글 시 selection 리셋
// ─────────────────────────────────────────────────────────────────────

describe('ChannelChangeDialog — open 토글 시 selection 리셋', () => {
  it('다이얼로그가 다시 열릴 때 currentChannel 로 selection 이 초기화된다', () => {
    const { rerender } = render(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    fireEvent.change(screen.getByTestId('channel-select'), {
      target: { value: 'beta' },
    });
    expect((screen.getByTestId('channel-select') as HTMLSelectElement).value).toBe(
      'beta',
    );

    // 닫기.
    rerender(
      <ChannelChangeDialog
        open={false}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    // 다시 열기 — currentChannel='stable' 로 리셋되어야 한다.
    rerender(
      <ChannelChangeDialog
        open={true}
        onClose={vi.fn()}
        currentChannel="stable"
      />,
    );

    expect((screen.getByTestId('channel-select') as HTMLSelectElement).value).toBe(
      'stable',
    );
  });
});
