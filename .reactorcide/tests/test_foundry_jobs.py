"""Tests for the Foundry Runnerlib lifecycle jobs."""

from __future__ import annotations

import importlib.util
import json
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
workflow_module = types.ModuleType("src.workflow")
workflow_module.set_workflow_var = mock.Mock()
workflow_module.workflow_vars = mock.Mock(return_value={})
src_module = types.ModuleType("src")
sys.modules.setdefault("src", src_module)
sys.modules.setdefault("src.logging", logging_module)
sys.modules.setdefault("src.plugins", plugins_module)
sys.modules.setdefault("src.workflow", workflow_module)

spec = importlib.util.spec_from_file_location("plugin_foundry_jobs", PLUGIN_PATH)
assert spec is not None and spec.loader is not None
jobs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(jobs)


class FoundryJobsTests(unittest.TestCase):
    def tearDown(self) -> None:
        for name in (
            "FOUNDRY_CI_JOB",
            "FOUNDRY_RELEASE_JOB",
            "FOUNDRY_RELEASE_PLATFORM",
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

    def test_go_job_runs_ci_plugin_tests(self) -> None:
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        with mock.patch.object(jobs, "_go_environment", return_value={}):
            with mock.patch.object(jobs, "_run", return_value=completed) as run:
                jobs.test_go(REPOSITORY_ROOT)

        commands = [call.args[0] for call in run.call_args_list]
        self.assertIn(
            [
                "python3",
                "-m",
                "unittest",
                "discover",
                "-s",
                REPOSITORY_ROOT / ".reactorcide" / "tests",
                "-p",
                "test_*.py",
                "-v",
            ],
            commands,
        )

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
            with mock.patch.object(jobs, "_run", return_value=completed) as run:
                metadata = jobs._release_metadata(Path("/tmp"), "v1", {})
        self.assertEqual(metadata, {"tag": "v1/v1.2.3", "version": "1.2.3"})
        self.assertEqual(
            run.call_args.args[0],
            [
                Path("semver-tags"),
                "run",
                "--output_json",
                "--directories",
                "v1",
            ],
        )

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

    def test_cli_release_caches_one_platform_asset(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        state = {
            "lane": "v1.2.3",
            "uploads": {
                "foundry-linux-arm64.tar.gz": {
                    "asset": "https://cache.test/asset",
                    "sha256": "https://cache.test/digest",
                }
            },
            "downloads": {},
            "manifest": "https://cache.test/manifest",
        }
        with tempfile.TemporaryDirectory() as directory:
            asset = Path(directory) / "foundry-1.2.3-linux-arm64.tar.gz"
            asset.write_bytes(b"archive")
            with mock.patch.object(
                jobs,
                "_active_release_metadata",
                return_value=metadata,
            ):
                with mock.patch.object(
                    jobs,
                    "_release_platform",
                    return_value=("linux", "arm64"),
                ):
                    with mock.patch.object(
                        jobs,
                        "_release_cache_state",
                        return_value=state,
                    ):
                        with mock.patch.object(
                            jobs,
                            "_build_release_asset",
                            return_value=asset,
                        ):
                            with mock.patch.object(
                                jobs,
                                "_put_presigned",
                            ) as put:
                                jobs.release_server_cli(REPOSITORY_ROOT)

        self.assertEqual(put.call_count, 2)
        self.assertEqual(put.call_args_list[0].args, ("https://cache.test/asset", b"archive"))
        self.assertEqual(put.call_args_list[1].args[0], "https://cache.test/digest")

    def test_prepare_recovers_missing_latest_release_as_draft(self) -> None:
        metadata = {"tag": "v1/v0.7.4", "version": "0.7.4"}
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        environment = {
            "GITHUB_PAT": "test-github-token",
            "REACTORCIDE_REPO": "catalystcommunity/foundry",
        }
        with mock.patch.dict(os.environ, environment, clear=False):
            with mock.patch.object(jobs, "_prepare_release_repository", return_value={}):
                with mock.patch.object(jobs, "_ensure_gh"):
                    with mock.patch.object(jobs, "_release_metadata", return_value=None):
                        with mock.patch.object(
                            jobs,
                            "_recoverable_server_release",
                            return_value=metadata,
                        ):
                            with mock.patch.object(
                                jobs,
                                "_github_release",
                                return_value=None,
                            ):
                                with mock.patch.object(
                                    jobs,
                                    "_write_release_workflow_state",
                                ) as write_state:
                                    with mock.patch.object(
                                        jobs,
                                        "_prepare_release_cache",
                                    ) as prepare_cache:
                                        with mock.patch.object(
                                            jobs,
                                            "_run",
                                            return_value=completed,
                                        ) as run:
                                            jobs.release_server_prepare(REPOSITORY_ROOT)

        args = run.call_args_list[0].args[0]
        self.assertEqual(args[:4], ["gh", "release", "create", "v1/v0.7.4"])
        self.assertIn("--draft", args)
        self.assertIn("--generate-notes", args)
        expected = {**metadata, "source_commit": ""}
        prepare_cache.assert_called_once_with(expected)
        write_state.assert_called_once_with(REPOSITORY_ROOT, expected)

    def test_recovery_reuses_tag_when_releasable_source_is_unchanged(self) -> None:
        completed = [
            subprocess.CompletedProcess([], 0, "v1/v0.7.4\n"),
            subprocess.CompletedProcess([], 0, "0ea4dcd\n"),
            subprocess.CompletedProcess([], 0, ""),
        ]
        with mock.patch.object(jobs, "_github_release", return_value=None):
            with mock.patch.object(jobs, "_run", side_effect=completed) as run:
                metadata = jobs._recoverable_server_release(REPOSITORY_ROOT)

        self.assertEqual(metadata, {"tag": "v1/v0.7.4", "version": "0.7.4"})
        diff_command = run.call_args_list[2].args[0]
        self.assertEqual(diff_command[-2:], ["v1", "csil"])

    def test_recovery_rejects_changed_releasable_source(self) -> None:
        completed = [
            subprocess.CompletedProcess([], 0, "v1/v0.7.4\n"),
            subprocess.CompletedProcess([], 0, "0ea4dcd\n"),
            subprocess.CompletedProcess([], 1, ""),
        ]
        with mock.patch.object(jobs, "_github_release", return_value=None):
            with mock.patch.object(jobs, "_run", side_effect=completed):
                with self.assertRaisesRegex(RuntimeError, "source changed"):
                    jobs._recoverable_server_release(REPOSITORY_ROOT)

    def test_release_workflow_state_contains_shared_build_metadata(self) -> None:
        metadata = {"tag": "v1/v1.2.3", "version": "1.2.3"}
        completed = subprocess.CompletedProcess([], 0, "abcdef0\n")
        with mock.patch.object(jobs, "_run", return_value=completed):
            with mock.patch.object(jobs, "set_workflow_var") as set_var:
                jobs._write_release_workflow_state(REPOSITORY_ROOT, metadata)

        values = {call.args[0]: call.args[1] for call in set_var.call_args_list}
        self.assertEqual(values[jobs.RELEASE_ACTIVE_VAR], "true")
        self.assertEqual(values[jobs.RELEASE_TAG_VAR], "v1/v1.2.3")
        self.assertEqual(values[jobs.RELEASE_VERSION_VAR], "1.2.3")
        self.assertEqual(values[jobs.RELEASE_COMMIT_VAR], "abcdef0")
        self.assertEqual(values[jobs.RELEASE_SOURCE_VAR], "abcdef0")
        self.assertRegex(
            values[jobs.RELEASE_DATE_VAR],
            r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$",
        )

    def test_prepare_cache_creates_six_staged_uploads_and_downloads(self) -> None:
        cache = mock.Mock()
        cache.get_bytes.side_effect = FileNotFoundError
        cache.presign.side_effect = (
            lambda method, key: f"https://cache.test/{method.lower()}/{key}"
        )
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "source_commit": "abcdef0123456789",
        }
        with mock.patch.object(
            jobs.ASSET_CACHE.S3Cache,
            "from_environment",
            return_value=cache,
        ):
            with mock.patch.object(jobs, "set_workflow_var") as set_var:
                jobs._prepare_release_cache(metadata)

        key, state = set_var.call_args.args
        self.assertEqual(key, jobs.RELEASE_CACHE_VAR)
        self.assertEqual(state["lane"], "v1.2.3")
        self.assertEqual(set(state["uploads"]), jobs._expected_cache_asset_names())
        self.assertEqual(set(state["downloads"]), jobs._expected_cache_asset_names())
        self.assertIn("complete.json", state["manifest"])

    def test_seal_verifies_and_promotes_all_six_cached_assets(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        lane = "v1.2.3"
        uploads = {}
        objects = {}
        for asset in jobs._expected_cache_asset_names():
            staging = "staging-" + asset
            content = ("archive:" + asset).encode()
            objects[jobs.ASSET_CACHE.object_key(lane, staging)] = content
            objects[jobs.ASSET_CACHE.object_key(lane, staging + ".sha256")] = (
                jobs.hashlib.sha256(content).hexdigest() + "\n"
            ).encode()
            uploads[asset] = {"asset": "put", "sha256": "put-digest"}
        state = {
            "lane": lane,
            "uploads": uploads,
            "downloads": {},
            "manifest": "https://cache.test/manifest",
        }
        cache = mock.Mock()
        cache.get_bytes.side_effect = lambda key: objects[key]
        cache.copy.side_effect = lambda source, destination: objects.__setitem__(
            destination,
            objects[source],
        )
        cache.put_bytes.side_effect = lambda key, content: objects.__setitem__(
            key,
            content,
        )
        cache.delete.side_effect = lambda key: objects.pop(key)
        with mock.patch.object(
            jobs,
            "_active_release_metadata",
            return_value=metadata,
        ):
            with mock.patch.object(jobs, "_release_cache_state", return_value=state):
                with mock.patch.object(
                    jobs.ASSET_CACHE.S3Cache,
                    "from_environment",
                    return_value=cache,
                ):
                    jobs.release_server_seal(REPOSITORY_ROOT)

        manifest = jobs.ASSET_CACHE.decode_manifest(
            objects[jobs.ASSET_CACHE.object_key(lane, jobs.ASSET_CACHE.MANIFEST)]
        )
        self.assertEqual(manifest["source_commit"], metadata["source_commit"])
        self.assertEqual(
            {asset["name"] for asset in manifest["assets"]},
            jobs._expected_cache_asset_names(),
        )
        for asset in jobs._expected_cache_asset_names():
            self.assertIn(jobs.ASSET_CACHE.object_key(lane, asset), objects)

    def test_seal_rejects_a_bad_cached_asset_checksum(self) -> None:
        asset = sorted(jobs._expected_cache_asset_names())[0]
        lane = "v1.2.3"
        state = {
            "lane": lane,
            "uploads": {asset: {"asset": "put", "sha256": "put-digest"}},
            "downloads": {},
            "manifest": "https://cache.test/manifest",
        }
        cache = mock.Mock()
        cache.get_bytes.side_effect = (b"archive", b"wrong\n")
        with mock.patch.object(
            jobs,
            "_active_release_metadata",
            return_value={
                "tag": "v1/v1.2.3",
                "version": "1.2.3",
                "git_commit": "abcdef0",
                "source_commit": "abcdef0123456789",
                "build_date": "2026-09-06T00:00:00Z",
            },
        ):
            with mock.patch.object(jobs, "_release_cache_state", return_value=state):
                with mock.patch.object(
                    jobs.ASSET_CACHE.S3Cache,
                    "from_environment",
                    return_value=cache,
                ):
                    with self.assertRaisesRegex(RuntimeError, "checksum"):
                        jobs.release_server_seal(REPOSITORY_ROOT)

    def test_cached_asset_download_checks_manifest_and_digest(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        payloads = {
            asset: ("archive:" + asset).encode()
            for asset in jobs._expected_cache_asset_names()
        }
        manifest = jobs.ASSET_CACHE.encode_manifest(
            {
                "schema": 1,
                "project": "foundry",
                "lane": "v1.2.3",
                "tag": metadata["tag"],
                "version": metadata["version"],
                "source_commit": metadata["source_commit"],
                "assets": [
                    {
                        "name": asset,
                        "sha256": jobs.hashlib.sha256(content).hexdigest(),
                        "size": len(content),
                    }
                    for asset, content in payloads.items()
                ],
            }
        )
        state = {
            "lane": "v1.2.3",
            "uploads": {},
            "downloads": {
                asset: "https://cache.test/" + asset for asset in payloads
            },
            "manifest": "https://cache.test/manifest",
        }

        def get_url(url):
            if url.endswith("/manifest"):
                return manifest
            return payloads[url.rsplit("/", 1)[-1]]

        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(jobs, "_release_cache_state", return_value=state):
                with mock.patch.object(jobs, "_get_presigned", side_effect=get_url):
                    downloaded = jobs._download_cached_release_assets(
                        Path(directory),
                        metadata,
                        (("linux", "amd64"), ("linux", "arm64")),
                    )
            self.assertEqual(
                downloaded[("linux", "amd64")].read_bytes(),
                payloads["foundry-linux-amd64.tar.gz"],
            )
            self.assertEqual(
                downloaded[("linux", "arm64")].read_bytes(),
                payloads["foundry-linux-arm64.tar.gz"],
            )

    def test_multiarch_image_build_uses_both_linux_platforms(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        environment = {
            "REGISTRY": "containers.example.test",
            "IMAGE_PATH": "foundry",
        }
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        with mock.patch.dict(os.environ, environment, clear=False):
            with mock.patch.object(
                jobs,
                "_active_release_metadata",
                return_value=metadata,
            ):
                with mock.patch.object(jobs, "_registry_environment", return_value={}):
                    with mock.patch.object(
                        jobs,
                        "_ensure_buildctl",
                        return_value=Path("buildctl"),
                    ):
                        with mock.patch.object(
                            jobs,
                            "_ensure_crane",
                            return_value=Path("crane"),
                        ):
                            with mock.patch.object(jobs, "_wait_for_buildkit"):
                                with mock.patch.object(
                                    jobs,
                                    "_download_cached_release_assets",
                                    return_value={
                                        ("linux", "amd64"): Path("amd64.tar.gz"),
                                        ("linux", "arm64"): Path("arm64.tar.gz"),
                                    },
                                ) as download:
                                    with mock.patch.object(jobs, "_extract_release_binary"):
                                        with mock.patch.object(
                                            jobs,
                                            "_verify_multiarch_image",
                                        ) as verify:
                                            with mock.patch.object(
                                                jobs,
                                                "_run",
                                                return_value=completed,
                                            ) as run:
                                                jobs.release_server_image(REPOSITORY_ROOT)

        args = run.call_args.args[0]
        self.assertIn("filename=Dockerfile.release", args)
        self.assertIn("platform=linux/amd64,linux/arm64", args)
        self.assertFalse(any("build-arg" in str(arg) for arg in args))
        self.assertIn(
            'type=image,"name=containers.example.test/foundry:1.2.3,'
            'containers.example.test/foundry:latest",push=true',
            args,
        )
        download.assert_called_once()
        self.assertEqual(
            download.call_args.args[2],
            (("linux", "amd64"), ("linux", "arm64")),
        )
        verify.assert_called_once_with(
            Path("crane"),
            REPOSITORY_ROOT,
            "containers.example.test/foundry:1.2.3",
            {},
        )

    def test_dockerfile_cross_compiles_on_the_build_platform(self) -> None:
        dockerfile = (REPOSITORY_ROOT / "Dockerfile").read_text(encoding="utf-8")
        self.assertIn("FROM --platform=$BUILDPLATFORM golang:1.25 AS build", dockerfile)
        self.assertIn("ARG TARGETOS", dockerfile)
        self.assertIn("ARG TARGETARCH", dockerfile)
        self.assertIn(
            "CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build",
            dockerfile,
        )

    def test_release_image_packages_cached_binaries_without_building(self) -> None:
        dockerfile = (REPOSITORY_ROOT / "Dockerfile.release").read_text(
            encoding="utf-8"
        )
        self.assertIn(
            "COPY ${TARGETOS}-${TARGETARCH}/foundry /usr/local/bin/foundry",
            dockerfile,
        )
        self.assertNotIn("go build", dockerfile)
        self.assertNotIn("FROM golang", dockerfile)

    def test_release_image_extracts_the_exact_cached_linux_binary(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "foundry"
            source.write_bytes(b"exact cached binary")
            archive_path = root / "foundry-linux-amd64.tar.gz"
            with tarfile.open(archive_path, "w:gz") as archive:
                archive.add(source, arcname="foundry", recursive=False)
            destination = root / "image" / "linux-amd64" / "foundry"

            jobs._extract_release_binary(archive_path, destination, "linux")

            self.assertEqual(destination.read_bytes(), b"exact cached binary")
            self.assertTrue(destination.stat().st_mode & 0o100)

    def test_multiarch_image_verification_rejects_missing_arm64(self) -> None:
        manifest = json.dumps(
            {
                "manifests": [
                    {"platform": {"os": "linux", "architecture": "amd64"}}
                ]
            }
        )
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout=manifest)
        with mock.patch.object(jobs, "_run", return_value=completed):
            with self.assertRaisesRegex(RuntimeError, "linux/amd64 and linux/arm64"):
                jobs._verify_multiarch_image(
                    Path("crane"),
                    REPOSITORY_ROOT,
                    "containers.example.test/foundry:1.2.3",
                    {},
                )

    def test_multiarch_image_verification_accepts_required_platforms(self) -> None:
        manifest = json.dumps(
            {
                "manifests": [
                    {"platform": {"os": "linux", "architecture": "amd64"}},
                    {"platform": {"os": "linux", "architecture": "arm64"}},
                ]
            }
        )
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout=manifest)
        with mock.patch.object(jobs, "_run", return_value=completed):
            jobs._verify_multiarch_image(
                Path("crane"),
                REPOSITORY_ROOT,
                "containers.example.test/foundry:1.2.3",
                {},
            )

    def test_multiarch_image_verification_rejects_non_index_manifest(self) -> None:
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="[]")
        with mock.patch.object(jobs, "_run", return_value=completed):
            with self.assertRaisesRegex(RuntimeError, "invalid manifest"):
                jobs._verify_multiarch_image(
                    Path("crane"),
                    REPOSITORY_ROOT,
                    "containers.example.test/foundry:1.2.3",
                    {},
                )

    def test_publish_rejects_invalid_cached_assets(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        release = {"isDraft": True, "assets": []}
        with mock.patch.object(
            jobs,
            "_active_release_metadata",
            return_value=metadata,
        ):
            with mock.patch.object(jobs, "_ensure_gh"):
                with mock.patch.object(jobs, "_github_release", return_value=release):
                    with mock.patch.object(
                        jobs,
                        "_download_cached_release_assets",
                        side_effect=RuntimeError("invalid cached asset"),
                    ):
                        with self.assertRaisesRegex(RuntimeError, "invalid cached asset"):
                            jobs.release_server_publish(REPOSITORY_ROOT)

    def test_publish_updates_chart_and_publishes_complete_draft(self) -> None:
        metadata = {
            "tag": "v1/v1.2.3",
            "version": "1.2.3",
            "git_commit": "abcdef0",
            "source_commit": "abcdef0123456789",
            "build_date": "2026-09-06T00:00:00Z",
        }
        draft = {"isDraft": True, "assets": []}
        complete = {
            "isDraft": True,
            "assets": [
                {"name": name}
                for name in jobs._expected_release_asset_names("1.2.3")
            ],
        }
        downloaded = {
            target: Path(jobs._release_asset_name("1.2.3", *target))
            for target in jobs.RELEASE_PLATFORMS
        }
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        environment = {
            "GITHUB_PAT": "test-github-token",
            "REACTORCIDE_REPO": "catalystcommunity/foundry",
        }
        with mock.patch.dict(os.environ, environment, clear=False):
            with mock.patch.object(
                jobs,
                "_active_release_metadata",
                return_value=metadata,
            ):
                with mock.patch.object(jobs, "_ensure_gh"):
                    with mock.patch.object(
                        jobs,
                        "_github_release",
                        side_effect=(draft, complete),
                    ):
                        with mock.patch.object(
                            jobs,
                            "_download_cached_release_assets",
                            return_value=downloaded,
                        ):
                            with mock.patch.object(
                            jobs,
                            "_prepare_release_repository",
                            return_value={},
                            ):
                                with mock.patch.object(
                                    jobs,
                                    "_replace_yaml_scalar",
                                ) as replace:
                                    with mock.patch.object(jobs, "_commit_and_push_chart"):
                                        with mock.patch.object(
                                            jobs,
                                            "_run",
                                            return_value=completed,
                                        ) as run:
                                            jobs.release_server_publish(REPOSITORY_ROOT)

        replace.assert_called_once_with(
            REPOSITORY_ROOT
            / "v1"
            / "charts"
            / "foundry-gateway-controller"
            / "Chart.yaml",
            "appVersion",
            "1.2.3",
        )
        commands = [call.args[0] for call in run.call_args_list]
        upload = next(command for command in commands if command[:3] == ["gh", "release", "upload"])
        self.assertEqual(upload[3], "v1/v1.2.3")
        self.assertEqual(set(upload[4:10]), set(downloaded.values()))
        edit = next(command for command in commands if command[:3] == ["gh", "release", "edit"])
        self.assertIn("--draft=false", edit)

    def test_pr_image_build_uses_both_linux_platforms(self) -> None:
        completed = subprocess.CompletedProcess(args=[], returncode=0, stdout="")
        with mock.patch.object(
            jobs,
            "_ensure_buildctl",
            return_value=Path("buildctl"),
        ):
            with mock.patch.object(jobs, "_run", return_value=completed) as run:
                jobs.test_image_build(REPOSITORY_ROOT)

        build_command = run.call_args_list[-1].args[0]
        self.assertIn("platform=linux/amd64,linux/arm64", build_command)

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

    def test_release_workflow_runs_for_release_job_changes(self) -> None:
        workflow = (
            REPOSITORY_ROOT / ".reactorcide" / "workflows" / "release-server.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn('- ".reactorcide/**"', workflow)
        for os_name, arch in jobs.RELEASE_PLATFORMS:
            self.assertIn(f"FOUNDRY_RELEASE_PLATFORM: {os_name}/{arch}", workflow)
        self.assertIn("FOUNDRY_RELEASE_JOB: prepare", workflow)
        self.assertIn("FOUNDRY_RELEASE_JOB: publish", workflow)
        self.assertIn("asset-seal:", workflow)
        self.assertIn("Dockerfile.release", workflow)

    def test_cli_build_jobs_use_signed_cache_urls_without_secrets(self) -> None:
        job = (
            REPOSITORY_ROOT / ".reactorcide" / "jobs" / "release-cli.yaml"
        ).read_text(encoding="utf-8")
        self.assertNotIn("${secret:", job)
        self.assertNotIn("GITHUB_PAT", job)

    def test_release_asset_control_uses_the_isolated_cache_secret(self) -> None:
        job = (
            REPOSITORY_ROOT
            / ".reactorcide"
            / "jobs"
            / "release-asset-control.yaml"
        ).read_text(encoding="utf-8")
        self.assertIn("catalystcommunity/asset-cache", job)
        self.assertNotIn("GITHUB_PAT", job)

    def test_release_build_jobs_request_six_cpu_and_eight_gb(self) -> None:
        job_dir = REPOSITORY_ROOT / ".reactorcide" / "jobs"
        for name in ("release-cli.yaml", "release-server.yaml"):
            job = (job_dir / name).read_text(encoding="utf-8")
            self.assertIn('request: "6"', job)
            self.assertIn('limit: "6"', job)
            self.assertIn('limit: "8GB"', job)

    def test_pr_test_and_image_workflows_run_for_ci_changes(self) -> None:
        workflows = REPOSITORY_ROOT / ".reactorcide" / "workflows"
        for name in ("pr-test.yaml", "pr-image.yaml"):
            workflow = (workflows / name).read_text(encoding="utf-8")
            self.assertIn('- ".reactorcide/**"', workflow)

    def test_bash_job_directory_is_empty(self) -> None:
        scripts = REPOSITORY_ROOT / ".reactorcide" / "jobs" / "scripts"
        self.assertFalse(scripts.exists() and any(scripts.iterdir()))


if __name__ == "__main__":
    unittest.main()
