// 시리즈 세부 설정의 이름 토큰 UI 테스트.
//
// 토큰 목록은 이름 입력 아래 상시 펼침 줄이었다가, 이름 입력 뒤 물음표 도움말로
// 옮겨졌다. 여기서는 (1) 기본 상태에서 토큰이 보이지 않고, (2) 물음표를 눌러야
// 열리며, (3) 열린 뒤 토큰 삽입이 종전대로 동작하고, (4) 미리보기는 물음표와
// 무관하게 상시 노출된다는 네 가지를 고정한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { SeriesDetailEditor } from './ChartPanelSections';
import type { StoreSeriesRef } from './panels/charts/chartChannelTypes';

function makeSeries(over: Partial<StoreSeriesRef> = {}): StoreSeriesRef {
  return {
    key: 'temp',
    field: 'value',
    tags: { location: '실습실', spot: 'A1' },
    ...over,
  } as StoreSeriesRef;
}

function renderEditor(over: Partial<StoreSeriesRef> = {}) {
  const onPatch = vi.fn();
  render(
    <SeriesDetailEditor
      series={makeSeries(over)}
      index={0}
      isLineChart={false}
      onPatch={onPatch}
    />,
  );
  return { onPatch };
}

describe('시리즈 세부 설정 — 이름 토큰 도움말', () => {
  it('기본 상태에서는 토큰 목록이 보이지 않는다', () => {
    renderEditor();

    expect(screen.getByTestId('chart-store-series-0-token-help')).toBeTruthy();
    expect(screen.queryByTestId('chart-store-series-0-tokens')).toBeNull();
    expect(screen.queryByTestId('chart-store-series-0-token-measurement')).toBeNull();
  });

  it('물음표를 누르면 사용 가능한 토큰이 열린다', () => {
    renderEditor();

    fireEvent.click(screen.getByTestId('chart-store-series-0-token-help'));

    expect(screen.getByTestId('chart-store-series-0-tokens')).toBeTruthy();
    // measurement / field / 태그 2종.
    expect(screen.getByTestId('chart-store-series-0-token-measurement').textContent).toBe(
      '{$.measurement}',
    );
    expect(screen.getByTestId('chart-store-series-0-token-field')).toBeTruthy();
    expect(screen.getByTestId('chart-store-series-0-token-location')).toBeTruthy();
    expect(screen.getByTestId('chart-store-series-0-token-spot')).toBeTruthy();
  });

  it('다시 누르면 닫힌다', () => {
    renderEditor();
    const btn = screen.getByTestId('chart-store-series-0-token-help');

    fireEvent.click(btn);
    expect(screen.getByTestId('chart-store-series-0-tokens')).toBeTruthy();

    fireEvent.click(btn);
    expect(screen.queryByTestId('chart-store-series-0-tokens')).toBeNull();
  });

  it('토큰을 누르면 이름에 삽입된다', () => {
    const { onPatch } = renderEditor();

    fireEvent.click(screen.getByTestId('chart-store-series-0-token-help'));
    fireEvent.click(screen.getByTestId('chart-store-series-0-token-location'));

    expect(onPatch).toHaveBeenCalledWith({ alias: '{$.location}' });
  });

  it('연속 삽입을 위해 토큰 클릭 후에도 목록은 열려 있다', () => {
    renderEditor();

    fireEvent.click(screen.getByTestId('chart-store-series-0-token-help'));
    fireEvent.click(screen.getByTestId('chart-store-series-0-token-spot'));

    expect(screen.getByTestId('chart-store-series-0-tokens')).toBeTruthy();
  });

  it('미리보기는 물음표와 무관하게 상시 노출된다', () => {
    renderEditor({ alias: '{$.location}-{$.spot}' });

    const preview = screen.getByTestId('chart-store-series-preview-0');
    expect(preview.textContent).toContain('실습실-A1');
    // 토큰 목록은 여전히 닫혀 있다.
    expect(screen.queryByTestId('chart-store-series-0-tokens')).toBeNull();
  });

  it('이름이 비면 미리보기는 키명으로 폴백한다', () => {
    renderEditor({ alias: undefined });

    expect(screen.getByTestId('chart-store-series-preview-0').textContent).toContain('temp');
  });

  it('삽입 가능한 토큰이 없으면 물음표를 노출하지 않는다', () => {
    render(
      <SeriesDetailEditor
        series={{ key: '', field: undefined, tags: {} } as StoreSeriesRef}
        index={0}
        isLineChart={false}
        onPatch={vi.fn()}
      />,
    );

    expect(screen.queryByTestId('chart-store-series-0-token-help')).toBeNull();
  });
});

// 한 줄 배치 — 이름·미리보기·색상·선 스타일이 같은 wrap 행에 나란히 놓인다.
//
// jsdom 에는 레이아웃 엔진이 없어 "실제로 한 줄에 그려지는지"는 잴 수 없다. 대신
// 그 배치의 전제 두 가지를 고정한다: (1) 네 항목이 같은 컨테이너의 형제일 것,
// (2) 그 컨테이너가 wrap 가능할 것. 항목이 서로 다른 줄 컨테이너로 흩어지면
// 폭이 남아도 한 줄로 모이지 않는다.
describe('시리즈 세부 설정 — 한 줄 배치', () => {
  function renderLineChartEditor() {
    render(
      <SeriesDetailEditor
        series={makeSeries({ alias: '{$.location}' })}
        index={0}
        isLineChart
        onPatch={vi.fn()}
      />,
    );
    return screen.getByTestId('series-detail-0');
  }

  it('이름·미리보기·색상·선 스타일이 같은 컨테이너의 직계 자식이다', () => {
    const row = renderLineChartEditor();

    const nameInput = screen.getByTestId('chart-store-series-alias-0');
    const preview = screen.getByTestId('chart-store-series-preview-0');
    const color = screen.getByTestId('chart-store-series-color-0');
    const lineStyle = screen.getByTestId('chart-store-series-0-line-style');

    // 이름·색상은 라벨과 묶인 그룹 안에 있으므로 그룹이 직계 자식이다.
    expect(nameInput.parentElement?.parentElement).toBe(row);
    expect(color.parentElement?.parentElement).toBe(row);
    // 미리보기·선 스타일은 그 자체가 직계 자식이다.
    expect(preview.parentElement).toBe(row);
    expect(lineStyle.parentElement).toBe(row);
  });

  it('행 컨테이너는 폭이 좁으면 접힌다(flex-wrap)', () => {
    const row = renderLineChartEditor();

    expect(row.className).toContain('flex');
    expect(row.className).toContain('flex-wrap');
  });

  it('라벨+컨트롤 그룹은 내부에서 접히지 않는다', () => {
    renderLineChartEditor();

    // 라벨만 윗줄에 남고 입력이 아랫줄로 떨어지면 무엇의 라벨인지 읽히지 않는다.
    const nameGroup = screen.getByTestId('chart-store-series-alias-0').parentElement!;
    const colorGroup = screen.getByTestId('chart-store-series-color-0').parentElement!;

    expect(nameGroup.className).not.toContain('flex-wrap');
    expect(colorGroup.className).not.toContain('flex-wrap');
  });
});
