import type { ReactNode } from "react";
import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

const COMMANDS: { cmd: ReactNode; desc: ReactNode }[] = [
  { cmd: <InlineCode>save</InlineCode>, desc: "Save current or selected sessions to disk" },
  {
    cmd: <InlineCode>restore --session NAME</InlineCode>,
    desc: "Restore a single session from disk",
  },
  {
    cmd: <InlineCode>picker</InlineCode>,
    desc: "Open session picker and restore selected session (default: TUI)",
  },
  {
    cmd: <InlineCode>picker --sessions-only</InlineCode>,
    desc: "Open the lightweight sessions-only TUI for a narrow quick-switch popup",
  },
  {
    cmd: <InlineCode>{"bootstrap [--session last|NAME]"}</InlineCode>,
    desc: "Restore one session automatically at tmux startup",
  },
  {
    cmd: <InlineCode>{"daemon [--interval DURATION]"}</InlineCode>,
    desc: "Periodically save all sessions in the background",
  },
  { cmd: <InlineCode>list</InlineCode>, desc: "List saved sessions" },
  {
    cmd: (
      <>
        <InlineCode>version</InlineCode> (<InlineCode>--version</InlineCode>,{" "}
        <InlineCode>-v</InlineCode>)
      </>
    ),
    desc: "Print the version",
  },
  {
    cmd: (
      <>
        <InlineCode>config gen</InlineCode> / <InlineCode>config show</InlineCode>
      </>
    ),
    desc: "Write a base config file, or print the effective config lazy-tmux actually reads",
  },
  {
    cmd: <InlineCode>wakeup --session NAME</InlineCode>,
    desc: "Restore a saved session (lazy load) that is not currently running",
  },
  {
    cmd: <InlineCode>{"sleep --session NAME [--scrollback] [--scrollback-lines N]"}</InlineCode>,
    desc: "Save session state (with optional scrollback) and close a running session",
  },
  {
    cmd: <InlineCode>--fzf-engine</InlineCode>,
    desc: "Use fzf backend instead of built-in TUI",
  },
  {
    cmd: <InlineCode>--fzf-engine --windows</InlineCode>,
    desc: "fzf backend: list and pick a specific window (not just a session), then restore its session focused on it",
  },
  {
    cmd: <InlineCode>--restore-timeout DURATION</InlineCode>,
    desc: (
      <>
        Max wait for restored pane commands to start before the command returns
        (e.g. <InlineCode>5s</InlineCode>; <InlineCode>0</InlineCode> disables
        waiting)
      </>
    ),
  },
  {
    cmd: <InlineCode>--session-sort EXPR</InlineCode>,
    desc: "Session sort (field[:asc|desc],...) fields: last-used, captured, name, windows, panes",
  },
  {
    cmd: <InlineCode>--window-sort EXPR</InlineCode>,
    desc: "Window sort (field[:asc|desc],...) fields: index, name, panes, cmd",
  },
  {
    cmd: <InlineCode>--scrollback</InlineCode>,
    desc: "Capture shell pane scrollback (opt-in)",
  },
  {
    cmd: <InlineCode>--scrollback-lines N</InlineCode>,
    desc: "Maximum captured lines per shell pane (default: 5000)",
  },
];

export function Cli() {
  return (
    <>
      <Seo
        title="CLI reference — lazy-tmux"
        description="lazy-tmux commands and flags: save, restore, picker, daemon, sorting, scrollback and storage."
        slug="cli"
      />
      <section className="doc-section">
        <h1>CLI</h1>

        <table className="cli-table">
          <thead>
            <tr>
              <th>Command / Flag</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {COMMANDS.map((row, i) => (
              <tr key={i}>
                <td>{row.cmd}</td>
                <td>{row.desc}</td>
              </tr>
            ))}
          </tbody>
        </table>

        <h3 className="cli-subtitle">Sorting examples</h3>
        <CodeBlock>{`# Sort sessions by name, then by captured time (newest first)
lazy-tmux picker --session-sort "name:asc,captured:desc"

# Sort windows by pane count, then by name
lazy-tmux picker --window-sort "panes:desc,name:asc"

# Use same sorting with fzf backend
lazy-tmux picker --fzf-engine --session-sort "last-used:desc,name:asc"

# fzf backend, but pick an individual window instead of a whole session
lazy-tmux picker --fzf-engine --windows --window-sort "name:asc"`}</CodeBlock>

        <h3 className="cli-subtitle">Sorting behavior</h3>
        <p className="muted">
          Default directions (when <InlineCode>:asc|:desc</InlineCode> is
          omitted):
        </p>
        <ul>
          <li>
            sessions: <InlineCode>name=asc</InlineCode>, all other session fields ={" "}
            <InlineCode>desc</InlineCode>
          </li>
          <li>
            windows: <InlineCode>index=asc</InlineCode>,{" "}
            <InlineCode>name=asc</InlineCode>, all other window fields ={" "}
            <InlineCode>desc</InlineCode>
          </li>
        </ul>
        <p className="muted">Current defaults (if no sort flags are passed):</p>
        <ul>
          <li>
            sessions:{" "}
            <InlineCode>last-used:desc,captured:desc,name:asc</InlineCode>
          </li>
          <li>
            windows: <InlineCode>index:asc,name:asc</InlineCode>
          </li>
        </ul>
        <p className="muted">Validation behavior:</p>
        <ul>
          <li>unknown fields are rejected with an error.</li>
          <li>
            invalid direction values are rejected (<InlineCode>asc</InlineCode>{" "}
            and <InlineCode>desc</InlineCode> only).
          </li>
          <li>duplicate fields in one expression are rejected.</li>
        </ul>

        <h3 className="cli-subtitle">Scrollback capture</h3>
        <p className="muted">
          By default, scrollback capture is disabled. Enable it explicitly:
        </p>
        <CodeBlock>{`lazy-tmux save --all --scrollback --scrollback-lines 5000
lazy-tmux daemon --interval 5m --scrollback --scrollback-lines 5000`}</CodeBlock>
        <p className="muted">Behavior:</p>
        <ul>
          <li>
            captures tmux pane scrollback only for panes that currently run an
            interactive shell (no detected foreground app command).
          </li>
          <li>
            stores scrollback as sidecar files and references them from session
            snapshots.
          </li>
          <li>
            on restore, writes captured scrollback back into pane tty before
            command replay.
          </li>
        </ul>
        <p className="muted">Storage layout:</p>
        <ul>
          <li>
            <InlineCode>~/.local/share/lazy-tmux/sessions/*.json</InlineCode>
          </li>
          <li>
            <InlineCode>
              {"~/.local/share/lazy-tmux/scrollback/<session>/*.log"}
            </InlineCode>
          </li>
        </ul>

        <h3 className="cli-subtitle">Storage</h3>
        <p className="muted">Default directory:</p>
        <ul>
          <li>
            <InlineCode>~/.local/share/lazy-tmux/index.json</InlineCode>
          </li>
          <li>
            <InlineCode>~/.local/share/lazy-tmux/sessions/*.json</InlineCode>
          </li>
          <li>
            <InlineCode>~/.local/share/lazy-tmux/scrollback/*</InlineCode>
          </li>
        </ul>
        <p className="muted">Override via:</p>
        <ul>
          <li>
            env: <InlineCode>LAZY_TMUX_DATA_DIR</InlineCode>
          </li>
          <li>
            flag: <InlineCode>--data-dir</InlineCode>
          </li>
          <li>
            config: <InlineCode>data_dir</InlineCode> in the TOML config file (see
            the Configuration page)
          </li>
        </ul>
      </section>
    </>
  );
}
