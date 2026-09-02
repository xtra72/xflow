package modbus

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// update_device 의 엔드포인트(host/port) 변경 (SPEC-MODBUS-013 M7)
// ---------------------------------------------------------------------------
//
// register_groups/unit_id/케이던스는 연결을 유지한 채 in-place 로 바꿀 수 있지만,
// host/port 는 트랜스포트 자체를 다시 만들어야 한다. 종전에는 이 필드를 파싱조차 하지
// 않아 조용히 무시됐고, UI 도 편집 모드에서 감춰 두었다.
//
// 여기서는 remove+add 를 한 번의 원자적 교체로 수행한다: 같은 device id 를 유지한 채
// 새 엔드포인트로 트랜스포트를 다시 만들고, 구 트랜스포트는 마지막-참조 규칙으로 닫는다.
// 캐시·통계는 device id 로 키잉되므로 그대로 유지된다(이력 보존).

// endpointChange 는 파싱된 host/port 변경 요청이다.
type endpointChange struct {
	host    string
	port    int
	hasHost bool
	hasPort bool
}

// wants 는 변경 요청이 하나라도 있는지 반환한다.
func (e endpointChange) wants() bool { return e.hasHost || e.hasPort }

// parseEndpointChange 는 update_device params 에서 host/port 를 파싱·검증한다.
// 키가 없으면 빈 요청을 돌려준다(기존 동작 유지).
func parseEndpointChange(params map[string]any) (endpointChange, error) {
	var ec endpointChange

	if v, ok := params["host"]; ok {
		s, isStr := v.(string)
		if !isStr {
			return endpointChange{}, fmt.Errorf("modbus update_device: host must be a string")
		}
		if s == "" {
			return endpointChange{}, fmt.Errorf("modbus update_device: host must not be empty")
		}
		ec.host, ec.hasHost = s, true
	}

	if v, ok := params["port"]; ok {
		p := toInt(v)
		if p < 1 || p > 65535 {
			return endpointChange{}, fmt.Errorf(
				"modbus update_device: port must be 1-65535 (got %d)", p)
		}
		ec.port, ec.hasPort = p, true
	}

	return ec, nil
}

// applyEndpointChangeLocked 는 디바이스를 새 엔드포인트로 교체한다.
// 호출자는 a.mu.Lock() 을 보유해야 한다.
//
// 반환값:
//   - replacement: 새 트랜스포트를 가진 교체 디바이스(변경이 불필요하면 원본 dev 그대로)
//   - closeOld: a.mu 해제 후 실행할 구 트랜스포트 close (불필요하면 nil)
//   - changed: 실제로 교체가 일어났는지
//
// RTU 디바이스는 host/port 개념이 없으므로 오류로 거부한다(부분 적용 없음 — 호출자가
// 다른 필드를 적용하기 전에 검사한다).
func (a *ModbusAgent) applyEndpointChangeLocked(
	dev *ModbusDevice, ec endpointChange,
) (replacement *ModbusDevice, closeOld func(), changed bool, err error) {
	if !ec.wants() {
		return dev, nil, false, nil
	}

	cfg := a.config
	if effectiveTransportKind(dev.config, cfg) != TransportTCP {
		return nil, nil, false, fmt.Errorf(
			"modbus update_device %q: host/port apply to TCP devices only (device transport is rtu)",
			dev.config.ID)
	}

	newCfg := dev.config // 값 복사 — RegisterGroups 슬라이스 헤더는 공유해도 무방(교체만 함).
	if ec.hasHost {
		newCfg.Host = ec.host
	}
	if ec.hasPort {
		newCfg.Port = ec.port
	}
	if newCfg.Host == dev.config.Host && newCfg.Port == dev.config.Port {
		return dev, nil, false, nil // 값이 같으면 연결을 건드리지 않는다.
	}

	// 구 디바이스의 폴링 루프를 먼저 정지한다(교체 중 구 트랜스포트로 요청이 나가지 않도록).
	a.stopDeviceBlockLoops(dev.config.ID)

	// 새 트랜스포트 선택은 add 경로와 동일한 규칙(공유 세션 / 상속 RTU 버스 / 독립)을 재사용한다.
	replacement = a.buildRuntimeDeviceLocked(newCfg)
	// unit_id 런타임 SSOT 를 승계한다(update_device 로 바뀐 값이 교체에서 유실되지 않도록).
	replacement.setUnitID(dev.UnitID())

	// copy-on-write 로 컬렉션에서 교체한다(원자적 적용).
	newDevices := make([]*ModbusDevice, 0, len(a.devices))
	for _, d := range a.devices {
		if d == dev {
			newDevices = append(newDevices, replacement)
			continue
		}
		newDevices = append(newDevices, d)
	}
	a.devices = newDevices

	// 구 트랜스포트 close 계획 — 교체 후 목록 기준으로 마지막-참조를 판정한다.
	// 실제 블로킹 Close 는 호출자가 a.mu 해제 후 수행한다(remove 경로와 동일 규약).
	closeOld = a.planTransportCloseLocked(dev, newDevices)

	a.logger.Info("modbus: update_device 엔드포인트 변경",
		"device", newCfg.ID,
		"host", newCfg.Host,
		"port", newCfg.Port,
	)
	return replacement, closeOld, true, nil
}

// connectReplacement 는 교체 디바이스를 a.mu 밖에서 연결한다.
// 실패는 치명적이지 않다 — add 경로와 동일하게 오프라인으로 편입하고 폴링 루프가 재연결한다.
func (a *ModbusAgent) connectReplacement(dev *ModbusDevice, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := dev.Connect(ctx); err != nil {
		a.logger.Warn("modbus: 엔드포인트 변경 후 연결 실패(오프라인 편입)",
			"device", dev.config.ID, "error", err)
	}
}
