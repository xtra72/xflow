// WebSocket message routing with typed handlers.
// Dispatches incoming messages to domain-specific callbacks.

import type { WSClient } from './wsClient';

// Wire-level message shape exchanged over the WebSocket connection
export interface WSMessage {
  type: string;
  payload: unknown;
  timestamp?: string;
}

// Known message types sent by the XFlow server
export const WS_MESSAGE_TYPES = {
  FLOW_STATUS: 'flow.status',
  FLOW_METRICS: 'flow.metrics',
  NODE_STATS: 'node.stats',
  AGENT_STATUS: 'agent.status',
  LOG_ENTRY: 'log.entry',
  SYSTEM_EVENT: 'system.event',
  DEVICE_STATUS: 'device.status',
  DEBUG_MESSAGE: 'debug.message',
  // 노드 출력 tap(관찰) 스트림. 와이어 연결 없이 임의 노드의 출력 메시지를 전달한다.
  NODE_OUTPUT: 'node.output',
} as const;

export type WSMessageType = (typeof WS_MESSAGE_TYPES)[keyof typeof WS_MESSAGE_TYPES];

// Typed callback signatures per message domain
export type FlowStatusHandler = (data: unknown) => void;
export type FlowMetricsHandler = (data: unknown) => void;
export type NodeStatsHandler = (data: unknown) => void;
export type AgentStatusHandler = (data: unknown) => void;
export type LogEntryHandler = (data: unknown) => void;
export type SystemEventHandler = (data: unknown) => void;
export type DeviceStatusHandler = (data: unknown) => void;
export type DebugMessageHandler = (data: unknown) => void;
export type NodeOutputHandler = (data: unknown) => void;

// Handler map for typed registration
export interface WSHandlerMap {
  [WS_MESSAGE_TYPES.FLOW_STATUS]?: FlowStatusHandler;
  [WS_MESSAGE_TYPES.FLOW_METRICS]?: FlowMetricsHandler;
  [WS_MESSAGE_TYPES.NODE_STATS]?: NodeStatsHandler;
  [WS_MESSAGE_TYPES.AGENT_STATUS]?: AgentStatusHandler;
  [WS_MESSAGE_TYPES.LOG_ENTRY]?: LogEntryHandler;
  [WS_MESSAGE_TYPES.SYSTEM_EVENT]?: SystemEventHandler;
  [WS_MESSAGE_TYPES.DEVICE_STATUS]?: DeviceStatusHandler;
  [WS_MESSAGE_TYPES.DEBUG_MESSAGE]?: DebugMessageHandler;
  [WS_MESSAGE_TYPES.NODE_OUTPUT]?: NodeOutputHandler;
}

// Set up a message router that dispatches incoming messages to typed callbacks.
// Returns a cleanup function that unregisters all handlers.
export function createMessageRouter(client: WSClient, handlers: WSHandlerMap): () => void {
  const entries = Object.entries(handlers) as [string, (data: unknown) => void][];

  for (const [type, handler] of entries) {
    client.on(type, handler);
  }

  return () => {
    for (const [type, handler] of entries) {
      client.off(type, handler);
    }
  };
}

// Convenience helpers for registering individual typed handlers.
// Each returns a cleanup function that removes the handler.

export function onFlowStatus(client: WSClient, handler: FlowStatusHandler): () => void {
  client.on(WS_MESSAGE_TYPES.FLOW_STATUS, handler);
  return () => client.off(WS_MESSAGE_TYPES.FLOW_STATUS, handler);
}

export function onFlowMetrics(client: WSClient, handler: FlowMetricsHandler): () => void {
  client.on(WS_MESSAGE_TYPES.FLOW_METRICS, handler);
  return () => client.off(WS_MESSAGE_TYPES.FLOW_METRICS, handler);
}

export function onNodeStats(client: WSClient, handler: NodeStatsHandler): () => void {
  client.on(WS_MESSAGE_TYPES.NODE_STATS, handler);
  return () => client.off(WS_MESSAGE_TYPES.NODE_STATS, handler);
}

export function onAgentStatus(client: WSClient, handler: AgentStatusHandler): () => void {
  client.on(WS_MESSAGE_TYPES.AGENT_STATUS, handler);
  return () => client.off(WS_MESSAGE_TYPES.AGENT_STATUS, handler);
}

export function onLogEntry(client: WSClient, handler: LogEntryHandler): () => void {
  client.on(WS_MESSAGE_TYPES.LOG_ENTRY, handler);
  return () => client.off(WS_MESSAGE_TYPES.LOG_ENTRY, handler);
}

export function onSystemEvent(client: WSClient, handler: SystemEventHandler): () => void {
  client.on(WS_MESSAGE_TYPES.SYSTEM_EVENT, handler);
  return () => client.off(WS_MESSAGE_TYPES.SYSTEM_EVENT, handler);
}

export function onDeviceStatus(client: WSClient, handler: DeviceStatusHandler): () => void {
  client.on(WS_MESSAGE_TYPES.DEVICE_STATUS, handler);
  return () => client.off(WS_MESSAGE_TYPES.DEVICE_STATUS, handler);
}

export function onDebugMessage(client: WSClient, handler: DebugMessageHandler): () => void {
  client.on(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handler);
  return () => client.off(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handler);
}

export function onNodeOutput(client: WSClient, handler: NodeOutputHandler): () => void {
  client.on(WS_MESSAGE_TYPES.NODE_OUTPUT, handler);
  return () => client.off(WS_MESSAGE_TYPES.NODE_OUTPUT, handler);
}
