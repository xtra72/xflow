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

import { fireEvent, render, screen } from '@testing-library/react';
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
    const help = screen.getByRole('button', { name: ko.property.fieldHelp.viewDescription });
    expect(help).toBeTruthy();
    expect(screen.queryByRole('tooltip')).toBeNull();

    fireEvent.click(help);
    expect(screen.getByRole('tooltip').textContent).toContain('7');
  });

  it('나누어떨어지는 값에는 고지도 `?` 도 없다', () => {
    renderDock();
    setStep('50');

    expect(screen.queryByTestId('canvas-grid-step-partial')).toBeNull();
    expect(
      screen.queryByRole('button', { name: ko.property.fieldHelp.viewDescription }),
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
