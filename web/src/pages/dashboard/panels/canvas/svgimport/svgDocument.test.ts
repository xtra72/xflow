// 문서 순회 시험 (SPEC-CANVAS-007 M5 · AC-03 · AC-07 · AC-E4 · AC-E6 · AC-E9 · AC-E10).
//
// **E1 이 이 파일의 변환 고정 입력을 정한다.** 항등 변환(속성이 없거나 `translate(0,0)`)은
// 변환 층 전부를 통과시키고, `translate` 만 쓴 고정 입력은 **합성 순서를 관측하지 못한다**
// (평행이동끼리는 교환법칙이 성립한다). 그래서 고정 입력은 **깊이 3 의 `<g>` 중첩**에
// 세 종류(`translate` · `rotate(a,cx,cy)` · 비균등 `scale`)를 쓴다.
//
// **E9 가 `<use>` 고정 입력을 정한다.** 중첩되지 않은 `<use>` 로는 깊이 상한 가드도 순환
// 검출도 **잠든다** — 이 저장소가 이미 물린 부류다. 그래서 깊이 5 사슬과 자기 참조와
// 서로 참조를 **각각** 둔다.
//
// **감춤 전파가 이 파일의 중심이다.** M1~M4 는 `isHidden` 을 **요소 하나에 대해서만**
// 판정했다. 그 판정만으로는 `<g visibility="hidden">` 안의 자식들이 제 속성에 아무 말도
// 없으므로 **전부 그대로 들어온다** — 사용자가 감춘 그림이 통째로 가져와지는, 이 층에서
// 가장 나쁜 실패다. 아래 "감춤" 묶음이 그 전파를 잰다.
//
// **확인한 뮤테이션(E12)** — 각 항목의 "→" 뒤가 빨개지는 단언이다.
//   1. `descend` 에서 감춤 전파(`parent.displayNone || …`)를 지우고 자기 판정만 남기면
//      → "감춘 <g> 안의 도형은 하나도 들어오지 않는다" 가 빨개진다.
//   2. `visibility` 의 "자기 값이 이긴다" 를 "물려받은 값이 이긴다" 로 바꾸면
//      → "자식이 visible 로 되살린다" 가 빨개진다.
//   3. `multiplyMatrix(parent.matrix, own)` 을 `multiplyMatrix(own, parent.matrix)` 로
//      뒤집으면 → "깊이 3 중첩의 좌표" 가 빨개진다. **`translate` 만 있는 고정 입력에서는
//      빨개지지 않는다** — 그래서 세 종류를 섞는다.
//   4. `expanding` 순환 검출만 지우면 → "자기 자신을 품은 <g> 는 한 벌만 나온다" 가
//      빨개진다(도형이 4개가 된다). **처음 세운 가드는 물지 않았다** — "자기 참조는
//      멈춘다" 만으로는 깊이 상한이 그 자리를 함께 덮어 **순환 검출을 지워도 멈춘다.**
//      두 가드가 따로 있는 이유는 "멈춘다" 가 아니라 "되풀이하지 않는다" 이므로,
//      **도형 수를 재는** 시험으로 강화했다.
//   5. `ctx.depth >= MAX_USE_DEPTH` 를 지우면 → "깊이 5 사슬은 상한에서 멈춘다" 가
//      빨개진다.
//   6. `<defs>` 건너뜀은 **한 지렛대로 깨지지 않는다**(과다 결정). `NON_RENDERED_TAGS`
//      에서 `defs` 를 빼도, `CONTAINER_TAGS` 에 `defs` 를 더해도 **각각 하나만으로는
//      어느 시험도 빨개지지 않는다** — 앞의 것을 빼면 순회의 기본(모르는 요소는 내려가지
//      않는다)이 그 자리를 덮고, 뒤의 것만 더하면 `NON_RENDERED_TAGS` 검사가 먼저 끊는다.
//      **둘을 함께** 바꾸면 → "<defs> 안의 도형은 그려지지 않는다" 를 포함해 다섯이
//      빨개진다(확인함). 처음 세운 고정 입력은 `<defs>` 안에 **안 쓰는 그라디언트만**
//      두어 그 성질조차 관측하지 못했으므로, **그려질 수 있는 `<rect>`** 를 넣어
//      강화했다 — 이제 시험이 재는 것은 어느 한 집합의 내용물이 아니라 **성질**이다.
//   7. `parseFailure` 에서 루트 태그 검사를 지우면 → "SVG 가 아닌 XML" 이 빨개진다.
//   8. **물지 않은 뮤테이션**: `parseFailure` 에서 `parsererror` 존재 검사를 지워도
//      **어느 시험도 빨개지지 않는다** — jsdom 은 깨진 문서의 **루트를** `parsererror` 로
//      만들어(측정) 루트 태그 검사가 그 자리를 함께 덮기 때문이다. 브라우저는 문서 **안에**
//      `<parsererror>` 를 끼우는 것으로 알려져 있으나 본 SPEC 은 그것을 실측하지 않았다
//      (가정 A9 — 미검증). 실측할 수 없는 것을 거동으로 잴 수 없으므로 **형상으로**
//      강화했다: 아래 "파싱 실패 판정은 둘 다 본다(형상)" 가 두 검사의 존재를 잰다.
//      그 가드를 지우면 뮤테이션 8 이 문다.

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it, vi } from 'vitest';

