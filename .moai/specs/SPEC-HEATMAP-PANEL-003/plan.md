# SPEC-HEATMAP-PANEL-003 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-003/spec.md`
개발 모드: hybrid (신규 코드 TDD, MVP 컴포넌트 확장은 DDD 특성 — 행위 보존)

## 선행 의존성 (Prerequisite Dependency)

- **본 SPEC 은 SPEC-HEATMAP-PANEL-001(MVP)에 의존한다.** 특히 등고선 입력은 MVP `idw.ts` 의 IDW 보간 격자
  (`interpolateIDW(...) → Float32Array`, gridW×gridH 스칼라장)이다. 이 격자 출력과 value bounds(min/max)가
  존재해야 marching squares 를 시작할 수 있다.
- **재보간 금지(핵심 제약)**: 등고선을 위해 별도 보간을 수행하지 않는다. MVP 가 이미 계산한 동일 격자를 그대로
  marching squares 입력으로 사용하여 히트맵 색과 등치선이 동일 스칼라장을 반영하도록 한다.
- SPEC-HEATMAP-PANEL-002(도면 배경/드래그 배치)와 **독립·비의존**. 002 레이어 스택이 있으면 등고선은 그 위(마커
  아래)에 얹히고, 002 없이 MVP 단독 위에서도 동작한다.

## 기술 스택 (Tech Stack)

- React 19 (client 컴포넌트, `useMemo` 로 격자 대비 등고선 memoize)
- TypeScript 5.9 (satisfies, 엄격 타입, `Segment`/`Level` 타입 안전)
- Vite 6 + Vitest, React Testing Library
- **marching squares 알고리즘**(순수 TS, 신규 도입) — 셀 case 0..15, 모서리 선형 보간, saddle(5/10) 결정적 처리
- 렌더(택1): SVG `<path>` 오버레이(벡터/라벨 용이) 또는 Canvas 2D `ctx.stroke`(단일 canvas)
- IDW 격자(`Float32Array`) — MVP `idw.ts` 출력 재사용(신규 계산 없음)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 용도 |
|------|------|------|
| IDW 보간 격자 | `web/src/pages/dashboard/panels/heatmap/idw.ts` (`interpolateIDW → Float32Array`) | **등고선 입력 격자 재사용(재보간 금지)** |
| 히트맵 MVP 진입점 | `web/src/pages/dashboard/panels/heatmap/HeatmapPanel.tsx` | 등고선 레이어(`ContourLayer`) 마운트 지점 |
| 히트맵 canvas | `web/src/pages/dashboard/panels/heatmap/HeatmapCanvas.tsx` | (b) canvas stroke 방식 선택 시 렌더 지점 |
| config 파서 | `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` | `parseHeatmapConfig` 에 `contour` 하위호환 파싱 |
| 설정 다이얼로그 | `web/src/pages/dashboard/PanelSettingsDialog.tsx` | 등고선 토글/레벨/스타일/라벨 UI |
| 색상 팔레트 | `web/src/pages/dashboard/colorSwatchPalette.tsx` | 선 색 선택 UI |
| i18n | `web/src/lib/i18n/{ko.json,en.json}` | 등고선 설정 라벨/안내 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — marching squares 순수 알고리즘

- T1. `marchingSquares.ts`(신규, **TDD 핵심**):
  - `resolveLevels(bounds, count?, explicit?)` — explicit 우선, 균등 분할, 범위 밖 필터.
  - `computeContours(grid, gridW, gridH, level)` — 셀 case 0..15, 모서리 선형 보간 교차점, saddle(5/10) 결정적 규칙,
    격자 경계 셀 처리.
  - `segmentsToPath(segments)` — SVG path d(또는 stroke 좌표열) 변환.
  - 골든 케이스 테스트: 알려진 소형 격자 → 알려진 세그먼트, saddle 케이스, 빈/범위밖 레벨.
- T2. `heatmapConfig.ts` 확장(TDD): `contour` 필드 하위호환 파싱 + 기본값(enabled=false, count=5, labels=false).

### Secondary Goal (2차 목표) — 등고선 레이어 렌더 + 격자 연동

- T3. `ContourLayer.tsx`(신규): 격자 + `contour` config → `resolveLevels` → 레벨별 `computeContours` → SVG `<path>`
  (또는 canvas stroke) 렌더. `useMemo` 로 격자/레벨/스타일 의존 memoize.
