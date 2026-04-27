# SPEC-CHART-001 — 구현 계획

> **관련 SPEC**: [spec.md](./spec.md)
> **방법론**: Hybrid (기존 코드 확장은 DDD / 신규 파일은 TDD)
> **우선순위 전략**: P0 (필수 백엔드) → P1 (필수 프론트엔드) → P2 (UX 향상) → P3 (옵션 문서/노드)

---

## 1. 구현 목표

사용자 승인 scope 대로 **플로우 기반 차트 패널 연동 시스템** 을 구축한다:

1. 플로우 노드 `chart-emitter` 가 종단점 역할로 WebSocket 채널에 데이터를 발행
2. 5종 차트 패널 (stat/line/bar/pie/table) 이 채널을 구독하여 렌더링
3. 필터/집계는 기존 `filter`/`aggregate` 노드로 플로우 상에서 해결
4. Store/InfluxDB 에 HTTP 쿼리 엔드포인트 추가 (보조, 초기 로드 및 외부 도구용)

**핵심 수치 목표:**
- 5종 차트 패널 각각 정상 렌더링
- WebSocket 채널 fan-out (1채널 ↔ N패널) 동작
- 링버퍼 backfill ≤ 300ms (100 entries 기준)
- 차트 패널 재연결 자동 복구 (exponential backoff)

---

## 2. 마일스톤 (우선순위 기반, 시간 예측 없음)

### Primary Goal (필수 최우선)

**M1 — `chart-emitter` 노드 + 채널 레지스트리 (백엔드 코어)**

- REQ-M1-01 ~ REQ-M1-10
- 핵심: 노드 구현, 링버퍼, 채널 레지스트리 (singleton), fail-fast 중복 감지
- 산출물:
  - `internal/agent/system/chart_channel_registry.go` + test
  - `internal/node/chart_emitter.go` + test
  - `internal/node/registry.go` 업데이트 (신규 노드 등록)

**M2 — WebSocket `/ws/chart/{channel}` 엔드포인트**

- REQ-M2-01 ~ REQ-M2-07
- 핵심: 구독자 관리, backfill + append 메시지 전송, 재연결 처리, channel_name 검증
- 산출물:
  - `internal/api/ws/chart_channel.go` + test
  - `internal/api/ws/hub.go` 또는 `internal/api/router.go` 라우팅 통합

### Secondary Goal (필수 두번째)

**M4-core — 5종 차트 패널 (프론트엔드)**

- REQ-M4-01 ~ REQ-M4-11
- 핵심: WS 구독 훅, Recharts 기반 4종 + HTML table 1종, 재연결 로직
- 산출물:
  - `web/src/services/ws/chartChannel.ts`
  - `web/src/pages/dashboard/panels/charts/useChartChannel.ts`
  - `web/src/pages/dashboard/panels/charts/{Stat,LineChart,BarChart,PieChart,Table}Panel.tsx`
  - `web/src/pages/dashboard/DashboardPage.tsx` 라우팅 업데이트

**M5 — 대시보드 설정 UI 확장**

- REQ-M5-01 ~ REQ-M5-05
- 핵심: AddPanelDialog 의 channel 드롭다운 (REQ-M5-02 의 `/api/v1/charts/channels` 엔드포인트 필요), PanelSettingsDialog 확장
- 산출물:
  - `internal/api/handler/chart.go` (GET /api/v1/charts/channels)
  - `web/src/pages/dashboard/AddPanelDialog.tsx`
  - `web/src/pages/dashboard/PanelSettingsDialog.tsx`
  - `web/src/config/panelDefaultSize.ts` 확장

**M6 — 플로우 캔버스 노드 통합**

- REQ-M6-01 ~ REQ-M6-04
- 핵심: nodeSchemas / nodeTypeMeta 등록, PropertyPanel 편집, 노드 카드 상태 표시
- 산출물:
  - `web/src/config/nodeSchemas.ts` (chart-emitter 스키마)
  - `web/src/config/nodeTypeMeta.ts` (chart-emitter 메타)
  - PropertyPanel 에 chart-emitter 용 편집 필드 (기존 DynamicForm 으로 자동 렌더)

### Final Goal (HTTP 보조 API + 문서)

**M3 — HTTP 쿼리 엔드포인트 (Store / InfluxDB)**

- REQ-M3-01 ~ REQ-M3-05
- 핵심: Store 의 HistoryQuery 를 HTTP 노출, InfluxDB 에 Flux 쿼리 API 추가
- 산출물:
  - `internal/api/handler/store_query.go` + test
  - `internal/api/handler/influxdb_query.go` + test
  - `internal/agent/system/influxdb_agent.go` Flux 쿼리 메서드 추가
  - `internal/api/router.go` 라우팅

