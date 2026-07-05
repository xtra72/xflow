package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// loginResponse 는 POST /api/v1/auth/login 응답의 data 필드 구조이다.
// 서버 DTO(dto.LoginResponse)와 동일한 {user, tokens} 중첩 구조를 따른다.
type loginResponse struct {
	User struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
		TokenType    string `json:"token_type"`
	} `json:"tokens"`
}

// refreshResponse 는 POST /api/v1/auth/refresh 응답의 data 필드 구조이다.
// 서버 DTO(dto.RefreshResponse)와 동일한 flat 구조를 따른다.
type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// secretPrompter 는 마스킹된 비밀번호 입력을 읽는 함수 타입이다.
// 기본 구현은 터미널에서 입력을 마스킹하며, 테스트에서는 스텁으로 대체한다.
type secretPrompter func(prompt string) (string, error)

// readSecret 은 표준 입력에서 마스킹된 비밀번호를 읽는다.
// stdin 이 터미널이면 x/term 으로 에코를 끄고, 아니면(파이프 등) 일반 라인으로 읽는다.
// 비밀번호는 어떤 경우에도 화면이나 로그에 평문 출력되지 않는다.
func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("비밀번호 입력 실패: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}

	// 비터미널(파이프/리다이렉션): 일반 라인 읽기
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("비밀번호 입력 실패: 입력이 없습니다")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// promptLine 은 표준 입력에서 한 줄(평문)을 읽는다. 사용자명 등 비밀이 아닌 입력에 사용한다.
func promptLine(reader io.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return "", fmt.Errorf("입력 실패: 입력이 없습니다")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// resolveConfigPath 는 커맨드의 --config 플래그 또는 기본 경로(~/.xflow/config.yaml)를 반환한다.
// root.go 의 PersistentPreRunE 와 동일한 우선순위를 따른다.
func resolveConfigPath(cmd *cobra.Command) string {
	if flagVal, _ := cmd.Root().PersistentFlags().GetString("config"); flagVal != "" {
		return flagVal
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".xflow", "config.yaml")
	}
	return ""
}

// saveTokenToConfig 는 발급/갱신된 토큰을 로컬 config(auth.token)에 저장한다.
// config.go 의 setConfigValue 헬퍼를 재사용하며, 출력은 io.Discard 로 버린다.
func saveTokenToConfig(cmd *cobra.Command, token string) error {
	path := resolveConfigPath(cmd)
	if path == "" {
		return fmt.Errorf("설정 파일 경로를 결정할 수 없습니다")
	}
	// 부모 디렉토리 보장 (config init 전에 login 하는 경우 대비)
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	return setConfigValue(path, "auth.token", token, io.Discard)
}

// clearTokenFromConfig 는 로컬 config 의 auth.token 값을 비운다.
func clearTokenFromConfig(cmd *cobra.Command) error {
	path := resolveConfigPath(cmd)
	if path == "" {
		return fmt.Errorf("설정 파일 경로를 결정할 수 없습니다")
	}
	if _, err := os.Stat(path); err != nil {
		// 설정 파일이 없으면 지울 토큰도 없으므로 성공 처리
		return nil
	}
	return setConfigValue(path, "auth.token", "", io.Discard)
}

// newAuthCmd 는 인증 관리 커맨드 그룹을 생성한다.
// 5개의 서브커맨드(login, logout, whoami, passwd, refresh)를 등록한다.
// confirmFn 은 향후 확인 프롬프트가 필요한 서브커맨드를 위해 시그니처에 포함한다.
func newAuthCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "인증 관리 명령어",
		Long:  "로그인, 로그아웃, 현재 사용자 조회, 비밀번호 변경 등 인증 관련 작업을 수행합니다.",
	}

	authCmd.AddCommand(newAuthLoginCmd(client))
	authCmd.AddCommand(newAuthLogoutCmd(client))
	authCmd.AddCommand(newAuthWhoamiCmd(client))
	authCmd.AddCommand(newAuthPasswdCmd(client))
	authCmd.AddCommand(newAuthRefreshCmd(client))

	return authCmd
}

