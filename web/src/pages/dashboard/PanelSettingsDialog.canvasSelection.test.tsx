// 설정 다이얼로그의 캔버스 배선 (SPEC-CANVAS-002 T11 · AC-06 · AC-07).
//
// 다이얼로그는 이 SPEC 에서 **감싸는 줄과 `forceEdit` prop 만** 얻는다(§위험 R5 — 8,300행
// 짜리 파일을 더 키우지 않는다). 그래서 여기서 재는 것도 그 두 가지뿐이다:
//
//   1. 미리보기와 요소 목록 편집기가 **한 선택**을 나눠 쓴다. 둘은 `PanelSettingsShell` 의
//      서로 다른 슬롯에 마운트되므로, 감싸기가 빠지면 각자 로컬 선택으로 떨어져 캔버스에서
//      고른 것이 목록에 아무 영향도 주지 못한다 — 그리고 그 실패는 **조용하다**
//      (컨텍스트 기본값이 `null` 이고 소비 훅이 로컬로 폴백하기 때문이다).
//   2. 미리보기는 `forced` 로 **항상 편집**이다(AC-07) — 그래서 팔레트와 손잡이가 뜬다.
//
// 관측 지점으로 **팔레트**를 고른 것에 뜻이 있다: 팔레트 누름은 좌표가 필요 없고(버튼
// 클릭이다) 한 번에 두 가지를 건넌다 — 요소가 draft config 로 흘러 목록에 나타나고,
// 선택이 컨텍스트로 흘러 그 행을 펼친다. 캔버스 도형을 좌표로 누르는 경로는 jsdom 이
// 레이아웃을 하지 않아 스테이지가 0×0 이므로 여기서 잴 것이 못 된다(그 계약은
// `CanvasEditOverlay.test.tsx` 가 스테이지를 심어 이미 잰다).
//
// @spec SPEC-CANVAS-002 REQ-04

import { describe, expect, it, vi, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClientProvider } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';

/** 캔버스 요소 하나를 실은 패널. 요소가 0개면 패널이 빈 상태를 그려 표면이 뜨지 않는다. */
const storeMock = vi.hoisted(() => ({
  panel: {
    id: 'p1',
    type: 'canvas',
    title: 'T',
    config: {
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }, style: {} },
      ],
    },
  } as never,
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [
      {
        id: 'd',
        name: 'x',
        isDefault: true,
        panels: [storeMock.panel],
        layout: [{ i: 'p1', x: 0, y: 0, w: 6, h: 4 }],
      },
    ],
    updatePanelConfig: vi.fn(),
    updatePanelTitle: vi.fn(),
    setDashboardLayout: vi.fn(),
    dashboardGridCols: 12,
    dashboardGridWidth: 1376,
    dashboardShowGridLines: false,
    dashboardRefreshInterval: 5,
    // 편집모드가 **꺼져 있어도** 미리보기는 `forced` 로 편집이다(AC-07).
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (sel: (s: unknown) => unknown) => sel(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import PanelSettingsDialog from './PanelSettingsDialog';

afterEach(cleanup);

function renderDialog(): void {
  render(
    <MemoryRouter>
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

/** 행이 펼쳐져 있는가. */
function isRowOpen(idx: number): boolean {
  return (
    screen.getByTestId(`canvas-element-toggle-${idx}`).getAttribute('aria-expanded') === 'true'
  );
}

describe('PanelSettingsDialog — 캔버스 미리보기와 목록 편집기가 한 선택을 나눠 쓴다', () => {
  it('미리보기와 요소 목록 편집기가 함께 뜬다', () => {
    renderDialog();
    expect(screen.getByTestId('canvas-preview-wrapper')).toBeTruthy();
    expect(screen.getByTestId('canvas-elements-editor')).toBeTruthy();
  });

  it('미리보기는 forced 로 항상 편집이라 도크가 뜨고 토글 버튼은 감춰진다 (AC-07)', () => {
    renderDialog();
    // 대시보드 편집모드는 꺼져 있는데도 도구가 있다 — `forceEdit` 가 세 겹 게이팅의
    // 셋째 겹이기 때문이다.
    expect(screen.getByTestId('canvas-dock-panel')).toBeTruthy();
    // `forced` 인 자리에서는 배치 편집 토글을 내지 않는다.
    expect(screen.queryByTestId('canvas-edit-toggle')).toBeNull();
  });

  it('도구는 축소되는 미리보기 **바깥**에 선다 — 미리보기가 대시보드와 같아야 한다', () => {
    // 도크를 패널 안에 두면 패널 자신의 레이아웃이 달라져 미리보기가 대시보드와 다른
    // 화면이 되고, 캔버스 스테이지도 도크 폭만큼 좁아진다.
    renderDialog();
    const stage = screen.getByTestId('preview-stage');
    const dock = screen.getByTestId('canvas-dock');
    expect(stage.contains(dock)).toBe(false);
    expect(dock.contains(screen.getByTestId('canvas-dock-panel'))).toBe(true);
    // 스테이지 위에 떠 있던 띠는 남아 있지 않다.
    expect(screen.queryByTestId('canvas-palette')).toBeNull();
  });

  it('미리보기 팔레트로 놓은 요소가 목록에 나타나고 **그 행이 펼쳐진다**', () => {
    renderDialog();
    expect(screen.getAllByTestId(/^canvas-element-\d+$/)).toHaveLength(1);

    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));

    // (1) 요소가 draft config 를 지나 목록까지 왔다.
    const rows = screen.getAllByTestId(/^canvas-element-\d+$/);
    expect(rows).toHaveLength(2);
    expect(rows[1]!.getAttribute('data-element-id')).toBe('el-1');

    // (2) 선택이 컨텍스트를 지나 목록까지 왔다 — 감싸기가 빠지면 여기서 조용히 실패한다.
    expect(isRowOpen(1)).toBe(true);
    expect(rows[1]!.getAttribute('data-selected')).toBe('true');
    // 캔버스가 펼친 행은 하나뿐이다 — 원래 있던 행은 접힌 채다.
    expect(isRowOpen(0)).toBe(false);
  });

  it('이어서 다른 것을 놓으면 이전 행이 도로 접힌다 (AC-06)', () => {
    renderDialog();
    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    fireEvent.click(screen.getByTestId('canvas-palette-add-text'));

    expect(screen.getAllByTestId(/^canvas-element-\d+$/)).toHaveLength(3);
    expect([0, 1, 2].map(isRowOpen)).toEqual([false, false, true]);
  });

  it('목록 하단의 추가 버튼은 사라졌다 — 도형을 만드는 자리는 팔레트 하나다', () => {
    // 팔레트가 도크로 옮겨 이름과 누를 면적을 갖춘 뒤로, 같은 함수를 부르는 입구가
    // 한 화면에 둘일 이유가 없어졌다. 위 시험들이 그 하나가 실제로 만드는 것을 잰다.
    renderDialog();
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.queryByTestId(`canvas-element-add-${kind}`)).toBeNull();
      expect(screen.getByTestId(`canvas-palette-add-${kind}`)).toBeTruthy();
    }
  });
});
