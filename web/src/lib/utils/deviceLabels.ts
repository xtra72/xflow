// 디바이스 속성 키를 한국어 라벨로 변환하는 유틸리티.

import type { DeviceInfo } from '@/types/device';

/** 디바이스 표시명을 결정한다. metadata.name > name > id 순으로 폴백. */
export function getDeviceDisplayName(device: DeviceInfo): string {
  if (device.metadata?.name) return device.metadata.name;
  if (device.name) return device.name;
  return device.id;
}

const NASA_INDOOR_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temp: '설정 온도',
  current_temp: '현재 온도',
  fan_speed: '풍량',
  swing_vertical: '상하 스윙',
  filter_alarm: '필터 알람',
  error_code: '에러 코드',
};

const NASA_OUTDOOR_LABELS: Record<string, string> = {
  power: '전원',
  current_temp: '현재 온도',
  error_code: '에러 코드',
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
  target_temp: '설정 온도',
  current_temp: '현재 온도',
  fan_speed: '풍량',
  swing_auto: '스윙 자동',
  locked: '잠금',
  plasma: '플라즈마',
  pipe_in_temp: '입구 온도',
  pipe_out_temp: '출구 온도',
  zone_load: '존 부하',
  zone_power: '존 전원',
  error_code: '에러 코드',
};

const LGCP_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temp: '설정 온도',
  current_temp: '현재 온도',
  fan_speed: '풍량',
  fan_motor_hz: '팬 모터 (Hz)',
  valve_open: '밸브 개도',
  pipe_temp1_c: '배관 온도 1',
  pipe_temp2_c: '배관 온도 2',
  compressor_cap: '압축기 용량',
  compressor_hz: '압축기 (Hz)',
  compressor_run: '압축기 가동',
  outdoor_active: '실외기 가동',
  heat_demand: '난방 요구',
  refrigerant_on: '냉매 순환',
  op_mode: '운전 상태',
};

const LGCNP_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  fan_speed: '풍량',
  target_temp: '설정 온도',
  current_temp: '현재 온도',
  inlet_temp: '흡입 온도',
  outlet_temp: '토출 온도',
  op_mode: '운전 모드 (원시)',
  status_flags: '상태 플래그 (원시)',
  outdoor_temp: '외기 온도',
  outdoor_temp_b: '외기 온도 B',
  compressor_flag: '압축기 플래그',
};

const COMMON_LABELS: Record<string, string> = {
  power: '전원',
  status: '상태',
  error_code: '에러 코드',
  online: '온라인',
  temperature: '온도',
  humidity: '습도',
  current_temp: '현재 온도',
  target_temp: '설정 온도',
};

/** 디바이스 타입을 한글 표시명으로 변환. */
export function getDeviceTypeLabel(type: string): string {
  switch (type) {
    case 'indoor':
      return '실내기';
    case 'outdoor':
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
  set_temperature: '온도 설정',
  set_mode: '운전 모드',
  set_power: '전원',
  set_fan_speed: '풍량',
  write_register: '레지스터 쓰기',
  write_coil: '코일 쓰기',
};

/** 파라미터 이름 → 한국어 라벨 */
const PARAM_LABELS: Record<string, string> = {
  target_temp: '설정 온도',
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
  dry: '제습',
  fan: '송풍',
  heat: '난방',
  low: '약',
  medium: '중',
  high: '강',
};

/** snake_case 키를 Title Case로 변환. 예: set_temperature → Set Temperature */
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
  if (protocol === 'nasa' && type === 'indoor') {
    const label = NASA_INDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'nasa') {
    const label = NASA_OUTDOOR_LABELS[key];
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
  if (protocol === 'lgcp') {
    const label = LGCP_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lgcnp') {
    const label = LGCNP_LABELS[key];
    if (label) return label;
  }
  return COMMON_LABELS[key] ?? humanizeKey(key);
}

/** 속성 표시 우선순위. 목록에 없는 키는 맨 뒤에 원래 순서대로 표시. */
const PROPERTY_ORDER: string[] = [
  // 제어 순서: 전원 → 운전 모드 → 온도 → 풍량 → 고정 설치
  'power',
  'mode',
  'target_temp',
  'current_temp',
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
  'pipe_temp1_c',
  'pipe_temp2_c',
  'pipe_in_temp',
  'pipe_out_temp',
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
  // 에러
  'error_code',
];

/** 속성 엔트리를 표시 우선순위에 따라 정렬한다. */
/** 속성값을 사람이 읽을 수 있는 문자열로 포맷. */
export function formatPropertyValue(key: string, value: unknown): string {
  if (value === null || value === undefined) return '-';
  if (typeof value === 'number') {
    const lowerKey = key.toLowerCase();
    if (lowerKey.includes('temp')) return `${value}\u00B0C`;
    return String(value);
  }
  if (typeof value === 'boolean') return value ? 'ON' : 'OFF';
  const str = String(value);
  // 운전 모드, 풍량 등 enum 값을 한국어로 변환
  return ENUM_LABELS[str] ?? str;
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
  'set_temperature',
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
