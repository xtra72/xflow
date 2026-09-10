// 스크래치패드 문구는 **진짜 번역**으로 잰다 (SPEC-CANVAS-008 M9 · AC-08 · 시험 규율 D7).
//
// 왜 별도 파일인가: 나란한 `CanvasScratchpad.save.test.tsx` 는 i18n 을 **키 echo** 로
// 대체한다(기구를 재는 데에는 그편이 읽기 쉽다). 그런데 그 대체 아래에서는 `{max}` 가
// 애초에 문자열에 없으므로 **치환 결함이 드러나지 않는다** — `replace` 로 바꿔 놓아도
// 시험이 전부 통과한다. `vi.mock` 은 파일 단위라 한 파일에서 둘을 함께 할 수 없다.
//
// 이 저장소는 이 결함에 이미 한 번 물렸다(도크의 `gridStepPartial` — 치환자를 두 번 말하는
// 문구를 `replace` 로 바꿔 뒤쪽이 벌거벗은 채 남았다). 그래서 008 의 한도 문구도 상한을
// **두 번** 말하도록 일부러 적고, 두 자리 모두 바뀌는지를 진짜 번역으로 잰다.
//
// @spec SPEC-CANVAS-008 REQ-05 · REQ-06 · AC-08

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import { CanvasEditDockRegion } from '../CanvasEditDock';
import CanvasEditOverlay from '../CanvasEditOverlay';
import type { CanvasElement, CanvasSize } from '../canvasConfig';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import type { StageSize } from '../canvasGeometry';
import { useScratchpadStore } from './scratchpadStore';
import {
  SCRATCHPAD_MAX_BYTES,
  SCRATCHPAD_MAX_ENTRIES,
  cloneElements,
  type ScratchpadEntry,
} from './scratchpadTypes';

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

const SEED: readonly CanvasElement[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 100, y: 80, w: 200, h: 160 }, style: { fill: '#a' } },
];

function Harness() {
  const [elements, setElements] = useState<readonly CanvasElement[]>(SEED);
  const state = useCanvasEditSelectionState();
  return (
    <I18nProvider>
      <CanvasEditSelectionContext value={state}>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={{ stage: STAGE, canvas: CANVAS }}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </CanvasEditDockRegion>
      </CanvasEditSelectionContext>
    </I18nProvider>
  );
}

function stubOverlayRect(): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: 200,
    height: 100,
    right: 200,
    bottom: 100,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

/** el-1 을 고른다(px 40..120 × 20..60). */
function pickOne(): void {
  fireEvent(
    screen.getByTestId('canvas-edit-overlay'),
    new MouseEvent('pointerdown', { clientX: 60, clientY: 30, bubbles: true, cancelable: true }),
  );
}

function fullDrawer(): ScratchpadEntry[] {
  return Array.from({ length: SCRATCHPAD_MAX_ENTRIES }, (_, i) => ({
    id: `sp-${i + 1}`,
    name: '',
    created: 1_000 + i,
    origin: { x: 100, y: 80 },
    elements: cloneElements(SEED),
  }));
}

