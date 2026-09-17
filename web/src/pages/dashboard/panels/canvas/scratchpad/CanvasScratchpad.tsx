// 스크래치패드 — 도크 안의 드롭 존 · 목록 · 단추들 (SPEC-CANVAS-008 M9 · REQ-03 · REQ-06).
//
// **이 파일은 도크 안에서만 산다.** 도크 자리(`canvasEditDockHost`)가 없으면 오버레이가
// 도크 본문을 그리지 않고 이 컴포넌트는 그 본문의 자식이다 — 대시보드에 놓인 패널에는
// 드롭 존도 목록도 **하나도 없다**. 그 "없음" 이 006 이 배달한 결함(불변식 I23)의 되풀이가
// 아닌 이유는 팔레트와 같다: 스크래치패드는 **표시 층이 아니라 컨트롤**이고, 캔버스 표면에
// 아무것도 그리지 않으므로 다스릴 층 자체를 만들지 않는다. 놓인 요소를 다스리는 컨트롤
// (선택 · 드래그 · 8핸들 · 정렬 · 순서)은 006 M7 이 넓혀 둔 그대로 그 표면에도 닿는다.
//
// **드롭 존은 제 손으로 놓임을 받지 않는다.** 오버레이가 누름에서 루트에 포인터를 잡으므로
// (실측), 도크 위에서 뗀 `pointerup` 은 오버레이로 간다 — 여기에 `onDrop` 을 달면 한 번도
// 불리지 않는다. 이 컴포넌트가 내놓는 것은 **제 노드 하나**이며, 판정은 오버레이가 한다
// (`canvasScratchpadDrop` 머리말).
//
// **끌기로만 되는 조작을 두지 않는다**(WCAG 2.2 SC 2.5.7 — 끌기 동작). "선택 항목 저장"
// 단추는 편의가 아니라 **요구**이며(REQ-03), 끌어 넣기와 **같은 저장 함수**를 부른다
// (불변식 J11) — 둘이 갈라지면 "끌어 넣은 것과 단추로 넣은 것이 다르다" 가 생기고 그 차이는
// 서랍을 열어 보기 전까지 드러나지 않는다.
//
// **지우기는 두 걸음이다.** 이 패널에는 되돌리기가 없으므로(위험 R8), 한 번의 잘못 누름이
// 저술을 지우면 회복할 길이 없다. 확인 줄로 갈아 끼우는 편이 모달보다 값싸고 도크 폭
// (`w-44`) 안에 든다.
//
// @spec SPEC-CANVAS-008 REQ-03 · REQ-05 · REQ-06 · AC-05 · AC-06 · AC-08 · AC-E9

import { useRef, useState } from 'react';

import { Check, Inbox, Save, StickyNote, Trash2, X } from 'lucide-react';

import { FieldHelp } from '@/components/property/FieldHelp';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { useScratchpadDropRef, type ScratchpadDropPoint } from './canvasScratchpadDrop';
import { useScratchpadStore, type ScratchpadSaveStatus } from './scratchpadStore';
import {
  SCRATCHPAD_MAX_BYTES,
  SCRATCHPAD_MAX_ENTRIES,
  type ScratchpadEntry,
} from './scratchpadTypes';

// --- 겉모습 --------------------------------------------------------------

/**
 * 드롭 존. 점선 상자는 "여기에 놓을 수 있다" 는 관용구이며, 손이 위에 오면 색으로 답한다 —
 * 그 강조는 **드롭 존 자신이 입는다**(REQ-07: 캔버스 표면에는 아무것도 그리지 않는다).
 */
const DROP_ZONE_CLASS =
  'flex min-h-14 items-center justify-center rounded border-2 border-dashed px-2 py-2 ' +
  'text-center text-[11px] leading-tight transition-colors';

const DROP_IDLE_CLASS = 'border-(--color-border-default) text-(--color-text-muted)';

