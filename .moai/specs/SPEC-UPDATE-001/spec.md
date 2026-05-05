---
id: SPEC-UPDATE-001
title: xflowd 애플리케이션 바이너리 자동 업데이트 메커니즘
version: 0.1.0
status: draft
created: 2026-05-05
updated: 2026-05-05
author: xtra
priority: high
---

# SPEC-UPDATE-001: xflowd 애플리케이션 바이너리 자동 업데이트 메커니즘

## HISTORY

- **0.1.0** (2026-05-05): 최초 작성 — xflowd 자가 교체 자동 업데이트 메커니즘. GitHub Releases 기본 채널, Ed25519 디지털 서명 + SHA256 검증, 원자적 바이너리 교체, 그레이스풀 재시작(in-flight 메시지 drain), 자동 롤백(health check 실패 감지), CLI 명령(check/apply/status/rollback/channel), REST API 엔드포인트 3종, 다운그레이드 방지(--force 옵션), 채널 선택(stable/beta/nightly), 구조화 로깅, 멀티 OS 호환(Linux/macOS) 도입.

| Version | Date       | Author | Change                                                                            |
| ------- | ---------- | ------ | --------------------------------------------------------------------------------- |
| 0.1.0   | 2026-05-05 | xtra   | 최초 작성 — 자가 교체 업데이트, GitHub Releases 채널, Ed25519 서명, 그레이스풀 재시작, 자동 롤백, CLI/API 통합 |

## 개요 (Overview)

xflowd는 xflow 플랫폼의 핵심 데몬 서버로, IoT 데이터 스트림을 처리하는 FBP 엔진과 REST API를 제공한다. 현재 xflowd 바이너리는 빌드 시점에 `Version`, `Commit`, `BuildDate` 정보가 임베드되지만, 자동 업데이트 인프라는 존재하지 않는다. 운영자는 새 버전 배포마다 수동으로 바이너리를 다운로드, 검증, 교체, 재시작해야 하며, BREAKING 변경(예: SPEC-STORE-003 v0.3.0의 yaml 마이그레이션)이 발생할 때 운영 부담이 급증한다.

본 SPEC은 **xflowd 자가 교체(self-update) 메커니즘**을 도입하여 다음을 달성한다:

1. **신뢰성**: GitHub Releases 채널에서 SHA256 체크섬 + Ed25519 디지털 서명 검증을 거친 바이너리만 적용
2. **무중단성**: in-flight 메시지/플로우 drain 후 graceful restart(`syscall.Exec`)로 운영 중단 최소화
3. **안전성**: 자동 롤백(health check 실패 시 이전 바이너리 복구)과 다운그레이드 방지(`--force` 명시 필요)
4. **운영 편의성**: CLI 명령(`xflowd update check/apply/status/rollback/channel`)과 REST API 엔드포인트로 통합 제어
5. **확장성**: 채널 선택(stable/beta/nightly), 주기적 자동 확인, pre/post-update hook(미래 확장)

xflow CLI(`xflow`)와 에이전트 런타임(`xflow-agent`)의 자동 업데이트는 본 SPEC의 우선 대상이 아니나, 동일 패턴(internal/updater 패키지 재사용)으로 후속 SPEC 확장이 가능하도록 모듈을 설계한다.

## 배경 (Background)

### 현재 상태

xflow는 3개 바이너리로 구성된다:

- `xflow`: CLI 클라이언트 (xflowd REST API 원격 호출)
- `xflow-agent`: 에이전트 런타임 (xflowd와 분리 배포 가능)
- `xflowd`: 메인 데몬 서버 (FBP 엔진 + REST API + 인증 + 영속화)

각 바이너리의 `cmd/{name}/main.go`에는 빌드 변수로 다음이 임베드된다:

- `Version` (예: `v0.3.0`)
- `Commit` (Git 단축 해시)
- `BuildDate` (RFC3339)

운영자는 `xflowd --version`으로 정보를 확인할 수 있으나, **새 버전 적용은 전적으로 수동**이다:

1. GitHub Releases에서 바이너리 다운로드
2. (선택) 체크섬 수동 비교
3. systemd/launchd 서비스 중지
4. 바이너리 교체
5. 서비스 재시작
6. 로그/health check 수동 확인
7. (실패 시) 이전 바이너리 수동 복구

### 운영 부담 증가 사례

최근 SPEC-STORE-003 v0.3.0의 BREAKING 변경(`allow_dynamic_keys` → `registration_type` clean rename, yaml 마이그레이션 필수)은 운영자에게 다음을 요구했다:

- 사전 yaml 변환 스크립트 적용
- 바이너리 교체와 yaml 변환의 동기화
- 부팅 실패 시 즉시 롤백 (수동 백업/복구 의존)

자동 업데이트가 있다면 이러한 BREAKING 릴리즈에서도 **사전 검증 → 단일 명령 적용 → 실패 시 자동 롤백** 흐름이 가능하다.

### 기술적 제약

- Go 1.22+ 기존 스택 유지 (신규 언어/런타임 도입 없음)
- POSIX 환경 우선 (Linux primary, macOS secondary; Windows는 별도 SPEC)
- systemd / launchd 등 supervisor 환경에서도 동작 (Type=notify 호환 고려)
- 외부 업데이트 서버 인프라를 추가로 운영하지 않음 (GitHub Releases 활용)

### SDD 2025 Constitution 정합성

- **언어**: Go 1.22+ (신규 외부 의존성 최소화)
- **금지 라이브러리 검토**: `github.com/inconshreveable/go-update`는 표준 패턴으로 널리 사용되며, atomic replacement에 한정 사용 → Constitution 충돌 없음
- **보안**: HTTPS 전용 + 공개키 핀닝 + Ed25519 서명 검증 → OWASP Software Update 권고 사항 준수
- **로깅**: `log/slog` (stdlib) 기반 구조화 로깅 (기존 프로젝트 패턴)
- **에러 모델**: `Err...` 네이밍 (기존 패턴 보존)

## EARS 요구사항 (EARS Requirements)

본 SPEC은 14개 EARS 모듈로 구성된다.

---

### M1: 업데이트 채널 (Update Channel)

- **Ubiquitous**: 시스템은 기본 업데이트 채널로 GitHub Releases를 지원해야 한다. 기본 저장소 URL은 빌드 시점에 임베드되며 (예: `https://api.github.com/repos/xtra72/xflow/releases`), 설정에서 재정의 가능하다.
- **Ubiquitous**: 시스템은 채널 선택을 위한 enum 필드를 지원해야 한다. 허용 값: `"stable"`, `"beta"`, `"nightly"`. 기본값: `"stable"`.
- **State-driven**: IF 채널이 `"stable"`이면, THEN 시스템은 `releases/latest`를 사용한다. IF `"beta"` 또는 `"nightly"`이면, THEN 해당 prerelease 태그 패턴(예: `*-beta.*`, `*-nightly.*`)을 갖는 가장 최근 릴리즈를 사용한다.
- **Unwanted**: WHEN 설정의 `update_url`이 `http://` 스킴이면 (HTTPS가 아니면), THEN 시스템은 부팅 또는 update 명령 실행을 거부하고 `ErrUpdateChannelInvalid`를 반환해야 한다.
- **Unwanted**: WHEN 채널 enum 값이 `"stable"`/`"beta"`/`"nightly"` 외의 임의 문자열이면, THEN 설정 로드를 실패시키고 명시적 에러를 반환해야 한다.

---

### M2: 버전 확인 (Version Check)

- **Event-driven**: WHEN 사용자가 `xflowd update check` 명령을 실행하면, THEN 시스템은 채널에서 최신 릴리즈 메타데이터를 가져와 현재 버전과 비교하고 결과를 반환해야 한다.
- **Event-driven**: WHEN POST `/api/v1/system/update/check` 요청이 인증된 호출자로부터 오면, THEN 시스템은 동일 로직으로 결과를 JSON으로 반환해야 한다.
- **State-driven**: IF `update.enabled == true` AND `check_interval > 0`, THEN 시스템은 주기적으로(기본 24시간) 자동 버전 확인을 수행하고 결과를 로그·내부 상태에 기록해야 한다.
- **Ubiquitous**: 응답 페이로드는 `current_version`, `latest_version`, `update_available` (bool), `release_notes_url` (string), `published_at` (RFC3339)을 포함해야 한다.
- **Unwanted**: WHEN 채널 fetch가 네트워크 오류 또는 5xx 상태로 실패하면, THEN 시스템은 캐시된 마지막 결과를 fallback으로 사용하고 명시적 에러 로그를 남겨야 한다 (정상 동작은 영향 없음).

---

### M3: 다운로드 (Download)

