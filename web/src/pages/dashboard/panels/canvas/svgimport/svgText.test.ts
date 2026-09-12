// 글자의 순수 산술 시험 (SPEC-CANVAS-007 · 결함 B 정정).
//
// **DOM 을 하나도 세우지 않는다.** 여기 재는 것은 전부 문자열 → 값이며, 그 시험이 jsdom 을
// 세우는 순간 규칙이 렌더 거동으로 위장한다(`svgText.ts` 머리말).
//
// **고정 입력이 피하는 함정 넷** — 넷 다 이 저장소가 한 번씩 물린 부류다.
//   - `text-anchor` 가 **없는** 문구를 함께 둔다. `middle` 만 재면 "언제나 center 를 쓴다" 는
//     뮤테이션이 통과한다.
//   - 크기 고정 입력의 배율이 **1 이 아니다**. 1 이면 곱셈을 지워도 답이 같다.
//   - 공백 고정 입력에 줄바꿈 · 탭 · 이어진 공백 · 앞뒤 공백이 **모두** 들어 있다. 하나만
//     들어 있으면 나머지 세 규칙이 잠든다.
//   - 글자가 **ASCII 가 아니다**. 한글 문자열은 길이 셈(UTF-16 단위)과 자르기가 함께 걸린다.
//
// **확인한 뮤테이션 22종 — 전부 물었다**(실제로 갈아 넣고 돌린 결과다).
//   1. 이어진 공백 줄이기(`replace(/ +/g, ' ')`) 삭제 → "이어진 공백을 하나로" 가 빨개진다.
//   2. `trim()` 삭제 → "앞뒤 공백을 뗀다" 를 포함해 다섯이 빨개진다.
//   3. `preserve` 갈래의 줄바꿈 치환 삭제 → "preserve 도 줄바꿈은 공백" 이 빨개진다.
//   4. `alignFromTextAnchor` 의 `end` → `'center'` → "end 는 오른쪽" 이 빨개진다.
//   5. `readFontSize` 의 `parsed > 0` → `>= 0` → "0 과 음수는 읽지 못한 것" 이 빨개진다.
//   6. `CARRIED_BASELINES` 검사 삭제 / `'central'` 만 빼기 / `'middle'` 만 빼기 / `'auto'`
//      검사 삭제 / `alignment-baseline` 을 목록에서 빼기 / `dominant-baseline` 을 빼기 —
//      **여섯 갈래가 각각** 빨개진다(집합의 내용이 과다 결정이 아님을 이 여섯이 잰다).
//   7. `resolveTextStyle` 이 칠을 `style.fill` 로 두기 → "칠은 글자색 칸으로" 를 포함해
//      여덟이 빨개진다.
//   8. `fontSizeUserUnits` 의 `* fontScale` 삭제 → "변환의 배율이 크기에 곱해진다" 가 빨개진다.
//   9. `truncateImportText` 의 `<=` → `<`. **글자만 재는 단언으로는 등가 뮤테이션이다**
//      (길이가 정확히 상한이면 `slice(0, 상한)` 이 같은 문자열을 낸다). 그래서 `truncated`
//      **플래그를 함께** 재도록 강화했고, 그 뒤 이 뮤테이션은 문다.
//  10. `UNCARRIED_TEXT_PROPS` 의 `'none'` 건너뛰기 삭제 → "none 은 말하지 않은 것" 이 빨개진다.
//  11. 토큰 정규식을 `{value}` 만으로 좁히기 → "셋만 본다" 가 빨개진다.
//  12. 토큰 정규식을 `\{` 하나로 넓히기 → "모르는 토큰은 어긋남이 아니다" 가 빨개진다.
//  13. 굵기 경계 `>= 600` → `>= 500` → "599 는 굵지 않다" 가 빨개진다.
//  14. `resolveTextStyle` 이 `stroke` 를 스타일에 싣기 → "선은 싣지 않는다" 가 빨개진다.
//  15. `readAnchorCoord` 가 `parseLength` 를 쓰도록 되돌리기 → "목록이면 첫 수" 가 빨개진다
//      (그것이 이 결함의 재현이다 — 목록에서 `undefined` 를 받아 기준점이 0 이 된다).
//  16. `perGlyph` 를 언제나 거짓으로 → "목록이라고 말한다" 가 빨개진다.
//  17. 목록의 **마지막** 수를 쓰기 → "첫 수" 가 빨개진다(첫 수와 마지막 수가 다른 고정 입력).

