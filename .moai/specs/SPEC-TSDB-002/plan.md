# SPEC-TSDB-002 구현 계획 (v0.2.0)

관련 문서: [spec.md](./spec.md) · [acceptance.md](./acceptance.md)

> **v0.1.0 대비 변경**: 모델 정정(§spec.md HISTORY-0.2.0)에 따라 마일스톤을 8개에서 7개로 재구성했다. v0.1.0 의 **M4(memTSDB 어댑터)는 통째로 삭제**되었고, 나머지가 재배열되었다. 대응표는 §7.

## 0. 전략 요약

프로젝트 개발 방식은 `hybrid`(`.moai/config/sections/quality.yaml`)다. 마일스톤마다 두 규율을 나눠 적용한다.

| 대상 | 방식 | 커버리지 목표 |
|------|------|---------------|
| 신규 파일 (`panelDataSource.ts` · `usePanelSeriesData.ts` · `tsdbSource.ts` · `useTsdbChartData.ts` · `influxdb_seriesquery.go` · `influxdb_schema.go` · `influxdb_series.go`) | **TDD** (RED → GREEN → REFACTOR) | 85% |
| 기존 파일 수정 (6개 패널 · `previewSeries.ts` · `PanelSettingsDialog.tsx` · `ChartPanelSections.tsx` · `store.ts` · `seriesDataSource.ts` · `AgentDetailPanel.tsx` · `influxdb_management.go`) | **DDD** (ANALYZE → PRESERVE → IMPROVE). 특성화 테스트를 **먼저** 작성해 현재 동작을 잠근 뒤 수정 | 85% |

### 두 축과 합류 지점

```
계약 축 (프론트, 무동작 리팩터)      백엔드 축 (Go, 프론트 무변경)
  M1 계약 타입 + 판정                  M4 InfluxDB 구조화 질의
  M2 특성화                            M5 InfluxDB 스키마 디스커버리
  M3 디스패치 일반화
            └──────────┬──────────────────────┘
                       M6 TSDB 어댑터 + 토글 영속 + 선택 UI   ← 첫 사용자 가치
                       M7 마무리
```

**M1~M3 에는 사용자에게 보이는 변화가 0이다.** 이 사실을 계획에 명시하는 이유는 spec.md §4.1 에 적었다 — v0.1.0 은 중간 마일스톤을 "독립적 사용자 가치"로 정당화했으나 그 근거는 정정된 모델에서 사라졌다.

### 핵심 위험 하나 — 10개 디스패치 지점의 동시 수정

[spec.md §1.2.5](./spec.md) 의 10개 지점은 5종 차트 패널 + 히트맵 + 게이지 + 미리보기 2곳 + 범례 1곳에 흩어져 있고, **각 지점이 서로 다른 부가 조건**을 갖는다.

| 지점 | 부가 조건 | 위험 |
|------|-----------|------|
| `StatPanel` · `BarChartPanel` · `PieChartPanel` · `TablePanel` · `LineChartPanel` | `(storeSource?.series?.length ?? 0) > 0` | 동형 — 낮음 |
| `HeatmapPanel:122` | 같은 형태이나 **다른 config 키**(`heatmapStoreSource`)를 읽는다 | 이관 시 키를 잘못 참조하면 히트맵만 조용히 빈다 |
| `gaugeLegacyBinding.ts:122` | `&& flags.hasSeriesReduce` **논리곱 추가** — SPEC-CHART-002 §2.9 소유 | 이 항을 `panelDataSource.ts` 로 함께 옮기면 게이지가 아닌 패널의 활성 판정이 바뀐다 |
| `PanelSettingsDialog.tsx:714` · `:728` | 미리보기 전용. 서로 다른 config 참조(`previewRenderPanel.config` vs `previewChartConfig`) | 두 지점의 미묘한 차이를 하나로 뭉개면 미리보기와 실제 렌더가 갈린다 |
| `previewSeries.ts:65` | 시리즈 0개면 채널 폴백 — **활성 판정과 폴백이 결합**되어 있다 | 활성 판정만 뽑아내면 폴백 동작이 사라진다 |

따라서 **M2(특성화)와 M3(일반화)를 반드시 분리하고 각각 단독 커밋한다.** M2 의 산출물은 테스트뿐이며 프로덕션 코드를 한 줄도 고치지 않는다.

### 마일스톤 간 검증

```bash
cd web && npm run build && npm test        # 프론트 마일스톤
go build ./... && go test ./...            # 백엔드 마일스톤
```

---

## 1. 마일스톤 분해

### M1 — 계약: 타입 + 소스 판정 순수 로직 (Priority High, TDD)

