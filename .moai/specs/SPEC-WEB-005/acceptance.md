---
spec_id: SPEC-WEB-005
version: 0.7.0
status: in_progress
updated: 2026-05-05
---

# SPEC-WEB-005: Acceptance Criteria

## v0.7.0 Note

본 문서의 v0.6.0 까지의 시나리오 (Scenario 1~10) 와 Edge Case Checklist 는 모두 **그대로 보존**된다. v0.7.0 진화는 SPEC-STORE-003 v0.3.0 BREAKING 적응을 위한 frontend 단독 변경이며, 신규 시나리오 (Scenario 11~16) 와 v0.7.0 Edge Case Checklist 가 추가된다.

## Given-When-Then 시나리오

### Scenario 1: 페이지 크기 변경

- **Given** TSDB 에이전트 상세 패널이 열려 있고, 시리즈 총 120개가 페이지 크기 10으로 표시된 상태
- **When** 사용자가 페이지 크기 셀렉터를 `25`로 변경
- **Then** 시스템은 다음을 수행한다:
  - `GET /api/v1/tsdb/series?page=1&size=25`를 호출한다
  - 페이지 1 위치에서 첫 25개 시리즈를 렌더링한다
  - 페이지네이션 컨트롤에 총 5페이지를 표시한다 (120 ÷ 25 = 4.8 → 5페이지)
  - 현재 페이지 번호는 1로 리셋된다

---

### Scenario 2: 데이터 뷰어 해피 패스

- **Given** 사용자가 시리즈 `temp,room=1` 행의 "데이터 보기" 버튼을 클릭하여 모달이 열린 상태이며, 해당 시리즈가 기본 선택되어 있다
- **When** 사용자가 다음 동작을 수행:
  - 추가로 `temp,room=2`를 멀티셀렉트에서 선택
  - 시작 시각을 `2026-04-23 00:00`(Local), 종료 시각을 `2026-04-23 06:00`(Local)로 입력
  - 인터벌 프리셋을 `1h`로 선택
  - 집계 라디오에서 `average` 선택 (기본값 유지)
  - 실행 버튼 클릭
- **Then** 시스템은 다음을 수행한다:
  - 로컬 시각을 UTC epoch ms(int64)로 변환하여 `POST /api/v1/tsdb/query` 전송
  - 요청 바디에 `keys: ["temp,room=1", "temp,room=2"]`, `interval: "1h"`, `aggregation: "average"` 포함
  - 응답 수신 후 컬럼 순서 `[timestamp, temp,room=1, temp,room=2]`의 매트릭스를 렌더링
  - 1시간 버킷 기준 6개 행 렌더링 (00:00 ~ 05:00)
  - 누락된 버킷-시리즈 조합의 셀에 em-dash(`—`) 표시
  - 타임스탬프 컬럼은 브라우저 로컬 형식 `YYYY-MM-DD HH:mm:ss`

---

### Scenario 3: 역순 시간 범위 검증

- **Given** 데이터 뷰어 모달이 열려 있고, 키 1개 이상이 선택된 상태
- **When** 사용자가 종료 시각을 시작 시각보다 이전 값으로 입력 (예: 시작 `2026-04-23 10:00`, 종료 `2026-04-23 08:00`)
- **Then** 시스템은 다음을 수행한다:
  - 실행 버튼을 비활성(disabled) 상태로 유지한다
  - 시간 입력 필드 하단에 에러 메시지 "종료 시각은 시작 시각 이후여야 합니다"를 표시한다
  - `POST /api/v1/tsdb/query` 네트워크 요청은 **전송하지 않는다**
  - 사용자가 입력을 수정하여 유효한 범위가 되면 에러 메시지는 사라지고 실행 버튼이 활성화된다

---

### Scenario 4: 5,000행 초과 경고

- **Given** 데이터 뷰어 모달에서 사용자가 다음과 같이 설정한 상태:
  - 선택 시리즈: 5개
  - 시간 범위: 30일 (2026-03-24 00:00 ~ 2026-04-23 00:00, Local)
  - 인터벌: `1m`
  - 예상 행 수: `30일 × 24h × 60m × 5 keys / 5 keys = 43,200 buckets × 5 = 43,200` 매트릭스 행 (약 43,200행, 5,000 초과)
