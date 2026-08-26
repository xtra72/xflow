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
        { id: 'air-1', name: '공기청정 에이전트', type: 'xsfm', status: 'running' },
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
  useXsfmDevices: () => ({
    data: [{ device_id: 'd1', name: '기기-1', station: 's1', place: 'p1', index: 0 }],
    isLoading: false,
  }),
}));

// SPEC-XSFM-GROUP-001 M7: 설비 그룹 스텝이 사용하는 그룹 훅 mock(역사·라인·커스텀 그룹 나열).
vi.mock('@/hooks/useGroups', () => ({
  useGroups: () => ({
    data: [
      { id: 'station:s1', name: '1역', type: 'station', member_count: 3, members: ['d1', 'd2', 'd3'] },
      { id: 'custom:g1', name: '커스텀그룹', type: 'custom', member_count: 2, members: ['d1', 'd2'] },
    ],
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

    it('카테고리 탭 6종이 표시됨 (상태·차트·데이터·시스템·콘텐트·기타)', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      for (const cat of ['status', 'chart', 'data', 'system', 'content', 'etc']) {
        expect(
          screen.getByRole('button', { name: `dashboard.panelCategories.${cat}` }),
        ).toBeInTheDocument();
      }
      // '제어' 카테고리는 사라지고 제어성 패널은 콘텐트 하위 그룹으로 옮겼다.
      expect(
        screen.queryByRole('button', { name: 'dashboard.panelCategories.control' }),
      ).toBeNull();
    });

    it('차트 외 non-device 패널 (텍스트) 선택 시 addPanel 즉시 호출 + 닫힘', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);

      // 텍스트는 기타 그룹으로 옮겼다.
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      // "텍스트" 패널 선택
      fireEvent.click(screen.getByText('dashboard.panelTypes.text'));

      expect(storeState.addPanelCalls).toEqual([{ type: 'text' }]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  // ---- Chart panel new behavior (REQ-M5-01/02/04) ----

  // 채널 이름 스텝은 이제 테이블에만 남는다. 통계/게이지/바/파이는 store 기본 소스로 즉시
  // 추가되고(아래 'store 기본 데이터 소스'), 라인 차트도 store 프리셋으로 즉시 추가된다.
  describe('차트 패널 channel_name 단계 (REQ-M5-01/02/04)', () => {
    it('차트 타입 선택 시 채널 선택 step 으로 진입 (REQ-M5-01)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

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
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

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
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      expect(screen.getByTestId('chart-channel-custom-input')).toBeInTheDocument();
    });

    it('유효하지 않은 channel_name 은 에러 메시지 + 저장 비활성 (REQ-M5-04)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

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
      // table 은 카탈로그 상 '기타' 카테고리에 있음 (UI 분류).
      // 하지만 isChartPanelType(table) === true 이므로 chart-config 스텝으로 라우팅된다.
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
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
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

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
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

      const select = await screen.findByTestId('chart-channel-select');
      await waitFor(() => {
        expect((select as HTMLSelectElement).options.length).toBeGreaterThan(2);
      });
      fireEvent.change(select, { target: { value: 'demo_temp' } });

      const save = screen.getByTestId('chart-channel-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);

      expect(storeState.addPanelWithConfigCalls).toEqual([
        { type: 'table', config: { channel_name: 'demo_temp' }, title: undefined },
      ]);
    });

    it('API 실패 시 경고 + Custom 으로는 계속 진행 가능', async () => {
      listChartChannelsMock.mockRejectedValueOnce(new Error('network down'));

      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.table'));

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

  // ---- store 기본 데이터 소스 (통계/게이지/바/파이) ----

  describe('store 기본 데이터 소스 — 통계/게이지/바/파이', () => {
    // 이 4종은 히트맵과 같이 채널 이름을 묻지 않고 즉시 추가된다. 데이터 소스 자체는
    // `uiStore.createDefaultPanel` 의 기본 config(`data_source: 'store'`)가 정하므로,
    // 여기서는 **채널 스텝을 거치지 않는다**는 라우팅만 확인한다.
    const CASES: Array<{ label: string; type: string; category: string }> = [
      { label: 'dashboard.panelTypes.stat', type: 'stat', category: 'chart' },
      { label: 'dashboard.panelTypes.gauge', type: 'gauge', category: 'chart' },
      { label: 'dashboard.panelTypes.barChart', type: 'bar-chart', category: 'chart' },
      { label: 'dashboard.panelTypes.pieChart', type: 'pie-chart', category: 'chart' },
    ];

    for (const { label, type, category } of CASES) {
      it(`${type}: 채널 스텝 없이 즉시 추가된다`, async () => {
        const onClose = vi.fn();
        render(<AddPanelDialog open={true} onClose={onClose} />);
        fireEvent.click(
          screen.getByRole('button', { name: `dashboard.panelCategories.${category}` }),
        );
        fireEvent.click(screen.getByText(label));

        expect(storeState.addPanelCalls).toEqual([{ type }]);
        expect(onClose).toHaveBeenCalledTimes(1);
        // 채널 선택 UI 가 뜨지 않는다 — 스텝 자체를 건너뛴다.
        expect(screen.queryByTestId('chart-channel-select')).toBeNull();
      });
    }

    it('채널 목록 조회를 발생시키지 않는다', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.stat'));

      // 스텝을 건너뛰므로 listChartChannels 가 호출될 이유가 없다.
      expect(listChartChannelsMock).not.toHaveBeenCalled();
    });
  });

  // ---- 카테고리 재구성 ----

  describe('카테고리 배치', () => {
    function open(cat: string) {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: `dashboard.panelCategories.${cat}` }));
    }

    const PLACEMENT: Array<[string, string[]]> = [
      ['status', [
        'dashboard.panelTypes.flows',
        'dashboard.panelTypes.agents',
        'dashboard.panelTypes.agentStatus',
        'dashboard.panelTypes.resource',
      ]],
      ['chart', [
        'dashboard.panelTypes.stat',
        'dashboard.panelTypes.gauge',
        'dashboard.panelTypes.lineChart',
        'dashboard.panelTypes.barChart',
        'dashboard.panelTypes.pieChart',
        'dashboard.addPanel.labels.heatmap',
      ]],
      ['system', ['dashboard.panelTypes.logs']],
      ['content', [
        'dashboard.panelTypes.acControl',
        'dashboard.panelTypes.hvacControl',
        'dashboard.addPanel.labels.outdoorControl',
        'dashboard.panelTypes.facilityLine',
        'dashboard.panelTypes.facilityGroup',
        'dashboard.panelTypes.facilityDevice',
        'dashboard.panelTypes.facilitySchedule',
        'dashboard.panelTypes.modbusVirtualDevices',
        'dashboard.panelTypes.modbusSharedRegisters',
        'dashboard.panelTypes.modbusDeviceRegisters',
        'dashboard.panelTypes.modbusBusStats',
        'dashboard.panelTypes.modbusSummaryStats',
      ]],
      ['etc', [
        'dashboard.panelTypes.text',
        'dashboard.panelTypes.devices',
        'dashboard.panelTypes.device',
        'dashboard.addPanel.labels.propertiesGrid',
        'dashboard.panelTypes.triggerConfig',
        'dashboard.panelTypes.table',
        'dashboard.panelTypes.modbusRealDevices',
        'dashboard.panelTypes.customControl',
      ]],
    ];

    for (const [cat, labels] of PLACEMENT) {
      it(`${cat}: 지정된 항목만 노출한다`, () => {
        open(cat);
        for (const label of labels) {
          expect(screen.getByText(label)).toBeInTheDocument();
        }
        // 이 카테고리에 없어야 하는 항목이 섞이지 않았는지 총 개수로 확인한다.
        expect(screen.getAllByRole('button').length - 6 /* 카테고리 탭 */ - 2 /* 뒤로/닫기 */)
          .toBe(labels.length);
      });
    }

    it('데이터 카테고리는 비어 있고 안내 문구를 보여준다', () => {
      // 요청 사양에 배정된 항목이 없다. 빈 화면 대신 사유를 표시한다.
      open('data');
      expect(screen.getByTestId('add-panel-empty')).toHaveTextContent(
        'dashboard.addPanel.emptyCategory',
      );
    });

    it('콘텐트는 하위 그룹 제목 3종으로 나뉜다', () => {
      open('content');
      const labels = screen
        .getAllByTestId('add-panel-group-label')
        .map((el) => el.textContent);
      expect(labels).toEqual([
        'dashboard.addPanel.groups.hvacr',
        'dashboard.addPanel.groups.facility',
        'dashboard.addPanel.groups.modbus',
      ]);
    });

    it('하위 그룹이 없는 카테고리는 그룹 제목을 렌더하지 않는다', () => {
      open('status');
      expect(screen.queryByTestId('add-panel-group-label')).toBeNull();
    });

    it('검색 중에는 카테고리·하위 그룹을 무시하고 한 묶음으로 보여준다', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      const search = screen.getByPlaceholderText('dashboard.addPanel.searchPlaceholder');
      fireEvent.change(search, { target: { value: 'modbus' } });
      // 콘텐트(MODBUS 5종) + 기타(실제 디바이스)가 함께 나온다.
      expect(screen.getByText('dashboard.panelTypes.modbusBusStats')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.modbusVirtualDevices')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.modbusRealDevices')).toBeInTheDocument();
      expect(screen.getAllByTestId('add-panel-group')).toHaveLength(1);
      expect(screen.queryByTestId('add-panel-group-label')).toBeNull();
    });
  });

  // ---- 라인 차트 (store 프리셋 단일 항목) ----

  describe('라인 차트 생성 항목', () => {
    function openChartCategory() {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
    }

    it('차트 카테고리의 라인 차트 항목은 하나뿐이다', () => {
      openChartCategory();
      expect(screen.getAllByText('dashboard.panelTypes.lineChart')).toHaveLength(1);
    });

    it('채널 기반 항목(다채널 비교 / Store 라인 차트)은 목록에서 사라졌다', () => {
      openChartCategory();
      expect(screen.queryByText('dashboard.addPanel.labels.multiChannel')).toBeNull();
      expect(screen.queryByText('dashboard.addPanel.labels.storeLineChart')).toBeNull();
    });

    it('선택 시 store 소스로 즉시 추가되고 채널 스텝에 들르지 않는다', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.chart' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.lineChart'));

      expect(storeState.addPanelWithConfigCalls).toHaveLength(1);
      const call = storeState.addPanelWithConfigCalls[0]!;
      expect(call.type).toBe('line-chart');
      expect(call.config).toMatchObject({
        data_source: 'store',
        store_source: { agent_name: '', namespace: 'default', series: [] },
      });
      // 채널 이름을 묻지 않으므로 목록 조회도 일어나지 않는다.
      expect(screen.queryByTestId('chart-channel-select')).toBeNull();
      expect(listChartChannelsMock).not.toHaveBeenCalled();
      expect(onClose).toHaveBeenCalled();
    });

    it('검색으로 찾을 수 있다', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      const search = screen.getByPlaceholderText('dashboard.addPanel.searchPlaceholder');
      fireEvent.change(search, { target: { value: 'lineChart' } });
      expect(screen.getAllByText('dashboard.panelTypes.lineChart')).toHaveLength(1);
    });
  });

  // ---- 설비 패널 (SPEC-FACILITY-DASHBOARD-001 M5) ----

  describe('설비 패널 (facility)', () => {
    it('콘텐트 카테고리에 설비 3종(라인·그룹·기기) 노출, 역사(station)는 생성 옵션에서 제외', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
      expect(screen.getByText('dashboard.panelTypes.facilityLine')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.facilityGroup')).toBeInTheDocument();
      expect(screen.getByText('dashboard.panelTypes.facilityDevice')).toBeInTheDocument();
      // 역사 패널은 그룹으로 흡수되어 더 이상 별도 생성 타입이 아니다.
      expect(screen.queryByText('dashboard.panelTypes.facilityStation')).toBeNull();
    });

    it('설비 옵션 선택 시 즉시 addPanel 되지 않고 설비 스텝으로 진입', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityGroup'));

      // 에이전트/대상 셀렉트 등장, 즉시 추가는 없음
      expect(screen.getByTestId('facility-agent-select')).toBeInTheDocument();
      expect(screen.getByTestId('facility-target-select')).toBeInTheDocument();
      expect(storeState.addPanelCalls).toEqual([]);
      expect(storeState.addPanelWithConfigCalls).toEqual([]);
    });

    it('facility-group: 에이전트→그룹 선택 후 저장 시 addPanelWithConfig({agentId, groupId})', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityGroup'));

      // 에이전트 셀렉트는 xsfm 만 노출 (store 에이전트 제외)
      const agentSelect = screen.getByTestId('facility-agent-select') as HTMLSelectElement;
      expect(agentSelect.options.length).toBe(2); // placeholder + xsfm 1개
      fireEvent.change(agentSelect, { target: { value: 'air-1' } });

      // 그룹 셀렉트는 역사·라인·커스텀 그룹을 나열한다(placeholder + station + custom).
      const targetSelect = screen.getByTestId('facility-target-select') as HTMLSelectElement;
      expect(targetSelect.options.length).toBe(3);
      fireEvent.change(targetSelect, { target: { value: 'custom:g1' } });

      const save = screen.getByTestId('facility-save') as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);

      // 저장: config.groupId 로 저장하고 기본 타이틀은 그룹명(멤버 수/type 라벨 제외).
      expect(storeState.addPanelWithConfigCalls).toEqual([
        { type: 'facility-group', config: { agentId: 'air-1', groupId: 'custom:g1' }, title: '커스텀그룹' },
      ]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('facility-line: 대상 셀렉트는 distinct line 값을 노출', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
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
      fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.content' }));
      fireEvent.click(screen.getByText('dashboard.panelTypes.facilityDevice'));

      const save = screen.getByTestId('facility-save') as HTMLButtonElement;
      expect(save.disabled).toBe(true);
    });
  });
});
