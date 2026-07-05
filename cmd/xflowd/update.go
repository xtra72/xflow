// @SPEC:SPEC-UPDATE-001 v0.1.0
// update.go — `xflowd update` Cobra 서브커맨드 트리.
//
// 명령 구조:
//
//	xflowd update                          # 도움말
//	  check                                # 새 버전 조회
//	  apply [--version vX.Y.Z] [--force]   # 적용
//	  status [--json]                      # 현재 상태
//	  rollback [--yes]                     # 백업 복원
//	  channel [stable|beta|nightly]        # 채널 조회/변경
//
// CLI 의 apply 는 ONE-SHOT (데몬 재시작은 데몬-사이드 API 의 책임).
//
// 디자인 결정:
//   - updateDeps 의존성 주입으로 테스트 격리
//   - 모든 설정 로딩은 loadUpdateSettings 한 곳으로 통합
//   - confirm() 헬퍼로 --yes 플래그와 stdin 입력을 통일
//   - channel 변경은 yaml 파일 in-place 편집 (기존 keys 보존)
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/updater"
)

// updateDeps 는 update 서브커맨드 의존성 셋이다.
//
// 모든 외부 효과 (HTTP, 디스크, 시간, stdin) 를 인터페이스로 추상화하여
// 테스트에서 가짜로 대체 가능하다.
type updateDeps struct {
	// NewChecker 는 updater.NewChecker 를 wrapping (테스트 시 fake checker 주입).
	NewChecker func(url string, channel updater.Channel) (*updater.Checker, error)

	// NewDownloader 는 updater.NewDownloader wrapper.
	NewDownloader func() *updater.Downloader

	// NewVerifier 는 updater.NewVerifier wrapper (공개키 주입).
	NewVerifier func(pubKey ed25519.PublicKey) (*updater.Verifier, error)

	// NewApplier 는 updater.NewApplier wrapper.
	NewApplier func(v *updater.Verifier, binaryPath string) *updater.Applier

	// NewRollback 은 updater.NewRollback wrapper.
	NewRollback func(binaryPath string) *updater.Rollback

	// LoadKey 는 updater.LoadPublicKeyFromFile wrapper. path 가 빈 문자열이면 빌트인 키 사용 시도.
	LoadKey func(path string) (ed25519.PublicKey, error)

	// BinaryPath 는 현재 실행 중인 바이너리 경로 (production: os.Executable).
	BinaryPath func() (string, error)

	// CurrentVersion 은 main.Version 의 값 반환 (production: getCurrentVersion).
	CurrentVersion func() string

	// Now 는 time.Now (테스트 결정론용).
	Now func() time.Time

	// Stdin 은 confirm() 의 stdin 소스 (production: os.Stdin).
	Stdin io.Reader
}

// defaultUpdateDeps 는 production 기본 의존성 셋을 반환한다.
func defaultUpdateDeps() updateDeps {
	return updateDeps{
		NewChecker:     updater.NewChecker,
		NewDownloader:  updater.NewDownloader,
		NewVerifier:    updater.NewVerifier,
		NewApplier:     updater.NewApplier,
		NewRollback:    updater.NewRollback,
		LoadKey:        updater.LoadPublicKeyFromFile,
		BinaryPath:     os.Executable,
		CurrentVersion: func() string { return Version },
		Now:            time.Now,
		Stdin:          os.Stdin,
	}
}

