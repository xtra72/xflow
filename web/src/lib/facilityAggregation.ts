// Facility dashboard 순수 집계 함수 (SPEC-FACILITY-DASHBOARD-001 B1 / M1).
//
// REQ-FACDASH-001-05-01/02/03: airpurifier 로스터(list_devices)와 역사 레지스트리
// (list_stations)를 화면 표시용으로 ROLL UP 만 한다. 부수효과·I/O 가 전혀 없는 순수
// 함수이며(테스트 대상 코어), station→line 매핑·fan-out·응답 대기 등의 도메인 로직은
// 여기서 재구현하지 않는다(UB-001). 이 모듈은 오직 roster + registry 를 합산한다.
//
// 미분류(UNCLASSIFIED) 규약(UB-004, REQ-05-03): 레지스트리에 없는(또는 station 이 빈)
// 디바이스는 호선 집계에서 제외하되 별도로 카운트한다.

import type { AirDevice, AirStation } from '@/hooks/useStation';

// ---- 결과 타입 ----

/** 상태 카운트 묶음. fan1/2/3 은 전원 상태와 무관한 원시 풍량 분포이다(표시 여부는 패널이 결정). */
export interface StatCounts {
  total: number;
  online: number;
  offline: number;
  powerOn: number;
  powerOff: number;
  fan1: number;
  fan2: number;
  fan3: number;
}

/** 한 역사(station)의 롤업 요약. entry 미등록 시 line='' order=0 displayName=station code. */
export interface StationSummary {
  station: string;
  displayName: string;
  line: string;
  order: number;
  stats: StatCounts;
  devices: AirDevice[];
}

/** 한 호선(line)의 롤업 요약. stations 는 order→code 정렬, stats 는 소속 전 디바이스 합산. */
export interface LineSummary {
  line: string;
  stations: StationSummary[];
  stats: StatCounts;
  deviceCount: number;
}

/** 미분류(레지스트리 미등록/빈 station) 디바이스 집계. */
export interface UnclassifiedSummary {
  count: number;
  deviceIds: string[];
}

/** aggregateByLine 결과: 호선별 롤업 + 미분류 분리. */
export interface LineAggregation {
  lines: LineSummary[];
  unclassified: UnclassifiedSummary;
}

/** 선로도(line-diagram) 노드: 레지스트리 order 기준 정렬된 논리 배치(좌표 없음). */
export interface LineDiagramNode {
  station: string;
  displayName: string;
  order: number;
  summary: StationSummary;
}

// ---- 헬퍼 ----

/** station code → 레지스트리 항목 맵. 빈 code 항목은 스킵한다. */
export function buildStationRegistry(stations: AirStation[]): Map<string, AirStation> {
  const map = new Map<string, AirStation>();
  for (const s of stations) {
    if (s.station) map.set(s.station, s);
  }
  return map;
}

/** device.station code → 디바이스 목록 맵(로스터 그룹핑). */
function groupDevicesByStation(devices: AirDevice[]): Map<string, AirDevice[]> {
  const map = new Map<string, AirDevice[]>();
  for (const d of devices) {
    const arr = map.get(d.station) ?? [];
    arr.push(d);
    map.set(d.station, arr);
  }
  return map;
}

/** order 오름차순, 동률 시 station code 오름차순 비교. */
function compareStationSummary(a: StationSummary, b: StationSummary): number {
  if (a.order !== b.order) return a.order - b.order;
  return a.station.localeCompare(b.station);
}

// ---- 집계 함수 ----

/**
 * 디바이스 목록의 상태 분포를 합산한다. online/offline, power on/off, 풍량(1/2/3) 원시 분포.
 * 풍량은 전원과 무관하게 raw 로 카운트한다(표시 로직은 패널이 결정, REQ-05-01).
 */
export function countStats(devices: AirDevice[]): StatCounts {
  const stats: StatCounts = {
    total: devices.length,
    online: 0,
    offline: 0,
    powerOn: 0,
    powerOff: 0,
    fan1: 0,
    fan2: 0,
    fan3: 0,
  };
  for (const d of devices) {
    if (d.online) stats.online += 1;
    else stats.offline += 1;
    if (d.power) stats.powerOn += 1;
    else stats.powerOff += 1;
    switch (d.fan_speed) {
      case 1:
        stats.fan1 += 1;
        break;
      case 2:
        stats.fan2 += 1;
        break;
      case 3:
        stats.fan3 += 1;
        break;
      default:
        break;
    }
  }
  return stats;
}

