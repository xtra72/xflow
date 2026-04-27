// Re-export the underlying axios instance and typed helpers.
export { apiClient, del, get, getList, post, put } from './client';

// Re-export interceptor setup (called once at app bootstrap).
export { setupInterceptors } from './interceptors';

// Re-export domain services.
export * as authService from './authService';
export * as flowService from './flowService';
export * as nodeService from './nodeService';
export * as agentService from './agentService';
export * as monitorService from './monitorService';
