// ModbusPanelFrame 헤더 게이팅 테스트.
//
// MODBUS 6종 패널이 이 프레임을 34곳에서 호출하므로, 타이틀 바 표시/숨김이 프레임 한 곳에서
// 결정되는지가 곧 6종 전체의 동작이다.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { PanelChromeProvider } from '../../PanelChromeProvider';
import { ModbusPanelFrame } from './panelChrome';

describe('ModbusPanelFrame — 타이틀 바 게이팅', () => {
  it('기본(Provider 없음)은 타이틀을 그린다 — 기존 동작', () => {
    render(<ModbusPanelFrame title="게이트웨이">본문</ModbusPanelFrame>);
    expect(screen.getByText('게이트웨이')).toBeInTheDocument();
  });

  it('showTitle=false 면 타이틀 바를 그리지 않는다', () => {
    render(
      <PanelChromeProvider config={{ showTitle: false }}>
        <ModbusPanelFrame title="게이트웨이">본문</ModbusPanelFrame>
      </PanelChromeProvider>,
    );
    expect(screen.queryByText('게이트웨이')).toBeNull();
  });

  it('타이틀을 숨겨도 본문은 그대로 렌더된다(숨김이 내용을 지우지 않는다)', () => {
    render(
      <PanelChromeProvider config={{ showTitle: false }}>
        <ModbusPanelFrame title="게이트웨이">본문</ModbusPanelFrame>
      </PanelChromeProvider>,
    );
    expect(screen.getByText('본문')).toBeInTheDocument();
  });

  it('showTitle 미설정 config 는 표시로 폴백한다', () => {
    render(
      <PanelChromeProvider config={{}}>
        <ModbusPanelFrame title="게이트웨이">본문</ModbusPanelFrame>
      </PanelChromeProvider>,
    );
    expect(screen.getByText('게이트웨이')).toBeInTheDocument();
  });
});