- **Event-driven**: WHEN 사용자가 `xflowd update apply` 명령을 실행하면 OR 자동 적용 모드(`auto_apply: true`)가 활성화되어 새 버전이 발견되면, THEN 시스템은 임시 디렉토리(`$TMPDIR` 또는 `/tmp`)에 새 바이너리, 체크섬 파일, 서명 파일을 다운로드해야 한다.
- **Ubiquitous**: 다운로드 대상은 현재 OS/아키텍처에 일치하는 단일 asset이어야 한다 (예: `xflowd-linux-amd64`, `xflowd-darwin-arm64`).
- **Ubiquitous**: 시스템은 다운로드 진행 상황을 보고해야 한다. CLI는 진행률(%) + 다운로드 속도, REST API는 status 엔드포인트로 폴링 가능한 상태 객체.
- **Unwanted**: WHEN 다운로드 시작 전 임시 디렉토리의 가용 디스크 공간이 (바이너리 크기 × 3) 미만이면, THEN 시스템은 다운로드를 시작하지 않고 `ErrUpdateInsufficientDiskSpace`를 반환해야 한다 (백업 + 새 바이너리 + 임시 여유 공간 확보).
- **Unwanted**: WHEN 다운로드 중 네트워크 오류가 발생하면, THEN 시스템은 임시 파일을 폐기하고 `ErrUpdateDownloadFailed`를 반환해야 한다 (재개 시도 없음, 다음 호출에서 새로 시작).
- **Ubiquitous**: 다운로드된 임시 파일은 권한 `0600` (소유자 읽기/쓰기만)으로 생성되어야 한다.

---

### M4: 검증 (Verification)

- **Ubiquitous**: 시스템은 다운로드된 바이너리에 대해 SHA256 체크섬을 계산하고, 채널이 제공하는 `checksums.txt` 또는 동등한 파일의 값과 비교해야 한다.
- **Ubiquitous**: 시스템은 다운로드된 바이너리에 대해 Ed25519 디지털 서명을 검증해야 한다. 공개키는 빌드 시점에 코드에 임베드(공개키 핀닝)되며 채널이 제공하는 `signature.bin`을 입력으로 사용한다.
- **Decision Point**: 서명 알고리즘은 **Ed25519**를 채택한다 (대안 검토: GPG는 복잡성 + 키 관리 부담, Sigstore/cosign은 외부 의존성 + OSS 인프라 종속). Ed25519는 Go stdlib(`crypto/ed25519`)으로 충분하며 공개키 핀닝과 결합 시 충분한 신뢰성 제공.
- **Unwanted**: WHEN 체크섬 비교 실패하면, THEN 시스템은 다운로드 파일을 즉시 삭제하고 `ErrUpdateChecksumMismatch`를 반환해야 한다.
- **Unwanted**: WHEN Ed25519 서명 검증 실패하면, THEN 시스템은 다운로드 파일을 즉시 삭제하고 `ErrUpdateSignatureInvalid`를 반환해야 한다 (체크섬은 통과했으나 서명 실패는 잠재적 침해 신호 → 명시적 보안 로그 + 알림).
- **Ubiquitous**: 검증 단계는 다운로드 직후 + 원자적 교체 직전에 모두 수행되어야 한다 (TOCTOU 공격 방어).

---

### M5: 원자적 교체 (Atomic Replacement)

- **Ubiquitous**: 시스템은 검증된 새 바이너리를 임시 위치에서 최종 위치(현재 실행 중 바이너리 경로)로 **POSIX `rename(2)` syscall**을 사용해 atomic move해야 한다.
- **Ubiquitous**: 교체 직전에 시스템은 현재 실행 바이너리를 `<binary>.previous`로 백업해야 한다 (롤백 대비, M7 참조).
- **State-driven**: IF 운영체제가 Linux 또는 macOS이면, THEN POSIX `rename(2)` 또는 `os.Rename`(Go stdlib, 동일 디렉토리 내에서 atomic 보장)을 사용한다.
- **Unwanted**: WHEN 교체 작업이 실패하면 (디스크 가득, 권한 부족 등), THEN 시스템은 백업 파일을 그대로 두고 임시 파일을 폐기하며, 명시적 에러를 반환해야 한다. 운영자가 수동 개입할 수 있도록 백업 파일 위치를 로그에 명시.
- **Ubiquitous**: 교체 작업은 동일 파일시스템 내에서 수행되어야 한다 (cross-device rename 회피). 임시 디렉토리는 바이너리 위치와 같은 마운트 포인트에 있어야 한다 (또는 fallback으로 같은 디렉토리 내 임시 파일 사용).
- **Unwanted**: 시스템은 교체 작업 도중 인터럽트(SIGKILL 등)가 발생하더라도 파일시스템 일관성을 유지해야 한다 (atomic rename의 원자성 보장).

