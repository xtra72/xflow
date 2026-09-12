// 대시보드/캔버스 상한 조회 서비스 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// **왜 이 창구가 생겼는가.** SVG 가져오기의 요소 상한은 프론트엔드의 컴파일 상수였고
// 대시보드 저장 예산은 서버의 컴파일 상수였다. 둘은 서로 모르는 채 어긋났고, 그 어긋남은
// **저장 시점의 413** 으로만 드러났다 — 가져오기는 통과했으니 사용자는 원인을 볼 수 없다.
// 이제 운영자가 요소 수를 설정하고 서버가 그 수에서 예산을 유도하므로, 편집기가 같은 수를
// 읽어야 두 층이 다시 갈라지지 않는다.
//
//   GET /dashboard-limits → data: { max_canvas_elements, payload_budget_bytes }
//
// 참고: client.ts 인터셉터가 성공 envelope 의 data 를 이미 언래핑하므로 get 의 반환값이
// 곧 data 페이로드다.

import { get } from './client';

/** GET 응답 data 형태(빈 값·구형 서버 대비 — 전부 선택적이다). */
interface DashboardLimitsResponse {
  max_canvas_elements?: number | null;
  payload_budget_bytes?: number | null;
}

/** 화면이 쓰는 모양. 값을 얻지 못한 축은 `undefined` 로 남아 호출부가 폴백한다. */
export interface DashboardLimits {
  /** 한 번의 가져오기가 만들 수 있는 요소 수. */
  readonly maxCanvasElements: number | undefined;
  /** 대시보드 PUT 본문 예산(바이트). */
  readonly payloadBudgetBytes: number | undefined;
}

/** 아무것도 알아내지 못한 상태 — 호출부가 컴파일 기본값으로 떨어진다. */
const UNKNOWN: DashboardLimits = {
  maxCanvasElements: undefined,
  payloadBudgetBytes: undefined,
};

/** 양의 정수만 통과시킨다. 그 밖(음수 · 0 · 소수 · NaN · null)은 `undefined`. */
function positiveInt(value: number | null | undefined): number | undefined {
  if (typeof value !== 'number' || !Number.isInteger(value) || value <= 0) return undefined;
  return value;
}

/**
 * 서버가 적용 중인 상한을 조회한다.
 *
 * **절대 던지지 않는다.** 이 값은 편집기가 **있으면 더 잘 동작하는** 정보이지 없으면 서지
 * 못하는 정보가 아니다 — 오프라인 · 구형 서버(404) · 권한 없음(403) · 응답 형상이 바뀐
 * 경우 전부 "모른다"(`UNKNOWN`)로 떨어지고, 호출부가 컴파일 기본값으로 가져오기를
 * 계속한다. 여기서 예외를 밖으로 내보내면 **서버에 닿지 못한 편집기가 가져오기 자체를
 * 못 하게 되며**, 그것은 상한이 낡는 것보다 나쁘다.
 */
export async function getDashboardLimits(): Promise<DashboardLimits> {
  try {
    const data = await get<DashboardLimitsResponse | null>('/dashboard-limits');
    return {
      maxCanvasElements: positiveInt(data?.max_canvas_elements),
      payloadBudgetBytes: positiveInt(data?.payload_budget_bytes),
    };
  } catch {
    return UNKNOWN;
  }
}
