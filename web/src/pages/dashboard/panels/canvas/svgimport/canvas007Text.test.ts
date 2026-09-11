// `<text>` 가 요소로 들어온다 — 문서 → 계획 → 요소 → 렌더 (SPEC-CANVAS-007 · 결함 B 정정).
//
// **재현이 먼저였다.** 고치기 전의 측정: `<text>` 둘이 든 문서에서
// `readSvgDocument(...).notes` 는 `[{kind:'dropped',reason:'textDropped',count:2}]` 였고
// `appendImportedElements(...).created` 의 `kind` 는 `['path']` 뿐이었다. 사용자가 본 것이
// "텍스트 출력안됨. 컴포넌트는 있음" 이며, 그 자리는 캔버스가 아니라 가져오기였다.
//
// **이 파일이 층을 건너는 이유.** 각 층은 제 시험에서 100% 초록일 수 있고 그 사이가 비어
// 있을 수 있다 — 이 저장소가 편집기↔렌더러 이음매에서 이미 물린 부류다. 그래서 아래
// "이음매" 묶음은 문서 문자열 하나에서 시작해 **`fillText` 가 실제로 불리는 것**까지 간다.
//
// **고정 입력이 피하는 함정 다섯.**
//   - `text-anchor` 가 **없는** 문구를 함께 둔다(정렬 대응이 잠들지 않게).
//   - 문구가 **문서 원점에 있지 않다**(평행이동이 잠들지 않게).
//   - `viewBox` 의 `minX`·`minY` 가 **0 이 아니다**(원점 빼기가 잠들지 않게).
//   - 축척이 **1 이 아니다**(곱셈이 잠들지 않게).
//   - 글자가 **ASCII 가 아니다**.
//
// **확인한 뮤테이션 31종 — 전부 물었다**(실제로 갈아 넣고 돌린 결과다. 둘은 처음에
// 살아남았고, 그 둘이 고정 입력의 구멍을 가리켜 시험을 강화했다 — 아래 13·22).
//
//   문서 층
//   1. `<text>` 가지를 감춤 판정 **뒤로** 옮기기 → "감춘 글자는 개수로만 센다" 가 빨개진다.
//   2. `hasTextPathChild` 검사 삭제 → textPath 시험 둘이 빨개진다.
//   3. `hasTextPathChild` 의 재귀 삭제(자식 한 겹만) → "한 겹 더 싸인 textPath" 가 빨개진다.
//   4. `strokePainted` 갈래를 지우고 언제나 감춤으로 → "테가 있으면 버림" 이 빨개진다.
//   5. `applyMatrix` 삭제 → "조상 transform 이 기준점에 녹는다" 가 빨개진다.
//   6. `overtakenTexts = texts.length` → `+=` → "그때까지의 문구 수" 가 빨개진다(수가 부푼다).
//  12. `TSPAN_HARMLESS_ATTRS` 를 빈 집합으로 / `'id'` 만 빼기 / `'xml:space'` 만 빼기 —
//      **셋이 각각** 빨개진다.
//  13. `countOwnTspans` 의 재귀 삭제 → **처음에는 살아남았다**. 중첩 `<tspan>` 고정 입력이
//      없었기 때문이며, 도구는 실제로 한 겹 더 싸서 낸다. 그 고정 입력을 더해 강화했다.
//  14. `textFontIgnored` 발행 삭제 → "말한 글꼴을 옮기지 못했다고 말한다" 가 빨개진다.
//  15. CSS 규칙의 활자(`pickText`) 삭제 → "class 규칙이 활자에 걸린다" 가 빨개진다.
//  16. 활자 상속 삭제 → "조상 <g> 의 font-size 가 내려온다" 가 빨개진다.
//  17. `xml:space` 상속 삭제 → "루트의 preserve 가 아래로 흐른다" 가 빨개진다.
//  18. 빈 글자 건너뛰기 삭제 → "빈 글자는 요소도 보고도 만들지 않는다" 가 빨개진다.
//  19. `textTruncated` 발행 삭제 → "상한을 넘은 글자를 자르고 말한다" 가 빨개진다.
//  20. `textTemplateToken` 발행 삭제 → "치환할 토큰이 든 글자를 말한다" 가 빨개진다.
//  21. `tspanDropped` 발행 삭제 → tspan 시험이 빨개진다.
//  22. 퇴화 변환 가드 삭제 → **처음에는 살아남았다**. `scale(0)` 고정 입력이 없었기
//      때문이며, 가드가 없으면 한 점으로 찌부러진 자리에 6px 짜리 글자가 선다. 강화했다.
//  15. 기준점을 `parseLength` 로 읽기(목록 이전의 형상) → "x 가 목록이면 첫 수에 선다" 가
//      빨개진다. 16. 글자마다의 자리를 보고하지 않기 → 그 보고 시험이 빨개진다.
//  17. `y` 를 `x` 에서 읽기(축 뒤바꿈) → 자리 시험 넷이 빨개진다.
//
//   계획 층
//   7. `planTexts` 의 `* ratio` 삭제 → "크기가 문서 축척을 탄다" 가 빨개진다.
//   8. 요소 상한에서 문구 수 빼기 → "문구도 요소 상한을 먹는다" 둘이 빨개진다.
//   9. `estimateBytes` 의 문구 순회 삭제 → "추정이 문구의 글자를 센다" 가 빨개진다.
//  26. 추정을 다시 `.length`(UTF-16)로 → "UTF-8 바이트로 센다" 가 빨개진다.
//  23. 죔 보고 삭제 / 죔 자체 삭제 → "범위를 넘으면 죄고 말한다" 가 각각 빨개진다.
//  24. `placePoint` 를 지나지 않고 사용자 좌표를 그대로 쓰기 → 자리 시험 셋이 빨개진다.
//
//   요소 층
//  10. 문구를 도형 **앞**에 붙이기 → "문구가 도형 위에 온다" 를 포함해 셋이 빨개진다.
//  11. 문구 씨앗 색 삭제 → "말하지 않은 글자도 그려진다" 를 포함해 다섯이 빨개진다.
//  25. 문구에 제 계단 오프셋 주기 → "계단은 한 번만" 을 포함해 셋이 빨개진다.

