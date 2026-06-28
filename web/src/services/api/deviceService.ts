import type {
  DeviceDetail,
  DeviceExecuteRequest,
  DeviceHistoryResponse,
  DeviceInfo,
  DeviceListParams,
  DeviceMetadataUpdateRequest,
} from '@/types/device';

import { del, get, getList, post, put } from './client';

// ---- Queries ----

/**
 * List devices with optional filters.
 */
export async function getDevices(
  params?: DeviceListParams,
): Promise<{ data: DeviceInfo[]; total: number }> {
  return getList<DeviceInfo>('/devices', { params });
}

/**
 * Get a single device detail by ID (includes state and commands).
 */
export async function getDevice(id: string): Promise<DeviceDetail> {
  return get<DeviceDetail>(`/devices/${id}`);
}

/**
 * Get recent device data history (periodic snapshots, newest first).
 *
 * GET /devices/{id}/history?limit=N
 *
 * 서버가 limit 을 max(MaxEntries)로 clamp 하므로 상한 초과는 안전하다.
 * 이력이 없으면 entries:[] 를 반환한다. 이력 비활성 구성이면 404(APIError)로 전파된다.
 *
 * @param id - 디바이스 식별자 (uid/UUID 우선)
 * @param limit - 조회 개수 (생략/0 이하면 서버 기본값 사용)
 */
export async function getDeviceHistory(
  id: string,
  limit?: number,
): Promise<DeviceHistoryResponse> {
  const params = limit && limit > 0 ? { limit } : undefined;
  return get<DeviceHistoryResponse>(`/devices/${id}/history`, { params });
}

// ---- Commands ----

/**
 * Execute a command on a device.
 */
export async function executeCommand(
  id: string,
  req: DeviceExecuteRequest,
): Promise<unknown> {
  return post<unknown>(`/devices/${id}/execute`, req);
}

// ---- Metadata ----

/**
 * Update device metadata (tags, location, group, labels).
 */
export async function updateMetadata(
  id: string,
  metadata: DeviceMetadataUpdateRequest,
): Promise<void> {
  await put<void>(`/devices/${id}/metadata`, metadata);
}

/**
 * Delete device metadata.
 */
export async function deleteMetadata(id: string): Promise<void> {
  await del(`/devices/${id}/metadata`);
}
