---
spec_id: SPEC-WEB-006
version: 0.1.0
status: draft
updated: 2026-05-06
---

# SPEC-WEB-006: Implementation Plan

## v0.1.0 Scope Note

본 계획서는 SPEC-WEB-006 v0.1.0 — **SPEC-UPDATE-001 v0.1.0 의 5종 REST API 를 소비하는 Web UI** —
의 frontend 단독 구현 계획이다. 백엔드 변경은 없으며, frontend 의 신규 컴포넌트 + API 훅 + 에러
매핑만 추가한다.

핵심 산출물 (M1-M12):

- `useSystemVersion` / `useUpdateCheck` / `useUpdateApply` / `useUpdateStatus` /
  `useUpdateRollback` 5종 TanStack Query 훅 (`web/src/services/api/systemUpdate.ts` 신규)
- `SystemStatusPanel` 컴포넌트 + 노출 위치 (Decision Point 1 결정에 따라 페이지/탭/dropdown/
  widget)
- `UpdateDialog` 다단계 모달 + 5단계 step 컴포넌트 (Info → Confirm → Apply → Progress → Result)
- `UpdateStateMachineViz` 9-state machine 시각화 컴포넌트
- `updaterErrorMapper` 백엔드 에러 → UI 메시지 매핑 (8종 에러)
- `update_available` 알림 (Decision Point 2 결정에 따라 toast / badge / 둘 다)

**5개 Decision Point 가 결정되어야 구현이 시작된다**. 권장 결정값으로 진행 시 작업량은 약 12-15
tasks (1.5-2 PR 단위), 모든 옵션 채택 시 약 18-20 tasks.

---

## 기술 스택 및 라이브러리 버전

### Frontend (기존 스택 재사용)

- **React**: 19 stable
- **TypeScript**: 5.9+ strict
- **TanStack Query**: v5.x (폴링 + 뮤테이션 + 캐싱)
- **Tailwind CSS**: 기존 설정 (9-state machine 시각화는 Tailwind only)
- **Zustand**: 필요 시 사용 (대부분 로컬 state 로 충분)
- **테스트**: Vitest + React Testing Library + `@testing-library/user-event`

### 신규 외부 라이브러리

**없음**. 기존 프로젝트 의존성만 사용한다. recharts, framer-motion 등 추가하지 않음.

### Backend 의존성 (전제)

- SPEC-UPDATE-001 v0.1.0 (현재 `feature/SPEC-UPDATE-001` 브랜치, 미머지). 본 SPEC 작업 시작 전
  SPEC-UPDATE-001 v0.1.0 이 develop 에 머지되어 있어야 한다.

---

## 영향 범위 (영향 모듈 표)

| 카테고리 | 파일 | 작업 유형 | 우선순위 |
|----------|------|-----------|----------|
| **신규 — API 훅** | `web/src/services/api/systemUpdate.ts` | 신규 | P0 |
| **신규 — API 훅 테스트** | `web/src/services/api/systemUpdate.test.ts` | 신규 | P0 |
| **신규 — 에러 매핑** | `web/src/lib/errors/updaterErrorMapper.ts` | 신규 | P0 |
| **신규 — 에러 매핑 테스트** | `web/src/lib/errors/updaterErrorMapper.test.ts` | 신규 | P0 |
| **신규 — 패널** | `web/src/components/system/SystemStatusPanel.tsx` | 신규 | P0 |
| **신규 — 패널 테스트** | `web/src/components/system/SystemStatusPanel.test.tsx` | 신규 | P0 |
| **신규 — 모달 컨테이너** | `web/src/components/system/UpdateDialog.tsx` | 신규 | P0 |
| **신규 — 모달 테스트** | `web/src/components/system/UpdateDialog.test.tsx` | 신규 | P0 |
| **신규 — Info step** | `web/src/components/system/UpdateDialogInfoStep.tsx` | 신규 | P1 |
| **신규 — Confirm step** | `web/src/components/system/UpdateDialogConfirmStep.tsx` | 신규 | P1 |
| **신규 — Progress step** | `web/src/components/system/UpdateDialogProgressStep.tsx` | 신규 | P0 |
| **신규 — Result step** | `web/src/components/system/UpdateDialogResultStep.tsx` | 신규 | P0 |
| **신규 — State machine 시각화** | `web/src/components/system/UpdateStateMachineViz.tsx` | 신규 | P0 |
| **신규 — Badge** | `web/src/components/system/UpdateAvailableBadge.tsx` | 신규 | P1 |
| **수정 — 라우팅** | `web/src/App.tsx` 또는 라우터 (Decision Point 1) | 수정 | P0 |
| **수정 — 헤더 (옵션 C 채택 시)** | `web/src/components/layout/Header.tsx` | 수정 | P2 |
| **수정 — Settings (옵션 B 채택 시)** | `web/src/pages/settings/*` | 수정 | P2 |
| **수정 — 대시보드 (옵션 D 채택 시)** | `web/src/pages/dashboard/*` | 수정 | P2 |
| **참고 — 디자인 자료 (옵션)** | `references/design/system-status-panel.pen` | 신규 | P2 |
| **참고 — 디자인 자료 (옵션)** | `references/design/update-dialog.pen` | 신규 | P2 |

