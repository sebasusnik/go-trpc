package main

import "fmt"

// === Go backend templates ===

func goModTemplate(opts createOptions) string {
	mod := fmt.Sprintf(`module %s/api

go 1.25.0

require (
	github.com/sebasusnik/go-trpc v0.5.0
)
`, opts.Name)

	if opts.DB {
		mod += `
require (
	github.com/jackc/pgx/v5 v5.7.0
)
`
	}

	return mod
}

func goMainTemplate(opts createOptions) string {
	imports := `	"log"

	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	"%s/api/handlers"`

	imports = fmt.Sprintf(imports, opts.Name)

	if opts.Auth {
		imports += fmt.Sprintf("\n\t\"%s/api/auth\"", opts.Name)
	}

	body := `func main() {
	r := gotrpc.NewRouter()
`

	if opts.Auth {
		body += `
	// Public routes
	gotrpc.Query(r, "health", handlers.Health)

	// Protected routes (require valid JWT)
	r.Use(auth.JWTMiddleware("your-secret-key"))
`
	}

	body += `
	// Register handlers
	handlers.RegisterUserRoutes(r)

	r.PrintRoutes("/trpc", ":3000")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":3000"})
	log.Println("Server starting on :3000")
	srv.Start()
}`

	return fmt.Sprintf(`package main

import (
%s
)

%s
`, imports, body)
}

func goUserHandlerTemplate(opts createOptions) string {
	t := `package handlers

import (
	"context"

	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// Input/output types — these become TypeScript types via gotrpc generate.

type GetUserInput struct {
	ID string ` + "`json:\"id\"`" + `
}

type CreateUserInput struct {
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

type User struct {
	ID    string ` + "`json:\"id\"`" + `
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

type HealthOutput struct {
	Status string ` + "`json:\"status\"`" + `
}

func Health(ctx context.Context, input struct{}) (HealthOutput, error) {
	return HealthOutput{Status: "ok"}, nil
}

func RegisterUserRoutes(r *gotrpc.Router) {
	gotrpc.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			// TODO: replace with real database lookup
			return User{
				ID:    input.ID,
				Name:  "Jane Doe",
				Email: "jane@example.com",
			}, nil
		},
	)

	gotrpc.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			// TODO: replace with real database insert
			return User{
				ID:    "new-id",
				Name:  input.Name,
				Email: input.Email,
			}, nil
		},
	)
}
`
	return t
}

// === Auth templates ===

func goAuthMiddlewareTemplate(opts createOptions) string {
	return `package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

type contextKey string

const UserIDKey contextKey = "userID"

// Claims represents minimal JWT claims.
type Claims struct {
	Sub string ` + "`json:\"sub\"`" + `
	Exp int64  ` + "`json:\"exp\"`" + `
}

// JWTMiddleware validates a Bearer token using HMAC-SHA256 and injects the
// user ID into context.
func JWTMiddleware(secret string) gotrpc.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := gotrpc.GetBearerToken(r.Context())
			if token == "" {
				trpcerrors.Write(w, trpcerrors.Unauthorized("missing token"))
				return
			}

			claims, err := validateToken(token, secret)
			if err != nil {
				trpcerrors.Write(w, trpcerrors.Unauthorized("invalid token"))
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.Sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts the authenticated user ID from context.
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

func validateToken(token, secret string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, trpcerrors.Unauthorized("malformed token")
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, trpcerrors.Unauthorized("invalid signature")
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, trpcerrors.Unauthorized("invalid payload")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, trpcerrors.Unauthorized("invalid claims")
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, trpcerrors.Unauthorized("token expired")
	}

	return &claims, nil
}
`
}

// === Database templates ===

func sqlcQueriesTemplate() string {
	return `-- name: GetUser :one
SELECT id, name, email, created_at
FROM users
WHERE id = $1;

-- name: ListUsers :many
SELECT id, name, email, created_at
FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateUser :one
INSERT INTO users (name, email)
VALUES ($1, $2)
RETURNING id, name, email, created_at;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
`
}

func sqlcMigrationTemplate() string {
	return `CREATE TABLE IF NOT EXISTS users (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_email ON users (email);
`
}

func sqlcConfigTemplate() string {
	return `version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema: "migrations/"
    gen:
      go:
        package: "db"
        out: "."
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_empty_slices: true
`
}

// === Frontend templates ===

func packageJSONTemplate(opts createOptions) string {
	deps := `    "@trpc/client": "^11.0.0",
    "@trpc/server": "^11.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"`

	devDeps := `    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.0",
    "autoprefixer": "^10.4.20",
    "postcss": "^8.4.49",
    "tailwindcss": "^3.4.17",
    "typescript": "^5.7.0",
    "vite": "^6.0.0"`

	return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
%s
  },
  "devDependencies": {
%s
  }
}
`, opts.Name, deps, devDeps)
}

func tsconfigTemplate() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true
  },
  "include": ["src"]
}
`
}

func viteConfigTemplate() string {
	return `import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/trpc": {
        target: "http://localhost:3000",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
`
}

func indexHTMLTemplate(opts createOptions) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, opts.Name)
}

func mainTSXTemplate() string {
	return `import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
`
}

