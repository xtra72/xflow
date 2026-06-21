---
id: SPEC-UPDATE-001
title: xflowd 애플리케이션 바이너리 자동 업데이트 메커니즘 — 수용 기준
version: 0.1.0
status: draft
created: 2026-05-05
updated: 2026-05-05
author: xtra
priority: high
---

# SPEC-UPDATE-001: 수용 기준 (Acceptance Criteria)

## 시나리오 (Given-When-Then)

> v0.1.0 신규 SPEC. 시나리오 1~10은 핵심 흐름(정상 + 보안 실패 + 운영 장애 + 다운그레이드 + API)을 망라.

### Scenario 1: 새 버전 발견 — 정상 케이스 (M2)

**Given**

- xflowd가 v0.3.0으로 운영 중이다
- `update.enabled: true`, `update.channel: "stable"`로 설정되어 있다
- GitHub Releases에 v0.4.0이 stable로 공개되어 있다
- 운영자가 적절 권한으로 셸에 접속해 있다

**When**

- 운영자가 `xflowd update check` 명령을 실행한다

**Then**

- 명령은 exit code 2로 종료한다 (업데이트 가능)
- stdout에 다음과 같은 정보가 출력된다:
  - 현재 버전: v0.3.0
  - 최신 버전: v0.4.0
  - 업데이트 가능: yes
  - 채널: stable
  - 릴리즈 노트 URL
- 구조화 로그 이벤트 `update.check`가 `result=success`로 기록된다

---

### Scenario 2: 자동 적용 — 해피 패스 (M3, M4, M5, M6)

**Given**

- xflowd가 v0.3.0으로 운영 중, 처리 중인 in-flight 메시지 없음
- v0.4.0 바이너리가 stable 채널에 게시되었으며 정상 서명되어 있다
- `/tmp` 디스크 여유 공간이 충분하다 (바이너리 크기 × 3 이상)

**When**

- 운영자가 `xflowd update apply --yes` 명령을 실행한다

**Then**

- 다음 단계가 순서대로 수행된다:
  1. 새 바이너리, checksums.txt, signature.bin 다운로드 → `/tmp/xflowd-update-*` (권한 `0600`)
  2. SHA256 체크섬 비교 통과
  3. Ed25519 서명 검증 통과
  4. 현재 바이너리 → `<binary>.previous` 백업
  5. 새 바이너리 → 원래 위치로 atomic rename (권한 `0755`)
  6. graceful shutdown (drain timeout 30s 안에 완료)
  7. 새 프로세스로 전환 (`syscall.Exec` 또는 systemd notify)
  8. 새 프로세스 health check 5초 안에 정상 응답
- 명령은 exit code 0으로 종료
- `xflowd --version`이 v0.4.0을 보고한다
- `<binary>.previous`가 디스크에 보존되어 있다 (롤백 대비)
- 구조화 로그 이벤트 `update.download`, `update.verify`, `update.apply`가 모두 `result=success`로 기록된다

---

### Scenario 3: 검증 실패 — 서명 위조 (M4)

**Given**

- xflowd가 v0.3.0으로 운영 중
- 채널의 v0.4.0 바이너리는 정상이나, signature.bin이 위조 또는 손상되었다 (예: 다른 키로 서명)

**When**

- `xflowd update apply --yes` 실행

**Then**

- 다운로드는 성공한다 (HTTPS + TLS 정상)
- SHA256 체크섬 비교가 통과할 수 있으나 (또는 실패할 수 있으나, 본 시나리오는 서명 단계 검증):
- Ed25519 서명 검증이 실패한다
- `ErrUpdateSignatureInvalid` 에러 반환
- 다운로드된 임시 파일이 즉시 삭제된다 (실행 권한 부여 안 됨)
- 현재 바이너리는 변경되지 않는다 (백업도 생성 안 됨)
- 명령은 exit code 2 (검증 실패)로 종료
- 보안 감사 로그에 `update.verify` 이벤트가 `result=failure, reason=signature_invalid`로 기록된다 (운영 모니터링 알림 trigger 가능)

---

### Scenario 4: 자동 롤백 — health check 실패 감지 (M7)

