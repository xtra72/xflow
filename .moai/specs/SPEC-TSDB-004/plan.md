# SPEC-TSDB-004 구현 계획

## 1. 작업 분해

마일스톤은 **실측 → 순수 함수 → 운반 → 계약 → 표시 → UI** 순이다. 각 마일스톤은 독립적으로 `go build ./... && go test ./...`(또는 `npm run build && npm test`)를 통과해야 한다.

M1(v3 실측)을 맨 앞에 두는 이유는 OQ3 의 답이 M2 의 v3 분기 형태를 바꾸기 때문이다. 실측 없이 짜면 "드러난다" 가정 위에 구현이 얹히고, 틀리면 M2~M4 를 되돌려야 한다.

### M1 — v3 `GROUP BY` 부분 지정 실측 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | 로컬 `influxdb:3-core` 컨테이너에 태그 2종(`host`·`rack`) 든 measurement 주입 | `.moai/specs/SPEC-TSDB-004/m1-probe.md` |
| 1.2 | `SELECT mean("v") FROM "m" WHERE ... GROUP BY time(60000ms), "host"` 실행 후 `iteratorToMaps` 결과 관찰 | 동일 |
| 1.3 | 태그가 행 컬럼으로 드러나는지 · 컬럼 이름이 태그 키 그대로인지 확인 | 동일 |
| 1.4 | 다중 키(`, "host", "rack"`) 및 결손 태그 시리즈에서의 거동 확인 | 동일 |
| 1.5 | **OQ3 확정** — 결과를 spec.md HISTORY 에 기록 | `spec.md` v0.2.0 |

[SPEC-TSDB-003](../SPEC-TSDB-003/spec.md) 의 `m1-probe.md` 와 같은 형식을 쓴다. 프로브 절차와 관측값을 그대로 남겨 후속 세션이 재현할 수 있어야 한다.

**결과(v0.3.0)**: 드러난다. 대체 경로는 폐기했다. 덧붙여 M3 의 `normalizeSeriesBuckets` 가 방언 중립으로 작성되어 v2/v3 분기 자체가 불필요함이 확인되었다 — OQ3 는 설계 결정이 아니라 검증 항목이었다.

**검증**: 프로브 문서에 실행한 쿼리와 관측 출력이 그대로 남아 있을 것.

### M2 — 쿼리 생성 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | `SeriesQuerySpec.GroupBy []string` 추가 + 키 오름차순 정렬 | `internal/agent/system/influxdb_seriesquery.go` |
| 2.2 | `ValidateIdentifiers` 가 `GroupBy` 키를 `Tags` 키와 동일하게 검증 | 동일 |
| 2.3 | `BuildFluxSeriesQuery` — `group(columns:)` 를 `aggregateWindow` 앞에, `keep` 목록에 그룹 키 추가 | 동일 |
| 2.4 | `BuildInfluxQLSeriesQuery` — `GROUP BY time(d), "t1", "t2"` (time 첫 자리 유지) | 동일 |
| 2.5 | **`GroupBy` 빈 경우 생성 결과 바이트 무변경** 특성화 테스트 | `influxdb_seriesquery_test.go` |

순수 함수만 다루므로 네트워크가 필요 없다. 기존 테스트 파일의 표 기반 테스트 형식을 따른다.

**검증**: `go test ./internal/agent/system/ -run Series`. AC-04 · AC-05 · AC-06 통과.

### M3 — 결과 운반 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `SeriesBucket.Tags map[string]string` 추가 (`GroupBy` 빈 경우 `nil`) | `internal/agent/system/influxdb_agent.go` |
| 3.2 | v2 결과 정규화 — Flux 행에서 그룹 키 컬럼을 읽어 `Tags` 채움 | 동일 |
| 3.3 | v3 결과 정규화 — M1 확정 경로에 따라 `Tags` 채움 | 동일 |
| 3.4 | 그룹 태그 값 사전순 안정 정렬 | 동일 |
| 3.5 | httptest 왕복 테스트(v2) — 그룹 2개가 각각 자기 태그를 달고 오는지 | `influxdb_agent_test.go` |

v2 는 httptest 왕복이 가능하다(`influxdb_schema_test.go` 의 `TestInfluxSchema_V2_어댑터_왕복_D2D3D4` 가 선례). v3 는 Arrow Flight(gRPC)라 httptest 로 대체할 수 없으므로 결과 정규화 함수를 순수 함수로 분리해 테스트한다.

**검증**: `go test ./internal/agent/system/...`. AC-07 · AC-17 통과.

