---
spec_id: SPEC-WEB-005
version: 0.6.0
status: completed
---

# SPEC-WEB-005: Implementation Plan

## 기술 스택 및 라이브러리 버전

### Backend

- **Go**: 1.22+ (프로젝트 기본)
- **Chi router**: 기존 버전 유지 (신규 의존성 없음)
- **테스트**: Go testing 표준 패키지 + `github.com/stretchr/testify` (기존 사용)

### Frontend

- **React**: 19 stable
- **TypeScript**: 5.7+
- **TanStack Query**: v5.x (API 호출 캐싱/상태 관리)
- **Zustand**: v5.x (필요 시 전역 상태; 기본은 로컬 `useState`/`useReducer`)
- **Tailwind CSS**: 기존 설정 활용
- **테스트**: Vitest + React Testing Library

### 신규 외부 라이브러리

없음. 기존 프로젝트 의존성만 사용한다.

---

## Task Decomposition

### Phase 1 — 백엔드 확장 (DDD: 기존 핸들러 보존 + 점진적 확장)

#### Task 1: DTO 확장

- **파일**: `internal/api/dto/tsdb.go`
- **작업**:
  - `PaginationMeta` 구조체 추가 (`Page`, `Size`, `Total`, `TotalPages` 필드)
  - `TSDBSeriesListResponse`에 optional `Pagination *PaginationMeta` 필드 추가 (nil이면 직렬화에서 생략되어 하위 호환성 유지)
- **우선순위**: High
- **의존성**: 없음

#### Task 2: Characterization 테스트 (DDD - PRESERVE)

- **파일**: `internal/api/handler/tsdb_test.go` (신규 또는 기존 확장)
- **작업**:
  - `page`/`size` 파라미터 없이 호출 시 기존 응답 포맷 `{Series []string, Count int}` 동일하게 유지됨을 보장하는 테스트 작성
  - 빈 시리즈 목록 응답 동작 확인
  - 기존 `MaxQueryPoints` 경로 확인
- **우선순위**: High
- **의존성**: 없음 (Task 3 이전 실행 필수)

#### Task 3: 핸들러 수정 — `ListSeries`

- **파일**: `internal/api/handler/tsdb.go`
- **작업**:
  - `page`, `size`, `agent_id` 쿼리 파라미터 파싱 (모두 optional)
  - 파라미터 없으면 기존 동작 유지 (backward-compatible)
  - `size` 상한 100 강제, 기본값 25
  - `agent_id`는 현재는 싱글톤 라우팅으로 무시하되 검증 로직만 추가
  - 페이징된 결과에 `Pagination` 메타 포함
- **우선순위**: High
- **의존성**: Task 1, Task 2

#### Task 4: 신규 페이징 동작 TDD 테스트

- **파일**: `internal/api/handler/tsdb_test.go`
- **작업**:
  - `size=10/25/50/100` 각각의 페이징 동작 검증
  - `size=101` 입력 시 상한 100으로 캡핑 또는 400 에러 반환 (결정 후 구현)
  - 빈 결과 + pagination 메타 응답 확인
  - 잘못된 `page` 입력(음수, 0, 문자열) 처리 검증
- **우선순위**: High
- **의존성**: Task 3

---

### Phase 2 — 프론트엔드 구현 (TDD: 신규 컴포넌트)

#### Task 5: API 훅 정의

- **파일**: `web/src/api/tsdb.ts` (신규)
- **작업**:
  - `useTsdbSeries({ page, size, agentId? })` — TanStack Query 기반 시리즈 목록 훅
  - `useTsdbQuery()` — mutation 훅으로 쿼리 실행
  - epoch ms 변환 유틸 (`toEpochMs(localDate)`, `fromEpochMs(ms)`) 포함
- **우선순위**: High
- **의존성**: 백엔드 Phase 1 완료

#### Task 6: Series List Panel 컴포넌트

- **파일**: `web/src/components/agents/TsdbSeriesListPanel.tsx` (신규)
- **작업**:
  - 페이지 크기 셀렉터 (`10/25/50/100`)
  - 페이지네이션 컨트롤 (이전/다음/직접 선택)
  - 행별 "데이터 보기" 버튼
  - 로딩 스켈레톤, 빈 상태 메시지
