# --- build ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY bible.txt ./
# bible.txt is //go:embed-ed. `deploy.sh` swaps the full (private) book in for
# the deploy; the committed bible.txt is a short public-safe placeholder.
# Static binary, no cgo.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /coca-oracle-ssh .

# --- run ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
WORKDIR /home/app
COPY --from=build /coca-oracle-ssh /usr/local/bin/coca-oracle-ssh
ENV PORT=23234 HOST=0.0.0.0
EXPOSE 23234
ENTRYPOINT ["coca-oracle-ssh"]
