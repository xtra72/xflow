// 011 이 지키기로 한 **형상**을 소스에서 잰다 (SPEC-CANVAS-011 M3'b · M4 · AC-28 · AC-39 · AC-61).
//
// ## 왜 소스를 읽는가
//
// AC-28 이 요구하는 것은 동작이 아니라 **구조**다 — "더블클릭 판정이 하나다". 판정이 둘이
// 되어도 화면은 한동안 멀쩡하다: 두 문턱이 같은 값으로 시작하기 때문이다. 어긋남은 한쪽만
// 고쳐진 뒤에, 그것도 "가끔 안 먹는다" 로만 드러난다. 그래서 행동 시험으로는 덮이지 않고
// 소스 텍스트가 그것을 붙든다.
//
// **주석은 걷어내고 읽는다.** 산문에 적힌 금지어가 가드를 헛되이 울리면 다음 사람이 주석을
// 고치는 것으로 가드를 지나가게 되고, 그때 가드는 이미 죽은 것이다(009 가 세운 그 규율).
//
// @spec SPEC-CANVAS-011 AC-28 · AC-39 · AC-61

import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

const CANVAS_DIR = __dirname;

/** 주석을 걷은 제품 소스. 007·009 의 가드가 세운 그 함수와 같은 규칙이다. */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
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

function source(name: string): string {
  return stripComments(fs.readFileSync(path.join(CANVAS_DIR, name), 'utf8'));
}

function countOf(text: string, re: RegExp): number {
  return (text.match(re) ?? []).length;
}

const OVERLAY = 'CanvasEditOverlay.tsx';
const RESOLVE = 'connector/resolveConnector.ts';

describe('그물이 성기지 않다', () => {
  it('제품 파일이 실제로 여럿 잡히고 오버레이가 그 안에 있다', () => {
    // 순회가 비면 아래 "없다" 가 전부 무조건 초록이 된다.
    const names = productSources().map((s) => s.name);
    expect(names.length).toBeGreaterThan(20);
    expect(names).toContain(OVERLAY);
    expect(names).toContain('connector/anchors.ts');
    expect(names).toContain('connector/canvasTools.ts');
    expect(names).toContain('connector/CanvasAnchorTools.tsx');
    expect(names).toContain('connector/connectorTypes.ts');
    expect(names).toContain(RESOLVE);
  });
});

// --- AC-28: 더블클릭 판정이 **하나**다 --------------------------------------

