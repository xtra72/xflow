// 캔버스 패널 등록 배선 테스트 (SPEC-CANVAS-001 REQ-01 / T3).
//
// sysMetricsWiring.test.ts 와 같은 취지다: 카탈로그·기본 config·기본 크기·i18n 이 서로
// 어긋나면 패널은 "추가는 되는데 화면이 비거나 원문 키가 뜨는" 상태가 되고, 어느 지점이
// 끊겼는지 화면만 봐서는 알 수 없다. 여기서 지점별로 잠근다.

import { describe, expect, it } from 'vitest';

import { useUIStore, type PanelType } from '@/stores/uiStore';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import { buildDefaultCanvasConfig, parseCanvasConfig } from './canvasConfig';

/**
 * 패널을 추가하고 그 config 와 레이아웃 항목을 돌려준다.
 *
 * `createDefaultPanel` / `PANEL_DEFAULT_SIZES` 는 모듈 비공개다. 테스트를 위해 내보내면
 * 프로덕션 API 가 넓어지므로 기존 uiStore 테스트와 같이 스토어 액션으로 간접 관측한다.
 */
function addAndRead(type: PanelType) {
  useUIStore.setState({
    dashboardPages: [{ id: 'p', name: 'p', isDefault: true, panels: [], layout: [] }],
    activeDashboardId: 'p',
  });
  useUIStore.getState().addPanel(type);
  const s = useUIStore.getState();
  const page = s.dashboardPages.find((p) => p.id === s.activeDashboardId)!;
  const panel = page.panels[page.panels.length - 1]!;
  return { panel, layout: page.layout.find((l) => l.i === panel.id)! };
}

/** 점 표기법 키를 번역 트리에서 조회한다(i18nKeyShape.test.ts 와 같은 방식). */
function resolveKey(tree: unknown, key: string): unknown {
  return key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
      tree,
    );
}

describe('캔버스 패널 등록', () => {
  it('기본 config 를 parseCanvasConfig 가 손실 없이 받아들인다', () => {
    // 빌더와 파서가 갈라지면 "방금 만든 패널을 다시 읽었더니 설정이 달라져 있다" 가 된다.
    // 왕복이 항등이어야 그 갈라짐이 없다.
    const { panel } = addAndRead('canvas');
    expect(parseCanvasConfig(panel.config)).toEqual(buildDefaultCanvasConfig());
  });

  it('기본 config 는 요소 0개로 시작한다', () => {
    // 요소 0개는 오류가 아니라 안내 상태다(REQ-05 / AC-E1).
    const { panel } = addAndRead('canvas');
    expect(parseCanvasConfig(panel.config).elements).toEqual([]);
  });

  it('기본 제목이 비어 있지 않다', () => {
    expect(addAndRead('canvas').panel.title).not.toBe('');
  });

  it('기본 크기가 표에 실려 있다 (일괄 폴백으로 태어나지 않는다)', () => {
    // SPEC REQ-01 권장값. 폴백(5×4)으로 태어나면 여기서 어긋난다.
    const { layout } = addAndRead('canvas');
    expect(layout).toMatchObject({ w: 6, h: 4, minW: 3, minH: 2 });
  });

  it('카탈로그 i18n 키가 ko·en 양쪽에서 문자열로 해석된다', () => {
    // 네임스페이스 객체나 결측이면 화면에 원문 키가 그대로 뜬다.
    for (const key of [
      'dashboard.addPanel.labels.canvas',
      'dashboard.addPanel.descriptions.canvas',
    ]) {
      for (const [name, tree] of [['ko', ko], ['en', en]] as const) {
        const value = resolveKey(tree, key);
        expect(typeof value, `${name}: ${key}`).toBe('string');
        expect(value, `${name}: ${key}`).not.toBe('');
      }
    }
  });
});
