---
id: SPEC-CHIRPSTACK-001
title: "ChirpStack LoRaWAN 에이전트 — 기술 설계"
version: 0.1.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: design
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: L
tags: [chirpstack, lorawan, mqtt, agent, design]
---

# ChirpStack 에이전트 — 기술 설계 (design.md)

> 근거: research.md. 구현 코드는 본 문서에 포함하지 않으며, `/moai:2-run` 에서 작성한다.

## 1. 2계층 컴포넌트 설계

### 1.1 개요

```
[MQTT Broker] --application/#--> [chirpstack agent] --recvCh--> [chirpstack-in node] --> store/influx consumers
                                       |
                                       +--> DeviceProvider() --> device registry (자동 wiring)
                                       +--> device_state.<trigger> (comm-state, optional)
```

- **에이전트 계층** (`internal/agent/chirpstack/`): MQTT 연결/구독/수신, 업링크 디코드, per-measurement 메시지 생성, 디바이스 자동 생성/메타데이터 지속화, comm-state watchdog.
- **노드 계층** (`internal/node/chirpstack.go`): 수신 전용 SourceNode. 에이전트 `ReceiveMessage` 를 `receiveLoop` 로 소비, 다운스트림 포트로 방출. Century/LG-HVACR01 패시브 패턴.

### 1.2 에이전트 파일 구성 (제안)

| 파일 | 책임 |
|------|------|
| `agent.go` | `ChirpStackAgent` 구조체, 라이프사이클(Init/Start/Stop), `MessageReceiver`/`TransportChecker`/`StatefulAgent` 구현 |
| `config.go` | `ChirpStackConfig` 파싱 (`MQTTConfig` 미러 + 추가 노브) |
| `decode.go` | ChirpStack 업링크 JSON → 내부 구조체 디코드, `object` fan-out |
| `message.go` | per-measurement event + `device_state` 이벤트 구조체/빌더 |
| `provider.go` | `DeviceProvider` 구현 (roster 노출) |
| `watchdog.go` | last-seen 추적기 + staleness/offline 판정 + report 타이머 |
| `registration.go` | `RegisterChirpStackTypes(mgr)` |
| `*_test.go` | 테이블 주도 테스트 (packet.json 픽스처) |

### 1.3 구현할 인터페이스

- 필수: `agent.Agent` (13 메서드).
- 선택: `MessageReceiver`(수신 경로), `SubscriberAgent`(동적 토픽), `TransportChecker`(브로커 연결), `StatefulAgent`(roster/last-seen 상태), `DeviceProvider`(디바이스 노출), 선택적 `ConnectionStatsProvider`/`SummaryStatsProvider`.
- 미구현: `MessagePublisher` (수신 전용).

## 2. 업링크 → per-measurement 메시지 매핑

### 2.1 프로즌 결정 1 반영

`object` 를 measurement 당 1개 메시지로 fan-out. 각 메시지는 기존 Lua `script`+`split` 파이프라인을 **대체**하며, `chirpstack-in` 노드가 store/influx 소비자에 직접 연결된다. 다운스트림 계약을 정확히 보존한다.

### 2.2 메시지 스키마 (measurement 당 1개)

| 필드 | 값 | 소스 |
|------|-----|------|
| `type` | `"event"` | 고정 |
| `timestamp` (top-level) | 업링크 시각 | `payload.time` → `WithTimestamp` (time.Time) |
| `payload.value` | 스칼라 | `object.<key>` (숫자 또는 문자열) |
| `metadata.measurement` | `<key>` | `object` 의 키 (SetGroup 아님, 단일 문자열 값) |
| `metadata.device` | `{id, name}` | **직접 조립 금지** — `unit_id`(=devEui) emit + DeviceInfo 등록으로 노드가 승격 |
| `metadata.tags` | `{...}` | `deviceInfo.tags` verbatim pass-through |

### 2.3 다운스트림 계약 (보존 대상 — 변경 금지)

기존 소비자가 읽는 경로:
- `$.payload.value`
- `$.metadata.measurement`
- `$.metadata.device.id`, `$.metadata.device.name`
- `$.metadata.tags.*`
- `$.timestamp`

### 2.4 매핑 규칙 상세

