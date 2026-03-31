---
id: SPEC-DEV-001
version: "1.0.0"
status: completed
created: "2026-03-31"
author: xtra
priority: high
tags: device, ui, metadata, name, edit-mode
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-03-31 | xtra | 초기 SPEC 작성 |

# SPEC-DEV-001: 디바이스 이름 관리 및 편집 모드 UI 개선

## 개요

디바이스에 사용자 정의 이름을 부여하고 편집할 수 있는 기능을 제공한다. Agent 페이지에서 디바이스 등록 시 이름을 설정하고, Device 페이지에서 이름을 변경할 수 있도록 한다. 또한 디바이스 목록에 행별 편집 버튼을 추가하고, 기존 MetadataSection의 개별 편집 버튼을 제거하여 단일 편집 모드(이름, 위치, 그룹, 태그, 라벨, 고정 여부)로 통합한다. Pinned 토글은 편집 모드에서만 변경 가능하도록 제한한다.

## 범위

### 포함

- 백엔드: `DeviceMetadata`에 `Name` 필드 추가 및 메타데이터 API 확장
- 백엔드: `add_device` 커맨드에 `name` 파라미터 추가 (NASA, LGAP, LGCP 공통)
- 프론트엔드: Agent 페이지 디바이스 등록 모달에 이름 입력 필드 추가
- 프론트엔드: Device 목록 행별 편집 버튼 추가
- 프론트엔드: DeviceDetailPanel 통합 편집 모드 구현
- 프론트엔드: Pinned 토글 편집 모드 전용화

### 제외

- 디바이스 제어 커맨드(전원, 온도, 모드 등)는 현행 유지 - 편집 모드와 무관
- 디바이스 자동 검색(auto-discovery) 로직 변경 없음
- 에이전트 자체 이름 변경 기능 (별도 SPEC)

## 환경 (Environment)

### 기존 인프라

- **백엔드**: Go, `internal/device/device.go`의 `DeviceMetadata` 구조체 (Tags, Location, Group, Labels, Pinned)
- **백엔드**: `internal/agent/config.go`의 `DeviceEntry` 구조체 (Address, Name 필드 존재)
- **백엔드**: `PUT /api/devices/{id}/metadata` 엔드포인트 (메타데이터 업데이트)
- **백엔드**: `POST /api/agents/{id}/exec` 엔드포인트 (`add_device` 커맨드)
- **프론트엔드**: React + TypeScript, `web/src/types/device.ts`의 `DeviceMetadata`/`DeviceInfo` 인터페이스
- **프론트엔드**: `DeviceListPage.tsx` (디바이스 목록), `DeviceDetailPanel.tsx` (디바이스 상세)
- **프론트엔드**: `AgentDetailPanel.tsx` (에이전트 상세 - 디바이스 등록 모달 포함)
- **프로토콜**: Samsung NASA, LG LGAP, LG LGCP 에이전트

### 관련 SPEC

- SPEC-DEVICE-001: 통합 디바이스 모델링 및 관리 시스템 (completed)
- SPEC-WEB-001: 웹 UI 기능 (ongoing)
- SPEC-LGAP-001: LGAP 프로토콜 에이전트
- SPEC-LGCP-001: LGCP 프로토콜 에이전트

## 가정 (Assumptions)

- A1: `DeviceMetadata`에 `Name` 필드를 추가해도 기존 메타데이터 저장소와 호환된다
- A2: `add_device` 커맨드의 `name` 파라미터는 선택적(optional)이며, 미지정 시 빈 문자열로 저장된다
- A3: 디바이스 이름은 `DeviceEntry.Name`(설정 파일 기반)과 `DeviceMetadata.Name`(런타임 수정) 두 곳에서 관리되며, 메타데이터의 이름이 우선한다
- A4: 편집 모드 진입/종료는 프론트엔드 로컬 상태로 관리하며 별도 API 없음
- A5: 디바이스 제어 커맨드는 편집 모드와 독립적으로 항상 동작한다

## 요구사항 (Requirements)

### 유비쿼터스 요구사항 (Ubiquitous)

