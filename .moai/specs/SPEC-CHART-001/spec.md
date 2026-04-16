---
id: SPEC-CHART-001
version: "1.1.0"
status: implemented
created: "2026-04-16"
updated: "2026-04-16"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-16 | 1.0.0 | 초기 SPEC 작성. 플로우 기반 차트 패널 연동 시스템 정의 (chart-emitter 노드 + 전용 WebSocket 채널 + 5종 차트 패널). SPEC-STORE-002 의 QueryHistory 와 SPEC-WEB-001 의 대시보드 패널 확장 위에 설계. |
| 2026-04-16 | 1.1.0 | M1-M7 구현 완료. 백엔드 (chart-emitter 노드 + 채널 레지스트리 + /ws/chart WS 엔드포인트 + Store/InfluxDB/charts HTTP 쿼리 API), 프론트엔드 (5종 차트 패널 + WebSocket 훅 + AddPanel/PanelSettings 확장 + 플로우 캔버스 등록), 활용 가이드 문서 전부 배포. Go 테스트 평균 93% 커버리지 + Vitest 132 테스트 평균 88% 커버리지, race clean. 6개 분할 커밋. 알려진 제약: REQ-M6-03 실시간 구독자 배지는 node.stats WS 확장 필요로 부분 구현 (스키마/메타만 완료). |

---

# SPEC-CHART-001: 차트 패널 플로우 연동 시스템

> **SPEC ID**: SPEC-CHART-001
> **제목**: 대시보드 차트 패널 ↔ 플로우 데이터 파이프라인 연동 시스템
> **생성일**: 2026-04-16
> **상태**: Approved (2026-04-16 사용자 승인, M1-M6 필수, M7 옵션)
> **우선순위**: High
> **관련 SPEC**:
>   - **SPEC-STORE-002** (Store Value History) — `store-read` 노드의 5종 QueryHistory 모드 활용
>   - **SPEC-TSDB-001** (TSDB Agent) — `tsdb-query` 노드 및 기존 HTTP 쿼리 API 활용
>   - **SPEC-INFLUX-002** (InfluxDB Agent) — `influxdb-query` 노드 활용
>   - **SPEC-WEB-001** (Web Dashboard Module 12/16/18/22/26/28/30) — 대시보드 패널 확장 베이스
>   - **SPEC-FLOW-001 / SPEC-NODE-004** — 플로우 노드 인프라 및 노드 스키마 정의
>   - **SPEC-FILTER-001** — 기존 filter 노드 (chart-emitter 앞단 필터링)
>   - **SPEC-AGG-001 / SPEC-AGG-002** — 기존 aggregate 노드 (시간 bin 집계)

---

## 1. Environment (환경)

### 1.1 배경 및 문제 정의

사용자 요구사항 원문:

> "차트 패널 데이터는 스토어 에이전트 또는 데이터 베이스 에이전트를 통해 데이터 베이스에서 가져와 출력함. 차트의 종류에 따라 마지막 데이터, 구간 데이터 등을 가져 올 수 있으며, 조건에 따라 필터링이나 정렬도 가능함"

**현재 상태:**

- 대시보드 패널 프레임워크는 존재하나 (`web/src/pages/dashboard/panels/`), 차트 계열 패널 5종(`stat`, `line-chart`, `bar-chart`, `pie-chart`, `table`) 은 `DashboardPage.tsx` L383-L396 에서 플레이스홀더 `<div>` 로만 렌더링됨.
- 데이터 소스는 3종 존재:
  - **Store 에이전트** (`internal/agent/system/store.go`): 5-mode `QueryHistory` Go API 제공 (latest, last_n, duration, time_range, since_n). **HTTP 엔드포인트 없음.**
  - **TSDB 에이전트** (`internal/agent/system/tsdb_agent.go`): `POST /api/v1/tsdb/query`, `GET /api/v1/tsdb/series/{key}/latest` HTTP 제공.
  - **InfluxDB 에이전트** (`internal/agent/system/influxdb_agent.go`): **쓰기 전용**, 쿼리 HTTP 없음.
- 플로우 노드 3종 존재: `store-read`, `tsdb-query`, `influxdb-query` + 보조 `filter`, `aggregate`, `mapping`, `transform`.
- WebSocket Hub (`internal/api/ws/`) 는 확장 가능. 현재 메시지 타입: `flow.status`, `flow.metrics`, `node.stats`, `agent.status`, `log.entry`, `system.event`, `device.status`, `debug.message`. **타임시리즈용 채널 타입은 없음.**
- 프론트엔드에 **Recharts 3.7.0** 이미 설치 (`web/package.json`) — `ResourceWidget`, `MetricsChart` 에서 Area 차트 사용 중.

**해결해야 할 문제:**

