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
        <span className="text-xs font-medium text-zinc-600">{username}</span>
        <span className="text-[10px] text-zinc-400">{time}</span>
      </div>
      <div
        className={`max-w-[80%] rounded-xl px-3 py-2 text-sm ${
          isOwn
            ? "bg-go-blue text-white rounded-br-sm"
            : "bg-zinc-100 text-zinc-800 rounded-bl-sm"
        }`}
      >
        {content}
      </div>
    </div>
  );
}
