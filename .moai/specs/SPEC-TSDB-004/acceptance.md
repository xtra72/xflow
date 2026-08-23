# SPEC-TSDB-004 인수 조건

공통 픽스처 — measurement `cpu`, field `usage`, 태그 2종:

| host | rack | region |
|------|------|--------|
| a | r1 | kr |
| b | r1 | kr |
| c | r2 | jp |

`$API` = `/api/v1`, 질의 라우트 = `POST $API/influxdb/{agent}/series/query`.

---

## AC-01 — 항목별 모드: 정확 일치 유지

**Given** `TsdbSeriesRef` 에 `group_by` 가 없거나 빈 배열이고
**When** 패널을 렌더하면
**Then** 시리즈가 **1개** 생기고 별칭·색·선스타일이 항목 값 그대로 적용된다(현행과 동일).

Vitest: `TsdbSourceSection.test.tsx` · `useTsdbChartData.test.tsx`

## AC-02 — 항목별 모드: group by 로 펼침

**Given** `{ key:'cpu', field:'usage', tags:{region:'kr'}, group_by:['host'] }` 이고
**When** 패널을 렌더하면
**Then** 시리즈가 **2개**(`host=a`, `host=b`) 생긴다 — `region=jp` 인 `c` 는 사전 필터에 걸려 제외된다.
**And** 같은 패널에 있는 정확 일치 항목은 영향을 받지 않는다.

## AC-03 — 질의 스펙 축과 키 정렬

**Given** `GroupBy: []string{"rack", "host"}` 인 `SeriesQuerySpec` 이 있고
**When** 쿼리를 생성하면
**Then** 생성된 문자열에서 그룹 키가 **오름차순**(`host`, `rack`)으로 나타난다.
**And** 슬라이스 순서를 바꿔 넣어도 생성 결과가 동일하다(결정성).

Go 테스트: `internal/agent/system/influxdb_seriesquery_test.go`

## AC-04 — v2 Flux 생성

**Given** `GroupBy: ["host"]` 이고
**When** `BuildFluxSeriesQuery` 를 호출하면
**Then** 생성 문자열이 아래를 **모두** 만족한다.

| 조건 | 확인 |
|------|------|
| `group(columns: ["host"])` 를 포함 | 포함 |
| `group(...)` 이 `aggregateWindow(...)` **앞**에 위치 | 인덱스 비교 |
| `keep(columns: ...)` 목록에 `"host"` 포함 | 포함 |
| `keep` 목록에 `"_time"` · `"_value"` 유지 | 포함 |

## AC-05 — v3 InfluxQL 생성

**Given** `GroupBy: ["host","rack"]` 이고
**When** `BuildInfluxQLSeriesQuery` 를 호출하면
**Then** `GROUP BY` 절이 `GROUP BY time(<d>), "host", "rack" FILL(<f>)` 형태다.
**And** `time(<d>)` 가 **첫 자리**다.
**And** `time()` 에 `offset` 인자가 없다(버킷 경계 계약 유지).

## AC-06 — `group_by` 빈 경우 생성 결과 바이트 무변경

**Given** `GroupBy` 가 `nil` 또는 빈 슬라이스이고
**When** v2·v3 쿼리를 각각 생성하면
**Then** 생성 문자열이 본 SPEC 이전 구현의 출력과 **바이트 단위로 동일**하다.

특성화 테스트로 고정한다 — 기대값을 문자열 리터럴로 박아 둔다.

## AC-07 — 버킷이 자기 그룹의 태그를 운반한다

**Given** `GroupBy: ["host"]` 로 질의하고 백엔드가 `host=a`·`host=b` 두 그룹을 돌려줄 때
**When** `QuerySeriesBuckets` 가 반환하면
**Then** 각 `SeriesBucket.Tags` 가 자기 그룹의 값(`{"host":"a"}` 또는 `{"host":"b"}`)을 담는다.

**Given** `GroupBy` 가 비어 있을 때
**Then** 모든 `SeriesBucket.Tags` 가 `nil` 이다.

Go 테스트: httptest 왕복(v2) + 결과 정규화 순수 함수(v3)

## AC-08 — 버킷 경계가 group by 로 바뀌지 않는다

**Given** 같은 인터벌(divisor `60000ms` · non-divisor `420000ms` 양쪽)에 대해
**When** `group_by` 유/무 두 쿼리를 각각 생성하면
**Then** 두 쿼리에서 읽어낸 윈도우 파라미터(폭 · offset · 레이블 위치)가 **동일**하다.
**And** Store 가 산출하는 버킷 시작과도 일치한다.

기존 `internal/api/handler/bucket_alignment_crosscheck_test.go` 를 group by 축으로 확장한다.

## AC-09 — 라벨은 결과에서 만들어진다

**Given** `tags: {"region":"kr"}` · `group_by: ["host"]` 로 질의하고
**When** 응답 엔트리를 보면
**Then** `host=a` 그룹 엔트리의 `labels` 가 `{"__field__":"usage", "region":"kr", "host":"a"}` 다.
**And** `host=b` 그룹은 `host` 값만 다르다.
**And** `__field__` 가 **모든 그룹에서 동일**하다.

## AC-10 — 그룹 실제 값이 요청 값을 덮는다

**Given** 요청 `tags` 에 `host` 가 없고 `group_by: ["host"]` 일 때
**Then** 각 엔트리의 `labels.host` 는 그 그룹의 실제 값이다(요청에 없던 키가 응답에 나타난다).

## AC-11 — 그룹 수에 상한이 없다

**Given** 그룹 키의 카디널리티가 큰 질의(예: 값 200종)이고
**When** 응답을 받으면
**Then** **200개 그룹이 전부** 반환된다 — 잘리지 않는다.
**And** `truncated` 가 group by 를 이유로 `true` 가 되지 않는다.

