// 속성 그리드의 measurements 전개 렌더 테스트.
//
// chirpstack 로스터는 measurements 를 { name: {value, time_ms} } 중첩 객체로 내보낸다.
// 이를 하나의 "[object Object]" 카드가 아니라 측정치별 카드로 렌더해야 한다.

import { render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { describe, expect, it } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import { StatePropertiesSection } from './DeviceDetailPanel';

function renderWithI18n(ui: ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

/** 그리드 카드의 라벨 텍스트 목록. */
function cardLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.grid > div')).map(
    (el) => el.querySelector('p')?.textContent ?? '',
  );
}

describe('StatePropertiesSection measurements 전개', () => {
  const now = Date.now();

  it('measurements 를 측정치별 카드로 렌더하고 "[object Object]" 를 표시하지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{
          measurements: {
            temperature: { value: 29.8, time_ms: now - 60_000 },
            humidity: { value: 55.2, time_ms: now - 60_000 },
          },
          rssi: -57,
          gateway_id: '24e124fffef79304',
        }}
        protocol="chirpstack"
        type="sensor"
      />,
    );

    expect(container.textContent).not.toContain('[object Object]');
    // 측정치별 값이 개별 표시되며 온도 단위가 유지된다.
    expect(screen.getByText('29.8°C')).toBeInTheDocument();
    expect(screen.getByText('55.2')).toBeInTheDocument();
    // 라벨은 측정치 이름 기준(온도/습도는 COMMON_LABELS 로 한국어 변환).
    const labels = cardLabels(container);
    expect(labels).toContain('온도');
    expect(labels).toContain('습도');
    // 불투명한 measurements 카드 자체는 남아있지 않다.
    expect(labels).not.toContain('Measurements');
    // 나머지 속성은 그대로 유지된다.
    expect(screen.getByText('-57')).toBeInTheDocument();
    expect(screen.getByText('24e124fffef79304')).toBeInTheDocument();
  });

  it('측정치별 갱신 시각을 상대 시간으로 함께 표시한다', () => {
    renderWithI18n(
      <StatePropertiesSection
        properties={{ measurements: { temperature: { value: 29.8, time_ms: now - 3 * 60_000 } } }}
        protocol="chirpstack"
        type="sensor"
      />,
    );
    expect(screen.getByText('3분 전')).toBeInTheDocument();
  });

  it('measurements 가 비어 있으면 카드를 만들지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection properties={{ measurements: {}, rssi: -57 }} protocol="chirpstack" type="sensor" />,
    );
    expect(container.textContent).not.toContain('[object Object]');
    // rssi 카드 하나만 남는다(humanizeKey('rssi') === 'Rssi').
    expect(cardLabels(container)).toEqual(['Rssi']);
  });

  it('하위 값이 {value,time_ms} 가 아닌 스칼라여도 깨지지 않는다', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection
        properties={{ measurements: { temperature: 29.8 } }}
        protocol="chirpstack"
        type="sensor"
      />,
    );
    expect(container.textContent).not.toContain('[object Object]');
    expect(screen.getByText('29.8°C')).toBeInTheDocument();
  });

  it('measurements 가 없어도 기존 렌더가 유지된다(회귀 방지)', () => {
    const { container } = renderWithI18n(
      <StatePropertiesSection properties={{ rssi: -57 }} protocol="chirpstack" type="sensor" />,
    );
    expect(container.textContent).not.toContain('[object Object]');
    expect(screen.getByText('-57')).toBeInTheDocument();
  });
});
