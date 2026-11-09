FROM node:22-alpine AS build
WORKDIR /src
COPY package*.json ./
COPY apps/web/package.json apps/web/package.json
COPY apps/extension/package.json apps/extension/package.json
COPY packages/client packages/client
RUN npm ci
COPY apps/web apps/web
RUN npm run build --workspace @ai-buddy/web
FROM nginx:1.28-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 80
