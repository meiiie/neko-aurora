import os
from pathlib import Path
import tempfile
import textwrap
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = (ROOT / '.github/workflows/ci.yml').read_text()
FILTER = textwrap.dedent(WORKFLOW.split("          python - <<'PY'\n", 1)[1].split('          PY', 1)[0])


class PathFilters(unittest.TestCase):
    def selected(self, files, event='pull_request'):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            with patch.dict(os.environ, EVENT=event, BASE='fixture', GITHUB_OUTPUT=str(output)):
                with patch('subprocess.check_output', return_value=('\0'.join(files) + '\0').encode()):
                    exec(FILTER, {})
            return {line.split('=')[0] for line in output.read_text().splitlines() if line.endswith('=true')}

    def test_shared_inputs(self):
        for path, expected in [
            ('app/contracts/canonical-v1.json', {'native', 'host', 'desktop'}),
            ('app/scripts/adapter_history_repair_vectors_20261013.json', {'native', 'host', 'desktop'}),
            ('app/scripts/cloud-worker-fixture.mjs', {'native', 'host', 'desktop'}),
            ('app/cloud-browser/server.mjs', {'native'}),
            ('app/scripts/core_supervisor.py', {'native', 'host', 'desktop'}),
            ('app/native/internal/protocol/core-runtimes-v1.json', {'native', 'host', 'desktop'}),
            ('migration/disabled-workflows/core/ci.yml', {'core'}),
            ('migration/disabled-workflows/core/release.yml', {'core'}),
        ]:
            with self.subTest(path=path):
                self.assertEqual(self.selected([path]), expected)

    def test_full_and_empty(self):
        all_jobs = {'native', 'host', 'desktop', 'core', 'docs'}
        self.assertEqual(self.selected([]), set())
        self.assertEqual(self.selected([], 'push'), all_jobs)
        self.assertEqual(self.selected([], 'workflow_dispatch'), all_jobs)
        self.assertEqual(self.selected(['.github/workflows/ci.yml']), all_jobs)
        self.assertEqual(self.selected(['tools/ci/core-tests.ts']), all_jobs)

    def test_components(self):
        self.assertEqual(self.selected(['app/native/go.mod']), {'native'})
        self.assertEqual(self.selected(['app/host/Cargo.toml']), {'host', 'desktop'})
        self.assertEqual(self.selected(['core/src/core/agent.ts']), {'core'})
        self.assertEqual(self.selected(['README.md']), {'docs'})


if __name__ == '__main__':
    unittest.main()
