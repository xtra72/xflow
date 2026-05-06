---
spec_id: SPEC-WEB-006
version: 0.1.0
status: draft
updated: 2026-05-06
---

# SPEC-WEB-006: Acceptance Criteria

## v0.1.0 Note

본 문서는 SPEC-WEB-006 v0.1.0 의 수용 기준이다. 12개 Given-When-Then 시나리오 + Edge Case
Checklist + TRUST 5 quality gates + Definition of Done 으로 구성된다.

> **Decision Point 의존성**: 시나리오 1, 4, 7, 10 은 Decision Point 1, 2, 3, 5 결정에 따라
> 검증 방식이 달라진다. 본 문서는 권장 결정값 (Decision Point 1 = 옵션 A `/admin/system`,
> Decision Point 2 = 옵션 C toast + badge, Decision Point 3 = 옵션 B+C CLI + future 노트,
> Decision Point 4 = 옵션 C 읽기 전용, Decision Point 5 = 옵션 B admin 전용) 기준으로 작성됨.
> 다른 결정값 채택 시 해당 시나리오를 적절히 조정한다.

---

## Given-When-Then 시나리오 (12개)

### Scenario 1: 정상 버전 표시 (M1, M2)

- **Given**:
  - admin 권한 운영자가 인증된 세션으로 xflow 웹 UI 에 접속
  - xflowd 백엔드는 v0.3.0 으로 운영 중이며, `update_available: false` 응답
  - Decision Point 1 권장값 (옵션 A) 채택 시 `/admin/system` 라우트 사용
- **When**:
  - 운영자가 좌측 nav 또는 결정된 위치를 통해 System Status Panel 로 이동
- **Then**:
  - 패널이 다음 정보를 표시한다:
    - 현재 버전: `v0.3.0`
    - 빌드 커밋: `6b9c531` (단축 해시 7자, GitHub commit URL 링크 가능)
    - 빌드 일시: 브라우저 로컬 형식 `2026-05-04 12:34:56` 등
    - Go 버전: `go1.22.5`
    - OS / Arch: `linux/amd64`
    - 채널: `stable`
    - 마지막 확인 시각: 현재 시각 (TanStack Query `dataUpdatedAt`)
  - "업데이트 가능" 인디케이터는 표시되지 않는다 (`update_available: false`)
  - "Check for updates" 버튼이 활성 상태로 표시된다
  - 60초 후 자동으로 폴링하여 마지막 확인 시각이 갱신된다

---

### Scenario 2: 업데이트 가능 시 인디케이터 표시 (M2, M4)

- **Given**:
  - System Status Panel 이 열려 있고 현재 버전 v0.3.0
  - 백엔드가 새로운 폴링 응답에서 `update_available: true, latest_version: "v0.4.0"` 반환
- **When**:
  - 60초 폴링이 발생하여 응답이 갱신됨 (또는 운영자가 "Check for updates" 버튼을 클릭하여
    즉시 갱신)
