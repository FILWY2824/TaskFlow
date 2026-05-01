# =============================================================
# TaskFlow web —— 多阶段:Vite 构建 + nginx 静态托管 + /api 反代
#
# 设计要点:
#   - 构建阶段 (node:20-alpine):npm ci + vite build,产出 dist/。
#   - 运行阶段 (nginx:1.27-alpine):把 dist/ 拷到 /usr/share/nginx/html,
#     用 deploy/docker/nginx.conf 反代 /api → backend:8080,/ws → backend:8080。
#   - 镜像内不内置任何用户密钥,所有运行参数(后端 upstream、最大上传体积等)由
#     compose 的环境变量在启动脚本里替换。
# =============================================================

FROM node:20-alpine AS build
WORKDIR /src

# 先 lockfile 让缓存复用
COPY web/package.json web/package-lock.json ./
RUN npm ci

# 再拷源码并构建
COPY web/ ./
RUN npm run build

# -------------------------------------------------------------

FROM nginx:1.27-alpine

# 静态资源
COPY --from=build /src/dist /usr/share/nginx/html

# nginx 配置(反代 /api 与 /ws 到后端容器)
COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf

# 健康检查端点
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1

EXPOSE 80
