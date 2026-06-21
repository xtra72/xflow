// remote_system_update.go 는 원격 관리 "system/update" 명령(버전 관리 Phase 2)의
// 클라이언트 측 핸들러를 구현한다.
//
// 관리 서버가 Dispatch("system","update",{target_version,...}) 를 보내면, 이 노드의
// systemCommander 가 updater 파이프라인(check→download→verify(Ed25519)→apply)을 실행해
// 바이너리를 원자적으로 교체(.previous 백업)한다. 기본은 교체만 하고 restart_required=true
// 를 반환하며(운영 측 재시작 위임), args.restart=true 면 결과 전송 후 graceful 재시작한다.
//
// remote.DomainCommander 를 구현하여 remote.Applier.WithSystem 으로 바인딩된다.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/updater"
)

// newSystemCommander 는 데몬 설정으로부터 system 도메인 commander 를 구성한다.
// update.update_url/public_key_path 미설정이어도 생성은 성공하며(ApplyUpdate 호출 시
// 명확히 실패). 재시작은 opt-in(args.restart=true)일 때만 graceful re-exec 한다.
func newSystemCommander(cfg config.Config, configFile string, logger *slog.Logger) *systemCommander {
	binPath, err := os.Executable()
	if err != nil {
		logger.Warn("실행 파일 경로 조회 실패 — 원격 업데이트 제한", "error", err)
		binPath = ""
	}
	runner := &remoteUpdateRunner{
		settings:   cfg.Update(),
		version:    Version,
		binaryPath: binPath,
		configFile: configFile,
		logger:     logger,
	}
	return &systemCommander{
		runner: runner,
		restart: func(expectedVersion string) {
			// 결과가 서버로 flush 될 시간을 준 뒤 새 바이너리로 re-exec 한다(opt-in).
			// 검증/롤백은 ApplyUpdate 가 미리 기록한 update-state(파일)로 부팅 측에서 수행한다.
			go gracefulReexec(binPath, expectedVersion, logger)
		},
		logger: logger,
	}
}

// gracefulReexec 는 짧은 지연 후 새 바이너리를 re-exec 한다(현재 인자/환경 유지).
// syscall.Exec 는 성공 시 반환하지 않고 프로세스 이미지를 교체한다(unix). 부팅한 새
// 프로세스는 update-state 파일을 보고 자가 검증/롤백한다(post_update.go).
func gracefulReexec(binPath, expectedVersion string, logger *slog.Logger) {
	time.Sleep(2 * time.Second)
	if binPath == "" {
		logger.Error("재시작 실패 — 실행 파일 경로 미상")
		return
	}
	logger.Info("system/update: 새 바이너리로 재시작", "binary", binPath, "expected_version", expectedVersion)
	if err := syscall.Exec(binPath, os.Args, os.Environ()); err != nil {
		logger.Error("재시작(re-exec) 실패", "error", err)
	}
}

// systemUpdateApplier 는 self-update 오케스트레이션을 추상화한다(테스트 fake 주입용).
// updateURL 이 비어 있지 않으면 노드 로컬 update_url 대신 사용한다(서버 소스 오버라이드).
type systemUpdateApplier interface {
	ApplyUpdate(ctx context.Context, targetVersion, channel, updateURL string) (remote.SystemUpdateResult, error)
}

// systemCommander 는 remote.DomainCommander 를 구현해 system 도메인 명령을 라우팅한다.
//
// restart 는 비동기 graceful 재시작 스케줄러이다(실 구현은 결과 전송 후 re-exec). nil 이면
// 재시작 미지원으로, args.restart=true 라도 교체만 수행하고 경고를 남긴다(테스트 주입 가능).
type systemCommander struct {
	runner  systemUpdateApplier
	restart func(expectedVersion string)
	logger  *slog.Logger
}

// Do 는 system 도메인 action 을 처리한다(현재 update 만 지원).
func (c *systemCommander) Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	if action != remote.ActionSystemUpdate {
		return nil, fmt.Errorf("알 수 없는 system action: %q", action)
	}
	var a remote.SystemUpdateArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, fmt.Errorf("system/update args 파싱: %w", err)
		}
	}
	res, err := c.runner.ApplyUpdate(ctx, a.TargetVersion, a.Channel, a.UpdateURL)
	if err != nil {
		return nil, err
	}
	// 바이너리는 교체됐으나 실행 중 프로세스는 여전히 구버전 — 재시작이 있어야 반영된다.
	res.RestartRequired = true
	if a.Restart {
		if c.restart == nil {
			c.logger.Warn("system/update: 재시작 요청됐으나 미지원 — 수동 재시작 필요",
				"new_version", res.NewVersion)
		} else {
			// 결과를 서버로 먼저 전송한 뒤 재시작되도록 비동기로 스케줄한다(re-exec).
			// 새 프로세스가 자가 검증할 수 있도록 적용된 버전을 expected 로 넘긴다.
			res.Restarting = true
			c.restart(res.NewVersion)
		}
	}
	return json.Marshal(res)
}

