# SPEC-CHIRPSTACK-001 — 실행 계획 + 태스크 분해 (progress.md)

> Phase 1 (분석/계획) + Phase 1.5 (태스크 분해) 산출물. `/moai:2-run` 착수 전 근거 문서.
> 모든 재사용 주장은 `file:line` 인용으로 검증됨. Frozen 결정(REQ-FROZEN-01..04)은 사용자 승인 — 변경 금지.

## §E.1 Plan-phase Audit-Ready Signal

- plan_status: audit-ready
- dev-mode: Hybrid (신규 코드 TDD, 흡수 코드 DDD), coverage ≥85%
- tier: L (신규 패키지 + 노드 + wiring 3곳, >15 파일 영향)
- coverage_verified: true (모든 REQ-* → 최소 1개 태스크 매핑, 아래 §커버리지 매트릭스)

---

## 1. 기술 접근 (plan_summary)

ChirpStack LoRaWAN MQTT 수신 전용 에이전트. 2계층:
- **에이전트** `internal/agent/chirpstack/`: MQTT 트랜스포트(system/mqtt_agent.go 이식) + 업링크 디코드 + per-measurement fan-out + 디바이스 자동생성 + comm-state watchdog.
- **노드** `internal/node/chirpstack.go`: 수신 전용 SourceNode(receiveLoop), 다운스트림 store/influx 직결. 기존 Lua `script`+`split` 파이프라인 대체(REQ-FROZEN-01).

### 확정 재사용 지점 (file:line 근거)

| 재사용 대상 | 위치 | 사용 방식 |
|------|------|-----------|
| stopped 가드 (atomic.Bool) | `internal/agent/system/mqtt_agent.go:146` (선언), `:320-325` (subscribe 가드), `:498` (Stop set), `:213` (Init reset) | Stop 후 paho 재연결 부활 방지. 발행 경로 제외하고 이식 |
| ReceiveMessage / TransportConnected | `mqtt_agent.go:452`, `:561` | 수신 채널 소비 + 브로커 연결 위임(paho IsConnected) |
| parseMQTTConfig 관용구 | `mqtt_agent.go` MQTTConfig `:21-58` + parse `:68-126` | ChirpStackConfig 미러 + comm-state 3키 추가 |
| ResolveDeviceID | `internal/agent/device_id_repo.go:72` `ResolveDeviceID(ctx, agentName, unitID) string` | devEui→UUID; 내부 normalizeAgentRef 로 이름/ID 정규화 |
| SetDeviceInfo | `internal/agent/device_info_repo.go:33` `SetDeviceInfo(agentName, unitID string, DeviceInfo{DeviceType, Label})` (struct `:20-23`) | 런타임 device_type/label 등록 |
| registry SetMetadata | `internal/device/registry.go:187` `SetMetadata(id string, DeviceMetadata) error`; struct `internal/device/device.go:114-121` | UUID 키로 Name/Tags/Labels 지속화 |
| DeviceProvider 자동 wiring | `cmd/xflowd/main.go:213-221` (WithOnStart 인터페이스 어서션 `deviceProviderAgent`) | 별도 코드 불필요 — `DeviceProvider()` 구현만 하면 자동 등록. `:347-357` stop 해제 |
| 노드 device 그룹 승격 | `internal/node/dedup_helper.go:148` `promoteDevIDWithUUID` (unit_id→device_id via ResolveDeviceID + GetDeviceInfo→device.type/name) | 에이전트는 `unit_id`(=devEui)만 emit, device 그룹은 노드가 승격 (REQ-M3-04) |
| SourceNode + receiveLoop 패턴 | `internal/node/mqtt.go:182-197` (MQTTSubNode), `:272` receiveLoop, `:281` ReceiveMessage; passive 예시 `internal/node/century_hvacr01.go:411` (StatusNode SourceNode), `:461` receiveLoop | 수동 SourceNode 이식 |
| agent.Agent 13 메서드 | `internal/agent/agent.go:127-141` (Init/Start/Stop/Pause/Resume/Health/Process/Configure/ID/Name/Type/Info/Stats) | ChirpStackAgent 구현 대상 |
| main.go 등록 블록 | `cmd/xflowd/main.go:360-413` (`// 5.1. 에이전트 타입 등록`), 미러 대상 `century.RegisterHvacr01Types(agentMgr):390` | `chirpstack.RegisterChirpStackTypes(agentMgr)` 호출 + import 추가 (누락 시 인스턴스화 불가) |
| 메시지 빌더 | `pkg/message/message.go:122` New, `:102` WithTimestamp(time.Time), payload `payload.go:57` Set, metadata 단일값 `metadata.go:94` Set(k,v), 그룹 `metadata.go:118` SetGroup(k, map) | per-measurement/device_state 빌드. metadata 값은 string 또는 map[string]string 만 허용 |

