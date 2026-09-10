// 보기 배율 칸 — **두 표면이 함께 그리는 그 한 칸**(SPEC-CANVAS-006 M10 · REQ-10 · 불변식 I24).
//
// M9 는 이 칸을 `CanvasEditDock.tsx` 안에 직접 적었다. 도크는 설정 다이얼로그에서만 펴지는
// 자리(`PanelSettingsDialog` 의 `CanvasEditDockRegion`)이므로, 대시보드에서 제자리 편집하는
// 사람에게는 그 칸이 화면에 **한 글자도 나타나지 않았다** — 줄어든 출력 영역과 저술 여백과
// 경계를 보면서 그것을 다스릴 손잡이가 없었다(불변식 I23 이 이름 붙인 그 결함).
//
// M10 이 그 자리를 메우면서 칸을 **여기로 옮긴다.** 옮김이지 다시 씀이 아니다 — 도크의
// 그림도 표적 이름도 한 글자도 달라지지 않는다.
//
// ## 이 파일이 혼자 소유하는 것
//
// 범위 둘(`MIN`/`MAX_WORKSPACE_ZOOM`) · `min`/`max`/`step` · `aria-label` 과 `title` ·
// `<datalist>` 제안 넷 · `aria-describedby` 배선 · **백분율과 분수를 오가는 환산 두 방향**.
// 두 표면이 각자 적으면 그 여섯이 두 벌이 되고, 이 SPEC 의 역사가 그 결말을 두 번 보였다
// (0.11.0 의 격자 간격 목록 — 같은 값의 두 어휘가 갈라졌다, 0.4.0 의 폭·높이 칸 — 적는 즉시
// 덮이는 칸이 남았다).
//
// **환산이 나타나도 되는 파일은 `canvasWorkspace.ts` 와 이 파일 둘뿐이다.** 그 가드는 M9 가
// 도크에 세운 것이며 칸을 따라 **여기로 옮겨 왔다**(위험 R27 — 파일 이름에 매인 가드는
// 옮김 한 번에 조용히 무장 해제된다).
//
// ## 도움말만 슬롯인 이유
//
// 도크는 `FieldHelp` 의 클릭 팝오버를 꽂고, 줄은 `sr-only` 문단을 꽂는다. 그 팝오버는
// `absolute left-0 top-full w-64` 로 **아래·오른쪽**에 열리는데 줄은 작업 영역 왼쪽 아래
// 모서리에 살고 표면 컨테이너에는 `overflow-hidden` 이 있어 **잘린다** — 열어도 보이지 않는
// `?` 는 화면이 지키지 못할 약속이다. `FieldHelp` 에 방향 prop 을 더하는 안은 기각했다:
// 다섯 패널이 함께 쓰는 컴포넌트를 이 SPEC 한 자리의 배치 문제 때문에 넓히는 일이다
// (불변식 I7 과 같은 부류의 금지).
//
// **그럼에도 배선은 여기가 소유한다.** 슬롯은 id 를 **받아** 제 문구에 달 뿐이고, 그 id 를
// 만들어 칸의 `aria-describedby` 에 잇는 것은 이 파일이다 — 그래야 한쪽 표면에서 배선이
// 빠지는 일이 형상 자체로 없다.
//
// @spec SPEC-CANVAS-006 REQ-09 REQ-10

import { useId, type ReactNode } from 'react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import {
  MAX_WORKSPACE_ZOOM,
  MIN_WORKSPACE_ZOOM,
  WORKSPACE_ZOOM_CHOICES,
  clampWorkspaceZoom,
  workspaceZoomPercent,
} from './canvasWorkspace';

export interface CanvasWorkspaceZoomFieldProps {
  /** 보기 배율(**분수**). 화면은 백분율 정수로 말하고, 그 환산은 이 파일이 진다. */
  zoom: number;
  /** 바뀐 배율(**분수**)을 돌려준다. 되죄지 않는다 — 죄는 것은 범위 하나뿐이다. */
  onZoomChange: (next: number) => void;
  /**
   * 입력 칸에 얹을 클래스. **표면마다 다르다** — 도크는 제 칸들과 같은 폭·같은 글자
   * 크기를 쓰고, 줄은 떠 있는 컨트롤의 좁은 눈금을 쓴다. 겉모습만 표면의 몫이고 계약은
   * 전부 이 파일의 것이다.
   */
  className?: string;
  /**
   * 도움말 슬롯. **id 를 받아** 제 문구에 달아 준다 — 그 id 를 만들고 칸에 잇는 것은 이
   * 컴포넌트다(위 §도움말만 슬롯인 이유).
   */
  renderHelp: (describedById: string) => ReactNode;
}

