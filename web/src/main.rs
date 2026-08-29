mod lint;

use axum::Router;
use axum::response::Html;
use axum::routing::{get, post};

const DEFAULT_HOST: &str = "127.0.0.1";
const DEFAULT_PORT: u16 = 3000;

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/", get(index))
        .route("/lint", post(lint::handle_lint));

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

async fn index() -> Html<&'static str> {
    Html(include_str!("index.html"))
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
}
