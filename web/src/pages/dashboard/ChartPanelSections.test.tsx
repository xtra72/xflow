// ChartPanelSections 단위 테스트.
// 각 섹션이 SPEC-CHART-001 §4.2.2 의 config 필드를 올바르게 렌더/편집하는지 검증.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import {
  ChartChannelSection,
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
} from './ChartPanelSections';
import type { PanelConfig } from '@/stores/uiStore';
import type { ChartChannelSummary } from '@/services/api/charts';

function makePanel(type: PanelConfig['type'], config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type, title: '테스트', config };
}

const emptyChannels: () => Promise<ChartChannelSummary[]> = () => Promise.resolve([]);
const twoChannels: () => Promise<ChartChannelSummary[]> = () =>
  Promise.resolve([
    {
      name: 'room1_temp',
      flow_id: 'flow-a',
      node_id: 'n1',
      buffer_size: 100,
      retention_sec: 3600,
      subscriber_count: 2,
      last_message_ms: 0,
    },
    {
      name: 'pump_rpm',
      flow_id: 'flow-b',
      node_id: 'n2',
      buffer_size: 50,
      retention_sec: 60,
      subscriber_count: 0,
      last_message_ms: 0,
    },
  ]);

describe('ChartChannelSection (REQ-M5-02/04: 드롭다운 + Custom)', () => {
  it('활성 채널 드롭다운에서 선택 시 즉시 저장', async () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: '' })}
        onConfigChange={onConfigChange}
        fetchChannels={twoChannels}
      />,
    );
    const select = await screen.findByTestId('chart-channel-name-select');
    await waitFor(() =>
      expect(screen.getByRole('option', { name: /room1_temp/ })).toBeInTheDocument(),
    );
    fireEvent.change(select, { target: { value: 'room1_temp' } });
    expect(onConfigChange).toHaveBeenCalledWith({ channel_name: 'room1_temp' });
  });

  it('Custom 선택 → 유효한 이름 입력 blur 시 저장', async () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('line-chart', { channel_name: '' })}
        onConfigChange={onConfigChange}
        fetchChannels={emptyChannels}
      />,
    );
    const select = await screen.findByTestId('chart-channel-name-select');
    fireEvent.change(select, { target: { value: '__custom__' } });

    const input = (await screen.findByTestId('chart-channel-name-input')) as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'valid_name-1' } });
    fireEvent.blur(input);
    expect(onConfigChange).toHaveBeenCalledWith({ channel_name: 'valid_name-1' });
  });

  it('Custom 모드에서 잘못된 이름 → 에러 표시 + commit 안 함', async () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('line-chart', { channel_name: '' })}
        onConfigChange={onConfigChange}
        fetchChannels={emptyChannels}
      />,
    );
    const select = await screen.findByTestId('chart-channel-name-select');
    fireEvent.change(select, { target: { value: '__custom__' } });

    const input = await screen.findByTestId('chart-channel-name-input');
    fireEvent.change(input, { target: { value: 'abc/def' } });
    expect(screen.getByTestId('chart-channel-name-error')).toHaveTextContent(
      '유효한 채널 이름이 아닙니다',
    );
    fireEvent.blur(input);
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('드롭다운에서 빈 값 선택 시 channel_name 초기화', async () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: 'old_name' })}
        onConfigChange={onConfigChange}
        fetchChannels={emptyChannels}
      />,
    );
    const select = await screen.findByTestId('chart-channel-name-select');
    fireEvent.change(select, { target: { value: '' } });
    expect(onConfigChange).toHaveBeenCalledWith({ channel_name: '' });
  });

  it('비활성 채널 이름은 "(현재 선택, 비활성)" 옵션으로 유지', async () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: 'gone_channel' })}
        onConfigChange={onConfigChange}
        fetchChannels={twoChannels}
      />,
    );
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /gone_channel — \(현재 선택, 비활성\)/ }),
      ).toBeInTheDocument(),
    );
  });

  it('목록 조회 실패 시 Custom 입력 유도 메시지 표시', async () => {
    const onConfigChange = vi.fn();
    const fail = () => Promise.reject(new Error('network down'));
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: '' })}
        onConfigChange={onConfigChange}
        fetchChannels={fail}
      />,
    );
    await waitFor(() =>
      expect(screen.getByText(/채널 목록 조회 실패/)).toBeInTheDocument(),
    );
  });
});

