// 가져오기 묶음이 **서버의 상한**을 쓰는지, 그리고 못 얻었을 때 **여전히 동작하는지**를
// 잰다 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// **왜 계획 층 시험으로 충분하지 않은가.** `svgImportLimits.test.ts` 는 `planSvgImport` 가
// 주어진 상한을 쓴다는 것을 잰다. 그러나 화면 층이 그 상한을 **넘기지 않으면**(조회를
// 빠뜨렸거나, 얻고도 인자에 싣지 않으면) 계획 층은 컴파일 기본값으로 조용히 돌아가고 그
// 시험은 여전히 초록이다. 이 파일이 그 이음매를 덮는다.
//
// **폴백이 여기서 가장 중요하다.** 서버에 닿지 못한 편집기가 가져오기를 **못 하게 되는
// 것**이 상한이 낡는 것보다 나쁘다 — 그 성질을 던지는 조회로 직접 친다.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { DashboardLimits } from '@/services/api/dashboardLimitsService';

import { DEFAULT_CANVAS_SIZE } from '../canvasConfig';
import { CanvasSvgImport } from './CanvasSvgImport';
import { MAX_IMPORT_ELEMENTS } from './svgImportTypes';

afterEach(cleanup);

/** 요소 n 개짜리 문서 — 사각형은 명령을 한 칸도 쓰지 않으므로 요소 축만 친다. */
function svgWith(n: number): string {
  const body = Array.from(
    { length: n },
    (_v, i) => `<rect x="${i % 20}" y="12" width="3" height="3" fill="#c0392b"/>`,
  ).join('');
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-13 7 317 181">${body}</svg>`;
}

/** 묶음을 펴고 파일을 골라, 준비됨이나 거절됨이 뜰 때까지 기다린다. */
async function chooseFile(): Promise<void> {
  fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
  const input = screen.getByTestId('canvas-svg-import-file');
  fireEvent.change(input, {
    target: { files: [new File(['x'], 'a.svg', { type: 'image/svg+xml' })] },
  });
  await waitFor(() => {
    const ready = screen.queryByTestId('canvas-svg-import-summary');
    const refused = screen.queryByTestId('canvas-svg-import-refused');
    expect(ready ?? refused).not.toBeNull();
  });
}

function renderWith(text: string, fetchLimits: () => Promise<DashboardLimits>): void {
  render(
    <I18nProvider>
      <CanvasSvgImport
        canvas={DEFAULT_CANVAS_SIZE}
        onPlace={() => {}}
        readFile={async () => text}
        fetchLimits={fetchLimits}
      />
    </I18nProvider>,
  );
}

describe('CanvasSvgImport — 서버 상한', () => {
  it('서버가 올린 상한을 쓰면 컴파일 기본값을 넘는 문서도 받는다', async () => {
    const over = MAX_IMPORT_ELEMENTS + 40;
    renderWith(svgWith(over), async () => ({
      maxCanvasElements: over,
      payloadBudgetBytes: over * 1024,
    }));

    await chooseFile();

    expect(screen.queryByTestId('canvas-svg-import-refused')).toBeNull();
    expect(screen.getByTestId('canvas-svg-import-summary')).not.toBeNull();
  });

  // --- 폴백 세 갈래 -------------------------------------------------------
  //
  // 셋을 갈라 세는 것은 **원인이 다르기 때문**이다: 값을 못 얻었다(undefined) · 조회가
  // 던졌다(오프라인) · 서버가 쓸 수 없는 수를 말했다. 어느 쪽이든 가져오기는 계속되고
  // 컴파일 기본값이 서야 한다.

  it('값을 못 얻으면(undefined) 컴파일 기본값으로 동작한다', async () => {
    renderWith(svgWith(MAX_IMPORT_ELEMENTS + 36), async () => ({
      maxCanvasElements: undefined,
      payloadBudgetBytes: undefined,
    }));

    await chooseFile();

    // 기본값(1024)으로 거절되고, **그 수를 화면이 말한다**.
    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.textContent).toContain(String(MAX_IMPORT_ELEMENTS));
    expect(line.textContent).toContain(String(MAX_IMPORT_ELEMENTS + 36));
  });

  it('조회가 **던져도** 가져오기가 서지 않는다 — 기본값으로 계속한다', async () => {
    const failing = vi.fn(async (): Promise<DashboardLimits> => {
      throw new Error('Network Error');
    });
    // 기본값 아래의 문서 — 조회 실패가 **거절로 번지지 않음**을 재려면 통과해야 한다.
    renderWith(svgWith(12), failing);

    await chooseFile();

    expect(failing).toHaveBeenCalled();
    expect(screen.queryByTestId('canvas-svg-import-refused')).toBeNull();
    expect(screen.getByTestId('canvas-svg-import-summary')).not.toBeNull();
  });

  it('서버가 쓸 수 없는 수를 말해도 기본값으로 떨어진다', async () => {
    renderWith(svgWith(MAX_IMPORT_ELEMENTS + 36), async () => ({
      maxCanvasElements: 0,
      payloadBudgetBytes: 0,
    }));

    await chooseFile();

    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.textContent).toContain(String(MAX_IMPORT_ELEMENTS));
  });

  it('상한을 **낮추면** 기본값 아래에서도 거절하고 그 수를 말한다', async () => {
    renderWith(svgWith(20), async () => ({
      maxCanvasElements: 10,
      payloadBudgetBytes: 256 * 1024,
    }));

    await chooseFile();

    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.textContent).toContain('10');
    expect(line.textContent).toContain('20');
    // 컴파일 상수가 새면 사용자가 적용되지 않은 수를 읽는다.
    expect(line.textContent).not.toContain(String(MAX_IMPORT_ELEMENTS));
  });
});