describe('009 의 `isSecondPress` 가 유일한 더블클릭 판정이다 (AC-28)', () => {
  it('판정 함수가 오버레이에 **한 번 정의되고 한 번 불린다**', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /function isSecondPress\b/g)).toBe(1);
    expect(countOf(text, /\bisSecondPress\(/g)).toBe(2); // 정의 한 번 + 호출 한 번
  });

  it('그 이름이 오버레이 **밖**의 제품 파일에 없다', () => {
    for (const { name, text } of productSources()) {
      if (name === OVERLAY) continue;
      expect(text.includes('isSecondPress'), name).toBe(false);
    }
  });

  it('문턱 둘도 오버레이 밖에 없다 — 베껴 적힌 두 번째 표가 없다', () => {
    for (const threshold of ['DOUBLE_PRESS_MS', 'DOUBLE_PRESS_SLOP_PX']) {
      for (const { name, text } of productSources()) {
        if (name === OVERLAY) continue;
        expect(text.includes(threshold), `${name}:${threshold}`).toBe(false);
      }
      expect(countOf(source(OVERLAY), new RegExp(`const ${threshold}\\b`, 'g')), threshold).toBe(1);
    }
  });

  it('시각을 읽는 자리가 **그 판정 하나**뿐이다', () => {
    // `event.timeStamp` 는 판정에 넘기는 값과 직전 누름에 적는 값 둘뿐이다. 셋째가 생기면
    // 어딘가에서 두 번째 시간 비교가 자라기 시작한 것이다.
    for (const { name, text } of productSources()) {
      const seen = countOf(text, /\btimeStamp\b/g);
      expect(seen, name).toBe(name === OVERLAY ? 2 : 0);
    }
  });

  it('브라우저의 연타 셈에 기대는 자리가 없다', () => {
    // `PointerEvent.detail` · `dblclick` · `onDoubleClick` 은 입력 장치에 따라 채워지지
    // 않으며, 채워지지 않으면 몸짓이 **조용히** 죽는다(`isSecondPress` 머리말).
    for (const { name, text } of productSources()) {
      for (const re of [/\.detail\b/, /\bdblclick\b/, /\bonDoubleClick\b/]) {
        expect(re.test(text), `${name} ${re}`).toBe(false);
      }
    }
  });

  it('앵커 갈래는 그 판정의 **결과를 읽을 뿐**이다', () => {
    // 갈림길이 `second` 라는 한 값에 걸려 있다는 사실을 텍스트로 못박는다. 이 줄이
    // 사라지면 앵커가 제 나름의 문턱을 갖기 시작한 것이다.
    expect(source(OVERLAY)).toContain('if (second && TOOL_ANCHOR_GESTURE[tool])');
  });

  it('집는 오차는 **파생**이지 새 눈금이 아니다', () => {
    // 숫자를 새로 적으면 그리는 크기와 집는 오차가 두 벌이 되고, 그 어긋남은 화면에서만
    // 보인다. 그래서 두 기존 수의 합으로만 적힌다.
    expect(source(OVERLAY)).toContain(
      'const ANCHOR_PICK_SLOP_PX = ANCHOR_DOT_PX / 2 + DOUBLE_PRESS_SLOP_PX',
    );
  });

  it('앵커 점을 **누를 수 있게** 만들지 않았다', () => {
    // 점이 단추가 되는 순간 "그 점 위의 더블클릭" 이라는 둘째 판정이 필요해진다 —
    // AC-28 이 막는 것이 바로 그 형상이다.
    const text = source(OVERLAY);
    const dotTag = text.match(/<(\w+)[^>]*canvas-anchor-dot-/);
    expect(dotTag?.[1]).toBe('div');
    expect(text).toContain("'pointer-events-none absolute h-2 w-2");
  });
});

// --- 앵커 자리를 셈하는 자리가 하나다 ---------------------------------------

describe('보이는 점과 집히는 점이 **같은 함수**에서 나온다 (위험 R1)', () => {
  it('`anchorPoints` 를 부르는 제품 파일이 **이름으로 적은 둘**뿐이다', () => {
    // 011 M5 가 이 목록을 하나 넓혔다. 구멍을 조용히 지나가지 않고 **세어서** 적는 것이
    // SPEC 이 좌표 변환 가드에 대해 정한 그 규율이다(§009 의 좌표 변환 가드를 넓힌다).
    //
    //   - 오버레이 — 그리는 점과 집는 점이 같은 지도에서 나와야 한다(위험 R1).
    //   - `connector/resolveConnector.ts` — 연결선의 끝점을 푸는 **한 함수**(AC-45).
    //
    // 셋째가 늘면 그 자리가 제 손으로 앵커를 셈하기 시작한 것이다. 여기에 이름을 적기
    // 전에 그 자리가 정말 필요한지부터 따져야 한다.
    const allowed = new Set([OVERLAY, RESOLVE]);
    for (const { name, text } of productSources()) {
      if (name === 'connector/anchors.ts') continue;
      const seen = countOf(text, /\banchorPoints\(/g);
      expect(seen > 0, name).toBe(allowed.has(name));
    }
  });

  it('오버레이는 그것을 **그리는 자리와 집는 자리** 둘에서만 부른다', () => {
    expect(countOf(source(OVERLAY), /\banchorPoints\(/g)).toBe(2);
  });

  it('오버레이가 임의 앵커 좌표를 제 손으로 환산하지 않는다 (AC-32)', () => {
    // 좌표 변환 호출자는 `group/groupOps.ts` 와 `connector/anchors.ts` 둘뿐이다.
    const text = source(OVERLAY);
    for (const fn of ['toAbsolutePoint', 'toLocalPoint', 'toLocalGeometry', 'toAbsoluteGeometry']) {
      expect(text.includes(fn), fn).toBe(false);
    }
  });
});

// --- AC-61: 도구가 두 표면에 함께 선다 --------------------------------------

describe('앵커 도구를 짓는 자리가 **하나**다 (AC-61 · 불변식 I24)', () => {
  it('컴포넌트를 쓰는 제품 파일이 오버레이 하나뿐이다', () => {
    for (const { name, text } of productSources()) {
      if (name === 'connector/CanvasAnchorTools.tsx') continue;
      expect(text.includes('<CanvasAnchorTools'), name).toBe(name === OVERLAY);
    }
  });

  it('오버레이가 그것을 **한 번 짓고 두 자리에서 그린다**', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /<CanvasAnchorTools\b/g)).toBe(1);
    // 지은 값이 **두 자리에서 읽힌다**: 떠 있는 줄과 도크. 이름이 넷 나오는 것은
    // 선언 하나 + 줄 하나 + 도크의 `anchorTools={anchorTools}` 둘이기 때문이다.
    expect(countOf(text, /\banchorTools\b/g)).toBe(4);
    expect(text).toContain('anchorTools={anchorTools}');
  });

  it('도크는 **자리와 이름만** 더한다 — 컨트롤을 스스로 짓지 않는다', () => {
    const dock = source('CanvasEditDock.tsx');
    expect(dock).toContain('anchorTools: React.ReactNode');
    expect(dock.includes('<CanvasAnchorTools')).toBe(false);
  });

  it('도구 상태가 도크로 내려가지 않는다 — 도크가 없는 표면에서도 도구가 산다', () => {
    const dock = source('CanvasEditDock.tsx');
    for (const name of ['CanvasTool', 'TOOL_ANCHOR_GESTURE', 'TOOL_SHOWS_ANCHORS', 'toggleTool']) {
      expect(dock.includes(name), name).toBe(false);
    }
  });
});

