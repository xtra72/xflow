// FlowDetailPanel 노드 ID 표시 + 검색 테스트.
//
// 배경: 엔진 에러 로그는 노드를 ID(서브플로우 확장 노드는
// `subflow_<부모 flow-node id>_<원본 노드 id>`)로만 지목하는데, 노드 목록에
// ID 가 없어 그 노드를 화면에서 찾을 방법이 없었다. ID 컬럼 + 검색으로
// 로그의 ID 를 그대로 붙여넣어 노드를 특정할 수 있어야 한다.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetProvider';
import { LOCAL_TARGET } from '@/lib/remote/target';
import type { FlowNodeInfo, FlowStatusInfo } from '@/types/flow';

const useFlowStatusTargetMock = vi.hoisted(() => vi.fn());
const useFlowNodesTargetMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDetailTargets', () => ({
  useFlowStatusTarget: useFlowStatusTargetMock,
  useFlowNodesTarget: useFlowNodesTargetMock,
}));
vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: vi.fn(),
  resetComponentLogLevel: vi.fn(),
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: () => vi.fn(),
}));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { shortenNodeId } from '@/lib/flow/subflowNamespace';

import FlowDetailPanel from './FlowDetailPanel';

/** 에러 로그에 실제로 찍힌 형태의 서브플로우 확장 노드 ID. */
const SUBFLOW_NODE_ID =
  'subflow_51f452f0-39a0-496c-9177-da56a43a3db1_534114d2-dea5-4029-9f43-5a03d13da7f3';
const SIBLING_NODE_ID =
  'subflow_51f452f0-39a0-496c-9177-da56a43a3db1_4e7f6ef1-99d6-4b2f-a47c-8b6dcea190fe';

function makeNode(o: Partial<FlowNodeInfo> = {}): FlowNodeInfo {
  return {
    node_id: 'n1',
    name: 'Inject',
    type: 'inject',
    state: 'running',
    ports: [],
    ...o,
  } as FlowNodeInfo;
}

function renderPanel() {
  return render(
    <TargetProvider target={LOCAL_TARGET}>
      <FlowDetailPanel flowId="f1" />
    </TargetProvider>,
  );
}

/** 검색 입력에 값을 넣는다(전체 교체 — 붙여넣기와 동일). */
function search(value: string) {
  fireEvent.change(screen.getByLabelText('flows.detail.searchPlaceholder'), {
    target: { value },
  });
}

beforeEach(() => {
  useFlowStatusTargetMock.mockReset().mockReturnValue({
    data: { status: 'running' } as FlowStatusInfo,
    isLoading: false,
    error: null,
  });
  useFlowNodesTargetMock.mockReset().mockReturnValue({
    data: [
      makeNode({ node_id: SUBFLOW_NODE_ID, name: 'Store Write', type: 'storage-write' }),
      makeNode({ node_id: SIBLING_NODE_ID, name: 'Influx Write', type: 'storage-write' }),
      makeNode({ node_id: 'n1', name: 'Inject', type: 'inject' }),
    ],
    isLoading: false,
    error: null,
  });
});

describe('shortenNodeId — 가운데 축약', () => {
  it('상한 이하 ID 는 그대로 둔다', () => {
    expect(shortenNodeId('n1')).toBe('n1');
  });

  it('긴 ID 는 앞뒤를 남기고 가운데를 줄인다 (원본 노드 id 가 보여야 한다)', () => {
    const out = shortenNodeId(SUBFLOW_NODE_ID);
    expect(out).toContain('...');
    expect(out.startsWith('subflow_')).toBe(true);
    // 형제 노드와 구분되는 꼬리(원본 노드 id)가 남아야 한다.
    expect(out.endsWith(SUBFLOW_NODE_ID.slice(-8))).toBe(true);
    expect(out).not.toBe(shortenNodeId(SIBLING_NODE_ID));
  });
});

describe('FlowDetailPanel — 노드 ID 컬럼', () => {
  it('노드 ID 컬럼 헤더를 표시한다', () => {
    renderPanel();
    expect(screen.getByText('flows.detail.nodeId')).toBeInTheDocument();
  });

  it('각 행의 전체 노드 ID 를 title 로 노출한다 (축약 표시여도 원본 확인 가능)', () => {
    renderPanel();
    const buttons = screen.getAllByLabelText('flows.detail.copyId');
    const titles = buttons.map((b) => b.getAttribute('title'));
    expect(titles).toContain(SUBFLOW_NODE_ID);
    expect(titles).toContain(SIBLING_NODE_ID);
  });
});

describe('FlowDetailPanel — 노드 검색', () => {
  it('로그에서 복사한 전체 노드 ID 로 해당 노드만 남긴다', () => {
    renderPanel();
    search(SUBFLOW_NODE_ID);

    expect(screen.getByText('Store Write')).toBeInTheDocument();
    expect(screen.queryByText('Influx Write')).not.toBeInTheDocument();
    expect(screen.queryByText('Inject')).not.toBeInTheDocument();
  });

  it('이름과 타입으로도 검색된다', () => {
    renderPanel();

    search('storage-write');
    expect(screen.getByText('Store Write')).toBeInTheDocument();
    expect(screen.getByText('Influx Write')).toBeInTheDocument();
    expect(screen.queryByText('Inject')).not.toBeInTheDocument();

    search('inject');
    expect(screen.getByText('Inject')).toBeInTheDocument();
    expect(screen.queryByText('Store Write')).not.toBeInTheDocument();
  });

  it('대소문자를 구분하지 않는다', () => {
    renderPanel();
    search('STORE WRITE');
    expect(screen.getByText('Store Write')).toBeInTheDocument();
  });

  it('일치하는 노드가 없으면 안내를 표시한다', () => {
    renderPanel();
    search('zzz-none');
    expect(screen.getByText('flows.detail.noMatch')).toBeInTheDocument();
  });

  it('검색어가 없으면 모든 노드를 표시한다 (회귀 없음)', () => {
    renderPanel();
    expect(screen.getByText('Store Write')).toBeInTheDocument();
    expect(screen.getByText('Influx Write')).toBeInTheDocument();
    expect(screen.getByText('Inject')).toBeInTheDocument();
    expect(screen.queryByText('flows.detail.noMatch')).not.toBeInTheDocument();
  });
});
