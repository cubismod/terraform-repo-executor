FROM quay.io/app-sre/golang:1.20.1 as builder
WORKDIR /build
COPY . .
RUN make build

FROM registry.access.redhat.com/ubi8-minimal
COPY --from=builder /build/terraform-repo-executor  /bin/terraform-repo-executor

RUN microdnf update -y && \
    microdnf install -y git && \
    microdnf install -y ca-certificates && \
    microdnf install -y unzip && \
    microdnf install -y wget && \
    microdnf clean all

RUN wget -q https://releases.hashicorp.com/terraform/1.4.5/terraform_1.4.5_linux_amd64.zip \
    && unzip terraform_1.4.5_linux_amd64.zip \
    && mv terraform /usr/local/bin/ \
    && rm terraform_1.4.5_linux_amd64.zip

ENTRYPOINT  [ "/bin/terraform-repo-executor" ]