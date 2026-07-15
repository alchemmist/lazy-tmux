import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function Configuration() {
  return (
    <>
      <Seo
        title="Configuration — lazy-tmux"
        description="lazy-tmux TOML config file: lookup order, precedence, restore handler, command allow/denylist, and settle timeout."
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
restore_handler = ""                           # optional handler run instead of direct replay
restore_handler_source = "saved"               # saved | resolved (exact, case-sensitive)
# restore_handler = "echo"
# restore_handler = "cowsay -f tux"

# Allowlist of commands lazy-tmux may replay or handle on restore. Patterns are
# anchored against the complete selected command. Omit this key to allow every
# command (default); use an empty list [] to allow no commands at all.
restore_allowlist = ["nvim( .*)?", "vim( .*)?", "htop", "less .*", "tail .*", "ssh .*"]

# Denylist of commands lazy-tmux must never replay or handle, using the same
# complete-command matching.
# Use this instead of an allowlist when you trust most commands and only want to
# exclude a few. The denylist wins over the allowlist. Omit or leave empty to
# block nothing (default).
restore_denylist = ["npm( .*)?", "node( .*)?"]

[scrollback]
enabled = false   # capture shell pane scrollback
lines   = 5000    # max scrollback lines per pane`}</CodeBlock>

        <h3 className="cli-subtitle">Restore handler</h3>
        <p className="muted">
          <InlineCode>restore_handler</InlineCode> is empty by default, which
          preserves direct application replay. A non-empty value is a trusted
          shell prefix that runs instead of direct replay. lazy-tmux appends the
          selected command source as one safely single-quoted argument, so
          values such as <InlineCode>echo</InlineCode> and{" "}
          <InlineCode>cowsay -f tux</InlineCode> receive the selected command as a
          single argument. Surrounding whitespace is trimmed, but the prefix is
          not otherwise parsed or home-expanded.
        </p>
        <p className="muted">
          <InlineCode>restore_handler_source</InlineCode> accepts only the exact,
          case-sensitive values <InlineCode>saved</InlineCode> and{" "}
          <InlineCode>resolved</InlineCode>; omission defaults to{" "}
          <InlineCode>saved</InlineCode>. Saved mode passes the normalized
          snapshot command and suppresses integration output. Resolved mode uses
          non-empty integration resolver output, then falls back to the normalized
          snapshot command. A non-empty shell-only resolver result is selected and
          skipped rather than falling back again. The setting does nothing when
          the handler is empty, so direct replay behavior is unchanged.
        </p>
        <p className="muted">
          For terminal safety, C0 control bytes and DEL in the selected command are
          represented visibly as lowercase <InlineCode>\xNN</InlineCode> text
          instead of being passed through exactly. Printable text and Unicode
          remain unchanged. The complete safely quoted handler line is sent as
          literal tmux input, followed by a separate Enter dispatch.
        </p>
        <p className="muted">
          Both source modes leave empty and shell-only selected commands untouched
          and do not wait for{" "}
          <InlineCode>restore_timeout</InlineCode>. Handler shell parse errors,
          missing executables, and non-zero exits appear in the pane but cannot
          affect lazy-tmux&apos;s exit code; tmux dispatch failures are still
          returned.
        </p>

        <h3 className="cli-subtitle">Restore command allowlist</h3>
        <p className="muted">
          Like tmux-resurrect, you can restrict which commands are replayed on
          restore, so lazy-tmux never re-runs an arbitrary program that happened
          to be active at save time:
        </p>
        <ul>
          <li>
            <strong>key omitted</strong> → every eligible command is replayed or
            handled (default).
          </li>
          <li>
            <strong>list given</strong> → only matching commands are replayed or
            handled; any other pane is left at the shell.
          </li>
          <li>
            <strong>
              empty list <InlineCode>[]</InlineCode>
            </strong>{" "}
            → no commands are replayed or handled at all.
          </li>
        </ul>
        <p className="muted">
          Each regular expression is anchored against the complete selected
          command string. In direct mode that is the effective replay command,
          including integration output. In handler mode it is the saved or
          resolved source before encoding and before the handler is added; the
          handler invocation itself is never matched. Therefore{" "}
          <InlineCode>nvim</InlineCode> matches only that exact command; use{" "}
          <InlineCode>nvim( .*)?</InlineCode> to also permit arguments. A path
          such as <InlineCode>/usr/bin/nvim main.go</InlineCode> requires a
          pattern that includes the path.
        </p>
        <p className="muted">
          For a Claude pane, <InlineCode>claude</InlineCode> permits the saved
          source but not <InlineCode>claude --resume sess-9</InlineCode>, while{" "}
          <InlineCode>claude --resume .*</InlineCode> permits the resolved command
          but not the saved source. Claude Code is the only current production
          integration; with <InlineCode>claude.session_id</InlineCode> metadata,
          resolved mode can pass <InlineCode>claude --resume &lt;session-id&gt;</InlineCode>.
        </p>

        <h3 className="cli-subtitle">Restore command denylist</h3>
        <p className="muted">
          The inverse of the allowlist: rather than enumerating everything you
          trust, list only the few programs to block with{" "}
          <InlineCode>restore_denylist</InlineCode>. Handy when you trust almost
          everything and just want to stop a long-running server or a program
          that re-prompts from being replayed. It composes with the allowlist and{" "}
          <strong>wins over it</strong> — a command is replayed only when it is
          not denied and (no allowlist is set or it is allowed). It uses the
          same anchored, complete-command matching as the allowlist; an omitted
          or empty list blocks nothing. Like the allowlist, it checks the selected
          source and never the generated handler invocation.
        </p>

        <h3 className="cli-subtitle">Restore settle timeout</h3>
        <p className="muted">
          On restore, lazy-tmux waits until each pane's command has actually
          started before returning (bounded by{" "}
          <InlineCode>restore_timeout</InlineCode> /{" "}
          <InlineCode>--restore-timeout</InlineCode>), so automation can trust the
          session is fully restored once the command exits. Set it to{" "}
          <InlineCode>0</InlineCode> to opt out and return immediately. A non-empty
          handler is asynchronous and never waits in either source mode; when the
          handler is empty, direct resolver, filtering, and settle behavior remain
          unchanged.
        </p>
      </section>
    </>
  );
}
