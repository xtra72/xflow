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

// ---------------------------------------------------------------------------
// 팝오버 목록용 — 포트별 가상 링크를 "상대 연결 정보"까지 포함해 계산한다.
//
// CustomNode 의 포트 옆 컴팩트 인디케이터를 클릭하면 노드 바깥에 뜨는 팝오버
// 목록의 데이터 소스다. 인라인 배지(노드 폭을 넓히던 원인) 를 대체한다.
// 순수 함수이므로 렌더링/스토어 의존성이 없고 단위 테스트로 보장한다.
// ---------------------------------------------------------------------------

/**
 * 가상 링크 1개의 상대(counterpart) 끝점 정보.
 *
 * - 출력 포트 항목의 상대 = 와이어의 타겟(노드 라벨 + targetHandle 포트).
 * - 입력 포트 항목의 상대 = 와이어의 소스(노드 라벨 + sourceHandle 포트).
 */
export interface LinkCounterpart {
  /** 상대 노드의 라벨(없으면 노드 id 로 폴백). */
  nodeLabel: string;
  /** 상대 쪽 포트 이름(핸들 id 없으면 DEFAULT_PORT). */
  port: string;
  /** 이 상대 끝점에 대응하는 가상 와이어의 edge id. */
  edgeId: string;
}

/**
 * 한 포트의 한 이름 그룹에 대한 팝오버 목록 항목.
 *
 * `(port, name)` 1개당 항목 1개로 묶이며(decision #1), 같은 이름이 여러 상대로
 * 연결되면 `counterparts` 에 모두 담는다(목록에서 다중 표시).
 */
export interface LinkListEntry {
  /** 이 노드 쪽 포트 이름(sourceHandle/targetHandle, 없으면 DEFAULT_PORT). */
  port: string;
  /** 링크 이름(그룹 식별자). 빈 문자열일 수 있다. */
  name: string;
  /** 이 항목으로 합쳐진 가상 와이어들의 edge id 목록(정렬됨, 중복 없음). */
  edgeIds: string[];
  /** 이름 그룹이 연결된 상대 끝점 목록(edgeId 기준 정렬). */
  counterparts: LinkCounterpart[];
}

/** 한 노드의 입력/출력 팝오버 목록 묶음. */
export interface NodeLinkList {
  /** source 포트(출력) 의 링크 목록 항목들. */
  outputs: LinkListEntry[];
  /** target 포트(입력) 의 링크 목록 항목들. */
  inputs: LinkListEntry[];
}

/** 그룹 맵에 항목을 누적할 때 사용하는 내부 가변 구조. */
interface MutableEntry {
  port: string;
  name: string;
  edgeIds: string[];
  counterparts: LinkCounterpart[];
}

/** (포트, 이름) 그룹 맵에 edge id 와 상대 끝점을 누적한다. */
function addToListGroup(
  map: Map<string, MutableEntry>,
  port: string,
  name: string,
  edgeId: string,
  counterpart: LinkCounterpart,
): void {
  const key = groupKey(port, name);
  const existing = map.get(key);
  if (existing) {
    if (!existing.edgeIds.includes(edgeId)) {
      existing.edgeIds.push(edgeId);
      existing.counterparts.push(counterpart);
    }
  } else {
    map.set(key, { port, name, edgeIds: [edgeId], counterparts: [counterpart] });
  }
}

/** 그룹 맵을 정렬된 목록 항목 배열로 변환한다(결정론적 순서). */
function finalizeEntries(map: Map<string, MutableEntry>): LinkListEntry[] {
  return Array.from(map.values())
    .map((e) => ({
      port: e.port,
      name: e.name,
      edgeIds: [...e.edgeIds].sort(),
      counterparts: [...e.counterparts].sort((a, b) =>
        a.edgeId.localeCompare(b.edgeId),
      ),
    }))
    .sort((a, b) => a.port.localeCompare(b.port) || a.name.localeCompare(b.name));
}

/**
 * 특정 노드의 가상 링크를 팝오버 목록용으로 계산하는 순수 헬퍼.
 *
 * `computeLinkBadges` 와 동일한 그룹핑 규칙(포트+이름 1항목, decision #1) 을
 * 쓰되, 각 항목에 상대 끝점 정보(상대 노드 라벨 + 포트) 를 추가로 담는다.
 *
 * @param edges        스토어의 전체 엣지 목록.
 * @param nodeId       기준 노드 id.
 * @param getNodeLabel 노드 id → 라벨 조회 함수(없으면 id 를 그대로 반환하도록 폴백).
 */
export function computeLinkList(
  edges: Edge[],
  nodeId: string,
  getNodeLabel: (id: string) => string,
): NodeLinkList {
  const outMap = new Map<string, MutableEntry>();
  const inMap = new Map<string, MutableEntry>();

  for (const edge of edges) {
    if (!isVirtualEdge(edge)) continue;
    const name = edgeLinkName(edge);

    if (edge.source === nodeId) {
      // 출력 항목: 상대 = 타겟 노드/포트.
      const port = edge.sourceHandle ?? DEFAULT_PORT;
      addToListGroup(outMap, port, name, edge.id, {
        nodeLabel: getNodeLabel(edge.target),
        port: edge.targetHandle ?? DEFAULT_PORT,
        edgeId: edge.id,
      });
    }
    if (edge.target === nodeId) {
      // 입력 항목: 상대 = 소스 노드/포트.
      const port = edge.targetHandle ?? DEFAULT_PORT;
      addToListGroup(inMap, port, name, edge.id, {
        nodeLabel: getNodeLabel(edge.source),
        port: edge.sourceHandle ?? DEFAULT_PORT,
        edgeId: edge.id,
      });
    }
  }

  return {
    outputs: finalizeEntries(outMap),
    inputs: finalizeEntries(inMap),
  };
}
