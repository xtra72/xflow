---
id: SPEC-CHIRPSTACK-003
title: "ChirpStack 게이트웨이 관리"
version: 0.1.0
status: draft
created: 2026-08-13
updated: 2026-08-13
author: xtra
priority: P1
phase: spec
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, gateway, rx-info, rssi, snr, channel, exec, web-tab]
---

# SPEC-CHIRPSTACK-003: ChirpStack 게이트웨이 관리

> 선행 SPEC-CHIRPSTACK-001(수신 에이전트, completed) / SPEC-CHIRPSTACK-002(status/control 노드 +
> 다운링크 코덱, completed) 위에, **게이트웨이를 1급 엔티티로** 모델링한다. 현재 디바이스의
> 링크 품질은 `gateway_id`/`rssi`/`snr` **스칼라 1쌍**으로 납작하게 접혀 있으나, 실제로는 하나의
> 업링크를 **여러 게이트웨이가 동시에 수신**하며 (gateway, device) 는 **쌍(pair)** 이고 RSSI/SNR/
> Channel 은 그 쌍에 귀속된다. 본 SPEC 은 그 쌍을 보존하고, 게이트웨이별 연결 디바이스 목록을
> 웹 탭으로 노출한다.
>
> 노출면은 **에이전트 exec 커맨드 + 웹 탭 전용**이다. 신규 Flow 노드 타입은 추가하지 않는다.

## Environment (환경)

- 언어/런타임: Go 1.23+ 모듈 `github.com/xtra/xflow`. 프론트엔드: React + react-query + Tailwind (`web/`).
- 확장 대상:
  - `internal/agent/chirpstack/decode.go` — 업링크 디코드 구조체.
  - `internal/agent/chirpstack/provider.go` — 디바이스 로스터(`devices` 맵) + `upsertDevice`.
  - `internal/agent/chirpstack/agent.go` — `Process` (현재 no-op) → exec 커맨드 디스패처.
  - `web/src/pages/agents/` — 신규 게이트웨이 탭 컴포넌트 + `AgentDetailPanel` 게이트.
- **현행 코드 상태 (실측)**:
  - `internal/agent/chirpstack/decode.go:25-29` `uplinkRxInfo` 는 **`gatewayId`/`rssi`/`snr` 3개만**
    파싱한다. `channel` 은 파싱 대상이 아니다. `txInfo` 는 `uplink` 구조체(`decode.go:34-39`)에
    아예 존재하지 않는다.
  - `internal/agent/chirpstack/decode.go:80-87` `bestGateway` 는 최대 RSSI 항목 1개만 남기고
    **`rxInfo[]` 의 나머지를 폐기한다**. 유일한 호출자는
    `internal/agent/chirpstack/watchdog.go:125` (`onUplinkCommState`) 이다.
  - `internal/agent/chirpstack/watchdog.go:14-20` `commEntry` 는 `rssi`/`snr`/`gatewayID` 를
    **스칼라**로 보유한다 — 슬라이스가 없다.
  - Go/웹 어디에도 **게이트웨이 엔티티가 존재하지 않는다**. `gateway_id` 는 comm-state 스칼라와
    `chirpDeviceAdapter.properties()`(`provider.go:476-491`)의 문자열 속성으로만 등장한다.
  - `internal/agent/chirpstack/agent.go:491-495` `Process` 는 **의도적 no-op** 이다
    (주석: "Process 는 사용하지 않는다. 다운링크 발행은 PublishMessage 인터페이스를 사용한다").
  - `internal/agent/chirpstack/provider.go:193-262` `upsertDevice` 는 **모든 업링크마다 무조건**
    실행된다. 반면 comm 맵 갱신은 `internal/agent/chirpstack/agent.go:309-311` 에서
    `emit_comm_state`(기본 **false**) 게이트 뒤에 있다.
  - `internal/agent/chirpstack/provider.go:19` `maxCachedMeasurements = 64` — 디바이스당 캐시
    상한의 선례.
  - `internal/device/device.go:123-132` `DeviceProvider` 가 유일한 하위 엔티티 프로바이더
    인터페이스이다. 비-디바이스 엔티티용 대응물은 없다.
