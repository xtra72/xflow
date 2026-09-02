// 패널 크롬(공통 외곽 UI) 옵션 전파 — 컨텍스트 / config 파서 / 읽기 훅.
//
// 타이틀 바 표시 여부는 패널 28종 전부가 알아야 하는 값인데, 각 패널이 config 를 받는 방식이
// 제각각이고(`config` / `panelConfig` / 아예 안 받음) MODBUS 6종은 공용 프레임을 34곳에서
// 호출한다. prop 으로 꿰면 호출부마다 편집이 필요하지만, context 로 두면 헤더를 그리는 쪽에서
// 한 번만 읽으면 된다.
//
// Provider 가 없으면 표시(true)다 — 패널을 단독 렌더하는 기존 테스트/화면이 그대로 동작한다.
// Provider 컴포넌트는 PanelChromeProvider.tsx 에 있다(Fast Refresh 를 위한 분리).

import { createContext, useContext } from 'react';
import type { CSSProperties } from 'react';

import {
  resolveFontColor,
  resolveFontFamily,
  resolveFontSize,
  type ChartFontFamily,
} from './panels/charts/textStyle';

/**
 * 타이틀 글자 모양. 패널 28종이 공유하는 값이라 크롬 옵션으로 둔다 — 패널마다 따로 두면
 * 같은 설정이 패널마다 다른 이름·다른 자리에 생긴다.
 *
 * 어휘는 차트 글자 스타일(`textStyle.ts`)과 **같다**. 파이 범례·축 글꼴을 이미 그 토큰으로
 * 고르고 있으므로, 타이틀만 다른 어휘를 쓰면 사용자가 두 번 배워야 한다.
 */
export interface PanelTitleFont {
  family?: ChartFontFamily;
  /** 글자 크기(px). */
  size?: number;
  /** 글자색(hex). */
  color?: string;
  weight?: 'normal' | 'bold';
}

export interface PanelChrome {
  /** 타이틀 바를 그릴지 여부. */
  showTitle: boolean;
  /**
   * 타이틀에 얹을 인라인 스타일. **미설정이면 `undefined`** 다.
   *
   * 빈 객체가 아니라 `undefined` 인 것이 중요하다 — 각 패널의 타이틀은 Tailwind 클래스로
   * 크기·굵기·색을 이미 정해 두었고, 인라인 스타일은 그것을 이긴다. 설정하지 않은 값까지
   * 채워 넣으면 저장된 대시보드의 타이틀이 조용히 바뀐다.
   */
  titleStyle?: CSSProperties;
}

const DEFAULT_CHROME: PanelChrome = { showTitle: true };

/**
 * 타이틀 글자 설정을 인라인 스타일로 편다. 지정한 항목만 넣는다.
 *
 * `undefined` 값을 가진 키조차 넣지 않는다 — `{ color: undefined }` 를 뒤에 펼치면 앞의
 * 색을 지우기 때문이다(패널 자체 강조색과 겹치는 자리가 실제로 있다).
 */
export function resolvePanelTitleStyle(v: unknown): CSSProperties | undefined {
  const font = (v ?? {}) as PanelTitleFont;
  const style: CSSProperties = {};
  const family = resolveFontFamily(font.family);
  if (family) style.fontFamily = family;
  const size = resolveFontSize(font.size);
  if (size !== undefined) style.fontSize = `${size}px`;
  const color = resolveFontColor(font.color);
  if (color) style.color = color;
  if (font.weight === 'normal' || font.weight === 'bold') style.fontWeight = font.weight;
  return Object.keys(style).length > 0 ? style : undefined;
}

export const PanelChromeContext = createContext<PanelChrome>(DEFAULT_CHROME);

/**
 * 패널 config 에서 크롬 옵션을 읽는다. `showTitle` 이 명시적으로 false 일 때만 숨긴다 —
 * 미설정/손상 값은 기존 동작(표시)으로 폴백한다.
 */
export function readPanelChrome(config: Record<string, unknown> | undefined): PanelChrome {
  return {
    showTitle: config?.showTitle !== false,
    titleStyle: resolvePanelTitleStyle(config?.title_font),
  };
}

/** 타이틀 바를 그릴지. Provider 밖에서는 항상 true(기존 동작). */
export function usePanelTitleVisible(): boolean {
  return useContext(PanelChromeContext).showTitle;
}

/** 타이틀에 얹을 스타일. Provider 밖·미설정이면 `undefined`(기존 모양 그대로). */
export function usePanelTitleStyle(): CSSProperties | undefined {
  return useContext(PanelChromeContext).titleStyle;
}
