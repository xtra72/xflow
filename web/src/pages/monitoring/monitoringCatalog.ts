// 모니터링 항목 카탈로그.
//
// 각 섹션에서 추가할 수 있는 항목의 표시 정보(아이콘 / 라벨 키 / 설명 키 / 그룹)를
// 한곳에 모은다. 추가 다이얼로그와 항목 렌더러가 같은 정의를 공유하므로 항목을
// 늘릴 때 손댈 곳은 여기와 monitoringLayout.ts 의 키 유니온 두 군데뿐이다.

import {
  AlertTriangle,
  ArrowDownToLine,
  ArrowUpFromLine,
  Bug,
  CheckCircle,
  Clock,
  Cpu,
  GitBranch,
  Info,
  Layers,
  MemoryStick,
  Play,
  Radio,
  Rocket,
  ScrollText,
  Server,
  TrendingUp,
  Wifi,
  Zap,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import type {
  EventItemKey,
  LogItemKey,
  MetricItemKey,
  MonitorSectionKey,
  NetworkItemKey,
  StatItemKey,
} from './monitoringLayout';

/** 항목 하나의 표시 정보 */
export interface MonitorItemMeta<K extends string> {
  key: K;
  icon: LucideIcon;
  /** 항목 이름 i18n 키 */
  labelKey: string;
  /** 항목 설명 i18n 키 (추가 다이얼로그에서만 노출) */
  descriptionKey: string;
  /** 추가 다이얼로그의 소그룹 라벨 i18n 키 (없으면 그룹 미표시) */
  groupKey?: string;
}

/** 통계 항목 — 런타임 / 플로우 / 수신량 / 연결 4개 그룹 */
export const STAT_ITEMS: MonitorItemMeta<StatItemKey>[] = [
  { key: 'cpuUsage', icon: Cpu, labelKey: 'monitoring.items.stats.cpuUsage', descriptionKey: 'monitoring.items.stats.cpuUsageDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'memoryUsage', icon: MemoryStick, labelKey: 'monitoring.items.stats.memoryUsage', descriptionKey: 'monitoring.items.stats.memoryUsageDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'goRoutines', icon: Layers, labelKey: 'monitoring.items.stats.goRoutines', descriptionKey: 'monitoring.items.stats.goRoutinesDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'heapAlloc', icon: Server, labelKey: 'monitoring.items.stats.heapAlloc', descriptionKey: 'monitoring.items.stats.heapAllocDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'memSys', icon: Server, labelKey: 'monitoring.items.stats.memSys', descriptionKey: 'monitoring.items.stats.memSysDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'uptime', icon: Clock, labelKey: 'monitoring.items.stats.uptime', descriptionKey: 'monitoring.items.stats.uptimeDesc', groupKey: 'monitoring.itemGroups.runtime' },
  { key: 'totalFlows', icon: GitBranch, labelKey: 'monitoring.totalFlows', descriptionKey: 'monitoring.items.stats.totalFlowsDesc', groupKey: 'monitoring.itemGroups.flow' },
  { key: 'runningFlows', icon: Play, labelKey: 'monitoring.runningFlows', descriptionKey: 'monitoring.items.stats.runningFlowsDesc', groupKey: 'monitoring.itemGroups.flow' },
  { key: 'logsReceived', icon: ScrollText, labelKey: 'monitoring.logsReceived', descriptionKey: 'monitoring.items.stats.logsReceivedDesc', groupKey: 'monitoring.itemGroups.ingest' },
  { key: 'eventsReceived', icon: Radio, labelKey: 'monitoring.eventsReceived', descriptionKey: 'monitoring.items.stats.eventsReceivedDesc', groupKey: 'monitoring.itemGroups.ingest' },
  { key: 'wsState', icon: Wifi, labelKey: 'monitoring.items.stats.wsState', descriptionKey: 'monitoring.items.stats.wsStateDesc', groupKey: 'monitoring.itemGroups.connection' },
];

/** 메트릭 차트 항목 */
export const METRIC_ITEMS: MonitorItemMeta<MetricItemKey>[] = [
  { key: 'cpu', icon: Cpu, labelKey: 'monitoring.cpu', descriptionKey: 'monitoring.items.metrics.cpuDesc', groupKey: 'monitoring.itemGroups.process' },
  { key: 'memory', icon: MemoryStick, labelKey: 'monitoring.memory', descriptionKey: 'monitoring.items.metrics.memoryDesc', groupKey: 'monitoring.itemGroups.process' },
  { key: 'throughput', icon: TrendingUp, labelKey: 'monitoring.throughputLabel', descriptionKey: 'monitoring.items.metrics.throughputDesc', groupKey: 'monitoring.itemGroups.process' },
  { key: 'errorRate', icon: Zap, labelKey: 'monitoring.errorRate', descriptionKey: 'monitoring.items.metrics.errorRateDesc', groupKey: 'monitoring.itemGroups.process' },
];

/** 네트워크 항목 — 단위시간당 증가량(rate)과 부팅 이후 누적(total) 두 그룹 */
export const NETWORK_ITEMS: MonitorItemMeta<NetworkItemKey>[] = [
  { key: 'rxBytes', icon: ArrowDownToLine, labelKey: 'monitoring.items.network.rxBytes', descriptionKey: 'monitoring.items.network.rxBytesDesc', groupKey: 'monitoring.itemGroups.netRate' },
  { key: 'txBytes', icon: ArrowUpFromLine, labelKey: 'monitoring.items.network.txBytes', descriptionKey: 'monitoring.items.network.txBytesDesc', groupKey: 'monitoring.itemGroups.netRate' },
  { key: 'rxPackets', icon: ArrowDownToLine, labelKey: 'monitoring.items.network.rxPackets', descriptionKey: 'monitoring.items.network.rxPacketsDesc', groupKey: 'monitoring.itemGroups.netRate' },
  { key: 'txPackets', icon: ArrowUpFromLine, labelKey: 'monitoring.items.network.txPackets', descriptionKey: 'monitoring.items.network.txPacketsDesc', groupKey: 'monitoring.itemGroups.netRate' },
  { key: 'rxBytesTotal', icon: Layers, labelKey: 'monitoring.items.network.rxBytesTotal', descriptionKey: 'monitoring.items.network.rxBytesTotalDesc', groupKey: 'monitoring.itemGroups.netTotal' },
  { key: 'txBytesTotal', icon: Layers, labelKey: 'monitoring.items.network.txBytesTotal', descriptionKey: 'monitoring.items.network.txBytesTotalDesc', groupKey: 'monitoring.itemGroups.netTotal' },
  { key: 'rxPacketsTotal', icon: Layers, labelKey: 'monitoring.items.network.rxPacketsTotal', descriptionKey: 'monitoring.items.network.rxPacketsTotalDesc', groupKey: 'monitoring.itemGroups.netTotal' },
  { key: 'txPacketsTotal', icon: Layers, labelKey: 'monitoring.items.network.txPacketsTotal', descriptionKey: 'monitoring.items.network.txPacketsTotalDesc', groupKey: 'monitoring.itemGroups.netTotal' },
];

/** 로그 뷰어 항목 (레벨 프리셋) */
export const LOG_ITEMS: MonitorItemMeta<LogItemKey>[] = [
  { key: 'all', icon: ScrollText, labelKey: 'monitoring.items.logs.all', descriptionKey: 'monitoring.items.logs.allDesc' },
  { key: 'error', icon: AlertTriangle, labelKey: 'monitoring.items.logs.error', descriptionKey: 'monitoring.items.logs.errorDesc' },
  { key: 'warn', icon: AlertTriangle, labelKey: 'monitoring.items.logs.warn', descriptionKey: 'monitoring.items.logs.warnDesc' },
  { key: 'info', icon: Info, labelKey: 'monitoring.items.logs.info', descriptionKey: 'monitoring.items.logs.infoDesc' },
  { key: 'debug', icon: Bug, labelKey: 'monitoring.items.logs.debug', descriptionKey: 'monitoring.items.logs.debugDesc' },
];

/** 이벤트 타임라인 항목 (유형 프리셋) */
export const EVENT_ITEMS: MonitorItemMeta<EventItemKey>[] = [
  { key: 'all', icon: Radio, labelKey: 'monitoring.items.events.all', descriptionKey: 'monitoring.items.events.allDesc' },
  { key: 'status_change', icon: CheckCircle, labelKey: 'monitoring.items.events.statusChange', descriptionKey: 'monitoring.items.events.statusChangeDesc' },
  { key: 'deployment', icon: Rocket, labelKey: 'monitoring.items.events.deployment', descriptionKey: 'monitoring.items.events.deploymentDesc' },
  { key: 'error', icon: AlertTriangle, labelKey: 'monitoring.items.events.error', descriptionKey: 'monitoring.items.events.errorDesc' },
  { key: 'system', icon: Server, labelKey: 'monitoring.items.events.system', descriptionKey: 'monitoring.items.events.systemDesc' },
];

/** 섹션 → 항목 카탈로그 */
export const SECTION_CATALOG: Record<MonitorSectionKey, MonitorItemMeta<string>[]> = {
  stats: STAT_ITEMS as MonitorItemMeta<string>[],
  metrics: METRIC_ITEMS as MonitorItemMeta<string>[],
  network: NETWORK_ITEMS as MonitorItemMeta<string>[],
  logs: LOG_ITEMS as MonitorItemMeta<string>[],
  events: EVENT_ITEMS as MonitorItemMeta<string>[],
};

/** 섹션 내 특정 항목의 표시 정보를 찾는다 (없으면 undefined) */
export function findItemMeta(
  section: MonitorSectionKey,
  key: string,
): MonitorItemMeta<string> | undefined {
  return SECTION_CATALOG[section].find((item) => item.key === key);
}
