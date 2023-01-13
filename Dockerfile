ARG BUILD_BASE_IMAGE

# The build image uses the os/arch of the host (i.e. $BUILDPLATFORM),
# and golang handles the cross-compilation to $TARGETOS/$TARGETARCH.
FROM --platform=$BUILDPLATFORM $BUILD_BASE_IMAGE AS build
WORKDIR /contour

ARG BUILD_GOPRIVATE
ARG BUILD_GOPROXY
ARG BUILD_GOSUMDB

ENV GOPRIVATE=${BUILD_GOPRIVATE}
ENV GOPROXY=${BUILD_GOPROXY}
ENV GOSUMDB=${BUILD_GOSUMDB}

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd
COPY internal internal
COPY pkg pkg
COPY apis apis
COPY Makefile Makefile

ARG BUILD_BRANCH
ARG BUILD_SHA
ARG BUILD_VERSION
ARG BUILD_CGO_ENABLED
ARG BUILD_EXTRA_GO_LDFLAGS
ARG BUILD_GOEXPERIMENT
ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod make build \
	    CGO_ENABLED=${BUILD_CGO_ENABLED} \
		EXTRA_GO_LDFLAGS="${BUILD_EXTRA_GO_LDFLAGS}" \
		GOOS=${TARGETOS} \
		GOARCH=${TARGETARCH} \
		GOEXPERIMENT=${BUILD_GOEXPERIMENT} \
	    BUILD_VERSION=${BUILD_VERSION} \
	    BUILD_SHA=${BUILD_SHA} \
	    BUILD_BRANCH=${BUILD_BRANCH}

# Ensure we produced a static binary.
RUN ldd contour 2>&1 | grep 'not a dynamic executable'

FROM scratch AS final
COPY --from=build /contour/contour /bin/contour
