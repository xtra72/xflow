package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api"
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
		host       string
		port       int
		logLevel   string
	)

	cmd := &cobra.Command{
		Use:   "xflowd",
		Short: "xflow 플랫폼 데몬 서버",
		Long:  "xflowd 는 xflow 플랫폼의 중앙 서버로, API 서버 + Flow Engine + Agent Manager 를 실행합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(configFile, host, port, logLevel)
		},
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", "", "설정 파일 경로 (기본: ~/.xflow/config.yaml)")
	cmd.PersistentFlags().StringVar(&host, "host", "", "바인드 호스트 (설정 파일 값 우선)")
	cmd.PersistentFlags().IntVar(&port, "port", 0, "바인드 포트 (설정 파일 값 우선)")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "로그 레벨 (debug, info, warn, error)")

	cmd.AddCommand(newVersionCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "버전 정보를 출력합니다",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("xflowd %s (commit: %s, built: %s, go: %s)\n",
				Version, Commit, BuildDate, runtime.Version())
		},
	}
}

func runServer(configFile, host string, port int, logLevel string) error {
	// 1. 설정 로딩
	var loadOpts []config.LoadOption
	if configFile != "" {
		loadOpts = append(loadOpts, config.WithConfigFile(configFile))
	}
	cfg, err := config.Load(loadOpts...)
	if err != nil {
		return fmt.Errorf("설정 로딩 실패: %w", err)
	}

	// 2. 관찰성 초기화
	obs := observe.New()
	logger := obs.Loggers.NewLogger("xflowd")

	logger.Info("xflowd 시작",
		"version", Version,
		"commit", Commit,
	)

	// 3. 노드 레지스트리 (빌트인 10종 자동 등록)
	registry := node.NewRegistry()

	// 4. Flow 엔진
	engineLogger := obs.Loggers.NewLogger("engine")
	eng := engine.NewEngine(
		engine.WithNodeRegistry(registry),
		engine.WithLogger(engineLogger),
		engine.WithMetrics(obs.Metrics),
	)

	// 5. Agent 매니저
	agentMgr := agent.NewManager()

	// 6. API 서버 설정
	serverCfg := cfg.Server()
	if host != "" {
		serverCfg.Host = host
	}
	if port != 0 {
		serverCfg.Port = port
	}

	apiLogger := obs.Loggers.NewLogger("api")
	server := api.NewServer(&serverCfg,
		api.WithLogger(apiLogger.Logger()),
	)

	// 7. 기본 라우트 (/health, /ready)
	server.SetupRoutes()

	// 8. Flow/Agent API 핸들러 등록
	// handler.FlowManager / handler.AgentManager 인터페이스는
	// engine.Engine / agent.DefaultManager 와 직접 매칭되지 않으므로
	// 서비스 계층 어댑터가 필요하다 (별도 SPEC 으로 구현 예정).
	// 현재는 /health, /ready 엔드포인트만 활성화 상태이다.
	_ = eng
	_ = agentMgr

	// 9. 시그널 처리 및 서버 시작
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("종료 시그널 수신", "signal", sig.String())
		cancel()
	}()

	logger.Info("서버 시작 중",
		"host", serverCfg.Host,
		"port", serverCfg.Port,
	)

	// server.Start 는 ctx 취소 시 자동으로 Stop 호출
	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("서버 실행 실패: %w", err)
	}

	// 정리: Agent 매니저 종료
	if shutdownErr := agentMgr.Shutdown(context.Background()); shutdownErr != nil {
		logger.Warn("에이전트 매니저 종료 실패", "error", shutdownErr)
	}

	logger.Info("xflowd 종료 완료")
	return nil
}
