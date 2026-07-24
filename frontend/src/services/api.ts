import { authService } from "./authService";
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "./tokenStorage";

const API_URL = "http://localhost:8080";

let refreshPromise: Promise<void> | null = null;

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
  options: RequestInit = {},
): Promise<T> {
  let response = await performRequest(endpoint, options);

  if (response.status === 401) {
    try {
      await refreshAccessToken();
      response = await performRequest(endpoint, options);
    } catch {
      clearAccessToken();
      throw new Error("Unauthorized");
    }
  }

  if (!response.ok) {
    throw new Error("Request failed");
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return response.json();
}
