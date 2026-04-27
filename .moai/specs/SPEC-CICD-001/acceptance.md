---
id: SPEC-CICD-001
type: acceptance
version: "1.0.0"
status: draft
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
---

# SPEC-CICD-001 인수 기준 (Acceptance Criteria)

본 문서는 SPEC-CICD-001의 각 요구사항에 대한 검증 가능한 인수 시나리오를 Given-When-Then 형식으로 정의한다. 모든 인수 기준은 수동 또는 자동화된 스모크 테스트로 확인 가능해야 한다.

---

## 공통 전제 조건 (Common Preconditions)

모든 시나리오는 다음을 가정한다:

- 리포지토리: `github.com/xtra/xflow` (또는 fork된 동일 구조)
- `main` 브랜치에 CI/CD 설정(ci.yml, release.yml, Dockerfile, dependabot.yml)이 이미 머지됨
- Go 1.25.6, Node.js 20 LTS가 설치된 개발 환경
- `GITHUB_TOKEN`은 GitHub Actions가 자동 제공
- `ghcr.io`는 `github.actor` 기본 권한으로 접근 가능

---

## AC1: PR 생성 시 CI 워크플로우 자동 실행

**요구사항 참조**: REQ-CICD-001-EV-01, REQ-CICD-001-UB-06

```gherkin
기능: PR 개설 시 CI 파이프라인 자동 실행 및 품질 게이트 검증

  배경:
    주어진 `ci.yml`이 `.github/workflows/`에 커밋되어 있고
    그리고 `main` 브랜치에서 파생된 작업 브랜치가 존재하고
    그리고 작업 브랜치에 의미 있는 코드 변경(Go 또는 TypeScript)이 있다

  시나리오: PR 생성 시 CI 실행 및 통과
    주어진 개발자가 작업 브랜치에서 코드 변경을 커밋하고 원격에 푸시했을 때
    만약 `main`을 대상으로 Pull Request를 생성하면
    그러면 `ci.yml` 워크플로우가 자동 트리거되어야 한다
    그리고 다음 Step이 순서대로 실행되어야 한다:
      | Step            | 기대 결과                          |
      | checkout        | 소스 체크아웃 성공                 |
      | setup-go        | Go 1.25.x 설치 및 모듈 캐시 활성화 |
      | setup-node      | Node 20 설치 및 npm 캐시 활성화    |
      | go vet          | 경고 0개로 통과                    |
      | golangci-lint   | 통과                               |
      | go test -race   | 모든 패키지 테스트 통과            |
      | npm ci & build  | web/dist 생성                      |
      | govulncheck     | HIGH/CRITICAL 없음 확인            |
    그리고 워크플로우 전체 소요 시간이 30분을 초과하지 않아야 한다
    그리고 최종 Conclusion이 "success"여야 한다
    그리고 PR 페이지에서 "All checks have passed" 상태를 표시해야 한다

  시나리오: PR 갱신(재푸시) 시 CI 재실행
    주어진 기존 PR에 새로운 커밋을 추가 푸시할 때
    만약 `synchronize` 이벤트가 발생하면
    그러면 이전 실행이 취소되고 새 CI 런이 시작되어야 한다
    그리고 동일한 품질 게이트가 재검증되어야 한다

  시나리오: CI 실패 시 머지 차단
    주어진 PR에 실패하는 테스트가 포함되어 있을 때
    만약 CI가 실행되면
    그러면 `go test` Step에서 실패가 발생해야 한다
    그리고 PR의 "Required" 상태 체크가 실패로 표시되어야 한다
    그리고 (브랜치 보호 규칙이 설정된 경우) 머지 버튼이 비활성화되어야 한다
```

**검증 방법**:

- GitHub UI의 Actions 탭에서 워크플로우 실행 확인
- PR 페이지의 Checks 섹션에서 상태 확인
- 실제 실패 테스트를 PR로 제출하여 차단 동작 확인

---

## AC2: 태그 푸시 시 GitHub Release 자동 발행

**요구사항 참조**: REQ-CICD-001-EV-03, REQ-CICD-001-UB-01, REQ-CICD-001-UB-02, REQ-CICD-001-UB-03

