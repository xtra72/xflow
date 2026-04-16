# 차트 패널 플로우 연동 가이드

> **관련 SPEC**: [SPEC-CHART-001](../../.moai/specs/SPEC-CHART-001/spec.md)
> **최종 업데이트**: 2026-04-16

이 가이드는 xflow 대시보드의 차트 패널(Stat/Line/Bar/Pie/Table)에 데이터를 공급하는
**플로우 기반 파이프라인** 구성 방법을 설명합니다. 차트 패널은 SQL/Flux 를 직접
질의하지 않고, 플로우 캔버스에서 구성된 노드 그래프의 종단(`chart-emitter` 노드) 이
발행하는 WebSocket 채널을 구독합니다.

---

## 1. 아키텍처 개요

```
┌──────────────────────────────────────────────────────────────┐
│  플로우 캔버스                                                │
│                                                              │
│  [데이터 소스]──→ [필터/집계/매핑]──→ [chart-emitter]         │
│   store-read       filter                 │                  │
│   tsdb-query       aggregate              ▼                  │
│   influxdb-query   mapping           /ws/chart/{channel}     │
│   ...              transform              │                  │
│                                           ▼                  │
└──────────────────────────────────────────┼───────────────────┘
                                           │ WebSocket
                                           ▼
              ┌───────────────────────────────────────────┐
              │ 대시보드 차트 패널                         │
              │  Stat / Line / Bar / Pie / Table          │
              │  (useChartChannel 훅으로 구독)            │
              └───────────────────────────────────────────┘
```

**핵심 원칙:**

1. 차트 패널은 데이터 소스를 몰라야 한다 — 오직 `channel_name` 만 안다.
2. 필터링/집계/정렬은 플로우 노드가 담당한다.
3. `chart-emitter` 는 종단 노드 (sink). 입력 포트 1개, 출력 포트 0개.
4. 채널은 링버퍼 보유 → 신규 구독자는 backfill 로 과거 데이터를 즉시 수신.
5. 모든 타임스탬프는 **epoch ms (int64)** 로 통일.

---

## 2. `chart-emitter` 노드 설정

| 필드            | 타입   | 기본값 | 설명                                                                        |
| --------------- | ------ | ------ | --------------------------------------------------------------------------- |
| `channel_name`  | string | 필수   | 고유 채널 이름. 정규식 `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`. 중복 시 Init 실패.  |
| `buffer_size`   | int    | 100    | 링버퍼 최대 크기 (1-10000). 신규 구독자에게 backfill 로 즉시 전달.          |
| `retention_sec` | int    | 3600   | 링버퍼 항목 최대 보존 시간 (초, 0-86400). 0 이면 시간 만료 비활성.          |

### 입력 payload 정규화 규칙

`chart-emitter` 는 입력 메시지의 payload 를 다음과 같이 정규화합니다:

| 입력 payload                                       | 정규화 결과                                                                 |
| -------------------------------------------------- | --------------------------------------------------------------------------- |
| `{"timestamp": 1713312000000, "value": 42.5}`      | 변형 없음                                                                   |
| `{"value": 42.5}` (timestamp 누락)                 | `{"timestamp": <현재 epoch ms>, "value": 42.5}`                             |
| `{"room": "A", "temp": 25}` (value 누락)           | `{"timestamp": <주입>, "value": {"room": "A", "temp": 25}}`                |
| `{"value": 42, "labels": {"r": "A"}, "meta": {}}`  | 변형 없음 (labels/meta 선택적 passthrough)                                  |

### 실패 조건 (fail-fast)

- `channel_name` 형식 검증 실패 → 노드 Configure 에러
- `channel_name` 중복 등록 → 노드 Init 에러 (에러 메시지에 기존 flow_id/node_id 포함)
- `buffer_size` / `retention_sec` 범위 초과 → Configure 에러

---

## 3. 예시 플로우 3종

### Example 1: 최신값 스탯

**시나리오:** Store 에이전트에 저장된 최신 온도를 Stat 패널에 표시.

```yaml
# flow.yaml
id: demo-latest-temp
nodes:
  - id: read-temp
    type: store-read
    config:
      agent_ref: room-store
      key_template: room1/temp
      read_mode: latest
      output_key: value

  - id: emit-stat
    type: chart-emitter
    config:
      channel_name: room1_temp_stat
      buffer_size: 10
      retention_sec: 300

edges:
  - from: read-temp.out
    to: emit-stat.in
```

**대시보드 설정:**

