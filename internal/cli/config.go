package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// defaultConfig 는 config init 시 생성되는 기본 설정 내용이다.
const defaultConfig = `server:
  url: "http://localhost:8080"
  timeout: 30s
auth:
  token: ""
output:
  format: "table"
  color: true
`

// maskToken 은 토큰 값을 마스킹한다.
// 빈 토큰은 빈 문자열, 4글자 이하는 "****", 5글자 이상은 "****" + 마지막 4글자를 반환한다.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

// newConfigCmd 는 config 커맨드와 하위 서브커맨드를 생성한다.
// configPath 는 설정 파일 경로 포인터, confirmFn 은 확인 함수(테스트 주입용)이다.
func newConfigCmd(configPath *string, confirmFn func(string, io.Reader) bool) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "설정 관리",
		Long:  "xflow CLI 설정 파일을 관리합니다.",
	}

	// 서브커맨드 등록
	configCmd.AddCommand(newConfigInitCmd(configPath, confirmFn))
	configCmd.AddCommand(newConfigGetCmd(configPath))
	configCmd.AddCommand(newConfigSetCmd(configPath))
	configCmd.AddCommand(newConfigServerCmd(configPath))
	configCmd.AddCommand(newConfigTokenCmd(configPath))
	configCmd.AddCommand(newConfigListCmd(configPath))

	return configCmd
}

// newConfigInitCmd 는 config init 서브커맨드를 생성한다.
// 기본 설정 파일을 생성하며, 이미 존재하면 confirmFn 으로 덮어쓰기 확인한다.
func newConfigInitCmd(configPath *string, confirmFn func(string, io.Reader) bool) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "설정 파일 초기화",
		Long:  "기본 설정 파일(~/.xflow/config.yaml)을 생성합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := *configPath

			// 기존 파일 존재 여부 확인
			if _, err := os.Stat(path); err == nil {
				// 파일이 이미 존재하면 덮어쓰기 확인
				if !confirmFn("설정 파일이 이미 존재합니다. 덮어쓰시겠습니까?", os.Stdin) {
					fmt.Fprintln(cmd.OutOrStdout(), "설정 초기화가 취소되었습니다.")
					return nil
				}
			}

			// 부모 디렉토리 생성
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("디렉토리 생성 실패: %s: %w", dir, err)
			}

			// 기본 설정 파일 쓰기
			if err := os.WriteFile(path, []byte(defaultConfig), 0644); err != nil {
				return fmt.Errorf("설정 파일 쓰기 실패: %s: %w", path, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "설정 파일이 생성되었습니다: %s\n", path)
			return nil
		},
	}
}

// newConfigGetCmd 는 config get <key> 서브커맨드를 생성한다.
// 설정 파일에서 지정된 키의 값을 출력한다.
func newConfigGetCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "설정 값 조회",
		Long:  "설정 파일에서 지정된 키의 값을 조회합니다. 키는 점 표기법을 사용합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			v := viper.New()
			v.SetConfigFile(*configPath)
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("설정 파일 읽기 실패: %w", err)
			}

			value := v.GetString(key)
			if value != "" {
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}

			return nil
		},
	}
}

// newConfigSetCmd 는 config set <key> <value> 서브커맨드를 생성한다.
// 설정 파일의 지정된 키에 값을 쓴다.
func newConfigSetCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "설정 값 변경",
		Long:  "설정 파일의 지정된 키에 값을 설정합니다.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			return setConfigValue(*configPath, key, value, cmd.OutOrStdout())
		},
	}
}

// newConfigServerCmd 는 config server <url> 서브커맨드를 생성한다.
// config set server.url <url> 의 단축 명령어이다.
func newConfigServerCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "server <url>",
		Short: "서버 URL 설정",
		Long:  "xflowd 서버 URL 을 설정합니다. 'config set server.url <url>' 의 단축 명령어입니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setConfigValue(*configPath, "server.url", args[0], cmd.OutOrStdout())
		},
	}
}

// newConfigTokenCmd 는 config token <token> 서브커맨드를 생성한다.
// config set auth.token <token> 의 단축 명령어이다.
func newConfigTokenCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "token <token>",
		Short: "인증 토큰 설정",
		Long:  "인증 토큰을 설정합니다. 'config set auth.token <token>' 의 단축 명령어입니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setConfigValue(*configPath, "auth.token", args[0], cmd.OutOrStdout())
		},
	}
}

// newConfigListCmd 는 config list 서브커맨드를 생성한다.
// 모든 활성 설정 값을 출력하며, 토큰은 마스킹 처리한다.
func newConfigListCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "모든 설정 값 출력",
		Long:  "현재 활성화된 모든 설정 값을 출력합니다. 토큰은 마스킹 처리됩니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.New()
			v.SetConfigFile(*configPath)
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("설정 파일 읽기 실패: %w", err)
			}

			// 모든 설정 키를 수집
			allKeys := v.AllKeys()
			sort.Strings(allKeys)

			w := cmd.OutOrStdout()
			for _, key := range allKeys {
				value := v.GetString(key)

				// 토큰 관련 키는 마스킹 처리
				if strings.Contains(key, "token") {
					value = maskToken(value)
				}

				fmt.Fprintf(w, "%s: %s\n", key, value)
			}

			return nil
		},
	}
}

// setConfigValue 는 설정 파일에서 키의 값을 변경한다.
// 기존 설정을 로드하고, 키를 업데이트한 뒤, 파일에 다시 쓴다.
func setConfigValue(configPath, key, value string, w io.Writer) error {
	v := viper.New()
	v.SetConfigFile(configPath)

	// 기존 설정 로드 (없으면 무시)
	_ = v.ReadInConfig()

	// 값 설정
	v.Set(key, value)

	// 파일에 쓰기
	if err := v.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("설정 파일 쓰기 실패: %w", err)
	}

	fmt.Fprintf(w, "%s = %s\n", key, value)
	return nil
}
