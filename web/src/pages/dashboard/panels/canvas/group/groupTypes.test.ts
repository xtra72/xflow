// 그룹 자료형과 로컬 격자 (SPEC-CANVAS-004 M1).
//
// 재는 것은 셋이다:
//   ① `GROUP_LOCAL_EXTENT` 가 008 의 격자에서 **파생**되었는가(값을 베끼지 않았는가 — A14).
//   ② 종류 이름이 **셋**이고 앞의 둘이 넓어지지 않았는가(A22 · AC-E14 의 004 쪽 절반).
//   ③ `isGroup` 이 판별의 유일한 자리로서 실제로 좁히는가.
//
// @spec SPEC-CANVAS-004 REQ-01 · AC-01 · AC-E14

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import type { CanvasElement, CanvasElementKind, CanvasPrimitiveKind } from '../canvasConfig';
import { PATH_LOCAL_EXTENT } from '../shapes/pathTypes';
import {
  GROUP_LOCAL_EXTENT,
  GROUP_LOCAL_SIZE,
  isGroup,
  type CanvasNode,
  type CanvasNodeKind,
  type GroupElement,
} from './groupTypes';

/** 주석을 걷은 제품 소스 — 산문에 적힌 숫자가 가드를 헛되이 울리지 않게 한다. */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function source(name: string): string {
  return stripComments(fs.readFileSync(path.join(__dirname, name), 'utf8'));
}

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', style: {}, geometry: { x: 0, y: 0, w: 10, h: 10 } };
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    parts: [rect('body')],
    ...over,
  };
}

// --- ① 격자는 파생이다 -----------------------------------------------------

describe('GROUP_LOCAL_EXTENT — 008 의 격자에서 파생한다 (A14)', () => {
  it('값이 `PATH_LOCAL_EXTENT` 와 같다', () => {
    expect(GROUP_LOCAL_EXTENT).toBe(PATH_LOCAL_EXTENT);
  });

  it('소스에 격자 **값이 적혀 있지 않다** — 베낀 것이 아니라 유도한 것이다', () => {
    const text = source('groupTypes.ts');
    // 켜져 있음을 먼저 잰다: 선언 줄이 실제로 있어야 아래 단언이 무엇인가를 재는 것이다.
    expect(text).toContain('export const GROUP_LOCAL_EXTENT = PATH_LOCAL_EXTENT;');
    // 값을 베꼈다면 `10000` 이 이 파일에 있다. 값을 따로 적으면 언젠가 한쪽만 바뀐다.
    expect(text).not.toContain(String(PATH_LOCAL_EXTENT));
  });

  it('`GROUP_LOCAL_SIZE` 는 두 축 모두 그 격자다', () => {
    expect(GROUP_LOCAL_SIZE).toEqual({ width: GROUP_LOCAL_EXTENT, height: GROUP_LOCAL_EXTENT });
  });
});

// --- ② 종류 이름은 셋이다 --------------------------------------------------

/**
 * 일곱 전부를 키로 갖는 표. **`Record` 라서 총망라를 요구한다** — `CanvasNodeKind` 에
 * 여덟째가 들어오면 이 선언이 컴파일에서 운다(`canvasElementKind.test.tsx` 와 같은 규율).
 *
 * SPEC-CANVAS-011 M4 가 일곱째(`connector`)를 더했다. 이 표는 **삭제하지 않고 넓힌다** —
 * 004 가 겨눈 것은 "몇 개인가" 가 아니라 "표와 타입이 갈라지지 않는가" 이고, 그 성질은
 * 원소가 늘어도 그대로 살아 있다(009 가 004 의 두 단언에 대해 한 그대로다).
 */
const ALL_NODE_KINDS: Record<CanvasNodeKind, true> = {
  rect: true,
  ellipse: true,
  line: true,
  text: true,
  path: true,
  group: true,
  connector: true,
};

