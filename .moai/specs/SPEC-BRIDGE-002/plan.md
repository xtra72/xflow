---
id: SPEC-BRIDGE-002
document: plan
version: "1.0.0"
created: "2026-03-05"
updated: "2026-03-05"
---

# SPEC-BRIDGE-002 구현 계획: Agent-Specific Dedicated Bridge Implementation

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **개발 모드**: Hybrid (DDD for 기존 코드 수정 + TDD for 신규 코드)
- **기존 코드 수정**: ANALYZE-PRESERVE-IMPROVE 사이클 (BridgeNode, BridgeTransformer 확장)
- **신규 코드**: RED-GREEN-REFACTOR 사이클 (BridgeAdapter 인터페이스, 각 어댑터, 프론트엔드 컴포넌트)

### 1.2 아키텍처 접근 방향

BridgeAdapter 인터페이스와 AdapterRegistry를 핵심 추상화로 도입하되, 기존 BridgeNode의 Init/Process/receiveLoop 구조는 최소한으로 변경한다. 기존 DefaultTransformer는 DefaultAdapter로 래핑하여 하위 호환성을 보장한다.

**핵심 원칙**:
- BridgeNode의 기존 동작을 변경하지 않고 어댑터 계층을 추가
- 어댑터가 없는 에이전트 타입은 자동으로 DefaultAdapter 폴백
- 각 어댑터는 독립적으로 테스트 가능한 순수 변환 로직

---

## 2. 마일스톤

### Milestone 1: BridgeAdapter 인터페이스 및 Registry (Primary Goal, P0)

**대상 모듈**: Module 1 (REQ-BRIDGE-002-001 ~ 004)

**태스크**:

1. `internal/node/bridge_adapter.go` 생성
   - BridgeAdapter 인터페이스 정의
   - AgentMeta 구조체 정의
   - BridgeAdapter와 기존 BridgeTransformer 간의 브릿지 래퍼 타입 정의

2. `internal/node/bridge_adapter_registry.go` 생성
   - AdapterRegistry 구조체 (sync.RWMutex + map[string]BridgeAdapter)
   - RegisterAdapter / GetAdapter 메서드
   - globalAdapterRegistry 전역 인스턴스
   - DefaultAdapter 구현 (기존 DefaultTransformer 동작 래핑)

3. `internal/node/bridge_adapter_test.go` 생성
   - DefaultAdapter 동작 검증 테스트
   - AdapterRegistry 등록/조회/폴백 테스트
   - 동시성 안전성 테스트 (race condition)

4. `internal/node/bridge.go` 수정
   - BridgeNode 구조체에 adapter 필드 추가
   - Init()에서 AdapterRegistry 조회 로직 추가
   - receiveLoop/Process에서 adapter 우선 사용 로직 추가

5. 기존 BridgeNode 테스트 회귀 검증

**의존성**: 없음 (첫 번째 마일스톤)
**선행 조건**: SPEC-BRIDGE-001 구현 완료 (완료됨)

---

### Milestone 2: System Agent 통합 어댑터 (Secondary Goal, P0)

**대상 모듈**: Module 6 (REQ-BRIDGE-002-050 ~ 052)

**태스크**:

1. `internal/node/adapter/system.go` 생성
   - SystemAdapter 구조체 (기존 BridgeHandler 래핑)
   - 에이전트 서브타입별 메타데이터 접두사 매핑
   - BridgeAdapter 인터페이스 구현

2. `internal/node/adapter/system_test.go` 생성
   - store 서브타입 변환 테스트
   - timer 서브타입 변환 테스트
   - logger 서브타입 변환 테스트
   - 유효하지 않은 서브타입 검증 테스트

3. `internal/node/adapter/register.go` 생성
   - init() 함수에서 SystemAdapter를 "system" 키로 자동 등록
   - 서브타입별 추가 등록 (store, timer, logger, event, file)

**의존성**: Milestone 1 완료 필요 (BridgeAdapter 인터페이스, AdapterRegistry)
**기존 코드 영향**: store_bridge.go, timer_bridge.go, logger_bridge.go는 수정하지 않고 래핑만 수행

