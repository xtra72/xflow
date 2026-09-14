# SPEC-COLOR-001 — 인수 기준

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-09-13 | 0.1.0 | 최초 작성. spec.md 0.1.0 에 대응 | xtra |

---

## 시험 규율 (이 문서 전체에 걸린다)

**꺼져 있어서 통과하는 초록은 초록이 아니다.** 이 SPEC 의 시험 가운데 다수가 조건부
렌더(스포이트 · 불투명도 슬라이더 · 비우기 · 상속색)를 잰다. 조건을 켜지 않은 채 "없다"
를 확인하는 시험은 **아무것도 재지 않는다.** 각 조건부 요소마다 **켠 갈래와 끈 갈래를
둘 다** 재고, 켠 갈래에서 실제로 **동작**하는 것까지 본다.

### 이 SPEC 이 새로 더하는 함정 (E1~E10)

| # | 함정 | 어떻게 막나 |
|---|------|-------------|
| **E1** | **8자리 소실이 조용하다.** 잘못 붙인 10자리 문자열은 예외를 내지 않고 선언만 버려진다 | 문자열 **값 자체**를 단언한다. `toHaveStyle` 은 무효 선언에서 빈 문자열을 내므로 "없음" 과 "잘못됨" 이 구분되지 않는다 → `element.style.borderColor` 를 직접 읽고 **비어 있지 않음**까지 단언 |
| **E2** | **`#3b82f6ff` 와 `#3b82f6` 가 같은 색이라 눈으로는 통과한다.** 그러나 프리셋 선택 표시가 문자열 비교라 깨진다 | 저장값을 **문자열로** 단언한다. 색이 같은지가 아니라 **바이트가 같은지** |
| **E3** | **`alpha: false` 자리의 누출 경로가 다섯이다** — 텍스트·프리셋·최근색·스포이트·슬라이더 | AC-E3 이 **다섯을 각각** 친다. 하나만 재고 "막았다" 고 하지 않는다 |
| **E4** | **기본 로케일이 `ko` 라 en 누락이 초록으로 통과한다**(기존 교훈) | i18n 시험은 **두 로케일을 다 훑는다** |
| **E5** | **jsdom 에 레이아웃이 없어 팝오버 배치를 컴포넌트로는 못 잰다** | 배치 산술을 순수 함수로 빼서 직접 호출 |
| **E6** | **`window.EyeDropper` 가 jsdom 에 없다** — 끈 갈래만 재기 쉽다 | 가짜를 꽂고 켠 갈래를, 지우고 끈 갈래를. **`delete` 로 원복**하지 않으면 다음 시험이 오염된다 |
| **E7** | **`localStorage` 가 시험 간에 살아남는다** | 각 시험 앞에서 비운다. 실패 폴백은 `setItem` 이 던지게 만들어 재는 것이지 "없다고 치고" 재는 것이 아니다 |
| **E8** | **감사 표를 코드를 읽어서 채우면 그것은 가설이지 검증이 아니다** | AC-E5 가 다섯 파서를 **실제로 호출**해 출력을 단언한다 |
| **E9** | **교체 커밋의 diff 가 커서 회귀가 묻힌다** | AC-E6 의 특성화 시험이 교체 **전에** 먼저 커밋된다 |
| **E10** | **grep 가드가 한 벌이면 놓친다** — `${…}${ALPHA}` 꼴은 `${…}NN` 정규식에 안 걸린다(실측) | 가드를 두 벌로 |

### 고정 입력 (이 문서가 함께 쓰는 값)

```
BLUE      = '#3b82f6'
BLUE_A50  = '#3b82f680'   (알파 128/255 ≈ 50%)
BLUE_FF   = '#3b82f6ff'   (완전 불투명 8자리 — 저장되어서는 안 되는 형식)
SHORT3    = '#abc'    → '#aabbcc'
SHORT4    = '#abcd'   → '#aabbccdd'  (alpha) / null (no-alpha)
SHORT4_FF = '#abcf'   → '#aabbcc'    (alpha) / null (no-alpha)
TINT      = 0x20 / 255            (기존 TILE_TINT_ALPHA '20' 에 대응)
UNION14   = [3b82f6 8b5cf6 06b6d4 10b981 f59e0b ef4444 ec4899 6b7280
             f97316 64748b 0f172a 94a3b8 ffffff 000000]  ← 기존 6배열 합집합(실측)
```

---

## 시나리오 (Given / When / Then)

### AC-01 — 색을 고르는 부품이 하나다 (REQ-01 · 불변식 I4)