- **재사용 선례 (exec 경로)**:
  - `internal/agent/xsfm/agent.go:855-889` `XSFMAgent.Process` 의 커맨드 디스패처 +
    `case "list_stations": return a.handleListStations()`.
  - `internal/api/handler/agent.go:230` `POST /agents/{id}/exec` → `:480` `Exec` 핸들러.
  - `internal/api/service/agent_adapter.go:644-670` `ExecAgent` → `ag.Process(data)`.
    로스터 영속화(`:662`)는 `add_device`/`remove_device`/`set_device` 에만 걸리므로
    조회 전용 커맨드는 저장소 경로를 건드리지 않는다.
  - `web/src/services/api/agentService.ts:117` `execAgent(id, {command, params})`.
  - `web/src/hooks/useStation.ts:120-131` `useStations` — react-query + `execAgent` 폴링 관용구.
- **재사용 선례 (UI)**:
  - `web/src/pages/agents/AgentDetailPanel.tsx:186` `HAS_STATIONS_TAB = new Set<string>(['xsfm'])`
    — 에이전트 타입별 탭 게이트. 탭 union `:142`, 탭 버튼 `:232`, 렌더 `:249`.
  - `web/src/pages/agents/XsfmStationsTab.tsx:515-556` 다중 확장 아코디언 행.
  - `web/src/pages/devices/DeviceListPage.tsx:850-917` 확장형 테이블 행.
  - `web/src/components/common/SortableHeader.tsx:29`, `web/src/pages/agents/TablePagination.tsx`.
  - i18n 키: `web/src/lib/i18n/en.json` / `ko.json`.
  - 로컬 에이전트 데이터는 react-query 폴링을 쓴다(SSE 는 원격 프록시 전용).
- 개발 방법론: Hybrid — 신규 코드 TDD, 기존 수신 경로 흡수부 DDD. 신규 코드 커버리지 목표 85%.

## Assumptions (가정)

각 항목은 근거 출처를 함께 기록한다. 출처가 없거나 실측이 없는 항목은 Risks 에 정직하게 남긴다.

- **A1. `UplinkEvent.rx_info` 는 `repeated` 이다.** 하나의 업링크를 여러 게이트웨이가 수신하는
  것이 정상 동작이다.
  출처: `https://github.com/chirpstack/chirpstack/blob/master/api/proto/integration/integration.proto`
- **A2. `rxInfo[]` 배열에는 순서 보장이 없다.** ChirpStack 의 중복 제거(dedup)는 Redis Set
  (`SADD`/`SMEMBERS`)에 프레임을 모으며 정렬하지 않는다. 따라서 **반드시 `gatewayId` 로 키잉**해야
  하고, 배열 인덱스를 식별자로 쓰거나 `rxInfo[0]` 을 "최적" 으로 가정해서는 안 된다.
  출처: `https://github.com/chirpstack/chirpstack/blob/master/chirpstack/src/uplink/mod.rs`
- **A3. `UplinkRxInfo` 필드 집합**(`api/proto/gw/gw.proto`):
  `gatewayId`(string, EUI64 hex), `uplinkId`, `gwTime`(Timestamp — **GNSS 필요, 대개 부재**),
  `timeSinceGpsEpoch`, `fineTimeSinceGpsEpoch`, `rssi`(int32), `snr`(float), **`channel`(uint32)**,
  `rfChain`, `board`, `antenna`, `location`(common.Location, 대개 부재), `context`(bytes→base64),
  `metadata`(map, 스키마 비보장), `crcStatus`(enum 이름 문자열),
  `nsTime`(Timestamp — ChirpStack 이 세팅하므로 **사실상 항상 존재**).
  출처: `https://github.com/chirpstack/chirpstack/blob/master/api/proto/gw/gw.proto`
- **A4. `channel` 의 의미는 "수신 게이트웨이의 concentrator IF 채널 인덱스"** 이다
  (Semtech UDP `rxpk.chan` 에 대응하는 **게이트웨이 로컬 하드웨어 값**). 게이트웨이 간 비교
  가능성이 약하다. 실제 RF **주파수는 `txInfo.frequency`(Hz)** 이며 이는 **프레임 레벨**로
  모든 `rxInfo` 항목에 공통이다. 변조 파라미터는
  `txInfo.modulation.lora.{spreadingFactor,bandwidth,codeRate}` 아래에 있다.
  → 구현자/UI 는 `channel` 을 "주파수" 로 제시해서는 안 된다.
  출처: `https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT`,
  `https://github.com/chirpstack/chirpstack/blob/master/api/proto/gw/gw.proto`
- **A5. proto3 JSON 은 기본값 필드를 생략한다.** 즉 `channel: 0` 은 **정당한 값이지만 JSON 에서
  사라진다**. 파싱은 **키 부재를 0 으로** 취급해야 하며 "미지원/미상" 으로 해석해서는 안 된다.
  Go `encoding/json` 이 `uint32` 필드로 언마샬하면 부재 시 0 이 되므로 값 타입 필드를 쓰는 한
  자동 충족되지만, `*uint32`/`sql.Null*` 등 nullable 표현을 쓰면 즉시 깨진다.
  출처: `https://protobuf.dev/programming-guides/proto3/#json`
