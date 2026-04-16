# Changelog

이 프로젝트의 주요 변경 사항을 기록한다.
형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르며,
[Semantic Versioning](https://semver.org/lang/ko/)을 적용한다.

## [Unreleased]

### 추가

- **차트 패널 플로우 연동 시스템 구현** (SPEC-CHART-001)
  - **`chart-emitter` 종단 노드** (`internal/node/chart_emitter.go`): 입력 메시지를 WebSocket 차트 채널로 발행하고 링버퍼(FIFO + retention 스윕)에 보관. config: `channel_name` (정규식 검증), `buffer_size` (1-10000), `retention_sec` (0-86400). 채널 이름 중복 시 fail-fast Init 에러.
  - **`ChartChannelRegistry` 싱글톤** (`internal/agent/system/chart_channel_registry.go`): 프로세스 전역 채널 레지스트리. ChartSubscriber 인터페이스, EncodeChart{Backfill,Append,Closed,Error} 프레임 헬퍼. race-clean (sync.RWMutex).
  - **`GET /ws/chart/{channel}` WebSocket 엔드포인트** (`internal/api/ws/chart_channel.go`): channel_name 정규식 검증 (HTTP 400), 미존재 채널 `chart.error`, backfill + append fan-out, slow-consumer 보호 (256 항목 버퍼 + 초과 시 close), 연결 종료 시 자동 unsubscribe.
  - **HTTP 쿼리 API** (외부 도구 / 디버깅용):
    - `GET /api/v1/charts/channels` — 활성 chart-emitter 채널 목록
    - `POST /api/v1/store/{agent}/query` — Store 5-모드 HistoryQuery (latest/last_n/duration/time_range/since_n)
    - `POST /api/v1/influxdb/{agent}/query` — Flux / InfluxQL 쿼리 (`InfluxDBAgent.ExecuteFluxQuery`/`ExecuteInfluxQLQuery` 신규 메서드)
    - 표준 응답 스키마: `{entries: [{timestamp, value, labels}], count, truncated}`
  - **5종 차트 패널** (`web/src/pages/dashboard/panels/charts/`): Stat (delta + 임계값 색상), Line Chart (multi-series 지원), Bar Chart (category / time_bin 모드), Pie Chart (집계), Table (정렬 + 페이지네이션). Recharts 3.7 기반 + HTML table.
  - **`useChartChannel` React 훅 + `ChartChannelClient`** (`web/src/services/ws/chartChannel.ts`): exponential backoff 재연결 (1→16s), chart.closed 수신 시 영구 종료, maxPoints 슬라이딩 윈도, factory 주입으로 테스트 가능.
  - **대시보드 UI 확장**:
    - AddPanelDialog: 차트 타입 선택 시 채널 드롭다운 (`GET /api/v1/charts/channels` 연동) + 수동 입력 + 정규식 인라인 검증
    - PanelSettingsDialog: 5종 차트 타입별 config 편집 섹션
    - `panelDefaultSize` SPEC 값 적용 (stat 2×1, line-chart 6×3, bar-chart 4×3, pie-chart 3×3, table 6×4)
    - uiStore v3→v4 persist 마이그레이션 (기존 dataSource/period config 보존)
  - **플로우 캔버스 통합**: `web/src/config/nodeSchemas.ts` + `web/src/pages/nodes/nodeTypeMeta.ts` 에 chart-emitter 등록. NodePalette 이 `category=output` 그룹에 자동 배치, PropertyPanel 이 DynamicForm 으로 설정 편집 UI 자동 생성.
  - **활용 가이드 문서** (`docs/guides/chart-panel-flow.md`): 아키텍처 다이어그램, payload 정규화 규칙, 3종 예시 플로우 YAML, 기존 노드 조합 패턴, 운영 주의사항, HTTP 쿼리 API curl 예시, 문제 해결 표.
  - **핵심 설계 원칙**:
    - 차트 패널은 데이터 소스를 몰라야 한다 — 오직 `channel_name` 만 안다
    - 필터링/집계/정렬은 플로우 노드(`filter`, `aggregate`, `mapping`)가 담당
    - 모든 타임스탬프는 epoch ms (int64) 로 통일
    - 채널 = 하나의 chart-emitter 인스턴스 (중복 이름 fail-fast)
  - **테스트**: Go 신규 파일 평균 93% 커버리지 + `go test -race` 통과 / Vitest 132 테스트 평균 88% 커버리지. TRUST 5 게이트 전부 통과.

- **TCP 소스 노드에 `connection_id` 메타데이터 주입** (SPEC-NODE-003)
  - `TCPInNode.receiveLoop`에서 매 메시지에 `connection_id` 메타데이터를 설정하여, framer 노드와 결합 시 TCP 서버의 다중 클라이언트 연결별 독립 프레이밍을 지원.
  - TCP 서버 모드(`ConnAwareReceiver`): `connection_id` = `remoteAddr` (host:port). `tcp.remote_addr`과 동일한 값으로 설정되며, 기존 `tcp.remote_addr` 메타데이터도 그대로 유지 (하위 호환).
  - TCP 클라이언트 모드(`MessageReceiver`): `connection_id` = `n.ID()` (노드 ID). 단일 연결이므로 고정 식별자로 일관된 메타데이터 구조 제공.
  - framer 노드의 기본 `stream_key_metadata="connection_id"`와 자동 연동되어, 추가 설정 없이 "tcp-in (서버) → framer" 파이프라인에서 연결별 프레이밍 동작.

- **`pkg/framing` 공개 패키지 신설** (SPEC-NODE-002)
  - 시리얼 에이전트 내부(`internal/agent/serial/framing.go`)에 있던 프레이밍 엔진을 `pkg/framing` 공개 패키지로 승격하여 범용 재사용이 가능하도록 함.
  - 6가지 프레이밍 모드 지원: `raw`, `newline`, `length_prefix`, `fixed_size`, `stream`, `frame`.
  - `Framer` 인터페이스, `Options` 구조체, `New` 팩토리 함수, `Drain` API, 모드 상수 (`ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame`) 노출.
  - `ScannerConfigurer` 인터페이스를 통해 기존 `SerialConnReader`와의 호환성 유지.
  - sentinel 에러 (`ErrETXMismatch`, `ErrChecksumMismatch`, `ErrMaxSizeExceeded`, `ErrLengthInvalid`) 노출로 에러 분류 지원.
  - 시리얼 에이전트(`internal/agent/serial`)는 `pkg/framing`을 import하여 기존과 동일한 동작을 유지.

- **`framer` 처리 노드 추가** (SPEC-NODE-002)
  - 임의의 바이트 스트림 소스 노드(serial-in, tcp-in, udp-in 등)의 출력을 받아 프로토콜 프레임으로 분리하는 `framer` 처리 노드를 `internal/node/`에 추가.
  - 입력 포트 `in`, 출력 포트 `out`, 에러 포트 `error`의 3포트 구조.
  - 메타데이터 기반 다중 스트림 버퍼 관리 (`connection_id` 등 `stream_key_metadata` 키로 스트림 분리). 키가 없으면 단일 공용 버퍼로 동작.
  - `max_streams`, `stream_idle_timeout` 옵션을 통한 자원 제한 및 유휴 스트림 lazy 축출.
  - 에러 포트를 통한 파싱 에러 분리 (에러 코드: `frame.input.invalid_payload`, `frame.parse.etx_mismatch`, `frame.parse.checksum_mismatch`, `frame.parse.max_size_exceeded`, `frame.buffer.max_streams_exceeded`, `frame.buffer.incomplete_on_stop`).
  - 출력 메시지는 소스 노드 규약(`raw` + `data` 키)을 그대로 따르며, 업스트림 메타데이터 보존 및 `frame.index`, `frame.framer_type`, `frame.stream_key` 메타데이터 추가.
  - 시리얼 에이전트 `framing=frame` 경로와의 완전 동등성을 Parity 테스트(`framer_parity_test.go`)로 검증.
  - Registry 팩토리(`framer_factory.go`)를 통한 빌트인 노드 등록 및 옵션 파싱.

- **에이전트 활성화/비활성화 기능** (SPEC-AGENT-005)
  - `internal/agent/config.go`: `AgentConfig.Enabled *bool` 필드와 `(*AgentConfig).IsEnabled() bool` 메서드 추가. `pkg/flow/node.go`의 `NodeDef.Enabled` 패턴을 재사용하며, `nil` 은 기본값 `true` 로 해석되어 하위 호환성을 보장한다.
  - `internal/agent/serialize.go`: JSON/YAML 직렬화에 `enabled` 필드 추가 (`omitempty`). 기존 저장 데이터는 마이그레이션 없이 그대로 사용 가능하다.
  - `cmd/xflowd/main.go`: 저장소 복원 로직을 테스트 가능한 `restoreAgents` 헬퍼로 추출하고, disabled 상태의 에이전트는 매니저에 등록(Create)만 하고 자동 시작(Start)을 건너뛰도록 변경. 건너뛴 경우 INFO 레벨로 "자동 시작 건너뜀 (disabled)" 로그를 남긴다.
  - `internal/api/handler/agent.go`: `POST /agents/{id}/enable`, `POST /agents/{id}/disable` 엔드포인트 추가. `AgentInfo` 응답에 `enabled: bool` 필드를 항상 포함한다.
  - `internal/api/service/agent_adapter.go`: `EnableAgent`/`DisableAgent` 서비스 메서드와 영속화 실패 시 in-memory 롤백을 수행하는 `setAgentEnabled` 공통 헬퍼 구현. Disable 은 런타임 Stop 을 호출하지 않으며, Enable 은 정지된 에이전트를 자동 Start 하지 않는다 (R3.7, R3.8).
  - `internal/engine/errors.go`: `ErrAgentDisabled` sentinel error 추가.
  - `internal/engine/agent_validation.go`: `validateAgentRefs` 가 플로우 노드가 참조하는 에이전트의 활성화 상태를 검증한다. 비활성화된 에이전트를 참조하는 플로우는 `DeployFlow` 시점에 거부되며, 다수의 검증 실패는 `errors.Join` 으로 한 번에 보고된다. 에러 메시지에는 에이전트 ID, 노드 이름, Enable API 안내가 포함된다.
  - `web/src/types/agent.ts`: `AgentInfo.enabled?: boolean` 필드 추가.
  - `web/src/services/api/agentService.ts`: `enableAgent(id)`, `disableAgent(id)` API 클라이언트 함수 추가.
  - `web/src/hooks/useAgent.ts`: `useEnableAgent`, `useDisableAgent` React Query mutation hook 추가. 성공 시 `['agents']` 및 상세 쿼리 캐시를 무효화한다.
  - `web/src/pages/agents/AgentEnabledBadge.tsx`: 비활성화된 에이전트를 시각적으로 구분하는 회색 배지 컴포넌트 추가. `enabled !== false` 인 경우 렌더링하지 않아 기존 UI 에 노이즈를 주지 않는다.
  - `web/src/pages/agents/AgentActionButtons.tsx`: Enable/Disable 토글 버튼(Power/PowerOff 아이콘) 추가. 비활성화된 에이전트의 수동 Start 시 사용자에게 일시 시작임을 안내하는 확인 다이얼로그 표시.
  - `web/src/pages/agents/AgentListPage.tsx`: 에이전트 이름 옆에 `AgentEnabledBadge` 표시.

- **통합 디바이스 관리 시스템** (SPEC-DEVICE-001)
  - `internal/device/`: 프로토콜 무관 통합 Device/ControllableDevice 인터페이스, DeviceState, CommandSpec, DeviceProvider, DeviceFilter, DeviceMetadata 모델
  - `internal/device/registry.go`: 중앙 DeviceRegistry - RegisterProvider/UnregisterProvider, 필터링, 동시성 안전 설계
  - `internal/device/adapter/nasa.go`: NASADevice를 통합 Device 인터페이스로 래핑하는 어댑터 (ControllableDevice 지원)
  - `internal/agent/samsung/provider.go`: NASAAgent에 DeviceProvider 인터페이스 구현
  - `internal/api/handler/device.go`: 디바이스 REST API 6개 엔드포인트 (목록/상세/실행/메타데이터/명령/상태)
  - `internal/agent/manager.go`: 에이전트 시작/중지 시 DeviceProvider 자동 등록/해제 라이프사이클 훅
  - `internal/api/ws/event_publisher.go`: WebSocket 기반 실시간 디바이스 상태 변경 알림
  - `web/src/pages/devices/DeviceListPage.tsx`: react-grid-layout 기반 디바이스 목록 대시보드 (카드, 필터, 검색, 추가)
  - `web/src/pages/devices/DeviceDetailPanel.tsx`: 디바이스 상세 패널 (상태/속성, CommandSpec 동적 제어 UI, 리모컨)
  - `web/src/pages/agents/AgentDetailPanel.tsx`: 에이전트별 디바이스 CRUD 탭 (추가/제거, 소스 배지)
  - `web/src/hooks/useWebSocket.ts`: WebSocket 연동 실시간 상태 업데이트

### 변경

- `internal/agent/samsung/config.go`: NASA 에이전트 `device_addresses` 설정을 선택 사항으로 변경 (기존: 필수)
- `internal/agent/samsung/agent.go`: processAddDevice/processRemoveDevice에서 req.Params 폴백 읽기 추가
- `web/src/config/agentSchemas.ts`: samsung-nasa 에이전트 스키마에서 device_addresses 필드 제거
- `web/src/config/nodeSchemas.ts`: framer 노드 스키마 추가 (framing_mode, stx, etx, checksum, length_size, max_frame_size, fixed_size, stream_key_metadata, max_streams, stream_idle_timeout 설정)

### 수정

- **ETX 필드 선택 사항 처리** (`pkg/framing`): `frame` 모드에서 ETX가 빈 값일 때 ETX 검증을 건너뛰도록 수정. ETX 없는 프레임 프로토콜 지원.
- **LengthSize 기본값 보정** (`pkg/framing`): `length_prefix` 모드에서 `length_size` 미지정 시 기본값 2를 적용하도록 수정. 이전에는 0으로 해석되어 프레이밍이 실패함.
- **configInt 문자열 처리** (`internal/node/framer_factory.go`): YAML/JSON에서 정수 설정이 문자열로 전달되는 경우를 처리. `strconv.Atoi` 폴백으로 `"2"` → `2` 변환 지원.
