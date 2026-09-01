// 임계값 범례의 **문구 조립** — 순수 모듈.
//
// 게이지는 임계값 구간을 색으로만 보여 준다. 색이 무엇을 뜻하는지는 설정 화면을 열어야
// 알 수 있고, 대시보드를 보는 사람은 설정을 열지 않는다. 범례는 그 색의 뜻을 화면에
// 남기는 자리다.

import { formatTickValue } from '../charts/unitOptions';
import type { ThresholdEntry } from './gaugeShapes';

/** 범례 한 줄. */
export interface ThresholdLegendItem {
  color: string;
  label: string;
}

/**
 * 구간 하나의 표시 문구.
 *
 * 이름이 있으면 이름을 쓴다 — 사용자가 붙인 이름이 곧 그 색의 뜻이다. 이름이 비어
 * 있으면(기본 3구간의 기본값) 범위를 적는다. 범위마저 없으면 색만 남는 빈 줄이 되므로
 * 빈 문자열을 돌려주고, 호출부가 그 줄을 버린다.
 *
 * 값 표기는 게이지 눈금과 **같은 규칙**을 쓴다 — 같은 수가 눈금과 범례에서 다르게
 * 적히면 둘이 다른 값처럼 읽힌다.
 */
export function thresholdLegendLabel(
  entry: ThresholdEntry,
  unit: string | undefined,
  decimals?: number,
): string {
  const name = entry.name?.trim() ?? '';
  if (name !== '') return name;
  if (!Number.isFinite(entry.from) || !Number.isFinite(entry.to)) return '';
  // 두 끝의 자릿수를 **맞춘다**. 눈금 규칙은 값마다 자릿수를 따로 고르므로 그대로 쓰면
  // 한 줄 안에서 `0.00~60` 처럼 갈린다 — 같은 축의 두 수가 다른 정밀도로 적히면 범위가
  // 아니라 서로 다른 두 값처럼 읽힌다. 큰 쪽(끝값)의 자릿수를 둘 다에 쓴다.
  const dec = decimals ?? decimalsOf(formatTickValue(entry.to, unit));
  return `${formatTickValue(entry.from, unit, dec)}~${formatTickValue(entry.to, unit, dec)}`;
}

/** 이미 포맷된 수 문자열의 소수 자릿수. 소수점이 없으면 0. */
function decimalsOf(text: string): number {
  const dot = text.indexOf('.');
  return dot < 0 ? 0 : text.length - dot - 1;
}

/**
 * 구간 목록을 범례 줄로 바꾼다. 문구가 빈 줄은 버린다 — 색만 있는 줄은 아무것도
 * 알려주지 않으면서 자리만 차지한다.
 */
export function thresholdLegendItems(
  thresholds: readonly ThresholdEntry[],
  unit: string | undefined,
  decimals?: number,
): ThresholdLegendItem[] {
  return thresholds
    .map((t) => ({ color: t.color, label: thresholdLegendLabel(t, unit, decimals) }))
    .filter((i) => i.label !== '');
}
