// 가져오기 묶음의 화면 시험 (SPEC-CANVAS-007 M9 · AC-07 · AC-E1 · AC-E11).
//
// **꺼져 있어서 통과하는 초록을 만들지 않는다.** 각 시나리오는 자기가 재려는 것이 **켜져
// 있음을 먼저 단언한다** — 보고 시험의 고정 입력에는 버림과 근사가 실제로 들어 있고, 상한
// 시험의 고정 입력은 실제로 상한을 넘으며, "가져올 것이 없다" 시험의 문서는 `viewBox` 는
// 있으나 그릴 것이 없다.
//
// **고정 입력의 `viewBox` 는 `"-13 7 317 181"` 이다**(시험 규율 E2 · E3): 원점이 0 이 아니고 ·
// `minX` 가 음수이며 · 비정사각이고 · `317`·`181` 이 `PATH_LOCAL_EXTENT`(10000)를 나누어
// 떨어뜨리지 않는다. 정사각 `viewBox` 는 x·y 축척을 맞바꾼 결함을 통째로 숨긴다.
//
// **파일 읽기를 갈아 끼운다.** jsdom 의 `File` 에는 `text()` 가 없고(측정) `FileReader` 는
// 비동기라, 이 파일에서는 읽기 함수를 주입해 **화면의 상태 기계**만 잰다. 진짜 `FileReader`
// 경로는 `svgImportPresent.test.ts` 가 따로 잰다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04 · REQ-06 · AC-07

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import { DEFAULT_CANVAS_SIZE } from '../canvasConfig';
import { appendImportedElements } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import { clearSurface, drawElements, type DrawContext2D } from '../drawElement';
import { MAX_PATH_COMMANDS } from '../shapes/pathTypes';
import { CanvasSvgImport } from './CanvasSvgImport';
import { fitPreviewSize } from './svgImportPresent';
import type { ImportedPathSpec } from './svgImportPlan';
import { MAX_IMPORT_ELEMENTS, MAX_IMPORT_FILE_BYTES } from './svgImportTypes';

