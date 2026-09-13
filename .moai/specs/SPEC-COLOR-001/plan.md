# SPEC-COLOR-001 — 구현 계획

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-09-13 | 0.1.0 | 최초 작성. spec.md 0.1.0 에 대응 | xtra |

---

## 이 계획이 먼저 정하는 것 — 순서가 곧 위험 관리다

이 작업의 위험은 **크기**가 아니라 **조용함**이다. 40자리를 갈아 끼우는 일 자체는
기계적이다. 위험한 것은 §결정 5 의 이어붙이기 12자리이며, 그것이 틀리면 **예외도 시험
실패도 없이 테두리가 사라진다.**

그래서 순서를 이렇게 둔다:

1. **산술을 먼저 세운다**(`colorFormat.ts`). DOM 이 없고, 전부 순수 함수이며, 여기서 틀리면
   뒤가 전부 틀린다.
2. **감사를 그 다음에 한다.** 감사는 UI 를 만들기 전에 끝나야 한다 — 감사 결과가 40자리
   각각의 `alpha` 값을 정하므로, 감사 전에 UI 를 꽂으면 `alpha` 를 짐작으로 켜게 된다.
3. **이어붙이기를 그 다음에 없앤다.** 이것이 끝나야 `panelColor` 가 A급이 된다.
4. **부품을 만들고**, **마지막에 40자리를 옮긴다.**

40자리 교체를 **가장 마지막**에 두는 이유는, 그것이 가장 크고 가장 안전한 단계이기
때문이다. 큰 것을 먼저 하면 작고 위험한 것들이 큰 diff 에 묻힌다.

---

## 기술 스택 (Tech Stack)

| 축 | 값 | 비고 |
|----|-----|------|
| 언어 | TypeScript 5.9 `strict` (`noUncheckedIndexedAccess`) | 실측 — 배열 인덱스에 `!` 또는 분기가 필요 |
| 프레임워크 | React 19 | |
| 번들·시험 | Vite 6 / Vitest (`jsdom`) | |
| 스타일 | Tailwind v4 (`bg-(--color-*)` 토큰) | 고정 다크 영역·`text-white` 는 토큰화 금지(기존 규율) |
| 차트 | recharts ^3.7.0 | 색은 SVG 표현 속성으로 |
| 신규 의존성 | **0** | HSV 산술·팝오버 배치 전부 직접 |

---

## 재사용 자산 (정찰로 검증 완료 — 인용)

| 자산 | 위치 | 무엇을 잇는가 |
|------|------|---------------|
| `normalizePanelColor` | `panelColorPresets.ts:34` | 3자리 펴기 · `#` 생략 허용 · 소문자 내리기. 세 규칙과 **그 이유**를 그대로 |
| `foldHex` | `canvas/svgimport/svgStyle.ts:380` | `withAlpha` 의 곱셈식(`existing * alpha`) |
| `ColorSwatchButton.place` | `colorSwatchPalette.tsx:53` | 팝오버 배치·뒤집기·가두기 산술 |
| `ColorSwatchButton` 의 닫기 규율 | 같은 파일 73–82행 | `fixed` 는 스크롤을 안 따라가므로 스크롤·리사이즈에 닫는다 |
| `StatElementStylePopover` 포커스 복귀 | 같은 파일 99–108행 | 닫을 때 연 자리로. 사라졌으면 그냥 둔다 |
| `PanelColorFreeInput` 초안 상태 | 같은 파일 27–42행 | 치는 도중을 막지 않되 형식이 맞을 때만 내보낸다 · `onBlur` 되돌리기 |
| `PALETTE_STORAGE_KEY` | `canvas/shapes/paletteGroups.ts:56` | `localStorage` 키 짓는 선례 |
| `inertQueryClient` 패턴 | 기존 패널 시험 | Provider 없이 훅 경로를 재는 규율 |

---

## 반드시 살아남아야 하는 불변식 (제약)

spec.md §불변식의 I1–I10 이 그대로 걸린다. 구현 중 특히 잊기 쉬운 넷을 다시 적는다.

- **I1 — `#rrggbbff` 를 저장하지 않는다.** HSV 슬라이더를 100% 로 올리면 8자리가 나오기
  쉽다. `normalizeColor` 가 아니라 **`onChange` 직전**에 접어야 한다.
- **I3 — `${색}NN` 이 하나도 남지 않는다.** grep 가드가 지킨다.
- **I8 — 6자리 기존 값의 렌더가 전후로 같다.** `withAlpha('#3b82f6', 0x20/255)` 가
  `'#3b82f6' + '20'` 과 **바이트 단위로 같아야 한다**. 반올림 방향을 확인할 것.
- **I10 — `vite.config.ts` 허용목록.** 적지 않으면 신규 모듈이 조용히 0% 로 빠진다. 그
  파일이 주석으로 두 번 경고한 자리다.

---

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 산술과 감사. DOM 은 아직 없다

