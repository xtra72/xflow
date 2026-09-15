// 견고성 · 경계값 · 예산 실측 (SPEC-CANVAS-007 M11 · REQ-07 · AC-E4 · AC-E5).
//
// **이 파일이 겨누는 것은 "빨갛게 실패하지 않는 실패" 다.** 예외를 밖으로 던지는 실패는
// 시끄러워서 발견되지만, 셋은 조용하다: `NaN` 이 섞인 좌표(그린 결과가 비어 보인다) ·
// 상한 판정의 부등호가 하나 어긋난 경계(정확히 상한인 파일이 거절된다) · **멈추지 않는
// 순회**(초록도 빨강도 아니고 그냥 끝나지 않는다). 셋째를 잡으려면 시간 상한이 있어야 하고,
// 그래서 병적 입력에는 **짧은 시간 상한**을 명시로 건다 — 걸리면 그 시험이 실패로 드러난다.
//
// **경계는 `>` 인가 `>=` 인가가 전부다.** "상한을 넘으면 거절" 은 **정확히 상한이면 받는다**
// 를 함께 뜻한다. 넘는 쪽만 재는 시험은 부등호가 하나 어긋난 코드를 통과시키고, 그 어긋남은
// "2MiB 짜리 파일이 왜 안 열리지" 로만 드러난다. 그래서 상한 셋 전부를 **정확히 그 값**에서
// 잰다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-07 · AC-E4 · AC-E5

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  isDegenerateLine,
  MIN_ELEMENT_EXTENT,
  parseCanvasConfig,
  type CanvasSize,
} from '../canvasConfig';
import { appendImportedElements } from '../canvasElementFactory';
import { MAX_PATH_COMMANDS, type PathCommand } from '../shapes/pathTypes';
import { SCRATCHPAD_MAX_BYTES } from '../scratchpad/scratchpadTypes';
import { planSvgImport, type ImportedShapeSpec, type SvgImportPlan } from './svgImportPlan';
import {
  MAX_IMPORT_COMMANDS,
  MAX_IMPORT_ELEMENTS,
  MAX_IMPORT_FILE_BYTES,
} from './svgImportTypes';

const CANVAS: CanvasSize = { width: 500, height: 400 };

/** 원점 ≠ 0 · `minX` 음수 · 비정사각 · 안 나누어떨어짐 (시험 규율 E2 · E3). */
const VIEW_BOX = 'viewBox="-13 7 317 181"';

