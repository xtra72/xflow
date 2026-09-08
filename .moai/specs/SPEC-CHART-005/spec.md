---
id: SPEC-CHART-005
title: 바·파이 차트 배치 편집 — 편집 장치 공용화 + 바 범례 신설
version: 1.0.0
status: completed
created: 2026-09-07
updated: 2026-09-08
author: xtra
priority: medium
domain: dashboard
related_specs:
  - SPEC-CHART-002
  - SPEC-CHART-003
  - SPEC-CHART-004
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-09-08 | xtra | **구현 완료 — `status: completed`.** v0.3.0 이 완료를 막는 유일한 항목으로 지목한 **plan.md M7.4 가 충족되었다**: `vite.config.ts` 의 커버리지 `include` 허용목록에서 존재하지 않는 `StatDragLayer.tsx` 를 걷어내고 실제 이동·신규 모듈을 등재한 뒤 `npx vitest run --coverage` 로 실측했다. 신규 계측된 11종이 statement 기준 **전량 85% 이상**(최저 `PanelDragLayer.tsx` 96.82%)이고, 와일드카드로 이미 계측되던 `panelEditAlign.ts` · `panelEditSelection.ts` 는 100% 다. 판정 지표를 statement 로 읽은 근거는 §8 "M7.4 판정 지표" 에 적었다 — 기준 문구가 지표를 명시하지 않고, 저장소에 임계값 설정도 없으며, SPEC-CHART-004 §8 이 **같은 파일** `PanelDragLayer.tsx` 를 96.82% 로 적어 이미 statement 로 읽었기 때문이다. **요구 커버리지**: §2 U1~U5 전량 구현. acceptance.md 의 AC-01~AC-16 · AC-E1~AC-E5 전량 충족. **검증**: `npx vitest run --coverage` exit 0(413 파일 / 6795 테스트) · `npx eslint .` 오류 0(기존 경고 12) · `npx tsc -b` exit 0. **계획 대비 분기**: export 이름이 통계 이름 그대로 남았고(IN-1), plan.md §2 가 예고하지 않은 파일 6종이 생겼으며(IN-2), 바 config 가 §3.1 보다 넓고(IN-3), 게이지가 §6 의 범위 밖에서 함께 손질되었으며(IN-4), 게이지 값 좌표 이관 코드가 없다(IN-5). 다섯 다 §2 의 요구를 미충족시키지 않으므로 완료를 막지 않으나, IN-4·IN-5 는 **별도 SPEC 이 필요한 후속**이고 IN-5 는 저장된 게이지 값 자리를 잃는 실사용 회귀다. branch 커버리지 4종이 85% 미만인 것도 후속 보강 대상으로 남긴다. 판정 근거는 §8 표에 있다. |
| 0.3.0 | 2026-09-08 | xtra | **`acceptance.md` 를 신설하고 전량 대조했다 — `draft` 유지.** v0.2.0 이 완료를 막는 두 가지로 적은 것 중 **문서 부재는 해소**했다: `acceptance.md` 에 AC-01~AC-16 과 엣지 5건을 작성하고, 그 문서가 **구현 뒤에 쓰인 사후 대조 기록**임을 머리에 명시했다. 번호는 이미 테스트 이름에 박혀 있던 `AC-05`~`AC-16` 을 그대로 받아 적었고, 어느 테스트에도 없던 `AC-01`~`AC-04` 는 구현되었으나 AC 번호가 없던 요구(§2.1 공용화 U1-1~U1-5 · §2.5 미리보기 U5-1·U5-2)에 배정했다. **요구 커버리지**: §2 U1~U5 전량 구현·확인(§8 요구사항 확인 표). AC-01~AC-16 · AC-E1~AC-E5 전량 충족. **검증**: `npx vitest run` exit 0(413 파일 / 6795 테스트) · `npx eslint .` 오류 0(기존 경고 12) · `npx tsc -b` exit 0. **남은 미충족은 하나 — plan.md M7.4**. `vite.config.ts` 의 커버리지 `include` 허용목록이 갱신되지 않아 `src/pages/dashboard/` 루트로 옮겨진 파일 8종이 계측 밖이고, 목록은 여전히 존재하지 않는 `StatDragLayer.tsx` 를 가리킨다. 이는 코드 변경이 필요한 항목이라 문서 작업으로 닫을 수 없으므로 `status` 는 `draft` 로 남긴다(§8 "완료 판정을 막는 것"). 함께 확인된 계획 대비 분기 4건(export 이름 미정리 · 예고 없던 파일 6개 · 바 config 확장 · 게이지 범위 이탈)과 게이지 좌표 이관 누락 1건의 판정은 §8 에 적는다. |
| 0.2.0 | 2026-09-08 | xtra | **구현 대조 결과 기록 — `draft` 유지.** `draft` 를 벗지 못하는 이유는 결함이 아니라 **문서 부재**다: 이 SPEC 에는 `acceptance.md` 가 없다. plan.md 머리말이 그 파일을 링크하고 M7.5 가 "acceptance.md 의 AC 전량 대조" 를 요구하는데 파일이 존재하지 않으며, 그럼에도 구현 쪽 테스트는 이미 `AC-05`~`AC-16` 을 이름에 달고 있다(`BarChartPanel.test.tsx` · `PieChartPanel.test.tsx`) — 번호를 코드가 먼저 쓰고 문서가 따라오지 못한 상태다. 함께 M7.4(커버리지 `include` 갱신)도 수행되지 않았다. §2 의 U1~U5 요구사항 자체는 코드에서 모두 확인되며, 그 근거와 계획 대비 분기 5건은 §8 에 적는다. |
| 0.1.0 | 2026-09-07 | xtra | 최초 작성 — SPEC-CHART-004 가 통계 패널에만 준 편집 장치(그리드·중심 표식·정렬 툴바·선택 모델·크기 핸들)를 **바·파이**로 넓힌다. 통계용으로 만든 모듈을 복사하지 않고 **공용 모듈로 끌어올려** 세 패널이 함께 쓴다. 바 차트는 끌 대상이 플롯 하나뿐이므로 **범례를 먼저 신설**해 라인·파이와 구성을 맞춘다. |

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 한 줄이다.

