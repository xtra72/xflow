// 로그 이름 클릭 → 대상 메뉴 이동 핸들러.
//
// LogViewer 는 라우터에 묶이지 않도록 콜백만 받는다. 라우터를 아는 쪽(모니터링
// 페이지 / 대시보드 로그 패널)이 이 훅으로 핸들러를 만들어 넘긴다.

import { useCallback } from 'react';
import { useNavigate } from 'react-router';

import { logComponentTarget } from './logNavigation';

/** 로그 항목의 소스/이름을 받아 해당 목록 페이지로 이동하는 핸들러를 만든다. */
export function useLogComponentNavigate(): (source: string, name: string) => void {
  const navigate = useNavigate();

  return useCallback(
    (source: string, name: string) => {
      const target = logComponentTarget(source, name);
      // 이동할 수 없는 소스는 LogViewer 가 애초에 링크로 그리지 않지만,
      // 호출부가 늘어날 수 있으므로 여기서도 한 번 막는다.
      if (target) navigate(target);
    },
    [navigate],
  );
}
