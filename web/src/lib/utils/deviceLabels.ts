// 디바이스 속성 키를 한국어 라벨로 변환하는 유틸리티.

import type { DeviceInfo } from '@/types/device';

/**
 * 디바이스 표시명을 결정한다.
 *
 * SPEC-DEVICE-IDENTITY-001 Phase D (M11):
 * fallback 순서: metadata.name → name → agent_name (composite 또는 UUID 노출 방지).
 * Phase D (xflowd v1.0) 부터 `device.id` 는 UUID 를 반환하므로 사용자에게
 * 직접 노출하면 raw UUID 가 화면에 표시되는 문제가 발생한다. `agent_name`
 * 을 최종 fallback 으로 사용하면 사람이 읽을 수 있는 라벨이 보장된다.
 */
export function getDeviceDisplayName(device: DeviceInfo): string {
  if (device.metadata?.name) return device.metadata.name;
  if (device.name) return device.name;
  if (device.agent_name) return device.agent_name;
  return device.uid ?? device.id;
}

const SAMSUNG_NASA_INDOOR_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  current_humidity: '현재 습도', // NASA V1.1 지표 세트 #10 (0x4038)
  fan_speed: '풍량',
  swing_vertical: '상하 스윙',
  filter_alarm: '필터 알람',
  error_code: '에러 코드',
};

const SAMSUNG_NASA_OUTDOOR_LABELS: Record<string, string> = {
  power: '전원',
  current_temperature: '현재 온도',
  error_code: '에러 코드',
  // 실외기(ODU) 텔레메트리 — internal/agent/samsung/outdoor.go outdoorFieldRegistry 와 대응.
  // 온도(°C) 계열
  outdoor_temperature: '실외 온도',
  compressor_discharge_temperature: '압축기 토출 온도',
  out_sensor_pipein3: '파이프 입구 온도 3',
  out_sensor_pipein4: '파이프 입구 온도 4',
  out_sensor_pipein5: '파이프 입구 온도 5',
  out_sensor_pipeout1: '파이프 출구 온도 1',
  out_sensor_pipeout2: '파이프 출구 온도 2',
  out_sensor_pipeout3: '파이프 출구 온도 3',
  out_sensor_pipeout4: '파이프 출구 온도 4',
  out_sensor_pipeout5: '파이프 출구 온도 5',
  // 운전 상태(ENUM)
  out_operation_odu_mode: '실외기 운전 상태',
  out_operation_heatcool: '냉난방',
  out_load_comp1: '압축기1',
  out_load_comp2: '압축기2',
  out_load_comp3: '압축기3',
  out_load_4way: '4-way 밸브',
  out_deice_step: '제상 단계',
  // 압축기 주파수(raw Hz)
  out_control_order_cfreq_comp2: '압축기2 지령 주파수',
  out_control_target_cfreq_comp2: '압축기2 목표 주파수',
  // 전기/전력(raw)
  out_sensor_ct1: '실외기 전류(CT1)',
  out_phase_current: '상 전류',
  out_sensor_voltage: '공급 전압',
  wattmeter_1min_sum: '순시 소비전력',
  wattmeter_all_unit_accum: '누적 전력량',
};

const MODBUS_LABELS: Record<string, string> = {
  host: '호스트',
  port: '포트',
  unit_id: '유닛 ID',
  connected: '연결 상태',
  temperature: '온도',
  humidity: '습도',
};

const LGAP_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  fan_speed: '풍량',
  swing_auto: '스윙 자동',
  locked: '잠금',
  plasma: '플라즈마',
  pipe_in_temperature: '입구 온도',
  pipe_out_temperature: '출구 온도',
  zone_load: '존 부하',
  zone_power: '존 전원',
  error_code: '에러 코드',
};

const LG_ICP02_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  fan_speed: '풍량',
  fan_motor_hz: '팬 모터 (Hz)',
  valve_open: '밸브 개도',
  pipe_temperature1_c: '배관 온도 1',
  pipe_temperature2_c: '배관 온도 2',
  compressor_cap: '압축기 용량',
  compressor_hz: '압축기 (Hz)',
  compressor_run: '압축기 가동',
  outdoor_active: '실외기 가동',
  heat_demand: '난방 요구',
  refrigerant_on: '냉매 순환',
  op_mode: '운전 상태',
};

