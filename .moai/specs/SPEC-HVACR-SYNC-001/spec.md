---
id: SPEC-HVACR-SYNC-001
title: "HVACR 에이전트 미러링/동기화 (게이트웨이 ↔ 서버) over MQTT"
version: "0.2.0"
status: draft
created: 2026-08-06
updated: 2026-08-06
author: xtra
priority: P2
phase: "v0.19.0 target"
module: "internal/agent/samsung"
lifecycle: spec-anchored
tags: "hvacr, samsung, nasa, mqtt, mirror, sync, gateway"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-08-06 | 0.1.0 | 초기 SPEC 작성. 게이트웨이 ↔ 서버 HVACR 에이전트 미러링/동기화(디코드 NASA 메시지 replay) over MQTT. 1차 대상 samsung Hvacr01Agent. |
| 2026-08-06 | 0.2.0 | Module 9(미러 브로커 보안) 추가 — MQTT 인증(username/password) + TLS(CA 인증서) 지원. thingplus 패턴 재사용. 브로커 ACL 운영 가이드 포함. mTLS/클라이언트 인증서는 후속 범위. |

---

# SPEC-HVACR-SYNC-001: HVACR 에이전트 미러링/동기화 (게이트웨이 ↔ 서버) over MQTT

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP(Flow-Based Programming) 플랫폼이다. Samsung HVACR-01 에이전트(`internal/agent/samsung/`, `Hvacr01Agent`)는 RS-485(serial) 또는 TCP 트랜스포트로 삼성 시스템 에어컨(NASA 프로토콜)과 통신하며, 수신 프레임을 디코드해 디바이스 상태를 재구성하고 제어 명령을 발행한다. SPEC-SAMSUNG-HVACR-001이 이 에이전트의 기반 SPEC이다.

본 SPEC은 **동일한 HVACR 에이전트를 물리적으로 분리된 두 노드(게이트웨이, 서버)에 배치하고, MQTT를 매개로 상태·제어를 실시간 동기화**하는 기능을 정의한다:

- **게이트웨이 노드**: 디바이스와 RS-485로 직결된 에이전트. 수신·디코드한 NASA 메시지를 MQTT로 업링크(uplink)한다. 서버가 발행한 제어 명령을 다운링크(downlink)로 수신해 RS-485로 실행한다.
- **서버 노드**: 로컬 RS-485가 없는 에이전트. MQTT로 받은 디코드 메시지를 자신의 **디코드-메시지 ingress**에 주입해 게이트웨이와 동일한 상태 머신을 replay 한다. 결과적으로 게이트웨이에 연결된 디바이스가 서버에 연결된 것처럼 동작한다. 서버 에이전트에 내린 제어는 MQTT로 발행되어 게이트웨이가 대칭적으로 실행한다.

### 1.2 확정된 아키텍처 (LOCKED — 재논의 금지)

다음 결정은 확정이며, 본 SPEC의 요구사항은 이를 전제로 한다:

- **전송 채널 = MQTT** (thingplus/ThingsBoard 무관, 범용 MQTT). 기존 `MQTTAgent`(`internal/agent/system/mqtt_agent.go`)의 `MessagePublisher`/`SubscriberAgent` 구현과 `internal/node/mqtt.go`를 활용 가능하다.
- **동기화 단위 = 디코드된 NASA 메시지 replay**. device-state 델타가 아니라, 게이트웨이가 파싱한 `NasaMessage`(SA/DA/CMD/SEQ/MessageSets)를 서버 에이전트의 `handleMessage`/`UpdateFromMessageSets` 경로로 주입한다 → online/offline·discovery까지 포함한 진짜 동기화를 달성한다.
- **게이트웨이 식별 = 동기화 채널/토픽 자체가 담당**. payload에 gateway_id를 억지로 넣지 않는다. RS-485 raw 스트림의 "게이트웨이 구분 불가" 문제는 토픽 스킴(§Module 6)으로 해소된다.
- **제어 = 대칭·디코드 수준**. 제어 명령도 파싱된 구조(명령 유형 + 파라미터)로 왕복한다.
- **1차 대상 = samsung `Hvacr01Agent`** (레퍼런스 구현). 설계는 lg 등 다른 HVACR로 일반화 가능하도록 하되, lg 확장은 후속 SPEC로 note 한다.

### 1.3 기술 환경 및 재사용 씸(Seam) 맵

본 기능은 신규 코드보다 기존 씸 재사용을 우선한다. 아래 file:line은 정찰(reconnaissance)로 확인된 재사용 지점이다.

**트랜스포트 추상화 (재사용 + 신규 구현체 추가):**

