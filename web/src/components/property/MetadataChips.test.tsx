// MetadataChips 단위 테스트.
//
// SPEC-STORE-003 v0.3.0 메타데이터 (data_type, metric_type, registration) 의
// 칩 표시 동작을 검증한다.
//
// @spec SPEC-WEB-005 v0.7.0 (M16)

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { MetadataChips } from './MetadataChips';

// i18n: 실제 ko 번역을 반환하는 mock — 컴포넌트가 useTranslation 을 쓰지만
// 이 테스트는 I18nProvider 로 감싸지 않으므로, ko.json 을 점 표기 키로 해석해
// 기존 한국어 단언을 그대로 통과시킨다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

describe('MetadataChips', () => {
  it('모든 prop 미지정 시 컨테이너만 렌더된다 (자식 없음)', () => {
    render(<MetadataChips />);
    const wrapper = screen.getByTestId('metadata-chips');
    expect(wrapper).toBeInTheDocument();
    // 어떠한 메타데이터 칩도 렌더되지 않는다.
    expect(screen.queryByTestId('metadata-data-type')).toBeNull();
    expect(screen.queryByTestId('metadata-metric-type')).toBeNull();
    expect(screen.queryByTestId('metadata-metric-type-unknown')).toBeNull();
    expect(screen.queryByTestId('metadata-registration-auto')).toBeNull();
    expect(screen.queryByTestId('metadata-registration-manual')).toBeNull();
  });

  it('dataType 만 지정 시 데이터 타입 칩만 노출된다', () => {
    render(<MetadataChips dataType="float" />);
    const chip = screen.getByTestId('metadata-data-type');
    expect(chip).toHaveTextContent('float');
    // 색상 클래스 검증 (float = green).
    expect(chip.className).toMatch(/bg-green-100/);
  });

  it('데이터 타입 별 색상 클래스가 다르게 적용된다', () => {
    const { rerender } = render(<MetadataChips dataType="int" />);
    expect(screen.getByTestId('metadata-data-type').className).toMatch(/bg-blue-100/);

    rerender(<MetadataChips dataType="boolean" />);
    expect(screen.getByTestId('metadata-data-type').className).toMatch(
      /bg-purple-100/,
    );

    rerender(<MetadataChips dataType="bytes" />);
    expect(screen.getByTestId('metadata-data-type').className).toMatch(
      /bg-orange-100/,
    );

    rerender(<MetadataChips dataType="json" />);
    expect(screen.getByTestId('metadata-data-type').className).toMatch(/bg-red-100/);

    rerender(<MetadataChips dataType="string" />);
    expect(screen.getByTestId('metadata-data-type').className).toMatch(/bg-gray-100/);
  });

  it('metricType 이 지정되면 메트릭 칩이 노출된다', () => {
    render(<MetadataChips metricType="temperature" />);
    const chip = screen.getByTestId('metadata-metric-type');
    expect(chip).toHaveTextContent('temperature');
    // unknown 변종은 노출되지 않는다.
    expect(screen.queryByTestId('metadata-metric-type-unknown')).toBeNull();
  });

  it('metricType=unknown 은 별도 muted 칩으로 표시된다', () => {
    render(<MetadataChips metricType="unknown" />);
    const muted = screen.getByTestId('metadata-metric-type-unknown');
    expect(muted).toHaveTextContent('unknown');
    expect(muted.className).toMatch(/opacity-70/);
    // 일반 metric_type 칩은 노출되지 않는다.
    expect(screen.queryByTestId('metadata-metric-type')).toBeNull();
  });

  it('showAutoBadge=false (기본) 면 registration 배지가 표시되지 않는다', () => {
    render(<MetadataChips registration="auto" />);
    expect(screen.queryByTestId('metadata-registration-auto')).toBeNull();
    expect(screen.queryByTestId('metadata-registration-manual')).toBeNull();
  });

  it('showAutoBadge=true + registration=auto → auto 배지 노출', () => {
    render(<MetadataChips registration="auto" showAutoBadge />);
    const badge = screen.getByTestId('metadata-registration-auto');
    expect(badge).toHaveTextContent('auto');
    // 접근성: aria-label 또는 title 제공.
    expect(badge.getAttribute('aria-label')).toBe('자동 등록');
  });

  it('showAutoBadge=true + registration=manual → manual 배지 노출', () => {
    render(<MetadataChips registration="manual" showAutoBadge />);
    const badge = screen.getByTestId('metadata-registration-manual');
    expect(badge).toHaveTextContent('manual');
    expect(badge.getAttribute('aria-label')).toBe('수동 등록');
  });

  it('전체 메타데이터를 한 번에 표시할 수 있다', () => {
    render(
      <MetadataChips
        dataType="float"
        metricType="temperature"
        registration="manual"
        showAutoBadge
      />,
    );
    expect(screen.getByTestId('metadata-data-type')).toHaveTextContent('float');
    expect(screen.getByTestId('metadata-metric-type')).toHaveTextContent(
      'temperature',
    );
    expect(screen.getByTestId('metadata-registration-manual')).toHaveTextContent(
      'manual',
    );
  });

  it('className prop 이 wrapper 에 전달된다', () => {
    render(<MetadataChips className="ml-4 custom-test-cls" dataType="int" />);
    const wrapper = screen.getByTestId('metadata-chips');
    expect(wrapper.className).toMatch(/ml-4/);
    expect(wrapper.className).toMatch(/custom-test-cls/);
  });

  it('칩에 title 속성이 부여되어 호버 툴팁을 제공한다', () => {
    render(
      <MetadataChips
        dataType="bytes"
        metricType="counter"
        registration="auto"
        showAutoBadge
      />,
    );
    expect(screen.getByTestId('metadata-data-type').getAttribute('title')).toBe(
      '데이터 타입: bytes',
    );
    expect(screen.getByTestId('metadata-metric-type').getAttribute('title')).toBe(
      '메트릭 타입: counter',
    );
    expect(
      screen.getByTestId('metadata-registration-auto').getAttribute('title'),
    ).toBe('런타임에 자동 등록된 키');
  });
});
