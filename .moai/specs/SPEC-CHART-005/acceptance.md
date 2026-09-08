# SPEC-CHART-005 인수 조건

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)
형식: Given-When-Then + 기계 검증 가능한 단언(vitest 파일·테스트명, 또는 grep/tsc 명령과 기대 출력)

## 이 문서의 내력 (읽는 사람이 먼저 알아야 할 것)

**이 문서는 구현이 끝난 뒤(2026-09-08)에 쓰였다.** 계획 단계에서 작성되어 구현을 이끈 문서가 아니라,
plan.md M7.5 가 대조 대상으로 지목한 문서가 존재하지 않는다는 것을 발견하고 **사후에 채운** 것이다.
따라서 여기 적힌 조건은 필연적으로 **만들어진 것을 서술한다** — 만들 것을 규정한 것이 아니다.

번호도 마찬가지다. `AC-05`~`AC-16` 은 이 문서보다 먼저 **테스트 이름에** 존재했다
(`BarChartPanel.test.tsx` · `PieChartPanel.test.tsx`). 이 문서는 그 번호를 **그대로 받아 적었다** —
이미 코드에 박힌 참조를 다시 매기면 문서와 테스트가 갈라진다. 어느 테스트에도 없던 `AC-01`~`AC-04` 는,
구현되었으나 AC 번호를 달지 않은 요구(§2.1 공용화 · §2.5 미리보기)에 배정했다.

여섯 달 뒤에 이 문서를 읽는 사람에게: 이것은 개발을 이끈 인수 기준이 아니다. 개발 결과를
요구사항(spec.md §2)에 되짚어 대조한 기록이다.

---

## 공통 픽스처

**F1 — 카테고리 모드 entries** (`mode: 'category'`, 기본 `label_field: labels.name`)

| # | value | label |
|---|-------|-------|
| e0 | 10 | A |
| e1 | 20 | B |

**F2 — 편집 상태 3종** (세 패널 공통 규칙)

| 상태 | props | 기대 |
|------|-------|------|
| 읽기 전용 | `onConfigChange` 없음 | 편집 입구 없음 |
| 미리보기 | `onConfigChange` + `forceEdit` | 항상 편집, 토글 없음 |
| 대시보드 | `onConfigChange`, `dashboardEditMode: true` | 토글 노출, 눌러야 편집 |

---

## A. 편집 장치 공용화 (U1)

### AC-01 — 계산 모듈이 패널 중립 이름·중립 타입으로 옮겨졌다 (U1-1 / U1-2 / U1-3)

```gherkin
Given 격자 스냅·정렬 계산과 선택 전이·무리 이동 클램프가 통계 전용 모듈에 있었을 때
When 공용화가 끝나면
Then 그 계산은 패널 중립 모듈이 소유하고, 요소 종류는 문자열 제네릭이라
     패널마다 자기 종류를 그대로 실어 나른다
And 격자·중심 표식 컴포넌트와 정렬 툴바도 패널 중립 이름의 파일로 옮겨진다
```

```bash
# 옛 이름은 사라졌다
ls web/src/pages/dashboard/panels/charts/statAlign.ts \
   web/src/pages/dashboard/panels/charts/statSelection.ts \
   web/src/pages/dashboard/StatEditGrid.tsx \
   web/src/pages/dashboard/StatAlignToolbar.tsx
# 기대: 4건 모두 No such file

# 새 이름이 있다
ls web/src/pages/dashboard/panels/charts/panelEditAlign.ts \
   web/src/pages/dashboard/panels/charts/panelEditSelection.ts \
   web/src/pages/dashboard/PanelEditGrid.tsx \
   web/src/pages/dashboard/PanelAlignToolbar.tsx
# 기대: 4건 모두 존재

# 계산이 요소 종류에 묶여 있지 않다
grep -n "K extends string" web/src/pages/dashboard/panels/charts/panelEditAlign.ts \
                          web/src/pages/dashboard/panels/charts/panelEditSelection.ts
# 기대: computeAlignPatches<K extends string> · nextSelection<K extends string> 등이 잡힌다
```

`npx tsc -b` — 오류 0

vitest: `panelEditAlign.test.ts`(14건) · `panelEditSelection.test.ts`(11건) — 이동 전 단언이 그대로 통과한다.

**주의**: 이 조건은 **파일 이름과 타입 시그니처**만 본다. 모듈 안의 export 이름은 여전히 통계 이름이다
(`StatSelection` · `StatElementBox` · `StatDragLayer`). 그 분기는 spec.md §8 IN-1 에 적혀 있으며,
§2 의 어느 요구도 export 이름을 규정하지 않으므로 이 조건의 판정에는 들어가지 않는다.