const DROP_ACTIVE_CLASS = 'border-blue-500 bg-blue-500/10 text-blue-500';

/** 도크의 줄 버튼과 같은 눈금 — 한 도구의 여러 표현이 같은 크기를 쓴다. */
const ROW_BUTTON_CLASS =
  'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs ' +
  'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300 ' +
  'disabled:pointer-events-none disabled:opacity-40';

const ICON_CLASS = 'h-4 w-4 shrink-0';

const SMALL_ICON_CLASS = 'h-3.5 w-3.5 shrink-0';

/** 항목 한 줄. 이름 칸이 남는 폭을 먹고 단추들이 오른쪽에 붙는다. */
const ENTRY_ROW_CLASS =
  'flex items-center gap-1 rounded px-1 py-0.5 hover:bg-(--color-bg-elevated)';

const NAME_INPUT_CLASS =
  'min-w-0 flex-1 rounded border border-transparent bg-transparent px-1 py-0.5 text-[11px] ' +
  'text-(--color-text-secondary) hover:border-(--color-border-default) ' +
  'focus:border-(--color-border-default) focus:outline-none focus:ring-1 focus:ring-blue-300';

const ICON_BUTTON_CLASS =
  'flex h-6 w-6 shrink-0 items-center justify-center rounded text-(--color-text-muted) ' +
  'hover:bg-(--color-bg-elevated) hover:text-blue-500 ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-300';

const HINT_CLASS = 'px-1 text-[11px] leading-tight text-(--color-text-muted)';

// --- 안내 문구 -----------------------------------------------------------

/**
 * 결과마다 말할 문구의 키. **거절도 결과다** — 조용히 실패하는 자리를 하나도 두지 않는다
 * (REQ-05). 리터럴 맵이라 어떤 키가 쓰이는지 검색으로 확인된다.
 */
const NOTICE_KEYS: Readonly<Record<ScratchpadSaveStatus, string>> = {
  stored: 'dashboard.canvas.edit.scratchpadSaved',
  volatile: 'dashboard.canvas.edit.scratchpadVolatile',
  empty: 'dashboard.canvas.edit.scratchpadNoSelection',
  'limit-entries': 'dashboard.canvas.edit.scratchpadLimitEntries',
  'limit-bytes': 'dashboard.canvas.edit.scratchpadLimitBytes',
};

/** 한도 문구는 상한을 **두 번** 말한다 — `replace` 면 뒤쪽이 벌거벗은 채 남는다(위험 R13). */
function noticeText(template: string): string {
  return template
    .replaceAll('{max}', String(SCRATCHPAD_MAX_ENTRIES))
    .replaceAll('{maxKb}', String(Math.round(SCRATCHPAD_MAX_BYTES / 1024)));
}

// --- 항목 한 줄 -----------------------------------------------------------

interface EntryRowProps {
  entry: ScratchpadEntry;
  onRename: (id: string, name: string) => void;
  onRemove: (id: string) => void;
  onPlace: (entry: ScratchpadEntry, at: ScratchpadDropPoint | null) => boolean;
}