**Given** 통합이 끝난 저장소
**When** `grep -rn 'type="color"' web/src --exclude='*.test.*'` 를 돌린다
**Then** **주석을 제외한 일치가 0건**이다.

**When** `grep -rn '<ColorSwatchButton\|PanelColorFreeInput\|PanelSettingsDropdown' web/src` 를 돌린다
**Then** **0건**이다(파일 자체가 없다).

**When** `<ColorPicker` 사용처를 센다
**Then** **40건 이상**이다 — 23(옛 네이티브) + 17(옛 스와치).

### AC-02 — 소비자 감사 표가 존재하고, 각 행이 실행 증거를 갖는다 (REQ-03 · E8)

**Given** 저장소의 모든 색 필드
**When** M2 감사를 수행한다
**Then** 아래 표가 채워져 있고, **`읽는 자리` 열의 모든 항목이 실제 grep 출력에서
나왔으며**, 새로 찾은 관문 셋은 **함수를 직접 불러** 확인했다.

**감사 수행일**: 2026-09-13 · **결과: 32필드 — A 22 · B 8 · C 2**

| # | 저장 키 | 편집 자리 | 읽는 자리 (대표) | 갈래 | 등급 | `alpha` |
|---|---------|----------:|------------------|------|------|---------|
| 1 | `config.panelColor` | 3 | `DashboardPage:787` C1 · `RemoteDashboardView:444` C1 · **12 이어붙이기 C5** | C1+C5 | **C→A** | 켬(M3 후) |
| 2 | `config.accentElements[<그룹>]` | 2 | `acColor` 여섯 사본 → **12 이어붙이기 C5** · `propertiesGridStyle:267` C1 | C1+C5 | **C→A** | 켬(M3 후) |
| 3 | `accentElements['labels.bg'\|'labels.text']` | 1 | `DeviceDetailPanel:833,834` C1 (유일) | C1 | **A** | 켬 |
| 4 | `config.series[i].color` | 1 | `ChartLegend:146` C1 · `BarChartPanel:472` C2 · `PieChartPanel:542` C2 · `pieLabel` C4-연착륙 | C1·C2·C4안전 | **A** | 켬 |
| 5 | `config.tsdbSource.series[i].color` | 1 | 위 series 경로와 같음 | C2 | **A** | 켬 |
| 6 | `config.sysmetricsSource.series[i].color` | 1 | 위 series 경로와 같음 | C2 | **A** | 켬 |
| 7 | `config.delta_display.{up,down,flat}_color` | 2 | `statDisplayOptions:63` (형식 검사 없음) → `StatSubLines:113` C1 | C1 | **A** | 켬 |
| 8 | `config.threshold_color_rules[i].color` | 1 | `StatPanel:710` C1 | C1 | **A** | 켬 |
| 9 | `config.y_thresholds[i].color` | 1 | `LineChartPanel:973,977,993` C2 | C2 | **A** | 켬 |
| 10 | `config.{x,y}_{label,tick}_font.color` | 1 | `chartChannelTypes:991` → recharts `fill` C2 (게이트 없음) | C2 | **A** | 켬 |
| 11 | 차트 `legend.font_color` | 1 | **`textStyle:56 resolveFontColor` → 8자리 거부** | C4-불안전 | **B** | 끔 |
| 12 | `legend_font_color` · `label_font_color` | 1 | **`textStyle:56` 거부**(`PieChartPanel:120` · `BarChartPanel:120`) | C4-불안전 | **B** | 끔 |
| 13 | 히트맵 `legend.font_color` | 1 | `heatmapConfig:576`(빈문자만) → `HeatmapLegend:302` C1 | C1 | **A** | 켬 |
| 14 | `config.color_table[i].color` | 1 | **`idw:108` `h.length !== 6` → `[0,0,0]` 검정** | C4-불안전 | **B** | 끔 |
| 15 | `config.contour.line.color` | 1 | `ContourLayer:101` `stroke` C2 | C2 | **A** | 켬 |
| 16 | `…valueColors[i].color` | 1 | `propertiesGridStyle:484` C1 · `tileSelection:159` C1 | C1 | **A** | 켬 |
| 17 | `config.propertyOverrides[k].bg` | 1 | `PropertiesGridPanel:453` C1 (tileSelection 경유 안 함) | C1 | **A** | 켬 |
| 18 | `{summaryStyles,badgeStyles,statTileStyles}[i].bg` | 2 | **`tileSelection:73` `HEX_COLOR` → 8자리 탈락, 배경 소실** | C4-불안전 | **B** | 끔 |
| 19 | 같은 키의 레거시 `.color` | (위와 같음) | **`tileSelection:80` C5** + `resolveFontColor` 게이트 | C4+C5 | **B** | 끔 |
| 20 | `config.tileBg` | 1 | `PropertiesGridPanel:453` C1 · **`tileSelection:154` 거부** | C1+C4-불안전 | **B** | 끔 |
| 21 | sysmetrics `valueColor`·`labelColor` | 1 | `SysMetricsPanelShell:224,238` C1 | C1 | **A** | 켬 |
| 22 | 게이지 `{caption,threshold_legend}_font_color` | 1 | **`textStyle:56` 거부**(`GaugePanel:244,564`) | C4-불안전 | **B** | 끔 |
| 23 | `config.needle_color` | 1 | `gaugeShapes:640,739,937` `stroke`/`fill` C2 | C2 | **A** | 켬 |
| 24 | `config.base_color` | 1 | `gaugeShapes:478,528,787` `fill` C2 | C2 | **A** | 켬 |
| 25 | 게이지 `thresholds[i].color` | 1 | `gaugeShapes:481,707,917` C2 | C2 | **A** | 켬 |
| 26 | `*_font.color` 일가(17 호출) | 1 공용 | **`panelChromeContext:69` → `textStyle:56` 거부** · **`tileSelection:80` C5** · **`listPanelStyle:80` C5** | C4-불안전+C5 | **B** | 끔 |
| 27 | `--color-*` 테마 토큰 | 1 | **`tokens:224 isValidHexColor` → `normalizeOverrides` 가 저장을 버린다** | C4-불안전(저장층) | **B** | 끔 |
| 28 | `controlButtonColor.{unselected,selectedColor,perButton}` | 3 | `acControlColors:126` → `AcControlPanel:350` C1 | C1 | **A** | 켬 |
| 29 | `fanLevelColor.{unselected,perLevel}` | 2 | `acControlColors:135` → `AcControlPanel:388` C1 | C1 | **A** | 켬 |
| 30 | `config.valueColor.ranges[i].color` | 1 | `acControlColors:62` → `AcControlPanel:215` C1 | C1 | **A** | 켬 |
| 31 | 캔버스 `config.background` | 1 | `drawElement:452` `ctx.fillStyle` C3 | C3 | **A** | 켬 |
| 32 | 캔버스 `elements[i].style.{fill,stroke,textColor}` + `rules[i].patch.*` | 5 | `drawElement:191,199,357` C3 · `canvasTween:115` C4-연착륙 | C3+C4안전 | **A** | 켬 |