- **When** 사용자가 실행 버튼을 클릭
- **Then** 시스템은 다음을 수행한다:
  - 즉시 쿼리를 전송하지 않는다
  - 경고 배너를 표시한다: "결과 행 수가 많아 렌더링이 느릴 수 있습니다" (예상 행 수 숫자 포함)
  - "계속 실행" 및 "취소" 버튼을 제공한다
  - 사용자가 "계속 실행" 선택 시 → `POST /api/v1/tsdb/query` 전송
  - 사용자가 "취소" 선택 시 → 모달은 유지되고 쿼리 전송되지 않으며, 사용자는 조건을 재조정할 수 있다

---

### Scenario 5: Edge Case — 빈 시리즈 목록

- **Given** TSDB 에이전트에 저장된 시리즈가 0개인 상태
- **When** 사용자가 해당 TSDB 에이전트의 상세 패널을 연다
- **Then** 시스템은 다음을 수행한다:
  - `GET /api/v1/tsdb/series?page=1&size=25` 호출
  - 응답 `{ series: [], count: 0, pagination: { page: 1, size: 25, total: 0, total_pages: 0 } }` 수신
  - 시리즈 리스트 패널에 "저장된 시리즈가 없습니다" 빈 상태 메시지 표시
  - 페이지 크기 셀렉터는 비활성(disabled) 상태로 렌더링
  - 페이지네이션 컨트롤은 숨김 또는 비활성화
  - "데이터 보기" 버튼이 있는 행이 존재하지 않으므로 모달은 열 수 없다

---

### Scenario 6 (v0.6.0): 결과 뷰 모드 토글 — 테이블 ↔ 차트

- **Given** 데이터 뷰어 모달에서 사용자가 시리즈 2개와 시간 범위, 인터벌, 집계를 지정하고 실행 버튼을 눌러 매트릭스 결과가 렌더링된 상태이며, 기본 표시 모드는 "테이블" 이다
- **When** 사용자가 매트릭스 헤더의 "라인 차트" 탭(`tsdb-result-view-chart`)을 클릭
- **Then** 시스템은 다음을 수행한다:
  - 테이블 본문과 페이지네이션 컨트롤을 숨긴다
  - `TsdbResultChart` (recharts `LineChart`)를 동일 영역에 렌더링한다
  - 컬럼별로 8개 cyclic 팔레트의 고유 색상을 부여한다
  - LineChart 의 animation 은 비활성화되며 `connectNulls` 옵션은 사용되지 않는다
  - 사용자가 "테이블" 탭(`tsdb-result-view-table`)을 다시 클릭하면 매트릭스 테이블과 페이지네이션이 다시 노출된다

---

### Scenario 7 (v0.6.0): 차트 결측값 처리 4모드

- **Given** 차트 모드로 전환된 상태이며, 일부 시리즈/버킷 조합에 결측값(null)이 존재한다
- **When** 사용자가 차트 컨테이너 상단 툴바의 null 처리 셀렉트(`tsdb-chart-null-mode`)를 다음과 같이 변경:
  - `gap` 선택 → 결측값 위치에서 라인이 끊긴다 (기본)
  - `previous` 선택 → 직전 값으로 forward-fill 처리되어 라인이 수평 연장된다
  - `value` 선택 → 채움값 number input(`tsdb-chart-null-fill-value`)이 노출되고, 입력한 상수로 결측값이 채워진다
  - `interpolate` 선택 → 양쪽 알려진 값 사이만 선형 보간된다. 한쪽만 알려진 구간(예: 시리즈 끝)은 forward-fill 로 채워지며, leading null 은 그대로 유지된다
- **Then** 시스템은 다음을 수행한다:
  - 모든 모드 변경 시 차트가 즉시 갱신된다
  - `value` 가 아닌 모드에서는 채움값 number input 이 노출되지 않는다
  - 모드 변경은 백엔드 재요청 없이 클라이언트측 변환으로 처리된다

---

### Scenario 8 (v0.6.0): 태그 필터 separator 변경

- **Given** 데이터 뷰어 모달이 열려 있고, separator 입력 박스(`tag-segment-separator`)는 기본값 ':' 로 표시되어 있으며, 사용자가 자동 추출된 태그 중 일부를 체크하여 필터로 적용한 상태이다
- **When** 사용자가 separator 입력 박스의 값을 '/' 로 변경
- **Then** 시스템은 다음을 수행한다:
  - 기존 태그 필터 선택을 자동으로 초기화한다 (`tagFilter.clearAll()`)
  - 모든 키에 대해 자동 추출된 태그 키/값을 새 separator 기준으로 재계산한다
  - 정적 태그(SPEC-STORE-003)가 등록된 키도 새 separator 의 영향을 받아 자동 추출 결과가 갱신되며, 정적 태그와 자동 추출 결과가 충돌하면 정적 태그를 우선한다
  - 사용자가 입력한 separator 가 키에 매칭되지 않을 때 폴백 분리자로 추출을 시도하지 않으며, 해당 키의 자동 추출 결과는 빈 객체로 반환된다
  - separator 입력은 4자 이내로만 허용된다

