# SPEC-CHART-003 인수 조건

관련 문서: [spec.md](./spec.md) · [plan.md](./plan.md)
형식: Given-When-Then + 기계 검증 가능한 단언(vitest 파일·테스트명, 또는 grep/tsc 명령과 기대 출력)

## 공통 픽스처

기존 `charts/__mocks__` 와 `StatPanelStoreSource.test.tsx` 의 모킹 패턴을 그대로 쓴다.

**F1 — 채널 모드 entries** (`display_field: 'value'`, `max_points: 10`)

| # | value |
|---|-------|
| e0 | 20 |
| e1 | 22 |
| e2 | 26 |
| e3 | 24 |
| e4 | 21 |

기대치: 마지막 값 `21` · 변화량 `21 − 24 = -3`(↓) · 평균 `22.6` · 최대 `26` · 최소 `20`

**F2 — 시리즈 매트릭스** (`series_reduce: 'last'`, 시리즈 3개)

| 시리즈 | b0 | b1 | b2 | b3 | b4 |
|--------|----|----|----|----|----|
| `room1` | 20 | 22 | 26 | 24 | 21 |
| `room2` | 18 | `null` | 19 | 19 | 23 |
| `room3` | `null` | `null` | `null` | `null` | `null` |

기대치:

| 시리즈 | last | 변화량(직전 표본 대비) | avg | max | min |
|--------|------|------------------------|-----|-----|-----|
| `room1` | 21 | `-3` (↓) | 22.6 | 26 | 20 |
| `room2` | 23 | `+4` (↑) | 19.75 | 23 | 18 |
| `room3` | `undefined` | 없음 | `undefined` | `undefined` | `undefined` |

`room2` 의 변화량이 `+4` 인 이유: 표본은 `18 · 19 · 19 · 23` 이고 `null` 은 표본이 아니다. 직전 표본은 `19` 다.

---

## A. 패널 색상 이관과 스타일 섹션 정리 (U1)

### AC-01 — stat 패널 설정에 스타일 섹션이 없다

```gherkin
Given 패널 타입이 stat 인 패널 설정 다이얼로그가 열려 있을 때
When 스타일 섹션을 검사하면
Then accent-group-picker 요소가 존재하지 않고, 죽은 그룹 4종의 버튼도 없다
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — "accent-group-picker 가 존재하지 않는다" · "죽은 그룹(header · badges · table)의 편집 입구가 사라졌다"

### AC-02 — 패널 색상은 패널 옵션에서 모든 타입이 공유한다

```gherkin
Given 임의의 패널 타입으로 패널 설정 다이얼로그가 열려 있을 때
When 패널 옵션 섹션을 검사하면
Then panel-color-row 가 존재한다
And 스와치를 클릭하면 config.panelColor 가 그 색으로 저장된다
And 초기화를 누르면 config.panelColor 가 undefined 가 된다
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — "스와치를 누르면 panelColor 가 그 색으로 저장된다" · "저장된 색이 있으면 초기화로 지울 수 있다" · "_base 그룹은 어느 패널 타입에서도 더 이상 나오지 않는다"

### AC-02b — 편집 입구가 없던 5종이 이제 색을 지정할 수 있다

```gherkin
Given 패널 타입이 flows · agents · devices · agent-status 중 하나일 때
When 패널 옵션 섹션을 검사하면
Then panel-color-row 가 존재한다
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — "스타일 섹션이 없던 목록형 패널도 이제 패널 색상을 지정할 수 있다"

### AC-03 — 저장된 accentElements 를 건드리지 않는다

```gherkin
Given config.accentElements 에 { header: '#ff0000' } 가 저장된 stat 패널이 있을 때
When 패널 색상을 바꾸면
Then 저장된 accentElements 가 그대로 유지된다
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — "패널 색상을 바꿔도 accentElements 값이 유지된다"

### AC-04 — 다른 패널 타입의 악센트 그룹은 그대로다

