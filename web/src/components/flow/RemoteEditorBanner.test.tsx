// RemoteEditorBanner 테스트 — 원격 편집기 시각 구분 배너.
//
// 검증:
//   - 호스트명 + 플로우명을 표시한다.
//   - 노드로 돌아가기 링크가 backHref 를 가리킨다.
//   - 원본 instanceId 는 본문이 아닌 title(툴팁)로만 노출한다.
//   - 신규 모드에서 플로우명이 비면 "새 플로우" 라벨로 폴백한다.

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { RemoteEditorBanner } from './RemoteEditorBanner';

function renderBanner(props: Parameters<typeof RemoteEditorBanner>[0]) {
  return render(
    <MemoryRouter>
      <RemoteEditorBanner {...props} />
    </MemoryRouter>,
  );
}

describe('RemoteEditorBanner', () => {
  it('호스트명과 플로우명을 표시한다', () => {
    renderBanner({
      hostname: 'gw-1',
      instanceId: 'inst-uuid-1234',
      flowName: 'temperature-flow',
      isNew: false,
      backHref: '/admin/remote',
    });
    expect(screen.getByTestId('remote-editor-banner')).toBeInTheDocument();
    expect(screen.getByText('gw-1')).toBeInTheDocument();
    expect(screen.getByText('temperature-flow')).toBeInTheDocument();
  });

  it('노드로 돌아가기 링크가 backHref 를 가리킨다', () => {
    renderBanner({
      hostname: 'gw-1',
      instanceId: 'inst-uuid-1234',
      flowName: 'temperature-flow',
      isNew: false,
      backHref: '/admin/remote',
    });
    expect(screen.getByTestId('remote-editor-banner-back')).toHaveAttribute(
      'href',
      '/admin/remote',
    );
  });

  it('원본 instanceId 는 본문이 아닌 title(툴팁)로만 노출한다', () => {
    renderBanner({
      hostname: 'gw-1',
      instanceId: 'inst-uuid-1234',
      flowName: 'temperature-flow',
      isNew: false,
      backHref: '/admin/remote',
    });
    // 본문 텍스트에는 UUID 가 나타나지 않는다.
    expect(screen.queryByText('inst-uuid-1234')).not.toBeInTheDocument();
    // 단, title(툴팁) 속성으로는 노출된다.
    expect(screen.getByTestId('remote-editor-banner-target')).toHaveAttribute(
      'title',
      'inst-uuid-1234',
    );
  });

  it('신규 모드에서 플로우명이 비면 새 플로우 라벨로 폴백한다', () => {
    renderBanner({
      hostname: 'gw-1',
      instanceId: 'inst-uuid-1234',
      flowName: '',
      isNew: true,
      backHref: '/admin/remote',
    });
    expect(screen.getByText('remote.editor.newFlow')).toBeInTheDocument();
  });
});