**Then** 모든 행이 A / B / C 중 하나로 판정되었다. 빈 칸 없음.
**Then** `alpha` 켠 필드 22 + 끈 필드 10(B 8 + C 2 는 M3 전 기준) = **32** = 표의 행 수.

### 감사가 SPEC 본문을 정정한 것 넷 — 전부 실행으로 확인

**정정 A — C4 관문이 다섯이 아니라 일곱이다. 여섯째가 가장 넓다.**
`panels/charts/textStyle.ts:56 resolveFontColor` 는 `/^#([0-9a-f]{3}|[0-9a-f]{6})$/i` 로
**8자리를 거부하고 `undefined` 를 낸다** — 사용자가 정한 글자색이 통째로 사라지는
"조용한 소실" 이다. 실행 확인: `resolveFontColor('#3b82f680') === undefined`.
`panelChromeContext:69` 를 통해 **모든 `*_font.color`** 를, 그리고 `ChartLegend` ·
`PieChartPanel` · `BarChartPanel` · `GaugePanel` 에서 **모든 `*_font_color`** 를 막는다.
**B급 8개 중 4개를 이 하나가 만든다.** spec.md §결정 2 의 C4 표에 없던 자리다.

**정정 B — 일곱째 관문은 저장 단계에 있다.**
`lib/theme/tokens.ts:224 isValidHexColor` 가 8자리를 거부하고 `normalizeOverrides:239` 가
그 값을 **저장 자체에서 버린다**. 실행 확인: `isValidHexColor('#3b82f680') === false`.
곧 테마 토큰이 B급인 근거는 §범위가 적은 "감사 범위가 전역이라" 가 아니라 **쓰기가
기계적으로 성립하지 않아서**다. OQ2 를 정할 때 이 사실이 먼저다.

