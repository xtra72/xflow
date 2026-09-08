# SPEC-CHART-004 인수 조건

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)
형식: Given-When-Then + 기계 검증 가능한 단언(vitest 파일·테스트명, 또는 grep/tsc 명령과 기대 출력)

> **추기(2026-09-08) — 실행 지시문의 이름을 실제 이름으로 맞췄다. 판정 내용은 바뀌지 않았다.**
>
> 이 문서의 인수 조건은 "이 파일의 이 테스트를 보라", "이 표식의 부재를 단언하라" 처럼 **그대로 실행할 수 있는 지시문**을 담는다. 그런데 그 이름 일부가 실제로 존재한 적 없는 것을 가리키고 있었다.
>
> - `StatDragLayer.test.tsx`(5곳: AC-07 · AC-08 · AC-09 · AC-13 · AC-32 의 명령 인자) → **`PanelDragLayer.test.tsx`**. spec.md §8 IN-1 이 적었듯 통계 전용 이름으로 커밋된 적이 없어, 지목된 파일은 처음부터 존재하지 않았다. 같은 단언을 담은 실제 파일로 바꿨다.
> - AC-32 기대 출력의 `StatDragLayer.tsx` → **`PanelDragLayer.tsx`**(측정 대상 모듈의 실제 이름).
> - AC-01 단언의 `stat-resize-value` · `[data-stat-drag]` → **`[data-panel-drag]` · `[data-panel-resize]`**(IN-6). 후속 SPEC-CHART-005 의 공용화로 표식 이름이 바뀌었고, 옛 이름의 부재를 단언하면 **무엇을 검사하든 항상 통과하는 빈 단언**이 된다. 뜻대로 검사하도록 실제 표식으로 고쳤다. 같은 줄의 `stat-edit-toggle` 은 **바꾸지 않았다** — 통계 패널 자신의 토글이고 지금도 그 이름이다.
> - 2026-09-08 에 실행된 심볼 개명(`StatDragTarget`→`PanelDragTarget` · `StatResizeHandle`→`PanelResizeHandle` · `StatDragLayer`→`PanelDragLayer` · `StatSelection`→`PanelSelection` · `StatElementBox`→`PanelElementBox`, SPEC-CHART-005 §8 IN-1 의 부채 해소)을 반영한 것이기도 하다.
>
> **바꾼 것은 이름뿐이다.** Given/When/Then 문장, 기대값, 검사 대상, 조건의 수와 번호는 하나도 손대지 않았다. 조건을 슬그머니 고쳐 쓴 것이 아니라, 가리키는 대상이 어긋나 있던 것을 맞춘 것이다. plan.md 와 spec.md 의 HISTORY·IN 항목은 **작성 시점의 기록이므로 고치지 않았다**.

## 공통 픽스처

**F1 — 채널 모드 entries** (`display_field: 'value'`)

| # | value |
|---|-------|
| e0 | 20 |
| e1 | 24 |

기대치: 본값 `24` · 변화량 `+4`(↑) · 평균 `22` · 최대 `24` · 최소 `20`

**F2 — 편집 상태 3종**

| 상태 | props | 기대 |
|------|-------|------|
| 읽기 전용 | `onConfigChange` 없음 | 편집 입구 없음 |
| 미리보기 | `onConfigChange` + `forceEdit` | 항상 편집, 토글 없음 |
| 대시보드 | `onConfigChange`, `dashboardEditMode: true` | 토글 노출, 눌러야 편집 |

---

## A. 편집 게이팅 (U4)

### AC-01 — onConfigChange 가 없으면 편집 입구가 없다

```gherkin
Given F1 데이터와 onConfigChange 를 넘기지 않은 stat 패널이 있을 때
When 렌더하면
Then 크기 핸들·편집 토글·드래그 표식이 하나도 존재하지 않는다
```

vitest: `StatPanel.test.tsx` — "onConfigChange 가 없으면 편집 입구가 없다"
단언: `stat-edit-toggle` · `[data-panel-drag]` · `[data-panel-resize]` 모두 부재

### AC-02 — 미리보기는 항상 편집이고 토글을 내지 않는다

