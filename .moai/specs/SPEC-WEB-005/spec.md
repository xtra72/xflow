---
id: SPEC-WEB-005
version: 0.7.0
status: in_progress
created: 2026-04-23
updated: 2026-05-05
author: xtra
priority: high
---

# SPEC-WEB-005: TSDB/Store 에이전트 시리즈 탐색 및 데이터 뷰어

## HISTORY

- **0.7.0** (2026-05-05): SPEC-STORE-003 v0.3.0 BREAKING API 적응 (frontend 단독 진화):
  (1) **타입 진화 (BREAKING for frontend)** — `StoreKeysRawResponse.keys: string[]` → `keys: StoreKeyObject[]` (`{key, registration, data_type, metric_type, tags}`). `tags` 별도 맵 필드 제거. `fetchStoreKeys` / `fetchStoreKeysWithTags` / `useStoreKeys` / `useStoreKeysWithTags` 모두 풀 마이그레이션 (어댑터 shim 없음, 백엔드 v0.3.0과 동기). 별도 클라이언트 `services/api/storeService.ts` 는 deprecation 후 제거.
  (2) **Config UI 진화 — registration_type** — `StoreKeysEditor` / `agentSchemas.ts` 의 `allow_dynamic_keys: bool` 토글을 `registration_type: enum` (`manual` | `auto`) 세그먼트 컨트롤로 교체. 라벨/도움말은 "수동 등록" / "자동 등록" 으로 표기.
  (3) **Config UI 진화 — data_type / metric_type** — `StoreKeysEditor` 의 키 행에 `data_type` 셀렉트 컬럼 (6종 enum: `int|float|string|boolean|bytes|json`) 과 `metric_type` 텍스트 입력 컬럼 (정규식 `^[a-zA-Z0-9_-]+$` 클라이언트 검증, 기본값 `"unknown"`) 추가. manual 모드에서 `data_type` 미입력 시 저장 차단 + 인라인 에러.
  (4) **PromoteToStaticDialog 진화** — 동적 → 정적 승격 시 사용자에게 `data_type` (필수) + `metric_type` (선택, 기본 `"unknown"`) 입력 받도록 폼 확장.
  (5) **에러 메시지 매핑** — 백엔드 신규 에러 (`ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`) 와 마이그레이션 에러 (`"config: 'allow_dynamic_keys' is removed..."`) 를 사용자 친화 문구로 매핑하여 토스트/다이얼로그에 표시.
  (6) **신규 메타데이터 표시** — `TsdbDataViewerModal` 시리즈 멀티셀렉트 행 / 저장소 탭 키 리스트에 `data_type` / `metric_type` / `registration` 칩을 보조 메타데이터로 노출 (태그 칩과 시각적으로 구분).
  (7) **신규 필터 UI (선택적, scope-flex)** — `data_type` / `metric_type` / `registration` 필터 칩을 기존 태그 필터와 병행 (Decision Point 1 에서 사용자 승인 시 포함, 기본 제안: 포함).
  (8) **자동/수동 키 시각 구분 (선택적, scope-flex)** — auto 등록된 키와 manual 등록된 키를 시각적으로 구분 (예: 작은 배지) — Decision Point 1 에서 결정.
  배경: SPEC-STORE-003 v0.3.0 (commit `a8d9acf`) 머지 후 frontend 가 `t.includes is not a function` 으로 크래시. v0.7.0 은 운영자에게 노출되지 않은 내부 API 컨트랙트 변경이므로 end-user 마이그레이션 가이드는 불필요하며 개발자 대상 변경만 다룬다. **신규 외부 라이브러리 없음.**
- **0.6.0** (2026-04-28): TSDB 데이터 뷰어 7종 개선 추가:
  (1) 모달 레이아웃 재구성 — 폼 영역 `min-h-0 shrink overflow-y-auto` + 매트릭스 영역 `min-h-[180px]` 보장으로 작은 화면에서 매트릭스/푸터 가림 방지, 시리즈 fieldset 좌(검색+체크박스)/우(태그 필터) 2열, 시간 범위 좌(범위+인터벌)/우(집계) 2열, 절대↔상대 모드 전환 시 인터벌 위치 안정 (`min-h-[7.5rem]` reserve), 선택 시리즈 별도 pill 표시 제거.
  (2) 시간 범위 기본 모드를 절대(absolute)에서 상대(relative)로 변경, 모달 오픈/리오픈 시 항상 'relative' 로 리셋.
  (3) 시리즈 행 우측에 자동 추출 태그 칩 최대 3개 노출 (`series-row-tags-{key}`).
  (4) 태그 필터 라벨 "태그로 필터링" → "필터링", 4자 이내 세그먼트 구분자 입력 박스 추가 (`tag-segment-separator`, 기본 ':'), 구분자 변경 시 기존 태그 필터 자동 초기화.
  (5) `keyTagExtractor` 3개 함수에 `separator: string` 파라미터 추가, 폴백 제거(미매칭 시 빈 객체), 자동 추출 + 정적 태그 병합(정적 우선).
  (6) 결과 뷰 모드 토글 (테이블 / 라인 차트, `tsdb-result-view-table` / `tsdb-result-view-chart`), 신규 `TsdbResultChart.tsx` (recharts LineChart, 컬럼별 8개 cyclic 색상, animation 비활성, connectNulls 미사용).
  (7) 차트 결측값 처리 4모드 (`gap` / `previous` / `value` / `interpolate`, `tsdb-chart-null-mode` + `tsdb-chart-null-fill-value`), 신규 `tsdbChartNullHandling.ts` 모듈.
  (8) viewMode 영속화 — 다시 실행 시 `<SeriesResultMatrix>` remount 되어도 표시 모드 유지를 위해 `viewMode?` + `onViewModeChange?` controlled props 추가, 모달 오픈 시 'table' 로 리셋.
  486/486 tests pass (+18 신규: 15 null handling + 3 view toggle integration + 1 controlled prop). TypeScript strict 통과.
