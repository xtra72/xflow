// 게이지 임계값 범례.
//
// **패널에 하나만** 그린다. 임계값은 패널 설정이라 모든 타일이 같은 구간을 쓰므로,
// 타일마다 붙이면 같은 문구가 시리즈 수만큼 반복되면서 정작 게이지가 들어갈 자리를
// 잡아먹는다.
//
// 게이지 **위에 겹쳐** 뜬다(절대 배치). 흐름에 끼워 넣으면 범례가 자리를 나눠 가져
// 게이지가 그만큼 작아지고, 끌어 옮긴 오프셋과 레이아웃이 서로를 밀어낸다 — 파이 범례가
// 같은 이유로 오버레이가 됐다. 겹치는 자리에서는 범례가 위에 보인다.
//
// 글꼴·크기·색과 자리 계산은 파이 범례와 같은 어휘를 쓴다(`textStyle` · `legendOverlay`)
// — 두 패널의 같은 설정이 다른 모양이면 두 번 익혀야 한다.

import { cn } from '@/lib/utils/cn';

import { resolveFontFamily } from '../charts/textStyle';
import type { ChartFontFamily } from '../charts/textStyle';
import { legendOverlayStyle } from '../charts/legendOverlay';
import type { ThresholdLegendItem } from './thresholdLegend';

/** 범례를 게이지 위에 둘지 아래에 둘지(기준 자리). 끌어서 미세 조정한다. */
export type ThresholdLegendPosition = 'top' | 'bottom';

/** 항목을 한 줄로 늘어놓을지, 한 칸씩 쌓을지. */
export type ThresholdLegendOrientation = 'horizontal' | 'vertical';

export function GaugeThresholdLegend({
  items,
  position,
  orientation,
  fontFamily,
  fontSize,
  fontColor,
  offsetX,
  offsetY,
  edit,
}: {
  items: readonly ThresholdLegendItem[];
  position: ThresholdLegendPosition;
  orientation: ThresholdLegendOrientation;
  fontFamily?: ChartFontFamily;
  /** 미지정은 상속(11px 기본 클래스). */
  fontSize?: number;
  /** 미지정은 테마 흐린 글자색. */
  fontColor?: string;
  /** 끌어 옮긴 오프셋(담는 상자 대비 %). 기준 자리에서 얼마나 밀렸는지. */
  offsetX: number;
  offsetY: number;
  /**
   * 편집 표식 — 정렬이 상자를 찾고, 선택 윤곽을 입는다.
   *
   * 감싸는 상자를 만들지 않는다. 이 범례는 스스로 `absolute` 로 떠 있어서, 흐름 안의
   * 상자로 감싸면 그 상자가 크기 0이 되고 윤곽이 엉뚱한 자리에 생긴다(파이 범례에서
   * 실제로 그렇게 만들었다가 위쪽에 얇은 띠만 남았다).
   */
  /** 편집 표면 — 표식·윤곽선·손잡이는 범례 **자신**에 붙어야 한다. */
  edit?: { props: Record<string, unknown>; outline: string; handle?: React.ReactNode };
}): React.ReactElement | null {
  if (items.length === 0) return null;
  const vertical = orientation === 'vertical';
  return (
    <div
      data-testid="gauge-threshold-legend"
      data-gauge-threshold-legend=""
      {...(edit?.props ?? {})}
      data-position={position}
      data-orientation={orientation}
      className={cn(
        // 게이지 위에 뜬다. 겹치는 자리에서 글자를 읽으려면 게이지 색이 비쳐서는 안
        // 되므로 옅은 패널색 판을 깔아 준다(파이 범례와 같은 규칙).
        'absolute z-10 flex max-h-full max-w-full overflow-auto rounded-md bg-(--color-bg-surface)/80 px-2 py-1',
        edit?.outline,
        vertical
          ? 'flex-col items-start gap-y-1'
          : 'flex-wrap items-center justify-center gap-x-3 gap-y-1',
        fontSize === undefined && 'text-[11px]',
        fontColor ? undefined : 'text-(--color-text-muted)',
      )}
      style={{
        ...legendOverlayStyle(position, offsetX, offsetY),
        fontFamily: resolveFontFamily(fontFamily),
        fontSize: fontSize === undefined ? undefined : `${fontSize}px`,
        color: fontColor,
      }}
    >
      {edit?.handle}
      {items.map((item) => (
        <span
          key={`${item.color}:${item.label}`}
          data-testid="gauge-threshold-legend-item"
          className="inline-flex items-center gap-1 whitespace-nowrap"
        >
          <span
            className="inline-block h-2 w-2 shrink-0 rounded-full"
            style={{ backgroundColor: item.color }}
          />
          {item.label}
        </span>
      ))}
    </div>
  );
}
