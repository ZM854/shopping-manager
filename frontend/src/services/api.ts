import { logger } from "../shared/logger";
import { authService } from "./authService";
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "./tokenStorage";

const API_URL = import.meta.env.VITE_API_URL;
const TAG = "API";

let refreshPromise: Promise<void> | null = null;

interface ApiFetchOptions extends RequestInit {
  skipRefresh?: boolean;
}

async function refreshAccessToken(): Promise<void> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      try {
        const response = await authService.refresh();
        setAccessToken(response.accessToken);
      } finally {
        refreshPromise = null;
      }
    })();
  }

  return refreshPromise;
}

async function performRequest(
  endpoint: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);

  headers.set("Content-Type", "application/json");

  const accessToken = getAccessToken();

  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  return fetch(`${API_URL}${endpoint}`, {
    ...options,
    credentials: "include",
    headers,
  });
}

export async function apiFetch<T>(
  endpoint: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const { skipRefresh = false, ...fetchOptions } = options;
  const method = (fetchOptions.method || "GET").toUpperCase();

  logger.info(TAG, `--> ${method} ${endpoint}`);

  let response: Response;

  try {
    response = await performRequest(endpoint, fetchOptions);
  } catch (error) {
    logger.error(TAG, `<-- NETWORK ERROR ${method} ${endpoint}`, error);
    throw error;
  }

  if (response.status === 401 && !skipRefresh) {
    logger.info(TAG, "attempt to refresh token");
    try {
      await refreshAccessToken();
      response = await performRequest(endpoint, fetchOptions);
      logger.info(TAG, "token refreshed successfully");
    } catch (error) {
      logger.error(
        TAG,
        `<-- FAIL ${method} ${endpoint} [${response.status} ${response.statusText}`,
        error,
      );
      clearAccessToken();
      throw error;
    }
  }

  if (!response.ok) {
    logger.error(
      TAG,
      `<-- FAIL ${method} ${endpoint} [${response.status} ${response.statusText}]`,
    );
    throw new Error("Request failed");
  }

  logger.debug(
    TAG,
    `<-- SUCCESS ${method} ${endpoint} [${response.status} ${response.statusText}]`,
  );

  if (response.status === 204) {
    return undefined as T;
  }
  return response.json();
}
