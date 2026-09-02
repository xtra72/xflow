---
id: SPEC-CHIRPSTACK-003
title: "ChirpStack 게이트웨이 관리 — 인수 기준"
version: 0.1.0
status: draft
created: 2026-08-13
updated: 2026-08-13
author: xtra
priority: P1
phase: acceptance
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, gateway, rx-info, channel, exec, web-tab, acceptance]
---

# ChirpStack 게이트웨이 관리 — 인수 기준 (acceptance.md)

> 형식: Given-When-Then. 노출면 = 에이전트 exec 커맨드 + 웹 탭 전용(신규 Flow 노드 없음).
> A5(`channel: 0` JSON 생략)는 **플래닝 시점 실브로커 미검증**이며 run-phase 전제조건이다(R1).

## AC-1: 다중 게이트웨이 링크 보존 — gatewayId 키잉 (REQ-M1-01, M1-03, M2-01, M2-02)

- **Given** 하나의 업링크가 `rxInfo[]` 에 서로 다른 `gatewayId` 3건
  (`gwA:{rssi:-80,snr:9.0,channel:1}`, `gwB:{rssi:-95,snr:2.5,channel:5}`,
  `gwC:{rssi:-70,snr:11.25,channel:3}`)을 싣고 도착
- **When** 에이전트가 업링크를 처리(`handleUplink` → `upsertDevice`)
- **Then** 해당 devEui 의 링크 캐시에 **정확히 3건**이 남고
  - 각 링크는 **`gatewayId` 로 키잉**된다(배열 인덱스 아님)
  - 각 링크의 `rssi`/`snr`/`channel` 이 **해당 게이트웨이 항목의 값과 정확히 일치**한다
    (최대 RSSI 항목의 값으로 덮이지 않음)
  - `bestGateway` 기반 comm 맵은 종전대로 최대 RSSI 1건(`gwC`)만 반영한다(REQ-M1-05 무회귀)

### AC-1b: rxInfo 순서 무관성 (REQ-M1-03, A2/R8)

- **Given** 동일한 rxInfo 3건을 담되 배열 순서를 셔플한 업링크 2건
- **When** 각각을 처리
- **Then** 두 경우의 링크 캐시가 **완전히 동일**하다(게이트웨이별 값 매핑이 순서에 불변).
  `rxInfo[0]` 을 "최적" 또는 "대표" 로 취급하는 경로가 존재하지 않는다

### AC-1c: emit_comm_state=false 에서도 로스터가 채워진다 (REQ-M2-01)

- **Given** 에이전트 `emit_comm_state = false`(기본값)
- **When** 다중 게이트웨이 업링크 수신
- **Then** 링크 캐시는 **정상적으로 채워지고** `list_gateways` 가 게이트웨이를 반환한다
  (comm 맵은 비어 있어도 무관). 게이트웨이 탭이 노브에 종속되어 조용히 비지 않는다

## AC-2: `channel` 파싱 — 키 부재는 0 (REQ-M1-02, A5/R1)

- **Given** `rxInfo` 항목 3종:
  (a) `"channel": 3` 명시, (b) `"channel": 0` 명시, (c) **`channel` 키 자체가 부재**
- **When** 디코드
- **Then**
  - (a) → `channel == 3`
  - (b) → `channel == 0`
  - (c) → `channel == 0` — **"미상/미지원" 이 아니라 0** 이며, 파싱 타입은 값 타입(`uint32`)
    이어야 한다(nullable 표현 사용 시 이 AC 는 FAIL)
  - (b)와 (c)의 결과가 **구분 불가능**하다는 사실이 Go doc 에 기록되어 있다
- **run-phase 전제조건(REQ-M5-04, R1)**: 실브로커 캡처(또는 실제 캡처 픽스처)로 `channel` 키의
  생략 여부를 확인하기 전에는 "검증됨" 을 주장하지 않는다

### AC-2b: 프레임 레벨 txInfo 부착 (REQ-M1-04, F-3)

- **Given** 업링크의 `txInfo.frequency = 922100000`,
  `txInfo.modulation.lora.spreadingFactor = 7`, `bandwidth = 125000`, rxInfo 3건
