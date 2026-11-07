import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";
import { CodeBlock } from "../components/CodeBlock";

export function Integrations() {
  return (
    <>
      <Seo
        title="Integrations — lazy-tmux"
        description="Program integrations for lazy-tmux save/restore. Codex requires --no-daemon; learn how to configure hooks and a shell alias."
        slug="integrations"
      />
      <section className="doc-section">
        <h1>Integrations</h1>

        <p>
          Integrations teach lazy-tmux about the interactive programs you run in a
          pane. Each one declares how to recognize its program, what
          program-specific state to record on save, what to replay on restore, and
          — optionally — a live status to surface in the picker. They plug into a
          registry, so the core save/restore logic stays untouched.
        </p>

        <h2 id="codex">Codex</h2>
        <p>
          <strong>
            Codex must run with <InlineCode>--no-daemon</InlineCode> for reliable
            session tracking and restoration. The shared daemon mode is not
            supported.
          </strong>{" "}
          Use lazy-tmux <strong>v0.3.6 or newer</strong>, which supports saving
          this flag and preserving it in the resume command.
        </p>
        <p>
          Install the lifecycle hooks once, then review and trust them in Codex:
        </p>
        <CodeBlock>lazy-tmux integrations setup codex</CodeBlock>
        <p>
          Start Codex inside your tmux pane with:
        </p>
        <CodeBlock>codex --no-daemon</CodeBlock>
        <p>
          We recommend adding this alias to <InlineCode>~/.bashrc</InlineCode>
          {" "}(bash) or <InlineCode>~/.zshrc</InlineCode> (zsh), then opening a
          new shell or sourcing that file:
        </p>
        <CodeBlock>{`alias codex='codex --no-daemon'`}</CodeBlock>
        <p>
          The alias applies to interactive shell commands. Scripts, tmux bindings,
          and other launchers that do not expand aliases must pass
          {" "}<InlineCode>--no-daemon</InlineCode> explicitly. lazy-tmux records
          the flag when saving a verified session and restores it with
          {" "}<InlineCode>codex resume &lt;session-id&gt; --no-daemon</InlineCode>,
          preserving any other supported launch options.
        </p>
        <h3>Why shared daemon mode is unsupported</h3>
        <p>
          A shared Codex daemon can outlive the tmux pane that started it. Its
          hooks may inherit that pane&apos;s old environment, and their process
          ancestry no longer identifies the current interactive Codex client.
          Although a hook reports the conversation ID, lazy-tmux cannot reliably
          associate it with the correct pane. This can cause hook failures and
          an <InlineCode>agent identity is unverified</InlineCode> error on
          restore. lazy-tmux refuses automatic resume rather than risk restoring
          the wrong conversation.
        </p>
        <p>
          After switching to <InlineCode>--no-daemon</InlineCode>, start a new
          Codex client and save the pane again once its session is verified.
          Updating lazy-tmux or adding the alias does not repair previously
          saved unverified panes. See{" "}
          <a href="https://github.com/alchemmist/lazy-tmux/issues/325">
            issue #325
          </a>{" "}
          for the daemon-mode limitation.
        </p>

        <h2>Claude Code</h2>

        <img
          src="/assets/claude.gif"
          alt="lazy-tmux TUI picker showing a live status glyph for each Claude Code window: working, waiting for a decision, waiting for input, and idle"
          style={{ width: "100%", height: "auto", borderRadius: 8 }}
        />

        <p>
          A window running <InlineCode>claude</InlineCode> is restored as{" "}
          <InlineCode>claude --resume &lt;session-id&gt;</InlineCode>, so the
          conversation continues instead of starting fresh.
        </p>

        <h2>Antex</h2>
        <p>
          A window running <InlineCode>antex</InlineCode> is restored as{" "}
          <InlineCode>antex resume &lt;session-id&gt;</InlineCode>. On every save,
          lazy-tmux rereads the rollout metadata and selects the newest session
          for that pane&apos;s working directory, so switching sessions is reflected
          in the next snapshot.
        </p>

        <p>
          The integration is enabled by default. Its data directory can be
          changed with <InlineCode>[integrations.antex]</InlineCode> and the{" "}
          <InlineCode>home</InlineCode> option (default:{" "}
          <InlineCode>~/.antex</InlineCode>).
        </p>

        <h3>Status dots</h3>
        <p>
          For live sessions, each <InlineCode>claude</InlineCode> window shows a
          colored status glyph in the <strong>State</strong> column, so you can
          tell at a glance which Claude is busy and which one is waiting for you —
          without switching around. Each state has both a distinct shape and
          color, so it stays readable in monochrome terminals:{" "}
          <strong>◐ green</strong> = working (an animated half-circle spinner),{" "}
          <strong>? amber</strong> = waiting for your answer to a question or
          permission, <strong>◌ blue</strong> = waiting for input,{" "}
          <strong>○ grey</strong> = idle. The dots refresh live while the picker
          is open, and the working state animates.
        </p>
        <p>
          Working/idle is detected out of the box. For the precise “waiting for
          your answer” state, install the Claude Code hooks once:{" "}
          <InlineCode>lazy-tmux claude-hooks</InlineCode> (merges into{" "}
          <InlineCode>~/.claude/settings.json</InlineCode>, preserving your
          existing hooks; <InlineCode>lazy-tmux claude-hooks --uninstall</InlineCode>{" "}
          removes them).
        </p>
      </section>
    </>
  );
}
