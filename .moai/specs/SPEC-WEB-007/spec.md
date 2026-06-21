---
id: SPEC-WEB-007
title: 로컬 인스턴스 시스템 정보 표출 (Self / Local Instance System Info Display)
version: 0.2.0
status: draft
created: 2026-06-21
updated: 2026-06-21
author: xtra
priority: medium
---

# SPEC-WEB-007: 로컬 인스턴스 시스템 정보 표출

## HISTORY

- **0.2.0** (2026-06-21): Decision Points 확정(RESOLVED). 1) Identity 데이터 소스 = 기존
  `GET /api/v1/system/version` 확장(`os/arch/hostname/mode/uptime_seconds` 추가), 신규
  `/system/info` 미생성. 2) Update 상태 카드 = 기존 `SystemVersionCard` 유지·공존. 3) Runtime
  폴링 = 5초. 4) 통합 방식 = 별도 페이지/탭 신설 없이 기존 `/admin/system` 페이지에 "시스템 정보"
  영역 추가. 본문 요구사항(M5 등)을 `system/version` 확장으로 단정 기술하고, Open Questions / 미해결
  Decision Point 표기를 제거.
- **0.1.0** (2026-06-21): 최초 작성 — 접속한 xflowd 인스턴스 **자신(self / local)** 의 시스템 정보를
  그 인스턴스의 Web UI 에 표시한다. 관리 서버가 원격 노드로부터 보고받아 표시하는 정보
  (NodeDashboard, REMOTE-001) 는 이미 동작하며 **본 SPEC 의 범위가 아니다**. 본 SPEC 의 GAP 은
  "로그인한 인스턴스 자신의 OS/Arch/hostname/uptime/실시간 리소스/remote 모드가 그 인스턴스의 UI 에
  제대로 표시되지 않는다" 는 점이다. 기존 `/admin/system` 페이지(SPEC-WEB-006)를 확장하여
  Identity + Runtime + (선택) Update 정보를 단일 "시스템 정보" 영역으로 표출한다. 백엔드는 최소
  추가(os/arch/hostname/mode/uptime)만 수행하고, 실시간 리소스는 기존 `GET /monitor/metrics` 를
  재사용한다. SELF 케이스이므로 프로토콜 변경은 없다.

| Version | Date       | Author | Change                                                              |
| ------- | ---------- | ------ | ------------------------------------------------------------------- |
| 0.2.0   | 2026-06-21 | xtra   | Decision Points 확정 — version 확장 / 카드 공존 / 5s 폴링 / 페이지 통합 |
| 0.1.0   | 2026-06-21 | xtra   | 최초 작성 — self/local 인스턴스 시스템 정보 표출 (Identity + Runtime) |

---

## 개요 (Overview)

### 목적

xflowd 운영자가 **현재 접속한 인스턴스 자신**의 시스템 정보(아이덴티티 + 런타임)를 그 인스턴스의
Web UI 에서 한눈에 확인할 수 있도록 한다. 관리 서버에 로그인하든, standalone/client 인스턴스에
로그인하든, **그 인스턴스 자신**의 hostname / OS / Arch / 버전 / remote 모드 / uptime / 실시간
리소스를 표시한다.

### 배경 (현재 상태 — 코드 근거)

