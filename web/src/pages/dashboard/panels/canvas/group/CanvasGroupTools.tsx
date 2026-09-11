// 그룹 · 그룹 해제 컨트롤 — **두 표면이 함께 그리는 한 벌** (SPEC-CANVAS-004 M6).
//
// ## 왜 컴포넌트 하나인가
//
// 006 의 불변식 I23 은 "표시 층이 그려지는 자리에는 그 층을 다스리는 손잡이가 닿아야
// 한다" 이고, 006 자신이 그것을 어겨 결함 하나를 배달했다 — 표시 층 셋은 조건 없이
// 그려지는데 컨트롤 전부가 `dockHost !== null` 뒤에 있었고, 도크를 펴는 곳은 설정
// 다이얼로그 한 자리뿐이었다. 004 의 그룹은 **두 표면 모두에서 그려지고 두 표면 모두에서
// 선택된다**(오버레이가 대시보드에서도 `edit.active` 로 선다 — 실측). 그러므로 컨트롤도
// 두 표면 모두에 서야 한다(가정 A21).
//
// 두 자리에 **각각 짜 넣지 않고** 컴포넌트 하나를 두 자리에서 그리는 것에 뜻이 있다.
// 배율 칸이 006 M10 에서 세운 그 형상이며(`CanvasWorkspaceZoomField`), 그 덕에 활성
// 조건 · 확인 절차 · 거절 문구가 두 벌이 되지 않는다(불변식 I24 — 한 도구의 두 표현이지
// 두 도구가 아니다). 두 벌이 되면 "설정에서는 물어보는데 대시보드에서는 그냥 풀린다" 가
// 표현 가능해지고, 그 차이는 규칙 표를 잃고 나서야 드러난다.
//
// ## 이 파일이 **판정하지 않는** 것 셋
//
//   1. **묶을 수 있는가** — `selection.size >= 2` 는 오버레이가 잰다. 여기서 다시 세면
//      단추의 활성 조건과 `groupNodes` 의 거절 조건이 갈라진다.
//   2. **무엇을 푸는가** — 고른 그룹을 찾는 일은 오버레이가 한다(핸들이 서는 규칙과
//      같은 자를 써야 하기 때문이다).
//   3. **몇 행이 버려지는가** — `rulesLostByUngroup` 하나가 판정하고(M5) 이 파일은 그 수를
//      **받아서** `> 0` 인지만 본다. 판정이 둘이 되면 "안내는 떴는데 실제로는 안 버렸다"
//      와 그 반대가 함께 가능해진다.
//
// 이 파일이 실제로 소유하는 상태는 **확인 중인가** 하나뿐이고, 그것도 스크래치패드의
// 지우기 확인이 이미 세운 관용구 그대로다(`scratchpadRemoveAsk`/`Yes`/`No`).
//
// @spec SPEC-CANVAS-004 REQ-07 / REQ-08

import { useEffect, useState } from 'react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { Check, Group, Ungroup, X } from 'lucide-react';

import type { GroupRefusal } from './groupOps';

/** 아이콘 칸. 도크의 정렬 6종이 쓰는 그 눈금이다 — 좁은 줄에도 같은 크기로 선다. */
const ICON_BUTTON_CLASS =
  'flex h-7 w-7 items-center justify-center rounded text-(--color-text-secondary) ' +
  'hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

/** 쓸 수 없는 칸. `pointer-events-none` 이 함께 있어야 흐려진 칸에서 호버가 살지 않는다. */
const DISABLED_CLASS = 'disabled:pointer-events-none disabled:opacity-40';

const ICON_CLASS = 'h-4 w-4 shrink-0';

/** 안내 문단 — 줄에서도 도크에서도 제 폭을 넘기지 않는다. */
const NOTICE_CLASS = 'max-w-48 px-1 text-[11px] leading-snug text-(--color-text-secondary)';

/**
 * 거절 사유 → i18n 키. **표로 두는 것이 요점이다** — 사유가 하나 늘면 컴파일러가 이 자리를
 * 가리킨다(`Record<GroupRefusal, string>`). `if/else` 로 적으면 새 사유가 조용히 빈 안내로
 * 떨어지고, 그때 화면은 "아무 일도 일어나지 않았다" 만 말한다.
 */
const REFUSAL_KEY: Readonly<Record<GroupRefusal, string>> = {
  nested: 'dashboard.canvas.edit.groupRefusalNested',
  tooFew: 'dashboard.canvas.edit.groupRefusalTooFew',
};

export interface CanvasGroupToolsProps {
  /** 묶을 것이 둘 이상인가. 판정은 오버레이가 소유한다. */
  canGroup: boolean;
  /** 풀 그룹이 정확히 하나 골라져 있는가. */
  canUngroup: boolean;
  /**
   * 풀면 버려질 규칙 행의 수(M5 의 `rulesLostByUngroup`).
   *
   * **0 보다 클 때만** 확인을 묻는다(REQ-07). 언제나 물으면 잃을 것이 없는 풀기에도 한
   * 걸음이 붙고, 한 걸음이 늘 붙는 확인은 곧 읽지 않고 누르는 확인이 된다.
   */
  rulesAtRisk: number;
  /** 마지막 묶기 거절 사유. `null` 이면 안내가 없다. */
  refusal: GroupRefusal | null;
  onGroup: () => void;
  onUngroup: () => void;
}