**예상 변경량 (권장 결정값 기준)**:

- 신규 파일: 약 14개 (소스 7 + 테스트 7)
- 수정 파일: 약 1-3개 (라우팅 + Decision Point 1 옵션에 따라)
- 신규 테스트: 약 30-40개
- 신규 외부 라이브러리: 0

---

## 기술적 접근

### 1. 컴포넌트 구조 (계층)

```
SystemStatusPanel (재사용 가능, 위치 무관)
├── VersionInfoSection
│   ├── version + commit + build_date 표시
│   ├── go_version + os/arch 표시
│   └── channel + last_checked_at 표시
├── UpdateAvailableBadge (조건부, update_available == true)
└── ActionsSection
    ├── "Check for updates" 버튼 → useUpdateCheck()
    └── "Update available" 버튼 → UpdateDialog 오픈

UpdateDialog (모달 컨테이너, TsdbDataViewerModal 레이아웃 패턴 재사용)
├── DialogHeader (close button, step indicator)
├── Step Content (조건부 렌더링)
│   ├── UpdateDialogInfoStep (current → target, release notes)
│   ├── UpdateDialogConfirmStep (force checkbox, warnings)
│   ├── UpdateDialogProgressStep (UpdateStateMachineViz + progress bar)
│   └── UpdateDialogResultStep (success | failed + rollback button)
└── DialogFooter (next/back/close buttons)

UpdateStateMachineViz (9-state visualization)
├── State indicators (idle | starting | checking | downloading |
│                     verifying | applying | ready_to_restart |
│                     completed | failed)
├── Current state highlight
├── Completed state checkmarks
└── State description text
```

### 2. API 훅 구현 패턴

```typescript
// web/src/services/api/systemUpdate.ts

import { useQuery, useMutation, UseQueryResult, UseMutationResult } from '@tanstack/react-query';

const STATUS_POLL_INTERVAL_MS = 1000;
const VERSION_POLL_INTERVAL_MS = 60000;
const VERSION_STALE_TIME_MS = 30000;

export function useSystemVersion(): UseQueryResult<VersionInfo> {
  return useQuery({
    queryKey: ['system', 'version'],
    queryFn: async () => {
      const response = await fetch('/api/v1/system/version');
      if (!response.ok) throw await mapApiError(response);
      return response.json().then((r) => r.data);
    },
    refetchInterval: VERSION_POLL_INTERVAL_MS,
    staleTime: VERSION_STALE_TIME_MS,
    refetchIntervalInBackground: false,
  });
}

export function useUpdateStatus(operationId: string | null): UseQueryResult<UpdateOperation> {
  return useQuery({
    queryKey: ['system', 'update', 'status', operationId],
    queryFn: async () => {
      const response = await fetch('/api/v1/system/update/status');
      if (!response.ok) throw await mapApiError(response);
      return response.json().then((r) => r.data);
    },
    enabled: operationId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === 'completed' || status === 'failed' || status === 'idle') return false;
      return STATUS_POLL_INTERVAL_MS;
    },
  });
}

// useUpdateCheck, useUpdateApply, useUpdateRollback 은 useMutation 패턴
```