// --- 고정 입력 -----------------------------------------------------------

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 · 안 나누어떨어짐 (E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

function svg(body: string, attrs = VIEW_BOX): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${body}</svg>`;
}

/** 씨앗 색(`#3b82f6`)과 **다른** 색을 쓴다 — 같으면 "읽었는가" 를 구분할 수 없다(E8). */
const PLAIN = svg('<rect x="5" y="12" width="40" height="20" fill="#c0392b"/>');

/** 버림 셋과 근사 하나가 실제로 들어 있는 문서. */
const NOISY = svg(
  '<style>.a{fill:red}</style>' +
    '<text x="1" y="2">버려짐</text>' +
    '<text x="3" y="4">둘째</text>' +
    '<image href="a.png" x="0" y="0" width="4" height="4"/>' +
    '<path d="M0 0 L10 10 L0 10 Z" fill="#c0392b" fill-rule="evenodd"/>' +
    '<defs><linearGradient id="g"><stop stop-color="#8e44ad"/></linearGradient></defs>' +
    '<metadata>무시</metadata><title>무시</title>',
);

function file(text: string, name = 'a.svg'): File {
  return new File([text], name, { type: 'image/svg+xml' });
}

interface Placed {
  shapes: readonly ImportedPathSpec[];
}

function renderImport(text: string | (() => Promise<string>)): { placed: Placed[] } {
  const placed: Placed[] = [];
  const readFile =
    typeof text === 'string' ? async (): Promise<string> => text : async (): Promise<string> => text();
  render(
    <I18nProvider>
      <CanvasSvgImport
        canvas={DEFAULT_CANVAS_SIZE}
        onPlace={(shapes) => placed.push({ shapes })}
        readFile={readFile}
      />
    </I18nProvider>,
  );
  return { placed };
}

/** 묶음을 펴고 파일을 고른다. */
async function choose(f: File): Promise<void> {
  fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
  const input = screen.getByTestId('canvas-svg-import-file');
  fireEvent.change(input, { target: { files: [f] } });
  await waitFor(() => {
    const ready = screen.queryByTestId('canvas-svg-import-summary');
    const refused = screen.queryByTestId('canvas-svg-import-refused');
    expect(ready ?? refused).not.toBeNull();
  });
}

/** 그린 것을 그대로 적는 2D context 대체. `fill`/`stroke` 는 그때의 색까지 적는다. */
type Call = (string | number)[];

function makeRecorder(calls: Call[]) {
  return {
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {
      calls.push(['beginPath']);
    },
    rect() {},
    ellipse() {},
    moveTo(x: number, y: number) {
      calls.push(['moveTo', x, y]);
    },
    lineTo(x: number, y: number) {
      calls.push(['lineTo', x, y]);
    },
    closePath() {
      calls.push(['closePath']);
    },
    bezierCurveTo(a: number, b: number, c: number, d: number, e: number, f: number) {
      calls.push(['bezierCurveTo', a, b, c, d, e, f]);
    },
    stroke() {
      calls.push(['stroke', String(this.strokeStyle), this.lineWidth]);
    },
    fill() {
      calls.push(['fill', String(this.fillStyle)]);
    },
    fillText() {},
    measureText(text: string) {
      return { width: text.length * 10 };
    },
    clearRect() {
      calls.push(['clearRect']);
    },
    fillRect() {},
    fillStyle: '' as string,
    strokeStyle: '' as string,
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'middle',
  };
}

beforeEach(() => {
  // jsdom 은 2D context 를 주지 않고 **부르는 자리마다 시끄럽게 운다**. 기본값을 `null` 로
  // 세워 두면 그 소음이 사라지고, 미리보기가 컨텍스트 없이도 서는지가 덤으로 재어진다 —
  // 무엇을 그리는지 재는 아래 시험이 이 대체를 다시 갈아 끼운다.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 무동작 보장 (AC-E1) ---------------------------------------------------

describe('AC-E1 — 파일을 고르기 전에는 묶음 머리 한 줄뿐이다', () => {
  it('묶음이 **기본으로 접혀** 있고 본문이 DOM 에 아예 없다', () => {
    renderImport(PLAIN);
    const head = screen.getByTestId('canvas-svg-import-group');
    expect(head.getAttribute('aria-expanded')).toBe('false');
    // `hidden` 이 아니라 **미마운트**다 — 접힌 묶음이 파일 입력을 초점 순회에 남기지 않는다.
    expect(screen.queryByTestId('canvas-svg-import-group-body')).toBeNull();
    expect(screen.queryByTestId('canvas-svg-import-file')).toBeNull();
    expect(screen.queryByTestId('canvas-svg-import-preview')).toBeNull();
    // 없는 id 를 가리키는 `aria-controls` 는 보조기술에게 거짓말이다.
    expect(head.getAttribute('aria-controls')).toBeNull();
  });

  it('펴면 파일 입력이 서고 접으면 다시 사라진다', () => {
    renderImport(PLAIN);
    const head = screen.getByTestId('canvas-svg-import-group');
    fireEvent.click(head);
    expect(head.getAttribute('aria-expanded')).toBe('true');
    expect(head.getAttribute('aria-controls')).toBe(
      screen.getByTestId('canvas-svg-import-group-body').id,
    );
    expect(screen.getByTestId('canvas-svg-import-file').getAttribute('accept')).toBe(
      '.svg,image/svg+xml',
    );
    fireEvent.click(head);
    expect(screen.queryByTestId('canvas-svg-import-group-body')).toBeNull();
  });
});

// --- 보고 (AC-07) ----------------------------------------------------------

describe('AC-07 — 못 다룬 것을 화면이 말한다', () => {
  it('요약 한 줄이 **접히지 않고** 개수를 셋 다 말한다', async () => {
    renderImport(NOISY);
    await choose(file(NOISY));

    const summary = screen.getByTestId('canvas-svg-import-summary');
    // 목록 토글을 누르지 않은 상태다 — 요약은 그래도 보인다.
    expect(screen.queryByTestId('canvas-svg-import-notes')).toBeNull();
    // 벌거벗은 치환자가 남지 않았다.
    expect(summary.textContent ?? '').not.toContain('{');
    // 도형 1개 · 명령 4개(M L L Z) — 켜져 있음을 수로 못박는다.
    expect(summary.textContent).toContain('1');
    expect(summary.textContent).toContain('4');
  });

  it('보고가 두 갈래로 갈리고 각 항목이 **개수**를 말한다', async () => {
    renderImport(NOISY);
    await choose(file(NOISY));

    fireEvent.click(screen.getByTestId('canvas-svg-import-notes-toggle'));
    const body = screen.getByTestId('canvas-svg-import-notes');

    // 버림 셋 · 근사 하나가 실제로 켜져 있다.
    const text = screen.getByTestId('canvas-svg-import-note-textDropped');
    expect(text.textContent).toContain('2'); // <text> 둘 — "있습니다" 가 아니라 "2개".
    expect(body.querySelector('[data-testid="canvas-svg-import-note-imageDropped"]')).not.toBeNull();
    expect(body.querySelector('[data-testid="canvas-svg-import-note-styleRuleDropped"]')).not.toBeNull();
    expect(body.querySelector('[data-testid="canvas-svg-import-note-evenOddWinding"]')).not.toBeNull();
  });

  it('`<defs>`·`<metadata>`·`<title>` 은 **어느 갈래에도 오르지 않는다**', async () => {
    renderImport(NOISY);
    await choose(file(NOISY));
    fireEvent.click(screen.getByTestId('canvas-svg-import-notes-toggle'));

    // 그리지 않는 것을 버렸다고 말하면 보고가 잡음이 되고, 잡음 섞인 보고는 읽히지 않는다.
    const ids = [...screen.getByTestId('canvas-svg-import-notes').querySelectorAll('[data-testid^="canvas-svg-import-note-"]')]
      .map((n) => n.getAttribute('data-testid'));
    expect(ids).not.toContain('canvas-svg-import-note-degenerateTransformDropped');
    expect(ids.filter((id) => id?.includes('defs') === true)).toEqual([]);
    // 갈래별 목록은 정확히 넷이다 — 다섯째가 생기면 무엇이 새로 올랐는지 여기서 드러난다.
    expect(ids).toHaveLength(4);
  });

  it('취소 뒤 다시 고르면 갈래 목록이 **다시 접힌다** — 앞 파일의 펼침을 물려받지 않는다', async () => {
    // 초기값(`useState(false)`)만으로는 이 성질이 재어지지 않는다: 파일을 고르기 전에는
    // 목록이 아예 없으므로 초기값을 뒤집어도 어느 단언도 울지 않는다(뮤테이션 M9-3). 관측되는
    // 것은 **대기로 돌아갈 때의 리셋**이며, 그것이 이 시험이 잡는 자리다.
    renderImport(NOISY);
    await choose(file(NOISY));
    fireEvent.click(screen.getByTestId('canvas-svg-import-notes-toggle'));
    expect(screen.getByTestId('canvas-svg-import-notes')).toBeTruthy();

    fireEvent.click(screen.getByTestId('canvas-svg-import-cancel'));
    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), { target: { files: [file(NOISY)] } });
    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy());
    expect(screen.getByTestId('canvas-svg-import-notes-toggle').getAttribute('aria-expanded')).toBe(
      'false',
    );
    expect(screen.queryByTestId('canvas-svg-import-notes')).toBeNull();
  });

  it('보고는 [놓기] 를 누르기 **전에** 보인다 — 누르지 않은 상태에서 잰다', async () => {
    const { placed } = renderImport(NOISY);
    await choose(file(NOISY));
    expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy();
    expect(screen.getByTestId('canvas-svg-import-notes-toggle')).toBeTruthy();
    // 사라지는 알림(토스트)이 아니다 — 문구가 **DOM 에 있고** 그대로 남는다.
    expect(placed).toHaveLength(0);
  });

  it('이상할 것이 하나도 없는 파일에서는 보고 토글 자체가 서지 않는다 (AC-E6 · 위험 R7)', async () => {
    renderImport(PLAIN);
    await choose(file(PLAIN));
    expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy();
    // "근사됨 0개 · 버림 0개" 는 아무것도 말하지 않으면서 한 줄을 먹는다.
    expect(screen.queryByTestId('canvas-svg-import-notes-toggle')).toBeNull();
  });

  it('명령 상한으로 버린 도형이 있으면 **다른 길을 이름으로 가리킨다** (§결정 4)', async () => {
    // 무리로 나눠도 상한을 넘는 한 덩어리 — 부분 경로가 하나이므로 나뉘지 않는다.
    const long =
      'M 0 0 ' +
      Array.from({ length: MAX_PATH_COMMANDS + 20 }, (_v, i) => `L ${i % 50} ${(i * 7) % 40}`).join(' ');
    const doc = svg(`<path d="${long}" fill="#c0392b"/><rect x="1" y="12" width="9" height="9" fill="#c0392b"/>`);
    renderImport(doc);
    await choose(file(doc));

    fireEvent.click(screen.getByTestId('canvas-svg-import-notes-toggle'));
    expect(screen.getByTestId('canvas-svg-import-note-commandLimitDropped').textContent).toContain('1');
    expect(screen.getByTestId('canvas-svg-import-command-limit-hint').textContent ?? '').not.toBe('');
  });

  it('가져올 것이 하나도 없으면 [놓기] 가 **쓸 수 없다** (REQ-03)', async () => {
    // `viewBox` 는 있고 그릴 것은 없다 — 그래서 거절이 아니라 "0개 준비됨" 이다.
    const empty = svg('<defs><linearGradient id="g"><stop stop-color="#8e44ad"/></linearGradient></defs>');
    renderImport(empty);
    await choose(file(empty));
    expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy();
    expect(screen.getByTestId('canvas-svg-import-empty')).toBeTruthy();
    expect((screen.getByTestId('canvas-svg-import-place') as HTMLButtonElement).disabled).toBe(true);
  });
});

