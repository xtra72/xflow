// 편집 모드 드래그 핸들(제어 바).
//
// 패널 위쪽에 겹쳐 띄운다(absolute). 흐름(flow) 요소로 두면 편집 모드에 들어가는 순간 핸들
// 높이(h-6)만큼 패널 본문이 줄어들어 **보이는 그림이 달라진다** — 히트맵 레터박스가 바뀌고,
// 차트 축과 그리드 열 수가 재계산되며, 편집을 끝내면 다시 원래대로 돌아온다. 편집 모드는
// 배치를 확인하는 화면이므로 본문 크기가 흔들리면 안 된다.
//
// GridLayout 의 dragConfig.handle 셀렉터가 `.dashboard-drag-handle` 이므로 클래스명은 유지한다
// (absolute 여도 드래그 대상 판정에는 영향이 없다).

/** 편집 모드 드래그 핸들 — 패널 상단에 겹치는 오버레이(레이아웃 비침습). */
export default function DragHandle() {
  return (
    <div
      data-testid="dashboard-drag-handle"
      className="dashboard-drag-handle absolute inset-x-0 top-0 z-10 flex h-6 cursor-grab items-center justify-center rounded-t-lg bg-(--color-bg-elevated)/80 active:cursor-grabbing"
    >
      <div className="flex gap-1">
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
      </div>
    </div>
  );
}
