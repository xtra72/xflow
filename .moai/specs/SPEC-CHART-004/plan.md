# SPEC-CHART-004 구현 계획

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

## 0. 전략 요약

프로젝트 개발 방식은 `hybrid`([`.moai/config/sections/quality.yaml`](../../config/sections/quality.yaml)).

| 대상 | 방식 | 커버리지 목표 |
|------|------|---------------|
| 신규 파일(`statLayout.ts`, `StatDragLayer.tsx`, `StatElementStylePopover.tsx`, `textStyleFields.tsx`) | **TDD** | 85% |
| 기존 파일 수정(`StatPanel` · `StatSubLines` · `PanelSettingsDialog` · `renderDashboardPanel` · `ChartPanelSections`) | **DDD** — 특성화로 현재 동작을 먼저 잠근다 | 85% |

핵심 위험은 셋이다.

1. **편집이 꺼진 상태의 회귀.** stat 패널은 대시보드 어디에나 놓여 있다. `onConfigChange` 가 없을 때(=읽기 전용) 종전과 **픽셀 단위로 같은** 화면이 나와야 한다. M1 특성화가 이것을 잠근다.
2. **크기 축 전환.** `font_size` 가 배율을 이기는 규칙(spec.md §3.2)이 세 자리(본값·변화량·구간통계)에 흩어지면 갈린다. 순수 함수 하나가 소유한다.
3. **`TextStyleFields` 추출(D5).** 공용 컴포넌트를 옮기는 기계적 리팩터다. 옮기기 전후로 기존 테스트가 그대로 통과해야 하며, 통과하지 못하면 그것은 추출이 아니라 변경이다.

각 마일스톤은 독립적으로 `npm run build && npm test` 를 통과해야 한다.

---

## 1. 마일스톤 분해

### M1 — 특성화 테스트 (Priority High, DDD-PRESERVE)

수정 전에 "편집 입구가 없는 상태" 를 잠근다.

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | `onConfigChange` 없이 렌더하면 드래그 레이어·핸들·팝오버가 없다 | `charts/StatPanel.test.tsx` |
| 1.2 | 현재 본값·보조 줄에 오프셋 transform 이 걸려 있지 않다 | 동일 |
| 1.3 | 현재 크기가 `기본 px × 배율` 이다(본값 36·보조 14) | 동일 |
| 1.4 | 타일 경로에도 편집 입구가 없다 | `charts/StatPanel.multiOutput.test.tsx` |

**검증**: `npx vitest run src/pages/dashboard/panels/charts/StatPanel` — 전량 GREEN(현재 동작 서술).

---

### M2 — `TextStyleFields` 추출 (Priority High, DDD-IMPROVE, D5)

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | `TextStyleFields` + `LabeledField` + `inputClass` 를 새 모듈로 이동 | `textStyleFields.tsx`(신규) |
| 2.2 | `ChartPanelSections` 는 새 모듈에서 import 하고 기존 이름으로 재export | `ChartPanelSections.tsx` |
| 2.3 | 크기 칸 상한 40 → 160 (spec.md §5 D1) | `textStyleFields.tsx` |

**설계 메모**: 순수 이동이다. 상한 변경 외에 동작을 바꾸지 않는다. 기존 [`textStyleFieldsLayout.test.tsx`](../../../web/src/pages/dashboard/textStyleFieldsLayout.test.tsx) 가 배치를 이미 잠그고 있으므로, 그 테스트가 그대로 통과하는지가 추출의 성공 판정이다.

상한을 넓히는 것이 다른 소비처(타이틀·축 라벨·범례)에 미치는 영향: 상한은 **입력 가능 범위**일 뿐 기본값이 아니므로 저장된 값은 그대로다. 다만 타이틀에 160px 을 넣을 수 있게 되므로 그 자리에서 의도치 않게 큰 값이 들어갈 여지가 생긴다 — 허용한다(되돌릴 수 있고, 막으면 stat 이 쓸 수 없다).

**검증**: `npx vitest run src/pages/dashboard/textStyleFieldsLayout src/pages/dashboard/ChartPanelSections` + `npx tsc --noEmit`.

---

