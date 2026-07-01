// 노드 아이콘 리졸버 (단일 소스).
//
// 기존에 CustomNode(캔버스)와 NodeItem(팔레트)이 각각 카테고리별 아이콘만
// 가지고 있어 노드의 "의미"가 드러나지 않았다. 이 모듈은 노드 타입별 의미
// 아이콘(NODE_TYPE_ICONS) → 카테고리 폴백(CATEGORY_ICONS) → 최종 폴백(Cog)
// 우선순위로 단일 lucide 아이콘을 결정해 두 곳에서 공유한다.
//
// 노드 타입은 normalizeNodeType 로 canonical(`-`) 표기로 정규화한 뒤 조회하므로,
// 저장된 플로우의 옛 `_` HVAC 타입도 동일 아이콘으로 해석된다.

import {
  Activity,
  Antenna,
  Archive,
  ArrowUpFromLine,
  BarChart3,
  Boxes,
  Bug,
  Cable,
  Code,
  Cog,
  Columns3,
  CopyMinus,
  Database,
  Filter,
  GitBranch,
  Globe,
  HardDrive,
  Inbox,
  Layers,
  LineChart,
  Monitor,
  Network,
  Plug,
  Puzzle,
  Radio,
  Replace,
  Route,
  ScrollText,
  ShieldAlert,
  Shuffle,
  Sigma,
  SlidersHorizontal,
  Sparkles,
  Split,
  Thermometer,
  ThermometerSun,
  TriangleAlert,
  Workflow,
  Zap,
  type LucideIcon,
} from 'lucide-react';

import { normalizeNodeType } from './nodeType';

/**
 * 카테고리별 폴백 아이콘 매핑.
 *
 * NODE_TYPE_ICONS 에 매핑이 없는 노드 타입은 이 맵으로 폴백한다. 기존
 * CustomNode/NodeItem 두 곳에 흩어져 있던 카테고리 키를 모두 보존한다
 * (processing/routing/io/error/debug/storage + 레거시 input/output/process/
 * bridge/special).
 */
export const CATEGORY_ICONS: Record<string, LucideIcon> = {
  processing: Cog,
  routing: GitBranch,
  io: Cable,
  error: ShieldAlert,
  debug: Bug,
  storage: Database,
  // 레거시 호환 (palette / 구 백엔드 카테고리)
  input: Zap,
  output: ArrowUpFromLine,
  process: Cog,
  bridge: Cable,
  special: Sparkles,
};

/**
 * 노드 타입별 의미 아이콘 매핑 (canonical `-` 키 기준).
 *
 * 같은 의미 그룹은 일관된 아이콘 계열을 쓰되, status(상태 수신/모니터)와
 * control(명령/제어)은 시각적으로 구분되게 배정한다:
 *   - HVAC status → Thermometer 계열, control → SlidersHorizontal/Power
 *   - 통합 노드(상태+제어) → ThermometerSun (양쪽 의미를 함의)
 */
export const NODE_TYPE_ICONS: Record<string, LucideIcon> = {
  // --- 프로토콜 / IO ---
  // MQTT: 무선 발행/구독 → Radio 계열, 수신은 Antenna 로 미세 구분
  mqtt: Radio,
  'mqtt-subscriber': Antenna,
  'mqtt-publisher': Radio,
  // Modbus: 산업용 시리얼/TCP 버스 → Cable/Plug/Network 계열
  modbus: Cable,
  'modbus-tcp': Network,
  'modbus-rtu': Cable,
  'modbus-tcp-server': Network,
  'modbus-poller': Cable,
  'modbus-writer': Plug,
  // HTTP → Globe
  http: Globe,
  // 시리얼/TCP → 물리 결선
  'serial-in': Plug,
  'serial-out': Plug,
  'tcp-in': Network,
  'tcp-out': Network,
  // 브리지 → 범용 결선
  bridge: Cable,

  // --- HVAC(공조) ---
  // status = 상태 수신/모니터(Thermometer), control = 제어(SlidersHorizontal),
  // 통합 = ThermometerSun.
  'samsung-hvacr01': ThermometerSun,
  'samsung-hvacr01-status': Thermometer,
  'samsung-hvacr01-control': SlidersHorizontal,
  lgap: ThermometerSun,
  'lgap-status': Thermometer,
  'lgap-control': SlidersHorizontal,
  'lg-hvacr01': ThermometerSun,
  'lg-hvacr01-status': Thermometer,
  'lg-hvacr01-control': SlidersHorizontal,
  'lg-hvacr02': ThermometerSun,
  'lg-hvacr02-status': Thermometer,
  'lg-hvacr02-control': SlidersHorizontal,
  'century-hvacr01': ThermometerSun,
  'century-hvacr01-status': Thermometer,
  'century-hvacr01-control': SlidersHorizontal,

  // --- 저장 / 시계열 ---
  // 시계열(influx/tsdb): write=Database, read/query=LineChart/Activity 로 구분
  influxdb: Database,
  'influxdb-write': Database,
  'influxdb-read': LineChart,
  'influxdb-query': Activity,
  'tsdb-write': Database,
  'tsdb-query': LineChart,
  // in-memory store: write=Archive, read=HardDrive
  'store-write': Archive,
  'store-read': HardDrive,

  // --- 처리 ---
  transform: Shuffle,
  mapping: Replace,
  'select-field': Columns3,
  aggregate: Sigma,
  deduplicate: CopyMinus,
  filter: Filter,
  script: Code,
  framer: Layers,
  enrich: Sparkles,
  inventory: Boxes,

  // --- 라우팅 ---
  switch: Split,

  // --- 트리거 / 입력 ---
  trigger: Zap,

  // --- 출력 / 디버그 ---
  output: ArrowUpFromLine,
  'chart-emitter': BarChart3,
  logger: ScrollText,
  'error-logger': TriangleAlert,

  // --- 에러 ---
  catch: ShieldAlert,
  deadletter: Inbox,
  error: TriangleAlert,

  // --- 기타 ---
  custom: Puzzle,
  'flow-node': Workflow,

  // --- 그 외 카테고리 폴백과 의미가 겹치는 보조 매핑 ---
  // (palette 등에서 표준화되지 않은 라우팅/출력 키가 올 경우 의미 보존)
  route: Route,
  monitor: Monitor,
};

/**
 * 노드 타입(+카테고리)에 해당하는 lucide 아이콘을 결정한다.
 *
 * 우선순위: NODE_TYPE_ICONS(노드 타입별) → CATEGORY_ICONS(카테고리 폴백) → Cog.
 * 노드 타입은 정규화(`_`→`-`)한 raw 두 표기를 모두 시도해 옛 HVAC 타입도 매칭한다.
 *
 * @param nodeType 노드 타입 식별자 (예: "mqtt-subscriber", 옛 "lg_hvacr01_status")
 * @param category 노드 카테고리 (폴백용, 선택)
 */
export function getNodeIcon(nodeType: string, category?: string): LucideIcon {
  const raw = nodeType ?? '';
  const canonical = normalizeNodeType(raw);

  // 1) 노드 타입별 의미 아이콘 (canonical 우선, raw 도 시도)
  const typeIcon = NODE_TYPE_ICONS[canonical] ?? NODE_TYPE_ICONS[raw];
  if (typeIcon) return typeIcon;

  // 2) 카테고리 폴백
  if (category) {
    const categoryIcon = CATEGORY_ICONS[category];
    if (categoryIcon) return categoryIcon;
  }

  // 3) 최종 폴백
  return Cog;
}
