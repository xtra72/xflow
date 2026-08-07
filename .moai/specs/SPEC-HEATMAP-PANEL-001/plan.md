# SPEC-HEATMAP-PANEL-001 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-001/spec.md`
개발 모드: hybrid (신규 코드 TDD, 기존 등록지점 수정은 DDD 특성 — 행위 보존)

## 기술 스택 (Tech Stack)

- React 19 (Server/Client 컴포넌트 규칙, 패널은 client 컴포넌트)
- TypeScript 5.9 (satisfies, 엄격 타입)
- Vite 6 + Vitest (단위 테스트), React Testing Library
- Tailwind v4 (레이아웃/빈 상태 스타일)
- HTML Canvas 2D API (`CanvasRenderingContext2D`, `ImageData`) — **코드베이스 최초 실 canvas 도입**
- IDW(Inverse Distance Weighting) 보간 알고리즘 (순수 TS, 백엔드 무관)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 용도 |
|------|------|------|
| store 폴링 훅 | `web/src/pages/dashboard/panels/charts/useStoreChartData.ts` | interval 폴링, AbortController, tag 재해석. `QueryMatrixFn`/`ResolveKeysFn` 주입으로 테스트 용이 |
| store config 타입 | `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts` | `StoreSourceConfig`(L97), `StoreSeriesRef`(L55), `ChartPanelConfigBase`(L152), `pickSeriesColor` |
| store REST 클라이언트 | `web/src/services/api/store.ts` | `storeSeriesDataSource`, `queryStoreMatrix`(L589), `storeChartValue`(L331) |
| 색상 팔레트 | `web/src/pages/dashboard/colorSwatchPalette.tsx` | `COLOR_PALETTE`, `ColorSwatchButton` — 색상표 편집 UI |
| 색상/threshold 패턴 | `chartChannelTypes.ts` | `SERIES_PALETTE`/threshold-color 패턴 → color_table 기본값 |
| 공간 DOM 선례 | `web/src/pages/dashboard/panels/FacilityLinePanel.tsx` | absolute 오버레이 좌표 배치 참고 |
| 색상 셀 그리드 선례 | `web/src/pages/dashboard/panels/modbus/RegisterMapGrid.tsx` | 색상 셀 렌더 참고 |
| 미러링 대상 패널 | `web/src/pages/dashboard/panels/charts/LineChartPanel.tsx` | isStore 분기(L338-361) — store 바인딩 구조 복제 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 패널 등록 + store 바인딩 골격

- T1. `HeatmapPanelConfig` 타입 + `parseHeatmapConfig` (신규, TDD): `heatmap/heatmapConfig.ts`. 하위호환 기본값(좌표 없음/bounds 없음/color_table 없음/power=2).
- T2. 패널 등록 4지점 수정(DDD, 행위 보존):
  - `uiStore.ts`: `PanelType` 유니온에 `'heatmap'`, `panelDefaultSize` case(`{w:5,h:4,minW:3,minH:3}`), `createDefaultPanel` case(기본 config).
  - `renderDashboardPanel.tsx`: `case 'heatmap'` → `<HeatmapPanel .../>`.
  - `AddPanelDialog.tsx`: chart 카테고리에 heatmap 옵션(아이콘/labelKey/descriptionKey) + i18n ko/en 키.
  - `PanelSettingsDialog.tsx`: heatmap CollapsibleSection 마운트.
- T3. `HeatmapPanel.tsx` 골격: `useStoreChartData(store_source, isStore)` 로 시리즈 획득, `LineChartPanel` isStore 분기 미러링. 이 단계에서는 canvas 대신 자리표시.

### Secondary Goal (2차 목표) — IDW 보간 + Canvas 렌더

- T4. `idw.ts` 순수 함수(신규, **TDD 핵심**):
  - `interpolateIDW(points: {x,y,value}[], gridW, gridH, power): Float32Array` — 0-거리 예외 처리, 빈 입력 처리.
  - `mapValueToColor(value, bounds:{min,max}, colorTable): [r,g,b,a]` — clamp + 정규화 + 색상표 보간.
