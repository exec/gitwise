/**
 * api.ts — exponential backoff tests
 *
 * Verifies that:
 * 1. 5xx responses are retried up to MAX_RETRIES times with delay.
 * 2. 4xx responses are NOT retried.
 * 3. A successful response after retries is returned normally.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { get, ApiError } from "./api";

const makeJsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

describe("api exponential backoff", () => {
  let originalFetch: typeof fetch;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let fetchMock: any;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
    // Speed up tests — use fake timers so backoff delays resolve instantly
    vi.useFakeTimers();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("retries on 503 and succeeds on the third attempt", async () => {
    fetchMock
      .mockResolvedValueOnce(makeJsonResponse(503, { errors: [{ code: "unavailable", message: "unavailable" }] }))
      .mockResolvedValueOnce(makeJsonResponse(503, { errors: [{ code: "unavailable", message: "unavailable" }] }))
      .mockResolvedValueOnce(makeJsonResponse(200, { data: { ok: true } }));

    const promise = get<{ ok: boolean }>("/some-endpoint");

    // Advance fake timers to resolve all backoff delays
    await vi.runAllTimersAsync();

    const result = await promise;
    expect(result.data.ok).toBe(true);
    // Should have been called 3 times (initial + 2 retries)
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("does NOT retry on 4xx errors", async () => {
    fetchMock.mockResolvedValueOnce(
      makeJsonResponse(404, { errors: [{ code: "not_found", message: "not found" }] }),
    );

    // Catch the rejection before timers run to avoid unhandled rejection
    const resultPromise = get("/nonexistent").catch((e) => e);
    await vi.runAllTimersAsync();

    const result = await resultPromise;
    expect(result).toBeInstanceOf(ApiError);
    // Only 1 call — no retries on 4xx
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("throws ApiError after exhausting retries", async () => {
    fetchMock.mockResolvedValue(
      makeJsonResponse(500, { errors: [{ code: "server_error", message: "internal error" }] }),
    );

    const resultPromise = get("/failing-endpoint").catch((e) => e);
    await vi.runAllTimersAsync();

    const result = await resultPromise;
    expect(result).toBeInstanceOf(ApiError);
    // 1 initial + 2 retries = 3 total calls
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("does not retry on 200 with errors in body", async () => {
    // Some APIs return 200 with an errors array — should NOT retry, just throw.
    fetchMock.mockResolvedValueOnce(
      makeJsonResponse(200, { errors: [{ code: "validation", message: "bad input" }] }),
    );

    const resultPromise = get("/bad-input").catch((e) => e);
    await vi.runAllTimersAsync();

    const result = await resultPromise;
    expect(result).toBeInstanceOf(ApiError);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
