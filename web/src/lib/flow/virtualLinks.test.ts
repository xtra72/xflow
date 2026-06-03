// 가상(네임드) 링크 배지 계산 헬퍼 테스트 (SPEC-LINK-001).
// 배지 합치기(같은 포트+이름 → 1개), 방향 분류, 비가상 무시를 검증한다.

import { describe, expect, it } from 'vitest';
import type { Edge } from '@xyflow/react';

import {
  computeLinkBadges,
  DEFAULT_PORT,
  edgeLinkName,
  isVirtualEdge,
} from './virtualLinks';

/** 테스트용 엣지 생성 헬퍼(최상위 virtual/name 속성 포함). */
function makeEdge(partial: Partial<Edge> & Record<string, unknown>): Edge {
  return {
    id: 'e',
    source: 's',
    target: 't',
    ...partial,
  } as Edge;
}

describe('isVirtualEdge / edgeLinkName', () => {
  it('virtual === true 만 가상으로 본다', () => {
    expect(isVirtualEdge(makeEdge({ virtual: true }))).toBe(true);
    expect(isVirtualEdge(makeEdge({ virtual: false }))).toBe(false);
    expect(isVirtualEdge(makeEdge({}))).toBe(false);
  });

  it('name 이 문자열이 아니면 빈 문자열로 반환한다', () => {
    expect(edgeLinkName(makeEdge({ name: 'sensor' }))).toBe('sensor');
    expect(edgeLinkName(makeEdge({}))).toBe('');
    expect(edgeLinkName(makeEdge({ name: 123 }))).toBe('');
  });
});

describe('computeLinkBadges', () => {
  it('소스 노드의 가상 와이어를 출력 배지로 분류한다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        target: 'B',
        sourceHandle: 'out',
        targetHandle: 'in',
        virtual: true,
        name: 'sensor',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs).toEqual([
      { port: 'out', name: 'sensor', edgeIds: ['e1'] },
    ]);
    expect(a.inputs).toEqual([]);

    const b = computeLinkBadges(edges, 'B');
    expect(b.inputs).toEqual([
      { port: 'in', name: 'sensor', edgeIds: ['e1'] },
    ]);
    expect(b.outputs).toEqual([]);
  });

  it('같은 (포트, 이름) 의 가상 와이어 N개를 배지 1개로 합치고 edgeIds 를 모은다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        target: 'B',
        sourceHandle: 'out',
        virtual: true,
        name: 'sensor',
      }),
      makeEdge({
        id: 'e2',
        source: 'A',
        target: 'C',
        sourceHandle: 'out',
        virtual: true,
        name: 'sensor',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs).toHaveLength(1);
    expect(a.outputs[0]).toEqual({
      port: 'out',
      name: 'sensor',
      edgeIds: ['e1', 'e2'],
    });
  });

  it('같은 포트라도 이름이 다르면 배지를 분리한다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        sourceHandle: 'out',
        virtual: true,
        name: 'sensor',
      }),
      makeEdge({
        id: 'e2',
        source: 'A',
        sourceHandle: 'out',
        virtual: true,
        name: 'alarm',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs.map((b) => b.name)).toEqual(['alarm', 'sensor']);
  });

  it('비가상 와이어는 배지 대상에서 제외한다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        sourceHandle: 'out',
        virtual: false,
        name: 'sensor',
      }),
      makeEdge({
        id: 'e2',
        source: 'A',
        sourceHandle: 'out',
        name: 'noflag',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs).toEqual([]);
    expect(a.inputs).toEqual([]);
  });

  it('핸들 id 가 없으면 DEFAULT_PORT 로 분류한다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        target: 'B',
        virtual: true,
        name: 'sensor',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs[0]?.port).toBe(DEFAULT_PORT);
    const b = computeLinkBadges(edges, 'B');
    expect(b.inputs[0]?.port).toBe(DEFAULT_PORT);
  });

  it('자기 자신을 source/target 으로 동시에 가지면 출력/입력 양쪽에 배지가 생긴다', () => {
    const edges = [
      makeEdge({
        id: 'e1',
        source: 'A',
        target: 'A',
        sourceHandle: 'out',
        targetHandle: 'in',
        virtual: true,
        name: 'loop',
      }),
    ];
    const a = computeLinkBadges(edges, 'A');
    expect(a.outputs).toHaveLength(1);
    expect(a.inputs).toHaveLength(1);
  });
});