```gherkin
기능: v*.*.* 태그 푸시 시 6개 플랫폼 릴리스 자동 생성

  배경:
    주어진 `main` 브랜치에 모든 CI 체크가 통과한 상태로 커밋이 있고
    그리고 `release.yml` 워크플로우가 저장소에 구성되어 있다

  시나리오: v0.1.0 태그 푸시 시 완전한 릴리스 생성
    주어진 `main` 브랜치 최신 커밋에 대해 다음 명령을 실행할 때:
      """
      git tag -a v0.1.0 -m "Release v0.1.0"
      git push origin v0.1.0
      """
    만약 태그가 GitHub에 푸시되면
    그러면 `release.yml` 워크플로우가 자동 트리거되어야 한다
    그리고 `quality-gate` Job이 먼저 실행되어 통과해야 한다
    그리고 `build-frontend` Job이 `web/dist` 아티팩트를 생성해야 한다
    그리고 `build-binaries` Job이 6개 플랫폼 매트릭스로 병렬 실행되어 모두 성공해야 한다
    그리고 `package` Job이 6개 플랫폼별 tar.gz를 조립해야 한다
    그리고 `checksums` Job이 `checksums.txt`를 생성해야 한다
    그리고 `release` Job이 GitHub Release를 발행해야 한다
    그리고 발행된 Release에는 다음 7개 파일이 포함되어야 한다:
      | 파일명                                       |
      | xflow-v0.1.0-darwin-amd64.tar.gz             |
      | xflow-v0.1.0-darwin-arm64.tar.gz             |
      | xflow-v0.1.0-linux-amd64.tar.gz              |
      | xflow-v0.1.0-linux-arm64.tar.gz              |
      | xflow-v0.1.0-linux-arm-v7.tar.gz             |
      | xflow-v0.1.0-linux-arm-v6.tar.gz             |
      | checksums.txt                                |
    그리고 Release 페이지에 자동 생성된 릴리스 노트가 포함되어야 한다
    그리고 태그 이름이 `v0.1.0`이고 Release 이름이 `xflow v0.1.0`이어야 한다
    그리고 `prerelease` 플래그가 `false`여야 한다

  시나리오: 프리릴리스 태그 식별
    주어진 태그가 `v1.0.0-rc.1` 형식일 때
    만약 해당 태그를 푸시하면
    그러면 릴리스가 생성되어야 한다
    그리고 해당 Release는 `prerelease: true`로 표시되어야 한다
    그리고 Docker `:latest` 태그는 갱신되지 않아야 한다
    그리고 `ghcr.io/xtra/xflow:v1.0.0-rc.1` 태그만 발행되어야 한다
```

**검증 방법**:

- GitHub Releases 페이지에서 파일 목록 확인
- `gh release view v0.1.0`로 CLI 확인
- 각 tar.gz의 파일 크기가 0보다 큰지 확인

---

## AC3: 패키지 내부 구조 검증

**요구사항 참조**: REQ-CICD-001-UB-02

