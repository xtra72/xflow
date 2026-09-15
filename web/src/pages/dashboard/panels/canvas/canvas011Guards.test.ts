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
const HIT_TEST = 'canvasHitTest.ts';
/** 꺾임을 끼워 넣고 빼는 산술 — M10 이 세운 모듈. */
const EDIT = 'connector/connectorEdit.ts';
/** 자리 산술이 사는 자리 — 점–선분의 가장 가까운 자리가 여기 하나다. */
const GEOMETRY = 'canvasGeometry.ts';
/** 노드를 **만드는** 유일한 모듈 — M8 이 연결선 입구를 여기 세웠다. */
const FACTORY = 'canvasElementFactory.ts';
/** 목록 편집기 — M12 가 연결선 행을 여기 세웠다(끊김을 **묻는** 넷째 자리). */
const LIST = 'CanvasElementsEditor.tsx';

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
    expect(names).toContain('connector/CanvasConnectorTools.tsx');
    expect(names).toContain(RESOLVE);
    expect(names).toContain(EDIT);
    expect(names).toContain(FACTORY);
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

// --- AC-72: 점 몸짓도 **그 판정 하나**를 읽는다 -------------------------------

describe('꺾임 편집이 두 번째 판정을 짓지 않았다 (AC-72 · M10)', () => {
  // AC-28 의 그 절과 **한 규율**이다. 되풀이하는 것에 뜻이 있다: M10 은 더블클릭을 받는
  // 문을 하나 더 열었고(연결선 손잡이 — 진짜 단추라 제 누름을 먹는다), 문이 둘이 되는 순간
  // "판정은 하나인데 장부가 둘" 이라는 새 형상이 열린다. 그 형상에서는 두 문이 서로의 누름을
  // 보지 못해 "선 위에서는 되는데 점 위에서는 가끔 안 된다" 가 된다 — 화면에서만 드러난다.

  it('묻고 적는 일을 **한 함수**가 한다 — 정의 하나 · 호출 둘', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /const pressedTwice = /g)).toBe(1);
    // 화살표 함수라 정의는 위 줄이 세고, 이 줄은 **부르는 자리**만 센다 — 두 문에서 하나씩.
    expect(countOf(text, /\bpressedTwice\(/g)).toBe(2);
  });

  it('직전 누름을 **적는 자리**가 하나다 — 장부가 둘이 되지 않았다', () => {
    // `= null`(사슬 끊기)은 여럿이어도 좋다. 둘이 되면 안 되는 것은 **기록**이다.
    const text = source(OVERLAY);
    expect(countOf(text, /lastPressRef\.current = \{/g)).toBe(1);
  });

  it('점 갈래도 그 판정의 **결과를 읽을 뿐**이다', () => {
    // 앵커 갈래의 그 줄과 나란한 형상이다. 사라지면 점 몸짓이 제 문턱을 갖기 시작한 것이다.
    expect(source(OVERLAY)).toContain('if (second && TOOL_POINT_GESTURE[tool]');
  });

  it('손잡이 갈래도 표를 보고 갈린다 — 조건을 즉석에서 적지 않았다', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /TOOL_POINT_GESTURE\[tool\]/g)).toBe(2);
  });

  it('점 몸짓을 정하는 자리가 `connectorEdit.ts` 하나다', () => {
    // 오버레이가 제 손으로 "어느 자리인가" 를 셈하기 시작하면 그 산술이 두 벌이 된다.
    for (const { name, text } of productSources()) {
      if (name === EDIT) continue;
      expect(text.includes('connectorPointGestureAt('), name).toBe(name === OVERLAY);
    }
    expect(countOf(source(EDIT), /export function connectorPointGestureAt\b/g)).toBe(1);
  });

  it('그 모듈은 시각도 연타도 모른다 — 몸짓은 오버레이의 것이다', () => {
    const text = source(EDIT);
    for (const banned of ['timeStamp', 'isSecondPress', 'DOUBLE_PRESS', 'PointerEvent']) {
      expect(text.includes(banned), banned).toBe(false);
    }
  });
});

