// 노드 출력 tap(관찰) 상태 관리 - zustand.
//
// 두 종류의 런타임 전용 상태를 보관한다:
//   1) tappedNodeIds   — 현재 관찰 중인 노드 ID 집합 (캔버스의 눈 인디케이터 +
//                        탭 출력 패널 필터에 사용). 서버 tap 상태의 클라이언트 미러.
//   2) outputsByNode   — 노드별 최근 출력 메시지 링 버퍼 (오래된 것부터 드롭).
//
// tap 은 서버 측에서도 인메모리/런타임 전용이므로 이 스토어도 영속화하지 않는다.

import { create } from 'zustand';

/**
 * `node.output` WebSocket 메시지의 `message` 필드.
 *
 * debug 노드의 whole-message 맵과 동일한 형상이다(백엔드 buildNodeOutputMessage).
 */
export interface NodeOutputMessage {
  id: string;
  type: string;
  /** human-readable RFC3339 문자열. */
  time: string;
  /** epoch milliseconds. */
  timestamp: number;
  payload: Record<string, unknown>;
  metadata: Record<string, unknown>;
}

/** `node.output` WebSocket 메시지의 페이로드(언랩된 envelope.payload). */
export interface NodeOutputPayload {
  flow_id: string;
  node_id: string;
  /** "out" | "error" | 사용자 정의 포트명. */
  port: string;
  message: NodeOutputMessage;
}

/** 패널에 표시되는 단일 tap 출력 엔트리. */
export interface TapEntry {
  /** 클라이언트 측 단조 증가 ID (React key). */
  id: number;
  nodeId: string;
  port: string;
  message: NodeOutputMessage;
}

/** 노드별 보관하는 최대 출력 엔트리 수 (오래된 것부터 드롭). */
export const TAP_RING_BUFFER_SIZE = 100;

interface TapState {
  /** 관찰 중인 노드 ID 집합 (값은 항상 true). */
  tappedNodeIds: Record<string, true>;
  /** 노드 ID → 최근 출력 엔트리 링 버퍼. */
  outputsByNode: Record<string, TapEntry[]>;

  // --- 액션 ---

  /** 단일 노드의 tap 상태를 설정한다(낙관적 UI 갱신용). */
  setTapped: (nodeId: string, enabled: boolean) => void;
  /** 서버 tap 목록으로 관찰 집합을 통째로 교체한다(복원용). */
  setTappedNodeIds: (nodeIds: string[]) => void;
  /** WS 페이로드를 해당 노드의 링 버퍼에 추가한다(오래된 것부터 드롭). */
  appendOutput: (payload: NodeOutputPayload) => void;
  /** 특정 노드의 출력 버퍼를 비운다. */
  clearNode: (nodeId: string) => void;
  /** 모든 출력 버퍼를 비운다(관찰 집합은 유지). */
  clearOutputs: () => void;
  /** 스토어 전체를 초기화한다(에디터 닫기/플로우 전환 시). */
  reset: () => void;
}

/** 링 버퍼 ID 발급용 모듈 카운터 (스토어 재생성과 무관하게 단조 증가). */
let nextEntryId = 0;

export const useTapStore = create<TapState>((set) => ({
  tappedNodeIds: {},
  outputsByNode: {},

  setTapped: (nodeId, enabled) =>
    set((state) => {
      const next = { ...state.tappedNodeIds };
      if (enabled) {
        next[nodeId] = true;
      } else {
        delete next[nodeId];
      }
      return { tappedNodeIds: next };
    }),

  setTappedNodeIds: (nodeIds) =>
    set(() => {
      const next: Record<string, true> = {};
      for (const id of nodeIds) {
        next[id] = true;
      }
      return { tappedNodeIds: next };
    }),

  appendOutput: (payload) =>
    set((state) => {
      const entry: TapEntry = {
        id: ++nextEntryId,
        nodeId: payload.node_id,
        port: payload.port,
        message: payload.message,
      };
      const existing = state.outputsByNode[payload.node_id] ?? [];
      const appended = [...existing, entry];
      const trimmed =
        appended.length > TAP_RING_BUFFER_SIZE
          ? appended.slice(-TAP_RING_BUFFER_SIZE)
          : appended;
      return {
        outputsByNode: {
          ...state.outputsByNode,
          [payload.node_id]: trimmed,
        },
      };
    }),

  clearNode: (nodeId) =>
    set((state) => {
      if (!(nodeId in state.outputsByNode)) return state;
      const next = { ...state.outputsByNode };
      delete next[nodeId];
      return { outputsByNode: next };
    }),

  clearOutputs: () => set({ outputsByNode: {} }),

  reset: () => set({ tappedNodeIds: {}, outputsByNode: {} }),
}));
