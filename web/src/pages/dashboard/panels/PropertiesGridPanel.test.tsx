// 디바이스 현황 패널 — 카드 끌어 옮기기 + 항목별 카드 배치.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// 디바이스 메타데이터·측정 시각을 테스트마다 바꿀 수 있게 들고 있는다.
const deviceMock = vi.hoisted(() => ({
  metadata: undefined as Record<string, unknown> | undefined,
  properties: { temperature: 21, humidity: 55 } as Record<string, unknown>,
}));

vi.mock('@/hooks/useDevice', () => ({
  useDeviceRealtime: () => ({
    data: {
      id: 'dev-1',
      name: '센서',
      type: 'sensor',
      protocol: 'modbus',
      metadata: deviceMock.metadata,
      state: { properties: deviceMock.properties },
    },
    isLoading: false,
  }),
}));

vi.mock('@/hooks/useDetailTargets', () => ({
  useDeviceDetailTarget: () => ({ data: undefined, isLoading: false }),
}));

import { PanelEditContext } from '../panelEditContext';
import PropertiesGridPanel from './PropertiesGridPanel';

const BASE = { deviceId: 'dev-1', visibleProperties: ['temperature', 'humidity'] };

function renderPanel(config: Record<string, unknown>, editing: boolean, onConfigChange = vi.fn()) {
  render(
    <PanelEditContext.Provider value={editing}>
      <PropertiesGridPanel
        panelId="p1"
        title="현황"
        config={config}
        onConfigChange={onConfigChange}
      />
    </PanelEditContext.Provider>,
  );
  return onConfigChange;
}

/** 카드를 끌어 다른 카드 위에 놓는다. */
function dragCard(from: number, to: number): void {
  const cards = screen.getAllByTestId('property-card');
  fireEvent.mouseDown(cards[from]!, { button: 0 });
  fireEvent.mouseEnter(cards[to]!);
  fireEvent.mouseUp(cards[to]!);
}

beforeEach(() => {
  vi.clearAllMocks();
  deviceMock.metadata = undefined;
  deviceMock.properties = { temperature: 21, humidity: 55 };
});