describe('StatChartSection', () => {
  it('display_field / unit / decimal_places 편집', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection
        panel={makePanel('stat', {
          display_field: 'value',
          unit: '',
          decimal_places: 2,
        })}
        onConfigChange={onConfigChange}
      />,
    );
    // unit 변경
    const unitInput = screen.getByPlaceholderText(/°C/);
    fireEvent.change(unitInput, { target: { value: 'kWh' } });
    expect(onConfigChange).toHaveBeenCalledWith({ unit: 'kWh' });
  });

  it('threshold_color_rules 추가/제거', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection
        panel={makePanel('stat', { threshold_color_rules: [] })}
        onConfigChange={onConfigChange}
      />,
    );
    const addBtn = screen.getByRole('button', { name: /추가/ });
    fireEvent.click(addBtn);
    expect(onConfigChange).toHaveBeenCalledWith({
      threshold_color_rules: [{ min: 0, color: '#3b82f6' }],
    });
  });
});

describe('LineChartSection', () => {
  it('max_points 편집 (기본 points 모드)', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', { max_points: 100 })}
        onConfigChange={onConfigChange}
      />,
    );
    const label = screen.getByText(/최대 포인트/);
    const input = label.parentElement?.querySelector('input[type="number"]') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '250' } });
    expect(onConfigChange).toHaveBeenCalledWith({ max_points: 250 });
  });

  // --- 다채널 행에 채널 드롭다운 ---
  describe('multi-channel rows: channel dropdown', () => {
    it('채널 추가 시 행에 채널 드롭다운 노출', async () => {
      render(
        <LineChartSection
          panel={makePanel('line-chart', { channels: [{ name: '' }] })}
          onConfigChange={vi.fn()}
          fetchChannels={twoChannels}
        />,
      );
      const select = await screen.findByTestId('line-chart-channel-row-select-0');
      expect(select).toBeInTheDocument();
      await waitFor(() =>
        expect(screen.getByRole('option', { name: /room1_temp/ })).toBeInTheDocument(),
      );
    });

    it('드롭다운 선택 시 해당 채널 row 의 name 만 갱신', async () => {
      const onConfigChange = vi.fn();
      render(
        <LineChartSection
          panel={makePanel('line-chart', {
            channels: [
              { name: '', alias: 'A' },
              { name: 'pump_rpm', alias: 'B' },
            ],
          })}
          onConfigChange={onConfigChange}
          fetchChannels={twoChannels}
        />,
      );
      const select = (await screen.findByTestId(
        'line-chart-channel-row-select-0',
      )) as HTMLSelectElement;
      await waitFor(() =>
        expect(
          Array.from(select.options).some((o) => o.value === 'room1_temp'),
        ).toBe(true),
      );
      fireEvent.change(select, { target: { value: 'room1_temp' } });
      expect(onConfigChange).toHaveBeenCalledWith({
        channels: [
          { name: 'room1_temp', alias: 'A' },
          { name: 'pump_rpm', alias: 'B' },
        ],
      });
    });

    it('Custom 옵션 선택 시 수동 입력 input 표시', async () => {
      render(
        <LineChartSection
          panel={makePanel('line-chart', { channels: [{ name: '' }] })}
          onConfigChange={vi.fn()}
          fetchChannels={twoChannels}
        />,
      );
      const select = await screen.findByTestId('line-chart-channel-row-select-0');
      fireEvent.change(select, { target: { value: '__custom__' } });
      expect(
        await screen.findByTestId('line-chart-channel-row-custom-0'),
      ).toBeInTheDocument();
    });

    it('Custom 입력 blur 시 channels 배열 갱신', async () => {
      const onConfigChange = vi.fn();
      render(
        <LineChartSection
          panel={makePanel('line-chart', { channels: [{ name: '', alias: 'X' }] })}
          onConfigChange={onConfigChange}
          fetchChannels={emptyChannels}
        />,
      );
      const select = await screen.findByTestId('line-chart-channel-row-select-0');
      fireEvent.change(select, { target: { value: '__custom__' } });
      const input = await screen.findByTestId('line-chart-channel-row-custom-0');
      fireEvent.change(input, { target: { value: 'pending_ch' } });
      fireEvent.blur(input);
      expect(onConfigChange).toHaveBeenCalledWith({
        channels: [{ name: 'pending_ch', alias: 'X' }],
      });
    });

    it('각 행에 drag handle 노출', async () => {
      render(
        <LineChartSection
          panel={makePanel('line-chart', {
            channels: [{ name: 'a' }, { name: 'b' }],
          })}
          onConfigChange={vi.fn()}
          fetchChannels={emptyChannels}
        />,
      );
      expect(await screen.findByTestId('line-chart-channel-drag-0')).toBeInTheDocument();
      expect(screen.getByTestId('line-chart-channel-drag-1')).toBeInTheDocument();
    });

    it('drop 으로 첫 행을 끝으로 이동 시 channels 순서 변경', async () => {
      const onConfigChange = vi.fn();
      render(
        <LineChartSection
          panel={makePanel('line-chart', {
            channels: [
              { name: 'a', alias: 'A' },
              { name: 'b', alias: 'B' },
              { name: 'c', alias: 'C' },
            ],
          })}
          onConfigChange={onConfigChange}
          fetchChannels={emptyChannels}
        />,
      );
      const handle0 = await screen.findByTestId('line-chart-channel-drag-0');
      const row2 = screen.getByTestId('line-chart-channel-row-2');

      // jsdom 의 dataTransfer 를 setData/getData 로 대체
      const dt: Record<string, string> = {};
      const dataTransfer = {
        setData: (k: string, v: string) => {
          dt[k] = v;
        },
        getData: (k: string) => dt[k] ?? '',
        effectAllowed: '',
        dropEffect: '',
      };

      fireEvent.dragStart(handle0, { dataTransfer });
      fireEvent.dragOver(row2, { dataTransfer });
      fireEvent.drop(row2, { dataTransfer });

      expect(onConfigChange).toHaveBeenCalledWith({
        channels: [
          { name: 'b', alias: 'B' },
          { name: 'c', alias: 'C' },
          { name: 'a', alias: 'A' },
        ],
      });
    });

    it('같은 위치로 drop 은 무시', async () => {
      const onConfigChange = vi.fn();
      render(
        <LineChartSection
          panel={makePanel('line-chart', {
            channels: [{ name: 'a' }, { name: 'b' }],
          })}
          onConfigChange={onConfigChange}
          fetchChannels={emptyChannels}
        />,
      );
      const handle0 = await screen.findByTestId('line-chart-channel-drag-0');
      const row0 = screen.getByTestId('line-chart-channel-row-0');
      const dt: Record<string, string> = {};
      const dataTransfer = {
        setData: (k: string, v: string) => {
          dt[k] = v;
        },
        getData: (k: string) => dt[k] ?? '',
        effectAllowed: '',
        dropEffect: '',
      };
      fireEvent.dragStart(handle0, { dataTransfer });
      fireEvent.dragOver(row0, { dataTransfer });
      fireEvent.drop(row0, { dataTransfer });
      expect(onConfigChange).not.toHaveBeenCalled();
    });

    it('현재 row name 이 활성 목록에 없으면 (비활성) 옵션으로 표시', async () => {
      render(
        <LineChartSection
          panel={makePanel('line-chart', {
            channels: [{ name: 'undeployed_ch' }],
          })}
          onConfigChange={vi.fn()}
          fetchChannels={twoChannels}
        />,
      );
      await screen.findByRole('option', { name: /room1_temp/ });
      const inactiveOption = screen.getByRole('option', {
        name: /undeployed_ch.*비활성/,
      });
      expect(inactiveOption).toBeInTheDocument();
    });
  });

  it('smooth 체크박스 토글', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', { smooth: false })}
        onConfigChange={onConfigChange}
      />,
    );
    const checkbox = screen.getByRole('checkbox');
    fireEvent.click(checkbox);
    expect(onConfigChange).toHaveBeenCalledWith({ smooth: true });
  });

  // --- 시간 윈도우 모드 ---
  it('time_window_mode=points (기본) 이면 max_points 필드가 노출되고 recent/fixed 필드는 숨김', () => {
    render(
      <LineChartSection
        panel={makePanel('line-chart', {})}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/최대 포인트/)).toBeInTheDocument();
    expect(screen.queryByText(/윈도우 크기 초/)).toBeNull();
    expect(screen.queryByText(/시작 \(epoch ms\)/)).toBeNull();
  });

  it('time_window_mode=recent 선택 시 recent_window_sec + time_window_refresh_ms 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', { time_window_mode: 'recent' })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/윈도우 크기 초/)).toBeInTheDocument();
    expect(screen.getByText(/갱신 주기 ms/)).toBeInTheDocument();
    expect(screen.queryByText(/최대 포인트/)).toBeNull();
  });

  it('time_window_refresh_ms 편집', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', {
          time_window_mode: 'recent',
          time_window_refresh_ms: 1000,
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const label = screen.getByText(/갱신 주기 ms/);
    const input = label.parentElement?.querySelector('input[type="number"]') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '500' } });
    expect(onConfigChange).toHaveBeenCalledWith({ time_window_refresh_ms: 500 });
  });

  it('time_window_mode=fixed 선택 시 start/end 필드 노출', () => {
    render(
      <LineChartSection
        panel={makePanel('line-chart', { time_window_mode: 'fixed' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/시작 \(epoch ms\)/)).toBeInTheDocument();
    expect(screen.getByText(/끝 \(epoch ms/)).toBeInTheDocument();
  });

  // --- Y축 모드 ---
  it('y_axis_mode=auto (기본) 이면 y_min/y_max/padding 필드 모두 숨김', () => {
    render(
      <LineChartSection
        panel={makePanel('line-chart', {})}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByText(/^Y 최소$/)).toBeNull();
    expect(screen.queryByText(/^Y 최대$/)).toBeNull();
    expect(screen.queryByText(/Y축 여백/)).toBeNull();
  });

  it('y_axis_mode=manual 이면 Y 최소/최대 입력 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', { y_axis_mode: 'manual', y_min: 0 })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/^Y 최소$/)).toBeInTheDocument();
    expect(screen.getByText(/^Y 최대$/)).toBeInTheDocument();
  });

  it('y_axis_mode=auto_padded 이면 padding_pct 입력 노출 + 편집', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', {
          y_axis_mode: 'auto_padded',
          y_axis_padding_pct: 5,
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const label = screen.getByText(/Y축 여백/);
    const input = label.parentElement?.querySelector('input[type="number"]') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '10' } });
    expect(onConfigChange).toHaveBeenCalledWith({ y_axis_padding_pct: 10 });
  });

  it('y_axis_mode select 변경 → onConfigChange', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('line-chart', {})}
        onConfigChange={onConfigChange}
      />,
    );
    const selects = screen.getAllByRole('combobox');
    const yModeSelect = selects.find((s) => (s as HTMLSelectElement).value === 'auto')!;
    fireEvent.change(yModeSelect, { target: { value: 'auto_padded' } });
    expect(onConfigChange).toHaveBeenCalledWith({ y_axis_mode: 'auto_padded' });
  });
});

