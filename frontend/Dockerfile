FROM node:22.21.0-alpine AS dependencies
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

FROM dependencies AS development
COPY . .
EXPOSE 80
CMD ["npm", "run", "dev", "--", "--port", "80", "--strictPort"]

FROM dependencies AS builder
COPY . .
ENV VITE_API_URL=/api
ENV VITE_ENV=production
RUN npm run build

FROM nginx:1.28-alpine AS runtime
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
