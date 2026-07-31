# Build a static garmin binary and ship it in a minimal image.
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/garmin ./cmd/garmin

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/garmin /usr/local/bin/garmin
# The MCP server reads tokens from $GARMINTOKENS (mount the file at runtime).
ENV GARMINTOKENS=/tokens/garmin_tokens.json
ENTRYPOINT ["/usr/local/bin/garmin"]
CMD ["mcp"]
