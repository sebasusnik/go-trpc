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
  const seenIds = useRef(new Set<string>());

  const addMessage = (msg: Message) => {
    if (seenIds.current.has(msg.id)) return;
    seenIds.current.add(msg.id);
    setMessages((prev) => [...prev, msg]);
  };

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
      JSON.stringify({ roomId })
    )}`;
    const es = new EventSource(url);

    es.addEventListener("data", (e) => {
      try {
        const parsed = JSON.parse(e.data);
        const msg = parsed.result?.data as Message;
        if (msg) addMessage(msg);
      } catch {
        // ignore parse errors
      }
    });

    return () => es.close();
  }, [roomId]);

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
      <div className="border-t border-zinc-200 p-4">
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
            className="flex-1 rounded-lg border border-zinc-200 px-4 py-2.5 text-sm focus:border-go-blue focus:outline-none"
          />
          <button
            type="button"
            onClick={handleSend}
            disabled={!input.trim() || sending}
            className="rounded-lg bg-go-blue px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}
