FROM node:24.20.0-bookworm-slim

WORKDIR /tests
COPY tests/sdk/javascript/package.json tests/sdk/javascript/package-lock.json ./
RUN npm ci
COPY packages/sdk/javascript ./fixture-session
COPY tests/sdk/javascript/test.mjs ./
CMD ["npm", "test"]
