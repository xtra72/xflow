// 분할 · 감김 · 상한 시험 (SPEC-CANVAS-007 M7 · AC-06 · AC-E7).
//
// **E7 이 이 파일의 고정 입력을 정한다.** 명령 상한에 닿지 않는 작은 파일로는 §명령 상한
// 결정 **전부**가 실행되지 않는다 — 감김 무리 분할도, 거절도, 상자 공유도. 그리고 **가장
// 나쁜 결함**(257번째 명령을 만들어 저장 왕복에서 잘리는 것)이 그대로 통과한다. 그래서
// 고정 입력은 명령 300개짜리 도형과 명령 360개짜리 세 조각 도형과 **도넛**을 함께 든다.
//
// **도넛은 채움으로 잰다.** 히트 규칙(윤곽까지의 거리)은 구멍이 사라져도 **같은 답을
// 낸다** — 008 이 감김 가드를 히트 규칙으로 재려다 걸린 그 자리다. 그래서 이 파일은
// `isInsidePath`(감기 수 합 = nonzero 채움 규칙)로 잰다. 그 함수는 `canvasHitTest` 가 아니라
// `pathFlatten` 에 있고, 히트 판정은 거기에 **거리 판정을 더해** 쓴다 — 즉 여기서 쓰는
// 자는 채움만 보는 자다.
//
// **왕복이 이 파일의 유일한 진짜 가드다**(위험 R4). 산출만 재는 시험은 "저장할 땐 맞고
// 다시 열면 잘린" 결함을 **통과시킨다** — 그 결함은 `parseCanvasConfig` 를 지나야 드러난다.
//
// **확인한 뮤테이션(E12)** — "→" 뒤가 빨개지는 단언이다.
//   1. `splitByCommandLimit` 를 무리 대신 **부분 경로 단위**로 나누면 → "상한을 넘는 도넛은
//      거절된다" 가 빨개진다(조각 둘이 나오고 구멍이 사라진다).
//   2. 거절(`undefined`)을 **절단**(`piece.slice(0, limit)`)으로 바꾸면 → "왕복 뒤에도 명령
//      수가 같다" 와 "산출 전부가 상한 이하다" 가 빨개진다.
//   3. `applyEvenOddWinding` 을 항등으로 바꾸면 → "evenodd 도넛이 구멍을 지킨다" 가
//      빨개진다. **두 부분 경로가 이미 반대 방향인 고정 입력에서는 빨개지지 않는다** —
//      그래서 고정 입력의 안팎이 **같은 방향**이다.
//   4. `reverseSubPath` 에서 두 제어점 맞바꿈을 빼면 → "뒤집은 곡선이 같은 자리를 지난다" 가
//      빨개진다(끝점만 맞고 배가 반대로 부푼다 — 구멍 시험으로는 보이지 않는다).
//   5. 요소 수 상한의 거절을 **앞의 64개만 가져오기**로 바꾸면 → "넘으면 하나도 만들지
//      않는다" 가 빨개진다.
//   6. 명령 총수 상한을 지우면 → "명령 총합이 넘으면 거절한다" 가 빨개진다.
//   7. `boxContains` 의 넓이 비교(`>`)를 `>=` 로 바꾸면 → "같은 상자인 두 부분 경로는 서로를
//      품지 않는다" 가 빨개진다(서로가 서로를 품어 무리가 지어진다).

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type BoxGeometry, type CanvasSize } from '../canvasConfig';
import { projectPathPoints, type PxBox } from '../canvasGeometry';
import { flattenPath, isInsidePath, FLATTEN_TOLERANCE_PX } from '../shapes/pathFlatten';
import { MAX_PATH_COMMANDS, type PathCommand } from '../shapes/pathTypes';

import {
  fitBox,
  planSvgImport,
  type ImportedPathSpec,
  type ImportedShapeSpec,
} from './svgImportPlan';
import { parseSvgPathData } from './svgPathData';
import {
  applyEvenOddWinding,
  containmentDepths,
  reverseSubPath,
  signedArea,
  splitByCommandLimit,
  splitSubPaths,
  windingGroups,
} from './svgImportSplit';
import { MAX_IMPORT_COMMANDS, MAX_IMPORT_ELEMENTS } from './svgImportTypes';

const VIEW_BOX = { minX: -13, minY: 7, width: 317, height: 181 } as const;
const CANVAS: CanvasSize = { width: 500, height: 400 };

/** `d` 한 줄을 **사용자 단위** 명령 목록으로. 로컬 정수로 옮기기 전의 모습이다. */
const parse = parseSvgPathData;

