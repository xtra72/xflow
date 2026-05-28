package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/century"
	"github.com/xtra/xflow/internal/agent/lg"
	"github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/internal/agent/samsung"
	"github.com/xtra/xflow/internal/agent/serial"
	"github.com/xtra/xflow/internal/agent/socket"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/api/service"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	_ "github.com/xtra/xflow/internal/node/adapter" // 브릿지 어댑터 init() 등록
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/internal/script"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/internal/updater"
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
	cmd.AddCommand(newUpdateCmd(defaultUpdateDeps())) // @SPEC:SPEC-UPDATE-001 v0.1.0
	cmd.AddCommand(newMigrateCmd())                   // @SPEC:SPEC-DEVICE-IDENTITY-001 Phase C § C1
	cmd.AddCommand(newPreflightCmd())                 // @SPEC:SPEC-DEVICE-IDENTITY-001 Phase D § D-T5

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

	// 4.1. 차트 채널 레지스트리 (SPEC-CHART-001 M2).
	// chart-emitter 노드 Init 과 /ws/chart/{channel} WS 핸들러가 공유하는 프로세스 전역 싱글톤.
	// 플로우 자동 시작(9.1a) 이전에 설정되어야 chart-emitter 가 Register 호출 시 사용 가능하다.
	chartChannelRegistry := system.NewChartChannelRegistry()
	system.SetDefaultChartChannelRegistry(chartChannelRegistry)
	defer func() {
		chartChannelRegistry.Close()
		system.SetDefaultChartChannelRegistry(nil)
	}()

	// 5. Device 레지스트리 (에이전트 라이프사이클 훅에 필요하므로 매니저보다 먼저 생성)
	deviceRegistry := device.NewRegistry()

	// eventPub 포인터 (훅 클로저에서 참조 - wsHub 생성 후 설정됨)
	var eventPubRef *ws.EventPublisher

	// deviceMetaRepo 포인터 (훅 클로저에서 참조 - 저장소 초기화 후 설정됨)
	var deviceMetaRepoRef *storage.DeviceMetadataFileRepository

	// engineRef 포인터 (OnRestart 훅에서 참조 - 엔진 생성 후 설정됨)
	var engineRef *engine.Engine

	// 5.1. Agent 매니저 (엔진보다 먼저 생성 - 엔진에 resolver로 주입)
	agentMgr := agent.NewManager(
		agent.WithObserver(obs),
		agent.WithOnStart(func(a agent.Agent) {
			type deviceProviderAgent interface {
				DeviceProvider() device.DeviceProvider
			}
			if dpa, ok := a.(deviceProviderAgent); ok {
				deviceRegistry.RegisterProvider(a.Name(), dpa.DeviceProvider())
				logger.Info("디바이스 프로바이더 등록", "agent", a.Name(), "type", a.Type())

				// 영속화된 메타데이터를 레지스트리에 복원.
				//
				// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1):
				// 메타데이터 key 는 UUID (Device.ID() == Device.UID()) 이다.
				// 본 에이전트 (a.Name()) 가 소유한 디바이스의 UUID 집합을 조회한 뒤,
				// 메타데이터 저장소에서 해당 UUID 의 항목만 복원한다.
				if repo := deviceMetaRepoRef; repo != nil {
					allMeta, err := repo.List(context.Background())
					if err == nil {
						ownedUIDs := make(map[string]bool)
						for _, dev := range dpa.DeviceProvider().Devices() {
							if id := dev.ID(); id != "" {
								ownedUIDs[id] = true
							}
						}
						for id, meta := range allMeta {
							if ownedUIDs[id] {
								if setErr := deviceRegistry.SetMetadata(id, meta); setErr == nil {
									logger.Debug("디바이스 메타데이터 복원", "device_id", id)
								}
							}
						}
					}
				}

				if ep := eventPubRef; ep != nil {
					ep.PublishDeviceEvent(ws.EventDeviceOnline, a.Name())
				}
			} else {
				logger.Info("디바이스 프로바이더 없음", "agent", a.Name(), "type", a.Type(), "impl", fmt.Sprintf("%T", a))
			}
			// 디바이스 상태 변경 시 WebSocket 브로드캐스트 콜백 등록.
			//
			// SPEC-DEVICE-IDENTITY-001 Phase D (M3 / D-T10/T11): V2 콜백만 1급
			// 진입점이다. Phase B 의 v1 fallback 분기는 greenfield 가정에 따라 제거됨.
			type deviceStateChangeAgentV2 interface {
				SetDeviceStateChangeCallbackV2(agent.DeviceStateChangeCallbackV2)
			}
			if dsaV2, ok := a.(deviceStateChangeAgentV2); ok {
				dsaV2.SetDeviceStateChangeCallbackV2(func(_, deviceUID, deviceCompositeID string) {
					if ep := eventPubRef; ep != nil {
						ep.PublishDeviceStateChangedV2(deviceUID, deviceCompositeID)
					}
				})
			}

			// 고정 설치(pinned) 디바이스 로드 및 등록.
			//
			// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1):
			// 메타데이터 key 는 UUID 이므로 composite prefix 매칭이 불가능하다.
			// agent 가 소유한 device 의 UUID 와 (agentName, localID) 매핑을
			// DeviceProvider 의 Devices() 로 enumerate 하여, 해당 UUID 가 pinned
			// 메타데이터에 있으면 RegisterPinnedDevices 로 보고한다.
			//
			// 단, 본 경로는 이미 device provider 에 등록된 디바이스 (auto-discover
			// 결과) 만 처리할 수 있다. 첫 부팅 시 pinned 메타데이터만 있고 디바이스가
			// 아직 발견되지 않은 경우는 별도 yaml 설정 또는 후속 발견에 의존한다.
			type pinnedDeviceAgent interface {
				RegisterPinnedDevices(entries []agent.DeviceEntry)
			}
			if pda, ok := a.(pinnedDeviceAgent); ok {
				if repo := deviceMetaRepoRef; repo != nil {
					allMeta, err := repo.List(context.Background())
					if err != nil {
						logger.Error("고정 설치 디바이스 조회 실패", "agent", a.Name(), "error", err)
					} else if dpa2, ok := a.(deviceProviderAgent); ok {
						// agent 가 소유한 디바이스의 UUID -> localID 매핑 구축.
						type localIDProvider interface {
							LocalID() string
						}
						uidToLocalID := make(map[string]string)
						for _, dev := range dpa2.DeviceProvider().Devices() {
							uid := dev.ID()
							if uid == "" {
								continue
							}
							// localID 는 device.Name() (사람이 읽는 라벨) 이 아닌
							// 어댑터 내부 식별자. 우선 LocalID() 확장 인터페이스를
							// 시도하고, 없으면 device.Name() 으로 fallback (대부분
							// 어댑터에서 label 이 동일하게 사용됨).
							if lp, ok := dev.(localIDProvider); ok {
								uidToLocalID[uid] = lp.LocalID()
							} else {
								uidToLocalID[uid] = dev.Name()
							}
						}
						var entries []agent.DeviceEntry
						for id, meta := range allMeta {
							if meta.Pinned == nil || !*meta.Pinned {
								continue
							}
							addr, owned := uidToLocalID[id]
							if !owned || addr == "" {
								continue
							}
							entries = append(entries, agent.DeviceEntry{
								Address: addr,
								Name:    "", // 에이전트 내부 기본 라벨 사용
							})
						}
						if len(entries) > 0 {
							pda.RegisterPinnedDevices(entries)
							logger.Info("고정 설치 디바이스 등록 완료", "agent", a.Name(), "count", len(entries))
						}
					}
				}
			}

			// 에이전트 시작 시 (초기 Start 및 Stop→Start 사이클 모두 포함) 해당
			// 에이전트를 참조하는 모든 실행 중인 노드를 재초기화한다. WithOnRestart
			// 는 Manager.Restart() 단일 호출 경로에서만 발화하므로, UI 가 Stop 과
			// Start 를 별도 호출하는 경로에서는 본 콜백에서 처리해야 한다.
			// 실행 중이지 않은 플로우의 노드는 ReinitNodesForAgent 내부에서
			// 건너뛰므로 초기 부트스트랩에서는 no-op 이다.
			if eng := engineRef; eng != nil {
				eng.ReinitNodesForAgent(a.ID(), a.Name())
			}
		}),
		agent.WithOnRestart(func(a agent.Agent) {
			if eng := engineRef; eng != nil {
				eng.ReinitNodesForAgent(a.ID(), a.Name())
			}
		}),
		agent.WithOnStop(func(a agent.Agent) {
			type deviceProviderAgent interface {
				DeviceProvider() device.DeviceProvider
			}
			if _, ok := a.(deviceProviderAgent); ok {
				deviceRegistry.UnregisterProvider(a.Name())
				if ep := eventPubRef; ep != nil {
					ep.PublishDeviceEvent(ws.EventDeviceOffline, a.Name())
				}
			}
		}),
	)

	// 5.1. 에이전트 타입 등록
	if err := system.RegisterHTTPTypes(agentMgr); err != nil {
		return fmt.Errorf("HTTP agent type registration failed: %w", err)
	}
	if err := system.RegisterConsoleLoggerType(agentMgr); err != nil {
		return fmt.Errorf("logger agent type registration failed: %w", err)
	}
	if err := system.RegisterMQTTTypes(agentMgr); err != nil {
		return fmt.Errorf("MQTT agent type registration failed: %w", err)
	}
	// SPEC-DEVICE-IDENTITY-001 Phase D § D-T17: dual-tag 부착 기능이 제거되어
	// RegisterInfluxDBTypesWithResolver 가 RegisterInfluxDBTypes 로 단일화됨.
	if err := system.RegisterInfluxDBTypes(agentMgr); err != nil {
		return fmt.Errorf("InfluxDB agent type registration failed: %w", err)
	}
	if err := samsung.RegisterSamsungHvacr01Types(agentMgr); err != nil {
		return fmt.Errorf("Samsung NASA agent type registration failed: %w", err)
	}
	if err := lg.RegisterLGLGAPTypes(agentMgr); err != nil {
		return fmt.Errorf("LG LGAP agent type registration failed: %w", err)
	}
	if err := lg.RegisterLGLGCPTypes(agentMgr); err != nil {
		return fmt.Errorf("LG LGCP agent type registration failed: %w", err)
	}
	if err := lg.RegisterHvacr01Types(agentMgr); err != nil {
		logger.Error("lg_hvacr01 에이전트 타입 등록 실패", "error", err)
	}
	if err := century.RegisterHvacr01Types(agentMgr); err != nil {
		return fmt.Errorf("Century HVACR-01 agent type registration failed: %w", err)
	}
	if err := modbus.RegisterModbusTypes(agentMgr); err != nil {
		return fmt.Errorf("MODBUS TCP agent type registration failed: %w", err)
	}
	if err := modbusserver.RegisterModbusServerTypes(agentMgr); err != nil {
		return fmt.Errorf("MODBUS TCP Server agent type registration failed: %w", err)
	}
	if err := socket.RegisterSocketTypes(agentMgr); err != nil {
		return fmt.Errorf("socket agent type registration failed: %w", err)
	}
	if err := serial.RegisterSerialTypes(agentMgr); err != nil {
		return fmt.Errorf("serial agent type registration failed: %w", err)
	}
	if err := system.RegisterTSDBTypes(agentMgr); err != nil {
		return fmt.Errorf("TSDB agent type registration failed: %w", err)
	}
	if err := system.RegisterStoreTypes(agentMgr); err != nil {
		return fmt.Errorf("Store agent type registration failed: %w", err)
	}

	// 6. Flow 엔진 (AgentResolver + 시스템 Timer + 스크립트 엔진을 NodeOption으로 전달)
	engineLogger := obs.Loggers.NewLogger("engine")
	agentResolver := engine.NewAgentManagerResolver(agentMgr)
	// Trigger 노드 등 시스템 타이머를 필요로 하는 노드용 주입 옵션.
	// 시스템 타이머는 agent manager 가 아닌 system agent manager 소속이므로
	// AgentResolver 경로로는 접근할 수 없어 직접 주입한다.
	timerNodeOpt := node.WithTimer(sysMgr.Timer())

	// Lua 스크립트 엔진 초기화. 단일 엔진을 모든 script 노드가 공유하되,
	// 노드별 어댑터(자체 scriptID 보관)를 통해 격리한다.
	scriptEngine := script.NewScriptEngine()
	if err := scriptEngine.Init(context.Background()); err != nil {
		return fmt.Errorf("스크립트 엔진 초기화 실패: %w", err)
	}
	defer func() {
		_ = scriptEngine.Shutdown(context.Background())
	}()
	scriptFactoryOpt := node.WithScriptEngineFactory(func(nodeID string) node.ScriptEngine {
		return script.NewNodeEngineAdapter(scriptEngine, nodeID)
	})

	// SPEC-INVENTORY-001: inventory 노드용 4종 의존성 resolver.
	// 함수형 resolver 는 eng 자기 참조(FlowRegistry) 의 초기화 순서 문제를 회피한다.
	// eng 가 채워진 후 inventory 노드 Init 시점에 함수가 호출되어 실제 인스턴스를 획득한다.
	var eng *engine.Engine
	inventoryDeviceRegOpt := node.WithDeviceRegistryFunc(func() device.DeviceRegistry { return deviceRegistry })
	inventoryAgentMgrOpt := node.WithAgentManagerFunc(func() agent.Manager { return agentMgr })
	inventoryNodeRegOpt := node.WithNodeRegistryFunc(func() *node.Registry { return registry })
	inventoryFlowRegOpt := node.WithFlowRegistryFunc(func() node.FlowRegistry { return eng })

	eng = engine.NewEngine(
		engine.WithNodeRegistry(registry),
		engine.WithLogger(engineLogger),
		engine.WithMetrics(obs.Metrics),
		engine.WithObserver(obs),
		engine.WithNodeOptions(
			node.WithAgentResolver(agentResolver),
			timerNodeOpt,
			scriptFactoryOpt,
			// SPEC-INVENTORY-001: inventory 노드 의존성 (4종 source 별 read-only resolver)
			inventoryDeviceRegOpt,
			inventoryAgentMgrOpt,
			inventoryNodeRegOpt,
			inventoryFlowRegOpt,
		),
		engine.WithAgentManager(agentMgr),
		engine.WithOnAgentStart(func(a agent.Agent) {
			agentMgr.NotifyStarted(a)
		}),
	)
	engineRef = eng

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

	// 6.7. 디바이스 메타데이터 저장소 초기화 (에이전트 복원 전에 필요)
	metadataDir := filepath.Join(filepath.Dir(storageCfg.SQLitePath), "device_metadata")
	deviceMetaRepo, err := storage.NewDeviceMetadataFileRepository(metadataDir)
	if err != nil {
		logger.Error("디바이스 메타데이터 저장소 초기화 실패", "error", err)
		return fmt.Errorf("디바이스 메타데이터 저장소 초기화 실패: %w", err)
	}
	defer deviceMetaRepo.Close()
	deviceMetaRepoRef = deviceMetaRepo

	// 6.7.1. 메타데이터 pre-load — 에이전트 시작 전에 모든 메타데이터를 registry
	// 에 적재.
	//
	// 배경: auto-discovered 디바이스는 부팅 직후엔 아직 발견되지 않을 수 있으므로
	// 에이전트 OnStart 의 ownedUIDs 필터가 비어 있어 metadata 복원이 skip 됨.
	// 결과: 사용자가 변경한 이름이 재시작 후 사라짐.
	// 해결: 에이전트 등록과 독립적으로 모든 metadata 를 registry 에 사전 적재.
	// SetMetadata 는 디바이스 미존재를 허용하므로 (2026-05-27 변경) 가능.
	if allMeta, listErr := deviceMetaRepo.List(context.Background()); listErr == nil {
		preloaded := 0
		for id, meta := range allMeta {
			if setErr := deviceRegistry.SetMetadata(id, meta); setErr == nil {
				preloaded++
			}
		}
		logger.Info("디바이스 메타데이터 pre-load 완료", "count", preloaded)
	} else {
		logger.Warn("디바이스 메타데이터 pre-load 실패", "error", listErr)
	}

	// 6.8. 디바이스 ID (UUID) 저장소 초기화 (v0.18.6).
	// 5 HVAC 에이전트가 (agentName, unitID) → device_id (UUID) 매핑을 영속화.
	deviceIDDir := filepath.Join(filepath.Dir(storageCfg.SQLitePath), "device_ids")
	deviceIDRepo, err := storage.NewDeviceIDFileRepository(deviceIDDir)
	if err != nil {
		logger.Error("디바이스 ID 저장소 초기화 실패", "error", err)
		return fmt.Errorf("디바이스 ID 저장소 초기화 실패: %w", err)
	}
	defer deviceIDRepo.Close()
	agent.SetDeviceIDRepository(deviceIDRepo)

	// 6.9. SPEC-DEVICE-IDENTITY-001 Phase D § D-T6 — 자동 부팅 sanity check.
	// device_metadata.json 에 composite key (legacy) 가 잔존하면 v1.0 부팅을 거부.
	// xflowd preflight 명령과 동일한 로직 (checkDeviceMetadataKeys) 재사용.
	dataDirForCheck := filepath.Dir(storageCfg.SQLitePath)
	if res := checkDeviceMetadataKeys(dataDirForCheck); !res.passed {
		logger.Error("부팅 거부: 영속 메타데이터에 composite key 잔존",
			"detail", res.message)
		return fmt.Errorf("xflowd v1.0 boot refused: %s.\n"+
			"Run 'xflowd preflight --data-dir %s' for full diagnostics, then "+
			"'xflowd migrate device-ids --metadata-dir %s/device_metadata' to migrate.",
			res.message, dataDirForCheck, dataDirForCheck)
	}
	logger.Info("부팅 sanity check 통과", "data_dir", dataDirForCheck)

	// 저장소에서 에이전트 로드
	agentConfigs, err := agentRepo.List(context.Background())
	if err != nil {
		storageLogger.Warn("저장소에서 에이전트 로드 실패", "error", err)
	} else {
		restoreAgents(context.Background(), agentMgr, agentConfigs, storageLogger.Logger())
	}

	// 7. API 서버 설정
	serverCfg := cfg.Server()
	if host != "" {
		serverCfg.Host = host
	}
	if port != 0 {
		serverCfg.Port = port
	}

	// 7.1. @SPEC:SPEC-DASHBOARD-001 v0.2.0 (UR-004, AC-9, M-8)
	// basic_auth 필수화 — 비활성 상태로는 부팅을 거부한다.
	//
	// 단, 환경 변수 XFLOW_ALLOW_NO_AUTH=1 이 설정되면 강제로 활성화하여 부팅을 계속
	// 진행한다 (개발/데모 환경 호환). 운영 환경에서는 사용하지 말 것.
	if !serverCfg.BasicAuth.Enabled {
		if os.Getenv("XFLOW_ALLOW_NO_AUTH") == "1" {
			serverCfg.BasicAuth.Enabled = true
			logger.Warn("auth: basic_auth 가 자동 활성화되었습니다 (XFLOW_ALLOW_NO_AUTH=1) — 운영 환경에서는 사용하지 마세요")
		} else {
			logger.Error("auth: basic_auth 는 필수입니다 (SPEC-DASHBOARD-001 v0.2.0). 부팅을 거부합니다. XFLOW_ALLOW_NO_AUTH=1 환경변수로 우회 가능 (권장하지 않음).")
			return fmt.Errorf("auth: basic_auth 는 필수입니다 (SPEC-DASHBOARD-001 v0.2.0). 설정에서 basic_auth.enabled=true 로 변경하거나 XFLOW_ALLOW_NO_AUTH=1 환경변수를 설정하세요")
		}
	}

	// 7.2. @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-8)
	// 공유 SQLite DB 핸들 — credentials / dashboard 두 모듈이 동일 xflow.db 를 공유.
	// flows / agents 저장소는 별도 *sql.DB 를 사용하지만 WAL 모드 (ASM-007) 하에서
	// 다중 핸들이 안전하게 공존한다.
	authDashboardDB, err := storage.OpenSQLiteDB(context.Background(), storageCfg.SQLitePath)
	if err != nil {
		return fmt.Errorf("auth/dashboard SQLite 초기화 실패: %w", err)
	}
	defer authDashboardDB.Close()

	// 7.3. 기본 인증(Basic Auth) 초기화 — SQLite 기반 자격증명 (UR-006).
	var serverOpts []api.ServerOption
	serverOpts = append(serverOpts, api.WithObserver(obs))

	// 자격증명 yaml 파일 경로 결정 (마이그레이션용 only — 이관 후 .migrated 로 rename).
	credYAMLPath := serverCfg.BasicAuth.CredentialsFile
	if credYAMLPath == "" {
		homeDir, _ := os.UserHomeDir()
		credYAMLPath = filepath.Join(homeDir, ".xflow", "users.yaml")
	}

	credentialsMgr := auth.NewCredentialsManager(authDashboardDB, credYAMLPath).
		WithLogger(obs.Loggers.NewLogger("auth").Logger())
	if err := credentialsMgr.EnsureDefaultAdmin(); err != nil {
		return fmt.Errorf("자격증명 초기화 실패: %w", err)
	}

	jwtSvc, err := auth.NewJWTService(
		serverCfg.BasicAuth.JWTSecret,
		serverCfg.BasicAuth.TokenExpiry,
		serverCfg.BasicAuth.RefreshExpiry,
	)
	if err != nil {
		return fmt.Errorf("JWT 서비스 초기화 실패: %w", err)
	}

	serverOpts = append(serverOpts, api.WithBasicAuth(jwtSvc))

	logger.Info("기본 인증 활성화",
		"yaml_migration_path", credYAMLPath,
		"token_expiry", serverCfg.BasicAuth.TokenExpiry,
	)

	server := api.NewServer(&serverCfg, serverOpts...)

	// 8. 기본 라우트 (/health, /ready)
	server.SetupRoutes()

	// 9. WebSocket 허브 (핸들러보다 먼저 생성 - EventPublisher 주입을 위해)
	wsHub := ws.NewHub(obs.Loggers.NewLogger("api.ws.hub").Logger())
	go wsHub.Run()
	defer wsHub.Stop()

	// DebugSink 주입: output 노드의 editor 출력을 WebSocket으로 브로드캐스트
	eng.SetDebugSink(ws.NewDebugSink(wsHub))

	eventPub := ws.NewEventPublisher(wsHub, obs.Loggers.NewLogger("api.ws.event").Logger())
	eventPubRef = eventPub

	// 9.1. Flow/Agent/Node API 핸들러 등록
	flowSvc := service.NewFlowServiceAdapter(eng, repo, obs.Loggers.NewLogger("api.service.flow").Logger())
	agentSvc := service.NewAgentServiceAdapter(agentMgr, agentRepo, obs.Loggers.NewLogger("api.service.agent").Logger())
	agentSvc.SetNameResolver(eng)
	nodeSvc := service.NewNodeServiceAdapter(registry, obs.Loggers.NewLogger("api.service.node").Logger())

	// 9.1a. 자동 시작 플로우 복원
	{
		autoStartLogger := obs.Loggers.NewLogger("flow.autostart").Logger()
		if storedFlows, flErr := repo.List(context.Background()); flErr == nil {
			autoStartCount := 0
			for _, f := range storedFlows {
				if f.Metadata()["auto_start"] == "true" {
					if err := flowSvc.StartFlow(context.Background(), f.ID()); err != nil {
						autoStartLogger.Warn("플로우 자동 시작 실패", "flowID", f.ID(), "flowName", f.Name(), "error", err)
					} else {
						autoStartLogger.Info("플로우 자동 시작 완료", "flowID", f.ID(), "flowName", f.Name())
						autoStartCount++
					}
				}
			}
			if autoStartCount > 0 {
				autoStartLogger.Info("플로우 자동 시작 완료", "count", autoStartCount)
			}
		}
	}

	flowHandler := handler.NewFlowHandler(flowSvc, obs.Loggers.NewLogger("api.handler.flow").Logger(), handler.WithEventPublisher(eventPub), handler.WithAgentManager(agentSvc))
	agentHandler := handler.NewAgentHandler(agentSvc, obs.Loggers.NewLogger("api.handler.agent").Logger(), handler.WithFlowManager(flowSvc))
	nodeHandler := handler.NewNodeHandler(nodeSvc, obs.Loggers.NewLogger("api.handler.node").Logger())

	// SPEC-CHART-001 M3/M5: 차트 채널 목록 + Store/InfluxDB HTTP 쿼리 핸들러.
	// agentMgr 는 AgentLookup(List() []agent.Agent) 인터페이스를 만족한다.
	chartHandler := handler.NewChartHandler(obs.Loggers.NewLogger("api.handler.chart").Logger())
	storeQueryHandler := handler.NewStoreQueryHandler(agentMgr,
		obs.Loggers.NewLogger("api.handler.store_query").Logger())
	influxdbQueryHandler := handler.NewInfluxDBQueryHandler(agentMgr,
		obs.Loggers.NewLogger("api.handler.influxdb_query").Logger())
	monitorMgr := handler.NewDefaultMonitorManager(obs.Loggers.NewLogger("api.handler.monitor").Logger(), obs.Levels)
	monitorHandler := handler.NewMonitorHandler(monitorMgr, obs.Loggers.NewLogger("api.handler.monitor").Logger())

	// 9.2. Device API 핸들러 등록
	deviceHandler := handler.NewDeviceHandler(deviceRegistry, deviceMetaRepo, obs.Loggers.NewLogger("api.handler.device").Logger(), handler.WithDeviceEventPublisher(eventPub))

	// 9.3. System / Update API 핸들러 등록 (SPEC-UPDATE-001 v0.1.0 M10)
	// 설정 로딩 실패 또는 binary path / 공개키 부재 시에도 데몬은 정상 기동하며,
	// /system/update/* 엔드포인트는 적절한 에러 (예: ErrUpdateInvalidInput) 를 반환한다.
	systemHandler := buildSystemHandler(cfg, obs)

	// 9.4. @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-8)
	// Dashboard API 핸들러 등록 — 공유/개인 snapshot 영속화.
	// authDashboardDB 는 7.2 에서 열린 공유 *sql.DB (credentials 와 공유).
	dashboardRepo, err := storage.NewDashboardSQLiteRepository(context.Background(), authDashboardDB)
	if err != nil {
		return fmt.Errorf("dashboard 저장소 초기화 실패: %w", err)
	}
	dashboardHandler := handler.NewDashboardHandler(
		dashboardRepo,
		server.JWTService(),
		obs.Loggers.NewLogger("api.handler.dashboard").Logger(),
	)

	server.RegisterRoutes(func(g *api.RouteGroup) {
		// 인증 상태 엔드포인트 (항상 등록 - 프론트엔드가 인증 활성화 여부를 확인)
		handler.RegisterAuthStatusRoute(g, serverCfg.BasicAuth.Enabled)

		// 인증 핸들러 (basic_auth 활성화 시)
		if serverCfg.BasicAuth.Enabled && credentialsMgr != nil && server.JWTService() != nil {
			authHandler := handler.NewAuthHandler(credentialsMgr, server.JWTService(), obs.Loggers.NewLogger("api.handler.auth").Logger())
			authHandler.RegisterRoutes(g)
		}

		flowHandler.RegisterRoutes(g)
		agentHandler.RegisterRoutes(g)
		nodeHandler.RegisterRoutes(g)
		monitorHandler.RegisterRoutes(g)
		deviceHandler.RegisterRoutes(g)

		// SPEC-CHART-001 M3/M5: 차트 채널 목록 및 Store/InfluxDB 쿼리 라우트.
		chartHandler.RegisterRoutes(g)
		storeQueryHandler.RegisterRoutes(g)
		influxdbQueryHandler.RegisterRoutes(g)

		// SPEC-UPDATE-001 v0.1.0 M10: 시스템 / 업데이트 라우트.
		systemHandler.RegisterRoutes(g)

		// SPEC-DASHBOARD-001 v0.2.0 M-8: 대시보드 라우트 (shared / mine).
		dashboardHandler.RegisterRoutes(g)
	})

	// 9.5. WebSocket 핸들러 등록
	wsHandler := handler.NewWebSocketHandler(wsHub, obs.Loggers.NewLogger("api.handler.websocket").Logger())
	if server.AuthEnabled() && server.JWTService() != nil {
		wsHandler.WithWebSocketAuth(server.JWTService())
	}
	server.RegisterRawHandler("GET /ws", wsHandler.HandleUpgrade)

	// 9.5a. 차트 채널 WebSocket 핸들러 (SPEC-CHART-001 M2)
	chartWSHandler := ws.NewChartChannelHandler(chartChannelRegistry,
		obs.Loggers.NewLogger("api.handler.chart_ws").Logger())
	server.RegisterRawHandler("GET /ws/chart/{channel}", chartWSHandler.HandleUpgrade)

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