- **우선순위**: High
- **의존성**: Task 5

#### Task 7: Data Viewer Modal 컴포넌트

- **파일**: `web/src/components/agents/TsdbDataViewerModal.tsx` (신규)
- **작업**:
  - 기존 `ImportDialog` 패턴 재사용 (포털, 포커스 트랩, Esc 처리)
  - 시리즈 멀티셀렉트 (autocomplete + 체크박스)
  - 시작/종료 datetime 입력 (브라우저 로컬, "(Local)" 라벨)
  - 인터벌 프리셋 드롭다운 + custom 자유 입력
  - 집계 함수 라디오 그룹 (`min`/`max`/`average`, 기본 `average`)
  - 실행 버튼, 진행 인디케이터, 에러 배너
  - 5,000행 초과 시 확인/취소 경고 배너
- **우선순위**: High
- **의존성**: Task 5

#### Task 8: Result Matrix 컴포넌트

- **파일**: `web/src/components/agents/TsdbResultMatrix.tsx` (신규)
- **작업**:
  - 시리즈 키 × 시간 버킷 매트릭스 렌더링
  - 누락 셀 em-dash(`—`) 표시
  - 타임스탬프 컬럼 브라우저 로컬 형식(`YYYY-MM-DD HH:mm:ss`)
  - 수직/수평 스크롤 지원
- **우선순위**: High
- **의존성**: Task 7

#### Task 9: AgentDetailPanel 통합

- **파일**: `web/src/components/agents/AgentDetailPanel.tsx` (수정)
- **작업**:
  - `agent.type === "tsdb"` 분기 추가
  - `TsdbSeriesListPanel` 렌더링
  - `TsdbDataViewerModal` 포털 마운트 및 상태 연결
- **우선순위**: High
- **의존성**: Task 6, Task 7

#### Task 10: Frontend 테스트 작성

- **파일**: `web/src/components/agents/__tests__/TsdbSeriesListPanel.test.tsx`, `TsdbDataViewerModal.test.tsx`, `TsdbResultMatrix.test.tsx` (신규)
- **작업**:
  - Vitest + React Testing Library
  - 시리즈 목록 페이지 크기 변경, 페이지 이동 상호작용
  - 모달 열림/닫힘, Esc 처리, 외부 클릭
  - 역순 시간 범위 시 버튼 비활성화
  - 5,000행 경고 배너 표시
  - epoch ms 변환 정확성
- **우선순위**: Medium
- **의존성**: Task 6, Task 7, Task 8

---

### Phase 3 — 문서 동기화

#### Task 11: 문서 업데이트

- **파일**: `CHANGELOG.md`, `README.md` (필요 시 `docs/api.md`)
- **작업**:
  - API 변경 기록: `GET /api/v1/tsdb/series` optional `page`/`size`/`agent_id` 추가, 응답 DTO에 optional `pagination` 필드 추가
  - UI 기능 추가 기록: TSDB 에이전트 상세 패널 데이터 뷰어
- **우선순위**: Low
- **의존성**: Phase 1, Phase 2 완료

---

### Phase 4 — v0.6.0 데이터 뷰어 UX/시각화 확장 (frontend-only)

#### Task 12: 모달 레이아웃 안정화

- **파일**: `web/src/pages/agents/TsdbDataViewerModal.tsx` (수정)
- **작업**:
  - 폼 영역에 `min-h-0 shrink overflow-y-auto`, 매트릭스 영역에 `min-h-[180px]` 적용
  - 시리즈 fieldset 좌(검색+체크박스 리스트) / 우(태그 필터) 2열 레이아웃
  - 시간 범위 영역 좌(범위+인터벌) / 우(집계 함수) 2열 레이아웃, 인터벌은 범위 아래
  - 절대↔상대 모드 전환 시 인터벌 위치 안정화를 위해 범위 컨트롤 영역에 `min-h-[7.5rem]` 사전 reserve
  - 선택된 시리즈 별도 pill 영역 제거 (체크 표시로만 표현)
