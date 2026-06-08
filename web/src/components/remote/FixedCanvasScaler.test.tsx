// FixedCanvasScaler 테스트 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M07/M08, OQ-M4).
//
// 범위:
//   - computeFitScale: 종횡비 보존 레터박스 배율 계산(동일 종횡비/와이드/톨/0 방어).
//   - 렌더: 고정 캔버스 크기 = 노드 해상도(width×height px), 자식 렌더, 폴백.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { computeFitScale, FixedCanvasScaler } from './FixedCanvasScaler';

describe('computeFitScale — 레터박스 배율(REQ-M08)', () => {
  it('가용 영역과 캔버스 종횡비가 같으면 단순 비율을 반환한다', () => {
    // 1920x1080 캔버스를 960x540 영역에 → 0.5.
    expect(computeFitScale(960, 540, 1920, 1080)).toBeCloseTo(0.5);
  });

  it('가용 영역이 더 넓으면(와이드) 높이로 제한해 필러박스 여백을 만든다', () => {
    // 1920x1080 캔버스, 가용 3840x1080 → min(2, 1)=1 (좌우 여백).
    expect(computeFitScale(3840, 1080, 1920, 1080)).toBeCloseTo(1);
  });

  it('가용 영역이 더 높으면(톨) 폭으로 제한해 레터박스 여백을 만든다', () => {
    // 1920x1080 캔버스, 가용 1920x4320 → min(1, 4)=1 (상하 여백).
    expect(computeFitScale(1920, 4320, 1920, 1080)).toBeCloseTo(1);
  });

  it('가용 영역이 작으면 축소 배율을 반환한다(fit)', () => {
    // 1920x1080 캔버스, 가용 1920x540 → min(1, 0.5)=0.5.
    expect(computeFitScale(1920, 540, 1920, 1080)).toBeCloseTo(0.5);
  });

  it('캔버스 크기가 0/음수면 1 을 반환한다(방어)', () => {
    expect(computeFitScale(800, 600, 0, 1080)).toBe(1);
    expect(computeFitScale(800, 600, 1920, 0)).toBe(1);
    expect(computeFitScale(800, 600, -1, -1)).toBe(1);
  });

  it('가용 영역이 0/음수면 1 을 반환한다(측정 전)', () => {
    expect(computeFitScale(0, 0, 1920, 1080)).toBe(1);
    expect(computeFitScale(-1, 600, 1920, 1080)).toBe(1);
  });

  it('crop/stretch 없이 비례를 유지한다(양 축 동일 배율)', () => {
    // 동일 배율(min) 을 양 축에 적용하므로 가로/세로 비율이 보존된다.
    const scale = computeFitScale(800, 1200, 1920, 1080);
    // 가용이 톨이므로 폭(800/1920) 으로 제한.
    expect(scale).toBeCloseTo(800 / 1920);
  });
});

describe('FixedCanvasScaler — 렌더(REQ-M07)', () => {
  it('자식을 노드 해상도(width×height) 고정 캔버스에 렌더한다', () => {
    render(
      <FixedCanvasScaler width={1920} height={1080}>
        <div data-testid="child">panel</div>
      </FixedCanvasScaler>,
    );

    const canvas = screen.getByTestId('fixed-canvas');
    // 고정 캔버스 크기 = 노드 해상도(px).
    expect(canvas).toHaveStyle({ width: '1920px', height: '1080px' });
    expect(canvas).toHaveAttribute('data-canvas-width', '1920');
    expect(canvas).toHaveAttribute('data-canvas-height', '1080');
    // 자식(패널)은 캔버스 안에 그대로 렌더된다(리플로우 없음).
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });

  it('폴백 해상도(1920×1080)를 캔버스 크기로 사용한다', () => {
    // 호출자가 미보고(0) 시 폴백을 결정한다 — 여기서는 폴백값을 직접 전달.
    render(
      <FixedCanvasScaler width={1920} height={1080}>
        <div data-testid="child" />
      </FixedCanvasScaler>,
    );
    const canvas = screen.getByTestId('fixed-canvas');
    expect(canvas).toHaveStyle({ width: '1920px', height: '1080px' });
  });

  it('비표준 해상도(예: 1080×1920 세로형)도 그대로 적용한다', () => {
    render(
      <FixedCanvasScaler width={1080} height={1920}>
        <div data-testid="child" />
      </FixedCanvasScaler>,
    );
    const canvas = screen.getByTestId('fixed-canvas');
    expect(canvas).toHaveAttribute('data-canvas-width', '1080');
    expect(canvas).toHaveAttribute('data-canvas-height', '1920');
  });
});
