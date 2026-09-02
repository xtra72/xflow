---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드 — 구현 계획"
version: 1.0.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: plan
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, downlink, control, status, node, codec, milesight-ws301]
---

# ChirpStack status/control 노드 — 구현 계획 (plan.md)

## 1. 기술 접근

- **발행 프리미티브 이식**: `mqtt_agent.go:609` `PublishMessage` 의 연결/토픽 가드를 `ChirpStackAgent` 에 이식하고 기존 `a.client` 를 재사용한다. SPEC-001 의 수신 전용 배제(agent.go:27 주석 + 인터페이스-체크 블록)를 명시적으로 역전한다(REQ-M1-02).
- **코덱 분리**: deviceProfile 별 다운링크 인코딩을 `Encode(cmd) (fPort, data, confirmed, err)` 인터페이스로 격리하고, per-deviceProfile 레지스트리로 분기한다(플러그형). v1 은 Milesight WS301 seed 코덱만 등록한다. 미등록/미지 command 는 error → no publish(REQ-M2-04).
- **applicationId 캐시**: 업링크 `deviceInfo.applicationId` 를 디바이스별로 캐시하고, control 노드는 이 캐시에서 토픽을 구성한다. 캐시 부재 시 error(REQ-M2-05, 순서 제약). config.go 노브/웹 필드 추가 없음(REQ-M4-04).
- **status 캐시 방출**: `chirpstack-status` 노드는 폴 없이 comm 맵의 마지막 캐시 상태를 `buildChirpStackDeviceStateMessage`(node/chirpstack.go:251) shape 로 방출한다. `commMu` + `agentName` pre-capture 로 락 트랩 회피(REQ-FROZEN-B/M3-03).
- **방법론**: Hybrid — 신규 파일(코덱/노드/발행 프리미티브) TDD(RED-GREEN-REFACTOR), SPEC-001 수신 경로 무회귀는 characterization 테스트로 방어.

## 2. 우선순위 기반 마일스톤

### Primary Goal (Priority High)

- **M1 — 발행 프리미티브 (MessagePublisher)**
  - `ChirpStackAgent.PublishMessage(topic,qos,retained,payload)` — `mqtt_agent.go:609` 가드 미러, `a.client` 재사용.
  - `agent.go:27` 주석 + 인터페이스-체크 블록(`var _ agent.MessagePublisher = ...`) 갱신 — SPEC-001 배제 역전 명시(REQ-M1-02, R1).
  - 미연결/stopped 발행 가드(REQ-M1-04).
  - (선택) typed `SendDownlink(applicationID, devEui, cmd)` helper.
- **M2 — control 노드 + 코덱 레지스트리**
  - `Encode(cmd) (fPort uint8, data []byte, confirmed bool, err error)` 인터페이스 + per-deviceProfile 레지스트리.
  - Milesight WS301 seed 코덱(문서-유도 golden-vector; 정확 바이트 검증은 run-phase, R2).
  - `chirpstack-control` 노드: devEui + 캐시 applicationId + typed command → codec → `application/{applicationId}/device/{devEui}/command/down` 발행(`{devEui, confirmed, fPort, data(base64)}`).
  - unknown-codec/command → error, no publish(REQ-M2-04). applicationId 미캐시 → error(REQ-M2-05).
- **M4 — wiring**
  - `internal/node/registry.go:~99` 에 `chirpstack-control` + `chirpstack-status` 등록(category `"io"`, chirpstack-in 인접).
  - `pkg/flow/validate.go:139` `agentRefRequiredTypes` 에 두 타입 추가.
  - 프론트엔드 팔레트는 `/nodes` 로 자동 노출(웹 편집 불필요). config.go 노브/CHIRPSTACK_FIELDS 변경 없음.

### Secondary Goal (Priority Medium)

- **M3 — status 노드 (캐시 방출)**
  - `chirpstack-status` 노드: input/trigger 시 comm 맵 마지막 캐시 상태(`online/rssi/snr/gateway_id/last_seen_ms`) 방출, `buildChirpStackDeviceStateMessage` shape 미러.
  - 0 publish/0 poll(REQ-M3-02). missing-entry → offline/unknown 방출(REQ-M3-04).
  - `commMu` + `agentName` pre-capture(REQ-M3-03, R4).

### 전 구간 병행

- **M5 — 테스트 + 품질 게이트**
  - table-driven: WS301 typed command → 토픽+페이로드 golden-vector, status 캐시 방출, unknown-codec 거부, SPEC-001 무회귀, 발행 가드.
  - 신규 코드 커버리지 ≥85%, TRUST 5 PASS, LSP zero errors/type/lint(run-phase).

## 3. 아키텍처 설계 방향