import { SEED_COLOR } from '../canvasElementFactory';

import { readSvgDocument } from './svgDocument';
import type { ImportNote } from './svgImportTypes';

function svg(body: string, rootAttrs = 'viewBox="-13 7 317 181"'): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${rootAttrs}>${body}</svg>`;
}

function read(text: string) {
  const outcome = readSvgDocument(text);
  if (!outcome.ok) throw new Error(`거절됨: ${outcome.refusal.reason}`);
  return outcome.document;
}

function reasonCount(notes: readonly ImportNote[], reason: string): number {
  return notes.find((n) => n.reason === reason)?.count ?? 0;
}

const SRC_DIR = path.dirname(new URL(import.meta.url).pathname);

/**
 * 주석을 걷어낸다. **금지 식별자를 이름으로 적어 둔 머리말이 가드를 빨갛게 만들면,
 * 다음 사람은 가드를 고치는 대신 머리말에서 그 이름을 지운다** — 그러면 왜 금지인지가
 * 사라지고 가드만 남는다. 가드가 재야 하는 것은 **코드**다.
 */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/\/\/[^\n]*/g, ' ');
}

/** `svgimport/` 의 **제품 파일**(시험 파일 제외). 시험 파일은 금지 식별자를 문자열로 든다. */
function productionSources(): { name: string; text: string }[] {
  return fs
    .readdirSync(SRC_DIR)
    .filter((name) => name.endsWith('.ts') && !name.endsWith('.test.ts'))
    .map((name) => ({
      name,
      text: stripComments(fs.readFileSync(path.join(SRC_DIR, name), 'utf8')),
    }));
}

describe('파싱 실패는 값으로 돌아온다 (AC-E4 · 뮤테이션 7·8)', () => {
  it.each([
    ['빈 문자열', ''],
    ['SVG 가 아닌 XML', '<note xmlns="urn:x"><to>a</to></note>'],
    ['HTML 문서', '<!doctype html><html><body><p>hi</p></body></html>'],
    ['닫히지 않은 태그', '<svg xmlns="http://www.w3.org/2000/svg"><g></svg>'],
    ['텍스트 쓰레기', 'banana'],
  ])('%s 는 notSvg 로 거절된다 — 예외가 아니다', (_label, text) => {
    const outcome = readSvgDocument(text);
    expect(outcome.ok).toBe(false);
    if (!outcome.ok) expect(outcome.refusal.reason).toBe('notSvg');
  });

  it('파싱 실패 판정은 둘 다 본다(형상) — 뮤테이션 8 을 무는 가드', () => {
    const source = fs.readFileSync(path.join(SRC_DIR, 'svgDocument.ts'), 'utf8');
    const body = source.slice(source.indexOf('function parseFailure'));
    const fn = body.slice(0, body.indexOf('\n}'));
    expect(fn).toContain("getElementsByTagName('parsererror')");
    expect(fn).toContain("!== 'svg'");
  });
});

describe('변환은 좌표에 녹는다, 순서대로 (AC-03 · E1 · 뮤테이션 3)', () => {
  // 깊이 3 · 세 종류. 손으로 계산한 기댓값(합성 순서를 뒤집으면 전혀 다른 수가 된다):
  //   S(p) = (2·px, 0.5·py)
  //   R    = rotate 30° about (4, 9)
  //   T    = +(7, 11)
  //   p=(0,0)  → S(0,0)   → R(5.0358984, −0.7942286) → (12.0358984, 10.2057714)
  //   p=(10,0) → S(20,0)  → R(22.3564065,  9.2057714) → (29.3564065, 20.2057714)
  const NESTED = svg(
    '<g transform="translate(7,11)"><g transform="rotate(30,4,9)">' +
      '<g transform="scale(2,0.5)"><rect x="0" y="0" width="10" height="4"/></g>' +
      '</g></g>',
  );

  it('깊이 3 중첩의 좌표가 M_바깥·M_중간·M_안쪽 순서로 나온다', () => {
    const { shapes } = read(NESTED);
    expect(shapes).toHaveLength(1);
    const [start, second] = shapes[0]!.commands as [
      { c: 'M'; x: number; y: number },
      { c: 'L'; x: number; y: number },
    ];
    expect(start.x).toBeCloseTo(12.0358984, 6);
    expect(start.y).toBeCloseTo(10.2057714, 6);
    expect(second.x).toBeCloseTo(29.3564065, 6);
    expect(second.y).toBeCloseTo(20.2057714, 6);
  });

  it('산출 도형에 변환 정보가 남아 있지 않다 — 전부 녹았다', () => {
    const { shapes } = read(NESTED);
    expect(Object.keys(shapes[0]!).sort()).toEqual([
      'closed',
      'commands',
      'evenOdd',
      'hasOwnStyle',
      'style',
    ]);
  });

  it('한 속성 안의 여러 함수는 왼쪽이 바깥이다', () => {
    // `translate(100,0) scale(3)` 은 점에 배율을 먼저 적용한 뒤 옮긴다 → (100+3·5, 0)
    const { shapes } = read(svg('<path d="M 5 0 L 5 0" transform="translate(100,0) scale(3)"/>'));
    const first = shapes[0]!.commands[0] as { c: 'M'; x: number; y: number };
    expect(first.x).toBeCloseTo(115, 9);
  });

  it('비균등 배율은 선 두께를 √|det| 로 옮기고 근사로 보고한다', () => {
    // scale(4,1) → |det| = 4 → √|det| = 2 → 두께 3 이 6 이 된다.
    const { shapes, notes } = read(
      svg('<g transform="scale(4,1)"><path d="M0 0 L1 1" stroke="#145a32" stroke-width="3"/></g>'),
    );
    expect(shapes[0]!.style.strokeWidth).toBeCloseTo(6, 9);
    expect(reasonCount(notes, 'nonUniformStrokeScale')).toBe(1);
  });

  it('|det| = 0 인 퇴화 변환은 버림으로 보고된다', () => {
    const { shapes, notes } = read(svg('<g transform="scale(0,3)"><rect width="10" height="4"/></g>'));
    expect(shapes).toHaveLength(0);
    expect(reasonCount(notes, 'degenerateTransformDropped')).toBe(1);
  });

  it('루트 <svg> 의 transform 은 적용되지 않고 근사로 보고된다', () => {
    const { shapes, notes } = read(
      svg('<path d="M 5 7 L 5 7"/>', 'viewBox="0 0 100 50" transform="translate(1000,1000)"'),
    );
    const first = shapes[0]!.commands[0] as { c: 'M'; x: number; y: number };
    expect(first.x).toBe(5);
    expect(first.y).toBe(7);
    expect(reasonCount(notes, 'rootTransformIgnored')).toBe(1);
  });
});

describe('감춤은 하위 트리로 흐른다 (REQ-05 · 뮤테이션 1·2)', () => {
  it('감춘 <g> 안의 도형은 하나도 들어오지 않는다 — 개수만 보고된다', () => {
    const { shapes, notes } = read(
      svg(
        '<g visibility="hidden"><rect width="10" height="4"/><circle cx="1" cy="1" r="2"/></g>' +
          '<rect x="20" y="20" width="5" height="5"/>',
      ),
    );
    // 자식들은 제 속성에 감춤을 한 마디도 말하지 않는다 — 전파가 없으면 셋이 들어온다.
    expect(shapes).toHaveLength(1);
    expect(reasonCount(notes, 'hiddenDropped')).toBe(2);
  });

  it('display:none 하위 트리도 같다 — 그리고 뒤집을 수 없다', () => {
    const { shapes, notes } = read(
      svg('<g style="display:none"><g visibility="visible"><rect width="10" height="4"/></g></g>'),
    );
    expect(shapes).toHaveLength(0);
    expect(reasonCount(notes, 'hiddenDropped')).toBe(1);
  });

  it('자식이 visibility="visible" 로 되살린다 — visibility 는 뒤집을 수 있다', () => {
    const { shapes, notes } = read(
      svg('<g visibility="hidden"><rect width="10" height="4" visibility="visible"/><circle r="2"/></g>'),
    );
    expect(shapes).toHaveLength(1);
    expect(reasonCount(notes, 'hiddenDropped')).toBe(1);
  });

  it('감춘 하위 트리의 미지원 내용은 보고하지 않는다 — 보고가 잡음이 되지 않는다', () => {
    // **글자는 이 무리에서 빠졌다**(결함 B 정정) — `<text>` 는 이제 지원되므로 감춰지면
    // 도형과 **같이** `hiddenDropped` 로 세어진다. 이 시험이 지키는 것은 그대로다:
    // 그리지 못하는 것들은 감춘 하위 트리에서 **한 줄도** 보고되지 않는다.
    const { notes } = read(
      svg('<g visibility="hidden"><image href="a.png"/><foreignObject width="1" height="1"/></g>'),
    );
    expect(reasonCount(notes, 'imageDropped')).toBe(0);
    expect(reasonCount(notes, 'foreignObjectDropped')).toBe(0);
    expect(notes).toHaveLength(0);
  });
});

describe('<use> 는 상한과 순환에서 멈춘다 (AC-E9 · 뮤테이션 4·5)', () => {
  it('깊이 5 사슬은 MAX_USE_DEPTH 에서 멈추고 그 가지가 버림으로 보고된다', () => {
    const chain =
      '<defs>' +
      '<g id="u5"><rect width="4" height="4"/></g>' +
      '<g id="u4"><use href="#u5"/></g>' +
      '<g id="u3"><use href="#u4"/></g>' +
      '<g id="u2"><use href="#u3"/></g>' +
      '<g id="u1"><use href="#u2"/></g>' +
      '</defs><use href="#u1"/>';
    const { shapes, notes } = read(svg(chain));
    expect(shapes).toHaveLength(0);
    expect(reasonCount(notes, 'useDepthDropped')).toBe(1);
  });

  it('깊이 4 사슬은 끝까지 산다 — 상한이 한 칸 일찍 닫히지 않는다', () => {
    const chain =
      '<defs>' +
      '<g id="v4"><rect width="4" height="4"/></g>' +
      '<g id="v3"><use href="#v4"/></g>' +
      '<g id="v2"><use href="#v3"/></g>' +
      '<g id="v1"><use href="#v2"/></g>' +
      '</defs><use href="#v1"/>';
    const { shapes, notes } = read(svg(chain));
    expect(shapes).toHaveLength(1);
    expect(reasonCount(notes, 'useDepthDropped')).toBe(0);
  });

  it('자기 참조는 멈춘다 — 무한 루프도 스택 초과도 아니다', () => {
    const started = Date.now();
    const { shapes, notes } = read(svg('<use id="self" href="#self"/>'));
    expect(Date.now() - started).toBeLessThan(2000);
    expect(shapes).toHaveLength(0);
    expect(reasonCount(notes, 'useDepthDropped')).toBe(1);
  });

  it('자기 자신을 품은 <g> 는 한 벌만 나온다 — 깊이 상한만으로는 네 벌이 된다', () => {
    // 순환 검출이 없으면 상한(4)에 닿을 때까지 같은 내용이 **되풀이 전개**된다:
    //   depth0 → #a(사각형 1) → depth1 → #a(사각형 2) → … → depth4 에서 정지 = 사각형 4개.
    // 깊이 상한은 **멈추게만** 하지 되풀이를 막지 못한다 — 그것이 두 가드가 따로 있는 이유다.
    const { shapes } = read(
      svg('<defs><g id="a"><rect width="4" height="4"/><use href="#a"/></g></defs><use href="#a"/>'),
    );
    expect(shapes).toHaveLength(1);
  });

  it('서로를 가리키는 두 <use> 도 멈춘다', () => {
    const { notes } = read(
      svg('<defs><use id="a" href="#b"/><use id="b" href="#a"/></defs><use href="#a"/>'),
    );
    expect(reasonCount(notes, 'useDepthDropped')).toBeGreaterThan(0);
  });

  it('외부 문서 참조는 버림이고 네트워크 요청이 일어나지 않는다', () => {
    const fetchSpy = vi.fn();
    const original = globalThis.fetch;
    globalThis.fetch = fetchSpy as unknown as typeof globalThis.fetch;
    try {
      const { shapes, notes } = read(svg('<use href="other.svg#b"/>'));
      expect(shapes).toHaveLength(0);
      expect(reasonCount(notes, 'externalRefDropped')).toBe(1);
      expect(fetchSpy).not.toHaveBeenCalled();
    } finally {
      globalThis.fetch = original;
    }
  });

  it('<symbol> 을 가리키면 그 내용이 살고 x/y 는 translate 로 접힌다', () => {
    const { shapes } = read(
      svg('<defs><symbol id="s"><path d="M 1 2 L 1 2"/></symbol></defs><use href="#s" x="30" y="40"/>'),
    );
    expect(shapes).toHaveLength(1);
    expect(shapes[0]!.commands[0]).toEqual({ c: 'M', x: 31, y: 42 });
  });

  it('xlink:href 도 같이 본다 — 구 도구가 그것만 쓴다', () => {
    const doc = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 50"><defs><rect id="r" width="4" height="4"/></defs><use xlink:href="#r"/></svg>`;
    expect(read(doc).shapes).toHaveLength(1);
  });

  it('<use> 의 width/height 는 무시하고 근사로 보고한다', () => {
    const { notes } = read(
      svg('<defs><rect id="r" width="4" height="4"/></defs><use href="#r" width="99" height="99"/>'),
    );
    expect(reasonCount(notes, 'useSizeIgnored')).toBe(1);
  });
});

