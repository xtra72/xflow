---
id: SPEC-COLOR-001
version: "0.1.0"
status: draft
created: 2026-09-13
updated: 2026-09-13
author: xtra
priority: P2
lifecycle_level: spec-first
title: "통합 색 고르개 — 색을 고르는 자리를 하나로 모으고, 알파를 받을 수 있는 자리와 받을 수 없는 자리를 갈라 적는다"
phase: plan
module: web/dashboard
tier: L
tags: [frontend, dashboard, color, picker, a11y, alpha, refactor]
---

# SPEC-COLOR-001 — 통합 색 고르개

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-09-13 | 0.1.0 | 최초 작성. 사용자가 draw.io 의 **글꼴 색** 대화상자 그림을 붙이고 **"컬러 팔렛트를 첨부한 이미지와 같게 다양한 기능 추가. 패널 컬러 외에도 배경색, 라인색, 문자색등 모든 컬러 설정에 적용"** 을 냈다. 사용자가 이미 정한 것 셋(① 알파 지원 — 8자리 hex + 불투명도 슬라이더, ② 스포이트 포함하되 **없는 브라우저에서는 단추를 그리지 않는다**, ③ 갈라진 프리셋 배열을 **하나로 합치고 모든 자리를 한 번에 옮긴다**)을 그대로 받는다. 정찰이 이 SPEC 의 크기를 정한 사실 **일곱**을 실측으로 확인했고, 그중 넷은 브리핑의 수치·주장을 **정정한다**(§정찰이 정정한 것). 가장 무거운 것은 다섯째다 — **`panelColor` 는 `${색}NN` 문자열 이어붙이기 12자리로 흘러들어 간다.** 그 자리가 8자리 색을 받으면 10자리 문자열이 되어 CSS 선언이 **조용히 버려진다**. 알파를 들이는 일의 실제 하중은 브리핑이 지목한 `border: 4px solid ${panelColor}` 가 아니라 여기에 있다 | xtra |

## 개요 (Overview)

### 정찰이 정정한 것 — 수치를 그대로 옮기지 않았다

브리핑이 준 재고 목록을 그대로 쓰지 않고 다시 쟀다. 다섯 곳이 달랐고, 그중 둘은 설계를
바꾼다.

**정정 1 — 살아 있는 `<input type="color">` 는 26이 아니라 24다.**
`grep -rn 'type="color"' web/src` 는 **27건**을 낸다. 그 가운데 **둘은 코드가 아니라
주석**이다(`panelColorPresets.ts:27` · `StatElementStylePopover.tsx:43`), **하나는 시험
파일**이다(`panelColorPresets.test.ts`). 남는 것이 24이며, 그 24 중 하나는 이미 공용 부품
안에 있다(`PanelColorFreeInput.tsx:49`). 곧 **옮길 호출 자리는 23**이다.

| 파일 | 브리핑 | 실측(살아 있는 것) | 차이 |
|------|-------:|------------------:|------|
| `PanelSettingsDialog.tsx` | 14 | **14** | 같음 |
| `ChartPanelSections.tsx` | 5 | **5** | 같음 |
| `StatElementStylePopover.tsx` | 2 | **1** | 43행은 주석 |
| `textStyleFields.tsx` | 1 | **1** | 같음 |
| `panels/LogPanel.tsx` | 1 | **1** | 같음 |
| `components/theme/ThemePaletteEditor.tsx` | 1 | **1** | 같음 |
| `components/common/PanelColorFreeInput.tsx` | 1 | **1** | 공용 부품 자신 |
| `panelColorPresets.ts` | 1 | **0** | 27행은 주석 |
| 합 | 26 / 8파일 | **24 / 7파일** | |

**정정 2 — 색을 고르는 자리는 `<input type="color">` 만이 아니다. 이미 두 번째 공용
부품이 있고, 그쪽이 더 많이 쓰인다.**
`ColorSwatchButton`(`web/src/pages/dashboard/colorSwatchPalette.tsx`)은 **네이티브 색 입력을
전혀 쓰지 않는다** — 단추 + 고정 팝오버 + 팔레트 격자뿐이고 **자유 입력이 아예 없다**.
그래서 브리핑의 재고 조사(`type="color"` 검색)가 이 표면을 통째로 놓쳤다. 실측
`<ColorSwatchButton` JSX 사용 **17건 / 7파일**:

| 파일 | 건수 |
|------|-----:|
| `AcControlStyleSection.tsx` | 6 |
| `panels/canvas/CanvasElementsEditor.tsx` | 5 |
| `PanelSettingsDialog.tsx` | 2 |
| `panels/canvas/CanvasRuleTableEditor.tsx` | 1 |
| `TsdbSourceSection.tsx` | 1 |
| `SysmetricsSourceSection.tsx` | 1 |
| `AcControlThresholdsSection.tsx` | 1 |

**옮길 자리의 참값은 23 + 17 = 40이다.** 브리핑의 26보다 **54% 크다.** 이 숫자가 §작업
분해의 밀레스톤 수를 정한다.

**정정 3 — 갈라진 프리셋 배열은 다섯이 아니라 여섯이고, 개수도 다르다.**

| 이름 | 위치 | 길이(실측) | 비고 |
|------|------|-----------:|------|
| `PANEL_COLORS` | `panelColorPresets.ts:9` | **8** | |
| `COLOR_PRESETS` | `PanelSettingsDialog.tsx:383` | **10** | |
| `SUB_COLOR_PRESETS` | `PanelSettingsDialog.tsx:6605` | **12** | `흰+검+COLOR_PRESETS` |
| `LOG_COLOR_PRESETS` | `panels/LogPanel.tsx:19` | **8** | 브리핑 9 — **틀림** |
| `COLOR_PALETTE` | `colorPalette.ts:6` | **10** | **브리핑이 놓쳤다** |
| `HEATMAP_COLOR_PRESETS` | `panels/heatmap/heatmapColorPresets.ts:28` | 5 | **모양이 다르다** |

`LOG_COLOR_PRESETS` 는 브리핑이 말한 "PANEL_COLORS 의 **근사** 사본" 이 아니라 **여덟 색이
같은 순서로 놓인 완전한 사본**이다(`3b82f6 8b5cf6 06b6d4 10b981 f59e0b ef4444 ec4899
6b7280`). `COLOR_PALETTE` 는 `COLOR_PRESETS` 와 **마지막 한 칸만** 다르다(`#0f172a` 대
`#94a3b8`). 곧 갈라짐의 실체는 "다섯 벌의 다른 팔레트" 가 아니라 **같은 팔레트의 여섯 벌
복사본이고 그중 둘이 한 칸씩 어긋난 것**이다. 이 사실이 §결정 3(통합 팔레트)의 값을 정한다.

`HEATMAP_COLOR_PRESETS` 는 브리핑의 짐작대로 **묶어서는 안 된다**. 그것은 스와치 목록이
아니라 `{ id, name, stops: ColorStop[] }` 이며 `stops` 는 `{ stop: 0..1, color }` 의
배열이다 — **gradient 프리셋**이다. 본 SPEC 은 이것을 건드리지 않는다(§범위).

**정정 4 — CSS 로 색 문자열을 그대로 끼워 넣는 자리는 하나가 아니라 둘이다.**
브리핑은 `RemoteDashboardView.tsx:444` 하나를 들었으나, `DashboardPage.tsx:787` 이 **같은
세 줄을 같은 모양으로** 갖고 있다(`--panel-accent` · `borderLeft` · `borderTop`). 편집
화면과 원격 보기 화면 양쪽이다.

**정정 5 — `PanelSettingsDropdown.tsx` 는 "importer 0" 이 아니다.**
제품 코드에서 부르는 곳은 **0**이 맞다. 그러나 `PanelColorFreeInput.test.tsx:13` 이
import 해서 통합 시나리오로 **실제로 렌더한다**(124행). 지금 지우면 그 시험 파일이 깨진다.
본 SPEC 의 처분은 §결정 8 에 적는다.

### 이 SPEC 이 바꾸지 않는 것부터 적는다 — 그것이 이 SPEC 의 크기다

- **백엔드 변경 0.** 색은 대시보드 `config` 안의 문자열이며 Go 쪽에 색을 해석하는 코드가
  없다. 저장 형식이 6자리에서 8자리로 늘어도 스냅샷 스키마는 바뀌지 않는다.
- **저장된 값의 마이그레이션 0.** 기존에 저장된 모든 색은 `#rrggbb` 6자리이고 §결정 4 의
  정규화가 6자리를 **그대로 6자리로** 통과시킨다. 읽기 경로는 한 자도 달라지지 않는다.
- **gradient(히트맵 `color_table`) 편집 모양 무변경.** 정지점 목록·프리셋 단추·정지점
  숫자 칸은 그대로다. 정지점 **한 칸의 색 고르개만** 통합 부품으로 갈린다.
