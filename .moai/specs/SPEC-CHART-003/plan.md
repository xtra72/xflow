# SPEC-CHART-003 구현 계획

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

## 0. 전략 요약

프로젝트 개발 방식은 `hybrid`([`.moai/config/sections/quality.yaml`](../../config/sections/quality.yaml))다. 마일스톤마다 규율을 나눈다.

| 대상 | 방식 | 커버리지 목표 |
|------|------|---------------|
| 신규 파일(`statDisplayOptions.ts`, `StatSubLines.tsx`) | **TDD** (RED → GREEN → REFACTOR) | 85% |
| 기존 파일 수정(`StatPanel` · `ChartPanelSections` · `PanelSettingsDialog` · `chartChannelTypes`) | **DDD** (ANALYZE → PRESERVE → IMPROVE) — 특성화 테스트로 현재 동작을 먼저 잠근다 | 85% |

핵심 위험은 둘이다.

1. **레거시 변화량 경로의 회귀.** `StatPanel.test.tsx` 의 `CH-01`~`CH-04` 가 이 동작을 잠그고 있다. 변화량을 설정 축으로 승격하면서 이 테스트들이 계속 통과해야 한다 — 통과하지 못하면 그것은 리팩터링이 아니라 동작 변경이다.
2. **미지정 기본값의 경로 의존성**(spec.md §5 D2). 판정이 패널 본문에 흩어지면 두 경로가 조용히 어긋난다. 순수 함수 하나가 소유하고 단위 테스트가 전수로 검증한다.

각 마일스톤은 독립적으로 `npm run build && npm test` 를 통과해야 한다.

---

## 1. 마일스톤 분해

### M1 — 특성화 테스트 보강 (Priority High, DDD-PRESERVE)

수정 전에 현재 동작을 잠근다. 기존 `CH-01`~`CH-04` 가 잠그지 않는 축을 채운다.

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | 변화량 글자색이 현재 emerald/rose/muted 라는 사실을 잠근다(클래스명 단언) | `charts/StatPanel.test.tsx` |
| 1.2 | 변화량 글자 크기가 `value_scale` 을 따라가지 **않는다**는 현재 동작을 잠근다 | 동일 |
| 1.3 | 다중 타일 경로에 보조 줄이 없다는 현재 동작을 잠근다(기존 `:161` 단언 확장 — 구간 통계 testid 도 부재) | `charts/StatPanel.multiOutput.test.tsx` |
| 1.4 | stat 패널 설정 다이얼로그가 현재 `accent-group-picker` 를 낸다는 사실을 잠근다 | `PanelSettingsDialog` 신규 테스트 파일 |

**검증**: `npx vitest run src/pages/dashboard/panels/charts/StatPanel` — 4건 모두 GREEN(현재 동작을 서술하므로 처음부터 통과한다).

**M1 을 먼저 두는 이유**: 이후 마일스톤이 전부 이 동작들을 건드린다. 잠금이 없으면 "고쳤다" 와 "부쉈다" 를 구분할 수 없다.

---

### M2 — 표시 옵션 순수 로직 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | `StatDeltaConfig` · `StatWindowStatsConfig` 타입 + `StatPanelConfig` 3필드 확장 | `charts/chartChannelTypes.ts` |
| 2.2 | `readDeltaEnabled(config, path): boolean` — 경로 의존 기본값(spec.md §5 D2)의 **유일한** 소유자 | `charts/statDisplayOptions.ts`(신규) |
| 2.3 | `readDeltaColors(config): { up, down, flat }` — 미지정 시 기본색 채움 | 동일 |
| 2.4 | `readWindowStats(config): Array<'avg'\|'max'\|'min'>` — 켜진 항목만, `avg → max → min` 고정 순서 | 동일 |
| 2.5 | `readSubValueScale(config): number` — `readValueScale` 클램프 재사용 | 동일 |
| 2.6 | 단위 테스트 — 각 함수 × (미지정 / 명시 true / 명시 false / 범위 밖 / 비수치 / 경로 2종) | `charts/statDisplayOptions.test.ts`(신규) |