### 3. 에러 매핑 패턴 (storeErrorMapper 차용)

```typescript
// web/src/lib/errors/updaterErrorMapper.ts

interface UpdaterErrorEntry {
  identifier: string;             // 예: "ErrUpdateChecksumMismatch"
  pattern?: RegExp;               // 메시지 패턴 매칭 (식별자 부재 시 fallback)
  userMessage: string;
  severity: 'info' | 'warning' | 'critical';
}

const ERROR_MAP: UpdaterErrorEntry[] = [
  {
    identifier: 'ErrUpdateChecksumMismatch',
    userMessage: '다운로드된 바이너리의 체크섬이 일치하지 않습니다. 다시 시도하거나, 채널 서버의 무결성을 확인하세요.',
    severity: 'warning',
  },
  {
    identifier: 'ErrUpdateSignatureInvalid',
    userMessage: '디지털 서명 검증에 실패했습니다. 잠재적 보안 침해 신호이므로 운영팀에 즉시 알려주세요.',
    severity: 'critical',
  },
  {
    identifier: 'ErrUpdateRollbackFailed',
    userMessage: '롤백에 실패했습니다. 백업 파일 (`<binary>.previous`) 위치를 수동으로 확인하고 운영자 개입이 필요합니다.',
    severity: 'critical',
  },
  // ... 나머지 6종
];

export function mapUpdaterError(error: unknown): { message: string; severity: 'info' | 'warning' | 'critical'; raw?: string } {
  const raw = extractErrorIdentifierAndMessage(error);
  const entry = ERROR_MAP.find(
    (e) => e.identifier === raw.identifier || (e.pattern && e.pattern.test(raw.message))
  );
  if (entry) return { message: entry.userMessage, severity: entry.severity, raw: raw.message };
  return { message: raw.message || '알 수 없는 오류가 발생했습니다.', severity: 'warning', raw: raw.message };
}
```

### 4. 9-state machine 시각화 패턴 (Tailwind only)

```tsx
// UpdateStateMachineViz.tsx 의 핵심 렌더 패턴
const STATES: { key: OperationStatus; label: string; description: string }[] = [
  { key: 'starting', label: '시작', description: '업데이트 준비 중...' },
  { key: 'checking', label: '확인', description: '채널에서 최신 버전 조회 중...' },
  { key: 'downloading', label: '다운로드', description: '새 바이너리 다운로드 중...' },
  { key: 'verifying', label: '검증', description: '서명 및 체크섬 검증 중...' },
  { key: 'applying', label: '적용', description: '원자적 바이너리 교체 중...' },
  { key: 'ready_to_restart', label: '재시작 대기', description: '운영자 재시작 필요' },
  { key: 'completed', label: '완료', description: '업데이트 성공' },
];

// 각 상태는 다음 3가지 시각 상태로 렌더링:
// - completed (체크 표시 + 무채색)
// - active (강조색 + pulse animation 또는 ring)
// - pending (흐림 톤)
```

### 5. 모달 레이아웃 (TsdbDataViewerModal 패턴 재사용)

- 외곽 컨테이너: `fixed inset-0 z-50 bg-black/50 flex items-center justify-center`
- 모달 본체: `bg-white dark:bg-gray-900 w-[95vw] max-w-2xl h-auto max-h-[90vh] rounded-lg shadow-xl flex flex-col`
  - Update Dialog 는 95vh 가 아닌 적절한 max-height (Progress step 의 9-state 시각화 + 설명
    텍스트 + 버튼이 모두 보일 정도) 로 조정
- 헤더 (`shrink-0`): 제목 + step indicator + close button
- 본문 (`min-h-0 shrink overflow-y-auto`): step content
- 푸터 (`shrink-0`): next / back / close 버튼

---

## Task Decomposition (12 tasks, 권장 결정값 기준)

### Phase 1 — 핵심 인프라 (P0, blocker)

#### Task 1: API 훅 + 타입 정의 (TDD)

