// 디바이스 현황(properties-grid) 패널의 표시 항목 목록 조작을 검증한다.
//
// 여기서 지키려는 것은 셋이다.
//  1. 전체 선택 / 전체 해제 — 항목이 많을 때 하나씩 누르지 않아도 된다.
//  2. 항목별 설정이 목록의 그 줄에서 열린다.
//  3. 항목별 세부 설정이 propertyOverrides[key] 에 쌓인다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: {
    id: 'p1',
    type: 'properties-grid',
    title: '디바이스 현황',
    config: { deviceId: 'dev-1', visibleProperties: ['temperature'] },
  } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [{ id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] }],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/hooks/useDevice', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useDevice')>();
  return {
    ...actual,
    useDevices: () => ({ data: { data: [{ id: 'dev-1', name: '센서', type: 'sensor' }] } }),
    useDeviceRealtime: () => ({
      data: {
        id: 'dev-1',
        name: '센서',
        type: 'sensor',
        protocol: 'modbus',
        state: { properties: { temperature: 21, humidity: 55 } },
      },
      isLoading: false,
    }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import PanelSettingsDialog from './PanelSettingsDialog';

function renderDialog() {
  return render(
    <QueryClientProvider client={inertQueryClient()}>
      <PanelSettingsDialog panelId="p1" onClose={() => {}} />
    </QueryClientProvider>,
  );
}

/** 항목 디자인은 표시 항목 목록의 각 줄에 있다. */
function openItemDesign(key: string): void {
  fireEvent.click(screen.getByTestId(`property-override-${key}-button`));
}

/** 저장까지 눌러 draft 를 커밋한 뒤, 마지막으로 저장된 config 를 돌려준다. */
function savedConfig(): Record<string, unknown> {
  fireEvent.click(screen.getByText('dashboard.settings.apply'));
  const calls = storeMock.updatePanelConfig.mock.calls;
  expect(calls.length).toBeGreaterThan(0);
  return calls[calls.length - 1]![1] as Record<string, unknown>;
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.panel = {
    id: 'p1',
    type: 'properties-grid',
    title: '디바이스 현황',
    config: { deviceId: 'dev-1', visibleProperties: ['temperature'] },
  };
});

describe('표시 항목 목록', () => {
  it('그룹 전체 선택은 그 그룹 항목을 모두 담는다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-status-select-all'));
    expect(savedConfig().visibleProperties).toEqual(
      expect.arrayContaining(['temperature', 'humidity']),
    );
  });

  it('그룹 전체 해제는 그 그룹만 걷어낸다 — 다른 그룹은 그대로 둔다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['temperature', 'meta.name'] },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-status-clear-all'));
    expect(savedConfig().visibleProperties).toEqual(['meta.name']);
  });

  it('순번은 없애고 디자인은 목록에 남긴다 — 자리는 레이아웃 팝업이 갖는다', () => {
    renderDialog();
    expect(screen.queryByTestId('property-order-temperature')).not.toBeInTheDocument();
    expect(screen.getByTestId('property-override-temperature-button')).toBeInTheDocument();
    expect(screen.getByTestId('property-toggle-temperature')).toBeInTheDocument();
  });

  it('차례는 그룹 안에서 ↑↓ 로 바꾼다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['temperature', 'humidity'] },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-status-layout-button'));
    fireEvent.click(screen.getByTestId('properties-grid-status-mode-table'));
    fireEvent.click(screen.getByTestId('properties-grid-status-down-temperature'));
    expect(savedConfig().visibleProperties).toEqual(['humidity', 'temperature']);
  });
});

describe('항목별 세부 설정', () => {
  it('글자 설정은 그 항목에만 쌓인다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-override-temperature-value_font-size'), {
      target: { value: '28' },
    });
    const overrides = savedConfig().propertyOverrides as Record<string, { value_font?: unknown }>;
    expect(overrides.temperature!.value_font).toMatchObject({ size: 28 });
    expect(overrides.humidity).toBeUndefined();
  });

  it('값에 따른 색 규칙을 더하고 지운다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.click(screen.getByTestId('property-rule-temperature-add'));
    fireEvent.change(screen.getByTestId('property-rule-temperature-value-0'), {
      target: { value: '30' },
    });
    const overrides = savedConfig().propertyOverrides as Record<
      string,
      { valueColors?: { op: string; value: string }[] }
    >;
    expect(overrides.temperature!.valueColors).toEqual([
      expect.objectContaining({ op: 'gte', value: '30' }),
    ]);

    fireEvent.click(screen.getByTestId('property-rule-temperature-remove-0'));
    const after = savedConfig().propertyOverrides as Record<string, { valueColors?: unknown[] }>;
    expect(after.temperature!.valueColors).toEqual([]);
  });
});