describe('카드 끌어 옮기기', () => {
  it('편집 중에는 끌어 놓은 자리로 순서가 바뀐다', () => {
    const onConfigChange = renderPanel(BASE, true);
    dragCard(1, 0);
    expect(onConfigChange).toHaveBeenCalledWith({ visibleProperties: ['humidity', 'temperature'] });
  });

  it('대시보드(편집 아님)에서는 끌어도 아무 일이 없다 — 패널 자체를 끄는 동작과 부딪힌다', () => {
    const onConfigChange = renderPanel(BASE, false);
    dragCard(1, 0);
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('제자리에 놓으면 순서를 쓰지 않는다', () => {
    const onConfigChange = renderPanel(BASE, true);
    dragCard(0, 0);
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('표시 항목을 고르지 않았어도 보이던 차례를 굳혀 옮긴다', () => {
    const onConfigChange = renderPanel({ deviceId: 'dev-1' }, true);
    dragCard(1, 0);
    expect(onConfigChange).toHaveBeenCalledWith({
      visibleProperties: ['humidity', 'temperature'],
    });
  });
});

describe('항목별 카드 배치', () => {
  it('따로 잡은 항목만 그 배치로 그린다', () => {
    renderPanel(
      {
        ...BASE,
        cardAreas: { label: { row: 1, col: 1, rowSpan: 1, colSpan: 1 } },
        propertyOverrides: {
          temperature: { cardAreas: { label: { row: 1, col: 1, rowSpan: 1, colSpan: 3 } } },
        },
      },
      false,
    );
    const labels = screen.getAllByTestId('property-label');
    // 첫 카드(temperature)만 한 줄 전체를 쓴다.
    expect(labels[0]).toHaveAttribute('data-area', '1,1,1,3');
    expect(labels[1]).toHaveAttribute('data-area', '1,1,1,1');
  });

  it('항목별 설정이 없으면 전체 배치를 그대로 쓴다', () => {
    renderPanel({ ...BASE, cardAreas: { label: { row: 2, col: 1, rowSpan: 1, colSpan: 2 } } }, false);
    const labels = screen.getAllByTestId('property-label');
    expect(labels[0]).toHaveAttribute('data-area', '2,1,1,2');
    expect(labels[1]).toHaveAttribute('data-area', '2,1,1,2');
  });
});

describe('글자 정렬', () => {
  it('고른 정렬이 칸 위치에서 정해지던 정렬을 이긴다', () => {
    // 값 조각을 1열에 두면 원래는 왼쪽 정렬이다. 가운데를 고르면 그것이 이겨야 한다.
    renderPanel(
      {
        ...BASE,
        cardAreas: { value: { row: 2, col: 1, rowSpan: 1, colSpan: 1 } },
        value_font: { align: 'center' },
      },
      false,
    );
    expect(screen.getAllByTestId('property-value')[0]!.firstElementChild).toHaveStyle({
      textAlign: 'center',
    });
  });

  it('고르지 않으면 지금까지의 정렬이 그대로 산다', () => {
    renderPanel({ ...BASE, cardAreas: { value: { row: 2, col: 1, rowSpan: 1, colSpan: 1 } } }, false);
    expect(screen.getAllByTestId('property-value')[0]!.firstElementChild).not.toHaveStyle({
      textAlign: 'center',
    });
  });
});

describe('갱신 시간 제한', () => {
  /** 마지막 갱신이 `ageSec` 초 전인 측정값을 심는다. */
  function withAge(ageSec: number): void {
    deviceMock.properties = {
      measurements: {
        temperature: { value: 21, time_ms: Date.now() - ageSec * 1000 },
      },
    };
  }

  it('제한을 넘긴 값은 흐려지고 오래됨 표시가 붙는다', () => {
    withAge(600);
    deviceMock.metadata = { stale_after_sec: 60 };
    renderPanel({ deviceId: 'dev-1' }, false);
    expect(screen.getByTestId('property-stale')).toBeInTheDocument();
    expect(screen.getAllByTestId('property-card')[0]).toHaveClass('opacity-50');
  });

  it('제한 안의 값은 그대로 둔다', () => {
    withAge(10);
    deviceMock.metadata = { stale_after_sec: 60 };
    renderPanel({ deviceId: 'dev-1' }, false);
    expect(screen.queryByTestId('property-stale')).not.toBeInTheDocument();
    expect(screen.getAllByTestId('property-card')[0]).not.toHaveClass('opacity-50');
  });

  it('제한이 없으면 아무리 오래되어도 표시하지 않는다', () => {
    withAge(999_999);
    renderPanel({ deviceId: 'dev-1' }, false);
    expect(screen.queryByTestId('property-stale')).not.toBeInTheDocument();
    expect(screen.getAllByTestId('property-card')[0]).not.toHaveClass('opacity-50');
  });
})

describe('첫 통신 전 고정 디바이스', () => {
  it('값이 하나도 없어도 고른 항목의 카드를 그리고 값은 \'-\' 로 둔다', () => {
    deviceMock.properties = {};
    renderPanel(BASE, false);

    const cards = screen.getAllByTestId('property-card');
    expect(cards).toHaveLength(2);
    for (const v of screen.getAllByTestId('property-value')) {
      expect(v.textContent).toBe('-');
    }
  });

  it('고른 항목도 값도 없을 때만 안내 문구를 낸다', () => {
    deviceMock.properties = {};
    renderPanel({ deviceId: 'dev-1' }, false);

    expect(screen.queryByTestId('property-card')).not.toBeInTheDocument();
    expect(screen.getByText('dashboard.panel.noProperties')).toBeInTheDocument();
  });

  it('값이 도착하면 자리표시자가 실제 값으로 바뀐다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(BASE, false);

    const values = screen.getAllByTestId('property-value').map((v) => v.textContent);
    expect(values).toEqual(['21\u00B0C', '-']);
  });
})

describe('갱신 시각 자리', () => {
  it('아직 안 온 값은 시각 자리에 \'수신 전\' 을 낸다', () => {
    deviceMock.properties = {};
    renderPanel(BASE, false);

    expect(screen.getAllByTestId('property-awaiting')).toHaveLength(2);
    for (const el of screen.getAllByTestId('property-awaiting')) {
      expect(el.textContent).toBe('dashboard.panel.awaitingData');
    }
  });

  it('메타데이터처럼 통신이 필요 없는 항목은 시각 자리를 비운다', () => {
    deviceMock.properties = {};
    deviceMock.metadata = { location: '사무실' };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['meta.location'] }, false);

    expect(screen.queryByTestId('property-awaiting')).not.toBeInTheDocument();
    // 비었어도 자리는 남는다 — 없으면 그 카드만 한 줄 짧아진다.
    expect(screen.getByTestId('property-time')).toBeInTheDocument();
  });

  it('통신이 필요 없는 항목과 기다리는 항목의 카드 높이가 같다 — 시각 자리를 늘 잡는다', () => {
    deviceMock.properties = {};
    deviceMock.metadata = { location: '사무실' };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature', 'meta.location'] }, false);

    // 두 카드 모두 시각 조각을 갖는다(하나는 \'수신 전\', 하나는 공백).
    expect(screen.getAllByTestId('property-time')).toHaveLength(2);
    expect(screen.getAllByTestId('property-awaiting')).toHaveLength(1);
  });

  it('값이 도착하면 시각을 그대로 낸다', () => {
    deviceMock.properties = {
      measurements: { temperature: { value: 21, time_ms: Date.now() - 5000 } },
    };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature'] }, false);

    expect(screen.queryByTestId('property-awaiting')).not.toBeInTheDocument();
    expect(screen.getByTestId('property-time').textContent).not.toBe('');
  });

  it('갱신 시각 표시를 끄면 자리 자체가 없다', () => {
    deviceMock.properties = {};
    renderPanel({ ...BASE, showUpdatedAt: false }, false);

    expect(screen.queryByTestId('property-time')).not.toBeInTheDocument();
    expect(screen.queryByTestId('property-awaiting')).not.toBeInTheDocument();
  });
})

describe('그룹 분리', () => {
  it('기본 정보 · 상태 정보 · 수신 정보로 나눈다', () => {
    deviceMock.metadata = { location: '사무실' };
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      { deviceId: 'dev-1', visibleProperties: ['meta.location', 'temperature', 'gw.rssi'] },
      false,
    );
    expect(screen.getByTestId('properties-grid-group-basic')).toBeInTheDocument();
    expect(screen.getByTestId('properties-grid-group-status')).toBeInTheDocument();
    expect(screen.getByTestId('properties-grid-group-gateway')).toBeInTheDocument();
  });

  it('값이 없는 그룹은 아예 내지 않는다 — 빈 제목만 남으면 읽히지 않는다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature'] }, false);
    expect(screen.getByTestId('properties-grid-group-status')).toBeInTheDocument();
    expect(screen.queryByTestId('properties-grid-group-basic')).not.toBeInTheDocument();
    expect(screen.queryByTestId('properties-grid-group-gateway')).not.toBeInTheDocument();
  });

  it('그룹마다 제 격자에 놓는다 — 기본 8x4, 카드 2x2', () => {
    deviceMock.properties = { temperature: 21, humidity: 55 };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature', 'humidity'] }, false);
    expect(screen.getByTestId('properties-grid-status-grid')).toHaveStyle({
      gridTemplateColumns: 'repeat(8, minmax(0, 1fr))',
    });
    const cards = screen.getAllByTestId('property-card');
    expect(cards[0]).toHaveStyle({ gridColumn: '1 / span 2', gridRow: '1 / span 2' });
    expect(cards[1]).toHaveStyle({ gridColumn: '3 / span 2' });
  });

  it('저장된 자리를 그대로 쓴다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        statusAreas: { temperature: { x: 5, y: 3, w: 4, h: 2 } },
      },
      false,
    );
    expect(screen.getAllByTestId('property-card')[0]).toHaveStyle({
      gridColumn: '5 / span 4',
      gridRow: '3 / span 2',
    });
  });

  it('그룹별 격자 크기를 따로 정한다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      { deviceId: 'dev-1', visibleProperties: ['temperature'], statusGrid: { rows: 2, cols: 4 } },
      false,
    );
    expect(screen.getByTestId('properties-grid-status-grid')).toHaveStyle({
      gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    });
  });
});