- 2계층 유지(SPEC-001 정신): 에이전트(발행 프리미티브 + 코덱 + applicationId 캐시) ↔ 노드(control/status Flow 노드). 노드는 에이전트 능력을 Flow 로 노출하는 얇은 어댑터.
- 제어 노출면은 Flow 노드 전용. control-panel ControllableDevice 어댑터 / REST `/execute` 경로는 범위 밖(승인 게이트 확정).
- 코덱 레지스트리는 확장점(extensibility seam): v1 WS301 만, 후속 deviceProfile 추가 시 코덱 등록만으로 확장.

## 4. 위험 및 대응

- **R2 (Critical)**: WS301 코덱 바이트/fPort 미검증(공식 문서 HTTP 401). 대응: run-phase 전제조건으로 실 문서 대조 검증 명문화. 그 전까지 golden-vector(문서-유도) 기반 테스트, "검증된 바이트" 주장 금지. `@MX:DEBT`/`@MX:TODO` 로 미검증 상태 표기 권장.
- **R1**: 발행 경로 역전 — agent.go:27 주석 + 인터페이스-체크 블록 갱신을 커밋 메시지/주석에 명시(무회귀 오인 방지, REQ-M1-02).
- **R4**: comm 맵 락 안전 — `commMu` 보유 구간 진입 전 `agentName` pre-capture(REQ-FROZEN-B). HVAC v0.18.6 재귀 RLock deadlock 트랩 재현 금지.
- **R5**: applicationId 캐시 의존 — 제어는 최초 업링크 캐시 이후에만 가능. 미캐시 시 명확한 error 반환(REQ-M2-05), 노드 문서에 순서 제약 기록.

## 5. 라이브러리/버전

- 신규 외부 의존 없음. paho MQTT(기존), 표준 `encoding/base64`, `encoding/json` 재사용. 상세 버전 확정은 run-phase.

## 6. 실측 결과 (as-built)

> 구현 커밋 `d5d0089d` (branch `feature/SPEC-CHIRPSTACK-002`) 기준. 계획(§1~§5)은 원문 그대로 보존하고,
> 실제 결과와 계획의 차이만 여기에 기록한다.

### 6.1 파일 인벤토리 (13 신규 / 11 수정)

`git show --stat d5d0089d` 실측: 24 파일, +2491 / -20.

**신규 13개**

| 파일 | 역할 |
|------|------|
| `internal/agent/chirpstack/codec.go` | 다운링크 코덱 인터페이스 + per-deviceProfile 레지스트리 (M2) |
| `internal/agent/chirpstack/codec_ws301.go` | Milesight WS301 seed 코덱 + 바이트 테이블 (M2) |
| `internal/agent/chirpstack/downlink.go` | `BuildDownlinkTopic` / `BuildDownlinkPayload` 순수 함수 (M1/M2) |
| `internal/agent/chirpstack/publish.go` | `PublishMessage` 발행 프리미티브 (M1) |
| `internal/agent/chirpstack/commstate_query.go` | `CommSnapshot` / `CommStateRecordJSON` / `CommStateEnabled` (M3) |
| `internal/node/chirpstack_control.go` | `chirpstack-control` 노드 (M2) |
| `internal/node/chirpstack_status.go` | `chirpstack-status` 노드 (M3) |
| `internal/agent/chirpstack/codec_test.go` | 코덱 골든 벡터 + 거부 케이스 (M5) |
| `internal/agent/chirpstack/downlink_test.go` | 토픽/페이로드 + `DownlinkTarget` 캐시 (M5) |
| `internal/agent/chirpstack/publish_test.go` | 발행 가드 / 성공 / 토큰 에러 (M5) |
| `internal/agent/chirpstack/commstate_query_test.go` | 캐시 스냅샷 / 부재 / 동시성 (M5) |
| `internal/node/chirpstack_control_test.go` | control 노드 테이블 주도 테스트 (M5) |
| `internal/node/chirpstack_status_test.go` | status 노드 테이블 주도 테스트 (M5) |

**수정 11개**

| 파일 | 변경 |
|------|------|
| `internal/agent/chirpstack/agent.go` | SPEC-001 발행 배제 선언 3곳 갱신 + `MessagePublisher` 어서션 추가, `a.client` 대입을 `a.mu` 하로 이동 |
| `internal/agent/chirpstack/provider.go` | `applicationId`/`deviceProfileName` 캐시 + `DownlinkTarget` 접근자 |
| `internal/agent/chirpstack/decode.go` | 업링크 `deviceInfo` 필드 확장 |
| `internal/node/registry.go` | `chirpstack-control` / `chirpstack-status` 등록 (category `io`) |
| `pkg/flow/validate.go` | `agentRefRequiredTypes` 에 두 타입 추가 |
| `internal/api/service/node_adapter_test.go` | 노드 타입 수 단언 71 → 73 |
| `internal/node/registry_test.go` | 노드 타입 수 단언 71 → 73 |
| `internal/node/mqtt_test.go` | 노드 타입 수 단언 71 → 73 |
| `internal/agent/chirpstack/agent_mqtt_test.go` | 인터페이스 어서션 테스트 확장 |
| `internal/agent/chirpstack/transport_test.go` | 테스트 더블 확장 |
| `.moai/specs/SPEC-CHIRPSTACK-002/lsp-baseline.json` | clean HEAD 실측 재현으로 baseline 정정 |

