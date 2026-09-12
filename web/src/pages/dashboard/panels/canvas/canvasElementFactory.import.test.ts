// 가져온 요소를 만드는 입구 시험 (SPEC-CANVAS-007 M8 · AC-09 · AC-E12).
//
// **이 파일이 재는 것은 "가져온 요소가 카탈로그 경로와 구별되지 않는가" 하나다**(REQ-08).
// 구별되면 009(정점 편집기)가 출처별 분기를 갖게 되고, 004 의 `group` 이 담을 때도 갈래가
// 생긴다. 그래서 시험은 산출의 **형상**을 카탈로그 경로와 나란히 놓고 견준다.
//
// **계단이 이 파일에서 가장 잘 틀리는 자리다.** 카탈로그 도형을 연달아 놓을 때 계단이 하는
// 일(겹치지 않게 25 단위씩 어긋내기)이, 한 그림을 이루는 조각들에는 **결함**이 된다 —
// 문서에서 겹쳐 그려지던 조각들이 흩어져 그림이 무너진다. 고정 입력이 **여러 조각**이어야
// 그 결함이 관측된다(조각 하나짜리 고정 입력에서는 요소마다 더하든 한 번 더하든 결과가
// 같다).
//
// **결함 D3 정정 뒤 이 파일에서 달라진 것.** 도형은 이제 **저마다 제 상자**를 들고 온다.
// 그래서 "계단을 한 번만 더한다" 를 "요소들의 상자가 전부 같다" 로 재던 옛 단언은 성립하지
// 않는다 — 상자가 애초에 다르기 때문이다. 재야 하는 것은 값이 아니라 **델타**이고, 그
// 단언이 옛것보다 강하다: 옛 단언은 모든 상자가 같다는 우연에 기대어 계단을 재었지만
// 델타 단언은 상자가 저마다 달라도 계단만을 잰다. 고정 입력의 상자 셋은 서로 **다르다** —
// 같게 두면 이 시험이 옛 공유 상자를 다시 재고 있는 것이고, 요소마다 계단을 더하는 뮤테이션
// 하나만 잡을 뿐 상자 뒤섞임은 잡지 못한다.
//
// **확인한 뮤테이션(E12)** — "→" 뒤가 빨개지는 단언이다.
//   1. 계단을 요소마다 더하면(`seedOffset(next.length)` 을 반복문 안으로) → "조각들이
//      흩어지지 않는다" 가 빨개진다. **조각 하나짜리 고정 입력에서는 빨개지지 않는다.**
//   1b. 도형의 상자를 무시하고 첫 도형의 상자를 전부에 쓰면(옛 공유 상자로 되돌리면) →
//      "상자는 도형마다 다르다" 가 빨개진다. **상자가 같은 고정 입력에서는 빨개지지 않는다.**
//   2. 명령 사본(`{ ...cmd }`)을 참조로 바꾸면 → "명령을 사본으로 싣는다" 가 빨개진다.
//   3. `pathSeedStyle` 갈래를 지우고 언제나 원본 스타일을 쓰면 → "말하지 않은 도형은
//      씨앗을 입는다" 가 빨개진다.
//   4. `hasOwnStyle` 갈래를 뒤집으면 → "말한 도형은 제 색을 지킨다" 가 빨개진다.
//   5. `nextElementId(next)` 를 `nextElementId(elements)` 로 바꾸면(자라는 배열을 보지
//      않으면) → "id 가 서로 다르다" 가 빨개진다.
//   6. `catalog_id` 를 심으면 → "가져온 요소는 catalog_id 를 갖지 않는다" 가 빨개진다.
//   7. **물지 않은 뮤테이션**: `geometry: { ...geometry }` 의 사본을 지워 조각들이 **같은
//      객체**를 공유하게 해도 어느 시험도 빨개지지 않는다 — 이 모듈은 기하를 제자리에서
//      고치지 않고, 고치는 통로(`patchNodeGeometry`)는 언제나 새 객체를 만들기 때문이다.
//      값이 아니라 **참조 공유**를 재는 가드를 세워(아래 "조각마다 제 상자 객체를 든다")
//      물게 했다.

import { describe, expect, it } from 'vitest';

import {
  appendElement,
  appendImportedElements,
  appendPathElement,
  nextElementId,
  SEED_COLOR,
  SEED_STROKE_WIDTH,
  type ImportedPathSource,
} from './canvasElementFactory';
import { parseCanvasConfig, type BoxGeometry, type CanvasElement, type PathElement } from './canvasConfig';
import type { PathCommand } from './shapes/pathTypes';
import type { CanvasNode } from './group/groupTypes';

/** 문서에서 서로 겹쳐 그려지던 조각 셋. 계단이 요소마다 더해지면 이 배치가 무너진다. */
const CLOSED: PathCommand[] = [
  { c: 'M', x: 0, y: 0 },
  { c: 'L', x: 4000, y: 0 },
  { c: 'L', x: 4000, y: 4000 },
  { c: 'Z' },
];
const OPEN: PathCommand[] = [
  { c: 'M', x: 1000, y: 1000 },
  { c: 'L', x: 9000, y: 9000 },
];