describe('배지', () => {
  it('기본은 갱신 · 종류 · 프로토콜 셋 — 연결은 헤더 아이콘이 이미 말한다', () => {
    deviceMock.properties = {
      measurements: { temperature: { value: 21, time_ms: Date.now() - 5000 } },
    };
    renderPanel({ deviceId: 'dev-1' }, false);
    expect(screen.getByTestId('properties-grid-badge-updated').textContent).not.toContain(
      'awaitingData',
    );
    expect(screen.getByTestId('properties-grid-badge-protocol').textContent).toContain('MODBUS');
    expect(screen.getByTestId('properties-grid-badge-kind')).toBeInTheDocument();
    expect(screen.queryByTestId('properties-grid-badge-online')).not.toBeInTheDocument();
  });

  it('갱신 시각이 없으면 수신 전으로 낸다', () => {
    deviceMock.properties = {};
    renderPanel({ deviceId: 'dev-1' }, false);
    expect(screen.getByTestId('properties-grid-badge-updated').textContent).toContain(
      'dashboard.panel.awaitingData',
    );
  });

  it('여러 항목 중 가장 최근 시각을 쓴다 — 가장 오래된 것을 쓰면 방금 온 값이 묻힌다', () => {
    const now = Date.now();
    deviceMock.properties = {
      measurements: {
        temperature: { value: 21, time_ms: now - 600_000 },
        humidity: { value: 55, time_ms: now - 1000 },
      },
    };
    renderPanel({ deviceId: 'dev-1' }, false);
    // 10분 전이 아니라 방금 전으로 읽혀야 한다.
    expect(screen.getByTestId('properties-grid-badge-updated').title).toBeTruthy();
  });

  it('끄면 배지 줄이 없다', () => {
    renderPanel({ deviceId: 'dev-1', showBadges: false }, false);
    expect(screen.queryByTestId('properties-grid-badges')).not.toBeInTheDocument();
  });
});

