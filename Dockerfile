FROM golang:1.12.1 AS build
WORKDIR /go/src/github.com/heptio/contour

ENV GO111MODULE on
ENV GOFLAGS -mod=vendor
COPY go.mod go.sum ./

COPY cmd cmd
COPY internal internal
COPY apis apis
RUN go mod vendor
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-ldflags=-w go build -o /go/bin/contour -ldflags=-s -v github.com/heptio/contour/cmd/contour

FROM scratch AS final
COPY --from=build /go/bin/contour /bin/contour
