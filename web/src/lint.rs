//! Handles `POST /lint`: validates a GitHub URL, clones it into a temp
//! directory, runs the `ctxlint` binary against the clone, and forwards its
//! JSON report to the caller.

use std::path::{Path, PathBuf};
use std::time::Duration;

use axum::Json;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use serde::Deserialize;
use serde_json::{Value, json};

const CLONE_TIMEOUT: Duration = Duration::from_secs(60);
const LINT_TIMEOUT: Duration = Duration::from_secs(30);

const GENERIC_ERROR: &str =
    "Something went wrong while linting that repository. Please try again later.";
const INVALID_URL_ERROR: &str = "That doesn't look like a valid GitHub repository URL. Expected format: https://github.com/<owner>/<repo>";

#[derive(Deserialize)]
pub struct LintRequest {
    url: String,
}

/// The detail behind an error never reaches the client: it may contain
/// internal paths, subprocess stderr, or other implementation details.
/// It's only ever logged server-side; the client gets `public_message`.
pub struct ApiError {
    status: StatusCode,
    log_detail: String,
    public_message: &'static str,
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (self.status, Json(json!({ "error": self.public_message }))).into_response()
    }
}

pub async fn handle_lint(Json(req): Json<LintRequest>) -> Result<Json<Value>, ApiError> {
    let url = req.url.clone();
    println!("POST /lint url={url:?}");

    let result = handle_lint_inner(req).await;

    match &result {
        Ok(_) => println!("POST /lint url={url:?} status={}", StatusCode::OK.as_u16()),
        Err(err) => println!(
            "POST /lint url={url:?} status={} error={:?}",
            err.status.as_u16(),
            err.log_detail
        ),
    }

    result
}

async fn handle_lint_inner(req: LintRequest) -> Result<Json<Value>, ApiError> {
    let clone_url =
        validate_github_url(&req.url).map_err(|detail| bad_request(detail, INVALID_URL_ERROR))?;

    let dir = tempfile::TempDir::new()
        .map_err(|e| server_error(format!("failed to create temp dir: {e}")))?;

    clone_repo(&clone_url, dir.path()).await?;
    let mut report = run_ctxlint(dir.path()).await?;

    if let Some(obj) = report.as_object_mut() {
        obj.insert("url".to_string(), Value::String(clone_url));
    }
    Ok(Json(report))
}

/// Accepts only `https://github.com/<owner>/<repo>[.git][/]`, rejecting
/// anything else. This is a security boundary as much as a UX one: git
/// supports transports like `ext::` that can execute arbitrary commands, so
/// the server must never hand user input to `git clone` unvalidated. Returns
/// a canonicalized clone URL rebuilt from the validated parts (rather than
/// the raw input) so stray query strings, fragments, or credentials in the
/// original can't slip through.
fn validate_github_url(raw: &str) -> Result<String, String> {
    let trimmed = raw.trim();
    let without_slash = trimmed.strip_suffix('/').unwrap_or(trimmed);
    let rest = without_slash
        .strip_prefix("https://github.com/")
        .ok_or("URL must start with https://github.com/")?;

    let parts: Vec<&str> = rest.split('/').collect();
    let [owner, repo] = parts[..] else {
        return Err("URL must look like https://github.com/<owner>/<repo>".to_string());
    };
    let repo = repo.strip_suffix(".git").unwrap_or(repo);

    let valid_segment = |s: &str| {
        !s.is_empty()
            && s.chars()
                .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.')
    };
    if !valid_segment(owner) || !valid_segment(repo) {
        return Err("owner/repo contains invalid characters".to_string());
    }

    Ok(format!("https://github.com/{owner}/{repo}"))
}

async fn clone_repo(url: &str, dest: &Path) -> Result<(), ApiError> {
    let mut cmd = tokio::process::Command::new("git");
    cmd.args(["clone", "--depth", "1", "--", url, &dest.to_string_lossy()])
        // Blocks git's non-http(s) transports (e.g. `ext::`, which can run
        // arbitrary commands) even if validation above were ever bypassed.
        .env("GIT_ALLOW_PROTOCOL", "https")
        .env("GIT_TERMINAL_PROMPT", "0")
        .kill_on_drop(true);

    let output = tokio::time::timeout(CLONE_TIMEOUT, cmd.output())
        .await
        .map_err(|_| {
            bad_request(
                "cloning the repository timed out",
                "Cloning that repository timed out. Please try again.",
            )
        })?
        .map_err(|e| server_error(format!("failed to run git: {e}")))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(bad_request(
            format!("git clone failed: {}", stderr.trim()),
            "Could not clone that repository. Make sure the URL points to a public GitHub repository.",
        ));
    }
    Ok(())
}

async fn run_ctxlint(repo_dir: &Path) -> Result<Value, ApiError> {
    let mut cmd = tokio::process::Command::new(ctxlint_binary_path());
    cmd.args(["--format", "json"])
        .current_dir(repo_dir)
        .kill_on_drop(true);

    let output = tokio::time::timeout(LINT_TIMEOUT, cmd.output())
        .await
        .map_err(|_| server_error("linting the repository timed out".to_string()))?
        .map_err(|e| server_error(format!("failed to run ctxlint: {e}")))?;

    // ctxlint exits 0 (clean) or 1 (findings reported) on a successful run;
    // anything else (2 = usage error, or killed by signal) means the run
    // itself failed and stdout won't be valid JSON.
    match output.status.code() {
        Some(0) | Some(1) => serde_json::from_slice(&output.stdout)
            .map_err(|e| server_error(format!("failed to parse ctxlint output: {e}"))),
        _ => {
            let stderr = String::from_utf8_lossy(&output.stderr);
            Err(server_error(format!("ctxlint failed: {}", stderr.trim())))
        }
    }
}

/// Locates the `ctxlint` binary as a sibling of this binary, since both are
/// built into the same workspace `target/` directory; falls back to `PATH`.
fn ctxlint_binary_path() -> PathBuf {
    let name = if cfg!(windows) {
        "ctxlint.exe"
    } else {
        "ctxlint"
    };
    if let Some(dir) = std::env::current_exe()
        .ok()
        .and_then(|exe| exe.parent().map(Path::to_path_buf))
    {
        let candidate = dir.join(name);
        if candidate.is_file() {
            return candidate;
        }
    }
    PathBuf::from(name)
}

fn bad_request(log_detail: impl Into<String>, public_message: &'static str) -> ApiError {
    ApiError {
        status: StatusCode::BAD_REQUEST,
        log_detail: log_detail.into(),
        public_message,
    }
}

fn server_error(log_detail: impl Into<String>) -> ApiError {
    ApiError {
        status: StatusCode::INTERNAL_SERVER_ERROR,
        log_detail: log_detail.into(),
        public_message: GENERIC_ERROR,
    }
}