---

### M6: 그레이스풀 재시작 (Graceful Restart)

- **Event-driven**: WHEN 바이너리 교체가 완료되면, THEN 시스템은 graceful shutdown 시퀀스를 시작해야 한다.
- **Ubiquitous**: graceful shutdown은 다음 단계를 순차 실행한다:
  1. 신규 인입 요청 차단 (HTTP listener의 `Shutdown(ctx)`)
  2. in-flight HTTP 요청 완료 대기 (timeout 30초, 설정 가능)
  3. in-flight FBP 메시지/플로우 drain (timeout 30초, 설정 가능)
  4. 영속화 레이어 flush (Store 등 작업 중인 데이터 commit)
- **Ubiquitous**: drain 완료 후 시스템은 `syscall.Exec` 또는 supervisor 신호(systemd `Type=notify`의 경우 SIGTERM 후 systemd가 재시작)를 사용해 새 바이너리로 전환해야 한다.
- **Unwanted**: WHEN drain timeout이 초과되면, THEN 시스템은 강제 종료를 수행하되 손실된 in-flight 메시지/요청 카운트를 명시적으로 로그에 기록해야 한다.
- **State-driven**: IF 환경이 systemd notify 모드이면, THEN 시스템은 `READY=1` / `STOPPING=1` notification을 적절히 발송해야 한다 (supervisor와의 정합성).
- **Ubiquitous**: 새 프로세스 시작 후 5초(설정 가능) 안에 health check 응답이 정상이어야 한다 (M7 자동 롤백 트리거 조건).

---

### M7: 롤백 (Rollback)

- **Ubiquitous**: 시스템은 매 update apply마다 이전 바이너리를 `<binary>.previous`로 백업해야 한다 (M5 참조).
- **Event-driven**: WHEN 새 바이너리가 시작 후 5초(설정 가능) 안에 health check 응답을 못 하면, THEN supervisor 또는 워치독 프로세스가 자동 롤백을 트리거해야 한다.
- **Ubiquitous**: 자동 롤백 절차:
  1. 새 프로세스 강제 종료 (timeout 후)
  2. `<binary>.previous` → 원래 위치로 atomic rename (역방향 복원)
  3. supervisor 재시작 또는 `syscall.Exec` 이전 바이너리
  4. health check 재확인
- **Event-driven**: WHEN 사용자가 `xflowd update rollback` 명령을 실행하면, THEN 시스템은 `<binary>.previous`가 존재하는 경우 동일 절차로 명시적 롤백을 수행해야 한다.
- **Unwanted**: WHEN `<binary>.previous`가 존재하지 않거나 손상되었으면, THEN 자동 롤백 시도는 `ErrUpdateRollbackFailed`로 실패하고 운영자 개입이 필요하다는 명시적 알림(critical log + 가능 시 webhook/이메일 통합 - 미래 확장)을 발송해야 한다.
- **Unwanted**: 시스템은 롤백 후 추가 자동 update apply를 일시 중지해야 한다 (loop 방지). 운영자가 명시적으로 lock을 해제하기 전까지 자동 업데이트는 비활성.
- **Ubiquitous**: 롤백 결과(성공/실패)는 구조화 로그 이벤트 `update.rollback`로 기록되어야 한다.

---

### M8: 다운그레이드 방지 (Anti-Downgrade)

- **Unwanted**: WHEN update apply 대상 버전이 현재 버전보다 낮으면 (semver 비교), THEN 시스템은 `--force` flag 없이는 적용을 거부하고 `ErrDowngradeRequiresForce`를 반환해야 한다.
- **Event-driven**: WHEN 사용자가 `--force` flag와 함께 명시적으로 다운그레이드를 요청하면, THEN 시스템은 명시적 경고 메시지를 출력하고 인터랙티브 확인(또는 `--yes` flag)을 요구해야 한다.
- **Ubiquitous**: 다운그레이드 시도(성공/실패 무관)는 구조화 로그 이벤트 `update.downgrade_attempt`로 기록되어야 한다.
- **Ubiquitous**: 자동 적용 모드(`auto_apply: true`)는 다운그레이드를 절대 자동으로 수행하지 않아야 한다 (운영자 명시 동의 필수).

---

### M9: CLI 명령 (CLI Commands)

