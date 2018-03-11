FROM golang:alpine as builder
MAINTAINER Jessica Frazelle <jess@linux.com>

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN	apk add --no-cache \
	ca-certificates

COPY . /go/src/github.com/genuinetools/magneto

RUN set -x \
	&& apk add --no-cache --virtual .build-deps \
		git \
		gcc \
		libc-dev \
		libgcc \
		make \
	&& cd /go/src/github.com/genuinetools/magneto \
	&& make static \
	&& mv magneto /usr/bin/magneto \
	&& apk del .build-deps \
	&& rm -rf /go \
	&& echo "Build complete."

FROM scratch

COPY --from=builder /usr/bin/magneto /usr/bin/magneto
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

ENTRYPOINT [ "magneto" ]
CMD [ "--help" ]
