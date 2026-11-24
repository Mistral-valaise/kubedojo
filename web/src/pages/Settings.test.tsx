import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Settings, SettingsPatch, SettingsTest } from "../api";

const providers: Settings["providers"] = [
  { id: "anthropic", label: "Anthropic", needsKey: true, defaultModel: "claude-opus-5-5" },
  { id: "openai", label: "OpenAI-compatible", needsKey: false, presets: [
    { label: "Ollama", baseURL: "http://localhost:11434/v1" }, { label: "LM Studio", baseURL: "http://localhost:1234/v1" },
    { label: "OpenAI", baseURL: "https://api.openai.com/v1" }, { label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1" },
  ] },
];
const file = <T,>(value: T) => ({ value, source: "file" as const, locked: false });
const locked = <T,>(value: T, by: string) => ({ value, source: by.startsWith("--") ? "flag" as const : "env" as const, locked: true, lockedBy: by });
/** GET /api/settings of a local server that runs Ollama. `over` replaces fields of `assistant`. */
function fixture(over: Partial<Settings["assistant"]> = {}, top: Partial<Settings> = {}): Settings {
  return {
    writable: true, hosted: false, providers,
    assistant: {
      enabled: file(true), provider: file("openai"), model: file("llama3.2"), baseURL: file("http://localhost:11434/v1"),
      apiKey: { set: false, source: "default", locked: false }, configured: true, problem: "", ...over,
    },
    traceContent: { value: false, reason: "no local observability stack" },
    ...top,
  };
}

const server = {
  settings: fixture(),
  loadError: null as Error | null,
  saved: [] as SettingsPatch[],
  tested: [] as SettingsPatch[],
  // What PUT and the test answer: an error to throw, or the result of the test.
  saveError: null as Error | null,
  testError: null as Error | null,
  testResult: { ok: true, provider: "ollama", model: "llama3.2", latencyMs: 412 } as SettingsTest,
  // A test that the test case answers itself.
  holdTest: false, releaseTest: null as null | (() => void),
  configCalls: 0,
};

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => { server.configCalls++; return { version: "0.5.0", tutor: false, user: "", maxTerminals: 4 }; },
    settings: async () => { if (server.loadError) throw server.loadError; return server.settings; },
    saveSettings: async (patch: SettingsPatch) => {
      server.saved.push(patch);
      if (server.saveError) throw server.saveError;
      const a = server.settings.assistant;
      server.settings = { ...server.settings, assistant: {
        ...a,
        enabled: patch.enabled === undefined ? a.enabled : file(patch.enabled),
        provider: patch.provider === undefined ? a.provider : file(patch.provider),
        model: patch.model === undefined ? a.model : file(patch.model),
        baseURL: patch.baseURL === undefined ? a.baseURL : file(patch.baseURL),
        apiKey: patch.apiKey === undefined ? a.apiKey
          : patch.apiKey === "" ? { set: false, source: "default", locked: false }
            : { set: true, last4: patch.apiKey.slice(-4), source: "file", locked: false },
      } };
      return server.settings;
    },
    testSettings: async (patch: SettingsPatch) => {
      server.tested.push(patch);
      if (server.holdTest) await new Promise<void>((resolve) => { server.releaseTest = resolve; });
      if (server.testError) throw server.testError;
      return server.testResult;
    },
  },
}));

import { ApiError } from "../api";
import { resetConfigCache } from "../config";
import { SettingsPage } from "./Settings";

async function open() {
  const view = render(<MemoryRouter initialEntries={["/settings"]}><SettingsPage /></MemoryRouter>);
  await waitFor(() => expect(screen.queryByRole("status", { name: "Loading the settings" })).toBeNull());
  return view;
}
const field = (label: string) => screen.getByLabelText(label) as HTMLInputElement;
const provider = (name: RegExp) => screen.getByRole("radio", { name }) as HTMLInputElement;
const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;
const save = async () => { fireEvent.click(button("Save")); await waitFor(() => expect(button("Save").textContent).toBe("Save")); await act(async () => { await Promise.resolve(); }); };
const change = (label: string, value: string) => fireEvent.change(field(label), { target: { value } });
/** The row that says whether a key is set. */
const keyRow = () => screen.getByText("API key").parentElement!.parentElement as HTMLElement;
const presets = () => ["Ollama", "LM Studio", "OpenAI", "OpenRouter"].filter((p) => screen.queryByRole("button", { name: p }));