describe('타일 이름', () => {
  it('정한 이름을 쓴다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { label: '실내 온도' } },
      },
      false,
    );
    expect(screen.getByTestId('property-label').textContent).toBe('실내 온도');
  });

  it('공백만 있는 이름은 정하지 않은 것으로 본다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { label: '   ' } },
      },
      false,
    );
    expect(screen.getByTestId('property-label').textContent).not.toBe('   ');
  });
})

describe('수신 전 그룹 표시', () => {
  it('데이터가 오기 전에도 기본 정보와 수신 정보를 낸다', () => {
    deviceMock.properties = {};
    deviceMock.metadata = { location: '사무실' };
    renderPanel(
      { deviceId: 'dev-1', visibleProperties: ['meta.location', 'gw.rssi'] },
      false,
    );
    expect(screen.getByTestId('properties-grid-group-basic')).toBeInTheDocument();
    expect(screen.getByTestId('properties-grid-group-gateway')).toBeInTheDocument();
  });
})

describe('타일 배경색', () => {
  it('정하면 타일에 얹고 기본 배경 클래스를 걷어낸다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      { deviceId: 'dev-1', visibleProperties: ['temperature'], tileBg: '#112233' },
      false,
    );
    const card = screen.getAllByTestId('property-card')[0]!;
    expect(card).toHaveStyle({ backgroundColor: '#112233' });
    expect(card.className).not.toContain('bg-(--color-bg-surface)');
  });

  it('정하지 않으면 패널 기본 배경이 그대로 산다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature'] }, false);
    expect(screen.getAllByTestId('property-card')[0]!.className).toContain('bg-(--color-bg-surface)');
  });
})

