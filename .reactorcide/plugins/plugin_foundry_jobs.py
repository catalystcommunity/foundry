"""Runnerlib lifecycle jobs for Foundry CI and releases."""

from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import os
import platform
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path
from typing import Callable, Mapping, Sequence

from src.logging import log_stdout
from src.plugins import Plugin, PluginContext, PluginPhase
from src.workflow import set_workflow_var, workflow_vars


ASSET_CACHE_PATH = (
    Path(__file__).resolve().parents[1] / "scripts" / "asset_cache.py"
)
ASSET_CACHE_SPEC = importlib.util.spec_from_file_location(
    "foundry_asset_cache",
    ASSET_CACHE_PATH,
)
if ASSET_CACHE_SPEC is None or ASSET_CACHE_SPEC.loader is None:
    raise RuntimeError("The Foundry asset-cache module is not available")
ASSET_CACHE = importlib.util.module_from_spec(ASSET_CACHE_SPEC)
sys.modules[ASSET_CACHE_SPEC.name] = ASSET_CACHE
ASSET_CACHE_SPEC.loader.exec_module(ASSET_CACHE)


CONVENTIONAL_COMMIT_PATTERN = re.compile(
    r"^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|norelease)"
    r"(\(.+\))?!?: .+"
)

BUILDKIT_VERSION = "0.17.3"
CRANE_VERSION = "0.20.3"
GHCLI_VERSION = "2.63.2"
HELM_VERSION = "3.14.0"
SEMVER_TAGS_VERSION = "v0.4.0"
RELEASE_PLATFORMS = (
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
)
RELEASE_ACTIVE_VAR = "FOUNDRY_RELEASE_ACTIVE"
RELEASE_TAG_VAR = "FOUNDRY_RELEASE_TAG"
RELEASE_VERSION_VAR = "FOUNDRY_RELEASE_VERSION"
RELEASE_COMMIT_VAR = "FOUNDRY_RELEASE_GIT_COMMIT"
RELEASE_SOURCE_VAR = "FOUNDRY_RELEASE_SOURCE_COMMIT"
RELEASE_DATE_VAR = "FOUNDRY_RELEASE_BUILD_DATE"
RELEASE_CACHE_VAR = "FOUNDRY_RELEASE_CACHE"


def _repo_root(context: PluginContext) -> Path:
    configured = Path(context.config.code_dir)
    if configured.is_dir():
        return configured.resolve()
    source_path = context.metadata.get("source_path")
    if source_path:
        return Path(source_path).resolve()
    return Path("/job/src")


