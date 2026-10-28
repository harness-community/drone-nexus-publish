FROM alpine:3.20.10
RUN apk add -U --no-cache 'ca-certificates>=20261020-r0' 'ca-certificates-bundle>=20261020-r0'

ENV GODEBUG netdns=go

ADD release/linux/amd64/plugin /bin/
ENTRYPOINT ["/bin/plugin"]