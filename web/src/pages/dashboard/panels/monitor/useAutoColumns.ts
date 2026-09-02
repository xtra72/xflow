// 열 개수 자동 조절 훅.
//
// 사용자가 정한 것은 열 개수의 "상한"이다. 패널이 좁아지면 항목이 찌그러지는 대신
// 열 수를 줄인다 — 대시보드는 그리드 폭이 자유롭게 바뀌므로 고정 열 수는 거의 항상
// 어느 한쪽(넓을 때 여백 과다 / 좁을 때 글자 잘림)에서 깨진다.

import { useEffect, useRef, useState } from 'react';

/**
 * 컨테이너 폭을 관찰해 실제 열 수를 계산한다.
 *
 * @param maxCols 사용자가 정한 열 개수 상한
 * @param minItemWidth 항목 하나가 읽히려면 필요한 최소 폭(px)
 * @returns [컨테이너에 붙일 ref, 실제 열 수]
 *
 * 폭을 아직 재지 못했으면(초기 렌더, ResizeObserver 없는 환경) 상한을 그대로 쓴다.
 * 0 으로 두면 첫 프레임에 1열로 깜빡였다가 펴지는 것이 보인다.
 */
export function useAutoColumns(
  maxCols: number,
  minItemWidth: number,
): [React.RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === 'undefined') return;

    const observer = new ResizeObserver((entries) => {
      const next = entries[0]?.contentRect.width ?? 0;
      // 소수점 흔들림으로 매 프레임 상태를 갈아치우지 않게 정수로 맞춘다.
      setWidth((prev) => (Math.round(prev) === Math.round(next) ? prev : next));
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const cols =
    width <= 0 ? maxCols : Math.min(maxCols, Math.max(1, Math.floor(width / minItemWidth)));

  return [ref, cols];
}