**설계 메모**: `path` 인자는 `'legacy' | 'tile'` 문자열이다. 불리언(`isTile`)으로 두면 호출부에서 뜻이 사라진다.

`readWindowStats` 가 배열을 반환하는 이유는, 렌더 쪽이 순서를 다시 정하지 않게 하기 위함이다 — 순서 규칙(U3-3)이 두 곳에 있으면 갈린다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/statDisplayOptions.test.ts` + `npx tsc --noEmit`.

---

### M3 — 보조 줄 렌더 컴포넌트 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `StatDeltaLine` — 화살표 + 부호 + 값 + 단위. 색은 방향으로, 크기는 `sub_value_scale` 로 | `charts/StatSubLines.tsx`(신규) |
| 3.2 | `StatWindowStatsLine` — 켜진 항목 인라인 한 줄. 값 없음은 `—`. 라벨 축약 여부는 `compact` prop | 동일 |
| 3.3 | 두 컴포넌트의 테스트 — 색/크기/순서/값없음/축약/접근성(방향이 색 없이도 읽히는가) | `charts/StatSubLines.test.tsx`(신규) |

**설계 메모**: 두 컴포넌트는 config 를 받지 않고 **이미 해석된 값**(색 3종·배율·항목 배열)을 받는다. config 해석은 M2 가 소유하고 렌더는 그리기만 한다. 이렇게 나누면 타일 경로가 같은 컴포넌트를 `compact` 만 바꿔 재사용할 수 있다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/StatSubLines.test.tsx`.

---

### M4 — StatPanel 배선 (Priority High, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 레거시 경로: 하드코딩 delta 렌더를 `StatDeltaLine` 으로 교체. 표시 여부는 `readDeltaEnabled(config, 'legacy')` | `charts/StatPanel.tsx` |
| 4.2 | 레거시 경로: `StatWindowStatsLine` 추가. 값은 `reduceSeries(entries, fn)` | 동일 |
| 4.3 | 다중 타일 경로: `reduceAllSeries` 결과에 시리즈별 delta·구간 통계를 덧붙인다(U2-10 — 시리즈별 계산) | 동일 |
| 4.4 | `StatTile` 에 보조 줄 2종 추가. `compact` 로 렌더 | 동일 |
| 4.5 | M1 특성화 테스트 갱신 — 색/크기가 이제 설정을 따른다는 사실로 서술을 바꾼다. `CH-01`~`CH-04` 는 **손대지 않는다** | `charts/StatPanel.test.tsx` |

**설계 메모**: 4.3 에서 파생 계산은 기존 `derived` useMemo 안에 머문다. 새 useMemo 를 추가하면 의존성 배열이 둘로 갈라져 한쪽만 갱신되는 사고가 난다.

