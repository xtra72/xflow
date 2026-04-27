---
id: SPEC-DEV-001
type: plan
version: "1.0.0"
created: "2026-03-31"
author: xtra
---

# SPEC-DEV-001: 구현 계획

## 마일스톤 개요

| 마일스톤 | 설명 | 우선순위 | 의존성 |
|---------|------|---------|--------|
| M1 | 백엔드 - DeviceMetadata 이름 필드 및 API 확장 | Primary Goal | 없음 |
| M2 | 프론트엔드 - 편집 모드 UI 및 이름 관리 | Primary Goal | M1 완료 |

---

## M1: 백엔드 - DeviceMetadata 이름 필드 및 API 확장

### M1-1: DeviceMetadata 구조체 확장

**목표**: DeviceMetadata에 Name 필드를 추가하여 사용자 정의 이름을 메타데이터로 관리

**수정 파일**:
- `internal/device/device.go` - `DeviceMetadata` 구조체에 `Name string` 필드 추가

**작업 내용**:
1. `DeviceMetadata` 구조체 첫 번째 필드로 `Name string \`json:"name"\`` 추가
2. 기존 `Device.Name()` 인터페이스 메서드와의 관계 정리 - 메타데이터 이름 우선 로직

**리스크**: 기존 메타데이터 직렬화/역직렬화에 영향. 빈 문자열이 기본값이므로 하위 호환성 유지됨.

### M1-2: add_device 커맨드 name 파라미터 추가

**목표**: 모든 프로토콜 에이전트의 add_device 커맨드에서 name 파라미터를 수용

**수정 파일**:
- `internal/agent/samsung/agent.go` - NASA add_device 핸들러에 name 파싱 추가
- `internal/agent/lg/` - LGAP add_device 핸들러에 name 파싱 추가
- LGCP 에이전트 해당 파일 - add_device 핸들러에 name 파싱 추가

**작업 내용**:
1. 각 에이전트의 `add_device` 커맨드 핸들러에서 `params["name"]` 파싱
2. 파싱된 이름을 `DeviceEntry.Name`에 설정
3. 디바이스 초기 `DeviceMetadata.Name`에도 동일 값 설정
4. name 파라미터 미지정 시 빈 문자열 유지 (기존 동작 변경 없음)

**의존성**: M1-1 (DeviceMetadata.Name 필드 존재)

### M1-3: 메타데이터 API 확장

**목표**: metadata API에서 name 필드를 읽고 쓸 수 있도록 확장

**수정 파일**:
- `internal/api/dto/request.go` - `DeviceMetadataUpdateRequest`에 Name 필드 추가
- `internal/api/handler/` - 디바이스 핸들러에서 name 필드 처리
- `internal/api/service/` - 디바이스 서비스 어댑터에서 name 업데이트 로직

**작업 내용**:
1. DTO 요청/응답에 `Name` 필드 추가
2. `PUT /api/devices/{id}/metadata` 핸들러에서 name 필드 처리
3. 메타데이터 업데이트 시 `DeviceEntry.Name` 동기화 로직 추가
4. `GET /api/devices`, `GET /api/devices/{id}` 응답에 `metadata.name` 포함 확인

**의존성**: M1-1

### M1-4: 이름 우선순위 로직

**목표**: 디바이스 이름 표시 시 메타데이터 이름 우선, DeviceEntry.Name 폴백

**수정 파일**:
- 각 프로토콜 에이전트의 Device 구현체 (NASA, LGAP, LGCP)
- `Device.Name()` 메서드 또는 API 응답 생성 로직

**작업 내용**:
1. `Device.Name()` 반환 시 `metadata.Name`이 비어있지 않으면 우선 반환
2. 비어있으면 `DeviceEntry.Name` 폴백
3. API 응답의 `name` 필드가 이 우선순위를 반영하도록 확인

**의존성**: M1-1, M1-2

---

## M2: 프론트엔드 - 편집 모드 UI 및 이름 관리

### M2-1: 타입 정의 업데이트

**목표**: TypeScript 타입에 name 필드 반영

**수정 파일**:
- `web/src/types/device.ts` - `DeviceMetadata`, `DeviceMetadataUpdateRequest`에 `name` 추가

**작업 내용**:
1. `DeviceMetadata` 인터페이스에 `name: string` 추가
2. `DeviceMetadataUpdateRequest`에 `name?: string` 추가

**의존성**: M1 완료 (백엔드 API가 name 필드를 지원해야 함)

### M2-2: Agent 페이지 디바이스 등록 모달 개선

**목표**: 디바이스 등록 시 이름을 입력할 수 있도록 모달 UI 확장

**수정 파일**:
- `web/src/pages/agents/AgentDetailPanel.tsx` - NASA/LGAP/LGCP 디바이스 등록 폼
- `web/src/pages/devices/AddDeviceDialog.tsx` - 디바이스 추가 다이얼로그 (있는 경우)

**작업 내용**:
1. NASA 디바이스 등록 폼에 "이름" 입력 필드 추가 (address, device_id, device_type 아래)
2. LGAP 디바이스 등록 폼에 "이름" 입력 필드 추가 (zone 아래)
3. LGCP 디바이스 등록 폼에 "이름" 입력 필드 추가
4. `add_device` 커맨드 params에 `name` 파라미터 포함
5. 이름 필드는 선택적 - 비어있으면 전송하지 않음

