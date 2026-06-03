// 가상(네임드) 링크 — 와이어 가상화 표시 헬퍼 (SPEC-LINK-001).
//
// `virtual=true` 인 엣지는 캔버스에서 긴 연결선을 숨기고, 양 끝 포트 옆에
// "출력 링크 [name]" / "입력 링크 [name]" 배지로 분해 표시한다. 이 모듈은
// 그 배지 목록을 계산하는 순수 함수만 제공한다(렌더링/스토어 의존성 없음).
//
// 계약: `virtual`(bool) 과 `name`(string) 은 백엔드 wire 필드와 1:1 매핑되는
// 엣지의 최상위 속성이다(`mode`/`buffer_size`/`name` 과 동일 패턴). React Flow
// `Edge` 타입에는 정의되지 않은 추가 속성이므로 Record 캐스팅으로 읽는다.

import type { Edge } from '@xyflow/react';

/** 핸들 id 가 없을 때 사용하는 기본 포트 이름. */
export const DEFAULT_PORT = 'default';

/**
 * 한 포트에 표시되는 가상 링크 배지 1개.
 *
 * 같은 (포트, 이름) 조합의 가상 와이어가 여러 개여도 배지는 하나로 합친다
 * (배지 합치기 규칙 — SPEC OPEN QUESTION #1 결정: 합친다). 합쳐진 모든
 * 와이어 id 는 `edgeIds` 로 보존해 삭제/디버깅 등에 활용할 수 있다.
 */
export interface LinkBadge {
  /** 배지가 붙는 포트 이름(sourceHandle 또는 targetHandle, 없으면 DEFAULT_PORT). */
  port: string;
  /** 링크 이름(그룹 식별자). 빈 문자열일 수 있다. */
  name: string;
  /** 이 배지로 합쳐진 가상 와이어들의 edge id 목록(정렬됨, 중복 없음). */
  edgeIds: string[];
}

/** 한 노드의 입력/출력 가상 링크 배지 묶음. */
export interface NodeLinkBadges {
  /** source 포트(출력) 옆에 표시할 "출력 링크" 배지들. */
  outputs: LinkBadge[];
  /** target 포트(입력) 옆에 표시할 "입력 링크" 배지들. */
  inputs: LinkBadge[];
}

/** 엣지가 가상 링크인지 여부(최상위 `virtual` 속성). */
export function isVirtualEdge(edge: Edge): boolean {
  return (edge as Record<string, unknown>).virtual === true;
}

/** 엣지의 링크 이름(최상위 `name` 속성). 없으면 빈 문자열. */
export function edgeLinkName(edge: Edge): string {
  const name = (edge as Record<string, unknown>).name;
  return typeof name === 'string' ? name : '';
}

/** (포트, 이름) 그룹을 식별하는 안정적인 키. */
function groupKey(port: string, name: string): string {
  // 구분자 충돌을 피하기 위해 길이 접두사로 직렬화한다.
  return `${port.length}:${port}|${name}`;
}

/**
 * 주어진 엣지 목록에서 특정 노드의 가상 링크 배지를 계산하는 순수 헬퍼.
 *
 * - `edge.source === nodeId` 이고 가상인 엣지 → 출력(`outputs`) 배지.
 *   포트 = `sourceHandle ?? DEFAULT_PORT`.
 * - `edge.target === nodeId` 이고 가상인 엣지 → 입력(`inputs`) 배지.
 *   포트 = `targetHandle ?? DEFAULT_PORT`.
 * - 같은 (포트, 이름) 조합은 배지 하나로 합치고, 합쳐진 edge id 를 모은다.
 *
 * 비가상 엣지는 무시한다(연결선으로 정상 렌더되므로 배지 대상이 아니다).
 */
export function computeLinkBadges(edges: Edge[], nodeId: string): NodeLinkBadges {
  const outMap = new Map<string, LinkBadge>();
  const inMap = new Map<string, LinkBadge>();

  for (const edge of edges) {
    if (!isVirtualEdge(edge)) continue;
    const name = edgeLinkName(edge);

    if (edge.source === nodeId) {
      const port = edge.sourceHandle ?? DEFAULT_PORT;
      addToGroup(outMap, port, name, edge.id);
    }
    if (edge.target === nodeId) {
      const port = edge.targetHandle ?? DEFAULT_PORT;
      addToGroup(inMap, port, name, edge.id);
    }
  }

  return {
    outputs: finalizeBadges(outMap),
    inputs: finalizeBadges(inMap),
  };
}

/** (포트, 이름) 그룹 맵에 edge id 를 누적한다. */
function addToGroup(
  map: Map<string, LinkBadge>,
  port: string,
  name: string,
  edgeId: string,
): void {
  const key = groupKey(port, name);
  const existing = map.get(key);
  if (existing) {
    if (!existing.edgeIds.includes(edgeId)) {
      existing.edgeIds.push(edgeId);
    }
  } else {
    map.set(key, { port, name, edgeIds: [edgeId] });
  }
}

/** 그룹 맵을 정렬된 배지 배열로 변환한다(결정론적 순서). */
function finalizeBadges(map: Map<string, LinkBadge>): LinkBadge[] {
  return Array.from(map.values())
    .map((b) => ({ ...b, edgeIds: [...b.edgeIds].sort() }))
    .sort((a, b) => a.port.localeCompare(b.port) || a.name.localeCompare(b.name));
}