function svg(body: string): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-13 7 317 181">${body}</svg>`;
}

function plan(text: string) {
  const result = planSvgImport(text, CANVAS);
  if (!result.ok) throw new Error(`거절됨: ${result.refusal.reason}`);
  return result;
}

/**
 * 사각 윤곽 하나를 `n` 개의 `L` 로 잘게 그린 `d` 조각. 시계 방향(양의 넓이)이다.
 * 명령 수 = 1(`M`) + n(`L`) + 1(`Z`).
 */
function denseRect(x: number, y: number, w: number, h: number, n: number): string {
  const points: string[] = [];
  const perimeter = 2 * (w + h);
  for (let i = 1; i <= n; i += 1) {
    let t = (i / n) * perimeter;
    if (t <= w) points.push(`${x + t} ${y}`);
    else if ((t -= w) <= h) points.push(`${x + w} ${y + t}`);
    else if ((t -= h) <= w) points.push(`${x + w - t} ${y + h}`);
    else points.push(`${x} ${y + h - (t - w)}`);
  }
  return `M ${x} ${y} L ${points.join(' L ')} Z`;
}

/** 축척 0.5 — **단위 축척이 아니다**(시험 규율: 축척 1 은 나눗셈을 항등으로 만든다). */
function pxBox(box: { x: number; y: number; w: number; h: number }): PxBox {
  return { x: box.x * 0.5, y: box.y * 0.5, w: box.w * 0.5, h: box.h * 0.5 };
}

/** 문서 전체가 놓이는 틀 — 요소 상자가 아니라 **문서 배치**다. */
const DOC_BOX = fitBox(VIEW_BOX, CANVAS);

/**
 * 사용자 좌표 한 점을 **문서 배치를 지나** px 까지 옮긴다.
 *
 * 도형마다 제 상자를 든 뒤로는 요소의 정규화 틀(`frame`)이 도형마다 다르므로, 질의 점을
 * 그 틀로 옮기면 "재는 자와 재는 대상이 같은 수" 가 되어 아무것도 재지 못한다. 문서 배치로
 * 옮기면 **상자와 재정규화된 명령의 이음매**를 잰다 — 상자만 좁히고 좌표를 두거나 좌표만
 * 옮기고 상자를 두면 이 점이 도형 밖으로 나간다.
 */
function pxOf(user: { x: number; y: number }): { x: number; y: number } {
  return {
    x: (DOC_BOX.x + ((user.x - VIEW_BOX.minX) * DOC_BOX.w) / VIEW_BOX.width) * 0.5,
    y: (DOC_BOX.y + ((user.y - VIEW_BOX.minY) * DOC_BOX.h) / VIEW_BOX.height) * 0.5,
  };
}

/** **채움 규칙으로** 안쪽인가 — 히트 규칙(윤곽까지의 거리)이 아니다. */
function filledAt(commands: readonly PathCommand[], box: PxBox, user: { x: number; y: number }): boolean {
  const flat = flattenPath(projectPathPoints(commands, box), FLATTEN_TOLERANCE_PX);
  return isInsidePath(flat, pxOf(user));
}

/** 3차 베지어 위의 점 하나. 매개변수 `t` 를 직접 넣어 **중간점 대칭의 함정**을 피한다. */
function cubicAt(
  x0: number,
  y0: number,
  cmd: { x1: number; y1: number; x2: number; y2: number; x: number; y: number },
  t: number,
): { x: number; y: number } {
  const u = 1 - t;
  const w0 = u * u * u;
  const w1 = 3 * u * u * t;
  const w2 = 3 * u * t * t;
  const w3 = t * t * t;
  return {
    x: w0 * x0 + w1 * cmd.x1 + w2 * cmd.x2 + w3 * cmd.x,
    y: w0 * y0 + w1 * cmd.y1 + w2 * cmd.y2 + w3 * cmd.y,
  };
}

/**
 * 경로 spec 만 골라 낸다. **원시형은 명령을 나르지 않으므로** 명령 상한을 재는 자리에서
 * 셀 것이 없다 — 이 골라 냄이 곧 그 사실의 선언이고, 고른 뒤의 검사는 **골라진 것 전부**를
 * 지나므로 "표본이 아니라 전수" 는 그대로 참이다.
 */
function pathSpecs(shapes: readonly ImportedShapeSpec[]): ImportedPathSpec[] {
  return shapes.filter((sh): sh is ImportedPathSpec => sh.kind === 'path');
}

