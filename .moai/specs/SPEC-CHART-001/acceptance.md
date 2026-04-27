# SPEC-CHART-001 — Acceptance Criteria

> **관련 SPEC**: [spec.md](./spec.md) | **계획**: [plan.md](./plan.md)
> **검증 방식**: 자동 테스트 (Go test / frontend unit) + 수동 E2E 시나리오

---

## 1. Definition of Done (DoD)

본 SPEC 은 다음을 **모두** 충족하면 완료로 간주한다:

- [ ] 모든 P0/P1 요구사항 (M1, M2, M4-core, M5, M6) 구현 완료
- [ ] M3 HTTP 쿼리 엔드포인트 구현 및 테스트 통과
- [ ] M7 문서 (`docs/guides/chart-panel-flow.md`) 작성
- [ ] 백엔드 테스트 커버리지 85%+ (`go test -cover ./internal/node/chart_emitter... ./internal/agent/system/chart_channel... ./internal/api/ws/chart_channel...`)
- [ ] 프론트엔드 `useChartChannel` 훅 단위 테스트 통과
- [ ] Race condition 검증: `go test -race ./...` 통과
- [ ] 수동 E2E 시나리오 (아래 Section 4) 전부 통과
- [ ] TRUST 5 품질 게이트 통과 (LSP errors = 0, warnings ≤ 10)
- [ ] spec.md / plan.md / acceptance.md 상호 링크 검증
- [ ] HISTORY 블록 업데이트 (1.0.0 → 1.0.1 이상, `status: completed`)

---

## 2. 요구사항별 Acceptance Criteria

### M1: `chart-emitter` 노드 + 채널 레지스트리

#### REQ-M1-01 — 노드 타입 존재

**Given** xflow 백엔드가 실행 중일 때
**When** `GET /api/v1/nodes/types` 를 호출하면
**Then** 응답에 `"chart-emitter"` 타입이 포함되어야 한다.

#### REQ-M1-02 — Config 필드 스키마

**Given** `chart-emitter` 노드 스키마를 조회하면
**When** nodeSchemas 응답을 확인할 때
**Then** 필드 `channel_name` (string, required), `buffer_size` (number, default 100), `retention_sec` (number, default 3600) 이 정의되어야 한다.

#### REQ-M1-03 — 메시지 입력 시 브로드캐스트 + 링버퍼 저장

**Given** 활성 `chart-emitter` 노드 (channel_name="test1", buffer_size=100)
**And** 이미 연결된 WebSocket 구독자 1명
**When** 업스트림 노드가 payload `{timestamp: 1713312000000, value: 42.5}` 를 입력으로 전달하면
**Then** 구독자는 `{type:"chart.append", channel:"test1", entry:{timestamp:1713312000000, value:42.5}}` 메시지를 3초 이내에 수신한다.
**And** 링버퍼에는 이 항목이 저장된다 (다음 신규 구독자가 backfill 로 받을 수 있어야 함).

#### REQ-M1-04 — Backfill

**Given** `chart-emitter` 노드가 이전에 50개 메시지를 받은 상태
**When** 신규 WebSocket 구독자가 연결하면
**Then** 연결 직후 `{type:"chart.backfill", channel:"test1", entries:[...50개...], backfilled_count:50}` 메시지를 수신한다.
**And** backfill entries 는 timestamp 오름차순으로 정렬되어야 한다.
**And** backfill 은 **1회** 만 전송된다 (이후 append 로 전환).

#### REQ-M1-05 — Payload 정규화

- **Case A (timestamp 없음)**: 입력 `{value: 10}` → 링버퍼에 `{timestamp: <현재 epoch ms>, value: 10}` 저장
- **Case B (value 없음)**: 입력 `{room: "A", temp: 25}` → `{timestamp: <주입>, value: {room:"A", temp:25}}` 로 정규화
- **Case C (둘 다 있음)**: 입력 `{timestamp: 100, value: 5, labels: {x: "y"}}` → 변형 없이 그대로 저장

#### REQ-M1-06 — FIFO 제거

**Given** `buffer_size=5` 로 설정된 chart-emitter
**When** 7개 메시지를 순차 입력하면 (t=1,2,3,4,5,6,7)
**Then** 링버퍼에는 t=3,4,5,6,7 의 5개만 남아야 한다.

