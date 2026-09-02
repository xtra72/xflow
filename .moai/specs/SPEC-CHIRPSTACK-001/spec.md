---
id: SPEC-CHIRPSTACK-001
title: "ChirpStack LoRaWAN 에이전트"
version: 0.1.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: spec
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: L
tags: [chirpstack, lorawan, mqtt, agent, device-state, comm-state]
---

# SPEC-CHIRPSTACK-001: ChirpStack LoRaWAN 에이전트

## Environment (환경)

- 언어/런타임: Go 1.23+ 모듈 `github.com/xtra/xflow`.
- 신규 패키지: `internal/agent/chirpstack/`, 신규 노드 `internal/node/chirpstack.go`.
- 외부 의존: ChirpStack LoRaWAN Network Server 가 발행하는 MQTT 업링크 (토픽 `application/#`), paho MQTT 클라이언트(기존 재사용).
- 재사용 기반: `internal/agent/system/mqtt_agent.go` (MQTT 트랜스포트), `internal/agent/device_id_repo.go`(UID), `internal/device/registry.go`(디바이스), `pkg/message`(발행).
- 개발 방법론: Hybrid (신규 코드 TDD, 기존 흡수 DDD), 커버리지 목표 85%.

## Assumptions (가정)

- A1. ChirpStack 업링크 JSON 구조는 packet.json 샘플과 호환된다 (`deviceInfo{devEui,deviceName,deviceProfileName,tags}`, `object`, `rxInfo[]`, `time`).
- A2. 다운스트림 store/influx 소비자는 기존 계약(`$.payload.value`, `$.metadata.measurement`, `$.metadata.device.id/name`, `$.metadata.tags.*`, `$.timestamp`)을 그대로 유지한다.
- A3. 에이전트 이름은 배포 내에서 고유하다 (device_id 키잉 전제).
- A4. `object` 스칼라 값은 숫자 또는 문자열이다 (packet.json `magnet_status:"close"` 실측). 비스칼라는 1차 범위에서 skip.
- A5. ChirpStack 은 패시브/푸시이며 폴 루프/error_count 가 없다. online 은 업링크 staleness 로만 추론한다.

## Requirements (EARS)

### 프로즌 결정 반영 요구 (사용자 승인 — 변경 금지)

- REQ-FROZEN-01 (per-measurement 출력): The system **shall** fan out the uplink `object` field into one message per measurement, where each message has `type="event"`, top-level `timestamp` set from the uplink `time` via WithTimestamp, `payload={value:<scalar>}`, and metadata carrying `measurement=<key>`, `device={id,name}`, `tags={...}`. 이 출력은 기존 Lua `script`+`split` 파이프라인을 대체한다.
- REQ-FROZEN-02 (다운스트림 계약 보존): The system **shall not** change the downstream read paths `$.payload.value`, `$.metadata.measurement`, `$.metadata.device.id`, `$.metadata.device.name`, `$.metadata.tags.*`, `$.timestamp`.
- REQ-FROZEN-03 (comm-state fold): Where `emit_comm_state` is enabled, the system **shall** emit `device_state.<trigger>` with a `state` group carrying `online:bool, rssi:int, snr:float, gateway_id:string, last_seen_ms:int64(UnixMilli)` and `trigger ∈ {change, report}`, folded into the device_state stream (별도 `device_connection` 타입 금지).
- REQ-FROZEN-04 (tags pass-through): The system **shall** carry `deviceInfo.tags` verbatim into `metadata.tags` with no key mapping.

### M1 — 에이전트 스켈레톤 + 등록 wiring

- REQ-M1-01 (Ubiquitous): The system **shall** register an agent type named `chirpstack` via `RegisterChirpStackTypes(mgr)` following the per-agent registration pattern.
- REQ-M1-02 (Ubiquitous): The system **shall** be invocable from `cmd/xflowd/main.go` 의 에이전트 타입 등록 블록 (`main.go:360-413`); 등록 호출 및 import 누락 시 인스턴스화 불가함을 방지한다.
- REQ-M1-03 (Ubiquitous): The `ChirpStackAgent` **shall** implement the `agent.Agent` interface (Init/Start/Stop/Pause/Resume/Health/Process/Configure/ID/Name/Type/Info/Stats).
- REQ-M1-04 (State-driven): While the agent name collides with an existing registered name, the system **shall not** silently overwrite identity; 이름 고유성 위반은 감지/거부되어야 한다.

### M2 — MQTT 연결 / 구독 / 수신

- REQ-M2-01 (Event-driven): When the agent starts, the system **shall** connect to the configured MQTT broker and subscribe to the configured topics (default `application/#`).
- REQ-M2-02 (Event-driven): When an MQTT message arrives on a subscribed topic, the system **shall** enqueue it to the receive channel and expose it via `ReceiveMessage(ctx)`.
- REQ-M2-03 (State-driven): While `auto_reconnect` is enabled, the system **shall** re-subscribe on reconnect; while the agent is stopped, the system **shall not** revive the session (stopped 가드).
- REQ-M2-04 (Ubiquitous): The system **shall** expose actual broker connectivity via `TransportConnected()`.

