// Thin fetch wrapper. Same-origin requests to the Go binary, credentialed
// via SameSite=Strict cookies; no CSRF header (backend checks Origin instead).

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: unknown,
  ) {
    super(`request failed with status ${status}`);
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'include',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (!res.ok) {
    const errorBody = await res.json().catch(() => undefined);
    // 401 is surfaced as ApiError; callers should redirect to /login.
    throw new ApiError(res.status, errorBody);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  send: <T>(method: string, path: string, body?: unknown) =>
    request<T>(method, path, body),
};