```
바/파이 차트에서 배치 편집, 그리드, 정렬 등 누락된 기능 추가
```

### 1.2 배경 — 세 패널의 출발점이 다르다

| | 배치 편집 | 편집 토글 | 그리드·중심 | 정렬 툴바 | 선택 구분 | 크기 핸들 |
|---|---|---|---|---|---|---|
| 통계 | ✅ 3요소 | ✅ | ✅ | ✅ | ✅ | ✅ |
| 파이 | ✅ 범례·파이 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 라인(참고) | ✅ 범례·플롯 | ✅ | ❌ | ❌ | ❌ | ❌ |
| **바** | ❌ **없음** | ❌ | ❌ | ❌ | ❌ | ❌ |

**파이**는 [`PieDragLayer`](../../../web/src/pages/dashboard/PieDragLayer.tsx) 로 범례·파이 그림을 이미 끌 수 있다. 없는 것은 그리드·정렬·선택·크기 핸들이다.

**바**는 [`BarChartPanel.tsx`](../../../web/src/pages/dashboard/panels/charts/BarChartPanel.tsx)(288줄)에 드래그 레이어도 오프셋 config 도 없다. 게다가 **범례가 없다** — 라인은 `ChartLegend`, 파이는 `PieLegend` 를 쓰는데 바는 어느 쪽도 쓰지 않아, 끌 수 있는 덩어리가 플롯 하나뿐이다.

### 1.3 통계용 모듈은 이미 대부분 패널 중립적이다

SPEC-CHART-004 가 만든 것들 중 이름만 `stat` 인 것과 진짜로 통계 전용인 것이 갈린다.

| 모듈 | 실제 성격 |
|------|-----------|
| [`statAlign.ts`](../../../web/src/pages/dashboard/panels/charts/statAlign.ts) | **중립** — 격자 스냅·정렬 계산. `StatElementKind` 타입만 통계에 묶여 있다 |
| [`statSelection.ts`](../../../web/src/pages/dashboard/panels/charts/statSelection.ts) | **중립** — 선택 전이·무리 이동 클램프 |
| [`StatEditGrid.tsx`](../../../web/src/pages/dashboard/StatEditGrid.tsx) | **중립** — 격자·중심 표식 |
| [`StatAlignToolbar.tsx`](../../../web/src/pages/dashboard/StatAlignToolbar.tsx) | **중립** — 정렬·초기화·스냅 토글 |
| [`StatDragLayer.tsx`](../../../web/src/pages/dashboard/StatDragLayer.tsx) | **절반** — 포인터 처리는 중립, 대상이 `value`/`delta`/`stats` 셋으로 못박혀 있다 |
| [`statLayout.ts`](../../../web/src/pages/dashboard/panels/charts/statLayout.ts) | **통계 전용** — 요소별 기본 px, config 키 |

그래서 복사가 아니라 **끌어올리기**가 맞다(§5 D1).

---

## 2. 요구사항 (EARS)

### 2.1 편집 장치 공용화 [U1]

