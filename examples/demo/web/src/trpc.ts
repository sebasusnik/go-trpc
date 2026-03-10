import { createTRPCClient, httpLink, TRPCClientError } from "@trpc/client";
import type { AppRouter } from "./generated/router";

// --- Request Logging (demo-specific) ---

export type LogEntry = {
  id: number;
  timestamp: string;
  method: "GET" | "POST";
  path: string;
  input?: unknown;
  output?: unknown;
  error?: string;
  duration: number;
};

let logId = 0;
let logListeners: ((entry: LogEntry) => void)[] = [];

export function onLog(listener: (entry: LogEntry) => void) {
  logListeners.push(listener);
  return () => {
    logListeners = logListeners.filter((l) => l !== listener);
  };
}

function emitLog(entry: LogEntry) {
  for (const l of logListeners) {
    l(entry);
  }
}

// --- Client ---

const baseUrl = import.meta.env.VITE_API_URL
  ? `${import.meta.env.VITE_API_URL}trpc`
  : "/trpc";

export const trpc = createTRPCClient<AppRouter>({
  links: [
    httpLink({
      url: baseUrl,
      async fetch(url, options) {
        const method = (options?.method ?? "GET") as "GET" | "POST";
        const path = new URL(
          url.toString(),
          window.location.origin,
        ).pathname.replace(/.*\/trpc\//, "");
        const start = performance.now();

        const res = await globalThis.fetch(url, options);
        const duration = performance.now() - start;

        // Clone so tRPC can still read the body
        const clone = res.clone();
        try {
          const data = await clone.json();
          const ok = !data.error;
          emitLog({
            id: ++logId,
            timestamp: new Date().toISOString(),
            method,
            path,
            input: undefined,
            ...(ok
              ? { output: data.result?.data }
              : { error: data.error?.message ?? "Unknown error" }),
            duration: Math.round(duration),
          });
        } catch {
          // non-JSON response, just log the failure
          emitLog({
            id: ++logId,
            timestamp: new Date().toISOString(),
            method,
            path,
            error: `HTTP ${res.status}`,
            duration: Math.round(duration),
          });
        }

        return res;
      },
    }),
  ],
});

export { TRPCClientError as GoTRPCError };
