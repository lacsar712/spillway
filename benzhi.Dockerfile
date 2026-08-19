# 官方 Go 镜像，保留完整工具链（评测用）
FROM golang:1.22

# 保证非 login shell 也能直接调用 go
ENV PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0

WORKDIR /app

# 本项目无外部依赖：无 go.sum，跳过 go mod download
COPY go.mod ./
COPY . .

# 预编译一次，把编译缓存留在镜像里
RUN go build ./...

CMD ["bash"]