describe('BarChartSection (REQ-M4-06 2 modes)', () => {
  it('mode=category 선택 시 label_field 필드 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { mode: 'category', label_field: 'labels.name' })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/라벨 필드/)).toBeInTheDocument();
    expect(screen.queryByText(/bin 간격/)).toBeNull();
  });

  it('mode=time_bin 선택 시 bin_sec 필드 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { mode: 'time_bin', bin_sec: 60 })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/bin 간격/)).toBeInTheDocument();
  });

  it('agg_func 변경 → onConfigChange', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { agg_func: 'avg' })}
        onConfigChange={onConfigChange}
      />,
    );
    const selects = screen.getAllByRole('combobox');
    // 마지막 select 가 agg_func 으로 추정 (mode, agg_func 순)
    const aggSelect = selects.find((s) => (s as HTMLSelectElement).value === 'avg')!;
    fireEvent.change(aggSelect, { target: { value: 'sum' } });
    expect(onConfigChange).toHaveBeenCalledWith({ agg_func: 'sum' });
  });
});

describe('PieChartSection', () => {
  it('show_legend 토글', () => {
    const onConfigChange = vi.fn();
    render(
      <PieChartSection
        panel={makePanel('pie-chart', { show_legend: true, show_percentage: true })}
        onConfigChange={onConfigChange}
      />,
    );
    const legendCheckbox = screen.getByLabelText(/범례 표시/) as HTMLInputElement;
    fireEvent.click(legendCheckbox);
    expect(onConfigChange).toHaveBeenCalledWith({ show_legend: false });
  });
});

describe('TableChartSection (REQ-M4-08)', () => {
  it('columns 추가', () => {
    const onConfigChange = vi.fn();
    const initialColumns = [
      { field: 'timestamp', header: '시간', format: 'datetime' as const },
      { field: 'value', header: '값', format: 'number' as const },
    ];
    render(
      <TableChartSection
        panel={makePanel('table', { columns: initialColumns })}
        onConfigChange={onConfigChange}
      />,
    );
    const addBtn = screen.getByRole('button', { name: /추가/ });
    fireEvent.click(addBtn);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [...initialColumns, { field: 'value', header: '새 열' }],
    });
  });

  it('columns 수정 (field 변경)', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: 'timestamp', header: '시간' }],
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const fieldInput = screen.getByDisplayValue('timestamp') as HTMLInputElement;
    fireEvent.change(fieldInput, { target: { value: 'ts' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [{ field: 'ts', header: '시간' }],
    });
  });

  it('단일 컬럼은 삭제 버튼 비활성', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: 'value', header: '값' }],
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const delBtn = screen.getByLabelText('열 삭제') as HTMLButtonElement;
    expect(delBtn.disabled).toBe(true);
  });
});
