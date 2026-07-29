---
id: SPEC-XSFM-001
title: "지하철 역사 설비 관리 에이전트 (MQTT)"
version: "0.3.0"
status: completed
created: 2026-07-28
updated: 2026-07-28
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/agent/xsfm"
lifecycle: spec-anchored
tier: M
tags: "xsfm, mqtt, facility, device-control, group-control, subway, station-registry, location-hierarchy, response-wait"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                    |
| ---------- | ----- | -------------------------------------------------------- |
| 2026-07-28 | 0.1.0 | 초기 SPEC 작성 — 지하철 역사 시설물 관리 비전의 첫 디바이스(설비) MQTT 제어 에이전트 |
| 2026-07-28 | 0.2.0 | 듀얼 트랜스포트 모드(`transport_mode`: direct \| port) 추가 — direct는 에이전트가 브로커 sub/pub 소유, port는 외부 mqtt-in/out 노드가 브로커 I/O를 담당하고 에이전트는 순수 프로토콜/로직 레이어로 동작(상태 입력 포트 · 제어 출력 포트 분리). M1 트랜스포트를 I/O 경계 vs 공유 페이로드 매핑/로직으로 분리 |
| 2026-07-28 | 0.3.0 | **디바이스 모델 · 제어 의미론 집중 확장** — (1) 디바이스 위치 계층 속성(`station`/`place`/`index`)을 로스터에 추가(선택/하위호환), (2) **역사 레지스트리(station registry)** 도입 — station→line(호선) 매핑을 SSOT로 정의(디바이스는 station만 보유, line은 station→line로 해석), (3) 개별(M3)·그룹(M4) 제어를 **응답 대기(state echo 대기)** 로 확장 — `control_response_timeout` + 디바이스별 pending-command 레지스트리 + 에코 상관(correlation by device_id), 타임아웃 시 `ErrControlTimeout`, (4) 일괄 제어(M4) 대상을 **line/station 셀렉터**로 확장(station registry 경유). 대시보드 패널은 본 SPEC 범위 외(SPEC-FACILITY-DASHBOARD-001) — 본 SPEC은 그들이 소비할 디바이스 계층 + 역사 레지스트리 + fan-out만 소유 |

---

# SPEC-XSFM-001: 지하철 역사 설비 관리 에이전트 (MQTT)

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP(Flow-Based Programming) 플랫폼이다. Agent 시스템은 Transport(통신 인터페이스)와 디바이스 프로토콜을 결합하여 외부 장비를 제어·모니터링하고, 상태 텔레메트리를 플로우 노드로 전달한다.

본 SPEC은 **지하철 역사 시설물 관리(facility management)** 라는 광의의 비전에서 **첫 번째 디바이스 타입인 설비(facility)** 를 대상으로 하는 제어 에이전트를 정의한다. 설비는 MQTT 브로커에 연결된 IoT 디바이스이며, 에이전트는 디바이스의 **상태(STATE)를 수신**하고 **제어 명령을 방출(emit)**한다.

에이전트는 **두 가지 트랜스포트 모드(`transport_mode`)** 를 지원한다. **`direct` 모드**에서는 에이전트가 MQTT(Paho) 클라이언트를 직접 소유하여 STATE 토픽을 **구독**하고 COMMAND 토픽으로 제어 명령을 **발행**한다. **`port` 모드**에서는 에이전트가 브로커에 직접 연결하지 않고, 외부 플로우 노드(상류 mqtt-in 노드 · 하류 mqtt-out 노드)가 브로커 I/O를 담당한다 — 디바이스 STATE는 **노드 입력 포트**로 유입되고, 제어 명령은 **별도의 제어 출력 포트**로 방출되어 하류 mqtt-out 노드가 발행한다. **두 모드는 페이로드 매핑·로스터·제어 명령 구성·그룹 fan-out·2-축 제어·관측 기반 emit 등 동일한 프로토콜/로직 레이어를 공유**하며, 오직 **I/O 경계(브로커 sub/pub vs 노드 입력/출력 포트)** 만 다르다. 기본값은 `direct`이다.

