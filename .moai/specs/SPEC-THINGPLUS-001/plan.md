---
id: SPEC-THINGPLUS-001
type: plan
version: "1.0.0"
spec_ref: SPEC-THINGPLUS-001
---

# SPEC-THINGPLUS-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반)
  - 신규 코드 (`thingplus_agent.go`, `thingplus_mapping.go`, `thingplus_codec.go`, `thingplus_register.go`): TDD (RED-GREEN-REFACTOR)
  - 기존 파일 수정 (`web/src/config/agentSchemas.ts`): DDD (ANALYZE-PRESERVE-IMPROVE, 기존 스키마 패턴 보존)
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race ./...` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.25+
- **MQTT**: Eclipse Paho MQTT Go (`github.com/eclipse/paho.mqtt.golang`, 기존 의존성)
- **테스트**: Go 표준 `testing` + `github.com/stretchr/testify`, table-driven + channel-mocked broker
- **동시성**:
  - `sync.RWMutex` (디바이스 상태 맵, NAME↔id 매핑 보호)
  - bounded 채널 버퍼 (업링크 무손실, `mqtt_agent.go` recvCh 패턴 재사용)
- **웹**: TypeScript (`agentSchemas.ts` ConfigField[])

### 1.3 패키지 위치

- `internal/agent/system/thingplus_agent.go`: 에이전트 코어 (연결/구독/발행/State)
- `internal/agent/system/thingplus_mapping.go`: NAME↔device_id 매핑, JSONPath 추출
- `internal/agent/system/thingplus_codec.go`: 페이로드 build/parse
- `internal/agent/system/thingplus_register.go`: 타입 등록
- `web/src/config/agentSchemas.ts`: 웹 설정 스키마
- `examples/agents/`, `examples/flows/`: 예제 YAML

---

## 2. 마일스톤

### Primary Goal (M1): Gateway Core & Connection/Auth (REQ-core-*)

**범위**: REQ-THINGPLUS-001-core-register, core-config, core-auth, core-reconnect, core-state, core-nocred-log

**작업 항목**:

1. `thingplus_register.go` 작성
   - `RegisterThingplusTypes(mgr)` 로 `"thingplus-gateway"` 팩토리 등록 (`mqtt_register.go:8` 패턴)
2. `thingplus_agent.go` 골격 작성
   - `ThingplusConfig` 구조체 및 파싱 (`Init`/`Configure`), 기본값(port=1883, path=`$.device`)
   - `NewThingplusGatewayAgent(config)`, `BaseAgent` 임베딩
   - Paho 연결: access token → username, password 빈 문자열, TLS(8883, CA) 옵션
   - `AutoReconnect` + `OnConnect` 핸들러 골격 (다운링크 구독은 M4에서 채움)
   - `State()` 스냅샷 (연결 상태, 디바이스 수, 통계)
   - access token 로그 마스킹
3. 테스트: 설정 파싱 table-driven, 연결(모킹) 성공/실패, State() 형태 검증

**산출물**: 등록 가능한 `thingplus-gateway` 에이전트 코어 및 인증 연결

---

### Secondary Goal (M2): Device Connect & NAME↔ID Mapping (REQ-map-*)

**범위**: REQ-THINGPLUS-001-map-resolve, map-connect, map-puback, map-reconnect, map-disconnect, map-fallback

**작업 항목**:

1. `thingplus_mapping.go` 작성
   - `nameIDMap` 양방향 매핑(NAME↔device_id), `sync.RWMutex`
   - JSONPath 추출 (`Payload.GetPath`, 기본 `$.device`, 대안 경로 허용)
   - `ResolveDeviceID(ctx, agentName, unitID)` 통합, repo-nil fallback(NAME=device_id)
2. `thingplus_agent.go` 확장
   - `deviceStateMap` 상태 머신 (disconnected→connecting→connected)
   - `v1/gateway/connect` / `v1/gateway/disconnect` 발행
   - connect PUBACK 게이팅 (`token.Wait()`/v5 PUBACK → connected 전이)
   - 재연결 시 알려진 디바이스 재connect
3. 테스트: 매핑 build/lookup/역매핑, nil-repo fallback, 중복 NAME, connect/PUBACK 상태 전이

**산출물**: 디바이스 자동 connect 및 NAME↔id 매핑 계층

---

### Tertiary Goal (M3): Telemetry/Attributes Uplink (REQ-up-*)

**범위**: REQ-THINGPLUS-001-up-telemetry, up-ts, up-attributes, up-batch, up-nolost

**작업 항목**:

1. `thingplus_codec.go` 텔레메트리/속성 빌더
   - `{NAME:[{ts,values}]}`, `ts = time.Time.UnixMilli()`, ts 생략 케이스
   - `{NAME:{k:v}}` 클라이언트 속성, 배치 조립
2. `thingplus_agent.go` 업링크 경로
   - 인입 메시지 → NAME 추출 → auto-connect 보장 → 텔레메트리/속성 발행
   - `boundedBuffer` 업링크 무손실 버퍼링, 초과 시 메트릭/에러
3. 테스트: 페이로드 build(ts 검증 포함), 배치, 연결 끊김 시 버퍼링 무손실

**산출물**: 업링크 텔레메트리/속성 발행 및 무손실 버퍼

---

### Quaternary Goal (M4): RPC/Shared-attr Downlink → Flow (REQ-dn-*)

**범위**: REQ-THINGPLUS-001-dn-rpc-recv, dn-rpc-reply, dn-shared, dn-attr-req, dn-name-resolve

**작업 항목**:

1. `thingplus_codec.go` 다운링크 파서/빌더
   - RPC 파싱 `{device,data:{id,method,params}}`, RPC 응답 빌더 `{device,id,data}`
   - 공유 속성 파싱 `{device,data}`
   - (선택) `attributes/request` 빌더 + `attributes/response` id 상관
2. `thingplus_agent.go` 다운링크 경로
   - `OnConnect`에서 `v1/gateway/rpc`, `v1/gateway/attributes` 구독
   - RPC → `Type()="thingplus.rpc.request"` 방출, 공유 속성 → `Type()="thingplus.attr.update"` 방출
   - 플로우 응답 → RPC 응답 발행, NAME→device_id 역매핑 일관성
3. **라이브 브로커 스모크 테스트** (M4 파서 확정 전): `attributes/response` 인코딩(A8), MQTT v5 PUBACK 타이밍(A7) 확인
4. 테스트: RPC SUB→emit→reply PUB (channel-mocked), 공유 속성 방출

**산출물**: RPC/공유 속성 다운링크 및 RPC 응답 왕복

---

### Final Goal (M5): Config Schema, Web & Observability (REQ-web-*, obs-*)

**범위**: REQ-THINGPLUS-001-web-schema, obs-connstats, obs-buffer, obs-example

**작업 항목**:

1. `web/src/config/agentSchemas.ts` 수정 (`MQTT_FIELDS ~line 29` 참고)
   - `thingplus-gateway` ConfigField[]: broker, port, tls(+ca), access_token(sensitive), device_name_path(기본 `"$.device"`), qos, keepalive, auto_reconnect, buffer_size
2. `thingplus_agent.go` 관찰성
   - `ConnectionStatsProvider`, `BufferInfoProvider` 구현
3. 예제 YAML: `examples/agents/thingplus-gateway.yaml`, `examples/flows/thingplus-gateway.yaml`
4. 테스트: 스키마 로딩(웹), 통계/버퍼 인터페이스 노출

**산출물**: 웹 설정 스키마, 관찰성 인터페이스, 예제 YAML

---

## 3. 기술적 접근

### 3.1 MQTT 에이전트 재사용

`mqtt_agent.go`의 MQTTConfig, connect(~194), subscribe(~285), PublishMessage(~523), recvCh(~389), State()(~737) 패턴을 참조 모델로 삼아 `thingplus_agent.go`를 구성한다. 게이트웨이 특화 로직(디바이스 다중화, 상태 머신, 코덱)만 신규 표면으로 추가한다.

### 3.2 디바이스 상태 머신 및 RPC 게이팅

```
disconnected --connect PUB--> connecting --PUBACK--> connected --disconnect PUB--> disconnected
```

RPC 응답 발행은 `connected` 상태에서만 수행한다. connect 토큰 완료를 상태 전이 조건으로 사용하여 v5 PUBACK 순서 불확실성(A7)에 대응한다.

### 3.3 NAME↔device_id 매핑

인입 메시지에서 `device_name_path`(기본 `$.device`) JSONPath로 NAME 추출 → `ResolveDeviceID`로 device_id 해석 → 양방향 맵 채움. repo-nil/미해석 시 NAME=device_id fallback으로 동작 지속.

---

## 4. 리스크 분석 및 대응

### Risk 1 (A7): connect PUBACK 순서 / RPC 유실

- **위험**: MQTT v5에서 PUBACK 순서 미보장 시, connect 완료 전에 RPC를 처리/발행하여 유실 가능.
- **대응**: 디바이스 상태 머신으로 게이팅. connect `token.Wait()` 완료(`connected`) 전에는 RPC 응답 발행을 보류. M4 확정 전 라이브 브로커로 v5 PUBACK 타이밍 검증.

### Risk 2 (A8): attributes/response 인코딩 불확실

- **위험**: `v1/gateway/attributes/response`의 다중 키(client/shared) 인코딩이 문서와 상이할 수 있음.
- **대응**: `dn-attr-req`를 Optional로 분리하고, M4 파서 확정 전 라이브 브로커 스모크 테스트로 실제 응답 구조 확인 후 코덱 확정.

### Risk 3: NAME↔id 매핑 불일치

- **위험**: 동일 NAME 중복, device_id 미해석, 에이전트 이름 충돌.
- **대응**: 양방향 맵 + `sync.RWMutex`, repo-nil fallback(NAME=device_id), 중복 NAME 처리 테스트로 일관성 보장.

### Risk 4: 재연결 시 디바이스 재connect 누락

- **위험**: 브로커 재연결 후 이전 connected 디바이스가 게이트웨이에 재연결되지 않아 텔레메트리/RPC 유실.
- **대응**: `OnConnect`에서 알려진 디바이스 순회 재connect + 다운링크 재구독. 통합 테스트로 검증.

### Risk 5: 비밀정보(access token) 노출

- **위험**: 토큰이 로그/State에 평문 노출.
- **대응**: username으로만 사용, 로그 마스킹, State/스키마에서 sensitive 표시.

---

## 5. 의존성 그래프

```
thingplus_register.go
  └── 의존: internal/agent (DefaultManager, RegisterType)
  └── 소비자: agent 부트스트랩 (RegisterThingplusTypes 호출)

