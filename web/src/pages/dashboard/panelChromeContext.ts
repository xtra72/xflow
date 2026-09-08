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
  resolveTextAlign,
  type ChartFontFamily,
  type ChartTextAlign,
} from './panels/charts/textStyle';

/**
 * 글자 모양 한 벌. 요약 타일·속성 그리드 카드처럼 "자리마다 글자 모양을 고르는" 설정이
 * 공유하는 값이다(타이틀 글자 설정은 걷어냈다 — `PanelChrome.titleStyle` 주석 참조).
 *
 * 어휘는 차트 글자 스타일(`textStyle.ts`)과 **같다**. 파이 범례·축 글꼴을 이미 그 토큰으로
 * 고르고 있으므로, 여기만 다른 어휘를 쓰면 사용자가 두 번 배워야 한다.
 */
export interface PanelTitleFont {
  family?: ChartFontFamily;
  /** 글자 크기(px). */
  size?: number;
  /** 글자색(hex). */
  color?: string;
  weight?: 'normal' | 'bold';
  /** 가로 정렬. 미지정이면 지금까지의 정렬을 그대로 둔다. */
  align?: ChartTextAlign;
}

export interface PanelChrome {
  /** 타이틀 바를 그릴지 여부. */
  showTitle: boolean;
  /**
   * 타이틀에 얹을 인라인 스타일.
   *
   * **더는 아무도 채우지 않는다.** 유일한 입력이던 타이틀 글자 모양 설정을 걷어내면서
   * 타이틀은 각 패널의 Tailwind 기본 모양으로 돌아갔다(지운 키 이름은
   * `removedFontConfigKeys.test.ts` 가 갖는다). 이 필드와 `usePanelTitleStyle` 은
   * 호출부 정리가 끝나면 함께 사라진다 — 남겨 둔 이유는 `CanvasPanel.tsx` 가 수정 금지
   * 파일이어서 30곳 중 한 곳을 지울 수 없기 때문이다(보고서 참조).
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
  const align = resolveTextAlign(font.align);
  if (align) style.textAlign = align;
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
  };
}

/** 타이틀 바를 그릴지. Provider 밖에서는 항상 true(기존 동작). */
export function usePanelTitleVisible(): boolean {
  return useContext(PanelChromeContext).showTitle;
}

/**
 * 타이틀에 얹을 스타일. 입력을 걷어냈으므로 **항상 `undefined`** 다(`PanelChrome.titleStyle`
 * 주석 참조). 호출부와 함께 지워질 자리다.
 */
export function usePanelTitleStyle(): CSSProperties | undefined {
  return useContext(PanelChromeContext).titleStyle;
}
