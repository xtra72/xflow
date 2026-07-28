---
id: SPEC-HVACR-CONNSTATE-001
title: Device Connection-State Reporting for Samsung/LG HVACR Agents
status: Deprecated
priority: High
created: 2026-07-10
deprecated: 2026-07-13
lifecycle: spec-anchored
related:
  - SPEC-SAMSUNG-HVACR-001
  - SPEC-LGAP-001
  - SPEC-LG-HVACR-001
  - SPEC-MESSAGE-TYPE-001
  - SPEC-DEVICE-IDENTITY-001
---

> **DEPRECATED (2026-07-13).** 별도 `device_connection.<trigger>` 메시지 타입은 폐지되었다.
> 연결 상태는 `device_state` 스트림으로 **일원화**되었다(century 에이전트의 단일 스트림 모델과 정렬).
> `device_state` 의 `state` 그룹이 `online` 과 함께 `error_count` / `offline_threshold` /
> `transport_connected` 를 싣고, online↔offline 전이 시 `device_state` change 로, 주기 보고는
> `report_interval` 로 방출된다. 폐지된 요소: `device_connection.*` 메시지, `connection_report_interval`,
> `startup_probe_timeout`, `trigger="initial"`(startup probe baseline). 아래 원문은 역사적 기록으로만
>보존한다.

# SPEC-HVACR-CONNSTATE-001: Device Connection-State Reporting for Samsung/LG HVACR Agents

## 1. 배경 (Context)

xflow의 HVACR 에이전트(Samsung / LG)는 각 device의 **동작 상태(operating state)**를 주기적으로 그리고 변경 시점에 `device_state` 메시지로 방출한다. 이 메시지는 냉난방 설정, 운전 모드 등 device가 "무엇을 하고 있는지"를 담는 데 최적화되어 있다.

그러나 운영 현장에서 실제로 가장 중요한 관제 신호 중 하나인 **연결 상태(connection state, online/offline)**는 현재 1급(first-class) 데이터로 취급되지 않는다. 연결 상태는 device 구조체에 저장되어 있고 일부 전이 이벤트(`device_offline`, `device_online`)가 존재하지만, 일관되고 완결된 리포팅 경로가 없다.

### 기존 구현 사실 (검증됨)

- 등록된 device는 `Online bool`, `LastSeen time.Time`, `ErrorCount int` 필드를 가진다 (`internal/agent/lg/device.go`의 `NasaDevice` / `LGAPDevice`).
- Offline 전이는 `ErrorCount >= OfflineThreshold`에서 발생한다.
  - Samsung: `incrementErrorCount` (`agent.go:1322-1345`)에서 `device_offline` 방출.
  - LGAP: `agent.go:1405-1407`.
- Online 전이는 LGAP `handleResponse`가 `Online=true`로 설정하고, 직전이 offline일 때 `device_online`을 방출한다 (`agent.go:1095-1106`).
- Transport 단절 시 일괄 offline 처리: LGAP `setAllDevicesOffline()` (`agent.go:1448-1454`), 재접속 경로(`agent.go:1196`)에서 호출.
- Transport 수준 상태: `TransportConnected()` → `a.transport.Available()` (Samsung `agent.go:79`, LGAP `agent.go:73`), `get_stats`의 `transport_connected`로 노출.
- 설정 노브: `offline_threshold` → `OfflineThreshold` (기본 3).
- 기존 주기 루프:
  - Samsung: `notifyTicker`(`agent.go:33`), `Start()`(`agent.go:244-249`)에서 `NotifyInterval>0`일 때 기동, `notifyLoop`(`agent.go:259`), `emitPeriodicReport`(`agent.go:278`).
  - LGAP: `go a.notifyLoop()`(`agent.go:206-207`), `emitPeriodicReport`(`agent.go:232`).
  - YAML 키: `report_interval` (duration string) → `NotifyInterval time.Duration`. Legacy alias `notify_interval`은 hard parse error. 기본값: samsung 60s, lg_hvacr01 60s, lgap 미설정(0 = 비활성).
- 기존 payload 스키마 (`device_state`): `{ unit_id, device_id, trigger, state, metadata, last_seen_ms }`, `trigger ∈ {"change","report"}`. 최상위 `type` 필드 없음. 스키마 정체성은 `metadata.message_type = "device_state.<trigger>"`로 표현.
- Timestamp: epoch milliseconds int64 (`.UnixMilli()`). `trigger=="report"`일 때 `lg_hvacr02`는 eventTime을 `time.Now()`로 덮어씀 (`lg_hvacr02_agent.go:2067-2077`) — 이것이 주기 리포트의 canonical 규칙.

## 2. 문제 정의 (Problem Statement)

1. **연결 상태가 1급 필드가 아니다.** 주기 `device_state` 리포트 payload는 동작 상태 중심이며, connection state는 별도 필드로 노출되지 않는다.
2. **Offline device가 주기 리포트에서 조용히 누락된다.** Samsung `emitPeriodicReport`는 `AllCoreObserved()`가 true인 device에 대해서만 방출한다. 즉 통신이 끊긴(offline) device는 주기 리포트에서 배제되는데, 연결 상태 관제가 **가장 알고 싶어 하는 대상이 바로 그 offline device**다. 이는 근본적 gap이다.
3. **Samsung은 online 복구 이벤트가 없다.** Samsung은 `device_offline`을 방출하지만 대칭되는 online-recovery 이벤트가 없다 (LGAP은 둘 다 존재). 관제 측은 device가 언제 복구되었는지 알 수 없다.
4. **Passive capture 에이전트의 범위 불명확.** `lg_hvacr01` / `lg_hvacr02`는 device map이 없는 passive capture 에이전트다. 이들에 대한 connection-state 리포팅 포함 여부를 명시적으로 결정·문서화해야 한다.