```gherkin
Given onConfigChange + forceEdit 를 넘긴 stat 패널이 있을 때
When 렌더하면
Then 드래그 표식과 크기 핸들이 존재하고, 편집 토글 버튼은 존재하지 않는다
```

vitest: `StatPanel.test.tsx` — "forceEdit 는 토글 없이 편집을 켠다"

### AC-03 — 대시보드는 편집모드 + 토글 2겹을 거친다

```gherkin
Given onConfigChange 를 넘기고 dashboardEditMode 가 참인 stat 패널이 있을 때
When 렌더하면
Then 편집 토글이 보이고 드래그 표식은 아직 없다
And 토글을 누르면 드래그 표식과 크기 핸들이 나타난다
```

vitest: `StatPanel.test.tsx` — "대시보드는 편집모드 + 토글 2겹을 거쳐야 켜진다"

### AC-04 — 편집모드를 벗어나면 오버레이가 사라진다

```gherkin
Given 대시보드 편집모드에서 토글을 눌러 편집 중인 stat 패널이 있을 때
When dashboardEditMode 가 거짓이 되면
Then 드래그 표식과 크기 핸들이 사라진다
```

vitest: `StatPanel.test.tsx` — "편집모드를 벗어나면 배치편집이 해제된다"

### AC-05 — 편집 상태는 config 에 저장되지 않는다 (U4-4)

```bash
grep -n "editing" web/src/pages/dashboard/panels/charts/StatPanel.tsx
# 기대: onConfigChange 인자에 편집 상태를 싣는 코드가 없다
```

vitest: `StatPanel.test.tsx` — "토글을 눌러도 onConfigChange 가 호출되지 않는다"

---

## B. 배치 (U1)

### AC-06 — 타입이 config 에 존재한다

```bash
grep -n "StatElementLayout\|value_layout\|delta_layout\|stats_layout" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts
# 기대: StatElementLayout 인터페이스 + StatPanelConfig 의 3필드
```

`npx tsc --noEmit` — 오류 0

### AC-07 — 세 요소가 독립적으로 움직인다

```gherkin
Given 편집이 켜진 stat 패널에서 본값을 오른쪽으로 끌었을 때
When onConfigChange 가 호출되면
Then value_layout.offset_x 만 바뀌고 delta_layout · stats_layout 은 인자에 없다
```

각 요소(본값 / 변화량 / 구간 통계)에 대해 같은 단언을 둔다.

vitest: `PanelDragLayer.test.tsx` — "본값을 끌면 value 대상만 갱신된다" 외 2건

### AC-08 — 오프셋은 백분율이며 ±50 으로 죈다 (U1-3 / U1-4)

```gherkin
Given 폭 200px 인 패널에서 본값을 오른쪽으로 20px 끌었을 때
When 오프셋을 계산하면
Then 10 (백분율 포인트) 이다
And 계속 끌어 50 을 넘기려 하면 50 에서 멈춘다 — 게이지·파이의 40 이 아니다
```

vitest: `PanelDragLayer.test.tsx` — "픽셀 이동량을 기준 상자 대비 백분율로 환산한다" · "±50 으로 죈다"
vitest: `statLayout.test.ts` — "지정한 값을 읽고 ±50 으로 죈다"

### AC-08b — 오프셋은 left/top 으로 편다 (U1-3b / D7)

```gherkin
Given value_layout: { offset_x: 10, offset_y: -5 } 인 stat 패널이 있을 때
When 렌더하면
Then position:relative + left:10% + top:-5% 이고 transform 은 비어 있다
And 오프셋이 0 이면 위치 속성이 붙지 않는다
```

transform 의 백분율은 요소 자신의 크기 기준이라, 요소가 작을수록 느리게 움직인다
(마우스 이동의 절반 속도로 보고된 증상).

vitest: `StatSubLines.test.tsx` — "담는 상자 기준으로 해석되는 left/top 을 낸다 — transform 이 아니다" · "보조 줄이 실제로 left/top 을 쓴다"
vitest: `StatPanel.test.tsx` — "저장된 오프셋이 상대 위치로 반영된다" · "오프셋이 0 이면 위치 속성을 붙이지 않는다"

### AC-09 — 요소 밖을 끌면 아무 일도 없다 (U1-5)