**Given**

- v0.4.0 바이너리가 v0.3.0에서 정상 적용되어 graceful restart까지 진행됨
- 하지만 v0.4.0 새 프로세스가 yaml 호환성 문제로 부팅 실패 (예: SPEC-STORE-003 v0.3.0 BREAKING 미적용)
- `<binary>.previous`(v0.3.0 백업)가 보존되어 있다

**When**

- 새 프로세스 health check 5초 안에 응답하지 않는다 (timeout)

**Then**

- 자동 롤백이 trigger된다
- `<binary>.previous` → 원래 위치로 atomic rename (역방향 복원)
- 이전 v0.3.0 프로세스 재시작
- v0.3.0 health check 정상 응답
- 운영자에게 critical 로그 알림: "auto rollback completed: v0.4.0 → v0.3.0, reason: health_check_timeout"
- 자동 update apply가 일시 중지(lock)된다 (loop 방지)
- 구조화 로그 이벤트 `update.rollback`이 `result=success, trigger=auto_health_check_failure`로 기록된다

---

### Scenario 5: 다운그레이드 거부 — `--force` 미지정 (M8)

**Given**

- xflowd가 v0.4.0으로 운영 중
- 운영자가 실수로 v0.3.0으로 되돌리려 한다

**When**

- `xflowd update apply --version v0.3.0 --yes` 실행 (`--force` 없음)

**Then**

- 다운로드는 시작되지 않는다 (사전 버전 비교 단계에서 거부)
- `ErrDowngradeRequiresForce` 에러 반환
- 명시적 메시지: "downgrade from v0.4.0 to v0.3.0 requires --force flag"
- 명령은 exit code 4로 종료
- `update.downgrade_attempt` 이벤트가 `result=rejected`로 기록된다
- 현재 바이너리 변경 없음

---

### Scenario 6: 디스크 부족 — 다운로드 시작 거부 (M3)

**Given**

- xflowd가 v0.3.0으로 운영 중
- v0.4.0이 발견되었으나 임시 디렉토리(`/tmp`)의 가용 공간이 바이너리 크기의 2배 미만 (사전 검사 임계값: 3배)

**When**

- `xflowd update apply --yes` 실행

**Then**

- 사전 디스크 검사가 실패한다
- 다운로드 단계에 진입하지 않는다
- `ErrUpdateInsufficientDiskSpace` 에러 반환
- 명시적 메시지: "insufficient disk space in /tmp: 50MB available, 75MB required"
- 명령은 exit code 1로 종료
- 현재 바이너리 변경 없음
- 구조화 로그 이벤트 `update.download`가 `result=skipped, reason=insufficient_disk`로 기록된다

---

### Scenario 7: HTTP-only URL 거부 (M1, M13)

**Given**

- xflowd 설정에 `update.update_url: "http://example.com/releases"` (HTTPS 아님)이 설정되어 있다

**When**

- xflowd 부팅 또는 `xflowd update check` 실행

**Then**

- 설정 로드 또는 update 명령이 거부된다
- `ErrUpdateChannelInvalid` 에러 반환
- 명시적 메시지: "update_url must use https:// scheme; got http://example.com/releases"
- 부팅 단계에서 발생한 경우 xflowd는 시작되지 않는다 (운영자가 설정을 수정해야 함)
- 명령 실행 단계에서 발생한 경우 exit code 1로 종료

---

### Scenario 8: REST API 버전 확인 — 인증된 호출 (M10)

**Given**

- xflowd v0.3.0이 정상 운영 중이며 REST API가 활성화되어 있다
- 클라이언트가 유효한 인증 토큰을 보유하고 있다
- v0.4.0이 채널에 게시되어 있다

**When**

- 클라이언트가 `POST /api/v1/system/update/check` 요청을 인증 헤더와 함께 전송

**Then**