## 3. 목표 (Goals)

- 등록된 각 device의 연결 상태를 **개별 메시지**로, **설정 가능한 주기**로, 그리고 **상태 변경 즉시** 방출한다.
- Offline device를 포함한 **전체 등록 device**를 주기 리포트에 포함한다.
- Samsung / LGAP 전반에서 online/offline 전이 이벤트를 **대칭적으로** 방출한다.
- 기존 `device_state` 봉투 규약과 일관된 payload 스키마를 정의한다.

## 4. 설계 결정 (Design Decisions)

### 4.1 신규 message_type: `device_connection.<trigger>` (device_state 확장 아님)

**결정: `device_state`를 확장하지 않고 신규 message_type `device_connection.<trigger>`를 도입한다.**

근거:
- **관심사 분리.** 동작 상태(operating state)와 연결 상태(connection state)는 수명주기와 방출 조건이 다르다. 동작 상태는 `AllCoreObserved()` 게이트(모든 코어 필드 관측 완료)에 묶여 있으나, 연결 상태는 그 게이트와 무관하게 — 오히려 게이트가 실패하는 offline 상황에서 — 반드시 방출되어야 한다.
- **게이트 결합 회피.** `device_state`를 확장하면 offline device를 포함하기 위해 Samsung의 `AllCoreObserved()` 게이트 의미론을 변경해야 하며, 이는 동작 상태 리포트의 기존 계약을 깨뜨린다. 별도 message_type은 항상 전체 device를 대상으로 방출하면서 동작 상태 리포트를 그대로 보존한다.
- **다운스트림 선택 구독.** bridge/node 다운스트림은 `metadata.message_type` 접두(`device_connection.` vs `device_state.`)로 두 스트림을 독립적으로 라우팅·구독할 수 있다.
- **봉투 일관성 유지.** 신규 타입도 동일한 봉투(`unit_id`, `device_id`, `trigger`, `metadata.message_type`, epoch-ms)를 사용하므로 다운스트림 파서 재사용이 가능하다.

### 4.2 신규 설정 키: `connection_report_interval` (기존 `report_interval` 재사용 아님)

**결정: 기존 `report_interval`을 재사용하지 않고 별도 키 `connection_report_interval` (duration string)를 도입한다.**

근거:
- **독립적 활성화가 필수.** LGAP은 `report_interval` 기본값이 0(비활성)이다. 연결 상태 관제는 동작 상태 리포팅이 꺼져 있어도 반드시 동작해야 한다. 키를 공유하면 동작 상태 리포팅을 켜야만 연결 상태 리포팅이 켜지는 원치 않는 결합이 생긴다.
- **독립적 cadence.** 연결 상태는 동작 상태보다 성기게(또는 촘촘하게) 보고할 수 있으며, 별도 키가 이를 허용한다.
- **하위 호환.** 기존 `report_interval` 소비자는 영향받지 않는다.
- 기본값: `connection_report_interval` 기본 `60s` (Samsung/LGAP 공통). 값이 `0` 또는 미설정이면 주기 연결 리포트는 비활성화되나, **change 이벤트와 startup 이벤트는 항상 방출된다**(§4.3, §4.6).
- `report_interval`과 동일한 파싱 규율을 따른다: duration string, 잘못된 legacy alias는 hard parse error로 처리한다(조용한 무시 금지).
- 보조 키 `startup_probe_timeout` (duration string): startup probe(§4.6)의 bounded timeout. 최종 의미론(OQ-A 결정):
  - **파생 기본값**: 미설정이면 `poll_interval`에서 파생하되 절대 상한 30s로 캡한다 → `min(2 × poll_interval, 30s)`.
  - **명시 override는 캡하지 않음**: YAML에서 `startup_probe_timeout`을 명시하면 그 값을 그대로 존중한다(30s 상한 미적용). 근거: 30s 캡은 파생 기본값의 안전장치일 뿐이며, 운영자가 의도적으로 더 긴 값을 설정한 경우(느린 대형 버스 등) 그 의도를 덮어써서는 안 된다. 캡은 "설정을 잊은" 경우를 보호하고, 명시 값은 "설정을 아는" 경우이므로 구분한다.
  - **`poll_interval`이 0/미설정인 경우 fallback**: 파생 근거가 없으므로 절대 기본값 `10s`를 사용한다(`min` 공식 대신 상수 fallback). 이 fallback 역시 명시 override가 있으면 그 값이 우선한다.
  - 이 키도 잘못된 alias는 hard parse error로 처리한다.

### 4.3 Change 이벤트는 tick과 완전히 독립

연결 상태가 online↔offline로 전이되는 순간 `device_connection.change`를 즉시 방출한다. 이는 `connection_report_interval` 값(0 포함)과 무관하다. 즉 주기 리포트를 꺼도 전이 이벤트는 항상 흐른다.

### 4.4 개별 메시지 (device 단위)

각 device는 하나의 메시지로 방출된다. 여러 device를 배열로 배칭하지 않는다. 이는 기존 `device_state` per-device 방출 관행과 일치한다.

### 4.6 Startup Probe와 `trigger="initial"` (신규, Q3 결정 반영)

**결정: 에이전트 Start 시 각 등록 device에 대해 1회성 startup probe로 초기 연결 상태를 확정하고, 그 결과를 device당 `device_connection.initial` 메시지로 방출한다. 연결 상태는 `online` boolean 단일 필드(`true`=online, `false`=offline)로 전송하며 `unknown` 상태를 도입하지 않는다. Probe는 비동기(background)로 실행된다(OQ-B 결정).**

사용자 결정 원문 의도: "기동 직후 확인해서 전송. 확인이 안될 경우, offline."

