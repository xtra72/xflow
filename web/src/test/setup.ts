// Vitest global setup.
// @testing-library/jest-dom matchers 를 모든 테스트에 자동 주입한다.

import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// React Testing Library 는 각 테스트 후 DOM 을 정리해야 한다.
afterEach(() => {
  cleanup();
});

// ResizeObserver 폴리필 (Recharts ResponsiveContainer 용).
// jsdom 환경에서는 기본 제공되지 않는다.
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = ResizeObserverMock as unknown as typeof ResizeObserver;
}
