// @SPEC:SPEC-UPDATE-001 v0.1.0
// update_image.go — `xflowd update keygen` / `xflowd update sign` 서브커맨드.
//
// 릴리스 이미지 생성 도구 (운영자/CI 전용):
//
//	xflowd update keygen [--out-dir DIR] [--name BASENAME] [--force]
//	  → Ed25519 키쌍 생성. {dir}/{name}.key (0600, hex) + {dir}/{name}.pub (hex).
//
//	xflowd update sign --key KEYFILE [--out FILE] BINARY
//	  → BINARY 바이트를 읽어 Ed25519 서명 → {BINARY}.sig (raw 64 bytes).
//
// 디자인 결정:
//   - update.go 의 updateDeps 패턴과 일관성을 위해 imageDeps 의존성 주입 사용
//   - sign 은 바이트만 읽으므로 cross-compile 된 임의 arch 바이너리도 호스트에서 서명 가능
//   - .sig 는 raw 64-byte 형식 (decodeSignature 가 그대로 수용; 최소 크기)
//   - --key 는 파일 경로가 우선이지만, 존재하지 않는 경로면 hex 문자열로 fallback (편의)
package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/updater"
)

// imageDeps 는 keygen/sign 서브커맨드의 의존성 셋이다 (테스트 격리용).
type imageDeps struct {
	// GenerateKeyPair 는 updater.GenerateKeyPair wrapper.
	GenerateKeyPair func() (ed25519.PrivateKey, ed25519.PublicKey, error)

	// LoadPrivateKey 는 updater.LoadPrivateKeyFromFile wrapper.
	LoadPrivateKey func(path string) (ed25519.PrivateKey, error)
}

// defaultImageDeps 는 production 기본 의존성 셋을 반환한다.
func defaultImageDeps() imageDeps {
	return imageDeps{
		GenerateKeyPair: updater.GenerateKeyPair,
		LoadPrivateKey:  updater.LoadPrivateKeyFromFile,
	}
}

// --- keygen 서브커맨드 ---