에이전트는 (1) 개별 디바이스 제어, (2) 그룹 제어, (3) 상태 모니터링, (4) 로그 관리(상태 시계열 + 제어 감사) 기능을 제공한다. 설비가 첫 시설물 디바이스이므로 에이전트 구조는 향후 확장을 고려해 깔끔하게 설계하되, **본 SPEC의 범위는 설비로 한정**한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/xsfm/` (신규)
- **MQTT 트랜스포트 참조**: `internal/agent/system/thingplus_agent.go` (Paho MQTT 게이트웨이 에이전트 — 브로커/TLS/토픽 설정, publish/subscribe, connect/reconnect, LWT). RS-485(Samsung/LG) 트랜스포트가 **아닌** MQTT 트랜스포트를 참조한다.
- **기존 인터페이스**:
  - `Agent` 인터페이스 (`internal/agent/agent.go`): Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
  - `MessageReceiver` 인터페이스: `ReceiveMessage(ctx)` — 상태 변경/이벤트 메시지를 JSON으로 반환
  - `BaseAgent` 구조체: `*lifecycle.BaseLifecycle` 임베딩, Stats 추적
  - `TypeRegistry`: RegisterType/CreateAgent/ListTypes/HasType
- **디바이스 어댑터 레이어** (`internal/device/adapter/`): `CommandSpec`(제어 명령 스키마 선언), `CommandExecutor`(명령 실행 브리지)
- **플로우 노드** (`internal/node/`): status 노드(상태 emit) + control 노드(제어) — Samsung/LG의 status+control 노드 분리 패턴
- **텔레메트리 경로**: 에이전트 → status 노드 → `influxdb_write` 노드 (`internal/node/influxdb_write.go`). 에이전트는 InfluxDB에 **직접 기록하지 않는다**.
- **제어 감사 경로**: `internal/storage/remote_audit_repository.go`
- **디바이스 ID 영속화**: `internal/agent/device_id_repo.go` + `internal/storage/device_id_repository.go`
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **기존 패턴 준수**: MQTT 트랜스포트/생명주기/재연결은 `thingplus_agent.go`, 디바이스 로스터/제어/노드 분리/영속화는 Samsung HVACR-01(SPEC-SAMSUNG-HVACR-001) 패턴을 따른다.
- **관심사 분리**: MQTT 트랜스포트, 설정 파싱, 디바이스 로스터, 제어 인코딩, 상태 디코딩을 별도 파일로 분리한다.
- **테스트 가능성**: MQTT 클라이언트를 인터페이스로 추상화하여 목(mock) 브로커 주입으로 단위 테스트가 가능해야 한다.
- **설정 주도 시임(seam)**: MQTT 토픽 스킴/페이로드 포맷은 디바이스 매뉴얼에 종속되므로 **하드코딩하지 않고** 설정 기반 토픽 템플릿 + 페이로드 필드 매핑으로 표현한다.
- **관측 기반 emit**: 관측된 필드만 상태 payload에 포함한다 (Samsung `StateForJSON` 규칙 준수).

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - XSFMAgent 구현 (Agent + MessageReceiver 인터페이스 준수)
  - **듀얼 트랜스포트 모드 (`transport_mode`: `direct` | `port`, 기본 `direct`)**
    - `direct`: 에이전트가 MQTT(Paho) 클라이언트 소유 — 브로커/TLS/LWT/재연결, STATE 구독 + COMMAND 발행
    - `port`: 브로커 연결 없이 외부 mqtt-in/out 노드가 I/O 담당 — STATE는 노드 **입력 포트**로 유입, 제어 명령은 **제어 출력 포트**로 방출 (상태 입력 포트 ≠ 제어 출력 포트, 분리)
  - **I/O 경계 vs 공유 프로토콜/로직 레이어 분리** — 페이로드 매핑/로스터/제어 구성/그룹 fan-out은 두 모드가 동일하게 재사용
  - 설정 주도 토픽 템플릿(direct 모드) + 페이로드 필드 매핑 시임(양 모드 공통)
  - 디바이스 로스터 (device_id 키잉, group_id 속성 포함)
  - **디바이스 위치 계층 속성 (`station`/`place`/`index`) — 선택/하위호환 (NET-NEW, v0.3.0)**
  - **역사 레지스트리 (station registry) — station→line(호선) 매핑 SSOT + CRUD/lookup (NET-NEW, v0.3.0)**
  - 로스터 add/remove/set 런타임 명령 + 영속화 (station/place/index 포함)
  - 2-축 상태 모델 (`set_power` on/off + `set_fan_speed` 1/2/3)
  - 개별 디바이스 제어
  - **제어 응답 대기 (state echo 대기) — `control_response_timeout` + pending-command 레지스트리 + 에코 상관 (NET-NEW, v0.3.0)**
  - **그룹 제어 — 에이전트 측 fan-out (NET-NEW)**
  - **line/station 셀렉터 기반 일괄 제어 (station registry 경유, M4 fan-out 확장, NET-NEW, v0.3.0)**
  - 상태 구독 → 로스터 갱신, 온라인/오프라인 감지
  - 상태 시계열 (status 노드 → influxdb_write) + 제어 감사
  - status 노드(상태 emit + **port 모드 상태 입력 포트**) + control 노드(제어 + **port 모드 제어 출력 포트**)
  - 디바이스 어댑터 CommandSpec (`set_power` bool, `set_fan_speed` enum)
  - 타입 등록 (RegisterXSFMTypes) + main.go 배선 + API 어댑터 + 프론트엔드 스키마
  - 센티널 에러 정의 + 단위 테스트
- **범위 외(Out-of-Scope)**:
  - 설비 이외의 시설물 디바이스 타입 (조명, 냉난방, CCTV 등 — 후속 SPEC)
  - **대시보드 패널 (라인/역사 패널, 일괄제어 UI, 라인 다이어그램 렌더링) — SPEC-FACILITY-DASHBOARD-001의 범위. 본 SPEC은 그 패널들이 소비할 디바이스 위치 계층(station/place/index) + 역사 레지스트리(station→line) + line/station fan-out 표면만 소유하며, 시각화/UI는 정의하지 않는다**
  - 하드웨어 브로드캐스트 방식의 그룹 제어 (본 SPEC은 **에이전트 측 per-device fan-out**만 정의)
  - 구체적 토픽 문자열/페이로드 스키마 확정 (디바이스 매뉴얼 확보 후 설정으로 확정)
  - PM2.5/CO2 등 센서 텔레메트리 수집 확장 (필요 시 후속 — 본 SPEC은 power/fan_speed/online 상태에 집중)
  - 제어 재시도/롤백 (응답 대기는 성공/타임아웃 판정까지만 정의하며, 타임아웃 후 자동 재발행/롤백은 범위 외 — 후속)
  - InfluxDB 직접 기록 (텔레메트리는 반드시 status 노드 → influxdb_write 경로)

---

## 2. Assumptions (가정)

### A-1. MQTT 매뉴얼 미확보 → 토픽/페이로드 설정화

대상 설비의 구체적 MQTT 토픽 스킴과 페이로드 JSON 필드는 디바이스 매뉴얼이 확보되기 전까지 알 수 없다. 따라서 설계는 이를 **설정 주도**로 표현해야 한다: state/command 토픽 템플릿(디바이스 ID placeholder 포함) + 페이로드 필드 매핑(어느 JSON 필드가 power/fan_speed/online을 나르는지). 구체적 문자열/스키마는 매뉴얼 확보 후 설정 값으로 확정되며, 설계에 하드코딩되지 **않는다**.

### A-2. MQTT 브로커 가용성

대상 시스템에 접근 가능한 MQTT 브로커(TCP 또는 TLS)가 존재하며, 에이전트가 STATE 토픽 구독과 COMMAND 토픽 발행 권한을 갖는다고 가정한다.

### A-3. 2-축 상태 모델

설비 상태는 `{power: on/off, fan_speed: 1|2|3, online: bool}` 2-축으로 모델링 가능하다고 가정한다. **풍량(fan_speed)은 전원이 ON일 때만 유효**하다. power가 OFF이면 fan_speed는 의미 없는 값으로 간주한다.

### A-4. 그룹은 디바이스 속성

그룹 멤버십은 각 디바이스의 선택적 속성(`group_id`/tag)으로 표현된다. 별도의 그룹 엔티티 테이블은 두지 않으며, 로스터의 디바이스 속성 조회로 그룹 멤버를 도출한다.

### A-5. 그룹 제어는 에이전트 측 fan-out

그룹 명령을 받으면 에이전트가 로스터에서 해당 group_id의 멤버 디바이스를 순회하며 **멤버별 개별 MQTT 명령을 발행**한다. 하드웨어 레벨 브로드캐스트가 아니다. Samsung 에이전트(`agent.go:530` 부근)가 forward-compat 차원에서 group_id를 수용하되 무시하는 현재 상태를 확장하여, 본 SPEC에서 실제 fan-out 동작을 신규 정의한다.

### A-6. device_id 키잉

디바이스는 device_id를 기준으로 키잉된다 (프로젝트 컨벤션 — 에이전트 이름 중복 시 device_id 분기). 로스터 맵의 기본 키는 device_id이다.

### A-7. transport_mode는 I/O 경계만 선택한다

`transport_mode`(`direct` | `port`)는 **오직 I/O 경계만 선택**하며, 그 위의 프로토콜/로직 레이어(페이로드 매핑, 로스터, 제어 명령 구성, 그룹 fan-out, 2-축 제어, 관측 기반 emit)는 **두 모드에서 완전히 동일**하다고 가정한다. 에이전트는 순수 프로토콜/로직 레이어이며, 실제 브로커 I/O는 `direct`에서는 에이전트가, `port`에서는 외부 mqtt-in/out 노드가 담당한다.

- **상태 입력 경계(state ingress)**: `direct`=브로커 구독 콜백, `port`=상태 노드의 **입력 포트**. 두 경우 모두 동일한 페이로드 디코딩 경로(REQ-XSFM-001-05-02)를 거쳐 로스터를 갱신한다.
- **명령 출력 경계(command egress)**: `direct`=브로커 publish, `port`=제어 노드의 **제어 출력 포트** emit. 두 경우 모두 동일한 제어 인코딩 경로(페이로드 매핑)로 명령 메시지를 구성한다.
- **상태 입력 포트와 제어 출력 포트는 서로 다른 별개의 포트**이다 (명시적 요구사항).
- `port` 모드에서는 브로커/토픽 템플릿 설정을 에이전트가 사용하지 않는다(외부 mqtt 노드가 토픽을 소유). 단, **페이로드 필드 매핑은 양 모드에서 동일하게 사용**된다(유입 상태 메시지 파싱 + 방출 제어 메시지 포맷).

### A-8. transport_mode 기본값과 설정 검증 분기

`transport_mode`는 미지정 시 `direct`로 기본 설정되어 기존 SPEC(v0.1.0)과 동작이 일치한다. `direct` 모드에서는 브로커 주소·토픽 템플릿이 **필수**이고, `port` 모드에서는 이들이 **선택/미사용**이라고 가정한다.

### A-9. 위치 계층은 디바이스의 구조화 속성 (station/place/index)

각 설비 디바이스는 자유 텍스트 `Location`(기존 `internal/device/device.go:117`) 대신/에 더해 **구조화된 위치 속성**을 갖는다: `station`(역사 식별자), `place`(역사 내 위치/구역 — 예: 승강장/대합실/출구), `index`(해당 station/place 내 디바이스 순번). 세 속성은 **선택(optional)이며 하위호환**이다 — 미지정 시 빈 값/0으로 두어 기존 v0.2.0 로스터와 동작이 일치한다. `index`는 **station(+place) 범위의 시퀀스 번호**로 가정한다(전역 유일 번호가 아님).

### A-10. line(호선)은 디바이스 속성이 아니라 station 상위 집계 레벨

**line(호선)은 디바이스 속성이 아니다.** line은 station의 **상위 집계 레벨**이며, station→line 매핑을 통해 해석된다. 따라서 디바이스는 `station`만 보유하고, 그 디바이스의 line은 역사 레지스트리(A-11)에서 `station`을 조회하여 도출한다. 이 규칙은 line/station 계층의 **SSOT를 역사 레지스트리 단일 지점**에 두기 위함이다(디바이스마다 line을 중복 기록하지 않는다).

### A-11. 역사 레지스트리는 device_metadata 패턴을 따르는 메타데이터 저장소

역사 레지스트리(station registry)는 `station` → { `line`(호선), station 표시명, 라인 다이어그램용 정렬/라인맵 위치 }를 매핑하는 설정/메타데이터 구조로, 기존 `internal/storage/device_metadata.go`(`DeviceMetadataFileRepository`, 단일 JSON 파일 + atomic write)의 패턴을 따르는 **파일 기반 저장소**로 영속화된다고 가정한다. 이 레지스트리는 line/station 계층의 **SSOT**이며, 향후 대시보드 패널이 소비한다. 초기 시드는 설정 섹션에서 로드될 수 있으나, 런타임 CRUD/lookup 표면을 통해 갱신·조회된다.

### A-12. 응답 = 상태 에코 (별도 ack가 아님), 상관은 device_id 기준

제어 명령의 "응답"은 **디바이스가 새 상태를 상태 유입 경로(state ingress)로 다시 보고하는 상태 에코(state echo)** 를 의미한다 — 별도의 명령 ack 프로토콜이 아니다. 성공 판정은 **commanded 변경이 유입 상태에 반영**되었을 때이며, 상관(correlation)은 **device_id**(+ 명령 종류)를 기준으로 한다. 에코는 **비동기**로 도착한다(특히 port 모드에서는 입력 포트를 통해). 따라서 명령 발행 직후 반환되는 것이 아니라, `control_response_timeout` 내 매칭 에코 유입 여부로 성공/타임아웃을 판정한다.

### A-13. pending-command 레지스트리는 device_id(+명령) 키잉, 명령별 타임아웃

응답 대기는 **device_id(+ 명령)로 키잉되는 pending-command 레지스트리**로 구현되며, 각 pending 항목은 자체 타임아웃을 갖는다. 상태 유입 경로가 매칭 에코를 만나면 해당 pending 명령을 resolve한다. 동일 디바이스에 대한 **동시 명령**은 명령 종류별로 구분되는 pending 항목으로 관리된다고 가정한다(예: 같은 디바이스의 set_power와 set_fan_speed는 별개 pending). 타임아웃 후 도착한 에코는 이미 만료된 pending에 대해 **무시**된다.

---

## 3. Requirements (요구사항)

REQ ID 체계: `REQ-XSFM-001-{모듈번호 2자리}-{순번 2자리}`.

### Module 1: MQTT Transport & Config (MQTT 트랜스포트 · 설정)

#### REQ-XSFM-001-01-01 (Ubiquitous) XSFMAgent 구조체

XSFMAgent 구조체는 **항상** 다음 필드를 포함해야 한다:

| 필드            | 타입                            | 설명                                    |
| ------------- | ----------------------------- | ------------------------------------- |
| `*BaseAgent`  | 임베딩                           | 기본 에이전트 기능 (생명주기, 통계)                 |
| `cfg`         | `XSFMConfig`           | 에이전트 전용 설정 (transport_mode/브로커/토픽 템플릿/페이로드 매핑) |
| `client`      | `MQTTClient`                  | 추상화된 MQTT 클라이언트 인터페이스 (Paho 래핑). **`direct` 모드에서만 생성**; `port` 모드에서는 `nil`(브로커 I/O를 외부 노드가 담당) |
| `cmdSink`     | `CommandSink`                 | 명령 출력 경계 추상화. `direct`=브로커 publish, `port`=제어 출력 포트 emit (REQ-XSFM-001-01-11) |
| `stateIngress`| (경로)                          | 상태 입력 경계. `direct`=브로커 구독 콜백, `port`=상태 노드 입력 포트 (REQ-XSFM-001-01-11) |
| `devices`     | `map[string]*Device`          | device_id 기반 디바이스 로스터                 |
| `groups`      | (도출) `map[string][]string`    | group_id → device_id 목록 (로스터 속성에서 도출) |
| `mu`          | `sync.RWMutex`                | 로스터 동시성 보호                            |
| `msgCh`       | `chan []byte`                 | 플로우로 전달할 상태 변경/이벤트 메시지 채널             |
| `stopCh`      | `chan struct{}`               | 정지 시그널 채널                             |
| `deviceIDRepo`| `DeviceIDRepository`          | 로스터 영속화 (device_id repo)             |

#### REQ-XSFM-001-01-02 (Ubiquitous) Agent / MessageReceiver 인터페이스 준수

XSFMAgent는 **항상** `agent.Agent`와 `agent.MessageReceiver` 인터페이스를 구현해야 한다. 컴파일 타임 검증 (`var _ agent.Agent = (*XSFMAgent)(nil)`)이 성공해야 한다.

#### REQ-XSFM-001-01-03 (Ubiquitous) MQTTClient 인터페이스

MQTTClient 인터페이스는 **항상** 다음 메서드를 제공해야 한다:

| 메서드           | 시그니처                                                     | 설명            |
| ------------- | -------------------------------------------------------- | ------------- |
| `Connect`     | `Connect() error`                                        | 브로커 연결        |
| `Disconnect`  | `Disconnect()`                                           | 연결 종료         |
| `Publish`     | `Publish(topic string, qos byte, payload []byte) error`  | 명령 발행         |
| `Subscribe`   | `Subscribe(topic string, qos byte, cb MessageHandler) error` | 상태 토픽 구독  |
| `IsConnected` | `IsConnected() bool`                                     | 연결 상태 확인      |

MQTT 클라이언트를 인터페이스로 추상화하여, 목(mock) 브로커 주입으로 단위 테스트가 가능해야 한다.

#### REQ-XSFM-001-01-04 (Ubiquitous) XSFMConfig 설정

XSFMConfig는 **항상** 다음 설정을 지원해야 한다 (`AgentConfig.Transport.Options` 기반 파싱, `thingplus_agent.go` 패턴 준수):

| 설정                    | 키                       | 타입       | 기본값     | 필수  | 설명                                             |
| --------------------- | ----------------------- | -------- | ------- | --- | ---------------------------------------------- |
| **트랜스포트 모드**        | `transport_mode`        | `string` | `"direct"` | No  | `direct`(에이전트가 브로커 sub/pub 소유) \| `port`(외부 mqtt-in/out 노드가 I/O 담당) (REQ-XSFM-001-01-10) |
| 브로커 주소                | `broker`                | `string` | -       | direct만 | MQTT 브로커 URL (예: `tcp://host:1883`). **`direct` 모드 필수**, `port` 모드 미사용 |
| TLS 사용                | `tls`                   | `bool`   | `false` | No  | TLS 활성화 (direct 모드)                             |
| CA 인증서                | `ca_cert`               | `string` | `""`    | No  | TLS CA 인증서 경로 (direct 모드)                       |
| 클라이언트 ID             | `client_id`             | `string` | 자동생성    | No  | MQTT 클라이언트 식별자 (direct 모드)                      |
| 사용자/비밀번호            | `username`/`password`   | `string` | `""`    | No  | 브로커 인증 (direct 모드)                              |
| QoS                   | `qos`                   | `byte`   | `1`     | No  | 발행/구독 QoS (direct 모드)                           |
| **상태 토픽 템플릿**        | `state_topic_template`  | `string` | -       | direct만 | device-id placeholder 포함 (예: `xsfm/{device_id}/state`). **`direct` 모드 필수**, `port` 모드 미사용(외부 mqtt-in 노드가 토픽 소유) |
| **명령 토픽 템플릿**        | `command_topic_template`| `string` | -       | direct만 | device-id placeholder 포함 (예: `xsfm/{device_id}/cmd`). **`direct` 모드 필수**, `port` 모드 미사용(외부 mqtt-out 노드가 토픽 소유) |
| **페이로드 필드 매핑**       | `payload_mapping`       | `object` | -       | Yes | JSON 필드 → 상태 축 매핑 (REQ-XSFM-001-01-06). **양 모드 공통 필수** (유입 상태 파싱 + 방출 제어 포맷) |
| 오프라인 임계 시간          | `offline_timeout`       | `string` | `"60s"` | No  | 상태 무수신 → 오프라인 판정 (0=비활성)                       |
| **제어 응답 대기 타임아웃**   | `control_response_timeout` | `string` | `"5s"` | No  | 제어 명령 후 상태 에코 대기 시간 (0=응답 대기 비활성, fire-and-forget). 경과 시 `ErrControlTimeout` (REQ-XSFM-001-03-08) |
| LWT 사용                | `lwt_enabled`           | `bool`   | `true`  | No  | Last Will and Testament 활용 (REQ-XSFM-001-05-03) |
| 로스터 영속화 경로          | `registry_path`         | `string` | `""`    | No  | 런타임 등록 디바이스 저장 경로 (station/place/index 포함)      |
| **역사 레지스트리 저장 경로**  | `station_registry_path` | `string` | `""`    | No  | station→line 매핑 저장소 경로 (device_metadata 패턴, REQ-XSFM-001-02-11). 빈 값이면 인메모리/설정 시드만 사용 |
| **역사 레지스트리 시드**     | `station_registry`      | `object` | -       | No  | station→{line, display_name, order} 초기 시드 항목 (REQ-XSFM-001-02-10) |

