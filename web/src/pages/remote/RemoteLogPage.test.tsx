// 원격 관리 로그 페이지의 계약 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 여기서 고정하는 것은 **조건이 서버로 내려가는가** 이다. 정렬·필터를 화면에서 하면
// 받아 온 쪽 안에서만 적용되어 결과가 전체를 대표하지 않는다.

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import RemoteLogPage from './RemoteLogPage';
import type { RemoteLogEntry } from '@/types/remote';

const listLogsMock = vi.hoisted(() => vi.fn());
const listNodesMock = vi.hoisted(() => vi.fn());
// 모듈 전체를 갈아치우면 useRemote 가 모듈 적재 시점에 참조하는 다른 함수들이
// 사라져 수집조차 되지 않는다. 원본을 펼친 뒤 필요한 셋만 바꾼다.
vi.mock('@/services/api/remoteService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/api/remoteService')>()),
  listRemoteLogs: listLogsMock,
  listNodes: listNodesMock,
  getRemoteMode: () => Promise.resolve({ mode: 'server' }),
}));

function entry(over: Partial<RemoteLogEntry> = {}): RemoteLogEntry {
  return {
    id: 1,
    instance_id: 'node-a',
    actor: 'admin',
    action: 'connect',
    result: 'ok',
    timestamp: Date.parse('2026-09-20T09:00:00Z'),
    ...over,
  };
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>
        <RemoteLogPage />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

/** 마지막 조회에 실린 조건. */
function lastQuery() {
  return listLogsMock.mock.calls.at(-1)?.[0] ?? {};
}

beforeEach(() => {
  localStorage.clear();
  listLogsMock.mockReset();
  listNodesMock.mockReset();
  listLogsMock.mockResolvedValue({ entries: [entry()], total: 120 });
  listNodesMock.mockResolvedValue([
    { instance_id: 'node-a', hostname: 'xagent03', version: 'v', status: 'approved', online: true, last_seen: 1 },
  ]);
});

describe('RemoteLogPage — 조건이 서버로 내려간다', () => {
  it('기본은 최신순 첫 쪽이다', async () => {
    renderPage();
    await waitFor(() => expect(listLogsMock).toHaveBeenCalled());
    expect(lastQuery()).toMatchObject({ sort: 'ts', asc: false, offset: 0 });
  });

  it('사건 필터를 고르면 그 값이 조회에 실린다', async () => {
    renderPage();
    await waitFor(() => expect(listLogsMock).toHaveBeenCalled());

    fireEvent.change(screen.getByTestId('remote-log-action-filter'), {
      target: { value: 'access' },
    });
    await waitFor(() => expect(lastQuery().action).toBe('access'));
  });

  it('헤더를 누르면 정렬 기준이 조회에 실린다', async () => {
    renderPage();
    await waitFor(() => expect(screen.getByTestId('remote-log-table')).toBeInTheDocument());

    fireEvent.click(screen.getByText('수행'));
    await waitFor(() => expect(lastQuery()).toMatchObject({ sort: 'actor', asc: true }));
  });

  it('쪽당 줄 수를 바꾸면 limit 이 따라간다', async () => {
    renderPage();
    await waitFor(() => expect(listLogsMock).toHaveBeenCalled());

    fireEvent.change(screen.getByLabelText('페이지당'), { target: { value: '200' } });
    await waitFor(() => expect(lastQuery().limit).toBe(200));
  });

  it('필터를 바꾸면 첫 쪽으로 돌아간다 — 3쪽에서 거르면 빈 화면에 선다', async () => {
    renderPage();
    await waitFor(() => expect(listLogsMock).toHaveBeenCalled());

    fireEvent.click(screen.getByLabelText('다음 페이지'));
    await waitFor(() => expect(lastQuery().offset).toBe(50));

    fireEvent.change(screen.getByTestId('remote-log-action-filter'), {
      target: { value: 'connect' },
    });
    await waitFor(() => expect(lastQuery().offset).toBe(0));
  });
});

describe('RemoteLogPage — 노드 표기', () => {
  /** 표 안의 글자만 본다 — 같은 이름이 필터 목록에도 있어 화면 전체로 찾으면 겹친다. */
  function tableText(): string {
    return screen.getByTestId('remote-log-table').textContent ?? '';
  }

  it('기본은 이름이고, 전환하면 id 로 바뀐다', async () => {
    renderPage();
    await waitFor(() => expect(tableText()).toContain('xagent03'));

    fireEvent.click(screen.getByTestId('remote-log-node-view-toggle'));
    await waitFor(() => expect(tableText()).toContain('node-a'));
    expect(tableText()).not.toContain('xagent03');
  });

  it('전환 선택은 브라우저에 남는다', async () => {
    const { unmount } = renderPage();
    await waitFor(() => expect(tableText()).toContain('xagent03'));
    fireEvent.click(screen.getByTestId('remote-log-node-view-toggle'));
    await waitFor(() => expect(tableText()).toContain('node-a'));
    unmount();

    renderPage();
    await waitFor(() => expect(tableText()).toContain('node-a'));
  });
});