- **0.5.0** (2026-04-27): TSDB 데이터 뷰어 3종 개선 추가:
  (1) 키 세그먼트에서 태그 자동 추출 (InfluxDB 스타일 `measurement,k=v,k=v` + colon/slash segment 위치 기반 `seg0/seg1/seg2`). 정적 태그 우선, 없으면 자동 fallback. TSDB 타입 에이전트도 태그 chip 필터 사용 가능.
  (2) 평균 집계 소수점 자릿수 입력 (0-6, 기본 1). 매트릭스 표시와 CSV 내보내기 모두 toFixed 적용.
  (3) 매트릭스 페이지네이션 (페이지 크기 [10, 25(기본), 50, 100]) 추가, react-window 가상화 제거. 그 외: 데이터 뷰어 multi-select max-h-40 → max-h-[40vh] 확장, 저장소 탭 행별 액션을 마지막 컬럼으로 분리, 전체 초기화 버튼 색상 중립화, property 편집기 8종 readOnly 가시성 일괄 복원, FormField boolean 기본값 표시 (별도 SPEC-STORE-003 항목과 일부 겹침).
- **0.4.0** (2026-04-24): Store 에이전트 UI에 "시리즈" 탭 제거 및 "저장소" 탭 통합, 설정 탭을 운영/데이터 2개 섹션으로 시각 분리(데이터 섹션은 SPEC-STORE-003 정적 키+태그 행 편집기), 저장소 리스트에 태그 컬럼 및 유니크 태그 chip 필터 추가, 데이터 보기 모달의 시리즈 멀티셀렉트 상단에 동일 chip 필터 추가. 백엔드 태그 API(SPEC-STORE-003)와 연동하여 클라이언트측 필터링은 AND 조건. 하위호환: 태그 없는 키/에이전트는 태그 컬럼 숨김 및 필터 미노출. 관련 SPEC: SPEC-STORE-003.
- **0.3.0** (2026-04-24): 시간 범위 절대/상대 모드 토글 UI, 결과 매트릭스 CSV 내보내기, store 서버측 집계 지원, 매트릭스 가상 스크롤(react-window) 추가. 백엔드 `POST /api/v1/store/{agent_name}/query` 에 optional `interval_ms` + `aggregation` 필드 추가(하위호환 유지). 프론트엔드는 서버측 집계 우선, 4xx 에러 시 클라이언트 집계 fallback. `react-window` v2.2.7 의존성 추가, 500행 이상에서 자동 가상화. CSV 내보내기는 zero-dependency Blob 다운로드 방식(UTF-8 BOM 포함, 로컬 ISO-8601 타임존 오프셋 표기). 326 tests pass (+32 신규).
- **0.2.0** (2026-04-23): `useSeriesDataSource` 추상화 추가로 `type === 'tsdb'` 와 `type === 'store'` 에이전트 모두에서 시리즈 탭을 지원. Store 는 서버 측 페이지네이션/집계가 없어 전체 키 로드 → 클라이언트 슬라이스, `time_range` 모드 원본 엔트리 → 클라이언트 측 버킷/집계 전략을 사용한다. 기존 `TsdbSeriesListPanel`/`TsdbDataViewerModal`/`TsdbResultMatrix` 는 `dataSource: SeriesDataSource` prop 을 받도록 리팩터되었고, `SeriesResultMatrix` 명시적 export 가 추가되었다. 백엔드 변경 없음.
- **0.1.0** (2026-04-23): Initial draft — TSDB 에이전트 상세 패널의 시리즈 페이지네이션, 데이터 뷰어 모달, 멀티 시리즈 쿼리 매트릭스 렌더링 요구사항 정의

---

## Overview

### 목적

xflow 대시보드의 에이전트 상세 패널에서 TSDB 타입 에이전트가 보유한 시리즈 키를 탐색하고, 사용자가 선택한 시리즈에 대해 시간 범위/인터벌/집계 함수를 지정하여 히스토리 데이터를 매트릭스 형태로 조회할 수 있는 UI를 제공한다.

> **v0.7.0 진화 노트**: SPEC-STORE-003 v0.3.0 (BREAKING) 머지 후 frontend 의 `StoreKeysRawResponse` 타입 가정이 깨졌다. v0.7.0 은 (a) 응답 객체 배열 적응, (b) `registration_type` / `data_type` / `metric_type` 필드를 Config UI 와 키 리스트에 노출, (c) 신규 백엔드 에러 매핑을 다룬다. 백엔드 변경은 없으며, frontend 단독 진화이다.

### 배경

현재 xflow 백엔드는 `GET /api/v1/tsdb/series`와 `POST /api/v1/tsdb/query` 엔드포인트를 제공하지만, 프론트엔드에서 이를 활용하는 UI가 존재하지 않는다. 운영자가 저장된 시계열 데이터를 검증하거나 대시보드 차트 구성을 계획할 때 REST 클라이언트 도구를 직접 사용해야 하는 불편이 있어, 이를 웹 UI에서 직접 수행할 수 있도록 한다.

### 범위

