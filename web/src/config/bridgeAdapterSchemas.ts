// 브릿지 에이전트 타입별 어댑터 설정 스키마.
// 각 프로토콜(MQTT, Modbus, HTTP 등)에 고유한 설정 필드를 정의한다.
// getBridgeAdapterFields()는 에이전트 타입에 따라 동적으로 ConfigField[]를 반환한다.

import type { ConfigField } from '@/types/node';

// --- MQTT 어댑터 설정 필드 ---

const MQTT_ADAPTER_FIELDS: ConfigField[] = [
  {
    name: 'default_qos',
    type: 'select',
    label: 'QoS 레벨',
    options: ['0', '1', '2'],
    default: '0',
    description: '0: 최대 1회, 1: 최소 1회, 2: 정확히 1회',
  },
  {
    name: 'default_retained',
    type: 'boolean',
    label: 'Retained 메시지',
    default: false,
    description: '발행 시 Retained 플래그 설정',
  },
  {
    name: 'publish_topic',
    type: 'string',
    label: '발행 토픽',
    description: '발행 토픽 템플릿 (예: devices/{device_id}/data)',
  },
];

// --- Modbus 어댑터 공통 설정 필드 ---

const MODBUS_ADAPTER_FIELDS: ConfigField[] = [
  {
    name: 'unit_id',
    type: 'number',
    label: 'Unit ID',
    default: 1,
    description: 'Modbus 슬레이브 주소 (1-247)',
  },
  {
    name: 'polling_interval_ms',
    type: 'number',
    label: '폴링 간격 (ms)',
    default: 1000,
    description: '레지스터 읽기 주기 (최소 100ms)',
  },
  {
    name: 'register_map',
    type: 'register_map',
    label: '레지스터 맵',
    description: '읽기/쓰기 대상 레지스터 영역 정의',
  },
];

// --- HTTP 어댑터 설정 필드 ---

const HTTP_ADAPTER_FIELDS: ConfigField[] = [
  {
    name: 'content_type',
    type: 'select',
    label: 'Content-Type',
    options: ['application/json', 'text/plain', 'application/x-www-form-urlencoded', 'multipart/form-data'],
    default: 'application/json',
    description: '요청/응답 본문 형식',
  },
  {
    name: 'url_template',
    type: 'string',
    label: 'URL 템플릿',
    description: 'URL 경로 템플릿 (예: /api/{resource}/{id})',
  },
  {
    name: 'timeout_ms',
    type: 'number',
    label: '타임아웃 (ms)',
    default: 5000,
    description: 'HTTP 요청 타임아웃 (밀리초)',
  },
];

// --- 에이전트 타입 -> 어댑터 필드 매핑 ---

const ADAPTER_SCHEMA_MAP: Record<string, ConfigField[]> = {
  'mqtt': MQTT_ADAPTER_FIELDS,
  'modbus-tcp': MODBUS_ADAPTER_FIELDS,
  'modbus-rtu': MODBUS_ADAPTER_FIELDS,
  'modbus-tcp-server': MODBUS_ADAPTER_FIELDS,
  'http': HTTP_ADAPTER_FIELDS,
};

/**
 * 에이전트 타입에 해당하는 어댑터 전용 설정 필드를 반환한다.
 * 매핑되지 않은 에이전트 타입은 빈 배열을 반환한다.
 */
export function getBridgeAdapterFields(agentType?: string): ConfigField[] {
  if (!agentType) return [];
  return ADAPTER_SCHEMA_MAP[agentType] ?? [];
}

/**
 * 에이전트 타입이 어댑터 전용 설정을 가지는지 확인한다.
 */
export function hasAdapterConfig(agentType?: string): boolean {
  if (!agentType) return false;
  return agentType in ADAPTER_SCHEMA_MAP;
}

/**
 * 어댑터 설정의 카테고리 라벨을 반환한다.
 */
export function getAdapterCategoryLabel(agentType?: string): string | undefined {
  if (!agentType) return undefined;
  const labels: Record<string, string> = {
    'mqtt': 'MQTT 설정',
    'modbus-tcp': 'Modbus 설정',
    'modbus-rtu': 'Modbus 설정',
    'modbus-tcp-server': 'Modbus 설정',
    'http': 'HTTP 설정',
  };
  return labels[agentType];
}