- HTTP 응답 코드 200
- 응답 바디는 다음 JSON과 일치한다:
  ```json
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
- 구조화 로그 이벤트 `update.check`가 `caller=api, result=success`로 기록된다

---

### Scenario 9: 미인증 API 호출 거부 (M10)

**Given**

- xflowd가 정상 운영 중

**When**

- 클라이언트가 인증 헤더 없이 `POST /api/v1/system/update/apply`를 호출

**Then**

- HTTP 응답 코드 401 Unauthorized
- 응답 바디:
  ```json
  {
    "success": false,
    "error": {
      "code": "UNAUTHORIZED",
      "message": "authentication required"
    }
  }
  ```
- 어떠한 update 동작도 수행되지 않는다
- 다운로드도 시작되지 않고 임시 파일도 생성되지 않는다

---

### Scenario 10: graceful drain timeout 초과 (M6)

**Given**

- xflowd가 v0.3.0으로 운영 중이며 in-flight FBP 메시지 100건이 처리 중
- 메시지 처리가 매우 느린 외부 의존성으로 인해 30초(drain_timeout) 안에 완료되지 않는 상황
- v0.4.0 update apply가 진행 중

**When**

- 검증 통과 + atomic 교체 완료 후 graceful shutdown 시퀀스가 시작된다
- 30초 drain timeout이 경과한다

**Then**

- 미처리 in-flight 메시지 (예: 35건)가 손실된다
- 구조화 로그에 `drain_timeout_exceeded, lost_messages=35` 명시 기록
- 강제 종료 후 새 프로세스로 전환 (syscall.Exec 또는 systemd 신호)
- 새 프로세스가 정상 부팅 및 health check 통과
- update apply는 성공 (`update.apply` 이벤트 `result=success_with_message_loss`)
- 운영자가 사후 손실 메시지 카운트를 확인 가능 (모니터링 메트릭 + 로그)

---

### Scenario 11: 릴리스 이미지 키 생성·서명·서명 이미지 빌드 (M15)

**Given**

- 릴리스 담당자가 빌드 환경에 접속해 있다

**When**

- `make keygen` 을 실행하고, 이어서 `make release-images VERSION=v0.4.0 SIGN_KEY=xflow-release.key` 를 실행한다

**Then**

- `keygen` 이 비공개키 `xflow-release.key`(권한 `0600`)와 공개키 `xflow-release.pub` 를 생성한다 (`xflowd update keygen` 위임)
- `release-images` 가 6 교차컴파일 타깃의 `xflowd` 바이너리를 산출하고, 각 바이너리에 대해 `xflowd update sign --key xflow-release.key` 로 raw 64-byte `.sig` 를 생성하며, 버전별 `checksum.txt` 를 만든다
- LDFLAGS 에 `-X main.Version=v0.4.0` 이 주입되어 산출 바이너리가 v0.4.0 을 보고한다 (M18)
- `VERSION` 또는 `SIGN_KEY` 미지정 시 `release-images` 가 명시적 에러로 즉시 실패한다 (가드)
- 비공개키는 산출 아티팩트/로그에 노출되지 않는다

---

### Scenario 12: CI release-images 잡 — 시크릿 주입·GitHub Release 첨부 (M15)

**Given**

- `release.yml` 에 `release-images` 잡이 정의되어 있고, 리포지토리 시크릿 `XFLOW_RELEASE_PRIVATE_KEY`(128-hex)가 설정되어 있다

**When**

- 버전 태그(예: `v0.4.0`)가 푸시된다

**Then**

- 잡이 시크릿에서 서명 비공개키를 임시 파일로 주입하고 `make release-images VERSION=… SIGN_KEY=…` 를 실행한다
- 6 타깃 서명 이미지(`.sig` 포함) + `checksum.txt` 가 GitHub Release 에 추가 첨부된다
- 빌드 후 서명 키 임시 파일이 제거된다
- 시크릿 `XFLOW_RELEASE_PRIVATE_KEY` 가 미설정이면 서명 이미지 발행을 건너뛰고 명시 notice 를 남긴다

---

### Scenario 13: 원격 업데이트 — public_key_path 필수·채널/자산 진단 (M16)

**Given**

- 노드가 원격 관리 서버에 등록·승인되어 있고, 서버가 `system/update` 를 디스패치한다

**When**

- 노드의 `update.public_key_path` 가 미설정인 상태로 원격 업데이트를 시도한다

**Then**

- 시스템은 업데이트를 거부한다 (내장 핀닝 키 없음 — 로컬 공개키 명시 필수)
- `public_key_path` 가 설정되면, 노드는 그 공개키로 Ed25519 서명을 검증한다
- 채널 불일치 또는 현재 OS/Arch 자산 누락 시, 구체적 진단 메시지(불일치 채널/누락 OS·Arch 자산명)를 포함한 에러를 반환한다
- `update.insecure_skip_verify=true` 면 다운로드 TLS 검증은 건너뛰되 Ed25519 검증은 그대로 수행되어 무결성이 유지된다
- 공개키는 노드 로컬에만 존재하며 서버로부터 전송받지 않는다

---

### Scenario 14: systemd ReadWritePaths — 바이너리 교체 권한 (M17)

**Given**

- xflowd 가 `/opt/xflow/xflowd` 로 설치되어 systemd 하드닝(`ProtectSystem=strict`) 하에 운영 중이다

**When**

- 자가 업데이트가 바이너리를 원자 교체(M5)하려 한다

**Then**

- `ReadWritePaths=/opt/xflow` 인 경우, 바이너리 교체 + `.previous` 백업이 정상 수행된다
- `ReadWritePaths=/opt/xflow/data` 만 허용된 경우, 바이너리 교체(`rename(2)`)가 `read-only file system` 으로 실패하며, 운영자가 `ReadWritePaths` 에 설치 디렉토리를 포함하도록 유닛을 수정해야 한다

---

## Edge Case Checklist

### 보안

- [ ] HTTPS 전용 강제 — `http://` 스킴은 부팅/실행 단계 모두에서 거부
- [ ] 공개키 핀닝 — 빌드 변수의 공개키만 신뢰, 외부 변경 시 검증 실패
- [ ] TLS 인증서 검증 — 자가 서명 인증서 default 거부 (`insecure_skip_verify=false`)
- [ ] `insecure_skip_verify=true` 명시적 설정 시 critical 경고 로그 + 동작 허용 (개발용)
- [ ] 다운로드 임시 파일 권한 `0600` (소유자 외 읽기 불가, 실행 불가)
- [ ] 검증 통과 후에만 `0755`로 권한 변경 (검증 전 절대 실행 가능 상태 아님)
- [ ] Replay 공격 방어 — release `published_at`이 현재 빌드 시점보다 이전이면 경고 로그
- [ ] 서명 검증 실패 → 임시 파일 즉시 삭제 + 보안 감사 로그
- [ ] Ed25519 공개키 PEM 파싱 실패 → 부팅 실패 (잘못된 빌드 감지)
- [ ] HTTP redirect 추적 시 HTTPS만 허용 (HTTP redirect 추적 거부)

