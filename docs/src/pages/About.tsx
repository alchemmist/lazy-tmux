import { Seo } from "../components/Seo";

export function About() {
  return (
    <>
      <Seo
        title="About — lazy-tmux"
        description="Why lazy-tmux exists and the philosophy behind it."
        slug="about"
      />

      <section className="doc-section">
        <h1>Why?</h1>
        <p>
          Most tmux session tools either restore everything at once or stay
          shallow. lazy-tmux is built for real workflows: it restores only what
          you pick, keeps history, and gives you full control without leaving
          tmux.
        </p>
        <ul>
          <li>
            <strong>Lazy restoration:</strong> Only load what you need, keep
            memory and startup time low. No more waiting for all sessions to
            restore at once.
          </li>
          <li>
            <strong>Deep visibility:</strong> Tree + table view shows sessions,
            windows, panes, commands, timestamps, and restore state.
          </li>
          <li>
            <strong>Real control:</strong> Create, rename, delete, save, and
            restore from the picker with fast keyboard flow.
          </li>
          <li>
            <strong>Durable context:</strong> Optional scrollback capture keeps
            logs and output across restarts, so a restored session picks up right
            where it left off.
          </li>
        </ul>
        <p>
          The result is a lightweight, tmux-native workflow that feels instant
          and predictable even across reboots. No bugs, no surprises.
        </p>
      </section>

      <section className="doc-section">
        <h1>Philosophy</h1>
        <p>
          lazy-tmux is built around a simple but powerful idea: save resources
          and free the user from unnecessary overhead. Each session is restored
          only when it's actually needed, and all processes start seamlessly,
          without pauses or conflicts.
        </p>
        <p>
          Everything is written in Go for lightning-fast execution. This is not a
          wrapper or a "combo" over tmux — it's a tool that fits perfectly into
          its ecosystem. No extra configuration, no duplicated responsibilities,
          no system bloat. Just a fast, reliable, and predictable way to manage
          sessions.
        </p>
        <p>
          The focus is on allowing users to work naturally with tmux while
          lazy-tmux quietly handles session snapshots and restoration in the
          background, preserving context and performance without getting in the
          way.
        </p>
      </section>
    </>
  );
}
