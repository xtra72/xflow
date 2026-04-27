// WebSocket service barrel exports

export { WSClient, createWSClient } from './wsClient';
export type { ConnectionState, WSClientOptions } from './wsClient';

export {
  createMessageRouter,
  onFlowStatus,
  onFlowMetrics,
  onNodeStats,
  onAgentStatus,
  onLogEntry,
  onSystemEvent,
  onDeviceStatus,
  WS_MESSAGE_TYPES,
} from './wsHandlers';
export type { WSMessage, WSMessageType, WSHandlerMap } from './wsHandlers';