describe('카드 배치 편집기', () => {
  /** 편집기는 팝업 안에 있다. 열고, jsdom 이 계산하지 않는 격자 크기를 심는다(3x3, 칸당 30px). */
  function stubGrid(): void {
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    const el = screen.getByTestId('card-layout-editor');
    el.getBoundingClientRect = () =>
      ({ left: 0, top: 0, right: 90, bottom: 90, width: 90, height: 90, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;
  }

  it('블록을 끌면 칸 단위로 옮겨진다', () => {
    renderDialog();
    stubGrid();
    // 값 조각은 기본 배치에서 2행 1열이다. 아래로 한 칸(30px), 오른쪽으로 한 칸.
    fireEvent.mouseDown(screen.getByTestId('card-block-value'), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: 30, clientY: 30 });
    fireEvent.mouseUp(window);
    const areas = savedConfig().cardAreas as Record<string, { row: number; col: number }>;
    expect(areas.value).toMatchObject({ row: 3, col: 2 });
  });

  it('칸의 절반을 넘지 않으면 제자리에 둔다 — 손떨림으로 배치가 바뀌면 안 된다', () => {
    renderDialog();
    stubGrid();
    fireEvent.mouseDown(screen.getByTestId('card-block-value'), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: 10, clientY: 10 });
    fireEvent.mouseUp(window);
    // 배치가 바뀌지 않았으므로 cardAreas 를 쓰지 않는다.
    expect(savedConfig().cardAreas).toBeUndefined();
  });

  it('모서리를 끌면 칸 수가 늘어난다', () => {
    renderDialog();
    stubGrid();
    fireEvent.mouseDown(screen.getByTestId('card-block-label-resize'), { button: 0, clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: 60, clientY: 0 });
    fireEvent.mouseUp(window);
    const areas = savedConfig().cardAreas as Record<string, { colSpan: number; row: number }>;
    // 항목명은 1행 1열에서 시작하므로 3열까지 늘어난다.
    expect(areas.label).toMatchObject({ row: 1, colSpan: 3 });
  });

  it('우클릭하면 그 조각의 글자 설정이 열린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.contextMenu(screen.getByTestId('card-block-time'));
    fireEvent.change(screen.getByTestId('card-block-time-font-size'), { target: { value: '9' } });
    expect(savedConfig().time_font).toMatchObject({ size: 9 });
  });

  it('우클릭 메뉴는 Esc 로 닫힌다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.contextMenu(screen.getByTestId('card-block-time'));
    expect(screen.getByTestId('card-block-time-design')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByTestId('card-block-time-design')).not.toBeInTheDocument();
  });
});

describe('항목별 카드 배치', () => {
  it('기본은 꺼짐 — 카드 전체 배치를 따른다', () => {
    renderDialog();
    openItemDesign('temperature');
    expect(screen.getByTestId('property-own-layout-temperature')).not.toBeChecked();
    expect(screen.queryByTestId('property-card-temperature')).not.toBeInTheDocument();
  });

  it('켜면 전체 배치를 복사해 시작한다 — 빈 화면에서 시작하면 방금 보이던 배치가 사라진 것처럼 보인다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.click(screen.getByTestId('property-own-layout-temperature'));
    const override = (savedConfig().propertyOverrides as Record<string, {
      cardRows?: number;
      cardCols?: number;
      cardAreas?: Record<string, unknown>;
    }>).temperature!;
    expect(override.cardRows).toBe(3);
    expect(override.cardCols).toBe(3);
    expect(override.cardAreas).toBeDefined();
  });

  it('끄면 따로 잡은 값을 지운다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { cardRows: 2, cardCols: 2, cardAreas: {} } },
      },
    };
    renderDialog();
    openItemDesign('temperature');
    fireEvent.click(screen.getByTestId('property-own-layout-temperature'));
    const override = (savedConfig().propertyOverrides as Record<string, {
      cardRows?: number;
      cardAreas?: unknown;
    }>).temperature!;
    expect(override.cardRows).toBeUndefined();
    expect(override.cardAreas).toBeUndefined();
  });

  it('켠 뒤에는 그 항목만의 배치 편집기가 나온다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.click(screen.getByTestId('property-own-layout-temperature'));
    expect(screen.getByTestId('property-card-temperature')).toBeInTheDocument();
    // 분할을 줄이면 그 항목에만 반영된다.
    fireEvent.change(screen.getByTestId('property-card-rows-temperature'), { target: { value: '2' } });
    const override = (savedConfig().propertyOverrides as Record<string, { cardRows?: number }>).temperature!;
    expect(override.cardRows).toBe(2);
  });
});

