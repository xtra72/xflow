// client_mirror.go 는 M4 클라이언트 측 인벤토리 미러 송신 루프를 구현한다
// (@SPEC:SPEC-REMOTE-001 M4, spec §5.5 client, REQ-E01/E02/E07, A04/A07, F06).
//
// 루프 동작(spec §1.5):
//   - 세션 시작: 노출 범위로 필터된 redacted 스냅샷을 push 한다(inventory_snapshot,
//     REQ-E01). 이를 baseline 으로 저장한다.
//   - 주기적(poll): 스냅샷을 다시 빌드하여 baseline 과 diff → 변경분을 inventory_delta
//     (add/update/remove)로 push 한다(REQ-E02). baseline 을 갱신한다.
//   - 노출 변경(A07): remirror 신호 수신 시 즉시 diff 를 수행한다. 새 노출 범위에서
//     사라진 자원은 remove 델타로 신호된다(REQ-A07 — 노출 해제 자원 제거).
//
// 델타 소스는 poll+diff 이다(데몬에 구독 가능한 변경 이벤트 소스 없음 — inventory.go
// 결정 주석). 세션 종료/ctx 취소 시 루프가 정리된다(goroutine leak 방지).
package remote

import (
	"context"
	"time"
)

// inventoryState 는 종류별 마지막으로 push 한 항목 집합(baseline)이다(diff 기준).
type inventoryState struct {
	flows   []InventoryItem
	agents  []InventoryItem
	devices []InventoryItem
}

// runMirror 는 세션 동안 인벤토리 미러 루프를 실행한다(REQ-E01/E02).
// sessionCtx 취소 시 반환한다.
func (c *Client) runMirror(ctx context.Context, conn Conn) {
	// 1) 접속 시 전체 스냅샷 push(REQ-E01) + baseline 설정.
	state, err := c.pushSnapshot(ctx, conn)
	if err != nil {
		c.logger.Debug("인벤토리 스냅샷 송신 실패", "error", err)
		// 스냅샷 실패 시에도 루프는 유지하여 다음 주기에 재시도한다.
		state = &inventoryState{}
	}

	c.logger.Info("인벤토리 미러 시작(poll 기반 델타 소스)",
		"instance_id", c.cfg.InstanceID, "poll_interval", c.cfg.InventoryPollInterval)

	ticker := time.NewTicker(c.cfg.InventoryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 주기적 poll+diff → 델타 push(REQ-E02).
			if err := c.pushDeltas(ctx, conn, state); err != nil {
				c.logger.Debug("인벤토리 델타 송신 실패", "error", err)
			}
		case <-c.remirror:
			// 노출 변경 재미러링(A07): diff 로 추가/제거를 즉시 반영한다(노출 해제 →
			// remove 델타). baseline 은 새 노출 범위 기준으로 갱신된다.
			c.logger.Info("노출 변경 감지 — 재미러링", "instance_id", c.cfg.InstanceID)
			if err := c.pushDeltas(ctx, conn, state); err != nil {
				c.logger.Debug("재미러링 델타 송신 실패", "error", err)
			}
		}
	}
}

// buildSnapshot 은 소스에서 인벤토리를 읽어 현재 노출 범위로 필터한다(REQ-E07/A04).
// 반환 항목의 Definition 은 소스가 이미 redaction(F06)한 상태이다.
func (c *Client) buildSnapshot(ctx context.Context) (*inventoryState, error) {
	exp := c.currentExposure()

	flows, err := c.cfg.Inventory.ListFlows(ctx)
	if err != nil {
		return nil, err
	}
	agents, err := c.cfg.Inventory.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	devices, err := c.cfg.Inventory.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	return &inventoryState{
		flows:   applyExposure(exp.Flows, flows),
		agents:  applyExposure(exp.Agents, agents),
		devices: applyExposure(exp.Devices, devices),
	}, nil
}

// pushSnapshot 은 전체 스냅샷을 빌드하여 push 하고 baseline 을 반환한다(REQ-E01).
func (c *Client) pushSnapshot(ctx context.Context, conn Conn) (*inventoryState, error) {
	state, err := c.buildSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: c.cfg.InstanceID,
		Flows:      state.flows,
		Agents:     state.agents,
		Devices:    state.devices,
	})
	if err != nil {
		return nil, err
	}
	if err := writeEnvelope(conn, msg); err != nil {
		return nil, err
	}
	return state, nil
}

// pushDeltas 는 현재 인벤토리를 baseline 과 diff 하여 변경분을 델타로 push 하고
// baseline 을 in-place 로 갱신한다(REQ-E02/A07).
func (c *Client) pushDeltas(ctx context.Context, conn Conn, baseline *inventoryState) error {
	current, err := c.buildSnapshot(ctx)
	if err != nil {
		return err
	}

	deltas := make([]InventoryDeltaPayload, 0)
	deltas = append(deltas, diffItems(KindFlow, baseline.flows, current.flows)...)
	deltas = append(deltas, diffItems(KindAgent, baseline.agents, current.agents)...)
	deltas = append(deltas, diffItems(KindDevice, baseline.devices, current.devices)...)

	for _, d := range deltas {
		d.InstanceID = c.cfg.InstanceID
		msg, encErr := NewInventoryDeltaMessage(d)
		if encErr != nil {
			c.logger.Error("inventory_delta 인코딩 실패", "error", encErr)
			continue
		}
		if wErr := writeEnvelope(conn, msg); wErr != nil {
			return wErr
		}
	}

	// baseline 갱신(다음 diff 기준).
	baseline.flows = current.flows
	baseline.agents = current.agents
	baseline.devices = current.devices
	return nil
}

// currentExposure 는 현재 노출 설정을 반환한다(A07 핫리로드와 경합 안전).
func (c *Client) currentExposure() ExposureSummary {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exposure
}

// UpdateExposure 는 노출 설정을 갱신하고 재미러링을 신호한다(REQ-A07/A06 핫리로드).
// cmd/xflowd 의 config OnChange 훅이 호출한다. 활성 미러 루프가 없으면 신호는
// 버퍼 1 채널에 보관되어 다음 세션 스냅샷이 새 범위를 반영한다.
func (c *Client) UpdateExposure(exp ExposureSummary) {
	c.mu.Lock()
	c.exposure = exp
	c.mu.Unlock()
	select {
	case c.remirror <- struct{}{}:
	default:
		// 이미 보류된 신호가 있으면(coalesce) 추가 신호는 불필요하다.
	}
}