- **Ubiquitous**: 시스템은 다음 CLI 서브커맨드를 제공해야 한다:
  - `xflowd update check`: 새 버전 확인 (M2)
  - `xflowd update apply [--version <ver>] [--force] [--yes]`: 업데이트 적용 (M3-M6)
  - `xflowd update status`: 진행 중인 업데이트 또는 마지막 결과 조회
  - `xflowd update rollback`: 명시적 롤백 (M7)
  - `xflowd update channel <stable|beta|nightly>`: 채널 변경 (런타임 설정 갱신, M1)
- **Ubiquitous**: 각 명령은 결과를 stdout(human-readable 또는 `--json` flag로 JSON)으로 출력하고 exit code로 성공(0) / 일반 실패(1) / 검증 실패(2) / 롤백 실패(3) / 다운그레이드 거부(4)를 반환해야 한다.
- **Event-driven**: WHEN `--json` flag가 지정되면, THEN 모든 출력은 단일 JSON 객체로 직렬화되어야 한다 (스크립팅/자동화 친화).
- **Unwanted**: WHEN 인증되지 않은 사용자(non-root, 적절 권한 없음)가 update apply를 실행하면, THEN 명시적 권한 에러를 반환해야 한다 (바이너리 경로 권한 검사).

---

### M10: REST API 엔드포인트 (REST API Endpoints)

- **Ubiquitous**: 시스템은 다음 REST API 엔드포인트를 제공해야 한다:
  - `GET /api/v1/system/version`: 현재 버전 정보 조회 (인증 선택, default 인증 필요)
  - `POST /api/v1/system/update/check`: 새 버전 확인 (인증 필수)
  - `POST /api/v1/system/update/apply`: 업데이트 적용 (인증 필수, body로 옵션 전달)
  - `GET /api/v1/system/update/status`: 진행 중 업데이트 상태 조회 (인증 필수)
  - `POST /api/v1/system/update/rollback`: 롤백 트리거 (인증 필수)
- **Unwanted**: WHEN 인증되지 않은 호출자가 update 관련 엔드포인트를 호출하면, THEN 시스템은 HTTP 401 Unauthorized를 반환해야 한다.
- **Unwanted**: WHEN 권한 부족(인증은 됐으나 update 권한 없음)이면, THEN HTTP 403 Forbidden을 반환해야 한다.
- **Ubiquitous**: 응답은 일관된 JSON 형식으로 직렬화되며 `success`, `data`, `error` 필드 패턴을 따라야 한다 (기존 API 컨벤션).
- **State-driven**: IF 동시에 두 개의 update apply 호출이 들어오면, THEN 두 번째 호출은 HTTP 409 Conflict를 반환해야 한다 (mutex/lock 직렬화).

---

### M11: 설정 (Configuration)

- **Ubiquitous**: 시스템은 xflowd 설정에 `update` 섹션을 지원해야 한다. 필드:
  - `enabled` (bool, default `false`): 자동 업데이트 활성화
  - `channel` (enum: `stable`/`beta`/`nightly`, default `stable`)
  - `check_interval` (duration, default `24h`, 0이면 자동 확인 비활성)
  - `auto_apply` (bool, default `false`): true면 새 버전 발견 시 자동 적용
  - `update_url` (string, optional): 채널 URL 재정의 (기본: GitHub Releases API)
  - `public_key_path` (string, optional): 공개키 파일 경로 (기본: 임베드된 키 사용)
  - `drain_timeout` (duration, default `30s`): graceful shutdown 시 drain 최대 대기
  - `health_check_timeout` (duration, default `5s`): 새 프로세스 health check 대기
  - `notify_only` (bool, default `false`): true면 자동 적용 없이 알림만 (auto_apply와 상호 배타)
- **State-driven**: IF `enabled == false`이면, THEN 모든 자동 동작(주기 확인, 자동 적용)은 비활성화되며 명시적 CLI/API 호출만 동작한다.
- **State-driven**: IF `auto_apply == true` AND `notify_only == true`이면, THEN 설정 로드 실패 (상호 배타 검증).
- **Event-driven**: WHEN 설정이 reload되면, THEN 새 설정이 즉시 반영되어야 한다 (재시작 불필요).

---

### M12: 로깅 및 관찰성 (Logging & Observability)

- **Ubiquitous**: 모든 update 관련 동작은 `log/slog` 기반 구조화 로그로 기록되어야 한다. 이벤트 타입:
  - `update.check`: 버전 확인 시도/결과
  - `update.download`: 다운로드 시작/진행/완료/실패
  - `update.verify`: 체크섬/서명 검증 결과
  - `update.apply`: 교체 + 재시작 트리거
  - `update.rollback`: 롤백 트리거/결과
  - `update.downgrade_attempt`: 다운그레이드 시도 (성공/거부 무관)