- 패널 추가 → `stat` 선택
- `channel_name` 드롭다운에서 `room1_temp_stat` 선택
- `display_field` = `value`, `unit` = `°C`, `decimal_places` = 1

---

### Example 2: 임계값 필터 라인 차트

**시나리오:** TSDB 에서 최근 5분간 온도 데이터를 가져오되, 25°C 초과 값만 라인 차트에 표시.

```yaml
# flow.yaml
id: demo-hot-temps
nodes:
  - id: q
    type: tsdb-query
    config:
      agent_ref: tsdb-main
      key: temp
      range_sec: 300

  - id: f
    type: filter
    config:
      condition: payload.value > 25

  - id: emit
    type: chart-emitter
    config:
      channel_name: hot_temps
      buffer_size: 200
      retention_sec: 3600

edges:
  - from: q.out
    to: f.in
  - from: f.out
    to: emit.in
```

**대시보드 설정:**

- 패널 추가 → `line-chart` 선택
- `channel_name` = `hot_temps`, `max_points` = 100
- `y_min` = 25 (하한 강제)

**활용 노드:**

- `filter`: `condition` 필드에 예: `payload.value > 25`, `payload.level == "error"`
- 필터 통과 메시지만 `out` 포트로, 그 외는 `reject` 포트로 (연결 없으면 폐기)

---

### Example 3: 카테고리 분포 파이 차트

**시나리오:** Store 에서 최근 20개 이벤트를 가져와 `labels.category` 별로 집계하여 파이 차트에 표시.

```yaml
# flow.yaml
id: demo-category-pie
nodes:
  - id: read-events
    type: store-read
    config:
      agent_ref: event-store
      key_template: events/all
      read_mode: last_n
      count: 20
      output_key: entries

  - id: agg
    type: aggregate
    config:
      group_by: labels.category
      agg_func: count

  - id: emit
    type: chart-emitter
    config:
      channel_name: event_categories
      buffer_size: 50
      retention_sec: 600

edges:
  - from: read-events.out
    to: agg.in
  - from: agg.out
    to: emit.in
```

**대시보드 설정:**

- 패널 추가 → `pie-chart` 선택
- `channel_name` = `event_categories`
- `label_field` = `labels.category`
- `agg_func` = `count`
- `show_legend` = true, `show_percentage` = true

---

## 4. 자주 쓰는 노드 조합 패턴

### 4.1 임계값 경보 스타일 (filter 앞단)

```
tsdb-query → filter(value > threshold) → chart-emitter
```

`filter.condition` 식 예:

- 단일 조건: `payload.value > 30`
- 복합 조건: `payload.value > 30 && payload.sensor == "temp"`
- 정규식: `payload.name =~ "room[0-9]+"`

### 4.2 시간 bin 집계 (aggregate)

```
store-read(last_n) → aggregate(bin_sec=60, agg_func=avg) → chart-emitter
```

`bar-chart` 패널의 `time_bin` 모드와 조합하면 분 단위 평균 막대 차트가 완성됩니다.

### 4.3 필드 추출/이름 변경 (mapping / transform)

```
tsdb-query → mapping(rename: sensor_temp → value) → chart-emitter
```

대시보드 패널이 기대하는 `display_field` 형태로 payload 구조를 맞출 때 사용합니다.

---

## 5. 운영 주의사항

### 5.1 채널 이름 규칙

- **반드시 유일해야 합니다** — xflow 인스턴스 내에서 `channel_name` 중복 시 나중에 시작된
  플로우의 `chart-emitter` Init 이 실패합니다 (fail-fast).
- 권장 네이밍: `<domain>_<metric>_<scope>` 예: `room1_temp_stat`, `pump_rpm_line`.
- 공백 / 슬래시 / 점 / 한글 사용 불가 (정규식 위반).

### 5.2 링버퍼 메모리

- 채널당 메모리 = `buffer_size × ~1KB` (평균). 기본 100개 × 1KB = ~100KB.
- 동시 활성 채널 50개 기준 ~5MB — 일반적인 배포에서 문제 없음.
- `retention_sec > 0` 이면 10초 간격으로 만료된 항목이 백그라운드 goroutine 에서 스윕됩니다.

### 5.3 플로우 undeploy 시 동작

- `chart-emitter` 가 포함된 플로우를 undeploy 하면:
  1. 모든 구독자가 `{type:"chart.closed", channel, reason:"flow_undeployed"}` 수신
  2. 링버퍼 해제
  3. `GET /api/v1/charts/channels` 응답에서 해당 채널 제거