백엔드 변경 0. 패널 렌더 코드 변경 0. **타입과 순수 함수만 추가한다.**

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | `ChartDataSourceKind` 를 3종으로 확장 (`'channel' \| 'store' \| 'tsdb'`) | `charts/chartChannelTypes.ts` |
| 1.2 | `TsdbBackend` · `TsdbSourceConfig` · `TsdbSeriesRef` 정의(**단일 블록**, spec.md §2.2). `ChartPanelConfigBase` 에 `tsdb_source?` 추가 | 동일 |
| 1.3 | `defaultTsdbSource()` — 조회 창은 `DEFAULT_STORE_SOURCE_WINDOW` 전개, `backend: 'influxdb'`, `agent_name: ''`, `series: []` | `ChartPanelSections.tsx` 또는 신규 모듈 |
| 1.4 | **`SeriesDataSourceKind` 개명** — `'tsdb'`(memTSDB) → `'memtsdb'`, 신규 `'tsdb'`(외부) 가산. 개명 지점 5곳: `seriesDataSource.ts:16 · :119`, `tsdb.ts:466`, `AgentDetailPanel.tsx:26 · :298` | `services/api/seriesDataSource.ts` 외 |
| 1.5 | `resolvePanelSourceBinding(config): PanelSourceBinding` — 소스 종류 판정 + 활성 조건 + `unknownKind` 플래그. **spec.md §2.3 의 표를 유일한 정본으로 구현** | `charts/panelDataSource.ts`(신규) |
| 1.6 | `panelSourceCapabilities(kind): SourceCapabilities` — spec.md §2.13 능력 표. `Record<ChartDataSourceKind, ...>` 로 두어 컴파일러가 전수성을 강제 | 동일 |
| 1.7 | 단위 테스트 — 3종 × (블록 부재 / 시리즈 0 / 시리즈 N / 에이전트 미선택 / tag 모드 / 인식 불가 문자열) | `charts/panelDataSource.test.ts`(신규) |

**설계 메모 1.** `resolvePanelSourceBinding` 은 게이지의 `hasSeriesReduce` 항을 **포함하지 않는다.** 그것은 소스 활성이 아니라 게이지 고유의 레거시 우선순위이며 [SPEC-CHART-002](../SPEC-CHART-002/spec.md) §2.9 가 소유한다. M3 에서 `resolveGaugeValueSource` 가 `resolvePanelSourceBinding` 을 **호출해 활성 여부만 받고**, `hasSeriesReduce` 논리곱은 `gaugeLegacyBinding.ts` 에 남긴다.

**설계 메모 2 (1.4 의 위험).** 개명은 식별자만 바꾸며 동작을 바꾸지 않는다. 위험이 낮은 이유는 개명 대상 경로(`/api/v1/tsdb/*`)가 현재 프로덕션에 등록되어 있지 않아(spec.md §1.2.1) 런타임 회귀가 발생할 코드 경로 자체가 없기 때문이다. 그럼에도 개명 커밋을 **단독**으로 두어 `tsc --noEmit` 이 완결성을 확인하게 한다.

**M1 을 먼저 두는 이유.** 이후 모든 프론트 마일스톤이 이 타입과 판정 함수를 전제한다. 순수 함수라 UI 없이 전수 테스트가 가능하므로, 활성 조건의 애매함(히트맵의 다른 키 · tag 모드 · 인식 불가 값)을 여기서 전부 소진한다.

**검증**: `npx vitest run src/pages/dashboard/panels/charts/panelDataSource.test.ts` + `npx tsc --noEmit`.

---

### M2 — 특성화: 10개 디스패치 지점의 현재 동작 잠금 (Priority High, DDD PRESERVE)

**프로덕션 코드 변경 0.** 이 마일스톤의 산출물은 테스트뿐이다. v0.1.0 M2 와 내용이 동일하다(모델 정정의 영향 없음).

| # | 대상 | 잠글 동작 | 파일 |
|---|------|-----------|------|
| 2.1 | `LineChartPanel` | `data_source:'store'` + 시리즈 N개 → store 훅 경로 / 시리즈 0개 → 채널 경로 / `data_source` 부재 → 채널 경로 | `charts/LineChartPanel.test.tsx`(확장) |
| 2.2 | `StatPanel` | 동일 3분기 + `series_reduce` 부재 시 레거시 렌더 유지 | `charts/StatPanelStoreSource.test.tsx`(확장) |
| 2.3 | `BarChartPanel` | 동일 3분기 | `charts/BarChartPanel.test.tsx`(확장) |
| 2.4 | `PieChartPanel` | 동일 3분기 | `charts/PieChartPanel.test.tsx`(확장) |
| 2.5 | `TablePanel` | 동일 3분기 | `charts/TablePanel.test.tsx`(확장 또는 신규) |
| 2.6 | `HeatmapPanel` | **`heatmapStoreSource` 키 경유**의 활성 판정. `store_source` 를 직접 읽지 않는다는 사실을 테스트가 알게 한다 | `heatmap/HeatmapPanel.test.tsx`(확장) |
| 2.7 | `gaugeLegacyBinding` | 진리표 4행(SPEC-CHART-002 §2.9)이 **무변경**임을 잠근다. 특히 `store` + 활성 + `series_reduce` **부재** → `legacy` | `charts/gaugeLegacyBinding.test.ts`(확장) |
| 2.8 | `PanelSettingsDialog` 미리보기 | `:714` 와 `:728` 두 게이트의 현재 참 조건. 두 지점이 **서로 다른 config 객체**를 본다는 사실 | `PanelSettingsDialog.linePreview.test.tsx`(확장) |
| 2.9 | `previewSeries` | `dataSource:'store'` + 시리즈 N → store 범례 / 시리즈 0 → **채널 폴백** / 채널 0개 → sample 한 줄 | `charts/previewSeries.test.ts`(확장 또는 신규) |
| 2.10 | 설정 UI 게이팅 | `ChartPanelSections.tsx:398·404·445·458` 4개 게이트가 store 모드에서만 참임 | `StoreSourceSection.test.tsx`(확장) |