- **우선순위**: High
- **의존성**: 없음
- **비고**: R-V-008 ~ R-V-011

#### Task 13: 시간 범위 기본 모드 변경 (relative)

- **파일**: `web/src/pages/agents/TsdbDataViewerModal.tsx` (수정)
- **작업**:
  - 시간 범위 모드 초기값을 'absolute' → 'relative' 로 변경
  - 모달 오픈/리오픈 시 항상 'relative' 로 리셋
- **우선순위**: High
- **의존성**: 없음
- **비고**: R-V-012

#### Task 14: 시리즈 행 태그 칩 열 + separator 설정 + 정적/자동 병합

- **파일**:
  - `web/src/pages/agents/TsdbDataViewerModal.tsx` (수정)
  - `web/src/pages/agents/keyTagExtractor.ts` (수정)
  - `TagFilterChips` 컴포넌트 (수정)
- **작업**:
  - 시리즈 행 우측에 자동 추출 태그 칩 최대 3개 노출 (`series-row-tags-{key}`), 라벨 `textGrowth: fixed-width + width: fill_container`
  - `TagFilterChips` 기본 라벨 "태그로 필터링" → "필터링", `separator` + `onSeparatorChange` optional props 추가, 4자 이내 입력 박스 (`tag-segment-separator`)
  - 모달 오픈 시 separator ':' 로 리셋, 변경 시 `tagFilter.clearAll()` 호출로 기존 태그 필터 자동 초기화
  - `extractTagsFromKey`, `buildExtractedTagPairs`, `buildExtractedTagsByKey` 3개 함수에 `separator: string` 파라미터 추가
  - `extractTagsFromKey`: 사용자 separator 미매칭 시 빈 객체 반환 (폴백 제거)
  - `buildExtractedTagPairs` / `buildExtractedTagsByKey`: 자동 추출 + 정적 태그 병합, 충돌 시 정적 우선
- **우선순위**: High
- **의존성**: Task 12
- **비고**: R-V-011, R-V-013

#### Task 15: 결과 뷰 모드 토글 + TsdbResultChart

- **파일**:
  - `web/src/pages/agents/TsdbResultChart.tsx` (신규)
  - `web/src/pages/agents/TsdbResultMatrix.tsx` (수정)
- **작업**:
  - recharts `LineChart` 기반 차트 컴포넌트 신규 작성, 컬럼별 8개 cyclic 색상 팔레트, animation 비활성, `connectNulls` 미사용
  - TsdbResultMatrix 헤더에 토글 탭 추가 (`tsdb-result-view-table`, `tsdb-result-view-chart`)
  - 차트 모드에서는 테이블/페이지네이션 미노출
- **우선순위**: High
- **의존성**: Task 12
- **비고**: R-V-001 ~ R-V-003

#### Task 16: 차트 결측값 처리 4모드

- **파일**:
  - `web/src/pages/agents/tsdbChartNullHandling.ts` (신규)
  - `web/src/pages/agents/tsdbChartNullHandling.test.ts` (신규)
  - `web/src/pages/agents/TsdbResultChart.tsx` (통합)
- **작업**:
  - 4모드 변환 유틸 작성: `gap` (라인 끊김), `previous` (forward-fill), `value` (사용자 상수 채움), `interpolate` (양쪽 알려진 값 사이 선형 보간; 한쪽만 있으면 forward-fill, leading null 유지)
  - 차트 컨테이너 상단 툴바에 select (`tsdb-chart-null-mode`) 노출
  - `value` 모드에서만 number input (`tsdb-chart-null-fill-value`) 노출
  - 단위 테스트 15개
- **우선순위**: High
- **의존성**: Task 15
- **비고**: R-V-004 ~ R-V-006

#### Task 17: viewMode 영속화 (controlled props)

- **파일**:
  - `web/src/pages/agents/TsdbResultMatrix.tsx` (수정)
  - `web/src/pages/agents/TsdbDataViewerModal.tsx` (수정)