```gherkin
Given 편집이 켜진 stat 패널에서 빈 여백을 끌었을 때
When 포인터를 움직이면
Then onConfigChange 가 호출되지 않는다
```

vitest: `PanelDragLayer.test.tsx` — "표식 없는 자리를 잡으면 무시한다"

---

## C. 크기 (U2)

### AC-11 — font_size 가 배율을 이긴다 (§3.2)

```gherkin
Given value_scale: 2 이고 value_layout.font_size: 50 인 stat 패널이 있을 때
When 본값의 fontSize 를 읽으면
Then 50px 이다 (36 × 2 = 72 도 아니고 50 × 2 = 100 도 아니다)
```

vitest: `statLayout.test.ts` — "font_size 는 배율을 이기고 곱해지지 않는다"

### AC-12 — font_size 미지정은 종전 계산이다 (U2-5)

```gherkin
Given value_layout 이 없고 value_scale: 2 인 stat 패널이 있을 때
When 본값의 fontSize 를 읽으면
Then 72px 이다 (36 × 2)
And 보조 줄은 sub_value_scale 기본 1 에서 14px 이다
```

vitest: `statLayout.test.ts` — "미지정이면 기본 px × 배율" (본값 · 변화량 · 구간통계 3건)

### AC-12b — 편집이 켜지면 잡을 수 있는 자리가 보인다 (U2-2b)

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 세 요소를 검사하면
Then 각각 점선 윤곽 클래스(outline-dashed)를 갖고 크기 손잡이가 하나씩 있다
And 편집이 꺼져 있으면 윤곽도 손잡이도 없다
```

vitest: `StatPanel.test.tsx` — "편집을 켜면 잡을 수 있는 자리가 화면에 보인다" · "편집이 꺼져 있으면 윤곽도 손잡이도 없다"

### AC-12c — layout 키가 미리보기 디바운스를 건너뛴다 (U4-6)

```gherkin
Given previewLiveKeys 의 PREVIEW_LIVE_KEYS 를 검사할 때
When 통계 layout 키를 찾으면
Then value_layout · delta_layout · stats_layout 이 모두 들어 있다
And mergeLivePreviewConfig 가 그 키만 최신 draft 값으로 덮는다
```

vitest: `previewLiveKeys.test.ts` — "통계 패널 세 요소의 layout 이 디바운스를 건너뛴다" · "통계 layout 은 최신 draft 값이 즉시 반영된다"

### AC-13 — 크기 핸들이 font_size 를 바꾼다 (U2-3)

```gherkin
Given 편집이 켜진 stat 패널에서 본값 크기 핸들을 오른쪽 아래로 끌었을 때
When onConfigChange 가 호출되면
Then value_layout.font_size 가 대각선 투영((dx+dy)/2)만큼 커진다
And 반대로 끌면 작아진다
And 소수 px 이 반올림되지 않는다 — 정수로 죄면 크기가 계단으로 뛴다
```

vitest: `PanelDragLayer.test.tsx` — "크기 핸들을 오른쪽 아래로 끌면 커진다" · "반대로 끌면 작아진다" · "소수 px 을 그대로 넘긴다"

### AC-14 — font_size 는 6~160 으로 죈다 (U2-4)

```gherkin
Given clampFontSize 에 각각 1 / 500 / 'x' / undefined 를 넘겼을 때
When 결과를 읽으면
Then 각각 6 / 160 / undefined / undefined 다
```

vitest: `statLayout.test.ts` — "clampFontSize 는 6~160 으로 죈다"

### AC-15 — 단위 글자가 본값과 함께 커진다 (U2-6)

```gherkin
Given value_layout.font_size: 72 이고 단위가 지정된 stat 패널이 있을 때
When 렌더하면
Then 단위 글자의 fontSize 가 종전 비율(20/36)을 유지한 40px 이다
```

vitest: `StatPanel.test.tsx` — "단위는 본값과 같은 비율로 커진다"

---

## D. 글자 스타일 (U3)

### AC-16 — 더블클릭으로 팝오버가 열린다

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 본값을 더블클릭하면
Then 글자 스타일 팝오버가 열린다
And 편집이 꺼져 있으면 더블클릭해도 열리지 않는다
```