### 안정성 / 동시성

- [ ] 동시 update apply 호출 → 두 번째는 `ErrUpdateInProgress` (HTTP 409)
- [ ] 다운로드 중 SIGINT/SIGTERM → 임시 파일 정리 후 종료 (백업 보존)
- [ ] 검증 단계 중 인터럽트 → 임시 파일 폐기, 현재 바이너리 무영향
- [ ] 원자적 교체 도중 SIGKILL → 파일시스템 일관성 (atomic rename 보장)
- [ ] 백업 파일(`<binary>.previous`)이 다음 update apply 시 덮어쓰기 → 직전 백업만 보존
- [ ] graceful drain 진행 중 추가 update apply 시도 → mutex로 직렬화
- [ ] scheduler가 동작 중일 때 명시적 `update apply` 호출 → 정상 진행 (scheduler는 다음 주기에 skip)
- [ ] HTTP 다운로드 partial 수신 (network reset) → SHA 검증 단계에서 실패 → 명시적 에러
- [ ] 디스크 사전 검사 통과 → 다운로드 중 디스크 가득 → 다운로드 실패 + 임시 파일 정리

### 호환성

- [ ] Linux x86_64 (primary)
- [ ] Linux arm64 (Raspberry Pi 등 IoT 환경)
- [ ] macOS x86_64 / arm64 (개발 환경)
- [ ] systemd Type=notify 환경 — `READY=1` / `STOPPING=1` notification 발송
- [ ] systemd 비-notify 환경 — `syscall.Exec`로 동일 PID 유지
- [ ] launchd (macOS) — KeepAlive 환경에서 자연스럽게 재시작
- [ ] 비-supervisor 환경 (foreground 실행) — `syscall.Exec` 자체로 충분
- [ ] Cross-device rename 회피 — 임시 디렉토리는 바이너리와 동일 마운트 포인트