import { describe, expect, it } from 'vitest';

import { SEED_COLOR } from '../canvasElementFactory';

import { MAX_IMPORT_TEXT_LENGTH } from './svgImportTypes';
import {
  SVG_INITIAL_FONT_SIZE,
  UNCARRIED_TEXT_PROPS,
  alignFromTextAnchor,
  fontWeightFrom,
  hasTemplateToken,
  ignoresTypography,
  normalizeSvgText,
  preservesSpace,
  readAnchorCoord,
  readFontSize,
  resolveTextStyle,
  truncateImportText,
} from './svgText';

// --- 공백 -----------------------------------------------------------------

describe('공백은 xml:space 규칙으로 한 줄이 된다 (뮤테이션 1·2·3)', () => {
  /** 줄바꿈 · 탭 · 이어진 공백 · 앞뒤 공백이 **모두** 들어 있다. */
  const PRETTY = '\n  회의실   A\t동\n';

  it('기본 규칙은 줄바꿈을 지우고 탭을 공백으로 바꾸고 이어진 공백을 하나로 줄인다', () => {
    expect(normalizeSvgText(PRETTY, false)).toBe('회의실 A 동');
  });

  it('앞뒤 공백을 뗀다 — 떼지 않으면 화면 왼쪽에 원인 없는 빈 자리가 생긴다', () => {
    expect(normalizeSvgText('   가   ', false)).toBe('가');
  });

  it('preserve 는 이어진 공백을 지키되 줄바꿈과 탭은 공백으로 바꾼다', () => {
    // 줄바꿈을 **지우지 않고 공백으로** 바꾸는 것이 preserve 의 규칙이다. 지우면 두 낱말이
    // 붙어 버리고, 그것은 사양이 말한 "그대로" 가 아니다.
    expect(normalizeSvgText(PRETTY, true)).toBe('   회의실   A 동 ');
  });

  it('공백뿐인 글자는 빈 문자열이 된다 — 그릴 것이 없다는 뜻이다', () => {
    expect(normalizeSvgText('\n \t ', false)).toBe('');
  });

  it('preserve 는 그 값일 때만이다 — default · 부재 · 오타는 전부 기본 규칙이다', () => {
    expect(preservesSpace('preserve')).toBe(true);
    expect(preservesSpace(' preserve ')).toBe(true);
    expect(preservesSpace('default')).toBe(false);
    expect(preservesSpace(undefined)).toBe(false);
    expect(preservesSpace('Preserve')).toBe(false);
  });
});

// --- 정렬과 굵기 ------------------------------------------------------------

describe('text-anchor 는 정렬이 된다 (뮤테이션 4)', () => {
  it('middle 은 가운데, end 는 오른쪽이다', () => {
    expect(alignFromTextAnchor('middle')).toBe('center');
    expect(alignFromTextAnchor('end')).toBe('right');
  });

  it('start · 부재 · 모르는 값은 **적지 않는다** — 렌더 기본이 이미 왼쪽이다', () => {
    expect(alignFromTextAnchor('start')).toBeUndefined();
    expect(alignFromTextAnchor(undefined)).toBeUndefined();
    expect(alignFromTextAnchor('inherit')).toBeUndefined();
    expect(alignFromTextAnchor('')).toBeUndefined();
  });

  it('대문자와 앞뒤 공백을 견딘다 — 속성 값은 사람이 적는다', () => {
    expect(alignFromTextAnchor(' MIDDLE ')).toBe('center');
  });
});

describe('font-weight 는 굵은 것만 적는다', () => {
  it('bold · bolder · 600 이상은 굵다', () => {
    expect(fontWeightFrom('bold')).toBe('bold');
    expect(fontWeightFrom('bolder')).toBe('bold');
    expect(fontWeightFrom('700')).toBe('bold');
    expect(fontWeightFrom('600')).toBe('bold');
  });

  it('599 는 굵지 않다 — 경계가 실제로 600 이다', () => {
    expect(fontWeightFrom('599')).toBeUndefined();
  });

  it('보통 굵기 · 부재 · 모르는 값은 **적지 않는다** — 렌더 기본이 이미 보통이다', () => {
    expect(fontWeightFrom('400')).toBeUndefined();
    expect(fontWeightFrom('normal')).toBeUndefined();
    expect(fontWeightFrom(undefined)).toBeUndefined();
    expect(fontWeightFrom('')).toBeUndefined();
    expect(fontWeightFrom('lighter')).toBeUndefined();
  });
});

