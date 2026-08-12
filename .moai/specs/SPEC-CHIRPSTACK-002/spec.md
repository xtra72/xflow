---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드"
version: 1.0.0
status: completed
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

  > **[run-phase 해소]** 위 A6 가정의 "미검증" 기록은 플래닝 시점의 정직한 상태 기록으로 그대로 보존한다. run-phase 에서 다음과 같이 해소되었다.
  >
  > - 플래닝 중 관측된 HTTP 401 은 **재현되지 않았다**. 공식 Milesight WS301 User Guide V1.4(2026-04-22, 33p)를 `https://resource.milesight.com/milesight/iot/document/ws301-user-guide-en.pdf` 에서 정상 수신했고, `https://www.milesight.com/products/docs/en/ws301/protocol/ws301/ws301-downlink.html` 및 공식 `Milesight-IoT/SensorDecoders` 의 `ws301-encoder.js` 와 교차 대조했다.
  > - **검증됨**: 다운링크 fPort **85**(Guide p.31); TLV 형식 `0xFF <cmd> <value...>` 이며 멀티바이트 파라미터는 **리틀엔디언**(Guide p.29); reboot `ff 10 ff`; 보고 주기 설정 `ff 03 <lo> <hi>` UINT16 LE 초 단위(Guide Ch.6 p.31-32, 예시 `ff03b004` = 1200초); 유효 범위 60~64800초는 ToolBox 범위 1~1080분(p.17) 및 공식 encoder 검증 기준.
  > - **여전히 미검증**: `ff 28 ff`(query device status)는 공식 encoder 에만 존재하고 User Guide V1.4 및 milesight.com HTML 문서에는 미등재다. 구현은 하되 `internal/agent/chirpstack/codec_ws301.go:41-45` 에 `@MX:DEBT` + `@MX:CEILING` + `@MX:UPGRADE` 를 부착해 확신 수준 차이를 표기했다.
  > - WS301 은 자석식 도어 컨택 센서로 **부저/알람 하드웨어가 없어** 해당 다운링크 명령이 존재하지 않는다. 따라서 v1 코덱 노출면은 reboot + 보고 주기 설정(+ query, debt 표기)이다.

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

  > **[run-phase 해소]** 위 R2 위험 기록은 플래닝 시점의 정직한 상태 기록으로 그대로 보존한다. run-phase 전제조건은 **충족**되었다.
  >
  > - 플래닝 중의 HTTP 401 은 재현되지 않았고, 공식 Milesight WS301 User Guide V1.4(2026-04-22, 33p)를 `https://resource.milesight.com/milesight/iot/document/ws301-user-guide-en.pdf` 에서 정상 수신했다. `https://www.milesight.com/products/docs/en/ws301/protocol/ws301/ws301-downlink.html` 및 공식 `Milesight-IoT/SensorDecoders` 의 `ws301-encoder.js` 와 교차 대조를 마쳤다.
  > - 대조로 확정된 값: fPort **85**(p.31), TLV `0xFF <cmd> <value...>` + 파라미터 **리틀엔디언**(p.29), reboot `ff 10 ff`, 보고 주기 `ff 03 <lo> <hi>` UINT16 LE 초(Ch.6 p.31-32, 예시 `ff03b004`=1200초), 유효 범위 60~64800초(ToolBox 범위 1~1080분, p.17 + 공식 encoder 검증).
  > - **잔여 예외 1건**: `ff 28 ff`(query device status)는 공식 encoder 에만 있고 User Guide V1.4·milesight.com HTML 문서 어디에도 없다. 이 바이트에 한해 "검증됨" 을 주장하지 않으며 `codec_ws301.go:41-45` 의 `@MX:DEBT`/`@MX:CEILING`/`@MX:UPGRADE` 로 표기했다.
  > - 검증 범위 한계: 문서 대조 검증이며 **실기기 다운링크 수신 확인은 수행하지 않았다**.
- **R1**: 발행 경로 역전(REQ-M1-02) — SPEC-001 의 의도적 배제를 역전하므로 agent.go:27 주석 + 인터페이스-체크 블록 갱신을 명시 기록(무회귀 오인 방지).
- **R4**: comm 맵 락 안전 — `commMu` + `agentName` pre-capture(REQ-FROZEN-B/M3-03), HVAC 재귀 RLock deadlock 트랩 회피.
- **R5**: `applicationId` 캐시 의존 — 제어 노드는 최초 업링크가 `applicationId` 를 캐시하기 전에는 토픽 구성 불가(REQ-M2-05, 순서 제약).

## Implementation Notes (run-phase 실측)

> 구현 커밋 `d5d0089d` (feature/SPEC-CHIRPSTACK-002) 기준 as-built 계약. 플래닝 기록과 다르거나
> 플래닝에 없던 항목만 기록한다.

### IN-1. control 노드 입력 계약 (사용자 결정)

대상 디바이스는 **설정이 아니라 입력 메시지**로 지정한다. `msg.Payload()` 에서 typed command 를 읽는다:

- `unit_id` (폴백 `device_id`, 그 다음 metadata 의 동일 2개 키) — 대상 devEui.
- `command` (string), `params` (map), `confirmed` (bool, 선택·기본 false).

노드 1개 인스턴스가 N 개 디바이스를 담당한다. `internal/node/lgap.go` / `xsfm.go` 의 기존 제어 노드 관용구를 미러한다. 구현: `internal/node/chirpstack_control.go` `chirpStackExtractDevEui` / `chirpStackExtractCommand` / `chirpStackExtractParams` / `chirpStackExtractConfirmed`.

### IN-2. status 노드 키잉 (사용자 결정)

