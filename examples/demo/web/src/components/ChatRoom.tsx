import { ChevronLeft, PanelLeftOpen, Send } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { emitLog, GoTRPCError, nextLogId, trpc } from "../trpc";
import ChatMessage from "./ChatMessage";

type Message = {
  id: string;
  roomId: string;
  username: string;
  content: string;
  createdAt: string;
};

type Props = {
  roomId: string;
  roomName: string;
  username: string;
  onBack?: () => void;
  onExpandSidebar?: () => void;
};

export default function ChatRoom({
  roomId,
  roomName,
  username,
  onBack,
  onExpandSidebar,
}: Props) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const seenIds = useRef(new Set<string>());

  const addMessage = useCallback((msg: Message) => {
    if (seenIds.current.has(msg.id)) return;
    seenIds.current.add(msg.id);
    setMessages((prev) => [...prev, msg]);
  }, []);

  // Load message history
  useEffect(() => {
    setMessages([]);
    seenIds.current = new Set();
    trpc.room.messages
      .query({ roomId })
      .then((res) => {
        const msgs = res.messages ?? [];
        for (const m of msgs) seenIds.current.add(m.id);
        setMessages(msgs);
      })
      .catch(() => setError("Failed to load messages"));
  }, [roomId]);

  // Subscribe to new messages via SSE
  useEffect(() => {
    const base = window.location.origin;
    const url = `${base}/trpc/chat.subscribe?input=${encodeURIComponent(
      JSON.stringify({ roomId }),
    )}`;
    const es = new EventSource(url);
    const start = performance.now();

    es.addEventListener("open", () => {
      emitLog({
        id: nextLogId(),
        timestamp: new Date().toISOString(),
        method: "SUB",
        path: "chat.subscribe",
        input: { roomId },
        duration: Math.round(performance.now() - start),
      });
    });

    es.addEventListener("data", (e) => {
      try {
        const parsed = JSON.parse(e.data);
        const msg = parsed.result?.data as Message;
        if (msg) {
          addMessage(msg);
          emitLog({
            id: nextLogId(),
            timestamp: new Date().toISOString(),
            method: "SUB",
            path: "chat.subscribe",
            output: msg,
            duration: 0,
          });
        }
      } catch {
        // ignore parse errors
      }
    });

    return () => es.close();
  }, [roomId, addMessage]);

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  });

  const handleSend = async () => {
    const content = input.trim();
    if (!content || sending) return;

    setSending(true);
    setError("");
    setInput("");

    try {
      const msg = await trpc.chat.send.mutate({ roomId, username, content });
      addMessage(msg);
    } catch (err) {
      if (err instanceof GoTRPCError) {
        setError(err.message);
      } else {
        setError("Failed to send message");
      }
      setInput(content);
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Room header */}
      <div className="flex h-10 items-center gap-2 border-b border-zinc-200/80 px-4">
        {onBack && (
          <button
            type="button"
            onClick={onBack}
            className="md:hidden text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
            aria-label="Back to rooms"
          >
            <ChevronLeft size={16} />
          </button>
        )}
        {onExpandSidebar && (
          <button
            type="button"
            onClick={onExpandSidebar}
            className="hidden md:flex items-center justify-center h-5 w-5 rounded text-zinc-300 hover:text-zinc-500 transition-colors cursor-pointer"
            aria-label="Show channels"
          >
            <PanelLeftOpen size={14} />
          </button>
        )}
        <h3 className="text-sm font-medium text-zinc-700">#{roomName}</h3>
        <span className="ml-auto rounded bg-go-blue/10 px-2 py-0.5 text-[11px] text-go-blue font-medium mono">
          {username}
        </span>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-5 space-y-4">
        {messages.length === 0 && (
          <p className="text-center text-sm text-zinc-300 mt-16">
            No messages yet. Be the first to say something!
          </p>
        )}
        {messages.map((msg) => (
          <ChatMessage
            key={msg.id}
            username={msg.username}
            content={msg.content}
            createdAt={msg.createdAt}
            isOwn={msg.username === username}
          />
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Error */}
      {error && (
        <div className="mx-4 mb-2 rounded-lg bg-red-50 border border-red-100 px-3 py-2 text-xs text-red-600">
          {error}
          <button
            type="button"
            onClick={() => setError("")}
            className="ml-2 font-medium cursor-pointer"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Input */}
      <div className="border-t border-zinc-200/80 p-3 pb-[max(calc(0.75rem+env(keyboard-inset-bottom,0px)),env(safe-area-inset-bottom,0.75rem))]">
        <div className="flex gap-3">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Type a message..."
            className="flex-1 rounded-xl border border-zinc-200 bg-zinc-50 px-4 py-2.5 text-sm placeholder:text-zinc-300 focus:border-go-blue focus:bg-white focus:outline-none focus:ring-1 focus:ring-go-blue/20 transition-colors"
          />
          <button
            type="button"
            onClick={handleSend}
            disabled={!input.trim() || sending}
            className="rounded-xl bg-go-blue px-3.5 py-2.5 text-white shadow-sm shadow-go-blue/25 transition-colors hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            aria-label="Send message"
          >
            <Send size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}
