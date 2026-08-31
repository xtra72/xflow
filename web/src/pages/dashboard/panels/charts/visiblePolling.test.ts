// 보이는 동안에만 도는 폴링.

import { describe, expect, it, vi } from 'vitest';

import { startVisiblePolling, type VisibilitySource } from './visiblePolling';

/** 수동으로 시각을 넘기는 가짜 타이머. */
function fakeTimer() {
  const cbs = new Map<number, () => void>();
  let next = 1;
  return {
    api: {
      set(cb: () => void, _ms: number) {
        const id = next++;
        cbs.set(id, cb);
        return id;
      },
      clear(id: number) {
        cbs.delete(id);
      },
    },
    /** 등록된 인터벌을 한 번씩 발화시킨다. */
    tick() {
      for (const cb of [...cbs.values()]) cb();
    },
    get active() {
      return cbs.size;
    },
  };
}

/** 수동으로 켜고 끄는 가시성. */
function fakeVisibility(initial = true) {
  let visible = initial;
  const listeners = new Set<() => void>();
  const src: VisibilitySource = {
    isVisible: () => visible,
    subscribe: (fn) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
  };
  return {
    src,
    set(v: boolean) {
      visible = v;
      for (const fn of [...listeners]) fn();
    },
    get subscriberCount() {
      return listeners.size;
    },
  };
}

describe('startVisiblePolling', () => {
  it('시작 시 즉시 1회 돈다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    startVisiblePolling({ intervalMs: 1000, run, visibility: fakeVisibility().src, timer: t.api });
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('보이는 동안 주기마다 돈다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    startVisiblePolling({ intervalMs: 1000, run, visibility: fakeVisibility().src, timer: t.api });
    t.tick();
    t.tick();
    expect(run).toHaveBeenCalledTimes(3); // 시작 1 + 2회
  });

  it('숨으면 인터벌을 멈춘다 — 아무도 보지 않는 질의를 내지 않는다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    const vis = fakeVisibility();
    startVisiblePolling({ intervalMs: 1000, run, visibility: vis.src, timer: t.api });
    run.mockClear();

    vis.set(false);
    expect(t.active).toBe(0);
    t.tick();
    expect(run).not.toHaveBeenCalled();
  });

  it('다시 보이면 즉시 1회 돌고 인터벌을 재개한다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    const vis = fakeVisibility();
    startVisiblePolling({ intervalMs: 1000, run, visibility: vis.src, timer: t.api });
    vis.set(false);
    run.mockClear();

    vis.set(true);
    // 돌아온 직후 옛 값이 떠 있는 시간을 없앤다.
    expect(run).toHaveBeenCalledTimes(1);

    t.tick();
    expect(run).toHaveBeenCalledTimes(2);
  });

  it('숨은 채로 시작해도 첫 1회는 돈다 — 다시 보일 때 빈 차트가 아니게', () => {
    const run = vi.fn();
    const t = fakeTimer();
    startVisiblePolling({
      intervalMs: 1000,
      run,
      visibility: fakeVisibility(false).src,
      timer: t.api,
    });
    expect(run).toHaveBeenCalledTimes(1);
    // 다만 인터벌은 걸지 않는다.
    expect(t.active).toBe(0);
  });

  it('정지하면 인터벌과 가시성 구독을 모두 푼다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    const vis = fakeVisibility();
    const stop = startVisiblePolling({
      intervalMs: 1000,
      run,
      visibility: vis.src,
      timer: t.api,
    });
    stop();

    expect(t.active).toBe(0);
    expect(vis.subscriberCount).toBe(0);

    run.mockClear();
    vis.set(true);
    t.tick();
    expect(run).not.toHaveBeenCalled();
  });

  it('주기가 0 이하이면 인터벌 없이 1회만 돈다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    startVisiblePolling({ intervalMs: 0, run, visibility: fakeVisibility().src, timer: t.api });
    expect(run).toHaveBeenCalledTimes(1);
    expect(t.active).toBe(0);
  });

  it('runOnStart 를 끄면 시작 시 돌지 않는다', () => {
    const run = vi.fn();
    const t = fakeTimer();
    startVisiblePolling({
      intervalMs: 1000,
      run,
      runOnStart: false,
      visibility: fakeVisibility().src,
      timer: t.api,
    });
    expect(run).not.toHaveBeenCalled();
    t.tick();
    expect(run).toHaveBeenCalledTimes(1);
  });
});