- **차트 시리즈 자동 배색 무변경.** `PIE_COLORS` · `SERIES_COLORS` ·
  `GAUGE_COLOR_THEMES` 는 사용자가 **고르는** 팔레트가 아니라 시스템이 **배정하는** 순환
  목록이다. 통합 대상이 아니다.
- **테마 토큰의 의미 무변경.** `ThemePaletteEditor` 는 고르개만 갈고, 토큰 값은 계속
  6자리다(§결정 2 · B급).
- **신규 의존성 0.** 색 변환·HSV 산술·팝오버 배치 전부 저장소 안에 이미 있는 것으로 쓴다.

### 무엇을 바꾸는가 — 셋이다

1. **색을 고르는 부품을 하나로 만든다.** 오늘 셋이다 — 네이티브 `<input type="color">`
   23자리, `PanelColorFreeInput`(네이티브 + 16진 칸), `ColorSwatchButton`(팔레트만).
   하나가 된다.
2. **알파를 들인다 — 그러나 모든 자리에 들이지 않는다.** 어느 자리가 8자리를 견디는지는
   **소비자 감사**가 정하고, 감사 결과가 자리마다 `alpha` 를 켜고 끈다(§결정 2).
3. **여섯 벌의 프리셋 복사본을 한 벌로 합친다**(§결정 3).

### 사용자가 이미 정한 것 셋 — 다시 묻지 않는다

| # | 정해진 것 | 이 SPEC 에서의 자리 |
|---|-----------|---------------------|
| 1 | 알파 지원 — 8자리 hex + 불투명도 슬라이더 | §결정 2 · §결정 4 · REQ-02 · REQ-03 |
| 2 | 스포이트 포함 · **없는 브라우저에서는 단추를 그리지 않는다** | §결정 6 · REQ-05 |
| 3 | 40자리 전부를 한 작업으로 옮기고 프리셋을 한 벌로 합친다 (커밋은 파일별로 쪼갤 수 있다) | §결정 3 · §결정 8 · plan.md |

---

## 중심 설계 결정 — 알파는 기능이 아니라 **계약 변경**이다

브리핑이 "알파를 들이려면 감사가 필요하다" 고 한 것은 옳다. 다만 **감사가 무엇을 찾을
것인지**가 브리핑의 짐작과 달랐다. 브리핑은 CSS 보간(`border: 4px solid ${panelColor}`)을
지목했는데, **그 자리는 멀쩡하다** — CSS Color 4 를 구현한 모든 현대 브라우저가
`border: 4px solid #3b82f680` 을 정상으로 읽는다.

**실제로 깨지는 자리는 색 문자열 뒤에 알파 두 자리를 손으로 이어붙이는 코드다.**

```
borderColor: `${acColor('borders')}20`      ← 6자리 + '20' = 8자리 (유효)
                                            ← 8자리 + '20' = 10자리 (무효 → 선언이 버려진다)
```

이것이 실패의 최악 형상인 이유는 예외가 나지 않기 때문이다. React 는 무효한 CSS 값을
DOM 에 넣고, 브라우저는 **그 선언만 조용히 버린다.** 테두리가 사라지는 것으로만 관측되며,
콘솔에도 시험에도 아무것도 뜨지 않는다.

### 실측 — 알파 두 자리를 이어붙이는 자리 12

| # | 파일:행 | 식 | 색의 출처 |
|---|---------|-----|-----------|
| 1 | `panels/PropertiesGridPanel.tsx:454` | `` `${borderColor}30` `` | `acColor('borders')` |
| 2 | `panels/LogPanel.tsx:392` | `` `${acColor('levels')}20` `` | `acColor` |
| 3 | `panels/SingleDevicePanel.tsx:110` | `` `${acColor('labels')}80` `` | `acColor` |
| 4 | `devices/DeviceDetailPanel.tsx:776` | `` `${acColor('borders')}40` `` | `acColor` |
| 5 | `devices/DeviceDetailPanel.tsx:779` | `` `${acColor('borders')}20` `` | `acColor` |
| 6 | `devices/DeviceDetailPanel.tsx:815` | `` `${acColor('borders')}20` `` | `acColor` |
| 7 | `devices/DeviceDetailPanel.tsx:847` | `` `${acColor('borders')}20` `` | `acColor` |
| 8 | `devices/DeviceDetailPanel.tsx:857` | `` `${acColor('temperature')}90` `` | `acColor` |
| 9 | `devices/DeviceDetailPanel.tsx:894` | `` `${acColor('borders')}20` `` | `acColor` |
| 10 | `devices/DeviceDetailPanel.tsx:993` | `` `${acColor('borders')}30` `` | `acColor` |
| 11 | `panels/tileSelection.ts:80` | `` `${own.color}${TILE_TINT_ALPHA}` `` | 타일 글자색 |
| 12 | `panels/listPanelStyle.ts:80` | `` `${badgeStyle.color}${BADGE_TINT_ALPHA}` `` | 배지 글자색 |

### 그리고 이 열둘이 `panelColor` 를 물고 있다

`acColor` 는 여섯 파일에 같은 모양으로 복제되어 있고(`PropertiesGridPanel:96` ·
`SingleDevicePanel:90` · `LogPanel:89` · `ResourceWidget:115` · `DeviceDetailPanel:748` ·
`DeviceDetailPanel:968`), 그 본문은 실측으로 이렇다:

```ts
const acColor = (group: string): string | undefined => {
  if (!accentElements) return panelColor;      // ← 여기
  const val = accentElements[group];
  if (val === false) return undefined;
  if (typeof val === 'string') return val;
  return panelColor;                           // ← 그리고 여기
};
```

**악센트 그룹에 색을 따로 정하지 않으면 `panelColor` 가 그 자리로 내려온다.** 곧
`panelColor` 에 알파를 허용하는 순간, 사용자가 악센트를 건드린 적이 없어도 위 열두 자리가
전부 10자리 문자열을 만든다. `panelColor` 는 저장소에서 **가장 많이 쓰이는 색 설정**이고,
사용자의 요구("패널 컬러 외에도 … 모든 컬러 설정에 적용")가 가장 먼저 가리키는 자리다.

그래서 고른 길은 **이어붙이기를 없애는 것**이다. §결정 5.

---

## 결정 1 — 부품 하나. 오늘의 셋이 그 하나로 접힌다

신규 `ColorPicker` 를 `web/src/components/common/colorpicker/` 아래 짓는다. 오늘의 셋이
전부 이것으로 대체된다.

| 오늘 | 무엇을 갖고 있나 | 처분 |
|------|------------------|------|
| 네이티브 `<input type="color">` 23자리 | OS 대화상자. 알파 없음. 프리셋 없음 | 대체 |
| `PanelColorFreeInput` | 네이티브 + 16진 텍스트 칸 | **삭제** — 기능이 `ColorPicker` 의 부분집합 |
| `ColorSwatchButton` | 팝오버 + 팔레트 격자 + 비우기(X) | **삭제** — 팝오버·비우기 기능을 `ColorPicker` 가 흡수 |

### 골격 — 참조 대화상자의 무엇을 받고 무엇을 안 받는가

사용자가 붙인 draw.io 대화상자의 구성 요소별 처분:

| 참조 요소 | 본 SPEC | 근거 |
|-----------|---------|------|
| 2D 채도·명도 판 | **받는다** | 자유 색 선택의 본체 |
| 세로 색상(Hue) 슬라이더 | **받는다** | 2D 판의 세 번째 축 |
| 16진 텍스트 칸 + 스와치 | **받는다** | 이미 `PanelColorFreeInput` 에 있다 |
| 스포이트 | **받는다(조건부)** | §결정 6 |
| 불투명도 슬라이더 | **받는다** | 사용자 결정 ① — 참조에는 없다 |
| 프리셋 격자 | **받는다** | §결정 3 |
| 비우기(✕) | **받는다(조건부)** | `clearable` — `ColorSwatchButton` 의 X 를 잇는다 |
| "자동" 드롭다운 | **자리만 잇는다** | `inheritedColor` 로 표현. 참조의 드롭다운 형태는 안 받는다(§열린 질문 OQ1) |
| 회색조 토글 | **안 받는다** | 통합 팔레트 1행이 무채색 8칸이라 토글의 값이 없다 |
| "고급" 펼침 | **안 받는다** | 펼칠 것이 남지 않는다 — 알파와 스포이트를 기본 면에 둔다 |

**기각한 안 — 대화상자로 띄운다.** 참조는 모달 대화상자다. 본 저장소의 색 자리 40 가운데
17이 이미 **스크롤 컨테이너 안의 표 행**이고(`ColorSwatchButton` 의 머리말이 실측으로 적어
둔 제약: `max-h-[45vh] overflow-auto`), 2가 이미 **포털 팝오버 안**이다
(`StatElementStylePopover`). 모달을 겹치면 포커스 덫이 둘이 된다. 그래서 **고정 위치
팝오버**를 유지한다 — `ColorSwatchButton` 이 이미 푼 문제이고 그 배치 코드를 잇는다.

