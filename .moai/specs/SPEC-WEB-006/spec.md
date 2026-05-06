---
id: SPEC-WEB-006
title: xflowd 자동 업데이트 Web UI (System Status Panel + Update Dialog)
version: 0.1.0
status: draft
created: 2026-05-06
updated: 2026-05-06
author: xtra
priority: medium
---

# SPEC-WEB-006: xflowd 자동 업데이트 Web UI

## HISTORY

- **0.1.0** (2026-05-06): Initial draft — SPEC-UPDATE-001 v0.1.0 (자가 교체 자동 업데이트, 5종 REST API)
  를 소비하는 frontend Web UI. 두 가지 핵심 컴포넌트 — (a) **System Status Panel** (현재 버전/빌드/채널/
  업데이트 가능 여부 표시 + 폴링) (b) **Update Dialog** (다단계 업데이트 흐름 — info → confirm →
  apply → progress → result, 9-state machine 시각화 + 실패 시 rollback) — 을 정의한다. 백엔드
  v0.1.0 의 알려진 한계인 (1) in-process restart 미지원, (2) channel 변경 REST 엔드포인트 부재,
  (3) 자동 health check + rollback wiring 미완 — 을 명시적 운영자 안내 + 명시적 rollback 버튼으로
  보완한다. 5개 Decision Point 명시: 위치/알림/재시작 UX/채널 UI/권한 모델. 신규 외부 의존성 없음
  (TanStack Query, Tailwind, Vitest 기존 스택 재사용).

| Version | Date       | Author | Change                                                                |
| ------- | ---------- | ------ | --------------------------------------------------------------------- |
| 0.1.0   | 2026-05-06 | xtra   | 최초 작성 — SPEC-UPDATE-001 v0.1.0 의 5 REST API 를 소비하는 Web UI 정의 |

---

## 개요 (Overview)

### 목적

xflow 운영자가 **CLI 접근 없이 Web UI 에서 xflowd 자동 업데이트를 가시화하고 적용**할 수 있도록
한다. SPEC-UPDATE-001 v0.1.0 가 도입한 5종 REST API (`GET /system/version`,
`POST /system/update/check`, `POST /system/update/apply`, `GET /system/update/status`,
`POST /system/update/rollback`) 를 소비하는 React 컴포넌트 두 종 — (1) System Status Panel,
(2) Update Dialog — 을 추가한다.

### 배경

SPEC-UPDATE-001 v0.1.0 도입 후 운영자는 `xflowd update apply` CLI 만으로 업데이트를 수행할 수 있다.
그러나 다음과 같은 운영 시나리오에서 Web UI 가 필수다:

1. **가시성**: 운영자가 SSH 없이도 현재 버전/채널/업데이트 가능 여부를 한눈에 확인
2. **다중 운영자 협업**: 여러 운영자가 동시에 같은 클러스터를 모니터링할 때, 누군가 update apply
   를 실행 중이라면 다른 운영자가 status 폴링으로 확인 가능
3. **알림 (update available)**: 새 stable 릴리즈가 게시되면 운영자에게 적극적으로 알림 →
   운영 부담 경감, 보안 패치 적용 시점 단축
4. **단계별 가이드**: 다운그레이드 시 `--force` flag 의 의미를 UI 가 명시적으로 경고하여 운영
   사고 예방
5. **9-state machine 시각화**: `idle → starting → checking → downloading → verifying →
   applying → ready_to_restart → completed | failed` 흐름을 시각적으로 추적

### 범위

- **포함**:
  - System Status Panel (현재 버전/빌드/채널/업데이트 가능 인디케이터 + Check 버튼)
  - Update Dialog (info → confirm → apply → progress → result 다단계 모달)
  - 9-state machine 시각화 (각 상태 강조)
  - 실패 시 명시적 rollback 옵션
  - 다운그레이드 시 `--force` 체크박스 + 추가 확인 단계
  - `ready_to_restart` 운영자 안내 (CLI 명령어 표시, v0.1.0 한계 보완)
  - 백엔드 5종 에러 (`ErrUpdateChecksumMismatch`, `ErrUpdateSignatureInvalid`,
    `ErrDowngradeRequiresForce`, `ErrUpdateApplyFailed`, `ErrUpdateRollbackFailed`) 사용자
    친화 메시지 매핑
  - TanStack Query 기반 60s 폴링 (Status Panel) + 1s 폴링 (active operation)
