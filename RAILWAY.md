# Deploy Dispatch backend on Railway

This service builds from [`Dockerfile`](Dockerfile) at the **repository root**. Use **Root Directory** `.` (or leave default) when importing this repo into Railway.

## 1. Add MongoDB

In the Railway project dashboard:

1. Click **New** → **Database** → **MongoDB**, or **Templates** → MongoDB.
2. After provisioning, open the MongoDB service → **Variables** and copy `MONGO_URL` (or `DATABASE_URL`, depending on template).

Alternatively use **MongoDB Atlas** (free tier): create a cluster, obtain the connection string, and paste it into `DISPATCH_MONGO_URI` below.

## 2. Configure environment variables

On your **Dispatch backend** web service (not the MongoDB plugin), add:

| Variable | Required | Description |
|----------|----------|-------------|
| `DISPATCH_MONGO_URI` | Yes | Connection string from Railway MongoDB (`MONGO_URL`) or Atlas |
| `DISPATCH_DB` | Yes | Database name, e.g. `dispatch` |
| `DISPATCH_JWT_SECRET` | Yes | Strong secret (e.g. `openssl rand -hex 32`) |
| `PORT` | No | Railway injects automatically; the server binds to `PORT` when present |

Optional:

| Variable | Default | Description |
|----------|---------|-------------|
| `DISPATCH_HTTP_ADDR` | `:8080` | If left at default, `PORT` from Railway overrides the listen address |

## 3. Deploy and get public URL

1. Connect this GitHub repo (or deploy with **Empty project** → **Dockerfile** at repo root).
2. Railway builds the image and assigns a domain under **Settings** → **Networking** → **Generate domain**.
3. Your API base URL is `https://<your-service>.up.railway.app` (no trailing slash).

Health check: `GET https://<your-service>.up.railway.app/api/teams` should return **401** without auth (proves Gin is up).

## 4. Frontend / desktop app

Configure the **dispatch-frontend** (or nested `frontend/` in **dispatch-tauri**) `.env.production` with `VITE_API_URL` set to your Railway HTTPS URL, then build the web app (`npm run build`) or the desktop bundle via the dispatch-tauri repo (`npm run tauri build`).
