---
id: SPEC-CHIRPSTACK-003
title: "ChirpStack 게이트웨이 관리 — 구현 계획"
version: 0.1.0
status: draft
created: 2026-08-13
updated: 2026-08-13
author: xtra
priority: P1
phase: plan
module: "internal/agent/chirpstack"
lifecycle: spec-anchored
tier: M
tags: [chirpstack, lorawan, gateway, rx-info, rssi, snr, channel, exec, web-tab]
---

# ChirpStack 게이트웨이 관리 — 구현 계획 (plan.md)

## 1. 기술 접근

- **rxInfo 전량 보존**: `decode.go:25-29` `uplinkRxInfo` 에 `channel uint32` 를 **값 타입**으로
  추가하고(A5 — 키 부재 = 0), `uplink` 구조체(`decode.go:34-39`)에 프레임 레벨 `txInfo`
  (`frequency`, `modulation.lora.{spreadingFactor,bandwidth}`)를 추가한다. 파싱은 추가적이며
  `bestGateway`(`decode.go:80-87`)와 그 유일 호출자(`watchdog.go:125`)는 **무변경**이다
  (REQ-M1-05 / REQ-FROZEN-C).
- **링크 캐시는 로스터 경로**: `deviceState`(`provider.go:77-88`)에 `links map[string]gatewayLink`
  를 추가하고 `upsertDevice`(`provider.go:193-262`)에서 병합한다. comm 맵이 아닌 이유는
  `applicationID`/`measurements` 캐시에 이미 문서화된 것과 동일하다 — `upsertDevice` 는 모든
  업링크마다 무조건 돌지만 comm 맵은 `emit_comm_state`(기본 false) 게이트 뒤에 있다
  (`agent.go:309-311`). SPEC-002 IN-3 이 실제로 겪은 함정을 재발시키지 않는다.
- **게이트웨이 로스터는 파생**: 별도 전역 맵/mutex 를 만들지 않고, `listDevices()` 스냅샷
  (`provider.go:287-295`)을 락 밖에서 **역인덱싱**해 게이트웨이별 디바이스 목록을 만든다.
  단일 소스이므로 두 표면이 갈라질 수 없고, 신규 락 순서 엣지도 생기지 않는다(REQ-FROZEN-B).
- **노출 경로는 exec 커맨드**: `ChirpStackAgent.Process`(`agent.go:491-495`, 현재 no-op)에
  xsfm 선례(`internal/agent/xsfm/agent.go:855-889`)와 동형의 디스패처를 넣고
  `case "list_gateways"` 를 구현한다. REST 경로는 기존
  `POST /agents/{id}/exec`(`internal/api/handler/agent.go:230`) →
  `ExecAgent`(`internal/api/service/agent_adapter.go:644-670`) 를 그대로 탄다. 조회 커맨드는
  영속화 분기(`:662`)에 걸리지 않으므로 저장소 부작용이 없다.
- **`State()` 확장을 거부한 근거**: `State()` 는 이미 REST 로 노출되지만, 에이전트 LIST 페이지는
  모든 에이전트에 `detail=summary` 로 질의하고 `agent_adapter.go:619-627` 은 하드코딩된
  `"entries"` 키 하나만 제거한다. 게이트웨이 목록을 넣으면 모든 LIST 응답이 비대해지고, 이를
  막으려면 다수 에이전트가 공유하는 파일을 고쳐야 한다(blast radius). → REQ-M3-05 로 명문화.
- **웹**: `HAS_GATEWAYS_TAB = new Set(['chirpstack'])` 게이트
  (`AgentDetailPanel.tsx:186` 의 `HAS_STATIONS_TAB` 패턴) + 전용 파일
  `ChirpstackGatewaysTab.tsx`(`XsfmStationsTab.tsx` 명명 관례). 데이터는 react-query 폴링
  (`useStation.ts:120-131` 관용구). 신규 노드 타입 없음 → `nodeSchemas.ts` / registry /
  노드 타입 수 단언 3곳(SPEC-002 IN-9) 무변경.
- **방법론**: Hybrid — 신규 코드(링크 캐시, 파생 로스터, 디스패처, 탭) TDD, SPEC-001/002 수신·
  status·다운링크 경로 무회귀는 characterization 으로 방어.

## 2. 우선순위 기반 마일스톤

> 시간 예측 없음. 우선순위 라벨과 의존 순서로만 기술한다.

### Primary Goal (Priority High)

