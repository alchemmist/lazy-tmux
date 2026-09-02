import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function TmuxSetup() {
  return (
    <>
      <Seo
        title="Quick tmux setup — lazy-tmux"
        description="Wire lazy-tmux into your tmux config: picker keybind, autosave daemon and save shortcut."
        slug="tmux-setup"
      />
      <section className="doc-section">
        <h1>Quick tmux setup</h1>

        <p>Keep your tmux config clean with an include file:</p>
        <CodeBlock>{`lazy-tmux setup >> ~/.tmux.conf`}</CodeBlock>

        <p>
          This command puts some rules to the end of your tmux config. After
          reloading tmux config:
        </p>
        <ul>
          <li>
            You can call picker with <InlineCode>{"<prefix> + f"}</InlineCode>
          </li>
          <li>
            You can save all sessions with 5000 lines of scrollback with{" "}
            <InlineCode>{"<prefix> + <C-s>"}</InlineCode>
          </li>
          <li>
            A daemon will run on your system, saving all sessions with 5000 lines
            of scrollback every 3 minutes.
          </li>
        </ul>

        <p>
          Feel free to open your config and edit any parameters. For example you
          can configure picker popup size:
        </p>
        <CodeBlock>{`bind-key f display-popup -B -w 65% -h 75% -E 'lazy-tmux picker'`}</CodeBlock>

        <h2>Alt-Tab-style session switching</h2>
        <p>
          The sessions-only picker reads the compact session index and skips window snapshots and
          integration status checks. It starts on the current session, wraps at both ends, and is
          designed for a narrow popup on the left edge. Sessions keep a stable alphabetical order;
          activity and recent use never reshuffle them. Popup bindings require tmux 3.2 or newer:
        </p>
        <CodeBlock>{`bind-key -n C-j display-popup -B -x 0 -y 0 -w 32 -h 100% -E 'lazy-tmux picker --sessions-only'
bind-key -n C-k display-popup -B -x 0 -y 0 -w 32 -h 100% -E 'lazy-tmux picker --sessions-only'`}</CodeBlock>
        <p>
          On tmux 2.9–3.1, open the same picker in a temporary window instead:
        </p>
        <CodeBlock>{`bind-key -n C-j new-window -n quick-switch 'lazy-tmux picker --sessions-only'
bind-key -n C-k new-window -n quick-switch 'lazy-tmux picker --sessions-only'`}</CodeBlock>
        <p>
          Configure your terminal to send <InlineCode>{"Ctrl-J"}</InlineCode> for{" "}
          <InlineCode>{"Cmd-J"}</InlineCode> and <InlineCode>{"Ctrl-K"}</InlineCode> for{" "}
          <InlineCode>{"Cmd-K"}</InlineCode>. The first shortcut opens the popup with the current
          session selected. While it is open, press the shortcut again to move down or up, then{" "}
          <InlineCode>Enter</InlineCode> to switch. lazy-tmux cannot bind the macOS Command key
          directly because tmux receives the key sequence produced by the terminal.
        </p>

        <p>Or edit time interval:</p>
        <CodeBlock>{`run-shell -b 'lazy-tmux daemon --interval 5m --scrollback > /tmp/lazy-tmux.log 2>&1 || tmux display-message "lazy-tmux daemon already running"'`}</CodeBlock>

        <p>Or up scrollback lines limit:</p>
        <CodeBlock>{`run-shell -b 'lazy-tmux daemon --interval 3m --scrollback --scrollback-lines 8000 > /tmp/lazy-tmux.log 2>&1 || tmux display-message "lazy-tmux daemon already running"'`}</CodeBlock>

        <p>Or remap saving shortcut:</p>
        <CodeBlock>{`bind-key C-S run-shell 'lazy-tmux save --all --scrollback && tmux display-message "All sessions saved successfully!"'`}</CodeBlock>
      </section>
    </>
  );
}