- T5. `HeatmapCanvas.tsx`: `ImageData` 픽셀 채움, grid_resolution → 표시 크기 스케일, ResizeObserver 리사이즈 대응.
- T6. 센서값+좌표 결합 로직: 시리즈 표시 이름(alias/key) → `sensor_positions` 조회, 좌표 미지정 시리즈 필터.

### Final Goal (최종 목표) — 설정 UI + 견고성 + 엣지 케이스

- T7. `HeatmapPanelSettingsSection`: store tag 필터 편집(기존 `StoreSourceSection` 재사용), per-sensor 숫자 x/y 입력, value_bounds min/max, color_table 편집(`colorSwatchPalette` 재사용), IDW power/resolution.
- T8. 견고성(REQ-04): 센서 0개/좌표 미지정/폴링 실패 graceful 상태 + i18n 안내 문구.
- T9. 품질 게이트: 신규 코드 85% 커버리지, TRUST 5, LSP 0 errors.

## 아키텍처 설계 방향

- 순수 로직(`idw.ts`)과 렌더(`HeatmapCanvas.tsx`)를 분리하여 보간 알고리즘을 DOM 없이 단위 테스트한다.
- store 바인딩은 신규 훅을 만들지 않고 `useStoreChartData` 를 그대로 사용(위험 최소화, 폴링/abort/tag 재해석 검증됨).
- config 는 불투명 JSON 이므로 백엔드 무변경. 좌표/색상표는 프론트 전용 필드로 추가.
- 색상표는 threshold-color 정지점 배열로 표현하여 기존 색상 UI 자산을 재사용.

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | Canvas 가 코드베이스 net-new — 렌더/리사이즈/DPR(devicePixelRatio) 처리 미숙 | 흐릿함/오정렬 | grid_resolution 저해상 격자를 표시 크기로 업스케일; DPR 스케일 처리; `HeatmapCanvas` 격리 + 스냅샷 테스트 |
| R2 | 센서 다수 × 고해상 격자에서 IDW O(pixels × sensors) 성능 저하 | 렌더 지연 | grid_resolution 상한 + 저격자 계산 후 업스케일; power/센서수에 따른 조기 종료; 필요 시 후속 최적화(Web Worker)는 002+ 로 이연 |
| R3 | store 시리즈 → 좌표 매핑 시 센서에 config 좌표가 없음 | 보간 누락/오해 | 좌표 미지정 시리즈는 입력에서 제외 + 설정 UI 에서 미배치 센서 목록 노출; 빈 상태 graceful(REQ-04) |
| R4 | tag 필터가 온도 외 센서를 포함 | 잘못된 온도장 | 설정에서 `type:'temperature'` 등 태그 AND 필터 안내; aggregation `last` 로 최신값만 사용 |
| R5 | 색상표/bounds 미설정 시 시각적 무의미 | UX 저하 | 자동 min/max(센서값 범위) + 기본 gradient 폴백(REQ-05) |

## 후속 SPEC 로드맵 (후속 SPEC 로드맵)

- **SPEC-HEATMAP-PANEL-002**: floor-plan 이미지 배경 표시 + 드래그 앤 드롭 센서 배치 에디터(수동 숫자 입력을 시각적 배치로 대체/보완). 본 MVP 의 `sensor_positions` 스키마를 그대로 소비.
- **SPEC-HEATMAP-PANEL-003**: 등고선(contour lines, marching squares) 오버레이 — MVP 의 보간 온도장(Float 격자)을 입력으로 등치선 렌더.

두 후속 SPEC 은 본 SPEC 이 정의한 `HeatmapPanelConfig`(특히 `sensor_positions`)와 `idw.ts` 보간 격자
출력을 확장 지점으로 사용하며, 본 SPEC 에서는 파일을 생성하지 않는다(로드맵 기록만).
