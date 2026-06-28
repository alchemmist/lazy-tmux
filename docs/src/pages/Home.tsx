import { Link } from "react-router-dom";
import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";

export function Home() {
  return (
    <>
      <Seo
        title="lazy-tmux: lazy tmux session manager with scrollback restore"
        description="Forget bugs of tmux-resurrect and tmux-continuum. lazy-tmux restores only the sessions you need, preserves scrollback history, and works instantly."
        jsonLd
      />

      <section className="hero">
        <div className="hero__brand">
          <img
            src="/assets/logo-white.svg"
            alt="lazy-tmux logo – lazy tmux session manager with scrollback restore"
          />
          <h1>lazy-tmux</h1>
        </div>
        <p className="subtitle">
          Session manager with scrollback restore. CLI that snapshots tmux
          sessions with running processes and scrollback and restores them
          lazily and seamlessly when you select one.
        </p>
        <div className="actions">
          <a
            className="btn"
            href="https://github.com/alchemmist/lazy-tmux"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub
          </a>
          <Link className="btn alt" to="/installation">
            Install
          </Link>
          <Link className="btn alt" to="/cli">
            Docs
          </Link>
        </div>
        <p className="try-line">
          Try lazy-tmux right now:{" "}
          <InlineCode>docker run -it --rm alchemmist/lazy-tmux:latest</InlineCode>
        </p>
      </section>

      <section className="doc-section">
        <h1>Demo Preview</h1>
        <video controls autoPlay muted loop preload="metadata" width="100%">
          <source src="/assets/demo.mp4" type="video/mp4" />
          Your browser does not support the video tag.
        </video>
        <p>
          We create activity in a temporary tmux session, then stop the tmux
          server and restore the sessions with lazy-tmux. Logs are preserved,
          and a Python HTTP server that ran in another session is restarted. The
          TUI picker shows whether each session is restored, while still letting
          you attach as if it were already loaded.
        </p>
      </section>
    </>
  );
}