describe('보고 세 갈래 (AC-07 · AC-E6 · 뮤테이션 6)', () => {
  it('버림이 개수를 말한다 — <image> · <style> · 중첩 <svg> · <foreignObject>', () => {
    // 개수를 말하는 본보기가 `<text>` 둘에서 `<image>` 둘로 바뀌었다(결함 B 정정) — 글자는
    // 이제 버림이 아니라 요소가 된다. 재는 성질은 그대로다: "있습니다" 가 아니라 "2개".
    //
    // `<style>` 의 두 규칙이 **복합 선택자**로 바뀌었다(결함 B 2차 정정). 앞의 `.a`·`.b` 는
    // 이제 실제로 적용되므로 버림에 오르지 않는다 — 쓴 것을 버렸다고 말하지 않는 것이 그
    // 정정이다. 여기서 재는 성질("버림이 개수를 말한다")은 **그대로**이며, 그 성질이 재어
    // 지려면 고정 입력이 정말로 옮기지 못하는 규칙을 들어야 한다.
    const { notes } = read(
      svg(
        '<image href="x.png"/><image href="y.png"/>' +
          '<style>g .a{fill:red}rect > .b{fill:blue}</style>' +
          '<svg width="10" height="10"/><foreignObject width="1" height="1"/>',
      ),
    );
    expect(reasonCount(notes, 'imageDropped')).toBe(2);
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(2);
    expect(reasonCount(notes, 'nestedSvgDropped')).toBe(1);
    expect(reasonCount(notes, 'foreignObjectDropped')).toBe(1);
  });

  it('아무것도 이상하지 않은 파일에서 보고 항목이 0개다', () => {
    const { shapes, notes } = read(
      svg(
        '<path d="M0 0 L10 0 Z" fill="#c0392b"/>' +
          '<path d="M0 0 L10 5 Z" fill="#145a32"/>' +
          '<path d="M2 2 L8 8 Z" fill="#8e44ad"/>',
      ),
    );
    expect(shapes).toHaveLength(3);
    expect(notes).toEqual([]);
  });

  it('<defs> 안의 도형은 그려지지 않는다 — 그리지 않는 것을 버렸다고 말하지도 않는다', () => {
    // `<defs>` 안에 **그려질 수 있는** 도형을 둔다. 안 쓰는 그라디언트만으로는
    // `<defs>` 건너뜀 가드가 잔다 — `<linearGradient>` 자체가 어차피 그려지지 않아서
    // 건너뛰든 내려가든 결과가 같기 때문이다.
    const { shapes, notes } = read(
      svg('<defs><rect id="d" width="40" height="20"/></defs><path d="M0 0 L10 0 Z" fill="#c0392b"/>'),
    );
    expect(shapes).toHaveLength(1);
    expect(notes).toEqual([]);
  });

  it('metadata · title · desc · defs · 주석 · 미지 네임스페이스를 더해도 여전히 0개다', () => {
    const doc =
      `<svg xmlns="http://www.w3.org/2000/svg" xmlns:sodipodi="urn:sodipodi" viewBox="0 0 100 50">` +
      `<metadata>m</metadata><title>t</title><desc>d</desc>` +
      `<defs><linearGradient id="g"><stop stop-color="#8e44ad"/></linearGradient></defs>` +
      `<!-- 주석 --><sodipodi:namedview id="nv"/>` +
      `<path d="M0 0 L10 0 Z" fill="#c0392b" sodipodi:nodetypes="cc"/></svg>`;
    const { shapes, notes } = read(doc);
    expect(shapes).toHaveLength(1);
    expect(notes).toEqual([]);
  });

  it('그라디언트 참조는 첫 stop 색이 되고 근사로 보고된다', () => {
    const { shapes, notes } = read(
      svg(
        '<defs><linearGradient id="g"><stop stop-color="#8e44ad" stop-opacity="0.5"/>' +
          '<stop stop-color="#000000"/></linearGradient></defs>' +
          '<rect width="10" height="4" fill="url(#g)"/>',
      ),
    );
    expect(shapes[0]!.style.fill).toBe('rgba(142, 68, 173, 0.5)');
    expect(reasonCount(notes, 'gradientToSolid')).toBe(1);
  });

  it('그라디언트 안쪽 깊이 있는 stop 도 찾는다 — 도구가 <g> 로 감싸 낸다', () => {
    const { shapes } = read(
      svg(
        '<defs><linearGradient id="g"><g><stop stop-color="#8e44ad"/></g></linearGradient></defs>' +
          '<rect width="10" height="4" fill="url(#g)"/>',
      ),
    );
    expect(shapes[0]!.style.fill).toBe('#8e44ad');
  });

  it('모르는 SVG 요소는 조용히 지나간다 — 그려지지 않는 것을 버렸다고 말하지 않는다', () => {
    const { shapes, notes } = read(svg('<blah/><switch/><path d="M0 0 L10 0 Z" fill="#c0392b"/>'));
    expect(shapes).toHaveLength(1);
    expect(notes).toEqual([]);
  });

  it('풀 수 없는 참조는 씨앗 색으로 떨어지고 그 사실을 말한다', () => {
    const { shapes, notes } = read(svg('<rect width="10" height="4" fill="url(#nope)"/>'));
    expect(shapes[0]!.style.fill).toBe(SEED_COLOR);
    expect(reasonCount(notes, 'paintUnresolved')).toBe(1);
  });

  it('기본값이 아닌 preserveAspectRatio 는 근사로 보고된다', () => {
    const { notes } = read(
      svg('<rect width="10" height="4"/>', 'viewBox="0 0 100 50" preserveAspectRatio="xMinYMin slice"'),
    );
    expect(reasonCount(notes, 'preserveAspectRatioIgnored')).toBe(1);
  });
});

