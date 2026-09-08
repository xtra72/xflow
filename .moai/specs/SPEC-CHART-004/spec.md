---
id: SPEC-CHART-004
title: 통계 패널 요소 직접 편집 — 드래그 배치 · 모서리 핸들 크기 · 더블클릭 글자 스타일
version: 1.0.0
status: completed
created: 2026-09-06
updated: 2026-09-08
author: xtra
priority: medium
domain: dashboard
related_specs:
  - SPEC-CHART-002
  - SPEC-CHART-003
  - SPEC-PANEL-SETTINGS-001
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-09-08 | xtra | **구현 완료 — `status: completed`.** v0.6.0 이 유일한 미충족으로 지목한 **AC-23 후반절이 구현·테스트와 함께 채워졌다**: `StatElementStylePopover.tsx:101` 이 포커스를 옮기기 전에 `document.activeElement` 를 붙들고, `:106` 의 effect 정리(=닫힘)가 `opener instanceof HTMLElement && opener.isConnected` 를 확인한 뒤 되돌린다. 이를 `StatElementStylePopover.test.tsx:203` "닫으면 포커스를 원래 요소로 되돌린다" 가, 앵커가 사라진 경우는 `:225` "열었던 요소가 사라졌으면 되돌리지 않는다" 가 잠근다. plan.md M5.4 의 "포커스 이동·**복귀**" 가 이로써 계획대로 충족되었다. **요구 커버리지**: §2 U1~U9 전량 구현. acceptance.md 의 AC-01~AC-32(AC-10 결번) 전량 충족하며 근거는 §8 표에 있다. 함께 §8 IN-4 가 완료 전 조건으로 걸어 둔 **acceptance.md §I(AC-33~AC-41, AC-39 결번)을 사후 문서로 채워** v0.4.0~v0.5.0 이 넓힌 §2.7·§2.8·§2.9 의 인수 조건 공백을 메웠다 — 그 절은 개발을 이끈 조건이 아니라 사후 대조 기록임을 절 머리에 밝혀 두었다. **검증**: `npx vitest run` exit 0(413 파일 / 6795 테스트) · `npx eslint .` 오류 0(기존 경고 12) · `npx tsc -b` exit 0. **계획 대비 분기**: 드래그 레이어·정렬·선택 모듈이 통계 전용 중간 이름을 거치지 않고 곧바로 공용 이름으로 태어났고(IN-1), `vite.config.ts` 커버리지 허용목록이 옛 이름에 머물러 8개 파일이 계측 밖이며(IN-2, **후속 필요**), plan.md §2 가 예고하지 않은 파일 5종이 생겼고(IN-3), AC-19 의 grep 단언 문구가 구현 방식을 예상하지 못했다(IN-5). 표식 이름이 `data-stat-*` → `data-panel-*` 로 바뀌어 AC-01 의 단언 문구도 옛 이름에 머물러 있다(IN-6). 넷 다 동작 결함이 아니라 문서·계측의 지연이며, 상세는 §8 에 남긴다. |
| 0.6.0 | 2026-09-08 | xtra | **인수 조건 대조 결과 기록 — `draft` 유지.** acceptance.md 의 AC-01~AC-32(AC-10 결번) 중 **AC-23 후반절이 미충족**이다: "닫으면 포커스가 본값으로 돌아온다" 를 구현한 코드도, 이를 단언하는 `StatElementStylePopover.test.tsx` 의 "닫으면 포커스를 원래 요소로 되돌린다" 테스트도 없다(팝오버는 열 때만 첫 입력으로 포커스를 옮긴다 — `StatElementStylePopover.tsx:97`). plan.md M5.4 가 "포커스 이동·**복귀**" 를 적었으므로 계획 대비 누락이다. 나머지 31건은 충족했고 그 근거는 §8 에 적는다. 함께 확인된 계측 결함 1건 — `vite.config.ts` 의 커버리지 `include` 허용목록이 이름이 바뀐 `StatDragLayer.tsx` 를 그대로 가리켜 후속 `PanelDragLayer.tsx` 가 조용히 0건으로 빠진다(그 목록의 주석이 이미 경고해 둔 함정이다). |
| 0.5.0 | 2026-09-07 | xtra | **선택 모델** 도입(§2.9 U9) — 요소를 눌러 고르고, Shift·Ctrl·Cmd 로 여럿을 고르며, 고른 것들이 **함께 움직인다**. 고른 것은 진한 실선, 아닌 것은 흐린 점선으로 구분하고 크기 손잡이는 고른 요소에만 붙인다. 무리 이동은 한 요소가 상한에 닿으면 **전체가 멈춘다** — 따로 죄면 대형이 무너진다(§5 D10). 격자 맞춤은 툴바의 자석 토글로 켜고 끌 수 있게 하고(§2.7 U7-7), 정렬은 2개 이상 골랐을 때 **고른 것끼리** 맞춘다(§2.8 U8-3b). |
| 0.4.0 | 2026-09-07 | xtra | 편집 보조 3종 추가 — 패널 안 **배치 그리드**와 **중심 `+` 표식**(§2.7 U7), **요소끼리 정렬** 툴바(§2.8 U8). 정렬 기준은 고른 요소들이 이루는 **바깥 상자**이며(디자인 도구의 정렬과 같다), 세로 정렬이 세로로 쌓인 요소를 겹치게 만드는 것은 그 조작의 뜻이므로 막지 않되 **배치 초기화**를 같은 줄에 둔다(§5 D9). 그리드는 보기와 **스냅**을 겸하고 Alt 로 잠시 끌 수 있다(§5 D8). |
| 0.3.0 | 2026-09-07 | xtra | 브라우저 확인에서 나온 두 건 반영. (1) **드래그가 마우스의 절반 속도로 움직이던 문제** — 오프셋을 `transform: translate(X%)` 로 폈는데, 그 백분율은 **요소 자신의 크기** 기준이라 패널 상자 기준으로 저장한 값과 뜻이 어긋났다(요소가 패널 폭의 절반이면 정확히 절반 속도). 상대 위치의 `left`/`top` 으로 바꾼다 — 이 둘의 백분율은 담는 상자 기준이라 저장 값의 뜻과 정확히 맞고 형제 요소의 흐름 배치도 건드리지 않는다. (2) **패널 일부 영역에서만 움직이던 문제** — 게이지·파이의 ±40(`PANEL_OFFSET_LIMIT`)을 그대로 썼다. 저 둘은 패널을 채우는 그림이라 40%면 이미 절반이 잘리지만, 통계는 가운데에서 시작하는 작은 글자라 모서리까지 50%가 필요하다. 통계 전용 상한 `STAT_OFFSET_LIMIT = 50` 을 신설한다(§5 D7). |
| 0.2.0 | 2026-09-06 | xtra | 브라우저 확인에서 나온 두 건 반영. (1) **편집 영역이 보이지 않던 문제** — §4.2 목업이 점선 상자를 그려 두었으나 구현은 커서 모양과 작은 손잡이뿐이었다. 세 요소에 점선 윤곽(`outline`, `border` 아님 — 상자 크기를 바꾸면 편집을 켜는 순간 배치가 흔들린다)을 붙이고 손잡이를 키운다. (2) **크기 변화가 끊겨 보이던 문제** — 미리보기 디바운스를 건너뛰는 키 목록(`previewLiveKeys.ts`)에 `value_layout` · `delta_layout` · `stats_layout` 을 넣지 않아 200ms 계단으로 따라왔다. 그 모듈의 주석이 "끌어 옮기는 값을 새로 만들 때마다 여기에 넣는 것을 잊었다" 고 같은 함정을 이미 기록해 두었다. 함께 §7 OQ1 의 크기 환산을 대각선 투영으로 정정한다. |
| 0.1.0 | 2026-09-06 | xtra | 최초 작성 — 통계 패널의 세 요소(본값 · 변화량 · 구간 통계)를 미리보기와 대시보드에서 직접 편집할 수 있게 한다. 끌어서 옮기고(백분율 오프셋), 모서리 핸들로 크기를 바꾸고(절대 px), 더블클릭으로 글꼴·크기·색을 정한다. 게이지·파이가 이미 쓰는 드래그 레이어 + `usePanelEditMode` 패턴을 따르며, 크기 축은 **절대 px 하나로 통일**하고 기존 `value_scale` / `sub_value_scale` 은 px 미지정 시의 폴백으로만 남긴다. |

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 한 줄이다.

