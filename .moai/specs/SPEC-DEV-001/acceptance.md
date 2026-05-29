---
id: SPEC-DEV-001
type: acceptance
version: "1.0.0"
created: "2026-03-31"
author: xtra
---

# SPEC-DEV-001: 수용 기준

## 테스트 시나리오

### AC-01: DeviceMetadata에 이름 저장 및 반환 (REQ-U01)

```gherkin
Scenario: 디바이스 메타데이터에 이름이 포함되어 저장/반환된다
  Given 디바이스 "nasa-agent:200001"이 등록되어 있다
  When PUT /api/devices/nasa-agent:200001/metadata 요청에 {"name": "거실 에어컨"} 을 전송한다
  Then 응답 상태 코드는 200이다
  And 응답 body의 metadata.name은 "거실 에어컨"이다

Scenario: 디바이스 목록 API에 이름이 포함된다
  Given 디바이스 "nasa-agent:200001"의 metadata.name이 "거실 에어컨"으로 설정되어 있다
  When GET /api/devices 를 요청한다
  Then 응답의 디바이스 목록에 해당 디바이스의 metadata.name이 "거실 에어컨"으로 포함된다
```

### AC-02: API 응답에 이름 필드 포함 (REQ-U02)

```gherkin
Scenario: 디바이스 상세 API 응답에 이름 포함
  Given 디바이스 "nasa-agent:200001"이 이름 "거실 에어컨"으로 등록되어 있다
  When GET /api/devices/nasa-agent:200001 을 요청한다
  Then 응답 body에 metadata.name 필드가 존재한다
  And metadata.name 값은 "거실 에어컨"이다
```

### AC-03: 편집 모드 외부에서 Pinned 토글 차단 (REQ-U03)

```gherkin
Scenario: 비편집 모드에서 Pinned 토글 비활성화
  Given DeviceDetailPanel이 읽기 전용 모드로 열려있다
  When Pinned 영역을 확인한다
  Then Pinned 토글은 비활성(disabled) 상태이거나 읽기 전용 배지로 표시된다
  And Pinned 값을 클릭해도 변경되지 않는다

Scenario: 편집 모드에서 Pinned 토글 활성화
  Given DeviceDetailPanel이 편집 모드로 열려있다
  When Pinned 토글을 클릭한다
  Then Pinned 값이 토글된다
```

### AC-04: Agent 페이지 디바이스 등록 시 이름 입력 (REQ-E01)

```gherkin
Scenario: NASA 디바이스 등록 시 이름 입력 필드 제공
  Given Agent 상세 페이지에서 NASA 에이전트를 보고 있다
  When 디바이스 추가 버튼을 클릭한다
  Then 등록 폼에 "이름" 입력 필드가 표시된다
  And 이름 필드는 선택적(비필수)이다

Scenario: 이름을 포함하여 NASA 디바이스 등록
  Given NASA 디바이스 등록 폼이 열려있다
  When address에 "200001", device_type에 "indoor", 이름에 "거실 에어컨"을 입력한다
  And 등록 버튼을 클릭한다
  Then add_device 커맨드가 params에 {"address": "200001", "device_type": "indoor", "name": "거실 에어컨"}을 포함하여 전송된다

Scenario: LGAP 디바이스 등록 시 이름 입력
  Given LGAP 에이전트의 디바이스 등록 폼이 열려있다
  When zone에 "0x10", 이름에 "회의실 에어컨"을 입력하고 등록한다
  Then add_device 커맨드 params에 name이 "회의실 에어컨"으로 포함된다

Scenario: 이름 없이 디바이스 등록
  Given 디바이스 등록 폼이 열려있다
  When 이름 필드를 비워두고 등록한다
  Then add_device 커맨드가 정상 전송된다
  And 디바이스 이름은 빈 문자열로 저장된다
```

### AC-05: 디바이스 목록 편집 버튼 클릭 (REQ-E02)

```gherkin
Scenario: 디바이스 목록 행에 편집 버튼 표시
  Given 디바이스 목록 페이지가 로드되었다
  When 디바이스 목록을 확인한다
  Then 각 행에 편집 아이콘 버튼이 표시된다

Scenario: 편집 버튼 클릭 시 편집 모드 진입
  Given 디바이스 목록에 "거실 에어컨" 디바이스가 있다
  When 해당 행의 편집 버튼을 클릭한다
  Then DeviceDetailPanel이 편집 모드로 열린다
  And 메타데이터 필드들이 편집 가능한 상태이다

Scenario: 행 클릭과 편집 버튼 클릭 구분
  Given 디바이스 목록이 표시되어 있다
  When 디바이스 행(편집 버튼 외 영역)을 클릭한다
  Then DeviceDetailPanel이 읽기 전용 모드로 열린다
```

### AC-06: 편집 모드에서 저장 (REQ-E03)

