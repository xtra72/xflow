# SPEC-CENTURY-001: Century HVAC 프로토콜 패시브 에이전트 및 플로우 노드

## 메타데이터

| 항목 | 값 |
|------|-----|
| ID | SPEC-CENTURY-001 |
| 버전 | 0.3.8 |
| 상태 | Draft |
| 생성일 | 2026-05-18 |
| 수정일 | 2026-05-19 |
| 작성자 | xtra |
| 우선순위 | Medium |
| 관련 SPEC | SPEC-SERIAL-001, SPEC-LGCNP-001, SPEC-NASA-001, SPEC-NODE-002, SPEC-AGENT-001, SPEC-AGENT-005, SPEC-ENGINE-001 |
| 프로토콜 문서 | `references/protocols/century_hvac_protocol_spec.md` (v0.3, 캡처 4건 검증) |

---

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 | 작성자 | 상태 |
|------|------|----------|--------|------|
| 2026-05-19 | 0.4.2 | **device_state emit 을 Reg02 AND Reg04 모두 수신 후로 gate (v0.4.1 후속 hotfix)**. 사용자 보고: v0.4.1 적용 후에도 첫 emit 이 `current_temp:0` (Reg04 미수신 fallback) 으로 노출 후 직후 정정되는 결함이 남아 있음 — "초기값이 없으며, 값이 설정되지 않으면 반환하지 않음" 요구에 맞게 strict gate. **수정**: `maybeEmitDeviceState` 가드를 Reg02 단독에서 `Reg02 != nil && Reg04Read != nil` 확장. 5 핵심 필드의 모든 원천 register 가 적어도 한 번 관측되어야 emit. 정상 시나리오에서는 master cycle (~512ms) 안에 Reg02/03/04 모두 polling 되므로 첫 emit 까지 최대 ~512ms 대기. master 가 Reg02 만 polling 하는 매우 드문 케이스에서는 emit 영구 보류 (5 핵심 중 current_temp 가 미정의이므로 의미 보존 우선). **테스트 마이그레이션**: AC-H3 (이전 "Reg02 emit + Reg04 emit 2회") → "Reg02+Reg04 통합 1회 emit" 으로 의미 변경. AC-H2/H4/H5/H6/H7/H10 + Coverage/Drain/Keepalive 테스트 모두 Reg04 frame 추가 주입. v0.4.1 의 `TestAgent_DeviceStateGatedByReg02` 와 대칭으로 `TestAgent_DeviceStateGatedByReg04` 추가 (Reg02 만 먼저 → 0 emit → Reg04 후 첫 emit 정상값 검증). | xtra | Draft |
| 2026-05-19 | 0.4.1 | **device_state emit 을 Reg02 수신 후로 gate (초기값 결함 hotfix)**. 사용자 보고: AC 가 켜진 상태인데 첫 device_state event 가 `mode:"off", power:false, fan_speed:0, target_temp:0` 으로 표시됨. 원인: Reg04 frame 이 먼저 도착하면 Reg02 미수신 상태에서 fallback 값 (0/off) 으로 emit 되어 "꺼진 것처럼" 보임. v0.3.0 의 A14 "단일 register 만 수신해도 emit + 미수신 필드는 0.0" 정책이 운영 환경에서 오해를 유발. **수정**: `maybeEmitDeviceState` 에 가드 추가 — `devSnap.State == nil || devSnap.State.Reg02 == nil` 이면 emit skip. Reg02 (power/mode/fan/setpoint 의 원천 register) 도착 후 첫 emit 부터 정상 power/mode 노출. online 전이 emit 도 함께 차단 (Reg04 만 본 디바이스가 offline 으로 갈 때 무의미한 emit 방지). 회귀 테스트 `TestAgent_DeviceStateGatedByReg02`: Reg04 frame 만 주입 → device_state 0개 → Reg02 frame 주입 → 첫 device_state 가 mode="cool"/power=true/fan_speed=17/target_temp=25/current_temp=25.2 의 정상 통합 상태. A14 갱신: "Reg02 수신 후부터 device_state emit, 미수신 시점에는 emit 보류". | xtra | Draft |
| 2026-05-19 | 0.4.0 | **Breaking — JSON schema 일관성**. 사용자 요구: device_state 의 상태 필드를 `state` 그룹으로 묶고, register-decoded 메시지에 `type` 필드를 추가하여 downstream 분기/필터를 가능하게 한다. **변경**: (1) `CenturyDeviceStateEvent` — `online`/`power`/`mode`/`fan_speed`/`target_temp`/`current_temp` 를 top-level 에서 nested `state` 그룹으로 이동. 신규 `CenturyDeviceStateInner` 구조체. top-level 에는 `type`/`dev_id`/`label`/`timestamp_ms`/`last_seen_ms`/`state`/`trigger` 만 남음. 출력 형태: `{"type":"device_state","dev_id":"0x3B",...,"state":{"online":true,"power":true,"mode":"cool","fan_speed":17,"target_temp":25,"current_temp":25.2},"trigger":"change"}`. (2) `Reg02Decoded`/`Reg03Decoded`/`Reg04ReadDecoded`/`Reg04WriteDecoded`/`ACKDecoded` 모두 `Type string` 필드 추가 (각각 `century_reg02_response`, `century_reg03_response`, `century_reg04_response`, `century_reg04_write_request`, `century_ack`). decoder 생성자 5곳에서 type 값 세팅. `transformDecodedPayload` 는 string scalar 필드는 그대로 top-level 에 유지하므로 별도 처리 불필요. (3) 신규 상수 `EventTypeReg02Response` 등 message.go 추가. (4) 회귀 테스트 — `deviceStateGroup` helper 추가 + AC-H2/H3/H5/H6/H7 의 모든 5 핵심 필드 access 를 nested 형태로 마이그레이션. 전체 century + node 패키지 테스트 통과. **Migration (v0.3.x → v0.4.0)**: device_state 컨슈머는 `m.mode` 대신 `m.state.mode` 로 접근. register-decoded 컨슈머는 `type` 필드로 분기 가능 (기존 `dev_id`/`register`/`state` 는 동일 위치 유지). | xtra | Draft |
| 2026-05-19 | 0.3.11 | **device_state 가 polling 노드에서 보이도록 fix (true root cause)**. v0.3.10 까지의 keepalive fix 는 `lastKeepaliveTime` 분리만 했을 뿐, **실제 사용자의 polling 노드 (century-status / century combined) 는 ringBuffer 만 polling 하고 `msgCh` (device_state event 의 행선지) 를 보지 않는다**. 따라서 keepalive emit 이 `msgCh` 로 정상 발생해도 polling 노드의 출력에는 절대 나타나지 않았다. 사용자 보고 "keepalive 전송 안됨" 의 진짜 root cause. **수정**: NASA 의 `recentSnapshots` 패턴을 참고하여 (1) agent 에 별도 `deviceStateBuf []json.RawMessage` (drop-oldest, capacity = `RingBufferSize/2`) 추가 — `msgCh` 와 독립적으로 운영되어 Bridge 컨슈머와 polling 노드가 경쟁하지 않음. (2) `maybeEmitDeviceState` 가 `emitToMsgCh` 직후 `pushDeviceStateBuf` 호출하여 두 경로에 모두 publish. (3) 신규 Process command `"drain_device_state"` 추가 — 버퍼를 비파괴로 drain 하여 `{"count":N,"events":[...]}` 반환. (4) `centuryNodeBase.drainDeviceStateEvents` helper 추가 — `CenturyStatusNode.pollLoop` / `CenturyNode.pollLoop` 가 매 ticker 마다 호출하여 device_state 이벤트를 `sourceCh` 로 forward (metadata `century_source="device_state"`). **회귀 테스트**: `TestProcessDrainDeviceState_BasicFlow` (drain 응답 형식 + change/keepalive 모두 포함 + 두 번째 drain 시 빈 응답) + `TestPushDeviceStateBuf_DropOldest` (drop-oldest semantics). Non-breaking — `msgCh` Bridge 경로는 그대로 유지. | xtra | Draft |
| 2026-05-19 | 0.3.10 | **keepalive 가 change 빈도와 무관하게 fire 하도록 fix (root cause)**. 사용자 보고: "change 메시지는 도착하고 keepalive 만 안옴". 원인: v0.3.0~v0.3.9 의 `checkKeepaliveEmits` 가 `lastEmitTime` 을 기준으로 했는데, 모든 change emit 마다 `lastEmitTime` 이 갱신되어 change 가 자주 일어나면 (예: 현장에서 `current_temp` 가 매 cycle 0.1°C 진동) keepalive 임계값 (60s) 에 절대 도달하지 못했다. **수정**: 신규 필드 `lastKeepaliveTime map[byte]time.Time` 을 추가하여 keepalive 타이머를 `lastEmitTime` (change/keepalive 공용) 과 완전히 분리. (1) 첫 emit 시 (change 또는 keepalive 무관) `lastKeepaliveTime` 을 anchor 로 초기화. (2) 이후 change emit 은 `lastKeepaliveTime` 을 갱신하지 않음. (3) keepalive emit 시에만 `lastKeepaliveTime = now` 로 갱신. (4) `checkKeepaliveEmits` 가 `lastKeepaliveTime` 을 기준으로 `shouldKeepaliveFire` 호출. 결과: change 가 매 cycle 발생하더라도 keepalive 가 interval 마다 독립적으로 fire. 신규 회귀 테스트 `TestAgent_KeepaliveFiresDespiteFrequentChanges`: 매 80ms 마다 mode alternating frame 주입 (지속적 change) + `keepalive_interval=300ms` 환경에서 1500ms 동안 keepalive 가 최소 1회 fire 함을 검증. AC-H6 기존 테스트도 통과 (no-change 상황에서도 정상 동작). Non-breaking — 정상 시나리오 (change 빈도 < interval) 에서는 v0.3.9 와 동일 동작. | xtra | Draft |
| 2026-05-19 | 0.3.9 | **keepalive emit 시점 정책 옵션 추가 (`keepalive_mode`)**. 사용자 요구: "일정 시간 간격으로 상태 전송 옵션 처리 — 시간 간격을 시작 시간부터 또는 특정 시간 간격마다(linux crontab 과 같이)". 신규 옵션 `keepalive_mode` (default `"relative"`) — (1) **`relative`** (기존 동작): 마지막 emit 시점으로부터 `keepalive_interval` 경과 시 emit. agent 시작 시점에 따라 emit 시각이 달라짐. (2) **`absolute`** (신규): wall-clock 정렬 — `now.Truncate(keepalive_interval)` 가 마지막 emit 시점 이후이면 emit. 매 interval 의 정수 배수 시각에 emit (예: 60s 면 매 분 0초, 5m 면 0/5/10/15... 분, 1h 면 매 시 정각). linux crontab 패턴. **신규 helper `shouldKeepaliveFire(now, last, interval, mode)`**: interval≤0 가드 + mode 별 분기 + unknown mode 는 relative fallback. **agent.go `checkKeepaliveEmits`** 가 직접 `now.Sub(lastTime) >= interval` 비교에서 helper 호출로 교체. **config.go `parseCenturyConfig`**: `keepalive_mode` 옵션 파싱, allowed values=`{"relative","absolute"}`, 빈 string → default, 기타 값 → `fmt.Errorf("invalid keepalive_mode")`. 신규 테스트 `keepalive_mode_test.go` — `TestShouldKeepaliveFire` (12 cases: invalid interval, relative 경계, default/unknown mode, absolute 1m/5m/1h 정렬) + `TestParseCenturyConfig_KeepaliveMode` (5 cases: default/explicit/empty/invalid). 예제 YAML 3종 + `web/agentSchemas.ts` + `agentTypeMeta.ts` 업데이트. Non-breaking — default `"relative"` 는 v0.3.0 동작 유지. **`absolute` 부작용**: agent 시작 시점에 따라 첫 keepalive emit 까지 최대 `keepalive_interval` 만큼 대기 (다음 정렬 시점까지). | xtra | Draft |
| 2026-05-19 | 0.3.8 | **노드 polling path 에 change detection 적용 (root cause hotfix)**. v0.3.6/v0.3.7 의 change detection 이 captureLoop msgCh emit 에만 적용되어, **century-status 노드의 processGetRecent/processDrain 경로 (ringBuffer 직접 polling) 에서는 매번 emit 되던 문제**. 사용자가 보고 있던 출력의 진짜 root cause. **frameToEventIfChanged 신규** — frameToEvent + ACK skip + shouldEmitRegisterChange 를 한 번에 처리. processGetRecent / processDrain 모두 적용. captureLoop msgCh emit 과 같은 `lastRegisterEmit` 캐시를 공유 (emitMu 로 동시 접근 보호). | xtra | Draft |
| 2026-05-19 | 0.3.7 | **change detection hotfix — 변동 메타 제외 비교 + 빈 의미 메시지 차단**. v0.3.6 의 change detection 이 `extractStateGroup` 만 사용해 state 그룹 없는 메시지 (Reg04Read 처럼 모든 필드가 inferred 인 경우) 는 보수적으로 매번 emit 되던 버그 수정. **`extractComparablePayload`** 신규 — `nonComparableEmitKeys` (timestamp_ms / seq / dev_id) 를 제거한 후 남은 의미 페이로드를 비교. 빈 페이로드 (비교 의미 데이터 없음) 시 emit 안 함. 사용자 보고의 반복 출력 (mode_cmd:off 반복, 빈 메시지 반복) 모두 차단. Reg02/03/04 + Reg04Write 의 state 변경 시에만 emit. | xtra | Draft |
| 2026-05-19 | 0.3.6 | **register-decoded 노이즈 제거**. (1) **빈 raw_hex 제거**: 노드 `buildCenturyMessage` 가 `fr.RawHex` 가 빈 string 인 경우 페이로드에 set 하지 않음 — include_raw_hex=false 시 capturedFrameEvent 가 빈 string 으로 emit 하는 문제 해결. (2) **ACK frame skip**: ACK (의미 없는 응답 ACK) 는 register-decoded 출력에서 제외. 사용자 trace 노이즈 제거. (3) **register-decoded change detection**: 신규 `lastRegisterEmit map[registerEmitKey][]byte` 캐시 + `shouldEmitRegisterChange()` helper — (dev_id, register) 별 state 그룹 비교, 동일 state 반복은 emit skip. `extractStateGroup()` 가 timestamp_ms / seq / direction 같은 메타를 무시하고 device-level state 만 비교. Reg04WriteDecoded 는 register=0x84 (0x04 | 0x80) 로 read 와 별도 캐시 키. bridgeActive 와 무관하게 캐시는 항상 갱신 — bridge 활성 직후 곧바로 dedup. Non-breaking, 운영 trace 가독성 향상. | xtra | Draft |
| 2026-05-19 | 0.3.5 | **register/raw_hex 옵션화 + sub_dev_id→dev_id rename + dev_id/timestamp_ms/state 기본 출력 명확화**. (1) 신규 옵션 `include_register_info` (default false): register 번호 + direction 등 register-level 메타데이터를 옵션 활성 시에만 출력. (2) 신규 옵션 `include_raw_hex` (default false): 원시 바이트 hex 표현을 옵션 활성 시에만 출력. century-raw-frame 노드는 자체 목적이므로 옵션 무관 항상 emit. (3) **필드 이름 단순화**: 모든 Reg*Decoded + CenturyDeviceStateEvent 의 `sub_dev_id` JSON tag → `dev_id`. (4) **기본 출력 보장**: dev_id / timestamp_ms / state 그룹 / direction 외 메타는 옵션과 무관하게 항상 출력. capturedFrameEvent 의 raw_hex/function_code/register 에 omitempty 추가하여 옵션 비활성 시 빈 값으로 처리. Non-breaking — 옵션 활성 시 v0.3.4 동작과 동일. | xtra | Draft |
| 2026-05-19 | 0.3.3 | **register-decoded 출력 status-grouped 재구성 + inferred 옵션화**. v0.3.2 의 단순 prune 을 더 일관된 transform 으로 교체. (1) 신규 옵션 `include_inferred_fields` (default false): inferred (추정 의미) 필드들(op_val_*, status_bits, temp_A_c, reg04_const_*, reg02_live_*, reg02_word_*) 도 옵션 활성 시에만 노출 — 사용자 보고: "옵션으로 체크되지 않으면 출력하지 않음". (2) **출력 구조 재설계**: `transformDecodedPayload` 가 confirmed 필드의 value 만 평탄화하여 `status: {...}` 그룹으로 묶고 (NASA/LGCNP 와 유사한 평탄화 + LGCNP `parsed` 와 유사한 그룹화 패턴 — 사용자 보고: "mode, setpoint_c 등도 status:{mode:off, set_temp:27}처럼 상태로 묶어서 출력"), inferred 는 `inferred: {...}` 그룹, unknown 은 `unknown: {...}` 그룹으로 옵션 활성 시에만 추가. 비-nested 필드 (register, sub_dev_id, raw_hex, seq, timestamp_ms, direction) 는 top-level 유지. (3) 적용 위치: captureLoop 의 msgCh emit + processGetRecent / processDrain (century-status 노드 source). Non-breaking 이며 register-decoded 출력의 가독성을 개선한다. | xtra | Draft |
| 2026-05-19 | 0.3.2 | **운영 가독성 + 노드 출력 일관성 + Web UI 2열 구성**. (1) Web UI: `AgentDetailPanel.tsx` 의 hardcoded agent layout map 에 `century-hvac` 추가 — LGCNP/NASA 와 동일하게 좌측='연결' (transport / serial / TCP / 주소), 우측='운영' (auto_discovery / dedupe / emit / keepalive / 로그) 2열 구성. (2) 신규 옵션 `include_unknown_fields` (default **false**): register-decoded 메시지 페이로드에서 `confirmation_status="unknown"` padding/reserved 바이트 (reg02_byte_*, reg03_pad_*, reg04_byte_3..6, write_byte_* 등) 를 emit 시점에 자동 제거. 운영 환경의 trace 가독성 우선, 프로토콜 RE/디버깅 시에만 true. (3) **노드 출력에도 prune 적용** — `processGetRecent` / `processDrain` (century-status 노드의 source) 의 frameToEvent 가 cfg.IncludeUnknownFields 를 반영. captureLoop 의 msgCh 와 동일한 정책. Non-breaking, additive. | xtra | Draft |
| 2026-05-19 | 0.3.1 | **3종 hotfix + schema 통일**. (1) `CenturyAgent.DeviceProvider()` 메서드 누락 fix — main.go:204 의 type assertion 이 실패하여 deviceRegistry 에 century 디바이스가 등록되지 않던 root cause. NASA/LGCNP 와 동일 패턴 적용. (2) `DeviceStateEvent` schema 를 Samsung NASA / LGCNP 와 통일: `set_temp_c` → `target_temp`, `current_temp_c` → `current_temp`, `fan` → `fan_speed`, `mode` 값 `"cooling"` → `"cool"`. `evap_temp_a_c` / `evap_temp_b_c` 는 device-level state 가 아닌 register-level 정보이므로 schema 에서 제거하고 emit_register_decoded 옵션의 Reg03Decoded 메시지로만 노출. (3) `emit_register_decoded=false` (default) 시 register-decoded 메시지가 절대 emit 되지 않도록 captureLoop 분기 검증 완료 (AC-H1 회귀 통과). Non-breaking 이며 v0.3.0 schema 의 외부 노출 직후 정정. | xtra | Draft |
| 2026-05-19 | 0.3.0 | **Breaking** — agent `msgCh` emit 의 default 가 register-decoded 메시지 (Reg02Decoded / Reg03Decoded / Reg04ReadDecoded / Reg04WriteDecoded / ACKDecoded) 에서 device-centric `DeviceStateEvent` 로 변경됨. 변경 감지 (5개 핵심 필드: power/mode/fan/set_temp_c/current_temp_c + online 전이) 시 즉시 `trigger="change"` emit, `keepalive_interval` (기본 60s) 동안 변경 없으면 `trigger="keepalive"` emit. 신규 REQ-CENTURY-033 (DeviceStateEvent schema, snake_case + epoch ms + 증발기 온도 포함), REQ-CENTURY-034 (`emit_device_state` 기본 true / `emit_register_decoded` 기본 **false** / `keepalive_interval` 기본 60s + `ErrCenturyNoOutputEnabled` 검증), REQ-CENTURY-035 (change detection + keepalive fallback + online 전이 즉시 emit). 신규 가정 A14~A16, 신규 리스크 R13~R15. 새 Group H acceptance (H1~H10) 신설. M7 마일스톤 신설 (v0.3.0 implementation). **Migration**: v0.2.x 의 register-decoded 메시지를 소비하던 downstream 은 (a) `emit_register_decoded: true` 로 명시 활성화하거나, (b) `type=="device_state"` 메시지로 마이그레이션해야 한다. | xtra | Draft |
| 2026-05-18 | 0.2.0 | TCP transport 지원 추가 (tcp-client + tcp-server, plain TCP only). `transport_type` 확장 (serial → serial/tcp-client/tcp-server). 신규 REQ-CENTURY-028~032 (transport 확장, TCP-client dial, TCP-server listen, exponential backoff 재연결, transport-aware `cycle_idle_timeout` 기본값). 신규 Group G acceptance (G1~G8) — TCP 동작 및 회귀. M6 마일스톤 추가 (TCP transport 구현). 모든 v0.1.2 기능과 AC-B9 transport.Write 0회 불변식 유지 (non-breaking, additive). | xtra | Draft |
| 2026-05-18 | 0.1.0 | 초안 작성. Century HVAC 마스터-슬레이브 바이너리 프로토콜의 RS-485 회선 **패시브 스니프(passive capture) 전용** 에이전트와 4종 플로우 노드(status/control/combined/raw-frame) 도입. NASA 에이전트 스켈레톤 + LGCNP 패시브 캡처 패턴(다층 검증, ring buffer, DeviceProvider)을 결합한 구조. 미확정 필드는 `confirmation_status` 마커(`confirmed`/`inferred`/`unknown`)로 타입드 노출하여 향후 캡처를 통한 확정 진화를 허용. | xtra | Draft |
| 2026-05-18 | 0.1.1 | 다중 IDU 자동 발견 v0.1.0 범위 포함 (A7/REQ-CENTURY-013 갱신, 리스크 R3 삭제). WRITE 중복 제거 옵션 추가 (REQ-CENTURY-027, dedupe_writes 설정 필드, 시나리오 F1~F4 신설). | xtra | Draft |
| 2026-05-18 | 0.1.2 | M1-M5 구현 완료. 6 commits 누적 (14ee853 spec → bfdfaf0 M1 → d33da37 M2 → bad2e06 M3 → 3f1b970 M4 → [M5]). 커버리지: `internal/agent/century` 88.9%, `internal/node/century.go` 평균 87.4% (44 함수, 85% 게이트 통과). 27 REQ-CENTURY-XXX 모두 구현 완료, 그룹 A~F 의 모든 AC 시나리오 자동 테스트로 커버됨. examples/agents/century-hvac.yaml + examples/flows/century-status-flow.yaml 추가. §5.9 M5 closure notes 신설로 M1-M4 미해결 사항 disposition 명시. | xtra | Implemented |