| 사실 | 근거 (file:line) |
| --- | --- |
| 관리 서버는 **원격 노드**가 보고한 정보(hostname/OS/arch/version/uptime/online)를 NodeDashboard 에 이미 표시 (동작함, 범위 외) | `internal/remote/protocol.go:355-359, 392-399` (Hostname/OS/Arch/StartedAt 보고) |
| 로컬 "시스템 상태" 페이지(`/admin/system`, admin 전용)는 **버전 + 업데이트 상태만** 표시. OS/Arch/hostname/uptime/실시간 리소스 없음 | `web/src/pages/system/SystemStatusPage.tsx:37-142` |
| `GET /api/v1/system/version` 응답은 `version, commit, build_date, go_version, channel, update_available, latest_version` 만 포함. **os/arch/hostname/mode/uptime 없음** | `internal/api/dto/update.go:16-33`, `internal/api/handler/system_update.go:897-902` |
| `GET /api/v1/monitor/metrics` 는 실시간 `cpu_usage_percent, memory_usage_percent, go_routines, go_mem_alloc_mb, go_mem_sys_mb, uptime_seconds` 반환. 그러나 **시스템 페이지에 렌더링되지 않음** | `internal/api/handler/monitor.go:33-40, 95-104, 221-236` |
| 인스턴스는 자신의 값을 로컬에서 알 수 있음 (`runtime.GOOS/GOARCH`, `os.Hostname()`, ldflags 빌드 메타, 프로세스 시작 시각) — **단일 LOCAL 엔드포인트가 os/arch/hostname 을 노출하지 않을 뿐** | `internal/api/handler/monitor.go:8(runtime), 222-226` |
| `remote_management.mode` 는 config 에서 조회 가능 (`server/client/disabled`) | `internal/config/defaults.go:85`, `internal/config/config.go:476` |
| 프론트엔드에는 이미 `monitorService.getMetrics()` + `ResourceWidget` 가 존재 (metrics 가 이미 프론트로 배선됨, 단 시스템 페이지엔 미사용) | `web/src/services/api/monitorService.ts:6-7`, `web/src/pages/dashboard/widgets/ResourceWidget.tsx` |

### 범위

- **포함 (SELF / LOCAL 인스턴스 정보)**:
  - **Identity 표시**: hostname, OS, architecture(GOOS/GOARCH), xflowd 버전 + 빌드 메타
    (commit, build_date, go_version), `remote_management.mode` (server/client/disabled)
  - **Runtime 표시**: 프로세스 uptime, 실시간 CPU %, 메모리 %, goroutines, Go heap(alloc/sys) —
    기존 `GET /monitor/metrics` 폴링 재사용
  - **Update 상태 (선택, 이미 존재)**: channel + update_available + latest_version
    (`GET /system/version` — 기존 `SystemVersionCard` 가 표시, 본 SPEC 의 시스템 정보 영역과 공존)
  - **데이터 소스 (확정)**: 기존 `GET /api/v1/system/version` 을 확장하여
    `os/arch/hostname/mode/uptime_seconds` 를 추가한다 (신규 `/system/info` 엔드포인트는 만들지
    않는다). 실시간 리소스(CPU/메모리/goroutines/heap)는 기존 `GET /monitor/metrics` 를 재사용한다.
  - **UI (확정)**: 별도 페이지/탭을 신설하지 않고 기존 `/admin/system` 페이지(`SystemStatusPage`)에
    "시스템 정보" 영역을 추가한다. remote 모드와 무관하게 동작, 기존 페이지와 동일한 admin 게이팅
- **제외**:
  - **원격 노드 정보 표시** — 관리 서버가 원격 노드의 보고를 표시하는 흐름(NodeDashboard,
    REMOTE-001)은 **이미 동작하며 본 SPEC 범위 외**. 본 SPEC 은 오로지 SELF/로컬 인스턴스 정보
  - **시크릿 노출** — jwt_secret, keys, credentials, bootstrap_secret 등은 **절대 표시하지 않음**
  - **버전/업데이트 적용 흐름** — SPEC-WEB-006 (9-state Update Dialog) 소관, 본 SPEC 은 표시만
  - **CPU 사용률 정밀 측정** — 백엔드 `monitor.go:229` 는 v1 에서 CPU 를 0 으로 반환(샘플링 미지원).
    정밀 CPU 샘플링은 별도 OBS SPEC 의 후속 작업(본 SPEC 은 존재하는 필드를 그대로 표시)
  - **시스템 정보 영속화 / 시계열 저장**

### 가정

- **인증/권한**: 본 정보 영역은 기존 `/admin/system` 페이지와 동일하게 admin 게이팅된다
  (`SystemStatusPage.tsx:47` 의 `isAdmin` 패턴 재사용). 실제 권한 검증은 백엔드가 수행한다.