**검증 기준(중요).** 신규 특성화 테스트는 **수정 전 코드에서 전부 GREEN** 이어야 한다. 하나라도 RED 면 그것은 특성화가 아니라 요구사항이므로 별도 이슈로 분리한다.

2.6 · 2.7 · 2.9 가 이 SPEC 전체에서 가장 중요한 안전망이다. 세 지점 모두 "store 인가" 판정에 추가 요소가 결합되어 있어, 기계적 치환 시 조용히 동작이 바뀔 수 있는 곳이다.

---

### M3 — 디스패치 일반화 (Priority High, DDD IMPROVE)

M2 의 테스트가 전부 GREEN 인 상태를 유지하면서 10개 지점을 치환한다.

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `usePanelSeriesData(config): UseStoreChartDataResult` — 소스 종류별 훅을 **조건 없이 전부 호출**하고 진 쪽에 `undefined` 를 넘겨 idle 로 둔다. M3 시점에는 store 분기만 실제 동작하고 tsdb 는 idle 스텁 | `charts/usePanelSeriesData.ts`(신규, TDD) |
| 3.2 | 5종 차트 패널의 `config.data_source === 'store' && ...` → `resolvePanelSourceBinding(config).active` + `usePanelSeriesData(config)` | `LineChartPanel` · `StatPanel` · `BarChartPanel` · `PieChartPanel` · `TablePanel` |
| 3.3 | `HeatmapPanel` 치환. **`heatmapStoreSource` 키 경유를 유지**하거나 `panelDataSource.ts` 가 그 키를 알게 한다(둘 중 하나를 M1 에서 확정) | `heatmap/HeatmapPanel.tsx` |
| 3.4 | `resolveGaugeValueSource` 가 `resolvePanelSourceBinding` 을 호출하도록 변경. `hasSeriesReduce` 논리곱은 **그대로 남긴다** | `charts/gaugeLegacyBinding.ts` |
| 3.5 | `PanelSettingsDialog.tsx:714 · :728` 치환 | `PanelSettingsDialog.tsx` |
| 3.6 | `previewSeries.ts:65` 치환. **활성 판정과 채널 폴백의 결합을 유지** | `charts/previewSeries.ts` |
| 3.7 | 설정 UI 게이팅 4곳 치환 | `ChartPanelSections.tsx` |
| 3.8 | 회귀 확인 — M2 특성화 전량 GREEN + 전체 프론트 테스트 GREEN | — |

**커밋 분할.** 3.2(동형 5개) / 3.3+3.4(이형 2개) / 3.5+3.6+3.7(미리보기·UI) 3개 커밋으로 나눈다. 이형 2개를 동형 5개와 섞으면 회귀 시 이등분 탐색이 어려워진다.

---

### M4 — InfluxDB 구조화 질의 백엔드 (Priority High, TDD) — *v0.1.0 의 M5*

**Go 만 변경.** 프론트 변경 0. **M1~M3 와 병렬 진행 가능**(파일 집합이 겹치지 않는다).

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 집계 어휘 매핑 — `average → mean`(Flux) / `MEAN`(InfluxQL). 5종 명시 `switch` + `default:` 오류 (spec.md §2.7 / §4.6) | `internal/agent/system/influxdb_seriesquery.go`(신규) |
| 4.2 | 식별자 이스케이프 — Flux 문자열 리터럴 · InfluxQL 식별자. **Flux 쪽은 트리에 대응물이 없다** | 동일 |
| 4.3 | Flux 생성 순수 함수 `buildFluxSeriesQuery(...) (string, error)`. **`timeSrc: "_start"` 고정** (spec.md §2.7 / §2.8) | 동일 |
| 4.4 | InfluxQL 생성 순수 함수 `buildInfluxQLSeriesQuery(...)`. `GROUP BY time(d)` 에 `offset` 인자 **미지정** | 동일 |
| 4.5 | `fill` 매핑 5종. `'avg'` → 오류 (spec.md §2.7) | 동일 |
| 4.6 | 순수 함수 전수 테스트 — (v2/v3) × (집계 5) × (fill 5) × (태그 0/1/N) + 이스케이프 경계 | `influxdb_seriesquery_test.go`(신규) |
| 4.7 | 에이전트 메서드 `QuerySeriesBuckets(ctx, req) ([]seriesBucket, error)` — 버전 분기 + 실행 + 버킷 정규화 | `internal/agent/system/influxdb_agent.go`(확장) |
| 4.8 | DTO — `influxSeriesQueryRequest`. **응답은 기존 `chartQueryResponse` 재사용**(신규 응답 타입을 만들지 않는다, spec.md §2.6 · UB1-23) | `internal/api/dto/influxdb.go` |
| 4.9 | 엔트리 조립 — `timestamp` = 버킷 시작, `labels` = `{"__field__": field, ...tags}`. Store 의 라벨 규약과 동일 | `internal/api/handler/influxdb_series.go`(신규) |
| 4.10 | 핸들러 `POST /influxdb/{agent_name}/series/query`, 권한 `store.read` | 동일 |
| 4.11 | 요청 검증 7종 + 버킷 수 100,000 가드(`maxAggregationBuckets` 재사용) (spec.md §2.6 / §2.9) | 동일 |
| 4.12 | 라우트 등록 + 권한 커버리지 테스트 갱신 | 기존 등록 지점, `route_permission_coverage_test.go` |
| 4.13 | 핸들러 테스트 — mock 에이전트로 200 / 400×7 / 404 / 408 | `influxdb_series_test.go`(신규) |

