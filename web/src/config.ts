import { useEffect, useState } from "react";
import { api, type Config } from "./api";

// The configuration of the server does not change while the page is open:
// it is fetched once and shared by everything that asks for it.
let request: Promise<Config> | null = null;
let loaded: Config | null = null;

/** The server configuration, fetched on the first call. A failed fetch is not kept: the next call tries again. */
export function loadConfig(): Promise<Config> {
  if (!request) {
    const mine = api.config();
    request = mine;
    mine.then(
      (config) => { if (request === mine) loaded = config; },
      () => { if (request === mine) request = null; },
    );
  }
  return request;
}

/**
 * The server configuration, or null while it is loading and when it cannot be
 * loaded. Pass false to not fetch it, for a component that may not need it.
 */
export function useConfig(enabled = true): Config | null {
  const [config, setConfig] = useState<Config | null>(loaded);
  useEffect(() => {
    if (!enabled || config) return;
    let alive = true;
    loadConfig().then((c) => { if (alive) setConfig(c); }).catch(() => undefined);
    return () => { alive = false; };
  }, [enabled, config]);
  return config;
}

/** For tests only: forget the configuration, so that the next test fetches its own. */
export function resetConfigCache() {
  request = null;
  loaded = null;
}
