# Sync Report — SPEC-PANEL-SETTINGS-001 M6 (데이터 소스 시리즈 선택 UI 개선)

- **SPEC**: SPEC-PANEL-SETTINGS-001 v0.5.0 (Tier L, `web/dashboard`)
- **마일스톤**: M6 — 데이터 소스 시리즈 선택 UI 개선 (REQ-15~22)
- **상태 전이**: `draft → in-progress` (updated 2026-08-11)
- **성격**: Non-breaking, 프론트엔드 전용, 신규 백엔드 0, 신규 npm 의존성 0, 불투명 config JSON shape 무변경

---

## 1. 요구사항 (REQ-15~22)

M1~M5(3분할 셸·공용 Store 리스트·색상 프리셋·2경계 리사이즈/영속·라이브 미리보기) 위에 데이터 소스 시리즈 선택 UX 를 개선.

| REQ | 내용 |
|-----|------|
| REQ-15 | 통일된 표시(display) 필터 — 같은 컬럼 다중값 OR, 다른 컬럼 AND. 태그는 **태그 키(종류)별** — 같은 태그 키 내 다중값 OR, 서로 다른 태그 키 간 AND (예: `device_id ∈ {A,B}` AND `type ∈ {report}`). |
| REQ-16 | 사용자 표기 "별칭(alias)"→"이름(name)" (데이터 필드 `alias` 유지). |
| REQ-17 | 선택 테이블 컬럼 순서 key · 이름 · metric · tag 재배치. |
| REQ-18 | 시리즈별 세부 정보 = 선택된(체크된) 각 시리즈 행의 **인라인 펼침/접힘** 상세(이름 편집 · 색상 · 선 스타일 · (heatmap)좌표 2열). 미선택 행은 상세 없음, 기본 접힘. |
| REQ-19 | **폐지(void/superseded)** — 펼침 상세의 키·종류·태그 설명 라인이 테이블 컬럼 + alias 컬럼 키 에코와 중복 → 제거. 키는 키 컬럼 + alias 컬럼 mono 에코로만 노출. 재번호 없음. |
| REQ-20 | 별도 `SelectedSeriesList` 그룹 제거 — 단일 편집 지점 = 행 펼침. |
| REQ-21 | heatmap 센서 좌표(x/y)를 세부 정보에 통합(이름 뒤 2열 레이아웃). |
| REQ-22 | 명시적 "동적 바인딩" 토글(표시 필터와 분리). 신규 패널 기본 OFF=명시 선택(keys), ON=poll 시 태그 기준 동적 해석(`selection_mode:'tag'` + `tag_filters`). 하위호환: 기존 `selection_mode:'tag'` 저장 패널은 토글 ON + `tag_filters` 보존으로 로드(동작 불변). |
| 추가 | 필터 팝오버 잘림 수정(viewport-clamped fixed positioning) + 세부 정보 폰트 확대(본문/sm 수준). |

---

## 2. 변경/신규 파일

**신규(files_created)**
- `web/src/pages/dashboard/panels/charts/storeColumnValueFilter.ts` — 통일된 표시 필터 순수 매처(같은 컬럼 OR·컬럼 간 AND·태그 키별 OR/AND). design.md 파일 계획에 이미 포함.
- `web/src/pages/dashboard/panels/charts/storeColumnValueFilter.test.ts` — 매처 단위 테스트.

**확장(files_modified)**
- `web/src/lib/i18n/en.json`, `web/src/lib/i18n/ko.json` — "이름" 표기 · 세부 정보 · 동적 바인딩 토글 라벨(ko/en 대칭).
- `web/src/pages/agents/StoreEntryTable.tsx`, `web/src/pages/agents/storeColumns.tsx` — 컬럼 순서 재배치 + 표시 필터/인라인 세부 정보 배선.
- `web/src/pages/dashboard/ChartPanelSections.tsx`, `web/src/pages/dashboard/PanelSettingsDataSource.tsx` — 행 펼침 세부 정보(이름 편집·색상·선스타일·(heatmap)좌표) 통합, `SelectedSeriesList` 그룹 제거, 동적 바인딩 토글 분리, 팝오버 fixed positioning·폰트 확대.

**테스트 확장**
- `web/src/pages/dashboard/PanelSettingsDataSource.test.tsx`
- `web/src/pages/dashboard/PanelSettingsDialog.heatmap.test.tsx`
- `web/src/pages/dashboard/ChartPanelSections`(관련) / `StoreSourceSection.test.tsx` / `StoreTagMode.test.tsx`

---

## 3. 검증 증거 (verbatim)

```
Test Files  236 passed (236)
Tests  2956 passed (2956)
tsc-exit=0
build-exit=0
```

- 회귀 프론트 전량 통과(실패 0).
- 타입 검사(`tsc -b`) exit 0 — 0 type errors.
- 빌드 exit 0.

---

## 4. Divergence 분석 (계획 대비 실제)

- **비계획 스코프 0** — 구현은 SPEC v0.5.0 요구그룹(REQ-15~22, REQ-19 폐지)을 그대로 추적.
- **신규 파일 계획 부합** — `storeColumnValueFilter.ts` 는 design.md 파일 계획에 이미 포함된 순수 매처이며 신규 추상화 계층이 아니다(단위 테스트 격리 목적).
- **REQ-19 폐지** — SPEC v0.5.0 §HISTORY 에서 명시적으로 void/superseded 처리(재번호 없음). 구현은 펼침 상세에서 키 설명 서브라인을 제거하고 키/alias 에코로만 노출.

---

## 5. 잔여 노트 (Residual)

- **`tag_filters` 데이터 모델 유지** — `tag_filters` 는 `Record<string,string>` 로 유지된다. 다중값-per-태그-키는 **표시 필터 상태 관심사**이며, 동적 바인딩 파생은 기존 **best-effort last-value**(SPEC v0.5.0 §HISTORY 명시). 즉 표시 필터에서 한 태그 키에 여러 값을 OR 로 걸어도, 동적 바인딩 토글 ON 시 `tag_filters` 로의 파생은 키당 마지막 값(best-effort)으로 축약된다.
- **`PANEL_FILTER_COLUMNS` 와 태그 키별 시맨틱의 위치 분리** — `PANEL_FILTER_COLUMNS` 는 그룹 필터 UI 표시를 위해 여전히 `tag` 를 단일 컬럼으로 열거하는 반면, 태그 키(종류)별 OR/AND 시맨틱(REQ-15)은 매처(`storeColumnValueFilter.ts`) 배선에 위치한다. UI 컬럼 목록과 매칭 시맨틱이 서로 다른 층에 있다는 점을 후속 작업 시 유의.
- **SPEC 상태** — 사용자 선택에 따라 `completed` 가 아니라 `in-progress` 로 전이(추가 정제 여지 보존).

---

## 6. Sync 산출물 요약

- `CHANGELOG.md` — `[Unreleased]` 에 M6 항목 추가(Korean, SPEC-PANEL-SETTINGS-001 참조).
- `.moai/specs/SPEC-PANEL-SETTINGS-001/spec.md` — frontmatter `status: draft → in-progress`, `updated: 2026-08-11`(본문 무변경).
- `.moai/specs/SPEC-PANEL-SETTINGS-001/progress.md` — `## §E.2 Run-phase Evidence — M6 마일스톤` 절 추가(기존 §E.N 헤딩 토큰 보존).
- `.moai/reports/sync-report-panel-settings-m6-001.md` — 본 리포트.