/**
 * 한 역사의 요약을 만든다. entry(레지스트리 항목)가 있으면 display_name/line/order 를 쓰고,
 * 없으면(미등록) line='' order=0 displayName=station code 로 방어한다.
 */
export function stationSummary(
  station: string,
  devices: AirDevice[],
  entry?: AirStation,
): StationSummary {
  return {
    station,
    displayName: entry?.display_name || station,
    line: entry?.line ?? '',
    order: entry?.order ?? 0,
    stats: countStats(devices),
    devices,
  };
}

/**
 * 로스터를 호선별로 롤업한다(REQ-05-02). 레지스트리 미등록/빈 station 디바이스는 호선 집계에서
 * 제외하고 unclassified 로 분리 카운트한다(UB-004, REQ-05-03). 각 호선은 소속 역사들을
 * order→code 정렬로 담고, 호선 stats 는 소속 전 디바이스를 합산한다. 호선은 line code 정렬.
 */
export function aggregateByLine(devices: AirDevice[], stations: AirStation[]): LineAggregation {
  const registry = buildStationRegistry(stations);
  const unclassifiedIds: string[] = [];
  // line → (station code → devices)
  const lineMap = new Map<string, Map<string, AirDevice[]>>();

  for (const d of devices) {
    const entry = d.station ? registry.get(d.station) : undefined;
    if (!entry) {
      unclassifiedIds.push(d.device_id);
      continue;
    }
    let stationMap = lineMap.get(entry.line);
    if (!stationMap) {
      stationMap = new Map<string, AirDevice[]>();
      lineMap.set(entry.line, stationMap);
    }
    const arr = stationMap.get(d.station) ?? [];
    arr.push(d);
    stationMap.set(d.station, arr);
  }

  const lines: LineSummary[] = [];
  for (const [line, stationMap] of lineMap) {
    const summaries: StationSummary[] = [];
    for (const [stationCode, devs] of stationMap) {
      summaries.push(stationSummary(stationCode, devs, registry.get(stationCode)));
    }
    summaries.sort(compareStationSummary);
    const allDevices = summaries.flatMap((s) => s.devices);
    lines.push({
      line,
      stations: summaries,
      stats: countStats(allDevices),
      deviceCount: allDevices.length,
    });
  }
  lines.sort((a, b) => a.line.localeCompare(b.line));

  return {
    lines,
    unclassified: { count: unclassifiedIds.length, deviceIds: unclassifiedIds },
  };
}

/**
 * 로스터를 역사별로 롤업한다. 로스터에 존재하는 station code 마다 하나의 요약을 만들며,
 * 레지스트리에 있으면 name/line/order 를 backfill 한다(미등록은 방어값). order→code 정렬.
 * 빈 station code(미분류) 디바이스도 하나의 요약으로 그룹핑된다(패널이 표시 결정).
 */
export function aggregateByStation(devices: AirDevice[], stations: AirStation[]): StationSummary[] {
  const registry = buildStationRegistry(stations);
  const byStation = groupDevicesByStation(devices);
  const summaries: StationSummary[] = [];
  for (const [stationCode, devs] of byStation) {
    summaries.push(stationSummary(stationCode, devs, registry.get(stationCode)));
  }
  summaries.sort(compareStationSummary);
  return summaries;
}

/**
 * 한 호선의 선로도 논리 배치를 만든다(REQ-05-02 선로도). 레지스트리에서 해당 호선에 속한 모든
 * 역사를 order 오름차순(동률 시 code)으로 정렬한 노드 목록을 반환한다. 디바이스가 없는 역사도
 * 레지스트리에 있으면 노드로 포함된다(빈 요약). 좌표 없는 순서 기반 배치이다.
 */
export function lineDiagramLayout(
  line: string,
  stations: AirStation[],
  devices: AirDevice[],
): LineDiagramNode[] {
  const byStation = groupDevicesByStation(devices);
  const nodes: LineDiagramNode[] = stations
    .filter((s) => s.line === line && s.station)
    .map((s) => {
      const summary = stationSummary(s.station, byStation.get(s.station) ?? [], s);
      return { station: s.station, displayName: summary.displayName, order: s.order, summary };
    });
  nodes.sort((a, b) => (a.order !== b.order ? a.order - b.order : a.station.localeCompare(b.station)));
  return nodes;
}