- **M1 — rxInfo/txInfo 파싱 확장**
  - `uplinkRxInfo` 에 `channel uint32` 추가(값 타입 — nullable 금지, A5/REQ-M1-02).
  - `uplink` 에 `txInfo` 추가: `frequency`(Hz), `modulation.lora.spreadingFactor`, `bandwidth`
    (REQ-M1-04).
  - `bestGateway` / comm 경로 무변경 확인(REQ-M1-05, characterization).
  - 게이트웨이 식별은 `gatewayId` 만 — 인덱스/`rxInfo[0]` 금지(REQ-M1-03).
- **M2 — (device, gateway) 링크 캐시 + 파생 로스터**
  - `deviceState.links map[string]gatewayLink` + `clone()` 깊은 복사 확장.
  - `upsertDevice` 에서 rxInfo 전량 병합 + 프레임 레벨 txInfo 부착(REQ-M2-02).
  - `maxCachedGatewayLinks`(잠정 8) cap + 최고령 `last_seen_ms` eviction(REQ-M2-03).
  - `listGateways()` — `listDevices()` 스냅샷을 락 밖에서 역인덱싱, `gateway_id`/`dev_eui`
    정렬(REQ-M2-04, REQ-M3-03), staleness 파생(REQ-M2-05, `deviceOnline` 과 동일 방향).
  - 락 규율: `devicesMu` 단독, `agentName` 락 밖 선캡처(REQ-M2-06 / REQ-FROZEN-B).
- **M3 — exec 디스패처 + `list_gateways`**
  - `Process` 디스패처 구현 + `agent.go:491-495` 주석/선언 갱신으로 역전 명시(REQ-M3-01, R4).
  - `list_gateways` 응답 shape(spec.md §Specifications) + 결정적 정렬.
  - 미지 커맨드는 명확한 에러(xsfm `ErrInvalidCommand` 관용구 미러).
  - 0 publish / 0 poll / 0 repo-write 구조적 보장(REQ-M3-04).

### Secondary Goal (Priority Medium)

- **M4 — 웹 게이트웨이 탭**
  - `AgentDetailPanel.tsx`: `Tab` union(`:142`)에 `'gateways'` 추가, `HAS_GATEWAYS_TAB` 게이트
    (`:186` 인접), 탭 버튼(`:232`) + 렌더(`:249`).
  - `ChirpstackGatewaysTab.tsx`: 게이트웨이 행 + 확장형 디바이스 서브리스트
    (`XsfmStationsTab.tsx:515-556` / `DeviceListPage.tsx:850-917` 선례),
    `SortableHeader`(`SortableHeader.tsx:29`) + `TablePagination`.
  - `useChirpstackGateways` 훅(react-query 폴링, `useStation.ts:120-131` 관용구).
  - `channel` 라벨은 "게이트웨이 IF 채널", 주파수는 `frequency_hz` 별도 컬럼(REQ-M4-04, R2).
  - stale 링크 시각적 구분(REQ-M4-05).
  - i18n 키 `en.json` / `ko.json` 양쪽 추가.

### Optional (Priority Low)

- **M4-opt — 디바이스 상세 역방향 뷰** (F-4): 디바이스 하나가 붙은 게이트웨이 전량 표시.
  동일 링크 캐시에서 파생 가능하므로 데이터 작업은 0 이며 UI 작업만 남는다. v1 에서는 제외.

### 전 구간 병행

- **M5 — 테스트 + 품질 게이트**
  - table-driven: 다중 게이트웨이 보존, rxInfo 셔플 무관성, `channel` 키 부재 → 0,
    cap eviction 결정성, 응답 정렬 결정성, stale 파생, `emit_comm_state=false` 에서도 로스터 충전,
    SPEC-001/002 무회귀.
  - `go test -race`(업링크 수신 ↔ `list_gateways` 동시 호출), 신규 코드 커버리지 ≥85%.
  - LSP/lint 는 **delta 기준** 판정(저장소 선재 RED: `internal/schedulelog/observer_test.go`).

### 의존 순서

M1 → M2 → M3 → M4. M4-opt 는 M4 이후. M5 는 전 구간 병행.

## 3. 아키텍처 설계 방향

- **2계층 유지(SPEC-001/002 정신)**: 에이전트가 링크 캐시와 파생 로스터를 소유하고, 노출은
  얇은 어댑터(exec 커맨드 → 기존 REST → react-query 훅)만 담당한다.