**4.3 이 이 마일스톤의 핵심이다.** `timeSrc: "_start"` 한 줄을 빠뜨리면 모든 데이터가 한 인터벌만큼 미래로 밀리고, 그 증상은 "값이 이상하다"가 아니라 "값이 약간 늦는다"이므로 리뷰에서 잡히지 않는다. AC-26 이 이를 기계적으로 검증한다.

**4.8 이 v0.1.0 대비 가장 크게 바뀐 항목이다.** v0.1.0 은 `{columns, rows}` 행렬 응답 DTO 2종(`influxSeriesQueryResponse` · `influxSeriesMatrixRow`)을 신설하려 했다. v0.2.0 은 **신규 응답 타입을 0개** 만든다.

**순수 함수와 실행을 분리하는 이유.** 쿼리 생성은 네트워크 없이 전수 테스트가 가능하다. 실행과 섞으면 조합 폭발(v2/v3 × 5 × 5 × 3 = 150)을 mock HTTP 로 돌려야 한다.

---

### M5 — InfluxDB 스키마 디스커버리 백엔드 (Priority Medium, TDD) — *v0.1.0 의 M6*

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | 클라이언트 인터페이스에 4종 디스커버리 메서드 추가 (`ListMeasurements` 는 이미 존재) | `internal/agent/system/influxdb_client.go` |
| 5.2 | v2 구현 — `schema.measurements` · `schema.tagKeys` · `schema.measurementTagValues` · **`schema.measurementFieldKeys`(신규)**. 앞 3종은 `internal/migrate/tsdbtags/client_v2.go` 의 쿼리 문자열을 복제 (spec.md §4.5) | `internal/agent/system/influxdb_schema.go`(신규) 또는 `influxdb_v2.go` 확장 |
| 5.3 | v3 구현 — `SHOW MEASUREMENTS` · `SHOW TAG KEYS FROM` · `SHOW TAG VALUES ... WITH KEY =` · **`SHOW FIELD KEYS FROM`(신규)**. 앞 3종은 `client_v3.go` 복제 | 동일 |
| 5.4 | v3 `ListMeasurements` 의 `ErrManagementNotSupported` 제거. **나머지 5종 관리 조작의 501 은 유지** (spec.md §2.10) | `internal/agent/system/influxdb_v3.go` |
| 5.5 | 기존 v3 501 기대 테스트 갱신 — measurements 만 반전, 나머지 5종은 그대로 | `influxdb_management_test.go`, `internal/api/handler/influxdb_management_test.go` |
| 5.6 | 신규 라우트 3종 (`tag-keys` · `tag-values` · `field-keys`), 권한 `store.read` | `internal/api/handler/influxdb_management.go`(확장) |
| 5.7 | 핸들러 테스트 + 라우트 권한 커버리지 갱신 | 각 `_test.go` |

**5.4 는 의도된 회귀다.** v3 에이전트에서 `GET /measurements` 가 501 → 200 으로 바뀐다. 그 동작에 의존하는 클라이언트가 있다면 501 을 오류로 처리하고 있었을 것이므로 200 은 개선이다. 다만 테스트 기대값 갱신을 **명시적 커밋**으로 남긴다.

---

### M6 — TSDB 어댑터 + 토글 영속 + 선택 UI (Priority High, TDD + DDD)

