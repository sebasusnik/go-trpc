import { useEffect, useState } from "react";
import { GoTRPCError, trpc } from "../trpc";

type Room = {
  id: string;
  name: string;
  createdAt: string;
};

type Props = {
  activeRoomId: string | null;
  onSelectRoom: (room: Room) => void;
};

export default function RoomList({ activeRoomId, onSelectRoom }: Props) {
  const [rooms, setRooms] = useState<Room[]>([]);
  const [newRoomName, setNewRoomName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  // biome-ignore lint/correctness/useExhaustiveDependencies: intentional mount-only effect
  useEffect(() => {
    trpc.room.list
      .query()
      .then((res) => {
        setRooms(res.rooms);
        if (!activeRoomId && res.rooms.length > 0) {
          onSelectRoom(res.rooms[0]);
        }
      })
      .catch(() => setError("Failed to load rooms"));
  }, []);

  const handleCreate = async () => {
    const name = newRoomName.trim();
    if (!name || creating) return;

    setCreating(true);
    setError("");

    try {
      const room = await trpc.room.create.mutate({ name });
      setNewRoomName("");
      setRooms((prev) => [...prev, room]);
      onSelectRoom(room);
    } catch (err) {
      if (err instanceof GoTRPCError) {
        setError(err.message);
      } else {
        setError("Failed to create room");
      }
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="px-3 py-3 border-b border-zinc-200/80">
        <h2 className="text-[11px] font-semibold text-zinc-400 uppercase tracking-widest">
          Rooms
        </h2>
      </div>

      {/* Room list */}
      <div className="flex-1 overflow-y-auto">
        {rooms.map((room) => (
          <button
            type="button"
            key={room.id}
            onClick={() => onSelectRoom(room)}
            className={`w-full text-left px-3 py-2.5 text-sm transition-colors cursor-pointer ${
              activeRoomId === room.id
                ? "bg-go-blue/5 text-go-blue font-medium border-l-2 border-go-blue"
                : "text-zinc-500 hover:bg-zinc-50 border-l-2 border-transparent"
            }`}
          >
            # {room.name}
          </button>
        ))}
      </div>

      {/* Error */}
      {error && <div className="mx-3 mb-2 text-xs text-red-500">{error}</div>}

      {/* Create room */}
      <div className="border-t border-zinc-200/80 p-3">
        <div className="flex gap-1.5">
          <input
            type="text"
            value={newRoomName}
            onChange={(e) => setNewRoomName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleCreate();
            }}
            placeholder="New room..."
            className="flex-1 rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-1.5 text-xs placeholder:text-zinc-300 focus:border-go-blue focus:bg-white focus:outline-none transition-colors"
          />
          <button
            type="button"
            onClick={handleCreate}
            disabled={!newRoomName.trim() || creating}
            className="rounded-lg bg-go-blue px-2.5 py-1.5 text-xs font-medium text-white hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            +
          </button>
        </div>
      </div>
    </div>
  );
}
