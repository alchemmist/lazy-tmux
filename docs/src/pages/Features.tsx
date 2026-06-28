import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";

export function Features() {
  return (
    <>
      <Seo
        title="Features — lazy-tmux"
        description="What lazy-tmux can do: lazy restore, scrollback capture, TUI picker, autosave daemon, config file and restore allowlist."
        slug="features"
      />
      <section className="doc-section">
        <h1>Features</h1>
        <ul>
          <li>
            Save current session, or a specific session, or all sessions to disk
            using <InlineCode>save</InlineCode>. Snapshots preserve windows,
            panes, layouts, running shell commands and{" "}
            <strong>shell scrollback history</strong> for later restoration.
          </li>
          <li>
            <strong>Lazy restore</strong> only the session you pick with{" "}
            <InlineCode>restore</InlineCode> command or interactively with{" "}
            <InlineCode>picker</InlineCode>. You don't need to spend RAM for all
            sessions at startup – unlike tmux-resurrect which restores everything
            at once.
          </li>
          <li>
            Interactive TUI session browser combining a deep tree view of
            sessions, windows, and panes with a table showing additional
            information: active command in each pane, last snapshot time, number
            of windows/panes per session, and session status (restored or not).
            Fuzzy search makes it lightning fast to locate any window or pane.
          </li>
          <li>
            Keyboard-driven picker that lets you search, navigate, and restore
            sessions without leaving tmux.
          </li>
          <li>
            Flexible session and window sorting through{" "}
            <InlineCode>--session-sort</InlineCode> and{" "}
            <InlineCode>--window-sort</InlineCode> flags. Sort by last-used,
            captured time, number of windows/panes, names, commands, or any
            combination.
          </li>
          <li>
            Use <InlineCode>--fzf-engine</InlineCode> to replace the built-in TUI
            with <InlineCode>fzf</InlineCode>. Can be set at install time for a
            lighter binary; note that keyboard-driven session/window control is
            unavailable.
          </li>
          <li>
            Autosave daemon mode periodically snapshots all sessions in the
            background, keeping session state safe across reboots. Only one
            autosave process runs at a time to avoid conflicts.
          </li>
          <li>
            Bootstrap restore at tmux startup allows restoring the latest or a
            specific session automatically, useful for automation after startup.
          </li>
          <li>
            Snapshot includes window and pane structure along with pane commands,
            enabling seamless reconstruction of your working environment. For
            example starting npm dev server, docker-compose, nvim, or any editor.
          </li>
          <li>
            Optional shell pane scrollback capture lets you save and replay
            previous output, preserving context for restored sessions.
          </li>
          <li>
            Configurable via an optional <strong>TOML config file</strong> (
            <InlineCode>~/.config/lazy-tmux/lazy-tmux.toml</InlineCode>): set a{" "}
            <strong>restore command allowlist</strong> so only trusted programs
            are replayed, tune the restore settle timeout, scrollback, autosave
            interval and storage paths.
          </li>
          <li>
            With the fzf backend, <InlineCode>--windows</InlineCode> lists
            individual windows so you can fuzzy-jump straight to any window, not
            just a session.
          </li>
        </ul>
      </section>
    </>
  );
}
