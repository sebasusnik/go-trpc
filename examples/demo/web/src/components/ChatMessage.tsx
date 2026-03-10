type Props = {
  username: string;
  content: string;
  createdAt: string;
  isOwn: boolean;
};

export default function ChatMessage({
  username,
  content,
  createdAt,
  isOwn,
}: Props) {
  const time = new Date(createdAt).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });

  return (
    <div className={`flex flex-col ${isOwn ? "items-end" : "items-start"}`}>
      <div className="flex items-baseline gap-2 mb-0.5">
        <span className="mono text-[11px] font-medium text-zinc-500">
          {username}
        </span>
        <span className="text-[10px] text-zinc-300">{time}</span>
      </div>
      <div
        className={`max-w-[75%] rounded-2xl px-3.5 py-2 text-[13px] leading-relaxed ${
          isOwn
            ? "bg-go-blue text-white rounded-br-sm shadow-sm"
            : "bg-zinc-100/80 text-zinc-700 rounded-bl-sm"
        }`}
      >
        {content}
      </div>
    </div>
  );
}