- **파일**: `web/src/services/api/systemUpdate.ts` (신규), `systemUpdate.test.ts` (신규)
- **작업**:
  - `VersionInfo`, `OperationStatus` (9종 enum), `UpdateOperation`, `CheckResult`, `ApplyRequest`,
    `RollbackResult` 타입 정의
  - `useSystemVersion` 훅 (60s 폴링, 30s staleTime, background false)
  - `useUpdateCheck` 뮤테이션 훅
  - `useUpdateApply` 뮤테이션 훅 (`ApplyRequest` 받아 `UpdateOperation` 반환)
  - `useUpdateStatus` 폴링 훅 (1s, terminal state 시 자동 중단)
  - `useUpdateRollback` 뮤테이션 훅
  - 단위 테스트: 각 훅의 happy path + 에러 경로 (msw 또는 fetch mock 사용)
- **우선순위**: P0
- **의존성**: 백엔드 SPEC-UPDATE-001 v0.1.0 머지

#### Task 2: 에러 매핑 모듈 (TDD)

- **파일**: `web/src/lib/errors/updaterErrorMapper.ts` (신규),
  `updaterErrorMapper.test.ts` (신규)
- **작업**:
  - 8종 에러 식별자 → 사용자 친화 메시지 매핑 (M12 표 참조)
  - severity 필드 (info / warning / critical) 추가
  - identifier 우선 매칭 + 메시지 패턴 fallback
  - 매핑 실패 시 원본 메시지 + GitHub Issues 링크 fallback
  - 단위 테스트: 8종 에러 + 매핑되지 않은 에러 + null/undefined 입력
- **우선순위**: P0
- **의존성**: Task 1

### Phase 2 — System Status Panel (P0)

#### Task 3: SystemStatusPanel 컴포넌트 (TDD)

- **파일**: `web/src/components/system/SystemStatusPanel.tsx` (신규),
  `SystemStatusPanel.test.tsx` (신규)
- **작업**:
  - `useSystemVersion` 훅 통합
  - 버전 정보 섹션 (version + commit + build_date + go_version + os/arch + channel)
  - "마지막 확인 시각" 표시 (TanStack Query `dataUpdatedAt` 활용, 로컬 형식)
  - 로딩 상태 (스켈레톤)
  - 에러 상태 (에러 배너 + 재시도 버튼)
  - "Check for updates" 버튼 → `useUpdateCheck` 뮤테이션 호출 + 진행 인디케이터
  - update_available 인디케이터 (배지 또는 강조 텍스트, M2)
  - 테스트: 정상 표시, 로딩, 에러, Check 버튼 동작, update_available 인디케이터 표시
- **우선순위**: P0
- **의존성**: Task 1

#### Task 4: 라우팅 / 위치 통합 (Decision Point 1)

- **파일**: Decision Point 1 결정에 따라:
  - 옵션 A: `web/src/pages/system/SystemStatusPage.tsx` (신규) + 라우팅 등록
  - 옵션 B: settings 컴포넌트 수정 + sub-tab 추가
  - 옵션 C: 헤더 컴포넌트 수정 + dropdown 추가
  - 옵션 D: 대시보드 컴포넌트 수정 + widget 추가
- **작업**:
  - SystemStatusPanel 컴포넌트를 결정된 위치에 mount
  - 권한 분기 (Decision Point 5, admin 전용 시 라우팅 가드)
  - 권장 옵션 A + C: `/admin/system` 라우트 + 헤더에 update_available 배지 (보완)
- **우선순위**: P0
- **의존성**: Task 3, **Decision Point 1 결정**

### Phase 3 — Update Dialog 다단계 모달 (P0)

#### Task 5: UpdateDialog 컨테이너 (TDD)

- **파일**: `web/src/components/system/UpdateDialog.tsx` (신규),
  `UpdateDialog.test.tsx` (신규)