### AC-02 — 드래그 레이어가 고정 3대상 대신 대상 맵을 받는다 (U1-4)

```gherkin
Given 드래그 레이어가 value/delta/stats 세 prop 을 못박아 받고 있었을 때
When 공용화가 끝나면
Then 레이어는 targets: Record<K, DragTarget> 하나를 받고,
     그 키가 곧 화면 표식의 값이 된다
And 표식 이름은 패널 중립이다 — data-panel-drag · data-panel-resize
```

```bash
grep -n "targets: Readonly<Record<K" web/src/pages/dashboard/PanelDragLayer.tsx
# 기대: 대상 맵 시그니처 1건

grep -rn "data-stat-drag\|data-stat-resize" web/src/
# 기대: 출력 없음 (exit 1) — 통계 전용 표식은 남아 있지 않다
```

vitest: `PanelDragLayer.test.tsx` — 통계 3대상 픽스처로 전량 통과(대상 수에 갈리는 코드가 없음을 보인다).

### AC-03 — 공용화가 통계 패널의 동작을 바꾸지 않았다 (U1-5)

```gherkin
Given 통계 패널의 기존 테스트가 이동 전에 전량 통과하고 있었을 때
When 모듈을 공용 이름으로 옮기고 대상 맵으로 바꾸면
Then 통계 테스트는 단언을 고치지 않고 그대로 통과한다 — 표식 이름 갱신만 예외다
```

이것이 이동과 변경을 가르는 유일한 판정이다(plan.md M2 검증절).

vitest: `StatPanel.test.tsx` · `PanelDragLayer.test.tsx` · `statLayout.test.ts` ·
`panelEditAlign.test.ts` · `panelEditSelection.test.ts` — 전량 GREEN.

### AC-04 — 미리보기가 신설 키를 즉시 반영한다 (U5-1 / U5-2)

```gherkin
Given 설정 다이얼로그가 바·파이 실패널을 미리보기로 그릴 때
When 미리보기를 렌더하면
Then 두 패널 모두 onConfigChange 와 forceEdit 를 받아 항상 편집이다
And 그림 상자·범례의 배치·크기·글자 키는 PREVIEW_LIVE_KEYS 에 있어
    200ms 디바운스를 건너뛴다
```

```bash
grep -n "pie_size\|pie_offset_\|legend_offset_\|legend_font_\|legend_position\|show_legend\|plot_offset_\|plot_size" \
  web/src/pages/dashboard/previewLiveKeys.ts
# 기대: 신설 키 전량이 PREVIEW_LIVE_KEYS 안에 있다

grep -n "forceEdit" web/src/pages/dashboard/PanelSettingsDialog.tsx | head
# 기대: bar-chart · pie-chart 미리보기 분기 둘 다 forceEdit 를 넘긴다
```

**범위 주의**: 바·파이에는 합성 샘플값 미니 프리뷰가 없다(`PanelSettingsDialog.tsx` §미리보기 게이트 주석).
게이트는 `previewRealData` 하나이며, 미리보기가 그려지는 모든 경로에서 편집이 켜진다.
U5-1 의 "합성 샘플값을 쓰는 경로에서도" 는 게이지에만 해당하는 문장이고, 이 SPEC 의 두 패널에는
해당 경로 자체가 없다.

---

## B. 바 차트 범례 (U2)

### AC-05 — 범례는 기본으로 꺼져 있다 (U2-3 / D4)

```gherkin
Given show_legend 를 적지 않은 바 차트 config 가 있을 때
When 렌더하면
Then 범례가 그려지지 않는다 — 저장된 대시보드의 외형이 바뀌지 않는다
```

vitest: `BarChartPanel.test.tsx` — "AC-05: 범례는 기본으로 꺼져 있다 — 저장된 대시보드의 외형이 바뀌지 않는다"
단언: `pie-chart-legend` 부재

### AC-06 — 켜면 파이와 같은 표현·같은 키로 그린다 (U2-1 / U2-2 / U2-4)

```gherkin
Given F1 데이터와 show_legend: true 인 바 차트가 있을 때
When 렌더하면
Then 카테고리별 항목이 순서대로 나오고(A, B)
And 비중(percent)은 내지 않는다 — 막대는 합계 대비 비중을 읽는 그림이 아니다
And 값(value)은 낸다
And legend_position · legend_offset_* 가 파이와 같은 규칙으로 먹는다
    (오른쪽 배치 + offset_y 12 → 기준 50% 에 접혀 62%)
And 범례는 그림 위에 겹친다(absolute) — 자리를 나눠 가지지 않는다
```

