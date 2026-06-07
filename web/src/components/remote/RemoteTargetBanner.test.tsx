// RemoteTargetBanner 테스트 (SPEC-REMOTE-001 M8, REQ-J13).

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import { LOCAL_TARGET } from '@/lib/remote/target';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { RemoteTargetBanner } from './RemoteTargetBanner';

function renderBanner(props: Parameters<typeof RemoteTargetBanner>[0]) {
  return render(
    <MemoryRouter>
      <RemoteTargetBanner {...props} />
    </MemoryRouter>,
  );
}

describe('RemoteTargetBanner', () => {
  it('로컬 타깃이면 아무것도 렌더링하지 않는다', () => {
    const { container } = renderBanner({
      target: LOCAL_TARGET,
      nodeReady: true,
      localHref: '/flows',
    });
    expect(container.firstChild).toBeNull();
  });

  it('원격 타깃이면 노드 라벨과 로컬 복귀 링크를 표시한다', () => {
    renderBanner({
      target: { type: 'remote', instanceId: 'node-a' },
      nodeLabel: 'gw-1',
      nodeReady: true,
      localHref: '/flows',
    });
    expect(screen.getByTestId('remote-target-banner')).toBeInTheDocument();
    expect(screen.getByText('gw-1')).toBeInTheDocument();
    expect(screen.getByTestId('remote-target-exit')).toHaveAttribute('href', '/flows');
    // 노드 ready 면 제어 비활성 경고를 표시하지 않는다.
    expect(screen.queryByText('remote.target.controlDisabled')).not.toBeInTheDocument();
  });

  it('노드가 ready 아니면 제어 비활성 경고를 표시한다', () => {
    renderBanner({
      target: { type: 'remote', instanceId: 'node-a' },
      nodeLabel: 'gw-1',
      nodeReady: false,
      localHref: '/flows',
    });
    expect(
      screen.getByText((content) => content.includes('remote.target.controlDisabled')),
    ).toBeInTheDocument();
  });

  it('nodeLabel 미지정 시 instanceId 를 표시한다', () => {
    renderBanner({
      target: { type: 'remote', instanceId: 'node-xyz' },
      nodeReady: true,
      localHref: '/flows',
    });
    expect(screen.getByText('node-xyz')).toBeInTheDocument();
  });
});
