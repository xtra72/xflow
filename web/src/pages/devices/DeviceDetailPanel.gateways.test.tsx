// 디바이스 상세 — 게이트웨이 링크 전용 섹션 렌더 테스트.
//
// chirpstack 디바이스 로스터는 이 디바이스를 수신한 모든 게이트웨이를
// state.properties.gateways[] 로 내보낸다. 이는 일반 key/value 속성이 아니라
// (디바이스, 게이트웨이) 쌍의 목록이므로 속성 그리드가 아닌 전용 섹션이 렌더해야 하고,
// 그리드에서는 제외되어야 한다(제외하지 않으면 객체 폴백으로 JSON 덩어리가 표시된다).

import { render, screen, within } from '@testing-library/react';
import type { ReactElement } from 'react';
import { describe, expect, it } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import { StatePropertiesSection } from './DeviceDetailPanel';

function renderWithI18n(ui: ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

/** 그리드 카드의 라벨 텍스트 목록(전용 섹션의 표는 포함되지 않는다). */
function cardLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.grid > div')).map(
    (el) => el.querySelector('p')?.textContent ?? '',
  );
}

/** 게이트웨이 섹션 표의 본문 행. */
function gatewayRows(container: HTMLElement): HTMLElement[] {
  const table = container.querySelector('table');
  if (!table) return [];
  return Array.from(table.querySelectorAll('tbody > tr'));
}

/** n 번째 게이트웨이 행. 없으면 실패시킨다(noUncheckedIndexedAccess 대응). */
function gatewayRow(container: HTMLElement, index: number): HTMLElement {
  const row = gatewayRows(container)[index];
  if (!row) throw new Error(`게이트웨이 행 ${index} 이 없습니다`);
  return row;
}

/** 행의 n 번째 셀. 없으면 실패시킨다. */
function cellAt(row: HTMLElement, index: number): HTMLElement {
  const cell = within(row).getAllByRole('cell')[index];
  if (!cell) throw new Error(`셀 ${index} 이 없습니다`);
  return cell;
}

const now = Date.now();

// 백엔드 관측 응답 형태. gateway_id 오름차순이며, 앞선 게이트웨이가 신호는 더 약하다
// (정렬이 신호 세기를 따르지 않는다는 사실을 드러내려는 의도적 배치).
const GW_WEAK = {
  gateway_id: '24e124fffef5dccc',
  rssi: -113,
  snr: -9.5,
  channel: 7,
  frequency_hz: 922100000,
  spreading_factor: 7,
  bandwidth: 125000,
  last_seen_ms: now - 3 * 60_000,
  stale: false,
};

const GW_STRONG = {
  gateway_id: '24e124fffef79304',
  rssi: -57,
  snr: 13.5,
  channel: 2,
  frequency_hz: 922100000,
  spreading_factor: 7,
  bandwidth: 125000,
  last_seen_ms: now - 3 * 60_000,
  stale: false,
};

const TWO_GATEWAYS = [GW_WEAK, GW_STRONG];

