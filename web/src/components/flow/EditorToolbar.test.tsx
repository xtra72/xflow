// EditorToolbar 의 플로우 타이틀(읽기 전용) + 전환 선택기(FlowSwitcher) 테스트.
//
// 검증 대상:
//   - 현재 플로우 이름 렌더링 (읽기 전용 FlowTitle, 인라인 편집 제거)
//   - 설명(description)이 있으면 이름 뒤 도움말(?) 아이콘 표시, 없으면 숨김
//   - chevron 버튼 클릭 시 전체 플로우 목록 드롭다운 표시
//   - 다른 플로우 선택 시 navigate('/editor/{id}') 호출
//   - 같은(현재) 플로우 선택 시 navigate 미호출

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type { FlowInfo } from '@/types/flow';

// react-router 의 useNavigate / useLocation 를 모킹한다.
// useLocation 은 서브플로우 "돌아가기" 버튼(SubflowBackButton)이 백 스택을
// 읽는 데 사용한다. 기본 location.state 는 null 이라 백 스택이 비어 돌아가기
// 버튼은 숨겨지며, 필요한 테스트에서 locationStateMock 으로 주입한다.
const navigateMock = vi.fn();
let locationStateMock: unknown = null;
vi.mock('react-router', () => ({
  useNavigate: () => navigateMock,
  useLocation: () => ({
    pathname: '/editor/flow-1',
    search: '',
    hash: '',
    state: locationStateMock,
    key: 'test',
  }),
}));