| 씸 | 위치 | 재사용 방식 |
| --- | --- | --- |
| `NasaTransport` 인터페이스 (`Open/Close/Send/Receive([]byte)/Available`) | `internal/agent/samsung/transport.go:20` | 서버측 mirror 입력을 이 인터페이스의 신규 구현체로 제공 |
| 구현체 serial/tcp-client/tcp-server | `transport.go:165/262/512` (Send), `:182/284/537` (Receive), `:202/308/583` (Available) | 참조 패턴 |
| 팩토리 `NewNasaTransport(transportType, opts)` | `transport.go:599` | 신규 transport_type 분기 추가 |
| config transport 검증 switch | `config.go:114-131` (`"serial"/"tcp-client"/"tcp-server"`) | 신규 타입을 유효 값에 등록 |
| 채널 급전형 transport 검증 사례 `mockTransport` | `agent_test.go:156-224` | mirror transport가 동일 인터페이스임을 입증하는 선례 |

**파싱 경로 (I/O 무관, 재사용):**

| 씸 | 위치 |
| --- | --- |
| `receiveLoop` (수신 루프) | `agent.go:2144` |
| 인라인 파싱 블록 (scanner.Write → Next → Decode → handleMessage) | `agent.go:2200-2246` |
| `frameScanner.Write` / `frameScanner.Next` | `frame_scanner.go:30` / `frame_scanner.go:44` |
| `nasaProtocol.Decode(frame) (*NasaMessage, error)` | `protocol.go:146` |
| `handleMessage(msg *NasaMessage)` (락 보유 처리) | `agent.go:2251` (내부 `a.mu.Lock()` at `agent.go:2258`) |
| `UpdateFromMessageSets` | `device.go` |

`receiveLoop`는 로컬 변수 `scanner := newFrameScanner()`를 보유하며, 인라인 블록이 트랜스포트→scanner→Decode→handleMessage를 직접 수행한다. **디코드-메시지 replay를 위해 이 블록/handleMessage 수준의 재사용 가능한 ingress 메서드를 추출**해야 한다.

**디코드 메시지 구조:**

| 씸 | 위치 |
| --- | --- |
| `NasaMessage{SourceAddr, DestAddr, CommandCode, SequenceNum, MessageSets, Checksum, Raw}` | `message.go:80` |
| `NasaMessageSet{Index uint16, Value []byte}` | `message.go` / `protocol.go:85` |
| `MessageSetValueSize(index)` (Index 2번째 니블 기반 Value 크기) | `message.go` |
| `EncodeMessageSets` / `ParseMessageSets` | `protocol.go:42` / `protocol.go:57` |

**제어 경로:**

| 씸 | 위치 |
| --- | --- |
| `Process(data []byte) ([]byte, error)` JSON 명령 dispatch | `agent.go:463` |
| 지원 명령 `set_power/set_mode/target_temperature/set_fan_speed/set_multiple` | `agent.go:471-487` |
| control 게이트 `control_enabled` | `agent.go:470-475` |

서버 에이전트는 로컬 RS-485가 없으므로, 제어를 트랜스포트로 실행하지 않고 **MQTT downlink로 발행**한다. 게이트웨이가 이를 구독해 자신의 `Process`로 실행하는 역경로가 필요하다.

**MQTT 발행/구독 (재사용):**

| 씸 | 위치 |
| --- | --- |
| `MessagePublisher.PublishMessage(topic, qos byte, retained bool, payload []byte)` | `internal/agent/agent.go:65` (구현: `mqtt_agent.go`) |
| `SubscriberAgent.Subscribe/Unsubscribe` | `internal/agent/agent.go:31` (구현: `mqtt_agent.go:359`) |
| `MessageReceiver.ReceiveMessage(ctx)` | `internal/agent/agent.go:25` |
| `MQTTAgent` (paho, AutoReconnect/ConnectRetry, retain 지원) | `mqtt_agent.go:130` |
| `internal/node/mqtt.go` | 노드 레벨 MQTT 연동 |

**업링크 tap (신규 설계 필요):**

| 씸 | 위치 | 비고 |
| --- | --- | --- |
| `FrameNotifier.FrameNotifyCh() <-chan struct{}` | `internal/agent/agent.go:77` (구현: `agent.go:2545`) | **상태 스냅샷 알림**이지 디코드-메시지 tap이 아님 |

게이트웨이 에이전트가 수신·디코드한 `NasaMessage`를 외부로 내보내는 **디코드-메시지 레벨 tap은 net-new**이다. 기존 `FrameNotifier`/ring buffer snapshot은 상태 스냅샷 수준이라 재사용 불가하다.

**타입 등록:**

| 씸 | 위치 |
| --- | --- |
| `RegisterSamsungHvacr01Types(agentMgr)` 호출 | `cmd/xflowd/main.go:378` |

> 교훈 (auto-memory): 신규 에이전트/트랜스포트 타입은 정의만으로는 부족하며 `cmd/xflowd/main.go`의 등록 경로에 배선되어야 한다.

### 1.4 설계 원칙