func appTSXTemplate(opts createOptions) string {
	return fmt.Sprintf(`import { useEffect, useState } from "react";
import { trpc } from "./trpc";
import { UserCard } from "./components/UserCard";

export function App() {
  const [user, setUser] = useState<{
    id: string;
    name: string;
    email: string;
  } | null>(null);

  useEffect(() => {
    trpc.getUser.query({ id: "1" }).then(setUser);
  }, []);

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <div className="mx-auto max-w-2xl px-4 py-16">
        <h1 className="mb-2 text-4xl font-bold tracking-tight">%s</h1>
        <p className="mb-8 text-zinc-400">
          Full-stack typesafe app powered by{" "}
          <a
            href="https://github.com/sebasusnik/go-trpc"
            className="text-blue-400 hover:underline"
          >
            go-trpc
          </a>
        </p>

        {user ? (
          <UserCard user={user} />
        ) : (
          <p className="text-zinc-500">Loading...</p>
        )}
      </div>
    </div>
  );
}
`, opts.Name)
}

func trpcClientTemplate(opts createOptions) string {
	if opts.WS {
		return `import {
  createTRPCClient,
  httpLink,
  splitLink,
  wsLink,
} from "@trpc/client";
import type { AppRouter } from "./generated/router";

const url = window.location.origin + "/trpc";
const wsUrl = url.replace(/^http/, "ws");

export const trpc = createTRPCClient<AppRouter>({
  links: [
    splitLink({
      condition: (op) => op.type === "subscription",
      true: wsLink({ url: wsUrl }),
      false: httpLink({ url }),
    }),
  ],
});
`
	}

	return `import { createTRPCClient, httpLink } from "@trpc/client";
import type { AppRouter } from "./generated/router";

export const trpc = createTRPCClient<AppRouter>({
  links: [httpLink({ url: "/trpc" })],
});
`
}

func stylesTemplate() string {
	return `@tailwind base;
@tailwind components;
@tailwind utilities;

body {
  font-family:
    "Inter",
    system-ui,
    -apple-system,
    sans-serif;
}
`
}

func userCardTemplate() string {
	return `interface UserCardProps {
  user: { id: string; name: string; email: string };
}

export function UserCard({ user }: UserCardProps) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-6">
      <div className="mb-4 flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-blue-600 text-sm font-bold">
          {user.name.charAt(0)}
        </div>
        <div>
          <h2 className="font-semibold">{user.name}</h2>
          <p className="text-sm text-zinc-400">{user.email}</p>
        </div>
      </div>
      <p className="text-xs text-zinc-500">ID: {user.id}</p>
    </div>
  );
}
`
}

func postcssConfigTemplate() string {
	return `export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
`
}

func tailwindConfigTemplate() string {
	return `/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {},
  },
  plugins: [],
};
`
}

// === DevOps templates ===

func makefileTemplate(opts createOptions) string {
	dbTargets := ""
	if opts.DB {
		dbTargets = `
.PHONY: db-migrate
db-migrate: ## Run database migrations
	@psql "$(DATABASE_URL)" -f api/db/migrations/001_init.sql

.PHONY: db-generate
db-generate: ## Generate sqlc Go code from SQL queries
	@cd api/db && sqlc generate

`
	}

	return fmt.Sprintf(`.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%%-15s\033[0m %%s\n", $$1, $$2}'

.PHONY: setup
setup: ## Install all dependencies
	@echo "Installing Go dependencies..."
	@cd api && go mod tidy
	@echo "Installing npm dependencies..."
	@cd web && npm install
	@echo "Done! Run 'make dev' to start developing."

.PHONY: dev
dev: ## Start backend, frontend, and codegen watcher
	@echo "Starting %s..."
	@trap 'kill 0' EXIT; \
		(cd api && go run .) & \
		(cd web && npm run dev) & \
		(gotrpc generate --watch) & \
		wait

.PHONY: dev-api
dev-api: ## Start only the Go backend
	@cd api && go run .

.PHONY: dev-web
dev-web: ## Start only the Vite frontend
	@cd web && npm run dev

.PHONY: generate
generate: ## Generate TypeScript types from Go handlers
	@gotrpc generate

.PHONY: build
build: generate ## Build for production
	@cd api && go build -o ../dist/server .
	@cd web && npm run build

.PHONY: lint
lint: ## Lint Go and TypeScript code
	@cd api && go vet ./...
	@cd web && npx tsc --noEmit
%s
.DEFAULT_GOAL := help
`, opts.Name, dbTargets)
}

func dockerfileTemplate(opts createOptions) string {
	return fmt.Sprintf(`# --- Build frontend ---
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

# --- Build backend ---
FROM golang:1.25-alpine AS api
WORKDIR /app
COPY api/go.mod api/go.sum* ./api/
RUN cd api && go mod download
COPY api/ ./api/
RUN cd api && CGO_ENABLED=0 go build -o /server .

# --- Final image ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=api /server .
COPY --from=web /app/web/dist ./web/dist
ENV PORT=3000
EXPOSE 3000
CMD ["./server"]
`)
}

func rootGitignoreTemplate() string {
	return `# Go
api/tmp/
dist/

# Node
web/node_modules/
web/dist/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Env
.env
.env.local
`
}

func dockerComposeTemplate(opts createOptions) string {
	services := fmt.Sprintf(`services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - PORT=3000`)

	if opts.DB {
		services += `
      - DATABASE_URL=postgres://postgres:postgres@db:5432/app?sslmode=disable
    depends_on:
      db:
        condition: service_healthy

  db:
    image: postgres:17-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: app
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./api/db/migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pgdata:`
	}

	return services + "\n"
}