### 해결된 flow-validation 경로 불일치 (필수 기록)

- **SPEC REQ-M3-06 은 `internal/flow/validate.go` 를 참조하나 그 파일은 존재하지 않는다** (`ls internal/flow/` → No such file). research.md/design.md 는 `pkg/flow/validate.go:140-150` 로 올바르게 정정되어 있음.
- **실제 메커니즘**: `pkg/flow/validate.go:139-186` 의 `agentRefRequiredTypes` 집합. `mqtt-subscriber` 가 `:148` 에 존재. 이는 "agent_ref 필수 노드" 집합이지 "알려진 노드 타입 화이트리스트"가 아니다.
- **중요**: `pkg/flow/validate.go` 에는 **알려지지 않은 노드 타입을 거부하는 존재성(existence) 화이트리스트가 없다** (grep `UNKNOWN_NODE`/`knownTypes`/`factories` → 0건). 노드 타입 등록/존재 판정은 `internal/node/registry.go` factories 맵이 담당하며, 미등록 타입은 build/instantiate 시점에 실패한다.
- **결론**: `chirpstack-in` 은 ChirpStack 에이전트에 바인딩(agent_ref 필요)하므로 `mqtt-subscriber` 처럼 `agentRefRequiredTypes` 에 추가해야 한다. 다만 이는 "미추가 시 플로우 거부"가 아니라 "agent_ref 미설정 검증 활성화" 목적이다.

---

## 2. 태스크 분해 (Phase 1.5) — 최대 10개

의존 순서: M1→M2→M3→M4; M5 는 M3/M4 이후; M6(테스트)은 전 구간 병행(TDD RED 우선)이나 최종 검증은 마지막.

| Task | Milestone | 설명 | REQ 매핑 | 의존 | 파일(create/modify) | AC |
|------|-----------|------|----------|------|---------------------|-----|
| TASK-001 | M1 | `ChirpStackAgent` 구조체 + agent.Agent 13 메서드 스켈레톤 + `RegisterChirpStackTypes(mgr)` + 이름 고유성 검증 | REQ-M1-01/03/04 | — | +agent.go, +registration.go | AC-7 |
| TASK-002 | M1 | main.go 5.1 블록에 `chirpstack.RegisterChirpStackTypes(agentMgr)` 호출 + import (load-bearing) | REQ-M1-02 | T001 | ~cmd/xflowd/main.go | AC-8 |
| TASK-003 | M2 | `ChirpStackConfig` (MQTTConfig 미러 + comm-state 3키) + parse (parseMQTTConfig 관용구) | REQ-M2(config), design §5 | T001 | +config.go | — |
| TASK-004 | M2 | MQTT 트랜스포트 이식: connect/subscribe(`application/#`)/recvCh/ReceiveMessage/TransportConnected/stopped 가드 (발행 경로 제외) | REQ-M2-01/02/03/04 | T003 | ~agent.go | AC-9 |
| TASK-005 | M3 | `decode.go`: 업링크 JSON(deviceInfo/object/rxInfo/time) 디코드 + object fan-out + 스칼라 판정 + 비스칼라 skip+경고 | REQ-M3-01/02/05 | T004 | +decode.go | AC-1c |
| TASK-006 | M3 | `message.go`: per-measurement event 빌더 — type=event, WithTimestamp(uplink time), payload.value, metadata.measurement(Set), tags verbatim(SetGroup), unit_id=devEui | REQ-M3-02/03/04, FROZEN-01/02/04 | T005 | +message.go | AC-1a/1b/2/4 |
| TASK-007 | M3 | `ChirpStackInNode` (SourceNode+receiveLoop) + `node/registry.go` 테이블 등록 + `pkg/flow/validate.go` `agentRefRequiredTypes` 추가 | REQ-M3-06, FROZEN-02 | T006 | +internal/node/chirpstack.go, ~internal/node/registry.go, ~pkg/flow/validate.go | AC-8 |
| TASK-008 | M4 | 디바이스 라이프사이클: ResolveDeviceID(UID) + SetDeviceInfo + registry SetMetadata(Name/tags→Labels) + `provider.go` DeviceProvider | REQ-M4-01/02/03/04/05 | T006 | +provider.go, ~agent.go | AC-3a/3b/3c |
| TASK-009 | M5 | comm-state(optional): `watchdog.go` last-seen map(mutex) + watchLoop + reportLoop + device_state.change/report + best-gateway(max rssi) + config gate + restart unknown 보류 | REQ-M5-01..06, FROZEN-03, REQ-M6-03(goroutine) | T005,T008 | +watchdog.go, ~message.go, ~agent.go | AC-5a..5d, AC-6 |
| TASK-010 | M6 | 테이블 주도 테스트(packet.json 픽스처): decode/fan-out/device-create/tags/comm-state + 다운스트림 golden 회귀 + goroutine 누수(context cancel) + coverage ≥85% + LSP zero | REQ-M6-01/02/03 | 전(T001..T009) | +*_test.go, +testdata/ | AC-1~9 검증 |