- **행위 보존(behavior-preserving)**: 기존 serial/tcp-client/tcp-server 입력 경로의 동작은 불변이어야 한다. 디코드-메시지 ingress 추출은 순수 리팩터로, 기존 경로가 추출된 메서드를 호출하도록만 변경한다.
- **인터페이스 재사용**: 서버측 mirror 입력은 `NasaTransport` 또는 디코드-메시지 급전 중 하나로 통합하되, 기존 팩토리/config 파싱 패턴을 따른다.
- **락 규약 준수**: `handleMessage`는 `a.mu.Lock()` 하에서 동작한다. 신규 ingress/tap도 락 보유 중 `a.Name()` 호출을 금지한다(v0.18.6 RWMutex 재귀 deadlock 트랩).
- **채널이 곧 식별자**: 게이트웨이 구분은 토픽 스킴이 담당하며 payload는 게이트웨이 중립적이다.

### 1.5 범위 경계

- **범위 내(In-Scope)**:
  - 디코드-메시지 ingress 메서드 추출 (behavior-preserving)
  - 서버측 mirror 입력 소스 (MQTT 구독 → ingress 주입)
  - 게이트웨이측 업링크 tap (디코드 메시지 → MQTT publish)
  - 제어 역경로 (서버 제어 → MQTT downlink → 게이트웨이 실행), 선택적 ack
  - 디코드 NASA 메시지 + 제어 명령 MQTT payload 와이어 포맷 정의
  - 업링크/다운링크 분리 토픽 스킴 (게이트웨이 식별 포함)
  - 동기화 의미론 (online/offline·discovery replay, 재연결/재동기화, QoS/retain)
  - samsung `Hvacr01Agent` 대상 구현 + 단위 테스트
- **범위 외(Out-of-Scope)**:
  - lg / century 등 다른 HVACR 에이전트로의 확장 (후속 SPEC)
  - 기존 serial/tcp 입력 경로의 동작 변경
  - thingplus/ThingsBoard 전용 스키마 (범용 MQTT만)
  - MQTT 브로커 자체의 배포/HA 구성
  - 다중 게이트웨이의 서버측 aggregation UI (본 SPEC은 미러링 데이터 경로만)
  - 실제 하드웨어 통합 테스트

---

## 2. Assumptions (가정)

### A-1. MQTT 브로커 가용성

게이트웨이와 서버가 공통으로 접근 가능한 MQTT 브로커가 존재한다. 기존 `MQTTAgent`의 paho 기반 AutoReconnect/ConnectRetry가 연결 복구를 담당한다.

### A-2. 게이트웨이-서버 1:1 또는 N:1

한 서버 에이전트 인스턴스는 하나의 게이트웨이(하나의 업링크 토픽 스코프)를 미러링한다. 다중 게이트웨이는 각각 별도의 서버 에이전트 인스턴스(별도 토픽 스코프)로 처리한다. 게이트웨이 식별은 토픽에 인코딩된다.

### A-3. 디코드 결정성(determinism)

동일한 디코드 `NasaMessage`를 게이트웨이와 서버에서 각각 `handleMessage`에 통과시키면 동일한 디바이스 상태에 수렴한다(상태 머신 결정성). 즉, replay 대상은 순수 파싱 결과이며 I/O·타이밍에 의존하지 않는다.

### A-4. 시계(clock) 규약

모든 payload 타임스탬프는 epoch milliseconds(int64, `time.Now().UnixMilli()`)를 사용한다(프로젝트 규약, `receiveLoop`의 `timestamp_ms` 선례).

### A-5. 제어 권한 분리

서버 에이전트는 로컬 트랜스포트가 없으므로 제어를 직접 실행하지 않고 항상 MQTT downlink로 위임한다. 실제 RS-485 실행 권한은 게이트웨이 에이전트에만 있다.

---

## 3. Requirements (요구사항)

### Module 1: Decoded-Message Ingress 추출 (behavior-preserving)

#### REQ-SYNC-001-01-01 (Ubiquitous) 디코드-메시지 ingress 메서드

`Hvacr01Agent`는 **항상** 디코드된 `*NasaMessage`를 받아 디바이스 상태에 반영하는 재사용 가능한 ingress 진입점을 제공해야 한다:

- `ingestDecodedMessage(msg *NasaMessage)`: 단일 디코드 메시지를 `handleMessage` 경로로 주입한다. (현재 `handleMessage`가 이미 이 역할에 근접하므로, 추출은 `handleMessage`를 재사용 가능한 형태로 노출/명명하는 것으로 충분할 수 있다.)
- (선택) `ingestFrameBytes(frame []byte)`: 완전한 프레임 바이트를 `Decode` → `ingestDecodedMessage`로 흘려보낸다(바이트 급전 재사용 경로).

#### REQ-SYNC-001-01-02 (Ubiquitous) receiveLoop 인라인 블록 재사용화

`receiveLoop`(`agent.go:2144`)의 인라인 파싱 블록(`agent.go:2200-2246`: scanner.Write → Next → Decode → handleMessage)은 **항상** 위 ingress 메서드를 호출하는 형태로 재구성되어야 한다. serial/tcp-client/tcp-server 경로는 리팩터 후에도 동일한 관측 가능한 동작을 유지해야 한다(디코드 성공/실패 카운팅, unsupported 인덱스 로그 억제, decode error 로그 옵션 포함).

