// apply.go 는 M3 클라이언트 측 명령 적용 인터페이스와 도메인 라우팅을 구현한다
// (@SPEC:SPEC-REMOTE-001 M3, spec §5.7 클라이언트 적용 경로, REQ-D02/D03/D04/D09, A5).
//
// 디커플링(import cycle 회피):
//
//	internal/remote 는 internal/api/service / internal/api/handler 를 import 하지
//	않는다(handler 가 remote 를 import 하므로 역방향은 cycle). 대신 본 파일은
//	도메인별 좁은 인터페이스(DomainCommander)만 정의하고, 구체 어댑터(FlowServiceAdapter
//	등) 바인딩은 cmd/xflowd 가 담당한다. 이로써 remote 패키지는 어댑터 구현에 비의존이며
//	apply_test.go 는 fake DomainCommander 로 라우팅을 단독 검증한다.
//
// 적용 일관성(A5/REQ-D09):
//
//	각 DomainCommander.Do 는 로컬 API 와 동일한 어댑터/검증 경로를 호출한다(원격 우회
//	없음). 적용 실패는 부분 적용 없이 오류로 보고되며, 호출자(client)는 이를
//	command_result 오류로 변환한다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnknownDomain 은 명령 domain 이 flow/agent/device 중 하나가 아닐 때 반환된다.
var ErrUnknownDomain = errors.New("remote: unknown command domain")

// ErrApplierUnavailable 은 해당 도메인의 commander 가 구성되지 않았을 때 반환된다.
var ErrApplierUnavailable = errors.New("remote: domain commander not configured")

// CommandApplier 는 클라이언트가 수신한 명령을 로컬에 적용하는 추상화이다(REQ-D02/D03/
// D04). client 는 이 인터페이스만 의존하며, cmd/xflowd 가 구체 어댑터를 바인딩한다.
type CommandApplier interface {
	// Apply 는 domain/action 에 따라 명령을 적용하고 결과(json.RawMessage)를 반환한다.
	// 실패 시 오류를 반환한다(부분 적용 없음 — REQ-D09).
	Apply(ctx context.Context, domain, action string, args json.RawMessage) (json.RawMessage, error)
}

// DomainCommander 는 한 도메인(flow|agent|device)의 명령 적용을 추상화한다.
//
// 구현(cmd/xflowd)은 action 별로 해당 어댑터 메서드(예: FlowServiceAdapter.DeployFlow)
// 에 라우팅하고, args 를 dto 로 디코드하며, 결과를 json.RawMessage 로 마샬한다.
// 적용은 로컬 API 와 동일 검증/제약을 받는다(A5).
type DomainCommander interface {
	// Do 는 action + args 로 명령을 적용하고 결과를 반환한다.
	Do(ctx context.Context, action string, args json.RawMessage) (json.RawMessage, error)
}

// Applier 는 domain → DomainCommander 라우팅을 수행하는 CommandApplier 구현이다.
//
// 각 필드는 nil 일 수 있으며, nil 도메인으로의 명령은 ErrApplierUnavailable 로
// 거부된다(해당 도메인 미노출 노드 보호).
type Applier struct {
	Flow   DomainCommander
	Agent  DomainCommander
	Device DomainCommander
	// System 은 노드 자체 운영 명령(예: system/update 자가 업데이트) 도메인이다(버전
	// 관리 Phase 2). nil 이면 system 도메인 명령은 ErrApplierUnavailable 로 거부된다.
	System DomainCommander
}

var _ CommandApplier = (*Applier)(nil)

// NewApplier 는 도메인 commander 를 바인딩한 Applier 를 생성한다.
func NewApplier(flow, agent, device DomainCommander) *Applier {
	return &Applier{Flow: flow, Agent: agent, Device: device}
}

// WithSystem 은 system 도메인 commander 를 바인딩한다(버전 관리 Phase 2, chainable).
// 주입하지 않으면 system 도메인 명령은 거부된다(자가 업데이트 미지원 노드 보호).
func (a *Applier) WithSystem(system DomainCommander) *Applier {
	a.System = system
	return a
}

// Apply 는 domain 에 따라 적절한 DomainCommander 로 라우팅한다(REQ-D02/D03/D04).
func (a *Applier) Apply(ctx context.Context, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	cmder, err := a.commanderFor(domain)
	if err != nil {
		return nil, err
	}
	return cmder.Do(ctx, action, args)
}

// commanderFor 는 domain 에 매핑된 DomainCommander 를 반환한다.
func (a *Applier) commanderFor(domain string) (DomainCommander, error) {
	switch domain {
	case DomainFlow:
		if a.Flow == nil {
			return nil, fmt.Errorf("%w: %s", ErrApplierUnavailable, domain)
		}
		return a.Flow, nil
	case DomainAgent:
		if a.Agent == nil {
			return nil, fmt.Errorf("%w: %s", ErrApplierUnavailable, domain)
		}
		return a.Agent, nil
	case DomainDevice:
		if a.Device == nil {
			return nil, fmt.Errorf("%w: %s", ErrApplierUnavailable, domain)
		}
		return a.Device, nil
	case DomainSystem:
		if a.System == nil {
			return nil, fmt.Errorf("%w: %s", ErrApplierUnavailable, domain)
		}
		return a.System, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDomain, domain)
	}
}