- **작업**:
  - TsdbResultMatrix 에 `viewMode?: 'table' | 'chart'` + `onViewModeChange?: (mode) => void` controlled props 추가 (uncontrolled 기본 동작 유지)
  - TsdbDataViewerModal 에 `resultViewMode` state 추가, 모달 오픈 시 'table' 로 리셋
  - `<SeriesResultMatrix>` unmount→remount 사이 사용자 선택 표시 모드 유지 검증
- **우선순위**: High
- **의존성**: Task 15
- **비고**: R-V-007

#### Task 18: 디자인 자료 분리 (참고)

- **파일**:
  - `references/design/tsdb-data-viewer.pen` (신규)
  - `references/design/gauge-chart.pen` (신규; SPEC-WEB-005 직접 범위 외)
- **작업**:
  - 상대/절대/차트 3종 모달 변형을 단독 .pen 파일로 분리
  - 게이지 차트 패널 10종을 단독 .pen 파일로 분리 (참고 자료)
- **우선순위**: Low
- **의존성**: 없음

---

## API 변경 사항

### `GET /api/v1/tsdb/series`

#### 기존 (변경 없음)

호출: `GET /api/v1/tsdb/series`
응답:
```json
{ "series": ["temp,room=1", "temp,room=2"], "count": 2 }
```

#### 신규 (페이징)

호출: `GET /api/v1/tsdb/series?page=1&size=25&agent_id=tsdb-1`
응답:
```json
{
  "series": ["temp,room=1", "..."],
  "count": 25,
  "pagination": {
    "page": 1,
    "size": 25,
    "total": 120,
    "total_pages": 5
  }
}
```

#### 파라미터 규칙

- `page`: 1 이상 정수, 기본 1
- `size`: `[10, 25, 50, 100]` 허용, 기본 25, 상한 100
- `agent_id`: 선택, 향후 멀티 인스턴스 라우팅용

### `POST /api/v1/tsdb/query`

기존 엔드포인트 사용. 요청 바디는 기존 스펙 유지:
```json
{
  "keys": ["temp,room=1", "temp,room=2"],
  "start_ms": 1714867200000,
  "end_ms": 1714888800000,
  "interval": "1h",
  "aggregation": "average"
}
```

변경 없음. 기존 `MaxQueryPoints` 제한 유지.

---

## Risks & Mitigations

### Risk 1: 대용량 결과 렌더링 성능

- **영향**: 5,000행 × 여러 시리즈 매트릭스 렌더링 시 DOM 성능 저하 가능
- **완화**:
  - 5,000행 초과 예상 시 실행 전 경고 배너
  - 가상 스크롤(react-window) 도입은 초기 범위에서 제외, 후속 최적화로 분리
  - 백엔드 `MaxQueryPoints` 제한은 이중 방어막으로 유지

### Risk 2: 타임존 불일치

- **영향**: 브라우저 로컬 입력과 백엔드 UTC 저장 간 혼동으로 잘못된 범위 조회
- **완화**:
  - 모든 datetime 입력 라벨에 "(Local)" 명시
  - 전송 전 `toEpochMs(localDate)` 일관 변환
  - 응답 epoch ms도 동일 변환 경로로 렌더링
  - 단위 테스트로 변환 왕복(round-trip) 검증

### Risk 3: API 하위 호환성

- **영향**: 기존 호출부가 `page`/`size` 없이 호출 시 응답 포맷 변경으로 파손 가능
- **완화**:
  - `page`/`size`/`agent_id`는 모두 optional
  - 파라미터 미지정 시 기존 응답 `{Series []string, Count int}` 유지
  - `Pagination`은 `omitempty`로 직렬화되어 기존 클라이언트에 영향 없음
  - Characterization 테스트(Task 2)로 기존 동작 보존 검증

### Risk 4: TSDB 에이전트 멀티 인스턴스 확장

- **영향**: 향후 TSDB 에이전트가 여러 인스턴스로 확장될 때 라우팅 변경 필요
- **완화**:
  - optional `agent_id` 쿼리 파라미터 선제 도입
  - 현재는 싱글톤 라우팅이므로 파라미터 존재 시 검증만 수행하고 무시
  - 향후 에이전트 레지스트리 연동 시 파라미터 해석 로직만 추가하면 됨