function svg(body: string, attrs = VIEW_BOX): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${body}</svg>`;
}

function plan(text: string): SvgImportPlan {
  return planSvgImport(text, CANVAS);
}

/**
 * 경로 spec 의 명령 목록. **경로가 아니면 시험이 선다.**
 *
 * 이 함수를 지나는 자리들의 고정 입력은 전부 `<polygon>` 과 `<path>` — 캔버스에 그 종류가
 * 없어 경로로 남는 것들이다. 그러므로 이 던지기는 "명령을 세는 시험이 명령 없는 요소를
 * 조용히 0 으로 세지 않는다" 는 단언이며, 고정 입력이 원시형이 되는 날 빨개진다.
 */
function commandsOf(spec: ImportedShapeSpec): readonly PathCommand[] {
  if (spec.kind !== 'path') throw new Error(`경로가 아니다: ${spec.kind}`);
  return spec.commands;
}

/** 명령 하나의 좌표 전부. `Z` 는 좌표를 나르지 않는다. */
function coordsOf(cmd: PathCommand): number[] {
  switch (cmd.c) {
    case 'Z':
      return [];
    case 'M':
    case 'L':
      return [cmd.x, cmd.y];
    case 'C':
      return [cmd.x1, cmd.y1, cmd.x2, cmd.y2, cmd.x, cmd.y];
  }
}

/**
 * 산출 전체에 유한하지 않은 수가 하나도 없다. **표본이 아니라 전수다.**
 *
 * 갈래 셋을 **이름으로** 가른다 — 경로는 상자와 명령을, 사각형·타원은 상자만을, 선은 두
 * 끝점을 나른다. `default:` 로 뭉뚱그리면 새 갈래가 조용히 검사 밖으로 나간다.
 */
function expectAllFinite(result: SvgImportPlan): void {
  if (!result.ok) return;
  for (const shape of result.shapes) {
    // 기하는 **전수**로 잰다 — 도형마다 제 기하를 들므로 하나만 보면 나머지가 새어 나간다.
    const numbers =
      shape.kind === 'line'
        ? [shape.line.x1, shape.line.y1, shape.line.x2, shape.line.y2]
        : [shape.box.x, shape.box.y, shape.box.w, shape.box.h];
    for (const value of numbers) {
      expect(Number.isFinite(value), JSON.stringify(numbers)).toBe(true);
    }
    if (shape.kind === 'path') {
      for (const cmd of shape.commands) {
        for (const n of coordsOf(cmd)) {
          expect(Number.isFinite(n), JSON.stringify(cmd)).toBe(true);
        }
      }
    }
    const width = shape.style.strokeWidth;
    if (width !== undefined) expect(Number.isFinite(width)).toBe(true);
  }
}

// --- 망가진 입력 (AC-E4) ---------------------------------------------------

describe('망가진 입력은 값으로 실패한다 — 예외도 `NaN` 도 멈추지 않는 순회도 없다', () => {
  const BROKEN: ReadonlyArray<readonly [name: string, text: string]> = [
    ['빈 문자열', ''],
    ['공백뿐', '   \n\t  '],
    ['XML 이 아닌 글자', 'this is not xml at all'],
    ['형식은 맞으나 SVG 가 아닌 문서', '<note xmlns="urn:x"><to>a</to><body>hi</body></note>'],
    ['HTML 문서', '<!doctype html><html><body><p>hi</p></body></html>'],
    ['닫히지 않은 태그', '<svg xmlns="http://www.w3.org/2000/svg"><g></svg>'],
    ['루트가 svg 인데 네임스페이스가 없다', '<svg><rect width="4" height="4"/></svg>'],
    ['그릴 것이 하나도 없는 svg', svg('')],
    ['그리지 않는 것만 든 svg', svg('<defs><linearGradient id="g"/></defs><metadata>x</metadata>')],
    ['viewBox 없이 가로선 하나 — 합집합이 퇴화한다', '<svg xmlns="http://www.w3.org/2000/svg"><line x1="0" y1="40" x2="120" y2="40"/></svg>'],
    ['viewBox 원소 부족', svg('<rect width="4" height="4"/>', 'viewBox="0 0"')],
    ['viewBox 음수 치수', svg('<rect width="4" height="4"/>', 'viewBox="0 0 -5 -5"')],
    ['viewBox 가 수가 아니다', svg('<rect width="4" height="4"/>', 'viewBox="a b c d"')],
    ['d 가 비었다', svg('<path d=""/>')],
    ['d 가 글자다', svg('<path d="banana"/>')],
    ['d 가 좌표 없는 M', svg('<path d="M"/>')],
    ['d 에 NaN 이 섞였다', svg('<path d="M 1 NaN L 5 5"/>')],
    ['d 에 무한대가 섞였다', svg('<path d="M 1e999 0 L 5 5"/>')],
    ['transform 이 닫히지 않았다', svg('<g transform="rotate("><rect width="4" height="4"/></g>')],
    ['matrix 인자가 모자란다', svg('<g transform="matrix(1,2,3)"><rect width="4" height="4"/></g>')],
    ['호의 반지름이 0 이다', svg('<path d="M 0 0 A 0 0 0 0 1 10 10"/>')],
    ['호의 두 끝점이 같다', svg('<path d="M 5 5 A 30 20 0 1 1 5 5"/>')],
    ['rect 의 rx 가 변보다 크다', svg('<rect x="5" y="12" width="40" height="20" rx="99" ry="99"/>')],
    ['rect 치수가 음수다', svg('<rect x="5" y="12" width="-40" height="-20"/>')],
    ['circle 의 r 이 0 이다', svg('<circle cx="10" cy="20" r="0"/>')],
    ['polygon 의 점이 홀수다', svg('<polygon points="1 2 3"/>')],
    ['polyline 의 점이 하나다', svg('<polyline points="1 2"/>')],
  ];

  // **시간 상한을 짧게 건다.** 멈추지 않는 순회는 초록도 빨강도 아니라 "끝나지 않음" 이고,
  // 상한이 없으면 그 형상이 시험 결과에 나타나지 않는다(SPEC-CANVAS-006 이 물린 부류).
  it.each(BROKEN)('%s — 예외 없이 값이 돌아오고 유한하다', (_name, text) => {
    let result: SvgImportPlan | undefined;
    expect(() => {
      result = plan(text);
    }).not.toThrow();
    expect(result).toBeDefined();
    expect(typeof result?.ok).toBe('boolean');
    expectAllFinite(result!);
    if (result?.ok === true) {
      // 살아남은 도형이 있다면 상한을 지킨다 — 망가진 입력이 상한을 뚫는 길이 없다.
      // **원시형에는 명령이 없다**(상한을 한 칸도 먹지 않는다) — 그래서 갈래를 가른다.
      for (const shape of result.shapes) {
        if (shape.kind === 'path') {
          expect(shape.commands.length).toBeLessThanOrEqual(MAX_PATH_COMMANDS);
        }
      }
    } else {
      // 거절도 **값**이다 — 사유와 두 수를 함께 든다.
      expect(typeof result?.refusal.reason).toBe('string');
      expect(Number.isFinite(result?.refusal.actual)).toBe(true);
      expect(Number.isFinite(result?.refusal.limit)).toBe(true);
    }
  }, 5000);

  it('손상 좌표는 **그 명령 하나만** 버리고 나머지가 산다 (REQ-07)', () => {
    const result = plan(svg('<path d="M 0 0 L 10 10 L NaN 5 L 20 20 Z" fill="#c0392b"/>'));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 도형이 통째로 죽지 않았다 — 켜져 있음을 먼저 잰다.
    expect(result.shapes).toHaveLength(1);
    const commands = commandsOf(result.shapes[0]!);
    // `M L L Z` — 손상된 `L` 하나만 빠졌다(`M L L L Z` 였을 것).
    expect(commands.map((c) => c.c)).toEqual(['M', 'L', 'L', 'Z']);
    expectAllFinite(result);
  });
});

// --- 경계값: 부등호가 하나 어긋나면 여기서 드러난다 -------------------------

describe('상한은 **정확히 그 값에서** 받는다 (REQ-03 · `>` 이지 `>=` 가 아니다)', () => {
  /** UTF-8 바이트 길이를 정확히 `bytes` 로 맞춘 문서. 여백은 XML 주석으로 채운다. */
  function padTo(bytes: number): string {
    const head = `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}><rect x="5" y="12" width="40" height="20" fill="#c0392b"/><!--`;
    const tail = '--></svg>';
    const pad = bytes - head.length - tail.length;
    expect(pad).toBeGreaterThan(0);
    return head + 'x'.repeat(pad) + tail;
  }

  it('정확히 `MAX_IMPORT_FILE_BYTES` 인 파일은 **받는다**', () => {
    const text = padTo(MAX_IMPORT_FILE_BYTES);
    expect(new TextEncoder().encode(text).length).toBe(MAX_IMPORT_FILE_BYTES);
    const result = plan(text);
    expect(result.ok).toBe(true);
  }, 20000);

  it('한 바이트 넘으면 **파싱하기 전에** 거절한다', () => {
    const text = padTo(MAX_IMPORT_FILE_BYTES + 1);
    const result = plan(text);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.refusal.reason).toBe('fileTooLarge');
    expect(result.refusal.actual).toBe(MAX_IMPORT_FILE_BYTES + 1);
    expect(result.refusal.limit).toBe(MAX_IMPORT_FILE_BYTES);
  }, 20000);

  it('바이트는 **문자 수가 아니다** — 한글 문서가 상한을 세 배 헐겁게 지나지 않는다', () => {
    // 한 글자가 3바이트다. 문자 수로 재면 상한의 1/3 짜리 문서가 통과한다.
    const korean = `<svg xmlns="http://www.w3.org/2000/svg" ${VIEW_BOX}><title>${'가'.repeat(
      MAX_IMPORT_FILE_BYTES / 2,
    )}</title></svg>`;
    expect(korean.length).toBeLessThan(MAX_IMPORT_FILE_BYTES);
    expect(new TextEncoder().encode(korean).length).toBeGreaterThan(MAX_IMPORT_FILE_BYTES);
    const result = plan(korean);
    expect(result.ok).toBe(false);
  }, 20000);

  /** 점 `n` 개짜리 폴리곤 — 명령은 `M` + `L`×(n−1) + `Z` 로 `n + 1` 개다. */
  function polygon(points: number): string {
    const pts = Array.from({ length: points }, (_v, i) => `${i % 30} ${(i * 7) % 40}`).join(' ');
    return `<polygon points="${pts}" fill="#c0392b"/>`;
  }

  it('정확히 `MAX_IMPORT_ELEMENTS` 개면 **받는다**', () => {
    const result = plan(svg(polygon(4).repeat(MAX_IMPORT_ELEMENTS)));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.shapes).toHaveLength(MAX_IMPORT_ELEMENTS);
  });

  it('하나 더 많으면 거절하고 **실제 수와 상한을 함께** 말한다', () => {
    const result = plan(svg(polygon(4).repeat(MAX_IMPORT_ELEMENTS + 1)));
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.refusal.reason).toBe('tooManyElements');
    expect(result.refusal.actual).toBe(MAX_IMPORT_ELEMENTS + 1);
    expect(result.refusal.limit).toBe(MAX_IMPORT_ELEMENTS);
  });

  it('정확히 `MAX_IMPORT_COMMANDS` 개면 **받는다** — 요소 수와 명령 수를 함께 죈다', () => {
    // 요소 1024개 × 명령 10개 = 10240. **두 상한 모두 정확히 그 값**이라 어느 쪽 부등호가
    // 어긋나도 여기서 드러난다. 상한 둘을 같은 배(64→1024 · 640→10240)로 올렸으므로
    // 요소당 10 명령이라는 이 비는 그대로다 — 수를 상수에서 끌어와 그 사실을 못박는다.
    const perElement = MAX_IMPORT_COMMANDS / MAX_IMPORT_ELEMENTS;
    expect(perElement).toBe(10);
    const result = plan(svg(polygon(perElement - 1).repeat(MAX_IMPORT_ELEMENTS)));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.shapes).toHaveLength(MAX_IMPORT_ELEMENTS);
    const total = result.shapes.reduce((sum, s) => sum + commandsOf(s).length, 0);
    expect(total).toBe(MAX_IMPORT_COMMANDS);
    expect(result.report.commands).toBe(MAX_IMPORT_COMMANDS);
  });

  it('명령이 하나 더 많으면 거절한다 — 요소 수는 상한 안인데도', () => {
    const perElement = MAX_IMPORT_COMMANDS / MAX_IMPORT_ELEMENTS;
    const body = polygon(perElement - 1).repeat(MAX_IMPORT_ELEMENTS - 1) + polygon(perElement);
    const result = plan(svg(body));
    expect(result.ok).toBe(false);
    if (result.ok) return;
    // 요소 수(1024)는 상한 안이다 — 그래서 이 거절은 **명령 상한**이 낸 것이다.
    expect(result.refusal.reason).toBe('tooManyCommands');
    expect(result.refusal.actual).toBe(MAX_IMPORT_COMMANDS + 1);
    expect(result.refusal.limit).toBe(MAX_IMPORT_COMMANDS);
  });

  it('정확히 `MAX_PATH_COMMANDS` 인 도형 하나는 **나뉘지도 거절되지도 않는다**', () => {
    // `M` + `L`×254 + `Z` = 256.
    const result = plan(svg(polygon(MAX_PATH_COMMANDS - 1)));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.shapes).toHaveLength(1);
    expect(commandsOf(result.shapes[0]!)).toHaveLength(MAX_PATH_COMMANDS);
  });

  /**
   * 서로 **겹치지 않는** 닫힌 부분 경로 하나. 명령 수는 정확히 `count` 다(`M` + `L`×(count−2)
   * + `Z`). 두 개를 나란히 두면 **독립된 감김 무리 둘**이 된다 — 도넛처럼 하나가 다른 하나
   * 안에 들면 무리가 하나로 묶여 이 시험이 재려는 것이 사라진다.
   */
  function subPath(count: number, ox: number, oy: number): string {
    const inner = Array.from(
      { length: count - 2 },
      (_v, i) => `L ${ox + (i % 20)} ${oy + ((i * 3) % 20)}`,
    ).join(' ');
    return `M ${ox} ${oy} ${inner} Z`;
  }

  it('부분 경로 **둘**이 합쳐 정확히 상한이면 나누지 않는다 — `<=` 이지 `<` 가 아니다', () => {
    // 부분 경로가 하나뿐인 고정 입력에서는 이 부등호가 관측되지 않는다: 나누어 보아도
    // 무리가 하나뿐이라 같은 결과가 나온다(뮤테이션 M11-7 이 그 자리에서 초록으로 통과했다).
    // 그래서 **독립된 무리 둘**을 둔다.
    const doc = svg(`<path d="${subPath(128, 0, 20)} ${subPath(128, 150, 20)}" fill="#c0392b"/>`);
    const result = plan(doc);
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 켜져 있음: 합이 정확히 상한이다.
    const total = result.shapes.reduce((n, sh) => n + commandsOf(sh).length, 0);
    expect(total).toBe(MAX_PATH_COMMANDS);
    // **한 요소**다 — 나눌 필요가 없으면 나누지 않는다(한 그림이 두 줄로 갈라지지 않는다).
    expect(result.shapes).toHaveLength(1);
  });

  it('나뉜 조각이 **정확히 상한**이면 받는다 — 무리 판정도 `>` 이지 `>=` 가 아니다', () => {
    // 256 + 10 = 266 > 상한 → 나눈다. 나뉜 조각 하나가 **정확히 256** 이므로, 무리 판정의
    // 부등호가 하나 어긋나면 여기서 통째 거절로 뒤집힌다(뮤테이션 M11-7'').
    const doc = svg(
      `<path d="${subPath(MAX_PATH_COMMANDS, 0, 20)} ${subPath(10, 150, 20)}" fill="#c0392b"/>`,
    );
    const result = plan(doc);
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.shapes).toHaveLength(2);
    expect(result.shapes.map((sh) => commandsOf(sh).length).sort((a, b) => b - a)).toEqual([
      MAX_PATH_COMMANDS,
      10,
    ]);
    // 거절이 아니므로 보고에 명령 상한 항목이 오르지 않는다.
    expect(result.report.notes.some((n) => n.reason === 'commandLimitDropped')).toBe(false);
  });

  it('하나 더 많은 도형은 **잘리지 않는다** — 나누거나 거절한다 (위험 R4)', () => {
    const result = plan(svg(polygon(MAX_PATH_COMMANDS)));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 부분 경로가 하나뿐이라 나눌 무리가 없다 → 그 도형을 거절하고 보고한다.
    expect(result.shapes).toHaveLength(0);
    expect(result.report.notes.some((n) => n.reason === 'commandLimitDropped')).toBe(true);
    // **잘라 만든 요소가 하나도 없다.**
    for (const shape of result.shapes) {
      expect(commandsOf(shape).length).toBeLessThanOrEqual(MAX_PATH_COMMANDS);
    }
  });
});

// --- 저장 왕복 (위험 R4 의 유일한 가드) -------------------------------------

describe('산출은 저장 왕복을 지나도 명령 수가 같다 (AC-06 · 위험 R4)', () => {
  it('**전수**로 잰다 — 표본이 아니다', () => {
    const result = plan(svg(polygonSet()));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 켜져 있음: 상한에 가까운 도형이 실제로 들어 있다.
    expect(result.shapes.length).toBeGreaterThanOrEqual(2);
    expect(Math.max(...result.shapes.map((s) => commandsOf(s).length))).toBeGreaterThan(200);

    const { created } = appendImportedElements([], result.shapes);
    const config = {
      canvas: { ...CANVAS },
      elements: created.map((el) => ({ ...el })),
    };
    const reopened = parseCanvasConfig(JSON.parse(JSON.stringify(config)) as unknown);
    expect(reopened.elements).toHaveLength(created.length);
    for (const [i, el] of reopened.elements.entries()) {
      const before = created[i]!;
      // 고정 입력은 `<polygon>` 뿐이라 **전부 경로로 남는다** — 이 단언이 그 사실을 든다.
      expect(el.kind, `#${i}`).toBe('path');
      expect(before.kind, `#${i}`).toBe('path');
      if (el.kind !== 'path' || before.kind !== 'path') continue;
      // `parsePathCommands` 는 상한 초과분을 **조용히 자른다**(실측). 자른 흔적이 있으면
      // 여기서 수가 어긋난다 — "저장할 땐 맞고 다시 열면 잘린" 결함의 유일한 가드다.
      expect(el.path, `#${i}`).toEqual(before.path);
      expect(el.geometry, `#${i}`).toEqual(before.geometry);
      // 퇴화 상자였다면 파서가 기하를 **통째로** 씨앗으로 갈아 끼운다(가정 A15).
      expect(el.geometry, `#${i}`).not.toEqual(DEFAULT_BOX_GEOMETRY);
    }
  });

  function polygonSet(): string {
    const pts = (n: number): string =>
      Array.from({ length: n }, (_v, i) => `${i % 30} ${(i * 11) % 40}`).join(' ');
    return (
      `<polygon points="${pts(MAX_PATH_COMMANDS - 1)}" fill="#c0392b"/>` +
      `<polygon points="${pts(9)}" fill="#145a32"/>`
    );
  }
});