// --- 크기 -------------------------------------------------------------------

describe('font-size 는 단위 없는 수와 px 만 읽는다 (뮤테이션 5)', () => {
  it('말하지 않았으면 사양의 초기값이고 **읽지 못한 것이 아니다**', () => {
    expect(readFontSize(undefined)).toEqual({ userUnits: SVG_INITIAL_FONT_SIZE, unreadable: false });
    expect(readFontSize('  ')).toEqual({ userUnits: SVG_INITIAL_FONT_SIZE, unreadable: false });
  });

  it('단위 없는 수와 px 를 읽는다', () => {
    expect(readFontSize('12')).toEqual({ userUnits: 12, unreadable: false });
    expect(readFontSize('12px')).toEqual({ userUnits: 12, unreadable: false });
  });

  it('다른 단위는 읽지 못한 것이다 — 초기값으로 떨어지고 보고에 오른다', () => {
    for (const raw of ['12pt', '1.5em', '120%', 'large']) {
      expect(readFontSize(raw), raw).toEqual({ userUnits: SVG_INITIAL_FONT_SIZE, unreadable: true });
    }
  });

  it('0 과 음수는 읽지 못한 것이다 — 크기 0 은 다시 잡을 수 없는 요소다', () => {
    expect(readFontSize('0').unreadable).toBe(true);
    expect(readFontSize('-5').unreadable).toBe(true);
  });
});

// --- 기준점 좌표 --------------------------------------------------------------

describe('x·y 는 목록일 수 있다 (뮤테이션 15·16)', () => {
  it('수 하나면 그 수이고 목록이 아니다', () => {
    expect(readAnchorCoord('10')).toEqual({ value: 10, perGlyph: false });
    expect(readAnchorCoord('10px')).toEqual({ value: 10, perGlyph: false });
    expect(readAnchorCoord(' -3.5 ')).toEqual({ value: -3.5, perGlyph: false });
  });

  it('목록이면 **첫 수**이고 목록이라고 말한다 — 잉크스케이프가 자간에 그 형상을 낸다', () => {
    expect(readAnchorCoord('10 20 30')).toEqual({ value: 10, perGlyph: true });
    expect(readAnchorCoord('10,20')).toEqual({ value: 10, perGlyph: true });
  });

  it('없거나 읽을 수 없으면 0 이다 — 사양의 초기값이다', () => {
    expect(readAnchorCoord(undefined)).toEqual({ value: 0, perGlyph: false });
    expect(readAnchorCoord('abc')).toEqual({ value: 0, perGlyph: false });
  });
});

// --- 옮기지 못한 활자 --------------------------------------------------------

describe('옮기지 못한 활자를 가린다 (뮤테이션 6)', () => {
  it('목록의 이름을 **하나씩** 물어 본다 — 어느 하나도 잠들어 있지 않다', () => {
    // 목록이 비면 아래 순회가 0회 돌고 초록이 된다.
    expect(UNCARRIED_TEXT_PROPS.length).toBeGreaterThan(0);
    for (const name of UNCARRIED_TEXT_PROPS) {
      expect(ignoresTypography({ [name]: 'x' }, false), name).toBe(true);
    }
  });

  it('빈 값과 none 은 말하지 않은 것이다', () => {
    expect(ignoresTypography({ 'font-family': '' }, false)).toBe(false);
    expect(ignoresTypography({ 'text-decoration': 'none' }, false)).toBe(false);
  });

  it('dominant-baseline 이 가운데면 어긋남이 **아니다** — 패널이 이미 그렇게 그린다', () => {
    expect(ignoresTypography({ 'dominant-baseline': 'middle' }, false)).toBe(false);
    expect(ignoresTypography({ 'dominant-baseline': 'central' }, false)).toBe(false);
    expect(ignoresTypography({ 'dominant-baseline': 'auto' }, false)).toBe(false);
  });

  it('가운데가 아닌 세로 기준은 어긋남이다', () => {
    expect(ignoresTypography({ 'dominant-baseline': 'hanging' }, false)).toBe(true);
    expect(ignoresTypography({ 'alignment-baseline': 'text-before-edge' }, false)).toBe(true);
  });

  it('읽지 못한 크기도 여기로 온다 — 사용자가 할 일이 같기 때문이다', () => {
    expect(ignoresTypography({}, true)).toBe(true);
    expect(ignoresTypography({}, false)).toBe(false);
  });
});

// --- 템플릿 토큰 -------------------------------------------------------------

