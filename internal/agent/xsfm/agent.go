package xsfm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// agentType 는 이 에이전트의 타입 식별자이다.
const agentType = "xsfm"

// msgChannelSize 는 노드 방출용 msgCh 버퍼 크기이다.
const msgChannelSize = 256

// XSFMAgent 는 지하철 역사 설비 관리 에이전트이다 (REQ-XSFM-001-01-01).
//
// thingplus MQTT 트랜스포트 셸 + samsung 로스터/관측 상태 모델을 결합한다. 에이전트는
// 순수 프로토콜/로직 레이어이며, I/O 경계(상태 유입 · 명령 방출)를 인터페이스로 추상화하여
// direct(브로커 소유)·port(외부 노드 I/O) 두 모드를 주입한다 (REQ-XSFM-001-01-11).
type XSFMAgent struct {
	*lifecycle.BaseLifecycle

	agentConfig agent.AgentConfig
	cfg         XSFMConfig

	// I/O 경계 (모드에 따라 주입). client 는 port 모드에서 nil.
	client  MQTTClient
	cmdSink CommandSink
	ctrlCh  chan ControlMessage // port 모드 제어 출력 포트 (direct 모드는 nil)

	// devices 는 로스터 PRIMARY 인덱스(device_id → Device)이다. 합성 주소 모델에서 device_id 는
	// 생성된 UUID 이고, blob/{device_id} 모델에서는 사용자/토픽이 부여한 식별자 그대로이다.
	devices map[string]*Device

	// secondary 는 위치 계층 합성 주소 → device_id 보조 인덱스(compositeKey → device_id)이다.
	// 유입 상태 토픽이 나르는 (station,place,index) 를 정규화 키로 조회해 대상 device_id(UUID)를
	// 찾는다. 로스터 상태의 일부이므로 devices 와 동일한 mu 로 보호하며, 로스터 add/remove/update
	// 마다 함께 갱신한다(pending/registry 락과 절대 중첩하지 않는다).
	secondary map[string]string

	mu sync.RWMutex

	// pendings 는 제어 응답 대기 레지스트리이다 (B3). 로스터 락(mu)과 분리된 자체 락으로
	// 보호되며 절대 중첩하지 않는다 (RWMutex 재진입 트랩 회피).
	pendings *pendingRegistry

	// stations 는 역사(station)→호선(line) 레지스트리이다 (B6, REQ-XSFM-001-02-11).
	// 자체 RWMutex 로 보호되며 로스터 락(mu)·pending 락과 절대 중첩하지 않는다.
	stations *StationRegistry

	// groups 는 그룹(1급 개념) 레지스트리이다 (SPEC-XSFM-GROUP-001, 신설 비침습 레이어).
	// 커스텀 그룹 멤버십의 SSOT 이며 자체 RWMutex 로 보호된다 — 로스터 락(mu)·pending
	// 락·station 레지스트리 락과 절대 중첩하지 않는다(REQ-01-07 락 규율). 기본 그룹
	// (station/line)은 저장하지 않고 station 레지스트리/로스터에서 파생한다.
	groups *GroupRegistry

	// lines 는 라인(1급 엔티티) 레지스트리이다 (SPEC-XSFM-LINE-001, 신설 비침습 레이어).
	// 라인 엔티티의 코드/표시명/정렬 SSOT 이며 자체 RWMutex 로 보호된다 — 로스터 락(mu)·
	// pending 락·station 레지스트리 락·group 레지스트리 락과 절대 중첩하지 않는다(REQ-07-01
	// 락 규율). line:<code> 멤버는 저장하지 않고 DevicesByLine 로 파생한다(REQ-01-08).
	lines *LineRegistry

	// registry 는 런타임 등록 디바이스(bridge/auto) 로스터의 파일 기반 영속 저장소이다
	// (B7, REQ-XSFM-001-02-05/08). registry_path 가 빈 값이면 nil(영속화 비활성). 자체
	// 락을 보유하며 로스터 락(mu)에 걸쳐 잡지 않는다(스냅샷 후 해제, 그다음 파일 I/O).
	registry *deviceRegistryStore

	msgCh  chan []byte
	stopCh chan struct{}

	// monitorWG 는 오프라인 감지 모니터 고루틴의 생명주기를 추적한다 (B5). Stop 이
	// stopCh 를 닫은 뒤 Wait 하여 고루틴 누수/타이머 누수를 방지한다.
	monitorWG sync.WaitGroup

	// deviceIDRepo 는 device_id 저장소 참조 슬롯이다 (후속 배치에서 배선).
	deviceIDRepo agent.DeviceIDRepository

	stats     *agent.AgentStats
	logger    *slog.Logger
	startedAt time.Time
	createdAt time.Time
}

// 컴파일 타임 인터페이스 체크 (REQ-XSFM-001-01-02).
var (
	_ agent.Agent           = (*XSFMAgent)(nil)
	_ agent.MessageReceiver = (*XSFMAgent)(nil)
)

// DeviceProvider 는 이 에이전트의 디바이스를 device.DeviceProvider 로 노출한다
// (samsung.Hvacr01Agent.DeviceProvider 패턴 미러). 에이전트 매니저의 OnStart 훅이
// 이 표면을 감지해 디바이스 레지스트리에 프로바이더를 자동 등록한다 (REQ-XSFM-001-08).
func (a *XSFMAgent) DeviceProvider() device.DeviceProvider {
	return NewXSFMDeviceProvider(a)
}