vitest: `BarChartPanel.test.tsx` — "AC-06: 켜면 카테고리별 항목이 나온다" ·
"AC-06: 범례는 비중을 내지 않는다 …" · "AC-06: 범례 자리·변위가 파이와 같은 키로 먹는다"

```bash
grep -n "absolute" web/src/pages/dashboard/panels/charts/PieLegend.tsx
# 기대: 범례 상자가 스스로 absolute 로 뜬다 (U2-4)
```

---

## C. 바 차트 배치 편집 (U3)

### AC-07 — 플롯 키가 라인 차트와 같다 (U3-3 / D5)

```gherkin
Given plot_offset_x: 10 · plot_offset_y: -5 · plot_size: 80 인 바 차트가 있을 때
When 렌더하면
Then 그림 상자에 translate(10%, -5%) 와 scale(0.8) 이 걸린다
```

vitest: `BarChartPanel.test.tsx` — "AC-07: 플롯 오프셋·크기가 라인과 같은 키로 먹는다"

### AC-08 — 편집 게이팅과 끌 대상 (U3-1 / U3-2 / U4-1 / U4-2)

```gherkin
Given onConfigChange 를 넘기지 않은 바 차트가 있을 때
When 렌더하면
Then 편집 토글·드래그 표식·그리드·정렬 툴바가 하나도 없다

Given onConfigChange + forceEdit 를 넘기고 범례를 켠 바 차트가 있을 때
When 렌더하면
Then 그리드·중심 표식·정렬 툴바가 나오고 끌 덩어리는 그림과 범례 둘이다
And 편집 토글 버튼은 없다 (미리보기는 토글을 감춘다)

Given 범례가 꺼져 있을 때
When 렌더하면
Then 끌 덩어리는 그림 하나다
```

vitest: `BarChartPanel.test.tsx` — "AC-08: onConfigChange 가 없으면 편집 입구가 없다" ·
"AC-08: forceEdit 면 그리드·툴바·드래그 표식이 나온다" · "AC-08: 범례가 꺼져 있으면 끌 덩어리는 그림 하나다"

### AC-09 — 고르면 진해지고 크기 손잡이가 붙는다 (U4-3 / U4-4 / U4-5)

```gherkin
Given 편집이 켜진 바 차트가 있을 때
When 그림 상자를 누르면
Then 흐린 점선(outline-dashed)이 진한 실선(outline-2)으로 바뀌고
And 크기 손잡이가 그 요소 하나에만 붙는다
```

vitest: `BarChartPanel.test.tsx` — "AC-09: 고르면 진한 실선이 되고 크기 손잡이가 붙는다"

크기 손잡이가 바꾸는 값: 플롯은 `plot_size`(%) — 가로 손잡이는 `bar_size`(막대 굵기 px),
세로는 `plot_size_y`(%) 로 축을 나눈다 · 범례는 `legend_font_size`(px).
(`BarChartPanel.tsx` 의 `targets` 맵 `onResize` 3종)

### AC-10 — 배치 초기화가 두 요소의 오프셋을 한 번에 지운다 (U4-2)

```gherkin
Given plot_offset_x: 20 · legend_offset_y: 10 이 저장된 편집 중 바 차트가 있을 때
When 정렬 툴바의 배치 초기화를 누르면
Then onConfigChange 가 정확히 한 번 호출되고
And 플롯·범례의 오프셋 네 키가 함께 undefined 로 지워진다
```

나눠 넘기면 앞의 쓰기가 반영되기 전에 다음이 옛 config 를 읽는다.

vitest: `BarChartPanel.test.tsx` — "AC-10: 배치 초기화가 두 요소의 오프셋을 한 번에 지운다"

---

## D. 파이 차트 편집 표면 (U4)

### AC-11 — 그리드와 중심 표식 (U4-1)

```gherkin
Given 편집이 켜진 파이 차트가 있을 때
When 렌더하면
Then 격자와 중심 + 표식이 보인다
And 편집이 꺼져 있으면 그리드도 툴바도 없다
```

vitest: `PieChartPanel.test.tsx` — "AC-11: 편집 중에는 그리드와 중심 표식이 보인다" ·
"AC-11: 편집이 꺼져 있으면 그리드도 툴바도 없다"

### AC-12 — 정렬 툴바와 스냅 토글 (U4-2 / U4-6)

```gherkin
Given 편집이 켜진 파이 차트가 있을 때
When 렌더하면
Then 정렬 툴바가 나오고, 격자 맞춤 토글의 기본은 켜짐이며, 배치 초기화 단추가 있다
```