```
통계 패널의 레이아웃을 미리보기에서 직접 편집할 수 있도록
```

확인 결과 세 가지 조작으로 갈렸다.

1. **위치** — 본값 · 변화량 · 구간 통계 **세 줄을 각각 독립적으로** 끌어 옮긴다.
2. **크기** — 각 요소의 모서리 핸들을 끌어 글자 크기를 바꾼다.
3. **글자 스타일** — 요소를 더블클릭하면 글꼴 · 크기 · 색을 정하는 팝오버가 열린다.

적용 면은 **미리보기와 대시보드 양쪽**이다.

### 1.2 배경 — 이미 놓여 있는 것

#### 1.2.1 드래그 레이어 패턴이 셋 있다

| 레이어 | 감싸는 대상 | 끌 수 있는 것 |
|--------|-------------|---------------|
| [`GaugeDragLayer`](../../../web/src/pages/dashboard/GaugeDragLayer.tsx) | **실제 `GaugePanel`** | 값 글자 · 임계값 범례 · 게이지 상자 |
| [`PieDragLayer`](../../../web/src/pages/dashboard/PieDragLayer.tsx) | **실제 `PieChartPanel`** | 범례 · 파이 |
| [`ChartDragLayer`](../../../web/src/pages/dashboard/ChartDragLayer.tsx) | 설정 다이얼로그의 **합성 미리보기** | 범례 · 플롯 영역 |

게이지·파이는 실제 패널이 자기 자신을 감싼다. 그래서 미리보기와 대시보드가 **같은 코드**로 동작한다. 본 SPEC 은 이 방식을 따른다 — 합성 미리보기용 별도 레이어(`ChartDragLayer` 방식)를 만들면 미리보기와 실제 패널이 서로 다른 코드로 자리를 계산하게 되어, "설정에서 맞춘 자리가 대시보드에서 다르다" 가 다시 생긴다.

게이팅은 [`usePanelEditMode`](../../../web/src/pages/dashboard/panels/PanelEditToggle.tsx) 가 세 겹으로 이미 처리한다 — `canEdit`(config 를 쓸 콜백이 있는가) · `editMode`(대시보드 편집모드인가) · `forced`(미리보기처럼 항상 편집인 자리인가, 토글 숨김).

#### 1.2.2 stat 미리보기는 이미 실제 패널을 그린다

[`PanelSettingsDialog.tsx:1665`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx) 의 `isStoreStatPreview` 는 `previewRealData` 토글만 보므로, 데이터 소스와 무관하게 실제 `StatPanel` 을 렌더한다. 미리보기 쪽 배선은 이미 서 있다.

다만 두 자리가 비어 있다.

- `StatPanel` 의 props 는 `{ panelId, title, config }` 뿐이다 — `onConfigChange` 가 없어 끌어도 저장할 곳이 없다. 게이지는 `onConfigChange={patchConfig}` + `forceEdit` 를 받는다([`:1650`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx)).
- [`renderDashboardPanel.tsx:355`](../../../web/src/pages/dashboard/renderDashboardPanel.tsx) 도 stat 에는 `onConfigChange` 를 넘기지 않는다.

#### 1.2.3 stat 에는 위치 축이 없고, 크기 축은 배율이다

게이지는 `value_offset_x/y` · `gauge_offset_x/y` · `threshold_legend_offset_x/y` 를 갖는다. stat 은 **오프셋이 하나도 없다.**

크기는 배율이다 — 본값 `36px × value_scale`, 타일 `24px × value_scale`, 보조 줄 `14px × sub_value_scale`([`StatPanel.tsx:26-33`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx), [`StatSubLines.tsx`](../../../web/src/pages/dashboard/panels/charts/StatSubLines.tsx) `SUB_LINE_PX`).

여기에 모서리 핸들과 팝오버 크기 칸을 더하면 같은 시각 속성을 건드리는 입구가 셋이 된다. 축을 하나로 정하는 것이 §5 결정 D1 이다.

#### 1.2.4 글자 스타일 편집기가 이미 있다

[`TextStyleFields`](../../../web/src/pages/dashboard/ChartPanelSections.tsx) 가 글꼴 · 크기 · 색 · 굵기 · 정렬을 편집한다. 요청한 "폰트, 크기, 컬러" 와 정확히 일치한다. 다만 두 가지가 걸린다.

- **크기 칸이 `min={6} max={40}`** 이다. stat 본값은 배율 2에서 72px 이라 범위를 벗어난다.
- **`ChartPanelSections.tsx`(약 3,900줄) 안에 산다.** 패널 컴포넌트가 여기서 import 하면 설정 모듈 전체가 패널 번들 청크로 딸려 온다(`renderDashboardPanel` 청크는 이미 429 kB). 순환 참조는 아니다 — `ChartPanelSections` 는 패널 컴포넌트를 import 하지 않는다 — 하지만 번들 방향이 뒤집힌다.

---

## 2. 요구사항 (EARS)

### 2.1 요소 배치 [U1]

- **[U1-1]** The stat 패널 config shall 요소별 배치·스타일 객체 `value_layout` · `delta_layout` · `stats_layout` 을 갖는다. 셋 다 옵셔널이며, 하위 필드도 모두 옵셔널이다.
- **[U1-2]** Where 편집이 켜져 있고 사용자가 본값 · 변화량 · 구간 통계 중 하나를 끄는 경우, the stat 패널 shall 그 요소의 `offset_x` / `offset_y` 만 바꾼다. 세 요소는 서로 독립이다.
- **[U1-3]** The 오프셋 shall 패널 상자 대비 **백분율 포인트**로 저장한다 — 픽셀로 두면 미리보기와 실제 패널의 크기가 달라 같은 값이 다른 자리를 가리킨다.
- **[U1-3b]** The 오프셋 shall 상대 위치의 `left` / `top` 으로 편다. `transform: translate(%)` 를 쓰지 않는다 — 그 백분율은 요소 자신의 크기 기준이라, 패널 상자 기준으로 저장한 값을 넣으면 요소가 작을수록 느리게 움직인다(§5 D7).
- **[U1-4]** The 오프셋 shall ±`STAT_OFFSET_LIMIT`(50)으로 죈다. 게이지·파이의 ±40 과 **다르다** — 근거는 §5 D7.
- **[U1-5]** Where 요소 밖을 끄는 경우, the stat 패널 shall 아무 일도 하지 않는다 — 미리보기에는 패널 크기 조절·휠 확대가 이미 있어 아무 데나 잡아도 끌리면 부딪힌다(게이지·파이와 같은 규칙).

