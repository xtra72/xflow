// 도면 종횡비를 못 구했을 때의 어긋남 방지 (보고: "패널 크기를 바꾸면 센서가 이미지와 안 맞는다").
//
// 스테이지는 기준 도면과 같은 종횡비여야 마커의 정규화 좌표가 도면에 고정된다. 종횡비를 못
// 구하면 스테이지가 패널 전체로 퇴화하고, 그러면 도면만 object-fit 으로 안에서 다시 레터박스돼
// **마커는 패널을, 도면은 자기 비율을 따르는** 상태가 된다. config 저장값(natural_*)도, 별도
// new Image() 실측도 실패할 수 있으므로(자산 URL·인증·캐시) 실제로 그려진 <img> 에서 받아온다.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent, act, cleanup } from '@testing-library/react';
import { useState } from 'react';

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
// config 저장값도 별도 실측도 실패한 상태를 만든다 — 이때가 문제의 상황이다.
vi.mock('./useFloorPlanAspect', () => ({ useFloorPlanAspect: () => undefined }));

import HeatmapPanel from './HeatmapPanel';
import { heatmapSensorId } from './sensorIdentity';

const sid = (key: string) => heatmapSensorId({ key });
const reading = (v: number): ChartEntry[] => [{ timestamp: 1, value: v }];

const size = { w: 400, h: 400 };

beforeEach(() => {
  size.w = 400;
  size.h = 400;
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({
        left: 0, top: 0, width: size.w, height: size.h,
        right: size.w, bottom: size.h, x: 0, y: 0, toJSON: () => ({}),
      }) as DOMRect,
  );
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
    sensor_positions: { [sid('s1')]: { x: 0.2, y: 0.8 } },
    idw: { power: 2, grid_resolution: 8 },
    // natural_* 가 없는 레거시/자산 도면.
    floor_plan: { image: 'data:image/png;base64,AAAA' },
    ...extra,
  } as Record<string, unknown>;
}

/** 그려진 <img> 가 원본 크기를 갖고 로드된 것처럼 만든다(jsdom 은 실제 디코드를 하지 않는다). */
function loadBaseImage(img: HTMLImageElement, w: number, h: number) {
  Object.defineProperty(img, 'naturalWidth', { value: w, configurable: true });
  Object.defineProperty(img, 'naturalHeight', { value: h, configurable: true });
  act(() => {
    fireEvent.load(img);
  });
}

describe('HeatmapPanel — 그려진 도면에서 종횡비를 받아 스테이지를 도면에 맞춘다', () => {
  it('종횡비를 모르는 동안 스테이지는 패널 전체이고, 이미지가 뜨면 도면 비율로 좁혀진다', () => {
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={vi.fn()} forcePlacement />,
    );
    const stage = () => {
      const s = getByTestId('heatmap-stage') as HTMLElement;
      return `${s.style.left} ${s.style.top} ${s.style.width} ${s.style.height}`;
    };
    // 종횡비 미상 → 스테이지 = 패널 전체(400x400). 이 상태에서는 도면만 안에서 레터박스된다.
    expect(stage()).toBe('0px 0px 400px 400px');

    // 2:1 도면이 실제로 그려졌다 → 스테이지가 도면 비율(400x200)로 좁혀지고 세로 가운데로 온다.
    loadBaseImage(getByTestId('floor-plan-background') as HTMLImageElement, 800, 400);
    expect(stage()).toBe('0px 100px 400px 200px');
  });

  it('원본 크기를 못 읽으면(0) 종횡비를 바꾸지 않는다', () => {
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={vi.fn()} forcePlacement />,
    );
    loadBaseImage(getByTestId('floor-plan-background') as HTMLImageElement, 0, 0);
    const s = getByTestId('heatmap-stage') as HTMLElement;
    expect(s.style.width).toBe('400px');
    expect(s.style.height).toBe('400px');
  });
});