// newUpdateCmd 는 `update` 명령 트리를 구성한다.
func newUpdateCmd(deps updateDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "xflowd 자동 업데이트 관리",
		Long: `자동 업데이트 채널 / 버전 / 적용 / 롤백을 제어한다.

@SPEC:SPEC-UPDATE-001 v0.1.0

서브커맨드:
  check     - 새 버전이 있는지 확인 (네트워크 호출)
  apply     - 새 버전을 적용 (다운로드 + 검증 + 원자적 교체)
  status    - 현재 버전 / 채널 / 마지막 확인 시각 출력
  rollback  - 직전 버전으로 복원
  channel   - 업데이트 채널 조회 또는 변경 (stable | beta | nightly)`,
	}
	cmd.AddCommand(newUpdateCheckCmd(deps))
	cmd.AddCommand(newUpdateApplyCmd(deps))
	cmd.AddCommand(newUpdateStatusCmd(deps))
	cmd.AddCommand(newUpdateRollbackCmd(deps))
	cmd.AddCommand(newUpdateChannelCmd(deps))
	// @SPEC:SPEC-UPDATE-001 v0.1.0 — 릴리스 이미지 생성 도구 (운영자/CI 전용).
	cmd.AddCommand(newUpdateKeygenCmd(defaultImageDeps()))
	cmd.AddCommand(newUpdateSignCmd(defaultImageDeps()))
	return cmd
}

// --- check 서브커맨드 ---