- **A6. 게이트웨이 이름/위치는 업링크에서 얻을 수 없다.** 업링크가 제공하는 것은 `gatewayId`
  뿐이다. 이름/설명/위치/state/last_seen_at 은 gRPC `GatewayService.List`
  (`GatewayListItem`)에 있다. 게이트웨이 stats MQTT 토픽(`eu868/gateway/+/event/stats`)은
  본 에이전트가 구독하는 **애플리케이션 토픽이 아니라 게이트웨이 백엔드 브로커**에 있다.
  → v1 탭은 **hex ID 표시**이며, 이름/위치는 Non-Goal 이자 후속 확장 지점이다.
  출처: `https://github.com/chirpstack/chirpstack/blob/master/api/proto/api/gateway.proto`,
  `https://www.chirpstack.io/docs/chirpstack-gateway-bridge/backends/mqtt.html`
- **A7. 게이트웨이 로스터는 업링크에서만 추론된다** (A6 의 귀결). 따라서
  (a) 담당 디바이스가 모두 침묵 중인 게이트웨이는 보이지 않고,
  (b) 커버하는 디바이스가 하나도 없는 게이트웨이는 애초에 발견되지 않는다.
  이는 설계상의 한계이며 R3 에 위험으로 기록한다.
- **A8. exec 커맨드 경로는 xsfm 선례와 동일하게 동작한다** — `POST /agents/{id}/exec` 는
  `ag.Process(data)` 를 호출하고 결과 JSON 을 그대로 반환하며
  (`internal/api/service/agent_adapter.go:644-670`), 조회 커맨드는 저장소 영속화 분기(`:662`)에
  걸리지 않는다.

## Requirements (EARS)

### 프로즌 캐리오버 — SPEC-CHIRPSTACK-001 / 002 (변경 금지)

- **REQ-FROZEN-A (수신 계약 보존)**: The system **shall not** change SPEC-001 의 per-measurement
  fan-out 및 다운스트림 읽기 경로 `$.payload.value`, `$.metadata.measurement`,
  `$.metadata.device.*`, `$.metadata.tags.*`, `$.timestamp`, device_state fold,
  tags verbatim pass-through, epoch = `int64 UnixMilli`.
  (SPEC-001 REQ-FROZEN-02 / SPEC-002 REQ-FROZEN-A 승계)
- **REQ-FROZEN-B (락 트랩 회피)**: While a chirpstack 에이전트 함수가 lock(`devicesMu`/`commMu`)
  을 보유 중이면, the system **shall** pre-capture `agentName`(`a.Name()`) before entering the
  locked section, and **shall not** hold two of these locks simultaneously — HVAC 재귀 RLock
  deadlock 트랩(v0.18.6) 재현 금지. 확립된 패턴은 `provider.go:311-341` `deviceAdapters()` 이다
  (락 밖 선캡처 → 락별 복사 → 락 밖 병합).
- **REQ-FROZEN-C (SPEC-002 노출면 보존)**: The system **shall not** change the `chirpstack-status`
  방출 shape(`payload.state = {online, rssi, snr, gateway_id, last_seen_ms}`), the
  `chirpstack-control` 다운링크 페이로드 계약, `CommSnapshot`/`CommStateRecordJSON`/
  `CommStateEnabled`/`DownlinkTarget` 접근자 시그니처, 또는 `commEntry` 의 필드 의미
  (best-gateway 스칼라). 신규 링크 데이터는 comm 맵이 아니라 **로스터 경로**에 둔다(REQ-M2-01).

### M1 — rxInfo/txInfo 파싱 확장

- **REQ-M1-01 (Ubiquitous)**: The system **shall** parse **every** entry of `rxInfo[]`, keyed by
  `gatewayId`, and **shall not** discard non-maximum-RSSI entries in the roster path.
  WHY: A1 — 하나의 업링크는 여러 게이트웨이가 수신하며 (gateway, device) 는 쌍이다.
  IMPACT: 미이행 시 게이트웨이 탭은 디바이스당 1개 게이트웨이만 보이는 현행과 동일해진다.
- **REQ-M1-02 (Ubiquitous)**: The system **shall** extend `uplinkRxInfo`
  (`decode.go:25-29`) with the `channel` field (`uint32`), parsed as a **value type** so that an
  absent JSON key yields `0`.
  WHY: A5 — proto3 JSON 은 `channel: 0` 을 생략하므로, nullable 표현을 쓰면 정당한 채널 0 이
  "미상" 으로 오독된다.
  IMPACT: nullable 로 구현하면 채널 0 게이트웨이가 전부 "unknown" 으로 표시된다.
