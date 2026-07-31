// 스케줄 로그 저장 백엔드(storage backend) 설정 API 서비스.
//
// 스케줄 실행 로그를 어디에 저장할지(sqlite/file/memory)를 조회/변경한다.
// 값은 서버 config 오버라이드에 영속화되며 재시작 후 적용된다. admin 전용 엔드포인트다.
//
//   GET /system/schedule-log-config → data: { storage_type }
//   PUT /system/schedule-log-config → body { storage_type } → data: { applied, needs_restart }
//
// 참고: client.ts 인터셉터가 성공 envelope 의 data 를 이미 언래핑하므로
// get/put 의 반환값은 곧 data 페이로드다.

import { get, put } from './client';

/** 선택 가능한 저장 방식(백엔드가 허용하는 값). */
export const SCHEDULE_LOG_STORAGE_TYPES = ['sqlite', 'file', 'memory'] as const;

/** 저장 방식 유니온 타입. */
export type ScheduleLogStorageType = (typeof SCHEDULE_LOG_STORAGE_TYPES)[number];

/** 문자열이 알려진 저장 방식인지 좁히는 타입 가드. */
export function isScheduleLogStorageType(value: string): value is ScheduleLogStorageType {
  return (SCHEDULE_LOG_STORAGE_TYPES as readonly string[]).includes(value);
}

/** GET 응답 data 형태(빈 값 대비). */
interface ScheduleLogConfigResponse {
  storage_type?: string | null;
}

/** PUT 응답 data 형태. */
interface ScheduleLogConfigUpdateResponse {
  applied?: string;
  needs_restart?: boolean;
}

/**
 * 현재 스케줄 로그 저장 방식을 조회한다(admin 전용).
 *
 * 응답 data 가 null 이거나 storage_type 이 비어 있으면 기본값 "sqlite" 로 정규화한다.
 */
export async function getScheduleLogStorageType(): Promise<string> {
  const data = await get<ScheduleLogConfigResponse | null>('/system/schedule-log-config');
  const storageType = data?.storage_type;
  return storageType != null && storageType.length > 0 ? storageType : 'sqlite';
}

/**
 * 스케줄 로그 저장 방식을 변경한다(admin 전용).
 *
 * 재시작 후 적용되며, needs_restart 여부를 정규화해 반환한다(누락 시 false).
 */
export async function setScheduleLogStorageType(
  storageType: ScheduleLogStorageType,
): Promise<{ needsRestart: boolean }> {
  const data = await put<ScheduleLogConfigUpdateResponse | null>('/system/schedule-log-config', {
    storage_type: storageType,
  });
  return { needsRestart: data?.needs_restart ?? false };
}
