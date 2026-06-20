# 릴리스 이미지 서명 (관리 서버 릴리스 저장소)

관리 서버가 원격 노드에 배포하는 `xflowd` 프로그램 이미지는 **Ed25519 서명**으로
무결성·진위를 보장한다. 서명 **개인키**는 릴리스 담당자만 보관하고, **공개키**만
각 노드에 배포한다. 서버는 사전 서명된 `.sig`와 바이너리 바이트만 저장한다(개인키 없음).

## 1. 키쌍 생성 (최초 1회)

```sh
xflowd update keygen --out-dir ./secrets --name xflow-release
```

- `secrets/xflow-release.key` — **개인키(128-hex, 0600)**. 서명에만 사용. **절대 커밋 금지**
  (`.gitignore` 의 `*.key` 로 무시됨).
- `secrets/xflow-release.pub` — **공개키(64-hex)**. 노드에 배포.

## 2. 공개키를 노드에 배포

각 노드 설정 `update.public_key_path` 를 공개키 파일 경로로 지정한다(또는 hex 를 파일로 저장).

```yaml
# xflowd.yaml (노드)
update:
  public_key_path: "/etc/xflow/xflow-release.pub"
```

노드는 이 공개키로 다운로드한 바이너리의 `.sig` 를 검증한다. 검증 실패 시 적용을 거부한다.

## 3. 이미지 생성 (로컬)

```sh
make release-images VERSION=v1.3.0 SIGN_KEY=./secrets/xflow-release.key
```

`bin/images/` 에 6개 타깃이 생성된다: `xflowd-{os}-{arch}` + `xflowd-{os}-{arch}.sig`
+ `checksum.txt` (linux amd64/arm64/arm(v6), darwin amd64/arm64, windows amd64).

생성된 `xflowd-{os}-{arch}` 와 `.sig` 를 **Web UI 릴리스 저장소**에 업로드한다
(`/admin/remote/releases`). `checksum.txt` 는 서버가 자동 생성하므로 업로드 불필요.

## 4. CI 자동 발행 (GitHub Actions)

태그(`v*.*.*`) 푸시 시 `release-images` 잡이 이미지를 빌드·서명해 GitHub Release 에
첨부한다. 개인키는 리포지토리 **시크릿** 에서 주입한다.

- 시크릿 이름: `XFLOW_RELEASE_PRIVATE_KEY`
- 값: 개인키 **128-hex** 문자열 (`secrets/xflow-release.key` 내용)

시크릿 미설정 시(포크/PR) 해당 잡은 조용히 건너뛰며 메인 릴리스에는 영향이 없다.

## 보안 수칙

- 개인키(`*.key`)는 절대 커밋·공유하지 않는다. 유출 시 키쌍을 폐기하고 재생성·재배포한다.
- 공개키만 노드에 배포한다. 서버는 개인키를 보관하지 않는다.
- 노드 Downloader 는 https 를 강제하므로 관리 서버는 TLS 로 서빙되어야 한다.
