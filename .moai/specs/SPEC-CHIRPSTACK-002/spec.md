---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드"
version: 0.1.0
status: draft
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: spec
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, downlink, control, status, node, codec, milesight-ws301]
---

# SPEC-CHIRPSTACK-002: ChirpStack status/control 노드

> 선행 SPEC-CHIRPSTACK-001(completed)의 수신 전용 에이전트 위에, LoRaWAN **다운링크(제어)** 와
> **캐시 상태 조회** 를 Flow 노드로 노출한다. 제어 노출면은 **Flow 노드 전용**이며
> control-panel ControllableDevice 어댑터 / REST `/execute` 경로는 범위 밖이다.

## Environment (환경)

- 언어/런타임: Go 1.23+ 모듈 `github.com/xtra/xflow`.
- 확장 대상 패키지: `internal/agent/chirpstack/` (에이전트 발행 프리미티브 + 코덱 레지스트리), `internal/node/chirpstack.go` (신규 control/status 노드).
- 재사용 기반:
  - `internal/agent/system/mqtt_agent.go:609` `PublishMessage(topic,qos,retained,payload)` — 발행 가드 패턴(연결/토픽 검증) 미러 소스.
  - `internal/agent/agent.go:62-67` `MessagePublisher` 선택적 인터페이스.
  - `internal/node/chirpstack.go:251` `buildChirpStackDeviceStateMessage` — status 노드 방출 shape 소스.
  - SPEC-CHIRPSTACK-001 M5 comm-state 추적 맵(`comm map[string]*commEntry`, `commMu sync.Mutex`).
- 외부 의존: ChirpStack MQTT 다운링크 토픽 `application/{applicationId}/device/{devEui}/command/down`, paho MQTT 클라이언트(기존 `a.client` 재사용).
- 개발 방법론: Hybrid (신규 코드 TDD, SPEC-001 흡수부 DDD), 신규 코드 커버리지 목표 85%.

## Assumptions (가정)

- A1. ChirpStack 다운링크 명령은 토픽 `application/{applicationId}/device/{devEui}/command/down` 에 JSON `{devEui, confirmed, fPort, data(base64)}` 페이로드로 발행하면 큐잉된다.
- A2. 디바이스별 `applicationId` 는 업링크 `deviceInfo.applicationId` 에서 관측 가능하며, 제어 노드는 최초 업링크 이후에만 토픽을 구성할 수 있다(순서 제약, R5).
- A3. 다운링크 인코딩은 deviceProfile 별로 다르므로 per-deviceProfile 코덱으로 분기한다. v1 은 Milesight WS301 코덱 seed 만 구현하며 레지스트리는 확장 가능하다.
- A4. `confirmed` 플래그는 다운링크 페이로드로 통과되나, ChirpStack 다운링크 큐(FIFO)/confirmed-ack 처리는 v1 범위 밖이다(fire-and-publish).
- A5. status 노드는 온디맨드 폴 없이 에이전트 comm 맵의 마지막 캐시 상태만 방출한다.
- **A6 (R2, 미검증)**. Milesight WS301 다운링크 TLV 바이트/fPort 는 **공식 문서 미확인** 상태이다(플래닝 중 공식 WS301 다운링크 유저 가이드가 HTTP 401 반환). 본 SPEC 은 검증된 바이트 값을 주장하지 않는다 — 정확한 바이트는 run-phase 에서 실제 문서 대조로 검증되어야 하며, 그 전까지 WS301 테스트는 문서-유도 인코딩에 대한 golden-vector 기반이다.

## Requirements (EARS)

### 프로즌 캐리오버 — SPEC-CHIRPSTACK-001 (변경 금지)

- REQ-FROZEN-A (수신 계약 보존): The system **shall not** change SPEC-001 의 per-measurement fan-out 및 다운스트림 읽기 경로 `$.payload.value`, `$.metadata.measurement`, `$.metadata.device.*`, `$.metadata.tags.*`, `$.timestamp`, device_state fold, tags verbatim pass-through, epoch=`int64 UnixMilli`.
- REQ-FROZEN-B (락 트랩 회피): While a chirpstack 에이전트 함수가 lock(`commMu` 등)을 보유 중이면, the system **shall** pre-capture `agentName`(a.Name()) before entering the locked section — HVAC 재귀 RLock deadlock 트랩(v0.18.6) 재현 금지. 또한 에이전트 이름 고유성(device_id 키잉 전제)을 유지한다.

