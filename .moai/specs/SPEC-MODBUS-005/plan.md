# SPEC-MODBUS-005: 구현 계획 (Implementation Plan)

---
id: SPEC-MODBUS-005
version: 0.1.0
type: plan
---

## 1. 구현 전략 (Implementation Strategy)

DDD (Domain-Driven Development) 방식으로 ANALYZE-PRESERVE-IMPROVE 사이클을 적용한다.

- **ANALYZE**: 기존 단일 디바이스 코드 구조 분석, 의존성 파악
- **PRESERVE**: 기존 동작을 보존하는 특성화 테스트 작성
- **IMPROVE**: 다중 디바이스 지원으로 점진적 확장

---

## 2. 마일스톤 (Milestones)

### M1: 백엔드 - 설정 구조 변경 (Primary Goal)

**목표**: DeviceConfig 도입, 하위 호환성 유지

**태스크**:
1. `DeviceConfig` 구조체 정의
2. `ModbusServerConfig`에 `Devices []DeviceConfig` 필드 추가
3. `parseModbusServerConfig()` 확장: `devices` 배열 파싱
4. 하위 호환 로직: 단일 `unit_id` + `register_map` -> `devices` 자동 변환
5. Unit ID 고유성 검증 로직
6. 기존 테스트 업데이트 + 신규 테스트 추가

**관련 요구사항**: REQ-BE-001, REQ-BE-002, REQ-BE-003

**영향 파일**:
| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `internal/agent/modbusserver/config.go` | 수정 | 높음 |
| `internal/agent/modbusserver/config_test.go` | 수정 | 높음 |

---

### M2: 백엔드 - DeviceManager + 요청 라우팅 (Primary Goal)

**목표**: 디바이스 매니저 구현, Unit ID 기반 라우팅

**태스크**:
1. `DeviceManager` 구조체 구현 (sync.RWMutex 보호)
2. `Device` 구조체 구현 (독립 RegisterMap + Stats)
3. `handler.go`의 UnitID 검증 로직을 DeviceManager 기반으로 변경
4. Broadcast (Unit ID 0) 처리: 쓰기 전파, 읽기 첫 번째 디바이스 응답
5. 미등록 Unit ID 경고 로그
6. 기존 handler 테스트 업데이트 + 신규 라우팅 테스트

**관련 요구사항**: REQ-BE-004, REQ-BE-005, REQ-BE-006, REQ-BE-011

**영향 파일**:
| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `internal/agent/modbusserver/device_manager.go` | 신규 | 높음 |
| `internal/agent/modbusserver/device_manager_test.go` | 신규 | 높음 |
| `internal/agent/modbusserver/handler.go` | 수정 | 높음 |
| `internal/agent/modbusserver/handler_test.go` | 수정 | 중간 |
| `internal/agent/modbusserver/agent.go` | 수정 | 중간 |

---

### M3: 백엔드 - Exec 명령 (Secondary Goal)

**목표**: 디바이스 관리 exec 명령 4종 구현

**태스크**:
1. `list_devices` exec 핸들러 구현
2. `add_device` exec 핸들러 구현 (런타임 디바이스 추가)
3. `remove_device` exec 핸들러 구현 (마지막 디바이스 삭제 방지)
4. `get_device_status` exec 핸들러 구현
5. `agent.go`의 `HandleExecCommand` 메서드에 명령 등록
6. `AgentStats.Extra`에 `device_count` 추가
7. exec 명령 테스트

**관련 요구사항**: REQ-BE-007, REQ-BE-008, REQ-BE-009, REQ-BE-010, REQ-BE-012

**영향 파일**:
| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `internal/agent/modbusserver/agent.go` | 수정 | 높음 |
| `internal/agent/modbusserver/agent_test.go` | 수정 | 높음 |
| `internal/agent/modbusserver/device_manager.go` | 수정 | 중간 |

---

### M4: 프론트엔드 - Devices 탭 확장 (Secondary Goal)

**목표**: Modbus Server 전용 디바이스 탭 UI 구현

**태스크**:
1. `ModbusDevicesTab.tsx` 컴포넌트 생성 (디바이스 목록 + 상세)
2. `DeviceCard` 컴포넌트: unit_id, name, register summary, status badge
3. `DeviceDetail` 컴포넌트: RegisterMapView + DeviceStatsView
4. `AddDeviceModal` 컴포넌트: unit_id, name, register_map 입력 폼
5. `AgentDetailPanel.tsx` Devices 탭 분기 로직 추가 (samsung-nasa / modbus-tcp-server)
6. `agentSchemas.ts` MODBUS_TCP_SERVER_FIELDS 업데이트
7. exec 명령 호출 훅 구현 (useExecAgent 활용)