**정정 C — `gaugeShapes.hexToRgb` 의 "조용한 알파 소실" 은 어떤 필드에도 닿지 않는다.**
`hexToRgb` 의 유일한 호출자는 `sampleColorTheme` 이고, 그것은 **하드코딩된
`GAUGE_COLOR_THEMES`**(`'green-red'`·`'blue-purple'`·`'cyan-blue'`, 전부 6자리 리터럴)
만 읽는다. 사용자가 고르는 `config.colorTheme` 은 **테마 ID 문자열**이지 색이 아니다.
곧 이 위험은 **어떤 필드의 등급도 낮추지 못한다.** 게이지의 실제 색 필드 셋은 전부
SVG `fill`/`stroke`(C2)라 A급이다.

> **0.1.0 의 행(폐기 · 남긴다)**: `` | `gauge/gaugeShapes.tsx:68` `hexToRgb` | … | **조용한
> 알파 소실** | `` — 함수의 거동 서술은 **맞다**. 틀린 것은 그것이 **살아 있는 위험**
> 이라는 함의다. **AC-E5 의 `gaugeShapes` 보간 행은 하드코딩 상수를 재게 되므로 아무것도
> 재지 못한다** — 그 행은 삭제하거나 "도달 불가" 로 다시 적어야 한다.

**정정 D — `PanelSettingsDialog` 의 색 입력 14 중 하나는 죽은 코드다.**
`TileGridEditor` 의 디자인 블록은 `{(onStylesChange || renderDesign) && …}` 로 막혀
있는데 호출자 셋 중 **어느 것도 두 속성을 넘기지 않는다**. 곧 **살아 있는 네이티브
호출 자리는 23이 아니라 22**이고, 교체 총계는 40이 아니라 **39**다.

### 감사가 확인한 명령 (수행 결과)

| 명령 | 결과 |
|------|------|
| `grep -rn 'type="color"' web/src` | 27 (주석 2 · 시험 1 제외 → 24, 부품 자신 1 제외 → **23**, 죽은 코드 1 제외 → **22**) |
| `grep -rn '<ColorSwatchButton' web/src --exclude='*.test.*'` | **17** |
| 이어붙이기 가드 두 벌 (교체 전) | **10 + 2 = 12** — SPEC 과 일치 |
| 이어붙이기 가드 두 벌 (교체 후) | **0 + 0 = 0** |
| `resolveFontColor('#3b82f680')` 실행 | `undefined` |
| `isValidHexColor('#3b82f680')` 실행 | `false` |
| `resolveTileStyle(undefined, {bg:'#3b82f680'})` 실행 | `{hasOwnBackground:false}` — 배경 소실 확인 |

### AC-03 — A급 자리에서 알파가 살아서 저장되고 살아서 그려진다 (REQ-03)

**Given** `alpha` 가 켜진 자리(감사가 A급으로 판정한 필드)
**When** 불투명도 슬라이더를 50% 로 옮긴다
**Then** `onChange` 가 `BLUE_A50` 을 **문자열 그대로** 낸다.

**When** 그 값을 저장한 대시보드를 렌더한다
**Then** 해당 DOM 요소의 인라인 스타일 문자열이 `BLUE_A50` 을 포함하고, **비어 있지
않다**(E1).

### AC-04 — 정규화: §결정 4 의 표 전 행 (REQ-02 · 불변식 I1 · E2)

**Given** `normalizeColor`
**When** spec.md §결정 4 표의 **모든 입력**을 `alpha: false` 와 `alpha: true` 양쪽으로 넣는다
**Then** 표의 기대값과 **문자열 단위로** 일치한다. 특히:

| 입력 | `alpha: false` | `alpha: true` |
|------|----------------|---------------|
| `#abc` | `'#aabbcc'` | `'#aabbcc'` |
| `abc` | `'#aabbcc'` | `'#aabbcc'` |
| `#AABBCC` | `'#aabbcc'` | `'#aabbcc'` |
| `#abcd` | **`null`** | `'#aabbccdd'` |
| `#abcf` | **`null`** | **`'#aabbcc'`** |
| `#aabbccdd` | **`null`** | `'#aabbccdd'` |
| `#aabbccff` | **`null`** | **`'#aabbcc'`** |
| `#aabbccddee` | `null` | `null` |
| `rgb(1,2,3)` | `null` | `null` |
| `red` | `null` | `null` |
| `var(--x)` | `null` | `null` |
| `''` · `'  '` | `null` | `null` |

**Given** `alpha` 가 켜진 고르개
**When** 불투명도 슬라이더를 **100%** 로 올린다
**Then** `onChange` 가 내는 값이 `BLUE` 이고 **`BLUE_FF` 가 아니다**(I1 · E2).

**Then** 그 상태에서 프리셋 `BLUE` 스와치가 **선택 표시를 갖는다** — 이것이 I1 을 요구하는
이유다.

### AC-05 — 통합 팔레트 (REQ-06 · 불변식 I5 · I6)

