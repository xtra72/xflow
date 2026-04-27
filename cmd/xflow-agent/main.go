package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/observe"
)

// 빌드 시 ldflags 로 주입되는 변수
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	rootCmd := newRootCmd()
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		configFile string
		serverURL  string
		agentID    string
		heartbeat  time.Duration
		logLevel   string
	)

	cmd := &cobra.Command{
		Use:   "xflow-agent",
		Short: "xflow 경량 에지 에이전트",
		Long:  "xflow-agent 는 에지 환경에서 데이터를 수집하여 중앙 xflowd 서버로 전송하는 경량 바이너리입니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(configFile, serverURL, agentID, heartbeat, logLevel)
		},
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", "", "설정 파일 경로 (기본: ~/.xflow/agent.yaml)")
	cmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8080", "중앙 서버 URL")
	cmd.PersistentFlags().StringVar(&agentID, "agent-id", "", "에이전트 고유 ID (기본: 호스트명 기반)")
	cmd.PersistentFlags().DurationVar(&heartbeat, "heartbeat", 30*time.Second, "하트비트 간격")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "로그 레벨 (debug, info, warn, error)")

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newStatusCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "버전 정보를 출력합니다",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("xflow-agent %s (commit: %s, built: %s, go: %s)\n",
				Version, Commit, BuildDate, runtime.Version())
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "에이전트 로컬 상태를 확인합니다",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("xflow-agent %s\n", Version)
			fmt.Println("상태: 실행 전 (이 명령어는 프로세스 상태를 확인합니다)")
		},
	}
}

func runAgent(configFile, serverURL, agentID string, heartbeat time.Duration, logLevel string) error {
	// 1. 설정 로딩
	var loadOpts []config.LoadOption
	if configFile != "" {
		loadOpts = append(loadOpts, config.WithConfigFile(configFile))
	}
	cfg, err := config.Load(loadOpts...)
	if err != nil {
		return fmt.Errorf("설정 로딩 실패: %w", err)
	}

	// 2. 관찰성 초기화 (경량: 로거만 사용)
	obs := observe.New()
	logger := obs.Loggers.NewLogger("xflow-agent")

	// 에이전트 ID 결정 (플래그 > 호스트명)
	if agentID == "" {
		agentID, _ = os.Hostname()
		if agentID == "" {
			agentID = "agent-unknown"
		}
	}

	logger.Info("xflow-agent 시작",
		"version", Version,
		"agent_id", agentID,
		"server", serverURL,
		"heartbeat", heartbeat.String(),
	)

	// 3. 경량 노드 레지스트리 (filter, transform 만 등록)
	registry := node.NewRegistry(node.WithoutBuiltins())
	if regErr := registry.Register("filter", node.NewFilterNode); regErr != nil {
		return fmt.Errorf("filter 노드 등록 실패: %w", regErr)
	}
	if regErr := registry.Register("transform", node.NewTransformNode); regErr != nil {
		return fmt.Errorf("transform 노드 등록 실패: %w", regErr)
	}

	// 4. 경량 Flow 엔진
	engineLogger := obs.Loggers.NewLogger("engine")
	eng := engine.NewEngine(
		engine.WithNodeRegistry(registry),
		engine.WithLogger(engineLogger),
	)

	_ = cfg // 추후 에이전트 전용 설정 활용
	_ = eng // 추후 에지 플로우 실행에 사용

	// 5. 중앙 서버 연결 확인
	if connErr := checkServer(serverURL); connErr != nil {
		logger.Warn("중앙 서버 연결 실패 (오프라인 모드로 시작)", "error", connErr)
	} else {
		logger.Info("중앙 서버 연결 확인 완료", "server", serverURL)
	}

	// 6. 시그널 처리
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("종료 시그널 수신", "signal", sig.String())
		cancel()
	}()

	// 7. 하트비트 루프
	logger.Info("하트비트 루프 시작", "interval", heartbeat.String())
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("xflow-agent 종료 완료")
			return nil
		case <-ticker.C:
			if hbErr := sendHeartbeat(serverURL, agentID); hbErr != nil {
				logger.Warn("하트비트 전송 실패", "error", hbErr)
			} else {
				logger.Debug("하트비트 전송 완료")
			}
		}
	}
}

// checkServer 는 중앙 서버의 /health 엔드포인트를 확인한다.
func checkServer(serverURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("서버 상태 비정상: %d", resp.StatusCode)
	}
	return nil
}

// sendHeartbeat 는 중앙 서버에 하트비트를 전송한다.
func sendHeartbeat(serverURL, agentID string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/agents/heartbeat", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Agent-ID", agentID)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("하트비트 응답 오류: %d", resp.StatusCode)
	}
	return nil
}
