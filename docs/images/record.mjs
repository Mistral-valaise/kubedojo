// Records the README's GIFs and screenshots from a running kubedojo.
//
//   npm install playwright            (anywhere; uses your installed Chrome)
//   kubedojo up --no-open --addr 127.0.0.1:4410 --data /tmp/kubedojo-demo --observability
//   node docs/images/record.mjs
//
// Needs ffmpeg and docker on the PATH. Output goes to docs/images/.
import { chromium } from "playwright";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const BASE = process.env.KUBEDOJO_URL ?? "http://127.0.0.1:4410";
const GRAFANA = process.env.GRAFANA_URL ?? "http://127.0.0.1:3000";
const OUT = dirname(fileURLToPath(import.meta.url));
const ROOT = join(OUT, "..", "..");
const only = process.argv.slice(2);
const want = (name) => only.length === 0 || only.includes(name);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function api(method, path, body) {
  const res = await fetch(BASE + "/api" + path, {
    method, headers: body ? { "Content-Type": "application/json" } : undefined, body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${data.error ?? ""}`);
  return data;
}

async function waitFor(fn, what, timeoutMs = 600_000) {
  const end = Date.now() + timeoutMs;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > end) throw new Error("timed out waiting for " + what);
    await sleep(2000);
  }
}

/** Captures frames at a fixed rate until stop() is called, then builds a GIF. */
function recorder(page, name, fps = 8) {
  const dir = join(tmpdir(), "kubedojo-frames-" + name);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  let running = true, n = 0;
  const loop = (async () => {
    const step = 1000 / fps;
    let next = Date.now();
    while (running) {
      await page.screenshot({ path: join(dir, `f${String(n++).padStart(5, "0")}.png`) }).catch(() => {});
      next += step;
      const wait = next - Date.now();
      if (wait > 0) await sleep(wait); else next = Date.now();
    }
  })();
  return async () => {
    running = false;
    await loop;
    const gif = join(OUT, name + ".gif");
    execFileSync("ffmpeg", ["-y", "-loglevel", "error", "-framerate", String(fps), "-i", join(dir, "f%05d.png"),
      "-vf", "scale=1000:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle",
      gif]);
    rmSync(dir, { recursive: true, force: true });
    console.log("wrote", gif, `(${n} frames)`);
  };
}

async function type(page, text, delay = 28) {
  await page.keyboard.type(text, { delay });
  await sleep(250);
  await page.keyboard.press("Enter");
}

/** Runs a question's reference solution on its cluster (to give the exam a realistic score). */
function solve(track, question, host) {
  const script = readFileSync(join(ROOT, "content", track, "questions", question, "solution.sh"), "utf8");
  execFileSync("docker", ["exec", "-e", "KUBECONFIG=/etc/kubernetes/admin.conf", "-e", "KD_HOST=" + host,
    host + "-control-plane", "bash", "-c", script], { stdio: "ignore" });
}

const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({ viewport: { width: 1280, height: 760 }, deviceScaleFactor: 1, colorScheme: "dark" });
await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: BASE });
const page = await context.newPage();

// ---- practice ---------------------------------------------------------------
if (want("practice")) {
  const q = "rbac-pipeline-serviceaccount";
  const s = await api("POST", "/sessions", { mode: "practice", track: "cka" });
  await api("POST", `/sessions/${s.id}/questions/${q}/prepare?reset=1`);
  const ready = await waitFor(async () => {
    const v = await api("GET", `/sessions/${s.id}`);
    const st = v.questions?.find((x) => x.id === q);
    if (st?.status === "error") throw new Error(st.error);
    return st?.status === "ready" ? v : null;
  }, "the practice question");
  const host = ready.envs.base;

  await page.goto(`${BASE}/t/cka/q/${q}`);
  await page.getByRole("button", { name: "Check my solution" }).waitFor();
  await page.locator(".xterm-screen").waitFor();
  await sleep(2500);
  const stop = recorder(page, "practice");
  await sleep(1200);
  await page.locator(".xterm-screen").click();
  await type(page, `ssh ${host}`);
  await sleep(1500);
  await type(page, "k -n team-orca create sa pipeline");
  await sleep(900);
  await type(page, "k -n team-orca create role pipeline --verb='*' --resource=deployments.apps,configmaps");
  await sleep(900);
  await type(page, "k -n team-orca create rolebinding pipeline --role=pipeline --serviceaccount=team-orca:pipeline");
  await sleep(1200);
  await page.getByRole("button", { name: "Check my solution" }).click();
  await page.getByText("checks passed").first().waitFor();
  await sleep(600);
  await page.getByText("How this was checked").first().scrollIntoViewIfNeeded();
  await sleep(3200);
  await page.locator(".xterm-screen").click();
  await type(page, "k -n team-orca delete role pipeline");
  await sleep(700);
  await type(page, "k -n team-orca create role pipeline --verb=create,list --resource=deployments.apps,configmaps");
  await sleep(1200);
  await page.getByRole("button", { name: "Check my solution" }).scrollIntoViewIfNeeded();
  await page.getByRole("button", { name: "Check my solution" }).click();
  await page.getByText("All checks passed").waitFor();
  await page.getByText("All checks passed").scrollIntoViewIfNeeded();
  await sleep(3000);
  await stop();
}

// ---- learn ------------------------------------------------------------------
if (want("learn")) {
  await page.goto(`${BASE}/t/cka`);
  await page.getByText("What to practise next").waitFor();
  await sleep(1200);
  if (want("stills") || only.length === 0) {
    await page.screenshot({ path: join(OUT, "track.png") });
    console.log("wrote track.png");
  }
  const stop = recorder(page, "learn");
  await sleep(1500);
  await page.mouse.wheel(0, 380);
  await sleep(1400);
  await page.getByRole("button", { name: "Lesson" }).first().click();
  await page.getByText("The 20% that solves 80%").waitFor();
  await sleep(1600);
  for (let i = 0; i < 7; i++) { await page.mouse.wheel(0, 150); await sleep(380); }
  await sleep(900);
  const code = page.locator("code.copyable", { hasText: "/etc/kubernetes/manifests/" }).first();
  await code.scrollIntoViewIfNeeded();
  await sleep(700);
  await code.hover();
  await sleep(600);
  await code.click();
  await sleep(1500);
  for (let i = 0; i < 5; i++) { await page.mouse.wheel(0, 170); await sleep(380); }
  await sleep(1500);
  await stop();
}

// ---- exam -------------------------------------------------------------------
if (want("exam")) {
  const s = await api("POST", "/sessions", { mode: "exam", track: "cka", minutes: 35, questions: 5 });
  const ready = await waitFor(async () => {
    const v = await api("GET", `/sessions/${s.id}`);
    if (v.status === "error") throw new Error(v.error);
    return v.status === "ready" ? v : null;
  }, "the exam environment", 900_000);
  await api("POST", `/sessions/${s.id}/start`);
  // Solve all but the last two tasks behind the scenes for a realistic report.
  for (const st of ready.questions.slice(0, -2)) {
    try { solve("cka", st.id, ready.meta[st.id].host); } catch { /* leave unsolved */ }
  }
  await sleep(20_000);

  await page.goto(`${BASE}/exam/${s.id}`);
  await page.getByRole("button", { name: "Finish exam" }).waitFor();
  await page.locator(".xterm-screen").waitFor();
  await sleep(2500);
  const stop = recorder(page, "exam");
  await sleep(1800);
  await page.getByRole("navigation", { name: "Questions" }).getByRole("button", { name: "2", exact: true }).click();
  await sleep(1800);
  await page.getByRole("button", { name: "Flag for review" }).click();
  await sleep(900);
  await page.getByRole("navigation", { name: "Questions" }).getByRole("button", { name: "3", exact: true }).click();
  await sleep(1500);
  await page.getByPlaceholder("Private notes for this task ...").click();
  await page.keyboard.type("check the selector again", { delay: 30 });
  await sleep(1100);
  await page.getByRole("button", { name: "Finish exam" }).click();
  await sleep(1600);
  await page.getByRole("button", { name: "Finish and grade" }).click();
  await page.getByText("Score by domain").waitFor({ timeout: 300_000 });
  await sleep(2800);
  await page.mouse.wheel(0, 330);
  await sleep(1500);
  await page.getByText("Task by task").scrollIntoViewIfNeeded();
  const rows = page.locator("button[aria-expanded]");
  await rows.nth(await rows.count() - 1).click();
  await sleep(1000);
  for (let i = 0; i < 5; i++) { await page.mouse.wheel(0, 170); await sleep(420); }
  await sleep(2500);
  await stop();
  await page.goto(`${BASE}/exam/${s.id}`);
  await page.getByText("Score by domain").waitFor();
  await sleep(800);
  await page.screenshot({ path: join(OUT, "report.png") });
  console.log("wrote report.png");
  await api("DELETE", `/sessions/${s.id}`);
}

// ---- grafana ----------------------------------------------------------------
if (want("grafana")) {
  try {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`${GRAFANA}/d/kubedojo-learning/kubedojo-learning-analytics?orgId=1&from=now-30m&to=now&kiosk`, { timeout: 15_000 });
    await sleep(6000);
    await page.screenshot({ path: join(OUT, "grafana.png") });
    console.log("wrote grafana.png");
  } catch (e) {
    console.log("skipped grafana.png:", e.message);
  }
}

await browser.close();