- **[U1-1]** The 격자 스냅·정렬 계산 shall 패널 중립 모듈이 소유한다. 요소 종류는 문자열 제네릭으로 두어 패널마다 자기 종류를 쓴다.
- **[U1-2]** The 선택 전이·무리 이동 클램프 shall 패널 중립 모듈이 소유한다.
- **[U1-3]** The 격자·중심 표식 컴포넌트와 정렬 툴바 shall 패널 중립 이름으로 옮긴다.
- **[U1-4]** The 드래그 레이어 shall 고정된 세 대상 대신 **대상 맵**(`Record<kind, DragTarget>`)을 받는다 — 패널마다 대상 수와 이름이 다르다.
- **[U1-5]** The 공용화 shall 통계 패널의 동작을 바꾸지 않는다. 기존 테스트가 그대로 통과해야 한다 — 통과하지 못하면 그것은 끌어올리기가 아니라 변경이다.

### 2.2 바 차트 범례 [U2]

- **[U2-1]** The 바 차트 config shall 범례 표시 여부·자리·글자 모양을 갖는다. 키 이름과 값의 뜻은 **파이와 같게** 한다 — 두 패널의 데이터 모양(카테고리 · 값 · 색)이 같으므로, 설정이 다르면 같은 것을 두 번 배워야 한다.
- **[U2-2]** The 바 범례 shall 파이와 같은 [`PieLegend`](../../../web/src/pages/dashboard/panels/charts/PieLegend.tsx) 표현을 쓴다(이름·값·색을 칸 맞춰 세로로 쌓는 표).
- **[U2-3]** Where 범례 표시가 꺼져 있는 경우(기본), the 바 차트 shall 종전과 **같은 화면**을 그린다 — 저장된 대시보드의 외형이 바뀌지 않는다.
- **[U2-4]** The 바 범례 shall 파이처럼 그림 **위에 겹쳐** 뜬다. 자리를 나눠 가지면 범례를 옮길 때마다 막대 폭이 따라 변한다.

### 2.3 바 차트 배치 편집 [U3]

- **[U3-1]** The 바 차트 shall `onConfigChange` · `forceEdit` prop 을 받고 `usePanelEditMode` 로 편집을 게이팅한다(통계·파이와 같은 3겹 규칙).
- **[U3-2]** The 바 차트 shall 플롯 영역과 범례를 각각 끌 수 있게 한다.
- **[U3-3]** The 플롯 배치 shall 라인 차트와 **같은 config 키**(`plot_offset_x` · `plot_offset_y` · `plot_size`)를 쓴다 — 같은 뜻의 값에 패널마다 다른 이름을 두면 설정을 옮겨 붙일 수 없다.

### 2.4 세 패널 공통 편집 표면 [U4]

- **[U4-1]** Where 편집이 켜져 있는 경우, the 바·파이 shall 격자와 중심 `+` 표식을 그린다.
- **[U4-2]** Where 편집이 켜져 있는 경우, the 바·파이 shall 정렬 툴바(정렬 6종 · 스냅 토글 · 배치 초기화)를 낸다.
- **[U4-3]** The 바·파이 shall 선택 모델을 갖는다 — 누르면 고르고, `Shift`·`Ctrl`·`Cmd` 로 여럿을 고르며, 고른 것들이 함께 움직인다.
- **[U4-4]** The 선택 표시 shall 통계와 같다 — 고른 것은 진한 실선, 아닌 것은 흐린 점선.
- **[U4-5]** The 크기 핸들 shall 고른 요소에만 붙는다. 파이 그림과 바 플롯은 **`*_size`(%)** 를, 범례는 글자 크기를 바꾼다.
- **[U4-6]** The 격자 스냅 shall 통계와 같은 규칙을 쓴다 — 요소의 중심을 격자 교점에 붙이고, `Alt` 로 잠시 끈다.

### 2.5 미리보기 [U5]

- **[U5-1]** The 설정 미리보기 shall 바·파이 모두 `onConfigChange` + `forceEdit` 를 넘긴다. 합성 샘플값을 쓰는 경로에서도 그렇다 — 값이 합성이라는 것과 자리를 옮길 수 있다는 것은 다른 축이다(SPEC-CHART-004 게이지에서 같은 결함이 있었다).
- **[U5-2]** The `*_offset_*` · `*_size` · 범례 키 shall 미리보기 디바운스를 건너뛴다(`previewLiveKeys`) — 늦추면 끄는 동안 200ms 계단으로 따라온다.

---

## 3. 데이터 계약

### 3.1 바 차트 신설 config