- **제외**:
  - **채널 변경 UI** (Decision Point 4 — 백엔드 REST 엔드포인트 부재로 v0.1.0 에서는 읽기 전용 또는
    제외; SPEC-UPDATE-002 에서 추가 후 본 SPEC v0.2.0 에서 재도입 가능)
  - **자동 in-process restart** (SPEC-UPDATE-001 v0.1.0 한계 — 운영자가 CLI 또는 systemd 로 수동
    재시작; SPEC-UPDATE-002 에서 추가 가능)
  - **자동 health check + rollback wiring** (SPEC-UPDATE-001 v0.1.0 한계 — UI 는 명시적 rollback
    버튼만 노출)
  - **Webhook/이메일 알림** (SPEC-UPDATE-001 future scope)
  - **차등 업데이트 (bsdiff)**, **운영 시간 외 자동 적용** 등

### 가정

- **백엔드 의존성**: SPEC-UPDATE-001 v0.1.0 이 머지되어 5종 REST API 가 동작한다 (현재 `feature/
  SPEC-UPDATE-001` 브랜치에 push 완료, 본 SPEC 머지 시점에는 develop 에 통합되어야 함).
- **인증 미들웨어**: 기존 `web/src/lib/auth` 미들웨어가 모든 update 관련 엔드포인트에 적용되어
  401/403 응답이 일관성 있게 반환된다.
- **타임스탬프 규약**: 백엔드는 `started_at`/`completed_at` 등 시각 필드를 RFC3339 (UTC) 로 반환
  한다. 프론트엔드는 브라우저 로컬 형식으로 렌더링한다 (참고: SPEC-UPDATE-001 응답 예시는 RFC3339).
  단, 본 SPEC 의 internal state 는 epoch ms 로 관리한다 (`project_timestamp_convention.md` 준수).
- **TSDB/Store 와 무관**: 본 SPEC 은 SPEC-WEB-005 의 TSDB 데이터 뷰어 흐름과 독립적으로 동작한다.

### 의존성

- **백엔드 (전제)**: SPEC-UPDATE-001 v0.1.0 (commit pending in `feature/SPEC-UPDATE-001`):
  - `internal/api/handler/system_update.go` (5 REST 엔드포인트)
  - `internal/api/dto/update.go` (요청/응답 DTO)
  - `internal/updater/state.go` (9-state machine + `OperationStatus` enum)
- **프론트엔드 (수정 대상)**:
  - `web/src/App.tsx` 또는 라우팅 설정 (Decision Point 1 결정에 따라)
  - `web/src/services/api/` (신규 `systemUpdate.ts`)
  - `web/src/lib/errors/` (신규 `updaterErrorMapper.ts` 또는 `storeErrorMapper.ts` 확장)
  - `web/src/components/Modal/` (기존 `TsdbDataViewerModal` 레이아웃 패턴 재사용)
- **참조 패턴 (재사용)**:
  - `TsdbDataViewerModal` (SPEC-WEB-005 Phase E) — `min-h-0 shrink overflow-y-auto`,
    `min-h-[180px]` 매트릭스 영역, 95vw × 95vh 모달 레이아웃
  - `ConfirmDialog` (SPEC-WEB-005 Phase E) — 단순 confirm 패턴
  - `MetadataChips` (SPEC-WEB-005 Phase E) — 무채색 톤 + monospaced 폰트 칩 표시
  - `storeErrorMapper` (SPEC-WEB-005 Phase E) — 백엔드 에러 → 사용자 친화 메시지

---

## EARS 요구사항 (M1-M12)

### M1: System Status Panel — 위치 및 노출 (Decision Point 1)

- **Decision Point 1**: System Status Panel 의 노출 위치는 다음 중 하나로 결정한다 (사용자 결정
  필요):
  - **옵션 A**: 새 라우트 `/admin/system` (좌측 nav 에 "System" 항목 추가, 권한별 분리 용이)
  - **옵션 B**: 기존 settings 탭 안에 sub-tab `Settings → System Status` (기존 nav 변경 없음)
  - **옵션 C**: 헤더 드롭다운 (메뉴바 우측에 버전 정보 + 업데이트 가능 배지, 미니멀 UX)
  - **옵션 D**: 대시보드 사이드바 위젯 (다른 위젯과 함께 노출, 운영자 시야 항상 확보)
  - **권장**: 옵션 A (라우팅 명확성 + 권한 분리 용이성). 옵션 C 는 update_available 배지만 보조
    노출하는 보완 패턴으로 함께 채택 가능.
- **Ubiquitous**: 시스템은 결정된 위치에서 System Status Panel 을 운영자에게 항상 노출해야 한다.
- **State-driven**: WHILE 인증되지 않은 사용자가 해당 위치로 접근하면, 시스템은 로그인 페이지로
  리다이렉트하거나 401/403 에러 화면을 표시해야 한다 (Decision Point 5 의 권한 모델에 따라).