// --- 퇴화 상자 (AC-E8) ------------------------------------------------------

describe('퇴화 상자를 만들지 않는다 (REQ-06 · 가정 A15)', () => {
  const CASES: ReadonlyArray<readonly [name: string, body: string, attrs: string]> = [
    ['가로선 하나', '<line x1="0" y1="40" x2="120" y2="40"/>', 'viewBox="0 0 200 1"'],
    ['세로선 하나', '<line x1="40" y1="0" x2="40" y2="120"/>', 'viewBox="0 0 1 200"'],
    ['길이 0 인 선(점 하나)', '<line x1="10" y1="20" x2="10" y2="20"/>', 'viewBox="0 0 200 1"'],
    ['아주 납작한 viewBox', '<rect x="1" y="0" width="100" height="1"/>', 'viewBox="0 0 4000 1"'],
  ];

  it.each(CASES)('%s — 두 변 모두 `MIN_ELEMENT_EXTENT` 이상이고 왕복 뒤에도 씨앗이 아니다', (_n, body, attrs) => {
    const result = plan(svg(body, attrs));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    for (const shape of result.shapes) {
      if (shape.kind === 'line') expect(isDegenerateLine(shape.line)).toBe(false);
      else {
        expect(shape.box.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
        expect(shape.box.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
      }
    }

    const { created } = appendImportedElements([], result.shapes);
    // 도형이 하나라도 있어야 왕복이 무언가를 잰다.
    expect(created.length).toBeGreaterThanOrEqual(1);
    const reopened = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: { ...CANVAS }, elements: created })) as unknown,
    );
    for (const el of reopened.elements) {
      // **갈래마다 파서가 갈아 끼우는 씨앗이 다르다.** 종전에는 산출이 경로뿐이라
      // `DEFAULT_BOX_GEOMETRY` 하나만 보면 됐지만, 태그가 말한 도형이 원시형이 되면서
      // 선은 `DEFAULT_LINE_GEOMETRY` 라는 **다른 씨앗**을 만난다(`parseLineGeometry`).
      // 이 시험이 지키는 것은 "왕복이 기하를 통째로 갈아 끼우지 않는다" 이고, 그 명제는
      // 갈래마다 다른 씨앗을 상대로 다시 세워야 참으로 남는다.
      if (el.kind === 'line') {
        expect(el.geometry).not.toEqual(DEFAULT_LINE_GEOMETRY);
        expect(isDegenerateLine(el.geometry)).toBe(false);
        continue;
      }
      // 던지기로 좁힌다 — `expect` 는 타입을 좁히지 않으므로 아래 `el.geometry` 가
      // 연결선 갈래(기하가 없다 — SPEC-CANVAS-011 M4)까지 보게 된다. 실패의 뜻은 같다.
      if (el.kind !== 'rect' && el.kind !== 'ellipse' && el.kind !== 'path') {
        throw new Error(`상자 도형이 아니다: ${el.kind}`);
      }
      expect(el.geometry).not.toEqual(DEFAULT_BOX_GEOMETRY);
      const box = el.geometry as { w: number; h: number };
      expect(box.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
      expect(box.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    }
  });
});

