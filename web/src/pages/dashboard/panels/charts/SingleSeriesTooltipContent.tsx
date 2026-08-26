// 단일 값 툴팁의 내용 컴포넌트.
//
// 판정(어느 라인인가)은 `tooltipSingle` 의 순수 함수가, 그리기는 recharts 기본
// 내용이 맡는다. 이 파일은 둘을 잇기만 한다 — recharts 훅에서 플롯 영역과 Y축
// 도메인을 읽어 판정에 넘기는 자리다.

import { DefaultTooltipContent, usePlotArea, useYAxisDomain } from 'recharts';
import type { TooltipContentProps } from 'recharts';
import type { NameType, ValueType } from 'recharts/types/component/DefaultTooltipContent';

import { cursorValueAt, pickNearestSeries } from './tooltipSingle';

/**
 * 단일 값 툴팁의 내용.
 *
 * 축 모드로 모인 payload 에서 **커서에 가장 가까운 라인 하나만** 남기고, 그리기
 * 자체는 recharts 기본 내용(DefaultTooltipContent)에 맡긴다. Tooltip 은 자기
 * 프롭 전부를 content 에 그대로 넘기므로(contentStyle · formatter · labelFormatter
 * 등) 여기서 다시 옮겨 적을 필요가 없다 — 옮겨 적으면 한쪽만 고쳐져 어긋난다.
 */
export function SingleSeriesTooltipContent(
  props: TooltipContentProps<ValueType, NameType>,
): React.ReactElement {
  const plot = usePlotArea();
  const yDomain = useYAxisDomain();
  const cursorValue = cursorValueAt(props.coordinate?.y, plot, yDomain);
  const payload = pickNearestSeries(props.payload, (p) => p.value, cursorValue);
  return <DefaultTooltipContent {...props} payload={[...payload]} />;
}

