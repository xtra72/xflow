// AddPanelDialog 테스트.
// 1) 기존 동작 보존 characterization 테스트 (non-chart 패널)
// 2) SPEC-CHART-001 M5 신규 기능 (차트 패널 channel_name 선택/검증)

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

// listChartChannels 를 hoisted vi.fn 으로 mock
const listChartChannelsMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/charts', () => ({
  listChartChannels: listChartChannelsMock,
}));

// useDevices 훅 (디바이스 선택 스텝 캐릭터라이제이션용)
vi.mock('@/hooks/useDevice', () => ({
  useDevices: () => ({ data: { data: [] }, isLoading: false }),
}));

// SPEC-FACILITY-DASHBOARD-001 M5: 설비 스텝이 사용하는 에이전트/로스터 훅 mock.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'air-1', name: '공기청정 에이전트', type: 'airpurifier', status: 'running' },
        { id: 'store-1', name: '스토어', type: 'store', status: 'running' },
      ],
    },
  }),
}));

vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({
    data: [
      { station: 's1', line: 'L1', display_name: '1역', order: 0, places: [] },
      { station: 's2', line: 'L2', display_name: '2역', order: 1, places: [] },
    ],
    isLoading: false,
  }),
  useAirpurifierDevices: () => ({
    data: [{ device_id: 'd1', name: '기기-1', station: 's1', place: 'p1', index: 0 }],
    isLoading: false,
  }),
}));

// uiStore 상태를 캡처하기 위한 mock
const storeState = vi.hoisted(() => ({
  addPanelCalls: [] as Array<{ type: string }>,
  addPanelWithConfigCalls: [] as Array<{
    type: string;
    config: Record<string, unknown>;
    title?: string;
  }>,
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({
      addPanel: (type: string) => {
        storeState.addPanelCalls.push({ type });
      },
      addPanelWithConfig: (
        type: string,
        config: Record<string, unknown>,
        title?: string,
      ) => {
        storeState.addPanelWithConfigCalls.push({ type, config, title });
      },
    }),
}));

// i18n 스텁 — 키를 그대로 반환하되, 보간 슬롯을 가진 키는 템플릿을 반환하여
// .replace('{slot}', value) 흐름이 실제 값을 만들어내도록 한다.
vi.mock('@/lib/i18n', () => {
  const templates: Record<string, string> = {
    'dashboard.addPanel.channelOption': '{name} — flow {flow} ({count} subs)',
    'dashboard.chart.channelNameError': '유효한 채널 이름이 아닙니다',
  };
  return {
    useTranslation: () => ({ t: (k: string) => templates[k] ?? k }),
  };
});

import AddPanelDialog from './AddPanelDialog';

