---
id: SPEC-THINGPLUS-001
version: "1.1.0"
status: completed
created: "2026-07-08"
updated: "2026-07-08"
author: xtra
priority: high
lifecycle_level: spec-first
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-07-08 | 1.0.0 | 초기 SPEC 작성 |
| 2026-07-08 | 1.1.0 | 구현 완료 (M1~M5), Implementation Notes 추가 |

---

# SPEC-THINGPLUS-001: Thingplus Gateway Agent (ThingsBoard Gateway MQTT) - 양방향 IoT 디바이스 연동 에이전트

## 1. Environment (환경)

### 1.1 시스템 개요

#### 현재 상태

xflow는 `internal/agent/system/mqtt_agent.go`를 통해 일반 MQTT 클라이언트 에이전트(`mqtt-client`)를 제공한다. 이 에이전트는 단일 브로커 연결에서 고정된 토픽 목록을 구독/발행하며, 개별 물리 디바이스 개념 없이 원시 MQTT 페이로드를 Bridge 노드를 통해 플로우로 전달한다.

그러나 Thingplus(ThingsBoard 기반 배포)와 같은 IoT 플랫폼은 **게이트웨이(gateway) 패턴**을 사용한다. 하나의 게이트웨이 디바이스가 여러 하위 디바이스(sub-device)를 대표하여, 단일 MQTT 연결로 다수 디바이스의 텔레메트리/속성을 대리 전송하고, 플랫폼으로부터 디바이스별 RPC 명령과 공유 속성(shared attribute) 변경을 수신한다.

#### 문제

- 일반 `mqtt-client` 에이전트는 ThingsBoard Gateway MQTT API(`v1/gateway/*`)의 디바이스 다중화(multiplexing) 규약을 모른다.
- 인입 메시지에서 디바이스를 식별하여 자동으로 게이트웨이에 연결(auto-connect)하고, 디바이스별 텔레메트리 프레임(`{device:[{ts,values}]}`)을 조립하는 계층이 없다.
- 플랫폼이 내려주는 RPC/공유 속성 다운링크를 디바이스 NAME 기준으로 파싱하여 플로우 메시지로 방출하고, RPC 응답을 다시 발행하는 왕복 흐름이 없다.
- xflow 내부 식별자(`device_id`)와 ThingsBoard 디바이스 NAME 간 매핑 계층이 없다.

본 SPEC은 이 문제를 해결하기 위해 `thingplus-gateway` 타입의 신규 시스템 에이전트를 정의한다. 이 에이전트는 단일 MQTT 연결을 다수 디바이스에 프록시하고, 업링크(텔레메트리/속성)와 다운링크(RPC/공유 속성)를 양방향으로 중계하며, NAME↔device_id 매핑을 관리한다.

### 1.2 기술 환경

