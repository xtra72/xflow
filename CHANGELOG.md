# Changelog

이 프로젝트의 주요 변경 사항을 기록한다.
형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르며,
[Semantic Versioning](https://semver.org/lang/ko/)을 적용한다.

## [Unreleased]

### 변경 (BREAKING) — `lgcp` 식별자 rename 으로 LG ICP-02 프로토콜 / LG HVACR-02 에이전트 분리

- **`lgcp` 식별자를 protocol / agent / node 3 가지 역할별로 분리 (Breaking)**

  기존 `lgcp` 단일 식별자가 protocol code, agent type, node type 3 가지 의미로 동시에 쓰이던 모호함을 해소. 동일 패턴의 lgcnp → lg_icp01/lg_hvacr01 (v1.x 이전 적용) 의 후속 작업.

  - **프로토콜 코드**: `lgcp` → `lg_icp02` (LG ICP-02 wire protocol)
  - **에이전트 타입**: `lgcp` → `lg_hvacr02` (LG HVACR-02 agent)
  - **노드 타입**: `lgcp` / `lgcp-status` / `lgcp-control` → `lg_hvacr02` / `lg_hvacr02_status` / `lg_hvacr02_control`
  - **복합 디바이스 ID**: `lgcp:<addr>` → `lg_icp02:<addr>`
  - **SPEC 디렉토리**: `SPEC-LGCP-001/002/003` → `SPEC-LG-HVACR-002-001/002/003`
  - **프로토콜 분석 문서**: `references/protocols/LGCP_Protocol_Analysis.md` → `LG-ICP-02_Protocol_Analysis.md`
  - **Go identifiers (B-3 convention)**: Agent side `LGCP*` → `Hvacr02*`, Protocol side `LGCP*` → `Icp02*`, Node side (vendor prefix) `LGCP*Node` → `LGHvacr02*Node`, Adapter `LGCP*` → `LGIcp02*`
  - 기존 yaml / flow 가 deprecated alias 를 사용했다면 부팅 실패 (parse error). 운영자 마이그레이션: examples 폴더의 새 형식 파일 참조.
  - 관련 commits: backend (bc700c9), frontend (b803ffe).

### 수정 (BREAKING) — Century reg 0x02 setpoint byte 위치 정정 (실측 검증)

- **Century ICP-01 프로토콜 spec 의 setpoint byte 위치 정정 (Breaking — emit 값 변경)**

  사용자 AC remote 6-point 실험 (18 / 20 / 22 / 24 / 26 / 28°C 순차 설정) 결과 setpoint 의 부호화 위치가 `data[7..8]` 이 아닌 **`data[11..12]`** 임이 실측으로 확정. 관측값 `data[11..12] ÷ 10` = 180/200/220/240/260/280 → 18~28°C 와 **완벽 linear 일치**.

  - **byte 위치 swap**: `setpoint_c` 의 source 가 `data[7..8] LE u16 ÷ 10` 에서 `data[11..12] LE u16 ÷ 10` 으로 변경. `data[7..8]` 은 별개 운전 파라미터로 재분류되어 새 필드 `reg02_word_7` (inferred) 로 노출 — ≤25°C 설정 시 250 고정, 26°C → 245, 28°C → 240 으로 5 씩 감소 (cooling capacity ceiling / max compressor speed 등 추정).
  - **이전 가정 미검증의 원인**: CAP-3/4 fixture 가 우연히 두 byte 쌍 모두 `0x00FA` = 250 (25.0°C) 이라 디코더 byte position 이 잘못되어도 fixture 테스트가 통과해왔음. 단일 setpoint 만 가진 fixture 의 검증 한계.
  - **CAP-1 (꺼짐) 재해석**: 이전 spec 의 "꺼짐 상태에서도 setpoint 25°C 유지" 관찰은 실제로 `data[7..8]` (현 `reg02_word_7`) 이 유지된 것이며 setpoint 그 자체가 아님. 꺼짐 상태에서 `data[11..12]=0` (active cooling target 없음) 이 자연스러운 해석.
  - **다운스트림 영향**: Go 필드 `Reg02Word11` + JSON key `reg02_word_11` → `Reg02Word7` / `reg02_word_7` 로 rename. `setpoint_c` 필드 이름은 유지. emit 메시지의 `target_temperature` 값이 이제 사용자 실제 설정과 일치 (이전엔 잘못된 byte 로 인해 일치하지 않을 수 있었음).
  - **회귀 위험**: CAP-3/4 fixture 테스트는 두 byte 쌍 모두 250 이라 swap 후에도 통과. CAP-1 (꺼짐) 테스트 어서션은 갱신 필요 (이미 적용). 25°C 단일 설정으로만 운영해왔다면 사용자 영향 없음, 다양한 setpoint 사용 시 이제 정확한 값 표출.
  - **관련**: `references/protocols/century_icp01_protocol_spec.md` v0.4, SPEC-CENTURY-HVACR-001 v0.20.0.

### 변경 (BREAKING) — Samsung HVACR-01 transport_type tcp-client/tcp-server 분리 지원

- **Samsung HVACR-01 의 `transport_type` 옵션이 LG / Century 와 동일한 3-모드 (`serial` / `tcp-client` / `tcp-server`) 로 통일 (Breaking)**

  이전엔 Samsung 만 `serial` / `tcp` 2-모드만 지원하여 TCP 서버 모드 (시리얼-Ethernet 컨버터의 push 연결) 가 불가능했다. LG / Century 와 동일한 패턴으로 `NasaTCPServerTransport` 를 추가하고 `transport_type` validation 을 확장한다. 기존 `tcp` 값은 더 이상 인식되지 않고 parse error 로 거부되며, 사용자는 `tcp-client` 로 명시적으로 마이그레이션해야 한다.

  - **신규**: `NasaTCPServerTransport` — LG `lgapTCPServerTransport` 패턴을 따르며, bind 주소 (`tcp_host`, 기본 `0.0.0.0`) 에서 단일 활성 연결 정책으로 동작한다. 새 연결이 들어오면 기존 활성 연결을 close 하고 교체한다.
  - **검증 변경**:
    - `transport_type`: `serial` / `tcp-client` / `tcp-server` 만 허용. `tcp` 입력 시 parse error.
    - `tcp_host`: `tcp-server` 는 기본값 `0.0.0.0` (모든 인터페이스), `tcp-client` 는 필수 (서버 IP 명시 필요).
    - `tcp_port`: 두 TCP 모드 모두 필수.
  - **마이그레이션**:
    - 기존 `transport_type: "tcp"` 설정은 `transport_type: "tcp-client"` 로 변경.
    - 예제 파일 rename: `examples/agents/samsung_hvacr01-tcp.yaml` → `samsung_hvacr01-tcp-client.yaml`.
    - 프론트엔드 schema (agentSchemas.ts, agentTypeMeta.ts) 도 3-모드 옵션 노출.

### 변경 (BREAKING) — 3종 HVACR-01 에이전트 (LG / Samsung / Century) config 필드·기본값·로그 옵션 통일 (LG 명세 기준)

- **3종 HVACR-01 에이전트 (LG / Samsung / Century) 의 에이전트 config 필드, 기본값, 로그 옵션을 LG 명세 기준으로 통일 (Breaking)**

  세 에이전트가 서로 다른 필드명·alias·기본값·로그 옵션을 사용하던 비대칭을 제거하고, 운영자가 어느 벤더 에이전트를 사용하더라도 동일한 멘탈 모델로 동작을 예측할 수 있도록 정렬한다. 본 변경은 backend 가 alias 를 silent accept 하지 않고 **명시적 parse error 로 거부**하므로, 부팅 즉시 실패 (fail loud) 한다.

  - **기본값 통일**:
    - Samsung `report_interval`: `0` → `"60s"` (기본 keepalive 활성화. 이전엔 변경 감지만 동작)
    - Samsung `auto_discovery`: `false` → `true` (LG / Century 와 동일하게 자동 탐색을 기본 활성)
    - Century `offline_timeout`: `"5s"` → `"30s"` (LG / Samsung 과 동일하게 30s 로 통일. 폴링 cycle 의 약 60배)

  - **필드 rename (alias 미수용, breaking)**:
    - Century `reconnect_initial` → `reconnect_interval` (Samsung 의 동명 필드와 정렬)
    - Samsung `include_raw_message_sets` → `include_raw_hex` (LG / Century 의 동명 필드와 정렬. 의미는 동일 — register-decoded / state response 에 원시 바이트 hex 포함 여부)
    - 3개 에이전트 모두: `notify_interval` deprecation alias **완전 제거** (이전엔 v1.6.0 / v0.6.0 부터 `report_interval` 로 통일하면서 alias 만 유지). 이제 `notify_interval` 키는 parse error.
    - Century: `keepalive_interval` / `keepalive_mode` **완전 제거** (이전 v0.3.x 의 device_state fallback emit 옵션). `report_interval` / `report_mode` 만 인식되며, 의미·동작 (relative / absolute crontab 패턴) 은 보존된다.

  - **로그 옵션 상향 통일 — 3개 에이전트 모두 동일 keys 노출**:
    - LG 신규 추가: `log_decode_errors`, `log_drops`, `log_state_updates`
    - Samsung 신규 추가: `log_drops`, `log_state_updates` (`log_decode_errors` 는 v1.9.0 부터 보유)
    - Century: 변경 없음 (이미 3개 모두 보유 — `log_decode_errors`, `log_drops`, `log_state_updates`)

  - **backend 거부 동작 (breaking — fail loud)**:
    - 다음 키가 config 에 존재하면 에이전트 init 시점에 parse error 로 즉시 부팅 실패: `notify_interval`, `include_raw_message_sets`, `reconnect_initial`, `keepalive_interval`, `keepalive_mode`.
    - 이전 v1.6.0 / v0.6.0 의 silent accept 방식이 운영자가 deprecation 사실을 인지하지 못한 채 alias 를 누적하던 문제 (구버전 yaml 이 작동하는 것처럼 보이지만 default 값이 적용됨) 를 해소한다.

  **운영자 마이그레이션**:
  - greenfield 환경: 별도 조치 불필요.
  - brownfield 환경: yaml 의 deprecated 필드를 신규 필드로 일괄 치환 후 부팅. 자세한 절차는 `docs/migration/hvacr-config-unification.md` 참조.
    - `notify_interval` → `report_interval`
    - `keepalive_interval` → `report_interval`
    - `keepalive_mode` → `report_mode`
    - `reconnect_initial` → `reconnect_interval`
    - `include_raw_message_sets` → `include_raw_hex`
    - Samsung `auto_discovery: true` 를 명시했던 기존 yaml: 생략 가능 (default 가 true)
    - Samsung `report_interval` 미설정 환경: 60s keepalive emit 이 시작됨. 변경 감지만 원하는 경우 `report_interval: "0s"` 명시.

### Removed

- **3종 HVACR-01 에이전트 (LG / Samsung / Century) deprecated config alias 5종 완전 제거 (breaking)** — `notify_interval`, `keepalive_interval`, `keepalive_mode`, `reconnect_initial`, `include_raw_message_sets`. config 에 존재 시 silent accept 되지 않고 parse error 로 거부된다. 이전엔 v1.6.0 / v0.6.0 부터 deprecation alias 로 일부만 수용되었으나, 본 변경에서 backend 가 명시적으로 거부하도록 통일했다.
- **LGAP / LGCP 에이전트의 `notify_interval` alias 완전 제거 (breaking)** — HVACR-01 3종에 이은 후속 정리. 두 에이전트가 마지막까지 `notify_interval` deprecation alias 를 silent accept 하던 비대칭을 해소. config 에 `notify_interval` 키가 존재하면 parse error 로 거부되며, `report_interval` 만 허용된다. 프론트엔드 schema 의 alias 안내 텍스트 (`이전 notify_interval, deprecation alias 유지`, `v0.6.0 통합 옵션`) 도 함께 제거.

### 변경 (BREAKING) — status 노드 3종 통일 (LG inactivity 모델) + 어드레싱 + 메타데이터 정리

- **`*_hvacr01_status` 노드 3종 (LG / Samsung / Century) config 구조를 LG inactivity 모델로 통일 (Breaking)**

  세 가지 status 노드가 서로 다른 모델 (LG = inactivity-fallback, Samsung/Century = ticker 기반 polling) 을 사용하던 비대칭을 제거하고, LG ICP-01 의 inactivity-fallback 모델을 표준으로 채택해 통일한다. 신규 어드레싱 필드 (`group_id`, `unit_id`) 를 도입하고, 의미가 모호하던 출력 metadata (`unit_id`, `slot_num`) 는 제거한다.

  - **동작 통일 — inactivity-fallback 모델**:
    - 노드는 에이전트의 `FrameNotifyCh` 신호를 수신하면서 frame 도착 시 즉시 처리한다.
    - `inactivity_timeout` (기본 `"90s"`) 동안 frame 신호가 수신되지 않으면 에이전트에 `request_state` 명령을 전송한다 (회선 silent 상태에서도 주기적 상태 확보).
    - 어드레싱 필드가 설정된 경우 매칭 frame 만 emit + `request_state` 의 target 으로 사용.

  - **신규 어드레싱 필드 (3종 status + 3종 combined 노드, advanced)**:
    - `unit_id` (string, hex): 프로토콜 디바이스 식별자.
      - LG: STX byte (`"58"` ODU, `"81"`–`"BF"` IDU, 64 units)
      - Samsung: NASA addr byte 2 (`"00"`–`"3F"` indoor; outdoor 는 group_id 와 동일)
      - Century: `sub_dev_id` (`"3B"` 등)
    - `group_id` (string, hex, Samsung 전용): NASA addr byte 1 / 외기 인덱스 (`"00"`–`"0F"`). LG / Century 는 schema parity 위해 필드 유지하나 미사용.
    - 두 필드 모두 빈 값일 때 모든 디바이스 처리 / broadcast `request_state`.

  - **Samsung status / combined 노드 변경**:
    - 제거: `device_id` (input config), `poll_command`, `poll_interval`, `device_address`
    - 추가: `inactivity_timeout`, `group_id`, `unit_id`
    - 단일 디바이스 조회는 노드 input 메시지 payload 의 `device_id` / `unit_id` override 로 가능 (제어 노드 / 통합 노드의 payload-level 지정은 유지).

  - **Century status / combined 노드 변경**:
    - 제거: `poll_command`, `poll_interval`, `recent_count`
    - 추가: `inactivity_timeout`, `group_id` (미사용), `unit_id`
    - 유지: `emit_raw_frames` (직전 raw-frame 통합 옵션)

  - **LG status / combined 노드 변경 (additive)**:
    - 추가: `group_id` (미사용, schema parity), `unit_id` (선택적 STX 필터)
    - 기존 `poll_interval` / `poll_command` / `recent_count` 는 deprecation alias 로 계속 수용 (no-op). LG 는 이미 inactivity 모델 — 동작 변경 없음.

- **출력 메시지 metadata 정리 — `unit_id` / `slot_num` 제거 (Breaking, 전 노드)**

  출력 metadata 의 `unit_id` 와 `slot_num` 은 프로토콜 해석 단계에서만 의미가 있는 내부 표현 (LGCNP `"0"`/`"1"`–`"5"`, NASA addr 분해 등) 으로, downstream consumer 가 알 필요가 없는 artifact 였다. 동일 정보가 필요한 경우 `metadata.device_id` (UUID) → DeviceRegistry 조회 또는 노드의 어드레싱 필드 (`unit_id`, `group_id`) 로 일대일 대응 가능하다.

  - `MetadataEmitOptions.UnitID` / `MetadataEmitOptions.SlotNum` 필드 제거
  - 노드 config 의 `emit_unit_id` / `emit_slot_num` 옵션 제거 (전 노드 — LG / Samsung / Century status·control·combined)
  - `dedup_helper` 의 `promoteDevIDToMetadata` / `promoteDevIDWithUUID` 에서 `unit_id` metadata emit 경로 삭제. payload 의 `unit_id` 는 항상 삭제되며 metadata 에는 노출되지 않는다.
  - 다운스트림 마이그레이션: `$.metadata.unit_id` / `$.metadata.slot_num` 참조 제거. 대신 `$.metadata.device_id` (UUID) 사용.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경:
    - Samsung flow yaml 의 status 노드 config 에서 `device_id` / `poll_interval` / `poll_command` / `device_address` 필드 제거 (필요 시 payload-level override 로 대체).
    - Century flow yaml 의 status 노드 config 에서 `poll_interval` / `poll_command` / `recent_count` 필드 제거. `emit_raw_frames` 는 유지.
    - 어드레싱이 필요한 경우 (단일 디바이스 만 처리) `unit_id` (Samsung 은 `group_id` 도) 를 advanced 필드로 설정.
    - downstream 의 `metadata.unit_id` / `metadata.slot_num` 필터 / 조인 키를 `metadata.device_id` 로 마이그레이션.

### 변경 (BREAKING) — `century-hvac` 식별자 rename 으로 Century ICP-01 프로토콜 / Century HVACR-01 에이전트 분리

- **Century `century-hvac` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `century-hvac` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다 (`lgcnp` / `samsung-nasa` rename 과 동일 패턴).

  - **프로토콜 코드**: `century-hvac` → `century_icp01` (Century ICP-01 와이어 프로토콜)
  - **에이전트 타입**: `century-hvac` → `century_hvacr01` (Century HVACR-01 에이전트)
  - **노드 타입**: `century` / `century-status` / `century-control` → `century_hvacr01` / `century_hvacr01_status` / `century_hvacr01_control`
  - Composite device ID 예: `century:3b` → `century_icp01:3b` (legacy ID 는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-CENTURY-001` → `SPEC-CENTURY-HVACR-001`
  - 프로토콜 분석 문서: `references/protocols/century_hvac_protocol_spec.md` → `references/protocols/century_icp01_protocol_spec.md`
  - 예제 에이전트: `examples/agents/century-hvac*.yaml` → `examples/agents/century_hvacr01*.yaml`, 예제 플로우: `examples/flows/century-status-flow.yaml` → `examples/flows/century_hvacr01-status-flow.yaml`
  - Backend (`internal/agent/century/`, `internal/node/century_hvacr01.go`, 노드 레지스트리) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

### 제거 (BREAKING) — `century-raw-frame` 노드 통합

- `century-raw-frame` 노드 타입이 제거되었다. 회선상 관측된 모든 raw frame (CRC 불일치 / payload prefix 위반 프레임 포함) 의 비파괴 emit 은 `century_hvacr01_status` 노드의 `emit_raw_frames: true` 옵션으로 흡수되었다 (ring buffer drain + raw frame 메시지 emit, dedupe_writes 와 무관). 동일한 raw frame payload schema 가 status 노드의 `out` 포트로 emit 되며, decoded 메시지 (`type=="century_reg02_response"` 등) 와 raw frame 메시지 (`type=="century_raw_frame"`) 는 `type` 필드로 구분한다. SPEC-CENTURY-HVACR-001 의 REQ-CENTURY-019 는 추적성 보존을 위해 REMOVED / CONSOLIDATED 노트로 유지된다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `century:XX` 또는 `century/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `century`, `century-status`, `century-control` 노드 타입 또는 `century-hvac` 에이전트 타입을 직접 참조하는 경우 `century_hvacr01`, `century_hvacr01_status`, `century_hvacr01_control` 로 갱신 필요. `century-raw-frame` 노드를 사용하던 flow 는 `century_hvacr01_status` + `emit_raw_frames: true` 옵션 조합으로 마이그레이션 필요.

### 변경 (BREAKING) — `nasa` / `samsung-nasa` 식별자 rename 으로 Samsung NASA 프로토콜 / Samsung HVACR-01 에이전트 분리

- **Samsung `nasa` / `samsung-nasa` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `nasa` / `samsung-nasa` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다.

  - **프로토콜 코드**: `nasa` → `samsung_nasa` (Samsung NASA 와이어 프로토콜)
  - **에이전트 타입**: `samsung-nasa` → `samsung_hvacr01` (Samsung HVACR-01 에이전트)
  - **노드 타입**: `nasa` / `nasa-status` / `nasa-control` → `samsung_hvacr01` / `samsung_hvacr01_status` / `samsung_hvacr01_control`
  - Composite device ID 예: `nasa:0x12` → `samsung_nasa:0x12` (legacy ID 는 `internal/migrate/tsdbtags` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-NASA-001` → `SPEC-SAMSUNG-HVACR-001`
  - 예제 플로우: `examples/flows/nasa-*.yaml` → `examples/flows/samsung_hvacr01-*.yaml`, 예제 에이전트: `examples/agents/samsung-nasa-*.yaml` → `examples/agents/samsung_hvacr01-*.yaml`, 예제 스크립트: `examples/scripts/nasa-*.xflow` → `examples/scripts/samsung_hvacr01-*.xflow`
  - LG 노드 Go 타입은 본 rename 의 선행 작업으로 `internal/node/lg_hvacr01.go` 에서 `LG` prefix 적용 완료 (Samsung 노드 타입과 충돌 회피).
  - Backend (`internal/agent/samsung/`, `internal/node/adapter/samsung_nasa.go`, 노드 레지스트리) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `nasa:XX` 또는 `nasa/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `nasa`, `nasa-status`, `nasa-control` 노드 타입 또는 `samsung-nasa` 에이전트 타입을 직접 참조하는 경우 `samsung_hvacr01`, `samsung_hvacr01_status`, `samsung_hvacr01_control` 로 갱신 필요.

### 변경 (BREAKING) — `lgcnp` 식별자 rename 으로 LG ICP-01 프로토콜 / LG HVACR-01 에이전트 분리

- **LG `lgcnp` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `lgcnp` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다.

  - **프로토콜 코드**: `lgcnp` → `lg_icp01` (LG ICP-01 와이어 프로토콜)
  - **에이전트 타입**: `lgcnp` → `lg_hvacr01` (LG HVACR-01 에이전트)
  - **노드 타입**: `lgcnp` / `lgcnp-status` / `lgcnp-control` → `lg_hvacr01` / `lg_hvacr01_status` / `lg_hvacr01_control`
  - Composite device ID 예: `lgcnp:81` → `lg_icp01:81` (legacy ID 는 `internal/migrate/tsdbtags` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-LGCNP-001` → `SPEC-LG-HVACR-001`
  - 프로토콜 분석 문서: `references/protocols/LGCNP-01_Protocol_Analysis.md` → `references/protocols/LG-ICP-01_Protocol_Analysis.md`
  - Backend (`internal/agent/lg/lgcnp_*.go` → `lg_hvacr01_*.go` / `lg_icp01_*.go`, `internal/node/lgcnp.go` → `lg_hvacr01.go`) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `lgcnp:NN` 또는 `lgcnp/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `lgcnp`, `lgcnp-status`, `lgcnp-control` 노드 타입을 직접 참조하는 경우 `lg_hvacr01`, `lg_hvacr01_status`, `lg_hvacr01_control` 로 갱신 필요.

### 변경 (BREAKING) — xflowd v1.0 진입 준비

- **SPEC-DEVICE-IDENTITY-001 Phase D — xflowd v1.0 메이저 (Breaking)**

  v0.x 의 composite 디바이스 식별자 (`agent_name:local_id` — 예: `lgcnp:81`) 가 완전히 제거되고, UUID v4 가 유일한 글로벌 식별자가 된다. 본 변경은 greenfield 환경 가정 (외부 클라이언트 부재) 하에 호환 alias 비용 없이 메이저 릴리스로 진행한다. brownfield 환경은 사전 마이그레이션 (`xflowd migrate device-ids`) 이 선결 조건이다.

  **핵심 Breaking 변경**:

  - **`Device.ID()` 시맨틱 변경**: composite key 대신 UUID v4 (`UID()` 와 동일 값) 반환. 5 어댑터 (NASA/LGCNP/LGCP/Century/Modbus) 모두 일관 적용. 사람이 읽는 식별이 필요한 호출자는 `AgentName()` + `Name()` 또는 `agent/name` REST 라우트 사용.
  - **REST URL composite alias 거부**: `GET /api/v1/devices/{composite}` (예: `lgcnp:81`) 는 HTTP 404 + 마이그레이션 안내 메시지. `ClassifyDeviceRef` 가 composite 패턴을 `DeviceRefUnknown` 으로 분류하여 `DeviceRegistry.ResolveDevice` 가 자동 거부.
  - **WebSocket emit payload 의 `device_id` 필드 완전 제거**: `device.status` 메시지가 `uid` (UUID) 만 노출. Frontend 는 PR2 (D-T20) 에서 마이그레이션 완료.
  - **HVAC 에이전트 emit payload**: 5 에이전트 모두 `{type, unit_id, device_id (UUID), trigger, state, metadata}` schema. composite `id` 필드 부재 확인.
  - **yaml composite 거부**: yaml 의 `pinned: ["lgcnp:81"]` 같은 composite 참조는 부팅 즉시 실패 (`ErrInvalidDeviceReference`). PR1 D-T13 의 yaml_resolver 변경에 D-T2 의 ClassifyDeviceRef 변경이 함께 적용되어 default 분기로 일관 거부.
  - **부팅 시 자동 sanity check**: runServer 시작 시 `device_metadata.json` 의 composite key 잔존을 자동 검증. 발견 시 부팅 거부 + `xflowd migrate device-ids` 명령 안내.

  **신규 명령**:

  - **`xflowd preflight`** — 부팅 사전 점검 명령 (D-T5). config yaml 형식 + composite 참조 부재, `device_ids.json` 로드 가능, `device_metadata.json` UUID-key 검증을 read-only 로 수행. 성공 시 exit 0 + PASSED, 실패 시 exit 1 + 항목별 actionable 메시지.

  **운영자 마이그레이션 가이드** (`docs/migration/device-identity.md`):

  - **greenfield 환경**: 자동 동작 — 아무 조치 불요.
  - **brownfield 환경 (v0.x → v1.0)**:
    1. `xflowd migrate device-ids --metadata-dir <data>/device_metadata` 실행 (PR1 D-T19 의 deprecated noop 이 아닌 실제 변환은 v0.x 빌드에서 수행).
    2. 모든 디바이스 메타데이터 파일이 UUID-key 명명인지 확인.
    3. 외부 클라이언트 / 대시보드 마이그레이션 확인 (REST URL / yaml / MQTT 구독자).
    4. `xflowd preflight` 실행 → PASSED 확인.
    5. v1.0 으로 업그레이드.

  **세부 인수 기준 (D-AC1 ~ D-AC18) 충족 매트릭스**: `.moai/specs/SPEC-DEVICE-IDENTITY-001/acceptance.md` 참조. PR1~PR4 누적으로 모두 GREEN.

### 추가 (Added)

- **SPEC-DEVICE-IDENTITY-001 Phase A** — 디바이스 ID 체계 통일의 첫 단계 (비파괴 추가). `Device` 인터페이스에 `UID() string` 메서드를 1급으로 격상하여 글로벌 유일·불변 UUID 를 노출한다 (Kubernetes 의 `metadata.uid` 패턴 차용). 5개 디바이스 어댑터(NASA/LGCNP/LGCP/Century/Modbus) 가 생성 시점에 `agent.ResolveDeviceID` 또는 동등 경로로 UUID 를 발급받아 보유하며, REST `GET /api/v1/devices` / `GET /api/v1/devices/{id}` 응답에 `uid` 필드가 1급으로 노출된다 (omitempty graceful degradation — `DeviceIDRepository` 미설정 환경에서는 필드가 생략된다). 기존 composite `id` 필드 (`"agent:local_id"`) 는 그대로 유지되어 외부 클라이언트는 영향을 받지 않는다. `DeviceIDRepository` 가 nil 인 경우 부팅 시 1회 경고 로그가 출력되며 (Phase D 에서 부팅 실패로 전환 예정), Prometheus 메트릭 `xflowd_device_uid_missing_total` 로 UUID 미발급 디바이스 수를 관측 가능하다. 인수 기준 A-AC1/A-AC2/A-AC4/A-AC5 충족 (A-AC3 emit map literal `uid` 키 추가는 Phase B 통합). 4 커밋 (`991e793`, `be86904`, `bc67561`, `510fc4b`), production 8 파일 / 테스트 5 파일, 회귀 0건. 후속 Phase B/C/D 는 별도 SPEC 진화로 진행 예정.

### 변경 (Changed)

- **SPEC-DEVICE-IDENTITY-001 Phase A** — `internal/node/inventory.go` 의 `resolveDeviceUUID` 가 디바이스 UUID 발급 시 `Device.UID()` 를 직접 사용하도록 정렬 (`510fc4b`, A-AC5). 이전에는 어댑터별 local ID 형식 차이로 인해 Century 어댑터의 `localID` 형식 불일치(`agent:device:N` 표준 대비 자체 ID 만 반환)가 inventory 노드 UUID 발급 경로에서 잠재적 매핑 실패를 일으킬 수 있었으나, 본 변경으로 동시에 해소되었다. 외부 동작 변경 없음 (비파괴).

### 수정 (Fixed)

- **SPEC-AUTH-004** — REST `POST /api/v1/auth/login` 응답 스키마 정합화 (`{user, tokens}` 중첩 구조) 및 클라이언트 `authStore` 의 토큰 보존 자가 회복 로직 도입. SPEC-AUTH-002 시점(`8635e1f`, 2026-03-31)부터 잠재했던 결함이 SPEC-DASHBOARD-001 v0.2.0 의 `basic_auth: true` 기본값 전환과 함께 표면화된 것을 해소. 서버 DTO 재구조화(`internal/api/dto/auth.go`, `internal/api/handler/auth.go`) + 클라이언트 매핑 변환(`web/src/services/api/authService.ts`) + UB1 `saveTokens` falsy 가드 + UB2 `loadTokens` broken state 자가 회복(`web/src/stores/authStore.ts`) 으로 구성. 기존 활성 사용자 세션은 invalidation 되며 자동 클린업 후 재로그인이 필요하다 (UB2 가 literal `"undefined"` 가 저장된 broken localStorage state 를 자가 회복). 자동화 acceptance AC-1/AC-2/AC-5/AC-6/AC-7 GREEN (906 tests pass), AC-3 (페이지 새로고침 후 인증 복원) / AC-4 (SPEC-AUTH-003 통합 WS 회귀) 는 main 머지 이전 수동 검증 게이트. 신규 의존성/디렉터리/아키텍처 패턴 0건. (commit `8bf49e0`)
- **SPEC-DASHBOARD-001 v0.2.1 hotfix** — dashboard handler 응답 envelope 표준화. `internal/api/handler/dashboard.go` 의 3개 `ctx.JSON` (GET, PUT 200, PUT 409) 이 `dto.NewSuccessResponse(...)` envelope 을 누락하여 클라이언트 axios interceptor (`client.ts:19-44`) 의 strict `body.success` 검증에서 정상 200/409 응답이 `APIError('UNKNOWN', 200)` 으로 변환되고 `DashboardServerError('unexpected status 200')` 를 throw 하여 사용자에게 "대시보드 저장에 실패했습니다…" 토스트가 표시되던 결함을 해소. 서버 측 DB 저장은 성공이었으나 클라이언트가 실패로 인지하던 비대칭 결함. 본 결함은 SPEC-AUTH-004 AC-3/AC-4 수동 검증 (`basic_auth.enabled: true`) 중 dashboard PUT 흐름 관찰로 식별됨. 22 dashboard handler tests + 222 전체 handler tests GREEN, SPEC-AUTH-004 auth 테스트 회귀 0건. 신규 의존성 0건. (commit `9d239c0`)

### 변경 (BREAKING)

- **대시보드 구성 서버 영속화 v0.2.0** (SPEC-DASHBOARD-001 v0.2.0, BREAKING)

  대시보드 페이지/그리드/레이아웃 구성이 브라우저 `localStorage` 에서 SQLite 서버 영속 저장소로 전환된다. v0.1.0 의 결정 일부가 무효화되어 SQLite 채택 + 공유/개인 병행 모델 + 자격증명 SQLite 이관 + localStorage 블랭크 슬레이트 정책이 1급 채택되었다.

  **사용자 안내 (UI 토스트 / CHANGELOG / README 공통 문구)**:

  > 이번 업데이트(v0.2.0)로 대시보드 구성이 서버 저장으로 전환되었습니다. 기존 로컬 구성은 초기화됩니다. 공유 대시보드는 관리자가 다시 구성해 주세요.

  **basic_auth 필수화 (ASM-003)**:
  - `serverCfg.BasicAuth.Enabled=true` 가 v0.2.0 기본값이며 `/api/dashboards/*` 모든 엔드포인트가 유효한 JWT 를 요구한다 (UR-004).
  - `basic_auth.enabled=false` 로 부팅 시 거부되거나, 개발/데모 환경 한정으로 `XFLOW_ALLOW_NO_AUTH=1` 환경변수를 설정하면 경고 로그와 함께 강제 활성화된다.

  **자격증명 저장소 이관 (UR-006, UB-007)**:
  - `~/.xflow/users.yaml` → SQLite `users` 테이블로 부팅 시 1회성 자동 이관 (`INSERT OR IGNORE`) 후 yaml 파일이 `users.yaml.migrated` 로 rename 되어 이후 어떤 인증 흐름에서도 참조되지 않는다.
  - `internal/auth/credentials.go` 의 외부 API (`Load`/`Save`/`Authenticate`/`ChangePassword`/`EnsureDefaultAdmin`/`GetUser`) 시그니처는 유지되어 호출부 변경 없음 (백워드 호환).
  - 로그: `auth: migrated N users from yaml to sqlite`.

  **localStorage 블랭크 슬레이트 (ASM-006 폐기)**:
  - v0.1.0 의 "localStorage → 서버 1회성 마이그레이션" 결정 폐기. 첫 부팅 시 클라이언트가 `dashboardPages`, `activeDashboardId`, `dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout` 6개 키를 명시적으로 제거하고 토스트를 1회 노출한다.
  - `xflow-ui:dashboard-migrated-v0.2` 플래그로 1회 보장 (새로고침 시 재실행 방지).
  - `theme`, `sidebarCollapsed`, `customThemeTokens` 는 기기별 환경설정으로 보존된다.

  **Zustand `partialize` 축소**:
  - 위 6개 대시보드 키가 직렬화 대상에서 완전 제외된다 (UB-002).
  - 메모리 상태에 `sharedSnapshot`, `mineSnapshot` 두 슬롯과 `activeDashboardScope: 'shared' | 'mine'` 추가.

  **품질 게이트 (TRUST 5 PASS)**:
  - 백엔드 85%+ 커버리지, `go test -race ./...` 통과.
  - 프론트엔드 `useDashboardSync` / `uiStore` 단위 테스트 통과.
  - golangci-lint / biome 0 issues.

### 추가

- **공유 + 개인 대시보드 REST API 6 엔드포인트** (SPEC-DASHBOARD-001 v0.2.0)

  운영자가 어떤 브라우저/기기/시크릿창에서 접속하더라도 (공유) + (본인 개인) 두 snapshot 이 일관되게 보이도록 한다. 자세한 API 명세는 `docs/api/dashboards.md` 참조.

  | Method | Path | 권한 |
  |--------|------|------|
  | GET | `/api/dashboards/shared` | 인증된 사용자 (전부) |
  | PUT | `/api/dashboards/shared` | admin only |
  | DELETE | `/api/dashboards/shared` | admin only |
  | GET | `/api/dashboards/mine` | 인증된 사용자 |
  | PUT | `/api/dashboards/mine` | 인증된 사용자 |
  | DELETE | `/api/dashboards/mine` | 인증된 사용자 |

  **신규 백엔드 파일**:
  - `internal/storage/dashboard_sqlite.go` — SQLite `dashboards` 테이블 기반 `DashboardRepository` 유일 구현 (`Get` / `Put` / `Delete`, 트랜잭션 내 If-Match + 서버측 version 부여).
  - `internal/storage/users_sqlite.go` — `users` 테이블 CRUD + yaml → SQLite 1회 이관 헬퍼.
  - `internal/api/handler/dashboard.go` — 6 엔드포인트 + JWT 미들웨어 + `requireAdmin` (shared PUT/DELETE) + owner spoofing 차단 + URL/body scope 불일치 거부.
  - `internal/api/dto/dashboard.go` — `DashboardSnapshot` DTO (`scope`, `owner`, `version`, `updatedAt`, `payload`).

  **SQLite 스키마 (자동 생성, `CREATE TABLE IF NOT EXISTS`)**:
  - `dashboards` 테이블 (`scope` ∈ `{'global','user'}`, `owner` nullable, `version` 단조 증가, `updated_at` epoch ms, `payload` JSON TEXT).
  - `dashboards_scope_owner_uidx` partial unique index 로 `COALESCE(owner, '')` 기반 cross-scope 공존 보장 (`scope=global,owner=NULL` 과 `scope=user,owner=<username>` 이 동일 인덱스에서 충돌 없이 공존).
  - `users` 테이블 (`username` UNIQUE, `password_hash`, `role` ∈ `{'admin','editor','viewer'}`, `created_at`/`updated_at` epoch ms).

  **신규 프론트엔드 파일**:
  - `web/src/types/dashboard.ts` — `DashboardSnapshot`, `DashboardScope`, `DashboardPayload` 타입.
  - `web/src/services/api/dashboardService.ts` — 6 메서드 REST 클라이언트 (`If-Match` 헤더 지원, 401/403/409 응답 분기).
  - `web/src/hooks/useDashboardSync.ts` — 부팅 시 `Promise.all([getShared, getMine])` 병렬 GET, 500ms debounce PUT, 409 last-write-wins 재PUT (1회 한정).
  - `web/src/pages/dashboard/DashboardPage.tsx` "공유" / "내 대시보드" 탭 토글 (admin 외에는 공유 탭 편집 컨트롤 비활성).

  **응답 코드 매트릭스**:
  - `200 OK` — 성공
  - `204 No Content` — DELETE 성공
  - `400 Bad Request` — payload schema 오류, URL vs body scope 불일치
  - `401 Unauthorized` — JWT 없음/만료
  - `403 Forbidden` — 권한 부족 (editor 가 shared PUT/DELETE 시)
  - `404 Not Found` — GET 시 snapshot 미존재 (초기 상태)
  - `409 Conflict` — `If-Match` version 불일치 (body 에 서버측 최신 snapshot)
  - `413 Payload Too Large` — payload 크기 256 KB 초과
  - `500 Internal Server Error` — 저장소 I/O 실패

### Deprecated

- **`~/.xflow/users.yaml`** (SPEC-DASHBOARD-001 v0.2.0): v0.2.0 부팅 시 SQLite `users` 테이블로 1회성 자동 이관된 후 `users.yaml.migrated` 로 rename 된다. 이후 어떤 인증 흐름에서도 yaml 은 참조되지 않으며, SQLite 만 source-of-truth 다 (UB-007). 신규 사용자는 SQLite 에 직접 INSERT 되며, 별도 등록/삭제/목록 관리 REST API 는 `SPEC-USER-MGMT-001` (OI-004, 추후) 로 분리된다.

### Removed

- **대시보드 페이지/그리드/레이아웃 localStorage 영속화** (SPEC-DASHBOARD-001 v0.2.0): `web/src/stores/uiStore.ts` 의 `partialize` 에서 `dashboardPages`, `activeDashboardId`, `dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout` 6개 키가 완전 제외되었다 (UB-002). 첫 부팅 시 1회 명시적 제거 후 더 이상 직렬화되지 않는다.
- **v0.1.0 의 `internal/storage/dashboard_file.go` 결정 폐기** (SPEC-DASHBOARD-001 v0.2.0): JSON 파일 1차 채택 결정이 SQLite 채택으로 무효화되었다 (OI-003 CLOSED). 해당 파일은 실제로 작성된 적이 없으며, v0.2.0 에서도 작성하지 않는다.
- **v0.1.0 의 "localStorage → 서버 1회성 마이그레이션" 흐름 삭제** (SPEC-DASHBOARD-001 v0.2.0, ASM-006 폐기): 사용자별 스코프와 권한 모델 도입으로 클라이언트 측 단일 페이로드를 "공유" 와 "개인" 중 어디로 보낼지 자의적으로 결정할 수 없기 때문이다. 대신 모든 사용자는 빌트인 기본 대시보드에서 새로 시작하며, admin 이 공유 대시보드를 새로 구성한다.

### Security

- **Cross-user 대시보드 접근 차단** (SPEC-DASHBOARD-001 v0.2.0, UB-005): `/api/dashboards/mine` 은 항상 JWT `Claims.Username` 으로만 owner 가 결정되며 별도의 username 파라미터를 받지 않는다. 사용자 A 는 사용자 B 의 개인 대시보드를 GET/PUT/DELETE 할 수 없다.
- **Owner spoofing 차단** (SPEC-DASHBOARD-001 v0.2.0, UB-003): 서버는 PUT 페이로드의 `scope`/`owner`/`version`/`updatedAt` 을 모두 무시하고, URL (shared/mine) + JWT `Claims.Username` + 저장소 상태로 결정한다. 클라이언트가 body 에 `owner: 'bob'` 을 보내도 alice 의 JWT 로 요청하면 alice 의 snapshot 이 갱신된다.
- **URL vs body scope 불일치 거부** (SPEC-DASHBOARD-001 v0.2.0, UB-006): silent normalize 금지. URL 의 scope (shared/mine) 와 body 의 `scope` 가 불일치하면 `400 Bad Request` 로 명시적으로 거부한다.
- **Admin role guard** (SPEC-DASHBOARD-001 v0.2.0, UB-004): editor/viewer 는 공유 대시보드를 GET 만 가능하며, `PUT /api/dashboards/shared` 또는 `DELETE /api/dashboards/shared` 시도는 `403 Forbidden` 으로 거부된다.
- **Payload 크기 캡 256 KB** (SPEC-DASHBOARD-001 v0.2.0, UR-003): 초과 시 `413 Payload Too Large`. JSON schema 오류는 `400 Bad Request` 로 거부.
- **basic_auth 강제** (SPEC-DASHBOARD-001 v0.2.0, UR-004): 모든 `/api/dashboards/*` 엔드포인트는 유효한 JWT 를 요구한다. 익명 접근 불허. `XFLOW_ALLOW_NO_AUTH=1` 환경변수는 개발/데모용 opt-out 으로만 사용되며 경고 로그를 남긴다.

- **Frontend Store 키 모델 v0.7.0 적응** (SPEC-WEB-005 v0.7.0, BREAKING for frontend internal API)
  
  SPEC-STORE-003 v0.3.0 백엔드 BREAKING (registration_type, data_type, metric_type, 객체 배열 응답)에 대응하는 frontend 단독 진화. 운영자에게 노출되지 않는 내부 API contract 변경이므로 end-user 마이그레이션 가이드는 불필요하며 개발자 대상 변경만 다룬다.
  
  **타입 진화 (M11)**:
  - `StoreKeysRawResponse.keys: string[]` → `keys: StoreKeyObject[]` (`{key, registration, data_type, metric_type, tags}`)
  - 신규 타입: `DataType`, `RegistrationSource`, `StoreKeyObject` (`@/services/api/store`)
  - 신규 함수: `fetchStoreKeyObjects(agentName)` (Phase E 에서 직접 활용)
  - 백워드 호환: `useStoreKeysWithTags` 가 `{keys: string[], tags: StoreKeyTagsMap, keyObjects: StoreKeyObject[]}` 반환 (기존 소비자 무수정)
  
  **Config UI 진화 (M12, M13)**:
  - `agentSchemas.ts`: `allow_dynamic_keys: bool` 토글 → `registration_type: select` (manual|auto, default auto)
  - `StoreKeysEditor`: 신규 `data_type` 셀렉트 컬럼 (6종 enum) + `metric_type` 입력 컬럼 (정규식 검증)
  - 신규 helper `storeKeysValidation.ts`: `validateDataType`, `validateMetricType`, `DATA_TYPE_OPTIONS`
  
  **PromoteToStaticDialog 진화 (M14)**:
  - 동적→정적 변환 시 `data_type` 필수 + `metric_type` 옵션 입력
  - `defaultDataType` prop 으로 백엔드 추론 값 사전 채움
  - `onConfirm` 시그니처 변경: `(tags) => void` → `(payload: PromoteToStaticPayload) => void`
  
  **에러 매핑 (M15)**:
  - 신규 모듈 `storeErrorMapper.ts`: 4종 백엔드 에러 (`ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`) + 마이그레이션 에러를 한국어 사용자 친화 메시지로 매핑
  - `mapStoreError(err): StoreErrorMapped` 통합 진입점
  
  **메타데이터 표시 + 필터 UI (M16, Task 13, Task 14)**:
  - 신규 컴포넌트 `MetadataChips`: data_type (6종 색상) / metric_type / registration auto/manual 배지
  - `TsdbDataViewerModal` 시리즈 행에 메타데이터 칩 표시
  - 신규 필터 UI 3축: `?data_type=`, `?metric_type=` (datalist 자동완성), `?registration=` (segmented), 모두 AND 결합
  - `StoreKeysEditor` 행에 manual 배지 (yaml 정의 = manual 시각 reminder)
  - `metric_type === "unknown"` 키는 muted 표시
  
  **품질 게이트**:
  - 625/625 tests pass (Vitest, +103 신규)
  - TypeScript strict pass (any 사용 0)
  - storeErrorMapper.ts / MetadataChips.tsx / storeKeysValidation.ts 100% 커버리지
  - StoreKeysEditor 99.35%, PromoteToStaticDialog 97.87%, TsdbDataViewerModal 90.62%
  - Vite production build success
  - 신규 외부 라이브러리 추가 없음
  
  **알려진 차이**: SPEC-STORE-003 v0.3.0 의 M9 known divergence (?metric_type= 빈 값) 는 v0.7.0 frontend 측 필터에서도 동일하게 no-op passthrough 로 처리됨.

- **Store 에이전트 키 메타데이터 모델 v0.3.0 진화** (SPEC-STORE-003 v0.3.0)

  v0.2.0의 `allow_dynamic_keys` (bool)을 `registration_type` (enum: `manual` | `auto`)로 **clean rename** 한다 (하위호환 shim 없음). 또한 `data_type` (6종 enum), `metric_type` (semantic free string) 1급 필드를 신설하고, `GET /keys` API 응답을 string 배열에서 객체 배열로 진화시킨다.

  **YAML 스키마 변경 (BREAKING)**:
  - `allow_dynamic_keys: false` → `registration_type: "manual"`
  - `allow_dynamic_keys: true` → `registration_type: "auto"` (또는 생략, default `auto`)
  - manual 모드의 `keys[]` 각 엔트리는 `data_type` 명시 필수 (6종: `int`/`float`/`string`/`boolean`/`bytes`/`json`)
  - 신규 optional `metric_type` 필드 (free string `^[a-zA-Z0-9_-]+$`, default `"unknown"`)
  - **부팅 가드**: `allow_dynamic_keys` 잔존 시 명시적 에러로 부팅 실패 ("removed in v0.3.0; use 'registration_type: manual|auto' instead")

  **API 응답 변경 (BREAKING)**:
  - `GET /api/v1/store/{name}/keys` 응답: string 배열 + 별도 `tags` 맵 → 객체 배열 `[{key, registration, data_type, metric_type, tags}]`
  - 응답 객체는 항상 5개 필드 모두 포함 (빈 tags도 `{}`로 명시)
  - 응답 배열은 `key` 알파벳 오름차순 정렬 (안정성 보장)

  **API 신규 필터 (NEW)**:
  - `?data_type=<int|float|string|boolean|bytes|json>` (단일 값)
  - `?metric_type=<value>` (단일 값)
  - `?registration=<manual|auto>` (단일 값)
  - 기존 `?tag=key:value`와 모두 **AND 조건** 결합 (`?registration=manual&metric_type=temperature&tag=room:1`)

  **신규 에러 4종**:
  - `ErrTypeMismatch`: 등록된 `data_type`과 쓰기 값 Go 타입 불일치 (auto 모드 첫 쓰기 후 영구 고정)
  - `ErrUnsupportedValueType`: nil/chan/func 등 추론 불가 타입 (auto 모드)
  - `ErrInvalidDataType`: yaml의 `data_type` 값이 6종 enum 외이거나 manual 모드에서 누락
  - `ErrInvalidMetricType`: `metric_type`이 정규식 위반

  **운영 마이그레이션** (필수):
  - 기존 yaml의 `allow_dynamic_keys` 모두 `registration_type`으로 변환 필요
  - manual 모드의 모든 정적 키에 `data_type` 추가 필요
  - 자세한 절차: `docs/migration/v0.3.0-store-keys.md` 참조

  **알려진 차이 (M9 known divergence)**: `?metric_type=` 빈 값은 SPEC 명시("빈 결과 반환")와 달리 no-op passthrough로 처리된다. metric_type normalize 정책으로 사용자 영향 없음. 다음 SPEC 갱신에서 SPEC을 구현에 맞춰 정렬할 예정.

  **연관 SPEC**:
  - SPEC-WEB-005 v0.5.0 (예정): UI는 객체 배열 응답에 적응 + `data_type`/`metric_type` 편집 UI 제공

  **품질**: TRUST 5 PASS, 1296 race-clean 테스트, `store_data_type.go` 100% 커버리지, golangci-lint 0 issues.

### 추가

- **xflowd 자동 업데이트 v0.2.0 진화** (SPEC-UPDATE-002 v0.1.0)

  SPEC-UPDATE-001 v0.1.0 (xflowd 자동 업데이트 기반) + SPEC-WEB-006 v0.1.0 (Admin UI) 의 후속 진화. v0.1.0 에서 운영자 부담으로 남겨두었던 3가지 한계 (수동 재시작, 채널 변경 부재, 단일 바이너리만 지원) 를 모두 해소한다. 신규 의존성 추가 없이 기존 라이브러리 재사용으로 backward compatibility 를 100% 유지한다.

  **In-Process Restart (M-1)**:
  - graceful drain (active connections 보호 + in-flight 요청 완료 대기)
  - TOCTOU 재검증 (`syscall.Exec` 직전 SHA256 + Ed25519 재검증으로 다운로드 후 디스크 변조 차단)
  - `syscall.Exec` 자기 교체 (PID 보존, OS 가 자동으로 새 바이너리로 프로세스 이미지 교체)
  - health check + auto rollback (재시작 후 health endpoint 폴링, 실패 시 `.previous` 자동 복원 후 재기동)
  - 신규 sentinel error: `ErrUpdateRestartFailed`, `ErrUpdateHealthCheckFailed`

  **Channel REST API (M-2, M-7, M-8)**:
  - `GET /api/v1/system/update/channel` — 현재 채널 (stable/beta/nightly) + manifest URL 조회
  - `POST /api/v1/system/update/channel` — 채널 변경 (admin role guard, 비-admin 시 HTTP 403)
  - `ContextKeyUserRole` export 로 role 기반 가드 일관화
  - `ChannelChangeDialog` UI (admin 전용 채널 선택/변경 다이얼로그)
  - `SystemVersionCard` 가 `isAdmin` 일 때만 채널 변경 버튼 노출

  **Multi-Binary Auto-Update (M-3, M-9, M-11, M-12)**:
  - 3개 바이너리 지원: `xflowd` (daemon) / `xflow-agent` (edge agent) / `xflow` (CLI)
  - `DependencyManifest` 에 semver constraint 표현 + `ManifestFetcher` (HTTPS 강제 + 64KB DoS cap + Ed25519 서명 검증)
  - `CompatibilityChecker` 로 다운그레이드/non-compatible upgrade 사전 차단
  - ReDoS-resistant semver regex (`^...$` 앵커 적용으로 백트래킹 폭발 차단)
  - target whitelist (`xflowd|xflow-agent|xflow` 3종만 허용 → path traversal / arbitrary binary swap 방어)
  - `UpdateDialog` 의 admin target dropdown (target 선택 + auto_restart checkbox)
  - 신규 sentinel error: `ErrUpdateIncompatibleVersion`

  **11-state OperationStatus Machine (M-1)**:
  - 기존 9-state 머신 → 11-state 확장
  - 신규 상태: `restarting` (in-process restart 중), `health_checking` (재시작 후 health 검증 중)
  - `UpdateProgressStepper` 시각화 + `StatusLabel` 한국어 라벨 + admin role 별 표시
  - 신규 상태는 `auto_restart=true` 시에만 진입 (v0.1.0 동작 보존)

  **Backward Compatibility (M-14)**:
  - `ApplyRequest` 확장: `target`, `auto_restart` 모두 옵셔널 (v0.1.0 동작 100% 보존)
  - `target` 미지정 → `xflowd` default
  - `auto_restart` 미지정 → `false` (v0.1.0 과 동일하게 운영자 수동 재시작 경로 유지)
  - 11-state machine 의 신규 상태 (`restarting`, `health_checking`) 는 `auto_restart=true` 시에만 진입
  - Scenario 11: `target=xflow` (CLI) 선택 시 `auto_restart` 자동 해제 + disabled (CLI 는 daemon 이 아니므로 self-restart 불필요)

  **품질 지표 (TRUST 5 PASS)**:
  - `internal/updater` 91.9% / `internal/api/system_update.go` 92.0% 커버리지
  - `go test -race ./...`: 모든 패키지 통과 (race-clean)
  - web test suite: 858/858 통과 (신규 ~50 tests 추가)
  - 41 `UpdateDialog` tests + 24 `RestartOrchestrator` tests + 20+ manifest tests
  - `ChannelChangeDialog` 99.46% 커버리지
  - TypeScript strict / ESLint / `gofmt` / `go vet` 모두 클린

  **보안 검증 (PASS)**:
  - TOCTOU 재검증 (`syscall.Exec` 직전 Ed25519 + SHA256 재검증으로 디스크 변조 공격 차단)
  - Admin role guard (HTTP 403 + `ContextKeyUserRole` 일관 적용)
  - target whitelist (`xflowd|xflow-agent|xflow` 만 허용, path traversal / arbitrary binary swap 방어)
  - HTTPS 강제 + 64KB DoS cap + ReDoS-resistant semver
  - OWASP A01 (Broken Access Control) / A02 (Cryptographic Failures) / A03 (Injection) / A06 (Vulnerable Components) / A08 (Software and Data Integrity Failures) 점검 통과

  **신규 외부 의존성**: 0개 (기존 lib 재사용 — `crypto/ed25519`, `crypto/sha256`, `syscall`, Go stdlib + 기존 frontend stack)

  **Out of Scope (후속)**:
  - SPEC-UPDATE-003 (예정): Windows 지원 (`syscall.Exec` 대안 — Windows 는 exec semantic 차이로 별도 SPEC 필요)
  - 향후: `cmd/xflowd` CLI 의 `--auto-restart`, `--target` 플래그를 daemon-side API 호출 모드로 활용 (현재는 daemon-side ApplyRequest 만 지원)

- **Web Admin: 시스템 자동 업데이트 UI** (SPEC-WEB-006 v0.1.0)

  관리자가 Web UI 에서 xflowd 자동 업데이트를 안전하게 관리할 수 있다.
  SPEC-UPDATE-001 v0.1.0 의 5 REST API 를 소비하는 frontend 컴포넌트 모음.

  **System Status Panel** (`/admin/system`):
  - 현재 버전 + 빌드 메타 (commit, build_date, go runtime) 표시
  - 채널 정보 (stable/beta/nightly) 배지
  - 업데이트 가능 인디케이터 + 최신 버전 표시
  - 60s 자동 폴링 (TanStack Query refetchInterval)
  - "업데이트 확인" 버튼 (즉시 채널 폴)

  **Update Dialog** (5단계 UX):
  - info → confirm → apply → progress → result
  - 9-state machine 시각화 (UpdateProgressStepper)
  - 1s 진행률 폴링 (백엔드 OperationStatus 동기화)
  - 다운그레이드 force checkbox (필요 시)
  - 실패 시 명시적 rollback 버튼 + 다시 시도

  **Restart Guide** (M8, v0.1.0 한계 보완):
  - in-process restart 미지원 → 운영자 수동 재시작 안내
  - systemd 명령 + 수동 명령 양쪽 표시
  - 클립보드 복사 버튼 (per-command)

  **Header Badge + Toast Notification**:
  - 헤더 우측 RefreshCw 아이콘 + 노란색 dot (update_available 시)
  - 한 번만 발생: false→true 전환 감지 (useRef 패턴)
  - 클릭 시 /admin/system 페이지로 이동
  - admin role 미보유 시 disabled

  **권한 모델** (M11, Decision Point 5):
  - 기존 AuthGuard 에 requireRole="admin" 옵션 확장
  - 비-admin 접근 시 ForbiddenPage 노출 (한국어 403)
  - authEnabled=false (dev mode) 우회 보존

  **에러 메시지 매핑** (M12):
  - 12종 백엔드 에러 분류 → 한글 사용자 친화 메시지
  - HTTP 401/409 + body keyword 우선순위 매칭
  - reuse 패턴 (storeErrorMapper, SPEC-WEB-005 v0.7.0)

  **품질 검증 (TRUST 5 PASS)**:
  - 808/808 vitest 통과 (신규 ~182 tests)
  - 신규 파일 함수 커버리지 100%
  - storeErrorMapper.ts 100%, UpdateProgressStepper.tsx 100%, etc.
  - TypeScript strict 통과 (any 0건)
  - Vite production build 성공 (SystemStatusPage 27.58 kB)

  **신규 외부 의존성**: 0개 (React 19 + TanStack Query + Tailwind + lucide-react 모두 기존)

  **알려진 차이/제약 (v0.1.0 백엔드 한계 보완)**:
  - in-process restart 미지원 → CLI 안내 + 클립보드 복사
  - 자동 health check + rollback wiring 미연결 → 명시적 rollback 버튼
  - 채널 변경 REST API 부재 → 읽기 전용 표시 + CLI 안내

  **후속 SPEC**:
  - SPEC-UPDATE-002 (예정): 멀티 바이너리 + 채널 변경 API + in-process restart
  - SPEC-WEB-007 (예정): RBAC 정식화

- **xflowd 자동 업데이트 메커니즘** (SPEC-UPDATE-001 v0.1.0)

  운영자는 GitHub Releases 채널 (stable/beta/nightly) 에서 새 xflowd 바이너리를
  안전하게 다운로드/검증/적용할 수 있다. 자가 교체 + 자동 롤백으로 BREAKING
  배포 후 운영자 부담 경감.

  **보안 (M4, M13)**:
  - Ed25519 디지털 서명 + SHA256 체크섬 검증 (timing-safe 비교, crypto/subtle.ConstantTimeCompare)
  - HTTPS 강제 (Checker + Downloader 다중 경계 검증)
  - 공개키 핀닝 (PEM/hex/file 로더, RSA 자동 거부)
  - TOCTOU 방어 (다운로드 직후 + 원자적 교체 직전 2회 검증)
  - DoS 방어 (io.LimitReader: checksum 1MB / signature 64KB)

  **흐름 (M3-M7)**: check → download (HTTPS) → verify → apply (atomic rename
  via go-update) → restart (syscall.Exec). 실패 시 백업 자동 복원 (.previous 접미사).

  **다운그레이드 차단 (M8)**: --force 플래그 없이 거부. 3단 방어 (Checker +
  CLI + REST API).

  **CLI (M9)**:
  - `xflowd update check` — 새 버전 확인
  - `xflowd update apply [--version vX.Y.Z] [--force] [--yes]` — 적용
  - `xflowd update status [--json]` — 작업 상태
  - `xflowd update rollback [--yes]` — 이전 버전 복원
  - `xflowd update channel <stable|beta|nightly>` — 채널 변경

  **REST API (M10)**:
  - `GET /api/v1/system/version` — 현재 버전 + 메타데이터
  - `POST /api/v1/system/update/check` — 채널 폴
  - `POST /api/v1/system/update/apply` — 비동기 작업 시작 (operation_id 반환)
  - `POST /api/v1/system/update/rollback` — 백업 복원
  - `GET /api/v1/system/update/status` — 마지막 작업 스냅샷

  **설정 (M11, `.moai/config/update.yaml`)**:
  - `enabled: false` (기본값, 명시적 opt-in)
  - `channel: stable|beta|nightly`
  - `update_url: https://api.github.com/...`
  - `public_key_path` 또는 `public_key_hex`
  - `auto_apply: false` (수동 승인 권장)
  - `drain_timeout`, `health_check_timeout`, `health_check_endpoint`

  **품질 게이트**:
  - 280+ 신규 테스트 (15 GWT + 16 보안 + 10 E2E + 핸들러 + CLI + 단위)
  - 16개 위협 벡터 보안 검증 (MITM/replay/서명 위조/바이너리 변조/timing/DoS)
  - internal/updater 92.6% 커버리지, verifier.go 100%
  - golangci-lint 0 issues, go vet clean, race-detector pass
  - 모든 39 패키지 회귀 0건

  **신규 의존성**: `github.com/inconshreveable/go-update` (atomic rename, 검증된 lib)

  **Out of Scope (후속 SPEC)**:
  - SPEC-UPDATE-002: xflow-agent / xflow CLI 멀티 바이너리 조정
  - SPEC-UPDATE-003: Windows 지원
  - SPEC-WEB-006: Web UI System Status Panel + 업데이트 다이얼로그

  **Known Limitations**:
  - In-process restart는 v0.1.0에서 ready_to_restart 상태로 종료 (외부 supervisor 의존)
  - 자동 헬스체크 + 자동 롤백 wiring 은 후속 SPEC iteration에서 구현

- **저장소 전체/개별 키 초기화 기능** (SPEC-STORE-003)
  - `DELETE /api/v1/store/{name}/keys/{key}` — 정적 키는 history만 삭제, 동적 키는 entry 완전 삭제
  - `DELETE /api/v1/store/{name}/keys` — bulk 적용, 카운트 응답
  - 신규 백엔드 메서드: `Store.ClearHistory(ctx, key)` (VolatileStore/NamespacedStore/PersistentStore 구현)
  - Frontend: `ConfirmDialog` 재사용 컴포넌트, "전체 초기화" 헤더 버튼, 행별 휴지통 아이콘

- **동적 키를 정적으로 변환하는 UI** (SPEC-STORE-003)
  - 저장소 리스트의 동적 키 행에 변환 버튼 추가
  - `PromoteToStaticDialog` 신규 컴포넌트: 태그 입력 후 config.keys 추가
  - Configure API 재사용 (별도 백엔드 변경 없음)

- **TSDB 데이터 뷰어 3종 개선** (SPEC-WEB-005)
  - 키 세그먼트에서 태그 자동 추출 (`keyTagExtractor` 유틸): InfluxDB 스타일 + colon/slash segments
  - 평균 집계 소수점 자릿수 입력 (기본 1, 0-6 범위) — 매트릭스 셀 + CSV 모두 적용
  - 매트릭스 페이지네이션 (페이지 크기 [10, 25(기본), 50, 100])
  - react-window 가상화 제거 (페이지네이션으로 대체)

- **TSDB/Store 에이전트 시리즈 탐색 및 데이터 뷰어** (SPEC-WEB-005 v0.4.0)
  - 페이지네이션 (10/25/50/100), 다중 시리즈 매트릭스 쿼리(키=컬럼, 시간=행)
  - 데이터 뷰어 모달 (95vw×95vh): 절대/상대 시간 모드, 인터벌 프리셋, 집계(min/max/avg)
  - 5,000행 경고 + react-window 가상 스크롤(500행 이상 자동)
  - CSV 내보내기 (UTF-8 BOM, 로컬 ISO-8601 timezone offset)
  - SeriesDataSource 통합 어댑터로 tsdb/store 모두 지원
  - 저장소 탭에 통합된 데이터 보기 버튼 + 페이지네이션
  - 설정 탭 운영/데이터 섹션 분리 + 태그 chip 필터(저장소 + 데이터 뷰어)
  - 363+ 테스트 신규/추가 (frontend), 35+ 테스트 (backend)

- **Store 에이전트 정적 키 정의 및 태그 메타데이터** (SPEC-STORE-003 v0.1.0)
  - `allow_dynamic_keys` (default true): false 시 정적 목록 외 키 쓰기 거부 (`ErrKeyNotAllowed`)
  - `keys: [{key, tags: map[string]string}]` 정적 키 정의 (태그 key regex `^[a-zA-Z0-9_-]+$`)
  - 신규 API: `GET /api/v1/store/{name}/keys?tag=k:v` (다중 AND 필터), `GET /api/v1/store/{name}/tags` (유니크 태그 페어 목록)
  - 기존 `GET /keys` 응답에 optional `tags` 맵 포함 (omitempty 하위호환)
  - api.Context 에 `QueryValues(name) []string` 추가 (다중 쿼리 파라미터)

- **chart-emitter 배치 입력 모드** (SPEC-CHART-001 v1.2.0)
  - `entries_field` config 추가. 설정 시 `payload[entries_field]` 배열을 개별 ChartEntry 로 분해하여 publish.
  - 배열은 **timestamp 오름차순으로 정렬**된 뒤 순차 publish → FIFO 링버퍼가 `buffer_size` 를 초과해도 최신 타임스탬프가 남음.
  - `store-read(read_mode=last_n/duration/time_range)` 의 배열 출력을 라인/바 차트 backfill 에 직접 공급 가능 (기존 v1.1.0 에서는 배열 전체가 하나의 `value` 로 감싸져 차트가 그려지지 않던 문제 해결).
  - 필드가 없거나 배열이 아니면 단일 엔트리 모드로 fallback → 동일 emitter 에 이력 배치 + 실시간 append 혼합 공급 허용.
  - primitive 배열 (`[21, 22, 23]`) 도 지원: 각 값이 `value` 로 저장되고 `timestamp` 는 현재 epoch ms 로 자동 주입.
  - Go 테스트 7개 추가 (`TestChartEmitterNode_Process_BatchMode_*`), race clean.
  - 문서 (`docs/guides/chart-panel-flow.md`) 의 Example 2 를 `entries_field` 사용 패턴으로 개편.
  - 노드 상세 패널의 입력 예제가 배치/단일 모드 양쪽을 명시적으로 보여주도록 갱신.

- **차트 패널 플로우 연동 시스템 구현** (SPEC-CHART-001)
  - **`chart-emitter` 종단 노드** (`internal/node/chart_emitter.go`): 입력 메시지를 WebSocket 차트 채널로 발행하고 링버퍼(FIFO + retention 스윕)에 보관. config: `channel_name` (정규식 검증), `buffer_size` (1-10000), `retention_sec` (0-86400). 채널 이름 중복 시 fail-fast Init 에러.
  - **`ChartChannelRegistry` 싱글톤** (`internal/agent/system/chart_channel_registry.go`): 프로세스 전역 채널 레지스트리. ChartSubscriber 인터페이스, EncodeChart{Backfill,Append,Closed,Error} 프레임 헬퍼. race-clean (sync.RWMutex).
  - **`GET /ws/chart/{channel}` WebSocket 엔드포인트** (`internal/api/ws/chart_channel.go`): channel_name 정규식 검증 (HTTP 400), 미존재 채널 `chart.error`, backfill + append fan-out, slow-consumer 보호 (256 항목 버퍼 + 초과 시 close), 연결 종료 시 자동 unsubscribe.
  - **HTTP 쿼리 API** (외부 도구 / 디버깅용):
    - `GET /api/v1/charts/channels` — 활성 chart-emitter 채널 목록
    - `POST /api/v1/store/{agent}/query` — Store 5-모드 HistoryQuery (latest/last_n/duration/time_range/since_n)
    - `POST /api/v1/influxdb/{agent}/query` — Flux / InfluxQL 쿼리 (`InfluxDBAgent.ExecuteFluxQuery`/`ExecuteInfluxQLQuery` 신규 메서드)
    - 표준 응답 스키마: `{entries: [{timestamp, value, labels}], count, truncated}`
  - **5종 차트 패널** (`web/src/pages/dashboard/panels/charts/`): Stat (delta + 임계값 색상), Line Chart (multi-series 지원), Bar Chart (category / time_bin 모드), Pie Chart (집계), Table (정렬 + 페이지네이션). Recharts 3.7 기반 + HTML table.
  - **`useChartChannel` React 훅 + `ChartChannelClient`** (`web/src/services/ws/chartChannel.ts`): exponential backoff 재연결 (1→16s), chart.closed 수신 시 영구 종료, maxPoints 슬라이딩 윈도, factory 주입으로 테스트 가능.
  - **대시보드 UI 확장**:
    - AddPanelDialog: 차트 타입 선택 시 채널 드롭다운 (`GET /api/v1/charts/channels` 연동) + 수동 입력 + 정규식 인라인 검증
    - PanelSettingsDialog: 5종 차트 타입별 config 편집 섹션
    - `panelDefaultSize` SPEC 값 적용 (stat 2×1, line-chart 6×3, bar-chart 4×3, pie-chart 3×3, table 6×4)
    - uiStore v3→v4 persist 마이그레이션 (기존 dataSource/period config 보존)
  - **플로우 캔버스 통합**: `web/src/config/nodeSchemas.ts` + `web/src/pages/nodes/nodeTypeMeta.ts` 에 chart-emitter 등록. NodePalette 이 `category=output` 그룹에 자동 배치, PropertyPanel 이 DynamicForm 으로 설정 편집 UI 자동 생성.
  - **활용 가이드 문서** (`docs/guides/chart-panel-flow.md`): 아키텍처 다이어그램, payload 정규화 규칙, 3종 예시 플로우 YAML, 기존 노드 조합 패턴, 운영 주의사항, HTTP 쿼리 API curl 예시, 문제 해결 표.
  - **핵심 설계 원칙**:
    - 차트 패널은 데이터 소스를 몰라야 한다 — 오직 `channel_name` 만 안다
    - 필터링/집계/정렬은 플로우 노드(`filter`, `aggregate`, `mapping`)가 담당
    - 모든 타임스탬프는 epoch ms (int64) 로 통일
    - 채널 = 하나의 chart-emitter 인스턴스 (중복 이름 fail-fast)
  - **테스트**: Go 신규 파일 평균 93% 커버리지 + `go test -race` 통과 / Vitest 132 테스트 평균 88% 커버리지. TRUST 5 게이트 전부 통과.

- **TCP 소스 노드에 `connection_id` 메타데이터 주입** (SPEC-NODE-003)
  - `TCPInNode.receiveLoop`에서 매 메시지에 `connection_id` 메타데이터를 설정하여, framer 노드와 결합 시 TCP 서버의 다중 클라이언트 연결별 독립 프레이밍을 지원.
  - TCP 서버 모드(`ConnAwareReceiver`): `connection_id` = `remoteAddr` (host:port). `tcp.remote_addr`과 동일한 값으로 설정되며, 기존 `tcp.remote_addr` 메타데이터도 그대로 유지 (하위 호환).
  - TCP 클라이언트 모드(`MessageReceiver`): `connection_id` = `n.ID()` (노드 ID). 단일 연결이므로 고정 식별자로 일관된 메타데이터 구조 제공.
  - framer 노드의 기본 `stream_key_metadata="connection_id"`와 자동 연동되어, 추가 설정 없이 "tcp-in (서버) → framer" 파이프라인에서 연결별 프레이밍 동작.

- **`pkg/framing` 공개 패키지 신설** (SPEC-NODE-002)
  - 시리얼 에이전트 내부(`internal/agent/serial/framing.go`)에 있던 프레이밍 엔진을 `pkg/framing` 공개 패키지로 승격하여 범용 재사용이 가능하도록 함.
  - 6가지 프레이밍 모드 지원: `raw`, `newline`, `length_prefix`, `fixed_size`, `stream`, `frame`.
  - `Framer` 인터페이스, `Options` 구조체, `New` 팩토리 함수, `Drain` API, 모드 상수 (`ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame`) 노출.
  - `ScannerConfigurer` 인터페이스를 통해 기존 `SerialConnReader`와의 호환성 유지.
  - sentinel 에러 (`ErrETXMismatch`, `ErrChecksumMismatch`, `ErrMaxSizeExceeded`, `ErrLengthInvalid`) 노출로 에러 분류 지원.
  - 시리얼 에이전트(`internal/agent/serial`)는 `pkg/framing`을 import하여 기존과 동일한 동작을 유지.

- **`framer` 처리 노드 추가** (SPEC-NODE-002)
  - 임의의 바이트 스트림 소스 노드(serial-in, tcp-in, udp-in 등)의 출력을 받아 프로토콜 프레임으로 분리하는 `framer` 처리 노드를 `internal/node/`에 추가.
  - 입력 포트 `in`, 출력 포트 `out`, 에러 포트 `error`의 3포트 구조.
  - 메타데이터 기반 다중 스트림 버퍼 관리 (`connection_id` 등 `stream_key_metadata` 키로 스트림 분리). 키가 없으면 단일 공용 버퍼로 동작.
  - `max_streams`, `stream_idle_timeout` 옵션을 통한 자원 제한 및 유휴 스트림 lazy 축출.
  - 에러 포트를 통한 파싱 에러 분리 (에러 코드: `frame.input.invalid_payload`, `frame.parse.etx_mismatch`, `frame.parse.checksum_mismatch`, `frame.parse.max_size_exceeded`, `frame.buffer.max_streams_exceeded`, `frame.buffer.incomplete_on_stop`).
  - 출력 메시지는 소스 노드 규약(`raw` + `data` 키)을 그대로 따르며, 업스트림 메타데이터 보존 및 `frame.index`, `frame.framer_type`, `frame.stream_key` 메타데이터 추가.
  - 시리얼 에이전트 `framing=frame` 경로와의 완전 동등성을 Parity 테스트(`framer_parity_test.go`)로 검증.
  - Registry 팩토리(`framer_factory.go`)를 통한 빌트인 노드 등록 및 옵션 파싱.

- **에이전트 활성화/비활성화 기능** (SPEC-AGENT-005)
  - `internal/agent/config.go`: `AgentConfig.Enabled *bool` 필드와 `(*AgentConfig).IsEnabled() bool` 메서드 추가. `pkg/flow/node.go`의 `NodeDef.Enabled` 패턴을 재사용하며, `nil` 은 기본값 `true` 로 해석되어 하위 호환성을 보장한다.
  - `internal/agent/serialize.go`: JSON/YAML 직렬화에 `enabled` 필드 추가 (`omitempty`). 기존 저장 데이터는 마이그레이션 없이 그대로 사용 가능하다.
  - `cmd/xflowd/main.go`: 저장소 복원 로직을 테스트 가능한 `restoreAgents` 헬퍼로 추출하고, disabled 상태의 에이전트는 매니저에 등록(Create)만 하고 자동 시작(Start)을 건너뛰도록 변경. 건너뛴 경우 INFO 레벨로 "자동 시작 건너뜀 (disabled)" 로그를 남긴다.
  - `internal/api/handler/agent.go`: `POST /agents/{id}/enable`, `POST /agents/{id}/disable` 엔드포인트 추가. `AgentInfo` 응답에 `enabled: bool` 필드를 항상 포함한다.
  - `internal/api/service/agent_adapter.go`: `EnableAgent`/`DisableAgent` 서비스 메서드와 영속화 실패 시 in-memory 롤백을 수행하는 `setAgentEnabled` 공통 헬퍼 구현. Disable 은 런타임 Stop 을 호출하지 않으며, Enable 은 정지된 에이전트를 자동 Start 하지 않는다 (R3.7, R3.8).
  - `internal/engine/errors.go`: `ErrAgentDisabled` sentinel error 추가.
  - `internal/engine/agent_validation.go`: `validateAgentRefs` 가 플로우 노드가 참조하는 에이전트의 활성화 상태를 검증한다. 비활성화된 에이전트를 참조하는 플로우는 `DeployFlow` 시점에 거부되며, 다수의 검증 실패는 `errors.Join` 으로 한 번에 보고된다. 에러 메시지에는 에이전트 ID, 노드 이름, Enable API 안내가 포함된다.
  - `web/src/types/agent.ts`: `AgentInfo.enabled?: boolean` 필드 추가.
  - `web/src/services/api/agentService.ts`: `enableAgent(id)`, `disableAgent(id)` API 클라이언트 함수 추가.
  - `web/src/hooks/useAgent.ts`: `useEnableAgent`, `useDisableAgent` React Query mutation hook 추가. 성공 시 `['agents']` 및 상세 쿼리 캐시를 무효화한다.
  - `web/src/pages/agents/AgentEnabledBadge.tsx`: 비활성화된 에이전트를 시각적으로 구분하는 회색 배지 컴포넌트 추가. `enabled !== false` 인 경우 렌더링하지 않아 기존 UI 에 노이즈를 주지 않는다.
  - `web/src/pages/agents/AgentActionButtons.tsx`: Enable/Disable 토글 버튼(Power/PowerOff 아이콘) 추가. 비활성화된 에이전트의 수동 Start 시 사용자에게 일시 시작임을 안내하는 확인 다이얼로그 표시.
  - `web/src/pages/agents/AgentListPage.tsx`: 에이전트 이름 옆에 `AgentEnabledBadge` 표시.

- **통합 디바이스 관리 시스템** (SPEC-DEVICE-001)
  - `internal/device/`: 프로토콜 무관 통합 Device/ControllableDevice 인터페이스, DeviceState, CommandSpec, DeviceProvider, DeviceFilter, DeviceMetadata 모델
  - `internal/device/registry.go`: 중앙 DeviceRegistry - RegisterProvider/UnregisterProvider, 필터링, 동시성 안전 설계
  - `internal/device/adapter/nasa.go`: NASADevice를 통합 Device 인터페이스로 래핑하는 어댑터 (ControllableDevice 지원)
  - `internal/agent/samsung/provider.go`: NASAAgent에 DeviceProvider 인터페이스 구현
  - `internal/api/handler/device.go`: 디바이스 REST API 6개 엔드포인트 (목록/상세/실행/메타데이터/명령/상태)
  - `internal/agent/manager.go`: 에이전트 시작/중지 시 DeviceProvider 자동 등록/해제 라이프사이클 훅
  - `internal/api/ws/event_publisher.go`: WebSocket 기반 실시간 디바이스 상태 변경 알림
  - `web/src/pages/devices/DeviceListPage.tsx`: react-grid-layout 기반 디바이스 목록 대시보드 (카드, 필터, 검색, 추가)
  - `web/src/pages/devices/DeviceDetailPanel.tsx`: 디바이스 상세 패널 (상태/속성, CommandSpec 동적 제어 UI, 리모컨)
  - `web/src/pages/agents/AgentDetailPanel.tsx`: 에이전트별 디바이스 CRUD 탭 (추가/제거, 소스 배지)
  - `web/src/hooks/useWebSocket.ts`: WebSocket 연동 실시간 상태 업데이트

### 변경

- **버킷 타임스탬프 벽시계 경계 정렬** (SPEC-STORE-003): `bucketStart = floor(tsMs / intervalMs) * intervalMs` (epoch zero 기준). 1m → 초=0, 5m → 분 0/5/10..., 1h → 분=초=0. TSDB 엔진은 이미 정렬되어 있어 변경 없음.
- **저장소 탭 행별 액션을 마지막 "액션" 컬럼으로 분리**: 타입 컬럼은 정적/동적 배지만, 마지막 컬럼에 [정적변환] [초기화] 버튼 모음
- **전체 초기화 버튼 색상 중립화**: 빨간 톤 → 다른 헤더 버튼과 동일 (destructive 의도는 ConfirmDialog danger variant 가 담당)
- **StoreKeysEditor 키:태그 컬럼 비율 1:3**: `table-fixed` + `<colgroup>` 25%/75%
- **데이터 뷰어 시리즈 multi-select 가시 영역 확장**: `max-h-40 → max-h-[40vh]` (모달 95vh 활용)
- **`UserStoreAgent.Configure` runtime 정책 반영** (SPEC-STORE-003): 이전에는 `agentConfig` 만 갱신하고 inner store 에 미반영 → 정책 필드(`allow_dynamic_keys`, `staticKeys`) 를 runtime 적용 (운영 필드는 restart 필요 유지)
- **`NodeStoreAdapter` lazy resolver 패턴**: 생성 시점 store 스냅샷 → 매 호출마다 resolver 함수로 현재 inner 조회. 에이전트 재시작 후에도 플로우 노드가 재연동 없이 자동으로 새 inner 사용
- `internal/agent/samsung/config.go`: NASA 에이전트 `device_addresses` 설정을 선택 사항으로 변경 (기존: 필수)
- `internal/agent/samsung/agent.go`: processAddDevice/processRemoveDevice에서 req.Params 폴백 읽기 추가
- `web/src/config/agentSchemas.ts`: samsung-nasa 에이전트 스키마에서 device_addresses 필드 제거
- `web/src/config/nodeSchemas.ts`: framer 노드 스키마 추가 (framing_mode, stx, etx, checksum, length_size, max_frame_size, fixed_size, stream_key_metadata, max_streams, stream_idle_timeout 설정)

### 수정

- **readOnly 모드에서 설정 값 가시성 복원** (FormField + StoreKeysEditor + property 편집기 8종)
  - `disabled={readOnly}` → `readOnly={readOnly}` (text/number/textarea)
  - readOnlyClass 단순화: `bg-(--color-bg-elevated)` 토큰 사용
  - 영향: TriggerScheduleEditor, BridgeHttp/Mqtt/ModbusConfig, KeyValueMapEditor, RegisterMapEditor, StringListEditor, TransformPipelineEditor, FormField, StoreKeysEditor
- **정적 키 태그 입력 자동 commit on blur** (SPEC-STORE-003): TagChipsEditor의 keyInput/valInput 이 폼 외부 클릭 시 자동으로 chip 으로 commit. 사용자가 "추가" 버튼 누르지 않고 저장 클릭해도 태그 보존.
- **FormField boolean 기본값 미표시** (SPEC-WEB-005): `value === undefined` 일 때 `field.default` 를 반영하도록 수정. SPEC-STORE-003 의 `allow_dynamic_keys` (default true) 가 기존 config 에 없을 때 unchecked 로 잘못 표시되던 문제 해결.
- **ETX 필드 선택 사항 처리** (`pkg/framing`): `frame` 모드에서 ETX가 빈 값일 때 ETX 검증을 건너뛰도록 수정. ETX 없는 프레임 프로토콜 지원.
- **LengthSize 기본값 보정** (`pkg/framing`): `length_prefix` 모드에서 `length_size` 미지정 시 기본값 2를 적용하도록 수정. 이전에는 0으로 해석되어 프레이밍이 실패함.
- **configInt 문자열 처리** (`internal/node/framer_factory.go`): YAML/JSON에서 정수 설정이 문자열로 전달되는 경우를 처리. `strconv.Atoi` 폴백으로 `"2"` → `2` 변환 지원.