#### 4.6.1 `trigger` 값 결정: 신규 세 번째 값 `"initial"`

기존 규약은 `trigger ∈ {"change","report"}`이나, startup 방출은 이 둘 중 어느 것도 정확히 아니다.

- `"change"`가 아닌 이유: startup은 실제 필드 전이가 아니라 baseline 확정이다. 재시작 시마다 이미 offline이던 device를 `change`(offline)로 방출하면 다운스트림이 "device가 방금 끊겼다"는 **거짓 전이 알람**을 재시작마다 발생시킨다.
- `"report"`가 아닌 이유: startup은 주기 tick 산출물이 아니며 `connection_report_interval`(0 포함)과 무관하게 항상 1회 발생한다.

**따라서 신규 세 번째 값 `"initial"`을 도입한다.** 이로써 `trigger ∈ {"change","report","initial"}`, `metadata.message_type ∈ {device_connection.change, device_connection.report, device_connection.initial}`.

다운스트림 영향(명시): `trigger` 문자열 집합을 소비하는 파서는 세 번째 값 `"initial"`을 반드시 허용(tolerate)해야 한다. 이점은 재시작 baseline을 정상 상태 churn과 구분할 수 있어, 소비자가 에이전트 재시작 시 per-device baseline을 깨끗하게 리셋할 수 있다는 점이다.

#### 4.6.2 Probe를 각 에이전트 기존 메커니즘에 매핑 (신규 프로토콜 조작 금지)

신규 프로토콜 operation을 발명하지 않는다. Probe는 "**bounded timeout으로 await하는 첫 poll 사이클**"로 표현한다.

- Samsung (active-poll NASA 에이전트, `poll_interval`): probe = 각 등록 device에 대한 **첫 poll 사이클**을, `startup_probe_timeout` 내에서 await. timeout 내 유효 응답 → `online`, 아니면 `offline`.
- LGAP (active master/slave polling 에이전트): probe = 각 zone/device에 대한 **첫 master poll 라운드**를, 동일한 `startup_probe_timeout` 내에서 await. 결과 판정은 동일.
- `startup_probe_timeout` 미설정 시 `poll_interval`에서 파생(`2 × poll_interval`).

#### 4.6.3 `offline_threshold` 상호작용: startup은 threshold를 **우회**한다 (의도된 비대칭)

- 정상 상태(steady-state) 규칙: device는 `ErrorCount >= OfflineThreshold`에 도달해야만 offline로 전이된다.
- **Startup 규칙(비대칭): 단 1회의 probe 실패(또는 timeout, 또는 transport 미가용) → 즉시 초기 상태 `offline`.** `OfflineThreshold`를 소진하지 않는다. 이는 사용자 의도("확인이 안될 경우, offline")를 직접 반영한다.
- 근거: 기동 시점에는 이전 baseline이 없어 threshold 누적의 근거가 없다. 초기 상태는 결정론적으로 확정되어야 한다.
- 후속 정상 상태 accounting은 fresh하게 시작한다. startup에서 `offline`로 확정된 device는 `ErrorCount`를 threshold로 미리 채우지 않으며, **첫 성공 poll이 도착하는 즉시 online 전이(E3, `device_connection.change` online)를 방출**한다. startup에서 `online`으로 확정된 device는 이후 실패가 `ErrorCount`에 누적되어 threshold에서만 offline로 전이된다.

#### 4.6.4 Start 시점 transport 미연결

Start가 완료되었으나 transport가 아직 연결되지 않은 경우(예: reconnect loop 진행 중), probe는 성공할 수 없으므로 **모든 등록 device는 초기 상태 `offline`을 방출**한다.

#### 4.6.5 비동기 실행과 순서 보장 (OQ-B 결정)

**결정: `Start`는 probe 완료를 기다리지 않고 즉시 반환한다. startup probe는 background goroutine에서 실행된다.** 근거: 느린 버스나 대규모 device 팜이 에이전트 기동을 blocking해서는 안 된다.

이로 인해 발생하는 순서 속성을 명시한다:

- **[수용된 비보장] device 간 `initial` 순서는 비결정적(non-deterministic)이다.** 각 device의 probe가 독립적으로 완료되므로 device A의 `initial`이 device B의 `initial`보다 먼저 나올지는 보장되지 않는다. **다운스트림 소비자는 device 간 `initial` 메시지 순서에 의존해서는 안 된다.**
- **[HARD 보장] per-device 순서는 반드시 보장된다.** 임의의 device D에 대해, D의 `initial` 메시지는 D의 어떤 `change`/`report` 메시지보다도 **먼저** 방출되어야 한다.
- **보장 유지 방법**: device별로 "initial 방출 완료" 상태 플래그(예: `initialEmitted map[deviceKey]bool` 또는 device 구조체의 bool 필드)를 두고, 해당 device의 probe가 완료되어 `initial`을 방출한 뒤에만 그 device에 대한 `change`/`report` 방출을 허용한다. 구체적 상호작용:
  - **주기 리포트 루프(E5)**: `connection_report_interval` tick이 발생했을 때, 아직 `initial`이 방출되지 않은 device에 대해서는 **`report`를 방출하지 않고 건너뛴다**(그 device는 다음 tick에서, initial 방출 이후에 포함된다). 이는 per-device 순서 보장을 유지하기 위함이다.
  - **change 이벤트(E1~E4)**: startup 창구 동안 `initial` 이전에 전이가 감지되더라도, 해당 device의 `change`는 `initial` 방출 이후로 순서화되어야 한다. 실무적으로 probe가 곧 그 상태를 확정하므로, `initial`이 baseline을 먼저 싣고 이후 실제 전이만 `change`로 흐른다.

#### 4.6.6 `initial`은 프로세스 수명당 device별 1회 (OQ-C 결정)

