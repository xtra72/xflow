# Sync Report — SPEC-FACILITY-DASHBOARD-001

- **SPEC**: SPEC-FACILITY-DASHBOARD-001 — 지하철 시설물 관리 대시보드 패널 (라인·역사·기기)
- **동기화 일자**: 2026-07-29
- **구현 커밋**: `74ada88`
- **버전**: v0.1.0
- **SPEC lifecycle**: spec-anchored (Tier M, 프론트엔드 전용)
- **depends_on**: SPEC-AIRPURIFIER-001
- **Phase**: /moai sync (문서 동기화 + 3-phase close)

---

## SPEC 상태 전이

| 항목 | 이전 | 이후 |
|------|------|------|
| `status` | `in-progress` | `completed` |
| `version` | `0.1.0` | `0.1.0` (유지) |
| `updated` | `2026-07-29` | `2026-07-29` (유지) |

spec.md / plan.md / acceptance.md **본문은 수정하지 않았다**(ownership 경계: manager-docs는 sync 커밋에서 frontmatter `status`+`updated`만 변경). 구현 노트·분기는 CHANGELOG + 본 리포트에 기록했다.

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-FACILITY-DASHBOARD-001/spec.md` | frontmatter `status: in-progress → completed` (본문 무변경) |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — 지하철 시설물 관리 대시보드 패널" 항목 추가 (3종 패널 + 집계 + 셀렉터 제어 + 클라이언트 사이드 집계 결정 + REQ-04-04 이연 요약) |
| `.moai/project/product.md` | "핵심 기능 › Web Dashboard" 절에 "지하철 시설물 관리 대시보드 패널" capability 항목 추가 |
| `.moai/project/structure.md` | "web/ › 주요 재사용 컴포넌트" 절에 facility 대시보드 신규 파일 블록 추가 (facilityAggregation.ts / useAirpurifierControl.ts / Facility*Panel.tsx / facilityShared.tsx + 와이어링 4지점) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `.moai/project/tech.md` | 중복(redundant). 신규 기술 스택 없음 — React 19 + TypeScript + Zustand + vitest는 이미 tech.md에 문서화되어 있고, 본 SPEC은 프론트엔드 전용 + 신규 외부 의존성 0. |
| `README.md` | 사용자향 최상위 개요 변경 불필요. 대시보드 패널은 웹 UI 내부 기능이며 README의 상위 기능 목록/example 표에 대응 행 없음(scope discipline). |

---

## 구현 요약 (참조)

지하철 역사 시설물 관리 비전의 첫 디바이스(공기청정기)를 대상으로, 운영자가 라인(호선)→역사(station)→기기(device) 계층으로 시설물을 조망·통계·제어하는 3종 대시보드 패널. 신규 17개 + 수정 8개 프론트엔드 파일. **신규 백엔드 코드 0** — SPEC-AIRPURIFIER-001의 exec 표면(`list_devices`/`list_stations`/제어 셀렉터)을 소비.

- **라인 패널**: 라인도(line diagram) + 역사별 상태 요약 + 라인 통계 타일 + 라인 일괄 제어.
- **역사 패널**: 역사 통계 + 기기별 상태 목록 + 역사 일괄 제어.
- **기기 패널**: 단일 기기 상태 + 제어(응답 대기 피드백).
- **순수 집계**(`web/src/lib/facilityAggregation.ts`): 역사→호선 해석, 통계 카운트, 라인도 순서 레이아웃, 미분류 분리.
- **셀렉터 제어 훅**(`web/src/hooks/useAirpurifierControl.ts`): `set_power`/`set_fan_speed`/`set_multiple`을 device_id/station/line 셀렉터로 execAgent(`fanOutResponse` 타입) + `useFacilityRoster`(refresh 폴링).
- **패널 컴포넌트**: `panels/{FacilityLine,FacilityStation,FacilityDevice}Panel.tsx` + 공용 `facilityShared.tsx`(StatTiles/ControlResultView/BulkControl).
- **와이어링 4지점**: `uiStore.ts`(PanelType/기본값), `renderDashboardPanel.tsx`(분기), `AddPanelDialog.tsx`(옵션 + facility 대상 단계), `PanelSettingsDialog.tsx`(FacilitySection). i18n `lib/i18n/{ko,en}.json`(`dashboard.facility.*`).

**주요 구현 파일**: `web/src/lib/facilityAggregation.ts`(+테스트), `web/src/hooks/useAirpurifierControl.ts`(+테스트), `web/src/pages/dashboard/panels/{FacilityLine,FacilityStation,FacilityDevice}Panel.tsx`(각 +테스트), `web/src/pages/dashboard/panels/facilityShared.tsx`, `web/src/pages/dashboard/{AddPanelDialog,PanelSettingsDialog,renderDashboardPanel}.tsx`, `web/src/stores/uiStore.ts`, `web/src/hooks/useStation.ts`, `web/src/lib/i18n/{ko,en}.json`.

---

## 분기 (Divergence Report)

| # | 항목 | SPEC 계획 | 실제 구현 | 판정 |
|---|------|-----------|-----------|------|
| 1 | **클라이언트 사이드 집계** (A-2, REQ-05-01) | 집계 표면 | 백엔드 집계 엔드포인트 없이 airpurifier `list_devices`/`list_stations` exec를 웹에서 클라이언트 집계 | 채택 — 백엔드 집계 엔드포인트는 대규모 fleet 성능을 위해 **OI-1로 이연** |
| 2 | **REQ-04-04 테스트 갭** | PanelSettingsDialog facility 설정 폼 단위 테스트 | FacilitySection **구현 완료**, 단, 비-export 내부 컴포넌트라 프로덕션 변경 없이는 테스트 커버 불가 | **이연** (후속: export + scoped RTL 테스트) — 잔여 갭 |
| 3 | **REQ-05-04(폴링) / REQ-06-04(로딩 스피너)** | 폴링 새로고침 / 로딩 스피너 | 타이머·브라우저 거동 특성 | manual / 단위-테스트 범위 외 분류 |
| 4 | **기기 제어 경로** | (제어 진입점) | 기기 패널이 `deviceService.executeCommand`가 아닌 `useAirpurifierControl`(device_id 셀렉터 execAgent) 사용 | 기능적 동등(둘 다 에이전트 `Process` 도달) — 구현 선택으로 명시 |
| 5 | **런타임 의존** | — | 대시보드는 airpurifier exec 표면(develop 머지 완료)을 소비 → airpurifier 백엔드 실행 필요 | 런타임 의존성 기록 |

---

## 품질 게이트 결과 (run-phase 보고, 관측 증거)

| 항목 | 명령 | 결과 |
|------|------|------|
| 타입 체크 | `tsc` | 0 errors |
| 단위 테스트 | `vitest` (전체 스위트) | 2141개 green |
| 린트 | `eslint` | 클린(0 issues) |
| 백엔드 | (Go 무변경) | 소스/테스트 무변경 — 프론트엔드 전용 SPEC |

- 신규 외부 의존성 0. 영속 스키마(uiStore.ts) 하위호환 유지.

---

## REQ → 커버리지 요약

near-complete. 대부분의 요구사항이 구현·테스트되었으며, 잔여 갭은 REQ-04-04 하나다.

| 구분 | 상태 |
|------|------|
| 3종 패널(라인/역사/기기) 렌더·통계·제어 | 구현 + 테스트 (green) |
| 집계 로직(facilityAggregation) | 구현 + 테스트 (green) |
| 셀렉터 제어 훅(useAirpurifierControl) | 구현 + 테스트 (green) |
| 와이어링 4지점(uiStore/render/AddPanel/PanelSettings) | 구현 + 테스트 (facility 관련 테스트 green) |
| i18n ko/en | 구현 |
| **REQ-04-04** (PanelSettingsDialog facility 설정 폼 단위 테스트) | **구현됨 · 단위 테스트 이연 (잔여 갭)** |
| REQ-05-04 / REQ-06-04 (폴링 / 로딩 스피너) | manual / 단위-테스트 범위 외 |

---

## 비고

- 소스 코드/테스트는 수정하지 않았다(문서 동기화 전용).
- 무관 untracked 파일(`data.json`, `output.json`, `packet.json`, `examples/subway/`, `.moai/config/sections/statusline.yaml`, `.moai/memory/last-session-state.json`, `.claude/settings.local.json.lock`)은 스테이징하지 않았다.
- `auto_push=false`: push 및 PR 생성 없음.