```gherkin
Scenario: 메타데이터 변경 후 저장
  Given DeviceDetailPanel이 편집 모드로 열려있다
  And 디바이스 ID가 "nasa-agent:200001"이다
  When 이름을 "거실 에어컨"에서 "안방 에어컨"으로 변경한다
  And 저장 버튼을 클릭한다
  Then PUT /api/devices/nasa-agent:200001/metadata 요청이 {"name": "안방 에어컨"} 을 포함하여 전송된다
  And 저장 성공 후 읽기 전용 모드로 전환된다
  And 변경된 이름 "안방 에어컨"이 표시된다

Scenario: 여러 필드 동시 변경 후 저장
  Given DeviceDetailPanel이 편집 모드이다
  When 이름을 "안방 에어컨", 위치를 "2층 안방", 그룹을 "HVAC"으로 변경한다
  And 태그에 "bedroom"을 추가한다
  And 저장 버튼을 클릭한다
  Then 모든 변경 사항이 단일 PUT 요청으로 전송된다
```

### AC-07: 편집 모드에서 취소 (REQ-E04)

```gherkin
Scenario: 변경 사항 취소
  Given DeviceDetailPanel이 편집 모드이다
  And 이름을 "거실 에어컨"에서 "안방 에어컨"으로 변경했다
  When 취소 버튼을 클릭한다
  Then 이름이 원래 값 "거실 에어컨"으로 복원된다
  And 읽기 전용 모드로 전환된다
```

### AC-08: add_device에 name 파라미터 처리 (REQ-E05)

```gherkin
Scenario: 백엔드에서 add_device name 파라미터 저장
  Given NASA 에이전트 "nasa-agent"가 실행 중이다
  When POST /api/agents/nasa-agent/exec 에 {"command": "add_device", "params": {"address": "200001", "name": "거실"}} 을 전송한다
  Then 디바이스가 성공적으로 등록된다
  And 해당 디바이스의 metadata.name이 "거실"로 설정된다
```

### AC-09: 편집 모드에서 모든 필드 편집 가능 (REQ-S01)

```gherkin
Scenario: 편집 모드 진입 시 모든 메타데이터 필드 편집 가능
  Given DeviceDetailPanel이 편집 모드이다
  Then 다음 필드들이 편집 가능하다:
    | 필드 | UI 요소 |
    | 이름 | 텍스트 입력 |
    | 위치 | 텍스트 입력 |
    | 그룹 | 텍스트 입력 |
    | 태그 | 태그 입력 (추가/삭제) |
    | 라벨 | 키-값 입력 (추가/삭제) |
    | Pinned | 토글 스위치 |
```

### AC-10: 비편집 모드에서 모든 필드 읽기 전용 (REQ-S02)

```gherkin
Scenario: 읽기 전용 모드에서 모든 필드 비활성화
  Given DeviceDetailPanel이 읽기 전용 모드이다
  Then 다음 필드들은 편집할 수 없다:
    | 필드 | 상태 |
    | 이름 | 텍스트로 표시 |
    | 위치 | 텍스트로 표시 |
    | 그룹 | 텍스트로 표시 |
    | 태그 | 배지로 표시 |
    | 라벨 | 키=값 텍스트로 표시 |
    | Pinned | 읽기 전용 배지 |
  And MetadataSection에 개별 편집 버튼이 존재하지 않는다
```

### AC-11: 메타데이터 이름이 있으면 우선 표시 (REQ-S03)

```gherkin
Scenario: 메타데이터 이름이 설정된 디바이스 표시
  Given 디바이스의 metadata.name이 "거실 에어컨"이다
  When 디바이스 목록을 확인한다
  Then 이름 열에 "거실 에어컨"이 표시된다

  When DeviceDetailPanel을 열면
  Then 이름 영역에 "거실 에어컨"이 표시된다
```

### AC-12: 이름 폴백 로직 (REQ-S04)

```gherkin
Scenario: 메타데이터 이름이 비어있으면 DeviceEntry.Name 폴백
  Given 디바이스의 metadata.name이 빈 문자열이다
  And DeviceEntry.Name이 "NASA Indoor Unit"이다
  When 디바이스 목록을 확인한다
  Then 이름 열에 "NASA Indoor Unit"이 표시된다

Scenario: 둘 다 비어있으면 디바이스 ID 표시
  Given 디바이스의 metadata.name이 빈 문자열이다
  And DeviceEntry.Name이 빈 문자열이다
  And 디바이스 ID가 "nasa-agent:200001"이다
  When 디바이스 목록을 확인한다
  Then 이름 열에 "nasa-agent:200001"이 표시된다
```

### AC-13: 편집 모드 외부에서 메타데이터 수정 UI 미표시 (REQ-N01)

```gherkin
Scenario: 읽기 전용 모드에서 편집 관련 UI 숨김
  Given DeviceDetailPanel이 읽기 전용 모드이다
  When MetadataSection을 확인한다
  Then 개별 편집 버튼이 존재하지 않는다
  And 인라인 편집 UI가 표시되지 않는다
```

