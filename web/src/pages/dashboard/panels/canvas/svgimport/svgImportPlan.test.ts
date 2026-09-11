// 가져오기 계획 시험 — viewBox · 상자 · 로컬 정규화 (SPEC-CANVAS-007 M6 · AC-05 · AC-E8).
//
// **E2 가 이 파일의 `viewBox` 를 정한다.** `"0 0 10000 10000"` 이나 그와 닮은 것은
// **매핑 전부를 통과시킨다** — 축척이 1 이고 원점이 0 이면 곱하지도 빼지도 않는 코드가
// 옳게 보이고, 정사각이면 **x·y 축척을 맞바꾼 결함**도 통과한다. 그래서 고정 입력은
// `"-13 7 317 181"` 이다: 원점 ≠ 0 · `minX` 는 음수인데 `minY` 는 양수(**부호가 다르다**) ·
// 비정사각 · 소수 축척.
//
// **E3**: `317` 도 `181` 도 `10000` 을 나누어떨어뜨리지 않는다 — 자투리가 없으면 반올림
// 방향 결함이 실패할 수 없다.
//
// **비대칭 도형을 함께 둔다.** 대칭 도형은 x·y 전치를 숨긴다 — 정사각형의 네 꼭짓점은
// 축을 맞바꿔도 같은 집합이다.
//
// **확인한 뮤테이션(E12)** — "→" 뒤가 빨개지는 단언이다.
//   1. `toLocalCommands` 에서 `− minX`/`− minY` 를 지우면 → "왼쪽 위가 로컬 원점이다" 가
//      빨개진다. **`viewBox="0 0 …"` 인 고정 입력에서는 빨개지지 않는다** — 그 자리에서
//      뺄셈은 항등이다. 그래서 원점이 0 이 아닌 고정 입력을 쓴다.
//   2. `localX`/`localY` 의 나누는 축을 맞바꾸면 → "비대칭 점이 제 축으로 간다" 가
//      빨개진다. **정사각 `viewBox` 에서는 빨개지지 않는다.**
//   3. `fitBox` 의 `Math.max(MIN_ELEMENT_EXTENT, …)` 를 지우면 → "납작한 문서의 상자가
//      파서 왕복을 견딘다" 가 빨개진다(기하가 통째로 씨앗으로 갈아 끼워진다).
//   4. `fitBox` 의 `Math.min(…)` 을 `Math.max(…)` 로 바꾸면 → "상자가 캔버스 안에 든다" 가
//      빨개진다.
//   5. 종횡비를 버리고 상자를 캔버스에 꽉 채우면(`w = canvas.width`) → "상자가 문서
//      종횡비를 든다" 가 빨개진다.
//   6. 바이트 상한 검사를 `read` **뒤로** 옮기면 → "파싱 함수가 불리지 않는다" 가
//      빨개진다.
//   7. `scaleStroke` 를 항등으로 바꾸면 → "선 두께가 상자÷viewBox 비로 옮겨진다" 가
//      빨개진다.
//   8. `resolveViewBox` 의 퇴화 검사를 지우면 → "가로선뿐인 문서는 거절된다" 가 빨개진다.
//   9. 요소 상자를 `tightCommandBounds` 가 아니라 `commandBounds`(제어점 껍질)로 재면 →
//      "상자 폭이 참 넓이에서 나온다" 가 빨개진다. **곡선이 없는 고정 입력에서는 빨개지지
//      않는다** — 직선만 있는 문서에서는 껍질과 참값이 같다.
//  10. 상자를 나뉜 **조각마다** 재면 → (분할 시험의) "나뉜 조각들의 상자가 같다" 가
//      빨개진다(AC-06).
//  11. 상자만 좁히고 정규화를 문서 `viewBox` 로 두면 → (분할 시험의) 도넛 채움이 빨개진다 —
//      그림이 제 상자 밖으로 통째로 나간다.
//  12. 잴 수 없는 도형의 폴백을 문서 틀에서 빼앗으면 → "잴 수 없는 도형은 문서 틀을 그대로
//      쓴다" 가 빨개진다.

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it, vi } from 'vitest';

