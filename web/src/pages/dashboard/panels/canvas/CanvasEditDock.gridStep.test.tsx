// 격자 간격 자유 입력 — **실제 번역 문구**로 재는 자리 (SPEC-CANVAS-002 0.11.0).
//
// 왜 파일을 나누는가: 이웃한 두 시험 파일(`CanvasEditDock.test.tsx` ·
// `CanvasEditOverlay.test.tsx`)은 i18n 을 **키를 그대로 돌려주는** 것으로 대체한다. 그
// 대체는 "어떤 문구가 뜨는가" 를 묻지 않는 시험에는 옳지만, 여기서 물으려는 것은 정확히
// 그 반대다 — 자투리 고지가 **어떤 두 정수가 그 결과를 냈는지**를 실제로 적어 내는가.
// 자리표시자(`{step}` · `{width}` · `{height}`)가 채워지지 않으면 사용자는 무엇을 바꿔야
// 하는지 알 수 없고, 그 실패는 키를 돌려주는 대체 아래에서는 드러나지 않는다.
// `vi.mock` 은 파일 단위이므로 대체를 달리하려면 파일을 나누는 수밖에 없다.
//
// 그래서 이 파일의 `t` 는 **ko.json 의 진짜 문구**를 돌려준다. 문구가 자리표시자를 잃으면
// (번역을 손보다 지우는 일이 실제로 일어난다) 이 시험이 곧바로 무너진다.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ko from '@/lib/i18n/ko.json';
import en from '@/lib/i18n/en.json';

vi.mock('@/lib/i18n', async () => {
  const messages = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const lookup = (key: string): string => {
    const value = key
      .split('.')
      .reduce<unknown>(
        (node, seg) =>
          node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
        messages,
      );
    return typeof value === 'string' ? value : key;
  };
  return { useTranslation: () => ({ t: lookup }) };
});

import { cleanup } from '@testing-library/react';

import { DEFAULT_CANVAS_SIZE, type CanvasElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection } from './canvasGeometry';

afterEach(cleanup);

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 160 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

