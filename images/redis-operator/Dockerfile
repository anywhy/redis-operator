FROM redis:5.0.7-alpine AS base
FROM alpine:3.10

# copy redis-cli to image
COPY --from=base /usr/local/bin/redis-cli /usr/local/bin/redis-cli

RUN apk add tzdata --no-cache
ADD bin/redis-controller-manager /usr/local/bin/redis-controller-manager
