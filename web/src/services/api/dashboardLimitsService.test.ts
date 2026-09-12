// dashboardLimitsService 단위 테스트 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// **이 시험이 지키는 성질은 하나다: 이 함수는 던지지 않는다.** 편집기는 서버에 닿지
// 못해도 가져오기를 계속해야 하고, 여기서 예외가 새어 나가면 그 자리가 막힌다.

import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({ get: getMock }));

import { getDashboardLimits } from './dashboardLimitsService';

describe('getDashboardLimits', () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it('GET 후 두 수를 언래핑해 반환한다', async () => {
    getMock.mockResolvedValueOnce({
      max_canvas_elements: 2048,
      payload_budget_bytes: 2048 * 1024,
    });

    const limits = await getDashboardLimits();

    expect(getMock).toHaveBeenCalledWith('/dashboard-limits');
    expect(limits).toEqual({ maxCanvasElements: 2048, payloadBudgetBytes: 2048 * 1024 });
  });

  // --- 값을 얻지 못하는 길 전부 -------------------------------------------
  //
  // 넷을 갈라 세는 것은 **호출부가 할 일이 같기 때문이 아니라 원인이 다르기 때문**이다:
  // 오프라인 · 구형 서버(404) · 권한 없음(403) · 응답 형상이 바뀜. 어느 쪽이든 던지지
  // 않아야 한다는 것이 요점이므로 네 길을 모두 친다.

  it('요청이 실패해도 던지지 않고 "모른다" 로 떨어진다', async () => {
    getMock.mockRejectedValueOnce(new Error('Network Error'));

    await expect(getDashboardLimits()).resolves.toEqual({
      maxCanvasElements: undefined,
      payloadBudgetBytes: undefined,
    });
  });

  it('404(구형 서버)도 같은 자리로 떨어진다', async () => {
    getMock.mockRejectedValueOnce({ response: { status: 404 } });

    const limits = await getDashboardLimits();
    expect(limits.maxCanvasElements).toBeUndefined();
  });

  it('403(권한 없음)도 같은 자리로 떨어진다', async () => {
    getMock.mockRejectedValueOnce({ response: { status: 403 } });

    const limits = await getDashboardLimits();
    expect(limits.maxCanvasElements).toBeUndefined();
  });

  it('data 가 null 이면 두 축 모두 undefined', async () => {
    getMock.mockResolvedValueOnce(null);

    await expect(getDashboardLimits()).resolves.toEqual({
      maxCanvasElements: undefined,
      payloadBudgetBytes: undefined,
    });
  });

  // --- 쓸 수 없는 수는 통과시키지 않는다 -----------------------------------
  //
  // **하나가 나빠도 나머지는 살린다** — 두 수는 서버에서 함께 유도되지만, 응답이 부분적으로
  // 상했을 때 멀쩡한 축까지 버리면 얻을 것이 없다.

  it.each([
    ['0', 0],
    ['음수', -1],
    ['소수', 12.5],
    ['NaN', Number.NaN],
    ['문자열', '2048' as unknown as number],
    ['null', null],
    ['누락', undefined],
  ])('max_canvas_elements 가 %s 이면 undefined 로 떨어진다', async (_label, value) => {
    getMock.mockResolvedValueOnce({
      max_canvas_elements: value,
      payload_budget_bytes: 1024 * 1024,
    });

    const limits = await getDashboardLimits();
    expect(limits.maxCanvasElements).toBeUndefined();
    expect(limits.payloadBudgetBytes).toBe(1024 * 1024);
  });

  it('payload_budget_bytes 만 상해도 요소 수는 살아남는다', async () => {
    getMock.mockResolvedValueOnce({ max_canvas_elements: 512, payload_budget_bytes: -3 });

    const limits = await getDashboardLimits();
    expect(limits.maxCanvasElements).toBe(512);
    expect(limits.payloadBudgetBytes).toBeUndefined();
  });
});
