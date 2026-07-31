"""Runnerlib lifecycle jobs for the semver-tags pull-request workflow."""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
from pathlib import Path
from typing import Callable, Dict, List, Optional

from src.logging import log_stdout
from src.plugins import Plugin, PluginContext, PluginPhase


CONVENTIONAL_COMMIT_PATTERN = re.compile(
    r"^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)"
    r"(\(.+\))?!?: .+"
)


def _run(
    command: List[str],
    *,
    cwd: Path,
    capture_output: bool = False,
    env: Optional[Dict[str, str]] = None,
) -> subprocess.CompletedProcess:
    """Run one CI command without a command shell."""
    log_stdout(f"Running: {shlex.join(command)}")
    return subprocess.run(
        command,
        cwd=cwd,
        check=True,
        text=True,
        capture_output=capture_output,
        env=env,
    )


def _go_environment() -> Dict[str, str]:
    """Return Go cache paths that the configured job user can write."""
    environment = os.environ.copy()
    home = Path(environment.get("HOME", "/home/runner"))
    environment["GOPATH"] = str(home / ".cache" / "go")
    environment["GOCACHE"] = str(home / ".cache" / "go-build")
    environment["GOMODCACHE"] = str(home / ".cache" / "go-mod")
    return environment


def _git_ref_exists(code_dir: Path, ref: str) -> bool:
    result = subprocess.run(
        ["git", "rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}"],
        cwd=code_dir,
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return result.returncode == 0


def _merge_base(code_dir: Path, ref: str) -> Optional[str]:
    if not _git_ref_exists(code_dir, ref):
        return None
    result = _run(
        ["git", "merge-base", "HEAD", ref],
        cwd=code_dir,
        capture_output=True,
    )
    value = result.stdout.strip()
    return value or None


def _find_diff_base(code_dir: Path) -> Optional[str]:
    explicit_base = os.environ.get("REACTORCIDE_DIFF_BASE", "").strip()
    if explicit_base:
        if not _git_ref_exists(code_dir, explicit_base):
            raise RuntimeError(
                f"REACTORCIDE_DIFF_BASE is not a commit: {explicit_base}"
            )
        return explicit_base

    base_branch = (
        os.environ.get("REACTORCIDE_BASE_REF")
        or os.environ.get("REACTORCIDE_PR_BASE_REF")
        or "main"
    )
    candidates = (
        f"upstream/{base_branch}",
        f"origin/{base_branch}",
        base_branch,
    )
    for candidate in candidates:
        base = _merge_base(code_dir, candidate)
        if base:
            return base

    if _git_ref_exists(code_dir, "HEAD^"):
        result = _run(
            ["git", "rev-parse", "HEAD^"],
            cwd=code_dir,
            capture_output=True,
        )
        return result.stdout.strip()

    return None


def _commit_records(code_dir: Path, diff_base: Optional[str]) -> List[tuple[str, str]]:
    revision = f"{diff_base}..HEAD" if diff_base else "HEAD"
    result = _run(
        ["git", "log", revision, "--pretty=format:%H%x00%s"],
        cwd=code_dir,
        capture_output=True,
    )
    records = []
    for line in result.stdout.splitlines():
        commit_hash, separator, subject = line.partition("\0")
        if separator:
            records.append((commit_hash, subject))
    return records


def validate_conventional_commits(code_dir: Path) -> None:
    """Validate commit subjects for the current pull-request range."""
    log_stdout("Validating conventional commits")
    diff_base = _find_diff_base(code_dir)
    failed = []

    for commit_hash, subject in _commit_records(code_dir, diff_base):
        if CONVENTIONAL_COMMIT_PATTERN.fullmatch(subject):
            log_stdout(f"OK: {subject}")
        else:
            log_stdout(f"FAIL: {subject} ({commit_hash})")
            failed.append(subject)

    if failed:
        raise RuntimeError(
            "Commit messages must match 'type(scope)?: description'. "
            "Valid types: feat, fix, docs, style, refactor, perf, test, "
            "build, ci, chore, and revert."
        )

    log_stdout("All commits follow the conventional commit format.")


def test_go(code_dir: Path) -> None:
    """Build, vet, and test the semver-tags module."""
    environment = _go_environment()
    _run(["go", "build", "./..."], cwd=code_dir, env=environment)
    _run(["go", "vet", "./..."], cwd=code_dir, env=environment)
    _run(
        ["go", "test", "-race", "./...", "-count=1"],
        cwd=code_dir,
        env=environment,
    )


GOLANGCI_LINT_VERSION = "v2.5.0"


def lint(code_dir: Path) -> None:
    """Check formatting, then run golangci-lint with the repository config."""
    environment = _go_environment()

    result = _run(
        ["gofmt", "-s", "-l", "."],
        cwd=code_dir,
        capture_output=True,
        env=environment,
    )
    unformatted = [line for line in result.stdout.splitlines() if line.strip()]
    if unformatted:
        raise RuntimeError(
            "These files are not formatted. Run 'gofmt -s -w .': "
            + ", ".join(unformatted)
        )

    _run(
        [
            "go",
            "install",
            "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"
            + GOLANGCI_LINT_VERSION,
        ],
        cwd=code_dir,
        env=environment,
    )
    golangci_lint = Path(environment["GOPATH"]) / "bin" / "golangci-lint"
    _run([str(golangci_lint), "run", "./..."], cwd=code_dir, env=environment)


def _last_json_object(output: str) -> Optional[dict]:
    """Return the last JSON object in command output, if there is one."""
    for line in reversed(output.splitlines()):
        line = line.strip()
        if not line:
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            return value
    return None


def dry_run(code_dir: Path) -> None:
    """Run semver-tags against its own history without writing any tag."""
    environment = _go_environment()
    binary = Path("/tmp/semver-tags-ci/semver-tags")
    binary.parent.mkdir(parents=True, exist_ok=True)
    _run(
        ["go", "build", "-o", str(binary), "."],
        cwd=code_dir,
        env=environment,
    )

    result = _run(
        [str(binary), "run", "--dry_run", "--output_json"],
        cwd=code_dir,
        capture_output=True,
        env=environment,
    )
    metadata = _last_json_object(result.stdout + "\n" + result.stderr)
    if metadata is None:
        raise RuntimeError("semver-tags did not return release metadata")
    if metadata.get("Dry_run") != "true":
        raise RuntimeError("semver-tags did not report a dry run")

    log_stdout(
        "Dry run reports tag "
        f"{metadata.get('New_release_git_tag')} "
        f"(published: {metadata.get('New_release_published')})"
    )


CI_JOBS: Dict[str, Callable[[Path], None]] = {
    "conventional-commits": validate_conventional_commits,
    "test-go": test_go,
    "lint": lint,
    "dry-run": dry_run,
}


class SemverTagsCIJobsPlugin(Plugin):
    """Run one selected semver-tags CI job after source preparation."""

    def __init__(self):
        super().__init__(name="semver_tags_ci_jobs", priority=50)

    def supported_phases(self):
        return [PluginPhase.POST_SOURCE_PREP]

    def execute(self, context: PluginContext) -> None:
        if context.phase != PluginPhase.POST_SOURCE_PREP:
            return

        job_name = os.environ.get("SEMVER_TAGS_CI_JOB", "").strip()
        if not job_name:
            return

        job = CI_JOBS.get(job_name)
        if job is None:
            names = ", ".join(sorted(CI_JOBS))
            raise RuntimeError(
                f"Unknown SEMVER_TAGS_CI_JOB '{job_name}'. Valid jobs: {names}"
            )

        code_dir = Path(context.config.code_dir)
        if not code_dir.is_dir():
            raise RuntimeError(f"Code directory does not exist: {code_dir}")

        log_stdout(f"Starting runnerlib lifecycle job: {job_name}")
        job(code_dir)
        log_stdout(f"Completed runnerlib lifecycle job: {job_name}")