- **Ubiquitous**: 각 이벤트는 최소한 `event_type`, `current_version`, `target_version`, `result` (success/failure/in_progress), `duration_ms`, `error` (실패 시) 필드를 포함해야 한다.
- **Optional**: WHERE Prometheus 등 메트릭 시스템 통합이 활성화되면, THEN 카운터 메트릭을 노출한다:
  - `xflowd_update_checks_total{result}`
  - `xflowd_update_applied_total{result}`
  - `xflowd_update_rollbacks_total{trigger}` (auto/manual)
  - `xflowd_update_download_bytes_total`
- **Ubiquitous**: 보안 관련 실패(서명 검증 실패 등)는 별도 보안 감사 로그로도 기록되어야 한다 (운영 모니터링 통합 용이).

---

### M13: 보안 (Security)

- **Ubiquitous**: 모든 채널 fetch + 바이너리/체크섬/서명 다운로드는 HTTPS 전용이어야 한다 (M1 강제).
- **Ubiquitous**: 시스템은 공개키 핀닝(pinned public key)을 사용해야 한다. 공개키는 빌드 시점에 Go embed 또는 빌드 변수로 코드에 포함되며, 운영자는 검증된 빌드 외 공개키 변경을 할 수 없다.
- **Unwanted**: WHEN 채널 서버의 TLS 인증서가 자가 서명 또는 신뢰할 수 없으면 (default), THEN 다운로드를 거부해야 한다. 사용자가 명시적으로 `insecure_skip_verify` flag를 설정한 경우에만 허용 (개발용 옵션, 명시적 경고 로그).
- **Unwanted**: 시스템은 검증되지 않은(체크섬 또는 서명 실패) 바이너리를 어떠한 경로로도 실행하지 않아야 한다 (zero unverified execution).
- **Ubiquitous**: 다운로드된 바이너리는 검증 통과 후에만 실행 권한(`0755`)으로 변경되며, 검증 전에는 `0600` (실행 불가)으로 유지된다.
- **Ubiquitous**: Replay 공격 방어를 위해 채널 응답에 release 메타데이터의 `published_at` 타임스탬프를 검증한다 (현재 버전 빌드 시점보다 이전이면 의심 → 경고 로그).

---

### M14: 멀티 바이너리 조정 (Multi-binary Coordination, Optional)

- **Optional**: WHERE 운영 환경에서 xflowd, xflow-agent, xflow CLI를 함께 업데이트해야 하면, THEN 본 SPEC은 xflowd 자가 교체에 한정하고 xflow-agent / xflow CLI는 후속 SPEC(`SPEC-UPDATE-002`, `SPEC-UPDATE-003`)에서 동일 패턴으로 다룬다.
- **Optional**: WHERE 사용자가 통합된 업데이트 흐름을 원하면, THEN 미래 SPEC에서 orchestrated update를 도입한다 (xflowd가 xflow-agent 업데이트를 조율하는 패턴).
- **Ubiquitous**: 본 SPEC의 `internal/updater` 패키지는 xflow-agent / xflow CLI에서 재사용 가능하도록 의존성 없이 설계되어야 한다 (특정 데몬에 강결합 회피).

---

## 명세 (Specifications)

### Config 스키마 (xflowd config)

```yaml
update:
  enabled: true                    # 자동 업데이트 기능 활성화
  channel: "stable"                # stable | beta | nightly
  check_interval: "24h"            # 0 = 자동 확인 비활성
  auto_apply: false                # true면 새 버전 발견 시 자동 적용
  notify_only: true                # true면 알림만 (auto_apply와 상호 배타)
  update_url: ""                   # 빈 문자열 = 기본 GitHub Releases 사용
  public_key_path: ""              # 빈 문자열 = 임베드된 공개키 사용
  drain_timeout: "30s"             # graceful shutdown 시 drain 최대 대기
  health_check_timeout: "5s"       # 새 프로세스 health check 대기
  insecure_skip_verify: false      # 개발용; production 절대 true 금지
```

### CLI 명령 시그니처

