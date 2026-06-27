package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// 빌드 시 ldflags 로 주입되는 변수
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// defaultServerURL 은 서버 URL 의 기본값이다.
const defaultServerURL = "http://localhost:8080"

// NewRootCmd 는 xflow CLI 의 루트 커맨드를 생성한다.
// 모든 서브커맨드는 이 루트 커맨드에 등록된다.
func NewRootCmd() *cobra.Command {
	// 서브커맨드에서 공유할 클라이언트 포인터
	var client *Client

	// 서브커맨드에서 공유할 설정 파일 경로
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "xflow",
		Short: "xflow - 워크플로우 오케스트레이션 CLI",
		Long:  "xflow 는 xflowd 서버와 통신하여 워크플로우를 관리하는 커맨드라인 도구입니다.",
		// 서브커맨드 실행 전 설정 파일 로딩 및 클라이언트 초기화
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// --config 플래그가 있으면 사용, 없으면 기본 경로 결정
			flagConfigPath, _ := cmd.Flags().GetString("config")
			if flagConfigPath != "" {
				configPath = flagConfigPath
			} else {
				if homeDir, err := os.UserHomeDir(); err == nil {
					configPath = filepath.Join(homeDir, ".xflow", "config.yaml")
				}
			}

			// 설정 파일 로딩 (파일이 없으면 무시)
			loadConfig(configPath)

			// 서버 URL 및 토큰 결정
			serverURL := resolveServerURL(cmd, configPath)
			token := resolveToken(cmd, configPath)

			// verbose 플래그 확인
			verbose, _ := cmd.Flags().GetBool("verbose")

			// 클라이언트 생성
			client = NewClient(serverURL, token, 30*time.Second, verbose)

			return nil
		},
	}

	// 글로벌 플래그 등록 (7개)
	pflags := rootCmd.PersistentFlags()
	pflags.String("config", "", "설정 파일 경로 (기본값: ~/.xflow/config.yaml)")
	pflags.String("server", "", "xflowd 서버 URL")
	pflags.String("format", "table", "출력 형식 (json|yaml|table|text)")
	pflags.String("token", "", "인증 토큰")
	pflags.Bool("verbose", false, "상세 출력 모드")
	pflags.Bool("quiet", false, "조용한 출력 모드")
	pflags.Bool("no-color", false, "색상 출력 비활성화")

	// 서브커맨드 등록
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newFlowCmd(&client, confirmAction))
	rootCmd.AddCommand(newAgentCmd(&client, confirmAction))
	rootCmd.AddCommand(newConfigCmd(&configPath, confirmAction))
	rootCmd.AddCommand(newNodeCmd(&client))
	rootCmd.AddCommand(newPluginCmd(&client, confirmAction))
	rootCmd.AddCommand(newStatusCmd(&client))
	rootCmd.AddCommand(newInteractiveCmd(rootCmd, &client))
	rootCmd.AddCommand(newScriptCmd(rootCmd, &client))
	rootCmd.AddCommand(newModbusCmd(&client))
	// SPEC-CLI-004 P1: Web UI 패리티 신규 도메인 명령
	rootCmd.AddCommand(newAuthCmd(&client, confirmAction))
	rootCmd.AddCommand(newDeviceCmd(&client, confirmAction))
	rootCmd.AddCommand(newStoreCmd(&client, confirmAction))
	rootCmd.AddCommand(newTsdbCmd(&client, confirmAction))
	rootCmd.AddCommand(newMonitorCmd(&client, confirmAction))
	rootCmd.AddCommand(newSystemCmd(&client, confirmAction))
	rootCmd.AddCommand(newSettingsCmd(&client))

	return rootCmd
}