**M1. `colorFormat.ts`**
- `normalizeColor(input, { alpha })` — spec.md §결정 4 의 표 전체.
- `withAlpha(color, alpha)` — 곱셈 · `ff` 접기.
- `hexToHsv` / `hsvToHex`.
- 시험: §결정 4 표의 **모든 행**을 `alpha` 두 값 각각으로. 왕복(hex→hsv→hex) 항등.
- **여기서 끝나야 할 것**: `#abcd` · `#abcf` · `#aabbccff` 세 경계가 확정된다.

**M2. 소비자 감사 — 이 SPEC 에서 가장 값비싼 단계**

40자리 각각에 대해 **저장 키**를 특정하고, 그 키를 읽는 모든 자리를 찾아 8자리 거동을
판정한다. 산출물은 acceptance.md AC-02 의 표이며, **각 행은 실행한 명령과 그 출력으로
뒷받침되어야 한다.**

수행 순서:
1. 40자리의 저장 키를 뽑는다(예: `config.panelColor` · `config.accentElements[g]` ·
   `font_color` · `color_table[i].color` · `--color-*` 토큰 …).
2. 키마다 `grep` 으로 읽는 자리를 전수 조사한다.
3. 읽는 자리를 C1–C5 다섯 갈래로 분류한다(spec.md §결정 2 표).
4. C4·C5 가 하나라도 있으면 B급, 없으면 A급, C5 만 있으면 C급.
5. C급은 M3 이 A 로 올린다.

**감사가 끝나기 전에 M5(부품 제작)를 시작하지 않는다** — `alpha` 기본값이 짐작이 된다.

**M3. 이어붙이기 12자리 제거**
- `withAlpha` 로 12자리 교체. `TILE_TINT_ALPHA` · `BADGE_TINT_ALPHA` 를 문자열 → 수.
- **결과 동등성 시험 먼저**: 교체 전 12자리의 출력 문자열을 특성화 시험으로 고정하고,
  교체 후 같은 문자열이 나오는지 본다(I8).
- grep 가드 추가(I3).
- **이 밀레스톤이 끝나면** `panelColor` 를 비롯한 C급 필드가 A급이 된다.

**M4. `palette.ts` + `recentColors.ts` + `popoverPlacement.ts`**
- `UNIFIED_PALETTE` 28색. 14색 포함 시험(I5).
- 최근색 MRU 12칸 · 중복 승격 · `undefined` 제외 · `localStorage` 실패 폴백.
- 배치 산술 순수 함수 — 뒤집기·가두기 경계.

### Secondary Goal (2차 목표) — 부품

**M5. `SaturationField.tsx` + `ColorPicker.tsx`**
- HSV 를 상태로 든다(hex 파생). 채도 0·명도 0 에서 색상이 소실되지 않아야 한다.
- 2D 판 `aria-hidden` · 탭 순서 밖.
- 색상 슬라이더 · 불투명도 슬라이더(`alpha` 자리만) · 16진 칸 · 스포이트(조건부) ·
  프리셋 격자(roving tabindex) · 최근색 · 비우기(조건부) · 상속색 미리보기.
- 팝오버: `role="dialog"` · 포커스 진입/복귀 · Esc · 바깥 클릭 · 스크롤/리사이즈 닫기.

**M6. i18n**
- `ko.json` · `en.json` **양쪽**에 9키. 키 안에 점 없음.
- **주의(기존 교훈)**: 기본 로케일이 `ko` 라 시험이 한쪽만 본다 — en 쪽 누락이 초록으로
  통과한다. 두 로케일을 다 훑는 시험을 붙인다.

**M7. `vite.config.ts` 커버리지 허용목록**
- `'src/components/common/colorpicker/**'` 추가. **M5 와 같은 커밋에** 넣는다 — 나중으로
  미루면 그 사이 커버리지가 0 으로 보고된다.

### Final Goal (최종 목표) — 40자리 교체와 철거

커밋은 파일별로 쪼갠다. 각 커밋이 **혼자서 빌드되고 시험을 통과해야** 한다.

**M8. `<ColorSwatchButton>` 17자리** — 7파일
`AcControlStyleSection`(6) → `CanvasElementsEditor`(5) → `PanelSettingsDialog`(2) →
`CanvasRuleTableEditor` · `TsdbSourceSection` · `SysmetricsSourceSection` ·
`AcControlThresholdsSection`(각 1)

**M9. `<input type="color">` 23자리** — 6파일
`PanelSettingsDialog`(14) → `ChartPanelSections`(5) → `StatElementStylePopover`(1) ·
`textStyleFields`(1) · `LogPanel`(1) · `ThemePaletteEditor`(1)

**M10. 프리셋 배열 통합과 철거**
- `LOG_COLOR_PRESETS` · `COLOR_PRESETS` · `SUB_COLOR_PRESETS` · `COLOR_PALETTE` ·
  `PANEL_COLORS` 삭제.
- `panelColorPresets.ts` · `colorPalette.ts` · `colorSwatchPalette.tsx` ·
  `PanelColorFreeInput.tsx`(+시험) · `PanelSettingsDropdown.tsx` 삭제.
- **재수출 껍데기를 남기지 않는다**(spec.md §결정 8).
- grep 가드(I4 · I6).

**M11. 게이트**
- 전체 시험 · 타입 검사 · 린트 · grep 가드 넷(I3 · I4 · I6 · 이어붙이기).
- acceptance.md 의 AC 전수.