- **When** 처리
- **Then** **3건 모두** 동일한 `frequency_hz`/`spreading_factor`/`bandwidth` 를 갖는다
  (프레임 레벨 값이며 게이트웨이별로 달라지지 않는다)

## AC-3: `list_gateways` exec 계약 (REQ-M3-01~04)

- **Given** 링크 캐시에 gwA(디바이스 2대), gwB(디바이스 1대)가 있고 에이전트가 Running
- **When** `POST /agents/{id}/exec` 로 `{"command":"list_gateways"}` 전송
- **Then**
  - 응답이 spec.md §Specifications 의 shape 를 정확히 따른다
    (`gateways[].{gateway_id, device_count, last_seen_ms, devices[]}`,
    `devices[].{dev_eui, device_id, device_name, device_profile_name, rssi, snr, channel,
    frequency_hz, spreading_factor, bandwidth, last_seen_ms, stale}`)
  - `device_count == len(devices)` 이고 gwA=2, gwB=1
  - **MQTT publish 0건, on-demand poll 0건, 저장소 write 0건**
    (`agent_adapter.go:662` 영속화 분기는 `add_device`/`remove_device`/`set_device` 전용)
  - 게이트웨이 이름/위치 필드는 응답에 **존재하지 않는다**(A6, Non-Goal)

### AC-3b: 결정적 정렬 (REQ-M3-03)

- **Given** 동일 상태에서 `list_gateways` 를 연속 5회 호출
- **Then** 5회 응답이 **바이트 동일**하며, `gateways` 는 `gateway_id` 오름차순,
  각 `devices` 는 `dev_eui` 오름차순이다(Go 맵 순회 순서에 비의존)

### AC-3c: 미지 커맨드 거부 (REQ-M3-01)

- **Given** `{"command":"nonexistent"}`
- **When** exec
- **Then** 명확한 에러가 반환되고(무음 `nil, nil` 아님), 어떤 상태도 변경되지 않는다.
  `agent.go:491-495` 의 "Process 는 사용하지 않는다" 주석이 REQ-M3-01 역전 기록으로 갱신되어
  있다(미갱신 시 FAIL — R4 회귀 오인 방지)

### AC-3d: 신규 노드 타입 없음 (REQ-M3-06)

- **Given** 본 SPEC 의 전체 diff
- **Then** `internal/node/registry.go`, `pkg/flow/validate.go`,
  `web/src/config/nodeSchemas.ts`, 그리고 노드 타입 수 하드코딩 단언 3곳
  (`internal/api/service/node_adapter_test.go`, `internal/node/registry_test.go`,
  `internal/node/mqtt_test.go` — SPEC-002 IN-9)이 **모두 무변경**이다

## AC-4: 상한 + eviction 결정성 (REQ-M2-03, F-5)

- **Given** 한 디바이스가 이미 8개 게이트웨이 링크를 보유하고, 그중 `gwOld` 의
  `last_seen_ms` 가 유일하게 최소
- **When** 9번째 새 `gatewayId`(`gwNew`)를 담은 업링크가 도착
- **Then**
  - 링크 수는 **8 로 유지**된다
  - `gwOld` 가 제거되고 `gwNew` 가 입장한다(최고령 `last_seen_ms` eviction — 결정적)
  - 동일 시나리오를 반복 실행해도 **항상 같은 게이트웨이**가 제거된다
- **And** 기존 8개 중 하나(`gwExisting`)만 담긴 업링크가 오면 cap 과 무관하게 **갱신만** 일어나고
  eviction 은 발생하지 않는다

## AC-5: staleness 파생 (REQ-M2-05, F-2)

- **Given** `offline_threshold = 300s`, 링크 L1 의 `last_seen_ms` 가 10초 전, L2 가 600초 전
- **When** `list_gateways` 호출
- **Then** L1 은 `stale=false`, L2 는 `stale=true` 로 반환되며,
  - **L2 는 삭제되지 않는다**(staleness 만으로 하드 제거하지 않음)
  - `stale` 은 저장 필드가 아니라 조회 시점 파생값이다(저장 불리언 금지 —
    `deviceOnline`(`provider.go:160-182`)이 영구 online 굳음 결함을 제거한 것과 동일 이유)
