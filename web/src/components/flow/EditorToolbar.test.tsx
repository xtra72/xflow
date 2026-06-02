// EditorToolbar 의 플로우 전환 선택기(FlowSwitcher) 테스트.
//
// 검증 대상:
//   - 현재 플로우 이름 렌더링 (FlowNameEditor)
//   - chevron 버튼 클릭 시 전체 플로우 목록 드롭다운 표시
//   - 다른 플로우 선택 시 navigate('/editor/{id}') 호출
//   - 같은(현재) 플로우 선택 시 navigate 미호출
//   - 인라인 이름 편집(FlowNameEditor) 동작 유지 — 클릭 시 input 으로 전환

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type { FlowInfo } from '@/types/flow';

// react-router 의 useNavigate 만 모킹한다.
const navigateMock = vi.fn();
vi.mock('react-router', () => ({
  useNavigate: () => navigateMock,
}));

// 플로우 훅 모킹. 전환 선택기와 이름 편집에 필요한 최소 동작만 제공한다.
const updateMutate = vi.fn();
const flowsList: FlowInfo[] = [
  { id: 'flow-1', name: '플로우 하나', status: 'stored', node_count: 0 },
  { id: 'flow-2', name: '플로우 둘', status: 'stored', node_count: 0 },
  { id: 'flow-3', name: '플로우 셋', status: 'stored', node_count: 0 },
];

vi.mock('@/hooks/useFlow', () => ({
  useFlow: (id: string) => ({
    data: flowsList.find((f) => f.id === id) ?? null,
  }),
  useFlows: () => ({ data: { data: flowsList, total: flowsList.length } }),
  useFlowStatus: () => ({ data: { status: 'stored' } }),
  useUpdateFlow: () => ({ mutate: updateMutate, isPending: false }),
  useDeployFlow: () => ({ mutate: vi.fn(), isPending: false }),
  useStartFlow: () => ({ mutate: vi.fn(), isPending: false }),
  useStopFlow: () => ({ mutate: vi.fn(), isPending: false }),
  useRestartFlow: () => ({ mutate: vi.fn(), isPending: false }),
}));

// FlowSettingsDialog 는 이 테스트와 무관하므로 가벼운 stub 으로 대체한다.
vi.mock('./FlowSettingsDialog', () => ({
  FlowSettingsDialog: () => null,
}));

import { EditorToolbar } from './EditorToolbar';

describe('EditorToolbar - 플로우 전환 선택기', () => {
  beforeEach(() => {
    navigateMock.mockClear();
    updateMutate.mockClear();
  });

  it('현재 플로우 이름을 렌더링한다', () => {
    render(<EditorToolbar flowId="flow-1" />);
    expect(screen.getByText('플로우 하나')).toBeInTheDocument();
  });

  it('전환 버튼 클릭 시 전체 플로우 목록을 표시한다', () => {
    render(<EditorToolbar flowId="flow-1" />);

    // 초기에는 다른 플로우 항목이 보이지 않는다.
    expect(screen.queryByText('플로우 둘')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));

    // 목록(listbox)에 모든 플로우가 표시된다.
    const listbox = screen.getByRole('listbox', { name: '플로우 목록' });
    expect(listbox).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /플로우 둘/ })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /플로우 셋/ })).toBeInTheDocument();
  });

  it('다른 플로우 선택 시 해당 에디터로 이동한다', () => {
    render(<EditorToolbar flowId="flow-1" />);

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    fireEvent.click(screen.getByRole('option', { name: /플로우 둘/ }));

    expect(navigateMock).toHaveBeenCalledTimes(1);
    expect(navigateMock).toHaveBeenCalledWith('/editor/flow-2');
  });

  it('현재 플로우를 다시 선택하면 이동하지 않는다', () => {
    render(<EditorToolbar flowId="flow-1" />);

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    fireEvent.click(screen.getByRole('option', { name: /플로우 하나/ }));

    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('Escape 키로 목록을 닫는다', () => {
    render(<EditorToolbar flowId="flow-1" />);

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    expect(screen.getByRole('listbox', { name: '플로우 목록' })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('listbox', { name: '플로우 목록' })).not.toBeInTheDocument();
  });

  it('인라인 이름 편집을 위해 이름 클릭 시 input 으로 전환한다', () => {
    render(<EditorToolbar flowId="flow-1" />);

    fireEvent.click(screen.getByRole('button', { name: '플로우 이름 (클릭하여 변경)' }));
    expect(screen.getByLabelText('플로우 이름')).toBeInTheDocument();
  });
});
