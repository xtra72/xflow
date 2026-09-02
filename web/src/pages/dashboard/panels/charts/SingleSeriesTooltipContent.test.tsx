// 단일 값 툴팁 content 의 통합 검증.
//
// 순수 판정(tooltipSingle.test.ts)과 별개로, recharts 훅에서 읽은 값이 실제로
// payload 를 좁히는지 본다 — 판정이 맞아도 훅 배선이 어긋나면 아무 일도 안 난다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

const hookMocks = vi.hoisted(() => ({
  plotArea: { y: 0, height: 100 } as { y: number; height: number } | undefined,
  yDomain: [0, 100] as unknown[] | undefined,
}));

vi.mock('recharts', () => ({
  usePlotArea: () => hookMocks.plotArea,
  useYAxisDomain: () => hookMocks.yDomain,
}));

// 좁혀진 payload 를 그대로 노출하는 가짜 내용. 이 파일이 보는 것은 **판정 결과**뿐이며,
// 그리기(정렬·여백)는 ChartTooltipContent 자신의 테스트가 본다.
vi.mock('./ChartTooltipContent', () => ({
  ChartTooltipContent: ({ payload }: { payload?: Array<{ name?: string }> }) => (
    <div data-testid="content">{(payload ?? []).map((p) => p.name).join(',')}</div>
  ),
}));

import { SingleSeriesTooltipContent } from './SingleSeriesTooltipContent';

function renderContent(cursorY: number | undefined, values: Array<number | null>) {
  const payload = values.map((v, i) => ({ name: `s${i}`, value: v }));
  // 실제 Tooltip 이 넘겨주는 형상의 최소 부분집합. 나머지 필드는 이 컴포넌트가
  // 읽지 않고 기본 내용으로 그대로 흘려보내므로 채우지 않는다.
  const props = {
    payload,
    coordinate: cursorY === undefined ? undefined : { x: 0, y: cursorY },
    active: true,
    label: 0,
    accessibilityLayer: false,
    activeIndex: '0',
  } as unknown as Parameters<typeof SingleSeriesTooltipContent>[0];
  render(<SingleSeriesTooltipContent {...props} />);
  return screen.getByTestId('content').textContent;
}

describe('단일 값 툴팁 content', () => {
  it('커서 높이에 가장 가까운 시리즈만 남긴다', () => {
    // 도메인 0~100, 플롯 높이 100 → 커서 y=20 은 값 80.
    expect(renderContent(20, [10, 80, 40])).toBe('s1');
  });

  it('커서를 아래로 내리면 다른 시리즈가 선택된다', () => {
    // y=90 → 값 10.
    expect(renderContent(90, [10, 80, 40])).toBe('s0');
  });

  it('커서를 모르면 전부 보여 준다 — 임의로 고르지 않는다', () => {
    expect(renderContent(undefined, [10, 80, 40])).toBe('s0,s1,s2');
  });

  it('Y축 도메인이 범주형이면 전부 보여 준다', () => {
    hookMocks.yDomain = ['a', 'b'];
    expect(renderContent(20, [10, 80])).toBe('s0,s1');
    hookMocks.yDomain = [0, 100];
  });

  it('플롯 영역을 모르면 전부 보여 준다', () => {
    hookMocks.plotArea = undefined;
    expect(renderContent(20, [10, 80])).toBe('s0,s1');
    hookMocks.plotArea = { y: 0, height: 100 };
  });

  it('값이 없는 시리즈는 후보에서 빠진다', () => {
    expect(renderContent(20, [null, 80])).toBe('s1');
  });
});