describe('HeatmapPanel — 종횡비를 모르는 동안에는 레거시 좌표를 승격하지 않는다', () => {
  it('승격은 스테이지가 도면에 고정된 뒤에만 일어난다', () => {
    // 종횡비 미상 상태의 환산은 항등이라, 그때 승격하면 컨테이너 좌표가 스테이지 좌표라는
    // 표식을 달고 굳는다 — 나중에 도면 비율이 도착하면 마커가 어긋난 채 고정된다.
    const onConfigChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={onConfigChange} />,
    );
    expect(onConfigChange).not.toHaveBeenCalled();

    loadBaseImage(getByTestId('floor-plan-background') as HTMLImageElement, 800, 400);
    // 400x400 본문 안의 400x200 스테이지 → (0.2, 0.8) 은 스테이지 기준 (0.2, 1.0) 이다.
    expect(onConfigChange).toHaveBeenCalledWith({
      sensor_positions: { [sid('s1')]: { x: 0.2, y: 1 } },
      sensor_space: 'stage',
    });
  });

  it('읽어 낸 도면 원본 크기를 config 에 한 번 남긴다(다음 세션은 첫 페인트부터 맞는다)', () => {
    // 런타임 실측은 매번 이미지 로드에 기댄다 — 캐시·자산 조회·디코드 중 하나만 어긋나도
    // 종횡비가 미상이 되어 스테이지가 패널 전체로 퇴화한다. 한 번 읽었으면 남겨 둔다.
    const onConfigChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapPanel panelId="p" config={config()} onConfigChange={onConfigChange} />,
    );
    loadBaseImage(getByTestId('floor-plan-background') as HTMLImageElement, 800, 400);
    const sizeWrite = onConfigChange.mock.calls
      .map(([c]) => c as Record<string, unknown>)
      .find((c) => 'floor_plans' in c);
    expect(sizeWrite).toBeDefined();
    expect(sizeWrite!.floor_plans).toEqual([
      expect.objectContaining({ natural_width: 800, natural_height: 400 }),
    ]);
  });

  it('이미 저장된 원본 크기가 있으면 다시 쓰지 않는다', () => {
    const onConfigChange = vi.fn();
    const { getByTestId } = render(
      <HeatmapPanel
        panelId="p"
        config={config({
          floor_plan: {
            image: 'data:image/png;base64,AAAA',
            natural_width: 800,
            natural_height: 400,
          },
        })}
        onConfigChange={onConfigChange}
      />,
    );
    loadBaseImage(getByTestId('floor-plan-background') as HTMLImageElement, 800, 400);
    expect(
      onConfigChange.mock.calls.map(([c]) => c as Record<string, unknown>).some((c) => 'floor_plans' in c),
    ).toBe(false);
  });

  it('도면이 없으면 기다리지 않는다(스테이지 = 컨테이너가 정상이다)', () => {
    const onConfigChange = vi.fn();
    const cfg = config();
    delete cfg.floor_plan;
    render(<HeatmapPanel panelId="p" config={cfg} onConfigChange={onConfigChange} />);
    expect(onConfigChange).toHaveBeenCalledWith({
      sensor_positions: { [sid('s1')]: { x: 0.2, y: 0.8 } },
      sensor_space: 'stage',
    });
  });
});

describe('HeatmapPanel — 캐시된 도면(load 이벤트 없음)도 대시보드·미리보기 모두에서 맞는다', () => {
  /** 마운트 시점에 이미 로드가 끝난 이미지(대시보드를 다시 여는 흔한 경로). */
  function withCachedImage(w: number, h: number, run: () => void) {
    const proto = HTMLImageElement.prototype as unknown as Record<string, unknown>;
    Object.defineProperty(proto, 'complete', { value: true, configurable: true });
    Object.defineProperty(proto, 'naturalWidth', { value: w, configurable: true });
    Object.defineProperty(proto, 'naturalHeight', { value: h, configurable: true });
    try {
      run();
    } finally {
      for (const k of ['complete', 'naturalWidth', 'naturalHeight']) delete proto[k];
    }
  }

  /** config 를 shallow merge 로 보관하는 부모(대시보드 `updatePanelConfig` 와 같은 규칙). */
  function Host({ force }: { force: boolean }) {
    const [cfg, setCfg] = useState(config());
    return (
      <HeatmapPanel
        panelId="p"
        config={cfg}
        forcePlacement={force}
        onConfigChange={(c) => setCfg((prev) => ({ ...prev, ...c }))}
      />
    );
  }

  /** 400x400 본문 + 2:1 도면 → 스테이지는 400x200, 세로 가운데. */
  const EXPECTED = '0px 100px 400px 200px';

  it('대시보드 경로(config 쓰기가 실제로 반영되는 경로)', () => {
    withCachedImage(800, 400, () => {
      const { getByTestId } = render(<Host force={false} />);
      const s = getByTestId('heatmap-stage') as HTMLElement;
      expect(`${s.style.left} ${s.style.top} ${s.style.width} ${s.style.height}`).toBe(EXPECTED);
    });
  });

  it('설정 미리보기 경로(config 를 쓰지 않는 경로)', () => {
    withCachedImage(800, 400, () => {
      const { getByTestId } = render(<Host force />);
      const s = getByTestId('heatmap-stage') as HTMLElement;
      expect(`${s.style.left} ${s.style.top} ${s.style.width} ${s.style.height}`).toBe(EXPECTED);
    });
  });
});