const BOX: BoxGeometry = { x: 50, y: 86, w: 400, h: 228 };
/** 첫 상자와 **네 수가 모두 다르다** — 한 수만 다르면 축을 뒤바꾼 뮤테이션이 통과한다. */
const BOX_B: BoxGeometry = { x: 71, y: 40, w: 120, h: 300 };
const BOX_C: BoxGeometry = { x: 12, y: 205, w: 61, h: 33 };

/**
 * 만들어진 것을 경로로 좁힌다. **이 파일의 원천은 전부 경로다** — 입구가 원시형도 받게
 * 넓어진 뒤로 `created` 는 `CanvasElement[]` 이므로, 좁히는 이 한 줄이 곧 "경로 원천은
 * 여전히 경로 요소가 된다" 는 단언이다(종전에는 반환 타입이 그것을 말했다).
 */
function asPaths(created: readonly CanvasElement[]): PathElement[] {
  return created.map((el) => {
    if (el.kind !== 'path') throw new Error(`경로가 아니다: ${el.kind}`);
    return el;
  });
}

function silent(commands: PathCommand[], box: BoxGeometry = BOX): ImportedPathSource {
  return { kind: 'path', commands, box, style: {}, hasOwnStyle: false };
}

function spoken(
  commands: PathCommand[],
  style: ImportedPathSource['style'],
  box: BoxGeometry = BOX,
): ImportedPathSource {
  return { kind: 'path', commands, box, style, hasOwnStyle: true };
}