/**
 * 백분율 정수 자유 입력 한 칸 + 제안 넷.
 *
 * 목록(콤보 상자)이 아닌 이유는 격자 간격이 0.11.0 에서 이미 걸어 본 길이다: "쓸모 있는
 * 값" 은 패널 크기와 그림에 따라 달라 목록이 알 수 없고, 그래서 목록은 지키지 못할 약속을
 * 하면서 고를 자유만 빼앗는다. 슬라이더도 기각한다 — 그것은 **연속 응답을 약속하는
 * 겉모습**인데 격자가 그것을 `1/step` 눈금으로 자르므로, 끄는 동안 여러 구간에서 화면이
 * 멈춘 것처럼 보인다(위험 R24 · 가정 A20).
 *
 * **지역 상태를 두지 않는다.** 한 글자마다 죄되, 읽을 수 없는 입력(빈 칸 · 글자)에는
 * `clampWorkspaceZoom` 이 지금 값을 그대로 돌려주므로 지우는 도중에 화면이 무너지지 않는다.
 */
export function CanvasWorkspaceZoomField({
  zoom,
  onZoomChange,
  className,
  renderHelp,
}: CanvasWorkspaceZoomFieldProps): React.ReactElement {
  const { t } = useTranslation();
  // 한 화면에 캔버스 패널이 둘 이상 뜰 수 있으므로 고정 id 를 쓸 수 없다 — id 가 겹치면
  // `aria-describedby` 가 남의 설명을 가리키고 `list` 가 남의 제안을 편다.
  const suggestId = useId();
  const hintId = useId();

  return (
    <>
      <div className="flex items-center gap-1">
        <input
          type="number"
          inputMode="numeric"
          data-testid="canvas-workspace-zoom"
          aria-label={t('dashboard.canvas.edit.workspaceZoom')}
          title={t('dashboard.canvas.edit.workspaceZoom')}
          // 상시 도움말이므로 이 연결도 상시다 — 조건부 고지는 간격 25 에서 사실상 늘
          // 뜨는 경고가 되어 위험 R18 이 이름 붙인 실패를 되풀이한다.
          aria-describedby={hintId}
          list={suggestId}
          min={workspaceZoomPercent(MIN_WORKSPACE_ZOOM)}
          max={workspaceZoomPercent(MAX_WORKSPACE_ZOOM)}
          step={1}
          value={workspaceZoomPercent(zoom)}
          onChange={(event) => onZoomChange(clampWorkspaceZoom(event.target.value, zoom))}
          className={cn(className)}
        />
        {renderHelp(hintId)}
      </div>
      {/* 제안값 — 고를 수 있는 값의 전부가 아니라 **곁들이**다(격자 간격과 같은 규율). */}
      <datalist id={suggestId} data-testid="canvas-workspace-zoom-suggestions">
        {WORKSPACE_ZOOM_CHOICES.map((choice) => (
          // 옵션 글자도 번역한다 — 벌거벗은 숫자는 그것이 백분율인지 캔버스 단위인지
          // 말하지 않으며, 도크에서는 바로 한 줄 아래 칸이 캔버스 단위를 받는다.
          //
          // 치환은 **`replaceAll`** 이다(SPEC-CANVAS-006 M11). `replace` 는 같은 치환자가
          // 둘인 문구에서 **첫 하나만** 바꾸고 나머지를 화면에 `{percent}` 로 남긴다 —
          // 이 저장소는 그 형상을 이미 갖고 있다(`gridStepPartial` 의 `{step}` 이 둘이고,
          // 도크가 `replaceAll` 을 쓰는 이유가 그것이다). 오늘 이 문구의 치환자는 하나라
          // 두 함수의 결과가 같지만, **번역을 손보다 치환자를 하나 더 넣는 일**이 그
          // 침묵을 깨는 순간이며 그때 고쳐야 할 자리가 여기라는 것을 이 한 낱말이 없애
          // 준다. `M11` 의 형상 가드가 이 낱말을 지킨다.
          <option key={choice} value={workspaceZoomPercent(choice)}>
            {t('dashboard.canvas.edit.workspaceZoomOption').replaceAll(
              '{percent}',
              String(workspaceZoomPercent(choice)),
            )}
          </option>
        ))}
      </datalist>
    </>
  );
}