- T4. `HeatmapPanel.tsx` 확장(DDD, 행위 보존): 등고선 활성 시 히트맵 위(마커 아래) 레이어로 `ContourLayer` 마운트.
  MVP 렌더 경로의 격자를 재보간 없이 참조 전달. 토글 off 시 기존 렌더 무영향.
- T5. 리사이즈/갱신 대응(REQ-02): 격자 갱신/설정 변경/패널 리사이즈 시 등고선 재계산·스케일 재렌더.

### Final Goal (최종 목표) — 설정 UI + 성능 + 견고성 + 엣지 케이스

- T6. `PanelSettingsDialog.tsx` 등고선 섹션: enabled 토글, level_count vs explicit levels, 선 스타일(색/두께/dash),
  라벨 토글. i18n ko/en 키.
- T7. 성능(REQ-04): 격자 참조/해시 기준 memoize(격자 불변 시 재계산 생략), 잦은 폴링 debounce.
- T8. 견고성(REQ-04): 재보간 금지 준수, 토글 off/빈 격자 graceful, saddle/경계 결정적, 범위 밖 레벨 무시/안내, additive-only.
- T9. 품질 게이트: 신규 코드 85% 커버리지, TRUST 5, LSP 0 errors.

## 아키텍처 설계 방향

- 순수 알고리즘(`marchingSquares.ts`)과 렌더(`ContourLayer.tsx`)를 분리하여 DOM 없이 골든 케이스 단위 테스트.
- 등고선은 **파생 레이어**: 상태 소스는 MVP 격자 하나. 등고선은 격자의 함수이므로 격자 대비 memoize 로 중복 계산 제거.
- 레이어 순서 규약: (배경/도면 → 히트맵 → 등고선 → 센서 마커). 002 유무와 무관하게 등고선은 히트맵 바로 위.
- config 는 additive-only. 토글 off 시 파이프라인/성능 무영향 → MVP·002 회귀 없음.
- 렌더 방식 택1 기준: 라벨/벡터 확대·선택이 필요하면 SVG `<path>`; 단일 canvas 단순화·성능이 우선이면 canvas stroke.
  1차 권장은 SVG `<path>`(등치값 라벨 배치와 선 스타일링이 용이).

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | marching squares saddle(5/10)·격자 경계 셀 처리 오류 | 끊긴/교차 잘못된 등치선 | 결정적 saddle 규칙(중앙값 기준 분기) 명시; 경계 셀 처리; 골든 케이스 단위 테스트(saddle 포함) |
| R2 | 매 store 폴링마다 등고선 재계산 → 성능 저하 | 렌더 지연/프레임 드랍 | 격자 참조/해시 기준 `useMemo`(격자 불변 시 생략); 폴링 debounce; grid_resolution 저격자 계산 후 스케일(MVP 전략 재사용) |
| R3 | 등고선용 별도 재보간 유혹(다른 알고리즘) | 히트맵 색과 등치선 불일치 | 재보간 **금지** 강제(REQ-04); MVP `idw.ts` 격자만 입력; 동일 격자 참조 코드 리뷰 게이트 |
| R4 | 등치값이 격자 범위(min/max) 밖 | 빈/전체 채움 오해 | `resolveLevels` 에서 범위 밖 레벨 필터/안내(REQ-04) |
| R5 | 리사이즈 시 등치선-히트맵-센서 정렬 어긋남 | 시각적 오정렬 | 격자 정규화 좌표 기준 스케일; 히트맵과 동일 표시 변환 공유(REQ-02) |
| R6 | MVP/002 필드 의미 변경으로 회귀 | 기존 렌더 깨짐 | additive-only(`contour`만 추가); `parseHeatmapConfig` 하위호환 테스트; 토글 off 시 무영향 확인 |

## 후속/연계

- 002(도면 배경/드래그 배치)와 독립. 두 후속 SPEC 을 모두 적용하면 레이어 스택은
  (도면 배경 → 히트맵(opacity 합성) → 등고선 → 센서 마커) 순서로 합성된다.
- 필요 시 등치선 라벨 충돌 회피·라벨 밀도 조절 등 고도화는 별도 후속 SPEC 으로 분리(본 SPEC 은 기본 라벨 배치까지).
