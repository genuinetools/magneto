FROM alpine:latest
MAINTAINER Jessica Frazelle <jess@linux.com>

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN	apk add --no-cache \
	ca-certificates

COPY . /go/src/github.com/jessfraz/magneto

RUN set -x \
	&& apk add --no-cache --virtual .build-deps \
		go \
		git \
		gcc \
		libc-dev \
		libgcc \
		linux-headers \
	&& cd /go/src/github.com/jessfraz/magneto \
	&& go build -o /usr/bin/magneto . \
	&& apk del .build-deps \
	&& rm -rf /go \
	&& echo "Build complete."


ENTRYPOINT [ "magneto" ]
