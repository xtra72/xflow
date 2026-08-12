---
id: SPEC-CHIRPSTACK-001
title: "ChirpStack LoRaWAN 에이전트 — 코드베이스 조사"
version: 0.1.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: research
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: L
tags: [chirpstack, lorawan, mqtt, agent, research]
---

# ChirpStack 에이전트 — 코드베이스 조사 (research.md)

> 본 문서는 SPEC-CHIRPSTACK-001 구현 착수 전 xflow 코드베이스 정찰 결과를 정리한 근거 산출물이다.
> 모든 사실은 `file:line` 인용으로 검증되었으며, `/moai:2-run` 단계에서 재확인한다.

## 0. 조사 범위 및 방법

- 대상 저장소: `github.com/xtra/xflow` (cwd `/Users/xtra/Projects/xflow`), Go 모듈.
- 조사 축: (1) 에이전트 2계층 아키텍처, (2) 디바이스 관리/식별, (3) 메시지 발행 관용구, (4) 기존 ChirpStack 처리(플로우+Lua), (5) 통신 상태(comm-state) 모델.
- 방법: 대표 에이전트(MQTT/Century/Samsung/LG)와 노드/레지스트리/디바이스 계층 직접 열람. 샘플 업링크 `packet.json` 실측.

## 1. 아키텍처: 2계층 (에이전트 + 플로우 노드)

### 1.1 에이전트 타입 레지스트리 및 부트스트랩

- 타입 레지스트리: `internal/agent/type_registry.go`, 매니저 `internal/agent/manager.go:476`(`RegisterType`) / `:94`(`Create`).
- 에이전트별 등록 함수 패턴(1개 함수 = 1개 타입 등록):
  - `internal/agent/system/mqtt_register.go:8-12` — `RegisterMQTTTypes` → `mgr.RegisterType("mqtt-client", NewMQTTAgent)`.
  - `internal/agent/samsung/register.go:8-12`, `internal/agent/century/registration.go:17` — 동일 패턴.
  - 실측 확인: `internal/agent/century/registration.go` 는 `RegisterHvacr01Types(mgr *agent.DefaultManager) error` 시그니처로 `mgr.RegisterType("century_hvacr01", func(config agent.AgentConfig) (agent.Agent, error) {...})` 반환.
- 부하가 걸리는(load-bearing) 호출 지점: `cmd/xflowd/main.go:360-413` (`// 5.1. 에이전트 타입 등록`).
  - 실측: `main.go:390-392` 에 `century.RegisterHvacr01Types(agentMgr)` 호출 존재. 신규 `RegisterChirpStackTypes(agentMgr)` 를 이 블록에 **반드시** 추가 + import 해야 인스턴스화 가능. (프로젝트 메모리 `project_agent_type_registration_bootstrap.md` 로 검증된 함정: 등록 함수를 정의만 하고 main.go 호출을 누락하면 타입은 영원히 생성 불가.)

### 1.2 에이전트 인터페이스 (필수 + 선택)

- 핵심 인터페이스 `internal/agent/agent.go:126-141`: `Init/Start/Stop/Pause/Resume/Health/Process/Configure/ID/Name/Type/Info/Stats`.
- 구현할 선택적 인터페이스(실측 `agent.go`):
  - `MessageReceiver` (`:25-27`, `ReceiveMessage(ctx) ([]byte, error)`) — **수신 경로**. ChirpStack 은 수신 전용.
  - `SubscriberAgent` (`:32-35`, `Subscribe/Unsubscribe`) — 동적 토픽 관리.
  - `StatefulAgent` (`:39-42`, `State() map[string]any`) — 런타임 상태 노출.
  - `TransportChecker` (`:46-48`, `TransportConnected() bool`) — 실제 브로커 연결 여부.
  - `BufferInfoProvider` (`:71-73`), `ConnectionStatsProvider` (`:97-99`), `SummaryStatsProvider` (`:113-115`) — 통계/관측성.
  - **미구현**: `MessagePublisher` (`:65-67`) — 수신 전용이므로 발행 인터페이스 불필요.

### 1.3 AgentConfig

- `internal/agent/config.go:16-30`: `BufferSize`(기본 1024), `HealthCheckInterval`(기본 30s), `Enabled`, `Transport.Options`(map — 에이전트별 설정 소스).
- 디바이스 목록 파서: `agent.ParseDevices(opts)` / `DeviceEntry` (`config.go:71-132`).

### 1.4 MQTT 트랜스포트 템플릿 (연결/구독/수신 통째 재사용)

- `internal/agent/system/mqtt_agent.go` 실측:
  - `MQTTConfig` `:21-58` — `Broker/ClientID/Username/Password/Topics/QoS/KeepAliveSec/AutoReconnect/CleanSession/BufferSize/ConnectTimeoutSec/MaxPubTopics`.
  - `parseMQTTConfig` `:68-126` — 기본값(broker `tcp://localhost:1883`, QoS 1, keep_alive 60s, auto_reconnect true, clean_session true) + `Transport.Options` 오버라이드.
  - `NewMQTTAgent` `:169`, `Init`(paho ClientOptions, OnConnect→subscribe) `:205-315`, `subscribe`(`application/#` 등 토픽 패턴) `:320-357`, `messageHandler`→`recvCh` `:432-448`, `ReceiveMessage` `:452-461`, `mqttTopicMatch` `:803-819`.
  - 재연결 부활(revival) 방지용 `stopped atomic.Bool` 가드 (`:146` 및 주석 `:138-145`) — ChirpStack 에도 동일 가드 채택 권장.