vitest: `StatPanel.test.tsx` — "편집 중 더블클릭하면 스타일 팝오버가 열린다" · "편집이 꺼져 있으면 열리지 않는다"

### AC-17 — 글꼴·크기·색·굵기를 편집한다 (U3-2)

```gherkin
Given 본값 팝오버가 열려 있을 때
When 글꼴을 serif 로 바꾸면
Then value_layout.font_family 가 'serif' 로 저장된다
```

크기 · 색 · 굵기에 대해서도 같은 단언을 둔다.

vitest: `StatElementStylePopover.test.tsx` — "글꼴/크기/색/굵기를 각각 저장한다"

### AC-18 — 팝오버 크기 칸과 핸들이 같은 필드를 쓴다 (U3-3)

```gherkin
Given value_layout.font_size: 50 인 상태에서 팝오버를 열었을 때
When 크기 칸을 읽으면
Then 50 이 표시된다
And 칸에 80 을 입력하면 value_layout.font_size 가 80 이 된다
```

vitest: `StatElementStylePopover.test.tsx` — "크기 칸은 핸들과 같은 font_size 를 편집한다"

### AC-19 — 변화량 팝오버는 방향별 3색을 낸다 (U3-4 / D4)

```gherkin
Given 변화량 요소의 팝오버가 열려 있을 때
When 색 영역을 검사하면
Then 증가·감소·변화없음 3개의 색 입력이 있고, 단일 글자색 입력은 없다
And 증가 색을 바꾸면 delta_display.up_color 가 저장된다 (delta_layout 이 아니다)
```

vitest: `StatElementStylePopover.test.tsx` — "변화량은 방향별 3색을 편집한다"

```bash
grep -n "font_color" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts | grep -i delta
# 기대: 출력 없음 (exit 1) — delta_layout 에는 font_color 가 없다
```

### AC-20 — 본값은 임계값 색이 font_color 를 이긴다 (U3-5 / D3)

```gherkin
Given value_layout.font_color: '#123456' 이고 threshold_color_rules 가 값에 걸리는 stat 패널이 있을 때
When 렌더하면
Then 본값 색은 임계값 규칙의 색이다
And 규칙에 걸리지 않는 값에서는 #123456 이다
```

vitest: `StatPanel.test.tsx` — "임계값 규칙이 기본 글자색을 이긴다"

### AC-21 — 팝오버가 화면 경계를 넘지 않는다 (U3-6)

```gherkin
Given 패널 오른쪽 끝의 요소에서 팝오버를 열었을 때
When 위치를 계산하면
Then 상자의 오른쪽 변이 잘라내는 영역 안에 들어온다
```

vitest: `StatElementStylePopover.test.tsx` — "경계를 넘지 않는 자리로 옮긴다"

### AC-22 — 바깥 클릭과 Esc 로 닫힌다 (U3-7)

vitest: `StatElementStylePopover.test.tsx` — "바깥을 클릭하면 닫힌다" · "Esc 로 닫힌다"

### AC-23 — 키보드로도 팝오버에 닿는다 (§4.4)

```gherkin
Given 편집이 켜진 stat 패널의 본값에 포커스가 있을 때
When Enter 를 누르면
Then 팝오버가 열린다
And 닫으면 포커스가 본값으로 돌아온다
```

vitest: `StatPanel.test.tsx` — "Enter 로도 팝오버가 열린다"
vitest: `StatElementStylePopover.test.tsx` — "닫으면 포커스를 원래 요소로 되돌린다"

---

## E. 적용 경로 (U5)

### AC-24 — 타일 경로에는 편집 입구가 없다

```gherkin
Given series_reduce 가 지정되고 시리즈가 3개인 stat 패널에 onConfigChange + forceEdit 를 넘겼을 때
When 렌더하면
Then 드래그 표식·크기 핸들이 존재하지 않는다
```

vitest: `StatPanel.multiOutput.test.tsx` — "타일 경로에는 요소 편집 입구가 없다"

### AC-25 — 타일 경로는 저장된 layout 을 무시하고 값은 보존한다 (U5-3)

```gherkin
Given value_layout: { offset_x: 20, font_size: 90 } 가 저장된 타일 경로 패널이 있을 때
When 렌더하면
Then 타일에 transform 오프셋이 걸리지 않고 크기도 종전 타일 기본(24 × value_scale)이다
```