- **작업**:
  - 모달 portal mount + `TsdbDataViewerModal` 레이아웃 패턴 재사용
  - 5단계 step 상태 관리 (`useState<'info' | 'confirm' | 'apply' | 'progress' | 'result'>`)
  - Esc / 외부 클릭 / X 버튼 닫기 핸들링 (단, **active operation 진행 중이면 백엔드 작업은
    계속됨을 사용자에게 명시하는 confirm 다이얼로그 표시**)
  - Step content placeholder (Task 6, 7, 8, 9 에서 구현)
  - 다음/이전 버튼 (Apply, Progress 단계에서는 비활성/숨김)
  - 테스트: 모달 열기/닫기, step 전환, Esc 처리, active operation 중 닫기 confirm
- **우선순위**: P0
- **의존성**: Task 1

#### Task 6: UpdateDialogInfoStep (TDD)

- **파일**: `web/src/components/system/UpdateDialogInfoStep.tsx` (신규),
  `UpdateDialogInfoStep.test.tsx` (신규)
- **작업**:
  - 현재 버전 → 타겟 버전 비교 (semver 시각화 — 큰 화살표 + 색상 강조)
  - 릴리즈 노트 링크 (`release_notes_url`, 새 탭)
  - 다운그레이드 감지 시 경고 배너 (semver 비교 유틸 필요 — `web/src/lib/semver.ts` 신규
    또는 기존 활용)
  - 다음 버튼 활성화
  - 테스트: 정상 비교, 다운그레이드 시 경고, 릴리즈 노트 링크 클릭
- **우선순위**: P1 (P0 와 비교 시 시각적 디테일이라 P1 으로 분류)
- **의존성**: Task 5

#### Task 7: UpdateDialogConfirmStep (TDD)

- **파일**: `web/src/components/system/UpdateDialogConfirmStep.tsx` (신규),
  `UpdateDialogConfirmStep.test.tsx` (신규)
- **작업**:
  - 다운그레이드 시 `--force` 체크박스 (M9)
  - 일반 적용 시 단순 confirm 메시지
  - 다음 버튼 ("적용") — 다운그레이드 시 force 체크박스 활성 시에만 활성화
  - 테스트: 정상 confirm, 다운그레이드 force 체크박스 동작, 비활성 상태 검증
- **우선순위**: P1
- **의존성**: Task 5, Task 6

#### Task 8: UpdateDialogProgressStep + UpdateStateMachineViz (TDD, P0)

- **파일**:
  - `web/src/components/system/UpdateDialogProgressStep.tsx` (신규)
  - `web/src/components/system/UpdateStateMachineViz.tsx` (신규)
  - 각 테스트 파일
- **작업**:
  - `useUpdateApply` 뮤테이션 호출 → `operation_id` 수신
  - `useUpdateStatus` 1초 폴링 (M6)
  - `UpdateStateMachineViz` — 9-state 시각화 (좌→우 또는 위→아래 단계 흐름)
  - `progress_percent` 가 있을 때 progress bar 추가 (M6 — downloading 단계)
  - 각 상태에 친화 설명 텍스트
  - terminal state 도달 시 (`completed`, `failed`, `ready_to_restart`) 자동으로 Result step
    으로 전환
  - 테스트: 9-state 모든 상태 시각화, 폴링 동작, terminal state 시 자동 전환
- **우선순위**: P0
- **의존성**: Task 1, Task 5

#### Task 9: UpdateDialogResultStep (TDD, P0)

- **파일**: `web/src/components/system/UpdateDialogResultStep.tsx` (신규),
  `UpdateDialogResultStep.test.tsx` (신규)
- **작업**:
  - 분기 1 (`status == 'completed'`): 성공 메시지 + 닫기 버튼
  - 분기 2 (`status == 'ready_to_restart'`): 운영자 안내 (M8 — Decision Point 3 결정에 따라 옵션
    B + C 권장: CLI 명령어 + 클립보드 복사 + future 노트)
  - 분기 3 (`status == 'failed'`): 실패 사유 표시 (`updaterErrorMapper` 매핑) + "Rollback 시도"
    버튼 + `<binary>.previous` 위치 안내
  - Rollback 버튼 클릭 시 `useUpdateRollback` 호출 + 결과 표시 (성공 / `ErrUpdateRollbackFailed`)
  - OS 감지 (navigator.platform 또는 백엔드 응답의 `os` 필드 활용) → CLI 명령어 분기
  - 테스트: 3가지 분기 시나리오, rollback 호출, rollback 실패 시 critical 메시지
