const BASE = "/api/v1";

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

  const res = await fetch(`${BASE}${path}`, opts);
  const envelope: ApiEnvelope<T> = await res.json();

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
