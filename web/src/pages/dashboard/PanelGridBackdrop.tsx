// 미리보기 뒤에 까는 그리드 가이드 라인.
//
// 대시보드와 같은 배치다 — 가이드는 패널 **뒤**에 있고, 패널은 그 위에 불투명한
// 카드로 얹힌다. 가이드를 패널 위에 그리면 패널의 실제 모양(카드 배경·테두리)이
// 가려져, 미리보기를 봐도 대시보드에서 어떻게 보일지 알 수 없다.
//
// 그래서 가이드는 패널 상자 안이 아니라 **미리보기 영역 전체**에 깔고, 칸 경계를
// 패널 상자에 맞춘다. 패널이 가린 부분은 주변 칸으로 이어 읽을 수 있다.

interface PanelGridBackdropProps {
  /** 미리보기 영역의 실측 크기(px) */
  areaW: number;
  areaH: number;
  /** 패널 상자의 화면 크기(px) — 칸 경계를 이 상자에 맞춘다 */
  boxW: number;
  boxH: number;
  /** 화면 기준 셀 크기와 간격(px) */
  cellW: number;
  cellH: number;
  gapX: number;
  gapY: number;
  /**
   * 패널 상자가 가운데에서 밀려난 화면 px(미리보기 팬 — `previewPan.ts`).
   *
   * 칸 경계는 **패널 상자**에 맞춘다. 그래서 그 상자가 끌려가면 격자도 같은 만큼 따라가야
   * 한다 — 따라가지 않으면 패널 모서리와 칸 경계가 어긋나, 격자가 "여기가 칸 경계다" 라고
   * 거짓말을 한다. 기본 0 이라 팬을 쓰지 않는 자리는 종전과 한 글자도 다르지 않다.
   */
  offsetX?: number;
  offsetY?: number;
}

/**
 * 첫 칸의 시작 좌표를 구한다.
 *
 * 패널 상자는 영역 가운데에 놓이므로, 그 왼쪽 모서리에서 칸 간격만큼 거슬러 올라가
 * 영역 밖까지 확장한다. 그래야 패널 주변의 칸이 패널의 칸과 같은 격자에 놓인다.
 */
function gridStart(areaLen: number, boxLen: number, pitch: number, offset: number): number {
  const boxStart = (areaLen - boxLen) / 2 + offset;
  if (!(pitch > 0)) return boxStart;
  return boxStart - Math.ceil(Math.max(0, boxStart) / pitch) * pitch;
}

/** 시작 좌표에서 영역 끝까지 덮는 칸 수. */
function gridCount(areaLen: number, start: number, pitch: number): number {
  if (!(pitch > 0)) return 0;
  return Math.max(1, Math.ceil((areaLen - start) / pitch));
}

export function PanelGridBackdrop({
  areaW,
  areaH,
  boxW,
  boxH,
  cellW,
  cellH,
  gapX,
  gapY,
  offsetX = 0,
  offsetY = 0,
}: PanelGridBackdropProps) {
  const pitchX = cellW + gapX;
  const pitchY = cellH + gapY;
  if (!(pitchX > 0) || !(pitchY > 0)) return null;

  const left = gridStart(areaW, boxW, pitchX, offsetX);
  const top = gridStart(areaH, boxH, pitchY, offsetY);
  const cols = gridCount(areaW, left, pitchX);
  const rows = gridCount(areaH, top, pitchY);

  return (
    <div
      aria-hidden="true"
      data-testid="panel-grid-backdrop"
      // z-0: 패널(transform 으로 스택 컨텍스트를 만든다)보다 아래에 둔다.
      className="pointer-events-none absolute inset-0 z-0 overflow-hidden"
    >
      <div
        className="absolute"
        style={{
          left: `${left}px`,
          top: `${top}px`,
          display: 'grid',
          gridTemplateColumns: `repeat(${cols}, ${cellW}px)`,
          gridTemplateRows: `repeat(${rows}, ${cellH}px)`,
          gap: `${gapY}px ${gapX}px`,
        }}
      >
        {Array.from({ length: cols * rows }).map((_, i) => (
          <div key={i} className="border border-dashed border-(--color-border-default)" />
        ))}
      </div>
    </div>
  );
}