- **포함**: TSDB 타입 에이전트에 한정된 시리즈 리스트 패널, 데이터 뷰어 모달, 매트릭스 결과 테이블, 페이지네이션, 필수 입력 검증, 에러 처리
- **제외**: 차트 시각화(SPEC-CHART 계열에서 담당), 시리즈 데이터 편집/삭제, 실시간 스트리밍 구독, 쿼리 결과 내보내기(CSV/JSON)

### 가정

- 프론트엔드 입력 시각은 브라우저 로컬 타임존으로 간주하며, 백엔드 전송 전에 UTC epoch milliseconds(int64)로 변환한다 (`project_timestamp_convention.md` 준수).
- 기존 `GET /api/v1/tsdb/series` 응답 포맷(`{Series []string, Count int}`)은 `page`/`size` 파라미터가 없는 경우 유지되며, 하위 호환성이 보장된다.
- TSDB 에이전트는 현재 프로젝트에서 싱글톤으로 배포되나, 미래 확장을 위해 optional `agent_id` 파라미터를 선제 도입한다.

### 의존성

- 백엔드: `internal/api/handler/tsdb.go`, `internal/api/dto/tsdb.go`
- 프론트엔드: `web/src/components/agents/AgentDetailPanel.tsx`, `web/src/api/`, `web/src/store/`
- **v0.7.0 추가**: SPEC-STORE-003 v0.3.0 (백엔드, 머지 완료 — commit `a8d9acf`). 본 SPEC 은 그 BREAKING 컨트랙트의 frontend 적응만 담당한다.

---

## Requirements (EARS)

### M1: Paginated Series List

- **Ubiquitous**: 시스템은 TSDB 타입 에이전트 상세 패널에서 해당 에이전트가 저장한 시리즈 키 목록을 페이지 크기 선택 가능한 페이지네이션과 함께 항상 표시해야 한다.
- **Event-driven**: WHEN 사용자가 페이지 크기 셀렉터 값을 `10/25/50/100` 중 하나로 변경하면, THEN 시스템은 현재 페이지를 1로 리셋하고 `GET /api/v1/tsdb/series?page=1&size={size}`를 호출하여 목록을 갱신해야 한다.
- **Event-driven**: WHEN 사용자가 페이지 네비게이션 컨트롤(이전/다음/특정 페이지)을 클릭하면, THEN 시스템은 해당 페이지의 시리즈 목록을 재조회해야 한다.
- **State-driven**: IF 에이전트 타입이 `tsdb`가 아닌 경우, THEN 시스템은 시리즈 리스트 패널을 렌더링하지 않아야 한다.
- **State-driven**: WHILE 시리즈 목록 조회가 진행 중이면, 시스템은 테이블 영역에 로딩 스켈레톤을 표시해야 한다.
- **Unwanted**: 시스템은 `size` 파라미터를 서버 허용 상한(100) 초과로 전송하지 않아야 한다.

### M2: Data Viewer Popup Control

- **Event-driven**: WHEN 사용자가 시리즈 행의 "데이터 보기" 버튼을 클릭하면, THEN 시스템은 해당 시리즈 키를 기본 선택 상태로 포함한 `TsdbDataViewerModal`을 열어야 한다.
- **Event-driven**: WHEN 사용자가 모달 외부 영역 클릭, Esc 키 입력, 또는 닫기 버튼 클릭을 수행하면, THEN 시스템은 모달을 닫고 폼 상태를 폐기해야 한다.
- **State-driven**: WHILE 쿼리 요청이 진행 중이면, 시스템은 실행 버튼을 비활성화하고 모달 헤더에 진행 인디케이터를 표시해야 한다.
- **Optional**: WHERE 추가 시리즈 선택이 가능할 때, 시스템은 전체 시리즈 목록에서 멀티셀렉트(autocomplete + 체크박스) 입력을 제공한다.

### M3: Multi-Series Query Execution

- **Event-driven**: WHEN 사용자가 실행 버튼을 클릭하면, THEN 시스템은 선택된 시리즈 키 배열, 시작/종료 epoch ms, 인터벌(Go duration string), 집계 함수를 포함한 요청을 `POST /api/v1/tsdb/query`에 전송해야 한다.
- **Ubiquitous**: 시스템은 프론트엔드 입력 시각을 브라우저 로컬로 간주하여 백엔드 전송 전 UTC epoch ms(int64)로 변환하여야 한다.
- **Unwanted**: 시스템은 선택된 시리즈 키가 0개인 경우 쿼리 요청을 전송하지 않아야 한다.
- **Unwanted**: 시스템은 종료 시각이 시작 시각과 같거나 이전인 경우 쿼리 요청을 전송하지 않아야 한다.

### M4: Matrix Result Rendering

- **Ubiquitous**: 시스템은 쿼리 응답을 선택된 시리즈 키를 컬럼으로, 시간 버킷을 행으로 하는 매트릭스 테이블로 항상 렌더링해야 한다.
- **State-driven**: IF 특정 버킷 × 시리즈 조합에 값이 없는 경우, THEN 시스템은 해당 셀에 em-dash(`—`)를 렌더링해야 한다.
- **Event-driven**: WHEN 예상 결과 행 수가 5,000을 초과하면, THEN 시스템은 실행 전 경고 배너를 표시하되 사용자가 실행을 계속할 수 있어야 한다.
- **Ubiquitous**: 시스템은 타임스탬프 컬럼을 브라우저 로컬 형식(`YYYY-MM-DD HH:mm:ss`)으로 렌더링해야 한다.

### M5: Validation & Error Handling