### M3 — 배치·크기 순수 로직 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `StatElementLayout` 타입 + `StatPanelConfig` 3필드 확장 | `charts/chartChannelTypes.ts` |
| 3.2 | `readStatLayout(config, kind): ResolvedStatLayout` — 오프셋·크기·글자 스타일을 한 번에 해석. 크기 폴백 규칙(spec.md §3.2)의 **유일한** 소유자 | `charts/statLayout.ts`(신규) |
| 3.3 | `writeStatLayout(config, kind, patch)` — 부분 갱신. 전부 비면 키를 지운다 | 동일 |
| 3.4 | `clampFontSize(v)` — 6~160 | 동일 |
| 3.5 | 단위 테스트 — 3요소 × (미지정 / px 지정 / 배율만 / 둘 다 / 범위 밖 / 비수치 / 타일 기본 px) | `charts/statLayout.test.ts`(신규) |

**설계 메모**: `kind` 는 `'value' | 'delta' | 'stats'` 문자열이다. 각 kind 가 자기 기본 px 과 폴백 배율 키를 안다 — 호출부가 `36` 이나 `'value_scale'` 을 들고 다니면 그 상수가 세 곳에 복제된다.

오프셋 클램프는 [`clampPercentOffset`](../../../web/src/pages/dashboard/panels/charts/panelGeometry.ts) 을 재사용한다. 새 상수를 만들지 않는다 — 게이지·파이와 범위가 갈리면 패널 유형을 바꿀 때 "가장자리" 의 뜻이 달라진다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/statLayout.test.ts` + `npx tsc --noEmit`.

---

### M4 — 드래그 레이어 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | `StatDragLayer` — 3요소 × (이동 / 크기) 6종 조작. `GaugeDragLayer` 의 rAF 배칭·ref 안정화 구조를 따른다 | `StatDragLayer.tsx`(신규) |
| 4.2 | 잡은 대상 판정 — `data-stat-drag="value|delta|stats"` 와 `data-stat-resize="..."` 표식으로 갈린다. 그 밖을 잡으면 무시 | 동일 |
| 4.3 | 크기 핸들 렌더 + `aria-label` | 동일 |
| 4.4 | 테스트 — 이동/크기 각 3요소, 클램프, 요소 밖 클릭 무시, rAF 배칭 | `StatDragLayer.test.tsx`(신규) |

**설계 메모**: `GaugeDragLayer` 는 좌표계가 둘(SVG viewBox / 백분율)이라 복잡하다. stat 은 **전부 백분율** 한 가지라 그만큼 단순하다. 크기만 px 이며, 이동량을 그대로 더한다(spec.md §7 OQ1).

포인터 이벤트는 프레임보다 자주 올 수 있으므로 rAF 로 프레임당 한 번만 반영한다 — 게이지와 같은 이유.

**검증**: `npx vitest run src/pages/dashboard/StatDragLayer.test.tsx`.

---

### M5 — 글자 스타일 팝오버 (Priority High, TDD)

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | `StatElementStylePopover` — 요소에 앵커링, `document.body` 포털, 경계 회피(OQ4) | `StatElementStylePopover.tsx`(신규) |
| 5.2 | 본값·구간통계는 `TextStyleFields`(M2 추출본) 그대로 | 동일 |
| 5.3 | 변화량은 색 칸을 방향별 3색으로 교체(D4) | 동일 |
| 5.4 | 바깥 클릭 / `Esc` 로 닫기, 포커스 이동·복귀(§4.4) | 동일 |
| 5.5 | 테스트 — 열기/닫기, 필드 반영, 변화량 3색 분기, 경계 회피, 포커스 | `StatElementStylePopover.test.tsx`(신규) |

**설계 메모**: 팝오버는 `pages/dashboard/` 에 둔다 — `StatDragLayer` 와 같은 층이며, 거기서 `textStyleFields` 를 import 해도 패널 → 설정 방향이 아니다(M2 가 그 방향을 정리한다).

---

### M6 — StatPanel 배선 (Priority High, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | `onConfigChange` · `forceEdit` prop 추가. `usePanelEditMode` 로 게이팅 | `charts/StatPanel.tsx` |
| 6.2 | 단일 값 경로를 `StatDragLayer` 로 감싼다. 타일 경로는 감싸지 않는다(U5-2) | 동일 |
| 6.3 | 본값에 오프셋 transform + `font_size` 적용. 단위 비율 유지(U2-6) | 동일 |
| 6.4 | 보조 줄 2종에 오프셋·크기 적용 — `StatSubLines` 가 `scale` 대신 해석된 px 을 받도록 | `charts/StatSubLines.tsx` |
| 6.5 | 더블클릭 → 팝오버 열기 + `Enter` 키 경로(§4.4) | `charts/StatPanel.tsx` |
| 6.6 | M1 특성화 갱신 — 편집이 켜졌을 때의 서술로 바꾼다. **편집이 꺼진 상태의 단언은 손대지 않는다** | `charts/StatPanel.test.tsx` |

**설계 메모**: 6.4 는 `StatSubLines` 의 props 변경이다. `scale: number` → `fontSize: number` 로 바꾸면 호출부가 해석 책임을 갖는다 — 그것이 맞다(해석은 M3 의 `readStatLayout` 이 소유).

`StatDragLayer` 는 편집이 꺼져 있으면 자식을 그대로 통과시켜야 한다(`enabled=false`). 그래야 M1 의 "편집 꺼짐 = 종전과 동일" 특성화가 계속 통과한다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/StatPanel` — 전량 GREEN.

