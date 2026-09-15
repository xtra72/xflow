// 도구 상태 기계 — 표 넷이 **함께** 자라는가 (SPEC-CANVAS-011 M3'b · M8).
//
// 이 모듈이 지키는 것은 값이 아니라 **형상**이다: 갈래가 하나 늘면 표 셋이 함께 자라야
// 하고, 하나라도 빠지면 그 도구는 조용히 기본값으로 떨어진다. 타입 층이 그것을 먼저
// 막지만(`Record<CanvasTool, …>`), 타입은 `as` 한 번으로 우회된다 — 그래서 여기서 키
// 집합을 실제로 센다.
//
// @spec SPEC-CANVAS-011 REQ-02 · REQ-02'

import { describe, expect, it } from 'vitest';

import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import {
  CANVAS_TOOLS,
  DEFAULT_CANVAS_TOOL,
  TOOL_ANCHOR_GESTURE,
  TOOL_CONNECTOR_ROUTE,
  TOOL_LABEL_KEYS,
  TOOL_SHOWS_ANCHORS,
  toggleTool,
} from './canvasTools';

/** 점 표기법 키를 번역 트리에서 조회한다. */
function lookup(tree: unknown, key: string): unknown {
  return key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
      tree,
    );
}

describe('도구 목록', () => {
  it('여섯이다 — 고르기 · 앵커 · 연결선 넷 (M8)', () => {
    // M3'b 는 이 수를 둘로 적고 "M8 이 넷을 더해 여섯이 된다" 고 예고했다. 더했고, 예고한
    // 대로 **이 수가 먼저 울었다** — 고친 사람은 그 길로 아래 네 표를 함께 보게 된다.
    expect([...CANVAS_TOOLS]).toEqual(['select', 'anchor', 'straight', 'elbow', 'curve', 'free']);
  });

  it('뒤의 넷이 `ConnectorRoute` 와 **같은 이름**이다 — 표가 그 둘을 이어 붙인다', () => {
    // 이름이 같은 것은 우연이 아니라 설계다(`canvasTools.ts` §`CANVAS_TOOLS`). 그 사실을
    // 여기서 재어 두면, 한쪽 이름만 고치는 변경이 `TOOL_CONNECTOR_ROUTE` 의 컴파일 오류
    // **와 함께** 이 단언에서도 운다.
    for (const tool of CANVAS_TOOLS) {
      const route = TOOL_CONNECTOR_ROUTE[tool];
      if (route === null) continue;
      expect(route, tool).toBe(tool);
    }
  });

  it('기본은 고르기다 — 011 이전과 같은 뜻으로 시작한다', () => {
    expect(DEFAULT_CANVAS_TOOL).toBe('select');
    expect(CANVAS_TOOLS).toContain(DEFAULT_CANVAS_TOOL);
  });

  it('이름에 중복이 없다 — 표의 키가 조용히 덮이지 않는다', () => {
    expect(new Set(CANVAS_TOOLS).size).toBe(CANVAS_TOOLS.length);
  });
});

describe('표 셋이 도구 목록과 **정확히** 같은 키를 갖는다', () => {
  const TABLES = [
    ['TOOL_LABEL_KEYS', TOOL_LABEL_KEYS],
    ['TOOL_SHOWS_ANCHORS', TOOL_SHOWS_ANCHORS],
    ['TOOL_ANCHOR_GESTURE', TOOL_ANCHOR_GESTURE],
    ['TOOL_CONNECTOR_ROUTE', TOOL_CONNECTOR_ROUTE],
  ] as const;

  it.each(TABLES)('%s 의 키가 `CANVAS_TOOLS` 와 같다', (name, table) => {
    expect(Object.keys(table).sort(), name).toEqual([...CANVAS_TOOLS].sort());
  });

  it('네 표를 손으로 센다 — 순회가 비면 위 단언들이 무조건 통과한다', () => {
    expect(TABLES).toHaveLength(4);
  });
});

