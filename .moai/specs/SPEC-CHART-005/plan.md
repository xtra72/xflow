# SPEC-CHART-005 구현 계획

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

## 0. 전략 요약

프로젝트 개발 방식은 `hybrid`. 마일스톤마다 규율을 나눈다.

| 대상 | 방식 | 커버리지 목표 |
|------|------|---------------|
| 공용화(모듈 이동) | **DDD** — 기존 테스트가 그대로 통과하는 것이 성공 판정 | 유지 |
| 신규(바 범례·바 드래그 레이어 배선) | **TDD** | 85% |
| 기존 수정(파이·미리보기) | **DDD** — 특성화로 현재 동작을 먼저 잠근다 | 85% |

핵심 위험은 둘이다.

1. **공용화가 통계를 깨뜨리는 것.** 이동은 동작 변경이 아니어야 한다. M1 은 순수 이동이며, 통계 테스트 전량(드래그 레이어 27 · StatPanel 60 · statAlign 14 · statSelection 11)이 **손대지 않은 채** 통과해야 한다.
2. **바 범례가 저장된 대시보드의 외형을 바꾸는 것.** 기본이 꺼짐이어야 하고, 특성화가 그것을 잠근다.

각 마일스톤은 독립적으로 `npm run build && npm test` 를 통과해야 한다.

---

## 1. 마일스톤 분해

### M1 — 특성화 (Priority High, DDD-PRESERVE)

수정 전에 세 패널의 현재 상태를 잠근다.

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | 바 차트에 범례·편집 입구가 없다는 현재 상태 | `charts/BarChartPanel.test.tsx` |
| 1.2 | 파이에 그리드·정렬 툴바·선택 구분이 없다는 현재 상태 | `charts/PieChartPanel.test.tsx` |
| 1.3 | 파이의 기존 드래그(범례·파이)는 그대로 동작한다 | 동일 |

**검증**: `npx vitest run src/pages/dashboard/panels/charts/BarChartPanel src/pages/dashboard/panels/charts/PieChartPanel` — 전량 GREEN(현재 동작 서술).

---

### M2 — 편집 장치 공용화 (Priority High, DDD-IMPROVE)

순수 이동이다. 상수·계산·마크업을 바꾸지 않는다.

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | `statAlign.ts` → `panelEditAlign.ts`. `StatElementKind` 를 `<K extends string>` 제네릭으로 | `charts/panelEditAlign.ts` |
| 2.2 | `statSelection.ts` → `panelEditSelection.ts`. 같은 제네릭화 | `charts/panelEditSelection.ts` |
| 2.3 | `StatEditGrid` → `PanelEditGrid` | `PanelEditGrid.tsx` |
| 2.4 | `StatAlignToolbar` → `PanelAlignToolbar` | `PanelAlignToolbar.tsx` |
| 2.5 | `StatDragLayer` → `PanelDragLayer`. 고정 3대상 → `targets: Record<K, DragTarget>` | `PanelDragLayer.tsx` |
| 2.6 | 통계 쪽 import 경로 갱신. `statLayout.ts` 는 통계에 남는다 | `StatPanel.tsx` · `StatSubLines.tsx` 외 |

**설계 메모**: 2.5 가 가장 큰 조각이다. 지금은 `value`/`delta`/`stats` 세 prop 을 받고 `stateRef.current[kind]` 로 찾는다. 맵으로 바꾸면 그 색인이 그대로 동작하고, 대상 수에 따라 갈리는 코드가 없다.

`data-stat-drag` / `data-stat-resize` 표식도 `data-panel-drag` / `data-panel-resize` 로 바꾼다 — 세 패널이 같은 레이어를 쓰므로 표식도 공용이어야 한다.

**검증**: 통계 테스트 전량이 **단언을 고치지 않고** 통과해야 한다(표식 이름만 갱신). 통과하지 못하면 이동이 아니라 변경이다.

---

### M3 — 바 차트 범례 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | 바 config 에 범례 키 추가(파이와 같은 이름) | `charts/chartChannelTypes.ts` |
| 3.2 | `BarChartPanel` 이 `PieLegend` 로 범례를 그린다. 기본은 꺼짐 | `charts/BarChartPanel.tsx` |
| 3.3 | 설정 섹션에 범례 블록 추가 — 파이의 것을 재사용 | `ChartPanelSections.tsx` |
| 3.4 | 테스트 — 기본 꺼짐 / 켜면 항목이 나온다 / 자리·글자 설정이 먹는다 | `charts/BarChartPanel.test.tsx` |

