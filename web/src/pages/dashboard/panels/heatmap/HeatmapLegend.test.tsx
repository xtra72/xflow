// HeatmapLegend 렌더 테스트 (색표 범례, additive).
//
// 그라디언트 막대(히트맵과 동일 color_table), 등간 눈금 라벨(값/개수/방향별 배치),
// 자리(모서리 프리셋 + 드래그 자유 위치)/크기(치수 저장값 + 프리셋 파생), 눈금 라벨이
// 막대를 침범하지 않는 여백·정렬, 글자 서식, 퇴화(min==max) 방어, colorTable 폴백,
// 접근성(role/aria)을 컴포넌트 레벨에서 커버한다.

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup, within, fireEvent } from '@testing-library/react';

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import HeatmapLegend from './HeatmapLegend';
import type { ColorStop, LegendConfig } from './heatmapConfig';

afterEach(() => cleanup());

/** 기본 범례 config(테스트 오버라이드). */
function legendCfg(overrides: Partial<LegendConfig> = {}): LegendConfig {
  return {
    enabled: true,
    orientation: 'vertical',
    position: 'bottom-right',
    size: 'md',
    tick_count: 5,
    ...overrides,
  };
}

const TABLE: ColorStop[] = [
  { stop: 0, color: '#0000ff' },
  { stop: 1, color: '#ff0000' },
];

describe('HeatmapLegend — 구조/접근성', () => {
  it('role=img + aria-label 을 가진 범례 컨테이너를 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 18, max: 26 }} colorTable={TABLE} legend={legendCfg()} />,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBeTruthy();
    expect(el.className).toContain('pointer-events-none');
  });

  it('tick_count 개수만큼 눈금 라벨을 렌더하고 양 끝은 min/max 값이다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 18, max: 26 }} colorTable={TABLE} legend={legendCfg({ tick_count: 5 })} />,
    );
    const legend = getByTestId('heatmap-legend');
    const ticks = within(legend).getAllByTestId(/heatmap-legend-tick-/);
    expect(ticks.length).toBe(5);
    // i=0(min)=18.0, i=last(max)=26.0. 소수 1자리 포맷.
    expect(ticks[0]!.textContent).toBe('18.0');
    expect(ticks[4]!.textContent).toBe('26.0');
    // 중앙(i=2) = 22.0.
    expect(ticks[2]!.textContent).toBe('22.0');
  });
});

describe('HeatmapLegend — 방향/그라디언트', () => {
  it('세로는 flex-row 배치 + 그라디언트 to top', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg({ orientation: 'vertical' })} />,
    );
    const legend = getByTestId('heatmap-legend');
    expect(legend.className).toContain('flex-row');
    // 막대(첫 자식 div)의 그라디언트 방향.
    const bar = legend.querySelector('div')!;
    expect(bar.style.background).toContain('to top');
  });

  it('가로는 flex-col 배치 + 그라디언트 to right', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg({ orientation: 'horizontal' })} />,
    );
    const legend = getByTestId('heatmap-legend');
    expect(legend.className).toContain('flex-col');
    const bar = legend.querySelector('div')!;
    expect(bar.style.background).toContain('to right');
  });
});

describe('HeatmapLegend — 위치/크기', () => {
  it('position 에 따라 모서리 배치 클래스를 적용한다', () => {
    const { getByTestId, rerender } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ position: 'top-left' })} />,
    );
    expect(getByTestId('heatmap-legend').className).toContain('top-2');
    expect(getByTestId('heatmap-legend').className).toContain('left-2');

    rerender(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ position: 'bottom-right' })} />,
    );
    expect(getByTestId('heatmap-legend').className).toContain('bottom-2');
    expect(getByTestId('heatmap-legend').className).toContain('right-2');
  });

  it('크기 프리셋에 따라 막대 길이가 달라진다(sm < lg)', () => {
    const { getByTestId, rerender } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ size: 'sm' })} />,
    );
    const smBar = getByTestId('heatmap-legend').querySelector('div')!;
    const smHeight = smBar.style.height;

    rerender(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ size: 'lg' })} />,
    );
    const lgBar = getByTestId('heatmap-legend').querySelector('div')!;
    expect(parseInt(lgBar.style.height, 10)).toBeGreaterThan(parseInt(smHeight, 10));
  });
});

