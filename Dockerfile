ARG BUILDPLATFORM=linux/amd64

FROM --platform=$BUILDPLATFORM golang:1.15.8 AS build
WORKDIR /contour

ENV GOPROXY=https://proxy.golang.org
COPY go.mod go.sum /contour/
RUN go mod download

COPY cmd cmd
COPY internal internal
COPY pkg pkg
COPY apis apis
COPY Makefile Makefile

ARG BUILD_BRANCH
ARG BUILD_SHA
ARG BUILD_VERSION
ARG TARGETOS
ARG TARGETARCH

RUN make build \
	    CGO_ENABLED=0 \
		GOOS=${TARGETOS} \
		GOARCH=${TARGETARCH} \
	    BUILD_VERSION=${BUILD_VERSION} \
	    BUILD_SHA=${BUILD_SHA} \
	    BUILD_BRANCH=${BUILD_BRANCH}

FROM scratch AS final
COPY --from=build /contour/contour /bin/contour
