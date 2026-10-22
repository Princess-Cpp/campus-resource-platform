#!/usr/bin/env python3
"""Local delivery check. No provisioning, pushes, migrations or deployment here."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
DATABASES = {'M1_TEST_DSN': 'campus_m1_check', 'M2_TEST_DSN': 'campus_m2_check',
             'IDENTITY_TEST_DSN': 'campus_identity_check', 'M3_TEST_DSN': 'campus_m3_check'}
PACKAGES = ('booking', 'classroom', 'identity', 'maintenance', 'maintenance/domain', 'notification')
REQUIRED = {'TestRepairHTTPFullLifecycleAndIndependentAcceptance',
            'TestRepairReleasePreservesOtherM3AndM2Sources',
            'TestRepairLostStartReplyRestartsWithSameOperation',
            'TestEvaluationOnlyReporterAndSameKeyReplays',
            'TestEscalationBoundaryConcurrentAndRestart',
            'TestNotificationRealSMTPPipeline', 'TestNotificationInboxOwnerReadIsolation',
            'TestNotificationResourceIntentDoesNotClaimSuccess'}


def preflight(env):
    errors = []
    for name, database in DATABASES.items():
        value = env.get(name, '')
        if not value:
            errors.append(name + ' is required (isolated test database only)')
        elif not re.fullmatch(r'[^?]*@(?:tcp|unix)\([^)]*\)/' + re.escape(database) + r'(?:\?.*)?$', value):
            errors.append(name + ' must target ' + database)
    for command in ('go', 'node', 'npm', 'bash'):
        import shutil
        if not shutil.which(command):
            errors.append(command + ' is unavailable')
    return errors


def summarize(events, exit_code):
    passed = [e for e in events if e.get('Action') == 'pass' and 'Test' in e]
    top = [e for e in passed if '/' not in e['Test']]
    missing = sorted(REQUIRED - {e['Test'] for e in top})
    packages = {p: sum(e.get('Package', '').endswith('/internal/' + p) for e in top) for p in PACKAGES}
    failures = sum(e.get('Action') == 'fail' for e in events)
    skips = sum(e.get('Action') == 'skip' and 'Test' in e for e in events)
    return {'passed': exit_code == 0 and not failures and not skips and not missing and all(packages.values()),
            'exitCode': exit_code, 'topPassed': len(top), 'subPassed': len(passed)-len(top),
            'failures': failures, 'skips': skips, 'missingRequiredTests': missing, 'packages': packages}


def summarize_frontend(output, exit_code):
    def metric(name):
        values = re.findall(r'^(?:#|\u2139) ' + name + r' (\d+)\s*$', output, re.MULTILINE)
        return int(values[-1]) if len(values) == 1 else None
    tests, passed, failed, skipped = (metric(n) for n in ('tests', 'pass', 'fail', 'skipped'))
    return {'passed': exit_code == 0 and tests is not None and tests > 0 and passed == tests and failed == 0 and skipped == 0,
            'exitCode': exit_code, 'tests': tests, 'testsPassed': passed, 'failures': failed, 'skips': skipped}


def redactor(env):
    values = [v for k, v in env.items() if v and re.search(r'(PASSWORD|TOKEN|SECRET|DSN)', k)]
    for name in DATABASES:
        match = re.match(r'[^:]*:(.*)@(?:tcp|unix)\(', env.get(name, ''))
        if match and match[1]:
            values.append(match[1])
    values = sorted(set(values), key=len, reverse=True)
    def scrub(value):
        for secret in values:
            value = value.replace(secret, '[REDACTED]')
        return value
    return scrub


def main():
    problems = preflight(os.environ)
    if problems:
        print(json.dumps({'status': 'preflight_failed', 'errors': problems}))
        return 2
    git_path = subprocess.check_output(['git', 'rev-parse', '--git-path', 'm3-checks'], cwd=ROOT, text=True).strip()
    run = Path(git_path)
    if not run.is_absolute():
        run = ROOT / run
    run /= datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
    run.mkdir(parents=True, mode=0o700)
    scrub = redactor(os.environ)
    report = {'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
              'scope': 'local_development_only', 'status': 'running', 'stages': {}, 'evidence': str(run)}
    # Include source hashes so dirty working trees cannot masquerade as HEAD.
    names = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0')
    report['sourceHashes'] = {p: hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in names
                              if p and (ROOT/p).is_file() and p.startswith(('backend/', 'frontend/', 'contracts/', 'scripts/'))}
    report['sourceHashes']['scripts/check-m3-delivery.py'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    report['sourceHashes']['scripts/tests/test_m3_delivery_check.py'] = hashlib.sha256((ROOT/'scripts/tests/test_m3_delivery_check.py').read_bytes()).hexdigest()
    def save():
        (run/'summary.json').write_text(json.dumps(report, indent=2)+'\n')
    def command(stage, args, cwd):
        lines = []
        with (run/(stage+'.log')).open('w') as log:
            proc = subprocess.Popen(args, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in proc.stdout:
                log.write(scrub(line)); lines.append(line)
            code = proc.wait()
        result = summarize_frontend(''.join(lines), code) if stage == 'frontend' else {'passed': code == 0, 'exitCode': code}
        report['stages'][stage] = result; save()
        print(stage + (': passed' if result['passed'] else ': FAILED'), flush=True)
        return result['passed']
    events = []
    with (run/'backend.jsonl').open('w') as log, (run/'backend-stderr.log').open('w') as stderr:
        # Read stderr after completion to redact before retaining it.
        import tempfile
        with tempfile.TemporaryFile(mode='w+') as raw_stderr:
            proc = subprocess.Popen(['go', 'test', '-mod=readonly', '-race', '-count=1', '-json', '-cover', './...'],
                                    cwd=ROOT/'backend', stdout=subprocess.PIPE, stderr=raw_stderr, text=True)
            malformed = False
            for line in proc.stdout:
                try:
                    event = json.loads(line); events.append(event)
                    if 'Output' in event:
                        event = dict(event, Output=scrub(event['Output']))
                    log.write(json.dumps(event, ensure_ascii=False)+'\n')
                except ValueError:
                    malformed = True; log.write(json.dumps({'invalidOutput': scrub(line)})+'\n')
            code = proc.wait(); raw_stderr.seek(0); stderr.write(scrub(raw_stderr.read()))
    result = summarize(events, code)
    result['malformedOutput'] = malformed
    result['passed'] = result['passed'] and not malformed
    report['stages']['backend'] = result; save()
    print('backend: '+('passed' if result['passed'] else 'FAILED')+'; evidence: '+str(run), flush=True)
    if not result['passed']:
        report['status'] = 'failed'; save(); return 1
    stages = [('vet', ['go', 'vet', '-mod=readonly', './...'], ROOT/'backend'),
              ('backend-build', ['go', 'build', '-mod=readonly', '-o', str(run/'campus-server'), './cmd/server'], ROOT/'backend'),
              ('frontend', ['bash', 'scripts/check-m3-preview.sh'], ROOT)]
    for stage, args, cwd in stages:
        if not command(stage, args, cwd):
            report['status'] = 'failed'; save(); return 1
    report['status'] = 'passed'; report['notVerified'] = ['team_review', 'manual_acceptance', 'INT-04', 'fresh_database', 'remote_ci', 'deployment']
    save(); return 0


if __name__ == '__main__':
    sys.exit(main())
