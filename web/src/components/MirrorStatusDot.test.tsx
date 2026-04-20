import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { MirrorStatusDot } from "./MirrorStatusDot";

describe("MirrorStatusDot", () => {
  it("renders success tooltip with relative time", () => {
    const { container } = render(
      <MirrorStatusDot status="success" direction="push"
        lastSyncedAt={new Date().toISOString()} />
    );
    const el = container.querySelector("span[title]");
    expect(el?.getAttribute("title")).toMatch(/Last synced/);
  });

  it("renders failed tooltip with error message", () => {
    const { container } = render(
      <MirrorStatusDot status="failed" direction="pull"
        lastError="bad credentials" lastSyncedAt={new Date().toISOString()} />
    );
    const el = container.querySelector("span[title]");
    expect(el?.getAttribute("title")).toMatch(/bad credentials/);
  });

  it("shows direction arrow — down for pull, up for push", () => {
    const { container: pull } = render(
      <MirrorStatusDot status="success" direction="pull" />
    );
    expect(pull.textContent).toContain("↓");

    const { container: push } = render(
      <MirrorStatusDot status="success" direction="push" />
    );
    expect(push.textContent).toContain("↑");
  });

  it("is clickable only when failed and onRetry is set", () => {
    const onRetry = () => {};
    const { container } = render(
      <MirrorStatusDot status="failed" direction="pull" onRetry={onRetry} />
    );
    const el = container.querySelector('span[role="button"]');
    expect(el).not.toBeNull();
  });

  it("is not clickable on success", () => {
    const { container } = render(
      <MirrorStatusDot status="success" direction="push" />
    );
    expect(container.querySelector('span[role="button"]')).toBeNull();
  });
});
