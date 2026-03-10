import { useCallback, useEffect, useRef, useState } from "react";
import ChatRoom from "./components/ChatRoom";
import CodePanel from "./components/CodePanel";
import RequestLog from "./components/RequestLog";
import RoomList from "./components/RoomList";
import TypePlayground from "./components/TypePlayground";
import { trpc } from "./trpc";

type HealthStatus = {
  status: string;
  timestamp: string;
  version: string;
} | null;

type Room = {
  id: string;
  name: string;
  createdAt: string;
};

export default function App() {
  const [health, setHealth] = useState<HealthStatus>(null);
  const [bottomPanel, setBottomPanel] = useState<"log" | "code" | "playground">(
    "log",
  );
  const [sidebarWidth, setSidebarWidth] = useState(420);
  const [mobileView, setMobileView] = useState<"app" | "devtools">("app");
  const [activeRoom, setActiveRoom] = useState<Room | null>(null);
  const [username] = useState(() => {
    const stored = localStorage.getItem("chat-username");
    if (stored) return stored;
    const name = `user-${Math.random().toString(36).slice(2, 6)}`;
    localStorage.setItem("chat-username", name);
    return name;
  });
  const isDragging = useRef(false);

  const handleMouseDown = useCallback(() => {
    isDragging.current = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      const newWidth = window.innerWidth - e.clientX;
      setSidebarWidth(Math.min(700, Math.max(280, newWidth)));
    };

    const handleMouseUp = () => {
      isDragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
  }, []);

  useEffect(() => {
    trpc.health.check
      .query()
      .then(setHealth)
      .catch(() => setHealth(null));
  }, []);

  return (
    <div className="flex h-screen flex-col bg-zinc-100 text-zinc-900">
      {/* Header */}
      <header className="border-b border-zinc-200/80 bg-white/80 backdrop-blur-sm px-4 py-2.5 md:px-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 md:gap-3">
            <h1 className="text-sm font-semibold tracking-tight">
              <span className="text-go-blue">go-trpc</span>{" "}
              <span className="text-zinc-400 font-normal">demo</span>
            </h1>
            <span className="hidden sm:inline rounded bg-zinc-100 px-2 py-0.5 text-[11px] text-zinc-500 font-medium">
              Chat Rooms
            </span>
            <span className="hidden sm:inline rounded bg-go-blue/10 px-2 py-0.5 text-[11px] text-go-blue font-medium mono">
              {username}
            </span>
          </div>
          <div className="flex items-center gap-3 md:gap-4 text-xs">
            {/* Mobile view toggle */}
            <div className="flex md:hidden rounded-lg bg-zinc-100 p-0.5">
              <button
                type="button"
                onClick={() => setMobileView("app")}
                className={`rounded-md px-3 py-1 text-xs font-medium transition-colors cursor-pointer ${
                  mobileView === "app"
                    ? "bg-white text-zinc-900 shadow-sm"
                    : "text-zinc-500"
                }`}
              >
                Chat
              </button>
              <button
                type="button"
                onClick={() => setMobileView("devtools")}
                className={`rounded-md px-3 py-1 text-xs font-medium transition-colors cursor-pointer ${
                  mobileView === "devtools"
                    ? "bg-white text-zinc-900 shadow-sm"
                    : "text-zinc-500"
                }`}
              >
                Dev Tools
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <div
                className={`h-2 w-2 rounded-full ${health ? "bg-emerald-500" : "bg-red-500"}`}
              />
              <span className="text-zinc-500">
                {health ? `API v${health.version}` : "API offline"}
              </span>
            </div>
            <a
              href="https://github.com/sebasusnik/go-trpc"
              target="_blank"
              rel="noopener noreferrer"
              className="text-zinc-400 hover:text-zinc-700 transition-colors"
            >
              GitHub
            </a>
          </div>
        </div>
      </header>

      {/* Main content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left panel — Room list + Chat */}
        <main
          className={`flex-1 flex overflow-hidden ${mobileView !== "app" ? "hidden md:flex" : ""}`}
        >
          {/* Room sidebar — hidden on mobile when a room is active */}
          <div
            className={`w-full md:w-56 shrink-0 border-r border-zinc-200/80 bg-white ${
              activeRoom ? "hidden md:block" : ""
            }`}
          >
            <RoomList
              activeRoomId={activeRoom?.id ?? null}
              onSelectRoom={setActiveRoom}
            />
          </div>

          {/* Chat area — hidden on mobile when no room is active */}
          <div
            className={`flex-1 overflow-hidden bg-white ${
              !activeRoom ? "hidden md:block" : ""
            }`}
          >
            {activeRoom ? (
              <ChatRoom
                roomId={activeRoom.id}
                roomName={activeRoom.name}
                username={username}
                onBack={() => setActiveRoom(null)}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-zinc-300">
                Select a room to start chatting
              </div>
            )}
          </div>
        </main>

        {/* Drag handle — desktop only */}
        <div
          role="separator"
          tabIndex={0}
          aria-label="Resize panels"
          aria-valuenow={sidebarWidth}
          onMouseDown={handleMouseDown}
          onKeyDown={(e) => {
            if (e.key === "ArrowLeft")
              setSidebarWidth((w) => Math.min(700, w + 20));
            if (e.key === "ArrowRight")
              setSidebarWidth((w) => Math.max(280, w - 20));
          }}
          className="hidden md:block w-px cursor-col-resize bg-zinc-200 hover:bg-zinc-300 active:bg-zinc-400 transition-colors"
        />

        {/* Right panel — Request Log / Code */}
        <aside
          className={`flex min-w-0 flex-col border-l border-zinc-200/80 bg-white ${
            mobileView !== "devtools" ? "hidden md:flex" : "flex-1"
          } md:flex`}
          style={{
            width: mobileView === "devtools" ? undefined : sidebarWidth,
          }}
        >
          <div className="flex border-b border-zinc-200/80">
            <button
              type="button"
              onClick={() => setBottomPanel("log")}
              className={`flex-1 px-4 py-2.5 text-xs font-medium transition-colors cursor-pointer ${
                bottomPanel === "log"
                  ? "border-b-2 border-zinc-900 text-zinc-900"
                  : "text-zinc-400 hover:text-zinc-600"
              }`}
            >
              Request Log
            </button>
            <button
              type="button"
              onClick={() => setBottomPanel("code")}
              className={`flex-1 px-4 py-2.5 text-xs font-medium transition-colors cursor-pointer ${
                bottomPanel === "code"
                  ? "border-b-2 border-zinc-900 text-zinc-900"
                  : "text-zinc-400 hover:text-zinc-600"
              }`}
            >
              Source Code
            </button>
            <button
              type="button"
              onClick={() => setBottomPanel("playground")}
              className={`flex-1 px-4 py-2.5 text-xs font-medium transition-colors cursor-pointer ${
                bottomPanel === "playground"
                  ? "border-b-2 border-zinc-900 text-zinc-900"
                  : "text-zinc-400 hover:text-zinc-600"
              }`}
            >
              Playground
            </button>
          </div>
          <div className="flex-1 overflow-hidden">
            {bottomPanel === "log" ? (
              <RequestLog />
            ) : bottomPanel === "code" ? (
              <CodePanel />
            ) : (
              <TypePlayground />
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}