describe('그리지 않는 것은 요소도 보고도 만들지 않는다 (§결정 2 · 위험 R7)', () => {
  // SVG 사양은 `r="0"` 인 원과 치수 0 인 사각을 **렌더 대상에서 뺀다**. 그러므로 이것들을
  // 건너뛰는 것이 곧 옳은 렌더이고, "버렸다" 고 보고하면 `<defs>` 를 버림으로 올리는 것과
  // 같은 잡음이 된다 — 잡음 섞인 보고는 읽히지 않고, 읽히지 않는 보고는 침묵과 같다.
  it.each([
    ['r=0 인 원', '<circle cx="10" cy="20" r="0"/>'],
    ['치수 0 인 사각', '<rect x="5" y="5" width="0" height="0"/>'],
    ['치수가 음수인 사각', '<rect x="5" y="5" width="-4" height="-4"/>'],
  ])('%s — 요소도 보고 항목도 만들지 않는다', (_n, body) => {
    const result = plan(svg(`<rect x="5" y="12" width="40" height="20" fill="#c0392b"/>${body}`));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 켜져 있음: 함께 둔 멀쩡한 사각 하나는 들어왔다(0개짜리 문서를 재고 있지 않다).
    expect(result.shapes).toHaveLength(1);
    expect(result.report.notes).toEqual([]);
  });
});

