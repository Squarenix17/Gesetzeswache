const { createInstallTargetAdapter } = require('./helpers');

module.exports = createInstallTargetAdapter({
  id: 'kimi-project',
  target: 'kimi',
  kind: 'project',
  rootSegments: ['.kimi'],
  installStatePathSegments: ['install-state.json'],
  nativeRootRelativePath: '.kimi',
});
