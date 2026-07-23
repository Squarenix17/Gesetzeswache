#!/usr/bin/env node
const { hookEnabled, readStdin, runExistingHook, transformToLegacyHook } = require('./adapter');
readStdin().then(raw => {
  try {
    const input = JSON.parse(raw);
    const legacyInput = transformToLegacyHook(input, {
      tool_input: { file_path: input.path || input.file || '' }
    });
    const legacyStr = JSON.stringify(legacyInput);

    // Accumulate edited paths for batch format+typecheck at stop time
    runExistingHook('post-edit-accumulator.js', legacyStr);
    runExistingHook('post-edit-console-warn.js', legacyStr);
    if (hookEnabled('post:edit:design-quality-check', ['standard', 'strict'])) {
      runExistingHook('design-quality-check.js', legacyStr);
    }
  } catch {}
  process.stdout.write(raw);
}).catch(() => process.exit(0));