vitest: `StatPanel.multiOutput.test.tsx` — "타일 경로는 layout 을 무시한다"

---

## F. 하위 호환 (§3.3)

### AC-26 — layout 이 없는 config 는 종전과 같은 화면을 낸다

기존 특성화 테스트 전량이 통과해야 한다.

```bash
npx vitest run src/pages/dashboard/panels/charts/StatPanel.test.tsx src/pages/dashboard/panels/charts/StatPanel.multiOutput.test.tsx src/pages/dashboard/panels/charts/StatPanelStoreSource.test.tsx
# 기대: 전량 통과 (SPEC-CHART-002 CH-01~CH-04 포함)
```

### AC-27 — TextStyleFields 추출이 동작을 바꾸지 않는다 (D5)

```bash
npx vitest run src/pages/dashboard/textStyleFieldsLayout.test.tsx
# 기대: 추출 전후 동일하게 통과

grep -n "TextStyleFields" web/src/pages/dashboard/ChartPanelSections.tsx
# 기대: 새 모듈에서 import 하고 재export 하는 형태
```

### AC-28 — 패널이 설정 모듈을 import 하지 않는다 (D5)

```bash
grep -rn "from './ChartPanelSections'\|from '../../ChartPanelSections'" web/src/pages/dashboard/panels/
# 기대: 출력 없음 (exit 1)
```

---

## G. 설정 섹션 (U6)

### AC-29 — 배율 슬라이더가 유지된다

```gherkin
Given stat 패널 설정 섹션을 렌더했을 때
When 검사하면
Then value_scale 과 sub_value_scale 슬라이더가 존재한다
```

vitest: `ChartPanelSections.test.tsx` — 기존 테스트가 그대로 통과

### AC-30 — font_size 가 지정되면 우선 안내가 뜬다 (U6-2)

```gherkin
Given value_layout.font_size 가 지정된 stat 패널의 설정 섹션이 있을 때
When 렌더하면
Then 직접 지정 크기가 우선한다는 안내가 표시된다
And font_size 가 없으면 안내가 없다
```

vitest: `ChartPanelSections.test.tsx` — "직접 지정 크기가 있으면 배율 우선 안내가 뜬다"

---

## H. 통합 (M8)

### AC-31 — 빌드·테스트·린트

```bash
cd web && npm run build   # 기대: exit 0
cd web && npm test        # 기대: exit 0
cd web && npm run lint    # 기대: exit 0
```

### AC-32 — 신규 파일 커버리지 85% 이상

```bash
cd web && npx vitest run --coverage \
  src/pages/dashboard/panels/charts/statLayout.test.ts \
  src/pages/dashboard/PanelDragLayer.test.tsx \
  src/pages/dashboard/StatElementStylePopover.test.tsx
# 기대: statLayout.ts · PanelDragLayer.tsx · StatElementStylePopover.tsx 모두 85% 이상
```

---

## I. 편집 보조 — 그리드·정렬·선택 (U7 / U8 / U9)

**이 절의 내력**: AC-01~AC-32 는 spec.md v0.1.0~v0.3.0 을 대상으로 계획 단계에 쓰였다.
그 뒤 v0.4.0(§2.7 그리드·중심 표식, §2.8 정렬 툴바)과 v0.5.0(§2.9 선택 모델)이 요구사항을 넓혔으나
인수 조건이 따라오지 못했고, 구현 쪽 테스트만 `AC-33`~`AC-41` 을 이름에 달았다(spec.md §8 IN-4).
**이 절은 2026-09-08 에 그 번호를 그대로 받아 사후에 채운 것이다** — 개발을 이끈 조건이 아니라,
이미 만들어진 것을 요구사항에 되짚어 대조한 기록이다. 번호는 테스트가 먼저 썼으므로 재배치하지 않는다.
`AC-39` 는 어느 테스트에도 없어 결번으로 둔다(`AC-10` 과 같다).

