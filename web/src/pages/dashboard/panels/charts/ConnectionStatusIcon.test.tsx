// ConnectionStatusIcon 컴포넌트 테스트.

import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';

import { ConnectionStatusIcon } from './ConnectionStatusIcon';

describe('ConnectionStatusIcon', () => {
  const statuses = ['idle', 'connecting', 'connected', 'disconnected', 'closed', 'error'] as const;

  for (const s of statuses) {
    it(`${s} 상태 아이콘 렌더`, () => {
      const { getByTestId } = render(<ConnectionStatusIcon status={s} />);
      const icon = getByTestId('chart-status-icon');
      expect(icon).toBeInTheDocument();
      expect(icon.getAttribute('data-status')).toBe(s);
    });
  }
});