### 운영

- [ ] 채널 변경(`xflowd update channel beta`) 즉시 반영 → 다음 check부터 beta 사용
- [ ] `check_interval=0` → scheduler 비활성, 명시적 호출만 동작
- [ ] `enabled=false` → 모든 자동 동작 비활성, scheduler 시작 안 함
- [ ] `auto_apply=true` AND `notify_only=true` → 설정 검증 실패 (상호 배타)
- [ ] 자동 롤백 후 자동 update lock 발동 → 운영자 명시 해제 전까지 자동 update 비활성
- [ ] 매우 큰 바이너리 다운로드 (예: 100MB) → 진행률 콜백 호출 빈도 적절 (1초 간격 또는 1MB마다)
- [ ] 채널 fetch 캐시 → 네트워크 일시 불통 시 마지막 결과로 fallback
- [ ] CLI `--json` flag 출력 → 모든 정보가 단일 JSON 객체로, parsing 친화

### 다운그레이드 / 버전 비교

- [ ] semver 비교 — v1.10.0 > v1.9.0 (string 비교 함정 회피)
- [ ] prerelease semver — v1.0.0-beta.2 vs v1.0.0 비교 (semver 표준 준수)
- [ ] 동일 버전 → 업데이트 불필요 (`update_available=false`)
- [ ] 다운그레이드 + `--force` 명시 → 인터랙티브 확인 또는 `--yes` 필수
- [ ] 자동 적용(`auto_apply=true`)은 다운그레이드를 절대 자동 수행 안 함

### Edge: GitHub Releases 특수 케이스

- [ ] Releases 0건 (신규 저장소) → `update_available=false` + 명시적 메시지
- [ ] Latest API rate limit (HTTP 403) → 명시적 에러 + 다음 check까지 cache
- [ ] Asset이 현재 OS/arch에 없음 → `ErrUpdateChannelInvalid` 또는 명시적 에러 (예: ARM 빌드 누락)
- [ ] checksums.txt에 현재 asset 항목 없음 → 검증 실패
- [ ] signature.bin 누락 (release에 포함 안 됨) → 검증 실패 (fail-closed)
- [ ] release tag가 semver 형식 아님 (예: `release-2026-05-05`) → 비교 실패 → skip + 경고 로그

### Edge: 권한 / 파일시스템

- [ ] 바이너리 위치 권한 부족 (운영자가 root 아님) → `ErrUpdateApplyFailed` + 명시적 권한 안내
- [ ] 백업 파일(`<binary>.previous`)이 이미 존재 (이전 update의 잔존) → 덮어쓰기 (가장 최근 버전이 백업)
- [ ] 바이너리 위치가 read-only 파일시스템 (예: 컨테이너 immutable rootfs) → 명시적 에러
- [ ] SELinux/AppArmor 컨텍스트 → 권한 거부 시 명시적 에러 (구체적 LSM 통합은 별도 SPEC)

### Edge: 빌드 변수 / 키 관리

- [ ] 빌드 시 `UpdaterPublicKey` 미주입 (빈 문자열) → 검증 단계에서 fail-closed (자동 업데이트 동작 불가)
- [ ] `public_key_path` 설정으로 외부 키 파일 사용 → 파일 없음/손상 시 부팅 실패
- [ ] 공개키 PEM 헤더 잘못됨 → 부팅 시 명시적 파싱 에러

---

## TRUST 5 품질 게이트

### T — Tested (테스트)

