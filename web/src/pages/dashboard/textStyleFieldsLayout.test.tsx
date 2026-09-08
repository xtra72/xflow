// 글자 스타일 편집 칸의 배치 검증.
//
// 이 칸들은 늘 좁은 디자인 팝오버(w-72 = 288px) 안에 들어간다. 다섯 칸을 한 줄에 두면
// 고정폭만 220px 을 넘어 글꼴 칸이 짜부라지고 마지막 칸이 상자 밖으로 밀려난다 —
// 화면에서만 드러나고 타입·테스트는 통과하는 종류의 결함이라 배치를 명시적으로 잠근다.

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { TextStyleFields } from './ChartPanelSections';
import { I18nProvider } from '@/lib/i18n';

function renderFields() {
  return render(
    <I18nProvider>
      <TextStyleFields
        label="라벨"
        family={undefined}
        size={undefined}
        color={undefined}
        weight="inherit"
        sizePlaceholder="상속"
        testIdPrefix="tsf"
        onChange={vi.fn()}
      />
    </I18nProvider>,
  );
}

describe('TextStyleFields 배치', () => {
  it('글꼴은 한 줄을 다 쓴다 — 좁은 칸에 몰아넣으면 이름이 보이지 않는다', () => {
    renderFields();
    expect(screen.getByTestId('tsf-family').className).toContain('w-full');
  });

  it('글꼴과 나머지 칸은 서로 다른 줄에 있다', () => {
    renderFields();

    const family = screen.getByTestId('tsf-family');
    const size = screen.getByTestId('tsf-size');
    // 같은 부모에 나란히 있으면 한 줄이라는 뜻이다.
    expect(size.parentElement).not.toBe(family.parentElement);
  });

  it('둘째 줄은 크기·색·굵기를 함께 담는다', () => {
    renderFields();

    const row = screen.getByTestId('tsf-size').parentElement!;
    expect(row).toContainElement(screen.getByTestId('tsf-color'));
    expect(row).toContainElement(screen.getByTestId('tsf-weight'));
  });

  it('굵기 칸은 남는 폭을 쓰되 줄어들 수 있어야 한다 — 고정폭이면 넘친다', () => {
    renderFields();

    const weight = screen.getByTestId('tsf-weight').className;
    expect(weight).toContain('flex-1');
    expect(weight).toContain('min-w-0');
  });
});
