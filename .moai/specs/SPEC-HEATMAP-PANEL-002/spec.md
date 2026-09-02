---
id: SPEC-HEATMAP-PANEL-002
version: "1.0.0"
status: completed
created: 2026-08-07
updated: 2026-08-07
author: xtra
priority: P2
lifecycle_level: spec-first
title: "Heatmap Panel — 도면 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터"
phase: plan
module: web/dashboard
tier: M
tags: [dashboard, panel, heatmap, floor-plan, drag-and-drop, sensor-placement, canvas, frontend]
---

# SPEC-HEATMAP-PANEL-002 — 도면 이미지 배경 + 드래그 앤 드롭 센서 배치 에디터

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-07 | 0.1.0 | 최초 작성. MVP(SPEC-HEATMAP-PANEL-001) 위에 floor-plan 이미지 배경 렌더 + 히트맵 합성 불투명도 + 시각적 드래그 앤 드롭 센서 배치 에디터를 추가. MVP 의 `sensor_positions` 정규화 좌표 스키마를 그대로 소비(스키마 신규 필드는 추가만). 이미지 저장 방식은 1차 data-URL-in-config(plan.md 결정 근거). | xtra |
| 2026-08-07 | 1.0.0 | 구현 완료(run 커밋 `bb32958a`, develop 직접 — personal-mode). REQ-01~05 전량 구현: 도면 배경 레이어(별도 DOM + CSS opacity 합성, 방식 (a)) + 드래그 앤 드롭 센서 배치 에디터(정규화 0..1 좌표, place-in/remove) + 설정 UI(이미지 첨부·제거·미리보기 / opacity 슬라이더 / fit select / 에디터 옵션). 신규 파일 `imageAsset.ts`·`FloorPlanBackground.tsx`·`placement.ts`(순수, 100% 커버리지)·`SensorPlacementOverlay.tsx`. 신규 코드 커버리지 96~100%, LSP 0, 회귀 826 tests 통과. 백엔드 무변경·신규 의존성 0. 상세 구현 노트는 §구현 노트 참조. | xtra |

## 개요 (Overview)

SPEC-HEATMAP-PANEL-001(MVP)이 도입한 `heatmap` 패널에 **공간 맥락(spatial context)** 을 부여한다. 사용자가
히트맵 패널에 **도면 이미지(floor-plan image)** 를 첨부하면, 보간된 히트맵 레이어 **뒤(behind)** 에 도면이
렌더되어 온도장이 실제 공간 위에 겹쳐 보인다. 히트맵 레이어는 도면이 가려지지 않도록 **설정 가능한 불투명도
(opacity)** 로 도면 위에 합성된다.

또한 MVP 에서 센서 좌표 `{x, y}` 를 **숫자 입력**으로만 지정하던 방식을 대체·보완하여, 도면 이미지 위에 센서
마커를 겹쳐 올린 **시각적 드래그 앤 드롭 배치 에디터(placement editor)** 를 제공한다. 마커를 끌면 해당 센서의
`sensor_positions[key] = {x, y}` 가 갱신된다. 좌표계는 **정규화(0..1 상대 좌표)** 를 유지하여 패널 리사이즈
후에도 배치가 보존된다(MVP REQ-03 의 리사이즈 대응 원칙을 재사용).

이 SPEC 은 MVP 의 config 스키마(`HeatmapPanelConfig`)와 렌더 파이프라인(`HeatmapPanel` / `HeatmapCanvas`)을
**확장 지점(extension point)** 으로 사용한다. 신규 필드는 **추가만(additive)** 하며 기존 필드 의미는 변경하지
않는다. 이미지 저장은 1차적으로 config JSON 에 **data-URL** 로 임베드한다(백엔드 스키마 무변경). 대용량 이미지의
페이로드 팽창 위험과 백엔드 asset 업로드 엔드포인트 대안은 plan.md 에서 결정 근거로 분석한다.

