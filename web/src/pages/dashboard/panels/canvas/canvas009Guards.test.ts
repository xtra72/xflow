// 009 가 지키기로 한 **형상**을 소스에서 잰다 (SPEC-CANVAS-009 M7).
//
// ## 왜 소스를 읽는가
//
// 이 SPEC 의 안전 논거 넷은 동작이 아니라 **구조**다 — "판정이 한 자리에 있다", "역투영
// 자리가 늘지 않았다", "기하 쓰기 모듈이 `parts` 를 모른다", "분리가 풀기와 같은 함수를
// 쓴다". 넷 다 어겨도 화면은 한동안 멀쩡하고, 어긋남은 한참 뒤 다른 결함으로만 드러난다.
// 그래서 행동 시험으로는 덮이지 않고, 소스 텍스트가 그것을 붙든다.
//
// **주석은 걷어내고 읽는다.** 산문에 적힌 금지어가 가드를 헛되이 울리면 다음 사람이
// 주석을 고치는 것으로 가드를 지나가게 되고, 그때 가드는 이미 죽은 것이다.
//
// @spec SPEC-CANVAS-009 AC-04 · AC-16 · AC-17 · AC-33 · AC-44 · AC-46

import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

const CANVAS_DIR = __dirname;

/** 주석을 걷은 제품 소스. 007 의 가드가 세운 그 함수와 같은 규칙이다. */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function source(...parts: string[]): string {
  return stripComments(fs.readFileSync(path.join(CANVAS_DIR, ...parts), 'utf8'));
}

/** 캔버스 디렉터리의 제품 파일 전량(하위 디렉터리 포함, 시험 제외). */
function productSources(): { name: string; text: string }[] {
  const out: { name: string; text: string }[] = [];
  const walk = (dir: string): void => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(full);
        continue;
      }
      if (!/\.tsx?$/.test(entry.name) || /\.test\.tsx?$/.test(entry.name)) continue;
      out.push({
        name: path.relative(CANVAS_DIR, full),
        text: stripComments(fs.readFileSync(full, 'utf8')),
      });
    }
  };
  walk(CANVAS_DIR);
  return out;
}

// --- AC-04: 판별과 분해가 한 자리에 있다 -----------------------------------

