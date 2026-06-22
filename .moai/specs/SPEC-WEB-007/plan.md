---
id: SPEC-WEB-007
title: 로컬 인스턴스 시스템 정보 표출 — 구현 계획
version: 0.2.0
status: completed
created: 2026-06-21
updated: 2026-06-22
author: xtra
---

# SPEC-WEB-007 구현 계획 (Plan)

`@PLAN:SPEC-WEB-007`

> Decision Points 확정(RESOLVED): Identity 소스 = `GET /api/v1/system/version` 확장
> (`os/arch/hostname/mode/uptime_seconds`), 신규 `/system/info` 미생성 / Update 카드 =
> `SystemVersionCard` 공존 / Runtime 폴링 = 5초 / 통합 = 기존 `/admin/system` 페이지에 영역 추가.

## 개요

접속한 xflowd 인스턴스 **자신(self/local)** 의 시스템 정보(Identity + Runtime)를 기존
`/admin/system` 페이지에 표출한다. 백엔드는 기존 `GET /api/v1/system/version` 을 확장하여
`os/arch/hostname/mode/uptime_seconds` 만 추가하고, 실시간 리소스는 기존 `GET /monitor/metrics` 를
재사용한다. 원격 노드 표시(NodeDashboard/REMOTE-001)는 범위 외.

## 개발 방법론

`.moai/config/sections/quality.yaml` 의 `development_mode: hybrid` 를 따른다.

- **신규 코드** (SystemInfoCard, SystemRuntimeCard, useSystemMetrics 훅):
  TDD (RED-GREEN-REFACTOR), 신규 커버리지 85% 목표
- **기존 코드 수정** (VersionResponse / UpdateService.Version() / SystemStatusPage / monitorService):
  DDD (ANALYZE-PRESERVE-IMPROVE), 회귀 방지 위한 characterization 우선

## 마일스톤 (우선순위 기반 — 시간 추정 없음)

### Primary Goal — 백엔드 Identity 확장 (Priority High)

- [ ] `internal/api/dto/update.go` `VersionResponse` 에 `os/arch/hostname/mode/uptime_seconds`
      필드 추가 (기존 7개 필드는 불변)
- [ ] `internal/api/handler/system_update.go` `UpdateService.Version()` 에서
      `runtime.GOOS / runtime.GOARCH / os.Hostname() / config.remote_management.mode / uptime` 채움
- [ ] 시크릿 비노출 검증 — 응답에 jwt_secret/bootstrap_secret/enrollment_token/키 부재 확인
- [ ] Go 단위 테스트: os/arch/hostname/mode/uptime_seconds 필드 채움 + 기존 필드 불변 +
      시크릿 부재 (table-driven, characterization 포함)

검증 기준: `GET /system/version` 가 os/arch/hostname/mode/uptime_seconds 를 반환하고 기존 7개
필드를 유지하며 시크릿을 포함하지 않고, `go test -race ./internal/api/...` 통과.

### Secondary Goal — 프론트엔드 데이터 계층 (Priority High)

- [ ] `web/src/services/api/systemUpdate.ts` `VersionInfo` 에
      `os/arch/hostname/mode/uptime_seconds` 추가
- [ ] `web/src/services/api/monitorService.ts` `getMetrics()` 를 `Record<string, unknown>` →
      `SystemMetrics` 타입으로 강화
- [ ] `useSystemMetrics()` TanStack Query 훅 추가 (`/monitor/metrics`, `refetchInterval: 5000`,
      `refetchIntervalInBackground: false`)
- [ ] Vitest: 훅 5초 폴링 주기/일시정지/타입 매핑 검증 (msw mock)

검증 기준: 타입 안전(`any` 금지), `useSystemMetrics` 폴링/일시정지 동작, 단위 테스트 통과.

### Final Goal — UI 컴포넌트 + 페이지 통합 (Priority Medium)

