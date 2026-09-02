package chirpstack

import (
	"errors"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/agent"
)

// ErrNameCollision 은 이미 등록된 에이전트 이름과 충돌하는(다른 ID 의) ChirpStack
// 에이전트 생성이 거부되었음을 나타낸다.
//
// device_id 는 (agentName, devEui) 로 키잉되므로(REQ-M4-01), 에이전트 이름이
// 중복되면 서로 다른 물리 게이트웨이의 디바이스가 동일 UUID 로 병합되는 위험이
// 있다. 따라서 이름 고유성 위반은 조용히 덮어쓰지 않고 거부한다 (REQ-M1-04).
var ErrNameCollision = errors.New("chirpstack: agent name already in use")

var (
	nameRegMu sync.Mutex
	// nameReg 는 등록된 에이전트 이름 → 소유 에이전트 ID 매핑이다.
	// 같은 (name,id) 재클레임(재시작 경로)은 idempotent 하고, 다른 id 가 기존
	// name 을 클레임하면 ErrNameCollision 을 반환한다.
	nameReg = make(map[string]string)
)

// claimAgentName 은 (name,id) 로 에이전트 이름을 클레임한다.
//
//   - name 이 비어 있으면 no-op(nil) — 이름 검증은 config.Validate 가 담당.
//   - name 이 미등록이면 등록.
//   - name 이 동일 id 로 이미 등록됐으면 idempotent(nil).
//   - name 이 다른 id 로 등록됐으면 ErrNameCollision.
func claimAgentName(name, id string) error {
	if name == "" {
		return nil
	}
	nameRegMu.Lock()
	defer nameRegMu.Unlock()
	if existing, ok := nameReg[name]; ok && existing != id {
		return fmt.Errorf("%w: %q (owned by agent %q)", ErrNameCollision, name, existing)
	}
	nameReg[name] = id
	return nil
}

// releaseAgentName 은 (name,id) 클레임을 해제한다. Stop 경로에서 호출되어
// 이름을 재사용할 수 있게 한다. id 가 일치하지 않으면 no-op(다른 소유자 보존).
func releaseAgentName(name, id string) {
	if name == "" {
		return
	}
	nameRegMu.Lock()
	defer nameRegMu.Unlock()
	if existing, ok := nameReg[name]; ok && existing == id {
		delete(nameReg, name)
	}
}

// resetNameRegistryForTest 는 테스트 전용으로 이름 레지스트리를 초기화한다.
// 프로덕션 코드는 호출하지 않는다.
func resetNameRegistryForTest() {
	nameRegMu.Lock()
	defer nameRegMu.Unlock()
	nameReg = make(map[string]string)
}

// RegisterChirpStackTypes 는 ChirpStack LoRaWAN 에이전트 타입을 agent.DefaultManager
// 에 등록한다 (REQ-M1-01/02, AC-8).
//
// 등록 이름은 "chirpstack-client" 이다. 부트스트랩 호출 예시:
//
//	if err := chirpstack.RegisterChirpStackTypes(agentMgr); err != nil {
//	    return fmt.Errorf("register chirpstack: %w", err)
//	}
//
// (cmd/xflowd/main.go 의 에이전트 타입 등록 블록에서 호출 + import 를 추가해야
// 인스턴스화가 가능하다 — 누락 시 manager.Create("chirpstack-client", ...) 가 실패한다.)
//
// @MX:ANCHOR: main.go 등록 블록(cmd/xflowd/main.go)이 이 함수를 호출해야 한다.
// @MX:REASON: 미호출 시 chirpstack 타입 미등록으로 플로우 인스턴스화 불가(REQ-M1-02).
func RegisterChirpStackTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("chirpstack-client", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewChirpStackAgent(config)
	})
}