func newUpdateCheckCmd(deps updateDeps) *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "새 버전이 있는지 확인합니다",
		Long:  "현재 버전과 채널의 최신 release 를 비교하여 업데이트 가능 여부를 보고한다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := loadUpdateSettings(configFile)
			if err != nil {
				return fmt.Errorf("설정 로딩 실패: %w", err)
			}
			ucfg, err := settings.ToUpdater()
			if err != nil {
				return fmt.Errorf("update 설정 검증 실패: %w", err)
			}

			checker, err := deps.NewChecker(ucfg.UpdateURL, ucfg.Channel)
			if err != nil {
				return fmt.Errorf("checker 생성 실패: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			current := updater.Version(deps.CurrentVersion())
			result, err := checker.Check(ctx, current, runtime.GOOS, runtime.GOARCH, "xflowd")
			if err != nil {
				return fmt.Errorf("버전 확인 실패: %w", err)
			}

			out := cmd.OutOrStdout()
			if result.Available {
				fmt.Fprintf(out, "현재: %s\n최신: %s\n채널: %s\n업데이트 가능: yes\n",
					current, result.Latest, ucfg.Channel)
				if result.ReleaseURL != "" {
					fmt.Fprintf(out, "릴리스 노트: %s\n", result.ReleaseURL)
				}
			} else {
				fmt.Fprintf(out, "현재: %s\n최신: %s\n채널: %s\n업데이트 가능: no (이미 최신)\n",
					current, result.Latest, ucfg.Channel)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "설정 파일 경로")
	return cmd
}

// --- apply 서브커맨드 ---

func newUpdateApplyCmd(deps updateDeps) *cobra.Command {
	var (
		configFile     string
		targetVersion  string
		forceDowngrade bool
		assumeYes      bool
		// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
		// CLI flag --auto-restart: REST API 의 auto_restart=true 와 동일.
		// 본 CLI 자체에서는 의미 제한적 (CLI 는 데몬이 아니므로 self-probe 무의미).
		// 주된 용도는 자동화 스크립트가 daemon-side API 를 호출하기 전 testing.
		autoRestart bool
		// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)
		// CLI flag --target: 업데이트 대상 바이너리 ("xflowd" / "xflow-agent" / "xflow").
		// default "xflowd" → v0.1.0 호환. CLI 자체 적용은 ONE-SHOT 이므로 본 flag 는 데몬-사이드 API 호출 모드에서 의미.
		targetBinary string
	)
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "새 버전을 적용합니다",
		Long: `업데이트를 다운로드 → 검증 → 원자적 교체 까지 수행한다.

CLI 의 apply 는 ONE-SHOT 작업이며, 데몬 재시작은 수행하지 않는다.
실행 중인 데몬에 적용하려면 데몬-사이드 API (POST /api/v1/system/update/apply) 를 사용해야 한다.

다운그레이드 (--version 이 현재보다 낮음) 는 --force 가 있어야 허용된다.

@SPEC:SPEC-UPDATE-002 v0.1.0 (M1):
--auto-restart flag 는 향후 데몬 자동 재시작을 위한 placeholder. 본 CLI 는 직접 데몬 재시작을
수행하지 않으므로 이 flag 는 향후 SPEC-UPDATE-002 의 데몬-사이드 API 호출 모드에서만 의미를 갖는다.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := loadUpdateSettings(configFile)
			if err != nil {
				return fmt.Errorf("설정 로딩 실패: %w", err)
			}
			ucfg, err := settings.ToUpdater()
			if err != nil {
				return fmt.Errorf("update 설정 검증 실패: %w", err)
			}

			checker, err := deps.NewChecker(ucfg.UpdateURL, ucfg.Channel)
			if err != nil {
				return fmt.Errorf("checker 생성 실패: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()

			current := updater.Version(deps.CurrentVersion())
			result, err := checker.Check(ctx, current, runtime.GOOS, runtime.GOARCH, "xflowd")
			if err != nil {
				return fmt.Errorf("버전 확인 실패: %w", err)
			}

			// --version 으로 명시한 경우 latest 를 override
			latest := result.Latest
			if targetVersion != "" {
				latest = updater.Version(targetVersion)
				if !latest.IsValid() {
					return fmt.Errorf("--version 형식 오류: %q (예: v0.4.0)", targetVersion)
				}
			}

			out := cmd.OutOrStdout()

			// 다운그레이드 게이트
			cmpResult := latest.Compare(current)
			if cmpResult < 0 && !forceDowngrade {
				return fmt.Errorf("downgrade %s → %s requires --force flag: %w",
					current, latest, updater.ErrDowngradeRequiresForce)
			}
			if cmpResult == 0 && targetVersion == "" {
				fmt.Fprintln(out, "already up to date")
				return nil
			}

			// 사용자 확인
			ok, err := confirm(deps.Stdin, out,
				fmt.Sprintf("%s → %s 적용하시겠습니까? [y/N]: ", current, latest),
				assumeYes)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "취소되었습니다.")
				return nil
			}

			// 본격 적용은 별도 함수 (테스트에서 일부만 검증 가능하게 분리)
			return runApply(ctx, deps, ucfg, settings, result, latest, out)
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "설정 파일 경로")
	cmd.Flags().StringVar(&targetVersion, "version", "", "특정 버전 지정 (vX.Y.Z 형식; 비워두면 최신)")
	cmd.Flags().BoolVar(&forceDowngrade, "force", false, "다운그레이드 허용 (위험)")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "확인 프롬프트 자동 승인")
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
	// --auto-restart: 데몬-사이드 API 호출 모드에서 의미 (Phase B+ 에서 활용 예정).
	// CLI 자체 적용은 ONE-SHOT 이므로 본 flag 는 현재 silent (placeholder).
	cmd.Flags().BoolVar(&autoRestart, "auto-restart", false, "v0.2.0: 적용 후 자동 재시작 (graceful drain + exec + health check + auto rollback). 데몬-사이드 API 호출 모드에서만 의미.")
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)
	// --target: 업데이트 대상 바이너리 (xflowd | xflow-agent | xflow). default "xflowd" → v0.1.0 호환.
	// CLI 자체 적용은 ONE-SHOT 이므로 본 flag 는 데몬-사이드 API 호출 모드에서만 의미를 가진다.
	cmd.Flags().StringVar(&targetBinary, "target", "xflowd",
		"v0.2.0: 업데이트 대상 바이너리 (xflowd | xflow-agent | xflow). 데몬-사이드 API 호출 모드에서만 의미.")
	// 사용은 추후 (현재는 컴파일 가드용 silence). 데몬-사이드 호출 미구현이므로 현재 silent.
	_ = autoRestart
	_ = targetBinary
	return cmd
}

// runApply 는 apply 의 다운로드 → 검증 → 교체 단계를 수행한다.
//
// 테스트에서는 BinaryPath / NewDownloader 등의 deps 를 fake 로 대체하면
// 이 함수도 통과하지만, 실제 atomic rename 자체는 dummy file 에서 시도된다.
func runApply(
	ctx context.Context,
	deps updateDeps,
	ucfg updater.UpdateConfig,
	settings config.UpdateSettings,
	result updater.CheckResult,
	target updater.Version,
	out io.Writer,
) error {
	if result.BinaryAsset == nil {
		return errors.New("apply: 플랫폼에 맞는 binary asset 없음")
	}

	// 공개키 로드
	if settings.PublicKeyPath == "" {
		return errors.New("apply: update.public_key_path 가 설정되지 않음 (Ed25519 검증에 필수)")
	}
	pubKey, err := deps.LoadKey(settings.PublicKeyPath)
	if err != nil {
		return fmt.Errorf("공개키 로딩 실패: %w", err)
	}

	// 다운로드 디렉토리
	binPath, err := deps.BinaryPath()
	if err != nil {
		return fmt.Errorf("실행 파일 경로 조회 실패: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "xflowd-update-*")
	if err != nil {
		return fmt.Errorf("임시 디렉토리 생성 실패: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binDest := filepath.Join(tmpDir, result.BinaryAsset.Name)

	dl := deps.NewDownloader()
	fmt.Fprintf(out, "다운로드 시작: %s\n", result.BinaryAsset.DownloadURL)
	if err := dl.Download(ctx, *result.BinaryAsset, binDest, nil); err != nil {
		return fmt.Errorf("다운로드 실패: %w", err)
	}

	// signature / checksum asset 가져오기
	if result.SignatureAsset == nil || result.ChecksumAsset == nil {
		return errors.New("apply: signature/checksum asset 누락 (release 에 .sig 또는 checksum.txt 가 없음)")
	}
	sigDest := filepath.Join(tmpDir, result.SignatureAsset.Name)
	if err := dl.Download(ctx, *result.SignatureAsset, sigDest, nil); err != nil {
		return fmt.Errorf("서명 다운로드 실패: %w", err)
	}
	checksumDest := filepath.Join(tmpDir, result.ChecksumAsset.Name)
	if err := dl.Download(ctx, *result.ChecksumAsset, checksumDest, nil); err != nil {
		return fmt.Errorf("체크섬 다운로드 실패: %w", err)
	}

	// SHA256 hex 추출
	checksumLine, err := readChecksumFor(checksumDest, result.BinaryAsset.Name)
	if err != nil {
		return fmt.Errorf("체크섬 파싱 실패: %w", err)
	}

	// 서명 raw bytes
	sigBytes, err := os.ReadFile(sigDest)
	if err != nil {
		return fmt.Errorf("서명 파일 읽기 실패: %w", err)
	}
	sigBytes = decodeSignature(sigBytes)

	manifest := updater.Manifest{
		Version:   target,
		SHA256:    checksumLine,
		Signature: sigBytes,
		BinaryURL: result.BinaryAsset.DownloadURL,
	}

	verifier, err := deps.NewVerifier(pubKey)
	if err != nil {
		return fmt.Errorf("verifier 생성 실패: %w", err)
	}
	applier := deps.NewApplier(verifier, binPath)

	fmt.Fprintf(out, "검증 + 원자적 교체 중: %s\n", binPath)
	applyRes, err := applier.Apply(ctx, updater.ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: binDest,
	})
	if err != nil {
		return fmt.Errorf("적용 실패: %w", err)
	}

	fmt.Fprintf(out, "적용 완료: %s → %s (백업: %s)\n",
		deps.CurrentVersion(), applyRes.NewVersion, applyRes.BackupPath)
	fmt.Fprintln(out, "주의: 데몬 재시작은 별도로 수행해야 적용이 반영됩니다.")
	_ = settings // settings 는 디버그용 보존
	return nil
}

// readChecksumFor 는 checksum.txt 파일에서 특정 asset 의 SHA256 hex 를 추출한다.
//
// 형식: "<sha256>  <filename>" 한 줄당 한 항목 (sha256sum 표준).
func readChecksumFor(path, assetName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// "<hex>  <filename>" 또는 "<hex> *<filename>"
		fname := strings.TrimPrefix(fields[len(fields)-1], "*")
		if fname == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %q not found", assetName)
}

// decodeSignature 는 .sig 파일이 hex/base64 형식이면 raw bytes 로 변환한다.
//
// 64-byte raw 면 그대로 반환 (ed25519 서명 길이 = 64).
// 128-char hex 면 디코딩.
// 그 외 (예: PEM) 는 호출자가 추가 처리.
func decodeSignature(b []byte) []byte {
	trimmed := strings.TrimSpace(string(b))
	if len(trimmed) == ed25519.SignatureSize*2 {
		if raw, err := hex.DecodeString(trimmed); err == nil {
			return raw
		}
	}
	return b
}

// --- status 서브커맨드 ---

func newUpdateStatusCmd(deps updateDeps) *cobra.Command {
	var (
		configFile string
		jsonOut    bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "현재 업데이트 상태를 출력합니다",
		Long:  "현재 버전, 채널, 자동 업데이트 활성 여부, 백업 존재 여부를 출력한다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := loadUpdateSettings(configFile)
			if err != nil {
				return fmt.Errorf("설정 로딩 실패: %w", err)
			}

			binPath, _ := deps.BinaryPath()
			rb := deps.NewRollback(binPath)

			payload := map[string]any{
				"current_version":  deps.CurrentVersion(),
				"channel":          settings.Channel,
				"enabled":          settings.Enabled,
				"auto_apply":       settings.AutoApply,
				"update_url":       settings.UpdateURL,
				"backup_available": rb.CanRollback(),
				"check_interval":   settings.CheckInterval.String(),
				"timestamp":        deps.Now().UTC().Format(time.RFC3339),
			}

			out := cmd.OutOrStdout()
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(out, "현재 버전:    %s\n", payload["current_version"])
			fmt.Fprintf(out, "채널:         %s\n", payload["channel"])
			fmt.Fprintf(out, "활성화:       %v\n", payload["enabled"])
			fmt.Fprintf(out, "자동 적용:    %v\n", payload["auto_apply"])
			fmt.Fprintf(out, "Update URL:   %s\n", payload["update_url"])
			fmt.Fprintf(out, "백업 존재:    %v\n", payload["backup_available"])
			fmt.Fprintf(out, "체크 주기:    %s\n", payload["check_interval"])
			return nil
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "설정 파일 경로")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON 형식으로 출력")
	return cmd
}

// --- rollback 서브커맨드 ---

func newUpdateRollbackCmd(deps updateDeps) *cobra.Command {
	var (
		configFile string
		assumeYes  bool
	)
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "직전 버전으로 복원합니다",
		Long: `백업된 직전 바이너리 (<binary>.previous) 를 메인 위치로 복원한다.

복원 후 백업 파일은 삭제된다 (재롤백 방지).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = configFile // status 와 시그니처 일관성을 위해 유지

			binPath, err := deps.BinaryPath()
			if err != nil {
				return fmt.Errorf("실행 파일 경로 조회 실패: %w", err)
			}

			rb := deps.NewRollback(binPath)
			if !rb.CanRollback() {
				return fmt.Errorf("rollback: no backup available at %s.previous", binPath)
			}

			info, err := rb.BackupInfo()
			if err != nil {
				return fmt.Errorf("백업 정보 조회 실패: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "백업 발견: %s (%d bytes, %s)\n",
				info.Path, info.Size, info.ModTime.Format(time.RFC3339))

			ok, err := confirm(deps.Stdin, out, "롤백하시겠습니까? [y/N]: ", assumeYes)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "취소되었습니다.")
				return nil
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			res, err := rb.Restore(ctx)
			if err != nil {
				return fmt.Errorf("롤백 실패: %w", err)
			}

			fmt.Fprintf(out, "복원 완료: %s → %s (at %s)\n",
				res.RestoredFrom, res.RestoredTo, res.RestoredAt.Format(time.RFC3339))
			fmt.Fprintln(out, "주의: 데몬 재시작이 필요합니다.")
			return nil
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "설정 파일 경로 (현재 미사용; 일관성용)")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "확인 프롬프트 자동 승인")
	return cmd
}