func newUpdateKeygenCmd(deps imageDeps) *cobra.Command {
	var (
		outDir   string
		baseName string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "릴리스 서명용 Ed25519 키쌍을 생성합니다",
		Long: `릴리스 이미지 서명에 사용할 Ed25519 키쌍을 생성한다.

생성물:
  {out-dir}/{name}.key   개인키 (hex, 권한 0600) — 절대 커밋 금지
  {out-dir}/{name}.pub   공개키 (hex)            — 노드에 배포

노드는 update.public_key_path 를 .pub 파일로 설정하거나, 출력된 hex 를 직접 붙여넣는다.

@SPEC:SPEC-UPDATE-001 v0.1.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			priv, pub, err := deps.GenerateKeyPair()
			if err != nil {
				return fmt.Errorf("키쌍 생성 실패: %w", err)
			}

			if outDir == "" {
				outDir = "."
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("출력 디렉토리 생성 실패: %w", err)
			}

			keyPath := filepath.Join(outDir, baseName+".key")
			pubPath := filepath.Join(outDir, baseName+".pub")

			// 덮어쓰기 방지 (--force 없으면 거부).
			if !force {
				if err := refuseExisting(keyPath); err != nil {
					return err
				}
				if err := refuseExisting(pubPath); err != nil {
					return err
				}
			}

			// 개인키: 0600 (소유자 read/write 만).
			if err := os.WriteFile(keyPath, []byte(updater.PrivateKeyToHex(priv)+"\n"), 0o600); err != nil {
				return fmt.Errorf("개인키 쓰기 실패: %w", err)
			}
			// 공개키: 0644 (배포 대상).
			if err := os.WriteFile(pubPath, []byte(updater.PublicKeyToHex(pub)+"\n"), 0o644); err != nil { //nolint:gosec // 공개키는 비밀이 아님
				return fmt.Errorf("공개키 쓰기 실패: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "키쌍 생성 완료:\n")
			fmt.Fprintf(out, "  개인키: %s (권한 0600)\n", keyPath)
			fmt.Fprintf(out, "  공개키: %s\n", pubPath)
			fmt.Fprintf(out, "\n공개키 (hex):\n%s\n", updater.PublicKeyToHex(pub))
			fmt.Fprintf(out, "\n노드 설정: update.public_key_path 를 %q 로 지정하거나 위 hex 를 붙여넣으세요.\n", pubPath)
			fmt.Fprintf(out, "보안 경고: %s 는 비밀입니다. 절대 커밋하거나 공유하지 마세요.\n", keyPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out-dir", ".", "키 파일 출력 디렉토리")
	cmd.Flags().StringVar(&baseName, "name", "xflow-release", "키 파일 베이스 이름 ({name}.key / {name}.pub)")
	cmd.Flags().BoolVar(&force, "force", false, "기존 파일 덮어쓰기 허용")
	return cmd
}

// refuseExisting 은 경로가 이미 존재하면 에러를 반환한다 (--force 미지정 시).
func refuseExisting(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("파일이 이미 존재합니다: %s (덮어쓰려면 --force)", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("파일 상태 확인 실패 %q: %w", path, err)
	}
	return nil
}

// --- sign 서브커맨드 ---

func newUpdateSignCmd(deps imageDeps) *cobra.Command {
	var (
		keyArg  string
		outPath string
	)
	cmd := &cobra.Command{
		Use:   "sign --key KEYFILE [--out FILE] BINARY",
		Short: "바이너리를 Ed25519 개인키로 서명합니다",
		Long: `바이너리 바이트를 읽어 Ed25519 서명을 생성한다.

서명은 BINARY 의 raw content 전체에 대해 수행되며 (checksum 이 아님),
출력은 raw 64-byte 서명 파일이다 (기본 {BINARY}.sig).

바이트만 읽으므로 cross-compile 된 임의 arch 의 바이너리도 호스트에서 서명할 수 있다.

--key 는 키 파일 경로 (hex 또는 PEM) 가 표준 형식이다.
경로가 존재하지 않으면 hex 문자열로 해석을 시도한다 (편의).

@SPEC:SPEC-UPDATE-001 v0.1.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			binaryPath := args[0]
			if keyArg == "" {
				return fmt.Errorf("--key 가 필요합니다 (키 파일 경로 또는 hex)")
			}

			priv, err := loadSigningKey(deps, keyArg)
			if err != nil {
				return fmt.Errorf("개인키 로딩 실패: %w", err)
			}

			content, err := os.ReadFile(binaryPath) //nolint:gosec // 운영자가 지정한 빌드 산출물
			if err != nil {
				return fmt.Errorf("바이너리 읽기 실패 %q: %w", binaryPath, err)
			}

			sig, err := updater.Sign(priv, content)
			if err != nil {
				return fmt.Errorf("서명 실패: %w", err)
			}

			dest := outPath
			if dest == "" {
				dest = binaryPath + ".sig"
			}
			if err := os.WriteFile(dest, sig, 0o644); err != nil { //nolint:gosec // 서명은 비밀이 아님
				return fmt.Errorf("서명 파일 쓰기 실패: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "서명 완료: %s → %s (%d bytes)\n", binaryPath, dest, len(sig))
			return nil
		},
	}
	cmd.Flags().StringVar(&keyArg, "key", "", "개인키 파일 경로 (hex 또는 PEM) 또는 hex 문자열")
	cmd.Flags().StringVar(&outPath, "out", "", "서명 출력 경로 (기본: {BINARY}.sig)")
	return cmd
}

// loadSigningKey 는 --key 값을 해석한다.
//
// 우선순위:
//  1. 경로가 존재하는 파일이면 LoadPrivateKeyFromFile (hex/PEM 자동 분기)
//  2. 그렇지 않으면 hex 문자열로 해석 (편의)
func loadSigningKey(deps imageDeps, keyArg string) (ed25519.PrivateKey, error) {
	if _, err := os.Stat(keyArg); err == nil {
		return deps.LoadPrivateKey(keyArg)
	}
	// 파일이 아니면 hex 직접 입력으로 간주.
	return updater.LoadPrivateKeyFromHex(keyArg)
}
