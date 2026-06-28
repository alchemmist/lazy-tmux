import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function Installation() {
  return (
    <>
      <Seo
        title="Installation — lazy-tmux"
        description="Install lazy-tmux via the install script, Arch (AUR) or Homebrew."
        slug="installation"
      />
      <section className="doc-section">
        <h1>Installation</h1>

        <div className="callout-warn">
          <p className="callout-warn__title">⚠️ tmux version requirement</p>
          <p>
            lazy-tmux requires <strong>tmux 3.6</strong> or{" "}
            <strong>tmux 3.6a</strong>. Older versions may not work correctly.
          </p>
        </div>

        <p>Install with builtin powerful TUI picker:</p>
        <CodeBlock>curl -fsSL https://lazy-tmux.xyz/install.sh | sh</CodeBlock>

        <p>
          Install pure, no-deps, lightweight binary (
          <InlineCode>fzf</InlineCode> required):
        </p>
        <CodeBlock>
          curl -fsSL https://lazy-tmux.xyz/install.sh | sh -s -- --fzf-engine
        </CodeBlock>

        <p>Or use your package manager. Arch:</p>
        <CodeBlock>yay -S lazy-tmux</CodeBlock>

        <p>MacOS (Homebrew):</p>
        <CodeBlock>brew install alchemmist/tap/lazy-tmux</CodeBlock>
      </section>
    </>
  );
}