---

### Scenario 9 (v0.6.0): 다시 실행 후 표시 모드 보존

- **Given** 사용자가 쿼리를 실행한 뒤 결과 뷰 모드를 "라인 차트" 로 전환하여 차트가 표시된 상태
- **When** 사용자가 폼의 일부 입력을 변경하고 "다시 실행" 버튼을 클릭하여 새 쿼리 결과가 도착하고 `<SeriesResultMatrix>` 가 unmount→remount 되었다
- **Then** 시스템은 다음을 수행한다:
  - 새 결과 데이터로 차트가 갱신되며, 표시 모드는 사용자가 마지막으로 선택한 "라인 차트" 그대로 유지된다 (테이블로 되돌아가지 않는다)
  - 영속화는 부모 컴포넌트의 `resultViewMode` state 와 controlled `viewMode` / `onViewModeChange` props 를 통해 이루어진다
  - 모달 자체를 닫았다가 다시 여는 경우에는 `resultViewMode` 가 'table' 로 리셋된다

---

### Scenario 10 (v0.6.0): 모달 레이아웃 안정성 + 시간 범위 기본 모드

- **Given** 사용자가 작은 화면(예: 800px 미만 높이)에서 데이터 뷰어 모달을 열었다
- **When** 모달이 마운트되어 초기 상태가 표시된다
- **Then** 시스템은 다음을 수행한다:
  - 시간 범위 모드는 항상 'relative' 로 표시된다 (모달 오픈 시 항상 리셋)
  - 폼 영역은 `min-h-0 shrink overflow-y-auto` 로 자체 스크롤되며, 매트릭스 영역은 최소 180px 높이를 보장받아 폼이 매트릭스/푸터를 가리지 않는다
  - 시리즈 fieldset 은 좌(검색+체크박스) / 우(태그 필터) 2열로 표시된다
  - 시간 범위 영역은 모드 탭 아래 좌(범위+인터벌) / 우(집계 함수) 2열로 표시된다
  - 사용자가 절대 모드와 상대 모드를 번갈아 전환하더라도 인터벌 컨트롤의 화면상 위치가 흔들리지 않는다 (`min-h-[7.5rem]` 사전 reserve)
  - 멀티셀렉트 영역 위에 별도의 "선택된 시리즈" pill 영역이 렌더링되지 않는다 (선택 상태는 체크 표시로만 표현)
  - 시리즈 리스트의 각 행 우측에는 자동 추출된 태그 칩이 최대 3개까지 표시된다 (`series-row-tags-{key}`)

---

## Edge Case Coverage Checklist

### 입력 검증

- [ ] 시작 시각과 종료 시각이 동일한 경우 실행 버튼 비활성
- [ ] 종료 시각이 시작 시각보다 이전인 경우 실행 버튼 비활성 + 에러 메시지
- [ ] 시리즈 키 0개 선택 시 실행 버튼 비활성
- [ ] custom 인터벌에 Go duration 문법 위반 입력(예: `5분`, `abc`) 시 에러 메시지 + 실행 비활성
- [ ] 시리즈 키가 1개만 선택된 경우에도 매트릭스 테이블이 정상 렌더링 (1개 컬럼)

### 네트워크 및 서버 응답

- [ ] 서버 4xx 응답 시 에러 배너 표시, 폼 입력 유지, 결과 테이블 갱신 안 함
- [ ] 서버 5xx 응답 시 에러 배너 표시, 재시도 버튼 또는 안내 제공
- [ ] 네트워크 타임아웃 시 에러 배너 표시, 폼 입력 유지
- [ ] 요청 중 사용자가 모달 닫으면 요청 취소(AbortController) 또는 응답 무시

### UI 상태

- [ ] 로딩 중 실행 버튼 비활성 + 진행 인디케이터 표시
- [ ] 에이전트 타입이 `tsdb`가 아닌 경우 시리즈 리스트 패널 렌더링 안 됨
- [ ] Esc 키 입력 시 모달 닫힘 + 폼 상태 폐기
- [ ] 모달 외부 클릭 시 모달 닫힘 + 폼 상태 폐기
- [ ] 모달 재오픈 시 폼 초기값으로 리셋

### 페이지네이션