// 플로우 훅 모킹. 전환 선택기와 타이틀 표시에 필요한 최소 동작만 제공한다.
// flow-1 은 설명이 있고, flow-2 / flow-3 은 설명이 없다.
const updateMutate = vi.fn();
const flowsList: FlowInfo[] = [
  {
    id: 'flow-1',
    name: '플로우 하나',
    description: '첫 번째 플로우 설명',
    status: 'stored',
    node_count: 0,
  },
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
import { FOCUS_DEPTH_ALL, useEditorStore } from '@/stores/editorStore';

// SPEC-SUBFLOW-001 M4: EditorToolbar 는 플로우 포트 패널 토글 props 를 요구한다.
// 대부분의 테스트는 이 토글과 무관하므로 기본값을 주입하는 헬퍼를 사용한다.
const togglePortPanelMock = vi.fn();
const renderToolbar = (props?: {
  flowId?: string;
  showPortPanel?: boolean;
  onTogglePortPanel?: () => void;
}) =>
  render(
    <EditorToolbar
      flowId={props?.flowId ?? 'flow-1'}
      showPortPanel={props?.showPortPanel ?? false}
      onTogglePortPanel={props?.onTogglePortPanel ?? togglePortPanelMock}
    />,
  );

describe('EditorToolbar - 플로우 타이틀 / 전환 선택기', () => {
  beforeEach(() => {
    navigateMock.mockClear();
    updateMutate.mockClear();
    // 기본적으로 백 스택은 비어 있어(들어가기로 진입하지 않은 상태) 돌아가기 숨김.
    locationStateMock = null;
  });

  it('현재 플로우 이름을 렌더링한다', () => {
    renderToolbar();
    expect(screen.getByText('플로우 하나')).toBeInTheDocument();
  });

  it('이름은 읽기 전용으로 표시되며 클릭해도 input 으로 전환되지 않는다', () => {
    renderToolbar();

    // 인라인 편집이 제거되어 "플로우 이름" input 이 존재하지 않는다.
    expect(screen.queryByLabelText('플로우 이름')).not.toBeInstanceOf(
      HTMLInputElement,
    );

    // 이름 텍스트를 클릭해도 input 이 나타나지 않는다.
    fireEvent.click(screen.getByText('플로우 하나'));
    expect(
      screen.queryByRole('textbox', { name: '플로우 이름' }),
    ).not.toBeInTheDocument();
  });

  it('설명이 있으면 이름 뒤에 도움말(?) 아이콘을 표시한다', () => {
    renderToolbar({ flowId: 'flow-1' });
    expect(
      screen.getByRole('button', { name: '설명 보기' }),
    ).toBeInTheDocument();
  });

  it('도움말(?) 클릭 시 설명 팝오버를 보여준다', () => {
    renderToolbar({ flowId: 'flow-1' });

    fireEvent.click(screen.getByRole('button', { name: '설명 보기' }));
    expect(screen.getByRole('tooltip')).toHaveTextContent('첫 번째 플로우 설명');
  });

  it('설명이 없으면 도움말(?) 아이콘을 표시하지 않는다', () => {
    renderToolbar({ flowId: 'flow-2' });
    expect(
      screen.queryByRole('button', { name: '설명 보기' }),
    ).not.toBeInTheDocument();
  });

  it('전환 버튼 클릭 시 전체 플로우 목록을 표시한다', () => {
    renderToolbar();

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
    renderToolbar();

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    fireEvent.click(screen.getByRole('option', { name: /플로우 둘/ }));

    expect(navigateMock).toHaveBeenCalledTimes(1);
    expect(navigateMock).toHaveBeenCalledWith('/editor/flow-2');
  });

  it('현재 플로우를 다시 선택하면 이동하지 않는다', () => {
    renderToolbar();

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    fireEvent.click(screen.getByRole('option', { name: /플로우 하나/ }));

    expect(navigateMock).not.toHaveBeenCalled();
  });

  it('Escape 키로 목록을 닫는다', () => {
    renderToolbar();

    fireEvent.click(screen.getByRole('button', { name: '다른 플로우로 전환' }));
    expect(screen.getByRole('listbox', { name: '플로우 목록' })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('listbox', { name: '플로우 목록' })).not.toBeInTheDocument();
  });
});

describe('EditorToolbar - 플로우 포트 패널 토글 (제어판 통합)', () => {
  beforeEach(() => {
    togglePortPanelMock.mockClear();
  });

  it('제어판에 "플로우 포트" 토글 버튼을 렌더링한다', () => {
    renderToolbar();
    expect(
      screen.getByRole('button', { name: '플로우 포트' }),
    ).toBeInTheDocument();
  });

  it('토글 버튼 클릭 시 onTogglePortPanel 을 호출한다', () => {
    renderToolbar();
    fireEvent.click(screen.getByRole('button', { name: '플로우 포트' }));
    expect(togglePortPanelMock).toHaveBeenCalledTimes(1);
  });

  it('showPortPanel=true 이면 토글 버튼이 눌림(active) 상태로 표시된다', () => {
    renderToolbar({ showPortPanel: true });
    expect(screen.getByRole('button', { name: '플로우 포트' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });
});

describe('EditorToolbar - 연결 단계 스테퍼 (1~5~전체)', () => {
  // 스테퍼는 실제 editorStore 와 연결되므로 매 테스트마다 포커스를 켜고
  // depth 를 초기화한다(스테퍼는 focusConnectionsOnSelect 가 true 일 때만 활성).
  beforeEach(() => {
    useEditorStore.setState({ focusConnectionsOnSelect: true, focusDepth: 1 });
  });

  const inc = () => screen.getByRole('button', { name: '연결 단계 늘리기' });
  const dec = () => screen.getByRole('button', { name: '연결 단계 줄이기' });

  it('초기 1 단계에서는 "1" 을 표시하고 줄이기 버튼이 비활성화된다', () => {
    renderToolbar();
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(dec()).toBeDisabled();
    expect(inc()).not.toBeDisabled();
  });

  it('늘리기를 5 까지 올린 뒤 한 번 더 누르면 전체(Infinity) 로 전환된다', () => {
    renderToolbar();

    // 1 → 5 까지 4회 증가.
    for (let i = 0; i < 4; i += 1) fireEvent.click(inc());
    expect(useEditorStore.getState().focusDepth).toBe(5);
    expect(screen.getByText('5')).toBeInTheDocument();

    // 5 에서 한 번 더 → 전체.
    fireEvent.click(inc());
    expect(useEditorStore.getState().focusDepth).toBe(FOCUS_DEPTH_ALL);
    expect(screen.getByText('전체')).toBeInTheDocument();
  });

  it('전체에서는 "전체" 라벨을 보이고 늘리기 버튼이 비활성화된다', () => {
    useEditorStore.setState({ focusDepth: FOCUS_DEPTH_ALL });
    renderToolbar();

    expect(screen.getByText('전체')).toBeInTheDocument();
    expect(inc()).toBeDisabled();
    expect(dec()).not.toBeDisabled();
  });

  it('전체에서 줄이기를 누르면 유한 상한(5) 으로 돌아간다', () => {
    useEditorStore.setState({ focusDepth: FOCUS_DEPTH_ALL });
    renderToolbar();

    fireEvent.click(dec());
    expect(useEditorStore.getState().focusDepth).toBe(5);
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('포커스가 꺼져 있으면 두 버튼 모두 비활성화된다', () => {
    useEditorStore.setState({ focusConnectionsOnSelect: false });
    renderToolbar();

    expect(inc()).toBeDisabled();
    expect(dec()).toBeDisabled();
  });
});

describe('EditorToolbar - 서브플로우 돌아가기', () => {
  beforeEach(() => {
    navigateMock.mockClear();
    locationStateMock = null;
  });

  it('백 스택이 비어 있으면 "돌아가기" 버튼을 숨긴다', () => {
    locationStateMock = null;
    renderToolbar({ flowId: 'flow-1' });
    expect(
      screen.queryByRole('button', { name: '돌아가기' }),
    ).not.toBeInTheDocument();
  });

  it('백 스택이 있으면 "돌아가기" 버튼을 표시한다', () => {
    locationStateMock = { subflowBack: ['flow-1'] };
    renderToolbar({ flowId: 'flow-2' });
    expect(
      screen.getByRole('button', { name: '돌아가기' }),
    ).toBeInTheDocument();
  });

  it('"돌아가기" 클릭 시 직전 플로우로 이동하고 남은 스택을 넘긴다(중첩)', () => {
    // A(flow-1) → B(flow-2) → C(flow-3) 로 들어간 상태(C 에서 스택 [A, B]).
    locationStateMock = { subflowBack: ['flow-1', 'flow-2'] };
    renderToolbar({ flowId: 'flow-3' });

    fireEvent.click(screen.getByRole('button', { name: '돌아가기' }));

    // 직전 플로우 B(flow-2) 로 이동하며 남은 스택 [A] 를 location.state 로 넘긴다.
    expect(navigateMock).toHaveBeenCalledTimes(1);
    expect(navigateMock).toHaveBeenCalledWith('/editor/flow-2', {
      state: { subflowBack: ['flow-1'] },
    });
  });

  it('백 스택 마지막 단계에서 돌아가면 빈 스택을 넘긴다', () => {
    locationStateMock = { subflowBack: ['flow-1'] };
    renderToolbar({ flowId: 'flow-2' });

    fireEvent.click(screen.getByRole('button', { name: '돌아가기' }));

    expect(navigateMock).toHaveBeenCalledWith('/editor/flow-1', {
      state: { subflowBack: [] },
    });
  });
});