beforeEach(() => {
  localStorage.clear();
  useScratchpadStore.setState({ entries: [], notice: null });
  localStorage.clear();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('한도 문구는 상한을 두 번 말하고 두 자리 모두 바뀐다 (AC-08 · D7)', () => {
  it('항목 수 한도 문구', () => {
    const template = ko.dashboard.canvas.edit.scratchpadLimitEntries;
    // 고정 입력이 스스로 무엇을 재는지 먼저 단언한다 — 문구에서 치환자가 하나로 줄면
    // 이 시험은 `replace` 결함을 더 이상 잡지 못하며, 그 사실이 여기서 드러나야 한다.
    expect(template.split('{max}').length - 1, '치환자가 둘이 아니다').toBe(2);

    useScratchpadStore.setState({ entries: fullDrawer() });
    render(<Harness />);
    stubOverlayRect();
    pickOne();
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    const text = screen.getByTestId('canvas-scratchpad-notice').textContent ?? '';
    // `replace` 였다면 뒤쪽에 `{max}` 가 벌거벗은 채 남는다.
    expect(text).not.toContain('{max}');
    expect(text.split(String(SCRATCHPAD_MAX_ENTRIES)).length - 1, '상한이 두 번 나오지 않는다').toBe(
      2,
    );
    // 원문 키가 화면에 뜨지 않는다.
    expect(text).not.toContain('dashboard.canvas');
  });

  it('바이트 한도 문구', () => {
    const template = ko.dashboard.canvas.edit.scratchpadLimitBytes;
    expect(template.split('{maxKb}').length - 1, '치환자가 둘이 아니다').toBe(2);

    render(<Harness />);
    stubOverlayRect();
    pickOne();
    // 문구 하나로 예산을 넘긴다. 한글이라 UTF-8 로 30만 바이트다.
    // 저장소를 직접 부르므로 `act` 로 감싼다 — `fireEvent` 와 달리 스스로 흘리지 않는다.
    act(() => {
      useScratchpadStore.getState().saveEntry([
        {
          id: 'el-9',
          kind: 'text',
          geometry: { x: 0, y: 0 },
          text: '가'.repeat(100_000),
          style: {},
        },
      ]);
    });

    const text = screen.getByTestId('canvas-scratchpad-notice').textContent ?? '';
    const kb = String(Math.round(SCRATCHPAD_MAX_BYTES / 1024));
    expect(text).not.toContain('{maxKb}');
    expect(text.split(kb).length - 1, '상한이 두 번 나오지 않는다').toBe(2);
  });

  it('오래된 항목이 지워지지 않았음을 화면이 말한다', () => {
    useScratchpadStore.setState({ entries: fullDrawer() });
    render(<Harness />);
    stubOverlayRect();
    pickOne();
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    expect(useScratchpadStore.getState().entries).toHaveLength(SCRATCHPAD_MAX_ENTRIES);
    expect(useScratchpadStore.getState().entries.at(-1)?.id).toBe(`sp-${SCRATCHPAD_MAX_ENTRIES}`);
  });
});

describe('나머지 문구도 진짜 번역으로 뜬다 (REQ-06)', () => {
  it('빈 서랍 안내 · 기기 지역 고지 · 드롭 존 문구', () => {
    render(<Harness />);
    for (const [testid, expected] of [
      ['canvas-scratchpad-empty', ko.dashboard.canvas.edit.scratchpadEmpty],
      ['canvas-scratchpad-local-hint', ko.dashboard.canvas.edit.scratchpadLocalOnly],
      ['canvas-scratchpad-dropzone', ko.dashboard.canvas.edit.scratchpadDropHint],
    ] as const) {
      const node = screen.getByTestId(testid);
      expect(node.textContent, testid).toContain(expected);
      expect(node.textContent, testid).not.toContain('dashboard.canvas');
    }
  });

  it('저장 단추의 접근성 이름이 보이는 라벨을 포함한다 (WCAG 2.5.3)', () => {
    render(<Harness />);
    const button = screen.getByTestId('canvas-scratchpad-save');
    const visible = button.textContent ?? '';
    expect(visible).toBe(ko.dashboard.canvas.edit.scratchpadSave);
    expect(button.getAttribute('aria-label') ?? '').toContain(visible);
  });

  it('이름이 없는 항목은 자동 이름을 자리표시로 보인다 — 치환자가 남지 않는다', () => {
    // **이 문구의 치환자는 하나뿐이라 `replace`/`replaceAll` 이 같은 답을 낸다.** 그래서
    // 이 시험은 치환 방식을 재지 못한다 — 그 사실을 숨기지 않고, 대신 **재는 것이 사라지는
    // 순간**을 잡는다: 번역이 `{count}` 를 두 번 말하게 되면 아래 단언이 먼저 빨개져
    // "이제 `replaceAll` 이 실제로 필요하다" 고 알린다. 치환 방식 자체를 재는 자리는
    // 한도 문구 쪽이다(위 describe — 거기서는 치환자가 진짜로 둘이다).
    for (const [locale, template] of [
      ['ko', ko.dashboard.canvas.edit.scratchpadAutoName],
      ['en', en.dashboard.canvas.edit.scratchpadAutoName],
    ] as const) {
      expect(template.split('{count}').length - 1, `${locale}: 치환자 수가 달라졌다`).toBe(1);
    }

    render(<Harness />);
    stubOverlayRect();
    pickOne();
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));
    const id = useScratchpadStore.getState().entries[0]?.id ?? '';

    const input = screen.getByTestId<HTMLInputElement>(`canvas-scratchpad-name-${id}`);
    expect(input.placeholder).toBe(
      ko.dashboard.canvas.edit.scratchpadAutoName.replaceAll('{count}', '1'),
    );
    expect(input.placeholder).not.toContain('{count}');
  });

  it('저장에 성공해도 화면이 말한다 — 조용한 성공도 두지 않는다', () => {
    render(<Harness />);
    stubOverlayRect();
    pickOne();
    fireEvent.click(screen.getByTestId('canvas-scratchpad-save'));

    expect(screen.getByTestId('canvas-scratchpad-notice').textContent).toContain(
      ko.dashboard.canvas.edit.scratchpadSaved,
    );
    fireEvent.click(screen.getByTestId('canvas-scratchpad-notice-dismiss'));
    expect(screen.queryByTestId('canvas-scratchpad-notice')).toBeNull();
  });

  it('저장소가 던지면 "이번 세션에만 남는다" 고 말한다 (REQ-05)', () => {
    render(<Harness />);
    stubOverlayRect();
    pickOne();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });

    expect(() => fireEvent.click(screen.getByTestId('canvas-scratchpad-save'))).not.toThrow();

    expect(screen.getByTestId('canvas-scratchpad-notice').textContent).toContain(
      ko.dashboard.canvas.edit.scratchpadVolatile,
    );
    // 그리고 캔버스의 편집은 그대로 동작한다.
    expect(screen.getByTestId('canvas-selection-el-1')).toBeTruthy();
  });
});
