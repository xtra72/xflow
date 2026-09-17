// 앵커 도구 — **두 표면이 함께 그리는 한 벌** (SPEC-CANVAS-011 M3'b · AC-61).
//
// ## 형상은 `group/CanvasGroupTools` 그대로다
//
// 조각(Fragment)을 돌려주고 감싸는 상자 · `role="group"` · 이름은 **부르는 쪽**이 준다.
// 도크는 제 절(section)에 이름을 달고, 떠 있는 줄은 제 묶음 안에 놓는다. 그래서 004 가
// 불변식 I24 로 세운 문장이 여기서도 형상으로 참이다 — **한 도구의 두 표현이지 두 도구가
// 아니다.** 004 의 그룹 도구가 도크에만 서서 배달된 결함(006)을 011 이 다시 심지 않는
// 유일한 길이 이것이다.
//
// ## 이 파일이 **판정하지 않는** 것 셋
//
//   1. **지금 어느 도구인가** — `CanvasTool` 은 오버레이가 든다. 여기는 "앵커 도구인가"
//      하나를 불리언으로 받는다. 도구가 여섯이 되는 날(M8) 그 표는 `canvasTools.ts` 한
//      자리에서 자라고 이 파일은 한 글자도 바뀌지 않는다.
//   2. **더블클릭이 무엇을 뜻하는가** — 몸짓은 오버레이의 포인터 경로가 소유한다. 그 판정을
//      여기 한 벌 더 들면 "단추는 켜졌는데 몸짓은 옛 뜻" 이 표현 가능해진다.
//   3. **왜 놓지 못했는가** — 거절 사유는 `anchorGestureAt` 하나가 낸다(`AnchorRefusal`).
//      이 파일은 그 값을 **받아서** 문구를 고를 뿐이다.
//
// **이 파일이 소유하는 상태는 없다.** 그룹 도구가 확인 단계 하나를 들었던 것과 달리 앵커는
// 되돌릴 수 없는 것을 버리지 않는다 — 잘못 놓았으면 그 자리를 다시 더블클릭해 빼면 되고,
// 그것이 REQ-02'-c 가 만드는 몸짓과 없애는 몸짓을 **같은 것**으로 둔 까닭이다.
//
// @spec SPEC-CANVAS-011 REQ-02 · REQ-02'

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { Anchor } from 'lucide-react';

import type { AnchorRefusal } from './anchorTypes';
import { TOOL_LABEL_KEYS } from './canvasTools';

/** 아이콘 칸. 그룹 도구가 쓰는 **그 눈금**이다 — 두 묶음이 한 줄에 서므로 크기가 같아야 한다. */
const ICON_BUTTON_CLASS =
  'flex h-7 w-7 items-center justify-center rounded text-(--color-text-secondary) ' +
  'hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/** 켜진 도구. 꺼진 칸과 **칠로** 갈린다 — `aria-pressed` 가 못 보는 사람에게 같은 말을 한다. */
const ACTIVE_CLASS = 'bg-blue-500/15 text-blue-500';

/** 안내 문단 — 줄에서도 도크에서도 제 폭을 넘기지 않는다(그룹 도구와 같은 눈금). */
const NOTICE_CLASS = 'max-w-48 px-1 text-[11px] leading-snug text-(--color-text-secondary)';

const ICON_CLASS = 'h-4 w-4 shrink-0';

/**
 * 거절 사유 → i18n 키. **표로 두는 것이 요점이다**(`REFUSAL_KEY` 와 같은 규율) — 사유가
 * 하나 늘면 컴파일러가 이 자리를 가리킨다. `if/else` 로 적으면 새 사유가 조용히 빈 안내로
 * 떨어지고, 그때 화면은 "아무 일도 일어나지 않았다" 만 말한다.
 */
const ANCHOR_REFUSAL_KEY: Readonly<Record<AnchorRefusal, string>> = {
  notBoxed: 'dashboard.canvas.edit.anchorRefusalNotBoxed',
};

export interface CanvasAnchorToolsProps {
  /** 앵커 도구가 켜져 있는가. 도구 상태의 주인은 오버레이다. */
  active: boolean;
  /** 단추를 눌렀다. 다음 도구가 무엇인지는 `toggleTool` 이 정한다. */
  onToggle: () => void;
  /** 마지막 거절 사유. `null` 이면 안내가 없다. */
  refusal: AnchorRefusal | null;
}

/**
 * 토글 하나 + 안내 하나.
 *
 * 접근성은 도크의 기존 줄이 세운 관용구를 그대로 따른다 — 진짜 `<button>` 에 `aria-label`
 * 과 `title` 을 같은 문구로 달고, 아이콘은 `aria-hidden` 이다. 토글이므로 `aria-pressed`
 * 가 한 줄 더 붙는다: 칠만으로 켜짐을 말하면 그 사실이 **보는 사람에게만** 있다.
 *
 * 안내는 `role="status"` 다. `alert` 가 아닌 것에 뜻이 있다 — 사용자가 방금 한 손짓에 대한
 * 응답이지 배경에서 터진 사건이 아니며, `alert` 는 읽던 문장을 끊는다(그룹 도구와 같다).
 */
export function CanvasAnchorTools({
  active,
  onToggle,
  refusal,
}: CanvasAnchorToolsProps): React.ReactElement {
  const { t } = useTranslation();
  return (
    <>
      <button
        type="button"
        data-testid="canvas-anchor-tool"
        aria-label={t(TOOL_LABEL_KEYS.anchor)}
        title={t(TOOL_LABEL_KEYS.anchor)}
        aria-pressed={active}
        className={cn(ICON_BUTTON_CLASS, active && ACTIVE_CLASS)}
        onClick={onToggle}
      >
        <Anchor className={ICON_CLASS} aria-hidden="true" />
      </button>
      {refusal !== null && (
        <p data-testid="canvas-anchor-refusal" role="status" className={NOTICE_CLASS}>
          {t(ANCHOR_REFUSAL_KEY[refusal])}
        </p>
      )}
    </>
  );
}
