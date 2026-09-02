---
id: SPEC-TSDB-002
title: 대시보드 패널 TSDB 데이터소스 — 패널측 계약 확립 + InfluxDB 첫 백엔드
version: 0.6.0
status: draft
created: 2026-08-22
updated: 2026-09-02
author: xtra
priority: high
domain: dashboard
related_specs:
  - SPEC-TSDB-001
  - SPEC-PANEL-SETTINGS-001
  - SPEC-WEB-005
  - SPEC-WEB-006
  - SPEC-CHART-002
  - SPEC-INFLUX-001
  - SPEC-INFLUX-002
  - SPEC-CHART-001
  - SPEC-STORE-004
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-22 | xtra | 최초 작성 — 패널측 계약을 확립하고 **인메모리 TSDB(memTSDB)** 어댑터로 입증한 뒤 InfluxDB 를 얹는 2단계 구성. |
| 0.2.0 | 2026-08-22 | xtra | **도메인 모델 정정에 따른 재범위(re-scope).** 아래 §HISTORY-0.2.0 참조. |
| 0.3.0 | 2026-08-22 | xtra | 신규 질문 NQ1~NQ5 **전부 확정**(모두 잠정안 그대로). NQ4 — 미등록 `/api/v1/tsdb/*` 라우트는 **본 SPEC 범위 밖의 별도 이슈**로 분리한다. TSDB 패널 소스가 memTSDB 를 쓰지 않게 되어 이 SPEC 의 구현을 막지 않으며, `internal/cli/tsdb.go` 하위명령 · 에이전트 상세 Series 탭 · `services/api/tsdb.ts` · `hooks/useTsdb.ts` 의 404/도달불가 상태와 SPEC-TSDB-001 REQ-TSDB-040 미충족 사실은 기록만 남긴다. NQ-rename — `SeriesDataSourceKind` 의 `'tsdb'` → `'memtsdb'` 리네임을 **확정**한다(같은 문자열이 두 화면에서 반대 뜻을 갖는 것을 막는다; 대상 5개 지점, 해당 경로가 라우트 미등록이라 리네임 위험이 낮다). NQ1·NQ2·NQ3·NQ5 도 잠정안 확정. 결정에 따른 요구사항 변경 없음 — 확정만 기록한다. |
| 0.4.0 | 2026-08-22 | xtra | **M1~M5 구현 회차의 사실 정정 3건.** 구현이 각 정본을 따른 결과 SPEC 본문의 서술 3지점이 사실과 어긋남을 확인했다. 요구사항 변경 없음 — 서술만 정정한다. §HISTORY-0.4.0 참조. |
| 0.5.0 | 2026-08-22 | xtra | **M6 + M7 구현 회차 기록.** M6 커밋 4건으로 첫 사용자 가치에 도달했고, M7 마무리에서 **구현 이탈 1건(eslint 게이트 범위 확대)** · **측정 범위 확대 1건(커버리지 include)** · **사실 정정 1건(3소스 교차 검증이 실제로는 2소스였음)** 이 발생했다. 요구사항 변경 없음 — 회차 사실과 그 처분만 기록한다. §HISTORY-0.5.0 참조. |
| 0.6.0 | 2026-09-02 | xtra | **구현 노트 추가**(§8). 본문 §1~§7 은 spec-first 규약대로 보존하고, 계획에서 벗어난 지점만 기록한다 — 채널 소스 완전 제거로 AC-09·AC-12 의 전제가 사라진 사실, `sources[]` 다중 소스(수평 확장, NQ1 과 다른 축), X축 구간의 패널 소유화, 범위 밖 차트 표시 개편 7종. |

### HISTORY-0.5.0 — M6 + M7 구현 회차

#### (1) M6 — 합류 지점 도달, 커밋 4건

계약 축(M1~M3)과 백엔드 축(M4~M5)이 합류해 처음으로 사용자에게 보이는 변화가 생겼다.

| 커밋 | 범위 |
|------|------|
| `caa5d62e` | 시리즈 매트릭스 피벗 공용화 — 주입 가능한 키 페처 (무동작 리팩터) |
| `427477a0` | TSDB 시리즈 어댑터 + 훅 배선 — 에이전트 타입 파생 · 부분 실패 격리 |
| `1bc1008b` | TSDB 토글 config 영속 — 로컬 상태 제거 · AC-05 제자리 반전 |
| `bf461774` | TSDB 선택 UI + 능력 게이팅 + 상태 4종 + i18n |

#### (2) M7 구현 이탈 1건 — eslint 게이트의 범위 확대

**AC-46 의 `npx eslint src --max-warnings 0` 을 통과시키기 위해, 사용자 승인 하에 저장소 전역 eslint 경고 46건(25개 파일)을 해소했다.** 이는 본 SPEC 이 계획한 범위를 넘어선 조치이므로 이탈로 기록한다.

그중 이 SPEC 이 원래 건드린 파일은 `AgentDetailPanel.tsx` **1개(3건)** 뿐이고, 나머지 **24개 파일은 본 SPEC 범위 밖의 기존 부채**였다. AC-46 이 `src` 전역을 대상으로 하는 게이트이므로, 이 SPEC 의 변경분만 고쳐서는 게이트를 통과할 수 없었다.

해소한 규칙은 2종이다.

| 규칙 | 건수 | 조치 |
|------|------|------|
| `react-hooks/exhaustive-deps` | 15 | 폴백 표현식을 `useMemo` 로 감쌌다 |
| `react-refresh/only-export-components` | 31 | 비컴포넌트 export 를 형제 모듈로 분리 — 신규 모듈 15개, 파일 분할 2건 |

파일 분할 2건은 `TargetContext.tsx` → `TargetContext.ts` + `TargetProvider.tsx`, `panelChromeContext.tsx` → `panelChromeContext.ts` + `PanelChromeProvider.tsx` 다.

**`eslint-disable` 주석 0건 · 규칙 severity 하향 0건.** 경고를 잠재우는 방식이 아니라 원인을 없애는 방식으로만 해소했다.

별도로, gitignore 된 `tsc -b` 빌드 산출물 `web/vite.config.js` · `vite.config.d.ts` 를 `web/eslint.config.js` 의 `ignores` 에 추가했다(비스코프 `npx eslint .` 의 `no-undef` 오류 1건 해소).

#### (3) M7.2 커버리지 — 측정 범위가 좁아 신규 파일 4개가 측정되지 않고 있었다

**`web/vite.config.ts` 의 coverage `include` 가 좁아 신규 파일 4개가 아예 측정 대상에 들어오지 않았다** — `TsdbSourceSection.tsx` · `seriesMatrixPivot.ts` · `tsdbSource.ts` 와 수정 파일 `seriesDataSource.ts`. 즉 "커버리지 85%"라는 DoD 항목이 이 파일들에 대해서는 **측정조차 되지 않은 채** 참으로 보일 수 있는 상태였다.

include 를 확장하고 `tsdbSource.test.ts` 에 테스트 11건을 보강해 `tsdbSource.ts` 를 75.95% → 100% 로 올렸다. 그 결과 **신규 파일 7종이 전부 85% 이상**이다.

| 신규 파일 | % Stmts |
|-----------|---------|
| `panelDataSource.ts` | 96.29 |
| `panelSeriesStatus.ts` | 100 |
| `usePanelSeriesData.ts` | 100 |
| `useTsdbChartData.ts` | 97.57 |
| `TsdbSourceSection.tsx` | 96.12 |
| `seriesMatrixPivot.ts` | 100 |
| `tsdbSource.ts` | 100 |

수정 파일 2종은 DoD 의 85% 대상이 아니므로 손대지 않고 기록만 남긴다 — `ChartPanelSections.tsx` 80.3%, `seriesDataSource.ts` 50%. 후자는 include 확장으로 이제 측정 대상에 들어와 향후 커버리지 표에 계속 보인다.

#### (4) M7.3 사실 정정 — "3소스 교차 검증"이 사실상 2소스였다

**`internal/api/handler/bucket_alignment_crosscheck_test.go` 는 v2 와 v3 를 `influxBucketStartsMs` 헬퍼 한 벌로 접어 `system.SeriesBucketStartMs` 를 공유 호출했다.** 즉 plan.md 7.3 이 요구한 3소스 단언이 아니라 "Store vs 공통 정규화" **2소스 단언**이었다. 두 백엔드가 같은 함수로 수렴하므로 어느 한쪽 방언만 깨져도 테스트가 잡아내지 못한다.

사용자 결정에 따라 **백엔드별 축을 분리**했다.

- **v2** — `BuildFluxSeriesQuery` 가 생성한 Flux `aggregateWindow` 인자에서 버킷 시작을 도출한다.
- **v3** — `BuildInfluxQLSeriesQuery` 가 생성한 InfluxQL `GROUP BY time()` 인자에서 도출한다.
- 그 둘과 Store 를 **3-way 로 단언**한다.

신규 테스트 2건을 추가했다 — `TestBucketAlignment_PerBackendWindowParameters`(각 백엔드의 윈도우 폭 · offset · 레이블 위치를 개별 고정) · `TestBucketAlignment_SharedRuntimeNormalizationMatchesStore`.

**런타임 수렴 사실 자체는 남는다.** `normalizeSeriesBuckets`(`influxdb_agent.go`)는 두 방언 모두 `SeriesBucketStartMs` 를 통과시키므로, **분리 가능한 지점은 정규화가 아니라 질의 생성**이다. 수렴 지점은 별도로 명명된 테스트(`..._SharedRuntimeNormalizationMatchesStore`)가 덮으며, 그것이 3소스 단언을 대체하지 않음을 테스트 주석에 명시했다.

축 분리의 실효는 **변이 검증 3건**으로 입증했다.

| 변이 | 기대 | 결과 |
|------|------|------|
| v2 전용: `timeSrc: "_start"` → `"_stop"` | v2 축만 실패 | 부합 |
| v2 전용: `every: interval` → `interval*2` | v2 축만 실패 | 부합 |
| v3 전용: `GROUP BY time(%s)` → `time(%s,7000ms)` | v3 축만 실패 | 부합 |

특히 **`every: interval*2` 변이는 개정 전 테스트 파일 전체가 통과시켰다** — 개정이 없었다면 Flux 윈도우 폭이 두 배가 되어도 교차 검증이 침묵했다는 뜻이다.

---

### HISTORY-0.4.0 — 구현 회차에서 드러난 사실 정정 3건

M1~M5 구현(커밋 `d96f9c66` ~ `1204abe5`) 과정에서 SPEC 본문의 서술 3지점이 트리의 실제 상태와 어긋남이 확인되었다. **요구사항은 바뀌지 않는다** — 구현은 세 지점 모두에서 SPEC 이 지정한 정본(§2.3 의 표 · §2.7 의 템플릿 · §4.5 의 복제 규칙)을 따랐고, 어긋난 것은 그 정본을 설명하는 산문이다.

**(1) §2.3 의 "동작 동일하게 이관한다" 는 거짓이었다.**

정본 표는 store 활성 조건에 tag 항(`selection_mode === 'tag'` + `tag_filters` ≥ 1)을 포함한다. 그러나 이관 전 6개 패널의 실제 조건은 둘로 갈려 있었다.

| 패널 | 이관 전 활성 조건 | 표 대비 |
|------|------------------|---------|
| `LineChartPanel` · `gaugeLegacyBinding` | `series > 0` **또는** tag 모드 | 동일 |
| `StatPanel` · `BarChartPanel` · `PieChartPanel` · `TablePanel` | `series > 0` **만** | tag 항 **가산** |

따라서 `{ data_source: 'store', store_source: { series: [], selection_mode: 'tag', tag_filters: {...} } }` 로 저장된 통계 · 바 · 파이 · 테이블 패널은 이관 전 채널 경로였고 이관 후 Store 경로로 활성화된다. **이는 §2.4 [U4] "저장된 config 의 렌더 결과 불변" 의 명시적 예외다.**

수용하는 이유: 대안은 `panelDataSource.ts` 에 패널별 예외를 두는 것인데, 그것은 §2.3 이 없애려는 상태(활성 조건이 지점마다 갈림) 그 자체다. 4개 패널이 tag 모드를 지원하지 않았던 것은 설계 결정이 아니라 SPEC-WEB-005 이관 시의 누락으로 보이며, 표 쪽이 옳다.

특성화 테스트 CT-01 ~ CT-05 는 이 4개 패널에 tag 모드를 시험하지 않으므로 RED 가 되지 않았다. `StatPanel.multiOutput.test.tsx:190` 이 같은 config 형태를 쓰지만 resolved key 0개로 동일한 빈 렌더를 산출해 통과한다. **런타임에서는 tag 필터가 실제 키를 해석하므로 렌더가 달라질 수 있다.**

**(2) v3 에서 `bucket` 은 질의에 도달하지 않는다.**

§2.7 의 InfluxQL 템플릿 `SELECT ... FROM "<m>"` 에는 database 를 담을 자리가 없다. v3 에서 database 는 클라이언트 연결에 바인딩되므로, `influxSeriesQueryRequest.bucket` 은 v2 에서만 유효하고 v3 에서는 무시된다. §2.7 에 이 사실을 명시했다. 선택 UI 가 v3 에이전트에서 bucket 입력의 한계를 드러내는 것은 M6 의 능력 게이팅(§2.13) 소관이다.

**(3) §2.10 D2 표의 v2 구현 이름이 틀렸다.**

`schema.tagKeys()` 로 적었으나 이는 버킷 전역 조회다. D2 라우트는 `?measurement=<m>` 로 measurement 한정이므로 올바른 함수는 `schema.measurementTagKeys(bucket:, measurement:)` 이며, 이는 `internal/migrate/tsdbtags/client_v2.go` 가 실제로 쓰는 함수이기도 하다. §4.5 의 복제 규칙이 지시하는 대상이 후자이므로 구현은 처음부터 옳았고, 표의 셀만 틀렸다.

---

### HISTORY-0.2.0 — 무엇이 바뀌었는가

**(1) 모델 정정 — 이것이 하중 지지점이다.**

v0.1.0 은 "TSDB = memTSDB(`internal/tsdb/`)"로 읽고 작업을 "memTSDB 먼저 배선 → 그 다음 InfluxDB"로 단계화했다. **그것은 제품 모델이 아니다.** 정정된 모델은 다음 셋을 서로 다른 것으로 취급한다.

| 이름 | 실체 | 패널 데이터소스인가 |
|------|------|---------------------|
| **Store** | **인메모리 TSDB.** `NewVolatileStore(maxKeyLength, maxHistorySize, historyTTL)`(`internal/agent/system/store.go:217`) — 히스토리와 TTL 을 가진 휘발성 저장소. 생성 가능한 에이전트 타입(`web/src/config/agentSchemas.ts:24`) | **그렇다** (`data_source: 'store'`, 이미 구현됨) |
| **TSDB** | **에이전트를 통해 접근하는 외부 시계열 DB.** **InfluxDB 가 첫 지원 백엔드**(`agentSchemas.ts:15`) | **그렇다 — 본 SPEC 이 신설한다** (`data_source: 'tsdb'`) |
| **memTSDB** | `internal/tsdb/` · `/api/v1/tsdb/*`. 플로우 노드(`tsdb-write` · `tsdb-query`)와 WS 구독자를 위한 **내부 설비** | **아니다.** §1.2.1 · §1.3 |