- `object` 순회: 각 키-값 쌍마다 1개 메시지.
- 값 타입: 숫자(float/int) 및 문자열 스칼라 모두 허용 (packet.json 실측 `magnet_status:"close"` 는 문자열). 중첩 객체/배열 값 처리 정책은 M3 에서 결정(1차: 스칼라만 방출, 비스칼라는 skip + 경고 로그).
- `unit_id` = `deviceInfo.devEui`. `device_id` 은 emit 하지 않아도 노드가 `promoteDevIDWithUUID`(unit_id + agentName)로 UUID 승격.
- `metadata.measurement` 는 SetGroup 이 아닌 단일 문자열 메타 값.
- `metadata.tags` 는 `SetGroup("tags", map[string]string)` — pass-through.

### 2.5 packet.json 예시 (WS301)

입력 `object = {magnet_status:"close"}` → 1개 메시지:
```
type=event, timestamp=2026-08-11T23:32:01.129Z
payload={value:"close"}
metadata: measurement=magnet_status, device={id:24e124141d180806(→UUID 승격), name:WS301-180806},
          tags={location:실습실, point:앞문, spot:앞문}
```

## 3. comm-state (device_state fold) 설계

### 3.1 프로즌 결정 2 반영

별도 `device_connection` 타입 아님(SPEC-HVACR-CONNSTATE-001 폐기). `device_state.<trigger>` 로 emit. Optional, config-gated. ChirpStack 은 패시브이므로 offline 은 **업링크 staleness watchdog** 로만 추론.

### 3.2 메시지 스키마 (Century Icp01DeviceStateEvent 모델링)

봉투(top-level metadata via 노드 승격 + emit):
| 필드 | 타입 | 소스 |
|------|------|------|
| `unit_id` | string | devEui |
| `device_id` | string (UUID) | 노드 승격 |
| `trigger` | enum `{change, report}` | watchdog |
| `last_seen_ms` | int64 (UnixMilli) | 마지막 업링크 수신 시각 |
| `state.online` | bool | watchdog 판정 |
| `state.rssi` | int | rxInfo 중 최적 게이트웨이 rssi |
| `state.snr` | float | 최적 게이트웨이 snr |
| `state.gateway_id` | string | 최적 게이트웨이 gatewayId |
| `state.last_seen_ms` | int64 (UnixMilli) | (state 내부 중복 노출 — Century 관례) |
| `metadata.name` | string | deviceName |
| `metadata.device_type` | string | deviceProfileName (예: `WS301`) |

노드 단 `metadata.message_type = "device_state.<trigger>"` 로 스키마 식별.

### 3.3 트리거 의미

- `change`: online 전이(online↔offline) 또는 핵심 state 필드 유의미 변화 시 즉시 emit.
- `report`: 변화 없이 `comm_report_interval` 경과 시 주기적 상태 보고. `comm_report_interval = 0` 이면 주기 report off (change 이벤트는 계속 흐름).

### 3.4 업링크-staleness watchdog 설계

- **last-seen 추적기**: devEui 별 마지막 업링크 UnixMilli 를 map 으로 유지(mutex 보호). 업링크 수신마다 갱신 + online 전이 검사.
- **offline 판정**: `now - last_seen_ms > offline_threshold` 이면 offline 으로 전이 → `change` emit. 폴 루프/error_count 없음 — 실패 요청 기반 아님.
- **watchLoop**: 주기 타이머(예: offline_threshold/2 또는 고정 tick)로 각 디바이스 staleness 검사, offline 전이 감지.
- **reportLoop**: `comm_report_interval > 0` 시 주기적 `report` emit.
- 재연결/재시작 시 last-seen 리셋 정책: 첫 업링크 수신 전까지는 online 판정 보류(unknown) — 오탐 방지.
- @MX:WARN 후보: watchLoop/reportLoop goroutine 은 context 취소로 종료 보장 필요.

## 4. 디바이스 라이프사이클

### 4.1 프로즌 결정 3 반영

- 수신 시 `devEui` 키로 디바이스 자동 생성 (devEui = unit_id/localID, UID 아님).
- 시스템 UID(UUID v4) 발급/조회: `agent.ResolveDeviceID(ctx, agentID, devEui)`.
- 런타임 타입/라벨 등록: `agent.SetDeviceInfo(agentID, devEui, {DeviceType: deviceProfileName, Label: deviceName})`.
- roster 노출: `DeviceProvider()` → `cmd/xflowd/main.go:210-221` 자동 등록.
- 메타데이터 지속화: `deviceName → Name/Label`, `tags(location/point/spot) → DeviceMetadata` via registry `SetMetadata` (UUID 키잉).

### 4.2 흐름