// --- channel 서브커맨드 ---

func newUpdateChannelCmd(_ updateDeps) *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "channel [stable|beta|nightly]",
		Short: "업데이트 채널을 조회 또는 변경합니다",
		Long: `인자 없이 호출하면 현재 채널을 출력한다.
인자가 주어지면 설정 파일의 update.channel 값을 변경한다.

변경 가능 값: stable, beta, nightly`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := loadUpdateSettings(configFile)
			if err != nil {
				return fmt.Errorf("설정 로딩 실패: %w", err)
			}

			out := cmd.OutOrStdout()

			if len(args) == 0 {
				fmt.Fprintln(out, settings.Channel)
				return nil
			}

			newCh := strings.ToLower(args[0])
			ch := updater.Channel(newCh)
			if !ch.IsValid() {
				return fmt.Errorf("invalid channel %q (allowed: stable, beta, nightly)", newCh)
			}

			path := configFile
			if path == "" {
				home, herr := os.UserHomeDir()
				if herr != nil {
					return fmt.Errorf("홈 디렉토리 조회 실패: %w", herr)
				}
				path = filepath.Join(home, ".xflow", "config.yaml")
			}

			if err := writeChannelToConfig(path, newCh); err != nil {
				return fmt.Errorf("설정 파일 업데이트 실패: %w", err)
			}

			fmt.Fprintf(out, "채널 변경: %s → %s (%s)\n", settings.Channel, newCh, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "설정 파일 경로")
	return cmd
}