이 정정이 OQ2 를 결정한다 — **에이전트 참조가 없으면 어느 외부 DB 에 연결할지 알 수 없으므로 에이전트 필드는 필수다.**

**(2) 확정된 열린 질문 12건**

| # | 확정 | v0.1.0 대비 |
|---|------|-------------|
| OQ1 | **단일 `tsdb_source` 블록 + 백엔드 판별자** | **반전** (v0.1.0 은 `tsdb_source`/`influx_source` 분리 유지) |
| OQ2 | **에이전트 참조 필수** (SPEC-WEB-006 의 `agent_id` 정본 + `agent_name` 폴백) | **반전** (v0.1.0 은 "필드를 두지 않음") |
| OQ3 | **평탄한 `chartQueryResponse`(`entries[]` + `labels`) 재사용 + 클라이언트 피벗** | **반전** (v0.1.0 은 서버 행렬 응답) |
| OQ4 | **폐기(moot)** — memTSDB 가 패널 소스가 아니므로 `time.Truncate` 정렬 불일치가 패널에서 도달 불가 | 폐기 |
| OQ5 | **폐기(moot)** — memTSDB 어댑터의 N요청 팬아웃은 본 SPEC 범위 밖 | 폐기 |
| OQ6 | 유지 — InfluxDB 태그 기반 동적 바인딩 **제공하지 않음** | 무변경 |
| OQ7 | **TSDB = 외부 시계열 DB.** 토글 3종(`채널`·`Store`·`TSDB`), `SeriesDataSourceKind` 의 memTSDB 식별자를 `'memtsdb'` 로 개명해 충돌 해소 | 확정 + UI 충돌 해소 추가 |
| OQ8 | 유지 — v3 measurements 501 해소를 **포함** | 무변경 |
| OQ9 | **폐기(moot)** — Store 와 InfluxDB 는 모든 인터벌에서 버킷이 일치하므로 `86400 % i == 0` 제한이 불필요 | 폐기 |
| OQ10 | 유지 — v2 는 bucket 목록, v3 는 자유 입력 | 무변경 |
| OQ11 | 유지 — 생성 쿼리는 기본 미노출 | 무변경 |
| OQ12 | 유지 — `web/src/hooks/useTsdb.ts` 제거하지 않음(비범위) | 무변경 |

**(3) 은퇴한 요구사항 · 인수조건**

| ID | 내용 | 은퇴 사유 |
|----|------|-----------|
| U5 | memTSDB 어댑터 3결함(G1 `fieldName` · G2 `AbortSignal` · G3 팬아웃) 수정 | memTSDB 는 패널 소스가 아니다. 세 결함은 **에이전트 상세 Series 탭 경로의 기존 부채**로 남으며 별도 SPEC 이 소유한다(§1.3) |
| AC-13 ~ AC-16 | U5 의 인수조건 | 상동 |
| AC-27 의 memTSDB 분기 | "420초 인터벌에서 memTSDB 가 Store 와 어긋난다"는 sentinel | 비교 대상에서 memTSDB 제거. 대신 **Store ↔ InfluxDB 가 420초에서도 일치함**을 검증하도록 반전 |
| §4.4 | memTSDB `time.Truncate` 처분 결정 | OQ4 폐기와 함께 은퇴 |
| §4.1 의 "memTSDB 로 계약을 입증한다" | 단계화 근거 | 정정된 모델에서 memTSDB 는 아무것도 입증하지 않는다(§4.1 재작성) |
| UB1-2 | "단일 블록 금지" | OQ1 확정으로 **반전** — 이제 백엔드별 블록 분리가 금지된다 |

**(4) 새로 발견해 기록한 사실**

- `NewTSDBHandler` 는 **프로덕션 호출자가 0건**이다. `/api/v1/tsdb/*` 6종 라우트는 **실행 중인 서버에 등록되지 않는다**(§1.2.1). 즉 v0.1.0 의 "memTSDB 어댑터는 이미 프로덕션에서 동작 중"이라는 전제는 **거짓**이었다.
- 쓰기 경로가 같은 문제를 이미 풀었다 — `newStorageBackend(a)`(`internal/node/storage_write.go:190`)가 **참조된 에이전트의 타입**으로 백엔드를 고른다(§1.2.6). 본 SPEC 은 그 축을 그대로 따른다.

---

## 1. 개요 (Overview)

### 1.1 목적

사용자 요구는 두 문장이다.

```
TSDB 데이터소스 구현
- InfluxDB 기본 지원
```

여기서 **TSDB 는 에이전트를 통해 접근하는 외부 시계열 DB**를 가리키며, **InfluxDB 가 첫 지원 백엔드**다. 이를 **한 SPEC · 두 축**으로 분해한다.

1. **패널측 계약 축** — 패널이 데이터소스 종류에 대해 하드코딩된 동등 비교를 하지 않고 **소스 종류로 키잉된 조회**를 하도록 바꾼다. 여기에 `data_source: 'tsdb'` 와 `tsdb_source` config 를 추가한다. 백엔드 변경 없음.
2. **InfluxDB 백엔드 축** — 구조화 질의 엔드포인트 · Flux/InfluxQL 생성 · 스키마 디스커버리. 프론트 변경 없음.

두 축은 파일 집합이 겹치지 않아 **병렬 진행이 가능**하며, 합류 지점(어댑터 + 선택 UI)에서 처음으로 사용자 가치가 생긴다.

**정직한 진술 하나** — v0.1.0 은 "memTSDB 어댑터가 중간 단계에서 그 자체로 사용자 가치를 낸다"고 적었다. 정정된 모델에서 그 중간 가치는 **없다.** 계약 축(M1~M3)은 순수 리팩터이며 사용자에게 보이는 변화가 0이다. 계약을 먼저 세우는 근거는 "중간 가치"가 아니라 **특성화로 잠근 무동작 리팩터를 InfluxDB 배선과 분리해 회귀 원인을 이등분 가능하게 만드는 것**이다(§4.1).

### 1.2 배경

#### 1.2.1 "TSDB" 라는 이름이 서로 무관한 세 대상을 가리킨다

본 SPEC 의 모든 문장은 아래 셋 중 무엇을 말하는지 **반드시 명시한다.**

| 이름 | 실체 | 위치 | 라우트 | 패널 소스 |
|------|------|------|--------|-----------|
| **Store** | 인메모리 TSDB. `NewVolatileStore(...)` — 히스토리 + TTL. 생성 가능한 에이전트 타입 | `internal/agent/system/store.go:217` | `/api/v1/store/{agent_name}/*` | **예** (기구현) |
| **TSDB** | 에이전트를 통해 접근하는 **외부** 시계열 DB. **InfluxDB 가 첫 백엔드** | `internal/agent/system/influxdb_*.go` | `/api/v1/influxdb/{agent_name}/*` | **예 — 본 SPEC** |
| **memTSDB** | 프로세스 내 시계열 저장소. 플로우 노드(`tsdb-write`·`tsdb-query`)와 WS 구독자용 **내부 설비** | `internal/tsdb/`, `internal/agent/system/tsdb_agent.go` | `/api/v1/tsdb/*` — **미등록**(아래) | **아니오** |

**memTSDB 가 패널 소스가 아닌 이유를 다음 독자를 위해 명시한다.** memTSDB 는 (a) 사용자가 "어느 DB에 저장했는가"를 고르는 대상이 아니라 플로우 내부의 임시 시계열 버퍼이고, (b) 그 HTTP 표면이 실행 중인 서버에 존재하지 않는다.

**(b) 는 본 SPEC 이 검증한 사실이다.**

```
$ grep -rn "NewTSDBHandler" --include='*.go' .
internal/api/handler/tsdb.go:35:   func NewTSDBHandler(...)      ← 정의
internal/api/handler/tsdb_test.go:105,122,552,561               ← 테스트 4건
```

프로덕션 호출자가 **0건**이며, `cmd/xflowd/main.go:921-967` 의 `server.RegisterRoutes(...)` 블록에 `tsdbHandler.RegisterRoutes(g)` 가 **없다.** 따라서 `POST /api/v1/tsdb/query` 를 비롯한 6종 라우트는 런타임에 존재하지 않는다.

> 부수 관측 — `internal/cli/tsdb.go` 와 `web/src/services/api/tsdb.ts` 는 그 라우트들을 호출한다. 즉 **CLI 의 `tsdb` 하위명령과 에이전트 상세 화면의 Series 탭은 현재 404 를 받는다.** 이는 본 SPEC 이 만든 문제가 아니고 본 SPEC 이 고칠 문제도 아니다(§1.3). 다만 v0.1.0 이 "memTSDB 어댑터는 프로덕션에서 동작 중이므로 계약을 입증할 수 있다"고 적은 전제를 무효화하므로 여기 기록한다.

한편 `tsdb` **에이전트 타입은 존재한다** — `internal/agent/system/tsdb_register.go:9` 가 등록하고 `cmd/xflowd/main.go:412` 가 호출하며, 각 인스턴스는 자기 `tsdb.New(...)` 를 갖는다(`tsdb_agent.go:190`). 그러나 프론트의 생성 가능 타입 목록(`agentSchemas.ts:8-30`)에 `tsdb` 가 **없고**, HTTP 표면이 미등록이므로 패널이 도달할 방법이 없다. v0.1.0 의 "memTSDB 는 싱글톤이며 에이전트가 아니다"는 서술은 **부정확**했다 — 정확히는 *에이전트 타입은 있으나 HTTP 표면이 없다*.

#### 1.2.2 프론트엔드의 통합 추상화는 이미 완성되어 있다

[`web/src/services/api/seriesDataSource.ts`](../../../web/src/services/api/seriesDataSource.ts) 가 공용 계약을 정의한다.

```ts
export type SeriesDataSourceKind = 'tsdb' | 'store';          // :16 — 현재 값

export interface SeriesMatrixQuery {
  keys: string[];
  seriesFilters?: Array<SeriesSelectorFilter | undefined>;     // { fieldName?, tags? }
  startMs: number; endMs: number; intervalMs: number;
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last';
  fill?: '' | 'null' | 'zero' | 'previous' | 'avg';
}

export interface SeriesMatrix {
  columns: string[];
  rows: Array<{ bucketStartMs: number; values: Array<number | null> }>;
}
```

이 계약이 본 SPEC 전체의 하중 지지점이다 — **패널은 `SeriesMatrix` 만 소비하므로, 그 형상을 반환하는 소스는 패널 렌더 코드를 고치지 않고도 꽂힌다.**

그리고 **`SeriesMatrixQuery` 의 축(`keys[i]` + `seriesFilters[i].{fieldName, tags}`)이 InfluxDB 의 시리즈 식별 축과 그대로 대응한다** — `key` ↔ measurement, `fieldName` ↔ field, `tags` ↔ tags. 즉 계약을 넓힐 필요가 없다(§2.2 · §4.3).

#### 1.2.3 패널의 TSDB 토글은 의도된 무동작(no-op)이다

[`ChartPanelSections.tsx:320`](../../../web/src/pages/dashboard/ChartPanelSections.tsx) 이 TSDB 모드를 **로컬 `useState` 에만** 담는다.

```ts
const [tsdbMode, setTsdbMode] = useState(false);
const effectiveMode: 'channel' | 'store' | 'tsdb' = tsdbMode ? 'tsdb' : dataSource;
```

같은 파일 `:372-381` 의 토글이 이미 **3종(`channel` · `store` · `tsdb`)을 렌더**하지만 TSDB 버튼은 `setTsdbMode(true)` 로만 라우팅하며 **`onConfigChange` 를 호출하지 않는다.** 이는 [SPEC-PANEL-SETTINGS-001](../SPEC-PANEL-SETTINGS-001/spec.md) REQ-05 의 요구이고, [`PanelSettingsDataSource.test.tsx:138`](../../../web/src/pages/dashboard/PanelSettingsDataSource.test.tsx) 의 AC-05 테스트가 `expect(onConfigChange).not.toHaveBeenCalled()` 로 잠갔다.

본 SPEC 은 그 비목표를 **대체(supersede)** 하며, 해당 단언을 §2.11 [E1] 에 따라 **제자리에서 반전**한다.

> **토글이 이미 3종이라는 점이 재범위의 부수 이익이다.** v0.1.0 은 4종(`채널`·`Store`·`TSDB`·`InfluxDB`)을 계획했으나, 정정된 모델에서는 InfluxDB 가 별도 종류가 아니라 TSDB 의 **백엔드**이므로 토글 구조가 무변경이다. 바뀌는 것은 TSDB 버튼의 동작뿐이다.

#### 1.2.4 프론트엔드에 없는 것 (재확인)

| 없는 것 | 확인 |
|---------|------|
| `ChartDataSourceKind` 에 `'tsdb'` | `chartChannelTypes.ts:49` = `'channel' \| 'store'` (2종) |
| `TsdbSourceConfig` 타입 · `tsdb_source` config 필드 | 트리 전체 0건 |
| `useTsdbChartData` 훅(`useStoreChartData` 대응물) | 없음 |
| `web/src/hooks/useTsdb.ts` 의 소비자 | **0건 — 사장 코드(dead code)** |

#### 1.2.5 소스 종류가 10개 지점에 하드코딩되어 있다

패널이 "store 인가"를 판정하는 **프로덕션 렌더 디스패치 지점**은 다음 10곳이다(주석 · 테스트 제외). 전부 v0.2.0 작성 시점에 재확인했다.

| # | 파일 | 행 | 형태 |
|---|------|-----|------|
| 1 | `charts/LineChartPanel.tsx` | 245 | `config.data_source === 'store' && ...` |
| 2 | `charts/StatPanel.tsx` | 80 | 동일 |
| 3 | `charts/BarChartPanel.tsx` | 100 | 동일 |
| 4 | `charts/PieChartPanel.tsx` | 89 | 동일 |
| 5 | `charts/TablePanel.tsx` | 101 | 동일 |
| 6 | `heatmap/HeatmapPanel.tsx` | 122 | 동일 |
| 7 | `charts/gaugeLegacyBinding.ts` | 122 | `flags.dataSource === 'store' && ...` (게이지 값 소스 판정) |
| 8 | `PanelSettingsDialog.tsx` | 714 | 미리보기 렌더 게이팅 |
| 9 | `PanelSettingsDialog.tsx` | 728 | 미리보기 config 게이팅 |
| 10 | `charts/previewSeries.ts` | 65 | 미리보기 범례 시리즈 구성 |

> 게이지의 실제 디스패치는 `GaugePanel.tsx:818` 이 **아니라** `gaugeLegacyBinding.ts:122` 다. `:818` 은 주석이다(재확인함).

여기에 config 를 **쓰는** 리터럴이 5곳 더 있다: `ChartPanelSections.tsx:330`, `AddPanelDialog.tsx:162`, `heatmapConfig.ts:271`, `gaugeLegacyBinding.ts:141`(타입) · `:231`(반환값). 그리고 설정 UI 게이팅이 `ChartPanelSections.tsx:398 · 404 · 445 · 458` 4곳, 게이지 이관 상태 판정이 `gaugeLegacyBinding.ts:201` 1곳이다.

