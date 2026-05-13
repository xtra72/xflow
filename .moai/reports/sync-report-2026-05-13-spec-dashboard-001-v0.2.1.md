# Sync Report — SPEC-DASHBOARD-001 v0.2.1 Hotfix

날짜: 2026-05-13
대상 commit: 9d239c0
브랜치: feature/SPEC-DASHBOARD-001 (parent: b5a0bf6 / 8bf49e0 / 9ec8597)

## 1. 발견 경로

SPEC-AUTH-004 AC-3/AC-4 수동 검증 도중 사용자가 `INFO dashboard snapshot saved` 서버 로그와 클라이언트 "대시보드 저장에 실패했습니다…" 토스트의 비대칭을 보고. MoAI 가 root cause 를 분석하여 SPEC-DASHBOARD-001 v0.2.0 의 handler envelope 표준 미준수가 잠재해 있었음을 식별.

## 2. Root Cause

- `internal/api/handler/dashboard.go` 의 3개 `ctx.JSON` (L164 GET, L226 PUT 409, L234 PUT 200) 이 `dto.NewSuccessResponse(...)` envelope 을 누락.
- `internal/api/handler/auth.go` 등 다른 handler 는 모두 envelope 표준 준수 (SPEC-AUTH-004 commit 8bf49e0 에서 재확인). dashboard handler 만 outlier.
- 결과: 모든 정상 200/409 응답이 클라이언트에서 `APIError('UNKNOWN', 200)` → `DashboardServerError` → 토스트.
- 9dcc69c (이전 hotfix) 는 무한 PUT 루프를 차단했으나 root cause 는 본 hotfix 가 제거.

## 3. 적용된 변경

- `internal/api/handler/dashboard.go`: 3개 ctx.JSON envelope 래핑. 한국어 의도 주석 1줄씩 추가.
- `internal/api/handler/dashboard_test.go`: decodeSnapshot 헬퍼 envelope 인식 갱신 (17개 테스트 영향). raw-body 검증 2건 단순화.
- 변경 파일 정확히 2개. 신규 의존성 0건.

## 4. 검증

- go vet ./... exit 0, go build ./... 성공
- go test ./internal/api/handler/... -run TestDashboard -count=1: 22 PASS / 0 FAIL
- go test ./internal/api/handler/... -count=1 (전체 회귀): 222 PASS / 0 FAIL (SPEC-AUTH-004 auth 테스트 회귀 0)
- 수동 검증: 사용자 환경에서 대시보드 PUT 후 "저장 실패" 토스트 소멸 확인.

## 5. 결과 (TRUST 5)

- T (Tested): server-side 자동 + 사용자 수동 검증 PASS
- R (Readable): 한국어 의도 주석 추가, 변경 의도 명확
- U (Unified): 모든 API handler 가 envelope 표준 일관 (auth.go 와 dashboard.go 동등)
- S (Secured): 응답 변경 없음 (success/data 구조만 추가), 보안 영향 0
- T (Trackable): commit 9d239c0 에 SPEC-DASHBOARD-001 v0.2.1 참조, 본 sync 보고서로 트레이서빌리티 완결

## 6. Manual gate 후속

SPEC-AUTH-004 AC-3/AC-4 + 본 hotfix 결합 검증 PASS. PR 머지 가능 상태.