### AC-33 — 격자와 중심 표식, 그리고 중심 기준 스냅 (U7-1 / U7-2 / U7-3 / U7-4 / U7-5 / U7-6)

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 렌더하면
Then 기준 상자 안에 격자와 정중앙 + 표식이 보인다
And 편집이 꺼져 있으면 둘 다 없다
And 격자는 포인터를 받지 않는다 — 그 위를 지나는 드래그가 끊기면 안 된다
```

```gherkin
Given 격자 맞춤이 켜지고 폭 200px 상자 안에 중심이 50% 자리인 요소가 있을 때
When 6px(=3%p) 끌면
Then 요소의 **중심**이 가까운 격자 교점으로 되붙어 오프셋 0 이 된다
And 24px(=12%p) 끌면 10 으로 붙는다
And Alt 를 누른 채 끌면 격자를 잠시 꺼 3 이 그대로 전달된다
And 격자 맞춤이 꺼져 있으면 3 이 그대로다
```

vitest: `StatPanel.test.tsx` — "AC-33: 편집 중에는 그리드와 중심 표식이 보인다" ·
"AC-33: 편집이 꺼져 있으면 그리드도 중심 표식도 없다" · "그리드는 포인터를 받지 않는다 …"
vitest: `PanelDragLayer.test.tsx` — "격자 스냅 (AC-33)" 4건
vitest: `panelEditAlign.test.ts` — "snapOffsetToGrid — 중심을 격자에 맞춘다 (AC-33)" 6건
(격자 간격 10% · 잡는 순간의 오프셋으로 흐름 자리를 되짚음(U7-6) · 상한 ±50)

### AC-34 — 정렬 계산은 고른 것들의 바깥 상자를 기준으로 한다 (U8-3 / U8-4)

```gherkin
Given 오프셋이 제각각인 요소 여럿이 있을 때
When 정렬을 계산하면
Then start 는 가장 앞선 변, end 는 가장 뒤선 변, center 는 그 둘의 가운데에 맞춘다
And 세로 축은 offsetY 만 담는다 — 가로 오프셋을 건드리지 않는다
And 이미 놓인 오프셋 위에서 계산한다
And ±50 으로 죈다
And 요소가 2개 미만이면 아무 일도 하지 않는다
And 기준 상자를 잴 수 없으면 아무것도 하지 않는다
```

vitest: `panelEditAlign.test.ts` — "computeAlignPatches — 요소끼리 맞춘다 (AC-34)" 8건

### AC-35 — 정렬 툴바 (U8-1 / U8-2 / U8-6)

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 렌더하면
Then role="toolbar" 인 정렬 툴바가 나오고
And 가로 3종(start/center/end) · 세로 3종의 단추 6개와 배치 초기화 단추가 있다
And 편집이 꺼져 있으면 툴바가 없다
```

vitest: `StatPanel.test.tsx` — "AC-35: 편집 중에는 정렬 툴바가 보인다" · "AC-35: 편집이 꺼져 있으면 툴바가 없다"

### AC-36 — 배치 초기화와 한 번의 쓰기 (U8-5 / U8-6)

```gherkin
Given 세 요소에 오프셋이 저장되고 본값에는 font_size 도 있는 편집 중 패널이 있을 때
When 배치 초기화를 누르면
Then onConfigChange 가 정확히 한 번 호출되고
And 오프셋만 지워져 value_layout 에는 font_size 만 남고 나머지 둘은 undefined 가 된다
```

나눠 넘기면 앞의 쓰기가 반영되기 전에 다음 계산이 옛 config 를 읽어 서로를 덮어쓴다.
정렬 단추도 같은 규칙으로 한 번에 묶어 넘긴다.

vitest: `StatPanel.test.tsx` — "AC-36: 배치 초기화가 세 요소의 오프셋을 한 번에 지운다" ·
"정렬 버튼은 세 요소의 오프셋을 한 번에 넘긴다"

### AC-37 — 선택 전이와 선택 표시 (U9-1 / U9-2 / U9-3 / U9-5 / U9-6)

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 아직 아무것도 고르지 않았으면
Then 세 요소가 모두 흐린 점선(outline-dashed)이고 크기 손잡이는 하나도 없다

When 본값을 누르면
Then 본값만 진한 실선(outline-2)이 되고 손잡이가 하나 붙는다

When Shift 를 누른 채 변화량을 더 누르면
Then 둘 다 진해지고 손잡이가 둘이 된다

