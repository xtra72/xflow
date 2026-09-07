// 편집 중 패널 아래에 떠 있는 **정렬 툴바**. 통계·바·파이가 함께 쓴다.
//
// 자리를 아래로 잡은 이유: 위쪽은 이미 배치 편집 토글(좌상단)과 연결 상태 아이콘
// (우상단)이 쓰고 있고, 통계 패널의 값은 세로 가운데에 모여 있어 아래 가장자리가
// 가장 덜 방해된다.
//
// 정렬은 **요소끼리** 맞춘다 — 고른 요소들이 이루는 바깥 상자에 맞추는 것이며,
// 계산은 `statAlign.computeAlignPatches` 가 소유한다(spec.md §5 D9).
//
// 세로 정렬은 세로로 쌓인 세 요소를 겹치게 만든다. 그것이 그 조작의 뜻이므로 막지
// 않되, 되돌릴 수단(배치 초기화)을 같은 줄에 둔다 — 되돌릴 수 없는 조작만 남기면
// 사용자가 눌러 보지 못한다.
//
// @spec SPEC-CHART-004 §2.8 [U8]

import {
  AlignCenterHorizontal,
  AlignCenterVertical,
  AlignEndHorizontal,
  AlignEndVertical,
  AlignStartHorizontal,
  AlignStartVertical,
  Magnet,
  RotateCcw,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import type { AlignAxis, AlignMode } from './panels/charts/panelEditAlign';

/**
 * 단추 6개의 정의.
 *
 * lucide 의 `AlignStartVertical` 은 **세로 축을 따라 늘어선 것들을 왼쪽에 맞춘다**는
 * 뜻이라 우리의 가로 정렬에 해당한다 — 이름과 축이 반대로 읽히므로 여기서 한 번만
 * 짝지어 두고, 쓰는 쪽은 `axis`/`mode` 만 본다.
 */
// 키 이름이 `alignElem*` 인 이유: `dashboard.chart.alignLeft` 는 **글자 정렬**
// (`TEXT_ALIGN_OPTIONS`)이 이미 쓰는 이름이다. 뜻이 다른 두 조작이 한 낱말을 나눠 쓰면
// 한쪽 문구를 고칠 때 다른 쪽이 조용히 바뀐다.
const BUTTONS: ReadonlyArray<{
  axis: AlignAxis;
  mode: AlignMode;
  Icon: typeof AlignStartVertical;
  labelKey: string;
}> = [
  { axis: 'horizontal', mode: 'start', Icon: AlignStartVertical, labelKey: 'dashboard.chart.alignElemLeft' },
  { axis: 'horizontal', mode: 'center', Icon: AlignCenterVertical, labelKey: 'dashboard.chart.alignElemCenterX' },
  { axis: 'horizontal', mode: 'end', Icon: AlignEndVertical, labelKey: 'dashboard.chart.alignElemRight' },
  { axis: 'vertical', mode: 'start', Icon: AlignStartHorizontal, labelKey: 'dashboard.chart.alignElemTop' },
  { axis: 'vertical', mode: 'center', Icon: AlignCenterHorizontal, labelKey: 'dashboard.chart.alignElemMiddle' },
  { axis: 'vertical', mode: 'end', Icon: AlignEndHorizontal, labelKey: 'dashboard.chart.alignElemBottom' },
];

export function PanelAlignToolbar({
  enabled,
  snap,
  onSnapChange,
  onAlign,
  onReset,
}: {
  enabled: boolean;
  /**
   * 격자에 붙이며 옮길지. Alt 임시 해제와는 별개인 **지속** 설정이다.
   *
   * **미지정이면 토글을 내지 않는다.** 스냅은 드래그 레이어가 하는 일인데, 자기 좌표
   * 규칙을 가진 레이어를 그대로 쓰는 패널(라인·게이지)은 아직 붙이지 못한다. 없는
   * 기능의 스위치를 두면 눌러도 아무 일도 없는 죽은 컨트롤이 된다.
   */
  snap?: boolean;
  onSnapChange?: (next: boolean) => void;
  onAlign: (axis: AlignAxis, mode: AlignMode) => void;
  /** 세 요소의 배치를 흐름상 시작 자리로 되돌린다. */
  onReset: () => void;
}): React.ReactElement | null {
  const { t } = useTranslation();
  if (!enabled) return null;
  return (
    <div
      data-testid="panel-align-toolbar"
      role="toolbar"
      aria-label={t('dashboard.chart.alignToolbar')}
      className="absolute bottom-2 left-1/2 z-30 flex -translate-x-1/2 items-center gap-0.5 rounded-lg bg-(--color-bg-elevated) p-1 shadow ring-1 ring-(--color-border-default)"
    >
      {BUTTONS.map(({ axis, mode, Icon, labelKey }) => (
        <button
          key={`${axis}-${mode}`}
          type="button"
          data-testid={`panel-align-${axis}-${mode}`}
          aria-label={t(labelKey)}
          title={t(labelKey)}
          onClick={() => onAlign(axis, mode)}
          className="rounded p-1 text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-surface) hover:text-(--color-text-primary)"
        >
          <Icon className="h-3.5 w-3.5" />
        </button>
      ))}
      <span className="mx-0.5 h-4 w-px bg-(--color-border-default)" />
      {snap !== undefined && onSnapChange !== undefined && (
      <button
        type="button"
        data-testid="panel-snap-toggle"
        aria-pressed={snap}
        aria-label={t('dashboard.chart.snapToGrid')}
        title={t('dashboard.chart.snapToGrid')}
        onClick={() => onSnapChange(!snap)}
        className={
          snap
            ? 'rounded bg-blue-500/15 p-1 text-blue-600 transition-colors dark:text-blue-300'
            : 'rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-surface) hover:text-(--color-text-primary)'
        }
      >
        <Magnet className="h-3.5 w-3.5" />
      </button>
      )}
      <button
        type="button"
        data-testid="panel-align-reset"
        aria-label={t('dashboard.chart.alignReset')}
        title={t('dashboard.chart.alignReset')}
        onClick={onReset}
        className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-surface) hover:text-(--color-text-primary)"
      >
        <RotateCcw className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