- **Unwanted**: 시스템은 서버 응답이 4xx 또는 5xx인 경우 결과 테이블을 갱신하지 않아야 한다.
- **Event-driven**: WHEN 네트워크 실패 또는 백엔드 에러 응답이 발생하면, THEN 시스템은 모달 내 에러 배너에 서버 메시지(존재 시) 또는 일반 에러 텍스트를 표시하고 폼 입력은 유지해야 한다.
- **State-driven**: WHILE 필수 입력(키 1개 이상, 올바른 시간 범위)이 충족되지 않은 상태에서는 실행 버튼은 비활성 상태여야 한다.

### M6: Result View Mode Toggle (v0.6.0)

- **R-V-001 Ubiquitous**: 시스템은 쿼리 결과 매트릭스 헤더에 "테이블"과 "라인 차트" 두 가지 표시 모드를 토글하는 탭 컨트롤을 항상 제공해야 한다 (testid: `tsdb-result-view-table`, `tsdb-result-view-chart`).
- **R-V-002 Event-driven**: WHEN 사용자가 "라인 차트" 탭을 선택하면, THEN 시스템은 매트릭스 테이블과 페이지네이션 컨트롤을 숨기고 `TsdbResultChart` (recharts `LineChart` 기반) 를 렌더링해야 한다.
- **R-V-003 Ubiquitous**: 시스템은 차트 모드에서 컬럼별로 8개 cyclic 팔레트의 고유 색상을 부여하고, animation 은 비활성화해야 하며, `connectNulls` 옵션은 사용하지 않아야 한다.
- **R-V-007 State-driven**: WHILE 사용자가 "다시 실행" 으로 인해 `<SeriesResultMatrix>` 가 unmount→remount 되더라도, 시스템은 사용자가 직전에 선택한 표시 모드(테이블/차트)를 유지해야 한다 (controlled `viewMode` / `onViewModeChange` props 통한 부모 영속화).
- **State-driven**: 시스템은 모달 오픈 시 `resultViewMode` 를 항상 'table' 로 리셋해야 한다.

### M7: Chart Null Handling (v0.6.0)

- **R-V-004 Ubiquitous**: 시스템은 차트 모드에서 결측값(null) 처리 모드를 4가지 중 하나로 선택할 수 있는 셀렉트를 차트 컨테이너 상단 툴바에 항상 제공해야 한다 (testid: `tsdb-chart-null-mode`). 모드는 `gap` (기본; 라인 끊김), `previous` (forward-fill), `value` (사용자 상수 채움), `interpolate` (양쪽 알려진 값 사이 선형 보간) 이다.
- **R-V-005 Event-driven**: WHEN 사용자가 null 처리 모드를 `value` 로 선택하면, THEN 시스템은 number 입력 박스를 노출하여 채움값 상수를 입력받아야 한다 (testid: `tsdb-chart-null-fill-value`).
- **R-V-006 Ubiquitous**: 시스템은 `interpolate` 모드에서 양쪽 알려진 값이 모두 존재하는 구간에 대해서만 선형 보간을 수행하고, 한쪽만 존재하는 구간(예: 시리즈 끝)은 forward-fill 로 처리하며, leading null 은 그대로 유지해야 한다.
- **State-driven**: WHILE null 처리 모드가 `value` 가 아니면, 시스템은 채움값 입력 박스를 표시하지 않아야 한다.

### M8: Layout Stability & Tag Discovery UX (v0.6.0)

- **R-V-008 Ubiquitous**: 시스템은 모달 폼 영역(`min-h-0 shrink overflow-y-auto`)과 매트릭스 영역(`min-h-[180px]`)을 사전 reserve 하여, 작은 화면에서도 폼이 매트릭스나 푸터를 가리지 않아야 한다.
- **R-V-009 Ubiquitous**: 시스템은 시리즈 선택 fieldset 을 좌(검색+체크박스 리스트) / 우(태그 필터) 2열 레이아웃으로, 시간 범위 영역을 모드 탭 아래 좌(범위+인터벌) / 우(집계 함수) 2열 레이아웃으로 렌더링해야 한다.
- **R-V-010 State-driven**: WHILE 사용자가 절대(absolute) 모드와 상대(relative) 모드를 전환하면, 시스템은 범위 컨트롤 영역의 높이(`min-h-[7.5rem]`)를 사전 reserve 하여 인터벌 컨트롤의 위치가 흔들리지 않도록 해야 한다.
- **R-V-011 Ubiquitous**: 시스템은 시리즈 리스트의 각 행 우측에 해당 키에서 자동 추출된 태그 칩을 최대 3개까지 표시해야 한다 (testid: `series-row-tags-{key}`).
- **Unwanted**: 시스템은 멀티셀렉트 영역 위에 별도의 "선택된 시리즈" pill 영역을 렌더링하지 않아야 한다 (선택 상태는 체크 표시로만 표현).

### M9: Time Range Default Mode (v0.6.0)

- **R-V-012 Ubiquitous**: 시스템은 시간 범위 모드의 기본값을 상대(relative)로 설정해야 한다.
- **State-driven**: WHILE 모달이 새로 열리거나 재오픈되면, 시스템은 시간 범위 모드를 항상 'relative' 로 리셋해야 한다.

### M10: Tag Filter Separator Configuration (v0.6.0)

