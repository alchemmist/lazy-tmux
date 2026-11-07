import { Seo } from "../components/Seo";
import { InlineCode } from "../components/InlineCode";
import { CodeBlock } from "../components/CodeBlock";

export function Integrations() {
  return (
    <>
      <Seo
        title="Integrations — lazy-tmux"
        description="Setup, session restoration, and limitations for Pi, Claude Code, Codex, and Antex integrations."
        slug="integrations"
      />
      <section className="doc-section">
        <h1>Integrations</h1>
        <p>
          lazy-tmux saves the exact conversation associated with an agent in a
          tmux pane, then resumes that conversation instead of starting a new one.
          Pi, Claude Code, Codex, and Antex are independent clients: their IDs,
          configuration homes, and saved conversations are never converted into
          another client&apos;s sessions.
        </p>
        <p>
          Each integration has its own <InlineCode>enabled</InlineCode> and
          {" "}<InlineCode>home</InlineCode> settings under
          {" "}<InlineCode>[integrations.pi]</InlineCode>,
          {" "}<InlineCode>[integrations.claude]</InlineCode>,
          {" "}<InlineCode>[integrations.codex]</InlineCode>, or
          {" "}<InlineCode>[integrations.antex]</InlineCode>.
          The configured home must match the one used by the client.
        </p>
        <nav aria-label="Agent integrations">
          <ul>
            <li><a href="#pi">Pi</a></li>
            <li><a href="#claude">Claude Code</a></li>
            <li><a href="#codex">Codex</a></li>
            <li><a href="#antex">Antex</a></li>
          </ul>
        </nav>

        <h2 id="pi">Pi</h2>
        <h3>Setup</h3>
        <p>
          Pi&apos;s identity extension is embedded in lazy-tmux and automatically
          installed or updated during normal setup, bootstrap, daemon, picker,
          save, and restore workflows. It lives at
          {" "}<InlineCode>&lt;pi-home&gt;/extensions/lazy-tmux-pi.ts</InlineCode>
          {" "}(default home: <InlineCode>~/.pi/agent</InlineCode>).
          No npm package or manual file copying is needed. To install explicitly:
        </p>
        <CodeBlock>lazy-tmux integrations setup pi</CodeBlock>
        <p>
          New Pi clients load the extension automatically. Clients already
          running before installation need a restart or
          {" "}<InlineCode>/reload</InlineCode> <strong>when idle</strong>.
          lazy-tmux does not reload or restart your agents. Pi&apos;s extension
          and project trust restrictions still apply.
        </p>
        <h3>Restoration and limitations</h3>
        <p>
          The extension publishes the exact session file and owner process on
          startup, session switches, forks, and after turns. lazy-tmux validates
          the session&apos;s JSONL header and restores with
          {" "}<InlineCode>pi --session &lt;file&gt;</InlineCode>. It does not guess
          the newest file in a directory or read identity from terminal output.
        </p>
        <p>
          <InlineCode>--no-session</InlineCode>, non-session print/RPC invocations,
          missing session files, and ambiguous pane ownership cannot establish
          a restorable identity. Preserve the session files when moving or
          restoring your environment.
        </p>
        <p>
          For custom locations, use <InlineCode>PI_CODING_AGENT_DIR</InlineCode>
          {" "}and <InlineCode>PI_CODING_AGENT_SESSION_DIR</InlineCode>.
          Explicit <InlineCode>[integrations.pi]</InlineCode>
          {" "}<InlineCode>home</InlineCode> and
          {" "}<InlineCode>session_dir</InlineCode> settings take precedence.
        </p>
        <p>
          <InlineCode>lazy-tmux integrations setup pi --uninstall</InlineCode>
          {" "}removes the managed extension and opts out of automatic installation;
          explicit setup re-enables it. Set
          {" "}<InlineCode>[integrations.pi] enabled = false</InlineCode> to disable
          the integration altogether. After upgrading lazy-tmux, also restart any
          running autosave daemon so it uses the new binary.
        </p>

        <h2 id="claude">Claude Code</h2>
        <img
          src="/assets/claude.gif"
          alt="lazy-tmux picker showing live Claude Code activity states"
          style={{ width: "100%", height: "auto", borderRadius: 8 }}
        />
        <h3>Setup</h3>
        <p>
          Install lifecycle hooks using an installed, stable lazy-tmux executable,
          then review and trust them in Claude Code:
        </p>
        <CodeBlock>lazy-tmux integrations setup claude</CodeBlock>
        <p>
          Hooks are merged into <InlineCode>&lt;claude-home&gt;/settings.json</InlineCode>
          {" "}(default: <InlineCode>~/.claude/settings.json</InlineCode>), preserving
          other settings and hooks and keeping a
          {" "}<InlineCode>.lazy-tmux.bak</InlineCode> backup.
          <InlineCode> lazy-tmux claude-hooks</InlineCode> is a compatibility alias
          for setup; use <InlineCode>lazy-tmux integrations setup claude --uninstall</InlineCode>
          {" "}to remove owned hooks. Installation does not bypass client trust or
          administrator policies.
        </p>
        <h3>Restoration and limitations</h3>
        <p>
          Claude Code resumes with
          {" "}<InlineCode>claude --resume &lt;session-id&gt;</InlineCode>.
          Lifecycle hooks establish the exact conversation identity and publish
          live activity. Older <InlineCode>hook claude-status</InlineCode> files
          are not used for identity or status.
        </p>
        <p>
          Hooks must run as descendants of the local Claude client that owns the
          pane. Remote or detached processes, nested agents, and suspended clients
          cannot establish ownership merely by supplying a conversation ID.
          Missing or untrusted hooks leave the identity unverified; lazy-tmux
          does not select a conversation by working directory.
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

        <h2 id="antex">Antex</h2>
        <h3>Setup</h3>
        <p>
          Antex publishes a native tmux binding; no lifecycle hook installation
          is needed. Use <strong>Antex 0.1.8 or newer</strong> for native bindings.
          <strong> Antex 0.1.9 or newer</strong> also publishes startup receipts
          used to acknowledge restoration. The integration is enabled by default,
          with <InlineCode>~/.antex</InlineCode> as its default home.
        </p>
        <h3>Restoration and limitations</h3>
        <p>
          A verified Antex pane resumes with
          {" "}<InlineCode>antex resume &lt;session-id&gt;</InlineCode>.
          lazy-tmux uses the ownership-verified binding, not the newest
          conversation sharing a working directory. Live activity is read from
          rollout lifecycle events only after verifying the live owner, session
          ID, and working directory.
        </p>
        <p>
          A missing or stale native binding cannot establish a verified identity.
          Antex and Codex remain separate integrations: lazy-tmux never changes a
          saved Codex conversation into an Antex conversation, or vice versa.
          Existing verified Antex snapshots do not need reconfiguration.
        </p>

        <h2>Live status and troubleshooting</h2>
        <p>
          The picker shows live working, waiting-for-input, waiting-for-decision,
          and idle states when the integration can verify the current owner and
          session. Pi also reports model or compaction errors. A missing live
          status remains unknown rather than reusing stale saved activity.
          <InlineCode> ?</InlineCode> indicates an unverified agent identity;
          <InlineCode> ↻</InlineCode> indicates a pending restoration.
        </p>
        <CodeBlock>{`lazy-tmux integrations doctor
lazy-tmux agent-session --agent codex --pane %1`}</CodeBlock>
        <p>
          Replace the agent and pane in the second command with your target.
          Doctor reports hook/extension installation and per-pane verification;
          installation alone does not certify client trust or compatibility.
        </p>
        <p>
          Unsupported launch options are rejected rather than silently dropped.
          Supported options preserve argument boundaries; initial prompts and
          images are not replayed. Previously saved unverified panes are not
          automatically repaired by installing hooks or upgrading. If you know
          the exact conversation identity, consult the
          {" "}<a href="https://github.com/alchemmist/lazy-tmux#old-snapshots-and-recovery">
            saved-pane repair instructions
          </a>. Manual repair does not prove live process ownership.
        </p>
      </section>
    </>
  );
}