### 인터페이스

```ts
export interface ColorPickerProps {
  /** 현재 값. `undefined` = 정해진 것 없음. */
  value: string | undefined;
  /** 정규화를 통과한 값만 나간다. `undefined` = 비움(clearable 일 때만 가능). */
  onChange: (next: string | undefined) => void;
  /** 접근성 이름. 필수 — 자리마다 다르므로 기본값을 두지 않는다. */
  ariaLabel: string;
  /**
   * 알파(8자리)를 허용하는가. **기본 `false`.**
   * 켜는 자리는 §결정 2 의 감사를 통과한 자리뿐이다.
   */
  alpha?: boolean;
  /**
   * 값이 없을 때 실제로 적용되는 색. **표시 전용 — 저장되지 않는다.**
   * 오늘 `AccentGroupPicker` 가 `gc ?? inheritedColor ?? '#3b82f6'` 로 하던 일이다.
   */
  inheritedColor?: string;
  /** 비우기(✕) 단추를 그리는가. 기본 `false`. */
  clearable?: boolean;
  /**
   * 프리셋 목록 대체. 기본은 통합 팔레트(§결정 3).
   * **통합 이후 이 속성을 쓰는 자리는 0 이어야 한다** — 남겨 두는 것은 gradient 정지점처럼
   * 팔레트가 도메인에 매인 자리가 뒤에 생길 때를 위한 문이지, 오늘의 갈라짐을 잇는
   * 통로가 아니다(불변식 I6).
   */
  presets?: readonly string[];
  /** 목록 안에서 개별 고르개를 집을 때. */
  testId?: string;
}
```

**`onChange(undefined)` 는 `clearable` 일 때만 난다.** 오늘 40자리 중 일부는
`(c: string) => void` 로 받고 일부는 `(c: string | undefined) => void` 로 받는다. 신호를
하나로 두고 **부를 수 있는 조건을 속성으로 좁힌다** — 두 벌의 콜백 모양을 두면 자리마다
어느 쪽인지 다시 읽어야 한다.

---

## 결정 2 — 알파는 자리마다 켠다. 감사가 그것을 정한다

**알파를 전역으로 켜지 않는다.** 저장된 색 문자열을 **읽는** 경로가 저장소 안에 여러
갈래이고, 갈래마다 8자리에 대한 답이 다르다. 실측한 소비자 갈래는 다섯이다.

### 소비자 갈래별 8자리 거동 (실측)

| # | 갈래 | 대표 자리 | 8자리를 주면 | 판정 |
|---|------|-----------|--------------|------|
| C1 | **CSS 값으로 그대로 보간** | `DashboardPage.tsx:787` · `RemoteDashboardView.tsx:444` · 모든 `style={{ backgroundColor: color }}` | 정상 동작(CSS Color 4) | **안전** |
| C2 | **SVG `fill`/`stroke` 표현 속성**(recharts) | `LineChartPanel.tsx:973` 등 | 정상 동작 | **안전** |
| C3 | **Canvas 2D `fillStyle`/`strokeStyle`** | `canvas/drawElement.ts:191,199` | 정상 동작 — SPEC-CANVAS-007 이 이미 8자리를 **의도적으로** 흘려보낸다(`svgStyle.ts` 머리말 · `HEX_COLOR` 가 3/4/6/8 을 받는다) | **안전** |
| C4 | **문자열을 직접 파싱하는 코드** | 아래 표 | 갈래마다 다르다 | **갈라진다** |
| C5 | **색 문자열 + 알파 두 자리 이어붙이기** | §중심 설계 결정의 12자리 | **10자리 무효 문자열 — 선언이 조용히 버려진다** | **깨진다** |

### C4 세부 — 문자열을 파싱하는 다섯 (실측)

| 파일:행 | 하는 일 | 8자리를 주면 | 등급 |
|---------|---------|--------------|------|
| `canvas/svgimport/svgStyle.ts:335,377` | `HEX_COLOR` 3/4/6/8 · `foldHex` 가 4/8 의 알파를 꺼내 곱한다 | **정확히 처리한다** | 안전 |
| `charts/pieLabel.ts:95` `sliceLabelTextColor` | `/^#?([0-9a-f]{6})$/` 로 휘도를 잰다 | 매칭 실패 → `'currentColor'` 반환 | **연착륙**(대비가 나빠질 수 있으나 조용히 틀린 색이 되지는 않는다) |
| `canvas/canvasTween.ts:116,123` `parseColor` | HEX3/HEX6/rgb()/rgba() | `null` → 그 축은 보간하지 않고 끝값으로 튄다 | **연착륙** |
| `gauge/gaugeShapes.tsx:68` `hexToRgb` | **길이를 검사하지 않는다.** `slice(0,2)(2,4)(4,6)` | RGB 는 맞게 읽고 **알파를 소리 없이 버린다.** `rgbToHex` 가 6자리로 되돌린다 | **조용한 알파 소실** |
| `heatmap/idw.ts:101` `parseHexColor` | `h.length !== 6` → `[0,0,0]` | **검정이 된다** | **조용한 오출력** |

여기에 하나 더, 파싱은 아니지만 같은 급의 관문이 있다:

| `panels/tileSelection.ts:59` | `HEX_COLOR = /^#([0-9a-f]{3}\|[0-9a-f]{6})$/i` 로 `font.bg` 를 통과시킨다 | 8자리는 **통과하지 못하고 버려진다** — 배경색이 사라진다 | **조용한 소실** |

### 등급 — 자리마다 `alpha` 를 어떻게 정하는가

색 **필드**(저장 키)마다 등급을 매기고, 그 등급이 `alpha` 속성을 정한다.

- **A급(`alpha` 켬)** — 그 필드를 읽는 모든 소비자가 C1·C2·C3 이거나 C4 중 안전한 것뿐.
- **B급(`alpha` 끔)** — 소비자 중 하나라도 조용히 틀리는 것(C4 의 gauge·idw·tileSelection)
  또는 C5 가 있다. 고르개가 **8자리를 아예 받지 않는다**(§결정 4 의 거절).
- **C급(고쳐서 A 로 올린다)** — C5 하나만 걸림돌인 필드. §결정 5 가 그 걸림돌을 없앤다.

**등급표는 acceptance.md 가 소유한다**(AC-02). 이 SPEC 본문에 표를 복사하지 않는 이유는
그것이 **감사의 산출물**이기 때문이다 — 산출물을 명세에 미리 적으면 감사가 확인이 아니라
받아쓰기가 된다.

**감사는 인수 조건이다, 가정이 아니다.** 40자리 각각에 대해 "이 필드를 읽는 자리는 어디이며
그 자리가 8자리를 어떻게 다루는가" 를 적은 표가 나와야 하고, 표의 각 행은 **실행한 명령과
그 출력**으로 뒷받침되어야 한다(AC-02 · AC-E1).

---

## 결정 3 — 통합 팔레트. 28칸이고, 기존 여섯 배열의 색을 한 색도 잃지 않는다

여섯 벌의 복사본을 **`UNIFIED_PALETTE` 한 벌**로 합친다. 자리는
`web/src/components/common/colorpicker/palette.ts`.

참조 대화상자의 격자는 크다. 그러나 "draw.io 처럼" 으로 적으면 값이 정해지지 않으므로
**실제 목록을 적는다.** 3행 구조이며 각 행의 길이가 다른 것은 의도다 — 무채색은 명도
사다리(8단), 색상은 색상환(10색)이고 두 축의 자연스러운 눈금 수가 다르다.

```
1행 무채색 8 :  #ffffff  #f1f5f9  #94a3b8  #6b7280  #64748b  #374151  #0f172a  #000000
2행 기본색 10:  #ef4444  #f97316  #f59e0b  #84cc16  #10b981  #06b6d4  #3b82f6  #6366f1  #8b5cf6  #ec4899
3행 짙은색 10:  #b91c1c  #c2410c  #b45309  #4d7c0f  #047857  #0e7490  #1d4ed8  #4338ca  #6d28d9  #be185d
```

### 왜 28이고 왜 이 28인가

**보존이 먼저다.** 기존 여섯 배열(gradient 제외)의 합집합은 **정확히 14색**이다:

```
3b82f6 8b5cf6 06b6d4 10b981 f59e0b ef4444 ec4899 6b7280
f97316 64748b 0f172a 94a3b8 ffffff 000000
```

**이 14색이 전부 위 28 안에 있다**(1행에 6 — `ffffff 94a3b8 6b7280 64748b 0f172a 000000`,
2행에 8 — `ef4444 f97316 f59e0b 10b981 06b6d4 3b82f6 8b5cf6 ec4899`). 이것이
**불변식 I5** 이며 기계로 검사한다(AC-05). 한 색이라도 빠지면 이미 저장된 대시보드에서
"프리셋에 선택 표시가 붙던 색이 붙지 않는" 회귀가 난다 — 선택 표시가 문자열 비교이기
때문이다(`panelColor === color`, 실측).