- **R-V-013 Ubiquitous**: 시스템은 `TagFilterChips` 헤더 라벨을 "필터링" 으로 렌더링하고, 4자 이내의 세그먼트 구분자(separator) 입력 박스를 함께 노출해야 한다 (testid: `tag-segment-separator`).
- **State-driven**: WHILE 모달이 오픈되면, 시스템은 separator 값을 ':' 로 리셋해야 한다.
- **Event-driven**: WHEN 사용자가 separator 값을 변경하면, THEN 시스템은 기존 태그 필터 선택을 자동으로 초기화(`tagFilter.clearAll()`)해야 한다.
- **Ubiquitous**: 시스템은 자동 추출된 태그와 정적 태그(SPEC-STORE-003)를 병합하여 표시하되, 키별로 동일한 태그 키가 충돌하면 정적 태그를 우선해야 한다. 정적 태그가 있는 키도 separator 변경의 영향을 받아 자동 추출 결과가 갱신되어야 한다.
- **Unwanted**: 시스템은 사용자가 지정한 separator 가 키에 매칭되지 않을 때 임의의 폴백 분리자로 추출을 시도하지 않아야 한다 (빈 결과 반환으로 사용자 인지 가능).

---

## v0.7.0 BREAKING 적응 (SPEC-STORE-003 v0.3.0 동기화)

> **의존성 노트**: 이 절의 모든 모듈은 SPEC-STORE-003 v0.3.0 (M9 — API 응답 진화) 의 백엔드 컨트랙트를 전제로 한다. 백엔드는 sealed 상태이며 본 SPEC 에서 수정하지 않는다.

### M11: Store Keys Response Type Evolution (v0.7.0, BREAKING for frontend)

- **Ubiquitous**: 시스템은 `GET /api/v1/store/{name}/keys` 응답을 v0.3.0 객체 배열 스키마 (`{count, keys: StoreKeyObject[]}`, `StoreKeyObject = {key, registration, data_type, metric_type, tags}`) 로 해석해야 한다.
- **Ubiquitous**: 시스템은 응답에서 별도의 `tags: Record<string, Record<string, string>>` 필드를 더 이상 기대하지 않아야 한다 (v0.2.0 분리 맵 구조 폐기).
- **Unwanted**: 시스템은 v0.2.0 형식 (string 배열 `keys`) 으로 동작하기 위한 호환 shim 또는 어댑터를 유지하지 않아야 한다 (clean migration; 백엔드 v0.3.0 와 1:1 동기).
- **Event-driven**: WHEN 응답 파싱 시 `keys` 항목이 string 또는 `key` 필드 누락된 객체이면, THEN 시스템은 해당 항목을 안전하게 스킵하고 콘솔 경고를 출력해야 한다 (네트워크 중간계층의 일시적 형식 오염 방어용).
- **State-driven**: WHILE 응답 객체 배열을 소비하는 화면이 동시에 이전 캐시 (TanStack Query) 와 신규 캐시를 보유할 수 있는 시점에는, 시스템은 두 형식이 섞여 있어도 런타임 크래시 (`t.includes is not a function`) 를 발생시키지 않아야 한다.

### M12: Store Config — registration_type Migration (v0.7.0)

- **Ubiquitous**: 시스템은 Store 에이전트 설정 폼에서 `allow_dynamic_keys: bool` 입력을 더 이상 표시하지 않아야 한다.
- **Ubiquitous**: 시스템은 `registration_type` 필드를 enum 입력 (세그먼트 컨트롤 또는 라디오 그룹) 으로 표시하며, 허용 값은 `"manual"` 과 `"auto"` 이고 기본값은 `"auto"` 이다.
- **Event-driven**: WHEN 사용자가 `registration_type` 을 `"manual"` 로 변경하면, THEN 시스템은 정적 키 목록의 모든 행에 대해 `data_type` 필드가 비어 있는 행을 강조 표시하고 저장 버튼을 비활성화해야 한다.
- **Unwanted**: 시스템은 사용자가 폼을 통해 백엔드로 `allow_dynamic_keys` 키를 포함한 페이로드를 전송하지 않아야 한다.
- **Event-driven**: WHEN 백엔드가 `"config: 'allow_dynamic_keys' is removed in v0.3.0; use 'registration_type: manual|auto' instead"` 메시지를 반환하면, THEN 시스템은 사용자에게 "이 에이전트의 yaml 설정이 구버전입니다. registration_type 으로 마이그레이션이 필요합니다." 안내 다이얼로그를 표시해야 한다.

### M13: Store Config — data_type & metric_type Editor (v0.7.0)

- **Ubiquitous**: 시스템은 `StoreKeysEditor` 의 키 행에 `data_type` 셀렉트 컬럼을 표시해야 한다. 옵션은 `int`, `float`, `string`, `boolean`, `bytes`, `json` 6종이며, manual 모드의 새 행 기본값은 unset (placeholder), auto 모드의 새 행 기본값은 `"float"` (단, optional 표기) 이다.
- **Ubiquitous**: 시스템은 `StoreKeysEditor` 의 키 행에 `metric_type` 텍스트 입력 컬럼을 표시해야 한다. 비어 있을 때의 effective 기본값은 `"unknown"` 이며, placeholder 로 `"unknown"` 을 표시한다.
- **State-driven**: WHILE `metric_type` 값이 정규식 `^[a-zA-Z0-9_-]+$` 를 위반하면, 시스템은 해당 입력 박스를 에러 상태로 표시하고 저장 버튼을 비활성화해야 한다 (빈 문자열은 검증 통과; 백엔드에서 default 적용).
- **State-driven**: WHILE `registration_type == "manual"` AND 어떤 키 행의 `data_type` 셀렉트가 unset 이면, 시스템은 해당 행을 에러 상태로 표시하고 저장 버튼을 비활성화해야 한다.
- **Ubiquitous**: 시스템은 키:`data_type`:`metric_type`:태그 컬럼 너비 비율을 `4:1.5:1.5:3` 으로 렌더링해야 한다 (v0.2.0 의 `1:3` 비율을 v0.7.0 메타데이터 추가에 맞춰 조정).
- **Event-driven**: WHEN 사용자가 키 행을 추가하면, THEN 시스템은 `registration_type` 에 따라 `data_type` 기본값을 다르게 적용해야 한다 (manual: unset/필수, auto: optional).

