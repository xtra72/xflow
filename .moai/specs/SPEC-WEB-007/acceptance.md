---
id: SPEC-WEB-007
title: 로컬 인스턴스 시스템 정보 표출 — 인수 기준
version: 0.3.0
status: completed
created: 2026-06-21
updated: 2026-06-22
author: xtra
---

# SPEC-WEB-007 인수 기준 (Acceptance Criteria)

`@ACCEPTANCE:SPEC-WEB-007`

각 시나리오는 Given-When-Then 형식이며, 괄호 안의 (Mx) 는 spec.md 요구사항 추적이다.

---

## AC-1: 시스템 정보 영역 노출 (M1)

```gherkin
Given admin 으로 인증된 사용자가
When `/admin/system` 페이지에 진입하면
Then "시스템 정보(System Info)" 영역이 표시되고
And 그 영역은 Identity 카드와 Runtime 카드를 포함한다
```

## AC-2: 모드 무관 동작 (M1)

```gherkin
Given remote_management.mode 가 server | client | disabled 중 어느 값이든
When 사용자가 `/admin/system` 에 진입하면
Then 시스템 정보 영역은 동일하게 표시된다
And 어떤 모드에서도 페이지가 게이팅되어 숨겨지지 않는다
```

## AC-3: 비-admin 접근 차단 (M1, M6)

```gherkin
Given admin 권한이 없는 사용자가 (authEnabled=true, role != admin)
When `/admin/system` 접근을 시도하면
Then 기존 페이지 정책대로 로그인 리다이렉트 또는 401/403 으로 처리되고
And 백엔드 401/403 이 최종 권한 게이트로 동작한다 (클라이언트 검사 단독 비의존)
```

## AC-4: Identity 정보 표시 (M2)

```gherkin
Given 데이터 소스가 os/arch/hostname/mode + 빌드 메타를 반환할 때
When SystemInfoCard 가 렌더링되면
Then hostname, OS/Arch(linux/amd64), version(v0.18.6), remote 모드(한글 라벨) 가 표시되고
And commit / build_date / go_version 은 SystemInfoCard 에 표시되지 않는다  # (M2, v0.3.0)
And 식별 필드는 monospaced 칩이 아니라 원격 노드 카드와 통일된 plain 텍스트 + 라벨
    (dt: uppercase tracking-wider, dd: plain) 스타일로 표시된다  # (M9, v0.3.0)
And 카드 컨테이너는 bg-surface border-default p-4, 헤더는 의미 아이콘 + text-sm 제목이다  # (M9)
And "이 인스턴스 (self)" 배지가 표시된다  # (M8, M9)
```

## AC-5: 모드 한글 라벨 매핑 (M2, M9)

```gherkin
Given mode 값이 각각 server / client / disabled 일 때
When Identity 카드가 렌더링되면
Then 각각 "관리 서버" / "클라이언트 노드" / "독립 실행 (standalone)" 라벨이 표시된다
```

## AC-6: Runtime 정보 표시 (M3)

```gherkin
Given `GET /monitor/metrics` 가 정상 응답할 때
When Runtime 카드가 렌더링되면
Then uptime("3d 4h 12m" 가독 형식), CPU %, 메모리 %, goroutines, Go heap(alloc/sys MB) 가 표시되고
And 메모리/CPU 는 소수점 1자리 + 단위(%/MB)로 표시된다  # (M9)
```

## AC-7: CPU 미지원 안내 (M3)

```gherkin
Given 백엔드가 cpu_usage_percent 를 0 으로 반환할 때 (v1 샘플링 미지원)
When Runtime 카드가 CPU 를 표시하면
Then "측정 미지원" 또는 동등한 보조 안내가 함께 표시되어 0% 가 오인되지 않는다
```

## AC-8: Runtime 폴링 (M4)

```gherkin
Given Runtime 카드가 마운트되어 있을 때
When 폴링 주기(5초, refetchInterval: 5000)가 경과하면
Then `GET /monitor/metrics` 가 재호출되어 값이 갱신되고
And 사용자가 다른 탭/페이지로 전환하면 백그라운드 폴링이 일시 중지되며
And 1초보다 짧은 간격으로는 자동 폴링하지 않는다
```

## AC-9: Identity 백엔드 확장 (M5)

```gherkin
Given 확장된 GET /api/v1/system/version 엔드포인트에 (신규 /system/info 없음)
When 클라이언트가 요청하면
Then 응답에 os(GOOS), arch(GOARCH), hostname(os.Hostname()), mode(remote_management.mode),
     uptime_seconds(프로세스 uptime) 가 포함되고
And 기존 version/commit/build_date/go_version/channel/update_available/latest_version
    필드는 변경 없이 유지된다 (SPEC-WEB-006 SystemVersionCard 회귀 없음)
```

