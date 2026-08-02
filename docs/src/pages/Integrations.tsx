import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";

export function Integrations() {
  return (
    <>
      <Seo
        title="Integrations — lazy-tmux"
        description="Program integrations adapt interactive tools to lazy-tmux's save/restore. Claude Code and Codex sessions resume from their current session."
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

        <h2>Codex</h2>
        <p>
          A window running <InlineCode>codex</InlineCode> is restored as{" "}
          <InlineCode>codex resume &lt;session-id&gt;</InlineCode>. On every save,
          lazy-tmux rereads the rollout metadata and selects the newest session
          for that pane&apos;s working directory, so switching sessions is reflected
          in the next snapshot.
        </p>

        <p>
          The integration is enabled by default. Its data directory can be
          changed with <InlineCode>[integrations.codex]</InlineCode> and the{" "}
          <InlineCode>home</InlineCode> option (default:{" "}
          <InlineCode>~/.codex</InlineCode>).
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