```text
xflowd update check [--json]
  → 새 버전 확인. exit 0 (최신) / 1 (네트워크 오류) / 2 (업데이트 가능)

xflowd update apply [--version <ver>] [--force] [--yes] [--json]
  → 업데이트 적용. --force는 다운그레이드 허용, --yes는 인터랙티브 건너뜀
  → exit 0 (성공) / 1 (일반 실패) / 2 (검증 실패) / 3 (롤백 실패) / 4 (다운그레이드 거부)

xflowd update status [--json]
  → 진행 중 또는 마지막 update 상태. exit 0 (정상) / 1 (조회 실패)

xflowd update rollback [--yes] [--json]
  → 명시적 롤백. exit 0 (성공) / 3 (롤백 실패)

xflowd update channel <stable|beta|nightly>
  → 런타임 채널 변경. 다음 check부터 적용. exit 0 (성공) / 1 (잘못된 값)
```

### REST API 엔드포인트

**`GET /api/v1/system/version`** (인증 필요):

```json
{
  "success": true,
  "data": {
    "version": "v0.3.0",
    "commit": "6b9c531",
    "build_date": "2026-05-04T12:34:56Z",
    "go_version": "go1.22.5",
    "os": "linux",
    "arch": "amd64"
  }
}
```

**`POST /api/v1/system/update/check`** (인증 필요):

요청 바디 없음. 응답:

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

**`POST /api/v1/system/update/apply`** (인증 필요):

요청 바디:

```json
{
  "version": "v0.4.0",   // optional: 명시적 타겟. 없으면 latest
  "force": false,         // optional: 다운그레이드 허용
  "skip_confirm": true    // 자동화 호출의 경우 true
}
```

응답 (비동기 트리거 + status 폴링 권장):

```json
{
  "success": true,
  "data": {
    "operation_id": "upd-2026-05-05-001",
    "status": "downloading",
    "current_version": "v0.3.0",
    "target_version": "v0.4.0",
    "started_at": "2026-05-05T10:00:00Z"
  }
}
```

**`GET /api/v1/system/update/status`** (인증 필요):

```json
{
  "success": true,
  "data": {
    "operation_id": "upd-2026-05-05-001",
    "status": "verifying",     // idle | downloading | verifying | applying | restarting | rolling_back | success | failed
    "progress_percent": 75,
    "current_version": "v0.3.0",
    "target_version": "v0.4.0",
    "started_at": "2026-05-05T10:00:00Z",
    "error": null
  }
}
```

**`POST /api/v1/system/update/rollback`** (인증 필요): 즉시 롤백 트리거.

### 에러 모델

신규 에러 타입:

- `ErrUpdateChannelInvalid`: 채널 URL/이름이 유효하지 않음 (HTTP 스킴, enum 외 값)
- `ErrUpdateDownloadFailed`: 다운로드 실패 (네트워크, 서버 오류)
- `ErrUpdateInsufficientDiskSpace`: 임시 디렉토리의 가용 공간 부족
- `ErrUpdateChecksumMismatch`: SHA256 체크섬 불일치
- `ErrUpdateSignatureInvalid`: Ed25519 서명 검증 실패
- `ErrUpdateApplyFailed`: 원자적 교체 실패 (권한, 파일시스템)
- `ErrUpdateRollbackFailed`: 롤백 시도 실패 (백업 파일 없음/손상)
- `ErrDowngradeRequiresForce`: 다운그레이드 시도에 `--force` 미지정
- `ErrUpdateInProgress`: 동시 update apply 호출 거부 (HTTP 409)

각 에러는 `errors.Is(err, ErrXxx)`로 비교 가능하도록 sentinel 또는 wrapping 패턴 사용.

## 관련 SPEC (Related SPECs)

- **SPEC-CLI-001 / -002 / -003**: CLI 명령 패턴 (xflow / xflowd 공통 컨벤션)
- **SPEC-API-001**: REST API 패턴 + 인증/응답 형식
- **SPEC-CFG-001**: 설정 시스템 (yaml 파싱 + reload)
- **SPEC-OBS-001 / -002**: 관찰성 + 구조화 로깅 (slog 통합)
- **SPEC-LIFE-001**: 런타임 생명주기 (graceful shutdown 패턴 재사용)
- **SPEC-WEB-005 v0.6.0 (예정)**: SystemStatusPanel UI에 버전 정보 + 업데이트 인디케이터 통합 (별도 프론트엔드 SPEC)
- **SPEC-UPDATE-002 (미래)**: xflow-agent 자가 업데이트 (본 SPEC 패턴 재사용)
- **SPEC-UPDATE-003 (미래)**: xflow CLI 자가 업데이트

## TAG Traceability

