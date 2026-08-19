FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/powerlevel ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends chromium ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/powerlevel /usr/local/bin/powerlevel
ENV APP_ADDRESS=:18781 \
    BROWSER_PATH=/usr/bin/chromium \
    BROWSER_HEADLESS=true
EXPOSE 18781
USER nobody
ENTRYPOINT ["powerlevel"]