---

## 커밋 분할 시 주의 (기존 교훈)

경로 휴리스틱과 import grep 만으로는 중간 커밋의 빌드를 보장하지 못한다. M10 의 삭제
커밋들은 **detached worktree 에서 실제로 빌드해 확인한다.** 특히:

- `PANEL_COLORS` 는 `PanelSettingsDropdown` · `PanelSettingsDialog` · `panelColorPresets.test.ts`
  세 곳이 물고 있다. 셋이 같은 커밋에서 사라지거나, 삭제가 마지막이어야 한다.
- `normalizePanelColor` 는 `PanelColorFreeInput` 이 물고 있다. `PanelColorFreeInput` 삭제가
  먼저다.

---

## 아키텍처 설계 방향

**형식을 아는 자리를 하나로.** `colorFormat.ts` 바깥의 어떤 파일도 hex 자릿수를 세지
않는다. 오늘 그 규율이 `PanelColorFreeInput` 머리말에 적혀 있고, 이 SPEC 은 그것을 40자리로
넓힌다.

**상태는 HSV, 표시는 hex.** hex 를 상태로 들면 채도·명도 0 에서 색상이 소실된다.

**팝오버 배치는 컴포넌트 밖으로.** jsdom 에 레이아웃이 없으므로 산술이 컴포넌트 안에
있으면 잴 수 없다. 오늘 `ColorSwatchButton` 이 그 상태이고, 이 SPEC 이 그것을 꺼낸다.

**감사가 설계를 정한다, 설계가 감사를 정하지 않는다.** `alpha` 기본값이 `false` 인 것이
그 방향의 표현이다 — 켜는 데 근거가 필요하고, 끄는 데는 필요 없다.

---

## 위험 분석 (Risk Analysis)

| # | 위험 | 형상 | 완화 |
|---|------|------|------|
| **R1** | **이어붙이기를 하나 빠뜨린다** | 테두리·배경이 조용히 사라진다. 예외 없음 | grep 가드(I3)를 **정규식 두 벌**로 — `${…}NN` 과 `${…}${…ALPHA}`. 두 벌이 필요한 것은 실측으로 확인했다(한 벌로는 `tileSelection` · `listPanelStyle` 이 안 잡혔다) |
| **R2** | **감사가 누락한 소비자가 있다** | 8자리를 저장한 뒤 어떤 화면에서만 검정/소실 | 저장 키 기준 전수 grep. **AC-E5 가 다섯 파서 각각을 직접 호출해 잰다** — 코드 읽기가 아니라 실행 |
| **R3** | **`withAlpha` 의 반올림이 기존 문자열과 어긋난다** | 6자리 기존 값의 색이 미세하게 달라진다(I8 위반) | 교체 전 특성화 시험으로 12자리 출력을 고정 |
| **R4** | **`#rrggbbff` 가 저장된다** | 프리셋 선택 표시가 같은 색에서 붙었다 안 붙었다 | `onChange` 직전 접기. AC-04 가 슬라이더 100% 경로를 직접 친다 |
| **R5** | **팝오버가 좁은 표 행에서 잘린다** | 17자리가 스크롤 컨테이너 안이다 | `position: fixed` 유지(오늘의 해법). 팔레트가 넓어지므로 폭 재산정 |
| **R6** | **`aria-hidden` 2D 판이 접근성 검사에서 반려된다** | 자동 검사 도구가 "조작 가능한데 숨겨짐" 을 잡을 수 있다 | 판에 포인터 핸들러만 두고 `tabIndex` 를 **주지 않는다**. 동등 키보드 길을 AC-07 이 실제로 걷는다 |
| **R7** | **커버리지 허용목록 누락** | 신규 모듈이 조용히 0% | I10 · AC-E7. M7 을 M5 와 같은 커밋에 |
| **R8** | **en 로케일 키 누락이 초록으로 통과** | 기본 로케일이 `ko` | 두 로케일 전수 시험(기존 교훈) |
| **R9** | **중간 커밋이 빌드되지 않는다** | M10 의 삭제 순서 | detached worktree 시뮬레이션(기존 교훈) |
| **R10** | **OQ1 의 답이 (나)로 밝혀진다** | "자동" 이 상속이 아니라 대비 기반 자동 선택 | `inheritedColor` 로 구현해 두면 (나)는 **덧붙이는 일**이지 되돌리는 일이 아니다 — 새 저장 표지와 그 소비자를 더하면 된다 |

---

## 이 작업이 만들어 내는 후속 자리 (본 SPEC 은 하지 않는다)

- **테마 토큰 알파**(OQ2) — 감사 범위가 전역이다.
- **`acColor` 여섯 사본 통합** — `withAlpha` 로 이어붙이기만 고치고 중복은 남는다.
- **스와치의 사람 말 이름** — 28×2 i18n 항목.
- **2D 판 화살표 조작** — spec.md §결정 7 (가)의 기각을 뒤집는다면.
- **대비 검사기** — `sliceLabelTextColor` 의 휘도 계산이 이미 저장소에 있으므로 값싸다.