### M4 — API 계약 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 요청 DTO 에 `group_by []string` 추가 | `internal/api/dto/influxdb.go` |
| 4.2 | `buildSeriesQuerySpec` 가 `group_by` 를 스펙으로 옮김 | `internal/api/handler/influxdb_series.go` |
| 4.3 | `tags` ∩ `group_by` ≠ ∅ 이면 400 (UB1-3) | 동일 |
| 4.4 | `buildInfluxSeriesEntries` — `labels = {__field__} ∪ req.Tags ∪ bucket.Tags` | 동일 |
| 4.5 | **상한 없음 확인** — group by 를 이유로 자르는 경로가 없고 `truncated` 가 그 때문에 켜지지 않음을 테스트로 고정 | 동일 |
| 4.6 | 라우트 권한 커버리지 무변경 확인 | `route_permission_coverage_test.go` |

**검증**: `go test ./internal/api/...`. AC-09 ~ AC-12 · AC-14 통과.

### M4b — 시리즈축 페이지네이션 (Priority High)

> v0.4.0 에서 추가. 페이지네이션을 질의 계층으로 내리는 결정에 따른 백엔드 작업이다.

| # | 작업 | 산출물 |
|---|------|--------|
| 4b.1 | `SeriesQuerySpec.GroupFilter []map[string]string` + 조합 정렬 · 빈 조합 거부 | `internal/agent/system/influxdb_seriesquery.go` |
| 4b.2 | Flux 페이지 술어 — 조합 내 `and`, 조합 간 `or`, `group()` 앞에 배치 | 동일 |
| 4b.3 | InfluxQL 페이지 술어 — `AND ((..) OR (..))` | 동일 |
| 4b.4 | DTO `group_filter` + 핸들러 전달 + 400 매핑 | `internal/api/dto/influxdb.go`, `influxdb_series.go` |
| 4b.5 | 버킷 경계 불변 테스트 — 페이지 간 윈도우 파라미터 동일 | `influxdb_seriesquery_test.go` |

**네이티브 기전은 없다** — v3 `SLIMIT`/`SOFFSET` 는 `HTTP 405` 미구현, Flux 에 테이블 개수 제한 없음(M1 실측). 2단계 절차(D5 열거 → 페이지 슬라이스 → `group_filter` 질의)가 유일한 경로다.

**검증**: `go test ./internal/agent/system/... ./internal/api/...`. AC-11 · AC-12 통과.

### M5 — 표시 메타데이터 귀속 (Priority High, 회귀 위험 최상)

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | `matrixToEntries` 의 위치 정렬(`aligned`)을 **라벨 기반 귀속**으로 교체 | `web/src/pages/dashboard/panels/charts/useStoreChartData.ts` |
| 5.2 | 정확 일치 항목의 귀속 결과가 현행과 동일함을 고정하는 특성화 테스트 | `useStoreChartData.test.tsx` |
| 5.3 | **Store 경로 렌더 결과 무변경** 회귀 테스트 | 동일 |
| 5.4 | group by 파생 시리즈의 색·별칭 배정 (OQ1 잠정안) | 동일 |

**이 마일스톤이 본 SPEC 의 최대 회귀 위험이다.** `matrixToEntries` 는 Store 와 TSDB 가 공유하며, 잘못 건드리면 기존 패널 전량의 표시 이름·색이 흔들린다. 5.2·5.3 을 5.4 보다 **먼저** 세워 안전망을 만든 뒤 동작을 바꾼다.

**검증**: `npm test -- useStoreChartData useTsdbChartData`. AC-15 · AC-16 통과.

### M6 — 설정 UI (Priority Medium)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | `TsdbSeriesRef.group_by?: string[]` 타입 추가 | `web/src/pages/dashboard/panels/charts/chartChannelTypes.ts` |
| 6.2 | `fetchTsdbSeries` 요청 본문에 `group_by` 전달 | `web/src/services/api/tsdbSource.ts` |
| 6.3 | 그룹 키 다중 선택 UI (태그 키 디스커버리 D2 재사용, 자유 입력 허용) | `web/src/pages/dashboard/TsdbSourceSection.tsx` |
| 6.4 | group by 항목 시각 구분 + 마지막 질의 그룹 수 표시 (S1) | 동일 |
| 6.5 | D5 열거 클라이언트 + 그룹 후보 투영·중복제거·정렬 + 페이지 슬라이스 → `group_filter` 전달 | `web/src/services/api/` 신규 |
| 6.6 | 페이지 이동 UI + 전체 그룹 수 표시 + 열거 절단 시 좁히는 방법 안내 (U7) | `TsdbSourceSection.tsx` |
| 6.7 | i18n ko/en 대칭 | `web/src/lib/i18n/` |

