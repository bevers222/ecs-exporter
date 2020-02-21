FROM golang:1.13.3 as builder
RUN echo "nobody:rx:65534:65534:Nobody:/:" > /etc_passwd
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 go build -installsuffix 'static' -o /app ./...
FROM scratch as final 
COPY --from=builder /etc_passwd /etc/passwd
COPY --from=builder /app /bin/ecs-exporter
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
EXPOSE 9677
USER nobody
ENTRYPOINT ["/bin/ecs-exporter"]