**M7 — 문서화 (활용 가이드)**

- REQ-M7-01 ~ REQ-M7-02
- 산출물:
  - `docs/guides/chart-panel-flow.md` (3종 예시 플로우 + 기존 노드 활용법)

### Optional Goal (여유 있을 때)

**M7-opt — `sort` 신규 노드**

- REQ-M7-03
- 기존 `aggregate` 로 대체 가능하므로 생략 가능. 구현하더라도 본 SPEC 의 acceptance 를 막지는 않음.

---

## 3. 의존성 그래프

```
M1 (chart-emitter + registry)
 ├─→ M2 (WS endpoint: registry.Get + broadcast)
 ├─→ M5 (list channels API: registry.List)
 └─→ M6 (flow canvas registration)

M2 (WS endpoint)
 └─→ M4-core (프론트엔드 훅이 이 엔드포인트에 연결)

M4-core ── needs ──→ M2
M5       ── needs ──→ M1 (registry list), M4-core (패널 타입 라우팅 존재)
M6       ── needs ──→ M1 (nodeSchemas 에 chart-emitter 필드 정의 동기화)

M3 (HTTP query APIs) — 독립적. M1/M2 와 병렬 가능.
M7 (docs) — M1/M2/M4-core/M5/M6 완료 후 통합 시연 가능한 시점에 작성.
```

## 4. 작업 순서 (권장)

### Phase 1 — 백엔드 코어 (병렬 가능)

1. **Task-B1**: `ChartChannelRegistry` + `ChartChannel` 타입 및 링버퍼 구현 (`chart_channel_registry.go`)
   - 테스트 먼저 (TDD): Register 중복 시 에러, buffer_size 초과 FIFO, retention 만료 스윕
2. **Task-B2**: `chart-emitter` 노드 구현 (`chart_emitter.go`)
   - 테스트 먼저: Init 실패 케이스 (channel_name 검증, 중복 채널), Process 에서 timestamp 주입, undeploy 시 채널 해제
3. **Task-B3**: WebSocket 핸들러 (`chart_channel.go`)
   - 테스트 먼저: 구독 → backfill 수신, append 브로드캐스트, 채널 미존재 에러, 연결 종료 시 clean-up
4. **Task-B4**: HTTP API 엔드포인트 — 독립적이므로 B1-B3 와 병렬 가능
   - `GET /api/v1/charts/channels` (M5 지원)
   - `POST /api/v1/store/{agent}/query` (M3-01)
   - `POST /api/v1/influxdb/{agent}/query` (M3-02, InfluxDB 에이전트 Flux 쿼리 메서드 선행)

### Phase 2 — 프론트엔드 (B1-B3 완료 후 시작)

5. **Task-F1**: WebSocket 클라이언트 + React 훅 (`chartChannel.ts`, `useChartChannel.ts`)
   - 재연결 로직 (exponential backoff), backfill → append 전환, 링버퍼 (프론트엔드 측 보조 캐시)
6. **Task-F2**: 5종 차트 패널 컴포넌트 구현 (병렬)
   - `StatPanel`, `LineChartPanel`, `BarChartPanel`, `PieChartPanel`, `TablePanel`
   - 각각 기존 `GaugePanel`, `AcControlPanel` 등 코드 스타일 참조
7. **Task-F3**: DashboardPage 라우팅 업데이트 (L383-L396 의 플레이스홀더 → 실제 패널)
8. **Task-F4**: AddPanelDialog / PanelSettingsDialog 확장 (M5)
   - `/api/v1/charts/channels` 조회 드롭다운
   - DynamicForm 의 visibleWhen 으로 차트 타입별 분기

### Phase 3 — 플로우 캔버스 (B1-B2 완료 후)

9. **Task-F5**: `nodeSchemas.ts` / `nodeTypeMeta.ts` 에 chart-emitter 등록
10. **Task-F6**: 노드 카드 상태 표시 (구독자 수, 최근 메시지 시각)
    - 기존 `node.stats` WebSocket 이벤트 확장 or `chart-emitter` 전용 이벤트 신설

### Phase 4 — 문서 + 통합 검증

11. **Task-D1**: `docs/guides/chart-panel-flow.md` 작성 (3종 예시 플로우 YAML)
12. **Task-D2**: E2E 시나리오 수동 검증 (acceptance.md 의 Demo Flow 실행)

---

## 5. 기술적 접근

### 5.1 링버퍼 구현

