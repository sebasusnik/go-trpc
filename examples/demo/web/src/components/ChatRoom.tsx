import { useEffect, useRef, useState } from "react";
import { trpc, GoTRPCError } from "../trpc";
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
};

export default function ChatRoom({ roomId, roomName, username }: Props) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const apiUrl = import.meta.env.VITE_API_URL
    ? `${import.meta.env.VITE_API_URL}trpc`
    : "/trpc";

  // Load message history
  useEffect(() => {
    setMessages([]);
    trpc.room.messages
      .query({ roomId })
      .then((res) => setMessages(res.messages))
      .catch(() => setError("Failed to load messages"));
  }, [roomId]);

  // Subscribe to new messages via SSE
  useEffect(() => {
    const url = `${apiUrl}/chat.subscribe?input=${encodeURIComponent(
      JSON.stringify({ json: { roomId } })
    )}`;
    const es = new EventSource(url);

    es.addEventListener("data", (e) => {
      try {
        const parsed = JSON.parse(e.data);
        const msg = parsed.result?.data as Message;
        if (msg) {
          setMessages((prev) => [...prev, msg]);
        }
      } catch {
        // ignore parse errors
      }
    });

    es.addEventListener("error", () => {
      // EventSource auto-reconnects
    });

    return () => es.close();
  }, [roomId, apiUrl]);

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const handleSend = async () => {
    const content = input.trim();
    if (!content || sending) return;

    setSending(true);
    setError("");
    setInput("");

    try {
      await trpc.chat.send.mutate({ roomId, username, content });
    } catch (err) {
      if (err instanceof GoTRPCError) {
        setError(err.message);
      } else {
        setError("Failed to send message");
      }
      setInput(content); // restore on failure
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Room header */}
      <div className="border-b border-zinc-200 px-4 py-3">
        <h3 className="text-sm font-semibold text-zinc-800">#{roomName}</h3>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {messages.length === 0 && (
          <p className="text-center text-sm text-zinc-400 mt-8">
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
        <div className="mx-4 mb-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600">
          {error}
          <button type="button" onClick={() => setError("")} className="ml-2 font-medium cursor-pointer">
            Dismiss
          </button>
        </div>
      )}

      {/* Input */}
      <div className="border-t border-zinc-200 p-3">
        <div className="flex gap-2">
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
            className="flex-1 rounded-lg border border-zinc-200 px-3 py-2 text-sm focus:border-go-blue focus:outline-none"
          />
          <button
            type="button"
            onClick={handleSend}
            disabled={!input.trim() || sending}
            className="rounded-lg bg-go-blue px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}