- **타임스탬프 규약**: 프로세스 시작 시각/uptime 관련 내부 상태는 epoch ms 로 관리한다
  (`project_timestamp_convention.md` 준수). `uptime_seconds` 는 백엔드가 이미 초 단위로 제공한다.
- **모드 무관 동작**: 본 페이지는 `remote_management.mode` 게이팅을 받지 않는다 — 관리 서버,
  client 노드, standalone 모든 인스턴스에서 동작해야 한다.
- **시크릿 비노출**: 본 SPEC 이 추가/표시하는 어떤 필드도 자격 증명/키/시크릿을 포함하지 않는다.

### 의존성

- **백엔드 (확장 대상 — 확정)**:
  - `internal/api/dto/update.go` `VersionResponse` 에 `os/arch/hostname/mode/uptime_seconds`
    필드 추가
  - `internal/api/handler/system_update.go` `GetVersion`/`UpdateService.Version()` 에서 위 필드
    채움 (신규 `/system/info` 핸들러는 만들지 않는다)
- **백엔드 (재사용, 변경 없음)**:
  - `internal/api/handler/monitor.go` `GET /monitor/metrics` (실시간 리소스)
  - `internal/config` `remote_management.mode` 조회 (`config.go:476`)
- **프론트엔드 (수정 대상)**:
  - `web/src/pages/system/SystemStatusPage.tsx` (시스템 정보 영역 추가)
  - `web/src/services/api/systemUpdate.ts` `VersionInfo` 타입 확장
    (`os/arch/hostname/mode/uptime_seconds`)
  - `web/src/services/api/monitorService.ts` `getMetrics()` 타입 강화 (현재 `Record<string,unknown>`)
- **참조 패턴 (재사용)**:
  - `SystemVersionCard` (SPEC-WEB-006) — 버전/빌드 칩 표시 패턴
  - `ResourceWidget` (대시보드) — `/monitor/metrics` 소비 + 실시간 표시 패턴
  - `MetadataChips` (SPEC-WEB-005) — 무채색 톤 + monospaced 칩

---

## EARS 요구사항 (M1-M9)

### M1: 시스템 정보 영역 — 노출 위치

- **Ubiquitous**: 시스템은 로컬 인스턴스의 시스템 정보를 `/admin/system` 페이지("시스템 상태")의
  "시스템 정보(System Info)" 영역에 항상 표출해야 한다.
- **Ubiquitous**: 시스템 정보 영역은 `remote_management.mode` 값과 무관하게(server/client/disabled
  모두) 표시되어야 한다.
- **State-driven**: WHILE 인증되지 않았거나 admin 권한이 없는 사용자가 접근하면, 시스템은 기존
  페이지와 동일한 정책(로그인 리다이렉트 또는 401/403)으로 처리해야 한다.

### M2: Identity 정보 표시

- **Ubiquitous**: 시스템 정보 영역은 다음 **아이덴티티** 정보를 항상 표시해야 한다:
  - **hostname** (`os.Hostname()` 결과, 예: `xflow-node-01`)
  - **OS / Arch** (`runtime.GOOS` / `runtime.GOARCH`, 예: `linux/amd64`)
  - **xflowd 버전** (`version`, 예: `v0.18.6`)
  - **빌드 커밋** (`commit`, 단축 해시)
  - **빌드 일시** (`build_date`, 브라우저 로컬 형식)
  - **Go 버전** (`go_version`, 예: `go1.25.0`)
  - **remote 모드** (`mode`, `server | client | disabled` — 사용자 친화 라벨 매핑)
- **Event-driven**: WHEN 확장된 `GET /api/v1/system/version` 이 응답을 반환하면, THEN 시스템은 위
  필드를 즉시 렌더링해야 한다.
- **Optional**: WHERE 운영자가 commit 또는 build_date 를 클릭하면, 시스템은 GitHub commit URL
  (`https://github.com/xtra72/xflow/commit/{commit}`)을 새 탭으로 열 수 있다.

