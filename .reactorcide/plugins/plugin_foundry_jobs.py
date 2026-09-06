"""Runnerlib lifecycle jobs for Foundry CI and releases."""

from __future__ import annotations

import base64
import json
import os
import platform
import re
import shlex
import shutil
import subprocess
import tarfile
import tempfile
import time
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path
from typing import Callable, Mapping, Sequence

from src.logging import log_stdout
from src.plugins import Plugin, PluginContext, PluginPhase


CONVENTIONAL_COMMIT_PATTERN = re.compile(
    r"^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|norelease)"
    r"(\(.+\))?!?: .+"
)

BUILDKIT_VERSION = "0.17.3"
CRANE_VERSION = "0.20.3"
DOCKER_VERSION = "27.5.1"
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

    completed = subprocess.run(
        command,
        cwd=cwd,
        env=command_env,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if not capture or completed.returncode != 0:
        for line in (completed.stdout or "").splitlines():
            log_stdout(f"  {line}")
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


def _ensure_docker() -> Path:
    existing = shutil.which("docker")
    if existing:
        return Path(existing)
    go_arch, _ = _machine_arch()
    archive_arch = {"amd64": "x86_64", "arm64": "aarch64"}[go_arch]
    archive = Path("/tmp") / f"docker-{DOCKER_VERSION}-{archive_arch}.tar.gz"
    _download(
        f"https://download.docker.com/linux/static/stable/{archive_arch}/"
        f"docker-{DOCKER_VERSION}.tgz",
        archive,
    )
    binary = _local_bin() / "docker"
    _extract_binary(archive, "docker/docker", binary)
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
    _section("Building the Foundry image without pushing it")
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


def _build_release_assets(
    root: Path,
    release_dir: Path,
    version: str,
    git_commit: str,
    build_date: str,
) -> list[Path]:
    """Build and package every Foundry release binary."""
    release_dir.mkdir(parents=True, exist_ok=True)
    module = root / "v1"
    ldflags = (
        f"-X main.Version={version} "
        f"-X main.GitCommit={git_commit} "
        f"-X main.BuildDate={build_date} -s -w"
    )
    assets = []

    for os_name, arch in RELEASE_PLATFORMS:
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
        _verify_native_release_binary(
            root,
            binary_path,
            version,
            os_name,
            arch,
        )

        archive_stem = f"foundry-{version}-{os_name}-{arch}"
        if os_name == "windows":
            archive_path = release_dir / f"{archive_stem}.zip"
            with zipfile.ZipFile(
                archive_path,
                "w",
                compression=zipfile.ZIP_DEFLATED,
            ) as archive:
                archive.write(binary_path, arcname=binary_name)
        else:
            archive_path = release_dir / f"{archive_stem}.tar.gz"
            with tarfile.open(
                archive_path,
                "w:gz",
                format=tarfile.USTAR_FORMAT,
            ) as archive:
                archive.add(binary_path, arcname=binary_name, recursive=False)
        assets.append(archive_path)
        binary_path.unlink()

    return assets


def _create_server_github_release(root: Path, tag: str, assets: Sequence[Path]) -> None:
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
            *assets,
        ],
        cwd=root,
        env=_gh_environment(),
    )


def release_server(root: Path) -> None:
    environment = _prepare_release_repository(root)
    metadata = _release_metadata(root, "v1", environment)
    if metadata is None:
        log_stdout("No new server release is required")
        return

    tag = metadata["tag"]
    version = metadata["version"]
    registry = _required_environment("REGISTRY")
    image_path = _required_environment("IMAGE_PATH")
    image = f"{registry}/{image_path}"
    docker = _ensure_docker()
    crane = _ensure_crane()
    _ensure_gh()

    git_commit = _run(
        ["git", "rev-parse", "--short", "HEAD"],
        cwd=root,
        capture=True,
    ).stdout.strip()
    build_date = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

    if not os.environ.get("DOCKER_HOST"):
        raise RuntimeError("DOCKER_HOST is not set; this job needs the docker capability")

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

    _section("Waiting for the Docker sidecar")
    for attempt in range(30):
        result = _run([docker, "info"], cwd=root, check=False, capture=True)
        if result.returncode == 0:
            break
        if attempt == 29:
            raise RuntimeError("The Docker sidecar was not ready after 30 seconds")
        time.sleep(1)

    archive = Path("/tmp/foundry-image.tar")
    _section(f"Building and publishing {image}:{version}")
    _run(
        [
            docker,
            "build",
            "--build-arg",
            f"VERSION={version}",
            "--build-arg",
            f"GIT_COMMIT={git_commit}",
            "--build-arg",
            f"BUILD_DATE={build_date}",
            "-t",
            f"{image}:{version}",
            ".",
        ],
        cwd=root,
    )
    _run([docker, "save", f"{image}:{version}", "-o", archive], cwd=root)
    _run([crane, "push", archive, f"{image}:{version}"], cwd=root)
    _run([crane, "push", archive, f"{image}:latest"], cwd=root)
    archive.unlink(missing_ok=True)

    with tempfile.TemporaryDirectory(prefix="foundry-release-") as directory:
        assets = _build_release_assets(
            root,
            Path(directory),
            version,
            git_commit,
            build_date,
        )

        chart = root / "v1" / "charts" / "foundry-gateway-controller" / "Chart.yaml"
        _replace_yaml_scalar(chart, "appVersion", version)
        _commit_and_push_chart(
            root,
            chart,
            f"ci: bump foundry appVersion to {version}",
            environment,
        )

        _create_server_github_release(root, tag, assets)
    log_stdout(f"Released {tag} as {image}:{version}")


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
    "helm": release_helm,
    "server": release_server,
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
