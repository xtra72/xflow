// UpdateSourceSettings 테스트 (서버 저장 업데이트 소스 편집).
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { UpdateSource } from '@/types/remote';

const setSourceMutate = vi.hoisted(() => vi.fn());
const sourceData = vi.hoisted(() => ({ current: { update_url: '', channel: '' } as UpdateSource }));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateSource: () => ({ data: sourceData.current }),
  useSetUpdateSource: () => ({ mutate: setSourceMutate, isPending: false }),
}));

const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { UpdateSourceSettings } from './UpdateSourceSettings';

function renderPanel(): void {
  render(
    <I18nProvider>
      <UpdateSourceSettings />
    </I18nProvider>,
  );
}

beforeEach(() => {
  setSourceMutate.mockReset();
  addNotificationMock.mockReset();
  sourceData.current = { update_url: '', channel: '' };
});

describe('UpdateSourceSettings', () => {
  it('기본(빈) 소스는 GitHub 으로 표시한다', () => {
    renderPanel();
    expect(screen.getByTestId('update-source-current')).toHaveTextContent('GitHub');
  });

  it('자체 호스팅 소스는 URL 을 표시한다', () => {
    sourceData.current = { update_url: 'https://dl.example.com/xflow', channel: 'beta' };
    renderPanel();
    expect(screen.getByTestId('update-source-current')).toHaveTextContent('https://dl.example.com/xflow');
    expect(screen.getByTestId('update-source-current')).toHaveTextContent('beta');
  });

  it('GitHub 선택 시 기본 URL 로 저장한다', () => {
    renderPanel();
    fireEvent.click(screen.getByTestId('update-source-edit'));
    fireEvent.click(screen.getByTestId('update-source-save'));
    expect(setSourceMutate).toHaveBeenCalledTimes(1);
    const [arg] = setSourceMutate.mock.calls[0] as [UpdateSource];
    expect(arg.update_url).toBe('https://api.github.com/repos/xtra72/xflow');
  });

  it('자체 호스팅 https URL 을 저장한다', () => {
    renderPanel();
    fireEvent.click(screen.getByTestId('update-source-edit'));
    fireEvent.click(screen.getByTestId('update-source-kind-self'));
    fireEvent.change(screen.getByTestId('update-source-url'), {
      target: { value: 'https://dl.example.com/xflow' },
    });
    fireEvent.change(screen.getByTestId('update-source-channel'), { target: { value: 'nightly' } });
    fireEvent.click(screen.getByTestId('update-source-save'));
    expect(setSourceMutate).toHaveBeenCalledTimes(1);
    const [arg] = setSourceMutate.mock.calls[0] as [UpdateSource];
    expect(arg.update_url).toBe('https://dl.example.com/xflow');
    expect(arg.channel).toBe('nightly');
  });

  it('비-https URL 은 저장하지 않고 오류를 알린다', () => {
    renderPanel();
    fireEvent.click(screen.getByTestId('update-source-edit'));
    fireEvent.click(screen.getByTestId('update-source-kind-self'));
    fireEvent.change(screen.getByTestId('update-source-url'), {
      target: { value: 'http://insecure.example.com' },
    });
    fireEvent.click(screen.getByTestId('update-source-save'));
    expect(setSourceMutate).not.toHaveBeenCalled();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'error' }),
    );
  });

  it('"이 관리 서버" 선택 시 origin 의 릴리스 피드 URL(https 강제)로 저장한다', () => {
    renderPanel();
    fireEvent.click(screen.getByTestId('update-source-edit'));
    fireEvent.click(screen.getByTestId('update-source-kind-this'));
    // 피드 URL 미리보기 패널이 노출된다.
    const info = screen.getByTestId('update-source-this-info');
    expect(info).toHaveTextContent('/api/v1/updates');
    expect(info).toHaveTextContent('https://');
    fireEvent.click(screen.getByTestId('update-source-save'));
    expect(setSourceMutate).toHaveBeenCalledTimes(1);
    const [arg] = setSourceMutate.mock.calls[0] as [UpdateSource];
    // jsdom origin(http://localhost:3000) → https 강제.
    expect(arg.update_url).toBe(
      `${window.location.origin.replace(/^http:\/\//, 'https://')}/api/v1/updates`,
    );
    expect(arg.update_url.startsWith('https://')).toBe(true);
    expect(arg.update_url.endsWith('/api/v1/updates')).toBe(true);
  });

  it('현재 소스가 이 관리 서버 피드면 "이 관리 서버"로 표시한다', () => {
    const feed = `${window.location.origin.replace(/^http:\/\//, 'https://')}/api/v1/updates`;
    sourceData.current = { update_url: feed, channel: 'stable' };
    renderPanel();
    expect(screen.getByTestId('update-source-current')).toHaveTextContent('이 관리 서버');
  });
});