- [ ] `internal/updater/verifier.go` 커버리지 **= 100%** (보안 critical, 모든 분기 커버)
- [ ] `internal/updater/checker.go` 커버리지 ≥ 90%
- [ ] `internal/updater/downloader.go` 커버리지 ≥ 90%
- [ ] `internal/updater/applier.go` 커버리지 ≥ 85%
- [ ] `internal/updater/rollback.go` 커버리지 ≥ 90%
- [ ] `internal/updater/restarter.go` 커버리지 ≥ 80%
- [ ] `internal/updater/scheduler.go` 커버리지 ≥ 85%
- [ ] `internal/updater/manager.go` 커버리지 ≥ 85%
- [ ] `internal/config/update.go` 커버리지 ≥ 90%
- [ ] `cmd/xflowd/update.go` 커버리지 ≥ 85%
- [ ] `internal/api/handler/system_update.go` 커버리지 ≥ 85%
- [ ] 통합 테스트: mock GitHub Releases server로 end-to-end check/apply/rollback 흐름 검증
- [ ] 통합 테스트: 검증 실패 (체크섬, 서명) 시 정상 거부 + 임시 파일 정리
- [ ] 통합 테스트: 다운그레이드 거부 + `--force` 동작
- [ ] 통합 테스트: 동시 apply 호출 → mutex 직렬화
- [ ] `go test -race ./...` 전체 통과 (특히 manager의 동시성)
- [ ] (선택) Fuzz: verifier Ed25519 입력 fuzz, checker JSON fuzz

### R — Readable (가독성)

- [ ] 신규 패키지 `internal/updater/`의 모든 export 함수/타입에 Go doc 주석 추가
- [ ] sentinel error는 `Err...` 네이밍 + 의도 주석 (특히 보안 관련)
- [ ] 한국어 주석 가능 (`code_comments=ko`)
- [ ] `gofmt`, `goimports` 준수
- [ ] 복잡한 흐름(graceful restart, rollback)은 단계별 주석 명시
- [ ] CLI 명령 help 텍스트는 운영자 친화적으로 작성 (예시 포함)

### U — Unified (통일성)

- [ ] 기존 에러 네이밍 패턴(`ErrXxx`) 준수 — `ErrUpdateXxx` 통일
- [ ] 기존 config 파싱 패턴(`parseXxxConfig`) 준수
- [ ] HTTP 핸들러 응답 형식 기존 컨벤션 일치 — `{success, data, error}` 패턴
- [ ] JSON 필드 네이밍 snake_case (예: `current_version`, `update_available`, `release_notes_url`)
- [ ] CLI flag 네이밍 kebab-case (예: `--force`, `--yes`, `--json`)
- [ ] 구조화 로그 이벤트 명명 dot.notation (예: `update.check`, `update.apply`)
- [ ] 기존 graceful shutdown 패턴(SPEC-LIFE-001)과 통합 — 별도 shutdown 구현 회피
- [ ] CLI `--json` 출력 형식이 REST API 응답 형식과 동일 키 사용

### S — Secured (보안)

