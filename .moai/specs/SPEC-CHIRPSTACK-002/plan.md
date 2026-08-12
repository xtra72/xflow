---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드 — 구현 계획"
version: 0.1.0
status: draft
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

## 6. Traceability

- spec.md REQ-FROZEN-A/B, M1~M5 ↔ 본 계획 마일스톤.
- acceptance.md AC-1~AC-5 ↔ M2/M3/M5 검증.
- 선행: SPEC-CHIRPSTACK-001(completed). 관련: SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001.
