// Product screenshots use the real application with isolated, synthetic API data.
import { createRequire } from "node:module";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { startHarness } from "../web/tests/helpers/harness.mjs";
import {
  active,
  openWorkspace,
  owner,
  skill,
} from "../web/tests/helpers/workspace.mjs";

const { chromium, expect } = createRequire(
  new URL("../web/package.json", import.meta.url),
)("@playwright/test");
const output = fileURLToPath(
  new URL("../docs/images/workspace/", import.meta.url),
);
const repository = fileURLToPath(new URL("../", import.meta.url));
const temporary = await mkdtemp(join(tmpdir(), "reet-docs-screens-"));
const source = spawnSync("git", ["rev-parse", "HEAD"], {
  cwd: repository,
  encoding: "utf8",
});
let previous;
try {
  previous = JSON.parse(await readFile(join(output, "manifest.json"), "utf8"));
} catch (error) {
  if (error.code !== "ENOENT") throw error;
}
const now = Date.now(),
  minute = 60_000;
const capabilities = {
  protocolVersion: 1,
  os: "linux",
  arch: "amd64",
  osUser: "sam",
  terminalEnabled: true,
  terminalControl: true,
  healthEnabled: true,
  readiness: "ready",
  launch: true,
  browse: true,
  agentsV2: true,
  acp: true,
  tools: ["claude", "codex", "opencode"],
};
const machines = [
  {
    agentId: "studio",
    host: "studio",
    ready: true,
    serverEnabled: true,
    capabilities,
  },
  {
    agentId: "laptop",
    host: "laptop",
    ready: true,
    serverEnabled: true,
    capabilities: { ...capabilities, os: "darwin", arch: "arm64" },
  },
];
const project = (id, name, dir, host = "studio") => ({
  id,
  name,
  host,
  machine: host,
  dir,
  isRepo: true,
  baseBranch: "main",
  worktreeDefault: "inherit",
  defaultTool: "claude",
  defaultModel: "sonnet",
  defaultEffort: "",
  hasStarterPrompt: false,
  colour: "blue",
  position: 0,
  createdAt: now,
  updatedAt: now,
  version: 1,
});
const projects = [
  project("reet", "Reet", "/home/sam/reet"),
  project("storefront", "Storefront", "/home/sam/storefront"),
  project("personal", "Personal site", "/Users/sam/site", "laptop"),
];
const draft = (id, title, prompt, over = {}) => ({
  id,
  title,
  prompt,
  agentId: "studio",
  projectId: "reet",
  dir: "/home/sam/reet",
  tool: "claude",
  model: "sonnet",
  effort: "",
  imageCount: 0,
  images: [],
  starting: false,
  version: 1,
  createdAt: now - 40 * minute,
  updatedAt: now - 20 * minute,
  ...over,
});
const drafts = [
  draft("idea", "Make the command palette feel quicker", "", { idea: true }),
  draft(
    "recovery",
    "Cover the session recovery edge cases",
    "Check a reconnect after the browser goes offline. Keep the running session and its history.",
    {
      tool: "codex",
      model: "gpt-5-codex",
      projectId: "storefront",
      dir: "/home/sam/storefront",
    },
  ),
  draft(
    "stories",
    "Bring the case studies up to date",
    "Refresh the project stories and add the new screenshots.",
    {
      agentId: "laptop",
      projectId: "personal",
      dir: "/Users/sam/site",
      tool: "opencode",
      model: "provider/default",
    },
  ),
];
const session = (id, title, over = {}) => ({
  id,
  title,
  agentId: "studio",
  projectId: "reet",
  dir: "/home/sam/reet",
  state: "running",
  tool: "codex",
  model: "gpt-5-codex",
  attention: "working",
  createdAt: now - 80 * minute,
  updatedAt: now - 5 * minute,
  exitCode: null,
  ...over,
});
const sessions = [
  session("picker", "Build a faster project picker"),
  session("checkout", "Review the checkout changes", {
    tool: "claude",
    model: "sonnet",
    projectId: "storefront",
    dir: "/home/sam/storefront",
  }),
  session("keyboard", "Tidy up the keyboard shortcuts", {
    state: "exited",
    attention: "finished",
    exitCode: 0,
    updatedAt: now - 35 * minute,
  }),
  session("launch", "Document the new launch options", {
    state: "ended",
    attention: "finished",
    tool: "claude",
    model: "sonnet",
    updatedAt: now - 90 * minute,
  }),
];
const skills = [
  {
    ...skill("frontend-design"),
    description:
      "Build distinctive interfaces with thoughtful typography, layout and interaction.",
    used: true,
  },
  {
    ...skill("code-review"),
    description:
      "Review the change, check its assumptions and keep the feedback actionable.",
    bookmarked: true,
  },
  {
    ...skill("testing"),
    description:
      "Check the behaviour that matters, including failure and recovery paths.",
  },
];
const environments = machines.map((machine) => ({
  id: `${machine.agentId}-agents`,
  host: machine.host,
  agent: "agents",
  name: machine.host,
  skills: skills.slice(0, 2),
  skillCount: 2,
  complete: true,
  scannedAt: now,
  lastSeen: now,
  os: machine.capabilities.os,
  arch: machine.capabilities.arch,
  version: "dev",
}));
const sync = {
  environments,
  machines: environments.map((environment) => ({
    host: environment.host,
    groups: [environment],
    skillCount: 2,
  })),
  agents: machines.map((machine) => ({
    id: machine.agentId,
    host: machine.host,
    version: "dev",
    lastSeen: now,
    paused: false,
    autoUpdate: false,
    keepSynced: true,
  })),
  targets: environments.map((target) => ({
    target,
    sourceId: "shelf",
    sourceName: "Shelf",
    desired: skills.slice(0, 2),
    missing: [],
    extra: [],
    revision: "example",
    appliedRevision: "example",
    updatedAt: now,
    mode: "additive",
    followSource: true,
  })),
  jobs: [],
  runs: [],
  updateJobs: [],
};
const manifest = {
  application: "standalone Reet workspace",
  sourceCommit: source.status === 0 ? source.stdout.trim() : null,
  data: "Synthetic example data; no account, machine or provider connection.",
  screenshots: {},
};
async function save(name, buffer) {
  const digest = createHash("sha256").update(buffer).digest("hex");
  const file = `${name}-${digest.slice(0, 12)}.png`;
  await writeFile(join(output, file), buffer);
  manifest.screenshots[name] = {
    file,
    width: buffer.readUInt32BE(16),
    height: buffer.readUInt32BE(20),
    sha256: digest,
  };
}

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
    for (const width of [1440, 390]) {
      const profile = {
        theme,
        timezone: "Europe/London",
        dateFormat: "",
        shelfView: width > 650 ? "board" : "list",
        cardsExpanded: false,
        hostedPrefix: "reet-",
        updatedAt: now,
      };
      const routes = {
        "/api/version": { version: "dev", commit: "example" },
        "/api/ignored": { ignored: [] },
        "/api/v2/skill-bundles": { bundles: [] },
        "/api/v2/software/batches/active": null,
        "/api/v2/profile": (request) => {
          if (request.method() === "PATCH")
            Object.assign(profile, request.postDataJSON());
          return { user: owner, profile, sessionCount: 1, accounts: true };
        },
        "/api/v2/sync/candidates": { skills },
        "/api/v2/terminal/sessions": { sessions },
        "/api/v2/session-backlog": { drafts },
        "/api/v2/folder-projects": { projects, savedWaiting: 0 },
        "/api/v2/machines/health": () => ({
          serverNow: Date.now(),
          machines: machines.map((machine) => ({
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
          })),
        }),
      };
      const app = await openWorkspace(browser, {
        width,
        height: width > 650 ? 1000 : 844,
        touch: width < 650,
        theme,
        skills,
        sync,
        inventory: { environments, machines: sync.machines },
        terminalStatus: active,
        terminalMachines: machines,
        routes,
      });
      // A reserved example host identifies the demo. Only its static assets are
      // answered from loopback; page.route supplies every API response above.
      await app.context.route("http://studio.reet.test/**", async (route) => {
        const url = new URL(route.request().url());
        const response = await app.context.request.get(
          harness.url + url.pathname + url.search,
        );
        await route.fulfill({ response });
      });
      await app.page.emulateMedia({ reducedMotion: "reduce" });
      for (const [path, name] of [
        ["/", "home"],
        ["/machines", "machines"],
        ["/skills", "skills"],
      ]) {
        await app.page.goto("http://studio.reet.test" + path);
        await app.page.locator("main h1").first().waitFor();
        if (name === "home") {
          await expect(app.page.locator(".session-card")).toHaveCount(7);
          await expect(
            app.page.getByText("Make the command palette feel quicker", {
              exact: true,
            }),
          ).toBeVisible();
        }
        if (name === "machines")
          await expect(app.page.locator("article.machine")).toHaveCount(2);
        if (name === "skills")
          await expect(
            app.page.getByRole("group", { name: "Shelf layout" }),
          ).toBeVisible();
        await app.page.waitForLoadState("networkidle");
        await app.page.evaluate(() => document.fonts.ready);
        expect(
          await app.page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        expect(app.errors).toEqual([]);
        expect([...new Set(app.state.unexpected)]).toEqual([]);
        await save(
          `${name}-${width > 650 ? "desktop" : "phone"}-${theme}`,
          await app.page.screenshot({ fullPage: false }),
        );
      }
      await app.context.close();
    }
  }
} finally {
  await browser.close();
  await harness.close();
}
const gaffer = spawnSync(
  process.execPath,
  [fileURLToPath(new URL("./capture-docs-gaffer.mjs", import.meta.url))],
  {
    env: { ...process.env, REET_DOCS_SCREENSHOT_DIR: temporary },
    stdio: "inherit",
  },
);
if (gaffer.status !== 0) throw new Error("Gaffer screenshots did not finish");
for (const theme of ["light", "dark"])
  await save(
    `gaffer-desktop-${theme}`,
    await readFile(join(temporary, `gaffer-showcase-${theme}.png`)),
  );
// Keep references together with their newly captured assets, including README
// images. Previous image URLs stay available to cached pages and external links.
for (const relative of [
  "README.md",
  "docs/index.md",
  "docs/session-board.md",
]) {
  const path = join(repository, relative);
  let text = await readFile(path, "utf8");
  for (const [name, screenshot] of Object.entries(manifest.screenshots)) {
    const old = previous?.screenshots?.[name]?.file;
    if (old && old !== screenshot.file) {
      text = text.replaceAll(old, screenshot.file);
    }
  }
  if (relative === "docs/index.md")
    text = text.replace(/<img\b[^>]*>/g, (tag) => {
      const screenshot = Object.values(manifest.screenshots).find((image) =>
        tag.includes(image.file),
      );
      return screenshot
        ? tag
            .replace(/\bwidth="\d+"/, `width="${screenshot.width}"`)
            .replace(/\bheight="\d+"/, `height="${screenshot.height}"`)
        : tag;
    });
  await writeFile(path, text);
}
await writeFile(
  join(output, "manifest.json"),
  JSON.stringify(manifest, null, 2) + "\n",
);
await rm(temporary, { recursive: true, force: true });
console.log(
  `Captured ${Object.keys(manifest.screenshots).length} versioned application screenshots.`,
);
