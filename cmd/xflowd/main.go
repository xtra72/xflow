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
	"time"

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
	"github.com/xtra/xflow/internal/agent/xsfm"
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
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/schedulelog"
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
	cmd.AddCommand(newVerifyCmd())                    // 원격 업데이트 pre-flight 스모크 테스트
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
	// 데몬 프로세스 시작 시각을 1회 캡처한다(epoch ms). 원격 관리 클라이언트의 BASIC
	// 시스템 정보(started_at)로 보고되어 서버가 uptime 을 파생한다(v1.4 M9, REQ-K07/K08).
	// protocol 내부가 아니라 여기서 1회 캡처해 ClientConfig 로 주입한다(시작 시각 안정성).
	daemonStartedAtMs := time.Now().UnixMilli()

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

	// 2-4. 로그 식별자 표시 스타일 설정 (config observe.id_style, 기본 "both").
	//      로그 레벨과 마찬가지로 런타임에 API(PUT /monitor/logstyle)로 변경 가능하다.
	observe.SetLogIDStyle(obsCfg.IDStyle)

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
	if err := system.RegisterThingplusTypes(agentMgr); err != nil {
		return fmt.Errorf("Thingplus gateway agent type registration failed: %w", err)
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
	if err := lg.RegisterLGHvacr02Types(agentMgr); err != nil {
		return fmt.Errorf("LG HVACR-02 agent type registration failed: %w", err)
	}
	if err := lg.RegisterHvacr01Types(agentMgr); err != nil {
		logger.Error("lg_hvacr01 에이전트 타입 등록 실패", "error", err)
	}
	if err := century.RegisterHvacr01Types(agentMgr); err != nil {
		return fmt.Errorf("Century HVACR-01 agent type registration failed: %w", err)
	}
	if err := xsfm.RegisterXSFMTypes(agentMgr); err != nil {
		return fmt.Errorf("XSFM agent type registration failed: %w", err)
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

	// message-slim-metadata / enrich (C): agent / device 정규 정보 룩업.
	// enrich 노드, expression 빌트인(agentInfo/deviceInfo), Lua stdlib(xflow.agent/device),
	// 그리고 WS slim-expand 가 동일한 룩업 경로를 공유하도록 여기서 1회 구성한다.
	// agentMgr.Get / deviceRegistry.Get 을 감싸 id → {type,name} 을 반환한다(미존재 시 ok=false).
	//
	// 주의: 스크립트 엔진이 stdlib deps 로 이 룩업들을 참조하므로, 엔진 생성보다 먼저 구성한다.
	agentInfoLookup := node.AgentLookupFunc(func(id string) (node.RegistryMeta, bool) {
		a, err := agentMgr.Get(id)
		if err != nil || a == nil {
			return node.RegistryMeta{}, false
		}
		return node.RegistryMeta{Type: a.Type(), ID: a.ID(), Name: a.Name()}, true
	})
	deviceInfoLookup := node.DeviceLookupFunc(func(id string) (node.RegistryMeta, bool) {
		var meta node.RegistryMeta
		if d, err := deviceRegistry.Get(id); err == nil && d != nil {
			meta = node.RegistryMeta{Type: string(d.Type()), ID: d.ID(), Name: d.Name()}
		}
		// 전 계층 이름 통일: 사용자 지정 이름(레지스트리 metadata) 을 권위 소스로
		// 우선한다. 디바이스가 오프라인/미등록이어도 metadata 는 남아 있으므로,
		// Get 실패와 무관하게 GetMetadata 로 사용자 이름을 병합한다. 이로써 REST
		// 목록·enrich 노드·expression·in-flow device_state 가 단일 이름을 공유한다.
		if m, err := deviceRegistry.GetMetadata(id); err == nil && m.Name != "" {
			meta.Name = m.Name
			meta.ID = id
		}
		if meta.ID == "" && meta.Name == "" && meta.Type == "" {
			return node.RegistryMeta{}, false
		}
		return meta, true
	})
	enrichAgentOpt := node.WithAgentInfoLookup(agentInfoLookup)
	enrichDeviceOpt := node.WithDeviceInfoLookup(deviceInfoLookup)

	// expression 빌트인(agentInfo/deviceInfo)용 프로세스 전역 룩업 1회 설정.
	// 미설정이면 두 빌트인이 노출되지 않으므로(기존 동작), 여기서 명시 설정한다.
	node.SetExprLookups(agentInfoLookup, deviceInfoLookup)

	// Lua 스크립트 엔진 초기화. 단일 엔진을 모든 script 노드가 공유하되,
	// 노드별 어댑터(자체 scriptID 보관)를 통해 격리한다.
	//
	// Follow-up 1: 프로덕션 VM 에 xflow stdlib 를 등록한다(WithStdlib). agent/device
	// 모듈은 위 룩업을 재사용하여 xflow.agent.get / xflow.device.get 이 실제 동작한다.
	//
	// Follow-up A: store 모듈을 활성화한다(EnableStore). 고정 Store 를 주입하지 않고
	// (Store=nil), 엔진이 실행별(per-Execute) StoreProvider 를 자동 연결한다. 스크립트
	// 노드가 자신의 agent_ref/namespace 로 해석한 네임스페이스 스토어를 실행 시점에만
	// 바인딩하므로, 풀링된 VM 을 공유해도 네임스페이스가 섞이지 않는다. 스토어 미구성
	// 노드는 바인딩이 없어 xflow.store 가 nil-safe(nil/false/no-op) 로 동작한다.
	scriptStdlibOpts := script.StdlibOptions{EnableAgent: true, EnableDevice: true, EnableStore: true}
	scriptStdlibDeps := script.StdlibDeps{
		Agent: func(id string) (script.AgentInfo, bool) {
			m, ok := agentInfoLookup.LookupAgent(id)
			if !ok {
				return script.AgentInfo{}, false
			}
			return script.AgentInfo{Type: m.Type, ID: m.ID, Name: m.Name}, true
		},
		Device: func(id string) (script.AgentInfo, bool) {
			m, ok := deviceInfoLookup.LookupDevice(id)
			if !ok {
				return script.AgentInfo{}, false
			}
			return script.AgentInfo{Type: m.Type, ID: m.ID, Name: m.Name}, true
		},
	}
	// dead-config 수정: 그동안 script: 섹션(cfg.Script())이 엔진/노드에 전혀
	// 반영되지 않아 timeout·vm_pool_size·sandbox 설정이 무시됐다. 여기서 실효
	// 설정으로 해석하여 엔진 옵션과 노드 타임아웃을 실제로 연결한다.
	scriptLog := obs.Loggers.NewLogger("script").Logger()
	effScript := effectiveScriptSettings(cfg.Script(), scriptLog)
	scriptLog.Info("스크립트 엔진 설정 적용",
		"timeout", effScript.Timeout,
		"vm_pool_size", effScript.PoolSize,
		"sandbox_enabled", effScript.Sandbox.Enabled,
		"sandbox_max_memory_mb", effScript.Sandbox.MaxMemoryMB,
		"sandbox_max_execution_ms", effScript.Sandbox.MaxExecutionTime.Milliseconds(),
	)
	// WithStdlib 를 먼저 적용한 뒤, config 기반 옵션(타임아웃/풀크기/샌드박스)을 얹는다.
	scriptEngine := script.NewScriptEngine(
		append([]script.EngineOption{script.WithStdlib(scriptStdlibOpts, scriptStdlibDeps)}, effScript.engineOptions()...)...,
	)
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
			// dead-config 수정: 스크립트 노드 타임아웃을 config(script.timeout)에서
			// 연결한다. 이전에는 노드가 하드코딩된 5s 만 사용했다.
			node.WithScriptTimeout(effScript.Timeout),
			// SPEC-INVENTORY-001: inventory 노드 의존성 (4종 source 별 read-only resolver)
			inventoryDeviceRegOpt,
			inventoryAgentMgrOpt,
			inventoryNodeRegOpt,
			inventoryFlowRegOpt,
			// message-slim-metadata / enrich (C): enrich 노드 룩업 주입.
			enrichAgentOpt,
			enrichDeviceOpt,
		),
		engine.WithAgentManager(agentMgr),
		engine.WithOnAgentStart(func(a agent.Agent) {
			agentMgr.NotifyStarted(a)
		}),
	)
	engineRef = eng

	// 라이브 브리지 경계 tap 노드 타입을 등록한다(SPEC-SUBFLOW-001 P2, REQ-SUBFLOW-RB07).
	// 원격 참조 flow-node 의 노드 측 브리지가 참조 플로우를 실행할 때, 경계 와이어를 이
	// tap 노드로 재배선하여 입력 주입/출력 중계를 수행한다(엔진 불변 — 일반 노드 타입 추가).
	// 등록 실패는 치명적이지 않으므로(브리지 미구성과 동일 — 일반 플로우엔 영향 없음) 경고만.
	if regErr := service.RegisterBridgeTapNodes(registry); regErr != nil {
		obs.Loggers.NewLogger("remote.bridge").Warn("브리지 tap 노드 등록 실패", "error", regErr)
	}

	// SPEC-SUBFLOW-001 P3(그룹 RB): 매니저 측 라이브 브리지 엔드포인트 노드(입력 forwarder/
	// 출력 emitter)를 등록한다. 살아남은 remote:// flow-node 가 server 모드 배포 시 이 두
	// 노드로 재배선되어 원격 노드와 입출력을 브리지한다(엔진 불변 — 일반 노드 타입 추가).
	// 비-server 모드에선 인스턴스화되지 않으며(opener 미주입 → 재배선 거부), 등록만 무해하다.
	if regErr := service.RegisterRemoteBridgeNodes(registry); regErr != nil {
		obs.Loggers.NewLogger("remote.bridge").Warn("매니저 브리지 노드 등록 실패", "error", regErr)
	}

	// SPEC-SUBFLOW-002: 로컬 shared 모드 공유 경계 탭 노드(입력/출력)를 등록한다. 참조 플로우가
	// 경계 포트를 가지고 배포될 때 경계 와이어를 이 탭으로 재배선하여, 부모의 shared flow-node 가
	// in-process 라이브 브리지로 단일 실행 인스턴스에 연결할 수 있게 한다(엔진 불변 — 일반 노드
	// 타입 추가). 로컬 in-process 이므로 모드와 무관하게 무해하게 등록한다(부착 브리지 0개면
	// StripBoundaryWires 와 동작 동일).
	if regErr := service.RegisterSharedBoundaryNodes(registry); regErr != nil {
		obs.Loggers.NewLogger("remote.bridge").Warn("공유 경계 탭 노드 등록 실패", "error", regErr)
	}

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

	// 설비 역사/위치/기기 레지스트리 기본 영속 경로 배선(SPEC-XSFM-001).
	// registry_path/station_registry_path 설정이 비어 있어도 영속화가 기본 ON 이 되도록
	// 서버 데이터 디렉터리를 기본 베이스로 주입한다. 실제 경로는 각 에이전트 Init 에서
	// <dataDir>/xsfm/<agentID>/ 로 유도된다(설정 경로가 있으면 그 경로가 우선).
	// device_metadata/device_ids 와 동일한 베이스(dir(sqlite_path))를 재사용한다.
	xsfm.SetDefaultRegistryDir(filepath.Dir(storageCfg.SQLitePath))

	// device_id / device_info 키를 항상 에이전트 ID 기준으로 정규화하는 resolver 를
	// 주입한다. agentRef 가 이름("LG HVACR2")으로 들어오든 ID(UUID)로 들어오든
	// 동일한 device_id 가 발급되도록 보장한다 (SPEC-DEVICE-IDENTITY-001).
	// 이름→ID 변환은 agentMgr.ResolveAgentID(registry.GetByName)를 사용한다.
	agent.SetAgentIDResolver(agentMgr.ResolveAgentID)

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

	// 노드 출력 tap 주입: 와이어 없이 임의 노드의 출력 메시지를 관측한다.
	// tapRegistry 는 런타임 전용 (flowID,nodeID) 관측 집합이고, TapObserver 는
	// tap 된 노드의 출력만 node.output 으로 브로드캐스트한다 (미관측 노드는 zero-overhead).
	// 플로우 시작 이전에 주입하므로 핫 패스 atomic 읽기와 경쟁하지 않는다.
	//
	// message-slim-metadata: 기본 egress 정책은 슬림 — node.output 메타데이터의
	// agent / device 그룹은 id-only 로 나간다(type/name 은 레지스트리 정규 데이터이므로
	// 중복 운반하지 않는다). 클라이언트가 type/name 까지 한 번에 받길 원하면(expand opt-in)
	// 아래 expander 를 .WithExpander(...) 로 주입하면 된다 — 레지스트리(agentMgr/deviceRegistry)
	// 에서 type/name 을 역-수화한다.
	//
	// 기본은 슬림 유지(behavior: 정규 데이터 중복 제거). expand 가 필요하면 enrich/
	// expression 과 동일한 룩업(agentInfoLookup/deviceInfoLookup, 위에서 구성)을 재사용해
	// ws expander 를 구성하고 .WithExpander(...) 로 주입하면 된다:
	//
	//	expander := ws.NewGroupExpander(
	//	    func(id string) (map[string]string, bool) {
	//	        m, ok := agentInfoLookup.LookupAgent(id)
	//	        if !ok { return nil, false }
	//	        return map[string]string{"type": m.Type, "name": m.Name}, true
	//	    },
	//	    func(id string) (map[string]string, bool) {
	//	        m, ok := deviceInfoLookup.LookupDevice(id)
	//	        if !ok { return nil, false }
	//	        return map[string]string{"type": m.Type, "name": m.Name}, true
	//	    },
	//	)
	//	eng.SetOutputObserver(ws.NewTapObserver(tapRegistry, wsHub).WithExpander(expander))
	tapRegistry := ws.NewTapRegistry()
	eng.SetOutputObserver(ws.NewTapObserver(tapRegistry, wsHub))

	eventPub := ws.NewEventPublisher(wsHub, obs.Loggers.NewLogger("api.ws.event").Logger())
	eventPubRef = eventPub

	// 9.1. Flow/Agent/Node API 핸들러 등록
	flowSvc := service.NewFlowServiceAdapter(eng, repo, obs.Loggers.NewLogger("api.service.flow").Logger())
	// SPEC-SUBFLOW-002: 로컬 shared flow-node 의 in-process 라이브 브리지 opener 를 주입한다.
	// 모드와 무관하게(로컬 in-process) 항상 주입하여, 참조 플로우 배포가 공유 경계 탭을 설치하고
	// 부모의 shared flow-node 가 단일 실행 인스턴스에 연결되게 한다(SH04). 미주입 시 shared
	// flow-node 배포가 ErrSharedBridgeUnavailable 로 거부된다.
	flowSvc.SetLocalBridgeOpener(service.NewLocalBridgeOpener(eng, obs.Loggers.NewLogger("api.service.flow.localbridge").Logger()))
	agentSvc := service.NewAgentServiceAdapter(agentMgr, agentRepo, obs.Loggers.NewLogger("api.service.agent").Logger())
	agentSvc.SetNameResolver(eng)
	nodeSvc := service.NewNodeServiceAdapter(registry, obs.Loggers.NewLogger("api.service.node").Logger())

	// 9.1a. 자동 시작 플로우 복원은 원격 관리 와이어링 완료 이후(아래 10.2절)로 미뤄진다.
	// remote:// flow-node 를 가진 플로우는 server 모드의 SetRemoteBridgeOpener / client 모드의
	// SetBridgeTapSource 가 flowSvc 에 주입된 뒤에야 배포(재배선)할 수 있기 때문이다. 여기서
	// 자동 시작하면 opener/tap source 미주입 상태라 remote:// flow-node 가 ErrRemoteBridge
	// Unavailable 로 배포 실패한다(부팅 auto-start 회귀). 이 블록 이후의 핸들러/인벤토리/쿼리
	// 소스는 flowSvc 인스턴스만 참조하고 "이미 시작된 플로우"에 의존하지 않으므로 이동이 안전하다.

	flowHandler := handler.NewFlowHandler(flowSvc, obs.Loggers.NewLogger("api.handler.flow").Logger(), handler.WithEventPublisher(eventPub), handler.WithAgentManager(agentSvc), handler.WithTapRegistry(tapRegistry))
	agentHandler := handler.NewAgentHandler(agentSvc, obs.Loggers.NewLogger("api.handler.agent").Logger(), handler.WithFlowManager(flowSvc))
	nodeHandler := handler.NewNodeHandler(nodeSvc, obs.Loggers.NewLogger("api.handler.node").Logger())

	// SPEC-CHART-001 M3/M5: 차트 채널 목록 + Store/InfluxDB HTTP 쿼리 핸들러.
	// agentMgr 는 AgentLookup(List() []agent.Agent) 인터페이스를 만족한다.
	chartHandler := handler.NewChartHandler(obs.Loggers.NewLogger("api.handler.chart").Logger())
	storeQueryHandler := handler.NewStoreQueryHandler(agentMgr,
		obs.Loggers.NewLogger("api.handler.store_query").Logger())
	influxdbQueryHandler := handler.NewInfluxDBQueryHandler(agentMgr,
		obs.Loggers.NewLogger("api.handler.influxdb_query").Logger())
	influxdbManagementHandler := handler.NewInfluxDBManagementHandler(agentMgr,
		obs.Loggers.NewLogger("api.handler.influxdb_management").Logger())
	monitorMgr := handler.NewDefaultMonitorManager(obs.Loggers.NewLogger("api.handler.monitor").Logger(), obs.Levels)
	monitorHandler := handler.NewMonitorHandler(monitorMgr, obs.Loggers.NewLogger("api.handler.monitor").Logger())

	// 9.2. Device API 핸들러 등록
	// 디바이스 수신 데이터 이력(주기 스냅샷) 레코더 — 설정에 따라 구성/주입.
	// 주기 스냅샷 방식: interval 마다 전체 디바이스의 현재 상태를 디바이스별
	// 링버퍼(최대 max_entries)에 저장한다. 비활성 시 nil → history 라우트 미등록.
	deviceHistoryCfg := cfg.DeviceHistory()
	var deviceHistoryRecorder *device.DeviceHistoryRecorder
	deviceHandlerOpts := []handler.DeviceHandlerOption{handler.WithDeviceEventPublisher(eventPub)}
	if deviceHistoryCfg.Enabled {
		deviceHistoryRecorder = device.NewDeviceHistoryRecorder(deviceRegistry, device.DeviceHistoryConfig{
			Interval:   deviceHistoryCfg.Interval,
			MaxEntries: deviceHistoryCfg.MaxEntries,
		})
		deviceHandlerOpts = append(deviceHandlerOpts, handler.WithDeviceHistory(deviceHistoryRecorder))
		logger.Info("디바이스 이력 레코더 구성",
			"interval", deviceHistoryCfg.Interval,
			"max_entries", deviceHistoryCfg.MaxEntries)
	}
	deviceHandler := handler.NewDeviceHandler(deviceRegistry, deviceMetaRepo, obs.Loggers.NewLogger("api.handler.device").Logger(), deviceHandlerOpts...)

	// 9.3. System / Update API 핸들러 등록 (SPEC-UPDATE-001 v0.1.0 M10)
	// 설정 로딩 실패 또는 binary path / 공개키 부재 시에도 데몬은 정상 기동하며,
	// /system/update/* 엔드포인트는 적절한 에러 (예: ErrUpdateInvalidInput) 를 반환한다.
	// @SPEC:SPEC-WEB-007
	// 프로세스 부팅 시각(daemonStartedAtMs, epoch ms)을 재사용해 self uptime 의
	// 기준 시각을 가장 이른 시점으로 맞춘다 (buildSystemHandler 내 time.Now() 보다 정확).
	systemHandler := buildSystemHandler(cfg, obs, time.UnixMilli(daemonStartedAtMs))

	// 원격 관리 클라이언트 설정 핸들러 (SPEC-REMOTE-001 원격 관리 클라이언트 설정 UI).
	// 이 인스턴스의 client 모드 설정을 조회/편집하며, config 오버라이드 레이어에 영속화한다.
	remoteConfigHandler := handler.NewRemoteConfigHandler(cfg, obs.Loggers.NewLogger("api.handler.remote_config").Logger())

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

	// 9.5. 전역 설정(settings) API 핸들러 등록.
	// 디바이스 컬럼 구성 등 "전역 1벌" UI/서버 설정을 영속화한다. 공유 xflow.db
	// 핸들(authDashboardDB)을 재사용해 별도 파일 핸들을 늘리지 않는다(WAL 공존, ASM-007).
	settingsRepo, err := storage.NewSettingsSQLiteRepositoryWithDB(context.Background(), authDashboardDB)
	if err != nil {
		return fmt.Errorf("settings 저장소 초기화 실패: %w", err)
	}
	settingsHandler := handler.NewSettingsHandler(settingsRepo, obs.Loggers.NewLogger("api.handler.settings").Logger())

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
		influxdbManagementHandler.RegisterRoutes(g)

		// SPEC-UPDATE-001 v0.1.0 M10: 시스템 / 업데이트 라우트.
		systemHandler.RegisterRoutes(g)

		// SPEC-REMOTE-001: 원격 관리 클라이언트 설정 라우트 (admin 전용).
		remoteConfigHandler.RegisterRoutes(g)

		// SPEC-DASHBOARD-001 v0.2.0 M-8: 대시보드 라우트 (shared / mine).
		dashboardHandler.RegisterRoutes(g)

		// 전역 설정 라우트 (GET/PUT /settings/{key}) — 디바이스 컬럼 구성 등.
		settingsHandler.RegisterRoutes(g)
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

	// 9.5b. 원격 관리 (@SPEC:SPEC-REMOTE-001 M2) — mode 분기.
	// server 모드는 등록/승인 상태 머신(managed_nodes 영속 + JWT 노드 토큰)을 구성
	// 하고, 관리 WS 핸들러(별도 엔드포인트 /api/remote/ws — REQ-N02) + 관리자 승인
	// REST API 를 등록한다. online/offline 추적 sweeper 는 ctx 생성 후(아래 10절)
	// 시작한다. disabled(기본)는 어떤 관리 연결도 생성/수락하지 않는다(REQ-N03 회귀).
	rmCfg := cfg.RemoteManagement()
	var (
		remoteServer            *remote.Server
		remoteAdminHandler      *handler.RemoteAdminHandler
		scheduleLogHandler      *handler.ScheduleLogHandler
		scheduleLogRepo         storage.ScheduleLogRepository
		remoteEnrollmentHandler *handler.RemoteEnrollmentHandler
		remoteEditHandler       *handler.RemoteEditHandler
		remoteQueryHandler      *handler.RemoteQueryHandler
		remoteStreamHandler     *handler.RemoteStreamHandler
		remoteGroupingHandler   *handler.RemoteGroupingHandler
		releaseFeedHandler      *handler.ReleaseFeedHandler
		releaseAdminHandler     *handler.ReleaseAdminHandler
		managedNodeRepo         storage.ManagedNodeRepository
		mirrorRepo              storage.MirrorRepository
		remoteAuditRepo         storage.RemoteAuditRepository
		enrollmentTokenRepo     storage.EnrollmentTokenRepository
		nodeVersionHistoryRepo  storage.NodeVersionHistoryRepository
	)
	switch rmCfg.Mode {
	case "server":
		// managed_nodes 영속(서버 캐시) — 공유 SQLite DB 경로를 재사용한다(§5.4).
		mnRepo, mnErr := storage.NewManagedNodeRepository(context.Background(), "sqlite", storageCfg.SQLitePath)
		if mnErr != nil {
			return fmt.Errorf("관리 노드 저장소 초기화 실패: %w", mnErr)
		}
		managedNodeRepo = mnRepo
		defer managedNodeRepo.Close()

		// 인벤토리 미러 캐시(M4, §5.4) — 동일 SQLite DB 에 미러 테이블을 멱등 추가.
		mrRepo, mrErr := storage.NewMirrorRepository(context.Background(), "sqlite", storageCfg.SQLitePath)
		if mrErr != nil {
			return fmt.Errorf("인벤토리 미러 저장소 초기화 실패: %w", mrErr)
		}
		mirrorRepo = mrRepo
		defer mirrorRepo.Close()

		// 원격 변경 감사 로그(M6, REQ-F05) — 동일 SQLite DB 에 remote_audit 테이블을
		// 멱등 추가. 명령/승인/거부/폐기 mutation 을 누가/언제/어느 노드/결과로 기록한다.
		auRepo, auErr := storage.NewRemoteAuditRepository(context.Background(), "sqlite", storageCfg.SQLitePath)
		if auErr != nil {
			return fmt.Errorf("원격 감사 저장소 초기화 실패: %w", auErr)
		}
		remoteAuditRepo = auRepo
		defer remoteAuditRepo.Close()

		// 설비 제어 감사 저장소 배선(REQ-XSFM-001-F05): xsfm 패키지의
		// 감사 저장소 슬롯에 원격 감사 저장소를 주입한다. 미주입 시 감사는 no-op.
		xsfm.SetAuditRepository(remoteAuditRepo)

		// 스케줄(예약) 실행 로그 저장소 배선(SPEC-SCHEDULE-VIEW-001 M2): 공유 SQLite DB 에
		// schedule_log 테이블을 멱등 추가한다. fire 이벤트(trigger 발화)와 result 이벤트(xsfm
		// 제어 실행)를 각각 별도 경로로 기록하므로, 두 슬롯을 모두 주입한다 — node 측 발화 관측자
		// (fire)와 xsfm 측 저장소(result). 초기화 실패는 audit 배선과 동일하게 best-effort 로 로깅만
		// 하고 계속하며(미설정 시 양측 no-op), 제어/발화 경로를 죽이지 않는다.
		if slRepo, slErr := storage.NewScheduleLogRepository(context.Background(), "sqlite", storageCfg.SQLitePath); slErr != nil {
			logger.Error("스케줄 로그 저장소 초기화 실패 — 스케줄 로그 비활성(제어/발화는 정상)", "error", slErr)
		} else {
			defer slRepo.Close()
			scheduleLogRepo = slRepo                                                  // 읽기 측(GET /schedules/logs) 재사용을 위한 공유 인스턴스 캡처(M3)
			xsfm.SetScheduleLogRepository(slRepo)                                     // result 이벤트(제어 실행 측)
			node.SetScheduleFireObserver(schedulelog.NewFireObserver(slRepo, logger)) // fire 이벤트(발화 측 어댑터)
		}

		// enrollment 토큰 저장소(v1.1 그룹 H) — 동일 SQLite DB 에 enrollment_tokens
		// 테이블을 멱등 추가. 토큰은 SHA-256 해시로만 저장된다(REQ-H06).
		etRepo, etErr := storage.NewEnrollmentTokenRepository(context.Background(), "sqlite", storageCfg.SQLitePath)
		if etErr != nil {
			return fmt.Errorf("enrollment 토큰 저장소 초기화 실패: %w", etErr)
		}
		enrollmentTokenRepo = etRepo
		defer enrollmentTokenRepo.Close()

		// 노드 버전 변경 이력(버전 관리 Phase 1) — 동일 SQLite DB 에 node_version_history
		// 테이블을 멱등 추가. 노드 version 이 직전 저장값과 달라질 때마다 한 줄 append 한다.
		vhRepo, vhErr := storage.NewNodeVersionHistoryRepository(context.Background(), "sqlite", storageCfg.SQLitePath)
		if vhErr != nil {
			return fmt.Errorf("노드 버전 이력 저장소 초기화 실패: %w", vhErr)
		}
		nodeVersionHistoryRepo = vhRepo
		defer nodeVersionHistoryRepo.Close()

		// 서버 호스팅 프로그램 이미지(버전 관리 — 서버 호스팅 이미지). 메타데이터는 공유
		// SQLite DB(authDashboardDB)에 releases/release_assets 테이블로 멱등 추가하고,
		// 바이너리 본체와 .sig 는 디스크({releasesDir})에 둔다. releasesDir 은 설정값이
		// 있으면 우선하고, 없으면 {dir(sqlite_path)}/releases 로 유도한다(device_metadata
		// 와 동일 컨벤션). 노드-측 익명 피드 + admin 업로드/관리 핸들러가 이 저장소를 공유한다.
		releasesDir := rmCfg.ReleasesDir
		if releasesDir == "" {
			releasesDir = filepath.Join(filepath.Dir(storageCfg.SQLitePath), "releases")
		}
		releaseRepo, rrErr := storage.NewReleaseRepository(authDashboardDB, releasesDir)
		if rrErr != nil {
			return fmt.Errorf("릴리즈 저장소 초기화 실패: %w", rrErr)
		}
		// 익명 GitHub-호환 피드(노드 Checker/Downloader 가 인증 없이 소비). 다운로드 URL
		// 의 public base 는 설정(public_base_url) 우선, 미설정 시 요청 Host 에서 유도(https 강제).
		releaseFeedHandler = handler.NewReleaseFeedHandler(
			releaseRepo, rmCfg.PublicBaseURL,
			obs.Loggers.NewLogger("api.handler.release_feed").Logger())
		// admin 릴리즈 관리(생성/삭제) + multipart 업로드(바이너리 + Ed25519 .sig). raw
		// 업로드 핸들러는 Auth 미들웨어를 우회하므로 JWTService 를 주입해 admin 을 직접 검증한다.
		releaseAdminHandler = handler.NewReleaseAdminHandler(
			releaseRepo, server.JWTService(),
			obs.Loggers.NewLogger("api.handler.release_admin").Logger())

		// 노드 토큰은 기존 JWTService 를 재사용한다(REQ-C04/C05/C07/F02/F07).
		tokenIssuer := remote.NewJWTTokenIssuer(server.JWTService())

		remoteServer = remote.NewServer(remote.ServerConfig{
			HeartbeatTimeout: 3 * rmCfg.HeartbeatInterval,
			Repo:             managedNodeRepo,
			Mirror:           mirrorRepo,
			TokenIssuer:      tokenIssuer,
			Audit:            remoteAuditRepo,
			Enrollment:       enrollmentTokenRepo,
			VersionHistory:   nodeVersionHistoryRepo,
			BootstrapSecret:  rmCfg.BootstrapSecret,
			Logger:           obs.Loggers.NewLogger("remote.server").Logger(),
		}, nil)

		// SPEC-SUBFLOW-001 v1.3(그룹 RB): 원격 참조 flow-node 는 라이브 브리지로 동작한다.
		// v1.2 의 배포 시 원격 정의 fetch+인라인 확장(SetRemoteFlowFetcher / newRemoteSubflowFetcher)
		// 은 device/secret 무동작 한계로 폐기되었다(§1.2 결정 5). 매니저는 원격 정의를
		// fetch·확장하지 않으며, 원격 참조 flow-node 는 ExpandSubflows 에서 라이브 노드로 남아
		// P3 의 매니저 측 브리지 통합(FlowBridgeOpener 구현 주입)에서 처리된다.

		// P3 라이브 브리지 opener 주입(REQ-SUBFLOW-RB05): server 모드에서만 살아남은 remote://
		// flow-node 가 라이브 브리지로 실행된다. flowSvc.DeployFlow 가 재배선 시 이 opener 로
		// remote.Server 위에 bridge_open 을 전송한다(노드 권위 경계 포트 — RB06). 비-server
		// 모드는 opener 미주입이므로 remote:// flow-node 배포가 명확한 오류로 거부된다.
		flowSvc.SetRemoteBridgeOpener(service.NewServerBridgeOpener(
			remoteServer, obs.Loggers.NewLogger("remote.bridge.opener").Logger()))

		// 관리 WS 핸들러: 노드 토큰 핸드셰이크 검증 활성화(재접속 세션 복원 — REQ-C05).
		remoteWSHandler := handler.NewRemoteHandler(remoteServer,
			obs.Loggers.NewLogger("api.handler.remote").Logger()).
			WithTokenValidator(tokenIssuer)
		server.RegisterRawHandler(handler.RemoteWSPattern, remoteWSHandler.HandleUpgrade)

		// 관리자 승인/거부/폐기/목록 REST API (admin 권한 강제 — REQ-C03/C07/F04).
		// 감사 저장소를 연결해 mutation 을 영속 기록하고 GET /remote/audit 로 관측한다(M6).
		remoteAdminHandler = handler.NewRemoteAdminHandler(remoteServer,
			obs.Loggers.NewLogger("api.handler.remote_admin").Logger()).
			WithAudit(remoteAuditRepo).
			WithSettings(settingsRepo)

		// 스케줄(예약) 실행 로그 조회 API(SPEC-SCHEDULE-VIEW-001 M3, RD-5). remote_admin 의
		// Audit 핸들러를 미러링하되 admin 게이팅 없이 인증된 전체 사용자에게 서비스한다(AC-17).
		// M2 에서 fire(node)/result(xsfm) 기록에 주입한 것과 동일한 저장소 인스턴스를 읽기 측으로
		// 재사용한다. 저장소 미구성(nil)이어도 핸들러는 등록하며 빈 목록을 반환한다(AC-5, audit 준용).
		scheduleLogHandler = handler.NewScheduleLogHandler(scheduleLogRepo)

		// 수동 enrollment 관리자 API(v1.1 그룹 H): 사전 등록 노드 생성/삭제 + enrollment
		// 토큰 발급/목록/폐기. *remote.Server 가 PreRegistrationService 를 만족한다.
		remoteEnrollmentHandler = handler.NewRemoteEnrollmentHandler(
			remoteServer,
			handler.NewEnrollmentTokenService(enrollmentTokenRepo),
			obs.Loggers.NewLogger("api.handler.remote_enrollment").Logger())

		// 원격 자원 편집 API(v1.2 그룹 I, M7): 승인·온라인 노드의 플로우/에이전트 FULL
		// CRUD. 편집은 명령(그룹 D) 전파 후 결과 수신 시에만 미러 캐시를 갱신한다(A4/E08
		// — 서버 단독 영속 금지). *remote.Server 가 RemoteEditService 를 만족한다.
		remoteEditHandler = handler.NewRemoteEditHandler(
			remoteServer,
			obs.Loggers.NewLogger("api.handler.remote_editing").Logger())

		// 원격 READ 프록시 API(v1.3 그룹 J, M8): 승인·온라인 노드의 flow/agent/device
		// 디테일/라이브 READ 를 노드 경유로 프록시한다(READ-ONLY). 노출 위반/오류 접근만
		// 감사하고(REQ-J15), 단기 TTL 캐시로 반복 질의를 흡수한다(REQ-J16). 노출 범위 밖
		// 자원은 404 로 거부한다(REQ-J05). *remote.Server 가 RemoteQueryService 를 만족한다.
		remoteQueryHandler = handler.NewRemoteQueryHandler(
			remoteServer,
			obs.Loggers.NewLogger("api.handler.remote_query").Logger()).
			WithAudit(remoteAuditRepo)

		// 원격 라이브 스트림 SSE 엔드포인트(v1.3 그룹 J, M8): device.state/agent.stats/
		// agent.series 의 서버→브라우저 단방향 스트림. 브라우저↔노드 팬아웃/teardown 은
		// remote.Server 의 streamManager 가 처리한다(REQ-J08/J08b). admin JWT 강제(WS 패턴
		// 준용 — Bearer/?token=). *remote.Server 가 RemoteStreamService 를 만족한다.
		remoteStreamHandler = handler.NewRemoteStreamHandler(remoteServer, server.JWTService()).
			WithLogger(obs.Loggers.NewLogger("api.handler.remote_stream").Logger())

		// 노드 그룹핑 + 노드 상세 API(v1.4 그룹 K, M9): 노드 그룹 배정/해제·distinct 그룹
		// 목록·노드 상세(BASIC 시스템 정보 + uptime + 미러 파생 운영 요약). 그룹은 서버
		// 운영 메타데이터이므로 노드로 명령을 전파하지 않는다(A13). admin 게이팅(REQ-K06/F04).
		// *remote.Server 가 NodeGroupingService 를 만족한다.
		remoteGroupingHandler = handler.NewRemoteGroupingHandler(remoteServer).
			WithSettings(settingsRepo).
			WithReleases(releaseRepo)

		logger.Info("원격 관리 서버 모드 활성화",
			"endpoint", handler.RemoteWSPattern)
	case "client":
		// 클라이언트 dialer 는 ctx 생성 후(아래 10절) 시작한다.
		logger.Info("원격 관리 클라이언트 모드 활성화",
			"server_url", rmCfg.ServerURL)
	default:
		// disabled — 아무 것도 하지 않는다(회귀 0).
	}

	// 9.5c. 관리자 승인 REST API 등록 (@SPEC:SPEC-REMOTE-001 M2).
	// server 모드에서만 등록한다. RegisterRoutes 는 동일 /api/v1 그룹에 추가
	// 등록하므로 위 9.4 의 핸들러들과 공존한다(별도 호출 안전).
	if remoteAdminHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			remoteAdminHandler.RegisterRoutes(g)
		})
	}

	// 9.5c'. 스케줄 로그 조회 API 등록(SPEC-SCHEDULE-VIEW-001 M3, RD-5). server 모드에서만
	// 등록한다. remote_admin 과 동일 /api/v1 인증 그룹에 추가하되 admin 게이팅은 없다(AC-17).
	if scheduleLogHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			scheduleLogHandler.RegisterRoutes(g)
		})
	}

	// 9.5b'. 수동 enrollment 관리자 API 등록(v1.1 그룹 H). server 모드에서만 등록한다.
	if remoteEnrollmentHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			remoteEnrollmentHandler.RegisterRoutes(g)
		})
	}

	// 9.5b''. 원격 자원 편집 API 등록(v1.2 그룹 I, M7). server 모드에서만 등록한다.
	// remote_admin 의 GET 미러 목록 라우트와 동일 경로(POST/PATCH/DELETE)로 공존한다.
	if remoteEditHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			remoteEditHandler.RegisterRoutes(g)
		})
	}

	// 9.5b'''. 원격 READ 프록시 API 등록(v1.3 그룹 J, M8). server 모드에서만 등록한다.
	// 자원-타깃 디테일/라이브 GET 라우트는 미러 목록(GET .../flows 등)보다 path 세그먼트가
	// 길어 충돌하지 않는다(REQ-J01).
	if remoteQueryHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			remoteQueryHandler.RegisterRoutes(g)
		})
	}

	// 9.5b''''. 원격 라이브 스트림 SSE 엔드포인트 등록(v1.3 그룹 J, M8). server 모드에서만
	// 등록한다. SSE 는 http.Flusher 직접 접근이 필요하므로 raw 핸들러로 등록한다(WS 패턴 준용).
	if remoteStreamHandler != nil {
		remoteStreamHandler.RegisterRawHandlers(server.RegisterRawHandler)
	}

	// 9.5b'''''. 노드 그룹핑 + 노드 상세 API 등록(v1.4 그룹 K, M9). server 모드에서만 등록한다.
	// GET /remote/nodes/{instance_id} 는 단일 세그먼트 패턴이므로 remote_admin 의 GET
	// /remote/nodes·/pending(리터럴) 및 .../flows 등(더 긴 path)과 충돌하지 않는다.
	if remoteGroupingHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			remoteGroupingHandler.RegisterRoutes(g)
		})
	}

	// 9.5b''''''. 서버 호스팅 프로그램 이미지(버전 관리 — 서버 호스팅 이미지). server
	// 모드에서만 등록한다. 익명 GitHub-호환 피드 + 다운로드는 raw 핸들러로 등록한다(노드
	// Checker/Downloader 는 토큰을 보내지 않으므로 /api/v1/* Auth 미들웨어를 우회해야 하고,
	// 다운로드는 octet-stream 바이트를 직접 스트리밍한다). admin 관리(GET/POST/DELETE)는
	// RouteGroup(Auth + requireAdmin)으로, multipart 업로드는 raw 핸들러(JWT 직접 검증)로 등록한다.
	if releaseFeedHandler != nil {
		releaseFeedHandler.RegisterRawHandlers(server.RegisterRawHandler)
	}
	if releaseAdminHandler != nil {
		server.RegisterRoutes(func(g *api.RouteGroup) {
			releaseAdminHandler.RegisterRoutes(g)
		})
		releaseAdminHandler.RegisterRawHandlers(server.RegisterRawHandler)
	}

	// 9.5d. 원격 관리 모드 조회 API 등록 (@SPEC:SPEC-REMOTE-001).
	// admin 라우트와 달리 모든 모드(server/client/disabled)에서 무조건 등록한다.
	// 모드 문자열만 필요하므로 remoteServer 가 nil 인 client/disabled 에서도 동작하며,
	// Web UI 가 server 전용 엔드포인트 호출 여부를 사전에 판단할 수 있게 한다.
	remoteModeHandler := handler.NewRemoteModeHandler(rmCfg.Mode)
	server.RegisterRoutes(func(g *api.RouteGroup) {
		remoteModeHandler.RegisterRoutes(g)
	})

	// 9.6. 모니터링 브로드캐스터 (WebSocket 을 통한 실시간 메트릭 전송)
	// M10(그룹 L, REQ-L06): client 모드에서 원격 monitor.logs 스트림이 노드의 로그
	// 파이프라인을 in-process 로 탭하도록, 로그 hub 를 브로드캐스터의 추가 로그 writer 로
	// 합류시킨다(A18 — 자가 WS dial 없음). server/disabled 모드에서는 nil(미합류).
	var remoteLogHub *logStreamHub
	if rmCfg.Mode == "client" {
		remoteLogHub = newLogStreamHub()
	}
	broadcaster := ws.NewMonitoringBroadcaster(wsHub, eng,
		obs.Loggers.NewLogger("api.ws.broadcaster").Logger(),
		ws.WithStreamRouter(obs.Streams),
		ws.WithExtraLogWriter(logHubWriter(remoteLogHub)))

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

	// 디바이스 이력 레코더 시작 (ctx 생성 후). ctx 취소(종료 시그널) 시 수집 고루틴이
	// 정리된다. nil(비활성)이면 Start 는 no-op 이다.
	if deviceHistoryRecorder != nil {
		deviceHistoryRecorder.Start(ctx)
		logger.Info("디바이스 이력 레코더 시작")
	}

	// 10.1. 원격 관리 라이프사이클 시작 (@SPEC:SPEC-REMOTE-001 M1).
	// ctx 취소(종료 시그널) 시 sweeper/client 고루틴이 정리된다.
	switch rmCfg.Mode {
	case "server":
		if remoteServer != nil {
			remoteServer.StartSweeper(ctx)
			logger.Info("원격 관리 online/offline 추적 시작")
		}
	case "client":
		// 영속 instance_id 해석(config override 우선, 없으면 데이터 디렉토리에
		// 생성·영속 — REQ-A03). 데이터 디렉토리는 SQLite 경로의 부모를 재사용한다.
		dataDir := filepath.Dir(storageCfg.SQLitePath)
		instanceID, idErr := remote.ResolveInstanceID(rmCfg.InstanceID, dataDir)
		if idErr != nil {
			logger.Error("instance_id 해석 실패 — 원격 클라이언트 미시작", "error", idErr)
			break
		}
		hostname, _ := os.Hostname()
		// 원격 명령 적용기(M3, REQ-D02/D03/D04): 로컬 API 와 동일한 어댑터 인스턴스를
		// 재사용하여 원격 변경과 로컬 변경이 동일 상태에 반영되도록 한다(A5 — 원격 우회
		// 없음). domain → DomainCommander 라우팅은 remote.Applier 가 담당한다.
		commandApplier := remote.NewApplier(
			&flowCommander{adapter: flowSvc},
			&agentCommander{adapter: agentSvc},
			// executor 는 로컬 DeviceHandler.Execute(POST /devices/{id}/execute)가
			// 호출하는 바로 그 deviceRegistry 인스턴스이다 — 원격 런타임 제어(execute)가
			// 로컬 제어와 동일한 경로/검증/오류 의미를 갖도록 동일 레지스트리를 재사용한다
			// (A5 — 원격 우회 없음, OQ-L4 — 제어 쓰기는 그룹 D 재사용).
			&deviceCommander{registry: deviceRegistry, repo: deviceMetaRepo, executor: deviceRegistry},
		).WithSystem(newSystemCommander(cfg, configFile, obs.Loggers.NewLogger("remote.system_update").Logger()))

		// 인벤토리 소스(M4, REQ-E01): 로컬 API 와 동일한 어댑터 인스턴스를 재사용하여
		// 미러가 로컬 상태와 일치하도록 한다. redaction(F06)은 소스 어댑터가 수행한다.
		inventorySource := newRemoteInventorySource(flowSvc, agentSvc, deviceRegistry)

		// READ/QUERY 프록시 + 스트림 소스(M8, REQ-J01/J08): 로컬 read 핸들러(FlowStatus/
		// ListFlowNodes/AgentStats/device State 등)를 query-action 으로 노출한다(A10 — 노드
		// 권위). redaction(REQ-J06)은 client 가 queryRedactor(secret_fields SoT)로 전송 전
		// 수행한다. 변경은 그룹 D/I 경로 유지(READ-ONLY — REQ-J03).
		//
		// store/series 는 동일한 agentMgr 인스턴스를 재사용해 store/tsdb 시스템 에이전트의
		// 로컬 read 메서드(StaticKeysSnapshot / TSDB().SeriesKeys)를 호출한다(A10 — 로컬
		// API 와 IDENTICAL 형상).
		storeReader := newAgentManagerStoreReader(agentMgr)
		seriesReader := newAgentManagerSeriesReader(agentMgr)
		querySource := newRemoteQuerySource(flowSvc, agentSvc, deviceRegistry, storeReader, seriesReader)
		// M10(그룹 L): 대시보드 config(get_shared/get_mine) + 시스템 메트릭(monitor.metrics)
		// read 소스를 바인딩한다(REQ-L01/L05). 로컬 /dashboards·/monitor/metrics 와 동일
		// 인스턴스를 재사용하여 노드-로컬 권위(A17)·동형 응답을 보장한다. READ-ONLY(REQ-J03).
		querySource.dashboard = dashboardRepo
		querySource.metrics = monitorMgr
		streamSource := newRemoteStreamSource(agentSvc, deviceRegistry, seriesReader, 0)
		// M10(그룹 L): 차트(chart.chart)는 in-process 차트 채널 레지스트리를 직접 탭하고
		// (REQ-L07 — /ws/chart 자가 dial 금지), 로그(monitor.logs)는 위 9.6 의 로그 hub 를
		// 탭한다(REQ-L06). 둘 다 라이브 스트림(캐시 우회 — REQ-J16).
		streamSource.charts = chartChannelRegistry
		streamSource.logs = remoteLogHub
		queryRedactor := newQueryRedactor()

		// 라이브 브리지 실행 어댑터(SPEC-SUBFLOW-001 P2, REQ-SUBFLOW-RB05/RB07): 원격
		// 참조 flow-node 의 bridge_open 수신 시 참조 플로우를 노드에서 실행하고 경계
		// 포트를 tap 한다(입력 주입/출력 중계). 로컬 API 와 동일한 flowSvc/eng 인스턴스를
		// 재사용하여 노드 실행이 로컬 배포와 동일 경로/검증/시크릿/디바이스를 갖게 한다
		// (RC01~RC03 해소). tap 노드 타입은 위 service.RegisterBridgeTapNodes 로 등록됨.
		bridgeRunner := service.NewBridgeFlowRunnerAdapter(flowSvc, eng,
			obs.Loggers.NewLogger("remote.bridge").Logger())

		// 노드 측 tap 소스 주입(참조 플로우 재시작 투명성): flowSvc.DeployFlow 가 활성 노드
		// 측 브리지 tap 컨트롤러가 있는 참조 플로우를 동일 tapID 로 경계 재배선하여 배포하도록
		// 한다. 이로써 사용자가 노드에서 참조 플로우를 재시작(Stop→Undeploy→Deploy→Start)해도
		// 매니저 측 브리지가 끊기지 않고 출력/입력이 재시작 너머로 보존된다(매니저 측
		// SetRemoteBridgeOpener 와 대칭 와이어링). client 모드에서만 설정된다.
		flowSvc.SetBridgeTapSource(bridgeRunner)

		// 노드 토큰은 instance_id 와 동일 데이터 디렉토리에 영속한다(REQ-C04/C05).
		// Exposure 요약은 register 에 운반되고, 미러 송신 시 노출 필터로 평가된다(REQ-A04/E07).
		// 관리 WS 다이얼러: insecure_skip_verify 면 자체 서명 인증서/사설망용으로 TLS
		// 인증서 검증을 건너뛰는 다이얼러를 쓴다(nil → 기본 보안 다이얼러).
		var clientDialer remote.Dialer
		if rmCfg.InsecureSkipVerify {
			clientDialer = remote.NewGorillaDialerInsecure()
			logger.Warn("원격 client TLS 인증서 검증 건너뜀(remote_management.insecure_skip_verify) — 자체 서명/사설망 전용")
		}
		remoteClient := remote.NewClient(remote.ClientConfig{
			ServerURL:  rmCfg.ServerURL,
			InstanceID: instanceID,
			Hostname:   hostname,
			Version:    Version,
			// BASIC 시스템 정보(v1.4 M9, REQ-K07): runtime.GOOS/GOARCH + 데몬 시작 시각.
			// register/heartbeat 로 보고되어 서버가 저장·uptime 파생한다(자원 메트릭 제외).
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			StartedAt: daemonStartedAtMs,
			// 노드 장비 모니터 해상도(v1.6 M11, REQ-M01): config(display.resolution 또는
			// width+height)에서 파싱된 값. 헤드리스 데몬이라 운영자 선언이 1차 출처(A20).
			// 0(미설정)은 미보고이며 관리자 뷰가 폴백한다(REQ-M03).
			DisplayWidth:      rmCfg.Display.Width,
			DisplayHeight:     rmCfg.Display.Height,
			HeartbeatInterval: rmCfg.HeartbeatInterval,
			BootstrapSecret:   rmCfg.BootstrapSecret,
			EnrollmentToken:   rmCfg.EnrollmentToken,
			DataDir:           dataDir,
			Exposure: remote.ExposureSummary{
				Flows:   rmCfg.Exposure.Flows,
				Agents:  rmCfg.Exposure.Agents,
				Devices: rmCfg.Exposure.Devices,
			},
			Applier:       commandApplier,
			Inventory:     inventorySource,
			QuerySource:   querySource,
			StreamSource:  streamSource,
			QueryRedactor: queryRedactor,
			// 라이브 브리지 실행기(P2): bridge_open 시 참조 플로우 실행 + 경계 tap.
			// BridgeAudit 은 nil(구조화 로그만 — 노드-로컬 감사 저장소 미사용). client 가
			// open/close/input 을 시크릿 페이로드 제외로 로깅한다(REQ-SUBFLOW-RB06/RB11).
			BridgeRunner: bridgeRunner,
			Logger:       obs.Loggers.NewLogger("remote.client").Logger(),
		}, clientDialer)
		remoteClient.Start(ctx)
		defer remoteClient.Stop()

		// 노출 설정 핫리로드(A07/A06): exposure 키 변경 시 새 범위로 재미러링한다.
		// 노출 해제된 자원은 remove 델타로 서버 캐시에서 제거된다(client_mirror.go).
		for _, key := range []string{
			"remote_management.exposure.flows",
			"remote_management.exposure.agents",
			"remote_management.exposure.devices",
		} {
			cfg.OnChange(key, func(_ config.ChangeEvent) {
				rm := cfg.RemoteManagement()
				remoteClient.UpdateExposure(remote.ExposureSummary{
					Flows:   rm.Exposure.Flows,
					Agents:  rm.Exposure.Agents,
					Devices: rm.Exposure.Devices,
				})
				logger.Info("노출 설정 변경 — 재미러링 신호", "instance_id", instanceID)
			})
		}

		logger.Info("원격 관리 클라이언트 시작",
			"instance_id", instanceID, "server_url", rmCfg.ServerURL)
	}

	// 10.2. 자동 시작 플로우 복원 (원격 관리 와이어링 완료 후).
	// 9.5b(SetRemoteBridgeOpener — server) 및 10.1 client(SetBridgeTapSource) 의 두 모드
	// switch 가 모두 완료된 뒤 실행한다. 이로써 remote:// flow-node 를 가진 플로우의 배포
	// (재배선)가 opener(server) / tap source(client) 주입 이후에 일어나 ErrRemoteBridge
	// Unavailable 없이 성공한다(부팅 auto-start 회귀 수정). server.Start(ctx) 직전에 두어
	// 노드의 WS dial-in 보다 먼저 매니저 측 브리지 컨트롤러를 오프라인 시작시킨다(노드 도착
	// 시 자동 연결 — remote_bridge_node.go 의 offline-at-boot 허용과 짝).
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

	// 원격 자가 업데이트 후 부팅이면(update-state 파일의 pending), 서버가 뜨는 동안 로컬
	// /health 를 폴링해 자가 검증하고 실패/반복크래시 시 자동 롤백한다(버전 관리 Phase 2).
	// 상태 파일이 없는 일반 부팅은 즉시 no-op 이므로 항상 호출해도 안전하다.
	go runPostUpdateSelfCheck(ctx, serverCfg.Port, logger.Logger())

	// server.Start 는 ctx 취소 시 자동으로 Stop 호출
	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("서버 실행 실패: %w", err)
	}

	// 정리: 디바이스 이력 레코더 수집 고루틴 종료 대기 (ctx 취소로 이미 정리 시작됨).
	if deviceHistoryRecorder != nil {
		deviceHistoryRecorder.Wait()
		logger.Info("디바이스 이력 레코더 종료 완료")
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
// @SPEC:SPEC-WEB-007
// startedAt 은 프로세스 부팅 시각이다. self uptime 계산의 기준 시각으로 svcCfg 에 전달된다.
func buildSystemHandler(cfg config.Config, obs *observe.Observer, startedAt time.Time) *handler.SystemHandler {
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
		// @SPEC:SPEC-WEB-007 — self identity + uptime.
		Mode:      cfg.RemoteManagement().Mode,
		StartedAt: startedAt,
	}
	svc := handler.NewUpdateService(svcCfg, logger)
	return handler.NewSystemHandler(svc, logger)
}