// --- AC-55 의 짝: 자리를 내는 산술도 하나다 ------------------------------------

describe('점–선분의 가장 가까운 자리가 **한 함수**다 (위험 R1 · M10)', () => {
  it('그 산술이 `canvasGeometry.ts` 에 한 번 정의된다', () => {
    expect(countOf(source(GEOMETRY), /export function closestPointOnSegment\b/g)).toBe(1);
  });

  it('부르는 제품 파일이 **이름으로 적은 둘**뿐이다', () => {
    // 잡는 쪽(거리 판정)과 찍는 쪽(새 점의 자리). 셋째가 늘면 그 자리가 제 손으로 선 위를
    // 셈하기 시작한 것이며, 그때 "잡히는 자리와 점이 놓이는 자리가 다르다" 가 열린다.
    const allowed = new Set([HIT_TEST, EDIT]);
    for (const { name, text } of productSources()) {
      if (name === GEOMETRY) continue;
      expect(/\bclosestPointOnSegment\(/.test(text), name).toBe(allowed.has(name));
    }
  });

  it('히트 층의 거리 판정이 그 자리를 **지나서** 잰다 — 산술을 두 벌 들지 않는다', () => {
    const text = source(HIT_TEST);
    expect(countOf(text, /function distanceToSegment\b/g)).toBe(1);
    // 투영 계수(내적을 길이 제곱으로 나누는 그 줄)가 이 파일에 남아 있으면 옛 산술이
    // 그대로 살아 있는 것이다.
    expect(/lengthSq/.test(text)).toBe(false);
  });

  it('끼워 넣는 쪽은 **평탄화를 재사용**한다 — 곡선 거리를 새로 적지 않았다 (AC-55)', () => {
    const text = source(EDIT);
    expect(/from '[^']*pathFlatten'/.test(text)).toBe(true);
    expect(text.includes('flattenPath(')).toBe(true);
    // 모양은 그리는 쪽과 같은 함수에서 온다.
    expect(text.includes('connectorPath(')).toBe(true);
    for (const banned of ['curveSegments', 'flattenCubic', 'TWO_THIRDS', 'bezierCurveTo']) {
      expect(text.includes(banned), banned).toBe(false);
    }
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
    for (const name of [
      'CanvasTool',
      'TOOL_ANCHOR_GESTURE',
      'TOOL_SHOWS_ANCHORS',
      'TOOL_CONNECTOR_ROUTE',
      'toggleTool',
    ]) {
      expect(dock.includes(name), name).toBe(false);
    }
  });
});

// --- AC-61: 연결선 도구도 두 표면에 함께 선다 (M8) ---------------------------

describe('연결선 도구를 짓는 자리가 **하나**다 (AC-61 · 불변식 I24)', () => {
  // 앵커 도구의 그 절과 **한 글자도 다르지 않은 규율**이다. 되풀이하는 것에 뜻이 있다:
  // 006 이 배달한 결함은 "컨트롤이 도크에만 있었다" 하나였고, 그 결함은 새 도구가 설
  // 때마다 새로 심을 수 있다. 가드가 도구마다 서지 않으면 다음 도구가 조용히 도크에만
  // 선다.
  it('컴포넌트를 쓰는 제품 파일이 오버레이 하나뿐이다', () => {
    for (const { name, text } of productSources()) {
      if (name === 'connector/CanvasConnectorTools.tsx') continue;
      expect(text.includes('<CanvasConnectorTools'), name).toBe(name === OVERLAY);
    }
  });

  it('오버레이가 그것을 **한 번 짓고 두 자리에서 그린다**', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /<CanvasConnectorTools\b/g)).toBe(1);
    // 앵커 묶음과 같은 셈이다: 선언 하나 + 떠 있는 줄 하나 + 도크의
    // `connectorTools={connectorTools}` 둘.
    expect(countOf(text, /\bconnectorTools\b/g)).toBe(4);
    expect(text).toContain('connectorTools={connectorTools}');
  });

  it('도크는 **자리와 이름만** 더한다 — 컨트롤을 스스로 짓지 않는다', () => {
    const dock = source('CanvasEditDock.tsx');
    expect(dock).toContain('connectorTools: React.ReactNode');
    expect(dock.includes('<CanvasConnectorTools')).toBe(false);
  });

  it('단추 넷을 **손으로 적지 않았다** — 표에서 파생한다', () => {
    // 목록을 손으로 적으면 다섯째 도구가 단추 **없이** 켜지기만 하고, 화면은
    // "도구는 켜졌는데 아무것도 그어지지 않는다" 만 말한다.
    const text = source('connector/CanvasConnectorTools.tsx');
    expect(text).toContain('CANVAS_TOOLS.map');
    expect(text).toContain('TOOL_CONNECTOR_ROUTE[id]');
    // 네 이름이 나오는 자리는 아이콘 표 **한 곳**뿐이다(`Record<ConnectorRoute, …>`).
    for (const route of ['straight', 'elbow', 'curve', 'free']) {
      expect(countOf(text, new RegExp(`\\b${route}\\b`, 'g')), route).toBe(1);
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

  it('`ConnectorEnd` 를 **타입으로** 들이는 제품 파일이 넷뿐이다', () => {
    // 자료형을 적는 자리 · 읽어 들이는 자리(파서) · 푸는 자리, 그리고 M8 이 더한 **만드는**
    // 자리(`appendConnector`). 구멍을 조용히 지나가지 않고 늘어난 자리를 **세어서** 적는
    // 것이 011 이 가드에 대해 정한 규율이다(§009 의 좌표 변환 가드를 넓힌다).
    //
    // 넷째가 **읽는** 자리가 아니라는 사실은 아래 두 단언이 붙든다: 속을 가르는 `'el' in`
    // 은 여전히 `resolveConnector` 하나뿐이고(바로 위 시험), 그 파일은 끝을 **인자로 받아
    // 그대로 싣기만** 한다. 다섯째가 생기면 그 자리가 정말 필요한지부터 따져야 한다.
    const allowed = [CONNECTOR_TYPES, 'canvasConfig.ts', RESOLVE, FACTORY].sort();
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
    // AC-45 ①의 **셋 중 첫째**다. 손잡이(M9)는 아직 서지 않았으므로 아래 `todo` 가 그
    // 자리를 비워 둔 채 남는다 — 셋을 한꺼번에 초록으로 만들지 않는다.
    //
    // 재는 것은 둘이다: 렌더 층이 그 함수를 **부르고**, 앵커를 **제 손으로 찾지 않는다**.
    // 둘째가 없으면 "부르기도 하고 따로 풀기도 한다" 가 지나간다.
    const text = source('drawElement.ts');
    expect(/\bresolveConnector\(/.test(text)).toBe(true);
    expect(/from '[^']*resolveConnector'/.test(text)).toBe(true);
    expect(text.includes('anchorPoints')).toBe(false);
    expect(text.includes("'el' in ")).toBe(false);
  });

  it('①-히트: 히트 층이 `resolveConnector` 를 지난다 (M7)', () => {
    // 셋 중 **둘째**다. 재는 잣대는 위 그리기와 글자 그대로 같다.
    const text = source(HIT_TEST);
    expect(/\bresolveConnector\(/.test(text)).toBe(true);
    expect(/from '[^']*resolveConnector'/.test(text)).toBe(true);
    expect(text.includes('anchorPoints')).toBe(false);
    expect(text.includes("'el' in ")).toBe(false);
  });

  it('①-손잡이: 오버레이가 `resolveConnector` 를 지난다 (M9)', () => {
    // 셋 중 **셋째**이자 마지막이다 — 이로써 AC-45 ①이 온전해진다. 재는 잣대는 위 둘과
    // 글자 그대로 같을 수 없다: 오버레이는 앵커를 **그리는** 층이기도 해서 `anchorPoints`
    // 를 둘 곳에서 부르고(위 §보이는 점과 집히는 점), 그 둘은 이 가드가 겨누는 대상이
    // 아니다. 그래서 여기서 재는 것은 "그 함수를 부르고, 끝점 자료형을 **제 손으로 가르지
    // 않는다**" 둘이다.
    const text = source(OVERLAY);
    expect(/\bresolveConnector\(/.test(text)).toBe(true);
    expect(/from '[^']*resolveConnector'/.test(text)).toBe(true);
    expect(text.includes("'el' in ")).toBe(false);
  });

  it('①의 셋에 **넷째가 이름으로** 더해졌다 — 목록 행 (M12)', () => {
    // 위 셋을 따로 재면 "셋이 전부인가" 를 아무도 묻지 않는다. 부르는 제품 파일을
    // **세어서** 적는 것이 011 이 가드에 대해 정한 그 규율이다(§009 의 좌표 변환 가드를
    // 넓힌다). 그래서 M12 가 넷째를 들이는 날 이 줄이 빨개졌고, 지우는 대신 **이름과
    // 까닭을 적어** 넓힌다.
    //
    // **넷째 — `CanvasElementsEditor.tsx`(M12 · AC-78).** 목록의 연결선 행이 "이 선이
    // 끊겼는가" 를 말해야 하고(REQ-08), 그 답은 이 함수의 **부재 여부**다. 판정을 제 손으로
    // 적는 길(참조 id 를 배열에서 찾아보는 길)을 고르면 끊김의 정의가 둘이 되고, 그중
    // 하나가 네 갈래(지워진 요소 · 부품 복합 키 · 연결선 참조 · 없는 앵커 이름) 가운데
    // 하나를 잃는 날 목록은 멀쩡하다고 말하는데 화면에는 아무것도 그려지지 않는다.
    //
    // 넷째가 **읽는 것은 답뿐**이라는 사실은 아래 두 시험이 붙든다. 다섯째가 생기면 그
    // 자리가 정말 필요한지부터 따져야 한다.
    const callers = productSources()
      .filter(({ name, text }) => name !== RESOLVE && /\bresolveConnector\(/.test(text))
      .map(({ name }) => name)
      .sort();
    expect(callers).toEqual([OVERLAY, LIST, 'canvasHitTest.ts', 'drawElement.ts'].sort());
  });

  it('넷째는 그 함수의 **답만** 읽는다 — 좌표를 한 자리도 쓰지 않는다', () => {
    // 목록에는 스테이지가 없어 1:1 투영을 넘긴다. 그 투영으로 나온 **좌표**를 읽으면
    // 그것은 화면의 값이 아닌 수를 화면에 내놓는 일이 되고, 그 어긋남은 캔버스와 목록을
    // 나란히 놓고 봐야만 보인다. 그래서 부르는 문형 자체를 못박는다: 이 파일에서
    // `resolveConnector(` 는 **언제나** `=== undefined` 비교로 끝난다.
    const text = source(LIST);
    const calls = text.match(/resolveConnector\([\s\S]*?\)\s*===\s*undefined/g) ?? [];
    expect(calls).toHaveLength(countOf(text, /\bresolveConnector\(/g));
    expect(calls).toHaveLength(1);
  });

  it('넷째가 넘기는 투영은 **1:1** 이다 — 지어낸 배율이 없다', () => {
    // 스테이지를 캔버스와 **같게** 두는 것이 항등 투영이다. 다른 수를 적으면 그것은
    // 아무도 잰 적 없는 배율이고, 그런 수가 한 번 들어오면 다음 사람은 그것을 잰 값으로
    // 읽는다.
    const text = source(LIST);
    expect(text).toContain('stage: { ...cfg.canvas }, canvas: cfg.canvas');
  });
});

// --- AC-63: 연결선에 8핸들이 서지 않는다 (M9) -------------------------------

describe('연결선 손잡이가 8핸들의 이름 공간을 **쓰지 않는다** (AC-63)', () => {
  it('`CanvasHandleId` 가 한 글자도 넓어지지 않았다', () => {
    // 넓히면 `HANDLE_ARIA_KEYS` · `HANDLE_CURSOR` 두 `Record` 가 뜻 없는 항목을 하나씩
    // 갖는다 — 중간점은 개수가 저술마다 다르므로 그 닫힌 집합에 넣을 고정 이름이 없다.
    const text = source('canvasEditGeometry.ts');
    const union = /export type CanvasHandleId =([^;]*);/.exec(text);
    expect(union).not.toBeNull();
    expect(union?.[1]?.trim()).toBe('BoxHandleId | LineHandleId | TextHandleId');
  });

  it('두 표가 여전히 `Record<CanvasHandleId, …>` 다 — 갈래가 늘면 컴파일러가 운다', () => {
    const text = source(OVERLAY);
    expect(countOf(text, /Record<CanvasHandleId, string>/g)).toBe(2);
  });

  it('연결선 손잡이는 **제 이름 공간**을 쓴다', () => {
    const text = source(OVERLAY);
    expect(text).toContain('canvas-connector-handle-');
    // 8핸들의 testid 를 짓는 자리는 여전히 하나뿐이다 — 둘이 되면 `canvas-handle-nw` 가
    // 무엇을 가리키는지 이름만으로 답하지 못한다.
    expect(countOf(text, /canvas-handle-\$\{/g)).toBe(1);
  });

  it('`handlePositions` 는 연결선을 **받지 못한다** — 타입이 그것을 막는다', () => {
    // `OutlinedNode` 는 연결선을 제외한 합집합이다(M4). 서명이 그대로인 한 8핸들이
    // 연결선에 서는 상태는 검사가 아니라 **형상**으로 불가능하다.
    const text = source('canvasEditGeometry.ts');
    expect(/export function handlePositions\(\s*el: OutlinedNode,/.test(text)).toBe(true);
    expect(/export function handlesFor\(kind: OutlinedNodeKind\)/.test(text)).toBe(true);
  });
});

// --- AC-55: 곡선 거리 판정이 평탄화를 지난다 ----------------------------------

describe('곡선 거리 산술을 **새로 적지 않았다** (AC-55)', () => {
  /** 이름 붙은 함수 하나의 몸통 — 다음 최상위 `}` 까지다. */
  function bodyOf(text: string, name: string): string {
    const from = text.indexOf(`function ${name}(`);
    expect(from, name).toBeGreaterThanOrEqual(0);
    const to = text.indexOf('\n}', from);
    expect(to, name).toBeGreaterThan(from);
    return text.slice(from, to);
  }

  it('연결선 갈래가 `flattenPath` 를 지난다', () => {
    const text = source(HIT_TEST);
    expect(/from '[^']*pathFlatten'/.test(text)).toBe(true);
    const body = bodyOf(text, 'hitsConnector');
    expect(body.includes('flattenPath(')).toBe(true);
    // 모양은 그리는 쪽과 **같은 함수**에서 온다 — 곡선을 여기서 다시 짓지 않는다.
    expect(body.includes('connectorPath(')).toBe(true);
    expect(body.includes('curveSegments')).toBe(false);
  });

  it('연결선 갈래에 **상자 판정이 없다** (REQ-07-b)', () => {
    // 빠른 걸러내기로도 두지 않는다 — 있으면 다음 사람이 그것을 판정으로 키운다.
    const body = bodyOf(source(HIT_TEST), 'hitsConnector');
    for (const banned of ['hitsBox', 'hitsEllipse', 'outlineBox', 'projectBox']) {
      expect(body.includes(banned), banned).toBe(false);
    }
    // 열린 선에는 안쪽이 없다.
    expect(body.includes('isInsidePath')).toBe(false);
  });

  it('베지어 산술이 사는 자리가 **하나도 늘지 않았다**', () => {
    // 3차 이등분(`flattenCubic`)은 008 의 평탄화 한 파일에만 있다. 히트가 곡선 거리를
    // 제 손으로 재기 시작하면 그 이름이나 그 산술이 여기 아닌 어딘가에 한 벌 더 생긴다.
    const subdividing = productSources()
      .filter(({ text }) => /\bflattenCubic\b/.test(text))
      .map(({ name }) => name)
      .sort();
    expect(subdividing).toEqual(['shapes/pathFlatten.ts']);

    // 2차→3차 환산(`2/3`)이 사는 자리는 **둘이고 둘 다 011 이전부터 있었다**: 연결선의
    // 곡선(M6)과 SVG 들이기의 `Q` 명령. 둘은 시각이 다르다 — 앞은 그릴 때마다, 뒤는 한 번
    // 읽어 들일 때. 셋째가 생기는 날이 011 이 금지한 그 자리다(AC-55).
    const converting = productSources()
      .filter(({ text }) => /2 \/ 3|TWO_THIRDS/.test(text))
      .map(({ name }) => name)
      .sort();
    expect(converting).toEqual(['connector/connectorCurve.ts', 'svgimport/svgPathData.ts']);

    const hit = source(HIT_TEST);
    expect(/\bflattenCubic\b|TWO_THIRDS|2 \/ 3|bezierCurveTo/.test(hit)).toBe(false);
  });

  it('모양을 정하는 자리가 `connectorPath.ts` **하나**다', () => {
    // 그리는 쪽과 잡는 쪽이 각각 `route` 를 가르면 "그려진 곡선과 잡히는 곡선이 다르다" 가
    // 시작된다. `route === 'curve'` 를 적는 제품 파일은 그 모듈 하나여야 한다.
    const branching = productSources()
      .filter(({ text }) => /route === 'curve'/.test(text))
      .map(({ name }) => name);
    expect(branching).toEqual(['connector/connectorPath.ts']);
    expect(countOf(source('connector/connectorPath.ts'), /export function connectorPath\b/g)).toBe(1);
  });
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
    // M8 이 더한 다섯 — 묶음 이름 하나 + 도구 이름 넷.
    'dashboard.canvas.edit.dockConnector',
    'dashboard.canvas.edit.toolStraight',
    'dashboard.canvas.edit.toolElbow',
    'dashboard.canvas.edit.toolCurve',
    'dashboard.canvas.edit.toolFree',
    // M9 가 더한 셋 — 연결선 손잡이의 이름. 끝점 둘과, **몇 번째인가**를 치환자로 받는
    // 중간점 하나다. 8핸들의 `HANDLE_ARIA_KEYS` 에 섞지 않은 까닭은 그 표가
    // `Record<CanvasHandleId, …>` 라 닫힌 이름 집합을 요구하기 때문이다(AC-63).
    'dashboard.canvas.edit.connectorHandleFrom',
    'dashboard.canvas.edit.connectorHandleTo',
    'dashboard.canvas.edit.connectorHandleMid',
    // M12 가 더한 열여덟 — **목록의 연결선 행**이 말하는 것 전부다(AC-78 · REQ-07).
    // 행 하나가 더한 수치고 많지만, 그 행이 말해야 하는 것이 그만큼이다: 종류 이름 ·
    // 그리는 법 넷 · 두 끝의 이름과 그 내용 · 꺾임점 · 끊김 표시와 고치는 길 · 읽는
    // 자리라는 안내 · 접근성 이름 넷. 세어서 적는 것이 011 의 규율이다.
    'dashboard.canvas.elements.connectorLabel',
    'dashboard.canvas.elements.connectorRouteStraight',
    'dashboard.canvas.elements.connectorRouteElbow',
    'dashboard.canvas.elements.connectorRouteCurve',
    'dashboard.canvas.elements.connectorRouteFree',
    'dashboard.canvas.elements.connectorFromLabel',
    'dashboard.canvas.elements.connectorToLabel',
    'dashboard.canvas.elements.connectorPointsLabel',
    'dashboard.canvas.elements.connectorEndAttached',
    'dashboard.canvas.elements.connectorEndFree',
    'dashboard.canvas.elements.connectorPointsSummary',
    'dashboard.canvas.elements.connectorBroken',
    'dashboard.canvas.elements.connectorBrokenHint',
    'dashboard.canvas.elements.connectorReadOnlyHint',
    'dashboard.canvas.elements.connectorDetailsAria',
    'dashboard.canvas.elements.connectorMoveUpAria',
    'dashboard.canvas.elements.connectorMoveDownAria',
    'dashboard.canvas.elements.connectorDeleteAria',
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

  it('중간점 문구가 번호 치환자를 **양쪽 로케일에** 들고 있다', () => {
    // 한쪽만 재면 다른 쪽에서 치환자가 빠진 것이 조용히 지나가고, 그때 점이 셋인 선의
    // 손잡이 셋이 **구별 불가능한 이름**을 읽는다. 로케일 기본값이 ko 라 en 쪽이 특히
    // 빠지기 쉬운 자리다.
    for (const [name, tree] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      const text = lookup(tree, 'dashboard.canvas.edit.connectorHandleMid') as string;
      expect(text, name).toContain('{index}');
    }
  });

  it('거절 문구가 **무엇을 해야 하는지**까지 말한다 — 안 된다는 말만 남기지 않는다', () => {
    for (const tree of [ko, en]) {
      const text = lookup(tree, 'dashboard.canvas.edit.anchorRefusalNotBoxed') as string;
      expect(text.length).toBeGreaterThan(30);
    }
  });

  it('끊김 안내도 **고치는 길**까지 말한다 (M12 · AC-78)', () => {
    // 위 거절 문구와 같은 잣대다. "끊겼습니다" 만 남기면 사용자는 그것이 제 실수인지
    // 고장인지, 무엇을 해야 되돌아오는지를 화면 어디에서도 알 수 없다.
    for (const tree of [ko, en]) {
      const text = lookup(tree, 'dashboard.canvas.elements.connectorBrokenHint') as string;
      expect(text.length).toBeGreaterThan(60);
    }
  });

  it('행이 쓰는 치환자가 **양쪽 로케일에** 그대로 있다 (M12)', () => {
    // 로케일 기본값이 ko 라 en 쪽 누락이 특히 조용히 지나간다. 빠지면 "요소의 자리" 를
    // 말해야 하는 줄이 요소 이름 없이 뜬다.
    const TOKENS: readonly (readonly [string, readonly string[]])[] = [
      ['dashboard.canvas.elements.connectorEndAttached', ['{element}', '{anchor}']],
      ['dashboard.canvas.elements.connectorEndFree', ['{x}', '{y}']],
      ['dashboard.canvas.elements.connectorPointsSummary', ['{count}']],
      ['dashboard.canvas.elements.connectorDetailsAria', ['{index}']],
      ['dashboard.canvas.elements.connectorMoveUpAria', ['{index}']],
      ['dashboard.canvas.elements.connectorMoveDownAria', ['{index}']],
      ['dashboard.canvas.elements.connectorDeleteAria', ['{index}']],
    ];
    for (const [key, tokens] of TOKENS) {
      for (const [name, tree] of [
        ['ko', ko],
        ['en', en],
      ] as const) {
        const text = lookup(tree, key) as string;
        for (const token of tokens) expect(text, `${name}:${key}`).toContain(token);
      }
    }
  });
});