---

## 1. 환경 (Environment)

### 1.1 시스템 컨텍스트

xflow 는 IoT/HVAC 데이터 스트림 처리를 위한 FBP 게이트웨이다. 현재 다음 HVAC 프로토콜이 통합되어 있다:

- **Samsung NASA** (`internal/agent/samsung/`): 능동 폴링 + 디코딩 (master/slave 모두 수행)
- **LG LGCP / LGAP** (`internal/agent/lg/lgcp_*.go`, `lgap_*.go`): 능동·패시브 혼합
- **LG LGCNP-01** (`internal/agent/lg/lgcnp_*.go`): RS-485 회선 **패시브 캡처 전용**, 다층 검증 + ring buffer + DeviceProvider 패턴 확립

본 SPEC 은 **Century 시스템 에어컨**용 신규 에이전트를 추가한다. Century 프로토콜은 마스터-슬레이브 바이너리 프로토콜로, xflow 는 기존 마스터(상위 컨트롤러) ↔ 슬레이브(에어컨 본체) RS-485 회선에 **passive tap** 하여 양방향 프레임을 모두 디코딩한다. 송신은 일절 수행하지 않는다.

- **Transport**: serial(RS-485 직결) + TCP(시리얼-Ethernet 컨버터 또는 외부 push) 두 차원 지원. v0.2.0 시점, 동작 모드(passive)와 transport(serial/tcp-client/tcp-server)는 직교 차원이며 모든 조합에서 AC-B9 (transport.Write 0회) 불변식이 유지된다.
- **Agent output policy v0.3.0** (Breaking): 메시지 stream 의 1차 산출물은 통합 device state (`DeviceStateEvent`, snake_case + epoch ms) 이며, register-decoded / raw frame 은 **옵션 활성화 시에만** emit 된다. v0.2.x 의 register-decoded default emit 은 더 이상 자동 적용되지 않는다 (REQ-CENTURY-033/034 참조).

### 1.2 기술 스택

- **언어**: Go 1.25.6
- **시리얼 통신**: SPEC-SERIAL-001 의 `internal/agent/serial` 패키지 또는 동등한 raw 바이트 스트림 트랜스포트 사용 (frame scanner 는 본 에이전트 내부에 둠)
- **라이프사이클**: `pkg/lifecycle.BaseLifecycle`
- **메시지 시스템**: `pkg/message.Message`, payload timestamp 는 epoch milliseconds (`int64`, `UnixMilli()`) — 프로젝트 컨벤션
- **에이전트 프레임워크**: `internal/agent.Agent` 인터페이스 + 선택 인터페이스(`MessageReceiver`, `StatefulAgent`, `TransportChecker`, `BufferInfoProvider`)
- **디바이스 관리**: `internal/device.DeviceProvider` 인터페이스
- **노드 프레임워크**: `internal/node` (`Node`, `SourceNode`, `MultiSourceNode`)
- **Web UI**: React + TypeScript (`web/src/config/agentSchemas.ts`, `web/src/config/nodeSchemas.ts`)
- **참조 구현 템플릿**:
  - 파일 레이아웃·등록 패턴·인터페이스 합성: Samsung NASA (`internal/agent/samsung/agent.go`, `crc.go`, `frame_scanner.go`, `protocol.go`, `register.go`, `provider.go`)
  - 패시브 캡처 + 다층 검증 + ring buffer + 노드 구성: LG LGCNP-01 (`internal/agent/lg/lgcnp_*.go`, `internal/node/lgcnp.go`)

### 1.3 LGCNP 와의 핵심 차이점

| 항목 | LGCNP-01 | Century |
|------|----------|---------|
| 물리 계층 | RS-485, 1200 bps, 8N1 | RS-485, 보레이트는 캡처 환경 의존 (사용자 설정) |
| 프레임 구분 | STX 패턴(0x58 / 0x81–0x85) + 고정 길이 | 8B 헤더(LE) + 가변 payload + 2B CRC. `payload_length` 로 경계 결정 |
| 무결성 검증 | 6계층(이중 기록 + 체크섬 + 물리 범위 + 변화율 + SEQ 순서) | 4계층(길이 → CRC → 헤더 → 페이로드 prefix → 레지스터별 길이) |
| CRC | 없음(SEQ 01/04/05 만 XOR/SUM 보조 체크섬) | **CRC-16/ARC** (poly 0x8005, init **0x0000**, RefIn/RefOut, LE 저장) — Modbus RTU(init 0xFFFF) 와 구분 |
| 주소 | 가변(IDU 0x81–0x85 / ODU 무주소) | Master=0x0030, Slave=0x0001 (LE u16) |
| 폴링 주기 | 약 4–6초(20B+40B×5 IDU 한 사이클) | 약 511.9 ms (σ=0.3 ms) per cycle |
| Function code | 없음(STX 가 유형 분류) | `0x06`=ACK, `0x0B`=Read, `0x0C`=Write |
| 레지스터 | 패킷 유형별 고정 오프셋 | 페이로드 prefix(`sub_dev_id`, reserved, register) + 레지스터별 data |
| 운전 방향 | 패시브 read-only | **패시브 read-only**(WRITE 프레임도 회선상에서 캡처·디코딩하나 본 에이전트가 송신하지는 않음) |

### 1.4 제약 (Constraints)

- **패시브 전용**: 본 에이전트는 RS-485 회선에 **수신 전용(RX-only)** 으로 부착된다. 어떤 경우에도 회선에 바이트를 송신하지 않는다. 시리얼 트랜스포트의 Write 경로를 호출해서는 안 된다.
- **반이중 단일 tap**: RS-485 반이중 회선에 직접 청취만 하므로 회선 충돌을 야기하지 않는다. 외부 컨트롤러의 폴링 사이클은 본 에이전트의 활성·비활성과 무관하게 계속된다.
- **재해석 가능성**: 미확정 필드(특히 reg 0x04 의 `status_bits`, `op_val_1/2`, WRITE 의 `write_live_0/1/14/15`)는 추가 캡처로 의미가 확정되면 SPEC 후속 버전에서 필드명을 갱신할 수 있다. payload 의 `confirmation_status` 마커가 이러한 진화를 가시화한다.

---

## 2. 가정 (Assumptions)