**합류 지점. 이 마일스톤이 끝나야 처음으로 사용자 가치가 생긴다.** v0.1.0 의 M4(토글 영속 · AC-05 반전)와 M7(어댑터 · 선택 UI)이 여기로 합쳐졌다.

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | `queryStoreMatrix`(`store.ts:589`)에서 **키별 페처를 주입 가능하게 추출**한다. 컬럼 구성 · 0행 자리 보존 · 버킷 합집합 피벗은 공용 함수가 된다 (spec.md §4.3 · UB1-25) | `services/api/seriesMatrixPivot.ts`(신규) + `store.ts` 리팩터 |
| 6.2 | 6.1 리팩터의 무동작 확인 — 기존 `store.test.ts` 전량 GREEN, Store 경로 렌더 무변경 | — |
| 6.3 | `fetchTsdbSeries(agentName, ref, params, signal)` — `POST /influxdb/{name}/series/query` 호출 + `groupEntriesBySeries` 로 라벨 그룹화 | `services/api/tsdbSource.ts`(신규, TDD) |
| 6.4 | `queryTsdbMatrix(...)` — 6.1 의 공용 피벗 + `Promise.allSettled`(부분 실패 격리, spec.md §2.19) + 각 요청에 `AbortSignal` 전달 | 동일 |
| 6.5 | **백엔드 파생** — `agent_id` → `resolveAgentName` → 에이전트 `type` → 어댑터 선택. 미지원 타입은 백엔드 불일치 오류 (spec.md §2.18) | 동일 |
| 6.6 | `tsdbSeriesDataSourceFor(agent)` — `kind: 'tsdb'`, `useKeys` 는 measurement 목록(D1) | 동일 |
| 6.7 | `useTsdbChartData` — `useStoreChartData` 와 동일한 폴링/재구독/보존 규칙으로 `UseStoreChartDataResult` + 부분 실패 신호를 반환 | `charts/useTsdbChartData.ts`(신규, TDD) |
| 6.8 | `usePanelSeriesData` 의 tsdb 분기를 6.7 에 연결 | `charts/usePanelSeriesData.ts` |
| 6.9 | `ChartPanelSections` 의 로컬 `tsdbMode` **제거**(현재 9개 참조). 토글이 `onConfigChange({ data_source: 'tsdb', tsdb_source: defaultTsdbSource() })` 를 호출 (spec.md §2.11) | `ChartPanelSections.tsx` |
| 6.10 | **AC-05 테스트 제자리 반전** — 사유 주석 + 대체 SPEC ID 명시. 삭제 금지. 대상은 `:138` **하나뿐** | `PanelSettingsDataSource.test.tsx` |
| 6.11 | `TsdbSourceSection` — 에이전트 셀렉트(`type === 'influxdb'` 필터, Store 셀렉트 패턴 재사용) + bucket 선택(v2 목록 / v3 자유 입력) | 신규 `TsdbSourceSection.tsx` |
| 6.12 | measurement → field → tag 3단 드릴다운 선택 UI. `SeriesSelectTable` 재사용. 시리즈 48 상한 강제 | 동일 |
| 6.13 | 능력 게이팅 — `fill: 'avg'` 비활성 + 사유, v3 는 관리 조작 비활성, Store 의 `fill` 3종 비활성 (spec.md §2.13) | `ChartPanelSections.tsx` |
| 6.14 | 상태 4종 표시 — 빈 선택 / 빈 결과 / 부분 실패 배지 / 오류 오버레이 (spec.md §2.14) | 패널 오버레이 |
| 6.15 | i18n ko/en — TSDB 라벨 · 백엔드 표기 · 능력 사유 · 부분 실패 · 백엔드 불일치. **placeholder 문구(`dataSourceTsdbTitle` · `dataSourceTsdbBody`) 은퇴** | `lib/i18n/{ko,en}.json` |

**6.10 의 처리 방식.** 테스트 본문을 지우지 않고 단언만 뒤집으며, 다음 형태의 주석을 남긴다.

```ts
// [SPEC-TSDB-002 §2.11 반전] SPEC-PANEL-SETTINGS-001 REQ-05 는 TSDB 토글을
// config 무기록(no-op)으로 규정했고 이 테스트가 그것을 잠갔다. SPEC-TSDB-002 가
// 그 비목표를 대체하므로 단언을 반전한다. 삭제하지 않는 이유는 "왜 바뀌었는가"의
// 기록을 diff 밖으로 내보내지 않기 위함이다.
```

**6.1 이 이 마일스톤의 첫 작업인 이유.** 피벗 기계를 먼저 공용화해야 6.4 가 그것을 재사용할 수 있다. 순서를 뒤집으면 TSDB 쪽에 피벗을 한 벌 더 쓰게 되고, 그것이 UB1-25 가 금지하는 상태다. 6.1 은 **무동작 리팩터**이므로 6.2 에서 기존 테스트로 즉시 검증한다.

---

### M7 — 마무리 (Priority Low)

