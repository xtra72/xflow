# Changelog

이 프로젝트의 주요 변경 사항을 기록한다.
형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르며,
[Semantic Versioning](https://semver.org/lang/ko/)을 적용한다.

## [Unreleased]

### 추가

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

- `internal/agent/samsung/config.go`: NASA 에이전트 `device_addresses` 설정을 선택 사항으로 변경 (기존: 필수)
- `internal/agent/samsung/agent.go`: processAddDevice/processRemoveDevice에서 req.Params 폴백 읽기 추가
- `web/src/config/agentSchemas.ts`: samsung-nasa 에이전트 스키마에서 device_addresses 필드 제거