const LG_ICP01_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  fan_speed: '풍량',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  inlet_temperature: '흡입 온도',
  outlet_temperature: '토출 온도',
  outdoor_temperature: '외기 온도',
  compressor_suction_temperature: '압축기 흡입 온도',
  compressor_discharge_temperature: '압축기 토출 온도',
  condenser_temperature_a: '응축기 온도 A',
  condenser_temperature_b: '응축기 온도 B',
  avg_temperature: '운전 평균 온도',
};

/**
 * 게이트웨이 수신 정보 · 메타데이터 카드의 라벨.
 *
 * 이 키들은 디바이스가 보고하는 속성이 아니라 **파생 항목**이라 접두사로 이름공간을
 * 나눈다(`gw.` / `meta.`). 접두사가 없으면 디바이스가 `rssi` 라는 속성을 실제로
 * 보고할 때 두 카드가 같은 키를 다투게 된다.
 */
const DERIVED_LABELS: Record<string, string> = {
  'gw.gateway_id': '게이트웨이 ID',
  'gw.rssi': 'RSSI',
  'gw.snr': 'SNR',
  'gw.channel': '채널',
  'gw.frequency_hz': '주파수',
  'gw.count': '수신 게이트웨이 수',
  'meta.name': '이름',
  'meta.id': '디바이스 ID',
  'meta.location': '위치',
  'meta.group': '그룹',
};

/**
 * 널리 쓰는 사용자 라벨 키의 표시 이름.
 *
 * 라벨 키는 사용자가 정하는 것이라 원칙적으로 그대로 보여 주지만, `dev_eui` 처럼
 * 표준에서 온 이름은 소문자·밑줄 그대로 두면 화면에서 읽히지 않는다.
 */
const KNOWN_LABEL_NAMES: Record<string, string> = {
  dev_eui: 'Device EUI',
  deveui: 'Device EUI',
  app_eui: 'App EUI',
  join_eui: 'Join EUI',
};

const COMMON_LABELS: Record<string, string> = {
  power: '전원',
  status: '상태',
  error_code: '에러 코드',
  online: '온라인',
  temperature: '온도',
  humidity: '습도',
  current_temperature: '현재 온도',
  target_temperature: '설정 온도',
  battery: '배터리',
  co2: 'CO2',
  pressure: '기압',
  illumination: '조도',
};

/** 디바이스 타입을 한글 표시명으로 변환. v0.18.3: HVACR.IDU/HVACR.ODU 신규 + 'indoor'/'outdoor' 레거시 호환. */
export function getDeviceTypeLabel(type: string): string {
  switch (type) {
    case 'HVACR.IDU':
    case 'indoor': // legacy
      return '실내기';
    case 'HVACR.ODU':
    case 'outdoor': // legacy
      return '실외기';
    case 'sensor':
      return '센서';
    case 'controller':
      return '컨트롤러';
    case 'gateway':
      return '게이트웨이';
    default:
      return type;
  }
}

/** 명령 이름 → 한국어 라벨 */
const COMMAND_LABELS: Record<string, string> = {
  target_temperature: '온도 설정',
  set_mode: '운전 모드',
  set_power: '전원',
  set_fan_speed: '풍량',
  write_register: '레지스터 쓰기',
  write_coil: '코일 쓰기',
};

/** 파라미터 이름 → 한국어 라벨 */
const PARAM_LABELS: Record<string, string> = {
  target_temperature: '설정 온도',
  mode: '모드',
  power: '전원',
  fan_speed: '풍량',
  address: '주소',
  value: '값',
};

/** 열거형 값 → 한국어 라벨 */
const ENUM_LABELS: Record<string, string> = {
  auto: '자동',
  cool: '냉방',
  cooling: '냉방',
  dry: '제습',
  dehumidify: '제습',
  fan: '송풍',
  heat: '난방',
  heating: '난방',
  low: '약',
  medium: '중',
  high: '강',
  quiet: '미풍',
  turbo: '터보',
};

