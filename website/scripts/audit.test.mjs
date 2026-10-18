import assert from 'node:assert/strict';
import {test} from 'node:test';
import {evaluateAudit, exception, parseAuditRun} from './audit.mjs';

const now = new Date('2026-10-18T12:00:00Z');
const lock = {packages: {'node_modules/braces': {version: '3.0.3'}}};
const advisory = {name: 'braces', url: exception.url, severity: 'high'};
const vulnerability = (name, via) => ({name, via, severity: 'high',
  nodes: [`node_modules/${name}`], fixAvailable: false});
const report = vulnerabilities => ({auditReportVersion: 2, vulnerabilities,
  metadata: {vulnerabilities: {total: Object.keys(vulnerabilities).length}}});
const fixture = () => report({
  braces: vulnerability('braces', [advisory]),
  micromatch: vulnerability('micromatch', ['braces']),
  builder: vulnerability('builder', ['micromatch']),
});

test('clean audit passes even after the exception expires', () => {
  assert.deepEqual(evaluateAudit(report({}), lock, new Date('2027-01-01')),
    {accepted: [], blocked: []});
});

test('accepts only the advisory and its transitive propagation', () => {
  const result = evaluateAudit(fixture(), lock, now);
  assert.equal(result.accepted.length, 1);
  assert.deepEqual(result.blocked, []);
});

test('a second advisory in an accepted dependency still blocks', () => {
  const r = fixture();
  r.vulnerabilities.braces.via.push({...advisory, url: 'https://github.com/advisories/GHSA-new'});
  const result = evaluateAudit(r, lock, now);
  assert.equal(result.accepted.length, 1);
  assert.equal(result.blocked.length, 1);
  assert.match(result.blocked[0], /GHSA-new/);
});

test('an unrelated moderate advisory blocks', () => {
  const r = fixture();
  r.vulnerabilities.builder.via.push({name: 'builder', severity: 'moderate', url: 'https://example.com/new'});
  assert.equal(evaluateAudit(r, lock, now).blocked.length, 1);
});

test('low severity retains the existing moderate threshold', () => {
  const r = report({tool: {...vulnerability('tool', [{name: 'tool', severity: 'low', url: 'https://example.com/low'}]), severity: 'low'}});
  assert.deepEqual(evaluateAudit(r, lock, now), {accepted: [], blocked: []});
});

test('exception expires exactly at its deadline', () => {
  assert.equal(evaluateAudit(fixture(), lock, new Date(exception.expires)).blocked.length, 1);
});

test('changed package version or an available fix requires review', () => {
  assert.equal(evaluateAudit(fixture(), {packages: {'node_modules/braces': {version: '3.0.4'}}}, now).blocked.length, 1);
  const r = fixture();
  r.vulnerabilities.braces.fixAvailable = {name: 'braces', version: '3.0.4'};
  assert.equal(evaluateAudit(r, lock, now).blocked.length, 1);
});

test('missing inventory and a second unreviewed installation block', () => {
  assert.equal(evaluateAudit(fixture(), {packages: {}}, now).blocked.length, 1);
  const r = fixture();
  r.vulnerabilities.braces.nodes.push('node_modules/other/node_modules/braces');
  assert.equal(evaluateAudit(r, lock, now).blocked.length, 1);
});

test('same advisory URL on another package is not accepted', () => {
  const r = fixture();
  r.vulnerabilities.builder.via.push({...advisory, name: 'builder'});
  assert.equal(evaluateAudit(r, lock, now).blocked.length, 1);
});

test('dependency cycles with a known root are supported', () => {
  const r = fixture();
  r.vulnerabilities.micromatch.via.push('builder');
  assert.deepEqual(evaluateAudit(r, lock, now).blocked, []);
});

test('missing roots, dangling references and malformed reports fail closed', () => {
  assert.throws(() => evaluateAudit(report({tool: vulnerability('tool', ['tool'])}), lock, now), /No advisory root/);
  assert.throws(() => evaluateAudit(report({tool: vulnerability('tool', ['absent'])}), lock, now), /Missing advisory dependency/);
  assert.throws(() => evaluateAudit({}, lock, now), /report format/);
  assert.throws(() => evaluateAudit({...fixture(), metadata: {}}, lock, now), /Incomplete/);
  const r = fixture();
  r.vulnerabilities.builder.via.push({});
  assert.throws(() => evaluateAudit(r, lock, now), /Malformed advisory/);
});

test('npm errors, signals and invalid JSON cannot produce a passing audit', () => {
  for (const result of [
    {status: 2, stdout: JSON.stringify(report({}))},
    {status: 0, signal: 'SIGTERM', stdout: '{}'},
    {status: 1, error: new Error('network failure'), stdout: '{}'},
    {status: 1, stdout: JSON.stringify({error: {code: 'ENOAUDIT'}})},
    {status: 0, stdout: 'not JSON'},
  ]) assert.throws(() => parseAuditRun(result));
  for (const status of [0, 1]) {
    assert.deepEqual(parseAuditRun({status, stdout: JSON.stringify(fixture())}), fixture());
  }
});
