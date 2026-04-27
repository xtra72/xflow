---
id: SPEC-DEVICE-001
type: plan
status: completed
created: "2026-03-12"
updated: "2026-03-13"
---

# SPEC-DEVICE-001 구현 계획

## 개발 방법론

**Hybrid** (TDD for new + DDD for legacy, `quality.yaml` 기준)

- **새 파일** (`internal/device/`, `web/src/pages/DevicesPage/`): TDD (RED-GREEN-REFACTOR)
- **기존 파일 수정** (`agent.go`, `router.go`, `engine.go`): DDD (ANALYZE-PRESERVE-IMPROVE)

---

## 마일스톤

| 마일스톤 | 모듈 | 우선순위 | 선행 의존성 | 상태 |
|----------|------|----------|-------------|------|
| M1: 통합 디바이스 모델 및 인터페이스 | Module 1 | P0 (즉시) | 없음 | 완료 |
| M2: 디바이스 레지스트리 | Module 2 | P0 (즉시) | M1 완료 필수 | 완료 |
| M3: NASA 디바이스 어댑터 | Module 1 | P0 (즉시) | M1 완료 필수 | 완료 |
| M4: MODBUS 디바이스 어댑터 | Module 1 | P0 (즉시) | M1 완료 필수 | 완료 |
| M5: 에이전트 DeviceProvider 통합 | Module 2 | P0 (즉시) | M2, M3, M4 완료 필수 | 완료 |
| M6: 메타데이터 영속성 | Module 2 | P1 (중요) | M2 완료 필수 | 완료 |
| M7: 디바이스 REST API | Module 3 | P1 (중요) | M5 완료 필수 | 완료 |
| M8: 디바이스 목록 웹 UI | Module 4 | P2 (개선) | M7 완료 필수 | 완료 |
| M9: 디바이스 상세/제어 웹 UI | Module 4 | P2 (개선) | M8 완료 필수 | 완료 |

---

## 마일스톤 상세

### M1: 통합 디바이스 모델 및 인터페이스

**대상 파일**:
- `internal/device/device.go` - Device, DeviceState, DeviceType, DeviceMetadata
- `internal/device/controllable.go` - ControllableDevice, CommandSpec, ParamSpec
- `internal/device/filter.go` - DeviceFilter
- `internal/device/errors.go` - 센티널 에러 (ErrDeviceNotFound, ErrNotControllable, ErrAgentStopped 등)

**방법론**: TDD (새 패키지)

**작업 내용**:
- Device 인터페이스 정의 (ID, Name, Type, Protocol, AgentName, Online, LastSeen, State, Metadata, Capabilities)
- DeviceState 구조체 (공통 필드 + Properties map)
- ControllableDevice 인터페이스 (Execute, Commands)
- CommandSpec/ParamSpec 명령 스키마 구조체
- DeviceMetadata 구조체 (Tags, Location, Group, Labels)
- DeviceType enum (indoor, outdoor, controller, sensor, actuator)
- DeviceProvider 인터페이스 (Devices, Device)

### M2: 디바이스 레지스트리

**대상 파일**:
- `internal/device/registry.go` - DeviceRegistry 인터페이스 및 구현체
- `internal/device/registry_test.go` - 레지스트리 단위 테스트

**방법론**: TDD (새 파일)

**작업 내용**:
- DeviceRegistry 인터페이스 정의 (List, Get, Count, RegisterProvider, UnregisterProvider, SetMetadata, GetMetadata, Execute)
- 인메모리 구현체 (DeviceProvider 맵 관리)
- 동시성 안전 (sync.RWMutex)
- DeviceFilter 기반 필터링 로직
- 중복 ID 방지 로직

### M3: NASA 디바이스 어댑터

**대상 파일**:
- `internal/device/adapter/nasa_adapter.go` - NASADevice -> Device 변환
- `internal/device/adapter/nasa_adapter_test.go` - 어댑터 테스트

**방법론**: TDD (새 파일)

**작업 내용**:
- NASADevice를 Device 인터페이스로 래핑하는 어댑터 구현
- NASA 프로토콜 상태값을 Properties map으로 변환
- ControllableDevice 구현 (Execute를 NASAAgent.Process로 위임)
- CommandSpec 생성 (set_temperature, set_mode, set_power 등)

### M4: MODBUS 디바이스 어댑터

**대상 파일**:
- `internal/device/adapter/modbus_adapter.go` - ModbusDevice -> Device 변환
- `internal/device/adapter/modbus_adapter_test.go` - 어댑터 테스트

**방법론**: TDD (새 파일)

**작업 내용**:
- ModbusDevice를 Device 인터페이스로 래핑하는 어댑터 구현
- MODBUS 레지스터 데이터를 Properties map으로 변환
- ControllableDevice 조건부 구현 (쓰기 가능 디바이스만)
- CommandSpec 생성 (write_register, write_coil 등)

### M5: 에이전트 DeviceProvider 통합

**대상 파일**:
- `internal/agent/samsung/provider.go` - NASAAgent에 DeviceProvider 구현 (새 파일, TDD)
- `internal/agent/modbus/provider.go` - MODBUSAgent에 DeviceProvider 구현 (새 파일, TDD)
- `internal/engine/engine.go` - DeviceRegistry 초기화 및 에이전트 연결 (기존 파일, DDD)