### M3: Runtime(실시간 리소스) 정보 표시

- **Ubiquitous**: 시스템 정보 영역은 다음 **런타임** 정보를 표시해야 한다:
  - **uptime** (`uptime_seconds` → "Xd Xh Xm" 가독 형식)
  - **CPU %** (`cpu_usage_percent`)
  - **메모리 %** (`memory_usage_percent`)
  - **goroutines** (`go_routines`)
  - **Go heap** (`go_mem_alloc_mb` / `go_mem_sys_mb`, MB 단위)
- **Ubiquitous**: 런타임 정보는 기존 `GET /api/v1/monitor/metrics` 를 데이터 소스로 사용해야 한다
  (신규 백엔드 추가 없이 재사용).
- **State-driven**: WHILE `cpu_usage_percent == 0` (백엔드 v1 미지원)이면, 시스템은 "측정 미지원"
  또는 보조 안내를 표시하여 0% 가 오인되지 않도록 해야 한다.

### M4: Runtime 폴링

- **Ubiquitous**: 시스템은 TanStack Query 훅으로 `GET /monitor/metrics` 를 5초 간격으로 폴링해야
  한다 (`refetchInterval: 5000` — 대시보드 `ResourceWidget` 과 일관).
- **State-driven**: WHILE 사용자가 다른 탭/페이지로 전환하면, 시스템은 백그라운드 폴링을 일시
  중지해야 한다 (`refetchIntervalInBackground: false`).
- **Unwanted**: 시스템은 1초보다 짧은 간격으로 metrics 를 자동 폴링하지 않아야 한다.

### M5: 데이터 소스 — Identity 백엔드 확장 (확정)

- **Ubiquitous**: 시스템은 Identity 데이터를 기존 `GET /api/v1/system/version` 을 확장하여 제공해야
  한다. 신규 `GET /api/v1/system/info` 엔드포인트는 만들지 않는다.
- **Ubiquitous**: 확장된 `GET /api/v1/system/version` 응답은 기존 필드(version/commit/build_date/
  go_version/channel/update_available/latest_version)에 더해 다음을 추가로 반환해야 한다:
  `os` (`runtime.GOOS`), `arch` (`runtime.GOARCH`), `hostname` (`os.Hostname()`), `mode`
  (`remote_management.mode`), `uptime_seconds` (프로세스 uptime, 초).
- **Ubiquitous**: 기존 7개 필드의 의미/형식은 변경되지 않아야 한다 (SPEC-WEB-006 `SystemVersionCard`
  회귀 방지).
- **Unwanted**: 확장된 엔드포인트는 어떤 자격 증명/키/시크릿(jwt_secret, bootstrap_secret,
  enrollment_token 등)도 반환하지 않아야 한다.

### M6: 보안 / 시크릿 비노출

- **Unwanted**: 시스템은 자기 자신의 시스템 정보 표출 과정에서 jwt_secret, TLS 키, bootstrap_secret,
  enrollment_token, server_url 의 인증 토큰 등 어떤 비밀 값도 응답 또는 UI 에 노출하지 않아야 한다.
- **Ubiquitous**: 시스템 정보 영역은 기존 `/admin/system` 페이지와 동일한 admin 게이팅 정책을
  따라야 한다.
- **Unwanted**: 시스템은 클라이언트 측 권한 검사만으로 보안을 보장하지 않아야 한다 (백엔드의
  401/403 이 최종 권한 게이트).

### M7: 로딩 / 에러 / 빈 상태

- **State-driven**: WHILE Identity 또는 Runtime 데이터가 로딩 중이면, 시스템은 스켈레톤/플레이스
  홀더를 표시해야 한다 (기존 `LoadingSkeleton` 패턴 재사용).
- **State-driven**: WHILE 데이터 조회가 실패하면, 시스템은 해당 영역에 명시적 에러 + "다시 시도"
  를 표시해야 한다 (기존 `ErrorState` 패턴 재사용). 한 영역의 실패가 다른 영역 렌더링을 막지
  않아야 한다 (Identity 와 Runtime 은 독립).