vitest: `PieChartPanel.test.tsx` — "AC-12: 정렬 툴바가 나온다"

### AC-13 — 끌 대상은 그림과 범례 둘이다

```gherkin
Given 편집이 켜지고 범례를 켠 파이 차트가 있을 때
When 렌더하면
Then 끌 덩어리가 둘이다
And 범례가 꺼져 있으면 하나다
```

vitest: `PieChartPanel.test.tsx` — "AC-13: 끌 수 있는 덩어리는 그림과 범례 둘이다" ·
"AC-13: 범례가 꺼져 있으면 그림 하나다"

### AC-14 — 고르면 진해지고 크기 핸들이 붙는다 (U4-3 / U4-4 / U4-5)

```gherkin
Given 편집이 켜진 파이 차트가 있을 때
When 파이 그림을 누르면
Then 흐린 점선이 진한 실선이 되고 크기 핸들이 하나 붙는다
```

핸들이 바꾸는 값: 파이 그림은 `pie_size`(%) · 범례는 `legend_font_size`(px) — OQ1 / OQ2 의 잠정 결정대로다.

vitest: `PieChartPanel.test.tsx` — "AC-14: 고르면 진한 실선이 되고 크기 핸들이 붙는다"

### AC-15 — 배치 초기화 (U4-2)

```gherkin
Given pie_offset_x: 12 · legend_offset_y: 8 이 저장된 편집 중 파이 차트가 있을 때
When 배치 초기화를 누르면
Then onConfigChange 가 한 번 호출되고 네 오프셋 키가 함께 지워진다
```

vitest: `PieChartPanel.test.tsx` — "AC-15: 배치 초기화가 두 요소의 오프셋을 한 번에 지운다"

### AC-16 — 편집 표식이 기존 배치를 무너뜨리지 않는다 (U1-5 정신 / 두 패널 공통)

```gherkin
Given 편집이 켜진 바·파이 차트가 있을 때
When 그림 상자의 클래스를 읽으면
Then 바는 h-full · w-full 을, 파이는 absolute · inset-0 을 그대로 갖는다
And 파이 상자에 relative 가 덧붙지 않는다 — absolute inset-0 을 이기면 높이가 0이 된다
And 범례의 편집 표식은 범례 자신에 붙는다 — 감싸는 상자를 만들면 그 상자가 크기 0이 되어
    윤곽이 패널 위쪽에 얇은 띠로 생긴다
```

vitest: `BarChartPanel.test.tsx` — "AC-16: 편집을 켜도 그림 상자의 크기 클래스가 살아 있다" ·
"AC-16: 편집 표식은 범례 자신에 붙는다 — 감싸는 상자를 만들지 않는다"
vitest: `PieChartPanel.test.tsx` — "AC-16: 편집을 켜도 그림 상자의 위치 클래스가 살아 있다" ·
"AC-16: 편집 표식은 범례 자신에 붙는다 — 감싸는 상자를 만들지 않는다"

---

## 엣지 케이스 (Edge Cases)

### AC-E1 — 범례가 꺼진 채 편집을 켜면 끌 대상이 하나뿐이다

바·파이 모두 해당한다. 대상이 하나면 정렬은 맞출 상대가 없어 아무 일도 하지 않는다
(`computeAlignPatches` 의 "요소가 2개 미만이면" 분기 — `panelEditAlign.test.ts`).

vitest: `BarChartPanel.test.tsx` "AC-08: 범례가 꺼져 있으면 …" · `PieChartPanel.test.tsx` "AC-13: 범례가 꺼져 있으면 …"

### AC-E2 — Alt 를 누른 채 끌면 격자를 잠시 끈다

```gherkin
Given 격자 맞춤이 켜진 상태에서 요소를 끌 때
When Alt 를 누른 채 6px(=3%p) 움직이면
Then 격자에 붙지 않고 3 이 그대로 전달된다
```

vitest: `PanelDragLayer.test.tsx` — "Alt 를 누르고 끌면 격자를 잠시 끈다 — 정밀 조정이 막히면 안 된다"

### AC-E3 — 저장된 파이 범례 오프셋이 공용화 뒤에도 그대로 반영된다

기존 동작 보존(M1 특성화). `PieDragLayer` 를 `PanelDragLayer` 로 갈아탄 뒤에도 값의 뜻이 같아야 한다.

vitest: `PieChartPanel.test.tsx` — "M1-1.3: 저장된 범례 오프셋이 그대로 반영된다(기존 동작 보존)"

### AC-E4 — 바 차트에 신설 키가 하나도 없는 저장 config

