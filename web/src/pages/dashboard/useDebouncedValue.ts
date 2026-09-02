// 값 변경을 디바운스하여 반환하는 훅. 라이브 미리보기가 잦은 편집에 과도하게
// 재렌더/재조회되지 않도록 보호한다(R3).
//
// @spec SPEC-PANEL-SETTINGS-001 (T9, AC-13)

import { useEffect, useState } from 'react';

/** value 가 delayMs 동안 안정되면 그 값을 반환한다(그 전까지는 직전 안정값 유지). */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState<T>(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}