### Risk 5: 인터벌 파싱 불일치

- **영향**: 프론트엔드와 Go `time.ParseDuration` 간 문법 차이로 잘못된 요청 전송 가능
- **완화**:
  - Go duration 문법만 허용 (`s`, `m`, `h`, `ms` 등)
  - 프론트엔드 정규식 검증으로 잘못된 입력 사전 차단
  - 프리셋 9개로 대부분 케이스 커버, custom은 보조 수단

---

## Testing Strategy

### 방식: Hybrid (DDD for 기존 핸들러 확장, TDD for 신규 컴포넌트)

#### Backend (DDD)

- **PRESERVE**: Characterization 테스트로 기존 `ListSeries` 응답 포맷 보존 검증 (Task 2)
- **IMPROVE**: 페이징 동작은 신규 기능이므로 TDD로 구현 (Task 4)
- **커버리지 목표**: `internal/api/handler/tsdb.go` 85% 이상

#### Frontend (TDD)

- **RED-GREEN-REFACTOR**: 신규 컴포넌트 3종 모두 테스트 선작성
- **도구**: Vitest, React Testing Library, `@testing-library/user-event`
- **커버리지 목표**: 신규 컴포넌트 라인 커버리지 85% 이상

#### E2E 검증

- 수동 검증 시나리오는 `acceptance.md`의 5개 시나리오로 정의
- 후속 Playwright 자동화는 별도 SPEC으로 분리

### TRUST 5 정렬

- **Tested**: 85% 라인 커버리지 + characterization 보존
- **Readable**: 컴포넌트 분리(Panel/Modal/Matrix), 명확한 이름
- **Unified**: 기존 dialog 패턴(`ImportDialog`) 재사용, 일관된 API 훅 컨벤션
- **Secured**: 입력 검증(범위, 인터벌 파싱), 서버 에러 메시지 노출 시 XSS 방어
- **Trackable**: Conventional commits, SPEC-WEB-005 참조

---

## v0.6.0 변경 모듈 요약

### 신규 모듈

- `web/src/pages/agents/TsdbResultChart.tsx`: recharts LineChart 기반 차트 뷰
- `web/src/pages/agents/tsdbChartNullHandling.ts`: 차트 결측값 처리 4모드 변환 유틸
- `web/src/pages/agents/tsdbChartNullHandling.test.ts`: 결측값 처리 단위 테스트 (15개)

### 변경 모듈 (수정)

- `web/src/pages/agents/TsdbDataViewerModal.tsx`: 모달 레이아웃 재구성, 시간 범위 기본 모드 변경, separator 통합, viewMode 상태 영속화
- `web/src/pages/agents/TsdbResultMatrix.tsx`: 결과 뷰 모드 토글 + controlled `viewMode` props
- `web/src/pages/agents/keyTagExtractor.ts`: separator 파라미터 추가, 폴백 제거, 정적 태그 병합
- `web/src/pages/agents/TsdbDataViewerModal.test.tsx`: 시리즈 행 태그 열 / separator / 시간 범위 기본 모드 / 다시 실행 후 모드 보존 검증
- `web/src/pages/agents/TsdbResultMatrix.test.tsx`: 토글 탭 / 차트 모드 페이지네이션 비노출 / controlled prop 검증
- `TagFilterChips` 컴포넌트: 기본 라벨 변경, `separator` / `onSeparatorChange` optional props 추가

### 디자인 자료 (참고)

- `references/design/tsdb-data-viewer.pen`: 상대/절대/차트 3종 모달 변형
- `references/design/gauge-chart.pen`: 게이지 차트 패널 10종 (SPEC-WEB-005 직접 범위 외)

### 통계

- 변경 파일: 7개 수정 + 3개 신규 생성 (코드 + 테스트)
- 신규 테스트: 18개 (15 null handling + 3 view toggle integration + 1 controlled prop)
- 전체 테스트: 486/486 통과
- TypeScript: strict 통과
- 신규 외부 라이브러리: 없음 (`recharts` 는 기존 의존성)
