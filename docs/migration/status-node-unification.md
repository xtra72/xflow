# status 노드 통일 마이그레이션 (v0.18.26)

LG / Samsung / Century 의 `*_hvacr01_status` 노드 (및 통합 `*_hvacr01` 노드) 가 LG ICP-01 의 inactivity-fallback 모델로 통일되면서 발생한 노드 config schema 변경에 대한 운영자 마이그레이션 가이드.

대상 버전: v0.18.25 이전 → v0.18.26 이후

대상 노드:

- `lg_hvacr01_status` / `lg_hvacr01`
- `samsung_hvacr01_status` / `samsung_hvacr01`
- `century_hvacr01_status` / `century_hvacr01`

영향 받지 않는 노드:

- `lg_hvacr01_control` / `samsung_hvacr01_control` / `century_hvacr01_control` (control 노드는 항상 Process(msg) 기반이며 polling 동작이 없었다).
- Samsung HVACR-01 agent (`type: samsung_hvacr01`) 자체의 `poll_interval` config 는 NASA 디바이스 양방향 폴링에 사용되는 별개 설정으로 보존된다.

## 핵심 변화 요약

| 항목 | 이전 | 이후 (v0.18.26) |
|------|------|----------------|
| 동작 모델 (Samsung / Century) | ticker 기반 polling (`poll_interval` 마다 `get_recent` / `get_all_states` 호출) | inactivity-fallback (FrameNotifyCh 즉시 emit + 무수신 시 `request_state`) |
| 동작 모델 (LG) | 이미 inactivity-fallback (v0.18.24+) | 변경 없음 (LG 가 표준이 됨) |
| 어드레싱 (단일 디바이스) | Samsung 노드 `device_id` 또는 `device_address` config | 모든 노드 `group_id` + `unit_id` advanced config (단, 제어 / 단일 조회는 payload override 우선) |
| 출력 metadata | `unit_id`, `slot_num` 키 포함 가능 (옵션) | `unit_id` / `slot_num` 키 영구 제거 (`device_id` UUID 와 어드레싱 필드로 대체) |

## 1. Samsung 마이그레이션

### 1-1. status / 통합 노드의 polling 제거

기존 flow yaml 의 `samsung_hvacr01_status` 또는 `samsung_hvacr01` 노드에서 다음 필드들을 **제거**한다:

- `device_id` (config 레벨)
- `device_address`
- `poll_interval`
- `poll_command`
- `include_raw`
- `emit_unit_id` / `emit_slot_num` (emit_metadata 옵션)

다음 필드들을 **추가**한다:

- `inactivity_timeout` (선택, 기본 `"90s"`, 최소 `5s`)
- `group_id` (선택, NASA addr byte 1 hex, `"00"`–`"0F"`. 비어있으면 모든 group)
- `unit_id` (선택, NASA addr byte 2 hex `"00"`–`"3F"` 또는 전체 dotted/compact 주소 `"10.0F.00"` / `"100F00"`. 비어있으면 모든 unit)

before:

```yaml
- name: "samsung-status"
  type: "samsung_hvacr01_status"
  config:
    agent_ref: "samsung-hvacr01-agent"
    device_id: "living-room"
    poll_interval: "10s"
    timeout: "5s"
    include_raw: false
```

after:

```yaml
- name: "samsung-status"
  type: "samsung_hvacr01_status"
  config:
    agent_ref: "samsung-hvacr01-agent"
    inactivity_timeout: "90s"
    timeout: "5s"
    # 단일 디바이스만 처리하려면 advanced 어드레싱 (선택):
    # group_id: "00"   # NASA addr byte 1 (외기 인덱스)
    # unit_id:  "00"   # NASA addr byte 2 (실내기 인덱스)
```

### 1-2. control 노드 (변경 없음)

`samsung_hvacr01_control` 노드 config 는 그대로 유지된다. 단일 디바이스 제어는 **payload-level override** 로 지정한다:

```json
{ "device_id": "living-room", "power": true, "mode": "cool", "temperature": 24 }
```

또는 NASA 어드레싱이 명확한 경우 `unit_id` 사용 (우선순위 더 높음):

```json
{ "unit_id": "10.0F.00", "command": "set_power", "params": { "power": true } }
```

## 2. Century 마이그레이션

### 2-1. status / 통합 노드의 polling 제거

기존 flow yaml 의 `century_hvacr01_status` 또는 `century_hvacr01` 노드에서 다음 필드들을 **제거**한다:

- `poll_interval`
- `poll_command`
- `recent_count`
- `emit_unit_id` / `emit_slot_num`

다음 필드들을 **추가**한다:

- `inactivity_timeout` (선택, 기본 `"90s"`)
- `unit_id` (선택, Century `sub_dev_id` hex, 예: `"3B"`)
- `group_id` (선택, Century 에서는 미사용; schema parity 유지용)

**유지**: `emit_raw_frames` (raw frame 아카이브 모드 — 활성화 시 inactivity / 어드레싱 우회하여 ring buffer 의 모든 frame emit).

before:

```yaml
- name: "century-status"
  type: "century_hvacr01_status"
  config:
    agent_ref: "century-living-room"
    poll_interval: "100ms"
    poll_command: "drain"
    recent_count: 10
    batch_size: 32
```

after:

```yaml
- name: "century-status"
  type: "century_hvacr01_status"
  config:
    agent_ref: "century-living-room"
    inactivity_timeout: "90s"
    batch_size: 32
    # 단일 디바이스만 처리하려면:
    # unit_id: "3B"   # Century sub_dev_id (hex)
```

### 2-2. raw frame 노드 (변경 없음)

`century_hvacr01_status` 노드에 `emit_raw_frames: true` 를 설정하면 raw frame 아카이브 모드로 동작한다 (CRC 불일치 / payload prefix 위반 frame 도 emit). 이 모드는 inactivity / 어드레싱 필터를 우회한다.

## 3. LG 마이그레이션 (additive only)

LG ICP-01 노드는 v0.18.24 부터 이미 inactivity-fallback 모델을 사용 중이었으므로 **동작 변경은 없다**. 다음의 추가 옵션을 활용할 수 있다 (선택):

- `unit_id` (advanced): STX byte hex 필터 (`"58"` ODU / `"81"`–`"BF"` IDU 64 units). 빈 값이면 모든 frame 처리.
- `group_id`: LG ICP-01 에서는 사용하지 않음 (schema parity 유지용 필드).

다음의 deprecated 옵션은 v0.18.24 부터 이미 no-op 였으며, 호환을 위해 계속 수용된다 (제거 시점 미정):

- `poll_interval`, `poll_command`, `recent_count`

### 출력 metadata 정리 (전 노드 공통)

`emit_unit_id` / `emit_slot_num` 옵션은 LG / Samsung / Century 노드 모두에서 v0.18.26 에서 제거되었다. flow yaml 의 `emit_metadata.emit_unit_id` 또는 평탄 `emit_unit_id` / `emit_slot_num` 키는 파싱 시 무시된다.

downstream 코드 / 쿼리에서 `$.metadata.unit_id` 또는 `$.metadata.slot_num` 을 사용하는 경우 다음 중 하나로 마이그레이션한다:

- `$.metadata.device_id` (UUID) + DeviceRegistry 조회 (영구 ID 가 필요한 경우)
- 노드 config 의 어드레싱 필드 (`unit_id`, `group_id`) 로 단일 디바이스 노드 인스턴스 분리 (프로토콜 식별자가 필요한 경우)

## 4. 동작 모델 비교

### Samsung / Century: ticker → inactivity 모델 전환

이전 (ticker polling):

- 노드가 `poll_interval` 주기로 에이전트의 `get_recent_states` / `get_all_states` / `drain` 명령을 호출
- 콘텐츠 dedup (state hash) 로 중복 emit 억제
- bus silent 상태에서도 주기적 polling 수행

이후 (inactivity-fallback):

- 노드가 에이전트의 `FrameNotifyCh` 를 구독 — 새 frame 도착 시 즉시 `drainNewFrames` 호출
- `inactivity_timeout` 동안 frame 신호가 없으면 `request_state` 명령 발송 (회선 silent 상태에서도 주기적 상태 확보)
- 어드레싱 (`unit_id` / `group_id`) 설정 시 매칭 frame 만 emit + `request_state` 의 target 으로 사용

### 데이터 흐름 영향

| 시나리오 | 이전 (ticker) | 이후 (inactivity) |
|---------|--------------|------------------|
| 새 frame 도착 | poll_interval 다음 주기에 emit (최대 poll_interval 지연) | 즉시 emit (FrameNotifyCh 신호) |
| 회선 silent 상태 | poll_interval 마다 명령 발송 (busy network) | inactivity_timeout 1회 (network friendly) |
| 동일 상태 반복 | content dedup 으로 silent | drainNewFrames 가 신규 frame 만 emit (자연스러운 dedup) |
| 어드레싱 (단일 디바이스) | node config `device_id` / `device_address` | `unit_id` + `group_id` (advanced) 또는 payload override |

## 5. 검증 절차

마이그레이션 완료 후 다음을 확인한다:

1. `xflow flow validate <flow-yaml>` — 새 schema 로 노드 config 가 파싱되는지 확인.
2. flow deploy 후 `xflow flow status <flow-id>` 의 메시지 카운터 — frame 수신 / emit 비율이 이전과 비슷한지 확인.
3. downstream consumer (MQTT subscriber 등) 가 `metadata.device_id` (UUID) 로 디바이스를 식별하는지 확인 — `metadata.unit_id` / `metadata.slot_num` 의존성 제거.
4. 회선 silent 시나리오 (HVAC 디바이스 전원 OFF) — inactivity_timeout 만료 후 `request_state` 가 발송되는지 로그에서 확인 (`agent 의 processRequestState 호출`).

## 6. 참고

- CHANGELOG: `CHANGELOG.md` — `[Unreleased]` → "status 노드 3종 통일 (LG inactivity 모델) + 어드레싱 + 메타데이터 정리" 섹션.
- SPEC: `.moai/specs/SPEC-LG-HVACR-001/spec.md`, `.moai/specs/SPEC-SAMSUNG-HVACR-001/spec.md`, `.moai/specs/SPEC-CENTURY-HVACR-001/spec.md` — 각 SPEC 의 변경 이력 v0.18.26 / v1.18.26 항목.
- 예제: `examples/flows/samsung_hvacr01-status-node.yaml`, `examples/flows/samsung_hvacr01-combined-node.yaml`, `examples/flows/century_hvacr01-status-flow.yaml` — 새 schema 의 reference yaml.
