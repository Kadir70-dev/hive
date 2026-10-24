# The Hive Society

Next.js 16 frontend + Go/Chi API + self-hosted PostgreSQL. Supabase is not required or used by the updated active application.

- `src/`: Next.js frontend, server-side same-origin backend proxy
- `backend/`: Go API and SQL migration
- `docker-compose.yml`: Hetzner VPS services (Go/Postgres/Redis/Caddy)
- `IMPLEMENTATION_STATUS.md`: honest feature status, limits and launch checklist

This is a **development checkpoint**, not a validated production-ready build. Database schema and API need integration and security testing. Configure `HIVE_API_INTERNAL_URL` on Netlify (server-only) and run migration before using the application.
