// TargetContext — 현재 자원 타깃(로컬 | 원격 노드) 컨텍스트와 읽기 훅
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J13).
//
// 목록 페이지가 URL 의 `?target=` 를 파싱해 Provider(TargetProvider.tsx)로 감싸면,
// 하위 상세 패널은 useTargetContext() 로 동일 타깃을 읽어 데이터 소스/액션을 전환한다.
// prop-drilling 없이 깊은 트리(예: AgentDetailPanel 탭들)에 타깃을 전파하기 위함이다.
//
// 미설정 시 기본값은 로컬이므로, Provider 로 감싸지 않은 기존 콜사이트(로컬
// 전용)는 회귀 없이 로컬로 동작한다.

import { createContext, useContext } from 'react';

import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';

export const TargetContext = createContext<ResourceTarget>(LOCAL_TARGET);

/** 현재 자원 타깃을 읽는다(미설정 시 로컬). */
export function useTargetContext(): ResourceTarget {
  return useContext(TargetContext);
}