| # | 작업 |
|---|------|
| 7.1 | 전체 회귀 — 프론트 전량 + `go test ./...` + `npx tsc --noEmit` + eslint + `go vet ./...` |
| 7.2 | 커버리지 확인 — 신규 파일 85% 이상 |
| 7.3 | 버킷 정렬 교차 검증 — **Store · InfluxDB v2 · InfluxDB v3 3소스**가 같은 인터벌(divisor / non-divisor 양쪽)에서 같은 버킷 시작을 산출하는지(AC-26 · AC-27) |
| 7.4 | i18n 누락 키 검사 (ko/en 대칭) + 은퇴 키 제거 확인 |
| 7.5 | `.moai/docs/` 데이터소스 문서 갱신 — Store / TSDB 2소스 능력 비교 표 + memTSDB 가 패널 소스가 **아니라는** 명시 |
| 7.6 | spec.md §7 의 NQ1 ~ NQ5 처분을 HISTORY 에 기록 |

---

## 2. 마일스톤 의존 순서

```
M1 (계약 타입 + 판정 + kind 개명)
 └─> M2 (특성화) ──> M3 (일반화) ──┐
                                    ├─> M6 (어댑터 + 토글 + UI) ──> M7
M4 (InfluxDB 질의) ──> M5 (디스커버리) ─┘
```

- **M4 · M5 는 M1 을 의존하지 않는다** (Go 코드는 프론트 계약과 무관). **M1~M3 와 완전 병렬**이다.
- **M6 은 M3 · M4 · M5 전부를 의존한다.** `usePanelSeriesData` 가 존재해야 하고, 질의 엔드포인트와 디스커버리 라우트가 있어야 선택 UI 를 만들 수 있다.
- M3 는 M2 없이 착수할 수 없다 — 특성화가 없으면 회귀를 감지할 방법이 없다.
- M6.1(피벗 공용화)은 M6 내부의 선행 작업이며 M3 완료 후 언제든 착수 가능하다.

---

## 3. 필요한 특성화 테스트 (M2 상세)

각 항목은 **수정 전 코드에서 GREEN** 이어야 한다. v0.1.0 의 CT-01 ~ CT-21 이 그대로 유효하며, CT-21 만 전방 참조 문구가 바뀐다.

### 3.1 소스 활성 판정 (5종 차트 패널 — 동형)

| ID | Given | Then |
|----|-------|------|
| CT-01 | `{}` (config 비어 있음) | 채널 경로. store 훅은 idle |
| CT-02 | `{ data_source: 'channel' }` | 채널 경로 |
| CT-03 | `{ data_source: 'store' }` (`store_source` 부재) | **채널 경로**(활성 조건 미충족) |
| CT-04 | `{ data_source: 'store', store_source: { series: [] } }` | **채널 경로** |
| CT-05 | `{ data_source: 'store', store_source: { series: [k1] } }` | store 경로 |

CT-03 · CT-04 가 특히 중요하다 — 현행 코드는 "store 를 골랐지만 시리즈가 0개"일 때 **채널로 폴백**한다. 일반화 시 이 폴백을 잃으면 빈 패널이 된다.

### 3.2 히트맵 (이형 — 다른 config 키)

| ID | Given | Then |
|----|-------|------|
| CT-06 | `data_source:'store'` + 히트맵 config 의 store 블록에 시리즈 N | store 경로 |
| CT-07 | 동일하나 시리즈 0 | 비활성 |
| CT-08 | `store_source` 는 있으나 히트맵이 읽는 키가 비어 있음 | **비활성** — 히트맵이 `store_source` 를 직접 읽지 않음을 잠근다 |

### 3.3 게이지 (이형 — `series_reduce` 논리곱)

`gaugeLegacyBinding.test.ts` 의 기존 진리표 4행을 그대로 유지하고, `resolvePanelSourceBinding` 이관 후에도 동일함을 확인한다.

| ID | `data_source` | 소스 활성 | `series_reduce` | 기대 |
|----|---------------|-----------|-----------------|------|
| CT-09 | 부재/`channel` | — | — | `legacy` |
| CT-10 | `store` | 비활성 | — | `legacy` |
| CT-11 | `store` | 활성 | 부재 | `legacy` |
| CT-12 | `store` | 활성 | 있음 | `store-source` |

### 3.4 미리보기 (이형 — 두 config 객체)

| ID | Given | Then |
|----|-------|------|
| CT-13 | `previewRenderPanel.config.data_source === 'store'` + 라인 차트 | `:714` 게이트 참 |
| CT-14 | `previewChartConfig.data_source === 'store'` | `:728` 게이트 참 |
| CT-15 | 두 config 가 갈리는 상태(draft 편집 중) | 두 게이트가 **독립적으로** 판정됨 |

### 3.5 범례 시리즈 (이형 — 활성 판정 + 폴백 결합)

| ID | Given | Then |
|----|-------|------|
| CT-16 | `dataSource:'store'` + 시리즈 N | store 시리즈 범례 N개 |
| CT-17 | `dataSource:'store'` + 시리즈 0 + 채널 M개 | **채널 범례 M개**(폴백) |
| CT-18 | `dataSource:'store'` + 시리즈 0 + 채널 0 | sample 한 줄 |

