// 들어가지 않은 그릇이 **말없이** 하위 트리를 삼키지 않는다 (SPEC-CANVAS-007 REQ-04).
//
// **이 SPEC 이 같은 꼴로 네 번 물렸다** — 읽기는 읽되 아무도 적용하지 않고 아무도 보고하지
// 않는 자리. `#id` 선택자 · 붙여 쓴 복합 선택자 · 적용한 규칙을 "버렸다" 고 말하던 보고,
// 그리고 여기. 앞의 셋과 달리 이 자리는 **파싱조차 하지 않는다**: `walk` 의 `default:` 가지와
// 미지 네임스페이스 가지가 조용히 `return` 하므로, 그 요소 **안의 도형과 글자 전부**가
// 그림에서도 보고에서도 사라진다.
//
// 사용자가 실제로 본 것이 그 조합이다 — 도형은 들어왔고, 글자는 하나도 없으며, 보고의
// 버림 칸은 **비어 있다**. 셋 다 참인 경로는 "파서가 글자를 아예 보지 못했다" 하나뿐이다.
//
// **이 파일은 보고만 재지 않는다.** 파싱 단언이 적용을 증명하지 못한다는 것이 이 SPEC 의
// 학습이므로, 여기서도 **문서 → 계획 → 요소 → 그린 기록** 끝까지 몰아 두 가지를 함께
// 읽는다: (1) 삼킨 하위 트리가 실제로 화면에 없다(보고가 거짓말이 아니다), (2) 그 사실이
// 보고에 오른다.
//
// **잡음이 아님을 함께 잰다(위험 R7).** 사실상 모든 가져오기에서 울리는 보고는 정보가 없고,
// 그런 보고는 읽히지 않으며, 읽히지 않는 보고는 침묵과 같다. 아래 "조용한 자리" 무리가
// 그 성질을 고정한다 — 특히 잉크스케이프가 **거의 모든 파일에** 내보내는
// `<sodipodi:namedview><inkscape:grid/></sodipodi:namedview>` 가 울리지 않아야 한다.
//
// @spec SPEC-CANVAS-007 REQ-02 · REQ-04

import { describe, expect, it } from 'vitest';

