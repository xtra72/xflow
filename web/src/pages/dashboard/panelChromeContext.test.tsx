// 패널 크롬 context 테스트 — 기본 표시(회귀 0) + 명시적 false 만 숨김.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { PanelChromeProvider } from './PanelChromeProvider';
import { readPanelChrome, usePanelTitleVisible } from './panelChromeContext';

function Probe() {
  return <span data-testid="probe">{usePanelTitleVisible() ? 'shown' : 'hidden'}</span>;
}

describe('readPanelChrome', () => {
  it('showTitle 이 명시적으로 false 일 때만 숨긴다', () => {
    expect(readPanelChrome({ showTitle: false }).showTitle).toBe(false);
  });

  it('미설정/true/손상 값은 표시로 폴백한다(기존 패널 회귀 0)', () => {
    expect(readPanelChrome(undefined).showTitle).toBe(true);
    expect(readPanelChrome({}).showTitle).toBe(true);
    expect(readPanelChrome({ showTitle: true }).showTitle).toBe(true);
    // 문자열 'false' 같은 손상 값도 숨김으로 보지 않는다 — 의도치 않은 숨김이 더 나쁘다.
    expect(readPanelChrome({ showTitle: 'false' }).showTitle).toBe(true);
  });
});

describe('usePanelTitleVisible', () => {
  it('Provider 밖에서는 표시다 — 패널 단독 렌더가 그대로 동작한다', () => {
    render(<Probe />);
    expect(screen.getByTestId('probe').textContent).toBe('shown');
  });

  it('Provider 가 config 의 showTitle=false 를 전파한다', () => {
    render(
      <PanelChromeProvider config={{ showTitle: false }}>
        <Probe />
      </PanelChromeProvider>,
    );
    expect(screen.getByTestId('probe').textContent).toBe('hidden');
  });

  it('중첩된 깊이와 무관하게 읽힌다(공용 프레임이 안쪽에서 읽는 경로)', () => {
    render(
      <PanelChromeProvider config={{ showTitle: false }}>
        <div>
          <div>
            <Probe />
          </div>
        </div>
      </PanelChromeProvider>,
    );
    expect(screen.getByTestId('probe').textContent).toBe('hidden');
  });
});