#### REQ-XSFM-001-01-05 (Ubiquitous) 설정 주도 토픽 템플릿 (시임)

`state_topic_template` / `command_topic_template`은 **항상** device-id placeholder(`{device_id}`)를 포함하는 문자열 템플릿이며, `renderTopic(template, deviceID) string` 헬퍼로 실제 토픽 문자열을 생성해야 한다.

- 구체적 토픽 스킴은 디바이스 매뉴얼 확보 후 설정 값으로 확정된다 — 설계는 이를 **하드코딩하지 않는다**.
- 템플릿에 `{device_id}` placeholder가 없으면 `ErrInvalidTopicTemplate` 에러를 반환한다.

#### REQ-XSFM-001-01-06 (Ubiquitous) 설정 주도 페이로드 필드 매핑 (시임)

`payload_mapping`은 **항상** 디바이스 상태 JSON의 어느 필드가 각 상태 축을 나르는지 선언해야 한다:

| 매핑 키              | 대상 축       | 설명                                                    |
| ------------------ | ---------- | ----------------------------------------------------- |
| `power_field`      | power      | 전원 상태 필드명 + on/off 값 표현(bool 또는 문자열/정수 매핑)          |
| `fan_speed_field`  | fan_speed  | 풍량 필드명 + 1/2/3단 값 표현                                 |
| `online_field`     | online     | (선택) 온라인 필드명. 미지정 시 LWT/타임아웃 기반 판정 (REQ-XSFM-001-05-03) |

- 구체적 페이로드 스키마는 매뉴얼 확보 후 설정으로 확정된다 — 설계에 **하드코딩하지 않는다**.
- 상태 디코딩(REQ-XSFM-001-05-02)과 제어 인코딩(REQ-XSFM-001-03-04)은 반드시 이 매핑을 통해 필드명/값 표현을 해석해야 한다.
- **양 모드 공통**: `payload_mapping`은 `direct`·`port` 모드에서 동일하게 사용된다. `direct` 모드는 추가로 토픽 템플릿(REQ-XSFM-001-01-05)을 사용하지만, `port` 모드는 토픽 템플릿을 사용하지 않고 페이로드 매핑만 사용한다.

#### REQ-XSFM-001-01-10 (Ubiquitous) transport_mode 트랜스포트 모드

시스템은 **항상** `transport_mode` 설정(`direct` | `port`, 기본 `direct`)을 지원해야 한다:

- **`direct`**: 에이전트가 MQTT(Paho) 클라이언트를 소유한다 — 브로커에 연결하고, STATE 토픽을 **구독**하며, COMMAND 토픽으로 제어 명령을 **발행**한다 (v0.1.0 동작과 동일).
- **`port`**: 에이전트가 브로커에 **직접 연결하지 않는다**. 트랜스포트가 분리(decoupled)되어 외부 플로우 노드(상류 mqtt-in 노드 · 하류 mqtt-out 노드)가 브로커 I/O를 담당한다. 디바이스 STATE는 상태 노드의 **입력 포트**로 유입되고, 제어 명령은 제어 노드의 **제어 출력 포트**로 방출된다.

