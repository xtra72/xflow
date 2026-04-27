import type {
  DeviceDetail,
  DeviceExecuteRequest,
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
