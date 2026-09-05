---
id: SPEC-CHART-003
title: 통계 패널 표시 옵션 재구성 — 죽은 스타일 컨트롤 정리 + 변화량 표기 설정화 + 구간 통계 보조 줄
version: 0.1.0
status: draft
created: 2026-09-05
updated: 2026-09-05
author: xtra
priority: medium
domain: dashboard
related_specs:
  - SPEC-CHART-001
  - SPEC-CHART-002
  - SPEC-PANEL-SETTINGS-001
  - SPEC-WEB-005
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-09-05 | xtra | 최초 작성 — 통계(stat) 패널의 설정 표면을 세 갈래로 손본다. (1) 패널 설정 다이얼로그의 "스타일" 섹션에서 stat 이 읽지 않는 악센트 그룹 3종을 걷어내고 살아 있는 `panelColor` 만 한 줄로 남긴다. (2) 지금 레거시 경로에만 하드코딩되어 있는 변화량 표기를 **설정 가능한 축**(표시 여부 · 증감별 색 · 크기)으로 승격하고 다중 타일 경로에도 낸다 — SPEC-CHART-002 §7 OQ5 의 "타일 보조 지표 미표시" 결정을 뒤집는다. (3) 구간 평균·최대·최소를 본값 아래 한 줄 인라인 보조 줄로 신설한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 세 줄이다.

```
통계 패널
- 스타일 삭제
- 변화량 표시, 증가/감소에 따른 색 설정, 크기 옵션
- 구간 평균, 최대, 최소 표시 옵션
```

세 축으로 분해한다.

1. **죽은 설정 표면 정리** — stat 패널의 "스타일" 섹션은 그룹 4개 중 3개가 아무 데도 닿지 않는다. 죽은 3개를 걷어내고, 살아 있는 하나(`panelColor`)는 고르기-편집 2단계 없이 색상 한 줄로 축소한다.
2. **변화량의 설정화** — 지금 변화량은 "레거시 경로에서, 끌 수 없이, 상승은 항상 녹색으로, 고정 크기로" 그려진다. 네 가지가 전부 하드코딩이다. 이를 표시 여부 · 증가색 · 감소색 · 변화없음색 · 크기의 설정 축으로 승격하고, 다중 타일 경로에도 낸다.
3. **구간 통계 보조 줄** — 시간 윈도우의 평균·최대·최소를 항목별로 켜서 본값 아래 한 줄에 인라인 배치한다.

### 1.2 배경 — 현재 코드가 놓인 자리

#### 1.2.1 stat 의 "스타일" 섹션은 4분의 3이 죽어 있다

[`PanelSettingsDialog.tsx:1382`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx) 는 목록형 패널 5종(`flows` · `agents` · `devices` · `properties-grid` · `agent-status`)만 스타일 섹션에서 제외하고, 나머지 전부에 `AccentGroupPicker` + `AccentGroupControls` 를 낸다. `stat` 은 제외 목록에 없으므로 이 섹션을 받는다.

stat 이 받는 그룹 목록은 `LIST_ACCENT_LABEL_KEYS`([`:6399`](../../../web/src/pages/dashboard/PanelSettingsDialog.tsx))로 4개다.

| 그룹 | 저장 위치 | 소비처 | 상태 |
|------|-----------|--------|------|
| `header` | `config.accentElements.header` | 없음 | **죽음** |
| `badges` | `config.accentElements.badges` | 없음 | **죽음** |
| `table` | `config.accentElements.table` | 없음 | **죽음** |
| `_base` | `config.panelColor` | [`DashboardPage.tsx:782`](../../../web/src/pages/dashboard/DashboardPage.tsx) — 격자 래퍼의 좌/상단 테두리 악센트 | **살아 있음** |

`accentElements` 를 실제로 읽는 패널은 `SingleDevicePanel` · `LogPanel` · `PropertiesGridPanel` 셋뿐이다. [`StatPanel.tsx`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx) 에는 `accentElements` 라는 낱말이 없다. 즉 사용자가 헤더·배지·표 색을 아무리 골라도 화면은 바뀌지 않는다. 이는 같은 파일 `:1377` 의 주석이 `agent-status` 를 제외한 사유("이 패널은 accentElements 를 **읽지 않아** 그룹을 골라 색을 정해도 화면이 바뀌지 않는 죽은 컨트롤이었다")와 글자 그대로 같은 상황이다.

