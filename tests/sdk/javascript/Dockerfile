FROM node:24.20.0-bookworm-slim

WORKDIR /tests
COPY package.json package-lock.json ./
RUN npm ci
COPY test.mjs ./
CMD ["npm", "test"]