```ts
// 파이와 같은 이름·같은 뜻. 두 패널의 데이터 모양이 같으므로 설정도 같아야 한다.
show_legend?: boolean;               // 기본 false — 종전 화면을 보존한다
legend_position?: PieLegendPosition; // 기본 'bottom'
legend_offset_x?: number;            // 담는 상자 대비 %
legend_offset_y?: number;
legend_font_size?: number;
legend_font_family?: ChartFontFamily;
legend_font_color?: string;

// 라인 차트와 같은 이름·같은 뜻.
plot_offset_x?: number;              // ±40 (panelGeometry 규칙)
plot_offset_y?: number;
plot_size?: number;                  // 20~100 (%)
```

파이는 이미 이 키들을 갖고 있으므로 신설은 바 쪽뿐이다. `show_legend` 기본값을 **false** 로 둔 이유는 §2.2 U2-3 — 저장된 대시보드의 외형을 바꾸지 않기 위함이다(파이는 기본 true 지만, 파이에는 원래 범례가 있었다).

### 3.2 하위 호환

| 저장된 config | 렌더 결과 |
|---------------|-----------|
| 바: 신설 키 없음 | 종전과 동일(범례 없음, 플롯 오프셋 0) |
| 파이: 지금까지의 키 | 그대로 — 그리드·정렬·선택이 더해질 뿐 배치 값의 뜻은 같다 |
| 통계: 모든 경우 | **변화 없음**(공용화는 이동이지 변경이 아니다 — U1-5) |

---

## 4. 화면

### 4.1 바 차트 — 범례를 켠 모습

```
┌───────────────────────────┐
│ 매출                      │
│ ┌───────────────────────┐ │
│ │   ▇                   │ │
│ │   ▇  ▇     ┌────────┐ │ │  ← 범례가 그림 위에 겹친다
│ │ ▇ ▇  ▇  ▇  │● A  120│ │ │
│ │ ▇ ▇  ▇  ▇  │● B   80│ │ │
│ └────────────┴────────┴─┘ │
└───────────────────────────┘
```

### 4.2 편집이 켜진 모습 (세 패널 공통)

```
┌───────────────────────────┐
│ ┌┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┐   │  ← 흐린 점선 = 고르지 않은 것
│ ┊   ▇  ▇     ┏━━━━━━┓ ┊   │
│ ┊ ▇ ▇  ▇  ▇  ┃● A 120┃◢┊   │  ← 진한 실선 = 고른 것 + 크기 핸들
│ ┊ ▇ ▇  ▇  ▇  ┗━━━━━━┛ ┊   │
│ └┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┘   │
│      ⊞ ⊟ ⊠ ⊡ ⊟ ⊞ │ 🧲 ↺   │  ← 정렬 툴바
└───────────────────────────┘
   (뒤에 10% 격자 + 중심 +)
```

---

## 5. 결정 (Decisions)

### D1 — 복사가 아니라 끌어올리기다

바·파이에 같은 조작을 주는 방법은 둘이다. 통계 모듈을 복사해 고치거나, 공용 모듈로 옮겨 셋이 함께 쓰거나.

복사를 고르지 않는 이유는 이 기능이 **이미 세 번 고쳐졌기** 때문이다 — 절반 속도(오프셋 좌표 기준), 스냅 기준(잡는 순간의 오프셋), 무리 이동(함께 멈춤). 셋 다 겉으로 드러나지 않는 계산 실수였고, 사본이 셋이면 다음 실수는 세 곳에서 각각 발견된다.

§1.3 표가 보여주듯 옮겨야 할 것 대부분은 이미 중립이다. 통계에 남는 것은 `statLayout.ts` 하나뿐이다.

### D2 — 요소 종류는 문자열 제네릭으로 둔다

패널마다 끌 대상이 다르다(통계 3, 파이 2, 바 2). 공용 모듈이 이 목록을 알 필요는 없고, 알면 패널이 늘 때마다 공용 모듈을 고쳐야 한다.

그래서 `StatElementKind` 대신 `<K extends string>` 로 두고, 패널이 자기 종류를 정의한다. 계산은 종류의 **개수**만 알면 되고 이름은 그대로 실어 나른다.

### D3 — 바 범례는 파이와 같은 설정·같은 표현이다

바와 파이는 데이터 모양이 같다(카테고리 · 값 · 색). `ChartLegend`(라인용)는 시리즈 타임라인의 "마지막 값" 칸을 갖고 있어 맞지 않는다.

키 이름을 파이와 같게 두면 두 패널을 오갈 때 배울 것이 없고, 범례 렌더도 한 구현을 공유해 "같은 설정인데 다르게 보인다" 가 생기지 않는다.

### D4 — 바 범례의 기본은 꺼짐이다

파이는 원래 범례가 있었으므로 기본 켜짐이 맞지만, 바에는 없었다. 기본을 켜면 저장된 모든 바 차트에 갑자기 범례가 나타나 그림을 덮는다.

### D5 — 플롯 키는 라인 차트와 공유한다