**결정: 늦은 transport 연결/재연결은 오직 `change` 이벤트만 방출한다. `initial` 재방출은 없다.**

- `trigger="initial"`은 **프로세스 수명(process lifetime) 동안 device당 최대 1회** 방출된다(hard invariant).
- 초기 baseline이 확정된 이후의 모든 연결성 전이는 — transport 미가용 상태로 startup한 뒤 첫 성공 통신을 포함하여, 그리고 이후의 모든 재연결을 포함하여 — 일반 `change` 이벤트(E1/E3)다.
- LGAP `setAllDevicesOffline()`(disconnect 시)과의 상호작용: 이 일괄 전이는 device당 `change`(offline)를 방출하며(E4), `initial`이 아니다.
- 이는 §5.5의 unwanted-behavior 요구(N9)로 강제된다.

### 4.5 lg_hvacr01 / lg_hvacr02: 명시적 범위 밖 (Out of Scope)

**결정: passive capture 에이전트(`lg_hvacr01`, `lg_hvacr02`)는 본 SPEC의 connection-state 리포팅 범위에서 제외한다.**

근거:
- 이들은 device map이 없다(passive capture, no device map). 등록된 device 목록·주소가 없으므로 "device당 개별 메시지"를 생성할 대상 자체가 없다.
- 요청/응답 기반 `ErrorCount`/`OfflineThreshold` 메커니즘이 없다. 프레임을 수동 관찰만 하므로 per-device online/offline 전이를 판정할 근거가 없다.
- 향후 확장 여지(범위 밖): 에이전트 수준의 "capture liveness"(프레임 무음 타임아웃 기반) 신호를 별도 SPEC으로 정의할 수 있으나, per-device connection state와는 의미가 다르므로 본 SPEC에 포함하지 않는다.

## 5. 요구사항 (EARS Requirements)

### 5.1 Ubiquitous (항상 활성)

- **U1**: The system **shall** emit every connection-state message using the existing `device_state` envelope conventions: top-level `unit_id`, `device_id`, `trigger`, `metadata.message_type`, and epoch-millisecond int64 timestamps.
- **U2**: The system **shall** emit one message **per device** (individual messages), never a batched array of devices.
- **U3**: The system **shall** set `metadata.message_type` to `device_connection.<trigger>` where `<trigger>` is one of `change`, `report`, or `initial`.
- **U4**: The system **shall** represent all timestamps as epoch milliseconds (`int64`, via `.UnixMilli()`).
- **U5**: The system **shall** include the connection state as a single first-class top-level boolean payload field `online` (`true`=online, `false`=offline), unified with the system-wide `online` convention. The prior dual `connected`/`connection_state` fields are removed; there is no `unknown` value.

### 5.2 Event-Driven (WHEN ... THEN)

- **E1**: **When** a device's connection state transitions between `online` and `offline`, **the** agent **shall** emit a `device_connection.change` message immediately, independent of any periodic tick.
- **E2**: **When** a device's `ErrorCount` reaches `OfflineThreshold`, **the** agent **shall** transition the device to `offline` and emit a `device_connection.change` message with `online=false`.
- **E3**: **When** an offline device resumes successful communication, **the** agent **shall** transition the device to `online` and emit a `device_connection.change` message with `online=true`. (This adds the missing symmetric online-recovery event to the Samsung agent.)
- **E4**: **When** the transport disconnects and all devices are bulk-marked offline (e.g. LGAP `setAllDevicesOffline()`), **the** agent **shall** emit an individual `device_connection.change` (offline) message for each affected device.
- **E5**: **When** the `connection_report_interval` tick fires, **the** agent **shall** emit a `device_connection.report` message for **every registered device**.
- **E6**: **When** the agent starts (`Start`), **the** agent **shall** launch the startup probe asynchronously (in a background goroutine) and **shall** return from `Start` without waiting for probe completion. The background probe performs a one-shot determination per registered device — expressed as the first poll cycle awaited within `startup_probe_timeout` — and emits an `device_connection.initial` message per device carrying the determined state.
- **E7**: **When** a startup probe confirms communication within `startup_probe_timeout`, **the** agent **shall** set that device's initial `online` field to `true`; **when** the probe fails, times out, or the transport is unavailable, **the** agent **shall** set the initial `online` field to `false`.
- **E8**: **When** `Start` completes while the transport is not yet connected, **the** agent **shall** emit an initial `offline` message for every registered device.
- **E9**: **When** the agent emits any `change` or `report` message for a device, **the** agent **shall** have already emitted that device's `initial` message first (per-device ordering guarantee). Inter-device ordering of `initial` messages is explicitly not guaranteed.

### 5.3 State-Driven (WHILE / IF ... THEN)

- **S1**: **While** `connection_report_interval > 0`, **the** agent **shall** run the periodic connection-report loop.
- **S2**: **While** a device is `offline`, **the** agent **shall** still include that device in the periodic `device_connection.report` output. The `AllCoreObserved()` operating-state gate **shall not** apply to connection reporting.
- **S3**: **While** `trigger == "report"`, **the** agent **shall** set the periodic event timestamp (`event_ms`) to `time.Now()` at tick time (the canonical periodic-report rule, per `lg_hvacr02_agent.go:2067-2077`).
- **S4**: **While** a device's initial state was determined by a failed startup probe (initial `offline`), **the** agent **shall not** pre-load `ErrorCount` toward `OfflineThreshold`; **the** agent **shall** emit an `online` change (E3) on the first subsequent successful poll.
- **S5**: **While** a device's `initial` message has not yet been emitted (its startup probe is still in flight), **the** agent **shall not** emit a periodic `report` for that device; the periodic loop **shall** skip that device until its `initial` is emitted, including it on a subsequent tick.

