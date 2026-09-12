// draw.io 이름표를 문구로 세운다 — `<switch>` → `<foreignObject>` (SPEC-CANVAS-007 REQ-04 · REQ-05).
//
// **이 SPEC 이 물린 부류가 하나 더 있었다.** draw.io 는 SVG `<text>` 를 **한 번도** 내보내지
// 않는다. 이름표마다 `<switch>` 안에 XHTML `<foreignObject>` 와 래스터 대안 `<image>` 를 함께
// 담고, `<text>` 만 찾던 파서는 그 파일의 **이름표를 하나도** 들이지 않았다. 사용자가 본 것이
// 그 조합이다 — 도형은 들어왔고 글자는 없다.
//
// **고치기 전의 실측을 이름으로 못박아 둔다**(아래 §되돌림 감시): 브리핑이 준 조각에 대해
// 배달된 코드는 `texts: 0` · `unenteredContainerDropped: 1` 을 냈다. 그 수를 재는 시험이
// 없으면 다음 사람이 이 가지를 지워도 **도형 시험은 전부 초록으로 남는다.**
//
// **파싱 단언으로 끝내지 않는다.** 이 SPEC 의 학습이 그것이다(0.5.0 ②) — 선택자가 표에
// 담기는지만 재던 시험이 결함 위에서 초록이었다. 그래서 여기서도 **문서 → 계획** 까지 몰아
// 요소가 실제로 서는지를 함께 읽는다.
//
// @spec SPEC-CANVAS-007 REQ-04 · REQ-05

import { describe, expect, it } from 'vitest';

