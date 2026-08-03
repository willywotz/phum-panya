# Dev image for the Next.js frontend (docker-compose.dev.yaml + `docker compose
# watch`). node_modules is baked in; watch syncs source changes for HMR, and
# rebuilds this image when the lockfile changes.
FROM node:24-alpine
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
EXPOSE 3000
CMD ["npm", "run", "dev", "--", "-H", "0.0.0.0", "-p", "3000"]