**늘린 14는 무엇인가.** 2행을 색상환으로 채우며 빈 자리 둘(`84cc16` lime · `6366f1`
indigo)을 메웠고, 3행은 2행의 같은 색상을 짙게 한 열이다. 짙은 행이 필요한 이유는
**글자색**이다 — 기존 여섯 배열은 전부 **배경·악센트**용으로 골라진 500단계 색이고, 밝은
배경 위 글자로 쓰면 대비가 모자란다. 사용자의 요구가 "문자색등 모든 컬러 설정" 이므로
글자로 쓸 수 있는 열이 있어야 한다.

**기각한 안 — 명도 5단 × 색상 10 = 50칸.** 참조 대화상자에 가깝고 표현력도 높다. 기각한
이유는 고르개가 40자리에 들어가고 그중 17이 좁은 표 행 안이기 때문이다. 팝오버 폭이
오늘 156px 이며(실측 `POPOVER_WIDTH = 156`) 50칸은 폭을 두 배 이상으로 밀어 올린다. 28은
`5칸 × 20px` 기준 폭을 크게 넘기지 않는다.

**기각한 안 — 자리별 프리셋 부분집합을 남긴다.** 오늘의 갈라짐이 설계가 아니라 복사의
결과였음이 실측으로 드러났으므로(정정 3 — `LOG_COLOR_PRESETS` 는 완전한 사본,
`COLOR_PALETTE` 는 한 칸 차이), 부분집합을 남길 근거가 없다. `presets` 속성은 남기되
**통합 이후 그것을 쓰는 자리는 0**이다(불변식 I6, 기계 검사).

---

## 결정 4 — 정규화. `normalizePanelColor` 가 정한 것을 잇고 알파로 넓힌다

오늘의 `normalizePanelColor`(`panelColorPresets.ts:34`)가 이미 정해 둔 셋을 **그대로
잇는다**: (1) `#abc` 3자리 약식을 6자리로 편다, (2) 앞의 `#` 이 없어도 받는다,
(3) 소문자로 내린다. 세 번째가 형식 통일이 아니라 **동작**이라는 그 파일의 기록도 그대로
유효하다 — 프리셋 선택 표시가 문자열 비교다.

신규 `normalizeColor(input: string, opts: { alpha: boolean }): string | null`.

| 입력 | `alpha: false` | `alpha: true` | 비고 |
|------|----------------|---------------|------|
| `#abc` | `#aabbcc` | `#aabbcc` | 오늘과 같다 |
| `abc`(`#` 없음) | `#aabbcc` | `#aabbcc` | 오늘과 같다 |
| `#AABBCC` | `#aabbcc` | `#aabbcc` | 오늘과 같다 |
| `#aabbcc` | `#aabbcc` | `#aabbcc` | 오늘과 같다 |
| **`#abcd`** | **`null`(거절)** | **`#aabbccdd`** | 4자리 약식 — 각 자리를 두 번 |
| **`#abcf`** | **`null`** | **`#aabbcc`** | 알파가 `ff` 로 펴지므로 **6자리로 접는다** |
| **`#aabbccdd`** | **`null`(거절)** | `#aabbccdd` | |
| **`#aabbccff`** | **`null`(거절)** | **`#aabbcc`** | 완전 불투명 → 6자리로 접는다 |
| `#aabbccddee` | `null` | `null` | 10자리 — 없는 형식 |
| `rgb(1,2,3)` · `red` · `var(--x)` | `null` | `null` | §아래 |
| 빈 문자열 · 공백 | `null` | `null` | |

### 세 가지를 못박는다

**(가) 저장 형식 불변식(I1).** 저장되는 색은 **언제나** `#` + 6자리 소문자, 또는
`alpha` 가 켜진 자리에서만 `#` + 8자리 소문자이며 **8자리의 마지막 두 자리는 절대
`ff` 가 아니다.** 곧 **완전 불투명한 색의 표현은 정확히 하나**다. 이 접기가 없으면
`#3b82f6` 과 `#3b82f6ff` 가 공존하여 프리셋 선택 표시가 같은 색에서 붙었다 안 붙었다
한다 — 오늘 대문자 문제와 **정확히 같은 결함**이며, 오늘의 코드가 소문자로 내리는 이유가
그것이다.

**(나) `alpha: false` 에서 8자리는 거절이지 절삭이 아니다.** 기각한 안은 `#aabbccdd` 를
받아 `#aabbcc` 로 잘라 저장하는 것이다. 그러면 불투명도를 넣은 사용자에게 **넣었다는
피드백을 준 뒤 조용히 버린다.** 거절하면 오늘 이미 있는 거동이 그대로 작동한다 — 초안이
형식에 안 맞으면 바깥으로 내보내지 않고, 칸을 떠날 때 실제 저장값으로 되돌린다
(`PanelColorFreeInput` 의 `onBlur`, 실측). 글자가 눈앞에서 되돌아가는 것이 조용한 소실보다
정직하다. **예외는 `#aabbccff` 다** — 그것은 "알파 없음" 과 **의미가 같으므로** 거절이
아니라 6자리로 접는다.

**(다) hex 아닌 표기는 받지 않는다 — 오늘도 그렇다.** `rgb()` · 이름 색 · `var(--x)` 를
전부 `null` 로 둔다. 이유는 세 가지이며 전부 실측이다:
- `normalizePanelColor` 가 이미 그렇다(이 SPEC 이 새로 좁히는 것이 아니다).
- `StatElementStylePopover.toColorInputValue`(43–50행)의 머리말이 기록하듯, 저장된 값에
  **`var(--color-text-muted)` 가 실제로 들어 있는 경우가 있다** — `DEFAULT_DELTA_COLORS`
  의 "변화 없음" 기본값이다. 그 값은 **읽기 경로에만** 살아 있고 고르개가 만들어 내는
  값이 아니다. 통합 고르개도 그 값을 **만들지는 않되 표시할 수 있어야 한다**(REQ-04).
- `canvasTween.parseColor` 의 머리말이 적은 대로, 이름 색 표를 흉내 내면 표가 늘 모자라고
  모자란 만큼이 조용히 다른 색이 된다.

---

## 결정 5 — `${색}NN` 이어붙이기를 없앤다. 그래야 `panelColor` 가 A급이 된다

§중심 설계 결정의 12자리를 **한 함수로 갈아 끼운다.**

```ts
/** 색 위에 불투명도를 접는다. 6자리·8자리 어느 쪽을 주어도 8자리(또는 6자리)를 낸다. */
export function withAlpha(color: string, alpha: number): string;
```

- `withAlpha('#3b82f6', 0.125)` → `#3b82f620`
- `withAlpha('#3b82f680', 0.125)` → `#3b82f610` — **기존 알파와 곱한다.** 사용자가 정한
  반투명 위에 UI 의 틴트를 다시 접는 것이므로 곱이 옳다. 이 규칙은 새것이 아니다 —
  `svgimport/svgStyle.ts` 의 `foldHex` 가 **이미 같은 곱**을 한다(`existing * alpha`, 실측).
- 알파가 `1` 로 접히면 6자리로 되돌린다(불변식 I1 과 같은 규칙).

**이어붙이기 12자리를 전부 `withAlpha` 로 바꾼다.** `TILE_TINT_ALPHA = '20'` ·
`BADGE_TINT_ALPHA` 두 상수는 문자열에서 수로 바뀐다(`0x20 / 255 ≈ 0.125`).

그리고 **금지 가드를 세운다**(불변식 I3, AC-E2): 저장소 어디에도
`` `${...}NN` `` 꼴로 색 뒤에 16진 두 자리를 붙이는 식이 남지 않아야 한다. 이 가드가 없으면
같은 결함이 다음 패널에서 다시 태어난다 — 그 모양이 오늘 여섯 파일에 복제된 사실이 그
증거다.

**기각한 안 — `panelColor` 만 B급으로 두고 12자리를 건드리지 않는다.** 작업은 크게
줄지만, 사용자의 요구가 첫 번째로 가리키는 **패널 컬러에서 불투명도를 못 쓰게 된다.**
그러면 "모든 컬러 설정에 적용" 이 참이 아니게 되고, 그 사실을 사용자가 알아차리는 자리는
설정 화면이 아니라 "왜 저기만 안 되지" 다.

**기각한 안 — 읽는 쪽에서 8자리를 6자리로 깎아 이어붙이기를 살린다.** 색을 두 번
해석하게 되고(깎고 → 붙이고), `panelColor` 에 넣은 불투명도가 악센트에서만 소리 없이
사라진다. 조용한 불일치를 하나 더 만드는 안이다.

---

## 결정 6 — 스포이트: 있으면 그리고 없으면 안 그린다