- `@SPEC:SPEC-UPDATE-001` → spec.md (이 문서)
- `@PLAN:SPEC-UPDATE-001` → plan.md
- `@ACCEPTANCE:SPEC-UPDATE-001` → acceptance.md
- 구현 경로 (예상):
  - `internal/updater/types.go` (Update, Version, Channel, Manifest, Status enum)
  - `internal/updater/checker.go` (GitHub Releases API + 버전 비교)
  - `internal/updater/downloader.go` (HTTPS 다운로드 + 진행률)
  - `internal/updater/verifier.go` (SHA256 + Ed25519)
  - `internal/updater/applier.go` (atomic replacement)
  - `internal/updater/restarter.go` (graceful + syscall.Exec)
  - `internal/updater/rollback.go` (백업 + 복구)
  - `internal/updater/scheduler.go` (주기 확인)
  - `cmd/xflowd/update.go` (CLI 서브커맨드)
  - `internal/api/handler/system_update.go` (REST API 핸들러)
  - `internal/api/dto/update.go` (요청/응답 DTO)
  - `internal/config/update.go` (설정 섹션 + 검증)

## Implementation Notes

### Decision Point: 서명 알고리즘

본 SPEC은 **Ed25519 + 공개키 핀닝**을 채택한다. 대안 비교:

| 방식 | 장점 | 단점 | 채택 여부 |
|------|------|------|-----------|
| **Ed25519** | Go stdlib(`crypto/ed25519`), 작은 키/서명 크기, 빠른 검증 | 키 회전 시 빌드 재발행 필요 | **채택** |
| GPG | 표준 PKI 인프라, 키 관리 도구 풍부 | 외부 도구(`gpg`) 의존, 통합 복잡 | 미채택 |
| Sigstore (cosign) | OSS 신뢰성, transparency log | 외부 인프라 의존, 학습 곡선, 신규 도입 비용 | 미래 검토 |

**키 관리**: 빌드 시 비공개키는 GitHub Actions secrets에 보관, 공개키는 `internal/updater/keys/public.pem` 또는 빌드 변수로 임베드. 키 회전 정책은 운영 문서(`docs/security/key-rotation.md`, 미래 작성)에 별도 명세.

### Decision Point: Atomic Replacement 라이브러리

**`github.com/inconshreveable/go-update` 채택**:

- 장점: 표준 패턴, 1.5K+ stars, atomic rename 추상화, rollback 지원, Linux/macOS/Windows 호환
- 단점: 활발한 개발은 멈춤(1년+), 의존성 추가
- 대안 검토: 직접 구현 (`os.Rename` + `os.Chmod` 조합) — 로직이 단순하나 cross-platform edge case 처리 부담
- **결정**: go-update 사용 + 핵심 로직 wrapper로 격리하여 미래 교체 가능성 보존

### Decision Point: graceful restart 메커니즘

**`syscall.Exec` 직접 호출**:

- 장점: 동일 PID 유지 (systemd integration 친화), 메모리/소켓 효율적 인계 가능
- 단점: in-flight 메시지/요청 인계 어려움 → drain timeout 후 강제 종료 fallback
- 대안: supervisor에 SIGTERM → supervisor가 재시작 — systemd Type=notify에서 자연스러우나 supervisor 의존
- **결정**: 양쪽 모드 지원. systemd 환경 감지 시 SIGTERM, 기타는 syscall.Exec.

### 운영체제 호환성

| OS | 우선순위 | 특이사항 |
|----|----------|----------|
| Linux (x86_64, arm64) | Primary | systemd Type=notify 기본 지원, POSIX rename atomic |
| macOS (x86_64, arm64) | Secondary | launchd 환경 호환, Apple notarization은 별도 SPEC (서명 + entitlement) |
| Windows | Out of scope | 별도 SPEC에서 `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` + Service Control Manager 통합 필요 |

### Future Extensions (본 SPEC 범위 외)

- **Pre/post-update hooks**: 업데이트 전후 외부 명령 실행 (예: yaml 마이그레이션 스크립트, config 백업)
- **Telemetry (opt-in)**: 익명 update 결과 통계 (성공률, 실패 원인 분포) — 별도 privacy SPEC 필요
- **Multi-binary orchestrated update**: xflowd가 xflow-agent 업데이트 조율 (SPEC-UPDATE-002+)
- **Web UI 통합**: SystemStatusPanel 버전 정보 + "업데이트 가능" 배지 + 한 클릭 적용 (SPEC-WEB-005 v0.6.0)
- **Differential update**: bsdiff 기반 부분 패치로 다운로드 크기 축소 (현재 단일 바이너리는 ~25MB로 충분히 작음 → 우선순위 낮음)
- **Update window**: 운영 시간 외에만 자동 적용 (cron-like 표현)
