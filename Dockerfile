FROM alpine:latest

WORKDIR /app

# Copy pre-built binary from Makefile build-linux output
# Use linux-amd64 by default, can be overridden with --build-arg
ARG ARCH=amd64
COPY bin/linux/server-${ARCH} /app/server

# Expose port
EXPOSE 8080

# Run the application
ENTRYPOINT ["/app/server"]
