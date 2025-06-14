FROM alpine:latest

COPY ./loopback /loopback

RUN mkdir /tmp/mp && mkdir /root/backend && touch /root/backend/test

CMD ["/loopback","--mountpoint=/tmp/mp","--backend=/root/backend"]
