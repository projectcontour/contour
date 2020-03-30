FROM golang:1.13.8 AS build
WORKDIR /contour

ENV GOPROXY=https://proxy.golang.org
COPY go.mod go.sum /contour/
RUN go mod download

COPY cmd cmd
COPY internal internal
COPY apis apis
COPY Makefile Makefile

ARG BUILD_BRANCH
ARG BUILD_SHA
ARG BUILD_VERSION

RUN make install \
	    CGO_ENABLED=0 \
	    GOOS=linux \
	    BUILD_VERSION=${BUILD_VERSION} \
	    BUILD_SHA=${BUILD_SHA} \
	    BUILD_BRANCH=${BUILD_BRANCH}

FROM scratch AS final
COPY --from=build /go/bin/contour /bin/contour
