// SPEC-SCHEDULE-VIEW-001 M4 — useScheduleDualWrite 단위 테스트.
// dual-write 훅 추출의 특성 보존 검증: happy path(LIVE+PERSIST/baseline 갱신/onApplied/
// 성공 통지), 404 persist-only, LIVE 비404 조기 반환, PERSIST 오류, conflict, saving 재진입 가드.

import { renderHook, act } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { APIError } from '@/types/api';

const state = vi.hoisted(() => ({
  notifications: [] as Array<{ type: string; message: string }>,
  configCalls: [] as Array<{ config: Record<string, unknown> }>,
}));

const configureNodeMock = vi.hoisted(() => vi.fn());
const getFlowMock = vi.hoisted(() => vi.fn());
const updateFlowMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/nodeService', () => ({
  configureNode: (flowId: string, nodeId: string, config: Record<string, unknown>) => {
    state.configCalls.push({ config });
    return configureNodeMock(flowId, nodeId, config);
  },
}));
vi.mock('@/services/api/flowService', () => ({
  getFlow: (id: string) => getFlowMock(id),
  updateFlow: (id: string, req: unknown) => updateFlowMock(id, req),
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({ addNotification: (n: { type: string; message: string }) => state.notifications.push(n) }),
}));

import { useScheduleDualWrite } from './useScheduleDualWrite';

const FLOW = 'flow-a';
const NODE = 'trig-1';

function setup() {
  return renderHook(() => useScheduleDualWrite({ flowId: FLOW, nodeId: NODE }));
}

beforeEach(() => {
  state.notifications = [];
  state.configCalls = [];
  configureNodeMock.mockReset().mockResolvedValue(undefined);
  // getFlow 는 대상 노드가 존재하되 config 는 baseline 과 동일(스케줄 없음)한 정의를 반환.
  getFlowMock.mockReset().mockResolvedValue({ config: { nodes: [{ id: NODE, data: {} }] } });
  updateFlowMock.mockReset().mockResolvedValue(undefined);
});

describe('happy path — LIVE + PERSIST', () => {
  it('configureNode/getFlow/updateFlow 호출 + onApplied + 성공 통지', async () => {
    const onApplied = vi.fn();
    const { result } = setup();
    const next = [{ name: 'A', enabled: true }];

    await act(async () => {
      await result.current.persist(next, onApplied);
    });

    expect(configureNodeMock).toHaveBeenCalledWith(FLOW, NODE, { schedules: next });
    expect(getFlowMock).toHaveBeenCalledWith(FLOW);
    expect(updateFlowMock).toHaveBeenCalledTimes(1);
    expect(onApplied).toHaveBeenCalledWith(next);
    expect(state.notifications).toEqual([{ type: 'success', message: '저장 완료' }]);
    expect(result.current.saving).toBe(false);
  });

  it('setBaseline 시드 → FULL config 병합 + 성공 후 baseline 갱신(다음 저장에 반영)', async () => {
    const { result } = setup();
    act(() => result.current.setBaseline({ foo: 'bar' }));

    const s1 = [{ name: 'A' }];
    await act(async () => {
      await result.current.persist(s1);
    });
    // baseline(foo) 보존 + schedules 교체.
    expect(state.configCalls[0]!.config).toEqual({ foo: 'bar', schedules: s1 });

    const s2 = [{ name: 'B' }];
    await act(async () => {
      await result.current.persist(s2);
    });
    // 성공으로 baseline 이 이전 fullConfig 로 갱신됨 → foo 유지 + schedules 재교체.
    expect(state.configCalls[1]!.config).toEqual({ foo: 'bar', schedules: s2 });
  });
});

describe('404 persist-only (AC-11)', () => {
  it('configureNode 404 → persist 는 계속 + persist-only 경고, onApplied 호출', async () => {
    configureNodeMock.mockRejectedValue(new APIError('NOT_FOUND', 'not running', 404));
    const onApplied = vi.fn();
    const { result } = setup();

    await act(async () => {
      await result.current.persist([{ name: 'A' }], onApplied);
    });

    expect(updateFlowMock).toHaveBeenCalledTimes(1);
    expect(onApplied).toHaveBeenCalledTimes(1);
    expect(state.notifications).toEqual([
      { type: 'warning', message: '노드 미실행 — 저장만 적용, 재배포 시 반영' },
    ]);
  });
});

describe('LIVE 비404 오류 — 조기 반환', () => {
  it('persist/onApplied 미실행 + 오류 통지 + saving 해제', async () => {
    configureNodeMock.mockRejectedValue(new APIError('INTERNAL', 'boom', 500));
    const onApplied = vi.fn();
    const { result } = setup();

    await act(async () => {
      await result.current.persist([{ name: 'A' }], onApplied);
    });

    expect(getFlowMock).not.toHaveBeenCalled();
    expect(updateFlowMock).not.toHaveBeenCalled();
    expect(onApplied).not.toHaveBeenCalled();
    expect(state.notifications).toEqual([
      { type: 'error', message: '저장 실패 — 라이브 반영 중 오류가 발생했습니다.' },
    ]);
    expect(result.current.saving).toBe(false);
  });
});

describe('PERSIST 오류 — 조기 반환', () => {
  it('updateFlow 실패 → onApplied 미실행 + 오류 통지', async () => {
    updateFlowMock.mockRejectedValue(new Error('put failed'));
    const onApplied = vi.fn();
    const { result } = setup();

    await act(async () => {
      await result.current.persist([{ name: 'A' }], onApplied);
    });

    expect(onApplied).not.toHaveBeenCalled();
    expect(state.notifications).toEqual([
      { type: 'error', message: '저장 실패 — 플로우 정의 지속화 중 오류가 발생했습니다.' },
    ]);
    expect(result.current.saving).toBe(false);
  });
});

describe('conflict — last-write-wins 경고', () => {
  it('신선 노드 config 가 baseline 과 다르면 추가 경고', async () => {
    // baseline 은 스케줄 없음, getFlow 는 외부 편집(다른 스케줄)이 있는 정의 반환.
    getFlowMock.mockResolvedValue({
      config: { nodes: [{ id: NODE, data: { schedules: [{ name: 'EXTERNAL' }] } }] },
    });
    const { result } = setup();

    await act(async () => {
      await result.current.persist([{ name: 'A' }]);
    });

    expect(state.notifications).toEqual([
      { type: 'success', message: '저장 완료' },
      { type: 'warning', message: '다른 편집이 감지되어 덮어썼습니다 (last-write-wins)' },
    ]);
  });
});

describe('saving 재진입 가드', () => {
  it('진행 중 재호출은 즉시 반환(configureNode 중복 없음)', async () => {
    let resolveLive: (() => void) | undefined;
    configureNodeMock.mockImplementation(
      () =>
        new Promise<void>((res) => {
          resolveLive = () => res();
        }),
    );
    const { result } = setup();

    // 1) 첫 persist 시작(LIVE 대기) → saving=true.
    let first: Promise<void>;
    act(() => {
      first = result.current.persist([{ name: 'A' }]);
    });

    // 2) 진행 중 재호출 → 가드로 즉시 반환.
    await act(async () => {
      await result.current.persist([{ name: 'B' }]);
    });
    expect(configureNodeMock).toHaveBeenCalledTimes(1);

    // 3) 첫 persist 완료 정리.
    await act(async () => {
      resolveLive?.();
      await first;
    });
    expect(configureNodeMock).toHaveBeenCalledTimes(1);
    expect(updateFlowMock).toHaveBeenCalledTimes(1);
  });
});