#### REQ-M1-07 — Retention 만료

**Given** `retention_sec=5` 로 설정된 chart-emitter, 현재 시각 t=100 (초)
**When** t=90, t=95, t=99 에 각각 1개씩 입력하고 시각이 t=101 로 진행되면
**Then** t=90 항목은 링버퍼에서 제거된다 (101-90=11 > 5).
**And** t=95, t=99 는 남아있어야 한다.

#### REQ-M1-08 — 플로우 undeploy 시 채널 해제

**Given** chart-emitter 를 포함한 플로우가 실행 중, 구독자 2명 연결
**When** 플로우를 undeploy (`POST /flows/{id}/undeploy`) 하면
**Then** 두 구독자 모두 `{type:"chart.closed", channel, reason:"flow_undeployed"}` 메시지 수신 후 연결 종료된다.
**And** `GET /api/v1/charts/channels` 응답에서 해당 채널이 제거된다.

#### REQ-M1-09 — 채널 이름 중복 시 fail-fast

**Given** 플로우 A 에서 `channel_name="dup1"` 인 chart-emitter 가 이미 실행 중
**When** 플로우 B 에서 같은 `channel_name="dup1"` 인 chart-emitter 의 Init 이 호출되면
**Then** Init 이 에러를 반환한다: `"channel_name \"dup1\" is already registered by flow <A-id> node <node-id>"`
**And** 플로우 B 는 Running 상태로 진입하지 않는다 (fail-fast).
**And** 경고 로그가 남는다.

#### REQ-M1-10 — 노드 포트 정의

**Given** chart-emitter 노드 스키마를 조회하면
**Then** 입력 포트 1개 (name="in"), 출력 포트 0개로 정의되어야 한다.

---

### M2: WebSocket 채널 프로토콜

#### REQ-M2-01 — 엔드포인트 존재

**Given** xflow 가 실행 중
**When** `ws://host/ws/chart/test1` 으로 WebSocket 연결 시도하면
**And** "test1" 채널이 활성 상태일 때
**Then** WebSocket 업그레이드가 성공한다 (HTTP 101).

#### REQ-M2-02 — Backfill 메시지 형식

REQ-M1-04 와 동일 (메시지 포맷: `type`, `channel`, `entries`, `backfilled_count`).

#### REQ-M2-03 — Append 메시지 형식

REQ-M1-03 와 동일 (메시지 포맷: `type`, `channel`, `entry`).

#### REQ-M2-04 — Fan-out (다중 구독자)

**Given** "test1" 채널에 구독자 3명이 연결된 상태
**When** chart-emitter 가 새 메시지 1개 수신
**Then** **3명 모두** 동일한 `chart.append` 메시지를 2초 이내에 수신한다.
**And** 한 구독자가 연결 종료해도 나머지 2명의 수신에는 영향 없다.

#### REQ-M2-05 — 미존재 채널 구독 에러

**Given** "nosuchchannel" 이라는 채널이 활성 상태가 아닐 때
**When** `ws://host/ws/chart/nosuchchannel` 으로 연결 시도
**Then** 서버는 `{type:"chart.error", channel:"nosuchchannel", reason:"channel_not_found"}` 전송 후 연결을 종료한다.

#### REQ-M2-06 — 연결 종료 시 구독자 제거

**Given** 구독자 2명이 연결된 채널
**When** 구독자 1명이 연결을 끊으면
**Then** 서버 측 구독자 리스트에서 해당 연결이 즉시 제거된다.
**And** 다음 브로드캐스트에서 해당 연결에 send 시도가 없다 (panic/error 없음).

#### REQ-M2-07 — 잘못된 channel_name 형식

**Given** `channel_name` 이 `abc/def` (슬래시 포함, 정규식 불일치)
**When** `ws://host/ws/chart/abc/def` 으로 연결 시도
**Then** HTTP 400 으로 응답하고 WebSocket 업그레이드가 일어나지 않는다.

---

### M3: HTTP 쿼리 엔드포인트

#### REQ-M3-01 — Store 쿼리 API