### M3 — 업링크 디코드 + per-measurement 발행

- REQ-M3-01 (Event-driven): When an uplink is received, the system **shall** decode `deviceInfo`, `object`, `rxInfo`, and `time`.
- REQ-M3-02 (Event-driven): When decoding `object`, the system **shall** emit one message per scalar measurement per REQ-FROZEN-01.
- REQ-M3-03 (Ubiquitous): The system **shall** set top-level `timestamp` from the uplink `time` (time.Time via WithTimestamp), and payload-internal epoch fields as int64 UnixMilli.
- REQ-M3-04 (Ubiquitous): The system **shall** emit `unit_id`(=devEui) and rely on node-side `device` group promotion rather than hand-building `metadata.device`.
- REQ-M3-05 (Unwanted): If an `object` value is non-scalar (nested object/array), then the system **shall** skip it and log a warning (1차 범위).
- REQ-M3-06 (Ubiquitous): The `chirpstack-in` node **shall** be registered in `node/registry.go` and whitelisted in `flow/validate.go`, connecting directly to store/influx consumers.

### M4 — 디바이스 자동 생성 + Provider + 메타데이터 지속화

- REQ-M4-01 (Event-driven): When an uplink is received, the system **shall** auto-create/lookup a device keyed by `devEui` and resolve its UID (UUID v4) via `agent.ResolveDeviceID(ctx, agentID, devEui)`.
- REQ-M4-02 (Event-driven): When a device is created/updated, the system **shall** register runtime info via `agent.SetDeviceInfo(agentID, devEui, {DeviceType=deviceProfileName, Label=deviceName})`.
- REQ-M4-03 (Ubiquitous): The system **shall** expose the device roster via `DeviceProvider()` so `cmd/xflowd/main.go` auto-registers it in the device registry.
- REQ-M4-04 (Event-driven): When device metadata changes, the system **shall** persist `deviceName → Name/Label` and `tags → DeviceMetadata` via registry `SetMetadata` keyed by UUID.
- REQ-M4-05 (Ubiquitous): The system **shall** comply with SPEC-DEVICE-001 (통합 레지스트리) and SPEC-DEVICE-IDENTITY-001 Phase D (`ID()==UID()==UUID v4`; emit uses `unit_id`+`device_id`).

### M5 — comm-state (device_state fold + staleness watchdog, optional)

- REQ-M5-01 (State-driven): While `emit_comm_state` is disabled, the system **shall not** emit any `device_state` message.
- REQ-M5-02 (Event-driven): When an uplink is received, the system **shall** update the per-devEui last-seen timestamp and, on a meaningful state change or online transition, emit `device_state.change`.
- REQ-M5-03 (State-driven): While `now - last_seen_ms > offline_threshold`, the system **shall** transition the device to offline and emit `device_state.change` (inferred from staleness, not failed requests).
- REQ-M5-04 (State-driven): While `comm_report_interval > 0`, the system **shall** periodically emit `device_state.report`; while `comm_report_interval == 0`, the system **shall not** emit periodic reports (change 이벤트는 유지).
- REQ-M5-05 (Ubiquitous): The `state` group **shall** derive `rssi/snr/gateway_id` from the best gateway in `rxInfo` (예: 최대 rssi).
- REQ-M5-06 (Unwanted): If last-seen data is unavailable after restart, then the system **shall not** report online prematurely (unknown 보류).

### M6 — 테스트 + 품질 게이트

- REQ-M6-01 (Ubiquitous): The system **shall** provide table-driven tests using the `packet.json` fixture covering decode, fan-out, device create, tags pass-through, and comm-state.
- REQ-M6-02 (Ubiquitous): The system **shall** achieve ≥85% coverage on new code and pass TRUST 5 gates (LSP zero errors/type/lint at run phase).
- REQ-M6-03 (Ubiquitous): The watchdog/report goroutines **shall** terminate on context cancellation (누수 없음).

## Specifications (사양 상세)

### point ↔ spot 불일치 명시 (필수 기록)

- 실측: `packet.json` 의 `deviceInfo.tags` 는 `{location, point, spot}` 세 키를 모두 가진다.
- 동일 파일 레거시 split 출력의 `metadata.tags` 는 `{location, spot}` (point 누락, spot=location 값) — Lua 매핑 아티팩트.
- 레거시 Lua 는 `tags.spot` 을 읽었다.
- **Pass-through(REQ-FROZEN-04) 하에서는 `deviceInfo.tags` 의 모든 키를 verbatim 전달**하며, `point`/`spot` 선택은 다운스트림 소비자 몫이다. 키 매핑/정규화 없음.