import { readSvgDocument } from './svgDocument';
import {
  anchorXInBox,
  anchorYInBox,
  collapseHtmlText,
  readForeignLabelBox,
  textAnchorFromHtmlFlow,
} from './svgForeignLabel';
import { planSvgImport } from './svgImportPlan';
import type { ImportedText } from './svgImportTypes';

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 (시험 규율 E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

const NS = [
  'xmlns="http://www.w3.org/2000/svg"',
  'xmlns:xlink="http://www.w3.org/1999/xlink"',
  // 미지 네임스페이스 — `localName` 은 같고 네임스페이스만 다른 요소를 세우는 데 쓴다.
  'xmlns:foo="urn:foo"',
].join(' ');

/** 삼켜지지 않는 기준 도형. 없으면 문서가 `emptyDocument` 로 거절된다. */
const ANCHOR_SHAPE = '<rect x="10" y="20" width="40" height="30" fill="#c81e1e"/>';

function svg(body: string): string {
  return `<svg ${NS} ${VIEW_BOX}>${ANCHOR_SHAPE}${body}</svg>`;
}

/**
 * 브리핑이 준 **실제 draw.io 조각** 그대로. 바꾼 것은 하나뿐이다 — `&nbsp;` 는 이 층에 닿을
 * 수 없으므로(아래 §엔티티) 쓰지 않는다.
 *
 * `label` 은 안쪽 `<div>` 의 내용 그대로 꽂힌다(마크업을 넣어 시험할 수 있다).
 */
function drawioLabel(
  label: string,
  options: {
    readonly justify?: string;
    readonly innerStyle?: string;
    readonly image?: string;
    /**
     * 보이지 않는 잡는 자리를 함께 낼 것인가. **기본은 낸다**(draw.io 의 실제 꼴이다).
     *
     * 끄는 자리는 하나뿐이다 — 감춤·퇴화처럼 **개수를 세는** 보고에서 그 사각형이 제 몫을
     * 함께 올리므로(실측: `display:none` 아래에서 `hiddenDropped: 2`), 이름표 **하나의**
     * 몫을 재려면 떼어 놓아야 한다. 수만 2 로 적으면 어느 1 이 이름표인지 시험이 말하지 못한다.
     */
    readonly hitRect?: boolean;
  } = {},
): string {
  const justify = options.justify ?? 'unsafe flex-start';
  const innerStyle =
    options.innerStyle ??
    'display: inline-block; font-size: 12px; font-family: Helvetica; ' +
      'color: light-dark(#000000, #ffffff); line-height: 1.2; font-weight: bold; ' +
      'white-space: normal; word-wrap: normal; ';
  const image =
    options.image ?? '<image x="42" y="133.5" width="58" height="17" xlink:href="data:image/png;base64,AAA"/>';
  const hitRect =
    options.hitRect === false
      ? ''
      : '<g><rect x="40.45" y="119.8" width="60" height="40" fill="none" stroke="none" pointer-events="all"/></g>';
  return (
    '<g data-cell-id="FMO34kK2EbGy4XcusBex-3">' +
    hitRect +
    '<g><g><switch>' +
    '<foreignObject style="overflow: visible; text-align: left;" pointer-events="none"' +
    ' width="100%" height="100%"' +
    ' requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility">' +
    '<div xmlns="http://www.w3.org/1999/xhtml"' +
    ` style="display: flex; align-items: unsafe center; justify-content: ${justify};` +
    ' width: 58px; height: 1px; padding-top: 140px; margin-left: 42px;">' +
    '<div style="box-sizing: border-box; font-size: 0; text-align: left; color: #000000; ">' +
    `<div style="${innerStyle}">${label}</div>` +
    '</div></div></foreignObject>' +
    image +
    '</switch></g></g>' +
    '</g>'
  );
}

/** 문서 층까지 읽는다. 거절이면 시험이 그 자리에서 죽는다(뒤의 단언이 거짓을 말하지 못하게). */
function readTexts(body: string): readonly ImportedText[] {
  const outcome = readSvgDocument(svg(body));
  if (!outcome.ok) throw new Error(`거절됨: ${outcome.refusal.reason}`);
  return outcome.document.texts;
}

function readNotes(body: string): readonly { kind: string; reason: string; count: number }[] {
  const outcome = readSvgDocument(svg(body));
  if (!outcome.ok) throw new Error(`거절됨: ${outcome.refusal.reason}`);
  return outcome.document.notes;
}

function countOf(body: string, reason: string): number {
  return readNotes(body)
    .filter((note) => note.reason === reason)
    .reduce((sum, note) => sum + note.count, 0);
}

describe('draw.io 이름표 — 실제 조각', () => {
  it('글자 · 상자 · 활자 · 색 · 정렬을 한 번에 옮긴다', () => {
    const texts = readTexts(drawioLabel('크기'));
    expect(texts).toHaveLength(1);
    const text = texts[0] as ImportedText;

    // 글자 — `<foreignObject>` 하위 트리의 내용이다.
    expect(text.text).toBe('크기');

    // 상자 — `<image x="42" y="133.5" width="58" height="17">` 에서 난다.
    // `justify-content: flex-start` 이므로 기준점은 **왼쪽 끝**이고, 세로는 상자 가운데다.
    expect(text.x).toBe(42);
    expect(text.y).toBe(133.5 + 17 / 2);

    // 활자 — 안쪽 `<div>` 의 `font-size: 12px` · `font-weight: bold`.
    expect(text.fontSizeUserUnits).toBe(12);
    expect(text.style.fontWeight).toBe('bold');

    // 색 — `color: light-dark(#000000, #ffffff)` 의 밝은 쪽. `resolveCssWideValue` 가 푼다.
    expect(text.style.textColor).toBe('#000000');
    expect(text.hasOwnStyle).toBe(true);

    // 정렬 — `start` 는 렌더 기본이므로 **적지 않는다**(`alignFromTextAnchor` 의 규율).
    expect(text.style.align).toBeUndefined();
  });

  it('세운 이름표를 그릇 버림으로 **다시** 보고하지 않는다', () => {
    // `408aaf12` 가 켠 보고는 삼킨 하위 트리에 대한 것이다. 옮긴 것을 버렸다고 말하면
    // 보고가 거짓이 되고, 거짓인 보고는 침묵보다 나쁘다.
    expect(countOf(drawioLabel('크기'), 'unenteredContainerDropped')).toBe(0);
  });

  it('계획 층까지 지나 문구 요소로 선다', () => {
    const plan = planSvgImport(svg(drawioLabel('크기')), { width: 500, height: 400 });
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    expect(plan.texts).toHaveLength(1);
    expect(plan.texts[0]?.text).toBe('크기');
    expect(plan.report.texts).toBe(1);
    // 보이지 않는 잡는 자리(`fill="none" stroke="none"`)는 `aead4981` 이 이미 건너뛴다 —
    // 그래서 이름표 하나가 먹는 요소는 **여전히 하나**다.
    expect(plan.report.shapes).toBe(1);
  });
});

describe('가로 정렬 — `justify-content` 가 기준점을 옮긴다', () => {
  it('flex-start → 상자 왼쪽 끝, 정렬은 적지 않는다', () => {
    const text = readTexts(drawioLabel('가', { justify: 'unsafe flex-start' }))[0] as ImportedText;
    expect(text.x).toBe(42);
    expect(text.style.align).toBeUndefined();
  });

  it('center → 상자 가운데 · `align: center`', () => {
    const text = readTexts(drawioLabel('가', { justify: 'unsafe center' }))[0] as ImportedText;
    expect(text.x).toBe(42 + 58 / 2);
    expect(text.style.align).toBe('center');
  });

  it('flex-end → 상자 오른쪽 끝 · `align: right`', () => {
    const text = readTexts(drawioLabel('가', { justify: 'unsafe flex-end' }))[0] as ImportedText;
    expect(text.x).toBe(42 + 58);
    expect(text.style.align).toBe('right');
  });

  it('읽지 못하는 `justify-content` 는 `text-align` 으로 떨어진다', () => {
    // 바깥 `<div>` 가 `space-between` 을 말하면 그것은 정렬이 아니다. 가운데 `<div>` 의
    // `text-align: left` 가 남으므로 왼쪽이다.
    const text = readTexts(drawioLabel('가', { justify: 'space-between' }))[0] as ImportedText;
    expect(text.x).toBe(42);
    expect(text.style.align).toBeUndefined();
  });
});

describe('글자 긁기 — 마크업 · 공백 · 줄바꿈', () => {
  it('중첩 마크업의 태그를 버리고 글자만 잇는다', () => {
    const texts = readTexts(drawioLabel('앞<b>굵게</b><font style="font-size: 14px;">큰</font>뒤'));
    expect(texts[0]?.text).toBe('앞굵게큰뒤');
  });

  it('이름표 **일부만** 꾸민 인라인 마크업은 활자 보고에 오른다', () => {
    // **옮기지 못한 활자를 한 마디도 말하지 않는 이름표**로 잰다. 기본 조각의 안쪽 `<div>`
    // 는 `font-family: Helvetica` 를 말하므로 그것만으로 이미 `textFontIgnored` 가 올라,
    // 그 조각으로 재면 이 단언이 **마크업 때문인지 글꼴 때문인지 가리지 못한다**(실측:
    // 마크업 판정을 꺼도 초록으로 남았다 — 뮤테이션 M21).
    const plain = 'font-size: 12px; color: #000000;';
    expect(countOf(drawioLabel('그냥글자', { innerStyle: plain }), 'textFontIgnored')).toBe(0);
    expect(
      countOf(drawioLabel('앞<font style="font-size: 14px;">큰</font>', { innerStyle: plain }), 'textFontIgnored'),
    ).toBe(1);
    // 그리고 그 크기를 **가져가지 않았다** — 안쪽 `<div>` 의 12 가 그대로다.
    expect(
      readTexts(drawioLabel('앞<font style="font-size: 14px;">큰</font>', { innerStyle: plain }))[0]
        ?.fontSizeUserUnits,
    ).toBe(12);
  });

  it('`<div>` 가 아닌 것의 `style` 은 걷지 않는다 — 색도 크기도 빼앗기지 않는다', () => {
    const text = readTexts(
      drawioLabel('앞<span style="color: #ff0000; font-size: 40px;">빨강</span>', {
        innerStyle: 'font-size: 12px; color: #000000;',
      }),
    )[0] as ImportedText;
    expect(text.style.textColor).toBe('#000000');
    expect(text.fontSizeUserUnits).toBe(12);
  });

  it('주석은 글자가 아니다 — 원본에 없는 글자를 config 에 싣지 않는다', () => {
    // 실측: XML 주석은 `nodeType 8` 로 서고 `nodeValue` 가 `" 숨은말 "` 이다. 요소만
    // 거르는 판정이 없으면 그 문자열이 이름표에 붙는다.
    expect(readTexts(drawioLabel('앞<!-- 숨은말 -->뒤'))[0]?.text).toBe('앞뒤');
  });

  it('CDATA 는 글자다', () => {
    // 실측: `nodeType 4` 로 따로 선다 — 글자 노드만 보면 그 내용이 통째로 사라진다.
    expect(readTexts(drawioLabel('앞<![CDATA[씨데이터]]>뒤'))[0]?.text).toBe('앞씨데이터뒤');
  });

  it('`<br/>` 은 공백 하나가 된다 — 낱말이 붙지 않는다', () => {
    // 문구 요소는 한 점 위에 서는 한 줄이므로(§결정 10) 두 줄을 나를 자리가 없다.
    // **줄바꿈을 지우면** `"위아래"` 가 되어 원본에 없는 낱말이 생긴다.
    expect(readTexts(drawioLabel('위<br/>아래'))[0]?.text).toBe('위 아래');
  });

  it('예쁘게 찍어 낸 `<div>` 의 줄바꿈과 들여쓰기를 접는다', () => {
    expect(readTexts(drawioLabel('\n      표시   필드\n    '))[0]?.text).toBe('표시 필드');
  });

  it('U+00A0 은 접지도 떼지도 않는다 — 공백이 아니라 글자다', () => {
    // CSS 의 `white-space: normal` 은 non-breaking space 를 보통 글자로 둔다. `\s` 나
    // `trim()` 을 쓰면 이 꼬리가 떨어져 `"X:"` 가 된다.
    expect(readTexts(drawioLabel('X:\u00a0'))[0]?.text).toBe('X:\u00a0');
    expect(readTexts(drawioLabel('표시 필드\u00a0'))[0]?.text).toBe('표시 필드\u00a0');
  });

  it('빈 이름표는 아무것도 세우지 않고 **아무 말도 하지 않는다**', () => {
    // 브라우저도 그리지 않는다 — 잃은 그림이 없으므로 버림에 올리면 보고가 잡음이 된다
    // (빈 `<text>` 를 세지 않는 것과 같은 규율).
    const body = drawioLabel('   \n   ');
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(0);
    expect(countOf(body, 'foreignObjectDropped')).toBe(0);
  });
});

describe('세우지 못한 `<switch>` 는 예전 그대로 보고에 오른다', () => {
  it('`<image>` 대안이 없으면 이름표를 세우지 않고 그릇 버림으로 말한다', () => {
    // 상자가 없으면 놓을 자리가 없다. 자리를 지어내면 문서의 값이 아니라 우리가 고른 값이다.
    const body = drawioLabel('크기', { image: '' });
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(1);
  });

  it('`<image>` 의 치수를 읽지 못해도 같다', () => {
    const body = drawioLabel('크기', {
      image: '<image x="42" y="133.5" width="58%" height="17" xlink:href="data:image/png;base64,AAA"/>',
    });
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(1);
  });

  it('퇴화한 상자(치수 0)도 세우지 않는다', () => {
    const body = drawioLabel('크기', {
      image: '<image x="42" y="133.5" width="58" height="0" xlink:href="data:image/png;base64,AAA"/>',
    });
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(1);
  });

  it('`<foreignObject>` 없이 `<image>` 만 든 `<switch>` 는 세우지 않는다', () => {
    // 래스터 대안만 남은 `<switch>` 다. 글자를 긁을 자리가 없으므로 이름표가 아니고,
    // **말없이 삼키면** 그 그림이 보고에서도 사라진다(실측: `<foreignObject>` 요구를 빼면
    // 이 `<switch>` 가 빈 이름표로 판정되어 조용해진다 — 뮤테이션 M5).
    const body = '<switch><image x="42" y="133.5" width="58" height="17" xlink:href="data:image/png;base64,AAA"/></switch>';
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(1);
  });

  it('미지 네임스페이스의 `image` 는 래스터 대안이 아니다', () => {
    // 실측: `<foo:image>` 는 `localName` 이 `image` 인 **다른** 요소로 선다. 이름만 보면
    // 그것을 상자로 읽어, 문서가 이름표 자리로 말한 적 없는 좌표에 글자를 세운다.
    const body = drawioLabel('크기', {
      image:
        '<foo:image x="1" y="1" width="5" height="5"/>' +
        '<image x="42" y="133.5" width="58" height="17" xlink:href="data:image/png;base64,AAA"/>',
    });
    expect(readTexts(body)[0]?.x).toBe(42);
  });

  it('`<foreignObject>` 자신의 `style` 도 선언 사슬의 맨 바깥이다', () => {
    // draw.io 는 거기에 `text-align` 을 적는다(브리핑의 조각이 그렇다). 그 파일에서는
    // 안쪽 `<div>` 들이 같은 값을 되풀이해 **덮이지만**, 사슬의 한 마디를 읽지 않는 코드는
    // 그 되풀이가 사라지는 날 조용히 어긋난다.
    const body = drawioLabel('가', { justify: 'space-between' }).replace(
      'style="overflow: visible; text-align: left;"',
      'style="overflow: visible; text-align: right;"',
    );
    // 가운데 `<div>` 의 `text-align: left` 가 바깥을 덮으므로 왼쪽이다 — 사슬이 실제로
    // 바깥에서 안으로 흐른다는 증거다.
    expect(readTexts(body)[0]?.x).toBe(42);
    // 가운데 `<div>` 가 말하지 않으면 바깥이 남는다.
    const outerOnly = body.replace('font-size: 0; text-align: left; color: #000000; ', 'font-size: 0; ');
    expect(readTexts(outerOnly)[0]?.x).toBe(42 + 58);
    expect(readTexts(outerOnly)[0]?.style.align).toBe('right');
  });

  it('이름표가 아닌 `<switch>`(대안이 도형)는 건드리지 않는다', () => {
    const body =
      '<switch>' +
      '<g systemLanguage="ko"><rect x="5" y="5" width="10" height="10" fill="#123456"/></g>' +
      '<g><rect x="5" y="5" width="10" height="10" fill="#654321"/></g>' +
      '</switch>';
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(1);
  });
});

describe('바깥 문맥 — 변환과 감춤이 이름표에도 흐른다', () => {
  it('조상 `transform` 이 기준점과 글자 크기에 함께 녹는다', () => {
    const body = `<g transform="translate(100, 50) scale(2)">${drawioLabel('크기')}</g>`;
    const text = readTexts(body)[0] as ImportedText;
    expect(text.x).toBe(100 + 42 * 2);
    expect(text.y).toBe(50 + (133.5 + 17 / 2) * 2);
    expect(text.fontSizeUserUnits).toBe(24);
  });

  it('감춘 조상 아래의 이름표는 요소가 되지 않고 개수로만 센다', () => {
    // 잡는 사각형을 떼고 잰다 — 그것도 감춤 개수를 제 몫으로 올리므로(실측: 함께 두면 2),
    // 떼지 않으면 이 시험이 이름표의 1 을 가리키지 못한다.
    const body = `<g display="none">${drawioLabel('크기', { hitRect: false })}</g>`;
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'hiddenDropped')).toBe(1);
    // 감춘 하위 트리의 미지원 내용은 보고하지 않는다 — 이름표도 그 규칙 안이다.
    expect(countOf(body, 'unenteredContainerDropped')).toBe(0);
  });

  it('퇴화 변환 아래에서는 버림으로 오른다', () => {
    const body = `<g transform="scale(0)">${drawioLabel('크기', { hitRect: false })}</g>`;
    expect(readTexts(body)).toHaveLength(0);
    expect(countOf(body, 'degenerateTransformDropped')).toBe(1);
  });
});