---

### Milestone 3: MQTT 전용 어댑터 (Tertiary Goal, P0)

**대상 모듈**: Module 2 (REQ-BRIDGE-002-010 ~ 015)

**태스크**:

1. `internal/node/adapter/mqtt.go` 생성
   - MQTTAdapter 구조체
   - 토픽 라우팅 변환 (mqtt.topic 메타데이터)
   - QoS 레벨 관리 (mqtt.qos 메타데이터)
   - Retained 플래그 처리 (mqtt.retained 메타데이터)
   - 토픽 템플릿 보간 로직 ({field_name} -> payload 값)
   - 와일드카드 구독 위임 로직
   - Validate() 구현 (토픽 형식, QoS 범위, 필수 토픽 검증)

2. `internal/node/adapter/mqtt_test.go` 생성
   - TransformToFlow 테이블 드리븐 테스트 (다양한 QoS, retained 조합)
   - TransformToAgent 테이블 드리븐 테스트 (토픽 라우팅, 템플릿 보간)
   - Validate 검증 테스트 (유효/무효 설정)
   - 와일드카드 토픽 처리 테스트

3. register.go에 MQTT 어댑터 등록 추가

**의존성**: Milestone 1 완료 필요
**기존 코드 참조**: `internal/agent/mqtt/` 패키지의 SubscriberAgent 인터페이스

---

### Milestone 4: Modbus 전용 어댑터 (Quaternary Goal, P0)

**대상 모듈**: Module 3 (REQ-BRIDGE-002-020 ~ 026)

**태스크**:

1. `internal/node/adapter/modbus.go` 생성
   - ModbusAdapter 구조체
   - RegisterDef 구조체 (이름, 주소, 개수, 데이터 타입, 바이트 오더, 스케일, 오프셋)
   - 레지스터 맵 변환: uint16[] -> Go 네이티브 타입 (internal/modbus/ 패키지 활용)
   - 역변환: Go 네이티브 타입 -> uint16[]
   - 폴링 인터벌 설정 관리
   - 기능 코드 <-> Bridge Direction 매핑 로직
   - Validate() 구현 (Unit ID 범위, 레지스터 맵, 데이터 타입, 폴링 인터벌)

2. `internal/node/adapter/modbus_test.go` 생성
   - 레지스터 -> 네이티브 타입 변환 테스트 (int16, uint16, float32, uint32, int32)
   - 역변환 테스트
   - 바이트 오더(big/little) 처리 테스트
   - 스케일/오프셋 적용 테스트
   - Validate 검증 테스트 (Unit ID, 레지스터 정의, 폴링 인터벌)
   - 기능 코드 매핑 테스트

3. register.go에 Modbus 어댑터 등록 추가 (modbus-tcp, modbus-rtu, modbus-tcp-server)

**의존성**: Milestone 1 완료 필요
**기존 코드 참조**: `internal/modbus/` 패키지의 데이터 타입 변환 유틸리티

---

### Milestone 5: HTTP 전용 어댑터 (Optional Goal, P1)

**대상 모듈**: Module 4 (REQ-BRIDGE-002-030 ~ 034)

**태스크**:

1. `internal/node/adapter/http.go` 생성
   - HTTPAdapter 구조체
   - Content-Type별 변환 전략 (JSON, Form, Multipart, Raw)
   - 상태 코드 -> 메타데이터 매핑
   - 헤더 -> 메타데이터 양방향 변환
   - URL 경로 템플릿 보간
   - Validate() 구현

2. `internal/node/adapter/http_test.go` 생성
   - Content-Type별 변환 테스트
   - 상태 코드 매핑 테스트
   - 헤더 변환 테스트
   - URL 템플릿 보간 테스트
   - Validate 검증 테스트

3. register.go에 HTTP 어댑터 등록 추가

**의존성**: Milestone 1 완료 필요

---

### Milestone 6: 프론트엔드 에이전트별 설정 UI (Optional Goal, P1)

**대상 모듈**: Module 5 (REQ-BRIDGE-002-040 ~ 045)

