// RemoteControlPage(노드 셀렉터) 테스트 (SPEC-REMOTE-001 M8, REQ-J13/J14).
//
// 승인된 노드를 카드로 나열하고, 각 카드가 `?target=remote:{id}` 쿼리로 로컬
// 페이지(/flows|agents|devices)로 라우팅하는 링크를 노출하는지 검증한다.

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ManagedNode } from '@/types/remote';

const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: useManagedNodesMock,
  useRemoteMode: useRemoteModeMock,
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/components/remote/RemoteNotServerNotice', () => ({
  RemoteNotServerNotice: () => <div data-testid="not-server" />,
}));

import RemoteControlPage from './RemoteControlPage';

function node(o: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '',
    status: 'approved',
    online: true,
    last_seen: 0,
    ...o,
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <RemoteControlPage />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useManagedNodesMock.mockReset().mockReturnValue({ data: [] });
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
});

describe('RemoteControlPage', () => {
  it('승인된 노드가 없으면 빈 상태를 표시한다', () => {
    renderPage();
    expect(screen.getByTestId('remote-control-empty')).toBeInTheDocument();
  });

  it('승인된 노드를 카드로 나열하고 target 쿼리 링크를 노출한다', () => {
    useManagedNodesMock.mockReturnValue({ data: [node()] });
    renderPage();

    expect(screen.getByTestId('remote-control-node-node-a')).toBeInTheDocument();
    // 각 자원 화면으로 가는 링크가 target 쿼리를 포함한다.
    const links = screen.getAllByRole('link');
    const hrefs = links.map((l) => l.getAttribute('href'));
    expect(hrefs).toContain('/flows?target=remote:node-a');
    expect(hrefs).toContain('/agents?target=remote:node-a');
    expect(hrefs).toContain('/devices?target=remote:node-a');
  });

  it('승인되지 않은 노드는 제외한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'pending-1', status: 'pending' })],
    });
    renderPage();
    expect(screen.queryByTestId('remote-control-node-pending-1')).not.toBeInTheDocument();
    expect(screen.getByTestId('remote-control-empty')).toBeInTheDocument();
  });

  it('server 모드가 아니면 안내만 표시하고 노드 쿼리를 막는다', () => {
    useRemoteModeMock.mockReturnValue({ data: { mode: 'client' } });
    renderPage();
    expect(screen.getByTestId('not-server')).toBeInTheDocument();
    // 비-server 면 enabled=false 로 노드 쿼리를 막는다.
    expect(useManagedNodesMock).toHaveBeenCalledWith(undefined, false);
  });
});