- **REQ-M1-03 (Ubiquitous)**: The system **shall not** use the `rxInfo[]` array index as an
  identity, and **shall not** assume `rxInfo[0]` is the best or the first-received gateway.
  WHY: A2 — dedup 이 Redis Set 이라 순서 보장이 없다.
  IMPACT: 인덱스 키잉 시 같은 게이트웨이가 업링크마다 다른 슬롯으로 튀어 링크 이력이 뒤섞인다.
- **REQ-M1-04 (Ubiquitous)**: The system **shall** parse the frame-level `txInfo`
  (`frequency` Hz, `modulation.lora.spreadingFactor`, `modulation.lora.bandwidth`) once per uplink
  and **shall** attach it to every link sample produced from that uplink.
  WHY: A4 — `channel` 은 게이트웨이 로컬 IF 인덱스이며 의미가 약하다. 실제 주파수는 프레임
  레벨의 `txInfo.frequency` 이고 획득 비용이 사실상 0 이다. (잠정 결정 F-3)
  IMPACT: 미포함 시 UI 는 게이트웨이 로컬 채널 인덱스만 보여줄 수 있고, 사용자가 이를 주파수로
  오해할 여지가 남는다.
- **REQ-M1-05 (Unwanted)**: The system **shall not** change `bestGateway`
  (`decode.go:80-87`) semantics or its comm-state caller (`watchdog.go:125`).
  WHY: REQ-FROZEN-C — SPEC-002 status 노드의 방출 shape 가 그 스칼라에 의존한다.
  IMPACT: 변경 시 SPEC-002 AC-2 가 회귀한다.

### M2 — (device, gateway) 링크 캐시 + 게이트웨이 로스터 파생

- **REQ-M2-01 (Ubiquitous, 배치 결정)**: The system **shall** store gateway link state in the
  **devices roster path** (`deviceState`, `upsertDevice`), **not** in the comm map.
  WHY: `upsertDevice`(`provider.go:193-262`)는 모든 업링크마다 무조건 실행되는 반면 comm 맵
  갱신은 `emit_comm_state`(기본 false) 게이트 뒤에 있다(`agent.go:309-311`). comm 맵에 두면
  그 노브가 꺼진 에이전트에서 게이트웨이 탭이 **조용히 빈 화면**이 된다. 이는
  `applicationID`/`measurements` 캐시에 이미 문서화된 동일 근거이다(`provider.go:63-88`).
  IMPACT: comm 맵 배치 시 SPEC-002 IN-3 과 동일한 함정(노브 종속 침묵 실패)이 재발한다.
- **REQ-M2-02 (Ubiquitous)**: The system **shall** keep, per (devEui, gatewayId) pair, the
  **latest** link sample: `{rssi, snr, channel, frequency_hz, spreading_factor, bandwidth,
  last_seen_ms}`. `last_seen_ms` 는 `int64 UnixMilli` 이다(REQ-FROZEN-A 시각 규약).
  (잠정 결정 F-1: 최신 1건 유지, 이력 미보관)
- **REQ-M2-03 (Ubiquitous, 메모리 상한)**: The system **shall** cap the number of distinct
  gateways cached per device at `maxCachedGatewayLinks` (잠정값 **8**), mirroring the
  `maxCachedMeasurements = 64` precedent (`provider.go:19`). While the cap is reached and a new
  `gatewayId` arrives, the system **shall** evict the entry with the **oldest `last_seen_ms`**
  (결정적 eviction) and admit the new one; 기존 키의 갱신은 항상 허용된다.
  WHY: 전역 메모리 상한을 `디바이스 수 × 8` 로 **구조적으로** 고정한다 — 별도 전역 게이트웨이
  맵/reaper goroutine 없이 상한이 성립한다. (잠정 결정 F-5)
  IMPACT: 상한이 없으면 오작동/스푸핑 게이트웨이가 로스터를 무한히 부풀린다.
- **REQ-M2-04 (Ubiquitous, 파생 로스터)**: The system **shall** derive the gateway roster by
  **inverting** the per-device link maps at query time (게이트웨이별 디바이스 목록), and
  **shall not** maintain a second authoritative gateway map.
  WHY: 단일 소스 유지 — 두 맵을 각각 갱신하면 조용히 갈라진다. 또한 신규 mutex 가 필요 없어
  REQ-FROZEN-B 의 락 순서 엣지를 새로 만들지 않는다.
  IMPACT: 별도 맵 도입 시 `devicesMu`↔신규 락 순서 엣지가 생긴다.