- **And** `offline_threshold <= 0`(미설정)이면 `defaultOfflineThreshold` 로 폴백해
  모든 링크가 즉시 stale 로 뒤집히지 않는다(`deviceOnline` 의 zero-threshold 정책과 동일)

## AC-6: 락 안전 + 무레이스 (REQ-FROZEN-B, REQ-M2-06, R5)

- **Given** 업링크 수신 goroutine 과 `list_gateways` 조회가 동시 실행되는 테스트
- **When** `go test -race` 로 chirpstack 패키지 실행
- **Then**
  - **레이스 0건**
  - 링크 캐시 읽기/쓰기 경로에서 `devicesMu` 보유 중 `a.Name()`(= `a.mu` 재획득)이
    **호출되지 않는다** — `agentName` 은 락 진입 전 1회 선캡처된다
  - `devicesMu` 와 `commMu` 가 **동시에 보유되는 지점이 없다**
    (`deviceAdapters()`(`provider.go:311-341`) 규율과 동형)
  - 반환 구조에 내부 맵 포인터가 노출되지 않는다(스냅샷 깊은 복사 —
    `deviceState.clone()`(`provider.go:94-112`) 계약이 `links` 로 확장됨)

## AC-7: 프로즌 계약 무회귀 (REQ-FROZEN-A / B / C)

- **Given** SPEC-001 수신 경로와 SPEC-002 status/control 경로
- **When** 본 SPEC 의 변경 후 characterization 회귀 테스트 실행
- **Then**
  - **SPEC-001**: 읽기 경로 `$.payload.value`, `$.metadata.measurement`, `$.metadata.device.*`,
    `$.metadata.tags.*`, `$.timestamp` 및 epoch = `int64 UnixMilli` 가 모두 동일하게 유지된다.
    per-measurement fan-out / device_state fold / tags verbatim pass-through 무변경
  - **SPEC-002**: `chirpstack-status` 방출 shape
    (`payload.state = {online, rssi, snr, gateway_id, last_seen_ms}`)와
    `chirpstack-control` 다운링크 페이로드 계약이 무변경.
    `CommSnapshot` / `CommStateRecordJSON` / `CommStateEnabled` / `DownlinkTarget` 시그니처 무변경.
    `commEntry` 필드 의미(best-gateway 스칼라) 무변경
  - `bestGateway`(`decode.go:80-87`)와 `watchdog.go:125` 호출부 무변경
  - `agent.go:491-495` 주석 갱신은 REQ-M3-01 로 문서화되므로 **회귀가 아니다**

## AC-8: 웹 게이트웨이 탭 (REQ-M4-01~06)

- **Given** 에이전트 상세 패널을 `agentType === 'chirpstack'` 으로 렌더
- **When** 사용자가 화면을 열람
- **Then**
  - "게이트웨이" 탭이 **노출**되고, `agentType` 이 다른 에이전트(예: `xsfm`, `mqtt`)에서는
    **노출되지 않는다**(`HAS_GATEWAYS_TAB` 게이트, `AgentDetailPanel.tsx:186` 패턴)
  - 탭 컴포넌트는 전용 파일 `ChirpstackGatewaysTab.tsx` 에 있다
  - 데이터는 `execAgent(agentId, { command: 'list_gateways' })`(react-query 폴링)로 조회된다
  - 게이트웨이 행 클릭 시 **연결 디바이스 서브리스트가 확장**되며, 각 디바이스 행은
    RSSI / SNR / Channel / 주파수 / 마지막 수신을 표시한다
  - 게이트웨이는 **hex ID 로 표시**된다(이름 표시 없음 — A6)
  - `channel` 컬럼 라벨이 "게이트웨이 IF 채널" 계열이며 **"주파수" 로 표기되지 않는다**;
    주파수는 `frequency_hz` 기반 별도 컬럼이다(REQ-M4-04, R2)
  - `stale` 링크는 시각적으로 구분된다(REQ-M4-05)
  - 모든 표시 문자열이 `web/src/lib/i18n/en.json` / `ko.json` 키를 통한다(하드코딩 문자열 0)
  - `agentSchemas.ts` / `CHIRPSTACK_FIELDS` 및 에이전트 config 노브가 **무변경**(REQ-M4-06)

