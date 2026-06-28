import type { ReactNode } from "react";
import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";

const KEYS: { key: ReactNode; action: ReactNode }[] = [
  { key: <InlineCode>Type</InlineCode>, action: "Fuzzy search sessions and windows." },
  { key: <InlineCode>{"<C-j>"}</InlineCode>, action: "Move down to the next selectable row." },
  { key: <InlineCode>{"<C-k>"}</InlineCode>, action: "Move up to the previous selectable row." },
  { key: <InlineCode>Enter</InlineCode>, action: "Restore the selected session or window." },
  {
    key: (
      <>
        <InlineCode>Esc</InlineCode>, <InlineCode>{"<C-c>"}</InlineCode>,{" "}
        <InlineCode>{"<C-q>"}</InlineCode>
      </>
    ),
    action: "Cancel and close the picker.",
  },
  { key: <InlineCode>{"<C-d>"}</InlineCode>, action: "Delete window under cursor." },
  {
    key: <InlineCode>{"<Alt-d>"}</InlineCode>,
    action: 'Delete the session that owns the window under the cursor. Confirm by typing "y".',
  },
  { key: <InlineCode>{"<C-r>"}</InlineCode>, action: "Rename window under cursor." },
  {
    key: <InlineCode>{"<Alt-r>"}</InlineCode>,
    action: "Rename the session that owns the window under the cursor.",
  },
  {
    key: <InlineCode>{"<C-n>"}</InlineCode>,
    action: "Create new window in session under cursor. Enter window name.",
  },
  { key: <InlineCode>{"<Alt-n>"}</InlineCode>, action: "Create new session. Enter session name." },
  {
    key: <InlineCode>{"<Alt-w>"}</InlineCode>,
    action: "Wakeup: Restore a saved session that is not currently running.",
  },
  {
    key: <InlineCode>{"<Alt-s>"}</InlineCode>,
    action: "Sleep: Save session state and close a running session.",
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

        <video controls autoPlay muted loop preload="metadata" width="100%">
          <source src="/assets/demo-tui.mp4" type="video/mp4" />
          Your browser does not support the video tag.
        </video>

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