function ScratchpadEntryRow({
  entry,
  onRename,
  onRemove,
  onPlace,
}: EntryRowProps): React.ReactElement {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState(false);
  /**
   * 방금 몸짓이 **끌어 놓기로 끝났는가**.
   *
   * 한 단추가 두 몸짓을 낸다: 누른 자리에서 그대로 떼면 누름(→ 계단 자리), 캔버스 위로
   * 끌어다 떼면 놓기(→ 그 자리). 브라우저는 잡힌 포인터의 뗌 뒤에도 `click` 을 이 단추로
   * 보내므로, 끌어 놓기로 이미 놓았으면 뒤따르는 누름을 **한 번 삼켜야** 한 몸짓이 요소를
   * 두 벌 만들지 않는다.
   *
   * 상태가 아니라 ref 인 것은 이 값이 그림을 바꾸지 않기 때문이다 — 바꾸면 `pointerup` 과
   * `click` 사이에 렌더가 한 번 끼어든다.
   */
  const placedByDrag = useRef(false);

  /** 이름이 없으면 자동 이름을 **보인다** — 저장하지는 않는다(`scratchpadTypes` §name). */
  const autoName = t('dashboard.canvas.edit.scratchpadAutoName').replaceAll(
    '{count}',
    String(entry.elements.length),
  );

  if (confirming) {
    return (
      <div className={ENTRY_ROW_CLASS} data-testid={`canvas-scratchpad-entry-${entry.id}`}>
        <span className="min-w-0 flex-1 truncate px-1 text-[11px] text-(--color-text-secondary)">
          {t('dashboard.canvas.edit.scratchpadRemoveAsk')}
        </span>
        <button
          type="button"
          data-testid={`canvas-scratchpad-remove-yes-${entry.id}`}
          aria-label={t('dashboard.canvas.edit.scratchpadRemoveYes')}
          title={t('dashboard.canvas.edit.scratchpadRemoveYes')}
          className={cn(ICON_BUTTON_CLASS, 'hover:text-red-500')}
          onClick={() => onRemove(entry.id)}
        >
          <Check className={SMALL_ICON_CLASS} aria-hidden="true" />
        </button>
        <button
          type="button"
          data-testid={`canvas-scratchpad-remove-no-${entry.id}`}
          aria-label={t('dashboard.canvas.edit.scratchpadRemoveNo')}
          title={t('dashboard.canvas.edit.scratchpadRemoveNo')}
          className={ICON_BUTTON_CLASS}
          onClick={() => setConfirming(false)}
        >
          <X className={SMALL_ICON_CLASS} aria-hidden="true" />
        </button>
      </div>
    );
  }

  return (
    <div className={ENTRY_ROW_CLASS} data-testid={`canvas-scratchpad-entry-${entry.id}`}>
      <input
        type="text"
        data-testid={`canvas-scratchpad-name-${entry.id}`}
        aria-label={t('dashboard.canvas.edit.scratchpadName')}
        placeholder={autoName}
        value={entry.name}
        className={NAME_INPUT_CLASS}
        onChange={(event) => onRename(entry.id, event.target.value)}
      />
      {/* 놓기 — **한 단추가 두 몸짓을 낸다**(REQ-04).

          누른 자리에서 그대로 떼면 계단 자리(`seedOffset`)에 놓이고, 캔버스 위로 끌어다
          떼면 **그 자리**에 놓인다. 둘을 다른 컨트롤로 가르면 좁은 도크에 줄이 하나 더
          생기고, 무엇보다 **끌지 않는 길이 눈에 보이지 않는 곳**에 남는다 —
          WCAG 2.2 SC 2.5.7 이 요구하는 것은 등가물이 **있는** 것이지 숨어 있는 것이 아니다.

          자리 판정은 오버레이가 한다. 여기서는 클라이언트 좌표를 그대로 올려 보낼 뿐이며,
          캔버스 밖에서 떼면 오버레이가 거짓을 돌려주어 뒤따르는 누름이 계단 자리에 놓는다. */}
      <button
        type="button"
        data-testid={`canvas-scratchpad-place-${entry.id}`}
        aria-label={t('dashboard.canvas.edit.scratchpadPlace')}
        title={t('dashboard.canvas.edit.scratchpadPlace')}
        className={ICON_BUTTON_CLASS}
        onPointerDown={(event) => {
          placedByDrag.current = false;
          // 잡아 두지 않으면 캔버스 위에서 뗀 사건이 오버레이로 가 버려 이 단추가 제
          // 몸짓의 끝을 보지 못한다. jsdom 에는 없는 API 라 존재를 확인하고 부른다.
          event.currentTarget.setPointerCapture?.(event.pointerId);
        }}
        onPointerUp={(event) => {
          placedByDrag.current = onPlace(entry, {
            clientX: event.clientX,
            clientY: event.clientY,
          });
        }}
        onClick={() => {
          // 끌어 놓기로 이미 놓았으면 그 뒤의 누름은 같은 몸짓의 꼬리다 — 한 번 삼킨다.
          if (placedByDrag.current) {
            placedByDrag.current = false;
            return;
          }
          onPlace(entry, null);
        }}
      >
        <StickyNote className={SMALL_ICON_CLASS} aria-hidden="true" />
      </button>
      <button
        type="button"
        data-testid={`canvas-scratchpad-remove-${entry.id}`}
        aria-label={t('dashboard.canvas.edit.scratchpadRemove')}
        title={t('dashboard.canvas.edit.scratchpadRemove')}
        className={ICON_BUTTON_CLASS}
        onClick={() => setConfirming(true)}
      >
        <Trash2 className={SMALL_ICON_CLASS} aria-hidden="true" />
      </button>
    </div>
  );
}

