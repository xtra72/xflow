# SPEC-TSDB-003 구현 계획 (v0.1.0)

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

---

## 0. 전략 요약

### 이 SPEC 의 하중 지지점 하나

**백엔드 열거 능력이 없으면 UI 는 아무것도 할 수 없다.** [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 는 계약 축과 백엔드 축이 파일이 겹치지 않아 **병렬 진행이 가능**했으나, 본 SPEC 은 그렇지 않다 — 선택 UI 가 소비할 데이터가 백엔드에만 있다.

따라서 마일스톤은 **엄격한 직렬**이며, 순서는 다음 한 문장으로 요약된다.

> 능력을 실측한다 → v2 로 만든다 → v3 로 만든다 → HTTP 로 낸다 → 프런트가 부른다 → UI 를 재배치한다.

### 시작하기 전에 반드시 닫아야 하는 것

**§7 의 OQ2 가 M3 을 막는다.** v3 의 기본 경로가 SQL `DISTINCT`(3쿼리)인지 InfluxQL `GROUP BY *`(1쿼리)인지 결정되지 않으면 M3 의 산출물 형상이 정해지지 않는다. 그래서 M1 이 **코드가 아니라 실측**을 산출물로 갖는다.

나머지 OQ 는 M1 종료 시점에 사용자와 함께 닫는다(§6 DoD).

### 사용자에게 보이는 첫 변화 지점

M1~M5 는 사용자에게 보이는 변화가 **0**이다. 첫 가치는 M6 에서 나온다. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §4.1 이 세운 정직성 규율을 그대로 따른다 — 중간 마일스톤을 "사용자 가치"로 정당화하지 않는다.

### 핵심 위험 하나 — 실측 없는 v3 설계

이 SPEC 에서 가장 값비싼 실패는 "v3 경로를 문서만 보고 설계했는데 실제로 동작하지 않음"이다. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §HISTORY-0.4.0 이 기록한 v3 measurements 501 오판이 정확히 그 형태였다(구현 회차에 가서야 `SHOW MEASUREMENTS` 가 동작함을 발견).

M1 은 그 실패를 **가장 싼 시점으로 앞당기기 위해** 존재한다.

---

## 1. 마일스톤 분해

### M1 — v3 열거 능력 실측 스파이크 (Priority High, 조사)

**산출물은 코드가 아니라 기록이다.** 제품 코드를 커밋하지 않는다.

| 항목 | 방법 |
|------|------|
| M1.1 | 실측 대상 v3 인스턴스를 확보한다(개발 환경 InfluxDB 3 에이전트 또는 컨테이너). 없으면 **사용자 결정을 요청하고 M1 을 중단한다** — 실측 없이 M3 을 시작하지 않는다 |
| M1.2 | `SHOW SERIES FROM "<m>"` 을 실행해 실패를 **관측한다.** 문서(§1.2.4)와 실제가 일치함을 확인한다. 오류 문구를 기록한다 |
| M1.3 | `SHOW TAG KEYS FROM "<m>"` · `SHOW FIELD KEYS FROM "<m>"` 이 동작함을 확인한다(기존 D2 · D4 경로이므로 회귀 확인이기도 하다) |
| M1.4 | SQL `SELECT DISTINCT "<tk>" FROM "<m>" WHERE time >= '...' AND time < '...' LIMIT 10` 을 `querySQL`(`influxdb_v3.go:90`) 경로로 실행해 **행 형상**을 기록한다 |
| M1.5 | InfluxQL `SELECT <f> FROM "<m>" WHERE time >= ... GROUP BY * LIMIT 1` 을 `queryInfluxQL`(`influxdb_v3.go:104`) 경로로 실행하고, `iteratorToMaps`(`:118`) 를 통과한 행에 **태그가 컬럼으로 나타나는지** 기록한다 — 이것이 OQ2 의 답이다 |
| M1.6 | 태그 키가 0개인 measurement 에서 두 경로의 동작을 확인한다(§2.5 의 예외 분기) |
| M1.7 | (선택) 카디널리티가 큰 measurement 에서 열거 응답 시간을 기록해 OQ4 의 상한 값을 보정한다 |

**종료 조건**: OQ2 가 닫히고, OQ3 · OQ4 에 대한 실측 근거가 확보된다.

**종료 시 사용자 확인**: OQ1 ~ OQ10 전부를 확정한다. 확정 결과는 spec.md 의 HISTORY 에 기록한다.

---

### M2 — v2 Flux 열거: 순수 생성 함수 + 클라이언트 메서드 (Priority High, TDD)

| 항목 | 내용 |
|------|------|
| M2.1 | `internal/agent/system/influxdb_seriesenum.go` 신규. `SeriesEnumSpec` 타입(measurement · bucket · 창 · 사전 필터 · 행 상한)과 `EnumeratedSeries{Tags, Fields}` 타입 |
| M2.2 | `BuildFluxSeriesEnumQuery(spec) (string, error)` — §2.4 의 형상. `validateSeriesIdentifier` · `escapeFluxStringLiteral` 재사용(§2.3) |
| M2.3 | 행 → 태그 집합 접기 함수 `foldEnumRows(rows []map[string]any) []EnumeratedSeries` — `_` 접두 컬럼 제외(`filterInternalTagKeys` 규칙 재사용), `_field` 추출, 태그 직렬화 오름차순 정렬 |
| M2.4 | `influxV2Client.EnumerateSeries(ctx, spec)` — `queryFlux` 재사용 |
| M2.5 | `InfluxSeriesEnumerator` 인터페이스 + `InfluxDBAgent` 위임 메서드. 기존 `InfluxSchemaDiscoverer`(`influxdb_schema.go:41`) 를 **넓히지 않고 별도 인터페이스로 둔다** — 이유는 §4 위험표 R5 |
| M2.6 | `measurementFieldKeys` · `measurementTagValues` 의 `start` 기본값 문서 재확인(OQ5) |

**테스트**: 쿼리 생성은 순수 함수이므로 네트워크 없이 전수. 조합 축 = (사전 필터 0/1/N) × (bucket 지정/미지정) × (창 지정/기본). 접기 함수는 fixture 행으로 검증.

---

### M3 — v3 열거 (Priority High, TDD) — **M1 확정 의존**

| 항목 | 내용 |
|------|------|
| M3.1 | M1 에서 확정된 경로의 순수 생성 함수. SQL 경로면 `BuildSQLSeriesEnumQuery`, InfluxQL 경로면 `BuildInfluxQLSeriesEnumQuery`. `escapeInfluxQLIdent` 재사용 |
| M3.2 | `influxV3Client.EnumerateSeries(ctx, spec)` — 확정 경로에 따라 `querySQL` 또는 `queryInfluxQL` 사용 |
| M3.3 | (SQL 경로일 때) 태그 키 조회 → DISTINCT → 필드 키 조회 3단 오케스트레이션. `field_exact = false` 고정 |
| M3.4 | 태그 키 0개 분기 — `[{tags: {}, fields: [...]}]` 단일 항목 |
| M3.5 | **`SHOW SERIES` 가 생성 문자열에 등장하지 않음을 기계 검증**(UB1-1) |

**테스트**: 모의 클라이언트로 3단 호출 순서 · 인자 · 접기 결과 검증. 문자열 생성은 순수 함수 전수.

---

### M4 — HTTP 라우트 D5 + 상한 · 절단 (Priority High, TDD)

| 항목 | 내용 |
|------|------|
| M4.1 | `internal/api/dto/influxdb.go` 확장 — `InfluxSeriesEnumResponse{Series, FieldExact, Count, Truncated, Window}` |
| M4.2 | `InfluxDBManagementHandler.ListSeries` + `g.GETPerm("/influxdb/{agent_name}/series", "store.read", ...)`(OQ1 확정에 따름) |
| M4.3 | 질의 파라미터 파싱 — `measurement`(필수) · `bucket` · `start_ms` · `end_ms` · `tags`(`k=v,k=v`) · `limit` |
| M4.4 | 창 기본값 적용(`now-30d` ~ `now`)과 응답 `window` 반영(§2.8) |
| M4.5 | 상한 강제 — 태그 집합 1,000 · 원시 행 20,000. `truncated` 신호(§2.7) |
| M4.6 | 오류 매핑 — 기존 `mapInfluxDiscoveryError`(`influxdb_management.go:345`) 재사용 |
| M4.7 | `cmd/xflowd/main.go` 배선 확인 — `InfluxDBManagementHandler` 는 이미 등록되어 있으므로 **신규 배선 없음**. 확인만 한다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §1.2.1 이 기록한 "미등록 핸들러" 함정을 반복하지 않기 위해 명시적으로 확인한다) |