- [ ] 첫 페이지에서 "이전" 버튼 비활성
- [ ] 마지막 페이지에서 "다음" 버튼 비활성
- [ ] 페이지 크기 변경 시 현재 페이지가 1로 리셋
- [ ] `size` 파라미터는 `[10, 25, 50, 100]` 외 값이 서버로 전송되지 않음

### 타임존 변환

- [ ] 브라우저 로컬 입력 → UTC epoch ms 변환 왕복(round-trip) 무손실
- [ ] DST 전환 시점 포함 범위에서도 정확한 버킷 계산
- [ ] 응답 epoch ms → 브라우저 로컬 포맷 렌더링 정확성

### 매트릭스 렌더링

- [ ] 모든 버킷-시리즈 조합에 값이 있을 때 정상 렌더링
- [ ] 일부 버킷에 값이 없을 때 해당 셀만 em-dash(`—`) 표시
- [ ] 시리즈 키에 특수 문자(`,`, `=`, 공백) 포함 시 컬럼 헤더 정상 표시
- [ ] 시간 버킷이 1개인 경우(매우 짧은 범위)에도 테이블 정상 렌더링

### 결과 뷰 모드 토글 (v0.6.0)

- [ ] 토글 탭 `tsdb-result-view-table` / `tsdb-result-view-chart` 노출 및 활성 상태 시각 구분
- [ ] 차트 모드 전환 시 매트릭스 테이블과 페이지네이션 컨트롤 미노출
- [ ] 차트 컬럼별 8개 cyclic 팔레트 색상 적용
- [ ] 차트 animation 비활성화, `connectNulls` 미사용
- [ ] 다시 실행으로 인한 unmount→remount 사이 사용자 선택 표시 모드 유지
- [ ] 모달 자체 재오픈 시 `resultViewMode` 'table' 로 리셋

### 차트 결측값 처리 (v0.6.0)

- [ ] `gap` 모드: 결측값 위치 라인 끊김 (기본)
- [ ] `previous` 모드: 직전 값으로 forward-fill, 라인 수평 연장
- [ ] `value` 모드: 채움값 number input(`tsdb-chart-null-fill-value`) 노출 및 사용자 상수로 결측 채움
- [ ] `interpolate` 모드: 양쪽 알려진 값 사이만 선형 보간, 한쪽만 알려진 구간은 forward-fill, leading null 유지
- [ ] `value` 가 아닌 모드에서는 채움값 input 미노출
- [ ] 모드 변경은 클라이언트측 변환만 수행 (백엔드 재요청 없음)

### 태그 필터 + Separator (v0.6.0)

- [ ] `TagFilterChips` 헤더 라벨이 "필터링" 으로 표시
- [ ] separator 입력 박스(`tag-segment-separator`) 4자 이내 입력 강제
- [ ] 모달 오픈 시 separator ':' 로 리셋
- [ ] separator 변경 시 기존 태그 필터 선택 자동 초기화
- [ ] 자동 추출 + 정적 태그 병합, 충돌 시 정적 태그 우선
- [ ] 정적 태그가 있는 키도 separator 변경의 영향을 받아 자동 추출 결과 갱신
- [ ] 사용자 separator 미매칭 시 빈 객체 반환 (폴백 미동작)

### 시리즈 행 태그 열 (v0.6.0)

- [ ] 각 행 우측에 자동 추출 태그 칩 최대 3개 노출 (`series-row-tags-{key}`)
- [ ] 자동 추출 태그가 0개인 키는 태그 칩 영역이 비어있되 레이아웃은 유지

### 모달 레이아웃 안정성 (v0.6.0)

- [ ] 폼 영역 `min-h-0 shrink overflow-y-auto` 적용
- [ ] 매트릭스 영역 `min-h-[180px]` 보장
- [ ] 시리즈 fieldset 좌(검색+체크박스) / 우(태그 필터) 2열
- [ ] 시간 범위 영역 모드 탭 아래 좌(범위+인터벌) / 우(집계) 2열
- [ ] 절대↔상대 모드 전환 시 인터벌 위치 흔들림 없음 (`min-h-[7.5rem]` reserve)
- [ ] 선택된 시리즈 별도 pill 영역 미노출

### 시간 범위 기본 모드 (v0.6.0)

- [ ] 모달 오픈 시 시간 범위 모드 'relative' 로 표시
- [ ] 모달 닫고 재오픈 시에도 'relative' 로 리셋

---

## Performance Criteria

### 초기 렌더링

