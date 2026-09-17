// 연결선 도구 넷 — **두 표면이 함께 그리는 한 벌** (SPEC-CANVAS-011 M8 · AC-61).
//
// ## 형상은 `CanvasAnchorTools` 그대로다
//
// 조각(Fragment)을 돌려주고, 감싸는 상자 · `role="group"` · 이름은 **부르는 쪽**이 준다.
// 도크는 제 절(section)에 이름을 달고, 떠 있는 줄은 제 묶음 안에 놓는다. 그래서 004 가
// 불변식 I24 로 세운 문장이 여기서도 형상으로 참이다 — **한 도구의 두 표현이지 두 도구가
// 아니다.** 004 의 그룹 도구가 도크에만 서서 배달된 결함(006)을 011 이 다시 심지 않는
// 유일한 길이 이것이다.
//
// ## 단추가 **표에서 나온다** — 넷을 손으로 적지 않는다
//
// 순회는 `CANVAS_TOOLS` 전량이고, 거르는 자도 아이콘을 고르는 자도 `TOOL_CONNECTOR_ROUTE`
// 하나다: 그 표가 `null` 이면 연결선 도구가 아니므로 단추가 서지 않고, `route` 면 그
// `route` 로 아이콘 표를 조회한다. 그래서 **목록이 이 파일에 없다** — 다섯째 연결선 도구가
// 생기면 단추가 저절로 서고, 다섯째 `route` 가 생기면 컴파일러가 아이콘 표를 가리킨다
// (`Record<ConnectorRoute, …>`). 손으로 적었다면 새 도구는 **단추 없이** 켜지기만 했을
// 것이고, 그것이 `canvasTools.ts` 가 표 넷으로 막는 결함의 화면 쪽 짝이다.
//
// ## 이 파일이 **판정하지 않는** 것 셋
//
//   1. **지금 어느 도구인가** — `CanvasTool` 은 오버레이가 든다. 여기는 그 값을 받아
//      `aria-pressed` 를 칠할 뿐이다.
//   2. **무엇이 이어지는가** — 앵커를 집는 일도, 연결선을 만드는 일도 오버레이의 포인터
//      경로와 `appendConnector` 의 몫이다. 그 판정을 여기 한 벌 더 들면 "단추는 켜졌는데
//      몸짓은 옛 뜻" 이 표현 가능해진다.
//   3. **왜 그어지지 않았는가** — 앵커가 아닌 곳에서 시작한 누름은 **거절이 아니라 다른
//      몸짓**이다(고르기 · 마키). 거절 사유가 없으므로 안내 문단도 없다 — 앵커 도구가
//      `AnchorRefusal` 을 든 것과 갈리는 자리이며, 없는 사유에 문단을 지어 두면 그 문단은
//      영영 뜨지 않는 죽은 DOM 이다.
//
// **이 파일이 소유하는 상태는 없다.**
//
// @spec SPEC-CANVAS-011 REQ-03 · REQ-04

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import {
  CornerDownRight,
  PenLine,
  Pentagon,
  Slash,
  Spline,
  Waypoints,
  type LucideIcon,
} from 'lucide-react';

import {
  CANVAS_TOOLS,
  TOOL_CONNECTOR_ROUTE,
  TOOL_LABEL_KEYS,
  type CanvasTool,
} from './canvasTools';
import type { ConnectorRoute } from './connectorTypes';

/**
 * 그리는 법 → 아이콘. **표로 두는 것이 요점이다**(`REFUSAL_KEY` · `ANCHOR_REFUSAL_KEY` 와
 * 같은 규율). `if/else` 로 적으면 새 갈래가 조용히 빈 칸으로 떨어진다.
 *
 * 키가 **도구가 아니라 `route`** 인 것에 뜻이 있다: 단추가 말하는 것은 "이 도구를 켜면
 * 어떤 선이 그어지는가" 이고, 그 답은 `route` 다. 도구 이름으로 키를 잡으면 연결선을 긋지
 * 않는 둘(고르기 · 앵커)까지 이 표에 자리를 얻어 `null` 아이콘이 생긴다.
 *
 * 고른 그림은 넷이 서로 **한눈에** 갈리는 것들이다: 곧은 빗금 · 직각으로 꺾인 길 · 부드러운
 * 곡선 · 손으로 그은 획.
 */
const ROUTE_ICON: Readonly<Record<ConnectorRoute, LucideIcon>> = {
  straight: Slash,
  elbow: Waypoints,
  // 직각(SPEC-CANVAS-015) — **꺾이는 모서리 하나**가 그림이다. 꺾은선(`Waypoints`)이 점
  // 여럿을 그리는 것과 달리, 이 아이콘은 "직각으로 한 번 꺾는다" 를 말한다.
  ortho: CornerDownRight,
  curve: Spline,
  free: PenLine,
};