### AC-14: 편집 모드가 디바이스 제어에 영향 없음 (REQ-N02)

```gherkin
Scenario: 편집 모드에서도 디바이스 제어 가능
  Given DeviceDetailPanel이 편집 모드이다
  And Commands 섹션이 표시되어 있다
  When 전원 on 커맨드를 실행한다
  Then 커맨드가 정상적으로 전송되고 실행된다
  And 편집 모드 상태는 변경되지 않는다

Scenario: 비편집 모드에서도 디바이스 제어 가능
  Given DeviceDetailPanel이 읽기 전용 모드이다
  When 온도 설정 커맨드를 실행한다
  Then 커맨드가 정상적으로 전송되고 실행된다
```

### AC-15: 이름 자동 생성 금지 (REQ-N03)

```gherkin
Scenario: 이름 미지정 시 빈 문자열 유지
  Given 디바이스 등록 폼에서 이름을 입력하지 않았다
  When 디바이스를 등록한다
  Then 디바이스의 metadata.name은 빈 문자열("")이다
  And "Device 1"이나 "Unnamed Device" 같은 자동 이름이 생성되지 않는다
```

---

## 엣지 케이스

### EC-01: 특수문자가 포함된 이름

```gherkin
Scenario: 특수문자가 포함된 디바이스 이름
  Given 디바이스 편집 모드이다
  When 이름에 "에어컨 (1층/거실) #001"을 입력하고 저장한다
  Then 이름이 정상적으로 저장되고 표시된다
```

### EC-02: 매우 긴 이름

```gherkin
Scenario: 긴 디바이스 이름
  Given 디바이스 편집 모드이다
  When 이름에 200자 이상의 문자열을 입력한다
  Then UI에서 적절히 잘림 처리(truncation)되어 표시된다
  And 전체 이름은 호버 또는 상세 보기에서 확인 가능하다
```

### EC-03: 동시 편집 충돌

```gherkin
Scenario: 편집 중 다른 사용자가 메타데이터를 변경
  Given 사용자 A가 디바이스 편집 모드에 진입했다
  And 사용자 B가 동일 디바이스의 위치를 변경하고 저장했다
  When 사용자 A가 이름만 변경하고 저장한다
  Then 사용자 A의 변경이 성공적으로 저장된다
  And 사용자 B의 위치 변경이 사용자 A의 저장으로 덮어쓸 수 있다 (마지막 저장 우선)
```

### EC-04: 네트워크 오류 시 저장 실패

```gherkin
Scenario: 저장 중 네트워크 오류
  Given 디바이스 편집 모드에서 이름을 변경했다
  When 저장 버튼을 클릭한다
  And API 호출이 네트워크 오류로 실패한다
  Then 오류 메시지가 표시된다
  And 편집 모드가 유지된다 (변경 사항 보존)
  And 재시도할 수 있다
```

### EC-05: 빈 문자열로 이름 초기화

```gherkin
Scenario: 기존 이름을 빈 문자열로 변경
  Given 디바이스의 metadata.name이 "거실 에어컨"이다
  When 편집 모드에서 이름을 모두 지우고 저장한다
  Then metadata.name이 빈 문자열로 업데이트된다
  And 디바이스 목록에서 폴백 이름(DeviceEntry.Name 또는 ID)이 표시된다
```

---

## 품질 게이트

### 백엔드 품질 기준

- [ ] `internal/device/device.go` 변경에 대한 단위 테스트 작성
- [ ] `add_device` 커맨드 name 파라미터 테스트 (NASA, LGAP, LG HVACR-02 각각)
- [ ] `PUT /api/devices/{id}/metadata` name 필드 처리 테스트
- [ ] 이름 우선순위 로직 단위 테스트
- [ ] 기존 메타데이터 역직렬화 하위 호환성 테스트
- [ ] Go test coverage 85% 이상

### 프론트엔드 품질 기준

- [ ] DeviceDetailPanel 편집 모드 진입/종료 테스트
- [ ] Pinned 토글 편집 모드 전용 동작 테스트
- [ ] 디바이스 등록 모달 이름 필드 테스트
- [ ] 이름 표시 폴백 로직 테스트
- [ ] 편집 모드에서 디바이스 제어 독립성 테스트
- [ ] TypeScript 타입 오류 없음

### 완료 정의 (Definition of Done)

- [ ] 모든 EARS 요구사항(REQ-U01~U03, REQ-E01~E05, REQ-S01~S04, REQ-N01~N03)이 구현됨
- [ ] 모든 수용 기준(AC-01~AC-15)이 통과됨
- [ ] 백엔드 테스트 커버리지 85% 이상
- [ ] TypeScript 빌드 오류 없음
- [ ] NASA, LGAP, LG HVACR-02 모든 프로토콜에서 동일하게 동작 확인
- [ ] 디바이스 제어 커맨드가 편집 모드와 무관하게 정상 동작 확인
