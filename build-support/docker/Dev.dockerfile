FROM golang:1.13.1 as builder
ARG GIT_COMMIT
ARG GIT_DIRTY
ARG GIT_DESCRIBE
ENV CONSUL_DEV=1
ENV COLORIZE=0
ADD . /opt/build
WORKDIR /opt/build
RUN make

FROM consul:latest

COPY --from=builder /opt/build/bin/eureka-aws /bin
