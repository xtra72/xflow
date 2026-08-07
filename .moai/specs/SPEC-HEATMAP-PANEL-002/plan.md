# SPEC-HEATMAP-PANEL-002 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-002/spec.md`
개발 모드: hybrid (신규 코드 TDD, MVP 컴포넌트 확장은 DDD 특성 — 행위 보존)

## 선행 의존성 (Prerequisite Dependency)

- **본 SPEC 은 SPEC-HEATMAP-PANEL-001(MVP)에 의존한다.** MVP 가 정의한 `heatmap` 패널 타입, `HeatmapPanelConfig`
  (특히 `sensor_positions` 정규화 좌표 스키마), `HeatmapPanel`/`HeatmapCanvas`/`heatmapConfig.ts` 가 존재해야
  구현을 시작할 수 있다.
- 본 SPEC 은 MVP 의 `sensor_positions`(정규화 0..1) 스키마를 **그대로 소비**하며, 숫자 입력으로 좌표를 지정하던
  방식을 **시각적 드래그 배치로 대체·보완**한다. 신규 config 필드는 **추가만(additive)** 한다.
- SPEC-HEATMAP-PANEL-003(등고선)은 본 SPEC 과 독립적으로 MVP 위에 얹힌다(002/003 상호 비의존).

## 기술 스택 (Tech Stack)

- React 19 (client 컴포넌트, 포인터 이벤트 기반 드래그)
- TypeScript 5.9 (satisfies, 엄격 타입, 정규화 좌표 타입 안전)
- Vite 6 + Vitest, React Testing Library (`@testing-library/user-event` 포인터 시뮬레이션)
- Tailwind v4 (absolute 오버레이/마커/z-index 스택 레이아웃)
- HTML Canvas 2D (`ctx.drawImage`, `globalAlpha` — 배경 합성 방식 선택 시)
- FileReader / data-URL (이미지 인코딩), ResizeObserver (컨테이너 실측 → 좌표 변환)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 용도 |
|------|------|------|
| 히트맵 MVP 진입점 | `web/src/pages/dashboard/panels/heatmap/HeatmapPanel.tsx` | 배경 레이어 + 에디터 오버레이 마운트 지점(확장) |
| 히트맵 canvas | `web/src/pages/dashboard/panels/heatmap/HeatmapCanvas.tsx` | `drawImage` + opacity 합성 지점(배경 (b) 방식) |
| config 파서 | `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` | `parseHeatmapConfig` 에 신규 필드 하위호환 파싱 |
| 공간 오버레이 선례 | `web/src/pages/dashboard/panels/FacilityLinePanel.tsx` | 컨테이너 위 absolute 마커/노드 배치 패턴 참고 |
| 설정 다이얼로그 | `web/src/pages/dashboard/PanelSettingsDialog.tsx` | heatmap 섹션에 이미지 첨부/불투명도/에디터 진입 추가 |
| 이미지 업로드 패턴 | `web/src/services/api/remoteService.ts` (`uploadReleaseAsset`) | multipart 업로드 **패턴** 참고(단, 릴리즈 전용 — 범용 asset 아님) |
| i18n | `web/src/lib/i18n/{ko.json,en.json}` | 첨부/제거/편집 안내 문구 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 도면 배경 렌더 + config 확장

- T1. `heatmapConfig.ts` 확장(신규 필드 TDD): `floor_plan`/`heatmap_opacity`/`editor` 하위호환 파싱 + 기본값
  (opacity=0.6, fit=contain). 기존 `sensor_positions`/MVP 필드 파싱 회귀 없음.
- T2. `imageAsset.ts`(신규, TDD): `readImageAsDataUrl(file)`(FileReader 주입), `assertImageSizeUnderLimit(dataUrl, maxBytes)`.
- T3. `FloorPlanBackground.tsx`(신규): data-URL 배경 + fit(contain/cover). 미첨부 시 null 렌더(graceful).
- T4. `HeatmapPanel.tsx` 확장(DDD, 행위 보존): 배경 레이어 → 히트맵 canvas(opacity 합성) z-index 스택. 기존 렌더 경로 회귀 없음.

### Secondary Goal (2차 목표) — 드래그 앤 드롭 배치 에디터

- T5. `placement.ts`(신규, **TDD 핵심**): `toNormalized`/`fromNormalized`/`clamp01`/`applySnap` 순수 함수. 경계값
  (0/1 clamp), 도면 밖 포인터 clamp, 스냅 반올림 테스트.
- T6. `SensorPlacementOverlay.tsx`(신규): absolute 마커, 포인터 드래그 → `placement.toNormalized` → `onPositionChange`.
  미배치 센서 목록에서 드래그-인, 마커 선택 후 제거. `FacilityLinePanel` absolute 패턴 참고.
- T7. `HeatmapPanel.tsx` 편집 모드 통합: 편집 활성(런타임 상태) 시 오버레이 마운트, store 폴링/히트맵 렌더는 비파괴 유지(REQ-04).

### Final Goal (최종 목표) — 설정 UI + 견고성 + 엣지 케이스