#### REQ-SYNC-001-01-03 (State-Driven) 락 규약 보존

**IF** ingress/tap이 `a.mu.Lock()` 보유 상태에서 동작하면 **THEN** 락 보유 중 `a.Name()`(또는 `a.ID()`)를 호출하지 **않아야 한다**. `handleMessage`의 기존 패턴(`agentName := a.agentConfig.Name`을 락 하에서 직접 읽음)을 따른다.

WHY: v0.18.6 RWMutex 재귀 RLock deadlock 트랩(auto-memory 교훈).
IMPACT: 위반 시 미러링 경로에서 교착이 발생한다.

#### REQ-SYNC-001-01-04 (Ubiquitous) frameScanner 상태 이동 시 가드

**IF** receiveLoop-local `frameScanner`를 에이전트 상태 필드로 승격하는 경우 **THEN** 스캐너 접근은 적절한 락(또는 단일 goroutine 소유)으로 보호되어야 한다. mirror transport와 로컬 transport가 동시에 동일 스캐너를 공유하지 않도록 한다(스캐너는 입력 소스별로 분리).

---

### Module 2: 서버측 Mirror 입력

#### REQ-SYNC-001-02-01 (Ubiquitous) mirror 입력 소스

서버 에이전트는 **항상** MQTT 업링크 토픽을 구독하여, 수신한 와이어 포맷 메시지(§Module 5)를 `*NasaMessage`로 역직렬화한 뒤 `ingestDecodedMessage`로 주입하는 입력 소스를 제공해야 한다.

이 입력 소스는 다음 두 설계 중 하나로 구현한다(구현 단계에서 확정, plan.md 참조):

- **(2a) 채널 급전형 `NasaTransport` 구현체** (`transport_type: "mirror"`): MQTT 구독 콜백이 역직렬화된 프레임 바이트를 내부 채널에 넣고, `Receive([]byte)`가 이를 반환한다. 기존 `receiveLoop`가 변경 없이 재사용된다. `Available()`은 MQTT 연결 상태를 반영한다.
- **(2b) 직접 ingress 급전**: MQTT 구독 콜백이 역직렬화된 `*NasaMessage`를 `ingestDecodedMessage`로 직접 전달한다(트랜스포트 우회).

> 설계 선호: (2a)는 `receiveLoop`/재연결/통계 인프라를 무변경 재사용하므로 우선 검토 대상이다. (2b)는 Decode를 생략(이미 디코드된 메시지)하는 이점이 있다. 와이어 포맷이 프레임 바이트가 아니라 구조화 JSON이면 (2b)가 자연스럽다.

#### REQ-SYNC-001-02-02 (Event-Driven) config에 mirror 타입 등록

**WHEN** `transport_type` 설정 값이 mirror 입력을 지정하면 **THEN**:

1. `config.go`의 transport 검증 switch(`config.go:114-131`)가 신규 값을 유효한 값으로 수락한다.
2. `NewNasaTransport`(`transport.go:599`)가 신규 분기에서 mirror 구현체(설계 2a인 경우)를 생성한다.
3. mirror 전용 설정(브로커 주소/업링크 토픽/QoS 등)을 파싱한다.

**IF** mirror 입력이 요구하는 필수 설정(브로커/토픽)이 없으면 **THEN** 명시적 에러를 반환한다.

#### REQ-SYNC-001-02-03 (State-Driven) 서버 에이전트의 로컬 제어 억제

**IF** 에이전트가 mirror 입력 모드로 동작하면 **THEN** 제어 명령을 로컬 트랜스포트로 전송하지 **않아야 한다**. 대신 제어를 MQTT downlink로 발행한다(REQ-SYNC-001-04-01).

---

### Module 3: 게이트웨이측 업링크 Tap

#### REQ-SYNC-001-03-01 (Event-Driven) 디코드-메시지 업링크 tap

**WHEN** 게이트웨이 에이전트가 로컬 트랜스포트에서 프레임을 수신·디코드하면(성공한 `Decode` 결과) **THEN** 해당 `*NasaMessage`를 정규화 와이어 포맷(§Module 5)으로 직렬화하여 업링크 토픽으로 MQTT publish 한다.

- tap 지점은 `ingestDecodedMessage`(또는 `handleMessage`) 직전/직후로, 로컬 상태 갱신과 업링크가 모두 일어나도록 한다.
- 업링크는 로컬 상태 반영을 차단하지 않아야 한다(비동기 발행 또는 짧은 버퍼).

#### REQ-SYNC-001-03-02 (Ubiquitous) tap 활성화 설정

게이트웨이 에이전트는 **항상** 업링크 tap을 활성/비활성하는 설정(예: `mirror_uplink_enabled`, 브로커/토픽/QoS/retain)을 지원해야 한다. tap이 비활성이면 게이트웨이는 기존 단독 에이전트로 동작한다(행위 보존).

#### REQ-SYNC-001-03-03 (Unwanted) CRC 실패/디코드 실패 메시지 미전송

