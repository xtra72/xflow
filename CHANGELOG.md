# Changelog

이 프로젝트의 주요 변경 사항을 기록한다.
형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르며,
[Semantic Versioning](https://semver.org/lang/ko/)을 적용한다.

## [Unreleased]

### 추가

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