`transport_mode`가 `direct`·`port` 이외의 값이면 `ErrInvalidTransportMode`를 반환한다.

#### REQ-XSFM-001-01-11 (Ubiquitous) I/O 경계 vs 공유 프로토콜/로직 레이어 분리

시스템은 **항상** 트랜스포트를 두 계층으로 분리해야 한다:

1. **I/O 경계 (transport_mode에 따라 달라지는 부분)**:
   - **상태 입력 경계(state ingress)**: `direct`=브로커 구독 콜백 → 에이전트, `port`=상태 노드 **입력 포트** → 에이전트
   - **명령 출력 경계(command egress)**: `direct`=에이전트 → 브로커 publish, `port`=에이전트 → 제어 노드 **제어 출력 포트** → mqtt-out 노드
2. **공유 프로토콜/로직 레이어 (transport_mode와 무관하게 동일)**: 페이로드 필드 매핑, 로스터, 제어 명령 구성/인코딩, 그룹 fan-out, 2-축 제어 의미론, 관측 기반 emit.

에이전트는 **순수 프로토콜/로직 레이어**이며, 상태 입력 경계·명령 출력 경계를 인터페이스(예: `CommandSink`, state ingress 경로)로 추상화하여 `direct`·`port` 두 구현을 주입할 수 있어야 한다. 이 추상화로 로직 레이어는 두 모드에서 **바이트 단위로 동일한 디코딩/인코딩 결과**를 산출해야 한다.

#### REQ-XSFM-001-01-12 (Ubiquitous) 상태 입력 포트 ≠ 제어 출력 포트 (port 모드)

`port` 모드에서 **상태 입력 포트**(디바이스 STATE 유입)와 **제어 출력 포트**(제어 명령 방출)는 **항상 서로 다른 별개의 포트**여야 한다 (명시적 요구사항). 두 포트는 단일 포트로 결합되어서는 **안 된다**:

- **상태 입력 포트**: 상류 mqtt-in 노드가 디바이스 STATE 메시지를 이 포트로 공급 → 상태 노드가 에이전트에 전달해 로스터/상태를 갱신한다 (direct 모드의 구독 콜백과 동일한 디코딩 경로).
- **제어 출력 포트**: 제어 명령(`set_power`/`set_fan_speed`/그룹 fan-out) 발생 시 제어 노드가 포맷된 MQTT 명령 메시지를 이 포트로 방출 → 하류 mqtt-out 노드가 발행한다. 그룹 fan-out 시 N개의 멤버별 명령 메시지가 이 출력 포트로 방출된다.

#### REQ-XSFM-001-01-13 (State-Driven) transport_mode별 설정 검증 분기

**IF** `transport_mode == "direct"`이면 **THEN** `broker`(비어 있으면 `ErrBrokerRequired`), `state_topic_template`·`command_topic_template`(`{device_id}` placeholder 없으면 `ErrInvalidTopicTemplate`)를 필수로 검증한다.
**IF** `transport_mode == "port"`이면 **THEN** 브로커/토픽 템플릿은 검증 대상에서 제외(선택/미사용)하되, `payload_mapping`은 **양 모드 공통 필수**로 검증한다(누락 시 `ErrInvalidPayloadMapping`).

#### REQ-XSFM-001-01-07 (Event-Driven) Init 생명주기

**WHEN** `Init(config)` 호출 시 **THEN**:

1. `config.Transport.Options`에서 `XSFMConfig`를 파싱·검증한다 (`parseXSFMConfig`). `transport_mode`를 파싱하고(기본 `direct`) REQ-XSFM-001-01-13에 따라 모드별로 검증한다
2. **IF** `direct`이면 토픽 템플릿에 `{device_id}` placeholder가 있는지 검증한다. `payload_mapping`은 양 모드 공통으로 유효성 검증한다
3. **IF** `direct`이면 MQTT 클라이언트를 생성하고(브로커/TLS/client_id/LWT 설정 적용) 명령 출력 경계를 브로커 publish로, 상태 입력 경계를 구독 콜백으로 배선한다. **IF** `port`이면 MQTT 클라이언트를 생성하지 않고(`client=nil`) 명령 출력 경계를 제어 출력 포트 emit으로, 상태 입력 경계를 입력 포트로 배선한다
4. 설정 기반 디바이스를 로스터에 등록한다 (`Source="config"`)
5. `registry_path` 파일이 존재하면 런타임 등록 디바이스를 로드한다
6. `BaseAgent.Init(config)`를 호출하여 상태를 `Running`으로 전이한다

#### REQ-XSFM-001-01-08 (Event-Driven) Start 생명주기 및 MQTT 재연결 (direct 모드)

**WHEN** `Start(ctx)` 호출 시 **AND** `transport_mode == "direct"`이면 **THEN**:

1. `client.Connect()`를 호출하여 브로커 연결을 시도한다
2. **WHEN** 연결 성공하면 **THEN** 모든 등록 디바이스의 state 토픽을 구독하고(REQ-XSFM-001-05-01), 오프라인 감지 루프를 시작한다
3. **WHEN** 초기 연결 실패 또는 런타임 연결 끊김 시 **THEN** Paho 자동 재연결 메커니즘(`thingplus_agent.go` 패턴)으로 재연결하고, 재연결 시 구독을 복원한다. `Start()`는 초기 연결 실패 시에도 에러를 반환하지 않고 재연결 상태로 진입한다.

**WHEN** `Start(ctx)` 호출 시 **AND** `transport_mode == "port"`이면 **THEN** 브로커 연결/구독을 수행하지 않는다. 상태 유입은 입력 포트를 통해 이루어지며, 오프라인 감지는 `offline_timeout` 기반 타임아웃 경로만 사용한다(LWT는 브로커 세션에 종속되므로 port 모드에서는 비활성). `Start()`는 정상적으로 Running 상태를 유지한다.

#### REQ-XSFM-001-01-09 (Event-Driven) Stop / Pause / Resume 생명주기

**WHEN** `Stop(ctx)` 호출 시 **THEN** `stopCh`를 닫고, 구독을 해제하며, MQTT 연결을 종료하고, 상태를 `Stopped`로 전이한다.
**WHEN** `Pause(ctx)` 호출 시 **THEN** 명령 발행을 일시 중지하되 구독/연결은 유지한다.
**WHEN** `Resume(ctx)` 호출 시 **THEN** 명령 발행을 재개한다.

---

### Module 2: Device Roster & Persistence (디바이스 로스터 · 영속화)

#### REQ-XSFM-001-02-01 (Ubiquitous) Device 구조체

Device 구조체는 **항상** 다음 필드를 포함해야 한다:

| 필드           | 타입          | 설명                                          |
| ------------ | ----------- | ------------------------------------------- |
| `DeviceID`   | `string`    | 디바이스 식별자 (로스터 기본 키)                         |
| `Name`       | `string`    | 표시 이름                                       |
| `GroupID`    | `string`    | 선택적 그룹 식별자/tag (빈 값이면 미소속)                  |
| `Station`    | `string`    | (선택) 소속 역사(station) 식별자 — 역사 레지스트리 엔트리 참조 (REQ-XSFM-001-02-07) |
| `Place`      | `string`    | (선택) 역사 내 위치/구역 (예: 승강장/대합실/출구)             |
| `Index`      | `int`       | (선택) station(+place) 범위 내 디바이스 순번 (전역 유일 아님) |
| `Power`      | `bool`      | 전원 상태 (관측된 경우)                              |
| `FanSpeed`   | `int`       | 풍량 1/2/3 (power=ON일 때만 유효)                  |
| `Online`     | `bool`      | 온라인 상태                                      |
| `LastSeen`   | `time.Time` | 마지막 상태 수신 시각                                |
| `Source`     | `string`    | 등록 출처: `"config"`, `"bridge"`, `"auto"`     |
| `observed`   | (내부)        | 각 축의 관측 여부 플래그 (관측 기반 emit용)               |

#### REQ-XSFM-001-02-02 (Ubiquitous) 로스터 조회 메서드

시스템은 **항상** 다음 조회를 제공해야 한다:

- `ListDevices() []Device`: 등록된 전체 디바이스 목록 (RWMutex 보호)
- `GetDevice(deviceID string) (*Device, error)`: device_id로 조회. 미등록 시 `ErrDeviceNotFound`
- `GroupMembers(groupID string) []string`: group_id에 속한 device_id 목록 (로스터 속성에서 도출)

#### REQ-XSFM-001-02-03 (Event-Driven) 설정 기반 디바이스 등록

**WHEN** `Init()` 시 설정에 디바이스 목록이 있으면 **THEN** 각 엔트리(`device_id`, `name`, `group_id`)를 로스터에 등록하고 `Source="config"`로 설정하며, `Online=false`로 초기화한다.

#### REQ-XSFM-001-02-04 (Event-Driven) 런타임 디바이스 등록/제거/수정

**WHEN** Bridge를 통해 `add_device` 명령이 `Process(data)`로 수신되면 **THEN** `device_id`(+선택 `name`/`group_id`/`station`/`place`/`index`)로 `Device`를 생성하고 `Source="bridge"`로 등록한다. 이미 존재하는 device_id이면 `ErrDeviceAlreadyRegistered`를 반환한다. 등록 성공 시 state 토픽을 구독하고 `device_registered` 이벤트를 `msgCh`에 전달한다.