`plot_offset_x` · `plot_offset_y` · `plot_size` 는 라인 차트가 이미 쓰는 이름이고 뜻도 같다(그림 상자를 옮기고 키운다). 바에만 다른 이름을 두면 같은 뜻의 값이 둘이 되고, 패널 유형을 바꿀 때 설정이 사라진다.

---

## 6. 범위 밖 (Out of Scope)

- **라인 차트에 그리드·정렬·선택 더하기** — 같은 공용 모듈로 이어 붙일 수 있지만, 이번 요청은 바·파이다. 공용화가 끝나면 라인은 배선만 남는다.
- **게이지에 그리드·정렬 더하기** — 위와 같다.
- **바 차트 축·눈금의 배치 편집** — 축은 그림에 붙어 있어 따로 옮길 수 있는 덩어리가 아니다.
- **범례 항목의 개별 배치** — 범례는 하나의 덩어리로 다룬다.

---

## 7. 열린 질문 (Open Questions)

| # | 질문 | 잠정 결정 |
|---|------|-----------|
| OQ1 | 파이 그림·바 플롯의 크기 핸들은 어느 값을 바꾸나 | `pie_size` / `plot_size`(%)다. 글자가 아니므로 px 이 아니라 백분율이 맞고, 두 값은 이미 존재한다 |
| OQ2 | 범례의 크기 핸들은 | 글자 크기(px)를 바꾼다 — 범례는 글자 덩어리이고, 통계 요소와 같은 규칙이다 |
| OQ3 | 정렬은 플롯과 범례를 서로 맞추는 것이 뜻이 있나 | 있다. 범례를 플롯의 왼쪽 변에 맞추는 것은 흔한 배치다. 다만 세로 정렬은 둘을 겹치게 만드는데, 통계와 같은 이유로 막지 않는다(배치 초기화가 짝이다) |
| OQ4 | 바에 범례를 더하면 설정 섹션도 늘어난다 | 파이의 범례 설정 블록을 그대로 재사용한다 — 키가 같으므로 컴포넌트도 같다 |

---

## 8. 구현 노트 (Implementation Notes)

2026-09-08 대조 시점 기준. 구현은 작업 트리에 있고 아직 커밋되지 않았다. **`status` 는 `completed` 다** — v0.2.0/v0.3.0 이 완료를 막는다고 적은 두 항목(문서 부재 · M7.4)이 모두 해소되었다.

### 해소 (v0.2.0~v0.3.0 이 미충족으로 적었던 것)

- **`acceptance.md` 가 없다 → 해소(v0.3.0).** AC-01~AC-16 과 엣지 5건을 작성해 §2 U1~U5 와 매핑했다. 번호는 이미 테스트 이름에 박혀 있던 `AC-05`~`AC-16` 을 그대로 받아 적었고, 어느 테스트에도 없던 `AC-01`~`AC-04` 는 구현되었으나 AC 번호가 없던 요구(§2.1 공용화 · §2.5 미리보기)에 배정했다. 그 문서는 **구현 뒤에 쓰인 사후 대조 기록**이며 그 사실을 문서 머리에 명시했다. plan.md M7.5("acceptance.md AC 전량 대조")는 이로써 수행되었다.

- **plan.md M7.4 — 커버리지 계측 경로 → 해소(v1.0.0).** `vite.config.ts` 의 커버리지 `include` 허용목록이 사라진 `StatDragLayer.tsx` 를 가리키고 있어, 공용화로 옮겨진 모듈들이 오류 없이 **조용히 0건으로 빠지고** 있었다. 허용목록을 실제 모듈로 갱신한 뒤 `npx vitest run --coverage` 로 실측했다(exit 0, 413 파일 / 6795 테스트). 결과는 아래 표와 같고, **statement 기준 전량 85% 이상**이다.

| 모듈 | % Stmts | % Branch | 미커버 |
|------|--------:|---------:|--------|
| `PanelAlignToolbar.tsx` | 100 | 100 | — |
| `PanelDragLayer.tsx` | **96.82** | 88.88 | 232-239 |
| `PanelEditGrid.tsx` | 100 | 100 | — |
| `PanelGridBackdrop.tsx` | 100 | 77.77 | 32, 38 |
| `PanelResizeOverlay.tsx` | 98.98 | 68.75 | 127 |
| `usePanelElementEdit.tsx` | 97.82 | 63.63 | 104 |
| `panelEditContext.ts` | 100 | 100 | — |
| `previewStage.ts` | 100 | 86.95 | 82-88, 99, 105-106 |
| `previewGridSize.ts` | 100 | 71.42 | 42-48, 93-95 |
| `StatElementStylePopover.tsx` | 100 | 100 | — |
| `textStyleFields.tsx` | 100 | 82.14 | 136, 152, 156, 171 |
| `panelEditAlign.ts` (기존 계측) | 100 | 96.55 | 76 |
| `panelEditSelection.ts` (기존 계측) | 100 | 100 | — |