- 시리즈 목록 100개 초기 렌더링: **500ms 이하** (TTI 기준)
- 시리즈 리스트 패널 마운트 → 첫 데이터 표시: **800ms 이하** (네트워크 제외)

### 매트릭스 렌더링

- 500행 × 5 컬럼 매트릭스 렌더링: **1초 이하**
- 1,000행 × 5 컬럼 매트릭스 렌더링: **2초 이하** (경고 없는 범위)
- 5,000행 이상은 경고 후 사용자 동의 시에만 렌더링 (성능 보장 범위 외)

### 상호작용 응답성

- 페이지 크기 변경 → 네트워크 요청 시작: **50ms 이하**
- 모달 열림 애니메이션 완료: **300ms 이하**
- 폼 입력 유효성 검사 즉시 반영: **16ms 이하 (1 frame)**

### 네트워크 효율

- 페이지 이동 시 중복 요청 방지 (TanStack Query 캐시 활용)
- 동일 쿼리 파라미터 반복 조회 시 캐시 응답 사용

---

## TRUST 5 Quality Gates

### Tested

- **백엔드**: `internal/api/handler/tsdb.go` 라인 커버리지 **85% 이상**
  - Characterization 테스트: 기존 응답 포맷 보존
  - 신규 테스트: 페이징 동작, 경계값(size=10/25/50/100/101), 빈 결과, 잘못된 파라미터
- **프론트엔드**: 신규 컴포넌트 3종 (`TsdbSeriesListPanel`, `TsdbDataViewerModal`, `TsdbResultMatrix`) 라인 커버리지 **85% 이상**
  - 상호작용 테스트: 페이지 이동, 모달 열림/닫힘, 유효성 검사
  - epoch ms 변환 유틸 왕복 테스트
- **품질 기준**: quality.yaml `hybrid_settings.min_coverage_new: 85`

### Readable

- 컴포넌트 책임 분리: List(Panel) / Form(Modal) / Display(Matrix)
- 변수/함수 명명: camelCase, 의미 명확 (예: `selectedKeys`, `toEpochMs`, `handleExecute`)
- 주석 언어: 한국어 (프로젝트 설정 `code_comments: ko` 준수)

### Unified

- 기존 대화 패턴 재사용: `ImportDialog` 참고 (포털, 포커스 트랩, Esc)
- API 훅 컨벤션: TanStack Query `useQuery`/`useMutation` 표준 패턴
- 스타일: 기존 Tailwind 클래스 + 프로젝트 커스텀 CSS 규칙
- Lint/Format: `go vet`, `golangci-lint`, `eslint`, `prettier` 모두 통과

### Secured

- 사용자 입력 검증:
  - 시간 범위(시작 < 종료)
  - Go duration 문법(정규식 검증)
  - `size` 상한(100)
- 서버 에러 메시지 표시 시 HTML 이스케이프 (React 기본 방어)
- CORS, CSRF는 기존 미들웨어 유지

### Trackable

- Conventional commits: `feat(web): TSDB 시리즈 뷰어 추가 (SPEC-WEB-005)`
- SPEC ID 참조: 모든 커밋 메시지 및 PR 설명에 `SPEC-WEB-005` 포함
- TAG BLOCK: 신규 파일 헤더 주석에 `@spec SPEC-WEB-005` 추가
- 로그: 백엔드 핸들러 경로별 요청 로그 유지

---

---

## v0.7.0 Given-When-Then 시나리오 (M11~M16)

### Scenario 11: GET /keys 객체 배열 응답 정상 처리 (M11)

**Given**
- 백엔드 SPEC-STORE-003 v0.3.0 배포 완료 (`{count, keys: [{key, registration, data_type, metric_type, tags}]}`)
- 사용자가 store 에이전트의 데이터 뷰어 모달을 연다

**When**
- frontend 가 `GET /api/v1/store/{name}/keys?namespace=default&pattern=*` 를 호출한다

**Then**
- TypeScript 컴파일 에러 없이 응답 파싱이 성공한다
- 시리즈 멀티셀렉트에 키 목록이 정상 렌더된다 (`t.includes is not a function` 크래시 발생하지 않음)
- 각 행에 `data_type`, `metric_type`, `registration` 메타데이터 칩이 표시된다 (M16)

---

### Scenario 12: registration_type 세그먼트 컨트롤 (M12)

**Given**
- 사용자가 store 에이전트 설정 폼을 연다
- 기존 yaml: `registration_type: "auto"` (또는 `manual`)

**When**
- 사용자가 폼을 검사한다

