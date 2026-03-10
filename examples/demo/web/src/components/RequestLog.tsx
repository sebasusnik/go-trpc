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
      <div className="flex items-center justify-between border-b border-zinc-200/80 px-4 py-2.5">
        <h3 className="text-sm font-medium text-zinc-700">
          Request Log
          {logs.length > 0 && (
            <span className="ml-1.5 text-[11px] font-normal text-zinc-300">
              ({logs.length})
            </span>
          )}
        </h3>
        {logs.length > 0 && (
          <button
            type="button"
            onClick={() => setLogs([])}
            className="text-[11px] text-zinc-400 hover:text-zinc-600 cursor-pointer"
          >
            Clear
          </button>
        )}
      </div>
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-2">
        {logs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-xs text-zinc-300">
            Interact with the chat to see tRPC requests here
          </div>
        ) : (
          <div className="space-y-1.5">
            {logs.map((log) => (
              <div
                key={log.id}
                role="button"
                tabIndex={0}
                className="log-entry cursor-pointer rounded-lg border border-zinc-100 bg-white p-3 text-xs transition-colors hover:border-zinc-200 hover:shadow-sm"
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
                      log.method === "SUB"
                        ? "bg-amber-50 text-amber-600"
                        : log.method === "GET"
                          ? "bg-emerald-50 text-emerald-600"
                          : "bg-violet-50 text-violet-600"
                    }`}
                  >
                    {log.method}
                  </span>
                  <span className="font-mono font-medium text-zinc-700">
                    /trpc/{log.path}
                  </span>
                  <span className="ml-auto text-zinc-300 tabular-nums">
                    {log.duration}ms
                  </span>
                  {log.error ? (
                    <span className="rounded bg-red-50 px-1.5 py-0.5 text-red-600 font-medium">
                      ERROR
                    </span>
                  ) : log.method === "SUB" ? (
                    <span className="rounded bg-amber-50 px-1.5 py-0.5 text-amber-600 font-medium">
                      {log.output ? "DATA" : "OPEN"}
                    </span>
                  ) : (
                    <span className="rounded bg-emerald-50 px-1.5 py-0.5 text-emerald-600 font-medium">
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
                        <pre className="code-block mt-0.5 overflow-x-auto rounded-md bg-zinc-50 p-2.5 text-zinc-600">
                          {JSON.stringify(log.input, null, 2)}
                        </pre>
                      </div>
                    )}
                    {log.output !== undefined && (
                      <div>
                        <span className="font-semibold text-zinc-500">
                          Output:
                        </span>
                        <pre className="code-block mt-0.5 overflow-x-auto rounded-md bg-zinc-50 p-2.5 text-zinc-600">
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