- [ ] HTTPS 전용 검증 (URL 스킴 검사 + redirect 추적 시 HTTPS만)
- [ ] TLS 인증서 검증 활성 (default `insecure_skip_verify=false`)
- [ ] Ed25519 공개키 핀닝 (빌드 변수 임베드)
- [ ] 다운로드 임시 파일 권한 `0600` 강제, 검증 후에만 `0755`
- [ ] 서명 검증 실패 시 임시 파일 즉시 삭제 (잔존 위험 0)
- [ ] 검증되지 않은 바이너리는 어떠한 경로로도 실행되지 않음 (zero unverified execution)
- [ ] `inferDataType` 등 helper에서 untrusted input 처리 시 panic 방어 (`recover`)
- [ ] CLI 권한 검사 — 바이너리 경로 write 권한 없으면 명시적 에러 (root/sudo 안내)
- [ ] REST API 인증/인가 미들웨어 통합 (SPEC-AUTH-001/002)
- [ ] 보안 critical 이벤트는 별도 감사 로그 (서명 실패, 검증 실패, 다운그레이드 시도)
- [ ] 다운로드 URL이 redirect되더라도 최종 URL이 HTTPS인지 검증
- [ ] OWASP Software Update 권고 사항 준수 (https://cheatsheetseries.owasp.org/)

### T — Trackable (추적성)

- [ ] SPEC TAG 주석: `// @SPEC:SPEC-UPDATE-001 v0.1.0` 신규 코드 모든 파일에 삽입
- [ ] Conventional commit: `feat(updater): introduce xflowd self-update mechanism [SPEC-UPDATE-001]`
- [ ] Tasks를 단계별 commit으로 분리 가능 (예: types/errors → checker → downloader → ... → CLI → API → tests)
- [ ] CHANGELOG.md에 신규 항목 기재:
  - 신규 기능: xflowd 자가 업데이트 (CLI + REST API)
  - 신규 의존성: `github.com/inconshreveable/go-update`
  - 신규 설정 섹션: `update`
  - 보안: Ed25519 서명 검증 + 공개키 핀닝
- [ ] 운영자 가이드 (`docs/updater-design.md`)에 다음 포함:
  - 채널 설정 방법 (stable/beta/nightly)
  - 자동 업데이트 활성화 절차
  - 롤백 시나리오 (자동/수동)
  - 트러블슈팅 (서명 실패, 디스크 부족, drain timeout)
  - 키 회전 절차 (미래 SPEC link)
- [ ] 모든 update 이벤트가 구조화 로그로 추적 가능 (event_type, current/target version, result, duration_ms, error)

---

## Definition of Done (DoD)

본 SPEC v0.1.0이 완료되려면 다음 조건을 모두 만족해야 한다.

### 1. 기능 완전성

- [ ] 14개 EARS 모듈(M1~M14)의 모든 요구사항이 구현됨
- [ ] 10개 Given-When-Then 시나리오가 자동화 테스트로 통과
- [ ] Edge case 체크리스트(보안/안정성/호환성/운영/다운그레이드/Releases 특수/권한)가 모두 검증됨
- [ ] 신규 9종 sentinel error가 모두 적절한 위치에서 반환됨

### 2. 품질 기준

- [ ] TRUST 5 품질 게이트 모든 항목 통과
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet` 및 `golangci-lint` 경고 0
- [ ] `internal/updater/verifier.go` 분기 커버리지 100% (보안 critical)
- [ ] 전체 신규 코드 평균 커버리지 87% 이상

### 3. 보안 검증

- [ ] HTTPS 전용 강제 검증 통과 (HTTP URL 거부)
- [ ] Ed25519 서명 검증 통과 (위조 서명 거부)
- [ ] 공개키 핀닝 검증 (잘못된 빌드 키로 시도 시 거부)
- [ ] 검증되지 않은 바이너리 실행 0건 (zero unverified execution)
- [ ] OWASP Software Update 권고 항목 체크리스트 통과
- [ ] (선택) 외부 보안 리뷰 1회 (Ed25519 통합 + 공개키 핀닝 흐름)

### 4. 운영 검증

- [ ] systemd Type=notify 환경에서 정상 동작 (READY=1 / STOPPING=1 신호)
- [ ] systemd 비-notify 환경에서 `syscall.Exec` 정상 동작 (동일 PID 유지)
- [ ] launchd (macOS) 환경 정상 동작 (KeepAlive 재시작)
- [ ] 자동 롤백 시나리오 검증 (health check fail → 백업 복구)
- [ ] 명시적 롤백(`xflowd update rollback`) 정상 동작
- [ ] 자동 적용 + 다운그레이드 거부 정상 동작

### 5. 빌드 시스템 통합

- [ ] (M18) Makefile LDFLAGS 에 `-X main.Version=$(VERSION)`(대문자 `main.Version`) 주입 — 대소문자 일치 확인(불일치 시 `dev` 폴백)
- [ ] (M15) `make keygen` 으로 Ed25519 키쌍 생성(비공개 `.key` `0600` + 공개 `.pub`)
- [ ] (M15) `make release-images VERSION SIGN_KEY` 가 6 타깃 교차컴파일 + `.sig` 서명 + `checksum.txt` 산출, `VERSION`/`SIGN_KEY` 미지정 시 가드 실패
- [ ] (M15) `release.yml` `release-images` 잡: 시크릿 `XFLOW_RELEASE_PRIVATE_KEY` 주입 → 서명 이미지 GitHub Release 첨부, 시크릿 미설정 시 스킵, 빌드 후 키 파일 제거
- [ ] 빌드 시 비공개키는 GitHub Actions secrets에서 안전하게 주입(코드/로그/아티팩트 비노출)
- [ ] 릴리즈 asset 명명 규칙 정착 (`xflowd-{os}-{arch}` + `.sig`)
- [ ] (M16) 공개키는 노드 로컬(`update.public_key_path`)에만 배치 — 내장 핀닝 키 없음, 원격 업데이트 시 미설정 거부
- [ ] (M17) systemd 유닛 `ReadWritePaths=/opt/xflow`(설치 디렉토리 전체) — `…/data` 만 허용 시 바이너리 교체 실패 회귀 확인

### 6. API 변경 검증

- [ ] 5개 REST 엔드포인트 모두 인증/인가 미들웨어 적용
- [ ] 응답 형식이 기존 API 컨벤션과 일치 (`{success, data, error}`)
- [ ] `GET /api/v1/system/version` 인증 정책 결정 + 문서화
- [ ] OpenAPI/Swagger 스키마 갱신 (관련 SPEC이 사용하는 경우)
- [ ] 동시 호출 시 HTTP 409 Conflict 정상 동작

### 7. CLI 변경 검증

- [ ] 5개 서브커맨드 모두 동작 (`check`/`apply`/`status`/`rollback`/`channel`)
- [ ] `--json` flag로 모든 명령의 구조화 출력 가능
- [ ] exit code 컨벤션 준수 (0/1/2/3/4)
- [ ] help 텍스트가 운영자 친화적 (예시 포함)

### 8. 설정 변경 검증

- [ ] yaml에 `update` 섹션이 없으면 기본값(`Enabled: false`)으로 정상 부팅
- [ ] `auto_apply: true` AND `notify_only: true` → 부팅 실패 (상호 배타)
- [ ] `update_url` HTTP 스킴 → 부팅 실패
- [ ] 잘못된 `channel` 값 → 부팅 실패
- [ ] `check_interval: 0` → scheduler 비활성

### 9. 문서화

- [ ] spec.md, plan.md, acceptance.md 3개 파일 v0.2.0 일관 작성(M15~M18 포함)
- [ ] CHANGELOG 갱신 (신규 기능 + 신규 의존성 + 신규 설정 + 보안)
- [ ] 운영자 가이드 (`docs/updater-design.md`) 작성
  - 채널 설정 / 자동 업데이트 활성화 / 롤백 / 트러블슈팅
- [ ] 9종 신규 에러가 운영자 문서에 의미 + 대응 절차로 기재
- [ ] (선택, 미래) `docs/security/key-rotation.md` 별도 SPEC

### 10. 통합 / 회귀

- [ ] 기존 SPEC 회귀 0건 — 특히 SPEC-LIFE-001 (graceful shutdown), SPEC-AUTH-001/002 (REST API 인증)
- [ ] 기존 CLI 명령 동작 무영향
- [ ] 기존 REST API 동작 무영향 (신규 엔드포인트만 추가)
- [ ] xflow-agent / xflow CLI 빌드 영향 0건 (별도 SPEC에서 동일 패턴 재사용 예정)

### 11. 릴리즈 준비

- [ ] PR 생성 + Conventional commit 메시지
- [ ] 코드 리뷰 1회 이상 승인 (보안 영역은 추가 리뷰 권장)
- [ ] 운영팀 사전 공지 (자동 업데이트 활성화 시점 + 키 관리 정책)
- [ ] 첫 release(v0.4.0 가정)에 본 SPEC 기능 포함 + 운영 가이드 동시 공개
- [ ] 베타 기간(예: v0.4.0-beta.1)에 limited rollout으로 자동 업데이트 흐름 실전 검증
- [ ] 병합 가능 상태 (merge-ready) — 모든 quality gate 통과