- **A1**: Century HVAC 실외기/실내기와 기존 마스터 컨트롤러는 xflow 가 비파괴적으로 tap 할 수 있는 전용 RS-485 버스를 사용한다.
- **A2**: 마스터의 폴링 주기는 약 511.9 ms (σ=0.3 ms) 이며, 한 사이클은 `READ reg 0x02 → RESP → READ reg 0x03 → RESP → READ reg 0x04 → RESP → WRITE reg 0x04 → ACK → WRITE reg 0x04 → ACK` 의 9 프레임으로 구성된다. WRITE 가 두 번 동일 전송되는 것은 신뢰성 목적의 redundancy 로 추정된다.
- **A3**: CRC-16/ARC 알고리즘은 프로토콜 문서 §3 및 §9 의 Python 참조 구현과 비트 단위 동일한 결과를 산출한다. 모든 캡처(CAP-1 ~ CAP-4)에서 CRC 일치율 99.7% 이상이 검증되었다.
- **A4**: 마스터 주소는 `0x0030`, 슬레이브 주소는 `0x0001`, `sub_dev_id` 는 `0x3B` 가 관측된 단일 indoor unit 환경이다. 다른 환경에서는 이 값들이 다를 수 있으므로 설정으로 노출한다. 다중 indoor unit (`sub_dev_id` 가 여러 개) 환경은 본 SPEC 의 v0.1.0 범위에 **포함** 되며(아래 A7 참조), 데이터 모델은 sub_dev_id 를 키로 한 자동 발견을 지원한다.
- **A5**: 모드 코드 중 `0x00`(꺼짐/대기)과 `0x01`(냉방)은 ground truth 로 확정되었으며, `0x02` 이상(난방/제습/송풍 등)은 미관측이다. 후속 캡처로 추가될 때 SPEC 의 enum 을 **additive** 하게 확장한다 — 기존 코드 호환을 깨지 않는다.
- **A6**: 미확정 바이트의 의미는 후속 SPEC 개정에서 변경될 수 있다. 이 SPEC 은 `confirmation_status` 라는 payload 마커(`confirmed` / `inferred` / `unknown`)로 각 필드의 신뢰도를 명시하여, downstream 컨슈머가 안정성을 알고 사용하도록 한다.
- **A7**: 다중 indoor unit 자동 발견을 v0.1.0에 포함한다. 각 `sub_dev_id`별로 독립된 `CenturyDevice` 인스턴스를 관리하며, 캡처된 모든 `sub_dev_id`에 대해 device tracking을 수행한다. 현재 검증된 캡처(CAP-1~CAP-4)는 단일 유닛(`sub_dev_id=0x3B`)뿐이므로 다중 유닛 코드 경로는 단위 테스트 및 추론 기반 시나리오로 검증한다. 본 에이전트는 패시브 캡처 전용이므로 "에어컨 제어"(reg 0x04 WRITE 프레임 전송)는 일절 수행하지 않는다 — 회선상에서 관측되는 WRITE 프레임은 외부 마스터의 명령이며 본 에이전트는 이를 디코딩만 한다.
- **A8**: 시리얼 회선 노이즈, 케이블 분리, USB 어댑터 hot-unplug 등 물리 계층 이상은 SPEC-SERIAL-001 의 트랜스포트 계층이 이미 처리한다. 본 에이전트는 그 위에서 프레임 경계 탐지·CRC 검증만 수행한다.
- **A9**: payload 의 모든 timestamp 필드는 프로젝트 컨벤션(`epoch_milliseconds`, `int64`, `time.Time.UnixMilli()`)을 따른다. `time.Time`/RFC3339 문자열을 payload 에 직접 노출하지 않는다.
- **A10** (v0.2.0): TCP 모드는 **plaintext 전용**이며 TLS 는 v0.3.0 deferral 이다. LAN 내 시리얼-Ethernet 컨버터(Moxa NPort, USR-N520 등) 사용을 전제로 한다. 인증·암호화가 필요한 환경에서는 별도 VPN/IPsec 또는 후속 SPEC 의 TLS 모드를 사용해야 한다.
- **A11** (v0.2.0): TCP-server 모드는 **단일 활성 연결**만 처리한다. 두 번째 연결은 즉시 거부(close) 한다. 다중 동시 연결(여러 컨버터가 동일 xflow 인스턴스로 push) 지원은 v0.3.0 deferral. 단일 활성 연결 정책은 frame 디코더의 상태 일관성(cycle tracker, write deduplicator) 을 단순화한다.
- **A12** (v0.2.0): TCP 모드에서 Nagle 알고리즘/패킷화/네트워크 jitter 로 인한 inter-frame timing 왜곡이 100ms idle fallback(REQ-CENTURY-027) 의 false-positive 를 유발할 수 있으므로 `cycle_idle_timeout` 의 기본값은 **transport-aware** (serial=100ms, tcp-client/tcp-server=200ms) 로 한다(REQ-CENTURY-032). 사용자가 명시적으로 설정하면 transport 와 무관하게 그 값을 사용한다.
- **A13** (v0.2.0): transport 차원과 동작 모드(passive)는 **직교**한다. tcp-client / tcp-server 모드에서도 회선 송신(transport.Write) 은 절대 수행하지 않는다 — TCP-server 의 accept 된 연결도 RX-only 로 사용한다. AC-B9 불변식은 모든 transport 조합에서 유지된다.
- **A14** (v0.3.0): 디바이스 상태는 register 0x02 (mode / fan / setpoint) + register 0x03 (증발기 온도 a/b) + register 0x04 read response (현재 온도 `temp_A_c`) 의 종합으로 정의된다. 단일 register 만 수신한 시점에서도 DeviceStateEvent 는 emit 되며, 미수신 register 의 필드는 0.0 (또는 spec 4.4 에 명시된 sentinel) 로 노출된다.
- **A15** (v0.3.0): 변경 감지는 5개 핵심 필드 (`power` / `mode` / `fan` / `set_temp_c` / `current_temp_c`) + `online` 상태 전이 기반이다. 증발기 온도 (`evap_temp_a_c` / `evap_temp_b_c`) 의 자잘한 변동은 emit 트리거가 아니며, 증발기 값은 가장 최근에 관측된 값이 그대로 payload 에 동봉된다. 증발기 변동을 트리거로 삼는 별도 옵션은 v0.3.0 범위 밖이며 v0.4.0 deferral 이다.
- **A16** (v0.3.0): `keepalive_interval` 의 기본값은 60s 이다. 너무 짧으면 (예: 1s) Century cycle 주기 (~512ms) 와의 상호작용으로 trigger="keepalive" 가 사실상 매 cycle 마다 emit 되어 downstream 에 register-decoded 와 유사한 noise 를 만든다. 운영자는 30s 이상을 권장하며, change-only 동작이 필요하면 `keepalive_interval=0` 으로 비활성화한다.

---

## 3. 요구사항 (Requirements)

### M1: 프레임 스캐너와 CRC

#### REQ-CENTURY-001: 에이전트 타입 등록

시스템은 **항상** `agent.DefaultManager` 에 `"century-hvac"` 에이전트 타입을 등록해야 한다.

- 등록 함수: `RegisterCenturyTypes(mgr *agent.DefaultManager) error`
- 팩토리: `func(config agent.AgentConfig) (agent.Agent, error)`
- LG LGCNP 와 동일한 등록 패턴을 따른다(`internal/agent/lg/lgcnp_register.go` 참조)
- `cmd/xflowd/main.go` 의 부트스트랩에 `century.RegisterCenturyTypes(agentMgr)` 호출을 추가한다.

#### REQ-CENTURY-002: 에이전트 설정 파싱

**WHEN** 사용자가 Century 에이전트를 생성할 때, **THEN** 시스템은 `AgentConfig.Transport.Options` 맵에서 다음 설정을 파싱해야 한다:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| transport_type | string | 선택 | "serial" | `serial` / `tcp-client` / `tcp-server` 중 하나. 그 외 값은 ErrUnknownTransportType 반환 (REQ-CENTURY-028) |
| serial_port | string | serial 모드 필수 | - | 시리얼 포트 경로 (예: `/dev/ttyUSB0`). transport_type 이 `tcp-*` 이면 무시됨 (검증 면제) |
| baud_rate | int | 선택 | 9600 | 캡처 환경에 따라 사용자 설정. 프로토콜 문서가 보레이트를 명시하지 않으므로 사용자가 측정 |
| data_bits | int | 선택 | 8 | 5/6/7/8 |
| stop_bits | int | 선택 | 1 | 1/2 |
| parity | string | 선택 | "none" | none/even/odd/mark/space |
| read_timeout | duration | 선택 | "200ms" | 0 이하는 200ms 로 클램프 |
| tcp_host | string | tcp-client 모드 필수 | tcp-server 모드 기본 "0.0.0.0" | TCP 호스트. tcp-client 미설정 시 ErrCenturyTCPHostRequired (REQ-CENTURY-028) |
| tcp_port | int | tcp-* 모드 필수 | - | TCP 포트 (1~65535). tcp-* 모드 미설정 시 ErrCenturyTCPPortRequired (REQ-CENTURY-028) |
| tcp_connect_timeout | duration | 선택 | "5s" | tcp-client `net.DialTimeout` 타임아웃 (REQ-CENTURY-029) |
| tcp_read_timeout | duration | 선택 | "3s" | TCP read 타임아웃. 초과 시 연결 종료 + 재연결 (REQ-CENTURY-029) |
| reconnect_initial | duration | 선택 | "5s" | tcp-client 재연결 backoff 초기값 (REQ-CENTURY-031) |
| max_reconnect_backoff | duration | 선택 | "5m" | tcp-client 재연결 backoff 상한 (REQ-CENTURY-031) |
| master_address | int (hex) | 선택 | 0x0030 | 마스터 주소(LE u16). 캡처 환경 의존 |
| slave_address | int (hex) | 선택 | 0x0001 | 슬레이브 주소(LE u16) |
| sub_dev_id | int (hex) | 선택 | 0x3B | payload prefix 의 sub_dev_id (indoor unit ID 추정) |
| ring_buffer_size | int | 선택 | 128 | 캡처 프레임 ring buffer 크기 |
| offline_timeout | duration | 선택 | "5s" | 폴링 주기(약 512ms)의 약 10배. 이 시간 동안 디바이스 프레임 미수신 시 오프라인 전이. TCP 모드의 연결 끊김 동안에도 이 timeout 후 device offline 표시 (REQ-CENTURY-014) |
| cycle_idle_timeout | duration | 선택 | transport-aware (serial=100ms, tcp-*=200ms) | polling cycle 경계 감지의 2차 fallback idle gap. 미설정 시 transport_type 에 따라 자동 결정 (REQ-CENTURY-032). 명시 시 transport 와 무관하게 그 값 사용 |
| auto_discovery | bool | 선택 | true | 회선상 관측된 `sub_dev_id` 를 디바이스로 자동 등록 |
| notify_interval | duration | 선택 | "0s" | 0 이면 상태 변경 시에만 알림, 양수면 주기적 알림 |
| log_decode_errors | bool | 선택 | false | per-error WARN 로그(CRC 불일치, 페이로드 prefix 위반 등) 토글. 통계 카운터는 항상 증가 |
| log_drops | bool | 선택 | false | ring buffer 가득 참으로 인한 프레임 드롭 시 per-drop WARN 로그 |
| log_unconfirmed_fields | bool | 선택 | false | 미확정(`unknown` 또는 `inferred`) 바이트가 알려진 값 외로 관측될 때 debug 로그 |
| dedupe_writes | bool | 선택 | true | 동일 polling cycle 내 중복 WRITE 프레임을 1개로 합침. raw frame 노드는 dedupe 와 무관하게 모든 frame emit. 자세한 cycle 경계 감지 휴리스틱은 REQ-CENTURY-027 참조 |
| emit_device_state | bool | 선택 | **true** (v0.3.0 default) | 통합 device_state 메시지를 msgCh 로 emit. v0.3.0 의 1차 출력 (REQ-CENTURY-033/034) |
| emit_register_decoded | bool | 선택 | **false** (v0.3.0 breaking default) | v0.2.x 와 같이 Reg02Decoded / Reg03Decoded / Reg04ReadDecoded / Reg04WriteDecoded / ACKDecoded 메시지를 msgCh 로 emit. v0.2.x 의 default true 에서 false 로 breaking 변경. Migration: 기존 소비자는 명시적으로 true 설정 필요 (REQ-CENTURY-034) |
| keepalive_interval | duration | 선택 | **60s** (v0.3.0) | device_state 의 fallback emit 주기. 변경 감지 없이 이 시간 경과 시 `trigger="keepalive"` emit. `0` 이면 keepalive 비활성 (change-only). 권장 최소 30s (A16). emit_device_state=false 시 무시됨 (REQ-CENTURY-035) |

#### REQ-CENTURY-003: 프레임 스캐너 (frame scanner)

시스템은 **항상** 바이트 스트림에서 Century 프레임 경계를 탐지해야 한다.

- 입력: `io.Reader` (시리얼 포트 또는 동등한 트랜스포트)
- 출력: 완전한 프레임 바이트(`src(2) + dst(2) + payload_length(2) + reserved(1) + fc(1) + payload(N) + crc(2)` = `10 + N` 바이트)
- 절차:
  1. 헤더 8바이트 읽기 (`src`, `dst`, `payload_length` LE u16, `reserved`, `function_code`)
  2. `payload_length` 가 합리 범위(`0 < N ≤ max_payload_length`, 기본 256) 내인지 검증
  3. `payload` N 바이트 + CRC 2바이트 읽기
  4. 완성된 프레임을 다음 단계(CRC 검증, 헤더 검증, 페이로드 prefix 검증)로 넘긴다
- **WHEN** `reserved` 바이트가 `0x00` 이 아니거나 `function_code` 가 알려진 값(`0x06`/`0x0B`/`0x0C`) 외이면, **THEN** 프레임을 폐기하고 다음 가능한 SOF 후보부터 재동기화한다.
- **IF** 헤더와 트레일 바이트 간 inter-frame gap 이 발생하면(읽기 중 일시 정지), 시스템은 최대 ~50 ms 의 in-frame gap 을 허용해야 한다. 그 이상은 incomplete frame 으로 폐기한다.

#### REQ-CENTURY-004: CRC-16/ARC 구현

시스템은 **항상** 다음 매개변수의 CRC-16/ARC 를 구현해야 한다:

- Polynomial: `0x8005` (reflected `0xA001`)
- Init: **`0x0000`** (Modbus RTU 의 `0xFFFF` 가 아님)
- RefIn / RefOut: `true` / `true`
- XorOut: `0x0000`
- 적용 범위: 프레임의 헤더(8B) + payload(N), CRC 2바이트 제외
- 저장 순서: little-endian (`frame[8+N] = crc & 0xFF`, `frame[8+N+1] = (crc >> 8) & 0xFF`)

**IF** 수신 프레임의 CRC 가 일치하지 않으면, **THEN** 프레임을 폐기하고 `framesInvalid` 카운터를 증가시킨다. `log_decode_errors=true` 이면 WARN 로그를 출력한다. 에이전트는 Error 상태로 전이하지 않는다.

**IF** 어떤 구현이 Modbus init `0xFFFF` 를 사용하면, 본 SPEC 의 캡처 프레임에 대해 검증 실패해야 한다 (회귀 방지 테스트 필수).

#### REQ-CENTURY-005: 헤더와 페이로드 prefix 파싱

**WHEN** 프레임 스캐너가 완성된 프레임을 전달하면, **THEN** 시스템은 다음을 추출해야 한다:

- `src_addr` (LE u16, payload offset 0–1)
- `dst_addr` (LE u16, 2–3)
- `payload_length` (LE u16, 4–5)
- `reserved` (1B, 6, 항상 `0x00`)
- `function_code` (1B, 7)
- `payload` (N B, 8 ~ 8+N-1)
- `crc` (LE u16, 8+N ~ 8+N+1)

**IF** `function_code` ≠ `0x06`(ACK) 이고 `payload_length` ≥ 3 이면, **THEN** 페이로드 prefix 를 추가로 파싱한다:

- `sub_dev_id` (1B, payload[0])
- `reserved2` (1B, payload[1], `0x00`)
- `register` (1B, payload[2], `0x02`/`0x03`/`0x04`)
- `data` (payload[3:])

**WHEN** `payload[1]` 이 `0x00` 이 아니거나 `register` 가 `{0x02, 0x03, 0x04}` 외이면, **THEN** 프레임을 미확정(`unknown`)으로 분류하고 raw frame 노드로만 노출한다(디코드된 status 이벤트 생성 X). 통계로 `payloadPrefixInvalid` 를 증가시킨다.

#### REQ-CENTURY-006: Register 0x02 응답 디코딩 (Read response, 17B data)

**WHEN** `function_code=0x06` 이고 `register=0x02` 이며 `data` 길이가 17B 이면, **THEN** 시스템은 다음 17 바이트를 모두 타입드 필드로 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `reg02_byte_0` | u8 | unknown | 4 캡처 모두 `0x00` |
| 1 | `mode` | u8 enum | **confirmed** | 운전 모드 (`0x00`=off/standby, `0x01`=cooling, 기타 미관측) |
| 2 | `fan` | u8 | **confirmed** | 바람 세기 (CAP-3: `0x11`=17, 1~3단 아닌 수치형 step) |
| 3–6 | `reg02_byte_3` ~ `reg02_byte_6` | u8 | unknown | 4 캡처 모두 `0x00` |
| 7–8 | `setpoint_c` | LE u16 ÷ 10.0 | **confirmed** | 설정 온도 (`0x00FA`=25.0℃) |
| 9–10 | `reg02_byte_9` ~ `reg02_byte_10` | u8 | unknown | 4 캡처 모두 `0x00` |
| 11–12 | `reg02_word_11` | LE u16 ÷ 10.0 | inferred | 25.0℃ 관측, setpoint 복제 또는 다른 슬롯 가능성 |
| 13 | `reg02_live_13` | u8 | inferred | CAP-3 `0x1B`=27, 운전 시 채워짐 |
| 14 | `reg02_live_14` | u8 | inferred | CAP-3 `0x39`↔`0x38` 미세 변동, 라이브 센서 추정 |
| 15 | `reg02_live_15` | u8 | inferred | CAP-3 `0x39`=57 |
| 16 | `reg02_byte_16` | u8 | unknown | reserved 추정 |

`mode` enum 디코딩은 **additive**: `0x00`/`0x01` 외 값은 `mode_unknown_<hex>` 문자열로 노출하고 `confirmation_status=unknown` 마커를 부여한다. 후속 SPEC 에서 확정된 값을 enum 에 추가할 때 기존 동작을 깨지 않는다.

**IF** `data` 길이가 17B 가 아니면, **THEN** 프레임을 `lengthMismatch` 로 폐기한다.

#### REQ-CENTURY-007: Register 0x03 응답 디코딩 (Read response, 16B data)

