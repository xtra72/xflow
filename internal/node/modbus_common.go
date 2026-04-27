package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// modbusNodeBase - Modbus 노드 공통 기반 구조체
// ---------------------------------------------------------------------------

// modbusNodeBase 는 Modbus 노드(modbus, modbus-poller, modbus-writer) 공통 기반 구조체이다.
// mqttNodeBase 패턴을 따르며, Agent 해석/호출/종료 로직을 공유한다.
type modbusNodeBase struct {
	*BaseNode
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 Agent 객체
	agentType string        // "server" | "client"
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// ---------------------------------------------------------------------------
// AgentResolver 초기화
// ---------------------------------------------------------------------------

// initModbusResolver 는 BaseNode config에서 AgentResolver를 추출한다.
// BridgeNode, MQTTNode 등과 동일한 패턴으로 _agent_resolver 키를 사용한다.
func (mb *modbusNodeBase) initModbusResolver() {
	if mb.BaseNode.config != nil {
		if r, ok := mb.BaseNode.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				mb.resolver = resolver
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Agent Resolve & 타입 감지
// ---------------------------------------------------------------------------

// resolveModbusAgent 는 AgentResolver를 통해 Modbus 에이전트를 resolve하고
// Server/Client 타입을 자동 감지한다.
// 성공 시 mb.transport, mb.agent, mb.agentType 이 설정된다.
func (mb *modbusNodeBase) resolveModbusAgent(ctx context.Context, agentRef string) error {
	if mb.resolver == nil {
		return ErrModbusNoResolver
	}

	ref := flow.AgentRef{AgentID: agentRef, AgentName: agentRef}
	transport, err := mb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("modbus init: agent resolve failed: %w", err)
	}
	mb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 감지
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrModbusAgentNotMODBUS
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *modbusserver.ModbusServerAgent:
		mb.agentType = agentTypeServer
		mb.agent = underlyingAgent
	case *modbus.ModbusAgent:
		mb.agentType = agentTypeClient
		mb.agent = underlyingAgent
	default:
		return ErrModbusAgentNotMODBUS
	}

	return nil
}

// ---------------------------------------------------------------------------
// Agent Process 호출
// ---------------------------------------------------------------------------

// callModbusAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
// 기존 ModbusNode.callAgentProcess 와 동일한 로직이며, modbusNodeBase 메서드로
// 이름을 구분하여 충돌을 방지한다.
func (mb *modbusNodeBase) callModbusAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	mb.mu.RLock()
	a := mb.agent
	mb.mu.RUnlock()

	if a == nil {
		return nil, ErrModbusNoResolver
	}

	// context timeout 설정
	timeoutCtx, cancel := context.WithTimeout(ctx, mb.timeout)
	defer cancel()

	// 채널 기반 timeout 처리
	type processResult struct {
		data []byte
		err  error
	}
	ch := make(chan processResult, 1)

	go func() {
		data, err := a.Process(cmdBytes)
		ch <- processResult{data: data, err: err}
	}()

	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("modbus: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// ---------------------------------------------------------------------------
// 공통 종료 로직
// ---------------------------------------------------------------------------

// modbusShutdown 은 공통 종료 로직을 수행한다.
func (mb *modbusNodeBase) modbusShutdown() error {
	return mb.BaseNode.TransitionTo(lifecycle.StateStopping)
}
