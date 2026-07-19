import { Alert } from "@gravity-ui/uikit";
import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function Configuration() {
  return (
    <>
      <Seo
        title="Configuration — lazy-tmux"
        description="lazy-tmux TOML config file: lookup order, precedence, restore command allow/denylist and restore settle timeout."
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

# Allowlist of commands lazy-tmux may replay on restore. Each entry is a regular
# expression matched against the full command, anchored to the whole line.
# Omit this key to restore every command (default). Provide a list to restore
# only matching commands; use an empty list [] to restore no commands at all.
restore_allowlist = ["nvim( .*)?", "vim( .*)?", "htop", "less .*", "tail .*", "ssh .*"]

# Denylist of commands lazy-tmux must never replay. Each entry is a regular
# expression matched against the full command (anchored). Use this when you trust
# most commands and only want to exclude a few. The denylist wins over the
# allowlist. Omit or leave empty to block nothing (default).
restore_denylist = ["npm .*", "node .*"]

[scrollback]
enabled = false   # capture shell pane scrollback
lines   = 5000    # max scrollback lines per pane`}</CodeBlock>

        <h3 className="cli-subtitle">Restore command allowlist</h3>
        <Alert
          theme="warning"
          className="version-alert"
          title="Breaking change"
          message={
            <>
              Allow/denylist entries are now <strong>regular expressions</strong>{" "}
              matched against the full command, not program names. A bare{" "}
              <InlineCode>nvim</InlineCode> used to match{" "}
              <InlineCode>nvim main.go</InlineCode>; it now matches only the exact
              command <InlineCode>nvim</InlineCode>. Rewrite name-based lists as
              patterns (e.g. <InlineCode>nvim</InlineCode> →{" "}
              <InlineCode>nvim( .*)?</InlineCode>). An invalid regex fails at config
              load.
            </>
          }
        />
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
            <strong>list given</strong> → only commands matching a pattern are
            replayed; any other pane is left at the shell.
          </li>
          <li>
            <strong>
              empty list <InlineCode>[]</InlineCode>
            </strong>{" "}
            → no commands are restored at all.
          </li>
        </ul>
        <p className="muted">
          Each entry is a regular expression matched against the full command,
          anchored end to end. So <InlineCode>nvim( .*)?</InlineCode> matches{" "}
          <InlineCode>nvim</InlineCode> with or without arguments, while a bare{" "}
          <InlineCode>nvim</InlineCode> matches only the exact command{" "}
          <InlineCode>nvim</InlineCode>. Write a literal command to allow exactly
          that command, or wildcard the arguments with <InlineCode>.*</InlineCode>.
        </p>

        <h3 className="cli-subtitle">Restore command denylist</h3>
        <p className="muted">
          The inverse of the allowlist: rather than enumerating everything you
          trust, list only the few programs to block with{" "}
          <InlineCode>restore_denylist</InlineCode>. Handy when you trust almost
          everything and just want to stop a long-running server or a program
          that re-prompts from being replayed. It composes with the allowlist and{" "}
          <strong>wins over it</strong> — a command is replayed only when it is
          not denied and (no allowlist is set or it is allowed). Each entry is a
          regular expression matched against the full command, and an omitted or
          empty list blocks nothing.
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