// processRequest 는 Process 의 JSON 요청 구조체이다 (제어 명령 디스패치).
type processRequest struct {
	Command  string `json:"command"`
	DeviceID string `json:"device_id,omitempty"`
	Name     string `json:"name,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
	Station  string `json:"station,omitempty"`
	Place    string `json:"place,omitempty"`
	Index    int    `json:"index,omitempty"`
	Line     string `json:"line,omitempty"`

	// 이름 기반 제어 셀렉터 (SPEC-XSFM-NAMESEL-001 RD-1). CRUD Name(json `name`) 필드와
	// 별개이며 제어 대상 지정에만 쓰인다: DeviceName 은 개별 이름 셀렉터(→ device_id 해소 후
	// 개별 제어), GroupName 은 그룹 이름 셀렉터(→ 그룹 해소 후 fan-out)이다.
	DeviceName string `json:"device_name,omitempty"`
	GroupName  string `json:"group_name,omitempty"`

	// 역사 레지스트리 CRUD 필드 (B6, add_station: display_name/order).
	DisplayName string `json:"display_name,omitempty"`
	Order       int    `json:"order,omitempty"`

	// StationNumber 는 역번호(실제 역사 대외 표시 번호, 선택)이다. add_station 에서 역사 코드와
	// 별개로 지정하며, 지정 시 디바이스 자동 이름의 역사 세그먼트에 우선 반영된다.
	StationNumber string `json:"station_number,omitempty"`

	// Code 는 라인/커스텀 그룹의 통일 코드 식별자이다 (SPEC-XSFM-LINE-001 RD-2/RD-6).
	// add_line{code} 및 add_group{code} 에서 사용한다.
	Code string `json:"code,omitempty"`

	Params map[string]any `json:"params,omitempty"`
	NodeID string         `json:"node_id,omitempty"`
	FlowID string         `json:"flow_id,omitempty"`
}

// fillFromParams 는 top-level 주소지정(addressing) 필드가 zero 값일 때 params 에서 backfill 한다.
//
// HTTP exec 엔드포인트(POST /agents/{id}/exec)는 요청 본문을 표준 {command, params} 계약으로
// 재직렬화하므로 top-level 주소지정 필드(device_id/station/place/index 등)가 소실된다. 이 헬퍼는
// params 에 담긴 주소지정 값을 구조체 필드로 승격해 표준 exec 계약을 지원한다. top-level 이 이미
// 지정된 경우 그대로 두어(top-level 우선) 기존 노드·유닛테스트 경로를 정확히 보존한다.
//
// 제어 값 params(power/fan_speed)는 주소지정이 아니므로 여기서 다루지 않으며 개별 제어 핸들러가
// 계속 req.Params 에서 직접 읽는다.
func (req *processRequest) fillFromParams() {
	if req.Params == nil {
		return
	}
	// 문자열 주소지정 필드: top-level 이 빈 값일 때만 params 에서 채운다. stringField 는
	// 누락 키에 대해 "" 를 반환하므로 zero 값이 그대로 유지된다.
	if req.DeviceID == "" {
		req.DeviceID = stringField(req.Params, "device_id")
	}
	if req.Name == "" {
		req.Name = stringField(req.Params, "name")
	}
	if req.GroupID == "" {
		req.GroupID = stringField(req.Params, "group_id")
	}
	// 이름 셀렉터 승격 (SPEC-XSFM-NAMESEL-001 RD-1, group_id 승격 패턴 계승): HTTP exec 계약이
	// device_name/group_name 을 params 로 나른 경우 top-level 로 끌어올린다(top-level 우선).
	if req.DeviceName == "" {
		req.DeviceName = stringField(req.Params, "device_name")
	}
	if req.GroupName == "" {
		req.GroupName = stringField(req.Params, "group_name")
	}
	if req.Station == "" {
		req.Station = stringField(req.Params, "station")
	}
	if req.Place == "" {
		req.Place = stringField(req.Params, "place")
	}
	if req.Line == "" {
		req.Line = stringField(req.Params, "line")
	}
	if req.DisplayName == "" {
		req.DisplayName = stringField(req.Params, "display_name")
	}
	if req.StationNumber == "" {
		req.StationNumber = stringField(req.Params, "station_number")
	}
	if req.Code == "" {
		req.Code = stringField(req.Params, "code")
	}
	// 정수 주소지정 필드: JSON 숫자는 float64, 숫자 문자열("3")도 허용(toInt). 누락 키는
	// firstPresent 가 nil 을 반환하고 toInt(nil)==0 이므로 zero 값이 유지된다.
	if req.Index == 0 {
		req.Index = toInt(firstPresent(req.Params, "index"))
	}
	if req.Order == 0 {
		req.Order = toInt(firstPresent(req.Params, "order"))
	}
}

// NewXSFMAgent 는 XSFMAgent 팩토리 함수이다 (agent.Agent 반환).
func NewXSFMAgent(config agent.AgentConfig) (agent.Agent, error) {
	cfg, err := parseXSFMConfig(config.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("xsfm agent: %w", err)
	}

	a := &XSFMAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName(agentType)),
		cfg:           cfg,
		devices:       make(map[string]*Device),
		secondary:     make(map[string]string),
		pendings:      newPendingRegistry(),
		msgCh:         make(chan []byte, msgChannelSize),
		stopCh:        make(chan struct{}),
		deviceIDRepo:  agent.GetDeviceIDRepository(),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// client_id 자동 생성 (direct 모드, 인스턴스별 고유성 — thingplus 패턴).
	if a.cfg.TransportMode == transportModeDirect && a.cfg.ClientID == "" {
		a.cfg.ClientID = "xflow-xsfm-" + uuid.NewString()
	}

	// I/O 경계 배선 (REQ-XSFM-001-01-07 step 3).
	switch a.cfg.TransportMode {
	case transportModeDirect:
		a.client = newPahoMQTTClient(a.cfg, a.logger)
		a.cmdSink = &brokerCommandSink{
			client:    a.client,
			topicTmpl: a.cfg.CommandTopicTemplate,
			qos:       a.cfg.QoS,
		}
	case transportModePort:
		a.client = nil
		a.ctrlCh = make(chan ControlMessage, controlPortBuffer)
		a.cmdSink = &portCommandSink{ch: a.ctrlCh, logger: a.logger}
	}

	// 설정 기반 디바이스 등록 (Source="config", Online=false — REQ-XSFM-001-02-03).
	// 로스터 키는 상태 템플릿의 placeholder 구성으로 합성한다: {device_id} 단일 필드는
	// device_id 자체, 다중 필드는 "{station_code}:{place_code}:{device_index}" (M14).
	for _, cd := range a.cfg.Devices {
		dev := &Device{
			DeviceID: cd.DeviceID,
			Name:     cd.Name,
			GroupID:  cd.GroupID,
			Station:  cd.Station,
			Place:    cd.Place,
			Index:    cd.Index,
			Online:   false,
			Source:   "config",
		}
		key, addr := a.seedKeyAndAddress(dev)
		dev.DeviceID = key
		dev.Address = addr
		if _, exists := a.devices[key]; exists {
			continue
		}
		// 합성 주소 모델의 config 디바이스: Name 미지정 시 합성 규칙으로 채우고 composite 로 표시한다.
		// 보조 인덱스에도 등록하여, 이 위치로 유입되는 상태가 새 디바이스를 auto 생성하지 않고 이
		// config 디바이스에 매칭되도록 한다(중복 방지). config 디바이스의 device_id 는 안정성을 위해
		// 합성 키를 그대로 유지한다(재시작마다 바뀌는 UUID 대신 — config 는 영속화 대상이 아님).
		if a.cfg.stateIsComposite && hasComposite(dev) {
			dev.composite = true
			if dev.Name == "" {
				// 구성 시점(Init 이전)에는 station 레지스트리가 아직 없어 라인·역번호가 미해석된다 →
				// resolveLineFor 는 "" 를(3-세그먼트, RD-4), stationDisplayFor 는 코드 폴백을 반환한다.
				dev.Name = composeName(a.resolveLineFor(dev.Station), a.stationDisplayFor(dev.Station), dev.Place, dev.Index)
			}
		}
		a.devices[key] = dev
		a.indexDeviceLocked(dev)
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}
	return a, nil
}

// Init 은 에이전트를 초기화하고 상태를 Running 으로 전이한다 (REQ-XSFM-001-01-07).
func (a *XSFMAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}
	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 영속 경로 해석 (자동 기본값): 설정 경로가 우선하고, 비어 있으면 서버 기본 디렉터리
	// (<dataDir>/xsfm/<agentID>/) 를 유도한다. 우선순위는 설정 경로 > 기본 경로 >
	// 인메모리 이며, 기본 디렉터리 미설정(단위 테스트 등)이면 종전 동작(빈 설정 → 인메모리)이
	// 그대로 유지된다. station_registry.json / device_registry.json 은 파일명이 달라 같은
	// 디렉터리를 공유해도 충돌하지 않는다. <agentID> 는 에이전트 고유 식별자(이름은 중복 가능
	// 하므로 안정 유일 키인 ID 를 사용)로 다중 설비 에이전트를 격리한다.
	stationPath := a.cfg.StationRegistryPath
	rosterPath := a.cfg.RegistryPath
	if base := getDefaultRegistryDir(); base != "" {
		perAgentDir := filepath.Join(base, "xsfm", config.ID)
		if stationPath == "" {
			stationPath = perAgentDir
		}
		if rosterPath == "" {
			rosterPath = perAgentDir
		}
	}

	// 역사 레지스트리 구성 (B6, REQ-XSFM-001-02-11): 해석된 station 경로의 영속 항목 +
	// station_registry 설정 시드 병합. 경로가 비면 인메모리/시드 전용. 자체 락 보유.
	stations, err := newStationRegistry(stationPath, a.cfg.StationRegistry)
	if err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}
	a.mu.Lock()
	a.stations = stations
	a.mu.Unlock()

	// 로스터 영속 저장소 구성 + 복원 (B7, REQ-XSFM-001-02-05/08): 해석된 roster 경로가
	// 있으면 저장소를 열고 런타임 등록 디바이스를 로스터에 복원한다. 설정 디바이스는 이미
	// NewXSFMAgent 에서 선등록됐으므로 device_id 충돌 시 설정이 우선한다(덮어쓰지 않음).
	if rosterPath != "" {
		store, err := newDeviceRegistryStore(rosterPath)
		if err != nil {
			return fmt.Errorf("xsfm init: %w", err)
		}
		a.mu.Lock()
		a.registry = store
		a.mu.Unlock()
		if err := a.loadPersistedRoster(); err != nil {
			return fmt.Errorf("xsfm init: restore roster: %w", err)
		}
	}

	// 그룹 레지스트리 구성 (SPEC-XSFM-GROUP-001, 신설 레이어): device_registry.json 과 동일
	// dir(rosterPath)에 group_registry.json 으로 커스텀 그룹을 영속한다. rosterPath 가 비면
	// 인메모리 전용(종전 동작 보존). 구성 후 기존 group_id 를 커스텀 그룹으로 마이그레이션한다
	// (REQ-02-04) — Device.GroupID 는 primary 로 유지되어 단일 태그 방출은 무회귀(RD-1).
	groups, err := newGroupRegistry(rosterPath)
	if err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}
	a.mu.Lock()
	a.groups = groups
	a.mu.Unlock()
	a.migrateGroupMembership()

	// 라인 레지스트리 구성 (SPEC-XSFM-LINE-001, 신설 레이어): device_registry.json 과 동일
	// dir(rosterPath)에 line_registry.json 으로 라인 엔티티를 영속한다. rosterPath 가 비면
	// 인메모리 전용(종전 동작 보존). 구성 후 로드 1회성·비파괴 마이그레이션을 수행한다:
	// (1) 기존 station.Line 값 → Line 엔티티 ensure-create(REQ-05-01), (2) 레거시
	// custom:<name> 그룹 코드 → custom:<code> slugify 승격(REQ-05-02, §4.6).
	lines, err := newLineRegistry(rosterPath)
	if err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}
	a.mu.Lock()
	a.lines = lines
	a.mu.Unlock()
	a.migrateLinesFromStations() // REQ-05-01: station.Line → Line 엔티티 ensure-create.
	if a.groups != nil {
		a.groups.migrateCodes() // REQ-05-02/§4.6: custom:<name> → custom:<code> slugify 승격.
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("xsfm init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	// 통계 시작 시각 기록 (LoadTime = 시작→첫 메시지 산출 기준).
	a.stats.SetStartedAt(time.Now())

	a.logger.Info("xsfm: agent initialized",
		"transport_mode", a.cfg.TransportMode,
		"devices", len(a.devices),
		"mqtt_client", a.client != nil,
	)
	return nil
}

// Start 는 direct 모드에서 브로커에 연결하고 모든 디바이스 state 토픽을 구독한다.
// port 모드는 브로커 연결 없이 Running 을 유지한다 (REQ-XSFM-001-01-08).
//
// 오프라인 감지 모니터(offline_timeout 경과 기반)는 양 모드에서 동작하므로 트랜스포트
// 분기 이전에 시작한다 (REQ-XSFM-001-05-03). LWT 경로는 direct 전용이며 별도 훅
// (handleLWTOffline)으로 노출된다.
func (a *XSFMAgent) Start(_ context.Context) error {
	// 오프라인 감지 모니터 (양 모드 공통, offline_timeout>0 일 때만 기동).
	a.startOfflineMonitor()
	// 주기 상태 방출기 (SPEC-XSFM-AGENT-IO-001, state_emit_mode ∈ {interval, both} 일 때만 기동).
	a.startStateEmitter()

	if a.cfg.TransportMode == transportModePort {
		a.logger.Info("xsfm: started in port mode (no broker)")
		return nil
	}

	// direct 모드: 초기 연결 실패는 치명적으로 보지 않는다 (auto-reconnect 시 재시도).
	if a.cfg.LogMQTT {
		a.logger.Info("xsfm mqtt: connecting", "broker", a.cfg.Broker)
	}
	if err := a.client.Connect(); err != nil {
		a.logger.Warn("xsfm: broker connect failed at start", "error", err)
	}

	// 상태 템플릿의 모든 placeholder 를 "+" 로 치환한 단일 와일드카드 토픽을 구독한다 (M14).
	// 디바이스별 렌더 구독을 대체하며, 콜백이 실제 토픽을 파싱해 대상 디바이스를 도출한다.
	if err := a.subscribeState(); err != nil {
		a.logger.Error("xsfm: subscribe state topic failed", "error", err)
	}

	a.logger.Info("xsfm: started in direct mode",
		"subscription", buildSubscriptionTopic(a.cfg.StateTopicTemplate))
	return nil
}

// subscribeState 는 상태 템플릿의 와일드카드 토픽 하나를 구독한다 (direct 모드, M14).
// 콜백 handleStateMessage 가 실제 토픽을 파싱해 주소/attribute 를 추출한다.
func (a *XSFMAgent) subscribeState() error {
	if a.client == nil {
		return nil
	}
	topic := buildSubscriptionTopic(a.cfg.StateTopicTemplate)
	return a.client.Subscribe(topic, a.cfg.QoS, a.handleStateMessage)
}

// handleStateMessage 는 와일드카드 구독으로 유입된 (실제 토픽, 페이로드)를 처리한다 (M14).
// 토픽을 상태 템플릿과 대조해 placeholder 를 추출하고, {attribute} 존재 여부로 디코드 모드를
// 디스패치한 뒤 ingestState 로 로스터를 갱신한다. 우리 소유가 아닌 토픽(리터럴 불일치)은 무시한다.
func (a *XSFMAgent) handleStateMessage(topic string, payload []byte) {
	fields, ok := parseTopic(a.cfg.StateTopicTemplate, topic)
	if !ok {
		return // 리터럴 불일치 / 세그먼트 수 불일치 → 우리 소유 아님
	}

	// 외부 수신 경계: 우리 소유 토픽이 확정된 지점에서 수신/바이트/활동/로드시각을 계측한다.
	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(len(payload)))
	a.stats.UpdateLastActivity()
	a.stats.RecordFirstMessage()

	var st decodedState
	if a.cfg.stateHasAttribute {
		attribute := fields[placeholderAttribute]
		var okDec bool
		st, okDec = a.cfg.PayloadMapping.decodeAttributeScalar(attribute, payload)
		if !okDec {
			a.stats.IncrExternalMessagesErrored()
			a.logger.Warn("xsfm: unknown attribute in state topic", "topic", topic, "attribute", attribute)
			return
		}
	} else {
		var err error
		st, err = decodeStatePayload(a.cfg.PayloadMapping, payload)
		if err != nil {
			a.stats.IncrExternalMessagesErrored()
			a.logger.Warn("xsfm: decode state payload failed", "topic", topic, "error", err)
			return
		}
	}
	if a.cfg.LogMessages {
		a.logFrame("RX", topic, payload, stateSummary(st))
	}
	a.ingestState(a.cfg.stateIsComposite, fields, st)
}

// ingestState 는 디코딩된 상태를 로스터에 반영하고 상태 변경을 방출하는 mode-agnostic 시임이다
// (REQ-XSFM-001-05-01/05-02, REQ-06-02). direct(토픽 파싱)·port(FeedState) 경로가 공유한다.
//
// 미등록 디바이스는 Source="auto" 로 자동 등록한다(config/bridge 우선순위 유지 — 이미 존재하면
// 갱신). 락 하에서 이전/신규 값을 비교해 changed_fields 를 산출하고 방출 스냅샷을 뜬 뒤 락을
// 해제하고 방출한다 — 락을 채널 송신에 걸쳐 잡지 않는다. fields 의 placeholder 는 device_state_changed
// 메타로 실려 하류 influx 태그(station_code/place_code/device_index/attribute)를 형성한다.
func (a *XSFMAgent) ingestState(composite bool, fields map[string]string, st decodedState) {
	// composite auto-생성 시 새 디바이스 이름의 라인·역사 세그먼트를 로스터 락 취득 전에 해석한다
	// (레지스트리 간 락 중첩 금지, REQ-07-01). 비-composite 경로는 이름 합성이 없어 불필요하다.
	// stationHint 는 역번호(있으면)-또는-코드 표시 값이다.
	lineHint := ""
	stationHint := ""
	if composite {
		lineHint = a.resolveLineFor(fields[placeholderStationCode])
		stationHint = a.stationDisplayFor(fields[placeholderStationCode])
	}

	a.mu.Lock()
	deviceID, dev, created := a.resolveDeviceLocked(composite, fields, lineHint, stationHint)

	prevOnline := dev.Online
	var changed []string

	// 각 축은 이전에 미관측이었거나(최초 관측) 값이 달라지면 changed 로 본다.
	if st.PowerSet {
		if !dev.isObserved(observedPower) || dev.Power != st.Power {
			changed = append(changed, "power")
		}
		dev.Power = st.Power
		dev.markObserved(observedPower)
	}
	if st.FanSpeedSet {
		if !dev.isObserved(observedFanSpeed) || dev.FanSpeed != st.FanSpeed {
			changed = append(changed, "fan_speed")
		}
		dev.FanSpeed = st.FanSpeed
		dev.markObserved(observedFanSpeed)
	}

	// online 축: 페이로드에 online 필드가 있으면 그 값을, 없으면 상태 유입 자체를 생존 신호로
	// 보아 암묵적으로 online=true 로 복원한다 (REQ-05-05). 명시적 online=false 는 존중한다.
	newOnline := true
	if st.OnlineSet {
		newOnline = st.Online
		dev.markObserved(observedOnline)
	}
	dev.Online = newOnline
	if prevOnline != newOnline {
		changed = append(changed, "online")
	}

	// LastSeen 설정(생존 판정 소스). 기본(liveness_source="receive")은 에이전트 수신 시각이다.
	// liveness_source="payload" 이고 디바이스 보고 시각(time_field 엔벨로프)이 추출됐으면 그 디바이스
	// 시각을 LastSeen 으로 쓴다 — offline 판정이 디바이스 보고 시각 기준이 된다. 이곳이 LastSeen 이
	// 디바이스 시각이 될 수 있는 유일한 지점이며, 방출 메시지 timestamp(emitTsMs, 아래)는 이 옵션과
	// 무관하게 time_field 설정 시 항상 디바이스 시각을 쓴다(불변).
	//
	// 주의(스큐 경고): payload 모드에서는 디바이스 시계 오차가 offline 판정에 직접 영향을 준다 —
	// 디바이스 시계가 뒤처지면 살아있어도 stale 로 오판할 수 있다(기본 receive 는 스큐에 영향 없음).
	dev.LastSeen = time.Now()
	if a.cfg.LivenessSource == livenessSourcePayload && st.TimestampSet {
		dev.LastSeen = time.UnixMilli(st.TimestampMs)
	}

	stateAxes := dev.StateForJSON()
	groupID := dev.GroupID
	lastSeenMs := dev.LastSeen.UnixMilli()
	a.mu.Unlock()

	// 자동 등록 시 로스터가 변경됐으므로 best-effort 영속화 (registry 미설정이면 no-op).
	if created {
		a.persistRoster()
	}

	// B3: 로스터 갱신 후 제어 응답 대기 에코 해소. 로스터 락 해제 후 별도 pending 락 하에서 처리.
	// pending 은 controlDevice 가 로스터 device_id(UUID)로 등록하므로 동일 키(deviceID)로 해소한다.
	a.pendings.resolve(deviceID, st)

	if !prevOnline && newOnline {
		a.emitOnlineTransition("device_online", deviceID, groupID, true, lastSeenMs)
	} else if prevOnline && !newOnline {
		a.emitOnlineTransition("device_offline", deviceID, groupID, false, lastSeenMs)
	}

	// 방출 타임스탬프: time_field 로 디바이스 보고 시각이 추출됐으면 그 값을(타임시리즈 데이터포인트
	// 시각), 아니면 수신 시각(lastSeenMs)으로 폴백한다. LastSeen 자체는 항상 수신 시각을 유지하므로
	// offline 판정(생존)은 디바이스 시각과 무관하게 신뢰성을 보존한다.
	emitTsMs := lastSeenMs
	if st.TimestampSet {
		emitTsMs = st.TimestampMs
	}

	// on-change 방출을 상태 방출 모드로 게이팅한다 (SPEC-XSFM-AGENT-IO-001 RD-2, §4.4).
	// event/both: 현행대로 변경 시 device_state_changed 방출. interval: 억제(주기 스냅샷이 대신함).
	// 전이 이벤트(device_online/offline, 위)는 방출 모드와 무관하게 항상 방출된다(범위 밖, 게이팅 제외).
	if len(changed) > 0 && (a.cfg.StateEmitMode == stateEmitModeEvent || a.cfg.StateEmitMode == stateEmitModeBoth) {
		a.emitStateChanged(deviceID, groupID, newOnline, changed, stateAxes, emitTsMs, fields)
	}

	// 수신 파싱-상태 forward 탭 (SPEC-XSFM-AGENT-IO-001 RD-1/RD-4). forward_received_to_node 가 ON 이면
	// 변경 여부(len(changed))·방출 모드와 무관하게 매 수신마다 device_state_received 를 방출한다.
	// ingestState 는 direct·port 공유 시임이므로 이 단일 지점 배치로 양 모드에 mode-agnostic 하게 적용된다.
	if a.cfg.ForwardReceivedToNode {
		a.emitStateReceived(deviceID, groupID, newOnline, stateAxes, emitTsMs, fields)
	}
}

// resolveDeviceLocked 는 유입 상태의 대상 로스터 디바이스를 조회하고, 미등록이면 auto 등록한다.
// 로스터 락을 보유한 채 호출한다. 반환: (로스터 device_id, *Device, 신규 생성 여부).
//
//   - composite(합성 주소 모델): 파싱된 (station,place,index) 를 정규화 compositeKey 로 만들어
//     보조 인덱스에서 device_id(UUID)를 찾는다. 없으면 UUID 를 생성해 새 디바이스를 만들고
//     Name 을 합성한 뒤 로스터 + 보조 인덱스에 등록한다(Source="auto"). 이 경로가 M14 유입의
//     핵심 변경점이다 — 조회 키가 "합성 주소 = device_id" 에서 "보조 인덱스 → UUID" 로 바뀐다.
//   - !composite(blob/{device_id} 모델): topic 의 device_id 를 로스터 키로 직접 조회한다.
//     미등록이면 그 device_id 를 키로 auto 생성한다(하위호환 — 기존 동작 보존).
func (a *XSFMAgent) resolveDeviceLocked(composite bool, fields map[string]string, lineHint, stationHint string) (string, *Device, bool) {
	if composite {
		ck := compositeKeyFromFields(fields)
		if id, ok := a.secondary[ck]; ok {
			if dev := a.devices[id]; dev != nil {
				return id, dev, false
			}
		}
		id := newDeviceID()
		dev := &Device{DeviceID: id, Online: false, Source: "auto", composite: true}
		applyAddressFields(dev, fields)
		// lineHint/stationHint 는 호출부(ingestState)가 로스터 락 취득 전에 레지스트리에서 해석해
		// 주입한다(레지스트리 간 락 중첩 금지, REQ-07-01). lineHint 미해석이면 "" → 3-세그먼트(RD-4),
		// stationHint 는 역번호가 있으면 역번호·없으면 역사 코드 폴백이다.
		dev.Name = composeName(lineHint, stationHint, dev.Place, dev.Index)
		dev.Address = nonAttrFields(fields)
		a.devices[id] = dev
		a.indexDeviceLocked(dev)
		return id, dev, true
	}

	id := fields[placeholderDeviceID]
	if dev := a.devices[id]; dev != nil {
		return id, dev, false
	}
	dev := &Device{DeviceID: id, Online: false, Source: "auto"}
	applyAddressFields(dev, fields)
	dev.Address = nonAttrFields(fields)
	a.devices[id] = dev
	a.indexDeviceLocked(dev) // 위치 계층이 있으면 보조 인덱스에도 등록(없으면 no-op).
	return id, dev, true
}

// newDeviceID 는 글로벌 고유 device_id(UUID v4)를 생성한다 (합성 주소 모델의 PRIMARY 키).
// 프로젝트의 device_id 저장소(samsung/lg)와 동일하게 github.com/google/uuid 를 사용한다.
func newDeviceID() string { return uuid.NewString() }

// indexDeviceLocked 는 디바이스를 보조 인덱스(compositeKey → device_id)에 등록한다. 위치 계층
// 합성 주소가 없는 디바이스(blob 모델의 위치 미지정)는 등록하지 않는다(빈 키 충돌 회피). 로스터
// 락을 보유한 채 호출한다.
func (a *XSFMAgent) indexDeviceLocked(dev *Device) {
	if !hasComposite(dev) {
		return
	}
	a.secondary[compositeKey(dev.Station, dev.Place, dev.Index)] = dev.DeviceID
}

// unindexDeviceLocked 는 디바이스의 보조 인덱스 항목을 제거한다(자기 소유 항목만 — 다른 디바이스가
// 같은 키를 차지한 경우 건드리지 않는다). 로스터 락을 보유한 채 호출한다.
func (a *XSFMAgent) unindexDeviceLocked(dev *Device) {
	if !hasComposite(dev) {
		return
	}
	ck := compositeKey(dev.Station, dev.Place, dev.Index)
	if a.secondary[ck] == dev.DeviceID {
		delete(a.secondary, ck)
	}
}

// seedKeyAndAddress 는 설정 시드 디바이스의 로스터 키와 주소 필드를 상태 템플릿 구성으로
// 도출한다 (M14). 단일 {device_id}(또는 템플릿 없음)는 device_id 자체가 키이며, 다중 필드는
// 비-attribute placeholder 값을 ":" 로 이어 합성한다.
func (a *XSFMAgent) seedKeyAndAddress(dev *Device) (string, map[string]string) {
	var nonAttr []string
	for _, n := range placeholderNames(a.cfg.StateTopicTemplate) {
		if n != placeholderAttribute {
			nonAttr = append(nonAttr, n)
		}
	}
	if len(nonAttr) == 0 || (len(nonAttr) == 1 && nonAttr[0] == placeholderDeviceID) {
		return dev.DeviceID, map[string]string{placeholderDeviceID: dev.DeviceID}
	}
	addr := make(map[string]string, len(nonAttr))
	parts := make([]string, 0, len(nonAttr))
	for _, n := range nonAttr {
		v, _ := deviceFieldValue(n, dev)
		addr[n] = v
		parts = append(parts, v)
	}
	key := strings.Join(parts, ":")
	if key == "" { // 위치 미지정 폴백: device_id 로 키잉.
		return dev.DeviceID, map[string]string{placeholderDeviceID: dev.DeviceID}
	}
	return key, addr
}

// buildCommandFieldsLocked 는 명령 토픽의 비-attribute placeholder 값을 디바이스에서 도출한다
// (M14). dev.Address 를 우선 참조하고, 없으면 표준 placeholder→Device 필드 매핑으로 폴백한다.
// 로스터 락을 보유한 채 호출한다.
func (a *XSFMAgent) buildCommandFieldsLocked(dev *Device) map[string]string {
	names := placeholderNames(a.cfg.CommandTopicTemplate)
	out := make(map[string]string, len(names))
	for _, n := range names {
		if n == placeholderAttribute {
			continue
		}
		// device_index 는 명령 토픽에서 항상 3자리 0-채움(%03d)으로 렌더한다(예: 1 → "001",
		// 23 → "023"). 디바이스 토픽 스킴(composeName 의 %03d 표시 규약)과 일치시키기 위함이며,
		// dev.Address 의 원시 문자열("1"/"3")이나 폴백보다 우선한다. 유입(STATE) 토픽 매칭은 index 를
		// int 로 정규화하므로 "/1/" 와 "/001/" 이 모두 매칭된다 — 이 패딩은 명령 egress 표기에만
		// 영향을 주고 매칭 경로는 불변이다. index 가 1000 이상이면 %03d 가 자연히 넓어진다(절삭 없음).
		if n == placeholderDeviceIndex {
			out[n] = fmt.Sprintf("%03d", dev.Index)
			continue
		}
		if dev.Address != nil {
			if v, ok := dev.Address[n]; ok {
				out[n] = v
				continue
			}
		}
		if v, ok := deviceFieldValue(n, dev); ok {
			out[n] = v
		}
	}
	return out
}

// emitStateChanged 는 device_state_changed 메시지를 msgCh 로 non-blocking 방출한다 (REQ-06-02).
//
// 메시지는 type/device_id/group_id(설정 시)/online/관측 축(StateForJSON)/changed_fields/
// timestamp(epoch ms int64)를 담는다. B8 status 노드가 이 shape 를 재사용한다.
func (a *XSFMAgent) emitStateChanged(deviceID, groupID string, online bool, changed []string, stateAxes map[string]any, timestampMs int64, meta map[string]string) {
	data := map[string]any{
		"device_id":      deviceID,
		"online":         online,
		"changed_fields": changed,
		"timestamp":      timestampMs,
	}
	if groupID != "" {
		data["group_id"] = groupID
	}
	// 관측 게이팅된 상태 축(power/fan_speed/online)을 최상위에 병합한다. online 축이 관측됐다면
	// 최상위 online 과 동일 값이므로 무해하게 덮어쓴다.
	for k, v := range stateAxes {
		data[k] = v
	}
	// 추출된 주소/attribute placeholder 를 메타로 병합한다 (M14): 하류 influx 태그가
	// station_code/place_code/device_index/attribute 를 나른다. device_id 는 최상위와 중복,
	// 상태 축과 충돌하는 키는 건너뛴다.
	for k, v := range meta {
		if k == placeholderDeviceID {
			continue
		}
		if _, exists := data[k]; exists {
			continue
		}
		data[k] = v
	}
	a.sendEvent("device_state_changed", data)
}

// emitOnlineTransition 는 device_online / device_offline 전이 이벤트를 방출한다 (REQ-05-05).
func (a *XSFMAgent) emitOnlineTransition(eventType, deviceID, groupID string, online bool, timestampMs int64) {
	data := map[string]any{
		"device_id": deviceID,
		"online":    online,
		"timestamp": timestampMs,
	}
	if groupID != "" {
		data["group_id"] = groupID
	}
	a.sendEvent(eventType, data)
}

// FeedState 는 port 모드에서 상태 입력 포트로 유입된 원시 상태 페이로드를 device_id 로
// 에이전트에 전달하는 배선 훅이다 (REQ-XSFM-001-01-12, JSON-blob/하위호환 경로). 미등록
// 디바이스는 ingestState 가 Source="auto" 로 자동 등록한다. attribute-per-topic port 유입은
// 토픽이 필요하므로 FeedStateFromTopic 을 사용한다.
func (a *XSFMAgent) FeedState(deviceID string, payload []byte) {
	// 외부 수신 경계 (port 모드 device_id 직접 유입): 항상 우리 소유이므로 진입 시 계측한다.
	a.stats.IncrExternalMessagesReceived()
	a.stats.AddBytesRead(int64(len(payload)))
	a.stats.UpdateLastActivity()
	a.stats.RecordFirstMessage()

	st, err := decodeStatePayload(a.cfg.PayloadMapping, payload)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("xsfm: decode state payload failed", "device_id", deviceID, "error", err)
		return
	}
	if a.cfg.LogMessages {
		a.logFrame("RX", deviceID, payload, stateSummary(st))
	}
	a.ingestState(false, map[string]string{placeholderDeviceID: deviceID}, st)
}

// FeedStateFromTopic 은 port 모드에서 실제 토픽까지 함께 유입될 때의 다중 필드 경로이다 (M14).
// direct 와이드카드 콜백과 동일하게 토픽을 파싱해 주소/attribute 를 추출한다.
func (a *XSFMAgent) FeedStateFromTopic(topic string, payload []byte) {
	a.handleStateMessage(topic, payload)
}

// ControlPort 는 port 모드 제어 출력 포트 채널을 반환한다 (direct 모드는 nil).
// 하류 노드가 이 채널을 drain 하여 mqtt-out 으로 발행한다 (REQ-XSFM-001-01-12).
func (a *XSFMAgent) ControlPort() <-chan ControlMessage {
	return a.ctrlCh
}

// Stop 은 에이전트를 정지한다 (REQ-XSFM-001-01-09).
func (a *XSFMAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("xsfm stop: %w", err)
	}
	close(a.stopCh)
	// B5: 오프라인 감지 모니터 고루틴이 stopCh 를 관측하고 종료할 때까지 대기하여 고루틴/
	// 타이머 누수를 방지한다. 모니터는 락을 채널 송신에 걸쳐 잡지 않으므로 Wait 가 데드락하지
	// 않는다 (offline_timeout<=0 이면 고루틴 미기동, Wait 즉시 반환).
	a.monitorWG.Wait()
	// B3: 미해소 pending 을 모두 종결하여 대기 중인 controlDevice 호출자를 즉시 해제하고
	// 타이머 누수를 방지한다.
	a.pendings.close()
	if a.client != nil {
		a.client.Disconnect()
	}
	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("xsfm stop: %w", err)
	}
	return nil
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *XSFMAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("xsfm pause: %w", err)
	}
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *XSFMAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("xsfm resume: %w", err)
	}
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *XSFMAgent) Health() agent.HealthStatus {
	now := time.Now()
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return agent.HealthStatus{Status: agent.HealthHealthy, LastCheck: now, Message: "xsfm agent is running"}
	case lifecycle.StatePaused:
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "xsfm agent is paused"}
	default:
		return agent.HealthStatus{Status: agent.HealthUnhealthy, LastCheck: now, Message: fmt.Sprintf("xsfm agent is in %s state", a.CurrentState())}
	}
}

// Process 는 JSON 명령을 디스패치한다 (REQ-XSFM-001-01-07 Process 표면).
//
// B2 는 런타임 디바이스 CRUD(Module 2)와 개별 2-축 제어(Module 3)를 구현한다. 셀렉터
// fan-out(group_id/station/line)·응답 대기·역사 레지스트리 CRUD 등은 후속 배치에서 구현되며
// 그 전까지 ErrInvalidCommand 를 반환한다.
func (a *XSFMAgent) Process(data []byte) ([]byte, error) {
	// 내부 수신 경계: 노드/플로우에서 명령이 유입된 지점.
	a.stats.IncrInternalMessagesReceived()

	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("xsfm process: invalid JSON: %w", err)
	}
	// HTTP exec 경로 지원: params 에 담긴 주소지정 필드를 구조체 필드로 backfill 한다
	// (top-level 우선 — 기존 노드·유닛테스트 경로 보존). 디스패치 전에 한 번만 수행.
	req.fillFromParams()

	switch req.Command {
	// Module 2 — 런타임 디바이스 CRUD.
	case "add_device":
		return a.handleAddDevice(req)
	case "remove_device":
		return a.handleRemoveDevice(req)
	case "set_device":
		return a.handleSetDevice(req, data)
	case "list_devices":
		return a.handleListDevices()

	// Module 3/4 — 2-축 제어 (device_id 단일 또는 셀렉터 fan-out). dispatchControl 이
	// 셀렉터 우선순위(device_id > station > line > group_id)로 라우팅한다.
	case "set_power", "set_fan_speed", "set_multiple":
		return a.dispatchControl(req)

	// Module 2B — 역사 레지스트리 CRUD (B6, REQ-XSFM-001-02-11).
	case "add_station":
		return a.handleAddStation(req)
	case "remove_station":
		return a.handleRemoveStation(req)
	case "list_stations":
		return a.handleListStations()

	// 위치(place) 런타임 CRUD — 역사 내에 위치를 등록 (SPEC-AIRPUR-001 Wave1).
	case "add_place":
		return a.handleAddPlace(req)
	case "remove_place":
		return a.handleRemovePlace(req)
	case "list_places":
		return a.handleListPlaces(req)

	// Module 5 — 캐시 상태 조회 (B5, REQ-XSFM-001-05-04). 브로커 통신 없이 로스터에서 즉시 반환.
	case "request_state":
		return a.handleRequestState(req)

	// 커스텀 그룹 CRUD (SPEC-XSFM-GROUP-001 M3, REQ-03-01..06). 기본 그룹(station/line)은
	// 파생이므로 list_groups 조회로만 노출되고 편집/삭제는 ErrGroupNotCustom 으로 거부된다.
	case "add_group":
		return a.handleAddGroup(req, data)
	case "remove_group":
		return a.handleRemoveGroup(req)
	case "set_group":
		return a.handleSetGroup(req, data)
	case "list_groups":
		return a.handleListGroups()

	// 라인 1급 엔티티 CRUD (SPEC-XSFM-LINE-001 M1, REQ-01-03/05/06).
	case "add_line":
		return a.handleAddLine(req)
	case "remove_line":
		return a.handleRemoveLine(req)
	case "list_lines":
		return a.handleListLines()

	// 후속 배치에서 구현.
	case "set_line", "set_station",
		"get_state", "get_all",
		"get_line_stations":
		return nil, fmt.Errorf("%w: %q not implemented in this batch", ErrInvalidCommand, req.Command)
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidCommand, req.Command)
	}
}

// ReceiveMessage 는 msgCh 에서 노드 방출 메시지를 수신한다 (agent.MessageReceiver).
func (a *XSFMAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.msgCh:
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("xsfm: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// 로스터 조회 (REQ-XSFM-001-02-02, RWMutex 보호)
// ---------------------------------------------------------------------------

// ListDevices 는 등록된 전체 디바이스의 값 복사본 목록을 반환한다 (device_id 정렬).
func (a *XSFMAgent) ListDevices() []Device {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]Device, 0, len(a.devices))
	for _, dev := range a.devices {
		out = append(out, dev.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}

// GetDevice 는 device_id 로 디바이스 값 복사본을 조회한다. 미등록 시 ErrDeviceNotFound.
func (a *XSFMAgent) GetDevice(deviceID string) (*Device, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dev, ok := a.devices[deviceID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, deviceID)
	}
	d := dev.clone()
	return &d, nil
}

// GroupMembers 는 group_registry.go / group_membership.go 로 재구현되어 접두사 분기 +
// 로스터 대조 필터를 적용한다 (SPEC-XSFM-GROUP-001 M1, RD-2/RD-3). 시그니처는 동일하여
// 호출부(handleSelectorControl)는 변경되지 않는다.

// ---------------------------------------------------------------------------
// Agent 인터페이스 나머지 (Configure/ID/Name/Type/Info/Stats)
// ---------------------------------------------------------------------------

// Configure 는 설정을 재파싱하여 갱신한다 (런타임 재구성; 트랜스포트 배선 변경은 후속 배치).
func (a *XSFMAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("xsfm configure: %w", err)
	}
	if len(config.Transport.Options) > 0 {
		cfg, err := parseXSFMConfig(config.Transport.Options)
		if err != nil {
			return fmt.Errorf("xsfm configure: re-parse: %w", err)
		}
		a.mu.Lock()
		a.cfg = cfg
		a.agentConfig = config
		a.mu.Unlock()
	} else {
		a.mu.Lock()
		a.agentConfig = config
		a.mu.Unlock()
	}
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *XSFMAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *XSFMAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *XSFMAgent) Type() string { return agentType }

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *XSFMAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}
	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      agentType,
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *XSFMAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = len(a.msgCh), cap(a.msgCh)
	return s
}