**Given** `UNIFIED_PALETTE`
**Then** 길이가 **28**이다.
**Then** 모든 항목이 `/^#[0-9a-f]{6}$/` 다(팔레트에 알파 없음).
**Then** 중복이 없다.
**Then** `UNION14` 의 **14색이 전부 포함**된다.

**When** `grep -rn 'presets={' web/src --exclude='*.test.*'` 를 돌린다
**Then** **0건**이다(I6).

**When** `grep -rn 'PANEL_COLORS\|COLOR_PRESETS\|SUB_COLOR_PRESETS\|LOG_COLOR_PRESETS\|COLOR_PALETTE' web/src` 를 돌린다
**Then** `HEATMAP_COLOR_PRESETS` 를 제외하고 **0건**이다.

### AC-06 — 상속 색과 비우기 (REQ-04)

**Given** `value={undefined}` · `inheritedColor={BLUE}`
**When** 고르개를 연다
**Then** 스와치와 2D 판이 `BLUE` 를 보여 준다.
**Then** **아무 `onChange` 도 나지 않았다** — 상속 색은 저장되지 않는다.

**Given** `clearable` 이 켜졌고 `value={BLUE}`
**When** 비우기(✕)를 누른다 **Then** `onChange(undefined)` 가 난다.

**Given** `clearable` 이 꺼졌다
**When** 고르개를 연다 **Then** 비우기 단추가 **문서에 없다**.
**When** 어떤 조작을 해도 **Then** `onChange` 인자가 `undefined` 인 호출이 **0회**다.

**Given** `value='var(--color-text-muted)'`(A10 의 실재 값)
**When** 고르개를 연다
**Then** 스와치가 그 값을 그대로 쓰고, 2D 판·슬라이더가 **중립 회색**에서 시작한다.
**Then** 아무것도 바꾸지 않고 닫으면 `onChange` 가 **0회**다 — 열었다는 이유로 값이
`#9ca3af` 로 덮이지 않는다.

### AC-07 — 키보드만으로 임의의 색을 지정한다 (REQ-08 · 불변식 I9)

**Given** 마우스를 쓰지 않는다
**When** Tab 으로 트리거에 닿아 Enter 로 연다
**Then** 포커스가 팝오버 안 **첫 조작 요소**에 있다.

**When** Tab 으로 훑는다
**Then** 만나는 탭 정지가 이 순서다: 16진 칸 → 색상 슬라이더 → (알파 자리면) 불투명도
슬라이더 → (있으면) 스포이트 → 프리셋 격자(**정지 하나**) → 최근색 격자(**정지 하나**) →
(있으면) 비우기.
**Then** **2D 판은 탭 정지가 아니다.**

**When** 16진 칸에 `abcd` 를 치고(알파 자리) 칸을 떠난다
**Then** `onChange('#aabbccdd')` 가 났다 — 2D 판 없이 임의의 색에 닿을 수 있다.

**When** 프리셋 격자에서 → ↓ 로 옮기고 Enter 를 누른다
**Then** 그 칸의 색이 나간다. 격자가 `role="listbox"` 이고 칸이 `role="option"` 이며
현재 값의 칸이 `aria-selected="true"` 다.

**When** Esc 를 누른다
**Then** 팝오버가 닫히고 **포커스가 트리거 단추로 돌아온다**(I9).

**Then** 2D 판 요소가 `aria-hidden="true"` 이고 `tabindex` 속성을 **갖지 않는다**.

### AC-08 — 스포이트: 두 갈래를 다 본다 (REQ-05 · E6)

**Given** `window.EyeDropper` 가 **없다**
**When** 고르개를 연다 **Then** 스포이트 단추가 **문서에 없다** — 비활성 단추도 없다.

**Given** `window.EyeDropper` 에 `open()` 이 `{ sRGBHex: '#ef4444' }` 로 resolve 하는 가짜를 꽂았다
**When** 고르개를 연다 **Then** 스포이트 단추가 **있다**.
**When** 누른다 **Then** `open()` 이 **호출되었다**(켠 갈래가 동작까지 간다).

**Given** 현재 값이 `BLUE_A50`(알파 50%) · `alpha` 켬
**When** 스포이트가 `#ef4444` 를 돌려준다
**Then** `onChange('#ef444480')` 가 난다 — **알파가 유지된다**(REQ-05).

**Given** `open()` 이 reject 한다(사용자 취소)
**When** 스포이트를 누른다
**Then** `onChange` 가 **0회**이고, 처리되지 않은 거부가 **없으며**, 화면에 오류 표시가
**없다**.

