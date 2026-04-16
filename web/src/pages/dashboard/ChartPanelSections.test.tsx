// ChartPanelSections 단위 테스트.
// 각 섹션이 SPEC-CHART-001 §4.2.2 의 config 필드를 올바르게 렌더/편집하는지 검증.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import {
  ChartChannelSection,
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
} from './ChartPanelSections';
import type { PanelConfig } from '@/stores/uiStore';

function makePanel(type: PanelConfig['type'], config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type, title: '테스트', config };
}

describe('ChartChannelSection (REQ-M5-04 검증)', () => {
  it('유효한 channel_name 을 blur 시 저장', () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: '' })}
        onConfigChange={onConfigChange}
      />,
    );
    const input = screen.getByTestId('chart-channel-name-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'valid_name-1' } });
    fireEvent.blur(input);
    expect(onConfigChange).toHaveBeenCalledWith({ channel_name: 'valid_name-1' });
  });

  it('잘못된 channel_name 은 에러 표시 + onConfigChange 호출 안 함', () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('line-chart', { channel_name: '' })}
        onConfigChange={onConfigChange}
      />,
    );
    const input = screen.getByTestId('chart-channel-name-input');
    fireEvent.change(input, { target: { value: 'abc/def' } });
    expect(screen.getByTestId('chart-channel-name-error')).toHaveTextContent(
      '유효한 채널 이름이 아닙니다',
    );
    fireEvent.blur(input);
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('빈 값 blur 시 에러 메시지 표시 안 함 (초기화 허용)', () => {
    const onConfigChange = vi.fn();
    render(
      <ChartChannelSection
        panel={makePanel('stat', { channel_name: 'old_name' })}
        onConfigChange={onConfigChange}
      />,
    );
    const input = screen.getByTestId('chart-channel-name-input');
    fireEvent.change(input, { target: { value: '' } });
    expect(screen.queryByTestId('chart-channel-name-error')).toBeNull();
    fireEvent.blur(input);
    expect(onConfigChange).toHaveBeenCalledWith({ channel_name: '' });
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
  it('max_points 편집', () => {
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