/** 산출을 실제 config 파서에 왕복시킨다 — "저장했다 다시 열었다" 의 기계적 재현. */
function roundTripCommandCounts(
  shapes: readonly { commands: readonly PathCommand[]; box: BoxGeometry }[],
): number[] {
  const raw = JSON.parse(
    JSON.stringify({
      canvas: CANVAS,
      elements: shapes.map((shape, i) => ({
        id: `el-${i + 1}`,
        kind: 'path',
        geometry: shape.box,
        path: shape.commands,
        style: {},
      })),
    }),
  ) as unknown;
  return parseCanvasConfig(raw).elements.map((el) => (el.kind === 'path' ? el.path.length : -1));
}

// 도넛 — **안팎이 같은 방향**이다. 반대 방향으로 그리면 evenodd 변환이 무동작이 되어
// 뮤테이션 3 이 물지 않는다.
const DONUT_OUTER = 'M 0 20 L 200 20 L 200 120 L 0 120 Z';
const DONUT_INNER = 'M 50 45 L 150 45 L 150 95 L 50 95 Z';
const DONUT_D = `${DONUT_OUTER} ${DONUT_INNER}`;
/** 구멍 한가운데. 변환이 없으면 채워지고, 있으면 뚫린다. */
const HOLE_POINT = { x: 100, y: 70 };
/** 고리 안(구멍 밖). 어느 쪽이든 채워져야 한다 — 이 점이 비면 도형이 통째로 사라진 것이다. */
const RING_POINT = { x: 25, y: 70 };

describe('부분 경로 자르기', () => {
  it('M 마다 새 부분 경로가 열린다', () => {
    expect(splitSubPaths(parse(DONUT_D))).toHaveLength(2);
  });

  it('Z 뒤에 M 없이 이어지면 그것도 새 부분 경로이고, 시작점은 닫은 자리다', () => {
    const subs = splitSubPaths([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10, y: 0 },
      { c: 'Z' },
      { c: 'L', x: 20, y: 20 },
    ]);
    expect(subs).toHaveLength(2);
    // 두 번째의 시작점은 **직전 점(10,0)이 아니라** 닫은 부분 경로의 시작점(0,0)이다.
    expect(subs[1]![0]).toEqual({ c: 'M', x: 0, y: 0 });
  });

  it('M 없이 시작한 목록에는 세울 부분 경로가 없다 — 예외가 아니다', () => {
    expect(splitSubPaths([{ c: 'L', x: 1, y: 1 }])).toEqual([]);
  });
});

