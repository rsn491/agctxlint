mod lint;

use axum::Router;
use axum::response::Html;
use axum::routing::{get, post};

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/", get(index))
        .route("/lint", post(lint::handle_lint));

    let listener = tokio::net::TcpListener::bind("127.0.0.1:3000")
        .await
        .expect("failed to bind 127.0.0.1:3000");
    println!(
        "listening on http://{}",
        listener.local_addr().expect("socket has no local addr")
    );
    axum::serve(listener, app).await.expect("server error");
}

async fn index() -> Html<&'static str> {
    Html(include_str!("index.html"))
}