**Given** Store 에이전트 "room-store" 실행 중, key "a/b" 에 3개 값 저장 (v1, v2, v3)
**When** `POST /api/v1/store/room-store/query` with body `{"key":"a/b","mode":"last_n","count":2}`
**Then** 응답: `{"entries":[{timestamp:..., value:v2}, {timestamp:..., value:v3}], "count":2, "truncated":false}`
**And** 5가지 모드 (latest/last_n/duration/time_range/since_n) 모두 정상 응답한다.

#### REQ-M3-02 — InfluxDB 쿼리 API

**Given** InfluxDB 에이전트 "metrics" 실행 중, 기존 데이터 존재
**When** `POST /api/v1/influxdb/metrics/query` with body `{"query_language":"flux","query":"from(bucket:\"xflow\") |> range(start:-5m)"}`
**Then** 응답에 `entries` 배열이 포함되고 각 항목은 `{timestamp, value, labels}` 구조를 따른다.

#### REQ-M3-03 — TSDB 엔드포인트 미변경

**Given** 기존 `POST /api/v1/tsdb/query` 엔드포인트
**When** 기존 테스트 스위트 실행
**Then** 모든 기존 테스트가 통과한다 (regression 없음).

#### REQ-M3-04 — 표준 응답 형식

세 엔드포인트 모두 다음 공통 스키마를 따라야 한다:
```json
{ "entries": [{"timestamp": number, "value": any, "labels"?: object}], "count": number, "truncated": boolean }
```

#### REQ-M3-05 — 차트 패널의 HTTP 미호출

**Given** 5종 차트 패널이 활성 상태
**When** 네트워크 트래픽을 모니터링하면
**Then** 패널에서 `/api/v1/store/*/query`, `/api/v1/influxdb/*/query`, `/api/v1/tsdb/query` 로의 HTTP 요청이 발생하지 않는다 (WebSocket 만 사용).

---

### M4: 5종 차트 패널

#### REQ-M4-01 — 5종 타입 존재

**Given** 대시보드에서 "패널 추가" 버튼을 클릭
**When** 패널 타입 선택 UI 확인
**Then** `stat`, `line-chart`, `bar-chart`, `pie-chart`, `table` 5종 모두 선택 가능하다.

#### REQ-M4-02 — 공통 config 필드

**Given** 각 차트 패널 편집 대화상자
**Then** `channel_name` 필드가 공통으로 존재한다.
**And** `display_field`, `label_field`, `max_points` 필드가 각 타입에서 노출된다 (타입별 visibility 정책 적용).

#### REQ-M4-03 — Recharts 사용 확인

**Given** line/bar/pie 차트 패널이 렌더링된 상태
**When** DOM 검사
**Then** Recharts 가 생성한 SVG 요소 (`.recharts-wrapper`, `.recharts-surface`) 가 존재한다.
**And** `table` 은 일반 `<table>` 요소로 렌더링된다.

#### REQ-M4-04 — Stat 패널

**Given** channel_name="temp1" 에 연속 값 25.0, 26.5 가 발행됨
**When** Stat 패널이 이 채널 구독
**Then** 화면에 "26.5" 가 큰 숫자로 표시된다 (decimal_places=1 이면).
**And** "↑ +1.5" (직전 대비 증가) 가 보조 표시된다.

#### REQ-M4-05 — Line Chart

**Given** 연속된 100개 timestamp/value 쌍이 발행된 채널
**When** Line Chart 패널이 구독하고 `max_points=100`
**Then** 100개 포인트가 시간순 연결된 선이 표시된다.
**And** x축은 시간 (datetime format), y축은 value 의 숫자 스케일로 표시된다.
**And** `multi_series_field="labels.room"` 설정 시 `labels.room` 값별로 다른 색 line 으로 분리된다.

#### REQ-M4-06 — Bar Chart 2 modes

**Category mode**:
- **Given** entries: `[{value:10, labels:{name:"A"}}, {value:20, labels:{name:"B"}}]`
- **Then** x축에 A, B 라벨, y축에 10, 20 높이의 막대 2개

**Time bin mode**:
- **Given** 60초 bin, agg_func="avg", 1분간 5개 값 발행
- **Then** x축에 1분 간격 tick, y축에 해당 분의 평균 값으로 막대 하나

