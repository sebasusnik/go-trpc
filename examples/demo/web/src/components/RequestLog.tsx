import { useEffect, useRef, useState } from "react";
import { type LogEntry, onLog } from "../trpc";

export default function RequestLog() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [expandedId, setExpandedId] = useState<number | null>(null);

  useEffect(() => {
    return onLog((entry) => {
      setLogs((prev) => [entry, ...prev].slice(0, 50));
    });
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: intentionally scroll when logs.length changes
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: 0, behavior: "smooth" });
  }, [logs.length]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-2.5">
        <h3 className="text-sm font-semibold text-zinc-700">
          Request Log
          {logs.length > 0 && (
            <span className="ml-1.5 text-xs font-normal text-zinc-400">
              ({logs.length})
            </span>
          )}
        </h3>
        {logs.length > 0 && (
          <button
            type="button"
            onClick={() => setLogs([])}
            className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer"
          >
            Clear
          </button>
        )}
      </div>
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-2">
        {logs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-xs text-zinc-400">
            Interact with the task manager to see tRPC requests here
          </div>
        ) : (
          <div className="space-y-1.5">
            {logs.map((log) => (
              <div
                key={log.id}
                role="button"
                tabIndex={0}
                className="log-entry cursor-pointer rounded-md border border-zinc-100 bg-white p-2.5 text-xs transition-colors hover:border-zinc-200"
                onClick={() =>
                  setExpandedId(expandedId === log.id ? null : log.id)
                }
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setExpandedId(expandedId === log.id ? null : log.id);
                  }
                }}
              >
                <div className="flex items-center gap-2">
                  <span
                    className={`rounded px-1.5 py-0.5 font-mono font-bold ${
                      log.method === "GET"
                        ? "bg-emerald-100 text-emerald-700"
                        : "bg-violet-100 text-violet-700"
                    }`}
                  >
                    {log.method}
                  </span>
                  <span className="font-mono font-medium text-zinc-700">
                    /trpc/{log.path}
                  </span>
                  <span className="ml-auto text-zinc-400">
                    {log.duration}ms
                  </span>
                  {log.error ? (
                    <span className="rounded bg-red-100 px-1.5 py-0.5 text-red-700">
                      ERROR
                    </span>
                  ) : (
                    <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-emerald-700">
                      OK
                    </span>
                  )}
                </div>
                {expandedId === log.id && (
                  <div className="mt-2 space-y-1.5 border-t border-zinc-100 pt-2">
                    {log.input !== undefined && (
                      <div>
                        <span className="font-semibold text-zinc-500">
                          Input:
                        </span>
                        <pre className="code-block mt-0.5 overflow-x-auto rounded bg-zinc-50 p-2 text-zinc-700">
                          {JSON.stringify(log.input, null, 2)}
                        </pre>
                      </div>
                    )}
                    {log.output !== undefined && (
                      <div>
                        <span className="font-semibold text-zinc-500">
                          Output:
                        </span>
                        <pre className="code-block mt-0.5 overflow-x-auto rounded bg-zinc-50 p-2 text-zinc-700">
                          {JSON.stringify(log.output, null, 2)}
                        </pre>
                      </div>
                    )}
                    {log.error && (
                      <div>
                        <span className="font-semibold text-red-500">
                          Error:
                        </span>
                        <pre className="code-block mt-0.5 overflow-x-auto rounded bg-red-50 p-2 text-red-700">
                          {log.error}
                        </pre>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
