#!/usr/bin/env python3
"""Check detached LuCI jobs, argument handling and error propagation locally."""
from pathlib import Path
import subprocess
import tempfile
import time

with tempfile.TemporaryDirectory(prefix='zakop-luci-test-') as temporary:
    base = Path(temporary)
    worker = base / 'zakopd'
    worker.write_text('#!/bin/sh\nsleep 1\nprintf "%s\\n" "$@"\necho diagnostic >&2\nexit 7\n')
    worker.chmod(0o755)
    helper = base / 'job.sh'
    helper.write_text(Path('embedded/files/usr/share/zakop/luci-job.sh').read_text()
                      .replace('/tmp/zakop-luci-jobs', str(base / 'jobs'))
                      .replace('/usr/bin/zakopd', str(worker)))
    helper.chmod(0o755)

    def run(*args, timeout=5):
        return subprocess.run([str(helper), *args], capture_output=True, text=True, timeout=timeout)

    # Closing the parent must not wait on the worker's inherited output pipes.
    started = run('start', 'latency', 'node with spaces', timeout=0.75)
    assert started.returncode == 0, started
    job = started.stdout.strip()
    assert run('status', job).stdout.startswith('running\n')
    deadline = time.monotonic() + 5
    while True:
        result = run('status', job)
        if result.stdout.startswith('exit:'):
            break
        assert time.monotonic() < deadline, result
        time.sleep(0.05)
    assert result.stdout.startswith('exit:7\n'), result
    assert result.stdout.endswith('node with spaces\n'), result
    assert result.stderr == 'diagnostic\n', result
    direct = run('status-direct', job)
    assert direct.stdout.endswith('\n__ZAKOP_JOB_STDERR__\ndiagnostic\n'), direct
    assert direct.stderr == '', direct
    assert run('remove', job).returncode == 0
    assert run('status', job).returncode != 0
    for args in [('start', 'arbitrary'), ('start', 'providers', '../invalid'),
                 ('status', 'job.../etc'), ('start', 'latency', '-config')]:
        assert run(*args).returncode != 0, args
print('LuCI background job checks passed')