## AC-10: 시크릿 비노출 (M5, M6)

```gherkin
Given 시스템 정보 데이터 소스 응답을 검사할 때
When 응답 본문 전체를 확인하면
Then jwt_secret, TLS 키, bootstrap_secret, enrollment_token, 인증 토큰 등
     어떤 비밀 값도 포함되지 않는다
And UI 어디에도 비밀 값이 렌더링되지 않는다
```

## AC-11: 영역별 독립 로딩/에러 (M7)

```gherkin
Given Identity 또는 Runtime 데이터가 로딩 중이거나 실패할 때
When 페이지가 렌더링되면
Then 로딩 중 영역은 스켈레톤을, 실패 영역은 명시적 에러 + "다시 시도" 를 표시하고
And 한 영역의 실패가 다른 영역의 정상 렌더링을 막지 않는다 (Identity ↔ Runtime 독립)
```

## AC-12: SELF vs REMOTE 명시적 구분 (M8)

```gherkin
Given 관리 서버(mode=server) 인스턴스에 admin 으로 접속했을 때
When 시스템 정보 영역을 보면
Then 표시 대상이 "현재 접속한 인스턴스 자신(self)" 임이 명확히 라벨링되고
And 원격 노드 목록(NodeDashboard)과 혼동되지 않으며
And (Optional) "원격 노드 목록은 대시보드 노드 뷰에서" 보조 링크가 표시될 수 있다
```

## AC-13: 프로토콜 불변 (M8)

```gherkin
Given 본 SPEC 의 모든 데이터가 로컬 소스일 때
When 기능을 구현/검증하면
Then REMOTE-001 heartbeat 프로토콜(internal/remote/protocol.go)은 변경되지 않고
And 어떤 원격 보고 흐름에도 의존하지 않는다
```

## AC-14: 설정 시스템 탭 노출 (M1, v0.3.0)

```gherkin
Given admin 으로 인증된 사용자가
When 설정(Settings) 페이지의 "시스템" 탭(SystemTab)에 진입하면
Then SystemInfoCard 와 SystemRuntimeCard 가 표시되고
And 두 카드는 /admin/system 의 것과 동일한 컴포넌트로 양쪽에 공존한다
And 설정 시스템 탭은 메뉴로 직접 접근 가능한 진입점이다 (헤더 업데이트 배지 외 진입점 부재 문제 해소)
```

> 비고(v0.3.0): SystemInfoCard 가 commit/build_date 를 더 이상 표시하지 않으므로, 기존 commit
> GitHub 링크(`https://github.com/xtra72/xflow/commit/{commit}`) 검증 AC 는 제거되었다. commit/
> build_date/go_version 표시는 별개 `SystemVersionCard`(SPEC-WEB-006) 소관이다.

---

## Definition of Done (DoD)

- [ ] AC-1 ~ AC-14 전부 통과 (AC-14 = v0.3.0 설정 시스템 탭 노출)
- [ ] Decision Points 확정 반영 완료 (version 확장 / 카드 공존 / 5초 폴링 / 페이지 통합)
- [ ] 백엔드: os/arch/hostname/mode/uptime_seconds 반환 + 기존 7개 필드 불변,
      시크릿 부재 단위 테스트 통과 (`go test -race ./...`)
- [ ] 프론트엔드: `VersionInfo`/`SystemMetrics` 타입 안전(`any` 금지), `useSystemMetrics` 폴링 검증
- [ ] 신규 코드 85%+ 커버리지, 기존 코드(SPEC-WEB-006 카드) 회귀 없음
- [ ] mode 무관 동작 + admin 게이팅 검증
- [ ] CPU 0% 안내 표시 검증
- [ ] TRUST 5 품질 게이트 통과 (LSP 0 errors)
- [ ] lint/format (golangci-lint, ESLint/Prettier) 통과

## 품질 게이트 기준

| 항목 | 기준 |
| --- | --- |
| 신규 코드 커버리지 | >= 85% |
| 기존 코드 회귀 | 0 (characterization 통과) |
| LSP errors / type errors / lint errors | 0 |
| 시크릿 노출 | 0 (테스트로 강제) |
| 신규 외부 의존성 | 0 |

## 검증 방법 / 도구

- **백엔드**: `go test -race ./internal/api/handler/...` (table-driven), 응답 JSON 스냅샷에서
  시크릿 키 부재 단언
- **프론트엔드**: Vitest + React Testing Library + msw (API mock), 폴링은 fake timer 로 검증
- **수동 확인**: server / client / disabled 3개 모드 인스턴스에서 페이지 표시 + self 라벨 확인