**의존성**: M1-2, M2-1

### M2-3: 디바이스 목록 행별 편집 버튼

**목표**: 디바이스 목록의 각 행에 편집 버튼을 추가하여 빠른 편집 모드 진입

**수정 파일**:
- `web/src/pages/devices/DeviceListPage.tsx` - 테이블 열에 편집 버튼 추가

**작업 내용**:
1. 디바이스 목록 테이블에 "액션" 열 추가 (또는 기존 열에 편집 아이콘 추가)
2. 편집 아이콘 버튼 (연필 아이콘) 렌더링
3. 클릭 시 해당 행의 DeviceDetailPanel을 편집 모드로 열기
4. 편집 버튼과 기존 행 클릭(상세 보기)은 구분 - 편집 버튼은 편집 모드, 행 클릭은 읽기 모드

**의존성**: M2-4 (편집 모드 구현)

### M2-4: DeviceDetailPanel 통합 편집 모드

**목표**: MetadataSection의 개별 편집 버튼을 제거하고 통합 편집 모드 구현

**수정 파일**:
- `web/src/pages/devices/DeviceDetailPanel.tsx` - 편집 모드 상태 관리 및 UI

**작업 내용**:
1. `isEditMode` 로컬 상태 추가
2. 편집 모드 진입 시 모든 메타데이터 필드를 편집 가능한 폼으로 전환:
   - 이름: 텍스트 입력
   - 위치: 텍스트 입력
   - 그룹: 텍스트 입력
   - 태그: 태그 입력 (추가/삭제)
   - 라벨: 키-값 입력 (추가/삭제)
   - Pinned: 토글 스위치
3. MetadataSection 기존 개별 편집 버튼 제거
4. 편집 모드 상단에 저장/취소 버튼 배치
5. 저장 시 `PUT /api/devices/{id}/metadata` 호출
6. 취소 시 원래 값으로 복원하고 읽기 모드로 전환
7. 비편집 모드에서 모든 필드는 읽기 전용 표시

**의존성**: M2-1

### M2-5: Pinned 토글 편집 모드 전용화

**목표**: Pinned 토글을 편집 모드에서만 변경 가능하도록 제한

**수정 파일**:
- `web/src/pages/devices/DeviceDetailPanel.tsx` - Pinned 토글 조건부 렌더링

**작업 내용**:
1. 비편집 모드: Pinned 상태를 텍스트 또는 비활성 배지로 표시 (클릭 불가)
2. 편집 모드: 기존 토글 스위치 렌더링 (클릭 가능)
3. 현재 Pinned 토글이 편집 모드 외부에서 동작하는 코드 제거

**의존성**: M2-4 (통합 편집 모드 구현)

### M2-6: 디바이스 이름 표시 폴백 로직

**목표**: 디바이스 이름 표시 시 일관된 폴백 체계 적용

**수정 파일**:
- `web/src/pages/devices/DeviceListPage.tsx` - 목록에서 이름 표시
- `web/src/pages/devices/DeviceDetailPanel.tsx` - 상세에서 이름 표시
- `web/src/lib/utils/deviceLabels.ts` - 유틸 함수 (필요 시)

**작업 내용**:
1. 이름 표시 유틸 함수 작성: `getDeviceDisplayName(device: DeviceInfo): string`
   - `metadata.name`이 비어있지 않으면 반환
   - 그렇지 않으면 `device.name` 반환
   - 둘 다 비어있으면 `device.id` 반환
2. 디바이스 목록과 상세 패널에서 해당 유틸 함수 사용

**의존성**: M2-1

---

## 기술적 접근 방식

### 백엔드 아키텍처

- **최소 침습적 변경**: 기존 `DeviceMetadata` 구조체에 필드 추가만으로 구현
- **하위 호환성**: `name` 필드의 기본값은 빈 문자열이므로 기존 데이터에 영향 없음
- **이름 우선순위**: 메타데이터 이름 > 설정 이름 > ID 순서로 일관된 폴백

### 프론트엔드 아키텍처

- **단일 편집 모드**: `isEditMode` 상태로 전체 메타데이터 편집을 통합
- **낙관적 업데이트**: 저장 시 UI를 먼저 업데이트하고 API 호출 - 실패 시 롤백
- **제어 독립성**: 디바이스 제어 커맨드는 편집 모드와 무관하게 항상 동작

### 리스크 및 대응

| 리스크 | 영향도 | 대응 |
|--------|--------|------|
| 기존 메타데이터 역직렬화 호환성 | 낮음 | Go의 JSON 언마샬은 새 필드를 무시 (zero value) |
| 이름 동기화 복잡도 (Entry vs Metadata) | 중간 | Metadata.Name을 단일 진실 소스로 지정 |
| 편집 모드 중 실시간 데이터 충돌 | 낮음 | 편집 모드에서 메타데이터 자동 갱신 비활성화 |
| Pinned 토글 UX 변경에 대한 사용자 혼란 | 낮음 | 편집 모드 진입 안내 UI 제공 |