시스템은 CRC 불일치 또는 디코드 실패한 프레임을 업링크로 전송하지 **않아야 한다**. 업링크 대상은 성공적으로 디코드된 메시지에 한한다(재현 시 서버가 게이트웨이와 동일 결과에 수렴하도록).

#### REQ-SYNC-001-03-04 (Ubiquitous) 발행 실패 격리

업링크 MQTT publish 실패는 게이트웨이의 로컬 상태 갱신·제어 실행을 **차단하지 않아야 한다**. 발행 실패는 카운터 증가 및 로그로 처리하고, 재연결은 `MQTTAgent`의 paho 재연결에 위임한다.

---

### Module 4: 제어 역경로 (Downlink)

#### REQ-SYNC-001-04-01 (Event-Driven) 서버 제어 → downlink 발행

**WHEN** mirror 모드 서버 에이전트의 `Process(data)`가 제어 명령(`set_power`/`set_mode`/`target_temperature`/`set_fan_speed`/`set_multiple`)을 수신하면 **THEN**:

1. 명령을 로컬 트랜스포트로 실행하지 않는다.
2. 명령 유형 + 대상(`device_id`/`address`) + 파라미터를 제어 와이어 포맷(§Module 5.2)으로 직렬화한다.
3. 다운링크 토픽으로 MQTT publish 한다.
4. (선택) 제어 ack 대기 설정에 따라 ack 토픽 응답을 대기하거나 즉시 `{"status":"accepted"}`를 반환한다.

#### REQ-SYNC-001-04-02 (Event-Driven) 게이트웨이 downlink 구독 → 실행

**WHEN** 게이트웨이 에이전트가 다운링크 토픽에서 제어 와이어 포맷을 수신하면 **THEN**:

1. 제어 와이어 포맷을 게이트웨이의 `Process` 입력 JSON으로 변환한다.
2. `Process`를 호출하여 실제 RS-485 제어를 수행한다(기존 `agent.go:463` dispatch 재사용).
3. (선택) 실행 결과를 ack 토픽으로 발행한다(REQ-SYNC-001-04-03).

#### REQ-SYNC-001-04-03 (Optional) 제어 Ack

**가능하면** 게이트웨이는 제어 실행 결과(성공/실패, 대상 주소)를 ack 토픽으로 발행하고, 서버는 이를 상관(correlate)하여 원 `Process` 호출의 응답으로 반영한다. 상관 키는 제어 와이어 포맷의 `req_id`(있을 경우)로 한다.

#### REQ-SYNC-001-04-04 (Unwanted) downlink 루프백 방지

게이트웨이는 자신이 실행한 제어의 결과로 생성되는 상태 변화 업링크를, downlink 명령 자체와 혼동하여 재-downlink 하지 **않아야 한다**. 업링크(디코드 메시지)와 다운링크(제어 명령)는 분리된 토픽으로 격리된다(§Module 6).

---

### Module 5: 와이어 포맷 (Wire Format)

#### REQ-SYNC-001-05-01 (Ubiquitous) 디코드 NASA 메시지 업링크 포맷

업링크 payload는 **항상** 디코드된 `NasaMessage`를 무손실 표현하는 JSON이어야 한다. 필드:

| 필드 | 타입 | 설명 |
| --- | --- | --- |
| `ts` | int64 | epoch milliseconds (`UnixMilli`) |
| `sa` | string | Source Address, compact hex ("100000") |
| `da` | string | Dest Address, compact hex |
| `cmd` | uint16(정수) | CommandCode |
| `seq` | uint8(정수) | SequenceNum |
| `sets` | array | `[{ "index": uint16, "value": "<hex>" }]` — MessageSet 목록. `index`는 정수, `value`는 hex 인코딩 문자열 |

`value`의 크기는 `MessageSetValueSize(index)` 규칙과 일치해야 하며, 라운드트립(직렬화→역직렬화→`handleMessage`)이 원본과 동일한 상태 반영을 낳아야 한다. `Checksum`/`Raw`는 재현에 불필요하므로 생략 가능하다(서버는 이미 디코드된 메시지를 신뢰).

> 대안: `Raw` 프레임 바이트를 그대로 hex로 실어 서버가 `Decode`를 재수행하는 방식(설계 2a와 결합). 이 경우 포맷은 `{ "ts": ..., "raw": "<hex frame>" }`로 단순화된다. plan.md에서 구조화 JSON vs raw-hex 트레이드오프를 확정한다.

#### REQ-SYNC-001-05-02 (Ubiquitous) 제어 명령 다운링크 포맷

다운링크 payload는 **항상** 게이트웨이 `Process`로 변환 가능한 제어 표현이어야 한다. 필드:

| 필드 | 타입 | 설명 |
| --- | --- | --- |
| `ts` | int64 | epoch milliseconds |
| `command` | string | `set_power`/`set_mode`/`target_temperature`/`set_fan_speed`/`set_multiple` |
| `device_id` | string(선택) | 대상 device_id (우선) |
| `address` | string(선택) | 대상 주소 compact/spaced hex |
| `params` | object | 명령별 파라미터 (기존 `Process` params 스키마와 동일) |
| `req_id` | string(선택) | ack 상관 키 |