### AC-09 — 최근 사용 색 (REQ-09)

**Given** 빈 `localStorage`
**When** 순서대로 `BLUE` · `#ef4444` · `BLUE` 를 고른다
**Then** 저장된 목록이 `['#3b82f6', '#ef4444']` 다 — **중복 없이 맨 앞으로 올라갔다**.

**When** 13개의 서로 다른 색을 고른다 **Then** 목록 길이가 **12**이고 가장 오래된 것이 없다.

**When** 비우기(`onChange(undefined)`)를 한다 **Then** 목록이 **변하지 않는다**.

**Given** 목록에 `BLUE_A50` 이 있다
**When** `alpha: false` 인 고르개를 연다
**Then** 최근색 격자에 `BLUE_A50` 칸이 **없다**.
**When** `alpha: true` 인 고르개를 연다 **Then** **있다**.

**Given** `localStorage.setItem` 이 던진다
**When** 색을 고른다
**Then** 예외가 밖으로 나오지 않고, 같은 세션 안에서는 최근색이 **여전히 동작한다**.

**Given** 목록을 저장한 뒤 컴포넌트를 완전히 언마운트하고 다시 마운트한다
**Then** 목록이 그대로다(세션 넘김).

---

## 엣지 케이스 (Edge Cases)

### AC-E1 — 감사가 모든 편집 자리를 덮었다 (REQ-03 · E8)

**Given** AC-02 의 표
**When** `<ColorPicker` 의 모든 사용처를 세고, 각 사용처의 저장 키를 표에서 찾는다
**Then** **표에 없는 저장 키를 쓰는 사용처가 0**이다.
**Then** 표에 있으나 사용처가 없는 행도 0이다(죽은 행 없음).

### AC-E2 — 이어붙이기가 하나도 남지 않았다 (REQ-07 · 불변식 I3 · E10)

**When** 아래 **두** 정규식을 돌린다

```
grep -rnE '\$\{[^}]*\}[0-9a-fA-F]{2}`' web/src --exclude='*.test.*'
grep -rnE '\$\{[^}]*\}\$\{[A-Z_]*(ALPHA|TINT|OPACITY)[A-Z_]*\}' web/src --exclude='*.test.*'
```

**Then** 색과 관련된 일치가 **0건**이다.
**Then** 교체 전 이 두 명령의 합이 **12건**이었다(실측 기준선 — 줄어든 것을 확인할 수
있어야 한다).

**When** `TILE_TINT_ALPHA` · `BADGE_TINT_ALPHA` 를 찾는다
**Then** 둘 다 **문자열이 아니라 수**다.

### AC-E3 — `alpha: false` 의 다섯 누출 경로 (REQ-03 · 불변식 I2 · E3)

**Given** `alpha: false` 인 고르개
**Then** 아래 다섯을 각각 친다. 다섯 모두에서 `onChange` 가 8자리를 내지 **않는다**.

| # | 경로 | 시험 |
|---|------|------|
| 1 | 텍스트 | 16진 칸에 `#aabbccdd` 를 친다 → `onChange` 0회. 칸을 떠나면 글자가 원래 값으로 되돌아온다 |
| 2 | 슬라이더 | 불투명도 슬라이더가 **문서에 없다** |
| 3 | 프리셋 | 팔레트 28색이 전부 6자리다(AC-05) |
| 4 | 최근색 | 8자리 항목이 격자에 없다(AC-09) |
| 5 | 스포이트 | 가짜가 `#ef4444` 를 준다 → `onChange('#ef4444')` — 6자리 |

### AC-E4 — 부품이 정말로 하나다 (REQ-01 · 불변식 I4)

**When** 삭제 대상 파일들의 존재를 확인한다
**Then** 아래 다섯이 **없다**:
`components/common/PanelColorFreeInput.tsx` · `components/common/PanelColorFreeInput.test.tsx` ·
`components/common/PanelSettingsDropdown.tsx` · `pages/dashboard/colorSwatchPalette.tsx` ·
`pages/dashboard/colorPalette.ts` · `pages/dashboard/panelColorPresets.ts`

**Then** `normalizePanelColor` 를 부르는 자리가 **0**이다 — 재수출 껍데기가 없다.

### AC-E5 — 다섯 파서를 실제로 호출해 잰다 (REQ-10 · 불변식 I7 · E8)

**코드를 읽어서 판정하지 않는다. 함수를 부른다.**