1. 사용자가 SQL/Flux 를 몰라도 플로우 캔버스에서 데이터 소스 + 필터 + 집계 조합을 구성해 차트에 연결할 수 있어야 한다.
2. 차트 패널은 "어느 데이터 소스에서 어떻게 가져오는지" 를 몰라야 한다 (decoupling). 플로우 출력만 구독한다.
3. 5종 차트 타입이 각각 다른 데이터 형태를 요구한다 (단일 값 vs 시계열 vs 카테고리 vs 분포 vs 행렬). 동일 프로토콜로 5종 모두 표현 가능해야 한다.
4. 초기 로드 시 과거 데이터도 보여야 하고, 이후 신규 데이터는 실시간 push 되어야 한다.

### 1.2 프로젝트 컨텍스트

- **프로젝트**: xflow (Go 모듈: `github.com/xtra/xflow`)
- **대상 화면**: 커스터마이징 가능한 대시보드 페이지 (`web/src/pages/dashboard/DashboardPage.tsx`)
- **대상 사용자**: xflow 대시보드를 구성하는 운영자/엔지니어 (SQL 비숙련)

### 1.3 기술 스택

**백엔드:**

- **언어**: Go 1.23+
- **노드 프레임워크**: `internal/node.Node`, `internal/node.SourceNode`, 포트 시스템 (`internal/node/ports.go`)
- **WebSocket**: `internal/api/ws` 허브 (broadcaster, event_publisher, log_writer)
- **메시지 시스템**: `pkg/message.Message` (payload 는 `map[string]any`)
- **HTTP 라우터**: 기존 `internal/api/handler/*.go` 패턴

**프론트엔드:**

- **언어**: TypeScript 5.x
- **프레임워크**: React 19 + Vite
- **차트 라이브러리**: **Recharts 3.7.0** (이미 설치됨, `stat` / `line-chart` / `bar-chart` / `pie-chart` 용)
- **상태 관리**: Zustand + localStorage (`web/src/stores/uiStore.ts`)
- **데이터 조회**: React Query 5.x (기존 `useDeviceRealtime` 패턴)
- **WebSocket 클라이언트**: 기존 `web/src/services/ws/` 인프라

### 1.4 설계 원칙 (사용자 승인된 Scope 기반)

본 SPEC 은 사용자와의 협의를 거쳐 다음 범위로 확정됨 (AskUserQuestion):

- **Scope**: Balanced — 5종 차트 패널 + 3종 데이터 소스 (Store/TSDB/InfluxDB)
- **Binding Model**: **Node Graph Style (Flow Connection)** — 차트 패널은 자체적으로 쿼리하지 않고, 플로우 노드가 출력한 데이터를 WebSocket 채널을 통해 구독한다.

이로부터 다음 원칙이 파생된다:

1. **플로우 중심 데이터 소싱**: 사용자는 플로우 캔버스에서 `store-read` / `tsdb-query` / `influxdb-query` 노드 중 택일 → (옵션) `filter` / `aggregate` / `transform` → **`chart-emitter` 노드** (신규) 로 종결되는 플로우를 구성한다. `chart-emitter` 는 각 메시지를 지정된 WebSocket 채널로 발행한다.
2. **채널 기반 decoupling**: `chart-emitter` 의 config = `{ channel_name: string, buffer_size: int, retention_sec: int }`. 패널은 `channel_name` 만 알면 된다. 한 emitter → 다수 패널 fan-out 허용.
3. **패널 측은 렌더링만**: 필터/정렬/집계는 플로우의 기존 노드에서 수행된다. 패널 설정 UI 에는 복잡한 필터 조건 UI 가 없다 — 대신 사용자가 플로우 노드를 활용한다.
4. **데이터 계약 표준화**: `chart-emitter` 로 전달되는 메시지는 `{ timestamp: int64 (epoch ms), value: any, labels?: object, meta?: object }` 규격을 따른다. 프로젝트 메모리 규칙: 모든 타임스탬프는 **epoch ms (int64)** 로 통일.
5. **HTTP 스냅샷 엔드포인트 (보조)**: 패널이 WS 구독 전 초기 로드용으로 HTTP 쿼리를 호출할 수 있도록 Store/InfluxDB 에 query HTTP 엔드포인트를 추가한다. TSDB 는 기존 엔드포인트 재사용.

---

## 2. Assumptions (가정)

### 2.1 데이터 소스 가정

- [A-01] `Store.QueryHistory` 의 5가지 모드 (latest / last_n / duration / time_range / since_n) 는 안정화되어 있다 (SPEC-STORE-002 v1.2.0 기준).
- [A-02] TSDB 의 `POST /api/v1/tsdb/query` 는 그대로 재사용 가능하다 (SPEC-TSDB-001).
- [A-03] InfluxDB 에이전트는 현재 쓰기 전용이나, Flux 쿼리 인터페이스를 신규 추가할 수 있다 (influxdb-client-go v2 의 `QueryAPI` 사용).
- [A-04] Store/TSDB/InfluxDB 모두 저장 시 타임스탬프는 **epoch ms** 로 통일되어 있다 (프로젝트 메모리 규칙).

### 2.2 WebSocket 인프라 가정

