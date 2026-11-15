#!/bin/sh
# Web (React + Vitest), Chrome extension and API client (node:test), each with an 80% line gate.
# Run through Docker: docker compose -f compose.test.yaml run --rm node
set -eu
mkdir -p /repo/coverage
npm ci --no-audit --no-fund --loglevel=error
echo "== Web"
npm run test:coverage --workspace @ai-buddy/web -- --coverage.reportsDirectory=/repo/coverage/web \
  --coverage.thresholds.lines=80
echo "== Chrome extension"
npm run coverage --workspace @ai-buddy/extension
rm -rf /repo/coverage/extension && cp -r apps/extension/coverage /repo/coverage/extension
echo "== API client"
cd packages/client
node --test --experimental-test-coverage --test-coverage-include='index.js' --test-coverage-lines=80 \
  --test-reporter=spec --test-reporter-destination=stdout \
  --test-reporter=lcov --test-reporter-destination=/repo/coverage/client.lcov.info test/*.test.js
