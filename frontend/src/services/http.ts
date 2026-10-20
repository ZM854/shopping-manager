import { logger } from '../shared/logger';

const API_URL = (import.meta.env.VITE_API_URL || '/api').replace(/\/+$/, '');

export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly fieldErrors?: Record<string, string>;
  constructor(
    status: number,
    message = 'Request failed',
    code?: string,
    fieldErrors?: Record<string, string>,
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fieldErrors = fieldErrors;
  }
}

export async function responseError(response: Response): Promise<ApiError> {
  const body: unknown = await response.json().catch(() => null);
  if (
    body &&
    typeof body === 'object' &&
    'code' in body &&
    typeof body.code === 'string' &&
    'message' in body &&
    typeof body.message === 'string'
  ) {
    const fields = 'fieldErrors' in body ? body.fieldErrors : undefined;
    const fieldErrors =
      fields && typeof fields === 'object' && !Array.isArray(fields)
        ? Object.fromEntries(
            Object.entries(fields).filter(
              ([, value]) => typeof value === 'string',
            ),
          )
        : undefined;
    return new ApiError(response.status, body.message, body.code, fieldErrors);
  }
  return new ApiError(response.status);
}

export async function performRequest(
  endpoint: string,
  options: RequestInit = {},
  accessToken: string | null = null,
): Promise<Response> {
  const headers = new Headers(options.headers);
  headers.set('Content-Type', 'application/json');
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`);
  logger.info('API', `${options.method || 'GET'} ${endpoint}`);
  // Auth uses this transport directly: it never triggers an automatic refresh.
  return fetch(`${API_URL}${endpoint}`, {
    ...options,
    credentials: 'include',
    headers,
  });
}

export async function readResponse<T>(response: Response): Promise<T> {
  if (!response.ok) throw await responseError(response);
  if (response.status === 204) return undefined as T;
  return response.json();
}

export async function requestJSON<T>(
  endpoint: string,
  options?: RequestInit,
): Promise<T> {
  return readResponse<T>(await performRequest(endpoint, options));
}
