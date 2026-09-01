// 패널 리사이즈 시 센서 마커가 도면 위에서 미끄러지지 않는지 검증한다.
//
// 배경(결함): 레거시 좌표(`sensor_space` 미지정)는 "패널 본문의 몇 %" 라는 뜻이라 도면이
// 어디에 레터박스되는지를 모른다. 그것을 렌더마다 현재 실측으로 스테이지 좌표로 풀면, 패널
// 종횡비가 바뀔 때마다 같은 좌표가 다른 자리로 풀려 마커가 도면 위에서 미끄러졌다
// (보고: "패널 크기를 바꾸면 이미지와 센서 자리가 따로 논다").
//
// 여기서는 마운트된 패널의 실측 크기를 바꿔(ResizeObserver 콜백) 리사이즈를 흉내낸다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, act, cleanup } from '@testing-library/react';

import type { ChartEntry } from '../charts/chartChannelTypes';
import type { UseStoreChartDataResult } from '../charts/useStoreChartData';

const idle: UseStoreChartDataResult = {
  entries: [],
  seriesEntries: new Map(),
  seriesStyles: new Map(),
  booleanSeries: new Set(),
  seriesNames: [],
  status: 'idle',
};
const storeMock = { current: idle };
vi.mock('../charts/useStoreChartData', () => ({ useStoreChartData: () => storeMock.current }));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
// 도면 종횡비를 즉시 확정한다(비동기 이미지 디코드 없이 결정적으로).
vi.mock('./useFloorPlanAspect', () => ({ useFloorPlanAspect: () => 2 })); // 2:1 도면

import HeatmapPanel from './HeatmapPanel';
import { heatmapSensorId } from './sensorIdentity';

const sid = (key: string) => heatmapSensorId({ key });
const tid = (s: string) => s.trim().replace(/\s+/g, ' ');
const reading = (v: number): ChartEntry[] => [{ timestamp: 1, value: v }];

/** 현재 실측 크기(모든 getBoundingClientRect 가 이 값을 돌려준다). */
const size = { w: 400, h: 200 };
/** 마운트된 ResizeObserver 콜백들 — 리사이즈를 흉내낼 때 직접 호출한다. */
const observers: Array<() => void> = [];

beforeEach(() => {
  observers.length = 0;
  size.w = 400;
  size.h = 200;
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({
        left: 0, top: 0, width: size.w, height: size.h,
        right: size.w, bottom: size.h, x: 0, y: 0, toJSON: () => ({}),
      }) as DOMRect,
  );
  (globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = class {
    constructor(private cb: (entries: unknown[], obs: unknown) => void) {
      observers.push(() => this.cb([], this));
    }
    observe() {}
    disconnect() {}
  };
  storeMock.current = {
    ...idle,
    seriesNames: [sid('s1')],
    seriesEntries: new Map([[sid('s1'), reading(22)]]),
    status: 'connected',
  };
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function config(extra: Record<string, unknown> = {}) {
  return {
    data_source: 'store',
    store_source: {
      agent_name: 'a', namespace: 'default', selection_mode: 'keys',
      series: [{ key: 's1' }],
      time_window_ms: 1000, interval_ms: 1000, aggregation: 'last',
    },
    // 비중심 좌표 — 중심(0.5,0.5)은 어떤 환산에도 불변이라 미끄러짐이 드러나지 않는다.
    sensor_positions: { [sid('s1')]: { x: 0.2, y: 0.8 } },
    idw: { power: 2, grid_resolution: 8 },
    floor_plan: { image: 'data:image/png;base64,AAAA' },
    ...extra,
  } as Record<string, unknown>;
}

/** 패널 폭을 넓혀(400x200 → 800x200) 리사이즈를 발생시킨다. */
function resizeWider() {
  act(() => {
    size.w = 800;
    size.h = 200;
    observers.forEach((fire) => fire());
  });
}

describe('HeatmapPanel — 리사이즈해도 마커가 도면 위 같은 지점에 남는다', () => {
  it('레거시 좌표(sensor_space 미지정)도 리사이즈로 미끄러지지 않는다', () => {
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={vi.fn()} forcePlacement />,
    );
    const at = () => {
      const m = getByTestId(`sensor-marker-${tid(sid('s1'))}`) as HTMLElement;
      return `${m.style.left} ${m.style.top}`;
    };
    const before = at();
    resizeWider();
    expect(at()).toBe(before);
  });

  it('스테이지 좌표(신규)는 종전대로 불변이다(회귀 0)', () => {
    const { getByTestId } = render(
      <HeatmapPanel
        panelId="p"
        config={config({ sensor_space: 'stage' })}
        onConfigChange={vi.fn()}
        forcePlacement
      />,
    );
    const at = () => {
      const m = getByTestId(`sensor-marker-${tid(sid('s1'))}`) as HTMLElement;
      return `${m.style.left} ${m.style.top}`;
    };
    expect(at()).toBe('20% 80%');
    resizeWider();
    expect(at()).toBe('20% 80%');
  });

  it('스테이지 박스 자체는 리사이즈를 따라간다(도면은 계속 가운데 레터박스)', () => {
    // 마커가 안 움직인다는 것이 "스테이지가 굳었다" 는 뜻이 아님을 못 박는다.
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={vi.fn()} forcePlacement />,
    );
    const box = () => {
      const s = getByTestId('heatmap-stage') as HTMLElement;
      return `${s.style.left} ${s.style.width}`;
    };
    expect(box()).toBe('0px 400px');
    resizeWider();
    expect(box()).toBe('200px 400px');
  });
});

describe('HeatmapPanel — 레거시 공간 승격을 config 에 한 번 적는다', () => {
  it('대시보드 경로: 첫 실측 결과를 sensor_space=stage 로 승격한다', () => {
    const onConfigChange = vi.fn();
    render(<HeatmapPanel panelId="p" config={config()} onConfigChange={onConfigChange} />);
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    expect(onConfigChange).toHaveBeenCalledWith({
      sensor_positions: { [sid('s1')]: { x: 0.2, y: 0.8 } },
      sensor_space: 'stage',
    });
    // 리사이즈가 승격을 다시 쓰지 않는다(쓰면 그때그때의 실측이 자리를 덮어쓴다).
    onConfigChange.mockClear();
    resizeWider();
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('이미 승격된 config 에는 아무것도 쓰지 않는다', () => {
    const onConfigChange = vi.fn();
    render(
      <HeatmapPanel
        panelId="p"
        config={config({ sensor_space: 'stage' })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('설정 미리보기(forcePlacement)는 승격을 쓰지 않는다(실측 기준이 대시보드와 다르다)', () => {
    const onConfigChange = vi.fn();
    render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={onConfigChange} forcePlacement />,
    );
    expect(onConfigChange).not.toHaveBeenCalled();
  });
});
