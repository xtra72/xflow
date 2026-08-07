// FloorPlanBackground 렌더 테스트 (SPEC-HEATMAP-PANEL-002 T3).
// 이미지 미첨부 시 null(AC-E1), 첨부 시 object-fit 배경 렌더를 커버한다.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import FloorPlanBackground from './FloorPlanBackground';

describe('FloorPlanBackground', () => {
  it('AC-E1: image 가 없으면 아무 것도 렌더하지 않는다(null)', () => {
    const { container } = render(<FloorPlanBackground />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByTestId('floor-plan-background')).toBeNull();
  });

  it('image 가 있으면 data-URL 배경 이미지를 기본 fit(contain)으로 렌더한다', () => {
    render(<FloorPlanBackground image="data:image/png;base64,AAAA" />);
    const img = screen.getByTestId('floor-plan-background');
    expect(img).toBeInTheDocument();
    expect(img.getAttribute('src')).toBe('data:image/png;base64,AAAA');
    expect((img as HTMLElement).style.objectFit).toBe('contain');
  });

  it('fit=cover 를 object-fit 으로 반영한다', () => {
    render(<FloorPlanBackground image="data:image/png;base64,BBBB" fit="cover" />);
    const img = screen.getByTestId('floor-plan-background');
    expect((img as HTMLElement).style.objectFit).toBe('cover');
  });
});