**태스크**:

1. `web/src/config/bridgeAdapterSchemas.ts` 생성
   - 에이전트 타입별 설정 필드 스키마 정의
   - MQTT: topics, publishTopic, qos, retained, topicTemplate
   - Modbus: unitId, pollingInterval, registerMap (RegisterDef[])
   - HTTP: urlPath, contentType, headers, timeout

2. `web/src/config/nodeSchemas.ts` 수정
   - BRIDGE_AGENT_DEFAULTS에 configFields 배열 추가
   - bridgeAdapterSchemas.ts import 및 통합

3. `web/src/components/property/BridgeMqttConfig.tsx` 생성
   - 구독 토픽 다중 입력 컴포넌트
   - QoS 드롭다운 (0, 1, 2)
   - Retained 토글
   - 발행 토픽 + 템플릿 입력

4. `web/src/components/property/BridgeModbusConfig.tsx` 생성
   - Unit ID 입력
   - 폴링 인터벌 입력
   - RegisterMapEditor 컴포넌트 통합 (기존 재사용 또는 신규 생성)

5. `web/src/components/property/BridgeHttpConfig.tsx` 생성
   - URL 경로 입력
   - Content-Type 선택
   - 헤더 key-value 편집기
   - 타임아웃 입력

6. `web/src/components/property/PropertyPanel.tsx` 수정
   - 에이전트 타입 감지 로직 추가
   - 에이전트별 설정 패널 조건부 렌더링
   - 에이전트 타입별 유효성 검증 연동

7. i18n 키 추가 (`web/src/lib/i18n/en.json`, `ko.json`)

**의존성**: Milestone 1~4의 인터페이스 설계 확정 필요 (설정 스키마 동기화)

---

## 3. 태스크 의존성 그래프

```
Milestone 1 (인터페이스 + Registry)
    |
    +---> Milestone 2 (System Adapter)
    |
    +---> Milestone 3 (MQTT Adapter)
    |
    +---> Milestone 4 (Modbus Adapter)
    |
    +---> Milestone 5 (HTTP Adapter)
    |
    +---> Milestone 6 (Frontend UI)
              |
              +---> Milestone 1~4 설계 확정 필요
```

- Milestone 2, 3, 4, 5는 서로 독립적이며 Milestone 1 이후 병렬 진행 가능
- Milestone 6은 백엔드 어댑터의 설정 구조가 확정된 후 진행

---

## 4. 기술적 접근

### 4.1 BridgeAdapter와 BridgeTransformer 공존 전략

기존 BridgeTransformer 인터페이스는 유지하면서, BridgeAdapter를 BridgeTransformer로 래핑하는 어댑터 패턴을 사용한다.

```
AdapterTransformerBridge 구조:
  - BridgeTransformer 인터페이스 구현
  - 내부에 BridgeAdapter 보유
  - AgentToFlow() -> adapter.TransformToFlow() 위임
  - FlowToAgent() -> adapter.TransformToAgent() 위임
```

이 접근 방식으로 BridgeNode의 기존 transformer 사용 코드를 최소한으로 변경한다.

### 4.2 AgentMeta 전파 전략

AgentTransport 인터페이스의 Send/Receive는 현재 message.Message를 사용한다. AgentMeta 정보는 Message의 Metadata에 프로토콜 접두사 키로 저장하여 전파한다.

- MQTT: `mqtt.topic`, `mqtt.qos`, `mqtt.retained`
- Modbus: `modbus.unit_id`, `modbus.function_code`, `modbus.register_addr`
- HTTP: `http.status_code`, `http.content_type`, `http.header.*`
- System: `{subtype}.operation`, `{subtype}.key`

### 4.3 Modbus 데이터 타입 변환 재사용

`internal/modbus/` 패키지의 기존 데이터 타입 변환 로직을 최대한 재사용한다. ModbusAdapter는 RegisterDef 설정에 따라 적절한 변환 함수를 선택하는 디스패처 역할만 수행한다.

### 4.4 프론트엔드 동적 렌더링 전략

