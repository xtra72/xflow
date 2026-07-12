package system

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/message"
)

// nameIDMap 는 ThingsBoard 디바이스 NAME과 xflow 내부 device_id 간의
// 양방향 매핑을 스레드 세이프하게 관리한다.
//
// forward: NAME → device_id (업링크에서 사용)
// reverse: device_id → NAME (다운링크에서 사용)
//
// 두 맵은 항상 일관성을 유지하며 sync.RWMutex로 보호된다.
type nameIDMap struct {
	mu      sync.RWMutex
	forward map[string]string // NAME → device_id
	reverse map[string]string // device_id → NAME
}

// newNameIDMap 는 빈 nameIDMap를 생성한다.
func newNameIDMap() *nameIDMap {
	return &nameIDMap{
		forward: make(map[string]string),
		reverse: make(map[string]string),
	}
}

// put 은 (NAME, device_id) 매핑을 등록한다.
// 동일 NAME이 새로운 device_id로 재매핑되면 이전 역방향 항목을 정리하여
// 정방향/역방향 맵의 일관성을 유지한다.
func (m *nameIDMap) put(name, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// NAME이 이전에 다른 device_id로 매핑되어 있었다면 이전 역방향 항목을 제거한다.
	if oldID, ok := m.forward[name]; ok && oldID != id {
		delete(m.reverse, oldID)
	}

	m.forward[name] = id
	m.reverse[id] = name
}

// deviceID 는 NAME에 해당하는 device_id를 반환한다 (정방향 조회).
func (m *nameIDMap) deviceID(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.forward[name]
	return id, ok
}

// name 은 device_id에 해당하는 NAME을 반환한다 (역방향 조회).
func (m *nameIDMap) name(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.reverse[id]
	return n, ok
}

// resolveDeviceID 는 device NAME을 xflow device_id로 해석하고 매핑을 채운다.
//
// agent.ResolveDeviceID(ctx, agentName, name)로 device_id를 조회하며,
// device_id_repo가 미설정(nil)이거나 해석에 실패하여 빈 문자열이 반환되면
// NAME 자체를 device_id로 사용한다 (REQ-map-fallback / graceful degradation).
//
// 해석 결과는 nameIDMap에 등록되어 이후 역매핑(NAME↔device_id)에 사용된다.
func (m *nameIDMap) resolveDeviceID(ctx context.Context, agentName, name string) string {
	id := agent.ResolveDeviceID(ctx, agentName, name)
	if id == "" {
		// repo 미설정 또는 미해석 → NAME 자체를 키로 사용한다.
		id = name
	}
	m.put(name, id)
	return id
}

// extractDeviceName 는 인입 메시지 페이로드에서 JSONPath로 디바이스 NAME을 추출한다.
//
// path는 "$."로 시작하는 JSONPath여야 하며(기본 "$.device"),
// 추출된 값이 문자열이 아니면 에러를 반환한다.
func extractDeviceName(p message.Payload, path string) (string, error) {
	v, err := p.GetPath(path)
	if err != nil {
		return "", fmt.Errorf("thingplus: device name 추출 실패 (path=%q): %w", path, err)
	}
	name, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("thingplus: device name이 문자열이 아님 (path=%q, type=%T)", path, v)
	}
	if name == "" {
		return "", fmt.Errorf("thingplus: device name이 비어 있음 (path=%q)", path)
	}
	return name, nil
}

// validateDeviceName 는 추출된 NAME 값이 비어 있지 않은 유효한 문자열인지 검증한다.
// extractDeviceName 의 empty 검증 규칙과 동일한 에러 메시지 형식을 재사용한다.
func validateDeviceName(name, path string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("thingplus: device name이 비어 있음 (path=%q)", path)
	}
	return name, nil
}

// resolveDeviceNameFromMessage 는 재구성된 메시지에서 device_name_path 로 디바이스 NAME을
// 스코프에 맞게 해석한다. 다음 네 가지 경로 형태를 지원한다.
//
//   - "$.metadata.<group>.<key>" → m.Metadata().GetGroup(group)[key] (예: "$.metadata.device.name")
//   - "$.metadata.<key>"         → m.Metadata().Get(key) (flat 메타데이터, 예: "$.metadata.device_id")
//   - "$.payload.<rest>"         → payload 에서 "$.<rest>" 로 조회 (명시적 payload 스코프)
//   - bare "$.<rest>"            → payload 에서 path 그대로 조회 (기본/하위 호환, 예: "$.device")
//
// m 이 nil(raw JSON, 메타데이터 없음)이면 메타데이터 스코프 경로는 명확한 에러를 반환하고,
// payload 스코프 경로는 payload 로 정상 동작한다.
func resolveDeviceNameFromMessage(m message.Message, payload map[string]any, path string) (string, error) {
	const metaPrefix = "$.metadata."
	const payloadPrefix = "$.payload."

	switch {
	case strings.HasPrefix(path, metaPrefix):
		if m == nil {
			return "", fmt.Errorf("thingplus: device name 추출 실패 (path=%q): 메타데이터 없음 (raw JSON 인입)", path)
		}
		rest := path[len(metaPrefix):]
		if rest == "" {
			return "", fmt.Errorf("thingplus: 잘못된 device_name_path (path=%q)", path)
		}
		md := m.Metadata()
		if dot := strings.IndexByte(rest, '.'); dot >= 0 {
			// "$.metadata.<group>.<key>" → nested group 조회.
			group := rest[:dot]
			key := rest[dot+1:]
			g, ok := md.GetGroup(group)
			if !ok {
				return "", fmt.Errorf("thingplus: device name 추출 실패 (path=%q): 메타데이터 그룹 %q 없음", path, group)
			}
			v, ok := g[key]
			if !ok {
				return "", fmt.Errorf("thingplus: device name 추출 실패 (path=%q): 그룹 %q 에 키 %q 없음", path, group, key)
			}
			return validateDeviceName(v, path)
		}
		// "$.metadata.<key>" → flat 메타데이터 조회.
		v, ok := md.Get(rest)
		if !ok {
			return "", fmt.Errorf("thingplus: device name 추출 실패 (path=%q): flat 메타데이터 키 %q 없음", path, rest)
		}
		return validateDeviceName(v, path)

	case strings.HasPrefix(path, payloadPrefix):
		// 명시적 payload 스코프: "$.payload.<rest>" → payload 에서 "$.<rest>" 로 조회.
		rest := path[len(payloadPrefix):]
		if rest == "" {
			return "", fmt.Errorf("thingplus: 잘못된 device_name_path (path=%q)", path)
		}
		return extractDeviceName(message.NewPayload(payload), "$."+rest)

	default:
		// bare "$.<rest>" (기본/하위 호환): payload 에서 path 그대로 조회.
		return extractDeviceName(message.NewPayload(payload), path)
	}
}