### M8: SELF vs REMOTE 명시적 구분

- **Ubiquitous**: 시스템 정보 영역은 표시 대상이 **현재 접속한 인스턴스 자신**임을 명확히 라벨링
  해야 한다 (예: "이 인스턴스 (self)" 또는 hostname 강조). 관리 서버에서 보는 원격 노드 목록
  (NodeDashboard)과 혼동되지 않아야 한다.
- **Ubiquitous**: 본 SPEC 은 SELF 케이스이므로 어떤 원격 프로토콜(REMOTE-001 heartbeat)도 변경
  하거나 의존하지 않아야 한다 (모든 데이터는 로컬 소스).

### M9: 표시 형식 / 가독성

- **Ubiquitous**: 버전/커밋/hostname/OS/Arch 는 monospaced 폰트 칩으로 표시해야 한다
  (`MetadataChips` 패턴).
- **Ubiquitous**: uptime 은 초가 아니라 가독 형식("3d 4h 12m")으로, 메모리/CPU 는 소수점 1자리
  + `%`/`MB` 단위로 표시해야 한다.
- **Optional**: WHERE remote 모드가 `server` 이면, 영역 하단에 "원격 노드 목록은 대시보드의 노드
  뷰에서 확인하세요" 보조 링크를 표시할 수 있다 (SELF vs REMOTE 안내, REMOTE-001 참조).

---

## 명세 (Specifications)

### 백엔드 확장 (확정 — VersionResponse 확장)

```go
// internal/api/dto/update.go — VersionResponse 에 추가 (SELF 시스템 정보)
type VersionResponse struct {
    Version         string `json:"version"`
    Commit          string `json:"commit"`
    BuildDate       string `json:"build_date"`
    GoVersion       string `json:"go_version"`
    Channel         string `json:"channel"`
    UpdateAvailable bool   `json:"update_available"`
    LatestVersion   string `json:"latest_version,omitempty"`
    // --- SPEC-WEB-007 추가 (SELF identity + uptime) ---
    OS            string  `json:"os"`             // runtime.GOOS
    Arch          string  `json:"arch"`           // runtime.GOARCH
    Hostname      string  `json:"hostname"`       // os.Hostname()
    Mode          string  `json:"mode"`           // remote_management.mode: server|client|disabled
    UptimeSeconds float64 `json:"uptime_seconds"` // 프로세스 uptime(초)
}
```

### 프론트엔드 타입

```typescript
// web/src/services/api/systemUpdate.ts — VersionInfo 확장 (확정)
export interface VersionInfo {
  version: string;
  commit: string;
  build_date: string;
  go_version: string;
  channel: Channel;
  update_available: boolean;
  latest_version: string | null;
  // --- SPEC-WEB-007 추가 ---
  os: string;             // 예: "linux"
  arch: string;           // 예: "amd64"
  hostname: string;       // 예: "xflow-node-01"
  mode: 'server' | 'client' | 'disabled';
  uptime_seconds: number; // 프로세스 uptime(초)
}

// web/src/services/api/monitorService.ts — getMetrics 타입 강화
export interface SystemMetrics {
  cpu_usage_percent: number;
  memory_usage_percent: number;
  go_routines: number;
  go_mem_alloc_mb: number;
  go_mem_sys_mb: number;
  uptime_seconds: number;
}
export function getMetrics(): Promise<SystemMetrics>;
```

### 컴포넌트 구조 (예정)

```
web/src/
├── pages/system/SystemStatusPage.tsx          # (수정) "시스템 정보" 영역 추가
├── components/system/
│   ├── SystemInfoCard.tsx                      # (신규) Identity 표시
│   └── SystemRuntimeCard.tsx                   # (신규) Runtime(metrics) 표시
└── services/api/
    ├── systemUpdate.ts                         # (수정) VersionInfo 확장
    └── monitorService.ts                       # (수정) SystemMetrics 타입 + useSystemMetrics 훅
```

