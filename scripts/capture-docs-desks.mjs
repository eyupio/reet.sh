// Capture a desk for the landing page: the real application, synthetic API answers and a fake terminal socket. No
// account, machine or provider is connected and nothing is started.
// Run after `npm --prefix web ci`: node scripts/capture-docs-desks.mjs
import { createRequire } from "node:module";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { startHarness } from "../web/tests/helpers/harness.mjs";

const { chromium, expect } = createRequire(
  new URL("../web/package.json", import.meta.url),
)("@playwright/test");
const output =
  process.env.REET_DOCS_SCREENSHOT_DIR ||
  fileURLToPath(new URL("../docs/images/ui/", import.meta.url));
const now = Date.now();
const capabilities = (osUser) => ({
  osUser,
  terminalEnabled: true,
  terminalControl: true,
  readiness: "ready",
  browse: true,
});
const machines = [
  { agentId: "studio", host: "studio", ready: true, serverEnabled: true, capabilities: capabilities("sam") },
  { agentId: "vps", host: "vps-london", ready: true, serverEnabled: true, capabilities: capabilities("sam") },
];
const session = (id, agentId, title, dir, tool) => ({
  id,
  agentId,
  title,
  state: "running",
  createdAt: now - 3_600_000,
  updatedAt: now,
  exitCode: null,
  dir,
  ...(tool ? { tool } : {}),
});
const sessions = [
  session("s-tests", "studio", "tests", "/home/sam/storefront"),
  session("s-agent", "studio", "checkout review", "/home/sam/storefront", "claude"),
  session("s-logs", "vps", "api logs", "/srv/storefront"),
  session("s-deploy", "vps", "deploy", "/srv/storefront"),
  session("s-db", "vps", "database", "/srv/storefront"),
];
const pane = (id, name, colour, host, sessionId, kind, dir) => ({
  id,
  name,
  colour,
  host,
  session: sessionId,
  kind,
  dir,
  dirAt: now,
  start: "",
});
const single = (id, name, colour, p) => ({
  id,
  name,
  colour,
  root: { pane: p.id },
  zoomed: "",
  active: p.id,
  panes: { [p.id]: p },
});
const work = {
  id: "t-work",
  name: "checkout",
  colour: "amber",
  root: {
    split: "down",
    ratio: 0.56,
    a: { split: "right", ratio: 0.5, a: { pane: "p-tests" }, b: { pane: "p-agent" } },
    b: { pane: "p-logs" },
  },
  zoomed: "",
  active: "p-tests",
  panes: {
    "p-tests": pane("p-tests", "tests", "green", "studio", "s-tests", "shell", "/home/sam/storefront"),
    "p-agent": pane("p-agent", "checkout review", "violet", "studio", "s-agent", "agent", "/home/sam/storefront"),
    "p-logs": pane("p-logs", "api logs", "blue", "vps", "s-logs", "shell", "/srv/storefront"),
  },
};
let desk = {
  id: "desk-storefront",
  name: "storefront",
  colour: "amber",
  template: false,
  position: 0,
  version: 1,
  schema: 1,
  doc: {
    active: "t-work",
    tabs: [
      work,
      single("t-deploy", "deploy", "red", pane("p-deploy", "deploy", "", "vps", "s-deploy", "shell", "/srv/storefront")),
      single("t-db", "database", "", pane("p-db", "database", "", "vps", "s-db", "shell", "/srv/storefront")),
    ],
  },
};
const summary = () => ({
  id: desk.id,
  name: desk.name,
  colour: desk.colour,
  template: false,
  position: 0,
  version: desk.version,
  tabs: desk.doc.tabs.length,
  panes: desk.doc.tabs.reduce((n, t) => n + Object.keys(t.panes).length, 0),
  sessions: desk.doc.tabs.flatMap((t) => Object.values(t.panes).map((p) => p.session)),
  hosts: ["studio", "vps"],
});