import { describe, expect, it } from 'vitest';

import { DEFAULT_CANVAS_SIZE, DEFAULT_FONT_SIZE, type CanvasElement } from '../canvasConfig';
import { CANVAS_FONT_SIZE_MAX, CANVAS_FONT_SIZE_MIN } from '../canvasEditGeometry';
import {
  SEED_COLOR,
  appendElement,
  appendImportedElements,
  seedOffset,
} from '../canvasElementFactory';
import { drawElements, type DrawContext2D } from '../drawElement';
import type { CanvasProjection } from '../canvasGeometry';

import { readSvgDocument } from './svgDocument';
import { estimateBytes, planSvgImport } from './svgImportPlan';
import { MAX_IMPORT_ELEMENTS, MAX_IMPORT_TEXT_LENGTH, type ImportNote } from './svgImportTypes';

/** `minX`·`minY` 가 0 이 아니고 두 축의 길이가 다르다 — 원점 빼기와 축 뒤바꿈이 함께 걸린다. */
const VIEW_BOX = 'viewBox="20 10 200 100"';

function svg(body: string, rootAttrs = VIEW_BOX): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${rootAttrs}>${body}</svg>`;
}

function read(text: string) {
  const outcome = readSvgDocument(text);
  if (!outcome.ok) throw new Error(`거절됨: ${outcome.refusal.reason}`);
  return outcome.document;
}

function plan(text: string, canvas = DEFAULT_CANVAS_SIZE) {
  const outcome = planSvgImport(text, canvas);
  if (!outcome.ok) throw new Error(`거절됨: ${outcome.refusal.reason}`);
  return outcome;
}

function reasonCount(notes: readonly ImportNote[], reason: string): number {
  return notes.find((n) => n.reason === reason)?.count ?? 0;
}

// --- 문서 층 ---------------------------------------------------------------

describe('`<text>` 는 버림이 아니라 문구가 된다 (결함 B)', () => {
  it('글자가 들어오고 버림 보고가 서지 않는다 — 고치기 전의 측정과 정확히 반대다', () => {
    const doc = read(svg('<text x="30" y="40">회의실 A</text>'));
    expect(doc.texts).toHaveLength(1);
    expect(doc.texts[0]?.text).toBe('회의실 A');
    expect(reasonCount(doc.notes, 'textDropped')).toBe(0);
  });

  it('조상 transform 이 기준점에 녹는다 (뮤테이션 5)', () => {
    const doc = read(svg('<g transform="translate(7 3) scale(2)"><text x="30" y="40">가</text></g>'));
    // (30,40) → scale 2 → (60,80) → translate(7,3) → (67,83).
    expect(doc.texts[0]).toMatchObject({ x: 67, y: 83 });
  });

  it('x·y 가 없으면 0 이다 — 그 값이 사양의 초기값이다', () => {
    expect(read(svg('<text>가</text>')).texts[0]).toMatchObject({ x: 0, y: 0 });
  });

  it('x 가 목록이면 첫 수에 선다 — 0 으로 떨어지지 않는다 (뮤테이션 15)', () => {
    // 잉크스케이프가 자간을 준 글자에 `x="10 20 30"` 을 낸다. `parseLength` 는 그 목록에서
    // `undefined` 를 내므로(실측), 그대로 쓰면 그 문서의 글자가 전부 문서 원점에 쌓인다.
    const doc = read(svg('<text x="30 40 50" y="40">가나다</text>'));
    expect(doc.texts[0]).toMatchObject({ x: 30, y: 40 });
  });

  it('글자마다 준 자리를 버렸다고 말한다 (뮤테이션 16)', () => {
    expect(reasonCount(read(svg('<text x="30 40" y="40">가나</text>')).notes, 'textFontIgnored')).toBe(1);
    // 수 하나짜리는 버린 것이 없다 — 보고가 잡음이 되지 않는다.
    expect(reasonCount(read(svg('<text x="30" y="40">가나</text>')).notes, 'textFontIgnored')).toBe(0);
  });

  it('변환의 배율이 글자 크기에 곱해진다', () => {
    const doc = read(svg('<g transform="scale(3)"><text font-size="10">가</text></g>'));
    expect(doc.texts[0]?.fontSizeUserUnits).toBe(30);
  });

  it('빈 글자와 공백뿐인 글자는 요소도 보고도 만들지 않는다 — SVG 도 그리지 않는다', () => {
    const doc = read(svg('<text x="1" y="2"></text><text x="3" y="4">   </text>'));
    expect(doc.texts).toHaveLength(0);
    expect(doc.notes).toEqual([]);
  });
});

describe('상속과 CSS 가 활자에 걸린다', () => {
  it('조상 <g> 의 font-size 와 text-anchor 가 내려온다', () => {
    const doc = read(
      svg('<g font-size="40" text-anchor="middle"><text x="30" y="40">가</text></g>'),
    );
    expect(doc.texts[0]?.fontSizeUserUnits).toBe(40);
    expect(doc.texts[0]?.style.align).toBe('center');
  });

  it('자기 값이 물려받은 값을 이긴다', () => {
    const doc = read(
      svg('<g font-size="40" text-anchor="middle"><text font-size="12" text-anchor="end">가</text></g>'),
    );
    expect(doc.texts[0]?.fontSizeUserUnits).toBe(12);
    expect(doc.texts[0]?.style.align).toBe('right');
  });

  it('상속되지 않는 것은 내려오지 않는다 — dominant-baseline 은 조상에 머문다', () => {
    // 내려온다면 아래 글자가 `textFontIgnored` 로 보고될 것이다.
    const doc = read(svg('<g dominant-baseline="hanging"><text>가</text></g>'));
    expect(reasonCount(doc.notes, 'textFontIgnored')).toBe(0);
  });

  it('<style> 의 class 규칙이 활자에 걸린다 — 칠과 같은 길을 탄다', () => {
    const doc = read(
      svg('<style>.big{font-size:40;fill:#145a32}</style><text class="big">가</text>'),
    );
    expect(doc.texts[0]?.fontSizeUserUnits).toBe(40);
    expect(doc.texts[0]?.style.textColor).toBe('#145a32');
  });

  it('인라인 속성이 CSS 규칙을 덮는다', () => {
    const doc = read(
      svg('<style>.big{font-size:40}</style><text class="big" font-size="11">가</text>'),
    );
    expect(doc.texts[0]?.fontSizeUserUnits).toBe(11);
  });
});

describe('공백은 xml:space 를 물려받는다', () => {
  const PRETTY = '<text x="1" y="2">\n  회의실   A\n</text>';

  it('기본은 한 줄로 접힌다 — 예쁘게 찍어 낸 문서가 흔하다', () => {
    expect(read(svg(PRETTY)).texts[0]?.text).toBe('회의실 A');
  });

  it('루트의 preserve 가 아래로 흐른다', () => {
    const doc = read(svg(PRETTY, `${VIEW_BOX} xml:space="preserve"`));
    expect(doc.texts[0]?.text).toBe('   회의실   A ');
  });
});

describe('옮기지 못하는 글자는 그대로 버림이다', () => {
  it('<textPath> 는 버림이고 요소가 서지 않는다 (뮤테이션 2)', () => {
    const doc = read(svg('<text x="1" y="2"><textPath href="#p">길 위</textPath></text>'));
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'textDropped')).toBe(1);
  });

  it('한 겹 더 싸인 <textPath> 도 잡는다 (뮤테이션 3)', () => {
    const doc = read(
      svg('<text x="1" y="2"><tspan><textPath href="#p">길 위</textPath></tspan></text>'),
    );
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'textDropped')).toBe(1);
  });

  it('fill="none" 이고 테도 없으면 감춤이다 — 문서에서도 보이지 않던 글자다', () => {
    const doc = read(svg('<text x="1" y="2" fill="none">안 보임</text>'));
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'hiddenDropped')).toBe(1);
    expect(reasonCount(doc.notes, 'textDropped')).toBe(0);
  });

  it('fill="none" 인데 테가 있으면 버림이다 — 보이던 글자를 못 옮겼다 (뮤테이션 4)', () => {
    const doc = read(svg('<text x="1" y="2" fill="none" stroke="#c0392b">테만</text>'));
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'textDropped')).toBe(1);
    expect(reasonCount(doc.notes, 'hiddenDropped')).toBe(0);
  });

  it('퇴화 변환 아래의 글자는 버림이다 — 도형과 **같은** 규칙이다 (뮤테이션 22)', () => {
    // `scale(0)` 아래에서는 SVG 도 아무것도 그리지 않는다. 가드가 없으면 한 점으로
    // 찌부러진 자리에 크기 6px(죔의 하한)짜리 글자가 선다 — 문서에 없던 그림이다.
    const doc = read(svg('<g transform="scale(0)"><text x="30" y="40">가</text></g>'));
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'degenerateTransformDropped')).toBe(1);
  });

  it('감춘 글자는 개수로만 센다 — 도형과 **같은** 규칙이다 (뮤테이션 1)', () => {
    const doc = read(svg('<g display="none"><text x="1" y="2">감춤</text></g>'));
    expect(doc.texts).toHaveLength(0);
    expect(reasonCount(doc.notes, 'hiddenDropped')).toBe(1);
  });
});

describe('보고가 옮기지 못한 것을 말한다', () => {
  it('문서가 말한 글꼴은 옮기지 못했다고 말한다', () => {
    const doc = read(svg('<text font-family="Courier">가</text>'));
    expect(reasonCount(doc.notes, 'textFontIgnored')).toBe(1);
  });

  it('아무 활자도 말하지 않은 글자는 보고를 만들지 않는다 — 잡음이 되지 않는다', () => {
    const doc = read(svg('<text x="1" y="2" font-size="12" fill="#145a32">가</text>'));
    expect(doc.notes).toEqual([]);
  });

  it('제 자리나 제 스타일을 말한 <tspan> 을 센다', () => {
    const doc = read(
      svg('<text x="1" y="2">앞<tspan x="5" dy="10">가운데</tspan><tspan fill="red">뒤</tspan></text>'),
    );
    expect(reasonCount(doc.notes, 'tspanDropped')).toBe(2);
    // **내용은 한 줄로 살아남는다.**
    expect(doc.texts[0]?.text).toBe('앞가운데뒤');
  });

  it('중첩된 <tspan> 도 센다 — 도구는 한 겹 더 싸서 낸다 (뮤테이션 13)', () => {
    const doc = read(svg('<text x="1" y="2"><tspan>앞<tspan fill="red">속</tspan></tspan></text>'));
    // 겉의 맨 `<tspan>` 은 세지 않고 안쪽의 `fill` 만 센다 — 재귀가 없으면 0 이 된다.
    expect(reasonCount(doc.notes, 'tspanDropped')).toBe(1);
    expect(doc.texts[0]?.text).toBe('앞속');
  });

  it('맨 <tspan> 은 세지 않는다 — 잃은 것이 없다 (뮤테이션 12)', () => {
    const doc = read(svg('<text x="1" y="2"><tspan id="a" xml:space="default">가</tspan></text>'));
    expect(reasonCount(doc.notes, 'tspanDropped')).toBe(0);
    expect(doc.texts[0]?.text).toBe('가');
  });

  it('패널이 치환할 토큰이 든 글자를 말한다 — 글자는 **그대로 싣는다**', () => {
    const doc = read(svg('<text x="1" y="2">{value} 도</text>'));
    expect(reasonCount(doc.notes, 'textTemplateToken')).toBe(1);
    expect(doc.texts[0]?.text).toBe('{value} 도');
  });

  it('상한을 넘은 글자를 자르고 말한다', () => {
    const doc = read(svg(`<text x="1" y="2">${'가'.repeat(MAX_IMPORT_TEXT_LENGTH + 5)}</text>`));
    expect(reasonCount(doc.notes, 'textTruncated')).toBe(1);
    expect(doc.texts[0]?.text).toHaveLength(MAX_IMPORT_TEXT_LENGTH);
  });
});

describe('z-order 가 바뀐 사실을 말한다 (뮤테이션 6)', () => {
  it('뒤따르는 도형이 있으면 그때까지의 문구 수를 센다', () => {
    const doc = read(
      svg(
        '<text x="1" y="2">가</text><text x="3" y="4">나</text>' +
          '<rect x="30" y="20" width="9" height="9"/><rect x="40" y="20" width="9" height="9"/>',
      ),
    );
    // 문구 둘이 도형 **둘** 위로 올라선다. 그래도 수는 4 가 아니라 2 다.
    expect(reasonCount(doc.notes, 'textOrderChanged')).toBe(2);
  });

  it('문구가 문서의 맨 뒤면 아무것도 바뀌지 않는다 — 보고가 서지 않는다', () => {
    const doc = read(svg('<rect x="30" y="20" width="9" height="9"/><text x="1" y="2">가</text>'));
    expect(reasonCount(doc.notes, 'textOrderChanged')).toBe(0);
  });
});

// --- 계획 층 ---------------------------------------------------------------

describe('문구는 캔버스 단위 기준점 위에 선다', () => {
  it('도형과 **같은 축척**으로 놓인다 — 이름표가 제 도형에서 떨어지지 않는다', () => {
    // viewBox 20 10 200 100 · 캔버스 500×400 → 축척 min(400/200, 320/100)=2,
    // 상자 400×200, 원점 (50,100). 문구 (30,40) → (50+(30−20)×2, 100+(40−10)×2) = (70,160).
    const result = plan(svg('<text x="30" y="40">가</text><rect x="30" y="40" width="10" height="10"/>'));
    expect(result.texts[0]?.at).toEqual({ x: 70, y: 160 });
    expect(result.shapes[0]?.box).toMatchObject({ x: 70, y: 160 });
  });

  it('크기가 문서 축척을 탄다 (뮤테이션 7)', () => {
    const result = plan(svg('<text x="30" y="40" font-size="12">가</text>'));
    expect(result.texts[0]?.style.fontSize).toBe(24);
  });

  it('범위를 넘으면 죄고 그 사실을 말한다 — 양 끝 모두', () => {
    const tiny = plan(svg('<text x="30" y="40" font-size="0.5">가</text>'));
    expect(tiny.texts[0]?.style.fontSize).toBe(CANVAS_FONT_SIZE_MIN);
    expect(reasonCount(tiny.report.notes, 'textSizeClamped')).toBe(1);

    const huge = plan(svg('<text x="30" y="40" font-size="400">가</text>'));
    expect(huge.texts[0]?.style.fontSize).toBe(CANVAS_FONT_SIZE_MAX);
    expect(reasonCount(huge.report.notes, 'textSizeClamped')).toBe(1);
  });

  it('범위 안이면 죄었다고 말하지 않는다', () => {
    const result = plan(svg('<text x="30" y="40" font-size="12">가</text>'));
    expect(reasonCount(result.report.notes, 'textSizeClamped')).toBe(0);
  });

  it('글자뿐인 문서도 viewBox 가 있으면 계획이 선다', () => {
    const result = plan(svg('<text x="30" y="40">가</text>'));
    expect(result.report.shapes).toBe(0);
    expect(result.report.texts).toBe(1);
    expect(result.texts).toHaveLength(1);
  });

  it('viewBox 도 크기도 없고 글자뿐이면 거절이다 — 글자의 폭을 잴 수 없다', () => {
    // `getBBox` 도 `measureText` 도 이 층에 없다(불변식 K4). 담을 종횡비를 지어내지 않는다.
    const refused = planSvgImport(svg('<text x="30" y="40">가</text>', 'id="x"'), DEFAULT_CANVAS_SIZE);
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.reason).toBe('emptyDocument');
  });
});

describe('예산 (뮤테이션 8·9)', () => {
  it('문구도 요소 상한을 먹는다', () => {
    const body = Array.from(
      { length: MAX_IMPORT_ELEMENTS + 1 },
      (_v, i) => `<text x="${30 + (i % 7)}" y="40">가</text>`,
    ).join('');
    const refused = planSvgImport(svg(body), DEFAULT_CANVAS_SIZE);
    expect(refused.ok).toBe(false);
    if (!refused.ok) {
      expect(refused.refusal.reason).toBe('tooManyElements');
      expect(refused.refusal.actual).toBe(MAX_IMPORT_ELEMENTS + 1);
    }
  });

  it('도형과 문구를 **합쳐** 센다 — 한쪽만 세면 상한 위의 요소가 조용히 놓인다', () => {
    const shapes = Array.from(
      { length: MAX_IMPORT_ELEMENTS - 1 },
      (_v, i) => `<rect x="${30 + (i % 7)}" y="40" width="4" height="4"/>`,
    ).join('');
    const refused = planSvgImport(
      svg(`${shapes}<text x="30" y="40">가</text><text x="31" y="41">나</text>`),
      DEFAULT_CANVAS_SIZE,
    );
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.actual).toBe(MAX_IMPORT_ELEMENTS + 1);
  });

  it('문구는 명령을 하나도 나르지 않는다', () => {
    expect(plan(svg('<text x="30" y="40">가</text>')).report.commands).toBe(0);
  });

  it('추정이 문구의 글자를 **UTF-8 바이트로** 센다 (뮤테이션 9)', () => {
    const short = plan(svg('<text x="30" y="40">가</text>'));
    const long = plan(svg(`<text x="30" y="40">${'가'.repeat(50)}</text>`));
    // 한글 49자가 늘었다 → 147 바이트. `length` 로 셌다면 49 이고, 그 1/3 의 추정이
    // "약 24KB" 라고 말한 가져오기를 저장에서 57KB 로 만든다.
    expect(long.report.estimatedBytes - short.report.estimatedBytes).toBe(49 * 3);
    // ASCII 에서는 두 셈이 같다 — 경로 쪽 추정이 한 바이트도 달라지지 않는다.
    const ascii = plan(svg(`<text x="30" y="40">${'W'.repeat(51)}</text>`));
    // ASCII 51자(51B) − 한글 1자(3B) = 48.
    expect(ascii.report.estimatedBytes - short.report.estimatedBytes).toBe(51 - 3);
    // 빈 배열은 대괄호 둘이다 — 쉼표 셈이 `n` 이면 여기가 3 이 된다.
    expect(estimateBytes([], [])).toBe(2);
  });
});

// --- 요소 층 ---------------------------------------------------------------

describe('문구 요소를 만드는 입구는 도형과 **하나**다', () => {
  const TEXT = { at: { x: 10, y: 20 }, text: '가', style: { fontSize: 20 }, hasOwnStyle: false };

  it('문구는 도형 **뒤**에 붙는다 = 도형 **위**에 그려진다 (뮤테이션 10)', () => {
    const shape = { commands: [{ c: 'M' as const, x: 0, y: 0 }], box: { x: 1, y: 2, w: 3, h: 4 }, style: {}, hasOwnStyle: false };
    const { next } = appendImportedElements([], [shape], [TEXT]);
    expect(next.map((el) => el.kind)).toEqual(['path', 'text']);
  });

  it('계단 오프셋이 도형과 문구에 **같이** 한 번만 더해진다 (REQ-06)', () => {
    const before = [0, 1, 2].reduce<ReturnType<typeof appendElement>['next']>(
      (acc) => appendElement(acc, 'rect').next,
      [],
    );
    const shape = { commands: [{ c: 'M' as const, x: 0, y: 0 }], box: { x: 10, y: 20, w: 3, h: 4 }, style: {}, hasOwnStyle: false };
    const { created, createdTexts } = appendImportedElements(before, [shape], [TEXT, TEXT]);
    const off = seedOffset(3);
    expect(off).toBeGreaterThan(0);
    expect(created[0]?.geometry).toMatchObject({ x: 10 + off, y: 20 + off });
    for (const text of createdTexts) expect(text.geometry).toEqual({ x: 10 + off, y: 20 + off });
  });

  it('id 가 도형과 이어서 발급된다 — 겹치지 않는다', () => {
    const shape = { commands: [{ c: 'M' as const, x: 0, y: 0 }], box: { x: 1, y: 2, w: 3, h: 4 }, style: {}, hasOwnStyle: false };
    const { next, created, createdTexts } = appendImportedElements([], [shape, shape], [TEXT]);
    expect([...created, ...createdTexts].map((el) => el.id)).toEqual(['el-1', 'el-2', 'el-3']);
    expect(new Set(next.map((el) => el.id)).size).toBe(3);
  });

  it('말하지 않은 글자는 씨앗 색을 입는다 — 그러지 않으면 보이지 않는다 (뮤테이션 11)', () => {
    const { createdTexts } = appendImportedElements([], [], [TEXT]);
    expect(createdTexts[0]?.style.textColor).toBe(SEED_COLOR);
    // 크기는 씨앗이 덮지 않는다 — 문서가 말한 것이 이긴다.
    expect(createdTexts[0]?.style.fontSize).toBe(20);
  });

  it('말한 글자는 제 색을 지킨다 — 씨앗이 덮지 않는다', () => {
    const { createdTexts } = appendImportedElements(
      [],
      [],
      [{ ...TEXT, style: { textColor: '#145a32' }, hasOwnStyle: true }],
    );
    expect(createdTexts[0]?.style).toEqual({ textColor: '#145a32' });
  });
});

// --- 이음매: 문서 문자열 하나에서 `fillText` 까지 -----------------------------

/** 그린 것을 그대로 적는 2D context 대체. */
function recorder(calls: (string | number)[][]) {
  return {
    save() {},
    restore() {},
    setTransform() {},
    beginPath() {},
    rect() {},
    ellipse() {},
    moveTo() {},
    lineTo() {},
    bezierCurveTo() {},
    closePath() {},
    stroke() {},
    fill() {},
    fillText(text: string, x: number, y: number) {
      calls.push(['fillText', text, x, y]);
    },
    measureText(text: string) {
      return { width: text.length * 7 };
    },
    clearRect() {},
    fillRect() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    globalAlpha: 1,
    font: '',
    textAlign: 'left' as CanvasTextAlign,
    textBaseline: 'middle' as CanvasTextBaseline,
  };
}

describe('이음매 — 문서 문자열 하나가 화면의 글자까지 간다', () => {
  const DOC = svg(
    '<rect x="30" y="20" width="40" height="20" fill="#c0392b"/>' +
      '<text x="30" y="40" text-anchor="middle" font-size="12" fill="#145a32">회의실 A</text>' +
      '<text x="60" y="60">2층</text>',
  );

  function place(): { elements: readonly CanvasElement[]; calls: (string | number)[][] } {
    const result = plan(DOC);
    const made = appendImportedElements([], result.shapes, result.texts);
    const elements = [...made.created, ...made.createdTexts];
    const calls: (string | number)[][] = [];
    const projection: CanvasProjection = {
      stage: { width: DEFAULT_CANVAS_SIZE.width, height: DEFAULT_CANVAS_SIZE.height },
      canvas: DEFAULT_CANVAS_SIZE,
    };
    drawElements(recorder(calls) as unknown as DrawContext2D, elements, {}, {}, projection);
    return { elements, calls };
  }

  it('두 글자가 **실제로 그려진다** — 고치기 전에는 `fillText` 가 한 번도 불리지 않았다', () => {
    const { calls } = place();
    expect(calls.map((c) => c[1])).toEqual(['회의실 A', '2층']);
  });

  it('정렬이 원점에 반영된다 — 가운데 정렬은 잰 폭의 절반만큼 왼쪽에서 시작한다', () => {
    const { calls } = place();
    const centered = calls[0]!;
    const left = calls[1]!;
    // 기준점 x: (30−20)×2+50 = 70 · (60−20)×2+50 = 130. 축척 1 의 스테이지라 px 도 같다.
    expect(centered[2]).toBe(70 - ('회의실 A'.length * 7) / 2);
    expect(left[2]).toBe(130);
  });

  it('세로 기준은 기준점 그대로다 — 보정하지 않는다(단위가 다르다)', () => {
    const { calls } = place();
    // (40−10)×2+100 = 160 · (60−10)×2+100 = 200.
    expect(calls[0]?.[3]).toBe(160);
    expect(calls[1]?.[3]).toBe(200);
  });

  it('말하지 않은 글자도 그려진다 — 씨앗 색이 있기 때문이다', () => {
    const { elements } = place();
    const silent = elements.find((el) => el.kind === 'text' && el.text === '2층');
    expect(silent?.style.textColor).toBe(SEED_COLOR);
    // 그리고 크기는 사양의 초기값 16 × 축척 2 = 32 이지 패널 기본값이 아니다.
    expect(silent?.style.fontSize).toBe(32);
    expect(silent?.style.fontSize).not.toBe(DEFAULT_FONT_SIZE);
  });

  it('놓인 문구는 config 파서 왕복을 견딘다', async () => {
    const { parseCanvasConfig } = await import('../canvasConfig');
    const { elements } = place();
    const parsed = parseCanvasConfig(
      JSON.parse(
        JSON.stringify({ canvas: DEFAULT_CANVAS_SIZE, elements }),
      ) as unknown,
    );
    expect(parsed.elements).toEqual(elements);
  });
});