### M2: System Status Panel — 콘텐츠 표시

- **Ubiquitous**: System Status Panel 은 다음 정보를 항상 표시해야 한다:
  - **현재 버전** (`version`, 예: `v0.3.0`)
  - **빌드 커밋** (`commit`, 단축 해시 7자, 예: `6b9c531`)
  - **빌드 일시** (`build_date`, 브라우저 로컬 형식 `YYYY-MM-DD HH:mm:ss`)
  - **Go 버전** (`go_version`, 예: `go1.22.5`)
  - **OS / Arch** (`os` / `arch`, 예: `linux/amd64`)
  - **현재 채널** (`channel`, `stable | beta | nightly`)
  - **마지막 확인 시각** (`last_checked_at`, 클라이언트가 마지막 폴링한 시점, 로컬 형식)
  - **업데이트 가능 여부** (`update_available: bool` — true 일 때 시각적 인디케이터)
  - **최신 버전** (`latest_version: string | null` — `update_available == true` 일 때만 표시)
- **Ubiquitous**: 시스템은 `update_available == true` 일 때 명확한 시각 인디케이터 (배지/dot/
  강조 텍스트) 를 표시하고 "업데이트 가능" 라벨을 노출해야 한다.
- **Optional**: WHERE 운영자가 build commit 또는 build_date 를 클릭하면, 시스템은 GitHub commit
  URL (예: `https://github.com/xtra72/xflow/commit/{commit}`) 을 새 탭으로 연다.

### M3: System Status Panel — 폴링 및 캐싱

- **Ubiquitous**: 시스템은 TanStack Query 의 `useSystemVersion` 훅을 사용하여 `GET /api/v1/
  system/version` 을 주기적으로 폴링해야 한다.
- **Ubiquitous**: 폴링 간격은 `refetchInterval: 60000` (60초), `staleTime: 30000` (30초) 이다.
- **State-driven**: WHILE 사용자가 다른 탭으로 전환하면, 시스템은 폴링을 일시 중지해야 한다
  (`refetchIntervalInBackground: false`).
- **Event-driven**: WHEN 사용자가 "Check for updates" 버튼을 클릭하면, THEN 시스템은
  `POST /api/v1/system/update/check` 을 호출하여 즉시 채널을 폴링하고, 그 결과로 `update_available`
  을 갱신해야 한다.
- **State-driven**: WHILE Check 요청이 진행 중이면, 시스템은 버튼을 비활성화하고 진행 인디케이터
  를 표시해야 한다.
- **Unwanted**: 시스템은 60초보다 짧은 간격으로 자동 폴링을 수행하지 않아야 한다 (네트워크 부담
  방지).

### M4: 업데이트 가능 알림 (Decision Point 2)

- **Decision Point 2**: 업데이트 가능 알림 방식은 다음 중 하나로 결정한다 (사용자 결정 필요):
  - **옵션 A**: Toast 알림 (한 번만 표시, 사용자가 dismiss 가능, push 형 강조)
  - **옵션 B**: Header badge dot (지속적 시각 인디케이터, 사용자가 클릭 시 Status Panel 로 이동)
  - **옵션 C**: 둘 다 (toast + badge — 적극적 강조)
  - **옵션 D**: Status Panel 내부만 (passive, 사용자가 직접 페이지 방문 시에만 인지)
  - **옵션 E**: 사용자 토글 (설정에서 알림 켜기/끄기)
  - **권장**: 옵션 C (toast 첫 등장 + badge 지속 표시) — 사용성 + 선명한 시각 강조 균형.
- **Ubiquitous**: 시스템은 `update_available` 이 `false → true` 로 전이될 때 결정된 알림 방식을
  활성화해야 한다.
- **State-driven**: WHILE `update_available == false` 또는 사용자가 toast 를 dismiss 했으면,
  시스템은 toast 를 다시 표시하지 않아야 한다 (단, 새로운 latest_version 이 감지되면 다시 표시
  가능).
- **State-driven**: WHILE Decision Point 5 의 권한 모델에 따라 admin 만 업데이트 권한을 갖는
  경우, 시스템은 일반 사용자에게 알림을 표시하지 않아야 한다.

### M5: Update Dialog — 단계별 UX

