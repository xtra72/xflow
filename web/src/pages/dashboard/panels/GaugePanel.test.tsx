// GaugePanel 테스트.
// chart-emitter 데이터 소스 바인딩의 live value 반영 + fallback 동작을 검증한다.
// 실제 SVG 경로 수식까지는 검증하지 않고 value 가 DOM 에 반영되는지만 확인한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from './charts/chartChannelTypes';

const mockChannel = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
  // 훅 호출 시 전달된 채널명 캡처 (바인딩 없을 때 호출되지 않음을 검증)
  lastCalledWith: undefined as string | undefined,
}));

vi.mock('./charts/useChartChannel', () => ({
  useChartChannel: (channelName: string | undefined) => {
    mockChannel.lastCalledWith = channelName;
    return mockChannel.current;
  },
}));

import GaugePanel from './GaugePanel';

function renderPanel(config: Record<string, unknown>) {
  return render(
    <GaugePanel panelId="p1" title="테스트 게이지" config={config} />,
  );
}

describe('GaugePanel', () => {
  beforeEach(() => {
    mockChannel.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    mockChannel.lastCalledWith = undefined;
  });

  describe('chart-emitter 바인딩 없음', () => {
    it('config.value 가 그대로 표시됨 (simple 게이지)', () => {
      renderPanel({ gaugeType: 'simple', value: 42, min: 0, max: 100, unit: '%' });
      expect(screen.getByText('42')).toBeInTheDocument();
      expect(screen.getByText('%')).toBeInTheDocument();
    });

    it('useChartChannel 이 undefined 채널명으로 호출됨 (idle)', () => {
      renderPanel({ gaugeType: 'simple', value: 10 });
      expect(mockChannel.lastCalledWith).toBeUndefined();
    });

    it('연결 상태 아이콘 미표시', () => {
      renderPanel({ gaugeType: 'simple', value: 10 });
      expect(screen.queryByTestId('chart-status-icon')).toBeNull();
    });
  });

  describe('chart-emitter 바인딩 있음', () => {
    const baseConfig = {
      gaugeType: 'needle',
      min: 0,
      max: 100,
      unit: '°C',
      value: 0, // static fallback
      dataSources: [
        { sourceType: 'chart-emitter', channelName: 'room1_temperature' },
      ],
    };

    it('최신 entry 의 value 가 static config.value 를 덮어씀', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 20 },
        { timestamp: 2000, value: 27 },
      ];
      renderPanel(baseConfig);
      expect(screen.getByText('27')).toBeInTheDocument();
      expect(screen.getByText('°C')).toBeInTheDocument();
    });

    it('구독 채널명이 훅에 전달됨', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 1 }];
      renderPanel(baseConfig);
      expect(mockChannel.lastCalledWith).toBe('room1_temperature');
    });

    it('displayField dot-path 지원 (labels.temp)', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 999, labels: { temp: '42' } },
      ];
      renderPanel({
        ...baseConfig,
        dataSources: [
          {
            sourceType: 'chart-emitter',
            channelName: 'room1_temperature',
            displayField: 'labels.temp',
          },
        ],
      });
      expect(screen.getByText('42')).toBeInTheDocument();
    });

    it('entries 비어있으면 값 -- 로 표시 (바늘 없음)', () => {
      mockChannel.current.entries = [];
      renderPanel({ ...baseConfig, value: 55 });
      expect(screen.getByText('--')).toBeInTheDocument();
    });

    it('displayField 가 숫자로 변환 불가면 -- 로 표시', () => {
      mockChannel.current.entries = [
        { timestamp: 1000, value: 'not-a-number' },
      ];
      renderPanel({ ...baseConfig, value: 77 });
      expect(screen.getByText('--')).toBeInTheDocument();
    });

    it('연결 상태 아이콘 표시', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 1 }];
      renderPanel(baseConfig);
      expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
    });
  });

  describe('복수 dataSources', () => {
    it('첫 번째 chart-emitter 바인딩만 구독', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 99 }];
      renderPanel({
        gaugeType: 'simple',
        min: 0,
        max: 200,
        unit: '%',
        value: 10,
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' }, // 무시됨
          { sourceType: 'chart-emitter', channelName: 'ch-a' }, // 선택됨
          { sourceType: 'chart-emitter', channelName: 'ch-b' }, // 스킵
        ],
      });
      expect(mockChannel.lastCalledWith).toBe('ch-a');
    });

    it('chart-emitter 바인딩에 channelName 이 없으면 스킵하고 다음 탐색', () => {
      mockChannel.current.entries = [{ timestamp: 1, value: 5 }];
      renderPanel({
        gaugeType: 'simple',
        min: 0,
        max: 100,
        unit: '%',
        value: 0,
        dataSources: [
          { sourceType: 'chart-emitter' }, // channelName 없음 → 스킵
          { sourceType: 'chart-emitter', channelName: 'ch-valid' },
        ],
      });
      expect(mockChannel.lastCalledWith).toBe('ch-valid');
    });

    it('chart-emitter 바인딩 없으면 구독 안 함', () => {
      renderPanel({
        gaugeType: 'simple',
        value: 10,
        dataSources: [
          { sourceType: 'resource', resource: 'cpu' },
          { sourceType: 'flow', flowId: 'f-1' },
        ],
      });
      expect(mockChannel.lastCalledWith).toBeUndefined();
    });
  });

  describe('showThresholdZones (옵션 — 파이 sector 영역)', () => {
    it('미설정 + thresholds 존재: 기본 ON 으로 sector 렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
        ],
      });
      // 기본값 ON 이므로 sector path 가 존재
      expect(container.querySelector('path[fill="#10b981"]')).not.toBeNull();
    });

    it('showThresholdZones=false (사용자 명시 해제): sector 미렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: false,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
        ],
      });
      const greenFill = container.querySelector('path[fill="#10b981"]');
      expect(greenFill).toBeNull();
    });

    it('showThresholdZones=true: 임계값 개수만큼 sector path 렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: true,
        thresholds: [
          { name: '정상', from: 0, to: 60, color: '#10b981' },
          { name: '주의', from: 60, to: 80, color: '#f59e0b' },
          { name: '위험', from: 80, to: 100, color: '#ef4444' },
        ],
      });
      expect(container.querySelector('path[fill="#10b981"]')).not.toBeNull();
      expect(container.querySelector('path[fill="#f59e0b"]')).not.toBeNull();
      expect(container.querySelector('path[fill="#ef4444"]')).not.toBeNull();
    });

    it('showThresholdZones=true 이지만 thresholds 가 비어있으면 sector 미렌더', () => {
      const { container } = renderPanel({
        gaugeType: 'needle',
        value: 50,
        min: 0,
        max: 100,
        showThresholdZones: true,
        thresholds: [],
      });
      // 임계값 컬러 fill 이 없어야 함
      const sectors = container.querySelectorAll('path[opacity="0.25"]');
      expect(sectors.length).toBe(0);
    });
  });
});