// --- <style> CSS ----------------------------------------------------------
//
// **여기서 묻는 것은 규칙표가 아니라 도형의 색이다.** `svgCssRules.test.ts` 는 `#id` 가
// 규칙표에 **들어가는지**를 재는데, 그 단언은 규칙을 **읽는 자리가 하나도 없어도** 초록이다
// (결함 A 가 꼭 그 형상으로 살아남았다). 그래서 아래 고정 입력은 **색의 유일한 출처**를
// 선택자 하나로 두고 `shapes[i].style` 을 묻는다.

describe('<style> 의 색이 그림에 닿는다 (결함 A)', () => {
  it('색의 출처가 .class 하나뿐일 때 색이 온다 — 이 길이 살아 있음을 먼저 못박는다', () => {
    const { shapes } = read(
      svg('<style>.only{fill:#c0392b}</style><rect class="only" width="4" height="4"/>'),
    );
    expect(shapes).toHaveLength(1);
    expect(shapes[0]!.style.fill).toBe('#c0392b');
    expect(shapes[0]!.hasOwnStyle).toBe(true);
  });

  it('색의 출처가 #id 하나뿐일 때도 색이 온다 — 씨앗 색으로 떨어지지 않는다', () => {
    const { shapes } = read(
      svg('<style>#only{fill:#c0392b}</style><rect id="only" width="4" height="4"/>'),
    );
    expect(shapes).toHaveLength(1);
    expect(shapes[0]!.style.fill).not.toBe(SEED_COLOR);
    expect(shapes[0]!.style.fill).toBe('#c0392b');
    expect(shapes[0]!.hasOwnStyle).toBe(true);
  });

  it('셋이 한 도형에 겹치면 #id 가 .class 를, .class 가 element 를 덮는다', () => {
    // cascade 도 specificity 도 없다 — **적용 순서**가 element → class → id 일 뿐이다.
    // 세 규칙이 같은 속성을 말하는 고정 입력이라야 그 순서가 관측된다.
    const { shapes } = read(
      svg(
        '<style>rect{fill:#111111}.mid{fill:#222222}#top{fill:#333333}</style>' +
          '<rect id="top" class="mid" width="4" height="4"/>' +
          '<rect class="mid" width="4" height="4"/>' +
          '<rect width="4" height="4"/>',
      ),
    );
    expect(shapes.map((s) => s.style.fill)).toEqual(['#333333', '#222222', '#111111']);
  });

  it('인라인 표현 속성이 #id 규칙을 이긴다 — 오늘의 우선순위를 못박는다', () => {
    const { shapes } = read(
      svg('<style>#only{fill:#c0392b}</style><rect id="only" fill="#145a32" width="4" height="4"/>'),
    );
    expect(shapes[0]!.style.fill).toBe('#145a32');
  });

  it('id 가 없는 도형은 #id 규칙을 집어 오지 않는다', () => {
    const { shapes } = read(
      svg('<style>#only{fill:#c0392b}</style><rect width="4" height="4"/>'),
    );
    // **씨앗 색은 이 층에 없다.** 문서가 칠을 한 마디도 말하지 않으면 `fill` 이 아예 없고
    // (`hasOwnStyle === false`), 008 의 `pathSeedStyle` 이 뒤에서 씨앗 색을 세운다 —
    // `SEED_COLOR` 가 여기 박혀 나오는 것은 `currentColor` 처럼 **말했으나 풀지 못한** 칠뿐이다.
    expect(shapes[0]!.style.fill).toBeUndefined();
    expect(shapes[0]!.hasOwnStyle).toBe(false);
  });
});