describe('AddPanelDialog', () => {
  beforeEach(() => {
    storeState.addPanelCalls = [];
    storeState.addPanelWithConfigCalls = [];
    listChartChannelsMock.mockReset();
    listChartChannelsMock.mockResolvedValue([]);
  });

  // ---- Characterization Tests (기존 동작 보존) ----

  describe('기존 non-chart 동작 (characterization)', () => {
    it('열려있으면 "패널 추가" 타이틀 표시', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      expect(screen.getByText('dashboard.addPanel.title')).toBeInTheDocument();
    });

    it('닫혀있으면 아무것도 렌더링하지 않음', () => {
      render(<AddPanelDialog open={false} onClose={() => {}} />);
      expect(screen.queryByText('dashboard.addPanel.title')).toBeNull();
    });

    it('카테고리 탭이 표시됨 (데이터, 차트, 콘텐츠, 제어)', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      expect(screen.getByRole('button', { name: 'dashboard.panelCategories.data' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'dashboard.panelCategories.content' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'dashboard.panelCategories.control' })).toBeInTheDocument();
    });

    it('차트 외 non-device 패널 (텍스트) 선택 시 addPanel 즉시 호출 + 닫힘', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);

      // 콘텐츠 탭으로 이동
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
      // "텍스트" 패널 선택
      fireEvent.click(screen.getByText('dashboard.panelTypes.text'));

      expect(storeState.addPanelCalls).toEqual([{ type: 'text' }]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  // ---- Chart panel new behavior (REQ-M5-01/02/04) ----

  describe('차트 패널 channel_name 단계 (REQ-M5-01/02/04)', () => {
    it('차트 타입 선택 시 채널 선택 step 으로 진입 (REQ-M5-01)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.lineChart'));

      await waitFor(() => {
        expect(screen.getByText(/dashboard\.addPanel\.channelSuffix$/)).toBeInTheDocument();
      });
      expect(screen.getByTestId('chart-channel-select')).toBeInTheDocument();
      // 즉시 addPanel 호출되지 않음
      expect(storeState.addPanelCalls).toEqual([]);
    });

    it('드롭다운이 listChartChannels 결과로 채워진다 (REQ-M5-02)', async () => {
      listChartChannelsMock.mockResolvedValue([
        {
          name: 'demo_temp',
          flow_id: 'flow-1',
          node_id: 'node-a',
          buffer_size: 100,
          retention_sec: 3600,
          subscriber_count: 3,
          last_message_ms: 0,
        },
        {
          name: 'another_ch',
          flow_id: 'flow-2',
          node_id: 'node-b',
          buffer_size: 50,
          retention_sec: 600,
          subscriber_count: 0,
          last_message_ms: 0,
        },
      ]);

      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.stat'));

      await waitFor(() => {
        expect(listChartChannelsMock).toHaveBeenCalledTimes(1);
      });
      const select = await screen.findByTestId('chart-channel-select');
      await waitFor(() => {
        expect((select as HTMLSelectElement).options.length).toBeGreaterThan(2);
      });
      expect(screen.getByText(/demo_temp — flow flow-1/)).toBeInTheDocument();
      expect(screen.getByText(/another_ch — flow flow-2/)).toBeInTheDocument();
    });

    it('"Custom..." 선택 시 수동 입력 필드 표시 (REQ-M5-02)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.barChart'));

      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      expect(screen.getByTestId('chart-channel-custom-input')).toBeInTheDocument();
    });

    it('유효하지 않은 channel_name 은 에러 메시지 + 저장 비활성 (REQ-M5-04)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.pieChart'));

      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      const input = screen.getByTestId('chart-channel-custom-input');
      fireEvent.change(input, { target: { value: 'abc/def' } });

      expect(screen.getByTestId('chart-channel-error')).toHaveTextContent(
        '유효한 채널 이름이 아닙니다',
      );
      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(true);
    });

    it('유효한 channel_name 입력 후 저장 시 addPanelWithConfig 호출 + onClose', async () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);
      // table 은 카탈로그 상 '데이터' 카테고리에 있음 (UI 분류).
      // 하지만 isChartPanelType(table) === true 이므로 chart-config 스텝으로 라우팅된다.
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.data' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      const input = screen.getByTestId('chart-channel-custom-input');
      fireEvent.change(input, { target: { value: 'room1_temp' } });

      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);

      expect(storeState.addPanelWithConfigCalls).toEqual([
        { type: 'table', config: { channel_name: 'room1_temp' }, title: undefined },
      ]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('빈 입력은 에러 메시지 숨김 + 저장 비활성 (초기 상태)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.stat'));

      const select = await screen.findByTestId('chart-channel-select');
      // 초기 빈 상태
      expect(select).toHaveValue('');
      expect(screen.queryByTestId('chart-channel-error')).toBeNull();
      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(true);
    });

    it('드롭다운에서 기존 채널 선택 → 저장 (REQ-M5-02)', async () => {
      listChartChannelsMock.mockResolvedValue([
        {
          name: 'demo_temp',
          flow_id: 'flow-1',
          node_id: 'n',
          buffer_size: 100,
          retention_sec: 3600,
          subscriber_count: 1,
          last_message_ms: 0,
        },
      ]);

      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.lineChart'));

      const select = await screen.findByTestId('chart-channel-select');
      await waitFor(() => {
        expect((select as HTMLSelectElement).options.length).toBeGreaterThan(2);
      });
      fireEvent.change(select, { target: { value: 'demo_temp' } });

      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);

      expect(storeState.addPanelWithConfigCalls).toEqual([
        { type: 'line-chart', config: { channel_name: 'demo_temp' }, title: undefined },
      ]);
    });

    it('API 실패 시 경고 + Custom 으로는 계속 진행 가능', async () => {
      listChartChannelsMock.mockRejectedValueOnce(new Error('network down'));

      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.stat'));

      await waitFor(() => {
        expect(screen.getByText(/dashboard\.addPanel\.loadErrorCustom/)).toBeInTheDocument();
      });
      // Custom 경로는 여전히 사용 가능
      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      const input = screen.getByTestId('chart-channel-custom-input');
      fireEvent.change(input, { target: { value: 'manual_ch' } });

      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
    });
  });

  // ---- 다채널 비교 프리셋 ----

  describe('multi-channel line-chart preset', () => {
    it('차트 카테고리에 "다채널 비교" 옵션 노출', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      expect(screen.getByText('dashboard.addPanel.labels.multiChannel')).toBeInTheDocument();
    });

    it('"다채널 비교" 선택 시 channels=[빈x2] 로 즉시 추가 + 닫힘 (channel-config 스킵)', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.addPanel.labels.multiChannel'));

      expect(storeState.addPanelWithConfigCalls).toHaveLength(1);
      const call = storeState.addPanelWithConfigCalls[0]!;
      expect(call.type).toBe('line-chart');
      expect(call.config).toEqual({
        channels: [{ name: '' }, { name: '' }],
      });
      // 다이얼로그 닫힘
      expect(onClose).toHaveBeenCalled();
    });

    it('"다채널 비교" 추가 시 chart-config 스텝(채널 선택 화면) 미진입', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.addPanel.labels.multiChannel'));
      // chart-config 스텝의 채널 선택 select 가 나타나지 않아야 함
      expect(screen.queryByTestId('chart-channel-select')).toBeNull();
    });

    it('"다채널 비교" 검색으로 찾을 수 있음', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      const search = screen.getByPlaceholderText('dashboard.addPanel.searchPlaceholder');
      // 검색은 번역된 라벨/설명(여기선 키 문자열) 기준으로 필터링되므로 키 substring 으로 검색.
      fireEvent.change(search, { target: { value: 'multiChannel' } });
      expect(screen.getByText('dashboard.addPanel.labels.multiChannel')).toBeInTheDocument();
    });
  });

  // ---- 설비 패널 (SPEC-FACILITY-DASHBOARD-001 M5) ----

  describe('설비 패널 3종 (facility)', () => {
    it('제어 카테고리에 설비 3종 옵션 노출', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
      expect(screen.getByText('dashboard.panelTypes.facilityLine')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.facilityStation')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.facilityDevice')).toBeInTheDocument();
    });

    it('설비 옵션 선택 시 즉시 addPanel 되지 않고 설비 스텝으로 진입', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityStation'));

      // 에이전트/대상 셀렉트 등장, 즉시 추가는 없음
      expect(screen.getByTestId('facility-agent-select')).toBeInTheDocument();
      expect(screen.getByTestId('facility-target-select')).toBeInTheDocument();
      expect(storeState.addPanelCalls).toEqual([]);
      expect(storeState.addPanelWithConfigCalls).toEqual([]);
    });

    it('facility-station: 에이전트→역사 선택 후 저장 시 addPanelWithConfig({agentId, station})', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityStation'));

      // 에이전트 셀렉트는 airpurifier 만 노출 (store 에이전트 제외)
      const agentSelect = screen.getByTestId('facility-agent-select') as HTMLSelectElement;
      expect(agentSelect.options.length).toBe(2); // placeholder + airpurifier 1개
      fireEvent.change(agentSelect, { target: { value: 'air-1' } });

      const targetSelect = screen.getByTestId('facility-target-select') as HTMLSelectElement;
      fireEvent.change(targetSelect, { target: { value: 's1' } });

      const save = screen.getByTestId('facility-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);

      expect(storeState.addPanelWithConfigCalls).toEqual([
        { type: 'facility-station', config: { agentId: 'air-1', station: 's1' }, title: '1역' },
      ]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('facility-line: 대상 셀렉트는 distinct line 값을 노출', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityLine'));

      fireEvent.change(screen.getByTestId('facility-agent-select'), { target: { value: 'air-1' } });
      const targetSelect = screen.getByTestId('facility-target-select') as HTMLSelectElement;
      // placeholder + L1 + L2
      expect(targetSelect.options.length).toBe(3);
      expect(screen.getByRole('option', { name: 'L1' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'L2' })).toBeInTheDocument();
    });

    it('에이전트 미선택 시 저장 비활성', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityDevice'));

      const save = screen.getByTestId('facility-save') as HTMLButtonElement;
      expect(save.disabled).toBe(true);
    });
  });
});