- [ ] `SystemInfoCard.tsx` 신규 — Identity(hostname/OS/Arch/version/commit/build_date/go_version/
      mode) 표시, monospaced 칩, mode 한글 라벨 매핑, "이 인스턴스 (self)" 라벨
- [ ] `SystemRuntimeCard.tsx` 신규 — Runtime(uptime/CPU%/mem%/goroutines/heap) 표시,
      uptime 가독 형식, CPU 0% "측정 미지원" 안내
- [ ] `SystemStatusPage.tsx` 수정 — 두 카드 마운트, 로딩/에러 영역별 독립 처리, admin 게이팅 유지
- [ ] commit/build_date 클릭 시 GitHub commit URL 새 탭 (Optional)
- [ ] Vitest 컴포넌트 테스트: 렌더링/로딩/에러/빈 상태/mode 라벨

검증 기준: 모든 acceptance.md 시나리오 통과, 영역별 독립 에러 처리 동작.

### Optional Goal — SELF/REMOTE 안내 보조 (Priority Low)

- [ ] mode == `server` 일 때 "원격 노드 목록은 대시보드 노드 뷰에서" 보조 링크 (M9 Optional)
- [ ] commit 클릭 → GitHub URL (M2 Optional)

## 기술 접근

| 항목 | 결정 |
| --- | --- |
| Identity 소스 | 기존 `GET /api/v1/system/version` 확장 (확정, 신규 `/system/info` 미생성) |
| Runtime 소스 | 기존 `GET /monitor/metrics` 재사용 (신규 백엔드 0) |
| Mode 소스 | `config.remote_management.mode` (`config.go:476`) |
| 폴링 | TanStack Query, Runtime 5초 고정(`refetchInterval: 5000`), 백그라운드 일시정지 |
| 시크릿 | 절대 미노출 — DTO 화이트리스트 필드만 |
| 권한 | 기존 `/admin/system` admin 게이팅 재사용 (`SystemStatusPage.tsx:47`) |
| 스타일 | Tailwind + `MetadataChips` 패턴, 신규 외부 의존성 없음 |

## 아키텍처 설계 방향

```
[브라우저]
   │  GET /api/v1/system/version (확장: os/arch/hostname/mode/uptime_seconds)  ← Identity (1회/저빈도)
   │  GET /api/v1/monitor/metrics                                              ← Runtime (5초 폴링)
   ▼
[xflowd LOCAL]
   ├ SystemHandler.GetVersion → UpdateService.Version()
   │     ← runtime.GOOS/GOARCH, os.Hostname(), config.mode, ldflags 빌드 메타
   └ MonitorHandler.Metrics → defaultMonitorManager.GetMetrics()
         ← runtime.MemStats, NumGoroutine, uptime
```

모든 데이터는 **로컬**. REMOTE-001 heartbeat 프로토콜과 무관하며 변경하지 않는다.

## 위험 및 대응

| 위험 | 영향 | 대응 |
| --- | --- | --- |
| `VersionResponse` 확장이 SPEC-WEB-006 `SystemVersionCard` 에 영향 | 기존 카드 회귀 | 추가 필드만 append, 기존 필드 불변 → characterization 테스트로 회귀 방지 |
| CPU 0% 가 "고장"으로 오인 | UX 혼란 | M3 "측정 미지원" 명시 안내 |
| 시크릿 우발 노출 | 보안 사고 | DTO 화이트리스트 + 단위 테스트로 시크릿 부재 강제 |
| 원격 노드 정보와 혼동 | 운영 혼선 | M8 "이 인스턴스 (self)" 명시 라벨 + server 모드 보조 링크 |

## 완료 정의(요약)

- Decision Points 확정 반영 완료 (version 확장 / 카드 공존 / 5초 폴링 / 페이지 통합)
- Identity/Runtime 정보가 `/admin/system` 에 표시, mode 무관 동작
- 시크릿 미노출 검증 통과
- 신규 코드 85%+ 커버리지, 기존 코드 회귀 없음, TRUST 5 게이트 통과
- 상세 기준은 acceptance.md 참조