#### REQ-M4-07 — Pie Chart

**Given** 최근 20개 항목이 labels.category 별로 그룹핑되어 {A:50, B:30, C:20}
**When** Pie Chart 가 구독 (agg_func="count")
**Then** 각각 50%, 30%, 20% 비율의 3색 파이가 그려진다.
**And** show_percentage=true 일 때 각 슬라이스에 "%" 라벨이 표시된다.

#### REQ-M4-08 — Table

**Given** columns=`[{field:"timestamp",header:"Time",format:"datetime"}, {field:"value",header:"Value",format:"number"}]`
**When** 10개 항목이 표시됨
**Then** 2열 테이블로 10행 표시된다.
**And** "Time" 열 헤더 클릭 시 정렬 순서가 asc → desc → asc 로 토글된다.
**And** timestamp 값이 "2026-04-16 14:30:00.123" 포맷으로 표시된다.

#### REQ-M4-09 — 연결 상태 아이콘

**Given** 차트 패널 렌더링
**When** 우측 상단 확인
**Then** connecting (노란 원) / connected (녹색 원) / disconnected (회색 원) / error (빨간 삼각) 아이콘 중 하나가 표시된다.

#### REQ-M4-10 — 자동 재연결

**Given** 연결된 상태에서 네트워크 단절
**When** 3초 후
**Then** 패널이 자동 재연결 시도한다 (1s, 2s, 4s, 8s, 16s 순서).
**And** 성공 시 backfill 을 재수신하여 차트가 복구된다.

#### REQ-M4-11 — Closed 메시지 처리

**Given** 채널 구독 중
**When** 서버가 `chart.closed` 메시지 전송
**Then** 패널에 "Channel closed: flow_undeployed" 안내 텍스트가 표시된다.
**And** 자동 재연결이 시도되지 않는다.

---

### M5: 대시보드 설정 UI

#### REQ-M5-01 — 패널 추가 시 channel_name 입력

**Given** "패널 추가" 대화상자에서 `line-chart` 선택
**Then** `channel_name` 입력 필드가 나타난다 (required 표시).

#### REQ-M5-02 — 활성 채널 드롭다운

**Given** 현재 3개의 chart-emitter 가 활성 (channels: "a", "b", "c")
**When** AddPanelDialog 에서 channel_name 필드 클릭
**Then** 드롭다운에 "a", "b", "c" 3개 옵션이 표시된다.
**And** 각 옵션에는 flow_id, subscriber_count 보조 정보가 함께 표시된다.
**And** "Custom..." 옵션으로 수동 입력도 가능하다.

#### REQ-M5-03 — 설정 다이얼로그에서 편집

**Given** 기존 차트 패널의 톱니 아이콘 클릭
**Then** PanelSettingsDialog 가 열리고 타입별 config 필드가 편집 가능하다.
**And** 필드는 DynamicForm 의 visibleWhen 조건으로 타입별 분기되어 표시된다.

#### REQ-M5-04 — channel_name 검증

**Given** 입력 필드에 "abc/def" (슬래시 포함) 입력
**Then** 인라인 에러 메시지 "유효한 채널 이름이 아닙니다..." 표시
**And** 저장 버튼이 비활성화된다.

#### REQ-M5-05 — 기본 패널 크기

**Given** 패널 추가 시
**Then** 각 타입별로 spec.md 에 명시된 기본 크기로 생성된다:
- stat: 2×1, line: 6×3, bar: 4×3, pie: 3×3, table: 6×4

---

### M6: 플로우 캔버스

#### REQ-M6-01 — AddNodeDialog 등록

**Given** 플로우 캔버스의 "노드 추가" 대화상자
**Then** "Output" 카테고리 하위에 `chart-emitter` 가 선택 가능하다.

#### REQ-M6-02 — PropertyPanel 편집

**Given** 캔버스에 chart-emitter 노드 선택
**Then** PropertyPanel 에 channel_name, buffer_size, retention_sec 편집 필드가 표시된다.
**And** 각 필드의 유효성 검증이 즉시 피드백된다.

