// Capture the current app, with synthetic data and no provider or machine connection.
// Run after `npm --prefix web ci`: node scripts/capture-docs-gaffer.mjs
import { createRequire } from "node:module";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { startHarness } from "../web/tests/helpers/harness.mjs";
import { active, openWorkspace } from "../web/tests/helpers/workspace.mjs";

const { chromium, expect } = createRequire(
  new URL("../web/package.json", import.meta.url),
)("@playwright/test");
const output =
  process.env.REET_DOCS_SCREENSHOT_DIR ||
  fileURLToPath(new URL("../docs/images/ui/", import.meta.url));
const machineId = "docs-studio",
  sessionId = "docs-gaffer-session";
const now = Date.now();
const session = {
  id: sessionId,
  agentId: machineId,
  title: "Review the checkout flow",
  state: "running",
  createdAt: now - 180000,
  updatedAt: now,
  activityAt: now,
  exitCode: null,
  tool: "opencode",
  model: "provider/default",
  effort: "high",
  yolo: false,
  projectId: "storefront",
  dir: "/home/dev/storefront",
  structured: true,
  attention: "working",
  attentionSource: "chat",
};
const machine = {
  agentId: machineId,
  host: "studio",
  ready: true,
  serverEnabled: true,
  capabilities: {
    protocolVersion: 1,
    terminalControl: true,
    terminalEnabled: true,
    healthEnabled: true,
    os: "linux",
    osUser: "dev",
    readiness: "ready",
    launch: true,
    tools: ["opencode"],
    acp: true,
    acpApproval: true,
    inspect: true,
  },
};
const root = `/api/v2/terminal/sessions/${sessionId}`;
const harness = await startHarness();
const browser = await chromium.launch({
  headless: true,
  executablePath:
    process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || "/usr/bin/chromium",
  args: ["--no-sandbox"],
});
try {
  await mkdir(output, { recursive: true });
  for (const theme of ["light", "dark"]) {
    const { page, context, errors } = await openWorkspace(browser, {
      width: 1440,
      height: 840,
      theme,
      terminalStatus: active,
      terminalMachines: [machine],
      routes: {
        "/api/v2/terminal/sessions": { sessions: [session] },
        "/api/v2/machines/health": () => ({
          serverNow: Date.now(),
          machines: [
            {
              ...machine,
              state: "connected",
              connection: {
                connected: true,
                reportFresh: true,
                reportAt: Date.now(),
                contactAt: Date.now(),
                missedProbes: 0,
                reconnects: 0,
                path: "direct",
              },
            },
          ],
        }),
        "/api/v2/folder-projects": {
          projects: [
            {
              id: "storefront",
              name: "Storefront",
              host: machineId,
              machine: "studio",
              dir: session.dir,
              isRepo: true,
              baseBranch: "main",
              worktreeDefault: "inherit",
              colour: "green",
              defaultTool: "opencode",
              defaultModel: "",
              hasStarterPrompt: false,
              note: "",
              position: 0,
              createdAt: now,
              updatedAt: now,
              version: 1,
            },
          ],
          savedWaiting: 0,
        },
        "/api/v2/projects": { projects: [] },
        "/api/v2/yolo": { allowed: false },
        [`${root}/seen`]: {},
        [`${root}/skills`]: { skills: [], machineSeen: now, host: "studio" },
        [`${root}/inspect`]: () => ({
          checkedAt: Date.now(),
          repo: true,
          branch: "review/checkout",
          ahead: 0,
          behind: 0,
          upstream: true,
          changed: 1,
          conflicts: 0,
          countsKnown: true,
          files: [
            { path: "src/checkout.ts", status: " M", added: 12, removed: 4 },
          ],
        }),
        [`${root}/chat/queue`]: { queue: [] },
        [`${root}/chat/controls`]: {
          controls: {
            working: true,
            waiting: false,
            compacting: false,
            configuring: false,
            steering: true,
            model: "provider/default",
            options: [],
          },
        },
      },
    });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.addInitScript(() => {
      class DemoStream {
        constructor() {
          this.listeners = {};
          window.__docsGaffer = this;
          setTimeout(() => this.onopen?.({}), 0);
        }
        addEventListener(type, listener) {
          (this.listeners[type] ??= []).push(listener);
        }
        emit(type, data) {
          for (const listener of this.listeners[type] ?? [])
            listener({ data: JSON.stringify(data) });
        }
        close() {}
      }
      window.EventSource = DemoStream;
    });
    await page.goto(
      `${harness.url}/sessions?agent=${machineId}&session=${sessionId}`,
    );
    await page.waitForFunction(() => window.__docsGaffer);
    await page.evaluate(() => {
      const events = [
        {
          kind: "user",
          text: "Review the checkout flow and simplify the validation. Keep the existing behavior and add focused checks.",
        },
        {
          kind: "agent",
          messageId: "review",
          text: "I’m checking the checkout flow and its validation. I’ll keep the changes focused, then run the relevant checks.",
        },
        {
          kind: "tool",
          toolCallId: "read",
          toolKind: "read",
          title: "Read the checkout flow",
          path: "/home/dev/storefront/src/checkout.ts",
          status: "completed",
        },
        {
          kind: "tool",
          toolCallId: "edit",
          toolKind: "edit",
          title: "Simplify checkout validation",
          path: "/home/dev/storefront/src/checkout.ts",
          status: "in_progress",
          added: 12,
          removed: 4,
        },
        { kind: "status", state: "working" },
      ];
      window.__docsGaffer.emit("reset", { truncated: false });
      events.forEach((event, i) =>
        window.__docsGaffer.emit("chat", { seq: i + 1, ...event }),
      );
    });
    const gaffer = page.getByRole("region", { name: "Gaffer’s work update" });
    await expect(gaffer).toHaveAttribute("data-phase", "editing");
    await gaffer
      .getByRole("button", { name: "Expand Gaffer’s card", exact: true })
      .click();
    await expect(
      page.getByText("Machine online", { exact: true }),
    ).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await gaffer.locator(".gaffer-avatar image").evaluateAll((images) =>
      Promise.all(
        images.map(async (element) => {
          const image = new Image();
          image.src = element.href.baseVal;
          await image.decode();
        }),
      ),
    );
    if (theme === "light") {
      expect(
        await gaffer
          .locator(".gaffer-avatar")
          .evaluate((element) => getComputedStyle(element).backgroundColor),
      ).toBe("rgb(107, 88, 72)");
    }
    expect(errors).toEqual([]);
    await page.screenshot({
      path: join(output, `gaffer-showcase-${theme}.png`),
      fullPage: true,
    });
    await context.close();
  }
  console.log(
    "Captured the current Gaffer UI in both themes, including the warm-brown light-mode portrait surround.",
  );
} finally {
  await browser.close();
  await harness.close();
}