```gherkin
Given 패널 타입이 graph-chart 또는 bar-chart 인 패널 설정 다이얼로그가 열려 있을 때
When 스타일 섹션을 검사하면
Then accent-group-picker 가 존재한다 (단, _base 그룹은 제외되어 있다)
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — "graph-chart 패널은 악센트 그룹 고르기를 계속 받는다" · "bar-chart 패널도 그대로다"
vitest: `PanelSettingsDialog.previewResize.test.tsx` — 그룹 선택 동작은 `header` 그룹으로 검증한다

### AC-04b — `_base` 가 어디에도 남아 있지 않다

```bash
grep -n "_base: 'dashboard.settings.accent.base'" web/src/pages/dashboard/PanelSettingsDialog.tsx
# 기대: 출력 없음 (exit 1)

grep -n "overallColor" web/src/pages/dashboard/AcControlStyleSection.tsx
# 기대: 출력 없음 (exit 1) — ac-control 의 중복 "전체 색상" 행 제거
```

### AC-05 — 미사용 컴포넌트에 의존을 만들지 않는다

```bash
grep -n "PanelSettingsDropdown" web/src/pages/dashboard/PanelSettingsDialog.tsx
# 기대: 출력 없음 (exit 1)
```

---

## B. 변화량 설정화 (U2)

### AC-06 — 타입이 config 에 존재한다

```bash
grep -n "delta_display\|StatDeltaConfig" web/src/pages/dashboard/panels/charts/chartChannelTypes.ts
# 기대: StatDeltaConfig 인터페이스 정의 + StatPanelConfig 의 delta_display 필드
```

`npx tsc --noEmit` — 오류 0

### AC-07 — enabled:false 는 두 경로 모두에서 변화량을 없앤다

```gherkin
Given F1 채널 데이터와 delta_display: { enabled: false } 가 주어졌을 때
When stat 패널을 렌더하면
Then stat-delta 요소가 존재하지 않는다
```

시리즈 경로(F2 + `series_reduce: 'last'`)에 대해서도 같은 단언을 둔다.

vitest: `StatPanel.test.tsx` — "delta_display.enabled=false 면 레거시 경로에서 변화량이 없다"
vitest: `StatPanel.multiOutput.test.tsx` — "delta_display.enabled=false 면 타일에 변화량이 없다"

### AC-08 — 미지정 기본값은 경로마다 다르다 (D2)

```gherkin
Given delta_display 가 미지정일 때
When 레거시 경로로 렌더하면
Then stat-delta 가 존재한다
And 다중 타일 경로로 렌더하면 stat-tile-delta 가 존재하지 않는다
```

vitest: `statDisplayOptions.test.ts` — "readDeltaEnabled: 미지정 + legacy → true"
vitest: `statDisplayOptions.test.ts` — "readDeltaEnabled: 미지정 + tile → false"
vitest: `StatPanel.multiOutput.test.tsx` — "delta_display 미지정이면 타일에 변화량이 없다(현행 보존)"

### AC-09 — enabled:true 는 두 경로 모두에서 변화량을 낸다

```gherkin
Given F2 시리즈 데이터와 series_reduce:'last', delta_display:{ enabled: true } 가 주어졌을 때
When stat 패널을 렌더하면
Then room1 타일의 변화량이 "↓ -3", room2 타일이 "↑ +4" 이고 room3 타일에는 변화량 줄이 없다
```

vitest: `StatPanel.multiOutput.test.tsx` — "enabled=true 면 타일마다 그 시리즈의 변화량을 그린다"

### AC-10 — 변화량은 시리즈별로 계산된다 (U2-10)

```gherkin
Given F2 시리즈 데이터가 주어졌을 때
When 타일 경로에서 변화량을 계산하면
Then room2 의 변화량은 19 → 23 의 +4 이며, 다른 시리즈의 값이 섞이지 않는다
```

vitest: `StatPanel.multiOutput.test.tsx` — "타일 변화량은 시리즈 경계를 넘지 않는다"

### AC-11 — 증감별 색이 설정을 따른다

```gherkin
Given delta_display: { enabled: true, up_color: '#123456', down_color: '#654321', flat_color: '#abcdef' } 가 주어졌을 때
When 변화량이 각각 양수/음수/0 인 데이터로 렌더하면
Then stat-delta 의 인라인 color 가 각각 그 값이다
```

vitest: `StatSubLines.test.tsx` — "변화량 색이 방향별 설정을 따른다"

### AC-12 — 색 미지정 시 현행 기본색이다

```gherkin
Given delta_display 에 색이 지정되지 않았을 때
When readDeltaColors 를 호출하면
Then { up: '#10b981', down: '#f43f5e', flat: 'var(--color-text-muted)' } 를 반환한다
```

vitest: `statDisplayOptions.test.ts` — "readDeltaColors: 미지정은 현행 기본색"

### AC-13 — 방향은 색 없이도 읽힌다 (§4.3)

```gherkin
Given up_color 와 down_color 를 같은 값으로 지정했을 때
When 양수/음수 변화량을 렌더하면
Then 각각 "↑" 와 "+", "↓" 와 "-" 가 텍스트에 포함된다
```

vitest: `StatSubLines.test.tsx` — "방향은 화살표와 부호가 함께 전달한다"

### AC-14 — 기준선은 직전 표본이다 (U2-5)

```gherkin
Given F1 채널 데이터(20 22 26 24 21)가 주어졌을 때
When 변화량을 계산하면
Then -3 이다 (21 - 24). 구간 시작 대비인 +1 (21 - 20) 이 아니다
```

vitest: `StatPanel.test.tsx` — "변화량 기준선은 직전 표본이다"

### AC-15 — 표본 부족·비수치는 변화량 줄이 없다 (U2-9, 현행 보존)

기존 `CH-01`~`CH-02` 특성화 테스트가 그대로 통과해야 한다.

```bash
npx vitest run src/pages/dashboard/panels/charts/StatPanel.test.tsx -t "CH-0"
# 기대: 전량 통과
```

### AC-16 — 단위·자릿수 규칙 보존 (U2-8)

기존 테스트(`StatPanel.test.tsx:116` — `'+2MB'`)가 그대로 통과해야 한다.

---

## C. 구간 통계 (U3)

### AC-17 — 미설정이 기본이며 아무것도 그리지 않는다

```gherkin
Given window_stats 가 미지정일 때
When stat 패널을 렌더하면
Then stat-window-stats 요소가 존재하지 않는다
```

vitest: `StatPanel.test.tsx` — "window_stats 미지정이면 구간 통계 줄이 없다"

### AC-18 — 켠 항목만 고정 순서로 그린다 (U3-3)

```gherkin
Given window_stats: { min: true, avg: true } 가 주어졌을 때 (max 는 꺼짐)
When readWindowStats 를 호출하면
Then ['avg', 'min'] 을 반환한다 — 설정에 적은 순서가 아니라 고정 순서다
```

vitest: `statDisplayOptions.test.ts` — "readWindowStats: 순서는 avg → max → min 고정"

### AC-19 — 값이 F1 기대치와 일치한다

```gherkin
Given F1 채널 데이터와 window_stats: { avg: true, max: true, min: true } 가 주어졌을 때
When stat 패널을 렌더하면
Then stat-window-stats 의 텍스트에 22.6 · 26 · 20 이 이 순서로 포함된다
```

vitest: `StatPanel.test.tsx` — "구간 통계가 평균·최대·최소를 한 줄에 그린다"

### AC-20 — 계산은 reduceSeries 를 재사용한다 (U3-4)

```bash
grep -n "reduceSeries" web/src/pages/dashboard/panels/charts/StatPanel.tsx
# 기대: import 와 호출이 존재 (평균/최대/최소 계산이 StatPanel 안에서 재구현되지 않았다)
```

### AC-21 — 값 없음은 자리를 지키며 — 를 그린다 (U3-6)

```gherkin
Given F2 의 room3(전 null) 시리즈와 window_stats: { avg: true, max: true, min: true } 가 주어졌을 때
When 그 타일을 렌더하면
Then 세 항목이 모두 — 로 그려지고 항목이 생략되지 않는다
```

vitest: `StatSubLines.test.tsx` — "값 없는 항목은 자리를 지키고 — 를 그린다"

### AC-22 — 타일 경로는 축약 라벨을 쓴다 (§4.2)

```gherkin
Given 다중 타일 경로에서 window_stats 가 켜졌을 때
When 타일을 렌더하면
Then 라벨이 축약형이다
And 타일이 1개일 때도 축약형이다 — 개수가 아니라 경로로 정한다
```

vitest: `StatSubLines.test.tsx` — "compact 는 타일 개수가 아니라 경로가 정한다"

### AC-23 — 채널 모드 max_points 안내 (U3-8)

```gherkin
Given 채널 모드 stat 패널의 설정에서 구간 통계를 켰을 때
When 설정 섹션을 검사하면
Then max_points 안내 문구가 표시된다
And 시리즈 소스가 활성인 패널에서는 표시되지 않는다
```

vitest: `ChartPanelSections` 테스트 — "채널 모드에서 구간 통계를 켜면 max_points 안내가 뜬다"

---

## D. 보조 줄 크기 (U4)

### AC-24 — 배율이 두 보조 줄에 함께 적용된다 (U4-2)

```gherkin
Given sub_value_scale: 2 와 변화량·구간 통계가 모두 켜진 상태일 때
When stat 패널을 렌더하면
Then stat-delta 와 stat-window-stats 의 fontSize 가 모두 28px 다 (기본 14px × 2)
```

vitest: `StatSubLines.test.tsx` — "sub_value_scale 은 두 보조 줄에 함께 적용된다"

### AC-25 — 클램프 규칙을 공유한다 (U4-3)

```gherkin
Given sub_value_scale 이 각각 0.1 / 5 / 'x' / undefined 일 때
When readSubValueScale 을 호출하면
Then 각각 0.3 / 3 / 1 / 1 을 반환한다
```

vitest: `statDisplayOptions.test.ts` — "readSubValueScale 은 readValueScale 클램프를 따른다"

### AC-26 — 본값 배율과 독립이다

```gherkin
Given value_scale: 2 이고 sub_value_scale 이 미지정일 때
When 렌더하면
Then 본값은 72px 이고 보조 줄은 14px 다 — 본값 배율이 보조 줄로 새지 않는다
```

vitest: `StatPanel.test.tsx` — "value_scale 은 보조 줄 크기를 바꾸지 않는다"

---

## E. 하위 호환 (§3.2)

### AC-27 — 세 필드가 모두 없는 config 는 현행과 같은 화면을 낸다

기존 특성화 테스트 전량이 신규 config 없이 통과해야 한다.

```bash
npx vitest run src/pages/dashboard/panels/charts/StatPanel.test.tsx src/pages/dashboard/panels/charts/StatPanel.multiOutput.test.tsx src/pages/dashboard/panels/charts/StatPanelStoreSource.test.tsx
# 기대: 전량 통과
```

### AC-28 — 저장된 panelColor 는 계속 그려지고 편집도 된다

```gherkin
Given config.panelColor 가 지정된 stat 패널이 대시보드에 있을 때
When 대시보드를 렌더하면
Then 격자 래퍼에 그 색의 좌/상단 테두리가 그려진다
And 패널 설정에서 그 색을 바꿀 수 있다
```

vitest: `PanelSettingsDialog.statStyle.test.tsx` — AC-02 와 함께 검증

---

## F. 통합 (M7)

### AC-29 — 빌드·테스트·린트

```bash
cd web && npm run build   # 기대: exit 0
cd web && npm test        # 기대: exit 0
cd web && npm run lint    # 기대: exit 0
```

### AC-30 — 신규 파일 커버리지 85% 이상

```bash
cd web && npx vitest run --coverage src/pages/dashboard/panels/charts/statDisplayOptions.test.ts src/pages/dashboard/panels/charts/StatSubLines.test.tsx
# 기대: statDisplayOptions.ts · StatSubLines.tsx 모두 85% 이상
```