`EyeDropper` 는 Chromium 계열에만 있다(Safari · Firefox 없음, 2026-09 기준). 사용자가
정한 대로 **없는 브라우저에서는 단추를 렌더하지 않는다.**

- 판정은 `typeof window !== 'undefined' && 'EyeDropper' in window` 한 번.
- 판정을 **모듈 로드 시각이 아니라 렌더 시각**에 한다. jsdom 시험에서 `window` 에 가짜를
  꽂았다 뺐다 하며 두 갈래를 다 덮으려면 모듈 상수여서는 안 된다.
- `EyeDropper.open()` 의 결과는 `{ sRGBHex: string }` 이며 **언제나 불투명**하다. 그러므로
  스포이트로 집은 값은 **알파를 건드리지 않는다** — 현재 알파를 유지한 채 RGB 만 바꾼다.
  (기각한 안: 알파를 1 로 되돌린다. 사용자가 반투명을 맞춰 둔 뒤 색만 다시 집는 흐름을
  깬다.)
- 사용자가 취소하면 `open()` 이 reject 한다. **조용히 삼킨다** — 취소는 오류가 아니다.

**기각한 안 — 없는 브라우저에서 단추를 그리고 비활성으로 둔다.** 참조 대화상자에 단추가
있으니 자리를 지킨다는 발상인데, 40자리 중 17이 좁은 표 행이라 **쓸 수 없는 칸 하나가
차지하는 폭이 아깝다.** 그리고 사용자가 이미 반대로 정했다.

---

## 결정 7 — 키보드와 화면 낭독기: 2D 판은 포인터 전용이고, 동등한 키보드 길이 따로 있다

2D 채도·명도 판은 **두 축을 동시에** 나르는 위젯이다. ARIA 에 두 축짜리 슬라이더 역할이
없으므로, 접근성을 갖추는 길은 셋뿐이다.

| 안 | 내용 | 판정 |
|----|------|------|
| (가) | 판에 `tabIndex=0` + `role="application"` 을 주고 화살표로 두 축을 움직이며 `aria-live` 로 값을 읽어 준다 | **기각** — `role="application"` 은 낭독기의 기본 탐색을 통째로 끄는 무거운 선언이고, 두 축 중 어느 축이 움직였는지를 매 키 입력마다 말해 주어야 해 낭독이 소음이 된다 |
| (나) | 판을 없애고 H·S·V·A 슬라이더 넷으로만 만든다 | **기각** — 마우스 사용자의 주된 조작 방식을 없앤다. 참조 대화상자의 중심이 그 판이다 |
| (다) | **판은 포인터 전용(`aria-hidden`)으로 두고, 같은 일을 하는 키보드 길을 따로 보장한다** | **채택** |

WCAG 2.1.1 은 **모든 기능에 키보드로 닿을 수 있을 것**을 요구하지 개별 위젯마다 키보드
조작을 요구하지 않는다. (다)는 그 조건을 다음으로 만족시킨다:

- **16진 텍스트 칸** — 임의의 색을 정확히 지정할 수 있다. 판이 하는 일의 상위집합이다.
- **색상(Hue) 슬라이더** — `<input type="range">` 네이티브. 0–359.
- **불투명도 슬라이더** — `<input type="range">` 네이티브. 0–100. `alpha` 가 켜진 자리만.
- **프리셋 격자** — **roving tabindex**. 격자 전체가 탭 정지 하나이고, 안에서 ←→↑↓ 로
  옮기며 Enter/Space 로 고른다. `role="listbox"` + `role="option"` + `aria-selected`.
- **비우기(✕)** — 보통 단추.

곧 **2D 판은 `aria-hidden="true"` 이고 탭 순서에 없다.** 이것을 감추지 않고 명세에 적는다.

### 그 밖의 접근성 계약

- 팝오버는 `role="dialog"` + `aria-label`. 열면 **첫 조작 요소로 포커스를 옮기고**, 닫으면
  **연 단추로 되돌린다**. 되돌리지 않으면 다음 Tab 이 문서 맨 앞으로 튄다 —
  `StatElementStylePopover` 가 이미 실측으로 겪고 고친 결함이다(그 파일 머리말 · AC-23).
- Esc 로 닫는다. 바깥 클릭으로 닫는다. **스크롤·리사이즈로도 닫는다** —
  `ColorSwatchButton` 이 `position: fixed` 를 쓰는 대가로 이미 채택한 규율이고, 통합 부품이
  그것을 잇는다.
- 트리거 단추는 `aria-haspopup="dialog"` · `aria-expanded`.
- 색 스와치의 접근성 이름은 **색 문자열이 아니라 이름**이어야 하나, 28색에 이름을 붙이면
  i18n 항목이 28×2 개 늘고 "짙은 청록" 류의 번역 품질을 보증할 수 없다. **오늘의 규율을
  잇는다** — `aria-label` 에 hex 문자열을 그대로 쓴다(실측: `ColorSwatchButton:121`
  `aria-label={c}`, `PanelColorRow:  aria-label={color}`). 이연 항목으로 남긴다.

---

## 결정 8 — 무엇을 지우는가

| 대상 | 처분 | 근거 |
|------|------|------|
| `components/common/PanelColorFreeInput.tsx` | **삭제** | 기능이 `ColorPicker` 의 부분집합 |
| `components/common/PanelColorFreeInput.test.tsx` | **삭제** | 지워지는 부품의 시험. 시나리오는 `ColorPicker` 의 시험으로 옮긴다(사라지지 않는다) |
| `pages/dashboard/colorSwatchPalette.tsx` | **삭제** | 기능이 `ColorPicker` 의 부분집합 |
| `pages/dashboard/colorPalette.ts` | **삭제** | `UNIFIED_PALETTE` 로 흡수 |
| `components/common/PanelSettingsDropdown.tsx` | **삭제** | 제품 importer 0(실측). 유일한 사용처가 사라지는 시험 파일이다 |
| `pages/dashboard/panelColorPresets.ts` | **유지하되 비운다** | `PANEL_COLORS` 와 `normalizePanelColor` 를 신규 모듈로 옮기고, 이 파일은 **재수출 없이 삭제**한다. 재수출 껍데기를 남기면 "정본이 하나" 라는 이 SPEC 의 주장 자체가 거짓이 된다 |
| `panels/LogPanel.tsx:19` `LOG_COLOR_PRESETS` | **삭제** | 완전한 사본(정정 3) |
| `PanelSettingsDialog.tsx` 의 `COLOR_PRESETS` · `SUB_COLOR_PRESETS` | **삭제** | |
| `heatmap/heatmapColorPresets.ts` | **손대지 않는다** | gradient — 모양이 다르다(§범위) |

**`PanelSettingsDropdown` 에 대한 브리핑의 판단은 절반만 맞다.** "importer 0" 은 제품
코드에 대해서만 참이다(실측). 그러나 그 유일한 사용처가 **함께 지워지는 시험 파일**이므로
결과는 같다 — 지운다. 이 SPEC 이 그 사실을 적는 이유는, 다음 사람이 `grep
PanelSettingsDropdown` 을 돌려 시험 파일 한 건을 보고 "쓰이고 있다" 고 오독하지 않게 하기
위해서다.

---

## 결정 9 — 최근 사용 색: 있다. `localStorage` 에 12칸, 모든 고르개가 공유한다

**존재한다.** 참조 대화상자에는 없지만, 40자리에서 같은 색을 반복해 고르는 흐름이 이
저장소의 실제 사용 모양이다(한 패널의 배경·테두리·글자를 서로 맞추는 작업).

| 축 | 정한 값 | 근거 |
|----|---------|------|
| 저장처 | `localStorage`, 키 `xflow-color-recent` | 선례가 있다 — `shapes/paletteGroups.ts:56` `PALETTE_STORAGE_KEY = 'xflow-canvas-palette'`(실측) |
| 세션 넘김 | **넘긴다** | `localStorage` 의 정의 |
| 범위 | **브라우저 전역 한 벌.** 자리별·패널별로 나누지 않는다 | 나누면 "방금 쓴 색" 이 옆 칸에서 안 보인다 — 기능의 목적이 사라진다 |
| 칸 수 | **12** | 팝오버 한 줄(6칸)의 두 배. 28칸 팔레트에 12를 더해도 폭이 늘지 않는다 |
| 순서 | MRU. 이미 있으면 맨 앞으로 올린다(중복을 만들지 않는다) | |
| 무엇이 들어가나 | `onChange` 로 **실제 나간 값**. 프리셋 클릭도 포함한다 | 프리셋을 뺄 근거가 없다 — 사용자에게 둘은 같은 행위다 |
| `onChange(undefined)` | **넣지 않는다** | 비움은 색이 아니다 |
| 서버 동기화 | **안 한다** | 대시보드 스냅샷은 256KB 예산을 공유한다(SPEC-CANVAS-007 실측). 편집 편의 상태를 그 예산에 넣지 않는다 |
| `alpha: false` 자리에서의 표시 | **8자리 항목을 걸러서 안 보인다** | 고를 수 없는 칸을 그리면 눌러도 아무 일이 안 일어난다 |
| 비우기 UI | **없다**(이연) | |
| `localStorage` 가 없거나 던지면 | **메모리 배열로 물러난다.** 기능이 죽지 않는다 | 사파리 프라이빗 모드 |

