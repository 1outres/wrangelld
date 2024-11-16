FROM golang:1.22.8 AS builder

RUN apt update && apt install -y llvm clang libbpf-dev
RUN ln -s /usr/include/x86_64-linux-gnu/asm /usr/include/asm

WORKDIR /app

COPY . .

RUN go mod download
RUN go generate gen/gen.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /wrangelld cmd/wrangelld/main.go

FROM gcr.io/distroless/base-debian11 AS runner

WORKDIR /

COPY --from=builder /wrangelld /wrangelld

USER nonroot:nonroot

ENTRYPOINT ["/wrangelld"]