게이트웨이는 `command`/`device_id`/`address`/`params`를 기존 `Process` 입력 JSON으로 그대로 매핑한다(스키마 재사용, `agent.go:463`).

#### REQ-SYNC-001-05-03 (Ubiquitous) 타임스탬프 규약

모든 와이어 포맷의 `ts`는 **항상** epoch milliseconds(int64)여야 한다. `time.Time`/RFC3339를 사용하지 **않는다**(프로젝트 규약).

#### REQ-SYNC-001-05-04 (Unwanted) 게이트웨이 식별자 payload 삽입 금지

와이어 포맷은 gateway_id를 payload에 포함하지 **않아야 한다**. 게이트웨이 식별은 토픽 스킴(§Module 6)이 전담한다.

---

### Module 6: 토픽 스킴 (Topic Scheme)

#### REQ-SYNC-001-06-01 (Ubiquitous) 토픽 스킴 정의

토픽 스킴은 **항상** 게이트웨이 식별 + 업링크/다운링크 분리를 표현해야 한다. 기본 스킴(설정으로 prefix 변경 가능):

| 방향 | 토픽 패턴 | 발행자 → 구독자 |
| --- | --- | --- |
| 업링크(디코드 메시지) | `{prefix}/{gateway_id}/up/nasa` | 게이트웨이 → 서버 |
| 다운링크(제어) | `{prefix}/{gateway_id}/down/control` | 서버 → 게이트웨이 |
| 제어 ack(선택) | `{prefix}/{gateway_id}/up/ack` | 게이트웨이 → 서버 |
| 라이프사이클 이벤트(선택) | `{prefix}/{gateway_id}/up/event` | 게이트웨이 → 서버 |

- `prefix` 기본값 예: `xflow/hvacr` (설정 가능).
- `gateway_id`는 게이트웨이 에이전트 설정으로 지정하는 안정적 식별자이며, 이 값으로 서버가 특정 게이트웨이를 구독한다.

#### REQ-SYNC-001-06-02 (Ubiquitous) 서버 구독 대상

서버 에이전트는 **항상** 자신이 미러링할 `gateway_id`의 업링크 토픽(`.../up/nasa`, 그리고 활성 시 `.../up/ack`)을 구독해야 한다.

#### REQ-SYNC-001-06-03 (State-Driven) 방향 격리

**IF** 에이전트가 게이트웨이 역할이면 **THEN** `.../up/*`에 발행하고 `.../down/control`을 구독한다. **IF** 서버 역할이면 **THEN** `.../up/*`를 구독하고 `.../down/control`에 발행한다. 두 역할은 동일 토픽에 동시에 발행/구독하지 않는다(루프백 방지, REQ-SYNC-001-04-04).

---

### Module 7: 동기화 의미론 (Synchronization Semantics)

#### REQ-SYNC-001-07-01 (Ubiquitous) online/offline·discovery replay

디코드 메시지 replay는 **항상** 상태 값뿐 아니라 online/offline 전이와 discovery(자동 탐색)까지 재현해야 한다. 서버 에이전트는 게이트웨이가 수신한 것과 동일한 C0xx/notification 메시지를 replay 받으므로, `handleMessage` 경로의 auto-discovery·online 전환·device_state emit이 서버측에서도 동일하게 발생한다.

#### REQ-SYNC-001-07-02 (Event-Driven) 서버 재시작 시 재동기화

**WHEN** 서버 에이전트가 재시작(또는 신규 구독)하면 **THEN** 게이트웨이의 현재 상태를 재확보할 수 있어야 한다. 다음 중 하나 이상의 메커니즘을 제공한다:

- **(7a) retain 스냅샷**: 게이트웨이가 디바이스별 최신 상태 스냅샷을 retained 메시지로 별도 토픽(예 `.../up/snapshot/{addr}`)에 유지 → 서버가 구독 즉시 최신 상태 수신.
- **(7b) resync 요청**: 서버가 다운링크로 resync 명령을 발행 → 게이트웨이가 캐시된 모든 디바이스 상태를 업링크로 재emit(기존 `request_state`/`get_all` dispatch 재사용, `agent.go:502-524`).

plan.md에서 (7a)/(7b) 중 채택안을 확정한다. NASA는 실외기/실내기가 주기적으로 상태를 브로드캐스트하므로, 채택 없이도 일정 시간 후 자연 수렴하나, 재시작 직후 공백을 줄이려면 (7a) 또는 (7b)가 필요하다.

#### REQ-SYNC-001-07-03 (Ubiquitous) QoS 및 retain 정책

- 업링크 디코드 메시지: **항상** 최소 QoS 1(at-least-once)로 발행한다. 디코드 메시지는 idempotent 재현이 원칙이나, 순서 민감한 online/offline 전이 손실을 줄이기 위해 QoS 1을 기본으로 한다. retain은 사용하지 않는다(스트림 성격).
- 스냅샷 토픽(7a 채택 시): retain=true, QoS 1.
- 다운링크 제어: QoS 1, retain=false(제어는 재적용 시 부작용 가능하므로 retain 금지).