반면 `_base` 는 죽지 않았다. `panelColor` 는 대시보드 격자에서 실제로 그려지고, **편집할 수 있는 다른 입구가 없다** — [`PanelSettingsDropdown.tsx`](../../../web/src/components/common/PanelSettingsDropdown.tsx) 에 색상 선택 UI가 존재하지만 그 컴포넌트는 어디에서도 import 되지 않는 미사용 코드다.

따라서 "스타일 삭제" 는 섹션의 **통째 제거가 아니라 축소**로 실현한다(§5 결정 D1).

#### 1.2.2 변화량은 하드코딩 4종 세트다

[`StatPanel.tsx:130-166`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx) 의 레거시 경로가 변화량을 계산하고 `:255-269` 가 그린다. 네 가지가 설정 밖에 있다.

| 축 | 현재 | 문제 |
|----|------|------|
| 표시 여부 | 계산되면 무조건 표시 | 본값만 크게 보고 싶은 패널에서 끌 수 없다 |
| 증감 색 | `text-emerald-500` / `text-rose-500` 하드코딩 | 상승이 항상 좋은 것은 아니다 — 에너지 사용량·오류율은 상승이 나쁘다. 지표의 뜻과 색이 어긋난다 |
| 크기 | `text-sm` 고정 | 본값은 `value_scale` 로 키울 수 있는데 변화량은 따라오지 않아, 키울수록 두 줄의 균형이 무너진다 |
| 경로 | 레거시 경로 전용 | SPEC-CHART-002 §7 OQ5 결정으로 다중 타일 경로에는 아예 없다 |

#### 1.2.3 구간 통계의 계산은 이미 있고, 표시만 없다

[`seriesReduce.ts`](../../../web/src/pages/dashboard/panels/charts/seriesReduce.ts) 의 `reduceSeries()` 가 `max` · `avg` · `min` · `last` · `sum` · `count` · `delta` 7종을 단일 패스로 계산한다. 표본 정규화(null · 비유한 · 비수치 제외) 규칙도 이 모듈이 이미 소유한다.

다만 현재 구조는 **패널당 대표값 1종**(`series_reduce`)을 골라 본값 자리에 쓰는 것이다. "본값은 마지막 값, 그 아래 평균·최대·최소를 함께" 는 대표값 축이 표현할 수 없다 — 축이 하나뿐이기 때문이다. 따라서 표시 전용의 **보조 축**을 신설한다(§2.3).

---

## 2. 요구사항 (EARS)

### 2.1 스타일 섹션 축소 [U1]

- **[U1-1]** Where 패널 타입이 `stat` 인 경우, the 패널 설정 다이얼로그 shall `AccentGroupPicker` 와 `AccentGroupControls` 를 렌더하지 않는다.
- **[U1-2]** Where 패널 타입이 `stat` 인 경우, the 패널 설정 다이얼로그 shall 스타일 섹션 자리에 **패널 색상 선택 한 줄**(색상 스와치 목록 + 초기화)을 렌더하고, 그 값은 `config.panelColor` 에 저장한다.
- **[U1-3]** The 축소된 패널 색상 컨트롤 shall 그룹 고르기 단계 없이 곧바로 색을 바꾼다 — 항목이 하나뿐인 목록에 "고르기 → 편집" 2단계를 남기면 빈 껍데기가 된다.
- **[U1-4]** The 축소 작업 shall 저장된 `config.accentElements` 값을 삭제하거나 변경하지 않는다 — 편집 입구만 사라지고 데이터는 그대로 둔다(다른 패널 타입으로 바꿨을 때 값이 되살아나야 한다).
- **[U1-5]** The 축소 작업 shall `stat` 외 패널 타입의 스타일 섹션 동작을 바꾸지 않는다.

### 2.2 변화량 설정화 [U2]