- T8. `PanelSettingsDialog.tsx` heatmap 섹션 확장: 이미지 첨부/미리보기/제거, `heatmap_opacity` 슬라이더, fit 선택,
  에디터 진입 버튼, 미배치 센서 안내. i18n ko/en 키.
- T9. 견고성(REQ-04): 미첨부 graceful, 좌표 0..1 clamp, 대용량 이미지 경고/차단, 편집-렌더 격리, additive-only 검증.
- T10. 품질 게이트: 신규 코드 85% 커버리지, TRUST 5, LSP 0 errors.

## 아키텍처 설계 방향

- 순수 좌표 로직(`placement.ts`)과 이미지 인코딩(`imageAsset.ts`)을 렌더/DOM 에서 분리하여 DOM 없이 단위 테스트.
- 레이어 합성 구조: `배경(도면) → 히트맵 canvas(opacity) → 마커 오버레이(편집 시)` z-index 스택. 세 레이어가 동일
  정규화 좌표 공간을 공유하도록 컨테이너 rect 기준 변환 일원화.
- 정규화 좌표(0..1)를 단일 저장 표현으로 유지 → 패널/도면 리사이즈 불변(A4). 화면 좌표는 표시 시점에만 파생.
- config 신규 필드는 additive-only. MVP 필드/의미 불변 → 후속 003(등고선)과도 충돌 없음.

## 이미지 저장 결정 (Decision — 위험 분석 겸)

| 항목 | 1차 채택: data-URL-in-config | 대안: 백엔드 asset 엔드포인트 |
|------|------------------------------|-------------------------------|
| 백엔드 변경 | 없음(불투명 JSON 에 문자열 추가) | 신규 핸들러/스토리지/라우트 필요 |
| 구현 비용 | 낮음(FileReader → data-URL) | 높음(업로드/조회/삭제/권한/GC) |
| 페이로드 | 대시보드 config 팽창(대용량 시) | config 는 URL 참조만(경량) |
| 재사용성 | 즉시 | 범용 asset 저장소 부재(릴리즈 바이너리 전용 multipart 만 존재) |
| 결론 | **1차 채택**: 크기 상한(1~2MB) 경고/차단으로 팽창 완화 | **후속 이연**: 대형 도면/공유 요구 시 별도 SPEC 로 도입 |

- **판단 근거(reconnaissance)**: 코드베이스에 범용 이미지 asset 업로드 엔드포인트가 **없다**. 존재하는
  `uploadReleaseAsset`(`POST /remote/releases/{version}/assets`)는 릴리즈 바이너리 버전 관리 전용이라 재사용
  불가하며, 히트맵 도면용으로 쓰려면 신규 범용 엔드포인트를 새로 만들어야 한다. 따라서 1차 컷은 백엔드 무변경의
  data-URL 방식을 채택하고, 대용량/공유 요구가 실증되면 백엔드 asset 엔드포인트를 별도 후속 SPEC 으로 분리한다.
- **주의(오케스트레이터 확정)**: 서브에이전트는 사용자에게 질의할 수 없으므로, 위 결정은 권장안 + 대안 명시로
  기록하며 구현 착수 승인(Implementation Kickoff) 시 오케스트레이터가 최종 확인한다.

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | 대용량 도면 data-URL 로 대시보드 config 페이로드 팽창 | 저장/전송 지연, localStorage/DB 한계 | 크기 상한(1~2MB) 경고·차단(`assertImageSizeUnderLimit`); 초과 시 리사이즈/축소 안내; 필요 시 백엔드 asset 엔드포인트 후속 |
| R2 | 드래그 좌표계 오정렬(도면 fit/패딩 vs 히트맵 좌표 공간 불일치) | 마커·온도장 어긋남 | 세 레이어 공통 컨테이너 rect 기준 정규화 변환 일원화; fit=contain 시 letterbox 영역 좌표 보정; 스냅샷/단위 테스트 |
| R3 | 포인터 드래그가 store 폴링/히트맵 렌더와 경합 | 편집 중 깜빡임/렌더 파괴 | 편집 오버레이를 별도 레이어로 격리(REQ-04); 좌표 갱신은 config 미리보기 상태로만, 폴링 비중단 |
| R4 | 정규화 좌표가 리사이즈/도면 교체 시 어긋남 | 배치 손실 | 0..1 저장 불변(A4); 도면 교체는 좌표 유지(이미지만 교체); fromNormalized 를 표시 시점 파생으로 |
| R5 | 좌표 [0,1] 범위 이탈 저장 | 마커 화면 밖/NaN | `clamp01` 강제(REQ-04); 드래그 종료 시 clamp 후 저장 |
| R6 | MVP 필드 의미 변경으로 회귀 | 기존 히트맵 깨짐/003 충돌 | additive-only 원칙 검증; `parseHeatmapConfig` 하위호환 테스트; MVP AC 회귀 없음 확인 |

## 후속 SPEC 연계

- SPEC-HEATMAP-PANEL-003(등고선)은 본 SPEC 과 독립. 등고선은 MVP `idw.ts` 보간 격자를 입력으로 하며, 본 SPEC 의
  배경/에디터 레이어 스택 위에 추가 오버레이 레이어로 얹을 수 있다(레이어 순서: 배경 → 히트맵 → 등고선 → 마커).
