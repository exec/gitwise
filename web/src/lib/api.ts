const BASE = "/api/v1";

// Maximum number of retries for 5xx responses (not counting the initial attempt)
const MAX_RETRIES = 2;
// Base delay in ms for exponential backoff
const BACKOFF_BASE_MS = 200;

interface ApiErrorEntry {
  code: string;
  message: string;
  field?: string;
}

interface ApiEnvelope<T> {
  data: T;
  meta?: Record<string, unknown>;
  errors?: ApiErrorEntry[];
}

export class ApiError extends Error {
  status: number;
  errors: ApiErrorEntry[];

  constructor(status: number, errors: ApiErrorEntry[]) {
    super(errors[0]?.message ?? `API error ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.errors = errors;
  }
}

/** Returns a jittered delay for retry attempt `n` (0-indexed). */
function backoffDelay(n: number): number {
  const base = BACKOFF_BASE_MS * Math.pow(2, n);
  // Add up to 50% jitter
  return base + Math.random() * base * 0.5;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<{ data: T; meta?: Record<string, unknown> }> {
  const opts: RequestInit = {
    method,
    credentials: "include",
    headers: {} as Record<string, string>,
  };

  if (body !== undefined) {
    (opts.headers as Record<string, string>)["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }

  let res!: Response;
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    if (attempt > 0) {
      await new Promise((resolve) => setTimeout(resolve, backoffDelay(attempt - 1)));
    }
    res = await fetch(`${BASE}${path}`, opts);
    // Only retry on 5xx (server errors); 4xx are not retryable
    if (res.status < 500 || attempt === MAX_RETRIES) break;
  }

  let envelope: ApiEnvelope<T>;
  try {
    envelope = await res.json();
  } catch {
    throw new ApiError(res.status, [
      { code: "parse_error", message: `Server returned non-JSON response (${res.status})` },
    ]);
  }

  if (!res.ok || envelope.errors?.length) {
    throw new ApiError(
      res.status,
      envelope.errors ?? [{ code: "unknown", message: `Request failed with status ${res.status}` }],
    );
  }

  return { data: envelope.data, meta: envelope.meta };
}

export function get<T>(path: string) {
  return request<T>("GET", path);
}

export function post<T>(path: string, body?: unknown) {
  return request<T>("POST", path, body);
}

export function patch<T>(path: string, body?: unknown) {
  return request<T>("PATCH", path, body);
}

export function put<T>(path: string, body?: unknown) {
  return request<T>("PUT", path, body);
}

export function del<T>(path: string) {
  return request<T>("DELETE", path);
}