- **REQ-U01**: 시스템은 **항상** 디바이스 이름을 `DeviceMetadata`의 일부로 저장하고 반환해야 한다
- **REQ-U02**: 시스템은 **항상** 디바이스 목록 및 상세 API 응답에 이름 필드를 포함해야 한다
- **REQ-U03**: 시스템은 **항상** 편집 모드 외부에서 Pinned 토글 변경을 차단해야 한다

### 이벤트 기반 요구사항 (Event-Driven)

- **REQ-E01**: **WHEN** 사용자가 Agent 페이지에서 디바이스를 등록할 때, **THEN** 시스템은 이름 입력 필드를 제공하고 `add_device` 커맨드에 이름을 포함하여 전송해야 한다
- **REQ-E02**: **WHEN** 사용자가 디바이스 목록에서 편집 버튼을 클릭할 때, **THEN** 시스템은 해당 디바이스의 DeviceDetailPanel을 편집 모드로 열어야 한다
- **REQ-E03**: **WHEN** 사용자가 편집 모드에서 저장 버튼을 클릭할 때, **THEN** 시스템은 변경된 메타데이터(이름, 위치, 그룹, 태그, 라벨, 고정 여부)를 `PUT /api/devices/{id}/metadata`로 전송해야 한다
- **REQ-E04**: **WHEN** 사용자가 편집 모드에서 취소 버튼을 클릭할 때, **THEN** 시스템은 변경 사항을 폐기하고 읽기 전용 모드로 돌아가야 한다
- **REQ-E05**: **WHEN** `add_device` 커맨드에 `name` 파라미터가 포함되면, **THEN** 시스템은 해당 이름을 디바이스 메타데이터에 저장해야 한다

### 상태 기반 요구사항 (State-Driven)

- **REQ-S01**: **IF** 디바이스가 편집 모드 상태이면, **THEN** 이름, 위치, 그룹, 태그, 라벨, Pinned 필드가 모두 편집 가능해야 한다
- **REQ-S02**: **IF** 디바이스가 읽기 전용(비편집) 모드이면, **THEN** Pinned 토글을 포함한 모든 메타데이터 필드는 읽기 전용으로 표시되어야 한다
- **REQ-S03**: **IF** 디바이스 메타데이터에 이름이 설정되어 있으면, **THEN** 디바이스 목록과 상세 패널에서 해당 이름을 표시해야 한다
- **REQ-S04**: **IF** 디바이스 메타데이터에 이름이 비어 있으면, **THEN** `DeviceEntry.Name` 또는 디바이스 ID를 폴백으로 표시해야 한다

### 금지 요구사항 (Unwanted)

- **REQ-N01**: 시스템은 편집 모드 외부에서 메타데이터 수정 UI를 표시**하지 않아야 한다**
- **REQ-N02**: 시스템은 편집 모드 진입/종료 시 디바이스 제어 커맨드의 동작을 변경**하지 않아야 한다**
- **REQ-N03**: 시스템은 이름 필드에 빈 문자열 이외의 기본값을 자동 생성**하지 않아야 한다**

## 사양 (Specifications)

### M1: 백엔드 - DeviceMetadata 이름 필드 및 API 확장

#### M1-1: DeviceMetadata 구조체 확장

- `internal/device/device.go`의 `DeviceMetadata`에 `Name string` 필드 추가
  ```go
  type DeviceMetadata struct {
      Name     string            `json:"name"`
      Tags     []string          `json:"tags"`
      Location string            `json:"location"`
      Group    string            `json:"group"`
      Labels   map[string]string `json:"labels"`
      Pinned   *bool             `json:"pinned,omitempty"`
  }
  ```

#### M1-2: add_device 커맨드 name 파라미터

- Samsung NASA agent (`internal/agent/samsung/agent.go`): `add_device` 커맨드 params에 `name` 필드 파싱 추가
- LGAP agent (`internal/agent/lg/`): `add_device` 커맨드 params에 `name` 필드 파싱 추가
- LGCP agent: `add_device` 커맨드 params에 `name` 필드 파싱 추가
- 파싱된 `name`은 `DeviceEntry.Name`과 `DeviceMetadata.Name` 양쪽에 설정

#### M1-3: 메타데이터 API 응답 확장