### 2.2 크기 조절 [U2]

- **[U2-1]** The 요소 크기 shall **절대 픽셀** `font_size` 하나로 표현한다. 모서리 핸들 · 더블클릭 팝오버 · 설정 섹션이 모두 같은 필드를 편집한다(§5 D1).
- **[U2-2]** Where 편집이 켜져 있는 경우, the stat 패널 shall 각 요소의 **우하단 모서리**에 크기 핸들을 그린다.
- **[U2-2b]** Where 편집이 켜져 있는 경우, the stat 패널 shall 세 요소에 **점선 윤곽**을 그려 잡을 수 있는 자리를 보여 준다. 커서 모양만으로는 포인터를 올리기 전에 무엇이 잡히는지 알 수 없다. 윤곽은 `outline` 으로 그린다 — `border` 는 상자 크기를 바꿔 편집을 켜는 순간 배치가 흔들린다.
- **[U2-3]** Where 사용자가 크기 핸들을 끄는 경우, the stat 패널 shall 그 요소의 `font_size` 를 바꾼다. 오른쪽·아래로 끌면 커지고, 반대면 작아진다. 증가량은 대각선 투영(§7 OQ1)이며 소수 px 을 유지한다.
- **[U2-4]** The `font_size` shall 6px 이상 160px 이하로 죈다. 하한은 읽을 수 없는 크기를, 상한은 상자를 넘는 크기를 막는다.
- **[U2-5]** Where `font_size` 가 미지정인 경우, the stat 패널 shall 종전 계산(기본 px × 배율)을 그대로 쓴다 — 본값 `36 × value_scale`(타일이면 `24 ×`), 보조 줄 `14 × sub_value_scale`.
- **[U2-6]** The 단위 글자 shall 본값과 함께 커진다 — 종전 규칙(`STAT_VALUE_PX.unit` 비율)을 `font_size` 기준으로 환산해 유지한다.

### 2.3 글자 스타일 [U3]

- **[U3-1]** Where 편집이 켜져 있고 사용자가 요소를 **더블클릭**하는 경우, the stat 패널 shall 그 요소에 붙는 글자 스타일 팝오버를 연다.
- **[U3-2]** The 팝오버 shall 글꼴(`font_family`) · 크기(`font_size`) · 색(`font_color`) · 굵기(`font_weight`)를 편집한다.
- **[U3-3]** The 팝오버의 크기 칸 shall U2-1 과 **같은 `font_size` 필드**를 편집한다 — 핸들과 팝오버가 서로 다른 값을 쓰면 두 조작이 어긋난다.
- **[U3-4]** Where 요소가 **변화량**인 경우, the 팝오버 shall 색 칸을 글자색이 아니라 `delta_display` 의 **방향별 3색**(증가/감소/변화없음)으로 낸다 — 변화량의 색은 방향이 정하므로(SPEC-CHART-003 §2.2), 정적 글자색을 따로 두면 두 축이 싸운다.
- **[U3-5]** Where 요소가 **본값**이고 `threshold_color_rules` 가 값에 걸리는 경우, the stat 패널 shall 임계값 색을 쓴다. `font_color` 는 규칙에 걸리지 않을 때의 **기본색**이다(§5 D3).
- **[U3-6]** The 팝오버 shall 화면 경계를 넘지 않는 자리에 뜬다.
- **[U3-7]** The 팝오버 shall 바깥을 클릭하거나 `Esc` 를 누르면 닫힌다.

### 2.4 적용 면과 게이팅 [U4]

- **[U4-1]** The `StatPanel` shall `onConfigChange` prop 을 받는다. 없으면 편집이 꺼진다(`canEdit` 거짓).
- **[U4-2]** The `StatPanel` shall `forceEdit` prop 을 받는다. 설정 미리보기는 이 값을 참으로 넘겨 항상 편집이며, 토글 버튼을 내지 않는다.
- **[U4-3]** Where 대시보드에 놓인 경우, the stat 패널 shall 대시보드 편집모드 + 패널 안 토글의 **2겹 게이팅**을 거쳐야 편집이 켜진다 — 늘 켜 두면 패널을 옮기거나 크기를 바꾸려는 조작과 부딪힌다.
- **[U4-4]** The 편집 상태 shall config 에 저장하지 않는다 — 저장하면 다음에 열 때도 오버레이가 떠 있고 그것을 끄는 방법이 화면에 없다.
- **[U4-5]** Where 읽기 전용 뷰(원격 대시보드)인 경우, the stat 패널 shall 편집 입구를 내지 않는다 — `usePanelEditMode` 의 `editMode` 게이팅이 자동으로 처리한다.
- **[U4-6]** The `*_layout` 키 shall 미리보기 디바운스를 건너뛴다(`previewLiveKeys.PREVIEW_LIVE_KEYS`). 조회에 관여하지 않고 그리기만 바꾸는 값이므로, 늦추면 끄는 동안 미리보기가 200ms 계단으로 따라와 조작이 끊겨 보인다.

### 2.7 배치 그리드와 중심 표식 [U7]

- **[U7-1]** Where 편집이 켜져 있는 경우, the stat 패널 shall 기준 상자 안에 `GRID_STEP_PERCENT`(10%) 간격의 격자를 그린다.
- **[U7-2]** Where 편집이 켜져 있는 경우, the stat 패널 shall 기준 상자의 정중앙에 `+` 표식을 그린다. 오프셋 0 이 곧 이 교점이므로 "가운데로 되돌렸다" 를 눈으로 확인할 수 있다.
- **[U7-3]** The 격자와 중심 표식 shall 요소 **뒤**에 깔리고 포인터를 받지 않는다 — 참조선이 이벤트를 먹으면 그 위를 지나는 드래그가 끊긴다.
- **[U7-4]** Where 사용자가 요소를 끄는 경우, the stat 패널 shall 요소의 **중심**을 가까운 격자 교점에 붙인다. 모서리를 맞추면 요소마다 다른 변이 기준이 되어 무엇에 붙었는지 읽히지 않는다.
- **[U7-5]** Where 사용자가 `Alt` 를 누른 채 끄는 경우, the stat 패널 shall 격자 붙임을 잠시 끈다 — 정밀 조정이 막히면 격자가 오히려 방해가 된다.
- **[U7-7]** The 툴바 shall 격자 맞춤을 켜고 끄는 토글을 낸다. `Alt` 임시 해제와는 별개인 **지속** 설정이며, 선택과 마찬가지로 config 에 저장하지 않는다.
- **[U7-6]** The 격자 스냅 shall 요소의 **절대 자리**에 건다. 오프셋을 10% 단위로 죄면, 흐름상 시작 자리가 격자에 놓여 있지 않은 요소(보조 줄 둘)는 영영 격자에 맞지 않는다.

### 2.8 요소끼리 정렬 [U8]