- 차트 패널은 `chart.closed` 수신 후 **자동 재연결을 시도하지 않습니다** (무한 루프 방지).
- 플로우를 재배포하면 패널의 새로고침 버튼을 눌러 재구독해야 합니다.

### 5.4 재연결 정책 (차트 패널)

- 네트워크 단절 / 서버 재시작 시 exponential backoff: **1s → 2s → 4s → 8s → 16s** (최대).
- 재연결 성공 시 자동으로 `chart.backfill` 수신 → 차트가 즉시 복구됩니다.

### 5.5 Fan-out (다수 패널 공유)

- 하나의 `chart-emitter` 채널에 **여러 차트 패널이 동시에 구독** 가능합니다.
- 예: `room1_temp` 채널 하나를 Stat 패널 + Line Chart 패널 + Table 패널이 동시 표시.
- 구독자 수는 `GET /api/v1/charts/channels` 응답의 `subscriber_count` 에서 확인 가능.

---

## 6. HTTP 쿼리 API (외부 도구 / 디버깅용)

차트 패널은 WebSocket 만 사용하지만, 외부 CLI / 스크립트가 직접 데이터를 조회해야 할
때는 다음 HTTP 엔드포인트를 사용합니다 (SPEC REQ-M3-01..04):

### Store 조회

```bash
curl -X POST http://localhost:8081/api/v1/store/room-store/query \
  -H 'Content-Type: application/json' \
  -d '{
    "key": "room1/temp",
    "mode": "last_n",
    "count": 10,
    "namespace": "default"
  }'
```

### InfluxDB Flux 쿼리

```bash
curl -X POST http://localhost:8081/api/v1/influxdb/metrics/query \
  -H 'Content-Type: application/json' \
  -d '{
    "query_language": "flux",
    "query": "from(bucket:\"xflow\") |> range(start:-1h) |> filter(fn:(r) => r._field == \"temp\")"
  }'
```

### TSDB (기존 엔드포인트)

```bash
curl -X POST http://localhost:8081/api/v1/tsdb/query \
  -H 'Content-Type: application/json' \
  -d '{ "series_key": "temp", "start_ms": 0, "end_ms": 0 }'
```

세 엔드포인트 모두 공통 응답 스키마를 따릅니다:

```json
{
  "success": true,
  "data": {
    "entries": [
      { "timestamp": 1713312000000, "value": 42.5, "labels": { "room": "r1" } }
    ],
    "count": 1,
    "truncated": false
  }
}
```

---

## 7. 문제 해결 (Troubleshooting)

| 증상                                            | 원인                                           | 조치                                                                     |
| ----------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------ |
| 패널에 "channel_not_found" 표시                 | 플로우가 아직 배포/시작되지 않음               | 플로우를 deploy + start 하거나 `channel_name` 오타 확인                  |
| Init 에러 `already registered by flow ...`      | 동일 `channel_name` 의 emitter 가 이미 실행 중 | 기존 플로우의 채널 이름 변경 또는 해당 플로우 undeploy                   |
| 차트가 비어있음 (backfill 도 0개)               | 링버퍼에 아직 데이터 없음 (신규 플로우)        | 업스트림 노드가 동작 중인지 확인. Debug 노드로 메시지 흐름 검사          |
| 재연결 아이콘이 계속 회전                       | 백엔드 unreachable 또는 잘못된 `channel_name`  | 브라우저 개발자도구 네트워크 탭에서 WS 상태 확인                         |
| 차트 값이 이상함 (객체로 표시)                  | `display_field` 경로 불일치                    | `labels.x` 같은 정확한 경로 설정 또는 `mapping` 노드로 value 필드 추출   |
| `retention_sec=0` 인데도 오래된 값 사라짐       | `buffer_size` FIFO 용량 초과                   | `buffer_size` 를 늘리거나 `retention_sec` 병행 사용                      |

---

## 8. 참고 문서

- [SPEC-CHART-001](../../.moai/specs/SPEC-CHART-001/spec.md) — 전체 명세
- [SPEC-STORE-002](../../.moai/specs/SPEC-STORE-002/spec.md) — Store 에이전트 `QueryHistory` 5-mode
- [SPEC-TSDB-001](../../.moai/specs/SPEC-TSDB-001/spec.md) — TSDB 에이전트
- [SPEC-INFLUX-002](../../.moai/specs/SPEC-INFLUX-002/spec.md) — InfluxDB 에이전트
- [SPEC-WEB-001](../../.moai/specs/SPEC-WEB-001/spec.md) — 대시보드 패널 프레임워크
