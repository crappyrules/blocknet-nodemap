# Build stage
FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nodemap .

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /nodemap /nodemap

EXPOSE 8080

ENTRYPOINT ["/nodemap"]
CMD ["-listen", ":8080"]