describe('<style> 보고는 **옮기지 못한 것만** 센다 (결함 B · AC-E6 · 위험 R7)', () => {
  it('적용된 규칙은 버림에 오르지 않는다 — 쓴 것을 버렸다고 말하지 않는다', () => {
    const { shapes, notes } = read(
      svg(
        '<style>.a{fill:#c0392b}circle{fill:#145a32}</style>' +
          '<rect class="a" width="4" height="4"/><circle cx="5" cy="5" r="2"/>',
      ),
    );
    // 켜져 있음을 먼저 못박는다 — 두 규칙이 실제로 그림에 닿았다.
    expect(shapes[0]!.style.fill).toBe('#c0392b');
    expect(shapes[1]!.style.fill).toBe('#145a32');
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(0);
  });

  it('선택자를 읽지 못한 규칙만 개수로 오른다', () => {
    const { notes } = read(
      svg(
        '<style>.a{fill:#c0392b}g .b{fill:#145a32}rect > path{fill:#8e44ad}</style>' +
          '<rect class="a" width="4" height="4"/>',
      ),
    );
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(2);
  });

  it('닫히지 않은 CSS 는 통째로 한 줄 오른다 — 읽지 못한 것이 조용히 사라지지 않는다', () => {
    const { notes } = read(
      svg('<style>.a{fill:#c0392b</style><rect class="a" width="4" height="4"/>'),
    );
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(1);
  });

  it('빈 <style> 는 아무 말도 하지 않는다', () => {
    const { notes } = read(svg('<style>   </style><rect width="4" height="4"/>'));
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(0);
  });
});