#### REQ-SYNC-001-07-04 (State-Driven) MQTT 연결 끊김 처리

**IF** MQTT 연결이 끊기면 **THEN** mirror 서버 에이전트의 `Available()`(설계 2a)은 연결 상태를 반영하고, `MQTTAgent`의 paho AutoReconnect/ConnectRetry가 복구를 담당한다. 재연결 후 서버는 재동기화(REQ-SYNC-001-07-02)를 수행한다.

---

### Module 8: 경계/스코프 가드 (Boundary Guards)

#### REQ-SYNC-001-08-01 (Ubiquitous) 1차 대상 samsung 한정

본 SPEC의 구현은 **항상** samsung `Hvacr01Agent`에 한정한다. lg/century 등 다른 HVACR 에이전트로의 확장은 후속 SPEC로 분리한다. 단, ingress/tap/와이어 포맷/토픽 스킴 설계는 프로토콜 중립적 구조를 지향해 재사용 가능하도록 한다.

#### REQ-SYNC-001-08-02 (Unwanted) 기존 입력 경로 무변경

시스템은 기존 serial/tcp-client/tcp-server 입력 경로의 관측 가능한 동작을 변경하지 **않아야 한다**. Module 1의 ingress 추출은 순수 리팩터로, 기존 경로가 추출 메서드를 호출하도록만 변경한다.

#### REQ-SYNC-001-08-03 (Unwanted) thingplus 경로 독립

본 미러링 데이터 경로는 thingplus/ThingsBoard 전용 로직과 독립적이어야 한다. 범용 MQTT 발행/구독만 사용하며 브로커 벤더에 종속되지 **않아야 한다**.

#### REQ-SYNC-001-08-04 (Ubiquitous) 타입/트랜스포트 등록 배선

신규 mirror transport 타입(설계 2a) 또는 신규 에이전트 역할은 **항상** `cmd/xflowd/main.go`의 등록 경로에 배선되어야 한다(`main.go:378` 인근). 정의만으로는 런타임에 활성화되지 않는다.

---

### Module 9: 미러 브로커 보안 (인증 + TLS)