**Given** 질의 API 응답을 보면
**Then** 페이지네이션 파라미터(`page` · `cursor` · `offset` 류)가 요청·응답 어디에도 없다.

> 상한을 두지 않는 이유는 spec.md §4.4 다 — `group_by` 는 "지정한 축의 전부" 를 요구하는 질의이며, 일부만 돌려주면 차트가 부분 집합을 완전한 그림처럼 그린다.

## AC-12 — 열거 표면이 페이지네이션한다

**Given** group by 결과가 표시 한 페이지에 담기지 않을 만큼 많고
**When** 설정 UI 의 그룹 목록을 보면
**Then** 목록이 페이지네이션되고 **전체 그룹 수**가 함께 표시된다.
**And** 차트 범례가 페이지네이션 또는 스크롤로 전량에 접근 가능하다.
**And** 어느 표면에서도 "일부만 존재한다" 는 인상을 주지 않는다.

## AC-13 — 저장된 config 렌더 불변

**Given** `group_by` 필드가 없는 기존 TSDB config 가 있고
**When** 패널을 렌더하면
**Then** 시리즈 수 · 표시 이름 · 색 · 선스타일이 본 SPEC 이전과 동일하다.

## AC-14 — `tags` ∩ `group_by` 거부

**Given** `tags: {"host":"a"}` 와 `group_by: ["host"]` 를 동시에 보내면
**Then** `400` 이 반환되고 질의가 수행되지 않는다.
**And** 응답 메시지가 충돌한 키 이름을 알려 준다.

## AC-15 — 메타데이터 귀속: 혼재 패널

**Given** 한 패널에 정확 일치 항목 1개와 group by 항목 1개(그룹 3개 산출)가 있어 **컬럼 4개 · config 항목 2개**일 때
**When** 렌더하면
**Then** 정확 일치 항목에서 나온 컬럼이 자기 항목의 `alias` · `color` · `stroke_style` · `stroke_width` · `smooth` 를 **그대로** 갖는다.

이것이 UB1-7 의 핵심 단언이다 — 현행 `aligned = columns.length === series.length` 로는 이 시나리오에서 **전 컬럼이 메타데이터를 잃는다**.

## AC-16 — Store 경로 렌더 결과 무변경

**Given** 기존 Store 소스 패널 config 가 있고
**When** `matrixToEntries` 변경 후 렌더하면
**Then** 시리즈 이름 · 색 · 선스타일 · `booleanSeries` 판정이 변경 전과 동일하다.

`matrixToEntries` 는 Store 와 TSDB 가 공유하므로 이 AC 가 §4.1 변경의 안전망이다.

Vitest: `useStoreChartData.test.tsx`

## AC-17 — 그룹 순서 안정성

**Given** group by 질의를 연속 3회 폴링하고
**When** 매번 시리즈 목록을 보면
**Then** 그룹 순서가 **태그 값 사전순으로 동일**하다.
**And** 각 그룹에 배정된 색이 폴링 간 변하지 않는다.

## AC-18 — 그룹 키 변경이 재질의를 촉발한다

**When** 설정 UI 에서 그룹 키를 `['host']` → `['host','rack']` 로 바꾸면
**Then** 해당 항목의 질의가 다시 수행되고 시리즈 집합이 교체된다.
**And** 같은 패널의 다른 항목은 재질의되지 않는다.

## AC-19 — group by 항목 상태 표시

**Given** 시리즈 항목이 group by 모드이고
**When** 설정 UI 를 보면
**Then** 그 항목이 여러 시리즈로 펼쳐진다는 사실이 표시된다.
**And** 마지막 질의에서 나온 **그룹 수**가 표시된다.
**And** 정확 일치 항목과 시각적으로 구분된다.

---

## 품질 게이트

| 항목 | 기준 |
|------|------|
| 빌드 | `go build ./...` 성공, `cd web && npm run build` 성공 |
| 테스트 | `go test ./...` 전체 통과, `cd web && npm test` 전체 통과 |
| 정적 분석 | `go vet ./...` 무출력, `npx tsc --noEmit` 오류 0, `npm run lint` exit 0 |
| 커버리지 | 신규·수정 파일 85% 이상 — **프론트와 Go 양쪽 모두 측정** |
| 회귀 | `group_by` 없는 경로 쿼리 문자열 바이트 무변경(AC-06) + Store 렌더 무변경(AC-16) |
| 무절단 | group by 를 이유로 결과가 잘리지 않음(AC-11) |
| 버킷 정렬 | group by 유/무가 같은 경계를 산출(AC-08) |
| i18n | ko/en 키 대칭 |

## 엣지 케이스

| 상황 | 기대 동작 |
|------|-----------|
| `group_by` 에 존재하지 않는 태그 키 | 그룹 0개 또는 빈 태그 그룹 1개. **500 아님** |
| 일부 시리즈에만 그룹 태그가 있음(태그 결손) | 결손 시리즈는 그 키가 빈 값인 별도 그룹. 조용히 버리지 않는다 |
| `group_by` 키가 1개인데 값이 1종뿐 | 그룹 1개. 정확 일치와 결과가 같되 경로는 group by |
| `group_by` 에 같은 키가 중복 | 중복 제거 후 처리(400 아님) |
| 그룹 태그 값에 구분자·따옴표 포함 | 이스케이프되어 쿼리·라벨 양쪽에서 온전히 보존 |
| group by 항목이 0개 그룹을 반환 | 그 항목만 빈 시리즈. 형제 항목은 영향 없음 |
| `group_by` 지정 + `field` 누락 | 400 (기존 field 필수 규칙 승계) |
| 그룹 수가 표시 한 페이지를 넘김 | 페이지네이션. 질의 결과는 전량 유지 |