- **[U2-1]** The stat 패널 config shall 변화량 설정 객체 `delta_display` 를 갖는다. 하위 필드는 `enabled` · `up_color` · `down_color` · `flat_color` 이며 모두 옵셔널이다.
- **[U2-2]** Where `delta_display.enabled` 가 명시적으로 `false` 인 경우, the stat 패널 shall 변화량 줄을 렌더하지 않는다.
- **[U2-3]** Where `delta_display.enabled` 가 미지정인 경우, the stat 패널 shall **레거시(단일 값) 경로에서는 변화량을 표시하고, 다중 타일 경로에서는 표시하지 않는다.** 미지정의 뜻이 경로마다 다른 이유는 §5 결정 D2 가 소유한다.
- **[U2-4]** Where `delta_display.enabled` 가 `true` 인 경우, the stat 패널 shall 두 경로 모두에서 변화량을 표시한다. 다중 타일 경로에서는 타일마다 그 시리즈의 변화량을 그린다.
- **[U2-5]** The 변화량 기준선 shall **직전 표본**이다 — 마지막 표본의 값에서 그 앞 표본의 값을 뺀다. 구간 시작(first) 대비가 아니다.
- **[U2-6]** Where 변화량이 양수인 경우, the stat 패널 shall `up_color` 를 글자색으로 쓴다. 음수면 `down_color`, 0 이면 `flat_color` 를 쓴다.
- **[U2-7]** Where 해당 색이 미지정인 경우, the stat 패널 shall 현행 기본색을 쓴다 — 상승 `#10b981`(emerald-500) · 하락 `#f43f5e`(rose-500) · 변화없음 `--color-text-muted`.
- **[U2-8]** The 변화량 표기 shall 본값과 같은 단위·자릿수 규칙(`formatValueWithUnit`)을 따른다 — 현행 동작을 그대로 보존한다.
- **[U2-9]** Where 변화량을 계산할 표본이 2개 미만이거나 값이 비수치인 경우, the stat 패널 shall 변화량 줄을 렌더하지 않는다 — 현행 동작을 그대로 보존한다.
- **[U2-10]** The 다중 타일 경로의 변화량 shall **시리즈별로** 계산한다 — 시리즈를 가로질러 마지막 두 값을 비교하지 않는다.

### 2.3 구간 통계 보조 줄 [U3]

- **[U3-1]** The stat 패널 config shall 구간 통계 설정 객체 `window_stats` 를 갖는다. 하위 필드는 `avg` · `max` · `min` 이며 모두 옵셔널 불리언이다.
- **[U3-2]** Where `window_stats` 의 어떤 하위 필드도 `true` 가 아닌 경우, the stat 패널 shall 구간 통계 줄을 렌더하지 않는다. 이것이 기본 상태다 — 신규 축이므로 켜지 않으면 아무 변화가 없다.
- **[U3-3]** Where `window_stats` 의 하위 필드가 하나 이상 `true` 인 경우, the stat 패널 shall 켜진 항목을 `평균 → 최대 → 최소` 순서로 **한 줄에 인라인** 배치한다. 순서는 켜진 조합과 무관하게 고정이다.
- **[U3-4]** The 구간 통계 값 shall `reduceSeries()` 의 `avg` · `max` · `min` 계산을 그대로 쓴다 — 표본 정규화 규칙을 복제하지 않는다.
- **[U3-5]** The 구간 통계의 "구간" shall 패널이 이미 로드한 표본 전체다. 레거시 경로는 채널 버퍼(`max_points`)이고, 다중 타일 경로는 시리즈별 조회 윈도우다.
- **[U3-6]** Where 어떤 항목의 값이 `undefined`(표본 0개)인 경우, the stat 패널 shall 그 항목 자리에 `—` 를 그린다 — 항목을 통째로 빼면 켜 둔 항목의 자리가 조합마다 움직인다.
- **[U3-7]** The 구간 통계 표기 shall 본값과 같은 단위·자릿수 규칙을 따른다.
- **[U3-8]** Where 레거시(채널) 경로에서 `window_stats` 가 켜진 경우, the 패널 설정 UI shall `max_points` 가 구간 통계의 표본 수를 정한다는 안내를 표시한다. 근거는 §5 결정 D4.

