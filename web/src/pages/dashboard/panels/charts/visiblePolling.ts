// 화면에 보일 때만 도는 폴링.
//
// 차트 폴링은 탭이 백그라운드로 가도 계속 돌았다. 대시보드는 오래 열어 두는
// 화면이라 보지 않는 동안의 조회가 그대로 쌓인다 — 패널 N개면 5초마다 N번,
// 캔들이면 그 4배다. 아무도 보지 않는 그림을 위한 질의다.
//
// WS 클라이언트는 이미 같은 판단을 한다(`wsClient` 의 visibilitychange 처리).
// 폴링만 어긋나 있던 것을 맞춘다.
//
// **다시 보이면 즉시 한 번 돈다.** 인터벌 재개만 하면 돌아온 직후 최대 한 주기
// 동안 옛 그림을 보게 되는데, 그건 "멈춰 있었다" 가 아니라 "틀린 값이 떠 있다"
// 로 읽힌다.

/** 문서 가시성 판정 — 테스트에서 갈아끼울 수 있도록 분리한다. */
export interface VisibilitySource {
  isVisible(): boolean;
  subscribe(onChange: () => void): () => void;
}

/** 브라우저 기본 구현. 문서가 없는 환경(SSR)에서는 항상 보이는 것으로 본다. */
export const documentVisibility: VisibilitySource = {
  isVisible: () => (typeof document === 'undefined' ? true : !document.hidden),
  subscribe: (onChange) => {
    if (typeof document === 'undefined') return () => {};
    document.addEventListener('visibilitychange', onChange);
    return () => document.removeEventListener('visibilitychange', onChange);
  },
};

export interface VisiblePollingOptions {
  /** 폴링 주기(ms). 0 이하이면 인터벌을 걸지 않고 즉시 1회만 돈다. */
  intervalMs: number;
  /** 매 주기 실행할 일. */
  run: () => void;
  /** 시작 시 즉시 1회 돌지. 기본 true. */
  runOnStart?: boolean;
  /** 가시성 판정. 기본은 document. */
  visibility?: VisibilitySource;
  /** 인터벌 등록/해제. 기본은 window. 테스트에서 갈아끼운다. */
  timer?: {
    set(cb: () => void, ms: number): number;
    clear(id: number): void;
  };
}

/**
 * 보이는 동안에만 도는 폴링을 시작한다.
 *
 * @returns 정지 함수. 인터벌과 가시성 구독을 모두 해제한다.
 */
export function startVisiblePolling(opts: VisiblePollingOptions): () => void {
  const {
    intervalMs,
    run,
    runOnStart = true,
    visibility = documentVisibility,
    timer = {
      set: (cb, ms) => window.setInterval(cb, ms),
      clear: (id) => window.clearInterval(id),
    },
  } = opts;

  let intervalId: number | undefined;
  let stopped = false;

  const stopInterval = (): void => {
    if (intervalId !== undefined) {
      timer.clear(intervalId);
      intervalId = undefined;
    }
  };

  const startInterval = (): void => {
    if (stopped || intervalId !== undefined || !(intervalMs > 0)) return;
    intervalId = timer.set(() => run(), intervalMs);
  };

  const onVisibilityChange = (): void => {
    if (stopped) return;
    if (visibility.isVisible()) {
      // 돌아온 즉시 한 번 — 옛 값이 떠 있는 시간을 없앤다.
      run();
      startInterval();
    } else {
      stopInterval();
    }
  };

  const unsubscribe = visibility.subscribe(onVisibilityChange);

  // 시작 시점의 1회는 가시성과 무관하다. 숨어 있어도 첫 그림은 있어야
  // 다시 보일 때 빈 차트가 아니다.
  if (runOnStart) run();
  if (visibility.isVisible()) startInterval();

  return () => {
    stopped = true;
    stopInterval();
    unsubscribe();
  };
}