- **Then**:
  - 패널에 명확한 시각 인디케이터 표시 (배지 dot + "업데이트 가능" 라벨)
  - 최신 버전 `v0.4.0` 이 함께 표시됨
  - "Update available — v0.4.0 으로 업데이트" 버튼 노출 (Update Dialog trigger)
  - **Decision Point 2 옵션 C 채택 시**: toast 알림 한 번 표시 ("v0.4.0 으로 업데이트가
    가능합니다") + 헤더 dropdown 의 배지 dot 활성화
  - 운영자가 toast 를 dismiss 하면 다시 표시되지 않으나, 다음 latest_version 변경 시 다시
    표시 가능

---

### Scenario 3: Check 버튼 클릭 → 채널 폴 → 업데이트 가능 → UI 갱신 (M3)

- **Given**:
  - System Status Panel 이 열려 있고, 현재 `update_available: false` (캐시된 응답)
  - 백엔드 채널에 새 v0.4.0 이 방금 게시됨
- **When**:
  - 운영자가 "Check for updates" 버튼을 클릭
- **Then**:
  - 시스템은 즉시 `POST /api/v1/system/update/check` 을 호출
  - 버튼이 비활성화되고 "확인 중..." 진행 인디케이터 표시
  - 응답 수신 (수 초 내, 정상 네트워크 환경) 후:
    - 버튼이 다시 활성화됨
    - `update_available: true` 인디케이터 활성화 (Scenario 2 와 동일 결과)
    - 마지막 확인 시각 즉시 갱신
  - 백엔드 폴링 캐시가 갱신되어 다음 60초 폴링은 동일 결과 반환

---

### Scenario 4: Apply 흐름 해피 패스 (M5, M6, M8)

- **Given**:
  - System Status Panel 에 v0.3.0 → v0.4.0 업데이트 가능 인디케이터 표시
  - admin 운영자가 적절 권한으로 인증되어 있음
  - v0.4.0 바이너리가 stable 채널에 정상 게시됨 (서명/체크섬 정상)
- **When**:
  - 운영자가 "Update available" 버튼 클릭 → Update Dialog 열림 → Info step
  - Info step 에서 v0.3.0 → v0.4.0 비교 + 릴리즈 노트 링크 확인 → "다음" 클릭 → Confirm step
  - Confirm step (다운그레이드 아니므로 force 체크박스 없음, 단순 confirm) → "적용" 클릭 →
    Apply step
  - Apply step 에서 자동으로 `POST /api/v1/system/update/apply` 호출 → operation_id 수신 →
    Progress step
  - Progress step 에서 1초 폴링으로 9-state machine 시각화 (`starting → checking →
    downloading → verifying → applying → ready_to_restart`)
  - `ready_to_restart` 도달 시 자동으로 Result step 으로 전환
- **Then**:
  - 각 상태가 순차적으로 강조되고, 완료된 상태는 체크 표시 + 무채색
  - downloading 단계에서 progress bar 가 0% → 100% 진행
  - Result step 표시:
    - "업데이트가 디스크에 적용되었습니다" 성공 메시지
    - 운영자 가이드 (Decision Point 3 옵션 B + C 권장):
      - "재시작이 필요합니다"
      - OS 별 CLI 명령어 표시 (Linux: `systemctl restart xflowd`, macOS: `launchctl
        kickstart -k system/xflowd`)
      - 클립보드 복사 버튼
      - "Future: SPEC-UPDATE-002 에서 in-process restart 자동화 예정" 노트
    - 백업 파일 위치 안내: `<binary>.previous` 가 보존되어 있음
    - "닫기" 버튼

---

### Scenario 5: Apply 흐름 실패 (signature invalid → rollback) (M7, M12)

- **Given**:
  - Apply 흐름이 진행 중이며 Progress step 에서 `verifying` 단계 도달
  - 백엔드가 Ed25519 서명 검증에 실패하여 `status: "failed"`, `error: "ErrUpdateSignatureInvalid"`
    응답
- **When**:
  - 1초 폴링에서 status `failed` 감지
- **Then**:
  - 자동으로 Result step 으로 전환
  - 다음을 표시:
    - critical 등급 에러 배너 (붉은 톤)
    - 매핑된 사용자 친화 메시지: "디지털 서명 검증에 실패했습니다. 잠재적 보안 침해 신호이므로
      운영팀에 즉시 알려주세요."
    - 펼침 영역에 원본 메시지 (`ErrUpdateSignatureInvalid`) + 발생 시각
    - **명시적 "Rollback 시도" 버튼** (백엔드 v0.1.0 한계 보완)
    - 백업 파일 위치 안내: `<binary>.previous`
    - "닫기" 버튼
  - 운영자가 "Rollback 시도" 버튼 클릭 → `POST /api/v1/system/update/rollback` 호출
  - rollback 성공 응답 수신 후:
    - "롤백이 완료되었습니다. v0.3.0 으로 복구되었습니다" 성공 메시지로 갱신
    - "닫기" 버튼만 노출

---

### Scenario 6: 다운그레이드 시 force checkbox 표시 (M9)

- **Given**:
  - 현재 버전 v0.3.0 운영 중
  - 운영자가 명시적으로 v0.2.0 으로 다운그레이드를 시도 (예: 채널에서 이전 버전 선택, 또는 버전
    수동 입력)
- **When**:
  - Info step 에서 타겟 버전 v0.2.0 표시 → "다음" 클릭 → Confirm step
- **Then**:
  - Info step 에 명시적 다운그레이드 경고 배너:
    - "⚠ 다운그레이드 시도입니다. 데이터 손실 또는 호환성 문제가 발생할 수 있습니다."
    - 빨간색 또는 주황색 강조 톤
  - Confirm step 에 다음 추가 노출:
    - `--force` 체크박스 (라벨: "다운그레이드를 강제 적용함을 확인합니다 (`--force`)")
    - 추가 경고 텍스트
    - "적용" 버튼은 체크박스 활성 시까지 비활성 상태
  - 운영자가 체크박스 활성 → "적용" 버튼 활성화 → 클릭
  - `POST /api/v1/system/update/apply` 요청 바디에 `force: true` 포함되어 전송
  - Progress step 정상 진행 (downgrade 도 v0.3.0 → v0.2.0 으로 9-state machine 정상 흐름)
  - **만약 force 체크박스 비활성 상태에서 race condition 으로 백엔드가
    `ErrDowngradeRequiresForce` 반환** → 사용자를 Confirm step 으로 되돌리고 force 체크박스 강조

---

### Scenario 7: ready_to_restart 상태 → 운영자 안내 표시 (M8)

- **Given**:
  - Apply 흐름이 정상 진행되어 Progress step 에서 `applying → ready_to_restart` 전이
  - Decision Point 3 권장값 (옵션 B + C — CLI 명령어 + future 노트) 채택
  - 백엔드 응답의 `os` 필드 = `linux`
- **When**:
  - 1초 폴링에서 `status: "ready_to_restart"` 감지
- **Then**:
  - 자동으로 Result step 으로 전환
  - 다음을 표시:
    - "업데이트가 디스크에 적용되었습니다" 성공 메시지 (info 등급)
    - **운영자 가이드 박스**:
      - "재시작이 필요합니다" 강조 헤더
      - "다음 명령어를 실행하세요:" 안내
      - **Linux 명령어 표시**: `systemctl restart xflowd` (monospaced 폰트, 코드 블록)
      - **클립보드 복사 버튼** 우측에 노출 (클릭 시 toast "복사됨")
      - 백업 파일 안내: "이전 바이너리는 `<binary>.previous` 에 보존되어 있습니다"
    - **Future 노트**: "📅 향후 SPEC-UPDATE-002 에서 in-process restart 자동화 예정 — 현재는
      운영자 재시작이 필요합니다"
    - "닫기" 버튼
  - macOS 환경 (`os: darwin`) 의 경우 명령어가 `launchctl kickstart -k system/xflowd` 로 분기
    표시
  - 운영자가 별도 터미널에서 명령어 실행 후 다음 폴링 (60초 후 또는 수동 새로고침) 에서 Status
    Panel 의 `version` 이 v0.4.0 으로 갱신됨

---

### Scenario 8: Rollback 흐름 (성공 / 백업 없음 → 명시적 에러) (M7, M12)

#### 8a: Rollback 성공

- **Given**: Scenario 5 에서 Result step 의 "Rollback 시도" 버튼 클릭 가능 상태
- **When**: 운영자가 "Rollback 시도" 버튼 클릭
- **Then**:
  - `POST /api/v1/system/update/rollback` 호출
  - 응답: `{success: true, from_version: "v0.4.0", to_version: "v0.3.0", error: null}`
  - Result step 메시지가 갱신되어 "롤백이 완료되었습니다" 성공 표시
  - toast 알림: "롤백 성공: v0.3.0 으로 복구"
  - "닫기" 버튼만 노출

#### 8b: 백업 파일 부재 → ErrUpdateRollbackFailed

- **Given**:
  - Scenario 5 와 동일하나 `<binary>.previous` 파일이 누락 또는 손상된 상태
- **When**: 운영자가 "Rollback 시도" 버튼 클릭
- **Then**:
  - `POST /api/v1/system/update/rollback` 호출
  - 응답: `{success: false, error: {code: "ErrUpdateRollbackFailed", message: "backup file not found at ..."}}`
  - Result step 에 critical 등급 메시지 표시:
    - "롤백에 실패했습니다. 백업 파일 (`<binary>.previous`) 위치를 수동으로 확인하고 운영자
      개입이 필요합니다."
    - 원본 메시지 펼침 영역
    - 백업 파일 예상 위치 (백엔드 응답에 포함되었거나, 운영자가 직접 확인하라는 가이드)
    - "운영팀 알림" 강조 노트 (메일/webhook 등 미래 통합 안내)
  - "닫기" 버튼만 노출 (rollback 재시도 버튼은 비활성)

---

### Scenario 9: 9-state machine 시각화 (각 단계 강조) (M6)

- **Given**: Apply 흐름이 진행 중이며 Progress step 활성
- **When**: 1초 폴링에서 status 가 순차적으로 변경:
  `starting → checking → downloading → verifying → applying → ready_to_restart`
- **Then**: 각 상태별로 다음 시각 표현이 적용됨:
  - **현재 상태 (active)**: 강조색 (예: 파란색) + ring 효과 또는 pulse animation + 굵은
    텍스트
  - **완료 상태 (completed)**: 체크 표시 (✓) + 무채색 (회색) + 일반 텍스트
  - **미진행 상태 (pending)**: 흐림 톤 (`opacity-40` 등) + 텍스트만 표시
  - 각 상태 옆에 친화 설명 텍스트:
    - `starting` → "업데이트 준비 중..."
    - `checking` → "채널에서 최신 버전 조회 중..."
    - `downloading` → "새 바이너리 다운로드 중..." (+ progress_percent bar)
    - `verifying` → "서명 및 체크섬 검증 중..."
    - `applying` → "원자적 바이너리 교체 중..."
    - `ready_to_restart` → "재시작 대기 중 — 운영자 동작 필요"
    - `completed` → "업데이트 성공"
    - `failed` → "업데이트 실패" (붉은 톤)
  - 화면 크기 변화에도 시각화가 깨지지 않음 (responsive)
  - terminal state (`completed`, `failed`, `ready_to_restart`) 도달 시 폴링이 자동 중단됨

---

### Scenario 10: 권한 없는 사용자 → 403 처리 (M11, Decision Point 5)

- **Given**:
  - Decision Point 5 권장값 (옵션 B — admin 전용) 채택
  - 일반 사용자 (admin 권한 없음) 가 인증된 세션으로 접속
- **When**:
  - 일반 사용자가 직접 URL `/admin/system` 으로 접근 시도
  - 또는 이미 Status Panel 이 열려 있는 상태에서 "Check for updates" 버튼 클릭 시도
- **Then**:
  - **URL 직접 접근 시도**: 라우팅 가드가 일반 사용자를 차단 → "권한 없음" 화면 또는 메인 대시
    보드로 리다이렉트
  - **Status Panel 진입 시 (옵션 D widget 채택 시)**: "Check for updates" 버튼 비활성화 또는
    숨김 처리
  - **Apply 시도 시도 (백엔드 호출이 실패)**:
    - 백엔드가 HTTP 403 Forbidden 응답
    - UI 가 inline 에러 또는 toast 표시: "권한이 없습니다. 관리자에게 문의하세요."
  - 일반 사용자에게는 update_available 알림 (toast / badge) 도 표시되지 않음
  - 보안 침해 시도 (URL 추측 등) 는 백엔드 audit 로그에 기록됨

---

### Scenario 11: 장시간 다운로드 중 다이얼로그 닫기 → 재오픈 시 진행 상태 복원 (M5, M6)

- **Given**:
  - Apply 흐름이 진행 중이며 Progress step 의 `downloading` 단계 (progress 35%)
- **When**:
  - 운영자가 Esc 키 입력 또는 X 버튼 클릭으로 모달을 닫으려 시도
- **Then**:
  - 시스템이 confirm 다이얼로그 표시:
    - "업데이트가 진행 중입니다. 닫아도 백엔드 작업은 계속됩니다. 닫으시겠습니까?"
    - "닫기" / "취소" 버튼
  - 운영자가 "닫기" 선택 → 모달 닫힘 (백엔드 작업은 계속 진행)
  - Status Panel 의 status 폴링은 계속되나 UI 표시는 없음 (단, badge 등으로 진행 중임을 보조
    표시 가능)
- **Then (재오픈)**:
  - 잠시 후 운영자가 Status Panel 에서 "Update Dialog" 를 다시 열기 (또는 "Update available"
    버튼 클릭)
  - UpdateDialog 가 mount 시점에 `useUpdateStatus()` 를 호출하여 active operation 자동 감지
  - `status != 'idle'` 이므로 자동으로 Progress step 으로 진입 (Info, Confirm step 건너뜀)
  - 9-state machine 시각화가 현재 상태 (예: `verifying`, 60%) 로 정확히 복원됨
  - 1초 폴링이 재개되며, terminal state 도달 시 정상적으로 Result step 으로 전환

---

### Scenario 12: 동시 다른 운영자가 apply 시작 → 409 → 명시적 안내 (M12)

- **Given**:
  - Operator A 가 이미 Apply 흐름을 시작하여 백엔드에서 `status: "downloading"` 진행 중
  - Operator B 가 같은 클러스터를 모니터링하며 Status Panel 을 열고 있음
- **When**:
  - Operator B 가 "Update available" 버튼 클릭 → Update Dialog 열림 → 정상적인 Confirm step
    까지 진행 → "적용" 버튼 클릭
- **Then**:
  - `POST /api/v1/system/update/apply` 호출
  - 백엔드가 HTTP 409 Conflict 응답: `{error: {code: "ErrUpdateInProgress", message: "..."}}`
  - UI 가 매핑된 메시지를 inline 또는 toast 로 표시:
    - "다른 사용자가 이미 업데이트를 진행 중입니다. status 화면에서 진행 상황을 확인하세요."
    - warning 등급 (붉지 않음, 노란/주황 톤)
  - 모달이 자동으로 Progress step 으로 전환되고 (`useUpdateStatus` 폴링 활성화), Operator A 의
    진행 상황을 같이 추적 가능
  - terminal state 도달 시 Operator B 도 정상적으로 Result step 으로 전환

---

## v0.1.0 Edge Case Checklist (~30 항목)

### A. Status Panel + 폴링 (8 항목)

- [ ] **A1**: Status Panel 첫 로드 시 로딩 스켈레톤 표시 후 정상 데이터 렌더링
- [ ] **A2**: 백엔드 5xx 응답 시 에러 배너 + "재시도" 버튼 표시 (TanStack Query retry 동작)
- [ ] **A3**: 백엔드 401 응답 시 로그인 페이지 리다이렉트 또는 권한 에러 화면
- [ ] **A4**: 60초 폴링이 다른 탭으로 전환 시 자동 일시 중지
  (`refetchIntervalInBackground: false`)
- [ ] **A5**: 현재 탭 복귀 시 즉시 폴링 재개
- [ ] **A6**: "Check for updates" 버튼 클릭 중 (mutation pending) 두 번 클릭 시 두 번째 클릭
  무시
- [ ] **A7**: build_date 가 미래 시각인 경우 (시계 동기화 문제 등) UI 가 깨지지 않음
- [ ] **A8**: latest_version 이 null 인 상태에서 update_available 인디케이터 표시 안 됨

### B. Update Dialog 다단계 흐름 (8 항목)

- [ ] **B1**: Info step 의 릴리즈 노트 링크가 새 탭으로 열림 (`target="_blank"
  rel="noopener noreferrer"`)
- [ ] **B2**: Confirm step 에서 force 체크박스 비활성 상태 → "적용" 버튼 비활성
- [ ] **B3**: Apply step 에서 백엔드 즉시 에러 응답 (예: ErrUpdateChannelInvalid) → Result step
  으로 즉시 전환
- [ ] **B4**: Progress step 에서 1초 폴링 중 백엔드 timeout (5xx) → retry 동작 후 명시적 에러
- [ ] **B5**: terminal state (completed, failed, ready_to_restart) 도달 시 폴링 자동 중단
  (TanStack Query `enabled: false`)
- [ ] **B6**: 모달 닫기 (active operation 중) confirm 다이얼로그 → "취소" 시 모달 유지
- [ ] **B7**: 모달 닫기 (active operation 종료 후, 즉 `status ∈ {idle, completed, failed,
  ready_to_restart}`) 시 confirm 없이 즉시 닫힘
- [ ] **B8**: ESC 키 입력으로 모달 닫기 시 confirm 표시 (active 인 경우)

### C. 9-state machine 시각화 (5 항목)

- [ ] **C1**: 모든 9개 상태 (idle, starting, checking, downloading, verifying, applying,
  ready_to_restart, completed, failed) 에서 시각화가 깨지지 않음
- [ ] **C2**: 모바일 화면 (375px) 에서도 9-state 시각화가 적절히 stack 또는 scroll 됨
- [ ] **C3**: progress_percent 가 null 인 경우 (downloading 외 단계) progress bar 미표시
- [ ] **C4**: 상태가 역순으로 변경되는 경우 (백엔드 race condition) UI 가 깨지지 않음
  (defensive)
- [ ] **C5**: 각 상태 설명 텍스트가 사용자의 conversation_language 기준으로 표시됨 (현재 ko)

### D. Rollback (4 항목)

- [ ] **D1**: rollback 성공 시 toast + Result step 메시지 갱신
- [ ] **D2**: ErrUpdateRollbackFailed 시 critical 등급 메시지 + 운영자 개입 안내
- [ ] **D3**: rollback 진행 중 (mutation pending) 버튼 비활성화 + 진행 인디케이터
- [ ] **D4**: rollback 후 Status Panel 다음 폴링에서 version 이 이전 버전으로 정상 표시

### E. 다운그레이드 force flag (3 항목)

- [ ] **E1**: semver 비교 정확성 (v0.3.0 vs v0.2.0, v1.0.0-rc.1 vs v1.0.0 등 prerelease 케이스)
- [ ] **E2**: force 체크박스 활성 후 다시 비활성으로 toggle → "적용" 버튼 다시 비활성
- [ ] **E3**: 다운그레이드가 아닌 일반 업데이트 (v0.3.0 → v0.4.0) 에서 force 체크박스 미표시

### F. 권한 / 인증 (3 항목)

- [ ] **F1**: 일반 사용자가 `/admin/system` URL 직접 입력 시 차단 (라우팅 가드)
- [ ] **F2**: 백엔드 403 응답 시 inline 에러 + 명시적 안내
- [ ] **F3**: 클라이언트 권한 검사 우회 시도 (예: devtools 로 버튼 강제 활성화) 시 백엔드 게이트
  가 차단

### G. 에러 매핑 (4 항목)

- [ ] **G1**: 8종 에러 식별자 (ErrUpdateChecksumMismatch 등) 모두 정확히 매핑됨
- [ ] **G2**: 매핑되지 않은 에러는 원본 메시지 + GitHub Issues 링크 fallback 표시
- [ ] **G3**: 에러 severity 에 따라 시각 톤 차별화 (info: 회색, warning: 노란, critical: 붉은)
- [ ] **G4**: 원본 메시지가 펼침 영역 또는 콘솔에 보조 노출됨 (디버깅 용이성)

### H. 알림 (Decision Point 2, 3 항목)

- [ ] **H1**: update_available false → true 전이 시 toast 한 번만 표시
- [ ] **H2**: toast dismiss 후 같은 latest_version 에 대해 다시 표시되지 않음
- [ ] **H3**: 새로운 latest_version 감지 시 toast 다시 표시 가능

---

## TRUST 5 Quality Gates

### T - Tested (테스트)

- [ ] **신규 컴포넌트 라인 커버리지 85% 이상**:
  - `web/src/services/api/systemUpdate.ts` ≥ 90%
  - `web/src/lib/errors/updaterErrorMapper.ts` = 100%
  - `web/src/components/system/SystemStatusPanel.tsx` ≥ 85%
  - `web/src/components/system/UpdateDialog.tsx` ≥ 85%
  - `web/src/components/system/UpdateDialog*Step.tsx` ≥ 85% each
  - `web/src/components/system/UpdateStateMachineViz.tsx` ≥ 90%
- [ ] 12개 GWT 시나리오의 자동화 가능 항목이 통합 테스트로 검증됨
- [ ] msw (또는 fetch mock) 으로 백엔드 응답 시뮬레이션
- [ ] 수동 검증 (DoD 체크리스트) 항목이 acceptance.md 에 명시됨

### R - Readable (가독성)

- [ ] 5단계 step 컴포넌트 명확 분리 (Info / Confirm / Progress / Result)
- [ ] 9-state machine 의 각 상태가 enum 타입으로 명시 (`OperationStatus`)
- [ ] 컴포넌트 prop 타입이 strict TypeScript 로 명시됨
- [ ] 사용자 친화 메시지가 백엔드 식별자와 분리되어 i18n 가능 구조

### U - Unified (일관성)

- [ ] `TsdbDataViewerModal` 의 모달 레이아웃 패턴 (`min-h-0 shrink overflow-y-auto`,
  95vw × 95vh) 재사용
- [ ] `ConfirmDialog` 패턴 (Confirm step) 재사용
- [ ] `MetadataChips` 패턴 (버전/커밋/채널 칩) 재사용
- [ ] `storeErrorMapper` 패턴 (에러 매핑 구조) 차용
- [ ] TanStack Query v5 컨벤션 일관 적용 (queryKey 구조, refetchInterval, enabled 패턴)

### S - Secured (보안) — **본 SPEC 의 핵심 보안 게이트**

- [ ] **운영자 confirm 강제**: 모든 apply 호출은 Confirm step 을 거침 (skip_confirm 우회 차단)
- [ ] **다운그레이드 force flag UI**: 명시적 체크박스 + 추가 동의 단계 (M9, Scenario 6)
- [ ] **클라이언트 권한 검사 의존 안 함**: 백엔드 401/403 응답이 최종 권한 게이트 (M11, F3)
- [ ] **Ed25519 서명 검증 실패 critical 등급**: ErrUpdateSignatureInvalid 는 단순 warning 이
  아닌 critical 톤 (M12, Scenario 5)
- [ ] **백엔드 audit 로그 의존**: 모든 update 시도/실패는 백엔드 구조화 로그에 기록됨 (본 SPEC
  은 백엔드 audit 의존, frontend 가 별도 audit 수행 안 함)
- [ ] **release_notes_url 외부 링크 안전 처리**: `target="_blank" rel="noopener noreferrer"`
- [ ] **사용자 입력 sanitize**: error 원본 메시지 표시 시 XSS 방어 (React JSX 자동 escape 신뢰)

### T - Trackable (추적성)

- [ ] Conventional commit 메시지 + SPEC-WEB-006 참조
- [ ] TAG `@SPEC:SPEC-WEB-006` 인덱싱 (`@PLAN:`, `@ACCEPTANCE:` 동일)
- [ ] 모든 신규 컴포넌트 헤더에 SPEC-WEB-006 v0.1.0 표기
- [ ] `useSystemVersion` 훅의 queryKey 가 `['system', 'version']` 으로 명시 (디버깅 용이)
- [ ] `useUpdateStatus` 훅의 queryKey 가 `['system', 'update', 'status', operationId]` 으로
  operation_id 별 분리

---

## Definition of Done (15+ 체크박스)

### 기능 완성도

- [ ] **DoD-1**: System Status Panel 이 Decision Point 1 결정 위치에서 정상 노출 + 60초 폴링
- [ ] **DoD-2**: "Check for updates" 버튼이 정상 동작 (POST /update/check)
- [ ] **DoD-3**: update_available 인디케이터가 시각적으로 명확하게 표시됨
- [ ] **DoD-4**: Update Dialog 가 5단계 step 으로 정상 동작 (Info → Confirm → Apply → Progress
  → Result)
- [ ] **DoD-5**: 9-state machine 시각화가 모든 상태에서 정상 동작
- [ ] **DoD-6**: 1초 status 폴링이 정상 동작 + terminal state 시 자동 중단
- [ ] **DoD-7**: 다운그레이드 시 force 체크박스 + 추가 동의 단계 정상 동작
- [ ] **DoD-8**: ready_to_restart 운영자 가이드 (CLI 명령어 + 클립보드 복사) 정상 표시
- [ ] **DoD-9**: 실패 시 명시적 rollback 버튼 + 정상 동작
- [ ] **DoD-10**: ErrUpdateRollbackFailed critical 등급 메시지 + 운영자 개입 안내

### 품질

- [ ] **DoD-11**: 신규 컴포넌트 라인 커버리지 85% 이상 달성
- [ ] **DoD-12**: TypeScript strict 통과 (no any, no implicit any)
- [ ] **DoD-13**: ESLint / Prettier 통과 (기존 프로젝트 룰)
- [ ] **DoD-14**: Vitest 모든 테스트 통과 (기존 + 신규)
- [ ] **DoD-15**: 12개 GWT 시나리오 중 자동화 가능 항목이 통합 테스트로 통과

### 결정 + 검토

- [ ] **DoD-16**: 5개 Decision Point 가 사용자에 의해 결정됨 (또는 권장값 적용 명시)
- [ ] **DoD-17**: spec.md 의 Decision Points 표가 결정값으로 갱신됨
- [ ] **DoD-18**: 백엔드 SPEC-UPDATE-001 v0.1.0 머지 확인 (또는 mock 응답 기반 단독 검증)

### 문서화 + 머지

- [ ] **DoD-19**: HISTORY 섹션이 v0.1.0 변경사항으로 갱신됨
- [ ] **DoD-20**: 디자인 자료 (선택): `references/design/system-status-panel.pen` 또는
  `update-dialog.pen`
- [ ] **DoD-21**: PR 머지 시 conventional commit 메시지 + SPEC 참조

---

## Status: draft (Level 1 spec-first lifecycle, v0.1.0 initial draft)