```gherkin
Given 이번 SPEC 이전에 저장된 바 차트 config 가 있을 때
When 렌더하면
Then 범례가 없고 플롯 오프셋이 0 인 종전 화면이 그대로 나온다
```

§3.2 하위 호환 표. `show_legend` 기본 false · `plot_offset_*` 기본 0 · `plot_size_y` 미지정이면 `plot_size` 와 같다.

### AC-E5 — jsdom 에는 레이아웃이 없다

정렬 계산은 요소의 실제 상자를 읽는다. jsdom 에서는 모든 상자가 0 이므로 패널 수준 테스트는
"한 번의 호출로 묶여 나간다" 만 보고, 계산 자체는 `panelEditAlign.test.ts` 가 잠근다.
이 분업을 깨고 패널 테스트에서 좌표를 단언하면 거짓 통과가 된다.

---

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 공용화(모듈 이동)는 DDD — 기존 테스트가 **단언을 고치지 않고** 통과하는 것이 성공 판정.
  신규(바 범례·바 드래그 레이어 배선)는 TDD, 목표 85%.
- **Readable**: 한국어 코드 주석(프로젝트 규약). 공용 모듈의 이름이 특정 패널을 가리키지 않는다.
- **Unified**: 세 패널이 같은 편집 표면(그리드·툴바·선택·손잡이)을 같은 표식(`data-panel-*`)으로 쓴다.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON. 신설 키는 모두 옵셔널이며 기본값이 종전 화면이다.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-CHART-005` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0.

---

## Definition of Done

- [x] §2 U1~U5 전부 구현 및 요구사항 매핑 충족 (spec.md §8 요구사항 확인 표).
- [x] AC-01~AC-16, AC-E1~AC-E5 전부 통과.
- [x] 통계 패널 행위 보존 — 공용화가 이동이지 변경이 아님(AC-03).
- [x] plan.md M7.1 `npm run build` exit 0 / M7.2 `npm test` exit 0 / M7.3 `npm run lint` exit 0.
- [x] **plan.md M7.4 — 신규·이동 파일 커버리지 85% 이상. (2026-09-08 충족)**
      `vite.config.ts` 의 커버리지 `include` 허용목록에서 존재하지 않는 `StatDragLayer.tsx` 를 걷어내고
      실제 이동·신규 모듈을 등재한 뒤 `npx vitest run --coverage` 로 실측했다(exit 0, 413 파일 / 6795 테스트).
      **판정 지표는 statement(= line) 커버리지다** — 근거는 spec.md §8 "M7.4 판정 지표" 참고.

      | 모듈 | % Stmts | % Branch | 미커버 |
      |------|--------:|---------:|--------|
      | `PanelAlignToolbar.tsx` | 100 | 100 | — |
      | `PanelDragLayer.tsx` | 96.82 | 88.88 | 232-239 |
      | `PanelEditGrid.tsx` | 100 | 100 | — |
      | `PanelGridBackdrop.tsx` | 100 | 77.77 | 32, 38 |
      | `PanelResizeOverlay.tsx` | 98.98 | 68.75 | 127 |
      | `usePanelElementEdit.tsx` | 97.82 | 63.63 | 104 |
      | `panelEditContext.ts` | 100 | 100 | — |
      | `previewStage.ts` | 100 | 86.95 | 82-88, 99, 105-106 |
      | `previewGridSize.ts` | 100 | 71.42 | 42-48, 93-95 |
      | `StatElementStylePopover.tsx` | 100 | 100 | — |
      | `textStyleFields.tsx` | 100 | 82.14 | 136, 152, 156, 171 |

      statement 기준 **전량 85% 이상**(최저 `PanelDragLayer.tsx` 96.82%). 와일드카드로 이미 계측되던
      `panelEditAlign.ts` 100 / `panelEditSelection.ts` 100 도 함께 확인했다.
      **branch 기준으로는 4종이 85% 미만**이다(`usePanelElementEdit.tsx` 63.63 ·
      `PanelResizeOverlay.tsx` 68.75 · `previewGridSize.ts` 71.42 · `PanelGridBackdrop.tsx` 77.77).
      이 SPEC 의 완료 판정에는 들어가지 않지만 **후속 보강 대상으로 남긴다** — 프로젝트가 뒷날
      branch 를 게이트 지표로 채택하면 정확히 이 4종이 걸린다.
      증거 로그: `verify/7-coverage.log`.
- [x] plan.md M7.5 — acceptance.md AC 전량 대조. 이 문서가 그 대조이며, 결과는 위 체크 상태 그대로다.