---

## 불변식 (Invariants) — 이것이 깨지면 이 SPEC 은 실패다

| # | 불변식 | 검사 |
|---|--------|------|
| **I1** | 저장되는 색은 `#` + 6자리 소문자 hex, 또는 `alpha` 자리에서만 `#` + 8자리 소문자 hex이며 **8자리의 끝 두 자리는 `ff` 가 아니다** | AC-04 |
| **I2** | `alpha: false` 인 고르개는 **어떤 경로로도** 8자리를 `onChange` 로 내보내지 않는다 — 텍스트·프리셋·최근색·스포이트·슬라이더 전부 | AC-04 · AC-E3 |
| **I3** | 저장소 어디에도 색 문자열 뒤에 16진 두 자리를 이어붙이는 식이 없다 | AC-E2(grep 가드) |
| **I4** | 색을 고르는 UI 부품은 `ColorPicker` 하나뿐이다 — `<input type="color">` 가 제품 코드에 0건 | AC-E4(grep 가드) |
| **I5** | 통합 팔레트가 기존 여섯 배열 합집합 **14색을 전부 포함**한다 | AC-05 |
| **I6** | `presets` 속성을 넘기는 호출 자리가 **0**이다 | AC-05 |
| **I7** | 8자리 색을 저장해도 그 색을 읽는 어떤 경로도 **검정을 그리지 않는다** | AC-E5 |
| **I8** | 6자리로 저장된 기존 값의 렌더 결과가 본 SPEC 전후로 **한 픽셀도 다르지 않다** | AC-E6 |
| **I9** | 팝오버를 키보드로 열고 닫으면 포커스가 **연 단추로** 돌아온다 | AC-07 |
| **I10** | 신규 모듈이 `vite.config.ts` 의 `coverage.include` 허용목록에 **적혀 있다** | AC-E7 |

---

## 범위 — 무엇이 들어오고 무엇이 안 들어오는가

### 들어온다

- 신규 `ColorPicker` 와 그 하위 모듈(정규화 · 팔레트 · HSV 산술 · 최근색 저장소).
- `<input type="color">` **23자리**와 `<ColorSwatchButton>` **17자리**, 합 **40자리**의 교체.
- §결정 5 의 이어붙이기 **12자리** 교체.
- 여섯 프리셋 배열의 통합과 삭제.
- §결정 2 의 **소비자 감사와 그 산출 표**.
- i18n 신규 키(`ko.json` · `en.json` 양쪽).

### 안 들어온다 (명시적 범위 밖)

- **gradient(히트맵 `color_table`) 편집 모양** — 정지점 목록·프리셋·정지점 숫자 칸은
  그대로. 정지점 한 칸의 고르개만 갈린다.
- **`HEATMAP_COLOR_PRESETS` 의 내용·모양** — 모양이 다르다(정정 3).
- **차트/게이지 자동 배색**(`PIE_COLORS` · `SERIES_COLORS` · `GAUGE_COLOR_THEMES`) — 사용자가
  고르는 것이 아니다.
- **테마 토큰에 알파를 들이는 일** — `ThemePaletteEditor` 는 고르개만 갈고 토큰은 6자리로
  남는다(B급). 토큰은 `--color-*` 로 화면 전역이 읽으므로 감사 범위가 이 SPEC 의 크기가
  아니다.
- **sRGB 밖의 색 공간**(OKLCH · P3 · `color()`).
- **`rgb()` · `hsl()` · 이름 색 입력 받기**(§결정 4 (다)).
- **팔레트 가져오기/내보내기 · 사용자 정의 팔레트 · 서버 동기 팔레트.**
- **대비 검사기**(WCAG 대비비 표시).
- **색 이름 붙이기**(스와치 `aria-label` 의 hex → 사람 말) — 이연.
- **2D 판의 화살표 조작** — 이연(§결정 7 (가)의 기각).
- **`acColor` 여섯 사본의 통합** — §결정 5 는 이어붙이기만 고치고 `acColor` 의 중복 자체는
  건드리지 않는다. 그것을 같이 하면 이 SPEC 이 악센트 시스템 리팩터링이 된다.

---

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드만. **백엔드 변경 0.** React 19 + TypeScript 5.9 `strict` +
  Vite 6/Vitest + Tailwind v4. **신규 의존성 0.**
- 시험 환경: `environment: 'jsdom'`(실측 `web/vite.config.ts`). 관련 제약: **`window.EyeDropper`
  없음**(가짜를 꽂아 두 갈래를 덮는다), 레이아웃 없음(`getBoundingClientRect` 가 0 —
  `ColorSwatchButton` 이 이미 `POPOVER_FALLBACK_HEIGHT` 로 대처한 자리다).
- **커버리지 대상은 명시 허용목록이다**(실측 `vite.config.ts` `coverage.include` — 파일이
  주석으로 두 번 경고한다: "적지 않으면 조용히 0% 로 빠진다"). 신규 모듈 경로를 **반드시
  적어야 한다**(불변식 I10).
- i18n: `web/src/lib/i18n/{ko,en}.json` **양쪽**. **기본 로케일 `ko`**(실측 `index.ts:34`).
  **키 이름 안에 점을 넣지 않는다**(가드 `i18nKeyShape.test.ts`).
- 차트 라이브러리는 **recharts ^3.7.0 하나**(실측 `package.json:29`). 색은 SVG 표현
  속성(`fill` · `stroke`)으로 들어간다.
- 손대는 자리(실측 행번호):
  - `pages/dashboard/PanelSettingsDialog.tsx` — 색 입력 14 + `ColorSwatchButton` 2 +
    `COLOR_PRESETS`(383) + `SUB_COLOR_PRESETS`(6605) + `AccentGroupPicker`(6899, `inheritedColor`
    를 쓰는 **유일한** 자리) + `PanelColorRow`(6721) + `SubColorRow`(6639)
  - `pages/dashboard/ChartPanelSections.tsx` — 색 입력 5 (1776 · 1821 · 2110 · 2412 · 3083)
  - `pages/dashboard/StatElementStylePopover.tsx` — 색 입력 1(182) + `toColorInputValue`(49)
  - `pages/dashboard/textStyleFields.tsx` — 색 입력 1(142)
  - `pages/dashboard/panels/LogPanel.tsx` — 색 입력 1(495) + `LOG_COLOR_PRESETS`(19) +
    이어붙이기 1(392)
  - `components/theme/ThemePaletteEditor.tsx` — 색 입력 1(210) + 16진 칸(218)
  - `pages/dashboard/{AcControlStyleSection,AcControlThresholdsSection,TsdbSourceSection,
    SysmetricsSourceSection}.tsx` · `panels/canvas/{CanvasElementsEditor,CanvasRuleTableEditor}.tsx`
    — `ColorSwatchButton` 17
  - 이어붙이기 12: `panels/PropertiesGridPanel.tsx:454` · `panels/LogPanel.tsx:392` ·
    `panels/SingleDevicePanel.tsx:110` · `devices/DeviceDetailPanel.tsx:{776,779,815,847,857,894,993}` ·
    `panels/tileSelection.ts:80` · `panels/listPanelStyle.ts:80`
- 선례(읽고 따르되 코드를 공유하지 않는다):
  - `panels/canvas/svgimport/svgStyle.ts` — 알파 접기(`foldHex`)와 "모르는 표기를 색으로
    통과시키지 않는다" 는 규율. §결정 4 (다)와 §결정 5 의 곱셈이 여기서 온다.
  - `panels/canvas/canvasTween.ts` `parseColor` — 이름 색 표를 흉내 내지 않는 근거.
  - `pages/dashboard/colorSwatchPalette.tsx` — 고정 위치 팝오버 배치·뒤집기·닫기 규율.
  - `pages/dashboard/StatElementStylePopover.tsx` — 포털 팝오버의 포커스 복귀 규율.