describe('공통 설정 자리와 정렬', () => {
  it('타일 설정이 표시 항목 목록보다 위에 있다 — 공통 설정이 개별 설정 아래면 덮는 방향이 뒤집혀 읽힌다', () => {
    renderDialog();
    const design = screen.getByTestId('properties-grid-card-button');
    const list = screen.getByTestId('properties-grid-status-select-all');
    // DOCUMENT_POSITION_FOLLOWING: design 다음에 list 가 온다.
    expect(design.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('카드 글자에 정렬을 고를 수 있다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.change(screen.getByTestId('property-value-font-align'), { target: { value: 'right' } });
    expect(savedConfig().value_font).toMatchObject({ align: 'right' });
  });

  it('항목별 설정에도 정렬이 있다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-override-temperature-value_font-align'), {
      target: { value: 'center' },
    });
    const overrides = savedConfig().propertyOverrides as Record<string, { value_font?: unknown }>;
    expect(overrides.temperature!.value_font).toMatchObject({ align: 'center' });
  });
});

describe('카드 설정 한 무리', () => {
  it('분할 · 배치 · 조각 글자가 한 팝업 안에 있다 — 종전에는 세 자리를 오가야 했다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));

    expect(screen.getByTestId('card-rows')).toBeInTheDocument();
    expect(screen.getByTestId('card-layout-editor')).toBeInTheDocument();
    expect(screen.getByTestId('property-value-font-size')).toBeInTheDocument();
  });

  it('갱신 시각 표시는 팝업 밖에 남는다 — 자주 켜고 끄는 것이다', () => {
    renderDialog();
    expect(screen.getByTestId('properties-grid-show-updated')).toBeInTheDocument();
  });

  it('배치는 그래픽과 표 중 하나만 보인다 — 둘을 같이 펼치면 같은 값이 두 벌이 된다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));

    // 기본은 그래픽.
    expect(screen.getByTestId('card-layout-editor')).toBeInTheDocument();
    expect(screen.queryByTestId('card-area-value-colSpan')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('card-layout-mode-table'));
    expect(screen.queryByTestId('card-layout-editor')).not.toBeInTheDocument();
    expect(screen.getByTestId('card-area-value-colSpan')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('card-layout-mode-graphic'));
    expect(screen.getByTestId('card-layout-editor')).toBeInTheDocument();
  });

  it('표에서 고친 배치가 그래픽 편집기에 그대로 나타난다 — 같은 값을 본다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.click(screen.getByTestId('card-layout-mode-table'));
    fireEvent.change(screen.getByTestId('card-area-value-colSpan'), { target: { value: '3' } });

    fireEvent.click(screen.getByTestId('card-layout-mode-graphic'));
    expect(screen.getByTestId('card-block-value')).toHaveStyle({ gridColumn: '1 / span 3' });
  });
})

