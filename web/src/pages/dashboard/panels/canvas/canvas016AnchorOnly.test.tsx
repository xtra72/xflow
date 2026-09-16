// 연결선은 앵커에서 앵커로만 산다 (SPEC-CANVAS-016 · REQ-01 · REQ-02 · REQ-04).
//
// **011 의 두 조항을 뒤집는다.** 그은 몸짓도 떼는 몸짓도 앵커 밖에서 끝나면 선이 남지
// 않는다. 뒤집힌 단언들은 011 의 제 파일에 근거와 함께 남겼고, 이 파일은 **새 사실**만 진다.
//
// @spec SPEC-CANVAS-016 REQ-01 · REQ-02 · REQ-04

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { parseCanvasConfig } from './canvasConfig';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import { resolveConnector } from './connector/resolveConnector';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';
import { removeNodesWithConnectors } from './canvasEditArrange';

const PROJ: CanvasProjection = { stage: { width: 200, height: 200 }, canvas: { width: 500, height: 400 } };

const R1 = { id: 'r1', kind: 'rect', geometry: { x: 50, y: 40, w: 100, h: 80 }, style: {} } as const;

function parsed(nodes: readonly Record<string, unknown>[]): CanvasNode[] {
  return parseCanvasConfig({ canvas: { width: 500, height: 400 }, elements: nodes.map((n) => ({ ...n })) })
    .elements;
}

describe('옛 저술의 자유 끝은 살아 있다 (REQ-03 · K2)', () => {
  const legacy = {
    id: 'c-legacy',
    kind: 'connector',
    from: { el: 'r1', a: 'e' },
    to: { x: 325, y: 380 },
    route: 'straight',
  } as const;

  it('파서가 버리지 않는다', () => {
    // 지우면 016 이전에 저장된 대시보드의 자유 끝 연결선이 **읽는 순간 사라지고**, 그것이
    // 011 REQ-08 이 "가장 나쁜 실패" 로 이름 적은 형상이다.
    const nodes = parsed([{ ...R1 }, { ...legacy }]);
    const line = nodes.find((n) => n.id === 'c-legacy');
    expect(line).toBeDefined();
    expect(isConnector(line!) && line.to).toEqual({ x: 325, y: 380 });
  });

  it('해석된다 — 그려지고 잡히는 선이라는 뜻이다', () => {
    const nodes = parsed([{ ...R1 }, { ...legacy }]);
    const line = nodes.find((n): n is ConnectorElement => n.id === 'c-legacy' && isConnector(n));
    expect(resolveConnector(line!, nodes, PROJ, {})).toBeDefined();
  });

  it('왕복이 그대로다 — 자유 끝이 붙은 끝으로 바뀌지 않는다', () => {
    const once = parsed([{ ...R1 }, { ...legacy }]);
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify({ elements: once }))).elements;
    expect(JSON.stringify(once)).toBe(JSON.stringify(twice));
  });
});

describe('지우는 입구가 하나다 (§결정 2 · K3)', () => {
  it('끝을 떼어 지우는 길이 목록·캔버스와 **같은 함수**를 지난다', () => {
    // 셋이 한 함수를 지나므로 "어디서 지웠느냐" 에 따라 결과가 달라질 수 없다.
    const nodes = parsed([
      { ...R1 },
      { id: 'c1', kind: 'connector', from: { el: 'r1', a: 'e' }, to: { x: 10, y: 10 }, route: 'straight' },
    ]);
    const next = removeNodesWithConnectors(nodes, new Set(['c1']));
    expect(next.find((n) => n.id === 'c1')).toBeUndefined();
    // 가리키던 도형은 남는다 — 선을 지우는 일이지 도형을 지우는 일이 아니다.
    expect(next.find((n) => n.id === 'r1')).toBeDefined();
  });

  it('오버레이가 그 함수를 실제로 부른다', () => {
    // 소스 판정이다. 제 손으로 `filter` 를 적으면 입구가 둘이 되고, 그때 한쪽만 고쳐진다.
    const src = readFileSync(
      resolve(process.cwd(), 'src/pages/dashboard/panels/canvas/CanvasEditOverlay.tsx'),
      'utf8',
    );
    const branch = src.slice(src.indexOf('앵커 밖에서 놓으면 그 선이 사라진다'));
    expect(branch.slice(0, 1200)).toContain('removeNodesWithConnectors');
  });
});

describe('움직이지 않았으면 아무 일도 없다 (REQ-04)', () => {
  it('오버레이에 그 가드가 있다 — 더블클릭이 선을 지우지 않는다', () => {
    // **출시된 시험이 이 가드를 요구했다**: 016 을 붙이자 `canvas011PointEdit` 의 끝점
    // 더블클릭 시험이 곧바로 울었다. 누름과 뗌이 같은 자리에서 일어난 것은 끈 것이
    // 아니라 누른 것이고, 거기서 선을 지우면 사용자는 점을 빼려다 선을 잃는다.
    const src = readFileSync(
      resolve(process.cwd(), 'src/pages/dashboard/panels/canvas/CanvasEditOverlay.tsx'),
      'utf8',
    );
    expect(src).toContain('ENDPOINT_DETACH_SLOP_PX');
    const branch = src.slice(src.indexOf('앵커 밖에서 놓으면 그 선이 사라진다'));
    // 지우기 **앞에** 움직임 판정이 있다.
    const guard = branch.indexOf('const moved');
    const remove = branch.indexOf('removeNodesWithConnectors');
    expect(guard).toBeGreaterThanOrEqual(0);
    expect(guard).toBeLessThan(remove);
  });

  it('앵커 집기 오차와 **다른 축**이다', () => {
    // 저쪽은 "이 자리가 앵커인가" 를 묻고 이쪽은 "손이 움직였는가" 를 묻는다. 한 상수로
    // 접으면 앵커 오차를 넓히는 변경이 더블클릭을 조용히 삭제로 바꾼다.
    const src = readFileSync(
      resolve(process.cwd(), 'src/pages/dashboard/panels/canvas/CanvasEditOverlay.tsx'),
      'utf8',
    );
    expect(src).toContain('const ENDPOINT_DETACH_SLOP_PX');
    expect(src).not.toContain('ENDPOINT_DETACH_SLOP_PX = ANCHOR_PICK_SLOP_PX');
  });
});