### M1 — 발행 프리미티브 (에이전트 MessagePublisher)

- REQ-M1-01 (Ubiquitous): The `ChirpStackAgent` **shall** implement the `agent.MessagePublisher` interface `PublishMessage(topic, qos, retained, payload)`, mirroring the guards of `mqtt_agent.go:609` (nil/미연결 클라이언트 거부, 빈 토픽 거부), reusing the existing `a.client`.
- REQ-M1-02 (Ubiquitous, 명시적 역전): The system **shall** intentionally reverse SPEC-CHIRPSTACK-001 의 의도적 발행 경로 배제. 구체적으로 `internal/agent/chirpstack/agent.go:27` 의 "발행 경로(MessagePublisher)는 구현하지 않는다" 주석과 인터페이스-체크 블록(`var _` 계열)을 갱신하여 발행을 활성화한다. WHY: SPEC-001 은 수신 전용을 의도적으로 배제했고, 본 SPEC 이 그 배제를 역전한다. IMPACT: 미기록 시 SPEC-001 대비 회귀로 오인된다.
- REQ-M1-03 (Optional): Where a typed helper simplifies control-node code, the system **shall** provide an optional `SendDownlink(applicationID, devEui, cmd)` helper that composes the downlink topic + payload and delegates to `PublishMessage`.
- REQ-M1-04 (State-driven): While the agent is disconnected/stopped, the system **shall not** publish — `PublishMessage` 는 미연결 시 에러를 반환하고 발행하지 않는다(발행 가드).

### M2 — control 노드 + 코덱 레지스트리

- REQ-M2-01 (Ubiquitous): The system **shall** define a pluggable per-deviceProfile codec registry with interface `Encode(cmd) (fPort uint8, data []byte, confirmed bool, err error)`, extensible by registering additional deviceProfile codecs.
- REQ-M2-02 (Ubiquitous): The system **shall** implement a Milesight WS301 seed codec as the only v1 codec (문서-유도 golden-vector 인코딩; 정확 바이트 검증은 run-phase, A6/R2).
- REQ-M2-03 (Event-driven): When the `chirpstack-control` node receives a typed command input, the system **shall** resolve `devEui` + the cached `applicationId` + the typed command, encode via the deviceProfile codec, and publish `{devEui, confirmed, fPort, data(base64)}` to `application/{applicationId}/device/{devEui}/command/down`.
- REQ-M2-04 (Unwanted): If the deviceProfile has no registered codec, or the command is unknown to the codec, then the system **shall** return an error and **shall not** publish — 일반(generic) passthrough 다운링크 금지.
- REQ-M2-05 (State-driven): While no uplink has yet cached the device's `applicationId`, the system **shall not** be able to compose the downlink topic and **shall** return an error (순서 제약, R5). `applicationId` 는 업링크 `deviceInfo.applicationId` 에서 디바이스별 캐시한다.

### M3 — status 노드 (캐시 상태 방출)

- REQ-M3-01 (Event-driven): When the `chirpstack-status` node receives an input/trigger, the system **shall** emit the last-known cached comm-state (`online, rssi, snr, gateway_id, last_seen_ms`) from the agent comm map, mirroring the output shape of `buildChirpStackDeviceStateMessage` (`internal/node/chirpstack.go:251`).
- REQ-M3-02 (Unwanted): The status node **shall not** perform any on-demand poll or MQTT publish — 캐시 읽기 전용.
- REQ-M3-03 (Ubiquitous): The system **shall** read the comm map under `commMu` with a pre-captured `agentName` (REQ-FROZEN-B, R4).
- REQ-M3-04 (State-driven): While a device has no cached comm entry (예: 재시작 직후 last-seen 부재), the system **shall** emit `offline`/`unknown` state and **shall not** report `online` prematurely — SPEC-001 REQ-M5-06 과 일관(선정된 동작: offline/unknown 방출).

### M4 — wiring

- REQ-M4-01 (Ubiquitous): The system **shall** register `chirpstack-control` and `chirpstack-status` node types in `internal/node/registry.go` (near `:99`, category `"io"`, `chirpstack-in` 등록 라인 인접).
- REQ-M4-02 (Ubiquitous): The system **shall** add both `chirpstack-control` and `chirpstack-status` to `pkg/flow/validate.go:139` `agentRefRequiredTypes` (agent_ref 필수 검증 활성화; mqtt-subscriber/publisher 패턴).
- REQ-M4-03 (Ubiquitous): The frontend node palette **shall** surface both nodes automatically via the `/nodes` endpoint — web 편집 불필요.
- REQ-M4-04 (Unwanted): The system **shall not** add new config.go knobs and **shall not** change web `CHIRPSTACK_FIELDS`. `applicationId` 는 에이전트의 디바이스별 캐시에서 읽고, `fPort` 는 코덱이 결정하며, `confirmed` 는 기본값 false 이다.

