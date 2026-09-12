// 가져오기 **전체 사슬**의 시험 — 파싱부터 실제로 그려질 때까지.
//
// **왜 층마다의 시험으로는 모자란가.** 파싱 층도, 배치 층도, 설정 층도, 렌더 층도 저마다
// 초록이면서 사슬이 끊어져 있을 수 있다. 실제로 그랬다: draw.io 의 이름표가 요소로는
// 서는데 화면에 글자가 없다는 보고가 들어왔고, 층마다 열어 보는 동안 어느 층도 빨개지지
// 않았다. 이 파일은 그 자리를 덮는다 — **SVG 문자열 한 개를 넣고 `fillText` 에 무엇이
// 닿는지**까지 본다.
//
// 사슬: `planSvgImport` → `appendImportedElements` → `parseNodes`(설정 왕복) →
// `drawElements`. 한 마디라도 끊어지면 여기서 빨개진다.
import { describe, expect, it } from 'vitest';

import { parseNodes } from '../canvasConfig';
import { appendImportedElements } from '../canvasElementFactory';
import { drawElements } from '../drawElement';

import { planSvgImport } from './svgImportPlan';

/** draw.io 가 실제로 뱉는 이름표 한 개 + 상자 한 개. */
const DRAWIO = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" width="501px" height="600px" viewBox="0 0 501 600">
<defs/>
<g><g data-cell-id="0"><g data-cell-id="1">
<g data-cell-id="A">
<g><rect x="40" y="120" width="60" height="40" fill="none" stroke="none" pointer-events="all"/></g>
<g><g><switch>
<foreignObject style="overflow: visible; text-align: left;" pointer-events="none" width="100%" height="100%" requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility">
<div xmlns="http://www.w3.org/1999/xhtml" style="display: flex; align-items: unsafe center; justify-content: unsafe flex-start; width: 58px; height: 1px; padding-top: 140px; margin-left: 42px;">
<div style="box-sizing: border-box; font-size: 0; text-align: left; color: #000000; ">
<div style="display: inline-block; font-size: 12px; font-family: Helvetica; color: light-dark(#000000, #ffffff); line-height: 1.2; font-weight: bold; white-space: normal; word-wrap: normal; ">크기</div>
</div></div></foreignObject>
<image x="42" y="133" width="58" height="17" xlink:href="data:image/png;base64,iVBORw0KGgo="/>
</switch></g></g>
</g>
<g data-cell-id="B"><g transform="translate(0.5,0.5)"><rect x="80" y="80" width="80" height="40" rx="6" ry="6" fill="#f5f5f5" stroke="#666666" pointer-events="all" style="fill: light-dark(rgb(245, 245, 245), rgb(26, 26, 26)); stroke: light-dark(rgb(102, 102, 102), rgb(149, 149, 149));"/></g></g>
</g></g></g></svg>`;

const CANVAS = { width: 800, height: 600 } as const;

/** `fillText` 와 `fillStyle` 만 들여다보는 최소 컨텍스트. */
function recordingContext(): {
  ctx: unknown;
  drawn: string[];
  fills: string[];
} {
  const drawn: string[] = [];
  const fills: string[] = [];
  const ctx = new Proxy({} as Record<string, unknown>, {
    get(_t, prop: string) {
      if (prop === 'measureText') return () => ({ width: 30 });
      if (prop === 'fillText') return (s: string) => void drawn.push(s);
      return () => undefined;
    },
    set(_t, prop: string, value: unknown) {
      if (prop === 'fillStyle' && typeof value === 'string') fills.push(value);
      return true;
    },
  });
  return { ctx, drawn, fills };
}

function place(): ReturnType<typeof appendImportedElements> {
  const outcome = planSvgImport(DRAWIO, CANVAS);
  if (!outcome.ok) throw new Error('가져오기가 거절되었다 — 사슬의 첫 마디가 끊겼다');
  return appendImportedElements([], outcome.shapes, outcome.texts);
}

describe('가져오기 사슬', () => {
  it('draw.io 의 이름표가 요소로 선다 — 도형 하나 + 글자 하나', () => {
    const made = place();
    // 히트 영역 사각형(`fill="none" stroke="none"`)은 요소가 되지 않는다.
    expect(made.created).toHaveLength(1);
    expect(made.createdTexts).toHaveLength(1);
    expect(made.createdTexts[0]?.text).toBe('크기');
  });

  it('설정에 저장됐다 돌아와도 글자가 남는다', () => {
    const made = place();
    const back = parseNodes(JSON.parse(JSON.stringify(made.next)) as unknown);
    expect(back.filter((n) => n.kind === 'text')).toHaveLength(1);
    expect(back).toHaveLength(made.next.length);
  });

  it('실제로 그려진다 — fillText 에 그 글자가 닿는다', () => {
    const made = place();
    const { ctx, drawn } = recordingContext();
    drawElements(ctx as never, made.next, {}, {}, { stage: CANVAS, canvas: CANVAS });
    expect(drawn).toEqual(['크기']);
  });

  it('원본 색으로 칠한다 — light-dark() 뒤의 색을 씨앗으로 바꾸지 않는다', () => {
    const made = place();
    const { ctx, fills } = recordingContext();
    drawElements(ctx as never, made.next, {}, {}, { stage: CANVAS, canvas: CANVAS });
    // `#f5f5f5` 는 표현 속성이 아니라 `style="fill: light-dark(...)"` 를 푼 값이다.
    expect(fills).toContain('rgb(245, 245, 245)');
    expect(fills).not.toContain('#3b82f6'); // 씨앗 파랑
  });
});