### 마일스톤 우선순위

- Priority High: M1, M2, M3, M4 (핵심 수신→발행→디바이스 경로).
- Priority Medium: M5 (comm-state, optional).
- Priority Low: M6 관측성 통계 확장(ConnectionStats 등).

의존: M1 완료 → M2 → M3 → M4; M5 는 M3/M4 이후; M6 은 전 구간 병행.

## Traceability (추적성)

- research.md: 코드베이스 근거 (file:line 인용).
- design.md: 컴포넌트/매핑/스키마/watchdog/config/wiring.
- acceptance.md: Given-When-Then 시나리오 + TRUST 5 + DoD.
- 관련 SPEC: SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001 (Phase D), SPEC-HVACR-CONNSTATE-001 (deprecated 근거).

## Implementation Notes (as-implemented)

> spec-anchored Level 2 정련 기록. 원 요구사항 텍스트는 보존하며 아래 분기는 **주석(annotate)** 이다(삭제 없음). 구현은 6개 run 커밋 M1~M6(`eb0339f9`/`6bb3a965`/`ca024d1d`/`bc08f3ce`/`b26a8850`/`88807142`)으로 완료되었고, TRUST 5 PASS(Critical 0), 신규 코드 커버리지 89.4%, go build/vet/gofmt 클린·신규 코드 lint 0, REQ-FROZEN-01~04 준수, 무회귀(무관한 pre-existing flaky E2E `internal/api/service` 는 본 SPEC 과 무관).

### 산출물 (신규)

- 신규 패키지 `internal/agent/chirpstack/`: `agent.go`, `config.go`, `decode.go`, `message.go`, `provider.go`, `watchdog.go`, `registration.go` + 테스트 + `testdata/packet.json`.
- 신규 노드 `internal/node/chirpstack.go` (`chirpstack-in` SourceNode).
- 신규 에이전트 타입 `chirpstack`, 신규 노드 타입 `chirpstack-in`.
- Wiring 3곳: `cmd/xflowd/main.go` (`RegisterChirpStackTypes` 호출 + import), `internal/node/registry.go` (`chirpstack-in` 테이블 등록), `pkg/flow/validate.go` (`agentRefRequiredTypes` 추가).

### 계획 대비 분기 (IN-1 ~ IN-4)

- **IN-1 (flow-validate 경로 정정)**: SPEC REQ-M3-06 은 `internal/flow/validate.go` 를 참조하나 **그 파일은 존재하지 않는다**. 실제 구현은 `pkg/flow/validate.go` 의 `agentRefRequiredTypes` 집합에 `chirpstack-in` 을 추가했다. 이는 "미등록 노드 타입 거부용 존재성(existence) 화이트리스트"가 아니라 "agent_ref 필수 노드 검증 활성화" 집합이다(노드 타입 존재 판정은 `internal/node/registry.go` factories 맵 소관). — REQ-M3-06 충족.
- **IN-2 (tags 지속화 타입)**: registry 지속화는 `DeviceMetadata.Tags []string` 가 아니라 **`DeviceMetadata.Labels map[string]string`** 에 매핑했다. ChirpStack tags 는 key-value map(`{location, point, spot}`)이라 `[]string` 슬롯과 형이 맞지 않기 때문이다. 메시지 레벨 `metadata.tags` verbatim pass-through(REQ-FROZEN-04)는 이 결정과 무관하게 그대로 유지된다. — REQ-M4-04/FROZEN-04 충족.
- **IN-3 (comm-state 배치)**: comm-state `state` 그룹은 메시지 `payload` 에 typed 값(`online:bool`, `rssi:int`, `snr:float`, `last_seen_ms:int64`)으로 담고, 디바이스 식별은 노드의 dedup 승격으로 처리한다. 노드는 `msg.Type()` 을 계층형 `device_state.<trigger>` 로 설정한다(별도 `device_connection` 타입 미도입). — REQ-FROZEN-03/M5-02~05 충족.
- **IN-4 (rxInfo 디코딩 위치 + 단일 전송)**: rxInfo 디코딩은 M5(comm-state 전용)에 위치한다. device_state 는 별도 채널이 아니라 **기존 recvCh 에 `record` 판별자(discriminator)로 접힌 단일 전송(folded stream)** 으로 실린다(REQ-FROZEN-03 fold 정신 준수). — REQ-FROZEN-03 충족.

### AC / REQ 충족 확인

- AC-1 ~ AC-9 전량 충족(decode·per-measurement fan-out·device 자동생성·tags pass-through·comm-state·이름 고유성·main.go wiring·MQTT 트랜스포트).
- REQ-FROZEN-01 (per-measurement 출력) / -02 (다운스트림 계약 보존) / -03 (comm-state fold) / -04 (tags pass-through) 준수.
- 신규 코드 커버리지 89.4% (목표 85% 초과), TRUST 5 PASS.