### 커버리지 매트릭스 (모든 REQ → 태스크)

- FROZEN-01→T006 · FROZEN-02→T006/T007/T010 · FROZEN-03→T009 · FROZEN-04→T006
- M1-01→T001 · M1-02→T002 · M1-03→T001 · M1-04→T001
- M2-01/02/03/04→T004 (config T003)
- M3-01→T005 · M3-02→T005/T006 · M3-03→T006 · M3-04→T006/T008 · M3-05→T005 · M3-06→T007
- M4-01/02/03/04/05→T008
- M5-01/02/03/04/05/06→T009
- M6-01→T010 · M6-02→T010 · M6-03→T009/T010
- **coverage_verified: true**

---

## 3. 파일 목록

### 신규 (create)
- `internal/agent/chirpstack/agent.go` — ChirpStackAgent + 라이프사이클 + MessageReceiver/TransportChecker/StatefulAgent
- `internal/agent/chirpstack/config.go` — ChirpStackConfig 파싱
- `internal/agent/chirpstack/decode.go` — 업링크 디코드 + object fan-out
- `internal/agent/chirpstack/message.go` — per-measurement + device_state 빌더
- `internal/agent/chirpstack/provider.go` — DeviceProvider 어댑터
- `internal/agent/chirpstack/watchdog.go` — last-seen/staleness/report (M5)
- `internal/agent/chirpstack/registration.go` — RegisterChirpStackTypes
- `internal/agent/chirpstack/*_test.go` + `testdata/packet.json` — 테스트
- `internal/node/chirpstack.go` — ChirpStackInNode (SourceNode)

### 수정 (modify) — wiring 3곳
1. `cmd/xflowd/main.go` — 5.1 블록(:360-413)에 RegisterChirpStackTypes 호출 + import (**load-bearing, 누락 시 인스턴스화 불가**)
2. `internal/node/registry.go` — registerBuiltins 테이블(:73-136)에 `{"chirpstack-in", NewChirpStackInNode, "io", "..."}` 추가
3. `pkg/flow/validate.go` — `agentRefRequiredTypes`(:139-186)에 `"chirpstack-in": {}` 추가 (**SPEC 의 internal/flow/validate.go 는 오류 — 정정된 실제 경로**)

> main.go:213-221 DeviceProvider 자동 wiring 은 인터페이스 어서션이므로 코드 추가 불필요(구현만).

---

## 4. 위험 / 불일치 (근거 인용)

1. **[불일치·해결] flow-validate 경로**: REQ-M3-06 `internal/flow/validate.go` 존재하지 않음 → 실제 `pkg/flow/validate.go:139-186 agentRefRequiredTypes`. 존재성 화이트리스트 아님(미추가해도 플로우 거부 안 됨; agent_ref 검증 목적).
2. **[데이터 모델 갭] tags 지속화 타입 불일치**: 메시지 `metadata.tags` 는 map(SetGroup, verbatim OK). 그러나 registry 지속화 `DeviceMetadata.Tags` 는 **`[]string`** (`device.go:116`), ChirpStack tags 는 key-value map{location,point,spot}. REQ-M4-04 지속화는 `DeviceMetadata.Labels map[string]string`(`device.go:119`)에 매핑해야 함(Tags []string 아님). run-phase 확정 필요.
3. **[패턴 선택 긴장] 노드 agent_ref 처리**: mqtt-subscriber 는 `AgentRef()` struct 메서드 → agentRefRequiredTypes 포함(`validate.go:148`). century 는 agent_ref 를 **config 필드**로 처리 → **일부러 집합에서 제외**(`validate.go:157-159` 주석). design 은 "century 패시브 패턴" 언급하나 research 는 "mqtt 처럼 validate.go 추가" 지시 → run-phase 에서 노드가 AgentRef struct 방식(권장, mqtt-subscriber 미러) 채택 시 agentRefRequiredTypes 추가가 정합.
4. **[비스칼라 object]**: 중첩 객체/배열 값은 1차 skip+경고(REQ-M3-05, AC-1c). packet.json 실측은 문자열 스칼라 `magnet_status:"close"`.
5. **[goroutine 누수]**: watchLoop/reportLoop 는 context 취소 종료 보장 필요(REQ-M6-03). 공유 last-seen map 은 mutex. **HVAC 락 함정 준수**: lock-holding 함수 내 `a.Name()` self-name 재조회 금지(재귀 RLock deadlock) — watchLoop/reportLoop 에서 agentName 은 사전 캡처값 사용.
6. **[timestamp 컨벤션]**: top-level timestamp = WithTimestamp(uplink `time` time.Time); payload 내 epoch(`last_seen_ms`) = int64 UnixMilli (프로젝트 메모리).
7. **[이름 고유성]**: device_id 키잉이 agentName→canonical ID 정규화(`agent_id_resolver.go`, `registry.go:24-27` 첫 등록 유지)에 의존 → 이름 중복 시 collision. REQ-M1-04 감지/거부.
8. **[best-gateway]**: rxInfo 다중 게이트웨이 중 최대 rssi 선택(REQ-M5-05, AC-5a rssi=-57/snr=13.5).

