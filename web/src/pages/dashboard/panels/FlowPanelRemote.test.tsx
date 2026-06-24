// FlowPanel 원격 target 데이터 소스 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L04).
//
// 검증:
//   - 원격 target(TargetProvider) 하에서 FlowPanel 은 useFlowsTarget(미러)을
//     소스로 쓰고 prop 의 flows 는 무시한다.
//   - 로컬(TargetProvider 없음) 하에서는 prop 의 flows 를 그대로 렌더한다(회귀 없음).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const useFlowsTargetMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useResourceTargets', () => ({
  useFlowsTarget: useFlowsTargetMock,
}));
// 액션/게이팅은 원격 분기에서 호출되므로 단순 스텁.
vi.mock('@/hooks/useResourceActions', () => ({
  useFlowActionsTarget: () => ({
    isRemote: true,
    supports: () => true,
    perform: vi.fn().mockResolvedValue(undefined),
    pending: {},
  }),
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: true,
    nodeReady: true,
    nodeLabel: 'host',
    canControl: () => true,
  }),
}));
// i18n: 키를 그대로 반환하는 스텁 (단언은 데이터 텍스트만 검사하므로 무관).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { TargetProvider } from '@/lib/remote/TargetContext';
import { LOCAL_TARGET } from '@/lib/remote/target';
import type { FlowInfo } from '@/types/flow';

import FlowPanel from './FlowPanel';

function flow(id: string, name: string): FlowInfo {
  return { id, name, status: 'running', node_count: 1, config: {} };
}

function renderPanel(target: 'local' | 'remote', flows: FlowInfo[]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <TargetProvider
          target={target === 'remote' ? { type: 'remote', instanceId: 'node-1' } : LOCAL_TARGET}
        >
          <FlowPanel flows={flows} />
        </TargetProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  useFlowsTargetMock.mockReset();
});

describe('FlowPanel — 원격 target 데이터 소스', () => {
  it('원격 target 이면 useFlowsTarget(미러)을 소스로 쓴다(prop flows 무시)', () => {
    useFlowsTargetMock.mockReturnValue({
      data: { data: [flow('rf-1', '원격플로우')], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
      isRemote: true,
    });

    renderPanel('remote', [flow('lf-1', '로컬플로우')]);

    expect(screen.getByText('원격플로우')).toBeInTheDocument();
    expect(screen.queryByText('로컬플로우')).not.toBeInTheDocument();
    expect(useFlowsTargetMock).toHaveBeenCalled();
  });

  it('로컬 target 이면 prop 의 flows 를 그대로 렌더한다(회귀 없음)', () => {
    useFlowsTargetMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
      isRemote: false,
    });

    renderPanel('local', [flow('lf-1', '로컬플로우')]);

    expect(screen.getByText('로컬플로우')).toBeInTheDocument();
  });
});