thingplus_mapping.go
  ├── 의존: pkg/message (Payload.GetPath, Metadata)
  ├── 의존: internal/agent/device_id_repo.go (ResolveDeviceID)
  └── 소비자: thingplus_agent.go

thingplus_codec.go
  ├── 의존: pkg/message (Type dot-notation)
  └── 소비자: thingplus_agent.go

thingplus_agent.go
  ├── 의존: internal/agent (BaseAgent + 옵션 인터페이스)
  ├── 의존: github.com/eclipse/paho.mqtt.golang
  ├── 의존: mqtt_agent.go 패턴 (참조 모델)
  ├── 의존: thingplus_mapping.go, thingplus_codec.go
  └── 소비자: internal/node/bridge.go (어댑터 경유 플로우 연동)

web/src/config/agentSchemas.ts
  └── 소비자: 웹 UI (thingplus-gateway 설정 폼)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | thingplus_register.go | 타입 등록 | internal/agent |
| 2 | thingplus_agent.go (코어) | 설정/연결/State 골격 | 1 |
| 3 | thingplus_mapping.go | NAME↔id 매핑, JSONPath | pkg/message, device_id_repo |
| 4 | thingplus_agent.go (디바이스) | 상태 머신, connect/disconnect | 2, 3 |
| 5 | thingplus_codec.go (업링크) | 텔레메트리/속성 빌더 | pkg/message |
| 6 | thingplus_agent.go (업링크) | 발행 + 버퍼링 | 4, 5 |
| 7 | thingplus_codec.go (다운링크) | RPC/공유 속성 파서/빌더 | pkg/message |
| 8 | thingplus_agent.go (다운링크) | 구독 + 방출 + RPC 응답 | 6, 7 |
| 9 | agentSchemas.ts | 웹 설정 스키마 | 없음 |
| 10 | examples/*.yaml | 예제 agent/flow | 전체 |

모든 Go 파일에 대해 Hybrid 방식(신규 TDD)으로 테스트 파일(`*_test.go`)을 작성한다. M4 다운링크 코덱 확정 전에 라이브 브로커 스모크 테스트를 반드시 수행한다.
