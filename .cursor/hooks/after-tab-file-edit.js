#!/usr/bin/env node
const { readStdin, runExistingHook, transformToLegacyHook } = require('./adapter');
readStdin().then(raw => {
  try {
    const input = JSON.parse(raw);
    const legacyInput = transformToLegacyHook(input, {
      tool_input: { file_path: input.path || input.file || '' }
    });
    runExistingHook('post-edit-format.js', JSON.stringify(legacyInput));
  } catch {}
  process.stdout.write(raw);
}).catch(() => process.exit(0));
