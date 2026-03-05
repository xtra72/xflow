// 드래그로 패널 너비를 조절하는 훅.
// localStorage 에 너비를 저장하여 세션 간 유지한다.

import { useCallback, useEffect, useRef, useState } from 'react';

interface UseResizableOptions {
  /** localStorage 키 */
  storageKey: string;
  /** 기본 너비 (px) */
  defaultWidth: number;
  /** 최소 너비 (px) */
  minWidth: number;
  /** 최대 너비 (px) */
  maxWidth: number;
  /** 리사이즈 방향 (left: 왼쪽 경계에서 드래그) */
  side: 'left' | 'right';
}

interface UseResizableReturn {
  width: number;
  isDragging: boolean;
  handleMouseDown: (e: React.MouseEvent) => void;
}

export function useResizable({
  storageKey,
  defaultWidth,
  minWidth,
  maxWidth,
  side,
}: UseResizableOptions): UseResizableReturn {
  const [width, setWidth] = useState(() => {
    const saved = localStorage.getItem(storageKey);
    if (saved) {
      const n = Number(saved);
      if (n >= minWidth && n <= maxWidth) return n;
    }
    return defaultWidth;
  });

  const [isDragging, setIsDragging] = useState(false);
  const startXRef = useRef(0);
  const startWidthRef = useRef(0);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startXRef.current = e.clientX;
      startWidthRef.current = width;
      setIsDragging(true);
    },
    [width],
  );

  useEffect(() => {
    if (!isDragging) return;

    const handleMouseMove = (e: MouseEvent) => {
      const delta = e.clientX - startXRef.current;
      // left side: 마우스를 왼쪽으로 이동하면 패널이 커짐
      const newWidth = side === 'left'
        ? startWidthRef.current - delta
        : startWidthRef.current + delta;
      setWidth(Math.max(minWidth, Math.min(maxWidth, newWidth)));
    };

    const handleMouseUp = () => {
      setIsDragging(false);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging, minWidth, maxWidth, side]);

  // 너비가 변경되면 localStorage 에 저장
  useEffect(() => {
    localStorage.setItem(storageKey, String(width));
  }, [storageKey, width]);

  return { width, isDragging, handleMouseDown };
}