---

### M7 — 미리보기·대시보드 배선 (Priority Medium, DDD-IMPROVE)

| # | 작업 | 산출물 |
|---|------|--------|
| 7.1 | 설정 미리보기에서 `onConfigChange={patchConfig}` + `forceEdit` 전달(게이지와 같은 형태) | `PanelSettingsDialog.tsx` |
| 7.2 | 대시보드 렌더에서 `onConfigChange={onCfg}` 전달 | `renderDashboardPanel.tsx` |
| 7.3 | 배율 슬라이더 옆 "직접 지정 크기 우선" 안내(U6-2) | `ChartPanelSections.tsx` |
| 7.4 | i18n 키 추가 | `lib/i18n/{ko,en}.json` |
| 7.5 | 테스트 — 미리보기에서 끌면 draft config 가 바뀐다 / 대시보드 편집모드 2겹 게이팅 | 각 테스트 파일 |

---

### M8 — 통합 검증 (Priority High)

| # | 작업 |
|---|------|
| 8.1 | `npm run build` — 타입 오류 0 |
| 8.2 | `npm test` — 전량 GREEN |
| 8.3 | `npm run lint` — 오류 0 |
| 8.4 | 신규 파일 커버리지 85% 이상 |
| 8.5 | acceptance.md AC 전량 대조 |

**주의**: 알려진 플레이크가 있다(`api/service` 자가치유 E2E, `internal/remote` dispatch). 실패 시 stash 한 깨끗한 트리에서 재현을 먼저 확인한다.

---

## 2. 파일별 변경 요약

| 파일 | 마일스톤 | 성격 |
|------|----------|------|
| `textStyleFields.tsx` | M2 | 신규(추출) |
| `charts/statLayout.ts` + `.test.ts` | M3 | 신규 |
| `StatDragLayer.tsx` + `.test.tsx` | M4 | 신규 |
| `StatElementStylePopover.tsx` + `.test.tsx` | M5 | 신규 |
| `charts/chartChannelTypes.ts` | M3 | 타입 확장 |
| `charts/StatPanel.tsx` | M6 | 편집 배선 |
| `charts/StatSubLines.tsx` | M6 | props 변경(scale → fontSize) |
| `charts/StatPanel.test.tsx` · `.multiOutput.test.tsx` | M1 · M6 | 특성화 후 갱신 |
| `ChartPanelSections.tsx` | M2 · M7 | 재export + 안내 |
| `PanelSettingsDialog.tsx` | M7 | 미리보기 배선 |
| `renderDashboardPanel.tsx` | M7 | 대시보드 배선 |
| `lib/i18n/{ko,en}.json` | M7 | 라벨 |

---

## 3. 커밋 단위

마일스톤 1개 = 커밋 1개. M2(추출)는 순수 리팩터라 단독으로 되돌릴 수 있게 앞에 둔다.

**주의**: 작업 트리에 이번 작업과 무관한 기존 변경(테마 토큰 치환 등)이 남아 있다. 커밋 시 이번 SPEC 의 파일만 골라 담고, 같은 파일에 두 작업이 섞인 경우 사용자에게 먼저 알린다.