에이전트 타입별 설정 스키마를 `bridgeAdapterSchemas.ts`에 중앙 관리하고, PropertyPanel에서 에이전트 타입에 따라 해당 설정 컴포넌트를 동적 import한다. React.lazy를 사용하여 사용하지 않는 어댑터 설정 컴포넌트의 번들 크기 영향을 최소화한다.

---

## 5. 리스크 분석

### 5.1 기존 BridgeNode 회귀 위험

- **리스크**: adapter 계층 추가 시 기존 DefaultTransformer 기반 동작이 깨질 수 있음
- **대응**: Milestone 1에서 기존 BridgeNode 테스트를 먼저 실행하여 베이스라인 확보, DefaultAdapter가 DefaultTransformer와 동일한 동작을 하는지 비교 테스트 작성

### 5.2 AgentTransport 인터페이스 변경 범위

- **리스크**: AgentMeta 정보를 전달하기 위해 AgentTransport.Send/Receive 시그니처 변경이 필요할 수 있음
- **대응**: Message.Metadata를 통한 AgentMeta 전파 방식 채택으로 인터페이스 변경 최소화. AgentMeta <-> Metadata 변환은 어댑터 내부에서 처리

### 5.3 Modbus 데이터 타입 변환 정확도

- **리스크**: float32 등 부동소수점 변환 시 정밀도 손실 가능
- **대응**: internal/modbus/ 패키지의 검증된 변환 로직 재사용, 경계값 테스트 추가 (NaN, Inf, 최대/최소값)

### 5.4 System Agent BridgeHandler 호환성

- **리스크**: 기존 BridgeHandler가 직접 호출되던 경로가 SystemAdapter를 통해 간접 호출로 변경되면서 동작 차이 발생 가능
- **대응**: SystemAdapter는 기존 BridgeHandler를 있는 그대로 래핑하며, 메타데이터 디스패치 로직을 변경하지 않음. 기존 BridgeHandler 테스트를 SystemAdapter 경로로도 실행하여 동등성 검증

### 5.5 프론트엔드 설정 스키마 동기화

- **리스크**: 백엔드 BridgeConfig 구조와 프론트엔드 설정 스키마 간 불일치 발생 가능
- **대응**: bridgeAdapterSchemas.ts의 타입 정의와 Go 구조체를 수동 대조 검증. 추후 코드 생성 도구 도입 검토

---

## 6. 참조

### 선행 SPEC

- **SPEC-BRIDGE-001**: Bridge Node System (기반 구현)
- **SPEC-MSG-001**: Message, Payload, Metadata 인터페이스
- **SPEC-MQTT-001**: MQTT Agent (SubscriberAgent 인터페이스)
- **SPEC-MODBUS-001**: MODBUS Agent
- **SPEC-STORE-001**: Store Agent (BridgeHandler)
- **SPEC-TIMER-001**: Timer Agent (TimerBridgeHandler)

### 영향 받는 기존 파일

- `internal/node/bridge.go` - BridgeNode 수정 (adapter 필드 추가, Init/Process 수정)
- `web/src/config/nodeSchemas.ts` - BRIDGE_AGENT_DEFAULTS 확장
- `web/src/components/property/PropertyPanel.tsx` - 동적 패널 렌더링 추가

### 신규 생성 파일

- `internal/node/bridge_adapter.go`
- `internal/node/bridge_adapter_registry.go`
- `internal/node/bridge_adapter_test.go`
- `internal/node/adapter/mqtt.go`
- `internal/node/adapter/mqtt_test.go`
- `internal/node/adapter/modbus.go`
- `internal/node/adapter/modbus_test.go`
- `internal/node/adapter/http.go`
- `internal/node/adapter/http_test.go`
- `internal/node/adapter/system.go`
- `internal/node/adapter/system_test.go`
- `internal/node/adapter/register.go`
- `web/src/config/bridgeAdapterSchemas.ts`
- `web/src/components/property/BridgeMqttConfig.tsx`
- `web/src/components/property/BridgeModbusConfig.tsx`
- `web/src/components/property/BridgeHttpConfig.tsx`

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-05*
*작성: MoAI SPEC Builder*
