# Sync Report — SPEC-HEATMAP-PANEL-002 (히트맵 패널 도면 배경 + 드래그 배치 에디터)

- **SPEC**: SPEC-HEATMAP-PANEL-002 — Heatmap Panel, floor-plan 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터
- **Tier**: M / lifecycle_level: spec-first (Level 1)
- **Run 커밋**: `bb32958a` (2026-08-07, develop 직접 — personal-mode, PR 없음)
- **동기화 일자**: 2026-08-07
- **상태 전이**: `in-progress` → `completed` / 버전 `0.1.0` → `1.0.0`
- **선행**: SPEC-HEATMAP-PANEL-001 v1.0.0(MVP, 구현 완료). 002 는 MVP 를 가산 확장(additive only, MVP 필드 의미 불변).

## 1. 동기화한 문서

| 문서 | 변경 요약 |
|------|-----------|
| `CHANGELOG.md` | `## [Unreleased]` 최상단에 002 항목 추가(`### 추가 — 히트맵 패널 도면 배경 + 드래그 앤 드롭 센서 배치 에디터`), 001 MVP 항목 위. 순수 로직/렌더·컴포넌트/확장 4지점/분기/품질/관련 SPEC 구조로 기존 스타일 준수. |
| `.moai/specs/SPEC-HEATMAP-PANEL-002/spec.md` | 프런트매터 `status: completed` + `version: "1.0.0"` 갱신(`updated: 2026-08-07`), HISTORY 1.0.0 완료 행 추가, `## 구현 노트 (Implementation Notes)` 섹션 신설(생성/확장 파일 / IN-1~4 분기 / 품질 / 후속). |
| `.moai/project/structure.md` | web 대시보드 패널 섹션에 `히트맵 패널 도면 배경 + 드래그 배치 에디터 (SPEC-HEATMAP-PANEL-002)` 하위 섹션 추가 — 신규 4개 소스(placement.ts / imageAsset.ts / FloorPlanBackground.tsx / SensorPlacementOverlay.tsx) 목적 + 확장 지점 기술. |
| `.moai/project/product.md` | Web Dashboard `히트맵 패널` 항목에 도면 배경 + 시각적 드래그 앤 드롭 센서 배치 역량을 확장 기술. |
| `.moai/project/tech.md` | Canvas 2D 절(001) 뒤에 `도면 배경 임베딩 + 포인터 드래그 정규화 좌표 배치` 하위 섹션 신설 — data-URL 임베딩(FileReader, 2MB 상한, 방식 (a) CSS opacity 합성) + 포인터 드래그 정규화 좌표 배치 기법. |
| `README.md` | **변경 없음** — README `주요 기능` 목록은 백엔드/데이터 계층 패키지만 열거하며 대시보드 패널을 나열하지 않는다(facility/trigger/facility-schedule/heatmap MVP 패널도 README 미기재). 002 는 대시보드 패널 확장이므로 해당 목록에 자리가 없어 001 선례대로 미변경. |
| `.moai/reports/sync-report-heatmap-panel-002.md` | 본 동기화 리포트 생성. |

## 2. 분기 요약 (Divergence — Level 1)

Level 1 spec-first SPEC — 계획된 파일 전량 생성. 모든 신규 config 필드는 additive only, MVP 필드 의미 불변. spec.md §구현 노트 IN-1~IN-4:

- **IN-1 이미지 저장 = data-URL-in-config**: config JSON 에 data-URL 임베드 + 2MB 크기 상한(경고/차단). 오케스트레이터 확정 1차 결정, 백엔드 asset 업로드 엔드포인트는 후속 SPEC 이연(백엔드 무변경, 불투명 JSON 유지).
- **IN-2 배경 합성 = 방식 (a)**: 별도 배경 DOM 레이어 + CSS `opacity` 합성. `HeatmapCanvas.tsx` 불변 — 후보 (b)(canvas `drawImage` + ImageData opacity) 미채택.
- **IN-3 드래그 테스트 = `fireEvent` + PointerEvent 폴리필**: MouseEvent 기반 PointerEvent 폴리필로 작성. `@testing-library/user-event` 미설치 — 신규 의존성 0.
- **IN-4 편집 모드 토글 = 패널 내부(런타임 비영속)**: 편집 모드 on/off 는 패널 런타임 상태. 설정 다이얼로그는 에디터 옵션(snap/marker_size) + 미배치 힌트만 제공.

### 신규 파일

- 순수 로직(TDD): `placement.ts`(좌표 변환·스냅·clamp, 커버리지 100%), `imageAsset.ts`(data-URL 인코딩 + 2MB 상한 검증).
- 컴포넌트: `FloorPlanBackground.tsx`(도면 배경 레이어), `SensorPlacementOverlay.tsx`(정규화 좌표 마커 오버레이).
- 각 동반 `*.test.tsx`/`*.test.ts`.

### 확장 파일

- `heatmapConfig.ts`(additive 파싱), `HeatmapPanel.tsx`(배경 레이어 + 편집 모드), `renderDashboardPanel.tsx`, `PanelSettingsDialog.tsx`(설정 UI), i18n `lib/i18n/{ko,en}.json`.

### 후속 SPEC (범위 밖, 불변)

- SPEC-HEATMAP-PANEL-003: 등고선(contour lines, marching squares) 오버레이. 002 와 독립이며 그 위에 층으로 쌓임.

## 3. 품질 결과 (as reported by run commit `bb32958a`)

- REQ-01~05 전량 구현.
- 신규 코드 커버리지 **96~100%**(`placement.ts` 100%).
- LSP **0**(tsc `--noEmit` + eslint 클린).
- 회귀 프론트 **826 tests** 통과.
- 신규 npm 의존성 **0**, 백엔드 무변경(data-URL, 불투명 JSON config 유지).

## 4. 잔여 사항

- 문서 동기화만 수행(소스 코드 무변경). 별도 git 커밋 단계는 후속으로 분리.
- 커버리지/LSP/테스트 수치는 run 커밋(`bb32958a`) 보고값 인용 — 본 sync 단계에서 재측정하지 않음.