---

### M5 — 프런트 클라이언트 + 능력 표 확장 (Priority Medium, TDD)

| 항목 | 내용 |
|------|------|
| M5.1 | `web/src/services/api/influxdbManagement.ts` 에 `fetchInfluxSeries(agentName, measurement, opts)` 추가. 기존 `pickStringList`(`:140`) 대신 객체 응답 파서 |
| M5.2 | `panelDataSource.ts` 의 `TsdbBackendCapabilities` 에 `seriesEnumeration` · `seriesFieldExact` 추가, `TSDB_BACKEND_CAPABILITIES` 두 항목 갱신(§2.6) |
| M5.3 | `CAPABILITY_REASON_KEYS` 에 `seriesFieldApprox` 키 추가 |
| M5.4 | `lib/i18n/{ko,en}.json` 신규 키 — 탐색 창 라벨 · 프리셋 · 절단 배너 · 근사 field 안내 · 목록에 없음 · 열거 오류 |

---

### M6 — 선택 UI 재배치 (Priority High, TDD + DDD)

**이 SPEC 의 유일한 사용자 가시 변경이다.**

| 항목 | 내용 |
|------|------|
| M6.1 | **특성화 먼저.** 현재 `TsdbSourceSection` 의 동작을 잠근다 — 에이전트 선택 시 series 초기화(`:376`), 48 상한(`:286-288`), 목록에 없는 에이전트/버킷 유지(`:400-408` · `:455-458`), 캐시 없음 |
| M6.2 | 탐색 창 컨트롤 추가(로컬 상태 — OQ6 확정에 따름). 프리셋 + 사용된 창 표시 |
| M6.3 | `useDiscoveryList` 로 열거 호출. measurement · 태그 사전 필터 · 탐색 창을 의존성으로(§2.11 · §2.12) |
| M6.4 | `rows` 생성기 교체 — `fieldKeys.items.map(...)`(`:268-278`) 를 열거 결과 전개로(§2.11). **행 ID 는 `storeSeriesId` 유지** |
| M6.5 | field 드릴다운의 클라이언트 직접 호출 제거(`fieldKeys` useDiscoveryList). 태그 드릴다운은 사전 필터로 역할 변경(§2.15) |
| M6.6 | 5상태 표시(§2.13) + 절단 배너 + 근사 field 안내(§2.14) |
| M6.7 | 저장된 시리즈가 열거에 없을 때 "목록에 없음" 유지(§2.10 · §2.18-2) |

