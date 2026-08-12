---
id: SPEC-CHIRPSTACK-001
title: "ChirpStack LoRaWAN 에이전트 — 구현 계획"
version: 0.1.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: plan
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: L
tags: [chirpstack, lorawan, mqtt, agent, plan]
---

# ChirpStack 에이전트 — 구현 계획 (plan.md)

## 1. 기술 접근

- **트랜스포트 재사용**: `internal/agent/system/mqtt_agent.go` 의 연결/구독/수신/재연결 가드(`stopped atomic.Bool`)를 ChirpStack 에이전트에 이식. 발행 경로(MessagePublisher)는 제외.
- **디코드 분리**: ChirpStack 특화 로직(`object` fan-out, `rxInfo` 최적 게이트웨이 선택, `deviceInfo` 추출)을 `decode.go` 로 격리해 테스트 용이성 확보.
- **디바이스 경로**: `ResolveDeviceID` → `SetDeviceInfo` → registry `SetMetadata` → `DeviceProvider` roster upsert. emit 은 `unit_id` 만, device 그룹은 노드 승격(`dedup_helper.go`)에 위임.
- **comm-state**: Century `Icp01DeviceStateEvent` 스키마 모델링, 단 offline 판정은 staleness watchdog(자체 last-seen map + watchLoop + reportLoop)로 구현.
- **방법론**: Hybrid — 신규 파일 TDD(RED-GREEN-REFACTOR), 다운스트림 계약 보존은 characterization 테스트(packet.json 픽스처)로 회귀 방지.

## 2. 우선순위 기반 마일스톤

### Primary Goal (Priority High)

- **M1 — 에이전트 스켈레톤 + 등록 wiring**
  - `ChirpStackAgent` 구조체 + `agent.Agent` 13 메서드.
  - `registration.go` `RegisterChirpStackTypes`.
  - `cmd/xflowd/main.go:360-413` 호출 + import 추가 (load-bearing).
  - 이름 고유성 검증.
- **M2 — MQTT 연결/구독/수신**
  - `ChirpStackConfig` 파싱 (MQTTConfig 미러 + 추가 노브).
  - paho 연결, `application/#` 구독, `ReceiveMessage`, `TransportConnected`, stopped 가드.
- **M3 — 업링크 디코드 + per-measurement 발행**
  - `decode.go` object fan-out, 스칼라 판정, 비스칼라 skip.
  - `message.go` per-measurement event 빌더 (timestamp/value/measurement/tags).
  - `internal/node/chirpstack.go` `ChirpStackInNode` + `node/registry.go` + `flow/validate.go` 등록.
  - 다운스트림 계약 보존 검증.
- **M4 — 디바이스 자동 생성 + Provider + 메타데이터**
  - devEui 키잉 자동 생성, `ResolveDeviceID` UID.
  - `SetDeviceInfo`, registry `SetMetadata`(tags/name).
  - `provider.go` `DeviceProvider`.

### Secondary Goal (Priority Medium)

- **M5 — comm-state (optional, config-gated)**
  - `watchdog.go` last-seen map + watchLoop + reportLoop.
  - `device_state.change`/`report` emit, best-gateway rssi/snr/gateway_id.
  - `emit_comm_state`/`comm_report_interval`/`offline_threshold` 노브.
  - restart online 오탐 방지(unknown 보류).

### Final Goal (Priority High)

- **M6 — 테스트 + 품질 게이트**
  - 테이블 주도 테스트(packet.json 픽스처): decode/fan-out/device-create/tags/comm-state.
  - 커버리지 ≥85%, TRUST 5, LSP zero(run).
  - goroutine 누수 검증(context cancel).

### Optional Goal (Priority Low)

- 관측성 확장: `ConnectionStats`(gateway/topic 단위), `SummaryStats`(devicesTotal/online).

## 3. 아키텍처 설계 방향

- 단일 책임 파일 분할(agent/config/decode/message/provider/watchdog/registration).
- 노드는 수동 SourceNode(Century/LG-HVACR01 패시브 패턴) — 제어 노드 없음.
- 동시성: 수신 채널 버퍼링 + watchLoop/reportLoop 는 context 종료. 공유 map 은 mutex 보호.
- 프로젝트 메모리 준수: HVAC 락 재귀 RLock 함정(`project_hvac_agent_lock_pattern.md`) — lock-holding 함수 내 self-name 재조회 금지, UnixMilli 컨벤션.

## 4. 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| main.go 호출 누락 | 타입 인스턴스화 불가 | M1 DoD 에 wiring 3곳 검증 포함 |
| 다운스트림 계약 회귀 | 기존 store/influx 파이프 파손 | characterization 테스트(packet.json 골든) |
| 에이전트 이름 중복 | device_id collision | 이름 고유성 검증 + 문서화 |
| offline_threshold 부적합 | online 오탐/지연 | 보수적 기본값 + 디바이스 업링크 주기 문서화 |
| 비스칼라 object 값 | 발행 실패/오염 | 1차 skip + 경고, 정책 M3 확정 |
| goroutine 누수 | 리소스 누수 | context cancel 종료 테스트 |
| rxInfo 다중 게이트웨이 | rssi/snr 대표값 모호 | 최대 rssi 게이트웨이 선택 정책 |

## 5. 완료 조건 연결

- 모든 REQ-* 구현 + acceptance.md 시나리오 통과.
- TRUST 5 게이트 통과, 커버리지 ≥85%.
- `/moai:2-run SPEC-CHIRPSTACK-001` → `/moai:3-sync SPEC-CHIRPSTACK-001`.

## 6. 라이브러리 버전

- MQTT/디바이스/메시지 계층은 기존 저장소 의존을 그대로 사용(신규 외부 의존 없음). 세부 버전 확인은 `/moai:2-run` 에서 `go.mod` 기준 확정.
