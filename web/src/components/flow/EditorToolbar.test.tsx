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

// i18n 은 키를 그대로 반환하도록 모킹한다. 로컬 경로 라벨은 하드코딩된 한국어
// 문자열이라 영향받지 않고, 원격 서브컴포넌트(RemoteFlowControls 등)의 툴팁만
// 키로 노출된다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 원격 타깃 전용 훅 모킹 — 원격 라이프사이클/게이팅/상태를 제어한다.
// 로컬 테스트(target 미지정)에서는 EditorToolbar 가 이 훅들을 마운트하지 않으므로
// 모킹은 원격 describe 블록에서만 의미가 있다.
const performMock = vi.fn<(action: string, id: string) => Promise<void>>(
  async () => {},
);
let supportsMock: (action: string) => boolean = () => true;
let canControlMock: () => boolean = () => true;
let remoteStatusMock = 'stored';

vi.mock('@/hooks/useResourceActions', () => ({
  useFlowActionsTarget: () => ({
    isRemote: true,
    perform: performMock,
    supports: (action: string) => supportsMock(action),
    pending: {},
  }),
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: true,
    nodeReady: canControlMock(),
    nodeLabel: 'gw-1',
    canControl: () => canControlMock(),
  }),
}));
vi.mock('@/hooks/useDetailTargets', () => ({
  useFlowStatusTarget: () => ({
    data: { status: remoteStatusMock },
    isLoading: false,
    error: null,
  }),
}));

// FlowSettingsDialog 는 이 테스트와 무관하므로 가벼운 stub 으로 대체한다.
vi.mock('./FlowSettingsDialog', () => ({
  FlowSettingsDialog: () => null,
}));

import { EditorToolbar } from './EditorToolbar';
import { FOCUS_DEPTH_ALL, useEditorStore } from '@/stores/editorStore';
import type { ResourceTarget } from '@/lib/remote/target';

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

describe('EditorToolbar - 원격 타깃(통합 툴바)', () => {
  const remoteTarget: ResourceTarget = { type: 'remote', instanceId: 'inst-1' };

  const renderRemote = (props?: {
    flowId?: string;
    flowName?: string;
    onSave?: () => void;
    isSaving?: boolean;
  }) =>
    render(
      <EditorToolbar
        flowId={props?.flowId ?? 'flow-9'}
        showPortPanel={false}
        onTogglePortPanel={vi.fn()}
        target={remoteTarget}
        nodeLabel="gw-1"
        nodeTitle="inst-uuid-1234"
        flowName={props?.flowName ?? 'temperature-flow'}
        onSave={props?.onSave ?? vi.fn()}
        isSaving={props?.isSaving ?? false}
      />,
    );

  beforeEach(() => {
    performMock.mockClear();
    supportsMock = (action) => action !== 'restart'; // 원격은 재시작 미지원.
    canControlMock = () => true; // 노드 승인+온라인.
    remoteStatusMock = 'stored'; // 시작/배포 가능 상태.
    // 저장 버튼 활성화를 위해 dirty 로 둔다.
    useEditorStore.setState({ isDirty: true });
  });

  it('원격 대상 노드 배지(호스트명)를 표시하고 UUID 는 툴팁에만 노출한다', () => {
    renderRemote();
    const badge = screen.getByTestId('remote-editor-node-badge');
    expect(badge).toHaveTextContent('gw-1');
    expect(badge).not.toHaveTextContent('inst-uuid-1234');
    expect(badge.getAttribute('title')).toContain('inst-uuid-1234');
  });

  it('로컬과 동일한 라이프사이클 메뉴(시작/중지/배포/재시작 + 저장)를 렌더한다', () => {
    renderRemote();
    expect(screen.getByRole('button', { name: '저장' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '배포' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '시작' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '중지' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '재시작' })).toBeInTheDocument();
    // 상태 배지도 항상 렌더되어 레이아웃이 로컬과 일치한다.
    expect(screen.getByTestId('remote-editor-status-badge')).toBeInTheDocument();
  });

  it('노드가 온라인이면 시작/중지(해당 상태)/배포가 활성화된다', () => {
    remoteStatusMock = 'stored';
    renderRemote();
    // stored: 시작 가능, 배포 가능, 중지 불가.
    expect(screen.getByRole('button', { name: '시작' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '배포' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '중지' })).toBeDisabled();
  });

  it('실행 중이면 중지가 활성화되고 시작/배포는 비활성화된다', () => {
    remoteStatusMock = 'running';
    renderRemote();
    expect(screen.getByRole('button', { name: '중지' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '시작' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '배포' })).toBeDisabled();
  });

  it('재시작은 원격 미지원이라 비활성화 + 미지원 툴팁을 표시한다', () => {
    renderRemote();
    const restart = screen.getByRole('button', { name: '재시작' });
    expect(restart).toBeDisabled();
    expect(restart.getAttribute('title')).toBe('remote.edit.unsupportedOnRemote');
  });

  it('노드가 미승인/오프라인이면 시작/중지/배포가 게이트 안내와 함께 비활성화된다', () => {
    canControlMock = () => false;
    renderRemote();
    const start = screen.getByRole('button', { name: '시작' });
    expect(start).toBeDisabled();
    expect(start.getAttribute('title')).toBe('remote.edit.actionGateHint');
  });

  it('시작 클릭 시 타깃 액션(perform("start"))으로 라우팅한다', () => {
    renderRemote({ flowId: 'flow-9' });
    fireEvent.click(screen.getByRole('button', { name: '시작' }));
    expect(performMock).toHaveBeenCalledTimes(1);
    expect(performMock).toHaveBeenCalledWith('start', 'flow-9');
  });

  it('저장 클릭 시 주입된 onSave(원격 명령 전파)를 호출한다', () => {
    const onSave = vi.fn();
    renderRemote({ onSave });
    fireEvent.click(screen.getByRole('button', { name: '저장' }));
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it('신규(빈 flowId) 플로우는 라이프사이클 버튼을 모두 비활성화한다', () => {
    renderRemote({ flowId: '' });
    expect(screen.getByRole('button', { name: '시작' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '배포' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '중지' })).toBeDisabled();
  });

  it('원격에서는 플로우 설정 버튼을 노출하지 않는다(명령 경로 밖)', () => {
    renderRemote();
    expect(
      screen.queryByRole('button', { name: '플로우 설정' }),
    ).not.toBeInTheDocument();
  });
});