describe('HeatmapLegend — 견고성', () => {
  it('min==max(퇴화) 이면 단일 라벨만 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 21, max: 21 }} colorTable={TABLE} legend={legendCfg({ tick_count: 5 })} />,
    );
    const ticks = within(getByTestId('heatmap-legend')).getAllByTestId(/heatmap-legend-tick-/);
    expect(ticks.length).toBe(1);
    expect(ticks[0]!.textContent).toBe('21.0');
  });

  it('colorTable 이 비면 DEFAULT_COLOR_TABLE 로 폴백해 그라디언트를 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={[]} legend={legendCfg()} />,
    );
    const bar = getByTestId('heatmap-legend').querySelector('div')!;
    // 기본 gradient 의 첫 색(#2166ac)이 포함된다.
    expect(bar.style.background).toContain('#2166ac');
  });
});

describe('HeatmapLegend — 드래그 이동(편집모드 한정)', () => {
  // jsdom 에는 PointerEvent 가 없어 fireEvent.pointerX 가 좌표 없는 일반 Event 로 폴백한다.
  // MouseEvent 기반 대역을 등록해 clientX/clientY 가 실제로 전달되게 한다.
  class FakePointerEvent extends MouseEvent {
    pointerId = 1;
  }
  (globalThis as unknown as { PointerEvent: typeof MouseEvent }).PointerEvent =
    FakePointerEvent as unknown as typeof MouseEvent;

  /**
   * jsdom 은 레이아웃을 계산하지 않아 getBoundingClientRect 가 전부 0 이다. 부모(컨테이너)와
   * 범례 자신의 rect 를 주입해 정규화 변환만 검증한다(픽셀 렌더는 브라우저 책임).
   */
  function withRects(container: HTMLElement, legend: HTMLElement) {
    const parent = legend.parentElement!;
    vi.spyOn(parent, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      width: 200,
      height: 100,
      right: 200,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);
    vi.spyOn(legend, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      width: 20,
      height: 20,
      right: 20,
      bottom: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);
    void container;
  }

  /** jsdom 에 없는 PointerEvent capture API 를 no-op 으로 채운다. */
  function stubPointerCapture(el: HTMLElement) {
    (el as unknown as { setPointerCapture: (id: number) => void }).setPointerCapture = () => {};
    (el as unknown as { hasPointerCapture: (id: number) => boolean }).hasPointerCapture = () => false;
    (el as unknown as { releasePointerCapture: (id: number) => void }).releasePointerCapture =
      () => {};
  }

  it('draggable=false(뷰어)에서는 pointer-events-none 이 유지된다 — 아래 레이어 클릭 보존', () => {
    const onOffsetChange = vi.fn();
    const { getByTestId } = render(
      <div>
        <HeatmapLegend
          bounds={{ min: 0, max: 10 }}
          colorTable={TABLE}
          legend={legendCfg()}
          onOffsetChange={onOffsetChange}
        />
      </div>,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.className).toContain('pointer-events-none');
    stubPointerCapture(el);
    fireEvent.pointerDown(el, { clientX: 10, clientY: 10 });
    expect(onOffsetChange).not.toHaveBeenCalled();
  });

  it('draggable=true 이면 포인터 이벤트를 받고 드래그가 정규화 좌표를 저장한다', () => {
    const onOffsetChange = vi.fn();
    const { container, getByTestId } = render(
      <div>
        <HeatmapLegend
          bounds={{ min: 0, max: 10 }}
          colorTable={TABLE}
          legend={legendCfg()}
          draggable
          onOffsetChange={onOffsetChange}
        />
      </div>,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.className).toContain('pointer-events-auto');
    withRects(container, el);
    stubPointerCapture(el);
    // 좌상단(0,0)을 잡고 (100,50)으로 끈다 → 200x100 컨테이너에서 (0.5, 0.5).
    fireEvent.pointerDown(el, { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(el, { clientX: 100, clientY: 50 });
    expect(onOffsetChange).toHaveBeenLastCalledWith({ x: 0.5, y: 0.5 });
  });

  it('컨테이너 밖으로 끌어도 범례가 안에 남도록 clamp 된다', () => {
    const onOffsetChange = vi.fn();
    const { container, getByTestId } = render(
      <div>
        <HeatmapLegend
          bounds={{ min: 0, max: 10 }}
          colorTable={TABLE}
          legend={legendCfg()}
          draggable
          onOffsetChange={onOffsetChange}
        />
      </div>,
    );
    const el = getByTestId('heatmap-legend');
    withRects(container, el);
    stubPointerCapture(el);
    fireEvent.pointerDown(el, { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(el, { clientX: 9999, clientY: 9999 });
    // 범례 20x20 / 컨테이너 200x100 → 상한 x=(200-20)/200=0.9, y=(100-20)/100=0.8.
    expect(onOffsetChange).toHaveBeenLastCalledWith({ x: 0.9, y: 0.8 });
  });

  it('offset 이 설정되면 모서리 프리셋 대신 % 좌표로 배치한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ position: 'bottom-right', offset: { x: 0.25, y: 0.75 } })}
      />,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.style.left).toBe('25%');
    expect(el.style.top).toBe('75%');
    // 모서리 프리셋 클래스는 붙지 않는다(둘이 겹치면 배치가 어긋난다).
    expect(el.className).not.toContain('bottom-2');
  });

  it('offset 미설정(기본)이면 기존 모서리 프리셋 그대로다(회귀 0)', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ position: 'top-left' })}
      />,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.className).toContain('top-2');
    expect(el.className).toContain('left-2');
    expect(el.style.left).toBe('');
  });
});