describe('표 둘은 **다른 물음**이다 — M8 에서 실제로 갈라졌다', () => {
  // M3'b 는 두 표가 같은 값을 갖는 것이 **우연**이라 적고 M8 에서 갈라진다고 예고했다.
  // 갈라졌으므로 그 단언을 **지우지 않고 뒤집는다** — 009 가 004 의 두 단언에 대해 한
  // 그대로이며, 뒤집힌 자리에 근거를 남긴다(SPEC-CANVAS-011 §뒤집히는 시험).
  it('연결선 도구 넷에서 두 답이 **다르다** — 보이되, 더블클릭은 앵커가 아니다', () => {
    // 보여야 하는 까닭(REQ-02-b): 앵커에서 눌러 앵커에서 놓는 것이 그 도구의 몸짓이다.
    // 앵커가 아닌 까닭(REQ-05 · M10): 그 도구의 더블클릭은 선 위의 **중간점**이다.
    for (const tool of CANVAS_TOOLS) {
      if (TOOL_CONNECTOR_ROUTE[tool] === null) continue;
      expect(TOOL_SHOWS_ANCHORS[tool], tool).toBe(true);
      expect(TOOL_ANCHOR_GESTURE[tool], tool).toBe(false);
    }
  });

  it('실제로 갈리는 도구가 **하나 이상** 있다 — 순회가 비면 위가 무조건 통과한다', () => {
    const split = CANVAS_TOOLS.filter((t) => TOOL_SHOWS_ANCHORS[t] !== TOOL_ANCHOR_GESTURE[t]);
    expect([...split]).toEqual(['straight', 'elbow', 'curve', 'free']);
  });

  it('고르기는 앵커를 보이지도 더하지도 않는다 — 011 이전의 뜻 그대로다', () => {
    expect(TOOL_SHOWS_ANCHORS.select).toBe(false);
    expect(TOOL_ANCHOR_GESTURE.select).toBe(false);
    expect(TOOL_CONNECTOR_ROUTE.select).toBeNull();
  });

  it('앵커 도구는 앞의 둘이 참이고 선을 긋지는 않는다', () => {
    expect(TOOL_SHOWS_ANCHORS.anchor).toBe(true);
    expect(TOOL_ANCHOR_GESTURE.anchor).toBe(true);
    expect(TOOL_CONNECTOR_ROUTE.anchor).toBeNull();
  });

  it('연결선을 긋는 도구가 정확히 넷이다', () => {
    const drawing = CANVAS_TOOLS.filter((t) => TOOL_CONNECTOR_ROUTE[t] !== null);
    expect([...drawing]).toEqual(['straight', 'elbow', 'curve', 'free']);
  });
});

describe('토글', () => {
  it('꺼진 도구를 누르면 켜진다', () => {
    expect(toggleTool('select', 'anchor')).toBe('anchor');
  });

  it('켜진 도구를 다시 누르면 **고르기로 돌아온다** — 끄는 단추를 따로 두지 않는다', () => {
    expect(toggleTool('anchor', 'anchor')).toBe(DEFAULT_CANVAS_TOOL);
  });

  it('두 번 누르면 제자리다 — 어느 도구에서 시작하든', () => {
    for (const from of CANVAS_TOOLS) {
      for (const pressed of CANVAS_TOOLS) {
        expect(toggleTool(toggleTool(from, pressed), pressed), `${from}→${pressed}`).toBe(
          from === pressed ? from : DEFAULT_CANVAS_TOOL,
        );
      }
    }
  });
});

describe('도구 이름이 ko · en 양쪽에 있다', () => {
  it.each([...CANVAS_TOOLS])('%s 의 키가 두 로케일에서 비어 있지 않은 문자열이다', (tool) => {
    const key = TOOL_LABEL_KEYS[tool];
    for (const [name, tree] of [
      ['ko', ko],
      ['en', en],
    ] as const) {
      const value = lookup(tree, key);
      expect(typeof value, `${name}:${key}`).toBe('string');
      expect((value as string).length, `${name}:${key}`).toBeGreaterThan(0);
    }
  });

  it('두 로케일이 **다른 문구**다 — 한쪽을 베껴 넣은 키를 잡는다', () => {
    for (const tool of CANVAS_TOOLS) {
      const key = TOOL_LABEL_KEYS[tool];
      expect(lookup(ko, key), key).not.toBe(lookup(en, key));
    }
  });
});