- **[U8-1]** Where 편집이 켜져 있는 경우, the stat 패널 shall 정렬 툴바를 패널 **아래 가장자리**에 띄운다 — 위쪽은 편집 토글(좌상단)과 연결 상태 아이콘(우상단)이 이미 쓰고, 값은 세로 가운데에 모여 있다.
- **[U8-2]** The 툴바 shall 가로 3종(왼쪽 / 좌우 가운데 / 오른쪽)과 세로 3종(위 / 상하 가운데 / 아래) 정렬 단추를 낸다.
- **[U8-3b]** Where 2개 이상 골라져 있는 경우, the 정렬 shall **고른 것끼리** 맞춘다. 아니면 렌더된 전부를 맞춘다 — 아무것도 고르지 않은 채 누른 것은 "다 맞춰라" 로 읽는 편이 자연스럽다.
- **[U8-3]** The 정렬 기준 shall 고른 요소들이 이루는 **바깥 상자**다 — `start` 는 가장 앞선 변, `end` 는 가장 뒤선 변, `center` 는 그 둘의 가운데.
- **[U8-4]** Where 렌더된 요소가 2개 미만인 경우, the 정렬 shall 아무 일도 하지 않는다 — 맞출 상대가 없다.
- **[U8-5]** The 정렬 shall 세 요소의 오프셋 변경을 **한 번의** `onConfigChange` 로 넘긴다 — 나눠 넘기면 앞의 쓰기가 반영되기 전에 다음 계산이 옛 config 를 읽어 서로를 덮어쓴다.
- **[U8-6]** The 툴바 shall **배치 초기화** 단추를 함께 낸다. 세 요소의 오프셋만 지우고 글자 크기·글꼴·색은 남긴다 — 배치가 아니기 때문이다.
- **[U8-7]** The 세로 정렬 shall 세로로 쌓인 요소들을 겹치게 만든다. 그것이 그 조작의 뜻이므로 막지 않는다(§5 D9).

### 2.9 선택 모델 [U9]

- **[U9-1]** Where 편집이 켜져 있고 사용자가 요소를 누르는 경우, the stat 패널 shall 그 요소 하나를 고른다. 이미 골라져 있던 것을 다시 누르면 선택이 유지된다 — 끌기를 시작하려는 것이지 고르기를 무르려는 것이 아니다.
- **[U9-2]** Where 사용자가 `Shift`·`Ctrl`·`Cmd` 를 누른 채 요소를 누르는 경우, the stat 패널 shall 그 요소를 선택에 더하거나 뺀다. 이 조작으로는 **끌리지 않는다** — 무리에 넣으려다 배치가 흐트러지면 안 된다.
- **[U9-3]** Where 사용자가 기준 상자 안의 빈 자리를 누르는 경우, the stat 패널 shall 선택을 푼다. 툴바처럼 기준 상자 밖을 누른 것은 선택을 건드리지 않는다 — 단추를 누를 때마다 풀리면 정렬을 이어서 쓸 수 없다.
- **[U9-4]** Where 고른 요소를 끄는 경우, the stat 패널 shall 선택 **전체**에 같은 이동량을 적용한다. 한 요소라도 상한(±50)에 닿으면 **무리 전체가 멈춘다**(§5 D10).
- **[U9-5]** The 선택 표시 shall 윤곽의 **모양과 진하기를 함께** 바꾼다 — 고른 것은 진한 실선, 아닌 것은 흐린 점선. 색만 바꾸면 흐린 화면이나 색각 이상에서 두 상태가 구분되지 않는다.
- **[U9-6]** The 크기 손잡이 shall **고른 요소에만** 붙는다. 셋에 늘 붙어 있으면 좁은 패널에서 손잡이가 글자를 덮고, 무엇을 고른 상태인지도 읽히지 않는다.
- **[U9-7]** The 크기 손잡이 shall 선택과 무관하게 **자기 요소 하나만** 바꾼다 — 글자 크기가 제각각인 여러 요소에 같은 px 을 더하는 것은 뜻이 모호하다.
- **[U9-8]** The 선택 shall config 에 저장하지 않는다. 편집이 꺼지면 함께 거둔다.

### 2.5 적용 경로 [U5]

- **[U5-1]** The 요소 직접 편집 shall **단일 값 경로에만** 제공한다(§5 D2).
- **[U5-2]** Where 다중 타일 경로(`series_reduce` 지정 + 시리즈 소스 활성)인 경우, the stat 패널 shall 드래그 레이어·핸들·팝오버를 렌더하지 않는다.
- **[U5-3]** Where 저장된 config 에 `*_layout` 이 있는데 타일 경로로 렌더되는 경우, the stat 패널 shall 그 값을 **무시**하고 종전 타일 배치를 그린다 — 값은 지우지 않는다(단일 값으로 되돌리면 되살아나야 한다).

### 2.6 설정 섹션 [U6]

- **[U6-1]** The `StatChartSection` shall 기존 `value_scale` / `sub_value_scale` 슬라이더를 유지한다 — `font_size` 가 미지정인 패널의 크기 축이며, 지우면 저장된 대시보드의 크기를 바꿀 방법이 사라진다.
- **[U6-2]** Where 어떤 요소의 `font_size` 가 지정된 경우, the 설정 섹션 shall 그 배율 슬라이더 옆에 "직접 지정된 크기가 우선한다" 는 안내를 표시한다 — 슬라이더를 움직여도 화면이 안 바뀌는 이유가 보여야 한다.

---

## 3. 데이터 계약

### 3.1 config 스키마 확장

```ts
/** 요소 하나의 배치 + 글자 스타일. 전부 옵셔널이며, 없으면 종전 렌더와 같다. */
export interface StatElementLayout {
  /** 가로 오프셋(백분율 포인트, ±40). 패널 상자 폭 대비. */
  offset_x?: number;
  /** 세로 오프셋(백분율 포인트, ±40). */
  offset_y?: number;
  /** 글자 크기(px, 6~160). 미지정이면 기본 px × 배율 폴백. */
  font_size?: number;
  font_family?: ChartFontFamily;
  /** 글자색. 변화량에는 없다(방향별 색이 `delta_display` 소유 — §2.3 U3-4). */
  font_color?: string;
  font_weight?: 'normal' | 'bold';
}

export interface StatPanelConfig extends ChartPanelConfigBase {
  // ... SPEC-CHART-003 까지의 필드 ...
  /** SPEC-CHART-004 */
  value_layout?: StatElementLayout;
  delta_layout?: Omit<StatElementLayout, 'font_color'>;
  stats_layout?: StatElementLayout;
}
```

### 3.2 크기 결정 순서

| 상태 | 본값 (단일) | 변화량 · 구간 통계 |
|------|-------------|--------------------|
| `font_size` 지정 | 그 px | 그 px |
| 미지정 | `36 × value_scale` | `14 × sub_value_scale` |

`font_size` 가 이긴다. 배율은 미지정일 때만 쓰이는 폴백이다. 두 값을 곱하지 않는다 — 곱하면 핸들로 맞춘 크기가 슬라이더를 건드릴 때마다 달라져 어느 쪽이 크기를 정하는지 알 수 없다.

### 3.3 하위 호환