### 5.4 Optional (WHERE)

- **O1**: **Where** an agent maintains a device map (Samsung `samsung_hvacr01`, LG `lgap`), **the** agent **shall** provide connection-state reporting per this SPEC.
- **O2**: **Where** transport-level availability is known, **the** agent **shall** include `transport_connected` in the message payload.

### 5.5 Unwanted Behavior (IF undesired, THEN shall / shall not)

- **N1**: **If** code emits a message while holding `a.mu.Lock()`, **then** the agent **shall not** call `a.Name()` or `a.ID()` (which internally take `a.mu.RLock()`); it **shall** read `a.agentConfig.Name` directly. (RWMutex non-reentrancy hard constraint — see §7.1.)
- **N2**: **If** `connection_report_interval` is `0` or unset, **then** the agent **shall not** emit periodic `device_connection.report` messages, but **shall** continue to emit `device_connection.change` events on transitions.
- **N3**: **The** agent **shall not** batch multiple devices into a single array message.
- **N4**: **If** an invalid or legacy alias config key is supplied for the connection interval, **then** parsing **shall** fail as a hard error and **shall not** be silently ignored (consistent with the `notify_interval` → `report_interval` precedent).
- **N5**: **The** agent **shall not** exclude offline devices from periodic reports.
- **N6**: **The** agent **shall not** default a device silently to `offline` at startup without first attempting the bounded startup probe; the initial state **shall** be the determined result of that probe.
- **N7**: **The** system **shall not** introduce an `unknown` connection value; the `online` boolean field **shall** remain two-valued (`true`=online, `false`=offline).
- **N8**: **At** startup, a single failed probe **shall** yield initial `offline` and **shall not** require exhausting `OfflineThreshold` (deliberate asymmetry with the steady-state threshold rule).
- **N9**: **The** agent **shall not** emit more than one `device_connection.initial` message for the same device within one process lifetime; late transport connects and every reconnect **shall** be reported as ordinary `change` events (E1/E3), never as a second `initial`.
- **N10**: **If** `Stop` is called while a startup probe is in flight, **then** the probe goroutine **shall** be joined via the `WaitGroup` (no leak) and **shall not** emit any message after `Stop` has begun.

## 6. 제안 Payload 스키마 (Proposed Payload Schema)

`message_type = "device_connection.<trigger>"`. 기존 `device_state` 봉투와 일관된 필드 명명 및 epoch-ms 규약을 사용한다.

```json
{
  "unit_id": "<string>",
  "device_id": "<string>",
  "trigger": "change | report | initial",
  "online": true,
  "error_count": 0,
  "offline_threshold": 3,
  "transport_connected": true,
  "last_seen_ms": 1752130800000,
  "event_ms": 1752130860000,
  "metadata": {
    "message_type": "device_connection.change"
  }
}
```

필드 의미:

| 필드 | 타입 | 의미 |
| --- | --- | --- |
| `unit_id` | string | 기존 봉투와 동일 (device unit 식별자) |
| `device_id` | string | `effectiveDeviceID`/`ResolveDeviceID`로 해석된 device_id (기존 규약) |
| `trigger` | string | `change`(전이 즉시), `report`(주기 tick), 또는 `initial`(Start 시 startup probe 확정, §4.6). 다운스트림 파서는 세 값을 모두 tolerate해야 한다 |
| `online` | bool | `Online` 필드의 boolean 값. **연결 상태는 이 단일 필드로 전송하며(시스템 전체 `online` 컨벤션과 통일), 이전의 `connected`/`connection_state` 이중 필드는 폐기했다.** `unknown` 값은 없다(`true`=online, `false`=offline) |
| `error_count` | int | 현재 `ErrorCount` |
| `offline_threshold` | int | 적용 중인 `OfflineThreshold` (관제 임계 노출용) |
| `transport_connected` | bool | `TransportConnected()` = `a.transport.Available()` |
| `last_seen_ms` | int64 | device `LastSeen.UnixMilli()` (마지막 성공 통신 시각). startup에서 아직 성공 통신이 없으면 0 또는 미확정일 수 있다 |
| `event_ms` | int64 | **최상위 필드(Q1 결정: metadata로 이동하지 않음)**. 메시지 방출 시각. `trigger=="change"`이면 전이 발생 시각, `trigger=="report"`이면 tick 시점 `time.Now().UnixMilli()`(canonical rule, S3), `trigger=="initial"`이면 probe 확정 시점 `time.Now().UnixMilli()` |

Startup 메시지 예 (`trigger="initial"`, probe 실패 → offline):

```json
{
  "unit_id": "<string>",
  "device_id": "<string>",
  "trigger": "initial",
  "online": false,
  "error_count": 0,
  "offline_threshold": 3,
  "transport_connected": false,
  "last_seen_ms": 0,
  "event_ms": 1752130800000,
  "metadata": {
    "message_type": "device_connection.initial"
  }
}
```

주의:
- 최상위 `type` 필드는 두지 않는다 (기존 `device_state` 규약과 동일하게 스키마 정체성은 `metadata.message_type`로만 표현).
- 에이전트는 topic을 명명하지 않는다. JSON을 bounded `msgCh chan []byte`로 push하며, 다운스트림 bridge/node가 topic 주소 지정을 담당한다.

순서·수명 의미론 (스키마 변경 없음, 소비자 계약 명시):
- `trigger="initial"`은 **프로세스 수명당 device별 정확히 1회** 방출된다(재방출 없음, §4.6.6 / N9).
- **per-device 순서 보장**: 임의 device의 `initial`은 그 device의 첫 `change`/`report`보다 먼저 온다(§4.6.5 / E9).
- **device 간 순서 비보장**: 비동기 probe로 인해 서로 다른 device의 `initial` 도착 순서는 비결정적이므로 소비자는 이에 의존해서는 안 된다.

