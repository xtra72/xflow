// remote_inventory.go 는 원격 관리 클라이언트(mode=client)의 인벤토리 소스 어댑터를
// 정의한다(@SPEC:SPEC-REMOTE-001 M4, spec §5.5 어댑터 브리지, REQ-E01/E04/E07, F06).
//
// internal/remote 는 import cycle 회피를 위해 internal/api/service / handler 를
// import 하지 않고 중립 remote.InventorySource 인터페이스만 정의한다. 본 파일이 그
// 인터페이스를 로컬 API 와 동일한 어댑터 인스턴스(FlowServiceAdapter /
// AgentServiceAdapter / device 레지스트리)에 바인딩한다(A5 — 동일 상태원).
//
// redaction(F06): 노드를 떠나기 전 flow 정의/agent 설정의 시크릿을 제거한다. handler
// 패키지의 단일 진실 공급원(secret_fields.go)인 RedactSensitiveConfig 를 재사용한다.
// device 메타데이터는 시크릿을 보유하지 않는 사용자 라벨/위치 정보이므로 그대로 노출한다.
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/remote"
)

// inventoryFlowLister 는 인벤토리에 필요한 FlowServiceAdapter 의 좁은 인터페이스이다.
type inventoryFlowLister interface {
	ListFlows(ctx context.Context, opts dto.ListOptions) ([]handler.FlowInfo, int64, error)
}

// inventoryAgentLister 는 인벤토리에 필요한 AgentServiceAdapter 의 좁은 인터페이스이다.
type inventoryAgentLister interface {
	ListAgents(ctx context.Context, opts dto.ListOptions) ([]handler.AgentInfo, int64, error)
}

// inventoryDeviceLister 는 인벤토리에 필요한 device 레지스트리의 좁은 인터페이스이다.
type inventoryDeviceLister interface {
	List(filter device.DeviceFilter) []device.Device
}

// remoteInventorySource 는 로컬 어댑터를 remote.InventorySource 로 어댑트한다.
// 노출 후보 전체를 redacted InventoryItem 으로 반환한다(노출 범위 필터는 remote 가
// 평가 — REQ-E07).
type remoteInventorySource struct {
	flows   inventoryFlowLister
	agents  inventoryAgentLister
	devices inventoryDeviceLister
}

var _ remote.InventorySource = (*remoteInventorySource)(nil)

// newRemoteInventorySource 는 어댑터 인스턴스를 바인딩한 인벤토리 소스를 생성한다.
func newRemoteInventorySource(flows inventoryFlowLister, agents inventoryAgentLister, devices inventoryDeviceLister) *remoteInventorySource {
	return &remoteInventorySource{flows: flows, agents: agents, devices: devices}
}

// inventoryPageSize 는 미러 전체 조회 시 페이지 크기이다(ListOptions.Normalize 가
// maxSize 로 클램프하므로 페이지네이션으로 전체를 수집한다 — 대규모 인벤토리 대응).
const inventoryPageSize = 100

// pageOpts 는 page 번호로 미러 조회 옵션을 만든다(필터 없음).
func pageOpts(page int) dto.ListOptions {
	opts := dto.ListOptions{}
	opts.Page = page
	opts.Size = inventoryPageSize
	opts.Normalize()
	return opts
}

// ListFlows 는 플로우를 redacted InventoryItem 으로 반환한다(REQ-E01/F06).
// 전체 수집을 위해 total 에 도달할 때까지 페이지네이션한다(대규모 인벤토리 대응).
func (s *remoteInventorySource) ListFlows(ctx context.Context) ([]remote.InventoryItem, error) {
	var flows []handler.FlowInfo
	for page := 1; ; page++ {
		batch, total, err := s.flows.ListFlows(ctx, pageOpts(page))
		if err != nil {
			return nil, err
		}
		flows = append(flows, batch...)
		if len(batch) == 0 || int64(len(flows)) >= total {
			break
		}
	}
	out := make([]remote.InventoryItem, 0, len(flows))
	for _, f := range flows {
		// 시크릿 제거(F06): Config 는 React Flow 정의이며 노드 설정에 시크릿이 있을 수
		// 있으므로 redaction 한다.
		def := marshalDefinition(handler.RedactSensitiveConfig(f.Config))
		out = append(out, remote.InventoryItem{
			ID:         f.ID,
			Name:       f.Name,
			Kind:       remote.KindFlow,
			Status:     f.Status,
			Definition: def,
			UpdatedAt:  parseRFC3339Millis(f.UpdatedAt),
		})
	}
	return out, nil
}

// ListAgents 는 에이전트를 redacted InventoryItem 으로 반환한다(REQ-E01/F06).
func (s *remoteInventorySource) ListAgents(ctx context.Context) ([]remote.InventoryItem, error) {
	var agents []handler.AgentInfo
	for page := 1; ; page++ {
		batch, total, err := s.agents.ListAgents(ctx, pageOpts(page))
		if err != nil {
			return nil, err
		}
		agents = append(agents, batch...)
		if len(batch) == 0 || int64(len(agents)) >= total {
			break
		}
	}
	out := make([]remote.InventoryItem, 0, len(agents))
	for _, a := range agents {
		// 시크릿 제거(F06): agent Config 는 자격증명(password/token 등)을 보유할 수 있다.
		def := marshalDefinition(handler.RedactSensitiveConfig(a.Config))
		out = append(out, remote.InventoryItem{
			ID:         a.ID,
			Name:       a.Name,
			Kind:       remote.KindAgent,
			Status:     a.Status,
			Definition: def,
		})
	}
	return out, nil
}

// ListDevices 는 IoT 디바이스를 InventoryItem 으로 반환한다(REQ-E01/E04).
//
// device 메타데이터는 사용자 라벨/위치/태그 등 비시크릿 정보이므로 그대로 노출한다.
// 디바이스 자격증명은 device 엔티티가 아닌 소유 에이전트의 Config 에 있으며 위에서
// redaction 된다(F06).
func (s *remoteInventorySource) ListDevices(_ context.Context) ([]remote.InventoryItem, error) {
	devices := s.devices.List(device.DeviceFilter{})
	out := make([]remote.InventoryItem, 0, len(devices))
	for _, d := range devices {
		meta := d.Metadata()
		name := meta.Name
		if name == "" {
			name = d.Name()
		}
		def := marshalDefinition(map[string]any{
			"protocol":   d.Protocol(),
			"type":       string(d.Type()),
			"agent_name": d.AgentName(),
			"online":     d.Online(),
		})
		status := "offline"
		if d.Online() {
			status = "online"
		}
		out = append(out, remote.InventoryItem{
			ID:         d.ID(),
			Name:       name,
			Kind:       remote.KindDevice,
			Status:     status,
			Definition: def,
			UpdatedAt:  d.LastSeen().UnixMilli(),
		})
	}
	return out, nil
}

// parseRFC3339Millis 는 RFC3339 시각 문자열을 epoch milliseconds 로 변환한다.
// 파싱 실패/빈 문자열은 0 을 반환한다(프로젝트 ts 규약 — epoch ms int64).
func parseRFC3339Millis(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// marshalDefinition 은 맵을 JSON 으로 직렬화한다. 실패/빈 입력은 빈 JSON 객체이다.
func marshalDefinition(m map[string]any) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