- **REQ-M2-05 (State-driven, staleness)**: While a link sample's `last_seen_ms` is older than the
  agent's `offline_threshold`, the system **shall** mark that link `stale = true` in query output
  and **shall not** delete it purely on staleness. 하드 제거는 REQ-M2-03 의 cap-pressure
  eviction 으로만 발생한다.
  WHY: 배경 reaper goroutine 없이 "범위를 벗어난 게이트웨이가 영원히 붙어 보이는" 문제를
  표시 계층에서 해소한다. 판정 방향은 `deviceOnline`(`provider.go:174-182`)과 동일한 파생 방식을
  따른다(저장된 불리언 금지 — 영구 online 굳음 결함 재발 방지). (잠정 결정 F-2)
  IMPACT: 삭제로 처리하면 "잠깐 안 들리던 게이트웨이" 가 사라졌다 나타났다를 반복한다.
- **REQ-M2-06 (Ubiquitous, 락 규율)**: The system **shall** read/write the link cache under
  `devicesMu` only, with `agentName` pre-captured outside the lock, and **shall not** acquire
  `commMu` while holding `devicesMu` (REQ-FROZEN-B, `provider.go:311-341` 패턴).

### M3 — exec 커맨드 디스패처 + `list_gateways`

- **REQ-M3-01 (Ubiquitous, 명시적 역전)**: The system **shall** implement a command dispatcher in
  `ChirpStackAgent.Process`, intentionally reversing the current deliberate no-op at
  `internal/agent/chirpstack/agent.go:491-495` ("Process 는 사용하지 않는다").
  주석과 관련 선언을 함께 갱신하여 역전을 명시한다.
  WHY: SPEC-001/002 는 Process 표면을 의도적으로 비워 두었고, 본 SPEC 이 그 배제를 역전한다.
  SPEC-002 REQ-M1-02(발행 경로 역전)와 동일한 기록 규율이다.
  IMPACT: 미기록 시 후속 리뷰어가 SPEC-001/002 대비 **회귀**로 오독한다.
- **REQ-M3-02 (Event-driven)**: When the agent receives an exec request with
  `command == "list_gateways"`, the system **shall** return the derived gateway roster as JSON:
  게이트웨이 배열, 각 항목에 `gateway_id`, `device_count`, `last_seen_ms`(해당 게이트웨이의 링크
  중 최신), 그리고 `devices[]`(각 항목: `dev_eui`, `device_id`, `device_name`,
  `device_profile_name`, `rssi`, `snr`, `channel`, `frequency_hz`, `spreading_factor`,
  `bandwidth`, `last_seen_ms`, `stale`).
- **REQ-M3-03 (Ubiquitous, 결정성)**: The system **shall** return gateways sorted by `gateway_id`
  and devices within each gateway sorted by `dev_eui`, so that repeated calls with unchanged state
  produce byte-identical output (맵 순회 순서 비의존).
- **REQ-M3-04 (Unwanted)**: The `list_gateways` command **shall not** perform any MQTT publish,
  any on-demand poll, or any repository write — 순수 캐시 조회이다.
  WHY: `internal/api/service/agent_adapter.go:662` 의 영속화 분기는 `add_device`/`remove_device`/
  `set_device` 에만 걸리므로 조회 커맨드는 저장소 경로에 닿지 않아야 한다.
- **REQ-M3-05 (Unwanted, 대안 거부 기록)**: The system **shall not** expose the gateway roster by
  extending `State()` (`agent.go:647-665`).
  WHY: 에이전트 LIST 페이지는 모든 에이전트에 `detail=summary` 로 질의하고,
  `internal/api/service/agent_adapter.go:619-627` 은 하드코딩된 `"entries"` 키 하나만 제거한다.
  게이트웨이 목록을 `State()` 에 넣으면 **모든 LIST 응답이 비대해지고**, 그것을 막으려면
  다수 에이전트가 공유하는 파일을 수정해야 한다.
  IMPACT: 미기록 시 후속 구현자가 "이미 REST 로 노출된 State() 가 더 쉽다" 며 되돌린다.
- **REQ-M3-06 (Unwanted)**: The system **shall not** introduce a new Flow node type, and
  **shall not** modify `internal/node/registry.go`, `pkg/flow/validate.go`,
  `web/src/config/nodeSchemas.ts`, or the hardcoded node-type-count assertions
  (SPEC-002 IN-9 의 3곳).
  WHY: 게이트웨이 조회는 관리 UI 관심사이지 플로우 런타임 관심사가 아니다.
  IMPACT: 노드 추가 시 SPEC-002 IN-10(팔레트↔스키마 이중 관리) 부담이 그대로 재발한다.