describe('감김과 포함 (AC-E7 · 뮤테이션 3·4·7)', () => {
  it('도넛의 두 부분 경로는 한 무리다 — 포함 관계로 이어져 있다', () => {
    const subs = splitSubPaths(parse(DONUT_D));
    expect(containmentDepths(subs)).toEqual([0, 1]);
    expect(windingGroups(subs)).toEqual([[0, 1]]);
  });

  it('같은 상자인 두 부분 경로는 서로를 품지 않는다', () => {
    const subs = splitSubPaths(parse(`${DONUT_OUTER} ${DONUT_OUTER}`));
    expect(windingGroups(subs)).toEqual([[0], [1]]);
  });

  it('떨어진 두 부분 경로는 두 무리다', () => {
    const subs = splitSubPaths(parse('M 0 20 L 40 20 L 40 60 Z M 100 20 L 140 20 L 140 60 Z'));
    expect(windingGroups(subs)).toEqual([[0], [1]]);
  });

  it('뒤집은 곡선이 같은 자리를 지난다 — 제어점도 함께 맞바뀐다', () => {
    // **비대칭 제어점**이어야 이 시험이 무언가를 잰다. 대칭 제어점(`C 10 0 30 0 40 20`)은
    // 맞바꿔도 같은 곡선이라 결함을 통째로 숨긴다 — 대칭 고정 입력의 함정 그대로다.
    const curve = parse('M 0 20 C 5 0 15 40 40 20 L 40 60 Z');
    const forward = splitSubPaths(curve)[0]!;
    const back = reverseSubPath(forward);
    expect(Math.sign(signedArea(back))).toBe(-Math.sign(signedArea(forward)));

    const outbound = forward[1]!;
    const inbound = back[back.length - 2]!;
    expect(outbound.c).toBe('C');
    expect(inbound.c).toBe('C');
    if (outbound.c !== 'C' || inbound.c !== 'C') return;
    // 정방향의 t=0.25 지점과 역방향의 t=0.75 지점은 **같은 점**이다.
    const a = cubicAt(0, 20, outbound, 0.25);
    const b = cubicAt(inbound === back[back.length - 2] ? 40 : 0, 20, inbound, 0.75);
    expect(a.x).toBeCloseTo(b.x, 9);
    expect(a.y).toBeCloseTo(b.y, 9);
    // 맞바꾸지 않으면 (7.65625, 25.625) 가 나온다 — 위 두 단언이 그것을 잡는다.
    expect(a.x).toBeCloseTo(4.84375, 9);
    expect(a.y).toBeCloseTo(14.375, 9);
  });

  it('evenodd 도넛이 구멍을 지킨다 — 채움으로 잰다', () => {
    const result = plan(svg(`<path d="${DONUT_D}" fill-rule="evenodd" fill="#c0392b"/>`));
    expect(result.shapes).toHaveLength(1);
    const box = pxBox(pathSpecs(result.shapes)[0]!.box);
    const commands = pathSpecs(result.shapes)[0]!.commands;
    expect(filledAt(commands, box, RING_POINT)).toBe(true);
    expect(filledAt(commands, box, HOLE_POINT)).toBe(false);
  });

  it('evenodd 변환은 성공한 경우에도 언제나 근사로 보고된다', () => {
    const result = plan(svg(`<path d="${DONUT_D}" fill-rule="evenodd" fill="#c0392b"/>`));
    expect(result.report.notes.map((n) => n.reason)).toContain('evenOddWinding');
  });

  it('부분 경로가 하나뿐이면 evenodd 변환은 무동작이다', () => {
    const one = parse(DONUT_OUTER);
    expect(applyEvenOddWinding(one)).toEqual(one);
  });

  it('서로 교차하는 부분 경로에서도 예외가 없고 보고가 있다', () => {
    // 두 상자가 겹치기만 하고 어느 쪽도 다른 쪽을 품지 않는다 — 두 규칙이 원리적으로
    // 다른 자리이며, 시험은 결과가 다를 수 있음을 **명시적으로 허용**한다.
    const crossing = 'M 0 20 L 120 20 L 120 80 L 0 80 Z M 60 50 L 200 50 L 200 110 L 60 110 Z';
    const result = plan(svg(`<path d="${crossing}" fill-rule="evenodd" fill="#c0392b"/>`));
    expect(result.shapes).toHaveLength(1);
    expect(result.report.notes.map((n) => n.reason)).toContain('evenOddWinding');
  });
});

