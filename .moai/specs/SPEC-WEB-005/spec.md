---
id: SPEC-WEB-005
version: 0.4.0
status: completed
created: 2026-04-23
updated: 2026-04-26
author: xtra
priority: medium
---

# SPEC-WEB-005: TSDB/Store 에이전트 시리즈 탐색 및 데이터 뷰어

## HISTORY

- **0.4.0** (2026-04-24): Store 에이전트 UI에 "시리즈" 탭 제거 및 "저장소" 탭 통합, 설정 탭을 운영/데이터 2개 섹션으로 시각 분리(데이터 섹션은 SPEC-STORE-003 정적 키+태그 행 편집기), 저장소 리스트에 태그 컬럼 및 유니크 태그 chip 필터 추가, 데이터 보기 모달의 시리즈 멀티셀렉트 상단에 동일 chip 필터 추가. 백엔드 태그 API(SPEC-STORE-003)와 연동하여 클라이언트측 필터링은 AND 조건. 하위호환: 태그 없는 키/에이전트는 태그 컬럼 숨김 및 필터 미노출. 관련 SPEC: SPEC-STORE-003.
- **0.3.0** (2026-04-24): 시간 범위 절대/상대 모드 토글 UI, 결과 매트릭스 CSV 내보내기, store 서버측 집계 지원, 매트릭스 가상 스크롤(react-window) 추가. 백엔드 `POST /api/v1/store/{agent_name}/query` 에 optional `interval_ms` + `aggregation` 필드 추가(하위호환 유지). 프론트엔드는 서버측 집계 우선, 4xx 에러 시 클라이언트 집계 fallback. `react-window` v2.2.7 의존성 추가, 500행 이상에서 자동 가상화. CSV 내보내기는 zero-dependency Blob 다운로드 방식(UTF-8 BOM 포함, 로컬 ISO-8601 타임존 오프셋 표기). 326 tests pass (+32 신규).
- **0.2.0** (2026-04-23): `useSeriesDataSource` 추상화 추가로 `type === 'tsdb'` 와 `type === 'store'` 에이전트 모두에서 시리즈 탭을 지원. Store 는 서버 측 페이지네이션/집계가 없어 전체 키 로드 → 클라이언트 슬라이스, `time_range` 모드 원본 엔트리 → 클라이언트 측 버킷/집계 전략을 사용한다. 기존 `TsdbSeriesListPanel`/`TsdbDataViewerModal`/`TsdbResultMatrix` 는 `dataSource: SeriesDataSource` prop 을 받도록 리팩터되었고, `SeriesResultMatrix` 명시적 export 가 추가되었다. 백엔드 변경 없음.
- **0.1.0** (2026-04-23): Initial draft — TSDB 에이전트 상세 패널의 시리즈 페이지네이션, 데이터 뷰어 모달, 멀티 시리즈 쿼리 매트릭스 렌더링 요구사항 정의

---

## Overview

### 목적

xflow 대시보드의 에이전트 상세 패널에서 TSDB 타입 에이전트가 보유한 시리즈 키를 탐색하고, 사용자가 선택한 시리즈에 대해 시간 범위/인터벌/집계 함수를 지정하여 히스토리 데이터를 매트릭스 형태로 조회할 수 있는 UI를 제공한다.

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

### Status: completed (Level 1 spec-first lifecycle)