| 저장된 config | 렌더 결과 | 근거 |
|---------------|-----------|------|
| `*_layout` 없음 | 종전과 동일(오프셋 0, 기본 px × 배율) | U1-1 · U2-5 |
| `value_scale` 만 있음 | 종전과 동일 | 3.2 폴백 |
| `*_layout` 있고 단일 값 경로 | 오프셋·크기·글자 스타일 반영 | U1 · U2 · U3 |
| `*_layout` 있고 타일 경로 | 무시. 값은 보존 | U5-3 |

즉 **저장된 대시보드는 이 SPEC 구현 후에도 외형이 바뀌지 않는다.**

---

## 4. 화면

### 4.1 편집이 꺼진 상태 (대시보드 기본)

```
      1,234 kW
      ↑ +12 kW
 평 980 · 고 1,540 · 저 210
```

종전과 같다. 핸들도 토글도 없다.

### 4.2 편집이 켜진 상태 (미리보기 / 대시보드 편집모드 + 토글)

```
┌───────────────────────────┐
│ ┌┈┈┈┈┈┈┈┈┈┈┈┈┈┐           │
│ ┊  1,234 kW   ┊◢          │  ← 점선 = 잡을 수 있는 영역
│ └┈┈┈┈┈┈┈┈┈┈┈┈┈┘           │     ◢ = 우하단 크기 핸들
│      ┌┈┈┈┈┈┈┈┈┈┐          │
│      ┊ ↑ +12kW ┊◢         │
│      └┈┈┈┈┈┈┈┈┈┘          │
│ ┌┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┐       │
│ ┊ 평980 고1.5k 저210┊◢     │
│ └┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┘       │
└───────────────────────────┘
```

### 4.3 더블클릭 팝오버

```
      1,234 kW
   ┌──────────────────┐
   │ 본값 글자        │
   │ [상속        ▾]  │  ← 글꼴
   │ [ 72 ] ■ [보통▾] │  ← 크기(px) · 색 · 굵기
   └──────────────────┘
```

변화량 요소에서는 색 칸이 방향별 3색으로 바뀐다.

```
      ↑ +12 kW
   ┌──────────────────┐
   │ 변화량 글자      │
   │ [상속        ▾]  │
   │ [ 18 ] [보통  ▾] │
   │ 증가 ■ 감소 ■ 없음 ■ │
   └──────────────────┘
```

### 4.4 접근성

- 크기 핸들은 `aria-label` 을 갖는다.
- 더블클릭 외에 **키보드 경로**를 둔다 — 요소는 편집 중 `tabIndex=0` 이며 `Enter` 로도 팝오버가 열린다. 더블클릭만 두면 키보드로는 글자 스타일에 닿을 수 없다.
- 팝오버는 열릴 때 첫 입력으로 포커스를 옮기고, 닫히면 원래 요소로 되돌린다.

---

## 5. 결정 (Decisions)

### D1 — 크기 축은 절대 px 하나다

핸들 · 팝오버 · 설정 슬라이더가 모두 크기를 건드린다. 축을 둘로 두면(배율 + px) 같은 시각 속성에 입구가 둘이 되어, 방금 `panelColor` 에서 정리한 것과 같은 문제가 생긴다.

절대 px 을 고른 이유는 셋이다.

1. **말과 숫자가 일치한다.** 팝오버가 "폰트 크기" 라고 부르는 칸에 `72` 가 들어가는 것이 `1.50×` 보다 직접적이다.
2. **`TextStyleFields` 를 그대로 쓴다.** 이미 px 을 전제한 편집기이며, 상한만 40 → 160 으로 넓히면 된다.
3. **요소마다 기본 px 이 다른 문제가 사라진다.** 배율이면 본값 36 · 타일 24 · 보조 줄 14 라는 서로 다른 기준을 계속 들고 다녀야 한다.

기존 `value_scale` / `sub_value_scale` 은 **지우지 않는다.** px 미지정일 때의 폴백으로 남긴다(§3.2). 지우면 저장된 대시보드의 크기 설정이 죽는다. 두 값을 곱하지 않는 이유는 §3.2 에 적었다.

### D2 — 요소 직접 편집은 단일 값 경로 전용이다

다중 타일 경로에서는 그리드가 각 타일의 자리를 정한다([`SeriesTileGrid`](../../../web/src/pages/dashboard/panels/charts/SeriesTileGrid.tsx) — 열 수·행 수·최소 폭). 요소에 오프셋을 걸면 **모든 타일에 같은 오프셋이 똑같이 걸려** 그리드와 싸운다. 타일마다 다른 자리를 주려면 타일별 오프셋이 필요한데, 시리즈가 늘거나 줄면 그 값이 어느 타일을 가리키는지 알 수 없다.

그래서 타일 경로에서는 배치 편집을 내지 않는다. 타일의 배치는 `tile_rows` · `multi_output_limit` 이 계속 소유한다.

### D3 — 본값 색은 임계값 규칙이 이긴다

`threshold_color_rules` 는 **값의 크기**로 색을 정하는 조건부 규칙이고, `font_color` 는 조건이 걸리지 않을 때의 기본색이다. 조건부가 기본을 덮는 것이 자연스러운 순서다.

반대로 하면 임계값 규칙이 죽는다 — 색을 한 번 지정한 순간 값이 위험 구간에 들어가도 색이 바뀌지 않아, 규칙을 설정한 사람이 알아채지 못한다.

### D4 — 변화량 팝오버의 색은 방향별 3색이다

변화량의 색은 SPEC-CHART-003 §2.2 에서 **방향**(증가/감소/변화없음)이 정한다. 여기에 정적 글자색을 더하면 어느 쪽이 이기는지 매번 물어야 한다.

그래서 `delta_layout` 에는 `font_color` 를 두지 않고, 팝오버의 색 칸 자리에 기존 `delta_display` 의 3색을 낸다. 사용자가 보기에는 "색을 여기서도 고칠 수 있다" 로 같고, 데이터에는 축이 하나만 남는다.

### D5 — `TextStyleFields` 를 별도 모듈로 뺀다

패널 컴포넌트가 `ChartPanelSections.tsx`(약 3,900줄)에서 import 하면 설정 모듈 전체가 패널 번들 청크로 딸려 온다. 순환 참조는 아니지만(설정 모듈은 패널을 import 하지 않는다) 번들 방향이 뒤집힌다 — 대시보드만 여는 사용자가 설정 다이얼로그 코드를 내려받게 된다.

`TextStyleFields` 와 그것이 쓰는 두 헬퍼(`LabeledField` · `inputClass`)를 `textStyleFields.tsx` 로 옮기고, `ChartPanelSections` 는 거기서 import 한다. 기존 호출부는 그대로 둔다(재export 로 이름을 유지).

### D10 — 무리 이동은 함께 멈춘다

요소마다 따로 상한을 적용하면, 상한에 먼저 닿은 것만 멈추고 나머지는 계속 가서 **대형이 무너진다.** 되돌리려면 각자를 다시 맞춰야 하는데, 그것은 "함께 옮긴다" 가 약속한 바가 아니다.

그래서 이동량을 무리 전체가 갈 수 있는 만큼으로 먼저 죈 뒤(`clampGroupDelta`) 모두에게 같은 값을 더한다. 한 요소가 못 가면 다 같이 멈추고, 상대 배치는 언제나 유지된다.