### 1.5 노드 계층 (수신 전용 SourceNode)

- 모델: `internal/node/mqtt.go` `MQTTSubNode` (`:185`, `initAgent` `:138`, `receiveLoop` `:272-314`).
- 신규 노드 등록: `internal/node/registry.go:73-136` 테이블에 `{"chirpstack-in", NewChirpStackInNode, "io", "..."}` 추가.
- 플로우 검증 화이트리스트: `pkg/flow/validate.go:140-150` 에 `chirpstack-in` 등록 필수.
- 패시브(제어 노드 없음) 패턴: Century/LG-HVACR01 (`registry.go:114-116`) 를 따른다.

## 2. 디바이스 관리

### 2.1 디바이스 모델 & 레지스트리

- 모델: `internal/device/device.go:33-101` — `Device` 인터페이스, `ID()==UID()==UUID v4`.
- 메타데이터: `DeviceMetadata` `:113-121` — `Name, Tags, Location, Group, Labels, Pinned`.
- Provider: `DeviceProvider` `:126-132` — `Devices()` / `Device(id)`.
- 레지스트리(pull-model, 저장하지 않고 Provider 를 실시간 순회): `internal/device/registry.go` — `RegisterProvider/UnregisterProvider/List/Get/GetByUID/SetMetadata/GetMetadata`.
- 자동 wiring: 에이전트 start 시 `cmd/xflowd/main.go:210-221` 에서 `DeviceProvider()` 인터페이스 어서션으로 자동 등록, stop 시 `:347-357` 에서 해제.

### 2.2 식별 (UID/UUID)

- `internal/agent/device_id_repo.go:28-89` — `DeviceIDRepository`, `ResolveDeviceID(ctx, agentName/ID, unitID) → UUID`. 키 `<agentName>:<unitID>`.
- 구현: `internal/storage/device_id_repository.go` (JSON 파일 `device_ids.json`).
- 이름→정규 ID 정규화: `internal/agent/agent_id_resolver.go:56-68`.
- 레지스트리는 첫 이름 등록을 유지: `internal/agent/registry.go:24-27` ⇒ **에이전트 이름은 고유해야 한다**.
- 런타임 타입/라벨: `internal/agent/device_info_repo.go` `SetDeviceInfo`.
- 어댑터 UID: `internal/device/adapter/uid.go:53-67` `ResolveAdapterUID`.

### 2.3 수신 시 자동 생성 템플릿

- Century: `touchDeviceFromDecoded` `internal/agent/century/agent.go:1744-1770`; Provider `internal/agent/century/provider.go`.
- emit-side `metadata.device` 그룹 조립은 노드 파이프라인 몫: `internal/node/dedup_helper.go` `mergeDeviceGroup` `:64-85` / `promoteDevIDWithUUID` `:148-170` (payload.unit_id + agentName 로부터).
  - ⇒ 에이전트는 `metadata.device` 를 손수 조립하지 말고 **`unit_id`(=devEui) 를 emit + DeviceInfo 등록**만 하면 된다.

### 2.4 관련 완료 SPEC

- SPEC-DEVICE-001 (완료, v1.1.0): 통합 디바이스 레지스트리, 발견 시 이벤트 구동 디바이스 등록.
- SPEC-DEVICE-IDENTITY-001 (완료, v0.3.0, Phase D): `UID=UUID v4`, `DeviceIDRepository` 필수, emit 은 `unit_id`+`device_id` 사용.

## 3. 메시지 발행 관용구 (`pkg/message`)

- 생성: `message.New(opts...)` (`message.go:122`).
- 페이로드: `Payload().Set` (`payload.go:57`).
- 메타데이터 그룹: `Metadata().SetGroup("device", map[string]string{...})` (`metadata.go:118`).
  - **제약**: 메타데이터 값은 엄격히 `string` 또는 `map[string]string` 만 허용.
- 타임스탬프: `WithTimestamp`/`SetTimestamp` — top-level `timestamp` 는 `time.Time` (influxdb-write 가 `timestamp_key:$.timestamp` 로 읽음).
- JSON 봉투: `{id, type, timestamp, payload, metadata}` (`json.go:10-18`).
- **페이로드 내 epoch 타임스탬프는 int64 UnixMilli** (프로젝트 컨벤션, 예: `last_seen_ms`). 프로젝트 메모리 `project_timestamp_convention.md` 로 검증.

## 4. 기존 ChirpStack 처리 (플로우 + Lua, 보존 대상 계약)

