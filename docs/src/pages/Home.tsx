import type { MouseEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@gravity-ui/uikit";
import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";
import { DemoVideo } from "../components/DemoVideo";

// A modified click (middle/right button or a modifier key) should keep the
// link's native behavior, e.g. open in a new tab, instead of client-routing.
function isModifiedClick(event: MouseEvent) {
  return (
    event.button !== 0 ||
    event.metaKey ||
    event.altKey ||
    event.ctrlKey ||
    event.shiftKey
  );
}

export function Home() {
  const navigate = useNavigate();

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
          <Button
            view="action"
            size="xl"
            href="https://github.com/alchemmist/lazy-tmux"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub
          </Button>
          <Button
            view="outlined"
            size="xl"
            href="/installation"
            onClick={(event) => {
              if (isModifiedClick(event)) {
                return;
              }
              event.preventDefault();
              navigate("/installation");
            }}
          >
            Install
          </Button>
          <Button
            view="outlined"
            size="xl"
            href="/cli"
            onClick={(event) => {
              if (isModifiedClick(event)) {
                return;
              }
              event.preventDefault();
              navigate("/cli");
            }}
          >
            Docs
          </Button>
        </div>
        <p className="try-line">
          Try lazy-tmux right now:{" "}
          <InlineCode>docker run -it --rm alchemmist/lazy-tmux:latest</InlineCode>
        </p>
      </section>

      <section className="doc-section">
        <h1>Demo Preview</h1>
        <DemoVideo src="/assets/demo.mp4" />
        <p>
          We create activity in a temporary tmux session, then stop the tmux
          server and restore the sessions with lazy-tmux. Logs are preserved,
          and a Python HTTP server that ran in another session is restarted. The
          TUI picker shows whether each session is restored, while still letting
          you attach as if it were already loaded.
        </p>
        <img
          src="/assets/demo.gif"
          alt="lazy-tmux: save sessions, kill the tmux server, then restore from the TUI picker"
          style={{ width: "100%", height: "auto", borderRadius: 8 }}
        />
        <p>
          Snapshot three sessions, run <InlineCode>tmux kill-server</InlineCode> to
          wipe everything, then bring it all back by picking a session in the TUI.
        </p>
      </section>
    </>
  );
}