// --- 예산 실측 (AC-E5) ------------------------------------------------------

describe('AC-E5 — 최악 가져오기의 실제 직렬화 바이트를 잰다 (가정 A4 · 위험 R6)', () => {
  /**
   * 대시보드 snapshot PUT 의 상한.
   *
   * **이것은 서버가 실제로 강제한다** — `internal/api/handler/dashboard.go` 의
   * `maxDashboardPayloadBytes = 256 * 1024` 이며, 넘으면 413 Payload Too Large 로
   * 거부하고 **저장하지 않는다**. 이 수를 넘긴 config 는 "조금 큰 config" 가 아니라
   * **저장되지 않는 config** 다.
   */
  const DASHBOARD_BUDGET = 256 * 1024;

  /**
   * **최악은 명령 전부가 `C` 인 경우다**(`{"c":"C",…}` 가 67B 로 가장 길다). 요소와 명령을
   * 상한까지 정확히 채운다 — 요소마다 `C` 아홉과 `M` 하나(= 요소당 10 명령).
   */
  function worstCase(): string {
    const perElement = MAX_IMPORT_COMMANDS / MAX_IMPORT_ELEMENTS;
    const curve = Array.from(
      { length: perElement - 1 },
      (_v, i) => `C ${i} ${i + 1} ${i + 2} ${i + 3} ${i + 4} ${i + 5}`,
    ).join(' ');
    return svg(
      `<path d="M 0 0 ${curve}" fill="#c0392b" stroke="#145a32" stroke-width="3"/>`.repeat(
        MAX_IMPORT_ELEMENTS,
      ),
    );
  }

  // **실측값(이 시험이 잰 수)**: 807,854 B = 789 KB = 256KB 예산의 **308.17%**.
  //
  // **상한을 64/640 에서 1024/10240 으로 올리기 전에는 이 수가 50,424 B(19.24%)였고, 가정
  // A4 는 "21% 를 넘지 않는다" 였다. 그 가정은 이제 거짓이다.** 최악 가져오기는 예산을
  // 세 배 넘게 넘기며, 그런 config 는 PUT 에서 413 을 받아 **저장되지 않는다**.
  //
  // **이 시험은 이제 "예산 안에 있다" 를 지키지 않는다.** 지키는 것은 두 가지다 —
  //   ① 최악이 얼마인지를 **실측한 수로** 남긴다(손으로 센 상수는 형상이 바뀌는 날 조용히
  //      틀린다. 상한을 올린 이 날이 바로 그날이었다).
  //   ② 상한 둘을 정확히 채웠다는 사실. 못 채웠으면 "최악" 이 최악이 아니다.
  //
  // **줄일 곳은 `MAX_IMPORT_COMMANDS` 가 아니다.** 명령을 한 칸도 쓰지 않는 문서
  // (문구 1024 × 한글 256자 = 919,470 B · 350.75% — `svgImportNative.test.ts` §예산 의 B)가
  // 이미 예산을 세 배 넘긴다. 즉 명령 상한을 0 으로 두어도 예산은 지켜지지 않는다. 예산을
  // 지키는 유일한 길은 **바이트로 세운 한도**이며, 그것은 이 상한들을 되돌리는 일이 아니라
  // 따로 짓는 일이다(아직 짓지 않았다).
  it('요소 1024 · 명령 10240(전부 `C`) = 807,854 B = 256KB 예산의 308.17% — **예산을 넘긴다**', () => {
    const result = plan(worstCase());
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    // 켜져 있음: 상한 둘을 **정확히** 채웠다. 못 채웠으면 "최악" 이 최악이 아니다.
    expect(result.shapes).toHaveLength(MAX_IMPORT_ELEMENTS);
    expect(result.report.commands).toBe(MAX_IMPORT_COMMANDS);
    expect(result.shapes.every((s) => commandsOf(s).filter((c) => c.c === 'C').length === 9)).toBe(
      true,
    );

    const { created } = appendImportedElements([], result.shapes);
    const bytes = new TextEncoder().encode(JSON.stringify(created)).length;

    // **실측을 못박는다.** 이 수가 움직이면 형상이 바뀐 것이고, 그때 다시 재야 한다.
    expect(bytes).toBe(807854);

    // **예산을 넘긴다는 사실 자체를 못박는다** — 되돌아가 초록이 되는 날은 바이트 한도를
    // 지었거나 상한을 도로 내린 날이며, 어느 쪽이든 이 줄이 알려야 한다.
    expect(bytes).toBeGreaterThan(DASHBOARD_BUDGET);
    expect(bytes / DASHBOARD_BUDGET).toBeGreaterThan(3);

    // 추정이 실측과 크게 어긋나지 않는다 — 화면이 사용자에게 보여 주는 수가 그 추정이고
    // (`CanvasSvgImport` 가 "약 N KB" 로 읽는다), 지금은 **그 표시가 유일한 방어**다.
    expect(result.report.estimatedBytes).toBeGreaterThan(bytes * 0.8);
    expect(result.report.estimatedBytes).toBeLessThan(bytes * 1.2);

    // 서랍 한 항목으로 넣었을 때도 마찬가지다(`SCRATCHPAD_MAX_BYTES` = 256KB) — 한 건이
    // 서랍 한 칸의 세 배이므로, 이런 가져오기는 서랍에 **한 건도** 들어가지 않는다.
    expect(bytes).toBeGreaterThan(SCRATCHPAD_MAX_BYTES);
  });
});