- **기존 Go 코드 없음.** 현재는 플로우 + Lua 스크립트로 처리.
- 활성 플로우 "조선대 실습실" (`9cb40f58`):
  `mqtt-subscriber(application/#)` → `script`(object→items[], devEui/deviceName→metadata.device, tags→metadata.tags) → `split($.payload.items[*], share_metadata)` → store/influx 서브플로우(`91bc2875`).
  → **이것이 보존해야 할 다운스트림 계약**이다.
- "ChirpStack to Store" 플로우 (`0286a6a8`) 는 flat-column 설계의 고아 — **SUPERSEDED, 재사용 대상 아님**.
- 샘플 업링크: `/Users/xtra/Projects/xflow/packet.json` (실측).

### 4.1 packet.json 실측 (WS301 도어 센서)

- `payload.deviceInfo`: `devEui="24e124141d180806"`, `deviceName="WS301-180806"`, `deviceProfileName="WS301"`, `applicationName="광주 캠퍼스"`.
- `payload.deviceInfo.tags`: **`{location:"실습실", point:"앞문", spot:"앞문"}` — 세 키 모두 존재** (packet.json:23-27).
- `payload.object`: `{magnet_status: "close"}` — **스칼라가 문자열** (숫자 아님). 매핑은 문자열/숫자 스칼라 모두 처리해야 함.
- `payload.time`: `"2026-08-11T23:32:01.129+00:00"` — 업링크 시각(top-level timestamp 소스).
- `payload.rxInfo[]`: 게이트웨이별 `rssi/snr/gatewayId` 배열 (2개 게이트웨이, rssi -113/-57, snr -9.5/13.5). comm-state 는 최적(예: 최대 rssi) 게이트웨이 선택 정책 필요.
- top-level `timestamp: 1786490870585` (수신측 UnixMilli).

### 4.2 point ↔ spot 불일치 (SPEC 기록 대상)

- **실측 불일치**: packet.json 의 `deviceInfo.tags` 는 `location`+`point`+`spot` 세 키를 모두 가진다.
- 그러나 동일 파일의 레거시 split 출력(packet.json:83-108, 2번째 JSON 객체)의 `metadata.tags` 는 `{location:"실습실", spot:"실습실"}` 로 **`point` 가 누락**되고 `spot` 값도 `location` 값으로 대체됨(Lua 매핑 아티팩트).
- 레거시 Lua 는 `tags.spot` 을 읽었다. 사용자 근거에는 "packet.json 에 spot 없음"으로 기술되었으나 **실측은 세 키 모두 존재**.
- **Pass-through 결정**에 따라 `deviceInfo.tags` 의 모든 키를 그대로 `metadata.tags` 에 전달한다. 키 매핑/정규화 없음. `point`/`spot` 선택은 다운스트림 소비자 몫. 본 불일치를 spec.md/acceptance.md 에 명시한다.

## 5. 통신 상태(comm-state) 모델

- SPEC-HVACR-CONNSTATE-001 은 **DEPRECATED (2026-07-13)** — 별도 `device_connection` 타입 폐기, `device_state` 스트림에 흡수.
- 라이브 구현 = Century (실측 `internal/agent/century/message.go:367-438`):
  - 트리거 상수 `TriggerChange="change"`, `TriggerReport="report"` (`:370-373`).
  - `Icp01DeviceStateInner` (`:385-397`): `online bool` + 도메인 핵심 필드 (state 그룹 내부).
  - `Icp01DeviceStateMetadata` (`:407-410`): `Label`(출력 키 `name`), `DeviceType`.
  - `Icp01DeviceStateEvent` (`:427-438`): `SubDevID`(출력 `unit_id`), `DeviceID`(출력 `device_id`), `Trigger`, `LastSeenMs int64`(UnixMilli), `State`, `Metadata`. 노드 단이 `metadata.message_type = "device_state.<trigger>"` 로 스키마 식별.
- `TransportConnected` at `agent.go:46-48`.
- **ChirpStack 캐비앗(SPEC 명시 필수)**: ChirpStack 은 패시브/푸시(폴 루프 없음, error_count 없음) ⇒ online/offline 은 실패 요청이 아니라 **업링크 staleness watchdog(last-seen 타이머)** 로만 추론. 에이전트가 자체 last-seen 추적기를 가져야 한다.

## 6. 조사 결론 (설계 입력 요약)

1. MQTT 트랜스포트는 `system/mqtt_agent.go` 통째 재사용 가능.
2. 에이전트는 수신 전용(`MessageReceiver`) + `DeviceProvider` + `TransportChecker`(+ 선택 통계).
3. 디바이스는 `devEui` 키잉 자동 생성, UID 는 `ResolveDeviceID`, 메타데이터는 registry `SetMetadata`.
4. emit 은 per-measurement, `unit_id`(=devEui) 포함 + DeviceInfo 등록으로 노드가 device 그룹 승격.
5. comm-state 는 `device_state` fold + staleness watchdog (자체 구현 필요, Century 와 트리거 의미 동일).
6. 등록 wiring 3곳: `register.go`(신규) + `main.go:360-413` 호출 + `node/registry.go` + `flow/validate.go`.
