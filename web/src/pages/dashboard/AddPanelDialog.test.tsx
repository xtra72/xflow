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
      expect(screen.getByText('패널 추가')).toBeInTheDocument();
    });

    it('닫혀있으면 아무것도 렌더링하지 않음', () => {
      render(<AddPanelDialog open={false} onClose={() => {}} />);
      expect(screen.queryByText('패널 추가')).toBeNull();
    });

    it('카테고리 탭이 표시됨 (데이터, 차트, 콘텐츠, 제어)', () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      expect(screen.getByRole('button', { name: '데이터' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: '차트' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: '콘텐츠' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: '제어' })).toBeInTheDocument();
    });

    it('차트 외 non-device 패널 (텍스트) 선택 시 addPanel 즉시 호출 + 닫힘', () => {
      const onClose = vi.fn();
      render(<AddPanelDialog open={true} onClose={onClose} />);

      // 콘텐츠 탭으로 이동
      fireEvent.click(screen.getByRole('button', { name: '콘텐츠' }));
      // "텍스트" 패널 선택
      fireEvent.click(screen.getByText('텍스트'));

      expect(storeState.addPanelCalls).toEqual([{ type: 'text' }]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  // ---- Chart panel new behavior (REQ-M5-01/02/04) ----

  describe('차트 패널 channel_name 단계 (REQ-M5-01/02/04)', () => {
    it('차트 타입 선택 시 채널 선택 step 으로 진입 (REQ-M5-01)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('라인 차트'));

      await waitFor(() => {
        expect(screen.getByText(/채널 선택$/)).toBeInTheDocument();
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
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('통계'));

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
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('바 차트'));

      const select = await screen.findByTestId('chart-channel-select');
      fireEvent.change(select, { target: { value: '__custom__' } });
      expect(screen.getByTestId('chart-channel-custom-input')).toBeInTheDocument();
    });

    it('유효하지 않은 channel_name 은 에러 메시지 + 저장 비활성 (REQ-M5-04)', async () => {
      render(<AddPanelDialog open={true} onClose={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('파이 차트'));

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
      fireEvent.click(screen.getByRole('button', { name: '데이터' }));
      fireEvent.click(screen.getByText('테이블'));

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
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('통계'));

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
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('라인 차트'));

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
      fireEvent.click(screen.getByRole('button', { name: '차트' }));
      fireEvent.click(screen.getByText('통계'));

      await waitFor(() => {
        expect(screen.getByText(/채널 목록 조회에 실패/)).toBeInTheDocument();
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
});