같은 이유로 **격자 스냅도 잡은 요소가 정한다.** 요소마다 따로 붙이면 저마다 다른 자리에 붙어 무리가 흐트러진다.

혼자 끌 때도 길이 1 인 무리로 다룬다 — 갈래를 나누면 두 경로가 서로 다르게 죄어져 "혼자일 때와 여럿일 때 상한이 다르다" 가 된다.

### D8 — 그리드는 보기와 스냅을 겸하고, Alt 로 끈다

참조선만 있는 그리드는 눈대중을 돕는 데 그친다. 붙이기까지 하면 같은 자리를 반복해서 맞출 수 있다 — 히트맵 센서 배치가 이미 [`applySnap`](../../../web/src/pages/dashboard/panels/heatmap/placement.ts) 으로 같은 선택을 했다.

다만 붙임만 있으면 격자 사이의 자리를 고를 수 없다. `Alt` 를 누르고 있는 동안만 끄는 방식이 그 둘을 함께 만족시킨다 — 별도 토글을 두면 켜 둔 것을 잊고 "왜 안 붙지" 가 된다.

**스냅은 오프셋이 아니라 절대 자리에 건다.** 오프셋을 10% 단위로 죄면 흐름상 시작 자리가 격자에 놓여 있지 않은 요소는 영영 격자에 맞지 않는다 — 보조 줄 둘이 그렇다.

**되짚는 기준은 잡는 순간의 오프셋이다.** 화면에서 잰 상자에는 그때의 오프셋이 이미 반영되어 있으므로, 새 오프셋으로 되짚으면 이동량이 두 번 반영되어 엉뚱한 자리에 붙는다(구현 중 실제로 그렇게 틀렸다).

### D9 — 정렬은 바깥 상자 기준이고, 겹침을 막지 않는다

"요소끼리 맞춤" 을 "가장 왼쪽 요소에 맞춘다" 로 정의하면 어느 것이 기준인지 예측하기 어렵고, 세 방식(앞/가운데/뒤)이 서로 다른 계산이 된다. **바깥 상자**를 기준으로 두면 셋이 한 계산으로 풀리고 디자인 도구의 관례와도 같다.

세로 정렬은 세로로 쌓인 세 요소를 겹치게 만든다. 막지 않는다 — "한 줄에 맞추라" 는 것이 그 조작의 뜻이고, 겹친 배치가 의도인 경우도 있다(큰 본값 위에 작은 변화량). 대신 **배치 초기화**를 같은 줄에 둔다. 되돌릴 수 없는 조작만 남기면 사용자가 눌러 보지 못한다.

### D7 — 오프셋은 `left`/`top` 이고 상한은 통계 전용 ±50 이다

구현 뒤 브라우저 확인에서 두 증상이 함께 보고됐다: "마우스 이동의 절반 속도로 움직인다" 와 "패널 일부 영역에서만 움직인다". 원인이 각각 다르다.

**속도** — `transform: translate(X%, Y%)` 의 백분율은 **변환되는 요소 자신의 border box** 기준이다. 그런데 저장 값은 **패널 상자** 기준으로 계산했다. 요소가 패널 폭의 절반이면 `translate(10%)` 는 패널의 5%만 움직인다 — 요소 크기에 비례해 속도가 달라진다.

상대 위치의 `left` / `top` 은 백분율이 **담는 상자** 기준이다(그리고 `top` 은 담는 상자의 높이 기준이다). 저장 값의 뜻과 정확히 일치하고, 흐름에서 빠지지 않으므로 형제 요소의 배치도 건드리지 않는다.

**범위** — 게이지·파이의 `PANEL_OFFSET_LIMIT`(±40)을 재사용한 것이 잘못이었다. 그 상수의 근거는 "40 을 넘기면 그림이 영역 가장자리에 붙어 절반 이상이 잘린다" 인데, 이는 **패널을 채우는 그림**의 사정이다. 통계 요소는 흐름상 가운데에서 시작하는 작은 글자 덩어리라, 패널의 어느 모서리에든 놓으려면 각 축으로 50%가 필요하다.

그래서 `STAT_OFFSET_LIMIT = 50` 을 통계 전용으로 둔다. 상수를 공유하지 않는 이유가 여기 적혀 있어야, 다음에 "왜 게이지와 다르지" 라는 물음이 코드만 보고도 풀린다.

### D6 — 실제 패널이 자기를 감싼다

합성 미리보기용 별도 레이어(`ChartDragLayer` 방식)를 만들지 않는다. 게이지·파이가 이미 "실제 패널이 자기를 감싼다" 로 갔고, 그래야 미리보기와 대시보드가 같은 코드로 자리를 계산한다. 두 벌로 두면 "설정에서 맞춘 자리가 대시보드에서 다르다" 가 다시 생긴다.

---

## 6. 범위 밖 (Out of Scope)

- **다중 타일 경로의 배치 편집** — §5 D2.
- **타이틀의 배치 편집** — 타이틀은 패널 크롬이며 이미 자체 디자인 팝오버(`title_font`)를 갖는다.
- **게이지 · 바 · 파이 패널로의 확장** — 본 SPEC 은 stat 만 다룬다. 여기서 만든 드래그 레이어가 재사용 가능한 모양이면 후속에서 잇는다.
- **정렬(`align`) 편집** — `TextStyleFields` 가 지원하지만, 오프셋으로 자리를 직접 정하는 이 화면에서는 정렬이 무엇을 기준으로 하는지 모호하다.
- **되돌리기(undo)** — 드래그 조작의 실행 취소는 패널 설정 전반의 문제이며 별개다.
- **스냅/가이드선** — 요소를 가운데나 서로에 맞춰 붙이는 보조선. 필요해지면 후속.

---

## 7. 열린 질문 (Open Questions)

| # | 질문 | 잠정 결정 |
|---|------|-----------|
| OQ1 | 크기 핸들을 끌 때 픽셀 이동량을 px 증가량으로 어떻게 환산할까 | **대각선 투영**(`(dx+dy)/2`)을 px 에 더한다 — 손잡이를 끈 거리만큼 글자가 커진다. 처음에는 `dx+dy` 를 그대로 더했으나 대각선으로 끌 때 포인터보다 두 배 빨리 커져 조작이 튄다는 보고를 받고 정정했다. 소수 px 을 반올림하지 않는다 — 정수로 죄면 천천히 움직여도 크기가 계단으로 뛴다 |
| OQ2 | 세 요소가 서로 겹치면 어떻게 할까 | 막지 않는다. 겹치는 배치가 의도일 수 있고(예: 큰 본값 위에 작은 변화량), 자동으로 밀어내면 끈 자리와 다른 곳에 놓인다. 편집 중에는 점선 상자가 겹침을 보여 준다 |
| OQ3 | 편집 중 요소가 패널 밖으로 나가면 | ±40% 클램프가 이미 절반 이상 잘리는 것을 막는다. 그 이상은 막지 않는다 — 되돌릴 수 있고, 막으면 좁은 패널에서 옮길 수 있는 범위가 사라진다 |
| OQ4 | 팝오버를 패널 안에 그릴까 `document.body` 에 포털할까 | 포털한다. 패널 안에 그리면 `overflow: hidden` 과 낮은 z-index 에 잘린다(`DesignPopover` 와 같은 판단) |

---

## 8. 구현 노트 (Implementation Notes)