### M14: PromoteToStaticDialog Evolution (v0.7.0)

- **Ubiquitous**: 시스템은 동적 키를 정적 키로 승격하는 다이얼로그에 `data_type` 셀렉트 (6종 enum, 필수) 와 `metric_type` 텍스트 입력 (선택, 기본 placeholder `"unknown"`) 을 항상 포함해야 한다.
- **Event-driven**: WHEN 다이얼로그가 처음 열릴 때 해당 키의 현재 추론된 `data_type` 이 응답에서 알려져 있으면, THEN 시스템은 해당 값을 셀렉트의 초기값으로 사전 채워야 한다.
- **Unwanted**: 시스템은 `data_type` 미선택 상태에서 "정적 키로 승격" 버튼을 활성화하지 않아야 한다.
- **State-driven**: WHILE `metric_type` 입력이 정규식 위반 상태이면, 시스템은 승격 버튼을 비활성화하고 인라인 에러를 표시해야 한다.

### M15: Backend Error UX Mapping (v0.7.0)

- **Ubiquitous**: 시스템은 백엔드 응답 코드/메시지를 다음 사용자 친화 문구로 매핑해야 한다:
  - `ErrTypeMismatch` → "이 키에 등록된 타입과 일치하지 않는 값입니다. 키를 삭제 후 재등록하거나 올바른 타입의 값을 사용하세요."
  - `ErrUnsupportedValueType` → "지원되지 않는 값 타입입니다 (nil 또는 추론 불가)."
  - `ErrInvalidDataType` → "선언된 data_type 값이 유효하지 않습니다. 허용 값: int / float / string / boolean / bytes / json."
  - `ErrInvalidMetricType` → "metric_type 형식이 유효하지 않습니다 (영문/숫자/하이픈/언더스코어만 허용)."
  - `'allow_dynamic_keys' is removed` 메시지 → "이 에이전트의 yaml 설정이 구버전 (v0.2.0) 입니다. registration_type 으로 마이그레이션이 필요합니다."
- **Event-driven**: WHEN 백엔드 에러 응답이 위 식별자/메시지 패턴 중 하나에 매칭되면, THEN 시스템은 토스트 또는 폼 인라인 에러로 매핑된 문구를 표시하고 원본 메시지는 콘솔 또는 펼침 영역에 보조적으로 노출해야 한다.
- **Unwanted**: 시스템은 매핑되지 않은 에러를 받았을 때 빈 문자열이나 일반화된 "오류" 만 표시하고 침묵하지 않아야 한다 (원본 메시지 fallback 표시).

### M16: Key Metadata Display (v0.7.0)

- **Ubiquitous**: 시스템은 저장소 탭의 키 리스트와 `TsdbDataViewerModal` 의 시리즈 멀티셀렉트 행에 `data_type`, `metric_type`, `registration` 메타데이터를 보조 칩 또는 컬럼으로 표시해야 한다 (태그 칩과 시각적으로 구분; 예: 무채색 톤 + monospaced 폰트).
- **State-driven**: IF 해당 키의 `metric_type` 이 `"unknown"` 이면, 시스템은 해당 칩을 흐림 (muted) 톤으로 표시하거나 생략해야 한다 (미세 시각 노이즈 감소).
- **Optional (scope-flex)**: WHERE Decision Point 1 에서 신규 필터 UI 가 승인되면, 시스템은 키 리스트 헤더에 `data_type` / `metric_type` / `registration` 필터 칩을 기존 태그 필터와 병행하여 노출하고, 백엔드 `?data_type=` / `?metric_type=` / `?registration=` 쿼리 파라미터로 전송해야 한다 (AND 결합).
- **Optional (scope-flex)**: WHERE Decision Point 1 에서 자동/수동 키 시각 구분이 승인되면, 시스템은 `registration == "auto"` 인 키 행에 작은 배지 (`AUTO`) 를 표시하여 yaml 정의 키와 시각적으로 분리해야 한다.

---

## Resolved Defaults (Open Questions)

1. **Timezone**: UI는 브라우저 로컬 datetime 입력을 받고 라벨에 "(Local)"을 표기한다. 프론트엔드는 백엔드 전송 전 UTC epoch ms(int64)로 변환하며, 응답 timestamp 또한 epoch ms로 수신하여 브라우저 로컬로 렌더링한다. (근거: `project_timestamp_convention.md`)
2. **Matrix row soft limit**: 5,000행. `(end - start) / interval × selected_keys > 5000`이면 실행 전 경고 배너("결과 행 수가 많아 렌더링이 느릴 수 있습니다")를 표시하되 차단하지 않는다. 백엔드는 기존 `MaxQueryPoints` 제한을 유지한다.
3. **Aggregation UI**: 라디오 버튼으로 `min`, `max`, `average` 3개 옵션만 제공한다. 기본값: `average`.
4. **Interval input**: 프리셋 드롭다운 `[10s, 30s, 1m, 5m, 15m, 30m, 1h, 6h, 1d]` + "custom" 옵션 선택 시 Go duration 문법(예: `2m`, `45s`)을 허용하는 자유 입력 텍스트박스가 나타난다. 기본 프리셋: `1m`.

---