---

## 5. 마일스톤별 커밋 단위 제안 (Route: main-direct Hybrid Trunk)

- M1: `feat(chirpstack): 에이전트 스켈레톤 + 타입 등록 wiring (T001-T002)`
- M2: `feat(chirpstack): MQTT 연결/구독/수신 + stopped 가드 (T003-T004)`
- M3: `feat(chirpstack): 업링크 디코드 + per-measurement fan-out (T005-T006)`
- M3: `feat(chirpstack): chirpstack-in 노드 + registry/validate wiring (T007)`
- M4: `feat(chirpstack): 디바이스 자동생성 + provider + 메타데이터 지속화 (T008)`
- M5: `feat(chirpstack): comm-state device_state fold + staleness watchdog (T009)`
- M6: `test(chirpstack): 테이블 주도 테스트 + golden 회귀 + 누수 검증 (T010)`

각 커밋 SPEC-CHIRPSTACK-001 참조, @MX:WARN(watchLoop/reportLoop) 태그 부착.

---

## §E.2 Run-phase Evidence

- run_status: complete
- 구현 커밋 (M1~M6, main-direct Hybrid Trunk):

  | Milestone | 커밋 | 내용 |
  |-----------|------|------|
  | M1 | `eb0339f9` | 에이전트 스켈레톤 + 타입 등록 wiring (T001-T002) |
  | M2 | `6bb3a965` | MQTT 연결/구독/수신 + stopped 가드 (T003-T004) |
  | M3 | `ca024d1d` | 업링크 디코드 + per-measurement fan-out (T005-T006) |
  | M3 | `bc08f3ce` | chirpstack-in 노드 + registry/validate wiring (T007) |
  | M4 | `b26a8850` | 디바이스 자동생성 + provider + 메타데이터 지속화 (T008) |
  | M5/M6 | `88807142` | comm-state device_state fold + staleness watchdog + 테이블 주도 테스트 (T009-T010) |

- 신규 산출물: `internal/agent/chirpstack/{agent,config,decode,message,provider,watchdog,registration}.go` + 테스트 + `testdata/packet.json`; `internal/node/chirpstack.go`.
- Wiring 3곳: `cmd/xflowd/main.go` (RegisterChirpStackTypes + import), `internal/node/registry.go` (chirpstack-in), `pkg/flow/validate.go` (agentRefRequiredTypes).
- 신규 타입: 에이전트 `chirpstack`, 노드 `chirpstack-in`.

## §E.3 Run-phase Audit-Ready Signal

- run_status: audit-ready
- 신규 코드 커버리지: 89.4% (목표 85% 초과)
- TRUST 5: PASS (Critical 0)
- go build/vet/gofmt: 클린; 신규 코드 golangci-lint: 0 finding
- REQ-FROZEN-01~04 준수; AC-1~AC-9 전량 충족
- 무회귀 (무관한 pre-existing flaky E2E `internal/api/service` 는 본 SPEC 과 무관 — 미변경)
- 분기(as-implemented) IN-1~IN-4: spec.md § Implementation Notes 참조

## §E.4 Sync-phase Audit-Ready Signal

- sync_status: audit-ready
- SPEC 상태 전이: draft → completed (spec.md/plan.md/acceptance.md/design.md/research.md frontmatter), updated: 2026-08-12
- 동기화 문서: spec.md(§ Implementation Notes 추가), CHANGELOG.md, README.md(구현 현황), .moai/project/{structure,tech,product}.md, .moai/reports/sync-report-chirpstack-001.md
- sync_commit_sha: pending-backfill-chirpstack-001 (단일 sync 커밋; 로컬 전용 — push/PR 미수행, personal git 모드 auto_push=false/auto_pr=false)
- 3-phase close (plan→run→sync) 완료; Mx 는 sync 하위 단계(별도 커밋 없음)