import { DEFAULT_CANVAS_SIZE } from '../canvasConfig';
import { appendImportedElements } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import { drawElements, type DrawContext2D } from '../drawElement';
import { planSvgImport } from './svgImportPlan';

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 · 안 나누어떨어짐 (시험 규율 E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

const NS = [
  'xmlns="http://www.w3.org/2000/svg"',
  'xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"',
  'xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"',
].join(' ');

function svg(body: string): string {
  return `<svg ${NS} ${VIEW_BOX}>${body}</svg>`;
}

/** 삼켜지지 않는 기준 도형. 이것이 없으면 문서가 `emptyDocument` 로 거절된다. */
const ANCHOR = '<rect x="10" y="20" width="40" height="30" fill="#c81e1e"/>';

const PROJECTION: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

/** 한 번의 그리기가 남긴 자국. 글자는 **내용까지** 든다 — 삼켰는지를 이름으로 읽는다. */
type Paint =
  | { readonly op: 'fill' | 'stroke'; readonly color: string }
  | { readonly op: 'fillText'; readonly color: string; readonly text: string };

function recorder(paints: Paint[]): DrawContext2D {
  return {
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {},
    rect() {},
    ellipse() {},
    moveTo() {},
    lineTo() {},
    closePath() {},
    bezierCurveTo() {},
    stroke() {
      paints.push({ op: 'stroke', color: String(this.strokeStyle) });
    },
    fill() {
      paints.push({ op: 'fill', color: String(this.fillStyle) });
    },
    fillText(text: string) {
      paints.push({ op: 'fillText', color: String(this.fillStyle), text });
    },
    measureText(text: string) {
      return { width: text.length * 10 };
    },
    clearRect() {},
    fillRect() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'middle',
  };
}

interface Run {
  /** 보고에 오른 사유들. 문자열로 읽어 **합집합이 넓어지기 전에도** 이 파일이 컴파일된다. */
  readonly reasons: readonly string[];
  readonly notes: ReadonlyArray<{ readonly kind: string; readonly reason: string; readonly count: number }>;
  readonly paints: readonly Paint[];
  readonly drawnTexts: readonly string[];
}

/** 문서를 **끝까지** 몬다 — 계획 · 요소 · 실제로 그린 기록을 함께 돌려준다. */
function run(body: string): Run {
  const plan = planSvgImport(svg(body), { ...DEFAULT_CANVAS_SIZE });
  if (!plan.ok) throw new Error(`고정 입력이 거절되었다: ${plan.refusal.reason}`);
  const { next } = appendImportedElements([], plan.shapes, plan.texts);
  const paints: Paint[] = [];
  drawElements(recorder(paints), next, {}, {}, PROJECTION);
  const notes = plan.report.notes.map((n) => ({
    kind: n.kind as string,
    reason: n.reason as string,
    count: n.count,
  }));
  return {
    reasons: notes.map((n) => n.reason),
    notes,
    paints,
    drawnTexts: paints.flatMap((p) => (p.op === 'fillText' ? [p.text] : [])),
  };
}

/** 사유 하나의 개수. 없으면 0 — "없음" 과 "0개" 를 한 자리에서 읽는다. */
function countOf(r: Run, reason: string): number {
  return r.notes.filter((n) => n.reason === reason).reduce((sum, n) => sum + n.count, 0);
}

const REASON = 'unenteredContainerDropped';

// --- 구멍 그 자체 ---------------------------------------------------------

describe('들어가지 않은 그릇의 하위 트리는 **버림으로 보고된다**', () => {
  it('`<switch>` 안의 글자는 그려지지 않고, 그 사실이 보고에 오른다 (사용자가 본 증상)', () => {
    const r = run(`${ANCHOR}<switch><text x="30" y="40" fill="#111111">라벨</text></switch>`);

    // (1) 기준 도형은 들어왔다 — "아무것도 안 들어왔다" 와 구분한다.
    expect(r.paints.some((p) => p.op === 'fill' && p.color === '#c81e1e')).toBe(true);
    // (2) 글자는 화면에 **없다**. 보고가 거짓말이 아님을 여기서 고정한다.
    expect(r.drawnTexts).toEqual([]);
    // (3) 그리고 그 사실이 말해진다 — 이 줄이 고치기 전의 침묵을 잡는다.
    expect(r.reasons).toContain(REASON);
    expect(countOf(r, REASON)).toBe(1);
    expect(r.notes.find((n) => n.reason === REASON)?.kind).toBe('dropped');
  });

  it('모르는 SVG 요소가 도형을 감싸도 같다', () => {
    const r = run(`${ANCHOR}<solidColorSwatch><rect x="5" y="5" width="9" height="9" fill="#0ea5e9"/></solidColorSwatch>`);

    expect(r.paints.some((p) => p.op === 'fill' && p.color === '#0ea5e9')).toBe(false);
    expect(countOf(r, REASON)).toBe(1);
  });

  it('미지 네임스페이스의 그릇도 같다 — 삼키는 문은 둘이고 증상은 하나다', () => {
    const r = run(`${ANCHOR}<inkscape:layerGroup><text x="30" y="40">라벨</text></inkscape:layerGroup>`);

    expect(r.drawnTexts).toEqual([]);
    expect(countOf(r, REASON)).toBe(1);
  });

  it('그릇 둘이면 2개, 겹쳐 있으면 1개다 — 바깥에서 끊으므로 겹쳐 세지 않는다', () => {
    const two = run(`${ANCHOR}<switch><text x="1" y="2">가</text></switch><switch><text x="3" y="4">나</text></switch>`);
    expect(countOf(two, REASON)).toBe(2);

    const nested = run(`${ANCHOR}<switch><unknownBox><text x="1" y="2">가</text></unknownBox></switch>`);
    expect(countOf(nested, REASON)).toBe(1);
  });
});

// --- "그렸을 것" 의 목록 ---------------------------------------------------
//
// 아래 둘은 **변이가 살아남아** 더해진 시험이다. `isDrawableTag` 에서 `path` 를 빼거나
// `use`·`image`·`foreignObject`·중첩 `svg` 를 빼도 위의 시험 전량이 초록이었다 — 도형과
// 글자만 재고 있었기 때문이다. 목록의 칸마다 한 줄씩 둔다.

describe('"그렸을 것" 의 목록은 도형과 글자만이 아니다', () => {
  it('경로 하나만 품어도 보고에 오른다', () => {
    const r = run(`${ANCHOR}<switch><path d="M0 0 L10 10 Z" fill="#0ea5e9"/></switch>`);
    expect(r.paints.some((p) => p.op === 'fill' && p.color === '#0ea5e9')).toBe(false);
    expect(countOf(r, REASON)).toBe(1);
  });

  it('제 사유로 보고되었을 것들(use · image · foreignObject · 중첩 svg)도 센다', () => {
    // 이것들은 들어갔더라면 **버림으로 보고되었을** 것이다. 그 보고까지 삼킨 것도 잃은 것이다
    // — 세지 않으면 "중첩 svg 가 있었다" 는 사실이 화면에서 영영 사라진다.
    const inner = [
      '<use href="#nowhere"/>',
      '<image href="a.png" width="4" height="4"/>',
      '<foreignObject width="4" height="4"/>',
      '<svg width="4" height="4"/>',
    ];
    for (const child of inner) {
      expect(countOf(run(`${ANCHOR}<madeUpTag>${child}</madeUpTag>`), REASON), child).toBe(1);
    }
  });

  it('이름만 같은 미지 네임스페이스 자식은 세지 않는다 — `sodipodi:rect` 는 사각형이 아니다', () => {
    // 네임스페이스 검사를 지운 변이가 여기서 죽는다. 검사가 없으면 이 문서가 울리고,
    // 그 보고는 **잃지 않은 것을 잃었다고 말한다**.
    const r = run(`${ANCHOR}<madeUpTag><sodipodi:rect width="4" height="4"/></madeUpTag>`);
    expect(countOf(r, REASON)).toBe(0);
  });
});

// --- 조용한 자리 (위험 R7) ------------------------------------------------

describe('잃은 것이 없으면 **말하지 않는다** (위험 R7)', () => {
  it('요소 자식이 없는 미지 요소는 잎이다 — 글자 노드도 주석도 요소가 아니다', () => {
    expect(countOf(run(`${ANCHOR}<madeUpTag>이것은 글자 노드다</madeUpTag>`), REASON)).toBe(0);
    expect(countOf(run(`${ANCHOR}<madeUpTag><!-- 주석 --></madeUpTag>`), REASON)).toBe(0);
    expect(countOf(run(`${ANCHOR}<madeUpTag/>`), REASON)).toBe(0);
  });

  it('잉크스케이프가 거의 모든 파일에 내보내는 `namedview` 는 울리지 않는다', () => {
    // 이 한 줄이 R7 의 시금석이다. 여기서 울리면 보고가 사실상 모든 가져오기에서 한 줄을
    // 더 내고, 그 줄은 정보가 없다.
    const r = run(
      `${ANCHOR}<sodipodi:namedview><inkscape:grid type="xygrid"/><sodipodi:guide position="0,0"/></sodipodi:namedview>`,
    );
    expect(countOf(r, REASON)).toBe(0);
  });

  it('요소 자식이 있어도 **그릴 것이 아니면** 울리지 않는다', () => {
    expect(countOf(run(`${ANCHOR}<madeUpTag><desc>설명</desc><title>이름</title></madeUpTag>`), REASON)).toBe(0);
  });

  it('그릴 것이 `<defs>` 안에 있으면 울리지 않는다 — 들어갔어도 그리지 않았을 것이다', () => {
    const r = run(`${ANCHOR}<madeUpTag><defs><rect id="d" width="4" height="4"/></defs></madeUpTag>`);
    expect(countOf(r, REASON)).toBe(0);
  });

  it('감춘 하위 트리 안에서는 울리지 않는다 — 그리지 않기로 한 그림의 목록이다', () => {
    const none = run(`${ANCHOR}<g display="none"><switch><rect width="4" height="4"/></switch></g>`);
    expect(countOf(none, REASON)).toBe(0);

    const hiddenNs = run(
      `${ANCHOR}<g visibility="hidden"><inkscape:box><rect width="4" height="4"/></inkscape:box></g>`,
    );
    expect(countOf(hiddenNs, REASON)).toBe(0);
  });

  it('`visibility="visible"` 로 되살린 자리에서는 다시 울린다 — 감춤은 뒤집힌다', () => {
    const r = run(
      `${ANCHOR}<g visibility="hidden"><g visibility="visible"><switch><rect width="4" height="4"/></switch></g></g>`,
    );
    expect(countOf(r, REASON)).toBe(1);
  });
});