범위 분할(3개 SPEC):
- SPEC-HEATMAP-PANEL-001 (MVP): 패널 타입 등록 + store 바인딩 + Canvas 2D IDW 보간 렌더 + 상하한 clamp + 색상표.
- **SPEC-HEATMAP-PANEL-002 (본 SPEC)**: floor-plan 이미지 배경 + 히트맵 합성 불투명도 + 드래그 앤 드롭 센서 배치 에디터.
- SPEC-HEATMAP-PANEL-003 (후속): 등고선(contour lines, marching squares) 오버레이.

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite 6/Vitest, Tailwind v4). 백엔드 변경 없음(1차 data-URL).
- 렌더 기술:
  - 배경: 도면 이미지를 히트맵 canvas **아래 레이어**에 배치. 두 가지 후보(plan.md 에서 택1) — (a) 별도
    `<img>`/배경 DOM 요소를 canvas 뒤에 z-index 로 겹치기, (b) 히트맵 canvas 렌더 파이프라인에서
    `ctx.drawImage(floorPlan, ...)` 를 히트맵 픽셀 채움 **이전**에 그린 뒤 히트맵 `ImageData` 를 opacity 로 합성.
  - 히트맵 레이어: MVP `HeatmapCanvas` 재사용. opacity 합성은 canvas globalAlpha 또는 CSS opacity 로 처리.
- 좌표계: 정규화 상대 좌표(0..1). 도면/패널 표시 크기와 무관하게 저장. 화면 픽셀 좌표 ↔ 정규화 좌표 변환은
  컨테이너 실측 크기(ResizeObserver 또는 부모 rect) 기준.
- 데이터 소스(기존, 신규 없음): MVP 와 동일하게 `useStoreChartData`(`selection_mode:'tag'`, REST 폴링). 본 SPEC 은
  store 바인딩을 변경하지 않는다.
- 공간 오버레이 선례(인용): `web/src/pages/dashboard/panels/FacilityLinePanel.tsx` — 컨테이너 위 absolute 배치
  마커/노드 오버레이(역 마커를 노선 위에 배치). 본 에디터의 absolute-positioned 마커 오버레이 구조 참고.
- 이미지 업로드 선례(인용): `web/src/services/api/remoteService.ts` `uploadReleaseAsset`(`POST /remote/releases/{version}/assets`,
  multipart/form-data), `internal/api/handler/release_admin*` — **릴리즈 바이너리 전용**이며 범용 이미지 asset
  엔드포인트가 아님. 즉 multipart 업로드 **패턴은 존재**하나 재사용 가능한 범용 asset 저장소는 **없음**(plan.md 결정 근거).
