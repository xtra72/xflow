// 붙여 쓴 복합 선택자가 **그림에 닿는다** (SPEC-CANVAS-007 REQ-03).
//
// **파싱 단언은 이 자리를 재지 못한다.** `rect.cls-2` 가 규칙표에 칸을 얻는지를 묻는 시험은
// 그 칸을 아무도 열지 않아도 초록이고, 이 SPEC 에서 그 형상으로 배포를 살아남은 구멍이 둘
// 있었다(`#id` 구멍 · 붙여 쓴 복합 구멍). 그래서 여기서는 **문서 → 계획 → 요소 → 그린 기록**
// 끝까지 몰아, 화면에 실제로 오른 색을 읽는다.
//
// **씨앗 색(`#3b82f6`)과 다른 색만 쓴다.** 같은 색을 쓰면 "규칙이 닿았는가" 와 "닿지 않아
// 씨앗이 섰는가" 를 구분할 수 없다. 칠하지 않은 도형은 이 층 아래에서 `fill` 이 **아예 없고**
// (`hasOwnStyle === false`), 씨앗은 `canvasElementFactory` 가 뒤에서 세운다 — 그래서 씨앗
// 색이 기록에 보이면 그것은 "규칙이 닿지 않았다" 는 뜻이다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04

import { describe, expect, it } from 'vitest';

import { DEFAULT_CANVAS_SIZE } from '../canvasConfig';
import { appendImportedElements, SEED_COLOR } from '../canvasElementFactory';
import type { CanvasProjection } from '../canvasGeometry';
import { drawElements, type DrawContext2D } from '../drawElement';
import { planSvgImport } from './svgImportPlan';

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 · 안 나누어떨어짐 (시험 규율 E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

function svg(body: string): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}>${body}</svg>`;
}

const PROJECTION: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { ...DEFAULT_CANVAS_SIZE },
};

/** 한 번의 그리기가 남긴 자국 — 무엇을 어떤 색으로 칠했는가. */
type Paint = { readonly op: 'fill' | 'stroke' | 'fillText'; readonly color: string };

/**
 * `drawElements` 가 실제로 부른 칠하기만 모으는 기록 context.
 *
 * 좌표는 모으지 않는다 — 이 파일이 묻는 것은 **색** 하나이고, 좌표까지 모으면 고정 입력의
 * 기하를 조금만 건드려도 시험이 빨개져 재려는 성질이 좌표 회귀에 묻힌다.
 */
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
    fillText() {
      paints.push({ op: 'fillText', color: String(this.fillStyle) });
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

/** 문서를 끝까지 몰아 **화면에 오른 칠**을 순서대로 돌려준다. */
function painted(doc: string): Paint[] {
  const plan = planSvgImport(doc, { ...DEFAULT_CANVAS_SIZE });
  if (!plan.ok) throw new Error(`고정 입력이 거절되었다: ${plan.refusal.reason}`);
  const { next } = appendImportedElements([], plan.shapes, plan.texts);
  const paints: Paint[] = [];
  drawElements(recorder(paints), next, {}, {}, PROJECTION);
  return paints;
}

/** 채움 색만, 그린 순서대로. */
function fills(doc: string): string[] {
  return painted(doc)
    .filter((p) => p.op === 'fill' || p.op === 'fillText')
    .map((p) => p.color);
}

// --- 일러스트레이터 · 잉크스케이프가 실제로 뱉는 꼴 -------------------------

describe('`tag.class` 꼴 복합 선택자가 그림에 닿는다 (결함 C)', () => {
  // 네 규칙 가운데 하나만 홑마디다 — 홑마디가 섞여 있어야 "복합만 닿지 않는다" 와
  // "아무것도 닿지 않는다" 가 갈린다.
  const SHEET =
    '<style>' +
    '.cls-1{fill:#c0392b;}' +
    'rect.cls-2{fill:#2980b9;}' +
    'text.t{fill:#145a32;}' +
    'polygon.p{fill:#8e44ad;}' +
    '</style>';
  const BODY =
    '<rect class="cls-1" x="5" y="12" width="40" height="20"/>' +
    '<rect class="cls-2" x="60" y="12" width="40" height="20"/>' +
    '<polygon class="p" points="5 60 45 60 45 90"/>' +
    '<text class="t" x="10" y="120">글자</text>';

  it('네 도형이 모두 자기 규칙의 색으로 칠해진다 — 씨앗 색이 하나도 없다', () => {
    const seen = fills(svg(SHEET + BODY));
    // 켜져 있음을 먼저 못박는다 — 기록이 비면 아래 비교가 `[] === []` 가 된다.
    expect(seen).toHaveLength(4);
    expect(seen).toEqual(['#c0392b', '#2980b9', '#8e44ad', '#145a32']);
    expect(seen).not.toContain(SEED_COLOR);
  });

  it('홑마디만으로 이루어진 시트는 고치기 전과 한 색도 다르지 않다 (회귀)', () => {
    const doc = svg(
      '<style>.cls-1{fill:#c0392b;}rect{fill:#2980b9;}#solo{fill:#8e44ad;}</style>' +
        '<rect class="cls-1" x="5" y="12" width="40" height="20"/>' +
        '<rect x="60" y="12" width="40" height="20"/>' +
        '<rect id="solo" x="110" y="12" width="40" height="20"/>',
    );
    expect(fills(doc)).toEqual(['#c0392b', '#2980b9', '#8e44ad']);
  });
});
