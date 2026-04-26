FROM golang:1.26.2 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/router ./cmd/router

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/router /router
ENTRYPOINT ["/router"]