- 신규 파일 위치: `web/src/components/common/colorpicker/`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | **살아 있는 `<input type="color">` 는 24, 옮길 호출 자리는 23** | 확정(실측) | `grep -rn 'type="color"' web/src` 27건 − 주석 2 − 시험 1 = 24. 24 중 하나는 `PanelColorFreeInput` 안 |
| A2 | **`<ColorSwatchButton>` 호출 자리 17** | 확정(실측) | `grep -rn '<ColorSwatchButton' web/src \| grep -v '\.test\.'` |
| A3 | **`${색}NN` 이어붙이기는 12자리** | 확정(실측) | 정규식 두 벌로 훑었다 — `${…}NN` 10건 + `${…}${…ALPHA}` 2건. §중심 설계 결정 표 |
| A4 | **`acColor` 가 `panelColor` 로 폴백한다** | 확정(실측) | 여섯 사본 전부 `return panelColor`. 그래서 `panelColor` 가 C5 에 물린다 |
| A5 | **CSS·SVG·Canvas 가 8자리 hex 를 받는다** | **높음(사양 근거 · 브라우저 실측 아님)** | CSS Color 4 §4.1 · SVG2 가 색을 CSS 로 정의. **Canvas 만 저장소 안 실측이 있다** — `svgStyle.ts` 머리말이 `ctx.fillStyle` 에 8자리가 산다고 기록(SPEC-CANVAS-007). CSS·SVG 는 브라우저에서 직접 재지 않았다 → AC-E5 가 실제로 잰다 |
| A6 | **`idw.parseHexColor` 는 8자리에 검정을 낸다** | 확정(실측) | `h.length !== 6` → `return [0,0,0]`(101–111행). 조용한 오출력 |
| A7 | **`gaugeShapes.hexToRgb` 는 8자리에서 알파를 조용히 버린다** | 확정(실측) | 길이 검사 없이 `slice(0,2)(2,4)(4,6)`(68–76행) |
| A8 | **`tileSelection.HEX_COLOR` 는 8자리를 버린다** | 확정(실측) | `/^#([0-9a-f]{3}\|[0-9a-f]{6})$/i`(59행). 통과 못 하면 `bg` 가 `undefined` |
| A9 | **기존 여섯 배열의 합집합은 정확히 14색** | 확정(실측) | §결정 3 |
| A10 | **저장된 값 가운데 hex 아닌 것이 있다** | 확정(실측) | `DEFAULT_DELTA_COLORS` 의 "변화 없음" 이 `var(--color-text-muted)`. `toColorInputValue` 머리말이 그 결함(검정으로 떨어짐)을 기록 |
| A11 | **`EyeDropper` 는 Chromium 계열에만 있다** | 높음 | 2026-09 기준. 그래서 판정을 렌더 시각에 한다 — 가정이 틀려도(사파리가 붙어도) 코드가 바뀌지 않는다 |
| A12 | **`EyeDropper` 결과는 언제나 불투명** | 확정(사양) | `{ sRGBHex: string }` — 알파 채널이 인터페이스에 없다 |
| A13 | **`PanelSettingsDropdown` 의 제품 importer 는 0** | 확정(실측) | 유일한 import 가 `PanelColorFreeInput.test.tsx:13` |
| A14 | **`localStorage` 가 없거나 던질 수 있다** | 확정 | 사파리 프라이빗. §결정 9 의 메모리 폴백 |
| A15 | **6자리 저장값의 렌더가 전후로 같다** | 확정(설계) | 정규화가 6자리를 6자리로 통과시키고 소비자 경로를 바꾸지 않는다. 단 **이어붙이기 12자리는 `withAlpha` 로 갈리므로** 그쪽은 결과 동등성을 따로 잰다(AC-E6) |
| A16 | **jsdom 에 레이아웃이 없어 팝오버 배치를 시험에서 못 잰다** | 확정(실측) | `ColorSwatchButton` 이 `offsetHeight \|\| POPOVER_FALLBACK_HEIGHT` 로 이미 대처. 배치 산술은 순수 함수로 빼서 잰다 |

---

## 요구사항 (Requirements — EARS)

### REQ-01 — 하나의 부품 (Ubiquitous)

시스템은 **항상** 색을 고르는 모든 자리에서 **같은 부품**(`ColorPicker`)을 써야 하며,
제품 코드에 `<input type="color">` 를 **하나도 남기지 않아야 한다**.

시스템은 **항상** `ColorPicker` 하나로 다음 다섯 가지 개념을 표현할 수 있어야 한다:
자유 색 선택 · 프리셋 선택 · **비우기**(`clearable`) · **상속 색 표시**(`inheritedColor`) ·
**알파 허용 여부**(`alpha`).

시스템은 **항상** `onChange` 로 정규화를 통과한 값만 내보내야 하며, 통과하지 못한 초안을
바깥으로 내보내지 **않아야 한다**.

### REQ-02 — 정규화와 저장 형식 (Ubiquitous)

시스템은 **항상** 저장하는 색을 `#` + 6자리 소문자 hex, 또는 `alpha` 가 허용된 자리에서만
`#` + 8자리 소문자 hex 로 두어야 한다.

시스템은 **항상** 알파가 완전 불투명(`ff`)으로 펴지는 입력을 **6자리로 접어야 한다** —
`#abcf` → `#aabbcc`, `#aabbccff` → `#aabbcc`.

시스템은 **항상** 3자리 약식(`#abc`)을 6자리로, 4자리 약식(`#abcd`)을 8자리로 펴야 하며,
앞의 `#` 이 없는 입력을 받고, 결과를 소문자로 내려야 한다.

시스템은 **항상** hex 가 아닌 표기(`rgb()` · `hsl()` · 이름 색 · `var(--x)`)를 **거절해야
한다**(`null`).

### REQ-03 — 알파는 자리마다 켜진다 (State-Driven)

**IF** 고르개가 `alpha: false` **THEN** 불투명도 슬라이더를 **그리지 않아야 하고**, 8자리
입력을 **거절해야 하며**(잘라서 받지 **않아야 한다**), 최근 사용 색 목록에서 8자리 항목을
**보이지 않아야 한다**.

**IF** 고르개가 `alpha: true` **THEN** 0–100% 불투명도 슬라이더를 그려야 하고, 8자리
입력을 받아야 한다.

시스템은 **항상** 어떤 색 필드에 `alpha` 를 켤지를 **소비자 감사 결과**로 정해야 하며,
감사를 통과하지 못한 필드에 `alpha` 를 켜지 **않아야 한다**.

### REQ-04 — 상속 색과 비우기 (Event-Driven)

**WHEN** `value` 가 `undefined` 이고 `inheritedColor` 가 주어졌다 **THEN** 시스템은 상속
색을 **미리보기로만** 써야 하며, 사용자가 아무것도 고르지 않은 상태에서 그 값을
**저장하지 않아야 한다**.

**WHEN** 사용자가 비우기(✕)를 눌렀다 **THEN** 시스템은 `onChange(undefined)` 를 내야 한다.

**IF** `clearable` 이 꺼져 있다 **THEN** 시스템은 비우기 단추를 그리지 **않아야 하고**
`onChange(undefined)` 를 내지 **않아야 한다**.

**WHEN** `value` 가 hex 가 아닌 표기다(A10) **THEN** 시스템은 그 값을 **스와치에 그대로
표시하되** 2D 판·슬라이더의 초기 위치는 **중립 회색**으로 두어야 하며, 사용자가 아무것도
바꾸지 않으면 그 값을 **덮어쓰지 않아야 한다**.

### REQ-05 — 스포이트 (State-Driven)

**IF** 렌더 시점에 `window.EyeDropper` 가 있다 **THEN** 시스템은 스포이트 단추를 그려야
한다.

**IF** 없다 **THEN** 시스템은 스포이트 단추를 **그리지 않아야 한다** — 비활성 단추를
남기지 **않아야 한다**.

**WHEN** 스포이트가 색을 돌려주었다 **THEN** 시스템은 RGB 만 갈아 끼우고 **현재 알파를
유지해야 한다**.

**WHEN** 사용자가 스포이트를 취소했다 **THEN** 시스템은 값을 바꾸지 **않아야 하고** 오류를
표시하지 **않아야 한다**.

### REQ-06 — 통합 팔레트 (Ubiquitous)

시스템은 **항상** 모든 고르개에서 **같은 28색 팔레트**를 보여야 한다.

시스템은 **항상** 통합 팔레트에 기존 여섯 배열 합집합 **14색을 전부** 포함해야 한다.

시스템은 **항상** 자리별 프리셋 부분집합을 **쓰지 않아야 한다**(`presets` 호출 0).

### REQ-07 — 이어붙이기 금지 (Unwanted)

시스템은 색 문자열 뒤에 16진 두 자리를 **이어붙이지 않아야 한다**.

시스템은 **항상** 색 위에 불투명도를 접을 때 `withAlpha(color, alpha)` 를 써야 하며,
그것이 **기존 알파와 곱해야 한다**.

시스템은 **항상** `withAlpha` 의 결과 알파가 1 로 접히면 **6자리를 내야 한다**.

### REQ-08 — 키보드와 화면 낭독기 (Ubiquitous)

시스템은 **항상** 2D 판 없이도 임의의 색을 지정할 수 있는 키보드 길을 제공해야 한다 —
16진 칸 · 색상 슬라이더 · (알파 자리에서) 불투명도 슬라이더 · 프리셋 격자.

시스템은 **항상** 프리셋 격자를 **탭 정지 하나**로 두고 방향키로 이동, Enter/Space 로
선택하게 해야 하며, `role="listbox"`/`role="option"`/`aria-selected` 를 붙여야 한다.