// newAuthLoginCmd 는 auth login 서브커맨드를 생성한다.
// POST /api/v1/auth/login 으로 인증 후 발급된 토큰을 프로세스 메모리(sessionToken)에 보관한다.
// 기본적으로 디스크에 저장하지 않으므로 프로세스 재시작 시 재로그인이 필요하며,
// --save 플래그를 지정하면 로컬 config(auth.token)에도 영구 저장한다.
// --username/--password 미제공 시 대화형으로 입력받으며, 비밀번호는 마스킹된다.
func newAuthLoginCmd(client **Client) *cobra.Command {
	var (
		username string
		password string
		save     bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "로그인 및 토큰 발급",
		Long:  "사용자 인증 후 JWT 토큰을 발급받아 현재 세션에 적용합니다. --save 지정 시 로컬 설정에 영구 저장합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 사용자명: 플래그 미제공 시 프롬프트
			if username == "" {
				v, err := promptLine(cmd.InOrStdin(), "사용자명: ")
				if err != nil {
					return err
				}
				username = v
			}
			if username == "" {
				return ErrInvalidInput("사용자명은 필수입니다")
			}

			// 비밀번호: 플래그 미제공 시 마스킹 프롬프트
			if password == "" {
				v, err := authReadSecret("비밀번호: ")
				if err != nil {
					return err
				}
				password = v
			}
			if password == "" {
				return ErrInvalidInput("비밀번호는 필수입니다")
			}

			reqBody := map[string]string{
				"username": username,
				"password": password,
			}

			var result loginResponse
			if err := (*client).Post("/api/v1/auth/login", reqBody, &result); err != nil {
				return err
			}

			token := result.Tokens.AccessToken
			if token == "" {
				return &CLIError{
					Message:  "서버가 액세스 토큰을 반환하지 않았습니다",
					ExitCode: 1,
				}
			}

			// 발급된 토큰을 프로세스 메모리(sessionToken)에 보관하여 동일 세션 내
			// 후속 명령이 인증되게 한다. 디스크에는 기본적으로 저장하지 않는다.
			sessionToken = token

			// 메모리상 클라이언트에도 토큰을 반영하여 동일 세션 내 후속 호출이 인증되게 한다.
			(*client).token = token

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "로그인 성공: %s (%s)\n", result.User.Username, result.User.Role)

			// --save 지정 시에만 로컬 config 에 영구 저장한다.
			if save {
				if err := saveTokenToConfig(cmd, token); err != nil {
					return err
				}
				fmt.Fprintln(w, "토큰이 설정에 저장되었습니다.")
			} else {
				fmt.Fprintln(w, "토큰이 현재 세션에 적용되었습니다 (디스크에 저장하지 않음 — 재시작 시 재로그인 필요).")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&username, "username", "u", "", "사용자명")
	cmd.Flags().StringVarP(&password, "password", "p", "", "비밀번호 (비권장: 평문 노출 위험, 미입력 시 마스킹 프롬프트 사용)")
	cmd.Flags().BoolVar(&save, "save", false, "토큰을 설정 파일에 영구 저장 (기본: 세션 메모리에만 보관)")

	return cmd
}

// newAuthLogoutCmd 는 auth logout 서브커맨드를 생성한다.
// POST /api/v1/auth/logout 으로 토큰을 블랙리스트 처리하고 로컬 config 의 토큰을 제거한다.
func newAuthLogoutCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "로그아웃 및 토큰 제거",
		Long:  "현재 토큰을 서버에서 무효화하고 로컬 설정의 토큰을 제거합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			// 서버 측 블랙리스트 처리 (현재 클라이언트 토큰을 Authorization 헤더로 전송)
			var result map[string]any
			err := (*client).Post("/api/v1/auth/logout", nil, &result)

			// 서버 호출 결과와 무관하게 로컬 토큰은 제거한다.
			// 세션 메모리 토큰과 --save 로 저장된 config 토큰을 모두 비운다.
			sessionToken = ""
			if clearErr := clearTokenFromConfig(cmd); clearErr != nil {
				return clearErr
			}
			(*client).token = ""

			if err != nil {
				// 서버 무효화 실패 시에도 로컬 토큰은 제거되었음을 알린다.
				fmt.Fprintln(w, "로컬 토큰이 제거되었습니다. (서버 무효화는 실패했습니다)")
				return err
			}

			fmt.Fprintln(w, "로그아웃되었습니다. 토큰이 제거되었습니다.")
			return nil
		},
	}
}

// authWhoamiFieldOrder 는 whoami 상세 출력의 필드 순서이다.
var authWhoamiFieldOrder = []string{"username", "role"}

// authWhoamiLabelMap 는 whoami 상세 출력의 필드 라벨 매핑이다.
var authWhoamiLabelMap = map[string]string{
	"username": "Username",
	"role":     "Role",
}