## 7. 비기능 제약 (Non-Functional Constraints)

### 7.1 [HARD] RWMutex 비재진입 트랩

각 에이전트의 `mu sync.RWMutex`(samsung `agent.go:30`, lgap `agent.go:28`, lg_hvacr01 `:35`, lg_hvacr02 `:36`)는 **재진입 불가**다. `Name()`과 `ID()`는 내부적으로 `a.mu.RLock()`을 취한다. 따라서 `a.mu.Lock()`을 보유한 상태에서 `a.Name()`을 호출하면 **deadlock**이 발생한다.

구현 규칙: lock을 보유한 채 메시지를 구성/방출하는 모든 경로는 반드시 `a.agentConfig.Name`을 직접 읽어야 한다. 선례:
- samsung `agent.go:1676-1680`
- lgap `emitDeviceStateLocked` `agent.go:267-268` / `:271`
- lg_hvacr02 `:2059-2061` / `:2073`

특히 offline 전이(E2)와 bulk offline(E4)은 이미 `a.mu.Lock()` 보유 경로에서 발생하므로 이 트랩에 직접 노출된다. **비동기 startup probe(§4.6.5)는 background goroutine에서 방출하므로 특히 주의한다**: probe goroutine이 `a.mu`를 보유한 채 `initial`을 방출한다면 절대 `a.Name()`/`a.ID()`를 호출하지 말고 반드시 `a.agentConfig.Name`을 직접 사용해야 한다(E6~E8).

### 7.2 [HARD] WaitGroup / goroutine-join 비대칭

- Samsung은 `a.wg`(WaitGroup) + `close(a.stopCh)`로 goroutine 수명을 관리하며, `Stop()`에서 join한다.
- LGAP은 bare goroutine(`go a.notifyLoop()`)을 기동하며 **WaitGroup이 없고 Stop 시 join되지 않는다**.

구현 규칙 (Q4 + OQ-B 결정 반영): 신규 `connection_report_interval` 주기 루프 **및 비동기 startup probe goroutine**을 추가할 때:
- Samsung: 기존 패턴 그대로 — `a.wg.Add(1)` + `defer a.wg.Done()` + `a.stopCh` 관측.
- LGAP: **오직 신규 connection-state goroutine들(주기 루프 + startup probe)에만** `sync.WaitGroup` join-on-Stop 처리를 적용한다(Samsung의 `a.wg` + `close(a.stopCh)` 패턴을 미러링). 즉 새 goroutine은 stop 신호로 정지되고 `Stop()`에서 join되어 goroutine leak이 없어야 한다.
- **[HARD] Stop-중-probe 안전성**: `Stop`이 in-flight probe와 경합해도 (a) probe goroutine은 `WaitGroup`으로 join되어 leak이 없고, (b) `Stop` 시작 이후에는 어떤 메시지도 방출하지 않아야 한다(N10). probe 루프는 방출 전에 stop 신호를 확인하고, stop이 감지되면 방출 없이 종료한다.
- **LGAP의 기존 bare goroutine은 그대로 둔다. 본 SPEC에서 리팩터링하지 않는다.**
- 이로써 LGAP 내부에는 **알려진, 수용된 수명주기 비대칭**이 남는다(신규 goroutine은 join, 기존 루프는 비-join). 이 비대칭의 전면 해소는 후속 리팩터링 SPEC이 올바른 처리 장소다.

### 7.3 성능/부하

- 개별 메시지(per-device)는 대규모 device 팜에서 tick당 N개의 메시지를 생성한다. `msgCh`는 bounded이므로 방출 경로는 채널 포화 시 blocking 여부를 기존 `emitPeriodicReport` 방출 관행과 동일하게 처리해야 한다(신규 blocking semantics 도입 금지).

## 8. 수용 기준 (Acceptance Criteria — Testable)

### AC-1: 설정 가능한 주기 + 기본값
- Given `connection_report_interval: 30s`가 설정된 Samsung 에이전트,
- When 에이전트가 기동되고 30초가 경과하면,
- Then 등록된 각 device에 대해 정확히 하나의 `device_connection.report` 메시지가 방출된다.
- And 키가 미설정이면 기본 `60s`가 적용된다.

### AC-2: interval 0/unset일 때 주기 리포트 비활성 + change는 유지
- Given `connection_report_interval: 0`,
- When 임의 시간이 경과해도,
- Then `device_connection.report`는 방출되지 않는다.
- But When 한 device가 offline로 전이되면,
- Then `device_connection.change` (offline) 메시지가 즉시 방출된다.

### AC-3: change 이벤트는 tick과 독립적으로 즉시 방출
- Given 주기 tick 사이 구간,
- When device의 `ErrorCount`가 `OfflineThreshold`에 도달하면,
- Then 다음 tick을 기다리지 않고 즉시 `device_connection.change` (offline)가 방출된다.

### AC-4: 대칭 online/offline (Samsung 포함)
- Given offline 상태로 전이된 Samsung device,
- When 통신이 복구되면,
- Then `device_connection.change` (online) 메시지가 방출된다. (기존 Samsung에 없던 대칭 이벤트)

### AC-5: startup probe 성공 → 초기 online 방출
- Given 등록된 device에 대해 transport가 연결되어 있고 첫 poll이 `startup_probe_timeout` 내에 유효 응답을 반환,
- When 에이전트가 `Start`되면,
- Then 해당 device에 대해 `trigger="initial"`, `online=true`인 `device_connection.initial` 메시지가 device당 정확히 1개 방출된다.