CT-17 이 핵심이다. `previewSeries.ts:65` 의 `if (dataSource === 'store') { ... if (series.length > 0) return ...; }` 구조는 **활성 판정 실패 시 아래로 흘러내리는 폴백**이며, 이를 조기 반환으로 바꾸면 사라진다(재확인함).

### 3.6 설정 UI 게이팅

| ID | Given | Then |
|----|-------|------|
| CT-19 | `data_source:'store'` | `StoreInfoPopover` · 에이전트 셀렉트 · `SeriesNameFormatField` · `SeriesReduceField`(대상 패널) 4곳 렌더 |
| CT-20 | `data_source:'channel'` | 4곳 미렌더 |
| CT-21 | `tsdbMode` 로컬 상태 true | 4곳 미렌더 + placeholder 렌더 — **M6.9 · M6.10 에서 반전될 기준선** |

---

## 4. 위험과 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| 10개 지점 일반화 중 조용한 회귀 | 특정 패널만 빈 화면 | M2 특성화 선행 + M3 커밋 3분할 |
| `heatmapStoreSource` 키 경유 누락 | 히트맵만 조용히 빔 | CT-06 ~ CT-08 |
| `previewSeries` 채널 폴백 소실 | 미리보기 범례가 sample 로 퇴화 | CT-17 |
| 게이지 `hasSeriesReduce` 항 오이관 | 다른 패널의 활성 판정이 바뀜 | CT-09 ~ CT-12 + M1 설계 메모 1 |
| **M6.1 피벗 공용화가 Store 동작을 바꿈** | Store 패널 전체가 조용히 어긋남 | M6.1 을 **무동작 리팩터**로 제한하고 M6.2 에서 기존 `store.test.ts` 전량 GREEN 을 게이트로 둔다. 단독 커밋 |
| **`SeriesDataSourceKind` 개명 누락** | 컴파일 오류 또는 에이전트 상세 Series 탭 오작동 | M1.4 단독 커밋 + `tsc --noEmit` + AC-01 |
| `timeSrc: "_start"` 누락 | InfluxDB 데이터가 한 인터벌 미래로 밀림. 리뷰에서 잡히지 않음 | AC-26 기계 검증 + 4.6 전수 테스트 |
| `average → mean` 매핑 누락 | 쿼리 실패 또는 파서 오류 | AC-25 5종 전수 + spec.md §4.6 명시 `switch` |
| **백엔드 파생을 config `backend` 로 대체** | 에이전트 교체 후 조용한 오라우팅 | AC-50 + spec.md §2.18 · UB1-22 |
| 폴링당 최대 48요청의 InfluxDB 부하 | 대시보드 다수 패널에서 InfluxDB 포화 | 시리즈 48 상한(§2.9) + spec.md §7 NQ2 에 배치 확장 여지 기록. **본 SPEC 에서 해결하지 않음** |
| AC-05 반전 시 테스트 삭제 유혹 | 변경 사유가 diff 밖으로 사라짐 | 6.10 의 주석 형식 강제 + AC-35 |
| AC-05 반전 대상 오인 | 무관한 테스트 2건(`:281` · `:991`)까지 반전 | AC-35 의 `awk` 블록 한정 단언 |
| v3 measurements 501 해제의 파급 | 기존 테스트 다수 갱신 | M5 5.5 를 **단독 커밋**으로 분리 |
| M4/M5 와 M1~M3 병렬 진행 시 충돌 | 없음(파일 겹침 0) | Go/프론트 파일 집합이 분리되어 있음 |

---

## 5. 커밋 분할 계획

| 커밋 | 범위 | 검증 |
|------|------|------|
| C1 | M1.4 (`SeriesDataSourceKind` 개명 단독) | `tsc --noEmit` + 전량 GREEN |
| C2 | M1.1 ~ M1.3, M1.5 ~ M1.7 (타입 + 판정 순수 함수 + 테스트) | `tsc --noEmit` + 신규 단위 테스트 |
| C3 | M2 전체 (특성화 테스트만, 프로덕션 무변경) | 전량 GREEN, `git diff --stat` 에 `.test.` 아닌 파일 0건 |
| C4 | M3.1 + M3.2 (`usePanelSeriesData` + 동형 5개 패널) | M2 전량 GREEN |
| C5 | M3.3 + M3.4 (히트맵 + 게이지) | 동일 |
| C6 | M3.5 ~ M3.7 (미리보기 2 + 범례 + 설정 UI 4) | 동일 |
| C7 | M4.1 ~ M4.6 (쿼리 생성 순수 함수 + 전수 테스트) | `go test ./internal/agent/system/...` |
| C8 | M4.7 ~ M4.13 (에이전트 메서드 + DTO + 핸들러 + 라우트) | `go test ./...` |
| C9 | M5.1 ~ M5.3, M5.6, M5.7 (디스커버리 구현 + 라우트) | `go test ./...` |
| C10 | M5.4 + M5.5 (**v3 measurements 501 해제 + 기대값 갱신**) — 단독 커밋 | `go test ./...` |
| C11 | M6.1 + M6.2 (**피벗 공용화, 무동작 리팩터**) — 단독 커밋 | 기존 `store.test.ts` 전량 GREEN |
| C12 | M6.3 ~ M6.8 (TSDB 어댑터 + 백엔드 파생 + 훅) | 신규 테스트 |
| C13 | M6.9 + M6.10 (토글 영속 + **AC-05 반전**) | AC-34 · AC-35 |
| C14 | M6.11 ~ M6.15 (선택 UI + 능력 게이팅 + 상태 표시 + i18n) | AC-37 ~ AC-40 · AC-49 |
| C15 | M7 (회귀 · 문서 · HISTORY) | 전체 |