**WHEN** `function_code=0x06` 이고 `register=0x03` 이며 `data` 길이가 16B 이면, **THEN** 시스템은 다음을 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0–1 | `temp_evap_a_c` | LE u16 ÷ 10.0 | **confirmed** | 증발기 냉매 배관 온도 word0 (위치 미지정, "a") |
| 2–3 | `temp_evap_b_c` | LE u16 ÷ 10.0 | **confirmed** | 증발기 냉매 배관 온도 word1 ("b") |
| 4–15 | `reg03_pad_4` ~ `reg03_pad_15` | u8 | unknown | 4 캡처 모두 `0x00` (zero-padding) |

확정 근거: CAP-3(과도, 19.5/19.5℃) → CAP-4(정상, 9.0/8.5℃) 거동으로 증발기 냉매 배관 온도 2개 독립 센서임이 확정됨(프로토콜 문서 §6.2 v0.2→v0.3).

#### REQ-CENTURY-008: Register 0x04 응답 디코딩 (Read response, 14B data)

**WHEN** `function_code=0x06` 이고 `register=0x04` 이며 `data` 길이가 14B 이면, **THEN** 시스템은 다음을 노출해야 한다:

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `status_bits` | u8 bitmap | inferred | 캡처별 변동(`0x63`/`0x4D`/`0x3B`/`0x39`). 전원/모드/동적 비트 혼합 추정 |
| 1 | `reg04_const_1` | u8 | inferred | 4 캡처 모두 `0xF6` |
| 2 | `reg04_const_2` | u8 | inferred | 4 캡처 모두 `0x09` |
| 3–6 | `reg04_byte_3` ~ `reg04_byte_6` | u8 | unknown | 4 캡처 모두 `0x00` |
| 7 | `reg04_const_7` | u8 | inferred | 4 캡처 모두 `0x2C` |
| 8–9 | `op_val_1` | LE u16 | inferred | 정상운전 시 채워짐 (CAP-4: 996). 압축기 주파수/소비전력/적산값 추정 |
| 10–11 | `temp_A_c` | LE u16 ÷ 10.0 | inferred | 운전 중 `25.2℃` 안정. 실내/리턴에어 온도 추정 |
| 12–13 | `op_val_2` | LE u16 | inferred | 운전 부하 변동 (CAP-3: 252, CAP-4: 1248) |

`status_bits` 는 개별 비트 디코딩을 시도하지 않는다(미확정). 전체 바이트를 그대로 노출하여 downstream 분석을 가능하게 한다.

#### REQ-CENTURY-009: Register 0x04 Write 요청 디코딩 (Write request, 16B data)

**WHEN** `function_code=0x0C` 이고 `register=0x04` 이며 `data` 길이가 16B 이면, **THEN** 시스템은 다음을 노출해야 한다(본 에이전트가 송신하는 것이 아니라 회선상 관측된 마스터의 명령):

| data 오프셋 | 필드명 | 인코딩 | confirmation_status | 의미 |
|---|---|---|---|---|
| 0 | `write_live_0` | u8 | inferred | 운전 중 set, CAP-4: `0x02` |
| 1 | `write_live_1` | u8 | inferred | 운전 중 set, CAP-4: `0x04` |
| 2–3 | `write_byte_2` ~ `write_byte_3` | u8 | unknown | 4 캡처 모두 `0x00` |
| 4 | `mode_cmd` | u8 enum | **confirmed** | 마스터의 모드 명령 (`0x00`=off, `0x01`=cooling, 기타 미관측). REQ-CENTURY-006 의 `mode` enum 과 동일 디코딩 규칙 |
| 5–13 | `write_byte_5` ~ `write_byte_13` | u8 | unknown | 4 캡처 모두 `0x00` |
| 14 | `write_byte_14` | u8 | inferred | 캡처별 `0xC4`/`0xC7`/`0xC0` (19.2~19.9 범위). 마스터 측 설정/펌웨어 추정, 미확정 |
| 15 | `write_live_15` | u8 | inferred | CAP-4 내 `0x0F`~`0x11` 불규칙 변동. 마스터 측 라이브 센서값 추정 |

이 디코딩은 **passive observation** 임을 명확히 한다. 본 에이전트는 어떠한 경우에도 WRITE 프레임을 송신하지 않는다.

#### REQ-CENTURY-010: ACK 프레임 처리 (1-byte payload)

**WHEN** `function_code=0x06` 이고 `payload_length=1` 이고 `payload=[0x00]` 이면, **THEN** 시스템은 이를 `ack` 이벤트로 노출해야 한다.

- `src_addr` 가 슬레이브(`0x0001`)인 ACK 는 직전 WRITE 에 대한 슬레이브의 확인이다
- ACK 프레임은 페이로드 prefix(sub_dev_id/register) 가 없으므로 별도 디코더로 처리한다

#### REQ-CENTURY-011: 다층 검증 순서

시스템은 **항상** 수신 프레임에 대해 다음 검증을 순서대로 수행해야 한다:

1. **길이 검증**: `len(frame) == 10 + payload_length`
2. **CRC 검증**: REQ-CENTURY-004 의 CRC-16/ARC 계산값 일치
3. **헤더 검증**: `reserved == 0x00`, `function_code ∈ {0x06, 0x0B, 0x0C}`
4. **페이로드 prefix 검증** (ACK 제외): `payload[1] == 0x00`, `register ∈ {0x02, 0x03, 0x04}`
5. **레지스터별 길이 검증**: 각 (function_code, register, role) 조합에 대해 정확한 `data` 길이

각 단계에서 실패한 프레임은 해당 단계의 통계 카운터를 증가시키고 후속 단계를 건너뛴다.

#### REQ-CENTURY-012: 캡처 ring buffer 와 드롭 처리

시스템은 **항상** 검증된 프레임을 ring buffer 에 저장해야 한다.

- 크기: `ring_buffer_size` (기본 128, 폴링 주기 약 512ms 기준 약 65초 분량)
- 동시성: atomic / lock-free 또는 short-lived mutex
- **WHEN** ring buffer 가 가득 차고 새 프레임이 들어오면, **THEN** 가장 오래된 프레임을 evict 하고 `framesDropped` 카운터를 증가시킨다.
- **IF** `log_drops=true` 이면, drop 시 per-drop WARN 로그를 출력한다(기본 `false`).
- 모든 드롭은 `log_drops` 값과 무관하게 `framesDropped` 통계로 계수되어야 한다 (samsung-nasa `log_decode_errors` 패턴과 일관).

### M2: 디바이스 관리

#### REQ-CENTURY-013: 디바이스 자동 발견 (다중 IDU 포함)

**WHEN** `auto_discovery=true` 이고 회선상 관측된 프레임의 `sub_dev_id` 가 처음 등장하면, **THEN** 시스템은 `CenturyDevice` 를 자동 등록해야 한다.

- 디바이스 키: `sub_dev_id` (예: `"3b"`)
- `Source`: `"auto"` (자동 발견) / `"config"` (사용자 설정)
- `Label`: 기본 `"indoor-<sub_dev_id_hex>"`, 사용자 설정으로 override 가능

**WHEN** 새로운 `sub_dev_id` 가 frame 에서 관측되면, **THEN** 시스템은 자동으로 새 `CenturyDevice` 인스턴스를 생성하고 `Source="auto"` 로 표시해야 한다. 기존 device 는 `sub_dev_id` 를 키로 lookup 되며 무관한 `sub_dev_id` frame 은 별도 device 로 분리되어 추적되어야 한다.

- v0.1.0 은 다중 indoor unit 자동 발견을 **포함** 한다 (A7 참조). 현재 검증된 캡처는 단일 유닛(`0x3B`)뿐이지만, `devices map[uint8]*CenturyDevice` 데이터 모델이 임의 개수의 sub_dev_id 를 키로 관리하며, 각 device 의 상태는 독립적으로 유지된다.
- 다중 IDU 코드 경로는 단위 테스트(합성 frame 의 다중 sub_dev_id 매핑) 와 추론 기반 시나리오(B1a/B1b) 로 검증한다 — 실제 다중 유닛 캡처가 확보되면 후속 SPEC 에서 명시적 ground truth 추가.

#### REQ-CENTURY-014: 온라인/오프라인 감지

**WHEN** 디바이스에서 `offline_timeout` 기간 동안 어떤 프레임도 수신되지 않으면, **THEN** 시스템은 해당 디바이스를 오프라인으로 전이해야 한다.

- 기본 `offline_timeout` = 5초 (폴링 주기 약 512ms 의 약 10배)
- 오프라인 전이 시 `onDeviceStateChange` 콜백 호출
- 디바이스의 마지막 알려진 status 는 보존(다음 프레임 수신까지 stale 표시)
- **TCP 모드 거동 (v0.2.0)**: TCP 모드에서 연결 끊김 동안 frame 수신이 없으므로 `offline_timeout` 후 device 가 offline 으로 표시된다. 재연결 + 수신 재개 시 다음 frame 수신 시점에 자동으로 online 으로 복귀한다. 재연결 backoff(REQ-CENTURY-031, 기본 5s~5min) 가 길게 누적되는 동안 device 는 offline 상태에 머무른다.

#### REQ-CENTURY-015: DeviceProvider 인터페이스 구현

시스템은 **항상** `device.DeviceProvider` 인터페이스를 구현하여 디바이스 목록을 노출해야 한다.

- `ListDevices() []device.Device`
- 각 디바이스는 `ID`, `Name`, `Properties` (현재 status 의 평탄화된 맵) 노출
- 통일 속성명(LGCNP/NASA 와 정렬): `power`, `mode`, `fan_speed`, `target_temp`, `current_temp`
- Century 전용 속성: `temp_evap_a`, `temp_evap_b`, `op_val_1`, `op_val_2`, `status_bits`

### M3: 플로우 노드

#### REQ-CENTURY-016: CenturyStatusNode (SourceNode)

시스템은 **항상** `century-status` 노드 타입을 제공해야 한다.

- 인터페이스: `SourceNode` (폴링 기반)
- 설정 필드: `agent_ref` (필수), `poll_interval` (기본 `100ms`), `timeout` (기본 `5s`), `poll_command` (기본 `drain`), `recent_count` (기본 10), `batch_size` (기본 32)
- 동작: 에이전트의 ring buffer 에서 디코딩된 reg 0x02/0x03/0x04 응답을 폴링하여, 각 프레임을 개별 status 이벤트 메시지로 `out` 포트에 송출
- `FrameNotifyCh` 지원: 에이전트 알림 즉시 반응
- 메타데이터: `century_source="poll_bulk"`, `century_node_id=<node id>`

#### REQ-CENTURY-017: CenturyControlNode (ProcessNode, not_supported)

시스템은 **항상** `century-control` 노드 타입을 제공해야 한다 (API 대칭성용 placeholder).

- 인터페이스: `Node.Process(ctx, msg)`
- **WHEN** 어떤 메시지가 `Process` 로 전달되면, **THEN** 노드는 **항상** 다음 응답을 반환해야 한다:

  ```json
  {
    "status": "not_supported",
    "reason": "century_passive_only",
    "message": "Century HVAC agent operates in passive sniff mode; control commands are never transmitted",
    "request": <원본 메시지의 payload 요약>
  }
  ```

- 노드는 절대로 에이전트의 트랜스포트 Write 경로를 호출하지 않는다.
- `error` 포트가 아닌 `out` 포트로 응답을 반환한다(downstream 로직이 status 필드로 분기 가능).

#### REQ-CENTURY-018: CenturyNode (combined SourceNode + ProcessNode)

시스템은 **항상** `century` 통합 노드 타입을 제공해야 한다.

- SourceNode 동작: REQ-CENTURY-016 과 동일하게 status 이벤트 송출
- ProcessNode 동작: 입력 메시지에 제어 키(`power`, `mode`, `temperature`, `setpoint`, `fan_speed`)가 포함되면 REQ-CENTURY-017 의 not_supported 응답을 반환. 제어 키가 없으면 상태 조회(`get_stats`/`get_recent`) 결과를 반환.
- LGCNP 의 `LGCNPNode` 통합 패턴과 일관 (`internal/node/lgcnp.go`)

#### REQ-CENTURY-019: CenturyRawFrameNode (SourceNode, 디버깅·역공학용)

시스템은 **항상** `century-raw-frame` 노드 타입을 제공해야 한다.

- 인터페이스: `SourceNode`
- 동작: 에이전트가 캡처한 **모든** 프레임(유효/무효 모두)을 디코딩 적용 **이전**의 원시 바이트로 송출
- payload 구조:
  - `raw` (bytes): 헤더 + payload + CRC 전체 (`10 + N` 바이트)
  - `src_addr`, `dst_addr` (u16)
  - `function_code` (u8)
  - `payload_length` (u16)
  - `register` (u8 or null, ACK 는 null)
  - `crc_ok` (bool)
  - `validation_stage` (string): `"length"` / `"crc"` / `"header"` / `"payload_prefix"` / `"register_length"` / `"ok"` — 어느 단계까지 통과했는지
  - `confirmation_status`: `"raw"` (마커 자체가 raw 임을 명시)
  - `timestamp` (`int64`, epoch milliseconds)
- 용도: 디버깅, 프레임 capture, 후속 캡처를 통한 미확정 필드 의미 확정 작업 지원

### M4: 메시지 페이로드 스키마

#### REQ-CENTURY-020: 타입드 페이로드 스키마와 confirmation_status 마커

시스템은 **항상** 디코딩된 프레임의 모든 바이트를 페이로드의 타입드 필드로 노출해야 한다.

- 각 필드 그룹별 분리된 객체 구조(예시는 §4.4 참조)
- 모든 필드는 다음 메타와 함께 노출:
  - `value`: 실제 값(숫자/문자열)
  - `raw`: 원시 바이트 값(hex 또는 u8/u16)
  - `confirmation_status`: `"confirmed"` / `"inferred"` / `"unknown"` 중 하나
- 모든 timestamp 는 `epoch_milliseconds` (`int64`, `UnixMilli()`) — 프로젝트 컨벤션 (`project_timestamp_convention.md`)

**v0.3.0 갱신**: v0.3.0 부터 1차 msgCh 출력은 device-centric `DeviceStateEvent` (REQ-CENTURY-033) 이며, 본 REQ 가 정의하는 register-decoded 타입드 페이로드는 `emit_register_decoded=true` 옵션 활성화 시에만 emit 된다 (REQ-CENTURY-034). 두 stream 은 모두 `type` 필드로 구분된다 (`"device_state"` vs `"century_reg02_response"` 등). 추정 필드 (`inferred` / `unknown` 마커) 는 register-decoded 메시지에만 노출되며, DeviceStateEvent 는 5개 핵심 + 메타 + 증발기 온도만 노출한다.

#### REQ-CENTURY-021: confirmation_status 분류 기준

시스템은 **항상** 다음 분류 기준을 적용해야 한다:

- **confirmed**: 프로토콜 문서 §8.1 "확정" 섹션에 해당. CAP-3/CAP-4 의 ground truth 또는 명확한 물리적 거동(예: 증발기 냉매 배관 온도)으로 검증됨
- **inferred**: 4개 캡처에서 일관된 패턴 또는 합리적 추론. 의미는 확실치 않으나 라이브 변동, 일관 상수 등 동작 특성이 확인됨
- **unknown**: 4개 캡처 모두 `0x00` 인 reserved/zero-padding 또는 의미 추정 불가
- 향후 캡처로 의미 확정 시: SPEC 후속 버전(0.2.0+)에서 `unknown` → `inferred` → `confirmed` 진화. 이때 필드명은 **유지** 하고 `confirmation_status` 만 갱신한다(downstream 호환). 의미가 달라질 때만 새 필드명을 추가하고 구필드명은 `deprecated` 마킹.

### M5: Web UI 스키마

#### REQ-CENTURY-022: Web UI 에이전트 스키마

시스템은 **항상** `web/src/config/agentSchemas.ts` 에 다음을 추가해야 한다:

- `AGENT_TYPES` 배열에 `{ value: 'century-hvac', label: 'Century HVAC (passive)' }`
- `CENTURY_HVAC_FIELDS` 상수: REQ-CENTURY-002 의 모든 설정 필드를 `ConfigField` 로 노출
- `transport_type` 의 visibleWhen 으로 `serial_port`, `baud_rate`, `data_bits`, `stop_bits`, `parity` 표시 제어
- `master_address`/`slave_address`/`sub_dev_id` 는 hex 입력 허용 (예: `"0x3B"` 또는 `"3B"`)