**WHEN** `remove_device` 명령이 수신되면 **THEN** 대상 device_id를 로스터에서 제거하고 state 토픽 구독을 해제한다. `Source="config"` 디바이스는 제거할 수 **없다** (`ErrConfigDeviceProtected`). 제거 성공 시 `device_unregistered` 이벤트를 전달한다.

**WHEN** `set_device` 명령이 수신되면 **THEN** 대상 device_id의 속성(`name`, `group_id`, `station`, `place`, `index`)을 갱신한다. `group_id`가 변경되면 그룹 도출 결과에, `station`이 변경되면 station/line 기반 대상 선정 결과에 즉시 반영되어야 한다.

#### REQ-XSFM-001-02-05 (Event-Driven) 로스터 영속화 라운드트립

**WHEN** `registry_path`가 설정되어 있고 로스터에 변경이 발생하면 **THEN** `Source`가 `"bridge"` 또는 `"auto"`인 디바이스(device_id, name, group_id 포함)를 영속화한다 (`GetPersistableDevices` 패턴). `Source="config"` 디바이스는 저장에서 제외한다.

**WHEN** `Init()` 시 영속화 파일이 존재하면 **THEN** 저장된 디바이스를 로스터에 복원하되, 설정 기반 디바이스와 device_id가 충돌하면 설정이 우선한다.

#### REQ-XSFM-001-02-06 (Ubiquitous) 로스터 목록 조회 명령

`list_devices` 명령은 **항상** 등록된 전체 디바이스(device_id, name, group_id, **station, place, index**, online, power, fan_speed, source)를 JSON으로 반환해야 한다.

#### REQ-XSFM-001-02-07 (Ubiquitous) 디바이스 위치 계층 속성 (station/place/index)

Device 구조체는 **항상** 선택적 위치 계층 속성 `Station`/`Place`/`Index`를 지원해야 한다 (REQ-XSFM-001-02-01):

- `Station`: 디바이스가 소속된 역사 식별자. 역사 레지스트리(REQ-XSFM-001-02-09) 엔트리를 참조한다. line(호선)은 디바이스에 **직접 저장하지 않으며**, `Station`을 레지스트리에서 조회하여 해석한다 (A-10).
- `Place`: 역사 내 위치/구역 (예: 승강장/대합실/출구 구역).
- `Index`: `Station`(+`Place`) 범위 내 디바이스 순번 (전역 유일 번호가 아니라 역사별 시퀀스, A-9).
- 세 속성은 **선택/하위호환**이다 — 미지정 시 빈 문자열/0으로 두며, 기존 v0.2.0 로스터와 동작이 일치해야 한다.
- `add_device` / `set_device`(REQ-XSFM-001-02-04)는 `station`/`place`/`index`를 선택 파라미터로 수용·갱신해야 한다.

#### REQ-XSFM-001-02-08 (Event-Driven) 위치 속성 영속화 라운드트립

**WHEN** `registry_path`가 설정되어 있고 로스터가 영속화될 때 **THEN** `Station`/`Place`/`Index`를 device_id/name/group_id와 함께 저장·복원해야 한다 (REQ-XSFM-001-02-05 라운드트립 확장). 저장 포맷에 위치 속성 필드가 없던 기존 파일을 로드할 때는 빈 값/0으로 하위호환 복원한다.

---

### Module 2B: Station Registry (역사 레지스트리 · station → line 호선 매핑)

> **NET-NEW (v0.3.0)**. line(호선)/station 계층의 **SSOT**. 디바이스는 `station`만 보유하고, line은 이 레지스트리의 station→line 매핑으로 해석된다(A-10). REQ 모듈번호는 Module 2에 이어 `02-09`~`02-13`을 사용한다(디바이스 모델의 연장).

#### REQ-XSFM-001-02-09 (Ubiquitous) StationEntry 구조체

역사 레지스트리 엔트리는 **항상** 다음 필드를 포함해야 한다:

| 필드            | 타입       | 설명                                                        |
| ------------- | -------- | --------------------------------------------------------- |
| `Station`     | `string` | 역사 식별자 (레지스트리 기본 키, 디바이스 `Station`이 참조)                    |
| `Line`        | `string` | 소속 호선(line) 식별자 — station의 상위 집계 레벨                        |
| `DisplayName` | `string` | 역사 표시명 (UI/대시보드 소비용)                                       |
| `Order`       | `int`    | 라인 다이어그램/라인맵 상 역사 정렬 위치 (호선 내 순서)                          |

#### REQ-XSFM-001-02-10 (Ubiquitous) 역사 레지스트리 설정 시드

시스템은 **항상** `station_registry` 설정 객체를 통해 초기 station→{line, display_name, order} 시드를 로드할 수 있어야 한다. 시드는 선택이며, 미지정 시 빈 레지스트리로 시작한다.

#### REQ-XSFM-001-02-11 (Ubiquitous) 역사 레지스트리 저장소 (device_metadata 패턴) + CRUD/Lookup 표면

역사 레지스트리는 **항상** `internal/storage/device_metadata.go`의 `DeviceMetadataFileRepository` 패턴(단일 JSON 파일 + atomic write + 인메모리 캐시)을 따르는 저장소로 영속화되어야 하며(`station_registry_path` 설정, 예: `{dir}/station_registry.json`), 다음 표면을 제공해야 한다:

| 메서드                          | 설명                                                    |
| ------------------------------ | ----------------------------------------------------- |
| `UpsertStation(entry)`         | station 엔트리 추가/갱신 (station→line, 표시명, 정렬)             |
| `RemoveStation(station)`       | station 엔트리 제거                                        |
| `GetStation(station) (Entry, error)` | station 엔트리 조회. 미등록 시 `ErrStationNotFound`        |
| `ResolveLine(station) (string, error)` | station→line 해석 (SSOT lookup). 미등록 시 `ErrStationNotFound` |
| `ListStations() []StationEntry` | 전체 역사 목록 (정렬 포함) — 대시보드/라인맵 소비용                       |
| `StationsByLine(line) []StationEntry` | 특정 호선에 속한 역사 목록 (Order 정렬)                          |

런타임 명령 표면(`add_station`/`remove_station`/`list_stations`)으로도 CRUD가 가능해야 하며, 레지스트리는 device_metadata와 **별개 저장소**로 관리된다.

#### REQ-XSFM-001-02-12 (Ubiquitous) 라인 계층 SSOT — 디바이스는 station만 보유

시스템은 **항상** line(호선)을 디바이스가 아닌 역사 레지스트리에서만 SSOT로 관리해야 한다. 디바이스의 line 질의는 `ResolveLine(device.Station)`로 도출하며, 디바이스 로스터에 line을 중복 저장하지 **않는다**. `station_registry`와 `device_metadata`는 서로 다른 저장소로 분리되어, station 계층(레지스트리)과 디바이스 개별 메타데이터(로스터)의 관심사를 겹치지 않게 유지한다.

#### REQ-XSFM-001-02-13 (Unwanted) 미등록 station 참조 처리

시스템은 역사 레지스트리에 없는 station으로의 line 해석을 **성공으로 처리하지 않아야 한다**. `ResolveLine`/`GetStation`은 `ErrStationNotFound`를 반환한다. 디바이스가 미등록 station을 참조해도 디바이스 등록 자체는 거부하지 않으나(위치 속성은 선택), line 기반 일괄 제어(REQ-XSFM-001-04-06) 대상 선정 시 미등록 station의 디바이스는 대상에서 제외되고 그 사실이 집계 응답/로그에 표기되어야 한다.

---

### Module 3: Per-Device Control (개별 디바이스 제어 · 2-축)

#### REQ-XSFM-001-03-01 (Ubiquitous) 2-축 상태 모델

디바이스 런타임 상태는 **항상** 2-축 `{power: on/off, fan_speed: 1|2|3, online: bool}`으로 모델링되어야 한다. **풍량은 전원이 ON일 때만 유효**하다.

#### REQ-XSFM-001-03-02 (Event-Driven) set_power 제어

**WHEN** `set_power` 명령이 `Process(data)`로 수신되면 **THEN**:

1. 대상 device_id를 확인한다 (미등록 시 `ErrDeviceNotFound`)
2. `payload_mapping.power_field`에 따라 on/off 값을 인코딩한 제어 페이로드를 구성한다 (양 모드 공통 인코딩 경로)
3. **`control_response_timeout > 0`이면** 명령 발행 전에 device_id(+명령)로 pending-command 항목을 등록한다 (REQ-XSFM-001-03-08)
4. 구성한 명령 메시지를 **명령 출력 경계로 방출**한다 (REQ-XSFM-001-01-11): `direct` 모드는 `renderTopic(command_topic_template, device_id)` 토픽으로 MQTT **발행**, `port` 모드는 제어 노드의 **제어 출력 포트로 emit**(하류 mqtt-out 노드가 발행)
5. **`control_response_timeout > 0`이면** 상태 에코 도착(REQ-XSFM-001-03-09) 또는 타임아웃(REQ-XSFM-001-03-10)까지 대기하여 성공/`ErrControlTimeout`을 판정한다. `= 0`이면 발행 즉시 fire-and-forget으로 성공 반환한다
6. 제어 감사 레코드를 기록한다 (REQ-XSFM-001-06-03) — 응답 대기 결과(에코 반영/타임아웃) 포함
7. 결과를 JSON으로 반환한다 (`status`: `ok` | `timeout`)

#### REQ-XSFM-001-03-03 (Event-Driven) set_fan_speed 제어 (1/2/3단)

**WHEN** `set_fan_speed` 명령이 수신되면 **THEN**:

1. 풍량 값이 `1`, `2`, `3` 중 하나인지 검증한다 (아니면 `ErrInvalidFanSpeed`)
2. `payload_mapping.fan_speed_field`에 따라 값을 인코딩하여 **명령 출력 경계로 방출**한다 (`direct`=명령 토픽 발행, `port`=제어 출력 포트 emit)
3. 제어 감사 레코드를 기록한다

**IF** 대상 디바이스의 전원이 OFF 상태이면 **THEN** 풍량 명령은 전원이 ON일 때만 유효하다는 의미론에 따라, 시스템은 (a) `set_fan_speed`를 거부(`ErrPowerOff`)하거나 (b) 전원 ON을 선행 발행 후 풍량을 발행하는 정책 중 하나를 **설정 가능**하게 제공해야 한다. 기본 정책은 거부(`ErrPowerOff`)이다.

#### REQ-XSFM-001-03-04 (Event-Driven) 복합 제어 (set_multiple)

**WHEN** `set_multiple` 명령이 수신되면 **THEN** `params`에 포함된 `power` 및/또는 `fan_speed`를 함께 처리한다. `power=true`와 `fan_speed`가 동시에 지정되면 전원 ON을 먼저 반영한 뒤 풍량을 발행한다.

#### REQ-XSFM-001-03-05 (Unwanted) 유효하지 않은 풍량 값 거부

시스템은 `1`, `2`, `3` 이외의 풍량 값을 **수락하지 않아야 한다**. `ErrInvalidFanSpeed` 에러를 반환한다.

#### REQ-XSFM-001-03-06 (Unwanted) 미등록 디바이스 제어 거부

시스템은 로스터에 등록되지 않은 device_id로의 제어 명령을 **수락하지 않아야 한다**. `ErrDeviceNotFound` 에러를 반환한다.

#### REQ-XSFM-001-03-07 (Event-Driven) Process 명령 표면

`Process(data []byte)`는 **항상** 다음 JSON 명령을 지원해야 한다: `set_power`, `set_fan_speed`, `set_multiple`, `add_device`, `remove_device`, `set_device`, `list_devices`, `request_state`, **`add_station`, `remove_station`, `list_stations`**(역사 레지스트리 CRUD, REQ-XSFM-001-02-11). 제어 명령 대상은 `device_id` **또는** 셀렉터 `group_id` / `station` / `line`으로 지정할 수 있다 (셀렉터 fan-out은 Module 4, 우선순위 REQ-XSFM-001-04-04).

**제어 명령 예시 (개별):**

```json
{ "command": "set_fan_speed", "device_id": "ap-station-101", "params": { "fan_speed": 2 } }
```

#### REQ-XSFM-001-03-08 (Ubiquitous) 제어 응답 대기 · pending-command 레지스트리 (NET-NEW)

시스템은 **항상** `control_response_timeout > 0`일 때 제어를 **응답 대기(state echo 대기)** 로 수행해야 한다:

- 제어 명령 발행 전에 **device_id(+ 명령 종류)로 키잉되는 pending-command 항목**을 pending 레지스트리에 등록하고, 각 항목에 자체 타임아웃(`control_response_timeout`)을 부여한다 (A-13).
- 상태 유입 경로(REQ-XSFM-001-05-01, `direct`=구독 콜백 / `port`=입력 포트)가 매칭 상태 에코를 만나면 해당 pending 항목을 resolve한다.
- "응답"은 별도 ack가 아니라 **디바이스가 새 상태를 상태 유입으로 다시 보고하는 상태 에코**이며, 상관은 device_id(+명령) 기준이다 (A-12).
- 동일 디바이스에 대한 **동시 명령**은 명령 종류별로 구분되는 별개 pending 항목으로 관리된다 (예: 같은 디바이스의 set_power와 set_fan_speed는 독립 pending).
- `control_response_timeout == 0`이면 응답 대기를 비활성화하고 발행 즉시 성공(fire-and-forget)으로 처리한다.

#### REQ-XSFM-001-03-09 (Event-Driven) 응답 대기 성공 (에코가 명령과 일치)

**WHEN** pending 명령이 등록된 디바이스에 대해 상태 에코가 유입되면 **AND** 유입 상태가 commanded 변경을 반영하면(예: `set_power true` 후 유입 상태 power=true, correlated by device_id) **THEN** 타임아웃 이전이라면 해당 pending 명령을 **성공(`ok`)** 으로 resolve하고 대기를 종료해야 한다.

- `port` 모드에서 에코는 **상태 입력 포트**를 통해 비동기로 도착하며, `direct` 모드와 **동일한 디코딩·상관 경로**로 pending을 resolve한다.

#### REQ-XSFM-001-03-10 (Unwanted / Complex) 응답 타임아웃 · 늦은 에코 · 동시성

**IF** pending 명령이 `control_response_timeout` 내에 매칭 에코로 resolve되지 못하면 **THEN** 시스템은 해당 명령을 **`ErrControlTimeout`** 으로 실패 처리하고 pending 항목을 제거해야 한다 (`status: "timeout"`).

- **타임아웃 후 도착한 에코**는 이미 만료·제거된 pending에 대해 **무시**되어야 한다(정상 상태 갱신은 수행하되 pending resolve로 소급 처리하지 않음).
- **동시 명령**: 같은 디바이스에 대한 서로 다른 명령 종류의 pending은 각각 독립적으로 성공/타임아웃 판정되어야 하며, 하나의 resolve/timeout이 다른 pending에 영향을 주어서는 **안 된다**.

---

### Module 4: Group Control — Agent-Side Fan-out (그룹 제어 · NET-NEW)

#### REQ-XSFM-001-04-01 (Event-Driven) 그룹/셀렉터 대상 명령 fan-out

**WHEN** 제어 명령(`set_power` / `set_fan_speed` / `set_multiple`)이 `device_id` 대신 **셀렉터**(`group_id` **또는** `station` **또는** `line`)로 지정되어 수신되면 **THEN**:

1. 셀렉터로 대상 멤버 device_id 목록을 도출한다: `group_id`→`GroupMembers(group_id)`, `station`→`DevicesByStation(station)`, `line`→`DevicesByLine(line)`(REQ-XSFM-001-04-05/04-06, station registry 경유)
2. 각 멤버 디바이스에 대해 **개별 명령 메시지를 명령 출력 경계로 방출**한다 (per-device fan-out — 하드웨어 브로드캐스트가 아님). `direct` 모드는 멤버별 MQTT **발행**, `port` 모드는 멤버별 명령 메시지를 **제어 출력 포트로 emit**(N개 멤버 → N개 메시지)
3. **각 멤버 명령은 자체 응답 대기(REQ-XSFM-001-03-08~10)를 수행**한다 — 멤버별로 pending 등록 → 에코 대기 → 성공/`ErrControlTimeout` 판정
4. 멤버별 결과(성공/실패/타임아웃)를 수집하여 집계 응답으로 반환한다 (REQ-XSFM-001-04-07)
5. 그룹/셀렉터 제어 감사 레코드를 기록한다 (대상 셀렉터 종류·값 + 멤버 목록 + 명령 내용)

> **NET-NEW**: 본 요구사항은 코드베이스에 선례가 없는 신규 동작이다. Samsung 에이전트(`agent.go:530` 부근)가 `group_id`를 forward-compat 차원에서 수용하되 무시하는 현재 상태를 확장하여, 실제 fan-out을 정의한다. v0.3.0에서 셀렉터를 `group_id` 외에 `station`/`line`으로 확장한다 — 이는 향후 대시보드의 라인/역사 패널 "일괄제어"가 호출할 표면이다(패널 UI 자체는 SPEC-FACILITY-DASHBOARD-001).

#### REQ-XSFM-001-04-02 (State-Driven) 빈 그룹 처리

**IF** 지정된 group_id에 멤버 디바이스가 하나도 없으면 **THEN** 시스템은 `ErrEmptyGroup` 에러(또는 빈 멤버 목록의 no-op 집계 응답)를 반환하고, 어떠한 MQTT 발행도 수행하지 않아야 한다.

#### REQ-XSFM-001-04-03 (Complex) 부분 실패 의미론 (partial-failure)

**WHILE** 그룹 fan-out이 진행 중일 때 **WHEN** 일부 멤버의 발행이 실패하면 **THEN**:

1. 시스템은 전체를 중단하지 않고 나머지 멤버에 대한 발행을 계속 시도해야 한다 (best-effort fan-out)
2. 집계 응답에 멤버별 성공/실패 상태를 포함해야 한다 (`{"group_id": "...", "results": [{"device_id": "...", "status": "ok|error", "error": "..."}]}`)
3. 하나 이상 실패가 있으면 응답의 최상위 상태를 `partial` 또는 `error`로 표기해야 한다

#### REQ-XSFM-001-04-04 (Ubiquitous) 셀렉터 대상 지정 우선순위

제어 명령에 여러 대상 셀렉터가 동시에 지정되면 시스템은 **항상** 다음 우선순위로 단일 해석해야 한다: **`device_id`(단일) > `station` > `line` > `group_id`**. 즉 `device_id`가 있으면 단일 대상으로 처리하고, 없으면 `station` → `line` → `group_id` 순으로 첫 번째로 지정된 셀렉터로 fan-out한다. 둘 이상의 셀렉터가 모호하게 지정된 경우 이 우선순위로 결정론적으로 해석된다.

**셀렉터 명령 예시:**

```json
{ "command": "set_power", "group_id": "concourse-b1", "params": { "power": true } }
{ "command": "set_power", "station": "ST-101", "params": { "power": true } }
{ "command": "set_fan_speed", "line": "line-2", "params": { "fan_speed": 1 } }
```