**Then**
- `allow_dynamic_keys` boolean 토글은 더 이상 존재하지 않는다
- 대신 "수동 등록 (manual)" / "자동 등록 (auto)" 두 옵션의 세그먼트 컨트롤이 표시된다
- 현재 yaml 의 값이 정확히 선택된 상태로 표시된다
- 사용자가 옵션을 변경하면 폼 변경 사항으로 표기된다 (저장 버튼 활성화)

---

### Scenario 13: data_type / metric_type 입력 + 검증 (M13)

**Given**
- 사용자가 manual 모드 store 에이전트 설정에서 `StoreKeysEditor` 를 사용한다

**When (정상 케이스)**
- 사용자가 새 행을 추가:
  - key: `indoor:1:room_temp`
  - data_type 셀렉트에서 `float` 선택
  - metric_type 입력에 `temperature` 입력
  - tag 추가: `room=1`
- 저장 버튼 클릭

**Then**
- yaml 에 `keys[]` 엔트리가 다음과 같이 추가된다:
  ```yaml
  - key: "indoor:1:room_temp"
    data_type: "float"
    metric_type: "temperature"
    tags:
      room: "1"
  ```

**When (manual 모드 data_type 누락)**
- 사용자가 새 행 추가 시 data_type 을 선택하지 않은 상태로 저장 시도

**Then**
- 인라인 에러 표시: "data_type 은 manual 모드에서 필수입니다"
- 저장이 차단된다 (백엔드 호출 전 클라이언트 검증)

**When (metric_type 정규식 위반)**
- 사용자가 metric_type 입력에 `room.temp` (점 포함) 입력 + blur

**Then**
- 인라인 에러 표시: "허용되지 않는 문자가 포함되었습니다 (영숫자, `_`, `-` 만 허용)"
- 저장 버튼이 비활성화된다

---

### Scenario 14: PromoteToStaticDialog 진화 (M14)

**Given**
- 동적으로 등록된 키 `sensor:dyn` 이 store 에이전트에 존재하고, 사용자가 저장소 탭에서 이 키 행의 "정적 등록으로 변환" 버튼을 클릭한다

**When**
- PromoteToStaticDialog 가 열린다

**Then**
- 다이얼로그에 다음 입력 필드가 표시된다:
  - key (read-only, `sensor:dyn` 표시)
  - **data_type 셀렉트 (필수, 6종 enum)** ← 신규
  - **metric_type 입력 (선택, 빈 값 시 default `unknown`, 정규식 검증)** ← 신규
  - tags 입력 (기존)

**When**
- 사용자가 data_type=`int`, metric_type=`count`, tags=`{kind: telemetry}` 입력 후 "변환" 클릭

**Then**
- 백엔드에 Configure 호출이 발생하여 yaml `keys[]` 에 해당 엔트리가 추가된다
- 저장소 탭 리스트가 갱신되어 해당 키가 `registration: "manual"` 로 표시된다

---

### Scenario 15: 백엔드 에러 사용자 친화 매핑 (M15)

**Given**
- 사용자가 store 에이전트의 키에 값을 쓰는 작업을 트리거 (UI 가 `Set` 호출)

**When (ErrTypeMismatch)**
- 백엔드가 `ErrTypeMismatch` (HTTP 400) 를 반환

**Then**
- UI 토스트 알림에 다음 메시지가 표시된다:
  > "키 `<key>` 의 등록된 타입(`<type>`)과 일치하지 않습니다"
- 토스트는 자동으로 닫히기 전까지 5초 이상 표시된다 (사용자 인지 시간 확보)

**When (allow_dynamic_keys 마이그레이션 에러)**
- 사용자가 v0.2.0 yaml 로 부팅 시도 시 백엔드가 `"'allow_dynamic_keys' is removed in v0.3.0; use 'registration_type: manual|auto' instead"` 를 반환

**Then**
- UI 다이얼로그에 다음이 표시된다:
  > "에이전트 설정 마이그레이션이 필요합니다. allow_dynamic_keys → registration_type 변경 후 재시작하세요."
- "마이그레이션 가이드 보기" 링크가 `docs/migration/v0.3.0-store-keys.md` 로 연결된다

---

### Scenario 16: 메타데이터 표시 + 시각 구분 (M16)

**Given**
- store 에이전트 설정에 다음 키가 존재:
  - `alpha` (manual, data_type=int, metric_type=count, tags={kind: a})
  - `beta` (manual, data_type=string, metric_type=unknown, tags={kind: b})
  - `gamma` (auto, data_type=boolean, metric_type=unknown, tags={})

