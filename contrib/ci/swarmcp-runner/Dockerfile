FROM golang:1.26.0-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=ci-dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags "-s -w -X github.com/cmmoran/swarmcp/cmd.Version=${VERSION}" \
  -o /out/swarmcp .

FROM alpine:3.23

RUN apk add --no-cache \
  bash \
  ca-certificates \
  curl \
  docker-cli \
  git \
  jq \
  openssh-client \
  tzdata \
  yq

COPY --from=build /out/swarmcp /usr/local/bin/swarmcp

RUN swarmcp version >/dev/null
RUN git --version >/dev/null \
  && docker --version >/dev/null \
  && jq --version >/dev/null \
  && yq --version >/dev/null \
  && command -v ssh-keyscan >/dev/null

WORKDIR /workspace

CMD ["sh"]