### 2.4 보조 줄 크기 [U4]

- **[U4-1]** The stat 패널 config shall 보조 줄 크기 배율 `sub_value_scale` 을 갖는다.
- **[U4-2]** The `sub_value_scale` shall 변화량 줄과 구간 통계 줄에 **함께** 적용된다 — 두 보조 줄이 서로 다른 배율을 가지면 어긋난다(§5 결정 D3).
- **[U4-3]** The `sub_value_scale` shall `readValueScale()` 의 클램프 규칙(하한 0.3 · 상한 3 · 비수치는 1)을 그대로 쓴다.
- **[U4-4]** Where `sub_value_scale` 이 미지정인 경우, the 보조 줄 shall 배율 1 로 그린다 — 즉 현행 `text-sm`(14px)과 같은 크기다.

### 2.5 설정 UI [U5]

- **[U5-1]** The `StatChartSection` shall 변화량 설정(표시 여부 체크박스 · 색상 3종 · 배율)과 구간 통계 설정(평균/최대/최소 체크박스 3개)을 렌더한다.
- **[U5-2]** Where 변화량 표시가 꺼져 있는 경우, the 설정 UI shall 색상 3종 컨트롤을 렌더하지 않는다 — 효과 없는 컨트롤을 남기지 않는다.
- **[U5-3]** The 신규 라벨 shall `ko.json` 과 `en.json` 양쪽에 추가된다.

---

## 3. 데이터 계약

### 3.1 config 스키마 확장

`StatPanelConfig`([`chartChannelTypes.ts:722`](../../../web/src/pages/dashboard/panels/charts/chartChannelTypes.ts))에 세 필드를 더한다. 전부 옵셔널이며, 셋 다 없는 config 는 §2 의 기본값 규칙에 따라 **현행과 같은 화면**을 낸다.

```ts
/** 변화량 표기 설정. 미지정은 "레거시 경로만 켬" 을 뜻한다(§5 D2). */
export interface StatDeltaConfig {
  /** 표시 여부. 미지정 = 경로별 기본값. */
  enabled?: boolean;
  /** 증가(양수) 글자색. 미지정이면 emerald-500. */
  up_color?: string;
  /** 감소(음수) 글자색. 미지정이면 rose-500. */
  down_color?: string;
  /** 변화 없음(0) 글자색. 미지정이면 muted. */
  flat_color?: string;
}

/** 구간 통계 보조 줄 설정. 켠 항목만 그린다. 순서는 avg → max → min 고정. */
export interface StatWindowStatsConfig {
  avg?: boolean;
  max?: boolean;
  min?: boolean;
}

export interface StatPanelConfig extends ChartPanelConfigBase {
  unit?: string;
  decimal_places?: number;
  threshold_color_rules?: Array<{ min: number; color: string }>;
  // --- SPEC-CHART-003 신설 ---
  delta_display?: StatDeltaConfig;
  window_stats?: StatWindowStatsConfig;
  /** 보조 줄(변화량 + 구간 통계) 공용 크기 배율. 본값의 `value_scale` 과 별개 축. */
  sub_value_scale?: number;
}
```

### 3.2 하위 호환

| 저장된 config | 렌더 결과 | 근거 |
|---------------|-----------|------|
| 세 필드 모두 없음 · 레거시 경로 | 본값 + 변화량(현행 색·크기). 구간 통계 없음 | U2-3 · U3-2 |
| 세 필드 모두 없음 · 다중 타일 경로 | 타일 배열만. 변화량·구간 통계 없음 | U2-3 · U3-2 |
| `accentElements` 에 값이 남아 있음 | 화면 변화 없음(원래도 읽지 않았다). 편집 입구만 사라짐 | U1-4 |
| `panelColor` 에 값이 있음 | 격자 테두리 악센트 그대로. 축소된 한 줄에서 편집 가능 | U1-2 |

즉 **저장된 대시보드는 이 SPEC 구현 후에도 외형이 바뀌지 않는다.** 새 표기는 사용자가 설정을 켜야만 나타난다.