```
업링크 수신
  → devEui 추출
  → ResolveDeviceID(ctx, agentID, devEui) → UUID (없으면 신규 발급)
  → SetDeviceInfo(agentID, devEui, {DeviceType, Label})
  → registry.SetMetadata(UUID, {Name, Tags, ...})
  → roster upsert (DeviceProvider 노출)
  → per-measurement emit (unit_id=devEui)
```

### 4.3 준수 SPEC & 제약

- SPEC-DEVICE-001: 통합 디바이스 레지스트리, 에이전트가 발견 디바이스 등록.
- SPEC-DEVICE-IDENTITY-001 Phase D: `Device.ID()==UID()==UUID v4`; emit payload 는 `unit_id + device_id`.
- **제약**: 에이전트 이름 고유. device_id 키잉이 name→canonical agent ID 정규화(`agent_id_resolver.go:56-68`)에 의존 — 이름 중복 시 branch/collision.

## 5. 설정 스키마 (`ChirpStackConfig`)

`MQTTConfig` 를 미러 + ChirpStack 전용 추가. `Transport.Options` map 에서 파싱.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|--------|------|
| `broker` | string | `tcp://localhost:1883` | MQTT 브로커 |
| `client_id` | string | `xflow-<uuid>` | 클라이언트 ID |
| `username` | string | `""` | 인증 |
| `password` | string | `""` | 인증 |
| `topics` | []string | `["application/#"]` | 구독 토픽 |
| `qos` | byte | 1 | QoS |
| `keep_alive_sec` | int | 60 | keep-alive |
| `auto_reconnect` | bool | true | 자동 재연결 |
| `clean_session` | bool | true | 클린 세션 |
| `connect_timeout_sec` | int | 10 | 연결 타임아웃 |
| `buffer_size` | int | 1024 | 수신 버퍼 |
| `devices` | []DeviceEntry | `[]` | 사전 등록 디바이스(선택, `ParseDevices`) |
| `emit_comm_state` | bool | false | comm-state emit 게이트 |
| `comm_report_interval` | duration/int(s) | 0 | 주기 report 간격 (0=off, change 는 유지) |
| `offline_threshold` | duration/int(s) | (예) 300 | staleness→offline 임계 |

- comm-state 관련 3키(`emit_comm_state`, `comm_report_interval`, `offline_threshold`)가 프로즌 결정 2 노브.
- 파싱은 `parseMQTTConfig` 관용구(타입 어서션 + 기본값) 재사용.

## 6. 등록 wiring 체크리스트 (Tier L 정찰 근거)

| # | 위치 | 작업 |
|---|------|------|
| 1 | `internal/agent/chirpstack/registration.go` (신규) | `RegisterChirpStackTypes(mgr) error` → `mgr.RegisterType("chirpstack", NewChirpStackAgent)` |
| 2 | `cmd/xflowd/main.go:360-413` | `chirpstack.RegisterChirpStackTypes(agentMgr)` 호출 + import 추가 (**누락 시 인스턴스화 불가**) |
| 3 | `cmd/xflowd/main.go:210-221` | `DeviceProvider()` 자동 wiring — 인터페이스 어서션이므로 별도 코드 불필요(구현만 하면 됨) 확인 |
| 4 | `internal/node/chirpstack.go` (신규) | `ChirpStackInNode` (SourceNode, receiveLoop) |
| 5 | `internal/node/registry.go:73-136` | `{"chirpstack-in", NewChirpStackInNode, "io", "..."}` 추가 |
| 6 | `pkg/flow/validate.go:140-150` | `chirpstack-in` 화이트리스트 추가 |

## 7. 관측성 / 통계 (선택)

- `TransportConnected`: paho `IsConnected` 위임.
- `SummaryStats`: `devicesTotal`, `messagesReceived`, `onlineDevices` 등 안정 key.
- `ConnectionStats`: gatewayId 또는 topic 단위 통계(선택, MVP 이후).

## 8. 미해결/위험 (SPEC 반영)

- 비스칼라 `object` 값(중첩 객체/배열) 처리 정책 — 1차 스칼라 only.
- rxInfo 다중 게이트웨이 중 comm-state 대표 게이트웨이 선택 정책(최대 rssi 제안).
- offline_threshold 기본값 — LoRaWAN 클래스 A 디바이스 업링크 주기에 의존(디바이스별 상이). 보수적 기본값 필요.
- 재시작 후 last-seen 유실 시 online 오탐 — unknown 보류로 완화.