```gherkin
기능: 각 tar.gz 패키지에 필수 파일이 모두 포함

  시나리오: Linux 패키지 구조 검증
    주어진 `xflow-v0.1.0-linux-amd64.tar.gz`를 다운로드하여 압축 해제했을 때
    그러면 다음 파일이 존재해야 한다:
      | 파일 경로                  | 설명                                |
      | xflowd                     | 서버 데몬 바이너리 (실행 권한)      |
      | xflow                      | CLI 바이너리 (실행 권한)            |
      | xflow-agent                | 에이전트 CLI (실행 권한)            |
      | web/dist/                  | 프론트엔드 정적 자산 디렉터리       |
      | web/dist/index.html        | 프론트엔드 엔트리 포인트            |
      | xflow.yaml                 | deploy/xflow.yaml에서 복사됨         |
      | xflowd.service             | systemd 유닛 파일                    |
      | install.sh                 | 설치 스크립트 (실행 권한)           |
      | uninstall.sh               | 제거 스크립트 (실행 권한)           |
    그리고 `file xflowd` 실행 결과가 "ELF 64-bit LSB executable, x86-64"를 포함해야 한다
    그리고 `./xflowd --version` 실행 결과가 "v0.1.0"을 포함해야 한다

  시나리오: macOS 패키지 구조 검증
    주어진 `xflow-v0.1.0-darwin-arm64.tar.gz`를 다운로드하여 압축 해제했을 때
    그러면 다음 파일이 존재해야 한다:
      | 파일 경로                  | 설명                                |
      | xflowd                     | 서버 데몬 바이너리 (실행 권한)      |
      | xflow                      | CLI 바이너리                        |
      | xflow-agent                | 에이전트 CLI                        |
      | web/dist/                  | 프론트엔드 정적 자산                |
      | xflow.yaml                 | 기본 설정                            |
    그리고 `xflowd.service`, `install.sh`, `uninstall.sh`는 **포함되지 않아야** 한다 (Linux 전용)
    그리고 `file xflowd` 결과가 "Mach-O 64-bit executable arm64"를 포함해야 한다

  시나리오: RPi ARMv6 패키지 구조 검증
    주어진 `xflow-v0.1.0-linux-arm-v6.tar.gz`를 다운로드했을 때
    그러면 Linux 필수 구성이 모두 포함되어야 한다
    그리고 `file xflowd` 결과가 "ELF 32-bit LSB executable, ARM"을 포함해야 한다
    그리고 바이너리가 RPi Zero에서 실행 가능해야 한다 (수동 검증)
```

**검증 방법**:

```bash
curl -LO https://github.com/xtra/xflow/releases/download/v0.1.0/xflow-v0.1.0-linux-amd64.tar.gz
mkdir -p /tmp/xflow-verify
tar xzf xflow-v0.1.0-linux-amd64.tar.gz -C /tmp/xflow-verify
ls -la /tmp/xflow-verify/
file /tmp/xflow-verify/xflowd
./tmp/xflow-verify/xflowd --version
```

---

## AC4: SHA256 체크섬 검증

**요구사항 참조**: REQ-CICD-001-UB-05, REQ-CICD-001-EV-06

```gherkin
기능: 릴리스의 모든 tar.gz에 대해 체크섬 파일이 검증 가능

  시나리오: 체크섬 파일 형식 검증
    주어진 릴리스 페이지에서 `checksums.txt`를 다운로드했을 때
    그러면 파일은 다음 형식의 라인을 포함해야 한다:
      """
      <64자 hex 해시>  xflow-v0.1.0-<os>-<arch>[-v6|-v7].tar.gz
      """
    그리고 정확히 6개의 해시 라인이 존재해야 한다

  시나리오: 모든 tar.gz에 대한 체크섬 검증 성공
    주어진 6개의 tar.gz 파일과 `checksums.txt`를 동일 디렉터리에 다운로드했을 때
    만약 `shasum -a 256 -c checksums.txt` (또는 `sha256sum -c checksums.txt`)를 실행하면
    그러면 출력에 6개의 "OK" 라인이 표시되어야 한다
    그리고 exit code가 0이어야 한다

  시나리오: 변조된 파일 탐지
    주어진 다운로드된 `xflow-v0.1.0-linux-amd64.tar.gz`의 1바이트를 임의로 변조했을 때
    만약 `shasum -a 256 -c checksums.txt`를 실행하면
    그러면 해당 파일에 대해 "FAILED" 표시가 나타나야 한다
    그리고 exit code가 0이 아니어야 한다
```

**검증 방법**:

```bash
# 모든 릴리스 파일 다운로드
gh release download v0.1.0 --dir ./verify
cd verify
# 체크섬 검증
shasum -a 256 -c checksums.txt
# 또는 Linux
sha256sum -c checksums.txt
```

---

## AC5: Docker 멀티아키 이미지 배포

**요구사항 참조**: REQ-CICD-001-UB-01 (Docker 관련), REQ-CICD-001-ST-03