// agentRestoreManager 는 restoreAgents 가 사용하는 최소 매니저 인터페이스이다.
// 테스트에서 가짜 매니저를 주입할 수 있도록 agent.Manager 의 일부만 추출한다.
type agentRestoreManager interface {
	Create(cfg agent.AgentConfig) (agent.Agent, error)
	Start(ctx context.Context, id string) error
}

// restoreAgents 는 저장소에서 로드한 에이전트 설정을 매니저에 등록하고
// enabled 상태인 경우에만 자동 시작한다.
//
// SPEC-AGENT-005 Phase 2 요구사항:
//   - R2.1: 모든 에이전트에 대해 IsEnabled() 를 확인한다.
//   - R2.2: disabled 에이전트는 Start() 를 호출하지 않는다.
//   - R2.3: "자동 시작 건너뜀 (disabled)" 로그를 INFO 레벨로 기록한다.
//   - R2.4: disabled 에이전트도 Create() 를 통해 매니저에 등록되어 List API 에 노출된다.
//   - R2.5: disabled 에이전트는 데몬 부팅 시 자동으로 시작되지 않는다.
func restoreAgents(ctx context.Context, mgr agentRestoreManager, configs []agent.AgentConfig, log *slog.Logger) {
	for _, cfg := range configs {
		if _, err := mgr.Create(cfg); err != nil {
			log.Warn("에이전트 복원 실패", "id", cfg.ID, "name", cfg.Name, "error", err)
			continue
		}

		// SPEC-AGENT-005: disabled 에이전트는 등록만 하고 자동 시작을 건너뛴다.
		if !cfg.IsEnabled() {
			log.Info("자동 시작 건너뜀 (disabled)", "id", cfg.ID, "name", cfg.Name)
			continue
		}

		// Create 후 Start 호출: onStart 콜백(DeviceProvider 등록 등)을 실행하고
		// 트랜스포트 연결 및 폴링/수신 루프를 시작한다.
		if err := mgr.Start(ctx, cfg.ID); err != nil {
			log.Warn("에이전트 시작 실패", "id", cfg.ID, "name", cfg.Name, "error", err)
			continue
		}
		log.Info("에이전트 복원 완료", "id", cfg.ID, "name", cfg.Name)
	}
	if len(configs) > 0 {
		log.Info("에이전트 복원 완료", "count", len(configs))
	}
}