- **우선순위**: P0
- **의존성**: Task 1, Task 5, Task 8

### Phase 4 — 알림 + 통합 (P1)

#### Task 10: UpdateAvailableBadge + 알림 (Decision Point 2)

- **파일**: `web/src/components/system/UpdateAvailableBadge.tsx` (신규),
  Decision Point 2 결정에 따라 toast 통합 (기존 toast 시스템 재사용)
- **작업**:
  - 배지 컴포넌트 (작은 dot 또는 텍스트 배지)
  - `update_available` 이 false → true 전이 감지 (이전 값과 비교)
  - 권장 옵션 C (toast + badge): toast 한 번 표시 (사용자 dismiss 또는 자동 사라짐) + badge
    지속 표시
  - Decision Point 5 결정에 따라 권한 없는 사용자에게 알림 노출 차단
  - 테스트: 배지 렌더링, 전이 감지, dismiss 후 재표시 안 함
- **우선순위**: P1
- **의존성**: Task 3, **Decision Point 2 결정**

#### Task 11: 채널 정보 표시 (Decision Point 4)

- **파일**: `SystemStatusPanel.tsx` 수정
- **작업**:
  - 권장 옵션 C (읽기 전용): 현재 채널 표시 + "채널 변경은 CLI 에서: `xflowd update channel
    <name>`" 도움말
  - 옵션 A 채택 시 (백엔드 SPEC-UPDATE-002 후): 채널 셀렉터 + `PUT /system/update/channel` 호출
    (본 SPEC v0.2.0 으로 분리)
  - 테스트: 도움말 텍스트 표시, 채널 표시 동작
- **우선순위**: P1
- **의존성**: Task 3, **Decision Point 4 결정**

#### Task 12: 통합 테스트 + 시나리오 검증

- **파일**: `web/src/components/system/__integration__/SystemStatusFlow.test.tsx` (신규)
- **작업**:
  - acceptance.md 의 12개 시나리오 중 자동화 가능한 항목 통합 테스트로 작성
  - msw (mock service worker) 로 백엔드 응답 시뮬레이션
  - 시나리오 4 (Apply 해피 패스): info → confirm → apply → progress → result 전체 흐름
  - 시나리오 5 (Apply 실패): signature_invalid → rollback 흐름
  - 시나리오 11 (다이얼로그 닫기 후 재오픈): operation_id 보존 검증
  - 시나리오 12 (동시 apply 409): ErrUpdateInProgress 매핑 검증
- **우선순위**: P1
- **의존성**: Task 1~10

---

## 마일스톤

### Primary Goal: 핵심 인프라 + 정상 흐름 (P0)

Task 1-5, 8, 9 완료 시 운영자가 다음을 수행 가능:

- System Status Panel 에서 현재 버전/채널 확인
- "Check for updates" 클릭 → 업데이트 가능 여부 확인
- "Update available" 클릭 → Update Dialog 열림
- Apply 흐름 진행 → 9-state machine 시각화 → ready_to_restart 또는 completed 결과
- 실패 시 명시적 rollback 버튼 노출

### Secondary Goal: 운영자 가이드 + UX 디테일 (P1)

Task 6, 7, 10, 11 완료 시:

- Info step 에서 다운그레이드 감지 + 경고
- Confirm step 에서 force 체크박스 + 명시적 동의
- update_available 알림 (toast + badge)
- 채널 정보 + CLI 안내

### Final Goal: 통합 테스트 + 품질 (P2)

Task 12 + 디자인 자료 (옵션) 완료 시:

- acceptance.md 모든 시나리오의 자동/수동 검증 통과
- 테스트 커버리지 85%+ 신규 컴포넌트
- 디자인 자료 분리 (`references/design/system-status-panel.pen`,
  `update-dialog.pen`)

---

## 리스크 및 완화

| 리스크 | 영향 | 완화 |
|--------|------|------|
| **백엔드 SPEC-UPDATE-001 v0.1.0 머지 지연** | 본 SPEC 작업 시작 차단 | (1) 백엔드 PR 우선 머지. (2) 백엔드 미머지 시 mock 응답 기반 frontend 단독 구현 후 백엔드 머지 시점에 검증. |
| **Decision Point 미결정 → 작업 지연** | 5개 결정 항목이 결정되어야 진행 가능 | (1) 권장 결정값으로 선제 진행 후 사용자 검토 시 변경. (2) 모든 Decision Point 를 단일 AskUserQuestion 흐름으로 묶어 한 번에 수집. |
| **9-state machine 동시성 race** (UI 가 polling 중 status 변경) | UI 가 stale state 표시 가능 | (1) `refetchInterval` 로 1초 폴링하여 lag 1초 이내 보장. (2) terminal state 도달 시 `enabled: false` 로 폴링 자동 중단. (3) `dataUpdatedAt` 표시로 사용자에게 마지막 갱신 시점 노출. |
| **Active operation 중 다이얼로그 닫기 → 재오픈 시 진행 상태 누락** | 사용자 혼란 | (1) UpdateDialog 가 mount 시 `useUpdateStatus` 를 호출하여 진행 중 operation 자동 감지. (2) `status != 'idle'` 이면 자동으로 Progress step 으로 진입. (3) acceptance.md 시나리오 11 로 검증. |
| **다른 운영자가 동시 apply 실행 → 409 충돌** | 두 번째 호출자 혼란 | (1) `ErrUpdateInProgress` 매핑 (M12). (2) 두 번째 호출자에게 status 폴링으로 진행 상황 안내 toast. (3) acceptance.md 시나리오 12 로 검증. |
| **권한 없는 사용자가 UI 만 우회 시도** | 보안 우려 | (1) M11 에 명시 — 클라이언트 측 권한 검사만으로 보안 보장 안 함. (2) 백엔드 401/403 응답이 최종 권한 게이트. (3) 보안 침해 시도는 백엔드 audit 로그에 기록. |
| **`ready_to_restart` 상태에서 운영자가 재시작 누락** | 새 바이너리 적용 안 됨 | (1) M8 운영자 가이드 — CLI 명령어 + 클립보드 복사 + future 노트. (2) Status Panel 의 다음 폴링에서 여전히 이전 버전 표시 → 재시작 필요 알림 inline 메시지 제공 (`commit` 비교로 감지 가능). |
| **rollback 시도 시 백업 파일 손상 또는 부재** | 운영 중단 위험 | (1) `ErrUpdateRollbackFailed` 매핑 — critical 등급 메시지. (2) `<binary>.previous` 위치 명시 + 운영자 수동 개입 안내. (3) acceptance.md 시나리오 8 로 검증. |
| **신규 외부 라이브러리 (recharts 등) 도입 유혹** | 의존성 증가 | (1) 9-state 시각화는 Tailwind only 로 충분. (2) progress bar 도 Tailwind only. (3) 디자인 시스템 일관성 확보. |

---

## 테스트 전략

### 개발 방법론: TDD (신규 컴포넌트)

본 SPEC 의 모든 작업은 신규 컴포넌트/모듈 작성이므로 **TDD (RED-GREEN-REFACTOR)** 가 자연스럽다.

- 각 Task 의 단위 테스트는 구현 전 작성
- 통합 테스트 (Task 12) 는 acceptance.md 시나리오 기반

### 도구

- **Vitest** (단위 테스트, 통합 테스트)
- **React Testing Library** (`render`, `screen`, `fireEvent`, `userEvent`)
- **`@testing-library/user-event`** (실제 사용자 동작 시뮬레이션)
- **msw (Mock Service Worker)** (백엔드 응답 모의, 통합 테스트용 — 기존 프로젝트에 도입 여부
  확인 필요; 미도입 시 `vi.fn()` + fetch mock 사용)

### 커버리지 목표

| 모듈 | 목표 |
|------|------|
| `web/src/services/api/systemUpdate.ts` | 90%+ |
| `web/src/lib/errors/updaterErrorMapper.ts` | 100% (8종 에러 + fallback) |
| `web/src/components/system/SystemStatusPanel.tsx` | 85%+ |
| `web/src/components/system/UpdateDialog.tsx` | 85%+ |
| `web/src/components/system/UpdateDialog*Step.tsx` | 85%+ each |
| `web/src/components/system/UpdateStateMachineViz.tsx` | 90%+ (시각화 단순) |
| `web/src/components/system/UpdateAvailableBadge.tsx` | 85%+ |

### TRUST 5 정렬

- **Tested**: 85%+ 라인 커버리지, 신규 작성이므로 characterization 불필요
- **Readable**: 5단계 step 컴포넌트 명확 분리, prop 타입 명확
- **Unified**: 기존 `TsdbDataViewerModal`, `ConfirmDialog`, `MetadataChips`,
  `storeErrorMapper` 패턴 재사용
- **Secured**: M11 — 클라이언트 권한 검사 의존하지 않음, 백엔드 401/403 게이트 신뢰. force flag
  UI 의 명시적 confirm 으로 운영 사고 예방. 디지털 서명 검증 실패 시 critical 등급 메시지 + 운영팀
  알림 안내.
- **Trackable**: Conventional commits, SPEC-WEB-006 참조, TAG `@SPEC:SPEC-WEB-006` 인덱싱

---

## API 사용 예시 (참고)

### Status Panel 폴링

```
GET /api/v1/system/version (60s 간격)
응답:
{
  "success": true,
  "data": {
    "version": "v0.3.0",
    "commit": "6b9c531",
    "build_date": "2026-05-04T12:34:56Z",
    "go_version": "go1.22.5",
    "os": "linux",
    "arch": "amd64",
    "channel": "stable",
    "update_available": true,
    "latest_version": "v0.4.0"
  }
}
```

### Check 트리거

```
POST /api/v1/system/update/check
응답:
{
  "success": true,
  "data": {
    "current_version": "v0.3.0",
    "latest_version": "v0.4.0",
    "update_available": true,
    "channel": "stable",
    "release_notes_url": "https://github.com/xtra72/xflow/releases/tag/v0.4.0",
    "published_at": "2026-05-05T08:00:00Z",
    "asset_size_bytes": 25165824
  }
}
```

### Apply (다운그레이드 force)

```
POST /api/v1/system/update/apply
요청:
{
  "version": "v0.2.0",
  "force": true,
  "skip_confirm": true
}
응답:
{
  "success": true,
  "data": {
    "operation_id": "upd-2026-05-06-001",
    "status": "starting",
    "from_version": "v0.3.0",
    "to_version": "v0.2.0",
    "started_at": "2026-05-06T10:00:00Z"
  }
}
```

### Status 폴링 (1초 간격)

```
GET /api/v1/system/update/status
응답 (downloading 단계):
{
  "success": true,
  "data": {
    "operation_id": "upd-2026-05-06-001",
    "status": "downloading",
    "progress_percent": 45,
    "from_version": "v0.3.0",
    "to_version": "v0.4.0",
    "started_at": "2026-05-06T10:00:00Z",
    "completed_at": null,
    "error": null
  }
}
```

### Rollback (실패 후)

```
POST /api/v1/system/update/rollback
응답 (성공):
{
  "success": true,
  "data": {
    "success": true,
    "from_version": "v0.4.0",
    "to_version": "v0.3.0",
    "error": null
  }
}

응답 (백업 파일 부재):
{
  "success": false,
  "error": {
    "code": "ErrUpdateRollbackFailed",
    "message": "backup file not found at /usr/local/bin/xflowd.previous"
  }
}
```

---

## 통계 (예상)

- 변경 파일: **약 14-17개 신규 + 1-3개 수정** (Decision Point 1 옵션에 따라)
- 신규 테스트: **약 30-40개** (단위 + 통합)
- 전체 테스트 영향: 기존 ~520 (SPEC-WEB-005 v0.7.0 후) + 신규 30-40 = **~550-560**
- TypeScript: strict 통과 유지
- 신규 외부 라이브러리: **0**
- 작업 단위: 권장 결정값 기준 약 **12 tasks (1.5-2 PR 단위)**

---

## Status: draft (Level 1 spec-first lifecycle, v0.1.0 initial draft)
