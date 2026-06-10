// tapStore 단위 테스트 — 관찰 집합 + 노드별 출력 링 버퍼.
//
// 검증 항목:
//   - setTapped / setTappedNodeIds 로 관찰 집합 갱신·복원
//   - appendOutput 이 node_id 로 키잉하여 별도 버퍼에 적재
//   - 링 버퍼가 TAP_RING_BUFFER_SIZE 초과 시 오래된 것부터 드롭
//   - clearNode / clearOutputs / reset 동작

import { beforeEach, describe, expect, it } from 'vitest';

import {
  TAP_RING_BUFFER_SIZE,
  useTapStore,
  type NodeOutputPayload,
} from './tapStore';

/** 테스트용 node.output 페이로드 팩토리. */
function makePayload(
  nodeId: string,
  value: number,
  port = 'out',
): NodeOutputPayload {
  return {
    flow_id: 'flow-1',
    node_id: nodeId,
    port,
    message: {
      id: `msg-${nodeId}-${value}`,
      type: 'data',
      time: '2026-01-01T00:00:00.000Z',
      timestamp: 1_700_000_000_000 + value,
      payload: { value },
      metadata: {},
    },
  };
}

describe('tapStore 관찰 집합', () => {
  beforeEach(() => {
    useTapStore.getState().reset();
  });

  it('setTapped(true) 는 노드를 관찰 집합에 추가한다', () => {
    useTapStore.getState().setTapped('node-1', true);
    expect(useTapStore.getState().tappedNodeIds['node-1']).toBe(true);
  });

  it('setTapped(false) 는 노드를 관찰 집합에서 제거한다', () => {
    useTapStore.getState().setTapped('node-1', true);
    useTapStore.getState().setTapped('node-1', false);
    expect(useTapStore.getState().tappedNodeIds['node-1']).toBeUndefined();
  });

  it('setTappedNodeIds 는 관찰 집합을 통째로 교체한다(복원)', () => {
    useTapStore.getState().setTapped('old', true);
    useTapStore.getState().setTappedNodeIds(['a', 'b', 'c']);

    const ids = useTapStore.getState().tappedNodeIds;
    expect(ids['old']).toBeUndefined();
    expect(ids['a']).toBe(true);
    expect(ids['b']).toBe(true);
    expect(ids['c']).toBe(true);
  });
});

describe('tapStore 출력 링 버퍼', () => {
  beforeEach(() => {
    useTapStore.getState().reset();
  });

  it('appendOutput 은 node_id 별로 분리된 버퍼에 적재한다', () => {
    useTapStore.getState().appendOutput(makePayload('node-1', 1));
    useTapStore.getState().appendOutput(makePayload('node-2', 2));
    useTapStore.getState().appendOutput(makePayload('node-1', 3));

    const { outputsByNode } = useTapStore.getState();
    expect(outputsByNode['node-1']).toHaveLength(2);
    expect(outputsByNode['node-2']).toHaveLength(1);
    expect(outputsByNode['node-1']?.[1]?.message.payload).toEqual({ value: 3 });
  });

  it('엔트리는 node_id, port, message 를 보존한다', () => {
    useTapStore.getState().appendOutput(makePayload('node-1', 7, 'error'));
    const entry = useTapStore.getState().outputsByNode['node-1']?.[0];
    expect(entry?.nodeId).toBe('node-1');
    expect(entry?.port).toBe('error');
    expect(entry?.message.id).toBe('msg-node-1-7');
  });

  it('TAP_RING_BUFFER_SIZE 초과 시 오래된 것부터 드롭한다', () => {
    const total = TAP_RING_BUFFER_SIZE + 10;
    for (let i = 0; i < total; i++) {
      useTapStore.getState().appendOutput(makePayload('node-1', i));
    }

    const buf = useTapStore.getState().outputsByNode['node-1'] ?? [];
    expect(buf).toHaveLength(TAP_RING_BUFFER_SIZE);
    // 가장 오래된 10개(value 0..9)는 드롭되고 value 10 이 첫 엔트리.
    expect(buf[0]?.message.payload).toEqual({ value: 10 });
    expect(buf[buf.length - 1]?.message.payload).toEqual({ value: total - 1 });
  });

  it('clearNode 는 해당 노드 버퍼만 비운다', () => {
    useTapStore.getState().appendOutput(makePayload('node-1', 1));
    useTapStore.getState().appendOutput(makePayload('node-2', 2));
    useTapStore.getState().clearNode('node-1');

    const { outputsByNode } = useTapStore.getState();
    expect(outputsByNode['node-1']).toBeUndefined();
    expect(outputsByNode['node-2']).toHaveLength(1);
  });

  it('clearOutputs 는 모든 버퍼를 비우되 관찰 집합은 유지한다', () => {
    useTapStore.getState().setTapped('node-1', true);
    useTapStore.getState().appendOutput(makePayload('node-1', 1));
    useTapStore.getState().clearOutputs();

    expect(useTapStore.getState().outputsByNode).toEqual({});
    expect(useTapStore.getState().tappedNodeIds['node-1']).toBe(true);
  });

  it('reset 은 관찰 집합과 버퍼를 모두 초기화한다', () => {
    useTapStore.getState().setTapped('node-1', true);
    useTapStore.getState().appendOutput(makePayload('node-1', 1));
    useTapStore.getState().reset();

    expect(useTapStore.getState().tappedNodeIds).toEqual({});
    expect(useTapStore.getState().outputsByNode).toEqual({});
  });
});