- Go 내장 slice 기반 원형 버퍼 (index wrap-around)
- `sync.RWMutex` 로 읽기 다수 / 쓰기 단일 동시성 보호
- retention 만료는 별도 goroutine 이 `time.Ticker(10s)` 로 주기 스윕
- 만료 스윕은 버퍼 head 부터 확인하여 단조 타임스탬프 가정으로 O(만료 개수) 비용

### 5.2 WebSocket 관리

- `gorilla/websocket` (기존 xflow 에서 사용 중인 패키지 재사용)
- 채널당 `map[*websocket.Conn]struct{}` 로 구독자 집합 관리
- write 는 goroutine 당 1개 (write pump 패턴) — concurrent write 문제 회피
- 클라이언트 ping/pong 은 기존 WS 인프라 설정 준수

### 5.3 프론트엔드 WS 클라이언트

- 별도 싱글턴이 아닌 **채널당 1 연결** (동일 채널 패널 중복도 동일 WS 공유 가능)
- React 훅 (`useChartChannel`) 내부에서 useEffect 로 connect/disconnect
- backfill 은 state 초기화, append 는 state 에 push (max_points 초과 시 shift)
- `useSyncExternalStore` 고려 가능하지만 현 구현은 단순 useState 로 충분

### 5.4 Recharts 활용

- `stat` 는 Recharts 불필요 (div + CSS)
- `line-chart`: `<LineChart>` + `<XAxis dataKey="timestamp" type="number" domain={['auto','auto']}>` + `<Tooltip labelFormatter=...>`
- `bar-chart`: `<BarChart>` + 모드별 dataKey 분기
- `pie-chart`: `<PieChart>` + `<Pie data={...} dataKey="value" nameKey="label">`
- 기존 `ResourceWidget.tsx` 의 Area 차트 구현을 참조

### 5.5 fail-fast 준수 (SPEC-STORE-002 v1.2.0 패턴)

`chart-emitter` Init 시점 검증:
- `channel_name` 형식 (정규식 불일치 → 에러)
- 레지스트리 중복 등록 (이미 존재 → 에러)
- buffer_size / retention_sec 범위 (범위 초과 → 에러)

에러 메시지 형식: `"channel_name \"%s\" is already registered by flow %s node %s"` (SPEC-STORE-002 스타일)

---

## 6. 아키텍처 설계 방향

### 6.1 레이어 분리

```
┌────────────────────────────────────────────────────────────┐
│ Frontend (React)                                            │
│ ┌──────────────────┐  ┌───────────────────────────────┐   │
│ │ Chart Panels (5) │──│ useChartChannel (WS hook)     │   │
│ └──────────────────┘  └───────────────┬───────────────┘   │
│ ┌──────────────────┐                  │                    │
│ │ AddPanelDialog   │── HTTP ──→ /api/v1/charts/channels    │
│ └──────────────────┘                  │                    │
└───────────────────────────────────────┼────────────────────┘
                                         │ WebSocket
┌───────────────────────────────────────▼────────────────────┐
│ Backend (Go)                                                │
│ ┌──────────────────────────────┐   ┌──────────────────┐   │
│ │ /ws/chart/{channel} handler  │──▶│ ChartChannel     │   │
│ │ /api/v1/charts/channels      │──▶│  Registry        │   │
│ └──────────────────────────────┘   │  (singleton)     │   │
│                                     │ ┌──────────────┐ │   │
│ ┌──────────────────────────────┐   │ │ Ring Buffer  │ │   │
│ │ chart-emitter node           │──▶│ │ + Subscribers│ │   │
│ └──────────────────────────────┘   │ └──────────────┘ │   │
│             ▲                       └──────────────────┘   │
│             │                                               │
│ ┌───────────┴──────────┐                                   │
│ │ filter / aggregate   │                                   │
│ │ (기존 노드 재사용)   │                                   │
│ └──────────────────────┘                                   │
│             ▲                                               │
│             │                                               │
│ ┌───────────┴───────────────────────────────────────────┐ │
│ │ store-read / tsdb-query / influxdb-query (기존)       │ │
│ └───────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌────────────────────────────────────────────────────────┐│
│ │ (병렬) HTTP 쿼리 API (외부 도구용)                     ││
│ │  POST /api/v1/store/{agent}/query                       ││
│ │  POST /api/v1/influxdb/{agent}/query                    ││
│ │  POST /api/v1/tsdb/query (기존)                         ││
│ └────────────────────────────────────────────────────────┘│
└────────────────────────────────────────────────────────────┘
```

### 6.2 채널 레지스트리 배치

