FROM quay.io/app-sre/golang:1.22.1 as builder
WORKDIR /build
COPY . .
RUN make lint build

FROM registry.access.redhat.com/ubi8-minimal
COPY --from=builder /build/terraform-repo-executor  /bin/terraform-repo-executor

RUN microdnf update -y && \
    microdnf install -y git && \
    microdnf install -y ca-certificates && \
    microdnf install -y unzip && \
    microdnf install -y wget && \
    microdnf clean all

RUN wget -q https://github.com/tofuutils/tenv/releases/download/v1.0.5/tenv_1.0.5_linux_amd64.zip \
    && unzip tenv_1.0.5_linux_amd64.zip -d tenv \
    && mv tenv/tenv /usr/local/bin/ \
    && rm tenv_1.0.5_linux_amd64.zip \
    && rm -r tenv/

# instruct tenv to install terraform binaries into /bin/tf
ENV TFENV_ROOT=/bin/tf

RUN mkdir -p /bin/tf && \
    tenv tf install 1.4.5 && \
    tenv tf install 1.5.0 && \
    tenv tf install 1.6.0 && \
    tenv tf install 1.7.0

ENTRYPOINT  [ "/bin/terraform-repo-executor" ]
