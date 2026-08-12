---
id: SPEC-CHIRPSTACK-001
title: "ChirpStack LoRaWAN 에이전트 — 인수 기준"
version: 0.1.0
status: completed
created: 2026-08-12
updated: 2026-08-12
author: xtra
priority: P1
phase: acceptance
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: L
tags: [chirpstack, lorawan, mqtt, agent, acceptance]
---

# ChirpStack 에이전트 — 인수 기준 (acceptance.md)

> 형식: Given-When-Then. 픽스처: `/Users/xtra/Projects/xflow/packet.json` (WS301 도어 센서).

## AC-1: 업링크 → per-measurement 발행 (REQ-FROZEN-01, REQ-M3-02)

### AC-1a: 단일 measurement (packet.json 실측)
- Given ChirpStack 업링크 `object={magnet_status:"close"}` 가 도착
- When 에이전트가 디코드/발행
- Then measurement 당 1개 메시지가 방출되고
  - `type=="event"`
  - top-level `timestamp` == 업링크 `time` (2026-08-11T23:32:01.129Z)
  - `payload.value == "close"` (문자열 스칼라 허용)
  - `metadata.measurement == "magnet_status"`
  - `metadata.device.name == "WS301-180806"`, `metadata.device.id` 가 UUID 로 승격
  - `metadata.tags == {location:"실습실", point:"앞문", spot:"앞문"}`

### AC-1b: 다중 measurement fan-out (temperature/humidity/co2/battery)
- Given 업링크 `object={temperature:24.5, humidity:60, co2:800, battery:95}`
- When 발행
- Then 4개 메시지 방출, 각 `metadata.measurement` ∈ {temperature,humidity,co2,battery}, 각 `payload.value` 는 해당 스칼라, 모두 동일 top-level timestamp/device/tags 공유

### AC-1c: 비스칼라 skip (REQ-M3-05)
- Given 업링크 `object={sensors:{a:1}}` (중첩 객체)
- When 발행
- Then 해당 키는 skip + 경고 로그, 스칼라 형제 키만 방출

## AC-2: 다운스트림 계약 보존 (REQ-FROZEN-02)

- Given 기존 store/influx 소비자
- When per-measurement 메시지 수신
- Then 읽기 경로 `$.payload.value`, `$.metadata.measurement`, `$.metadata.device.id`, `$.metadata.device.name`, `$.metadata.tags.*`, `$.timestamp` 가 모두 이전과 동일하게 해석됨 (golden 픽스처 회귀 테스트)

## AC-3: 디바이스 자동 생성 + UID + 메타데이터 (REQ-M4-*)

### AC-3a: 자동 생성 + UID 해결
- Given devEui `24e124141d180806` 첫 업링크
- When 처리
- Then `ResolveDeviceID(ctx, agentID, "24e124141d180806")` 가 UUID v4 발급, roster 에 upsert, `DeviceProvider().Devices()` 에 노출

### AC-3b: 재수신 시 동일 UID
- Given 동일 devEui 재업링크
- When 처리
- Then 동일 UUID 재사용(신규 발급 없음)

### AC-3c: 메타데이터 지속화
- Given deviceName/tags 포함 업링크
- When 처리
- Then `SetDeviceInfo`(DeviceType=`WS301`, Label=`WS301-180806`) 등록, registry `SetMetadata`(UUID 키)로 Name/Tags 지속화

## AC-4: tags raw pass-through + point/spot 기록 (REQ-FROZEN-04)

- Given `deviceInfo.tags={location:"실습실", point:"앞문", spot:"앞문"}`
- When 발행
- Then `metadata.tags` 에 세 키 모두 verbatim 전달, 키 매핑/정규화 없음
- And 명시 기록: packet.json 은 `point`+`spot` 모두 보유(실측), 레거시 split 출력은 `spot`(=location 값)만 노출했으나 pass-through 는 원본 그대로 전달 — 선택은 다운스트림 몫

## AC-5: comm-state change/report + staleness→offline (REQ-M5-*)

### AC-5a: change on online 전이
- Given `emit_comm_state=true`, 디바이스 첫 업링크
- When 처리
- Then `device_state.change` emit, `state.online==true`, `state.rssi/snr/gateway_id` 는 최적(최대 rssi) 게이트웨이 값(예: rssi=-57, snr=13.5, gateway_id=24e124fffef79304), `last_seen_ms` int64 UnixMilli

### AC-5b: staleness → offline
- Given `offline_threshold=300s`, 마지막 업링크 후 301s 경과
- When watchdog tick
- Then offline 전이 감지, `device_state.change` emit, `state.online==false`

### AC-5c: 주기 report
- Given `comm_report_interval=60s`, 변화 없음
- When 60s 경과
- Then `device_state.report` emit (trigger=report)

### AC-5d: report off
- Given `comm_report_interval=0`
- When 시간 경과(변화 없음)
- Then 주기 report 미방출; 단 online 전이 change 이벤트는 정상 방출

## AC-6: config-gated comm-state off (REQ-M5-01)

- Given `emit_comm_state=false`
- When 업링크 다수 처리
- Then 어떤 `device_state` 메시지도 방출되지 않음 (per-measurement 만 방출)

## AC-7: 에이전트 이름 고유성 제약 (REQ-M1-04)

- Given 이미 등록된 이름과 충돌하는 ChirpStack 에이전트 생성 시도
- When 등록
- Then 조용한 덮어쓰기 없이 충돌 감지/거부 (device_id collision 방지)

## AC-8: 등록 wiring + 인스턴스화 (REQ-M1-02, REQ-M3-06)

- Given `RegisterChirpStackTypes` 정의
- When `cmd/xflowd/main.go:360-413` 에 호출/import 추가
- Then `agentMgr.Create("chirpstack", ...)` 성공, `chirpstack-in` 노드가 `node/registry.go` 등록 + `flow/validate.go` 화이트리스트로 플로우 검증 통과

## AC-9: MQTT 연결/구독/재연결 (REQ-M2-*)

- Given 브로커 설정
- When start
- Then `application/#` 구독, 메시지 수신 시 `ReceiveMessage` 반환, `TransportConnected()==true`
- And stop 후 auto_reconnect 하에서도 세션 부활 없음(stopped 가드)

## TRUST 5 게이트 체크리스트

- Tested: 테이블 주도 테스트(packet.json), 다운스트림 golden 회귀, 커버리지 ≥85% (신규 코드).
- Readable: 파일 단일 책임 분할, 한국어 주석(code_comments=ko), 명확 명명.
- Unified: gofmt/goimports/golangci-lint 통과.
- Secured: MQTT 자격증명은 config(env) 경유, 코드 하드코딩 금지; 입력 JSON 방어적 파싱.
- Trackable: Conventional commits + SPEC-CHIRPSTACK-001 참조, @MX 태그(watchLoop/reportLoop WARN).

## Definition of Done (DoD)

- [ ] AC-1 ~ AC-9 전부 통과.
- [ ] REQ-FROZEN-01~04 및 REQ-M1~M6 구현.
- [ ] 커버리지 ≥85%, LSP run-phase zero(errors/type/lint).
- [ ] wiring 3곳(register.go/main.go/node registry+validate) 검증.
- [ ] goroutine 누수 없음(context cancel 종료 테스트).
- [ ] 다운스트림 계약 회귀 없음(golden).
- [ ] point/spot 불일치 SPEC 기록 확인.