- **Ubiquitous**: Update Dialog 는 다음 5단계로 구성되어야 한다:
  1. **Info step**: 현재 버전 → 타겟 버전 비교, 릴리즈 노트 링크, 버전 차이 시각화
  2. **Confirm step**: 다운그레이드 시 `--force` 체크박스, 경고 배너, "확인" 버튼
  3. **Apply step**: `POST /api/v1/system/update/apply` 호출, `operation_id` 수신
  4. **Progress step**: `GET /api/v1/system/update/status` 1초 폴링, 9-state machine 시각화
  5. **Result step**: 성공 시 `ready_to_restart` 운영자 안내 OR `completed` 메시지, 실패 시 에러
     + rollback 옵션
- **Event-driven**: WHEN 사용자가 Status Panel 의 "Update available" 버튼을 클릭하면, THEN 시스템은
  Info step 부터 모달을 열어야 한다.
- **Event-driven**: WHEN 사용자가 모달 외부 영역 클릭, Esc 키 입력, 또는 X 버튼 클릭을 수행하면,
  THEN 시스템은 모달을 닫아야 한다. 단, **active operation 이 진행 중이면 (status ∈
  {downloading, verifying, applying, ready_to_restart}) 모달을 닫아도 백엔드 작업은 계속
  진행된다**.
- **Ubiquitous**: 모달은 `TsdbDataViewerModal` 의 레이아웃 패턴 (`min-h-0 shrink overflow-y-auto`)
  을 재사용하여 작은 화면에서도 내용이 가려지지 않아야 한다.
- **State-driven**: WHILE 단계가 Apply, Progress 면, "다음" / "이전" 네비게이션은 비활성화 또는
  숨김 처리해야 한다.

### M6: Update Dialog — 진행 시각화 (9-state machine)

- **Ubiquitous**: Progress step 은 9개 상태를 좌→우 또는 위→아래 단계 흐름으로 시각화해야 한다:
  `idle | starting | checking | downloading | verifying | applying | ready_to_restart |
  completed | failed`.
- **Ubiquitous**: 시스템은 현재 상태를 강조 표시하고, 완료된 상태는 체크 표시 + 무채색, 미진행
  상태는 흐림 톤으로 렌더링해야 한다.
- **Ubiquitous**: 시스템은 `GET /api/v1/system/update/status` 를 1초 간격으로 폴링하여 상태를
  갱신해야 한다 (`refetchInterval: 1000`).
- **State-driven**: WHILE `status ∈ {idle, completed, failed}` 이면, 시스템은 폴링을 중단해야
  한다 (active operation 종료).
- **Event-driven**: WHEN `status` 가 `downloading` 이면, 시스템은 다운로드 진행률 (`progress_
  percent` 필드, SPEC-UPDATE-001 응답에 존재) 을 progress bar 로 함께 표시해야 한다.
- **Ubiquitous**: 각 상태 옆에는 사용자 친화 설명 텍스트를 표시해야 한다 (예: `verifying` →
  "서명 및 체크섬 검증 중...", `applying` → "원자적 바이너리 교체 중...").
- **Unwanted**: 시스템은 1초보다 짧은 간격으로 status 폴링을 수행하지 않아야 한다.

### M7: Update Dialog — 실패/롤백 처리

- **State-driven**: WHILE `status == "failed"` 면, 시스템은 Result step 으로 전환하여 다음을 표시
  해야 한다:
  - 실패 사유 (백엔드 `error` 필드, 사용자 친화 메시지로 매핑 — M12 참조)
  - 영향받지 않은 백업 파일 위치 안내 (`<binary>.previous`)
  - **명시적 "Rollback 시도" 버튼** (백엔드 v0.1.0 한계 보완 — 자동 rollback wiring 미완)
  - "닫기" 버튼
- **Event-driven**: WHEN 사용자가 "Rollback 시도" 버튼을 클릭하면, THEN 시스템은 `POST /api/v1/
  system/update/rollback` 을 호출하고, 응답에 따라 다시 Result step 의 메시지를 갱신해야 한다.
- **State-driven**: WHILE rollback 응답이 `ErrUpdateRollbackFailed` 이면 (백업 파일 없음/손상),
  시스템은 명시적 critical 경고 + "운영자 수동 개입 필요" 메시지 + `<binary>.previous` 위치
  안내를 표시해야 한다.
- **Ubiquitous**: rollback 결과는 toast 또는 inline 알림으로 사용자에게 즉시 반영되어야 한다.

### M8: Update Dialog — "ready_to_restart" 운영자 안내 (Decision Point 3)