**핵심 문제는 개수가 아니라 증가율이다.** 소스 종류를 N개로 늘릴 때마다 각 지점에 동등 비교가 하나씩 더 붙으면 지점 × 종류로 곱해진다. 게다가 각 지점이 서로 다른 부가 조건(`series.length > 0` / `tag_filters` / `series_reduce`)을 갖고 있어, 새 종류를 추가할 때 **어느 지점에서 어느 조건을 빠뜨렸는지 컴파일러가 알려주지 않는다.**

#### 1.2.6 쓰기 경로가 같은 문제를 이미 풀었다 — 본 SPEC 이 따를 선례

`storage-write` 노드는 **참조된 에이전트의 타입에서 백엔드를 파생**한다.

```go
// internal/node/storage_write.go:152 (resolveBackend 내부)
backend, err := newStorageBackend(a)

// internal/node/storage_write.go:190
func newStorageBackend(underlying any) (storageBackend, error) {
    typer, ok := underlying.(agentTyper)
    if !ok { return nil, fmt.Errorf("agent does not expose Type()") }
    switch t := typer.Type(); t {
    case "store":    return &storeBackend{}, nil
    case "influxdb": return &influxdbBackend{}, nil
    default:
        return nil, fmt.Errorf("unsupported agent type %q for storage-write (expected: store, influxdb)", t)
    }
}
```

구현 둘이 하나의 인터페이스(`storageBackend`: `backendName()` · `resolve()` · `write()`)를 만족한다.

- `internal/node/storage_backend_store.go:79` → `backendName() == "store"`
- `internal/node/storage_backend_influxdb.go:44` → `backendName() == "influxdb"`

그리고 **중립 어휘 매핑 규약**이 각 백엔드 파일 주석에 정본으로 적혀 있다.

| 중립 어휘 | Store | InfluxDB |
|-----------|-------|----------|
| `series_key` | Store 키 | measurement |
| `values[].name` | field | field 이름 |
| `tags` | 시리즈 태그 | InfluxDB 태그 |

`storage_write.go:190` 의 주석은 왜 **능력(interface) 기반 판별을 쓸 수 없는지**까지 적어 두었다 — Store 와 InfluxDB 에이전트가 둘 다 `Process([]byte)` 를 가지므로 인터페이스 단언이 양쪽에 성립하기 때문이다. 읽기 경로도 같은 함정을 갖는다.

또한 백엔드 전용 키의 처리 방식도 선례가 있다 — `storage_write.go:214` 주석: *"백엔드 전용 키 (해당 없는 백엔드는 무시): `namespace`, `ttl` (store) / `bool_to_int` (influxdb)"*. **평탄한 config + 백엔드 스코프 주석**이지 중첩 블록 분리가 아니다.

**본 SPEC 의 읽기측 데이터소스는 이 축을 그대로 따른다** — 병행 축을 새로 만들지 않는다(§2.2 · §2.18 · §4.2).

#### 1.2.7 에이전트 참조는 SPEC-WEB-006 패턴이 이미 정본을 갖고 있다

`StoreSourceConfig`(`chartChannelTypes.ts:102-120`)가 두 필드를 둔다.

```ts
agent_id?: string;    // 안정적 ID(정본). 리네임에도 끊기지 않는다.
agent_name: string;   // 표시용 스냅샷 + 하위호환 폴백.
```

해석은 `resolveStoreAgentName(agentId, fallbackName, agents)`(`panels/charts/storeAgentResolve.ts:35`)가 **질의 시점에** 수행하며, 소비자가 4곳(`useStoreChartData.ts:285` · `PanelSettingsDataSource.tsx:206` · `PanelSettingsDialog.tsx:3973` · `GaugePanel.tsx:109`)이다. 함수 시그니처는 `StoreAgentRef { id, name }` 만 요구하므로 **이미 소스 중립**이다.

#### 1.2.8 평탄한 응답 + 소스 중립 피벗 기계가 이미 존재한다

| 자산 | 위치 | 성질 |
|------|------|------|
| `chartQueryResponse{entries[], count, truncated}` | `internal/api/handler/store_query.go:231` | Store 표준 응답 |
| **원문 통과 InfluxDB 가 이미 이 타입을 재사용한다** | `internal/api/handler/influxdb_query.go:115` | `resp := chartQueryResponse{...}` |
| `chartQueryEntry{timestamp, value, labels}` | `store_query.go:224` | 시리즈 구분은 `labels` |
| 예약 라벨 `__field__` + 태그 | `web/src/services/api/seriesLabels.ts:22` | `METRIC_LABEL_KEY` |
| `seriesSignature(labels)` · `makeSeriesId(key, metric, tags)` | `seriesLabels.ts:67 · :89` | **소스 중립** |
| `groupEntriesBySeries(entries)` | `web/src/services/api/store.ts:434` | **소스 중립** — 라벨 서명으로 그룹화 |
| 버킷 합집합 피벗 | `web/src/services/api/store.ts:640-654` | **소스 중립** — 합집합 수집 → 정렬 → 컬럼 순서대로 배치 |
| 컬럼 자리 보존(0행 key 도 단일 컬럼) | `store.ts:626-631` | 인덱스 기반 색상·라벨 매칭 보호 |

**즉 "요청 1건 = 시리즈 1개, 응답은 평탄한 `entries[]`, 클라이언트가 라벨로 그룹화하고 버킷 합집합으로 피벗"이라는 파이프라인이 이미 작동 중이며 소스를 모른다.** InfluxDB 를 같은 형상에 맞추면 새 기계가 생기지 않는다(§4.3).

#### 1.2.9 InfluxDB 백엔드에 있는 것 / 없는 것

**있는 것:**

| 자산 | 위치 | 성질 |
|------|------|------|
| `POST /influxdb/{agent_name}/query` | `influxdb_query.go:49` | **원문 통과(raw passthrough)** 전용. `{query_language:"flux"\|"influxql", query:"<원문>", params}` |
| bucket CRUD · truncate · measurement 목록/삭제 | `influxdb_management.go:58-63` | **v2 전용.** v3 는 6종 전부 `ErrManagementNotSupported` → **501** |
| v2/v3 클라이언트 · write · 쿼리 실행 | `influxdb_v2.go` · `influxdb_v3.go` | 존재 |
| **v2+v3 스키마 디스커버리 완제품** | `internal/migrate/tsdbtags/` | 존재. **어디에도 라우팅되지 않음** |

`influxDBQueryRequest.Params`(`:56`)는 **선언만 되어 있고 한 번도 읽히지 않는다** — 스텁이다.

**v3 SQL 은 HTTP 로 도달할 수 없다.** 클라이언트는 SQL 을 지원하고(`influxdb_v3.go:76`), v3 의 설정 기본 쿼리 언어도 `sql` 이지만(`influxdb_config.go:101`), 핸들러가 `flux|influxql` 만 화이트리스트한다(`influxdb_query.go:78-80`). 결과적으로 v3 는 API 를 통해서는 InfluxQL 전용이다.

**없는 것(전수 확인):**

| # | 없는 것 | 확인 |
|---|---------|------|
| 1 | 구조화 질의 엔드포인트 | `{measurement, field, tags, start_ms, end_ms, interval_ms, aggregation}` 를 받는 핸들러 0건 |
| 2 | `storeQueryRequest` 대응 요청 타입 | 없음 |
| 3 | **Flux/InfluxQL 생성 계층** | 트리 전체에서 `aggregateWindow` **0건**, `internal/agent/system/` 에서 `GROUP BY time(` **0건** |
| 4 | 집계 어휘 매핑 | `"mean"` · `"MEAN"` 모두 **0건** |
| 5 | 버킷 원점 정렬 계약 | 없음 |
| 6 | 버킷 수 가드 | 없음 (Store 에는 `maxAggregationBuckets = 100_000` 존재 — `internal/api/handler/` 에 4건) |
| 7 | 필드 키 디스커버리 | 핸들러 · 에이전트 메서드 · 클라이언트 메서드 **모두 없음** |
| 8 | HTTP 노출 태그 키/값 디스커버리 | 없음 |

#### 1.2.10 `internal/migrate/tsdbtags/` — 최대 지렛대

이 패키지는 **완성되고 테스트된 v2+v3 스키마 디스커버리 구현**이며, 소비자가 마이그레이션 CLI(`cmd/xflowd/migrate_tsdb_tags.go`) 하나뿐이다. `grep -rn "migrate/tsdbtags" internal/api/ internal/agent/` 는 **0건**이다.

```go
// internal/migrate/tsdbtags/client.go:20
type SchemaClient interface {
    Version() Target
    Ping(ctx) error
    ListMeasurements(ctx) ([]string, error)                    // v2: schema.measurements / v3: SHOW MEASUREMENTS
    ListTagKeys(ctx, measurement) ([]string, error)            // v2: schema.measurementTagKeys / v3: SHOW TAG KEYS
    ListTagValues(ctx, measurement, tagKey) ([]string, error)  // v2: schema.measurementTagValues / v3: SHOW TAG VALUES
    Close() error
}
```