2026-09-08 대조 시점 기준. 구현은 작업 트리에 있고 아직 커밋되지 않았다. **`status` 는 `completed` 다** — v0.6.0 이 남긴 미충족 1건이 해소되었고, 완료를 막던 문서 공백(IN-4)도 채웠다.

### 해소 (v0.6.0 이 미충족으로 적었던 것)

- **AC-23 후반절 — 닫으면 포커스가 원래 요소로 돌아온다. 충족.** `StatElementStylePopover.tsx:101` 이 팝오버 안 첫 입력으로 포커스를 옮기기 **전에** `document.activeElement` 를 `opener` 로 붙든다. 같은 effect 의 정리 함수(`:106`, 곧 닫힘 시점)가 `opener instanceof HTMLElement && opener.isConnected` 를 확인한 뒤 `opener.focus()` 로 되돌린다. `isConnected` 가드는 그 사이 요소가 화면에서 사라진 경우(패널 재구성·항목 삭제)에 없는 자리를 붙잡지 않기 위한 것이다. 단언은 두 갈래로 나뉜다 — 정상 경로는 `StatElementStylePopover.test.tsx:203` "닫으면 포커스를 원래 요소로 되돌린다"(앵커에 포커스를 준 뒤 `unmount` 하면 포커스가 앵커로 돌아옴), 앵커 소멸 경로는 `:225` "열었던 요소가 사라졌으면 되돌리지 않는다"(앵커를 `remove()` 한 뒤 `unmount` 가 던지지 않음)가 잠근다. AC-23 전반절("Enter 로도 팝오버가 열린다")은 종전대로 `StatPanel.test.tsx` 가 잠근다. plan.md M5.4 가 적은 "포커스 이동·**복귀**" 는 이로써 계획대로 충족되었다.

- **IN-4 의 문서 공백 — 채웠다.** v0.6.0 이 "`completed` 로 넘기기 전에 acceptance.md 를 그 번호대로 채워야 한다" 고 적어 둔 조건에 따라, acceptance.md 에 **§I(AC-33~AC-41, AC-39 결번)** 을 더해 v0.4.0~v0.5.0 이 넓힌 §2.7(U7 그리드·중심 표식) · §2.8(U8 정렬) · §2.9(U9 선택)의 인수 조건을 명문화했다. 번호는 이미 테스트 이름에 박혀 있던 것을 그대로 받아 적었고(재배치하면 문서와 코드가 갈라진다), 그 절이 **구현 뒤에 쓰인 사후 대조 기록**이라는 사실을 절 머리에 밝혀 두었다.

### 참고 — v0.6.0 이 적었던 원문 (해소 전 상태)

- ~~**AC-23 후반절 — 팝오버를 닫아도 포커스가 원래 요소로 돌아오지 않는다.**~~ 전반절("Enter 로도 팝오버가 열린다")은 `StatPanel.test.tsx` 가 잠근다. 후반절은 구현도 테스트도 없다. `StatElementStylePopover.tsx:97` 은 열릴 때 첫 입력으로 포커스를 옮기지만, 닫힐 때 앵커로 되돌리는 경로가 없어 포커스가 `document.body` 로 떨어진다. 키보드만 쓰는 사용자에게는 팝오버를 한 번 닫을 때마다 편집하던 자리를 잃는다는 뜻이다. `usePanelElementEdit.tsx` · `PanelResizeOverlay.tsx` · `StatPanel.tsx` 어디에도 복귀 코드가 없음을 확인했다. plan.md M5.4 는 "바깥 클릭 / `Esc` 로 닫기, 포커스 이동·**복귀**(§4.4)" 로 적었으므로 계획 대비 누락이다.

### 충족 근거 (AC-01~AC-32, AC-10 결번 — 전량)

| 묶음 | 충족 근거 |
|------|-----------|
| A. 편집 게이팅 (AC-01~AC-05) | `StatPanel.test.tsx` 의 AC-01/02/03/04/05 명시 테스트 5건. `grep -n "editing" StatPanel.tsx` 무출력 — 편집 상태가 config 로 새지 않는다 |
| B. 배치 (AC-06~AC-09) | `chartChannelTypes.ts:761` `StatElementLayout` + `:786`~`:789` 3필드. `PanelDragLayer.test.tsx` 가 요소별 독립 갱신 3건 · 백분율 환산 · ±50 클램프 · 여백 무시를 단언한다. `left`/`top` 사용(AC-08b)은 `StatSubLines.test.tsx` 의 "담는 상자 기준으로 해석되는 left/top 을 낸다 — transform 이 아니다" 가 잠근다 |
| C. 크기 (AC-11~AC-15) | `statLayout.test.ts` 23건(폴백 순서 · `clampFontSize` 6~160 · 타일 기본 24px). 크기 핸들의 대각선 투영과 소수 px 보존은 `PanelDragLayer.test.tsx` 가, 디바운스 건너뛰기(AC-12c)는 `previewLiveKeys.test.ts` 가 단언한다 |
| D. 글자 스타일 (AC-16~AC-22) | `StatElementStylePopover.test.tsx` 19건 — 글꼴·크기·색·굵기 개별 저장, 방향별 3색 분기, 경계 회피, 바깥 클릭·Esc 닫기 |
| D. 키보드 접근 (AC-23) | **충족.** 전반절은 `StatPanel.test.tsx` "Enter 로도 팝오버가 열린다". 후반절은 구현 `StatElementStylePopover.tsx:101`(열기 전 `document.activeElement` 를 `opener` 로 붙듦) + `:106`(닫힘 정리에서 `opener instanceof HTMLElement && opener.isConnected` 확인 후 `focus()`)와 테스트 `StatElementStylePopover.test.tsx:203` "닫으면 포커스를 원래 요소로 되돌린다" · `:225` "열었던 요소가 사라졌으면 되돌리지 않는다" |
| E. 적용 경로 (AC-24~AC-25) | `StatPanel.multiOutput.test.tsx` — "타일 경로에는 편집 입구가 없고 저장된 layout 도 읽지 않는다" 한 건이 둘을 함께 잠근다 |
| F. 하위 호환 (AC-26~AC-28) | 특성화 전량 통과. `TextStyleFields` 는 `textStyleFields.tsx` 가 소유하고 `ChartPanelSections.tsx:70,72` 가 import 후 재export 한다. 패널 → 설정 방향 의존은 없다(grep 무출력) |
| G. 설정 섹션 (AC-29~AC-30) | `ChartPanelSections.test.tsx:1554`·`:1559` 가 font_size 우선 안내의 유무를 양쪽으로 잠근다 |
| H. 통합 (AC-31) | `npm run build` · `npm test` · `npm run lint` 모두 exit 0 |
| I. 편집 보조 (AC-33~AC-41, AC-39 결번) | 사후 명문화(acceptance.md §I). 그리드·중심 표식·중심 기준 스냅은 `StatPanel.test.tsx`(AC-33 3건) · `PanelDragLayer.test.tsx`("격자 스냅 (AC-33)" 4건) · `panelEditAlign.test.ts`(`snapOffsetToGrid` 6건). 정렬 계산은 `panelEditAlign.test.ts`(`computeAlignPatches` 8건, AC-34), 툴바·초기화는 `StatPanel.test.tsx`(AC-35/AC-36). 선택 전이·무리 이동은 `panelEditSelection.test.ts`(AC-37 5건 / AC-38 6건) + `StatPanel.test.tsx`(AC-37 4건) + `PanelDragLayer.test.tsx`(AC-38 2건). 스냅 토글의 비저장은 `StatPanel.test.tsx`(AC-40 2건), 미리보기 실제 드래그는 `PanelSettingsDialog.previewEdit.test.tsx`(AC-41) |

