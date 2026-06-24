# Build
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

# Run (small, runs anywhere: App Runner, ECS, a plain box)
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