계획 §4 대비 `web/` 및 `config.go` 무변경(REQ-M4-04 준수) 확인.

### 6.2 F-1~F-4 설계 결정 (해소 결과)

- **F-1 (control 노드 입력 계약)** — 대상 디바이스를 **설정이 아니라 입력 메시지**로 지정한다. payload `unit_id`(폴백 `device_id`, 이후 metadata 의 동일 2키) + `command` + `params` + 선택 `confirmed`. 노드 1개가 N 개 디바이스 담당. `lgap.go` / `xsfm.go` 관용구 미러. (spec.md IN-1)
- **F-2 (status 노드 키잉)** — 동일하게 payload `unit_id`, 트리거 1건당 메시지 1건. (spec.md IN-2)
- **F-3 (REQ-M1-03 typed helper 형태)** — `SendDownlink` 에이전트 메서드를 **추가하지 않고** 패키지 레벨 순수 함수 `BuildDownlinkTopic` / `BuildDownlinkPayload` 로 제공, 노드가 조립 후 `PublishMessage` 직접 호출. `internal/node/mqtt.go` 의 thin-adapter 원칙. (spec.md IN-4)
- **F-4 (`applicationId` 캐시 위치)** — comm 맵이 아니라 **`devices` 맵**(`upsertDevice` 경유). comm 맵 갱신이 `EmitCommState` 게이트에 종속되므로, 거기에 캐시하면 제어 경로가 그 노브에 숨은 의존성을 갖는다. (spec.md IN-5)

### 6.3 계획 대비 분기 (divergence)

1. **R6 신규 식별 (원 SPEC 미기재)** — comm 맵은 `emit_comm_state`(기본 false)가 켜져 있을 때만 채워지므로, 꺼진 에이전트에 붙은 status 노드는 항상 offline/unknown 을 방출한다. 해소: REQ-M3-01 엄격 준수 + 전제조건 공개(registry 설명 + Go doc + `Init` 경고 1회). `devices` 로스터 폴백 없음, 하드 실패 없음. (spec.md IN-3)
2. **하드코딩 단언 갱신 1곳 예상 → 실제 3곳** — 노드 타입 수 71 → 73 반영에 `node_adapter_test.go`, `registry_test.go`, `mqtt_test.go` 3곳이 필요했다. (spec.md IN-9)
3. **패키지 경계 export 3건 + 접근자 2건** — `BuildDownlinkTopic` / `BuildDownlinkPayload` / `DownlinkTarget` export, `CommStateRecordJSON` / `CommStateEnabled` 접근자 추가. 계획 §3 의 "노드는 얇은 어댑터" 원칙 유지를 위한 최소 노출면. (spec.md IN-6)
4. **`a.client` 동기화 수정** — 발행 경로 추가로 선재하던 비동기화 쓰기가 실제 레이스가 되어 `a.mu.Lock()` 하로 이동. 계획에 없던 최소 수정. (spec.md IN-7)
5. **base64 인코딩 근거 확정** — `StdEncoding`. ChirpStack v4 소스(integration.proto / mqtt.rs / pbjson lib.rs) 대조. `ignore_unknown_fields` 때문에 필드명 오타가 무음 실패하므로 필드명을 골든 벡터로 고정. 실브로커 미검증. (spec.md IN-8)
6. **R2/A6 해소** — 계획 §4 의 "run-phase 전제조건" 충족. WS301 User Guide V1.4 대조 완료, `ff 28 ff` 1건만 잔여 예외(`@MX:DEBT` 표기). (spec.md A6/R2 해소 블록)

### 6.4 품질 실측

- 신규 소스 7개 파일 statement 가중 커버리지 **93.43%** (256/274) — 목표 85% 초과.
- `go test -race` 클린, 스코프 `go vet` / `go build` exit 0.
- SPEC-001 characterization 무회귀: `internal/node/chirpstack_test.go` / `internal/agent/chirpstack/comm_state_test.go` 본 커밋에서 무변경.
- 상세 근거 및 미해소 항목: `.moai/reports/sync-report-chirpstack-002.md`.

## 7. Traceability

- spec.md REQ-FROZEN-A/B, M1~M5 ↔ 본 계획 마일스톤.
- acceptance.md AC-1~AC-5 ↔ M2/M3/M5 검증.
- 선행: SPEC-CHIRPSTACK-001(completed). 관련: SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001.
