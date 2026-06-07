// RemoteEditorToolbar 테스트 — 원격 편집 대상 배지가 호스트명을 표시하고,
// 원본 instanceId(UUID)는 본문이 아닌 title(툴팁)로만 노출하는지 검증한다.
//
// 스토어(useEditorStore/useUIStore)는 실제 구현을 사용한다(초기값으로 충분).
// i18n 만 키 그대로 반환하도록 모킹한다.

import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { RemoteEditorToolbar } from './RemoteEditorToolbar';

function renderToolbar(props?: Partial<Parameters<typeof RemoteEditorToolbar>[0]>) {
  return render(
    <RemoteEditorToolbar
      nodeLabel={props?.nodeLabel ?? 'gw-1'}
      nodeTitle={props?.nodeTitle ?? 'inst-uuid-1234'}
      flowName={props?.flowName ?? 'temperature-flow'}
      isNew={props?.isNew ?? false}
      isSaving={props?.isSaving ?? false}
      onSave={props?.onSave ?? vi.fn()}
      showPortPanel={props?.showPortPanel ?? false}
      onTogglePortPanel={props?.onTogglePortPanel ?? vi.fn()}
    />,
  );
}

describe('RemoteEditorToolbar', () => {
  it('대상 노드 배지가 호스트명을 표시한다', () => {
    renderToolbar({ nodeLabel: 'gw-1' });
    const badge = screen.getByTestId('remote-editor-node-badge');
    expect(badge).toHaveTextContent('gw-1');
  });

  it('원본 instanceId(UUID)는 본문이 아닌 title(툴팁)로만 노출한다', () => {
    renderToolbar({ nodeLabel: 'gw-1', nodeTitle: 'inst-uuid-1234' });
    const badge = screen.getByTestId('remote-editor-node-badge');
    // 본문에는 UUID 가 나타나지 않는다.
    expect(badge).not.toHaveTextContent('inst-uuid-1234');
    // title 속성으로는 UUID 가 노출된다.
    expect(badge.getAttribute('title')).toContain('inst-uuid-1234');
  });

  it('flowName 을 표시한다', () => {
    renderToolbar({ flowName: 'temperature-flow' });
    expect(screen.getByText('temperature-flow')).toBeInTheDocument();
  });
});