describe('종류 이름 셋 — 앞의 둘을 넓히지 않고 셋째를 더했다 (A22)', () => {
  it('최상위 노드 종류는 **일곱**이다 (SPEC-CANVAS-011 M4 가 하나를 더했다)', () => {
    expect(Object.keys(ALL_NODE_KINDS).sort()).toEqual([
      'connector',
      'ellipse',
      'group',
      'line',
      'path',
      'rect',
      'text',
    ]);
  });

  it('앞의 둘은 그대로다 — 요소 다섯, 원시형 넷', () => {
    // 앞의 두 이름을 이 파일에서도 한 번 더 총망라한다. 004 가 둘 가운데 하나를 넓히면
    // 아래 두 선언이 **여기서도** 컴파일에서 운다.
    const elementKinds: Record<CanvasElementKind, true> = {
      rect: true,
      ellipse: true,
      line: true,
      text: true,
      path: true,
    };
    const primitiveKinds: Record<CanvasPrimitiveKind, true> = {
      rect: true,
      ellipse: true,
      line: true,
      text: true,
    };
    expect(Object.keys(elementKinds)).toHaveLength(5);
    expect(Object.keys(primitiveKinds)).toHaveLength(4);
    // 차이는 정확히 `group` 과 `connector` 둘이다 — 둘 다 **요소가 아닌** 최상위 노드다.
    // 앞의 두 이름이 한 글자도 넓어지지 않았다는 사실이 이 줄에 그대로 남아 있다.
    expect(
      Object.keys(ALL_NODE_KINDS).filter((k) => !(k in elementKinds)),
    ).toEqual(['group', 'connector']);
  });

  it('셋째 이름은 `canvasConfig.ts` 바깥에 산다 — 그 파일의 소스 텍스트를 읽는 가드가 있다', () => {
    // `canvas007Regression.test.tsx` 는 `canvasConfig.ts` 의 `CanvasElementKind` 선언을
    // 정규식으로 읽어 원소가 정확히 둘임을 단언한다. 셋째 이름을 그 파일에 두면 그
    // 가드가 우는 것은 아니지만, 두 이름이 한 파일에서 붙어 자라는 것을 막아 둔다.
    expect(source('../canvasConfig.ts')).not.toContain('CanvasNodeKind =');
    // SPEC-CANVAS-011 M4 — 유니온이 `OutlinedNodeKind` 와 연결선 종류의 합으로 적힌다.
    // 연결선 이름을 리터럴로 적지 않고 **그 자료형에서 파생**시키는 것이 AC-39 의 요구다
    // (그 문자열이 적히는 자리는 `connector/connectorTypes.ts` 하나뿐이다).
    expect(source('groupTypes.ts')).toContain(
      "export type CanvasNodeKind = OutlinedNodeKind | ConnectorElement['kind'];",
    );
    expect(source('groupTypes.ts')).toContain(
      "export type OutlinedNodeKind = CanvasElementKind | 'group';",
    );
  });
});

// --- ③ 판별은 한 자리다 ----------------------------------------------------

describe('isGroup — 판별의 유일한 자리', () => {
  it('그룹에 참, 요소 다섯 종에 거짓이다', () => {
    expect(isGroup(group())).toBe(true);
    const elements: CanvasElement[] = [
      { id: 'r', kind: 'rect', style: {}, geometry: { x: 0, y: 0, w: 1, h: 1 } },
      { id: 'e', kind: 'ellipse', style: {}, geometry: { x: 0, y: 0, w: 1, h: 1 } },
      { id: 'l', kind: 'line', style: {}, geometry: { x1: 0, y1: 0, x2: 1, y2: 1 } },
      { id: 't', kind: 'text', style: {}, geometry: { x: 0, y: 0 } },
      { id: 'p', kind: 'path', style: {}, geometry: { x: 0, y: 0, w: 1, h: 1 }, path: [] },
    ];
    expect(elements.map(isGroup)).toEqual([false, false, false, false, false]);
  });

  it('타입을 좁힌다 — 좁힌 뒤 `parts` 가 보인다', () => {
    const node: CanvasNode = group({ parts: [rect('a'), rect('b')] });
    if (!isGroup(node)) throw new Error('그룹이어야 한다');
    // 이 줄이 컴파일된다는 것이 좁히기의 증거다(`CanvasNode` 에는 `parts` 가 없다).
    expect(node.parts.map((p) => p.id)).toEqual(['a', 'b']);
  });
});
