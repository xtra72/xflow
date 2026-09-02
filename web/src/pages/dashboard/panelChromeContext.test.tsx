// 패널 크롬 context 테스트 — 기본 표시(회귀 0) + 명시적 false 만 숨김.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { PanelChromeProvider } from './PanelChromeProvider';
import {
  readPanelChrome,
  resolvePanelTitleStyle,
  usePanelTitleStyle,
  usePanelTitleVisible,
} from './panelChromeContext';

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

describe('resolvePanelTitleStyle', () => {
  // 미설정이 `undefined` 인 것이 핵심이다. 각 패널의 타이틀은 Tailwind 클래스로 크기·굵기·
  // 색을 이미 정해 두었고 인라인 스타일이 그것을 이긴다 — 빈 객체나 undefined 값을 가진
  // 키를 넘기면 저장된 대시보드의 타이틀이 조용히 바뀐다.
  it('미설정이면 undefined — 저장된 패널의 타이틀이 변하지 않는다', () => {
    expect(resolvePanelTitleStyle(undefined)).toBeUndefined();
    expect(resolvePanelTitleStyle({})).toBeUndefined();
  });

  it('지정한 항목만 넣는다', () => {
    expect(resolvePanelTitleStyle({ size: 20 })).toEqual({ fontSize: '20px' });
    expect(resolvePanelTitleStyle({ color: '#ff0000' })).toEqual({ color: '#ff0000' });
    expect(resolvePanelTitleStyle({ weight: 'bold' })).toEqual({ fontWeight: 'bold' });
  });

  it('글꼴 토큰을 스택으로 편다', () => {
    const style = resolvePanelTitleStyle({ family: 'mono' });
    expect(style?.fontFamily).toContain('ui-monospace');
  });

  it('인식 불가 값은 넣지 않는다 — 임의 색으로 떨어뜨리면 어두운 배경에서 글자가 사라진다', () => {
    expect(resolvePanelTitleStyle({ color: 'red', size: 0, family: 'comic' })).toBeUndefined();
  });

  it('키가 없는 스타일 객체를 만들지 않는다 — 뒤에 펼쳐 앞의 색을 지우면 안 된다', () => {
    const style = resolvePanelTitleStyle({ size: 14 });
    expect(Object.keys(style ?? {})).toEqual(['fontSize']);
  });
});

function StyleProbe() {
  const style = usePanelTitleStyle();
  return <span data-testid="style-probe" style={style} />;
}

describe('usePanelTitleStyle', () => {
  it('Provider 밖에서는 스타일이 없다 — 패널 단독 렌더가 그대로 동작한다', () => {
    render(<StyleProbe />);
    expect(screen.getByTestId('style-probe').getAttribute('style')).toBeNull();
  });

  it('Provider 가 config 의 title_font 를 전파한다', () => {
    render(
      <PanelChromeProvider config={{ title_font: { size: 18, weight: 'bold' } }}>
        <StyleProbe />
      </PanelChromeProvider>,
    );
    const style = screen.getByTestId('style-probe').getAttribute('style') ?? '';
    expect(style).toContain('font-size: 18px');
    expect(style).toContain('font-weight: bold');
  });

  it('readPanelChrome 도 같은 값을 만든다 — 두 경로가 갈리지 않는다', () => {
    expect(readPanelChrome({ title_font: { size: 18 } }).titleStyle).toEqual(
      resolvePanelTitleStyle({ size: 18 }),
    );
  });
});
