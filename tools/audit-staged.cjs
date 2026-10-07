'use strict';

const {execFileSync} = require('node:child_process');

const git = (...args) => execFileSync('git', args, {maxBuffer: 32 * 1024 * 1024});
const entries = git('ls-files', '--stage', '-z').toString('utf8').split('\0').filter(Boolean);
const issues = [];
const rules = [
  ['private-key', /-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----/g],
  ['github-token', /\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{30,})\b/g],
  ['api-secret', /\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,}\b/g],
  ['aws-access-key', /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/g],
  ['jwt', /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g],
  ['credential-url', /https?:\/\/[^/\s:@]+:[^/\s@]+@/g],
  ['personal-windows-path', /\b[A-Za-z]:[\\/]+Users[\\/]+[^\\/\s]+/gi],
  ['personal-linux-path', /\/(?:home\/[A-Za-z0-9_.-]+|root)(?:\/|\b)/g],
  ['configured-secret', /(?:["']?(?:api[_-]?key|management[_-]?key|password|secret[_-]?key|access[_-]?token|refresh[_-]?token)["']?)\s*[:=]\s*["'][A-Za-z0-9_+./=-]{20,}["']/gi],
];
const forbidden = /(?:^|\/)(?:data|auth|plugins|node_modules|dist|output|screenshots|\.playwright-cli|release-[^/]+)(?:\/|$)|\.(?:sqlite(?:3)?(?:-[^/]*)?|db(?:-[^/]*)?|pem|key|p12|pfx|so|dll|dylib|exe|zip|png|jpe?g|webp|log)$|(?:^|\/)(?:\.env[^/]*|config\.ya?ml|auth\.json|isolated-auth\.json|deploy[^/]*|[^/]*(?:deployment|production)-report[^/]*)$/i;
const add = (path, line, rule) => issues.push({path, line, rule});

for (const entry of entries) {
  const match = entry.match(/^(\d+) [0-9a-f]+ (\d+)\t([\s\S]+)$/);
  if (!match) throw new Error('Unexpected index entry');
  const [, mode, stage, path] = match;
  if (stage !== '0' || !['100644', '100755'].includes(mode)) {
    add(path, 1, 'non-regular-or-unmerged-file');
    continue;
  }
  if (forbidden.test(path)) add(path, 1, 'forbidden-runtime-file');
  const raw = git('show', ':' + path);
  if (raw.includes(0)) {
    add(path, 1, 'binary-file');
    continue;
  }
  const text = raw.toString('utf8');
  const lineAt = offset => text.slice(0, offset).split('\n').length;
  for (const [name, pattern] of rules) {
    pattern.lastIndex = 0;
    for (const found of text.matchAll(pattern)) add(path, lineAt(found.index), name);
  }
  const ipv4 = /(?<![\d.])\b(?:\d{1,3}\.){3}\d{1,3}\b(?!\d|\.\d)/g;
  for (const found of text.matchAll(ipv4)) {
    const octets = found[0].split('.').map(Number);
    if (octets.every(value => value <= 255) && found[0] !== '127.0.0.1') {
      add(path, lineAt(found.index), 'non-loopback-ipv4');
    }
  }
}

if (entries.length === 0) throw new Error('No staged files to audit');
if (issues.length) {
  for (const issue of issues) console.error(`${issue.path}:${issue.line}: ${issue.rule}`);
  console.error(`Disclosure guard failed: ${issues.length} finding(s). Matched values are intentionally hidden.`);
  process.exitCode = 1;
} else {
  console.log(`Disclosure guard passed: ${entries.length} staged text files inspected.`);
}
