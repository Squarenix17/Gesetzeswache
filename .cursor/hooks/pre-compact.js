#!/usr/bin/env node
const { readStdin, runExistingHook, transformToLegacyHook } = require('./adapter');
readStdin().then(raw => {
  const cursorInput = JSON.parse(raw || '{}');
  runExistingHook('pre-compact.js', transformToLegacyHook(cursorInput));
  process.stdout.write(raw);
}).catch(() => process.exit(0));