- **언어**: Go 1.25+
- **MQTT 라이브러리**: Eclipse Paho MQTT Go (`github.com/eclipse/paho.mqtt.golang`, 기존 의존성 재사용)
- **패키지 경로**: `internal/agent/system/` (기존 `mqtt_agent.go`와 나란히 신규 파일 추가)
- **Tier**: Tier 2 - 내부 실행 계층 (internal), 시스템 에이전트
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-00x): `Agent`, `BaseAgent`, 선택적 인터페이스(`MessageReceiver`, `SubscriberAgent`, `StatefulAgent`, `MessagePublisher`, `TransportChecker`, `BufferInfoProvider`, `ConnectionStatsProvider`), `type_registry.go`
  - `internal/agent/system/mqtt_agent.go` (SPEC-MQTT-001/002/003): MQTT 연결/구독/발행 패턴 및 통계 재사용
  - `internal/agent/device_id_repo.go` (SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001): `ResolveDeviceID(ctx, agentName, unitID)`
  - `internal/node/bridge.go` (SPEC-BRIDGE-001): 플로우 연동(In/Out/InOut/RequestReply) 및 어댑터
  - `pkg/message/` (SPEC-MSG-001, SPEC-MESSAGE-TYPE-001): `Message`, `Payload.GetPath`(JSONPath), `Metadata`, dot-notation `Type()`
  - `github.com/eclipse/paho.mqtt.golang`: MQTT v3.1.1 클라이언트
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`
- **테스트 전략**: table-driven 단위 테스트 + channel-mocked broker 기반 통합 테스트

### 1.3 설계 원칙

- **MQTT 에이전트 패턴 재사용**: `mqtt_agent.go`의 연결(connect), 구독(subscribe), 발행(PublishMessage), 수신 채널(recvCh), 상태(State()) 패턴을 최대한 재사용하여 신규 로직 표면을 최소화한다.
- **단일 연결 다중 프록시**: 하나의 게이트웨이 MQTT 연결이 다수의 논리 디바이스를 프록시한다. 디바이스마다 별도 연결을 만들지 않는다.
- **NAME↔device_id 매핑 계층**: ThingsBoard 디바이스 NAME과 xflow 내부 `device_id`를 양방향으로 매핑하는 계층을 분리하여, 업링크/다운링크 양쪽에서 일관되게 사용한다.
- **점진적 열화(graceful degradation)**: `device_id_repo`가 없거나 매핑이 실패해도 NAME을 그대로 키로 사용하여 동작을 지속한다. 연결 끊김 시 버퍼링으로 무손실을 지향한다.
- **비밀정보 로그 금지**: access token 등 민감 정보를 평문 로그로 남기지 않는다.

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:

- `thingplus-gateway` 타입 정의 및 `type_registry` 등록(`RegisterThingplusTypes`)
- ThingsBoard Gateway MQTT API 지원: `v1/gateway/*` 토픽, access token 인증(MQTT username), 포트 1883/8883, 선택적 TLS
- **업링크**: 인입 플로우 메시지에서 디바이스 추출 → 미등록 디바이스 자동 connect → 텔레메트리(`v1/gateway/telemetry`) 및 클라이언트 속성(`v1/gateway/attributes`) 발행
- **다운링크**: RPC(`v1/gateway/rpc`) 및 공유 속성(`v1/gateway/attributes`) 구독 → 플로우 메시지로 방출, RPC 응답 발행
- **NAME↔device_id 매핑**: JSONPath 기반 NAME 추출(기본 `$.device`, 설정 가능), epoch-ms 타임스탬프 변환
- 웹 설정 스키마(`agentSchemas.ts`), `State()`/관찰성(observability)
- 단위 + 통합 테스트, 예제 agent/flow YAML

**OUT OF SCOPE (별도 SPEC 또는 제외)**:

- Daliworks Thing+ 고유 프로토콜(`v/a/g/*`) — 본 SPEC은 ThingsBoard 호환 게이트웨이 API만 대상으로 한다("Thingplus" = ThingsBoard 기반 배포를 의미).
- 플랫폼 측 "Is gateway" 플래그 설정 및 프로비저닝 UI (ThingsBoard 서버 측 책임)
- Bridge 코어 변경 (어댑터 추가만 수행, 코어는 변경하지 않음)
- 속성 영속화(attribute persistence) 및 RPC 비즈니스 로직 (플로우에 위임)
- MQTT v5 전용 고급 기능 (목표는 v3.1.1이며, v5 PUBACK 순서만 리스크로 고려)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MQTT-001 | 의존/재사용 | Paho 구독 관리 패턴, SubscriberAgent 패턴 |
| SPEC-MQTT-002 | 의존/재사용 | Paho connect 및 재연결 패턴 |
| SPEC-MQTT-003 | 의존/재사용 | Paho publish 및 발행 통계 패턴 |
| SPEC-BRIDGE-001 | 의존 | 플로우 연동(In/Out/InOut/RequestReply), Bridge 어댑터 |
| SPEC-DEVICE-001 | 의존 | device 모델 및 device_id 개념 |
| SPEC-DEVICE-IDENTITY-001 | 의존 | `ResolveDeviceID(ctx, agentName, unitID)` |
| SPEC-AGENT-00x | 의존 | Agent/BaseAgent 인터페이스 및 타입 등록 |
| SPEC-INVENTORY-001 | 연관 | 디바이스 인벤토리 |
| SPEC-MESSAGE-TYPE-001 | 의존 | `Type()` dot-notation 네이밍 규약 |

---

## 2. Assumptions (가정)

| ID | 가정 | 위험도 |
|----|------|--------|
| A1 | SPEC-AGENT-00x의 `Agent`/`BaseAgent` 인터페이스와 `type_registry.go`의 `RegisterType(typeName, factory)` 패턴이 구현되어 있다 | 낮음 |
| A2 | `mqtt_agent.go`의 MQTT 연결/구독/발행/수신 패턴(MQTTConfig, connect, subscribe, PublishMessage, recvCh, State)이 재사용 가능하다 | 낮음 |
| A3 | Eclipse Paho MQTT Go의 `Subscribe`/`Publish`/`Unsubscribe`는 동시 호출에 안전하다 | 낮음 |
| A4 | `pkg/message`의 `Payload.GetPath`(JSONPath)와 `Metadata`가 인입 메시지에서 디바이스 NAME 추출에 사용 가능하다 | 낮음 |
| A5 | `device_id_repo.go`의 `ResolveDeviceID(ctx, agentName, unitID)`가 NAME 또는 unitID를 xflow `device_id`로 해석한다. repo가 nil이면 NAME을 키로 사용한다 | 중간 |
| A6 | ThingsBoard Gateway는 `v1/gateway/connect`로 미지의 디바이스를 자동 생성(auto-provision)하며, connect 이후에만 해당 디바이스의 RPC를 전달한다 | 중간 |
| A7 | RPC 발행 전 디바이스 connect의 PUBACK 수신이 완료되어야 한다. 특히 MQTT v5에서 PUBACK 순서가 보장되지 않으면 RPC가 유실될 수 있다 | **높음** |
| A8 | `v1/gateway/attributes/response`의 정확한 다중 키(client/shared) 인코딩은 라이브 브로커 검증이 필요하다 (예상 구조: `{"id":1,"device":"...","value":...}`) | **높음** |
| A9 | 텔레메트리 타임스탬프는 epoch milliseconds(int64)이며, `time.Time.UnixMilli()`로 변환한다(프로젝트 타임스탬프 규약 일치). ts 생략 시 서버 시각이 사용된다 | 낮음 |
| A10 | 게이트웨이 access token은 MQTT username으로 전달되며 password는 비어 있다. 포트는 1883(평문) 또는 8883(TLS)이다 | 낮음 |

---

## 3. Requirements (요구사항)

### Module 1: Gateway Core & Connection/Auth - 게이트웨이 코어 및 연결/인증 (P0)

#### REQ-THINGPLUS-001-core-register (Ubiquitous) 타입 등록

시스템은 **항상** `internal/agent/system/`에 `RegisterThingplusTypes(mgr *agent.DefaultManager) error` 함수를 제공하고, 이를 통해 `"thingplus-gateway"` 타입을 팩토리와 함께 `type_registry`에 등록해야 한다. 웹 표시 레이블은 "Thingplus Gateway"이다.

#### REQ-THINGPLUS-001-core-config (Event-Driven) 설정 파싱

**WHEN** 에이전트가 `Init(config)` 또는 `Configure(config)`로 설정을 수신하면, **THEN** 브로커 주소, 포트(기본 1883, TLS 시 8883), TLS 사용 여부 및 CA, access token, `device_name_path`(기본 `$.device`), QoS, keepalive, 자동 재연결, 버퍼 크기를 파싱하여 내부 설정으로 보관해야 한다.

#### REQ-THINGPLUS-001-core-auth (Event-Driven) 게이트웨이 인증 연결

**WHEN** 에이전트가 `Start(ctx)`로 브로커에 연결을 시도하면, **THEN** access token을 MQTT username으로, 빈 문자열을 password로 설정하여 연결해야 하며, TLS가 활성화된 경우 8883 포트로 CA 인증서를 사용하여 TLS 핸드셰이크를 수행해야 한다.

#### REQ-THINGPLUS-001-core-reconnect (State-Driven) 자동 재연결 및 재구독

**IF** 자동 재연결이 활성화되어 있고 브로커 연결이 끊긴 상태이면, **THEN** 에이전트는 재연결을 시도하고, 재연결 성공 시 게이트웨이 다운링크 토픽(`v1/gateway/rpc`, `v1/gateway/attributes`)을 재구독하며, 이전에 연결되어 있던 알려진 디바이스들을 다시 connect해야 한다.

#### REQ-THINGPLUS-001-core-state (Ubiquitous) 상태 노출

시스템은 **항상** `State()`를 통해 브로커 연결 상태, 연결된 디바이스 수, 업링크/다운링크 통계를 포함하는 상태 스냅샷을 노출해야 한다.

#### REQ-THINGPLUS-001-core-nocred-log (Unwanted) 비밀정보 로그 금지

시스템은 access token 등 민감한 자격 증명을 평문으로 로그에 출력**하지 않아야 한다**. 로그가 필요한 경우 마스킹된 형태로만 기록한다.

---

### Module 2: Device Connect/Disconnect & NAME↔ID Mapping & Auto-provision - 디바이스 연결/해제 및 매핑 (P0)

#### REQ-THINGPLUS-001-map-resolve (Event-Driven) 디바이스 NAME 추출

**WHEN** 인입 플로우 메시지를 처리하면, **THEN** 설정된 `device_name_path` JSONPath(기본 `$.device`, `$.metadata.device_id` 등도 설정 가능)를 사용하여 메시지에서 디바이스 NAME을 추출해야 하며, 추출된 NAME을 `ResolveDeviceID`를 통해 xflow `device_id`와 상호 연결해야 한다.

#### REQ-THINGPLUS-001-map-connect (State-Driven) 미등록 디바이스 자동 connect

**IF** 추출된 디바이스 NAME이 아직 게이트웨이에 연결되지 않은 상태이면, **THEN** 에이전트는 텔레메트리/속성 발행 이전에 `v1/gateway/connect` 페이로드 `{"device":"<NAME>","type":"<optional>"}`를 발행하여 해당 디바이스를 게이트웨이에 연결(및 서버 측 자동 생성)해야 한다.

#### REQ-THINGPLUS-001-map-puback (State-Driven) connect PUBACK 게이팅

**IF** 디바이스가 `connecting` 상태(connect 발행 후 PUBACK 미수신)이면, **THEN** 에이전트는 해당 디바이스에 대한 RPC 응답 및 이후 처리를 PUBACK 수신(상태 `connected` 전이)까지 게이팅해야 한다.

#### REQ-THINGPLUS-001-map-reconnect (State-Driven) 재연결 시 알려진 디바이스 재connect

**IF** 브로커 재연결이 발생하면, **THEN** 에이전트는 이전에 `connected` 상태였던 모든 알려진 디바이스에 대해 `v1/gateway/connect`를 다시 발행하여 상태를 복구해야 한다.

#### REQ-THINGPLUS-001-map-disconnect (Event-Driven) 디바이스 disconnect

**WHEN** 디바이스 연결 종료가 요청되면(예: 명시적 제어 또는 정리), **THEN** 에이전트는 `v1/gateway/disconnect` 페이로드 `{"device":"<NAME>"}`를 발행하고 해당 디바이스를 연결 상태 맵에서 제거해야 한다.

#### REQ-THINGPLUS-001-map-fallback (Optional) 매핑 fallback

**가능하면** `device_id_repo`가 nil이거나 `ResolveDeviceID`가 매핑을 찾지 못하는 경우, 추출된 디바이스 NAME 자체를 키(device_id 대체)로 사용하여 동작을 지속해야 한다.

---

### Module 3: Telemetry/Attributes Uplink - 텔레메트리/속성 업링크 (P0)

#### REQ-THINGPLUS-001-up-telemetry (Event-Driven) 텔레메트리 발행

**WHEN** 디바이스가 `connected` 상태이고 인입 메시지에 텔레메트리 값이 포함되어 있으면, **THEN** 에이전트는 `v1/gateway/telemetry` 페이로드 `{"<NAME>":[{"ts":<epoch_ms>,"values":{...}}]}` 형식으로 텔레메트리를 발행해야 한다.

#### REQ-THINGPLUS-001-up-ts (Ubiquitous) 타임스탬프 규약

시스템은 **항상** 텔레메트리 `ts` 필드를 epoch milliseconds(int64)로 인코딩해야 하며, 발행 레이어에서 `time.Time.UnixMilli()`로 변환해야 한다. 타임스탬프가 없는 경우 `ts` 필드를 생략하여 서버 시각을 사용하도록 한다.

#### REQ-THINGPLUS-001-up-attributes (Optional) 클라이언트 속성 발행

**가능하면** 인입 메시지에 디바이스 클라이언트 속성이 포함된 경우, 에이전트는 `v1/gateway/attributes` 페이로드 `{"<NAME>":{"<k>":<v>}}` 형식으로 클라이언트 속성을 발행해야 한다.

#### REQ-THINGPLUS-001-up-batch (Optional) 배치 발행

**가능하면** 동일 디바이스의 다수 텔레메트리 항목을 하나의 `v1/gateway/telemetry` 페이로드 배열로 배치 조립하여 발행해야 한다.

#### REQ-THINGPLUS-001-up-nolost (Unwanted) 무손실 버퍼링

시스템은 브로커 연결이 끊긴 동안 업링크 메시지를 조용히 폐기(silent drop)**하지 않아야 한다**. 경계가 있는(bounded) 버퍼에 저장하고, 버퍼 초과 시 관찰 가능한 방식(에러/메트릭)으로 처리해야 한다.

---

### Module 4: RPC/Shared-attr Downlink → Flow - RPC/공유 속성 다운링크 (P0)

#### REQ-THINGPLUS-001-dn-rpc-recv (Event-Driven) RPC 수신 및 방출

**WHEN** `v1/gateway/rpc` 구독으로 `{"device":"<NAME>","data":{"id":<n>,"method":"<m>","params":{...}}}` 형식의 RPC 요청이 수신되면, **THEN** 에이전트는 NAME을 device_id로 해석하고 `Type()`이 `thingplus.rpc.request`인 플로우 메시지를 방출해야 한다.

#### REQ-THINGPLUS-001-dn-rpc-reply (Event-Driven) RPC 응답 발행

**WHEN** 플로우로부터 RPC 응답 메시지가 에이전트로 전달되면, **THEN** 에이전트는 원 요청의 device NAME과 RPC id를 사용하여 `v1/gateway/rpc` 페이로드 `{"device":"<NAME>","id":<n>,"data":{...}}` 형식으로 응답을 발행해야 한다.

#### REQ-THINGPLUS-001-dn-shared (Event-Driven) 공유 속성 push 방출

**WHEN** `v1/gateway/attributes` 구독으로 `{"device":"<NAME>","data":{"<k>":<v>}}` 형식의 공유 속성 변경이 수신되면, **THEN** 에이전트는 `Type()`이 `thingplus.attr.update`인 플로우 메시지를 방출해야 한다.

#### REQ-THINGPLUS-001-dn-attr-req (Optional) 속성 요청/응답

**가능하면** 에이전트는 `v1/gateway/attributes/request` 페이로드 `{"id":<n>,"device":"<NAME>","client":[...],"shared":[...]}`를 발행하고 `v1/gateway/attributes/response` 응답을 id로 상관(correlate)하여 처리해야 한다. (정확한 응답 인코딩은 라이브 브로커 검증 후 확정한다 — A8.)

#### REQ-THINGPLUS-001-dn-name-resolve (State-Driven) NAME→device_id 역매핑

**IF** 다운링크 메시지가 device NAME을 포함하면, **THEN** 에이전트는 역매핑(NAME→device_id)을 수행하여 방출되는 플로우 메시지 및 응답 발행에서 일관된 식별자를 사용해야 한다.

---

### Module 5: Config Schema, Web & Observability - 설정 스키마, 웹 및 관찰성 (P1)

#### REQ-THINGPLUS-001-web-schema (Ubiquitous) 웹 설정 스키마

시스템은 **항상** `web/src/config/agentSchemas.ts`에 `thingplus-gateway` 타입의 설정 필드를 제공해야 한다: broker, port, tls(+ca), access_token(sensitive), device_name_path(기본 `"$.device"`), qos, keepalive, auto_reconnect, buffer_size.

#### REQ-THINGPLUS-001-obs-connstats (Optional) 연결 통계

**가능하면** 에이전트는 `ConnectionStatsProvider` 인터페이스를 구현하여 연결/재연결 통계를 노출해야 한다.

#### REQ-THINGPLUS-001-obs-buffer (Optional) 버퍼 정보

**가능하면** 에이전트는 `BufferInfoProvider` 인터페이스를 구현하여 업링크 버퍼 사용량을 노출해야 한다.

#### REQ-THINGPLUS-001-obs-example (Ubiquitous) 예제 YAML

시스템은 **항상** `thingplus-gateway` 에이전트 및 이를 사용하는 플로우의 예제 YAML을 제공해야 한다.

---

## 4. Detailed Design (상세 설계)

### 4.1 파일 구조 (신규/변경 대상)

```
internal/agent/system/
├── thingplus_agent.go          // [신규] ThingplusGatewayAgent 구조체, connect/subscribe/publish
├── thingplus_mapping.go        // [신규] NAME↔device_id 양방향 매핑, JSONPath 추출
├── thingplus_codec.go          // [신규] 텔레메트리/속성/RPC 페이로드 build/parse
├── thingplus_register.go       // [신규] RegisterThingplusTypes ("thingplus-gateway")
├── thingplus_agent_test.go     // [신규] 통합 테스트 (channel-mocked broker)
├── thingplus_mapping_test.go   // [신규] 매핑 단위 테스트
└── thingplus_codec_test.go     // [신규] 코덱 단위 테스트 (ts=epoch ms 포함)

web/src/config/
└── agentSchemas.ts             // [수정] thingplus-gateway 필드 추가 (MQTT_FIELDS ~line 29 참고)

examples/
├── agents/thingplus-gateway.yaml  // [신규] 예제 에이전트
└── flows/thingplus-gateway.yaml   // [신규] 예제 플로우
```

### 4.2 에이전트 구조체 및 임베딩

`ThingplusGatewayAgent`는 `mqtt_agent.go`의 `MQTTAgent`(MQTTConfig:20, connect ~194, subscribe ~285, PublishMessage ~523, recvCh ~389, State() ~737)를 참조 모델로 하여 `agent.BaseAgent`(agent.go:130)와 Paho 클라이언트를 임베딩/보유한다.

```go
package system

import (
    "context"
    "sync"

    mqtt "github.com/eclipse/paho.mqtt.golang"
    "github.com/xtra/xflow/internal/agent"
)

// ThingplusConfig 는 thingplus-gateway 에이전트 설정이다.
type ThingplusConfig struct {
    Broker         string `json:"broker"`
    Port           int    `json:"port"`             // 기본 1883, TLS 시 8883
    TLS            bool   `json:"tls"`
    CACert         string `json:"ca_cert"`          // TLS CA 경로/PEM
    AccessToken    string `json:"access_token"`     // MQTT username (sensitive)
    DeviceNamePath string `json:"device_name_path"` // JSONPath, 기본 "$.device"
    QoS            byte   `json:"qos"`
    KeepAliveSec   int    `json:"keep_alive_sec"`
    AutoReconnect  bool   `json:"auto_reconnect"`
    BufferSize     int    `json:"buffer_size"`
}

// ThingplusGatewayAgent 는 ThingsBoard Gateway MQTT API를 프록시하는 시스템 에이전트이다.
type ThingplusGatewayAgent struct {
    *agent.BaseAgent
    cfg     ThingplusConfig
    client  mqtt.Client
    devices *deviceStateMap  // NAME -> 디바이스 상태 (disconnected/connecting/connected)
    mapping *nameIDMap       // NAME <-> device_id 양방향 매핑
    recvCh  chan []byte      // 다운링크 수신 버퍼 (mqtt_agent recvCh 패턴)
    upBuf   *boundedBuffer   // 업링크 무손실 버퍼
    mu      sync.RWMutex
}

// 선택적 인터페이스 컴파일 타임 체크
var (
    _ agent.Agent                   = (*ThingplusGatewayAgent)(nil)
    _ agent.MessageReceiver         = (*ThingplusGatewayAgent)(nil)
    _ agent.MessagePublisher        = (*ThingplusGatewayAgent)(nil)
    _ agent.StatefulAgent           = (*ThingplusGatewayAgent)(nil)
    _ agent.ConnectionStatsProvider = (*ThingplusGatewayAgent)(nil)
    _ agent.BufferInfoProvider      = (*ThingplusGatewayAgent)(nil)
)
```

옵션 인터페이스 참조: `MessageReceiver`(agent.go:25), `SubscriberAgent`(:29), `StatefulAgent`(:37), `MessagePublisher`(:62), `TransportChecker`(:44), `BufferInfoProvider`(:69), `ConnectionStatsProvider`(:94).

### 4.3 타입 등록

`mqtt_register.go:8`의 `mgr.RegisterType("mqtt-client", factory)` 패턴을 따른다.

```go
// thingplus_register.go
func RegisterThingplusTypes(mgr *agent.DefaultManager) error {
    return mgr.RegisterType("thingplus-gateway", func(config agent.AgentConfig) (agent.Agent, error) {
        return NewThingplusGatewayAgent(config)
    })
}
```

### 4.4 설정 파싱 및 연결/인증

- `Init`/`Configure`에서 `ThingplusConfig`로 언마샬. `Port` 기본 1883, `TLS` 활성 시 기본 8883 검증.
- 연결 시 Paho `ClientOptions.SetUsername(cfg.AccessToken)`, `SetPassword("")` 설정. TLS 시 `SetTLSConfig`(CA 로드).
- `AutoReconnect`에 따라 Paho 자동 재연결 옵션 및 `OnConnect` 핸들러에서 다운링크 구독 + 알려진 디바이스 재connect 수행.

### 4.5 디바이스 상태 머신

```
disconnected --(connect 발행)--> connecting --(PUBACK 수신)--> connected
connected --(disconnect 발행)--> disconnected
```

- `connecting` 상태에서는 해당 디바이스 RPC 처리를 게이팅한다(A7). MQTT v5 환경에서 PUBACK 순서 불확실성을 고려하여, connect 토큰 완료(`token.Wait()` 또는 v5 PUBACK)를 상태 전이 조건으로 사용한다.
- 재연결 시 `connected`였던 디바이스를 순회하여 다시 connect 발행한다.

### 4.6 NAME↔device_id 양방향 매핑

- **build/lookup**: `nameIDMap`은 `map[string]string`(NAME→device_id)과 역방향 `map[string]string`(device_id→NAME)을 함께 유지하고 `sync.RWMutex`로 보호한다.
- **JSONPath 추출**: 인입 메시지에서 `Payload.GetPath(cfg.DeviceNamePath)`(payload.go:19, 기본 `$.device`)로 NAME을 얻는다. `$.metadata.device_id` 등 대안 경로도 설정으로 허용한다.
- **ResolveDeviceID 통합**: `device_id_repo.ResolveDeviceID(ctx, agentName, unitID)`로 device_id를 해석하여 매핑을 채운다.
- **repo-nil fallback**: repo가 nil이거나 해석 실패 시 NAME 자체를 device_id로 사용한다(REQ-map-fallback).

### 4.7 텔레메트리 배치 빌더

- `thingplus_codec.go`에서 `{ "<NAME>": [ {"ts": <UnixMilli>, "values": {...}} ] }` 형식을 조립한다.
- `ts`는 `time.Time.UnixMilli()`(int64). 타임스탬프 부재 시 필드 생략.
- 동일 디바이스의 다수 항목은 배열로 배치(REQ-up-batch).

### 4.8 다운링크 파싱 → 메시지 방출

- `OnConnect`에서 `v1/gateway/rpc`, `v1/gateway/attributes` 구독(subscribe ~285 패턴).
- RPC 수신 `{"device","data":{"id","method","params"}}` → device_id 역해석 후 `Type() = "thingplus.rpc.request"` 메시지 방출(recvCh ~389 경유 Bridge 어댑터로 전달).
- 공유 속성 수신 `{"device","data":{...}}` → `Type() = "thingplus.attr.update"` 메시지 방출.
- `Type()`는 dot-notation 규약(SPEC-MESSAGE-TYPE-001)을 따른다.

### 4.9 RPC 응답 발행

- 플로우 응답 메시지의 device NAME과 RPC id를 사용하여 `v1/gateway/rpc` `{"device":"<NAME>","id":<n>,"data":{...}}`를 `PublishMessage`(~523 패턴)로 발행.
- 응답 발행은 디바이스가 `connected`일 때만 수행(A6/A7).

### 4.10 버퍼링/백프레셔

- `mqtt_agent.go`의 `bufferSize`/`recvCh` 패턴을 재사용한다.
- 업링크는 `boundedBuffer`(BufferSize)로 관리하여 연결 끊김 시 무손실 지향(REQ-up-nolost). 초과 시 메트릭/에러로 관찰 가능하게 처리.

### 4.11 TLS/토큰 처리 및 State()

- TLS: `ca_cert` PEM 로드하여 `RootCAs` 설정, 8883 연결.
- 토큰: username으로만 사용, 로그 마스킹(REQ-core-nocred-log).
- `State()`(~737 패턴): `{connected: bool, deviceCount: int, uplink/downlink counters, bufferUsage}` 스냅샷.

---

## 5. Test Plan (테스트 계획)

### 5.1 단위 테스트

- **설정 파싱**: broker/port/tls/token/device_name_path/qos/keepalive/auto_reconnect/buffer_size 파싱 및 기본값(port=1883, path=`$.device`) 검증.
- **페이로드 build/parse**:
  - 텔레메트리 빌더 `{NAME:[{ts,values}]}` 조립, `ts == UnixMilli` 검증, ts 생략 케이스.
  - 속성 빌더 `{NAME:{k:v}}`.
  - RPC 수신 파싱 `{device,data:{id,method,params}}`, RPC 응답 빌더 `{device,id,data}`.
  - 공유 속성 파싱 `{device,data}`.
- **NAME↔id 매핑**: 정상 build/lookup, nil-repo fallback(NAME=device_id), 중복 NAME 처리, 역매핑(device_id→NAME).

### 5.2 통합 테스트 (channel-mocked broker)

- **connect→PUBACK→telemetry**: 미등록 디바이스 인입 → connect 발행 → PUBACK 모킹 → `connected` 전이 → 텔레메트리 발행 검증.
- **rpc SUB→emit→reply PUB**: 모킹 브로커가 `v1/gateway/rpc` RPC 주입 → `thingplus.rpc.request` 방출 검증 → 플로우 응답 주입 → `v1/gateway/rpc` 응답 발행 검증.
- **reconnect re-connect**: 연결 끊김→재연결 시 이전 `connected` 디바이스들이 다시 connect 발행되는지 검증.

---

## 6. Implementation Plan (구현 계획)

마일스톤은 다음 순서로 진행한다(시간 추정 없음, 의존 순서 기반):

- **Primary Goal (M1)**: 코어/연결 — 타입 등록, 설정 파싱, access token 인증 연결, TLS, State(), 재연결 골격.
- **Secondary Goal (M2)**: 매핑/connect — NAME↔device_id 매핑, JSONPath 추출, 디바이스 상태 머신(connect/PUBACK/disconnect), auto-provision, 재연결 재connect.
- **Tertiary Goal (M3)**: 업링크 — 텔레메트리(ts=UnixMilli)/속성 발행, 배치, 무손실 버퍼링.
- **Quaternary Goal (M4)**: 다운링크 — RPC/공유 속성 구독 → 메시지 방출(`thingplus.rpc.request`/`thingplus.attr.update`), RPC 응답 발행, (선택) 속성 요청/응답.
- **Final Goal (M5)**: 스키마/관찰성 — 웹 설정 스키마, ConnectionStatsProvider/BufferInfoProvider, 예제 agent/flow YAML.

**주의**: M4 파서를 확정하기 전에 라이브 브로커 스모크 테스트를 수행하여 `v1/gateway/attributes/response`의 다중 키 인코딩(A8)과 MQTT v5 PUBACK 타이밍(A7)을 확인한다.

---

## 7. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-THINGPLUS-001-core-* | M1 Gateway Core & Connection/Auth | thingplus_agent.go, thingplus_register.go | P0 |
| REQ-THINGPLUS-001-map-* | M2 Device Connect & Mapping | thingplus_mapping.go, thingplus_agent.go | P0 |
| REQ-THINGPLUS-001-up-* | M3 Telemetry/Attributes Uplink | thingplus_codec.go, thingplus_agent.go | P0 |
| REQ-THINGPLUS-001-dn-* | M4 RPC/Shared-attr Downlink | thingplus_codec.go, thingplus_agent.go | P0 |
| REQ-THINGPLUS-001-web-*, obs-* | M5 Config Schema & Observability | agentSchemas.ts, thingplus_agent.go, examples/ | P1 |

---

## Implementation Notes (구현 완료)

**상태**: `completed` · **구현 커밋**: `eda584a` (2026-07-08)

M1~M5 마일스톤이 모두 구현되어 `thingplus-gateway` 시스템 에이전트가 ThingsBoard Gateway MQTT API(`v1/gateway/*`)를 양방향으로 프록시한다. 단일 MQTT 연결로 다수 하위 디바이스를 다중화하며, 업링크(텔레메트리/속성)와 다운링크(RPC/공유 속성)를 중계하고 NAME↔device_id 매핑을 관리한다.

### 구현 범위 (마일스톤별)

- **M1 (코어/연결)**: `"thingplus-gateway"` 타입 등록, 설정 파싱, access token 기반 MQTT username 인증, TLS(8883/CA), `State()`, 재연결 골격.
- **M2 (매핑/connect)**: NAME↔device_id 양방향 매핑, JSONPath 기반 디바이스 NAME 추출, 디바이스 상태 머신(`disconnected→connecting→connected`), 자동 connect/auto-provision, 재연결 시 알려진 디바이스 재connect, repo-nil fallback(NAME을 device_id로 사용).
- **M3 (업링크)**: 텔레메트리(`ts=UnixMilli`, 부재 시 생략) 및 클라이언트 속성 발행, 배치 조립, 경계가 있는 무손실 업링크 버퍼(연결 끊김 시 버퍼링, 재연결 시 flush, 초과 시 관찰 가능).
- **M4 (다운링크)**: `v1/gateway/rpc` 및 `v1/gateway/attributes` 구독 → `Type()`이 `thingplus.rpc.request` / `thingplus.attr.update`인 플로우 메시지 방출, RPC 응답 발행, 디바이스별 `pendingRPC` 상관.
- **M5 (스키마/관찰성)**: 웹 설정 스키마(`agentSchemas.ts`), `ConnectionStats`/`BufferInfo` 관찰성, 예제 agent/flow YAML.

### 구현 파일 (14개)

- `internal/agent/system/thingplus_agent.go`, `thingplus_codec.go`, `thingplus_mapping.go`, `thingplus_register.go` (+ 3 테스트: `thingplus_agent_test.go`, `thingplus_codec_test.go`, `thingplus_mapping_test.go`)
- `internal/node/adapter/thingplus.go` (+ `thingplus_test.go`, adapter `register.go`)
- `web/src/config/agentSchemas.ts`, `web/src/pages/agents/AgentTypesPage.tsx`
- `examples/agents/thingplus-gateway.yaml`, `examples/flows/thingplus-gateway.yaml`

### SPEC 대비 분기 (Divergence)

- **구조체 임베딩 변경 (승인됨)**: §4.2 스케치의 `*agent.BaseAgent` 대신, 기존 `mqtt_agent.go`와 동일하게 `*lifecycle.BaseLifecycle`를 임베딩했다. 기존 시스템 에이전트 패턴과의 일관성을 위한 사용자 승인 결정.
- **stateless Bridge 어댑터 추가 (범위 내)**: §4.1 파일 목록에는 없던 `internal/node/adapter/thingplus.go`(+등록)를 추가했다. 플로우 경계를 넘어 메시지 `Type()`을 보존하기 위한 얇은 무상태 어댑터이며, Bridge 코어 변경이 없으므로 §1.4 스코프("어댑터 추가만 수행, 코어는 변경하지 않음") 내에 해당한다.
- **Process 기반 인바운드 통합**: 플로우→에이전트 업링크/RPC 응답은 `bridge.go`가 보장 호출하는 진입점인 에이전트 `Process([]byte)`로 라우팅했다. 다운링크는 `Type()`을 실어 나르기 위해 완전 마샬된 `message.Message`를 `recvCh`로 방출한다.
- **A8 속성 요청/응답 (선택, 이연)**: `v1/gateway/attributes/request`/`response` 빌더·파서는 구현했으나, 라이브 브로커 인코딩 검증 전까지 tolerant/deferred 상태로 둔다.

### 이연 항목 (라이브 브로커 스모크 테스트 — acceptance.md DoD 기준)

- **A7**: MQTT v5 PUBACK 타이밍 (connect PUBACK 게이팅) — 라이브 브로커 스모크 테스트 대기.
- **A8**: `v1/gateway/attributes/response` 다중 키(client/shared) 인코딩 확정 — 라이브 브로커 스모크 테스트 대기.

### 의존성/구조

- **신규 외부 의존성 0** (기존 Eclipse Paho 재사용). **신규 디렉토리 0** (`internal/node/adapter/`는 기존 존재).

### 품질/커버리지 결과

- 전체 회귀: 11개 패키지 0 FAIL. `go test -race` 통과. `golangci-lint` 0 issues.
- 커버리지: codec 92.6% / mapping 96.6% / adapter 95.0% / 에이전트 코어 80.4% (브로커 전용 경로 제외 시 >90%).
- 품질 findings 3건 수정: (1) lint unused-const → 실사용 연결, (2) `a.cfg` 런타임 재설정 데이터 레이스 → RLock 스냅샷, (3) `pendingRPC` 무한 증가/충돌 → 복합 키 + 경계 있는 축출(bounded eviction).
