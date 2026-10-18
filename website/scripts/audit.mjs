import {spawnSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import {fileURLToPath, pathToFileURL} from 'node:url';

// Accepted build-tool risk, not a patched dependency. See ../../SECURITY.md.
export const exception = Object.freeze({
  url: 'https://github.com/advisories/GHSA-vfj7-8cjw-p6xm',
  name: 'braces',
  version: '3.0.3',
  expires: '2026-11-18T00:00:00Z',
});

const levels = ['info', 'low', 'moderate', 'high', 'critical'];
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const requireValid = (condition, message) => {
  if (!condition) throw new Error(message);
};

export function parseAuditRun(result) {
  requireValid(!result.error && !result.signal && [0, 1].includes(result.status),
    `npm audit failed: ${result.error?.message || result.signal || result.stderr || result.status}`);
  const report = JSON.parse(result.stdout);
  requireValid(!report.error, `npm audit error: ${JSON.stringify(report.error)}`);
  return report;
}

export function evaluateAudit(report, lock, now = new Date()) {
  requireValid(Number.isFinite(now.getTime()), 'Invalid audit evaluation date');
  requireValid(report.auditReportVersion === 2 && object(report.vulnerabilities),
    'Unexpected npm audit report format');
  requireValid(object(lock.packages), 'Missing lockfile package inventory');
  const vulnerabilities = report.vulnerabilities;
  requireValid(report.metadata?.vulnerabilities?.total === Object.keys(vulnerabilities).length,
    'Incomplete npm audit report');

  for (const [name, vulnerability] of Object.entries(vulnerabilities)) {
    requireValid(vulnerability.name === name && levels.includes(vulnerability.severity)
      && Array.isArray(vulnerability.via) && vulnerability.via.length > 0,
    `Malformed vulnerability: ${name}`);
    for (const via of vulnerability.via) {
      if (typeof via === 'string') {
        requireValid(Object.hasOwn(vulnerabilities, via), `Missing advisory dependency: ${via}`);
      } else {
        requireValid(object(via) && typeof via.url === 'string' && typeof via.name === 'string'
          && levels.includes(via.severity), `Malformed advisory: ${name}`);
      }
    }
  }

  // Evaluate advisory roots, not just package names: a package can inherit the
  // accepted advisory AND an unrelated advisory, which must still fail CI.
  const accepted = new Set();
  const blocked = new Set();
  for (const [name, vulnerability] of Object.entries(vulnerabilities)) {
    const visited = new Set();
    const roots = [];
    const visit = key => {
      if (visited.has(key)) return;
      visited.add(key);
      for (const via of vulnerabilities[key].via) {
        if (typeof via === 'string') visit(via);
        else roots.push(via);
      }
    };
    visit(name);
    requireValid(roots.length > 0, `No advisory root found for ${name}`);
    for (const advisory of roots) {
      if (levels.indexOf(vulnerability.severity) < 2 && levels.indexOf(advisory.severity) < 2) continue;
      const source = vulnerabilities[exception.name];
      const pinned = Array.isArray(source?.nodes) && source.nodes.length > 0
        && source.nodes.every(node => lock.packages[node]?.version === exception.version);
      const allowed = advisory.url === exception.url && advisory.name === exception.name
        && pinned && source.fixAvailable === false
        && now.getTime() < Date.parse(exception.expires);
      (allowed ? accepted : blocked).add(`${advisory.name}: ${advisory.url}`);
    }
  }
  return {accepted: [...accepted].sort(), blocked: [...blocked].sort()};
}

function main() {
  const cwd = fileURLToPath(new URL('../', import.meta.url));
  const result = spawnSync(process.platform === 'win32' ? 'npm.cmd' : 'npm',
    ['audit', '--json', '--audit-level=moderate'],
    {cwd, encoding: 'utf8', timeout: 120_000, maxBuffer: 16 * 1024 * 1024});
  const report = parseAuditRun(result);
  const lock = JSON.parse(readFileSync(new URL('../package-lock.json', import.meta.url), 'utf8'));
  const {accepted, blocked} = evaluateAudit(report, lock);
  for (const advisory of accepted) {
    console.warn(`ACCEPTED until ${exception.expires}: ${advisory} (documentation build only; see SECURITY.md)`);
  }
  for (const advisory of blocked) console.error(`BLOCKED: ${advisory}`);
  if (blocked.length) process.exitCode = 1;
  else console.log(`Dependency audit passed (${accepted.length} explicitly accepted advisory).`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main();
  } catch (error) {
    console.error(`Dependency audit failed: ${error.message}`);
    process.exitCode = 1;
  }
}
