"""Tests for the Foundry Runnerlib lifecycle jobs."""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tarfile
import tempfile
import types
import unittest
import zipfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
PLUGIN_PATH = (
    REPOSITORY_ROOT / ".reactorcide" / "plugins" / "plugin_foundry_jobs.py"
)


class Plugin:
    def __init__(self, name: str, priority: int):
        self.name = name
        self.priority = priority


class PluginPhase:
    POST_SOURCE_PREP = "post_source_prep"


logging_module = types.ModuleType("src.logging")
logging_module.log_stdout = mock.Mock()
plugins_module = types.ModuleType("src.plugins")
plugins_module.Plugin = Plugin
plugins_module.PluginContext = object
plugins_module.PluginPhase = PluginPhase
src_module = types.ModuleType("src")
sys.modules.setdefault("src", src_module)
sys.modules.setdefault("src.logging", logging_module)
sys.modules.setdefault("src.plugins", plugins_module)

spec = importlib.util.spec_from_file_location("plugin_foundry_jobs", PLUGIN_PATH)
assert spec is not None and spec.loader is not None
jobs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(jobs)


class FoundryJobsTests(unittest.TestCase):
    def tearDown(self) -> None:
        for name in (
            "FOUNDRY_CI_JOB",
            "FOUNDRY_RELEASE_JOB",
            "REACTORCIDE_DIFF_BASE",
        ):
            os.environ.pop(name, None)

    def test_conventional_commit_pattern(self) -> None:
        accepted = (
            "feat: add lifecycle jobs",
            "fix(k3s): reconcile node versions",
            "docs!: replace old workflow",
            "norelease: update generated state",
        )
        rejected = (
            "Add lifecycle jobs",
            "feature: unsupported type",
            "fix missing separator",
            "fix:",
        )
        for subject in accepted:
            self.assertIsNotNone(jobs.CONVENTIONAL_COMMIT_PATTERN.fullmatch(subject))
        for subject in rejected:
            self.assertIsNone(jobs.CONVENTIONAL_COMMIT_PATTERN.fullmatch(subject))

    def test_architecture_mapping_supports_amd64_and_arm64(self) -> None:
        with mock.patch.object(jobs.platform, "machine", return_value="x86_64"):
            self.assertEqual(jobs._machine_arch(), ("amd64", "x86_64"))
        with mock.patch.object(jobs.platform, "machine", return_value="aarch64"):
            self.assertEqual(jobs._machine_arch(), ("arm64", "arm64"))

    def test_architecture_mapping_rejects_unknown_machine(self) -> None:
        with mock.patch.object(jobs.platform, "machine", return_value="riscv64"):
            with self.assertRaisesRegex(RuntimeError, "Unsupported CI runner"):
                jobs._machine_arch()

    def test_ci_plugin_dispatches_selected_job(self) -> None:
        selected = mock.Mock()
        context = SimpleNamespace(
            config=SimpleNamespace(code_dir=str(REPOSITORY_ROOT)),
            phase=PluginPhase.POST_SOURCE_PREP,
            metadata={},
        )
        with mock.patch.dict(jobs.CI_JOBS, {"test-go": selected}, clear=True):
            os.environ["FOUNDRY_CI_JOB"] = "test-go"
            jobs.FoundryCIJobsPlugin().execute(context)
        selected.assert_called_once_with(REPOSITORY_ROOT.resolve())

    def test_release_plugin_dispatches_selected_job(self) -> None:
        selected = mock.Mock()
        context = SimpleNamespace(
            config=SimpleNamespace(code_dir=str(REPOSITORY_ROOT)),
            phase=PluginPhase.POST_SOURCE_PREP,
            metadata={},
        )
        with mock.patch.dict(jobs.RELEASE_JOBS, {"server": selected}, clear=True):
            os.environ["FOUNDRY_RELEASE_JOB"] = "server"
            jobs.FoundryReleaseJobsPlugin().execute(context)
        selected.assert_called_once_with(REPOSITORY_ROOT.resolve())

    def test_plugins_are_inactive_without_a_selector(self) -> None:
        context = SimpleNamespace(
            config=SimpleNamespace(code_dir=str(REPOSITORY_ROOT)),
            phase=PluginPhase.POST_SOURCE_PREP,
            metadata={},
        )
        with mock.patch.dict(jobs.CI_JOBS, {"test-go": mock.Mock()}, clear=True):
            jobs.FoundryCIJobsPlugin().execute(context)
            jobs.CI_JOBS["test-go"].assert_not_called()
        with mock.patch.dict(jobs.RELEASE_JOBS, {"server": mock.Mock()}, clear=True):
            jobs.FoundryReleaseJobsPlugin().execute(context)
            jobs.RELEASE_JOBS["server"].assert_not_called()

    def test_unknown_selector_fails_with_valid_names(self) -> None:
        context = SimpleNamespace(
            config=SimpleNamespace(code_dir=str(REPOSITORY_ROOT)),
            phase=PluginPhase.POST_SOURCE_PREP,
            metadata={},
        )
        os.environ["FOUNDRY_CI_JOB"] = "unknown"
        with self.assertRaisesRegex(RuntimeError, "Valid jobs"):
            jobs.FoundryCIJobsPlugin().execute(context)

    def test_release_metadata_uses_the_last_json_line(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[],
            returncode=0,
            stdout=(
                "informational output\n"
                '{"New_release_git_tag":"v1/v1.2.3",'
                '"New_release_published":"true"}\n'
            ),
        )
        with mock.patch.object(jobs, "_ensure_semver_tags", return_value=Path("semver-tags")):
            with mock.patch.object(jobs, "_run", return_value=completed):
                metadata = jobs._release_metadata(Path("/tmp"), "v1", {})
        self.assertEqual(metadata, {"tag": "v1/v1.2.3", "version": "1.2.3"})

    def test_release_metadata_reports_no_release(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[],
            returncode=0,
            stdout=(
                '{"New_release_git_tag":"v1/v1.2.3",'
                '"New_release_published":"false"}\n'
            ),
        )
        with mock.patch.object(jobs, "_ensure_semver_tags", return_value=Path("semver-tags")):
            with mock.patch.object(jobs, "_run", return_value=completed):
                self.assertIsNone(jobs._release_metadata(Path("/tmp"), "v1", {}))

    def test_release_assets_use_exact_matrix_and_single_file_archives(self) -> None:
        version = "1.2.3"
        git_commit = "abcdef0"
        build_date = "2026-09-05T00:00:00Z"
        build_calls = []

        def run_command(args, *, cwd, env=None, capture=False, check=True):
            if args[0] == "go":
                output = Path(args[args.index("-o") + 1])
                output.write_bytes(b"foundry release binary")
                output.chmod(0o755)
                build_calls.append((args, cwd, env))
                return subprocess.CompletedProcess(args, 0, "")
            return subprocess.CompletedProcess(
                args,
                0,
                f"foundry version {version}\n",
            )

        expected_names = [
            "foundry-1.2.3-linux-amd64.tar.gz",
            "foundry-1.2.3-linux-arm64.tar.gz",
            "foundry-1.2.3-darwin-amd64.tar.gz",
            "foundry-1.2.3-darwin-arm64.tar.gz",
            "foundry-1.2.3-windows-amd64.zip",
            "foundry-1.2.3-windows-arm64.zip",
        ]
        expected_targets = list(jobs.RELEASE_PLATFORMS)
        expected_ldflags = (
            "-X main.Version=1.2.3 "
            "-X main.GitCommit=abcdef0 "
            "-X main.BuildDate=2026-09-05T00:00:00Z -s -w"
        )

        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(jobs, "_go_environment", side_effect=dict):
                with mock.patch.object(jobs, "_run", side_effect=run_command):
                    assets = jobs._build_release_assets(
                        REPOSITORY_ROOT,
                        Path(directory),
                        version,
                        git_commit,
                        build_date,
                    )

            self.assertEqual([asset.name for asset in assets], expected_names)
            for asset in assets:
                if asset.suffix == ".zip":
                    with zipfile.ZipFile(asset) as archive:
                        self.assertEqual(archive.namelist(), ["foundry.exe"])
                        self.assertEqual(
                            archive.read("foundry.exe"),
                            b"foundry release binary",
                        )
                else:
                    with tarfile.open(asset) as archive:
                        self.assertEqual(archive.getnames(), ["foundry"])
                        member = archive.extractfile("foundry")
                        self.assertIsNotNone(member)
                        self.assertEqual(member.read(), b"foundry release binary")

        self.assertEqual(len(build_calls), len(expected_targets))
        for (args, cwd, environment), (os_name, arch) in zip(
            build_calls,
            expected_targets,
        ):
            self.assertEqual(cwd, REPOSITORY_ROOT / "v1")
            self.assertEqual(args[args.index("-ldflags") + 1], expected_ldflags)
            self.assertEqual(args[-1], "./cmd/foundry")
            self.assertEqual(environment["CGO_ENABLED"], "0")
            self.assertEqual(environment["GOOS"], os_name)
            self.assertEqual(environment["GOARCH"], arch)

    def test_native_release_binary_must_report_version(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[],
            returncode=0,
            stdout="foundry version development\n",
        )
        with mock.patch.object(jobs.platform, "system", return_value="Linux"):
            with mock.patch.object(
                jobs,
                "_machine_arch",
                return_value=("amd64", "x86_64"),
            ):
                with mock.patch.object(jobs, "_run", return_value=completed):
                    with self.assertRaisesRegex(
                        RuntimeError,
                        "did not report 'foundry version 1.2.3'",
                    ):
                        jobs._verify_native_release_binary(
                            REPOSITORY_ROOT,
                            Path("foundry"),
                            "1.2.3",
                            "linux",
                            "amd64",
                        )

    def test_server_release_uploads_assets_to_existing_tag(self) -> None:
        assets = [Path(f"asset-{index}") for index in range(6)]
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        environment = {
            "REACTORCIDE_REPO": "catalystcommunity/foundry",
            "GITHUB_PAT": "secret",
        }
        with mock.patch.dict(os.environ, environment, clear=False):
            with mock.patch.object(jobs, "_run", return_value=completed) as run:
                jobs._create_server_github_release(
                    REPOSITORY_ROOT,
                    "v1/v1.2.3",
                    assets,
                )

        args = run.call_args.args[0]
        self.assertEqual(args[:4], ["gh", "release", "create", "v1/v1.2.3"])
        self.assertEqual(args[-6:], assets)
        self.assertIn("--generate-notes", args)
        self.assertNotIn("secret", args)

    def test_yaml_scalar_update_requires_one_field(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            chart = Path(directory) / "Chart.yaml"
            chart.write_text("version: 1.0.0\nname: foundry\n", encoding="utf-8")
            jobs._replace_yaml_scalar(chart, "version", "1.1.0")
            self.assertEqual(
                chart.read_text(encoding="utf-8"),
                'version: "1.1.0"\nname: foundry\n',
            )
            with self.assertRaisesRegex(RuntimeError, "Could not update"):
                jobs._replace_yaml_scalar(chart, "appVersion", "1.1.0")

    def test_all_jobs_use_runnerlib_without_raw_commands(self) -> None:
        for job_file in (REPOSITORY_ROOT / ".reactorcide" / "jobs").glob("*.yaml"):
            content = job_file.read_text(encoding="utf-8")
            self.assertIn("runnerlib run --job-command true", content)
            self.assertNotIn("raw_command:", content)

    def test_bash_job_directory_is_empty(self) -> None:
        scripts = REPOSITORY_ROOT / ".reactorcide" / "jobs" / "scripts"
        self.assertFalse(scripts.exists() and any(scripts.iterdir()))


if __name__ == "__main__":
    unittest.main()