```gherkin
기능: ghcr.io에 linux/amd64 + linux/arm64 이미지 발행

  시나리오: amd64 호스트에서 Docker 이미지 풀 및 실행
    주어진 x86_64 Linux 또는 Docker Desktop(macOS/Windows) 환경에서
    만약 `docker pull ghcr.io/xtra/xflow:v0.1.0`을 실행하면
    그러면 이미지 풀이 성공해야 한다
    그리고 `docker inspect ghcr.io/xtra/xflow:v0.1.0 --format '{{.Architecture}}'` 결과가 "amd64"여야 한다
    그리고 `docker run --rm ghcr.io/xtra/xflow:v0.1.0 --version` 결과가 "v0.1.0"을 포함해야 한다

  시나리오: arm64 호스트에서 Docker 이미지 풀 및 실행
    주어진 Apple Silicon Mac 또는 AWS Graviton 또는 RPi 4/5(64비트) 환경에서
    만약 `docker pull ghcr.io/xtra/xflow:v0.1.0`을 실행하면
    그러면 이미지 풀이 성공해야 한다
    그리고 `docker inspect ghcr.io/xtra/xflow:v0.1.0 --format '{{.Architecture}}'` 결과가 "arm64"여야 한다
    그리고 컨테이너가 정상 시작되어야 한다

  시나리오: 정식 릴리스의 :latest 태그 갱신
    주어진 v0.1.0이 정식 릴리스(prerelease: false)로 발행될 때
    그러면 `ghcr.io/xtra/xflow:latest`가 v0.1.0 매니페스트를 가리켜야 한다
    그리고 `docker pull ghcr.io/xtra/xflow:latest`가 v0.1.0 내용을 반환해야 한다

  시나리오: 프리릴리스의 :latest 태그 비갱신
    주어진 v1.0.0-rc.1이 프리릴리스로 발행될 때
    그러면 `ghcr.io/xtra/xflow:v1.0.0-rc.1`는 발행되어야 한다
    그리고 `ghcr.io/xtra/xflow:latest`는 이전 정식 릴리스를 유지해야 한다

  시나리오: 이미지 매니페스트에 멀티아키 지원 확인
    주어진 Docker Buildx가 설치된 환경에서
    만약 `docker buildx imagetools inspect ghcr.io/xtra/xflow:v0.1.0`을 실행하면
    그러면 출력에 "linux/amd64"와 "linux/arm64" 매니페스트가 모두 포함되어야 한다
```

**검증 방법**:

```bash
docker pull ghcr.io/xtra/xflow:v0.1.0
docker buildx imagetools inspect ghcr.io/xtra/xflow:v0.1.0
docker run --rm ghcr.io/xtra/xflow:v0.1.0 --version
```

---

## AC6: Dependabot 주간 의존성 업데이트

**요구사항 참조**: REQ-CICD-001-EV-07, REQ-CICD-001-EV-08

