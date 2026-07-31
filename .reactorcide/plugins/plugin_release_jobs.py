"""Runnerlib lifecycle jobs for the semver-tags release workflow.

The release runs in two stages. A merge to main runs the `tag` job, which
creates a draft GitHub release that records the source commit and then pushes
the semver tag. The pushed tag starts the release workflow, which runs
`prepare` to prove that CI made the draft, builds one archive for each
platform, and finally runs `publish`.

The draft is the authorization record. A tag that a person pushes by hand has
no draft, so `prepare` fails and nothing is published.
"""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import tarfile
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path
from typing import Any, Dict, List, Optional

from src.logging import log_stdout
from src.plugins import Plugin, PluginContext, PluginPhase


GITHUB_API_VERSION = "2022-11-28"
REPOSITORY_PATTERN = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")
PLATFORM_PATTERN = re.compile(r"^(linux|darwin|windows)/(amd64|arm64)$")
RELEASE_TAG_PATTERN = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$")
RELEASE_MARKER_PREFIX = "<!-- semver-tags-release-source:"
PLATFORMS = (
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
)
# Other repositories download this asset name to install the tool, so the
# release keeps it next to the platform archives.
LEGACY_ASSET_NAME = "semver-tags.tar.gz"
LEGACY_PLATFORM = ("linux", "amd64")
SECRET_ENVIRONMENT_NAMES = ("GITHUB_PAT",)
BUILD_DIR = Path("/tmp/semver-tags-release")


def _sanitized_environment() -> Dict[str, str]:
    environment = os.environ.copy()
    for name in SECRET_ENVIRONMENT_NAMES:
        environment.pop(name, None)
    return environment


def _run(
    command: List[str],
    *,
    cwd: Path,
    env: Optional[Dict[str, str]] = None,
    capture_output: bool = False,
) -> subprocess.CompletedProcess:
    """Run one command without a command shell."""
    log_stdout(f"Running: {shlex.join(command)}")
    command_environment = env if env is not None else _sanitized_environment()
    return subprocess.run(
        command,
        cwd=cwd,
        env=command_environment,
        check=True,
        text=True,
        capture_output=capture_output,
    )


def _masked(text: str) -> str:
    """Replace secret values in captured output."""
    for name in SECRET_ENVIRONMENT_NAMES:
        value = os.environ.get(name, "")
        if value:
            text = text.replace(value, "***")
    return text


def _semver_failure(error: subprocess.CalledProcessError) -> RuntimeError:
    """Return a safe error with the captured semver-tags output."""
    output = "\n".join(
        part.strip()
        for part in (error.stdout or "", error.stderr or "")
        if part.strip()
    )
    output = _masked(output)
    detail = output[-4000:] if output else str(error)
    return RuntimeError(f"semver-tags failed: {detail}")