#### REQ-M6-03 — 노드 카드 상태 시각화

**Given** 실행 중인 chart-emitter 노드
**Then** 노드 카드에 구독자 수 배지 (예: "👥 3") 가 표시된다.
**And** 최근 메시지 발행 시각이 "2s ago" 형식으로 표시된다.
**And** 값은 실시간 (1-3초 간격) 업데이트된다.

#### REQ-M6-04 — 아이콘 및 카테고리

**Given** chart-emitter 노드 카드
**Then** 아이콘은 TrendingUp (lucide-react) 이다.
**And** 카테고리는 "Output" 이다.

---

### M7: 필터/정렬 문서

#### REQ-M7-01 — 가이드 문서 존재

**Given** 저장소에 `docs/guides/chart-panel-flow.md` 파일이 존재
**Then** 기존 `filter`, `aggregate`, `mapping`, `transform` 노드를 chart-emitter 와 조합하는 예시가 포함되어야 한다.

#### REQ-M7-02 — 3종 예시 YAML

다음 예시가 문서에 포함되어야 한다:
1. 최신값 스탯: `store-read(latest) → chart-emitter`
2. 임계값 필터 라인: `tsdb-query → filter(v>threshold) → chart-emitter`
3. 카테고리 분포 파이: `store-read(last_n) → aggregate(group_by) → chart-emitter`

각 예시는 실제 배포 가능한 YAML 이어야 한다.

---

## 3. Non-Functional Acceptance

- [ ] **성능**: 100 entries 링버퍼 backfill < 300ms (localhost 기준)
- [ ] **동시성**: 50 채널 × 5 구독자 동시 운영 시 메모리 < 200MB 증가
- [ ] **안정성**: `go test -race ./...` 통과 (race condition 없음)
- [ ] **보안**: channel_name 정규식 우회 시도 (path traversal, SQL injection 유사 문자열) 모두 400 차단
- [ ] **호환성**: 기존 SPEC-WEB-001, SPEC-STORE-002, SPEC-TSDB-001 의 테스트가 모두 여전히 통과

---

## 4. 수동 E2E Demo Flow

### Demo Scenario 1: 단순 스탯 패널

**Step 1** — 플로우 정의 (flow.yaml):
```yaml
nodes:
  - id: timer
    type: timer
    config: { interval: 2s }
  - id: rand
    type: random-value
    config: { min: 20, max: 30 }
  - id: emit
    type: chart-emitter
    config: { channel_name: "demo_temp", buffer_size: 50 }
edges:
  - { from: "timer.out", to: "rand.in" }
  - { from: "rand.out",  to: "emit.in"  }
```

**Step 2** — `moai flow deploy demo.yaml && moai flow start demo`

**Step 3** — 대시보드에서:
- "패널 추가" → `stat` 선택
- channel_name 드롭다운에서 "demo_temp" 선택
- display_field = "value", unit = "°C", decimal_places = 1
- 저장

**Expected**:
- 2초마다 숫자가 업데이트된다
- delta 화살표가 표시된다 (±)

### Demo Scenario 2: 라인 차트 + 필터

**Step 1** — 플로우:
```yaml
nodes:
  - id: q
    type: tsdb-query
    config: { agent_ref: tsdb, key: temp, range_sec: 300 }
  - id: f
    type: filter
    config: { predicate: "value > 25" }
  - id: emit
    type: chart-emitter
    config: { channel_name: "hot_temps" }
edges:
  - { from: "q.out", to: "f.in" }
  - { from: "f.out", to: "emit.in" }
```

**Step 2** — Line Chart 패널 추가, channel_name = "hot_temps", max_points = 100

**Expected**:
- 25도를 초과한 값만 선 그래프에 표시된다
- 5분 범위 데이터가 backfill 로 먼저 채워지고, 이후 실시간 업데이트된다

### Demo Scenario 3: Fan-out (한 채널 여러 패널)

**Given** Scenario 1 의 "demo_temp" 채널 활성

**Step 1** — 대시보드에 3개 패널 추가:
- Stat 패널 (channel: demo_temp)
- Line Chart 패널 (channel: demo_temp, max_points: 30)
- Table 패널 (channel: demo_temp, columns: [timestamp, value])