`reduceSeries` 를 시리즈당 3회(avg/max/min) 호출하는 것은 각 호출이 단일 패스라 시리즈 수 × 3 패스다. 시리즈 상한이 12개(`DEFAULT_MULTI_OUTPUT_LIMIT`)이므로 최적화하지 않는다 — 하나로 합친 다중 반환 함수를 만들면 `reduceSeries` 의 계약이 둘로 갈린다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/StatPanel` — `CH-01`~`CH-04` 포함 전량 GREEN.

---

### M5 — 스타일 섹션 축소 (Priority Medium, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | `stat` 을 `AccentGroupPicker`/`AccentGroupControls` 미노출 목록으로 옮기고, 대신 패널 색상 한 줄 컨트롤을 렌더 | `PanelSettingsDialog.tsx` |
| 5.2 | 패널 색상 한 줄 컨트롤 — 스와치 목록 + 초기화. `panelColor` 만 쓰고 `accentElements` 는 건드리지 않는다 | 동일 |
| 5.3 | M1 1.4 특성화 테스트 갱신 — 이제 picker 가 없고 색상 스와치가 있다 | `PanelSettingsDialog` 테스트 |
| 5.4 | 다른 패널 타입의 스타일 섹션이 그대로임을 검증하는 회귀 테스트 | 동일 |

**설계 메모**: 색상 팔레트 상수는 [`PanelSettingsDropdown.tsx`](../../../web/src/components/common/PanelSettingsDropdown.tsx) 의 `PANEL_COLORS` 와 같은 목록이어야 한다. 다만 그 파일은 미사용 컴포넌트이므로 import 하지 않고, 상수를 공용 자리로 끌어올리거나 새로 정의한다 — 미사용 컴포넌트에 의존을 만들면 그 파일을 지울 수 없게 된다.

**M5 를 뒤에 두는 이유**: M2~M4 와 파일이 겹치지 않고 되돌리기 쉽다. 앞의 마일스톤이 길어져도 이 조각만 단독 커밋할 수 있다.

---

### M6 — 설정 UI (Priority Medium, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | `StatChartSection` 에 변화량 블록 추가 — 표시 체크박스 + (켜졌을 때만) 색 3종 | `ChartPanelSections.tsx` |
| 6.2 | 구간 통계 블록 추가 — 평균/최대/최소 체크박스 3개 | 동일 |
| 6.3 | 보조 줄 배율 필드 추가 — `ValueScaleField` 와 같은 형태, 키만 `sub_value_scale` | 동일 |
| 6.4 | 채널 모드 + 구간 통계 켜짐 → `max_points` 안내 표시(U3-8) | 동일 |
| 6.5 | i18n 키 추가 | `lib/i18n/{ko,en}.json` |
| 6.6 | 설정 UI 테스트 — 체크박스 조작이 config 에 반영되는가, 꺼졌을 때 색 컨트롤이 없는가, 안내가 채널 모드에서만 뜨는가 | `ChartPanelSections` 테스트 |

**검증**: `npx vitest run src/pages/dashboard` + `npm run build`.

---

### M7 — 통합 검증 (Priority High)

| # | 작업 |
|---|------|
| 7.1 | `npm run build` — 타입 오류 0 |
| 7.2 | `npm test` — 전량 GREEN |
| 7.3 | `npm run lint` — 오류 0 |
| 7.4 | `npx vitest run --coverage src/pages/dashboard/panels/charts` — 신규 파일 85% 이상 |
| 7.5 | acceptance.md 의 AC 전량 대조 |

**주의**: 이 저장소에는 알려진 플레이크가 있다(`api/service` 자가치유 E2E, `internal/remote` dispatch). 실패가 나오면 stash 한 깨끗한 트리에서 먼저 재현을 확인하고 회귀로 단정하지 않는다.

---

## 2. 파일별 변경 요약

| 파일 | 마일스톤 | 성격 |
|------|----------|------|
| `charts/statDisplayOptions.ts` | M2 | 신규 |
| `charts/statDisplayOptions.test.ts` | M2 | 신규 |
| `charts/StatSubLines.tsx` | M3 | 신규 |
| `charts/StatSubLines.test.tsx` | M3 | 신규 |
| `charts/chartChannelTypes.ts` | M2 | 타입 3필드 추가 |
| `charts/StatPanel.tsx` | M4 | 보조 줄 배선 |
| `charts/StatPanel.test.tsx` | M1 · M4 | 특성화 보강 후 갱신 |
| `charts/StatPanel.multiOutput.test.tsx` | M1 · M4 | 동일 |
| `PanelSettingsDialog.tsx` | M5 | 스타일 섹션 분기 |
| `ChartPanelSections.tsx` | M6 | 설정 UI |
| `lib/i18n/{ko,en}.json` | M6 | 라벨 |

---

## 3. 커밋 단위

마일스톤 1개 = 커밋 1개. M4 와 M5 는 파일이 겹치지 않으므로 순서를 바꿔도 되지만, M5 를 단독으로 되돌릴 수 있게 하려면 뒤에 두는 편이 낫다.