### 모드 라벨 매핑

| `mode` 값 | 한글 라벨 |
| --- | --- |
| `server` | 관리 서버 |
| `client` | 클라이언트 노드 |
| `disabled` | 독립 실행 (standalone) |

---

## Decision Points (확정 — RESOLVED)

모든 Decision Point 가 사용자 승인으로 확정되었다. 추가 결정 대기 항목 없음.

| # | 결정 항목 | 확정 값 | 비고 |
| - | --- | --- | --- |
| 1 | Identity 데이터 소스 | **A: 기존 `GET /api/v1/system/version` 확장** (`os/arch/hostname/mode/uptime_seconds` 추가) | 신규 `/system/info` 미생성. 프론트 단일 호출 |
| 2 | Update 상태(channel/update_available) 표시 | **B: 기존 `SystemVersionCard` 유지·공존** | 신규 시스템 정보 영역과 같은 `/admin/system` 페이지에 병존 |
| 3 | Runtime 폴링 주기 | **5초** (`refetchInterval: 5000`) | 대시보드 `ResourceWidget` 과 일관 |
| 4 | 통합 방식 | **기존 `/admin/system` 페이지에 "시스템 정보" 영역 추가** | 별도 페이지/탭 신설하지 않음 |

---

## TAG Traceability

- `@SPEC:SPEC-WEB-007` → spec.md (이 문서)
- `@PLAN:SPEC-WEB-007` → plan.md
- `@ACCEPTANCE:SPEC-WEB-007` → acceptance.md
- 구현 경로 (예정):
  - `internal/api/dto/update.go` (수정 — `VersionResponse` 에 os/arch/hostname/mode/uptime_seconds 추가)
  - `internal/api/handler/system_update.go` (`UpdateService.Version()` 에서 위 필드 채움)
  - `web/src/services/api/systemUpdate.ts` (수정 — VersionInfo 확장)
  - `web/src/services/api/monitorService.ts` (수정 — SystemMetrics 타입 + useSystemMetrics 훅)
  - `web/src/components/system/SystemInfoCard.tsx` (신규)
  - `web/src/components/system/SystemRuntimeCard.tsx` (신규)
  - `web/src/pages/system/SystemStatusPage.tsx` (수정 — 영역 마운트)

---

## 관련 SPEC

- **SPEC-WEB-006 v0.1.0** (확장 대상 / 공존): `/admin/system` 페이지 + `SystemVersionCard` +
  Update Dialog. 본 SPEC 은 같은 페이지에 SELF 시스템 정보(Identity + Runtime)를 **추가**한다.
  버전/업데이트 적용 흐름은 WEB-006 소관으로 유지한다.
- **SPEC-REMOTE-001** (대조, 범위 외): 원격 노드가 hostname/OS/arch/started_at 을 관리 서버에
  보고하는 흐름 (이미 동작, NodeDashboard 표시). 본 SPEC 은 그 **반대편(SELF/로컬)** 케이스.
- **SPEC-UPDATE-001 v0.1.0** (참조): `GET /system/version` + 빌드 메타(version/commit/build_date/
  go_version) 출처.
- **SPEC-OBS-001** (참조): `/monitor/metrics` 메트릭 시스템 출처 (CPU/mem/goroutines/uptime).
- **SPEC-API-001** (참조): REST 응답 envelope + 인증 미들웨어.

---

## 알려진 제약

| 제약 | 비고 |
| --- | --- |
| CPU 사용률은 백엔드 v1 에서 0 반환 (`monitor.go:229`, 샘플링 미지원) | M3 에서 "측정 미지원" 안내로 보완. 정밀 CPU 는 별도 OBS 후속 |
| 원격 노드 정보 표시는 범위 외 | NodeDashboard / REMOTE-001 이 이미 담당 |

---

## Status: draft (Level 1 spec-first lifecycle, v0.2.0 — Decision Points 확정 완료)