미러 동기화 MQTT 경로는 초기 설계에서 인증·암호화가 없었다(평문 tcp:// + 익명 접속). 게이트웨이·서버가 서로 다른 물리 노드에 있고 신뢰되지 않은 네트워크(원격 사이트, 인터넷 경유)를 건널 수 있으므로, 보안 브로커(인증 + per-topic ACL) 사용을 위한 자격증명·TLS 설정을 제공해야 한다.

설계 원칙: 기존 thingplus_agent(`internal/agent/system/thingplus_agent.go`)의 검증된 패턴을 재사용한다 — paho `SetUsername`/`SetPassword`(빈 값이면 미적용), `SetTLSConfig`(CA PEM 또는 파일 경로 → `*tls.Config{MinVersion: TLS1.2}`). 모든 옵션은 **선택**이며, 미설정 시 기존 무인증/평문 동작이 그대로 보존된다(행위 보존, REQ-SYNC-001-08-02와 정합).

#### REQ-SYNC-001-09-01 (Ubiquitous) MQTT 인증 지원

시스템은 미러 브로커 연결에 대해 username/password 인증을 지원해야 한다. `mirror_username`/`mirror_password` 옵션이 비어 있지 않으면 paho `SetUsername`/`SetPassword`로 적용한다. 둘 다 비어 있으면 익명 접속(기존 동작)을 유지한다.

#### REQ-SYNC-001-09-02 (Ubiquitous) TLS 암호화 지원

**WHEN** `mirror_tls`가 true이면 **THEN** 시스템은 미러 브로커 연결에 TLS를 적용해야 한다(`SetTLSConfig`, 최소 버전 TLS 1.2). 브로커 주소는 `ssl://host:8883` 형식으로 지정한다.

#### REQ-SYNC-001-09-03 (Ubiquitous) CA 인증서 검증

`mirror_ca_cert` 옵션은 서버 인증서 검증용 CA를 PEM 문자열 또는 파일 경로로 받아야 한다(두 형식 모두 지원, thingplus `buildTLSConfig`와 동일). 값이 비어 있으면 시스템 루트 CA를 사용한다. PEM 파싱 실패 시 명시적 에러를 반환해야 한다(silent 무시 금지).

#### REQ-SYNC-001-09-04 (Unwanted) 기존 동작 보존

보안 옵션 4종(`mirror_username`/`mirror_password`/`mirror_tls`/`mirror_ca_cert`)은 모두 선택이며, 신규 required-field 에러를 추가하지 **않아야 한다**. 미설정 config는 이전과 동일하게 동작해야 한다.

#### REQ-SYNC-001-09-05 (스코프 경계) mTLS 후속 분리

본 모듈은 서버 인증(단방향 TLS) + username/password까지를 범위로 한다. 클라이언트 인증서(mTLS)는 후속 SPEC로 분리한다.

#### 브로커 ACL 운영 가이드 (권장 배포 구성)

미러 동기화를 보안 브로커로 운영할 때 권장하는 구성이다. 에이전트 코드가 아니라 **브로커 측 설정**으로 강제한다:

1. **필수 인증(익명 금지)**: 브로커에서 익명 접속을 비활성화하고(`allow_anonymous false` 등) 모든 클라이언트가 username/password로 인증하도록 강제한다.
2. **per-topic ACL(게이트웨이별 최소권한)**: 각 게이트웨이 자격증명이 자신의 토픽 네임스페이스 `{prefix}/{gateway_id}/*`만 발행/구독하도록 ACL을 설정한다. 예:
   - 게이트웨이 `gw01`: `{prefix}/gw01/up/#` 발행 + `{prefix}/gw01/down/#` 구독만 허용.
   - 서버: `{prefix}/+/up/#` 구독 + `{prefix}/+/down/#` 발행(또는 담당 게이트웨이로 한정).
   - 이로써 탈취된 게이트웨이 자격증명이 다른 게이트웨이 토픽을 위조·감청하지 못한다(토픽 스킴이 보안 경계를 겸함, §Module 6과 정합).
3. **TLS 전송 암호화**: 신뢰되지 않은 네트워크를 건너는 경우 `mirror_tls: true` + 서버 인증서 검증(`mirror_ca_cert`)으로 도청·MITM을 방지한다.
4. **네트워크 격리**: 가능하면 브로커를 VPN/사설망 뒤에 두고, 노출이 필요한 경우 방화벽으로 소스 IP를 제한한다.

이 가이드는 권장 사항이며, 실제 강제는 브로커(Mosquitto/EMQX 등)의 인증·ACL 설정으로 수행한다. 에이전트는 자격증명·TLS를 제공하는 역할까지만 담당한다.

> **복붙용 ACL 템플릿**(Mosquitto ACL 파일 + EMQX 규칙, 게이트웨이·서버 양방향, 멀티테넌트 범위 제한 포함): [docs/guides/hvacr-mirror-broker-security.md](../../../docs/guides/hvacr-mirror-broker-security.md)

---

## 4. Specifications (사양 요약)

### 4.1 데이터 흐름

```
[디바이스] --RS485--> [게이트웨이 Hvacr01Agent]
   receiveLoop → frameScanner → Decode → ingestDecodedMessage(handleMessage)
        │                                        │
        │ (업링크 tap)                            └─ 로컬 상태 갱신 (게이트웨이도 완전한 상태 보유)
        ▼
   wire(디코드 메시지) --MQTT publish--> {prefix}/{gw}/up/nasa  (QoS1)
                                            │
                                            ▼  MQTT subscribe
                          [서버 Hvacr01Agent (mirror 입력)]
                          역직렬화 → ingestDecodedMessage(handleMessage)
                          → 서버측 상태 = 게이트웨이측 상태 (수렴)

[서버 Process(제어)] → wire(제어) --publish--> {prefix}/{gw}/down/control (QoS1)
                                                   │ subscribe
                                                   ▼
                          [게이트웨이] wire(제어) → Process → RS485 실행
                                                   └─(선택) ack → {prefix}/{gw}/up/ack
```

### 4.2 Traceability (추적성)

| 요구사항 모듈 | 주요 재사용 씸 | 신규 산출물 |
| --- | --- | --- |
| M1 ingress 추출 | `agent.go:2144/2200-2246/2251` | `ingestDecodedMessage` 추출/명명 |
| M2 서버 mirror 입력 | `transport.go:20/599`, `config.go:114-131`, `mqtt_agent.go` | mirror transport(2a) 또는 ingress 급전(2b) |
| M3 업링크 tap | `agent.go:2251`, `MessagePublisher` | net-new 디코드-메시지 tap |
| M4 제어 역경로 | `agent.go:463`, `SubscriberAgent` | downlink 구독→Process, 선택 ack |
| M5 와이어 포맷 | `message.go:80`, `MessageSetValueSize` | JSON 스키마 정의 + (역)직렬화 |
| M6 토픽 스킴 | `internal/node/mqtt.go` | 토픽 상수/설정 |
| M7 동기화 의미론 | `agent.go:502-524`(request_state/get_all) | retain 스냅샷 또는 resync |
| M8 경계 가드 | `cmd/xflowd/main.go:378` | 등록 배선 |
| M9 미러 브로커 보안 | `thingplus_agent.go:700-712/762-789`(auth/TLS 패턴), `mqtt_agent.go:258-263`(auth gating) | `mirrorBrokerConn` + `buildMirrorTLSConfig` + config 4필드 |

관련 SPEC: SPEC-SAMSUNG-HVACR-001 (기반 에이전트), SPEC-LG-HVACR-001 / SPEC-LGAP-001 (후속 일반화 대상), SPEC-BRIDGE-001/002 (브릿지 연동).
