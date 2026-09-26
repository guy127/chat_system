"use client";

import { useMemo, useSyncExternalStore } from "react";

// "storage" fires for other tabs; "smalltalk-storage" for writes in this tab (see lib/session).
const subscribe = (onChange: () => void) => {
  window.addEventListener("storage", onChange);
  window.addEventListener("smalltalk-storage", onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener("smalltalk-storage", onChange);
  };
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