### AC-8b: 빈 상태 (A7/R3)

- **Given** 아직 어떤 업링크도 수신되지 않아 게이트웨이가 0건
- **Then** 탭은 빈 상태 안내를 표시하며, 그 문구가 **로스터가 업링크 파생이라는 한계**
  (침묵 중인 게이트웨이는 보이지 않음)를 사용자에게 알린다

## TRUST 5 체크리스트

- **Tested**: table-driven 테스트(AC-1~AC-8), 신규 코드 커버리지 ≥85%, `go test -race` 클린,
  SPEC-001/002 characterization 무회귀. `channel` 키 부재 케이스는 **실 캡처 픽스처 기반**으로
  작성하고, 미검증 상태는 명시 기록한다(R1).
- **Readable**: `gatewayLink` / `listGateways` 명료 네이밍, `code_comments = ko` 준수,
  A4(채널 의미) / A5(기본값 생략) 근거를 Go doc 에 기록.
- **Unified**: gofmt / goimports 클린, SPEC-001/002 의 2계층 + 락 규율 패턴 일관,
  웹은 기존 `SortableHeader` / `TablePagination` / 아코디언 관용구 재사용.
- **Secured**: 조회 전용 표면(0 publish / 0 poll / 0 repo-write), 상한(cap 8)으로
  스푸핑 게이트웨이 ID 에 의한 메모리 팽창 차단, 내부 맵 포인터 미노출(깊은 복사).
- **Trackable**: Conventional commit + `SPEC-CHIRPSTACK-003` 참조,
  REQ-M3-01(`Process` no-op 역전)을 커밋 메시지/주석에 명시.

## Definition of Done (DoD)

- [ ] **M1**: `uplinkRxInfo` 에 `channel`(값 타입) 추가 + `txInfo`(frequency/SF/BW) 파싱.
      `bestGateway` 및 `watchdog.go:125` 호출부 무변경. → AC-1, AC-2, AC-2b, AC-7
- [ ] **M2**: `deviceState.links` 캐시 + `clone()` 깊은 복사 확장 + `upsertDevice` 전량 병합 +
      cap(8)/최고령 eviction + `listGateways()` 파생(정렬·staleness). 신규 mutex 0개.
      → AC-1, AC-1c, AC-4, AC-5, AC-6
- [ ] **M3**: `Process` 디스패처 + `list_gateways` + `agent.go:491-495` 주석 역전 기록.
      0 publish / 0 poll / 0 repo-write. 신규 노드 타입 없음. → AC-3, AC-3b, AC-3c, AC-3d
- [ ] **M4**: `HAS_GATEWAYS_TAB` 게이트 + `ChirpstackGatewaysTab.tsx` + react-query 훅 +
      확장형 디바이스 서브리스트 + i18n(en/ko). config 노브 무변경. → AC-8, AC-8b
- [ ] **M5**: AC-1~AC-8 전량 PASS, 신규 코드 커버리지 ≥85%, `go test -race` 클린,
      TRUST 5 PASS, LSP/lint **delta 기준** 무회귀
      (저장소 선재 RED: `internal/schedulelog/observer_test.go` — 절대값 판정 금지)
- [ ] **R1 run-phase 전제조건**: A5(`channel: 0` JSON 생략)를 **실브로커 캡처 또는 실제 캡처
      픽스처**로 대조 검증. 검증 전 "검증됨" 주장 금지. 미해소 시 `@MX:DEBT` + `@MX:CEILING` +
      `@MX:UPGRADE` 로 확신 수준을 소스에 표기(SPEC-002 `codec_ws301.go:41-45` 선례)
- [ ] **F-1 ~ F-5 확정**: 사용자 확정 결과를 spec.md 요구사항에 반영(변경 시 REQ 수정 + 근거 기록)