/** snake_case 키를 Title Case로 변환. 예: target_temperature → Set Temperature */
export function humanizeKey(key: string): string {
  return key
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** 명령 이름을 한국어 라벨로 변환. */
export function getCommandLabel(name: string): string {
  return COMMAND_LABELS[name] ?? humanizeKey(name);
}

/** 파라미터 이름을 한국어 라벨로 변환. */
export function getParamLabel(name: string): string {
  return PARAM_LABELS[name] ?? humanizeKey(name);
}

/** 열거형 값을 한국어 라벨로 변환. */
export function getEnumLabel(value: string): string {
  return ENUM_LABELS[value] ?? value;
}

/** 속성 키를 한국어 라벨로 변환. 알 수 없는 키는 Title Case로 변환. */
export function getPropertyLabel(key: string, protocol?: string, type?: string): string {
  if (protocol === 'samsung_nasa' && (type === 'HVACR.IDU' || type === 'indoor')) {
    const label = SAMSUNG_NASA_INDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'samsung_nasa') {
    const label = SAMSUNG_NASA_OUTDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'modbus') {
    const label = MODBUS_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lgap') {
    const label = LGAP_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lg_icp02') {
    const label = LG_ICP02_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lg_icp01') {
    const label = LG_ICP01_LABELS[key];
    if (label) return label;
  }
  if (DERIVED_LABELS[key]) return DERIVED_LABELS[key];
  // meta.label.<키> — 널리 쓰는 키는 읽을 수 있는 이름으로 바꾸고, 나머지는 이름 그대로.
  if (key.startsWith('meta.label.')) {
    const raw = key.slice('meta.label.'.length);
    return KNOWN_LABEL_NAMES[raw.toLowerCase()] ?? raw;
  }
  // 대소문자는 가리지 않는다 — 같은 값을 `battery` 로 보내는 디바이스와 `Battery` 로
  // 보내는 디바이스가 한 화면에 섞이면 이름이 갈린다.
  return COMMON_LABELS[key] ?? COMMON_LABELS[key.toLowerCase()] ?? humanizeKey(key);
}

/** 속성 그리드가 카드를 만들 원본. 속성 외에 파생 항목의 재료도 함께 받는다. */
export interface DisplaySource {
  /** 디바이스 상태 속성 */
  properties?: Record<string, unknown>;
  /** 표시용 디바이스 식별자(메타데이터 카드의 "디바이스 ID") */
  id?: string;
  /** 사용자 메타데이터 */
  metadata?: {
    name?: string;
    tags?: string[];
    location?: string;
    group?: string;
    labels?: Record<string, string>;
  };
}

/** 값이 비었으면 '-' 로 채운다 — 빈 카드는 무엇을 보는 자리인지 알 수 없다. */
function orDash(v: string | undefined): string {
  return v && v !== '' ? v : '-';
}

/**
 * 게이트웨이 수신 정보 카드.
 *
 * 게이트웨이가 여럿일 수 있으나 이 패널은 "값 하나 = 카드 하나" 격자라 표를 얹을 수
 * 없다. 그래서 **첫 링크**(백엔드가 gateway_id 오름차순으로 결정적 정렬)의 값을 낸다.
 * 신호가 가장 센 링크를 고르면 업링크마다 다른 게이트웨이로 값이 튀어, 같은 카드가
 * 무엇을 가리키는지 알 수 없다. 링크가 여럿이면 `gw.count` 로 그 사실을 알린다.
 *
 * 각 카드의 갱신 시각은 그 링크의 마지막 수신 시각이다(측정치와 같은 규율).
 */
function buildGatewayEntries(properties: Record<string, unknown>): PropertyEntry[] {
  const links = extractGatewayLinks(properties);
  const link = links?.[0];
  const timeMs = link && link.last_seen_ms > 0 ? link.last_seen_ms : undefined;

  // 링크가 아직 없어도 항목은 낸다 — 첫 업링크 전에 수신 정보가 통째로 사라지면
  // 고를 수도, 자리를 잡아 둘 수도 없다. 값은 '-' 로, 시각은 '수신 전' 으로 나온다.
  const entries: PropertyEntry[] = [
    { id: 'gw.gateway_id', key: 'gw.gateway_id', value: link?.gateway_id, timeMs },
    { id: 'gw.rssi', key: 'gw.rssi', value: link?.rssi, timeMs },
    { id: 'gw.snr', key: 'gw.snr', value: link?.snr, timeMs },
    { id: 'gw.channel', key: 'gw.channel', value: link?.channel, timeMs },
    {
      id: 'gw.frequency_hz',
      key: 'gw.frequency_hz',
      // Hz 원값은 자릿수가 많아 카드에서 읽히지 않는다 — MHz 로 접어 보여 준다.
      value: link ? `${(link.frequency_hz / 1_000_000).toFixed(1)} MHz` : undefined,
      timeMs,
    },
  ];
  if (!links || links.length === 0) return entries;
  if (links.length > 1) {
    entries.push({ id: 'gw.count', key: 'gw.count', value: links.length });
  }
  return entries;
}

/**
 * 메타데이터 카드.
 *
 * 사용자가 지정한 고정 값이라 **갱신 시각을 붙이지 않는다** — 실시간으로 바뀌는 값이
 * 아니므로 시각을 달면 의미 없는 "몇 분 전"이 따라다닌다.
 */
function buildMetadataEntries(source: DisplaySource): PropertyEntry[] {
  const meta = source.metadata;
  const entries: PropertyEntry[] = [
    { id: 'meta.name', key: 'meta.name', value: orDash(meta?.name) },
    { id: 'meta.id', key: 'meta.id', value: orDash(source.id) },
    { id: 'meta.location', key: 'meta.location', value: orDash(meta?.location) },
    { id: 'meta.group', key: 'meta.group', value: orDash(meta?.group) },
  ];
  for (const [key, value] of Object.entries(meta?.labels ?? {})) {
    entries.push({ id: `meta.label.${key}`, key: `meta.label.${key}`, value: orDash(value) });
  }
  return entries;
}

/**
 * 파생 카드(게이트웨이 수신 정보 · 메타데이터)인지.
 *
 * 이 항목들은 디바이스가 보고하는 속성이 아니라 부가 정보라, **고른 경우에만** 그린다.
 * "전체" 기본값에 끼워 넣으면 이미 쓰고 있던 패널에 카드가 갑자기 늘어난다.
 */
/**
 * 이 항목이 **통신으로 채워지는가**.
 *
 * 메타데이터(`meta.*`)는 사용자가 적어 둔 값이라 디바이스가 보고하지 않는다. 그런 항목에
 * 갱신 시각을 붙이면 영영 오지 않을 무언가를 기다리는 것처럼 보인다 — 시각 자리는 비워
 * 두되 자리 자체는 남겨 카드 높이가 어긋나지 않게 한다.
 *
 * 게이트웨이 수신 정보(`gw.*`)는 파생이지만 업링크가 있어야 생기므로 통신이 필요하다.
 */
/** 속성 카드가 속한 그룹. */
export type PropertyGroup = 'basic' | 'status' | 'gateway';

export const PROPERTY_GROUPS: PropertyGroup[] = ['basic', 'status', 'gateway'];

/**
 * 항목이 속한 그룹.
 *
 * 셋은 성격이 다르다: 기본 정보는 사람이 적어 둔 값(통신과 무관), 상태 정보는 디바이스가
 * 보고하는 값, 수신 정보는 게이트웨이가 업링크를 받으며 남긴 값이다. 한데 섞어 두면
 * "이 값이 언제 것인지" 를 항목마다 다르게 읽어야 한다.
 */
export function propertyGroupOf(key: string): PropertyGroup {
  if (key.startsWith('meta.')) return 'basic';
  if (key.startsWith('gw.')) return 'gateway';
  return 'status';
}

export function needsReception(key: string): boolean {
  return !key.startsWith('meta.');
}

export function isDerivedPropertyKey(key: string): boolean {
  return key.startsWith('gw.') || key.startsWith('meta.');
}

/**
 * 속성 그리드가 그릴 카드 항목을 만든다.
 *
 * 표시 순서 정렬 → 전용 섹션 키 제외 → measurements 전개에 더해, 게이트웨이 수신
 * 정보와 메타데이터를 파생 카드로 붙인다.
 *
 * 패널과 설정 화면이 이 함수 하나를 공유하므로 "고를 수 있는 항목" 과 "그려지는
 * 카드" 가 갈라질 수 없다 — 종전에는 양쪽이 각자의 규칙을 갖고 있었다.
 */
export function buildDisplayEntries(source: DisplaySource | undefined): PropertyEntry[] {
  if (!source) return [];
  const properties = source.properties ?? {};
  return [
    ...expandMeasurementEntries(
      excludeDedicatedSectionKeys(sortProperties(Object.entries(properties))),
    ),
    ...buildGatewayEntries(properties),
    ...buildMetadataEntries(source),
  ];
}

/**
 * 이 디바이스가 **속성 그리드에 실제로 그리는** 항목의 키 목록.
 *
 * 설정의 "표시 항목" 과 패널의 카드는 반드시 같아야 한다. 종전에는 둘이 서로 다른
 * 규칙으로 목록을 만들어 어긋났다:
 *   - 프로토콜 라벨표로 채워, 그 디바이스가 보고하지 않는 항목까지 고를 수 있었다.
 *   - `measurements` 컨테이너와 그 하위(온도·습도)가 함께 떴다.
 *   - 전용 섹션이 그리는 키(`gateways`)도 목록에 남아, 골라도 카드가 되지 않았다.
 *
 * 그래서 **패널이 카드를 만드는 그 경로**를 그대로 쓴다 — 정렬·전용 섹션 제외·
 * measurements 전개까지 같은 함수를 거치므로 둘이 갈라질 수 없다.
 */
export function listDisplayableProperties(source: DisplaySource | undefined): string[] {
  const entries = buildDisplayEntries(source);
  const seen = new Set<string>();
  const keys: string[] = [];
  for (const { key } of entries) {
    if (seen.has(key)) continue;
    seen.add(key);
    keys.push(key);
  }
  return keys;
}

/** 속성 표시 우선순위. 목록에 없는 키는 맨 뒤에 원래 순서대로 표시. */
const PROPERTY_ORDER: string[] = [
  // 제어 순서: 전원 → 운전 모드 → 온도 → 풍량 → 고정 설치
  'power',
  'mode',
  'target_temperature',
  'current_temperature',
  'fan_speed',
  // 고정 설치/상태
  'swing_vertical',
  'swing_auto',
  'locked',
  'plasma',
  'filter_alarm',
  // 센서/배관
  'fan_motor_hz',
  'valve_open',
  'pipe_temperature1_c',
  'pipe_temperature2_c',
  'pipe_in_temperature',
  'pipe_out_temperature',
  'zone_load',
  'zone_power',
  // 컨트롤러/실외기
  'compressor_cap',
  'compressor_hz',
  'compressor_run',
  'outdoor_active',
  'heat_demand',
  'refrigerant_on',
  'op_mode',
  // Samsung 실외기(ODU) 텔레메트리 — outdoorFieldRegistry 순서.
  'out_operation_odu_mode',
  'out_operation_heatcool',
  'out_load_comp1',
  'out_load_comp2',
  'out_load_comp3',
  'out_load_4way',
  'out_deice_step',
  'outdoor_temperature',
  'compressor_discharge_temperature',
  'out_sensor_pipein3',
  'out_sensor_pipein4',
  'out_sensor_pipein5',
  'out_sensor_pipeout1',
  'out_sensor_pipeout2',
  'out_sensor_pipeout3',
  'out_sensor_pipeout4',
  'out_sensor_pipeout5',
  'out_control_order_cfreq_comp2',
  'out_control_target_cfreq_comp2',
  'out_sensor_ct1',
  'out_phase_current',
  'out_sensor_voltage',
  'wattmeter_1min_sum',
  'wattmeter_all_unit_accum',
  // 에러
  'error_code',
];

/** 속성 엔트리를 표시 우선순위에 따라 정렬한다. */
/** 속성값을 사람이 읽을 수 있는 문자열로 포맷. */
// v0.7.5+ hvac 통일 ID (int) → 영문 enum 키. mode / fan_speed 가 백엔드에서
// int 로 emit 되므로 한국어 라벨 변환 전에 enum 키로 정규화한다.
const MODE_ID_TO_ENUM: Record<number, string> = {
  0: 'auto', 1: 'cool', 2: 'heat', 3: 'dry', 4: 'fan',
};
const FAN_SPEED_ID_TO_ENUM: Record<number, string> = {
  0: 'auto', 1: 'auto', 2: 'quiet', 3: 'low', 4: 'medium', 5: 'high', 6: 'turbo',
};

// 전원 OFF 시 값이 신뢰할 수 없는(자동/0°C 등으로 정규화된 기본값) 운전 계열 속성 키.
// power=false 이면 이 필드들은 실제 값이 아니므로 '-' 로 표시한다. 진단 필드
// (filter_alarm/error_code)와 전원(power) 자체는 전원과 무관한 상태라 그대로 표시한다.
const POWER_OFF_UNRELIABLE_KEYS = new Set<string>([
  'mode',
  'target_temperature',
  'current_temperature',
  'fan_speed',
  'swing_vertical',
]);

/**
 * 값과 단위 사이의 간격.
 *
 * `°C` · `%` 처럼 기호로 시작하는 단위는 붙여 쓰고, `ppm` · `V` 처럼 글자로 시작하면
 * 한 칸 띄운다 — 둘을 같게 두면 어느 한쪽이 늘 어색하다.
 */
function unitSeparator(unit: string): string {
  return /^[^\p{L}\p{N}]/u.test(unit) ? '' : ' ';
}

/**
 * 키가 원래 갖는 단위.
 *
 * 종전에는 이름에 붙여 두었다(`RSSI (dBm)`). 이름과 단위가 한 덩어리면 단위만 바꿀 수
 * 없고, 이름을 바꾸면 단위가 함께 사라진다. 이름에서 떼어 값 쪽에 둔다 — 사용자가 정한
 * 단위가 있으면 그것이 이긴다.
 */
const DEFAULT_UNITS: Record<string, string> = {
  'gw.rssi': 'dBm',
  'gw.snr': 'dB',
};

/** 키가 원래 갖는 단위. 없으면 `undefined`. */
export function defaultUnitOf(key: string): string | undefined {
  return DEFAULT_UNITS[key];
}

export function formatPropertyValue(
  key: string,
  value: unknown,
  opts?: { powerOff?: boolean; unit?: string },
): string {
  // 전원 OFF 시 운전 계열 값은 정규화된 기본값이라 실제 값이 아니므로 '-' 로 표시.
  if (opts?.powerOff && POWER_OFF_UNRELIABLE_KEYS.has(key)) return '-';
  if (value === null || value === undefined) return '-';
  // 사용자가 단위를 정했으면 그것만 붙인다 — 키 이름으로 짐작한 단위(온도의 °C)와
  // 겹쳐 "26.4°C ppm" 같은 것이 되지 않게, 짐작을 건너뛴다.
  const unit = opts?.unit?.trim() || DEFAULT_UNITS[key];
  if (unit && typeof value === 'number') return `${value}${unitSeparator(unit)}${unit}`;
  if (typeof value === 'number') {
    // mode / fan_speed 는 hvac 통일 ID (int) — enum 키로 변환 후 라벨링.
    if (key === 'mode') {
      const enumKey = MODE_ID_TO_ENUM[value];
      if (enumKey) return ENUM_LABELS[enumKey] ?? enumKey;
    }
    if (key === 'fan_speed') {
      const enumKey = FAN_SPEED_ID_TO_ENUM[value];
      if (enumKey) return ENUM_LABELS[enumKey] ?? enumKey;
    }
    const lowerKey = key.toLowerCase();
    if (lowerKey.includes('temp')) return `${value}\u00B0C`;
    return String(value);
  }
  if (typeof value === 'boolean') return value ? 'ON' : 'OFF';
  // 중첩 객체/배열은 String() 이 "[object Object]" 를 만들므로 compact JSON 으로 폴백한다.
  // (예: chirpstack 의 measurements 처럼 예상치 못한 중첩 값이 들어오는 경우)
  // 측정치는 expandMeasurementEntries 로 개별 항목으로 펼치는 것이 우선이며, 이 분기는
  // 그 경로를 타지 않는 모든 위치(이력 테이블 셀 등)를 위한 방어적 폴백이다.
  if (typeof value === 'object') return stringifyUnknownObject(value);
  const str = String(value);
  // 운전 모드, 풍량 등 enum 값을 한국어로 변환
  return ENUM_LABELS[str] ?? str;
}

/** 객체 폴백 표시 최대 길이. 초과분은 말줄임한다(그리드 셀/표 셀 레이아웃 보호). */
const OBJECT_FALLBACK_MAX_LEN = 80;

/**
 * 알 수 없는 객체/배열을 compact JSON 문자열로 변환한다.
 * 순환 참조 등으로 직렬화가 실패하면 타입 표기로 폴백한다(예외를 던지지 않는다).
 */
function stringifyUnknownObject(value: object): string {
  let json: string;
  try {
    json = JSON.stringify(value);
  } catch {
    return Array.isArray(value) ? '[…]' : '{…}';
  }
  // JSON.stringify 는 undefined/함수 등에서 undefined 를 반환할 수 있다.
  if (json === undefined) return '-';
  if (json.length <= OBJECT_FALLBACK_MAX_LEN) return json;
  return `${json.slice(0, OBJECT_FALLBACK_MAX_LEN)}…`;
}

/** 속성 그리드에 표시할 단일 항목. measurements 는 측정치별로 펼쳐진다. */
export interface PropertyEntry {
  /** React key 용 고유 식별자 (measurements 전개 항목은 "measurements.<이름>"). */
  id: string;
  /** 라벨/포맷 조회에 사용할 키 (측정치 이름 또는 원본 속성 키). */
  key: string;
  value: unknown;
  /** 측정치별 갱신 시각(epoch ms). 값이 없거나 형식이 다르면 undefined. */
  timeMs?: number;
}

/** measurements 하위 값이 {value, time_ms} 형태인지 판별한다. */
function isMeasurementRecord(v: unknown): v is { value: unknown; time_ms?: unknown } {
  return typeof v === 'object' && v !== null && !Array.isArray(v) && 'value' in v;
}

/**
 * 속성 엔트리 목록에서 `measurements` 를 측정치별 개별 항목으로 펼친다.
 *
 * chirpstack 디바이스 로스터는 측정치를 다음 형태로 내보낸다:
 *   measurements: { temperature: { value: 29.8, time_ms: 1786491121129 }, ... }
 * 이를 하나의 불투명한 카드가 아니라 측정치별 카드로 렌더하기 위한 순수 변환이며,
 * DeviceDetailPanel(GenericPropertiesGrid)과 대시보드 PropertiesGridPanel 이 공유한다.
 *
 * 예외 상황 처리:
 *   - measurements 없음 → 원본 그대로
 *   - measurements 가 빈 객체 → 항목 0개 (빈 카드/[object Object] 를 만들지 않음)
 *   - measurements 가 객체가 아님(문자열 등) → 원본 항목 그대로 유지
 *   - 하위 값이 {value, time_ms} 가 아닌 단순 스칼라 → 값만 사용하고 시각은 생략
 */
export function expandMeasurementEntries(entries: [string, unknown][]): PropertyEntry[] {
  const out: PropertyEntry[] = [];
  for (const [key, value] of entries) {
    if (key !== 'measurements' || typeof value !== 'object' || value === null || Array.isArray(value)) {
      out.push({ id: key, key, value });
      continue;
    }
    for (const [name, raw] of Object.entries(value as Record<string, unknown>)) {
      if (isMeasurementRecord(raw)) {
        out.push({
          id: `measurements.${name}`,
          key: name,
          value: raw.value,
          timeMs: typeof raw.time_ms === 'number' ? raw.time_ms : undefined,
        });
      } else {
        // 구형 데이터/타 프로바이더: 시각 없이 값만 있는 형태.
        out.push({ id: `measurements.${name}`, key: name, value: raw });
      }
    }
  }
  return out;
}

// ---- 게이트웨이 링크(chirpstack `gateways`) ----

/**
 * 이 디바이스를 수신한 게이트웨이 1건 — **디바이스 축(device axis)** 엔트리.
 *
 * 백엔드는 게이트웨이 로스터를 두 축으로 노출한다:
 *   - 게이트웨이 축: `list_gateways` → ChirpstackGateway.devices[] (게이트웨이 아래 디바이스)
 *   - 디바이스 축: device `state.properties.gateways[]` (디바이스 아래 게이트웨이) ← 이 타입
 *
 * 8개 필드의 JSON 태그가 같지만 `ChirpstackGatewayDevice` 와 타입을 공유하지 않는다.
 * 그쪽은 디바이스 신원(dev_eui/device_id/device_name/device_profile_name)을 갖고 이쪽은
 * 갖지 않으며, 두 페이로드는 서로 다른 백엔드 경로에서 독립적으로 진화한다.
 * `Omit<ChirpstackGatewayDevice, ...>` 로 파생하면 게이트웨이 축에 필드가 하나
 * 추가될 때 디바이스 축 타입이 조용히 따라 바뀌어 실제 와이어와 어긋난다.
 * (표시 포맷은 lib/utils/format.ts 의 공용 포맷터로 통일해 표기 불일치를 막는다.)
 */
export interface DeviceGatewayLink {
  gateway_id: string;
  /** 이 게이트웨이가 수신한 신호 세기(dBm). */
  rssi: number;
  /** 이 게이트웨이가 수신한 SNR(dB). 와이어 값은 고정 소수가 아니다. */
  snr: number;
  /**
   * 수신 게이트웨이의 concentrator IF 채널 인덱스 — **게이트웨이 로컬 하드웨어 값**이다.
   * 주파수가 아니며 게이트웨이 간 비교할 수 없다. 실제 RF 주파수는 `frequency_hz`.
   */
  channel: number;
  /** 프레임 레벨 RF 주파수(Hz). 업링크에 txInfo 가 없으면 0 일 수 있다. */
  frequency_hz: number;
  spreading_factor: number;
  bandwidth: number;
  /** epoch milliseconds. */
  last_seen_ms: number;
  /** 서버가 호출 시점에 파생한 값. 클라이언트에서 재계산하지 않는다. */
  stale: boolean;
}

/**
 * 전용 섹션이 따로 렌더하므로 일반 key/value 그리드에서 제외할 속성 키.
 * 제외하지 않으면 객체 폴백(stringifyUnknownObject)을 타 JSON 덩어리로 표시된다.
 */
const DEDICATED_SECTION_KEYS = new Set<string>(['gateways']);

/** 전용 섹션이 담당하는 키를 속성 엔트리 목록에서 제거한다. */
export function excludeDedicatedSectionKeys<T>(entries: [string, T][]): [string, T][] {
  return entries.filter(([key]) => !DEDICATED_SECTION_KEYS.has(key));
}

/**
 * 숫자 필드 정규화. 값이 없거나 숫자가 아니면 0 으로 떨어뜨린다.
 *
 * 백엔드 설계상 `channel`/`frequency_hz`/`spreading_factor`/`bandwidth` 의 0 은
 * "없음"과 "실제 0" 을 구분하지 않는다(proto3 value-type 파싱). 여기서도 같은 규칙을
 * 적용해 렌더 계층이 undefined/NaN 을 따로 다루지 않도록 한다.
 */
function numOrZero(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

/** 알 수 없는 값 1건을 게이트웨이 링크로 정규화한다. gateway_id 가 없으면 버린다. */
function toGatewayLink(v: unknown): DeviceGatewayLink | null {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) return null;
  const o = v as Record<string, unknown>;
  if (typeof o.gateway_id !== 'string' || o.gateway_id === '') return null;
  return {
    gateway_id: o.gateway_id,
    rssi: numOrZero(o.rssi),
    snr: numOrZero(o.snr),
    channel: numOrZero(o.channel),
    frequency_hz: numOrZero(o.frequency_hz),
    spreading_factor: numOrZero(o.spreading_factor),
    bandwidth: numOrZero(o.bandwidth),
    last_seen_ms: numOrZero(o.last_seen_ms),
    stale: o.stale === true,
  };
}

/**
 * 디바이스 상태 속성에서 게이트웨이 링크 목록을 추출한다.
 *
 * 아직 어느 게이트웨이에서도 수신되지 않은 디바이스는 `gateways` 키 자체가 없다
 * (빈 배열이 아니다). 표시할 링크가 하나도 없으면 `undefined` 를 반환해 호출부가
 * 빈 껍데기 섹션을 만들지 않도록 한다.
 *
 * 백엔드는 `gateway_id` 오름차순으로 결정적 정렬해 내려주며 그 순서를 유지한다
 * (신호 세기 정렬은 업링크마다 행이 뒤바뀌어 특정 게이트웨이를 추적할 수 없다).
 */
export function extractGatewayLinks(
  properties: Record<string, unknown>,
): DeviceGatewayLink[] | undefined {
  const raw = properties['gateways'];
  if (!Array.isArray(raw)) return undefined;
  const links: DeviceGatewayLink[] = [];
  for (const item of raw) {
    const link = toGatewayLink(item);
    if (link) links.push(link);
  }
  return links.length > 0 ? links : undefined;
}

export function sortProperties<T>(entries: [string, T][]): [string, T][] {
  return entries.slice().sort((a, b) => {
    const ia = PROPERTY_ORDER.indexOf(a[0]);
    const ib = PROPERTY_ORDER.indexOf(b[0]);
    // 목록에 없는 키는 뒤로
    const oa = ia === -1 ? PROPERTY_ORDER.length : ia;
    const ob = ib === -1 ? PROPERTY_ORDER.length : ib;
    return oa - ob;
  });
}

/** 커맨드 표시 우선순위: 전원 → 운전 모드 → 온도 → 풍량 → 고정 설치 */
const COMMAND_ORDER: string[] = [
  'set_power',
  'set_mode',
  'target_temperature',
  'set_fan_speed',
  'set_swing',
  'set_lock',
  'set_plasma',
];

/** 커맨드를 표시 우선순위에 따라 정렬한다. */
export function sortCommands<T extends { name: string }>(commands: T[]): T[] {
  return commands.slice().sort((a, b) => {
    const ia = COMMAND_ORDER.indexOf(a.name);
    const ib = COMMAND_ORDER.indexOf(b.name);
    const oa = ia === -1 ? COMMAND_ORDER.length : ia;
    const ob = ib === -1 ? COMMAND_ORDER.length : ib;
    return oa - ob;
  });
}