describe('HeatmapLegend — 눈금 라벨이 막대와 겹치지 않는다', () => {
  it('라벨 상자가 실제 크기를 가져 범례 배경이 라벨까지 감싼다', () => {
    // 라벨은 전부 absolute 라 상자를 비워 두면 폭이 0 이 되고, 글자가 배경 밖으로 흘러
    // 히트맵 위에 떠서 막대와 뒤엉켜 보인다(보고된 겹침).
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: -12.5, max: 133.5 }}
        colorTable={TABLE}
        legend={legendCfg({ orientation: 'vertical' })}
      />,
    );
    const box = getByTestId('heatmap-legend-ticks');
    expect(parseFloat(box.style.width)).toBeGreaterThan(0);
  });

  it('막대와 라벨 사이 여백이 글자 크기를 따라 커진다(고정 여백은 큰 글자에서 붙어 보인다)', () => {
    const small = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ font_size: 8 })}
      />,
    ).getByTestId('heatmap-legend');
    const smallGap = parseFloat(small.style.gap);
    cleanup();
    const large = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ font_size: 32 })}
      />,
    ).getByTestId('heatmap-legend');
    expect(parseFloat(large.style.gap)).toBeGreaterThan(smallGap);
  });

  it('양 끝 라벨은 가운데 정렬을 버려 막대 범위 밖으로 나가지 않는다(가로)', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ orientation: 'horizontal', tick_count: 3 })}
      />,
    );
    // 첫 라벨은 왼쪽 끝에 맞추고, 마지막 라벨은 오른쪽 끝에 맞춘다. 가운데만 -50%.
    expect(getByTestId('heatmap-legend-tick-0').style.transform).toBe('translateX(0%)');
    expect(getByTestId('heatmap-legend-tick-1').style.transform).toBe('translateX(-50%)');
    expect(getByTestId('heatmap-legend-tick-2').style.transform).toBe('translateX(-100%)');
  });

  it('양 끝 라벨은 세로에서도 막대 범위 안에 남는다(위치축이 반대다)', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ orientation: 'vertical', tick_count: 3 })}
      />,
    );
    // i=0(min)은 아래 끝이라 위로 올려 붙이고, i=last(max)는 위 끝이라 내려 붙인다.
    expect(getByTestId('heatmap-legend-tick-0').style.transform).toBe('translateY(-100%)');
    expect(getByTestId('heatmap-legend-tick-1').style.transform).toBe('translateY(-50%)');
    expect(getByTestId('heatmap-legend-tick-2').style.transform).toBe('translateY(0%)');
  });
});