---

## 4. 화면 (렌더 형태)

### 4.1 레거시(단일 값) 경로

```
        1,234 kW          ← 본값 (value_scale)
        ↑ +12 kW          ← 변화량 (delta_display, sub_value_scale)
   평균 980 · 최대 1,540 · 최소 210   ← 구간 통계 (window_stats, sub_value_scale)
```

### 4.2 다중 타일 경로

```
  ┌─────────┬─────────┬─────────┐
  │ room1   │ room2   │ room3   │  ← 시리즈 라벨 (시리즈 색)
  │  26.0   │  19.8   │    —    │  ← 대표값 (series_reduce)
  │ ↑ +2.0  │ ↓ -0.4  │    —    │  ← 변화량 (시리즈별)
  │ 평 22.6 │ 평 19.7 │    —    │  ← 구간 통계 (좁으면 라벨 축약)
  └─────────┴─────────┴─────────┘
```

타일 폭이 좁으므로 타일 경로의 구간 통계 라벨은 축약형(`평` · `최대` · `최소`)을 쓴다. 축약 여부는 타일 개수가 아니라 **경로**로 정한다 — 타일이 1개일 때만 전체 라벨을 쓰면 시리즈를 하나 지웠을 때 라벨이 갑자기 길어진다.

### 4.3 접근성

- 변화량의 방향은 화살표 글리프(`↑`/`↓`/`→`)와 부호(`+`/`-`)가 **모두** 전달한다 — 색만으로 방향을 알리지 않는다. 사용자가 증가·감소에 같은 색을 지정해도 방향이 읽힌다.
- 구간 통계의 라벨(`평균`/`최대`/`최소`)은 축약형이라도 스크린리더에는 완결 낱말로 전달한다.

---

## 5. 결정 (Decisions)

### D1 — "스타일 삭제" 는 통째 제거가 아니라 축소다

요청은 "스타일 삭제" 였다. 그러나 §1.2.1 에서 확인했듯 그룹 4개 중 `_base`(`panelColor`)는 살아 있고 **다른 편집 입구가 없다**. 통째로 지우면 저장된 테두리 색은 계속 그려지는데 바꿀 수는 없는 상태가 된다 — 요청이 없애려 한 "쓸모없는 컨트롤" 이 아니라, 새로 만들어지는 "손댈 수 없는 유령 값" 이다.

따라서 죽은 그룹 3개와 고르기-편집 2단계를 없애고 살아 있는 색 하나만 한 줄로 남긴다. 사용자 확인을 거친 결정이다.

### D2 — `delta_display.enabled` 미지정의 뜻은 경로마다 다르다

경로별로 현행 동작이 반대이기 때문이다. 레거시 경로는 변화량을 **항상 그리고** 있었고, 다중 타일 경로는 SPEC-CHART-002 §7 OQ5 결정에 따라 **한 번도 그리지 않았다**. 미지정의 뜻을 한쪽으로 통일하면 반드시 한쪽 저장 대시보드의 외형이 바뀐다.

- 미지정 = 항상 켬 → 기존 다중 타일 대시보드에 변화량 줄이 갑자기 생긴다.
- 미지정 = 항상 끔 → 기존 단일 값 대시보드에서 변화량이 갑자기 사라진다.

둘 다 사용자가 손대지 않은 화면을 바꾸는 것이라 받아들이지 않는다. 대신 미지정을 "현행 유지" 로 정의하고, 명시적 `true`/`false` 가 두 경로에서 똑같이 작동하게 한다. 이 판정은 헬퍼 함수 하나가 소유하며 패널 본문에 삼항식으로 흩어지지 않는다.

이 결정은 SPEC-CHART-002 §7 OQ5("stat 타일 보조 지표 미표시")를 **뒤집는다** — 다만 기본값이 아니라 선택지로 뒤집는다. OQ5 의 근거였던 "타일이 좁아 보조 줄이 들어갈 자리가 없다" 는 여전히 유효하므로, 켜는 것은 사용자의 판단으로 남긴다.

### D3 — 보조 줄 크기는 하나의 축이다