When 기준 상자 안의 빈 자리를 누르면
Then 선택이 풀리고 손잡이가 사라진다
```

```gherkin
Given 이미 고른 요소가 있을 때
When 그것을 다시 누르면
Then 선택이 유지된다 — 끌기를 시작하려는 것이지 고르기를 무르려는 것이 아니다
```

vitest: `StatPanel.test.tsx` — "AC-37: 고르기 전에는 셋 다 흐린 점선이고 손잡이가 없다" ·
"AC-37: 고른 요소는 진한 실선이 되고 손잡이가 붙는다" · "AC-37: Shift 로 두 개를 고르면 둘 다 진해진다" ·
"AC-37: 빈 자리를 누르면 선택이 풀린다" · "편집을 끄면 선택도 거둔다 — 상자만 남으면 푸는 방법이 없다"(U9-8)
vitest: `panelEditSelection.test.ts` — "nextSelection (AC-37)" 5건
vitest: `PanelDragLayer.test.tsx` — "선택과 무리 이동 (AC-37 / AC-38)"

### AC-38 — 무리 이동은 함께 멈춘다 (U9-4 / D10)

```gherkin
Given 두 요소를 골라 둔 편집 중 패널이 있을 때
When 그중 하나를 끌면
Then 선택 전체가 같은 만큼 움직인다
And 한 요소가 먼저 상한(±50)에 닿으면 무리 전체가 거기서 멈춘다 — 따로 죄면 대형이 무너진다
And 두 축은 따로 죈다 — 한 축이 막혔다고 다른 축이 멈추지 않는다
And 이미 상한을 넘겨 저장된 값에서도 더 벌어지지 않는다
```

vitest: `panelEditSelection.test.ts` — "clampGroupDelta — 무리가 함께 멈춘다 (AC-38)" 6건
vitest: `PanelDragLayer.test.tsx` — "AC-38: 고른 요소를 끌면 선택 전체가 같은 만큼 움직인다" ·
"AC-38: 한 요소가 상한에 닿으면 무리 전체가 멈춘다"

### AC-40 — 격자 맞춤 토글은 지속 설정이되 저장하지 않는다 (U7-7 / U9-8)

```gherkin
Given 편집이 켜진 stat 패널이 있을 때
When 툴바를 검사하면
Then 격자 맞춤 토글이 있고 기본은 켜짐(aria-pressed="true")이다
And 누르면 꺼짐으로 바뀐다
And 그 조작으로 onConfigChange 가 호출되지 않는다 —
    저장하면 다음에 열 때도 꺼져 있고 그 이유를 알 수 없다
```

vitest: `StatPanel.test.tsx` — "AC-40: 격자 맞춤 토글이 툴바에 있고 기본은 켜짐이다" ·
"AC-40: 격자 설정은 config 에 저장되지 않는다"

### AC-41 — 설정 미리보기에서 실제로 끌린다 (U4-2 / U4-6)

```gherkin
Given 채널 모드 통계 패널의 설정 다이얼로그를 열었을 때
When 미리보기를 렌더하면
Then forceEdit 로 드래그 표식과 정렬 툴바가 나온다

When 미리보기의 본값을 실제로 끌면
Then 미리보기가 지연 없이 따라온다
```

표식이 있는 것만으로는 부족하다 — 미니 프리뷰도 진짜 패널을 쓰므로 표식은 늘 있었고,
그 단언만으로 오탐이 났다. 드래그 레이어가 실제로 켜졌는지(cursor-move)와 값이 따라오는지를 함께 본다.

vitest: `PanelSettingsDialog.previewEdit.test.tsx` — "통계 미리보기 (AC-41)" ·
"미리보기에서 실제로 끌린다 (AC-41)" ("통계 본값을 끌면 미리보기가 따라온다")

### AC-E1 — jsdom 에는 레이아웃이 없다

정렬 계산은 요소의 실제 상자를 읽는데 jsdom 에서는 전부 0 이다. 그래서 패널 수준 테스트는
"한 번의 호출로 묶여 나간다" 만 보고, 좌표 계산은 `panelEditAlign.test.ts` 가 잠근다.
이 분업을 깨고 패널 테스트에서 좌표를 단언하면 거짓 통과가 된다.
