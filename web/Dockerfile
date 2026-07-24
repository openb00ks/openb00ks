# syntax=docker/dockerfile:1

FROM node:20-alpine AS build
WORKDIR /app

# Same-origin default: the SPA calls /api/*, which this image's nginx reverse-proxies to the API
# (see web/nginx.conf). The published GHCR image inherits this; CI smoke builds and local compose
# override it with a concrete host (e.g. http://localhost:8181).
ARG PUBLIC_API_BASE_URL=/api
ENV PUBLIC_API_BASE_URL=${PUBLIC_API_BASE_URL}

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web ./
RUN npm run build

FROM nginx:1.27-alpine

COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build /usr/share/nginx/html

EXPOSE 80
