FROM golang:1.9
WORKDIR /go/src/github.com/heptio/contour

RUN go get github.com/golang/dep/cmd/dep
COPY Gopkg.toml Gopkg.lock ./
RUN dep ensure -v -vendor-only

COPY cmd cmd
COPY internal internal
COPY pkg pkg
RUN CGO_ENABLED=0 GOOS=linux go install -ldflags="-w -s" -v github.com/heptio/contour/cmd/contour

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
COPY --from=0 /go/bin/contour /bin/contour
