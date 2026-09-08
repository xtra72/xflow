// 패널이 "편집 중"인지 알리는 컨텍스트.
//
// 설정 대화상자의 미리보기는 대시보드와 **같은 렌더러**를 쓴다(renderDashboardPanel).
// 같은 컴포넌트가 두 자리에서 그려지므로, 미리보기에서만 켜야 하는 직접 조작(카드를
// 끌어 옮기기 등)을 prop 으로 꿰면 28종 호출부를 모두 고쳐야 한다.
//
// Provider 밖에서는 false — 대시보드에 그려진 패널은 지금까지와 똑같이 동작한다.

import { createContext, useContext } from 'react';

export const PanelEditContext = createContext(false);

/** 지금 이 패널이 설정 미리보기 안에 있는지. Provider 밖에서는 false. */
export function usePanelEditing(): boolean {
  return useContext(PanelEditContext);
}