### AC-6: startup probe 실패 → 초기 offline (offline_threshold 대기 없음)
- Given 등록된 device의 첫 poll이 `startup_probe_timeout` 내에 응답하지 않음(단 1회 실패),
- When 에이전트가 `Start`되면,
- Then 해당 device에 대해 `trigger="initial"`, `online=false`인 메시지가 즉시 방출된다.
- And `OfflineThreshold` 소진을 기다리지 않는다(단일 실패 → 즉시 offline).
- And 이후 첫 성공 poll이 도착하면 `device_connection.change` (online)가 방출된다(S4/E3).

### AC-7: Start 시 transport 미가용 → 전체 device 초기 offline
- Given `Start` 완료 시점에 transport가 아직 연결되지 않음(reconnect loop 진행 중),
- When startup probe가 수행되면,
- Then 모든 등록 device에 대해 `trigger="initial"`, `online=false` 메시지가 device당 1개씩 방출된다.

### AC-8: `Start`는 probe 완료를 기다리지 않고 즉시 반환 (비동기)
- Given 첫 poll이 `startup_probe_timeout`에 가깝게 느리게 응답하는 device를 다수 보유한 에이전트,
- When `Start`가 호출되면,
- Then `Start`는 probe들의 완료를 기다리지 않고 (probe 최대 소요 시간보다 훨씬 짧게) 즉시 반환한다.
- And `initial` 메시지들은 이후 background에서 완료되는 대로 방출된다.

### AC-9: per-device 순서 보장 (initial이 첫 change/report보다 먼저)
- Given 임의의 device D,
- When D에 대한 `change` 또는 `report` 메시지가 방출되면,
- Then 그 시점 이전에 D의 `initial` 메시지가 이미 방출되어 있다.
- And 서로 다른 device 간 `initial` 도착 순서는 검증하지 않는다(비보장으로 명시).
- And 특히 아직 `initial`이 방출되지 않은 device는 주기 tick에서 `report` 대상에서 제외된다(S5).

### AC-10: in-flight probe 중 Stop → leak 없음, Stop 이후 방출 없음
- Given startup probe가 아직 진행 중인 에이전트,
- When `Stop()`이 probe와 경합하여 호출되면,
- Then probe goroutine은 `WaitGroup`으로 join되어 goroutine leak이 없고,
- And `Stop` 시작 이후에는 어떤 메시지(`initial` 포함)도 방출되지 않는다.
- And 이는 `go test -race`(leak detector + 방출 카운트 검증)로 검증 가능하다.

### AC-11: startup 이후 재연결은 `change`, 두 번째 `initial` 없음
- Given transport 미가용으로 startup하여 초기 `offline`을 방출한 device(또는 이후 정상 재연결되는 device),
- When transport가 연결되어 첫 성공 통신이 발생하면,
- Then `device_connection.change` (online)가 방출된다.
- And 그 device에 대해 두 번째 `device_connection.initial`은 프로세스 수명 동안 절대 방출되지 않는다(N9).

### AC-12: 파생 timeout 30s 캡 + 명시 override는 그대로 존중 + poll_interval=0 fallback
- Given `startup_probe_timeout` 미설정,
- When `poll_interval = 20s`이면,
- Then 파생 timeout은 `min(2 × 20s, 30s) = 30s`로 캡된다.
- And `poll_interval = 5s`이면 파생 timeout은 `min(10s, 30s) = 10s`이다.
- And `poll_interval`이 0/미설정이면 fallback 절대 기본값 `10s`가 적용된다.
- But Given `startup_probe_timeout: 60s`가 명시되면,
- Then 30s 캡이 적용되지 않고 `60s`가 그대로 사용된다.

### AC-13: offline device가 주기 리포트에 포함
- Given 등록된 device 3개 중 1개가 offline(`AllCoreObserved()==false`), 모두 `initial` 방출 완료,
- When `connection_report_interval` tick이 발생하면,
- Then 3개 device 모두에 대해 `device_connection.report`가 방출되며, offline device도 `online=false`로 포함된다.

### AC-14: 개별 메시지 (배칭 금지)
- Given 등록된 device N개,
- When 주기 tick이 발생하면,
- Then 정확히 N개의 개별 메시지가 방출되고, 배열로 묶인 단일 메시지는 방출되지 않는다.

### AC-15: bulk offline 시 device당 개별 이벤트
- Given LGAP 에이전트가 M개 device를 online으로 보유,
- When transport가 끊겨 `setAllDevicesOffline()`가 호출되면,
- Then M개의 개별 `device_connection.change` (offline) 메시지가 방출된다(두 번째 `initial` 아님).

### AC-16: report trigger의 event_ms canonical 규칙
- Given `trigger=="report"` 메시지,
- Then `event_ms`는 tick 시점의 `time.Now().UnixMilli()`이고, `last_seen_ms`는 device의 실제 마지막 통신 시각을 유지한다(둘이 서로 다를 수 있다).

### AC-17: 봉투 규약 준수 (trigger 3-값 포함)
- Given 임의의 connection-state 메시지,
- Then payload는 `unit_id`, `device_id`, `trigger`, `metadata.message_type`를 포함하고, `trigger ∈ {change, report, initial}`, `online ∈ {true(online), false(offline)}`이며, 모든 타임스탬프는 epoch-ms int64(최상위 `event_ms` 포함)이고, 최상위 `type` 필드는 없다.

### AC-18: 잘못된 config 키는 hard error
- Given connection interval 또는 `startup_probe_timeout`에 대한 잘못된/legacy alias 키,
- When 설정을 파싱하면,
- Then hard parse error가 발생하고 조용히 무시되지 않는다.

### AC-19: RWMutex deadlock 회귀 방지
- Given offline 전이, startup 비동기 방출, bulk offline(lock 보유 경로)에서 방출되는 메시지,
- When `go test -race ./internal/agent/...`를 실행하면,
- Then deadlock 및 data race 없이 통과한다. (lock 보유 중 `a.Name()`/`a.ID()` 호출 없음)