**When**
- 사용자가 데이터 뷰어 모달을 열거나 저장소 탭의 키 리스트를 본다

**Then**
- 각 행에 5개 필드가 시각적으로 구분되어 표시된다:
  - key 이름
  - data_type 칩 (예: `[int]`, `[string]`, `[boolean]` — 색상으로 구분 가능)
  - metric_type 칩 (예: `[count]`, `[unknown]` — `unknown` 은 흐리게 표시)
  - registration 배지 (`auto` 키는 시각적 배지 — 선택적, Decision Point 1 결정)
  - tags 칩 목록 (기존 v0.6.0 스타일)
- 정렬은 key 알파벳 오름차순 (백엔드 보장 + 추가 정렬 안 함)

---

## v0.7.0 Edge Case Checklist

### 타입 진화 (M11)

- [ ] 빈 keys 응답 (`{count: 0, keys: []}`) → 빈 멀티셀렉트 + "키 없음" 안내
- [ ] 백엔드가 일시적으로 v0.2.0 응답 반환 (배포 race) → 명확한 에러 메시지 표시 (TypeScript 런타임 검증 또는 Zod 스키마 검증)
- [ ] 매우 많은 키 (1000+) → 페이지네이션 동작 + 메타데이터 칩 lazy 렌더로 성능 유지

### Config UI (M12, M13, M14)

- [ ] data_type 셀렉트에 6종 enum 모두 표시
- [ ] metric_type 빈 입력 → "unknown" 자동 적용 (placeholder 또는 default)
- [ ] metric_type 정규식 위반 (점/공백/콜론/유니코드) → 인라인 에러 + 저장 차단
- [ ] manual 모드 + data_type 미선택 → 저장 차단 + 인라인 에러
- [ ] auto 모드 + data_type 미선택 → 저장 허용 (backend 추론)
- [ ] PromoteToStaticDialog 에서 data_type 미선택 → "변환" 버튼 비활성화
- [ ] registration_type 변경 시 즉시 미리보기 (변경 후 manual/auto 의미 변화 안내)
- [ ] yaml import (ImportDialog) 에서 v0.2.0 형식 yaml 업로드 → frontend 측 검증 또는 backend 부팅 실패 시 명확한 메시지

### 에러 매핑 (M15)

- [ ] 백엔드의 4종 신규 에러 모두 사용자 친화 메시지 매핑
- [ ] 알 수 없는 에러 (네트워크 실패 등) → fallback 메시지 + 디버깅 정보 (개발 모드)
- [ ] 마이그레이션 에러 다이얼로그에서 마이그레이션 가이드 링크 정상 동작

### 메타데이터 표시 (M16)

- [ ] data_type 칩 색상이 일관되게 적용 (예: int=blue, float=green, string=gray, boolean=purple, bytes=orange, json=red)
- [ ] metric_type=unknown 인 키는 시각적으로 흐리게 (auto 등록 임을 암시)
- [ ] auto 등록 키와 manual 등록 키가 한 화면에 혼재 시 시각 구분 (배지 또는 행 색상)
- [ ] 매우 긴 metric_type (예: 50자 이상) → ellipsis 처리 + tooltip 으로 전체 표시

### 통합 / 회귀 (v0.6.0 보존)

- [ ] v0.6.0 의 시리즈 멀티셀렉트 / 모달 레이아웃 / 차트 뷰 / null 처리 모두 동일 동작
- [ ] v0.6.0 의 태그 자동 추출 + separator 설정 동작 보존
- [ ] v0.5.0 의 페이지네이션 [10, 25(기본), 50, 100] 동작 보존
- [ ] v0.4.0 의 태그 chip 필터 동작 보존 (정적 태그 메타데이터를 신규 응답 shape 에서 추출)

---

## v0.7.0 TRUST 5 품질 게이트

### T — Tested

- [ ] `web/src/services/api/store.ts` 커버리지 ≥ 90%
- [ ] `web/src/lib/errors/storeErrorMapper.ts` (신규) 커버리지 = 100%
- [ ] `web/src/components/property/StoreKeysEditor.tsx` 커버리지 ≥ 85%
- [ ] `web/src/components/property/PromoteToStaticDialog.tsx` 커버리지 ≥ 85%
- [ ] `web/src/pages/agents/TsdbDataViewerModal.tsx` 커버리지 ≥ 85%
- [ ] Vitest 전체 통과 (기존 486 + v0.7.0 신규 25-35)
- [ ] TypeScript strict 통과 (any/non-null assertion 미사용)

### R — Readable

