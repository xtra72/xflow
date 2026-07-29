---
id: SPEC-SAMSUNG-HVACR-001
version: "1.21.0"
status: active
created: "2026-02-24"
updated: "2026-07-13"
author: xtra
priority: P2
---

## HISTORY


| 날짜         | 버전    | 변경 내용                                                                                                                                                                                                                                                                       |
| ---------- | ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-13 | 1.21.0 | **device_state 가시성 개선 3종 (fix + additive)**. (1) **power 관측 기반 emit (fix)** — `StateForJSON` 이 `power` 를 관측 여부와 무관하게 항상 emit 하던 것을 `observed(observedPower)` 게이트로 전환. 재시작 직후 power 프레임(`0x4000`) 미수신 상태에서 기본값 `false` 가 방출되어, 실제 on 인 디바이스가 대시보드에서 default off 로 덮어써져 "꺼졌다 켜진" 것처럼 보이던 회귀 수정. 미관측 시 payload 에서 생략해 마지막 확인값 보존(mode/온도/fan 과 동일 규칙). (2) **transport_connected 조건부 emit (schema)** — device_state `state` 그룹의 `transport_connected` 를 `online=true` 이면 생략하고 `online=false` 일 때만 실는다. online 이면 트랜스포트가 항상 열려 있어(=true) 정보량이 없으므로, 필드 존재 자체가 "버스(포트/TCP) 문제인지 디바이스 침묵인지" 판별 신호가 되게 한다(관측/의미 기반 emit). `error_count`/`offline_threshold` 는 상시 유지. 에이전트 전역 `get_stats`/`State()` 의 `transport_connected`(버스 상태 상시 진단)는 불변. LGAP(SPEC-LGAP-001 v1.18.14)도 동일 규칙. 다운스트림: device_state 의 `transport_connected` 상시 존재를 가정한 소비자는 online 디바이스에서 값 부재(=연결됨)를 처리해야 한다. (3) **실외기 텔레메트리 device provider 노출 (additive)** — `hvacr01DeviceToInfo` 가 `dev.Outdoor.Fields`(관측된 `out_*`/`outdoor_temperature` 등)를 `ExtraProperties` 로 노출 → 어댑터 `State()` 가 device REST `Properties` 에 평탄화 → UI 디바이스 상세에 실외기 상태 속성이 표시된다. 이전엔 실내기(`dev.State`)만 매핑돼 ODU 상세가 비어 있었다(REQ-NASA-001-04-13 후속). 관련: `internal/agent/samsung/device.go`, `agent.go`, `provider.go`, `internal/device/adapter/samsung_nasa.go`. |
| 2026-07-13 | 1.20.0 | **실외기(ODU) 상태 디코딩 + 폴링 요청 간격 (additive)**. (1) **폴링 요청 간격 신규 (additive)** — `inter_command_delay` (Duration, default `"1s"`). `pollLoop` 에서 여러 디바이스에 status query 를 연속 전송할 때 프레임 간 최소 간격을 강제해 요청이 μs 단위로 겹치는 것을 방지 (LG `inter_command_delay` 와 명칭 통일, 단 Samsung default 는 `"1s"`). `probeSilentDevices` 에도 동일 적용. 대기는 `stopCh`/`disconnectCh` 로 인터럽트 가능. `0s` = 딜레이 없음(기존 동작). UI(`agentSchemas.ts` protocol 섹션) 노출. (2) **실외기 상태 텔레메트리 디코딩 신규 (additive)** — 실외기는 자기 상태를 C0 14 Notification 으로 버스에 브로드캐스트하므로 SA=`10 xx 00` 프레임의 `0x80xx`(ENUM 1B)/`0x82xx`(VAR 2B)/`0x84xx`·`0x24FC`(LVAR 4B) 대역 Message Set 을 수동 디코딩한다. `NasaDevice.Outdoor *OutdoorState` 추가 (실내기 `State` 와 병렬; ODU 는 `State=nil`). 온도는 signed `/10 = °C`, ENUM/VAR/LVAR 은 raw 정수. 관측 기반 emit (수신된 필드만 노출). 출처: esphome_samsung_hvac_bus / pysamsungnasa 리버스 엔지니어링 — 모델별 존재·스케일 상이 가능(전기값 단위 미확정, 실측 검증 필요). (3) **emit 정책** — 이산 필드(운전상태·압축기 On/Off·4way·제상단계·에러코드) 변경 시에만 즉시 `device_state_changed` emit, 연속 센서(온도·압축기 주파수·전류·전력)는 정기 보고(`report_interval`) 주기에만 실어 emit 폭주 방지. (4) **센티넬 처리** — 미장착 압축기/센서가 값 폭 전체를 `0xFF` 로 보고(예: 단일 압축기 유닛 comp2 주파수 `0xFFFF`)하는 경우 raw 정수 필드(ENUM `0xFF`/VAR `0xFFFF`/LVAR `0xFFFFFFFF`)를 무효로 간주해 payload 에서 생략(관측 안 됨과 동일, raw 는 보존). 온도(signed)는 `0xFFFF`(-0.1°C)가 정상 저온일 수 있어 마스킹 제외. (5) **LG 필드명 통일** — 의미가 1:1 일치하는 실외기 필드는 LG HVACR emit 키와 통일: `0x8204 → outdoor_temperature`, `0x8280 → compressor_discharge_temperature`, `0x8235 → error_code`. 그 외(파이프 온도·다중 압축기·전류/전압/전력 등)는 LG 대응 필드 부재로 Samsung 고유명 유지. 관련: `internal/agent/samsung/outdoor.go`, `references/protocols/samsung_nasa_protocol.md`. |
| 2026-05-29 | 1.19.0 | **BREAKING — 3종 HVACR-01 에이전트 config 통일 (LG 기준)**. (1) **기본값 변경 (Breaking)** — `report_interval`: `"0s"` → `"60s"` (변경 감지만 → 60s keepalive 활성. 변경 감지만 원하면 명시적으로 `"0s"` 설정). `auto_discovery`: `false` → `true` (LG / Century 와 정렬. 자동 탐색 비활성 원하면 명시적으로 `false`). (2) **필드 rename (Breaking, alias 미수용)** — `include_raw_message_sets` → `include_raw_hex` (LG / Century 의 동명 필드와 정렬, 의미 동일 — register-decoded / state response 의 원시 바이트 hex 포함 여부). (3) **deprecated alias 완전 제거 (Breaking)** — `notify_interval`: v1.9.0a 의 deprecation alias 가 backend 에서 더 이상 silent accept 되지 않고 명시적 parse error 로 거부. `include_raw_message_sets`: 본 버전에서 rename, 이전 키는 parse error. (4) **로그 옵션 신규 노출 (additive)** — `log_drops`, `log_state_updates` (boolean, default false). 의미는 LG / Century 와 동일 — msgCh / ring buffer drop per-message WARN 로그 + device state 변경/report emit DEBUG 로그. `log_decode_errors` 는 v1.9.0 부터 보유 (변경 없음). (5) **REVISED REQ 항목** — `NotifyInterval` (REQ-NASA-001-04-XX): NotifyInterval 필드명·default 변경 (`notify_interval "0s"` → `report_interval "60s"`). `AutoDiscovery` (REQ-NASA-001-04-XX): default 변경 (`false` → `true`). `IncludeRawMessageSets` (REQ-NASA-001-04-XX, Scenario 11.5 / 14.3): rename → `IncludeRawHex` / `include_raw_hex`. (6) **본 변경의 회귀 위험 평가** — 기존 yaml 이 deprecated alias 를 사용했다면 부팅 실패. 운영자 마이그레이션 가이드 `docs/migration/hvacr-config-unification.md` 제공. 관련: SPEC-LG-HVACR-001 v1.18.27, SPEC-CENTURY-HVACR-001 v0.5.0, CHANGELOG.md [Unreleased]. |
| 2026-05-28 | 1.18.26 | **BREAKING — status 노드 통일 (LG inactivity 모델) + 어드레싱 + metadata 정리**. (1) **노드 동작 모델 변경** — Samsung HVACR-01 status / combined 노드가 ticker 기반 폴링 (`get_recent_states` / `get_all_states`) 에서 LG ICP-01 의 inactivity-fallback 모델로 전환. 노드는 에이전트의 `FrameNotifyCh` 신호로 새 frame 도착 시 즉시 처리하고, `inactivity_timeout` (기본 `"90s"`) 동안 신호가 없으면 `request_state` 명령으로 강제 상태 확보. agent 측에 `processRequestState` 추가 — 모든 디바이스의 `trigger="response"` emit. (2) **노드 config 변경** — 제거: `device_id`, `device_address`, `poll_interval`, `poll_command`. 추가: `inactivity_timeout`, `group_id` (NASA addr byte 1, 외기 인덱스 `"00"`–`"0F"`), `unit_id` (NASA addr byte 2 또는 dotted/compact 형식 `"10.0F.00"` / `"100F00"` 모두 인식). 두 어드레싱 필드 모두 비어있으면 모든 디바이스 frame 처리 + broadcast `request_state`. (3) **출력 metadata 정리 (Breaking)** — `MetadataEmitOptions.UnitID` / `SlotNum` 필드 제거 + 노드 config 의 `emit_unit_id` / `emit_slot_num` 옵션 제거. `unit_id` / `slot_num` 은 프로토콜 해석 단계에서만 의미가 있던 내부 표현으로, `metadata.device_id` (UUID) 와 노드 어드레싱 필드로 대체된다. (4) **agent 측 변경** — `applySamsungHvacr01Overrides` 시그니처 변경 (`(cfg, deviceID)` 반환). 제어 명령 시 payload override (`device_id` / `unit_id`) 가 우선, 다음 `cfg.UnitID`. `pollLoop` / `pollRecentBulk` / `pollSnapshot` / `samsungHvacr01StateHash` / `splitSamsungHvacr01PollResult` / 콘텐츠 dedup 모두 제거. `receiveLoop` + `drainNewFrames` + `requestStateRefresh` (LG 패턴) 로 교체. (5) **하위 영향** — control 노드는 payload-level `device_id` / `unit_id` override 그대로 수용 (단일 디바이스 제어 가능). 본 SPEC 의 REQ-NASA-001-09-XX (M9, NASA Nodes) 중 polling 동작 관련 항목 (`PollInterval`, `get_state` / `get_all_states` 폴링 분기 등) 은 REVISED — inactivity 모델로 대체된 동작 기술. Samsung agent 자체의 `poll_interval` (디바이스 polling 주기) 은 보존된다 (agent-level 설정으로 NASA 디바이스 양방향 폴링은 유지). |
| 2026-05-26 | 1.18.14 | **`mode`/`fan_speed` 통일 ID 정합 — adapter `Device.State()` (v1.18.13 후속)**. v1.18.13 이 agent direct emit (`processGetAllStates`) 경로만 수정했으나, `internal/device/adapter/nasa.go` 의 `NASADeviceAdapter.State()` (REST/inventory 경로) 가 `*a.info.Mode` / `*a.info.FanSpeed` 를 raw string ("cool"/"low") 그대로 emit 하여 inventory 출력에서 NASA HVACR.IDU 만 schema 불일치 (다른 HVAC 디바이스: int). 수정: `hvac.ModeFromName` / `hvac.FanSpeedFromName` 로 wrap. 영향: inventory/REST 응답이 다른 HVAC 에이전트 (LGCP/LGCNP/Century) 와 schema 정합. NASA 의 모든 emit 경로 (agent direct + adapter) 가 hvac 통일 ID 로 일관. |
| 2026-05-26 | 1.18.13 | **`mode`/`fan_speed` 통일 ID 정합 — agent direct emit (v0.7.5 후속)**. v0.7.5 가 NASA adapter (samsung/device.go) 의 `Mode: hvac.ModeFromName(s.Mode)` 변환은 적용했으나, agent direct emit 경로 (samsung/agent.go:1954 `processGetAllStates`) 의 `dev.State.Mode`/`dev.State.FanSpeed` 가 raw string 그대로 emit 되어 inventory/REST 와 schema 분기. Century 의 동일 결함 발견 시점에 NASA 도 함께 발견. 수정: `hvac.ModeFromName(dev.State.Mode)` + `hvac.FanSpeedFromName(dev.State.FanSpeed)` 로 wrap. 영향: 브릿지 명령 `get_all_states` 응답이 inventory/REST 와 동일 int 통일 schema. |
| 2026-05-24 | 1.18.12 | **BREAKING — node_id / unit_id 옵션화**. `MetadataEmitOptions` 에 `UnitID` / `NodeID` 필드 추가, default OFF. 이전엔 unit_id 가 필수 + node_id 가 자동 emit 이었으나 v0.18.12 부터 명시 토글 필요. `promoteDevIDToMetadata` 시그니처에 `emitUnitID bool` 추가, `splitNASAPollResult` 도 갱신된 opts 사용. Web UI nodeSchemas 에 `emit_unit_id` / `emit_node_id` boolean 노출. 노드 id 자체도 Web UI 에서 `crypto.randomUUID()` 로 생성 (이전: `${type}-${Date.now()}`). 다운스트림 마이그레이션: `metadata.node_id` / `metadata.unit_id` 가 자동 emit 되지 않으므로 옵션 명시적 활성화 필요. |
| 2026-05-24 | 1.18.8 | **메타데이터 emit 옵션 (`emit_metadata`) 도입**. 기존 자동 emit 되던 `device_type` / `label` / `node_source` / `slot_num` 가 default OFF 로 변경 (breaking). `device_id` / `unit_id` 는 항상 emit (필수). `nasa-status` / `nasa-control` / `nasa` 노드 config 에 `emit_metadata` (또는 평탄 `emit_*` 키) 추가. Web UI nodeSchemas 에 4개 boolean 필드 노출 (advanced 섹션). `splitNASAPollResult` 시그니처에 `opts MetadataEmitOptions` 추가. |
| 2026-05-24 | 1.18.7 | **register-decoded 경로 UUID `device_id` 자동 주입 + 노드/에이전트 storage 키 통일 (AgentID)**. (1) `promoteDevIDWithUUID` 헬퍼 신설 — `payload.unit_id` 로 글로벌 UUID 를 조회해 metadata 에 주입. poll_bulk / register-decoded emit 경로도 device_state 경로와 동일한 metadata 시그니처 노출. (2) `DeviceInfoRepository` (runtime-only) 신설 — agent 가 device 등록 시 `(agentName, unitID) → {device_type, label}` 을 publish, 노드의 promote 가 조회해 metadata 주입. (3) Samsung NASA agent 의 `ResolveDeviceID` 호출 키를 `a.Name()` → `a.ID()` 로 통일 — 노드의 `cfg.AgentRef` (AgentID UUID) 와 동일 키 공유로 단일 device 가 단일 UUID 발급. |
| 2026-05-23 | 1.18.3 | **BREAKING — `device_type` 값 카테고리 prefix 도입**. `"indoor"` → `"HVACR.IDU"`, `"outdoor"` → `"HVACR.ODU"`. Samsung NASA agent 의 `DetectDeviceType`, `DiscoveryResult.DeviceType`, `NASADeviceInfo.Type` 일괄 변경. `provider.go` 의 `dev.Type == "indoor"` 비교도 변경. |
| 2026-05-22 | 1.18.0 | **status 노드 OFF 상태 필드 제거 옵션**. `nasa-status` / `nasa-control` / `nasa` 노드에 `omit_state_when_off` (boolean, default false) 옵션 추가. 활성화하고 `payload.power == false` 이면 신뢰할 수 없는 상태 필드 (`current_temperature`, `mode`, `fan_speed`) 를 emit/response 메시지에서 제거. `target_temperature`, `online`, 식별자 등 OFF 에서도 의미있는 필드는 보존. `splitNASAPollResult` 시그니처 갱신 (omitStateWhenOff 매개변수 추가). |
| 2026-02-24 | 0.1.0 | 초기 SPEC 작성 (SPEC-SAGENT-001 Module 8에서 분리)                                                                                                                                                                                                                                  |
| 2026-02-24 | 0.2.0 | 실제 Samsung NASA 프로토콜 사양 반영 (3바이트 주소, CRC16-CCITT, Message Set 구조, 실외기 관리 프로토콜)                                                                                                                                                                                              |
| 2026-02-24 | 0.3.0 | 사용자 승인 SPEC: 프로토콜 정의 엔진 의존성 제거, 자체 인코더/디코더 사용, 시리얼 팩토리 함수 기반 테스트 가능 설계                                                                                                                                                                                                      |
| 2026-02-24 | 1.0.0 | 구현 완료: 21개 파일, 7,352줄, 87.4% 커버리지                                                                                                                                                                                                                                           |
| 2026-03-12 | 1.1.0 | v1.0.0 이후 구현된 기능 문서화: NASAConfig 신규 필드 3종 (UnsupportedMsgSets, LogUnsupportedMsgSets, IncludeRawMessageSets), HexKeyByteMap 커스텀 타입, StateForJSON 메서드, Message Set 필터링 로직, CommandPollAdapter 인터페이스, NASAAdapter 브릿지 어댑터, startCommandPollLoop, 예제 YAML 파일 3종, State() 출력 개선 |
| 2026-03-12 | 1.2.0 | Transport 재연결 로직 요구사항 추가 (REQ-NASA-001-01-09~12, REQ-NASA-001-02-05, REQ-NASA-001-07-03)                                                                                                                                                                                    |
| 2026-03-12 | 1.3.0 | NASA 프로토콜 전용 노드 타입 추가: nasa-status (상태 조회), nasa-control (제어 명령), nasa (복합) — Module 9 (REQ-NASA-001-09-01~23)                                                                                                                                                              |
| 2026-03-13 | 1.4.0 | BuzzerOnControl 설정 추가, RS-485 프리앰블(0x55 x 100) 전송, 부저 자동 제어(sendControlCommand), 프론트엔드 buzzer_on_control UI. v1.2.0/v1.3.0 구현 완료 반영 |
| 2026-03-16 | 1.5.0 | 비-bridge 노드 AgentRef 해석 수정 (ID+Name 이중 검색), DynamicForm agent_select 매핑 수정, 플로우 액션 메뉴 버그 수정 |
| 2026-03-17 | 1.6.0 | Transport ENXIO 에러 처리 추가: 시리얼 디바이스 분리 시 자동 재연결 (isConnectionError에 syscall.ENXIO 추가) |
| 2026-03-27 | 1.7.0 | Device Configuration 통합 구조체 리팩터링 (`Devices []agent.DeviceEntry`), 주소 형식 표준화 (컴팩트 헥스), TransportChecker 인터페이스, NASADeviceAdapter 프로토콜 추상화 (Protocol/ExtraProperties/DeviceSource 필드) |
| 2026-03-27 | 1.8.0 | splitNASAPollResult 멀티 메시지 지원, NASAAgent Start() Stopped 상태 복구 로직, LGAP 에이전트 타입 추가 (internal/agent/lg/) |
| 2026-05-22 | 1.17.0 | **BREAKING — 메시지 필드명 정리**. `dev_id` → `device_id`, `dev_type` → `device_type`, `current_temp` → `current_temperature`, `inlet_temp` → `inlet_temperature`, `outlet_temp` → `outlet_temperature`, `comp_discharge_temp` → `compressor_discharge_temperature`, `comp_suction_temp` → `compressor_suction_temperature`, `condenser_temp_a` → `condenser_temperature_a`, `condenser_temp_b` → `condenser_temperature_b`. NASA agent device.go / agent.go / device adapter 의 JSON tag 일괄 변경. 다운스트림 마이그레이션 필요. |
| 2026-05-21 | 1.16.0 | **BREAKING — payload.state wrapper 평탄화**. msg.Type="device_state.X" 가 schema 명시이므로 state wrapper 는 중복. flattenStateToPayload 헬퍼로 state 의 키들을 payload 루트로 hoist. 다운스트림: `$.payload.state.<field>` → `$.payload.<field>`. |
| 2026-05-21 | 1.15.0 | **BREAKING — Message schema 정리**: `metadata.message_type` → `msg.Type()`, `payload.dev_id` → `metadata.dev_id`, `payload.last_seen_ms` → `msg.Timestamp()`. pkg/message 에 Type/SetType/SetTimestamp 추가. 5 HVAC 노드 + 비-HVAC 노드 모든 emit 사이트 적용. debug 노드 출력에 type 포함. 다운스트림: `$.metadata.message_type` → `$.type`, `$.payload.dev_id` → `$.metadata.dev_id`, `$.payload.last_seen_ms` → `$.timestamp`. |
| 2026-05-21 | 1.14.0 | **BREAKING — protocol-prefixed metadata 키 제거**. `nasa_node_id` → `node_id`, `nasa_source` → `node_source`, `nasa_seq` → `seq`. 모든 노드 (HVAC + mqtt + modbus) 통일 prefix-less 표준. protocol 식별은 node_id 값과 message_type 으로 가능. 다운스트림: `$.metadata.nasa_*` 참조를 통일 키로 마이그레이션. |
| 2026-05-21 | 1.13.0 | **BREAKING — `nasa_source="request"` 제거**. message_type="device_state.response" 와 중복이므로 정리. Process 응답에서 nasa_source 라인 삭제. nasa_source="poll" / "poll_bulk" 는 노드 경로 식별자로 유지. 다운스트림: `nasa_source == "request"` → `message_type == "device_state.response"`. |
| 2026-05-21 | 1.12.0 | **BREAKING — payload.type 제거**. v0.8.0 에서 message_type 이 `device_state.<trigger>` 계층형이 되면서 payload.type="device_state" 가 prefix 의 중복이 됨. Samsung NASA emit map literal 에서 "type" 키 제거. 다운스트림: `$.payload.type` 검사는 `$.metadata.message_type` 의 prefix 검사로 변경. |
| 2026-05-21 | 1.11.0 | **BREAKING — metadata.message_type 계층형 분류 + payload.trigger 제거**. 직교 분류 (`trigger` + `message_type="event\|response"`) 가 종속 관계라는 사용자 지적에 따라 단일 진실원천 통합. `applyDeviceStateMessageType(msg, payload, defaultSubType)` 헬퍼로 payload.trigger → `metadata.message_type="device_state.<trigger>"` 변환 + payload 에서 trigger 제거. 값 체계: `device_state.change` / `.report` / `.init` / `.poll` (자발 emit) + `device_state.response` (Process 응답). NASA 노드의 pollRecentBulk / splitNASAPollResult / Process 모든 emit 사이트 적용. 다운스트림 필터 변경 필요. |
| 2026-05-21 | 1.10.14 | **HVAC status payload 의 nested metadata 를 message metadata 로 promote**. `promotePayloadMetadata` 헬퍼 신설. NASA poll_bulk / splitNASAPollResult 의 모든 emit 사이트 적용. |
| 2026-05-21 | 1.10.8 | **5 HVAC 통합 v0.7.x**. (1) v0.7.0: 출력 schema `type:"device_state"` 단일화 + emit 패턴 통일 (change/report). (2) v0.7.1: 폴링 명령 명칭 통일 — `get_recent_states` → `get_recent`, `get_all_states` → `get_all` (deprecation alias 유지). (3) v0.7.3: `processGetStats` 추가 (Century/LGCNP/LGCP 패턴 차용). (4) v0.7.5: StateForJSON 의 Mode/FanSpeed 출력을 string → hvac 통일 ID (int) 로 변환 (`internal/agent/hvac/codes.go`). Mode 0=off/auto/1=cool/2=heat/3=dry/4=fan, FanSpeed 0=off/1=auto/2=quiet/3=low/4=medium/5=high/6=turbo. Power=false 시 0 강제. (5) v0.7.6: Manager.Restart lock holding 단축 — token 변경 후 deadlock fix (NASA agent 의 InfluxDB-like 외부 I/O 영향). (6) v0.7.7~v0.7.8: 노드 측 dedup 강화. |
| 2026-05-20 | 1.10.0 | **NASA 정기 보고 (`trigger=report`) 실제 구현**. v1.9.x 까지 `notifyTicker` 만 생성하고 소비 goroutine 없어 정기 보고 동작 안 함. `pushRecentSnapshotWithTrigger(addr, trigger)` 분리 + `notifyLoop` goroutine 추가 (NotifyInterval>0 일 때만 시작). AllCoreObserved 통과 device 만 trigger="report" 송신. a.wg 등록으로 Stop 시 정상 종료. |
| 2026-05-20 | 1.9.7 | **온도 게이트 범위**. `event_temp_threshold` 적용 — Power/Mode/TargetTemp/FanSpeed 변경 시 즉시 emit, 실내온도(CurrentTemp) 만 변경 시 \|Δ\| ≥ threshold (default 1.0℃) 일 때만 emit. `nonTempFieldsChangedNASA` + `maxTempDeltaNASA` helper. |
| 2026-05-19 | 1.9.4 | **NASA pushRecentSnapshot label fallback chain**. auto-discovered device 의 Name 이 비어있으면 metadata.label 이 누락되던 결함. fallback chain: Name → DeviceID → addr.String(). |
| 2026-05-19 | 1.9.2 | **NASA `offline_timeout` 설정 추가**. 디바이스 통신 없음 → 오프라인 판정 시간 (기본 30s, 0=비활성). Web UI 옵션 추가. |
| 2026-05-19 | 1.9.1 | **NASA `status_query_enabled` 옵션 (passive sniff only 모드)**. true (기본) 면 기존 동작 — pollLoop 가 status query 송신. false 면 송신 skip, 외부 컨트롤러의 polling 만 sniff. |
| 2026-05-19 | 1.9.0a | **옵션 명칭 통일 (notify→report)**. `notify_interval` → `report_interval`, `notify_mode` → `report_mode`. trigger 값 `keepalive` → `report`. 이전 명칭 deprecation alias. |
| 2026-05-14 | 1.9.0 | xagent04 실배포 검증 hotfix 반영. (1) **TCP 설정 필드 분리** — `tcp_address` 단일 필드를 `tcp_host` + `tcp_port` 로 분리 (REQ-NASA-001-02-03, §4.1 amend, 커밋 `7dcedfd`). (2) **TCP_NODELAY 활성화** — serial-to-ethernet 어댑터 경유 시 Nagle 알고리즘이 제어 프레임 타이밍을 깨뜨리는 문제 해결 (REQ-NASA-001-02-03 amend). (3) **poll_bulk content dedup + last_seen 시간 메타 제외** — 폴링 결과 중복 제거 시 `last_seen` 등 시간 메타데이터를 비교에서 제외, `message_type` 표준 도입 (REQ-NASA-001-04-03 amend, 커밋 `553d484`). (4) **log_decode_errors 옵션** — decode error WARN 로그를 옵션으로 억제 (기본 false, 신규 NASAConfig 필드). (5) **노드 Init-tolerance** — nasa/nasa-status/nasa-control 노드가 Init 시점에 agent 미발견 시 hard-fail 대신 deferred connection (REQ-NASA-001-09-04 amend). 관련: SPEC-ENGINE-001 v1.3.0 Module 8, SPEC-AGENT-005 v1.1.0, SPEC-SERIAL-001 v2.2.0. |