`ChartChannelRegistry` 는 `internal/agent/system` 에 두어 다른 시스템 에이전트와 동일한 수준에서 접근 가능하게 한다. 대안으로 `internal/node` 내부 패키지도 가능하지만, WS 핸들러가 `internal/api/ws` 에서 참조해야 하므로 보다 상위(system)에 두는 쪽이 의존 방향이 자연스럽다.

---

## 7. 위험 요소 및 대응 계획

| 위험 | 가능성 | 영향 | 대응 |
|------|--------|------|------|
| 채널 레지스트리 concurrent map 데드락 | 중 | 고 | `sync.RWMutex` 로 분리. 테스트에 `-race` 플래그 적용 |
| 링버퍼 retention 스윕 중 락 장기 점유 | 저 | 중 | 스윕 시점에 만료 인덱스만 먼저 계산 후 짧은 write lock 으로 절삭 |
| 프론트엔드 WS 재연결 폭주 | 중 | 중 | Exponential backoff (1→2→4→8→16s) + jitter, 채널 당 1 연결 싱글턴 |
| Recharts 의 대용량 데이터 렌더링 성능 | 중 | 중 | `max_points` 로 상한 제한 (line: 100, table: 200). 필요 시 sampling 노드 권장 |
| InfluxDB Flux 쿼리 타임아웃 | 중 | 중 | 기본 30s timeout, config 로 조정 가능. HTTP 408 반환 |
| 채널 이름 충돌 (REQ-M1-09) 로 인한 플로우 배포 실패 | 중 | 저 | 에러 메시지에 기존 등록자 flow_id/node_id 명시. 운영자 가이드 문서화 |
| DashboardPage 의 panel type union 확장 시 타입 에러 | 저 | 저 | `web/src/stores/uiStore.ts` 의 PanelConfig.type union 에 5종 추가 확인 (이미 있다면 skip) |
| 네트워크 불안정 환경에서 backfill 실패 | 중 | 중 | 패널 측 refresh 버튼 + 자동 재구독 시 backfill 재수신 |

---

## 8. 테스트 전략

### 8.1 백엔드 (TDD)

- **단위 테스트**:
  - ChartChannelRegistry: Register 중복, Get 미존재, List 결과, Broadcast fan-out
  - RingBuffer: FIFO 제거, retention 만료, thread safety (`go test -race`)
  - chart-emitter 노드: Init 검증 (fail-fast 케이스), Process timestamp 주입/정규화, Close 시 채널 해제
- **통합 테스트**:
  - WebSocket 핸들러: 실제 goroutine 에서 구독 → backfill 수신 → append 수신 → 연결 종료 시 레지스트리에서 제거 확인
  - chart-emitter → WebSocket 엔드투엔드: `httptest.NewServer` + gorilla websocket 클라이언트

### 8.2 프론트엔드

- **단위 테스트**: `useChartChannel` 훅 (MSW 로 WS 모킹), 각 차트 패널 렌더링
- **E2E 테스트 (수동)**: acceptance.md 의 Demo Flow 시나리오 수행

### 8.3 수동 검증 시나리오

acceptance.md 참조.

---

## 9. TRUST 5 품질 기준

- **Tested**: 신규 Go 파일 85%+ 커버리지, `go test -race ./...` 통과
- **Readable**: 각 컴포넌트 파일 200줄 이내, 명확한 네이밍 (`ChartEntry`, `ChartChannel`, `ChartChannelRegistry`)
- **Unified**: 기존 xflow 코드 스타일 준수 (gofmt, eslint), 기존 WS 메시지 컨벤션 (`type`, `channel`) 준수
- **Secured**: `channel_name` 정규식 검증, WebSocket 업그레이드 전 HTTP 400 처리 (path injection 방지)
- **Trackable**: 각 commit 에 `SPEC-CHART-001` TAG, 노드 시작/종료 시 INFO 로그 (flow_id/node_id/channel_name 포함)

---

## 10. 통합 시연 플로우 (M7 문서 제작 시)

```yaml
# Example 1: 최신 온도 스탯
nodes:
  - id: read-temp
    type: store-read
    config:
      agent_ref: room-store
      key: room1/temp
      mode: latest
  - id: emit-stat
    type: chart-emitter
    config:
      channel_name: room1_temp_stat
edges:
  - from: read-temp.out
    to: emit-stat.in
```

→ Dashboard 에서 Stat 패널 추가, channel_name="room1_temp_stat" 선택 → 최신 온도 표시.

---

## 11. 다음 단계

- [ ] 본 plan.md 승인 후 `/moai:2-run SPEC-CHART-001` 진입
- [ ] 구현 완료 후 `/moai:3-sync SPEC-CHART-001` 로 SPEC 메타 업데이트 + CHANGELOG 반영