**설계 메모**: `PieLegend` 는 `items: {name, value, percent, color}` 를 받는다. 바의 `chartData` 는 `{label, value, fill}` 이므로 그 자리에서 변환한다. `percent` 는 합계 대비 비중으로 계산한다 — 바에서는 뜻이 옅지만, 비중 표시를 끄면(`showPercentage=false`) 쓰이지 않는다.

---

### M4 — 바 차트 배치 편집 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | `onConfigChange` · `forceEdit` prop + `usePanelEditMode` | `charts/BarChartPanel.tsx` |
| 4.2 | `PanelDragLayer` 로 플롯·범례를 감싼다. 플롯 키는 라인과 공유 | 동일 |
| 4.3 | 그리드·중심 표식·정렬 툴바·선택 배선 | 동일 |
| 4.4 | `renderDashboardPanel` 과 설정 미리보기에 배선 | `renderDashboardPanel.tsx` · `PanelSettingsDialog.tsx` |
| 4.5 | 테스트 — 편집 게이팅 3종 / 선택 / 무리 이동 / 그리드 / 정렬 | `charts/BarChartPanel.test.tsx` |

---

### M5 — 파이 차트 편집 표면 보강 (Priority Medium, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | `PieDragLayer` 를 `PanelDragLayer` 로 갈아탄다(범례·파이 2대상) | `charts/PieChartPanel.tsx` |
| 5.2 | 그리드·중심 표식·정렬 툴바·선택 배선 | 동일 |
| 5.3 | 크기 핸들 — 파이는 `pie_size`(%), 범례는 글자 크기(px) | 동일 |
| 5.4 | M1 특성화 갱신 | `charts/PieChartPanel.test.tsx` |

**설계 메모**: `PieDragLayer` 는 갈아탄 뒤 남는 소비처가 없으면 지운다. 남겨 두면 두 레이어가 공존해 다음 사람이 어느 쪽을 고쳐야 할지 모른다.

---

### M6 — 미리보기·라이브 키 (Priority Medium)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | 바·파이 미리보기에 `onConfigChange` + `forceEdit` | `PanelSettingsDialog.tsx` |
| 6.2 | 신설 키를 `PREVIEW_LIVE_KEYS` 에 추가 | `previewLiveKeys.ts` |
| 6.3 | i18n 키 추가 | `lib/i18n/{ko,en}.json` |
| 6.4 | 미리보기 편집 회귀 테스트 확장 | `PanelSettingsDialog.previewEdit.test.tsx` |

**주의**: 6.2 를 잊는 것이 이 저장소에서 네 번 반복된 실수다(`previewLiveKeys.ts` 머리말). 새 드래그 대상을 만들면 반드시 함께 넣는다.

---

### M7 — 통합 검증 (Priority High)

| # | 작업 |
|---|------|
| 7.1 | `npm run build` — 타입 오류 0 |
| 7.2 | `npm test` — 전량 GREEN |
| 7.3 | `npm run lint` — 오류 0 |
| 7.4 | 신규·이동 파일 커버리지 85% 이상(`vite.config.ts` include 목록 갱신 필요) |
| 7.5 | acceptance.md AC 전량 대조 |

---

## 2. 파일별 변경 요약

| 파일 | 마일스톤 | 성격 |
|------|----------|------|
| `charts/panelEditAlign.ts` | M2 | 이동(+제네릭) |
| `charts/panelEditSelection.ts` | M2 | 이동(+제네릭) |
| `PanelEditGrid.tsx` · `PanelAlignToolbar.tsx` | M2 | 이동 |
| `PanelDragLayer.tsx` | M2 | 이동 + 대상 맵화 |
| `charts/StatPanel.tsx` · `StatSubLines.tsx` | M2 | import 경로 |
| `charts/BarChartPanel.tsx` | M3 · M4 | 범례 + 편집 |
| `charts/PieChartPanel.tsx` | M5 | 편집 표면 보강 |
| `charts/chartChannelTypes.ts` | M3 | 바 범례 키 |
| `ChartPanelSections.tsx` | M3 | 바 범례 설정 |
| `renderDashboardPanel.tsx` · `PanelSettingsDialog.tsx` | M4 · M6 | 배선 |
| `previewLiveKeys.ts` | M6 | 라이브 키 |
| `vite.config.ts` | M7 | 커버리지 include |

---

## 3. 커밋 단위

마일스톤 1개 = 커밋 1개. M2(공용화)는 순수 이동이라 단독으로 되돌릴 수 있게 앞에 둔다.

**주의**: 작업 트리에 이번 작업과 무관한 기존 변경(테마 토큰 치환 등)이 남아 있다. 같은 파일에 두 작업이 섞이면 사용자에게 먼저 알린다.