증거 로그: `verify/7-coverage.log`.

### M7.4 판정 지표 — statement 커버리지로 읽는다

plan.md M7.4 도, acceptance.md 의 품질 게이트도, `.moai/config/sections/quality.yaml` 의 `test_coverage_target: 85` · `min_coverage_new: 85` 도 **어느 것도 지표를 명시하지 않는다**. 그래서 무엇을 재는지는 해석의 문제이고, 아래 근거로 **statement(= line) 커버리지**로 읽었다.

1. **기댈 기계적 정의가 없다.** `vite.config.ts` 의 coverage 블록에는 `thresholds` 설정이 없다(`provider` · `reporter` · `include` · `exclude` 뿐). 프로젝트가 어떤 지표를 게이트로 삼는지 코드로 못박은 적이 없으므로, 뜻은 일관된 관행에서 나올 수밖에 없다.
2. **저장소의 모든 SPEC 이 단일 수치로만 적어 왔다.** `.moai/specs/` 전체에서 branch 커버리지를 언급한 문서는 하나도 없다(`grep -rln "브랜치 커버리지\|branch coverage\|% Branch" .moai/specs/` 무출력). SPEC-HEATMAP-PANEL-001 은 완료 행에 "신규 코드 커버리지 99%" 라고만 적었다.
3. **가장 가까운 선례가 같은 파일에 대한 것이다.** SPEC-CHART-004 §8 AC-32 는 `PanelDragLayer.tsx 96.82%` 라고 적었다 — 이 표의 **% Stmts** 값이고, 그 파일의 % Branch 는 88.88 이다. 두 SPEC 은 같은 허용목록을 공유하므로, 같은 파일의 같은 수치를 한쪽은 statement 로 다른 쪽은 branch 로 읽으면 기록이 서로 어긋난다.

**과장하지 않기 위해 함께 적는다**: branch 기준으로는 `usePanelElementEdit.tsx` 63.63 · `PanelResizeOverlay.tsx` 68.75 · `previewGridSize.ts` 71.42 · `PanelGridBackdrop.tsx` 77.77 **4종이 85% 미만**이다. 이 판정에서는 지표 밖이지만 사실은 사실이므로, **후속 보강 대상**으로 남긴다 — 프로젝트가 뒷날 branch 를 게이트 지표로 채택하면 정확히 이 4종이 걸린다. 그때는 이 SPEC 을 다시 여는 것이 아니라 별도의 테스트 보강 작업으로 다룬다.

### 열린 항목 판정 (완료를 막는가 / 후속으로 남는가)

| # | 항목 | 판정 | 근거 |
|---|------|------|------|
| 1 | plan.md M7.4 — 커버리지 `include` 미갱신 | **해소됨 (더는 막지 않는다)** | 허용목록을 실제 모듈로 갱신하고 `npx vitest run --coverage` 로 실측해, statement 기준 11종 전량 85% 이상(최저 96.82%)을 확인했다. 판정 지표를 statement 로 읽은 근거는 위 "M7.4 판정 지표" 절에 있다. **branch 4종 85% 미만은 후속 보강 대상으로 남는다** |
| 2 | plan.md M2.5 — export 이름 미정리 (IN-1) | 막지 않는다 | §2 의 **어느 요구도 export 이름을 규정하지 않는다**. U1-1~U1-3 은 "패널 중립 모듈이 소유한다 / 중립 이름으로 옮긴다" 를 요구하고 파일·타입은 그것을 충족한다(`panelEditAlign.ts` · `panelEditSelection.ts` · `PanelEditGrid` · `PanelAlignToolbar` 는 파일과 심볼이 모두 중립이다). U1-4 는 대상 맵을 요구하고 그것도 충족한다. M2.5 의 산출물 열도 파일(`PanelDragLayer.tsx`)을 적었다. **이름 위생 부채이지 요구 미충족이 아니다** — 다만 §5 D1 이 복사를 거부한 논리와 같은 이유로 남겨 두면 다음 사람이 "이것은 통계용인가" 를 되묻게 되므로, 후속 정리 대상으로 남긴다 |
| 3 | 게이지 범위 이탈 (IN-4) | 막지 않는다 | §2 의 U1~U5 중 게이지를 요구하는 항목이 없고, §6 은 게이지를 범위 밖으로 적었다. 즉 게이지 작업은 **이 SPEC 의 인수 조건을 하나도 건드리지 않는다** — 그것을 이유로 본 SPEC 을 막아도 게이지 작업이 정리되지는 않는다. 다만 `previewLiveKeys.ts:29` 가 `threshold_legend_font_size` 를 "(SPEC-CHART-005)" 로 귀속시켜 **잘못된 소유권 표시**를 남겼고, `panels/gauge/` 의 어느 파일에도 `@spec` 이 없다. **어느 SPEC 도 요구하지 않은 작업**이므로 별도 SPEC 으로 분리해 귀속을 바로잡아야 한다 — 후속 필수 |
| 4 | 게이지 값 좌표 이관 누락 (IN-5) | **본 SPEC 의 범위 밖 — 막지 않는다. 다만 사용자 영향이 있는 회귀다** | 항목 3 의 판정에 따라 게이지는 이 SPEC 의 범위가 아니므로 이 SPEC 의 완료 판정에는 들어가지 않는다. 그러나 결함 자체는 실재한다: 값 글자의 오프셋이 viewBox 단위 `value_offset_x/y` 에서 패널 대비 백분율 `value_pos_x/y` 로 바뀌었는데 **옛 키를 읽는 코드가 어디에도 없다**(`grep -rn "value_offset_x" web/src/` → `PanelSettingsDialog.tsx:7213`(초기화 시 지우는 경로)와 `GaugePanel.test.tsx`(초기화가 그 키를 건드리지 않음을 단언하는 AC-20) 둘뿐). 이관 코드도 없으므로 **저장된 대시보드에서 값 글자를 끌어 옮겨 두었다면 그 자리를 잃고 기본 위치로 돌아간다.** 항목 3 이 요구하는 게이지 SPEC 의 첫 항목으로 다뤄야 한다 |

