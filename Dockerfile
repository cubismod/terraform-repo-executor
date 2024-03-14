FROM quay.io/app-sre/golang:1.22.1 as builder
WORKDIR /build
COPY . .
RUN make lint build

FROM registry.access.redhat.com/ubi8/ubi:8.8 as downloader
WORKDIR /download
ENV TENV_VERSION=1.2.0

RUN curl -sfL https://github.com/tofuutils/tenv/releases/download/v${TENV_VERSION}/tenv_v${TENV_VERSION}_Linux_x86_64.tar.gz \
    -o tenv.tar.gz \
    && tar -zvxf tenv.tar.gz

FROM registry.access.redhat.com/ubi8-minimal
COPY --from=builder /build/terraform-repo-executor  /bin/terraform-repo-executor
COPY --from=downloader /download/tenv /usr/local/bin

ENV TFENV_ROOT=/usr/bin/tf

RUN microdnf update -y && \
    microdnf install -y git && \
    microdnf install -y ca-certificates && \
    microdnf clean all

RUN mkdir -p ${TFENV_ROOT} && \
    tenv tf install 1.4.5 && \
    tenv tf install 1.5.0 && \
    tenv tf install 1.6.0 && \
    tenv tf install 1.7.0

ENTRYPOINT  [ "/bin/terraform-repo-executor" ]