#### REQ-CENTURY-023: Web UI 노드 스키마

시스템은 **항상** `web/src/config/nodeSchemas.ts` 에 다음을 추가해야 한다:

- `century-status` (카테고리: io, 출력 포트: `out`/`error`)
- `century-control` (카테고리: io, 출력 포트: `out`/`error`, 설명에 "제어 미지원 (패시브 전용)" 명시)
- `century` 통합 (카테고리: io)
- `century-raw-frame` (카테고리: io 또는 debug, 출력 포트: `out`)
- 모든 노드의 `agent_ref` 옵션에 `century-hvac` 포함

### M6: 품질 게이트와 운영

#### REQ-CENTURY-024: 테스트 커버리지와 골든 픽스처

시스템은 **항상** 다음 테스트를 포함해야 한다:

- `go test -race ./internal/agent/century/...` 통과
- `go test -race ./internal/node/...` 통과 (century 노드 포함)
- 패키지 커버리지 ≥85%
- 골든 픽스처: 프로토콜 문서 부록 A(CAP-3 냉방 시작), 부록 B(CAP-1 꺼짐), 부록 C(CAP-4 냉방 정상) 의 원시 프레임 hex 를 testdata 로 보존하고, 디코딩 결과의 회귀를 검증한다
- CRC 회귀 테스트: Modbus init `0xFFFF` 사용 구현이 본 SPEC 의 캡처 프레임에 실패함을 명시적으로 검증 (REQ-CENTURY-004)

#### REQ-CENTURY-025: 구조화 로그와 통계

시스템은 **항상** 다음 통계를 atomic 카운터로 추적해야 한다:

- `framesCaptured` (전체 캡처 시도)
- `framesValid` (다층 검증 통과)
- `framesInvalid` (검증 실패: 단계별 세분화 — `lengthMismatch`, `crcMismatch`, `headerInvalid`, `payloadPrefixInvalid`, `registerLengthInvalid`)
- `framesDropped` (ring buffer 가득 참)
- `bytesReceived`
- 디코딩별 카운터: `reg02ResponseCount`, `reg03ResponseCount`, `reg04ResponseCount`, `reg04WriteCount`, `ackCount`, `unconfirmedFieldObservations`

구조화 로그(`slog`)는 다음을 출력한다:

- CRC 불일치 (level: WARN, `log_decode_errors=true` 일 때만 per-error)
- ring buffer 드롭 (level: WARN, `log_drops=true` 일 때만 per-drop)
- 미확정 필드의 새 관측값 (level: DEBUG, `log_unconfirmed_fields=true` 일 때만; 예: `mode` 가 `0x02` 이상 처음 관측)
- 디바이스 자동 발견 (level: INFO, 1회)
- 디바이스 오프라인/온라인 전이 (level: INFO)

#### REQ-CENTURY-026: 확장성 — additive enum 과 reserved 필드명

시스템은 **항상** 다음 확장 정책을 따라야 한다:

- `mode`/`mode_cmd` enum 은 **additive only**: 새 코드(`0x02` 이상)가 확정되면 enum 에 **추가** 한다. 기존 코드(`0x00`/`0x01`) 의 의미를 변경하지 않는다.
- 미확정(`unknown`/`inferred`) 필드의 이름은 캡처 위치 기반으로 명명(`reg02_byte_3`, `write_live_15` 등) 하여, 의미가 확정되면 새 alias 필드명을 추가하고 기존 이름을 `deprecated` 마킹한다. 기존 이름의 값은 새 이름과 동일하게 유지된다(transitional duplication).
- payload 스키마의 새 필드 추가는 SemVer minor 버전 증가, 의미 변경은 major 버전 증가에 해당한다.

#### REQ-CENTURY-027: WRITE 중복 제거

Century 마스터는 신뢰성 목적으로 각 polling cycle 마다 동일 WRITE 프레임을 두 번 전송한다(A2 참조). 이 redundancy 를 다운스트림에 그대로 노출하지 않기 위한 옵션을 제공한다.

**WHEN** `dedupe_writes=true`(기본값)이고 같은 polling cycle 내에서 동일한 raw payload 를 가진 두 번째 WRITE 프레임이 관측되면, **THEN** 시스템은 두 번째 프레임에 대한 decoded event 를 emit 하지 않아야 한다(첫 번째만 emit). raw frame 노드는 dedupe 와 무관하게 모든 frame 을 emit 한다.

**IF** `dedupe_writes=false` 이면 **THEN** 모든 WRITE 프레임이 그대로 emit 되어야 한다.

**WHEN** cycle 경계 감지가 필요할 때, **THEN** 시스템은 마지막 READ response(reg `0x04` 응답) 또는 ACK 직후를 cycle 시작점으로 간주한다. 또는 inter-frame idle 이 `100ms` 를 초과하면 새 cycle 로 간주한다.

- 중복 판정 기준: `(function_code, sub_dev_id, register, payload data 의 raw bytes)` 가 동일
- dedup 된 frame 은 `framesValid` 통계에는 계수되지만 별도 카운터(`writesDeduped`)로도 추적
- `log_drops=true` 인 경우 dedup 된 frame 도 DEBUG 레벨로 로깅 가능 (운영자가 cycle 경계 휴리스틱을 검증할 수 있게)

### M6: TCP Transport (v0.2.0)

#### REQ-CENTURY-028: Transport type 확장

**WHEN** `transport_type` YAML 필드가 `serial` / `tcp-client` / `tcp-server` 중 하나로 설정되면, **THEN** `parseCenturyConfig` 는 해당 값을 `CenturyConfig.TransportType` 에 저장하고 성공적으로 반환해야 한다.

**IF** `transport_type` 가 위 3가지 중 하나가 아니면, **THEN** `ErrUnknownTransportType` 을 반환해야 한다.

**IF** `transport_type` 가 `tcp-client` 또는 `tcp-server` 이면, **THEN** `tcp_port` 가 필수이고, 1~65535 범위여야 한다. 미설정 또는 범위 위반 시 `ErrCenturyTCPPortRequired` 반환.

**IF** `transport_type` 가 `tcp-client` 이면, **THEN** `tcp_host` 가 필수이다. 미설정 시 `ErrCenturyTCPHostRequired` 반환.

**WHEN** `transport_type` 가 `tcp-server` 이고 `tcp_host` 미설정 시, **THEN** 기본 `0.0.0.0` 으로 바인드한다.

**WHEN** `transport_type` 가 `tcp-*` 이면, **THEN** `serial_port` 필드는 무시된다 (검증 면제). 동시에 `baud_rate`/`data_bits`/`stop_bits`/`parity`/`read_timeout` 의 serial-specific 필드도 무시된다.

#### REQ-CENTURY-029: TCP-client transport

**WHEN** `agent.Start` 가 호출되고 `TransportType="tcp-client"` 이면, **THEN** `net.DialTimeout("tcp", host:port, tcp_connect_timeout)` 으로 연결한다.

- 연결 성공 시 `transportProvider` 가 반환하는 `io.ReadWriteCloser` 는 해당 `net.Conn` 을 RX-only 로 wrap 한다.
- frame scanner 는 `net.Conn` 을 `io.Reader` 로 사용하여 기존 serial 코드 경로를 재사용한다.

**WHEN** TCP 연결이 끊어지면, **THEN** captureLoop 는 `io.EOF` / `io.ErrUnexpectedEOF` / `net.OpError` 를 감지하고 재연결을 시도한다 (REQ-CENTURY-031 의 backoff 적용).

**WHEN** TCP read 가 `tcp_read_timeout` 을 초과하면, **THEN** 연결을 종료하고 재연결한다.

- `net.Conn.SetReadDeadline(time.Now().Add(tcp_read_timeout))` 을 매 read 직전에 갱신
- timeout 발생 시 `net.Error.Timeout()` 으로 감지하여 close → reconnect

**IF** dial 도중 `agent.Stop` 의 context cancel 이 발생하면, **THEN** dial 을 즉시 중단하고 종료한다.

#### REQ-CENTURY-030: TCP-server transport

**WHEN** `agent.Start` 가 호출되고 `TransportType="tcp-server"` 이면, **THEN** `net.Listen("tcp", host:port)` 으로 listen 한다.

**WHEN** 클라이언트가 접속하면, **THEN** `Accept` 후 단일 활성 연결로 capture 한다. 두 번째 동시 접속은 즉시 close 한다 (v0.2.0; 다중 연결은 v0.3.0 deferral, A11 참조).

- 두 번째 접속 거부 시 INFO 레벨 로그 출력 ("tcp-server: rejected secondary connection from <peer>")

**WHEN** 활성 연결이 종료되면, **THEN** 다시 `Accept` loop 로 돌아간다 (재연결 backoff 미적용 — listen 은 유지).

- listener 자체는 `agent.Stop` 호출까지 유지된다.
- 활성 연결의 종료(`io.EOF` 또는 read timeout) 와 listener 의 생명주기는 독립이다.

**IF** `agent.Stop` 이 호출되면, **THEN** listener 와 활성 연결을 모두 close 하고 accept loop 가 종료된다.

#### REQ-CENTURY-031: 재연결 backoff (TCP-client)

**WHEN** TCP-client 연결 실패 또는 연결 끊김 시, **THEN** exponential backoff 를 적용한다:

- 초기: `reconnect_initial` (기본 5s)
- 매 실패 시: 2배 (5s → 10s → 20s → 40s → ...)
- 상한: `max_reconnect_backoff` (기본 5min)
- backoff 도중 jitter 는 v0.2.0 범위 외 (단순 deterministic doubling)

**IF** 재연결 성공 시, **THEN** backoff timer 를 `reconnect_initial` 로 리셋한다.

**WHEN** `agent.Stop` 이 호출되면, **THEN** 재연결 loop 가 즉시 종료된다 (context cancel).

- backoff sleep 중에도 context.Done() 을 select 하여 즉시 빠져나온다.
- 진행 중인 dial 도 context cancel 로 중단.

**IF** TCP-server 모드이면, **THEN** 본 REQ 의 backoff 는 적용되지 않는다 (REQ-CENTURY-030 의 accept loop 사용).

#### REQ-CENTURY-032: Transport-aware cycle_idle_timeout default

**IF** YAML 에 `cycle_idle_timeout` 이 명시되지 않았으면, **THEN** `TransportType` 에 따라 자동 결정한다:

- `serial` → 100ms
- `tcp-client` / `tcp-server` → 200ms

**IF** 사용자가 `cycle_idle_timeout` 을 명시하면, **THEN** transport 와 무관하게 그 값을 사용한다.

본 변경은 REQ-CENTURY-027 의 cycle 경계 휴리스틱 (2차 fallback idle gap) 과 상호작용한다. §5.6 Implementation Notes 의 cycle 경계 휴리스틱 설명도 transport-aware default 를 반영한다.

근거 (A12): TCP 모드에서는 Nagle 알고리즘, 패킷화, 네트워크 jitter 로 inter-frame 간격이 변동할 수 있어 serial 의 100ms 임계값을 그대로 사용하면 false break(같은 cycle 내 frame 을 다른 cycle 로 잘못 인식) 위험이 있다. 200ms 도 부족하면 사용자가 명시적으로 더 큰 값을 설정해야 한다.

### M7: Device-centric output (v0.3.0, Breaking)

#### REQ-CENTURY-033: DeviceStateEvent schema

시스템은 **항상** §4.4 예시 5 의 `DeviceStateEvent` JSON schema 와 정확히 일치하는 payload 를 msgCh 로 전송해야 한다 (snake_case 식별자, epoch ms timestamp).

스키마 (top-level JSON object):

| 필드 | 타입 | 의미 |
|------|------|------|
| `type` | string | 항상 `"device_state"` (downstream type 분기용) |
| `sub_dev_id` | string (hex) | 디바이스 식별자, `"0x3B"` 형식 |
| `label` | string | `CenturyDevice.Label` (예: `"indoor-3b"`) |
| `timestamp_ms` | int64 | event emission time (`time.Now().UnixMilli()`) |
| `last_seen_ms` | int64 | `CenturyDevice.LastSeen.UnixMilli()` |
| `online` | bool | `CenturyDevice.Online` |
| `power` | bool | `mode != 0x00` 이면 true (즉 off 가 아닌 모든 모드는 power=on) |
| `mode` | string | `ModeCode.String()` 결과 (`"off"` / `"cooling"` / `"unknown(0xNN)"`) |
| `fan` | uint8 | register 0x02 data[2] (raw uint8) |
| `set_temp_c` | float32 | register 0x02 data[7..8] (LE u16 ÷ 10.0) |
| `current_temp_c` | float32 | register 0x04 read response data[10..11] (LE u16 ÷ 10.0). 미수신 시 0.0 |
| `evap_temp_a_c` | float32 | register 0x03 data[0..1] (LE u16 ÷ 10.0). 미수신 시 0.0 |
| `evap_temp_b_c` | float32 | register 0x03 data[2..3] (LE u16 ÷ 10.0). 미수신 시 0.0 |
| `trigger` | string | `"change"` (값이 바뀌어서) 또는 `"keepalive"` (interval fallback) |

**IF** 디바이스가 한 번도 register 0x04 read response 를 받지 못했다면, **THEN** `current_temp_c` 는 0.0 으로 emit 된다 (또는 null — 구현 선택; 패키지 전체에 걸쳐 일관성을 유지).

**IF** 디바이스가 register 0x03 응답을 받지 못했다면, **THEN** `evap_temp_a_c` / `evap_temp_b_c` 는 0.0 (또는 null) 로 emit 된다.

**WHEN** mode 가 `0x00` 이면, **THEN** `power` 는 false 이고, 그 외 모든 mode 값은 `power=true` 이다 (A15, R15 참조).

추정 필드 (`status_bits` 비트, `op_val_1/2`, `write_*` 등) 는 DeviceStateEvent 에 노출하지 **않는다** — register-decoded 메시지에서만 접근 가능.

#### REQ-CENTURY-034: Output mode 옵션과 디폴트 변경 (Breaking)

**IF** YAML config 에 `emit_device_state` (기본 **true**) 가 설정되면, **THEN** captureLoop 는 §4.4 예시 5 의 DeviceStateEvent 메시지를 msgCh 로 emit 한다.

**IF** `emit_register_decoded` (기본 **false** — v0.2.x 의 true 에서 breaking 변경) 가 true 면, **THEN** captureLoop 는 v0.2.x 와 동일하게 Reg02Decoded / Reg03Decoded / Reg04ReadDecoded / Reg04WriteDecoded / ACKDecoded 메시지도 msgCh 로 emit 한다. 두 stream 은 interleaved 로 emit 되며 downstream 은 payload 의 `type` 필드로 구분한다 (`"device_state"` vs `"century_reg02_response"` 등).

**IF** 두 옵션이 모두 false 이면, **THEN** `parseCenturyConfig` 는 `ErrCenturyNoOutputEnabled` 를 반환한다. 최소 하나의 output stream 은 활성화되어야 한다.

**WHEN** 두 옵션이 모두 true 이면, **THEN** 같은 frame 이 device_state 와 register-decoded 메시지 둘 다 트리거할 수 있다. 소비자는 `type` 필드로 분기한다.

**Migration (v0.2.x → v0.3.0)**:

- v0.2.x 의 register-decoded 출력을 기대하는 소비자는 `emit_register_decoded: true` 를 명시 설정해야 한다.
- v0.3.0 의 default device_state 소비자는 `type=="device_state"` 만 처리한다.
- 두 stream 을 모두 소비하면서 점진 마이그레이션도 가능하다.

#### REQ-CENTURY-035: 변경 감지 + keepalive fallback

**WHEN** 디바이스의 5개 핵심 필드 (`power` / `mode` / `fan` / `set_temp_c` / `current_temp_c`) 중 하나라도 이전 emit 값과 다르면, **THEN** 시스템은 `trigger="change"` 로 즉시 device_state 를 emit 한다.

**WHEN** 디바이스의 `online` 상태가 true → false 또는 false → true 로 바뀌면, **THEN** 시스템은 즉시 `trigger="change"` 로 emit 한다 (offline 전이도 downstream 에 알림).