def _required_environment(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def _release_repository() -> str:
    repository = _required_environment("REACTORCIDE_REPO")
    if not REPOSITORY_PATTERN.fullmatch(repository):
        raise RuntimeError("REACTORCIDE_REPO must use the OWNER/REPOSITORY format")
    return repository


def _go_environment(
    os_name: Optional[str] = None,
    arch: Optional[str] = None,
) -> Dict[str, str]:
    environment = _sanitized_environment()
    home = Path(environment.get("HOME", "/home/runner"))
    environment["GOPATH"] = str(home / ".cache" / "go")
    environment["GOCACHE"] = str(home / ".cache" / "go-build")
    environment["GOMODCACHE"] = str(home / ".cache" / "go-mod")
    if os_name:
        environment["GOOS"] = os_name
    if arch:
        environment["GOARCH"] = arch
    environment["CGO_ENABLED"] = "0"
    return environment


def _source_sha(code_dir: Path) -> str:
    result = _run(["git", "rev-parse", "HEAD"], cwd=code_dir, capture_output=True)
    sha = result.stdout.strip()
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise RuntimeError("The checked-out source does not have a valid Git SHA")
    return sha


def _release_marker(source_sha: str) -> str:
    return f"{RELEASE_MARKER_PREFIX}{source_sha} -->"


def _github_request(
    method: str,
    path_or_url: str,
    *,
    payload: Optional[Any] = None,
    content_type: str = "application/vnd.github+json",
) -> Any:
    token = _required_environment("GITHUB_PAT")
    if path_or_url.startswith("https://"):
        url = path_or_url
    else:
        url = f"https://api.github.com{path_or_url}"

    data = None
    if payload is not None:
        if isinstance(payload, bytes):
            data = payload
        else:
            data = json.dumps(payload).encode("utf-8")

    request = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "Content-Type": content_type,
            "User-Agent": "semver-tags-release",
            "X-GitHub-Api-Version": GITHUB_API_VERSION,
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            body = response.read()
    except urllib.error.HTTPError as error:
        raise RuntimeError(
            f"GitHub API request failed with status {error.code}: "
            f"{method} {urllib.parse.urlsplit(url).path}"
        ) from error

    if not body:
        return None
    return json.loads(body)


def _find_draft_release(repository: str, source_sha: str) -> Optional[Dict[str, Any]]:
    marker = _release_marker(source_sha)
    encoded_repository = urllib.parse.quote(repository, safe="/")
    for page in range(1, 6):
        releases = _github_request(
            "GET",
            f"/repos/{encoded_repository}/releases?per_page=100&page={page}",
        )
        if not isinstance(releases, list):
            raise RuntimeError("GitHub returned an invalid release list")
        for release in releases:
            if release.get("draft") and marker in (release.get("body") or ""):
                return release
        if len(releases) < 100:
            break
    return None


def _release_for_source(code_dir: Path) -> Optional[Dict[str, Any]]:
    repository = _release_repository()
    source_sha = _source_sha(code_dir)
    return _find_draft_release(repository, source_sha)


def _build_semver_tags(code_dir: Path) -> Path:
    """Build the release tool from the source it is releasing."""
    binary = BUILD_DIR / "semver-tags"
    binary.parent.mkdir(parents=True, exist_ok=True)
    _run(
        ["go", "build", "-buildvcs=false", "-trimpath", "-o", str(binary), "."],
        cwd=code_dir,
        env=_go_environment(),
    )
    return binary


def _git_push_environment(code_dir: Path) -> Dict[str, str]:
    """Build an environment that lets Git authenticate without exposing a token."""
    repository = _release_repository()
    result = _run(
        ["git", "remote", "get-url", "origin"],
        cwd=code_dir,
        capture_output=True,
    )
    remote = urllib.parse.urlsplit(result.stdout.strip())
    remote_repository = remote.path.strip("/")
    if remote_repository.endswith(".git"):
        remote_repository = remote_repository[:-4]
    if (
        remote.scheme != "https"
        or remote.hostname != "github.com"
        or remote.username is not None
        or remote_repository.lower() != repository.lower()
    ):
        raise RuntimeError("The origin remote must be the configured GitHub repository")

    askpass = BUILD_DIR / "git-askpass.py"
    askpass.parent.mkdir(parents=True, exist_ok=True)
    askpass.write_text(
        """#!/usr/bin/env python3
import os
import sys

prompt = sys.argv[1].lower() if len(sys.argv) > 1 else ""
name = (
    "SEMVER_TAGS_GIT_USERNAME"
    if "username" in prompt
    else "SEMVER_TAGS_GIT_PASSWORD"
)
sys.stdout.write(os.environ[name] + "\\n")
""",
        encoding="utf-8",
    )
    askpass.chmod(0o700)

    environment = _go_environment()
    environment.pop("GIT_CONFIG_GLOBAL", None)
    environment["GIT_CONFIG_COUNT"] = "1"
    environment["GIT_CONFIG_KEY_0"] = "credential.helper"
    environment["GIT_CONFIG_VALUE_0"] = ""
    environment["GIT_ASKPASS"] = str(askpass)
    environment["GIT_TERMINAL_PROMPT"] = "0"
    environment["SEMVER_TAGS_GIT_USERNAME"] = "x-access-token"
    environment["SEMVER_TAGS_GIT_PASSWORD"] = _required_environment("GITHUB_PAT")
    return environment


def _release_metadata(
    code_dir: Path,
    *,
    push: bool = False,
) -> Optional[Dict[str, str]]:
    """Ask semver-tags what the next release is, and optionally push its tag."""
    semver_tags = _build_semver_tags(code_dir)
    command = [str(semver_tags), "run", "--output_json"]
    if push:
        # The job checked out a commit, not a branch, so push only the tags
        environment = _git_push_environment(code_dir)
        command += ["--branch", ""]
    else:
        environment = _go_environment()
        command.append("--dry_run")

    try:
        result = _run(
            command,
            cwd=code_dir,
            env=environment,
            capture_output=True,
        )
    except subprocess.CalledProcessError as error:
        raise _semver_failure(error) from error

    output_lines = [
        line.strip()
        for line in (result.stdout + "\n" + result.stderr).splitlines()
        if line.strip()
    ]
    for line in reversed(output_lines):
        try:
            metadata = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(metadata, dict):
            continue
        if str(metadata.get("New_release_published", "")).lower() != "true":
            return None
        tag = str(metadata.get("New_release_git_tag", ""))
        if not RELEASE_TAG_PATTERN.fullmatch(tag):
            raise RuntimeError(f"semver-tags returned an invalid release tag: {tag}")
        return {
            "tag": tag,
            "notes": str(metadata.get("New_release_notes", "")).strip(),
        }
    raise RuntimeError("semver-tags did not return release metadata")


def _create_draft_release(
    repository: str,
    source_sha: str,
    metadata: Dict[str, str],
) -> Dict[str, Any]:
    """Create the draft that authorizes a later tag-push release workflow."""
    notes = metadata["notes"] or "Released by Reactorcide CI."
    body = f"{notes}\n\n{_release_marker(source_sha)}"
    encoded_repository = urllib.parse.quote(repository, safe="/")
    return _github_request(
        "POST",
        f"/repos/{encoded_repository}/releases",
        payload={
            "tag_name": metadata["tag"],
            "target_commitish": source_sha,
            "name": metadata["tag"],
            "body": body,
            "draft": True,
            "prerelease": False,
        },
    )


def tag_release(code_dir: Path) -> None:
    """Create a draft release, then push its semver tag with CI credentials."""
    repository = _release_repository()
    source_sha = _source_sha(code_dir)
    metadata = _release_metadata(code_dir)
    if metadata is None:
        log_stdout("No release is required for this source commit")
        return

    release = _find_draft_release(repository, source_sha)
    if release:
        if release.get("tag_name") != metadata["tag"]:
            raise RuntimeError(
                "The existing draft release tag does not match semver-tags"
            )
        log_stdout(f"Reusing draft release {release['tag_name']}")
    else:
        release = _create_draft_release(repository, source_sha, metadata)
        log_stdout(f"Created draft release {release['tag_name']}")

    pushed = _release_metadata(code_dir, push=True)
    if pushed is None or pushed["tag"] != metadata["tag"]:
        raise RuntimeError("semver-tags did not push the planned release tag")
    log_stdout(f"Pushed release tag {pushed['tag']}")


def prepare_release(code_dir: Path) -> None:
    """Verify that a CI-created draft authorizes this tag-push release."""
    event_type = _required_environment("REACTORCIDE_EVENT_TYPE")
    if event_type != "tag_created":
        raise RuntimeError("Release preparation requires a tag_created event")

    tag = _required_environment("REACTORCIDE_BRANCH")
    if not RELEASE_TAG_PATTERN.fullmatch(tag):
        raise RuntimeError(f"The pushed tag is not a release tag: {tag}")

    source_sha = _source_sha(code_dir)
    release = _release_for_source(code_dir)
    if release is None:
        raise RuntimeError(
            "No CI-created draft release authorizes this tag and source commit"
        )
    if release.get("tag_name") != tag:
        raise RuntimeError("The pushed tag does not match the CI-created draft release")

    tag_result = _run(
        ["git", "rev-parse", f"refs/tags/{tag}^{{commit}}"],
        cwd=code_dir,
        capture_output=True,
    )
    if tag_result.stdout.strip() != source_sha:
        raise RuntimeError(
            "The pushed tag does not point at the checked-out source commit"
        )
    log_stdout(f"Verified CI-created draft release {tag}")


def _platform() -> tuple[str, str]:
    value = _required_environment("SEMVER_TAGS_RELEASE_PLATFORM")
    if not PLATFORM_PATTERN.fullmatch(value):
        raise RuntimeError(f"Unsupported release platform: {value}")
    os_name, arch = value.split("/", 1)
    return os_name, arch


def _release_asset_paths(
    code_dir: Path,
    version: str,
    os_name: str,
    arch: str,
) -> List[Path]:
    """Build one platform binary and return every archive to upload for it."""
    release_dir = BUILD_DIR / "assets"
    release_dir.mkdir(parents=True, exist_ok=True)
    binary_name = "semver-tags.exe" if os_name == "windows" else "semver-tags"
    binary_path = release_dir / binary_name
    _run(
        [
            "go",
            "build",
            "-buildvcs=false",
            "-trimpath",
            "-o",
            str(binary_path),
            ".",
        ],
        cwd=code_dir,
        env=_go_environment(os_name, arch),
    )

    asset_paths = []
    archive_stem = f"semver-tags-{version}-{os_name}-{arch}"
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
        with tarfile.open(archive_path, "w:gz") as archive:
            archive.add(binary_path, arcname=binary_name)
    asset_paths.append(archive_path)

    if (os_name, arch) == LEGACY_PLATFORM:
        legacy_path = release_dir / LEGACY_ASSET_NAME
        with tarfile.open(legacy_path, "w:gz") as archive:
            archive.add(binary_path, arcname=binary_name)
        asset_paths.append(legacy_path)

    binary_path.unlink()
    return asset_paths


def _upload_release_asset(
    repository: str,
    release: Dict[str, Any],
    asset_path: Path,
) -> None:
    release_id = int(release["id"])
    encoded_repository = urllib.parse.quote(repository, safe="/")
    assets = _github_request(
        "GET",
        f"/repos/{encoded_repository}/releases/{release_id}/assets?per_page=100",
    )
    for asset in assets:
        if asset.get("name") == asset_path.name:
            _github_request(
                "DELETE",
                f"/repos/{encoded_repository}/releases/assets/{int(asset['id'])}",
            )

    upload_url = str(release["upload_url"]).split("{", 1)[0]
    upload_url = f"{upload_url}?{urllib.parse.urlencode({'name': asset_path.name})}"
    _github_request(
        "POST",
        upload_url,
        payload=asset_path.read_bytes(),
        content_type="application/octet-stream",
    )
    log_stdout(f"Uploaded release asset {asset_path.name}")


def build_release_cli(code_dir: Path) -> None:
    """Build and upload the archives for one platform."""
    repository = _release_repository()
    release = _release_for_source(code_dir)
    if release is None:
        log_stdout("No draft release exists; skipping CLI build")
        return

    os_name, arch = _platform()
    version = str(release["tag_name"]).removeprefix("v")
    for asset_path in _release_asset_paths(code_dir, version, os_name, arch):
        _upload_release_asset(repository, release, asset_path)


def _expected_asset_names(tag: str) -> set[str]:
    version = tag.removeprefix("v")
    names = {LEGACY_ASSET_NAME}
    for os_name, arch in PLATFORMS:
        suffix = "zip" if os_name == "windows" else "tar.gz"
        names.add(f"semver-tags-{version}-{os_name}-{arch}.{suffix}")
    return names


def publish_release(code_dir: Path) -> None:
    """Publish the draft after every platform archive is uploaded."""
    repository = _release_repository()
    release = _release_for_source(code_dir)
    if release is None:
        log_stdout("No draft release exists; nothing to publish")
        return

    release_id = int(release["id"])
    encoded_repository = urllib.parse.quote(repository, safe="/")
    assets = _github_request(
        "GET",
        f"/repos/{encoded_repository}/releases/{release_id}/assets?per_page=100",
    )
    actual_names = {str(asset.get("name")) for asset in assets}
    missing_names = sorted(_expected_asset_names(str(release["tag_name"])) - actual_names)
    if missing_names:
        raise RuntimeError(
            "The draft release is missing required assets: " + ", ".join(missing_names)
        )

    published = _github_request(
        "PATCH",
        f"/repos/{encoded_repository}/releases/{release_id}",
        payload={"draft": False},
    )
    log_stdout(f"Published release {published['tag_name']}")


RELEASE_JOBS = {
    "tag": tag_release,
    "prepare": prepare_release,
    "cli": build_release_cli,
    "publish": publish_release,
}


class SemverTagsReleaseJobsPlugin(Plugin):
    """Run one selected release job after source preparation."""

    def __init__(self):
        super().__init__(name="semver_tags_release_jobs", priority=60)

    def supported_phases(self):
        return [PluginPhase.POST_SOURCE_PREP]

    def execute(self, context: PluginContext) -> None:
        if context.phase != PluginPhase.POST_SOURCE_PREP:
            return

        job_name = os.environ.get("SEMVER_TAGS_RELEASE_JOB", "").strip()
        if not job_name:
            return
        job = RELEASE_JOBS.get(job_name)
        if job is None:
            names = ", ".join(sorted(RELEASE_JOBS))
            raise RuntimeError(
                f"Unknown SEMVER_TAGS_RELEASE_JOB '{job_name}'. Valid jobs: {names}"
            )

        code_dir = Path(context.config.code_dir)
        if not code_dir.is_dir():
            raise RuntimeError(f"Code directory does not exist: {code_dir}")

        log_stdout(f"Starting release lifecycle job: {job_name}")
        job(code_dir)
        log_stdout(f"Completed release lifecycle job: {job_name}")