**관련 요구사항**: REQ-FE-001, REQ-FE-002, REQ-FE-003, REQ-FE-004, REQ-FE-005, REQ-FE-006

**영향 파일**:
| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `web/src/pages/agents/ModbusDevicesTab.tsx` | 신규 | 높음 |
| `web/src/pages/agents/AgentDetailPanel.tsx` | 수정 | 중간 |
| `web/src/config/agentSchemas.ts` | 수정 | 중간 |

---

### M5: 통합 테스트 + 문서 (Final Goal)

**목표**: 전체 통합 검증, SPEC 동기화

**태스크**:
1. 백엔드 통합 테스트: 다중 디바이스 설정 -> 라우팅 -> exec 명령 E2E
2. 프론트엔드-백엔드 연동 확인
3. 기존 단일 디바이스 설정 회귀 테스트
4. SPEC 상태 동기화 (draft -> completed)
5. YAML 예제 파일 업데이트 (다중 디바이스 설정 예시)

**관련 요구사항**: 전체

**영향 파일**:
| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `examples/agents/modbus-server-multi.yaml` | 신규 | 낮음 |
| `.moai/specs/SPEC-MODBUS-005/spec.md` | 수정 | 낮음 |

---

## 3. 기술적 접근 (Technical Approach)

### 3.1 백엔드 아키텍처

**현재 구조** (SPEC-MODBUS-002):
```
Agent -> Handler (단일 unitID) -> RequestHandler -> RegisterMap (단일)
```

**목표 구조** (SPEC-MODBUS-005):
```
Agent -> Handler -> DeviceManager (unit_id 라우팅)
                      +-- Device[1] -> RegisterMap[1]
                      +-- Device[2] -> RegisterMap[2]
                      +-- Device[N] -> RegisterMap[N]
```

**핵심 변경점**:
- `handler.go`의 `unitID` 필드를 `*DeviceManager`로 교체
- `handleRequest()`에서 `DeviceManager.GetDevice(unitID)` 호출
- 동시성 보호: `sync.RWMutex` (읽기: RLock, 쓰기: Lock)

### 3.2 하위 호환 전략

`parseModbusServerConfig()` 진입점에서 분기:

1. `devices` 키가 존재 -> 새 형식으로 파싱
2. `unit_id` + `register_map` 키가 존재하고 `devices` 없음 -> 단일 DeviceConfig로 변환
3. 둘 다 없음 -> 기본 디바이스 (unit_id=1, 빈 RegisterMap)

### 3.3 프론트엔드 전략

- 기존 `DevicesTab` 컴포넌트의 props 패턴 재사용
- `useExecAgent` 훅으로 exec 명령 호출
- Samsung NASA의 `useDevicesRealtime` 패턴 참고하되, Modbus는 exec 기반으로 구현
- 디바이스 목록: 주기적 폴링 또는 stats WebSocket 이벤트 활용

---

## 4. 의존성 그래프 (Dependency Graph)

```
M1 (Config)
  |
  v
M2 (DeviceManager + Routing) --+
  |                              |
  v                              v
M3 (Exec Commands)          M4 (Frontend)
  |                              |
  +--------- M5 (Integration) --+
```

- M1은 M2의 선행 조건
- M2 완료 후 M3와 M4는 병렬 진행 가능
- M5는 M3, M4 모두 완료 후 진행

---

## 5. 리스크 평가 (Risk Assessment)

| 리스크 | 영향도 | 확률 | 대응 전략 |
|--------|--------|------|-----------|
| 동시성 이슈 (디바이스 맵 접근) | 높음 | 중간 | `sync.RWMutex` 사용, `-race` 플래그 테스트 |
| 하위 호환성 파손 | 높음 | 낮음 | 기존 설정 파싱 테스트 강화, 변환 로직 분리 |
| handler.go 대규모 리팩토링 | 중간 | 중간 | DeviceManager로 위임하여 handler 변경 최소화 |
| 프론트엔드 타입 불일치 | 중간 | 낮음 | agentSchemas.ts 수정 후 TypeScript 컴파일 검증 |
| Broadcast 쓰기 순서 보장 | 낮음 | 낮음 | 디바이스 순서대로 순차 쓰기, 에러 집계 |

---

## 6. 추적성 태그 (Traceability Tags)

- `[SPEC-MODBUS-005]` - 본 SPEC 전체
- `[SPEC-MODBUS-005:M1]` - 설정 구조 변경
- `[SPEC-MODBUS-005:M2]` - DeviceManager + 라우팅
- `[SPEC-MODBUS-005:M3]` - Exec 명령
- `[SPEC-MODBUS-005:M4]` - 프론트엔드 UI
- `[SPEC-MODBUS-005:M5]` - 통합 테스트 + 문서