describe('StatePropertiesSection 게이트웨이 섹션', () => {
  it('게이트웨이를 전용 섹션으로 렌더하고 요청된 5개 필드를 모두 표시한다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{ gateways: [GW_STRONG] }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    // 섹션 제목 + 컬럼 헤더(게이트웨이 ID / RSSI / SNR / 채널 / 마지막 수신).
    expect(screen.getByText('이 디바이스를 수신한 게이트웨이')).toBeInTheDocument();
    expect(screen.getByText('게이트웨이 ID')).toBeInTheDocument();
    expect(screen.getByText('RSSI (dBm)')).toBeInTheDocument();
    expect(screen.getByText('SNR (dB)')).toBeInTheDocument();
    // 채널 라벨은 에이전트 게이트웨이 탭과 동일해야 한다(같은 값의 표기 불일치 방지).
    expect(screen.getByText('게이트웨이 IF 채널')).toBeInTheDocument();
    expect(screen.getByText('마지막 수신')).toBeInTheDocument();

    expect(gatewayRows(container)).toHaveLength(1);
    const row = gatewayRow(container, 0);
    expect(cellAt(row, 0)).toHaveTextContent('24e124fffef79304');
    expect(cellAt(row, 1)).toHaveTextContent('-57');
    expect(cellAt(row, 2)).toHaveTextContent('13.5');
    expect(cellAt(row, 3)).toHaveTextContent('2');
    // 마지막 수신은 상대 시간으로, 정확한 시각은 title 로 제공한다.
    expect(cellAt(row, 5)).toHaveTextContent('3분 전');
    expect(cellAt(row, 5).getAttribute('title')).toBeTruthy();
  });

  it('게이트웨이가 여러 건이면 각각 자기 rssi/snr/channel 로 렌더한다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{ gateways: TWO_GATEWAYS }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    expect(gatewayRows(container)).toHaveLength(2);

    // 백엔드가 준 gateway_id 오름차순을 유지한다(신호 세기로 재정렬하지 않는다).
    const first = gatewayRow(container, 0);
    expect(cellAt(first, 0)).toHaveTextContent('24e124fffef5dccc');
    expect(cellAt(first, 1)).toHaveTextContent('-113');
    expect(cellAt(first, 2)).toHaveTextContent('-9.5');
    expect(cellAt(first, 3)).toHaveTextContent('7');

    const second = gatewayRow(container, 1);
    expect(cellAt(second, 0)).toHaveTextContent('24e124fffef79304');
    expect(cellAt(second, 1)).toHaveTextContent('-57');
    expect(cellAt(second, 2)).toHaveTextContent('13.5');
    expect(cellAt(second, 3)).toHaveTextContent('2');

    // 정렬을 흔들지 않고 최고 신호 링크를 배지로 표시한다.
    expect(within(second).getByText('최고 신호')).toBeInTheDocument();
    expect(within(first).queryByText('최고 신호')).not.toBeInTheDocument();
  });

  it('gateways 는 일반 속성 그리드에 표시되지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{ gateways: TWO_GATEWAYS, battery: 87 }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    // 객체 폴백(JSON 덩어리)이 나오지 않는다.
    expect(container.textContent).not.toContain('[object Object]');
    expect(container.textContent).not.toContain('"gateway_id"');
    // 그리드에는 gateways 카드가 없고 나머지 속성만 남는다.
    expect(cardLabels(container)).toEqual(['Battery']);
  });

  it('gateways 키가 없으면 섹션 자체를 렌더하지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection properties={{ battery: 87 }} protocol="chirpstack" type="sensor" />,
    );

    expect(screen.queryByText('이 디바이스를 수신한 게이트웨이')).not.toBeInTheDocument();
    expect(container.querySelector('table')).toBeNull();
  });

  it('gateways 가 빈 배열이어도 빈 껍데기 섹션을 만들지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection properties={{ gateways: [] }} protocol="chirpstack" type="sensor" />,
    );

    expect(screen.queryByText('이 디바이스를 수신한 게이트웨이')).not.toBeInTheDocument();
    expect(container.querySelector('table')).toBeNull();
  });

  it('stale 링크를 흐림 + 배지로 시각적으로 구분한다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{
          gateways: [
            { ...GW_WEAK, stale: true },
            { ...GW_STRONG, stale: false },
          ],
        }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    const staleRow = gatewayRow(container, 0);
    const liveRow = gatewayRow(container, 1);
    expect(staleRow.className).toContain('opacity-60');
    expect(liveRow.className).not.toContain('opacity-60');
    // 흐림만으로는 신호가 약하므로 텍스트 단서를 함께 준다.
    expect(within(staleRow).getByText('오래됨')).toBeInTheDocument();
    expect(within(liveRow).queryByText('오래됨')).not.toBeInTheDocument();
  });

  it('frequency_hz 가 0 이면 오류가 아니라 "-" 로 표시한다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{ gateways: [{ ...GW_STRONG, channel: 0, frequency_hz: 0 }] }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    const row = gatewayRow(container, 0);
    // channel 0 은 "없음"과 "실제 0" 을 구분하지 않는 설계값이라 그대로 0 을 표시한다.
    expect(cellAt(row, 3)).toHaveTextContent('0');
    // 주파수 0 은 물리적으로 불가능하므로 '-' 로 표시한다(게이트웨이 탭과 동일).
    expect(cellAt(row, 4)).toHaveTextContent('-');
  });

  it('measurements 와 gateways 가 함께 와도 각자 자기 섹션으로 렌더한다(회귀 방지)', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{
          gateways: TWO_GATEWAYS,
          measurements: { temperature: { value: 21.5, time_ms: now - 60_000 } },
        }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    expect(container.textContent).not.toContain('[object Object]');
    // measurements 는 종전대로 측정치별 카드로 펼쳐진다.
    expect(cardLabels(container)).toEqual(['온도']);
    expect(screen.getByText('21.5°C')).toBeInTheDocument();
    // gateways 는 표로 분리된다.
    expect(gatewayRows(container)).toHaveLength(2);
  });
});
