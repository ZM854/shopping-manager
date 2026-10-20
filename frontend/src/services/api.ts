import { performRequest, readResponse } from './http';
import { sessionCoordinator } from './session';
import { getAccessToken } from './tokenStorage';

export { ApiError } from './http';

export async function apiFetch<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const context = sessionCoordinator.captureRequest();
  sessionCoordinator.assertRequest(context);
  let token = getAccessToken();
  const request = async () => {
    try {
      return await performRequest(endpoint, options, token);
    } finally {
      sessionCoordinator.assertRequest(context);
    }
  };
  let response = await request();
  if (response.status === 401) {
    // A parallel request may already have refreshed this access token.
    if (token === getAccessToken()) await sessionCoordinator.refresh();
    sessionCoordinator.assertRequest(context);
    token = getAccessToken();
    response = await request();
  }
  let result: T;
  try {
    result = await readResponse<T>(response);
  } catch (error) {
    sessionCoordinator.assertRequest(context);
    if (response.status === 401)
      sessionCoordinator.rejectAccess(context, token);
    throw error;
  }
  sessionCoordinator.assertRequest(context);
  return result;
}
