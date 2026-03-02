// API response types matching Go DTO structs in internal/api/dto/response.go

/**
 * Standard API error detail.
 * Maps to Go ErrorDetail struct.
 */
export interface ErrorDetail {
  code: string;
  message: string;
  details?: unknown;
}

/**
 * Pagination metadata for list responses.
 * Maps to Go PaginationMeta struct.
 */
export interface PaginationMeta {
  page: number;
  size: number;
  total: number;
  total_pages: number;
}

/**
 * Response metadata envelope.
 * Maps to Go Meta struct.
 */
export interface Meta {
  request_id?: string;
  pagination?: PaginationMeta;
}

/**
 * Generic API response envelope.
 * All API responses are wrapped in this structure.
 * Maps to Go APIResponse[T] struct.
 */
export interface APIResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: ErrorDetail;
  meta?: Meta;
}

/**
 * Pagination query parameters for list requests.
 * Maps to Go PaginationParams struct.
 */
export interface PaginationParams {
  page?: number;
  size?: number;
}

/**
 * List query options extending pagination with sort/filter.
 * Maps to Go ListOptions struct.
 */
export interface ListOptions extends PaginationParams {
  sort?: string;
  filter?: string;
  status?: string;
}

/**
 * Custom error class for API errors.
 * Provides structured error information from API responses.
 */
export class APIError extends Error {
  readonly code: string;
  readonly status: number;
  readonly details?: unknown;

  constructor(code: string, message: string, status: number, details?: unknown) {
    super(message);
    this.name = 'APIError';
    this.code = code;
    this.status = status;
    this.details = details;
  }
}
