// 패널 안에서 **배치 편집 모드**를 켜고 끄는 토글 — 히트맵이 먼저 쓰던 규칙을 공유한다.
//
// 게이팅이 세 겹이다. 셋 다 이유가 다르므로 하나로 줄일 수 없다.
//   1. `canEdit`  — config 를 쓸 콜백이 있는가. 없으면 끌어도 저장할 곳이 없다.
//   2. `editMode` — 대시보드 편집모드인가. gear/삭제 버튼과 같은 게이팅이라, 읽기 전용
//      뷰(원격 대시보드)에서는 자동으로 사라진다.
//   3. `forced`   — 설정 미리보기처럼 **항상** 편집인 자리인가. 그때는 토글을 감춘다.
//      끌 수 있다는 사실이 화면 맥락으로 이미 드러나 있고, 토글이 미리보기를 가린다.

import { useEffect, useState } from 'react';
import { Move } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';

export interface PanelEditModeState {
  /** 배치 오버레이(드래그)를 켤 것인가. */
  active: boolean;
  /** 토글 버튼. 노출 조건이 아니면 `null`. */
  toggle: React.ReactElement | null;
}

/**
 * 배치 편집 상태와 토글 버튼을 만든다.
 *
 * @param canEdit  config 를 쓸 콜백이 있는가.
 * @param forced   설정 미리보기처럼 항상 편집인가(토글 숨김).
 * @param testId   토글 버튼의 test id — 패널마다 다르다.
 * @param below    타이틀 바가 있어 버튼을 한 줄 내려야 하는가. 둘 다 좌상단을 쓴다.
 */
export function usePanelEditMode({
  canEdit,
  forced = false,
  testId,
  below,
}: {
  canEdit: boolean;
  forced?: boolean;
  testId: string;
  below: boolean;
}): PanelEditModeState {
  const { t } = useTranslation();
  // 런타임 상태다 — 저장하지 않는다. 편집 중이라는 사실이 config 에 남으면 다음에 열
  // 때도 오버레이가 떠 있고, 그것을 끄는 방법이 화면에 없다.
  const [editing, setEditing] = useState(false);
  const editMode = useUIStore((s) => s.dashboardEditMode);

  // 편집모드를 벗어나면 진행 중이던 배치편집을 강제 해제한다(오버레이 잔존 방지).
  useEffect(() => {
    if (!editMode) setEditing(false);
  }, [editMode]);

  const showToggle = canEdit && editMode && !forced;
  return {
    active: canEdit && (editing || forced),
    toggle: showToggle ? (
      <button
        type="button"
        data-testid={testId}
        aria-pressed={editing}
        onClick={() => setEditing((v) => !v)}
        className={cn(
          'absolute left-2 z-30 inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium shadow transition-colors',
          below ? 'top-9' : 'top-2',
          editing
            ? 'bg-blue-600 text-white dark:bg-blue-500'
            : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-surface)',
        )}
      >
        <Move className="h-3 w-3" />
        {editing ? t('dashboard.panel.editExit') : t('dashboard.panel.editEnter')}
      </button>
    ) : null,
  };
}
