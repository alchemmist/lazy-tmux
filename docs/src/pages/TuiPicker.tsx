import type { ReactNode } from "react";
import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";
import { DemoVideo } from "../components/DemoVideo";

const KEYS: { key: ReactNode; action: ReactNode }[] = [
  { key: <InlineCode>Type</InlineCode>, action: "Fuzzy search sessions and windows." },
  {
    key: <InlineCode>/</InlineCode>,
    action: (
      <>
        Open the command palette: <InlineCode>/delete</InlineCode>, <InlineCode>/rename</InlineCode>,{" "}
        <InlineCode>/new</InlineCode>, <InlineCode>/wake</InlineCode>, <InlineCode>/sleep</InlineCode>.{" "}
        <InlineCode>Tab</InlineCode> completes, <InlineCode>Enter</InlineCode> runs the highlighted
        command and enters its color-coded mode.
      </>
    ),
  },
  {
    key: <InlineCode>?</InlineCode>,
    action:
      "Toggle a full keybinding cheat sheet (only when the search box is empty; any key closes it).",
  },
  { key: <InlineCode>{"<C-j>"}</InlineCode>, action: "Move down to the next selectable row." },
  { key: <InlineCode>{"<C-k>"}</InlineCode>, action: "Move up to the previous selectable row." },
  {
    key: <InlineCode>Enter</InlineCode>,
    action: "Restore the selected target — or, inside a mode, perform that mode's action.",
  },
  {
    key: <InlineCode>Esc</InlineCode>,
    action: "Cancel the active mode and return to browse; in browse mode, close the picker.",
  },
  {
    key: (
      <>
        <InlineCode>{"<C-c>"}</InlineCode>, <InlineCode>{"<C-q>"}</InlineCode>
      </>
    ),
    action: "Cancel and close the picker.",
  },
  {
    key: <InlineCode>Space</InlineCode>,
    action: (
      <>
        In delete mode, mark or unmark the row under the cursor. Marking a session marks all of its
        windows, so a fully-marked session is deleted as a whole.
      </>
    ),
  },
  {
    key: <InlineCode>{"<C-d>"}</InlineCode>,
    action: "Enter delete mode with the window under the cursor already marked.",
  },
  {
    key: <InlineCode>{"<Alt-d>"}</InlineCode>,
    action: "Enter delete mode with the whole session under the cursor marked.",
  },
  {
    key: <InlineCode>{"<C-r>"}</InlineCode>,
    action: "Enter rename mode on the window under the cursor.",
  },
  {
    key: <InlineCode>{"<Alt-r>"}</InlineCode>,
    action: "Enter rename mode on the session that owns the window under the cursor.",
  },
  {
    key: <InlineCode>{"<C-n>"}</InlineCode>,
    action: "Enter new mode to add a window to the session under the cursor.",
  },
  { key: <InlineCode>{"<Alt-n>"}</InlineCode>, action: "Enter new mode to create a fresh session." },
  {
    key: <InlineCode>{"<Alt-w>"}</InlineCode>,
    action: "Enter wake mode (sleeping sessions only): restore a saved session that is not running.",
  },
  {
    key: <InlineCode>{"<Alt-s>"}</InlineCode>,
    action: "Enter sleep mode (live sessions only): save a running session's state and close it.",
  },
];

export function TuiPicker() {
  return (
    <>
      <Seo
        title="TUI picker — lazy-tmux"
        description="Keyboard-driven lazy-tmux TUI picker: navigation and management keybinds."
        slug="tui-picker"
      />
      <section className="doc-section">
        <h1>TUI picker</h1>

        <DemoVideo src="/assets/demo-tui.mp4" />

        <p>
          Actions are organized into <strong>color-coded modes</strong>. Type <InlineCode>/</InlineCode>{" "}
          to open the command palette, or use the shortcut for a mode directly. While a mode is
          active the whole frame recolors (red for delete, blue for rename, green for new, cyan for
          wake/sleep), the list is filtered to the targets that mode can act on, and{" "}
          <InlineCode>Esc</InlineCode> returns to the resting browse mode.
        </p>

        <h2>Claude status dots</h2>
        <p>
          For live sessions, windows running <InlineCode>claude</InlineCode> show a colored status
          glyph in the <strong>State</strong> column, so you can tell at a glance which Claude is
          busy and which one is waiting for you — without switching around. Each state has both a
          distinct shape and color, so it stays readable in monochrome terminals:{" "}
          <strong>◐ green</strong> = working (an animated half-circle spinner),{" "}
          <strong>? amber</strong> = waiting for your answer to a question or permission,{" "}
          <strong>◌ blue</strong> = waiting for input, <strong>○ grey</strong> = idle. The dots
          refresh live while the picker is open, and the working state animates.
        </p>
        <p>
          Working/idle is detected out of the box. For the precise “waiting for your answer” state,
          install the Claude Code hooks once:{" "}
          <InlineCode>lazy-tmux claude-hooks</InlineCode> (merges into{" "}
          <InlineCode>~/.claude/settings.json</InlineCode>, preserving your existing hooks;{" "}
          <InlineCode>lazy-tmux claude-hooks --uninstall</InlineCode> removes them).
        </p>

        <table className="cli-table">
          <thead>
            <tr>
              <th>Key</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody>
            {KEYS.map((row, i) => (
              <tr key={i}>
                <td>{row.key}</td>
                <td>{row.action}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </>
  );
}
