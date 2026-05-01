# =============================================================
# TaskFlow server —— 多阶段构建
#
# 设计要点:
#   - 构建阶段 (golang:1.22-alpine):拉模块 + 静态编译纯 Go 二进制(modernc.org/sqlite
#     是纯 Go 实现的 sqlite,所以不需要 cgo,可以彻底关掉它)。
#   - 运行阶段 (gcr.io/distroless/static-debian12:nonroot):只装一个 ~2MB 的二进制 +
#     必备的 timezone / certs。distroless 没有 shell,攻击面最小。
#   - 运行用户为 nonroot (uid=65532)。/data 与 /backup 目录由 compose 挂卷提供,
#     在镜像里只是个占位 VOLUME 声明。
# =============================================================

FROM golang:1.22-alpine AS build
WORKDIR /src

# 先单独拷贝 go.mod / go.sum 让 docker 缓存模块下载层
COPY server/go.mod server/go.sum ./
RUN go mod download

# 再拷贝源码
COPY server/ ./

# 静态编译 —— CGO_ENABLED=0 因为 modernc.org/sqlite 是纯 Go 实现。
# -trimpath 让二进制去除本机路径,-ldflags '-s -w' 去掉调试信息减小体积。
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/taskflow-server ./cmd/server

# -------------------------------------------------------------

FROM gcr.io/distroless/static-debian12:nonroot

# 携带时区数据,SQLite WAL 与 RRULE 提醒需要本地时区计算。
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip
ENV TZ=UTC

# 二进制
COPY --from=build /out/taskflow-server /taskflow-server

# 默认配置文件:容器内放一份最小 config.toml,真实秘密走环境变量 / .env。
COPY deploy/docker/config.toml /etc/taskflow/config.toml

# 数据目录 —— 由宿主挂卷,镜像内只是占位
VOLUME ["/data", "/backup"]

# 服务端口
EXPOSE 8080

# 运行:用 /etc/taskflow/config.toml,.env 由 docker-compose 注入到环境变量
USER nonroot:nonroot
ENTRYPOINT ["/taskflow-server"]
CMD ["-config", "/etc/taskflow/config.toml"]
