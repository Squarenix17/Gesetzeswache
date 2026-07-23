#!/usr/bin/env node
const { readStdin, runExistingHook, transformToLegacyHook, hookEnabled } = require('./adapter');
readStdin().then(raw => {
  const input = JSON.parse(raw || '{}');
  const legacyInput = transformToLegacyHook(input);
  if (hookEnabled('session:end:marker', ['minimal', 'standard', 'strict'])) {
    runExistingHook('session-end-marker.js', legacyInput);
  }
  process.stdout.write(raw);
}).catch(() => process.exit(0));
