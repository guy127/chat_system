"use client";

import { useMemo, useSyncExternalStore } from "react";

const subscribe = (onChange: () => void) => {
  window.addEventListener("storage", onChange);
  return () => window.removeEventListener("storage", onChange);
};

/**
 * Reads a value derived from localStorage without a hydration mismatch: the
 * server render and first client render use `fallback`, then the stored value.
 */
export function useStored<T>(read: () => T, fallback: T): T {
  // Snapshot as a JSON string so an unchanged value keeps the same identity.
  const raw = useSyncExternalStore(
    subscribe,
    () => JSON.stringify(read()),
    () => null,
  );
  return useMemo(() => (raw === null ? fallback : (JSON.parse(raw) as T)), [raw, fallback]);
}