---

# SPEC-SAMSUNG-HVACR-001: Samsung HVACR-01 에이전트 / Samsung NASA 프로토콜 구현

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP(Flow-Based Programming) 플랫폼이다. Agent 시스템은 Transport Interface(통신 인터페이스)와 Protocol Definition(프로토콜 정의)을 결합하여 외부 장비와 통신한다.

Samsung NASA(Next-generation of Air-conditioning System Architecture) Agent는 삼성 시스템 에어컨을 RS-485 시리얼 또는 TCP 통신을 통해 제어하고 모니터링하는 커스텀 에이전트이다. NASA 프로토콜 정의 파일(`nasa.yaml`)을 기반으로 바이트 데이터를 파싱/직렬화하며, 디바이스 자동 탐색, 상태 폴링, 제어 명령 전송 기능을 제공한다.

기존 SPEC-SAGENT-001 Module 8에서 정의된 요구사항을 기반으로 하되, 독립적이고 완전한 SPEC으로 확장한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/samsung/`
- **신규 의존 패키지**: `go.bug.st/serial` v1.6+ (RS-485 시리얼 통신)
- **기존 인터페이스**:
  - `Agent` 인터페이스 (`internal/agent/agent.go`): Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
  - `BaseAgent` 구조체: `*lifecycle.BaseLifecycle` 임베딩, Transport 래핑, Stats 추적
  - `AgentConfig`: ID, Name, Type, Transport (TransportConfig: Type string + Options map[string]any)
  - `TypeRegistry`: RegisterType/CreateAgent/ListTypes/HasType
  - `SubscriberAgent` 인터페이스: Subscribe/Unsubscribe (토픽 기반 구독 - NASA에서는 디바이스 주소 기반 구독으로 활용 가능)
- **프로토콜 정의 엔진** (`internal/agent/protocol/`):
  - `definition.go`: 프로토콜 정의 구조체
  - `parser.go`: 설정 기반 바이트 파서/직렬화
  - `loader.go`: YAML 프로토콜 정의 파일 로더
  - `checksum.go`: 체크섬/CRC 검증
- **브릿지 노드** (`internal/node/bridge.go`):
  - `BridgeIn`: 에이전트 -> 플로우 (상태 데이터 수신)
  - `BridgeOut`: 플로우 -> 에이전트 (제어 명령 전송)
  - `BridgeInOut`: 양방향 (상태 수신 + 제어 명령)
  - `BridgeRequestReply`: 요청-응답 (상태 조회 명령)
- **브릿지 어댑터** (`internal/node/bridge_adapter.go`):
  - `BridgeAdapter` 인터페이스: 에이전트 타입별 데이터 변환
  - `CommandPollAdapter` 인터페이스: JSON 명령 기반 폴링 (v1.1.0 추가)
  - `PollableAdapter` 인터페이스: 레지스터 단위 폴링 (Modbus 등)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **기존 패턴 준수**: MQTT Agent, InfluxDB Agent의 설정 파싱, 생명주기 관리, Bridge 연동 패턴 활용
- **관심사 분리**: 에이전트 로직, 설정 파싱, 디바이스 관리, 프로토콜 처리, 트랜스포트 추상화를 별도 파일로 분리
- **테스트 가능성**: 인터페이스 기반 설계로 Transport와 Protocol을 목(mock) 주입하여 단위 테스트 가능
- **트랜스포트 추상화**: Serial과 TCP를 동일한 인터페이스로 추상화하여 런타임 선택 가능
- **프로토콜 정의 엔진 활용**: nasa.yaml을 통해 프로토콜 구조를 선언적으로 정의하고, Protocol Definition Engine이 바이트 파싱을 처리

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - NASAAgent 구현 (Agent 인터페이스 준수)
  - NASAConfig 설정 파싱 (Transport.Options 기반)
  - NASAAddress 타입 (`[3]byte`) 및 주소 체계 구현
  - NASADevice / NASADeviceState 타입 정의
  - NASA 프로토콜 프레임 인코딩/디코딩 (STX/LEN/SA/DA/CMD/SEQ#/CNT/MSGs/CRC/ETX)
  - NASAMessageSet 파싱/직렬화 (Index + Value 구조)
  - CRC16-CCITT 체크섬 구현
  - Serial(RS-485) / TCP 트랜스포트 추상화
  - 실외기/실내기 주소 자동 탐색 프로토콜 (C001/C005/C011/C012/C014/C015)
  - 디바이스 상태 폴링 및 관리 (C014 Notification 파싱)
  - 실내기 제어 명령 (C013: SetPower, SetMode, SetTemperature, SetFanSpeed)
  - 타입 등록 (RegisterSamsungNASATypes)
  - 센티널 에러 정의
  - 단위 테스트
  - Message Set 필터링 (UnsupportedMsgSets 기반) (v1.1.0)
  - HexKeyByteMap 커스텀 JSON 직렬화 타입 (v1.1.0)
  - StateForJSON 조건부 직렬화 메서드 (v1.1.0)
  - CommandPollAdapter 인터페이스 및 NASAAdapter 브릿지 어댑터 (v1.1.0)
  - 예제 YAML 설정 파일 (v1.1.0)
  - Transport 재연결 루프 및 지수 백오프 (v1.2.0)
  - 수신 루프 연결 끊김 감지 및 복구 (v1.2.0)
  - Transport I/O 에러 시 Available() 상태 갱신 (v1.2.0)
  - 재연결 이벤트 메시지 (transport_disconnected/reconnecting/reconnected) (v1.2.0)
  - Samsung NASA 프로토콜 전용 노드 타입 3종 (samsung_hvacr01_status, samsung_hvacr01_control, samsung_hvacr01) (v1.3.0)
  - NASANodeConfig 노드 레벨 설정 구조체 (v1.3.0)
  - 노드 레지스트리 등록 (12 -> 15 빌트인 노드) (v1.3.0)
  - 프론트엔드 노드 스키마 및 메타데이터 (v1.3.0)
  - BuzzerOnControl 설정 및 부저 자동 제어 (v1.4.0)
  - RS-485 프리앰블(0x55 x 100) 전송 (v1.4.0)
  - 프론트엔드 buzzer_on_control UI (v1.4.0)
  - Device Configuration 통합 구조체 (`Devices []agent.DeviceEntry`) (v1.7.0)
  - 주소 형식 표준화: 컴팩트 헥스 형식("200000") 통일 (v1.7.0)
  - TransportChecker 인터페이스 (`TransportConnected() bool`) (v1.7.0)
  - NASADeviceAdapter 프로토콜 추상화 (Protocol, ExtraProperties, DeviceSource) (v1.7.0)
- **범위 외(Out-of-Scope)**:
  - Bridge 노드 자체의 변경
  - Protocol Definition Engine의 변경 (기존 엔진 활용)
  - NASA 프로토콜의 전체 명령어 세트 중 미사용 항목 (롱바람, 무풍, 청정 등 고급 기능)
  - 실외기 직접 제어 기능 (실외기는 탐색 및 상태 모니터링만)
  - 실제 RS-485 하드웨어 통합 테스트
  - 다중 외부제어기 구성 (프로토콜상 1대 제한)

---

## 2. Assumptions (가정)

### A-1. NASA 프로토콜 가용성

NASA 프로토콜의 메시지 구조(패킷 포맷, 명령 코드, CRC16-CCITT 체크섬)가 리버스 엔지니어링과 테스트를 통해 분석되어 있다 (`references/protocols/samsung_nasa_protocol.md`). 다만 비공식 분석이므로 실제 프로토콜과 일부 차이가 있을 수 있다. `nasa.yaml` 프로토콜 정의 파일로 표현 가능하다고 가정한다.

### A-2. RS-485 하드웨어 접근

대상 시스템에 RS-485 USB 어댑터(`/dev/ttyUSB0` 등) 또는 TCP-to-Serial 게이트웨이가 설치되어 있다고 가정한다.

### A-3. 디바이스 주소 사전 설정

제어 대상 디바이스의 NASA 3바이트 주소(실외기: `10 xx 00`, 실내기: `20 xx yy`)가 사전에 알려져 있거나, 자동 탐색(Auto Discovery) 프로토콜을 통해 획득 가능하다고 가정한다. 외부제어기 주소는 `6A EE FF`로 고정되며, 전체 구성에서 외부제어기는 1대만 연결 가능하다.

### A-4. Protocol Definition Engine 호환성

기존 Protocol Definition Engine(`internal/agent/protocol/`)이 NASA 프로토콜의 바이트 구조를 파싱/직렬화할 수 있다고 가정한다. 가변 길이 필드, 조건부 파싱 등 고급 기능이 필요한 경우 프로토콜 모듈 내에서 자체 처리한다.

### A-5. 통신 환경

RS-485 버스에서의 충돌 방지는 NASA 프로토콜 레벨에서 처리되며(마스터-슬레이브 방식), 에이전트는 마스터 역할을 수행한다고 가정한다.

---

## 3. Requirements (요구사항)

### Module 1: NASAAgent Core (에이전트 코어)

#### REQ-NASA-001-01-01 (Ubiquitous) NASAAgent 구조체

NASAAgent 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드             | 타입                                | 설명                            |
| -------------- | --------------------------------- | ----------------------------- |
| `*BaseAgent`   | 임베딩                               | 기본 에이전트 기능 (생명주기, 통합 통계)      |
| `nasaConfig`   | `NASAConfig`                      | NASA 에이전트 전용 설정               |
| `devices`      | `map[NASAAddress]*NASADevice`     | 3바이트 주소 기반 디바이스 목록            |
| `deviceIDs`    | `map[string]NASAAddress`          | device_id → 주소 매핑 (역방향 조회용)   |
| `transport`    | `NASATransport`                   | 추상화된 트랜스포트 인터페이스              |
| `protocol`     | `NASAProtocol`                    | NASA 프로토콜 인코더/디코더             |
| `mu`           | `sync.RWMutex`                    | 디바이스 목록 동시성 보호                |
| `seqNum`       | `byte`                            | 시퀀스 넘버 (패킷마다 증가, 이전보다 큰 값 사용) |
| `pollTicker`   | `*time.Ticker`                    | 상태 폴링 타이머                     |
| `notifyTicker` | `*time.Ticker`                    | 주기적 상태 알림 타이머                 |
| `lastStates`   | `map[NASAAddress]NASADeviceState` | 이전 폴링 상태 (변경 감지용)             |
| `registryPath` | `string`                          | 디바이스 레지스트리 저장 경로              |
| `stopCh`       | `chan struct{}`                   | 정지 시그널 채널                     |
| `msgCh`        | `chan []byte`                     | Bridge로 전달할 수신 메시지 채널         |


#### REQ-NASA-001-01-02 (Ubiquitous) Agent 인터페이스 준수

NASAAgent는 **항상** `agent.Agent` 인터페이스를 구현해야 한다:

- `Init(config AgentConfig) error`
- `Start(ctx context.Context) error`
- `Stop(ctx context.Context) error`
- `Pause(ctx context.Context) error`
- `Resume(ctx context.Context) error`
- `Health() HealthStatus`
- `Process(data []byte) ([]byte, error)`
- `Configure(config AgentConfig) error`
- `ID() string`, `Name() string`, `Type() string`
- `Info() AgentInfo`, `Stats() StatsSnapshot`

#### REQ-NASA-001-01-03 (Ubiquitous) MessageReceiver 인터페이스 준수

NASAAgent는 **항상** `agent.MessageReceiver` 인터페이스를 구현해야 한다:

- `ReceiveMessage(ctx context.Context) ([]byte, error)`: 내부 `msgCh` 채널에서 디바이스 상태 변경 메시지를 JSON으로 반환한다.

#### REQ-NASA-001-01-04 (Event-Driven) Init 생명주기

**WHEN** `Init(config)` 호출 시 **THEN**:

1. `config.Transport.Options`에서 NASA 전용 설정을 파싱한다 (`parseNASAConfig`)
2. 트랜스포트 유형(`serial` 또는 `tcp`)에 따라 `NASATransport`를 생성한다
3. `nasa.yaml` 프로토콜 정의 파일을 로드하여 `NASAProtocol`을 초기화한다
4. 설정된 디바이스 주소 목록으로 `devices` 맵을 초기화한다
5. `BaseAgent.Init(config)`를 호출하여 상태를 `Running`으로 전이한다

#### REQ-NASA-001-01-05 (Event-Driven) Start 생명주기

**WHEN** `Start(ctx)` 호출 시 **THEN**:

1. `transport.Open()`을 호출하여 트랜스포트 연결을 시도한다
2. **WHEN** `transport.Open()`이 성공하면 **THEN**:
  - 폴링 고루틴(`pollLoop`)을 시작하여 `PollInterval` 주기로 디바이스 상태를 조회한다
  - 수신 고루틴(`receiveLoop`)을 시작하여 트랜스포트에서 응답 데이터를 읽고 파싱한다
3. **WHEN** `transport.Open()`이 실패하면 **THEN**:
  - WARN 레벨 로그를 1회 기록한다 ("트랜스포트 초기 연결 실패, 재연결 루프 시작")
  - `reconnectLoop` 고루틴을 시작한다 (REQ-NASA-001-01-09 참조)
  - `Start()`는 에러를 반환하지 **않는다** (`nil` 반환). 에이전트는 reconnecting 상태로 진입한다
  - `transport_reconnecting` 이벤트를 `msgCh`에 전달한다 (REQ-NASA-001-07-03 참조)

#### REQ-NASA-001-01-06 (Event-Driven) Stop 생명주기

**WHEN** `Stop(ctx)` 호출 시 **THEN**:

1. `stopCh` 채널을 닫아 폴링/수신 고루틴을 중지한다
2. `pollTicker`를 정지한다
3. 트랜스포트 연결을 닫는다
4. `BaseAgent.Stop(ctx)`를 호출하여 상태를 `Stopped`로 전이한다

#### REQ-NASA-001-01-07 (Event-Driven) Pause/Resume 생명주기

**WHEN** `Pause(ctx)` 호출 시 **THEN** 폴링을 일시 중지하되 트랜스포트 연결은 유지한다.
**WHEN** `Resume(ctx)` 호출 시 **THEN** 폴링을 재개한다.

#### REQ-NASA-001-01-08 (Ubiquitous) State() 디바이스 요약 출력 (v1.1.0)

`State()` 메서드(`agent.StatefulAgent` 인터페이스)는 **항상** 다음 정보를 포함하는 `map[string]any`를 반환해야 한다:


| 키                      | 타입                 | 설명                                                                                |
| ---------------------- | ------------------ | --------------------------------------------------------------------------------- |
| `device_count`         | `int`              | 등록된 전체 디바이스 수                                                                     |
| `online_count`         | `int`              | 현재 온라인 디바이스 수                                                                     |
| `devices`              | `[]map[string]any` | 각 디바이스의 주소, device_id, 타입, 온라인 상태, 상태 요약, last_seen                               |
| `unsupported_msg_sets` | `[]string`         | 설정된 필터링 대상 메시지 셋 인덱스 목록 (`"0x0608"` 형식). `UnsupportedMsgSets`가 비어있으면 이 키를 포함하지 않음 |
| `transport_connected`  | `bool`             | 현재 트랜스포트 연결 상태 (v1.2.0)                                                           |
| `reconnecting`         | `bool`             | 현재 재연결 루프 실행 중 여부 (v1.2.0)                                                        |
| `reconnect_attempts`   | `int`              | 현재 재연결 시도 횟수. 재연결 성공 시 0으로 리셋 (v1.2.0)                                            |


#### REQ-NASA-001-01-09 (Complex) Transport 재연결 루프 (v1.2.0)

**IF** transport 연결이 끊어진 상태에서 **AND WHEN** `ReconnectInterval`이 경과하면 **THEN**:

1. `transport.Close()`를 호출한 후 `transport.Open()`을 호출하여 재연결을 시도한다
2. **첫 번째 시도**: WARN 레벨 로그 "재연결 시도 중" 기록
3. **2번째 이후 시도**: DEBUG 레벨 로그만 기록 (WARN/ERROR 레벨 로그 출력 금지)
4. 지수 백오프 적용: `min(ReconnectInterval * 2^attempt, MaxReconnectBackoff)`
5. **WHEN** `transport.Open()`이 성공하면 **THEN**:
  - INFO 레벨 로그 "재연결 성공" 기록
  - `pollLoop` 및 `receiveLoop` 고루틴을 시작한다
  - `transport_reconnected` 이벤트를 `msgCh`에 전달한다 (`attempt_count`, `downtime_seconds` 포함)
  - 백오프 카운터를 리셋한다
6. **WHEN** `stopCh`가 닫히면 **THEN** `reconnectLoop`를 즉시 종료한다

#### REQ-NASA-001-01-10 (Event-Driven) 수신 루프 연결 끊김 감지 (v1.2.0)

**WHEN** `receiveLoop`의 `transport.Receive()` 호출이 실패하고 `transport.Available()`이 `false`를 반환하면 **THEN**:

1. 연결이 끊어진 것으로 판단한다 (`ErrTransportNotConnected` 또는 `Available()==false`)
2. 타임아웃 에러(`net.Error.Timeout()==true`)는 연결 단절로 간주하지 **않는다** (무시하고 수신 루프를 계속한다)
3. `disconnectCh` 채널을 통해 `pollLoop`에 중지 신호를 전달한다
4. `reconnectLoop` 고루틴을 시작한다 (중복 시작 방지를 위해 `reconnecting` atomic.Bool 확인)

#### REQ-NASA-001-01-11 (State-Driven) 연결 끊김 상태에서 폴링 중지 (v1.2.0)

**IF** transport가 연결 끊김 상태이면 **THEN** `pollLoop()`는 디바이스 폴링을 수행하지 **않아야 한다**. `disconnectCh` 시그널 수신 시 `pollLoop`를 종료한다.

#### REQ-NASA-001-01-12 (Ubiquitous) 재연결 관련 NASAAgent 필드 (v1.2.0)

NASAAgent 구조체는 **항상** 다음 재연결 관련 필드를 포함해야 한다:


| 필드             | 타입              | 설명                                        |
| -------------- | --------------- | ----------------------------------------- |
| `disconnectCh` | `chan struct{}` | receiveLoop에서 pollLoop로 연결 끊김 신호를 전달하는 채널 |
| `reconnecting` | `atomic.Bool`   | 중복 재연결 루프 시작 방지용 플래그                      |


---

### Module 2: Transport Layer (트랜스포트 레이어)

#### REQ-NASA-001-02-01 (Ubiquitous) NASATransport 인터페이스

NASATransport 인터페이스는 **항상** 다음 메서드를 제공해야 한다:


| 메서드         | 시그니처                               | 설명       |
| ----------- | ---------------------------------- | -------- |
| `Open`      | `Open() error`                     | 연결 열기    |
| `Close`     | `Close() error`                    | 연결 닫기    |
| `Send`      | `Send(data []byte) error`          | 데이터 전송   |
| `Receive`   | `Receive(buf []byte) (int, error)` | 데이터 수신   |
| `Available` | `Available() bool`                 | 연결 상태 확인 |


**Available() 계약 (v1.2.0 강화)**:

- `Available()`은 **항상** 실제 연결 상태를 반영해야 한다
- **WHEN** `Send()` 또는 `Receive()`에서 연결 단절 I/O 에러(`io.EOF`, `net.ErrClosed`, connection reset 등)가 발생하면 **THEN** 내부 `open` 플래그를 `false`로 설정하여 `Available()`이 `false`를 반환하도록 한다 (REQ-NASA-001-02-05 참조)
- 타임아웃 에러(`net.Error.Timeout()==true`)는 연결 단절로 간주하지 **않는다**

#### REQ-NASA-001-02-02 (Ubiquitous) Serial 트랜스포트

Serial 트랜스포트(`NASASerialTransport`)는 **항상** 다음 설정을 지원해야 한다:


| 설정         | 타입              | 기본값      | 설명                            |
| ---------- | --------------- | -------- | ----------------------------- |
| `Port`     | `string`        | - (필수)   | 시리얼 포트 경로 (예: `/dev/ttyUSB0`) |
| `BaudRate` | `int`           | 9600     | 통신 속도                         |
| `DataBits` | `int`           | 8        | 데이터 비트                        |
| `StopBits` | `int`           | 1        | 스톱 비트                         |
| `Parity`   | `string`        | `"even"` | 패리티 (none, odd, even)         |
| `Timeout`  | `time.Duration` | `1s`     | 읽기 타임아웃                       |


`go.bug.st/serial` v1.6+ 패키지를 사용하여 시리얼 포트를 관리한다.

#### REQ-NASA-001-02-03 (Ubiquitous) TCP 트랜스포트

TCP 트랜스포트(`NASATCPTransport`)는 **항상** 다음 설정을 지원해야 한다:


| 설정               | 타입              | 기본값    | 설명                               |
| ---------------- | --------------- | ------ | -------------------------------- |
| `Host`           | `string`        | - (필수) | TCP 호스트 (예: `192.168.1.100`) — v1.9.0 |
| `Port`           | `int`           | - (필수) | TCP 포트 (예: `4196`) — v1.9.0      |
| `ConnectTimeout` | `time.Duration` | `5s`   | 연결 타임아웃                          |
| `ReadTimeout`    | `time.Duration` | `3s`   | 읽기 타임아웃                          |


**v1.2.0 변경**: `ReconnectInterval`과 `MaxReconnectAttempts` 필드는 TCP 트랜스포트에서 제거됨. 재연결 로직은 에이전트 레벨(`NASAAgent.reconnectLoop`)에서 통합 관리한다 (REQ-NASA-001-01-09 참조). Serial과 TCP 트랜스포트 모두 동일한 재연결 메커니즘을 사용한다.

**v1.9.0 변경 — TCP 설정 필드 분리**: 기존 `Address` (`tcp_address`, `"host:port"` 단일 문자열) 필드를 `Host` (`tcp_host`) + `Port` (`tcp_port`) 두 필드로 분리한다. 하위 호환을 위해 `tcp_address` 가 제공되면 `host:port` 로 분해하여 파싱하되, 새 설정에서는 `tcp_host`/`tcp_port` 를 기본으로 사용한다.

**v1.9.0 변경 — TCP_NODELAY 활성화**: `NASATCPTransport` 는 연결 수립 후 **항상** 소켓에 `TCP_NODELAY` 를 설정하여 Nagle 알고리즘을 비활성화해야 한다. serial-to-ethernet 어댑터를 경유할 때 Nagle 알고리즘이 작은 제어 프레임을 묶어 전송 타이밍을 깨뜨리는 문제를 방지한다.

#### REQ-NASA-001-02-04 (Event-Driven) 트랜스포트 팩토리

**WHEN** `transport_type` 설정 값이 `"serial"` **THEN** `NASASerialTransport`를 생성한다.
**WHEN** `transport_type` 설정 값이 `"tcp"` **THEN** `NASATCPTransport`를 생성한다.
**IF** `transport_type` 값이 `"serial"` 또는 `"tcp"`가 아닌 경우 **THEN** `ErrInvalidTransportType` 에러를 반환한다.

#### REQ-NASA-001-02-05 (Event-Driven) Transport I/O 에러 시 상태 갱신 (v1.2.0)

**WHEN** Serial 또는 TCP 트랜스포트의 `Send()` 또는 `Receive()`에서 `io.EOF`, `net.ErrClosed`, 또는 connection reset 에러가 발생하면 **THEN**:

1. 내부 `open` 플래그를 `false`로 설정한다 (`Available()`이 `false`를 반환하도록)
2. 기존 연결 리소스를 정리한다 (소켓/포트 닫기 등)
3. 타임아웃 에러(`net.Error.Timeout()==true`)는 연결 단절로 간주하지 **않는다** — `open` 플래그를 변경하지 않는다

#### REQ-NASA-001-02-07 (Ubiquitous) TransportChecker 인터페이스 (v1.7.0)

`TransportChecker` 인터페이스는 **항상** 다음 메서드를 제공해야 한다:


| 메서드                    | 시그니처                          | 설명                              |
| --------------------- | ----------------------------- | ------------------------------- |
| `TransportConnected`  | `TransportConnected() bool`   | 실제 시리얼/TCP 트랜스포트 연결 상태를 반환한다 |


**설계 의도:**

- `TransportConnected()`는 에이전트의 라이프사이클 상태(`Running`, `Paused` 등)와 **별개로** 실제 물리적 트랜스포트 연결 상태를 반환한다
- NASAAgent가 이 인터페이스를 구현하여, 외부에서 트랜스포트 연결 상태를 라이프사이클과 독립적으로 조회할 수 있도록 한다
- `State()` 메서드의 `transport_connected` 필드와 동일한 값을 반환한다

#### REQ-NASA-001-02-06 (Ubiquitous) RS-485 프리앰블 전송 (v1.4.0)

Serial 및 TCP 트랜스포트의 `Send()` 메서드는 **항상** 프레임 데이터 앞에 RS-485 프리앰블을 추가하여 전송해야 한다:

1. 프리앰블은 100바이트의 `0x55` 값으로 구성된다 (`preambleLen = 100`)
2. `prependPreamble(frame []byte) []byte` 헬퍼 함수가 프리앰블 + 프레임 결합 버퍼를 반환한다
3. 프리앰블은 패키지 레벨 `init()` 함수에서 한 번 초기화된다
4. Serial과 TCP 트랜스포트 모두 동일한 프리앰블 로직을 사용한다

RS-485 버스에서 수신 측이 바이트 동기화를 확보할 수 있도록 프리앰블 바이트를 선행 전송한다.

---

### Module 3: NASA Protocol (프로토콜 처리)

#### REQ-NASA-001-03-01 (Ubiquitous) NASAAddress 타입

`NASAAddress` 타입(`[3]byte`)은 **항상** 다음 헬퍼 메서드를 제공해야 한다:


| 메서드            | 시그니처                                        | 설명                                 |
| -------------- | ------------------------------------------- | ---------------------------------- |
| `String`       | `String() string`                           | 주소를 `"XX XX XX"` 형태 문자열로 반환        |
| `Hex`          | `Hex() string`                              | 주소를 `"XXXXXX"` compact hex 문자열로 반환 |
| `IsOutdoor`    | `IsOutdoor() bool`                          | 실외기 주소 여부 (`10 xx 00`)             |
| `IsIndoor`     | `IsIndoor() bool`                           | 실내기 주소 여부 (`20 xx yy`)             |
| `IsController` | `IsController() bool`                       | 외부제어기 주소 여부 (`6A EE FF`)           |
| `IsBroadcast`  | `IsBroadcast() bool`                        | 브로드캐스트 주소 여부 (`B0`/`B2`/`B3` 프리픽스) |
| `OutdoorIndex` | `OutdoorIndex() byte`                       | 실외기 물리 주소 `xx` (0x00~0x0F) 반환      |
| `IndoorIndex`  | `IndoorIndex() (outdoor byte, indoor byte)` | 실내기의 실외기 주소와 실내기 주소 반환             |


**알려진 주소 상수:**


| 상수명                   | 값                           | 설명                     |
| --------------------- | --------------------------- | ---------------------- |
| `AddrController`      | `[3]byte{0x6A, 0xEE, 0xFF}` | 외부제어기 (전체 구성에서 1대만 가능) |
| `AddrBroadcastAll`    | `[3]byte{0xB0, 0xFF, 0xFF}` | 전체 브로드캐스트              |
| `AddrBroadcastIndoor` | `[3]byte{0xB2, 0xFF, 0x20}` | 전체 실내기 브로드캐스트          |


**주소 생성 헬퍼 함수:**


| 함수                    | 시그니처                                                   | 설명                                  |
| --------------------- | ------------------------------------------------------ | ----------------------------------- |
| `NewOutdoorAddr`      | `NewOutdoorAddr(index byte) NASAAddress`               | `[3]byte{0x10, index, 0x00}` 생성     |
| `NewIndoorAddr`       | `NewIndoorAddr(outdoor, indoor byte) NASAAddress`      | `[3]byte{0x20, outdoor, indoor}` 생성 |
| `NewOutdoorBroadcast` | `NewOutdoorBroadcast(index byte) NASAAddress`          | `[3]byte{0xB0, index, 0xFF}` 생성     |
| `NewIndoorBroadcast`  | `NewIndoorBroadcast(outdoor, indoor byte) NASAAddress` | `[3]byte{0xB3, outdoor, indoor}` 생성 |


**주소 파싱 함수:**


| 함수                 | 시그니처                                              | 설명                                                                                                                      |
| ------------------ | ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `ParseNASAAddress` | `ParseNASAAddress(s string) (NASAAddress, error)` | 문자열을 NASAAddress로 파싱. spaced hex(`"20 00 01"`), compact hex(`"200001"`) 두 형식 모두 지원. 유효하지 않은 형식이면 `ErrInvalidAddress` 반환 |


**지원 주소 문자열 형식:**


| 형식          | 예시           | 설명                              |
| ----------- | ------------ | ------------------------------- |
| Spaced hex  | `"20 00 01"` | 공백으로 구분된 3개의 2자리 hex (기본 출력 형식) |
| Compact hex | `"200001"`   | 공백 없는 6자리 hex 문자열               |


두 형식 모두 대소문자를 구분하지 않는다 (`"2A00FF"`, `"2a00ff"`, `"2A 00 FF"` 모두 유효).

#### REQ-NASA-001-03-02 (Ubiquitous) NASAMessage 구조체

NASAMessage 구조체는 **항상** 실제 NASA 프레임 구조에 따라 다음 필드를 포함해야 한다:

**프레임 구조:**

```
[STX][LEN][SA][DA][CMD][SEQ#][CNT][MSG0][MSG1]...[CRC][ETX]
```


| 필드            | 타입                 | 크기      | 설명                         |
| ------------- | ------------------ | ------- | -------------------------- |
| `SourceAddr`  | `NASAAddress`      | 3 bytes | 송신 주소 (SA)                 |
| `DestAddr`    | `NASAAddress`      | 3 bytes | 수신 주소 (DA)                 |
| `CommandCode` | `uint16`           | 2 bytes | 명령 코드 (CMD)                |
| `SequenceNum` | `byte`             | 1 byte  | 시퀀스 번호 (SEQ#, 이전보다 큰 값 사용) |
| `MessageSets` | `[]NASAMessageSet` | 가변      | Message Set 목록 (CNT개)      |
| `Checksum`    | `uint16`           | 2 bytes | CRC16-CCITT 체크섬            |
| `Raw`         | `[]byte`           | 가변      | 원본 바이트 데이터 (STX~ETX 포함)    |


**프레임 바이트 구조:**


| 위치      | 크기      | 필드   | 설명                                |
| ------- | ------- | ---- | --------------------------------- |
| 0       | 1 byte  | STX  | 고정값 `0x32`                        |
| 1~2     | 2 bytes | LEN  | 패킷 길이 (STX, ETX 제외, Big-Endian)   |
| 3~5     | 3 bytes | SA   | Source Address                    |
| 6~8     | 3 bytes | DA   | Destination Address               |
| 9~10    | 2 bytes | CMD  | Command Code                      |
| 11      | 1 byte  | SEQ# | Sequence Number                   |
| 12      | 1 byte  | CNT  | Number of Message Sets            |
| 13~N    | 가변      | MSGs | Message Set 데이터 (Index + Value 쌍) |
| N+1~N+2 | 2 bytes | CRC  | CRC16-CCITT                       |
| N+3     | 1 byte  | ETX  | 고정값 `0x34`                        |


#### REQ-NASA-001-03-02-01 (Ubiquitous) NASAMessageSet 구조체

NASAMessageSet 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드      | 타입       | 설명                                       |
| ------- | -------- | ---------------------------------------- |
| `Index` | `uint16` | Message Index (2 bytes)                  |
| `Value` | `[]byte` | Message Value (크기는 Index의 2번째 니블에 의해 결정) |


**Value 크기 결정 규칙** (Index 2번째 니블 기준):


| Index 2번째 니블 | Value 크기 (bytes) | 예시                              |
| ------------ | ---------------- | ------------------------------- |
| `0`          | 1                | `4000` (Power) → Value 1 byte   |
| `1`          | 1                | `4011` (Swing) → Value 1 byte   |
| `2`          | 2                | `4201` (설정온도) → Value 2 bytes   |
| `4`          | 4                | `0409` (리모컨 제한) → Value 4 bytes |
| `6`          | Command에 따라 다름   | 가변 길이                           |


#### REQ-NASA-001-03-02-02 (Ubiquitous) 알려진 명령 코드 상수

다음 명령 코드 상수가 **항상** 정의되어야 한다:


| 상수명                  | 값        | 소스      | 대상      | 설명                     |
| -------------------- | -------- | ------- | ------- | ---------------------- |
| `CmdStandbyRequest`  | `0xC001` | 외부제어기   | 실외기     | 대기 요청 (주소 확인)          |
| `CmdStandbyResponse` | `0xC005` | 실외기     | 외부제어기   | 대기 응답                  |
| `CmdNormalRequest`   | `0xC011` | 외부제어기   | 실외기/실내기 | 일반 요청 (상태 조회)          |
| `CmdNormalSetting`   | `0xC012` | 외부제어기   | 실외기     | 일반 설정 (주소 확정)          |
| `CmdNormalControl`   | `0xC013` | 외부제어기   | 실내기     | 일반 제어 (실내기 제어)         |
| `CmdNotification`    | `0xC014` | 실내외기    | -       | 상태 알림/통보               |
| `CmdAddressResponse` | `0xC015` | 실외기/실내기 | 외부제어기   | 주소/준비 상태 응답            |
| `CmdControlResponse` | `0xC016` | 실내기     | 외부제어기   | 제어 응답 (Message Set 없음) |


#### REQ-NASA-001-03-02-03 (Ubiquitous) 알려진 Message Index 상수

다음 Message Index 상수가 **항상** 정의되어야 한다:


| 상수명                   | Index    | Value 크기  | 설명        | Value 매핑                                                      |
| --------------------- | -------- | --------- | --------- | ------------------------------------------------------------- |
| `MsgPower`            | `0x4000` | 1 byte    | 전원        | `0x00`=off, `0x01`=on                                         |
| `MsgMode`             | `0x4001` | 1 byte    | 운전 모드     | `0x00`=auto, `0x01`=cool, `0x02`=dry, `0x03`=fan, `0x04`=heat |
| `MsgFanSpeed`         | `0x4006` | 1 byte    | 풍량        | `0x00`=auto, `0x01`=low, `0x02`=medium, `0x03`=high           |
| `MsgLongWind`         | `0x4007` | 1 byte    | 롱바람       | -                                                             |
| `MsgSwingVertical`    | `0x4011` | 1 byte    | 풍향 (상하)   | `0x00`=off, `0x01`=on                                         |
| `MsgFilterCleanReset` | `0x4025` | 1 byte    | 필터 청소 리셋  | `0x00`=off, `0x01`=on                                         |
| `MsgFilterCleanAlarm` | `0x4027` | 1 byte    | 필터 청소 알림  | `0x00`=off, `0x01`=on                                         |
| `MsgXSFM`      | `0x4043` | 1 byte    | 청정        | -                                                             |
| `MsgBuzzer`           | `0x4050` | 1 byte    | 부저        | `0x00`=on, `0x01`=off (역논리)                                   |
| `MsgWindless`         | `0x4060` | 1 byte    | 무풍        | -                                                             |
| `MsgSwingHorizontal`  | `0x407E` | 1 byte    | 풍향 (좌우)   | -                                                             |
| `MsgAutoDry`          | `0x4111` | 1 byte    | 자동건조 설정   | -                                                             |
| `MsgTargetTemp`       | `0x4201` | 2 bytes   | 설정 온도     | temp x 10, uint16 BE (18C=`0x00B4`)                           |
| `MsgCurrentTemp`      | `0x4203` | 2 bytes   | 실내 온도     | temp x 10, `0x0000~~0x7FFF`=영상, `0x8000~~0xFFFF`=영하           |
| `MsgErrorCode`        | `0x0202` | 2 bytes   | 에러 코드     | 2 bytes                                                       |
| `MsgRemoteLimit`      | `0x0409` | 4 bytes   | 리모컨 사용 제한 | `0x00000000`=제한없음, `0x00006A6A`=사용제한                          |
| `MsgAddrInfo`         | `0x0408` | 4 bytes   | 주소 정보     | 주소 확인/등록에 사용                                                  |
| `MsgAddrRegister`     | `0x2004` | 1~4 bytes | 주소 등록 상태  | `0x00`=확인요청, `0x01`=등록필요, `0x03`=등록요청, `0x04`=등록완료            |
| `MsgReadyState`       | `0x2010` | 1 byte    | 통신 준비 상태  | `0xAx`=준비완료, 기타=준비안됨                                          |


#### REQ-NASA-001-03-01-01 (Ubiquitous) NASAProtocol 인터페이스

NASAProtocol 인터페이스는 **항상** 다음 메서드를 제공해야 한다:


| 메서드                   | 시그니처                                                                                        | 설명                                  |
| --------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------- |
| `Encode`              | `Encode(msg *NASAMessage) ([]byte, error)`                                                  | 메시지를 NASA 프레임 바이트로 인코딩 (STX~ETX 포함) |
| `Decode`              | `Decode(data []byte) (*NASAMessage, error)`                                                 | 바이트를 NASAMessage로 디코딩               |
| `BuildStatusQuery`    | `BuildStatusQuery(addr NASAAddress, seqNum byte) ([]byte, error)`                           | C011 상태 조회 메시지 생성                   |
| `BuildControlCommand` | `BuildControlCommand(addr NASAAddress, seqNum byte, sets []NASAMessageSet) ([]byte, error)` | C013 제어 명령 메시지 생성                   |
| `CalculateChecksum`   | `CalculateChecksum(data []byte) uint16`                                                     | CRC16-CCITT 체크섬 계산                  |
| `ParseMessageSets`    | `ParseMessageSets(data []byte, count int) ([]NASAMessageSet, error)`                        | 바이트에서 Message Set 목록 파싱             |
| `EncodeMessageSets`   | `EncodeMessageSets(sets []NASAMessageSet) []byte`                                           | Message Set 목록을 바이트로 인코딩            |


#### REQ-NASA-001-03-03 (Event-Driven) 프로토콜 메시지 파싱

**WHEN** 트랜스포트에서 데이터가 수신되면 **THEN**:

1. STX 바이트(`0x32`)를 탐지하여 프레임 시작을 식별한다 (STX 앞의 임의 바이트 무시)
2. LEN 필드(2 bytes, Big-Endian)를 읽어 패킷 길이를 확인한다
3. SA(3 bytes), DA(3 bytes), CMD(2 bytes), SEQ#(1 byte), CNT(1 byte)를 순서대로 파싱한다
4. CNT 값에 따라 Message Set을 파싱한다 (각 Index의 2번째 니블로 Value 크기 결정)
5. CRC(2 bytes, CRC16-CCITT)를 읽고 검증한다
6. ETX 바이트(`0x34`)를 확인한다
7. 파싱된 `NASAMessage`를 반환한다

**참고**: 수신 데이터가 끊길 경우 버퍼링하여 다음 데이터와 연결 처리한다.

#### REQ-NASA-001-03-04 (Unwanted) CRC 불일치 거부

시스템은 CRC16-CCITT 체크섬이 일치하지 않는 메시지를 **수락하지 않아야 한다**. CRC 오류 시 `ErrChecksumMismatch` 에러를 반환하고, 오류 카운터를 증가시키며, 로그에 기록한다.

#### REQ-NASA-001-03-04-01 (Ubiquitous) CRC16-CCITT 구현

CRC16-CCITT 알고리즘이 **항상** 별도 파일(`crc.go`)로 구현되어야 한다:

- 다항식: 0x1021
- 초기값: 프로토콜 사양에 따라 결정 (테스트 벡터로 검증)
- 입력: SA~마지막 Message Set (STX, LEN, CRC, ETX 제외)
- 출력: `uint16` (2 bytes, Big-Endian으로 프레임에 삽입)

#### REQ-NASA-001-03-05 (Ubiquitous) nasa.yaml 프로토콜 정의

NASA 프로토콜 정의 파일(`nasa.yaml`)은 **항상** Protocol Definition Engine과 호환되는 형식으로 다음 정보를 포함해야 한다:

- 패킷 구조: STX(`0x32`), LEN(2 bytes BE), SA(3 bytes), DA(3 bytes), CMD(2 bytes), SEQ#(1 byte), CNT(1 byte), MSGs(가변), CRC(2 bytes CRC16-CCITT), ETX(`0x34`)
- 필드 정의: 타입, 오프셋, 크기, 바이트 오더 (전체 Big-Endian)
- 명령 코드 목록: `C001`(Standby Request), `C005`(Standby Response), `C011`(Normal Request), `C012`(Normal Setting), `C013`(Normal Control), `C014`(Notification), `C015`(Address/Ready Response), `C016`(Control Response)
- Message Index 정의: `4000`(전원), `4001`(모드), `4006`(풍량), `4201`(설정온도), `4203`(실내온도) 등
- Value 크기 규칙: Index 2번째 니블 기반 (`0`→1byte, `1`→1byte, `2`→2bytes, `4`→4bytes, `6`→가변)
- 체크섬 알고리즘: CRC16-CCITT (2 bytes)

#### REQ-NASA-001-03-06 (Optional) 내장 프로토콜 정의

**가능하면** `nasa.yaml` 파일을 Go 바이너리에 `embed` 패키지로 내장하여, 외부 파일 없이도 기본 프로토콜 정의를 사용할 수 있도록 제공한다.

#### REQ-NASA-001-03-07 (Event-Driven) Message Set 필터링 (v1.1.0)

**WHEN** 트랜스포트에서 수신된 메시지의 Message Set을 파싱한 후 **THEN**:

1. `NASAConfig.UnsupportedMsgSets`에 등록된 인덱스를 가진 Message Set을 필터링하여 제외한다
2. **WHEN** `NASAConfig.LogUnsupportedMsgSets`가 `true`이면 **THEN** 필터링된 각 인덱스를 디버그 레벨로 로그에 기록한다 (주소, 인덱스 포함)
3. 필터링된 Message Set 목록을 `UpdateFromMessageSets()`에 전달한다

`filterMessageSets(sets []NASAMessageSet, addr NASAAddress) []NASAMessageSet` 메서드로 구현되며, `handleMessage` 내에서 `UpdateFromMessageSets()` 호출 전에 적용된다.

---

### Module 4: Device Management (디바이스 관리)

#### REQ-NASA-001-04-01 (Ubiquitous) NASADevice 구조체

NASADevice 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드           | 타입                 | 설명                                                                                     |
| ------------ | ------------------ | -------------------------------------------------------------------------------------- |
| `Address`    | `NASAAddress`      | 3바이트 NASA 주소 (예: `[3]byte{0x20, 0x00, 0x01}` = 실외기 0번의 실내기 1번)                         |
| `DeviceID`   | `string`           | 사용자 지정 디바이스 식별자 (예: `"living-room"`, `"bedroom-1"`). 설정 또는 런타임 등록 시 지정 가능. 빈 문자열이면 미설정 |
| `Type`       | `string`           | 디바이스 유형: `"indoor"`, `"outdoor"`, `"controller"` — Address 프리픽스로 자동 판별                 |
| `Online`     | `bool`             | 온라인 상태                                                                                 |
| `Ready`      | `bool`             | 통신 준비 완료 상태 (실외기: C015 응답의 `0xAx` 값으로 판단)                                              |
| `LastSeen`   | `time.Time`        | 마지막 응답 수신 시각                                                                           |
| `State`      | `*NASADeviceState` | 현재 디바이스 상태 (실내기만 해당)                                                                   |
| `ErrorCount` | `int`              | 연속 에러 횟수                                                                               |
| `Source`     | `string`           | 등록 출처: `"config"`, `"bridge"`, `"auto"`, `"discovery"`                                 |


#### REQ-NASA-001-04-02 (Ubiquitous) NASADeviceState 구조체

NASADeviceState 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드               | 타입              | 설명                                                                                       | Message Index |
| ---------------- | --------------- | ---------------------------------------------------------------------------------------- | ------------- |
| `Power`          | `bool`          | 전원 상태 (on/off)                                                                           | `0x4000`      |
| `Mode`           | `string`        | 운전 모드: `"cool"`, `"heat"`, `"dry"`, `"fan"`, `"auto"`                                    | `0x4001`      |
| `TargetTemp`     | `float32`       | 설정 온도 (temp*10 uint16 BE에서 디코딩)                                                          | `0x4201`      |
| `CurrentTemp`    | `float32`       | 현재 실내 온도 (0x0000~~0x7FFF=영상, 0x8000~~0xFFFF=영하)                                          | `0x4203`      |
| `FanSpeed`       | `string`        | 풍량: `"auto"`, `"low"`, `"medium"`, `"high"`                                              | `0x4006`      |
| `SwingVertical`  | `bool`          | 풍향 상하 스윙                                                                                 | `0x4011`      |
| `FilterAlarm`    | `bool`          | 필터 청소 알림 상태                                                                              | `0x4027`      |
| `ErrorCode`      | `uint16`        | 에러 코드 (0 = 정상, 2 bytes)                                                                  | `0x0202`      |
| `RawMessageSets` | `HexKeyByteMap` | 수신된 전체 Message Set 원본 (미해석 포함). v1.1.0에서 타입이 `map[uint16][]byte`에서 `HexKeyByteMap`으로 변경됨 | -             |


**온도 디코딩 규칙:**

- 수신된 uint16 BE 값을 10으로 나누어 섭씨 온도 산출
- `0x0000`~`0x7FFF`: 영상 온도 (예:` 0x00F0` = 24.0C)
- `0x8000`~`0xFFFF`: 영하 온도 (예:` 0xFFF6` = -1.0C, 2의 보수 해석)

#### REQ-NASA-001-04-02-01 (Ubiquitous) HexKeyByteMap 커스텀 타입 (v1.1.0)

`HexKeyByteMap` 타입(`map[uint16][]byte`)은 **항상** 다음 JSON 직렬화/역직렬화 동작을 제공해야 한다:


| 메서드               | 설명                                                                                                 |
| ----------------- | -------------------------------------------------------------------------------------------------- |
| `MarshalJSON()`   | `uint16` 키를 `"0x0402"` 형식의 4자리 대문자 hex 문자열로 변환하여 JSON 출력. 기본 Go JSON의 10진수 키(`"1026"`) 대신 사용       |
| `UnmarshalJSON()` | `"0x0402"`(hex) 및 `"1026"`(10진수) 형식 모두를 파싱하여 `uint16` 키로 변환. `0x`/`0X` 접두사가 있으면 16진수, 없으면 10진수로 해석 |


`NASADeviceState.RawMessageSets` 필드의 타입으로 사용되어, 상태 JSON 출력 시 Message Set 인덱스가 읽기 쉬운 hex 형식으로 표시된다.

#### REQ-NASA-001-04-02-02 (Ubiquitous) StateForJSON 조건부 직렬화 (v1.1.0)

`StateForJSON(includeRaw bool) any` 메서드는 **항상** 다음 동작을 수행해야 한다:


| `includeRaw` | 반환 타입              | 설명                                                                                                                        |
| ------------ | ------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| `true`       | `*NASADeviceState` | 전체 상태 반환 (RawMessageSets 포함)                                                                                              |
| `false`      | `*stateWithoutRaw` | `RawMessageSets`를 제외한 상태 반환. Power, Mode, TargetTemp, CurrentTemp, FanSpeed, SwingVertical, FilterAlarm, ErrorCode 필드만 포함 |


`NASAConfig.IncludeRawMessageSets` 설정으로 제어되며, `get_state`/`get_all_states` 명령의 JSON 응답에서 `RawMessageSets` 포함 여부를 결정한다. 기본값은 `true` (포함).

#### REQ-NASA-001-04-03 (Event-Driven) 디바이스 상태 폴링

**WHEN** `PollInterval` 주기가 도래하면 **THEN**:

1. 등록된 모든 디바이스 주소에 상태 조회 명령을 순차 전송한다
2. 각 응답을 파싱하여 `NASADeviceState`를 업데이트한다
3. `LastSeen` 타임스탬프를 갱신한다
4. `lastStates`와 비교하여 상태 변경이 감지되면 즉시 `device_state_changed` 메시지를 `msgCh`에 전달한다
5. `lastStates`를 현재 상태로 갱신한다

**참고**: 폴링은 디바이스 내부 상태 갱신 주기이며, 플로우 알림은 REQ-NASA-001-06-02에서 별도 관리한다.

#### REQ-NASA-001-04-03-01 (Event-Driven) poll_bulk content dedup (v1.9.0)

**WHEN** 멀티 메시지 폴링 결과(`splitNASAPollResult`, REQ v1.8.0)를 중복 제거(dedup)할 때, **THEN** 시스템은 다음을 수행해야 한다:

1. 메시지 콘텐츠를 기준으로 중복 여부를 판정한다
2. 중복 판정 시 `last_seen` 등 **시간 관련 메타데이터는 비교에서 제외**한다 — 동일한 상태가 폴링 시각만 다르게 반복 전달되는 것을 방지한다
3. 각 폴링 결과 메시지의 `metadata.message_type` 필드를 표준 값(`event` 또는 `response`)으로 설정한다 — 모든 agent 노드에 통일된 `message_type` 메타데이터 표준을 따른다

> v1.8.0 까지는 시간 메타데이터까지 비교에 포함되어 사실상 모든 폴링 결과가
> "변경됨" 으로 판정되어 dedup 이 무력화되는 문제가 있었다.

#### REQ-NASA-001-04-07 (Event-Driven) 주기적 상태 알림

**WHEN** `NotifyInterval` 주기가 도래하면 **THEN**:

1. 등록된 모든 온라인 디바이스의 현재 상태를 `device_state_report` 메시지로 `msgCh`에 전달한다
2. 상태 변경 여부와 무관하게 매 주기마다 보고한다
3. `NotifyInterval`이 0이면 주기적 알림을 비활성화한다 (변경 알림만 동작)

#### REQ-NASA-001-04-12 (Event-Driven) 실외기 주소 탐색 프로토콜

**WHEN** `Start(ctx)` 호출 시 또는 `AutoDiscovery`가 활성화된 상태에서 초기화 시 **THEN** 다음 단계로 실외기를 탐색한다:

**단계 1 — 주소 등록 필요 확인:**

1. C014 명령을 SA=`6A EE FF`(외부제어기), DA=`B0 FF FF`(전체 브로드캐스트)로 전송
2. Message Set: Index=`0x2004`, Value=`0x00` (주소 등록 필요 확인)
3. 주소 등록이 필요한 실외기(`10 FF FF`)가 Message Set Index=`0x2004` Value=`0x01`과 함께 Random Address(`0x0418`), Network Address(`0x0217`), Origin Address(`0x0417`), Setting Address(`0x0419`)를 응답

**단계 2 — 주소 확인 (Standby Request/Response):**

1. C001 명령을 SA=`6A EE FF`, DA=`B0 FF 10`(실외기 브로드캐스트)로 전송
2. Message Set: Index=`0x0408`, Value=`0xFFFFFFFF` (주소 확인 요청)
3. 등록된 실외기(`10 xx 00`)들이 C005 응답으로 자신의 주소를 반환
4. 수신된 각 실외기 주소를 `devices` 맵에 `Source="discovery"`로 등록

**단계 3 — 주소 확정 (필요 시):**

1. 단계 1에서 등록 필요 응답을 받은 실외기에 대해:
2. C012 명령을 해당 실외기의 Random Address로 전송
3. Message Set: Index=`0x2004` Value=`0x03`(등록요청) + 수신된 주소 정보 포함
4. 실외기가 C015 응답으로 Index=`0x2004` Value=`0x04`(등록완료)를 반환

**단계 4 — 통신 준비 상태 확인:**

1. C011 명령을 SA=`6A EE FF`, DA=`B0 FF 10`으로 전송
2. Message Set: Index=`0x2010`, Value=`0xFF`
3. 실외기가 C015 응답으로 Index=`0x2010` 값을 반환
4. Value가 `0xAx` 패턴이면 Ready 상태 — `NASADevice.Ready`를 `true`로 설정
5. 그 외 값이면 아직 준비 안됨 — 일정 시간 후 재시도

#### REQ-NASA-001-04-13 (Event-Driven) 실내기 주소 탐색 프로토콜

**WHEN** 실외기 탐색이 완료된 후 **THEN** 다음 단계로 실내기를 탐색한다:

1. C011 명령을 SA=`6A EE FF`, DA=`B2 FF 20`(전체 실내기 브로드캐스트)로 전송
2. Message Set: Index=`0x0408`, Value=`0xFFFFFFFF`
3. 각 실내기(`20 xx yy`)가 C015 응답으로 자신의 주소를 반환
4. 수신된 각 실내기 주소를 `devices` 맵에 `Source="discovery"`로 등록
5. `device_discovered` 이벤트를 `msgCh`에 전달한다

#### REQ-NASA-001-04-04 (Event-Driven) 디바이스 오프라인 감지

**WHEN** 디바이스가 연속 3회 폴링에 응답하지 않으면 **THEN**:

1. `Online` 상태를 `false`로 변경한다
2. 오프라인 이벤트 메시지를 `msgCh`에 전달한다
3. 로그에 경고를 기록한다

**WHEN** 오프라인 디바이스가 다시 응답하면 **THEN**:

1. `Online` 상태를 `true`로 복원한다
2. `ErrorCount`를 0으로 초기화한다
3. 온라인 복구 이벤트 메시지를 `msgCh`에 전달한다

#### REQ-NASA-001-04-05 (Ubiquitous) 디바이스 목록 조회

`ListDevices()` 메서드는 **항상** 현재 등록된 모든 디바이스의 목록을 `[]NASADevice` 형태로 반환해야 한다. `sync.RWMutex`로 동시성을 보호한다.

#### REQ-NASA-001-04-06 (Ubiquitous) 디바이스 상태 조회

`GetDeviceState(addr NASAAddress)` 메서드는 **항상** 지정된 3바이트 주소의 디바이스 현재 상태를 `*NASADeviceState` 형태로 반환해야 한다. 미등록 주소인 경우 `ErrDeviceNotFound` 에러를 반환한다.

#### REQ-NASA-001-04-14 (Ubiquitous) device_id로 디바이스 조회

`GetDeviceByID(deviceID string)` 메서드는 **항상** 등록된 device_id로 디바이스를 조회하여 `*NASADevice`를 반환해야 한다. 미등록 device_id인 경우 `ErrDeviceIDNotFound` 에러를 반환한다. `deviceIDs` 맵을 통해 O(1) 조회를 수행한다.

#### REQ-NASA-001-04-15 (Ubiquitous) NASADeviceAdapter 프로토콜 추상화 (v1.7.0)

`NASADeviceAdapter` (또는 디바이스 어댑터 레이어)는 **항상** 다음 프로토콜 추상화 필드를 지원해야 한다:


| 필드                | 타입                 | 기본값      | 설명                                                                      |
| ----------------- | ------------------ | -------- | ----------------------------------------------------------------------- |
| `Protocol`        | `string`           | `"nasa"` | 프로토콜 이름 오버라이드. `"nasa"`, `"lgap"` 등 프로토콜 식별자. 상태/이벤트 JSON의 `protocol` 필드에 반영 |
| `ExtraProperties` | `map[string]any`   | `nil`    | 프로토콜별 확장 상태 속성. State() 출력에 병합되어 프로토콜별 추가 정보를 제공                        |
| `DeviceSource`    | `string`           | `""`     | 디바이스 출처 추적 (`"config"`, `"auto"` 등). `Source()` 메서드로 접근                 |


**Protocol 필드:**

- 기본값은 `"nasa"`이며, 설정에서 오버라이드 가능하다
- 상태 조회, 이벤트 메시지의 JSON 출력에 `protocol` 필드로 포함된다
- 동일한 NASA 프로토콜 기반의 다른 브랜드/프로토콜(예: LGAP)을 구분하는 데 사용된다

**ExtraProperties 필드:**

- `map[string]any` 타입으로 프로토콜별 확장 속성을 저장한다
- `State()` 메서드의 출력에 병합되어 반환된다
- 예: LGAP 프로토콜의 경우 `{"lgap_version": "2.0", "indoor_unit_type": "wall_mounted"}` 등

**Source() 메서드:**

- `DeviceSource` 필드 값을 반환하는 접근자 메서드이다
- NASADevice의 기존 `Source` 필드와 연계되어 디바이스 등록 출처를 프로그래밍 방식으로 조회한다

#### REQ-NASA-001-04-16 (Ubiquitous) Device Configuration 통합 구조체 (v1.7.0)

`agent.DeviceEntry` 통합 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드        | JSON/YAML 키   | 타입       | 필수  | 설명                                           |
| --------- | ------------- | -------- | --- | -------------------------------------------- |
| `Address` | `address`     | `string` | Yes | 디바이스 주소 (컴팩트 헥스 형식: `"200000"`)              |
| `ID`      | `id`          | `string` | No  | 디바이스 식별자 (예: `"living-room"`)                |


**v1.7.0 변경 사항:**

- 기존 `DeviceAddresses []string` + `DeviceIDs map[string]string` 두 개의 분리된 설정 필드를 `Devices []agent.DeviceEntry` 단일 배열로 통합
- `agent.ParseDevices(opts map[string]any) ([]agent.DeviceEntry, error)` 헬퍼 함수로 파싱 로직을 중앙화
- 주소 형식은 **컴팩트 헥스**(`"200000"`)로 표준화 — 기존 공백 구분 형식(`"20 00 00"`)은 하위 호환을 위해 파싱 시 지원하되, 새 설정에서는 컴팩트 헥스를 기본으로 사용

**하위 호환성:**

- `ParseDevices()` 함수는 새 `devices` 형식과 레거시 `device_addresses`/`device_ids` 형식 모두를 파싱할 수 있다
- 레거시 형식이 감지되면 내부적으로 `[]agent.DeviceEntry`로 변환한다

#### REQ-NASA-001-04-08 (Ubiquitous) 수동 등록 — 설정 기반

시스템은 **항상** `devices` 설정으로 지정된 디바이스를 `Init()` 시점에 등록해야 한다:

1. `agent.ParseDevices(opts)`로 설정의 디바이스 목록을 파싱한다 (v1.7.0: `[]agent.DeviceEntry` 반환)
2. 각 엔트리의 `Address`(컴팩트 헥스)를 `ParseNASAAddress`로 파싱한다
3. 주소 프리픽스(`0x10`=실외기, `0x20`=실내기)에 따라 `Type`을 자동 결정한다
4. `NASADevice` 인스턴스를 생성하고 `Source`를 `"config"`로 설정한다
5. 엔트리에 `ID`가 설정되어 있으면 `DeviceID`를 설정하고 `deviceIDs` 맵에 등록한다
6. `Online`을 `false`로 초기화한다 (첫 폴링 응답 시 `true`로 전환)

#### REQ-NASA-001-04-09 (Event-Driven) 수동 등록 — Bridge 런타임 등록

**WHEN** Bridge를 통해 `add_device` 명령이 `Process(data)`로 수신되면 **THEN**:

1. `address` 필드(spaced/compact hex 모두 지원)를 `ParseNASAAddress`로 파싱하여 `NASADevice` 인스턴스를 생성한다
2. `device_id` 필드가 있으면 `DeviceID`를 설정하고 `deviceIDs` 맵에 등록한다. 이미 사용 중인 device_id이면 `ErrDuplicateDeviceID` 에러를 반환한다
3. `Source`를 `"bridge"`로 설정한다
4. 이미 등록된 주소인 경우 `ErrDeviceAlreadyRegistered` 에러를 반환한다
5. 등록 성공 시 `device_registered` 이벤트를 `msgCh`에 전달한다
6. `RegistryPath`가 설정되어 있으면 레지스트리 파일을 갱신한다

**WHEN** Bridge를 통해 `remove_device` 명령이 `Process(data)`로 수신되면 **THEN**:

1. `address` 또는 `device_id` 필드로 대상 디바이스를 식별한다 (둘 다 제공 시 `device_id` 우선)
2. 지정된 디바이스를 `devices` 맵에서 제거하고, `DeviceID`가 있으면 `deviceIDs` 맵에서도 제거한다
3. `Source`가 `"config"`인 디바이스는 제거할 수 **없다** (`ErrConfigDeviceProtected` 에러 반환)
4. 제거 성공 시 `device_unregistered` 이벤트를 `msgCh`에 전달한다
5. `RegistryPath`가 설정되어 있으면 레지스트리 파일을 갱신한다

#### REQ-NASA-001-04-10 (Event-Driven) 자동 등록 — 디바이스 자동 탐색

**WHEN** `AutoDiscovery`가 `true`이고, 트랜스포트에서 미등록 디바이스 주소의 응답이 수신되면 **THEN**:

1. 해당 주소로 `NASADevice` 인스턴스를 자동 생성한다
2. `Source`를 `"auto"`로 설정한다
3. `device_discovered` 이벤트를 `msgCh`에 전달한다 (플로우에서 신규 디바이스 감지 가능)
4. `RegistryPath`가 설정되어 있으면 레지스트리 파일을 갱신한다

**IF** `AutoDiscovery`가 `false`이면 **THEN** 미등록 주소의 응답은 무시하고, 경고 로그만 기록한다.

#### REQ-NASA-001-04-11 (Event-Driven) 디바이스 레지스트리 영속화

**WHEN** `RegistryPath`가 설정되어 있고, 디바이스 목록에 변경이 발생하면 (등록/제거/자동 탐색) **THEN**:

1. `Source`가 `"bridge"` 또는 `"auto"`인 디바이스 목록을 JSON 파일로 저장한다
2. `Source`가 `"config"`인 디바이스는 저장에서 제외한다 (설정 파일이 원본)

**WHEN** `Init()` 시 `RegistryPath` 파일이 존재하면 **THEN**:

1. 저장된 디바이스 목록을 로드하여 `devices` 맵에 추가한다
2. 설정 기반 디바이스와 주소가 충돌하면 설정 기반이 우선한다
3. 로드된 디바이스의 `Source`는 원본 값(`"bridge"` 또는 `"auto"`)을 유지한다

#### REQ-NASA-001-04-13 (Event-Driven) 실외기(ODU) 상태 텔레메트리 디코딩 (v1.20.0)

실외기는 자기 상태를 C0 14 Notification 으로 버스에 브로드캐스트하므로, 외부제어기는 별도 요청 없이 SA=`10 xx 00`(실외기) 프레임의 Message Set 을 수동 수집하여 상태를 파악한다.

**WHEN** 디바이스 타입이 `HVACR.ODU`로 판별되면 **THEN** `NasaDevice.Outdoor`(`*OutdoorState`)를 초기화한다 (실내기 `State`와 병렬 구조이며, ODU 는 `State=nil`). 초기화 지점은 config 로드·`add_device`·자동 탐색 3곳이다.

**WHEN** `HVACR.ODU` 디바이스로부터 Message Set 을 수신하면 **THEN** 다음 대역을 문서화된 인덱스 레지스트리로 디코딩한다:

1. `0x82xx`(VAR 2B, 온도): signed `raw/10 = °C` (예: `0x8204 outdoor_temperature`, `0x8280 compressor_discharge_temperature`)
2. `0x80xx`(ENUM 1B): raw 정수 (예: `0x8001 out_operation_odu_mode`, `0x8010~0x8012 out_load_comp1~3`)
3. `0x84xx`·`0x24FC`(LVAR 4B): raw 정수 (예: `0x8413 wattmeter_1min_sum`, `0x24FC out_sensor_voltage`)
4. 관측 기반 emit — 한 번이라도 수신된 필드만 상태 payload 에 포함한다. 미등록 인덱스도 `RawMessageSets`에는 보존한다.

**WHEN** 이산 필드(운전상태·압축기 On/Off·4-way·제상단계·에러코드)의 값이 새로 관측되거나 변경되면 **THEN** 즉시 `device_state_changed`(trigger=`change`)를 emit 한다.

**IF** 변경된 필드가 연속 센서(온도·압축기 주파수·전류·전력)뿐이면 **THEN** 즉시 emit 하지 않고 `report_interval` 정기 보고(trigger=`report`) 주기에만 최신 값을 싣는다 (매 프레임 변동으로 인한 emit 폭주 방지).

**IF** raw 정수 필드의 값이 폭 전체 `0xFF` 센티넬(ENUM `0xFF` / VAR `0xFFFF` / LVAR `0xFFFFFFFF`, 미장착 압축기·센서)이면 **THEN** 무효로 간주하여 상태 payload 에서 생략한다 (관측 안 됨과 동일, raw 는 보존). 온도(signed)는 `0xFFFF`(-0.1°C)가 정상 저온일 수 있어 센티넬 마스킹에서 제외한다.

**주의**: 실외기 인덱스 매핑은 esphome_samsung_hvac_bus / pysamsungnasa 리버스 엔지니어링 결과이며, 모델별로 존재 여부·의미·스케일이 다를 수 있다. 온도는 `/10`으로 확인되었으나 전류·전압·전력은 raw 로만 노출되며 단위·계수는 실측 검증이 필요하다. 의미가 1:1 일치하는 필드(`outdoor_temperature`, `compressor_discharge_temperature`, `error_code`)는 LG HVACR emit 키와 통일한다.

---

### Module 5: Control Commands (제어 명령)

#### REQ-NASA-001-05-01 (Ubiquitous) NASACommand 타입

NASACommand는 **항상** 다음 제어 명령을 지원해야 한다:


| 명령    | 메서드                                                    | 설명       |
| ----- | ------------------------------------------------------ | -------- |
| 전원 제어 | `SetPower(addr NASAAddress, on bool) error`            | 전원 켜기/끄기 |
| 모드 설정 | `SetMode(addr NASAAddress, mode string) error`         | 운전 모드 변경 |
| 온도 설정 | `SetTemperature(addr NASAAddress, temp float32) error` | 설정 온도 변경 |
| 풍량 설정 | `SetFanSpeed(addr NASAAddress, speed string) error`    | 풍량 변경    |


**내부 인코딩 규칙:**

모든 제어 명령은 C013(Normal Control) 명령으로 인코딩되며, SA=`6A EE FF`(외부제어기), DA=대상 실내기 주소로 전송된다.

**모드 매핑 (string <-> byte):**


| 문자열      | 바이트    | 설명  |
| -------- | ------ | --- |
| `"auto"` | `0x00` | 자동  |
| `"cool"` | `0x01` | 냉방  |
| `"dry"`  | `0x02` | 제습  |
| `"fan"`  | `0x03` | 송풍  |
| `"heat"` | `0x04` | 난방  |


**풍량 매핑 (string <-> byte):**


| 문자열        | 바이트    | 설명  |
| ---------- | ------ | --- |
| `"auto"`   | `0x00` | 자동  |
| `"low"`    | `0x01` | 미풍  |
| `"medium"` | `0x02` | 약풍  |
| `"high"`   | `0x03` | 강풍  |


**온도 인코딩:**

- 입력: `float32` (섭씨 온도, 16.0~30.0)
- 인코딩: `uint16(temp * 10)`, Big-Endian 2 bytes
- 예시: 18.0C -> `0x00B4`, 24.0C -> `0x00F0`, 30.0C -> `0x012C`
- Message Set: Index=`0x4201`, Value=인코딩된 2 bytes

#### REQ-NASA-001-05-02 (Event-Driven) Process 메서드를 통한 제어

**WHEN** Bridge 노드에서 `Process(data []byte)` 가 호출되면 **THEN**:

1. JSON 데이터를 파싱하여 명령 유형(`command`)과 파라미터를 추출한다
2. 대상 디바이스를 다음 두 가지 방식으로 지정할 수 있다 (둘 다 제공 시 `device_id` 우선):
  - `address` 필드: 3바이트 주소 문자열(`"20 00 01"` 또는 `"200001"`)을 `ParseNASAAddress`로 파싱
  - `device_id` 필드: 등록된 device_id를 `deviceIDs` 맵에서 `NASAAddress`로 변환. 미등록 device_id인 경우 `ErrDeviceIDNotFound` 반환
3. 명령 유형에 따라 분기한다:
  - **제어 명령** (`set_power`, `set_mode`, `target_temperature`, `set_fan_speed`):
  1. 파라미터를 `NASAMessageSet` 목록으로 변환 (예: power=true → Index=`0x4000`, Value=`[0x01]`)
  2. C013(Normal Control) 프레임으로 인코딩 (SA=외부제어기, DA=대상 실내기)
  3. `seqNum`을 증가시키고 프레임에 포함
  4. 트랜스포트로 전송
  5. C016(Control Response) 응답 수신 대기
  6. 결과 반환
    상태 조회** (`get_state`): 캐시된 디바이스 상태를 즉시 반환 (트랜스포트 통신 없음)
    전체 상태 조회** (`get_all_states`): 모든 디바이스의 캐시된 상태를 반환
    디바이스 등록** (`add_device`): 런타임 디바이스 추가 (REQ-NASA-001-04-09)
    디바이스 제거** (`remove_device`): 런타임 디바이스 제거 (REQ-NASA-001-04-09)
    디바이스 목록** (`list_devices`): 등록된 전체 디바이스 목록 반환 (Source 포함)

**제어 명령 예시 — address 지정:**

```json
{
  "command": "set_power",
  "address": "20 00 01",
  "params": { "power": true }
}
```

**제어 명령 예시 — compact hex address:**

```json
{
  "command": "set_power",
  "address": "200001",
  "params": { "power": true }
}
```

**제어 명령 예시 — device_id 지정:**

```json
{
  "command": "set_power",
  "device_id": "living-room",
  "params": { "power": true }
}
```

**복합 제어 명령 예시 (여러 설정 동시 변경):**

```json
{
  "command": "set_multiple",
  "device_id": "bedroom-1",
  "params": {
    "power": true,
    "mode": "cool",
    "fan_speed": "high",
    "target_temp": 24.0
  }
}
```

**상태 조회 명령 예시 (address 또는 device_id 사용 가능):**

```json
{
  "command": "get_state",
  "device_id": "living-room"
}
```

```json
{
  "command": "get_state",
  "address": "200001"
}
```

**전체 상태 조회 명령:**

```json
{
  "command": "get_all_states"
}
```

**제어 명령 응답:**

```json
{
  "status": "ok",
  "address": "20 00 01",
  "device_id": "living-room",
  "result": { "power": true }
}
```

**상태 조회 응답:**

```json
{
  "status": "ok",
  "address": "20 00 01",
  "device_id": "living-room",
  "device_type": "indoor",
  "state": {
    "power": true,
    "mode": "cool",
    "target_temp": 24.0,
    "current_temp": 26.5,
    "fan_speed": "auto",
    "swing_vertical": false,
    "filter_alarm": false,
    "error_code": 0
  },
  "online": true,
  "last_seen": "2026-02-24T10:30:00Z"
}
```

**전체 상태 조회 응답:**

```json
{
  "status": "ok",
  "devices": [
    { "address": "20 00 00", "device_id": "living-room", "device_type": "indoor", "online": true, "state": { ... } },
    { "address": "20 00 01", "device_id": "bedroom-1", "device_type": "indoor", "online": false, "state": null },
    { "address": "10 00 00", "device_id": "", "device_type": "outdoor", "online": true, "ready": true }
  ]
}
```

**디바이스 등록 명령 (address + 선택적 device_id):**

```json
{
  "command": "add_device",
  "address": "200100",
  "device_id": "room-3",
  "device_type": "indoor"
}
```

**디바이스 제거 명령:**

```json
{
  "command": "remove_device",
  "address": "20 01 00"
}
```

**디바이스 목록 조회 명령:**

```json
{
  "command": "list_devices"
}
```

**디바이스 목록 응답:**

```json
{
  "status": "ok",
  "devices": [
    { "address": "20 00 00", "device_id": "living-room", "device_type": "indoor", "online": true, "source": "config" },
    { "address": "20 01 00", "device_id": "room-3", "device_type": "indoor", "online": false, "source": "bridge" },
    { "address": "10 00 00", "device_id": "", "device_type": "outdoor", "online": true, "source": "discovery" }
  ]
}
```

#### REQ-NASA-001-05-03 (Unwanted) 유효하지 않은 모드 값 거부

시스템은 `"cool"`(`0x01`), `"heat"`(`0x04`), `"dry"`(`0x02`), `"fan"`(`0x03`), `"auto"`(`0x00`) 이외의 모드 값을 **수락하지 않아야 한다**. 유효하지 않은 값이 전달되면 `ErrInvalidMode` 에러를 반환한다.

#### REQ-NASA-001-05-04 (Unwanted) 유효하지 않은 풍량 값 거부

시스템은 `"auto"`(`0x00`), `"low"`(`0x01`), `"medium"`(`0x02`), `"high"`(`0x03`) 이외의 풍량 값을 **수락하지 않아야 한다**. `"turbo"` 값은 실제 프로토콜에서 지원되지 않으므로 `ErrInvalidFanSpeed` 에러를 반환한다.

#### REQ-NASA-001-05-05 (Unwanted) 온도 범위 초과 거부

시스템은 16.0~30.0 범위를 벗어나는 온도 값을 **수락하지 않아야 한다**. 범위 초과 시 `ErrTemperatureOutOfRange` 에러를 반환한다.

#### REQ-NASA-001-05-06 (Unwanted) 미등록 디바이스 주소 거부

시스템은 `devices` 맵에 등록되지 않은 디바이스 주소로의 제어 명령을 **수락하지 않아야 한다**. 미등록 주소인 경우 `ErrDeviceNotFound` 에러를 반환한다.

#### REQ-NASA-001-05-07 (Ubiquitous) 부저 자동 제어 (v1.4.0)

`sendControlCommand` 메서드는 **항상** 제어 명령 전송 시 MsgBuzzer(0x4050) MessageSet을 자동으로 관리해야 한다:

1. 전달받은 `sets` 목록에 MsgBuzzer(0x4050)가 이미 포함되어 있으면, 그대로 사용한다 (명시적 지정 우선)
2. MsgBuzzer가 없으면:
   - `NASAConfig.BuzzerOnControl`이 `true`이면 → `{Index: MsgBuzzer, Value: []byte{0x00}}` (부저 On) 추가
   - `NASAConfig.BuzzerOnControl`이 `false`(기본값)이면 → `{Index: MsgBuzzer, Value: []byte{0x01}}` (부저 Off, 억제) 추가

에어컨의 기본 동작이 부저 울림이므로, 명시적으로 부저 상태를 설정하여 예측 가능한 동작을 보장한다.

---

### Module 6: Bridge Integration (Bridge 연동)

#### REQ-NASA-001-06-01 (Ubiquitous) Bridge 메시지 포맷

Bridge를 통해 플로우와 교환되는 메시지는 **항상** JSON 포맷이어야 한다.

**상태 변경 알림 (에이전트 -> 플로우)** — 개별 디바이스 상태 변경 시 즉시 전송:

```json
{
  "type": "device_state_changed",
  "address": "20 00 01",
  "device_id": "bedroom-1",
  "device_type": "indoor",
  "online": true,
  "state": {
    "power": true,
    "mode": "cool",
    "target_temp": 24.0,
    "current_temp": 26.5,
    "fan_speed": "auto",
    "swing_vertical": false,
    "filter_alarm": false,
    "error_code": 0
  },
  "changed_fields": ["current_temp"],
  "timestamp": "2026-02-24T10:30:00Z"
}
```

**주기적 상태 보고 (에이전트 -> 플로우)** — NotifyInterval마다 모든 온라인 디바이스 일괄 보고:

```json
{
  "type": "device_state_report",
  "devices": [
    {
      "address": "20 00 00",
      "device_id": "living-room",
      "device_type": "indoor",
      "online": true,
      "state": { "power": true, "mode": "cool", "target_temp": 24.0, "current_temp": 26.5, "fan_speed": "auto", "error_code": 0 }
    },
    {
      "address": "20 00 01",
      "device_id": "bedroom-1",
      "device_type": "indoor",
      "online": true,
      "state": { "power": false, "mode": "auto", "target_temp": 22.0, "current_temp": 24.0, "fan_speed": "auto", "error_code": 0 }
    }
  ],
  "timestamp": "2026-02-24T10:30:00Z"
}
```

**이벤트 메시지 (에이전트 -> 플로우)** — 디바이스 이벤트:

```json
{
  "type": "device_offline",
  "address": "20 00 01",
  "device_id": "bedroom-1",
  "device_type": "indoor",
  "timestamp": "2026-02-24T10:30:00Z"
}
```

**디바이스 등록 이벤트 (에이전트 -> 플로우)** — 등록/제거/자동 탐색:

```json
{
  "type": "device_registered",
  "address": "20 01 00",
  "device_id": "room-3",
  "device_type": "indoor",
  "source": "bridge",
  "timestamp": "2026-02-24T10:31:00Z"
}
```

```json
{
  "type": "device_discovered",
  "address": "10 01 00",
  "device_type": "outdoor",
  "source": "discovery",
  "timestamp": "2026-02-24T10:32:00Z"
}
```

```json
{
  "type": "device_unregistered",
  "address": "20 01 00",
  "timestamp": "2026-02-24T10:33:00Z"
}
```

#### REQ-NASA-001-06-02 (Event-Driven) 상태 알림 (3가지 트리거)

플로우로의 상태 알림은 다음 3가지 트리거로 동작한다:

**트리거 1 — 상태 변경 알림 (즉시)**
**WHEN** 디바이스 상태가 이전 폴링 대비 변경되면 **THEN** `device_state_changed` 타입의 JSON 메시지를 즉시 `msgCh` 채널에 전달한다.

**트리거 2 — 주기적 상태 보고**
**WHEN** `NotifyInterval` 주기가 도래하면 **THEN** 모든 온라인 디바이스의 현재 상태를 `device_state_report` 타입의 JSON 메시지로 `msgCh` 채널에 전달한다. 변경 여부와 무관하게 보고한다.

**트리거 3 — 온디맨드 상태 조회 (플로우 요청)**
**WHEN** Bridge를 통해 `get_state` 명령이 `Process(data)`로 수신되면 **THEN** 해당 디바이스(또는 전체)의 현재 캐시된 상태를 즉시 JSON으로 반환한다.

#### REQ-NASA-001-06-03 (Ubiquitous) ReceiveMessage 채널 기반 구현

`ReceiveMessage(ctx context.Context)` 메서드는 **항상** `msgCh` 채널과 `ctx.Done()` 채널을 `select`로 대기하여, 컨텍스트 취소 시 즉시 반환하고 메시지 수신 시 JSON 바이트를 반환해야 한다.

#### REQ-NASA-001-06-04 (Ubiquitous) CommandPollAdapter 인터페이스 (v1.1.0)

`CommandPollAdapter` 인터페이스(`internal/node/bridge_adapter.go`)는 **항상** 다음 메서드를 제공해야 한다:


| 메서드                   | 시그니처                                                            | 설명                                    |
| --------------------- | --------------------------------------------------------------- | ------------------------------------- |
| `PollCommand`         | `PollCommand() []byte`                                          | `ag.Process()`에 전달할 JSON 명령 바이트를 반환한다 |
| `AssemblePollMessage` | `AssemblePollMessage(response []byte) (message.Message, error)` | `Process()` 응답을 플로우 `Message`로 변환한다   |


`PollableAdapter`(레지스터 단위 읽기 기반, Modbus 등)와 대비되는 JSON 명령 기반 폴링 인터페이스이다. `BridgeNode.Init()`에서 어댑터가 `CommandPollAdapter`를 구현하는지 감지하여, 구현 시 `startCommandPollLoop()`을 시작한다.

#### REQ-NASA-001-06-05 (Ubiquitous) NASAAdapter 브릿지 어댑터 (v1.1.0)

`NASAAdapter`(`internal/node/adapter/samsung_nasa.go`)는 **항상** `BridgeAdapter`와 `CommandPollAdapter` 인터페이스를 모두 구현해야 한다. 어댑터 레지스트리에 `"samsung_hvacr01"` 키로 등록된다.

**BridgeAdapter 구현:**


| 메서드                           | 설명                                                                                                                                                         |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Validate(config)`            | 항상 성공 반환 (추가 검증 없음)                                                                                                                                        |
| `DefaultConfig()`             | 빈 `BridgeConfig` 반환                                                                                                                                        |
| `TransformToFlow(data, meta)` | JSON 이벤트 데이터를 플로우 메시지로 변환. JSON 파싱 성공 시 각 필드를 `Payload`에 설정하고 `node_source=event` 메타데이터 추가. JSON 파싱 실패 시 원시 데이터를 `raw` 필드에 저장하고 `format=raw` 메타데이터 추가 |
| `TransformToAgent(msg)`       | 플로우 메시지의 Payload를 JSON 바이트로 직렬화하여 에이전트 명령으로 변환. `AgentMeta.AgentType`을 `"samsung_hvacr01"`로 설정                                                                |
| `HandleControl(msg)`          | 제어 메시지 처리 (현재 항상 nil 반환)                                                                                                                                   |


**CommandPollAdapter 구현:**


| 메서드                             | 설명                                                                      |
| ------------------------------- | ----------------------------------------------------------------------- |
| `PollCommand()`                 | `{"command": "get_all_states"}` JSON 바이트를 반환                            |
| `AssemblePollMessage(response)` | JSON 응답을 플로우 메시지로 변환. 각 필드를 `Payload`에 설정하고 `node_source=poll` 메타데이터 추가 |


#### REQ-NASA-001-06-06 (Event-Driven) startCommandPollLoop 폴링 루프 (v1.1.0)

**WHEN** `BridgeNode.Init()`에서 어댑터가 `CommandPollAdapter`를 구현하는 것이 감지되면 **THEN**:

1. 설정된 폴링 간격(`polling_interval_ms` 또는 기본값 5초)으로 `startCommandPollLoop()`을 시작한다
2. 매 주기마다 `poller.PollCommand()`로 명령 바이트를 얻고 `ag.Process()`로 전송한다
3. `Process()` 응답을 `poller.AssemblePollMessage()`로 플로우 메시지로 변환한다
4. 변환된 메시지를 플로우의 다음 노드로 전달한다
5. 컨텍스트 취소 시 루프를 종료한다

**WHEN** 에이전트가 `agent.MessageReceiver`를 구현하면 **THEN** 커맨드 폴링과 병행하여 `startReceiveLoop()`도 시작하여 비동기 이벤트도 수신한다.

---

### Module 7: Error Handling (에러 처리)

#### REQ-NASA-001-07-01 (Ubiquitous) 센티널 에러 정의

다음 센티널 에러가 **항상** 정의되어야 한다:


| 에러 변수                        | 설명                                |
| ---------------------------- | --------------------------------- |
| `ErrInvalidTransportType`    | 유효하지 않은 트랜스포트 유형                  |
| `ErrSerialPortRequired`      | 시리얼 포트 경로 미설정                     |
| `ErrTCPAddressRequired`      | TCP 주소 미설정                        |
| `ErrDeviceNotFound`          | 미등록 디바이스 주소                       |
| `ErrInvalidMode`             | 유효하지 않은 운전 모드                     |
| `ErrInvalidFanSpeed`         | 유효하지 않은 풍량 값                      |
| `ErrTemperatureOutOfRange`   | 온도 범위 초과 (16.0~30.0)              |
| `ErrChecksumMismatch`        | CRC16-CCITT 체크섬 불일치               |
| `ErrProtocolParseFailed`     | 프로토콜 파싱 실패                        |
| `ErrInvalidFrameSTX`         | 잘못된 STX 바이트 (0x32 아님)             |
| `ErrInvalidFrameETX`         | 잘못된 ETX 바이트 (0x34 아님)             |
| `ErrInvalidFrameLength`      | 프레임 길이 불일치 (LEN 필드와 실제 크기)        |
| `ErrInvalidAddress`          | 유효하지 않은 3바이트 NASA 주소 형식           |
| `ErrInvalidMessageSetIndex`  | 알 수 없는 Message Set Index 니블 값     |
| `ErrDeviceOffline`           | 오프라인 디바이스에 대한 명령                  |
| `ErrDeviceNotReady`          | 통신 준비가 완료되지 않은 디바이스 (Ready=false) |
| `ErrTransportNotConnected`   | 트랜스포트 미연결 상태                      |
| `ErrInvalidCommand`          | 유효하지 않은 명령 형식                     |
| `ErrDeviceAlreadyRegistered` | 이미 등록된 디바이스 주소                    |
| `ErrConfigDeviceProtected`   | 설정 기반 디바이스는 제거 불가                 |
| `ErrSequenceNumOverflow`     | 시퀀스 번호 순환 (0xFF 이후 리셋)            |
| `ErrDeviceIDNotFound`        | 등록되지 않은 device_id                 |
| `ErrDuplicateDeviceID`       | 이미 사용 중인 device_id                |


모든 에러는 `errors.New()`로 정의하며, `errors.Is()`로 비교 가능해야 한다.

#### REQ-NASA-001-07-02 (Unwanted) 오프라인 디바이스 명령 거부

시스템은 `Online` 상태가 `false`인 디바이스에 대한 제어 명령을 **수락하지 않아야 한다**. `ErrDeviceOffline` 에러를 반환한다.

#### REQ-NASA-001-07-03 (Ubiquitous) 재연결 이벤트 메시지 (v1.2.0)

NASAAgent는 **항상** 다음 재연결 관련 이벤트를 `msgCh`를 통해 전달해야 한다:


| 이벤트 타입                   | 발생 시점          | 추가 필드                                                                          |
| ------------------------ | -------------- | ------------------------------------------------------------------------------ |
| `transport_disconnected` | 연결 끊김이 감지되었을 때 | -                                                                              |
| `transport_reconnecting` | 첫 번째 재연결 시도 시  | -                                                                              |
| `transport_reconnected`  | 재연결 성공 시       | `attempt_count` (int): 재연결 시도 횟수, `downtime_seconds` (float64): 연결 끊김 지속 시간(초) |


**이벤트 메시지 JSON 예시:**

```json
{
  "type": "transport_disconnected",
  "timestamp": "2026-03-12T10:30:00Z"
}
```

```json
{
  "type": "transport_reconnected",
  "attempt_count": 3,
  "downtime_seconds": 35.2,
  "timestamp": "2026-03-12T10:30:35Z"
}
```

---

### Module 8: TypeRegistry Registration (타입 등록)

#### REQ-NASA-001-08-01 (Ubiquitous) 에이전트 타입 등록 함수

`RegisterSamsungHvacr01Types(registry *agent.DefaultManager)` 함수는 **항상** `"samsung_hvacr01"` 타입을 에이전트 팩토리에 등록해야 한다.

#### REQ-NASA-001-08-02 (Event-Driven) 팩토리를 통한 생성

**WHEN** `registry.CreateAgent("samsung_hvacr01", config)` 호출 시 **THEN** 설정을 파싱하고 `NASAAgent` 인스턴스를 생성하여 반환한다.

#### REQ-NASA-001-08-03 (Ubiquitous) 어댑터 레지스트리 등록 (v1.1.0)

`NASAAdapter`는 **항상** 어댑터 레지스트리(`internal/node/adapter/register.go`)에 `"samsung_hvacr01"` 키로 등록되어야 한다. `BridgeNode`가 에이전트 타입에 매칭되는 어댑터를 자동으로 로드한다.

---

### Module 9: NASA Nodes (NASA 프로토콜 전용 노드) (v1.3.0)

> ModbusNode 패턴을 따른다. AgentResolver -> AgentTransport -> AgentAccessor 파이프라인으로 `*samsung.NASAAgent`를 타입 체크한다. `callAgentProcess()`로 JSON 명령을 전달하고, 채널 기반 타임아웃을 사용한다.

#### REQ-NASA-001-09-01 (Ubiquitous) SamsungHvacr01NodeConfig 설정 구조체 — v1.18.26 REVISED (Breaking)

`SamsungHvacr01NodeConfig` 구조체는 **항상** 다음 필드를 포함해야 한다 (v1.18.26 status 노드 통일):


| 필드                | JSON 키               | 타입       | 기본값     | 필수  | 설명                                                                                                                                       |
| ----------------- | -------------------- | -------- | ------- | --- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `AgentRef`        | `agent_ref`          | `string` | -       | Yes | 대상 Samsung HVACR-01 Agent 이름/ID                                                                                                          |
| `InactivityTimeout` | `inactivity_timeout` | `string` | `"90s"` | No  | **v1.18.26**: receiveLoop 의 무수신 fallback 임계. 이 기간 동안 `FrameNotifyCh` 신호 미수신 시 `request_state` 명령 전송. 최소 5s.                              |
| `Timeout`         | `timeout`            | `string` | `"5s"`  | No  | Agent Process() 호출 타임아웃                                                                                                                  |
| `BatchSize`       | `batch_size`         | `int`    | 32      | No  | 벌크 수신 수량                                                                                                                                 |
| `OmitStateWhenOff` | `omit_state_when_off` | `bool` | `false` | No  | v1.18.0: `payload.power=false` 시 신뢰할 수 없는 상태 필드 제거                                                                                       |
| `GroupID`         | `group_id`           | `string` | `""`    | No  | **v1.18.26 advanced**: NASA addr byte 1 (외기 인덱스 hex, `"00"`–`"0F"`). 빈 값이면 모든 group.                                                       |
| `UnitID`          | `unit_id`            | `string` | `""`    | No  | **v1.18.26 advanced**: NASA addr byte 2 (`"00"`–`"3F"` indoor; outdoor 는 group_id 와 동일). dotted/compact 형식 (`"10.0F.00"` 또는 `"100F00"`) 모두 인식. 빈 값이면 모든 unit. |
| `EmitMetadata`    | `emit_metadata`      | `MetadataEmitOptions` | -    | No  | v1.18.8 metadata 옵션 (default off; `device_id` 만 always emit). **v1.18.26**: `emit_unit_id` / `emit_slot_num` 옵션 제거.                       |

**v1.18.26 제거된 필드 (Breaking)**:
- ~~`DeviceID`~~ (`device_id`): node config 에서 제거. 단일 디바이스 조회는 payload-level override (`$.payload.device_id` 또는 `$.payload.unit_id`) 로 가능.
- ~~`DeviceAddress`~~ (`device_address`): node config 에서 제거. 어드레싱은 `group_id` + `unit_id` 로 통일.
- ~~`PollInterval`~~ (`poll_interval`): ticker 기반 폴링 제거 (inactivity 모델로 교체).
- ~~`IncludeRaw`~~ (`include_raw`): 노드 레벨 raw 토글 제거 (agent 의 `include_raw_hex` 만 사용 — v1.19.0 부터 `include_raw_message_sets` 에서 rename).

`parseSamsungHvacr01NodeConfig(config map[string]any)` 함수로 노드 설정 맵에서 파싱한다. 두 어드레싱 필드 모두 비어있으면 모든 디바이스 frame 처리 + broadcast `request_state`.

#### REQ-NASA-001-09-02 (Ubiquitous) NASAStatusNode 구조체

NASAStatusNode 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드           | 타입                     | 설명                      |
| ------------ | ---------------------- | ----------------------- |
| `*BaseNode`  | 임베딩                    | 기본 노드 기능 (생명주기, 포트, 설정) |
| `nasaConfig` | `NASANodeConfig`       | NASA 노드 전용 설정           |
| `resolver`   | `AgentResolver`        | 에이전트 리졸버                |
| `transport`  | `AgentTransport`       | 에이전트 트랜스포트              |
| `agent`      | `agent.Agent`          | 원본 NASAAgent 객체         |
| `timeout`    | `time.Duration`        | Process 호출 타임아웃         |
| `sourceCh`   | `chan message.Message` | SourceNode 폴링 채널        |
| `stopCh`     | `chan struct{}`        | 폴링 종료 시그널               |
| `mu`         | `sync.RWMutex`         | 설정 보호 뮤텍스               |


#### REQ-NASA-001-09-03 (Event-Driven) NASAStatusNode Configure

**WHEN** `Configure(config)` 호출 시 **THEN**:

1. `BaseNode.Configure(config)`를 호출한다
2. `parseNASANodeConfig(config)`로 NASA 전용 설정을 파싱한다
3. `agent_ref`가 비어있으면 `ErrNASAMissingAgentRef` 에러를 반환한다
4. `timeout` 설정을 파싱한다 (기본값 `5s`)

#### REQ-NASA-001-09-04 (Event-Driven) NASAStatusNode Init

**WHEN** `Init(ctx)` 호출 시 **THEN**:

1. `BaseNode.TransitionTo(StateInitializing)`을 호출한다
2. `resolver`가 nil이면 `ErrNASANoResolver` 에러를 반환한다 (구성 오류 — hard-fail 유지)
3. `resolver.ResolveAgent(ctx, ref)`로 에이전트를 resolve한다
4. `transport.(AgentAccessor).UnderlyingAgent()`로 원본 Agent를 획득한다
5. `switch agent.(type)` — `*samsung.NASAAgent` 타입이면 `n.agent`에 저장한다
6. 그 외 타입이면 `ErrNASAAgentNotNASA` 에러를 반환한다
7. `nasaConfig.PollInterval`이 유효하면 `sourceCh` 채널 생성 및 폴링 고루틴을 시작한다
8. `BaseNode.TransitionTo(StateRunning)`을 호출한다

**v1.9.0 변경 — Init-tolerance (deferred connection)**: 3단계에서 에이전트를 찾을 수 없는 경우(disabled 또는 미등록), `Init()` 은 **hard-fail 하지 않는다**. 대신:

- 경고(WARNING) 로그를 남긴다 (`agent_ref`, 노드 ID 포함)
- 에이전트 연결을 보류(deferred)한 채 `StateRunning` 으로 전이한다
- 이후 해당 에이전트가 활성화되면 SPEC-ENGINE-001 `ReinitNodesForAgent` 경로를 통해 자동 재초기화·재연결된다

단, **2단계의 `resolver` nil (구성 오류)** 와 **6단계의 타입 불일치** 는 deferred connection 으로 회복 불가능하므로 기존대로 hard-fail(에러 반환)한다. 본 변경은 NASAControlNode(REQ-NASA-001-09-09), NASANode(REQ-NASA-001-09-13)에도 동일하게 적용된다. 관련: SPEC-AGENT-005 v1.1.0, SPEC-ENGINE-001 v1.3.0 Module 8.

#### REQ-NASA-001-09-05 (Event-Driven) SamsungHvacr01StatusNode Process — v1.18.26 REVISED

**WHEN** `Process(ctx, msg)` 호출 시 **THEN**:

1. `msg.Payload()`에서 런타임 오버라이드를 적용한다 (`device_id`, `unit_id`, `timeout` 필드). `unit_id` 가 명시되면 `device_id` 보다 우선.
2. 단일 디바이스 조회는 payload override 의 `device_id` 또는 `unit_id` 로 가능. 미지정 시 `cfg.UnitID` (advanced) 사용, 그것도 빈 값이면 broadcast (`get_all`).
3. `callAgentProcess(ctx, cmdBytes)`로 Agent에 명령을 전달한다.
4. 응답 JSON을 파싱하여 출력 메시지의 Payload에 설정한다.
5. 출력 메시지에 메타데이터 `node_source=node`, `node_type=samsung_hvacr01_status`를 설정한다. **v1.18.26**: `unit_id` / `slot_num` metadata 키는 더 이상 emit 되지 않는다.

#### REQ-NASA-001-09-06 (Event-Driven) SamsungHvacr01StatusNode SourceNode 동작 — v1.18.26 REVISED (Breaking)

**REVISED from ticker-based polling to inactivity-fallback model (LG ICP-01 통일)**.

**WHEN** `Init()` 호출 시 **THEN**:

1. `sourceCh` 채널(버퍼 크기 1)을 생성한다.
2. `receiveLoop` 고루틴을 시작하여 에이전트의 `FrameNotifyCh` 신호를 구독한다.
3. 새 frame 신호 수신 시 `drainNewFrames` 로 ring buffer 의 신규 frame 만 emit (어드레싱 필터 적용). 어드레싱이 설정되지 않은 경우 모든 frame 처리.
4. `inactivity_timeout` (기본 `"90s"`) 동안 frame 신호가 없으면 `requestStateRefresh` 호출 — 에이전트의 `request_state` 명령으로 `cfg.GroupID` / `cfg.UnitID` 타겟 (또는 broadcast) 의 강제 상태 emit 유발.
5. `stopCh` 신호 수신 시 `receiveLoop` 종료.

`SourceCh() <-chan message.Message` 메서드를 구현하여 `SourceNode` 인터페이스를 충족한다.

**제거된 동작 (Breaking)**: ~~`PollInterval` 주기마다 `get_state` / `get_all_states` 명령 실행~~ → inactivity 모델로 대체. ticker 기반 폴링 / 콘텐츠 dedup / `samsungHvacr01StateHash` 모두 제거됨.

#### REQ-NASA-001-09-07 (Ubiquitous) NASAControlNode 구조체

NASAControlNode 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드           | 타입               | 설명              |
| ------------ | ---------------- | --------------- |
| `*BaseNode`  | 임베딩              | 기본 노드 기능        |
| `nasaConfig` | `NASANodeConfig` | NASA 노드 전용 설정   |
| `resolver`   | `AgentResolver`  | 에이전트 리졸버        |
| `transport`  | `AgentTransport` | 에이전트 트랜스포트      |
| `agent`      | `agent.Agent`    | 원본 NASAAgent 객체 |
| `timeout`    | `time.Duration`  | Process 호출 타임아웃 |
| `mu`         | `sync.RWMutex`   | 설정 보호 뮤텍스       |


NASAControlNode는 SourceNode 인터페이스를 구현하지 **않는다** (쓰기 전용 노드).

#### REQ-NASA-001-09-08 (Event-Driven) NASAControlNode Configure

**WHEN** `Configure(config)` 호출 시 **THEN**:

1. `BaseNode.Configure(config)`를 호출한다
2. `parseNASANodeConfig(config)`로 NASA 전용 설정을 파싱한다
3. `agent_ref`가 비어있으면 `ErrNASAMissingAgentRef` 에러를 반환한다

#### REQ-NASA-001-09-09 (Event-Driven) NASAControlNode Init

**WHEN** `Init(ctx)` 호출 시 **THEN**:
1~6 단계는 NASAStatusNode.Init()과 동일하다 (REQ-NASA-001-09-04 참조)
7. `BaseNode.TransitionTo(StateRunning)`을 호출한다

#### REQ-NASA-001-09-10 (Event-Driven) NASAControlNode Process — 직접 명령 형식

**WHEN** `Process(ctx, msg)` 호출 시, msg.Payload에 `command` 키가 존재하면 **THEN**:

1. Payload에서 `command`, `device_address`/`device_id`, `params` 필드를 추출한다
2. Payload의 `device_address`/`device_id`가 없으면 노드 설정의 기본값을 사용한다
3. 지원 명령: `set_power`, `set_mode`, `target_temperature`, `set_fan_speed`, `set_multiple`
4. JSON 명령 바이트를 구성하여 `callAgentProcess(ctx, cmdBytes)`로 전달한다
5. 응답을 출력 메시지 Payload에 설정한다
6. 출력 메시지에 메타데이터 `node_source=node`, `node_type=samsung_hvacr01_control`을 설정한다

#### REQ-NASA-001-09-11 (Event-Driven) NASAControlNode Process — 간소화 형식

**WHEN** `Process(ctx, msg)` 호출 시, msg.Payload에 `command` 키가 없고 제어 키(`power`, `mode`, `temperature`/`target_temp`, `fan_speed`) 중 하나 이상이 존재하면 **THEN**:

1. Payload에서 제어 키들을 추출하여 `params` 맵으로 구성한다
2. `device_address`/`device_id`는 Payload 또는 노드 설정 기본값을 사용한다
3. `set_multiple` 명령으로 자동 변환한다:
  ```json
   {"command": "set_multiple", "device_id": "...", "params": {"power": true, "mode": "cool"}}
  ```
4. `callAgentProcess(ctx, cmdBytes)`로 전달하고 응답을 출력 메시지에 설정한다

#### REQ-NASA-001-09-12 (Ubiquitous) NASANode 복합 노드 구조체

NASANode(복합) 구조체는 **항상** 다음 필드를 포함해야 한다:


| 필드           | 타입                     | 설명                     |
| ------------ | ---------------------- | ---------------------- |
| `*BaseNode`  | 임베딩                    | 기본 노드 기능               |
| `nasaConfig` | `NASANodeConfig`       | NASA 노드 전용 설정          |
| `resolver`   | `AgentResolver`        | 에이전트 리졸버               |
| `transport`  | `AgentTransport`       | 에이전트 트랜스포트             |
| `agent`      | `agent.Agent`          | 원본 NASAAgent 객체        |
| `timeout`    | `time.Duration`        | Process 호출 타임아웃        |
| `sourceCh`   | `chan message.Message` | SourceNode 폴링 채널 (선택적) |
| `stopCh`     | `chan struct{}`        | 폴링 종료 시그널              |
| `mu`         | `sync.RWMutex`         | 설정 보호 뮤텍스              |


NASANode는 SourceNode 인터페이스를 **선택적으로** 구현한다 (`PollInterval > 0`일 때만).

#### REQ-NASA-001-09-13 (Event-Driven) NASANode Configure 및 Init

Configure 및 Init은 NASAStatusNode와 동일한 로직을 따른다 (REQ-NASA-001-09-03, REQ-NASA-001-09-04 참조).

#### REQ-NASA-001-09-14 (Complex) NASANode Process — 자동 감지

**IF** msg.Payload에 제어 키(`power`, `mode`, `temperature`/`target_temp`, `fan_speed`) 또는 `command` 키가 존재하면 **THEN** NASAControlNode의 Process 로직을 수행한다 (REQ-NASA-001-09-10, 09-11 참조).

**IF** msg.Payload에 제어 키도 `command` 키도 없으면 **THEN** NASAStatusNode의 Process 로직을 수행한다 (REQ-NASA-001-09-05 참조).

자동 감지 우선순위:

1. `command` 키 존재 → 직접 명령 형식 (제어)
2. 제어 키(`power`, `mode`, `temperature`, `target_temp`, `fan_speed`) 존재 → 간소화 형식 (제어)
3. 그 외 → 상태 조회

#### REQ-NASA-001-09-15 (Ubiquitous) 센티널 에러 정의 (NASA Nodes)

다음 센티널 에러가 **항상** `internal/node/errors.go`에 정의되어야 한다:


| 에러 변수                    | 설명                                            |
| ------------------------ | --------------------------------------------- |
| `ErrNASAAgentNotNASA`    | resolve된 Agent가 `*samsung.NASAAgent` 타입이 아닐 때 |
| `ErrNASAMissingAgentRef` | `agent_ref` 설정이 없을 때                          |
| `ErrNASANoResolver`      | AgentResolver가 설정되지 않았을 때                     |
| `ErrNASAProcessFailed`   | Agent Process() 호출이 실패했을 때                    |


모든 에러는 기존 ModbusNode 에러 패턴을 따른다: `fmt.Errorf("nasa: %w: ...", ErrInvalidConfig)`.

#### REQ-NASA-001-09-16 (Ubiquitous) 노드 레지스트리 등록

`internal/node/registry.go`의 `registerBuiltins()` 함수에 다음 3개 노드 팩토리가 **항상** 등록되어야 한다:


| 타입명                       | 팩토리 함수               | 카테고리           | 설명                                |
| ------------------------- | -------------------- | -------------- | --------------------------------- |
| `samsung_hvacr01_status`  | `NewNASAStatusNode`  | `"processing"` | Samsung HVACR-01 디바이스 상태 조회       |
| `samsung_hvacr01_control` | `NewNASAControlNode` | `"processing"` | Samsung HVACR-01 디바이스 제어 명령       |
| `samsung_hvacr01`         | `NewNASANode`        | `"processing"` | Samsung HVACR-01 복합 (상태 + 제어 자동 감지) |


등록 후 빌트인 노드 총 수는 15개이다.

#### REQ-NASA-001-09-17 (Ubiquitous) 포트 정의

각 노드의 포트는 **항상** 다음과 같이 정의되어야 한다:

**NASAStatusNode:**

- Input: `in` (트리거 메시지 수신 또는 SourceNode 폴링)
- Output: `out` (상태 조회 결과)
- Error: `error` (에러 메시지)

**NASAControlNode:**

- Input: `in` (제어 명령 수신)
- Output: `out` (제어 결과)
- Error: `error` (에러 메시지)

**NASANode:**

- Input: `in` (상태 조회 트리거 또는 제어 명령 수신)
- Output: `out` (상태 결과 또는 제어 결과)
- Error: `error` (에러 메시지)

#### REQ-NASA-001-09-18 (Ubiquitous) callAgentProcess 공통 함수

`callAgentProcess(ctx context.Context, ag agent.Agent, cmdBytes []byte, timeout time.Duration) ([]byte, error)` 함수는 **항상** 다음 동작을 수행해야 한다:

1. `context.WithTimeout(ctx, timeout)`으로 타임아웃 컨텍스트를 생성한다
2. 고루틴에서 `ag.Process(cmdBytes)`를 호출한다
3. `select`로 타임아웃과 결과를 대기한다
4. 타임아웃 시 `context.DeadlineExceeded`를 래핑하여 반환한다

ModbusNode의 `callAgentProcess`와 동일한 패턴이다. NASA 노드 3종이 공유한다.

#### REQ-NASA-001-09-19 (Event-Driven) Shutdown

**WHEN** `Shutdown(ctx)` 호출 시 **THEN**:

1. `stopCh`가 nil이 아니면 닫아 폴링 고루틴을 종료한다
2. `BaseNode.TransitionTo(StateStopping)`을 호출한다

NASAStatusNode, NASAControlNode, NASANode 3종 모두 동일한 Shutdown 패턴을 사용한다.

#### REQ-NASA-001-09-20 (Event-Driven) 런타임 메시지 오버라이드 — v1.18.26 REVISED

**WHEN** `Process(ctx, msg)` 호출 시 **THEN** msg.Payload에서 다음 키가 존재하면 노드 설정 / 명령 target 을 런타임으로 오버라이드한다:


| 키           | 우선순위 / 사용처                                                            | 설명                                                                                                        |
| ----------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `unit_id`   | **최우선** (control / status 명령 target)                                  | NASA addr byte 2 (`"00"`–`"3F"` indoor) 또는 dotted/compact 전체 주소 (`"10.0F.00"` / `"100F00"`).                |
| `device_id` | 차순위 (`unit_id` 미지정 시 사용)                                              | 등록된 디바이스 ID (논리적 이름). `unit_id` 와 함께 제공되면 `unit_id` 가 우선.                                                  |
| `timeout`   | 노드 `cfg.Timeout` override                                             | `time.Duration` 문자열.                                                                                       |


`applySamsungHvacr01Overrides(cfg, msg)` 헬퍼가 `(cfg, deviceID)` 를 반환 — `cfg` 에는 timeout override 가, `deviceID` 에는 명령 target 이 들어간다.

**v1.18.26 제거**: ~~`device_address`~~ (어드레싱은 `group_id` + `unit_id` 로 통일), ~~`include_raw`~~ (노드 레벨 raw 토글 제거).

#### REQ-NASA-001-09-21 (Ubiquitous) 컴파일 타임 인터페이스 검증

다음 컴파일 타임 인터페이스 검증이 **항상** 포함되어야 한다:

```go
var (
    _ Node       = (*NASAStatusNode)(nil)
    _ SourceNode = (*NASAStatusNode)(nil)
    _ Node       = (*NASAControlNode)(nil)
    _ Node       = (*NASANode)(nil)
)
```

#### REQ-NASA-001-09-22 (Ubiquitous) 프론트엔드 노드 스키마 — v1.18.26 REVISED (Breaking)

`web/src/config/nodeSchemas.ts`에 **항상** 다음 3개 스키마가 정의되어야 한다:

- `samsung_hvacr01_status`: `agent_ref`, `inactivity_timeout` (기본 `"90s"`), `timeout`, `batch_size`, `omit_state_when_off`. **advanced**: `group_id`, `unit_id`, `emit_metadata.*` (단 `emit_unit_id` / `emit_slot_num` 은 제거).
- `samsung_hvacr01_control`: `agent_ref`, `timeout`. **runtime override** (payload-level): `device_id`, `unit_id`, `timeout`.
- `samsung_hvacr01` (통합): `agent_ref`, `inactivity_timeout`, `timeout`, `batch_size`, `omit_state_when_off`, advanced 의 `group_id` / `unit_id`, `emit_metadata.*` (status 와 동일 정책).

**제거된 필드 (v1.18.26 Breaking)**: ~~`device_address`~~, ~~`device_id`~~ (config 레벨), ~~`poll_interval`~~, ~~`poll_command`~~, ~~`include_raw`~~, ~~`emit_unit_id`~~, ~~`emit_slot_num`~~.

#### REQ-NASA-001-09-23 (Ubiquitous) 프론트엔드 노드 메타데이터

`web/src/config/nodeTypeMeta.ts`에 **항상** 다음 3개 메타데이터가 정의되어야 한다:


| 타입                       | 카테고리         | 라벨                       | 설명                                |
| ------------------------- | ------------ | ------------------------ | --------------------------------- |
| `samsung_hvacr01_status`  | `processing` | `Samsung HVACR-01 Status`  | Samsung HVACR-01 디바이스 상태 조회       |
| `samsung_hvacr01_control` | `processing` | `Samsung HVACR-01 Control` | Samsung HVACR-01 디바이스 제어          |
| `samsung_hvacr01`         | `processing` | `Samsung HVACR-01`         | Samsung HVACR-01 복합 (상태 + 제어)     |


---

## 4. Specifications (상세 명세)

### 4.1 NASAConfig 설정 구조

`parseNASAConfig(opts map[string]any)` 함수는 `AgentConfig.Transport.Options`에서 다음 필드를 파싱한다:


| 필드                    | 키                          | 타입                  | 기본값      | 필수          | 설명                                                                                                         |
| --------------------- | -------------------------- | ------------------- | -------- | ----------- | ---------------------------------------------------------------------------------------------------------- |
| TransportType         | `transport_type`           | `string`            | -        | Yes         | `"serial"` 또는 `"tcp"`                                                                                      |
| SerialPort            | `serial_port`              | `string`            | -        | Serial 시 필수 | 시리얼 포트 경로                                                                                                  |
| BaudRate              | `baud_rate`                | `int`               | 9600     | No          | 통신 속도                                                                                                      |
| DataBits              | `data_bits`                | `int`               | 8        | No          | 데이터 비트                                                                                                     |
| StopBits              | `stop_bits`                | `int`               | 1        | No          | 스톱 비트                                                                                                      |
| Parity                | `parity`                   | `string`            | `"even"` | No          | 패리티                                                                                                        |
| ~~TCPAddr~~           | ~~`tcp_address`~~          | ~~`string`~~        | -        | ~~TCP 시 필수~~ | **v1.9.0에서 분리됨** — `tcp_host` + `tcp_port` 로 대체. 하위 호환을 위해 파싱은 계속 지원 (`host:port` 분해)                  |
| TCPHost               | `tcp_host`                 | `string`            | -        | TCP 시 필수    | TCP 호스트 (v1.9.0)                                                                                           |
| TCPPort               | `tcp_port`                 | `int`               | -        | TCP 시 필수    | TCP 포트 (v1.9.0)                                                                                            |
| ConnectTimeout        | `connect_timeout`          | `string`            | `"5s"`   | No          | 연결 타임아웃 (time.Duration)                                                                                    |
| ReadTimeout           | `read_timeout`             | `string`            | `"3s"`   | No          | 읽기 타임아웃                                                                                                    |
| PollInterval          | `poll_interval`            | `string`            | `"30s"`  | No          | 디바이스 상태 폴링 주기 (time.Duration)                                                                              |
| InterCommandDelay     | `inter_command_delay`      | `string` (Duration) | `"1s"`   | No          | **v1.20.0 신규**. `pollLoop`/`probeSilentDevices` 에서 여러 디바이스에 status query 를 연속 전송할 때 프레임 간 최소 간격. 요청이 μs 단위로 겹치는 것을 방지 (LG `inter_command_delay` 명칭 통일, Samsung default `"1s"`). 대기는 `stopCh`/`disconnectCh` 로 인터럽트 가능. `0s` = 딜레이 없음(기존 동작). 음수는 parse error. |
| NotifyInterval        | `report_interval`          | `string`            | `"60s"`  | No          | **v1.19.0 REVISED** — 주기적 상태 보고 간격. default `"0s"` → `"60s"` (3종 HVACR-01 통일). `0` = 비활성화, 변경 감지만 동작. `notify_interval` (v1.9.0a deprecation alias) 는 v1.19.0 부터 parse error 로 거부. |
| ~~DeviceAddresses~~   | ~~`device_addresses`~~     | ~~`[]string`~~      | -        | ~~Yes~~     | **v1.7.0에서 제거됨** — `Devices` 필드로 대체                                                                        |
| ~~DeviceIDs~~         | ~~`device_ids`~~           | ~~`map[string]string`~~ | -    | ~~No~~      | **v1.7.0에서 제거됨** — `Devices` 필드로 대체                                                                        |
| Devices               | `devices`                  | `[]agent.DeviceEntry` | -      | Yes         | 디바이스 설정 목록 (v1.7.0). 각 엔트리에 `address`(컴팩트 헥스)와 선택적 `id` 포함. `agent.ParseDevices()` 헬퍼로 파싱               |
| ProtocolFile          | `protocol_file`            | `string`            | 내장       | No          | NASA 프로토콜 정의 파일 경로                                                                                         |
| AutoDiscovery         | `auto_discovery`           | `bool`              | `true`   | No          | **v1.19.0 REVISED** — 미등록 디바이스 자동 탐색 및 등록. default `false` → `true` (LG / Century 와 정렬, 3종 HVACR-01 통일). 자동 탐색 비활성을 원하면 `false` 명시. |
| RegistryPath          | `registry_path`            | `string`            | `""`     | No          | 디바이스 레지스트리 저장 경로 (비어있으면 영속화 비활성화)                                                                          |
| OfflineThreshold      | `offline_threshold`        | `int`               | 3        | No          | 오프라인 판정 연속 실패 횟수                                                                                           |
| MsgChannelSize        | `msg_channel_size`         | `int`               | 256      | No          | 메시지 채널 버퍼 크기                                                                                               |
| UnsupportedMsgSets    | `unsupported_msg_sets`     | `[]int` (hex)       | -        | No          | 필터링할 메시지 셋 인덱스 목록 (v1.1.0). YAML에서 `0x0608` 형식으로 지정. `map[uint16]bool`로 파싱됨                                |
| LogUnsupportedMsgSets | `log_unsupported_msg_sets` | `bool`              | `false`  | No          | 필터링된 메시지 셋을 디버그 로그에 기록할지 여부 (v1.1.0)                                                                       |
| IncludeRawHex         | `include_raw_hex`          | `bool`              | `true`   | No          | **v1.19.0 RENAMED** (was `include_raw_message_sets`, v1.1.0). 상태 조회 응답에 RawMessageSets / raw hex 포함 여부. `false`로 설정하면 `get_state`/`get_all_states` 응답에서 RawMessageSets 제외. 이전 키 `include_raw_message_sets` 는 v1.19.0 부터 parse error 로 거부 (LG / Century 의 동명 필드와 정렬, 3종 HVACR-01 통일). |
| ReconnectInterval     | `reconnect_interval`       | `string` (Duration) | `"5s"`   | No          | 재연결 기본 간격 (v1.2.0). 지수 백오프의 초기값으로 사용                                                                       |
| MaxReconnectBackoff   | `max_reconnect_backoff`    | `string` (Duration) | `"5m"`   | No          | 재연결 최대 백오프 (v1.2.0). 지수 백오프의 상한값                                                                           |
| BuzzerOnControl       | `buzzer_on_control`        | `bool`              | `false`  | No          | 제어 명령 시 실내기 부저 울림 여부 (v1.4.0). `true`면 부저 On(0x00), `false`면 부저 Off(0x01, 억제). sendControlCommand에서 MsgBuzzer(0x4050) 자동 추가 |
| LogDecodeErrors       | `log_decode_errors`        | `bool`              | `false`  | No          | decode error 발생 시 WARN 로그 출력 여부 (v1.9.0). `false`(기본값)면 decode error 로그를 억제한다. 노이즈가 많은 RS-485 버스에서 로그 폭주를 방지 |
| LogDrops              | `log_drops`                | `bool`              | `false`  | No          | **v1.19.0 신규** (3종 HVACR-01 통일). msgCh / ring buffer 가득 참으로 인한 frame drop 시 per-drop WARN 로그. 통계 카운터 (`framesDropped`) 는 옵션 값과 무관하게 항상 증가 (LG / Century `log_drops` 패턴 동일). |
| LogStateUpdates       | `log_state_updates`        | `bool`              | `false`  | No          | **v1.19.0 신규** (3종 HVACR-01 통일). device state 변경 / report / keepalive emit 시 DEBUG 로그. 운영자가 emit 흐름을 추적하고 싶을 때 활성. |


### 4.2 파일 구조

```
internal/agent/samsung/
 ├── agent.go          # NASAAgent 구현 (Init, Start, Stop, Process, ReceiveMessage, filterMessageSets)
 ├── config.go         # NASAConfig 파싱 및 검증 (parseNASAConfig, UnsupportedMsgSets/LogUnsupportedMsgSets/IncludeRawHex/BuzzerOnControl 포함, v1.19.0)
 ├── address.go        # NASAAddress 타입 ([3]byte), 주소 상수, 헬퍼 함수
 ├── device.go         # NASADevice, NASADeviceState, HexKeyByteMap, StateForJSON 타입 정의
 ├── protocol.go       # NASAProtocol 인터페이스 및 구현 (인코딩/디코딩)
 ├── message.go        # NASAMessage, NASAMessageSet, 명령 코드/인덱스 상수 정의
 ├── crc.go            # CRC16-CCITT 체크섬 구현
 ├── discovery.go      # 실외기/실내기 주소 탐색 프로토콜 구현
 ├── transport.go      # NASATransport 인터페이스, Serial/TCP 구현
 ├── register.go       # RegisterSamsungNASATypes 등록 함수
 ├── errors.go         # 센티널 에러 정의
 ├── nasa.yaml         # NASA 프로토콜 정의 파일 (embed 대상)
 └── samsung_test.go   # 단위 테스트

internal/node/
 ├── bridge_adapter.go # CommandPollAdapter 인터페이스 정의 (v1.1.0)
 ├── bridge.go         # startCommandPollLoop 메서드 (v1.1.0)
 └── adapter/
     ├── nasa.go       # NASAAdapter 구현 (BridgeAdapter + CommandPollAdapter) (v1.1.0)
     ├── nasa_test.go  # NASAAdapter 단위 테스트 (v1.1.0)
     └── register.go   # "samsung_hvacr01" 어댑터 레지스트리 등록 (v1.1.0)

examples/
 ├── agents/
 │   ├── samsung_hvacr01-serial.yaml  # Serial 모드 예제 (v1.1.0)
 │   └── samsung_hvacr01-tcp.yaml     # TCP 모드 예제 (v1.1.0)
 └── flows/
     ├── samsung_hvacr01-monitoring.yaml   # 이벤트 기반 모니터링 플로우
     └── samsung_hvacr01-polling.yaml      # CommandPollAdapter 폴링 플로우 (v1.1.0)

web/src/config/
 ├── nodeSchemas.ts               # NASA 노드 스키마 3종 추가 (v1.3.0)
 └── nodeTypeMeta.ts              # NASA 노드 메타데이터 3종 추가 (v1.3.0)
```

### 4.3 YAML 에이전트 설정 예시

```yaml
agents:
  - id: "samsung-hvacr01-01"
    name: "Samsung HVACR-01 Controller"
    type: "samsung_hvacr01"
    transport:
      type: "custom"
      options:
        transport_type: "serial"
        serial_port: "/dev/ttyUSB0"
        baud_rate: 9600
        parity: "even"
        poll_interval: "30s"
        # v1.19.0 — `notify_interval` rename + default 변경: `report_interval` "60s" (was "0s").
        # `notify_interval` 키는 parse error 로 거부됨.
        report_interval: "60s"
        # v1.7.0: 통합 devices 형식 (agent.DeviceEntry)
        # address는 컴팩트 헥스 형식("200000"), id는 선택사항
        devices:
          - address: "200000"
            id: "living-room"    # 실외기 0번의 실내기 0번
          - address: "200001"
            id: "bedroom-1"      # 실외기 0번의 실내기 1번
          - address: "200002"
            id: "bedroom-2"      # 실외기 0번의 실내기 2번
          - address: "200100"
            id: "kitchen"        # 실외기 1번의 실내기 0번
        # v1.19.0 — default 변경: `auto_discovery` true (was false). 비활성 원하면 명시.
        auto_discovery: true
        registry_path: "/var/lib/xflow/nasa-hvac-01-devices.json"
        offline_threshold: 3
        # v1.1.0: 지원하지 않는 메시지 셋 필터링
        unsupported_msg_sets:
          - 0x0608
          - 0x060C
          - 0x8601
          - 0x860C
          - 0x860D
        log_unsupported_msg_sets: false
        # v1.1.0: 상태 조회 시 RawMessageSets 포함 여부 (기본값: true)
        # v1.19.0 — rename: `include_raw_message_sets` → `include_raw_hex` (LG / Century 동명 필드와 정렬).
        # 이전 키는 parse error 로 거부됨.
        include_raw_hex: false
        # v1.19.0 — 3종 HVACR-01 통일 로그 옵션 (default false)
        log_decode_errors: false
        log_drops: false
        log_state_updates: false
```

### 4.4 예제 파일 설명 (v1.1.0)


| 파일                                                 | 설명                                                                                                                  |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `examples/agents/samsung_hvacr01-serial.yaml`      | RS-485 시리얼 포트 기반 Samsung HVACR-01 에이전트 설정 예제. `unsupported_msg_sets`, `log_unsupported_msg_sets`, `include_raw_hex` (v1.19.0 rename, was `include_raw_message_sets`) 포함 |
| `examples/agents/samsung_hvacr01-tcp.yaml`         | TCP 기반 Samsung HVACR-01 에이전트 설정 예제. `unsupported_msg_sets`, `log_unsupported_msg_sets`, `include_raw_hex` (v1.19.0 rename) 포함           |
| `examples/flows/samsung_hvacr01-polling.yaml`      | `CommandPollAdapter`를 사용하는 폴링 플로우 예제. `samsung_hvacr01` 어댑터가 `get_all_states` 명령으로 주기적 상태 조회 후 console-logger로 출력      |


### 4.5 Traceability (추적성)


| 요구사항                  | 원본 SPEC                     | 모듈       |
| --------------------- | --------------------------- | -------- |
| REQ-NASA-001-01-01    | REQ-SAGENT-001-08-01        | Module 1 |
| REQ-NASA-001-04-01    | REQ-SAGENT-001-08-03        | Module 4 |
| REQ-NASA-001-04-02    | REQ-SAGENT-001-08-04        | Module 4 |
| REQ-NASA-001-04-03    | REQ-SAGENT-001-08-05        | Module 4 |
| REQ-NASA-001-05-01    | REQ-SAGENT-001-08-06        | Module 5 |
| REQ-NASA-001-03-03    | REQ-SAGENT-001-08-07        | Module 3 |
| REQ-NASA-001-05-06    | REQ-SAGENT-001-08-08        | Module 5 |
| REQ-NASA-001-05-03/04 | REQ-SAGENT-001-08-09        | Module 5 |
| REQ-NASA-001-03-07    | v1.1.0 신규                   | Module 3 |
| REQ-NASA-001-04-02-01 | v1.1.0 신규                   | Module 4 |
| REQ-NASA-001-04-02-02 | v1.1.0 신규                   | Module 4 |
| REQ-NASA-001-06-04    | v1.1.0 신규                   | Module 6 |
| REQ-NASA-001-06-05    | v1.1.0 신규                   | Module 6 |
| REQ-NASA-001-06-06    | v1.1.0 신규                   | Module 6 |
| REQ-NASA-001-08-03    | v1.1.0 신규                   | Module 8 |
| REQ-NASA-001-01-08    | v1.1.0 신규                   | Module 1 |
| REQ-NASA-001-01-09    | v1.2.0 신규                   | Module 1 |
| REQ-NASA-001-01-10    | v1.2.0 신규                   | Module 1 |
| REQ-NASA-001-01-11    | v1.2.0 신규                   | Module 1 |
| REQ-NASA-001-01-12    | v1.2.0 신규                   | Module 1 |
| REQ-NASA-001-02-05    | v1.2.0 신규                   | Module 2 |
| REQ-NASA-001-07-03    | v1.2.0 신규                   | Module 7 |
| REQ-NASA-001-09-01    | v1.3.0 신규                   | Module 9 |
| REQ-NASA-001-09-02~06 | v1.3.0 신규 (NASAStatusNode)  | Module 9 |
| REQ-NASA-001-09-07~11 | v1.3.0 신규 (NASAControlNode) | Module 9 |
| REQ-NASA-001-09-12~14 | v1.3.0 신규 (NASANode 복합)     | Module 9 |
| REQ-NASA-001-09-15    | v1.3.0 신규 (센티널 에러)          | Module 9 |
| REQ-NASA-001-09-16    | v1.3.0 신규 (레지스트리)           | Module 9 |
| REQ-NASA-001-09-17~21 | v1.3.0 신규 (공통)              | Module 9 |
| REQ-NASA-001-09-22~23 | v1.3.0 신규 (프론트엔드)           | Module 9 |
| REQ-NASA-001-05-07    | v1.4.0 신규 (부저 자동 제어)        | Module 5 |
| REQ-NASA-001-02-06    | v1.4.0 신규 (RS-485 프리앰블)     | Module 2 |
| REQ-NASA-001-02-07    | v1.7.0 신규 (TransportChecker) | Module 2 |
| REQ-NASA-001-04-15    | v1.7.0 신규 (프로토콜 추상화)        | Module 4 |
| REQ-NASA-001-04-16    | v1.7.0 신규 (Device 통합 구조체)   | Module 4 |
| REQ-NASA-001-02-03 (amend) | v1.9.0 (TCP host/port 분리, TCP_NODELAY) | Module 2 |
| REQ-NASA-001-04-03-01 | v1.9.0 신규 (poll_bulk content dedup, message_type) | Module 4 |
| REQ-NASA-001-09-04 (amend) | v1.9.0 (노드 Init-tolerance, deferred connection) | Module 9 |


---

---

## 5. Implementation Notes (구현 노트)

### 5.1 구현 요약


| 항목       | 값                    |
| -------- | -------------------- |
| 구현 기간    | 2026-02-24           |
| 파일 수     | 21개 (소스 11 + 테스트 10) |
| 코드 줄 수   | 7,352줄               |
| 테스트 커버리지 | 87.4%                |
| 레이스 디텍터  | 통과                   |
| 커밋       | `94ac8d2`            |


### 5.2 SPEC 대비 주요 변경사항

#### 5.2.1 아키텍처 변경

- **BaseAgent 대신 BaseLifecycle 임베딩**: SPEC에서는 `*BaseAgent` 임베딩을 명세했으나, 실제 구현은 InfluxDB Agent 패턴을 따라 `*lifecycle.BaseLifecycle`을 직접 임베딩. 이는 Agent 인터페이스의 13개 메서드를 모두 직접 구현하되, 라이프사이클 관리만 BaseLifecycle에 위임하는 패턴.
- **DefaultManager 사용**: SPEC에서 언급된 `DefaultTypeRegistry` 대신 `agent.DefaultManager`를 사용하여 타입 등록. 이는 프로젝트의 실제 레지스트리 구현체가 `DefaultManager`이기 때문.

#### 5.2.2 프로토콜 처리 변경

- **자체 프로토콜 구현**: SPEC에서 계획한 `nasa.yaml` + Protocol Definition Engine 조합 대신, `protocol.go`에서 직접 NASA 프레임 인코딩/디코딩을 구현. NASA 프로토콜의 가변 길이 Message Set과 Index 기반 Value 크기 결정이 범용 파서로 처리하기 어려워 전용 구현 선택.
- **CRC16-CCITT 초기값 0x0000**: 다항식 0x1021, 초기값 0x0000으로 확정.
- **시리얼 팩토리 함수**: `go.bug.st/serial` 직접 의존 대신, `SerialOpener` 함수 변수를 통해 시리얼 포트 팩토리를 주입할 수 있도록 설계. 테스트에서 mock으로 대체 가능.

#### 5.2.3 기능 범위 변경

- **디스커버리 간소화**: 실외기 디스커버리는 단계 2(Standby Query, C001)만 구현. 단계 1(주소 등록 확인), 단계 3(주소 확정), 단계 4(Ready 상태 확인)는 미구현 (향후 확장 가능).
- **NotifyInterval 미구현**: 주기적 상태 보고(`device_state_report`)는 미구현. 상태 변경 알림(`device_state_changed`)만 동작.
- **디바이스 레지스트리 영속화 미구현**: `RegistryPath` 설정은 파싱되나, JSON 파일 저장/로드 로직은 미구현.
- **오프라인 감지 간소화**: `OfflineThreshold` 기반 연속 실패 카운터는 미구현. 응답이 오면 Online, 미등록이면 AutoDiscovery에 따라 처리.

#### 5.2.4 파일 구조 변경

- SPEC 계획의 단일 테스트 파일 `samsung_test.go` 대신 모듈별 테스트 파일로 분리:
  - `address_test.go`, `agent_test.go`, `config_test.go`, `crc_test.go`, `device_test.go`, `discovery_test.go`, `message_test.go`, `protocol_test.go`, `register_test.go`, `transport_test.go`

#### 5.2.5 v1.1.0 이후 추가 구현 사항

다음 기능들은 v1.0.0 SPEC 완료 이후에 별도로 구현되어 v1.1.0에서 문서화되었다:

- **NASAConfig 신규 필드 3종**: `UnsupportedMsgSets`, `LogUnsupportedMsgSets`, `IncludeRawMessageSets` 설정 필드 추가 (config.go)
- **HexKeyByteMap 커스텀 타입**: `uint16` 키를 hex 문자열로 직렬화하는 `map[uint16][]byte` 래퍼 타입 (device.go)
- **StateForJSON 조건부 직렬화**: `IncludeRawMessageSets` 설정에 따라 RawMessageSets 포함/제외 (device.go)
- **Message Set 필터링**: `UnsupportedMsgSets`에 등록된 인덱스의 메시지 셋을 수신 시 제외하는 `filterMessageSets()` 메서드 (agent.go)
- **State() 출력 개선**: `unsupported_msg_sets` 리스트를 State() 응답에 포함 (agent.go)
- **CommandPollAdapter 인터페이스**: JSON 명령 기반 폴링을 위한 새 인터페이스 (bridge_adapter.go)
- **NASAAdapter 브릿지 어댑터**: `BridgeAdapter` + `CommandPollAdapter` 구현, `"samsung-nasa"` 레지스트리 등록 (adapter/nasa.go, adapter/register.go)
- **startCommandPollLoop**: `CommandPollAdapter` 기반 주기적 폴링 루프 (bridge.go)
- **예제 YAML 파일**: Serial/TCP 에이전트 설정 예제 및 CommandPollAdapter 폴링 플로우 예제 (examples/)

#### 5.2.6 v1.2.0 신규 요구사항

다음 요구사항은 v1.2.0에서 추가되었으며, 구현 완료되었다 (커밋 `e3f28b4`):

- **Transport 재연결 루프** (REQ-NASA-001-01-09): 에이전트 레벨의 지수 백오프 기반 재연결 루프
- **수신 루프 연결 끊김 감지** (REQ-NASA-001-01-10): receiveLoop에서 I/O 에러 감지 및 reconnectLoop 시작
- **연결 끊김 시 폴링 중지** (REQ-NASA-001-01-11): disconnectCh를 통한 pollLoop 중지
- **재연결 관련 NASAAgent 필드** (REQ-NASA-001-01-12): disconnectCh, reconnecting 필드
- **Transport I/O 에러 시 상태 갱신** (REQ-NASA-001-02-05): Send()/Receive() I/O 에러 시 open 플래그 갱신
- **재연결 이벤트 메시지** (REQ-NASA-001-07-03): transport_disconnected/reconnecting/reconnected 이벤트
- **NASAConfig 신규 필드**: ReconnectInterval, MaxReconnectBackoff
- **State() 출력 확장**: transport_connected, reconnecting, reconnect_attempts 필드
- **Start() 동작 변경** (REQ-NASA-001-01-05 수정): transport.Open() 실패 시 에러 대신 reconnectLoop 시작
- **TCP 트랜스포트 변경** (REQ-NASA-001-02-03 수정): ReconnectInterval/MaxReconnectAttempts 제거 (에이전트 레벨로 이동)

#### 5.2.7 v1.3.0 신규 요구사항

다음 요구사항은 v1.3.0에서 추가되었으며, 구현 완료되었다 (커밋 `1027a57`):

- **NASANodeConfig 설정 구조체** (REQ-NASA-001-09-01): 노드 레벨 설정 (agent_ref, device_address, device_id, poll_interval, include_raw, timeout)
- **NASAStatusNode** (REQ-NASA-001-09-02~06): 상태 조회 전용 노드. SourceNode 인터페이스로 주기적 폴링 지원
- **NASAControlNode** (REQ-NASA-001-09-07~11): 제어 명령 전용 노드. 직접 명령 + 간소화 형식 자동 변환
- **NASANode 복합** (REQ-NASA-001-09-12~14): 상태 + 제어 자동 감지 복합 노드
- **센티널 에러** (REQ-NASA-001-09-15): ErrNASAAgentNotNASA, ErrNASAMissingAgentRef, ErrNASANoResolver, ErrNASAProcessFailed
- **레지스트리 등록** (REQ-NASA-001-09-16): samsung_hvacr01_status, samsung_hvacr01_control, samsung_hvacr01 3종 빌트인 등록 (12 -> 15)
- **프론트엔드 스키마/메타데이터** (REQ-NASA-001-09-22~23): nodeSchemas.ts, nodeTypeMeta.ts

#### 5.2.8 v1.4.0 신규 요구사항

다음 요구사항은 v1.4.0에서 추가되었으며, 구현 완료되었다:

- **부저 자동 제어** (REQ-NASA-001-05-07): `sendControlCommand`에서 MsgBuzzer(0x4050) 자동 추가. `BuzzerOnControl` 설정으로 On/Off 제어 (agent.go)
- **RS-485 프리앰블** (REQ-NASA-001-02-06): Serial/TCP 트랜스포트 `Send()`에 0x55 x 100 프리앰블 추가. `prependPreamble()` 헬퍼 함수 (transport.go)
- **NASAConfig 신규 필드**: `BuzzerOnControl` (`buzzer_on_control`, bool, 기본값 false). `toBool()` 헬퍼 함수 (config.go)
- **프론트엔드 UI**: `buzzer_on_control` 필드 Samsung NASA 에이전트 스키마에 추가 (agentSchemas.ts)

#### 5.2.9 v1.7.0 신규 요구사항

다음 요구사항은 v1.7.0에서 추가되었으며, 구현 완료되었다:

- **Device Configuration 통합 구조체** (REQ-NASA-001-04-16): `DeviceAddresses []string` + `DeviceIDs map[string]string` 두 개의 분리된 설정 필드를 `Devices []agent.DeviceEntry` 단일 배열로 통합. `agent.ParseDevices()` 헬퍼 함수로 파싱 로직 중앙화 (agent/config.go)
- **주소 형식 표준화**: 기존 공백 구분 형식(`"20 00 00"`)에서 컴팩트 헥스 형식(`"200000"`)으로 통일. 예제 YAML 파일 및 내부 처리 모두 컴팩트 헥스 기준으로 변경
- **TransportChecker 인터페이스** (REQ-NASA-001-02-07): `TransportConnected() bool` 메서드 추가. 에이전트 라이프사이클 상태와 별개로 실제 시리얼/TCP 트랜스포트 연결 상태를 반환 (agent.go)
- **NASADeviceAdapter 프로토콜 추상화** (REQ-NASA-001-04-15):
  - `Protocol` 필드: 프로토콜 이름 오버라이드 가능 (`"nasa"` → `"lgap"` 등). 상태/이벤트 JSON에 `protocol` 필드로 반영
  - `ExtraProperties map[string]any`: 프로토콜별 확장 상태 속성 지원. `State()` 출력에 병합
  - `DeviceSource` 필드 + `Source()` 메서드: 디바이스 출처 추적 (`"config"` vs `"auto"`)
- **예제 설정 업데이트**: `samsung_hvacr01-serial.yaml`, `samsung_hvacr01-tcp.yaml`이 새 `devices` 형식으로 변경. 시리얼 포트 경로 수정 (파일명은 추후 식별자 rename 으로 갱신됨)

### 5.3 미구현 항목 (향후 확장)


| 항목                         | SPEC 요구사항          | 상태    |
| -------------------------- | ------------------ | ----- |
| 디바이스 레지스트리 영속화             | REQ-NASA-001-04-11 | 미구현   |
| 주기적 상태 보고 (NotifyInterval) | REQ-NASA-001-04-07 | 미구현   |
| 오프라인 감지 (연속 실패 카운터)        | REQ-NASA-001-04-04 | 부분 구현 |
| 실외기 디스커버리 단계 1,3,4         | REQ-NASA-001-04-12 | 부분 구현 |
| cmd/xflowd/main.go 등록 호출   | plan.md 작업 10      | 미구현   |


---

*SPEC-SAMSUNG-HVACR-001 v1.9.0*
*작성자: xtra*
*날짜: 2026-05-14*