describe('그룹별 표시 항목', () => {
  beforeEach(() => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['temperature', 'meta.name', 'gw.rssi'] },
    };
  });

  it('그룹마다 목록과 레이아웃을 따로 낸다', () => {
    renderDialog();
    for (const group of ['basic', 'status', 'gateway']) {
      expect(screen.getByTestId(`properties-grid-${group}-select-all`)).toBeInTheDocument();
      expect(screen.getByTestId(`properties-grid-${group}-layout-button`)).toBeInTheDocument();
    }
  });

  // 첫 업링크 전에 수신 정보가 사라지면 고를 수도, 자리를 잡아 둘 수도 없다.
  it('수신 정보는 데이터가 오기 전에도 고를 수 있다', () => {
    renderDialog();
    expect(screen.getByTestId('properties-grid-gateway-select-all')).toBeInTheDocument();
    expect(screen.getByTestId('property-toggle-gw.rssi')).toBeInTheDocument();
  });

  it('한 항목을 꺼도 다른 그룹은 그대로다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('property-toggle-temperature'));
    expect(savedConfig().visibleProperties).toEqual(['meta.name', 'gw.rssi']);
  });

  it('고른 것이 없는 그룹에는 레이아웃을 내지 않는다 — 놓을 자리가 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['temperature'] },
    };
    renderDialog();
    expect(screen.getByTestId('properties-grid-status-layout-button')).toBeInTheDocument();
    expect(screen.queryByTestId('properties-grid-basic-layout-button')).not.toBeInTheDocument();
  });

  it('열 수 입력은 없다 — 그룹마다 제 격자를 갖는다', () => {
    renderDialog();
    expect(screen.queryByTestId('properties-grid-cols')).not.toBeInTheDocument();
  });

  it('표시 목록이 비어 있어도 한 항목만 끄면 나머지는 남는다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1' },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('property-toggle-temperature'));
    // 비어 있음 = 전부. 실제 목록으로 펴 둔 뒤 하나만 빠져야 한다.
    expect(savedConfig().visibleProperties).toEqual(expect.arrayContaining(['humidity']));
    expect(savedConfig().visibleProperties).not.toContain('temperature');
  });
})

describe('타일 이름 바꾸기', () => {
  it('이름을 정하면 그 타일에만 쓰인다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-name-temperature'), {
      target: { value: '실내 온도' },
    });
    const overrides = savedConfig().propertyOverrides as Record<string, { label?: string }>;
    expect(overrides.temperature!.label).toBe('실내 온도');
    expect(overrides.humidity).toBeUndefined();
  });

  it('비우면 기본 이름으로 돌아간다 — 빈 이름을 그대로 쓰면 무엇을 보는 자리인지 사라진다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { label: '실내 온도' } },
      },
    };
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-name-temperature'), { target: { value: '' } });
    const overrides = savedConfig().propertyOverrides as Record<string, { label?: string }>;
    expect(overrides.temperature!.label).toBeUndefined();
  });

  it('정한 이름이 목록에도 나온다 — 두 곳이 다르면 어느 것이 그 타일인지 알 수 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { label: '실내 온도' } },
      },
    };
    renderDialog();
    // 목록과 미리보기 양쪽에 같은 이름이 나온다.
    expect(screen.getAllByText('실내 온도').length).toBeGreaterThan(0);
  });
})

describe('배경색과 초기화', () => {
  it('공통 속성 디자인에서 타일 배경색을 정하고 되돌린다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.change(screen.getByTestId('properties-grid-tile-bg'), { target: { value: '#112233' } });
    expect(savedConfig().tileBg).toBe('#112233');

    fireEvent.click(screen.getByTestId('properties-grid-tile-bg-reset'));
    expect(savedConfig().tileBg).toBeUndefined();
  });

  it('공통 속성 초기화는 분할·배치·글자·배경을 한꺼번에 되돌린다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        cardRows: 2,
        cardCols: 2,
        cardAreas: { label: { row: 1, col: 1, rowSpan: 1, colSpan: 1 } },
        value_font: { size: 30 },
        tileBg: '#112233',
      },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-card-button'));
    fireEvent.click(screen.getByTestId('properties-grid-card-reset'));

    const saved = savedConfig();
    for (const key of ['cardRows', 'cardCols', 'cardAreas', 'value_font', 'tileBg']) {
      expect(saved[key]).toBeUndefined();
    }
  });

  it('레이아웃 초기화는 그 그룹의 격자와 자리만 되돌린다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        statusGrid: { rows: 2, cols: 4 },
        statusAreas: { temperature: { x: 3, y: 1, w: 2, h: 2 } },
        basicGrid: { rows: 3, cols: 6 },
      },
    };
    renderDialog();
    fireEvent.click(screen.getByTestId('properties-grid-status-layout-button'));
    fireEvent.click(screen.getByTestId('properties-grid-status-reset'));

    const saved = savedConfig();
    expect(saved.statusGrid).toBeUndefined();
    expect(saved.statusAreas).toBeUndefined();
    // 다른 그룹은 건드리지 않는다.
    expect(saved.basicGrid).toMatchObject({ rows: 3, cols: 6 });
  });
})