def _run(
    args: Sequence[str | Path],
    *,
    cwd: Path,
    env: Mapping[str, str] | None = None,
    capture: bool = False,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    """Run one command and put its output in the Runnerlib job log."""
    command = tuple(str(arg) for arg in args)
    log_stdout(f"+ {shlex.join(command)}")
    command_env = os.environ.copy()
    if env:
        command_env.update(env)

    if capture:
        completed = subprocess.run(
            command,
            cwd=cwd,
            env=command_env,
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        if completed.returncode != 0:
            for line in (completed.stdout or "").splitlines():
                log_stdout(f"  {line}")
    else:
        process = subprocess.Popen(
            command,
            cwd=cwd,
            env=command_env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            bufsize=1,
        )
        if process.stdout is None:
            raise RuntimeError(f"Could not capture output from {shlex.join(command)}")
        for line in process.stdout:
            log_stdout(f"  {line.rstrip()}")
        completed = subprocess.CompletedProcess(command, process.wait(), "")
    if check and completed.returncode != 0:
        raise RuntimeError(
            f"{shlex.join(command)} failed with exit status {completed.returncode}"
        )
    return completed


def _section(title: str) -> None:
    log_stdout("")
    log_stdout(f"=== {title} ===")


def _machine_arch() -> tuple[str, str]:
    machine = platform.machine().lower()
    arches = {
        "x86_64": ("amd64", "x86_64"),
        "amd64": ("amd64", "x86_64"),
        "aarch64": ("arm64", "arm64"),
        "arm64": ("arm64", "arm64"),
    }
    try:
        return arches[machine]
    except KeyError as error:
        raise RuntimeError(f"Unsupported CI runner architecture: {machine}") from error


def _home() -> Path:
    home = Path(os.environ.get("HOME", "/tmp"))
    home.mkdir(parents=True, exist_ok=True)
    return home


def _local_bin() -> Path:
    local_bin = _home() / ".local" / "bin"
    local_bin.mkdir(parents=True, exist_ok=True)
    path = os.environ.get("PATH", "")
    if str(local_bin) not in path.split(os.pathsep):
        os.environ["PATH"] = f"{local_bin}{os.pathsep}{path}"
    return local_bin


def _go_environment() -> dict[str, str]:
    environment = os.environ.copy()
    cache = Path("/job/cache")
    try:
        cache.mkdir(parents=True, exist_ok=True)
    except PermissionError:
        cache = _home() / ".cache" / "foundry-ci"
        cache.mkdir(parents=True, exist_ok=True)
    environment["GOPATH"] = str(cache / "go")
    environment["GOMODCACHE"] = str(cache / "go" / "pkg" / "mod")
    environment["GOCACHE"] = str(cache / "go-build")
    environment["GOBIN"] = str(_local_bin())
    return environment


def _download(url: str, destination: Path) -> None:
    log_stdout(f"Downloading {url}")
    urllib.request.urlretrieve(url, destination)


def _extract_binary(archive: Path, member_name: str, destination: Path) -> None:
    with tarfile.open(archive) as bundle:
        member = bundle.getmember(member_name)
        source = bundle.extractfile(member)
        if source is None:
            raise RuntimeError(f"Archive member is not a file: {member_name}")
        with destination.open("wb") as output:
            shutil.copyfileobj(source, output)
    destination.chmod(0o755)
    archive.unlink()


def _ensure_buildctl() -> Path:
    existing = shutil.which("buildctl")
    if existing:
        return Path(existing)
    buildkit_arch, _ = _machine_arch()
    archive = Path("/tmp") / f"buildkit-{BUILDKIT_VERSION}-{buildkit_arch}.tar.gz"
    _download(
        "https://github.com/moby/buildkit/releases/download/"
        f"v{BUILDKIT_VERSION}/buildkit-v{BUILDKIT_VERSION}.linux-{buildkit_arch}.tar.gz",
        archive,
    )
    binary = _local_bin() / "buildctl"
    _extract_binary(archive, "bin/buildctl", binary)
    return binary


def _ensure_helm() -> Path:
    existing = shutil.which("helm")
    if existing:
        return Path(existing)
    go_arch, _ = _machine_arch()
    archive = Path("/tmp") / f"helm-{HELM_VERSION}-{go_arch}.tar.gz"
    _download(
        f"https://get.helm.sh/helm-v{HELM_VERSION}-linux-{go_arch}.tar.gz",
        archive,
    )
    binary = _local_bin() / "helm"
    _extract_binary(archive, f"linux-{go_arch}/helm", binary)
    return binary


def _ensure_crane() -> Path:
    existing = shutil.which("crane")
    if existing:
        return Path(existing)
    _, archive_arch = _machine_arch()
    archive = Path("/tmp") / f"crane-{CRANE_VERSION}-{archive_arch}.tar.gz"
    _download(
        "https://github.com/google/go-containerregistry/releases/download/"
        f"v{CRANE_VERSION}/go-containerregistry_Linux_{archive_arch}.tar.gz",
        archive,
    )
    binary = _local_bin() / "crane"
    _extract_binary(archive, "crane", binary)
    return binary


def _ensure_gh() -> Path:
    existing = shutil.which("gh")
    if existing:
        return Path(existing)
    go_arch, _ = _machine_arch()
    directory = f"gh_{GHCLI_VERSION}_linux_{go_arch}"
    archive = Path("/tmp") / f"{directory}.tar.gz"
    _download(
        f"https://github.com/cli/cli/releases/download/v{GHCLI_VERSION}/"
        f"{directory}.tar.gz",
        archive,
    )
    binary = _local_bin() / "gh"
    _extract_binary(archive, f"{directory}/bin/gh", binary)
    return binary


def _ensure_semver_tags(root: Path) -> Path:
    binary = _local_bin() / "semver-tags"
    if binary.is_file():
        return binary
    _section(f"Installing semver-tags {SEMVER_TAGS_VERSION}")
    _run(
        [
            "go",
            "install",
            f"github.com/catalystcommunity/semver-tags@{SEMVER_TAGS_VERSION}",
        ],
        cwd=root,
        env=_go_environment(),
    )
    return binary


def _commit_records(root: Path) -> list[tuple[str, str]]:
    diff_base = os.environ.get("REACTORCIDE_DIFF_BASE", "").strip()
    revision = f"{diff_base}..HEAD" if diff_base else "HEAD"
    result = _run(
        ["git", "log", revision, "--pretty=format:%H%x00%s"],
        cwd=root,
        capture=True,
    )
    records = []
    for line in result.stdout.splitlines():
        commit_hash, separator, subject = line.partition("\0")
        if separator:
            records.append((commit_hash, subject))
    return records


def validate_conventional_commits(root: Path) -> None:
    _section("Validating conventional commits")
    failures = []
    for commit_hash, subject in _commit_records(root):
        if CONVENTIONAL_COMMIT_PATTERN.fullmatch(subject):
            log_stdout(f"OK: {subject}")
        else:
            log_stdout(f"FAIL: {subject} ({commit_hash})")
            failures.append(subject)
    if failures:
        raise RuntimeError(
            "Commit messages must match 'type(scope)?: description'."
        )


def test_go(root: Path) -> None:
    module = root / "v1"
    environment = _go_environment()
    environment["PYTHONDONTWRITEBYTECODE"] = "1"
    _section("Running Foundry CI plugin tests")
    _run(
        [
            "python3",
            "-m",
            "unittest",
            "discover",
            "-s",
            root / ".reactorcide" / "tests",
            "-p",
            "test_*.py",
            "-v",
        ],
        cwd=root,
        env=environment,
    )
    _section("Checking Go formatting")
    result = _run(["gofmt", "-l", "."], cwd=module, env=environment, capture=True)
    if result.stdout.strip():
        raise RuntimeError("These files are not gofmt-clean:\n" + result.stdout.strip())
    _section("Running Go vet")
    _run(["go", "vet", "./..."], cwd=module, env=environment)
    _section("Building Foundry")
    _run(["go", "build", "./..."], cwd=module, env=environment)
    _section("Running short Go tests")
    _run(["go", "test", "-short", "./..."], cwd=module, env=environment)


def validate_helm(root: Path) -> None:
    chart = root / "v1" / "charts" / "foundry-gateway-controller"
    helm = _ensure_helm()
    _section("Linting the Foundry gateway controller chart")
    _run([helm, "lint", chart], cwd=root)
    _section("Rendering the Foundry gateway controller chart")
    _run([helm, "template", chart], cwd=root, capture=True)


def test_image_build(root: Path) -> None:
    buildctl = _ensure_buildctl()
    _section("Waiting for the builder sidecar")
    for attempt in range(30):
        result = _run([buildctl, "debug", "info"], cwd=root, check=False, capture=True)
        if result.returncode == 0:
            break
        if attempt == 29:
            raise RuntimeError("The builder sidecar was not ready after 30 seconds")
        time.sleep(1)
    _section("Building the multi-architecture Foundry image without pushing it")
    _run(
        [
            buildctl,
            "build",
            "--frontend",
            "dockerfile.v0",
            "--local",
            "context=.",
            "--local",
            "dockerfile=.",
            "--opt",
            "platform=linux/amd64,linux/arm64",
            "--output",
            "type=image,name=foundry:build",
        ],
        cwd=root,
    )


def _required_environment(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(f"Required environment variable is not set: {name}")
    return value


def _git_auth_environment(root: Path) -> dict[str, str]:
    repository = _required_environment("REACTORCIDE_REPO")
    result = _run(["git", "remote", "get-url", "origin"], cwd=root, capture=True)
    remote = urllib.parse.urlsplit(result.stdout.strip())
    remote_repository = remote.path.strip("/").removesuffix(".git")
    if (
        remote.scheme != "https"
        or remote.hostname != "github.com"
        or remote.username is not None
        or remote_repository.lower() != repository.lower()
    ):
        raise RuntimeError("The origin remote must be the configured GitHub repository")

    askpass = Path("/tmp/foundry-git-askpass.py")
    askpass.write_text(
        """#!/usr/bin/env python3
import os
import sys

prompt = sys.argv[1].lower() if len(sys.argv) > 1 else ""
name = "FOUNDRY_GIT_USERNAME" if "username" in prompt else "FOUNDRY_GIT_PASSWORD"
sys.stdout.write(os.environ[name] + "\\n")
""",
        encoding="utf-8",
    )
    askpass.chmod(0o700)

    environment = _go_environment()
    environment["GIT_CONFIG_COUNT"] = "1"
    environment["GIT_CONFIG_KEY_0"] = "credential.helper"
    environment["GIT_CONFIG_VALUE_0"] = ""
    environment["GIT_ASKPASS"] = str(askpass)
    environment["GIT_TERMINAL_PROMPT"] = "0"
    environment["FOUNDRY_GIT_USERNAME"] = "x-access-token"
    environment["FOUNDRY_GIT_PASSWORD"] = _required_environment("GITHUB_PAT")
    return environment


def _prepare_release_repository(root: Path) -> dict[str, str]:
    environment = _git_auth_environment(root)
    _run(["git", "config", "user.name", "catalystcommunityci"], cwd=root)
    _run(["git", "config", "user.email", "ci@catalystcommunity.org"], cwd=root)
    _run(["git", "fetch", "--tags", "--force", "origin"], cwd=root, env=environment)
    return environment


def _release_metadata(root: Path, directory: str, environment: Mapping[str, str]) -> dict[str, str] | None:
    semver_tags = _ensure_semver_tags(root)
    result = _run(
        [
            semver_tags,
            "run",
            "--output_json",
            "--directories",
            directory,
        ],
        cwd=root,
        env=environment,
        capture=True,
    )
    for line in reversed(result.stdout.splitlines()):
        try:
            metadata = json.loads(line)
        except json.JSONDecodeError:
            continue
        if str(metadata.get("New_release_published", "")).lower() != "true":
            return None
        tag = str(metadata.get("New_release_git_tag", ""))
        if not tag:
            raise RuntimeError("semver-tags did not return a release tag")
        return {"tag": tag, "version": tag.rsplit("/", 1)[-1].removeprefix("v")}
    raise RuntimeError("semver-tags did not return release metadata")


def _commit_and_push_chart(root: Path, path: Path, message: str, environment: Mapping[str, str]) -> None:
    _run(["git", "add", path], cwd=root)
    staged = _run(["git", "diff", "--cached", "--quiet"], cwd=root, check=False, capture=True)
    if staged.returncode == 0:
        return
    _run(["git", "commit", "-m", message], cwd=root)
    _run(["git", "fetch", "origin", "main"], cwd=root, env=environment)
    _run(["git", "rebase", "origin/main"], cwd=root, env=environment)
    _run(["git", "push", "origin", "HEAD:main"], cwd=root, env=environment)


def _replace_yaml_scalar(path: Path, field: str, value: str) -> None:
    content = path.read_text(encoding="utf-8")
    updated, count = re.subn(
        rf"(?m)^{re.escape(field)}:.*$",
        f'{field}: "{value}"',
        content,
        count=1,
    )
    if count != 1:
        raise RuntimeError(f"Could not update {field} in {path}")
    path.write_text(updated, encoding="utf-8")


def _gh_environment() -> dict[str, str]:
    environment = os.environ.copy()
    environment["GH_TOKEN"] = _required_environment("GITHUB_PAT")
    return environment


def _release_build_environment(os_name: str, arch: str) -> dict[str, str]:
    environment = _go_environment()
    environment["CGO_ENABLED"] = "0"
    environment["GOOS"] = os_name
    environment["GOARCH"] = arch
    return environment


def _release_asset_name(version: str, os_name: str, arch: str) -> str:
    suffix = "zip" if os_name == "windows" else "tar.gz"
    return f"foundry-{version}-{os_name}-{arch}.{suffix}"


def _expected_release_asset_names(version: str) -> set[str]:
    return {
        _release_asset_name(version, os_name, arch)
        for os_name, arch in RELEASE_PLATFORMS
    }


def _cache_asset_name(os_name: str, arch: str) -> str:
    suffix = "zip" if os_name == "windows" else "tar.gz"
    return f"foundry-{os_name}-{arch}.{suffix}"


def _expected_cache_asset_names() -> set[str]:
    return {
        _cache_asset_name(os_name, arch)
        for os_name, arch in RELEASE_PLATFORMS
    }


def _put_presigned(url: str, content: bytes) -> None:
    request = urllib.request.Request(url, data=content, method="PUT")
    for attempt in range(5):
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                response.read()
            return
        except urllib.error.HTTPError as error:
            if error.code not in {408, 429, 500, 502, 503, 504} or attempt == 4:
                raise RuntimeError(
                    f"The exact-object asset upload failed with HTTP {error.code}"
                ) from None
        except (urllib.error.URLError, TimeoutError):
            if attempt == 4:
                raise RuntimeError("The exact-object asset upload failed") from None
        time.sleep(2**attempt)
    raise RuntimeError("The exact-object asset upload failed")


def _get_presigned(url: str) -> bytes:
    request = urllib.request.Request(url, method="GET")
    for attempt in range(5):
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                return response.read()
        except urllib.error.HTTPError as error:
            if error.code not in {408, 429, 500, 502, 503, 504} or attempt == 4:
                raise RuntimeError(
                    f"The exact-object asset download failed with HTTP {error.code}"
                ) from None
        except (urllib.error.URLError, TimeoutError):
            if attempt == 4:
                raise RuntimeError("The exact-object asset download failed") from None
        time.sleep(2**attempt)
    raise RuntimeError("The exact-object asset download failed")


def _verify_native_release_binary(
    root: Path,
    binary_path: Path,
    version: str,
    os_name: str,
    arch: str,
) -> None:
    host_os = platform.system().lower()
    host_arch, _ = _machine_arch()
    if (os_name, arch) != (host_os, host_arch):
        return

    result = _run([binary_path, "--version"], cwd=root, capture=True)
    expected = f"foundry version {version}"
    if result.stdout.strip() != expected:
        raise RuntimeError(
            f"The release binary did not report '{expected}': "
            f"{result.stdout.strip()}"
        )
    log_stdout(f"Verified {binary_path.name} --version reports {version}")


def _build_release_asset(
    root: Path,
    release_dir: Path,
    version: str,
    git_commit: str,
    build_date: str,
    os_name: str,
    arch: str,
) -> Path:
    """Build and package one Foundry release binary."""
    if (os_name, arch) not in RELEASE_PLATFORMS:
        raise RuntimeError(f"Unsupported release platform: {os_name}/{arch}")
    release_dir.mkdir(parents=True, exist_ok=True)
    module = root / "v1"
    ldflags = (
        f"-X main.Version={version} "
        f"-X main.GitCommit={git_commit} "
        f"-X main.BuildDate={build_date} -s -w"
    )
    binary_name = "foundry.exe" if os_name == "windows" else "foundry"
    binary_path = release_dir / binary_name
    _section(f"Building Foundry {version} for {os_name}/{arch}")
    _run(
        [
            "go",
            "build",
            "-buildvcs=false",
            "-trimpath",
            "-ldflags",
            ldflags,
            "-o",
            binary_path,
            "./cmd/foundry",
        ],
        cwd=module,
        env=_release_build_environment(os_name, arch),
    )
    _verify_native_release_binary(root, binary_path, version, os_name, arch)

    archive_path = release_dir / _release_asset_name(version, os_name, arch)
    if os_name == "windows":
        with zipfile.ZipFile(
            archive_path,
            "w",
            compression=zipfile.ZIP_DEFLATED,
        ) as archive:
            archive.write(binary_path, arcname=binary_name)
    else:
        with tarfile.open(
            archive_path,
            "w:gz",
            format=tarfile.USTAR_FORMAT,
        ) as archive:
            archive.add(binary_path, arcname=binary_name, recursive=False)
    binary_path.unlink()
    return archive_path


def _build_release_assets(
    root: Path,
    release_dir: Path,
    version: str,
    git_commit: str,
    build_date: str,
) -> list[Path]:
    """Build every asset. This helper supports local verification."""
    return [
        _build_release_asset(
            root,
            release_dir,
            version,
            git_commit,
            build_date,
            os_name,
            arch,
        )
        for os_name, arch in RELEASE_PLATFORMS
    ]


def _github_release(root: Path, tag: str) -> dict[str, object] | None:
    result = _run(
        [
            "gh",
            "release",
            "view",
            tag,
            "--repo",
            _required_environment("REACTORCIDE_REPO"),
            "--json",
            "tagName,isDraft,assets",
        ],
        cwd=root,
        env=_gh_environment(),
        capture=True,
        check=False,
    )
    if result.returncode == 0:
        try:
            release = json.loads(result.stdout)
        except json.JSONDecodeError as error:
            raise RuntimeError("gh returned invalid release metadata") from error
        if not isinstance(release, dict):
            raise RuntimeError("gh returned invalid release metadata")
        return release
    if "release not found" in result.stdout.lower() or "http 404" in result.stdout.lower():
        return None
    raise RuntimeError(f"Could not read GitHub release {tag}: {result.stdout.strip()}")


def _latest_server_tag(root: Path) -> str | None:
    result = _run(
        ["git", "tag", "--list", "v1/v*", "--sort=-version:refname"],
        cwd=root,
        capture=True,
    )
    return next((line.strip() for line in result.stdout.splitlines() if line.strip()), None)


def _recoverable_server_release(root: Path) -> dict[str, str] | None:
    tag = _latest_server_tag(root)
    if tag is None:
        return None
    release = _github_release(root, tag)
    if release is not None and not bool(release.get("isDraft")):
        return None

    tag_commit = _run(
        ["git", "rev-parse", f"{tag}^{{commit}}"],
        cwd=root,
        capture=True,
    ).stdout.strip()
    diff = _run(
        [
            "git",
            "diff",
            "--quiet",
            tag_commit,
            "HEAD",
            "--",
            "v1",
            "csil",
        ],
        cwd=root,
        capture=True,
        check=False,
    )
    if diff.returncode == 1:
        raise RuntimeError(
            f"Cannot recover {tag}: releasable source changed after the tag"
        )
    if diff.returncode != 0:
        raise RuntimeError(f"Could not compare {tag} with the current source")
    log_stdout(f"Recovering incomplete server release {tag}")
    return {"tag": tag, "version": tag.rsplit("/", 1)[-1].removeprefix("v")}


def _write_release_workflow_state(
    root: Path,
    metadata: Mapping[str, str] | None,
) -> None:
    if metadata is None:
        set_workflow_var(RELEASE_ACTIVE_VAR, "false")
        return
    tag = metadata["tag"]
    source_commit = metadata.get("source_commit") or _run(
        ["git", "rev-parse", f"{tag}^{{commit}}"],
        cwd=root,
        capture=True,
    ).stdout.strip()
    git_commit = _run(
        ["git", "rev-parse", "--short", f"{tag}^{{commit}}"],
        cwd=root,
        capture=True,
    ).stdout.strip()
    set_workflow_var(RELEASE_ACTIVE_VAR, "true")
    set_workflow_var(RELEASE_TAG_VAR, tag)
    set_workflow_var(RELEASE_VERSION_VAR, metadata["version"])
    set_workflow_var(RELEASE_COMMIT_VAR, git_commit)
    set_workflow_var(RELEASE_SOURCE_VAR, source_commit)
    set_workflow_var(
        RELEASE_DATE_VAR,
        time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    )


def _active_release_metadata() -> dict[str, str] | None:
    values = workflow_vars()
    if str(values.get(RELEASE_ACTIVE_VAR, "")).lower() != "true":
        log_stdout("No server release work is required")
        return None
    metadata = {
        "tag": str(values.get(RELEASE_TAG_VAR, "")),
        "version": str(values.get(RELEASE_VERSION_VAR, "")),
        "git_commit": str(values.get(RELEASE_COMMIT_VAR, "")),
        "source_commit": str(values.get(RELEASE_SOURCE_VAR, "")),
        "build_date": str(values.get(RELEASE_DATE_VAR, "")),
    }
    missing = [name for name, value in metadata.items() if not value]
    if missing:
        raise RuntimeError(
            "Release workflow state is missing: " + ", ".join(sorted(missing))
        )
    return metadata


def _cache_manifest_is_reusable(
    cache: object,
    lane: str,
    source_commit: str,
) -> bool:
    try:
        content = cache.get_bytes(ASSET_CACHE.object_key(lane, ASSET_CACHE.MANIFEST))
    except FileNotFoundError:
        return False

    try:
        manifest = ASSET_CACHE.decode_manifest(content)
    except (
        RuntimeError,
        UnicodeDecodeError,
        ValueError,
        json.JSONDecodeError,
    ):
        return False

    assets = manifest.get("assets")
    if manifest.get("source_commit") != source_commit or not isinstance(assets, list):
        return False
    by_name = {
        item.get("name"): item
        for item in assets
        if isinstance(item, dict)
    }
    if set(by_name) != _expected_cache_asset_names():
        return False
    for name, item in by_name.items():
        try:
            payload = cache.get_bytes(ASSET_CACHE.object_key(lane, str(name)))
            recorded = cache.get_bytes(
                ASSET_CACHE.object_key(lane, str(name) + ".sha256")
            ).decode().strip()
        except FileNotFoundError:
            return False
        except UnicodeDecodeError:
            return False
        digest = hashlib.sha256(payload).hexdigest()
        if (
            recorded != digest
            or item.get("sha256") != digest
            or item.get("size") != len(payload)
        ):
            return False
    return True


def _prepare_release_cache(metadata: Mapping[str, str]) -> None:
    lane = ASSET_CACHE.version_lane(metadata["version"])
    cache = ASSET_CACHE.S3Cache.from_environment()
    reusable = _cache_manifest_is_reusable(
        cache,
        lane,
        metadata["source_commit"],
    )
    uploads: dict[str, dict[str, str]] = {}
    if not reusable:
        for asset in sorted(_expected_cache_asset_names()):
            staging = "staging-" + asset
            uploads[asset] = {
                "asset": cache.presign(
                    "PUT",
                    ASSET_CACHE.object_key(lane, staging),
                ),
                "sha256": cache.presign(
                    "PUT",
                    ASSET_CACHE.object_key(lane, staging + ".sha256"),
                ),
            }
    downloads = {
        asset: cache.presign("GET", ASSET_CACHE.object_key(lane, asset))
        for asset in sorted(_expected_cache_asset_names())
    }
    set_workflow_var(
        RELEASE_CACHE_VAR,
        {
            "lane": lane,
            "uploads": uploads,
            "downloads": downloads,
            "manifest": cache.presign(
                "GET",
                ASSET_CACHE.object_key(lane, ASSET_CACHE.MANIFEST),
            ),
        },
    )
    action = "Reusing" if reusable else "Prepared"
    log_stdout(f"{action} Foundry release asset lane {lane}")


def _release_cache_state() -> dict[str, object]:
    state = workflow_vars().get(RELEASE_CACHE_VAR)
    if not isinstance(state, dict):
        raise RuntimeError("The release asset-cache state is missing")
    lane = state.get("lane")
    uploads = state.get("uploads")
    downloads = state.get("downloads")
    manifest = state.get("manifest")
    if (
        not isinstance(lane, str)
        or not isinstance(uploads, dict)
        or not isinstance(downloads, dict)
        or not isinstance(manifest, str)
    ):
        raise RuntimeError("The release asset-cache state is invalid")
    return state


def release_server_prepare(root: Path) -> None:
    environment = _prepare_release_repository(root)
    _ensure_gh()
    metadata = _release_metadata(root, "v1", environment)
    if metadata is None:
        metadata = _recoverable_server_release(root)
    if metadata is None:
        _write_release_workflow_state(root, None)
        log_stdout("No new or incomplete server release is present")
        return

    tag = metadata["tag"]
    release = _github_release(root, tag)
    if release is None:
        _section(f"Creating draft GitHub release {tag}")
        _run(
            [
                "gh",
                "release",
                "create",
                tag,
                "--repo",
                _required_environment("REACTORCIDE_REPO"),
                "--title",
                tag,
                "--generate-notes",
                "--draft",
            ],
            cwd=root,
            env=_gh_environment(),
        )
    elif not bool(release.get("isDraft")):
        _write_release_workflow_state(root, None)
        log_stdout(f"GitHub release {tag} is already published")
        return
    else:
        log_stdout(f"Using existing draft GitHub release {tag}")
    metadata = dict(metadata)
    metadata["source_commit"] = _run(
        ["git", "rev-parse", f"{tag}^{{commit}}"],
        cwd=root,
        capture=True,
    ).stdout.strip()
    _prepare_release_cache(metadata)
    _write_release_workflow_state(root, metadata)


def _release_platform() -> tuple[str, str]:
    value = _required_environment("FOUNDRY_RELEASE_PLATFORM")
    try:
        os_name, arch = value.split("/", 1)
    except ValueError as error:
        raise RuntimeError(f"Unsupported release platform: {value}") from error
    if (os_name, arch) not in RELEASE_PLATFORMS:
        raise RuntimeError(f"Unsupported release platform: {value}")
    return os_name, arch


def release_server_cli(root: Path) -> None:
    metadata = _active_release_metadata()
    if metadata is None:
        return
    os_name, arch = _release_platform()
    state = _release_cache_state()
    uploads = state["uploads"]
    if not isinstance(uploads, dict):
        raise RuntimeError("The release asset upload map is invalid")
    cache_name = _cache_asset_name(os_name, arch)
    upload = uploads.get(cache_name)
    if upload is None:
        log_stdout(f"Reusing cached release asset {cache_name}")
        return
    if not isinstance(upload, dict):
        raise RuntimeError("The release asset upload entry is invalid")
    asset_url = upload.get("asset")
    digest_url = upload.get("sha256")
    if not isinstance(asset_url, str) or not isinstance(digest_url, str):
        raise RuntimeError("The release asset upload URLs are invalid")
    with tempfile.TemporaryDirectory(prefix="foundry-release-") as directory:
        asset = _build_release_asset(
            root,
            Path(directory),
            metadata["version"],
            metadata["git_commit"],
            metadata["build_date"],
            os_name,
            arch,
        )
        digest = ASSET_CACHE.file_sha256(asset)
        _put_presigned(asset_url, asset.read_bytes())
        _put_presigned(digest_url, (digest + "\n").encode())
        log_stdout(f"Built and cached release asset {cache_name}")


def release_server_seal(root: Path) -> None:
    metadata = _active_release_metadata()
    if metadata is None:
        return
    state = _release_cache_state()
    lane = str(state["lane"])
    uploads = state["uploads"]
    if not isinstance(uploads, dict):
        raise RuntimeError("The release asset upload map is invalid")
    cache = ASSET_CACHE.S3Cache.from_environment()
    assets = []
    for asset in sorted(_expected_cache_asset_names()):
        if asset in uploads:
            staging = "staging-" + asset
            staging_key = ASSET_CACHE.object_key(lane, staging)
            staging_digest_key = ASSET_CACHE.object_key(
                lane,
                staging + ".sha256",
            )
            content = cache.get_bytes(staging_key)
            recorded = cache.get_bytes(staging_digest_key).decode().strip()
        else:
            content = cache.get_bytes(ASSET_CACHE.object_key(lane, asset))
            recorded = cache.get_bytes(
                ASSET_CACHE.object_key(lane, asset + ".sha256")
            ).decode().strip()
        digest = hashlib.sha256(content).hexdigest()
        if recorded != digest:
            raise RuntimeError(f"The cached asset checksum does not match: {asset}")
        if asset in uploads:
            final_key = ASSET_CACHE.object_key(lane, asset)
            cache.copy(staging_key, final_key)
            copied = cache.get_bytes(final_key)
            if hashlib.sha256(copied).hexdigest() != digest or len(copied) != len(content):
                raise RuntimeError(f"The sealed asset copy is invalid: {asset}")
            cache.put_bytes(
                ASSET_CACHE.object_key(lane, asset + ".sha256"),
                (digest + "\n").encode(),
            )
            cache.delete(staging_key)
            cache.delete(staging_digest_key)
        assets.append({"name": asset, "sha256": digest, "size": len(content)})
    manifest = {
        "schema": 1,
        "project": ASSET_CACHE.PROJECT,
        "lane": lane,
        "tag": metadata["tag"],
        "version": metadata["version"],
        "source_commit": metadata["source_commit"],
        "assets": assets,
    }
    cache.put_bytes(
        ASSET_CACHE.object_key(lane, ASSET_CACHE.MANIFEST),
        ASSET_CACHE.encode_manifest(manifest),
    )
    log_stdout(f"Sealed Foundry release asset lane {lane}")


def _download_cached_release_assets(
    destination: Path,
    metadata: Mapping[str, str],
    platforms: Sequence[tuple[str, str]],
) -> dict[tuple[str, str], Path]:
    state = _release_cache_state()
    manifest_url = state["manifest"]
    downloads = state["downloads"]
    if not isinstance(manifest_url, str) or not isinstance(downloads, dict):
        raise RuntimeError("The release asset download state is invalid")
    try:
        manifest = ASSET_CACHE.decode_manifest(_get_presigned(manifest_url))
    except (json.JSONDecodeError, RuntimeError) as error:
        raise RuntimeError("The release asset manifest is invalid") from error
    assets = manifest.get("assets")
    if (
        manifest.get("project") != ASSET_CACHE.PROJECT
        or manifest.get("tag") != metadata["tag"]
        or manifest.get("version") != metadata["version"]
        or manifest.get("source_commit") != metadata["source_commit"]
        or not isinstance(assets, list)
    ):
        raise RuntimeError("The release asset manifest does not match the release")
    by_name = {
        item.get("name"): item
        for item in assets
        if isinstance(item, dict)
    }
    if set(by_name) != _expected_cache_asset_names():
        raise RuntimeError("The release asset manifest has unexpected assets")

    destination.mkdir(parents=True, exist_ok=True)
    downloaded: dict[tuple[str, str], Path] = {}
    for os_name, arch in platforms:
        cache_name = _cache_asset_name(os_name, arch)
        url = downloads.get(cache_name)
        if not isinstance(url, str):
            raise RuntimeError(f"The release asset URL is missing: {cache_name}")
        content = _get_presigned(url)
        item = by_name[cache_name]
        if (
            item.get("sha256") != hashlib.sha256(content).hexdigest()
            or item.get("size") != len(content)
        ):
            raise RuntimeError(f"The downloaded release asset is invalid: {cache_name}")
        path = destination / _release_asset_name(
            metadata["version"],
            os_name,
            arch,
        )
        path.write_bytes(content)
        downloaded[(os_name, arch)] = path
    return downloaded


def _extract_release_binary(
    archive_path: Path,
    destination: Path,
    os_name: str,
) -> None:
    binary_name = "foundry.exe" if os_name == "windows" else "foundry"
    if os_name == "windows":
        with zipfile.ZipFile(archive_path) as archive:
            if archive.namelist() != [binary_name]:
                raise RuntimeError(f"The release archive is invalid: {archive_path.name}")
            content = archive.read(binary_name)
    else:
        with tarfile.open(archive_path, "r:gz") as archive:
            if archive.getnames() != [binary_name]:
                raise RuntimeError(f"The release archive is invalid: {archive_path.name}")
            member = archive.extractfile(binary_name)
            if member is None:
                raise RuntimeError(f"The release archive is invalid: {archive_path.name}")
            content = member.read()
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(content)
    destination.chmod(0o755)


def _registry_environment(registry: str) -> dict[str, str]:
    environment = os.environ.copy()
    docker_config = _home() / ".docker"
    docker_config.mkdir(parents=True, exist_ok=True)
    credentials = (
        f"{_required_environment('REGISTRY_USER')}:"
        f"{_required_environment('REGISTRY_PASSWORD')}"
    )
    config_path = docker_config / "config.json"
    config_path.write_text(
        json.dumps(
            {
                "auths": {
                    registry: {
                        "auth": base64.b64encode(credentials.encode()).decode()
                    }
                }
            }
        ),
        encoding="utf-8",
    )
    config_path.chmod(0o600)
    environment["DOCKER_CONFIG"] = str(docker_config)
    return environment


def _wait_for_buildkit(buildctl: Path, root: Path, environment: Mapping[str, str]) -> None:
    _section("Waiting for the builder sidecar")
    for attempt in range(30):
        result = _run(
            [buildctl, "debug", "info"],
            cwd=root,
            env=environment,
            check=False,
            capture=True,
        )
        if result.returncode == 0:
            return
        if attempt == 29:
            raise RuntimeError("The builder sidecar was not ready after 30 seconds")
        time.sleep(1)


def _verify_multiarch_image(
    crane: Path,
    root: Path,
    image: str,
    environment: Mapping[str, str],
) -> None:
    result = _run([crane, "manifest", image], cwd=root, env=environment, capture=True)
    try:
        manifest = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise RuntimeError(f"Registry returned an invalid manifest for {image}") from error
    if not isinstance(manifest, dict):
        raise RuntimeError(f"Registry returned an invalid manifest for {image}")
    manifests = manifest.get("manifests")
    if not isinstance(manifests, list):
        raise RuntimeError(f"Registry returned an invalid image index for {image}")
    platforms = {
        (
            str(item.get("platform", {}).get("os", "")),
            str(item.get("platform", {}).get("architecture", "")),
        )
        for item in manifests
        if isinstance(item, dict) and isinstance(item.get("platform"), dict)
    }
    required = {("linux", "amd64"), ("linux", "arm64")}
    if not required.issubset(platforms):
        raise RuntimeError(
            f"Image {image} does not contain linux/amd64 and linux/arm64"
        )
    log_stdout(f"Verified multi-architecture image {image}")


def release_server_image(root: Path) -> None:
    metadata = _active_release_metadata()
    if metadata is None:
        return
    registry = _required_environment("REGISTRY")
    image_path = _required_environment("IMAGE_PATH")
    image = f"{registry}/{image_path}"
    versioned_image = f"{image}:{metadata['version']}"
    environment = _registry_environment(registry)
    buildctl = _ensure_buildctl()
    crane = _ensure_crane()
    _wait_for_buildkit(buildctl, root, environment)

    with tempfile.TemporaryDirectory(prefix="foundry-release-image-") as directory:
        workspace = Path(directory)
        context = workspace / "context"
        archives = _download_cached_release_assets(
            workspace / "archives",
            metadata,
            (("linux", "amd64"), ("linux", "arm64")),
        )
        for target, archive in archives.items():
            os_name, arch = target
            _extract_release_binary(
                archive,
                context / f"{os_name}-{arch}" / "foundry",
                os_name,
            )

        _section(f"Publishing multi-architecture image {versioned_image}")
        _run(
            [
                buildctl,
                "build",
                "--frontend",
                "dockerfile.v0",
                "--local",
                f"context={context}",
                "--local",
                "dockerfile=.",
                "--opt",
                "filename=Dockerfile.release",
                "--opt",
                "platform=linux/amd64,linux/arm64",
                "--output",
                f'type=image,"name={versioned_image},{image}:latest",push=true',
            ],
            cwd=root,
            env=environment,
        )
    _verify_multiarch_image(crane, root, versioned_image, environment)


def release_server_publish(root: Path) -> None:
    metadata = _active_release_metadata()
    if metadata is None:
        return
    _ensure_gh()
    release = _github_release(root, metadata["tag"])
    if release is None:
        raise RuntimeError(f"Draft GitHub release {metadata['tag']} does not exist")
    with tempfile.TemporaryDirectory(prefix="foundry-release-publish-") as directory:
        assets = _download_cached_release_assets(
            Path(directory),
            metadata,
            RELEASE_PLATFORMS,
        )
        _section(f"Uploading six assets to GitHub release {metadata['tag']}")
        _run(
            [
                "gh",
                "release",
                "upload",
                metadata["tag"],
                *[assets[target] for target in RELEASE_PLATFORMS],
                "--repo",
                _required_environment("REACTORCIDE_REPO"),
                "--clobber",
            ],
            cwd=root,
            env=_gh_environment(),
        )

    release = _github_release(root, metadata["tag"])
    if release is None:
        raise RuntimeError(f"Draft GitHub release {metadata['tag']} does not exist")
    release_assets = release.get("assets")
    if not isinstance(release_assets, list):
        raise RuntimeError("GitHub returned invalid release asset metadata")
    actual_names = {
        str(asset.get("name", ""))
        for asset in release_assets
        if isinstance(asset, dict)
    }
    missing = sorted(_expected_release_asset_names(metadata["version"]) - actual_names)
    if missing:
        raise RuntimeError("Release assets are missing: " + ", ".join(missing))

    environment = _prepare_release_repository(root)
    chart = root / "v1" / "charts" / "foundry-gateway-controller" / "Chart.yaml"
    _replace_yaml_scalar(chart, "appVersion", metadata["version"])
    _commit_and_push_chart(
        root,
        chart,
        f"ci: bump foundry appVersion to {metadata['version']}",
        environment,
    )
    if bool(release.get("isDraft")):
        _run(
            [
                "gh",
                "release",
                "edit",
                metadata["tag"],
                "--repo",
                _required_environment("REACTORCIDE_REPO"),
                "--draft=false",
            ],
            cwd=root,
            env=_gh_environment(),
        )
    log_stdout(f"Published server release {metadata['tag']}")


def release_helm(root: Path) -> None:
    environment = _prepare_release_repository(root)
    chart_relative = Path("v1/charts/foundry-gateway-controller")
    metadata = _release_metadata(root, str(chart_relative), environment)
    if metadata is None:
        log_stdout("No new chart release is required")
        return

    tag = metadata["tag"]
    version = metadata["version"]
    chart_file = root / chart_relative / "Chart.yaml"
    _replace_yaml_scalar(chart_file, "version", version)
    _commit_and_push_chart(
        root,
        chart_file,
        f"ci: bump chart version to {version}",
        environment,
    )

    helm = _ensure_helm()
    _ensure_gh()
    _section(f"Packaging chart {version}")
    _run([helm, "package", root / chart_relative], cwd=root)
    package = root / f"foundry-gateway-controller-{version}.tgz"
    _run(
        [
            "gh",
            "release",
            "create",
            tag,
            "--repo",
            _required_environment("REACTORCIDE_REPO"),
            "--title",
            tag,
            "--notes",
            f"Helm chart {version}",
            package,
        ],
        cwd=root,
        env=_gh_environment(),
    )
    log_stdout(f"Released chart {tag}")


CI_JOBS: dict[str, Callable[[Path], None]] = {
    "conventional-commits": validate_conventional_commits,
    "helm-validate": validate_helm,
    "image-build-test": test_image_build,
    "test-go": test_go,
}

RELEASE_JOBS: dict[str, Callable[[Path], None]] = {
    "cli": release_server_cli,
    "helm": release_helm,
    "image": release_server_image,
    "prepare": release_server_prepare,
    "publish": release_server_publish,
    "seal": release_server_seal,
}


class FoundryCIJobsPlugin(Plugin):
    """Run one selected Foundry CI job after source preparation."""

    def __init__(self):
        super().__init__(name="foundry_ci_jobs", priority=50)

    def supported_phases(self):
        return [PluginPhase.POST_SOURCE_PREP]

    def execute(self, context: PluginContext) -> None:
        if context.phase != PluginPhase.POST_SOURCE_PREP:
            return
        job_name = os.environ.get("FOUNDRY_CI_JOB", "").strip()
        if not job_name:
            return
        job = CI_JOBS.get(job_name)
        if job is None:
            raise RuntimeError(
                f"Unknown FOUNDRY_CI_JOB '{job_name}'. Valid jobs: "
                + ", ".join(sorted(CI_JOBS))
            )
        root = _repo_root(context)
        log_stdout(f"Starting Foundry CI lifecycle job: {job_name}")
        job(root)
        log_stdout(f"Completed Foundry CI lifecycle job: {job_name}")


class FoundryReleaseJobsPlugin(Plugin):
    """Run one selected Foundry release job after source preparation."""

    def __init__(self):
        super().__init__(name="foundry_release_jobs", priority=60)

    def supported_phases(self):
        return [PluginPhase.POST_SOURCE_PREP]

    def execute(self, context: PluginContext) -> None:
        if context.phase != PluginPhase.POST_SOURCE_PREP:
            return
        job_name = os.environ.get("FOUNDRY_RELEASE_JOB", "").strip()
        if not job_name:
            return
        job = RELEASE_JOBS.get(job_name)
        if job is None:
            raise RuntimeError(
                f"Unknown FOUNDRY_RELEASE_JOB '{job_name}'. Valid jobs: "
                + ", ".join(sorted(RELEASE_JOBS))
            )
        root = _repo_root(context)
        log_stdout(f"Starting Foundry release lifecycle job: {job_name}")
        job(root)
        log_stdout(f"Completed Foundry release lifecycle job: {job_name}")
