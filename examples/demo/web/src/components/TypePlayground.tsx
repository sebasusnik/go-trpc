import { useEffect, useRef, useState } from "react";
import { trpc, GoTRPCError } from "../trpc";
import CodeBlock from "./CodeBlock";
import { getHighlighter } from "./CodeBlock";

const defaultCode = `type Room struct {
    ID        string \`json:"id"\`
    Name      string \`json:"name"\`
    CreatedAt string \`json:"createdAt"\`
}

type Message struct {
    ID        string \`json:"id"\`
    RoomID    string \`json:"roomId"\`
    Username  string \`json:"username"\`
    Content   string \`json:"content"\`
    CreatedAt string \`json:"createdAt"\`
}`;

function HighlightedEditor({
  value,
  onChange,
  onSubmit,
}: {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
}) {
  const [html, setHtml] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    getHighlighter("go").then((h) => {
      const result = h.codeToHtml(value, { lang: "go", theme: "one-dark-pro" });
      if (!cancelled) setHtml(result);
    });
    return () => {
      cancelled = true;
    };
  }, [value]);

  const handleScroll = () => {
    if (textareaRef.current && preRef.current) {
      preRef.current.scrollTop = textareaRef.current.scrollTop;
      preRef.current.scrollLeft = textareaRef.current.scrollLeft;
    }
  };

  return (
    <div className="relative flex-1 min-h-0 overflow-hidden bg-zinc-950">
      {/* Highlighted layer (behind) */}
      <div
        ref={preRef}
        className="absolute inset-0 overflow-hidden pointer-events-none [&_pre]:!bg-transparent [&_pre]:p-3 [&_pre]:text-sm [&_pre]:m-0 [&_code]:text-sm"
        aria-hidden
        // biome-ignore lint/security/noDangerouslySetInnerHtml: Shiki output
        dangerouslySetInnerHTML={html ? { __html: html } : undefined}
      />
      {/* Textarea layer (on top, transparent text) */}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={handleScroll}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
            e.preventDefault();
            onSubmit();
          }
        }}
        spellCheck={false}
        className="absolute inset-0 w-full h-full resize-none bg-transparent p-3 text-sm font-mono text-transparent caret-zinc-300 focus:outline-none selection:bg-zinc-700/50"
        placeholder="Type Go struct definitions here..."
      />
      {/* Show placeholder when empty (since text is transparent) */}
      {!value && (
        <div className="absolute top-3 left-3 text-sm font-mono text-zinc-600 pointer-events-none">
          Type Go struct definitions here...
        </div>
      )}
    </div>
  );
}

export default function TypePlayground() {
  const [code, setCode] = useState(defaultCode);
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleConvert = async () => {
    if (!code.trim()) return;

    setLoading(true);
    setError("");
    setOutput("");

    try {
      const result = await trpc.playground.convert.query({ code });
      if (result.error) {
        setError(result.error);
      } else {
        setOutput(result.typescript);
      }
    } catch (err) {
      if (err instanceof GoTRPCError) {
        setError(err.message);
      } else {
        setError("Failed to connect to the API");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Input section */}
      <div className="flex-1 flex flex-col min-h-0">
        <div className="flex items-center justify-between px-3 py-2 border-b border-zinc-200">
          <span className="text-xs font-medium text-zinc-500">Go Types</span>
          <button
            type="button"
            onClick={handleConvert}
            disabled={loading || !code.trim()}
            className="rounded bg-go-blue px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            {loading ? "Converting..." : "Convert"}
          </button>
        </div>
        <HighlightedEditor
          value={code}
          onChange={setCode}
          onSubmit={handleConvert}
        />
      </div>

      {/* Output section */}
      <div className="flex-1 flex flex-col min-h-0 border-t border-zinc-700">
        <div className="flex items-center px-3 py-2 border-b border-zinc-200 bg-white">
          <span className="text-xs font-medium text-zinc-500">
            TypeScript Output
          </span>
          <span className="ml-2 text-[10px] text-zinc-400">
            {"\u2318"}+Enter to convert
          </span>
        </div>
        <div className="flex-1 overflow-auto bg-zinc-950">
          {error ? (
            <div className="p-3 text-sm text-red-400 font-mono">{error}</div>
          ) : output ? (
            <CodeBlock code={output} lang="typescript" />
          ) : (
            <div className="p-3 text-sm text-zinc-600 font-mono">
              Click "Convert" to generate TypeScript types
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
