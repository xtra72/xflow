// TargetProvider — 현재 자원 타깃(로컬 | 원격 노드)을 페이지/패널 트리에 공급한다
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J13).
//
// 컨텍스트 객체와 읽기 훅(useTargetContext)은 TargetContext.ts 에 있다. 컴포넌트
// 파일이 컴포넌트만 내보내야 Fast Refresh 가 동작하기 때문에 Provider 만 분리했다
// (react-refresh/only-export-components).

import { useMemo, type ReactNode } from 'react';

import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import { TargetContext } from '@/lib/remote/TargetContext';

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