describe('엔티티 — 이 층에는 풀 자리가 없다', () => {
  it('맨 `&nbsp;` 는 **문서를 통째로** 죽인다(XML 이 미리 정한 엔티티가 아니다)', () => {
    // 그래서 엔티티 표를 이 코드베이스에 두면 **한 번도 돌지 않는다.** 이 시험이 그 사실을
    // 못박는다 — 다음 사람이 디코더를 쓰려 들면 여기서 전제를 읽는다.
    const outcome = readSvgDocument(svg(drawioLabel('X:&nbsp;')));
    expect(outcome.ok).toBe(false);
    if (outcome.ok) return;
    expect(outcome.refusal.reason).toBe('notSvg');
  });

  it('수치 엔티티와 리터럴 U+00A0 은 `DOMParser` 가 이미 푼다', () => {
    expect(readTexts(drawioLabel('X:&#160;'))[0]?.text).toBe('X:\u00a0');
    expect(readTexts(drawioLabel('X:\u00a0'))[0]?.text).toBe('X:\u00a0');
  });
});

describe('순수 산술 — 경계값', () => {
  it('`collapseHtmlText` 는 접을 수 있는 넷만 접는다', () => {
    expect(collapseHtmlText(' \t\n\r\f가 \t 나 \n ')).toBe('가 나');
    expect(collapseHtmlText('\u00a0가\u00a0')).toBe('\u00a0가\u00a0');
    expect(collapseHtmlText('')).toBe('');
    expect(collapseHtmlText('   ')).toBe('');
  });

  it('`textAnchorFromHtmlFlow` — 두 축의 우선순위와 폴백', () => {
    // `justify-content` 가 `text-align` 을 이긴다.
    expect(textAnchorFromHtmlFlow('unsafe center', 'left')).toBe('middle');
    expect(textAnchorFromHtmlFlow('flex-end', 'center')).toBe('end');
    // 읽지 못하면 뒤 축으로 떨어진다.
    expect(textAnchorFromHtmlFlow('space-around', 'right')).toBe('end');
    expect(textAnchorFromHtmlFlow(undefined, 'center')).toBe('middle');
    // 둘 다 없으면 `start` — CSS 의 초기값이다.
    expect(textAnchorFromHtmlFlow(undefined, undefined)).toBe('start');
    expect(textAnchorFromHtmlFlow('space-around', 'nonsense')).toBe('start');
    // 논리 어휘도 읽는다.
    expect(textAnchorFromHtmlFlow('start', undefined)).toBe('start');
    expect(textAnchorFromHtmlFlow('end', undefined)).toBe('end');
    expect(textAnchorFromHtmlFlow(undefined, 'justify')).toBe('start');
  });

  it('`readForeignLabelBox` — 넷이 모두 읽히고 두 치수가 양수일 때만 성립한다', () => {
    expect(readForeignLabelBox({ x: '42', y: '133.5', width: '58', height: '17' })).toEqual({
      minX: 42,
      minY: 133.5,
      width: 58,
      height: 17,
    });
    // px 도 읽는다 — `parseLength` 와 같은 규율이다.
    expect(readForeignLabelBox({ x: '1px', y: '2px', width: '3px', height: '4px' })?.width).toBe(3);
    expect(readForeignLabelBox({ y: '2', width: '3', height: '4' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', width: '3', height: '4' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', y: '2', height: '4' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', y: '2', width: '3' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', y: '2', width: '0', height: '4' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', y: '2', width: '3', height: '-1' })).toBeUndefined();
    expect(readForeignLabelBox({ x: '1', y: '2', width: '50%', height: '4' })).toBeUndefined();
  });

  it('`anchorXInBox` · `anchorYInBox` — 상자가 점으로 접히는 세 자리', () => {
    const box = { minX: 42, minY: 133.5, width: 58, height: 17 };
    expect(anchorXInBox('start', box)).toBe(42);
    expect(anchorXInBox('middle', box)).toBe(71);
    expect(anchorXInBox('end', box)).toBe(100);
    expect(anchorYInBox(box)).toBe(142);
  });
});

describe('예산 — 이름표가 요소 상한을 먹는 몫', () => {
  /** 도형 하나(칠과 선을 둘 다 말하는 draw.io 사각형). */
  function styledShape(i: number): string {
    const x = 10 + (i % 10) * 28;
    const y = 10 + Math.floor(i / 10) * 28;
    return (
      `<g data-cell-id="s${i}"><g><rect x="${x}" y="${y}" width="24" height="18" fill="#f5f5f5"` +
      ' stroke="#666666" style="fill: light-dark(rgb(245,245,245), rgb(26,26,26));"/></g></g>'
    );
  }

  function measure(shapes: number, labels: number): number | 'refused' {
    let body = '';
    for (let i = 0; i < shapes; i += 1) body += styledShape(i);
    for (let i = 0; i < labels; i += 1) body += drawioLabel(`라벨${i}`);
    const plan = planSvgImport(`<svg ${NS} ${VIEW_BOX}>${body}</svg>`, { width: 500, height: 400 });
    return plan.ok ? plan.shapes.length + plan.texts.length : 'refused';
  }

  it('이름표 하나가 먹는 요소는 **하나**다 — 잡는 사각형은 여전히 서지 않는다', () => {
    // `aead4981` 이전에는 이름표 자리마다 보이지 않는 사각형이 하나씩 섰고, 그 커밋이
    // 그것을 끊었다. 이번 개정이 그 자리를 **문구로** 다시 채우므로 몫은 도로 1 이다 —
    // 둘이 되면 상한 64 가 절반으로 줄어든다. 그 수를 못박는다.
    expect(measure(0, 1)).toBe(1);
    expect(measure(0, 10)).toBe(10);
  });

  it('사용자 파일의 꼴(도형 25 · 이름표 30)은 상한 64 아래에 선다', () => {
    // **실측 55** — 상한까지 9 칸이 남는다.
    expect(measure(25, 30)).toBe(55);
  });

  it('상한은 도형과 이름표의 **합**에 걸린다 — 64 에서 정확히 끊긴다', () => {
    expect(measure(25, 39)).toBe(64);
    expect(measure(34, 30)).toBe(64);
    // 넘으면 앞부분만 가져오지 않고 **거절한다**(§결정 7).
    expect(measure(25, 40)).toBe('refused');
    expect(measure(35, 30)).toBe('refused');
  });
});

describe('되돌림 감시 — 고치기 전의 수를 이름으로 못박는다', () => {
  it('이 가지를 지우면 이름표가 **하나도** 들어오지 않는다', () => {
    // 실측(고치기 전): 브리핑의 조각에 대해 `texts: 0` · `unenteredContainerDropped: 1`.
    // 두 수를 함께 재는 것에 뜻이 있다 — 글자 수만 재면 "보고가 거짓이 되는" 쪽이 안 물리고,
    // 보고만 재면 "글자가 없다" 는 증상 자체가 안 물린다.
    const body = drawioLabel('크기');
    expect(readTexts(body)).toHaveLength(1);
    expect(countOf(body, 'unenteredContainerDropped')).toBe(0);
  });

  it('이름표 여럿이 든 문서에서 수가 함께 는다', () => {
    const body = drawioLabel('하나') + drawioLabel('둘') + drawioLabel('셋');
    expect(readTexts(body).map((t) => t.text)).toEqual(['하나', '둘', '셋']);
  });
});
