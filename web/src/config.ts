import { useEffect, useState } from "react";
import { api, type Config } from "./api";

// The configuration of the server rarely changes while the page is open: it
// is fetched once and shared by everything that asks for it. The one thing
// that does change it is the settings page, which calls refreshConfig().
let request: Promise<Config> | null = null;
let loaded: Config | null = null;
const listeners = new Set<(config: Config) => void>();

function fetchConfig(): Promise<Config> {
  const mine = api.config();
  request = mine;
  mine.then(
    (config) => {
      if (request !== mine) return;
      loaded = config;
      listeners.forEach((tell) => tell(config));
    },
    // A failed fetch is not kept: what was known before stays in force.
    () => { if (request === mine) request = loaded ? Promise.resolve(loaded) : null; },
  );
  return mine;
}

/** The server configuration, fetched on the first call. A failed fetch is not kept: the next call tries again. */
export function loadConfig(): Promise<Config> {
  return request ?? fetchConfig();
}

/**
 * Fetches the configuration again and hands it to everything that shows it,
 * without a reload of the page: after the settings changed, or when the server
 * says the assistant is not what the page believes it to be.
 */
export function refreshConfig(): Promise<Config> {
  return fetchConfig();
}

/**
 * The server configuration, or null while it is loading and when it cannot be
 * loaded. Pass false to not fetch it, for a component that may not need it.
 */
export function useConfig(enabled = true): Config | null {
  const [config, setConfig] = useState<Config | null>(loaded);
  useEffect(() => {
    if (!enabled) return;
    let alive = true;
    const take = (c: Config) => { if (alive) setConfig(c); };
    listeners.add(take);
    if (loaded) take(loaded);
    else loadConfig().then(take).catch(() => undefined);
    return () => { alive = false; listeners.delete(take); };
  }, [enabled]);
  return config;
}

/** For tests only: forget the configuration, so that the next test fetches its own. */
export function resetConfigCache() {
  request = null;
  loaded = null;
}