시스템은 **항상** 2D 판을 `aria-hidden="true"` 로 두고 탭 순서에서 빼야 한다.

**WHEN** 팝오버가 열렸다 **THEN** 시스템은 포커스를 첫 조작 요소로 옮겨야 한다.
**WHEN** 닫혔다 **THEN** 시스템은 포커스를 **연 단추로 되돌려야 한다**(단, 그 사이 단추가
문서에서 사라졌으면 그냥 둔다).

시스템은 **항상** Esc · 바깥 클릭 · 스크롤 · 리사이즈로 팝오버를 닫아야 한다.

### REQ-09 — 최근 사용 색 (Event-Driven)

**WHEN** `onChange` 로 색이 나갔다 **THEN** 시스템은 그 색을 최근 목록 맨 앞에 올려야
하며, 이미 있으면 **중복을 만들지 않고 올려야 하고**, 목록을 **12칸으로 잘라야 한다**.

**IF** 값이 `undefined` 다(비움) **THEN** 시스템은 최근 목록에 넣지 **않아야 한다**.

**IF** `localStorage` 를 쓸 수 없다 **THEN** 시스템은 메모리 목록으로 물러나야 하며 기능이
멈추지 **않아야 한다**.

### REQ-10 — 무회귀 (Unwanted)

시스템은 6자리로 저장된 기존 값의 렌더 결과를 본 SPEC 전후로 **다르게 만들지 않아야
한다**.

시스템은 8자리 색을 저장한 결과로 어떤 화면에서도 **검정을 그리지 않아야 하고**, **선언이
조용히 버려지게 하지 않아야 한다**.

시스템은 대시보드 스냅샷 스키마를 바꾸지 **않아야 하고**, 저장된 값을 마이그레이션하지
**않아야 한다**.

---

## 명세 (Specifications)

### 층 — 색 형식을 아는 모듈은 하나뿐이다

```
components/common/colorpicker/
  ColorPicker.tsx        UI 조립. 형식을 모른다
  colorFormat.ts         normalizeColor · withAlpha · hexToHsv · hsvToHex   ← 형식을 아는 유일한 자리
  palette.ts             UNIFIED_PALETTE (28색)
  recentColors.ts        localStorage MRU 12칸 + 메모리 폴백
  popoverPlacement.ts    배치 산술(순수 함수 — jsdom 에서 잰다)
  SaturationField.tsx    2D 판 (aria-hidden, 포인터 전용)
  ColorPicker.test.tsx / colorFormat.test.ts / recentColors.test.ts / popoverPlacement.test.ts
```

`colorFormat.ts` 가 **형식을 아는 유일한 자리**다. 40 호출 자리도, 12 이어붙이기 자리도
형식을 모른다 — 오늘 `PanelColorFreeInput` 이 "값 형식 판정은 `normalizePanelColor` 한
곳에만 있다. 화면은 형식을 모른다" 고 적어 둔 규율을 그대로 잇는다.

### 자료 모델

```ts
/** 정규화된 색. 6자리 또는 8자리(끝 두 자리 ≠ ff) 소문자 hex. */
type ColorValue = string;

interface Hsva { h: number; s: number; v: number; a: number; }  // h:0–359, s/v/a:0–1
```

### 산술 — 못박는 식

**`withAlpha(color, alpha)`**: `color` 의 기존 알파를 `a0`(8자리면 `끝두자리/255`, 6자리면
`1`)이라 할 때 결과 알파 `a = a0 × alpha`. `round(a × 255)` 가 `255` 면 6자리를 내고, 아니면
`#rrggbb` + `round(a × 255)` 를 두 자리 hex 로. 이 곱은 `svgimport/svgStyle.ts` 의
`foldHex`(`existing * alpha`)와 **같은 식**이다.

**`hexToHsv` / `hsvToHex`**: 표준 sRGB↔HSV. 왕복 오차 때문에 **2D 판을 끌 때 HSV 를 상태로
들고**, hex 는 그로부터 파생한다 — hex 를 상태로 들면 채도 0 이나 명도 0 에서 색상이
소실되어 슬라이더가 튄다.

### 팝오버 배치

`popoverPlacement.ts` 가 순수 함수로 낸다: 아래로 열되 화면을 넘으면 위로 뒤집고, 좌우는
화면 안에 가둔다. 오늘 `ColorSwatchButton.place`(53–65행)가 하는 것과 **같은 산술**이며,
다른 점은 **그것이 컴포넌트 밖으로 나와 jsdom 에서 측정 가능해진다**는 것뿐이다. 상수는
그대로 잇되 팔레트가 넓어지므로 폭만 다시 잡는다(`POPOVER_WIDTH`).

### 호출 자리 교체 — 셋 가운데 하나로 접힌다

| 오늘의 모양 | 교체 후 |
|-------------|---------|
| `<input type="color" value={c ?? F} onChange={e => set(e.target.value)} />` | `<ColorPicker value={c} onChange={set} ariaLabel={…} />` |
| `<PanelColorFreeInput value={c} onChange={set} />` | `<ColorPicker value={c} onChange={set} ariaLabel={…} clearable />` |
| `<ColorSwatchButton color={c} onChange={set} ariaLabel={a} testId={t} />` | `<ColorPicker value={c} onChange={set} ariaLabel={a} testId={t} clearable />` |

`AccentGroupPicker`(6899)만 `inheritedColor` 를 더한다 — 오늘 `gc ?? inheritedColor ??
'#3b82f6'` 를 손으로 쓰던 자리다. 그 자리에 오늘 붙어 있는 **하드코딩 `100%` 글자**(6904행)는
동작한 적이 없는 불투명도 자리표시자이며, 이제 진짜 슬라이더가 그 자리를 대신한다.

### i18n 신규 키 (양쪽 로케일 · 키 안에 점 없음)

`colorPicker.{ariaDialog, hue, opacity, hex, eyedropper, presets, recent, clear, custom}`

### 추적성 (Traceability)

| 요구 | 결정 | 불변식 | 인수 |
|------|------|--------|------|
| REQ-01 | 1 · 8 | I4 | AC-01 · AC-E4 |
| REQ-02 | 4 | I1 | AC-04 |
| REQ-03 | 2 | I2 | AC-02 · AC-03 · AC-E3 |
| REQ-04 | 1 | — | AC-06 |
| REQ-05 | 6 | — | AC-08 |
| REQ-06 | 3 | I5 · I6 | AC-05 |
| REQ-07 | 5 | I3 | AC-E2 |
| REQ-08 | 7 | I9 | AC-07 |
| REQ-09 | 9 | — | AC-09 |
| REQ-10 | 2 · 5 | I7 · I8 | AC-E5 · AC-E6 |
| (환경) | — | I10 | AC-E7 |

---

## 열린 질문 (Open Questions) — 사용자 없이는 정하지 않는다

### OQ1 — 참조의 "자동" 드롭다운을 어떤 개념으로 받을 것인가

붙여 준 대화상자에는 "자동" 이라는 **드롭다운**이 있다. 본 SPEC 은 그 자리를
`inheritedColor`(상속 색 미리보기)로 잇는다 — 오늘 저장소에 이미 있는 개념이고
(`AccentGroupPicker` 가 `panelColor` 를 상속한다), 구현이 작다.

그러나 draw.io 의 "자동" 은 **상속보다 넓은** 개념일 수 있다(예: 배경 밝기에 따라 글자색을
자동으로 흑/백으로 고르는 것 — 이 저장소에도 `sliceLabelTextColor` 라는 같은 계산이 이미
있다, `charts/pieLabel.ts:92`). 그쪽이라면 `inheritedColor` 로는 표현되지 않고 **새 저장
값**(`"auto"` 같은 표지)이 필요하며, 그 표지를 읽는 모든 소비자를 또 훑어야 한다 — 이
SPEC 의 크기를 한 번 더 키운다.

**묻는 것**: "자동" 이 (가) 상위 설정에서 색을 물려받는 것인지, (나) 배경 대비로 흑/백을
스스로 고르는 것인지. **정하기 전까지는 (가)로 구현한다** — (나)가 답이면 별도 SPEC 으로
올린다.

### OQ2 — 테마 토큰에 알파를 허용할 것인가

`ThemePaletteEditor` 를 B급(알파 끔)으로 둔 것은 감사 비용 때문이지 기술적 불가능
때문이 아니다. 토큰은 `--color-*` 로 화면 **전역**이 읽으므로 소비자가 사실상 코드베이스
전체다. 사용자가 "테마 배경을 반투명하게" 를 원한다면 그것은 이 SPEC 이 아니라 후속
SPEC 의 주제이며, 그때 감사 범위가 이 SPEC 의 40자리가 아니라 전역이 된다.

**묻는 것**: 테마 토큰의 반투명이 이번 요구에 포함되는가. **정하기 전까지는 B급**이다.