### AC-32 — 커버리지 충족 (계측 경로는 2026-09-08 복구)

허용목록을 우회해 직접 계측하면 `statLayout.ts` 100% · `StatElementStylePopover.tsx` 100% · `PanelDragLayer.tsx` 96.82% 로 세 파일 모두 목표 85% 를 넘는다(수치는 statement 기준 — 지표 판정 근거는 SPEC-CHART-005 §8 "M7.4 판정 지표" 참고).

**추기(2026-09-08) — 계측 경로가 복구되었다.** 아래 IN-2 가 적은 `vite.config.ts` 허용목록의 옛 이름(`StatDragLayer.tsx`)이 제거되고 실제 모듈이 등재되어, 이제 표준 경로로도 같은 수치가 나온다. 전체 `--coverage` 실행에서 `PanelDragLayer.tsx` 96.82% · `StatElementStylePopover.tsx` 100% · `statLayout.ts` 100% 를 확인했다. 다만 **acceptance.md AC-32 의 명령문에 적힌 테스트 파일 이름 `StatDragLayer.test.tsx` 는 여전히 존재하지 않는다**(실제 파일은 `PanelDragLayer.test.tsx` — IN-1 의 문서 지연). 명령을 글자 그대로 실행하려면 그 이름을 고쳐야 한다.

### 분기 (Divergence, as-implemented)

- **IN-1 — 드래그 레이어가 `StatDragLayer.tsx` 가 아니라 `PanelDragLayer.tsx` 로 태어났다.** plan.md M4.1 은 통계 전용 `StatDragLayer.tsx` 를, M2.5 에 해당하는 공용화는 후속 SPEC-CHART-005 로 미뤄 두었다. 실제로는 두 SPEC 이 같은 작업 트리에서 이어 붙어, 통계 전용 이름으로 커밋된 적 없이 곧바로 공용 이름으로 남았다. 마찬가지로 `statAlign.ts` → `panelEditAlign.ts`, `statSelection.ts` → `panelEditSelection.ts`, `StatEditGrid` → `PanelEditGrid`, `StatAlignToolbar` → `PanelAlignToolbar` 도 중간 이름이 존재하지 않는다. 그 결과 acceptance.md 가 지목한 `StatDragLayer.test.tsx` 는 없고 `PanelDragLayer.test.tsx` 가 같은 단언을 담고 있다. 파일 이름만 다르고 검증 내용은 일치한다.
- **IN-2 — `vite.config.ts` 커버리지 허용목록이 옛 이름에 머물러 있다.** 목록은 여전히 `src/pages/dashboard/StatDragLayer.tsx` 를 가리키고, 그 파일은 존재하지 않는다. 목록이 **명시 허용목록**이므로 `PanelDragLayer.tsx` 는 오류 없이 0건으로 빠지고, AC-32 의 명령을 그대로 실행하면 그 행이 아예 나오지 않는다. 같은 이유로 `PanelAlignToolbar.tsx` · `PanelEditGrid.tsx` · `PanelGridBackdrop.tsx` · `PanelResizeOverlay.tsx` · `usePanelElementEdit.tsx` · `previewStage.ts` · `previewGridSize.ts` 도 계측 밖이다. 그 목록에 달린 주석이 "적지 않으면 조용히 0건으로 빠진다" 고 이미 경고해 둔 함정에 정확히 걸렸다. **해소(2026-09-08)**: 허용목록에서 옛 이름을 걷어내고 `PanelDragLayer.tsx` · `PanelAlignToolbar.tsx` · `PanelEditGrid.tsx` · `PanelGridBackdrop.tsx` · `PanelResizeOverlay.tsx` · `usePanelElementEdit.tsx` · `panelEditContext.ts` · `previewStage.ts` · `previewGridSize.ts` 를 등재했다. 실측 결과와 지표 판정은 SPEC-CHART-005 §8 에 있다(두 SPEC 이 같은 목록을 공유한다).
- **IN-3 — plan.md §2 가 예고하지 않은 파일이 생겼다.** `PanelResizeOverlay.tsx` · `PanelGridBackdrop.tsx` · `panelEditContext.ts` · `previewStage.ts` · `previewGridSize.ts` 는 계획 표에 없다. 이 중 `PanelResizeOverlay.tsx` · `PanelGridBackdrop.tsx` · `panelEditContext.ts` · `previewStage.ts` · `previewGridSize.ts` 는 `@spec` 주석도 달고 있지 않아 어느 SPEC 소유인지 코드만으로는 판정할 수 없다.
- **IN-4 — acceptance.md 가 spec.md v0.4.0~v0.5.0 을 따라가지 못했다. (v1.0.0 에서 해소)** §2.7(그리드·중심 표식) · §2.8(정렬 툴바) · §2.9(선택 모델)에 대응하는 인수 조건이 acceptance.md 에 없었고(AC-32 에서 끝났다), 구현 쪽에는 이미 `AC-33`~`AC-38` · `AC-40` · `AC-41` 을 이름에 단 테스트가 있었다 — 번호를 코드가 먼저 쓰고 문서가 따라가지 못한 상태였다. v1.0.0 에서 acceptance.md 에 **§I** 를 더해 그 번호대로 채웠다. 그 절은 구현 뒤에 쓰인 **사후 대조 기록**이며, 그 사실을 절 머리에 명시했다. `AC-39` 는 어느 테스트에도 없어 `AC-10` 과 마찬가지로 결번이다.
- **IN-5 — AC-19 의 grep 단언이 잘못 적혀 있다.** `grep -n "font_color" chartChannelTypes.ts | grep -i delta` 가 "출력 없음" 을 기대하지만, 실제 코드는 `delta_layout?: Omit<StatElementLayout, 'font_color'>;`(`:788`)로 **타입 수준에서 font_color 를 제외**하고 있어 그 줄이 grep 에 걸린다. 뜻은 정확히 AC-19 가 요구하는 바이고 동작도 `StatElementStylePopover.test.tsx` 가 잠근다 — 단언 문구가 구현 방식을 예상하지 못했을 뿐이다. 문서 쪽을 고쳐야 한다.
- **IN-6 — 드래그 표식 이름이 바뀌어 AC-01 의 단언 문구가 옛 이름에 머물러 있다.** acceptance.md AC-01 은 `stat-resize-value` · `[data-stat-drag]` 의 부재를 단언하라고 적었으나, 후속 SPEC-CHART-005 의 공용화로 표식이 `[data-panel-drag]` · `panel-resize-value` 로 바뀌었다(`grep -rn "data-stat-drag\|data-stat-resize" web/src/` 무출력). 실제 테스트(`StatPanel.test.tsx` AC-01)는 새 이름으로 같은 것을 단언하므로 검증 내용은 일치하고, 문구만 지연된 상태다. IN-1 과 같은 성격의 문서 지연이다.
