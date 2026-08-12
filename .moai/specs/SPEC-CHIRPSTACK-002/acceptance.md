---
id: SPEC-CHIRPSTACK-002
title: "ChirpStack status/control 노드 — 인수 기준"
version: 1.0.0
status: completed
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

### 실측 결과 (커밋 `d5d0089d`)

**커버리지 93.43%** — 신규 소스 7개 파일 statement 가중 실측 256/274 (목표 85% 초과):

| 파일 | 커버리지 |
|------|---------:|
| `internal/agent/chirpstack/codec.go` | 95.24% (20/21) |
| `internal/agent/chirpstack/codec_ws301.go` | 100.00% (27/27) |
| `internal/agent/chirpstack/commstate_query.go` | 93.75% (15/16) |
| `internal/agent/chirpstack/downlink.go` | 100.00% (6/6) |
| `internal/agent/chirpstack/publish.go` | 100.00% (18/18) |
| `internal/node/chirpstack_control.go` | 92.52% (99/107) |
| `internal/node/chirpstack_status.go` | 89.87% (71/79) |

**AC → 테스트 매핑** (모든 테스트 함수명 소스 대조 확인 완료):

| AC | 검증 테스트 |
|----|-------------|
| AC-1 | `TestEncodeDownlink_WS301Golden` (codec_test.go:15), `TestChirpStackControlNode_PublishesDownlink` (chirpstack_control_test.go:105) |
| AC-1b | `TestChirpStackControlNode_ConfirmedPassthrough` (chirpstack_control_test.go:161) |
| AC-2 | `TestChirpStackStatusNode_EmitsCachedCommState` (chirpstack_status_test.go:125) |
| AC-2b | `TestChirpStackStatusNode_MissingEntryEmitsOffline` (chirpstack_status_test.go:216) |
| AC-3 | `TestChirpStackControlNode_Rejections` (chirpstack_control_test.go:199), `TestEncodeDownlink_Rejections` (codec_test.go:99) |
| AC-3b | `TestDownlinkTarget_CachedAfterUplink` (downlink_test.go:93) + `TestChirpStackControlNode_Rejections` 의 "applicationId 미캐시 (AC-3b)" 테이블 케이스 (chirpstack_control_test.go:216) |
| AC-4 | `internal/node/chirpstack_test.go` / `internal/agent/chirpstack/comm_state_test.go` — 본 커밋에서 무변경(최종 수정 `88807142`, SPEC-001 M6) 상태로 통과 |
| AC-5 | `TestPublishMessage_Guards` (publish_test.go:19), `TestChirpStackControlNode_PublishGuardError` (chirpstack_control_test.go:252) |

## Definition of Done (DoD)

- [x] M1: `PublishMessage` 구현 + `mqtt_agent.go:609` 가드 미러 + `a.client` 재사용, agent.go:27 주석/인터페이스-체크 블록 갱신(REQ-M1-02).
  - 근거: `internal/agent/chirpstack/publish.go` 신규(커버리지 100%). `agent.go` 배제 선언 3곳(패키지 doc / 타입 doc / `Process` doc) 갱신 + `var _ agent.MessagePublisher = (*ChirpStackAgent)(nil)` 어서션 추가. `TestPublishMessage_Guards` / `_Success` / `_TokenError` PASS.
- [x] M2: per-deviceProfile 코덱 레지스트리 + WS301 seed 코덱 + `chirpstack-control` 노드, unknown/미캐시 → error+no publish.
  - 근거: `codec.go` + `codec_ws301.go` + `chirpstack_control.go` 신규. `TestEncodeDownlink_Rejections` / `TestChirpStackControlNode_Rejections`(미등록 코덱 · 미지 command · applicationId 미캐시 · 필수 입력 누락 4종) 가 error + 발행 0건 단언.
- [x] M3: `chirpstack-status` 노드 캐시 방출(0 publish/0 poll), missing-entry offline/unknown, commMu+agentName pre-capture.
  - 근거: `chirpstack_status.go` 신규 — 파일 전체에 발행 인터페이스 참조 없음(구조적 0 publish), 타이머/티커 없음(0 poll). `agentName` 은 `initAgent` 에서 1회 pre-capture(REQ-FROZEN-B). `CommSnapshot` 은 `commMu` 보유 중 `a.Name()` 미호출. `TestChirpStackStatusNode_EmitsCachedCommState` / `_MissingEntryEmitsOffline` PASS, `TestCommSnapshot_ConcurrentWithUplink` 로 동시성 확인.
- [x] M4: registry.go(~99, io) + validate.go:139 wiring, `/nodes` 팔레트 자동 노출, config 노브/웹 필드 변경 없음.
  - 근거: `registry.go` 에 두 타입 category `"io"` 로 `chirpstack-in` 인접 등록, `pkg/flow/validate.go` `agentRefRequiredTypes` 에 추가. 노드 타입 수 71 → 73. `web/` · `config.go` 무변경(REQ-M4-04) — `git show --stat d5d0089d` 에 해당 경로 부재로 확인.
- [x] M5: AC-1~AC-5 전량 PASS, 커버리지 ≥85%, TRUST 5 PASS, LSP run-phase zero errors/type/lint.
  - 근거: 스코프 `go test` / `go vet` / `go build` exit 0, `go test -race` 클린, 커버리지 93.43%. golangci-lint 는 SPEC-002 파일에서 지적 0건(저장소 선재 `unused` 4건은 `deduplicate.go` / `flow_node.go` / `modbus_common.go`).
  - **단서**: 저장소 전체 `go vet ./...` / `go test ./...` 는 `internal/schedulelog/observer_test.go` 의 **선재 결함**으로 RED 이다(본 SPEC 무관, `lsp-baseline.json` 에 기록). 따라서 무회귀 판정은 절대값이 아니라 delta 기준이다.
- [x] **R2 run-phase 전제조건**: Milesight WS301 유저 가이드 대조로 코덱 바이트/fPort 검증 완료 — 미검증 상태로 "검증됨" 주장 금지.
  - **충족**. 공식 Milesight WS301 User Guide V1.4(2026-04-22, 33p, `https://resource.milesight.com/milesight/iot/document/ws301-user-guide-en.pdf`)를 정상 수신해 대조했다(플래닝 시점의 HTTP 401 재현되지 않음). milesight.com WS301 downlink HTML 문서 및 공식 `Milesight-IoT/SensorDecoders` `ws301-encoder.js` 와 교차 확인.
  - 검증된 값: fPort **85**(p.31), TLV `0xFF <cmd> <value...>` + 파라미터 **리틀엔디언**(p.29), reboot `ff 10 ff`, 보고 주기 `ff 03 <lo> <hi>` UINT16 LE 초(Ch.6 p.31-32, 예시 `ff03b004`=1200초), 유효 범위 60~64800초(ToolBox 범위 1~1080분, p.17 + 공식 encoder 검증).
  - **잔여 예외 1건**: `ff 28 ff`(query device status)는 공식 encoder 에만 존재하고 User Guide V1.4·milesight.com 문서에 미등재 — 이 바이트에 한해 "검증됨" 을 주장하지 않으며 `codec_ws301.go:41-45` 의 `@MX:DEBT` / `@MX:CEILING` / `@MX:UPGRADE` 로 표기했다.
  - 검증 범위 한계: **문서 대조**이며 실기기 다운링크 수신 확인은 수행하지 않았다.