describe('명령 상한: 나누고, 안 되면 거절한다 (AC-06 · 뮤테이션 1·2)', () => {
  it('상한 안이면 나누지 않는다', () => {
    const pieces = splitByCommandLimit(parse(DONUT_D), MAX_PATH_COMMANDS);
    expect(pieces).toHaveLength(1);
  });

  it('떨어진 세 조각은 무리마다 하나씩 나뉜다 — 전부 상한 이하다', () => {
    const d = [denseRect(0, 20, 40, 30, 118), denseRect(80, 20, 40, 30, 118), denseRect(160, 20, 40, 30, 118)].join(' ');
    const result = plan(svg(`<path d="${d}" fill="#c0392b"/>`));
    expect(result.shapes).toHaveLength(3);
    const pieces = pathSpecs(result.shapes);
    expect(pieces).toHaveLength(3);
    for (const shape of pieces) {
      expect(shape.commands.length).toBeLessThanOrEqual(MAX_PATH_COMMANDS);
    }
    // **AC-06 — 나뉜 조각들의 상자가 깊은 비교로 같다.** 조각들은 한 `<path>` 였으므로
    // 상자를 나누기 **전의** 도형에서 잰다. 조각마다 재면 도넛의 구멍이 바깥에 대해
    // 반올림만큼 어긋나고, 그 어긋남은 채움으로만 보인다.
    for (const shape of pieces) expect(shape.box).toEqual(pieces[0]!.box);
    // 켜져 있음: 그 상자가 조각 하나가 아니라 **셋 전부**를 감싼다. 세 사각이 x 0..200 에
    // 걸쳐 있으므로 상자 폭이 한 조각(40)의 폭보다 훨씬 넓다.
    expect(pieces[0]!.box.w).toBeGreaterThan(3 * 40 * (DOC_BOX.w / VIEW_BOX.width));
    // 왕복 뒤에도 명령 수가 같다 — **자르지 않았다**(위험 R4 의 유일한 가드).
    expect(roundTripCommandCounts(pieces)).toEqual(pieces.map((s) => s.commands.length));
  });

  it('한 부분 경로가 홀로 상한을 넘으면 그 도형을 거절한다 — 자르지 않는다', () => {
    const result = plan(svg(`<path d="${denseRect(0, 20, 200, 100, 298)}" fill="#c0392b"/>`));
    expect(result.shapes).toHaveLength(0);
    expect(result.report.notes.map((n) => n.reason)).toContain('commandLimitDropped');
  });

  it('상한을 넘는 도넛은 거절된다 — 무리를 쪼개 구멍을 없애지 않는다', () => {
    const d = `${denseRect(0, 20, 200, 100, 148)} ${denseRect(50, 45, 100, 50, 148)}`;
    const result = plan(svg(`<path d="${d}" fill-rule="evenodd" fill="#c0392b"/>`));
    // 부분 경로 단위로 나누면 조각 둘(각 150)이 나오고 **구멍이 사라진다**.
    expect(result.shapes).toHaveLength(0);
    expect(result.report.notes.map((n) => n.reason)).toContain('commandLimitDropped');
  });

  it('산출 요소 전부가 상한 이하다 — 표본이 아니라 전수 검사', () => {
    const d = [
      denseRect(0, 20, 40, 30, 118),
      denseRect(80, 20, 40, 30, 118),
      denseRect(160, 20, 40, 30, 118),
    ].join(' ');
    const result = plan(
      svg(
        `<path d="${d}" fill="#c0392b"/>` +
          `<path d="${DONUT_D}" fill-rule="evenodd" fill="#145a32"/>` +
          '<rect width="10" height="4"/>',
      ),
    );
    expect(result.shapes.length).toBeGreaterThan(0);
    // 켜져 있음: 문서에 섞어 둔 `<rect>` 가 실제로 원시형이 되었다. 그래야 이 시험이
    // "섞인 문서에서도 경로 쪽 상한이 지켜진다" 를 재는 것이지, 경로뿐인 문서를 재는 것이
    // 아니다.
    expect(result.shapes.some((sh) => sh.kind === 'rect')).toBe(true);
    const paths = pathSpecs(result.shapes);
    expect(paths.length).toBeGreaterThan(0);
    const overs = paths.filter((s) => s.commands.length > MAX_PATH_COMMANDS);
    expect(overs).toEqual([]);
    expect(roundTripCommandCounts(paths)).toEqual(paths.map((s) => s.commands.length));
  });
});

describe('가져오기 상한 둘 (AC-06 · 뮤테이션 5·6)', () => {
  it('요소 수가 넘으면 하나도 만들지 않고 실제 수와 상한을 말한다', () => {
    const over = MAX_IMPORT_ELEMENTS + 1;
    const body = Array.from({ length: over }, (_, i) => `<rect x="${i}" y="10" width="4" height="4"/>`).join('');
    const refused = planSvgImport(svg(body), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) {
      expect(refused.refusal.reason).toBe('tooManyElements');
      expect(refused.refusal.actual).toBe(over);
      expect(refused.refusal.limit).toBe(MAX_IMPORT_ELEMENTS);
    }
  });

  it('요소 수가 상한과 같으면 통과한다 — 경계가 한 칸 일찍 닫히지 않는다', () => {
    const body = Array.from({ length: MAX_IMPORT_ELEMENTS }, (_, i) => `<line x1="${i}" y1="10" x2="${i}" y2="20"/>`).join('');
    const result = plan(svg(body));
    expect(result.shapes).toHaveLength(MAX_IMPORT_ELEMENTS);
  });

  it('명령 총합이 넘으면 거절한다 — 요소 수는 상한 안인데도', () => {
    // 60개 × 172명령 = 10320. 요소 수(60)는 1024 이하이므로 요소 상한으로는 걸리지 않고,
    // 경로 하나의 명령(172)도 `MAX_PATH_COMMANDS`(256) 아래라 나뉘지도 않는다 — 그래서
    // 이 거절을 낼 수 있는 것은 **명령 총수 상한**뿐이다.
    const body = Array.from({ length: 60 }, (_, i) => {
      const points = Array.from({ length: 170 }, (_, k) => `${i + k} ${20 + k}`).join(' L ');
      return `<path d="M ${i} 20 L ${points} Z"/>`;
    }).join('');
    const refused = planSvgImport(svg(body), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) {
      expect(refused.refusal.reason).toBe('tooManyCommands');
      expect(refused.refusal.actual).toBeGreaterThan(MAX_IMPORT_COMMANDS);
      expect(refused.refusal.limit).toBe(MAX_IMPORT_COMMANDS);
    }
  });
});

