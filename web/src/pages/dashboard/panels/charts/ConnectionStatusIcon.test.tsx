// ConnectionStatusIcon 컴포넌트 테스트.

import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

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