describe('복합 키의 분해는 `frameKey.ts` 바깥에 없다 (AC-04)', () => {
  it('그물이 성기지 않다 — 제품 파일이 실제로 여럿 잡힌다', () => {
    // 순회가 비면 아래 "없다" 가 무조건 초록이 된다. 켜져 있음을 먼저 단언한다.
    const names = productSources().map((s) => s.name);
    expect(names.length).toBeGreaterThan(20);
    expect(names).toContain('group/frameKey.ts');
    expect(names).toContain('CanvasEditOverlay.tsx');
  });

  it('구분자 리터럴로 키를 쪼개는 코드가 `frameKey.ts` 밖에 없다', () => {
    const splitters = [/\.split\(\s*['"]\//, /\.indexOf\(\s*['"]\//, /\.lastIndexOf\(\s*['"]\//];
    for (const { name, text } of productSources()) {
      if (name === 'group/frameKey.ts') continue;
      for (const re of splitters) {
        expect(`${name}:${re.test(text)}`, `${name} ${re}`).toBe(`${name}:false`);
      }
    }
  });

  it('`frameKey.ts` 는 구분자 상수를 **한 번만** 적는다', () => {
    const text = source('group', 'frameKey.ts');
    const literals = text.match(/['"]\/['"]/g) ?? [];
    expect(literals).toHaveLength(1);
    expect(text).toContain('const PART_SEPARATOR');
  });

  it('분해와 판별이 그 상수를 쓴다 — 값을 베끼지 않는다', () => {
    const text = source('group', 'frameKey.ts');
    for (const fn of ['export function parseFrameKey', 'export function isPartKey']) {
      expect(text, fn).toContain(fn);
    }
    const uses = text.match(/PART_SEPARATOR/g) ?? [];
    // 선언 1 + `frameKey` 1 + `parseFrameKey` 2 + `isPartKey` 1.
    expect(uses.length).toBeGreaterThanOrEqual(5);
  });
});

// --- AC-16: 역투영 자리가 늘지 않았다 --------------------------------------
//
// ## 011 AC-32 가 이 가드를 **넓혔다**
//
// 009 는 이름 넷(`toLocalGeometry` · `toAbsoluteGeometry` · `toLocalBox` · `toAbsoluteBox`)을
// 보았고 허용 호출자는 `groupOps.ts` 하나였다. 011 의 임의 앵커가 점 환산을 쓰는데 그것은
// 009 의 목록에 **없어서** 새 호출자가 조용히 지나갔다 — 가드가 있는데 잡지 못하는 것이
// 가장 나쁘다.
//
// 그래서 011 은 목록을 **합집합 일곱**으로 넓히고 허용 호출자를 이름으로 적는다. AC-32 의
// 문장은 넷만 이름 짓지만 그대로 바꿔 적으면 009 가 보던 `toLocalBox` · `toAbsoluteBox` 가
// **빠진다** — 넓히는 것이지 걷어내는 것이 아니므로 합집합이 옳다.
//
// 일곱인 까닭은 점 되돌림이 **한 쌍**이기 때문이다(`…Rounded` 는 저장되는 기하로,
// `…Exact` 는 파생되는 자리로 간다 — `groupCoords.ts` 의 그 산문). 쌍 가운데 하나만 적어
// 두면 나머지 하나로 들어온 호출자가 통과하고, 그것이 011 이 애초에 이 가드를 넓힌 이유와
// **똑같은 구멍**이다.
//
// **넷째 호출자가 생기면 다시 운다.** 그것이 이 가드가 하는 일의 전부다.

/** 그룹 로컬 환산의 이름 — 009 의 넷 ∪ 011 이 더한 셋(`toLocalPoint` + 되돌림 한 쌍). */
const COORD_CONVERTERS = [
  'toLocalGeometry',
  'toAbsoluteGeometry',
  'toLocalBox',
  'toAbsoluteBox',
  'toLocalPoint',
  'toAbsolutePointRounded',
  'toAbsolutePointExact',
] as const;

/** 그 이름들을 적어도 되는 제품 파일. 정의 자리 하나 + 호출자 둘이다. */
const COORD_CALLERS = ['group/groupCoords.ts', 'group/groupOps.ts', 'connector/anchors.ts'];

describe('좌표 변환을 부르는 자리가 이름으로 적힌 둘뿐이다 (AC-32 — 009 AC-16 을 넓힌 것)', () => {
  it('허용 호출자 밖에서는 일곱 이름이 한 번도 나오지 않는다', () => {
    for (const { name, text } of productSources()) {
      if (COORD_CALLERS.includes(name)) continue;
      for (const fn of COORD_CONVERTERS) {
        expect(`${name}:${text.includes(fn)}`, `${name} ${fn}`).toBe(`${name}:false`);
      }
    }
  });

  it('허용 호출자 셋이 실제로 그 자리에 있다', () => {
    // 목록이 오타로 헛돌면 위 단언이 무조건 초록이 된다 — 이름이 실재함을 먼저 잰다.
    const names = productSources().map((s) => s.name);
    for (const caller of COORD_CALLERS) {
      expect(names, caller).toContain(caller);
    }
  });

  it('011 이 더한 이름을 실제로 쓰는 자리가 `connector/anchors.ts` 다', () => {
    const text = source('connector', 'anchors.ts');
    // 앵커는 **파생 자리**이므로 정확 쪽을 쓴다. 정수화 쪽을 쓰면 꼭지점에서 반 단위 밀린다.
    expect(text).toContain('toAbsolutePointExact(');
    expect(text).not.toContain('toAbsolutePointRounded');
    // 반대 방향은 한 벌뿐이다 — 로컬 좌표는 저장되는 값이라 정수화를 유지한다.
    expect(text).toContain('toLocalPoint(');
  });

  it('되돌림 한 쌍의 정수화 쪽은 **저장되는 기하**로만 간다', () => {
    // `toAbsolutePointRounded` 를 부르는 자리는 `toAbsoluteGeometry` 하나이고, 그것은
    // `withGeometry` 를 지나 `el.geometry` 에만 쓰인다. 파생 소비자가 이쪽으로 새면
    // 004 의 정수 규율이 파생값에 잘못 걸린다.
    const text = source('group', 'groupCoords.ts');
    const calls = text.match(/toAbsolutePointRounded\(/g) ?? [];
    // 정의 1 + `toAbsoluteGeometry` 갈래 1.
    expect(calls).toHaveLength(2);
  });

  it('`groupOps.ts` 에서 그 둘을 넘기는 자리가 `withGeometry` 하나다', () => {
    const text = source('group', 'groupOps.ts');
    // `convert` 인자로 넘기는 호출만 센다 — 타입 자리(`typeof …`)와 import 는 뺀다.
    const calls = text.match(/withGeometry\(/g) ?? [];
    // 정의 1 + 묶기 1 + 풀기 1 + 읽기 1 + 기하 쓰기 1 + 부품 되쓰기 1 + 분리 1.
    expect(calls.length).toBeGreaterThanOrEqual(6);
    // 그 함수 **밖**에서 변환을 직접 부르는 자리가 없다.
    const direct = text.match(/\btoLocalGeometry\(|\btoAbsoluteGeometry\(/g) ?? [];
    expect(direct).toHaveLength(0);
  });

  it('`로컬 ÷ EXTENT × 상자변` 이 `groupOps.ts` 에 적히지 않는다 (불변식 G2)', () => {
    const text = source('group', 'groupOps.ts');
    expect(text).not.toContain('GROUP_LOCAL_EXTENT');
  });
});

// --- AC-17: 기하 쓰기 모듈이 `parts` 를 모른다 -----------------------------

describe('`canvasEditGeometry.ts` 가 `parts` 를 모른다 (AC-17 — 004 의 기존 가드 유지)', () => {
  it('`parts` 필드를 대입하는 자리가 없다', () => {
    const text = source('canvasEditGeometry.ts');
    expect(text).not.toMatch(/parts\s*:/);
    expect(text).not.toMatch(/\.parts\s*=/);
  });

  it('`groupOps` 를 들이지도 않는다 — 부품 쓰기는 저 모듈을 지나지 않는다', () => {
    expect(source('canvasEditGeometry.ts')).not.toContain('groupOps');
  });
});

// --- AC-33: 분리와 풀기가 같은 함수를 쓴다 ---------------------------------

describe('분리 경로에 별도 구현이 없다 (AC-33)', () => {
  it('`detachPart` 가 `ungroupNode` · `bakeStyle` · `withGeometry` 를 그대로 부른다', () => {
    const text = source('group', 'groupOps.ts');
    const body = text.slice(text.indexOf('export function detachPart'));
    const detach = body.slice(0, body.indexOf('\n}\n') + 2);
    for (const fn of ['ungroupNode(', 'bakeStyle(', 'withGeometry(', 'nextElementId(']) {
      expect(detach, fn).toContain(fn);
    }
  });

  it('분리가 제 나름의 스타일 굽기를 적지 않는다', () => {
    const text = source('group', 'groupOps.ts');
    const body = text.slice(text.indexOf('export function detachPart'));
    // 곱셈으로 투명도를 접는 산술은 `bakeStyle` 한 자리뿐이다.
    expect(body).not.toMatch(/opacity[^\n]*\*/);
  });

  it('`rulesLostByDetach` 가 `rulesLostByUngroup` 의 판정을 쓴다 (REQ-05-c)', () => {
    const text = source('group', 'groupOps.ts');
    const body = text.slice(text.indexOf('export function rulesLostByDetach'));
    expect(body.slice(0, 400)).toContain('rulesLostByUngroup(');
  });
});

// --- AC-44: 그룹 중첩이 여전히 불가능하다 ----------------------------------

describe('그룹 안에 그룹이 오지 않는다 (AC-44 — 004 A6)', () => {
  it('`parts` 의 원소 타입이 `CanvasElement[]` 다', () => {
    expect(source('group', 'groupTypes.ts')).toContain('parts: CanvasElement[]');
  });

  it('009 가 더한 함수들이 `parts` 에 노드를 넣지 않는다', () => {
    const text = source('group', 'groupOps.ts');
    expect(text).not.toMatch(/parts:\s*CanvasNode/);
  });
});

// --- AC-46: 뒤집힌 시험 옆에 근거가 남는다 ---------------------------------

describe('뒤집힌 단언에 009 를 가리키는 주석이 있다 (AC-46)', () => {
  // 0.3.0 에서 **둘로 줄었다.** 0.2.0 은 캔버스 단일 클릭까지 뒤집어 셋이었으나, 그
  // 뒤집힘이 그룹을 클릭으로 고를 길을 없앤다는 사용자 보고가 들어와 되돌렸다
  // (단일 클릭은 004 와 같이 그룹, 더블클릭이 그 안). 그래서 `CanvasEditOverlay.delete`
  // 는 더는 뒤집힌 시험이 아니다 — 그 파일의 주석이 되돌린 경위를 대신 든다.
  const inverted: ReadonlyArray<readonly [file: string, marker: string]> = [
    ['canvas004GroupRows.test.tsx', '뒤집힌 단언 ①'],
    ['canvas004GroupRows.test.tsx', '뒤집힌 단언 ②'],
  ];

  it.each(inverted)('%s 에 근거 주석이 있다 (%s)', (file, marker) => {
    const raw = fs.readFileSync(path.join(CANVAS_DIR, file), 'utf8');
    expect(raw).toContain(marker);
    expect(raw).toContain('SPEC-CANVAS-009');
  });

  it('되돌린 자리에도 경위가 남는다 — 0.2.0 이 뒤집었다가 0.3.0 이 되돌린 곳', () => {
    const raw = fs.readFileSync(path.join(CANVAS_DIR, 'CanvasEditOverlay.delete.test.tsx'), 'utf8');
    expect(raw).toContain('SPEC-CANVAS-009 0.3.0');
    expect(raw).toContain('단일 클릭은 그룹');
  });

  it('뒤집힌 시험이 **삭제되지 않았다** — 같은 자리에 새 단언이 섰다', () => {
    const raw = fs.readFileSync(path.join(CANVAS_DIR, 'canvas004GroupRows.test.tsx'), 'utf8');
    expect(raw).toContain('부품 행을 누르면 **그 부품**이 골라진다');
    expect(raw).toContain('부품 행을 펼치면 **캔버스 단위** 수치 칸이 선다');
  });
});

// --- i18n: 009 가 더한 키 -------------------------------------------------

describe('009 가 더한 문구가 두 로케일에 있고 짝이 맞는다', () => {
  const ADDED = ['dashboard.canvas.edit.groupDetach', 'dashboard.canvas.edit.groupDetachAsk'];

  const lookup = (tree: unknown, dotted: string): string | undefined => {
    let cur: unknown = tree;
    for (const seg of dotted.split('.')) {
      if (typeof cur !== 'object' || cur === null) return undefined;
      cur = (cur as Record<string, unknown>)[seg];
    }
    return typeof cur === 'string' ? cur : undefined;
  };

  it.each(ADDED)('%s 가 ko·en 양쪽에 있다', (key) => {
    for (const [name, tree] of [['ko', ko], ['en', en]] as const) {
      const value = lookup(tree, key);
      expect(value, `${name} ${key}`).toBeTruthy();
      // 원문 키가 새지 않는다.
      expect(value, `${name} ${key}`).not.toContain('dashboard.canvas');
    }
  });

  it('두 로케일이 서로 다른 문구다 — 한쪽을 베껴 넣지 않았다', () => {
    for (const key of ADDED) {
      expect(lookup(ko, key), key).not.toBe(lookup(en, key));
    }
  });

  it('`groupDetachAsk` 가 `{rows}` 를 **한 번씩만** 말한다 (`replaceAll` 등가 가드)', () => {
    for (const [name, tree] of [['ko', ko], ['en', en]] as const) {
      const template = lookup(tree, 'dashboard.canvas.edit.groupDetachAsk') ?? '';
      expect((template.match(/\{rows\}/g) ?? []).length, name).toBe(1);
    }
  });

  it('뒤집힌 문구가 더는 "수치 칸을 두지 않는다" 고 말하지 않는다', () => {
    // 004 의 `groupPartsHint` 는 그렇게 말하고 있었다. 009 가 칸을 열었으므로 그 문장은
    // 참이 아니다 — 화면이 지키지 못할 약속을 하지 않는다는 이 저장소의 규율이다.
    expect(lookup(ko, 'dashboard.canvas.elements.groupPartsHint')).not.toContain('두지 않습니다');
    expect(lookup(en, 'dashboard.canvas.elements.groupPartsHint')).not.toContain('no numeric');
  });
});