function Harness() {
  const [elements, setElements] = useState<readonly CanvasElement[]>([]);
  const selection = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={selection}>
      <CanvasEditDockRegion enabled>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={PROJ}
          textWidths={{}}
          onElementsChange={setElements}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

function stepInput(): HTMLInputElement {
  return screen.getByTestId('canvas-grid-step') as HTMLInputElement;
}

function setStep(value: string): void {
  fireEvent.change(stepInput(), { target: { value } });
}

function renderDock(): void {
  render(<Harness />);
  // 간격 칸은 격자를 켜야 살아난다 — 눌러도 화면이 그대로인 컨트롤은 고장으로 보인다.
  fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
}

describe('자투리 고지는 어떤 두 정수가 그 결과를 냈는지 적는다', () => {
  it('나누어떨어지지 않으면 간격과 캔버스 두 축이 문구에 그대로 있다', () => {
    renderDock();
    setStep('7');

    const hint = screen.getByTestId('canvas-grid-step-partial');
    expect(hint.textContent).toContain('7');
    expect(hint.textContent).toContain('500');
    expect(hint.textContent).toContain('400');
    // 자리표시자가 남아 있으면 채우지 못했다는 뜻이다.
    expect(hint.textContent).not.toContain('{step}');
    expect(hint.textContent).not.toContain('{width}');
    expect(hint.textContent).not.toContain('{height}');
  });

  it('고지는 `?` 뒤에 산다 — 늘 펼쳐 두면 좁은 도크가 경고문으로 덮인다', () => {
    renderDock();
    setStep('7');

    // 이 저장소의 관용구: 눈으로 보는 사람에게는 단추 하나, 보조기기에게는 sr-only 설명.
    //
    // **찾는 자리를 이 고지로 좁힌다**(SPEC-CANVAS-006 M9). 0.6.0 이 도크 맨 앞에 **상시**
    // 도움말 하나를 세웠으므로 "도크에 `?` 가 하나뿐" 이라는 전제가 더는 참이 아니다 —
    // 그 전제는 이 시험이 재려던 것이 아니라 도크가 그때 그랬을 뿐이다. 단언의 뜻은 한
    // 글자도 바뀌지 않는다: **이 고지**가 `?` 뒤에 살고, 눌러야 열리고, 열면 그 두 정수를
    // 말한다.
    const help = within(screen.getByTestId('canvas-grid-step-partial').parentElement!).getByRole(
      'button',
      { name: ko.property.fieldHelp.viewDescription },
    );
    expect(help).toBeTruthy();
    expect(screen.queryByRole('tooltip')).toBeNull();

    fireEvent.click(help);
    expect(screen.getByRole('tooltip').textContent).toContain('7');
  });

  it('나누어떨어지는 값에는 고지도 `?` 도 없다', () => {
    renderDock();
    setStep('50');

    expect(screen.queryByTestId('canvas-grid-step-partial')).toBeNull();
    // 같은 이유로 자리를 **격자 묶음 안**으로 좁힌다(위 주석). 보기 묶음의 상시 도움말은
    // 다른 묶음의 것이고, 여기서 없어야 하는 것은 **격자 간격 칸 옆의** `?` 다.
    const gridSection = screen.getByTestId('canvas-grid-step').closest('section');
    expect(gridSection).not.toBeNull();
    expect(
      within(gridSection!).queryByRole('button', { name: ko.property.fieldHelp.viewDescription }),
    ).toBeNull();
  });

  it('문구는 격자 · 붙임 · Shift 가 그대로 그 값을 쓴다고 말한다 — 자투리는 고장이 아니다', () => {
    // 고지가 "잘린다" 만 말하면 사용자는 값이 무시된다고 읽는다. 실제로는 셋 다 적은 값
    // 그대로 동작하고 마지막 칸만 짧다 — 그 사실이 문구에 있어야 오해가 생기지 않는다.
    renderDock();
    setStep('7');

    const text = screen.getByTestId('canvas-grid-step-partial').textContent ?? '';
    expect(text).toContain('Shift');
  });
});

describe('두 언어 모두 자리표시자를 갖는다 (한쪽만 고치면 다른 쪽이 벌거벗은 문장이 된다)', () => {
  it('ko · en 문구에 {step} · {width} · {height} 가 모두 있다', () => {
    for (const messages of [ko, en]) {
      const template = messages.dashboard.canvas.edit.gridStepPartial;
      expect(template).toContain('{step}');
      expect(template).toContain('{width}');
      expect(template).toContain('{height}');
    }
  });
});

// --- 보기 배율 문구 (SPEC-CANVAS-006 M9 · AC-09 (AV)) --------------------

describe('배율 제안 글자가 실제 번역 문구로 채워진다', () => {
  it('옵션이 벌거벗은 숫자가 아니다 — 치환자가 남지 않고 백분율임이 글자로 읽힌다', () => {
    // 바로 한 줄 아래 칸이 **캔버스 단위**를 받으므로, 이 목록이 벌거벗은 숫자만 보이면
    // 그것이 백분율인지 좌표인지 알 수 없다. 키를 그대로 돌려주는 대체 아래에서는 이
    // 실패가 드러나지 않아 파일을 나눠 진짜 문구로 잰다.
    renderDock();

    const options = [
      ...screen.getByTestId('canvas-workspace-zoom-suggestions').querySelectorAll('option'),
    ];
    expect(options.map((o) => o.value)).toEqual(['25', '50', '75', '100']);
    for (const option of options) {
      const text = option.textContent ?? '';
      expect(text).not.toContain('{percent}');
      expect(text).toContain(option.value);
      // 숫자만 있는 것이 아니다 — 단위가 글자로 붙어 있다.
      expect(text.replace(/[0-9]/g, '').trim().length).toBeGreaterThan(0);
    }
  });

  it('상시 도움말이 진짜 문구로 뜨고 치환자가 남지 않는다', () => {
    renderDock();

    const hint = screen.getByTestId('canvas-workspace-zoom-hint');
    expect(hint.textContent ?? '').not.toMatch(/\{[a-z]+\}/i);
    expect((hint.textContent ?? '').length).toBeGreaterThan(0);
  });
});

describe('두 언어 모두 배율 문구를 갖는다 (키 이름에 점이 없다)', () => {
  it('ko · en 에 넷이 모두 있고 옵션 문구에 {percent} 가 있다', () => {
    for (const messages of [ko, en]) {
      const edit = messages.dashboard.canvas.edit;
      expect(edit.dockView.length).toBeGreaterThan(0);
      expect(edit.workspaceZoom.length).toBeGreaterThan(0);
      expect(edit.workspaceZoomOption).toContain('{percent}');
      expect(edit.workspaceZoomHint.length).toBeGreaterThan(0);
    }
    // 두 언어가 실제로 다른 문구다 — 한쪽을 복사해 두면 번역이 없는 것과 같다.
    expect(ko.dashboard.canvas.edit.dockView).not.toBe(en.dashboard.canvas.edit.dockView);
    expect(ko.dashboard.canvas.edit.workspaceZoomHint).not.toBe(
      en.dashboard.canvas.edit.workspaceZoomHint,
    );
  });

  it('키 이름 안에 점이 없다 (프로젝트 규약)', () => {
    for (const messages of [ko, en]) {
      for (const key of Object.keys(messages.dashboard.canvas.edit)) {
        expect(key).not.toContain('.');
      }
    }
  });
});