---

### M7 — 마무리 (Priority Low)

| 항목 | 내용 |
|------|------|
| M7.1 | 정적 검사 — `go build ./...` · `go vet ./...` · `npx tsc --noEmit` · `npx eslint src --max-warnings 0` |
| M7.2 | 신규 파일 커버리지 85% 확인. **`web/vite.config.ts` 의 coverage `include` 에 신규 파일이 들어오는지 먼저 확인한다** — [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §HISTORY-0.5.0 (3) 이 기록한 "측정되지 않은 채 참으로 보이는" 함정을 반복하지 않는다 |
| M7.3 | 전체 회귀 — `go test ./...` · `npx vitest run` |
| M7.4 | spec.md HISTORY 에 구현 회차 사실 기록(이탈 · 정정 포함) |

---

## 2. 마일스톤 의존 순서

```
M1 (실측 · OQ 확정)
 │
 ├──> M2 (v2 열거)  ──┐
 │                     ├──> M4 (HTTP D5) ──> M5 (프런트 클라이언트 · 능력 표) ──> M6 (UI) ──> M7
 └──> M3 (v3 열거)  ──┘
```

| 간선 | 성질 | 사유 |
|------|------|------|
| M1 → M3 | **경성(hard)** | OQ2 가 M3 의 산출물 형상을 정한다 |
| M1 → M2 | 연성 | v2 경로는 문서·트리 근거가 충분해 M1 없이도 착수 가능하나, OQ5·OQ10 확정을 기다리는 편이 재작업이 적다 |
| M2 ∥ M3 | 병렬 가능 | 파일이 다르고 인터페이스만 공유한다 |
| M2·M3 → M4 | 경성 | 핸들러가 두 구현을 모두 호출한다 |
| M4 → M5 | 경성 | 응답 형상이 클라이언트 파서를 정한다 |
| M5 → M6 | 경성 | UI 가 클라이언트와 능력 표를 소비한다 |

**M6 을 M4 이전에 시작하지 않는다.** 모의 응답으로 UI 를 먼저 만들면 실제 응답과 어긋난 지점이 M6 종료 후에 드러나고, 그때는 UI 변경과 백엔드 변경이 한 회차에 섞여 이등분 탐색이 무력해진다.

---

## 3. 필요한 특성화 테스트 (M6.1 상세)

본 SPEC 은 **기존 UI 를 고치는** SPEC 이므로, 고치기 전에 현재 동작을 잠근다. 대상은 `TsdbSourceSection.tsx` 의 아래 여섯 지점이며, 전부 열거 도입과 **무관하게 유지되어야 하는** 동작이다.

| # | 잠글 동작 | 현재 위치 | 열거 도입 후 |
|---|-----------|-----------|--------------|
| CT-01 | 에이전트 미선택 시 조회하지 않고 안내를 표시 | `:590-594`(`chart-tsdb-no-agent`) | 무변경 |
| CT-02 | 에이전트 변경 시 `series: []` 로 초기화 | `:376` | 무변경 |
| CT-03 | 48 초과 선택 시 절삭 + 배너 | `:284-296` | 무변경 |
| CT-04 | 목록에 없는 에이전트/버킷을 선택 상태로 유지 | `:400-408` · `:455-458` | 무변경 + **시리즈에도 같은 규칙 적용**(§2.10) |
| CT-05 | 디스커버리 응답을 캐시하지 않음(같은 조건 재마운트 시 재호출) | `useDiscoveryList` | 무변경 |
| CT-06 | measurement 미선택 시 선택 표 미노출 | `:595-599`(`chart-tsdb-no-measurement`) | 무변경 |

**CT-07(변경 대상)**: 현재 "행 = field 키 × 태그 1벌"(`:268-278`)을 **명시적으로 잠근 뒤 반전**한다. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §2.11 이 AC-05 에 대해 한 것과 같이 **삭제하지 않고 제자리에서 반전**하며, 테스트 본문에 반전 사유와 대체 SPEC ID 를 주석으로 남긴다.

---

## 4. 위험과 대응

| # | 위험 | 징후 | 대응 |
|---|------|------|------|
| R1 | **v3 실측 환경이 없다** | M1.1 에서 즉시 드러남 | M1 을 중단하고 사용자에게 결정을 요청한다. 실측 없이 M3 을 시작하지 않는다. 대안: v2 만 열거를 지원하고 v3 는 `seriesEnumeration: false` 로 두어 기존 드릴다운을 유지 — **이 대안은 OQ 로 승격해야 한다** |
| R2 | v3 SQL `DISTINCT` 가 태그 컬럼에 대해 동작하지 않음 | M1.4 | `GROUP BY *` 경로로 전환(M1.5 가 이미 실측한다). 둘 다 실패하면 R1 의 대안으로 후퇴 |
| R3 | v2 Flux 열거가 큰 measurement 에서 타임아웃 | M2 통합 테스트 또는 M6 실사용 | `limit(n:)` 을 낮추고, 사전 필터 권유를 절단 배너보다 앞당긴다. 타임아웃은 이미 60초(`influxdb_management.go:43`) |
| R4 | 열거 결과 1,000행이 선택 표를 느리게 만듦 | M6 실사용 | OQ4 의 값을 낮춘다. 가상화는 범위 밖(§1.3)이므로 **상한 조정만으로 대응**한다 |
| R5 | 인터페이스 확장이 D2~D4 를 깨뜨림 | M2.5 컴파일 | `InfluxSchemaDiscoverer`(`influxdb_schema.go:41`)를 넓히지 않고 **별도 인터페이스**로 둔다. 넓히면 그 인터페이스를 만족하던 모든 구현(테스트 모의 포함)이 동시에 깨진다 |
| R6 | 프런트 커버리지 include 누락으로 85% 가 거짓이 됨 | M7.2 | `vite.config.ts` 의 `include` 를 **먼저** 확인한다([SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) §HISTORY-0.5.0 (3) 재발 방지) |
| R7 | eslint 전역 게이트가 범위 밖 부채로 실패 | M7.1 | [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) M7 에서 저장소 전역 46건을 이미 해소했으므로 baseline 이 0 이다. 신규 경고만 다루면 된다 |
| R8 | `field_exact: false` 를 사용자가 오류로 읽음 | M6 사용자 확인 | 문구를 능력 사유(§2.14)로 표현하고 오류 색상을 쓰지 않는다 |
| R9 | 절단 배너가 클라이언트 필터 적용 중에 사라져 오해를 만듦 | M6.6 | 필터 상태와 무관하게 유지(§2.18-5) |

---

## 5. 커밋 분할 계획

| # | 마일스톤 | 커밋 | 성질 |
|---|----------|------|------|
| 1 | M1 | (커밋 없음 — 기록은 spec.md HISTORY 와 OQ 확정으로) | 조사 |
| 2 | M2 | `feat(influxdb): v2 시리즈 열거 쿼리 생성 + 클라이언트 메서드` | 신규 |
| 3 | M3 | `feat(influxdb): v3 시리즈 열거 — SHOW SERIES 부재 대체 경로` | 신규 |
| 4 | M4 | `feat(api): 시리즈 열거 라우트 D5 + 상한 · 절단 신호` | 신규 |
| 5 | M5 | `feat(web): 열거 클라이언트 + 백엔드 능력 표 확장 + i18n` | 신규 |
| 6 | M6.1 | `test(chart): TSDB 선택 UI 특성화 — 열거 도입 전 동작 잠금` | 특성화 |
| 7 | M6.2~M6.7 | `feat(chart): TSDB 시리즈 열거 선택 — 태그 집합 행 · 탐색 창 · 절단 배너` | 반전 포함 |
| 8 | M7 | `test(tsdb): 마무리 — 커버리지 · 정적 검사 · 회귀` | 마무리 |

**커밋 6과 7 을 합치지 않는다.** 특성화가 별도 커밋이어야 "잠근 동작"과 "바꾼 동작"이 diff 로 구분된다.

---

## 6. 완료 정의 (Definition of Done)

| # | 항목 | 판정 |
|---|------|------|
| 1 | §7 의 OQ1 ~ OQ10 이 **전부 확정**되고 spec.md HISTORY 에 기록됨 | M1 종료 시 |
| 2 | acceptance.md 의 AC-01 ~ AC-55 전부 충족 | M7 |
| 3 | `go build ./...` · `go vet ./...` exit 0 | M7.1 |
| 4 | `npx tsc --noEmit` exit 0 · `npx eslint src --max-warnings 0` exit 0 | M7.1 |
| 5 | `go test ./...` · `npx vitest run` 전부 통과 | M7.3 |
| 6 | 신규 파일 커버리지 85% 이상, **그리고 그 파일들이 coverage include 에 실제로 포함됨** | M7.2 |
| 7 | 저장된 `tsdb_source` config 의 렌더 결과 불변 — 특성화로 고정 | M6.1 |
| 8 | 신규 문구가 `ko.json` · `en.json` 두 로케일에 존재 | M5.4 |
| 9 | v3 경로에 `SHOW SERIES` 문자열이 존재하지 않음 | AC-45 |
| 10 | `internal/migrate/tsdbtags` import 0건 유지 | AC-47 |

---

## 7. [SPEC-TSDB-002](../SPEC-TSDB-002/spec.md) 와의 관계

| 축 | TSDB-002 | 본 SPEC |
|----|----------|---------|
| `data_source: 'tsdb'` 도입 | **소유** | 소비 |
| `TsdbSourceConfig` 형상 | **소유** | 무변경 |
| `POST /series/query` | **소유** | 무변경 |
| D1~D4 디스커버리 | **소유** | 무변경(D4 의 클라이언트 소비만 사라짐) |
| 능력 게이팅 기계 | **소유** | 항목 2개 추가 |
| 시리즈 선택 상한 48 | **소유** | 무변경 |
| 3단 드릴다운(§2.15 [O1]) | 소유 | **역할 재배치**(OQ8 이 대체 선언 여부를 정한다) |
| 시리즈 **열거** | 없음 | **소유** |