// --- 거절 ------------------------------------------------------------------

describe('거절은 값으로 돌아와 화면에 뜬다 (REQ-03 · REQ-07)', () => {
  it('SVG 가 아닌 문서는 사유를 말하고 **다른 파일 고르기**를 내민다', async () => {
    const bad = '<note xmlns="urn:x"><to>a</to></note>';
    renderImport(bad);
    await choose(file(bad));
    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.getAttribute('role')).toBe('status');
    expect(line.textContent ?? '').not.toContain('{');
    expect(screen.getByTestId('canvas-svg-import-pick').textContent ?? '').not.toBe('');
    expect(screen.queryByTestId('canvas-svg-import-place')).toBeNull();
  });

  it('요소 상한을 넘으면 **실제 수와 상한을 함께** 말하고 하나도 만들지 않는다', async () => {
    const many = svg(
      Array.from({ length: MAX_IMPORT_ELEMENTS + 36 }, (_v, i) => `<rect x="${i % 20}" y="12" width="3" height="3" fill="#c0392b"/>`).join(''),
    );
    const { placed } = renderImport(many);
    await choose(file(many));
    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.textContent).toContain(String(MAX_IMPORT_ELEMENTS + 36));
    expect(line.textContent).toContain(String(MAX_IMPORT_ELEMENTS));
    expect(placed).toHaveLength(0);
    expect(screen.queryByTestId('canvas-svg-import-preview')).toBeNull();
  });

  it('바이트 상한은 **읽기보다 먼저** 본다 — 읽기 함수가 불리지 않는다', async () => {
    const read = vi.fn(async () => PLAIN);
    render(
      <I18nProvider>
        <CanvasSvgImport canvas={DEFAULT_CANVAS_SIZE} onPlace={() => {}} readFile={read} />
      </I18nProvider>,
    );
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
    const big = new File(['x'], 'big.svg', { type: 'image/svg+xml' });
    // `File.size` 는 읽기 전용이라 값을 심는다 — 2MiB 짜리 문자열을 시험이 만들 이유가 없다.
    Object.defineProperty(big, 'size', { value: MAX_IMPORT_FILE_BYTES + 1 });
    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), { target: { files: [big] } });

    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-refused')).toBeTruthy());
    expect(read).not.toHaveBeenCalled();
    const line = screen.getByTestId('canvas-svg-import-refused');
    expect(line.textContent ?? '').not.toContain('{');
    // 상한을 KB 로 말한다 — 바이트 수를 그대로 내밀면 사람이 못 읽는다.
    expect(line.textContent).toContain(String(MAX_IMPORT_FILE_BYTES / 1024));
  });

  it('읽지 못한 파일은 **문서가 나쁜 것과 다른 문구**로 말한다', async () => {
    const failing = vi.fn(async () => {
      throw new Error('boom');
    });
    render(
      <I18nProvider>
        <CanvasSvgImport canvas={DEFAULT_CANVAS_SIZE} onPlace={() => {}} readFile={failing} />
      </I18nProvider>,
    );
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), { target: { files: [file(PLAIN)] } });

    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-refused')).toBeTruthy());
    const unreadable = screen.getByTestId('canvas-svg-import-refused').textContent;
    cleanup();

    // 같은 자리에 SVG 가 아닌 문서를 넣으면 **다른 문구**가 나온다.
    renderImport('<note xmlns="urn:x"/>');
    await choose(file('<note xmlns="urn:x"/>'));
    expect(screen.getByTestId('canvas-svg-import-refused').textContent).not.toBe(unreadable);
  });
});

