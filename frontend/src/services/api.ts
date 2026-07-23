import { getAccessToken } from "./tokenStorage";

const API_URL = "http://localhost:8080";

export async function apiFetch<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);

  headers.set("Content-Type", "application/json");

  const accessToken = getAccessToken();

  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  const response = await fetch(`${API_URL}${endpoint}`, {
    ...options,
    credentials: "include",
    headers,
  });

  if (!response.ok) {
    throw new Error("Request failed");
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return response.json();
}