**완료 시점에 남는 후속 작업** (어느 것도 §2 의 요구를 미충족시키지 않으므로 완료를 막지 않는다):

1. **게이지 작업의 SPEC 귀속** (항목 3) — 별도 SPEC 분리 + `previewLiveKeys.ts:29` 의 "(SPEC-CHART-005)" 오귀속 정정 + `panels/gauge/` 에 `@spec` 부여. **필수.**
2. **게이지 값 좌표 이관** (항목 4) — `value_offset_x/y` → `value_pos_x/y` 읽기 폴백 또는 이관 코드. 저장된 대시보드에서 값 글자 자리를 잃는 **실사용 회귀**이므로 위 1의 첫 항목으로 다룬다.
3. **공용 모듈 export 이름 정리** (항목 2) — `StatDragLayer` · `StatResizeHandle` · `StatDragTarget` · `StatSelection` · `StatElementBox` → 중립 이름, 소비처 별칭 제거.
4. **branch 커버리지 보강** — `usePanelElementEdit.tsx` 63.63 · `PanelResizeOverlay.tsx` 68.75 · `previewGridSize.ts` 71.42 · `PanelGridBackdrop.tsx` 77.77.

### 요구사항 확인 (§2 U1~U5)

| 요구 | 확인 근거 |
|------|-----------|
| U1-1 · U1-2 | `panels/charts/panelEditAlign.ts:131` `computeAlignPatches<K extends string>` · `panelEditSelection.ts:14,26` — 요소 종류가 문자열 제네릭이다 |
| U1-3 | `PanelEditGrid.tsx` · `PanelAlignToolbar.tsx` 로 파일명이 옮겨졌다 (다만 IN-1 참고) |
| U1-4 | `PanelDragLayer.tsx` 가 `targets: Record<K, DragTarget>` 를 받는다. 표식도 `data-panel-drag` · `data-panel-resize` 로 공용화되었다 |
| U1-5 | 통계 테스트 전량이 단언을 고치지 않고 통과한다(`StatPanel.test.tsx` · `PanelDragLayer.test.tsx` 27건 · `statLayout.test.ts` 23건) |
| U2-1 ~ U2-4 | `BarChartPanel.tsx:31` 이 `PieLegend` 를 그대로 쓰고 `:111` 이 `show_legend` 기본값을 `false` 로 읽는다. `BarChartPanel.test.tsx` 의 AC-05/AC-06 4건이 기본 꺼짐 · 항목 노출 · 비중 미표기 · 파이와 같은 키를 잠근다 |
| U3-1 ~ U3-3 | `BarChartPanel.tsx:64,277` `usePanelEditMode` · `:127~128` `plot_offset_x/y` · `:124` `plot_size`. `BarChartPanel.test.tsx` AC-07/AC-08 |
| U4-1 ~ U4-6 | 바: AC-08/AC-09/AC-10/AC-16. 파이: `PieChartPanel.test.tsx` AC-11~AC-16. 두 패널 모두 `PanelEditGrid` · `PanelAlignToolbar` · `usePanelElementEdit` 를 붙였고, 크기 핸들은 파이 `pie_size`(%) · 바 `plot_size`(%) · 범례 글자 크기(px)를 각각 바꾼다 |
| U5-1 · U5-2 | `previewLiveKeys.ts` 가 `pie_size` · `pie_offset_x/y` · `legend_offset_x/y` · `legend_font_size` · `legend_font_family` · `legend_font_color` · `legend_position` · `show_legend` · `plot_offset_x/y` 를 담는다 |