### M4 — 웹 게이트웨이 탭

- **REQ-M4-01 (Ubiquitous)**: The frontend **shall** expose a ChirpStack-only "게이트웨이" tab via
  the existing per-agent-type gate precedent — `HAS_GATEWAYS_TAB = new Set<string>(['chirpstack'])`
  (`web/src/pages/agents/AgentDetailPanel.tsx:186` 의 `HAS_STATIONS_TAB` 패턴), with the tab
  component in its own file `ChirpstackGatewaysTab.tsx` (`XsfmStationsTab.tsx` 명명 관례).
- **REQ-M4-02 (Ubiquitous)**: The tab **shall** fetch via
  `agentService.execAgent(agentId, { command: 'list_gateways' })` inside a react-query hook with
  polling (`web/src/hooks/useStation.ts:120-131` 관용구). SSE 는 사용하지 않는다(원격 프록시 전용).
- **REQ-M4-03 (Ubiquitous)**: The tab **shall** render one row per gateway with an expandable
  device sub-list (`XsfmStationsTab.tsx:515-556` / `DeviceListPage.tsx:850-917` 아코디언 선례),
  reusing `SortableHeader` (`web/src/components/common/SortableHeader.tsx:29`) and
  `TablePagination` (`web/src/pages/agents/TablePagination.tsx`). 모든 표시 문자열은
  `web/src/lib/i18n/{en,ko}.json` 키를 통한다.
- **REQ-M4-04 (Ubiquitous, 표기 정확성)**: The UI **shall** label `channel` as the receiving
  gateway's IF channel index (게이트웨이 로컬 값) and **shall not** present it as the frequency;
  the frequency column **shall** be sourced from `frequency_hz`.
  WHY: A4.
  IMPACT: 오표기 시 사용자가 게이트웨이 간 채널 인덱스를 주파수로 비교해 잘못된 운영 판단을 한다.
- **REQ-M4-05 (State-driven)**: While a link is `stale`, the UI **shall** visually distinguish it
  (예: 흐린 처리 + 마지막 수신 상대시각) and **shall not** present it as a live link.
- **REQ-M4-06 (Unwanted)**: The system **shall not** add new agent config knobs and **shall not**
  change `CHIRPSTACK_FIELDS` / `agentSchemas.ts` — 게이트웨이 탭은 순수 조회 표면이다.

### M5 — 테스트 + 품질 게이트

- **REQ-M5-01 (Ubiquitous)**: The system **shall** provide table-driven tests covering:
  다중 게이트웨이 rxInfo 보존(3개 → 3링크, gatewayId 키잉), 순서 무관성(rxInfo 셔플 시 동일 결과),
  `channel` 키 부재 → 0, cap 도달 시 최고령 eviction 결정성, `list_gateways` 응답 shape/정렬 결정성,
  0 publish / 0 poll, stale 파생, `emit_comm_state=false` 에서도 로스터가 채워짐,
  SPEC-001/002 무회귀 characterization.
- **REQ-M5-02 (Ubiquitous)**: The system **shall** run `go test -race` clean on the chirpstack
  package (동시 업링크 + `list_gateways` 동시 호출 케이스 포함) and **shall** achieve ≥85%
  coverage on new code.
- **REQ-M5-03 (Ubiquitous)**: The system **shall** pass TRUST 5 gates and LSP run-phase
  (zero errors / type / lint) **delta 기준**으로 판정한다 — 저장소 전체는 선재 결함
  (`internal/schedulelog/observer_test.go`, SPEC-002 DoD 단서)으로 RED 이므로 절대값 판정 금지.
- **REQ-M5-04 (Ubiquitous, run-phase 전제조건)**: The implementation **shall** verify A5
  (`channel: 0` 이 실제 ChirpStack JSON 에서 생략되는지) against a **live broker capture or a real
  captured fixture** before claiming it verified. 검증 전까지 "검증됨" 표기 금지(R1).

## Specifications (사양 상세)

### 링크 샘플 계약

per (devEui, gatewayId) 최신 1건:

| 필드 | 타입 | 출처 | 비고 |
|------|------|------|------|
| `gateway_id` | string | `rxInfo[].gatewayId` | EUI64 hex, 키 |
| `rssi` | int | `rxInfo[].rssi` | 게이트웨이별 |
| `snr` | float64 | `rxInfo[].snr` | 게이트웨이별 |
| `channel` | uint32 | `rxInfo[].channel` | 게이트웨이 로컬 IF 인덱스. **키 부재 = 0** (A5) |
| `frequency_hz` | uint64 | `txInfo.frequency` | **프레임 레벨** — 모든 rxInfo 공통 (A4) |
| `spreading_factor` | uint32 | `txInfo.modulation.lora.spreadingFactor` | 프레임 레벨 |
| `bandwidth` | uint32 | `txInfo.modulation.lora.bandwidth` | 프레임 레벨 |
| `last_seen_ms` | int64 | 업링크 파생 시각 | `UnixMilli` (REQ-FROZEN-A) |
| `stale` | bool | 파생 | `last_seen_ms` 가 `offline_threshold` 초과 시 true (저장 안 함) |

### `list_gateways` 응답 shape

```
{
  "gateways": [
    {
      "gateway_id": "0016c001f1500812",
      "device_count": 2,
      "last_seen_ms": 1765432100000,
      "devices": [
        {
          "dev_eui": "24e124141d180806",
          "device_id": "<UUID v4 | \"\">",
          "device_name": "...",
          "device_profile_name": "WS301",
          "rssi": -87, "snr": 9.25, "channel": 3,
          "frequency_hz": 922100000, "spreading_factor": 7, "bandwidth": 125000,
          "last_seen_ms": 1765432100000, "stale": false
        }
      ]
    }
  ]
}
```

- `gateways` 는 `gateway_id` 오름차순, `devices` 는 `dev_eui` 오름차순 (REQ-M3-03).
- `device_id` 는 `agent.ResolveDeviceID(agentName, devEui)` 의 UUID v4 이며 저장소 미설정 시
  빈 문자열이다(`provider.go:414-416` 과 동일 규약, SPEC-DEVICE-IDENTITY-001 Phase D).
- 게이트웨이 이름/위치 필드는 **없다**(A6, Non-Goal).

### 저장 위치 및 상한

- `deviceState`(`provider.go:77-88`)에 `links map[string]gatewayLink` 를 추가한다
  (키 = `gatewayId`). `clone()` 은 이 맵도 깊은 복사한다(`provider.go:94-112` 계약 확장).
- 상한 `maxCachedGatewayLinks`(잠정 8) 도달 시 최고령 `last_seen_ms` eviction(REQ-M2-03).
- 전역 메모리 상한 = `디바이스 수 × maxCachedGatewayLinks`. 별도 전역 맵/reaper 없음.

### Non-Goals (v1 범위 밖)

- 게이트웨이 이름 / 설명 / 위치 / state / last_seen_at (gRPC `GatewayService.List` 필요, A6).
- 게이트웨이 stats(수신/송신 카운트, RX/TX 패킷) — 게이트웨이 백엔드 브로커 토픽 필요(A6).
- 게이트웨이 CRUD(등록/삭제/편집) — 로스터는 업링크 파생 read-only 이다.
- Flow 노드 타입 추가(REQ-M3-06), control-panel / REST `/execute` 어댑터.
- 링크 품질 시계열 이력(F-1: 최신 1건만).

### 미결 설계 결정 (사용자 확정 대기)

아래 5건은 **잠정 결정 + 근거**를 요구사항에 반영해 두었으나 사용자 확정이 필요하다.
상세 대안 비교와 권고는 `plan.md` §6 참조.

| ID | 쟁점 | 잠정 결정 | 반영 요구사항 |
|----|------|-----------|---------------|
| F-1 | 링크 이력 보관 | 최신 1건만 | REQ-M2-02 |
| F-2 | 링크 만료 정책 | staleness 파생 표시 + cap-pressure eviction (삭제 없음) | REQ-M2-05 |
| F-3 | `frequency`/SF 동반 수집 | 수집한다 | REQ-M1-04 |
| F-4 | 디바이스 역방향 조회 | v1 게이트웨이-우선만, 디바이스 상세 역방향은 후속 | REQ-M3-02 (Non-Goal) |
| F-5 | 메모리 상한 | 디바이스당 게이트웨이 8개 cap + 최고령 eviction | REQ-M2-03 |

## Risks (위험 — 정직 기록)