- **단일 진실 소스**: 게이트웨이는 **저장된 엔티티가 아니라 파생 뷰**다. 두 번째 권위 맵을 두지
  않는 것이 이 SPEC 의 핵심 구조 결정이며, `deviceOnline`(`provider.go:160-182`)이 저장 불리언을
  버리고 파생으로 바꾼 것과 동일한 이유(드리프트 불가능성)를 따른다.
- **락 토폴로지 불변**: 신규 mutex 0개. 링크는 이미 존재하는 `devicesMu` 보호 구조 안에 들어가고,
  파생은 스냅샷을 락 밖에서 처리한다(`deviceAdapters()` §313-322 규율과 동형).
- **상한의 구조화**: 전역 상한을 별도 정책/reaper 로 두지 않고 `디바이스당 8` 이라는 국소 cap
  으로 환원한다 — `maxCachedMeasurements = 64`(`provider.go:19`)와 동일한 설계 어휘.
- **확장점**: 게이트웨이 이름/위치/stats 는 gRPC `GatewayService.List` 또는 게이트웨이 백엔드
  브로커가 필요하므로(A6) 별도 SPEC 의 확장 지점으로 남긴다. 본 SPEC 의 `gateway_id` 키는 그
  확장이 붙을 조인 키가 된다.

## 4. 위험 및 대응

| ID | 위험 | 대응 |
|----|------|------|
| R1 | `channel: 0` 생략 여부 **실브로커 미검증** | run-phase 전제조건으로 실 캡처 대조 명문화(REQ-M5-04). 그 전까지 "검증됨" 주장 금지. 값 타입 `uint32` 파싱으로 실패 양상을 양성화(REQ-M1-02). 필요 시 `@MX:DEBT` + `@MX:CEILING` + `@MX:UPGRADE` 표기 |
| R2 | `channel` 을 주파수로 오표기 | UI 라벨 규정(REQ-M4-04) + `frequency_hz` 별도 컬럼(REQ-M1-04) + Go doc 에 A4 근거 기록 |
| R3 | 업링크 파생 로스터의 구조적 불완전성 | v1 은 한계 공개(탭 안내 문구 + Go doc). gRPC 통합은 후속 SPEC |
| R4 | `Process` no-op 역전이 회귀로 오인 | `agent.go:491-495` 주석 갱신 + 커밋 메시지에 REQ-M3-01 명시(SPEC-002 R1/REQ-M1-02 와 동일 규율) |
| R5 | 락 안전 / 데이터 레이스 | `devicesMu` 단독 + `agentName` 선캡처. `-race` 로 업링크↔조회 동시성 검증(REQ-M5-02) |
| R6 | 메모리 증가 | 디바이스당 cap 8 + 최고령 eviction(REQ-M2-03). 전역 상한 = 디바이스 수 × 8 |
| R7 | exec 응답 비대 | 클라이언트 페이지네이션 + 보수적 폴링 주기. 서버측 페이징은 후속 |
| R8 | rxInfo 순서 무보장 | `gatewayId` 키잉 강제(REQ-M1-03) + 셔플 테스트 |
| R9 | `gwTime`/`location`/`metadata` 부재·비보장 | v1 미소비. 후속 사용 시 부재 처리 선설계 |

## 5. 라이브러리 / 버전

- **신규 외부 의존 없음.** Go 측은 표준 `encoding/json` + 기존 paho MQTT 만 사용한다.
- 웹 측도 신규 패키지 없음 — 기존 react-query / lucide-react / Tailwind + 기존 공용 컴포넌트
  (`SortableHeader`, `TablePagination`, `BulkRegisterPanel` 계열)를 재사용한다.
- 정확한 버전 확정은 run-phase 에서 `go.mod` / `web/package.json` 실측으로 수행한다.

## 6. 미결 설계 결정 (F-1 ~ F-5) — 사용자 확정 대기

각 항목은 **권고안 + 근거 + 대안의 비용**을 함께 기술한다. 요구사항에는 권고안이 잠정 반영되어
있으며, 사용자가 다르게 결정하면 해당 REQ 를 수정한다.

### F-1. 링크 이력 보관 범위 (→ REQ-M2-02)

- **권고: (device, gateway) 당 최신 1건만 보관.**
- 근거: 요구사항은 "디바이스 연결 정보로 RSSI, SNR, Channel 저장" 이며 현재 상태 조회가 목적이다.
  이력은 시계열 저장소(influxdb 에이전트 경로)의 관심사이고, 인메모리 링 버퍼는 상한·만료·
  직렬화 비용을 모두 새로 도입한다.