- [ ] 신규 타입/함수에 JSDoc 주석 추가 (Korean)
- [ ] `@spec SPEC-WEB-005 v0.7.0` 추적 태그
- [ ] `prettier` / `eslint` 경고 0

### U — Unified

- [ ] 기존 컴포넌트 props 네이밍 패턴 준수 (camelCase, optional `?` 표기)
- [ ] 기존 토스트/다이얼로그 사용 패턴 준수 (Sonner 등 기존 라이브러리 재사용)
- [ ] 기존 form validation 패턴 준수 (인라인 에러 + 저장 차단)
- [ ] data_type / metric_type / registration 칩 시각 스타일 기존 태그 칩과 일관

### S — Secured

- [ ] 백엔드 응답 무결성 검증 (Zod 또는 런타임 type guard 권장)
- [ ] XSS 방지: metric_type / tags 표시 시 HTML escape (React 기본 방어)
- [ ] 정규식 검증 ReDoS 안전성 (`^[a-zA-Z0-9_-]+$` 는 안전한 패턴)
- [ ] 에러 메시지 노출 시 내부 stack trace 누출 방지

### T — Trackable

- [ ] `@spec SPEC-WEB-005 v0.7.0` 주석 신규/변경 코드 블록에 삽입
- [ ] Conventional commit 메시지 (`feat(web)!: SPEC-WEB-005 v0.7.0 — SPEC-STORE-003 v0.3.0 BREAKING 적응`)
- [ ] CHANGELOG.md 갱신 (BREAKING for frontend, but no end-user migration needed)

---

## Definition of Done

- [x] 모든 5개 기본 시나리오 + v0.6.0 시나리오 5종 (Scenario 6~10) 이 수동 또는 자동화 테스트로 통과
- [x] Edge Case Coverage Checklist 의 모든 항목 (v0.6.0 항목 포함) 확인
- [x] 백엔드 커버리지 85% 이상 (Characterization + 신규 TDD 포함)
- [x] 프론트엔드 신규/변경 컴포넌트 커버리지 85% 이상
- [x] `go test -race ./...` 전체 통과
- [x] `npm run test` (Vitest) 전체 통과 — 486/486
- [x] TypeScript strict 통과
- [x] `go vet`, `golangci-lint`, `eslint` 경고 0
- [x] CHANGELOG 에 API 변경 및 UI 추가 기록
- [ ] README에 TSDB 데이터 뷰어 사용 안내 추가 (선택)
- [x] 기존 `GET /api/v1/tsdb/series` 호출부가 수정 없이 동작함을 검증 (하위 호환성)
- [x] v0.6.0: 신규 외부 라이브러리 추가 없음 (`recharts` 는 기존 의존성)
- [x] v0.6.0: 신규 테스트 18개 (15 null handling + 3 view toggle integration + 1 controlled prop) 통과

### v0.7.0 DoD (frontend BREAKING 적응)

- [ ] Scenario 11~16 모두 자동화 테스트로 통과
- [ ] v0.7.0 Edge Case Checklist 전 항목 검증
- [ ] frontend 커버리지: store.ts 90%+, storeErrorMapper.ts 100%, StoreKeysEditor 85%+, PromoteToStaticDialog 85%+, TsdbDataViewerModal 85%+
- [ ] Vitest 전체 통과 (기존 486 + 신규 25-35 = ~511-521)
- [ ] TypeScript strict 통과 (any 사용 0)
- [ ] eslint 경고 0
- [ ] 백엔드 SPEC-STORE-003 v0.3.0 와 frontend v0.7.0 이 같은 develop/main 브랜치에 공존 시 정상 동작 (E2E smoke)
- [ ] `t.includes is not a function` 크래시 해소 확인 (AgentListPage 정상 렌더)
- [ ] StoreKeysEditor 에서 v0.3.0 yaml 형식으로 키 추가/수정/삭제 정상 동작
- [ ] PromoteToStaticDialog 에서 동적 키를 정적 등록으로 변환 정상 동작
- [ ] 백엔드 신규 4종 에러가 사용자 친화 메시지로 표시
- [ ] CHANGELOG.md 에 v0.7.0 frontend 변경 항목 기재 (BREAKING for frontend, no end-user migration)
- [ ] 신규 외부 라이브러리 추가 없음
- [ ] **선택 작업 (Decision Point 1 결정)**:
  - [ ] 신규 필터 UI (data_type/metric_type/registration 필터 칩) — 포함 시 선택 항목
  - [ ] auto/manual 시각 구분 배지 — 포함 시 선택 항목
