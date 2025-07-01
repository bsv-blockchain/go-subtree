# Example Dockerfile for a Go application (this is just a placeholder)
FROM scratch
COPY go-subtree /
ENTRYPOINT ["/go-subtree"]