- **Decision Point 3**: `ready_to_restart` 상태에서의 운영자 가이드는 다음 중 하나로 결정한다
  (사용자 결정 필요):
  - **옵션 A**: 운영자 안내만 (간단한 문구 — "재시작이 필요합니다. systemd 또는 launchd 가 자동으로
    재시작하거나, 수동으로 `systemctl restart xflowd` 를 실행하세요.")
  - **옵션 B**: CLI 명령어 표시 (운영체제별 분기 — Linux: `systemctl restart xflowd`, macOS:
    `launchctl kickstart -k system/xflowd`, 클립보드 복사 버튼 포함)
  - **옵션 C**: 향후 SPEC 에서 자동화 (현재는 옵션 B 표시 + "Future: SPEC-UPDATE-002 에서 in-
    process restart 자동화 예정" 노트)
  - **권장**: 옵션 B + C (CLI 명령어 + 미래 자동화 노트). v0.1.0 한계의 명시적 보완.
- **State-driven**: WHILE `status == "ready_to_restart"` 이면, 시스템은 운영자 가이드를 Result
  step 또는 Progress step 끝부분에 표시해야 한다.
- **Ubiquitous**: 운영자 가이드는 다음을 포함해야 한다:
  - 새 바이너리가 디스크에 적용되었음을 명시
  - 백업 파일 위치 (`<binary>.previous`) 안내
  - 운영자가 재시작을 실행해야 함을 강조 (또는 supervisor 가 자동 재시작함을 표시)
  - SPEC-UPDATE-002 future 노트 (옵션 C 채택 시)
- **Optional**: WHERE 옵션 B 가 채택되면, 시스템은 클립보드 복사 버튼을 명령어 옆에 표시한다.

### M9: 다운그레이드 force flag UI

- **State-driven**: WHILE 사용자가 Info step 에서 타겟 버전을 확인하고, 그 버전이 현재 버전보다
  낮으면 (semver 비교), 시스템은 다음을 수행해야 한다:
  - 명시적 경고 배너 표시: "⚠ 다운그레이드 시도입니다. 데이터 손실 또는 호환성 문제가 발생할 수
    있습니다."
  - Confirm step 에 `--force` 체크박스 추가
  - "확인" 버튼은 체크박스가 활성화될 때까지 비활성 상태
  - 체크박스 라벨: "다운그레이드를 강제 적용함을 확인합니다 (`--force`)"
- **Event-driven**: WHEN 사용자가 force 체크박스를 활성화하고 확인 버튼을 클릭하면, THEN 시스템은
  `POST /api/v1/system/update/apply` 의 요청 바디에 `force: true` 를 포함하여 전송해야 한다.
- **Unwanted**: 시스템은 force 체크박스가 비활성 상태에서 다운그레이드 apply 요청을 전송하지
  않아야 한다.
- **State-driven**: WHILE 백엔드가 `ErrDowngradeRequiresForce` 를 반환하면 (예: race condition),
  시스템은 사용자를 Confirm step 으로 되돌리고 force 체크박스를 강조해야 한다.

### M10: 채널 변경 UI (Decision Point 4)

- **Decision Point 4**: 채널 변경 UI 는 다음 중 하나로 결정한다 (사용자 결정 필요):
  - **옵션 A**: **포함** (백엔드 SPEC-UPDATE-002 에서 `PUT /system/update/channel` 신규 엔드포인트
    추가 필요 → 본 SPEC v0.2.0 에서 도입 가능, v0.1.0 에서는 제외 권장)
  - **옵션 B**: **제외** (CLI 명령어 안내만 표시 — `xflowd update channel <stable|beta|nightly>`)
  - **옵션 C**: **읽기 전용** (현재 채널만 표시, 변경 불가)
  - **권장**: 옵션 C (읽기 전용 + CLI 안내 부가 표시). v0.1.0 백엔드 엔드포인트 부재 한계 명시.
- **State-driven**: WHILE 옵션 C 가 채택되면, 시스템은 현재 채널을 표시하고 그 옆에 "채널 변경은
  CLI 에서: `xflowd update channel <name>`" 도움말을 표시해야 한다.
- **Optional**: WHERE 옵션 A 가 미래 SPEC-UPDATE-002 에 도입되면, 본 SPEC v0.2.0 에서 채널 셀렉터
  를 추가한다 (현재 SPEC 범위 외).

### M11: 권한 / 인증 (Decision Point 5)

- **Decision Point 5**: 권한 모델은 다음 중 하나로 결정한다 (사용자 결정 필요):
  - **옵션 A**: 모든 인증된 사용자 (단순; 작은 팀에 적합)
  - **옵션 B**: admin 전용 (RBAC; 운영 사고 예방, 권장)
  - **옵션 C**: 환경변수 토글 (`XFLOW_UPDATE_UI_RBAC=admin|all`, 배포별 결정)
  - **옵션 D**: 사용자별 권한 (세분화된 RBAC, 본 SPEC 범위 외 — 별도 SPEC 필요)
  - **권장**: 옵션 B (admin 전용). 운영 안전성 확보. 옵션 C 는 점진적 도입을 위한 토글 옵션.
- **State-driven**: WHILE 사용자가 권한 없이 update 관련 엔드포인트를 호출하면, 시스템은 백엔드의
  401/403 응답을 받아 명시적 에러 화면 또는 inline 메시지를 표시해야 한다.
- **Ubiquitous**: 시스템은 권한 없는 사용자에게 Status Panel 의 "Update available" 버튼을 비활성
  화하거나 숨겨야 한다 (Decision Point 5 결정에 따라).
- **Unwanted**: 시스템은 클라이언트 측 권한 검사만으로 보안을 보장하지 않아야 한다 (백엔드의
  401/403 응답이 최종 권한 게이트).

### M12: 에러 메시지 매핑

- **Ubiquitous**: 시스템은 백엔드 에러 식별자/메시지를 다음 사용자 친화 문구로 매핑해야 한다:
  - `ErrUpdateChecksumMismatch` → "다운로드된 바이너리의 체크섬이 일치하지 않습니다. 다시 시도
    하거나, 채널 서버의 무결성을 확인하세요."
  - `ErrUpdateSignatureInvalid` → "디지털 서명 검증에 실패했습니다. 잠재적 보안 침해 신호이므로
    운영팀에 즉시 알려주세요."
  - `ErrUpdateDownloadFailed` → "바이너리 다운로드에 실패했습니다. 네트워크 연결을 확인하고 다시
    시도하세요."
  - `ErrUpdateInsufficientDiskSpace` → "임시 디렉토리의 가용 공간이 부족합니다. 디스크 공간을
    확보 후 다시 시도하세요."
  - `ErrUpdateApplyFailed` → "바이너리 교체에 실패했습니다. 권한 또는 파일시스템 상태를 확인하세요."
  - `ErrUpdateRollbackFailed` → "롤백에 실패했습니다. 백업 파일 (`<binary>.previous`) 위치를
    수동으로 확인하고 운영자 개입이 필요합니다."
  - `ErrDowngradeRequiresForce` → "다운그레이드를 적용하려면 `--force` 옵션이 필요합니다."
  - `ErrUpdateInProgress` (HTTP 409) → "다른 사용자가 이미 업데이트를 진행 중입니다. status
    화면에서 진행 상황을 확인하세요."
  - `ErrUpdateChannelInvalid` → "채널 설정이 유효하지 않습니다. 운영팀에 문의하세요."
- **Event-driven**: WHEN 백엔드 에러 응답이 위 식별자 패턴 중 하나에 매칭되면, THEN 시스템은
  매핑된 문구를 toast 또는 모달 inline 에러로 표시하고, 원본 메시지는 콘솔 또는 펼침 영역에
  보조 노출해야 한다.
- **Unwanted**: 시스템은 매핑되지 않은 에러를 빈 문자열이나 일반화된 "오류" 만으로 표시하고
  침묵하지 않아야 한다 (원본 메시지 fallback 표시 + GitHub Issues 링크 안내).

---

## 명세 (Specifications)

### TypeScript 타입

```typescript
// web/src/services/api/systemUpdate.ts (신규)

export type UpdateChannel = 'stable' | 'beta' | 'nightly';

export interface VersionInfo {
  version: string;             // 예: "v0.3.0"
  commit: string;              // 단축 해시 7자
  build_date: string;          // RFC3339, 백엔드에서 epoch ms 또는 RFC3339 결정 필요
  go_version: string;          // 예: "go1.22.5"
  os: string;                  // 예: "linux"
  arch: string;                // 예: "amd64"
  channel: UpdateChannel;
  update_available: boolean;
  latest_version: string | null;
}

export type OperationStatus =
  | 'idle'
  | 'starting'
  | 'checking'
  | 'downloading'
  | 'verifying'
  | 'applying'
  | 'ready_to_restart'
  | 'completed'
  | 'failed';

export interface UpdateOperation {
  operation_id: string;
  status: OperationStatus;
  progress_percent: number | null;
  from_version: string;
  to_version: string;
  started_at: string;          // RFC3339
  completed_at: string | null; // RFC3339, status ∈ {completed, failed} 시 채워짐
  error: string | null;        // status == 'failed' 시 백엔드 에러 메시지
}

export interface CheckResult {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  channel: UpdateChannel;
  release_notes_url: string;
  published_at: string;
  asset_size_bytes: number;
}

export interface ApplyRequest {
  version?: string;            // optional: 명시적 타겟. 없으면 latest
  force?: boolean;             // optional: 다운그레이드 허용
  skip_confirm?: boolean;      // 자동화 호출 시 true
}

export interface RollbackResult {
  success: boolean;
  from_version: string;
  to_version: string;
  error: string | null;
}
```

### TanStack Query 훅 시그니처

```typescript
// 폴링 훅 (Status Panel)
export function useSystemVersion(): UseQueryResult<VersionInfo>;

// Check 뮤테이션 (Status Panel "Check for updates" 버튼)
export function useUpdateCheck(): UseMutationResult<CheckResult>;

// Apply 뮤테이션 (Update Dialog Apply step)
export function useUpdateApply(): UseMutationResult<UpdateOperation, ApplyRequest>;

// Status 폴링 훅 (Update Dialog Progress step, 1초 간격)
export function useUpdateStatus(operationId: string | null): UseQueryResult<UpdateOperation>;

// Rollback 뮤테이션 (Update Dialog Result step, 실패 시)
export function useUpdateRollback(): UseMutationResult<RollbackResult>;
```

### 라우팅 (Decision Point 1 결정 후 확정)

- **옵션 A 채택 시**: 신규 라우트 `/admin/system` → `<SystemStatusPage />` 컴포넌트
- **옵션 B 채택 시**: 기존 settings 라우트에 sub-tab → `<SystemStatusSection />` 컴포넌트
- **옵션 C 채택 시**: 헤더 컴포넌트에 dropdown trigger → `<SystemStatusDropdown />`
- **옵션 D 채택 시**: 대시보드 컴포넌트에 widget → `<SystemStatusWidget />`

### 컴포넌트 구조 (예정)

```
web/src/
├── pages/
│   └── system/
│       └── SystemStatusPage.tsx          # 옵션 A 채택 시 라우트 컴포넌트
├── components/
│   └── system/
│       ├── SystemStatusPanel.tsx          # 핵심 패널 (위치 무관 재사용)
│       ├── UpdateAvailableBadge.tsx       # 인디케이터 배지
│       ├── UpdateDialog.tsx               # 다단계 모달 컨테이너
│       ├── UpdateDialogInfoStep.tsx       # Step 1: Info
│       ├── UpdateDialogConfirmStep.tsx    # Step 2: Confirm + force 체크박스
│       ├── UpdateDialogProgressStep.tsx   # Step 3+4: Apply + Progress (9-state)
│       ├── UpdateDialogResultStep.tsx     # Step 5: Result (success | failed)
│       └── UpdateStateMachineViz.tsx      # 9-state machine 시각화
├── services/
│   └── api/
│       └── systemUpdate.ts                # API 클라이언트 + 훅
└── lib/
    └── errors/
        └── updaterErrorMapper.ts          # 백엔드 에러 → UI 메시지 매핑
```

---

## Decision Points (사용자 결정 필요, 5종)

| # | 결정 항목 | 옵션 | 권장 | 영향 |
|---|----------|------|------|------|
| 1 | System Status Panel 위치 | A: 새 라우트 / B: settings 탭 / C: header dropdown / D: dashboard widget | A + C 보완 | 라우팅 + 권한 분리 + UX |
| 2 | 알림 방식 | A: toast / B: badge / C: 둘 다 / D: 패널 내부만 / E: 사용자 토글 | C (toast + badge) | UX 적극성 |
| 3 | 재시작 UX | A: 안내만 / B: CLI 명령어 표시 / C: 향후 자동화 노트 | B + C (CLI + 미래 노트) | 운영자 가이드 명확성 |
| 4 | 채널 변경 UI | A: 포함 (백엔드 신규 API 필요) / B: 제외 (CLI 안내) / C: 읽기 전용 | C (읽기 전용 + CLI 안내) | v0.1.0 백엔드 한계 |
| 5 | 권한 모델 | A: 모든 인증 사용자 / B: admin 전용 / C: 환경변수 토글 / D: 세분화 RBAC | B (admin 전용) | 운영 안전성 |

---

## TAG Traceability

- `@SPEC:SPEC-WEB-006` → spec.md (이 문서)
- `@PLAN:SPEC-WEB-006` → plan.md
- `@ACCEPTANCE:SPEC-WEB-006` → acceptance.md
- 구현 경로 (예정):
  - `web/src/services/api/systemUpdate.ts` (신규 — TanStack Query 훅)
  - `web/src/lib/errors/updaterErrorMapper.ts` (신규 — 또는 `storeErrorMapper.ts` 확장)
  - `web/src/components/system/SystemStatusPanel.tsx` (신규)
  - `web/src/components/system/UpdateDialog.tsx` (신규)
  - `web/src/components/system/UpdateDialogInfoStep.tsx` (신규)
  - `web/src/components/system/UpdateDialogConfirmStep.tsx` (신규)
  - `web/src/components/system/UpdateDialogProgressStep.tsx` (신규)
  - `web/src/components/system/UpdateDialogResultStep.tsx` (신규)
  - `web/src/components/system/UpdateStateMachineViz.tsx` (신규)
  - `web/src/components/system/UpdateAvailableBadge.tsx` (신규)
  - Decision Point 1 결정에 따라 추가:
    - 옵션 A: `web/src/pages/system/SystemStatusPage.tsx` (신규 라우트)
    - 옵션 B: settings 탭 컴포넌트 수정
    - 옵션 C: 헤더 컴포넌트 수정 (`web/src/components/layout/Header.tsx` 등)
    - 옵션 D: 대시보드 위젯 추가

---

## 관련 SPEC

- **SPEC-UPDATE-001 v0.1.0** (전제, 구현 완료, 머지 대기): 5종 REST API + 9-state machine +
  Ed25519 서명 검증 + atomic replacement + 백업/롤백 인프라
- **SPEC-WEB-005 v0.7.0** (참조, 완료): `TsdbDataViewerModal` 모달 레이아웃 패턴, `MetadataChips`,
  `storeErrorMapper`, `ConfirmDialog` 패턴 재사용
- **SPEC-API-001** (참조, 완료): REST API 응답 형식 + 인증 미들웨어
- **SPEC-UPDATE-002 (예정)**: xflow-agent 자가 업데이트 + **`PUT /system/update/channel` REST
  엔드포인트 신규 추가** + **in-process restart API** → 본 SPEC v0.2.0 에서 채널 셀렉터 + 자동
  재시작 통합 가능
- **SPEC-UPDATE-003 (예정)**: Windows 지원 (`MoveFileEx` + Service Control Manager) — 본 SPEC
  은 현재 Linux/macOS 한정
- **SPEC-AUTH-001 / -002** (참조): 인증 미들웨어 + 권한 모델 (Decision Point 5 결정 시 확장 가능)

---

## v0.1.0 알려진 제약 (백엔드 v0.1.0 한계 보완)

본 SPEC 은 SPEC-UPDATE-001 v0.1.0 의 다음 한계를 명시적 운영자 안내로 보완한다:

| 백엔드 한계 (v0.1.0) | UI 보완 방안 (본 SPEC) |
|---------------------|----------------------|
| In-process restart 미지원 (operation 이 `ready_to_restart` 에서 종료) | M8 — 운영자 가이드 + CLI 명령어 표시 + 클립보드 복사 |
| 자동 health check + rollback wiring 미완 | M7 — Result step 에 명시적 "Rollback 시도" 버튼 노출 |
| 채널 변경 REST 엔드포인트 부재 (CLI only) | M10 — 읽기 전용 + CLI 명령어 안내 (Decision Point 4) |

본 SPEC 의 v0.2.0 에서 SPEC-UPDATE-002 도입 후 위 한계가 백엔드에서 해소되면, UI 도 점진적으로
자동화 패턴으로 진화한다.

---

## Implementation Notes

### 신규 외부 의존성

- **없음**. TanStack Query v5, Tailwind CSS, Vitest + React Testing Library 모두 기존 프로젝트
  의존성. recharts 등도 사용하지 않음 (단순 progress bar 와 step 시각화는 Tailwind CSS 만으로
  구현 가능).

### 기존 패턴 재사용

- `TsdbDataViewerModal` — 모달 레이아웃 (`min-h-0 shrink overflow-y-auto`, 95vw × 95vh),
  Esc 처리, focus trap, portal mount 패턴
- `ConfirmDialog` — Confirm step 의 단순 confirm 패턴 (force 체크박스 통합)
- `MetadataChips` (SPEC-WEB-005 Phase E) — 버전/커밋/채널 칩 표시 (무채색 톤 + monospaced 폰트)
- `storeErrorMapper` (SPEC-WEB-005 Phase E) — 신규 `updaterErrorMapper` 구현 시 동일 패턴
  (식별자/메시지 패턴 매칭 + fallback 처리) 차용

### 디자인 자료 (참고)

- 새 디자인 자료 파일 권장: `references/design/system-status-panel.pen` 또는
  `references/design/update-dialog.pen`
- v0.6.0 의 `tsdb-data-viewer.pen` 모달 레이아웃을 참고하여 일관된 디자인 톤 유지

### Status: draft (Level 1 spec-first lifecycle, v0.1.0 initial draft)