// writeChannelToConfig 는 yaml 파일의 update.channel 키만 변경하고 나머지는 보존한다.
//
// 파일이 없거나 update 섹션이 없으면 새로 추가한다.
func writeChannelToConfig(path, newChannel string) error {
	// 1) 기존 파일 읽기 (없으면 빈 map 으로 시작)
	var root map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("yaml 파싱 실패: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("기존 설정 파일 읽기 실패: %w", err)
	}
	if root == nil {
		root = map[string]any{}
	}

	// 2) update 섹션 추출/생성
	updRaw, ok := root["update"].(map[string]any)
	if !ok {
		updRaw = map[string]any{}
	}
	updRaw["channel"] = newChannel
	root["update"] = updRaw

	// 3) 디렉토리 생성 후 atomic write
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("디렉토리 생성 실패: %w", err)
		}
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("yaml 직렬화 실패: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("임시 파일 쓰기 실패: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename 실패: %w", err)
	}
	return nil
}

// --- 공통 헬퍼 ---

// loadUpdateSettings 는 config 패키지를 통해 update 섹션을 로딩한다.
func loadUpdateSettings(configFile string) (config.UpdateSettings, error) {
	var opts []config.LoadOption
	if configFile != "" {
		opts = append(opts, config.WithConfigFile(configFile))
	}
	cfg, err := config.Load(opts...)
	if err != nil {
		return config.UpdateSettings{}, err
	}
	return cfg.Update(), nil
}

// confirm 은 사용자 확인 프롬프트를 출력하고 응답을 읽는다.
//
// assumeYes 가 true 면 stdin 을 읽지 않고 즉시 true 반환.
// 응답이 "y"/"yes" 인 경우만 true (대소문자 무시), 그 외는 false.
func confirm(stdin io.Reader, out io.Writer, prompt string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if stdin == nil {
		return false, nil
	}
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return false, err
	}
	r := bufio.NewReader(stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read stdin: %w", err)
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}
