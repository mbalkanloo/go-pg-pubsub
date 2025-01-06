# Pub/sub for PostgreSQL notifications

A server, written in Go, that listens for PostgreSQL notifications and publishes to subscribers over websockets.

## Installation

```bash
git clone https://github.com/mbalkanloo/go-pg-pubsub.git
cd go-pg-pubsub
go build
go install
```

## Usage

```bash
Usage of go-pg-pubsub:
  -chan string
        comma-separated list of postgresql channels (ex. foo,bar)
  -conn string
        db connection string (default "postgresql://postgres@localhost:5432")
  -port string
        listen port (default "80")
```

```bash
go-pg-pubsub -chan foo,bar -port 8000
```

## Example

## TODO