// newScriptCmd 는 script 서브커맨드를 생성한다.
// 스크립트 파일을 읽어 명령어를 순차 실행한다.
func newScriptCmd(rootCmd *cobra.Command, client **Client) *cobra.Command {
	var filePath string
	var continueOnError bool
	var dryRun bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "script",
		Short: "스크립트 파일 실행",
		Long:  "스크립트 파일에 작성된 명령어를 순차적으로 실행합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			writer := cmd.OutOrStdout()

			// 파일 파싱
			lines, err := ParseScriptFile(filePath)
			if err != nil {
				return err
			}

			// 세션 생성 및 실행
			session := NewInteractiveSession(rootCmd, client, writer)
			executor := NewScriptExecutor(session, continueOnError, writer)
			executor.SetDryRun(dryRun)
			executor.SetVerbose(verbose)
			result := executor.Execute(lines)

			// 요약 출력
			result.PrintSummary(writer)

			// 에러가 있으면 non-zero 종료 코드
			if result.HasErrors() {
				return fmt.Errorf("스크립트 실행 중 %d개의 명령어가 실패했습니다", result.Failed)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "실행할 스크립트 파일 경로")
	_ = cmd.MarkFlagRequired("file")
	cmd.Flags().BoolVar(&continueOnError, "continue-on-error", false, "에러 발생 시에도 계속 실행")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "명령어를 실행하지 않고 목록만 출력")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "각 명령어의 상세 실행 정보 출력")

	return cmd
}

// newVersionCmd 는 version 서브커맨드를 생성한다.
// 버전, 커밋 해시, 빌드 시간, Go 버전을 출력한다.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "xflow 버전 정보 출력",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(),
				"xflow version %s\ncommit: %s\nbuild date: %s\ngo version: %s\n",
				Version, Commit, BuildDate, runtime.Version())
			return nil
		},
	}
}

// loadConfig 는 설정 파일을 Viper 로 로드한다.
// 파일이 존재하지 않으면 에러 없이 기본값으로 동작한다.
func loadConfig(configPath string) {
	v := viper.New()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 기본 경로: ~/.xflow/config.yaml
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		v.SetConfigFile(filepath.Join(homeDir, ".xflow", "config.yaml"))
	}

	// 파일이 없으면 무시
	_ = v.ReadInConfig()
}

// configValue 는 설정 파일에서 지정된 키의 값을 읽는다.
// 설정 파일이 없거나 키가 존재하지 않으면 빈 문자열을 반환한다.
func configValue(configPath, key string) string {
	if configPath == "" {
		return ""
	}
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString(key)
}

// resolveServerURL 은 서버 URL 을 우선순위에 따라 결정한다.
// 우선순위: 1) --server 플래그 2) XFLOW_SERVER 환경변수 3) 설정 파일 4) 기본값
func resolveServerURL(cmd *cobra.Command, configPath string) string {
	// 1. --server 플래그
	if flagVal, _ := cmd.Flags().GetString("server"); flagVal != "" {
		return flagVal
	}

	// 2. XFLOW_SERVER 환경변수
	if envVal := os.Getenv("XFLOW_SERVER"); envVal != "" {
		return envVal
	}

	// 3. 설정 파일의 server.url
	if val := configValue(configPath, "server.url"); val != "" {
		return val
	}

	// 4. 기본값
	return defaultServerURL
}

// resolveToken 은 인증 토큰을 우선순위에 따라 결정한다.
// 우선순위: 1) --token 플래그 2) XFLOW_TOKEN 환경변수 3) 설정 파일
func resolveToken(cmd *cobra.Command, configPath string) string {
	// 1. --token 플래그
	if flagVal, _ := cmd.Flags().GetString("token"); flagVal != "" {
		return flagVal
	}

	// 2. XFLOW_TOKEN 환경변수
	if envVal := os.Getenv("XFLOW_TOKEN"); envVal != "" {
		return envVal
	}

	// 3. 설정 파일의 auth.token
	return configValue(configPath, "auth.token")
}

// confirmAction 은 사용자에게 Y/N 확인을 요청한다.
// "y" 또는 "yes" (대소문자 무관) 입력 시 true 를 반환한다.
func confirmAction(prompt string, reader io.Reader) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return false
	}

	input := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return input == "y" || input == "yes"
}

// readFile 은 지정된 경로의 파일을 읽고 에러를 래핑한다.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("파일 읽기 실패: %s: %w", path, err)
	}
	return data, nil
}

// detectFileFormat 은 파일 확장자를 기반으로 형식을 감지한다.
// json, yaml/yml 확장자를 인식하며, 알 수 없는 확장자는 빈 문자열을 반환한다.
func detectFileFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}