// What each terminal shows. Plain example output; nothing here ran.
const E = "\x1b[", green = `${E}32m`, dim = `${E}2m`, bold = `${E}1m`, cyan = `${E}36m`, yellow = `${E}33m`, off = `${E}0m`;
const prompt = (host, dir) => `${green}sam@${host}${off}:${cyan}${dir}${off}$ `;
const screens = {
  "s-tests": [
    `${prompt("studio", "~/storefront")}npm test -- checkout`,
    "",
    ` ${green}✓${off} checkout/cart.test.ts ${dim}(14 tests) 212ms${off}`,
    ` ${green}✓${off} checkout/totals.test.ts ${dim}(9 tests) 96ms${off}`,
    ` ${green}✓${off} checkout/address.test.ts ${dim}(11 tests) 143ms${off}`,
    ` ${green}✓${off} checkout/payment.test.ts ${dim}(17 tests) 388ms${off}`,
    "",
    ` ${bold}Test Files${off}  ${green}4 passed${off} (4)`,
    ` ${bold}     Tests${off}  ${green}51 passed${off} (51)`,
    ` ${bold}  Duration${off}  1.92s`,
    "",
    prompt("studio", "~/storefront"),
  ],
  "s-agent": [
    `${bold}Checkout review${off} ${dim}· ~/storefront${off}`,
    "",
    "I read the checkout flow and its tests. Three things are worth",
    "changing before this ships:",
    "",
    `  ${yellow}1.${off} The order total is rounded per line, then summed.`,
    `  ${yellow}2.${off} A failed card leaves the basket locked for 15 minutes.`,
    `  ${yellow}3.${off} The address form drops the county on Back.`,
    "",
    "I have a fix for the first with a test. Shall I open a pull request,",
    "or carry on to the other two first?",
    "",
    `${dim}>${off} `,
  ],
  "s-logs": [
    `${prompt("vps-london", "/srv/storefront")}journalctl -u storefront-api -f`,
    `${dim}10:42:07${off} GET  /api/basket            200  12ms`,
    `${dim}10:42:07${off} POST /api/checkout/address  200  31ms`,
    `${dim}10:42:09${off} POST /api/checkout/payment  200 284ms`,
    `${dim}10:42:09${off} ${green}order 48213 placed${off} total=64.50 items=3`,
    `${dim}10:42:14${off} GET  /api/orders/48213      200   9ms`,
    `${dim}10:42:21${off} GET  /healthz               200   1ms`,
    `${dim}10:42:26${off} GET  /api/basket            200  11ms`,
  ],
  "s-deploy": [prompt("vps-london", "/srv/storefront")],
  "s-db": [prompt("vps-london", "/srv/storefront")],
};

const api = async (route) => {
  const request = route.request(),
    path = new URL(request.url()).pathname,
    method = request.method();
  const json = (data) => route.fulfill({ json: data });
  if (path === "/api/v2/auth/status")
    return json({ mode: "accounts", user: { id: 1, name: "Sam", email: "sam@example.test", role: "owner" } });
  if (path === "/api/v2/profile")
    return json({ profile: { theme, shelfView: "list", cardsExpanded: false, timezone: "Europe/London" } });
  if (path === "/api/skills") return json({ skills: [], counts: { all: 0, used: 0, liked: 0, bookmarked: 0 } });
  if (path === "/api/environments") return json({ environments: [], machines: [] });
  if (path === "/api/tags") return json({ tags: [] });
  if (path === "/api/v2/lists") return json({ lists: [], linkEnabled: false });
  if (path === "/api/v2/sync") return json({ environments: [], machines: [], targets: [], agents: [], jobs: [] });
  if (path === "/api/v2/notifications/count") return json({ unread: 0 });
  if (path === "/api/version") return json({ version: "dev", commit: "example" });
  if (path === "/api/v2/machines/health") return json({ serverNow: Date.now(), machines: [], history: [] });
  if (path === "/api/v2/terminal/machines") return json({ machines });
  if (path === "/api/v2/terminal/status") return json({ available: true, configured: true, reason: "" });
  if (path === "/api/v2/folder-projects") return json({ projects: [], savedWaiting: 0 });
  if (path === "/api/v2/project-groups") return json({ groups: [] });
  if (path === "/api/v2/terminal/sessions") return json({ sessions });
  if (path.endsWith("/command-history")) return json({ entries: [], sources: [], truncated: false, skipped: 0 });
  if (path.endsWith("/attachments")) return json({ ticket: `ticket-${path.split("/").at(-2)}` });
  if (path === "/api/v2/desks") return json({ desks: [summary()] });
  if (path === `/api/v2/desks/${desk.id}`) {
    if (method === "PATCH") {
      const body = request.postDataJSON();
      desk = { ...desk, ...(body.doc ? { doc: body.doc } : {}), version: desk.version + 1 };
    }
    return json(desk);
  }
  return json({});
};
const socket = (ws) => {
  ws.onMessage((message) => {
    if (typeof message !== "string") return;
    const data = JSON.parse(message);
    if (!data.ticket) return;
    const lines = screens[String(data.ticket).replace(/^ticket-/, "")] ?? [];
    ws.send('{"type":"ready"}');
    ws.send(Buffer.from(`\x1b[2J\x1b[H${lines.join("\r\n")}`));
  });
};

let theme = "light";
const harness = await startHarness();
const browser = await chromium.launch({
  headless: true,
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || "/usr/bin/chromium",
  args: ["--no-sandbox"],
});
try {
  await mkdir(output, { recursive: true });
  for (theme of ["light", "dark"]) {
    desk = { ...desk, version: 1 };
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: theme });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("**/api/**", api);
    await page.routeWebSocket("**/api/v2/terminal/socket", socket);
    await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
    await page.goto(`${harness.url}/desks/${desk.id}`);
    await expect(page.getByRole("heading", { name: "storefront", level: 1 })).toBeVisible({ timeout: 60_000 });
    await expect(page.locator(".desk-pane .xterm-screen")).toHaveCount(3);
    await expect(page.locator('.desk-pane[data-state="live"]')).toHaveCount(3);
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(1200);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect(errors).toEqual([]);
    await page.screenshot({ path: join(output, `desks-showcase-${theme}.png`) });
    await context.close();
  }
} finally {
  await browser.close();
  await harness.close();
}
console.log("Captured the desk in light and dark.");
