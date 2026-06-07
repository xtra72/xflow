// FlowListPage — hideRemoteBanner 게이팅 테스트 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K14).
//
// 범위:
//   - 단독 원격 사용(`?target=` 딥링크 또는 target prop, hideRemoteBanner 미지정):
//     RemoteTargetBanner 를 렌더한다(회귀 없음, REQ-J13).
//   - 노드 대시보드 임베드(hideRemoteBanner=true): 배너를 렌더하지 않는다(중복 제거).
//
// 페이지의 데이터/게이팅 훅과 무거운 하위 컴포넌트는 스텁으로 격리하고, 배너 분기
// 자체만 검증한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ResourceTarget } from '@/lib/remote/target';

const REMOTE_TARGET: ResourceTarget = { type: 'remote', instanceId: 'node-a' };

// 데이터 소스: 빈 목록(테이블 빈 상태 경로로 단순화).
vi.mock('@/hooks/useResourceTargets', () => ({
  useFlowsTarget: () => ({
    data: { data: [] },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

// 게이팅: 원격 라벨/준비 상태.
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({ nodeLabel: 'gw-1', nodeReady: true }),
}));

// URL `?target=` 파라미터(미지정 시 로컬). prop 우선 경로를 검증하므로 로컬 반환.
vi.mock('@/hooks/useTargetParam', () => ({
  useTargetParam: () => ({ type: 'local' }),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 배너는 실제 컴포넌트의 분기(로컬이면 null)와 무관하게 "렌더 호출 여부"만
// 검증하기 위해 단순 스텁으로 대체한다(원격 타깃이 주어지면 항상 그린다).
vi.mock('@/components/remote/RemoteTargetBanner', () => ({
  RemoteTargetBanner: () => <div data-testid="remote-target-banner" />,
}));

// TargetProvider 는 컨텍스트만 제공하므로 통과 스텁.
vi.mock('@/lib/remote/TargetContext', () => ({
  TargetProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// 무거운 하위 컴포넌트는 빈 스텁으로 격리.
vi.mock('@/components/common/ImportDialog', () => ({ default: () => null }));
vi.mock('@/pages/dashboard/CreateFlowModal', () => ({ default: () => null }));
vi.mock('./FlowSearchFilter', () => ({ default: () => null }));
vi.mock('./FlowActionMenu', () => ({ default: () => null }));
vi.mock('./FlowDetailPanel', () => ({ default: () => null }));

import FlowListPage from './FlowListPage';

function renderPage(props: { target?: ResourceTarget; hideRemoteBanner?: boolean }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <FlowListPage {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('FlowListPage — 원격 배너 게이팅', () => {
  it('단독 원격 사용(hideRemoteBanner 미지정)에서는 배너를 렌더한다', () => {
    renderPage({ target: REMOTE_TARGET });
    expect(screen.getByTestId('remote-target-banner')).toBeInTheDocument();
  });

  it('hideRemoteBanner=true(대시보드 임베드)에서는 배너를 렌더하지 않는다', () => {
    renderPage({ target: REMOTE_TARGET, hideRemoteBanner: true });
    expect(screen.queryByTestId('remote-target-banner')).not.toBeInTheDocument();
  });
});