**Expected**:
- 모두 같은 순간에 업데이트된다 (fan-out 정상 동작)
- `GET /api/v1/charts/channels` 에서 "demo_temp" 의 subscriber_count 가 3 이다

### Demo Scenario 4: 재연결

**Given** Scenario 1 이 정상 동작 중

**Step 1** — xflow 서버를 10초간 중단했다가 재시작

**Expected**:
- Stat 패널 우측 상단 아이콘이 connected → disconnected → connecting → connected 로 전이
- 재연결 후 backfill 을 통해 차트가 즉시 복구된다

### Demo Scenario 5: 중복 채널 fail-fast

**Given** "dup_chan" channel_name 을 가진 chart-emitter 가 플로우 A 에서 실행 중

**Step 1** — 플로우 B 에서 동일 channel_name 으로 chart-emitter 를 포함하고 deploy + start 시도

**Expected**:
- 플로우 B 의 Init 이 에러 반환
- 로그에 경고 메시지 (flow A 의 id 명시)
- 플로우 B 는 Running 상태로 진입 안 함
- 플로우 A 는 계속 정상 동작

### Demo Scenario 6: 플로우 undeploy 시 정리

**Given** Scenario 1 실행 중, Stat 패널 구독 중

**Step 1** — `moai flow undeploy demo`

**Expected**:
- Stat 패널에 "Channel closed: flow_undeployed" 메시지 표시
- 자동 재연결 시도 안 함
- `GET /api/v1/charts/channels` 응답에서 "demo_temp" 삭제됨

### Demo Scenario 7: HTTP 쿼리 API (외부 도구 검증)

**Step 1** — curl:
```bash
curl -X POST http://localhost:8081/api/v1/store/room-store/query \
  -H 'Content-Type: application/json' \
  -d '{"key":"room1/temp","mode":"last_n","count":10}'
```

**Expected**: 표준 응답 스키마 `{entries, count, truncated}` 로 최근 10개 항목 반환

---

## 5. 검증 우선순위 매트릭스

| 우선순위 | 요구사항 그룹 | 검증 방법 | 자동화 여부 |
|---------|--------------|----------|-----------|
| P0 (blocking) | M1-01~10, M2-01~07, M4-01~11 | Go unit + integration test, React component test | 자동 |
| P0 (blocking) | Demo Scenario 1, 3, 5 | 수동 E2E | 수동 |
| P1 | M3-01~05, M5-01~05, M6-01~04 | Go/React test + 수동 UI 검증 | 혼합 |
| P1 | Demo Scenario 2, 4, 6 | 수동 E2E | 수동 |
| P2 | M7-01, M7-02 (문서) | 문서 리뷰 + 예시 YAML deploy 검증 | 수동 |
| P3 | M7-03 (옵션 sort 노드) | 구현 시 Go unit test | 자동 |

---

## 6. Regression Checklist

본 SPEC 구현이 **기존 기능을 손상시키지 않는지** 확인:

- [ ] `go test ./...` 전체 통과 (기존 SPEC 테스트 포함)
- [ ] 기존 WebSocket 메시지 타입 (`flow.status`, `agent.status`, `log.entry`, `device.status` 등) 모두 정상 동작
- [ ] 기존 대시보드 패널 타입 (AcControl, HvacControl, Gauge, Device, Agent, Flow, Log 등) 모두 정상 동작
- [ ] 기존 노드 타입 (`store-read`, `tsdb-query`, `filter`, `aggregate` 등) 동작 변경 없음
- [ ] SPEC-STORE-002 의 store-read/store-write Init fail-fast 동작 유지
- [ ] `POST /api/v1/tsdb/query` 응답 포맷 변경 없음

---

## 7. Sign-Off

- [ ] 구현자 자가검증 (acceptance.md 체크리스트 완료)
- [ ] manager-quality 의 TRUST 5 게이트 통과
- [ ] 수동 E2E Demo Scenario 1-7 모두 통과
- [ ] 사용자(xtra) 최종 승인

**승인 완료 시** → `/moai:3-sync SPEC-CHART-001` 로 문서/버전 동기화 → SPEC.md status 를 `implemented` 로 변경.
