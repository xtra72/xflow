// PropertyPanel 노드 설정 패널 회귀 테스트.
//
// 검증 대상:
//  1) 헤더에 "타입" / "아이디" 라벨 필드가 표시된다.
//  2) 저장된 옛 `_` HVAC 타입은 canonical `-` 표기로 표시된다(normalizeNodeType).
//  3) 타입 설명과 포트 설명이 인라인 텍스트가 아니라 `?` 도움말(FieldHelp)로 렌더된다.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

// useAgents 는 React Query 훅이므로 QueryClientProvider 없이 렌더되도록 모킹한다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

import { useEditorStore } from '@/stores/editorStore';
import { PropertyPanel } from './PropertyPanel';

const NODE_ID = 'node-test-1';

/** 지정 nodeType 의 노드를 스토어에 심고 선택한 뒤 PropertyPanel 을 렌더한다. */
function renderPanelFor(
  nodeType: string,
  ports: Array<{ name: string; direction: 'input' | 'output' | 'error' }>,
  extraData: Record<string, unknown> = {},
) {
  useEditorStore.setState({
    nodes: [
      {
        id: NODE_ID,
        type: 'custom',
        position: { x: 0, y: 0 },
        data: {
          label: 'sample',
          nodeType,
          category: 'io',
          ports,
          ...extraData,
        },
      },
    ],
    edges: [],
    selectedNodeId: NODE_ID,
  });
  return render(<PropertyPanel />);
}

describe('PropertyPanel — 타입/아이디 타이틀 + ? 도움말', () => {
  beforeEach(() => {
    useEditorStore.setState({ nodes: [], edges: [], selectedNodeId: null });
  });

  it('헤더에 "타입"과 "아이디" 라벨을 표시한다', () => {
    renderPanelFor('filter', [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
    expect(screen.getByText('타입')).toBeInTheDocument();
    expect(screen.getByText('아이디')).toBeInTheDocument();
    // 노드 id 가 아이디 값으로 표시된다.
    expect(screen.getByText(NODE_ID)).toBeInTheDocument();
  });

  it('저장된 옛 `_` HVAC 타입을 canonical `-` 표기로 표시한다', () => {
    renderPanelFor('samsung_hvacr01_status', [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
      { name: 'error', direction: 'error' },
    ]);
    // canonical 표기로 노출되고, 옛 `_` 표기는 노출되지 않는다.
    expect(screen.getByText('samsung-hvacr01-status')).toBeInTheDocument();
    expect(screen.queryByText('samsung_hvacr01_status')).not.toBeInTheDocument();
  });

  it('타입 설명을 인라인이 아니라 `?` 도움말(FieldHelp)로 렌더한다', () => {
    renderPanelFor('filter', [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
    // FieldHelp 의 토글 버튼은 aria-label="설명 보기".
    const helpButtons = screen.getAllByRole('button', { name: '설명 보기' });
    expect(helpButtons.length).toBeGreaterThan(0);
  });

  it('포트 설명을 포트별 `?` 도움말로 렌더한다 (NODE_TYPE_META 보유 타입)', () => {
    // filter 는 NODE_TYPE_META 에 in/out/reject/error 포트 설명을 가진다.
    renderPanelFor('filter', [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
    // 타입 설명(1) + 포트 in/out 설명(2) = 최소 3개의 도움말 토글이 존재한다.
    const helpButtons = screen.getAllByRole('button', { name: '설명 보기' });
    expect(helpButtons.length).toBeGreaterThanOrEqual(3);
  });
});

describe('PropertyPanel — 출력 미연결 경고 끄기 토글 (모든 노드 공통)', () => {
  beforeEach(() => {
    useEditorStore.setState({ nodes: [], edges: [], selectedNodeId: null });
  });

  it('모든 노드 타입에 토글을 렌더하고 기본값은 off(false)이다', () => {
    // filter 는 타입 전용 스키마와 무관하게 공통 토글이 노출되어야 한다.
    renderPanelFor('filter', [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ]);
    expect(screen.getByText('출력 미연결 경고 끄기')).toBeInTheDocument();
    const toggle = screen.getByRole('switch', { name: '출력 미연결 경고 끄기' });
    // 기본값은 off — config 에 키가 없으면 aria-checked=false.
    expect(toggle).toHaveAttribute('aria-checked', 'false');
  });

  it('토글하면 노드 config 에 suppress_unconnected_warning: true 를 기록한다', () => {
    // filter 의 필수 필드(condition)를 미리 채워 "적용" 버튼이 활성화되게 한다.
    renderPanelFor(
      'filter',
      [
        { name: 'in', direction: 'input' },
        { name: 'out', direction: 'output' },
      ],
      { condition: '.payload != null' },
    );

    // 토글 ON
    const toggle = screen.getByRole('switch', { name: '출력 미연결 경고 끄기' });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'true');

    // 드래프트 변경 후 "적용" 으로 스토어에 반영한다.
    fireEvent.click(screen.getByRole('button', { name: /적용/ }));

    const node = useEditorStore.getState().nodes.find((n) => n.id === NODE_ID);
    expect(node?.data.suppress_unconnected_warning).toBe(true);
  });

  it('다시 토글하면 false 를 기록한다(기본값 직렬화)', () => {
    renderPanelFor(
      'filter',
      [
        { name: 'in', direction: 'input' },
        { name: 'out', direction: 'output' },
      ],
      { condition: '.payload != null' },
    );
    const toggle = screen.getByRole('switch', { name: '출력 미연결 경고 끄기' });
    // ON → OFF
    fireEvent.click(toggle);
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'false');

    fireEvent.click(screen.getByRole('button', { name: /적용/ }));

    const node = useEditorStore.getState().nodes.find((n) => n.id === NODE_ID);
    expect(node?.data.suppress_unconnected_warning).toBe(false);
  });
});