**주의 — 이 자산은 태그 디스커버리만 덮는다.** 필드 키 디스커버리(§1.2.9 #7)는 이 패키지에도 없으며 **새로 작성해야 한다.**

부수 효과 하나: v3 의 `ListMeasurements` 는 `SHOW MEASUREMENTS` 로 **실제 동작한다.** 즉 이 자산의 승격은 기존 `GET /influxdb/{agent}/measurements` 의 v3 501 을 해소한다(§2.10).

#### 1.2.11 `'tsdb'` 라는 코드 식별자가 이미 memTSDB 를 가리키는 자리가 있다

| 자리 | 값 | 의미 |
|------|-----|------|
| `services/api/seriesDataSource.ts:16` | `SeriesDataSourceKind = 'tsdb' \| 'store'` | `'tsdb'` = memTSDB |
| `services/api/tsdb.ts:466` | `tsdbSeriesDataSource()` 가 `kind: 'tsdb'` | memTSDB 어댑터 |
| `pages/agents/AgentDetailPanel.tsx:298` | `const kind: SeriesDataSourceKind = agentType === 'store' ? 'store' : 'tsdb';` | 에이전트 상세 Series 탭 |
| `pages/agents/AgentDetailPanel.tsx:179` | `HAS_SERIES_TAB = new Set(['tsdb'])` | 에이전트 **타입** 문자열(별개 축) |

OQ7 이 `TSDB` 를 외부 시계열 DB 로 확정하므로, `SeriesDataSourceKind` 의 `'tsdb'` 가 memTSDB 를 가리키는 상태를 그대로 두면 **같은 문자열이 두 화면에서 반대 뜻**이 된다. §2.1 이 처분을 정한다.

### 1.3 비범위 (Out of Scope)

- **memTSDB 를 패널 데이터소스로 배선하는 것** — `internal/tsdb/` · `/api/v1/tsdb/*` 는 플로우 노드와 WS 구독자를 위한 내부 설비이며 패널 소스가 아니다(§1.2.1 · §2.16 #21)
- **`/api/v1/tsdb/*` 라우트 미등록 문제의 해소** — `NewTSDBHandler` 의 프로덕션 호출자가 0건이라는 사실(§1.2.1)은 본 SPEC 이 **기록만** 하고 고치지 않는다. 별도 이슈다
- **memTSDB 어댑터(`queryTsdbMatrixPivoted`)의 3결함** — v0.1.0 이 G1/G2/G3 으로 기록한 결함은 여전히 존재하나 **에이전트 상세 Series 탭 경로의 기존 부채**이며 본 SPEC 범위 밖이다. 특히 G1(`seriesFilters.fieldName` 미전달 → `extractScalar` 의 "첫 번째 유한 숫자 필드" 폴백, `tsdb.ts:236`)은 **조용한 오답**이므로 별도 SPEC 으로 승격할 가치가 있다. 단 그 경로 자체가 현재 404 이므로 도달 불가다
- **`POST /influxdb/{agent}/query` 원문 통과 경로의 변경** — v3 SQL 화이트리스트 해제, `params` 스텁 구현 모두 범위 밖. 신규 구조화 엔드포인트는 별도 경로다
- **v3 의 bucket/measurement 관리 조작(생성 · 삭제 · truncate) 지원** — 5종의 501 은 그대로 남는다. 본 SPEC 은 **읽기 전용 디스커버리**만 다룬다
- **Prometheus · 기타 TSDB 백엔드** — 판별자는 확장 가능하게 두되 구현하지 않는다
- **`store_source` 를 `tsdb_source` 로 흡수하는 것** — 쓰기 경로는 Store 와 InfluxDB 를 한 노드가 덮지만, 읽기 경로의 Store 는 이미 배포되어 저장된 config 가 존재한다. 통합은 별도 SPEC(§7 NQ1)
- **패널 타입 추가 · 렌더 규칙 변경** — 기존 패널이 `SeriesMatrix` 를 소비하는 방식은 무변경
- **`series_reduce` · `multi_output_limit` 의 의미 변경** — [SPEC-CHART-002](../SPEC-CHART-002/spec.md) 가 소유. TSDB 소스는 그 축과 **합성**되며 우회하지 않는다(§2.4)
- **Store 데이터소스의 동작 변경** — Store 는 참조 구현으로만 쓰이고 코드 변경 대상이 아니다. 단 §2.3 의 디스패치 일반화가 Store 분기의 **형태**를 바꾼다(동작은 불변, 특성화로 고정)
- **InfluxDB 쓰기 경로** — `influxdb-write` 노드([SPEC-INFLUX-002](../SPEC-INFLUX-002/spec.md)) · `storage-write` 노드는 무변경
- **원격 노드(remote) 패널** — 읽기 전용 프록시로 무변경
- **`web/src/hooks/useTsdb.ts` 사장 코드의 제거** — 별도 정리 이슈. 본 SPEC 은 이 파일을 **소비하지도 확장하지도 않는다**

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 세 이름의 어휘 분리

시스템은 **Store** · **TSDB** · **memTSDB** 를 서로 다른 것으로 취급하며, 어느 하나의 식별자 · 라우트 · config 형상을 다른 것에 재사용하지 않는다.

| 축 | Store | TSDB (외부) | memTSDB |
|----|-------|-------------|---------|
| `ChartDataSourceKind` 값 | `'store'` | `'tsdb'` | **없음**(패널 소스 아님) |
| `SeriesDataSourceKind` 값 | `'store'` | `'tsdb'` | `'memtsdb'` |
| 패널 config 블록 | `store_source` | `tsdb_source` | **없음** |
| 바인딩 단위 | 에이전트(`agent_id` + `agent_name`) | 에이전트(동일) | 해당 없음 |
| 시리즈 식별자 | `key` + `field` + `tags` | `key`(=measurement) + `field` + `tags` | 해당 없음 |

**시스템은 `SeriesDataSourceKind` 의 `'tsdb'` 값을 외부 TSDB 에 배정하고, 기존 memTSDB 용도의 값을 `'memtsdb'` 로 개명한다.** 개명 대상은 5곳이다 — `seriesDataSource.ts:16` · `:119`, `tsdb.ts:466`, `AgentDetailPanel.tsx:26` · `:298`. 이는 **식별자 개명이며 동작 변경이 아니다**(§2.16 #24 가 동작 변경을 금지한다).

개명하는 이유는 §1.2.11 의 충돌이다. `ChartDataSourceKind.'tsdb'` 를 외부 TSDB 로 두면서 `SeriesDataSourceKind.'tsdb'` 를 memTSDB 로 남기면 **같은 문자열이 두 화면에서 반대 뜻**이 되고, 그것이 OQ7 이 없애라고 지시한 모호성 그 자체다. v0.1.0 은 "기존 자산을 개명하지 않는다"를 전제로 삼았으나(가정 1), 그 전제는 memTSDB 를 패널 소스로 본 모델에서만 성립한다.

**사용자에게 보이는 문구.** 데이터소스 토글의 `TSDB` 라벨은 **외부 시계열 DB** 를 가리키며, 백엔드 이름(현재는 `InfluxDB`)을 보조 표기로 함께 노출한다. 에이전트 상세 화면의 Series 탭은 패널 데이터소스와 무관한 별개 화면이므로 라벨을 바꾸지 않는다.

### 2.2 [U2] (Ubiquitous) 패널측 데이터소스 계약 — 단일 `tsdb_source` 블록

시스템은 패널 config 에 다음 형상을 정의한다.

```ts
// chartChannelTypes.ts
export type ChartDataSourceKind = 'channel' | 'store' | 'tsdb';

/** TSDB 소스가 지원하는 백엔드. 확장 지점. */
export type TsdbBackend = 'influxdb';

/**
 * 외부 시계열 DB 소스 설정(data_source === 'tsdb' 일 때 사용).
 *
 * 백엔드 중립 키와 백엔드 전용 키가 한 블록에 평탄하게 공존한다.
 * 이는 쓰기 경로의 선례를 따른 것이다 — `storage_write.go:214` 주석이
 * "백엔드 전용 키 (해당 없는 백엔드는 무시)" 를 같은 방식으로 다룬다.
 */
export interface TsdbSourceConfig {
  /**
   * 기록된 백엔드(스냅샷). **질의 라우팅의 정본이 아니다** — 정본은 항상
   * 참조된 에이전트의 실제 타입이다(§2.18). 이 값은 (a) 에이전트 목록이
   * 로드되기 전 설정 UI 를 그리기 위한 낙관적 표시값이고, (b) 불일치를
   * 감지하기 위한 대조군이다.
   */
  backend: TsdbBackend;

  /** 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006 */
  agent_id?: string;
  /** 에이전트 이름(표시용 스냅샷 + 하위호환 폴백). @spec SPEC-WEB-006 */
  agent_name: string;

  /** [influxdb 전용] v2 = bucket, v3 = database. 미지정이면 에이전트 기본값. */
  bucket?: string;

  /** 조회할 시리즈. 비어 있으면 소스는 비활성이다(§2.3). */
  series: TsdbSeriesRef[];

  time_window_ms: number;
  interval_ms: number;
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last';
  fill?: '' | 'null' | 'zero' | 'previous';
  refresh_interval_ms?: number;
  series_name_format?: string;
}

/**
 * TSDB 시리즈 참조. **어휘는 `StoreSeriesRef` 와 동일**하며, 백엔드별 개념
 * 대응은 쓰기 경로의 매핑 규약(§1.2.6)을 그대로 따른다.
 *
 *   key → measurement (influxdb)
 *   field → field
 *   tags → tags
 */
export interface TsdbSeriesRef {
  /** 시리즈 키. influxdb 백엔드에서는 measurement 이름이다. */
  key: string;
  /** 값 필드. TSDB 소스에서는 **필수**다(§2.16 #4). */
  field: string;
  tags?: Record<string, string>;
  alias?: string;
  color?: string;
  stroke_style?: StrokeStyle;
  stroke_width?: number;
  smooth?: boolean;
}
```

**백엔드별로 블록을 쪼개지 않는다.** v0.1.0 은 반대로 결정했었다(`tsdb_source` / `influx_source` 분리). OQ1 이 이를 반전했고, 근거는 쓰기 경로의 선례다 — `storage-write` 는 백엔드가 둘인데도 config 블록이 하나이며, 백엔드 전용 키를 평탄하게 두고 "해당 없는 백엔드는 무시"라고 주석으로 못박는다. 읽기 경로가 반대 축을 만들면 같은 개념이 두 형태로 존재하게 된다.

**기본값은 Store 와 공유한다.** `defaultTsdbSource()` 의 시간창 · 인터벌 · 집계 · 폴링 주기는 `DEFAULT_STORE_SOURCE_WINDOW`(`chartChannelTypes.ts:186`, 1시간 / 1분 / `average` / 5초)를 그대로 전개한다. 값을 복제하면 한쪽만 바뀔 때 소스를 갈아탄 사용자가 조용히 다른 창을 보게 된다.

**공유되는 것은 config 형상이 아니라 런타임 계약이다.** 두 소스 모두 최종적으로 `SeriesMatrix` 를 산출하며, 패널은 오직 그것만 소비한다(§1.2.2).

### 2.3 [U3] (Ubiquitous) 소스 종류 키잉 디스패치

시스템은 §1.2.5 의 10개 프로덕션 렌더 디스패치 지점에서 데이터소스 종류에 대한 **하드코딩된 동등 비교를 제거**하고, 소스 종류로 키잉된 단일 조회로 대체한다.

**시스템은 각 지점에 두 번째 동등 비교(`=== 'tsdb'`)를 추가해서는 안 된다.** 그렇게 하면 지점 × 종류로 곱해진다(§1.2.5).

계약의 정본은 신규 순수 모듈이 소유한다.

```ts
// charts/panelDataSource.ts (신규)

export interface PanelSourceBinding {
  kind: ChartDataSourceKind;
  /** 조회 가능한 상태인가(에이전트/시리즈/필터가 갖춰졌는가). */
  active: boolean;
  /** `data_source` 가 인식 불가 값이어서 channel 로 폴백했는가(§2.17-2). */
  unknownKind?: boolean;
}

export function resolvePanelSourceBinding(
  config: Record<string, unknown>,
): PanelSourceBinding;
```

각 소스 종류의 활성 조건은 **한 곳(`panelDataSource.ts`)에만** 존재한다.

| 종류 | 활성 조건 |
|------|-----------|
| `channel` | 항상 활성(채널 훅이 빈 상태를 스스로 처리) |
| `store` | `store_source.series.length > 0` **또는** `selection_mode === 'tag'` 이고 `tag_filters` 가 1개 이상 — **이 표가 정본이다.** 이관은 `LineChartPanel` · `gaugeLegacyBinding` 에 대해서만 동작 동일이며, `StatPanel` · `BarChartPanel` · `PieChartPanel` · `TablePanel` 에는 tag 항이 **가산된다**(v0.3.0 의 "동작 동일" 단언은 거짓이었다 — §HISTORY-0.4.0 (1)) |
| `tsdb` | `tsdb_source.agent_name` 이 비어 있지 않고 **그리고** `tsdb_source.series.length > 0` |

**하위 호환 위험 지점**: 현재 6개 패널의 store 활성 조건은 `(storeSource?.series?.length ?? 0) > 0` 로 동일해 보이지만, `HeatmapPanel.tsx:122` 는 다른 config 키(`heatmapStoreSource`)를 읽고, `gaugeLegacyBinding.ts:122` 는 `series_reduce` 존재까지 논리곱한다. 이관은 위 표의 활성 조건을 정본으로 삼으며 §2.14 의 특성화 테스트가 이형 3지점(히트맵 · 게이지 · 범례 폴백)을 잠근다. **동형으로 분류된 5종 중 4종은 실제로 동형이 아니었다** — `StatPanel` · `BarChartPanel` · `PieChartPanel` · `TablePanel` 은 tag 항 없이 `series.length > 0` 만 보고 있었으므로, 이관 후 tag 모드 config 가 새로 활성화된다. 이 델타는 §2.4 [U4] 의 예외로 명시적으로 수용한다(§HISTORY-0.4.0 (1)). 게이지의 `hasSeriesReduce` 항은 `panelDataSource.ts` 로 옮기지 **않는다** — 그것은 소스 활성이 아니라 게이지 고유의 레거시 우선순위 규칙이며 [SPEC-CHART-002](../SPEC-CHART-002/spec.md) §2.9 가 소유한다.

**시리즈 데이터 훅도 종류로 키잉된다.** 패널은 소스 종류별 훅을 조건 없이 전부 호출하고, 진 쪽에 `undefined` 를 넘겨 idle 로 둔다(`GaugePanel.tsx` 가 이미 쓰는 React 훅 규칙 준수 패턴).

```ts
// charts/usePanelSeriesData.ts (신규)
export function usePanelSeriesData(
  config: Record<string, unknown>,
): UseStoreChartDataResult;   // 기존 결과 형상을 그대로 반환한다
```

반환 형상을 **`UseStoreChartDataResult` 그대로** 두는 것이 이 SPEC 의 두 번째 하중 지지점이다. 6개 패널이 이미 `entries` · `seriesEntries` · `seriesNames` · `seriesStyles` · `booleanSeries` · `status` 를 소비하고 있으므로, 형상을 유지하면 **패널 렌더 코드가 한 줄도 바뀌지 않는다.**

### 2.4 [U4] (Ubiquitous) 하위 호환 — 저장된 config 의 렌더 결과 불변

시스템은 본 SPEC 이전에 저장된 모든 패널 config 에 대해 **바이트 동일한 렌더 결과**를 유지한다.

| `data_source` | 해석 | 근거 |
|---------------|------|------|
| 부재 | `'channel'` | 현행 `?? 'channel'` 폴백 유지 |
| `'channel'` | 채널 경로 | 무변경 |
| `'store'` | Store 경로 | 무변경. 활성 조건은 §2.3 으로 이관되나 **동작 동일** |
| `'tsdb'` | TSDB 경로 | 본 SPEC 이전에는 이 값이 config 에 **물리적으로 존재할 수 없다**(§1.2.3 의 no-op) |
| 인식 불가 문자열 | `'channel'` 로 폴백 + 패널에 경고 표시 | 구버전 클라이언트가 신규 값을 만날 일은 없으나 역방향(신규 config → 구버전 클라이언트)은 발생 가능 |

`'tsdb'` 가 기존 config 에 존재할 수 없다는 사실이 하위 호환의 **구조적 근거**다. §1.2.3 의 토글이 `onConfigChange` 를 호출하지 않았으므로, 저장된 config 의 `data_source` 는 부재 · `'channel'` · `'store'` 셋 중 하나다.

**[SPEC-CHART-002](../SPEC-CHART-002/spec.md) 축과의 합성**: `series_reduce` · `multi_output_limit` · `SeriesTileGrid` 는 `store_source` 가 아니라 `ChartPanelConfigBase` 에 있으므로 **소스 종류와 직교한다.** TSDB 소스 패널도 `series_reduce` 를 지정하면 동일하게 다중 출력이 된다. 시스템은 `series_reduce` 처리 경로를 소스별로 분기해서는 안 된다 — `usePanelSeriesData` 가 `seriesEntries` 를 반환하는 순간 [SPEC-CHART-002](../SPEC-CHART-002/spec.md) 의 `reduceAllSeries` 는 소스를 모른 채 동작한다. `REDUCE_PANEL_TYPES` 게이팅도 소스와 무관하게 유지된다.

동일하게 `gaugeLegacyBinding` 의 레거시 우선순위는 무변경이다. 게이지가 `data_source === 'tsdb'` 인 경우의 판정은 §2.13 [S1] 이 정한다.

> **[U5] (은퇴)** — v0.1.0 §2.5 의 memTSDB 어댑터 요구(G1 `fieldName` · G2 `AbortSignal` · G3 팬아웃)는 은퇴했다. 사유는 §HISTORY-0.2.0 (3) · §1.3. **번호는 재사용하지 않는다.** G2/G3 의 교훈은 §2.19 [U12] 가 TSDB 어댑터 축에서 승계한다.

### 2.6 [U6] (Ubiquitous) InfluxDB 구조화 질의 엔드포인트

시스템은 신규 라우트를 제공한다.

```
POST /api/v1/influxdb/{agent_name}/series/query      권한: store.read
```

기존 `POST /influxdb/{agent_name}/query`(원문 통과)는 **그대로 남으며 변경되지 않는다.** 두 경로는 목적이 다르다 — 원문 통과는 사용자가 쿼리를 쓰는 도구, 구조화 질의는 패널이 쓰는 계약이다.

**요청 — `storeQueryRequest` 와 같은 축(시리즈 1개)이다.**

```go
type influxSeriesQueryRequest struct {
    Bucket      string            `json:"bucket,omitempty"`   // v2=bucket, v3=database. 빈 값이면 에이전트 기본값
    Measurement string            `json:"measurement"`         // 필수
    Field       string            `json:"field"`               // 필수
    Tags        map[string]string `json:"tags,omitempty"`
    StartMs     int64             `json:"start_ms"`
    EndMs       int64             `json:"end_ms"`              // exclusive
    IntervalMs  int64             `json:"interval_ms"`         // > 0
    Aggregation string            `json:"aggregation"`         // min|max|average|first|last
    Fill        string            `json:"fill,omitempty"`      // ""|null|zero|previous
}
```

**응답 — 기존 `chartQueryResponse` 를 그대로 재사용한다.**

```go
// internal/api/handler/store_query.go:231 — 신규 타입을 만들지 않는다
type chartQueryResponse struct {
    Entries   []chartQueryEntry `json:"entries"`
    Count     int               `json:"count"`
    Truncated bool              `json:"truncated"`
}
```

각 `chartQueryEntry` 는 다음을 담는다.

| 필드 | 값 |
|------|-----|
| `timestamp` | **버킷 시작 시각**(epoch ms). 버킷 끝이 아니다(§2.8) |
| `value` | 집계값 |
| `labels` | `{ "__field__": <field>, ...tags }` — Store 와 **동일한 예약 라벨 규약**(`seriesLabels.ts:22`) |

**시스템은 세 번째 응답 형상을 도입해서는 안 된다.** v0.1.0 은 `{columns, rows}` 행렬 응답을 도입하려 했고 OQ3 가 이를 반전했다. 근거는 §4.3.

**요청 1건이 시리즈 1개를 처리한다.** N개 시리즈를 가진 패널은 N회 요청하며, 이는 Store 가 이미 하는 방식(`store.ts:611` `Promise.all(params.keys.map(...))`)과 동일하다. 그 귀결(부분 실패 · 요청 수)은 §2.19 [U12] 와 §5 가 다룬다.

**요청 검증:**

| 조건 | 응답 |
|------|------|
| `measurement` 가 빈 문자열 | 400 |
| `field` 가 빈 문자열 | 400 |
| `interval_ms <= 0` | 400 |
| `end_ms <= start_ms` | 400 |
| `aggregation` 이 5종 밖 | 400 |
| `fill` 이 4종 밖(예: `'avg'`) | 400 (§2.7) |
| 이스케이프 불가 식별자 | 400 (§2.7) |
| 버킷 수가 상한 초과 | 400 (§2.9) |
| 에이전트 없음 | 404 |
| 에이전트가 InfluxDB 가 아님 | 400 |
| 쿼리 타임아웃 | 408 (기존 `defaultInfluxQueryTimeout` = 30초 재사용) |

에이전트 오류 3종의 매핑은 기존 `influxdb_query.go:82-95` 의 규약(`agent_not_found` 404 / `not_an_influxdb_agent` 400 / `DeadlineExceeded` 408)을 그대로 따른다.

### 2.7 [U7] (Ubiquitous) v2 Flux / v3 InfluxQL 생성 분기와 집계 어휘 매핑

시스템은 에이전트의 InfluxDB 버전에 따라 쿼리를 생성한다. 생성 로직은 **문자열 조립을 담당하는 순수 함수**로 분리하며, 네트워크 없이 전수 테스트 가능해야 한다.

**v2 → Flux**

```flux
from(bucket: "<bucket>")
  |> range(start: time(v: <startNs>), stop: time(v: <endNs>))
  |> filter(fn: (r) => r._measurement == "<m>")
  |> filter(fn: (r) => r._field == "<f>")
  |> filter(fn: (r) => r["<tagk>"] == "<tagv>")     // 태그마다 1행
  |> aggregateWindow(every: <interval>, fn: <fluxFn>, createEmpty: <bool>, timeSrc: "_start")
  |> keep(columns: ["_time", "_value"])
```

**`timeSrc: "_start"` 는 선택 사항이 아니다.** `aggregateWindow` 의 기본값은 `timeSrc: "_stop"` 이며, 그 경우 방출되는 `_time` 이 버킷 **끝**이다. `chartQueryEntry.timestamp` 는 버킷 **시작**이어야 하므로(§2.8), 기본값을 쓰면 모든 값이 한 인터벌만큼 미래로 밀린다. 이 한 줄이 §2.8 정렬 계약의 절반이다.

**v3 → InfluxQL** (v3 는 Flux 를 지원하지 않으며 SQL 은 HTTP 로 도달 불가 — §1.2.9)

```sql
SELECT <IQLFN>("<f>") FROM "<m>"
 WHERE time >= '<startRFC3339Nano>' AND time < '<endRFC3339Nano>'
   AND "<tagk>" = '<tagv>'
 GROUP BY time(<interval>) FILL(<fill>)
```

**v3 의 `bucket` 은 질의에 반영되지 않는다.** InfluxQL 의 `FROM "<m>"` 에는 database 를 담을 자리가 없으며, v3 에서 database 는 클라이언트 연결에 바인딩되어 있다. 따라서 `influxSeriesQueryRequest.bucket` 은 **v2(Flux `from(bucket:)`)에서만 유효**하고 v3 에서는 무시되어 에이전트 기본 database 가 사용된다. 선택 UI 는 v3 에이전트에서 bucket 입력의 이 한계를 사용자에게 드러내야 한다(§2.13 능력 게이팅).

**집계 어휘 매핑 — 정본 표**

| `aggregation` | Flux `fn` | InfluxQL | Store 백엔드 |
|--------------|-----------|----------|--------------|
| `min` | `min` | `MIN` | `min` |
| `max` | `max` | `MAX` | `max` |
| `average` | `mean` | `MEAN` | `avg` |
| `first` | `first` | `FIRST` | **미지원**(클라이언트 폴백) |
| `last` | `last` | `LAST` | **미지원**(클라이언트 폴백) |

**`average → mean` 은 매핑 표의 유일한 이름 불일치이며 조용한 오답의 원천이다.** Flux 에는 `average` 함수가 없고, InfluxQL 에는 `AVG` 가 없다(`MEAN` 이다). 매핑을 빠뜨리면 쿼리가 실패하거나(Flux) 파서 오류가 난다(InfluxQL). 매핑은 **컴파일 시 전수성이 강제되는 형태**(Go: 5개 케이스를 명시한 `switch` + `default:` 오류)로 구현한다.

`first` / `last` 가 Store **백엔드**에서 미지원인 것은 기존 사실이며(`store.ts:377-389` 의 `toBackendAggregation` 이 `null` 을 반환해 클라이언트 집계로 폴백한다) 본 SPEC 이 바꾸지 않는다. **TSDB 는 두 값을 백엔드에서 직접 지원**하므로 능력 표(§2.13)에서 차이가 드러난다.

**`fill` 매핑**

| 요청 | Flux | InfluxQL |
|------|------|----------|
| `""` (생략) | `createEmpty: false` | `FILL(none)` |
| `"null"` | `createEmpty: true` | `FILL(null)` |
| `"zero"` | `createEmpty: true` + 후처리 `fill(value: 0.0)` | `FILL(0)` |
| `"previous"` | `createEmpty: true` + `fill(usePrevious: true)` | `FILL(previous)` |
| `"avg"` | **미지원 → 400** | **미지원 → 400** |

`"avg"` fill 은 `SeriesMatrixQuery.fill` 유니온에 남아 있는 값이지만 InfluxDB 양쪽 모두 대응물이 없다. 조용히 다른 전략으로 대체하지 않고 거부한다.

**식별자 이스케이프 — 필수.** measurement · field · tag key · tag value 는 모두 사용자 데이터에서 유래한다. 시스템은 생성 전에 이스케이프해야 하며, 이스케이프 불가능한 문자를 포함하면 400 으로 거부한다. 현재 트리에는 `escapeInfluxQLIdent`(`internal/migrate/tsdbtags/client_v3.go`)만 존재하고 **Flux 문자열 리터럴 이스케이프는 없다.**

### 2.8 [U8] (Ubiquitous) 버킷 원점 정렬 계약

시스템은 모든 **패널 데이터소스**가 **동일한 버킷 경계**를 산출하도록 다음을 강제한다.

> 버킷 시작 시각 = `floor(timestampMs / intervalMs) * intervalMs` (UTC epoch 0 기준)
> 엔트리 `timestamp` = 버킷 **시작** 시각 (끝이 아니다)

| 소스 | 현재 동작 | 조치 |
|------|-----------|------|
| Store | `(tsMs / intervalMs) * intervalMs` — **epoch-zero, 시작 레이블**. `store_query.go:574` 주석: "epoch-zero 정렬: 사용자 시작 시각과 무관하게 벽시계 경계에 맞춘다" | **정본.** 무변경 |
| InfluxDB v2 | 없음 | `aggregateWindow` 는 기본적으로 epoch 정렬이나 레이블이 `_stop` 이다 → `timeSrc: "_start"` 로 고정(§2.7) |
| InfluxDB v3 | 없음 | InfluxQL `GROUP BY time(d)` 는 기본 offset 0 = epoch 정렬 + 시작 레이블 → 그대로 부합. `offset` 인자를 **지정하지 않는다** |

**두 소스는 모든 인터벌에서 일치한다.** v0.1.0 은 memTSDB 의 `time.Truncate`(Go 영시각 기준, `internal/tsdb/query.go:312`)가 `86400 % intervalSec != 0` 인 인터벌에서 어긋난다는 사실을 발견하고, 그 대응으로 UI 인터벌 선택지를 제한하려 했다(OQ9). **정정된 모델에서 memTSDB 는 패널 소스가 아니므로 그 불일치는 패널에서 도달 불가능하다.** 따라서 인터벌 제한은 불필요하며 은퇴한다.

memTSDB 의 `time.Truncate` 동작 자체는 여전히 존재하고 플로우 노드 경로에서 유효하다. 본 SPEC 은 그것을 고치지도 문서화 대상으로 삼지도 않는다(§1.3).

**시스템은 클라이언트에서 버킷 경계를 재계산해서는 안 된다.** 정렬은 서버가 소유한다. 클라이언트가 하는 일은 **버킷 합집합 수집과 컬럼 배치**뿐이며(`store.ts:640-654`), 이는 경계 계산이 아니다. 클라이언트가 `floor(ts/interval)` 을 수행하면 소스별로 보정 규칙이 갈리고, 그 규칙은 테스트하기 어려운 곳(렌더 경로)에 놓인다.

### 2.9 [U9] (Ubiquitous) 버킷 수 · 시리즈 수 가드

| 축 | 상한 | 강제 위치 | 근거 |
|----|------|-----------|------|
| 버킷 수 = `ceil((end_ms - start_ms) / interval_ms)` | **100,000** | **서버**(구조화 질의 핸들러). 초과 시 400 | Store 의 `maxAggregationBuckets = 100_000`(`store_query.go:24`)과 **같은 상수를 쓴다** |
| 시리즈 수 | **48** | **클라이언트**(선택 UI). 초과 선택 불가 | Store 의 `STORE_SERIES_LIMIT = 48` 과 동일 |

시리즈 수 상한이 서버가 아니라 클라이언트에 있는 이유는 요청 1건이 시리즈 1개를 처리하기 때문이다(§2.6) — 서버는 셀 대상이 없다. v0.1.0 은 요청이 N개 시리즈를 담는 형상이었으므로 서버 가드를 두었다.

두 상한을 Store 와 **같은 값**으로 두는 이유는 소스를 바꿨을 때 "왜 이건 되고 저건 안 되지"가 생기지 않게 하기 위함이다. 상한이 다르면 그 차이 자체가 사용자에게 설명되어야 하는데, 설명할 이유가 없다.

거부 메시지는 초과한 축과 실제 값 · 상한을 포함한다. `too many buckets` 만으로는 사용자가 시간창을 줄여야 할지 인터벌을 늘려야 할지 알 수 없다.

### 2.10 [U10] (Ubiquitous) 스키마 디스커버리

시스템은 TSDB 선택 UI 가 필요로 하는 4종 디스커버리를 제공한다.

| # | 라우트 | 권한 | v2 구현 | v3 구현 | 출처 |
|---|--------|------|---------|---------|------|
| D1 | `GET /influxdb/{agent_name}/measurements` | `store.read` | `schema.measurements()` | `SHOW MEASUREMENTS` | **기존 라우트.** v3 는 현재 501 → 본 SPEC 이 해소 |
| D2 | `GET /influxdb/{agent_name}/tag-keys?measurement=<m>` | `store.read` | `schema.measurementTagKeys()` | `SHOW TAG KEYS FROM` | 신규 라우트, 기존 구현 승격. **`schema.tagKeys()` 가 아니다** — 라우트가 measurement 한정이므로 버킷 전역 조회를 쓰면 안 된다(§HISTORY-0.4.0 (3)) |
| D3 | `GET /influxdb/{agent_name}/tag-values?measurement=<m>&tag_key=<k>` | `store.read` | `schema.measurementTagValues()` | `SHOW TAG VALUES FROM ... WITH KEY =` | 신규 라우트, 기존 구현 승격 |
| D4 | `GET /influxdb/{agent_name}/field-keys?measurement=<m>` | `store.read` | `schema.measurementFieldKeys()` | `SHOW FIELD KEYS FROM` | **신규 구현 — 트리에 대응물이 전혀 없다** |

**D1~D3 는 이관(lift-and-shift)이고 D4 만 신규 작성이다.** `internal/migrate/tsdbtags/`(§1.2.10)의 `SchemaClient` 구현이 D1~D3 의 v2·v3 쿼리 문자열과 응답 파싱을 이미 갖고 있으며 테스트도 존재한다. 본 SPEC 은 그 능력을 `internal/agent/system` 의 InfluxDB 클라이언트 인터페이스로 승격하고 핸들러를 붙인다.

승격 방식은 **인터페이스 확장**이며 `internal/migrate/tsdbtags/` 패키지를 API 계층이 import 하지 않는다(§4.5).

**D1 의 v3 501 해소는 부수 효과이며 회귀 위험이 있다.** 현재 v3 에서 `GET /measurements` 는 501 을 반환하고, 그 동작에 의존하는 테스트가 있다. 본 SPEC 은 그 기대값을 **의도적으로 갱신**하며, 나머지 5종 관리 조작(bucket 생성/삭제/truncate · measurement 삭제)의 501 은 **그대로 유지**한다(§1.3).

시스템은 디스커버리 응답을 **캐시해서는 안 된다.** 스키마는 쓰기에 따라 변하며, 오래된 목록에서 고른 시리즈는 조회 시 빈 결과가 된다.

### 2.11 [E1] (Event-driven) 데이터소스 토글이 config 에 영속된다

**When** 사용자가 패널 설정의 데이터 소스 토글에서 TSDB 를 선택하면, **the system shall** `onConfigChange({ data_source: 'tsdb', ... })` 를 호출해 선택을 config 에 기록하고, 처음 전환이면 `defaultTsdbSource()` 블록을 함께 채운다.

이는 `ChartPanelSections.tsx:320` 의 로컬 `tsdbMode` 상태를 **제거**한다는 뜻이다(현재 9개 참조). 모드의 단일 소스 오브 트루스가 `useState` 에서 `config.data_source` 로 이동한다. 토글의 3종 구조(`:372`)는 **무변경**이며, TSDB 버튼의 `onClick` 이 `setTsdbMode(true)` 에서 `setDataSource('tsdb')` 로 바뀐다.

**[SPEC-PANEL-SETTINGS-001](../SPEC-PANEL-SETTINGS-001/spec.md) 대체 선언.** 그 SPEC 의 REQ-05("TSDB 를 실제로 바인딩·질의하지 **않아야 한다**", 비목표 "TSDB 데이터소스의 실제 바인딩·질의·렌더")는 본 SPEC 으로 대체된다. 이를 잠근 인수 테스트는 다음과 같이 **제자리에서 반전**한다.

| 파일 | 현재 단언 | 반전 후 |
|------|-----------|---------|
| `PanelSettingsDataSource.test.tsx:138` (`'TSDB 선택 시 placeholder + 선택 테이블 미노출 + Store config 보존(AC-05)'`) | `expect(onConfigChange).not.toHaveBeenCalled()` + placeholder 존재 | `expect(onConfigChange).toHaveBeenCalledWith(expect.objectContaining({ data_source: 'tsdb' }))` + placeholder **부재** + TSDB 선택 UI 렌더 |

시스템은 이 테스트를 **삭제해서는 안 된다.** 테스트 본문에 반전 사유와 대체 SPEC ID 를 주석으로 남긴다. 삭제하면 "이 동작이 왜 바뀌었는가"의 기록이 diff 밖으로 사라진다.

> 주의 — `expect(onConfigChange).not.toHaveBeenCalled()` 는 이 파일에 3건(`:138` · `:281` · `:991`) 존재한다(재확인함). **반전 대상은 `:138` 하나뿐이다.**

**전환은 비파괴여야 한다.** `store` → `tsdb` 전환 시 기존 `store_source` 는 config 에 그대로 남는다(§2.12).

### 2.12 [E2] (Event-driven) 소스 전환 시 기존 설정 보존

**When** 사용자가 데이터 소스를 다른 종류로 전환하면, **the system shall** `data_source` 값만 갱신하고 다른 소스의 설정 블록(`store_source` · `tsdb_source` · 채널 설정)을 **삭제하지 않는다.**

되돌리기가 가능해야 사용자가 전환을 시도한다. 파괴적 전환은 "다른 소스를 한번 눌러보는" 행위를 되돌릴 수 없는 결정으로 만든다.

이는 `data_source === 'store'` 인 상태에서 채널 설정이 유지되는 현행 동작(`PanelSettingsDialog.tsx:603` 주석: "공존, 하위 호환")의 연장이다.

**When** 사용자가 처음으로 TSDB 를 선택하고 `tsdb_source` 가 config 에 없으면, **the system shall** `defaultTsdbSource()` 블록을 생성한다. 조회 창 기본값은 `DEFAULT_STORE_SOURCE_WINDOW` 를 전개하며(§2.2), `backend` 는 `'influxdb'`, `agent_name` 은 빈 문자열(= 미선택 → 비활성), `series` 는 빈 배열이다.

### 2.13 [S1] (State-Driven) 소스별 능력 게이팅

**While** 특정 데이터소스가 선택되어 있는 동안, **the system shall** 그 소스가 지원하지 않는 설정 선택지를 비활성으로 표시하고 사유를 제공한다. **숨기지 않는다** ([SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) §4.2 원칙 승계).

| 설정 | `channel` | `store` | `tsdb` (influxdb 백엔드) |
|------|-----------|---------|--------------------------|
| 집계 `min`/`max`/`average` | — | O | O |
| 집계 `first`/`last` | — | O (클라이언트 집계 폴백) | O (백엔드 직접) |
| `fill: null/zero/previous` | — | **X** (Store 백엔드 미지원, 무시됨) | O |
| `fill: avg` | — | X | **X** (§2.7 → 400) |
| 에이전트 선택 | — | O | **O — 필수**(§2.18) |
| bucket/database 선택 | — | X | O (v2 는 목록, v3 는 자유 입력 — OQ10) |
| 태그 기반 동적 바인딩(`selection_mode:'tag'`) | — | O | X (OQ6) |
| 시리즈 이름 형식 | — | O | O |
| `series_reduce`(SPEC-CHART-002) | — | O | O |

능력 표는 `Record<ChartDataSourceKind, SourceCapabilities>` 로 두어 **컴파일러가 3종 전수성을 강제**하게 한다.

**While** InfluxDB v3 에이전트가 선택되어 있는 동안, **the system shall** bucket/measurement **관리** 조작(생성 · 삭제 · truncate)을 비활성으로 표시하고 "InfluxDB 3.x 는 관리 조작을 지원하지 않습니다"로 안내한다. **디스커버리(§2.10)는 v3 에서도 활성이다.**

**While** `data_source` 가 `'tsdb'` 인 게이지 패널인 동안, **the system shall** [SPEC-CHART-002](../SPEC-CHART-002/spec.md) §2.9 의 게이지 판정 규칙을 **소스 종류만 넓혀 그대로 적용**한다. 즉 `(data_source ∈ {store, tsdb}) AND (해당 소스 블록 활성) AND (series_reduce 지정)` 일 때만 레거시 `dataSources[]` 를 밀어낸다. 판정 진리표의 나머지 축은 무변경이다.

### 2.14 [S2] (State-Driven) 조회 실패 · 부분 실패 · 빈 결과

**While** 데이터소스 조회가 실패한 동안, **the system shall** 마지막 성공 렌더를 파괴하지 않고 `status:'error'` 만 갱신한다(현행 `useStoreChartData` 동작 승계).

시스템은 다음 네 상태를 화면에서 **서로 구분 가능하게** 표시해야 한다.

| 상태 | 의미 | 표시 |
|------|------|------|
| 빈 선택 | 시리즈를 0개 골랐다 / 에이전트를 고르지 않았다 | 빈 상태 안내 + 설정 유도 |
| 빈 결과 | 골랐으나 그 구간에 데이터가 없다 | 빈 차트(오류 아님) |
| 부분 실패 | 일부 시리즈만 실패(§2.19) | 성공 시리즈 렌더 + 실패 개수 배지 |
| 전체 실패 | 조회 자체가 실패 | 오류 오버레이 |

"빈 결과"와 "전체 실패"를 같은 글리프로 표시하면 사용자가 센서 단선을 정상으로 오독한다([SPEC-CHART-002](../SPEC-CHART-002/spec.md) §4.3 과 같은 원칙).

### 2.15 [O1] (Optional) 선택 기능

- **가능하면** TSDB 시리즈 선택 UI 를 `SeriesSelectTable`(`web/src/pages/agents/SeriesSelectTable.tsx`)로 구현해 Store 선택 표와 조작감을 일치시킨다.
- **가능하면** 설정 화면의 라이브 미리보기가 TSDB 소스에서도 실제 데이터를 렌더한다(`isStoreLinePreview` 선례).
- **가능하면** TSDB 시리즈 선택 시 `measurements`(D1) → `field-keys`(D4) → `tag-values`(D3) 3단 드릴다운을 제공한다.
- **가능하면** 생성된 Flux/InfluxQL 을 개발자 도구에서 확인할 수 있게 응답에 `debug_query` 필드를 선택적으로 포함한다(에이전트의 기존 `debug: true` 옵션과 연동 — OQ11).
- **가능하면** `resolveStoreAgentName` 을 `resolveAgentName` 으로 개명하고 Store/TSDB 양쪽이 같은 함수를 부르게 한다. 시그니처가 이미 소스 중립이므로(§1.2.7) 개명만으로 충분하다.

### 2.16 [UB1] (Unwanted-Behavior) 금지 동작

시스템은 다음을 허용하지 않는다.

| # | 금지 동작 | 대응 |
|---|-----------|------|
| 1 | 디스패치 지점에 두 번째 하드코딩 동등 비교(`=== 'tsdb'`)를 추가 | 소스 종류 키잉 조회로 대체(§2.3) |
| 2 | **백엔드마다 config 블록을 쪼개기**(`tsdb_source` / `influx_source` 분리) | 단일 블록 + 판별자(§2.2). **v0.1.0 에서 반전됨** |
| 3 | 소스 전환 시 다른 소스의 설정 블록 삭제 | 전량 보존(§2.12) |
| 4 | TSDB 시리즈에서 `field` 를 생략하고 "첫 번째 숫자 필드" 같은 폴백에 의존 | `field` 는 `TsdbSeriesRef` 의 필수 필드이며 요청에도 필수(§2.2 · §2.6). 폴백은 조용한 오답이다 |
| 5 | TSDB 어댑터에서 `AbortSignal` 폐기 | 각 요청에 전달 필수(§2.19) |
| 6 | 1개 시리즈 실패로 패널 전체를 비움 | `allSettled` 부분 실패 격리(§2.19) |
| 7 | `aggregateWindow` 를 기본 `timeSrc`(=`_stop`)로 생성 | `timeSrc: "_start"` 고정(§2.7). 전 데이터가 한 인터벌 미래로 밀린다 |
| 8 | `average` 를 Flux `average` / InfluxQL `AVG` 로 생성 | `mean` / `MEAN` 매핑(§2.7). 두 이름은 존재하지 않는다 |
| 9 | 이스케이프 없이 measurement/field/tag 를 쿼리 문자열에 삽입 | 이스케이프 필수, 불가 시 400(§2.7) |
| 10 | 클라이언트에서 버킷 **경계를 계산**(`floor(ts/interval)`) | 서버 소유(§2.8). 합집합 수집은 경계 계산이 아니므로 허용 |
| 11 | 버킷 수 상한 없이 InfluxDB 질의 실행 | 100,000 강제(§2.9) |
| 12 | 디스커버리 응답 캐시 | 캐시 금지(§2.10) |
| 13 | AC-05 테스트 삭제 | 사유 주석과 함께 제자리 반전(§2.11) |
| 14 | `data_source: 'store'` 저장 config 의 렌더 결과 변경 | 특성화로 고정(§2.4) |
| 15 | 기존 `POST /influxdb/{agent}/query` 원문 통과 경로의 계약 변경 | 신규 경로로 분리(§2.6) |
| 16 | v3 의 bucket/measurement **관리** 조작 501 해제 | 디스커버리만 해소(§2.10) |
| 17 | `fill: 'avg'` 를 InfluxDB 에서 다른 전략으로 조용히 대체 | 400 거부(§2.7) |
| 18 | 데이터 0행 시리즈의 컬럼 자리를 제외 | 자리 유지. 책임은 클라이언트 어댑터에 있으며 `store.ts:626-631` 의 동작을 승계한다. 인덱스 매칭이 어긋난다 |
| 19 | TSDB 소스 패널에서 `series_reduce` 경로를 별도 분기로 재구현 | 소스와 직교(§2.4) |
| 20 | `web/src/hooks/useTsdb.ts`(사장 코드)를 신규 경로의 기반으로 사용 | 소비자 0건. `seriesDataSource` 계약만 사용(§1.3) |
| 21 | **memTSDB(`internal/tsdb/` · `/api/v1/tsdb/*`)를 패널 데이터소스로 배선** | memTSDB 는 플로우 노드 · WS 구독자용 내부 설비다(§1.2.1 · §1.3) |
| 22 | config 에 기록된 `backend` 값을 질의 라우팅의 정본으로 사용 | 정본은 참조된 에이전트의 실제 타입(§2.18) |
| 23 | `chartQueryResponse` 외 세 번째 응답 형상 도입 | 평탄 형상 재사용(§2.6 · §4.3) |
| 24 | `'tsdb'` → `'memtsdb'` 개명 시 에이전트 상세 Series 탭의 **동작** 변경 | 식별자 개명만. 화면 동작 무변경(§2.1) |
| 25 | 피벗 로직을 TSDB 용으로 별도 구현 | `groupEntriesBySeries` + 버킷 합집합 피벗 재사용(§1.2.8 · §4.3) |

### 2.17 [UB2] (Unwanted-Behavior) 상태 불일치

1. `data_source: 'tsdb'` 인데 `tsdb_source` 가 없으면 패널은 **빈 선택 상태**를 보인다. `'channel'` 로 조용히 폴백하지 않는다 — 사용자가 명시한 선택을 뒤집으면 토글이 고장난 것으로 보인다.
2. `data_source` 가 인식 불가 문자열이면 `'channel'` 로 폴백하되 패널에 경고를 표시한다(§2.4). 이는 1번과 다르다 — 인식 불가 값은 사용자가 명시할 수 없는 상태이므로 폴백이 안전하다.
3. `tsdb_source.agent_id` 가 현재 에이전트 목록에 없으면 `agent_name` 으로 폴백한다(SPEC-WEB-006 규약). 그 이름도 404 면 패널은 마지막 성공 렌더를 유지하고 오류 오버레이로 사유를 표시하며, **무한 재시도하지 않는다**(재시도 간격이 폴링 주기를 넘지 않는다).
4. `tsdb_source.bucket` 이 존재하지 않는 버킷을 가리키면 InfluxDB 는 오류가 아니라 **빈 결과**를 반환한다. 이는 §2.14 의 "빈 결과"로 표시되며, 사용자가 오타를 알아채기 어렵다. 시스템은 **가능하면** bucket 목록(v2)에서 선택하게 해 자유 입력을 줄인다.
5. 시리즈 선택 후 스키마가 바뀌어 measurement/field 가 사라지면 해당 컬럼은 전 버킷 `null` 이 된다. 이는 "빈 결과"이며 오류가 아니다.
6. 부분 실패 상태에서 다음 폴링이 전부 성공하면 실패 배지는 사라져야 한다. 실패 표시가 남아 있으면 사용자는 복구를 인지하지 못한다.
7. 참조된 에이전트가 지원 백엔드가 아니면(예: `tsdb_source.agent_id` 가 `store` 에이전트를 가리킴) 시스템은 **명시적 백엔드 불일치 오류**를 표시한다. 조용히 Store 로 질의하지 않는다(§2.18).

### 2.18 [U11] (Ubiquitous) 백엔드는 참조된 에이전트에서 파생한다

시스템은 TSDB 소스의 백엔드를 **참조된 에이전트의 실제 타입**으로 결정한다. 이는 쓰기 경로의 `newStorageBackend`(`internal/node/storage_write.go:190`)와 **같은 축**이다.

```
tsdb_source.agent_id
  → resolveAgentName(agent_id, agent_name, agents)   // SPEC-WEB-006, §1.2.7
  → 해석된 에이전트의 type
  → 'influxdb' → InfluxDB 어댑터(/influxdb/{name}/series/query)
  → 그 외      → 백엔드 불일치 오류
```

**시스템은 능력(interface/필드 존재) 기반으로 백엔드를 추정해서는 안 된다.** `storage_write.go:139-142` 의 주석이 그 이유를 이미 적었다 — Store 와 InfluxDB 에이전트가 둘 다 `Process([]byte)` 를 가지므로 능력 단언이 양쪽에 성립한다. 읽기 경로도 동형이다.

**config 의 `backend` 값과 에이전트의 실제 타입이 어긋나면:**

| 상황 | 처분 |
|------|------|
| `backend: 'influxdb'`, 에이전트 타입 = `influxdb` | 정상 |
| `backend: 'influxdb'`, 에이전트 타입 = 다른 **지원** 백엔드 | **에이전트 타입이 이긴다.** 질의는 실제 타입으로 라우팅하고, 설정 UI 는 불일치 배너와 함께 `backend` 재동기화를 제안한다. 시리즈 참조(`key`/`field`/`tags`)는 중립 어휘이므로 보존하되, 해석되지 않으면 §2.14 의 "빈 결과"로 나타난다 |
| `backend: 'influxdb'`, 에이전트 타입 = **미지원**(예: `store`, `mqtt-client`) | **백엔드 불일치 오류**(§2.17-7). 질의하지 않는다 |
| 에이전트 자체가 해석 불가(삭제됨) | §2.17-3 |

`backend` 필드를 config 에 남겨 두는 이유는 두 가지다 — (a) 에이전트 목록이 로드되기 전에 설정 UI 가 어느 백엔드용 컨트롤을 그릴지 알아야 하고, (b) 불일치를 **감지**하려면 기록된 값이 있어야 한다. 필드를 없애면 불일치가 감지 불가능해지고 조용한 오작동이 된다.

### 2.19 [U12] (Ubiquitous) 시리즈당 1요청의 귀결 — 취소 전파와 부분 실패 격리

요청 1건이 시리즈 1개를 처리하므로(§2.6), N개 시리즈 패널은 폴링마다 N회 요청한다. 시스템은 그 귀결 둘을 명시적으로 처리한다.

| # | 귀결 | 규칙 |
|---|------|------|
| 1 | 폴링 중 config 가 바뀌면 이전 요청이 남는다 | 어댑터는 `AbortSignal` 을 **각 요청에 전달**하고, 취소된 요청의 결과를 상태에 반영하지 않는다 |
| 2 | 1개 요청 실패가 전체를 없앨 수 있다 | `Promise.allSettled` 로 격리한다. 실패한 시리즈의 컬럼은 전 버킷 `null` 이 되고 나머지는 정상 렌더된다. 실패한 컬럼 인덱스를 상위에 전달한다 |

**시스템은 부분 실패를 성공으로 보고해서는 안 된다.** `usePanelSeriesData` 는 이를 `status:'error'` 가 아닌 별도의 부분 실패 신호로 표면화하고(§2.14), 다음 폴링이 전부 성공하면 신호를 해제한다(§2.17-6).

시리즈 상한이 48 이므로 최악의 경우 폴링마다 48회 요청이다. 이는 Store 가 이미 하는 일과 동일하지만 InfluxDB 쿼리는 Store 조회보다 무겁다. 이 비용의 처분은 §7 NQ2 가 열어 둔다.

> **참고** — 현행 Store 어댑터(`store.ts:611`)는 `Promise.all` 을 쓰므로 부분 실패 격리가 **없다.** 본 SPEC 은 Store 를 바꾸지 않으며(§1.3), TSDB 어댑터에만 `allSettled` 를 요구한다. 두 소스의 이 차이는 의도적이며, Store 쪽 정렬은 별도 SPEC 이 다룬다.

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 어휘 분리 + `'memtsdb'` 개명 | `services/api/seriesDataSource.ts`, `services/api/tsdb.ts`, `pages/agents/AgentDetailPanel.tsx`, `lib/i18n/{ko,en}.json` | AC-01, AC-02 |
| U2 계약 타입(단일 블록) | `charts/chartChannelTypes.ts`(`TsdbSourceConfig` · `TsdbSeriesRef` · `TsdbBackend`) | AC-03, AC-04 |
| U3 디스패치 일반화 | `charts/panelDataSource.ts`(신규), `charts/usePanelSeriesData.ts`(신규), §1.2.5 의 10개 지점 | AC-05 ~ AC-08 |
| U4 하위 호환 | 6개 패널 + `previewSeries.ts` + `PanelSettingsDialog.tsx` | AC-09 ~ AC-12 (특성화) |
| ~~U5~~ | **은퇴** | — |
| U6 구조화 엔드포인트 | `internal/api/handler/influxdb_series.go`(신규), `internal/api/dto/influxdb.go` | AC-17 ~ AC-20 |
| U7 쿼리 생성 + 어휘 매핑 | `internal/agent/system/influxdb_seriesquery.go`(신규, 순수 함수) | AC-21 ~ AC-25 |
| U8 버킷 정렬 | 동일 + `internal/api/handler/store_query.go`(정본, 무변경) | AC-26, AC-27 |
| U9 가드 | `internal/api/handler/influxdb_series.go`, `PanelSettingsDataSource.tsx`(시리즈 48) | AC-28 |
| U10 디스커버리 | `internal/agent/system/influxdb_schema.go`(신규), `internal/api/handler/influxdb_management.go`(확장) | AC-29 ~ AC-33 |
| U11 백엔드 파생 | `services/api/tsdbSource.ts`(신규), `charts/useTsdbChartData.ts`(신규) | AC-50, AC-51 |
| U12 취소·부분 실패 | `services/api/tsdbSource.ts`, `charts/usePanelSeriesData.ts` | AC-52 |
| E1 토글 영속 + AC-05 반전 | `ChartPanelSections.tsx:320`(로컬 상태 제거), `PanelSettingsDataSource.test.tsx:138` | AC-34, AC-35 |
| E2 전환 비파괴 | `ChartPanelSections.tsx`(`setDataSource`) | AC-36 |
| S1 능력 게이팅 | `ChartPanelSections.tsx`, `charts/panelDataSource.ts`(능력 표) | AC-37, AC-38 |
| S2 실패 상태 구분 | `charts/usePanelSeriesData.ts`, 패널 오버레이 | AC-39, AC-40 |
| O1 선택 기능 | `SeriesSelectTable.tsx` 재사용, 미리보기 | (선택) |
| UB1 금지 동작 | 전 파일 | AC-41 ~ AC-46, AC-53 |
| UB2 상태 정합 | 패널 빈/오류 분기 | AC-47, AC-48, AC-54 |

---

## 4. 설계 결정

### 4.1 계약 축과 백엔드 축을 분리하고 병렬로 진행한다

v0.1.0 은 "계약 → memTSDB 로 입증 → InfluxDB" 순서를 택했고, 근거는 *"memTSDB 어댑터는 이미 프로덕션에서 `SeriesMatrix` 를 반환하므로 계약 결함만 단독으로 드러난다"* 였다. **그 전제가 거짓이다**(§1.2.1 — 라우트 미등록). 그리고 정정된 모델에서 memTSDB 는 패널 소스가 아니므로, 그 어댑터를 꽂아 보는 행위 자체가 제품이 필요로 하지 않는 일이다.

재검토한 세 순서:

| 순서 | 결과 |
|------|------|
| InfluxDB 배선 먼저, 계약은 나중 | 10개 지점에 `=== 'tsdb'` 를 임시로 추가하게 되고, 그 임시 코드를 나중에 다시 걷어내야 한다. 지점 × 종류 증가를 한 번 겪는다 |
| 계약과 InfluxDB 배선을 한 커밋에 | 패널이 안 그려질 때 원인이 계약 일반화 회귀인지 InfluxDB 결함인지 구분 불가. 특성화 테스트가 있어도 **두 변경이 섞인 커밋**은 이등분 탐색을 무력화한다 |
| **계약 축(무동작 리팩터) → 백엔드 축과 합류** | 계약 축의 산출물은 **렌더 결과가 바이트 동일한 리팩터**이며 특성화 21건이 그것을 잠근다. 회귀가 나면 원인이 계약 축임이 즉시 확정된다. 백엔드 축(Go)은 파일이 겹치지 않아 병렬 진행 가능 |

세 번째를 택한다. **단, v0.1.0 이 주장한 "중간 단계의 독립적 사용자 가치"는 없다.** 계약 축이 끝나도 사용자에게 보이는 변화는 0이며, 첫 가치는 합류 지점에서 나온다. 이 사실을 숨기지 않고 적는 이유는, 중간 마일스톤을 "사용자 가치"로 정당화하면 그것이 지연될 때 잘못된 우선순위 판단을 하게 되기 때문이다.

### 4.2 백엔드는 에이전트에서 파생하고, config 는 그것을 기록만 한다

§2.18 이 규칙이고 여기가 근거다. 세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| config 의 `backend` 가 라우팅 정본 | 에이전트를 교체하면 config 가 거짓말을 한다. 사용자는 InfluxDB 를 고른 적이 없는데 InfluxDB 로 질의된다 |
| `backend` 필드를 아예 두지 않고 매번 파생 | 라우팅은 옳지만 **불일치를 감지할 방법이 없다.** 또 에이전트 목록 로드 전에 설정 UI 가 어느 컨트롤을 그릴지 모른다 |
| **에이전트 타입이 라우팅 정본, `backend` 는 기록 + 대조군** | 쓰기 경로(`newStorageBackend`)와 같은 축. 불일치가 감지 가능하고, 낙관적 렌더가 가능하며, 정본이 하나다 |

세 번째를 택한다. 이는 v0.1.0 §4.2 의 결정(*"백엔드가 읽지 않는 필드를 두면 조용한 거짓말"*)과 **모순되지 않는다** — 그때의 문제는 `agent_id` 가 **아무 효과도 없다**는 것이었고, 지금의 `backend` 는 (a) 감지와 (b) 낙관적 렌더라는 실효를 갖는다.

### 4.3 평탄한 `chartQueryResponse` 를 재사용하고 클라이언트에서 피벗한다

v0.1.0 §4.3 은 서버 행렬 응답(`{columns, rows}`)을 택했고 근거 셋을 들었다. OQ3 가 반전했으므로 그 셋을 하나씩 처분한다.

| v0.1.0 근거 | 처분 |
|-------------|------|
| "평탄한 형상은 시리즈 구분을 `labels` 에 의존하는데, InfluxDB 의 시리즈 식별자는 `{measurement, field, tags}` 3요소이므로 `labels` 하나로 인코딩할 수 없다" | **요청 1건 = 시리즈 1개**로 두면 measurement 는 **요청 축**에 있고 `labels` 는 `__field__` + tags 만 담으면 된다. 이는 Store 가 이미 하는 일과 정확히 같다(`storeQueryRequest{Key, Field, Tags}`). 인코딩 문제가 발생하지 않는다 |
| "행렬을 서버가 만들면 버킷 합집합과 정렬이 서버 소유가 된다" | **버킷 *경계* 는 여전히 서버 소유다**(§2.8). 클라이언트가 하는 일은 합집합 수집과 컬럼 배치이며, 그 코드는 `store.ts:640-654` 에 **이미 존재하고 소스를 모른다**. 새로 만드는 것이 아니라 재사용한다 |
| "N개 시리즈를 1회 요청으로 처리한다" | 이 이점은 **실제로 포기한다.** N회 요청이 된다. 대가는 §2.19 와 §5 에 명시하고, 배치 확장 여지는 §7 NQ2 에 남긴다 |

얻는 것:

- **응답 형상이 3개가 아니라 2개로 남는다.** 신규 엔드포인트는 `influxdb_query.go:115` 가 이미 재사용 중인 `chartQueryResponse` 를 그대로 쓴다.
- **`groupEntriesBySeries` · `seriesSignature` · 버킷 합집합 피벗 · 0행 컬럼 자리 보존이 전부 무변경으로 재사용된다.** `seriesLabels.ts` 에 예약 라벨을 추가할 필요조차 없다.
- **`SeriesMatrixQuery` / `SeriesDataSource` 계약이 무변경이다.** `keys[i]` ↔ measurement, `seriesFilters[i]` ↔ `{fieldName, tags}` 로 그대로 맞는다.

`queryStoreMatrix`(`store.ts:589`)의 본문에서 **키별 페처만 주입 가능하게 뽑아내면** TSDB 어댑터는 URL 과 요청 바디만 다른 같은 함수가 된다. 그것이 §2.16 #25 가 요구하는 바다.

### 4.4 (은퇴) memTSDB 의 `time.Truncate` 정렬 처분

v0.1.0 §4.4 는 memTSDB 의 Go 영시각 기준 절삭이 `86400 % intervalSec != 0` 인 인터벌에서 Store 와 어긋난다는 사실을 발견하고, 문서화 + 회귀 테스트 + UI 인터벌 제한으로 대응하려 했다.

**정정된 모델에서 이 결정은 은퇴한다.** memTSDB 가 패널 소스가 아니므로 같은 패널에서 소스만 바꿔 버킷이 밀리는 현상이 발생할 수 없다. Store 와 InfluxDB 는 둘 다 epoch-zero 정렬이므로 **모든 인터벌에서 일치한다**(§2.8).

발견 자체는 유효하며 플로우 노드 경로(`tsdb-query`)에서는 여전히 참이다. 본 SPEC 은 그것을 고치지 않고, 은폐하지도 않으며, 범위 밖으로 명시한다(§1.3).

### 4.5 디스커버리는 인터페이스 확장이며 `tsdbtags` 패키지를 import 하지 않는다

§2.10 에 근거를 적었다. 대가는 쿼리 문자열이 두 곳에 존재하게 된다는 점이다(마이그레이션 도구와 API 계층).

대안은 `tsdbtags.SchemaClient` 를 공용 패키지로 승격하고 양쪽이 import 하는 것이다. 그러나 그 패키지의 존재 이유는 "마이그레이션 도구는 write 하지 않는다"는 컴파일 타임 보장이고, API 계층이 같은 인터페이스를 쓰면 그 좁힘이 API 계층의 필요와 충돌한다(예: API 는 field key 도 필요한데 마이그레이션은 필요 없다). 인터페이스를 넓히면 마이그레이션 쪽 보장이 약해진다.

쿼리 문자열 중복은 **테스트로 관리 가능한 중복**이다. 양쪽 모두 fixture 기반 테스트를 갖고, 어느 한쪽이 바뀌면 그쪽 테스트만 깨진다. 인터페이스 결합은 그렇게 관리되지 않는다.

### 4.6 `average → mean` 매핑을 컴파일 시 전수성이 강제되는 형태로 둔다

§2.7 이 지적한 유일한 이름 불일치다. 5종 유니온을 `map[string]string` 리터럴로 두면 키를 빠뜨려도 컴파일된다. 대신 5개 케이스를 명시적으로 나열하고 `default:` 에서 오류를 반환하는 `switch` 로 구현한다. Go 에는 exhaustive switch 강제가 없으므로 **테스트로 5종 전수 검증**을 병행한다(AC-25).

TypeScript 쪽은 `Record<ChartDataSourceKind, ...>` 로 두면 컴파일러가 전수성을 강제한다(능력 표, §2.13). 이 비대칭(TS 는 타입으로, Go 는 테스트로)은 언어 차이이며 각 언어에서 가장 강한 수단을 쓴 결과다.

### 4.7 부분 실패를 오류가 아닌 별도 상태로 둔다

`Promise.allSettled` 로 바꾸면 "일부 성공"이라는 세 번째 상태가 생긴다. 이를 성공으로 뭉개면 사용자는 시리즈가 왜 비어 보이는지 모르고, 오류로 뭉개면 성공한 47개 시리즈까지 오버레이에 가린다.

별도 상태로 두는 대가는 상태 축이 하나 늘어난다는 점이다(§2.14 의 4상태). 그러나 그 4상태는 이미 개념적으로 존재했고(`useStoreChartData` 가 빈 결과와 오류를 구분한다), 본 SPEC 은 그 사이에 한 칸을 명시적으로 넣을 뿐이다.

### 4.8 `SeriesDataSourceKind` 의 `'tsdb'` 를 개명한다

§2.1 이 규칙이고 여기가 근거다. v0.1.0 가정 1 은 *"본 SPEC 은 `SeriesDataSourceKind` 유니온에 값을 **가산**하는 것 외에는 수정하지 않는다"* 였다. 그 가정은 "`'tsdb'` = memTSDB 가 옳다"는 모델 위에 서 있었다.

세 선택지:

| 선택지 | 결과 |
|--------|------|
| `'tsdb'` = memTSDB 유지, 외부 TSDB 에 `'influxdb'` 배정 | `ChartDataSourceKind.'tsdb'` = 외부, `SeriesDataSourceKind.'tsdb'` = memTSDB. **같은 문자열이 두 화면에서 반대 뜻** — OQ7 이 없애라고 한 모호성 그 자체 |
| 외부 TSDB 에 `ChartDataSourceKind` 만 `'tsdb'` 주고 어댑터 kind 는 `'influxdb'` | 백엔드가 늘 때마다 kind 가 늘어난다. §2.18 의 "백엔드는 파생"과 모순 |
| **memTSDB 를 `'memtsdb'` 로 개명, `'tsdb'` 를 외부에 배정** | 문자열 하나 = 뜻 하나. 개명 지점 5곳, 동작 변경 0 |

세 번째를 택한다. 개명 위험이 낮은 이유는 (a) 지점이 5곳이고, (b) 개명 대상 경로(`/api/v1/tsdb/*`)가 현재 미등록이라 런타임 회귀가 발생할 코드 경로 자체가 없기 때문이다. 그럼에도 §2.16 #24 가 동작 변경을 금지하고 AC-01 이 개명 완결성을 기계 검증한다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 성능 (프론트) | 소스 종류 판정(`resolvePanelSourceBinding`)은 O(1) 순수 함수. 렌더당 1회 `useMemo` |
| 성능 (백엔드) | 구조화 질의 1건은 시리즈 1개를 처리. 엔트리 조립은 O(버킷 수) 단일 패스 |
| 네트워크 | **TSDB 는 시리즈당 1요청**(Store 와 동일 축). 시리즈 48 상한이므로 폴링당 최대 48요청. 이는 v0.1.0 의 "패널당 1요청"에서 **의도적으로 후퇴한 지점**이며 근거는 §4.3, 개선 여지는 §7 NQ2 |
| 하위 호환 | `data_source` 부재 · `'channel'` · `'store'` config 의 렌더 결과가 바이트 동일. 특성화 테스트로 고정 |
| 하위 호환 (백엔드) | 기존 `POST /influxdb/{agent}/query` · 관리 라우트 6종의 요청/응답 계약 무변경. v3 의 measurements 501 만 의도적으로 갱신 |
| 보안 | measurement · field · tag 는 전부 이스케이프. 이스케이프 불가 문자는 400. 생성된 쿼리는 기본적으로 응답에 포함하지 않는다(§2.15 의 `debug_query` 는 선택) |
| 자원 | 버킷 수 100,000(서버) · 시리즈 48(클라이언트). 쿼리 타임아웃 30초(기존 `defaultInfluxQueryTimeout`) |
| 테스트 | 신규 코드 커버리지 85% 이상(`quality.yaml`). 쿼리 생성 순수 함수는 (v2/v3) × (집계 5종) × (fill 5종) × (태그 0/1/N) 조합 테스트. 버킷 정렬은 Store ↔ InfluxDB 교차 검증 |
| 접근성 | 비활성 설정 컨트롤은 `aria-disabled` + 사유 툴팁(§2.13). 부분 실패 배지는 스크린리더에 실패 개수를 전달 |
| 국제화 | 신규 문구(소스 라벨 · 백엔드 표기 · 능력 게이팅 사유 · 부분 실패 · 백엔드 불일치 · 디스커버리 오류)는 `web/src/lib/i18n/{ko,en}.json` 두 로케일 |
| LSP | `tsc --noEmit` 0 에러, eslint 0 에러, `go vet ./...` 0 에러 (전부 v0.2.0 작성 시점 baseline 실측: exit 0) |

---

## 6. 가정 및 제약

1. `SeriesDataSource` / `SeriesMatrix` / `SeriesMatrixQuery` 계약([SPEC-WEB-005](../SPEC-WEB-005/spec.md) 소유)은 안정적이며, 본 SPEC 은 **`SeriesDataSourceKind` 의 값 하나를 개명**하는 것 외에는 수정하지 않는다(§4.8). `SeriesMatrixQuery` 의 축은 InfluxDB 에 그대로 맞으므로 확장이 필요 없다.
2. `UseStoreChartDataResult` 반환 형상은 안정적이며 `usePanelSeriesData` 가 그것을 그대로 재현한다. 6개 패널의 소비 코드는 무변경이다(§2.3).
3. **memTSDB 는 패널 데이터소스가 아니며**, `/api/v1/tsdb/*` 라우트는 현재 프로덕션에 등록되어 있지 않다(§1.2.1). 본 SPEC 기간 중 이 상태가 바뀌더라도 본 SPEC 의 결정은 영향받지 않는다.
4. 패널 config 는 불투명 JSON(`json.RawMessage`)이므로 신규 필드 추가에 백엔드 스키마 변경이 필요 없다([SPEC-DASHBOARD-001](../SPEC-DASHBOARD-001/spec.md) 승계).
5. InfluxDB 에이전트의 v2/v3 판별은 기존 `InfluxDBConfig.Version` 으로 가능하며 런타임에 바뀌지 않는다.
6. InfluxDB v2 는 Flux, v3 는 InfluxQL 로 질의한다. v3 SQL 은 HTTP 계층에서 도달 불가이며 본 SPEC 이 이를 해소하지 않는다(§1.3).
7. Store 의 버킷 정렬(`(tsMs / intervalMs) * intervalMs`, `store_query.go:574`)이 정렬 계약의 정본이며 본 SPEC 이 바꾸지 않는다.
8. 프론트가 에이전트 목록과 각 에이전트의 `type` 을 알 수 있다(`useAgents()` — `useStoreChartData.ts:283` 이 이미 사용 중). §2.18 의 백엔드 파생이 이에 의존한다.
9. 대상 프론트엔드는 React 19 + TypeScript 5.9 + Vitest, 백엔드는 Go 이며 기존 `api.RouteGroup` / `dto.NewSuccessResponse` 패턴을 따른다.
10. 현재 브랜치는 `feature/storage-write-unification` 이며 [SPEC-CHART-002](../SPEC-CHART-002/spec.md) 가 도입한 `series_reduce` · `multi_output_limit` · `SeriesTileGrid` · `gaugeLegacyBinding` 과 `storage-write` 통합(`internal/node/storage_backend_*.go`)이 트리에 존재한다. 본 SPEC 은 전자를 **합성**하고(§2.4) 후자를 **선례로 삼는다**(§1.2.6).
11. `internal/migrate/tsdbtags/` 의 v2/v3 스키마 쿼리 문자열은 검증된 상태이며 본 SPEC 은 그것을 **복제**해 API 계층에 놓는다(§4.5).

---

## 7. 열린 설계 질문

§7 의 OQ1 ~ OQ12 는 **전부 확정되었다**(§HISTORY-0.2.0 (2)). 아래 NQ1 ~ NQ5 는 모델 정정이 **새로 만든** 질문이며, v0.3.0 에서 **전부 잠정안 그대로 확정되었다** — 따라서 "잠정" 열의 값이 곧 확정된 구현 규칙이고, "대안" 열은 후속 SPEC 에서 재검토할 여지를 남긴 기록이다.

| # | 질문 | 잠정 | 대안 |
|---|------|------|------|
| **NQ1** | 읽기 경로도 쓰기 경로처럼 **소스 종류를 하나로 합칠 것인가**? 쓰기 경로는 `storage-write` 노드 하나가 `agent_ref` 로 Store/InfluxDB 를 모두 덮는다(§1.2.6). 읽기 경로를 완전히 대칭으로 만들면 `data_source` 는 `channel \| storage` 2종이 되고 `store_source`/`tsdb_source` 가 한 블록으로 합쳐진다 | **합치지 않는다** — `store_source` 는 이미 배포되어 저장된 config 가 존재하며 §2.4 가 렌더 불변을 요구한다 | 마이그레이션 SPEC 을 별도로 세워 `store_source` → 통합 블록 승격 + 읽기 시 하위호환 어댑터 |
| **NQ2** | 시리즈당 1요청(최대 48회/폴링)의 비용을 언제 어떻게 줄일 것인가? | **줄이지 않는다** — Store 와 같은 축을 유지한다(OQ3 의 일관성 우선 판단) | 같은 엔드포인트가 `series[]` 배열을 받도록 확장. **단 순수 가산이 아니다** — 한 응답에 여러 measurement 가 섞이면 `labels` 에 measurement 예약 키가 필요하고, 그것은 `seriesLabels.ts` 의 `parseSeriesLabels`(모든 비-`__field__` 키를 태그로 간주)를 바꿔야 하므로 Store 표시 이름 경로까지 영향을 준다 |
| **NQ3** | Store 어댑터의 `Promise.all`(부분 실패 격리 없음, `store.ts:611`)을 TSDB 와 같이 `allSettled` 로 맞출 것인가? | **본 SPEC 에서는 맞추지 않는다**(§1.3 — Store 무변경) | 별도 SPEC. 두 소스가 실패 처리에서 다르게 동작하는 상태가 남는다 |
| **NQ4** | `/api/v1/tsdb/*` 6종 라우트가 미등록이라 CLI `tsdb` 하위명령과 에이전트 상세 Series 탭이 404 인 문제를 언제 처리할 것인가? | **본 SPEC 범위 밖**(§1.3). 사실만 기록한다 | (a) `main.go` 에 `tsdbHandler` 배선 (b) 도달 불가 코드로 판정하고 CLI 하위명령 · Series 탭 · `services/api/tsdb.ts` · `hooks/useTsdb.ts` 를 함께 제거 |
| **NQ5** | `resolveStoreAgentName` 을 `resolveAgentName` 으로 개명해 Store/TSDB 가 공유할 것인가? | **개명한다**(§2.15 O1, 선택 항목) | 개명하지 않고 TSDB 가 같은 함수를 그대로 호출(이름만 Store 를 가리키는 상태가 남는다) |

---

## 8. 구현 노트 (2026-09-02 sync)

`lifecycle_level: spec-first` 이므로 위 §1~§7 본문은 **작성 당시 상태 그대로 보존**한다. 이 절은 실제 구현이 계획에서 벗어난 지점만 기록한다 — 본문을 고쳐 쓰면 "무엇을 계획했고 무엇이 실제였는가" 를 나중에 대조할 수 없다.

### 8.1 채널 소스 제거 — AC-09 · AC-12 의 전제 소멸

본문 §2.4 는 `ChartDataSourceKind = 'channel' | 'store' | 'tsdb'` 3종 유니온을 계약으로 두고, `'channel'` 을 인식 불가 값의 폴백 자리로 삼았다. 인수 조건 **AC-09**(저장된 config 3형태의 렌더 결과가 바이트 동일) · **AC-12**(미리보기 범례의 채널 폴백이 보존된다)가 이를 잠갔다.

구현은 채널을 **패널 데이터 소스에서 완전히 걷어냈다**. 유니온은 `'store' | 'tsdb' | 'sysmetrics'` 가 되었고, 폴백 자리는 `'store'` 로 옮겨졌다.

- 옮긴 근거는 두 가지다. (1) 신규 패널이 이미 store 로 태어난다(`uiStore.createDefaultPanel`). (2) 채널 모드로 저장된 패널은 설정을 열 때 store 로 이관되므로(`SERIES_SOURCE_PANEL_TYPES`), 폴백과 이관 대상이 같아야 두 경로가 갈리지 않는다.
- **활성 판정도 함께 바뀌었다.** 채널 폴백은 언제나 활성이었지만(채널 훅이 빈 상태를 스스로 처리했다) store 는 시리즈를 골라야 활성이다. 활성을 참으로 두면 고른 것이 없는 패널이 "조회 중" 으로 보인다.
- 폐지된 `'channel'` 값은 **경고 없이** store 로 접는다. 사용자가 고른 적 없는 손상된 값과 달리, 폐지된 값은 구제 대상이지 오류가 아니다.
- 삭제된 모듈: `useChartChannel` · `useChartChannels` · `services/api/charts` · `services/ws/remoteChartChannel` (+ 각 테스트, 총 9개 파일). 플로우·서버측 채널 인프라는 **건드리지 않았다** — 제거 범위는 패널 옵션에 한정된다.
- AC-09 · AC-12 를 대체하는 특성화는 `PanelSettingsDialog.channelRemoval.test.tsx` 가 갖는다(채널 모드 저장 패널의 store 자동 이관 + 4종 패널의 채널 섹션 부재).

### 8.2 다중 데이터 소스 — 수평 확장 축 추가

본문은 단일 `data_source` 축만 다룬다. 구현은 `config.sources[]` 를 더해 **종류당 최대 4개**(`MAX_SOURCES_PER_KIND`) 인스턴스를 동시에 둘 수 있게 했다.

- §NQ1 이 확정한 "읽기 경로를 하나로 **합치지 않는다**" 와는 다른 축이다. NQ1 은 `store_source`/`tsdb_source` 를 한 블록으로 **수직 통합**할지의 질문이었고, 이번 변경은 같은 종류를 **여러 개 나열**하는 수평 확장이다. 두 소스 블록은 그대로 분리되어 있다.
- 세 세대의 config 를 함께 읽는다: `sources[]` → `data_sources[]` → `data_source`. 구 패널은 단일 축으로 되돌아가므로 렌더가 종전과 같다.
- 소스별 결과는 `mergeSeriesResults` 가 합친다. 계열 이름이 겹칠 때만 접미사를 붙이고, 입력이 하나면 **그 결과를 참조 그대로** 돌려준다(불필요한 리렌더 방지).
- 상태 병합 규칙: `idle` 이 가장 낮고 `error` 가 가장 높다.
- 활성 판정은 목록 전체를 본다(`isPanelSeriesActive`). 단일 축 해석기(`resolvePanelSourceBinding`)로 판정하면 목록 쪽 인스턴스에만 시리즈가 있는 패널이 비활성으로 보인다 — 실제로 "시리즈를 골라도 미리보기가 반영되지 않는" 결함으로 나타났다.
- 종류 버튼은 **더하기 전용**이다. 토글로 두면 같은 버튼을 다시 눌렀을 때 그 소스의 설정이 통째로 사라진다. 지우는 길은 인스턴스 머리의 휴지통 하나뿐이다.

### 8.3 X축 구간의 소유권 이전

가져올 데이터의 구간은 **데이터 소스가 아니라 패널 옵션**이 소유한다(`panelXRange.ts`). 소스마다 구간을 따로 두면 한 그래프 안에서 축이 갈라진다.

`max_points` 는 구간 재정의 판정에서 **빠진다** — bar/pie/stat 에서 그 값은 "막대 개수" 를 뜻하므로, 12막대 패널이 12포인트만 조회하는 결함이 있었다.

### 8.4 범위 밖 추가 — 차트 패널 표시 개편

아래는 어느 SPEC 에도 없는, 사용자 요청으로 이번 사이클에 함께 들어간 항목이다. 요구사항 형태로는 남아 있지 않으므로 목록만 기록한다.

| 항목 | 모듈 |
|---|---|
| 타이틀·범례 디자인 팝오버(폰트·크기·색) | `panelChromeContext.ts`, `ChartPanelSections.tsx` |
| 범례·그래프 영역 자유 좌표 드래그 | `ChartDragLayer.tsx`, `legendOverlay.ts` |
| 그래프 영역 크기 조절 + 기본값 리셋 | `chartLayout.ts` |
| 툴팁 레이블 좌측·값 우측 정렬 | `ChartTooltipContent.tsx` |
| Y축 값 짤림(자동 도메인) | `axisSize.ts` — `yTickSampleValues` |
| 드래그 중 미리보기 디바운스 우회 | `previewLiveKeys.ts` |
| Figma 설정 화면 추출 도구 | `design/figma/panel-settings/` |

`previewLiveKeys.ts` 를 별도 모듈로 뺀 이유는 반복된 실수 때문이다 — 끌어 옮기는 값을 새로 만들 때마다 200ms 디바운스 제외 목록에 넣는 것을 잊어 드래그가 끊겼다. 목록을 한 곳이 소유하고 대상별 시험을 붙여 누락이 드러나게 했다.

### 8.5 미해결로 남긴 것

- `sources[]` 의 소스별 색 팔레트(계열 색이 소스 간에 겹칠 수 있다).
- 소스별 조회 주기 — 구간과 마찬가지로 패널이 소유할지 미정.
- `references/modbus-device/` 는 이번 변경과 무관한 별도 반입물이다.