// --- 붙여 쓴 복합 선택자 ---------------------------------------------------
//
// **파싱 단언은 이 자리를 재지 못한다.** `rect.red` 는 규칙표에 `"rect.red"` 라는 칸으로
// 들어가지만, `getCSSPropertiesForElement` 이 만드는 열쇠는 `rect` · `.red` · `#top`
// 셋뿐이라 그 칸은 **아무도 열지 않는다**. 그래서 아래 고정 입력은 색의 **유일한 출처**를
// 붙여 쓴 복합 선택자 하나로 두고, 도형의 색과 보고 개수를 **함께** 묻는다 — 어느 한쪽만
// 물으면 "적용하지 않으면서 보고도 하지 않는" 자리가 다시 초록으로 지나간다.

describe('붙여 쓴 복합 선택자는 닿지 않으며 **그 사실이 보고에 오른다**', () => {
  // 세 꼴은 일러스트레이터·잉크스케이프가 실제로 뱉는 모양이다.
  const CASES: ReadonlyArray<readonly [string, string, string]> = [
    ['rect.red', 'rect.red{fill:#c0392b}', '<rect class="red" width="4" height="4"/>'],
    ['.cls-1.cls-2', '.cls-1.cls-2{fill:#c0392b}', '<rect class="cls-1 cls-2" width="4" height="4"/>'],
    ['rect#top', 'rect#top{fill:#c0392b}', '<rect id="top" width="4" height="4"/>'],
  ];

  for (const [name, css, body] of CASES) {
    it(`${name} — 칠이 오지 않고 버림이 한 줄 오른다`, () => {
      const { shapes, notes } = read(svg(`<style>${css}</style>${body}`));
      expect(shapes).toHaveLength(1);
      // **적용은 달라지지 않는다** — 이 규칙은 고치기 전에도 도형에 닿은 적이 없다.
      // 씨앗 색은 이 층에 없다: 문서가 칠을 말하지 못하면 `fill` 이 아예 없다.
      expect(shapes[0]!.style.fill).toBeUndefined();
      expect(shapes[0]!.hasOwnStyle).toBe(false);
      // **달라지는 것은 보고뿐이다** — 고치기 전에는 이 수가 0 이었다(실측).
      expect(reasonCount(notes, 'styleRuleDropped')).toBe(1);
    });
  }

  it('붙여 쓴 것 옆의 홑마디는 그대로 닿는다 — 좁히기가 멀쩡한 규칙을 데려가지 않는다', () => {
    const { shapes, notes } = read(
      svg(
        '<style>.ok{fill:#c0392b}rect.red{fill:#145a32}</style>' +
          '<rect class="ok red" width="4" height="4"/>',
      ),
    );
    expect(shapes[0]!.style.fill).toBe('#c0392b');
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(1);
  });

  it('이름 없는 홑점 `.` 은 더 이상 칠하지 않고 보고로 나온다', () => {
    // **고치기 전의 실측**: `class=" "` 는 `['','']` 로 갈라져 `.` 열쇠를 만들었고,
    // `.{fill:…}` 규칙이 **실제로 칠했다**(fill=#c0392b, 보고 0). 어느 브라우저도 `.` 을
    // 선택자로 읽지 않으므로(파싱 오류 → 규칙 통째 버림) 그 칠은 아무도 따라 하지 않는
    // 칠이었다. 이제 브라우저와 같이 버리되, **말없이 버리지 않는다**.
    const { shapes, notes } = read(
      svg('<style>.{fill:#c0392b}</style><rect class=" " width="4" height="4"/>'),
    );
    expect(shapes[0]!.style.fill).toBeUndefined();
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(1);
  });

  it('쉼표로 늘어놓은 선택자는 여전히 읽지 못하고 한 줄로 센다 — 이 고침이 건드리지 않는다', () => {
    const { shapes, notes } = read(
      svg('<style>rect, circle{fill:#c0392b}</style><rect width="4" height="4"/>'),
    );
    expect(shapes[0]!.style.fill).toBeUndefined();
    expect(reasonCount(notes, 'styleRuleDropped')).toBe(1);
  });

  it('선언이 비거나 망가진 덩이는 여전히 담기지도 세지도 않는다 (007 조용한 자리 둘 (나), 미착수)', () => {
    // **일부러 고치지 않은 자리다** — 잃는 칠이 없으므로 SPEC 0.4.0 이 미착수로 적어 두었다.
    // 여기 못을 박는 뜻은 고치는 것이 아니라, 이번 좁히기가 이 자리를 **건드리지 않았음**을
    // 재는 데 있다.
    expect(reasonCount(read(svg('<style>rect { }</style><rect width="4" height="4"/>')).notes, 'styleRuleDropped')).toBe(0);
    expect(reasonCount(read(svg('<style>rect { color }</style><rect width="4" height="4"/>')).notes, 'styleRuleDropped')).toBe(0);
  });
});

