// 낙관적 토글 훅.
// 토글 클릭 시 즉시 타겟 상태로 고정하고, 타임아웃까지 서버 상태 변동을 무시한다.
// 타임아웃 후 서버 상태로 동기화한다.

import { useEffect, useRef, useState } from 'react';

interface UseOptimisticToggleReturn {
  /** 화면에 표시할 값 (대기 중이면 타겟 값, 아니면 서버 값) */
  displayValue: boolean | undefined;
  /** 타겟 값 설정 (토글 클릭 시 호출) */
  setOptimistic: (value: boolean) => void;
  /** 서버 확인 대기 중 여부 */
  isPendingConfirmation: boolean;
}

/**
 * 서버 상태에 대한 낙관적 토글 훅.
 *
 * 동작:
 * 1. setOptimistic(target) 호출 → displayValue 즉시 target으로 전환
 * 2. 타임아웃 동안 서버 상태 변동 무시 (컨트롤러 오버라이드 진동 차단)
 * 3. 타임아웃 후 서버 상태로 복원 (확인 실패 시 원래 상태로 돌아감)
 *
 * @param serverValue 서버에서 받은 현재 상태
 * @param timeout 대기 타임아웃 (ms, 기본 5000)
 */
export function useOptimisticToggle(
  serverValue: boolean | undefined,
  timeout = 5000,
): UseOptimisticToggleReturn {
  const [optimistic, setOptimisticState] = useState<boolean | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // 클린업
  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const setOptimistic = (value: boolean) => {
    setOptimisticState(value);
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    // 타임아웃 후 서버 상태로 동기화
    timeoutRef.current = setTimeout(() => setOptimisticState(null), timeout);
  };

  const displayValue = optimistic !== null ? optimistic : serverValue;

  return {
    displayValue,
    setOptimistic,
    isPendingConfirmation: optimistic !== null,
  };
}