- [A-05] 기존 `internal/api/ws.Hub` 는 확장 가능하며, 새로운 채널 타입 추가가 다른 메시지 타입 처리에 영향을 주지 않는다.
- [A-06] 채널당 구독자 수는 일반적으로 1-5명 (하나의 차트를 여러 명이 보는 경우), 동시 활성 채널 수는 ~50개 내외를 가정한다.
- [A-07] 링버퍼 기본 크기 100개 항목 × ~1KB/항목 = 채널당 ~100KB 메모리. 50개 채널 × 100KB = ~5MB 로 허용 가능.

### 2.3 프론트엔드 가정

- [A-08] Recharts 3.7.0 은 5종 차트 중 `table` 를 제외한 4종 (stat/line/bar/pie) 모두 지원한다.
- [A-09] `PanelConfig.config: Record<string, unknown>` 에 차트별 설정을 저장하는 기존 패턴 (`web/src/stores/uiStore.ts` L63-L69) 을 재사용한다.
- [A-10] 차트 패널 추가/편집은 기존 `AddPanelDialog` / `PanelSettingsDialog` 를 확장하는 방식으로 충분하다.

### 2.4 사용자 워크플로우 가정

- [A-11] 사용자는 **먼저 플로우를 배포** 하고, **그 다음 대시보드에 차트 패널을 추가** 한다. 패널 생성 시점에 대상 플로우가 이미 실행 중이어야 한다.
- [A-12] `chart-emitter` 노드의 `channel_name` 은 하나의 xflow 인스턴스 내에서 **고유** 해야 한다 (중복 시 설계상 마지막으로 시작된 노드가 채널을 점유).
- [A-13] 플로우가 중단(Undeploy) 되면 해당 채널의 신규 데이터 발행은 중지되지만, 링버퍼 내용은 플로우가 재시작될 때까지 유지될 수 있다 (자세한 동작은 REQ-M1-08 참조).

### 2.5 보안 가정

- [A-14] WebSocket 엔드포인트 `/ws/chart/{channel_name}` 은 기존 WebSocket Hub 와 동일한 인증 모델을 따른다 (현재 인증 미적용 상태이면 그대로 유지).
- [A-15] `channel_name` 은 URL path 에 포함되므로 유효 문자만 허용한다: `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`.

---

## 3. Requirements (요구사항) — EARS 형식

### M1: `chart-emitter` 플로우 노드

**[REQ-M1-01]** 시스템은 **항상** `chart-emitter` 노드 타입을 제공해야 한다.

