import { useEffect, useId, useRef, useState } from "react";
import { Check, CircleAlert, Lock, PlugZap, TriangleAlert } from "lucide-react";
import { api, ApiError, type ProviderInfo, type Settings, type SettingsPatch } from "../api";
import { refreshConfig } from "../config";
import { useLoad } from "../hooks";
import { plural, t } from "../i18n";
import { Badge, Button, Card, ErrorNote, Field, Input, Loading, Page, Skeleton, Switch, cx } from "../components/ui";

/** The settings of this kubedojo: for now, the assistant and the model behind it. */
export function SettingsPage() {
  const { data, error, reload, setData } = useLoad(() => api.settings(), []);
  // Counts the saves: the form starts over from what the server stored.
  const [version, setVersion] = useState(0);
  const [saved, setSaved] = useState(false);
  return (
    <Page crumbs={[{ label: t("Settings") }]}>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="text-3xl font-bold tracking-tight">{t("Settings")}</h1>
          <p className="text-body text-muted">
            {data && !data.writable
              ? t("How the assistant of this kubedojo is set up.")
              : t("Stored on this machine in your kubedojo data directory. Values set by a flag or an environment variable are shown locked.")}
          </p>
        </div>
        {error && <ErrorNote onRetry={() => void reload()}>{t("The settings could not be loaded.")} <span className="font-mono text-xs break-words">{error}</span></ErrorNote>}
        {!data && !error && (
          <Loading label={t("Loading the settings")}>
            <Card padded className="space-y-5">
              <Skeleton className="h-6 w-40" />
              {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-11" />)}
            </Card>
          </Loading>
        )}
        {data && (
          <>
            {!data.writable && (
              <p className="flex items-start gap-2.5 rounded-xl border bg-panel2 px-4 py-3 text-sm">
                <Lock className="mt-0.5 size-4 shrink-0 text-muted" aria-hidden />{t("This kubedojo is configured by its administrator.")}
              </p>
            )}
            <AssistantForm key={version} settings={data} saved={saved} onEdit={() => setSaved(false)}
              onSaved={(next) => { setData(next); setVersion((n) => n + 1); setSaved(true); void refreshConfig().catch(() => undefined); }} />
            <Sees settings={data} />
          </>
        )}
      </div>
    </Page>
  );
}

const describeProvider = (p: ProviderInfo): string => {
  if (p.id === "anthropic") return t("Claude models. Needs an API key.");
  if (p.presets?.length) return t("{list} and other servers with the OpenAI API", { list: p.presets.map((x) => x.label).join(", ") });
  return p.needsKey ? t("Needs an API key.") : t("A key only if the server wants one.");
};

/** Scheme, host and port of an address, or the text itself when it is none. A stored key belongs to one of them. */
function originOf(url: string): string {
  try { return new URL(url).origin; } catch { return url.trim(); }
}

type KeyEdit = { mode: "keep" } | { mode: "replace"; value: string } | { mode: "remove" };
type TestState = { state: "idle" } | { state: "running" } | { state: "ok"; model: string; ms: number } | { state: "failed"; text: string };

function AssistantForm({ settings, saved, onEdit, onSaved }: {
  settings: Settings; saved: boolean; onEdit: () => void; onSaved: (next: Settings) => void;
}) {
  const a = settings.assistant;
  const editable = settings.writable;
  const [enabled, setEnabled] = useState(a.enabled.value);
  const [provider, setProvider] = useState(a.provider.value);
  const [model, setModel] = useState(a.model.value);
  const [baseURL, setBaseURL] = useState(a.baseURL.value);
  const [key, setKey] = useState<KeyEdit>({ mode: "keep" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [failure, setFailure] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [test, setTest] = useState<TestState>({ state: "idle" });
  const keyInput = useRef<HTMLInputElement>(null);
  const providerLegend = useId();

  const info = settings.providers.find((p) => p.id === provider);
  const switched = provider !== a.provider.value;
  const touch = (field: string) => {
    onEdit();
    setTest({ state: "idle" });
    setFailure(null);
    setErrors((e) => (e[field] ? { ...e, [field]: "" } : e));
  };
  const chooseProvider = (id: string) => {
    if (id === provider) return;
    touch("provider");
    setProvider(id);
    // The model and the address of one provider mean nothing to another. Back
    // on the stored provider its stored values return; a locked value stays.
    const back = id === a.provider.value;
    if (!a.model.locked) setModel(back ? a.model.value : "");
    if (!a.baseURL.locked) setBaseURL(back ? a.baseURL.value : "");
  };

  // What differs from what the server has. A field that was not touched is not sent.
  const changes = (): SettingsPatch => {
    const p: SettingsPatch = {};
    if (enabled !== a.enabled.value) p.enabled = enabled;
    if (switched) p.provider = provider;
    if (model.trim() !== a.model.value || (switched && !a.model.locked)) p.model = model.trim();
    if (baseURL.trim() !== a.baseURL.value || (switched && !a.baseURL.locked)) p.baseURL = baseURL.trim();
    if (key.mode === "replace" && key.value.trim() !== "") p.apiKey = key.value.trim();
    if (key.mode === "remove") p.apiKey = "";
    return p;
  };
  const patch = changes();
  const dirty = Object.keys(patch).length > 0;
  // A stored key belongs to the provider and the server it was entered for.
  const keyGoes = a.apiKey.set && !a.apiKey.locked && key.mode === "keep"
    && (switched || (patch.baseURL !== undefined && originOf(baseURL) !== originOf(a.baseURL.value)));

  const refused = (e: unknown) => {
    if (e instanceof ApiError && e.field) setErrors((x) => ({ ...x, [e.field as string]: e.message }));
    else if (e instanceof ApiError && e.status === 429) {
      setFailure(e.retryAfter ? plural(e.retryAfter, "Too many requests. Try again in {n} second.", "Too many requests. Try again in {n} seconds.") : t("Too many requests. Try again in a moment."));
    } else setFailure(e instanceof Error && e.message ? e.message : t("kubedojo could not be reached."));
  };
  const save = async () => {
    if (!dirty || saving) return;
    setSaving(true); setFailure(null); setErrors({});
    try { onSaved(await api.saveSettings(patch)); }
    catch (e) { refused(e); setSaving(false); }
  };
  const run = async () => {
    setTest({ state: "running" }); setFailure(null); setErrors({});
    // The form as it stands, saved or not. The key only if one was typed: without it the stored one is used.
    const body: SettingsPatch = { provider, model: model.trim(), baseURL: baseURL.trim() };
    if (patch.apiKey !== undefined) body.apiKey = patch.apiKey;
    try {
      const res = await api.testSettings(body);
      setTest(res.ok ? { state: "ok", model: res.model, ms: res.latencyMs } : { state: "failed", text: res.message || t("The connection failed.") });
    } catch (e) {
      setTest({ state: "idle" });
      refused(e);
    }
  };

  const lock = (f: { locked: boolean; lockedBy?: string }) => (f.locked && f.lockedBy ? t("Set by {by}", { by: f.lockedBy }) : undefined);
  // Under a field: how to fill it in, or how to get it back into one's own hands. Nothing where nothing can be changed.
  const note = (f: { locked: boolean }, text: string) => (!editable ? undefined : f.locked ? t("Start kubedojo without it to change this here.") : text);

  return (
    <Card className="overflow-hidden">
      <form onSubmit={(e) => { e.preventDefault(); void save(); }} noValidate>
        <fieldset disabled={!editable} className="contents">
          <div className="border-b px-5 py-5 sm:px-6">
            <Field label={t("AI assistant")} layout="row" locked={lock(a.enabled)} error={errors.enabled}
              description={t("Explains failed checks from your task, the check results and a read-only look at your sandbox.")}>
              <Switch checked={enabled} onChange={(v) => { touch("enabled"); setEnabled(v); }} disabled={!editable} />
            </Field>
          </div>

          <div className="flex flex-col gap-6 px-5 py-6 sm:px-6">
            {!a.configured && a.problem && (
              <p className="flex items-start gap-2.5 rounded-xl border border-warn/40 bg-warn-soft px-4 py-3 text-sm">
                <CircleAlert className="mt-0.5 size-4 shrink-0 text-warn" aria-hidden />
                <span><span className="font-medium">{a.enabled.value ? t("The assistant cannot answer yet.") : t("Not ready to switch on.")}</span> {a.problem}</span>
              </p>
            )}

            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span id={providerLegend} className="text-sm font-medium">{t("Provider")}</span>
                {lock(a.provider) && <LockBadge>{lock(a.provider)}</LockBadge>}
              </div>
              <div role="radiogroup" aria-labelledby={providerLegend} className="grid gap-2.5 sm:grid-cols-2">
                {settings.providers.map((p) => (
                  <label key={p.id} className={cx("flex min-h-16 flex-col justify-center rounded-xl border px-3.5 py-3 transition has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent",
                    p.id === provider ? "border-accent bg-accent-soft" : "bg-bg",
                    a.provider.locked || !editable ? "cursor-not-allowed" : "cursor-pointer hover:border-accent/60",
                    (a.provider.locked || !editable) && p.id !== provider && "opacity-60")}>
                    <input type="radio" name="provider" value={p.id} checked={p.id === provider} disabled={a.provider.locked || !editable}
                      onChange={() => chooseProvider(p.id)} className="sr-only" />
                    <span className="text-sm font-semibold">{p.label}</span>
                    <span className="pt-0.5 text-label text-muted">{describeProvider(p)}</span>
                  </label>
                ))}
              </div>
              {errors.provider && <p role="alert" className="text-label font-medium text-bad">{errors.provider}</p>}
            </div>

            <Field label={t("Base URL")} locked={lock(a.baseURL)} error={errors.baseURL}
              description={note(a.baseURL, t("Where the model is served. Leave it empty for the address of the provider itself."))}>
              <Input mono value={baseURL} onChange={(e) => { touch("baseURL"); setBaseURL(e.target.value); }} inputMode="url" spellCheck={false} autoComplete="off"
                placeholder={t("The provider's own address")} />
              {info?.presets && info.presets.length > 0 && !a.baseURL.locked && editable && (
                <div className="flex flex-wrap gap-1.5">
                  {info.presets.map((preset) => (
                    <button key={preset.baseURL} type="button" aria-pressed={baseURL.trim() === preset.baseURL}
                      onClick={() => { touch("baseURL"); setBaseURL(preset.baseURL); }}
                      className={cx("h-8 rounded-2xl border px-3 text-label transition",
                        baseURL.trim() === preset.baseURL ? "border-accent bg-accent-soft text-accent" : "bg-panel2 hover:border-accent/60")}>
                      {preset.label}
                    </button>
                  ))}
                </div>
              )}
            </Field>

            <Field label={t("Model")} locked={lock(a.model)} error={errors.model}
              description={note(a.model, info?.defaultModel
                ? t("The name the provider knows the model by. Empty means {model}.", { model: info.defaultModel })
                : t("The name the server knows the model by, such as llama3.2."))}>
              <Input mono value={model} onChange={(e) => { touch("model"); setModel(e.target.value); }} spellCheck={false} autoComplete="off"
                placeholder={info?.defaultModel ?? ""} />
            </Field>

            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-sm font-medium" id="settings-key">{t("API key")}</span>
                {lock(a.apiKey) && <LockBadge>{lock(a.apiKey)}</LockBadge>}
              </div>
              {key.mode === "replace" ? (
                <div className="flex flex-wrap items-center gap-2">
                  {/* Never filled in advance: the key cannot be read back, and a password manager has no business here. */}
                  <Input ref={keyInput} type="password" mono autoComplete="off" spellCheck={false} aria-labelledby="settings-key" aria-invalid={errors.apiKey ? true : undefined}
                    value={key.value} onChange={(e) => { touch("apiKey"); setKey({ mode: "replace", value: e.target.value }); }}
                    placeholder={t("Paste the key")} className="min-w-0 flex-1 basis-56" />
                  <Button type="button" onClick={() => { touch("apiKey"); setKey({ mode: "keep" }); }}>{t("Cancel")}</Button>
                </div>
              ) : (
                <div className="flex min-h-11 flex-wrap items-center gap-2.5 rounded-[0.625rem] border bg-bg py-1.5 pr-1.5 pl-3.5">
                  <span className="min-w-0 flex-1 basis-36 py-1.5 text-sm">
                    {key.mode === "remove" ? t("Will be removed when you save")
                      : !a.apiKey.set ? <span className="text-muted">{t("Not set")}</span>
                        : a.apiKey.last4 ? <>{t("Set · ends in")} <span className="font-mono text-label">••••{a.apiKey.last4}</span></>
                          : t("Set")}
                  </span>
                  {editable && !a.apiKey.locked && (
                    <span className="flex gap-1">
                      {key.mode === "remove" ? (
                        <Button type="button" size="sm" onClick={() => { touch("apiKey"); setKey({ mode: "keep" }); }}>{t("Keep it")}</Button>
                      ) : (
                        <>
                          <Button type="button" size="sm" onClick={() => { touch("apiKey"); setKey({ mode: "replace", value: "" }); }}>{a.apiKey.set ? t("Replace") : t("Set a key")}</Button>
                          {a.apiKey.set && <Button type="button" size="sm" variant="ghost" className="text-bad hover:text-bad" onClick={() => { touch("apiKey"); setKey({ mode: "remove" }); }}>{t("Remove")}</Button>}
                        </>
                      )}
                    </span>
                  )}
                </div>
              )}
              {errors.apiKey && <p role="alert" className="text-label font-medium text-bad">{errors.apiKey}</p>}
              {keyGoes && <p className="text-label font-medium text-warn">{t("The stored key was entered for another provider or server. It is removed when you save, unless you enter one.")}</p>}
              <p className="text-label leading-relaxed text-muted">
                {!editable ? t("The key is set by the administrator and is never shown.")
                  : a.apiKey.locked ? t("Start kubedojo without it to change this here.")
                    : info?.needsKey ? t("The key is never shown again and never leaves this machine except to the provider above.")
                      : t("The key is never shown again and never leaves this machine except to the provider above. A local model such as Ollama needs none.")}
              </p>
            </div>
          </div>
        </fieldset>

        {editable && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2.5 border-t px-5 py-4 sm:px-6">
            <Button type="button" loading={test.state === "running"} icon={<PlugZap className="size-4" aria-hidden />} onClick={() => void run()}>{t("Test connection")}</Button>
            <p role="status" className="min-w-0 flex-1 basis-56 text-label">
              {test.state === "ok" && (
                <span className="inline-flex items-center gap-1.5 text-ok"><Check className="size-4 shrink-0" aria-hidden />{t("Connected to {model} in {ms} ms", { model: test.model, ms: test.ms })}</span>
              )}
              {test.state === "failed" && (
                <span className="inline-flex items-start gap-1.5 text-bad"><TriangleAlert className="mt-px size-4 shrink-0" aria-hidden /><span className="break-words">{test.text}</span></span>
              )}
              {test.state === "running" && <span className="text-muted">{t("Asking the model for one word ...")}</span>}
              {test.state === "idle" && saved && !dirty && <span className="inline-flex items-center gap-1.5 text-ok"><Check className="size-4 shrink-0" aria-hidden />{t("Saved")}</span>}
              {test.state === "idle" && dirty && <span className="text-muted">{t("Unsaved changes")}</span>}
            </p>
            <Button type="submit" variant="primary" loading={saving} disabled={!dirty} className="ml-auto">{t("Save")}</Button>
            {failure && <p role="alert" className="basis-full text-label font-medium text-bad">{failure}</p>}
          </div>
        )}
      </form>
      <FocusKey input={keyInput} when={key.mode === "replace"} />
    </Card>
  );
}

/** Moves the focus into the key input when it appears. */
function FocusKey({ input, when }: { input: React.RefObject<HTMLInputElement | null>; when: boolean }) {
  useEffect(() => { if (when) input.current?.focus(); }, [input, when]);
  return null;
}

function LockBadge({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md border bg-panel2 px-2 py-0.5 text-xs text-warn">
      <Lock className="size-3" aria-hidden />{children}
    </span>
  );
}

function Sees({ settings }: { settings: Settings }) {
  const on = settings.traceContent.value;
  return (
    <Card padded className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("What the assistant sees")}</h2>
      <ul className="list-disc space-y-1.5 pl-5 text-sm leading-relaxed text-muted marker:text-muted">
        <li>{t("The task text, your check results and the hints you already opened")}</li>
        <li>{t("Your note for the question and a short read-only snapshot of the sandbox")}</li>
        <li>{t("Never the solution unless you have viewed it, and never during an exam")}</li>
        <li>{t("No terminal history, no other question and no user name")}</li>
      </ul>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-t pt-4">
        <span className="min-w-0 flex-1 basis-64 text-sm">{t("Prompt and answer text in traces")}</span>
        <Badge tone={on ? "ok" : "muted"}>{on ? t("On") : t("Off")}</Badge>
        {settings.traceContent.reason && <p className="basis-full text-label text-muted">{t("Reason: {reason}", { reason: settings.traceContent.reason })}</p>}
      </div>
    </Card>
  );
}
