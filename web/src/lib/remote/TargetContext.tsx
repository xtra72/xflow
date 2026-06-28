// TargetContext — 현재 자원 타깃(로컬 | 원격 노드)을 페이지/패널 트리에 공급한다
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J13).
//
// 목록 페이지가 URL 의 `?target=` 를 파싱해 Provider 로 감싸면, 하위 상세 패널은
// useTargetContext() 로 동일 타깃을 읽어 데이터 소스/액션을 전환한다. prop-drilling
// 없이 깊은 트리(예: AgentDetailPanel 탭들)에 타깃을 전파하기 위함이다.
//
// 미설정 시 기본값은 로컬이므로, Provider 로 감싸지 않은 기존 콜사이트(로컬
// 전용)는 회귀 없이 로컬로 동작한다.

import { createContext, useContext, useMemo, type ReactNode } from 'react';

import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';

const TargetContext = createContext<ResourceTarget>(LOCAL_TARGET);

/** 현재 자원 타깃을 공급한다. */
export function TargetProvider({
  target,
  children,
}: {
  target: ResourceTarget;
  children: ReactNode;
}): React.JSX.Element {
  // 참조 안정성: type/instanceId 가 같으면 동일 객체를 유지한다.
  const instanceKey = target.type === 'remote' ? target.instanceId : '';
  const value = useMemo<ResourceTarget>(
    () => (instanceKey ? { type: 'remote', instanceId: instanceKey } : LOCAL_TARGET),
    [instanceKey],
  );
  return <TargetContext.Provider value={value}>{children}</TargetContext.Provider>;
}

/** 현재 자원 타깃을 읽는다(미설정 시 로컬). */
export function useTargetContext(): ResourceTarget {
  return useContext(TargetContext);
}