```gherkin
기능: Dependabot이 Go 모듈, npm, GitHub Actions 업데이트를 주간 감지

  시나리오: Go 모듈 업데이트 PR 생성
    주어진 `.github/dependabot.yml`에 `gomod` 생태계가 주간 스케줄로 구성되어 있고
    그리고 `go.mod`에 명시된 의존성 중 하나의 마이너/패치 버전이 새로 릴리스되었을 때
    만약 다음 월요일이 도래하면
    그러면 Dependabot이 자동으로 PR을 생성해야 한다
    그리고 PR 제목이 `chore(deps): bump <package> from <old> to <new>` 형식이어야 한다
    그리고 PR에 `dependencies`, `go` 라벨이 부여되어야 한다
    그리고 PR이 CI를 자동으로 통과해야 한다 (의존성 호환성 문제 없을 시)
    그리고 감지부터 PR 생성까지 7일을 초과하지 않아야 한다

  시나리오: npm (web/) 업데이트 PR 생성
    주어진 `web/package.json`의 의존성 중 마이너 업데이트가 존재할 때
    그러면 Dependabot이 `web/` 디렉터리 컨텍스트로 PR을 생성해야 한다
    그리고 PR의 diff가 `web/package.json`과 `web/package-lock.json`을 포함해야 한다

  시나리오: GitHub Actions 업데이트 PR 생성
    주어진 사용 중인 액션(예: `actions/checkout@v4`)의 새 릴리스가 있을 때
    그러면 Dependabot이 `.github/workflows/*.yml` 내 참조를 업데이트하는 PR을 생성해야 한다
    그리고 PR 제목이 `chore(ci): bump actions/checkout from v4 to v5` 형식이어야 한다

  시나리오: 보안 권고 즉시 처리
    주어진 Go 모듈 의존성에 대한 GitHub Security Advisory가 게시되었을 때
    만약 Dependabot이 해당 의존성을 감지하면
    그러면 주간 스케줄과 무관하게 1일 이내 보안 업데이트 PR을 생성해야 한다
    그리고 PR에 "security" 관련 라벨 또는 본문 알림이 포함되어야 한다

  시나리오: 그룹핑 동작 검증
    주어진 같은 주에 여러 Go 모듈의 patch 업데이트가 동시에 발생했을 때
    그러면 `go-minor-patch` 그룹으로 단일 PR로 통합되어야 한다
    그리고 `open-pull-requests-limit: 5` 제한이 적용되어야 한다
```

**검증 방법**:

- GitHub 저장소 Insights → Dependency graph → Dependabot 탭에서 활동 확인
- `gh pr list --author "dependabot[bot]"`로 PR 목록 확인
- 최초 설정 후 1주 대기 후 자동 PR 생성 확인

---

## AC7: govulncheck 취약점 차단

**요구사항 참조**: REQ-CICD-001-UN-02

```gherkin
기능: govulncheck가 HIGH/CRITICAL 취약점 탐지 시 CI/릴리스 차단

  시나리오: 알려진 취약 의존성 도입 시 CI 차단
    주어진 실험용 브랜치에서 알려진 취약점이 있는 Go 패키지를 go.mod에 추가했을 때
    (예: 과거 취약점이 확인된 특정 버전의 의존성)
    만약 해당 브랜치에서 PR을 생성하면
    그러면 `ci.yml`의 `govulncheck` Step이 실패해야 한다
    그리고 워크플로우 Conclusion이 "failure"여야 한다
    그리고 로그에 취약한 함수와 CVE/GHSA 식별자가 출력되어야 한다
    그리고 PR의 해당 체크가 머지 차단 상태가 되어야 한다

  시나리오: 취약점 없는 의존성 그래프에서 통과
    주어진 모든 의존성이 현행 안전 버전일 때
    만약 `govulncheck ./...`를 실행하면
    그러면 "No vulnerabilities found" 또는 동등한 메시지가 출력되어야 한다
    그리고 exit code가 0이어야 한다

  시나리오: 릴리스 워크플로우에서 취약점 차단
    주어진 취약점이 포함된 상태에서 릴리스 태그를 푸시할 때
    그러면 `quality-gate` Job의 `govulncheck` Step이 실패해야 한다
    그리고 `build-binaries`, `package`, `release` Job이 실행되지 않아야 한다
    그리고 GitHub Release가 생성되지 않아야 한다

  시나리오: 비상 override 플래그 (신중 사용)
    주어진 시간 제약으로 취약점 fix가 불가능한 상황에서
    만약 `workflow_dispatch`로 `override_vuln=true` 입력을 제공하면
    그러면 `govulncheck` 실패에도 불구하고 릴리스가 진행되어야 한다
    그리고 릴리스 노트 또는 PR 디스커션에 override 사실이 명시적으로 기록되어야 한다
    그리고 (운영 권고) 정식 릴리스에는 이 플래그를 사용하지 않아야 한다
```

**검증 방법**:

- 테스트 브랜치에서 취약 버전 의존성으로 교체 후 PR
- 로그 출력에서 CVE 식별자 확인
- `gh run view <run-id> --log`로 상세 로그 확인

---

## AC8: 테스트 실패 시 릴리스 아티팩트 차단

**요구사항 참조**: REQ-CICD-001-UN-01, REQ-CICD-001-ST-01

```gherkin
기능: go test 실패 시 릴리스 파이프라인 중단

  시나리오: 테스트 실패로 인한 릴리스 차단
    주어진 특정 패키지에 실패하는 테스트가 포함된 커밋이 `main` 브랜치에 있을 때
    만약 해당 커밋에 `v0.2.0` 태그를 생성하여 푸시하면
    그러면 `release.yml`의 `quality-gate` Job이 트리거되어 실패해야 한다
    그리고 `build-frontend`, `build-binaries` Job이 실행되지 않아야 한다
    그리고 `release` Job이 호출되지 않아야 한다
    그리고 GitHub Releases 페이지에 `v0.2.0` 릴리스가 생성되지 않아야 한다
    그리고 태그 자체는 Git에 존재하지만 리소스가 배포되지 않아야 한다

  시나리오: 테스트 복구 후 재시도
    주어진 실패한 테스트가 수정되어 `main`에 머지된 상태에서
    만약 사용자가 `v0.2.0` 태그를 삭제 후 재생성하면:
      """
      git push --delete origin v0.2.0
      git tag -d v0.2.0
      git tag -a v0.2.0 -m "Release v0.2.0 (retry)"
      git push origin v0.2.0
      """
    그러면 릴리스 워크플로우가 재실행되어야 한다
    그리고 정상적인 릴리스가 발행되어야 한다

  시나리오: 빌드 실패로 인한 차단
    주어진 특정 플랫폼(예: linux/arm)에서만 빌드가 실패하는 CGO 의존성이 도입되었을 때
    만약 릴리스 태그를 푸시하면
    그러면 `build-binaries` Job의 해당 매트릭스 셀이 실패해야 한다
    그리고 후속 Job(`package`, `release`)이 실행되지 않아야 한다
    그리고 릴리스가 발행되지 않아야 한다
    그리고 워크플로우 로그에 실패한 플랫폼이 명확히 식별되어야 한다
```

**검증 방법**:

- 의도적으로 실패하는 테스트를 별도 브랜치에 작성 후 태그 푸시 (테스트 환경 한정)
- GitHub Actions 로그에서 Job 의존성 연쇄 중단 확인
- `gh release list`로 릴리스 미발행 확인

---

## AC9: 수동 workflow_dispatch 프리릴리스 빌드

**요구사항 참조**: REQ-CICD-001-EV-04, REQ-CICD-001-ST-02

```gherkin
기능: GitHub UI 또는 gh CLI로 수동 릴리스 트리거

  시나리오: UI에서 수동 실행
    주어진 저장소 Actions 탭에서 "release.yml"을 선택했을 때
    만약 "Run workflow" 버튼 클릭 후 다음 입력을 제공하면:
      | 입력 필드        | 값            |
      | version         | v0.1.0-rc.1   |
      | prerelease      | true          |
      | override_vuln   | false         |
    그러면 워크플로우가 `main` 브랜치 헤드로 실행되어야 한다
    그리고 태그가 자동 생성되지 않아야 한다 (Git 태그 없음)
    그리고 최종 Release는 `prerelease: true`로 표시되어야 한다
    그리고 Release에 `v0.1.0-rc.1` 이름이 지정되어야 한다
    그리고 6개 플랫폼 tar.gz와 checksums.txt가 업로드되어야 한다
    그리고 Docker 이미지 `ghcr.io/xtra/xflow:v0.1.0-rc.1`만 발행되어야 한다
    그리고 Docker `:latest` 태그는 갱신되지 않아야 한다

  시나리오: gh CLI로 수동 실행
    주어진 GitHub CLI가 인증된 환경에서
    만약 다음 명령을 실행하면:
      """
      gh workflow run release.yml \
        -f version=v0.1.0-rc.2 \
        -f prerelease=true
      """
    그러면 동일한 효과로 프리릴리스 빌드가 생성되어야 한다

  시나리오: 정식 릴리스 수동 발행
    주어진 태그 없이 정식 릴리스를 긴급 발행해야 할 때
    만약 다음 입력으로 수동 실행하면:
      | 입력 필드   | 값       |
      | version     | v0.1.0   |
      | prerelease  | false    |
    그러면 Release가 `prerelease: false`로 발행되어야 한다
    그리고 Docker `:latest` 태그가 갱신되어야 한다
    그리고 (운영 권고) 이후 `git tag v0.1.0 && git push origin v0.1.0`으로 태그를 소급 생성해야 한다
```

**검증 방법**:

- GitHub Actions UI의 Run workflow 기능 사용
- `gh run watch`로 실행 진행 모니터링
- 발행된 Release의 메타데이터 확인

---

## AC10: macOS Gatekeeper 우회 후 실행

**요구사항 참조**: v1 Gatekeeper 제약 문서화, REQ-CICD-001-UB-02 (docs/gatekeeper-bypass.md 포함)

```gherkin
기능: macOS 사용자가 Gatekeeper 경고를 우회하고 xflowd 실행

  배경:
    주어진 macOS (Sonoma 이상) 환경이 있고
    그리고 `xflow-v0.1.0-darwin-arm64.tar.gz`를 다운로드하고 압축 해제했다

  시나리오: 최초 실행 시 Gatekeeper 경고
    주어진 압축 해제된 `xflowd` 바이너리가 `~/Downloads/xflow-verify/`에 있을 때
    만약 터미널에서 `./xflowd --config xflow.yaml`을 실행하면
    그러면 macOS가 다음 중 하나의 행동을 취해야 한다:
      - "xflowd의 개발자를 확인할 수 없음" 대화 상자 표시, 또는
      - "악성 소프트웨어 여부 확인 불가" 경고로 실행 차단

  시나리오: Control-click 방식 우회
    주어진 Finder에서 `xflowd`를 선택했을 때
    만약 Control-click(또는 우클릭) 후 "열기" 메뉴를 선택하면
    그러면 "개발자를 확인할 수 없음" 대화에서 "열기" 버튼이 활성화되어야 한다
    그리고 해당 버튼 클릭 후 바이너리가 실행되어야 한다
    그리고 이후 실행부터는 경고 없이 진행되어야 한다

  시나리오: xattr 명령으로 quarantine 속성 제거
    주어진 터미널에서 다음 명령을 실행할 때:
      """
      xattr -d com.apple.quarantine ./xflowd
      xattr -d com.apple.quarantine ./xflow
      xattr -d com.apple.quarantine ./xflow-agent
      """
    그러면 세 바이너리 모두 이후 실행 시 Gatekeeper 차단을 받지 않아야 한다

  시나리오: Gatekeeper 우회 후 xflowd 정상 동작
    주어진 Gatekeeper 우회가 완료된 상태에서
    만약 `./xflowd --config xflow.yaml` 명령으로 데몬을 시작하면
    그러면 xflowd가 정상적으로 시작되어야 한다
    그리고 설정된 HTTP 포트(예: 8080)에서 리스닝을 시작해야 한다
    그리고 `curl http://localhost:8080/` 응답으로 `web/dist/index.html`이 서빙되어야 한다
    그리고 `./xflowd --version` 실행 결과가 "v0.1.0"을 포함해야 한다

  시나리오: 릴리스 노트에 우회 절차 포함
    주어진 GitHub Release 페이지를 확인할 때
    그러면 릴리스 노트 또는 README 링크에 Gatekeeper 우회 절차가 명시되어야 한다
    그리고 절차에는 Control-click 방법, System Settings 방법, xattr 방법이 모두 포함되어야 한다
```

**검증 방법**:

- 실제 macOS 환경에서 다운로드 → Gatekeeper 경고 확인 → 각 우회 방법 순차 시도
- `xflowd --version` 출력 확인
- 브라우저 또는 curl로 `http://localhost:8080/` 접속하여 웹 자산 로딩 확인

---

## 추가 인수 시나리오 (선택)

### AC11: 캐시 적중 검증

```gherkin
기능: Go 모듈 및 npm 캐시 재사용으로 CI 속도 개선

  시나리오: 첫 CI 실행 후 두 번째 실행 가속
    주어진 동일한 `go.sum`과 `web/package-lock.json`으로 두 번째 PR을 생성할 때
    그러면 두 번째 CI 실행의 `setup-go` Step이 "Cache restored" 로그를 출력해야 한다
    그리고 `setup-node` Step도 동일하게 캐시 적중을 보고해야 한다
    그리고 두 번째 실행이 첫 번째 실행보다 최소 30% 이상 빨라야 한다
```

### AC12: 태그 재발행 시나리오 (edge case)

```gherkin
기능: 동일 태그 재푸시 시 릴리스 갱신 동작

  시나리오: 동일 태그 강제 재푸시
    주어진 `v0.1.0` 릴리스가 이미 발행되어 있을 때
    만약 `git tag -f v0.1.0` 후 `git push --force origin v0.1.0`을 실행하면
    그러면 릴리스 워크플로우가 재실행되어야 한다
    그리고 (softprops/action-gh-release 기본 동작) 기존 릴리스 에셋이 갱신되거나 오류로 중단되어야 한다
    그리고 (운영 권고) 태그 강제 재푸시는 지양하고 신규 patch 버전 사용을 권장한다
```

### AC13: 병렬 매트릭스 빌드 독립성

```gherkin
기능: 한 플랫폼 빌드 실패가 다른 플랫폼 실행에 영향을 주지 않음

  시나리오: 독립적 실행 보장
    주어진 `strategy.fail-fast: false`가 설정되어 있고
    그리고 6개 플랫폼 중 linux/arm v6 빌드가 실패할 때
    그러면 나머지 5개 플랫폼 빌드는 계속 진행되어 완료 상태에 도달해야 한다
    그리고 `package` Job은 `build-binaries`의 모든 매트릭스 완료를 기다리지만
         실패한 매트릭스로 인해 전체 파이프라인은 실패로 표시되어야 한다
    그리고 (운영 권고) 단일 플랫폼만 재시도 가능하도록 Job 재실행 기능 활용
```

---

## 인수 검증 체크리스트

다음은 전체 시스템이 운영 환경에 진입하기 전 검증해야 하는 최종 체크리스트이다.

### 기본 동작

- [ ] AC1: PR CI 자동 실행 및 품질 게이트 통과
- [ ] AC2: 태그 푸시 시 릴리스 7개 파일 발행
- [ ] AC3: 각 플랫폼 패키지 구조 검증 (최소 Linux amd64, macOS arm64, Linux arm64)
- [ ] AC4: checksums.txt 검증 OK
- [ ] AC5: Docker 멀티아키 이미지 amd64/arm64 양쪽 pull 성공

### 자동화 및 보안

- [ ] AC6: Dependabot PR 생성 (1주 대기 후 확인)
- [ ] AC7: govulncheck CI 차단 검증
- [ ] AC8: 테스트 실패 시 릴리스 차단

### 유연성

- [ ] AC9: workflow_dispatch 수동 프리릴리스 빌드
- [ ] AC10: macOS Gatekeeper 우회 후 xflowd 정상 실행

### 선택 항목

- [ ] AC11: 캐시 적중으로 CI 속도 개선
- [ ] AC12: 태그 재발행 시나리오 (주의)
- [ ] AC13: 매트릭스 빌드 독립성

### 완료 기준 (Definition of Done)

본 SPEC의 구현이 완료된 것으로 판정하려면 다음 조건을 모두 충족해야 한다:

1. AC1~AC10까지 필수 10개 인수 기준이 모두 통과
2. 최소 1회 이상 End-to-End 정식 릴리스 수행 (예: `v0.1.0`)
3. 6개 플랫폼 중 P0(macOS arm64, Linux amd64, Linux arm64) 3개에서 실제 바이너리 실행 확인
4. Docker 이미지가 amd64 및 arm64 호스트에서 pull + `--version` 실행 성공
5. Dependabot의 최초 자동 PR 1건 이상 확인
6. `docs/gatekeeper-bypass.md` 문서가 작성되어 리포지토리에 포함
7. 릴리스 노트에 macOS Gatekeeper 우회 절차 링크 또는 내용 포함

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-21*
*작성: MoAI SPEC Builder (manager-spec)*