**[REQ-M1-02]** `chart-emitter` 노드는 **항상** 다음 설정 필드를 지원해야 한다:
- `channel_name` (string, required, 정규식 `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
- `buffer_size` (int, default 100, range 1-10000) — 링버퍼에 보관할 최근 메시지 개수
- `retention_sec` (int, default 3600, range 0-86400) — 링버퍼 항목 최대 보존 시간 (초). 0 이면 시간 기반 만료 비활성.

**[REQ-M1-03]** **WHEN** `chart-emitter` 노드에 메시지가 입력되면 **THEN** 시스템은 해당 메시지의 payload 를 `channel_name` 에 해당하는 WebSocket 토픽으로 브로드캐스트하고, 동시에 내부 링버퍼에 저장해야 한다.

**[REQ-M1-04]** **WHEN** 신규 WebSocket 구독자가 해당 채널에 연결하면 **THEN** 시스템은 링버퍼에 보관된 최근 `buffer_size` 개 항목(단, `retention_sec` 이내인 것만)을 즉시 backfill 메시지로 전송해야 한다.

**[REQ-M1-05]** 시스템은 **항상** `chart-emitter` 에 입력된 메시지 payload 에 `timestamp` (epoch ms, int64) 와 `value` 필드가 있는지 검증해야 한다. `timestamp` 가 없으면 `chart-emitter` 가 현재 시각(`time.Now().UnixMilli()`)을 주입한다. `value` 가 없으면 메시지를 통째로 `value` 필드로 감싼 `{timestamp, value: <원본 payload>}` 형태로 정규화한다.

**[REQ-M1-06]** 시스템은 **항상** 링버퍼 크기가 `buffer_size` 를 초과하면 가장 오래된 항목부터 FIFO 로 제거해야 한다.

**[REQ-M1-07]** 시스템은 **항상** `retention_sec > 0` 이면 `now - timestamp > retention_sec` 인 항목을 링버퍼에서 자동 제거해야 한다 (주기적 스윕, 최소 10초 간격).

**[REQ-M1-08]** **WHEN** `chart-emitter` 노드가 포함된 플로우가 undeploy 되면 **THEN** 시스템은 해당 채널의 WebSocket 구독자에게 `{type:"chart.closed", channel, reason:"flow_undeployed"}` 를 전송한 뒤 링버퍼를 해제해야 한다.

**[REQ-M1-09]** 시스템은 **동일한** `channel_name` 을 가진 `chart-emitter` 노드가 2개 이상 동시에 활성화되려 하면 **경고 로그**를 남기고 **나중에 시작된 쪽의 Init 을 실패** 시켜야 한다 (fail-fast, SPEC-STORE-002 v1.2.0 의 fail-fast 패턴 준수).

**[REQ-M1-10]** `chart-emitter` 노드는 **항상** 입력 포트 1개 (`in`), 출력 포트 0개로 정의되어야 한다 (종단 노드, sink).

### M2: WebSocket 차트 채널 프로토콜

**[REQ-M2-01]** 시스템은 **항상** `GET /ws/chart/{channel_name}` WebSocket 엔드포인트를 제공해야 한다.

**[REQ-M2-02]** **WHEN** 구독자가 연결되면 **THEN** 시스템은 최초로 backfill 메시지를 다음 형식으로 **1회** 전송해야 한다:

```json
{
  "type": "chart.backfill",
  "channel": "<channel_name>",
  "entries": [
    {"timestamp": 1713312000000, "value": 42.5, "labels": {...}, "meta": {...}},
    ...
  ],
  "backfilled_count": <정수>
}
```

**[REQ-M2-03]** **WHEN** 해당 채널에 신규 메시지가 발행되면 **THEN** 시스템은 모든 활성 구독자에게 다음 형식으로 스트리밍해야 한다:

```json
{
  "type": "chart.append",
  "channel": "<channel_name>",
  "entry": {"timestamp": 1713312001000, "value": 43.0, "labels": {...}, "meta": {...}}
}
```

**[REQ-M2-04]** 시스템은 **항상** 한 채널에 다수 구독자(fan-out)를 허용해야 하며, 구독자 연결/해제는 서로 독립적이어야 한다.

**[REQ-M2-05]** **WHEN** 존재하지 않는 `channel_name` 으로 구독 요청이 오면 **THEN** 시스템은 즉시 `{type:"chart.error", channel, reason:"channel_not_found"}` 를 전송하고 연결을 종료해야 한다.

**[REQ-M2-06]** 시스템은 **항상** 구독자 연결이 끊어지면 해당 구독자를 채널 구독 리스트에서 즉시 제거해야 한다 (메모리 누수 방지).

**[REQ-M2-07]** 시스템은 **항상** `channel_name` 형식 검증을 실패한 요청에 대해 HTTP 400 으로 응답해야 한다 (WebSocket 업그레이드 이전에).

### M3: Store / InfluxDB HTTP 쿼리 엔드포인트 (보조)

**[REQ-M3-01]** 시스템은 **항상** `POST /api/v1/store/{agent_name}/query` 엔드포인트를 제공해야 한다. 요청 바디:

```json
{
  "key": "room1/temp",
  "mode": "last_n",
  "count": 50,
  "duration_sec": 0,
  "start_ms": 0,
  "end_ms": 0,
  "since_version": 0
}
```

`mode` 는 `latest` / `last_n` / `duration` / `time_range` / `since_n` 중 하나. 각 모드별 필수 필드 조합은 `HistoryQuery` 구조체와 동일.

**[REQ-M3-02]** 시스템은 **항상** `POST /api/v1/influxdb/{agent_name}/query` 엔드포인트를 제공해야 한다. 요청 바디:

```json
{
  "query_language": "flux",
  "query": "from(bucket:\"xflow\") |> range(start:-1h)",
  "params": {}
}
```

`query_language` 는 `flux` 또는 `influxql`. 응답은 time-series 결과 배열.

**[REQ-M3-03]** TSDB 는 기존 `POST /api/v1/tsdb/query` 를 그대로 재사용한다 (신규 구현 불필요).

**[REQ-M3-04]** M3 의 모든 쿼리 엔드포인트 응답은 **항상** 다음 표준 형식을 따라야 한다:

```json
{
  "entries": [{"timestamp": <ms>, "value": <any>, "labels": {...}}],
  "count": <정수>,
  "truncated": <bool>
}
```

**[REQ-M3-05]** M3 엔드포인트는 패널 초기 로드 용도로만 사용되며, 차트 패널이 **직접 호출하지는 않는다** (차트 패널은 WebSocket 만 구독). 사용자 도구(CLI, 디버깅 UI, 외부 스크립트)가 사용한다.

### M4: 5종 차트 패널 구현

**[REQ-M4-01]** 시스템은 **항상** 다음 5종 차트 패널 타입을 구현해야 한다:
- `stat` — 숫자 카드 (최신 값 + delta)
- `line-chart` — 시계열 선 차트
- `bar-chart` — 막대 차트 (카테고리 또는 시간 bin)
- `pie-chart` — 파이 차트 (카테고리 분포)
- `table` — 표 (행: entry, 열: payload 필드)

**[REQ-M4-02]** 각 차트 패널의 공통 config 필드는 **항상** 다음을 포함해야 한다:

```typescript
interface ChartPanelConfigBase {
  channel_name: string;            // 구독할 channel_name (required)
  display_field?: string;          // payload 에서 값으로 사용할 필드 경로 (default: "value")
  label_field?: string;            // 라벨로 사용할 필드 경로 (default: "labels.name")
  max_points?: number;             // 차트에 표시할 최대 항목 수 (default: 차트 타입별)
  refresh_on_reconnect?: boolean;  // WS 재연결 시 backfill 재요청 (default: true)
}
```

**[REQ-M4-03]** 시스템은 **항상** `stat` 을 제외한 차트 패널에 **Recharts 3.7.0** 을 사용해야 한다. `table` 은 일반 HTML `<table>` 사용.

**[REQ-M4-04]** `stat` 패널은 **항상** 링버퍼 최신 1개 항목의 `display_field` 값을 큰 숫자로 표시하고, 직전 항목 대비 delta (절대값 + 화살표 ↑/↓/→) 를 부가 표시해야 한다. 추가 config:

```typescript
{ unit?: string; decimal_places?: number; threshold_color_rules?: {min: number, color: string}[] }
```

**[REQ-M4-05]** `line-chart` 패널은 **항상** x축=timestamp, y축=`display_field` 로 최근 `max_points` (default 100) 개를 선으로 그려야 한다. 추가 config:

```typescript
{ y_min?: number; y_max?: number; smooth?: boolean; multi_series_field?: string }
```

`multi_series_field` 가 지정되면 해당 label 값별로 line 을 분리한다.

**[REQ-M4-06]** `bar-chart` 패널은 **항상** 다음 두 모드 중 하나를 지원해야 한다:
- **category** mode: `label_field` 별로 최신 값을 막대로 표시
- **time_bin** mode: `bin_sec` (default 60) 간격으로 시간 bin 별 집계 (count/sum/avg 중 `agg_func` 설정)

**[REQ-M4-07]** `pie-chart` 패널은 **항상** 최근 `max_points` (default 20) 항목을 `label_field` 별로 그룹화한 값의 비율을 파이로 표시해야 한다. 추가 config:

```typescript
{ agg_func?: "count" | "sum" | "avg"; show_legend?: boolean; show_percentage?: boolean }
```

**[REQ-M4-08]** `table` 패널은 **항상** 다음을 지원해야 한다:
- 각 행 = 하나의 entry
- 열 = `columns` config 로 지정된 payload 필드 목록 (default: `["timestamp", "value"]`)
- 열 헤더 클릭 시 해당 열 기준 정렬 토글 (asc / desc / unsorted)
- `timestamp` 열은 `YYYY-MM-DD HH:mm:ss.SSS` 로 포맷 표시

추가 config:

```typescript
{ columns: {field: string, header: string, format?: "datetime"|"number"|"string"}[]; rows_per_page?: number }
```

**[REQ-M4-09]** 모든 차트 패널은 **항상** 채널 연결 상태를 표시해야 한다: connecting / connected / disconnected / error. 연결 상태는 패널 우측 상단 작은 아이콘으로 표시 (SPEC-WEB-001 Module 20 의 connected 아이콘 패턴 재사용).

**[REQ-M4-10]** **WHEN** 채널 연결이 끊어지면 **THEN** 패널은 자동으로 재연결을 시도해야 한다 (exponential backoff: 1s → 2s → 4s → 8s → 16s max).

**[REQ-M4-11]** **WHEN** `chart.closed` 메시지를 수신하면 **THEN** 패널은 "Channel closed: {reason}" 안내를 표시하고 재연결을 시도하지 않아야 한다.

### M5: 대시보드 패널 설정 UI 확장

**[REQ-M5-01]** 시스템은 **항상** `AddPanelDialog` 에서 chart 계열 패널(`stat`/`line-chart`/`bar-chart`/`pie-chart`/`table`) 선택 시 `channel_name` 입력 필드를 제공해야 한다.

**[REQ-M5-02]** 시스템은 **항상** 현재 xflow 인스턴스에서 활성(running 상태)인 `chart-emitter` 노드의 `channel_name` 목록을 드롭다운으로 제시해야 한다. 드롭다운은 다음 API 로 조회: `GET /api/v1/charts/channels` → `{channels: [{name, flow_id, node_id, subscriber_count}]}`. 수동 입력도 허용 (아직 배포되지 않은 플로우 대비).

**[REQ-M5-03]** `PanelSettingsDialog` 에서 **항상** 각 차트 패널 타입에 맞는 config 필드 (REQ-M4-04 ~ REQ-M4-08) 를 편집할 수 있어야 한다. 폼은 `DynamicForm` (SPEC-WEB-001 Module 14) 의 `visibleWhen` 패턴으로 타입별 분기.

**[REQ-M5-04]** 시스템은 **항상** 패널 생성/편집 시 `channel_name` 형식(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)을 검증해야 한다. 잘못된 입력은 인라인 에러 메시지로 표시.

**[REQ-M5-05]** 시스템은 **항상** `PanelConfig.type` 의 5종 chart 타입에 대해 `panelDefaultSize` 매핑을 정의해야 한다 (SPEC-WEB-001 Module 30 패턴):
- `stat`: {w:2, h:1}
- `line-chart`: {w:6, h:3}
- `bar-chart`: {w:4, h:3}
- `pie-chart`: {w:3, h:3}
- `table`: {w:6, h:4}

### M6: 플로우 캔버스 UI 확장

**[REQ-M6-01]** `chart-emitter` 노드는 **항상** `nodeSchemas.ts` (SPEC-WEB-001 Module 30) 에 등록되어 `AddNodeDialog` 에서 선택 가능해야 한다.

**[REQ-M6-02]** `chart-emitter` 노드 편집 패널 (`PropertyPanel`) 에서 **항상** 다음 필드를 편집할 수 있어야 한다:
- `channel_name` (string, required, 형식 검증 포함)
- `buffer_size` (number, 1-10000)
- `retention_sec` (number, 0-86400)

**[REQ-M6-03]** `chart-emitter` 노드 카드는 **항상** 현재 채널 상태를 시각화해야 한다:
- 구독자 수 배지 (예: "👥 3")
- 최근 메시지 타임스탬프 (상대 시간, 예: "2s ago")

정보 소스는 기존 `node.stats` WebSocket 메시지 확장.

**[REQ-M6-04]** `chart-emitter` 노드는 **항상** `nodeTypeMeta.ts` 에서 category = "Output", icon = `<TrendingUp>` (lucide-react) 로 정의되어야 한다.

### M7: 필터 / 정렬 (기존 노드 활용 + 문서화) — 옵션

**[REQ-M7-01]** 시스템은 **항상** `docs/guides/chart-panel-flow.md` 에 다음 패턴을 문서화해야 한다:
- 기존 `filter` 노드 (predicate: `value > 30`) 를 `chart-emitter` 앞단에 배치하여 임계값 필터링
- 기존 `aggregate` 노드를 시간 bin 집계에 활용
- 기존 `mapping` / `transform` 노드를 필드 추출/이름 변경에 활용

**[REQ-M7-02]** 시스템은 **항상** 위 문서에서 플로우 예시 YAML 3종을 제공해야 한다:
- 단순 최신값 스탯: `store-read(latest) → chart-emitter`
- 임계값 필터 라인차트: `tsdb-query(range) → filter(value>threshold) → chart-emitter`
- 카테고리 분포 파이: `store-read(last_n) → aggregate(group_by=label) → chart-emitter`

**[REQ-M7-03]** (옵션) 시스템은 `sort` 노드를 신규 정의할 수 있다. config: `{field: string, order: "asc"|"desc", limit?: number}`. **본 SPEC 의 필수 범위는 아님** — 기존 `aggregate` 노드로 충분히 커버 가능하다면 생략한다.

---

## 4. Specifications (사양)

### 4.1 패킷 / 메시지 규격

#### 4.1.1 `chart-emitter` 입력 메시지 (정규화 후)

```typescript
type ChartEntry = {
  timestamp: number;          // epoch ms (int64), 없으면 chart-emitter 가 주입
  value: any;                 // 차트 값 (숫자/문자열/객체 모두 허용, 차트 타입별로 해석)
  labels?: Record<string, string>;  // 선택적 태그 (bar/pie 의 카테고리)
  meta?: Record<string, unknown>;   // 디버깅/추적용 메타데이터 (차트에 표시되지 않음)
};
```

#### 4.1.2 WebSocket 프레임

**서버 → 클라이언트:**

```typescript
type ChartBackfillMessage = {
  type: "chart.backfill";
  channel: string;
  entries: ChartEntry[];
  backfilled_count: number;
};

type ChartAppendMessage = {
  type: "chart.append";
  channel: string;
  entry: ChartEntry;
};

type ChartClosedMessage = {
  type: "chart.closed";
  channel: string;
  reason: "flow_undeployed" | "node_removed" | "buffer_overflow" | string;
};

type ChartErrorMessage = {
  type: "chart.error";
  channel: string;
  reason: "channel_not_found" | "invalid_channel_name" | string;
};
```

**클라이언트 → 서버:** (현재 범위에서는 단순 구독만. 향후 필요 시 `request_backfill` 등 추가 가능)

### 4.2 Config 스키마

#### 4.2.1 `chart-emitter` 노드 config

```yaml
# flow.yaml 예시
nodes:
  - id: my-emitter
    type: chart-emitter
    config:
      channel_name: "room1_temp"     # required, regex: ^[a-zA-Z][a-zA-Z0-9_-]{0,63}$
      buffer_size: 100                # default 100, range 1-10000
      retention_sec: 3600             # default 3600, range 0-86400
```

#### 4.2.2 PanelConfig.config (차트 패널별)

`stat`:
```typescript
{
  channel_name: string;
  display_field?: string;              // default "value"
  unit?: string;
  decimal_places?: number;             // default 2
  threshold_color_rules?: {min: number, color: string}[];
}
```

`line-chart`:
```typescript
{
  channel_name: string;
  display_field?: string;
  max_points?: number;                 // default 100
  y_min?: number;
  y_max?: number;
  smooth?: boolean;                    // default false
  multi_series_field?: string;         // e.g. "labels.room"
}
```

`bar-chart`:
```typescript
{
  channel_name: string;
  display_field?: string;
  mode: "category" | "time_bin";       // default "category"
  label_field?: string;                // category mode
  bin_sec?: number;                    // time_bin mode, default 60
  agg_func?: "count" | "sum" | "avg";  // default "avg"
  max_points?: number;                 // default 20
}
```

`pie-chart`:
```typescript
{
  channel_name: string;
  display_field?: string;              // default "value" (sum 대상)
  label_field?: string;                // default "labels.name"
  agg_func?: "count" | "sum" | "avg";  // default "sum"
  show_legend?: boolean;               // default true
  show_percentage?: boolean;           // default true
  max_points?: number;                 // default 20
}
```

`table`:
```typescript
{
  channel_name: string;
  columns: {
    field: string;                     // payload field path
    header: string;                    // display header
    format?: "datetime" | "number" | "string";
  }[];
  rows_per_page?: number;              // default 20
  max_points?: number;                 // default 200
  default_sort?: {field: string; order: "asc"|"desc"};
}
```

### 4.3 파일 구조

**백엔드 (신규):**

```
internal/node/chart_emitter.go                    # chart-emitter 노드 구현
internal/node/chart_emitter_test.go
internal/agent/system/chart_channel_registry.go   # 채널 레지스트리 (singleton)
internal/agent/system/chart_channel_registry_test.go
internal/api/handler/chart.go                      # GET /api/v1/charts/channels
internal/api/handler/store_query.go                # POST /api/v1/store/{agent}/query
internal/api/handler/influxdb_query.go             # POST /api/v1/influxdb/{agent}/query
internal/api/ws/chart_channel.go                   # /ws/chart/{channel_name} 핸들러
internal/api/ws/chart_channel_test.go
```

**백엔드 (수정):**

```
internal/agent/system/store.go                     # QueryHistory 를 HTTP 핸들러에서 호출 가능하게 export 확인
internal/agent/system/influxdb_agent.go            # Flux 쿼리 API 추가
internal/node/registry.go                          # chart-emitter 등록
internal/api/ws/hub.go                             # chart channel routing 통합
internal/api/router.go                             # 신규 HTTP/WS 엔드포인트 라우팅
```

**프론트엔드 (신규):**

```
web/src/pages/dashboard/panels/charts/StatPanel.tsx
web/src/pages/dashboard/panels/charts/LineChartPanel.tsx
web/src/pages/dashboard/panels/charts/BarChartPanel.tsx
web/src/pages/dashboard/panels/charts/PieChartPanel.tsx
web/src/pages/dashboard/panels/charts/TablePanel.tsx
web/src/pages/dashboard/panels/charts/useChartChannel.ts    # WS 구독 훅
web/src/pages/dashboard/panels/charts/chartChannelTypes.ts  # 공통 타입
web/src/services/ws/chartChannel.ts                          # WS 클라이언트 로직
```

**프론트엔드 (수정):**

```
web/src/pages/dashboard/DashboardPage.tsx                    # 5종 차트 패널 라우팅
web/src/pages/dashboard/AddPanelDialog.tsx                   # channel_name 드롭다운
web/src/pages/dashboard/PanelSettingsDialog.tsx              # 차트별 config 편집 폼
web/src/config/nodeSchemas.ts                                # chart-emitter 스키마
web/src/config/nodeTypeMeta.ts                               # chart-emitter 메타 (아이콘/카테고리)
web/src/config/panelDefaultSize.ts                           # 5종 차트 기본 크기
web/src/stores/uiStore.ts                                    # panel type union 확장 (이미 있음, 확인만)
```

**문서 (신규):**

```
docs/guides/chart-panel-flow.md                              # REQ-M7-01/02 가이드
```

### 4.4 채널 레지스트리 내부 구조 (Go)

```go
// internal/agent/system/chart_channel_registry.go (개념 스케치, 상세는 구현 단계)

type ChartEntry struct {
    Timestamp int64                  `json:"timestamp"`
    Value     any                    `json:"value"`
    Labels    map[string]string      `json:"labels,omitempty"`
    Meta      map[string]any         `json:"meta,omitempty"`
}

type ChartChannel struct {
    Name          string
    FlowID        string
    NodeID        string
    BufferSize    int
    RetentionSec  int
    ring          *RingBuffer[ChartEntry]  // thread-safe
    subscribers   map[*websocket.Conn]struct{}
    mu            sync.RWMutex
}

type ChartChannelRegistry interface {
    Register(name string, ch *ChartChannel) error   // fail if duplicate
    Unregister(name string) error
    Get(name string) (*ChartChannel, bool)
    List() []ChartChannelInfo
    Broadcast(name string, entry ChartEntry) error
}
```

---

## 5. Traceability (추적성)

### 5.1 Requirement ↔ File 매핑 (예정)

| Requirement ID | Component | 파일 |
|----------------|-----------|------|
| REQ-M1-01..10  | chart-emitter node | `internal/node/chart_emitter.go` |
| REQ-M1-02..07  | Ring buffer + retention | `internal/agent/system/chart_channel_registry.go` |
| REQ-M1-09      | Duplicate detection | `chart_channel_registry.go` (Register) |
| REQ-M2-01..07  | WebSocket handler | `internal/api/ws/chart_channel.go` |
| REQ-M3-01      | Store query API | `internal/api/handler/store_query.go` |
| REQ-M3-02      | InfluxDB query API | `internal/api/handler/influxdb_query.go` + `internal/agent/system/influxdb_agent.go` |
| REQ-M3-03      | TSDB (기존) | `internal/api/handler/tsdb.go` (변경 없음) |
| REQ-M4-01..11  | Chart panels | `web/src/pages/dashboard/panels/charts/*.tsx` |
| REQ-M5-01..05  | Panel dialogs | `AddPanelDialog.tsx`, `PanelSettingsDialog.tsx`, `panelDefaultSize.ts` |
| REQ-M6-01..04  | Flow canvas | `nodeSchemas.ts`, `nodeTypeMeta.ts`, PropertyPanel 확장 |
| REQ-M7-01..03  | 문서 | `docs/guides/chart-panel-flow.md` |

### 5.2 Upstream SPEC 참조

- **SPEC-STORE-002**: `Store.QueryHistory` 5-mode API 및 HistoryQuery 구조체 활용 (REQ-M3-01 요청 바디가 HistoryQuery 와 1:1 대응)
- **SPEC-TSDB-001**: 기존 `POST /api/v1/tsdb/query` 재사용 (REQ-M3-03)
- **SPEC-INFLUX-002**: InfluxDB 에이전트 확장 베이스 (REQ-M3-02)
- **SPEC-FLOW-001 / SPEC-NODE-004**: 노드 프레임워크, AgentRef fail-fast 패턴 (REQ-M1-09 준수)
- **SPEC-WEB-001 Module 12/16/18/22/26/28/30**: 대시보드 패널 프레임워크, DynamicForm, panelDefaultSize, nodeSchemas
- **SPEC-FILTER-001**: `filter` 노드 (REQ-M7-01 문서화 대상)
- **SPEC-AGG-001/002**: `aggregate` 노드 (REQ-M7-01 문서화 대상)

### 5.3 Downstream 영향

- 신규 WebSocket 경로 `/ws/chart/{channel}` 추가는 기존 WS 엔드포인트 (`/ws/flow`, `/ws/log` 등) 에 영향 없음.
- `chart-emitter` 노드 추가는 기존 노드 타입 목록 확장일 뿐 기존 동작에 영향 없음.
- Store/InfluxDB 에 추가되는 HTTP 엔드포인트는 기존 엔드포인트와 경로가 다르므로 영향 없음.

---

## 6. Non-Goals (명시적 범위 제외)

본 SPEC 은 다음을 **다루지 않는다**:

- ❌ 차트 패널 내부의 복잡한 필터/정렬 UI — 플로우 노드로 해결 (REQ-M7)
- ❌ 다중 채널을 하나의 차트에 합치는 기능 — 향후 확장 (단, `line-chart` 의 `multi_series_field` 로 부분 커버)
- ❌ 차트 export (PNG/CSV) — 향후 별도 SPEC
- ❌ 히스토리컬 리플레이 / time scrubber — 향후 별도 SPEC
- ❌ 차트 알람 (threshold-based alerting) — 기존 `filter` 노드 + 알람 에이전트로 조합
- ❌ Store / InfluxDB 쿼리 HTTP 엔드포인트의 인증/인가 — 기존 xflow 인증 모델 준수 (현재 미적용)
- ❌ `chart-emitter` 노드의 멀티 인스턴스(같은 channel) 자동 병합 — REQ-M1-09 에서 fail-fast 로 처리

---

## 7. Open Questions / 후속 검토 사항

다음 항목은 구현 단계에서 확정한다:

- **Q1**: `chart-emitter` 의 링버퍼가 메모리에만 존재하므로 xflow 재시작 시 초기화된다. Store 에이전트를 백엔드로 사용하는 하이브리드 옵션이 필요한가? → **M1 범위에서는 메모리 only**. 필요 시 후속 SPEC.
- **Q2**: `table` 패널의 페이지네이션 방식 (client-side vs server-side 추가 페이지 요청)? → **M4-08 범위에서는 client-side only** (최대 `max_points` 까지만 보관).
- **Q3**: `pie-chart` 의 카테고리가 너무 많을 때 (예: 50+) 처리? → 상위 N개만 표시 + "Others" 집계. 구현 단계에서 N=10 기본값으로 추가 config 제공.
- **Q4**: 플로우가 여러 인스턴스로 스케일 아웃될 때 채널 충돌? → 현재 xflow 는 단일 프로세스. 분산 시나리오는 future work.

---

**Status**: 본 SPEC 은 `/moai:2-run SPEC-CHART-001` 로 구현 단계 진입 준비 완료.