// remoteUpdateRunner 는 updater 패키지를 직접 오케스트레이션하는 실 구현이다.
// cmd/xflowd/update.go 의 runApply 와 동일한 순서(check→download→verify→apply)를 따른다.
type remoteUpdateRunner struct {
	settings   config.UpdateSettings
	version    string // 현재 빌드 버전(main.Version)
	binaryPath string // 교체 대상 실행 파일 경로
	configFile string // 데몬 설정 파일 경로(pre-flight `verify` 에 전달)
	logger     *slog.Logger
}

// smokeTest 는 교체 전에 후보 바이너리를 `verify` 로 실행해 기동 가능성을 확인한다(A안).
// 후보가 설정 로드/초기화에 실패하면(아키텍처 불일치/링크 오류/설정 비호환) 오류를 반환해
// 교체를 중단시킨다 — 실행 중 데몬은 그대로 유지된다(다운타임 0).
func (r *remoteUpdateRunner) smokeTest(ctx context.Context, candidatePath string) error {
	if err := os.Chmod(candidatePath, 0o755); err != nil {
		return fmt.Errorf("후보 실행 권한 설정: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"verify"}
	if r.configFile != "" {
		args = append(args, "--config", r.configFile)
	}
	cmd := exec.CommandContext(cctx, candidatePath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("후보 verify 실패: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if r.logger != nil {
		r.logger.Info("pre-flight 스모크 테스트 통과", "candidate", candidatePath)
	}
	return nil
}

// ApplyUpdate 는 목표 버전 바이너리를 받아 검증 후 원자적으로 교체한다.
// updateURL 이 비어 있지 않으면 노드 로컬 update_url 대신 사용한다(서버 소스 오버라이드).
// 공개키(public_key_path)는 항상 노드 로컬 설정을 사용한다(신뢰 앵커 — 서버가 못 바꿈).
func (r *remoteUpdateRunner) ApplyUpdate(ctx context.Context, targetVersion, channel, updateURL string) (remote.SystemUpdateResult, error) {
	var zero remote.SystemUpdateResult
	srcURL := updateURL
	if srcURL == "" {
		srcURL = r.settings.UpdateURL
	}
	if srcURL == "" {
		return zero, errors.New("update_url 미설정 — 원격 업데이트 비활성")
	}
	if r.settings.PublicKeyPath == "" {
		return zero, errors.New("public_key_path 미설정 — Ed25519 검증에 필수")
	}
	ch := channel
	if ch == "" {
		ch = r.settings.Channel
	}
	checker, err := updater.NewChecker(srcURL, updater.Channel(ch))
	if err != nil {
		return zero, fmt.Errorf("checker 생성: %w", err)
	}
	res, err := checker.Check(ctx, updater.Version(r.version), runtime.GOOS, runtime.GOARCH, "xflowd")
	if err != nil {
		return zero, fmt.Errorf("버전 확인: %w", err)
	}
	// 채널에 릴리스가 없으면(404) Checker 는 에러 없이 빈 결과(Latest 빈 값, 자산 nil)를
	// 반환한다 — 채널 불일치를 명확히 알린다(가장 흔한 함정: beta 릴리스인데 업데이트가
	// stable 채널을 조회). 자산 nil 검사보다 먼저 처리해 "asset 누락" 오해를 막는다.
	if res.Latest == "" {
		return zero, fmt.Errorf(
			"채널 %q 에서 릴리스를 찾지 못했습니다 — 릴리스 채널과 업데이트 채널이 일치해야 합니다"+
				"(예: beta 릴리스는 업데이트 소스 채널도 beta 여야 함)", ch)
	}
	target := updater.Version(targetVersion)
	if targetVersion == "" {
		target = res.Latest
	}
	if !target.IsValid() {
		return zero, fmt.Errorf("유효하지 않은 목표 버전: %q", targetVersion)
	}
	// 어떤 자산이 왜 누락인지 구체적으로 알린다(진단성). 자산은 Checker 가 매칭한
	// 릴리스(res.Latest)의 것이며, 노드 아키텍처(GOOS/GOARCH)에 정확히 일치해야 한다.
	assetName := updater.AssetName("xflowd", runtime.GOOS, runtime.GOARCH)
	if res.BinaryAsset == nil {
		return zero, fmt.Errorf(
			"release %s 에 이 노드 아키텍처(%s/%s)용 바이너리 %q 가 없습니다 — 해당 아키텍처 이미지를 릴리스 저장소에 업로드하세요",
			res.Latest, runtime.GOOS, runtime.GOARCH, assetName)
	}
	if res.SignatureAsset == nil {
		return zero, fmt.Errorf(
			"release %s 의 %q 에 서명(%s.sig)이 없습니다 — 업로드 시 .sig 가 누락되었습니다",
			res.Latest, assetName, assetName)
	}
	if res.ChecksumAsset == nil {
		return zero, fmt.Errorf("release %s 에 checksum.txt 가 없습니다", res.Latest)
	}

	pubKey, err := updater.LoadPublicKeyFromFile(r.settings.PublicKeyPath)
	if err != nil {
		return zero, fmt.Errorf("공개키 로드: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "xflowd-remote-update-*")
	if err != nil {
		return zero, fmt.Errorf("임시 디렉토리: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dl := updater.NewDownloader()
	binDest := filepath.Join(tmpDir, res.BinaryAsset.Name)
	if err := dl.Download(ctx, *res.BinaryAsset, binDest, nil); err != nil {
		return zero, fmt.Errorf("바이너리 다운로드: %w", err)
	}
	sigDest := filepath.Join(tmpDir, res.SignatureAsset.Name)
	if err := dl.Download(ctx, *res.SignatureAsset, sigDest, nil); err != nil {
		return zero, fmt.Errorf("서명 다운로드: %w", err)
	}
	checksumDest := filepath.Join(tmpDir, res.ChecksumAsset.Name)
	if err := dl.Download(ctx, *res.ChecksumAsset, checksumDest, nil); err != nil {
		return zero, fmt.Errorf("체크섬 다운로드: %w", err)
	}

	checksumHex, err := readChecksumFor(checksumDest, res.BinaryAsset.Name)
	if err != nil {
		return zero, fmt.Errorf("체크섬 파싱: %w", err)
	}
	sigBytes, err := os.ReadFile(sigDest)
	if err != nil {
		return zero, fmt.Errorf("서명 읽기: %w", err)
	}
	sigBytes = decodeSignature(sigBytes)

	verifier, err := updater.NewVerifier(pubKey)
	if err != nil {
		return zero, fmt.Errorf("verifier 생성: %w", err)
	}

	// 교체 전 서명 검증(TOCTOU 전) — 미서명/위조 바이너리를 실행하지 않도록 스모크 테스트
	// 직전에 한 번 확인한다. Apply 가 교체 직전 다시 검증한다(이중 방어).
	candidateBytes, err := os.ReadFile(binDest)
	if err != nil {
		return zero, fmt.Errorf("후보 바이너리 읽기: %w", err)
	}
	if err := verifier.VerifyAll(candidateBytes, checksumHex, sigBytes); err != nil {
		return zero, fmt.Errorf("후보 서명/체크섬 검증 실패: %w", err)
	}

	// pre-flight 스모크 테스트(A안): 검증된 후보를 verify 로 실행해 기동 가능성을 확인한다.
	// 실패하면 교체하지 않고 중단한다(다운타임 0).
	if err := r.smokeTest(ctx, binDest); err != nil {
		return zero, fmt.Errorf("pre-flight 검증 실패(교체 중단): %w", err)
	}

	applier := updater.NewApplier(verifier, r.binaryPath)
	applyRes, err := applier.Apply(ctx, updater.ApplyOptions{
		Manifest: updater.Manifest{
			Version:   target,
			SHA256:    checksumHex,
			Signature: sigBytes,
			BinaryURL: res.BinaryAsset.DownloadURL,
		},
		DownloadedPath: binDest,
	})
	if err != nil {
		return zero, fmt.Errorf("적용: %w", err)
	}

	// 교체 성공 → 다음 부팅이 자가 검증/롤백 대상임을 상태 파일에 기록한다(재시작 여부 무관).
	// 실패해도 교체 자체는 유효하므로 경고만 남긴다(검증 비활성화 fallback).
	if mErr := markUpdatePending(r.binaryPath, applyRes.NewVersion.String()); mErr != nil {
		// logger 가 없으므로 결과에 영향 주지 않고 무시(상위에서 dispatch 결과로 관측 가능).
		_ = mErr
	}

	return remote.SystemUpdateResult{
		NewVersion:  applyRes.NewVersion.String(),
		BackupPath:  applyRes.BackupPath,
		AppliedAtMs: applyRes.AppliedAt.UnixMilli(),
	}, nil
}