### AC-20: 신규 goroutine(주기 루프 + probe)은 Stop 시 WaitGroup으로 join
- Given 신규 connection-state goroutine들(주기 루프 + 비동기 startup probe)이 기동된 LGAP/Samsung 에이전트,
- When `Stop()`이 호출되면,
- Then 모든 신규 goroutine은 stop 신호로 정지되고 `WaitGroup`으로 join되어 goroutine leak이 없다.
- And 이는 `go test -race`(예: goroutine 수 검증 또는 leak detector)로 검증 가능하다.
- Note: LGAP 기존 bare goroutine은 본 SPEC 범위 밖이며 그 비대칭은 의도적으로 유지된다.

### AC-21: passive 에이전트 범위 밖
- Given `lg_hvacr01` / `lg_hvacr02` 에이전트,
- Then 이들은 어떤 `device_connection.*` 메시지도 방출하지 않는다(본 SPEC 범위 밖).

## 9. 범위 밖 (Out of Scope)

- `lg_hvacr01` / `lg_hvacr02` passive capture 에이전트의 per-device connection-state 리포팅 (§4.5).
- 에이전트 수준 "capture liveness"(프레임 무음 타임아웃) 신호 정의 — 별도 SPEC 후보.
- 다운스트림 topic 주소 지정/라우팅 규칙 (bridge/node 책임, 에이전트는 topic 미명명).
- `device_state`(동작 상태) 메시지 스키마 변경.
- LGAP goroutine 수명주기의 전면 리팩터링(모든 bare goroutine에 WaitGroup 도입) — 본 SPEC은 **신규 connection-state 루프에만** join 처리를 적용하며(§7.2, Q4), 기존 bare goroutine 리팩터링은 후속 SPEC 범위.
- 신규 프로토콜 operation 설계 — startup probe는 각 에이전트의 기존 첫 poll 사이클을 재사용하며 새 프로토콜 조작을 도입하지 않는다(§4.6.2).
- InfluxDB/TSDB 저장 스키마 및 대시보드 시각화.
- `offline_threshold`의 의미론 변경(기존 값을 그대로 노출만 함).
- device 간 `initial` 순서 보장 — 비동기 probe로 인해 비결정적이며, 이는 의도된 수용된 속성이다(§4.6.5). 순서 결정성이 필요하면 별도 SPEC이 처리 장소.
- transport 재연결 시 `initial` 재확정/재방출 — `change`(E1/E3)로만 처리한다(§4.6.6).

## 10. 열린 질문 (Open Questions)

### 해결됨 (RESOLVED)

1. **[해결] `event_ms` 필드 위치 → 최상위 필드 유지.** `event_ms`는 `last_seen_ms`와 나란히 최상위 필드로 유지한다. 다운스트림 파서는 신규 최상위 필드를 tolerate해야 하며, `metadata`로 이동하지 않는다. (§6 반영)
2. **[해결] `connection_report_interval` 기본값 → 60s 유지.** 기존 `report_interval`과 일관되게 60s를 유지한다. (§4.2 반영)
3. **[해결] 초기 상태 → `unknown` 상태 없음, startup probe 도입.** 연결 상태는 `online` boolean 단일 필드(`true`=online, `false`=offline)로 전송한다. Start 시 각 device에 대해 bounded startup probe(첫 poll 사이클, `startup_probe_timeout`)로 초기 상태를 확정하고 device당 `device_connection.initial`(신규 `trigger="initial"`)로 방출한다. 단일 probe 실패/timeout/transport 미가용 → 즉시 `offline`(threshold 우회). (§4.6, E6~E8, S4, N6~N8, AC-5~AC-7 반영)
4. **[해결] LGAP goroutine 수명주기 → 신규 루프만 join.** 신규 connection-state 루프에만 `sync.WaitGroup` join-on-Stop을 적용한다. 기존 LGAP bare goroutine은 그대로 두며, 남는 수명주기 비대칭은 알려진·수용된 상태로 두고 후속 리팩터링 SPEC에서 해소한다. (§7.2, AC-15 반영)

### 해결됨 — 2차 (RESOLVED, startup probe 설계에서 파생)

5. **[해결] OQ-A: `startup_probe_timeout` 기본값 → `min(2 × poll_interval, 30s)`, 명시 override는 캡 미적용.** 파생 기본값에 절대 상한 30s를 적용한다. YAML 명시 값은 그대로 존중한다(캡 없음). `poll_interval`이 0/미설정이면 fallback 절대 기본값 `10s`. (§4.2, AC-12 반영)
6. **[해결] OQ-B: startup probe 실행 → 비동기(background).** `Start`는 probe 완료를 기다리지 않고 즉시 반환한다. device 간 `initial` 순서는 비결정적(소비자 비의존 계약), per-device 순서(initial이 첫 change/report보다 먼저)는 HARD 보장. 주기 루프는 `initial` 미방출 device를 skip한다. probe goroutine은 `WaitGroup` join 대상이며 Stop 이후 방출 금지. (§4.6.5, E6/E9, S5, N10, AC-8~AC-10, §7.2 반영)
7. **[해결] OQ-C: 재연결 → `change`만, `initial` 재방출 없음.** `trigger="initial"`은 프로세스 수명당 device별 최대 1회. 늦은 연결/모든 재연결은 `change`(E1/E3). `setAllDevicesOffline()`도 `change`(offline). (§4.6.6, N9, AC-11/AC-15 반영)

### 남은 열린 질문

**없음.** 위 결정들은 새로운 미해결 질문을 발생시키지 않았다. §10의 모든 열린 질문(1~7)이 RESOLVED 상태다.