describe('viewBox 와 크기 읽기 (REQ-02)', () => {
  it('viewBox 네 수를 그대로 읽는다', () => {
    expect(read(svg('<rect width="1" height="1"/>')).viewBox).toEqual({
      minX: -13,
      minY: 7,
      width: 317,
      height: 181,
    });
  });

  it.each([
    ['원소 부족', 'viewBox="0 0"'],
    ['음수 치수', 'viewBox="0 0 -5 -5"'],
    ['비수치', 'viewBox="a b c d"'],
  ])('%s 인 viewBox 는 없는 것으로 읽는다', (_label, attr) => {
    expect(read(svg('<rect width="1" height="1"/>', attr)).viewBox).toBeUndefined();
  });

  it('width/height 는 단위 없음 또는 px 만 크기로 읽는다 — %는 크기가 아니다', () => {
    expect(read(svg('<rect width="1" height="1"/>', 'width="320" height="180"')).size).toEqual({
      width: 320,
      height: 180,
    });
    expect(read(svg('<rect width="1" height="1"/>', 'width="320px" height="180px"')).size).toEqual({
      width: 320,
      height: 180,
    });
    expect(read(svg('<rect width="1" height="1"/>', 'width="100%" height="100%"')).size).toBeUndefined();
  });
});

describe('DOM 경계 (AC-E10 · 불변식 K3 · K4 · K7)', () => {
  it('파싱 결과를 살아 있는 문서에 붙이는 자리가 하나도 없다', () => {
    for (const { name, text } of productionSources()) {
      for (const forbidden of [
        'appendChild',
        'insertBefore',
        'replaceWith',
        'importNode',
        'adoptNode',
        'document.body',
        'document.createElement',
      ]) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });

  it('SVG 기하 DOM API 를 하나도 쓰지 않는다', () => {
    for (const { name, text } of productionSources()) {
      for (const forbidden of [
        'getBBox',
        'getTotalLength',
        'getPointAtLength',
        'pathSegList',
        'getCTM',
        'getComputedStyle',
      ]) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });

  it('DOMParser · Document · Element 를 참조하는 제품 파일이 정확히 하나다', () => {
    const referencing = productionSources()
      .filter(({ text }) => /\bDOMParser\b|\bDocument\b|\bElement\b/.test(text))
      .map(({ name }) => name);
    expect(referencing).toEqual(['svgDocument.ts']);
  });

  it('<script> 가 든 문서를 읽어도 전역이 오염되지 않다', () => {
    const marker = '__svg_import_script_marker__';
    const scoped = globalThis as unknown as Record<string, unknown>;
    expect(scoped[marker]).toBeUndefined();
    read(svg(`<script>globalThis['${marker}'] = 1;</script><rect width="4" height="4"/>`));
    expect(scoped[marker]).toBeUndefined();
  });
});