| 파서 | 입력 | 기대 | 근거 |
|------|------|------|------|
| `svgStyle.normalizePaint` | `'#3b82f680'` | `{ kind: 'color', value: '#3b82f680' }` | `HEX_COLOR` 가 8자리를 받는다 |
| `svgStyle.foldHex`(간접) | `'#3b82f680'`, alpha 0.5 | 알파가 곱해진 `rgba(...)` | |
| `pieLabel.sliceLabelTextColor` | `'#3b82f680'` | `'currentColor'` | 연착륙 — **검정이 아니다** |
| `canvasTween.parseColor` | `'#3b82f680'` | `null` | 연착륙 |
| `gaugeShapes` 보간(간접) | 8자리 테마 색 | RGB 는 맞고 **알파가 사라진다** | 조용한 소실 — 이 사실을 시험이 **고정**한다 |
| `idw.parseHexColor`(간접, `DEFAULT_COLOR_TABLE` 경로) | `'#3b82f680'` | **`[0,0,0]` 검정** | 조용한 오출력 — **이 필드가 B급인 이유** |
| `tileSelection.resolveTileStyle` | `{ bg: '#3b82f680' }` | `bg` 가 무시되어 `hasOwnBackground` 가 base 를 따른다 | 조용한 소실 |

**Then** 위 표의 기대와 실제 출력이 일치한다.
**Then** **8자리를 저장할 수 있는 필드(A급) 가운데, 위 다섯 중 검정/소실을 내는 파서를
읽는 자리를 가진 것이 0이다**(I7). 이것이 감사와 등급 부여를 이어 주는 문장이다.

### AC-E6 — 6자리 기존 값의 렌더가 전후로 같다 (REQ-10 · 불변식 I8 · E9)

**Given** 교체 **전** 저장소
**When** 이어붙이기 12자리 각각의 출력 문자열을 특성화 시험으로 고정한다
**Then** 예: `PropertiesGridPanel` 의 `borderColor` 가 `'#3b82f630'` 이다.

**Given** 교체 **후**(`withAlpha` 적용)
**When** 같은 시험을 돌린다
**Then** **같은 문자열**이 나온다 — `withAlpha('#3b82f6', 0x30/255) === '#3b82f630'`.
**Then** 12자리 전부에서 그렇다. 반올림 방향 차이로 한 자리라도 다르면 실패다.

**Given** 8자리 입력
**Then** `withAlpha('#3b82f680', 0x20/255)` 가 두 알파를 **곱한** 값을 낸다
(`0x80/255 × 0x20/255 ≈ 0.0627` → `'#3b82f610'`).
**Then** `withAlpha('#3b82f6', 1)` 이 **`'#3b82f6'`** 이다 — 8자리 `ff` 가 아니다.

### AC-E7 — 신규 모듈이 커버리지에 잡힌다 (불변식 I10 · 기존 교훈)

**When** `vite.config.ts` 의 `coverage.include` 를 읽는다
**Then** `'src/components/common/colorpicker/**'` 가 **있다**.
**When** 커버리지를 돌린다
**Then** `colorpicker/` 하위 파일들이 보고서에 **0%가 아닌 수치로** 나타난다 — 목록에서
빠지면 파일이 아예 보고서에 없다.

### AC-E8 — i18n 두 로케일 (E4 · 기존 교훈)

**Then** `colorPicker.*` 9키가 `ko.json` · `en.json` **양쪽**에 있다.
**Then** 키 안에 `.` 이 없다(`i18nKeyShape.test.ts` 가 잡는 형상).
**Then** 로케일을 `en` 으로 강제한 렌더에서 **치환되지 않은 키 문자열이 화면에 없다** —
기본 로케일이 `ko` 라 이 확인 없이는 en 누락이 초록으로 통과한다.

### AC-E9 — 무동작 보장 (REQ-10)

**Given** 색을 하나도 건드리지 않은 기존 대시보드 스냅샷
**When** 통합 전후로 렌더한다
**Then** DOM 의 인라인 스타일 문자열이 **완전히 같다**.
**Then** 스냅샷 JSON 이 **바이트 단위로 같다** — 열었다 닫는 것만으로 색 형식이 바뀌지
않는다.

### AC-E10 — 팝오버 배치 산술 (E5)

**Given** `popoverPlacement`(순수 함수)
**When** 트리거가 화면 아래 끝에 있다 **Then** 위로 뒤집는다.
**When** 트리거가 왼쪽 끝에 있다 **Then** `left` 가 여백 아래로 내려가지 않는다.
**When** 측정 높이가 `0` 이다(jsdom) **Then** 폴백 높이를 쓴다 — 오늘의 규율 그대로.

### AC-E11 — 스크롤·리사이즈로 닫는다