describe('가져온 요소는 배열 끝에 문서 순서대로 붙는다 (AC-09)', () => {
  it('끝에 붙는 것이 곧 맨 위다 — 배열 순서가 유일한 z-order 다', () => {
    const before = appendElement([], 'rect').next;
    const { next, created } = appendImportedElements(before, [silent(CLOSED), silent(OPEN)]);
    expect(next).toHaveLength(3);
    expect(next.slice(1)).toEqual(created);
  });

  it('id 가 서로 다르고 기존 id 와도 겹치지 않는다', () => {
    const before = appendElement(appendElement([], 'rect').next, 'line').next;
    const { next, created } = appendImportedElements(before, [silent(CLOSED), silent(OPEN)]);
    const ids = next.map((el) => el.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(created.map((el) => el.id)).toEqual(['el-3', 'el-4']);
    // 다음 id 발급도 이어서 성립한다 — 가져오기가 발급 규칙을 흐리지 않았다.
    expect(nextElementId(next)).toBe('el-5');
  });

  it('el-${n} 을 만드는 자리가 가져오기 층에 없다 (형상 판정 · 불변식 K9)', async () => {
    const fs = await import('node:fs');
    const path = await import('node:path');
    const dir = path.join(path.dirname(new URL(import.meta.url).pathname), 'svgimport');
    for (const name of fs.readdirSync(dir).filter((n) => n.endsWith('.ts') && !n.endsWith('.test.ts'))) {
      const text = fs.readFileSync(path.join(dir, name), 'utf8');
      expect(`${name}:${/`el-\$\{/.test(text)}`).toBe(`${name}:false`);
    }
  });
});

describe('계단은 무리 전체에 한 번만 (REQ-06 · 뮤테이션 1 · 1b)', () => {
  /** 이미 요소가 셋 있으므로 계단은 0 이 아니다. 0 이면 이 시험이 아무것도 재지 못한다. */
  function withThreeBefore(sources: readonly ImportedPathSource[]) {
    const before = [0, 1, 2].reduce<CanvasNode[]>((acc) => appendElement(acc, 'rect').next, []);
    return asPaths(appendImportedElements(before, sources).created);
  }

  it('조각들이 흩어지지 않는다 — 상자는 저마다 달라도 **같은 계단**이 더해진다', () => {
    const sources = [silent(CLOSED, BOX), silent(OPEN, BOX_B), silent(CLOSED, BOX_C)];
    const created = withThreeBefore(sources);
    expect(created).toHaveLength(3);
    const deltas = created.map((el, i) => ({
      dx: el.geometry.x - sources[i]!.box.x,
      dy: el.geometry.y - sources[i]!.box.y,
    }));
    // 델타가 **전부 같다** = 계단이 한 번만 더해졌다. 상자 값이 같은지는 묻지 않는다 —
    // 옛 시험이 그것을 물었고, 그래서 상자가 달라지는 변경에 통째로 무너졌다.
    for (const d of deltas) expect(d).toEqual(deltas[0]);
    // 계단이 실제로 더해졌다 — 0 이면 "한 번만 더한다" 와 "안 더한다" 를 구분할 수 없다.
    expect(deltas[0]!.dx).toBeGreaterThan(0);
    expect(deltas[0]!.dx).toBe(deltas[0]!.dy);
  });

  it('상자는 도형마다 다르다 — 첫 도형의 상자가 나머지를 덮지 않는다 (뮤테이션 1b)', () => {
    const created = withThreeBefore([silent(CLOSED, BOX), silent(OPEN, BOX_B), silent(CLOSED, BOX_C)]);
    // 세 상자가 서로 다르다. **집합이 아니라 짝**으로 견준다 — 집합만 보면 순서를 뒤섞은
    // 결함(둘째 도형이 셋째 상자를 받는)이 통과한다.
    const off = created[0]!.geometry.x - BOX.x;
    expect(created.map((el) => el.geometry)).toEqual(
      [BOX, BOX_B, BOX_C].map((b) => ({ x: b.x + off, y: b.y + off, w: b.w, h: b.h })),
    );
  });

  it('상자 크기는 계단에 흔들리지 않는다', () => {
    const before = appendElement([], 'rect').next;
    const created = asPaths(appendImportedElements(before, [silent(CLOSED, BOX_B)]).created);
    expect(created[0]!.geometry.w).toBe(BOX_B.w);
    expect(created[0]!.geometry.h).toBe(BOX_B.h);
  });

  it('조각마다 제 상자 객체를 든다 — 하나를 고쳐도 다른 것이 따라 바뀌지 않는다', () => {
    // 같은 상자 **객체**를 둘에 준다. 그래도 만들어진 둘은 서로 다른 객체여야 한다 —
    // 한 도형이 상한 때문에 나뉜 조각들이 정확히 이 형상이다.
    const created = asPaths(appendImportedElements([], [silent(CLOSED, BOX), silent(OPEN, BOX)]).created);
    expect(created[0]!.geometry).toEqual(created[1]!.geometry);
    expect(created[0]!.geometry).not.toBe(created[1]!.geometry);
  });
});

describe('스타일 (REQ-06 · 뮤테이션 3·4)', () => {
  it('말하지 않은 도형은 008 의 씨앗을 입는다 — 두 번째 씨앗 규칙을 만들지 않는다', () => {
    const created = asPaths(appendImportedElements([], [silent(CLOSED), silent(OPEN)]).created);
    // 닫힘 → 채움, 열림 → 선. `pathSeedStyle` 이 정확히 그 둘을 가른다.
    expect(created[0]!.style).toEqual({ fill: SEED_COLOR });
    expect(created[1]!.style).toEqual({ stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH });
  });

  it('말한 도형은 제 색을 지킨다 — 씨앗이 덮지 않는다', () => {
    const style = { fill: 'rgba(192, 57, 43, 0.6)', stroke: 'rgba(20, 90, 50, 0.9)', strokeWidth: 3.8 };
    const created = asPaths(appendImportedElements([], [spoken(CLOSED, style)]).created);
    expect(created[0]!.style).toEqual(style);
  });

  it('칠을 말하지 않았어도 불투명도는 살아남는다', () => {
    const created = asPaths(
      appendImportedElements([], [
        { kind: 'path', commands: CLOSED, box: BOX, style: { opacity: 0.4 }, hasOwnStyle: false },
      ]).created,
    );
    expect(created[0]!.style).toEqual({ fill: SEED_COLOR, opacity: 0.4 });
  });
});

describe('명령을 사본으로 싣는다 (뮤테이션 2)', () => {
  it('만든 요소의 명령을 고쳐도 입력 목록이 바뀌지 않는다', () => {
    const source = silent(CLOSED);
    const created = asPaths(appendImportedElements([], [source]).created);
    const first = created[0]!.path[0]!;
    if (first.c === 'M') first.x = 9999;
    expect(source.commands[0]).toEqual({ c: 'M', x: 0, y: 0 });
  });

  it('같은 도형을 두 번 실어도 두 요소가 목록을 공유하지 않는다', () => {
    const source = silent(CLOSED);
    const created = asPaths(appendImportedElements([], [source, source]).created);
    expect(created[0]!.path).not.toBe(created[1]!.path);
    expect(created[0]!.path[0]).not.toBe(created[1]!.path[0]);
  });
});

describe('카탈로그 경로와 구별되지 않는다 (REQ-08 · AC-E12 · 뮤테이션 6)', () => {
  it('가져온 요소는 catalog_id 를 갖지 않는다 — 한 필드가 두 뜻을 갖지 않는다', () => {
    const created = asPaths(appendImportedElements([], [silent(CLOSED)]).created);
    expect(created[0]!.catalog_id).toBeUndefined();
    expect('catalog_id' in created[0]!).toBe(false);
  });

  it('필드 이름 집합이 카탈로그 경로의 것과 같다(catalog_id 를 뺀 나머지)', () => {
    const catalog = appendPathElement([], 'donut', CLOSED).created;
    const imported = asPaths(appendImportedElements([], [silent(CLOSED)]).created)[0]!;
    expect(Object.keys(imported).sort()).toEqual(
      Object.keys(catalog)
        .filter((k) => k !== 'catalog_id')
        .sort(),
    );
  });

  it('config 파서 왕복을 그대로 견딘다 — 명령도 스타일도 상자도 같다', () => {
    const style = { fill: 'rgba(192, 57, 43, 0.6)', stroke: 'rgba(20, 90, 50, 0.9)', strokeWidth: 3.8 };
    const { next } = appendImportedElements([], [spoken(CLOSED, style), silent(OPEN)]);
    const parsed = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: { width: 500, height: 400 }, elements: next })) as unknown,
    );
    expect(parsed.elements).toEqual(next);
  });
});