/** 아이콘 칸. 앵커·그룹 도구가 쓰는 **그 눈금**이다 — 세 묶음이 한 줄에 선다. */
const ICON_BUTTON_CLASS =
  'flex h-7 w-7 items-center justify-center rounded text-(--color-text-secondary) ' +
  'hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/** 켜진 도구. 꺼진 칸과 **칠로** 갈린다 — `aria-pressed` 가 못 보는 사람에게 같은 말을 한다. */
const ACTIVE_CLASS = 'bg-blue-500/15 text-blue-500';

const ICON_CLASS = 'h-4 w-4 shrink-0';

export interface CanvasConnectorToolsProps {
  /** 지금 손에 쥔 도구. 넷 중 하나이거나 그 밖이며, 그 밖이면 넷 다 꺼져 있다. */
  tool: CanvasTool;
  /** 단추를 눌렀다. 다음 도구가 무엇인지는 `toggleTool` 이 정한다. */
  onToggle: (tool: CanvasTool) => void;
  /** 도형 모드가 켜져 있는가 (SPEC-CANVAS-022 REQ-01). */
  shapeMode: boolean;
  /** 도형 단추를 눌렀다. 모양을 정할 도구까지 켜는 일은 **부르는 쪽**이 한다. */
  onToggleShape: () => void;
}

/**
 * 토글 넷. 언제나 **많아야 하나**가 켜져 있다 — 도구 상태가 갈래 하나이기 때문이며,
 * "직선과 곡선이 함께 켜져 있다" 는 형상으로 표현 불가능하다(`canvasTools.ts` 머리말).
 *
 * 접근성은 앵커 도구가 세운 관용구를 그대로 따른다 — 진짜 `<button>` 에 `aria-label` 과
 * `title` 을 같은 문구로 달고, 아이콘은 `aria-hidden` 이며, 토글이므로 `aria-pressed` 가
 * 한 줄 더 붙는다. 칠만으로 켜짐을 말하면 그 사실이 **보는 사람에게만** 있다.
 */
export function CanvasConnectorTools({
  tool,
  onToggle,
  shapeMode,
  onToggleShape,
}: CanvasConnectorToolsProps): React.ReactElement {
  const { t } = useTranslation();
  return (
    <>
      {CANVAS_TOOLS.map((id) => {
        const route = TOOL_CONNECTOR_ROUTE[id];
        // 연결선을 긋지 않는 도구는 이 묶음에 단추를 갖지 않는다 — 고르기와 앵커는 제
        // 자리가 따로 있고, 없는 도구를 여기 세우면 묶음의 이름이 거짓말이 된다.
        if (route === null) return null;
        const Icon = ROUTE_ICON[route];
        const active = tool === id;
        return (
          <button
            key={id}
            type="button"
            data-testid={`canvas-connector-tool-${id}`}
            aria-label={t(TOOL_LABEL_KEYS[id])}
            title={t(TOOL_LABEL_KEYS[id])}
            aria-pressed={active}
            className={cn(ICON_BUTTON_CLASS, active && ACTIVE_CLASS)}
            onClick={() => onToggle(id)}
          >
            <Icon className={ICON_CLASS} aria-hidden="true" />
          </button>
        );
      })}
      {/* **도형 모드** (SPEC-CANVAS-022 REQ-01 · §결정 1).

          위 넷과 **배타가 아니다** — 저 넷은 "어떤 선을 긋는가" 를 고르고 이것은 "그 선으로
          무엇을 짓는가" 를 고른다. 그래서 `aria-pressed` 를 쓰되 같은 무리의 다섯째 단추로
          두지 않고, 같은 줄 끝에 선다.

          단추가 이 부품에 있는 것에 뜻이 있다: 이 부품은 도크와 떠 있는 줄 **둘 다**에
          서므로(불변식 I23), 여기 두면 대시보드에 놓인 패널에서도 도형을 그릴 수 있다.
          도크에만 두면 006 이 배달한 그 결함을 다시 심는다. */}
      <button
        type="button"
        data-testid="canvas-shape-tool"
        aria-label={t('dashboard.canvas.edit.toolShape')}
        title={t('dashboard.canvas.edit.toolShape')}
        aria-pressed={shapeMode}
        className={cn(ICON_BUTTON_CLASS, shapeMode && ACTIVE_CLASS)}
        onClick={onToggleShape}
      >
        <Pentagon className={ICON_CLASS} aria-hidden="true" />
      </button>
    </>
  );
}
