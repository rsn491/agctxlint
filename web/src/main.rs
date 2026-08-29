mod lint;

use axum::Router;
use axum::extract::State;
use axum::response::Html;
use axum::routing::{get, post};
use std::sync::Arc;

const DEFAULT_HOST: &str = "127.0.0.1";
const DEFAULT_PORT: u16 = 3000;

/// Placeholder in `index.html` that the rendered Google Analytics snippet (or
/// nothing, if tracking is off) gets substituted into.
const GA_PLACEHOLDER: &str = "<!--GA-->";

#[derive(Clone)]
struct AppState {
    index_html: Arc<String>,
}

#[tokio::main]
async fn main() {
    let state = AppState {
        index_html: Arc::new(render_index(ga_measurement_id_from_env().as_deref())),
    };
    let app = Router::new()
        .route("/", get(index))
        .route("/lint", post(lint::handle_lint))
        .with_state(state);

    let addr = bind_addr();
    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .unwrap_or_else(|e| panic!("failed to bind {addr}: {e}"));
    println!(
        "listening on http://{}",
        listener.local_addr().expect("socket has no local addr")
    );
    axum::serve(listener, app).await.expect("server error");
}

/// Builds the listen address from `HOST` and `PORT`. Platforms like Render
/// route traffic to the port they hand the process in `PORT`, and reach it
/// only if the server listens on every interface, so a deploy there sets
/// `HOST=0.0.0.0`. The default stays loopback: `/lint` shells out to
/// `git clone` on a URL the caller supplies, which is not something a dev
/// machine should offer the rest of its network by accident.
fn bind_addr() -> String {
    let host = std::env::var("HOST").unwrap_or_else(|_| DEFAULT_HOST.to_string());
    let port = match std::env::var("PORT") {
        Ok(raw) => raw
            .parse::<u16>()
            .unwrap_or_else(|e| panic!("PORT must be a port number, got {raw:?}: {e}")),
        Err(_) => DEFAULT_PORT,
    };
    format!("{host}:{port}")
}

async fn index(State(state): State<AppState>) -> Html<String> {
    Html((*state.index_html).clone())
}

/// Reads the Google Analytics measurement ID from the environment, deploy to
/// deploy, rather than baking it into the binary: local dev then runs with no
/// tracking by default, and a staging deploy can point at a different GA
/// property just by setting a different value. Rejects anything outside the
/// id's expected charset so a malformed value can't inject arbitrary markup
/// into the page.
fn ga_measurement_id_from_env() -> Option<String> {
    let raw = std::env::var("GA_MEASUREMENT_ID").ok()?;
    let raw = raw.trim();
    if raw.is_empty() {
        return None;
    }
    if !raw
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
    {
        eprintln!("GA_MEASUREMENT_ID {raw:?} contains unexpected characters; ignoring");
        return None;
    }
    Some(raw.to_string())
}

fn render_index(ga_id: Option<&str>) -> String {
    let snippet = match ga_id {
        Some(id) => ga_script(id),
        None => String::new(),
    };
    include_str!("index.html").replace(GA_PLACEHOLDER, &snippet)
}

fn ga_script(id: &str) -> String {
    format!(
        "<script async src=\"https://www.googletagmanager.com/gtag/js?id={id}\"></script>\n\
         <script>\n\
         \x20 window.dataLayer = window.dataLayer || [];\n\
         \x20 function gtag(){{dataLayer.push(arguments);}}\n\
         \x20 gtag('js', new Date());\n\
         \x20 gtag('config', '{id}');\n\
         </script>"
    )
}

#[cfg(test)]
mod tests {
    /// The page reads the report's scores straight out of the `/lint` JSON, so
    /// a rename on the Rust side would silently blank the score widgets. Pin
    /// the field names and the band classes the page depends on.
    #[test]
    fn index_renders_the_report_scores() {
        let html = include_str!("index.html");
        for needle in [
            "summary.score",
            "file.score",
            "score-card",
            "score-badge",
            "score-green",
            "score-yellow",
            "score-orange",
            "score-red",
        ] {
            assert!(html.contains(needle), "index.html is missing {needle:?}");
        }
    }

    use super::{GA_PLACEHOLDER, render_index};

    #[test]
    fn render_index_omits_tracking_when_no_id_given() {
        let html = render_index(None);
        assert!(!html.contains(GA_PLACEHOLDER));
        assert!(!html.contains("gtag"));
    }

    #[test]
    fn render_index_injects_the_given_measurement_id() {
        let html = render_index(Some("G-2S2KMEE05P"));
        assert!(!html.contains(GA_PLACEHOLDER));
        assert!(html.contains("googletagmanager.com/gtag/js?id=G-2S2KMEE05P"));
        assert!(html.contains("gtag('config', 'G-2S2KMEE05P');"));
    }
}