**Given** 팝오버가 열려 있다
**When** 조상 요소에 `scroll` 이 난다 **Then** 닫힌다.
**When** `resize` 가 난다 **Then** 닫힌다.
**Then** 닫힌 뒤 리스너가 **제거되었다**(언마운트 누수 없음).

---

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

| 축 | 기준 |
|----|------|
| **Tested** | 신규 모듈(`colorpicker/**`) 커버리지 **85% 이상**. `colorFormat.ts` 는 **분기 100%** — 형식을 아는 유일한 자리이므로 |
| | 교체되는 기존 자리는 **특성화 시험 먼저**(AC-E6), 그다음 교체 |
| **Readable** | 주석은 한국어. **왜 그렇게 했는지**를 적는다 — 무엇을 하는지는 코드가 말한다 |
| **Unified** | 기존 규율 승계: 형식 판정은 한 곳 · 팝오버는 `fixed` · 포커스 복귀 · 초안과 저장값 분리 |
| **Secured** | 사용자 입력 문자열이 CSS 로 들어가는 유일한 경로가 `normalizeColor` 다. 정규식 통과 없이는 한 자도 못 들어간다 |
| **Trackable** | 커밋마다 `@spec SPEC-COLOR-001` · 밀레스톤 표기. 파일별 분할 커밋 각각이 **혼자 빌드된다** |
| **LSP** | 타입 오류 0 · 린트 오류 0 |

---

## Definition of Done

- [x] REQ-01 ~ REQ-10 전부 구현
- [x] 불변식 I1 ~ I10 전부 기계 검사로 확인 — I3 · I4 · I6 은
      `components/common/colorpicker/singleSource.test.ts` 가 CI 에서 지킨다(가드 7).
      그 가드가 실제로 무는지 일부러 어겨 확인했다(`type="color"` 한 줄 + `${…}80` 한 줄을
      넣자 해당 두 가드가 붉어졌다)
- [x] AC-01 ~ AC-09 · AC-E1 ~ AC-E11 전부 통과 — AC-03~09 · E3 · E6 · E8 · E10 · E11 은
      `ColorPicker.test.tsx`(51) 외 부품 시험 160개가, AC-01 · E2 · E4 는 `singleSource.test.ts`
      가, AC-02 · E1 · E5 는 §AC-02 의 감사 표와 그 아래 실행 기록이 진다
- [x] **AC-02 의 감사 표가 빈 칸 없이 채워졌고, 각 행이 실행 증거를 갖는다**
- [x] 39 호출 자리 전부 교체(옛 네이티브 22 + 옛 스와치 17 — §정정 D 가 40 을 39 로
      정정했다), 삭제 대상 6파일 제거(+ 그 시험 2), 재수출 껍데기 0
- [x] 이어붙이기 12자리 → `withAlpha`, grep 가드 두 벌 통과(교체 후 0 + 0)
- [x] `vite.config.ts` 허용목록 갱신, 신규 모듈이 보고서에 나타남 — 구문 100% · 분기 99.15%
- [x] i18n 9키 × 2로케일 (+ M9 에서 `ruleColorAria` · `thresholdColorAria` 2키, 갈 곳이
      없어진 6키 제거 — 전부 두 로케일 대칭)
- [x] 중간 커밋 전부 detached worktree 에서 빌드 확인 — 네 커밋(b9526c8b · 6a1a878f ·
      5fe8b65e · 19acd27d) 각각에서 `tsc -b` 통과, 앞 셋은 `vite build` 도 통과
- [x] **OQ1 이 닫혔다.** 사용자의 답은 **"자동 — 테마에 맞는 적절한 색 추천"** 이며,
      (가)도 (나)도 아닌 **역할별 테마 팔레트 해석**이다. spec.md §OQ1 이 역할→토큰 대응
      (글자 `--color-text-primary` · 칠 `--color-bg-surface` · 선 `--color-border-default`)
      과 day/night 값을 적고, "없음 = 테마 기본" 이 **성립하지 않는 네 갈래**(부모 상속 ·
      안 그림 · 얼어붙은 리터럴 · 캔버스 `var()` 불가)를 표로 가른다. **본 SPEC 은 "자동"
      을 구현하지 않는다** — `inheritedColor`(표시 전용 · 저장 안 함)는 갈래 ①의 답으로
      그대로 유효하고, 나머지 셋은 후속 SPEC 이다
- [ ] **OQ2 가 사용자에게 제시되었다.** (AC-02 정정 B 가 근거를 하나 더한다 — 테마 토큰이
      B급인 것은 감사 비용이 아니라 `isValidHexColor` 가 8자리를 **저장 단계에서** 버리기
      때문이다)
