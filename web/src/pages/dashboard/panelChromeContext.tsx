// 패널 크롬(공통 외곽 UI) 옵션 전파.
//
// 타이틀 바 표시 여부는 패널 28종 전부가 알아야 하는 값인데, 각 패널이 config 를 받는 방식이
// 제각각이고(`config` / `panelConfig` / 아예 안 받음) MODBUS 6종은 공용 프레임을 34곳에서
// 호출한다. prop 으로 꿰면 호출부마다 편집이 필요하지만, context 로 두면 헤더를 그리는 쪽에서
// 한 번만 읽으면 된다.
//
// Provider 가 없으면 표시(true)다 — 패널을 단독 렌더하는 기존 테스트/화면이 그대로 동작한다.

import { createContext, useContext, useMemo, type ReactNode } from 'react';

export interface PanelChrome {
  /** 타이틀 바를 그릴지 여부. */
  showTitle: boolean;
}

const DEFAULT_CHROME: PanelChrome = { showTitle: true };

const PanelChromeContext = createContext<PanelChrome>(DEFAULT_CHROME);

/**
 * 패널 config 에서 크롬 옵션을 읽는다. `showTitle` 이 명시적으로 false 일 때만 숨긴다 —
 * 미설정/손상 값은 기존 동작(표시)으로 폴백한다.
 */
export function readPanelChrome(config: Record<string, unknown> | undefined): PanelChrome {
  return { showTitle: config?.showTitle !== false };
}

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

/** 타이틀 바를 그릴지. Provider 밖에서는 항상 true(기존 동작). */
export function usePanelTitleVisible(): boolean {
  return useContext(PanelChromeContext).showTitle;
}