describe('배지 항목 고르기', () => {
  it('고른 것만 고른 차례로 낸다', () => {
    renderPanel({ deviceId: 'dev-1', badgeItems: ['protocol', 'updated'] }, false);
    const rendered = Array.from(screen.getByTestId('properties-grid-badges').children).map((el) =>
      el.getAttribute('data-testid'),
    );
    expect(rendered).toEqual([
      'properties-grid-badge-protocol',
      'properties-grid-badge-updated',
    ]);
  });

  it('연결 배지를 켜면 온라인 여부를 글자로 낸다', () => {
    renderPanel({ deviceId: 'dev-1', badgeItems: ['online'] }, false);
    expect(screen.getByTestId('properties-grid-badge-online')).toBeInTheDocument();
  });

  it('전부 끄면 배지 줄이 없다', () => {
    renderPanel({ deviceId: 'dev-1', badgeItems: [] }, false);
    expect(screen.queryByTestId('properties-grid-badges')).not.toBeInTheDocument();
  });

  it('항목마다 모양을 따로 정한다', () => {
    renderPanel(
      {
        deviceId: 'dev-1',
        badgeStyles: { protocol: { value_font: { size: 16 }, bg: '#112233' } },
      },
      false,
    );
    const tile = screen.getByTestId('properties-grid-badge-protocol');
    expect(tile).toHaveStyle({ fontSize: '16px', backgroundColor: '#112233' });
    expect(screen.getByTestId('properties-grid-badge-kind')).not.toHaveStyle({ fontSize: '16px' });
  });

  it('타이틀 옆 프로토콜 글자는 없앴다 — 같은 것을 두 번 말하지 않는다', () => {
    renderPanel({ deviceId: 'dev-1', badgeItems: [] }, false);
    expect(screen.queryByText('MODBUS')).not.toBeInTheDocument();
  });
})

describe('단위', () => {
  it('정한 단위를 값 뒤에 붙인다', () => {
    deviceMock.properties = { co2: 812 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['co2'],
        propertyOverrides: { co2: { unit: 'ppm' } },
      },
      false,
    );
    expect(screen.getByTestId('property-value').textContent).toBe('812 ppm');
  });

  it('정하지 않으면 종전 그대로', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel({ deviceId: 'dev-1', visibleProperties: ['temperature'] }, false);
    expect(screen.getByTestId('property-value').textContent).toBe('21°C');
  });
})

describe('항목별 배경색', () => {
  it('그 항목에만 걸린다', () => {
    deviceMock.properties = { temperature: 21, humidity: 55 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['temperature', 'humidity'],
        propertyOverrides: { temperature: { bg: '#331111' } },
      },
      false,
    );
    const cards = screen.getAllByTestId('property-card');
    expect(cards[0]).toHaveStyle({ backgroundColor: '#331111' });
    expect(cards[1]).not.toHaveStyle({ backgroundColor: '#331111' });
  });

  it('공통 배경을 이긴다 — 좁은 쪽이 이긴다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        tileBg: '#112233',
        propertyOverrides: { temperature: { bg: '#331111' } },
      },
      false,
    );
    expect(screen.getAllByTestId('property-card')[0]).toHaveStyle({ backgroundColor: '#331111' });
  });

  it('정하지 않으면 공통 배경을 따른다', () => {
    deviceMock.properties = { temperature: 21 };
    renderPanel(
      { deviceId: 'dev-1', visibleProperties: ['temperature'], tileBg: '#112233' },
      false,
    );
    expect(screen.getAllByTestId('property-card')[0]).toHaveStyle({ backgroundColor: '#112233' });
  });
})
