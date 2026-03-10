import { useEffect, useState } from "react";
import { trpc, GoTRPCError } from "../trpc";

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

  const loadRooms = async () => {
    try {
      const res = await trpc.room.list.query();
      setRooms(res.rooms);
      // Auto-select first room if none selected
      if (!activeRoomId && res.rooms.length > 0) {
        onSelectRoom(res.rooms[0]);
      }
    } catch {
      setError("Failed to load rooms");
    }
  };

  useEffect(() => {
    loadRooms();
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
      <div className="px-3 py-3 border-b border-zinc-200">
        <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">
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
                ? "bg-go-blue/10 text-go-blue font-medium border-l-2 border-go-blue"
                : "text-zinc-600 hover:bg-zinc-50 border-l-2 border-transparent"
            }`}
          >
            # {room.name}
          </button>
        ))}
      </div>

      {/* Error */}
      {error && (
        <div className="mx-3 mb-2 text-xs text-red-500">{error}</div>
      )}

      {/* Create room */}
      <div className="border-t border-zinc-200 p-3">
        <div className="flex gap-1.5">
          <input
            type="text"
            value={newRoomName}
            onChange={(e) => setNewRoomName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleCreate();
            }}
            placeholder="New room..."
            className="flex-1 rounded border border-zinc-200 px-2 py-1.5 text-xs focus:border-go-blue focus:outline-none"
          />
          <button
            type="button"
            onClick={handleCreate}
            disabled={!newRoomName.trim() || creating}
            className="rounded bg-go-blue px-2.5 py-1.5 text-xs font-medium text-white hover:bg-go-dark disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            +
          </button>
        </div>
      </div>
    </div>
  );
}