---

## 6. 완료 정의 (Definition of Done)

> 체크 표시는 **M7 회차에 직접 관측한 것만** 붙였다. 미체크 항목은 "거짓"이 아니라 **이 회차에 전수 재검증하지 않았다**는 뜻이다. 관측 근거는 acceptance.md §M2 와 `.moai/state/verify/tsdb002-m7/` 의 로그다.

- [ ] spec.md 의 U1~U4 · U6~U12 · E1~E2 · S1~S2 · UB1~UB2 전부 구현 (**U5 는 은퇴**) — 요구사항 단위 전수 재검증은 이 회차에 수행하지 않음
- [ ] acceptance.md 의 AC-01 ~ AC-12 · AC-17 ~ AC-54 전부 통과 (**AC-13 ~ AC-16 은 은퇴**) — 이 회차에 실행한 것은 §M2 에 열거한 부분집합
- [x] M2 특성화 테스트 CT-01 ~ CT-21 이 M3 이후에도 전량 GREEN — 마커 21종 전부 존재하고 `npx vitest run` 4067건 전량 통과(exit 0)
- [x] `data_source` 부재 · `'channel'` · `'store'` config 의 렌더 결과 무변경 — 위 CT-01 ~ CT-21 이 잠근 동작이며 전량 GREEN
- [x] 신규 응답 DTO 0개 — `chartQueryResponse` 재사용 확인 (`influxSeriesQueryResponse|influxSeriesMatrixRow` **0건**, `influxdb_series.go` 의 `chartQueryResponse` **2건**)
- [x] 피벗 구현 1벌 — TSDB 전용 피벗 0건 (`seriesMatrixPivot.ts` present · `allBuckets` 3건, `store.ts` **0건**, `tsdbSource.ts` **0건**)
- [x] 신규 파일 커버리지 85% 이상 — 신규 7종 전부 충족 (최저 `TsdbSourceSection.tsx` 96.12%)
- [x] `npx tsc --noEmit` · eslint · `go build ./...` · `go vet ./...` · `go test ./...` 전부 통과 — 6종 전부 exit 0 (`npx eslint .` 비스코프 포함)
- [x] i18n ko/en 대칭 + 은퇴 키 제거 — `ko-only: 0 en-only: 0`, 은퇴 placeholder 키 2종 부재. **본 SPEC 귀속 고아 키 0건**(잔여 후보 21건은 전부 선행 SPEC 귀속 → 범위 밖)
- [x] spec.md §7 NQ1 ~ NQ5 처분이 HISTORY 에 기록 — `0.3.0` 행이 NQ1~NQ5 + NQ-rename 을 모두 기록 (M7.6 은 착수 시점에 **이미 충족**)

---

## 7. v0.1.0 마일스톤 대응표

| v0.1.0 | v0.2.0 | 처분 |
|--------|--------|------|
| M1 계약 타입 + 판정 | **M1** | 유지. `ChartDataSourceKind` 4종→3종, `TsdbSourceConfig`/`InfluxSourceConfig` 2블록→1블록, `SeriesDataSourceKind` 개명 추가 |
| M2 특성화 | **M2** | 무변경 |
| M3 디스패치 일반화 | **M3** | 무변경 |
| M4 memTSDB 어댑터 + 설정 영속 | **삭제 + 분해** | memTSDB 어댑터(4.1~4.7)는 **삭제**. 토글 영속·AC-05 반전·능력 게이팅·i18n(4.8~4.13)은 **M6 으로 이동**. 인터벌 제한(4.12)은 **삭제**(OQ9 폐기) |
| M5 InfluxDB 백엔드 | **M4** | 유지. 응답 DTO 신설 → `chartQueryResponse` 재사용으로 변경, 요청이 N시리즈 → 1시리즈 |
| M6 디스커버리 백엔드 | **M5** | 무변경 |
| M7 InfluxDB 어댑터 + 선택 UI | **M6 의 일부** | 유지. `useKeys` 계약 마찰(v0.1.0 7.2)은 **해소** — 요청 축이 `key`(=measurement) 이므로 `useKeys` 가 measurement 목록을 반환하는 것이 계약과 일치한다. 피벗 공용화(6.1)가 앞에 추가됨 |
| M8 마무리 | **M7** | 유지. 버킷 교차 검증 대상이 4소스 → 3소스 |
