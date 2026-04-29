# Dispatch backend

Go (Gin) + MongoDB API for **team sync**.

## Requirements

- Go 1.22+
- MongoDB URI

## Configure

Set environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `DISPATCH_MONGO_URI` | Yes | MongoDB connection string |
| `DISPATCH_DB` | Yes | Database name, e.g. `dispatch` |
| `DISPATCH_JWT_SECRET` | Yes | Secret for JWT signing |
| `DISPATCH_HTTP_ADDR` | No | Default `:8080`; `PORT` overrides on platforms like Railway |

## Run locally

```bash
go run ./cmd/server
```

## Railway

See [RAILWAY.md](RAILWAY.md) for Docker deploy and env vars. After split, set repository **Root Directory** to **`.`** (repo root), not `backend`.