#### REQ-XSFM-001-04-05 (Event-Driven) station 셀렉터 일괄 제어 (NET-NEW)

**WHEN** 제어 명령이 `station`으로 지정되면 **THEN** 시스템은 `DevicesByStation(station)`로 해당 역사에 소속된(디바이스의 `Station` 속성이 일치하는) 모든 로스터 디바이스를 도출하여 per-device fan-out(각 멤버 응답 대기 포함)을 수행해야 한다. 멤버가 없으면 `ErrEmptyGroup` 의미론(REQ-XSFM-001-04-02)과 동일하게 no-op 집계/`ErrEmptyGroup`을 반환한다.

#### REQ-XSFM-001-04-06 (Event-Driven) line(호선) 셀렉터 일괄 제어 (NET-NEW)

**WHEN** 제어 명령이 `line`으로 지정되면 **THEN** 시스템은 다음으로 대상을 도출해야 한다:

1. 역사 레지스트리에서 `StationsByLine(line)`로 해당 호선에 속한 station 목록을 도출한다
2. 각 station에 대해 `DevicesByStation(station)`로 디바이스를 모아 line 전체 대상 집합을 만든다 (즉 line→stations→devices, station→line SSOT를 역방향 조회)
3. 대상 집합에 per-device fan-out(각 멤버 응답 대기 포함)을 수행한다

미등록 station을 참조하는 디바이스는 대상에서 제외되고 그 사실이 집계 응답에 표기된다 (REQ-XSFM-001-02-13). 해당 호선에 대상 디바이스가 하나도 없으면 `ErrEmptyGroup` 의미론을 따른다.

#### REQ-XSFM-001-04-07 (Complex) 멤버별 응답 대기 집계 (partial + timeout)

**WHILE** 셀렉터 fan-out이 진행 중일 때 **WHEN** 각 멤버가 자체 응답 대기를 수행하면 **THEN** 집계 응답은 멤버별로 `ok` | `error` | `timeout` 상태를 포함해야 한다:

```json
{
  "selector": { "type": "line", "value": "line-2" },
  "results": [
    { "device_id": "ap-101", "status": "ok" },
    { "device_id": "ap-102", "status": "timeout", "error": "ErrControlTimeout" },
    { "device_id": "ap-103", "status": "error", "error": "..." }
  ],
  "status": "partial"
}
```

- best-effort: 일부 멤버가 타임아웃/실패해도 나머지 멤버 처리를 계속한다 (REQ-XSFM-001-04-03 확장).
- 하나 이상 `error`/`timeout`이 있으면 최상위 `status`를 `partial`(또는 `error`)로 표기한다.
- 그룹/셀렉터 감사 레코드에 멤버별 성공/타임아웃 결과를 포함한다.

#### REQ-XSFM-001-04-08 (Ubiquitous) 셀렉터 fan-out은 개별 제어와 동일 의미론 재사용

station/line/group 셀렉터 fan-out은 **항상** 개별 제어(Module 3)의 값 검증·2-축·응답 대기·인코딩 경로를 멤버마다 재사용해야 한다. 셀렉터는 오직 **대상 집합 선정**만 담당하며, 멤버별 제어 실행 로직은 개별 제어와 동일하다(중복 구현 금지).

---

### Module 5: Status Monitoring (상태 모니터링)

#### REQ-XSFM-001-05-01 (Event-Driven) 상태 유입 → 로스터 갱신 (양 모드)

**WHEN** 디바이스가 로스터에 등록되면(설정/런타임/자동) **AND** `transport_mode == "direct"`이면 **THEN** `renderTopic(state_topic_template, device_id)`로 해당 디바이스의 state 토픽을 구독한다. **`port` 모드에서는 구독을 수행하지 않으며**, 상태는 상태 노드의 **입력 포트**를 통해 유입된다(외부 mqtt-in 노드가 공급).

**WHEN** 상태 입력 경계로 디바이스 STATE 메시지가 유입되면(`direct`=구독 콜백, `port`=입력 포트) **THEN** (양 모드 동일 디코딩 경로):

1. `payload_mapping`에 따라 페이로드 JSON에서 power/fan_speed/(online) 값을 디코딩한다
2. 대상 디바이스의 로스터 상태와 `LastSeen`을 갱신한다
3. 관측된 축의 `observed` 플래그를 설정한다 (관측 기반 emit)
4. **해당 device_id에 pending-command가 있고 유입 상태가 commanded 변경을 반영하면 pending을 성공으로 resolve한다 (REQ-XSFM-001-03-09 응답 대기 에코 경로)**
5. 이전 상태 대비 변경이 있으면 `device_state_changed` 메시지를 `msgCh`에 전달한다 (텔레메트리는 Module 6)

#### REQ-XSFM-001-05-02 (Ubiquitous) 관측 기반 상태 emit

상태 emit 시 시스템은 **항상** 관측된 축만 payload에 포함해야 한다. 아직 관측되지 않은 축(예: 재시작 직후 fan_speed 미수신)은 기본값을 방출하지 않고 payload에서 생략한다 (Samsung `StateForJSON` 규칙 준수). `power=false`이면 신뢰할 수 없는 `fan_speed`는 payload에서 생략할 수 있다.

#### REQ-XSFM-001-05-03 (Event-Driven) 온라인/오프라인 감지 (LWT + 타임아웃)

시스템은 **항상** 다음 두 경로로 오프라인을 감지해야 한다:

1. **MQTT LWT** (`direct` 모드 전용): `lwt_enabled=true`이면 LWT 토픽/메시지를 활용하여 디바이스(또는 브로커 세션) 단절 시 오프라인으로 판정한다. `port` 모드에서는 에이전트가 브로커 세션을 소유하지 않으므로 LWT 경로가 비활성이며, 타임아웃 경로만 사용한다.
2. **상태 타임아웃** (양 모드 공통): `offline_timeout` 동안 해당 디바이스의 state 메시지가 유입되지 않으면 오프라인으로 판정한다 (0이면 비활성). `port` 모드에서는 입력 포트로의 상태 무유입을 기준으로 판정한다.

**WHEN** 오프라인으로 판정되면 **THEN** `Online=false`로 변경하고 `device_offline` 이벤트를 `msgCh`에 전달한다.
**WHEN** 오프라인 디바이스가 다시 상태를 보고하면 **THEN** `Online=true`로 복원하고 `device_online` 이벤트를 전달한다.

#### REQ-XSFM-001-05-04 (Event-Driven) request_state 온디맨드 조회

**WHEN** `request_state` 명령이 수신되면 **THEN** 대상 디바이스(또는 전체)의 현재 캐시된 로스터 상태를 즉시 JSON으로 반환한다 (MQTT 통신 없음, 캐시 응답).

---

### Module 6: Logging (로깅 · 상태 시계열 + 제어 감사)

#### REQ-XSFM-001-06-01 (Event-Driven) 상태 시계열 emit (status 노드 → influxdb_write)

**WHEN** power/fan_speed/online 상태가 변경되면 **THEN** 에이전트는 상태 변경 메시지를 `msgCh`로 emit하고, status 노드가 이를 플로우로 전달하여 `influxdb_write` 노드를 통해 시계열로 기록되도록 해야 한다.

시스템은 InfluxDB에 **직접 기록하지 않아야 한다** — 텔레메트리는 반드시 `에이전트 → status 노드 → influxdb_write 노드` 경로를 따른다 (SPEC-INFLUX-002 참조).

#### REQ-XSFM-001-06-02 (Ubiquitous) 상태 메시지 포맷

상태 변경/보고 메시지는 **항상** JSON이며 최소한 다음을 포함해야 한다: `type`(예: `device_state_changed`), `device_id`, `group_id`(있으면), `online`, 관측된 상태 축(`power`, `fan_speed`), `changed_fields`, `timestamp`(epoch milliseconds, int64 — 프로젝트 컨벤션).

#### REQ-XSFM-001-06-03 (Event-Driven) 제어 감사 기록

**WHEN** 제어 명령(개별 또는 그룹)이 처리되면 **THEN** 시스템은 제어 감사 레코드(who/when/what/대상 device_id 또는 group_id)를 감사 경로(`internal/storage/remote_audit_repository.go`)로 기록해야 한다.

그룹 제어의 경우 감사 레코드는 대상 group_id와 fan-out된 멤버 목록을 포함해야 한다.

#### REQ-XSFM-001-06-04 (Ubiquitous) 오프라인/스테일 모니터링 정합

온라인/오프라인 상태 변화(REQ-XSFM-001-05-03)는 **항상** 이벤트 메시지로 emit되어, SPEC-HVACR-CONNSTATE-001의 offline/stale 모니터링 규약과 정합해야 한다.

---

### Module 7: Registration & Wiring (타입 등록 · 배선)

#### REQ-XSFM-001-07-01 (Ubiquitous) 에이전트 타입 등록 함수

`RegisterXSFMTypes(registry *agent.DefaultManager)` 함수는 **항상** `"xsfm"` 타입을 에이전트 팩토리에 등록해야 한다 (Samsung `RegisterSamsungHvacr01Types` 패턴).

#### REQ-XSFM-001-07-02 (Event-Driven) main.go 배선

**WHEN** `cmd/xflowd/main.go`의 에이전트 타입 등록 지점(다른 HVACR 타입 등록과 동일한 위치, `:376-400` 부근)에서 초기화 시 **THEN** `RegisterXSFMTypes`가 호출되어야 한다. (프로젝트 컨벤션: RegisterXxxTypes는 정의만으로 부족하며 main.go에서 반드시 호출되어야 함)