- **R1 (High, 미검증)**: **A5 는 실브로커로 검증되지 않았다.** `channel: 0` 이 ChirpStack 의
  Rust JSON 인코더에서 실제로 생략되는지는 proto3 JSON 매핑 규격
  (`https://protobuf.dev/programming-guides/proto3/#json`)과 proto 정의로부터의 **추론**이며,
  라이브 브로커 캡처로 확인하지 않았다. 이는 본 SPEC 에서 **가장 발생 가능성이 높은 파싱 결함**
  이다. **run-phase 전제조건**: 실 캡처(또는 실제 캡처 픽스처)로 `channel` 키의 존재/부재를
  확인하기 전에는 "검증됨" 을 주장하지 않는다(REQ-M5-04). 완화: 값 타입 `uint32` 파싱은 어느
  쪽이든 0 을 산출하므로 nullable 표현만 금지하면 실패 양상이 양성이다(REQ-M1-02).
  (SPEC-002 A6/R2 와 동일한 기록 규율)
- **R2 (Medium)**: **`channel` 의 의미가 게이트웨이 로컬 IF 인덱스**라는 점(A4)이 UI 에서
  주파수로 오표기될 위험. 두 게이트웨이의 `channel: 3` 은 같은 주파수를 뜻하지 않는다.
  완화: REQ-M4-04(라벨 규정) + `frequency_hz` 별도 컬럼(REQ-M1-04).
- **R3 (Medium, 구조적 한계)**: **로스터가 업링크 파생이라 불완전하다**(A7). 담당 디바이스가
  모두 침묵 중인 게이트웨이는 사라지고, 커버 디바이스가 없는 게이트웨이는 발견되지 않는다.
  완화: v1 은 한계를 UI 문구/문서로 공개한다. 근본 해소는 gRPC `GatewayService.List` 통합이며
  별도 SPEC 이다.
- **R4 (Medium)**: **`Process` no-op 역전**(REQ-M3-01) — SPEC-001/002 의 의도적 배제를 역전하므로
  `agent.go:491-495` 주석 갱신을 커밋/주석에 명시하지 않으면 회귀로 오인된다.
  (SPEC-002 R1 과 동형)
- **R5 (Medium)**: **락 안전** — 링크 캐시는 `devicesMu` 하에 두고 `commMu` 와 절대 중첩하지
  않는다. `a.Name()` 은 락 밖 선캡처(REQ-FROZEN-B, HVAC v0.18.6 재귀 RLock 트랩).
  `list_gateways` 는 업링크 수신과 동시 실행되므로 `-race` 검증이 필수다(REQ-M5-02).
- **R6 (Medium)**: **메모리** — 링크 캐시는 디바이스 × 게이트웨이 이다. 상한이 없으면 오작동/
  스푸핑 게이트웨이 ID 가 로스터를 부풀린다. 완화: REQ-M2-03 cap(8) + 결정적 eviction.
  잔여: `devices` 맵 자체는 현재도 상한이 없다(선재 조건, 본 SPEC 범위 밖).
- **R7 (Low)**: **exec 응답 크기** — `list_gateways` 는 디바이스 × 8 링크를 상한으로 하므로
  대규모 배치에서 응답이 커질 수 있다. 완화: 웹은 클라이언트 페이지네이션
  (`TablePagination`)을 쓰고 폴링 주기를 보수적으로 잡는다. 서버측 페이징은 후속.
- **R8 (Low)**: **rxInfo 순서 무보장**(A2) — 인덱스 키잉/`rxInfo[0]` 가정 시 링크가 뒤섞인다.
  완화: REQ-M1-03 + 셔플 테스트(REQ-M5-01).
- **R9 (Low)**: `gwTime`/`location` 은 대개 부재하고(A3) `metadata` 는 스키마 비보장이므로,
  v1 은 이 필드들을 소비하지 않는다. 후속에서 쓰려면 부재 처리를 먼저 설계해야 한다.

## Traceability (추적성)

- `plan.md`: 기술 접근, 우선순위 마일스톤(M1~M5), 아키텍처 방향, 위험 대응, 미결 결정 F-1~F-5.
- `acceptance.md`: Given-When-Then AC-1~AC-8 + TRUST 5 + DoD.
- 선행 SPEC: SPEC-CHIRPSTACK-001(completed, 수신 에이전트 + comm-state),
  SPEC-CHIRPSTACK-002(completed, status/control 노드 + 다운링크 코덱).
- 관련 SPEC: SPEC-DEVICE-001(통합 레지스트리), SPEC-DEVICE-IDENTITY-001(Phase D, ID==UID==UUID v4)
  — SPEC-001/002 계약과 동일 승계.
- 외부 계약 출처: ChirpStack `integration.proto` / `gw.proto` / `api/gateway.proto` /
  `chirpstack/src/uplink/mod.rs`, Semtech `packet_forwarder/PROTOCOL.TXT`, protobuf proto3 JSON 매핑.
