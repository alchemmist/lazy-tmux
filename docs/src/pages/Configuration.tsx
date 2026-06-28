import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function Configuration() {
  return (
    <>
      <Seo
        title="Configuration — lazy-tmux"
        description="lazy-tmux TOML config file: lookup order, precedence, restore command allowlist and restore settle timeout."
        slug="configuration"
      />
      <section className="doc-section">
        <h1>Configuration</h1>

        <p>
          lazy-tmux reads an optional <strong>TOML config file</strong>. Every
          key is optional — without a file, built-in defaults are used. Settings
          are layered, so the most specific source wins:
        </p>
        <p className="muted">built-in defaults → config file → command-line flags</p>
        <p className="muted">
          The file is looked up in this order (first match wins):
        </p>
        <ul>
          <li>
            env: <InlineCode>$LAZY_TMUX_CONFIG</InlineCode> (explicit path)
          </li>
          <li>
            <InlineCode>$XDG_CONFIG_HOME/lazy-tmux/lazy-tmux.toml</InlineCode>
          </li>
          <li>
            <InlineCode>~/.config/lazy-tmux/lazy-tmux.toml</InlineCode>
          </li>
        </ul>
        <p className="muted">
          A missing file is fine; a malformed file or an unknown key fails loudly
          so typos don't go unnoticed.
        </p>

        <CodeBlock>{`# ~/.config/lazy-tmux/lazy-tmux.toml — all keys are optional

tmux_bin        = "tmux"                       # tmux binary to use
data_dir        = "~/.local/share/lazy-tmux"   # where snapshots are stored (~ is expanded)
save_interval   = "5m"                         # daemon autosave interval (Go duration)
restore_timeout = "5s"                         # max wait for restored pane commands to start (0 disables)

# Allowlist of commands lazy-tmux may replay on restore, matched by program name.
# Omit this key to restore every command (default). Provide a list to restore
# only those programs; use an empty list [] to restore no commands at all.
restore_allowlist = ["nvim", "vim", "htop", "less", "tail", "ssh"]

[scrollback]
enabled = false   # capture shell pane scrollback
lines   = 5000    # max scrollback lines per pane`}</CodeBlock>

        <h3 className="cli-subtitle">Restore command allowlist</h3>
        <p className="muted">
          Like tmux-resurrect, you can restrict which commands are replayed on
          restore, so lazy-tmux never re-runs an arbitrary program that happened
          to be active at save time:
        </p>
        <ul>
          <li>
            <strong>key omitted</strong> → every command is restored (default).
          </li>
          <li>
            <strong>list given</strong> → only those programs are replayed; any
            other pane is left at the shell.
          </li>
          <li>
            <strong>
              empty list <InlineCode>[]</InlineCode>
            </strong>{" "}
            → no commands are restored at all.
          </li>
        </ul>
        <p className="muted">
          Matching is by program name, so <InlineCode>nvim</InlineCode> also
          matches <InlineCode>/usr/bin/nvim main.go</InlineCode>.
        </p>

        <h3 className="cli-subtitle">Restore settle timeout</h3>
        <p className="muted">
          On restore, lazy-tmux waits until each pane's command has actually
          started before returning (bounded by{" "}
          <InlineCode>restore_timeout</InlineCode> /{" "}
          <InlineCode>--restore-timeout</InlineCode>), so automation can trust the
          session is fully restored once the command exits. Set it to{" "}
          <InlineCode>0</InlineCode> to opt out and return immediately.
        </p>
      </section>
    </>
  );
}