**WHEN** 디바이스가 마지막 emit 후 `keepalive_interval` (기본 60s, 0=비활성화) 동안 변경 감지가 없으면, **THEN** 시스템은 `trigger="keepalive"` 로 emit 한다.

**IF** `keepalive_interval=0` 이면, **THEN** keepalive fallback 은 비활성화되어 오직 change-only emit 만 발생한다.

**IF** `emit_device_state=false` 이면, **THEN** 변경 감지 / keepalive 로직 자체가 비활성화된다 (REQ-CENTURY-034 와 결합).

증발기 온도 (`evap_temp_a_c` / `evap_temp_b_c`) 의 변동은 emit 트리거가 아니다 (A15). 가장 최근에 관측된 값이 emit 되는 device_state payload 에 동봉만 된다.

---

## 4. 명세 (Specifications)

### 4.1 파일 구조 (신규)

```
internal/agent/century/
  agent.go           # CenturyAgent: lifecycle + capture loop + interface 구현
  config.go          # CenturyConfig + parseCenturyConfig
  device.go          # CenturyDevice, CenturyDeviceState, DeviceProvider 구현
  errors.go          # 센티널 에러 정의
  crc.go             # CRC-16/ARC 구현 (init 0x0000)
  frame_scanner.go   # 헤더 → payload_length → CRC 경계 탐지
  frame.go           # CenturyFrame 구조체, src/dst/fc/register 접근자
  register.go        # 디코더 디스패치 (function_code × register × role → decoder fn)
  decoder_reg02.go   # Register 0x02 응답 디코더 (17B)
  decoder_reg03.go   # Register 0x03 응답 디코더 (16B)
  decoder_reg04.go   # Register 0x04 응답 + Write 디코더
  decoder_ack.go     # ACK 프레임 디코더
  message.go         # MessagePayload 구성 (typed fields + confirmation_status)
  provider.go        # DeviceProvider 어댑터
  ring_buffer.go     # 캡처 프레임 ring buffer
  serial_opener.go   # 시리얼 트랜스포트 wrapper (SPEC-SERIAL-001 재사용)
  registration.go    # RegisterCenturyTypes (관례상 register.go 와 다른 파일에)
  agent_test.go
  crc_test.go
  frame_scanner_test.go
  decoder_reg02_test.go
  decoder_reg03_test.go
  decoder_reg04_test.go
  decoder_ack_test.go
  testdata/
    cap1_off.hex        # CAP-1 한 사이클 (꺼짐)
    cap3_cool_start.hex # CAP-3 한 사이클 (냉방 시작)
    cap4_cool_steady.hex# CAP-4 한 사이클 (냉방 정상)

internal/node/
  century.go            # CenturyStatusNode, CenturyControlNode, CenturyNode, CenturyRawFrameNode
  century_test.go

web/src/config/
  agentSchemas.ts       # CENTURY_HVAC_FIELDS 추가
  nodeSchemas.ts        # century-status, century-control, century, century-raw-frame 추가

cmd/xflowd/
  main.go               # century.RegisterCenturyTypes(agentMgr) 호출 추가 (보통 lg.RegisterLGCNPTypes 인근)

examples/config/
  century-hvac-passive.yaml  # 패시브 캡처 샘플 설정
```

### 4.2 인터페이스 합성

`CenturyAgent` 는 다음 인터페이스를 구현한다 (NASA 패턴 참조). 다중 IDU 지원을 위해 내부 상태는 `devices map[uint8]*CenturyDevice` (sub_dev_id 를 키로) 로 보관하며, 모든 디코딩 이벤트는 해당 `sub_dev_id` 의 device 인스턴스로 라우팅된다.

**Transport abstraction (v0.2.0)**: `transportProvider func() (io.ReadWriteCloser, error)` 가 transport_type 에 따라 분기한다 — `serial` 은 기존 SPEC-SERIAL-001 트랜스포트, `tcp-client` 는 `net.DialTimeout` wrapper, `tcp-server` 는 `net.Listen` + Accept loop wrapper. frame scanner 는 `io.Reader` 기반이므로 transport 와 무관하게 동일한 디코더 디스패치를 사용한다. 별도 신규 필드는 추가하지 않으며, `transportProvider` 의 factory 분기만으로 충분하다. AC-B9 (transport.Write 0회) 불변식은 wrapper 가 Write 경로 자체를 사용하지 않도록 보장한다 (A13).

**Device-centric emit state (v0.3.0)**: `CenturyAgent` 에 다음 필드가 추가된다 (REQ-CENTURY-033/034/035):

- `lastEmitState map[byte]deviceStateSnapshot` — sub_dev_id 별 마지막 emit 한 5개 핵심 필드 + online 값을 보관 (change detection 비교용)
- `lastEmitTime map[byte]time.Time` — sub_dev_id 별 마지막 emit 시각 (keepalive 경과시간 계산용)
- `keepaliveStopCh chan struct{}` — keepalive ticker goroutine 종료 신호
- `emitMu sync.Mutex` — lastEmitState / lastEmitTime 접근을 직렬화하는 mutex (변경 감지 분기와 keepalive ticker 가 동시 접근하므로 필요)

captureLoop 의 emit 분기는 다음과 같이 재설계된다:

1. frame decode 성공 → device.Update(decoded, now) (기존)
2. **신규**: `emit_device_state=true` 이면 device.Snapshot() → 5개 핵심 필드 + online 비교 → 변경 시 `trigger="change"` emit + lastEmitState/lastEmitTime 갱신
3. `emit_register_decoded=true` 이면 v0.2.x 와 동일하게 decoded 메시지 emit (interleaved)

별도 keepalive goroutine (1초 ticker) 이 각 device 의 `now - lastEmitTime[id] >= keepalive_interval` 을 검사하여 expired 시 `trigger="keepalive"` emit.

오프라인 전이 (offlineWatchLoop) 도 online=false 변경 시 즉시 `trigger="change"` emit 트리거 hook 추가.


- `agent.Agent` (필수: Init/Start/Stop/Pause/Resume/Configure/Process/ID/Name/Type/Info/Stats/Health)
- `agent.MessageReceiver` (`ReceiveMessage() <-chan []byte`)
- `agent.StatefulAgent` (`State() agent.AgentState`)
- `agent.TransportChecker` (`TransportConnected() bool`)
- `agent.BufferInfoProvider` (`BufferInfo() agent.BufferInfo`, `FrameNotifyCh() <-chan struct{}`)
- `device.DeviceProvider` (`ListDevices() []device.Device`)

`Process()` 커맨드는 다음을 지원한다:

- `get_stats`: 캡처 통계 JSON 반환
- `get_recent`: 최근 프레임 조회 (`count`, `last_seq` 인자)
- `drain`: ring buffer 의 모든 프레임 소비
- 그 외 알려지지 않은 커맨드: `not_supported` 오류 반환

### 4.3 디코더 디스패치 (register.go)

```go
type decoderKey struct {
    fc       byte // 0x06 / 0x0B / 0x0C
    register byte // 0x02 / 0x03 / 0x04
    role     role // master / slave (src_addr 기반 판정)
}

type decoderFn func(frame *CenturyFrame) (CenturyMessage, error)

var decoders = map[decoderKey]decoderFn{
    {0x06, 0x02, slaveResp}: decodeReg02Response,
    {0x06, 0x03, slaveResp}: decodeReg03Response,
    {0x06, 0x04, slaveResp}: decodeReg04Response,
    {0x0C, 0x04, masterReq}: decodeReg04Write,
    // ACK 는 payload_length=1 로 별도 분기
}
```

### 4.4 메시지 페이로드 예시 (REQ-CENTURY-020)

#### 예시 1: Register 0x02 응답 (CAP-3, 냉방 25℃, 바람 17)

```json
{
  "type": "century_reg02_response",
  "timestamp": 1737216000123,
  "seq": 1024,
  "src_addr": 1,
  "dst_addr": 48,
  "sub_dev_id": 59,
  "register": 2,
  "raw_hex": "010030001400000663b00020001110000000000fa0000000fa001b3939",
  "fields": {
    "mode":        { "value": "cooling",  "raw": 1,    "confirmation_status": "confirmed" },
    "fan":         { "value": 17,         "raw": 17,   "confirmation_status": "confirmed" },
    "setpoint_c":  { "value": 25.0,       "raw": 250,  "confirmation_status": "confirmed" },
    "reg02_byte_0":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_3":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_4":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_5":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_6":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_9":   { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_byte_10":  { "value": 0,    "raw": 0,    "confirmation_status": "unknown" },
    "reg02_word_11":  { "value": 25.0, "raw": 250,  "confirmation_status": "inferred" },
    "reg02_live_13":  { "value": 27,   "raw": 27,   "confirmation_status": "inferred" },
    "reg02_live_14":  { "value": 57,   "raw": 57,   "confirmation_status": "inferred" },
    "reg02_live_15":  { "value": 57,   "raw": 57,   "confirmation_status": "inferred" },
    "reg02_byte_16":  { "value": 0,    "raw": 0,    "confirmation_status": "unknown" }
  }
}
```

#### 예시 2: Register 0x04 응답 (CAP-4 정상상태)

```json
{
  "type": "century_reg04_response",
  "timestamp": 1737218640456,
  "seq": 5832,
  "sub_dev_id": 59,
  "register": 4,
  "fields": {
    "status_bits":   { "value": 57,   "raw": "0x39", "confirmation_status": "inferred" },
    "reg04_const_1": { "value": 246,  "raw": "0xf6", "confirmation_status": "inferred" },
    "reg04_const_2": { "value": 9,    "raw": "0x09", "confirmation_status": "inferred" },
    "reg04_byte_3":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_4":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_5":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_byte_6":  { "value": 0,    "raw": 0,      "confirmation_status": "unknown" },
    "reg04_const_7": { "value": 44,   "raw": "0x2c", "confirmation_status": "inferred" },
    "op_val_1":      { "value": 996,  "raw": 996,    "confirmation_status": "inferred" },
    "temp_A_c":      { "value": 25.2, "raw": 252,    "confirmation_status": "inferred" },
    "op_val_2":      { "value": 1248, "raw": 1248,   "confirmation_status": "inferred" }
  }
}
```

#### 예시 3: Register 0x04 Write 요청 (CAP-4)

```json
{
  "type": "century_reg04_write_request",
  "observation_mode": "passive",
  "timestamp": 1737218640712,
  "sub_dev_id": 59,
  "register": 4,
  "fields": {
    "write_live_0":  { "value": 2,    "raw": "0x02", "confirmation_status": "inferred" },
    "write_live_1":  { "value": 4,    "raw": "0x04", "confirmation_status": "inferred" },
    "mode_cmd":      { "value": "cooling", "raw": 1, "confirmation_status": "confirmed" },
    "write_byte_14": { "value": 192,  "raw": "0xc0", "confirmation_status": "inferred" },
    "write_live_15": { "value": 17,   "raw": "0x11", "confirmation_status": "inferred" }
  }
}
```

#### 예시 4: Raw frame (REQ-CENTURY-019)

```json
{
  "type": "century_raw_frame",
  "timestamp": 1737216000123,
  "raw": "0100300014000006...",
  "src_addr": 1,
  "dst_addr": 48,
  "function_code": 6,
  "payload_length": 20,
  "register": 2,
  "crc_ok": true,
  "validation_stage": "ok",
  "confirmation_status": "raw"
}
```

#### 예시 5: DeviceStateEvent (REQ-CENTURY-033, v0.3.0 default emit)

`emit_device_state=true` (기본값) 일 때 msgCh 의 1차 출력. top-level snake_case + epoch ms.

```json
{
  "type": "device_state",
  "sub_dev_id": "0x3B",
  "label": "indoor-3b",
  "timestamp_ms": 1715985000000,
  "last_seen_ms": 1715985000000,
  "online": true,
  "power": true,
  "mode": "cooling",
  "fan": 17,
  "set_temp_c": 25.0,
  "current_temp_c": 25.2,
  "evap_temp_a_c": 9.0,
  "evap_temp_b_c": 8.5,
  "trigger": "change"
}
```

`trigger` 의 값:
- `"change"`: 5개 핵심 필드 (power/mode/fan/set_temp_c/current_temp_c) 중 하나라도 이전 emit 값과 다를 때, 또는 online 전이 발생 시 (REQ-CENTURY-035).
- `"keepalive"`: 변경 없이 `keepalive_interval` (기본 60s) 경과 시 fallback emit (REQ-CENTURY-035).

미수신 register 의 필드는 0.0 으로 emit 된다 (구현 통일성 유지; REQ-CENTURY-033 의 미수신 정책 참조).

### 4.5 예시 YAML 에이전트 설정

#### 4.5.1 Serial transport 예시 (`examples/config/century-hvac-passive.yaml`)

```yaml
agents:
  - id: century-living-room
    type: century-hvac
    transport:
      type: serial
      options:
        serial_port: /dev/ttyUSB0
        baud_rate: 9600
        data_bits: 8
        stop_bits: 1
        parity: none
        read_timeout: 200ms
        master_address: 0x0030
        slave_address: 0x0001
        sub_dev_id: 0x3B
        ring_buffer_size: 128
        offline_timeout: 5s
        auto_discovery: true
        log_decode_errors: false
        log_drops: false
        log_unconfirmed_fields: false
        dedupe_writes: true
        # v0.3.0 device-centric output (breaking default)
        emit_device_state: true
        emit_register_decoded: false  # v0.2.x compatibility: 명시 true 로 활성화
        keepalive_interval: 60s

flows:
  - id: century-status-flow
    nodes:
      - id: src
        type: century-status
        agent_ref: century-living-room
        poll_interval: 100ms
      - id: log
        type: debug
    wires:
      - { from: src.out, to: log.in }

  - id: century-raw-capture-flow
    nodes:
      - id: src
        type: century-raw-frame
        agent_ref: century-living-room
      - id: store
        type: file-write
        options:
          path: /var/log/xflow/century-frames.jsonl
    wires:
      - { from: src.out, to: store.in }
```

#### 4.5.2 TCP-client transport 예시 (v0.2.0, xflow → 시리얼-Ethernet 컨버터)

xflow 가 능동적으로 외부 컨버터(예: Moxa NPort, USR-N520) 의 TCP 서버로 연결하는 구성. 컨버터가 RS-485 트래픽을 TCP 로 forward 한다.

```yaml
type: century-hvac
options:
  transport_type: tcp-client
  tcp_host: 192.168.1.100
  tcp_port: 4196
  tcp_connect_timeout: 5s
  tcp_read_timeout: 3s
  reconnect_initial: 5s
  max_reconnect_backoff: 5m
  master_address: 0x0030
  slave_address: 0x0001
  sub_dev_id: 0x3B
  auto_discovery: true
  dedupe_writes: true
```

#### 4.5.3 TCP-server transport 예시 (v0.2.0, 컨버터 → xflow push)

컨버터가 능동적으로 xflow 의 TCP listener 로 push 하는 구성. xflow 가 LAN 내 고정 IP/포트로 listen 하고, 컨버터가 그 endpoint 로 연결한다.

```yaml
type: century-hvac
options:
  transport_type: tcp-server
  tcp_host: 0.0.0.0
  tcp_port: 4197
  tcp_read_timeout: 3s
  master_address: 0x0030
  slave_address: 0x0001
  sub_dev_id: 0x3B
  auto_discovery: true
  dedupe_writes: true
```

TCP-server 모드는 단일 활성 연결만 처리한다(A11). 두 번째 접속은 즉시 거부된다.

### 4.6 추적성 태그

모든 REQ 의 구현 상태와 인계 commit 을 명시한다. 파일 경로는 `internal/agent/century/` 기준
(노드는 `internal/node/`, web 은 `web/src/config/`). v0.1.2 시점 27 REQ-CENTURY-001~027 은 자동 테스트로 검증된 Implemented 상태이며, v0.2.0 추가 REQ-CENTURY-028~032 은 M6 마일스톤에서 구현 예정인 Planned 상태이다.