describe('HeatmapLegend — 크기와 글자 서식', () => {
  it('치수 저장값이 막대 크기를 정한다(세로: 두께=폭, 길이=높이)', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ bar_length: 200, bar_thickness: 24 })}
      />,
    );
    const bar = getByTestId('heatmap-legend').firstElementChild as HTMLElement;
    expect(bar.style.width).toBe('24px');
    expect(bar.style.height).toBe('200px');
  });

  it('치수가 없으면 기존 프리셋에서 파생한다(프리셋으로 저장된 패널의 그림 보존)', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg({ size: 'lg' })} />,
    );
    const bar = getByTestId('heatmap-legend').firstElementChild as HTMLElement;
    expect(bar.style.width).toBe('14px');
    expect(bar.style.height).toBe('140px');
  });

  it('글자 크기·색 지정이 눈금에 반영된다', () => {
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ font_size: 20, font_color: '#ff0000' })}
      />,
    );
    const box = getByTestId('heatmap-legend-ticks');
    expect(box.style.fontSize).toBe('20px');
    expect(box.style.color).toBe('rgb(255, 0, 0)');
  });

  it('글자 색 미지정이면 색을 강제하지 않는다(테마 상속)', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg()} />,
    );
    expect(getByTestId('heatmap-legend-ticks').style.color).toBe('');
  });
});

describe('HeatmapLegend — 손잡이로 크기 조절', () => {
  it('뷰어(draggable=false)에는 손잡이가 없다', () => {
    const { queryByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg()} />,
    );
    expect(queryByTestId('heatmap-legend-resize')).toBeNull();
  });

  it('세로 막대: 아래로 끌면 길어지고 옆으로 끌면 두꺼워진다', () => {
    const onSizeChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ bar_length: 100, bar_thickness: 10 })}
        draggable
        onOffsetChange={vi.fn()}
        onSizeChange={onSizeChange}
      />,
    );
    const handle = getByTestId('heatmap-legend-resize');
    fireEvent.pointerDown(handle, { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(handle, { clientX: 6, clientY: 40 });
    expect(onSizeChange).toHaveBeenLastCalledWith({ bar_length: 140, bar_thickness: 16 });
  });

  it('가로 막대: 축이 바뀐다(오른쪽=길이, 아래=두께)', () => {
    const onSizeChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ orientation: 'horizontal', bar_length: 100, bar_thickness: 10 })}
        draggable
        onOffsetChange={vi.fn()}
        onSizeChange={onSizeChange}
      />,
    );
    const handle = getByTestId('heatmap-legend-resize');
    fireEvent.pointerDown(handle, { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(handle, { clientX: 40, clientY: 6 });
    expect(onSizeChange).toHaveBeenLastCalledWith({ bar_length: 140, bar_thickness: 16 });
  });

  it('안쪽으로 끝까지 끌어도 하한 아래로는 줄지 않는다(다시 잡을 수 없게 되는 것을 막는다)', () => {
    const onSizeChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg({ bar_length: 100, bar_thickness: 10 })}
        draggable
        onOffsetChange={vi.fn()}
        onSizeChange={onSizeChange}
      />,
    );
    const handle = getByTestId('heatmap-legend-resize');
    fireEvent.pointerDown(handle, { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(handle, { clientX: -9999, clientY: -9999 });
    expect(onSizeChange).toHaveBeenLastCalledWith({ bar_length: 24, bar_thickness: 4 });
  });

  it('손잡이 누름은 이동 드래그를 시작시키지 않는다(같은 누름을 나눠 갖지 않는다)', () => {
    const onOffsetChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapLegend
        bounds={{ min: 0, max: 10 }}
        colorTable={TABLE}
        legend={legendCfg()}
        draggable
        onOffsetChange={onOffsetChange}
        onSizeChange={vi.fn()}
      />,
    );
    fireEvent.pointerDown(getByTestId('heatmap-legend-resize'), { clientX: 0, clientY: 0 });
    fireEvent.pointerMove(getByTestId('heatmap-legend'), { clientX: 50, clientY: 50 });
    expect(onOffsetChange).not.toHaveBeenCalled();
  });
});