// --- 놓기와 취소 -----------------------------------------------------------

describe('놓기와 취소 (REQ-06 · 불변식 K15)', () => {
  it('[놓기] 가 도형마다 제 상자를 얹어 넘기고 묶음이 대기로 돌아간다 (AC-05)', async () => {
    const doc = svg('<rect x="5" y="12" width="40" height="20" fill="#c0392b"/><circle cx="60" cy="40" r="9" fill="#145a32"/>');
    const { placed } = renderImport(doc);
    await choose(file(doc));
    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));

    expect(placed).toHaveLength(1);
    expect(placed[0]?.shapes).toHaveLength(2);
    const [rect, circle] = placed[0]!.shapes;
    // **그림이 일그러지지 않는다** — 그것이 AC-05 가 문서 종횡비로 말하려던 것이다.
    // 상자가 도형마다인 지금은 종횡비 하나를 물을 자리가 없고, 대신 **각 도형이 제 사용자
    // 단위 종횡비를 그대로 든다**를 묻는다. 이 편이 강하다: 공유 상자 시절의 단언은 상자
    // 하나의 비만 보았으므로 도형이 저마다 일그러져도 통과했다.
    expect(rect!.box.w / rect!.box.h).toBeCloseTo(40 / 20, 1);
    // 원은 **정사각**이다. 축척을 축마다 다르게 준 결함은 여기서만 보인다 — 사각 하나로는
    // 40:20 과 축이 뒤바뀐 20:40 을 구분할 수 있어도, 축척이 조금 어긋난 것은 못 본다.
    expect(circle!.box.w).toBe(circle!.box.h);
    for (const shape of placed[0]!.shapes) {
      expect(shape.box.w).toBeGreaterThan(0);
      expect(shape.box.h).toBeGreaterThan(0);
    }
    // 그리고 두 상자가 **다르다** — 같으면 공유 상자로 되돌아간 것이다.
    expect(rect!.box).not.toEqual(circle!.box);
    // 놓은 뒤에는 대기로 — 같은 계획이 두 번 놓이지 않는다.
    expect(screen.queryByTestId('canvas-svg-import-summary')).toBeNull();
    expect(screen.getByTestId('canvas-svg-import-pick')).toBeTruthy();
  });

  it('[취소] 는 **무동작**이다 — 콜백이 한 번도 불리지 않는다', async () => {
    const { placed } = renderImport(PLAIN);
    await choose(file(PLAIN));
    fireEvent.click(screen.getByTestId('canvas-svg-import-cancel'));
    expect(placed).toEqual([]);
    expect(screen.queryByTestId('canvas-svg-import-summary')).toBeNull();
  });

  it('고른 뒤 입력 값을 **비운다** — 비우지 않으면 같은 파일을 다시 고를 수 없다', async () => {
    // `input.value` 를 그냥 읽는 단언은 jsdom 에서 아무것도 재지 않는다: `fireEvent.change` 는
    // `files` 만 심고 `value` 는 언제나 `''` 이라, 비우는 줄을 지워도 초록으로 통과한다.
    // 그래서 **대입 자체**를 관측한다.
    renderImport(PLAIN);
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
    const input = screen.getByTestId('canvas-svg-import-file');
    const assigned: string[] = [];
    Object.defineProperty(input, 'value', {
      configurable: true,
      get: () => '',
      set: (next: string) => assigned.push(next),
    });

    fireEvent.change(input, { target: { files: [file(PLAIN)] } });
    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-summary')).toBeTruthy());
    // 브라우저는 같은 파일을 다시 고를 때 값이 그대로면 `change` 를 내지 않는다.
    expect(assigned).toContain('');
  });
});