| 요구사항 | 파일 | 함수/구조체 | 구현 상태 | 인계 Commit |
|----------|------|------------|----------|-------------|
| REQ-CENTURY-001 | registration.go, cmd/xflowd/main.go | RegisterCenturyTypes | Implemented | bad2e06 (M3) |
| REQ-CENTURY-002 | config.go | CenturyConfig, parseCenturyConfig | Implemented | bad2e06 (M3) |
| REQ-CENTURY-003 | frame_scanner.go | FrameScanner.Next, scanHeader | Implemented | bfdfaf0 (M1) |
| REQ-CENTURY-004 | crc.go | CRC16ARC | Implemented | bfdfaf0 (M1) |
| REQ-CENTURY-005 | frame.go, frame_parser.go | CenturyFrame, parsePayloadPrefix | Implemented | bfdfaf0 (M1) |
| REQ-CENTURY-006 | decoder_reg02.go | DecodeReg02Response | Implemented | d33da37 (M2) |
| REQ-CENTURY-007 | decoder_reg03.go | DecodeReg03Response | Implemented | d33da37 (M2) |
| REQ-CENTURY-008 | decoder_reg04.go | DecodeReg04Response | Implemented | d33da37 (M2) |
| REQ-CENTURY-009 | decoder_reg04.go | DecodeReg04Write | Implemented | d33da37 (M2) |
| REQ-CENTURY-010 | decoder.go | DecodeAck | Implemented | d33da37 (M2) |
| REQ-CENTURY-011 | decoder.go, frame_scanner.go | Dispatch (단계별 검증 체이닝) | Implemented | d33da37 (M2) |
| REQ-CENTURY-012 | ring_buffer.go, agent.go | RingBuffer.Push (드롭 카운팅), log_drops 분기 | Implemented | bad2e06 (M3) |
| REQ-CENTURY-013 | device.go, agent.go | CenturyAgent.discoverDevice (sub_dev_id 키 다중 IDU) | Implemented | bad2e06 (M3) |
| REQ-CENTURY-014 | device.go, agent.go | CenturyAgent.checkOfflineTimeout | Implemented | bad2e06 (M3) |
| REQ-CENTURY-015 | provider.go | CenturyDeviceProvider.ListDevices | Implemented | bad2e06 (M3) |
| REQ-CENTURY-016 | internal/node/century.go | CenturyStatusNode | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-017 | internal/node/century.go | CenturyControlNode (not_supported) | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-018 | internal/node/century.go | CenturyNode | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-019 | internal/node/century.go | CenturyRawFrameNode | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-020 | message.go | MessagePayload, FieldMeta | Implemented | d33da37 (M2) |
| REQ-CENTURY-021 | message.go | ConfirmationStatus enum | Implemented | d33da37 (M2) |
| REQ-CENTURY-022 | web/src/config/agentSchemas.ts | CENTURY_HVAC_FIELDS | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-023 | web/src/config/nodeSchemas.ts | century-* node entries | Implemented | 3f1b970 (M4) |
| REQ-CENTURY-024 | testdata/, *_test.go | golden fixture 회귀 테스트 | Implemented | bfdfaf0 (M1) → 3f1b970 (M4) |
| REQ-CENTURY-025 | agent.go | atomic counters, slog 구조화 로그 | Implemented | bad2e06 (M3) |
| REQ-CENTURY-026 | decoder_reg02.go, decoder_reg04.go | additive mode/mode_cmd enum | Implemented | d33da37 (M2) |
| REQ-CENTURY-027 | agent.go, cycle_tracker.go, write_deduplicator.go | cycle tracker + writeDeduplicator (writesDeduped 카운터) | Implemented | bad2e06 (M3) |
| REQ-CENTURY-028 | config.go, errors.go | parseCenturyConfig (transport_type 분기), ErrUnknownTransportType, ErrCenturyTCPPortRequired, ErrCenturyTCPHostRequired | Planned (M6) | - |
| REQ-CENTURY-029 | transport_tcp.go, agent.go | tcpClientProvider (net.DialTimeout, SetReadDeadline), captureLoop EOF 감지 | Planned (M6) | - |
| REQ-CENTURY-030 | transport_tcp.go, agent.go | tcpServerProvider (net.Listen, Accept loop, single-active 정책) | Planned (M6) | - |
| REQ-CENTURY-031 | transport_tcp.go, agent.go | reconnectWithBackoff (exponential, context-aware) | Planned (M6) | - |
| REQ-CENTURY-032 | config.go, cycle_tracker.go | parseCenturyConfig (cycle_idle_timeout transport-aware default) | Planned (M6) | - |
| REQ-CENTURY-033 | message.go, device.go | CenturyDeviceStateEvent (JSON marshaling), CenturyDeviceState.Snapshot (5 핵심 + 증발기) | Planned (M7) | - |
| REQ-CENTURY-034 | config.go, agent.go, errors.go | EmitDeviceState/EmitRegisterDecoded/KeepaliveInterval, ErrCenturyNoOutputEnabled, captureLoop emit 분기 | Planned (M7) | - |
| REQ-CENTURY-035 | agent.go | lastEmitState/lastEmitTime/emitMu, change detector, keepaliveLoop, offline 전이 hook | Planned (M7) | - |

**Acceptance 시나리오 자동 테스트 커버리지** (그룹 A~H, 총 59 시나리오; v0.1.2 41 + v0.2.0 8 + v0.3.0 10):

| 그룹 | 시나리오 수 | 대표 테스트 함수 | 위치 |
|------|------------|-----------------|------|
| A (프레임 디코딩) | 11 (A1-A11) | TestDecodeReg02Response_CAP3, TestDecodeReg03Response_CAP4, TestDecodeReg04Response_CAP4, TestDecodeReg04Write_CAP3, TestDecodeAck, TestCRC16ARC_CAP3, TestCRC16ARC_Modbus_Rejects_*, TestFrameScanner_* | crc_test.go, frame_scanner_test.go, decoder_*_test.go |
| B (에이전트 런타임) | 10 (B1a, B1b, B2-B10) | TestCenturyAgent_AutoDiscovery, TestCenturyAgent_MultiSubDevID, TestCenturyAgent_OfflineDetection, TestRingBuffer_OverflowDrop, TestCenturyAgent_Lifecycle, TestCenturyAgent_GetStats, TestCenturyAgent_DrainCommand, TestCenturyAgent_NoTransportWrite, TestCenturyAgent_BufferInfoFrameNotify | agent_test.go, ring_buffer_test.go, agent_coverage_test.go |
| C (플로우 노드) | 8 (C1-C8) | TestCenturyStatusNode_Poll, TestCenturyStatusNode_MissingAgentRef, TestCenturyControlNode_NotSupported, TestCenturyNode_CombinedDispatch, TestCenturyRawFrameNode_AllFrames, TestCenturyStatusNode_DeferredInit, TestCenturyStatusNode_FrameNotifyCh | internal/node/century_test.go |
| D (설정 및 등록) | 7 (D1-D7) | TestRegisterCenturyTypes, TestParseCenturyConfig_MissingSerialPort, TestParseCenturyConfig_HexInput, TestParseCenturyConfig_ReadTimeoutClamp, web/src/config/__tests__/centurySchema.test.ts, cmd/xflowd registration | registration_test.go, config_test.go, web tests |
| E (필드 디코딩 정책) | 5 (E1-E5) | TestMessagePayload_ConfirmationStatusMarkers, TestDecodeReg02_ModeAdditiveEnum, TestMessagePayload_TimestampEpochMS, TestMessagePayload_RawHex | message_test.go, decoder_reg02_test.go |
| F (WRITE 중복 처리) | 4 (F1-F4) | TestWriteDeduplicator_SameCycleDedupe, TestWriteDeduplicator_DisabledEmitsAll, TestCycleTracker_NewCycleAfterReg04Resp, TestCycleTracker_IdleFallback100ms | write_deduplicator_test.go, cycle_tracker_test.go, agent_test.go |
| G (TCP transport, v0.2.0) | 8 (G1-G8) | TestTCPClient_DialAndDecode, TestTCPClient_DialFailureBackoff, TestTCPClient_ReconnectAfterEOF, TestTCPClient_ReadTimeout, TestTCPServer_AcceptAndDecode, TestTCPServer_RejectSecondaryConnection, TestParseCenturyConfig_CycleIdleTimeoutDefault, TestTCPTransport_NeverWrites | Planned (M6): transport_tcp_test.go, agent_test.go |
| H (Device-centric output, v0.3.0) | 10 (H1-H10) | TestAgent_DeviceState_RegisterDecodedOptOut, TestAgent_DeviceState_FirstReg02EmitsChange, TestAgent_DeviceState_Reg04UpdatesCurrentTemp, TestAgent_DeviceState_SameValueNoReEmit, TestAgent_DeviceState_ModeTransitionEmitsPowerChange, TestAgent_DeviceState_KeepaliveAfterInterval, TestAgent_DeviceState_OfflineTransitionEmits, TestAgent_DeviceState_RegisterOnlyMode, TestAgent_DeviceState_BothOptionsOffReturnsError, TestAgent_DeviceState_MultiSubDevIDIndependent | Planned (M7): agent_test.go, message_test.go |

---

## 5. 구현 노트 (Implementation Notes)

### 5.1 프레임 경계 탐지 전략

- Century 프레임은 STX 가 없으므로 LGCNP/NASA 와 달리 **헤더 8B 사전 읽기 → `payload_length` 사용 → 정확한 길이 확보** 가 가능하다.
- 재동기화는 헤더 검증 실패 시(reserved≠0x00, fc 미지원, payload_length 비합리) 1바이트씩 shift 하며 다음 후보를 찾는 sliding window 방식.
- inter-frame in-flight gap 은 최대 ~50 ms 허용. 폴링 주기 약 512 ms 에 비해 충분히 짧음.

### 5.2 반이중 회선 sniff 의 송신 금지

- `SerialAgent` 의 `Process()` 가 Write 경로를 트리거하는 통상의 동작과 달리, Century 에이전트는 어떤 경로로도 Write 를 호출해서는 안 된다.
- `CenturyControlNode` 의 `not_supported` 응답은 agent.Process 를 거치지 않고 노드 내부에서 즉시 응답한다 (REQ-CENTURY-017).
- v0.2.0+ 에서 능동 폴링 모드를 추가할 경우, 본 SPEC 과 별도의 SPEC 으로 분리한다(예: SPEC-CENTURY-002 "Active polling and control" — 본 SPEC 의 호환을 깨지 않음).

### 5.3 골든 픽스처 (testdata)

프로토콜 문서 §부록 A/B/C 의 hex 덤프를 testdata 파일로 보존하고, 각 디코더 테스트에서 다음을 검증:

- **CAP-1** (꺼짐): mode=off, fan=0, setpoint=25.0℃, reg04 운전 필드 모두 0
- **CAP-3** (냉방 시작): mode=cooling, fan=17, setpoint=25.0℃, reg03=19.5/19.5℃ (과도), reg04 temp_A=25.2℃ op_val_2=252
- **CAP-4** (냉방 정상): reg03=9.0/8.5℃ (정상 증발기 온도), reg04 op_val_1=996 op_val_2=1248
- **CRC 회귀**: 각 캡처 프레임이 CRC-16/ARC (init `0x0000`) 로 검증 성공, init `0xFFFF` 로는 모두 실패함을 확인

### 5.4 LGCNP 와의 코드 재사용

LGCNP 패시브 캡처 구조에서 다음 패턴을 차용하되 Century 도메인으로 재작성한다:

- ring buffer (`recentFrames` slice + atomic seq counter)
- DeviceProvider 어댑터 패턴 (`internal/agent/lg/lgcnp_device.go` 참조)
- `BufferInfoProvider`/`FrameNotifyCh` 구현
- 노드 base 패턴 (`internal/node/lgcnp.go` 의 `lgcnpNodeBase` → `centuryNodeBase`)
- Init-tolerance / deferred connection (`REQ-SERIAL-016`, LGCNP v1.3 패턴): Century 노드 4종은 Init 시점에 에이전트 resolve 실패 시 hard-fail 하지 않고 deferred connection 으로 Running 전이

NASA 패턴에서 차용하는 것:

- 파일 분할 단위 (frame_scanner / crc / protocol / executor / serial_opener / provider 의 명확한 분리)
- `RegisterXXXTypes(mgr)` 호출 위치 (cmd/xflowd/main.go:309 인근 패턴)
- 에이전트 인터페이스 합성 방식 (`agent.Agent` + `MessageReceiver` + `StatefulAgent` + `TransportChecker` + `BufferInfoProvider`)

### 5.5 MaxPayloadLength = 256 의 근거

프레임 스캐너의 `payload_length` 합리성 검증은 `MaxPayloadLength=256` 을 상한으로 사용한다 (REQ-CENTURY-003). CAP-1~CAP-4 에서 관측된 최대 payload 길이는 reg 0x02 응답의 20B 이지만, 256 으로 보수적으로 잡은 이유:

- 펌웨어 버전 차이로 레지스터별 데이터 길이가 확장될 가능성 (예: 향후 reg 0x05 도입 시)
- 16B/32B 정렬 등 합리적 메모리 예산 안에 머무름
- ring buffer 한 칸당 비용은 frame 헤더 10B + payload(최대 256B) + CRC 2B = ~270B 이므로 기본 128 칸 기준 약 34 KB 로 무시 가능

이 값은 단순히 비합리적 헤더(예: payload_length=0xFFFF) 를 거부하기 위한 sanity gate 이며, 실제 디코더는 레지스터별 정확한 길이를 다시 검증한다 (REQ-CENTURY-011 의 단계 5).

### 5.6 WRITE 중복 제거의 cycle 경계 휴리스틱 (REQ-CENTURY-027)

Century 의 한 polling cycle 은 9 프레임으로 구성되며(A2 참조), 두 번 반복되는 WRITE 가 cycle 내부에 위치한다. 새 cycle 의 시작을 판정하는 신호 우선순위:

1. **1차 신호**: 마지막 READ response(reg `0x04` 응답) 또는 ACK 직후를 cycle 시작점으로 간주. 9 프레임 시퀀스의 구조적 마커이므로 가장 신뢰도 높음.
2. **2차 신호 (fallback)**: inter-frame idle 이 `cycle_idle_timeout` 을 초과하면 새 cycle 로 간주. 마스터의 cycle 간 idle 은 약 100ms~수백ms 로 관측됨 (전체 cycle ~511.9ms 중 9 프레임이 차지하는 시간 + idle).

휴리스틱 오작동 시 운영자 진단을 위해 `log_drops=true` 일 때 dedup 된 WRITE frame 도 DEBUG 레벨로 로깅한다(`writesDeduped` 카운터와 함께). 실제 신규 명령이 누락되는 정황이 확인되면 `dedupe_writes=false` 로 fallback 가능.

**v0.2.0 transport-aware default (REQ-CENTURY-032)**: `cycle_idle_timeout` 의 default 는 transport-aware 로 결정된다 — serial=100ms, tcp-*=200ms. TCP 모드에서는 Nagle 알고리즘, 패킷화, 네트워크 jitter 로 inter-frame 간격이 변동할 수 있어 100ms 는 false break(같은 cycle 의 frame 을 다른 cycle 로 잘못 인식하여 dedup 무효화) 위험이 있다. 200ms 도 부족하면 사용자가 명시적으로 더 큰 값을 설정해야 한다. v0.1.2 의 serial 사용자는 기본값 변경 영향을 받지 않으며(여전히 100ms), TCP 사용자에게만 200ms default 가 적용된다.

### 5.7 다중 IDU 테스트 전략 (REQ-CENTURY-013, A7)

현재 검증된 캡처(CAP-1~CAP-4) 는 단일 IDU(`sub_dev_id=0x3B`) 뿐이지만, v0.1.0 의 데이터 모델은 다중 IDU 를 정식 지원한다. 검증은 다음 방식으로 수행:

- **합성 frame 단위 테스트**: CAP-3 의 reg 0x02 응답을 base 로 `sub_dev_id` 만 `0x3C`, `0x3D` 등으로 변경한 합성 frame 을 주입하여, `devices` 맵에 독립 device 가 등록되고 각 device 의 상태가 격리됨을 검증 (B1a/B1b 시나리오).
- **CRC 재계산**: 합성 frame 의 sub_dev_id 변경은 payload 의 일부이므로 CRC 도 재계산해야 함 (테스트 헬퍼 제공).
- **다중 IDU 환경 권장 사항**: 실제 다중 유닛 회선에 배포할 때는 점진적 roll-out — 먼저 `log_decode_errors=true`, `log_unconfirmed_fields=true` 로 운영하여 sub_dev_id 별 frame 패턴을 관측한 뒤, 의심 정황이 없으면 운영 모드로 전환.
- **후속 SPEC**: 실제 다중 IDU 캡처가 확보되면 SPEC-CENTURY-002(또는 본 SPEC v0.2.0) 에서 ground truth 기반 acceptance 추가.

### 5.8 단계별 검증 카운터 (REQ-CENTURY-011, REQ-CENTURY-025)

다층 검증의 각 단계에서 별도 카운터를 증가시켜, 운영 중 회선 품질·프로토콜 deviation 을 진단할 수 있게 한다. 예시:

- `lengthMismatch` 증가 → 프레임 스캐너가 끊긴 바이트 스트림을 보고 있음. 트랜스포트 layer 점검 필요
- `crcMismatch` 비율 ≥ 0.3% → 회선 노이즈 또는 다른 프로토콜이 같은 회선에 섞여 있을 가능성
- `payloadPrefixInvalid` 증가 → `sub_dev_id` 가 설정값과 다름. 다중 unit 환경 가능성
- `registerLengthInvalid` 증가 → 펌웨어 버전 차이로 데이터 길이가 다른 변종 존재 가능

### 5.9 M5 Closure Notes — M1-M4 Open Questions Disposition

각 마일스톤에서 노출된 미해결 사항의 v0.1.2 시점 최종 처리:

**M1 (Foundation) 관련**:

- *ScannerStats.String() retention*: M1 에서 임시 헬퍼로 노출된 `ScannerStats.String()` 은 운영 도구·로그 포맷에 직접 의존하지 않으므로 v0.1.2 에서는 유지. v0.2.0 에서 metrics export 시스템(예: Prometheus exporter) 도입 시 필드 단위 노출로 마이그레이션 예정. 현재는 `get_stats` 커맨드 응답이 동일 정보를 JSON 으로 제공하므로 외부 호환성 영향 없음.
- *FrameScanner.ReadTimeout 적용 범위*: M1 에서 `ReadTimeout` 은 헤더-payload 읽기 사이의 in-frame gap 에는 명시적으로 적용되지 않고 호출자(`io.Reader`)의 deadline 에 의존한다. 실측 환경(폴링 주기 511.9ms, in-frame gap < 50ms)에서는 충돌 사례가 보고되지 않았으므로 v0.1.2 에서 정책 변경 없이 유지. 회선 노이즈가 심한 환경에서는 트랜스포트 layer 의 read deadline 으로 충분.

**M3 (Agent + Devices) 관련**:

- *provider.go 마일스톤 재배치*: 원래 계획상 M3 의 deliverable 이었으나 실제 구현 흐름에서 디코더 dispatch 와 디바이스 상태 매핑이 강결합되어 M3 (`bad2e06`) 에 완성되었다 — 별도 마일스톤 재배치는 v0.1.2 에서는 archival 사항으로만 기록.

**M4 (Nodes + Web UI) 관련**:

- *registry_test.go gofmt noise*: M4 에서 발견된 사소한 gofmt drift 는 commit `3f1b970` 시점에 정리 완료. M5 에서 `gofmt -l ./internal/node/registry_test.go` 재확인 결과 추가 변경 없음.
- *CenturyRawFrameNode 의 `crc_ok` 항상 true*: 현재 `CenturyRawFrameNode` 는 에이전트의 **검증된 ring buffer** 를 통해 frame 을 수신하므로 CRC 가 검증된 프레임만 노출되며 `crc_ok` 가 항상 true 이다. CRC 실패 프레임의 raw 노출은 v0.1.2 의 범위를 벗어나며, **알려진 한계** 로 명시한다 (아래 "Known Limitations" 참조).
- *pollSingle 0% coverage*: M4 에서 보고된 `pollSingle` 미커버 분기는 M5 의 coverage 측정에서 `internal/node/century.go:359 pollSingle 83.3%` 로 확인됨 — 후속 테스트(`TestCenturyStatusNode_PollSingleAdjacency`)가 M4 후반에 추가되어 자연 해소되었다.
- *TS schema tests 부재*: web 측 `centurySchema.test.ts` 는 M4 에서 추가되어 D5/D6 시나리오를 커버한다. 별도 미해결 사항 없음.

**Known Limitations (v0.2.0 deferral)**:

다음 항목은 v0.1.2 의 범위를 의도적으로 벗어나며, 후속 SPEC 에서 다룬다:

1. **CRC-failed raw frame surface**: `CenturyRawFrameNode` 가 CRC 검증 실패 프레임의 raw 바이트를 노출하지 않는다. 현재 구조는 검증된 ring buffer 만 사용하지만, 디버깅 시나리오(역공학용 캡처 수집) 에서는 invalid 프레임도 보고 싶을 수 있다. v0.2.0 에서 별도 `invalid_frames` ring buffer 또는 별도 채널 도입 검토.
2. **능동 폴링 / 송신 모드**: 본 SPEC 은 패시브 전용. 능동 폴링·제어 명령 송신은 SPEC-CENTURY-002 (가칭 "Active polling and control") 로 분리하며, 본 SPEC 의 호환을 깨지 않는다.
3. **실제 다중 IDU ground truth**: 데이터 모델·자동 발견 로직은 다중 IDU 를 지원하나, 검증된 캡처는 단일 유닛(`sub_dev_id=0x3B`)뿐. 실제 다중 unit 회선 캡처가 확보되면 v0.2.0 에서 ground truth acceptance 추가.
4. **미확정 필드 의미 발굴**: `status_bits` 의 개별 비트 매핑, `op_val_1`/`op_val_2` 의 단위(주파수/소비전력/적산), `write_byte_14`/`write_live_15` 의 의미는 `confirmation_status=inferred`/`unknown` 으로 노출만 한다. v0.2.0+ 에서 추가 캡처로 의미 확정 시 `confirmed` 로 진화하며, REQ-CENTURY-026 의 additive 정책에 따라 기존 필드명은 deprecated alias 로 유지.
5. **모드 코드 `0x02` 이상**: 난방/제습/송풍 등 미관측 모드 코드는 `mode_unknown_<hex>` 로 디코딩되며, 후속 캡처 확보 시 additive enum 으로 확장.
6. **Metrics export**: 현재 `get_stats` 커맨드가 JSON 응답으로 atomic 카운터를 노출하나, Prometheus / OpenTelemetry 등 메트릭 시스템 직접 연계는 v0.2.0+ 에서 진행.

**v0.2.0 (M6) 진입 시점 추가 disposition**: TCP transport 확장 (이 SPEC) 은 transport 차원 확장이며 active mode 와 무관하다. 위 deferral 항목 중 항목 2 ("능동 폴링 / 송신 모드") 는 여전히 SPEC-CENTURY-002 (가칭 "Active polling and control") 로 별도 분리한다. v0.2.0 의 TCP transport 추가는 본 SPEC v0.1.2 의 모든 패시브 캡처 보장(AC-B9 transport.Write 0회 불변식 포함) 을 유지한 채 transport 옵션만 확장한다.

### 5.10 TCP transport 구현 노트 (v0.2.0, REQ-CENTURY-028~031)

**TCP-client 구조**:

- 연결 수립: `net.DialTimeout("tcp", net.JoinHostPort(host, port), tcp_connect_timeout)`. dial 도중 context cancel(`agent.Stop`) 시 즉시 중단되도록 dial 호출을 별도 goroutine 으로 보내거나, `Dialer.DialContext` 사용 권장 (lgcnp `lgapTCPClientTransport` 의 단순 `DialTimeout` 패턴을 따르되 context 대응을 추가).
- Read deadline: 매 read 직전 `conn.SetReadDeadline(time.Now().Add(tcp_read_timeout))` 갱신. timeout 발생 시 `net.Error.Timeout()` 으로 감지.
- Write 경로 없음: wrapper 는 `io.Reader + io.Closer` 만 노출. `io.ReadWriteCloser` 인터페이스 호환을 위해 Write 메서드를 둘 경우 `ErrTransportPassiveOnly` 반환하여 AC-B9 회귀 방지.

**TCP-server 구조**:

- Listener: `net.Listen("tcp", host:port)`. listener 는 `agent.Stop` 호출까지 유지.
- Accept loop: 단일 활성 연결 정책 (A11). 활성 연결 보유 중 두 번째 `Accept` 결과는 즉시 close + INFO 로그.
- 활성 연결의 EOF/timeout 은 listener 와 독립이며, 종료 시 accept loop 로 복귀 (backoff 없음).

**재연결 backoff (TCP-client 만, REQ-CENTURY-031)**:

- Exponential: `next = min(prev * 2, max_reconnect_backoff)`, 초기 `reconnect_initial`.
- backoff sleep 은 `select { case <-time.After(d): case <-ctx.Done(): return }` 으로 cancel-aware.
- 재연결 성공 시 timer 리셋. 실패 누적 횟수와 현재 backoff 를 stats 또는 DEBUG 로그로 노출 (운영자 진단용).

**Transport abstraction**:

- `agent.go` 의 `transportProvider func() (io.ReadWriteCloser, error)` 가 이미 transport-agnostic 이다.
- 신규 `transport_tcp.go` 모듈에서 tcp-client / tcp-server 구현.
- 기존 inline serial 코드는 `transport_serial.go` 로 분리 (refactor) — M3 의 inline 로직과 동일 행동, 파일만 분리.
- frame scanner 는 transport 와 무관: 이미 `io.Reader` 기반이라 변경 불필요.

**AC-B9 불변식 (transport.Write 0회) 유지**:

- TCP wrapper 는 `io.Reader` + `io.Closer` 의 합으로 노출. Write 메서드를 두지 않는다.
- 단일 활성 연결 정책(TCP-server) 도 RX-only — accept 된 `net.Conn` 에 절대 송신하지 않는다.
- 단위 테스트에서 mock listener / mock dial 결과로 wrapper 의 Write 호출이 0회임을 검증 (AC-G8).

### 5.11 DeviceStateEvent emit 로직 (v0.3.0, REQ-CENTURY-033/034/035)

**Change detector 알고리즘**:

1. captureLoop 가 frame decode 성공 후 `device.Update(decoded, now)` 를 호출하여 register 별 슬롯을 갱신한다 (기존 로직).
2. `emit_device_state=true` 이면 다음을 수행:
   - `snapshot := device.Snapshot()` (lock-free 사본)
   - `current := buildDeviceStateEventFields(snapshot)` — 5개 핵심 (`power`, `mode`, `fan`, `set_temp_c`, `current_temp_c`) + online + 증발기 a/b
   - `emitMu.Lock()` 으로 동기화한 뒤 `lastEmitState[subDevID]` 와 5 핵심 + online 비교
   - 차이가 있으면 `trigger="change"` 로 emit, `lastEmitState[subDevID] = current`, `lastEmitTime[subDevID] = now`
   - 차이가 없으면 emit 하지 않음 (keepalive ticker 에 위임)
3. `emit_register_decoded=true` 이면 v0.2.x 와 동일하게 decoded 메시지 emit (interleaved).

**Keepalive ticker goroutine** (별도 goroutine, 1초 주기):

1. `time.NewTicker(1 * time.Second)` 로 깨어남 (1초 jitter 허용 — fine-grained 한 keepalive 가 필요한 사용 사례 없음)
2. 각 device 의 `now - lastEmitTime[id] >= keepalive_interval` 검사
3. expired 시 `emitMu.Lock()` 으로 동기화한 뒤 현재 snapshot 으로 `trigger="keepalive"` emit + `lastEmitTime[id] = now` (lastEmitState 는 그대로 유지)
4. `keepalive_interval == 0` 이면 ticker 시작하지 않음 (change-only 모드)
5. `keepaliveStopCh` close 또는 ctx.Done() 시 종료

**Online 전이 hook**:

- 기존 offlineWatchLoop 에서 device 가 online=false 로 전이될 때 즉시 change detector path 를 호출하여 `trigger="change"` emit (`online: false` 포함된 snapshot).
- 다음 frame 수신 시 device.Touch() 가 online=true 로 복귀하면, captureLoop 의 change detector 가 자동으로 다시 `trigger="change"` 발생.

**Power 정의 (A15)**:

- `power = (snapshot.State.Reg02 != nil && snapshot.State.Reg02.Mode.Value != ModeOff)` 또는 동등한 단순화.
- mode 가 한 번도 수신되지 않은 device (reg 0x02 nil) 는 `power=false` 로 default emit.

**Mutex 설계**:

- `lastEmitState` / `lastEmitTime` 만 보호하면 충분 (`device.mu` 와 별개의 mutex).
- emit 채널 (msgCh) 송신은 mutex 밖에서 수행하여 blocking 회피 (기존 captureLoop 의 best-effort drop 정책 유지).

**미수신 register 처리**:

- `current_temp_c`: `snapshot.State.Reg04Read == nil` 이면 0.0 emit (구현 통일성).
- `evap_temp_a_c` / `evap_temp_b_c`: `snapshot.State.Reg03 == nil` 이면 0.0 emit.
- 향후 null 마커가 필요해지면 v0.4.0 에서 enum 표현 추가 검토.

### 5.12 Migration guide (v0.2.x → v0.3.0)

**Breaking change 요약**: agent 의 msgCh 1차 출력이 register-decoded 다중 메시지 → 단일 device-centric `DeviceStateEvent` 로 변경.

**Migration 시나리오 1: v0.2.x register-decoded 를 그대로 유지**:

```yaml
# v0.2.x 호환 (register-decoded 만 emit, device_state 비활성)
options:
  emit_device_state: false
  emit_register_decoded: true
  # 또는 둘 다 활성화하여 점진 마이그레이션
```

**Migration 시나리오 2: v0.3.0 device_state 만 사용 (권장)**:

```yaml
# v0.3.0 default — 명시 불필요하지만 가독성을 위해 명시 권장
options:
  emit_device_state: true
  emit_register_decoded: false
  keepalive_interval: 60s
```

Downstream flow node 는 `type=="device_state"` 로 분기. 추정 필드 (`status_bits` 비트, `op_val_1/2` 등) 가 필요하면 `emit_register_decoded: true` 로 두 stream 모두 활성화.

**Migration 시나리오 3: 두 stream 모두 활성 (점진 마이그레이션)**:

```yaml
options:
  emit_device_state: true
  emit_register_decoded: true
```

기존 register-decoded 소비자가 점진적으로 device_state 로 마이그레이션할 때 사용. 같은 frame 이 두 메시지 모두 트리거할 수 있으므로 downstream 은 `type` 필드로 분기 필수.

**확인 사항**:

- v0.2.x 운영자가 `/moai:2-run` 으로 v0.3.0 으로 업그레이드 후 register-decoded 메시지가 사라지면, 위 시나리오 1 또는 3 으로 config 보강 필요.
- 두 옵션 모두 false 로 설정하면 `parseCenturyConfig` 가 `ErrCenturyNoOutputEnabled` 반환 (의도된 fail-fast).

---

*SPEC 버전: 0.3.0 (Draft)*
*이전 버전: 0.2.0 (Draft, M6 미구현)*
*초안 작성일: 2026-05-18 (v0.1.0), 갱신: 2026-05-18 (v0.1.1 — 다중 IDU + dedupe_writes), 2026-05-18 (v0.1.2 — M1-M5 구현 완료, Implemented 상태 전이), 2026-05-18 (v0.2.0 — TCP transport, M6 신설, Draft), 2026-05-19 (v0.3.0 — Breaking: device-centric output default, M7 신설)*
*작성자: xtra*
*프로토콜 ground truth: references/protocols/century_hvac_protocol_spec.md v0.3 (CAP-1 ~ CAP-4)*
*구현 commit 체인 (v0.1.2 까지): 14ee853 → bfdfaf0 → d33da37 → bad2e06 → 3f1b970 → [M5]*
*v0.2.0 M6 commit: 미정 (M6 구현 시 갱신)*
*v0.3.0 M7 commit: 미정 (M7 구현 시 갱신)*