// --- AC-39: 연결선 판별이 **한 자리**다 -------------------------------------

const CONNECTOR_TYPES = 'connector/connectorTypes.ts';

describe('`kind === \'connector\'` 가 `isConnector` 바깥에 없다 (AC-39)', () => {
  it('그 문자열이 적힌 제품 파일이 `connectorTypes.ts` 하나뿐이다', () => {
    // 주석은 걷고 읽으므로(위 `stripComments`) 산문에 적힌 예시는 걸리지 않는다.
    for (const { name, text } of productSources()) {
      expect(text.includes("'connector'"), name).toBe(name === CONNECTOR_TYPES);
    }
  });

  it('그 파일에서도 **상수 선언 한 줄**에만 적힌다', () => {
    const text = source(CONNECTOR_TYPES);
    expect(countOf(text, /'connector'/g)).toBe(1);
    expect(text).toContain("export const CONNECTOR_KIND = 'connector';");
  });

  it('`kind` 를 그 문자열과 **직접** 견주는 자리가 어디에도 없다', () => {
    // 상수를 두었으므로 리터럴 비교는 한 자리도 남지 않는다. 이 단언이 빨개진다는 것은
    // 누군가 판별을 제 손으로 다시 적기 시작했다는 뜻이다.
    for (const { name, text } of productSources()) {
      expect(/===\s*'connector'/.test(text), name).toBe(false);
    }
  });

  it('비교하는 함수 둘이 그 파일 안에 있고, 둘 다 **같은 상수**를 본다', () => {
    const text = source(CONNECTOR_TYPES);
    // 노드를 받는 판별 하나와, 파싱 전 날것 `kind` 를 받는 문 하나다. 파서는 아직 노드가
    // 아닌 것을 들고 있어 앞의 것을 쓸 수 없으므로 갈래가 둘이지만, 보는 상수는 하나다.
    expect(countOf(text, /export function isConnector\b/g)).toBe(1);
    expect(countOf(text, /export function isConnectorKind\b/g)).toBe(1);
    expect(countOf(text, /=== CONNECTOR_KIND/g)).toBe(2);
  });

  it('연결선을 건너뛰는 자리는 전부 그 함수를 지난다', () => {
    // "연결선은 건너뛴다" 가 여러 자리에 붙는 것은 004 가 그룹에서 치른 대가 그대로다.
    // 그 자리들이 **한 함수**를 지나는 한 판별은 하나로 남는다.
    const users = productSources().filter(({ text }) => /\bisConnector\(/.test(text));
    expect(users.length).toBeGreaterThan(3);
    for (const { name, text } of users) {
      expect(text.includes("from './connector/connectorTypes'") ||
        text.includes("from '../connector/connectorTypes'") ||
        // 같은 디렉터리의 이웃(`connector/resolveConnector.ts`)이 쓰는 형태다.
        text.includes("from './connectorTypes'") ||
        name === CONNECTOR_TYPES, name).toBe(true);
    }
  });
});

// --- AC-45: 참조를 푸는 자리가 하나다 ---------------------------------------

describe('참조를 푸는 자리가 `resolveConnector.ts` 하나뿐이다 (AC-45 — 오늘 잴 수 있는 절반)', () => {
  // ## 이 절이 AC-45 의 **절반**인 까닭
  //
  // AC-45 는 둘을 요구한다.
  //
  //   ① 그리기 · 히트 · 손잡이가 **모두** `resolveConnector` 를 지난다.
  //   ② 그 밖에 참조를 **제 손으로 푸는 자리가 없다.**
  //
  // ①의 세 소비자는 M6(`drawElement.ts`) · M7(`canvasHitTest.ts`) · M9(오버레이)가
  // 세우므로 오늘은 잴 대상이 아예 없다 — 아래 마지막 `todo` 가 그 자리를 비워 둔다.
  // 나머지는 전부 ②이고, ②는 **오늘부터** 짐을 진다: 세 소비자가 설 때 참조를 제 손으로
  // 풀면 그 자리에서 빨개지고, `resolveConnector` 를 부르면 조용히 초록으로 지나간다.

  it('붙은 끝과 자유 끝을 **가르는 자리**가 하나뿐이다', () => {
    // `'el' in …` 이 그 판별이다. 파서는 아직 `ConnectorEnd` 가 아닌 날것 객체에서
    // `optionalString(e.el)` 로 읽으므로 이 문형에 걸리지 않는다.
    for (const { name, text } of productSources()) {
      expect(text.includes("'el' in "), name).toBe(name === RESOLVE);
    }
  });

  it('`ConnectorEnd` 를 **타입으로** 들이는 제품 파일이 셋뿐이다', () => {
    // 짓는 자리(자료형) · 읽어 들이는 자리(파서) · 푸는 자리. 넷째가 생기면 그 파일이
    // 끝점의 **속**을 들여다보기 시작한 것이다.
    const allowed = [CONNECTOR_TYPES, 'canvasConfig.ts', RESOLVE].sort();
    const seen = productSources()
      .filter(({ text }) => /\bConnectorEnd\b/.test(text))
      .map(({ name }) => name)
      .sort();
    expect(seen).toEqual(allowed);
  });

  it('그 함수 안에서 상자를 **다시 재지 않는다** (AC-33)', () => {
    // 끝이 둘이어도 `anchorPoints` 호출은 **한 자리**이고, 그 함수 안의 `outlineBox` 도
    // 하나다. 이 파일에 `outlineBox` 가 나타나면 두 번째 측정이 생긴 것이며, 그 갈라짐은
    // 크기를 바꾼 뒤에야 화면에서만 보인다.
    const text = source(RESOLVE);
    expect(countOf(text, /\banchorPoints\(/g)).toBe(1);
    expect(text.includes('outlineBox')).toBe(false);
  });

  it('복합 키를 **쪼개지 않는다** — 막는 것은 조회이지 검사가 아니다 (AC-44)', () => {
    // 부품 키가 풀리지 않는 까닭은 최상위 목록에 그 id 가 **없기** 때문이다. 여기서
    // 구분자를 적으면 009 AC-04(구분자 리터럴은 `frameKey.ts` 하나뿐)가 함께 빨개진다.
    const text = source(RESOLVE);
    expect(countOf(text, /['"]\/['"]/g)).toBe(0);
    for (const re of [/\.split\(/, /\.indexOf\(/, /\.lastIndexOf\(/]) {
      expect(re.test(text), String(re)).toBe(false);
    }
    expect(text.includes('frameKey')).toBe(false);
  });

  it('부르는 자리는 **그 모듈에서 들여온다** — 이름만 같은 둘째가 없다', () => {
    // M6 이 그리기를 세우면서 이 순회에 첫 이름이 들어왔다(`drawElement.ts`). 위 ②의
    // 가드들이 그 파일을 이미 붙들고 있다.
    for (const { name, text } of productSources()) {
      if (name === RESOLVE) continue;
      if (!/\bresolveConnector\(/.test(text)) continue;
      expect(/from '[^']*resolveConnector'/.test(text), name).toBe(true);
    }
    expect(countOf(source(RESOLVE), /export function resolveConnector\b/g)).toBe(1);
  });

  it('①-그리기: 렌더 층이 `resolveConnector` 를 지난다 (M6)', () => {
    // AC-45 ①의 **셋 중 첫째**다. 히트(M7)·손잡이(M9)는 아직 서지 않았으므로 아래 `todo`
    // 가 그 둘의 자리를 비워 둔 채 남는다 — 셋을 한꺼번에 초록으로 만들지 않는다.
    //
    // 재는 것은 둘이다: 렌더 층이 그 함수를 **부르고**, 앵커를 **제 손으로 찾지 않는다**.
    // 둘째가 없으면 "부르기도 하고 따로 풀기도 한다" 가 지나간다.
    const text = source('drawElement.ts');
    expect(/\bresolveConnector\(/.test(text)).toBe(true);
    expect(/from '[^']*resolveConnector'/.test(text)).toBe(true);
    expect(text.includes('anchorPoints')).toBe(false);
    expect(text.includes("'el' in ")).toBe(false);
  });

  it.todo('①-나머지: 히트(M7) · 손잡이(M9)도 `resolveConnector` 를 지난다 — 둘이 선 뒤에 잰다');
});

// --- M4: 앞의 두 이름을 넓히지 않았다 ---------------------------------------

describe('연결선은 `CanvasElement` 가 아니다 (AC-34)', () => {
  it('요소 파서의 `kind` 화이트리스트에 연결선이 없다', () => {
    // 출시된 가드 둘(`canvasElementKind.test.tsx` · `canvas007Regression.test.tsx`)이
    // 유니온의 원소를 세고, 이 줄은 **파서 쪽**에서 같은 사실을 못박는다. 셋이 함께
    // 빨개져야 "요소 종류가 늘었다" 는 변경이 조용히 지나가지 못한다.
    const text = source('canvasConfig.ts');
    const whitelist = /function parseElement\(raw: unknown\)[\s\S]*?switch \(kind\)/.exec(text);
    expect(whitelist).not.toBeNull();
    expect(whitelist?.[0].includes('CONNECTOR_KIND')).toBe(false);
  });

  it('파서가 갈라지는 자리는 `parseNode` **한 함수**뿐이다', () => {
    const text = source('canvasConfig.ts');
    expect(countOf(text, /\bisConnectorKind\(/g)).toBe(1);
    expect(countOf(text, /\bparseConnector\(/g)).toBe(2); // 정의 한 번 + 호출 한 번
  });
});

// --- i18n: 011 이 더한 키 ---------------------------------------------------

describe('011 이 더한 문구가 ko · en 양쪽에 있다', () => {
  const ADDED = [
    'dashboard.canvas.edit.dockAnchor',
    'dashboard.canvas.edit.toolSelect',
    'dashboard.canvas.edit.toolAnchor',
    'dashboard.canvas.edit.anchorRefusalNotBoxed',
  ];

  const lookup = (tree: unknown, key: string): unknown =>
    key
      .split('.')
      .reduce<unknown>(
        (node, seg) =>
          node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
        tree,
      );

  it.each(ADDED)('%s 가 두 로케일에서 비어 있지 않은 문자열이다', (key) => {
    for (const [name, tree] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      const value = lookup(tree, key);
      expect(typeof value, `${name}:${key}`).toBe('string');
      expect((value as string).trim().length, `${name}:${key}`).toBeGreaterThan(0);
    }
  });

  it.each(ADDED)('%s 의 두 로케일이 서로 다르다 — 한쪽을 베껴 넣지 않았다', (key) => {
    expect(lookup(ko, key), key).not.toBe(lookup(en, key));
  });

  it('거절 문구가 **무엇을 해야 하는지**까지 말한다 — 안 된다는 말만 남기지 않는다', () => {
    for (const tree of [ko, en]) {
      const text = lookup(tree, 'dashboard.canvas.edit.anchorRefusalNotBoxed') as string;
      expect(text.length).toBeGreaterThan(30);
    }
  });
});