describe('설정 자리', () => {
  it('배지 표시가 공통 속성보다 위에 있다', () => {
    renderDialog();
    const badges = screen.getByTestId('properties-grid-show-badges');
    const common = screen.getByTestId('properties-grid-card-button');
    // DOCUMENT_POSITION_FOLLOWING: badges 다음에 common 이 온다.
    expect(badges.compareDocumentPosition(common) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
})

describe('배지 항목 설정', () => {
  it('기본은 갱신 · 종류 · 프로토콜만 켜져 있다', () => {
    renderDialog();
    for (const badge of ['updated', 'kind', 'protocol']) {
      expect(screen.getByTestId(`device-badge-${badge}`)).toBeChecked();
    }
    expect(screen.getByTestId('device-badge-online')).not.toBeChecked();
  });

  it('항목마다 디자인을 정한다', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('device-badge-design-protocol-button'));
    fireEvent.change(screen.getByTestId('device-badge-protocol-value_font-size'), {
      target: { value: '16' },
    });
    const styles = savedConfig().badgeStyles as Record<string, { value_font?: { size?: number } }>;
    expect(styles.protocol!.value_font).toMatchObject({ size: 16 });
    expect(styles.kind).toBeUndefined();
  });

  it('배지를 끄면 항목 목록도 감춘다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['temperature'], showBadges: false },
    };
    renderDialog();
    expect(screen.queryByTestId('device-badge-protocol')).not.toBeInTheDocument();
  });
})

describe('단위 설정', () => {
  it('상태 정보 항목에는 단위 칸이 있다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-unit-temperature'), { target: { value: 'K' } });
    const overrides = savedConfig().propertyOverrides as Record<string, { unit?: string }>;
    expect(overrides.temperature!.unit).toBe('K');
  });

  it('수신 정보 항목에도 단위 칸이 있다 — 빈칸은 원래 단위를 알려 준다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['gw.rssi'] },
    };
    renderDialog();
    openItemDesign('gw.rssi');
    const input = screen.getByTestId('property-unit-gw.rssi') as HTMLInputElement;
    expect(input.placeholder).toBe('dBm');
  });

  it('기본 정보 항목에는 단위 칸이 없다 — 사람이 적어 둔 값에 붙일 단위가 없다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['meta.name'] },
    };
    renderDialog();
    openItemDesign('meta.name');
    expect(screen.queryByTestId('property-unit-meta.name')).not.toBeInTheDocument();
  });

  it('비우면 정하지 않은 것으로 돌아간다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: {
        deviceId: 'dev-1',
        visibleProperties: ['temperature'],
        propertyOverrides: { temperature: { unit: 'K' } },
      },
    };
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-unit-temperature'), { target: { value: '' } });
    const overrides = savedConfig().propertyOverrides as Record<string, { unit?: string }>;
    expect(overrides.temperature!.unit).toBeUndefined();
  });
})

describe('항목별 배경색 설정', () => {
  it('정하고 되돌린다', () => {
    renderDialog();
    openItemDesign('temperature');
    fireEvent.change(screen.getByTestId('property-bg-temperature'), {
      target: { value: '#331111' },
    });
    const overrides = savedConfig().propertyOverrides as Record<string, { bg?: string }>;
    expect(overrides.temperature!.bg).toBe('#331111');

    fireEvent.click(screen.getByTestId('property-bg-reset-temperature'));
    const after = savedConfig().propertyOverrides as Record<string, { bg?: string }>;
    expect(after.temperature!.bg).toBeUndefined();
  });

  it('기본 정보 항목에도 있다 — 배경은 값의 성격과 무관하다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'properties-grid',
      title: '디바이스 현황',
      config: { deviceId: 'dev-1', visibleProperties: ['meta.name'] },
    };
    renderDialog();
    openItemDesign('meta.name');
    expect(screen.getByTestId('property-bg-meta.name')).toBeInTheDocument();
  });
})