// --- 미리보기 --------------------------------------------------------------

describe('미리보기는 실제 렌더 경로다 (§도크 UI)', () => {
  it('캔버스 종횡비를 지킨 상자를 세운다 — 눌린 미리보기를 내지 않는다', async () => {
    renderImport(PLAIN);
    await choose(file(PLAIN));
    const node = screen.getByTestId('canvas-svg-import-preview') as HTMLCanvasElement;
    const size = fitPreviewSize(DEFAULT_CANVAS_SIZE);
    expect(node.style.width).toBe(`${size.width}px`);
    expect(node.style.height).toBe(`${size.height}px`);
    // 뒷면은 CSS 크기의 정수배다 — 배율이 1 이면 이 단언이 아무것도 재지 않는다.
    expect(node.width).toBe(size.width * 2);
    expect(node.height).toBe(size.height * 2);
    // 그림은 이름을 나르지 않는다 — 이름은 요약 한 줄에 있다.
    expect(node.getAttribute('aria-hidden')).toBe('true');
  });

  it('미리보기가 그린 것이 **놓았을 때 그려질 것과 한 명령도 다르지 않다** (층 건너기)', async () => {
    // 두 층을 따로 재면 이음매가 덮이지 않는다 — 이 저장소가 편집기↔렌더러에서 이미 물린
    // 부류다. 여기서는 **화면이 그린 기록**과 **놓인 산출로 다시 그린 기록**을 견준다.
    const seen: Call[] = [];
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      () => makeRecorder(seen) as unknown as CanvasRenderingContext2D,
    );

    const doc = svg(
      '<rect x="5" y="12" width="40" height="20" fill="#c0392b"/>' +
        '<path d="M 60 30 L 90 30 L 90 50" stroke="#145a32" stroke-width="3" fill="none"/>',
    );
    const { placed } = renderImport(doc);
    await choose(file(doc));

    // 켜져 있음을 먼저 단언한다 — 기록이 비면 아래 비교가 `[] === []` 가 된다.
    expect(seen.length).toBeGreaterThan(4);
    expect(seen.some((c) => c[0] === 'fill' && c[1] === '#c0392b')).toBe(true);
    expect(seen.some((c) => c[0] === 'stroke' && c[1] === '#145a32')).toBe(true);

    fireEvent.click(screen.getByTestId('canvas-svg-import-place'));
    const { shapes } = placed[0]!;

    // 놓인 산출로 **같은 투영에** 다시 그린다.
    const size = fitPreviewSize(DEFAULT_CANVAS_SIZE);
    const projection: CanvasProjection = {
      stage: { width: size.width * 2, height: size.height * 2 },
      canvas: DEFAULT_CANVAS_SIZE,
    };
    const again: Call[] = [];
    const ctx = makeRecorder(again) as unknown as DrawContext2D;
    clearSurface(ctx, { ...projection.stage, scale: 2 });
    drawElements(ctx, appendImportedElements([], shapes).created, {}, {}, projection);

    expect(seen).toEqual(again);
  });
});

// --- 고르지 않은 채 닫기 ---------------------------------------------------

describe('파일 고르기를 취소하면 아무 일도 일어나지 않는다', () => {
  it('빈 목록으로 `change` 가 와도 상태가 대기 그대로다', async () => {
    // 브라우저의 파일 대화상자를 열고 [취소] 를 누르면 이 형상이 온다. 이 가지가 없으면
    // `undefined` 가 아래로 흘러 읽기 함수에 닿는다.
    const read = vi.fn(async () => PLAIN);
    render(
      <I18nProvider>
        <CanvasSvgImport canvas={DEFAULT_CANVAS_SIZE} onPlace={() => {}} readFile={read} />
      </I18nProvider>,
    );
    fireEvent.click(screen.getByTestId('canvas-svg-import-group'));
    fireEvent.change(screen.getByTestId('canvas-svg-import-file'), { target: { files: [] } });

    await waitFor(() => expect(screen.getByTestId('canvas-svg-import-pick')).toBeTruthy());
    expect(read).not.toHaveBeenCalled();
    expect(screen.queryByTestId('canvas-svg-import-summary')).toBeNull();
    expect(screen.queryByTestId('canvas-svg-import-refused')).toBeNull();
  });
});