## Library & Version Constraints

- **Go**: 1.22+
- **Chi router**: 기존 버전 유지 (신규 의존성 없음)
- **React**: 19 stable
- **TypeScript**: 5.7+
- **TanStack Query**: v5.x
- **Zustand**: v5.x (전역 상태 필요 시 사용, 기본은 로컬 컴포넌트 state)
- **Tailwind CSS**: 기존 커스텀 CSS 병행
- **테스트**: Vitest (frontend), Go testing + testify (backend)
- **신규 외부 라이브러리**: 없음

---

## Related Documents

- Implementation plan: [plan.md](plan.md)
- Acceptance criteria: [acceptance.md](acceptance.md)
- 프로젝트 타임스탬프 규약: `~/.claude/projects/-Users-xtra-Projects-xflow/memory/project_timestamp_convention.md`

## Implementation Notes

### Divergence from Original Plan (v0.1.0)

- **파일 경로**: 계획에서는 `web/src/components/agents/Tsdb*.tsx` 와 `web/src/api/tsdb.ts` 였으나, 프로젝트 컨벤션에 맞춰 `web/src/pages/agents/Tsdb*.tsx` 와 `web/src/services/api/tsdb.ts` 로 배치
- **백엔드 query API 차이**: 기존 `POST /api/v1/tsdb/query` 는 단일 `series_key` + RFC3339Nano + `bucket` + `"avg"` 사용. 프론트엔드에 어댑터 레이어를 추가하여 다중 키 + epoch ms + `"average"` 인터페이스로 노출하고 N개 병렬 호출 + 클라이언트 merge 처리
- **시리즈 탭 통합**: 별도 "시리즈" 탭으로 시작했으나 v0.4.0 에서 store 에이전트는 기존 "저장소" 탭에 데이터 보기 버튼 + 페이지네이션을 통합. tsdb 타입 에이전트 (향후) 만 별도 시리즈 탭 유지
- **react-window v2 API**: 계획상 `FixedSizeList` 였으나 설치된 v2.2.7 은 `List` + rowComponent 패턴 사용. 기능적으로 동등
- **서버측 집계 통합**: v0.3.0 에서 store 서버측 집계 API 추가 후 클라이언트는 4xx 시 클라이언트 집계 fallback 으로 graceful degradation

### Architectural additions beyond plan

- **`SeriesDataSource` 통합 어댑터** (v0.2.0): tsdb 와 store 두 데이터소스를 동일 인터페이스로 추상화. UI 컴포넌트는 `kind` 무관하게 동일 동작
- **`tsdbCsvExport.ts`** (v0.3.0): 재사용 가능한 CSV 변환/다운로드 유틸 (zero-dep)
- **`TagFilterChips`** (v0.4.0): 재사용 가능한 태그 chip 필터 컴포넌트 + `matchesTagFilter` 유틸
- **`StoreKeysEditor`** (v0.4.0): 정적 키 + 태그 행 편집기 컴포넌트
- **`keyTagExtractor`** (v0.5.0): 키 문자열에서 태그를 자동 추출하는 유틸. InfluxDB 라인 프로토콜 스타일 (`measurement,k=v,k=v`) 와 colon/slash 구분자 segments 위치 기반 (`seg0/seg1/seg2`) 두 가지 모드 지원. 정적 태그가 없을 때 fallback 으로 사용되어 TSDB 타입 에이전트도 태그 chip 필터 활용 가능.
- **`TsdbResultChart`** (v0.6.0): recharts `LineChart` 기반 차트 뷰. 컬럼별 8개 cyclic 색상 팔레트, animation 비활성, `connectNulls` 미사용. null 처리 4모드 셀렉트와 채움값 입력을 차트 컨테이너 상단 툴바에 노출.
- **`tsdbChartNullHandling`** (v0.6.0): 차트 결측값 처리 4모드 (`gap` / `previous` / `value` / `interpolate`) 변환 유틸. 단위 테스트 15개 동반.

### v0.6.0 Notes

- **모달 레이아웃 안정성**: 폼 영역에 `min-h-0 shrink overflow-y-auto`, 매트릭스 영역에 `min-h-[180px]`, 절대↔상대 모드 전환부에 `min-h-[7.5rem]` 사전 reserve 적용. 작은 화면에서도 폼이 매트릭스/푸터를 가리지 않으며 인터벌 위치가 흔들리지 않음.
- **시간 범위 기본 모드 변경**: 절대(absolute) → 상대(relative). 모달 오픈/리오픈 시 항상 'relative' 로 리셋.
- **시리즈 행 태그 열**: `tagFilter.tagsByKey[k]` 로부터 자동 추출 태그 칩 최대 3개를 행 우측에 노출. 라벨에 `textGrowth: fixed-width + width: fill_container` 적용으로 우측 정렬 확보.
- **태그 필터 라벨 변경 + separator 설정**: "태그로 필터링" → "필터링". 헤더에 4자 이내 separator 입력 박스 추가, 모달 오픈 시 ':' 로 리셋, 변경 시 기존 태그 필터 선택 자동 초기화.
- **정적 태그 + 자동 추출 병합**: `keyTagExtractor.ts` 의 `extractTagsFromKey` / `buildExtractedTagPairs` / `buildExtractedTagsByKey` 모두 `separator: string` 파라미터를 명시적으로 받도록 변경. `extractTagsFromKey` 는 폴백 제거 (사용자 separator 미매칭 시 빈 객체 반환). `buildExtractedTagPairs` / `buildExtractedTagsByKey` 는 자동 추출 + 정적 태그 병합 (정적 태그가 동일 키 충돌 시 우선). 정적 태그가 있는 키도 separator 변경의 영향을 받음.
- **결과 뷰 모드 토글**: TsdbResultMatrix 헤더에 테이블/차트 토글 탭 추가. 차트 모드에서는 테이블/페이지네이션 미노출.
- **차트 결측값 처리**: 4모드 (`gap`, `previous`, `value`, `interpolate`). `interpolate` 는 양쪽 알려진 값 사이만 선형 보간하고 한쪽만 있으면 forward-fill, leading null 은 유지. `value` 모드에서만 number input 노출.
- **viewMode 영속화**: TsdbResultMatrix 에 `viewMode?` + `onViewModeChange?` controlled props 추가 (uncontrolled 기본 동작 유지). TsdbDataViewerModal 에 `resultViewMode` state 추가, 모달 오픈 시 'table' 로 리셋. 다시 실행 클릭으로 인한 unmount→remount 사이에도 사용자 선택 표시 모드 유지.