status 노드도 동일하게 payload `unit_id` 로 대상 devEui 를 지정하며, 트리거 1건당 메시지 1건을 방출한다(`internal/node/chirpstack_status.go` `Process`).

### IN-3. R6 — comm 맵은 `emit_comm_state` 게이트에 종속 (신규 식별 위험, 원 SPEC 미기재)

comm 맵은 에이전트의 `emit_comm_state` 가 true 일 때만 채워진다(기본값 false). 따라서 이 노브가 꺼진 에이전트에 붙은 status 노드는 **항상 offline/unknown 을 방출**한다. 원 SPEC 은 이 전제조건을 기술하지 않았다.

선택한 해소책은 **REQ-M3-01 엄격 준수 + 전제조건 공개**이다:

- 방출 소스는 REQ-M3-01 문구 그대로 comm 맵을 유지한다.
- 전제조건을 registry 설명 문자열(`internal/node/registry.go`: "에이전트 emit_comm_state=true 필요")과 노드 Go doc 양쪽에 명시한다.
- `Init` 에서 `CommStateEnabled()` 를 확인해 **경고 1회**를 남긴다(하드 실패시키지 않는다).
- `devices` 로스터 폴백은 도입하지 않으며, 하드 실패도 시키지 않는다.

### IN-4. REQ-M1-03(Optional) — `SendDownlink` 에이전트 메서드 없이 충족

typed helper 를 에이전트 메서드로 추가하지 않고, 패키지 레벨 **순수 함수** `BuildDownlinkTopic` / `BuildDownlinkPayload` 를 제공한다. 노드가 이 둘로 토픽·페이로드를 조립한 뒤 `PublishMessage` 를 직접 호출한다. 근거: `internal/node/mqtt.go` 와 동일한 thin-adapter 원칙 — 에이전트에 노드 전용 편의 메서드를 쌓지 않는다.

### IN-5. `applicationId` 캐시 위치 — `devices` 맵 (comm 맵 아님)

`applicationId` 는 업링크 `deviceInfo.applicationId` 에서 `upsertDevice` 를 통해 **`devices` 맵**에 캐시한다. comm 맵에 두지 않은 이유: comm 맵 갱신은 `EmitCommState` 게이트에 종속되므로, 거기에 캐시하면 제어 경로가 그 노브에 숨은 의존성을 갖게 된다(IN-3 과 동일한 함정). 조회 접근자는 `DownlinkTarget(devEui) (applicationID, deviceProfileName, ok)`.

### IN-6. 패키지 경계로 인한 export 3건 + 접근자 2건

`internal/node` 가 패키지 경계를 넘어 소비하므로 다음 3개 식별자를 export 한다: `BuildDownlinkTopic`, `BuildDownlinkPayload`, `DownlinkTarget`.

동일한 이유로 접근자 2개를 추가했다:

- `CommStateRecordJSON` — 레코드 조립을 에이전트 패키지 안에 유지해 방출 shape 빌더가 **정확히 하나**가 되도록 한다(수신 경로와 갈라진 2차 구현 방지). `commEntry` 가 패키지 비공개 타입이라 노드가 직접 다룰 수 없다.
- `CommStateEnabled` — IN-3 의 경고에 필요.

### IN-7. `a.client` 대입을 `a.mu.Lock()` 하로 이동

`internal/agent/chirpstack/agent.go` `connect()` 의 `a.client` 대입은 기존에 비동기화 상태였다. 발행 경로가 노드 goroutine 에서 호출될 수 있게 되면서 선재하던 비동기화 쓰기가 **실제 데이터 레이스**가 되었으므로 `a.mu.Lock()` 하로 옮겼다(동작 보존 최소 수정).

### IN-8. base64 는 `StdEncoding`

ChirpStack v4 소스 대조로 확정: `api/proto/integration/integration.proto` 의 `DownlinkCommand`, `chirpstack/src/integration/mqtt.rs`, `influxdata/pbjson` 의 `lib.rs` — pbjson 은 표준 알파벳을 1순위로 시도하고 `-`/`_` 를 만났을 때만 URL-safe 로 fallback 한다. 따라서 `StdEncoding` 은 fallback 에 의존하지 않는 경로다.

주의: ChirpStack 파서는 `ignore_unknown_fields` 이므로 **필드명 오타가 에러 없이 무시되고 기본값이 쓰인다**. 필드명(`devEui`/`confirmed`/`fPort`/`data`)은 golden-vector 테스트로 고정했다. **실브로커 대조는 수행하지 않았다.**

### IN-9. 노드 타입 수 71 → 73, 하드코딩 단언 3곳 (플래닝 예상 1곳)

`chirpstack-control` + `chirpstack-status` 추가로 노드 타입 수가 71 → 73(59 canonical + 14 alias)이 되었다. plan.md 는 갱신 대상을 1곳으로 예상했으나 실제로는 **3곳**의 하드코딩 단언을 갱신해야 했다:

- `internal/api/service/node_adapter_test.go`
- `internal/node/registry_test.go`
- `internal/node/mqtt_test.go`

## Traceability (추적성)

- plan.md: 기술 접근, 우선순위 마일스톤, 위험 대응.
- acceptance.md: Given-When-Then(AC-1~AC-5) + TRUST 5 + DoD.
- 선행 SPEC: SPEC-CHIRPSTACK-001 (completed, 수신 에이전트 기반).
- 관련 SPEC: SPEC-DEVICE-001(통합 레지스트리), SPEC-DEVICE-IDENTITY-001(Phase D, ID==UID==UUID v4) — SPEC-001 계약과 동일 승계.
