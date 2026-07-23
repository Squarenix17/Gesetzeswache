#!/usr/bin/env node
const { readStdin, runExistingHook, transformToLegacyHook, hookEnabled } = require('./adapter');
readStdin().then(raw => {
  const input = JSON.parse(raw || '{}');
  const legacyInput = transformToLegacyHook(input);
  if (hookEnabled('session:start', ['minimal', 'standard', 'strict'])) {
    runExistingHook('session-start.js', legacyInput);
  }
  process.stdout.write(raw);
}).catch(() => process.exit(0));
