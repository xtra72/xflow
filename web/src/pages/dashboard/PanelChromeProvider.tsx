// 패널 크롬(공통 외곽 UI) 옵션 Provider.
//
// 컨텍스트 객체와 읽기 훅(usePanelTitleVisible), config 파서(readPanelChrome)는
// panelChromeContext.ts 에 있다. 컴포넌트 파일이 컴포넌트만 내보내야 Fast Refresh 가
// 동작하기 때문에 Provider 만 분리했다(react-refresh/only-export-components).

import { useMemo, type ReactNode } from 'react';

import { PanelChromeContext } from './panelChromeContext';

/** 패널 하나를 감싸 크롬 옵션을 그 안쪽 전체에 전파한다. */
export function PanelChromeProvider({
  config,
  children,
}: {
  config: Record<string, unknown> | undefined;
  children: ReactNode;
}) {
  const showTitle = config?.showTitle !== false;
  const value = useMemo(() => ({ showTitle }), [showTitle]);
  return <PanelChromeContext.Provider value={value}>{children}</PanelChromeContext.Provider>;
}
