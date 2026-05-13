# ─── Stage 1: Build React/Vite app ───────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

# Cache npm install layer separately from source changes
COPY frontend/package*.json ./
RUN npm install --legacy-peer-deps

COPY frontend/ .
RUN npm run build

# ─── Stage 2: Serve with nginx ───────────────────────────────────────────────
FROM nginx:1.27-alpine

# The nginx config expects static files at /app/frontend/dist and proxies
# /api/, /ws/, /wa/webhook/ etc. to the go-audio upstream.
COPY --from=builder /app/dist /app/frontend/dist

# Use the project's Docker-specific nginx virtual-host config
COPY nginx/callified-docker.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost/health || exit 1