// newAuthWhoamiCmd 는 auth whoami 서브커맨드를 생성한다.
// GET /api/v1/auth/me 로 현재 인증된 사용자 정보를 조회한다.
func newAuthWhoamiCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "현재 사용자 정보 조회",
		Long:  "현재 인증된 사용자의 이름과 역할을 표시합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var info map[string]any
			if err := (*client).Get("/api/v1/auth/me", &info); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(authWhoamiFieldOrder, authWhoamiLabelMap, nil)
				return df.Format(info, w)
			}
			return PrintResult(w, format, info, nil, nil)
		},
	}
}

// newAuthPasswdCmd 는 auth passwd 서브커맨드를 생성한다.
// PUT /api/v1/auth/password 로 현재 사용자의 비밀번호를 변경한다.
// --current/--new 미제공 시 대화형 마스킹 프롬프트로 입력받는다.
func newAuthPasswdCmd(client **Client) *cobra.Command {
	var (
		current string
		newPass string
	)

	cmd := &cobra.Command{
		Use:   "passwd",
		Short: "비밀번호 변경",
		Long:  "현재 비밀번호를 확인한 뒤 새 비밀번호로 변경합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if current == "" {
				v, err := authReadSecret("현재 비밀번호: ")
				if err != nil {
					return err
				}
				current = v
			}
			if newPass == "" {
				v, err := authReadSecret("새 비밀번호: ")
				if err != nil {
					return err
				}
				newPass = v
				// 확인 입력으로 오타를 방지한다.
				confirm, err := authReadSecret("새 비밀번호 확인: ")
				if err != nil {
					return err
				}
				if confirm != newPass {
					return ErrInvalidInput("새 비밀번호가 일치하지 않습니다")
				}
			}

			if current == "" || newPass == "" {
				return ErrInvalidInput("현재 비밀번호와 새 비밀번호는 필수입니다")
			}

			reqBody := map[string]string{
				"current_password": current,
				"new_password":     newPass,
			}

			var result map[string]any
			if err := (*client).Put("/api/v1/auth/password", reqBody, &result); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "비밀번호가 변경되었습니다.")
			return nil
		},
	}

	cmd.Flags().StringVar(&current, "current", "", "현재 비밀번호 (비권장: 미입력 시 마스킹 프롬프트 사용)")
	cmd.Flags().StringVar(&newPass, "new", "", "새 비밀번호 (비권장: 미입력 시 마스킹 프롬프트 사용)")

	return cmd
}

// newAuthRefreshCmd 는 auth refresh 서브커맨드를 생성한다.
// POST /api/v1/auth/refresh 로 토큰을 갱신하고 새 액세스 토큰을 프로세스 메모리에 반영한다.
// login 과 동일하게 기본적으로 디스크에 저장하지 않으며, --save 지정 시 config 에 영구 저장한다.
// --refresh-token 플래그로 리프레시 토큰을 직접 전달할 수 있다.
func newAuthRefreshCmd(client **Client) *cobra.Command {
	var (
		refreshToken string
		save         bool
	)

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "토큰 갱신",
		Long:  "리프레시 토큰으로 새 액세스 토큰을 발급받아 현재 세션에 적용합니다. --save 지정 시 로컬 설정에 영구 저장합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if refreshToken == "" {
				return ErrInvalidInput("리프레시 토큰(--refresh-token)이 필요합니다")
			}

			reqBody := map[string]string{
				"refresh_token": refreshToken,
			}

			var result refreshResponse
			if err := (*client).Post("/api/v1/auth/refresh", reqBody, &result); err != nil {
				return err
			}

			token := result.AccessToken
			if token == "" {
				return &CLIError{
					Message:  "서버가 액세스 토큰을 반환하지 않았습니다",
					ExitCode: 1,
				}
			}

			// 새 토큰을 세션 메모리와 라이브 클라이언트에 반영한다 (디스크 미저장 기본).
			sessionToken = token
			(*client).token = token

			w := cmd.OutOrStdout()
			if save {
				if err := saveTokenToConfig(cmd, token); err != nil {
					return err
				}
				fmt.Fprintln(w, "토큰이 갱신되어 설정에 저장되었습니다.")
			} else {
				fmt.Fprintln(w, "토큰이 갱신되어 현재 세션에 적용되었습니다 (디스크에 저장하지 않음 — 재시작 시 재로그인 필요).")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&refreshToken, "refresh-token", "", "갱신에 사용할 리프레시 토큰")
	cmd.Flags().BoolVar(&save, "save", false, "토큰을 설정 파일에 영구 저장 (기본: 세션 메모리에만 보관)")

	return cmd
}

// authReadSecret 은 마스킹 비밀번호 입력 함수이다.
// 테스트에서 이 변수를 교체하여 입력을 주입한다(터미널 없이 검증 가능).
var authReadSecret secretPrompter = readSecret
