package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/internal/agent/samsung"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/api/service"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	_ "github.com/xtra/xflow/internal/node/adapter" // 브릿지 어댑터 init() 등록
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/internal/storage"
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
		logOutput  string
	)

	cmd := &cobra.Command{
		Use:   "xflowd",
		Short: "xflow 플랫폼 데몬 서버",
		Long:  "xflowd 는 xflow 플랫폼의 중앙 서버로, API 서버 + Flow Engine + Agent Manager 를 실행합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(configFile, host, port, logLevel, logOutput)
		},
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", "", "설정 파일 경로 (기본: ~/.xflow/config.yaml)")
	cmd.PersistentFlags().StringVar(&host, "host", "", "바인드 호스트 (설정 파일 값 우선)")
	cmd.PersistentFlags().IntVar(&port, "port", 0, "바인드 포트 (설정 파일 값 우선)")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "로그 레벨 (debug, info, warn, error)")
	cmd.PersistentFlags().StringVar(&logOutput, "log-output", "", "로그 출력 대상 (stdout, 파일 경로, stdout+파일경로)")

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

func runServer(configFile, host string, port int, logLevel, logOutput string) error {
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
	obsCfg := cfg.Observe()
	var obsOpts []observe.Option

	// 2-1. 로그 레벨 설정 (CLI --log-level > config observe.default_level > 기본 info)
	if logLevel != "" {
		lvl, err := observe.ParseLogLevel(logLevel)
		if err != nil {
			return fmt.Errorf("잘못된 --log-level 값: %w", err)
		}
		obsOpts = append(obsOpts, observe.WithObserverDefaultLevel(lvl))
	} else if obsCfg.DefaultLevel != "" {
		lvl, err := observe.ParseLogLevel(obsCfg.DefaultLevel)
		if err != nil {
			return fmt.Errorf("잘못된 observe.default_level 설정: %w", err)
		}
		obsOpts = append(obsOpts, observe.WithObserverDefaultLevel(lvl))
	}

	// 2-2. 로그 포맷 설정 (config observe.format)
	if obsCfg.Format != "" {
		obsOpts = append(obsOpts, observe.WithObserverFormat(obsCfg.Format))
	}

	// 2-3. 로그 출력 대상 설정 (CLI --log-output > config observe.output > 기본 stdout)
	outputTarget := obsCfg.Output
	if logOutput != "" {
		outputTarget = logOutput
	}
	if outputTarget != "" && outputTarget != "stdout" {
		writer, closer, err := config.ParseLogOutput(outputTarget)
		if err != nil {
			return fmt.Errorf("로그 출력 설정 실패: %w", err)
		}
		if closer != nil {
			defer closer.Close()
		}
		obsOpts = append(obsOpts, observe.WithObserverWriter(writer))
	}

	obs := observe.New(obsOpts...)
	logger := obs.Loggers.NewLogger("xflowd")

	logger.Info("xflowd 시작",
		"version", Version,
		"commit", Commit,
	)

	// 3. 시스템 에이전트 매니저
	sysMgr := system.NewSystemAgentManager()
	sysCfg := system.SystemConfig{
		FileSandboxRoot: os.TempDir(),
		Observer:        obs,
	}
	if err := sysMgr.Initialize(sysCfg); err != nil {
		return fmt.Errorf("시스템 에이전트 초기화 실패: %w", err)
	}
	if err := sysMgr.Start(context.Background()); err != nil {
		return fmt.Errorf("시스템 에이전트 시작 실패: %w", err)
	}

	// 4. 노드 레지스트리 (빌트인 10종 자동 등록)
	registry := node.NewRegistry()

	// 5. Agent 매니저 (엔진보다 먼저 생성 - 엔진에 resolver로 주입)
	agentMgr := agent.NewManager(agent.WithObserver(obs))

	// 5.1. 에이전트 타입 등록
	if err := system.RegisterHTTPTypes(agentMgr); err != nil {
		return fmt.Errorf("HTTP agent type registration failed: %w", err)
	}
	if err := system.RegisterConsoleLoggerType(agentMgr); err != nil {
		return fmt.Errorf("console-logger agent type registration failed: %w", err)
	}
	if err := system.RegisterMQTTTypes(agentMgr); err != nil {
		return fmt.Errorf("MQTT agent type registration failed: %w", err)
	}
	if err := system.RegisterInfluxDBTypes(agentMgr); err != nil {
		return fmt.Errorf("InfluxDB agent type registration failed: %w", err)
	}
	if err := samsung.RegisterSamsungNASATypes(agentMgr); err != nil {
		return fmt.Errorf("Samsung NASA agent type registration failed: %w", err)
	}
	if err := modbus.RegisterModbusTypes(agentMgr); err != nil {
		return fmt.Errorf("MODBUS TCP agent type registration failed: %w", err)
	}
	if err := modbusserver.RegisterModbusServerTypes(agentMgr); err != nil {
		return fmt.Errorf("MODBUS TCP Server agent type registration failed: %w", err)
	}

	// 6. Flow 엔진 (AgentResolver를 NodeOption으로 전달)
	engineLogger := obs.Loggers.NewLogger("engine")
	agentResolver := engine.NewAgentManagerResolver(agentMgr)
	eng := engine.NewEngine(
		engine.WithNodeRegistry(registry),
		engine.WithLogger(engineLogger),
		engine.WithMetrics(obs.Metrics),
		engine.WithObserver(obs),
		engine.WithNodeOptions(node.WithAgentResolver(agentResolver)),
	)

	// 6.5. 플로우 저장소 초기화
	storageCfg := cfg.Storage()
	storageLogger := obs.Loggers.NewLogger("storage")
	repo, err := storage.NewRepository(context.Background(), storageCfg)
	if err != nil {
		return fmt.Errorf("스토리지 초기화 실패: %w", err)
	}
	defer repo.Close()
	storageLogger.Info("스토리지 초기화 완료", "type", storageCfg.Type)

	// 6.6. Agent 저장소 초기화
	agentRepo, err := storage.NewAgentRepository(context.Background(), storageCfg)
	if err != nil {
		storageLogger.Error("agent 저장소 초기화 실패", "error", err)
		return fmt.Errorf("agent 저장소 초기화 실패: %w", err)
	}
	defer agentRepo.Close()

	// 저장소에서 에이전트 로드
	agentConfigs, err := agentRepo.List(context.Background())
	if err != nil {
		storageLogger.Warn("저장소에서 에이전트 로드 실패", "error", err)
	} else {
		for _, cfg := range agentConfigs {
			if _, err := agentMgr.Create(cfg); err != nil {
				storageLogger.Warn("에이전트 복원 실패", "id", cfg.ID, "name", cfg.Name, "error", err)
			} else {
				storageLogger.Info("에이전트 복원 완료", "id", cfg.ID, "name", cfg.Name)
			}
		}
		if len(agentConfigs) > 0 {
			storageLogger.Info("에이전트 복원 완료", "count", len(agentConfigs))
		}
	}

	// 7. API 서버 설정
	serverCfg := cfg.Server()
	if host != "" {
		serverCfg.Host = host
	}
	if port != 0 {
		serverCfg.Port = port
	}

	server := api.NewServer(&serverCfg,
		api.WithObserver(obs),
	)

	// 8. 기본 라우트 (/health, /ready)
	server.SetupRoutes()

	// 9. WebSocket 허브 (핸들러보다 먼저 생성 - EventPublisher 주입을 위해)
	wsHub := ws.NewHub(obs.Loggers.NewLogger("api.ws.hub").Logger())
	go wsHub.Run()
	defer wsHub.Stop()

	eventPub := ws.NewEventPublisher(wsHub, obs.Loggers.NewLogger("api.ws.event").Logger())

	// 9.1. Flow/Agent/Node API 핸들러 등록
	flowSvc := service.NewFlowServiceAdapter(eng, repo, obs.Loggers.NewLogger("api.service.flow").Logger())
	agentSvc := service.NewAgentServiceAdapter(agentMgr, agentRepo, obs.Loggers.NewLogger("api.service.agent").Logger())
	nodeSvc := service.NewNodeServiceAdapter(registry, obs.Loggers.NewLogger("api.service.node").Logger())

	flowHandler := handler.NewFlowHandler(flowSvc, obs.Loggers.NewLogger("api.handler.flow").Logger(), handler.WithEventPublisher(eventPub), handler.WithAgentManager(agentSvc))
	agentHandler := handler.NewAgentHandler(agentSvc, obs.Loggers.NewLogger("api.handler.agent").Logger())
	nodeHandler := handler.NewNodeHandler(nodeSvc, obs.Loggers.NewLogger("api.handler.node").Logger())
	monitorMgr := handler.NewDefaultMonitorManager(obs.Loggers.NewLogger("api.handler.monitor").Logger(), obs.Levels)
	monitorHandler := handler.NewMonitorHandler(monitorMgr, obs.Loggers.NewLogger("api.handler.monitor").Logger())

	// 9.2. Device API 핸들러 등록
	deviceRegistry := device.NewRegistry()
	metadataDir := filepath.Join(filepath.Dir(storageCfg.SQLitePath), "device_metadata")
	deviceMetaRepo, err := storage.NewDeviceMetadataFileRepository(metadataDir)
	if err != nil {
		logger.Error("디바이스 메타데이터 저장소 초기화 실패", "error", err)
		return fmt.Errorf("디바이스 메타데이터 저장소 초기화 실패: %w", err)
	}
	defer deviceMetaRepo.Close()
	deviceHandler := handler.NewDeviceHandler(deviceRegistry, deviceMetaRepo, obs.Loggers.NewLogger("api.handler.device").Logger())

	server.RegisterRoutes(func(g *api.RouteGroup) {
		flowHandler.RegisterRoutes(g)
		agentHandler.RegisterRoutes(g)
		nodeHandler.RegisterRoutes(g)
		monitorHandler.RegisterRoutes(g)
		deviceHandler.RegisterRoutes(g)
	})

	// 9.5. WebSocket 핸들러 등록
	wsHandler := handler.NewWebSocketHandler(wsHub, obs.Loggers.NewLogger("api.handler.websocket").Logger())
	server.RegisterRawHandler("GET /ws", wsHandler.HandleUpgrade)

	// 9.6. 모니터링 브로드캐스터 (WebSocket 을 통한 실시간 메트릭 전송)
	broadcaster := ws.NewMonitoringBroadcaster(wsHub, eng, obs.Loggers.NewLogger("api.ws.broadcaster").Logger(), ws.WithStreamRouter(obs.Streams))

	// 9.8. Web UI 정적 파일 서빙 (모든 라우트 등록 후 마지막에 설정)
	if serverCfg.WebUI.Enabled {
		if err := server.SetupWebUI(serverCfg.WebUI.Dir); err != nil {
			logger.Warn("Web UI 서빙 비활성화", "error", err)
		}
	}

	// 10. 시그널 처리 및 서버 시작
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 모니터링 브로드캐스터 시작 (ctx 생성 후)
	broadcaster.Start(ctx)
	defer broadcaster.Stop()

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

	// 정리: 시스템 에이전트 매니저 종료
	if stopErr := sysMgr.Stop(context.Background()); stopErr != nil {
		logger.Warn("시스템 에이전트 매니저 종료 실패", "error", stopErr)
	}

	logger.Info("xflowd 종료 완료")
	return nil
}