- `PUT /api/devices/{id}/metadata` 요청 바디에 `name` 필드 허용
- `GET /api/devices`, `GET /api/devices/{id}` 응답에 `metadata.name` 포함 확인
- 이름 업데이트 시 `DeviceEntry.Name`도 동기화

#### M1-4: 이름 우선순위 로직

- `Device.Name()` 메서드에서 `DeviceMetadata.Name`이 비어있지 않으면 우선 반환
- 비어있으면 `DeviceEntry.Name` 폴백
- 둘 다 비어있으면 빈 문자열 반환 (프론트엔드에서 ID 폴백 처리)

### M2: 프론트엔드 - 편집 모드 UI 및 이름 관리

#### M2-1: 타입 정의 업데이트

- `web/src/types/device.ts`의 `DeviceMetadata`에 `name` 필드 추가
- `DeviceMetadataUpdateRequest`에 `name` 필드 추가

#### M2-2: Agent 페이지 디바이스 등록 모달

- `AgentDetailPanel.tsx`의 NASA/LGAP/LGCP 디바이스 등록 폼에 이름 입력 필드 추가
- `add_device` 커맨드 params에 `name` 포함하여 전송

#### M2-3: 디바이스 목록 편집 버튼

- `DeviceListPage.tsx`의 각 행에 편집 아이콘 버튼 추가
- 편집 버튼 클릭 시 해당 행을 펼치고 DeviceDetailPanel을 편집 모드로 진입

#### M2-4: DeviceDetailPanel 통합 편집 모드

- MetadataSection의 기존 개별 편집 버튼 제거
- 통합 편집 모드 진입 시 모든 메타데이터 필드(이름, 위치, 그룹, 태그, 라벨, Pinned)가 편집 가능
- 편집 모드: 저장/취소 버튼 표시
- 비편집 모드: 모든 필드 읽기 전용 (Pinned 토글 포함)

#### M2-5: Pinned 토글 편집 모드 전용화

- 현재 항상 변경 가능한 Pinned 토글을 편집 모드에서만 변경 가능하도록 제한
- 비편집 모드에서는 Pinned 상태를 읽기 전용으로 표시

#### M2-6: 디바이스 이름 표시 로직

- 디바이스 목록과 상세 패널에서 `metadata.name` 우선 표시
- `metadata.name`이 비어있으면 기존 `name` 필드 사용
- 둘 다 비어있으면 디바이스 ID 표시

## 우선순위 매트릭스

| 우선순위 | 항목 | 근거 |
|---------|------|------|
| Primary Goal | M1: 백엔드 DeviceMetadata 이름 필드 및 API | 프론트엔드의 전제 조건 |
| Primary Goal | M2-1: 타입 정의 업데이트 | 프론트엔드 작업의 기반 |
| Primary Goal | M2-4: 통합 편집 모드 | 핵심 UX 개선 |
| Secondary Goal | M2-2: Agent 등록 시 이름 입력 | 디바이스 등록 개선 |
| Secondary Goal | M2-3: 목록 편집 버튼 | UX 접근성 개선 |
| Secondary Goal | M2-5: Pinned 토글 제한 | 데이터 보호 |
| Final Goal | M2-6: 이름 표시 폴백 로직 | 사용자 경험 완성 |

## 트레이서빌리티

| 요구사항 | 사양 | 수용 기준 |
|---------|------|----------|
| REQ-U01 | M1-1, M1-3 | AC-01 |
| REQ-U02 | M1-3, M2-6 | AC-02 |
| REQ-U03 | M2-5 | AC-03 |
| REQ-E01 | M1-2, M2-2 | AC-04 |
| REQ-E02 | M2-3, M2-4 | AC-05 |
| REQ-E03 | M2-4, M1-3 | AC-06 |
| REQ-E04 | M2-4 | AC-07 |
| REQ-E05 | M1-2 | AC-08 |
| REQ-S01 | M2-4 | AC-09 |
| REQ-S02 | M2-4, M2-5 | AC-10 |
| REQ-S03 | M2-6 | AC-11 |
| REQ-S04 | M1-4, M2-6 | AC-12 |
| REQ-N01 | M2-4 | AC-13 |
| REQ-N02 | M2-4 | AC-14 |
| REQ-N03 | M1-2 | AC-15 |
