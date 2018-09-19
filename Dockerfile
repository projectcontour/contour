FROM golang:1.10.4 AS build
WORKDIR /go/src/github.com/heptio/contour

RUN go get github.com/golang/dep/cmd/dep
COPY Gopkg.toml Gopkg.lock ./
RUN dep ensure -v -vendor-only

COPY cmd cmd
COPY internal internal
COPY apis apis
RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/contour -ldflags="-w -s" -v github.com/heptio/contour/cmd/contour

FROM alpine:3.8 AS final
RUN apk --no-cache add ca-certificates
COPY --from=build /go/bin/contour /bin/contour