describe('패널이 치환할 토큰을 가린다', () => {
  it('셋만 본다', () => {
    expect(hasTemplateToken('{value}')).toBe(true);
    expect(hasTemplateToken('온도 {value}℃')).toBe(true);
    expect(hasTemplateToken('{name}')).toBe(true);
    expect(hasTemplateToken('{unit}')).toBe(true);
  });

  it('모르는 토큰과 맨 중괄호는 어긋남이 아니다 — 렌더가 원문 그대로 둔다', () => {
    expect(hasTemplateToken('{foo}')).toBe(false);
    expect(hasTemplateToken('{value')).toBe(false);
    expect(hasTemplateToken('회의실 A')).toBe(false);
  });
});

// --- 길이 상한 ---------------------------------------------------------------

describe('글자 수 상한 (뮤테이션 9 — 등가 뮤테이션을 플래그로 강화)', () => {
  it('넘지 않으면 **자르지 않았다고 말한다** — 같은 문자열이 그대로 나온다', () => {
    const exact = '가'.repeat(MAX_IMPORT_TEXT_LENGTH);
    expect(truncateImportText(exact)).toEqual({ text: exact, truncated: false });
  });

  it('넘으면 자르고 잘랐다고 말한다 — 말줄임표를 지어내지 않는다', () => {
    const over = '가'.repeat(MAX_IMPORT_TEXT_LENGTH + 7);
    const cut = truncateImportText(over);
    expect(cut.truncated).toBe(true);
    expect(cut.text).toHaveLength(MAX_IMPORT_TEXT_LENGTH);
    expect(cut.text.endsWith('…')).toBe(false);
  });
});

// --- 스타일 ------------------------------------------------------------------

describe('문구 스타일 (뮤테이션 7·8)', () => {
  it('칠은 **글자색 칸**으로 간다 — 채움색 칸에 남기지 않는다', () => {
    const out = resolveTextStyle({ fill: '#145a32' }, {});
    expect(out.style.textColor).toBe('#145a32');
    expect(out.style.fill).toBeUndefined();
    expect(out.hasOwnStyle).toBe(true);
  });

  it('선은 싣지 않는다 — 이 패널은 글자에 테를 두르지 않는다', () => {
    const out = resolveTextStyle({ stroke: '#c0392b', 'stroke-width': '3' }, {});
    expect(out.style.stroke).toBeUndefined();
    expect(out.style.strokeWidth).toBeUndefined();
    // 그러나 **사양의 규칙은 살아 있다**: 테만 말한 글자는 검게 칠해진다.
    expect(out.style.textColor).toBe('#000000');
    expect(out.strokePainted).toBe(true);
  });

  it('아무 칠도 말하지 않았으면 색이 없고 hasOwnStyle 이 거짓이다 — 씨앗은 요소 층이 입힌다', () => {
    const out = resolveTextStyle({}, {});
    expect(out.style.textColor).toBeUndefined();
    expect(out.hasOwnStyle).toBe(false);
    expect(out.strokePainted).toBe(false);
    // 씨앗 색이 이 층에서 새어 나오지 않는다.
    expect(JSON.stringify(out.style)).not.toContain(SEED_COLOR);
  });

  it('불투명도는 살아남고 알파는 색에 접힌다', () => {
    const out = resolveTextStyle({ fill: '#145a32', 'fill-opacity': '0.5', opacity: '0.4' }, {});
    expect(out.style.opacity).toBe(0.4);
    expect(out.style.textColor).toBe('rgba(20, 90, 50, 0.5)');
  });

  it('변환의 배율이 크기에 곱해진다 — 축척 1 이 아닌 고정 입력이다', () => {
    const out = resolveTextStyle({}, { 'font-size': '12' }, { fontScale: 2.5 });
    expect(out.fontSizeUserUnits).toBe(30);
  });

  it('활자 자루에서 정렬과 굵기를 읽는다 — 칠 자루가 아니다', () => {
    const out = resolveTextStyle({}, { 'text-anchor': 'end', 'font-weight': 'bold' });
    expect(out.style.align).toBe('right');
    expect(out.style.fontWeight).toBe('bold');
  });

  it('비균등 변환은 옮기지 못한 활자로 본다 — 기울어진 글자를 곧게 그리기 때문이다', () => {
    expect(resolveTextStyle({}, {}, { nonUniform: true }).typographyIgnored).toBe(true);
    expect(resolveTextStyle({}, {}, { nonUniform: false }).typographyIgnored).toBe(false);
  });
});