// buildSystemHandler 는 SystemHandler 를 구성한다 (SPEC-UPDATE-001 v0.1.0 M10).
//
// update 설정 로딩 / 공개키 로딩 / binary path 추출 중 어느 단계라도 실패하면
// 핸들러는 여전히 생성되지만 PublicKey=nil 상태로 남으며, Apply 호출 시
// ErrUpdateInvalidInput 으로 실패한다 (graceful degradation).
//
// 데몬 기동을 update 설정 부재로 막지 않도록 모든 에러를 warn 로그로만 기록한다.
func buildSystemHandler(cfg config.Config, obs *observe.Observer) *handler.SystemHandler {
	logger := obs.Loggers.NewLogger("api.handler.system").Logger()

	// 1. UpdateSettings → updater.UpdateConfig 변환.
	updateCfg, err := cfg.Update().ToUpdater()
	if err != nil {
		logger.Warn("업데이트 설정 변환 실패 (handler 는 nil 공개키로 생성)",
			slog.String("error", err.Error()))
		updateCfg = updater.DefaultConfig()
	}

	// 2. 현재 실행 바이너리 경로 (롤백 / 교체 대상).
	binaryPath, err := os.Executable()
	if err != nil {
		logger.Warn("os.Executable 실패 (rollback 불가)",
			slog.String("error", err.Error()))
		binaryPath = ""
	}

	// 3. 공개키 로딩 (운영자가 명시한 PEM 파일 또는 빌트인 키).
	var pubKey ed25519.PublicKey
	pubKey, keyErr := updater.LoadPublicKeyFromFile(updateCfg.PublicKeyPath)
	if keyErr != nil {
		logger.Warn("공개키 로딩 실패 (Apply 는 ErrUpdateInvalidInput 으로 실패한다)",
			slog.String("error", keyErr.Error()),
			slog.String("public_key_path", updateCfg.PublicKeyPath))
	}

	svcCfg := handler.UpdateServiceConfig{
		Config:         updateCfg,
		BinaryPath:     binaryPath,
		PublicKey:      pubKey,
		CurrentVersion: Version,
		Commit:         Commit,
		BuildDate:      BuildDate,
		BinaryName:     "xflowd",
		Factories:      handler.DefaultUpdateServiceFactories(),
	}
	svc := handler.NewUpdateService(svcCfg, logger)
	return handler.NewSystemHandler(svc, logger)
}