// --- 절 ------------------------------------------------------------------

export interface CanvasScratchpadProps {
  /** 지금 고른 것이 있는가 — 없으면 저장 단추를 끈다(REQ-03). */
  canSave: boolean;
  /** 끌기와 **같은 저장 함수**를 부른다(J11). 오버레이가 그 한 함수를 소유한다. */
  onSave: () => void;
  /** 끌던 손이 지금 드롭 존 위에 있는가. 판정은 오버레이가 하고 강조는 여기가 입는다. */
  dropActive: boolean;
  /**
   * 항목을 캔버스에 놓는다. `at` 이 `null` 이면 계단 자리(`seedOffset`)다.
   *
   * **놓았는가를 돌려준다.** 캔버스 밖에서 뗀 몸짓은 놓기가 아니며, 그 사실을 아는 것은
   * 오버레이(제 상자를 든 쪽)뿐이다 — 서랍은 그 답으로 뒤따르는 누름을 삼킬지 정한다.
   */
  onPlace: (entry: ScratchpadEntry, at: ScratchpadDropPoint | null) => boolean;
  /** 묶음 제목을 이을 id(도크의 다른 절과 같은 배선). */
  titleId: string;
}

export function CanvasScratchpad({
  canSave,
  onSave,
  dropActive,
  onPlace,
  titleId,
}: CanvasScratchpadProps): React.ReactElement {
  const { t } = useTranslation();
  const dropRef = useScratchpadDropRef();
  // 액션만 고른다 — 항목 배열을 구독하면 이 절이 아니라 **도크 전체**가 항목마다 다시
  // 그려진다. 액션 참조는 zustand 에서 고정이다.
  const entries = useScratchpadStore((s) => s.entries);
  const renameEntry = useScratchpadStore((s) => s.renameEntry);
  const removeEntry = useScratchpadStore((s) => s.removeEntry);
  const notice = useScratchpadStore((s) => s.notice);
  const clearNotice = useScratchpadStore((s) => s.clearNotice);

  return (
    <section role="group" aria-labelledby={titleId} className="flex flex-col gap-1">
      {/* 제목과 그 뒤의 `?`. 줄로 깔려 있던 두 안내가 여기로 들어왔다 — 빈 서랍이 시키던
          **할 일**과 기기 지역 저장 **고지**다(REQ-06). 둘 다 늘 읽을 글이 아니라 물어볼
          때 읽는 글이고, 서랍은 도크에서 가장 키가 큰 절이라 줄이 둘 줄면 목록이 그만큼
          접히지 않고 선다.

          **한 제목에 물음표는 하나다.** 두 문장은 빈 줄로 나누어 한 팝오버에 담는다 —
          `FieldHelp` 가 `whitespace-pre-wrap` 이라 그 빈 줄이 단락 사이가 된다
          (`components/property/PropertyPanel.tsx` 가 포트 설명 둘을 합치는 그 방식).

          **`?` 는 제목 `<p>` 의 형제다.** 안에 넣으면 `aria-labelledby` 가 가리키는 글자에
          도움말 본문(sr-only)이 섞여 절 이름이 문단만큼 길어진다. */}
      <div className="flex items-center gap-1 px-1 pb-1">
        <p id={titleId} className="text-[11px] font-medium text-(--color-text-muted)">
          {t('dashboard.canvas.edit.dockScratchpad')}
        </p>
        <FieldHelp
          text={
            `${t('dashboard.canvas.edit.scratchpadEmptyHint')}\n\n` +
            t('dashboard.canvas.edit.scratchpadLocalOnly')
          }
          testId="canvas-scratchpad-local-hint"
        />
      </div>

      {/* 드롭 존 — 노드만 내놓는다. 사건은 오버레이가 받는다(위 머리말). */}
      <div
        ref={dropRef ?? undefined}
        data-testid="canvas-scratchpad-dropzone"
        data-active={dropActive ? 'true' : 'false'}
        className={cn(DROP_ZONE_CLASS, dropActive ? DROP_ACTIVE_CLASS : DROP_IDLE_CLASS)}
      >
        {t('dashboard.canvas.edit.scratchpadDropHint')}
      </div>

      {/* 끌지 않고도 되는 길 — 편의가 아니라 요구다(WCAG 2.2 SC 2.5.7 · REQ-03). */}
      <button
        type="button"
        data-testid="canvas-scratchpad-save"
        aria-label={t('dashboard.canvas.edit.scratchpadSave')}
        title={t('dashboard.canvas.edit.scratchpadSave')}
        disabled={!canSave}
        className={ROW_BUTTON_CLASS}
        onClick={onSave}
      >
        <Save className={ICON_CLASS} aria-hidden="true" />
        <span>{t('dashboard.canvas.edit.scratchpadSave')}</span>
      </button>

      {notice !== null && (
        <div
          data-testid="canvas-scratchpad-notice"
          role="status"
          className="flex items-start gap-1 rounded bg-(--color-bg-elevated) px-1.5 py-1"
        >
          <span className="min-w-0 flex-1 text-[11px] leading-tight text-(--color-text-secondary)">
            {noticeText(t(NOTICE_KEYS[notice.status]))}
          </span>
          <button
            type="button"
            data-testid="canvas-scratchpad-notice-dismiss"
            aria-label={t('dashboard.canvas.edit.scratchpadNoticeDismiss')}
            title={t('dashboard.canvas.edit.scratchpadNoticeDismiss')}
            className={cn(ICON_BUTTON_CLASS, 'h-4 w-4')}
            onClick={clearNotice}
          >
            <X className="h-3 w-3 shrink-0" aria-hidden="true" />
          </button>
        </div>
      )}

      {entries.length === 0 ? (
        // 항목이 없으면 **없다는 사실 하나**다(REQ-05). 캔버스 표면에는 아무것도 더하지
        // 않는다. 뒤에 붙어 있던 "어떻게 채우는가" 는 제목 뒤 `?` 로 옮겼다 — 빈 자리의
        // 일은 빈 곳을 메우는 것이므로 **사실은 남고 지시만 간다**.
        <p data-testid="canvas-scratchpad-empty" className={HINT_CLASS}>
          <Inbox className="mr-1 inline h-3 w-3" aria-hidden="true" />
          {t('dashboard.canvas.edit.scratchpadEmpty')}
        </p>
      ) : (
        <div data-testid="canvas-scratchpad-list" className="flex flex-col">
          {entries.map((entry) => (
            <ScratchpadEntryRow
              key={entry.id}
              entry={entry}
              onRename={renameEntry}
              onRemove={removeEntry}
              onPlace={onPlace}
            />
          ))}
        </div>
      )}

    </section>
  );
}