- 대안: 링당 최근 N개 링 버퍼 — 메모리가 `디바이스 × 게이트웨이 × N` 으로 늘고, exec 응답도
  N배가 된다. 필요해지면 링 버퍼는 추가적(additive) 변경이므로 나중에 붙일 수 있다.

### F-2. 링크 만료(eviction) 정책 (→ REQ-M2-05, REQ-M2-03)

- **권고: staleness 는 파생 표시로만, 하드 제거는 cap-pressure LRU 로만.**
- 근거: 배경 reaper goroutine 을 추가하지 않으면서 "범위를 벗어난 게이트웨이가 영원히 붙어
  보이는" 문제를 해소한다. `deviceOnline`(`provider.go:160-182`)이 저장 불리언 대신 파생을
  택한 것과 같은 이유 — 저장된 상태는 되돌릴 주체가 없으면 굳는다.
- 대안 A(TTL 하드 삭제 + reaper): goroutine 1개 추가, 생명주기 관리(REQ-M6-03 계열 누수 위험),
  잠깐 침묵한 게이트웨이가 사라졌다 나타났다 깜빡인다.
- 대안 B(만료 없음): 이동한 디바이스가 옛 게이트웨이를 영구히 매달고 다닌다.

### F-3. `txInfo.frequency` / SF 동반 수집 (→ REQ-M1-04)

- **권고: 수집한다.**
- 근거: 사용자는 Channel 만 요청했으나, A4 에 따르면 `channel` 은 게이트웨이 로컬 IF 인덱스라
  단독으로는 오해를 부른다. `frequency` 는 프레임 레벨 필드 1개를 더 파싱하는 비용뿐이며
  (게이트웨이 수와 무관), Channel 을 정확히 해석할 수 있게 만드는 유일한 값이다.
- 대안: Channel 만 저장 — 요구사항에는 부합하나 R2(오표기 위험)를 완화할 수단이 없어진다.

### F-4. 디바이스 역방향 조회 (→ 현재 Non-Goal)

- **권고: v1 은 게이트웨이-우선만. 디바이스 상세의 역방향 뷰는 Priority Low 옵션(M4-opt).**
- 근거: 사용자 요구는 "게이트웨이와 연결되어 있는 디바이스 리스트" 이므로 게이트웨이-우선이
  1급이다. 데이터는 이미 디바이스별로 저장되므로 역방향 뷰는 **데이터 작업 0, UI 작업만**
  남는다 — 나중에 붙여도 비용이 늘지 않는다.
- 참고: `chirpDeviceAdapter.properties()`(`provider.go:476-491`)에 `gateways` 키를 넣는 방식도
  가능하나, 이는 `device.DeviceState.Properties` 를 통해 **모든 디바이스 조회 응답**에 실리므로
  응답 비대화를 부른다. 결정 시 이 비용을 함께 판단해야 한다.

### F-5. 메모리 상한 (→ REQ-M2-03)

- **권고: 디바이스당 게이트웨이 8개 cap, 초과 시 최고령 `last_seen_ms` eviction.**
- 근거: 실제 배치에서 한 디바이스를 듣는 게이트웨이는 보통 1~3개이고 8은 충분한 여유다.
  전역 상한이 `디바이스 수 × 8` 로 **구조적으로** 성립해 별도 전역 정책이 필요 없다.
  선례: `maxCachedMeasurements = 64`(`provider.go:19`).
- cap 도달 시 동작: 새 `gatewayId` 는 최고령 항목을 밀어내고 입장한다. 기존 키의 값 갱신은
  cap 과 무관하게 항상 허용된다(`mergeMeasurements` 와 동일한 의미론, `provider.go:126-139`).
- 대안: 전역 게이트웨이 수 cap — 어떤 디바이스의 링크를 버릴지 결정이 비국소적이 되고,
  결정성 확보가 어렵다.

## 7. Traceability

- spec.md REQ-FROZEN-A/B/C, M1~M5 ↔ 본 계획 §2 마일스톤.
- acceptance.md AC-1~AC-8 ↔ M1/M2/M3/M4/M5 검증.
- 선행: SPEC-CHIRPSTACK-001(completed), SPEC-CHIRPSTACK-002(completed).
- 관련: SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001.