beforeEach(() => {
  Object.assign(server, {
    settings: fixture(), loadError: null, saved: [], tested: [], saveError: null, testError: null,
    testResult: { ok: true, provider: "ollama", model: "llama3.2", latencyMs: 412 }, holdTest: false, releaseTest: null, configCalls: 0,
  });
  resetConfigCache();
});
afterEach(cleanup);

describe("SettingsPage", () => {
  it("shows the settings of the assistant as the server has them", async () => {
    await open();
    expect(screen.getByRole("heading", { level: 1, name: "Settings" })).toBeTruthy();
    const enabled = screen.getByRole("switch", { name: "AI assistant" });
    expect(enabled.getAttribute("aria-checked")).toBe("true");
    expect(screen.getAllByRole("radio").map((r) => `${(r as HTMLInputElement).value} ${(r as HTMLInputElement).checked}`)).toEqual(["anthropic false", "openai true"]);
    expect(within(screen.getByRole("radiogroup", { name: "Provider" })).getByText("Ollama, LM Studio, OpenAI, OpenRouter and other servers with the OpenAI API")).toBeTruthy();
    expect(field("Base URL").value).toBe("http://localhost:11434/v1");
    expect(field("Model").value).toBe("llama3.2");
    expect(within(keyRow()).getByText("Not set")).toBeTruthy();
    // The presets of the chosen provider, and the one in use is marked.
    expect(presets()).toEqual(["Ollama", "LM Studio", "OpenAI", "OpenRouter"]);
    expect(button("Ollama").getAttribute("aria-pressed")).toBe("true");
    expect(button("LM Studio").getAttribute("aria-pressed")).toBe("false");
    // Nothing is locked, nothing is wrong, nothing is changed.
    expect(screen.queryByText(/^Set by /)).toBeNull();
    expect(screen.queryByText(/cannot answer yet/)).toBeNull();
    expect(button("Save").disabled).toBe(true);
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    expect(screen.queryByText("This kubedojo is configured by its administrator.")).toBeNull();
  });

  it("says what the assistant sees, and whether prompts and answers go into traces", async () => {
    await open();
    const card = screen.getByRole("heading", { name: "What the assistant sees" }).parentElement as HTMLElement;
    expect(within(card).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "The task text, your check results and the hints you already opened",
      "Your note for the question and a short read-only snapshot of the sandbox",
      "Never the solution unless you have viewed it, and never during an exam",
      "No terminal history, no other question and no user name",
    ]);
    const row = within(card).getByText("Prompt and answer text in traces").parentElement as HTMLElement;
    expect(within(row).getByText("Off")).toBeTruthy();
    expect(within(row).getByText("Reason: no local observability stack")).toBeTruthy();
    cleanup();

    server.settings = fixture({}, { traceContent: { value: true, reason: "local observability stack" } });
    await open();
    const on = screen.getByText("Prompt and answer text in traces").parentElement as HTMLElement;
    expect(within(on).getByText("On")).toBeTruthy();
    expect(within(on).getByText("Reason: local observability stack")).toBeTruthy();
  });

  it("says what is missing while the assistant is not configured", async () => {
    server.settings = fixture({ model: { value: "", source: "default", locked: false }, configured: false, problem: "Choose a model." });
    await open();
    expect(screen.getByText("The assistant cannot answer yet.")).toBeTruthy();
    expect(screen.getByText("Choose a model.")).toBeTruthy();
    cleanup();
    server.settings = fixture({ enabled: { value: false, source: "default", locked: false }, configured: false, problem: "" });
    await open();
    expect(screen.queryByText("The assistant cannot answer yet.")).toBeNull();
    expect(screen.queryByText("Not ready to switch on.")).toBeNull();
  });

  it("shows a field that a flag or the environment sets as locked, with what sets it", async () => {
    server.settings = fixture({
      enabled: locked(true, "--ai"), provider: locked("openai", "--ai-provider"), model: locked("llama3.2", "--ai-model"),
      baseURL: locked("http://localhost:11434/v1", "KUBEDOJO_AI_BASE_URL"),
      apiKey: { set: true, last4: "x9Qd", source: "env", locked: true, lockedBy: "KUBEDOJO_AI_API_KEY" },
    });
    await open();
    expect(screen.getAllByText(/^Set by /).map((b) => b.textContent)).toEqual([
      "Set by --ai", "Set by --ai-provider", "Set by KUBEDOJO_AI_BASE_URL", "Set by --ai-model", "Set by KUBEDOJO_AI_API_KEY",
    ]);
    expect((screen.getByRole("switch", { name: "AI assistant" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getAllByRole("radio").every((r) => (r as HTMLInputElement).disabled)).toBe(true);
    expect(field("Base URL").disabled).toBe(true);
    expect(field("Model").disabled).toBe(true);
    // A locked address has no presets to click, and a locked key no buttons.
    expect(presets()).toEqual([]);
    expect(within(keyRow()).getByText("••••x9Qd")).toBeTruthy();
    expect(within(keyRow()).queryByRole("button")).toBeNull();
    expect(screen.getAllByText("Start kubedojo without it to change this here.").length).toBeGreaterThanOrEqual(2);
  });

  it("locks one field and leaves the others to change", async () => {
    server.settings = fixture({ model: locked("llama3.2", "--ai-model") });
    await open();
    expect(screen.getAllByText(/^Set by /).map((b) => b.textContent)).toEqual(["Set by --ai-model"]);
    expect(field("Model").disabled).toBe(true);
    expect(field("Base URL").disabled).toBe(false);
    change("Base URL", "http://localhost:1234/v1");
    await save();
    // The locked value is not sent: it did not change.
    expect(server.saved).toEqual([{ baseURL: "http://localhost:1234/v1" }]);
  });

  it("sends only what changed, and never a key that was not touched", async () => {
    server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
    await open();
    change("Model", "qwen2.5-coder");
    expect(screen.getByText("Unsaved changes")).toBeTruthy();
    expect(button("Save").disabled).toBe(false);
    await save();
    expect(server.saved).toEqual([{ model: "qwen2.5-coder" }]);
    // The form starts over from what the server stored.
    expect(field("Model").value).toBe("qwen2.5-coder");
    expect(screen.getByText("Saved")).toBeTruthy();
    expect(button("Save").disabled).toBe(true);
    expect(screen.queryByText("Unsaved changes")).toBeNull();

    fireEvent.click(screen.getByRole("switch", { name: "AI assistant" }));
    expect(screen.queryByText("Saved")).toBeNull();
    await save();
    expect(server.saved[1]).toEqual({ enabled: false });
  });

  it("is not changed by a value typed and taken back", async () => {
    await open();
    change("Model", "other");
    change("Model", "llama3.2");
    expect(button("Save").disabled).toBe(true);
    // Spaces around a value are not a change either.
    change("Base URL", " http://localhost:11434/v1 ");
    expect(button("Save").disabled).toBe(true);
    fireEvent.submit(button("Save").closest("form") as HTMLFormElement);
    expect(server.saved).toEqual([]);
  });

  it("tells the pages that show the assistant after a save, without a reload", async () => {
    await open();
    const before = server.configCalls;
    change("Model", "qwen2.5-coder");
    await save();
    await waitFor(() => expect(server.configCalls).toBe(before + 1));
  });

  it("fills in the address of a preset", async () => {
    await open();
    fireEvent.click(button("LM Studio"));
    expect(field("Base URL").value).toBe("http://localhost:1234/v1");
    expect(button("LM Studio").getAttribute("aria-pressed")).toBe("true");
    expect(button("Ollama").getAttribute("aria-pressed")).toBe("false");
    await save();
    expect(server.saved).toEqual([{ baseURL: "http://localhost:1234/v1" }]);
  });

  it("starts the model and the address over for another provider, and gives them back for the stored one", async () => {
    server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
    await open();
    fireEvent.click(provider(/Anthropic/));
    expect(provider(/Anthropic/).checked).toBe(true);
    expect(field("Model").value).toBe("");
    expect(field("Model").getAttribute("placeholder")).toBe("claude-opus-5-5");
    expect(field("Base URL").value).toBe("");
    // Presets belong to the provider that has them.
    expect(presets()).toEqual([]);
    expect(screen.getByText("The stored key was entered for another provider or server. It is removed when you save, unless you enter one.")).toBeTruthy();

    fireEvent.click(provider(/OpenAI-compatible/));
    expect(field("Model").value).toBe("llama3.2");
    expect(field("Base URL").value).toBe("http://localhost:11434/v1");
    expect(presets()).toHaveLength(4);
    expect(screen.queryByText(/The stored key was entered for another/)).toBeNull();
    expect(button("Save").disabled).toBe(true);

    fireEvent.click(provider(/Anthropic/));
    change("Model", "claude-sonnet-5-5");
    await save();
    // The provider with what belongs to it; the empty address means the provider's own.
    expect(server.saved).toEqual([{ provider: "anthropic", model: "claude-sonnet-5-5", baseURL: "" }]);
  });

  it("warns that a stored key goes with the server it was entered for", async () => {
    server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
    await open();
    // Another path on the same server keeps the key.
    change("Base URL", "http://localhost:11434/api/v1");
    expect(screen.queryByText(/The stored key was entered for another/)).toBeNull();
    change("Base URL", "https://openrouter.ai/api/v1");
    expect(screen.getByText(/The stored key was entered for another/)).toBeTruthy();
    // With a new key there is nothing to warn about.
    fireEvent.click(within(keyRow()).getByRole("button", { name: "Replace" }));
    expect(screen.queryByText(/The stored key was entered for another/)).toBeNull();
  });

  describe("the API key", () => {
    it("is never shown: only that there is one, and how it ends", async () => {
      server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
      const { container } = await open();
      expect(keyRow().textContent).toContain("Set · ends in ••••x9Qd");
      expect(within(keyRow()).getAllByRole("button").map((b) => b.textContent)).toEqual(["Replace", "Remove"]);
      expect(container.querySelector('input[type="password"]')).toBeNull();
      cleanup();
      // A short key has no last characters to show.
      server.settings = fixture({ apiKey: { set: true, source: "file", locked: false } });
      await open();
      expect(within(keyRow()).getByText("Set")).toBeTruthy();
      expect(keyRow().textContent).not.toContain("••••");
    });

    it("offers to set one where there is none", async () => {
      await open();
      expect(within(keyRow()).getAllByRole("button").map((b) => b.textContent)).toEqual(["Set a key"]);
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Set a key" }));
      const input = field("API key");
      expect([input.type, input.getAttribute("autocomplete"), input.value]).toEqual(["password", "off", ""]);
      await waitFor(() => expect(document.activeElement).toBe(input));
      // Nothing typed is nothing to save.
      expect(button("Save").disabled).toBe(true);
      fireEvent.change(input, { target: { value: "  sk-test-1234567890abcd  " } });
      await save();
      expect(server.saved).toEqual([{ apiKey: "sk-test-1234567890abcd" }]);
      expect(keyRow().textContent).toContain("Set · ends in ••••abcd");
      expect(screen.queryByLabelText("API key")).toBeNull();
    });

    it("replaces a key through an empty password field, and can take that back", async () => {
      server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
      await open();
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Replace" }));
      const input = field("API key");
      expect([input.type, input.getAttribute("autocomplete"), input.value]).toEqual(["password", "off", ""]);
      fireEvent.change(input, { target: { value: "sk-new" } });
      expect(button("Save").disabled).toBe(false);
      fireEvent.click(button("Cancel"));
      expect(keyRow().textContent).toContain("Set · ends in ••••x9Qd");
      expect(button("Save").disabled).toBe(true);
      // Opened again it is empty again.
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Replace" }));
      expect(field("API key").value).toBe("");
    });

    it("removes a key by sending an empty one, after a save", async () => {
      server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
      await open();
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Remove" }));
      expect(within(keyRow()).getByText("Will be removed when you save")).toBeTruthy();
      expect(server.saved).toEqual([]);
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Keep it" }));
      expect(keyRow().textContent).toContain("Set · ends in ••••x9Qd");
      expect(button("Save").disabled).toBe(true);

      fireEvent.click(within(keyRow()).getByRole("button", { name: "Remove" }));
      await save();
      expect(server.saved).toEqual([{ apiKey: "" }]);
      expect(within(keyRow()).getByText("Not set")).toBeTruthy();
    });
  });

  describe("a save that is refused", () => {
    it.each([
      [409, "model", "Model", "model is set by --ai-model and cannot be changed here."],
      [400, "baseURL", "Base URL", "Plain http is allowed for local and private addresses only."],
    ])("shows a %i about %s at that field", async (status, name, label, message) => {
      await open();
      change("Model", "other");
      change("Base URL", "http://example.org/v1");
      server.saveError = new ApiError(status, message, undefined, name);
      await save();
      const input = field(label);
      expect(input.getAttribute("aria-invalid")).toBe("true");
      const alert = screen.getByRole("alert");
      expect(alert.textContent).toBe(message);
      expect(input.getAttribute("aria-describedby")).toContain(alert.id);
      // What was typed is still there, and can be sent again.
      expect(field("Model").value).toBe("other");
      expect(button("Save").disabled).toBe(false);
      // Editing the field takes its error away.
      change(label, "x");
      expect(screen.queryByRole("alert")).toBeNull();
    });

    it("shows an error about the key and about the provider at their place", async () => {
      await open();
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Set a key" }));
      fireEvent.change(field("API key"), { target: { value: "has space" } });
      server.saveError = new ApiError(400, "apiKey contains white space or a control character.", undefined, "apiKey");
      await save();
      expect(within(keyRow().parentElement as HTMLElement).getByRole("alert").textContent).toBe("apiKey contains white space or a control character.");
      expect(field("API key").getAttribute("aria-invalid")).toBe("true");

      server.saveError = new ApiError(409, "provider is set by ANTHROPIC_API_KEY and cannot be changed here.", undefined, "provider");
      fireEvent.click(provider(/Anthropic/));
      await save();
      expect(screen.getAllByRole("alert").map((a) => a.textContent)).toEqual(["provider is set by ANTHROPIC_API_KEY and cannot be changed here."]);
    });

    it("shows an error that names no field next to the button", async () => {
      await open();
      const before = server.configCalls;
      change("Model", "other");
      server.saveError = new ApiError(403, "The settings of a hosted server cannot be changed.", "hosted");
      await save();
      expect(screen.getByRole("alert").textContent).toBe("The settings of a hosted server cannot be changed.");
      // Nothing changed, so nobody is told.
      expect(server.configCalls).toBe(before);
    });
  });

  describe("Test connection", () => {
    it("tests the form as it stands, without saving it, and says what answered", async () => {
      server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
      server.holdTest = true;
      await open();
      change("Model", "qwen2.5-coder");
      server.testResult = { ok: true, provider: "ollama", model: "qwen2.5-coder", latencyMs: 412 };
      fireEvent.click(button("Test connection"));
      // While it runs it cannot be started again.
      await waitFor(() => expect(button("Test connection").disabled).toBe(true));
      expect(screen.getByText("Asking the model for one word ...")).toBeTruthy();
      await act(async () => { server.releaseTest?.(); });
      expect(await screen.findByText("Connected to qwen2.5-coder in 412 ms")).toBeTruthy();
      expect(button("Test connection").disabled).toBe(false);
      // Provider, model and address of the form; no switch, and no key: the stored one is used.
      expect(server.tested).toEqual([{ provider: "openai", model: "qwen2.5-coder", baseURL: "http://localhost:11434/v1" }]);
      expect(server.saved).toEqual([]);
      expect(button("Save").disabled).toBe(false);
      // The result is about the values it was run with.
      change("Model", "other");
      expect(screen.queryByText(/^Connected to/)).toBeNull();
    });

    it("says what the server found when the connection fails", async () => {
      server.testResult = { ok: false, code: "unreachable", message: "Could not reach http://localhost:11434. Is Ollama running?" };
      await open();
      fireEvent.click(button("Test connection"));
      expect(await screen.findByText("Could not reach http://localhost:11434. Is Ollama running?")).toBeTruthy();
      expect(screen.queryByText(/^Connected to/)).toBeNull();
    });

    it("tests with a key that was typed, and without one that is to be removed", async () => {
      server.settings = fixture({ apiKey: { set: true, last4: "x9Qd", source: "file", locked: false } });
      await open();
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Replace" }));
      fireEvent.change(field("API key"), { target: { value: "sk-new" } });
      fireEvent.click(button("Test connection"));
      await screen.findByText(/^Connected to/);
      fireEvent.click(button("Cancel"));
      fireEvent.click(within(keyRow()).getByRole("button", { name: "Remove" }));
      fireEvent.click(button("Test connection"));
      await screen.findByText(/^Connected to/);
      expect(server.tested.map((t) => t.apiKey)).toEqual(["sk-new", ""]);
    });

    it("shows a value the server refuses at its field, and a rate limit with its wait", async () => {
      await open();
      server.testError = new ApiError(400, "Plain http is allowed for local and private addresses only.", undefined, "baseURL");
      fireEvent.click(button("Test connection"));
      expect((await screen.findByRole("alert")).textContent).toBe("Plain http is allowed for local and private addresses only.");
      expect(field("Base URL").getAttribute("aria-invalid")).toBe("true");

      server.testError = new ApiError(429, "Too many messages.", "rate_limited", undefined, 12);
      fireEvent.click(button("Test connection"));
      expect(await screen.findByText("Too many requests. Try again in 12 seconds.")).toBeTruthy();
    });
  });

  it("is read-only on a hosted server: what is set, by what, and no key details", async () => {
    server.settings = fixture({
      enabled: locked(true, "--ai"), provider: locked("openai", "KUBEDOJO_AI_PROVIDER"), model: locked("llama3.2", "KUBEDOJO_AI_MODEL"),
      baseURL: locked("http://ollama.ai.svc:11434/v1", "KUBEDOJO_AI_BASE_URL"),
      apiKey: { set: true, source: "env", locked: true, lockedBy: "KUBEDOJO_AI_API_KEY" },
    }, { writable: false, hosted: true });
    const { container } = await open();
    expect(screen.getByText("This kubedojo is configured by its administrator.")).toBeTruthy();
    expect(screen.getByText("How the assistant of this kubedojo is set up.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Test connection" })).toBeNull();
    expect(field("Base URL").value).toBe("http://ollama.ai.svc:11434/v1");
    expect(field("Model").value).toBe("llama3.2");
    for (const control of container.querySelectorAll("input, button[role=switch]")) expect((control as HTMLInputElement).matches(":disabled"), control.outerHTML).toBe(true);
    expect(presets()).toEqual([]);
    expect(within(keyRow()).getByText("Set")).toBeTruthy();
    expect(keyRow().textContent).not.toContain("••••");
    expect(within(keyRow()).queryByRole("button")).toBeNull();
    expect(screen.getByText("The key is set by the administrator and is never shown.")).toBeTruthy();
    // How to fill in a field says nothing to somebody who cannot.
    expect(screen.queryByText("Start kubedojo without it to change this here.")).toBeNull();
    expect(screen.queryByText(/Leave it empty/)).toBeNull();
    expect(screen.getByRole("heading", { name: "What the assistant sees" })).toBeTruthy();
  });

  it("is read-only on a hosted server whose values are defaults, too", async () => {
    server.settings = fixture({ enabled: { value: false, source: "default", locked: false }, configured: false, problem: "" }, { writable: false, hosted: true });
    const { container } = await open();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    for (const control of container.querySelectorAll("input, button[role=switch]")) expect((control as HTMLInputElement).matches(":disabled"), control.outerHTML).toBe(true);
    expect(within(keyRow()).queryByRole("button")).toBeNull();
  });

  it("says so when the settings cannot be loaded, and tries again", async () => {
    server.loadError = new ApiError(404, "This server has no assistant.");
    await open();
    expect(screen.getByText("The settings could not be loaded.")).toBeTruthy();
    expect(screen.getByText("This server has no assistant.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    server.loadError = null;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByLabelText("Model")).toBeTruthy();
    expect(screen.queryByText("The settings could not be loaded.")).toBeNull();
  });

  it("is a page like the others, with its place in the header", async () => {
    await open();
    expect(within(screen.getByRole("navigation", { name: "Breadcrumb" })).getByText("Settings").getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("link", { name: "Settings" }).getAttribute("href")).toBe("/settings");
    expect(screen.getByRole("contentinfo")).toBeTruthy();
  });
});