### M5 — 테스트 + 품질 게이트

- REQ-M5-01 (Ubiquitous): The system **shall** provide table-driven tests covering: WS301 typed command → 정확한 `command/down` 토픽+페이로드(golden-vector), status 캐시 방출(0 publish/0 poll), unknown-codec 거부(0 publish), SPEC-001 무회귀 characterization, 미연결 시 발행 가드.
- REQ-M5-02 (Ubiquitous): The system **shall** achieve ≥85% coverage on new code and pass TRUST 5 gates (LSP run-phase: zero errors/type/lint).
- REQ-M5-03 (Ubiquitous): The WS301 golden-vector tests **shall** be documented as doc-derived and pending run-phase byte verification against the real Milesight WS301 user guide (A6/R2).

## Specifications (사양 상세)

### 다운링크 페이로드 계약

- 토픽: `application/{applicationId}/device/{devEui}/command/down`.
- 페이로드 JSON: `{"devEui": <string>, "confirmed": <bool=false 기본>, "fPort": <uint8, 코덱 결정>, "data": <base64(TLV bytes)>}`.
- `confirmed` 는 페이로드로 통과되나 큐/ack 처리는 v1 미구현(A4, fire-and-publish).

### 코덱 레지스트리 계약

- 인터페이스: `Encode(cmd) (fPort uint8, data []byte, confirmed bool, err error)`.
- 키: deviceProfile(예: WS301 의 `deviceProfileName`). 미등록 프로파일 또는 미지 command → error, no publish(REQ-M2-04).
- v1 seed: Milesight WS301 만. 레지스트리는 후속 deviceProfile 코덱 추가로 확장 가능.

### status 방출 shape (buildChirpStackDeviceStateMessage 미러)

- `payload.state = {online:bool, rssi:int, snr:float, gateway_id:string, last_seen_ms:int64(UnixMilli)}`, top-level `last_seen_ms`, `unit_id`(=devEui) → device 그룹 승격.
- missing-entry: `online=false`/unknown 방출(REQ-M3-04).

### 마일스톤 우선순위

- Priority High: M1(발행 프리미티브), M2(control 노드 + 코덱), M4(wiring).
- Priority Medium: M3(status 노드).
- Priority Low: —.
- 의존: M1 완료 → M2; M3 는 M4 wiring 과 병행; M5 는 전 구간 병행.

## Risks (위험 — 정직 기록)

- **R2 (Critical, 미검증)**: Milesight WS301 코덱 바이트/fPort 미확인(A6). 공식 WS301 다운링크 유저 가이드가 플래닝 중 HTTP 401. run-phase 구현자는 실제 문서로 정확 바이트/fPort 를 검증해야 하며, 그 전까지 WS301 테스트는 문서-유도 golden-vector 기반이다. **run-phase 전제조건**: 실 문서 대조 검증 없이 "검증된 바이트" 주장 금지.
- **R1**: 발행 경로 역전(REQ-M1-02) — SPEC-001 의 의도적 배제를 역전하므로 agent.go:27 주석 + 인터페이스-체크 블록 갱신을 명시 기록(무회귀 오인 방지).
- **R4**: comm 맵 락 안전 — `commMu` + `agentName` pre-capture(REQ-FROZEN-B/M3-03), HVAC 재귀 RLock deadlock 트랩 회피.
- **R5**: `applicationId` 캐시 의존 — 제어 노드는 최초 업링크가 `applicationId` 를 캐시하기 전에는 토픽 구성 불가(REQ-M2-05, 순서 제약).

## Traceability (추적성)

- plan.md: 기술 접근, 우선순위 마일스톤, 위험 대응.
- acceptance.md: Given-When-Then(AC-1~AC-5) + TRUST 5 + DoD.
- 선행 SPEC: SPEC-CHIRPSTACK-001 (completed, 수신 에이전트 기반).
- 관련 SPEC: SPEC-DEVICE-001(통합 레지스트리), SPEC-DEVICE-IDENTITY-001(Phase D, ID==UID==UUID v4) — SPEC-001 계약과 동일 승계.
