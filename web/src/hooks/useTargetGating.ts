// useTargetGating — 원격 타깃 제어 게이팅 (SPEC-REMOTE-001 M8, 그룹 J, REQ-J05/J09).
//
// 원격 타깃에서 편집/제어 가능 여부를 결정한다. RemoteResourcesPage(M7)의 canEdit
// 로직(노드 approved ∧ online ∧ 자원 online)을 타깃 추상화로 일반화한 것이다.
//
//   - 로컬 타깃: 항상 제어 가능(canControl=true) — 기존 동작 불변.
//   - 원격 타깃: 노드가 승인+온라인이어야 제어 가능. 자원 단위 online 은
//     호출자가 자원별로 추가 평가한다(resourceOnline 인자).
//
// 이 훅은 server 모드가 아니거나 로컬 타깃이면 노드 쿼리를 발행하지 않는다.

import { useMemo } from 'react';

import { useManagedNodes, useRemoteMode } from '@/hooks/useRemote';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';

/** 타깃 게이팅 결과. */
export interface TargetGating {
  /** 원격 타깃 여부. */
  isRemote: boolean;
  /** 대상 노드가 승인+온라인인지(원격만 의미). 로컬은 항상 true. */
  nodeReady: boolean;
  /** 대상 노드 표시명(원격, 배너/배지용). 로컬은 undefined. */
  nodeLabel: string | undefined;
  /**
   * 자원 제어/편집 가능 여부를 계산한다. 로컬은 항상 true. 원격은 노드 ready ∧
   * (자원 online == true). resourceOnline 미지정 시 노드 ready 만으로 판정한다.
   */
  canControl: (resourceOnline?: boolean) => boolean;
}

/**
 * 타깃 게이팅을 평가한다.
 *
 * @param target - 로컬 또는 원격 노드 타깃.
 */
export function useTargetGating(target: ResourceTarget): TargetGating {
  const remote = isRemoteTarget(target);

  // server 모드가 아니면 노드 쿼리를 막아 404 노이즈를 방지한다.
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  // 원격 타깃일 때만 노드 목록을 조회한다(게이팅용).
  const { data: nodes } = useManagedNodes(undefined, remote && isServer);

  return useMemo<TargetGating>(() => {
    if (!remote) {
      return {
        isRemote: false,
        nodeReady: true,
        nodeLabel: undefined,
        canControl: () => true,
      };
    }
    const node = (nodes ?? []).find((n) => n.instance_id === target.instanceId);
    const nodeReady = !!node && node.status === 'approved' && node.online;
    const nodeLabel = node?.hostname || target.instanceId;
    return {
      isRemote: true,
      nodeReady,
      nodeLabel,
      canControl: (resourceOnline?: boolean) =>
        nodeReady && (resourceOnline === undefined || resourceOnline),
    };
  }, [remote, nodes, target]);
}