#### REQ-XSFM-001-07-03 (Ubiquitous) API 어댑터

`internal/api/service/agent_adapter.go`는 **항상** `"xsfm"` 타입의 에이전트 상태/설정을 REST/inventory 표면으로 노출할 수 있어야 한다 (기존 어댑터 패턴 준수).

#### REQ-XSFM-001-07-04 (Ubiquitous) 프론트엔드 에이전트 스키마

`web/src/config/agentSchemas.ts`는 **항상** 신규 `XSFM_FIELDS` 배열과 에이전트 타입 엔트리(`{ value: 'xsfm', label: 'Subway Facilities Manager' }`)를 포함해야 한다. 필드에는 브로커/TLS/토픽 템플릿/페이로드 매핑/QoS/offline_timeout 등 설정이 노출되어야 한다. i18n 문자열(en/ko)도 추가한다.

#### REQ-XSFM-001-07-05 (Ubiquitous) 플로우 노드 등록 (입력/출력 포트 포함)

`internal/node/xsfm_status.go`(상태 emit)와 `internal/node/xsfm_control.go`(제어)가 **항상** 노드 레지스트리에 등록되어야 한다 (Samsung/LG status+control 노드 분리 패턴을 **입력/출력 포트 개념으로 확장**).

- **상태 노드(xsfm_status)**: 상태 텔레메트리를 플로우로 emit하는 기존 역할에 더해, `port` 모드에서 디바이스 STATE가 유입되는 **상태 입력 포트**를 갖는다. 상류 mqtt-in 노드가 이 입력 포트에 연결되며, 유입 메시지를 에이전트에 전달해 로스터/상태를 갱신한다. `direct` 모드에서는 이 입력 포트가 사용되지 않고, 상태 노드는 에이전트의 브로커 구독에서 상태를 받아 emit한다.
- **제어 노드(xsfm_control)**: 제어 명령을 처리하는 기존 역할에 더해, `port` 모드에서 포맷된 MQTT 명령 메시지를 방출하는 **제어 출력 포트**를 갖는다. 하류 mqtt-out 노드가 이 출력 포트에 연결되어 실제 발행을 수행한다. `direct` 모드에서는 에이전트가 직접 브로커에 발행한다.
- **상태 입력 포트와 제어 출력 포트는 서로 다른 별개의 포트**여야 한다 (REQ-XSFM-001-01-12).

---

### Module 8: Device Adapter CommandSpec (디바이스 어댑터)

#### REQ-XSFM-001-08-01 (Ubiquitous) CommandSpec 선언

`internal/device/adapter/xsfm.go`는 **항상** 다음 CommandSpec을 선언해야 한다:

| 명령            | 타입     | 값                       | 설명       |
| ------------- | ------ | ----------------------- | -------- |
| `set_power`   | bool   | `true`/`false`          | 전원 on/off |
| `set_fan_speed` | enum | `["1", "2", "3"]`       | 풍량 1/2/3단 |

#### REQ-XSFM-001-08-02 (Ubiquitous) CommandExecutor 브리지

디바이스 어댑터의 CommandExecutor는 **항상** CommandSpec 명령을 에이전트의 `Process()` JSON 명령으로 변환하여 실행해야 한다 (Samsung NASADeviceAdapter 패턴). 어댑터 레지스트리에 `"xsfm"` 키로 등록된다.

---

### Module 9: Error Handling (에러 처리)

#### REQ-XSFM-001-09-01 (Ubiquitous) 센티널 에러 정의

다음 센티널 에러가 **항상** 정의되어야 한다 (`errors.New()` + `errors.Is()` 호환):

| 에러 변수                        | 설명                              |
| ---------------------------- | ------------------------------- |
| `ErrInvalidTransportMode`    | `transport_mode`가 `direct`/`port` 이외의 값 |
| `ErrBrokerRequired`          | MQTT 브로커 주소 미설정 (`direct` 모드)   |
| `ErrInvalidTopicTemplate`    | 토픽 템플릿에 `{device_id}` placeholder 없음 |
| `ErrInvalidPayloadMapping`   | 페이로드 필드 매핑 누락/오류               |
| `ErrDeviceNotFound`          | 미등록 device_id                   |
| `ErrDeviceAlreadyRegistered` | 이미 등록된 device_id                |
| `ErrConfigDeviceProtected`   | 설정 기반 디바이스는 제거 불가              |
| `ErrInvalidFanSpeed`         | 유효하지 않은 풍량 값 (1/2/3 이외)         |
| `ErrPowerOff`                | 전원 OFF 상태에서 풍량 명령 (기본 정책)       |
| `ErrEmptyGroup`              | 멤버가 없는 group_id/station/line 대상 명령 |
| `ErrControlTimeout`          | 응답 대기 타임아웃 — `control_response_timeout` 내 상태 에코 미도착 (REQ-XSFM-001-03-10) |
| `ErrStationNotFound`         | 역사 레지스트리에 없는 station 조회/해석 (REQ-XSFM-001-02-13) |
| `ErrLineNotFound`            | 역사 레지스트리에 매핑된 station이 없는 line 대상 |
| `ErrNotConnected`            | MQTT 미연결 상태                     |
| `ErrInvalidCommand`          | 유효하지 않은 명령 형식                   |

---

## 4. Specifications (사양 요약)

- **아키텍처 3계층**: (1) 에이전트 `internal/agent/xsfm/` (순수 프로토콜/로직 + 로스터 + 제어/상태), (2) 플로우 노드 `internal/node/xsfm_{status,control}.go` (상태 입력 포트 · 제어 출력 포트), (3) 디바이스 어댑터 `internal/device/adapter/xsfm.go` (CommandSpec + Executor).
- **듀얼 트랜스포트 모드**: `transport_mode`(`direct` | `port`, 기본 `direct`). `direct`=에이전트가 브로커 sub/pub 소유(`thingplus_agent.go` Paho 패턴 — 브로커/TLS/LWT/재연결/구독복원), `port`=외부 mqtt-in/out 노드가 브로커 I/O 담당(상태 입력 포트 · 제어 출력 포트 분리).
- **I/O 경계 vs 공유 로직 분리**: 상태 입력 경계·명령 출력 경계만 모드별로 다르고, 페이로드 매핑/로스터/제어 구성/그룹 fan-out/2-축 제어/관측 emit은 두 모드 동일. 에이전트는 순수 프로토콜/로직 레이어.
- **설정 시임**: 토픽 템플릿(direct 전용) + 페이로드 매핑(양 모드 공통) — 디바이스 매뉴얼 확보 후 확정, 하드코딩 금지.
- **디바이스 위치 계층 (v0.3.0)**: `station`/`place`/`index` 선택 속성(하위호환). line은 디바이스 속성이 아니라 station 상위 집계 레벨.
- **역사 레지스트리 (v0.3.0, SSOT)**: station→{line, 표시명, 정렬} 매핑을 device_metadata 패턴 파일 저장소로 관리 + CRUD/lookup(`ResolveLine`/`StationsByLine`/`DevicesByStation`). 디바이스는 station만 보유, line은 조회로 해석.
- **상태 모델**: 2-축 `{power, fan_speed(1/2/3), online}`, 풍량은 power ON에서만 유효.
- **제어 응답 대기 (v0.3.0)**: `control_response_timeout` + device_id(+명령) 키잉 pending-command 레지스트리. 성공 = 타임아웃 내 상태 에코가 명령 반영(상관 by device_id), 실패 = `ErrControlTimeout`. 늦은 에코 무시, 동시 명령 독립 판정. 개별(M3)·셀렉터(M4) 공통.
- **그룹/셀렉터 제어**: 에이전트 측 per-device fan-out (NET-NEW). 셀렉터 `group_id`/`station`/`line`(station registry 경유). 멤버별 응답 대기 + `ok`/`error`/`timeout` 집계, best-effort.
- **로깅**: 상태 시계열(status 노드 → influxdb_write, 직접 기록 금지) + 제어 감사(remote_audit_repository, 셀렉터·멤버별 응답 결과 포함).

## 5. Traceability (추적성)

- 상세 구현 계획: `plan.md`
- 인수 기준 (Gherkin): `acceptance.md`
- 선행 SPEC: SPEC-SAMSUNG-HVACR-001(에이전트+노드+제어 형태), SPEC-LG-HVACR-002-002/003(제어 인코더/제어 노드 분리), SPEC-MODBUS-005(다중 디바이스 로스터 + Web UI), SPEC-DEVICE-IDENTITY-001 + SPEC-AGENT-STORE-001(로스터 영속화), SPEC-HVACR-CONNSTATE-001(offline/stale), SPEC-INFLUX-002(텔레메트리), thingplus 게이트웨이 에이전트(MQTT 트랜스포트).
- 후속 소비 SPEC: **SPEC-FACILITY-DASHBOARD-001**(대시보드 라인/역사 패널 · 일괄제어 UI) — 본 SPEC이 소유하는 디바이스 위치 계층(station/place/index) + 역사 레지스트리(station→line) + line/station fan-out 표면을 소비한다. 시각화/UI는 그 SPEC의 범위.
- 참조 코드(v0.3.0 확장 기반): `internal/device/device.go`(DeviceMetadata.Location — 위치 계층으로 확장), `internal/storage/device_metadata.go`(DeviceMetadataFileRepository — 역사 레지스트리 저장소 패턴 참조).
