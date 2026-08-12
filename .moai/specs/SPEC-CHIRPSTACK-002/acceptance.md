---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드 — 인수 기준"
version: 0.1.0
status: draft
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: acceptance
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, downlink, control, status, node, codec, acceptance]
---

# ChirpStack status/control 노드 — 인수 기준 (acceptance.md)

> 형식: Given-When-Then. 제어 노출면 = Flow 노드 전용(control-panel/REST 범위 밖).
> WS301 바이트/fPort 는 문서-유도 golden-vector(정확 바이트 검증은 run-phase, R2/A6).

## AC-1: WS301 typed command → 정확한 다운링크 토픽+페이로드 (REQ-M2-01~03, M1)

- Given `deviceProfileName == "WS301"`, 최초 업링크가 `deviceInfo.applicationId` 를 디바이스별로 캐시했고, `chirpstack-control` 노드가 WS301 typed command 입력을 수신
- When 노드가 devEui + 캐시 applicationId + typed command → WS301 코덱 인코딩 → 발행
- Then 정확히 1건 발행되고
  - 토픽 == `application/{applicationId}/device/{devEui}/command/down`
  - 페이로드 == `{"devEui":<devEui>, "confirmed":false, "fPort":<코덱 결정값>, "data":<base64(TLV bytes)>}`
  - `fPort`/`data` 는 WS301 golden-vector(문서-유도 인코딩)와 정확히 일치
  - (run-phase 전제) 실제 Milesight WS301 유저 가이드 대조로 바이트/fPort 검증 완료 후에만 "검증됨" 표기

### AC-1b: confirmed 통과 (REQ-M2-03, A4)
- Given confirmed=true 를 명시한 command
- When 발행
- Then 페이로드 `confirmed:true` 로 통과됨(단, 큐/ack 처리는 v1 미구현 — fire-and-publish)

## AC-2: status 캐시 상태 방출 — 0 publish / 0 poll (REQ-M3-01~03)

- Given 에이전트 comm 맵에 devEui 의 마지막 상태(`online=true, rssi=-90, snr=7.5, gateway_id="gw1", last_seen_ms=<UnixMilli>`)가 캐시됨
- When `chirpstack-status` 노드가 input/trigger 수신
- Then 캐시 상태가 1개 메시지로 방출되고
  - `payload.state == {online:true, rssi:-90, snr:7.5, gateway_id:"gw1", last_seen_ms:<int64 UnixMilli>}`
  - top-level `last_seen_ms` 존재, `unit_id`(=devEui) → device 그룹 승격(`buildChirpStackDeviceStateMessage` shape)
  - **MQTT publish 0건, on-demand poll 0건**
  - comm 맵 읽기는 `commMu` 하에서 pre-captured `agentName` 로 수행(R4)

### AC-2b: missing-entry (REQ-M3-04, SPEC-001 REQ-M5-06 일관)
- Given devEui 의 comm 엔트리 부재(예: 재시작 직후)
- When status trigger
- Then `online=false`/unknown 방출, `online=true` 조기 보고 금지

## AC-3: unknown deviceProfile codec → error + 0 publish (REQ-M2-04)

- Given `chirpstack-control` 노드가 미등록 deviceProfile(또는 코덱이 모르는 command) 입력을 수신
- When 인코딩 시도
- Then error 반환 + **발행 0건** — 일반(generic) passthrough 다운링크 미발생

### AC-3b: applicationId 미캐시 (REQ-M2-05, R5)
- Given 해당 devEui 의 업링크가 아직 없어 applicationId 미캐시
- When control 입력
- Then error 반환 + 발행 0건(토픽 구성 불가, 순서 제약)

## AC-4: SPEC-001 수신 경로 무회귀 (REQ-FROZEN-A)

- Given SPEC-CHIRPSTACK-001 의 수신 전용 경로(per-measurement fan-out, 다운스트림 읽기 경로, device_state fold, tags verbatim)
- When SPEC-002 의 발행 프리미티브/코덱/노드 추가 후 characterization 회귀 테스트 실행
- Then 읽기 경로 `$.payload.value`, `$.metadata.measurement`, `$.metadata.device.*`, `$.metadata.tags.*`, `$.timestamp` 및 epoch=int64 UnixMilli 모두 SPEC-001 과 동일하게 유지(무회귀). `agent.go:27` 주석/인터페이스-체크 블록 갱신은 REQ-M1-02 로 문서화됨(회귀 아님)

## AC-5: 발행 가드 — 미연결/stopped (REQ-M1-04)

- Given 에이전트가 disconnected 또는 stopped 상태
- When `chirpstack-control` 이 발행을 시도
- Then `PublishMessage` 가 error 반환 + 발행 0건(nil/미연결 클라이언트 거부, `mqtt_agent.go:609` 가드 미러)

## TRUST 5 체크리스트

- **Tested**: table-driven 테스트(AC-1~AC-5), 신규 코드 커버리지 ≥85%, WS301 golden-vector(문서-유도, run-phase 바이트 검증 대기 명시).
- **Readable**: 코덱 인터페이스/레지스트리 명료 네이밍, code_comments=ko 준수.
- **Unified**: gofmt/goimports 클린, SPEC-001 2계층 패턴 일관.
- **Secured**: 미등록/미지 command 거부(no generic passthrough), 미연결 발행 가드, base64 인코딩 검증.
- **Trackable**: Conventional commit + SPEC-CHIRPSTACK-002 참조, REQ-M1-02(역전) 커밋/주석 명시.

## Definition of Done (DoD)

- [ ] M1: `PublishMessage` 구현 + `mqtt_agent.go:609` 가드 미러 + `a.client` 재사용, agent.go:27 주석/인터페이스-체크 블록 갱신(REQ-M1-02).
- [ ] M2: per-deviceProfile 코덱 레지스트리 + WS301 seed 코덱 + `chirpstack-control` 노드, unknown/미캐시 → error+no publish.
- [ ] M3: `chirpstack-status` 노드 캐시 방출(0 publish/0 poll), missing-entry offline/unknown, commMu+agentName pre-capture.
- [ ] M4: registry.go(~99, io) + validate.go:139 wiring, `/nodes` 팔레트 자동 노출, config 노브/웹 필드 변경 없음.
- [ ] M5: AC-1~AC-5 전량 PASS, 커버리지 ≥85%, TRUST 5 PASS, LSP run-phase zero errors/type/lint.
- [ ] **R2 run-phase 전제조건**: Milesight WS301 유저 가이드 대조로 코덱 바이트/fPort 검증 완료 — 미검증 상태로 "검증됨" 주장 금지.