- 확장 대상(기존 MVP 자산):
  - `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` — `HeatmapPanelConfig` + `parseHeatmapConfig`(추가 필드 파싱).
  - `web/src/pages/dashboard/panels/heatmap/HeatmapPanel.tsx` — 배경 레이어 + 에디터 오버레이 마운트.
  - `web/src/pages/dashboard/panels/heatmap/HeatmapCanvas.tsx` — (b) 방식 선택 시 drawImage + opacity 합성 지점.
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx` — heatmap 설정 섹션에 이미지 첨부/불투명도/에디터 진입 추가.
- i18n: `web/src/lib/i18n/{ko.json,en.json}`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | MVP(SPEC-001)가 선행 완료되어 `HeatmapPanelConfig.sensor_positions: Record<string,{x,y}>`(정규화 0..1)와 `HeatmapPanel`/`HeatmapCanvas` 가 존재한다 | 확정(선행 의존성) | SPEC-001 spec.md §명세 config 스키마 / 컴포넌트 구조 |
| A2 | 패널 config 는 불투명 JSON(`json.RawMessage`)으로 영속되어, data-URL 문자열/opacity/에디터 필드를 추가해도 백엔드 스키마 변경이 불필요하다 | 높음 | SPEC-001 A4, 기존 모든 차트 패널 config 영속 방식 |
| A3 | 범용 이미지 asset 업로드 엔드포인트는 코드베이스에 존재하지 않는다(릴리즈 바이너리 전용 multipart 만 존재) | 높음 | reconnaissance: `remoteService.uploadReleaseAsset` 는 `/remote/releases/{version}/assets` 릴리즈 전용 |
| A4 | 정규화 좌표(0..1)는 패널/도면 리사이즈에 불변이므로 드래그 결과가 리사이즈 후에도 보존된다 | 확정 | SPEC-001 REQ-03 리사이즈 재계산 원칙 + 정규화 좌표 설계 |
| A5 | 도면 이미지 미첨부 시 본 SPEC 기능은 graceful 하게 비활성(배경 없음, 에디터는 빈 배경 위 배치)되어야 한다 | 확정 | 오케스트레이터 확정 엣지 케이스 요구 |
| A6 | 대용량 data-URL 은 대시보드 config 페이로드를 팽창시키므로 크기 상한/경고가 필요하다 | 확정 | 결정 위험(plan.md R1) |
| A7 | 등고선(contour)은 본 SPEC 범위 밖이다(003) | 확정 | 범위 분할 |

## 요구사항 (Requirements — EARS)

EARS 5개 유형(Ubiquitous / Event-Driven / State-Driven / Unwanted / Optional)을 5개 모듈에 모두 표현한다.

### REQ-01 — 도면 이미지 배경 렌더 (Ubiquitous)

시스템은 **항상**, 히트맵 패널 config 에 도면 이미지가 설정되어 있으면 해당 이미지를 보간 히트맵 레이어의
**배경(뒤 레이어)** 으로 렌더해야 한다. 배경 도면은 히트맵 레이어와 **동일한 정규화 좌표 공간(0..1)** 을
공유하여, 센서 마커/온도장이 도면과 정렬되도록 배치되어야 한다. 도면 이미지는 패널 표시 영역에 맞춰
**종횡비를 보존(contain/fit)** 하며 렌더되어야 하고, 히트맵 레이어는 항상 도면 **위(above)** 에 합성되어야 한다.

### REQ-02 — 이미지 첨부·저장 (Event-Driven)

**WHEN** 사용자가 설정에서 도면 이미지 파일을 첨부(선택)하면 **THEN** 시스템은 해당 이미지를 읽어
**data-URL** 로 인코딩하여 `HeatmapPanelConfig.floor_plan.image` 에 저장하고 배경으로 즉시 반영해야 한다.
**WHEN** 사용자가 첨부된 이미지를 제거하면 **THEN** 시스템은 config 에서 이미지 필드를 비우고 배경을
제거해야 한다(다른 히트맵 설정은 보존). **WHEN** config 가 변경되면 **THEN** 시스템은 백엔드 스키마 변경
없이 기존 패널 config 저장 경로(불투명 JSON)로만 영속해야 한다.

### REQ-03 — 드래그 앤 드롭 센서 배치 에디터 (State-Driven)

**IF** 사용자가 배치 편집 모드(placement editor)를 활성화하면 **THEN** 시스템은 도면 배경 위에 각 센서의
현재 정규화 좌표에 대응하는 **드래그 가능한 마커**를 absolute 오버레이로 표시해야 한다. **IF** 사용자가
마커를 드래그하면 **THEN** 시스템은 포인터의 컨테이너 상대 위치를 정규화 좌표(0..1)로 변환하여 해당 센서의
`sensor_positions[key]` 를 실시간 갱신하고 배치를 미리보기해야 한다. **IF** 사용자가 미배치 센서를 배치
목록에서 선택하여 도면 위에 놓으면 **THEN** 시스템은 그 센서에 좌표를 부여해야 하고, **IF** 사용자가 마커를
선택 후 제거하면 **THEN** 해당 센서의 좌표 항목을 삭제해야 한다(센서 자체는 store 바인딩에서 유지).

### REQ-04 — 견고성 / 금지 동작 (Unwanted)

시스템은 다음을 **하지 않아야 한다**:
- 도면 이미지가 미첨부일 때 배경/에디터 렌더 예외를 던지지 **않아야 한다**(배경 없이 빈 좌표 공간 위 배치로 graceful 처리).
- 드래그 결과 정규화 좌표가 [0,1] 범위를 벗어나 저장되지 **않아야 한다**(0..1 로 clamp; 도면 밖 배치 금지).
- 설정된 상한을 초과하는 대용량 이미지를 경고 없이 data-URL 로 무제한 임베드하지 **않아야 한다**(크기 상한 경고/차단).
- 배치 편집 모드에서 store 폴링/기존 히트맵 렌더를 파괴하거나 중단하지 **않아야 한다**(편집은 좌표 오버레이 레이어에 국한).
- MVP 의 `sensor_positions` 좌표 의미(정규화 0..1)나 기존 config 필드 의미를 변경하지 **않아야 한다**(추가만 허용).
- 백엔드 스키마를 변경하거나 신규 엔드포인트를 추가하지 **않아야 한다**(1차 data-URL, 불투명 JSON 유지).

### REQ-05 — 합성 불투명도 + 정규화 좌표 옵션 (Optional)

**Where** 사용자가 히트맵 레이어 불투명도(`heatmap_opacity`, 0..1)를 설정하면 시스템은 도면 위에 히트맵을
합성할 때 해당 불투명도를 적용하여 도면 가시성을 조절해야 한다(미설정 시 합리적 기본값, 예: 0.6). **Where**
사용자가 도면 정렬/맞춤 방식(fit: contain/cover)을 설정하면 시스템은 그에 맞춰 배경을 배치해야 한다. **Where**
사용자가 마커 스냅(예: 그리드 스냅) 또는 표시 크기를 설정하면 시스템은 드래그 시 해당 스냅/크기를 반영해야
한다. 좌표는 항상 **정규화(0..1)** 로 저장되어 리사이즈에 불변이어야 한다.

## 명세 (Specifications)

### config 스키마 확장 (추가 필드 — additive only)

MVP `HeatmapPanelConfig` 에 다음 필드를 **추가**한다(기존 필드/의미 불변):

- `floor_plan?: { image?: string; fit?: 'contain' | 'cover'; natural_width?: number; natural_height?: number }`
  - `image`: 도면 이미지 data-URL(1차 저장 방식). 미설정 시 배경 없음.
  - `fit`: 배경 맞춤(기본 `contain`).
  - `natural_width`/`natural_height`: 로드된 이미지 원본 크기(종횡비 유지용, 선택).
- `heatmap_opacity?: number` — 도면 위 히트맵 합성 불투명도(0..1, 기본 0.6).
- `editor?: { snap?: number; marker_size?: number }` — 배치 에디터 옵션(스냅 격자/마커 크기, 선택). 편집 활성
  여부는 런타임 상태(비영속).
- `sensor_positions: Record<string, { x: number; y: number }>` — **MVP 정의 재사용**(신규 아님). 본 SPEC 은
  이 필드를 드래그로 **쓰기(write)** 한다. 좌표는 정규화 0..1.

### 컴포넌트 구조 (`web/src/pages/dashboard/panels/heatmap/`)

- `HeatmapPanel.tsx`(MVP 확장): 배경 레이어(`FloorPlanBackground`) + 히트맵 canvas + (편집 모드 시) 마커 오버레이
  (`SensorPlacementOverlay`) 를 z-index 스택으로 합성.
- `FloorPlanBackground.tsx`(신규): data-URL 이미지 배경 렌더 + fit(contain/cover) 처리. 또는 `HeatmapCanvas` 의
  `drawImage` 통합(plan.md 에서 택1).
- `SensorPlacementOverlay.tsx`(신규): absolute 마커 오버레이. `FacilityLinePanel` absolute 배치 패턴 참고. 포인터
  드래그 → 컨테이너 rect 기준 정규화 좌표 변환 → `onPositionChange(key, {x,y})`.
- `placement.ts`(신규, **TDD 핵심**): 순수 함수 —
  - `toNormalized(clientX, clientY, rect): {x,y}`(0..1 clamp),
  - `fromNormalized({x,y}, rect): {left, top}`(픽셀 배치),
  - `applySnap({x,y}, snap): {x,y}`(선택 스냅),
  - `clamp01(v)`(범위 방어). DOM 없이 단위 테스트.
- `imageAsset.ts`(신규, TDD): `readImageAsDataUrl(file): Promise<string>`, `assertImageSizeUnderLimit(dataUrl, maxBytes)`
  (대용량 경고/차단). FileReader 는 주입/모킹으로 테스트.
- `heatmapConfig.ts`(MVP 확장): `parseHeatmapConfig` 에 `floor_plan`/`heatmap_opacity`/`editor` 하위호환 파싱(미설정 기본값).

### 이미지 저장 결정 (1차 방식)

- **1차 채택**: data-URL-in-config. 백엔드 무변경, 즉시 구현 가능. 크기 상한(예: 1~2MB) 경고/차단으로 페이로드
  팽창 완화(REQ-04). 근거·대안(백엔드 asset 엔드포인트)은 plan.md §위험/결정 참조.

### 추적성 (Traceability)

- REQ-01 → `FloorPlanBackground.tsx` / `HeatmapPanel.tsx`(레이어 스택), `HeatmapCanvas.tsx`(drawImage 방식 시)
- REQ-02 → `imageAsset.ts:readImageAsDataUrl`, `PanelSettingsDialog.tsx`(이미지 첨부/제거 UI), `heatmapConfig.ts`
- REQ-03 → `SensorPlacementOverlay.tsx`, `placement.ts:toNormalized/fromNormalized`, `HeatmapPanel.tsx`(편집 모드)
- REQ-04 → `placement.ts:clamp01`, `imageAsset.ts:assertImageSizeUnderLimit`, `HeatmapPanel.tsx`(미첨부/편집 격리 분기)
- REQ-05 → `heatmap_opacity` 합성(`HeatmapPanel`/`HeatmapCanvas`), `placement.ts:applySnap`, `FloorPlanBackground`(fit)

## 구현 노트 (Implementation Notes)

run 커밋 `bb32958a`(develop 직접, personal-mode) as-implemented 기록. Level 1(spec-first) SPEC — 계획된 파일 전량 생성. MVP config 필드 의미는 불변(모든 신규 필드는 additive only).

### 생성/확장 파일

- **신규 순수 로직(TDD)**:
  - `web/src/pages/dashboard/panels/heatmap/placement.ts` — `toNormalized`/`fromNormalized`/`clamp01`/`applySnap` 순수 함수. DOM 의존 없음, **커버리지 100%**.
  - `web/src/pages/dashboard/panels/heatmap/imageAsset.ts` — `readImageAsDataUrl`(FileReader data-URL 인코딩) + `assertImageSizeUnderLimit`(2MB 상한 검증). FileReader 는 모킹으로 테스트.
- **신규 컴포넌트**:
  - `web/src/pages/dashboard/panels/heatmap/FloorPlanBackground.tsx` — data-URL 도면 배경 레이어(contain/cover fit). 미첨부 시 graceful(배경 없음).
  - `web/src/pages/dashboard/panels/heatmap/SensorPlacementOverlay.tsx` — 정규화 좌표 absolute 마커 오버레이(`FacilityLinePanel` 배치 패턴 참고). 드래그 배치 + 미배치 배치-인 + 마커 제거.
  - 각 컴포넌트/모듈의 `*.test.tsx`/`*.test.ts` 동반.
- **확장(MVP 자산)**:
  - `heatmapConfig.ts` — `floor_plan`/`heatmap_opacity`/`editor` 하위호환 파싱 추가(additive).
  - `HeatmapPanel.tsx` — 배경→히트맵(opacity)→마커 오버레이 z-스택 + 편집 모드.
  - `renderDashboardPanel.tsx` — 렌더 배선.
  - `PanelSettingsDialog.tsx` — 이미지 첨부/제거/미리보기 + opacity 슬라이더 + fit + 에디터 옵션 UI.
  - i18n `lib/i18n/{ko,en}.json`.

### 분기 / 결정 (Divergence — Level 1)

- **IN-1 이미지 저장 = data-URL-in-config**: 이미지를 config JSON 에 data-URL 로 임베드하고 **2MB 크기 상한**(초과 시 경고/차단)을 둔다. 오케스트레이터 확정 1차 결정으로, 백엔드 asset 업로드 엔드포인트는 후속 SPEC 으로 이연(백엔드 무변경, config 는 불투명 JSON 유지).
- **IN-2 배경 합성 = 방식 (a)**: 별도 배경 DOM 레이어 + CSS `opacity` 합성(spec.md §환경 후보 (a))을 채택. `HeatmapCanvas.tsx` 는 **불변** — 후보 (b)(canvas `drawImage` + `ImageData` opacity 합성)는 채택하지 않음.
- **IN-3 드래그 테스트 = `fireEvent` + PointerEvent 폴리필**: 드래그 테스트는 `fireEvent`(테스트 내 MouseEvent 기반 PointerEvent 폴리필)로 작성. `@testing-library/user-event` **미설치 — 신규 의존성 0**.
- **IN-4 편집 모드 토글 = 패널 내부(런타임 비영속)**: 배치 편집 모드 on/off 는 패널 런타임 상태(비영속)로 패널 **안에** 둔다. 설정 다이얼로그는 에디터 옵션(snap/marker_size)과 미배치 센서 힌트만 제공(모드 토글 자체는 설정에 두지 않음).

### 품질 (run 커밋 `bb32958a` 보고값 인용)

- REQ-01~05 전량 구현.
- 신규 코드 커버리지 **96~100%**(`placement.ts` 100%).
- LSP **0**(tsc `--noEmit` + eslint 클린).
- 회귀 프론트 **826 tests** 통과.
- 백엔드 무변경, 신규 npm 의존성 **0**, config 불투명 JSON 유지.

### 후속 (범위 밖, 불변)

- SPEC-HEATMAP-PANEL-003: 등고선(contour lines, marching squares) 오버레이. 002 와 독립이며 그 위에 층으로 쌓임.