**방법론**: 혼합 (새 파일 TDD + 기존 파일 DDD)

**작업 내용**:
- NASAAgent에서 DeviceProvider.Devices() 구현 (내부 디바이스 맵을 어댑터로 래핑)
- MODBUSAgent에서 DeviceProvider.Devices() 구현 (내부 디바이스 맵을 어댑터로 래핑)
- Engine에서 DeviceRegistry 생성 및 에이전트 시작/중지 시 Provider 등록/해제
- DDD: Engine 기존 동작에 대한 characterization test 선행

### M6: 메타데이터 영속성

**대상 파일**:
- `internal/storage/device_metadata.go` - DeviceMetadataRepository (새 파일, TDD)
- `internal/storage/device_metadata_test.go` - 리포지토리 테스트

**방법론**: TDD (새 파일)

**작업 내용**:
- DeviceMetadataRepository 인터페이스 정의
- SQLite/PostgreSQL 기반 구현 (기존 storage 패턴 활용)
- 디바이스 ID 기반 CRUD (tags, location, group, labels)
- 레지스트리와 연동 (SetMetadata/GetMetadata 위임)

### M7: 디바이스 REST API

**대상 파일**:
- `internal/api/handler/device.go` - DeviceHandler (새 파일, TDD)
- `internal/api/dto/device.go` - 디바이스 DTO (새 파일)
- `internal/api/router.go` - `/api/devices` 라우트 등록 (기존 파일, DDD)
- `internal/api/handler/device_test.go` - API 핸들러 테스트

**방법론**: 혼합 (새 파일 TDD + 기존 파일 DDD)

**작업 내용**:
- DeviceHandler 구현 (List, Get, UpdateMetadata, Execute, GetCommands, GetState)
- DeviceDTO, DeviceStateDTO, CommandSpecDTO 정의
- Device -> DTO 변환 로직
- 에러 핸들링 (404 NotFound, 405 NotAllowed, 409 Conflict)
- DDD: router.go 기존 라우트에 대한 characterization test 선행
- 라우트 그룹 `/api/devices` 등록

### M8: 디바이스 목록 웹 UI

**대상 파일**:
- `web/src/pages/DevicesPage/DevicesPage.tsx` - 디바이스 목록 페이지
- `web/src/types/device.ts` - TypeScript 타입 정의
- `web/src/services/deviceApi.ts` - API 클라이언트
- `web/src/stores/deviceStore.ts` - Zustand 스토어
- `web/src/App.tsx` - 라우트 추가 (기존 파일, DDD)
- `web/src/components/Sidebar/` - 메뉴 항목 추가 (기존 파일, DDD)

**방법론**: 혼합 (새 파일 TDD + 기존 파일 DDD)

**작업 내용**:
- Device TypeScript 타입 정의 (DeviceResponse, DeviceState, CommandSpec 등)
- deviceApi 서비스 (fetchDevices, fetchDevice, executeCommand, updateMetadata)
- deviceStore (Zustand: devices, selectedDevice, filter, loading)
- DevicesPage 컴포넌트 (테이블, 필터, 검색)
- DDD: App.tsx, Sidebar 기존 동작 보존

### M9: 디바이스 상세/제어 웹 UI

**대상 파일**:
- `web/src/pages/DeviceDetailPage/DeviceDetailPage.tsx` - 디바이스 상세 페이지
- `web/src/components/DevicePanel/DeviceStateCard.tsx` - 상태 카드
- `web/src/components/DevicePanel/DeviceControlPanel.tsx` - 제어 패널
- `web/src/components/DevicePanel/DeviceMetadataEditor.tsx` - 메타데이터 편집기

**방법론**: TDD (새 파일)

**작업 내용**:
- DeviceDetailPage: 상세 정보 렌더링
- DeviceStateCard: 상태 시각화 (온라인/오프라인, Properties 테이블)
- DeviceControlPanel: CommandSpec 기반 동적 제어 UI 생성 (슬라이더, 드롭다운, 버튼)
- DeviceMetadataEditor: 태그, 위치, 그룹, 레이블 편집

---

## 기술 스택

| 영역 | 기술 |
|------|------|
| Backend | Go 1.23+, Fiber (기존 라우터), SQLite/PostgreSQL (기존 스토리지) |
| Frontend | React 19, TypeScript, Zustand, Tailwind CSS (기존 스택) |
| 테스트 (Backend) | `go test`, table-driven tests, `go test -race` |
| 테스트 (Frontend) | Vitest |

---

## 리스크 분석

| 리스크 | 영향 | 대응 |
|--------|------|------|
| NASAAgent 비동기 디바이스 탐색과 레지스트리 동기화 타이밍 | 중 | Pull 모델로 동기화 이슈 최소화 |
| ModbusDevice 폴링 기반 상태와 실시간 조회 정합성 | 중 | DeviceProvider.Devices()는 현재 스냅샷 반환 |
| 기존 에이전트 코드 변경 시 회귀 | 높 | DDD 방법론으로 characterization test 선행 |
| 제어 명령의 프로토콜별 변환 복잡도 | 중 | CommandSpec으로 명령 스키마 표준화 |

---

status: completed
updated: "2026-03-13"