/**
 * 단추 둘 + 안내 하나. **상태는 "확인 중인가" 하나뿐이다.**
 *
 * 접근성은 도크의 기존 줄이 세운 관용구를 그대로 따른다 — 진짜 `<button>` 에 `aria-label`
 * 과 `title` 을 같은 문구로 달고, 아이콘은 `aria-hidden` 이다. 그래서 탭 이동 · Enter/Space
 * 활성화 · 스크린 리더 이름이 전부 공짜로 따라온다(칠한 손잡이였다면 그 전부를 손으로
 * 다시 만들어야 한다 — 002 가 핸들에 대해 적은 그 근거다).
 *
 * 안내 둘은 `role="status"` 다. `alert` 가 아닌 것에 뜻이 있다 — 사용자가 방금 누른 단추에
 * 대한 응답이지 배경에서 터진 사건이 아니며, `alert` 는 읽던 문장을 끊는다.
 */
export function CanvasGroupTools({
  canGroup,
  canUngroup,
  rulesAtRisk,
  refusal,
  onGroup,
  onUngroup,
}: CanvasGroupToolsProps): React.ReactElement {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState(false);

  // 풀 대상이 사라지거나 바뀌면 확인을 거둔다. 거두지 않으면 다른 것을 고른 뒤에도 확인이
  // 남아, 그 자리에서 "예" 를 누르면 **묻지 않은 것이 풀린다**.
  useEffect(() => {
    if (!canUngroup) setConfirming(false);
  }, [canUngroup]);

  const askBeforeUngroup = (): void => {
    // **잃을 것이 없으면 묻지 않는다**(REQ-07 — 안내는 `rulesAtRisk > 0` 일 때만 뜬다).
    if (rulesAtRisk > 0) {
      setConfirming(true);
      return;
    }
    onUngroup();
  };

  const confirmUngroup = (): void => {
    setConfirming(false);
    onUngroup();
  };

  return (
    <>
      <button
        type="button"
        data-testid="canvas-group-create"
        aria-label={t('dashboard.canvas.edit.groupCreate')}
        title={t('dashboard.canvas.edit.groupCreate')}
        disabled={!canGroup}
        className={cn(ICON_BUTTON_CLASS, DISABLED_CLASS)}
        onClick={onGroup}
      >
        <Group className={ICON_CLASS} aria-hidden="true" />
      </button>
      <button
        type="button"
        data-testid="canvas-group-ungroup"
        aria-label={t('dashboard.canvas.edit.groupUngroup')}
        title={t('dashboard.canvas.edit.groupUngroup')}
        disabled={!canUngroup}
        className={cn(ICON_BUTTON_CLASS, DISABLED_CLASS)}
        onClick={askBeforeUngroup}
      >
        <Ungroup className={ICON_CLASS} aria-hidden="true" />
      </button>
      {/* 풀기 확인 — **푸는 것을 되돌릴 수 없어서가 아니라, 규칙 표를 되돌릴 수 없어서다.**
          좌표는 왕복하지만 그룹의 `rules` 는 버려진다(가정 A19): 규칙은 값이 아니라 판정
          이므로 N 개 부품에 복사하면 프레임당 평가가 1회에서 N회로 늘고, 그중 한 표만
          나중에 고쳐지는 순간 "같이 흐려지던 것이 따로 논다". 잃는다는 사실을 **잃기
          전에** 말하지 않으면 그 손실은 조용하다. */}
      {confirming && (
        <div
          data-testid="canvas-group-ungroup-ask"
          role="status"
          className="flex items-center gap-1"
        >
          {/* `replaceAll` 이다. 오늘의 두 문구는 `{rows}` 를 **한 번씩만** 말하므로
              `.replace` 로 바꿔도 아무 시험이 빨개지지 않는다 — **등가 뮤테이션이며,
              적어 두지 않으면 다음 사람이 없는 결함을 잡으러 간다.** 그 등가가 조용히
              깨지지 않도록 치환자 횟수 자체를 가드로 못박아 두었다(`canvas004I18n.test.tsx`
              §TOKEN_COUNTS) — 번역을 다듬다 둘째 자리가 생기면 그 가드가 먼저 울고,
              그때 `.replace` 는 더 이상 등가가 아니다. 008 이 `{shape}` 에서 물린 그
              자리이므로 이 저장소의 기본형을 따른다. */}
          <span className={NOTICE_CLASS}>
            {t('dashboard.canvas.edit.groupUngroupAsk').replaceAll(
              '{rows}',
              String(rulesAtRisk),
            )}
          </span>
          <button
            type="button"
            data-testid="canvas-group-ungroup-yes"
            aria-label={t('dashboard.canvas.edit.groupUngroupYes')}
            title={t('dashboard.canvas.edit.groupUngroupYes')}
            className={cn(ICON_BUTTON_CLASS, 'hover:text-red-500')}
            onClick={confirmUngroup}
          >
            <Check className={ICON_CLASS} aria-hidden="true" />
          </button>
          <button
            type="button"
            data-testid="canvas-group-ungroup-no"
            aria-label={t('dashboard.canvas.edit.groupUngroupNo')}
            title={t('dashboard.canvas.edit.groupUngroupNo')}
            className={ICON_BUTTON_CLASS}
            onClick={() => setConfirming(false)}
          >
            <X className={ICON_CLASS} aria-hidden="true" />
          </button>
        </div>
      )}
      {refusal !== null && (
        <p data-testid="canvas-group-refusal" role="status" className={NOTICE_CLASS}>
          {t(REFUSAL_KEY[refusal])}
        </p>
      )}
    </>
  );
}
