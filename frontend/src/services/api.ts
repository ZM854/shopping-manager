const API_URL = "http://localhost:8080";

export async function apiFetch<T>(
  endpoint: string,
  options?: RequestInit,
): Promise<T> {
  const response = await fetch(`${API_URL}${endpoint}`, options);

  if (!response.ok) {
    throw new Error("Request failed");
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return response.json();
}
