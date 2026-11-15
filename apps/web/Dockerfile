FROM node:22-alpine AS build
WORKDIR /src
COPY package*.json ./
COPY apps/web/package.json apps/web/package.json
COPY apps/extension/package.json apps/extension/package.json
COPY packages/client packages/client
RUN npm ci
COPY apps/web apps/web
COPY apps/extension apps/extension
COPY data/kb data/kb
RUN npm run build --workspace @ai-buddy/web
# Ready-to-install Chrome extension, served at /downloads/ai-buddy-extension.zip (see public/install-extension.html).
# The site proxies /api to the backend, so one origin serves both the site and the API.
ARG EXTENSION_ORIGIN=http://127.0.0.1:5173
RUN apk add --no-cache zip  && SITE_ORIGIN=$EXTENSION_ORIGIN BACKEND_ORIGIN=$EXTENSION_ORIGIN node apps/extension/build.mjs  && mkdir -p apps/web/dist/downloads  && cd apps/extension/dist && zip -qr ../../web/dist/downloads/ai-buddy-extension.zip .
FROM nginx:1.28-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 80