**검증**: `npm test -- TsdbSourceSection`. AC-01 · AC-02 · AC-18 · AC-19 통과.

---

## 2. 기술 스택

- Go 1.x — 신규 의존성 없음
- InfluxDB v2 Flux · v3 InfluxQL — 기존 클라이언트 그대로
- React 19 + TypeScript — 신규 의존성 없음

**새 외부 의존성 없음.**

---

## 3. 위험 분석

| 위험 | 영향 | 완화 |
|------|------|------|
| `matrixToEntries` 변경이 Store 패널을 깨뜨림 | 기존 대시보드 전량 표시 이름·색 붕괴 | M5.2·M5.3 안전망을 동작 변경 **전에** 세움 + AC-16 |
| v3 `GROUP BY` 부분 지정에서 태그가 안 드러남 | v3 group by 불가 | M1 실측을 맨 앞에 배치 + 대체 경로(`GROUP BY *` 후 접기) 준비 |
| 그룹 순서 변동으로 폴링마다 색이 바뀜 | 사용자가 라인을 추적 못함 | 태그 값 사전순 안정 정렬(M3.4) + AC-17 |
| 고카디널리티 태그로 수천 시리즈 렌더 | 질의 부하 · 페이로드 · 브라우저 정지 | **수용된 잔여 위험**(spec.md §2.7). 완화는 그룹 수 표시 + 좁히기 안내 + 열거 표면 페이지네이션(M6.4·M6.5)뿐이며 질의 부하는 줄지 않는다 |
| `GROUP BY` 에 태그를 더해 버킷 경계가 바뀜 | 소스 간 시각 불일치 | `time(d)` 첫 자리 유지 + AC-08 이 기존 교차검증 테스트를 group by 축으로 확장 |
| `group_by` 빈 경로가 미묘하게 달라짐 | 저장된 config 렌더 회귀 | 바이트 단위 무변경 특성화 테스트(M2.5) + AC-06 · AC-13 |

---

## 4. 구현 순서

M1 → M2 → M3 → M4 → **M4b** → M5 → M6

- **M1 이 맨 앞인 이유**: OQ3 의 답이 M3.3 의 형태를 결정한다. 가정 위에 구현을 얹으면 되돌림 비용이 M2~M4 전체에 걸린다.
- **M4(백엔드 완결)가 M5 이전인 이유**: 프론트가 소비할 계약이 확정되어야 한다. M4 완료 시점에 API 만으로 group by 가 동작해야 하며, 프론트가 없어도 `curl` 로 검증 가능해야 한다.
- **M5 가 M6 이전인 이유**: M6 의 UI 는 M5 가 정한 색·별칭 규칙을 화면에 설명해야 한다. 순서를 뒤집으면 UI 문구를 두 번 쓴다.

---

## 5. 검증 방법

각 마일스톤 완료 시:

```bash
go build ./... && go test ./internal/agent/system/... ./internal/api/...
go vet ./...
```

프론트엔드 마일스톤(M5~M6):

```bash
cd web && npm run build && npm test && npx tsc --noEmit && npm run lint
```

전체 완료 시 acceptance.md 의 AC-01 ~ AC-19 를 순서대로 검증한다.

---

## 6. 완료 정의 (Definition of Done)

- [x] spec.md 의 U1~U9 · E1 · S1 · UB1 전부 구현 — M1~M6c 완료
- [ ] acceptance.md 의 AC-01 ~ AC-19 전부 통과
- [x] OQ1 · OQ2 처분이 spec.md HISTORY 에 기록 (v0.2.0)
- [x] OQ3 처분이 spec.md HISTORY 에 기록 (v0.3.0, m1-probe.md)
- [x] `group_by` 없는 경로의 쿼리 문자열 · 응답 · 렌더 결과 무변경 — 바이트 무변경 테스트(v2·v3) + 라벨 무변경 + 패널 바 미표시
- [x] Store 경로 렌더 결과 무변경 (AC-16) — 골든 안전망을 동작 변경 전에 세워 확인
- [ ] 신규·수정 파일 커버리지 85% 이상 — **프론트와 Go 양쪽 모두 측정** (SPEC-TSDB-002 M7.2 가 프론트만 측정해 Go 미달을 놓친 선례)
- [x] `go build` · `go vet` · `go test ./...` · `tsc --noEmit` · eslint · `npm test` 전부 통과 — 6종 exit 0
- [x] i18n ko/en 대칭 — 각 3533, 비대칭 0