### 분기 (Divergence, as-implemented)

- **IN-1 — 파일 이름만 공용화되고 export 이름은 통계 이름 그대로다.** plan.md M2.5 는 "`StatDragLayer` → `PanelDragLayer`" 로 적었으나, 실제로는 파일만 `PanelDragLayer.tsx` 로 옮겨졌고 그 안의 export 는 여전히 `StatDragLayer` · `StatResizeHandle` · `StatDragTarget` 이다. `panelEditSelection.ts` 는 `StatSelection` 을, `panelEditAlign.ts` 는 `StatElementBox` 를 export 한다. 소비처는 `import { StatDragLayer as PanelDragLayer } from '../../PanelDragLayer'` 처럼 **가져오는 자리에서 별칭**을 붙여 쓰고 있다(`BarChartPanel.tsx:66` · `PieChartPanel.tsx:45`). 동작은 같지만, §5 D1 이 복사를 거부한 이유("사본이 셋이면 다음 실수는 세 곳에서 각각 발견된다")와 같은 논리로 이름이 갈리면 다음 사람이 "이것은 통계용인가" 를 매번 되묻게 된다. 이름 정리는 남은 일이다.
- **IN-2 — plan.md §2 가 예고하지 않은 파일이 6개 생겼다.** `usePanelElementEdit.tsx`(본 SPEC 의 `@spec` 을 달고 있다) · `PanelResizeOverlay.tsx` · `PanelGridBackdrop.tsx` + 테스트 · `panelEditContext.ts` · `previewStage.ts` + 테스트 · `previewGridSize.ts` + 테스트. 이 중 `usePanelElementEdit.tsx` 를 뺀 다섯은 `@spec` 주석이 없어 소유 SPEC 을 코드만으로 판정할 수 없다. `usePanelElementEdit` 는 §2.1 U1 이 요구한 공용화의 자연스러운 귀결(선택·스냅·정렬·초기화 상태를 세 패널이 함께 쓴다)이므로 설계 위반은 아니고 계획 표의 누락이다.
- **IN-3 — 바 차트 config 가 §3.1 이 적은 것보다 넓다.** §3.1 은 `plot_offset_x` · `plot_offset_y` · `plot_size` 셋만 신설한다고 적었으나, 구현은 `plot_size_y`(세로 배율)와 `bar_size`(막대 굵기 px)를 함께 들였다. 대응 테스트가 `BarChartPanel.test.tsx` 에 3건 있고 미지정 시 종전 동작을 보존하지만, 데이터 계약 문서가 이 두 축을 말하지 않는다.
- **IN-4 — 게이지가 함께 손질되었으나 §6 은 게이지를 범위 밖으로 적었다.** `previewLiveKeys.ts:29` 는 `threshold_legend_font_size` 를 "(SPEC-CHART-005)" 로 귀속시키고, 임계값 범례에 크기 손잡이가 붙었으며(`GaugePanel.tsx:520-534,589-598`), 세로바 게이지에 `gauge_bar_width` · `gauge_bar_height` 축이 신설되었고, 현재값 글자가 도형 SVG 밖 오버레이로 나왔다(`gaugeValue.tsx` 신설, `valueDrag.ts` 삭제). §6 은 "게이지에 그리드·정렬 더하기" 만 범위 밖으로 적었으므로 이 작업들이 전부 그 문장에 걸리지는 않지만, 어느 SPEC 도 이들을 요구하지 않았다 — `panels/gauge/` 의 어느 파일에도 `@spec` 주석이 없다. 별도 SPEC 으로 분리하거나 본 SPEC 의 범위를 넓혀 적어야 한다.
- **IN-5 — 게이지 현재값 좌표계가 바뀌었는데 이관 코드가 없다.** 값 글자의 오프셋은 viewBox 단위 `value_offset_x/y` 에서 패널 대비 백분율 `value_pos_x/y` 로 바뀌었다. 옛 키를 읽는 코드는 어디에도 없고(`PanelSettingsDialog.tsx:7213` 의 초기화 경로가 유일하게 그 키를 지운다), 이관 코드도 없다. 저장된 대시보드에서 값 글자를 끌어 옮겨 두었다면 그 자리를 잃고 기본 위치로 돌아간다.