요청은 변화량의 크기 옵션이었다. 그러나 변화량과 구간 통계는 본값 아래 나란히 놓이는 같은 층의 글자다. 각자 배율을 가지면 사용자가 한쪽만 키웠을 때 두 줄의 글자 크기가 어긋난다 — [`StatPanel.tsx:26-33`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx) 의 주석이 값과 단위에 대해 이미 같은 판단("값과 단위가 **함께** 커져야 한다")을 기록해 두었다.

따라서 설정 항목은 하나(`sub_value_scale`)이고 두 줄에 함께 적용된다. 설정 항목 수도 하나 줄어든다.

### D4 — 채널 모드의 구간 통계는 `max_points` 에 갇힌다

[`StatPanel.tsx:59`](../../../web/src/pages/dashboard/panels/charts/StatPanel.tsx) 의 `parseConfig` 는 `max_points` 기본값을 **2** 로 둔다. 변화량 계산에 직전 값 하나만 있으면 되기 때문이다.

이 기본값 아래에서 구간 통계를 켜면 평균·최대·최소가 전부 표본 2개에서 나온다 — 값은 나오지만 "구간" 이라는 낱말이 뜻하는 바가 아니다. 자동으로 올리지 않는 이유는, `max_points` 가 채널 버퍼 길이라 조용히 늘리면 메모리 사용과 갱신 비용이 사용자 모르게 바뀌기 때문이다.

대신 설정 UI 가 안내를 낸다(U3-8). 다중 타일 경로는 조회 윈도우가 표본 수를 정하므로 이 함정이 없다.

### D5 — 선행 결함은 고치지 않는다

SPEC-CHART-002 v0.3.0 이 범위 밖으로 분리한 선행 결함 중 하나가 "stat store 모드 delta 의 시리즈 교차 비교" 다 — 시리즈 소스가 활성인데 `series_reduce` 가 없으면 레거시 경로로 떨어지고, 그 경로의 `entries` 는 시리즈를 가로질러 평탄화되어 있어 변화량이 서로 다른 시리즈의 두 값을 뺀 수가 될 수 있다.

본 SPEC 은 이 결함을 고치지 않는다(범위 밖). 다만 신설되는 다중 타일 경로의 변화량은 **시리즈별로** 계산하므로(U2-10) 새 코드가 같은 결함을 재생산하지는 않는다.

---

## 6. 범위 밖 (Out of Scope)

- 게이지 · 바 · 파이 패널의 변화량/구간 통계 — 본 SPEC 은 stat 만 다룬다.
- `series_reduce` 축의 변경 — 대표값 선택 방식은 그대로다.
- §5 D5 의 선행 결함(레거시 경로 store 모드 delta 의 시리즈 교차 비교) 수정.
- `PanelSettingsDropdown` 미사용 컴포넌트의 삭제 여부 — 별개 정리 대상이다.
- 다른 패널 타입의 스타일 섹션 정리 — 같은 죽은 컨트롤 문제가 다른 타입에도 있을 수 있으나 확인하지 않았다.

---

## 7. 열린 질문 (Open Questions)

| # | 질문 | 잠정 결정 |
|---|------|-----------|
| OQ1 | 구간 통계에 `sum` · `count` 도 노출할까 | 하지 않는다 — 요청은 평균·최대·최소 셋이다. 필요해지면 `window_stats` 에 필드를 더하면 되고 스키마는 그 확장을 이미 감당한다 |
| OQ2 | 타일 경로에서 보조 줄 2개가 다 켜지면 타일 높이가 모자라지 않나 | 그리드는 이미 `tile_rows` 로 행 수를 사용자가 정한다. 좁으면 사용자가 행을 줄이거나 보조 줄을 끈다 — 자동 숨김은 넣지 않는다(어떤 화면 폭에서 무엇이 사라지는지 예측할 수 없게 된다) |
| OQ3 | 변화량 색을 임계값 색상 규칙과 합칠까 | 합치지 않는다 — 임계값 규칙은 **값의 크기**로 본값 색을 정하고, 변화량 색은 **변화의 방향**으로 보조 줄 색을 정한다. 축이 다르다 |
