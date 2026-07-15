# ---- frontend ----
FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- backend ----
FROM golang:1.26-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
COPY backend/third_party/ ./third_party/
RUN go mod download
COPY backend/ ./
# swap the placeholder dist for the real build, then embed
RUN rm -rf internal/webdist/dist
COPY --from=frontend /src/frontend/dist/ internal/webdist/dist/
ARG VERSION=0.9.1
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/palhelm ./cmd/palhelm

# ---- runtime ----
FROM alpine:3.22
# gcompat + libstdc++: the runtime-downloaded Oodle decompressor is a glibc
# binary; gcompat lets musl-based Alpine dlopen it.
RUN apk add --no-cache ca-certificates tzdata gcompat libstdc++ \
    && addgroup -S palhelm && adduser -S -G palhelm palhelm
COPY --from=backend /out/palhelm /usr/local/bin/palhelm
USER palhelm
ENV PALHELM_ADDR=:8080 PALHELM_DATA_DIR=/data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1
ENTRYPOINT ["palhelm"]
CMD ["serve"]
