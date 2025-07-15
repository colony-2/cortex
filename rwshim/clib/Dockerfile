# syntax=docker/dockerfile:1
############################
##  Stage 1 – Build shim  ##
############################
FROM ubuntu:24.04 AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY intercept.c .

# produce intercept.so
RUN gcc -shared -fPIC -o intercept.so intercept.c -ldl

############################
##  Stage 2 – Final image ##
############################
FROM ubuntu:24.04

# Create a home for the shim
WORKDIR /app
COPY --from=builder /build/intercept.so /app/intercept.so