import {
  DEFAULT_BOX_GEOMETRY,
  parseCanvasConfig,
  type CanvasSize,
  type PathElement,
} from '../canvasConfig';
import { PATH_LOCAL_EXTENT } from '../shapes/pathTypes';

import { commandBounds, fitBox, planSvgImport, resolveViewBox, toLocalCommands } from './svgImportPlan';
import { MAX_IMPORT_FILE_BYTES } from './svgImportTypes';

/** E2 · E3 의 고정 입력. 원점 ≠ 0 · 부호가 다르다 · 비정사각 · 안 나누어떨어진다. */
const VIEW_BOX = { minX: -13, minY: 7, width: 317, height: 181 } as const;
/** 시험 규율: 정사각 스테이지가 숨긴 결함 때문에 캔버스는 정사각이 아니다. */
const CANVAS: CanvasSize = { width: 500, height: 400 };

function svg(body: string, rootAttrs = 'viewBox="-13 7 317 181"'): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${rootAttrs}>${body}</svg>`;
}

function plan(text: string, canvas: CanvasSize = CANVAS) {
  const result = planSvgImport(text, canvas);
  if (!result.ok) throw new Error(`거절됨: ${result.refusal.reason}`);
  return result;
}

/** 산출을 실제 config 파서에 왕복시킨다 — "저장했다 다시 열었다" 의 기계적 재현. */
function roundTrip(box: { x: number; y: number; w: number; h: number }, path_: unknown) {
  const raw = JSON.parse(
    JSON.stringify({
      canvas: CANVAS,
      elements: [{ id: 'el-1', kind: 'path', geometry: box, path: path_, style: {} }],
    }),
  ) as unknown;
  const parsed = parseCanvasConfig(raw);
  return parsed.elements[0]!;
}

describe('viewBox 가 로컬 격자에 앉는다 (AC-05 · E2 · 뮤테이션 1·2)', () => {
  it('왼쪽 위가 로컬 원점이고 오른쪽 아래가 로컬 끝이다', () => {
    const local = toLocalCommands(
      [
        { c: 'M', x: -13, y: 7 },
        { c: 'L', x: 304, y: 188 },
      ],
      VIEW_BOX,
    );
    expect(local[0]).toEqual({ c: 'M', x: 0, y: 0 });
    expect(local[1]).toEqual({ c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT });
  });

  it('비대칭 점이 제 축으로 간다 — 축을 맞바꾸면 전혀 다른 수가 된다', () => {
    // x 는 가로 중간(158.5/317), y 는 세로 원점. 축을 맞바꾸면 158.5/181 = 8757 이 된다.
    const local = toLocalCommands([{ c: 'M', x: 145.5, y: 7 }], VIEW_BOX);
    expect(local[0]).toEqual({ c: 'M', x: 5000, y: 0 });
  });

  it('로컬 좌표를 clamp 하지 않는다 — 문서 밖 저술은 합법이다', () => {
    const local = toLocalCommands([{ c: 'M', x: -330, y: 7 + 362 }], VIEW_BOX);
    expect(local[0]).toEqual({ c: 'M', x: -10000, y: 20000 });
  });

  it('C 명령은 제어점까지 함께 옮겨진다', () => {
    const local = toLocalCommands(
      [{ c: 'C', x1: -13, y1: 7, x2: 145.5, y2: 7, x: 304, y: 188 }],
      VIEW_BOX,
    );
    expect(local[0]).toEqual({
      c: 'C',
      x1: 0,
      y1: 0,
      x2: 5000,
      y2: 0,
      x: PATH_LOCAL_EXTENT,
      y: PATH_LOCAL_EXTENT,
    });
  });

  it('비유한 좌표는 폴백 0 으로 떨어진다 — 예외가 아니다', () => {
    expect(toLocalCommands([{ c: 'M', x: Number.NaN, y: Number.POSITIVE_INFINITY }], VIEW_BOX)).toEqual([
      { c: 'M', x: 0, y: 0 },
    ]);
  });
});

describe('상자가 문서 종횡비를 든다 (AC-05 · 뮤테이션 4·5)', () => {
  it('상자의 w : h 가 viewBox 의 w : h 와 같다', () => {
    const box = fitBox(VIEW_BOX, CANVAS);
    expect(Math.abs(box.w / box.h - VIEW_BOX.width / VIEW_BOX.height)).toBeLessThan(0.01);
  });

  it('상자가 캔버스 안에 든다', () => {
    const box = fitBox(VIEW_BOX, CANVAS);
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.x + box.w).toBeLessThanOrEqual(CANVAS.width);
    expect(box.y + box.h).toBeLessThanOrEqual(CANVAS.height);
  });

  it('세로로 긴 문서도 캔버스 안에 든다 — 죄는 축이 바뀐다', () => {
    const box = fitBox({ minX: 0, minY: 0, width: 100, height: 900 }, CANVAS);
    expect(box.x + box.w).toBeLessThanOrEqual(CANVAS.width);
    expect(box.y + box.h).toBeLessThanOrEqual(CANVAS.height);
    expect(Math.abs(box.w / box.h - 100 / 900)).toBeLessThan(0.01);
  });

  it('도형마다 제 상자를 든다 — 계획은 무리가 함께 쓰는 상자를 들지 않는다 (결함 D3)', () => {
    const result = plan(svg('<rect width="10" height="4"/><circle cx="50" cy="50" r="9"/>'));
    expect(result.shapes).toHaveLength(2);
    // 형상 판정: 상자는 도형이 든다. `box` 키가 사라지면 여기서 빨개진다.
    expect(Object.keys(result.shapes[0]!).sort()).toEqual(['box', 'commands', 'hasOwnStyle', 'style']);
    // 그리고 계획에는 상자가 **없다** — 있으면 옛 공유 상자가 남아 있는 것이다.
    expect('box' in result).toBe(false);
    // 두 도형은 문서에서 서로 떨어져 있으므로 상자도 달라야 한다. 같으면 공유 상자다.
    expect(result.shapes[0]!.box).not.toEqual(result.shapes[1]!.box);
  });
});

describe('요소 상자가 곡선의 **참** 넓이를 든다 — 제어점 껍질이 아니다 (결함 D3)', () => {
  // `M 0 20 C 120 20 20 80 60 110` — 제어점 `x1 = 120` 이 껍질을 120 까지 벌리지만 곡선의
  // 실제 최대는 `t ≈ 0.411` 의 **61.46** 이다. 껍질로 재면 상자가 2.5 배 넓어지고, 그만큼
  // 손잡이 여덟이 그림에서 떨어져 선다 — 이 SPEC 이 고치려는 그 결함의 축소판이다.
  const CURVE = '<path d="M 0 20 C 120 20 20 80 60 110" fill="#c0392b"/>';

  it('상자 폭이 참 넓이(61.46)에서 나온다 — 껍질(120)에서가 아니다', () => {
    const result = plan(svg(CURVE));
    const box = result.shapes[0]!.box;
    const scaleX = fitBox(VIEW_BOX, CANVAS).w / VIEW_BOX.width;
    // 참값: round(61.46 × 400/317) = 78. 껍질값: round(120 × 400/317) = 151.
    expect(box.w).toBe(78);
    expect(box.w).toBeLessThan(Math.round(120 * scaleX));
  });

  it('곡선의 끝점이 로컬 격자의 끝에 닿지 **않는다** — 오른쪽 끝은 극값이 잡는다', () => {
    // 끝점 x = 60 이고 상자의 오른쪽 변은 극값 61.46 이므로 로컬 x 는 9762 다. 껍질로 재면
    // 60/120 = 5000 이 된다. 상자 폭만 재는 단언은 "상자는 좁혔는데 좌표는 옛 격자 그대로"
    // 라는 결함을 통과시키므로, 좌표 쪽에서도 같은 수를 확인한다.
    const result = plan(svg(CURVE));
    const last = result.shapes[0]!.commands[1]!;
    expect(last.c).toBe('C');
    if (last.c !== 'C') return;
    expect(last.x).toBe(9762);
  });

  it('잴 수 없는 도형은 문서 틀을 그대로 쓴다 — 요소를 잃지 않는다', () => {
    // `transform="scale(1e200)"` 이 좌표를 `±∞` 로 밀어 버린 도형(실측: `|det|` 이 0 이
    // 아니므로 문서 층을 통과한다). 잴 바운딩 박스가 없으므로 문서 틀로 떨어지며, 그것이
    // 007 이 배달했던 그 자리다 — 떨어질 곳을 없애면 요소가 캔버스 구석으로 찌부러진다.
    const result = plan(
      svg('<rect x="1e200" y="1e200" width="1e200" height="1e200" transform="scale(1e200)"/><rect width="10" height="4"/>'),
    );
    expect(result.shapes).toHaveLength(2);
    expect(result.shapes[0]!.box).toEqual(fitBox(VIEW_BOX, CANVAS));
    // 켜져 있음: 함께 둔 멀쩡한 사각은 **제 작은 상자**를 얻었다(둘 다 문서 틀이 아니다).
    expect(result.shapes[1]!.box).not.toEqual(fitBox(VIEW_BOX, CANVAS));
  });
});

describe('퇴화 상자를 만들지 않는다 (AC-E8 · 가정 A15 · 뮤테이션 3)', () => {
  it('납작한 문서의 상자가 파서 왕복을 견딘다 — 두 변 모두 최소 크기 이상이다', () => {
    // `0 0 1000 1` → 높이 = round(1 × 0.4) = 0. 죄지 않으면 그대로 저장되고,
    // 다시 열 때 `isDegenerateBox` 가 기하를 **통째로** 씨앗으로 갈아 끼운다.
    const flat = plan(svg('<line x1="0" y1="0.5" x2="1000" y2="0.5"/>', 'viewBox="0 0 1000 1"'));
    const box = flat.shapes[0]!.box;
    expect(box.w).toBeGreaterThanOrEqual(1);
    expect(box.h).toBeGreaterThanOrEqual(1);

    const reopened = roundTrip(box, flat.shapes[0]!.commands);
    expect(reopened.geometry).toEqual(box);
    expect(reopened.geometry).not.toEqual(DEFAULT_BOX_GEOMETRY);
    // **그리고 선이 상자 한가운데를 지난다.** 상자만 1 로 밀어 올리고 정규화의 나누는 수를
    // 0 으로 두면 좌표가 폴백 0 으로 내려앉아 선이 상자 **위 모서리**에 붙는다 — 상자 크기만
    // 재는 단언은 그 결함을 통과시킨다.
    expect(reopened.kind).toBe('path');
    const reopenedPath = (reopened as PathElement).path;
    expect(reopenedPath.every((cmd) => cmd.c !== 'Z' && cmd.y === PATH_LOCAL_EXTENT / 2)).toBe(true);
  });

  it('가로선 · 세로선 · 반지름 0 인 원도 같다', () => {
    for (const [body, attrs] of [
      ['<line x1="0" y1="40" x2="120" y2="40"/>', 'viewBox="0 0 120 0.4"'],
      ['<line x1="40" y1="0" x2="40" y2="120"/>', 'viewBox="0 0 0.4 120"'],
      ['<circle cx="1" cy="1" r="0"/><rect width="4" height="4"/>', 'viewBox="0 0 4 0.2"'],
    ] as const) {
      const result = plan(svg(body, attrs));
      // **전수다.** 도형 하나만 보면 둘째 도형의 퇴화가 통과한다.
      for (const shape of result.shapes) {
        expect(shape.box.w, body).toBeGreaterThanOrEqual(1);
        expect(shape.box.h, body).toBeGreaterThanOrEqual(1);
        expect(roundTrip(shape.box, shape.commands).geometry, body).not.toEqual(
          DEFAULT_BOX_GEOMETRY,
        );
      }
    }
  });
});

describe('viewBox 3단 폴백 (REQ-02 · 뮤테이션 8)', () => {
  it('선언된 viewBox 가 이긴다', () => {
    const outcome = resolveViewBox(VIEW_BOX, { width: 9, height: 9 }, []);
    expect(outcome).toEqual({ ok: true, viewBox: VIEW_BOX });
  });

  it('없으면 width/height 속성을 쓴다', () => {
    expect(resolveViewBox(undefined, { width: 320, height: 180 }, [])).toEqual({
      ok: true,
      viewBox: { minX: 0, minY: 0, width: 320, height: 180 },
    });
  });

  it('둘 다 없으면 변환을 녹인 뒤의 합집합 바운딩 박스를 쓴다', () => {
    const result = plan(
      svg(
        '<g transform="translate(100,200)"><rect x="0" y="0" width="40" height="20"/></g>',
        'id="no-size"',
      ),
    );
    // 합집합 = (100,200)~(140,220) → 그 도형 하나가 문서 전부이므로 상자 종횡비는 40 : 20 이다.
    const box = result.shapes[0]!.box;
    expect(Math.abs(box.w / box.h - 2)).toBeLessThan(0.01);
    // 그 합집합의 왼쪽 위가 로컬 원점이 된다.
    expect(result.shapes[0]!.commands[0]).toEqual({ c: 'M', x: 0, y: 0 });
  });

  it('합집합은 도형 **전부**를 감싼다 — 첫 도형만 보면 오른쪽 조각이 상자 밖으로 나간다', () => {
    const result = plan(
      svg('<rect x="0" y="0" width="40" height="20"/><rect x="60" y="0" width="40" height="20"/>', 'id="no-size"'),
    );
    // 합집합 = (0,0)~(100,20). **첫 도형만 보면 축척이 2.5 배가 되어 오른쪽 조각의 상자가
    // 캔버스 밖으로 나간다** — 그것이 이 시험이 재는 것이다. 종횡비로는 잴 수 없다:
    // 상대 배치는 축척에 불변이라 첫 도형만 본 폴백에서도 같은 비가 나온다(실측 확인).
    for (const shape of result.shapes) {
      expect(shape.box.x).toBeGreaterThanOrEqual(0);
      expect(shape.box.x + shape.box.w).toBeLessThanOrEqual(CANVAS.width);
      expect(shape.box.y + shape.box.h).toBeLessThanOrEqual(CANVAS.height);
    }
    // 그리고 두 조각의 **간격**이 문서의 뜻대로다 — 사이 20, 폭 40 이므로 폭의 절반이다.
    const [first, second] = [result.shapes[0]!.box, result.shapes[1]!.box];
    expect(second.x - (first.x + first.w)).toBeCloseTo(first.w / 2, 0);
  });

  it('합집합이 한 축이라도 퇴화하면 거절한다 — 담을 종횡비가 없다', () => {
    const refused = planSvgImport(svg('<line x1="0" y1="40" x2="120" y2="40"/>', 'id="x"'), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.reason).toBe('degenerateViewBox');
  });

  it('그릴 것이 하나도 없으면 emptyDocument 다 — 퇴화와 구분한다', () => {
    const refused = planSvgImport(svg('<text>글자뿐</text>', 'id="x"'), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.reason).toBe('emptyDocument');
  });

  it('viewBox 가 있으면 그릴 것이 없어도 계획은 선다 — 보고를 보여야 한다', () => {
    const result = plan(svg('<text>글자뿐</text><image href="a.png"/>'));
    expect(result.report.shapes).toBe(0);
    expect(result.report.notes.map((n) => n.reason).sort()).toEqual(['imageDropped', 'textDropped']);
  });
});

describe('바이트 상한은 파싱보다 먼저다 (REQ-03 · 위험 R10 · 뮤테이션 6)', () => {
  it('2MiB 를 넘으면 파싱 함수가 불리지 않는다', () => {
    const reader = vi.fn();
    const huge = `<svg xmlns="http://www.w3.org/2000/svg">${'<!-- x -->'.repeat(250_000)}</svg>`;
    expect(huge.length).toBeGreaterThan(MAX_IMPORT_FILE_BYTES);
    const refused = planSvgImport(huge, CANVAS, { readDocument: reader });
    expect(reader).not.toHaveBeenCalled();
    expect(refused.ok).toBe(false);
    if (!refused.ok) {
      expect(refused.refusal.reason).toBe('fileTooLarge');
      expect(refused.refusal.limit).toBe(MAX_IMPORT_FILE_BYTES);
      expect(refused.refusal.actual).toBeGreaterThan(MAX_IMPORT_FILE_BYTES);
    }
  });

  it('주입된 읽기 함수의 실패가 그대로 값으로 돌아온다 — 예외가 아니다', () => {
    const refused = planSvgImport(svg('<rect width="4" height="4"/>'), CANVAS, {
      readDocument: () => ({ ok: false, refusal: { reason: 'notSvg', actual: 0, limit: 0 } }),
    });
    expect(refused.ok).toBe(false);
    if (!refused.ok) expect(refused.refusal.reason).toBe('notSvg');
  });

  it('바이트를 문자 수가 아니라 UTF-8 로 센다 — 한글은 문자당 3바이트다', () => {
    const reader = vi.fn();
    // 문자 수로는 상한 아래지만 UTF-8 바이트로는 넘는다.
    const korean = `<svg xmlns="http://www.w3.org/2000/svg"><desc>${'가'.repeat(800_000)}</desc></svg>`;
    expect(korean.length).toBeLessThan(MAX_IMPORT_FILE_BYTES);
    expect(planSvgImport(korean, CANVAS, { readDocument: reader }).ok).toBe(false);
    expect(reader).not.toHaveBeenCalled();
  });
});

describe('선 두께와 보고 (REQ-05 · 뮤테이션 7)', () => {
  it('선 두께가 상자 ÷ viewBox 비로 옮겨진다', () => {
    const result = plan(svg('<path d="M0 0 L10 10" stroke="#145a32" stroke-width="3"/>'));
    const ratio = fitBox(VIEW_BOX, CANVAS).w / VIEW_BOX.width;
    expect(result.shapes[0]!.style.strokeWidth).toBeCloseTo(3 * ratio, 9);
    // 비가 1 이 아니어야 이 시험이 무언가를 잰다 — 항등 축척은 곱셈 결함을 숨긴다.
    expect(Math.abs(ratio - 1)).toBeGreaterThan(0.2);
  });

  it('보고가 도형 수 · 명령 총수 · 추정 바이트를 함께 말한다', () => {
    const result = plan(svg('<rect width="10" height="4"/><rect x="5" y="5" width="10" height="4"/>'));
    expect(result.report.shapes).toBe(2);
    expect(result.report.commands).toBe(10);
    expect(result.report.estimatedBytes).toBeGreaterThan(0);
    // 추정이 실제 직렬화 길이를 재는지 — 상수를 곱한 값이 아니다.
    const actual = JSON.stringify(
      result.shapes.map((s) => ({
        id: 'el-00',
        kind: 'path',
        geometry: s.box,
        path: s.commands,
        style: s.style,
      })),
    ).length;
    expect(result.report.estimatedBytes).toBe(actual);
  });
});

describe('바운딩 박스는 제어점까지 센다', () => {
  it('3차 곡선의 상자가 제어점을 감싼다 — 잉크가 상자 밖으로 나가지 않는다', () => {
    expect(commandBounds([{ c: 'C', x1: -5, y1: 0, x2: 5, y2: 40, x: 10, y: 0 }])).toEqual({
      minX: -5,
      minY: 0,
      maxX: 10,
      maxY: 40,
    });
  });

  it('Z 만 있는 목록에는 상자가 없다', () => {
    expect(commandBounds([{ c: 'Z' }])).toBeUndefined();
  });
});

describe('브라우저에게 크기를 묻지 않는다 (REQ-02 · 형상 판정)', () => {
  it('naturalWidth · <img> · createElement 가 svgimport/ 어디에도 없다', () => {
    const dir = path.dirname(new URL(import.meta.url).pathname);
    const sources = fs
      .readdirSync(dir)
      .filter((name) => name.endsWith('.ts') && !name.endsWith('.test.ts'))
      .map((name) => ({
        name,
        text: fs
          .readFileSync(path.join(dir, name), 'utf8')
          .replace(/\/\*[\s\S]*?\*\//g, ' ')
          .replace(/\/\/[^\n]*/g, ' '),
      }));
    for (const { name, text } of sources) {
      for (const forbidden of ['naturalWidth', 'naturalHeight', 'new Image', "createElement('img'"]) {
        expect(`${name}:${text.includes(forbidden)}`).toBe(`${name}:false`);
      }
    }
  });
});