### v0.5.0 Notes

- **react-window 제거**: 매트릭스 가상 스크롤 (500행 자동 가상화) 을 페이지네이션으로 대체. 페이지 크기 [10, 25(기본), 50, 100]. `react-window` 의존성은 `package.json` 에 남아있으나 활성 사용처 없음.
- **소수점 자릿수 옵션**: 평균 집계 시 사용자가 0-6 자릿수 지정 (기본 1). 매트릭스 셀과 CSV 내보내기 양쪽에 `toFixed(n)` 동일 적용.
- **태그 자동 추출 fallback**: 정적 태그(SPEC-STORE-003) 미존재 시에만 동작. 사용자 명시 태그가 우선이며 추출 결과는 보조.

### v0.7.0 Notes (BREAKING 적응)

- **타입 진화 전략**: 어댑터 (string[] ↔ object[]) 유지가 아닌 **풀 마이그레이션** 채택. 백엔드 v0.3.0 은 명시적으로 BREAKING 으로 설계되었고 운영자에게 노출되지 않은 내부 컨트랙트이므로 어댑터 보존은 기술 부채만 누적시킨다 (Decision Point 1 — 권장: 풀 마이그레이션).
- **타입 정의 위치**: `web/src/services/api/store.ts` 에 `StoreKeyObject` interface 신설:
  ```ts
  export interface StoreKeyObject {
    key: string;
    registration: 'manual' | 'auto';
    data_type: 'int' | 'float' | 'string' | 'boolean' | 'bytes' | 'json';
    metric_type: string;            // default 'unknown'
    tags: Record<string, string>;
  }
  export interface StoreKeysRawResponse {
    count: number;
    keys: StoreKeyObject[];
  }
  ```
  `tags` map 별도 필드는 응답 타입에서 제거 (v0.2.0 폐기). `fetchStoreKeysWithTags` 는 응답 객체 배열을 그대로 반환하거나, 호환을 위해 `{keys: string[], tags: Record<string,Record<string,string>>}` 형태로 어댑팅 (구체적인 형태는 plan.md 의 Task 7 결정).
- **컴포넌트 영향 범위 (확정)**:
  - `web/src/services/api/store.ts` (lines 41-45 type, 141-146 fetchStoreKeys, 392-402 fetchStoreKeysWithTags, 358-382 useStoreKeys, 405-414 useStoreKeysWithTags) — 타입 + 어댑터 진화
  - `web/src/services/api/store.test.ts` — 응답 fixture 마이그레이션 (string 배열 → 객체 배열)
  - `web/src/services/api/storeService.ts` (line 7 type, line 18 URL) — 별도 클라이언트, deprecation 후 제거 권장
  - `web/src/pages/agents/TsdbDataViewerModal.tsx` (line 49, 51, 183, 239) — 객체 배열 소비
  - `web/src/services/api/seriesDataSource.ts` (line 23, 35) — 타입 정의 동기화
  - `web/src/components/property/StoreKeysEditor.tsx` — `data_type` / `metric_type` 컬럼 추가, 컬럼 비율 4:1.5:1.5:3
  - `web/src/components/property/PromoteToStaticDialog.tsx` — `data_type` / `metric_type` 입력 필드 추가
  - `web/src/config/agentSchemas.ts` (line 234, 249, 277) — `allow_dynamic_keys` 제거, `registration_type` 추가, `STORE_DATA_FIELDS` set 갱신
  - `web/src/pages/agents/AgentDetailPanel.tsx` (line 572) — 주석/도움말 갱신, registration_type 토글 placement
- **에러 매핑 위치**: 신규 `web/src/services/api/storeErrorMessages.ts` (또는 동등) 모듈에 식별자/메시지 패턴 → 사용자 친화 문구 맵핑 테이블 도입. 토스트 시스템 + 폼 인라인 에러 양쪽에서 재사용.
- **scope-flex 항목 (Decision Point 1)**:
  - 신규 필터 UI (data_type/metric_type/registration 필터 칩) — 권장: 포함 (사용성 큼, 비용 작음)
  - 자동/수동 키 시각 구분 (AUTO 배지) — 권장: 포함 (단일 컴포넌트 변경, 디버깅 가치 큼)
- **테스트 전략**: hybrid (DDD for 기존 컴포넌트 수정, TDD for 신규 모듈). characterization 테스트는 기존 동작 보존이 아닌 v0.6.0 까지의 통과 테스트 fixture 진화이므로 fixture 마이그레이션이 본질이다.

### Status: in_progress (Level 1 spec-first lifecycle, v0.7.0 evolution from completed v0.6.0)
