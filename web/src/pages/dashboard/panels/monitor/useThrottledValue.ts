// 값 갱신 throttle 훅.
//
// 실시간 스트림은 초당 여러 번 바뀔 수 있다. 패널이 그 속도로 다시 그릴 이유는 없고,
// 차트가 많이 붙은 대시보드에서는 그대로 CPU 부담이 된다. 사용자가 정한 갱신 주기로
// 조여 준다.

import { useEffect, useRef, useState } from 'react';

/**
 * `value` 를 최대 `intervalMs` 마다 한 번만 반영해 돌려준다.
 *
 * 첫 값은 지연 없이 그대로 통과시킨다 — 패널을 열자마자 빈 화면을 보는 것을 막는다.
 * 주기 안에 들어온 값은 버리지 않고 보관했다가 주기가 끝나면 최신값으로 반영한다.
 */
export function useThrottledValue<T>(value: T, intervalMs: number): T {
  const [shown, setShown] = useState(value);
  const latest = useRef(value);
  const lastEmit = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  latest.current = value;

  useEffect(() => {
    // 주기가 사실상 없으면 throttle 하지 않는다.
    if (intervalMs <= 0) {
      setShown(value);
      return;
    }

    const now = Date.now();
    const elapsed = now - lastEmit.current;

    if (elapsed >= intervalMs) {
      lastEmit.current = now;
      setShown(value);
      return;
    }

    if (timer.current !== null) return; // 이미 다음 반영이 예약되어 있다.
    timer.current = setTimeout(() => {
      timer.current = null;
      lastEmit.current = Date.now();
      setShown(latest.current);
    }, intervalMs - elapsed);
  }, [value, intervalMs]);

  // 언마운트 시 예약된 타이머 정리.
  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current);
    },
    [],
  );

  return shown;
}
