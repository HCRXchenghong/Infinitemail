FROM node:22-alpine AS build

WORKDIR /src
COPY package.json package-lock.json ./
COPY packages ./packages
RUN npm ci
COPY index.html vite.config.mjs ./
COPY src ./src
RUN npm run build:user

FROM nginx:1.27-alpine

COPY deploy/nginx-user.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 1788
